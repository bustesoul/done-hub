package openai

import (
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/registry"
	"done-hub/internal/gateway/requeststate"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// The receiver decodes into generic JSON, not the same types used by the sender:
// otherwise a shared codec bug could make both sides agree on a broken payload.
func TestToolRequestsAcrossChatAndResponses(t *testing.T) {
	function := `{"name":"echo","description":"echo text","parameters":{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false},"strict":false}`
	chatTools := `[{"type":"function","function":` + function + `}]`
	responsesTools := `[{"type":"function",` + function[1:] + `]`
	for _, from := range []string{"chat", "responses"} {
		for _, to := range []string{"chat", "responses"} {
			modes := []string{"structured", "passthrough", "extra_body", "passthrough_extra_body"}
			if from == "responses" && to == "responses" {
				modes = append(modes, "no_raw_body")
			}
			for _, mode := range modes {
				for _, streaming := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s_to_%s/%s/stream=%t", from, to, mode, streaming), func(t *testing.T) {
						inPath, outPath := "/v1/chat/completions", "/v1/chat/completions"
						toolsJSON, wantTools := chatTools, chatTools
						input := `"messages":[{"role":"user","content":"echo OK"}]`
						if from == "responses" {
							inPath, toolsJSON, input = "/v1/responses", responsesTools, `"input":"echo OK"`
						}
						if to == "responses" {
							outPath, wantTools = "/v1/responses", responsesTools
						}
						choice, wantChoice := `{"type":"function","function":{"name":"echo"}}`, `{"type":"function","function":{"name":"echo"}}`
						if from == "responses" {
							choice = `{"type":"function","name":"echo"}`
						}
						if to == "responses" {
							wantChoice = `{"type":"function","name":"echo"}`
						}
						raw := fmt.Sprintf(`{"model":"client-model",%s,"stream":%t,"tools":%s,"tool_choice":%s,"x-client-extra":"keep-native-only"}`, input, streaming, toolsJSON, choice)
						var expectedChoice any
						if err := json.Unmarshal([]byte(wantChoice), &expectedChoice); err != nil {
							t.Fatal(err)
						}
						var expectedTools any
						if err := json.Unmarshal([]byte(wantTools), &expectedTools); err != nil {
							t.Fatal(err)
						}
						received := make(chan map[string]any, 1)
						server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							if r.URL.Path != outPath {
								t.Errorf("上游路径=%s，期望 %s", r.URL.Path, outPath)
							}
							var body map[string]any
							if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
								t.Error(err)
							}
							received <- body
							response := `{"id":"resp-test","object":"response","status":"completed","model":"upstream-model","output":[],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4}}`
							if streaming {
								w.Header().Set("Content-Type", "text/event-stream")
								if to == "chat" {
									fmt.Fprint(w, "data: {\"id\":\"chat-test\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
								} else {
									fmt.Fprintf(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":%s}\n\n", response)
								}
							} else if to == "chat" {
								fmt.Fprint(w, chatResponseFixture)
							} else {
								fmt.Fprint(w, response)
							}
						}))
						defer server.Close()
						oldClient, oldLogger := requester.HTTPClient, logger.Logger
						requester.HTTPClient, logger.Logger = server.Client(), zap.NewNop()
						defer func() { requester.HTTPClient, logger.Logger = oldClient, oldLogger }()
						customParams := `{"audit_tag":"server"}`
						provider := CreateOpenAIProvider(&model.Channel{Type: config.ChannelTypeOpenAI, Key: "test-key", PassThroughBody: strings.Contains(mode, "passthrough"), AllowExtraBody: strings.Contains(mode, "extra_body"), CustomParameter: &customParams}, server.URL)
						bodyForContext := raw
						if mode == "no_raw_body" {
							bodyForContext = ""
						}
						inRequest, state := requeststate.Ensure(httptest.NewRequest(http.MethodPost, inPath, strings.NewReader(bodyForContext)))
						protocols := map[string]domain.Protocol{"chat": domain.ProtocolOpenAIChat, "responses": domain.ProtocolOpenAIResponses}
						state.BindProtocolRoute(registry.ConverterSpec{From: protocols[from], To: protocols[to]})
						ctx := base.NewMemoryRequestContext(inRequest)
						if mode != "no_raw_body" {
							ctx.Set(config.GinRequestBodyKey, []byte(raw))
						}
						ctx.Set(config.GinRawPassThroughAllowedKey, from == to)
						provider.SetContext(ctx)
						provider.SetUsage(&types.Usage{PromptTokens: 3})
						var chat *types.ChatCompletionRequest
						var responses *types.OpenAIResponsesRequest
						if from == "chat" {
							chat = &types.ChatCompletionRequest{}
							if err := json.Unmarshal([]byte(raw), chat); err != nil {
								t.Fatal(err)
							}
							if to == "responses" {
								responses = chat.ToResponsesRequest()
								responses.ConvertChat = true
							}
						} else {
							responses = &types.OpenAIResponsesRequest{}
							if err := json.Unmarshal([]byte(raw), responses); err != nil {
								t.Fatal(err)
							}
							if to == "chat" {
								var err error
								chat, err = responses.ToChatCompletionRequest()
								if err != nil {
									t.Fatal(err)
								}
							}
						}
						var stream requester.StreamReaderInterface[string]
						var apiErr *types.OpenAIErrorWithStatusCode
						if to == "chat" {
							chat.Model = "upstream-model"
							if streaming {
								stream, apiErr = provider.CreateChatCompletionStream(chat)
							} else {
								_, apiErr = provider.CreateChatCompletion(chat)
							}
						} else {
							responses.Model = "upstream-model"
							if streaming {
								stream, apiErr = provider.CreateResponsesStream(responses)
							} else {
								_, apiErr = provider.CreateResponses(responses)
							}
						}
						if apiErr != nil {
							t.Fatal(apiErr)
						}
						if stream != nil {
							drainToolTestStream(t, stream)
						}
						body := <-received
						if !reflect.DeepEqual(body["tools"], expectedTools) {
							t.Fatalf("上游实际 tools=%v，期望 %v", body["tools"], expectedTools)
						}
						actualStream, _ := body["stream"].(bool)
						if body["model"] != "upstream-model" || actualStream != streaming || !reflect.DeepEqual(body["tool_choice"], expectedChoice) || body["audit_tag"] != "server" {
							t.Fatalf("模型映射或请求选项改变: %v", body)
						}
						if from != to {
							oldInput := "input"
							if from == "chat" {
								oldInput = "messages"
							}
							if _, exists := body[oldInput]; exists {
								t.Fatalf("跨协议混入原始字段 %s: %v", oldInput, body)
							}
							if _, exists := body["x-client-extra"]; exists {
								t.Fatal("跨协议不应合并原始未知字段")
							}
						}
						if from == to && mode != "no_raw_body" && (from == "responses" || mode != "structured") && body["x-client-extra"] != "keep-native-only" {
							t.Fatal("同协议透传丢失扩展字段")
						}
					})
				}
			}
		}
	}
}

func drainToolTestStream(t *testing.T, stream requester.StreamReaderInterface[string]) {
	t.Helper()
	defer stream.Close()
	data, errs := stream.Recv()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for data != nil || errs != nil {
		select {
		case _, ok := <-data:
			if !ok {
				data = nil
			}
		case err, ok := <-errs:
			if !ok {
				errs = nil
			} else if err != io.EOF {
				t.Fatal(err)
			}
		case <-deadline.C:
			t.Fatal("流式请求未结束")
		}
	}
}

func TestNativeNonChatRequestPassthroughWithoutProtocolRoute(t *testing.T) {
	for _, mode := range []int{config.RelayModeEmbeddings, config.RelayModeModerations, config.RelayModeAudioSpeech} {
		t.Run(fmt.Sprint(mode), func(t *testing.T) {
			provider := CreateOpenAIProvider(&model.Channel{Type: config.ChannelTypeOpenAI, PassThroughBody: true}, "http://unused.invalid")
			provider.SetContext(base.NewMemoryRequestContext(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"client-model","input":"hello","x-extra":true}`))))
			req, apiErr := provider.GetRequestTextBody(mode, "upstream-model", map[string]any{"model": "upstream-model"})
			if apiErr != nil {
				t.Fatal(apiErr)
			}
			defer req.Body.Close()
			var body map[string]any
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["input"] != "hello" || body["x-extra"] != true || body["model"] != "upstream-model" {
				t.Fatalf("改变了无协议路由的原生透传: %v", body)
			}
		})
	}
}

func TestConvertedChatRequestDoesNotReuseRawBodyWithoutRoute(t *testing.T) {
	provider := CreateOpenAIProvider(&model.Channel{Type: config.ChannelTypeOpenAI, PassThroughBody: true, AllowExtraBody: true}, "http://unused.invalid")
	provider.SetContext(base.NewMemoryRequestContext(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"client-model","messages":[],"x-extra":true}`))))
	req, apiErr := provider.getResponsesRequest(&types.OpenAIResponsesRequest{Model: "upstream-model", Input: "converted", ConvertChat: true})
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	defer req.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["input"] != "converted" || body["messages"] != nil || body["x-extra"] != nil {
		t.Fatalf("转换后的请求被原始请求覆盖: %v", body)
	}
}
