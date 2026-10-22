package model

import "math"

// LogOutputSpeedSample uses non-reasoning output and the complete upstream
// response duration. Older logs fall back to the recorded total request time.
// First response remains a separate latency metric and is never subtracted.
func LogOutputSpeedSample(l *Log) (tokens int64, durationMs float64) {
	if l == nil || l.Type != LogTypeConsume {
		return
	}
	m := l.Metadata.Data()
	reasoning := max(metadataInt64(m, "reasoning_tokens"), 0)
	tokens = max(int64(l.CompletionTokens)-reasoning, 0)
	durationMs = float64(l.RequestTime)
	if _, measured := m["upstream_response_ms"]; measured {
		durationMs = metadataNumber(m, "upstream_response_ms")
	}
	if tokens <= 0 || durationMs <= 0 || math.IsNaN(durationMs) || math.IsInf(durationMs, 0) {
		return 0, 0
	}
	return
}
