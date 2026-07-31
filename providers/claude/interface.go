package claude

import (
	"done-hub/common/requester"
	"done-hub/types"
)

type ClaudeChatInterface interface {
	CreateClaudeChat(request *ClaudeRequest) (*ClaudeResponse, *types.OpenAIErrorWithStatusCode)
	CreateClaudeChatStream(request *ClaudeRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode)
}
