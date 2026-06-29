package types

import (
	"done-hub/common/config"
	"encoding/json"
	"strings"
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

func TestBuiltinNeed2ResponseModels_IncludesMimoModels(t *testing.T) {
	modelSet := config.BuildNeed2ResponseModelSet(nil)
	for _, model := range []string{
		"mimo-v2.5-pro",
		"mimo-v2-pro",
		"mimo-v2.5",
		"mimo-v2-omni",
		"mimo-v2-flash",
	} {
		if _, ok := modelSet[model]; !ok {
			t.Fatalf("内置 Chat 转 Responses 模型缺少 %s", model)
		}
	}

	if config.IsBuiltinNeed2ResponseModel("gpt-5.5") {
		t.Fatal("gpt-5.5 不应进入 MiMo 专用内置 Chat 转 Responses 模型集合")
	}
}

func TestChatCompletionToResponses_PreservesReasoningEncryptedContent(t *testing.T) {
	reasoning := "full mimo reasoning trace"
	response := &ChatCompletionResponse{
		ID:      "chatcmpl-test",
		Model:   "mimo-v2.5-pro",
		Created: 123,
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
		Choices: []ChatCompletionChoice{
			{
				FinishReason: FinishReasonToolCalls,
				Message: ChatCompletionMessage{
					Role:             ChatMessageRoleAssistant,
					ReasoningContent: reasoning,
					ToolCalls: []*ChatCompletionToolCalls{
						{
							Id:   "call_1",
							Type: "function",
							Function: &ChatCompletionToolCallsFunction{
								Name:      "read_file",
								Arguments: `{"path":"README.md"}`,
							},
						},
					},
				},
			},
		},
	}

	responses := response.ToResponses(&OpenAIResponsesRequest{Model: "mimo-v2.5-pro"})
	if len(responses.Output) != 2 {
		t.Fatalf("应输出 reasoning 和 function_call 两项，实际 %#v", responses.Output)
	}
	reasoningOutput := responses.Output[0]
	if reasoningOutput.Type != InputTypeReasoning {
		t.Fatalf("第一项应为 reasoning，实际 %s", reasoningOutput.Type)
	}
	if reasoningOutput.EncryptedContent == nil || *reasoningOutput.EncryptedContent != reasoning {
		t.Fatalf("reasoning encrypted_content 未保留: %#v", reasoningOutput.EncryptedContent)
	}
	if reasoningOutput.GetSummaryString() != reasoning {
		t.Fatalf("reasoning 摘要读取应优先返回完整 encrypted_content")
	}
	if responses.Output[1].Type != InputTypeFunctionCall {
		t.Fatalf("第二项应为 function_call，实际 %s", responses.Output[1].Type)
	}
}

func TestResponsesToChatCompletionRequest_RestoresReasoningContentForToolCall(t *testing.T) {
	reasoning := "full mimo reasoning trace"
	request := &OpenAIResponsesRequest{
		Model: "mimo-v2.5-pro",
		Input: []InputResponses{
			{
				Type:             InputTypeReasoning,
				EncryptedContent: &reasoning,
				Summary: []SummaryResponses{
					{
						Type: ContentTypeSummaryText,
						Text: "visible summary",
					},
				},
			},
			{
				Type:      InputTypeFunctionCall,
				CallID:    "call_1",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"README.md"}`),
			},
			{
				Type:   InputTypeFunctionCallOutput,
				CallID: "call_1",
				Output: "ok",
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(chat.Messages) != 2 {
		t.Fatalf("应转换出 assistant tool call 和 tool output 两条消息，实际 %#v", chat.Messages)
	}
	assistant := chat.Messages[0]
	if assistant.Role != ChatMessageRoleAssistant {
		t.Fatalf("第一条应为 assistant，实际 %s", assistant.Role)
	}
	if assistant.ReasoningContent != reasoning {
		t.Fatalf("未恢复完整 reasoning_content: %q", assistant.ReasoningContent)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Function.Name != "read_file" {
		t.Fatalf("tool_calls 未保留: %#v", assistant.ToolCalls)
	}
}

func TestResponsesToChatCompletionRequest_MergesAssistantMessageReasoningAndToolCall(t *testing.T) {
	reasoning := "reasoning belongs to the tool-call assistant message"
	request := &OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []InputResponses{
			{
				Type: InputTypeMessage,
				Role: ChatMessageRoleAssistant,
				Content: []ContentResponses{
					{
						Type: ContentTypeOutputText,
						Text: "I will inspect the file.",
					},
				},
			},
			{
				Type:             InputTypeReasoning,
				EncryptedContent: &reasoning,
			},
			{
				Type:      InputTypeFunctionCall,
				CallID:    "call_1",
				Name:      "read_file",
				Arguments: json.RawMessage(`{"path":"README.md"}`),
			},
			{
				Type:   InputTypeFunctionCallOutput,
				CallID: "call_1",
				Output: "ok",
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(chat.Messages) != 2 {
		t.Fatalf("assistant message、tool call 和 tool output 不应被拆错，实际 %#v", chat.Messages)
	}

	assistant := chat.Messages[0]
	if assistant.Role != ChatMessageRoleAssistant {
		t.Fatalf("第一条应为 assistant，实际 %s", assistant.Role)
	}
	if assistant.ReasoningContent != reasoning {
		t.Fatalf("reasoning_content 未合并到 tool-call assistant: %q", assistant.ReasoningContent)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Id != "call_1" {
		t.Fatalf("tool_calls 未合并到 assistant: %#v", assistant.ToolCalls)
	}
	parts, ok := assistant.Content.([]ChatMessagePart)
	if !ok || len(parts) != 1 || parts[0].Text != "I will inspect the file." {
		t.Fatalf("assistant 正文未保留: %#v", assistant.Content)
	}
}

func TestResponsesToChatCompletionRequest_SanitizesHistoricalToolCallArguments(t *testing.T) {
	request := &OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []InputResponses{
			{
				Type:      InputTypeFunctionCall,
				CallID:    "call_broken",
				Name:      "shell",
				Arguments: json.RawMessage(`{"command":["bash","-lc","echo a"],"workdir":`),
			},
			{
				Type:   InputTypeFunctionCallOutput,
				CallID: "call_broken",
				Output: []ContentResponses{
					{
						Type: ContentTypeOutputText,
						Text: "done",
					},
					{
						Type:     ContentTypeInputImage,
						ImageUrl: "data:image/png;base64,aaa",
					},
				},
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(chat.Messages) != 2 {
		t.Fatalf("应转换出 assistant tool call 和 tool output 两条消息，实际 %#v", chat.Messages)
	}

	assistant := chat.Messages[0]
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("tool_calls 未保留: %#v", assistant.ToolCalls)
	}
	if assistant.ToolCalls[0].Function.Arguments != "{}" {
		t.Fatalf("非 JSON arguments 应被修复为空对象，实际 %q", assistant.ToolCalls[0].Function.Arguments)
	}
	if assistant.Content != "" {
		t.Fatalf("tool-call assistant 不应发送 content:null，实际 %#v", assistant.Content)
	}

	toolOutput, ok := chat.Messages[1].Content.(string)
	if !ok {
		t.Fatalf("tool output 应文本化为 string，实际 %#v", chat.Messages[1].Content)
	}
	if toolOutput != "done[omitted 1 image attachment(s) from tool output]" {
		t.Fatalf("tool output 文本化结果不符合预期: %q", toolOutput)
	}
}

func TestResponsesToChatCompletionRequest_MimoDropsOrphanToolOutputs(t *testing.T) {
	request := &OpenAIResponsesRequest{
		Model: "mimo-v2.5-pro",
		Input: []InputResponses{
			{
				Type:   InputTypeFunctionCallOutput,
				CallID: "call_orphan",
				Output: "orphan output",
			},
			{
				Type:      InputTypeFunctionCall,
				CallID:    "call_valid",
				Name:      "shell",
				Arguments: json.RawMessage(`{"command":["pwd"]}`),
			},
			{
				Type:   InputTypeFunctionCallOutput,
				CallID: "call_valid",
				Output: "ok",
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(chat.Messages) != 2 {
		t.Fatalf("应只保留有效 tool output，实际 %#v", chat.Messages)
	}
	if chat.Messages[0].Role != ChatMessageRoleAssistant || len(chat.Messages[0].ToolCalls) != 1 {
		t.Fatalf("第一条应为 assistant tool call，实际 %#v", chat.Messages[0])
	}
	if chat.Messages[1].Role != "tool" || chat.Messages[1].ToolCallID != "call_valid" {
		t.Fatalf("第二条应为有效 tool output，实际 %#v", chat.Messages[1])
	}
}

func TestResponsesToChatCompletionRequest_MimoSynthesizesMissingToolOutputs(t *testing.T) {
	request := &OpenAIResponsesRequest{
		Model: "mimo-v2.5-pro",
		Input: []InputResponses{
			{
				Type:      InputTypeFunctionCall,
				CallID:    "call_missing",
				Name:      "shell",
				Arguments: json.RawMessage(`{"command":["pwd"]}`),
			},
			{
				Type: InputTypeMessage,
				Role: "user",
				Content: []ContentResponses{
					{
						Type: ContentTypeInputText,
						Text: "continue",
					},
				},
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(chat.Messages) != 3 {
		t.Fatalf("应补齐缺失 tool output，实际 %#v", chat.Messages)
	}
	if chat.Messages[1].Role != "tool" || chat.Messages[1].ToolCallID != "call_missing" {
		t.Fatalf("缺失 tool output 未补齐: %#v", chat.Messages[1])
	}
	if chat.Messages[2].Role != "user" {
		t.Fatalf("用户消息顺序被破坏: %#v", chat.Messages[2])
	}
}

func TestResponsesToChatCompletionRequest_MimoBackfillsReasoningOnlyForMimo(t *testing.T) {
	makeRequest := func(model string) *OpenAIResponsesRequest {
		return &OpenAIResponsesRequest{
			Model: model,
			Input: []InputResponses{
				{
					Type: InputTypeMessage,
					Role: ChatMessageRoleAssistant,
					Content: []ContentResponses{
						{
							Type: ContentTypeOutputText,
							Text: "done",
						},
					},
				},
			},
		}
	}

	mimoChat, err := makeRequest("mimo-v2.5-pro").ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("MiMo 转换失败: %v", err)
	}
	if mimoChat.Messages[0].ReasoningContent == "" {
		t.Fatalf("MiMo 历史 assistant 应补 reasoning_content: %#v", mimoChat.Messages[0])
	}
	if mimoChat.Messages[0].Content != "done" {
		t.Fatalf("MiMo 纯文本 assistant 应折叠为 string，实际 %#v", mimoChat.Messages[0].Content)
	}

	gptChat, err := makeRequest("gpt-5.5").ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("GPT 转换失败: %v", err)
	}
	if gptChat.Messages[0].ReasoningContent != "" {
		t.Fatalf("普通 GPT 5.5 不应补 MiMo reasoning 占位: %#v", gptChat.Messages[0])
	}
	if _, ok := gptChat.Messages[0].Content.([]ChatMessagePart); !ok {
		t.Fatalf("普通 GPT 5.5 应保持原有 content parts 形态，实际 %#v", gptChat.Messages[0].Content)
	}
}

func TestResponsesToChatCompletionRequest_MimoNonVisionDropsImages(t *testing.T) {
	request := &OpenAIResponsesRequest{
		Model: "mimo-v2.5-pro",
		Input: []InputResponses{
			{
				Type: InputTypeMessage,
				Role: "user",
				Content: []ContentResponses{
					{
						Type:     ContentTypeInputImage,
						ImageUrl: "data:image/png;base64,aaa",
					},
				},
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	content, ok := chat.Messages[0].Content.(string)
	if !ok {
		t.Fatalf("MiMo 非视觉模型图片应改写为文本占位，实际 %#v", chat.Messages[0].Content)
	}
	if !strings.Contains(content, "omitted 1 image attachment") {
		t.Fatalf("图片占位不符合预期: %q", content)
	}
}

func TestResponsesToChatCompletionRequest_MimoVisionAddsTextForImageOnly(t *testing.T) {
	request := &OpenAIResponsesRequest{
		Model: "mimo-v2.5",
		Input: []InputResponses{
			{
				Type: InputTypeMessage,
				Role: "user",
				Content: []ContentResponses{
					{
						Type:     ContentTypeInputImage,
						ImageUrl: "https://example.com/a.png",
					},
				},
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	parts, ok := chat.Messages[0].Content.([]ChatMessagePart)
	if !ok {
		t.Fatalf("MiMo 视觉模型应保留 image_url parts，实际 %#v", chat.Messages[0].Content)
	}
	if len(parts) != 2 || parts[0].Type != "image_url" || parts[1].Type != "text" {
		t.Fatalf("image-only 消息应补 text part，实际 %#v", parts)
	}
}

func TestResponsesToChatCompletionRequest_DedupesFlattenedTools(t *testing.T) {
	request := &OpenAIResponsesRequest{
		Model: "mimo-v2.5-pro",
		Input: "hi",
		Tools: []ResponsesTools{
			{
				Type:        "function",
				Name:        "shell",
				Description: "first",
			},
			{
				Type: "namespace",
				Tools: []ResponsesTools{
					{
						Type:        "function",
						Name:        "shell",
						Description: "duplicate",
					},
					{
						Type:        "tool_search",
						Description: "search tools",
					},
				},
			},
			{
				Type: "local_shell",
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(chat.Tools) != 2 {
		t.Fatalf("应去重后保留 shell 和 tool_search 两个工具，实际 %#v", chat.Tools)
	}
	if chat.Tools[0].Function.Name != "shell" || chat.Tools[0].Function.Description != "first" {
		t.Fatalf("应保留第一个 shell 定义，实际 %#v", chat.Tools[0])
	}
	if chat.Tools[1].Function.Name != "tool_search" {
		t.Fatalf("应保留 tool_search，实际 %#v", chat.Tools[1])
	}
}

func TestResponsesToChatCompletionRequest_Gpt55KeepsExistingToolMapping(t *testing.T) {
	request := &OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: "hi",
		Tools: []ResponsesTools{
			{
				Type:        "function",
				Name:        "shell",
				Description: "first",
			},
			{
				Type:        "function",
				Name:        "shell",
				Description: "duplicate kept for non-MiMo compatibility",
			},
			{
				Type: "local_shell",
			},
			{
				Type: "namespace",
				Tools: []ResponsesTools{
					{
						Type: "tool_search",
					},
				},
			},
		},
	}

	chat, err := request.ToChatCompletionRequest()
	if err != nil {
		t.Fatalf("转换失败: %v", err)
	}
	if len(chat.Tools) != 2 {
		t.Fatalf("普通 GPT 5.5 应保持原有仅透传 function 工具行为，实际 %#v", chat.Tools)
	}
	if chat.Tools[0].Function.Description != "first" || chat.Tools[1].Function.Description != "duplicate kept for non-MiMo compatibility" {
		t.Fatalf("普通 GPT 5.5 function 工具顺序或内容被改变: %#v", chat.Tools)
	}
}
