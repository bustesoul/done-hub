package audit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type sessionKey struct{}

type Session struct {
	mu                        sync.Mutex
	recorder                  *Recorder
	config                    Config
	base                      Event
	request                   []byte
	observed                  int64
	requestOutcome            string
	requestReason             string
	attempt, channelID        int
	active, matched, finished bool
	exchanges                 []*Exchange
	exchangeCount             int
}

// Observe installs a lazy read observer before parsing or parameter mapping.
// A channel-only filter needs a provisional bounded copy until routing is known.
func (s *Service) Observe(req *http.Request, requestID string, tokenID int) (*http.Request, *Session) {
	c := s.config.Load()
	r := s.recorder.Load()
	if r == nil || !c.Enabled() || !c.matchesToken(tokenID) || req.Method == http.MethodConnect ||
		strings.EqualFold(req.Header.Get("Upgrade"), "websocket") {
		return req, nil
	}
	session := &Session{recorder: r, config: *c, base: Event{
		RequestID: requestID, TokenID: tokenID, Method: req.Method,
		Path: safeURL(req.URL), Headers: safeHeaders(req.Header),
	}}
	req = req.WithContext(context.WithValue(req.Context(), sessionKey{}, session))
	if req.Body != nil {
		req.Body = &inboundBody{ReadCloser: req.Body, session: session}
	} else {
		session.requestOutcome = "eof"
	}
	return req, session
}

func fromContext(ctx context.Context) *Session {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(sessionKey{}).(*Session)
	return s
}

// BeginAttempt binds the actual Gateway attempt, rather than the temporary
// provider lookup used by pre_add. All HTTP capabilities share this lifecycle.
func BeginAttempt(ctx context.Context, number, channelID int) func() {
	s := fromContext(ctx)
	if s == nil {
		return func() {}
	}
	s.mu.Lock()
	s.attempt, s.channelID, s.active = number, channelID, true
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.active = false
		exchanges := s.exchanges
		s.exchanges = nil
		s.mu.Unlock()
		for _, exchange := range exchanges {
			exchange.finish("attempt_finished")
		}
	}
}

func BeginHTTP(ctx context.Context) *Exchange {
	s := fromContext(ctx)
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || !s.active || !s.config.Matches(s.channelID, s.base.TokenID) {
		return nil
	}
	s.matched = true
	s.exchangeCount++
	e := &Exchange{recorder: s.recorder, base: Event{
		RequestID: s.base.RequestID, TokenID: s.base.TokenID,
		ChannelID: s.channelID, Attempt: s.attempt, Exchange: s.exchangeCount,
	}}
	s.exchanges = append(s.exchanges, e)
	e.emit("upstream_start", nil)
	return e
}

func (s *Session) capture(data []byte, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.observed += int64(len(data))
	if s.requestReason == "" && len(data) > 0 {
		length := min(len(data), MaxRequestBytes-len(s.request))
		needed := len(s.request) + length
		if needed > cap(s.request) {
			capacity := min(MaxRequestBytes, max(ChunkSize, needed, cap(s.request)*2))
			if reserve(&s.recorder.buffered, int64(capacity-cap(s.request)), MaxBufferedBytes) {
				buffer := make([]byte, len(s.request), capacity)
				copy(buffer, s.request)
				s.request = buffer
			} else {
				s.requestReason = "buffer_limit"
			}
		}
		if s.requestReason == "" {
			s.request = append(s.request, data[:length]...)
			if length < len(data) {
				s.requestReason = "body_limit"
			}
		}
		if s.requestReason != "" {
			s.recorder.truncated.Add(1)
		}
	}
	if err != nil {
		if errors.Is(err, io.EOF) {
			s.requestOutcome = "eof"
		} else {
			s.requestOutcome = "read_error"
		}
	}
}

func (s *Session) Finish() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished {
		return
	}
	s.finished = true
	defer func() {
		s.recorder.buffered.Add(-int64(cap(s.request)))
		s.request = nil
	}()
	for _, e := range s.exchanges {
		e.finish("handler_finished")
	}
	if !s.matched {
		return
	}
	event := s.base
	event.Type = "request_start"
	dropped := !s.recorder.enqueue(event)
	event.Headers, event.Method, event.Path = nil, "", ""
	for offset := 0; offset < len(s.request) && !dropped; offset += ChunkSize {
		event.Type = "request_body"
		event.Seq++
		event.Data = s.request[offset:min(offset+ChunkSize, len(s.request))]
		dropped = !s.recorder.enqueue(event)
	}
	event.Type, event.Data = "request_end", nil
	event.ObservedBytes, event.CapturedBytes = s.observed, int64(len(s.request))
	event.Outcome, event.Reason = s.requestOutcome, s.requestReason
	if event.Outcome == "" {
		event.Outcome = "handler_finished"
	}
	if dropped {
		event.Reason = "queue_full"
	}
	event.Incomplete = event.Outcome != "eof" || event.Reason != ""
	s.recorder.enqueue(event)
}

type inboundBody struct {
	io.ReadCloser
	session *Session
}

func (b *inboundBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.session.capture(p[:n], err)
	return n, err
}

func (b *inboundBody) Close() error {
	err := b.ReadCloser.Close()
	b.session.mu.Lock()
	if b.session.requestOutcome == "" {
		b.session.requestOutcome = "closed"
		if err != nil {
			b.session.requestOutcome = "close_error"
		}
	}
	b.session.mu.Unlock()
	return err
}

type Exchange struct {
	mu                 sync.Mutex
	recorder           *Recorder
	base               Event
	seq                int
	observed, captured int64
	finished           bool
	reason             string
}

func (e *Exchange) emit(kind string, data []byte) {
	event := e.base
	event.Type, event.Seq, event.Data = kind, e.seq, data
	if !e.recorder.enqueue(event) && e.reason == "" {
		e.reason = "queue_full"
	}
}

// Response must be called even when sending fails, to retain transport errors.
func (e *Exchange) Response(resp *http.Response, err error) {
	if e == nil {
		return
	}
	if err != nil {
		e.finish("transport_error")
		return
	}
	e.mu.Lock()
	if e.finished {
		e.mu.Unlock()
		return
	}
	e.base.Status, e.base.Headers = resp.StatusCode, safeHeaders(resp.Header)
	e.emit("response_start", nil)
	e.base.Headers = nil
	e.mu.Unlock()
	if resp.Body == nil {
		e.finish("eof")
		return
	}
	resp.Body = &responseBody{ReadCloser: resp.Body, exchange: e}
}

func (e *Exchange) capture(data []byte, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.finished {
		return
	}
	e.observed += int64(len(data))
	for len(data) > 0 && e.reason == "" {
		length := min(len(data), ChunkSize, MaxResponseBytes-int(e.captured))
		if length == 0 {
			e.reason = "body_limit"
			e.recorder.truncated.Add(1)
			break
		}
		e.seq++
		e.emit("response_body", data[:length])
		if e.reason == "" {
			e.captured += int64(length)
		}
		data = data[length:]
	}
	if err != nil {
		outcome := "read_error"
		if errors.Is(err, io.EOF) {
			outcome = "eof"
		}
		e.finishLocked(outcome)
	}
}

func (e *Exchange) finish(outcome string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.finishLocked(outcome)
}

func (e *Exchange) finishLocked(outcome string) {
	if e.finished {
		return
	}
	e.finished = true
	event := e.base
	event.Type, event.Seq = "response_end", e.seq
	event.ObservedBytes, event.CapturedBytes = e.observed, e.captured
	event.Outcome, event.Reason = outcome, e.reason
	event.Incomplete = outcome != "eof" || e.reason != ""
	e.recorder.enqueue(event)
}

type responseBody struct {
	io.ReadCloser
	exchange *Exchange
}

func (b *responseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.exchange.capture(p[:n], err)
	return n, err
}

func (b *responseBody) Close() error {
	err := b.ReadCloser.Close()
	outcome := "closed"
	if err != nil {
		outcome = "close_error"
	}
	b.exchange.finish(outcome)
	return err
}

// Only operational headers are retained. In particular Authorization, cookies,
// API keys, provider signatures and arbitrary custom headers are never logged.
func safeHeaders(headers http.Header) http.Header {
	out := make(http.Header)
	for _, key := range []string{"Content-Type", "Content-Encoding", "Accept", "User-Agent", "X-Request-Id", "Request-Id", "X-Amzn-Requestid"} {
		if value := headers.Get(key); value != "" {
			out.Set(key, clip(value))
		}
	}
	return out
}

func clip(value string) string {
	if len(value) > 4096 {
		return value[:4096]
	}
	return value
}

func safeURL(u *url.URL) string {
	query := u.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "signature") || lower == "authorization" {
			query.Set(key, "[REDACTED]")
		}
	}
	path := u.EscapedPath()
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return clip(path)
}
