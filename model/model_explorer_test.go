package model

import (
	"context"
	"done-hub/common/config"
	"done-hub/common/logger"
	"done-hub/internal/gateway/secret"
	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"testing"
	"time"
)

func explorerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&Channel{}, &Log{}); err != nil {
		t.Fatal(err)
	}
	if err = AutoMigrateGatewayResources(db); err != nil {
		t.Fatal(err)
	}
	old := DB
	DB = db
	t.Cleanup(func() { DB = old; sql, _ := db.DB(); sql.Close() })
	return db
}

func TestExplorerFamilyIgnoresPricingProvider(t *testing.T) {
	db := explorerTestDB(t)
	oldPricing, oldOwners := PricingInstance, ModelOwnedBysInstance
	t.Cleanup(func() { PricingInstance, ModelOwnedBysInstance = oldPricing, oldOwners })
	PricingInstance = &Pricing{Prices: map[string]*Price{
		"gemini-3.7-flash": {ChannelType: config.ChannelTypeGemini},
		"gemini-3.8-flash": {ChannelType: config.ChannelTypeOpenRouter, ModelInfo: &ModelInfoResponse{Name: "Gemini 3.8 Flash"}},
		"custom-model":     {ChannelType: config.ChannelTypeOpenRouter},
	}}
	ModelOwnedBysInstance = &ModelOwnedBys{ModelOwnedBy: map[int]*ModelOwnedBy{
		config.ChannelTypeGemini:     {Name: "Google Gemini"},
		config.ChannelTypeOpenRouter: {Name: "OpenRouter"},
	}}
	if err := db.Create(&GatewayEndpoint{ChannelID: 1, Status: config.ChannelStatusEnabled}).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"gemini-3.7-flash", "gemini-3.8-flash", "custom-model"} {
		if err := db.Create(&GatewayModelRoute{ChannelID: 1, PublicModel: name, UpstreamModel: name}).Error; err != nil {
			t.Fatal(err)
		}
	}
	models, err := GetExplorerModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 3 {
		t.Fatalf("models=%d", len(models))
	}
	for _, m := range models {
		want := "Gemini"
		if m.ID == "custom-model" {
			want = "未分类"
		}
		if m.Family != want {
			t.Errorf("%s: family=%q, want %q", m.ID, m.Family, want)
		}
		if m.ID == "gemini-3.8-flash" && m.Name != "Gemini 3.8 Flash" {
			t.Error("model metadata display name lost")
		}
	}
}

func TestExplorerModelFamilies(t *testing.T) {
	for _, tt := range []struct {
		name      string
		upstreams []string
		want      string
	}{
		{"google/gemini-3.8-flash", nil, "Gemini"},
		{"openrouter/google/gemini-3.7-flash:free", nil, "Gemini"},
		{"Gemini-3.8-Flash", nil, "Gemini"},
		{"anthropic/claude-sonnet-4", nil, "Claude"},
		{"openai/gpt-5", nil, "GPT"},
		{"o3-mini", nil, "OpenAI o系列"},
		{"qwen/qwen3-32b", nil, "Qwen"},
		{"gemma-3", nil, "Gemma"},
		{"geminiproxy", nil, "未分类"},
		{"openrouter/auto", nil, "未分类"},
		{"alias", []string{"gemini-3.7-flash", "google/gemini-3.8-flash"}, "Gemini"},
		{"alias", []string{"gemini-3.8-flash", "claude-sonnet-4"}, "未分类"},
		{"alias", []string{"gemini-3.8-flash", "unknown"}, "未分类"},
		{"gemini-3.8-flash", []string{"internal-deployment"}, "Gemini"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := &ExplorerModel{ID: tt.name}
			for _, name := range tt.upstreams {
				m.Routes = append(m.Routes, &ModelExplorerRoute{Upstream: name})
			}
			if got := explorerModelFamily(m); got != tt.want {
				t.Errorf("family=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestModelChannelOverridePersistsAndFiltersAllGroups(t *testing.T) {
	db := explorerTestDB(t)
	oldLogger := logger.Logger
	logger.Logger = zap.NewNop()
	t.Cleanup(func() { logger.Logger = oldLogger })
	oldKey := viper.GetString("gateway_secret_key")
	viper.Set("gateway_secret_key", "model-explorer-test")
	t.Cleanup(func() { viper.Set("gateway_secret_key", oldKey) })
	GatewayRoutes = GatewayRouteIndex{}
	t.Cleanup(func() { GatewayRoutes = GatewayRouteIndex{} })
	channels := []Channel{
		{Type: config.ChannelTypeOpenAI, ProtocolProfileID: "openai-chat-completions", Key: "test", Name: "primary", Status: 1, Models: "model-a,model-b,wild-*", Group: "default,vip"},
		{Type: config.ChannelTypeOpenAI, ProtocolProfileID: "openai-chat-completions", Key: "test", Name: "fallback", Status: 1, Models: "model-a", Group: "default,vip"},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatal(err)
	}
	cipher, _ := secret.New("model-explorer-test")
	if err := BackfillGatewayResources(db, cipher); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&GatewayModelRoute{}).Where("channel_id = ?", channels[0].Id).Update("priority", 100).Error; err != nil {
		t.Fatal(err)
	}
	GatewayRoutes.Load()
	ctx := context.Background()
	if err := SetModelChannelEnabled(ctx, channels[0].Id, "model-a", false); err != nil {
		t.Fatal(err)
	}
	for _, group := range []string{"default", "vip"} {
		ch, err := GatewayRoutes.Next(group, "model-a")
		if err != nil || ch.Id != channels[1].Id {
			t.Fatalf("disabled primary selected: %v %v", ch, err)
		}
		if n := GatewayRoutes.CountAvailableChannels(group, "model-a"); n != 1 {
			t.Fatalf("count=%d", n)
		}
		ch, err = GatewayRoutes.NextByValidatedModel(group, "model-b", nil)
		if err != nil || ch.Id != channels[0].Id {
			t.Fatal("other model affected", err)
		}
	}
	if err := SyncGatewayChannels(db, []int{channels[0].Id}); err != nil {
		t.Fatal(err)
	}
	GatewayRoutes.Load()
	if !GatewayRoutes.IsModelDisabled(channels[0].Id, "model-a") {
		t.Fatal("channel edit reset override")
	}
	if err := SetModelChannelEnabled(ctx, channels[0].Id, "wild-*", false); err != nil {
		t.Fatal(err)
	}
	if _, err := GatewayRoutes.Next("default", "wild-actual"); err == nil {
		t.Fatal("wildcard bypass")
	}
	if _, err := GatewayRoutes.NextByValidatedModel("default", "wild-*", nil); err == nil {
		t.Fatal("validated wildcard bypass")
	}
	if err := UpdateChannelStatusById(channels[0].Id, config.ChannelStatusManuallyDisabled); err != nil {
		t.Fatal(err)
	}
	if err := SetModelChannelEnabled(ctx, channels[0].Id, "model-a", true); err == nil {
		t.Fatal("global off allowed enabling")
	}
	catalog, err := GetExplorerModels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 3 || len(catalog[0].Routes) != 2 || catalog[0].Routes[0].Global || catalog[0].Routes[0].Enabled {
		t.Fatalf("closed binding missing: %+v", catalog)
	}
	if err := UpdateChannelStatusById(channels[0].Id, config.ChannelStatusEnabled); err != nil {
		t.Fatal(err)
	}
	if !GatewayRoutes.IsModelDisabled(channels[0].Id, "model-a") {
		t.Fatal("global enable reset override")
	}
	if err := SetModelChannelEnabled(ctx, channels[0].Id, "model-a", true); err != nil {
		t.Fatal(err)
	}
	ch, err := GatewayRoutes.Next("default", "model-a")
	if err != nil || ch.Id != channels[0].Id {
		t.Fatal("binding did not restore primary")
	}
	if err := SetModelChannelEnabled(ctx, channels[0].Id, "missing", false); err == nil {
		t.Fatal("nonexistent binding accepted")
	}
	if err := DeleteGatewayChannelResources(db, channels[0].Id); err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&ModelChannelOverride{}).Count(&count)
	if count != 0 {
		t.Fatal("orphan override", count)
	}
}

func TestExplorerHistoryAttributionCostsAndWindow(t *testing.T) {
	db := explorerTestDB(t)
	now := time.Unix(200000, 0)
	meta := func(v map[string]any) datatypes.JSONType[map[string]any] { return datatypes.NewJSONType(v) }
	models := []*ExplorerModel{
		{ID: "a", Routes: []*ModelExplorerRoute{{ID: 1, Model: "a", Upstream: "shared"}, {ID: 2, Model: "a", Upstream: "unique"}}},
		{ID: "b", Routes: []*ModelExplorerRoute{{ID: 1, Model: "b", Upstream: "shared"}}},
		{ID: "empty", Routes: []*ModelExplorerRoute{{ID: 3, Model: "empty", Upstream: "empty"}}},
	}
	logs := []Log{
		{CreatedAt: now.Unix() - 1, Type: 2, ChannelId: 1, ModelName: "shared", Quota: 100, CostQuota: 70, IsStream: true, RequestTime: 1000, CompletionTokens: 90, Metadata: meta(map[string]any{"route_model": "a", "first_response": 100})},
		{CreatedAt: now.Unix() - 2, Type: 5, ChannelId: 1, ModelName: "shared", Quota: 999, CostQuota: 999, RequestTime: 4, Metadata: meta(map[string]any{"route_model": "a", "status_code": 429, "error_message": "rate limit"})},
		{CreatedAt: now.Unix() - 3, Type: 2, ChannelId: 1, ModelName: "shared", Quota: 999, CostQuota: 999}, // ambiguous: never duplicate
		{CreatedAt: now.Unix() - 86400, Type: 2, ChannelId: 2, ModelName: "unique", Quota: 200, CostQuota: 0, IsStream: true, RequestTime: 2000, CompletionTokens: 160, Metadata: meta(map[string]any{"first_response": 400})},
		{CreatedAt: now.Unix() - 86401, Type: 2, ChannelId: 2, ModelName: "unique", Quota: 999},
		{CreatedAt: now.Unix(), Type: 2, ChannelId: 2, ModelName: "unique", Quota: 999},
		{CreatedAt: now.Unix() - 4, Type: 2, ChannelId: 1, ModelName: "b", Quota: 10, RequestTime: 100, IsStream: true, Metadata: meta(map[string]any{"first_response": 200})},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	h, err := GetExplorerHistory(context.Background(), models, now, true, true)
	if err != nil {
		t.Fatal(err)
	}
	a := h.Totals["a"]
	if a.Quota != 300 || a.CostQuota != 70 || a.ConsumeCount != 2 || a.ErrorCount != 1 || a.CostSampleCount != 1 || h.Unattributed != 1 {
		t.Fatalf("wrong totals: %+v", a)
	}
	if a.Ratio != nil || a.Ambiguous != 1 || h.Totals["b"].Ambiguous != 1 {
		t.Fatal("ambiguous ratio presented as complete")
	}
	if a.FirstResponseMs == nil || *a.FirstResponseMs != 250 || a.LatencyMs == nil || *a.LatencyMs != 1500 || a.TPS == nil || *a.TPS != 100 {
		t.Fatalf("bad performance: %+v", a)
	}
	if h.Totals["empty"].Ratio != nil || h.Totals["b"].FirstResponseMs != nil {
		t.Fatal("fabricated missing metrics")
	}
	if len(a.RecentErrors) != 1 || a.RecentErrors[0].Status != 429 {
		t.Fatal("error metadata missing")
	}
	h, err = GetExplorerHistory(context.Background(), models, now, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if h.ByModel["a"][2].Ratio != nil {
		t.Fatal("disabled error logging yielded success ratio")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = GetExplorerHistory(ctx, models, now, true, true); err == nil {
		t.Fatal("ignored context cancellation")
	}
}
