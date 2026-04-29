package types

import (
	"done-hub/common/config"
	"encoding/json"
	"testing"
)

// TestResponsesUsageInputTokensDetails_ZeroValueSerialization 测试零值字段的正确序列化
func TestResponsesUsageInputTokensDetails_ZeroValueSerialization(t *testing.T) {
	tests := []struct {
		name     string
		input    ResponsesUsageInputTokensDetails
		expected string
	}{
		{
			name: "所有字段为零值",
			input: ResponsesUsageInputTokensDetails{
				CachedTokens: 0,
				TextTokens:   0,
				ImageTokens:  0,
			},
			expected: `{"cached_tokens":0}`, // text_tokens 和 image_tokens 有 omitempty，零值不输出
		},
		{
			name: "仅 cached_tokens 为零",
			input: ResponsesUsageInputTokensDetails{
				CachedTokens: 0,
				TextTokens:   100,
				ImageTokens:  50,
			},
			expected: `{"cached_tokens":0,"text_tokens":100,"image_tokens":50}`,
		},
		{
			name: "所有字段为非零值",
			input: ResponsesUsageInputTokensDetails{
				CachedTokens: 10,
				TextTokens:   200,
				ImageTokens:  30,
			},
			expected: `{"cached_tokens":10,"text_tokens":200,"image_tokens":30}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := json.Marshal(tt.input)
			if err != nil {
				t.Fatalf("序列化失败: %v", err)
			}
			if string(result) != tt.expected {
				t.Errorf("序列化结果不匹配\n期望: %s\n实际: %s", tt.expected, string(result))
			}
		})
	}
}

// TestResponsesUsage_FullSerialization 测试完整 ResponsesUsage 结构体的序列化
func TestResponsesUsage_FullSerialization(t *testing.T) {
	usage := &ResponsesUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		OutputTokensDetails: &ResponsesUsageOutputTokensDetails{
			ReasoningTokens: 20,
		},
		InputTokensDetails: &ResponsesUsageInputTokensDetails{
			CachedTokens: 0,
			TextTokens:   100,
			ImageTokens:  0,
		},
	}

	result, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	// 反序列化验证零值字段存在
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}

	inputDetails, ok := parsed["input_tokens_details"].(map[string]interface{})
	if !ok {
		t.Fatal("input_tokens_details 不存在或类型错误")
	}

	// 验证 cached_tokens 字段存在且值为 0
	if cachedTokens, exists := inputDetails["cached_tokens"]; !exists {
		t.Error("cached_tokens 字段应该存在（即使值为0）")
	} else if cachedTokens != float64(0) {
		t.Errorf("cached_tokens 应为 0，实际为 %v", cachedTokens)
	}

	// text_tokens 和 image_tokens 有 omitempty，零值时不存在是正常的
}

// TestToOpenAIUsage 测试 ResponsesUsage 转换为 Usage
func TestToOpenAIUsage(t *testing.T) {
	responsesUsage := &ResponsesUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		OutputTokensDetails: &ResponsesUsageOutputTokensDetails{
			ReasoningTokens: 20,
		},
		InputTokensDetails: &ResponsesUsageInputTokensDetails{
			CachedTokens: 0,
			TextTokens:   80,
			ImageTokens:  20,
		},
	}

	usage := responsesUsage.ToOpenAIUsage()

	if usage.PromptTokens != 100 {
		t.Errorf("PromptTokens 应为 100，实际为 %d", usage.PromptTokens)
	}
	if usage.CompletionTokens != 50 {
		t.Errorf("CompletionTokens 应为 50，实际为 %d", usage.CompletionTokens)
	}
	if usage.CompletionTokensDetails.ReasoningTokens != 20 {
		t.Errorf("ReasoningTokens 应为 20，实际为 %d", usage.CompletionTokensDetails.ReasoningTokens)
	}
	if usage.PromptTokensDetails.CachedTokens != 0 {
		t.Errorf("CachedTokens 应为 0，实际为 %d", usage.PromptTokensDetails.CachedTokens)
	}
	if usage.PromptTokensDetails.TextTokens != 80 {
		t.Errorf("TextTokens 应为 80，实际为 %d", usage.PromptTokensDetails.TextTokens)
	}
}

func TestResponsesRequest_PromptCacheFieldsRoundTrip(t *testing.T) {
	body := []byte(`{
		"model":"gpt-5.5",
		"prompt_cache_key":"session-a",
		"prompt_cache_retention":"24h",
		"safety_identifier":"user-hash",
		"service_tier":"auto",
		"user":"legacy-user",
		"metadata":{"trace":"abc"},
		"input":"hello"
	}`)

	var request OpenAIResponsesRequest
	if err := json.Unmarshal(body, &request); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if request.PromptCacheKey != "session-a" {
		t.Fatalf("PromptCacheKey 丢失: %q", request.PromptCacheKey)
	}
	if request.PromptCacheRetention != "24h" {
		t.Fatalf("PromptCacheRetention 丢失: %q", request.PromptCacheRetention)
	}
	if request.SafetyIdentifier != "user-hash" {
		t.Fatalf("SafetyIdentifier 丢失: %q", request.SafetyIdentifier)
	}
	if request.ServiceTier != "auto" {
		t.Fatalf("ServiceTier 丢失: %q", request.ServiceTier)
	}
	if request.User != "legacy-user" {
		t.Fatalf("User 丢失: %q", request.User)
	}
	if request.Metadata["trace"] != "abc" {
		t.Fatalf("Metadata 丢失: %#v", request.Metadata)
	}

	encoded, err := json.Marshal(&request)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	var output map[string]any
	if err := json.Unmarshal(encoded, &output); err != nil {
		t.Fatalf("输出反序列化失败: %v", err)
	}
	for _, key := range []string{"prompt_cache_key", "prompt_cache_retention", "safety_identifier", "service_tier", "user", "metadata"} {
		if _, ok := output[key]; !ok {
			t.Fatalf("输出缺少字段 %s: %s", key, string(encoded))
		}
	}
}

func TestChatCompletionToResponsesRequest_PreservesCacheFields(t *testing.T) {
	instructions := "stable instructions"
	store := false
	chat := &ChatCompletionRequest{
		Model:                "gpt-5.5",
		Messages:             []ChatCompletionMessage{{Role: ChatMessageRoleUser, Content: "hello"}},
		Instructions:         &instructions,
		Metadata:             map[string]any{"trace": "abc"},
		PromptCacheKey:       "cache-key",
		PromptCacheRetention: "24h",
		SafetyIdentifier:     "safe-user",
		ServiceTier:          "priority",
		Store:                &store,
		User:                 "legacy-user",
	}

	responses := chat.ToResponsesRequest()
	if responses.Instructions != instructions {
		t.Fatalf("Instructions 未保留: %q", responses.Instructions)
	}
	if responses.PromptCacheKey != "cache-key" {
		t.Fatalf("PromptCacheKey 未保留: %q", responses.PromptCacheKey)
	}
	if responses.PromptCacheRetention != "24h" {
		t.Fatalf("PromptCacheRetention 未保留: %q", responses.PromptCacheRetention)
	}
	if responses.SafetyIdentifier != "safe-user" {
		t.Fatalf("SafetyIdentifier 未保留: %q", responses.SafetyIdentifier)
	}
	if responses.ServiceTier != "priority" {
		t.Fatalf("ServiceTier 未保留: %q", responses.ServiceTier)
	}
	if responses.Store == nil || *responses.Store {
		t.Fatalf("Store 未保留: %#v", responses.Store)
	}
	if responses.User != "legacy-user" {
		t.Fatalf("User 未保留: %q", responses.User)
	}
	if responses.Metadata["trace"] != "abc" {
		t.Fatalf("Metadata 未保留: %#v", responses.Metadata)
	}
}

func TestResponsesToChatCompletionRequest_PreservesCacheFields(t *testing.T) {
	store := false
	request := &OpenAIResponsesRequest{
		Model:                "gpt-5.5",
		Input:                "hello",
		Instructions:         "stable instructions",
		Metadata:             map[string]any{"trace": "abc"},
		PromptCacheKey:       "cache-key",
		PromptCacheRetention: "24h",
		SafetyIdentifier:     "safe-user",
		ServiceTier:          "priority",
		Store:                &store,
		User:                 "legacy-user",
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if chat.Instructions == nil || *chat.Instructions != "stable instructions" {
		t.Fatalf("Instructions 未保留: %#v", chat.Instructions)
	}
	if chat.PromptCacheKey != "cache-key" {
		t.Fatalf("PromptCacheKey 未保留: %q", chat.PromptCacheKey)
	}
	if chat.PromptCacheRetention != "24h" {
		t.Fatalf("PromptCacheRetention 未保留: %q", chat.PromptCacheRetention)
	}
	if chat.SafetyIdentifier != "safe-user" {
		t.Fatalf("SafetyIdentifier 未保留: %q", chat.SafetyIdentifier)
	}
	if chat.ServiceTier != "priority" {
		t.Fatalf("ServiceTier 未保留: %q", chat.ServiceTier)
	}
	if chat.Store == nil || *chat.Store {
		t.Fatalf("Store 未保留: %#v", chat.Store)
	}
	if chat.User != "legacy-user" {
		t.Fatalf("User 未保留: %q", chat.User)
	}
	if chat.Metadata["trace"] != "abc" {
		t.Fatalf("Metadata 未保留: %#v", chat.Metadata)
	}
}

func TestUsageGetExtraTokens_PreservesOpenAICachedTokens(t *testing.T) {
	usage := &Usage{
		PromptTokensDetails: PromptTokensDetails{
			CachedTokens: 123,
		},
	}

	extra := usage.GetExtraTokens()
	if extra[config.UsageExtraCache] != 123 {
		t.Fatalf("cached_tokens 应保留为标准 OpenAI 字段，实际 extra=%#v", extra)
	}
}

func TestUsageGetExtraTokens_DoesNotDoubleCountCachedTokens(t *testing.T) {
	usage := &Usage{
		PromptTokensDetails: PromptTokensDetails{
			CachedTokens: 123,
		},
		CacheReadInputTokens: 123,
	}

	extra := usage.GetExtraTokens()
	if extra[config.UsageExtraCache] != 123 {
		t.Fatalf("cached_tokens 应保留为标准 OpenAI 字段，实际 extra=%#v", extra)
	}
	if extra[config.UsageExtraCachedRead] != 0 {
		t.Fatalf("cache_read_input_tokens 不应与 cached_tokens 重复计费，实际 extra=%#v", extra)
	}
}

// TestToResponsesUsage 测试 Usage 转换为 ResponsesUsage
func TestToResponsesUsage(t *testing.T) {
	usage := &Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		PromptTokensDetails: PromptTokensDetails{
			CachedTokens: 0,
			TextTokens:   80,
			ImageTokens:  20,
		},
		CompletionTokensDetails: CompletionTokensDetails{
			ReasoningTokens: 30,
		},
	}

	responsesUsage := usage.ToResponsesUsage()

	if responsesUsage.InputTokens != 100 {
		t.Errorf("InputTokens 应为 100，实际为 %d", responsesUsage.InputTokens)
	}
	if responsesUsage.OutputTokens != 50 {
		t.Errorf("OutputTokens 应为 50，实际为 %d", responsesUsage.OutputTokens)
	}
	if responsesUsage.OutputTokensDetails == nil {
		t.Error("OutputTokensDetails 不应为 nil（当 ReasoningTokens > 0 时）")
	} else if responsesUsage.OutputTokensDetails.ReasoningTokens != 30 {
		t.Errorf("ReasoningTokens 应为 30，实际为 %d", responsesUsage.OutputTokensDetails.ReasoningTokens)
	}
	if responsesUsage.InputTokensDetails == nil {
		t.Error("InputTokensDetails 不应为 nil")
	} else {
		if responsesUsage.InputTokensDetails.CachedTokens != 0 {
			t.Errorf("CachedTokens 应为 0，实际为 %d", responsesUsage.InputTokensDetails.CachedTokens)
		}
	}
}

// TestBidirectionalConversion 测试双向转换的一致性
func TestBidirectionalConversion(t *testing.T) {
	original := &ResponsesUsage{
		InputTokens:  100,
		OutputTokens: 50,
		TotalTokens:  150,
		OutputTokensDetails: &ResponsesUsageOutputTokensDetails{
			ReasoningTokens: 20,
		},
		InputTokensDetails: &ResponsesUsageInputTokensDetails{
			CachedTokens: 0,
			TextTokens:   80,
			ImageTokens:  20,
		},
	}

	// ResponsesUsage -> Usage -> ResponsesUsage
	usage := original.ToOpenAIUsage()
	converted := usage.ToResponsesUsage()

	if converted.InputTokens != original.InputTokens {
		t.Errorf("双向转换后 InputTokens 不一致: 期望 %d, 实际 %d", original.InputTokens, converted.InputTokens)
	}
	if converted.OutputTokens != original.OutputTokens {
		t.Errorf("双向转换后 OutputTokens 不一致: 期望 %d, 实际 %d", original.OutputTokens, converted.OutputTokens)
	}
	if converted.InputTokensDetails.CachedTokens != original.InputTokensDetails.CachedTokens {
		t.Errorf("双向转换后 CachedTokens 不一致: 期望 %d, 实际 %d",
			original.InputTokensDetails.CachedTokens, converted.InputTokensDetails.CachedTokens)
	}
}

// TestNilInputTokensDetails 测试 nil 值处理
func TestNilInputTokensDetails(t *testing.T) {
	responsesUsage := &ResponsesUsage{
		InputTokens:        100,
		OutputTokens:       50,
		TotalTokens:        150,
		InputTokensDetails: nil,
	}

	usage := responsesUsage.ToOpenAIUsage()

	// 当 InputTokensDetails 为 nil 时，PromptTokensDetails 应保持零值
	if usage.PromptTokensDetails.CachedTokens != 0 {
		t.Errorf("CachedTokens 应为 0，实际为 %d", usage.PromptTokensDetails.CachedTokens)
	}
}

// TestZeroReasoningTokens 测试 ReasoningTokens 为 0 时 OutputTokensDetails 为 nil
func TestZeroReasoningTokens(t *testing.T) {
	usage := &Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		CompletionTokensDetails: CompletionTokensDetails{
			ReasoningTokens: 0,
		},
	}

	responsesUsage := usage.ToResponsesUsage()

	// 当 ReasoningTokens 为 0 时，OutputTokensDetails 应为 nil
	if responsesUsage.OutputTokensDetails != nil {
		t.Error("当 ReasoningTokens 为 0 时，OutputTokensDetails 应为 nil")
	}
}
