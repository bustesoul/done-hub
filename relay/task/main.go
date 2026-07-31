package task

import (
	"context"
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/billing"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/execution"
	"done-hub/internal/gateway/requeststate"
	gatewayretry "done-hub/internal/gateway/retry"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/metrics"
	"done-hub/model"
	"done-hub/relay/relay_util"
	"done-hub/relay/task/base"
	"done-hub/types"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// buildTaskChannelFilters 为任务构建渠道过滤器列表
func buildTaskChannelFilters(c *gin.Context) []model.ChannelsFilterFunc {
	var filters []model.ChannelsFilterFunc

	if skipChannelIds := taskRequestState(c).SkippedEndpointIDs(); len(skipChannelIds) > 0 {
		filters = append(filters, model.FilterChannelId(skipChannelIds))
	}

	if types, exists := c.Get("allow_channel_type"); exists {
		if allowTypes, ok := types.([]int); ok {
			filters = append(filters, model.FilterChannelTypes(allowTypes))
		}
	}

	return filters
}

func taskRequestState(c *gin.Context) *requeststate.State {
	request, state := requeststate.Ensure(c.Request)
	c.Request = request
	return state
}

func RelayTaskSubmit(c *gin.Context) {
	taskRequestState(c)
	taskAdaptor, err := GetTaskAdaptor(GetRelayMode(c), c)
	if err != nil {
		taskErr := base.StringTaskError(http.StatusBadRequest, "adaptor_not_found", "adaptor not found", true)
		c.JSON(http.StatusBadRequest, taskErr)
		return
	}

	if taskErr := taskAdaptor.Init(); taskErr != nil {
		taskAdaptor.HandleError(taskErr)
		return
	}

	plan := domain.RoutePlan{
		Request: domain.RequestContext{
			RequestID:      c.GetString(logger.RequestIdKey),
			UserID:         c.GetInt("id"),
			TokenID:        c.GetInt("token_id"),
			Group:          c.GetString("token_group"),
			RequestedModel: taskAdaptor.GetModelName(),
			Capability:     domain.CapabilityTask,
			Protocol:       domain.ProtocolNative,
			StartedAt:      c.GetTime("requestStartTime"),
		},
		Model: domain.ModelRoute{
			PublicModel: taskAdaptor.GetModelName(),
			Capability:  domain.CapabilityTask,
			Protocol:    domain.ProtocolNative,
		},
	}
	selector := &taskGatewaySelector{context: c, adaptor: taskAdaptor}
	runner := &taskGatewayRunner{context: c, adaptor: taskAdaptor}
	coordinator := execution.NewGatewayEngine(gatewayretry.DefaultPolicy())
	coordinator.Selector = selector
	coordinator.Cooldowns = &taskCooldownStore{context: c}
	coordinator.Observer = &taskAttemptObserver{context: c}
	coordinator.MaxAttempts = config.RetryTimes + 1
	if coordinator.MaxAttempts < 1 {
		coordinator.MaxAttempts = 1
	}
	if config.RetryTimeOut > 0 {
		coordinator.Deadline = time.Duration(config.RetryTimeOut) * time.Second
	}

	result := coordinator.Run(c.Request.Context(), plan, runner)
	taskRequestState(c).SetAttemptCount(len(result.Attempts))
	if result.Err == nil {
		CompletedTask(runner.gatewayBilling, taskAdaptor)
		taskAdaptor.GinResponse()
		metrics.RecordProvider(c, http.StatusOK)
		return
	}
	if runner.gatewayBilling != nil {
		if billingErr := runner.gatewayBilling.RefundIfReserved(c.Request.Context(), "task_attempts_failed"); billingErr != nil {
			logger.LogError(c.Request.Context(), "gateway billing refund: "+billingErr.Error())
		}
	}
	if runner.lastError != nil {
		taskAdaptor.HandleError(runner.lastError)
		return
	}
	if selector.lastError != nil {
		taskAdaptor.HandleError(selector.lastError)
		return
	}
	taskAdaptor.HandleError(base.StringTaskError(http.StatusServiceUnavailable, "gateway_unavailable", result.Err.Error(), false))
}

type taskGatewaySelector struct {
	context   *gin.Context
	adaptor   base.TaskInterface
	lastError *base.TaskError
}

func (s *taskGatewaySelector) Next(
	_ context.Context,
	_ domain.RoutePlan,
	_ execution.SelectionState,
) (domain.Endpoint, error) {
	if taskErr := s.adaptor.SetProvider(); taskErr != nil {
		s.lastError = taskErr
		return domain.Endpoint{}, errors.New(taskErr.Message)
	}
	provider := s.adaptor.GetProvider()
	if provider == nil || provider.GetChannel() == nil {
		s.lastError = base.StringTaskError(http.StatusServiceUnavailable, "gateway_unavailable", "provider is unavailable", false)
		return domain.Endpoint{}, errors.New(s.lastError.Message)
	}
	channel := provider.GetChannel()
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
	}, nil
}

type taskGatewayRunner struct {
	context        *gin.Context
	adaptor        base.TaskInterface
	gatewayBilling *relay_util.GatewayBilling
	billedUsage    *types.Usage
	lastError      *base.TaskError
}

func (r *taskGatewayRunner) Run(
	_ context.Context,
	attempt domain.Attempt,
	session *gatewaystream.Session,
) (domain.AttemptResult, *gatewayretry.UpstreamError) {
	if r.gatewayBilling == nil {
		r.billedUsage = &types.Usage{PromptTokens: 1, TotalTokens: 1}
		r.gatewayBilling = relay_util.NewGatewayBilling(
			r.context,
			r.adaptor.GetModelName(),
			1000,
			r.billedUsage,
			false,
			nil,
		)
		if billingErr := r.gatewayBilling.Precharge(r.context.Request.Context(), billing.Estimate{
			Model:        r.adaptor.GetModelName(),
			PromptTokens: 1000,
		}); billingErr != nil {
			errWithOA, ok := billingErr.(*types.OpenAIErrorWithStatusCode)
			if !ok {
				errWithOA = common.ErrorWrapperLocal(billingErr, "pre_consume_token_quota_failed", http.StatusForbidden)
			}
			r.lastError = base.OpenAIErrToTaskErr(errWithOA)
			return domain.AttemptResult{Attempt: attempt}, &gatewayretry.UpstreamError{
				Class:       gatewayretry.ErrorClassLocalValidation,
				StatusCode:  errWithOA.StatusCode,
				Local:       true,
				Description: errWithOA.OpenAIError.Message,
			}
		}
	}

	taskErr := r.adaptor.Relay()
	if taskErr == nil {
		_ = session.MarkAccepted()
		r.lastError = nil
		return domain.AttemptResult{
			Attempt:          attempt,
			UpstreamAccepted: true,
		}, nil
	}
	r.lastError = taskErr
	upstreamErr := classifyTaskError(taskErr)
	if !r.adaptor.ShouldRetry(r.context, taskErr) ||
		(r.context.GetInt("specific_channel_id") > 0 && !r.context.GetBool("specific_channel_id_ignore")) {
		upstreamErr.Class = gatewayretry.ErrorClassProtocol
	}
	return domain.AttemptResult{Attempt: attempt}, upstreamErr
}

func classifyTaskError(taskErr *base.TaskError) *gatewayretry.UpstreamError {
	upstreamErr := gatewayretry.ClassifyHTTP(taskErr.StatusCode, errors.New(taskErr.Message))
	if taskErr.LocalError {
		upstreamErr.Class = gatewayretry.ErrorClassLocalValidation
		upstreamErr.Local = true
	}
	if taskErr.StatusCode == http.StatusTemporaryRedirect {
		upstreamErr.Class = gatewayretry.ErrorClassTransient
	}
	return upstreamErr
}

type taskCooldownStore struct {
	context *gin.Context
}

func (s *taskCooldownStore) Cooldown(
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
	taskRequestState(s.context).Skip(endpoint.ID)
	return nil
}

type taskAttemptObserver struct {
	context *gin.Context
}

func (o *taskAttemptObserver) AttemptFinished(
	result domain.AttemptResult,
	upstreamErr *gatewayretry.UpstreamError,
	decision gatewayretry.Decision,
) {
	if upstreamErr == nil {
		logger.LogInfo(o.context.Request.Context(), fmt.Sprintf(
			"gateway_task_attempt_finished request_id=%s attempt=%d endpoint_id=%d outcome=success duration_ms=%d accepted=%t",
			result.Attempt.RequestID, result.Attempt.Number, result.Attempt.Endpoint.ID,
			result.CompletedAt.Sub(result.Attempt.StartedAt).Milliseconds(), result.UpstreamAccepted,
		))
		return
	}
	logger.LogWarn(o.context.Request.Context(), fmt.Sprintf(
		"gateway_task_attempt_finished request_id=%s attempt=%d endpoint_id=%d outcome=failed status_code=%d class=%s retry=%t cooldown=%t duration_ms=%d",
		result.Attempt.RequestID, result.Attempt.Number, result.Attempt.Endpoint.ID, upstreamErr.StatusCode,
		upstreamErr.Class, decision.Retry, decision.Cooldown,
		result.CompletedAt.Sub(result.Attempt.StartedAt).Milliseconds(),
	))
}

func CompletedTask(gatewayBilling *relay_util.GatewayBilling, taskAdaptor base.TaskInterface) {
	if err := gatewayBilling.Settle(context.Background(), domain.OutcomeSucceeded); err != nil {
		logger.SysError("gateway billing settle: " + err.Error())
	}

	task := taskAdaptor.GetTask()
	task.Quota = common.QuotaFromFloat(gatewayBilling.Quota().GetInputRatio() * 1000)

	err := task.Insert()
	if err != nil {
		logger.SysError(fmt.Sprintf("task error: %s", err.Error()))
	}

	// 激活任务
	ActivateUpdateTaskBulk()
}

func GetRelayMode(c *gin.Context) int {
	relayMode := config.RelayModeUnknown
	path := c.Request.URL.Path
	if strings.HasPrefix(path, "/suno") {
		relayMode = config.RelayModeSuno
	} else if strings.HasPrefix(path, "/kling") {
		relayMode = config.RelayModeKling
	}

	return relayMode
}
