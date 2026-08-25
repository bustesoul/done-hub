package relay

import (
	"done-hub/common"
	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type reasoningEffortSnapshot struct {
	Value  string
	Source string
}

// captureOriginalReasoningEffort must run before pre-mapping and provider
// conversion. Values are deliberately not normalized so logs reflect exactly
// what the client sent (for example minimal remains minimal and MINIMAL remains
// MINIMAL).
func captureOriginalReasoningEffort(c *gin.Context) {
	rawBody, err := common.ReadBodyRaw(c)
	if err != nil {
		return
	}

	snapshot := extractOriginalReasoningEffort(inboundProtocol(c), rawBody)
	if snapshot.Value == "" {
		return
	}
	c.Set(config.GinReasoningEffortKey, snapshot.Value)
	c.Set(config.GinReasoningEffortSourceKey, snapshot.Source)
}

func extractOriginalReasoningEffort(protocol domain.Protocol, rawBody []byte) reasoningEffortSnapshot {
	var paths []string
	switch protocol {
	case domain.ProtocolOpenAIChat:
		// Matches the Chat provider's precedence: the standard top-level field
		// wins over the compatible nested field when both are present.
		paths = []string{"reasoning_effort", "reasoning.effort"}
	case domain.ProtocolOpenAIResponses:
		paths = []string{"reasoning.effort"}
	case domain.ProtocolClaudeMessages:
		paths = []string{"output_config.effort"}
	case domain.ProtocolGemini:
		paths = []string{"generationConfig.thinkingConfig.thinkingLevel"}
	default:
		return reasoningEffortSnapshot{}
	}

	for _, path := range paths {
		value := gjson.GetBytes(rawBody, path)
		if value.Type == gjson.String && value.String() != "" {
			return reasoningEffortSnapshot{Value: value.String(), Source: path}
		}
	}

	// done-hub supports model#low style reasoning effort for Chat requests.
	// search and thinking are separate feature switches, not effort levels.
	if protocol == domain.ProtocolOpenAIChat {
		model := gjson.GetBytes(rawBody, "model")
		if model.Type == gjson.String {
			parts := strings.Split(model.String(), "#")
			if len(parts) > 1 && parts[1] != "" && parts[1] != "search" && parts[1] != "thinking" {
				return reasoningEffortSnapshot{Value: parts[1], Source: "model_suffix"}
			}
		}
	}

	return reasoningEffortSnapshot{}
}
