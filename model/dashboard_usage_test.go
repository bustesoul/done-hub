package model

import (
	"context"
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
		{UserId: 9, CreatedAt: start.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-a", Quota: 10, PromptTokens: 100, CompletionTokens: 20, RequestTime: 1000, Metadata: datatypes.NewJSONType(map[string]any{"cached_tokens": 80, "cached_write_tokens": 10, "openai_cache_write_tokens": 5})},
		{UserId: 9, CreatedAt: start.Add(25 * time.Hour).Unix(), Type: LogTypeConsume, ModelName: "gpt-b", Quota: 20, PromptTokens: 200, CompletionTokens: 40, RequestTime: 3000, Metadata: datatypes.NewJSONType(map[string]any{"cached_read_tokens": 50})},
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
	if usage.Totals.CachedReadTokens != 130 || usage.Totals.CachedWriteTokens != 15 {
		t.Fatalf("cache tokens were not aggregated: %+v", usage.Totals)
	}
	if usage.Totals.CacheHitRate != 43.3 || usage.Totals.ErrorRate != 33.3 || usage.Totals.AverageLatencyMs != 2000 {
		t.Fatalf("unexpected rates: %+v", usage.Totals)
	}
	if len(usage.Daily) != 2 || usage.Daily[0].CachedReadTokens != 80 || usage.Daily[1].CachedReadTokens != 50 || usage.Daily[1].Errors != 1 || usage.Daily[1].ErrorRate != 50 {
		t.Fatalf("unexpected daily usage: %+v", usage.Daily)
	}
	if len(usage.Models) != 2 || usage.Models[0].ModelName != "gpt-b" {
		t.Fatalf("model ranking is not sorted by quota: %+v", usage.Models)
	}
}

func TestMetadataCachedReadTokensPrefersExplicitBucket(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]any
		want     int64
	}{
		{name: "OpenAI standard cache", metadata: map[string]any{"cached_tokens": float64(80)}, want: 80},
		{name: "explicit cache read", metadata: map[string]any{"cached_read_tokens": float64(50)}, want: 50},
		{name: "explicit bucket wins", metadata: map[string]any{"cached_tokens": float64(80), "cached_read_tokens": float64(50)}, want: 50},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := metadataCachedReadTokens(test.metadata); got != test.want {
				t.Fatalf("metadataCachedReadTokens() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestDashboardAnalysisFiltersPercentilesAndDrilldown(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(&Log{}); err != nil {
		t.Fatal(err)
	}
	old := DB
	DB = db
	defer func() { DB = old }()
	start := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	meta := func(first int) datatypes.JSONType[map[string]any] {
		return datatypes.NewJSONType(map[string]any{"first_response": first, "reasoning_effort": "high", "price_type": "tokens", "input_ratio": 2.0, "group_ratio": 0.5, "cached_read_tokens": 50, "cached_tokens": 50, "cached_read_tokens_ratio": 0.1, "cached_write_tokens": 10, "cached_write_tokens_ratio": 1.25})
	}
	logs := []Log{
		{UserId: 1, CreatedAt: start.Unix(), Type: LogTypeConsume, ModelName: "a", TokenName: "dev", IsStream: true, RequestTime: 1000, CompletionTokens: 90, PromptTokens: 100, Quota: 100, Metadata: meta(100)},
		{UserId: 1, CreatedAt: start.Add(time.Hour).Unix(), Type: LogTypeConsume, ModelName: "a", TokenName: "dev", IsStream: true, RequestTime: 2000, CompletionTokens: 170, PromptTokens: 100, Quota: 100, Metadata: meta(300)},
		{UserId: 1, CreatedAt: start.Add(2 * time.Hour).Unix(), Type: LogTypeError, ModelName: "a", TokenName: "dev", IsStream: true, RequestTime: 5, Metadata: datatypes.NewJSONType(map[string]any{"reasoning_effort": "high", "status_code": 429})},
		{UserId: 1, CreatedAt: start.Add(3 * time.Hour).Unix(), Type: LogTypeConsume, ModelName: "b", TokenName: "test", RequestTime: 90000, Quota: 999},
		{UserId: 2, CreatedAt: start.Unix(), Type: LogTypeConsume, ModelName: "a", TokenName: "dev", Quota: 99999, Metadata: meta(1)},
		{UserId: 1, CreatedAt: start.Add(-time.Hour).Unix(), Type: LogTypeConsume, ModelName: "a", TokenName: "dev", IsStream: true, RequestTime: 800, CompletionTokens: 80, Quota: 50, Metadata: meta(200)},
		{UserId: 1, CreatedAt: start.Add(24 * time.Hour).Unix(), Type: LogTypeConsume, ModelName: "a", TokenName: "dev", Quota: 99999, Metadata: meta(1)},
	}
	if err = db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	q := DashboardQuery{Start: start, End: start.Add(24 * time.Hour), PreviousStart: start.Add(-24 * time.Hour), PreviousEnd: start, Compare: true, Granularity: "hour", Model: "a", Token: "dev", Stream: "stream", Effort: "high"}
	result, err := GetDashboardAnalysis(context.Background(), 1, q, true, true)
	if err != nil {
		t.Fatal(err)
	}
	s := result.Totals
	if s.Requests != 2 || s.Errors != 1 || s.Quota != 200 || s.AverageLatencyMs != 1500 {
		t.Fatalf("wrong isolated/filter totals: %+v", s)
	}
	if s.FirstP50 == nil || *s.FirstP50 != 200 || *s.FirstP95 != 290 || *s.DurationP95 != 1950 || *s.OutputSpeed != 87.5 || s.PerformanceSamples != 2 {
		t.Fatalf("wrong performance: %+v", s)
	}
	if s.CacheSavingsQuota != 86 || s.CachePricedRequests != 2 || s.CachedReadTokens != 100 {
		t.Fatalf("wrong historical savings: %+v", s)
	}
	if result.Previous.Requests != 1 || result.Previous.Quota != 50 || len(result.Buckets) != 24 || len(result.Daily) != 2 || len(result.Filters.Models) != 2 {
		t.Fatalf("wrong comparison/buckets/facets: %+v", result)
	}
	if result.Buckets[0].PreviousStart != q.PreviousStart.Unix() || result.Buckets[0].PreviousEnd != q.PreviousStart.Add(time.Hour).Unix() {
		t.Fatal("comparison drill-down must keep previous bucket bounds")
	}
	requests, err := GetDashboardRequests(context.Background(), 1, q, 0, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Total != 3 || len(requests.Items) != 1 || requests.Items[0].Type != LogTypeError || requests.Items[0].RequestTime != 5000 {
		t.Fatalf("wrong pagination or legacy duration: %+v", requests)
	}
	q.MinFirst = 250
	requests, err = GetDashboardRequests(context.Background(), 1, q, 0, 10, nil)
	if err != nil || requests.Total != 1 || requests.Items[0].ID != logs[1].Id {
		t.Fatalf("slow drilldown: %+v %v", requests, err)
	}
	exported := 0
	_, err = GetDashboardRequests(context.Background(), 1, q, 0, 10, func(l *Log) error {
		exported++
		if l.Id != logs[1].Id {
			t.Fatalf("export escaped filter")
		}
		return nil
	})
	if err != nil || exported != 1 {
		t.Fatal(err, exported)
	}
}

func TestDashboardMissingPerformanceAndCachePrices(t *testing.T) {
	s := DashboardUsageTotals{}
	for _, l := range []Log{
		{Type: LogTypeConsume, IsStream: false, RequestTime: 2000, Metadata: datatypes.NewJSONType(map[string]any{"first_response": 100})},
		{Type: LogTypeConsume, IsStream: true, RequestTime: 500, Metadata: datatypes.NewJSONType(map[string]any{"first_response": 1000, "cached_tokens": 30})},
		{Type: LogTypeError, RequestTime: 99000},
	} {
		s.add(&l)
	}
	s.finish()
	if s.FirstP50 != nil || s.OutputSpeed != nil || s.DurationSamples != 2 || s.CacheRequests != 1 || s.CachePricedRequests != 0 || s.AverageLatencyMs != 1250 {
		t.Fatalf("fabricated samples: %+v", s)
	}
	l := Log{Type: LogTypeError, RequestTime: 5100, Metadata: datatypes.NewJSONType(map[string]any{"request_time_unit": "ms"})}
	if LogRequestTimeMs(&l) != 5100 {
		t.Fatal("new errors multiplied twice")
	}
}
