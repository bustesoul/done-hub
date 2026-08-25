package relay

import (
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/utils"
	"done-hub/internal/gateway/billing"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/requeststate"
	gatewaystream "done-hub/internal/gateway/stream"
	"done-hub/model"
	"done-hub/relay/relay_util"
	"done-hub/types"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func Relay(c *gin.Context) {
	// 在请求完成后清理缓存的请求体，防止内存泄漏
	defer func() {
		c.Set(config.GinRequestBodyKey, nil)
		state := gatewayRequestState(c)
		state.Set(config.GinProcessedBodyKey, nil)
		state.Set(config.GinProcessedBodyIsVertexAI, nil)
		state.Set(config.GinRawMapBodyKey, nil)
		state.Set(config.GinProcessedBytesKey, nil)
		state.Set(config.GinProcessedBytesIsVertexAI, nil)
	}()
	gatewayRequestState(c)

	relay := Path2Relay(c, c.Request.URL.Path)
	if relay == nil {
		common.AbortWithMessage(c, http.StatusNotFound, "Not Found")
		return
	}

	// Snapshot the client-provided reasoning effort before any mapping or
	// provider conversion can normalize, move, or remove the original field.
	captureOriginalReasoningEffort(c)

	// Apply pre-mapping before setRequest to ensure request body modifications take effect
	applyPreMappingBeforeRequest(c)

	if err := relay.setRequest(); err != nil {
		openaiErr := common.StringErrorWrapperLocal(err.Error(), "one_hub_error", http.StatusBadRequest)
		relay.HandleJsonError(openaiErr)
		return
	}

	c.Set("is_stream", relay.IsStream())
	if serviceTier := relay_util.NormalizeServiceTier(relay.GetServiceTier()); serviceTier != "" {
		c.Set("service_tier", serviceTier)
	}
	heartbeat := relay.SetHeartbeat(relay.IsStream())
	if heartbeat != nil {
		defer heartbeat.Close()
	}

	apiErr := executeRelayGateway(relay)
	if apiErr == nil {
		return
	}

	state := gatewayRequestState(c)
	selection := state.Selection()
	modelName := selection.UpstreamModel
	if modelName == "" {
		modelName = relay.getOriginalModel()
	}
	startTime := c.GetTime("requestStartTime")
	finalAttempt := state.AttemptCount()
	channelID := selection.ChannelID
	errorSource := "upstream"
	if apiErr.LocalError {
		errorSource = "local"
	}
	errorMetadata := map[string]any{
		"status_code":   apiErr.StatusCode,
		"error_type":    apiErr.OpenAIError.Type,
		"error_message": utils.TruncateBase64InMessage(apiErr.OpenAIError.Message),
		"local_error":   apiErr.LocalError,
		"error_source":  errorSource,
		"attempt_count": finalAttempt,
		"channel_type":  selection.ChannelType,
	}
	if reasoningEffort := c.GetString(config.GinReasoningEffortKey); reasoningEffort != "" {
		errorMetadata["reasoning_effort"] = reasoningEffort
		errorMetadata["reasoning_effort_source"] = c.GetString(config.GinReasoningEffortSourceKey)
	}
	model.RecordErrorLog(
		c.Request.Context(),
		c.GetInt("id"),
		channelID,
		modelName,
		c.GetString("token_name"),
		fmt.Sprintf("[%d] %s", apiErr.StatusCode, apiErr.OpenAIError.Type),
		int(time.Since(startTime).Seconds()),
		relay.IsStream(),
		errorMetadata,
		c.ClientIP(),
	)
	if heartbeat != nil && heartbeat.IsSafeWriteStream() {
		relay.HandleStreamError(apiErr)
		return
	}
	relay.HandleJsonError(apiErr)
}

// promptTokenForcer 由能在忽略 PreCost 开关的前提下强制计算输入 token 的 relay 实现。
// 当渠道关闭预扣费（getPromptTokens 返回 0）时，超限守卫用它兜底，避免被绕过。
type promptTokenForcer interface {
	forcePromptTokens() int
}

// checkPromptTokenLimit 在发送上游前拦截超过模型上下文上限的请求（仅 AWS/Bedrock 渠道）。
// 有效上限：优先 model_info.ContextLength(>0)，否则回落全局 config.MaxPromptTokens；<=0 视为不限。
func checkPromptTokenLimit(relay RelayBaseInterface, promptTokens int) *types.OpenAIErrorWithStatusCode {
	provider := relay.getProvider()
	if provider == nil {
		return nil
	}
	ch := provider.GetChannel()
	if ch == nil || (ch.Type != config.ChannelTypeBedrock && ch.Type != config.ChannelTypeBedrockMessages) {
		return nil
	}

	limit := config.MaxPromptTokens
	// GetPrice 未命中也返回默认 Price（ModelInfo 为 nil），不会返回 nil，故只需判 ModelInfo。
	if price := model.PricingInstance.GetPrice(relay.getModelName()); price.ModelInfo != nil && price.ModelInfo.ContextLength > 0 {
		limit = price.ModelInfo.ContextLength
	}
	if limit <= 0 {
		return nil
	}

	tokens := promptTokens
	if tokens <= 0 {
		// 渠道关闭预扣费时 promptTokens 为 0，强制计一次数
		if forcer, ok := relay.(promptTokenForcer); ok {
			tokens = forcer.forcePromptTokens()
		}
	}

	if tokens > limit {
		// 文案对齐 Anthropic 官方超上下文的 400：`prompt is too long: N tokens > M maximum`，
		// 且 Type=invalid_request_error。这样经 done-hub 的 Claude Code / Anthropic SDK 客户端
		// 能像识别官方 400 一样识别它（触发 /compact 或提示压缩），而非当成陌生的自定义错误。
		msg := fmt.Sprintf("prompt is too long: %d tokens > %d maximum", tokens, limit)
		return &types.OpenAIErrorWithStatusCode{
			OpenAIError: types.OpenAIError{
				Message: msg,
				Type:    "invalid_request_error",
				Code:    "prompt_tokens_exceed_limit",
			},
			StatusCode: http.StatusBadRequest,
			LocalError: true,
		}
	}
	return nil
}

func RelayHandler(relay RelayBaseInterface) (err *types.OpenAIErrorWithStatusCode, done bool) {
	err, done, _ = relayHandlerWithSession(relay, beginGatewayAttempt(relay.getContext()), nil)
	return err, done
}

func relayHandlerWithSession(
	relay RelayBaseInterface,
	streamSession *gatewaystream.Session,
	billingSession *relay_util.GatewayBilling,
) (err *types.OpenAIErrorWithStatusCode, done bool, session *relay_util.GatewayBilling) {
	session = billingSession
	setGatewayStreamSession(relay.getContext(), streamSession)
	promptTokens, tonkeErr := relay.getPromptTokens()
	if tonkeErr != nil {
		err = common.ErrorWrapperLocal(tonkeErr, "token_error", http.StatusBadRequest)
		done = true
		return
	}

	// 超大请求预拦截（仅 AWS/Bedrock）：在预扣费与发送上游之前拦下，避免超限请求
	// 在上游静默挂起直到墙钟超时。拒绝时不预扣费、不触发 undo。
	if err = checkPromptTokenLimit(relay, promptTokens); err != nil {
		done = true
		return
	}

	usage := &types.Usage{
		PromptTokens: promptTokens,
	}

	relay.getProvider().SetUsage(usage)

	if billingSession == nil {
		billingSession = relay_util.NewGatewayBilling(
			relay.getContext(),
			relay.getModelName(),
			promptTokens,
			usage,
			relay.IsStream(),
			relay.GetFirstResponseTime,
		)
		session = billingSession
		if billingErr := billingSession.Precharge(relay.getContext().Request.Context(), billing.Estimate{
			Model:        relay.getModelName(),
			PromptTokens: promptTokens,
			ServiceTier:  relay.GetServiceTier(),
		}); billingErr != nil {
			if !errors.As(billingErr, &err) {
				err = common.ErrorWrapperLocal(billingErr, "pre_consume_token_quota_failed", http.StatusForbidden)
			}
			done = true
			return
		}
	} else {
		billingSession.BeginAttempt(
			relay.getContext(),
			relay.getModelName(),
			promptTokens,
			usage,
			relay.IsStream(),
			relay.GetFirstResponseTime,
		)
	}

	err, done = relay.send()
	// 最后处理流式中断时计算tokens
	if usage.CompletionTokens == 0 && usage.TextBuilder.Len() > 0 {
		usage.CompletionTokens = common.CountTokenText(usage.TextBuilder.String(), relay.getModelName())
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}

	if !streamSession.CanFailover() {
		done = true
	}

	// A provider-reported usage record proves that the upstream accepted the
	// attempt even if no client bytes were emitted (for example a prompt-only
	// failure). Such an attempt is terminal: retrying would duplicate work and
	// BeginAttempt would otherwise replace its Usage pointer.
	if err != nil && relay_util.HasReportedUsage(usage) {
		_ = streamSession.MarkAccepted()
		done = true
	}
	_ = streamSession.AddUsage(relay_util.DomainUsage(usage))

	// 即使出错，只要有实际输出就记录计费，避免上游已计费但本地无记录。
	// CompletionTokens 来自上游返回的 usage（image_*.go 在 ErrorHandle 前也会落 usage），
	// 是"上游真的处理了请求"的可靠信号；PromptTokens 不行，它在 send 之前就被本地 tokenize 填了。
	if err != nil {
		if !streamSession.CanFailover() {
			done = true
			if billingErr := billingSession.Settle(relay.getContext().Request.Context(), domain.OutcomeFailed); billingErr != nil {
				logger.LogError(relay.getContext().Request.Context(), "gateway billing settle: "+billingErr.Error())
			}
		}
		return
	}

	if billingErr := billingSession.Settle(relay.getContext().Request.Context(), domain.OutcomeSucceeded); billingErr != nil {
		logger.LogError(relay.getContext().Request.Context(), "gateway billing settle: "+billingErr.Error())
	}

	return err, done, billingSession
}

// applies pre-mapping before setRequest to ensure modifications take effect
func applyPreMappingBeforeRequest(c *gin.Context) {
	// check if this is a chat completion request that needs pre-mapping
	path := c.Request.URL.Path
	if !(strings.HasPrefix(path, "/v1/chat/completions") || strings.HasPrefix(path, "/v1/completions")) {
		return
	}

	// 使用 ReadBodyRaw 读取并缓存请求体，避免与下游 setRequest 重复读 body
	bodyBytes, err := common.ReadBodyRaw(c)
	if err != nil {
		return
	}

	// gjson 提取 model 字段，替代 json.Unmarshal 整个 body
	modelName := gjson.GetBytes(bodyBytes, "model").String()
	if modelName == "" {
		return
	}

	// 保存原始的 context 值，避免被 GetProvider 修改
	originalTokenGroup := c.GetString("token_group")
	originalBackupGroup := c.GetString("token_backup_group")
	originalGroup := c.GetString("group")
	originalGroupRatio := c.GetFloat64("group_ratio")

	// 确保恢复原始值，防止 GetProvider 内部修改导致状态污染
	defer func() {
		c.Set("token_group", originalTokenGroup)
		c.Set("token_backup_group", originalBackupGroup)
		c.Set("group", originalGroup)
		c.Set("group_ratio", originalGroupRatio)
		// 清除 GetProvider 设置的其他字段
		c.Set("original_token_group", nil)
		c.Set("is_backupGroup", nil)
		gatewayRequestState(c).SetSelection(requeststate.Selection{})
	}()

	provider, _, err := GetProvider(c, modelName)
	if err != nil {
		return
	}

	customParams, err := provider.CustomParameterHandler()
	if err != nil || customParams == nil {
		return
	}

	preAdd, exists := customParams["pre_add"]
	if !exists || preAdd != true {
		return
	}

	var requestMap map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &requestMap); err != nil {
		return
	}

	// Apply custom parameter merging
	modifiedRequestMap := mergeCustomParamsForPreMapping(requestMap, customParams)

	// Convert back to JSON - if successful, update cache
	if modifiedBodyBytes, err := json.Marshal(modifiedRequestMap); err == nil {
		c.Set(config.GinRequestBodyKey, modifiedBodyBytes)
	}
}
