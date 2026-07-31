package catalog

import (
	"testing"

	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
)

func TestConnectionProfilesAreOrderedAndProtocolSafe(t *testing.T) {
	definitions := []domain.ProviderDefinition{
		provider(config.ChannelTypeOpenAI, "openai", domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses),
		provider(config.ChannelTypeCustom, "openai-compatible", domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses),
		provider(config.ChannelTypeAnthropic, "anthropic", domain.ProtocolClaudeMessages, domain.ProtocolOpenAIChat),
		provider(config.ChannelTypeBedrockMessages, "bedrock-messages", domain.ProtocolClaudeMessages),
		provider(config.ChannelTypeGemini, "gemini", domain.ProtocolGemini, domain.ProtocolOpenAIChat),
		provider(config.ChannelTypeVertexAI, "vertex-ai", domain.ProtocolGemini, domain.ProtocolClaudeMessages),
	}

	profiles, err := ConnectionProfiles(definitions)
	if err != nil {
		t.Fatalf("build profiles: %v", err)
	}
	want := []domain.ProtocolProfileID{
		domain.ProfileOpenAIChat,
		domain.ProfileOpenAIResponses,
		domain.ProfileAnthropicMessages,
		domain.ProfileGoogleGemini,
	}
	if len(profiles) != len(want) {
		t.Fatalf("expected %d profiles, got %d", len(want), len(profiles))
	}
	for index, profile := range profiles {
		if profile.ID != want[index] {
			t.Fatalf("profile %d: expected %q, got %q", index, want[index], profile.ID)
		}
		if !profile.Featured || profile.CatalogSection != "mainstream" || len(profile.Variants) == 0 {
			t.Fatalf("profile %q is incomplete: %#v", profile.ID, profile)
		}
		for _, variant := range profile.Variants {
			definition := definitionByType(definitions, variant.ChannelType)
			if err := ValidateProfileForProvider(profile.ID, definition); err != nil {
				t.Fatalf("variant %q is invalid for profile %q: %v", variant.ProviderID, profile.ID, err)
			}
		}
	}
}

func TestConnectionProfilesRejectProtocolMismatch(t *testing.T) {
	definitions := []domain.ProviderDefinition{
		provider(config.ChannelTypeOpenAI, "openai", domain.ProtocolOpenAIChat),
		provider(config.ChannelTypeCustom, "openai-compatible", domain.ProtocolOpenAIChat),
		provider(config.ChannelTypeAnthropic, "anthropic", domain.ProtocolClaudeMessages),
		provider(config.ChannelTypeBedrockMessages, "bedrock-messages", domain.ProtocolClaudeMessages),
		provider(config.ChannelTypeGemini, "gemini", domain.ProtocolGemini),
		provider(config.ChannelTypeVertexAI, "vertex-ai", domain.ProtocolClaudeMessages),
	}
	if _, err := ConnectionProfiles(definitions); err == nil {
		t.Fatal("expected vertex profile protocol mismatch")
	}
}

func TestDefaultProfileUsesProvidersNativeFirstProtocol(t *testing.T) {
	tests := []struct {
		definition domain.ProviderDefinition
		want       domain.ProtocolProfileID
	}{
		{provider(1, "openai", domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses), domain.ProfileOpenAIChat},
		{provider(14, "anthropic", domain.ProtocolClaudeMessages, domain.ProtocolOpenAIChat), domain.ProfileAnthropicMessages},
		{provider(25, "gemini", domain.ProtocolGemini, domain.ProtocolOpenAIChat), domain.ProfileGoogleGemini},
		{provider(59, "codex", domain.ProtocolOpenAIResponses, domain.ProtocolOpenAIChat), domain.ProfileOpenAIResponses},
	}
	for _, test := range tests {
		if got := DefaultProfileForProvider(test.definition); got != test.want {
			t.Fatalf("provider %q: expected %q, got %q", test.definition.ID, test.want, got)
		}
	}
}

func provider(channelType int, id string, protocols ...domain.Protocol) domain.ProviderDefinition {
	return domain.ProviderDefinition{
		ID:           id,
		DisplayName:  id,
		ChannelType:  domain.ProviderID(channelType),
		Capabilities: []domain.Capability{domain.CapabilityChat},
		Protocols:    protocols,
		AuthModes:    []domain.AuthMode{domain.AuthModeAPIKey},
	}
}

func definitionByType(definitions []domain.ProviderDefinition, channelType domain.ProviderID) domain.ProviderDefinition {
	for _, definition := range definitions {
		if definition.ChannelType == channelType {
			return definition
		}
	}
	return domain.ProviderDefinition{}
}
