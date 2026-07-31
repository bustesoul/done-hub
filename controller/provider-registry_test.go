package controller

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetProviderDefinitions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	GetProviderDefinitions(context)

	if recorder.Code != 200 {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var response struct {
		Success bool `json:"success"`
		Data    []struct {
			ID           string   `json:"id"`
			ChannelType  int      `json:"channel_type"`
			Capabilities []string `json:"capabilities"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || len(response.Data) == 0 {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Data[0].ID != "openai" || response.Data[0].ChannelType != 1 {
		t.Fatalf("unexpected first provider: %#v", response.Data[0])
	}
	if len(response.Data[0].Capabilities) == 0 {
		t.Fatal("OpenAI provider should expose capabilities")
	}
}

func TestGetConnectionProfiles(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)

	GetConnectionProfiles(context)

	if recorder.Code != 200 {
		t.Fatalf("expected status 200, got %d", recorder.Code)
	}
	var response struct {
		Success bool `json:"success"`
		Data    []struct {
			ID           string `json:"id"`
			Protocol     string `json:"protocol"`
			Featured     bool   `json:"featured"`
			DisplayOrder int    `json:"display_order"`
			Variants     []struct {
				ProviderID  string `json:"provider_id"`
				ChannelType int    `json:"channel_type"`
			} `json:"variants"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []string{"openai-chat-completions", "openai-responses", "anthropic-messages", "google-gemini"}
	if !response.Success || len(response.Data) != len(want) {
		t.Fatalf("unexpected response: %#v", response)
	}
	for index, profile := range response.Data {
		if profile.ID != want[index] || !profile.Featured || profile.DisplayOrder <= 0 || len(profile.Variants) == 0 {
			t.Fatalf("profile %d is incomplete: %#v", index, profile)
		}
	}
}
