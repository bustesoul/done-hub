package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProviderOAuthSessionRegistryCoversDescriptorProviders(t *testing.T) {
	for _, provider := range []string{"gemini-cli", "antigravity", "claude-code", "codex", "copilot"} {
		strategy, exists := providerOAuthSessionStrategies[provider]
		if !exists || strategy.Start == nil || strategy.Cancel == nil {
			t.Fatalf("OAuth strategy %q is not registered", provider)
		}
	}
}

func TestProviderOAuthSessionRejectsUnknownProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "provider", Value: "unknown"}}

	StartProviderOAuthSession(context)

	var response struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Success || response.Message == "" {
		t.Fatalf("unexpected response: %#v", response)
	}
}
