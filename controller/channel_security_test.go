package controller

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"done-hub/common/config"
	commonlogger "done-hub/common/logger"
	"done-hub/model"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"go.uber.org/zap"
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

func TestProviderConnectionProbeConfigChanged(t *testing.T) {
	baseURL := "https://api.example.com"
	proxy := "http://proxy.example.com"
	before := &model.Channel{
		Type: 1, ProtocolProfileID: "openai-chat-completions", BaseURL: &baseURL,
		Proxy: &proxy, Other: "v1", TestModel: "gpt-test",
	}
	after := *before
	if providerConnectionProbeConfigChanged(before, &after) {
		t.Fatal("unchanged provider config was invalidated")
	}
	after.TestModel = "gpt-changed"
	if !providerConnectionProbeConfigChanged(before, &after) {
		t.Fatal("changed test model did not invalidate probe state")
	}
}

func TestProviderConnectionDraftProbeReportsMissingTestModel(t *testing.T) {
	body := []byte(`{
		"type": 1,
		"protocol_profile_id": "openai-chat-completions",
		"name": "diagnostic",
		"key": "candidate-key",
		"models": "gpt-test"
	}`)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/provider-connections/probe", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	ProbeProviderConnectionDraft(context)

	if recorder.Code != http.StatusBadGateway || !strings.Contains(recorder.Body.String(), "请填写测速模型") {
		t.Fatalf("expected actionable missing test model response, got %d: %s", recorder.Code, recorder.Body.String())
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

func TestProviderConnectionCreateAllowsDisabledUnverifiedDraft(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:unverified-draft?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(&model.Channel{}); err != nil {
		t.Fatalf("migrate channel: %v", err)
	}
	if err := model.AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	oldDB := model.DB
	model.DB = db
	defer func() { model.DB = oldDB }()
	oldLogger := commonlogger.Logger
	commonlogger.Logger = zap.NewNop()
	defer func() { commonlogger.Logger = oldLogger }()
	viper.Set("gateway_secret_key", "unverified-draft-test")
	defer viper.Set("gateway_secret_key", "")

	body := []byte(`{
		"type": 1,
		"protocol_profile_id": "openai-chat-completions",
		"name": "unverified",
		"key": "candidate-key",
		"models": "gpt-test",
		"test_model": "gpt-test",
		"group": "default",
		"status": 1,
		"save_unverified": true
	}`)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/admin/provider-connections", bytes.NewReader(body))
	context.Request.Header.Set("Content-Type", "application/json")

	AddProviderConnection(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected draft create 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var channel model.Channel
	if err := db.First(&channel).Error; err != nil {
		t.Fatalf("load draft channel: %v", err)
	}
	if channel.Status != config.ChannelStatusManuallyDisabled || channel.TestTime != 0 {
		t.Fatalf("draft entered runnable state: status=%d test_time=%d", channel.Status, channel.TestTime)
	}
	if channel.Key != "" {
		t.Fatal("draft credential leaked to channels.key")
	}

	statusRecorder := httptest.NewRecorder()
	statusContext, _ := gin.CreateTestContext(statusRecorder)
	statusContext.Params = gin.Params{{Key: "id", Value: "1"}}
	statusContext.Request = httptest.NewRequest(http.MethodPost, "/api/admin/provider-connections/1/status", bytes.NewBufferString(`{"status":1}`))
	statusContext.Request.Header.Set("Content-Type", "application/json")
	SetProviderConnectionStatus(statusContext)
	if statusRecorder.Code != http.StatusConflict || !strings.Contains(statusRecorder.Body.String(), "尚未通过") {
		t.Fatalf("expected unverified enable rejection, got %d: %s", statusRecorder.Code, statusRecorder.Body.String())
	}
}
