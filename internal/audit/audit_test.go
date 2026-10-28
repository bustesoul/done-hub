package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func drain(t *testing.T, r *Recorder, output *bytes.Buffer) []Event {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	var events []Event
	decoder := json.NewDecoder(output)
	for decoder.More() {
		var event Event
		if err := decoder.Decode(&event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func testService() (*Service, *Recorder, *bytes.Buffer) {
	output := &bytes.Buffer{}
	r := NewRecorder(output)
	s := NewService()
	s.SetRecorder(r)
	return s, r, output
}

func TestConfigIntersectionAndValidation(t *testing.T) {
	for _, item := range []struct {
		config         Config
		channel, token int
		match          bool
	}{
		{Config{}, 1, 2, false},
		{Config{ChannelIDs: []int{1}}, 1, 2, true},
		{Config{TokenIDs: []int{2}}, 1, 2, true},
		{Config{ChannelIDs: []int{1}, TokenIDs: []int{2}}, 1, 2, true},
		{Config{ChannelIDs: []int{1}, TokenIDs: []int{2}}, 3, 2, false},
		{Config{ChannelIDs: []int{1}, TokenIDs: []int{2}}, 1, 3, false},
		{Config{ChannelIDs: []int{1}}, 1, 0, false},
	} {
		if got := item.config.Matches(item.channel, item.token); got != item.match {
			t.Errorf("%+v: got %v", item, got)
		}
	}
	for _, value := range []string{"", "null", "[]", `{"token_ids":[0]}`, `{"channel_ids":[-1]}`, `{"token_ids":["sk-secret"]}`, `{"archive_enabled":"true"}`, `{"enabled":true}`, `{} {}`} {
		if _, err := ParseConfig(value); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	c, err := ParseConfig(`{"channel_ids":[3,1,3],"token_ids":null}`)
	if err != nil || len(c.ChannelIDs) != 2 || c.ChannelIDs[0] != 1 || c.TokenIDs == nil {
		t.Fatal(c, err)
	}
}

func TestDisabledAndSnapshots(t *testing.T) {
	s, r, output := testService()
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("original"))
	originalBody := req.Body
	if observed, session := s.Observe(req, "off", 2); session != nil || observed != req || req.Body != originalBody {
		t.Fatal("disabled observer changed request")
	}
	s.Apply(Config{ChannelIDs: []int{1}, TokenIDs: []int{2}})
	req, session := s.Observe(req, "inflight", 2)
	s.Apply(Config{}) // disabling must not split an existing request
	_, _ = io.ReadAll(req.Body)
	finish := BeginAttempt(req.Context(), 1, 1)
	e := BeginHTTP(req.Context())
	if e == nil {
		t.Fatal("snapshot changed in flight")
	}
	e.Response(nil, errors.New("dial failed"))
	finish()
	session.Finish()
	if _, next := s.Observe(httptest.NewRequest("POST", "/", nil), "next", 2); next != nil {
		t.Fatal("hot disable not effective")
	}
	events := drain(t, r, output)
	if len(events) == 0 {
		t.Fatal("inflight request missing")
	}
	if r.Status().BufferedBytes != 0 {
		t.Fatal("request copy leaked")
	}
}

func TestRawBytesRetryAttributionAndSecrets(t *testing.T) {
	s, r, output := testService()
	s.Apply(Config{ChannelIDs: []int{25}, TokenIDs: []int{7}})
	raw := []byte(" {\"model\": \"original\", \"input\": \"hello\"} \n")
	req := httptest.NewRequest("POST", "/v1/responses?key=private-query&mode=raw", bytes.NewReader(raw))
	req.Header.Set("Authorization", "Bearer private-auth")
	req.Header.Set("X-Api-Key", "private-header")
	req.Header.Set("Cookie", "private-cookie")
	req.Header.Set("Content-Type", "application/json")
	req, session := s.Observe(req, "same-request", 7)
	read, err := io.ReadAll(req.Body)
	if err != nil || !bytes.Equal(read, raw) {
		t.Fatal("observer changed request")
	}
	// First attempt must not match, even though it belongs to a matching token.
	finish := BeginAttempt(req.Context(), 1, 24)
	if BeginHTTP(req.Context()) != nil {
		t.Fatal("union instead of intersection")
	}
	finish()
	finish = BeginAttempt(req.Context(), 2, 25)
	e := BeginHTTP(req.Context())
	sse := []byte("event: message\r\ndata: { \"text\": \"hi\" }\r\n\r\n:heartbeat\n\ndata: [DONE]\n\n\xff")
	resp := &http.Response{StatusCode: 429, Header: http.Header{"Set-Cookie": {"private-upstream"}, "Content-Type": {"text/event-stream"}}, Body: io.NopCloser(bytes.NewReader(sse))}
	e.Response(resp, nil)
	got, err := io.ReadAll(resp.Body)
	if err != nil || !bytes.Equal(got, sse) {
		t.Fatal("observer changed response")
	}
	_ = resp.Body.Close()
	finish()
	session.Finish()
	events := drain(t, r, output)
	var requestData, responseData []byte
	endCount := 0
	for _, event := range events {
		switch event.Type {
		case "request_body":
			requestData = append(requestData, event.Data...)
		case "response_body":
			responseData = append(responseData, event.Data...)
		case "response_end":
			endCount++
			if event.ChannelID != 25 || event.Attempt != 2 || event.Incomplete || event.Status != 429 {
				t.Fatal(event)
			}
		}
		encoded, _ := json.Marshal(event)
		if strings.Contains(string(encoded), "private-") {
			t.Fatalf("secret leaked: %s", encoded)
		}
	}
	if endCount != 1 || !bytes.Equal(requestData, raw) || !bytes.Equal(responseData, sse) {
		t.Fatal("raw bytes or attempt events lost")
	}
}

type partialBody struct {
	reads, closes int
	err           error
}

func (b *partialBody) Read(p []byte) (int, error) { b.reads++; return copy(p, "tail"), b.err }
func (b *partialBody) Close() error               { b.closes++; return b.err }

func TestReadErrorAndEarlyCloseDoNotDrain(t *testing.T) {
	for _, readFirst := range []bool{false, true} {
		s, r, output := testService()
		s.Apply(Config{TokenIDs: []int{7}})
		req, session := s.Observe(httptest.NewRequest("POST", "/v1/chat/completions", nil), "cancel", 7)
		finish := BeginAttempt(req.Context(), 1, 25)
		e := BeginHTTP(req.Context())
		original := &partialBody{err: context.Canceled}
		resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: original}
		e.Response(resp, nil)
		if readFirst {
			p := make([]byte, 10)
			if n, err := resp.Body.Read(p); n != 4 || err != context.Canceled {
				t.Fatal(n, err)
			}
		}
		if err := resp.Body.Close(); err != context.Canceled {
			t.Fatal("close error changed")
		}
		finish()
		session.Finish()
		events := drain(t, r, output)
		ends := 0
		for _, event := range events {
			if event.Type == "response_end" {
				ends++
				if !event.Incomplete {
					t.Fatal("canceled/closed response declared complete")
				}
			}
		}
		if ends != 1 || original.closes != 1 || original.reads != map[bool]int{false: 0, true: 1}[readFirst] {
			t.Fatal("observer drained or double finalized", original)
		}
	}
}

func TestBodyAndGlobalBufferLimitsPreserveBusinessReads(t *testing.T) {
	s, r, output := testService()
	s.Apply(Config{TokenIDs: []int{7}})
	body := strings.Repeat("x", MaxRequestBytes+1)
	req, session := s.Observe(httptest.NewRequest("POST", "/", strings.NewReader(body)), "large", 7)
	got, _ := io.ReadAll(req.Body)
	if string(got) != body {
		t.Fatal("request truncated for business")
	}
	finish := BeginAttempt(req.Context(), 1, 25)
	e := BeginHTTP(req.Context())
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("y", MaxResponseBytes+1)))}
	e.Response(resp, nil)
	// Keep this a body-limit test, independently of writer scheduling under
	// -race. io.Discard.ReadFrom uses 8 KiB reads and can saturate the event
	// queue before this large response reaches its per-body limit.
	if n, err := io.CopyBuffer(struct{ io.Writer }{io.Discard}, resp.Body, make([]byte, ChunkSize)); n != MaxResponseBytes+1 || err != nil {
		t.Fatal("response truncated for business", n, err)
	}
	finish()
	session.Finish()
	events := drain(t, r, output)
	ends := 0
	for _, event := range events {
		if event.Type == "request_end" || event.Type == "response_end" {
			ends++
			if !event.Incomplete || event.Reason != "body_limit" {
				t.Fatal(event)
			}
		}
	}
	if ends != 2 || r.Status().TruncatedBodies != 2 || r.Status().BufferedBytes != 0 {
		t.Fatal(r.Status())
	}

	s, r, output = testService()
	s.Apply(Config{ChannelIDs: []int{25}})
	r.buffered.Store(MaxBufferedBytes)
	req, session = s.Observe(httptest.NewRequest("POST", "/", strings.NewReader("not-buffered")), "budget", 7)
	got, _ = io.ReadAll(req.Body)
	if string(got) != "not-buffered" {
		t.Fatal("global limit changed business body")
	}
	finish = BeginAttempt(req.Context(), 1, 25)
	BeginHTTP(req.Context()).Response(nil, errors.New("transport failed"))
	finish()
	session.Finish()
	for _, event := range drain(t, r, output) {
		if event.Type == "request_end" && event.Reason != "buffer_limit" {
			t.Fatal(event)
		}
	}
	if r.Status().BufferedBytes != MaxBufferedBytes {
		t.Fatal("budget released another request's memory")
	}
}

type blockingWriter struct {
	entered, release chan struct{}
	once             sync.Once
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.release
	return len(p), nil
}

type failingWriter struct{ panic bool }

func (w failingWriter) Write([]byte) (int, error) {
	if w.panic {
		panic("disk panic")
	}
	return 0, errors.New("disk full")
}

func TestBlockedFullQueueFailedWriterAndBoundedShutdown(t *testing.T) {
	w := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	r := NewRecorder(w)
	r.enqueue(Event{Type: "first"})
	<-w.entered
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1500; i++ {
			r.enqueue(Event{Type: "body", Data: []byte("data")})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(w.release)
		t.Fatal("queue blocked business")
	}
	if r.Status().DroppedEvents == 0 || r.Status().QueueBytes > MaxQueueBytes {
		t.Fatal(r.Status())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := r.Close(ctx); err == nil {
		t.Fatal("stalled writer ignored close deadline")
	}
	cancel()
	close(w.release)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if r.Status().QueueBytes != 0 {
		t.Fatal("queue memory leaked")
	}
	for _, panicWriter := range []bool{false, true} {
		r = NewRecorder(failingWriter{panicWriter})
		r.enqueue(Event{Type: "first"})
		r.enqueue(Event{Type: "second"})
		if err := r.Close(ctx); err != nil {
			t.Fatal(err)
		}
		if r.Status().WriteErrors != 2 || r.Status().LastError == "" {
			t.Fatal("failed writer stopped worker", r.Status())
		}
	}
}

func TestFileIsLazyPrivateAndWriteFailureIsContained(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "audit.log")
	w := NewFileWriter(path)
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("disabled created files")
	}
	r := NewRecorder(w)
	r.enqueue(Event{Type: "request_start"})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("audit file permission", info, err)
	}
	// ENOTDIR is deterministic even when tests run as root.
	badWriter := NewFileWriter(filepath.Join(path, "nested.log"))
	r = NewRecorder(badWriter)
	r.enqueue(Event{Type: "request_start"})
	if err := r.Close(ctx); err != nil || r.Status().WriteErrors != 1 {
		t.Fatal(err, r.Status())
	}
}

func TestConcurrentConfigPublicationAndCapture(t *testing.T) {
	s, r, output := testService()
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				s.Apply(Config{ChannelIDs: []int{25}, TokenIDs: []int{7}})
				_ = s.ConfigJSON()
				_ = s.Status()
				req, session := s.Observe(httptest.NewRequest("POST", "/", strings.NewReader("raw")), "concurrent", 7)
				if session != nil {
					_, _ = io.ReadAll(req.Body)
					finish := BeginAttempt(req.Context(), 1, 25)
					if e := BeginHTTP(req.Context()); e != nil {
						e.Response(nil, errors.New("network"))
					}
					finish()
					session.Finish()
				}
				s.Apply(Config{})
			}
		}()
	}
	wg.Wait()
	drain(t, r, output)
	if r.Status().BufferedBytes != 0 {
		t.Fatal("concurrent capture leaked")
	}
}

func TestCompletedAttemptsReleaseExchangesAndKeepUniqueIDs(t *testing.T) {
	s, r, output := testService()
	s.Apply(Config{TokenIDs: []int{7}})
	req, session := s.Observe(httptest.NewRequest("POST", "/v1/chat/completions", nil), "retries", 7)
	for attempt := 1; attempt <= 3; attempt++ {
		finish := BeginAttempt(req.Context(), attempt, 20+attempt)
		exchange := BeginHTTP(req.Context())
		if exchange.base.Exchange != attempt {
			t.Fatal("exchange IDs reused across attempts")
		}
		exchange.Response(nil, errors.New("connection failed"))
		finish()
		if len(session.exchanges) != 0 {
			t.Fatal("finished attempt retained old exchanges")
		}
	}
	session.Finish()
	ends := 0
	for _, event := range drain(t, r, output) {
		if event.Type == "response_end" {
			ends++
			if event.Attempt != ends || event.Exchange != ends || event.ChannelID != 20+ends {
				t.Fatal("retry attribution changed", event)
			}
		}
	}
	if ends != 3 {
		t.Fatal("terminal events lost", ends)
	}
}

type closeErrorBody struct {
	io.Reader
	err    error
	closes int
}

func (b *closeErrorBody) Close() error { b.closes++; return b.err }

func TestCloseErrorsPreserveBusinessErrorAndEarlierEOF(t *testing.T) {
	for _, readToEOF := range []bool{false, true} {
		s, r, output := testService()
		s.Apply(Config{TokenIDs: []int{7}})
		sentinel := errors.New("connection closed with error")
		incoming := &closeErrorBody{Reader: strings.NewReader("client bytes"), err: sentinel}
		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		req.Body = incoming
		req, session := s.Observe(req, "close-error", 7)
		if readToEOF {
			if _, err := io.ReadAll(req.Body); err != nil {
				t.Fatal(err)
			}
		}
		if err := req.Body.Close(); err != sentinel {
			t.Fatal("inbound close error changed", err)
		}
		finish := BeginAttempt(req.Context(), 1, 25)
		exchange := BeginHTTP(req.Context())
		upstream := &closeErrorBody{Reader: strings.NewReader("upstream bytes"), err: sentinel}
		resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: upstream}
		exchange.Response(resp, nil)
		if readToEOF {
			if _, err := io.ReadAll(resp.Body); err != nil {
				t.Fatal(err)
			}
		}
		if err := resp.Body.Close(); err != sentinel {
			t.Fatal("upstream close error changed", err)
		}
		finish()
		session.Finish()
		ends := 0
		for _, event := range drain(t, r, output) {
			if event.Type == "request_end" || event.Type == "response_end" {
				ends++
				outcome := "close_error"
				if readToEOF {
					outcome = "eof"
				}
				if event.Outcome != outcome || event.Incomplete == readToEOF {
					t.Fatal("close replaced earlier EOF or hid error", event)
				}
			}
		}
		if ends != 2 || incoming.closes != 1 || upstream.closes != 1 {
			t.Fatal("duplicate closes or terminal events", ends, incoming.closes, upstream.closes)
		}
	}
}

func TestLateResponseDoesNotEmitAfterAttemptEnd(t *testing.T) {
	s, r, output := testService()
	s.Apply(Config{TokenIDs: []int{7}})
	req, session := s.Observe(httptest.NewRequest("POST", "/", nil), "late-response", 7)
	finish := BeginAttempt(req.Context(), 1, 25)
	exchange := BeginHTTP(req.Context())
	finish()
	body := io.NopCloser(strings.NewReader("late bytes"))
	resp := &http.Response{StatusCode: 200, Header: make(http.Header), Body: body}
	exchange.Response(resp, nil)
	if resp.Body != body {
		t.Fatal("late response body changed")
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil || string(got) != "late bytes" {
		t.Fatal("late business response changed", string(got), err)
	}
	session.Finish()
	for _, event := range drain(t, r, output) {
		if event.Type == "response_start" || event.Type == "response_body" {
			t.Fatal("response event emitted after terminal event", event)
		}
	}
}

func BenchmarkObserveDisabled(b *testing.B) {
	s := NewService()
	r := NewRecorder(io.Discard)
	s.SetRecorder(r)
	req := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader("body"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if observed, session := s.Observe(req, "disabled", 7); observed != req || session != nil {
			b.Fatal("disabled audit changed request")
		}
	}
	b.StopTimer()
	_ = r.Close(context.Background())
}
