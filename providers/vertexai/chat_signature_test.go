package vertexai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"done-hub/common/cache"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/types"
	"go.uber.org/zap"
)

// 两轮工具调用后继续生成：通过真实 provider HTTP 入口验证全部历史签名，
// 包括 Vertex 请求清理、自定义参数处理和 OpenAI JSON 存取。
func TestVertexToolSignatureHTTPRoundTrip(t *testing.T) {
	oldClient, oldLogger := requester.HTTPClient, logger.Logger
	logger.Logger = zap.NewNop()
	cache.InitCacheManager()
	t.Cleanup(func() { requester.HTTPClient, logger.Logger = oldClient, oldLogger })
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				var signatures []string
				for _, value := range body["contents"].([]any) {
					content := value.(map[string]any)
					for _, value := range content["parts"].([]any) {
						part := value.(map[string]any)
						if _, ok := part["functionCall"]; !ok {
							continue
						}
						sig, _ := part["thoughtSignature"].(string)
						signatures = append(signatures, sig)
					}
				}
				if len(signatures) != requests-1 {
					t.Errorf("round %d: signatures=%v", requests, signatures)
				}
				for i, sig := range signatures {
					if sig != fmt.Sprintf("opaque-signature-%d+/=", i+1) {
						t.Errorf("round %d: signature changed/lost: %q", requests, sig)
					}
				}
				part := `{"text":"Done"}`
				if requests < 3 {
					part = fmt.Sprintf(`{"functionCall":{"name":"generate_image","args":{"prompt":"cat %d"}},"thoughtSignature":"opaque-signature-%d+/="}`, requests, requests)
				}
				response := `{"candidates":[{"index":0,"content":{"role":"model","parts":[` + part + `]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`
				if strings.Contains(r.URL.Path, "streamGenerateContent") {
					w.Header().Set("Content-Type", "text/event-stream")
					fmt.Fprintf(w, "data: %s\n\n", response)
				} else {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, response)
				}
			}))
			defer server.Close()
			requester.HTTPClient = server.Client()
			channel := &model.Channel{Id: 987655, Type: 42, Other: "global|test-project", Key: "test-credential"}
			if err := cache.SetCache(channel.CredentialCacheKey(TokenCacheKey), "test-token", time.Minute); err != nil {
				t.Fatal(err)
			}
			p := (VertexAIProviderFactory{}).CreateAdapter(channel).(*VertexAIProvider)
			p.Config.BaseURL = server.URL + "/%sprojects/%s/locations/%s/models/%s:%s"
			p.Usage = &types.Usage{}
			req := &types.ChatCompletionRequest{Model: "gemini-4-argon", Stream: stream, Messages: []types.ChatCompletionMessage{{Role: "user", Content: "Generate images"}}}
			for round := 1; round <= 3; round++ {
				var message types.ChatCompletionMessage
				if stream {
					reader, apiErr := p.CreateChatCompletionStream(req)
					if apiErr != nil {
						t.Fatal(apiErr)
					}
					message = collectSignatureToolStream(t, reader)
				} else {
					response, apiErr := p.CreateChatCompletion(req)
					if apiErr != nil {
						t.Fatal(apiErr)
					}
					message = response.Choices[0].Message
				}
				if round == 3 {
					if message.StringContent() != "Done" {
						t.Fatalf("missing final text: %+v", message)
					}
					break
				}
				if len(message.ToolCalls) != 1 {
					t.Fatalf("missing tool call: %+v", message)
				}
				req.Messages = append(req.Messages, message, types.ChatCompletionMessage{Role: "tool", ToolCallID: message.ToolCalls[0].Id, Content: "image generated"})
				// 客户端重发整个对话，经历实际 JSON 解码而非直接复用内存对象。
				encoded, err := json.Marshal(req)
				if err != nil {
					t.Fatal(err)
				}
				req = &types.ChatCompletionRequest{}
				if err := json.Unmarshal(encoded, req); err != nil {
					t.Fatal(err)
				}
			}
			if requests != 3 {
				t.Fatalf("requests=%d", requests)
			}
		})
	}
}

func collectSignatureToolStream(t *testing.T, reader requester.StreamReaderInterface[string]) types.ChatCompletionMessage {
	t.Helper()
	defer reader.Close()
	message := types.ChatCompletionMessage{Role: "assistant"}
	data, errs := reader.Recv()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	text := ""
	for data != nil || errs != nil {
		select {
		case raw, ok := <-data:
			if !ok {
				data = nil
				continue
			}
			var chunk types.ChatCompletionStreamResponse
			if err := json.Unmarshal([]byte(raw), &chunk); err != nil {
				t.Fatal(err)
			}
			for _, choice := range chunk.Choices {
				text += choice.Delta.Content
				for _, tc := range choice.Delta.ToolCalls {
					if tc.Id != "" {
						message.ToolCalls = append(message.ToolCalls, tc)
						continue
					}
					message.ToolCalls[tc.Index].Function.Arguments += tc.Function.Arguments
				}
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
			} else if err != io.EOF {
				t.Fatal(err)
			}
		case <-deadline.C:
			t.Fatal("tool stream did not finish")
		}
	}
	message.Content = text
	return message
}
