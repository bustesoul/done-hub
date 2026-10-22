package model

import (
	"math"
	"testing"

	"gorm.io/datatypes"
)

func TestOutputSpeedUsesTotalDurationAndExcludesReasoning(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata map[string]any
		want     float64
	}{
		{"reported historical log", map[string]any{"first_response": 2863, "reasoning_tokens": 64}, 1000.0 / 2879},
		{"measured upstream", map[string]any{"first_response": 2863, "reasoning_tokens": 64, "upstream_response_ms": 2000}, 0.5},
		{"first response irrelevant", map[string]any{"first_response": 1, "reasoning_tokens": 64, "upstream_response_ms": 2000}, 0.5},
		{"no reasoning", map[string]any{"upstream_response_ms": 2000}, 32.5},
		{"only reasoning", map[string]any{"reasoning_tokens": 65}, 0},
		{"invalid reasoning", map[string]any{"reasoning_tokens": 100}, 0},
		{"invalid timing must not fallback", map[string]any{"upstream_response_ms": 0}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			l := Log{Type: LogTypeConsume, RequestTime: 2879, CompletionTokens: 65, Metadata: datatypes.NewJSONType(test.metadata)}
			for _, stream := range []bool{false, true} {
				l.IsStream = stream
				tokens, duration := LogOutputSpeedSample(&l)
				var got float64
				if duration > 0 {
					got = float64(tokens) * 1000 / duration
				}
				if math.Abs(got-test.want) > 1e-9 {
					t.Fatalf("stream=%v got %v want %v", stream, got, test.want)
				}
				s := ExplorerStats{}
				s.add(&l)
				s.finish(true, true)
				d := DashboardUsageTotals{}
				d.add(&l)
				d.finish()
				if test.want > 0 {
					if s.TPS == nil || math.Abs(*s.TPS-test.want) > 1e-9 || d.OutputSpeed == nil || math.Abs(*d.OutputSpeed-roundMetric(test.want)) > 1e-9 {
						t.Fatal("aggregate TPS differs from per-request calculation", s.TPS, d.OutputSpeed)
					}
				} else if s.TPS != nil || d.OutputSpeed != nil {
					t.Fatal("fabricated output speed")
				}
			}
		})
	}
}
