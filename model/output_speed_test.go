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

func TestResponseMetricsUseSameAttemptClock(t *testing.T) {
	for _, stream := range []bool{false, true} {
		l := Log{Type: LogTypeConsume, IsStream: stream, RequestTime: 8552, CompletionTokens: 188, Metadata: datatypes.NewJSONType(map[string]any{"response_metrics_version": 2, "response_first_ms": 4820.0, "first_response": 4837, "upstream_response_ms": 8480.628079})}
		if got := LogFirstResponseMs(&l); got != 4820 {
			t.Fatal(got)
		}
		s := ExplorerStats{}
		s.add(&l)
		s.finish(true, true)
		d := DashboardUsageTotals{}
		d.add(&l)
		d.finish()
		if *s.FirstResponseMs != 4820 || math.Abs(*s.LatencyMs-8480.628079) > 1e-6 || *d.FirstP50 != 4820 || d.AverageLatencyMs != 8480.6 {
			t.Fatal("metric consumers disagree")
		}
		if !dashboardMatches(&l, DashboardQuery{MinFirst: 4800}) || dashboardMatches(&l, DashboardQuery{MinFirst: 4830}) {
			t.Fatal("first-response filter uses old clock")
		}
	}
	legacy := Log{Type: LogTypeConsume, IsStream: true, RequestTime: 8552, Metadata: datatypes.NewJSONType(map[string]any{"first_response": 4837, "upstream_response_ms": 8480.628079})}
	if LogFirstResponseMs(&legacy) != 4837 {
		t.Fatal("legacy timing changed")
	}
	invalid := legacy
	invalid.Metadata = datatypes.NewJSONType(map[string]any{"response_metrics_version": 2, "response_first_ms": 9000, "first_response": 4837, "upstream_response_ms": 8480})
	if LogFirstResponseMs(&invalid) != 0 {
		t.Fatal("invalid measured first response fell back to legacy")
	}
}

func TestBillingSnapshotOverridesSalesGroupForSavings(t *testing.T) {
	m := map[string]any{"price_type": "tokens", "input_ratio": 2., "group_ratio": 9., "service_tier_ratio": 1.5, "effective_ratio": .75, "channel_ratio": .5, "cached_read_tokens": 100, "cached_read_tokens_ratio": .1}
	l := Log{Type: LogTypeConsume, Quota: 100, Metadata: datatypes.NewJSONType(m)}
	if got, ok := dashboardCacheSavings(&l); !ok || got != 135 {
		t.Fatalf("new savings=%v valid=%v", got, ok)
	}
	delete(m, "effective_ratio")
	l.Metadata = datatypes.NewJSONType(m)
	if got, ok := dashboardCacheSavings(&l); !ok || got != 2430 {
		t.Fatalf("legacy savings=%v valid=%v", got, ok)
	}
}
