package requester

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"done-hub/internal/gateway/requeststate"
)

type timingTransport func(*http.Request) (*http.Response, error)

func (f timingTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUpstreamTimingIncludesInitialWaitAndStopsAtLastData(t *testing.T) {
	oldClient := HTTPClient
	defer func() { HTTPClient = oldClient }()
	incoming, state := requeststate.Ensure(httptest.NewRequest("POST", "/v1/chat/completions", nil))
	HTTPClient = &http.Client{Transport: timingTransport(func(req *http.Request) (*http.Response, error) {
		if started, _, _ := state.UpstreamResponseTiming(); started == 0 {
			t.Error("timing started after send")
		}
		time.Sleep(20 * time.Millisecond) // Upstream thinking/waiting must be included.
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: hello\n\n")), Header: make(http.Header)}, nil
	})}
	r := NewHTTPRequester("", nil)
	r.Context = incoming.Context()
	req, _ := r.NewRequest("POST", "http://upstream/v1/chat/completions")
	resp, err := r.SendRequestRaw(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 100)
	n, readErr := resp.Body.Read(buf)
	if readErr != nil || string(buf[:n]) != "data: hello\n\n" {
		t.Fatal(n, readErr)
	}
	_, last, duration := state.UpstreamResponseTiming()
	if state.UpstreamFirstResponseMs() != duration {
		t.Fatal("first/body duration clocks differ")
	}
	if duration < 20 {
		t.Fatalf("initial upstream wait excluded: %v", duration)
	}
	time.Sleep(20 * time.Millisecond) // EOF/close/client-side work must not extend the timer.
	_, _ = resp.Body.Read(buf)
	_, afterLast, afterDuration := state.UpstreamResponseTiming()
	if last != afterLast || duration != afterDuration {
		t.Fatal("EOF extended response duration")
	}

	// Unrelated OAuth calls must not replace the inference request timing.
	HTTPClient.Transport = timingTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})
	req, _ = r.NewRequest("POST", "http://upstream/oauth/token")
	var out map[string]any
	if _, e := r.SendRequest(req, &out, false); e != nil {
		t.Fatal(e)
	}
	_, afterLast, afterDuration = state.UpstreamResponseTiming()
	if last != afterLast || duration != afterDuration {
		t.Fatal("OAuth overwrote inference timing")
	}

	req, _ = r.NewRequest("POST", "http://upstream/v1/responses")
	if _, e := r.SendRequest(req, &out, false); e != nil {
		t.Fatal(e)
	}
	if _, last, duration := state.UpstreamResponseTiming(); last == 0 || duration <= 0 {
		t.Fatal("nonstream response not timed")
	}
}
