package model

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

func TestGetUserDashboardUsageIncludesCacheAndErrors(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&Log{}); err != nil {
		t.Fatal(err)
	}
	oldDB := DB
	DB = db
	defer func() { DB = oldDB }()

	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	logs := []Log{
		{UserId: 9, CreatedAt: start.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-a", Quota: 10, PromptTokens: 100, CompletionTokens: 20, RequestTime: 1000, Metadata: datatypes.NewJSONType(map[string]any{"cached_read_tokens": 80, "cached_write_tokens": 10, "openai_cache_write_tokens": 5})},
		{UserId: 9, CreatedAt: start.Add(25 * time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-b", Quota: 20, PromptTokens: 200, CompletionTokens: 40, RequestTime: 3000},
		{UserId: 9, CreatedAt: start.Add(26 * time.Hour).Unix(), Type: LogTypeError, ModelName: "gpt-b", RequestTime: 2000},
		{UserId: 10, CreatedAt: start.Add(time.Hour).Unix(), Type: LogTypeConsume, Quota: 999, PromptTokens: 999},
	}
	if err = db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}

	usage, err := GetUserDashboardUsage(9, 2, start, start.AddDate(0, 0, 2), true)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Totals.Requests != 2 || usage.Totals.Errors != 1 || usage.Totals.Quota != 30 {
		t.Fatalf("unexpected totals: %+v", usage.Totals)
	}
	if usage.Totals.CachedReadTokens != 80 || usage.Totals.CachedWriteTokens != 15 {
		t.Fatalf("cache tokens were not aggregated: %+v", usage.Totals)
	}
	if usage.Totals.CacheHitRate != 26.7 || usage.Totals.ErrorRate != 33.3 || usage.Totals.AverageLatencyMs != 2000 {
		t.Fatalf("unexpected rates: %+v", usage.Totals)
	}
	if len(usage.Daily) != 2 || usage.Daily[1].Errors != 1 || usage.Daily[1].ErrorRate != 50 {
		t.Fatalf("unexpected daily usage: %+v", usage.Daily)
	}
	if len(usage.Models) != 2 || usage.Models[0].ModelName != "gpt-b" {
		t.Fatalf("model ranking is not sorted by quota: %+v", usage.Models)
	}
}
