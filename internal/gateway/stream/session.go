package stream

import (
	"errors"
	"sync"
	"time"

	"done-hub/internal/gateway/domain"
)

type State uint8

const (
	StateNotStarted State = iota
	StateHeadersSent
	StateDataSent
	StateTerminal
	StateClosed
)

var (
	ErrInvalidTransition = errors.New("invalid stream state transition")
	ErrStreamClosed      = errors.New("stream session is closed")
)

type Snapshot struct {
	State       State
	Bytes       int64
	FirstByteAt time.Time
	Usage       domain.Usage
	Accepted    bool
	TerminalErr error
}

type Session struct {
	mu          sync.RWMutex
	state       State
	bytes       int64
	firstByteAt time.Time
	usage       domain.Usage
	accepted    bool
	terminalErr error
}

func NewSession() *Session {
	return &Session{state: StateNotStarted}
}

func (s *Session) BeginHeaders() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateNotStarted {
		return ErrInvalidTransition
	}
	s.state = StateHeadersSent
	return nil
}

func (s *Session) RecordData(size int) error {
	if size < 0 {
		return ErrInvalidTransition
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	switch s.state {
	case StateNotStarted:
		s.state = StateDataSent
	case StateHeadersSent, StateDataSent:
		s.state = StateDataSent
	case StateTerminal:
		return ErrInvalidTransition
	case StateClosed:
		return ErrStreamClosed
	}
	if s.firstByteAt.IsZero() {
		s.firstByteAt = time.Now()
	}
	s.bytes += int64(size)
	return nil
}

func (s *Session) AddUsage(delta domain.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateClosed {
		return ErrStreamClosed
	}
	s.usage = s.usage.Add(delta)
	return nil
}

// MarkAccepted records an irreversible upstream side effect, such as an
// asynchronous task ID or an accepted file upload. Accepted attempts may not
// fail over even when no client-visible bytes have been written.
func (s *Session) MarkAccepted() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateClosed {
		return ErrStreamClosed
	}
	s.accepted = true
	return nil
}

func (s *Session) Terminate(err error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateClosed {
		return ErrStreamClosed
	}
	if s.state == StateTerminal {
		return nil
	}
	s.state = StateTerminal
	s.terminalErr = err
	return nil
}

func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateClosed {
		return nil
	}
	if s.state != StateTerminal {
		s.state = StateTerminal
	}
	s.state = StateClosed
	return nil
}

func (s *Session) CanFailover() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == StateNotStarted && !s.accepted
}

func (s *Session) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Snapshot{
		State:       s.state,
		Bytes:       s.bytes,
		FirstByteAt: s.firstByteAt,
		Usage:       s.usage,
		Accepted:    s.accepted,
		TerminalErr: s.terminalErr,
	}
}
