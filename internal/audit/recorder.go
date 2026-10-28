package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	ChunkSize        = 32 << 10
	MaxRequestBytes  = 4 << 20
	MaxResponseBytes = 8 << 20
	MaxBufferedBytes = 32 << 20
	MaxQueueBytes    = 32 << 20
)

// Data is encoded by encoding/json as Base64, preserving arbitrary bytes,
// including SSE whitespace, invalid UTF-8, multipart and binary responses.
type Event struct {
	Time          time.Time   `json:"time"`
	RequestID     string      `json:"request_id"`
	TokenID       int         `json:"token_id"`
	ChannelID     int         `json:"channel_id,omitempty"`
	Attempt       int         `json:"attempt,omitempty"`
	Exchange      int         `json:"exchange,omitempty"`
	Type          string      `json:"type"`
	Seq           int         `json:"seq,omitempty"`
	Data          []byte      `json:"data,omitempty"`
	Method        string      `json:"method,omitempty"`
	Path          string      `json:"path,omitempty"`
	Headers       http.Header `json:"headers,omitempty"`
	Status        int         `json:"status,omitempty"`
	Outcome       string      `json:"outcome,omitempty"`
	ObservedBytes int64       `json:"observed_bytes,omitempty"`
	CapturedBytes int64       `json:"captured_bytes,omitempty"`
	Incomplete    bool        `json:"incomplete,omitempty"`
	Reason        string      `json:"reason,omitempty"`
}

type queuedEvent struct {
	event Event
	cost  int64
}

type Recorder struct {
	queue       chan queuedEvent
	mu          sync.RWMutex // Only coordinates nonblocking enqueue with shutdown.
	closed      bool
	done        chan struct{}
	queued      atomic.Int64
	buffered    atomic.Int64
	dropped     atomic.Uint64
	writeErrors atomic.Uint64
	truncated   atomic.Uint64
	lastError   atomic.Pointer[string]
	writer      io.Writer
	service     atomic.Pointer[Service]
}

type Stats struct {
	QueueBytes      int64  `json:"queue_bytes"`
	BufferedBytes   int64  `json:"buffered_bytes"`
	DroppedEvents   uint64 `json:"dropped_events"`
	WriteErrors     uint64 `json:"write_errors"`
	TruncatedBodies uint64 `json:"truncated_bodies"`
	LastError       string `json:"last_error"`
}

func NewRecorder(writer io.Writer) *Recorder {
	r := &Recorder{queue: make(chan queuedEvent, 1024), done: make(chan struct{}), writer: writer}
	go r.run()
	return r
}

func reserve(counter *atomic.Int64, n, limit int64) bool {
	for {
		previous := counter.Load()
		if previous+n > limit {
			return false
		}
		if counter.CompareAndSwap(previous, previous+n) {
			return true
		}
	}
}

// enqueue owns the copy: Read buffers may be immediately reused by net/http.
func (r *Recorder) enqueue(event Event) bool {
	cost := int64(1024 + len(event.Data) + len(event.Path) + len(event.RequestID))
	for key, values := range event.Headers {
		cost += int64(len(key))
		for _, value := range values {
			cost += int64(len(value))
		}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed || !reserve(&r.queued, cost, MaxQueueBytes) {
		r.dropped.Add(1)
		return false
	}
	event.Time = time.Now().UTC()
	event.Data = append([]byte(nil), event.Data...)
	select {
	case r.queue <- queuedEvent{event, cost}:
		return true
	default:
		r.queued.Add(-cost)
		r.dropped.Add(1)
		return false
	}
}

func (r *Recorder) run() {
	defer close(r.done)
	if closer, ok := r.writer.(io.Closer); ok {
		defer r.writerOperation(closer.Close)
	}
	var ticks <-chan time.Time
	maintenance, maintains := r.writer.(interface{ Maintain(time.Time) error })
	if maintains {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		ticks = ticker.C
	}
	for {
		select {
		case item, ok := <-r.queue:
			if !ok {
				return
			}
			r.write(item.event)
			r.queued.Add(-item.cost)
		case now := <-ticks:
			r.writerOperation(func() error {
				r.configureWriter()
				return maintenance.Maintain(now)
			})
		}
	}
}

// Configuration publication never waits for the disk. File lifecycle changes
// run on this same worker, including idle time rotation and shutdown sealing.
func (r *Recorder) configureWriter() {
	if writer, ok := r.writer.(interface{ SetArchiveEnabled(bool) }); ok {
		enabled := false
		if service := r.service.Load(); service != nil {
			enabled = service.config.Load().ArchiveEnabled
		}
		writer.SetArchiveEnabled(enabled)
	}
}

func (r *Recorder) writerOperation(operation func() error) {
	defer func() {
		if recover() != nil {
			r.recordError("审计 writer panic")
		}
	}()
	if err := operation(); err != nil {
		r.recordError(err.Error())
	}
}

func (r *Recorder) write(event Event) {
	// A failing or panicking audit writer must not stop serving requests.
	defer func() {
		if recover() != nil {
			r.recordError("审计 writer panic")
		}
	}()
	r.configureWriter()
	data, err := json.Marshal(event)
	if err == nil {
		if r.writer == nil {
			err = fmt.Errorf("审计 writer 未初始化")
		} else {
			data = append(data, '\n')
			var n int
			n, err = r.writer.Write(data)
			if err == nil && n != len(data) {
				err = io.ErrShortWrite
			}
		}
	}
	if err != nil {
		r.recordError(err.Error())
	}
}

func (r *Recorder) recordError(message string) {
	r.writeErrors.Add(1)
	if len(message) > 512 {
		message = message[:512]
	}
	r.lastError.Store(&message)
}

func (r *Recorder) Status() Stats {
	s := Stats{QueueBytes: r.queued.Load(), BufferedBytes: r.buffered.Load(), DroppedEvents: r.dropped.Load(), WriteErrors: r.writeErrors.Load(), TruncatedBodies: r.truncated.Load()}
	if message := r.lastError.Load(); message != nil {
		s.LastError = *message
	}
	return s
}

// Close has a caller-supplied deadline: a stalled disk cannot hold shutdown.
func (r *Recorder) Close(ctx context.Context) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		close(r.queue)
	}
	r.mu.Unlock()
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
