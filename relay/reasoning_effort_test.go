package relay

import (
	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/requeststate"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestExtractOriginalReasoningEffort(t *testing.T) {
	tests := []struct {
		name     string
		protocol domain.Protocol
		body     string
		want     reasoningEffortSnapshot
	}{
		{
			name:     "chat top-level minimal is preserved",
			protocol: domain.ProtocolOpenAIChat,
			body:     `{"model":"gpt-5","reasoning_effort":"minimal"}`,
			want:     reasoningEffortSnapshot{Value: "minimal", Source: "reasoning_effort"},
		},
		{
			name:     "chat top-level wins over nested",
			protocol: domain.ProtocolOpenAIChat,
			body:     `{"model":"gpt-5","reasoning_effort":"low","reasoning":{"effort":"high"}}`,
			want:     reasoningEffortSnapshot{Value: "low", Source: "reasoning_effort"},
		},
		{
			name:     "responses nested custom value is preserved",
			protocol: domain.ProtocolOpenAIResponses,
			body:     `{"model":"gpt-5","reasoning":{"effort":"xhigh"}}`,
			want:     reasoningEffortSnapshot{Value: "xhigh", Source: "reasoning.effort"},
		},
		{
			name:     "claude output config",
			protocol: domain.ProtocolClaudeMessages,
			body:     `{"model":"claude-opus","output_config":{"effort":"high"}}`,
			want:     reasoningEffortSnapshot{Value: "high", Source: "output_config.effort"},
		},
		{
			name:     "gemini case is preserved",
			protocol: domain.ProtocolGemini,
			body:     `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"MINIMAL"}}}`,
			want:     reasoningEffortSnapshot{Value: "MINIMAL", Source: "generationConfig.thinkingConfig.thinkingLevel"},
		},
		{
			name:     "chat model suffix fallback",
			protocol: domain.ProtocolOpenAIChat,
			body:     `{"model":"o3-mini#low"}`,
			want:     reasoningEffortSnapshot{Value: "low", Source: "model_suffix"},
		},
		{
			name:     "thinking suffix is not effort",
			protocol: domain.ProtocolOpenAIChat,
			body:     `{"model":"claude-3-7-sonnet#thinking"}`,
			want:     reasoningEffortSnapshot{},
		},
		{
			name:     "unspecified effort",
			protocol: domain.ProtocolOpenAIResponses,
			body:     `{"model":"gpt-5"}`,
			want:     reasoningEffortSnapshot{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := extractOriginalReasoningEffort(test.protocol, []byte(test.body))
			if got != test.want {
				t.Fatalf("extractOriginalReasoningEffort() = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestCaptureOriginalReasoningEffortStoresRequestSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	context.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/responses",
		strings.NewReader(`{"model":"gpt-5","reasoning":{"effort":"minimal"}}`),
	)
	request, state := requeststate.Ensure(context.Request)
	context.Request = request
	state.SetInboundProtocol(domain.ProtocolOpenAIResponses)

	captureOriginalReasoningEffort(context)

	if got := context.GetString(config.GinReasoningEffortKey); got != "minimal" {
		t.Fatalf("captured effort = %q, want minimal", got)
	}
	if got := context.GetString(config.GinReasoningEffortSourceKey); got != "reasoning.effort" {
		t.Fatalf("captured source = %q, want reasoning.effort", got)
	}
	if cached, exists := context.Get(config.GinRequestBodyKey); !exists || string(cached.([]byte)) == "" {
		t.Fatal("original request body was not cached for downstream parsing")
	}
}
