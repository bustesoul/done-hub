package gemini

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"done-hub/types"
)

// Exercise actual JSON on both sides; signatures are opaque, even when short.
func TestToolThoughtSignatureRoundTrip(t *testing.T) {
	for _, stream := range []bool{false, true} {
		for _, splitChunks := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%v/split=%v", stream, splitChunks), func(t *testing.T) {
				parts := []string{
					`{"functionCall":{"name":"generate_image","args":{"prompt":"cat"}},"thoughtSignature":"YWJj+/=="}`,
					`{"functionCall":{"name":"lookup_image","args":{"id":"42"}}}`,
				}
				var message map[string]any
				request := &types.ChatCompletionRequest{Model: "gemini-4-argon"}
				if !stream {
					var candidate GeminiChatCandidate
					mustDecodeSignatureJSON(t, `{"content":{"role":"model","parts":[`+parts[0]+`,`+parts[1]+`]},"finishReason":"STOP"}`, &candidate)
					choice := candidate.ToOpenAIChoice(request)
					encoded, _ := json.Marshal(choice.Message)
					mustDecodeSignatureJSON(t, string(encoded), &message)
				} else {
					h := &GeminiStreamHandler{Request: request, Usage: &types.Usage{}}
					data, errs := make(chan string, 32), make(chan error, 1)
					batches := []string{parts[0] + "," + parts[1]}
					if splitChunks {
						batches = parts
					}
					for _, batch := range batches {
						line := []byte(`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[` + batch + `]}}]}`)
						h.HandlerStream(&line, data, errs)
					}
					line := []byte(`data: {"candidates":[{"index":0,"content":{"role":"model","parts":[]},"finishReason":"STOP"}]}`)
					h.HandlerStream(&line, data, errs)
					if len(errs) > 0 {
						t.Fatal(<-errs)
					}
					close(data)
					calls := []any{}
					finished := false
					for raw := range data {
						var chunk map[string]any
						mustDecodeSignatureJSON(t, raw, &chunk)
						for _, item := range chunk["choices"].([]any) {
							choice := item.(map[string]any)
							if reason := choice["finish_reason"]; reason != nil && reason != "" {
								if reason != "tool_calls" || finished {
									t.Fatalf("unexpected finish: %v", choice)
								}
								finished = true
							}
							delta := choice["delta"].(map[string]any)
							for _, value := range asSignatureSlice(delta["tool_calls"]) {
								if finished {
									t.Fatal("tool call arrived after finish")
								}
								tc := value.(map[string]any)
								index := int(tc["index"].(float64))
								if index == len(calls) {
									calls = append(calls, map[string]any{"function": map[string]any{"arguments": ""}})
								}
								if index >= len(calls) {
									t.Fatalf("nonsequential index: %d", index)
								}
								aggregate := calls[index].(map[string]any)
								for _, key := range []string{"id", "type", "extra_content"} {
									if v, ok := tc[key]; ok {
										aggregate[key] = v
									}
								}
								fn := aggregate["function"].(map[string]any)
								for k, v := range tc["function"].(map[string]any) {
									if k == "arguments" {
										fn[k] = fn[k].(string) + v.(string)
									} else {
										fn[k] = v
									}
								}
							}
						}
					}
					if !finished {
						t.Fatal("missing tool_calls finish")
					}
					message = map[string]any{"role": "assistant", "tool_calls": calls}
				}
				calls := message["tool_calls"].([]any)
				if len(calls) != 2 {
					t.Fatalf("calls: %v", calls)
				}
				first, second := calls[0].(map[string]any), calls[1].(map[string]any)
				if first["id"] == second["id"] {
					t.Fatal("parallel calls share an ID")
				}
				extra, ok := first["extra_content"].(map[string]any)
				if !ok {
					t.Fatal("outbound signature lost")
				}
				if extra["google"].(map[string]any)["thought_signature"] != "YWJj+/==" {
					t.Fatal("signature changed")
				}
				if _, exists := second["extra_content"]; exists {
					t.Fatal("signature copied to unsigned parallel call")
				}
				// Mimic a client storing and returning JSON, with reversed tool results.
				history := []any{map[string]any{"role": "user", "content": "make an image"}, message,
					map[string]any{"role": "tool", "tool_call_id": second["id"], "content": "found"},
					map[string]any{"role": "tool", "tool_call_id": first["id"], "content": "created"}}
				encoded, _ := json.Marshal(history)
				var received []types.ChatCompletionMessage
				mustDecodeSignatureJSON(t, string(encoded), &received)
				converted, _, apiErr := OpenAIToGeminiChatContent(received)
				if apiErr != nil {
					t.Fatal(apiErr)
				}
				if string(converted[1].Parts[0].ThoughtSignature) != `"YWJj+/=="` {
					t.Fatalf("inbound signature lost: %+v", converted[1])
				}
				if len(converted[1].Parts[1].ThoughtSignature) != 0 {
					t.Fatal("unsigned call gained signature")
				}
			})
		}
	}
}

func mustDecodeSignatureJSON(t *testing.T, raw string, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(raw), target); err != nil {
		t.Fatal(err)
	}
}

func asSignatureSlice(v any) []any {
	if v == nil {
		return nil
	}
	return v.([]any)
}

func TestStreamWithoutToolsKeepsExistingWireLayout(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		choices        []string
	}{
		{"empty", `{"candidates":[]}`, []string{`[]`}},
		{"multiple", `{"candidates":[{"index":0,"content":{"parts":[{"text":"a"}]}},{"index":1,"content":{"parts":[{"text":"b"}]}}]}`,
			[]string{`[{"index":0,"delta":{"role":"assistant","content":"a"},"finish_reason":null},{"index":1,"delta":{"role":"assistant","content":"b"},"finish_reason":null}]`}},
		{"length", `{"candidates":[{"index":0,"content":{"parts":[{"text":"partial"}]},"finishReason":"MAX_TOKENS"}]}`,
			[]string{`[{"index":0,"delta":{"role":"assistant","content":"partial"},"finish_reason":"length"}]`}},
		{"stop", `{"candidates":[{"index":0,"content":{"parts":[{"text":"done"}]},"finishReason":"STOP"}]}`,
			[]string{`[{"index":0,"delta":{"role":"assistant","content":"done"},"finish_reason":null}]`, `[{"index":0,"delta":{"role":"assistant"},"finish_reason":"stop"}]`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &GeminiStreamHandler{Request: &types.ChatCompletionRequest{Model: "gemini-4-argon"}, Usage: &types.Usage{}}
			data, errs := make(chan string, 8), make(chan error, 1)
			line := []byte("data: " + tc.response)
			h.HandlerStream(&line, data, errs)
			if len(errs) > 0 {
				t.Fatal(<-errs)
			}
			if len(data) != len(tc.choices) {
				t.Fatalf("chunks=%d, want %d", len(data), len(tc.choices))
			}
			for _, expected := range tc.choices {
				var got map[string]any
				var want any
				mustDecodeSignatureJSON(t, <-data, &got)
				mustDecodeSignatureJSON(t, expected, &want)
				if !reflect.DeepEqual(got["choices"], want) {
					t.Fatalf("wire layout changed: %+v, want %+v", got["choices"], want)
				}
			}
		})
	}
}
