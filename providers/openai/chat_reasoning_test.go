package openai

import (
	"done-hub/types"
	"encoding/json"
	"testing"
)

func TestOtherProcessingNormalizesReasoningForNonGPTCompatibleModel(t *testing.T) {
	request := &types.ChatCompletionRequest{
		Model: "deepseek-v4-flash",
		Reasoning: &types.ChatReasoning{
			Effort: "high",
		},
	}

	otherProcessing(request, "")

	if request.Reasoning != nil {
		t.Fatalf("内部 reasoning 不应序列化给 Chat 上游: %#v", request.Reasoning)
	}
	if request.ReasoningEffort == nil || *request.ReasoningEffort != "high" {
		t.Fatalf("reasoning_effort 未标准化: %#v", request.ReasoningEffort)
	}
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("解析请求失败: %v", err)
	}
	if payload["reasoning_effort"] != "high" {
		t.Fatalf("上游请求缺少 reasoning_effort: %s", body)
	}
	if _, exists := payload["reasoning"]; exists {
		t.Fatalf("上游请求不应包含嵌套 reasoning: %s", body)
	}
}

func TestOtherProcessingKeepsExplicitReasoningEffort(t *testing.T) {
	explicit := "medium"
	request := &types.ChatCompletionRequest{
		Model:           "compatible-model",
		ReasoningEffort: &explicit,
		Reasoning:       &types.ChatReasoning{Effort: "high"},
	}

	otherProcessing(request, "low")

	if request.ReasoningEffort == nil || *request.ReasoningEffort != explicit {
		t.Fatalf("显式 reasoning_effort 被覆盖: %#v", request.ReasoningEffort)
	}
}
