package requeststate

import (
	"testing"
	"time"
)

func TestUpstreamTimingTracksFinalAttemptOnly(t *testing.T) {
	s := &State{}
	previous := s.BeginUpstreamRequest()
	previous.RecordData()
	s.SetStreamSession(nil)
	if started, _, _ := s.UpstreamResponseTiming(); started != 0 {
		t.Fatal("retry inherited timing")
	}
	current := s.BeginUpstreamRequest()
	base := time.Now()
	current.started = base
	current.last = base.Add(2879 * time.Millisecond)
	previous.RecordData() // A delayed old body cannot overwrite the current request.
	start, end, ms := s.UpstreamResponseTiming()
	if ms != 2879 || end-start != 2879 {
		t.Fatal(start, end, ms)
	}
}
