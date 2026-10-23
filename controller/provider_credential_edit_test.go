package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"done-hub/common/cache"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/middleware"
	"done-hub/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func setupEditableProvider(t *testing.T) (*gorm.DB, *model.Channel) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Channel{}, &model.User{}); err != nil {
		t.Fatal(err)
	}
	if err := model.AutoMigrateGatewayResources(db); err != nil {
		t.Fatal(err)
	}
	oldDB, oldLogger := model.DB, logger.Logger
	model.DB, logger.Logger = db, zap.NewNop()
	model.GatewayRoutes = model.GatewayRouteIndex{}
	oldKey := viper.GetString("gateway_secret_key")
	viper.Set("gateway_secret_key", "editable-provider-test")
	cache.InitCacheManager()
	t.Cleanup(func() {
		model.DB, logger.Logger = oldDB, oldLogger
		model.GatewayRoutes = model.GatewayRouteIndex{}
		viper.Set("gateway_secret_key", oldKey)
		sqlDB, _ := db.DB()
		sqlDB.Close()
	})
	channel := &model.Channel{
		Type: config.ChannelTypeOpenAI, ProtocolProfileID: "openai-chat-completions",
		Name: "editable", Key: "original-secret", Models: "gpt-test", TestModel: "gpt-test",
		Status: config.ChannelStatusEnabled, TestTime: 123, Group: "default",
	}
	if err := channel.Insert(); err != nil {
		t.Fatal(err)
	}
	channel, err = model.GetChannelById(channel.Id)
	if err != nil {
		t.Fatal(err)
	}
	return db, channel
}

func callProviderHandler(t *testing.T, id int, method string, body any, handler gin.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "id", Value: fmt.Sprint(id)}}
	context.Request = httptest.NewRequest(method, "/", bytes.NewReader(encoded))
	context.Request.Header.Set("Content-Type", "application/json")
	handler(context)
	return recorder
}

func TestActiveProviderCredentialSecretRequiresAdmin(t *testing.T) {
	db, channel := setupEditableProvider(t)
	for _, user := range []model.User{
		{Username: "reader", Role: config.RoleCommonUser, Status: config.UserStatusEnabled, AccessToken: "reader-token", AffCode: "reader"},
		{Username: "admin", Role: config.RoleAdminUser, Status: config.UserStatusEnabled, AccessToken: "admin-token", AffCode: "admin"},
	} {
		if err := db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
	}
	router := gin.New()
	router.Use(sessions.Sessions("test", cookie.NewStore([]byte("credential-test-session"))))
	router.GET("/:id/credentials/active", middleware.AdminAuth(), GetActiveProviderCredentialSecret)
	for _, token := range []string{"", "reader-token", "admin-token"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/%d/credentials/active", channel.Id), nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		router.ServeHTTP(recorder, request)
		hasSecret := strings.Contains(recorder.Body.String(), "original-secret")
		if hasSecret != (token == "admin-token") {
			t.Fatalf("unexpected credential access for %q", token)
		}
		if token == "admin-token" && recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("credential response must not be cached")
		}
	}
	for _, id := range []int{0, channel.Id + 1} {
		response := callProviderHandler(t, id, http.MethodGet, nil, GetActiveProviderCredentialSecret)
		if response.Code < 400 || strings.Contains(response.Body.String(), "original-secret") {
			t.Fatal("invalid channel exposed credential")
		}
	}
	credential, _, err := model.LoadActiveGatewayCredentialSecret(db, channel.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(credential).Update("status", model.GatewayCredentialRevoked).Error; err != nil {
		t.Fatal(err)
	}
	response := callProviderHandler(t, channel.Id, http.MethodGet, nil, GetActiveProviderCredentialSecret)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "original-secret") {
		t.Fatal("revoked credential was exposed")
	}
}

func TestProviderCredentialCanBeEditedAndEnabledWithoutProbe(t *testing.T) {
	db, channel := setupEditableProvider(t)
	channel.Key = "edited-secret"
	channel.SaveUnverified = true
	response := callProviderHandler(t, channel.Id, http.MethodPatch, channel, UpdateChannel)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("edit failed: %d %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "edited-secret") {
		t.Fatal("normal update response exposed credential")
	}
	credential, plaintext, err := model.LoadActiveGatewayCredentialSecret(db, channel.Id)
	if err != nil || plaintext != "edited-secret" || credential.TestedAt != nil || credential.SecretVersion != 2 {
		t.Fatal("edited credential was not saved encrypted with untested state")
	}
	var stored model.Channel
	if err := db.First(&stored, channel.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Key != "" || stored.Status != config.ChannelStatusManuallyDisabled || stored.TestTime != 0 {
		t.Fatal("unverified edit did not preserve encrypted storage and disabled state")
	}
	response = callProviderHandler(t, channel.Id, http.MethodPost, gin.H{"status": 1}, SetProviderConnectionStatus)
	loaded := model.GatewayRoutes.GetChannel(channel.Id)
	if response.Code != http.StatusOK || loaded == nil || loaded.Key != "edited-secret" {
		t.Fatal("manual enable did not immediately load edited credential into routing")
	}
	channel, err = model.GetChannelById(channel.Id)
	if err != nil {
		t.Fatal(err)
	}
	response = callProviderHandler(t, channel.Id, http.MethodPatch, channel, UpdateChannel)
	if response.Code != http.StatusOK {
		t.Fatalf("unchanged key could not be saved: %s", response.Body.String())
	}
	credentials, err := model.ListGatewayCredentials(db, channel.Id)
	if err != nil || len(credentials) != 2 {
		t.Fatal("unchanged key unexpectedly created a new credential version")
	}
}

func TestProviderDraftProbeUsesEditedCredential(t *testing.T) {
	db, channel := setupEditableProvider(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer edited-secret" {
			t.Error("probe did not use edited credential")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"test","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()
	oldClient := requester.HTTPClient
	requester.HTTPClient = server.Client()
	defer func() { requester.HTTPClient = oldClient }()
	channel.Key, channel.BaseURL = "edited-secret", &server.URL
	response := callProviderHandler(t, channel.Id, http.MethodPost, channel, ProbeProviderConnection)
	if response.Code != http.StatusOK {
		t.Fatalf("probe failed: %s", response.Body.String())
	}
	var result struct {
		Data struct {
			ValidationToken string `json:"validation_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	channel.ValidationToken = result.Data.ValidationToken
	response = callProviderHandler(t, channel.Id, http.MethodPatch, channel, UpdateChannel)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatalf("verified key edit failed: %s", response.Body.String())
	}
	credential, plaintext, err := model.LoadActiveGatewayCredentialSecret(db, channel.Id)
	if err != nil || plaintext != "edited-secret" || credential.TestedAt == nil {
		t.Fatal("verified edit did not activate the probed credential")
	}
}

func TestProviderCredentialEditRollsBackWithConfiguration(t *testing.T) {
	db, channel := setupEditableProvider(t)
	if err := db.Callback().Create().Before("gorm:create").Register("fail_policy_sync", func(tx *gorm.DB) {
		if tx.Statement.Table == "gateway_policies" {
			tx.AddError(errors.New("test projection failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	channel.Name, channel.Key, channel.SaveUnverified = "changed-name", "edited-secret", true
	response := callProviderHandler(t, channel.Id, http.MethodPatch, channel, UpdateChannel)
	if strings.Contains(response.Body.String(), `"success":true`) {
		t.Fatal("failed projection update unexpectedly succeeded")
	}
	credential, plaintext, err := model.LoadActiveGatewayCredentialSecret(db, channel.Id)
	if err != nil || plaintext != "original-secret" || credential.SecretVersion != 1 {
		t.Fatal("failed configuration update changed the active credential")
	}
	stored, err := model.GetChannelById(channel.Id)
	if err != nil || stored.Name != "editable" || stored.Status != config.ChannelStatusEnabled {
		t.Fatal("failed credential update changed the channel configuration")
	}
	credentials, err := model.ListGatewayCredentials(db, channel.Id)
	if err != nil || len(credentials) != 1 {
		t.Fatal("failed credential update left a partial version")
	}
}
