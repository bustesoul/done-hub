package requeststate

import (
	"sync"
	"time"
)

// UpstreamTiming belongs to one outbound inference request. Only the start and
// most recent body-data receipt are retained, not a sequence of chunk timings.
type UpstreamTiming struct {
	mu      sync.RWMutex
	started time.Time
	last    time.Time
}

func (s *State) BeginUpstreamRequest() *UpstreamTiming {
	if s == nil {
		return nil
	}
	timing := &UpstreamTiming{started: time.Now()}
	s.mu.Lock()
	s.upstreamTiming = timing
	s.mu.Unlock()
	return timing
}

func (t *UpstreamTiming) RecordData() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.last = time.Now()
	t.mu.Unlock()
}

func (s *State) UpstreamResponseTiming() (startedAt, lastByteAt int64, durationMs float64) {
	if s == nil {
		return
	}
	s.mu.RLock()
	t := s.upstreamTiming
	s.mu.RUnlock()
	if t == nil {
		return
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	startedAt = t.started.UnixMilli()
	if !t.last.IsZero() {
		lastByteAt = t.last.UnixMilli()
		durationMs = float64(t.last.Sub(t.started)) / float64(time.Millisecond)
	}
	return
}
