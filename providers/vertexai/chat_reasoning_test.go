package vertexai

import (
	"encoding/json"
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
	"done-hub/providers/gemini"
	"done-hub/types"
	"go.uber.org/zap"
)

// Exercise Send, including serialization, Vertex cleanup and custom parameters.
// Decode the wire body independently instead of asserting converter structs.
func TestVertexReasoningLevelOnWire(t *testing.T) {
	oldClient, oldLogger := requester.HTTPClient, logger.Logger
	logger.Logger = zap.NewNop()
	cache.InitCacheManager()
	t.Cleanup(func() { requester.HTTPClient, logger.Logger = oldClient, oldLogger })
	for _, tc := range []struct {
		name, fields, want string
		history            bool
	}{
		{"high", `"reasoning_effort":"high"`, "HIGH", false},
		{"medium", `"reasoning_effort":"medium"`, "MEDIUM", false},
		{"low", `"reasoning_effort":"low"`, "LOW", false},
		{"top_level_wins", `"reasoning_effort":"medium","reasoning":{"effort":"high","max_tokens":1024}`, "MEDIUM", false},
		{"empty_top_level_falls_back", `"reasoning_effort":"","reasoning":{"effort":"low"}`, "LOW", false},
		{"nested_compatibility", `"reasoning":{"effort":"high"}`, "HIGH", false},
		{"text_block_history", `"reasoning_effort":"high"`, "HIGH", true},
		{"unspecified", `"temperature":1`, "", false},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(tc.name+map[bool]string{false: "/nonstream", true: "/stream"}[stream], func(t *testing.T) {
				received := make(chan struct{}, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					received <- struct{}{}
					action := "generateContent"
					if stream {
						action = "streamGenerateContent"
					}
					if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, ":"+action) {
						t.Errorf("wrong target: %s %s", r.Method, r.URL.Path)
					}
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					generation, _ := body["generationConfig"].(map[string]any)
					thinking, _ := generation["thinkingConfig"].(map[string]any)
					if tc.want == "" {
						if len(thinking) != 0 {
							t.Errorf("unspecified effort changed default: %#v", thinking)
						}
					} else {
						if thinking["thinkingLevel"] != tc.want {
							t.Errorf("wire thinkingLevel=%v, want %s", thinking["thinkingLevel"], tc.want)
						}
						if _, exists := thinking["thinkingBudget"]; exists {
							t.Errorf("level must not also send budget: %#v", thinking)
						}
					}
					if _, ok := body["reasoning_effort"]; ok {
						t.Error("OpenAI field leaked to Vertex")
					}
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"candidates":[{"content":{"role":"model","parts":[{"text":"OK"}]},"finishReason":"STOP"}]}`)
				}))
				defer server.Close()
				requester.HTTPClient = server.Client()
				channel := &model.Channel{Id: 987654, Type: 42, Other: "global|test-project", Key: "test-credential"}
				if err := cache.SetCache(channel.CredentialCacheKey(TokenCacheKey), "test-token", time.Minute); err != nil {
					t.Fatal(err)
				}
				p := (VertexAIProviderFactory{}).CreateAdapter(channel).(*VertexAIProvider)
				p.Config.BaseURL = server.URL + "/%sprojects/%s/locations/%s/models/%s:%s"
				messages := `[{"role":"user","content":"Reply OK"}]`
				if tc.history {
					messages = `[{"role":"user","content":"Hello"},{"role":"assistant","content":[{"type":"text","text":"Hello"}]},{"role":"user","content":"Reply OK"}]`
				}
				var req types.ChatCompletionRequest
				if err := json.Unmarshal([]byte(`{"model":"gemini-4-argon","messages":`+messages+`,`+tc.fields+`}`), &req); err != nil {
					t.Fatal(err)
				}
				req.Stream = stream
				resp, apiErr := p.Send(&req)
				if apiErr != nil {
					t.Fatalf("send: %+v", apiErr)
				}
				resp.Body.Close()
				select {
				case <-received:
				default:
					t.Fatal("upstream not reached")
				}
			})
		}
	}
}

func TestVertexReasoningBudgetCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		budget, maxOutput, want int
	}{
		{"explicit_zero", 0, 256, 0},
		{"positive_budget", 128, 256, 128},
		{"clamped_budget", 512, 256, 255},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.ChatCompletionRequest{Model: "gemini-2.5-flash", MaxTokens: tc.maxOutput, Reasoning: &types.ChatReasoning{MaxTokens: tc.budget}}
			converted, apiErr := gemini.ConvertFromChatOpenai(req)
			if apiErr != nil {
				t.Fatalf("convert: %+v", apiErr)
			}
			thinking := converted.GenerationConfig.ThinkingConfig
			if thinking == nil || thinking.ThinkingBudget == nil || *thinking.ThinkingBudget != tc.want || thinking.ThinkingLevel != "" {
				t.Fatalf("budget behavior changed: %+v", thinking)
			}
		})
	}
}
