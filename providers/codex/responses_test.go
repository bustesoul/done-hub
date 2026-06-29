package codex

import (
	"done-hub/types"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestPrepareCodexRequest_PreservesAndCleansPromptCacheFields(t *testing.T) {
	request := &types.OpenAIResponsesRequest{
		Model:                "gpt-5.5",
		Input:                "hello",
		PreviousResponseID:   "resp_previous",
		PromptCacheKey:       "cache-key",
		PromptCacheRetention: "24h",
		SafetyIdentifier:     "safe-user",
		ServiceTier:          "priority",
		User:                 "legacy-user",
		Metadata:             map[string]any{"trace": "abc"},
	}

	provider := &CodexProvider{}
	provider.prepareCodexRequest(request)

	if request.PromptCacheKey != "cache-key" {
		t.Fatalf("PromptCacheKey 应保留，实际: %q", request.PromptCacheKey)
	}
	if request.PromptCacheRetention != "" {
		t.Fatalf("Codex internal 不支持 PromptCacheRetention，实际: %q", request.PromptCacheRetention)
	}
	if request.SafetyIdentifier != "" {
		t.Fatalf("Codex internal 不支持 SafetyIdentifier，实际: %q", request.SafetyIdentifier)
	}
	if request.ServiceTier != "" {
		t.Fatalf("Codex internal 不支持 ServiceTier，实际: %q", request.ServiceTier)
	}
	if request.User != "" {
		t.Fatalf("User 应迁移/清理，实际: %q", request.User)
	}
	if request.PreviousResponseID != "" {
		t.Fatalf("Codex internal HTTP 路径不支持 PreviousResponseID，实际: %q", request.PreviousResponseID)
	}
	if request.Metadata != nil {
		t.Fatalf("Codex internal 不支持 Metadata，实际: %#v", request.Metadata)
	}
	if request.Store == nil || *request.Store {
		t.Fatalf("Codex 请求必须 store=false，实际: %#v", request.Store)
	}
}

func TestPrepareCodexRequest_UsesSessionHeaderAsPromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("session_id", "session-from-header")

	request := &types.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: "hello",
	}
	provider := &CodexProvider{}
	provider.Context = ctx

	provider.prepareCodexRequest(request)

	if request.PromptCacheKey != "session-from-header" {
		t.Fatalf("应从 session_id 补齐 PromptCacheKey，实际: %q", request.PromptCacheKey)
	}
}

func TestPrepareCodexRequest_DerivesStablePromptCacheKeyFromInputPrefix(t *testing.T) {
	firstTurn := &types.OpenAIResponsesRequest{
		Model:        "gpt-5.5",
		Instructions: "stable instructions",
		Input: []types.InputResponses{
			{
				Type:    types.InputTypeMessage,
				Role:    types.ChatMessageRoleUser,
				Content: []types.ContentResponses{{Type: types.ContentTypeInputText, Text: "stable first user"}},
			},
			{
				Type:    types.InputTypeMessage,
				Role:    types.ChatMessageRoleUser,
				Content: []types.ContentResponses{{Type: types.ContentTypeInputText, Text: "turn one only"}},
			},
		},
	}
	secondTurn := &types.OpenAIResponsesRequest{
		Model:        "gpt-5.5",
		Instructions: "stable instructions",
		Input: []types.InputResponses{
			{
				Type:    types.InputTypeMessage,
				Role:    types.ChatMessageRoleUser,
				Content: []types.ContentResponses{{Type: types.ContentTypeInputText, Text: "stable first user"}},
			},
			{
				Type:    types.InputTypeMessage,
				Role:    types.ChatMessageRoleUser,
				Content: []types.ContentResponses{{Type: types.ContentTypeInputText, Text: "turn two only"}},
			},
		},
	}

	provider := &CodexProvider{}
	provider.prepareCodexRequest(firstTurn)
	provider.prepareCodexRequest(secondTurn)

	if firstTurn.PromptCacheKey == "" {
		t.Fatal("应派生稳定 PromptCacheKey")
	}
	if firstTurn.PromptCacheKey != secondTurn.PromptCacheKey {
		t.Fatalf("相同稳定前缀应派生相同 PromptCacheKey: %q != %q", firstTurn.PromptCacheKey, secondTurn.PromptCacheKey)
	}
}

func TestPrepareCodexRequest_PromotesSystemInputToInstructions(t *testing.T) {
	raw := []byte(`{
		"model": "gpt-5.5",
		"input": [
			{"type": "message", "role": "system", "content": [{"type": "input_text", "text": "system rules"}]},
			{"type": "message", "role": "developer", "content": "developer rules"},
			{"type": "message", "role": "user", "content": [{"type": "input_text", "text": "hello"}]}
		],
		"instructions": "existing instructions"
	}`)
	request := &types.OpenAIResponsesRequest{}
	if err := json.Unmarshal(raw, request); err != nil {
		t.Fatalf("请求解析失败: %v", err)
	}

	provider := &CodexProvider{}
	provider.prepareCodexRequest(request)

	expectedInstructions := "existing instructions\n\nsystem rules\n\ndeveloper rules"
	if request.Instructions != expectedInstructions {
		t.Fatalf("instructions 未正确提升: %q", request.Instructions)
	}
	input, ok := request.Input.([]any)
	if !ok {
		t.Fatalf("input 类型应保持 JSON 解码后的 []any，实际: %T", request.Input)
	}
	if len(input) != 1 {
		t.Fatalf("system/developer 消息应被移除，实际: %#v", input)
	}
	message, ok := input[0].(map[string]any)
	if !ok || message["role"] != types.ChatMessageRoleUser {
		t.Fatalf("剩余消息应是 user，实际: %#v", input[0])
	}
}

func TestPrepareCodexRequest_UsesPromotedSystemInputInsteadOfDefaultInstructions(t *testing.T) {
	request := &types.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Input: []types.InputResponses{
			{
				Type:    types.InputTypeMessage,
				Role:    types.ChatMessageRoleSystem,
				Content: "system rules",
			},
			{
				Type:    types.InputTypeMessage,
				Role:    types.ChatMessageRoleUser,
				Content: "hello",
			},
		},
	}

	provider := &CodexProvider{}
	provider.prepareCodexRequest(request)

	if request.Instructions != "system rules" {
		t.Fatalf("应使用提升后的 system 文本作为 instructions，实际: %q", request.Instructions)
	}
	input, ok := request.Input.([]types.InputResponses)
	if !ok {
		t.Fatalf("input 类型应保持 typed slice，实际: %T", request.Input)
	}
	if len(input) != 1 || input[0].Role != types.ChatMessageRoleUser {
		t.Fatalf("system 消息应被移除，实际: %#v", input)
	}
}

func TestApplyPromptCacheHeaders_BackfillsSessionHeaders(t *testing.T) {
	headers := map[string]string{}
	provider := &CodexProvider{}

	provider.applyPromptCacheHeaders(headers, "cache-key")

	if headers["session_id"] != "cache-key" {
		t.Fatalf("session_id 未补齐: %#v", headers)
	}
	if headers["conversation_id"] != "cache-key" {
		t.Fatalf("conversation_id 未补齐: %#v", headers)
	}
}

func TestApplyPromptCacheHeaders_DoesNotOverwriteClientSession(t *testing.T) {
	headers := map[string]string{
		"session_id": "client-session",
	}
	provider := &CodexProvider{}

	provider.applyPromptCacheHeaders(headers, "cache-key")

	if headers["session_id"] != "client-session" {
		t.Fatalf("不应覆盖客户端 session_id: %#v", headers)
	}
	if headers["conversation_id"] != "cache-key" {
		t.Fatalf("conversation_id 应用 prompt_cache_key 补齐: %#v", headers)
	}
}

func TestApplyPromptCacheHeaders_BackfillsSessionIDWhenOnlyXSessionIDExists(t *testing.T) {
	headers := map[string]string{
		"x-session-id": "client-x-session",
	}
	provider := &CodexProvider{}

	provider.applyPromptCacheHeaders(headers, "client-x-session")

	if headers["x-session-id"] != "client-x-session" {
		t.Fatalf("不应覆盖客户端 x-session-id: %#v", headers)
	}
	if headers["session_id"] != "client-x-session" {
		t.Fatalf("仅有 x-session-id 时应补齐 session_id: %#v", headers)
	}
}
