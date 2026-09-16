package openai

import (
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
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

const chatResponseFixture = `{ "id":"chat-1", "object":"chat.completion", "model":"upstream-model", "choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}], "usage":{"prompt_tokens":89,"completion_tokens":59,"total_tokens":148,"prompt_tokens_details":{"cached_tokens":64}}, "vendor_extra":{"keep":"<value>"} }`

func TestChatCompletionResponseEnvelope(t *testing.T) {
	toolResponse := strings.Replace(chatResponseFixture, `"content":"OK"`, `"content":null,"tool_calls":[{"id":"call-1","type":"function","function":{"name":"lookup","arguments":"{}"}}]`, 1)
	for _, inner := range []string{chatResponseFixture, toolResponse} {
		var got chatCompletionResponse
		if err := json.Unmarshal([]byte(`{"success":true,"data":`+inner+`}`), &got); err != nil {
			t.Fatal(err)
		}
		var want OpenAIProviderChatResponse
		if err := json.Unmarshal([]byte(inner), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.OpenAIProviderChatResponse, want) || string(got.unwrappedBody) != inner {
			t.Fatalf("解包应保留完整响应和内层字节: %#v", got)
		}
	}
}

func TestChatCompletionResponsePreservesExistingParsing(t *testing.T) {
	cases := map[string]string{
		"standard":        chatResponseFixture,
		"top_level_wins":  `{"choices":[{"message":{"content":"top"}}],"success":true,"data":` + chatResponseFixture + `}`,
		"empty_choices":   `{"choices":[],"success":true,"data":` + chatResponseFixture + `}`,
		"null_choices":    `{"choices":null,"success":true,"data":` + chatResponseFixture + `}`,
		"standard_error":  `{"error":{"message":"denied","type":"upstream_error"},"success":true,"data":` + chatResponseFixture + `}`,
		"null_error":      `{"error":null,"success":true,"data":` + chatResponseFixture + `}`,
		"false_success":   `{"success":false,"data":` + chatResponseFixture + `}`,
		"missing_success": `{"data":` + chatResponseFixture + `}`,
		"string_success":  `{"success":"true","data":` + chatResponseFixture + `}`,
		"unrelated_data":  `{"success":true,"data":{"object":"list","choices":[{}]}}`,
		"array_data":      `{"success":true,"data":[` + chatResponseFixture + `]}`,
		"null_data":       `{"success":true,"data":null}`,
		"no_choices":      `{"success":true,"data":{"object":"chat.completion","choices":[]}}`,
		"nested_envelope": `{"success":true,"data":{"success":true,"data":` + chatResponseFixture + `}}`,
		"invalid_json":    `{"choices":`,
		"invalid_choices": `{"choices":"invalid"}`,
		"null":            `null`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			var got chatCompletionResponse
			var want OpenAIProviderChatResponse
			gotErr := json.Unmarshal([]byte(body), &got)
			wantErr := json.Unmarshal([]byte(body), &want)
			if (gotErr == nil) != (wantErr == nil) {
				t.Fatalf("改变了原有错误行为: got=%v want=%v", gotErr, wantErr)
			}
			if gotErr == nil && !reflect.DeepEqual(got.OpenAIProviderChatResponse, want) {
				t.Fatalf("改变了标准解析结果: got=%#v want=%#v", got, want)
			}
			if got.unwrappedBody != nil {
				t.Fatal("不应触发解包")
			}
		})
	}
}

func newChatResponseTestProvider(t *testing.T, body string, stream bool) *OpenAIProvider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("unexpected upstream request: %s", r.URL.Path)
		}
		var request types.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Stream != stream {
			t.Errorf("unexpected stream flag: %v, %v", request.Stream, err)
		}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	oldClient, oldLogger := requester.HTTPClient, logger.Logger
	requester.HTTPClient, logger.Logger = server.Client(), zap.NewNop()
	t.Cleanup(func() { requester.HTTPClient, logger.Logger = oldClient, oldLogger })
	provider := CreateOpenAIProvider(&model.Channel{Type: config.ChannelTypeOpenAI, Key: "test-key"}, server.URL)
	provider.SetContext(base.NewMemoryRequestContext(httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)))
	provider.SetUsage(&types.Usage{PromptTokens: 17})
	return provider
}

func TestCreateChatCompletionEnvelopeAndPassthrough(t *testing.T) {
	for _, wrapped := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			for _, mapped := range []bool{false, true} {
				name := fmt.Sprintf("wrapped=%t/passthrough=%t/mapped=%t", wrapped, passthrough, mapped)
				t.Run(name, func(t *testing.T) {
					body := chatResponseFixture + "\n \t"
					if wrapped {
						body = `{"success":true,"data":` + chatResponseFixture + `}`
					}
					provider := newChatResponseTestProvider(t, body, false)
					provider.Channel.PassThroughBody = passthrough
					provider.Context.Set(config.GinRawPassThroughAllowedKey, passthrough)
					oldUnified := config.UnifiedRequestResponseModelEnabled
					config.UnifiedRequestResponseModelEnabled = mapped
					t.Cleanup(func() { config.UnifiedRequestResponseModelEnabled = oldUnified })
					provider.SetOriginalModel("client-model")
					response, apiErr := provider.CreateChatCompletion(&types.ChatCompletionRequest{Model: "upstream-model"})
					if apiErr != nil {
						t.Fatal(apiErr)
					}
					if response.GetContent() != "OK" || provider.Usage.PromptTokens != 89 || provider.Usage.CompletionTokens != 59 || provider.Usage.PromptTokensDetails.CachedTokens != 64 {
						t.Fatalf("正文或计费数据不正确: response=%#v usage=%#v", response, provider.Usage)
					}
					wantModel := "upstream-model"
					if mapped {
						wantModel = "client-model"
					}
					if response.Model != wantModel {
						t.Fatalf("model=%q want=%q", response.Model, wantModel)
					}
					raw, exists := provider.Context.Get(config.GinRawResponseBodyKey)
					if exists != passthrough {
						t.Fatalf("原始响应透传状态错误: %v", exists)
					}
					if passthrough {
						wantRaw := body
						if wrapped {
							wantRaw = chatResponseFixture
						}
						if mapped {
							wantRaw = strings.Replace(wantRaw, `"model":"upstream-model"`, `"model":"client-model"`, 1)
						}
						if string(raw.([]byte)) != wantRaw {
							t.Fatalf("原始响应字节不匹配: %s", raw)
						}
					}
				})
			}
		}
	}
}

func TestCreateChatCompletionStreamRemainsStandard(t *testing.T) {
	chunk := `{"object":"chat.completion.chunk","model":"upstream-model","choices":[{"index":0,"delta":{"content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":89,"completion_tokens":59,"total_tokens":148}}`
	provider := newChatResponseTestProvider(t, "data: "+chunk+"\n\ndata: [DONE]\n\n", true)
	stream, apiErr := provider.CreateChatCompletionStream(&types.ChatCompletionRequest{Model: "upstream-model", Stream: true})
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	defer stream.Close()
	dataCh, errCh := stream.Recv()
	var chunks []string
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for dataCh != nil || errCh != nil {
		select {
		case data, ok := <-dataCh:
			if !ok {
				dataCh = nil
			} else {
				chunks = append(chunks, data)
			}
		case err, ok := <-errCh:
			if !ok {
				errCh = nil
			} else if err != io.EOF {
				t.Fatal(err)
			}
		case <-deadline.C:
			t.Fatal("stream did not finish")
		}
	}
	if len(chunks) != 1 || chunks[0] != chunk || provider.Usage.CompletionTokens != 59 {
		t.Fatalf("流式响应改变: chunks=%v usage=%#v", chunks, provider.Usage)
	}
}
