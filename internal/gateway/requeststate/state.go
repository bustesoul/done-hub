package requeststate

import (
	"context"
	"net/http"
	"sync"

	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/registry"
	"done-hub/internal/gateway/stream"
)

type contextKey struct{}

type Selection struct {
	ChannelID            int
	ChannelType          int
	OriginalModel        string
	UpstreamModel        string
	BillingOriginalModel bool
}

type State struct {
	mu sync.RWMutex

	inboundProtocol  domain.Protocol
	protocolRoute    registry.ConverterSpec
	hasProtocolRoute bool
	streamSession    *stream.Session
	attemptCount     int
	skipped          map[int]struct{}
	selection        Selection
	values           map[string]any
	params           map[string]string
}

func Ensure(request *http.Request) (*http.Request, *State) {
	if request == nil {
		return nil, nil
	}
	if state := From(request.Context()); state != nil {
		return request, state
	}
	state := &State{
		skipped: make(map[int]struct{}),
		values:  make(map[string]any),
		params:  make(map[string]string),
	}
	return request.WithContext(context.WithValue(request.Context(), contextKey{}, state)), state
}

func (s *State) Get(key string) (any, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.values[key]
	return value, exists
}

func (s *State) Set(key string, value any) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.values == nil {
		s.values = make(map[string]any)
	}
	if value == nil {
		delete(s.values, key)
	} else {
		s.values[key] = value
	}
	s.mu.Unlock()
}

func (s *State) GetString(key string) string {
	value, _ := s.Get(key)
	result, _ := value.(string)
	return result
}

func (s *State) GetBool(key string) bool {
	value, _ := s.Get(key)
	result, _ := value.(bool)
	return result
}

func (s *State) SetParam(key, value string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.params == nil {
		s.params = make(map[string]string)
	}
	s.params[key] = value
	s.mu.Unlock()
}

func (s *State) Param(key string) string {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.params[key]
}

func From(ctx context.Context) *State {
	if ctx == nil {
		return nil
	}
	state, _ := ctx.Value(contextKey{}).(*State)
	return state
}

func (s *State) SetInboundProtocol(protocol domain.Protocol) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.inboundProtocol = protocol
	s.mu.Unlock()
}

func (s *State) InboundProtocol() domain.Protocol {
	if s == nil {
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inboundProtocol
}

func (s *State) BindProtocolRoute(route registry.ConverterSpec) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.protocolRoute = route
	s.hasProtocolRoute = true
	s.mu.Unlock()
}

func (s *State) ProtocolRoute() (registry.ConverterSpec, bool) {
	if s == nil {
		return registry.ConverterSpec{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.protocolRoute, s.hasProtocolRoute
}

func (s *State) SetStreamSession(session *stream.Session) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.streamSession = session
	s.mu.Unlock()
}

func (s *State) StreamSession() *stream.Session {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streamSession
}

func (s *State) SetAttemptCount(count int) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.attemptCount = count
	s.mu.Unlock()
}

func (s *State) AttemptCount() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.attemptCount
}

func (s *State) Skip(endpointID int) {
	if s == nil || endpointID <= 0 {
		return
	}
	s.mu.Lock()
	if s.skipped == nil {
		s.skipped = make(map[int]struct{})
	}
	s.skipped[endpointID] = struct{}{}
	s.mu.Unlock()
}

func (s *State) SkippedEndpointIDs() []int {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]int, 0, len(s.skipped))
	for endpointID := range s.skipped {
		ids = append(ids, endpointID)
	}
	return ids
}

func (s *State) SetSelection(selection Selection) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.selection = selection
	s.mu.Unlock()
}

func (s *State) Selection() Selection {
	if s == nil {
		return Selection{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selection
}
