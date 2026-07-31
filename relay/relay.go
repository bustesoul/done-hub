package relay

import (
	"context"
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/execution"
	gatewayretry "done-hub/internal/gateway/retry"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/metrics"
	"done-hub/model"
	"done-hub/providers/azure"
	"done-hub/providers/openai"
	"done-hub/types"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func RelayOnly(c *gin.Context) {
	gatewayRequestState(c)
	rawRelay := &relayRaw{relayBase: relayBase{c: c}}
	rawRelay.setOriginalModel("")
	selector := &relayGatewaySelector{relay: rawRelay}
	runner := &rawGatewayRunner{relay: rawRelay}
	coordinator := execution.NewGatewayEngine(gatewayretry.DefaultPolicy())
	coordinator.Selector = selector
	coordinator.Observer = &relayAttemptObserver{c: c}
	// Raw bodies are not generally replayable and several endpoints are
	// non-idempotent. They still use GatewayEngine, but never transparently retry.
	coordinator.MaxAttempts = 1
	result := coordinator.Run(c.Request.Context(), domain.RoutePlan{
		Request: domain.RequestContext{
			RequestID:  c.GetString(logger.RequestIdKey),
			UserID:     c.GetInt("id"),
			TokenID:    c.GetInt("token_id"),
			Group:      c.GetString("token_group"),
			Capability: domain.CapabilityPassthrough,
			Protocol:   domain.ProtocolNative,
			StartedAt:  c.GetTime("requestStartTime"),
		},
		Model: domain.ModelRoute{Capability: domain.CapabilityPassthrough, Protocol: domain.ProtocolNative},
	}, runner)
	if result.Err != nil {
		apiErr := runner.lastAPIError
		if apiErr == nil {
			apiErr = common.UpstreamUnavailableError(result.Err.Error())
		}
		newErrWithCode := FilterOpenAIErr(c, apiErr)
		relayResponseWithOpenAIErr(c, &newErrWithCode)
		return
	}
	metrics.RecordProvider(c, http.StatusOK)

	requestTime := 0
	path := c.Request.URL.Path
	requestStartTimeValue := c.Request.Context().Value("requestStartTime")
	if requestStartTimeValue != nil {
		requestStartTime, ok := requestStartTimeValue.(time.Time)
		if ok {
			requestTime = int(time.Since(requestStartTime).Milliseconds())
		}
	}
	model.RecordConsumeLog(c.Request.Context(), c.GetInt("id"), gatewayRequestState(c).Selection().ChannelID, 0, 0, "", c.GetString("token_name"), 0, 0, "中继:"+path, requestTime, false, nil, c.ClientIP())

}

type relayRaw struct {
	relayBase
}

type rawGatewayRunner struct {
	relay        *relayRaw
	lastAPIError *types.OpenAIErrorWithStatusCode
}

func (r *rawGatewayRunner) Run(
	_ context.Context,
	attempt domain.Attempt,
	session *gatewaystream.Session,
) (domain.AttemptResult, *gatewayretry.UpstreamError) {
	setGatewayStreamSession(r.relay.c, session)
	provider := r.relay.provider
	channel := provider.GetChannel()
	if channel.Type != config.ChannelTypeOpenAI && channel.Type != config.ChannelTypeAzure {
		r.lastAPIError = common.StringErrorWrapperLocal(
			"provider must be of type azureopenai or openai",
			"channel_error",
			http.StatusServiceUnavailable,
		)
		return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, r.lastAPIError, channel.Type)
	}

	path := r.relay.c.Request.URL.Path
	var requestURL string
	if openAIProvider, ok := provider.(*openai.OpenAIProvider); ok {
		requestURL = openAIProvider.GetFullRequestURL(path, "")
	} else if azureProvider, ok := provider.(*azure.AzureProvider); ok {
		requestURL = azureProvider.GetFullRequestURL(path, "")
	} else {
		r.lastAPIError = common.StringErrorWrapperLocal("provider must be of type openai", "channel_error", http.StatusServiceUnavailable)
		return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, r.lastAPIError, channel.Type)
	}

	mapHeaders := provider.GetRequestHeaders()
	for key, values := range r.relay.c.Request.Header {
		if _, exists := mapHeaders[key]; exists {
			continue
		}
		mapHeaders[key] = strings.Join(values, ", ")
	}
	httpRequester := provider.GetRequester()
	req, err := httpRequester.NewRequest(
		r.relay.c.Request.Method,
		requestURL,
		httpRequester.WithBody(r.relay.c.Request.Body),
		httpRequester.WithHeader(mapHeaders),
	)
	if err != nil {
		r.lastAPIError = common.ErrorWrapperLocal(err, "invalid_passthrough_request", http.StatusBadRequest)
		return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, r.lastAPIError, channel.Type)
	}
	defer req.Body.Close()

	response, apiErr := httpRequester.SendRequestRaw(req)
	if apiErr != nil {
		r.lastAPIError = apiErr
		return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, apiErr, channel.Type)
	}
	if apiErr = responseMultipart(r.relay.c, response); apiErr != nil {
		r.lastAPIError = apiErr
		return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, apiErr, channel.Type)
	}
	r.lastAPIError = nil
	return domain.AttemptResult{Attempt: attempt}, nil
}
