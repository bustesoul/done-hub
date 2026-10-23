package model

import (
	"context"
	"encoding/json"
	"fmt"
	"gorm.io/gorm"
	"math"
	"sort"
	"time"
)

// DashboardQuery is shared by aggregates, request drill-down and CSV export.
// Time bounds are Unix seconds and always half-open, including log drill-down.
type DashboardQuery struct {
	Start, End                                   time.Time
	PreviousStart, PreviousEnd                   time.Time
	Granularity                                  string
	Model, Token, Stream, Effort, Status, Search string
	MinFirst                                     float64
	ModelSet, TokenSet                           bool
	Compare                                      bool
}

const DashboardMaxRecords = 300000

type DashboardUsageTotals struct {
	Requests                  int64    `json:"requests"`
	Errors                    int64    `json:"errors"`
	Quota                     int64    `json:"quota"`
	PromptTokens              int64    `json:"prompt_tokens"`
	CompletionTokens          int64    `json:"completion_tokens"`
	CachedReadTokens          int64    `json:"cached_read_tokens"`
	CachedWriteTokens         int64    `json:"cached_write_tokens"`
	AverageLatencyMs          float64  `json:"average_latency_ms"`
	ErrorRate                 float64  `json:"error_rate"`
	CacheHitRate              float64  `json:"cache_hit_rate"`
	FirstP50                  *float64 `json:"first_p50_ms"`
	FirstP95                  *float64 `json:"first_p95_ms"`
	DurationP95               *float64 `json:"duration_p95_ms"`
	OutputSpeed               *float64 `json:"output_tokens_per_second"`
	PerformanceSamples        int      `json:"performance_samples"`
	DurationSamples           int      `json:"duration_samples"`
	SpeedSamples              int      `json:"speed_samples"`
	CacheSavingsQuota         float64  `json:"cache_savings_quota"`
	CachePricedRequests       int64    `json:"cache_priced_requests"`
	CacheRequests             int64    `json:"cache_requests"`
	firsts, durations, speeds []float64
}

type DashboardUsageDay struct {
	DashboardUsageTotals
	Date          string                           `json:"date"`
	Start         int64                            `json:"start"`
	End           int64                            `json:"end"`
	PreviousStart int64                            `json:"previous_start,omitempty"`
	PreviousEnd   int64                            `json:"previous_end,omitempty"`
	Previous      *DashboardUsageTotals            `json:"previous,omitempty"`
	Models        map[string]*DashboardUsageTotals `json:"models,omitempty"`
}
type DashboardUsageModel struct {
	DashboardUsageTotals
	ModelName string                `json:"model_name"`
	TokenName string                `json:"token_name,omitempty"`
	Previous  *DashboardUsageTotals `json:"previous,omitempty"`
}
type DashboardRange struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}
type DashboardFilters struct {
	Models  []string `json:"models"`
	Tokens  []string `json:"tokens"`
	Efforts []string `json:"efforts"`
}
type DashboardUsage struct {
	Days           int                    `json:"days"`
	StartDate      string                 `json:"start_date"`
	EndDate        string                 `json:"end_date"`
	ErrorsTracked  bool                   `json:"errors_tracked"`
	ConsumeTracked bool                   `json:"consume_tracked"`
	Totals         DashboardUsageTotals   `json:"totals"`
	Previous       *DashboardUsageTotals  `json:"previous"`
	Daily          []*DashboardUsageDay   `json:"daily"`
	Buckets        []*DashboardUsageDay   `json:"buckets"`
	Models         []*DashboardUsageModel `json:"models"`
	Keys           []*DashboardUsageModel `json:"keys"`
	Filters        DashboardFilters       `json:"filters"`
	Range          DashboardRange         `json:"range"`
	PreviousRange  DashboardRange         `json:"previous_range"`
	Granularity    string                 `json:"granularity"`
	GeneratedAt    int64                  `json:"generated_at"`
}

// Retained for existing callers. New API calls use the same aggregation engine.
func GetUserDashboardUsage(userID, days int, start, end time.Time, errorsTracked bool) (*DashboardUsage, error) {
	r, err := GetDashboardAnalysis(context.Background(), userID, DashboardQuery{Start: start, End: end, Granularity: "day"}, errorsTracked, true)
	if r != nil {
		r.Days = days
	}
	return r, err
}

// Uses the existing raw-log aggregation path instead of statistics (which loses
// percentiles/metadata). Unlike GetUserLogsList, this supports metadata predicates
// before pagination and never loads administrator-only columns or channel data.
func scanDashboardLogs(ctx context.Context, userID int, q DashboardQuery, start, end time.Time, visit func(*Log) error) error {
	tx := DB.WithContext(ctx).Model(&Log{}).Select("id", "created_at", "content", "type", "model_name", "token_name", "quota", "prompt_tokens", "completion_tokens", "request_time", "is_stream", "metadata").Where("user_id = ? AND created_at >= ? AND created_at < ? AND type IN ?", userID, start.Unix(), end.Unix(), []int{LogTypeConsume, LogTypeError})
	return scanUsageLogs(tx, visit)
}

// Shared bounded scanner; each caller owns its authorization and selected columns.
func scanUsageLogs(tx *gorm.DB, visit func(*Log) error) error {
	rows, err := tx.Order("created_at DESC, id DESC").Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	scanned := 0
	for rows.Next() {
		scanned++
		if scanned > DashboardMaxRecords {
			return fmt.Errorf("记录超过 %d 条，请缩短时间范围", DashboardMaxRecords)
		}
		var l Log
		if err = tx.ScanRows(rows, &l); err != nil {
			return err
		}
		if err = visit(&l); err != nil {
			return err
		}
	}
	return rows.Err()
}
func dashboardMatches(l *Log, q DashboardQuery) bool {
	if (q.ModelSet || q.Model != "") && q.Model != l.ModelName {
		return false
	}
	if (q.TokenSet || q.Token != "") && q.Token != l.TokenName {
		return false
	}
	if q.Stream == "stream" && !l.IsStream || q.Stream == "nonstream" && l.IsStream {
		return false
	}
	effort, _ := l.Metadata.Data()["reasoning_effort"].(string)
	if effort == "" {
		effort = "default"
	}
	if q.Effort != "" && q.Effort != effort {
		return false
	}
	if q.Status == "success" && l.Type != LogTypeConsume || q.Status == "error" && l.Type != LogTypeError {
		return false
	}
	if q.Status != "" && q.Status != "success" && q.Status != "error" && fmt.Sprint(metadataInt64(l.Metadata.Data(), "status_code")) != q.Status {
		return false
	}
	first := LogFirstResponseMs(l)
	if q.MinFirst > 0 && (l.Type != LogTypeConsume || first < q.MinFirst) {
		return false
	}
	if q.Search != "" && q.Search != fmt.Sprint(l.Id) {
		return false
	}
	return true
}
func GetDashboardAnalysis(ctx context.Context, userID int, q DashboardQuery, errorsTracked, consumeTracked bool) (*DashboardUsage, error) {
	r := &DashboardUsage{ErrorsTracked: errorsTracked, ConsumeTracked: consumeTracked, StartDate: q.Start.Format("2006-01-02"), EndDate: q.End.Add(-time.Second).Format("2006-01-02"), Range: DashboardRange{q.Start.Unix(), q.End.Unix()}, Granularity: q.Granularity, GeneratedAt: time.Now().Unix(), Models: []*DashboardUsageModel{}, Keys: []*DashboardUsageModel{}}
	r.Daily = dashboardBuckets(q.Start, q.End, "day", true)
	r.Days = len(r.Daily)
	r.Buckets = dashboardBuckets(q.Start, q.End, q.Granularity, false)
	models, keys := map[string]*DashboardUsageModel{}, map[string]*DashboardUsageModel{}
	fm, ft, fe := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var previousBuckets []*DashboardUsageDay
	pm := map[string]*DashboardUsageModel{}
	start := q.Start
	if q.Compare {
		start = q.PreviousStart
		r.Previous = &DashboardUsageTotals{}
		r.PreviousRange = DashboardRange{q.PreviousStart.Unix(), q.PreviousEnd.Unix()}
		previousBuckets = dashboardBuckets(q.PreviousStart, q.PreviousEnd, q.Granularity, false)
	}
	err := scanDashboardLogs(ctx, userID, q, start, q.End, func(l *Log) error {
		current := l.CreatedAt >= q.Start.Unix()
		if current {
			fm[l.ModelName] = true
			ft[l.TokenName] = true
			effort, _ := l.Metadata.Data()["reasoning_effort"].(string)
			if effort == "" {
				effort = "default"
			}
			fe[effort] = true
		}
		if !dashboardMatches(l, q) {
			return nil
		}
		if !current {
			if !q.Compare || l.CreatedAt >= q.PreviousEnd.Unix() {
				return nil
			}
			r.Previous.add(l)
			dashboardGroup(pm, l.ModelName, false).add(l)
			if b := findDashboardBucket(previousBuckets, l.CreatedAt); b != nil {
				b.add(l)
			}
			return nil
		}
		r.Totals.add(l)
		dashboardGroup(models, l.ModelName, false).add(l)
		dashboardGroup(keys, l.TokenName, true).add(l)
		if b := findDashboardBucket(r.Daily, l.CreatedAt); b != nil {
			b.add(l)
		}
		if b := findDashboardBucket(r.Buckets, l.CreatedAt); b != nil {
			b.add(l)
			if b.Models == nil {
				b.Models = map[string]*DashboardUsageTotals{}
			}
			s := b.Models[l.ModelName]
			if s == nil {
				s = &DashboardUsageTotals{}
				b.Models[l.ModelName] = s
			}
			s.add(l)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	r.Totals.finish()
	if r.Previous != nil {
		r.Previous.finish()
	}
	for _, b := range r.Daily {
		b.finish()
	}
	for i, b := range r.Buckets {
		b.finish()
		for _, m := range b.Models {
			m.finish()
		}
		if i < len(previousBuckets) {
			previousBuckets[i].finish()
			b.Previous = &previousBuckets[i].DashboardUsageTotals
			b.PreviousStart = previousBuckets[i].Start
			b.PreviousEnd = previousBuckets[i].End
		}
	}
	for _, m := range models {
		m.finish()
		if p := pm[m.ModelName]; p != nil {
			p.finish()
			m.Previous = &p.DashboardUsageTotals
		}
		r.Models = append(r.Models, m)
	}
	for _, k := range keys {
		k.finish()
		r.Keys = append(r.Keys, k)
	}
	sortDashboardGroups(r.Models)
	sortDashboardGroups(r.Keys)
	r.Filters = DashboardFilters{sortedDashboardKeys(fm), sortedDashboardKeys(ft), sortedDashboardKeys(fe)}
	return r, nil
}
func sortedDashboardKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func dashboardGroup(m map[string]*DashboardUsageModel, name string, key bool) *DashboardUsageModel {
	s := m[name]
	if s == nil {
		s = &DashboardUsageModel{ModelName: name}
		if key {
			s.TokenName = name
		}
		m[name] = s
	}
	return s
}
func sortDashboardGroups(a []*DashboardUsageModel) {
	sort.Slice(a, func(i, j int) bool {
		if a[i].Quota == a[j].Quota {
			return a[i].ModelName < a[j].ModelName
		}
		return a[i].Quota > a[j].Quota
	})
}
func dashboardBuckets(start, end time.Time, unit string, calendar bool) []*DashboardUsageDay {
	out := []*DashboardUsageDay{}
	cursor := start
	for cursor.Before(end) {
		var next time.Time
		switch unit {
		case "hour":
			next = cursor.Add(time.Hour)
		case "week":
			next = cursor.AddDate(0, 0, 7)
		default:
			next = cursor.AddDate(0, 0, 1)
		}
		if calendar {
			next = time.Date(cursor.Year(), cursor.Month(), cursor.Day()+1, 0, 0, 0, 0, cursor.Location())
		}
		if next.After(end) {
			next = end
		}
		out = append(out, &DashboardUsageDay{Date: cursor.Format("2006-01-02"), Start: cursor.Unix(), End: next.Unix()})
		cursor = next
	}
	return out
}
func findDashboardBucket(b []*DashboardUsageDay, t int64) *DashboardUsageDay {
	i := sort.Search(len(b), func(i int) bool { return b[i].End > t })
	if i < len(b) && b[i].Start <= t {
		return b[i]
	}
	return nil
}
func (s *DashboardUsageTotals) add(l *Log) {
	if l.Type == LogTypeError {
		s.Errors++
		return
	}
	s.Requests++
	s.Quota += int64(l.Quota)
	s.PromptTokens += int64(l.PromptTokens)
	s.CompletionTokens += int64(l.CompletionTokens)
	m := l.Metadata.Data()
	s.CachedReadTokens += metadataCachedReadTokens(m)
	s.CachedWriteTokens += metadataInt64(m, "cached_write_tokens") + metadataInt64(m, "cached_write_1h_tokens") + metadataInt64(m, "openai_cache_write_tokens")
	if duration := LogResponseDurationMs(l); duration > 0 {
		s.durations = append(s.durations, duration)
	}
	first := LogFirstResponseMs(l)
	if first > 0 {
		s.firsts = append(s.firsts, first)
	}
	if tokens, duration := LogOutputSpeedSample(l); duration > 0 {
		s.speeds = append(s.speeds, float64(tokens)*1000/duration)
	}
	if sRead := metadataCachedReadTokens(m) + metadataInt64(m, "cached_write_tokens") + metadataInt64(m, "cached_write_1h_tokens") + metadataInt64(m, "openai_cache_write_tokens"); sRead > 0 {
		s.CacheRequests++
		if saved, ok := dashboardCacheSavings(l); ok {
			s.CachePricedRequests++
			s.CacheSavingsQuota += saved
		}
	}
}
func (s *DashboardUsageTotals) finish() {
	s.PerformanceSamples = len(s.firsts)
	s.DurationSamples = len(s.durations)
	s.SpeedSamples = len(s.speeds)
	s.FirstP50 = dashboardPercentile(s.firsts, .5)
	s.FirstP95 = dashboardPercentile(s.firsts, .95)
	s.DurationP95 = dashboardPercentile(s.durations, .95)
	s.OutputSpeed = dashboardPercentile(s.speeds, .5)
	if len(s.durations) > 0 {
		var sum float64
		for _, v := range s.durations {
			sum += v
		}
		s.AverageLatencyMs = roundMetric(sum / float64(len(s.durations)))
	}
	if s.Requests+s.Errors > 0 {
		s.ErrorRate = roundMetric(float64(s.Errors) * 100 / float64(s.Requests+s.Errors))
	}
	s.CacheHitRate = cacheHitRate(s.CachedReadTokens, s.PromptTokens)
}
func dashboardPercentile(a []float64, p float64) *float64 {
	if len(a) == 0 {
		return nil
	}
	sort.Float64s(a)
	pos := float64(len(a)-1) * p
	lo := int(pos)
	hi := int(math.Ceil(pos))
	v := roundMetric(a[lo] + (a[hi]-a[lo])*(pos-float64(lo)))
	return &v
}

// Uses historical billing snapshots and the same integer token adjustments as
// GetIncreaseTokens. It is an estimate before final quota rounding, not a refund.
func dashboardCacheSavings(l *Log) (float64, bool) {
	m := l.Metadata.Data()
	if m["price_type"] != TokensPriceType || l.Quota <= 0 {
		return 0, false
	}
	if _, ok := m["input_ratio"]; !ok {
		return 0, false
	}
	if _, ok := m["group_ratio"]; !ok && m["effective_ratio"] == nil {
		return 0, false
	}
	ratio := metadataNumber(m, "input_ratio") * LogBillingRatio(m)
	for _, key := range []string{"long_context_input_ratio"} {
		if _, ok := m[key]; ok {
			ratio *= metadataNumber(m, key)
		}
	}
	var adjustment int
	for _, key := range []string{"cached_tokens", "cached_read_tokens", "cached_write_tokens", "cached_write_1h_tokens", "openai_cache_write_tokens"} {
		if key == "cached_tokens" && metadataInt64(m, "cached_read_tokens") > 0 {
			continue
		}
		tokens := metadataInt64(m, key)
		if tokens <= 0 {
			continue
		}
		if _, ok := m[key+"_ratio"]; !ok {
			return 0, false
		}
		adjustment += GetIncreaseTokens(int(tokens), metadataNumber(m, key+"_ratio"))
	}
	return -float64(adjustment) * ratio, true
}
func metadataNumber(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case json.Number:
		n, _ := v.Float64()
		return n
	}
	return 0
}
func metadataInt64(m map[string]any, k string) int64 { return int64(metadataNumber(m, k)) }
func metadataCachedReadTokens(m map[string]any) int64 {
	if n := metadataInt64(m, "cached_read_tokens"); n > 0 {
		return n
	}
	return metadataInt64(m, "cached_tokens")
}
func cacheHitRate(read, prompt int64) float64 {
	if read <= 0 || prompt <= 0 {
		return 0
	}
	return math.Min(100, roundMetric(float64(read)*100/float64(prompt)))
}
func roundMetric(v float64) float64 { return math.Round(v*10) / 10 }

// Error logs before request_time_unit=ms were recorded in seconds.
func LogRequestTimeMs(l *Log) int {
	if l.Type == LogTypeError && l.Metadata.Data()["request_time_unit"] != "ms" {
		return l.RequestTime * 1000
	}
	return l.RequestTime
}

type DashboardRequest struct {
	ID               int            `json:"id"`
	CreatedAt        int64          `json:"created_at"`
	ModelName        string         `json:"model_name"`
	TokenName        string         `json:"token_name"`
	Type             int            `json:"type"`
	Status           int            `json:"status"`
	IsStream         bool           `json:"is_stream"`
	RequestTime      int            `json:"request_time"`
	Quota            int            `json:"quota"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	Metadata         map[string]any `json:"metadata"`
}
type DashboardRequests struct {
	Items []DashboardRequest `json:"items"`
	Total int                `json:"total"`
}

func GetDashboardRequests(ctx context.Context, userID int, q DashboardQuery, page, size int, export func(*Log) error) (*DashboardRequests, error) {
	result := &DashboardRequests{Items: []DashboardRequest{}}
	err := scanDashboardLogs(ctx, userID, q, q.Start, q.End, func(l *Log) error {
		if !dashboardMatches(l, q) {
			return nil
		}
		index := result.Total
		result.Total++
		if export != nil {
			return export(l)
		}
		if index < page*size || index >= (page+1)*size {
			return nil
		}
		status := 200
		if l.Type == LogTypeError {
			status = int(metadataInt64(l.Metadata.Data(), "status_code"))
		}
		result.Items = append(result.Items, DashboardRequest{l.Id, l.CreatedAt, l.ModelName, l.TokenName, l.Type, status, l.IsStream, LogRequestTimeMs(l), l.Quota, l.PromptTokens, l.CompletionTokens, l.Metadata.Data()})
		return nil
	})
	return result, err
}
