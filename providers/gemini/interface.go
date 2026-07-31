package gemini

import (
	"done-hub/common/requester"
	"done-hub/types"
)

type GeminiChatInterface interface {
	CreateGeminiChat(request *GeminiChatRequest) (*GeminiChatResponse, *types.OpenAIErrorWithStatusCode)
	CreateGeminiChatStream(request *GeminiChatRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode)
}

type GeminiVeoInterface interface {
	CreateVeoVideoAndDownload(request *VeoVideoRequest, modelName string) ([]byte, string, *types.OpenAIErrorWithStatusCode)
}
