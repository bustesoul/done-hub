package openai

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/types"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type signatureTestTransport func(*http.Request) (*http.Response, error)

func (f signatureTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestToolSignatureTargetPolicyOnWire(t *testing.T) {
	for _, target := range []struct {
		name, address string
		channelType   int
		strip         bool
	}{
		{"official", "https://api.openai.com", config.ChannelTypeOpenAI, true},
		{"official_custom", "https://api.openai.com", config.ChannelTypeCustom, true},
		{"unknown_compatible", "https://relay.example", config.ChannelTypeOpenAI, false},
		{"lookalike", "https://api.openai.com.example", config.ChannelTypeOpenAI, false},
		{"google_compatible", "https://generativelanguage.googleapis.com", config.ChannelTypeGemini, false},
	} {
		for _, mode := range []string{"structured", "passthrough", "extra_body", "custom"} {
			for _, streaming := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/stream=%v", target.name, mode, streaming), func(t *testing.T) {
					// The second call has unrelated fields that must never be removed.
					messages := `[{"role":"assistant","tool_calls":[{"id":"call_a","type":"function","function":{"name":"echo","arguments":"{}"},"extra_content":{"google":{"thought_signature":"signature-a"}}},{"id":"call_b","type":"function","function":{"name":"lookup","arguments":"{}"},"extra_content":{"google":{"thought_signature":"signature-b","keep_google":1},"vendor":{"keep":true}}}]}]`
					raw := fmt.Sprintf(`{"model":"gemini-alias","stream":%v,"messages":%s}`, streaming, messages)
					received := make(chan []byte, 1)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							t.Error(err)
							return
						}
						if r.ContentLength != int64(len(body)) {
							t.Errorf("wrong ContentLength: %d/%d", r.ContentLength, len(body))
						}
						received <- body
						if streaming {
							w.Header().Set("Content-Type", "text/event-stream")
							fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
						} else {
							fmt.Fprint(w, chatResponseFixture)
						}
					}))
					defer server.Close()
					local, _ := url.Parse(server.URL)
					oldClient, oldLogger := requester.HTTPClient, logger.Logger
					logger.Logger = zap.NewNop()
					defer func() { requester.HTTPClient, logger.Logger = oldClient, oldLogger }()
					requester.HTTPClient = &http.Client{Transport: signatureTestTransport(func(r *http.Request) (*http.Response, error) {
						if r.URL.Scheme+"://"+r.URL.Host != target.address {
							t.Errorf("unexpected target %s", r.URL)
						}
						body, err := io.ReadAll(r.Body)
						if err != nil {
							return nil, err
						}
						if r.GetBody == nil {
							t.Error("missing GetBody")
						} else {
							copyBody, err := r.GetBody()
							if err != nil {
								return nil, err
							}
							copyBytes, _ := io.ReadAll(copyBody)
							copyBody.Close()
							if string(body) != string(copyBytes) {
								t.Error("GetBody retains stale signature")
							}
						}
						r.Body.Close()
						cloned := r.Clone(r.Context())
						cloned.URL.Scheme, cloned.URL.Host, cloned.Host = local.Scheme, local.Host, local.Host
						cloned.Body = io.NopCloser(strings.NewReader(string(body)))
						return server.Client().Transport.RoundTrip(cloned)
					})}
					channel := &model.Channel{Type: target.channelType, Key: "test-key", PassThroughBody: mode == "passthrough", AllowExtraBody: mode == "extra_body"}
					// Signatures injected by final custom parameters must also be removed.
					custom := `{"messages":` + strings.ReplaceAll(messages, "signature-", "custom-signature-") + `}`
					if mode == "custom" || mode == "passthrough" {
						channel.CustomParameter = &custom
					}
					p := CreateOpenAIProvider(channel, target.address)
					ctx := base.NewMemoryRequestContext(httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(raw)))
					ctx.Set(config.GinRequestBodyKey, []byte(raw))
					ctx.Set(config.GinRawPassThroughAllowedKey, true)
					p.SetContext(ctx)
					p.SetUsage(&types.Usage{})
					var req types.ChatCompletionRequest
					if err := json.Unmarshal([]byte(raw), &req); err != nil {
						t.Fatal(err)
					}
					before, _ := json.Marshal(req.Messages)
					if streaming {
						stream, apiErr := p.CreateChatCompletionStream(&req)
						if apiErr != nil {
							t.Fatal(apiErr)
						}
						drainToolTestStream(t, stream)
					} else {
						if _, apiErr := p.CreateChatCompletion(&req); apiErr != nil {
							t.Fatal(apiErr)
						}
					}
					body := <-received
					for i := 0; i < 2; i++ {
						prefix := fmt.Sprintf("messages.0.tool_calls.%d", i)
						sig := gjson.GetBytes(body, prefix+".extra_content.google.thought_signature")
						if target.strip == sig.Exists() {
							t.Errorf("wrong signature policy: %s", body)
						}
						if !gjson.GetBytes(body, prefix+".function.name").Exists() {
							t.Error("function lost")
						}
					}
					if target.strip && gjson.GetBytes(body, "messages.0.tool_calls.0.extra_content").Exists() {
						t.Error("empty signature container retained")
					}
					if gjson.GetBytes(body, "messages.0.tool_calls.1.extra_content.google.keep_google").Int() != 1 || !gjson.GetBytes(body, "messages.0.tool_calls.1.extra_content.vendor.keep").Bool() {
						t.Errorf("unrelated fields removed: %s", body)
					}
					after, _ := json.Marshal(req.Messages)
					rawBody, _ := p.GetRawBody()
					if string(before) != string(after) || string(rawBody) != raw {
						t.Error("client history was mutated")
					}
				})
			}
		}
	}
}

func TestOpenAIStreamToolCallsPreserveStandardAndExtraFields(t *testing.T) {
	for _, escape := range []bool{false, true} {
		for _, extra := range []string{"", `,"extra_content":"vendor-string"`, `,"extra_content":{"vendor":{"x":1}}`, `,"extra_content":{"google":{"thought_signature":"opaque"}}`} {
			t.Run(fmt.Sprintf("escape=%v/extra=%s", escape, extra), func(t *testing.T) {
				h := OpenAIStreamHandler{Usage: &types.Usage{}, EscapeJSON: escape}
				for _, delta := range []string{
					`{"role":"assistant","content":"hello"}`,
					`{"tool_calls":[{"id":"call_a","type":"function","index":0,"function":{"name":"echo","arguments":""}` + extra + `},{"id":"call_b","type":"function","index":1,"function":{"name":"lookup","arguments":""}}]}`,
					`{"tool_calls":[{"index":1,"function":{"arguments":"{}"}}]}`,
				} {
					raw := `{"id":"chat-test","object":"chat.completion.chunk","choices":[{"index":0,"delta":` + delta + `,"finish_reason":null}]}`
					line := []byte("data: " + raw)
					data, errs := make(chan string, 2), make(chan error, 1)
					h.HandlerChatStream(&line, data, errs)
					if len(errs) > 0 {
						t.Fatal(<-errs)
					}
					if len(data) != 1 {
						t.Fatalf("chunks=%d", len(data))
					}
					out := <-data
					if !escape && out != raw {
						t.Fatal("raw OpenAI stream changed")
					}
					var want, got map[string]any
					json.Unmarshal([]byte(raw), &want)
					json.Unmarshal([]byte(out), &got)
					if !reflect.DeepEqual(want["choices"], got["choices"]) {
						t.Fatalf("OpenAI choices changed: %s", out)
					}
				}
			})
		}
	}
}
