package base

import (
	"bytes"
	"done-hub/common/config"
	"io"
	"net/http"
	"strings"
	"sync"
)

type ContextStore interface {
	Get(key string) (value any, exists bool)
	Set(key string, value any)
	GetString(key string) string
	GetBool(key string) bool
	Param(key string) string
}

type RequestContext struct {
	Request            *http.Request
	store              ContextStore
	mu                 sync.RWMutex
	originalModel      string
	unifiedModelLogged bool
}

func (c *RequestContext) SetOriginalModel(model string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.originalModel = model
	c.mu.Unlock()
}

func (c *RequestContext) OriginalModel() string {
	if c == nil {
		return ""
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.originalModel
}

func (c *RequestContext) MarkUnifiedModelLogged() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.unifiedModelLogged {
		return false
	}
	c.unifiedModelLogged = true
	return true
}

func NewRequestContext(request *http.Request, store ContextStore) *RequestContext {
	return &RequestContext{
		Request: request,
		store:   store,
	}
}

func NewMemoryRequestContext(request *http.Request) *RequestContext {
	return NewRequestContext(request, &memoryContextStore{
		values: make(map[string]any),
		params: make(map[string]string),
	})
}

func (c *RequestContext) Get(key string) (any, bool) {
	if c == nil || c.store == nil {
		return nil, false
	}
	return c.store.Get(key)
}

func (c *RequestContext) Set(key string, value any) {
	if c == nil || c.store == nil {
		return
	}
	c.store.Set(key, value)
}

func (c *RequestContext) GetString(key string) string {
	if c == nil || c.store == nil {
		return ""
	}
	return c.store.GetString(key)
}

func (c *RequestContext) GetBool(key string) bool {
	if c == nil || c.store == nil {
		return false
	}
	return c.store.GetBool(key)
}

func (c *RequestContext) Param(key string) string {
	if c == nil || c.store == nil {
		return ""
	}
	return c.store.Param(key)
}

func (c *RequestContext) GetRawData() ([]byte, error) {
	if cached, exists := c.Get(config.GinRequestBodyKey); exists {
		if data, ok := cached.([]byte); ok && data != nil {
			return data, nil
		}
	}
	if c == nil || c.Request == nil || c.Request.Body == nil {
		c.Set(config.GinRequestBodyKey, []byte{})
		return []byte{}, nil
	}

	data, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, err
	}
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewReader(data))
	c.Set(config.GinRequestBodyKey, data)
	return data, nil
}

func (c *RequestContext) ContentType() string {
	if c == nil || c.Request == nil {
		return ""
	}
	contentType := c.Request.Header.Get("Content-Type")
	if index := strings.IndexByte(contentType, ';'); index >= 0 {
		contentType = contentType[:index]
	}
	return strings.TrimSpace(contentType)
}

type memoryContextStore struct {
	mu     sync.RWMutex
	values map[string]any
	params map[string]string
}

func (s *memoryContextStore) Get(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, exists := s.values[key]
	return value, exists
}

func (s *memoryContextStore) Set(key string, value any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
}

func (s *memoryContextStore) GetString(key string) string {
	value, _ := s.Get(key)
	result, _ := value.(string)
	return result
}

func (s *memoryContextStore) GetBool(key string) bool {
	value, _ := s.Get(key)
	result, _ := value.(bool)
	return result
}

func (s *memoryContextStore) Param(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.params[key]
}

func (s *memoryContextStore) SetParam(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.params[key] = value
}
