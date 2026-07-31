package registry

import (
	"context"
	"errors"
	"testing"

	"done-hub/internal/gateway/domain"
)

type protocolConverterFake struct{}

func (protocolConverterFake) ID() string { return "fake" }
func (protocolConverterFake) ConvertRequest(_ context.Context, value any) (any, error) {
	return value, nil
}
func (protocolConverterFake) ConvertResponse(_ context.Context, value any) (any, error) {
	return value, nil
}
func (protocolConverterFake) ConvertStream(_ context.Context, value any) (any, error) {
	return value, nil
}

func TestProviderRegistryLifecycle(t *testing.T) {
	t.Parallel()

	registry := NewProviderRegistry[string]()
	registry.MustRegister(ProviderEntry[string]{
		Definition: domain.ProviderDefinition{
			ID:           "second",
			DisplayName:  "Second",
			ChannelType:  2,
			Capabilities: []domain.Capability{domain.CapabilityChat},
		},
		Factory: "second-factory",
	})
	registry.MustRegister(ProviderEntry[string]{
		Definition: domain.ProviderDefinition{
			ID:           "first",
			DisplayName:  "First",
			ChannelType:  1,
			Capabilities: []domain.Capability{domain.CapabilityResponses},
		},
		Factory: "first-factory",
	})

	if err := registry.Freeze(); err != nil {
		t.Fatalf("freeze registry: %v", err)
	}
	entry, err := registry.Get(1)
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	if entry.Factory != "first-factory" {
		t.Fatalf("unexpected factory %q", entry.Factory)
	}

	definitions := registry.Definitions()
	if len(definitions) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(definitions))
	}
	if definitions[0].ChannelType != 1 || definitions[1].ChannelType != 2 {
		t.Fatalf("definitions are not sorted: %#v", definitions)
	}

	err = registry.Register(ProviderEntry[string]{
		Definition: domain.ProviderDefinition{
			ID:           "third",
			DisplayName:  "Third",
			ChannelType:  3,
			Capabilities: []domain.Capability{domain.CapabilityChat},
		},
	})
	if !errors.Is(err, ErrRegistryFrozen) {
		t.Fatalf("expected frozen error, got %v", err)
	}
}

func TestProviderRegistryRejectsInvalidAndDuplicateDefinitions(t *testing.T) {
	t.Parallel()

	registry := NewProviderRegistry[struct{}]()
	entry := ProviderEntry[struct{}]{
		Definition: domain.ProviderDefinition{
			ID:           "openai",
			DisplayName:  "OpenAI",
			ChannelType:  1,
			Capabilities: []domain.Capability{domain.CapabilityChat},
		},
	}
	if err := registry.Register(entry); err != nil {
		t.Fatalf("register provider: %v", err)
	}
	if err := registry.Register(entry); !errors.Is(err, ErrDuplicateProvider) {
		t.Fatalf("expected duplicate provider error, got %v", err)
	}

	err := NewProviderRegistry[struct{}]().Register(ProviderEntry[struct{}]{
		Definition: domain.ProviderDefinition{
			ID:           "invalid",
			DisplayName:  "Invalid",
			ChannelType:  2,
			Capabilities: []domain.Capability{domain.CapabilityChat, domain.CapabilityChat},
		},
	})
	if !errors.Is(err, ErrDuplicateCapability) {
		t.Fatalf("expected duplicate capability error, got %v", err)
	}

	if _, err := registry.Get(99); !errors.Is(err, ErrProviderNotFound) {
		t.Fatalf("expected provider not found, got %v", err)
	}
}

func TestProtocolRegistry(t *testing.T) {
	t.Parallel()

	registry := NewProtocolRegistry()
	spec := ConverterSpec{
		From:      domain.ProtocolOpenAIChat,
		To:        domain.ProtocolClaudeMessages,
		Stream:    true,
		Quality:   ConversionGood,
		Converter: protocolConverterFake{},
	}
	if err := registry.Register(spec); err != nil {
		t.Fatalf("register converter: %v", err)
	}
	if err := registry.Register(spec); !errors.Is(err, ErrDuplicateProtocol) {
		t.Fatalf("expected duplicate protocol error, got %v", err)
	}
	got, err := registry.Get(spec.From, spec.To)
	if err != nil {
		t.Fatalf("get converter: %v", err)
	}
	if got.From != spec.From || got.To != spec.To || got.Converter.ID() != spec.Converter.ID() {
		t.Fatalf("unexpected converter: %#v", got)
	}
}

func TestProtocolRegistryRejectsMetadataOnlyRoute(t *testing.T) {
	t.Parallel()

	err := NewProtocolRegistry().Register(ConverterSpec{
		From:    domain.ProtocolOpenAIChat,
		To:      domain.ProtocolGemini,
		Quality: ConversionFair,
	})
	if !errors.Is(err, ErrConverterMissing) {
		t.Fatalf("expected executable converter requirement, got %v", err)
	}
}
