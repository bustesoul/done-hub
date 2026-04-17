package types

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

func ShouldDebugResponsesTools() bool {
	return os.Getenv("DONE_API_DEBUG_RESPONSES_TOOLS") != ""
}

func SummarizeResponsesRequestToolsJSON(body []byte) string {
	var payload struct {
		Tools []map[string]any `json:"tools"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Sprintf("tools_summary=parse_error:%v", err)
	}

	return summarizeRawResponseTools(payload.Tools)
}

func SummarizeResponsesRequestToolsAny(payload any) string {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Sprintf("tools_summary=marshal_error:%v", err)
	}
	return SummarizeResponsesRequestToolsJSON(body)
}

func SummarizeResponsesTools(tools []ResponsesTools) string {
	if len(tools) == 0 {
		return "tools=0"
	}

	parts := make([]string, 0, len(tools))
	for index, tool := range tools {
		extraKeys := make([]string, 0, len(tool.ExtraFields))
		for key := range tool.ExtraFields {
			extraKeys = append(extraKeys, key)
		}
		sort.Strings(extraKeys)

		part := fmt.Sprintf("#%d(type=%s,name=%s,nested_tools=%d", index, tool.Type, tool.Name, len(tool.Tools))
		if len(extraKeys) > 0 {
			part += fmt.Sprintf(",extra=%s", strings.Join(extraKeys, "|"))
		}
		part += ")"
		parts = append(parts, part)
	}

	return fmt.Sprintf("tools=%d %s", len(tools), strings.Join(parts, "; "))
}

func HasNamespaceToolsInRequestJSON(body []byte) bool {
	var payload struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return false
	}

	for _, tool := range payload.Tools {
		if toolType, _ := tool["type"].(string); toolType == "namespace" {
			return true
		}
	}

	return false
}

func HasNamespaceToolsWithoutNested(tools []ResponsesTools) bool {
	for _, tool := range tools {
		if tool.Type == "namespace" && len(tool.Tools) == 0 {
			return true
		}
	}

	return false
}

func summarizeRawResponseTools(tools []map[string]any) string {
	if len(tools) == 0 {
		return "tools=0"
	}

	parts := make([]string, 0, len(tools))
	for index, tool := range tools {
		toolType, _ := tool["type"].(string)
		name, _ := tool["name"].(string)
		nestedCount := 0
		if nested, ok := tool["tools"].([]any); ok {
			nestedCount = len(nested)
		}

		keys := make([]string, 0, len(tool))
		for key := range tool {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		parts = append(parts, fmt.Sprintf("#%d(type=%s,name=%s,nested_tools=%d,keys=%s)", index, toolType, name, nestedCount, strings.Join(keys, "|")))
	}

	return fmt.Sprintf("tools=%d %s", len(tools), strings.Join(parts, "; "))
}
