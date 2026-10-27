package model

import (
	"context"
	"done-hub/internal/gateway/requeststate"
	"math"
)

// LogOutputSpeedSample excludes reasoning tokens and first-response latency.
// Legacy stream timings use the request-entry clock for both endpoints. Logs
// without a usable first response fall back to their complete recorded duration.
func LogOutputSpeedSample(l *Log) (tokens int64, durationMs float64) {
	if l == nil || l.Type != LogTypeConsume {
		return
	}
	m := l.Metadata.Data()
	reasoning := max(metadataInt64(m, "reasoning_tokens"), 0)
	tokens = max(int64(l.CompletionTokens)-reasoning, 0)
	durationMs = LogResponseDurationMs(l)
	if first := LogFirstResponseMs(l); durationMs > 0 && first > 0 {
		if metadataNumber(m, "response_metrics_version") < 2 {
			durationMs = float64(LogRequestTimeMs(l))
		}
		durationMs -= first
	}
	if tokens <= 0 || durationMs <= 0 || math.IsNaN(durationMs) || math.IsInf(durationMs, 0) {
		return 0, 0
	}
	return
}

// LogResponseDurationMs keeps the end-to-end request_time unchanged for history/export.
func LogResponseDurationMs(l *Log) float64 {
	if l == nil {
		return 0
	}
	m := l.Metadata.Data()
	duration := float64(LogRequestTimeMs(l))
	if _, measured := m["upstream_response_ms"]; measured {
		duration = metadataNumber(m, "upstream_response_ms")
	}
	if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0
	}
	return duration
}

// New first-response timings start at HTTP send. Older stream logs retain their
// recorded request-entry timing, rather than inventing an unavailable first byte.
func LogFirstResponseMs(l *Log) float64 {
	if l == nil {
		return 0
	}
	m := l.Metadata.Data()
	first, limit := metadataNumber(m, "first_response"), float64(LogRequestTimeMs(l))
	if metadataNumber(m, "response_metrics_version") >= 2 {
		first, limit = metadataNumber(m, "response_first_ms"), LogResponseDurationMs(l)
	} else if !l.IsStream {
		return 0
	}
	if first <= 0 || first > limit || math.IsNaN(first) || math.IsInf(first, 0) {
		return 0
	}
	return first
}

// LogBillingRatio reads the immutable effective price snapshot; legacy logs keep
// their group/service-tier pricing even after channel settings change.
func LogBillingRatio(m map[string]any) float64 {
	if _, ok := m["effective_ratio"]; ok {
		return metadataNumber(m, "effective_ratio")
	}
	ratio := 1.0
	for _, key := range []string{"group_ratio", "service_tier_ratio"} {
		if _, ok := m[key]; ok {
			ratio *= metadataNumber(m, key)
		}
	}
	return ratio
}

// SetLogResponseMetrics is shared by final request logging and late usage settlement.
func SetLogResponseMetrics(ctx context.Context, metadata map[string]any) {
	s := requeststate.From(ctx)
	if started, last, duration := s.UpstreamResponseTiming(); started > 0 {
		metadata["upstream_started_at"] = started
		metadata["upstream_last_byte_at"] = last
		metadata["upstream_response_ms"] = duration
		metadata["response_metrics_version"] = 2
		metadata["response_first_ms"] = s.UpstreamFirstResponseMs()
	}
}
