package registry

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"done-hub/internal/gateway/domain"
)

var (
	ErrRegistryFrozen      = errors.New("provider registry is frozen")
	ErrDuplicateProvider   = errors.New("provider already registered")
	ErrProviderNotFound    = errors.New("provider is not registered")
	ErrInvalidProvider     = errors.New("provider definition is invalid")
	ErrDuplicateCapability = errors.New("provider capability is duplicated")
)

type ProviderEntry[T any] struct {
	Definition domain.ProviderDefinition
	Factory    T
}

type ProviderRegistry[T any] struct {
	mu      sync.RWMutex
	entries map[domain.ProviderID]ProviderEntry[T]
	frozen  bool
}

func NewProviderRegistry[T any]() *ProviderRegistry[T] {
	return &ProviderRegistry[T]{
		entries: make(map[domain.ProviderID]ProviderEntry[T]),
	}
}

func (r *ProviderRegistry[T]) Register(entry ProviderEntry[T]) error {
	if err := validateDefinition(entry.Definition); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.frozen {
		return ErrRegistryFrozen
	}
	if _, exists := r.entries[entry.Definition.ChannelType]; exists {
		return fmt.Errorf("%w: channel type %d", ErrDuplicateProvider, entry.Definition.ChannelType)
	}

	r.entries[entry.Definition.ChannelType] = entry
	return nil
}

func (r *ProviderRegistry[T]) MustRegister(entry ProviderEntry[T]) {
	if err := r.Register(entry); err != nil {
		panic(err)
	}
}

func (r *ProviderRegistry[T]) Freeze() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.entries) == 0 {
		return fmt.Errorf("%w: registry is empty", ErrInvalidProvider)
	}
	r.frozen = true
	return nil
}

func (r *ProviderRegistry[T]) Get(channelType domain.ProviderID) (ProviderEntry[T], error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	entry, exists := r.entries[channelType]
	if !exists {
		var zero ProviderEntry[T]
		return zero, fmt.Errorf("%w: channel type %d", ErrProviderNotFound, channelType)
	}
	return entry, nil
}

func (r *ProviderRegistry[T]) Definitions() []domain.ProviderDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	definitions := make([]domain.ProviderDefinition, 0, len(r.entries))
	for _, entry := range r.entries {
		definition := entry.Definition
		definition.Capabilities = append([]domain.Capability(nil), definition.Capabilities...)
		definition.Protocols = append([]domain.Protocol(nil), definition.Protocols...)
		definition.AuthModes = append([]domain.AuthMode(nil), definition.AuthModes...)
		definition.Defaults = cloneStringMap(definition.Defaults)
		if definition.OAuth != nil {
			oauth := *definition.OAuth
			definition.OAuth = &oauth
		}
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool {
		return definitions[i].ChannelType < definitions[j].ChannelType
	})
	return definitions
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func validateDefinition(definition domain.ProviderDefinition) error {
	if definition.ChannelType <= 0 || definition.ID == "" || definition.DisplayName == "" {
		return fmt.Errorf("%w: id, display name and positive channel type are required", ErrInvalidProvider)
	}
	if len(definition.Capabilities) == 0 {
		return fmt.Errorf("%w: provider %q has no capabilities", ErrInvalidProvider, definition.ID)
	}

	seen := make(map[domain.Capability]struct{}, len(definition.Capabilities))
	for _, capability := range definition.Capabilities {
		if capability == "" {
			return fmt.Errorf("%w: provider %q has an empty capability", ErrInvalidProvider, definition.ID)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("%w: provider %q capability %q", ErrDuplicateCapability, definition.ID, capability)
		}
		seen[capability] = struct{}{}
	}
	return nil
}
