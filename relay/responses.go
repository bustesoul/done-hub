package relay

import (
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/internal/gateway/domain"
	providersBase "done-hub/providers/base"
	"done-hub/relay/relay_util"
	"done-hub/types"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

type relayResponses struct {
	relayBase
	responsesRequest types.OpenAIResponsesRequest
	isCompact        bool
}

func NewRelayResponses(c *gin.Context) *relayResponses {
	relay := &relayResponses{}
	relay.c = c
	return relay
}

// NewRelayResponsesCompact 处理 POST /v1/responses/compact。
// 与 NewRelayResponses 共用请求结构和路由，仅在 send() 阶段走 compact 分支。
func NewRelayResponsesCompact(c *gin.Context) *relayResponses {
	relay := &relayResponses{isCompact: true}
	relay.c = c
	return relay
}

func (r *relayResponses) setRequest() error {
	rawHasNamespace := false
	if types.ShouldDebugResponsesTools() {
		if rawBody, err := common.ReadBodyRaw(r.c); err == nil {
			rawHasNamespace = types.HasNamespaceToolsInRequestJSON(rawBody)
			logger.LogDebug(r.c.Request.Context(), "responses inbound raw "+types.SummarizeResponsesRequestToolsJSON(rawBody))
		}
	}

	if err := common.UnmarshalBodyReusable(r.c, &r.responsesRequest); err != nil {
		return err
	}

	if types.ShouldDebugResponsesTools() {
		logger.LogDebug(r.c.Request.Context(), "responses inbound parsed "+types.SummarizeResponsesTools(r.responsesRequest.Tools))
		if rawHasNamespace && len(r.responsesRequest.Tools) > 0 && types.HasNamespaceToolsWithoutNested(r.responsesRequest.Tools) {
			logger.LogError(r.c.Request.Context(), "responses inbound parse dropped namespace.tools")
		}
	}

	r.setOriginalModel(r.responsesRequest.Model)

	return nil
}

func (r *relayResponses) getRequest() interface{} {
	return &r.responsesRequest
}

func (r *relayResponses) IsStream() bool {
	// compact 端点永远是非流式响应，不受请求体中 stream 字段影响。
	if r.isCompact {
		return false
	}
	return r.responsesRequest.Stream
}

func (r *relayResponses) GetServiceTier() string {
	return r.responsesRequest.ServiceTier
}

func (r *relayResponses) getPromptTokens() (int, error) {
	channel := r.provider.GetChannel()
	return common.CountTokenInputMessages(r.responsesRequest.Input, r.modelName, channel.PreCost), nil
}

func (r *relayResponses) send() (err *types.OpenAIErrorWithStatusCode, done bool) {
	r.responsesRequest.Model = r.modelName

	if r.isCompact {
		return r.sendCompact()
	}

	channel := r.provider.GetChannel()
	responsesProvider, ok := r.provider.(providersBase.ResponsesInterface)
	if channel != nil && channel.ProtocolProfileID != "" {
		switch domain.ProtocolProfileID(channel.ProtocolProfileID) {
		case domain.ProfileOpenAIResponses:
			if !ok || !r.provider.GetSupportedResponse() {
				err = common.StringErrorWrapperLocal(
					"selected connection profile does not support OpenAI Responses",
					"channel_error", http.StatusServiceUnavailable)
				done = true
				return
			}
		case domain.ProfileOpenAIChat:
			chatProvider, chatOK := r.provider.(providersBase.ChatInterface)
			if !chatOK {
				err = common.StringErrorWrapperLocal("channel not implemented", "channel_error", http.StatusServiceUnavailable)
				done = true
				return
			}
			return r.compatibleSend(chatProvider)
		case domain.ProfileAnthropicMessages, domain.ProfileGoogleGemini:
			chatProvider, chatOK := r.provider.(providersBase.ChatInterface)
			if !chatOK {
				err = common.StringErrorWrapperLocal("channel protocol adapter not implemented", "channel_error", http.StatusServiceUnavailable)
				done = true
				return
			}
			return r.compatibleSend(chatProvider)
		default:
			err = common.StringErrorWrapperLocal(
				"selected connection profile cannot serve OpenAI Responses",
				"channel_error", http.StatusServiceUnavailable)
			done = true
			return
		}
	} else if !ok || channel.CompatibleResponse || !r.provider.GetSupportedResponse() || config.IsBuiltinNeed2ResponseModel(r.modelName) {
		// 做一层Chat的兼容
		chatProvider, ok := r.provider.(providersBase.ChatInterface)
		if !ok {
			err = common.StringErrorWrapperLocal("channel not implemented", "channel_error", http.StatusServiceUnavailable)
			done = true
			return
		}

		return r.compatibleSend(chatProvider)
	}

	// 入口协议 == responses 且走 responses provider 原样直返，放行响应字节透传；
	// 上方 chat 兼容路径（compatibleSend）已提前返回，不会走到这里。
	gatewayRequestState(r.c).Set(config.GinRawPassThroughAllowedKey, true)

	if r.responsesRequest.Stream {
		var response requester.StreamReaderInterface[string]
		response, err = responsesProvider.CreateResponsesStream(&r.responsesRequest)
		if err != nil {
			return
		}

		doneStr := func() string {
			return ""
		}

		firstResponseTime := responseGeneralStreamClient(r.c, response, doneStr)
		r.SetFirstResponseTime(firstResponseTime)
	} else {
		var response *types.OpenAIResponsesResponses
		response, err = responsesProvider.CreateResponses(&r.responsesRequest)
		if err != nil {
			return
		}
		openErr := responseJsonClient(r.c, response)

		if openErr != nil {
			err = openErr
		}
	}

	if err != nil {
		done = true
	}

	return
}

// sendCompact 处理 /v1/responses/compact 请求。
// compact 不支持 chat 兼容路径（chat 渠道没有 compact 概念），
// 不支持的渠道直接返回错误。
func (r *relayResponses) sendCompact() (err *types.OpenAIErrorWithStatusCode, done bool) {
	if channel := r.provider.GetChannel(); channel != nil && channel.ProtocolProfileID != "" &&
		domain.ProtocolProfileID(channel.ProtocolProfileID) != domain.ProfileOpenAIResponses {
		err = common.StringErrorWrapperLocal(
			"selected connection profile does not support /v1/responses/compact",
			"channel_error", http.StatusServiceUnavailable)
		done = true
		return
	}
	compactProvider, ok := r.provider.(providersBase.ResponsesCompactInterface)
	if !ok || !r.provider.GetSupportedResponse() {
		err = common.StringErrorWrapperLocal("channel does not support /v1/responses/compact", "channel_error", http.StatusServiceUnavailable)
		done = true
		return
	}

	// compact 端点：responses 协议原样直返，放行响应字节透传。
	gatewayRequestState(r.c).Set(config.GinRawPassThroughAllowedKey, true)

	response, err := compactProvider.CreateResponsesCompaction(&r.responsesRequest)
	if err != nil {
		done = true
		return
	}

	if openErr := responseJsonClient(r.c, response); openErr != nil {
		err = openErr
		done = true
	}
	return
}

func (r *relayResponses) compatibleSend(chatProvider providersBase.ChatInterface) (errWithCode *types.OpenAIErrorWithStatusCode, done bool) {
	convertedRequest, err := convertProtocolRequest(r.c, &r.responsesRequest)
	if err != nil {
		return common.ErrorWrapperLocal(err, "protocol_conversion_failed", http.StatusInternalServerError), true
	}
	chatReq, ok := convertedRequest.(*types.ChatCompletionRequest)
	if !ok {
		return common.StringErrorWrapperLocal("protocol converter returned an invalid chat request", "protocol_conversion_failed", http.StatusInternalServerError), true
	}

	if r.responsesRequest.Stream {
		var response requester.StreamReaderInterface[string]
		response, errWithCode = chatProvider.CreateChatCompletionStream(chatReq)
		if errWithCode != nil {
			return
		}
		convertedStream, conversionErr := convertProtocolStream(r.c, response)
		if conversionErr != nil {
			return common.ErrorWrapperLocal(conversionErr, "protocol_conversion_failed", http.StatusInternalServerError), true
		}
		response, ok = convertedStream.(requester.StreamReaderInterface[string])
		if !ok {
			return common.StringErrorWrapperLocal("protocol converter returned an invalid stream", "protocol_conversion_failed", http.StatusInternalServerError), true
		}
		firstResponseTime := r.chatToResponseStreamClient(response)
		r.SetFirstResponseTime(firstResponseTime)
	} else {
		var response *types.ChatCompletionResponse
		response, errWithCode = chatProvider.CreateChatCompletion(chatReq)
		if errWithCode != nil {
			return
		}

		convertedResponse, conversionErr := convertProtocolResponse(r.c, chatToResponsesResult{
			Response: response,
			Request:  &r.responsesRequest,
		})
		if conversionErr != nil {
			return common.ErrorWrapperLocal(conversionErr, "protocol_conversion_failed", http.StatusInternalServerError), true
		}
		responseResp, ok := convertedResponse.(*types.OpenAIResponsesResponses)
		if !ok {
			return common.StringErrorWrapperLocal("protocol converter returned an invalid responses response", "protocol_conversion_failed", http.StatusInternalServerError), true
		}
		responseJsonClient(r.c, responseResp)
	}

	if errWithCode != nil {
		done = true
	}

	return
}

// 将chat转换成兼容的responses流处理
func (r *relayResponses) chatToResponseStreamClient(stream requester.StreamReaderInterface[string]) (firstResponseTime time.Time) {
	requester.SetEventStreamHeaders(r.c)
	dataChan, errChan := stream.Recv()

	// 创建一个done channel用于通知处理完成
	done := make(chan struct{})

	defer func() {
		stream.Close()
	}()

	var isFirstResponse bool

	converter := relay_util.NewOpenAIResponsesStreamConverter(r.c, &r.responsesRequest, r.provider.GetUsage())

	// 在新的goroutine中处理stream数据
	gopool.Go(func() {
		defer func() {
			close(done)
		}()

		for {
			select {
			case data, ok := <-dataChan:
				if !ok {
					return
				}

				if !isFirstResponse {
					firstResponseTime = time.Now()
					isFirstResponse = true
				}

				// 尝试写入数据，如果客户端断开也继续处理
				select {
				case <-r.c.Request.Context().Done():
					// 客户端已断开，不执行任何操作，直接跳过
				default:
					// 客户端正常，发送数据
					converter.ProcessStreamData(data)
				}

			case err := <-errChan:
				if !errors.Is(err, io.EOF) {
					// 处理错误情况
					select {
					case <-r.c.Request.Context().Done():
						// 客户端已断开，不执行任何操作，直接跳过
					default:
						// 客户端正常，发送错误信息
						converter.ProcessError(err.Error())
					}
				} else {
					// 要发送最后的完成状态
					converter.ProcessStreamData("[DONE]")
				}
				return
			}
		}
	})

	// 等待处理完成
	<-done
	return firstResponseTime
}
