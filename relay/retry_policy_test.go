package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"done-hub/common/config"
	commonlogger "done-hub/common/logger"
	"done-hub/internal/gateway/domain"
	gatewayretry "done-hub/internal/gateway/retry"
	"done-hub/model"
	"done-hub/types"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestRelayRetryDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name        string
		channelType int
		err         *types.OpenAIErrorWithStatusCode
		setup       func(*gin.Context)
		retry       bool
	}{
		{
			name:        "rate limited retries",
			channelType: config.ChannelTypeOpenAI,
			err:         relayTestError(http.StatusTooManyRequests, "rate limited", false),
			retry:       true,
		},
		{
			name:        "server error retries",
			channelType: config.ChannelTypeOpenAI,
			err:         relayTestError(http.StatusBadGateway, "bad gateway", false),
			retry:       true,
		},
		{
			name:        "unauthorized retries another endpoint and cools invalid credential",
			channelType: config.ChannelTypeOpenAI,
			err:         relayTestError(http.StatusUnauthorized, "invalid key", false),
			retry:       true,
		},
		{
			name:        "local error stops",
			channelType: config.ChannelTypeOpenAI,
			err:         relayTestError(http.StatusBadRequest, "invalid request", true),
		},
		{
			name:        "model not found stops",
			channelType: config.ChannelTypeOpenAI,
			err:         relayTestError(http.StatusNotFound, "model not found", false),
		},
		{
			name:        "specific channel stops",
			channelType: config.ChannelTypeOpenAI,
			err:         relayTestError(http.StatusBadGateway, "bad gateway", false),
			setup: func(context *gin.Context) {
				context.Set("specific_channel_id", 9)
			},
		},
		{
			name:        "Gemini invalid key retries",
			channelType: config.ChannelTypeGemini,
			err: &types.OpenAIErrorWithStatusCode{
				OpenAIError: types.OpenAIError{
					Message: "API key not valid. Please pass a valid API key.",
					Param:   "INVALID_ARGUMENT",
				},
				StatusCode: http.StatusBadRequest,
			},
			retry: true,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			if test.setup != nil {
				test.setup(context)
			}
			upstreamErr := classifyRelayError(context, test.err, test.channelType)
			decision := gatewayretry.DefaultPolicy().Decide(upstreamErr, gatewayretry.DecisionContext{
				Attempt:     1,
				MaxAttempts: 2,
			})
			if decision.Retry != test.retry {
				t.Fatalf("expected retry=%t, got %#v", test.retry, decision)
			}
			if test.err.StatusCode == http.StatusUnauthorized && (!decision.Cooldown || decision.Delay <= 0) {
				t.Fatalf("unauthorized decision must cool the failed endpoint: %#v", decision)
			}
		})
	}
}

func TestRetryPolicyCooldownIsAppliedByRelayLoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldLogger := commonlogger.Logger
	commonlogger.Logger = zap.NewNop()
	defer func() { commonlogger.Logger = oldLogger }()
	config.SetRetryCooldownPerStatusMap(map[int]int{})
	defer config.SetRetryCooldownPerStatusMap(map[int]int{})

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	context.Set("new_model", "cooldown-test-model")
	channel := &model.Channel{Id: 987654, Type: config.ChannelTypeOpenAI}
	apiErr := relayTestError(http.StatusUnauthorized, "invalid key", false)

	upstreamErr := classifyRelayError(context, apiErr, channel.Type)
	decision := gatewayretry.DefaultPolicy().Decide(upstreamErr, gatewayretry.DecisionContext{
		Attempt:     1,
		MaxAttempts: 2,
	})
	store := &relayCooldownStore{c: context}
	if err := store.Cooldown(context.Request.Context(), domain.Endpoint{ID: channel.Id}, "cooldown-test-model", upstreamErr, decision); err != nil {
		t.Fatalf("apply cooldown: %v", err)
	}
	if !model.GatewayRoutes.IsInCooldown(channel.Id, "cooldown-test-model") {
		t.Fatal("failed endpoint was not placed in cooldown")
	}
	model.GatewayRoutes.ClearChannelCooldowns(channel.Id)
}

func relayTestError(statusCode int, message string, local bool) *types.OpenAIErrorWithStatusCode {
	return &types.OpenAIErrorWithStatusCode{
		OpenAIError: types.OpenAIError{
			Message: message,
		},
		StatusCode: statusCode,
		LocalError: local,
	}
}
