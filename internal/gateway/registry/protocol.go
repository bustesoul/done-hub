package registry

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"done-hub/internal/gateway/domain"
)

var (
	ErrDuplicateProtocol = errors.New("protocol conversion already registered")
	ErrProtocolNotFound  = errors.New("protocol conversion is not registered")
	ErrConverterMissing  = errors.New("protocol conversion has no executable converter")
	ErrStreamUnsupported = errors.New("protocol conversion does not support streaming")
)

type ConversionQuality string

const (
	ConversionGood        ConversionQuality = "good"
	ConversionFair        ConversionQuality = "fair"
	ConversionDiscouraged ConversionQuality = "discouraged"
)

type ConverterSpec struct {
	From          domain.Protocol
	To            domain.Protocol
	Stream        bool
	Quality       ConversionQuality
	AllowMultiHop bool
	Converter     Converter
}

// Converter is the executable half of a protocol route. Payloads remain typed
// values owned by protocol packages; the registry only coordinates their
// direction and lifecycle.
type Converter interface {
	ID() string
	ConvertRequest(ctx context.Context, value any) (any, error)
	ConvertResponse(ctx context.Context, value any) (any, error)
}

type StreamConverter interface {
	ConvertStream(ctx context.Context, value any) (any, error)
}

type protocolKey struct {
	from domain.Protocol
	to   domain.Protocol
}

type ProtocolRegistry struct {
	mu          sync.RWMutex
	conversions map[protocolKey]ConverterSpec
}

func NewProtocolRegistry() *ProtocolRegistry {
	return &ProtocolRegistry{
		conversions: make(map[protocolKey]ConverterSpec),
	}
}

func (r *ProtocolRegistry) Register(spec ConverterSpec) error {
	if spec.From == "" || spec.To == "" || spec.Quality == "" {
		return fmt.Errorf("%w: from, to and quality are required", ErrInvalidProvider)
	}
	if spec.Converter == nil || spec.Converter.ID() == "" {
		return fmt.Errorf("%w: %s -> %s", ErrConverterMissing, spec.From, spec.To)
	}
	if spec.Stream {
		if _, ok := spec.Converter.(StreamConverter); !ok {
			return fmt.Errorf("%w: %s -> %s", ErrStreamUnsupported, spec.From, spec.To)
		}
	}

	key := protocolKey{from: spec.From, to: spec.To}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.conversions[key]; exists {
		return fmt.Errorf("%w: %s -> %s", ErrDuplicateProtocol, spec.From, spec.To)
	}
	r.conversions[key] = spec
	return nil
}

func (r *ProtocolRegistry) Get(from, to domain.Protocol) (ConverterSpec, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, exists := r.conversions[protocolKey{from: from, to: to}]
	if !exists {
		return ConverterSpec{}, fmt.Errorf("%w: %s -> %s", ErrProtocolNotFound, from, to)
	}
	return spec, nil
}

func (r *ProtocolRegistry) List() []ConverterSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	specs := make([]ConverterSpec, 0, len(r.conversions))
	for _, spec := range r.conversions {
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].From == specs[j].From {
			return specs[i].To < specs[j].To
		}
		return specs[i].From < specs[j].From
	})
	return specs
}
