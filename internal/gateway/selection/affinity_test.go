package selection

import (
	"net/http"
	"testing"

	"done-hub/internal/gateway/domain"
)

func TestBuildAffinityKeyProtocolSignals(t *testing.T) {
	tests := []struct {
		name     string
		protocol domain.Protocol
		body     string
		headers  http.Header
		source   string
	}{
		{name: "responses", protocol: domain.ProtocolOpenAIResponses, body: `{"prompt_cache_key":"cache-a"}`, source: "prompt_cache_key"},
		{name: "chat-cache-key", protocol: domain.ProtocolOpenAIChat, body: `{"prompt_cache_key":"cache-a"}`, source: "prompt_cache_key"},
		{name: "chat", protocol: domain.ProtocolOpenAIChat, body: `{"user":"customer-a"}`, source: "request_user"},
		{name: "anthropic", protocol: domain.ProtocolClaudeMessages, body: `{"metadata":{"user_id":{"session_id":"session-a"}}}`, source: "claude_session"},
		{name: "gemini", protocol: domain.ProtocolGemini, body: `{"cachedContent":"cachedContents/123"}`, source: "cached_content"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			key, ok := BuildAffinityKey(AffinityInput{
				Protocol: test.protocol,
				Body:     []byte(test.body),
				Headers:  test.headers,
				UserID:   7,
				TokenID:  9,
				Group:    "default",
				Model:    "model-a",
			})
			if !ok || key.Digest == "" {
				t.Fatal("expected affinity key")
			}
			if key.Source != test.source {
				t.Fatalf("source = %q, want %q", key.Source, test.source)
			}
		})
	}
}

func TestBuildAffinityKeyFallsBackToAuthenticatedIdentity(t *testing.T) {
	first, ok := BuildAffinityKey(AffinityInput{Protocol: domain.ProtocolOpenAIChat, UserID: 11, TokenID: 12, Group: "g", Model: "m"})
	if !ok || first.Source != "authenticated_user" {
		t.Fatalf("unexpected fallback: %#v, %t", first, ok)
	}
	second, _ := BuildAffinityKey(AffinityInput{Protocol: domain.ProtocolOpenAIChat, UserID: 11, TokenID: 12, Group: "g", Model: "m"})
	if first.Digest != second.Digest {
		t.Fatal("same identity must produce stable affinity key")
	}
}

func TestBuildAffinityKeyRejectsAnonymousCaller(t *testing.T) {
	key, ok := BuildAffinityKey(AffinityInput{
		Protocol: domain.ProtocolOpenAIResponses,
		Body:     []byte(`{"prompt_cache_key":"shared-cache"}`),
		Model:    "gpt-5",
	})
	if ok || key != (AffinityKey{}) {
		t.Fatalf("anonymous caller must not receive affinity key: %#v", key)
	}
}

func TestPickWeightedRendezvousIsStableAndFailsOver(t *testing.T) {
	key := AffinityKey{Digest: "stable-key"}
	candidates := []Candidate{{EndpointID: 1, Weight: 1}, {EndpointID: 2, Weight: 3}, {EndpointID: 3, Weight: 1}}
	selected, ok := PickWeightedRendezvous(key, candidates)
	if !ok {
		t.Fatal("expected selection")
	}
	again, _ := PickWeightedRendezvous(key, candidates)
	if selected != again {
		t.Fatalf("selection changed: %d != %d", selected, again)
	}
	fallback := make([]Candidate, 0, 2)
	for _, candidate := range candidates {
		if candidate.EndpointID != selected {
			fallback = append(fallback, candidate)
		}
	}
	next, ok := PickWeightedRendezvous(key, fallback)
	if !ok || next == selected {
		t.Fatalf("expected deterministic fallback, got %d", next)
	}
}

func TestAffinityKeyIsIsolatedByRoute(t *testing.T) {
	base := AffinityInput{Protocol: domain.ProtocolOpenAIResponses, Body: []byte(`{"prompt_cache_key":"same"}`), UserID: 1, TokenID: 2, Group: "g", Model: "m1"}
	first, _ := BuildAffinityKey(base)
	base.Model = "m2"
	second, _ := BuildAffinityKey(base)
	if first.Digest == second.Digest {
		t.Fatal("models must not share affinity keys")
	}
}
