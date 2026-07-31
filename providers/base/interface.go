package base

import (
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/types"
	"net/http"

	"github.com/gorilla/websocket"
)

type Requestable interface {
	types.CompletionRequest | types.ChatCompletionRequest | types.EmbeddingRequest | types.ModerationRequest | types.SpeechAudioRequest | types.AudioRequest | types.ImageRequest | types.ImageEditRequest
}

// ProviderRuntime is the composition root for state shared by capability
// executors. Capabilities below deliberately do not embed this interface:
// callers obtain runtime services and an optional capability independently.
type ProviderRuntime interface {
	ProviderContext
	ProviderUsage
	ProviderModel
	ProviderTransport
	ProviderMetadata
}

type ProviderContext interface {
	SetContext(c *RequestContext)
	GetContext() *RequestContext
}

type ProviderUsage interface {
	GetUsage() *types.Usage
	SetUsage(usage *types.Usage)
}

type ProviderModel interface {
	SetOriginalModel(modelName string)
	GetOriginalModel() string
	GetResponseModelName(requestModel string) string
	ModelMappingHandler(modelName string) (string, error)
}

type ProviderTransport interface {
	GetRequestHeaders() map[string]string
	GetRequester() *requester.HTTPRequester
}

type ProviderMetadata interface {
	GetChannel() *model.Channel
	SetOtherArg(otherArg string)
	GetOtherArg() string
	CustomParameterHandler() (map[string]interface{}, error)
	GetSupportedResponse() bool
}

// 完成接口
type CompletionInterface interface {
	CreateCompletion(request *types.CompletionRequest) (*types.CompletionResponse, *types.OpenAIErrorWithStatusCode)
	CreateCompletionStream(request *types.CompletionRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode)
}

// 聊天接口
type ChatInterface interface {
	CreateChatCompletion(request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, *types.OpenAIErrorWithStatusCode)
	CreateChatCompletionStream(request *types.ChatCompletionRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode)
}

// 嵌入接口
type EmbeddingsInterface interface {
	CreateEmbeddings(request *types.EmbeddingRequest) (*types.EmbeddingResponse, *types.OpenAIErrorWithStatusCode)
}

// 审查接口
type ModerationInterface interface {
	CreateModeration(request *types.ModerationRequest) (*types.ModerationResponse, *types.OpenAIErrorWithStatusCode)
}

// 文字转语音接口
type SpeechInterface interface {
	CreateSpeech(request *types.SpeechAudioRequest) (*http.Response, *types.OpenAIErrorWithStatusCode)
}

// 语音转文字接口
type TranscriptionsInterface interface {
	CreateTranscriptions(request *types.AudioRequest) (*types.AudioResponseWrapper, *types.OpenAIErrorWithStatusCode)
}

// 语音翻译接口
type TranslationInterface interface {
	CreateTranslation(request *types.AudioRequest) (*types.AudioResponseWrapper, *types.OpenAIErrorWithStatusCode)
}

// 图片生成接口
type ImageGenerationsInterface interface {
	CreateImageGenerations(request *types.ImageRequest) (*types.ImageResponse, *types.OpenAIErrorWithStatusCode)
}

// 图片编辑接口
type ImageEditsInterface interface {
	CreateImageEdits(request *types.ImageEditRequest) (*types.ImageResponse, *types.OpenAIErrorWithStatusCode)
}

type ImageVariationsInterface interface {
	CreateImageVariations(request *types.ImageEditRequest) (*types.ImageResponse, *types.OpenAIErrorWithStatusCode)
}

// type RelayInterface interface {
// 	ProviderRuntime
// 	CreateRelay() (*http.Response, *types.OpenAIErrorWithStatusCode)
// }

type ModelListInterface interface {
	GetModelList() ([]string, error)
}

// 余额接口
type BalanceInterface interface {
	Balance() (float64, error)
}

// type ProviderResponseHandler interface {
// 	// 响应处理函数
// 	ResponseHandler(resp *http.Response) (OpenAIResponse any, errWithCode *types.OpenAIErrorWithStatusCode)
// }

// Rerank接口
type RerankInterface interface {
	CreateRerank(request *types.RerankRequest) (*types.RerankResponse, *types.OpenAIErrorWithStatusCode)
}

type RealtimeInterface interface {
	CreateChatRealtime(modelName string) (*websocket.Conn, requester.MessageHandler, *types.OpenAIErrorWithStatusCode)
}

type ResponsesInterface interface {
	CreateResponses(request *types.OpenAIResponsesRequest) (*types.OpenAIResponsesResponses, *types.OpenAIErrorWithStatusCode)
	CreateResponsesStream(request *types.OpenAIResponsesRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode)
}

// ResponsesCompactInterface /v1/responses/compact 端点的能力。
// compact 永远是非流式响应，因此不需要 stream 版本。
type ResponsesCompactInterface interface {
	CreateResponsesCompaction(request *types.OpenAIResponsesRequest) (*types.OpenAIResponsesResponses, *types.OpenAIErrorWithStatusCode)
}
