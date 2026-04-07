package copilot

import (
	"done-hub/common"
	"done-hub/common/requester"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// CreateResponses sends a non-streaming Responses API request to the Copilot API.
func (p *CopilotProvider) CreateResponses(request *types.OpenAIResponsesRequest) (*types.OpenAIResponsesResponses, *types.OpenAIErrorWithStatusCode) {
	req, errWithCode := p.getResponsesHTTPRequest(request, false)
	if errWithCode != nil {
		return nil, errWithCode
	}
	defer req.Body.Close()

	resp, errWithCode := p.Requester.SendRequestRaw(req)
	if errWithCode != nil {
		if errWithCode.StatusCode == http.StatusUnauthorized {
			invalidateToken(p.Channel.Id)
		}
		return nil, errWithCode
	}

	handler := &copilotResponsesStreamHandler{usage: p.Usage}
	stream, errWithCode := requester.RequestNoTrimStream(p.Requester, resp, handler.handle)
	if errWithCode != nil {
		return nil, errWithCode
	}

	return p.collectResponsesStream(stream)
}

// CreateResponsesStream sends a streaming Responses API request to the Copilot API.
func (p *CopilotProvider) CreateResponsesStream(request *types.OpenAIResponsesRequest) (requester.StreamReaderInterface[string], *types.OpenAIErrorWithStatusCode) {
	req, errWithCode := p.getResponsesHTTPRequest(request, true)
	if errWithCode != nil {
		return nil, errWithCode
	}
	defer req.Body.Close()

	resp, errWithCode := p.Requester.SendRequestRaw(req)
	if errWithCode != nil {
		if errWithCode.StatusCode == http.StatusUnauthorized {
			invalidateToken(p.Channel.Id)
		}
		return nil, errWithCode
	}

	handler := &copilotResponsesStreamHandler{usage: p.Usage}
	return requester.RequestNoTrimStream(p.Requester, resp, handler.handle)
}

// getResponsesHTTPRequest builds the HTTP request for /responses.
func (p *CopilotProvider) getResponsesHTTPRequest(request *types.OpenAIResponsesRequest, stream bool) (*http.Request, *types.OpenAIErrorWithStatusCode) {
	headers, errWithCode := p.buildCopilotHeaders("user")
	if errWithCode != nil {
		return nil, errWithCode
	}

	if stream {
		headers["Accept"] = "text/event-stream"
	} else {
		headers["Accept"] = "application/json"
	}

	// /responses is only available on the canonical base URL (not plan subdomains).
	fullURL := fmt.Sprintf("%s/responses", strings.TrimSuffix(CopilotAPIBase, "/"))

	req, err := p.Requester.NewRequest(http.MethodPost, fullURL, p.Requester.WithBody(request), p.Requester.WithHeader(headers))
	if err != nil {
		return nil, common.ErrorWrapper(err, "new_request_failed", http.StatusInternalServerError)
	}

	return req, nil
}

// ─── Stream handler ───────────────────────────────────────────────────────────

type copilotResponsesStreamHandler struct {
	usage       *types.Usage
	eventType   string
	eventBuffer strings.Builder
}

// handle passes Responses API SSE events through while extracting usage data.
func (h *copilotResponsesStreamHandler) handle(rawLine *[]byte, dataChan chan string, errChan chan error) {
	rawStr := string(*rawLine)

	if strings.HasPrefix(rawStr, "event: ") {
		h.eventType = strings.TrimPrefix(rawStr, "event: ")
		h.eventBuffer.Reset()
		h.eventBuffer.WriteString(rawStr)
		h.eventBuffer.WriteString("\n")
		return
	}

	if !strings.HasPrefix(rawStr, "data: ") {
		if h.eventBuffer.Len() > 0 {
			h.eventBuffer.WriteString(rawStr)
			h.eventBuffer.WriteString("\n")
		} else {
			dataChan <- rawStr
		}
		return
	}

	dataLine := strings.TrimSpace(strings.TrimPrefix(rawStr, "data: "))
	if dataLine == "[DONE]" {
		if h.eventBuffer.Len() > 0 {
			dataChan <- h.eventBuffer.String()
			h.eventBuffer.Reset()
			h.eventType = ""
		}
		return
	}

	// Opportunistically extract usage from terminal event.
	var event types.OpenAIResponsesStreamResponses
	if err := json.Unmarshal([]byte(dataLine), &event); err == nil {
		if event.Type == "response.completed" && event.Response != nil && event.Response.Usage != nil {
			h.usage.PromptTokens = event.Response.Usage.InputTokens
			h.usage.CompletionTokens = event.Response.Usage.OutputTokens
			h.usage.TotalTokens = event.Response.Usage.TotalTokens
		}
	}

	// Pass through the full SSE event.
	if h.eventBuffer.Len() > 0 {
		h.eventBuffer.WriteString(rawStr)
		h.eventBuffer.WriteString("\n")
		if strings.HasSuffix(h.eventBuffer.String(), "\n\n") {
			dataChan <- h.eventBuffer.String()
			h.eventBuffer.Reset()
			h.eventType = ""
		}
	} else {
		dataChan <- rawStr
	}
}

// collectResponsesStream accumulates a Responses stream into a complete response.
func (p *CopilotProvider) collectResponsesStream(stream requester.StreamReaderInterface[string]) (*types.OpenAIResponsesResponses, *types.OpenAIErrorWithStatusCode) {
	var response *types.OpenAIResponsesResponses

	dataChan, errChan := stream.Recv()

	for {
		select {
		case data, ok := <-dataChan:
			if !ok {
				goto buildResponse
			}
			if strings.TrimSpace(data) == "" {
				continue
			}
			jsonData := extractResponsesJSONFromSSE(data)
			if jsonData == "" {
				continue
			}
			var ev types.OpenAIResponsesStreamResponses
			if err := json.Unmarshal([]byte(jsonData), &ev); err != nil {
				continue
			}
			if ev.Type == "response.completed" && ev.Response != nil {
				response = ev.Response
				if response.Usage != nil {
					p.Usage.PromptTokens = response.Usage.InputTokens
					p.Usage.CompletionTokens = response.Usage.OutputTokens
					p.Usage.TotalTokens = response.Usage.TotalTokens
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
	if response == nil {
		return nil, common.StringErrorWrapperLocal("no response received from Copilot /responses", "no_response", http.StatusInternalServerError)
	}
	return response, nil
}

// extractResponsesJSONFromSSE extracts the JSON payload from an SSE data line.
func extractResponsesJSONFromSSE(sseData string) string {
	for _, line := range strings.Split(sseData, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			return strings.TrimPrefix(line, "data: ")
		}
	}
	return ""
}
