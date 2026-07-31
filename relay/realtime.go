package relay

import (
	"context"
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/internal/gateway/billing"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/execution"
	gatewayretry "done-hub/internal/gateway/retry"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/metrics"
	providersBase "done-hub/providers/base"
	"done-hub/relay/relay_util"
	"done-hub/types"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type RelayModeChatRealtime struct {
	relayBase
	userConn       *websocket.Conn
	messageHandler requester.MessageHandler
	providerConn   *websocket.Conn
	usage          *types.UsageEvent
	billedUsage    *types.Usage
	gatewayBilling *relay_util.GatewayBilling
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Subprotocols: []string{"realtime"},
}

func ChatRealtime(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		common.AbortWithMessage(c, http.StatusBadRequest, "model_name_required")
		return
	}

	userConn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		fmt.Println("upgrade failed", err)
		common.AbortWithMessage(c, http.StatusInternalServerError, "upgrade_failed")
		return
	}

	relay := &RelayModeChatRealtime{
		relayBase: relayBase{
			c: c,
		},
		userConn: userConn,
	}
	relay.setOriginalModel(modelName)
	relay.usage = &types.UsageEvent{}
	relay.billedUsage = &types.Usage{}

	if !relay.connectProvider() {
		if relay.gatewayBilling != nil {
			if billingErr := relay.gatewayBilling.Refund(context.Background(), "realtime_provider_unavailable"); billingErr != nil {
				logger.LogError(c.Request.Context(), "gateway billing refund: "+billingErr.Error())
			}
		}
		return
	}

	wsProxy := requester.NewWSProxy(relay.userConn, relay.providerConn, time.Minute*2, relay.messageHandler, relay.usageHandler)

	wsProxy.Start()

	// 在 spawn TrackedGoroutine 之前完成 snapshot：handler 在 wsProxy.Wait() 返回后
	// 就会 return，gin 会把 *gin.Context 归还 pool 并可能被新请求 Reset。闭包持有值不持指针，
	// 彻底消除 c-pool 复用的数据竞争窗口。
	snap := relay_util.NewConsumeSnapshot(relay.c)
	common.TrackedGoroutine(func() {
		var closedBy string
		select {
		case <-wsProxy.UserClosed():
			closedBy = "user"
		case <-wsProxy.SupplierClosed():
			closedBy = "provider"
		}

		logger.LogInfo(snap.Ctx, fmt.Sprintf("连接由%s关闭", closedBy))
		wsProxy.Close()
		*relay.billedUsage = *relay.usage.ToChatUsage()
		outcome := domain.OutcomeSucceeded
		if closedBy == "user" {
			outcome = domain.OutcomeCanceled
		}
		if billingErr := relay.gatewayBilling.Settle(context.Background(), outcome); billingErr != nil {
			logger.LogError(snap.Ctx, "gateway billing settle: "+billingErr.Error())
		}
	})

	wsProxy.Wait()
}

func (r *RelayModeChatRealtime) abortWithMessage(message string) {
	eventErr := types.NewErrorEvent("", "system_error", "system_error", message)

	r.userConn.WriteMessage(websocket.TextMessage, []byte(eventErr.Error()))
	r.userConn.Close()
}

func (r *RelayModeChatRealtime) connectProvider() bool {
	plan := domain.RoutePlan{
		Request: domain.RequestContext{
			RequestID:      r.c.GetString(logger.RequestIdKey),
			UserID:         r.c.GetInt("id"),
			TokenID:        r.c.GetInt("token_id"),
			Group:          r.c.GetString("token_group"),
			RequestedModel: r.getOriginalModel(),
			Capability:     domain.CapabilityRealtime,
			Protocol:       domain.ProtocolOpenAIChat,
			Stream:         true,
			StartedAt:      r.c.GetTime("requestStartTime"),
		},
		Model: domain.ModelRoute{
			PublicModel: r.getOriginalModel(),
			Capability:  domain.CapabilityRealtime,
			Protocol:    domain.ProtocolOpenAIChat,
		},
	}
	selector := &relayGatewaySelector{relay: r}
	runner := &realtimeGatewayRunner{relay: r}
	coordinator := execution.NewGatewayEngine(gatewayretry.DefaultPolicy())
	coordinator.Selector = selector
	coordinator.Cooldowns = &relayCooldownStore{c: r.c}
	coordinator.Observer = &relayAttemptObserver{c: r.c}
	coordinator.MaxAttempts = config.RetryTimes + 1
	if coordinator.MaxAttempts < 1 {
		coordinator.MaxAttempts = 1
	}
	if config.RetryTimeOut > 0 {
		coordinator.Deadline = time.Duration(config.RetryTimeOut) * time.Second
	}
	result := coordinator.Run(r.c.Request.Context(), plan, runner)
	if result.Err == nil {
		metrics.RecordProvider(r.c, http.StatusOK)
		return true
	}
	message := result.Err.Error()
	if runner.lastAPIError != nil {
		message = runner.lastAPIError.OpenAIError.Message
	} else if selector.lastError != nil {
		message = selector.lastError.Error()
	}
	r.abortWithMessage(message)
	return false
}

type realtimeGatewayRunner struct {
	relay        *RelayModeChatRealtime
	lastAPIError *types.OpenAIErrorWithStatusCode
}

func (r *realtimeGatewayRunner) Run(
	_ context.Context,
	attempt domain.Attempt,
	session *gatewaystream.Session,
) (domain.AttemptResult, *gatewayretry.UpstreamError) {
	setGatewayStreamSession(r.relay.c, session)
	realtimeProvider, ok := r.relay.provider.(providersBase.RealtimeInterface)
	if !ok {
		apiErr := common.StringErrorWrapperLocal("channel not implemented", "channel_error", http.StatusServiceUnavailable)
		r.lastAPIError = apiErr
		return domain.AttemptResult{Attempt: attempt}, &gatewayretry.UpstreamError{
			Class:       gatewayretry.ErrorClassProtocol,
			StatusCode:  apiErr.StatusCode,
			Local:       true,
			Description: apiErr.OpenAIError.Message,
		}
	}
	if r.relay.gatewayBilling == nil {
		r.relay.gatewayBilling = relay_util.NewGatewayBilling(
			r.relay.c,
			r.relay.getModelName(),
			0,
			r.relay.billedUsage,
			false,
			nil,
		)
		if billingErr := r.relay.gatewayBilling.Precharge(r.relay.c.Request.Context(), billing.Estimate{
			Model: r.relay.getModelName(),
		}); billingErr != nil {
			r.relay.gatewayBilling = nil
			apiErr := common.ErrorWrapperLocal(billingErr, "pre_consume_token_quota_failed", http.StatusForbidden)
			r.lastAPIError = apiErr
			return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, apiErr, int(attempt.Endpoint.ProviderID))
		}
	}

	providerConn, messageHandler, apiErr := realtimeProvider.CreateChatRealtime(r.relay.modelName)
	if apiErr != nil {
		r.lastAPIError = apiErr
		return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, apiErr, int(attempt.Endpoint.ProviderID))
	}
	r.relay.messageHandler = messageHandler
	r.relay.providerConn = providerConn
	firstMessageOK, firstMessageErr := r.relay.getRealtimeFirstMessage()
	if firstMessageOK {
		r.lastAPIError = nil
		return domain.AttemptResult{Attempt: attempt}, nil
	}
	_ = providerConn.Close()
	apiErr = common.ErrorWrapper(firstMessageErr, "realtime_first_message_failed", http.StatusBadGateway)
	r.lastAPIError = apiErr
	return domain.AttemptResult{Attempt: attempt}, classifyRelayError(r.relay.c, apiErr, int(attempt.Endpoint.ProviderID))
}

func (r *RelayModeChatRealtime) getRealtimeFirstMessage() (bool, error) {
	messageType, firstMessage, err := r.providerConn.ReadMessage()
	if err != nil {
		return false, err
	}

	if messageType != websocket.TextMessage {
		return false, fmt.Errorf("unexpected realtime message type %d", messageType)
	}

	shouldContinue, _, newMessage, err := r.messageHandler(requester.SupplierMessage, messageType, firstMessage)

	if !shouldContinue || err != nil {
		if err != nil {
			return false, err
		}
		return false, errors.New("realtime message handler stopped before first output")
	}

	if newMessage != nil {
		err = r.userConn.WriteMessage(websocket.TextMessage, newMessage)
		if err == nil {
			recordGatewayOutput(r.c, len(newMessage))
		}
	} else {
		err = r.userConn.WriteMessage(websocket.TextMessage, firstMessage)
		if err == nil {
			recordGatewayOutput(r.c, len(firstMessage))
		}
	}
	if err != nil {
		terminateGatewayOutput(r.c, err)
		return false, err
	}

	return true, nil
}

func (r *RelayModeChatRealtime) usageHandler(usage *types.UsageEvent) error {
	err := r.gatewayBilling.Quota().UpdateUserRealtimeQuota(r.usage, usage)
	if err != nil {
		return types.NewErrorEvent("", "system_error", "system_error", err.Error())
	}

	return nil
}
