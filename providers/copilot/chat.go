package copilot

import (
	"done-hub/common"
	"done-hub/common/requester"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CreateChatCompletion sends a non-streaming chat request to the Copilot API.
func (p *CopilotProvider) CreateChatCompletion(request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, *types.OpenAIErrorWithStatusCode) {
	initiator := copilotInitiatorFromMessages(request.Messages)
	headers, errWithCode := p.buildCopilotHeaders(initiator)
	if errWithCode != nil {
		return nil, errWithCode
	}
	headers["Accept"] = "application/json"

	baseURL := strings.TrimSuffix(p.chatBaseURL(), "/")
	fullURL := fmt.Sprintf("%s/chat/completions", baseURL)

	req, err := p.Requester.NewRequest(http.MethodPost, fullURL, p.Requester.WithBody(request), p.Requester.WithHeader(headers))
	if err != nil {
		return nil, common.ErrorWrapper(err, "new_request_failed", http.StatusInternalServerError)
	}
	defer req.Body.Close()

	response := &types.ChatCompletionResponse{}
	_, errWithCode = p.Requester.SendRequest(req, response, false)
	if errWithCode != nil {
		// On 401, invalidate the cached token so next request re-exchanges.
		if errWithCode.StatusCode == http.StatusUnauthorized {
			invalidateToken(p.Channel.Id)
		}
		return nil, errWithCode
	}

	// Ensure usage is populated.
	if response.Usage == nil {
		response.Usage = p.Usage
	} else {
		p.Usage.PromptTokens = response.Usage.PromptTokens
		p.Usage.CompletionTokens = response.Usage.CompletionTokens
		p.Usage.TotalTokens = response.Usage.TotalTokens
	}

	return response, nil
}

// CreateChatCompletionStream sends a streaming chat request to the Copilot API.
func (p *CopilotProvider) CreateChatCompletionStream(request *types.ChatCompletionRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode) {
	initiator := copilotInitiatorFromMessages(request.Messages)
	headers, errWithCode := p.buildCopilotHeaders(initiator)
	if errWithCode != nil {
		return nil, errWithCode
	}
	headers["Accept"] = "text/event-stream"

	baseURL := strings.TrimSuffix(p.chatBaseURL(), "/")
	fullURL := fmt.Sprintf("%s/chat/completions", baseURL)

	req, err := p.Requester.NewRequest(http.MethodPost, fullURL, p.Requester.WithBody(request), p.Requester.WithHeader(headers))
	if err != nil {
		return nil, common.ErrorWrapper(err, "new_request_failed", http.StatusInternalServerError)
	}
	defer req.Body.Close()

	resp, errWithCode := p.Requester.SendRequestRaw(req)
	if errWithCode != nil {
		if errWithCode.StatusCode == http.StatusUnauthorized {
			invalidateToken(p.Channel.Id)
		}
		return nil, errWithCode
	}

	handler := &copilotChatStreamHandler{
		usage:   p.Usage,
		request: request,
	}

	return requester.RequestStream(p.Requester, resp, handler.handle)
}

// ─── Stream handler ───────────────────────────────────────────────────────────

type copilotChatStreamHandler struct {
	usage   *types.Usage
	request *types.ChatCompletionRequest
}

func (h *copilotChatStreamHandler) handle(rawLine *[]byte, dataChan chan string, errChan chan error) {
	if rawLine == nil || len(*rawLine) == 0 {
		return
	}

	line := string(*rawLine)

	if !strings.HasPrefix(line, "data:") {
		return
	}

	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

	if data == "[DONE]" {
		return
	}

	var chunk types.ChatCompletionStreamResponse
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}

	// Accumulate usage from the final chunk.
	if chunk.Usage != nil {
		h.usage.PromptTokens = chunk.Usage.PromptTokens
		h.usage.CompletionTokens = chunk.Usage.CompletionTokens
		h.usage.TotalTokens = chunk.Usage.TotalTokens
	}

	// Re-serialise and forward.
	out, err := json.Marshal(chunk)
	if err != nil {
		return
	}
	dataChan <- string(out)
}

// collectStreamResponse accumulates a stream into a non-stream response.
func (p *CopilotProvider) collectChatStreamResponse(stream requester.StreamReaderInterface[string], request *types.ChatCompletionRequest) (*types.ChatCompletionResponse, *types.OpenAIErrorWithStatusCode) {
	var fullContent strings.Builder
	var responseID string
	model := request.Model
	finishReason := "stop"

	dataChan, errChan := stream.Recv()

	for {
		select {
		case data, ok := <-dataChan:
			if !ok {
				goto buildResponse
			}
			var chunk types.ChatCompletionStreamResponse
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if responseID == "" && chunk.ID != "" {
				responseID = chunk.ID
			}
			if chunk.Model != "" {
				model = chunk.Model
			}
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				fullContent.WriteString(choice.Delta.Content)
				if fr, ok := choice.FinishReason.(string); ok && fr != "" {
					finishReason = fr
				}
			}
		case err, ok := <-errChan:
			if !ok {
				continue
			}
			if err != nil && err.Error() != "EOF" {
				return nil, common.ErrorWrapper(err, "stream_read_failed", http.StatusInternalServerError)
			}
			goto buildResponse
		}
	}

buildResponse:
	return &types.ChatCompletionResponse{
		ID:      responseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []types.ChatCompletionChoice{
			{
				Index: 0,
				Message: types.ChatCompletionMessage{
					Role:    "assistant",
					Content: fullContent.String(),
				},
				FinishReason: finishReason,
			},
		},
		Usage: &types.Usage{
			PromptTokens:     p.Usage.PromptTokens,
			CompletionTokens: p.Usage.CompletionTokens,
			TotalTokens:      p.Usage.TotalTokens,
		},
	}, nil
}
