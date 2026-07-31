package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"done-hub/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func TestGetChannelDoesNotReturnCredential(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	channel := model.Channel{
		Name:   "secret-test",
		Type:   1,
		Key:    "must-not-leak",
		Status: 1,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel: %v", err)
	}
	oldDB := model.DB
	model.DB = db
	defer func() { model.DB = oldDB }()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: "1"}}
	GetChannel(context)

	if recorder.Code != 200 {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Key                  string `json:"key"`
			CredentialConfigured bool   `json:"credential_configured"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success || response.Data.Key != "" || !response.Data.CredentialConfigured {
		t.Fatalf("credential exposure regression: %#v", response)
	}
}

func TestProviderValidationTokenBindsProbedConfiguration(t *testing.T) {
	viper.Set("gateway_secret_key", "provider-validation-test")
	defer viper.Set("gateway_secret_key", "")

	baseURL := "https://api.example.com"
	channel := &model.Channel{
		Type:              8,
		ProtocolProfileID: "openai-responses",
		Key:               "candidate-key",
		BaseURL:           &baseURL,
		TestModel:         "gpt-test",
	}
	token, err := issueProviderValidationToken(channel)
	if err != nil {
		t.Fatalf("issue validation token: %v", err)
	}
	channel.ValidationToken = token
	if err := verifyProviderValidationToken(channel); err != nil {
		t.Fatalf("verify unchanged config: %v", err)
	}
	channel.Key = "changed-after-probe"
	if err := verifyProviderValidationToken(channel); err == nil {
		t.Fatal("expected changed credential to invalidate probe token")
	}
}

func TestProviderConnectionCreateRejectsMissingProbeToken(t *testing.T) {
	viper.Set("gateway_secret_key", "provider-validation-test")
	defer viper.Set("gateway_secret_key", "")

	gin.SetMode(gin.TestMode)
	body := []byte(`{
		"type": 1,
		"protocol_profile_id": "openai-chat-completions",
		"name": "unverified",
		"key": "candidate-key",
		"models": "gpt-test",
		"group": "default"
	}`)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/channel/", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	AddProviderConnection(context)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected missing probe token to return 409, got %d: %s", recorder.Code, recorder.Body.String())
	}
}
