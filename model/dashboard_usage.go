package model

import (
	"encoding/json"
	"math"
	"sort"
	"time"
)

type DashboardUsageTotals struct {
	Requests          int64   `json:"requests"`
	Errors            int64   `json:"errors"`
	Quota             int64   `json:"quota"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	CachedReadTokens  int64   `json:"cached_read_tokens"`
	CachedWriteTokens int64   `json:"cached_write_tokens"`
	AverageLatencyMs  float64 `json:"average_latency_ms"`
	ErrorRate         float64 `json:"error_rate"`
	CacheHitRate      float64 `json:"cache_hit_rate"`
}

type DashboardUsageDay struct {
	Date              string  `json:"date"`
	Requests          int64   `json:"requests"`
	Errors            int64   `json:"errors"`
	Quota             int64   `json:"quota"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	CachedReadTokens  int64   `json:"cached_read_tokens"`
	CachedWriteTokens int64   `json:"cached_write_tokens"`
	AverageLatencyMs  float64 `json:"average_latency_ms"`
	ErrorRate         float64 `json:"error_rate"`
	CacheHitRate      float64 `json:"cache_hit_rate"`
	latencyTotal      int64
	attempts          int64
}

type DashboardUsageModel struct {
	ModelName        string  `json:"model_name"`
	Requests         int64   `json:"requests"`
	Quota            int64   `json:"quota"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedReadTokens int64   `json:"cached_read_tokens"`
	AverageLatencyMs float64 `json:"average_latency_ms"`
	latencyTotal     int64
}

type DashboardUsage struct {
	Days          int                    `json:"days"`
	StartDate     string                 `json:"start_date"`
	EndDate       string                 `json:"end_date"`
	ErrorsTracked bool                   `json:"errors_tracked"`
	Totals        DashboardUsageTotals   `json:"totals"`
	Daily         []*DashboardUsageDay   `json:"daily"`
	Models        []*DashboardUsageModel `json:"models"`
}

// GetUserDashboardUsage 从原始日志汇总缓存命中等 statistics 表尚未保存的指标。
// JSON 元数据在 Go 中解析，避免绑定 MySQL/PostgreSQL/SQLite 各自不同的 JSON 语法。
func GetUserDashboardUsage(userID, days int, start, end time.Time, errorsTracked bool) (*DashboardUsage, error) {
	result := &DashboardUsage{
		Days:          days,
		StartDate:     start.Format("2006-01-02"),
		EndDate:       end.Add(-time.Nanosecond).Format("2006-01-02"),
		ErrorsTracked: errorsTracked,
	}

	daily := make(map[string]*DashboardUsageDay, days)
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 0, 1) {
		date := cursor.Format("2006-01-02")
		daily[date] = &DashboardUsageDay{Date: date}
	}
	models := make(map[string]*DashboardUsageModel)

	rows, err := DB.Model(&Log{}).
		Select("created_at", "type", "model_name", "quota", "prompt_tokens", "completion_tokens", "request_time", "metadata").
		Where("user_id = ? AND created_at >= ? AND created_at < ? AND type IN ?", userID, start.Unix(), end.Unix(), []int{LogTypeConsume, LogTypeError}).
		Order("created_at ASC").Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var latencyTotal int64
	var attempts int64
	for rows.Next() {
		var log Log
		if err = DB.ScanRows(rows, &log); err != nil {
			return nil, err
		}
		date := time.Unix(log.CreatedAt, 0).In(start.Location()).Format("2006-01-02")
		day, ok := daily[date]
		if !ok {
			continue
		}

		attempts++
		day.attempts++
		latencyTotal += int64(log.RequestTime)
		day.latencyTotal += int64(log.RequestTime)
		if log.Type == LogTypeError {
			result.Totals.Errors++
			day.Errors++
			continue
		}

		// Anthropic-style providers record cache reads as cached_read_tokens,
		// while OpenAI/Gemini use the standard cached_tokens field. Prefer the
		// explicit bucket and fall back to the standard one to avoid double-counting.
		cachedRead := metadataCachedReadTokens(log.Metadata.Data())
		cachedWrite := metadataInt64(log.Metadata.Data(), "cached_write_tokens") +
			metadataInt64(log.Metadata.Data(), "cached_write_1h_tokens") +
			metadataInt64(log.Metadata.Data(), "openai_cache_write_tokens")

		result.Totals.Requests++
		result.Totals.Quota += int64(log.Quota)
		result.Totals.PromptTokens += int64(log.PromptTokens)
		result.Totals.CompletionTokens += int64(log.CompletionTokens)
		result.Totals.CachedReadTokens += cachedRead
		result.Totals.CachedWriteTokens += cachedWrite

		day.Requests++
		day.Quota += int64(log.Quota)
		day.PromptTokens += int64(log.PromptTokens)
		day.CompletionTokens += int64(log.CompletionTokens)
		day.CachedReadTokens += cachedRead
		day.CachedWriteTokens += cachedWrite

		modelName := log.ModelName
		if modelName == "" {
			modelName = "unknown"
		}
		item, exists := models[modelName]
		if !exists {
			item = &DashboardUsageModel{ModelName: modelName}
			models[modelName] = item
		}
		item.Requests++
		item.Quota += int64(log.Quota)
		item.PromptTokens += int64(log.PromptTokens)
		item.CompletionTokens += int64(log.CompletionTokens)
		item.CachedReadTokens += cachedRead
		item.latencyTotal += int64(log.RequestTime)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	if attempts > 0 {
		result.Totals.AverageLatencyMs = roundMetric(float64(latencyTotal) / float64(attempts))
		result.Totals.ErrorRate = roundMetric(float64(result.Totals.Errors) * 100 / float64(attempts))
	}
	result.Totals.CacheHitRate = cacheHitRate(result.Totals.CachedReadTokens, result.Totals.PromptTokens)

	result.Daily = make([]*DashboardUsageDay, 0, len(daily))
	for cursor := start; cursor.Before(end); cursor = cursor.AddDate(0, 0, 1) {
		item := daily[cursor.Format("2006-01-02")]
		if item.attempts > 0 {
			item.AverageLatencyMs = roundMetric(float64(item.latencyTotal) / float64(item.attempts))
			item.ErrorRate = roundMetric(float64(item.Errors) * 100 / float64(item.attempts))
		}
		item.CacheHitRate = cacheHitRate(item.CachedReadTokens, item.PromptTokens)
		result.Daily = append(result.Daily, item)
	}

	result.Models = make([]*DashboardUsageModel, 0, len(models))
	for _, item := range models {
		if item.Requests > 0 {
			item.AverageLatencyMs = roundMetric(float64(item.latencyTotal) / float64(item.Requests))
		}
		result.Models = append(result.Models, item)
	}
	sort.Slice(result.Models, func(i, j int) bool {
		if result.Models[i].Quota == result.Models[j].Quota {
			return result.Models[i].Requests > result.Models[j].Requests
		}
		return result.Models[i].Quota > result.Models[j].Quota
	})

	return result, nil
}

func metadataInt64(metadata map[string]any, key string) int64 {
	value, ok := metadata[key]
	if !ok {
		return 0
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return parsed
	default:
		return 0
	}
}

func metadataCachedReadTokens(metadata map[string]any) int64 {
	cachedRead := metadataInt64(metadata, "cached_read_tokens")
	if cachedRead > 0 {
		return cachedRead
	}
	return metadataInt64(metadata, "cached_tokens")
}

func cacheHitRate(cachedRead, prompt int64) float64 {
	if prompt <= 0 || cachedRead <= 0 {
		return 0
	}
	return math.Min(100, roundMetric(float64(cachedRead)*100/float64(prompt)))
}

func roundMetric(value float64) float64 {
	return math.Round(value*10) / 10
}
