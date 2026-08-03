package relay

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/execution"
	gatewayretry "done-hub/internal/gateway/retry"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/metrics"
	"done-hub/model"
	providersBase "done-hub/providers/base"
	"done-hub/relay/relay_util"
	"done-hub/types"

	"github.com/gin-gonic/gin"
)

// executeRelayGateway is the production HTTP/SSE boundary for GatewayEngine.
// Protocol-specific relay implementations remain capability runners; selection,
// retry, cooldown, output gating and attempt observation are owned here.
func executeRelayGateway(relay RelayBaseInterface) *types.OpenAIErrorWithStatusCode {
	c := relay.getContext()
	request := domain.RequestContext{
		RequestID:      c.GetString(logger.RequestIdKey),
		UserID:         c.GetInt("id"),
		TokenID:        c.GetInt("token_id"),
		Group:          c.GetString("token_group"),
		RequestedModel: relay.getOriginalModel(),
		Capability:     capabilityForPath(c.Request.URL.Path),
		Protocol:       inboundProtocol(c),
		Stream:         relay.IsStream(),
		StartedAt:      c.GetTime("requestStartTime"),
	}
	plan := domain.RoutePlan{
		Request: request,
		Model: domain.ModelRoute{
			PublicModel: relay.getOriginalModel(),
			Capability:  request.Capability,
			Protocol:    request.Protocol,
		},
	}

	selector := &relayGatewaySelector{relay: relay}
	runner := &relayGatewayRunner{relay: relay}
	coordinator := execution.NewGatewayEngine(gatewayretry.DefaultPolicy())
	coordinator.Selector = selector
	coordinator.Cooldowns = &relayCooldownStore{c: c}
	coordinator.Observer = &relayAttemptObserver{c: c}
	coordinator.MaxAttempts = config.RetryTimes + 1
	if coordinator.MaxAttempts < 1 {
		coordinator.MaxAttempts = 1
	}
	if config.RetryTimeOut > 0 {
		coordinator.Deadline = time.Duration(config.RetryTimeOut) * time.Second
	}

	result := coordinator.Run(c.Request.Context(), plan, runner)
	gatewayRequestState(c).SetAttemptCount(len(result.Attempts))
	if result.Err == nil {
		metrics.RecordProvider(c, http.StatusOK)
		return nil
	}
	if runner.gatewayBilling != nil {
		if err := runner.gatewayBilling.RefundIfReserved(c.Request.Context(), "gateway_attempts_failed"); err != nil {
			logger.LogError(c.Request.Context(), "gateway billing refund: "+err.Error())
		}
	}
	if runner.lastAPIError != nil {
		return runner.lastAPIError
	}
	if selector.lastError != nil {
		if IsModelNotFound(selector.lastError) {
			return common.ModelNotFoundError(relay.getOriginalModel())
		}
		return common.UpstreamUnavailableError(selector.lastError.Error())
	}
	return common.UpstreamUnavailableError(result.Err.Error())
}

type relayGatewaySelector struct {
	relay     providerSelectable
	lastError error
}

type providerSelectable interface {
	setProvider(modelName string) error
	getOriginalModel() string
	getProvider() providersBase.ProviderRuntime
}

func (s *relayGatewaySelector) Next(
	_ context.Context,
	_ domain.RoutePlan,
	_ execution.SelectionState,
) (domain.Endpoint, error) {
	if err := s.relay.setProvider(s.relay.getOriginalModel()); err != nil {
		s.lastError = err
		return domain.Endpoint{}, err
	}
	provider := s.relay.getProvider()
	if provider == nil || provider.GetChannel() == nil {
		s.lastError = execution.ErrNoCandidates
		return domain.Endpoint{}, s.lastError
	}
	channel := provider.GetChannel()
	return endpointFromChannel(channel), nil
}

func endpointFromChannel(channel *model.Channel) domain.Endpoint {
	if channel == nil {
		return domain.Endpoint{}
	}
	weight := 0
	if channel.Weight != nil {
		weight = int(*channel.Weight)
	}
	priority := 0
	if channel.Priority != nil {
		priority = int(*channel.Priority)
	}
	return domain.Endpoint{
		ID:                channel.Id,
		ProviderID:        domain.ProviderID(channel.Type),
		Name:              channel.Name,
		BaseURL:           channel.GetBaseURL(),
		Group:             channel.Group,
		Weight:            weight,
		Priority:          priority,
		Enabled:           channel.Status == config.ChannelStatusEnabled,
		ProtocolProfileID: domain.ProtocolProfileID(channel.ProtocolProfileID),
		AffinityEnabled:   channel.AffinityEnabled,
	}
}

type relayGatewayRunner struct {
	relay          RelayBaseInterface
	gatewayBilling *relay_util.GatewayBilling
	lastAPIError   *types.OpenAIErrorWithStatusCode
}

func (r *relayGatewayRunner) Run(
	_ context.Context,
	attempt domain.Attempt,
	session *gatewaystream.Session,
) (domain.AttemptResult, *gatewayretry.UpstreamError) {
	c := r.relay.getContext()
	setGatewayStreamSession(c, session)
	apiErr, done, gatewayBilling := relayHandlerWithSession(r.relay, session, r.gatewayBilling)
	r.gatewayBilling = gatewayBilling
	result := domain.AttemptResult{Attempt: attempt}
	if apiErr == nil {
		r.lastAPIError = nil
		return result, nil
	}

	r.lastAPIError = apiErr
	channel := r.relay.getProvider().GetChannel()
	notifyChannelRelayError(c.Request.Context(), c, channel, apiErr)
	metrics.RecordProvider(c, apiErr.StatusCode)
	upstreamErr := classifyRelayError(c, apiErr, channel.Type)
	if done && session.CanFailover() {
		// "done" is a runner-level terminal decision (for example local
		// protocol validation) even when no bytes were written.
		upstreamErr.Class = gatewayretry.ErrorClassProtocol
	}
	return result, upstreamErr
}

type relayCooldownStore struct {
	c *gin.Context
}

func (s *relayCooldownStore) Cooldown(
	_ context.Context,
	endpoint domain.Endpoint,
	modelName string,
	upstreamErr *gatewayretry.UpstreamError,
	decision gatewayretry.Decision,
) error {
	duration := decision.Delay
	if upstreamErr != nil {
		if seconds, configured := config.GetRetryCooldownForStatus(upstreamErr.StatusCode); configured {
			duration = time.Duration(seconds) * time.Second
		} else if upstreamErr.StatusCode == http.StatusTooManyRequests && upstreamErr.RetryAfter <= 0 {
			duration = time.Duration(config.RetryCooldownSeconds) * time.Second
		}
	}
	if duration > 0 {
		seconds := int64((duration + time.Second - 1) / time.Second)
		model.GatewayRoutes.SetCooldownsWithDuration(endpoint.ID, modelName, seconds)
	}
	gatewayRequestState(s.c).Skip(endpoint.ID)
	return nil
}

type relayAttemptObserver struct {
	c *gin.Context
}

func (o *relayAttemptObserver) AttemptFinished(
	result domain.AttemptResult,
	upstreamErr *gatewayretry.UpstreamError,
	decision gatewayretry.Decision,
) {
	state := gatewayRequestState(o.c)
	affinitySource := state.GetString("gateway_affinity_source")
	affinityFingerprint := state.GetString("gateway_affinity_key_fingerprint")
	if upstreamErr == nil {
		logger.LogInfo(o.c.Request.Context(), fmt.Sprintf(
			"gateway_attempt_finished request_id=%s attempt=%d endpoint_id=%d outcome=success duration_ms=%d input_tokens=%d output_tokens=%d output_started=%t accepted=%t affinity_source=%s affinity_key_fp=%s",
			result.Attempt.RequestID, result.Attempt.Number, result.Attempt.Endpoint.ID,
			result.CompletedAt.Sub(result.Attempt.StartedAt).Milliseconds(),
			result.Usage.InputTokens, result.Usage.OutputTokens,
			result.OutputStarted, result.UpstreamAccepted, affinitySource, affinityFingerprint,
		))
		return
	}
	logger.LogWarn(o.c.Request.Context(), fmt.Sprintf(
		"gateway_attempt_finished request_id=%s attempt=%d endpoint_id=%d outcome=failed status_code=%d class=%s retry=%t cooldown=%t delay=%s duration_ms=%d input_tokens=%d output_tokens=%d output_started=%t accepted=%t affinity_source=%s affinity_key_fp=%s",
		result.Attempt.RequestID, result.Attempt.Number, result.Attempt.Endpoint.ID,
		upstreamErr.StatusCode, upstreamErr.Class, decision.Retry, decision.Cooldown,
		decision.Delay, result.CompletedAt.Sub(result.Attempt.StartedAt).Milliseconds(),
		result.Usage.InputTokens, result.Usage.OutputTokens,
		result.OutputStarted, result.UpstreamAccepted, affinitySource, affinityFingerprint,
	))
}

func capabilityForPath(path string) domain.Capability {
	switch {
	case path == "/v1/responses", path == "/v1/responses/compact":
		return domain.CapabilityResponses
	case path == "/v1/embeddings":
		return domain.CapabilityEmbeddings
	case path == "/v1/rerank":
		return domain.CapabilityRerank
	case path == "/v1/moderations":
		return domain.CapabilityModerations
	case path == "/v1/images/generations":
		return domain.CapabilityImageGenerations
	case path == "/v1/images/edits":
		return domain.CapabilityImageEdits
	case path == "/v1/images/variations":
		return domain.CapabilityImageVariations
	case path == "/v1/audio/speech":
		return domain.CapabilitySpeech
	case path == "/v1/audio/transcriptions":
		return domain.CapabilityTranscriptions
	case path == "/v1/audio/translations":
		return domain.CapabilityTranslations
	case path == "/v1/completions":
		return domain.CapabilityCompletions
	default:
		return domain.CapabilityChat
	}
}
