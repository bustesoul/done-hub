package relay

import (
	"done-hub/common"
	"done-hub/common/config"
	providersBase "done-hub/providers/base"
	"done-hub/types"
	"net/http"

	"github.com/gin-gonic/gin"
)

func RelayRerank(c *gin.Context) {
	// 在请求完成后清理缓存的请求体，防止内存泄漏
	defer func() {
		c.Set(config.GinRequestBodyKey, nil)
		gatewayRequestState(c).Set(config.GinProcessedBodyKey, nil)
		gatewayRequestState(c).Set(config.GinProcessedBodyIsVertexAI, nil)
	}()

	relay := NewRelayRerank(c)

	if err := relay.setRequest(); err != nil {
		common.AbortWithErr(c, http.StatusBadRequest, &types.RerankError{Detail: err.Error()})
		return
	}

	apiErr := executeRelayGateway(relay)
	if apiErr == nil {
		return
	}
	// rerank 走自有响应格式（detail 字段），因此保留其最终错误外壳。
	if apiErr.StatusCode == http.StatusTooManyRequests && config.ChannelFailErrorWrapEnabled {
		apiErr.OpenAIError.Message = config.GetChannelFailErrorMessage()
	}
	relayRerankResponseWithErr(c, apiErr)
}

type relayRerank struct {
	relayBase
	request types.RerankRequest
}

func NewRelayRerank(c *gin.Context) *relayRerank {
	relay := &relayRerank{}
	relay.c = c
	return relay
}

func (r *relayRerank) setRequest() error {
	if err := common.UnmarshalBodyReusable(r.c, &r.request); err != nil {
		return err
	}

	r.setOriginalModel(r.request.Model)

	return nil
}

func (r *relayRerank) getPromptTokens() (int, error) {
	channel := r.provider.GetChannel()
	return common.CountTokenRerankMessages(r.request, r.modelName, channel.PreCost), nil
}

func (r *relayRerank) send() (err *types.OpenAIErrorWithStatusCode, done bool) {
	chatProvider, ok := r.provider.(providersBase.RerankInterface)
	if !ok {
		err = common.StringErrorWrapperLocal("channel not implemented", "channel_error", http.StatusServiceUnavailable)
		done = true
		return
	}

	r.request.Model = r.modelName

	var response *types.RerankResponse
	response, err = chatProvider.CreateRerank(&r.request)
	if err != nil {
		return
	}
	err = responseJsonClient(r.c, response)

	if err != nil {
		done = true
	}

	return
}
