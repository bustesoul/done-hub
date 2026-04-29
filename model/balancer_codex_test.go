package model

import (
	"done-hub/common/config"
	"testing"
)

type codexSessionSeedContext struct {
	headers map[string]string
	body    []byte
}

func (c *codexSessionSeedContext) Get(key string) (interface{}, bool) {
	if key == config.GinRequestBodyKey && c.body != nil {
		return c.body, true
	}
	return nil, false
}

func (c *codexSessionSeedContext) GetHeader(key string) string {
	return c.headers[key]
}

func TestExtractCodexSessionSeed_Priority(t *testing.T) {
	ctx := &codexSessionSeedContext{
		headers: map[string]string{
			"conversation_id": "conversation-header",
		},
		body: []byte(`{"prompt_cache_key":"body-cache-key","user":"body-user"}`),
	}

	got := extractCodexSessionSeed(ctx)
	if got != "conversation-header" {
		t.Fatalf("应优先使用显式 header，实际: %q", got)
	}
}

func TestExtractCodexSessionSeed_UsesPromptCacheKeyBody(t *testing.T) {
	ctx := &codexSessionSeedContext{
		headers: map[string]string{},
		body:    []byte(`{"prompt_cache_key":"body-cache-key","user":"body-user"}`),
	}

	got := extractCodexSessionSeed(ctx)
	if got != "body-cache-key" {
		t.Fatalf("应使用请求体 prompt_cache_key，实际: %q", got)
	}
}

func TestDeriveCodexContentSessionSeed_StableAcrossLaterTurns(t *testing.T) {
	first := []byte(`{
		"model":"gpt-5.5",
		"instructions":"stable instructions",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"stable first user"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"turn one only"}]}
		]
	}`)
	second := []byte(`{
		"model":"gpt-5.5",
		"instructions":"stable instructions",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"stable first user"}]},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"turn two only"}]}
		]
	}`)

	firstSeed := deriveCodexContentSessionSeed(first)
	secondSeed := deriveCodexContentSessionSeed(second)
	if firstSeed == "" {
		t.Fatal("应能从稳定前缀派生 session seed")
	}
	if firstSeed != secondSeed {
		t.Fatalf("后续轮次变化不应改变稳定 seed: %q != %q", firstSeed, secondSeed)
	}
}
