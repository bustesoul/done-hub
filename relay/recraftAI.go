package relay

import (
	"context"
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/billing"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/execution"
	gatewayretry "done-hub/internal/gateway/retry"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/metrics"
	"done-hub/providers/recraftAI"
	"done-hub/relay/relay_util"
	"done-hub/types"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func RelayRecraftAI(c *gin.Context) {
	gatewayRequestState(c)
	modelName := Path2RecraftAIModel(c.Request.URL.Path)
	operation := &recraftGatewayOperation{
		context:    c,
		modelName:  modelName,
		requestURL: strings.Replace(c.Request.URL.Path, "/recraftAI", "", 1),
	}
	plan := domain.RoutePlan{
		Request: domain.RequestContext{
			RequestID:      c.GetString(logger.RequestIdKey),
			UserID:         c.GetInt("id"),
			TokenID:        c.GetInt("token_id"),
			Group:          c.GetString("token_group"),
			RequestedModel: modelName,
			Capability:     domain.CapabilityPassthrough,
			Protocol:       domain.ProtocolNative,
			StartedAt:      c.GetTime("requestStartTime"),
		},
		Model: domain.ModelRoute{
			PublicModel: modelName,
			Capability:  domain.CapabilityPassthrough,
			Protocol:    domain.ProtocolNative,
		},
	}
	engine := execution.NewGatewayEngine(gatewayretry.DefaultPolicy())
	engine.Selector = operation
	engine.Cooldowns = &relayCooldownStore{c: c}
	engine.Observer = &relayAttemptObserver{c: c}
	engine.MaxAttempts = config.RetryTimes + 1
	if engine.MaxAttempts < 1 {
		engine.MaxAttempts = 1
	}
	if config.RetryTimeOut > 0 {
		engine.Deadline = time.Duration(config.RetryTimeOut) * time.Second
	}
	result := engine.Run(c.Request.Context(), plan, operation)
	gatewayRequestState(c).SetAttemptCount(len(result.Attempts))
	if result.Err == nil {
		metrics.RecordProvider(c, http.StatusOK)
		return
	}
	if operation.gatewayBilling != nil {
		if err := operation.gatewayBilling.RefundIfReserved(c.Request.Context(), "recraft_attempts_failed"); err != nil {
			logger.LogError(c.Request.Context(), "gateway billing refund: "+err.Error())
		}
	}
	if operation.lastAPIError != nil {
		filtered := FilterOpenAIErr(c, operation.lastAPIError)
		relayResponseWithOpenAIErr(c, &filtered)
		return
	}
	if operation.lastSelectError != nil && IsModelNotFound(operation.lastSelectError) {
		filtered := FilterOpenAIErr(c, common.ModelNotFoundError(modelName))
		relayResponseWithOpenAIErr(c, &filtered)
		return
	}
	message := result.Err.Error()
	if operation.lastSelectError != nil {
		message = operation.lastSelectError.Error()
	}
	common.AbortWithMessage(c, http.StatusServiceUnavailable, message)
}

type recraftGatewayOperation struct {
	context         *gin.Context
	modelName       string
	requestURL      string
	provider        *recraftAI.RecraftProvider
	gatewayBilling  *relay_util.GatewayBilling
	lastAPIError    *types.OpenAIErrorWithStatusCode
	lastSelectError error
}

func (o *recraftGatewayOperation) Next(
	_ context.Context,
	_ domain.RoutePlan,
	_ execution.SelectionState,
) (domain.Endpoint, error) {
	provider, err := getRecraftProvider(o.context, o.modelName)
	if err != nil {
		o.lastSelectError = err
		return domain.Endpoint{}, err
	}
	o.provider = provider
	return endpointFromChannel(provider.GetChannel()), nil
}

func (o *recraftGatewayOperation) Run(
	_ context.Context,
	attempt domain.Attempt,
	session *gatewaystream.Session,
) (domain.AttemptResult, *gatewayretry.UpstreamError) {
	setGatewayStreamSession(o.context, session)
	usage := &types.Usage{PromptTokens: 1}
	o.provider.SetUsage(usage)
	if o.gatewayBilling == nil {
		o.gatewayBilling = relay_util.NewGatewayBilling(o.context, o.modelName, 1, usage, false, nil)
		if err := o.gatewayBilling.Precharge(o.context.Request.Context(), billing.Estimate{
			Model:        o.modelName,
			PromptTokens: 1,
		}); err != nil {
			apiErr, ok := err.(*types.OpenAIErrorWithStatusCode)
			if !ok {
				apiErr = common.ErrorWrapperLocal(err, "pre_consume_token_quota_failed", http.StatusForbidden)
			}
			o.lastAPIError = apiErr
			return domain.AttemptResult{Attempt: attempt}, classifyRelayError(o.context, apiErr, int(attempt.Endpoint.ProviderID))
		}
	} else {
		o.gatewayBilling.BeginAttempt(o.context, o.modelName, 1, usage, false, nil)
	}

	response, apiErr := o.provider.CreateRelay(o.requestURL)
	result := domain.AttemptResult{Attempt: attempt}
	if apiErr != nil {
		o.lastAPIError = apiErr
		notifyChannelRelayError(o.context.Request.Context(), o.context, o.provider.GetChannel(), apiErr)
		metrics.RecordProvider(o.context, apiErr.StatusCode)
		return result, classifyRelayError(o.context, apiErr, int(attempt.Endpoint.ProviderID))
	}
	if writeErr := responseMultipart(o.context, response); writeErr != nil {
		o.lastAPIError = writeErr
		return result, classifyRelayError(o.context, writeErr, int(attempt.Endpoint.ProviderID))
	}
	_ = session.AddUsage(relay_util.DomainUsage(usage))
	if err := o.gatewayBilling.Settle(o.context.Request.Context(), domain.OutcomeSucceeded); err != nil {
		logger.LogError(o.context.Request.Context(), "gateway billing settle: "+err.Error())
	}
	o.lastAPIError = nil
	return result, nil
}

func Path2RecraftAIModel(path string) string {
	parts := strings.Split(path, "/")
	lastPart := parts[len(parts)-1]

	return "recraft_" + lastPart
}

func getRecraftProvider(c *gin.Context, model string) (*recraftAI.RecraftProvider, error) {
	provider, _, fail := GetProvider(c, model)
	if fail != nil {
		// common.AbortWithMessage(c, http.StatusServiceUnavailable, fail.Error())
		return nil, fail
	}

	recraftProvider, ok := provider.(*recraftAI.RecraftProvider)
	if !ok {
		return nil, errors.New("provider not found")
	}

	return recraftProvider, nil
}
