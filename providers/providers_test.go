package providers

import (
	"errors"
	"testing"

	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
	gatewayregistry "done-hub/internal/gateway/registry"
	"done-hub/model"
	providersbase "done-hub/providers/base"
	claudeprovider "done-hub/providers/claude"
	"done-hub/providers/kling"
	"done-hub/providers/openai"

	"github.com/glebarez/sqlite"
	"github.com/spf13/viper"
	"gorm.io/gorm"
)

func TestProviderRegistryCoversSupportedChannelTypes(t *testing.T) {
	t.Parallel()

	expectedTypes := []int{
		config.ChannelTypeOpenAI,
		config.ChannelTypeAzure,
		config.ChannelTypeCustom,
		config.ChannelTypePaLM,
		config.ChannelTypeAnthropic,
		config.ChannelTypeBaidu,
		config.ChannelTypeZhipu,
		config.ChannelTypeAli,
		config.ChannelTypeXunfei,
		config.ChannelType360,
		config.ChannelTypeOpenRouter,
		config.ChannelTypeTencent,
		config.ChannelTypeAzureSpeech,
		config.ChannelTypeGemini,
		config.ChannelTypeBaichuan,
		config.ChannelTypeMiniMax,
		config.ChannelTypeDeepseek,
		config.ChannelTypeMoonshot,
		config.ChannelTypeMistral,
		config.ChannelTypeGroq,
		config.ChannelTypeBedrock,
		config.ChannelTypeLingyi,
		config.ChannelTypeMidjourney,
		config.ChannelTypeCloudflareAI,
		config.ChannelTypeCohere,
		config.ChannelTypeStabilityAI,
		config.ChannelTypeCoze,
		config.ChannelTypeOllama,
		config.ChannelTypeHunyuan,
		config.ChannelTypeSuno,
		config.ChannelTypeVertexAI,
		config.ChannelTypeLLAMA,
		config.ChannelTypeIdeogram,
		config.ChannelTypeSiliconflow,
		config.ChannelTypeFlux,
		config.ChannelTypeJina,
		config.ChannelTypeGithub,
		config.ChannelTypeRecraft,
		config.ChannelTypeReplicate,
		config.ChannelTypeKling,
		config.ChannelTypeAzureDatabricks,
		config.ChannelTypeAzureV1,
		config.ChannelTypeXAI,
		config.ChannelTypeGeminiCli,
		config.ChannelTypeClaudeCode,
		config.ChannelTypeCodex,
		config.ChannelTypeAntigravity,
		config.ChannelTypeVertexAIExpress,
		config.ChannelTypeCopilot,
		config.ChannelTypeBedrockMessages,
	}

	definitions := ProviderDefinitions()
	if len(definitions) != len(expectedTypes) {
		t.Fatalf("expected %d providers, got %d", len(expectedTypes), len(definitions))
	}
	for index, channelType := range expectedTypes {
		if int(definitions[index].ChannelType) != channelType {
			t.Fatalf("definition %d: expected channel type %d, got %d", index, channelType, definitions[index].ChannelType)
		}
		if len(definitions[index].Capabilities) == 0 {
			t.Fatalf("definition %q has no capabilities", definitions[index].ID)
		}
	}
}

func TestProviderManagementMetadataIsDescriptorDriven(t *testing.T) {
	tests := []struct {
		channelType  int
		policy       domain.BaseURLPolicy
		oauthFlow    domain.OAuthFlow
		defaultOther string
	}{
		{config.ChannelTypeCustom, domain.BaseURLPolicyRequired, "", ""},
		{config.ChannelTypeAzure, domain.BaseURLPolicyRequired, "", "2024-05-01-preview"},
		{config.ChannelTypeXunfei, domain.BaseURLPolicyOptional, "", "v2.1"},
		{config.ChannelTypeGeminiCli, domain.BaseURLPolicyOptional, domain.OAuthFlowBrowserCallback, ""},
		{config.ChannelTypeClaudeCode, domain.BaseURLPolicyOptional, domain.OAuthFlowManualCallback, ""},
		{config.ChannelTypeCopilot, domain.BaseURLPolicyOptional, domain.OAuthFlowDeviceCode, ""},
	}
	for _, test := range tests {
		definition, err := GetProviderDefinition(test.channelType)
		if err != nil {
			t.Fatalf("get provider %d: %v", test.channelType, err)
		}
		if definition.BaseURLPolicy != test.policy {
			t.Fatalf("provider %q policy: got %q want %q", definition.ID, definition.BaseURLPolicy, test.policy)
		}
		if definition.Defaults["other"] != test.defaultOther {
			t.Fatalf("provider %q default other: got %q want %q", definition.ID, definition.Defaults["other"], test.defaultOther)
		}
		if test.oauthFlow == "" {
			if definition.OAuth != nil {
				t.Fatalf("provider %q unexpectedly exposes OAuth metadata", definition.ID)
			}
		} else if definition.OAuth == nil || definition.OAuth.Flow != test.oauthFlow || definition.OAuth.Provider == "" {
			t.Fatalf("provider %q OAuth metadata is incomplete: %#v", definition.ID, definition.OAuth)
		}
	}
}

func TestUnknownProviderDoesNotFallbackToOpenAI(t *testing.T) {
	t.Parallel()

	channel := &model.Channel{
		Type:    999,
		BaseURL: stringPointer("https://example.invalid"),
	}
	provider, err := GetProviderWithError(channel, nil)
	if provider != nil {
		t.Fatalf("expected nil provider, got %T", provider)
	}
	if !errors.Is(err, gatewayregistry.ErrProviderNotFound) {
		t.Fatalf("expected provider not found, got %v", err)
	}
}

func TestCompatibilityAndTaskProvidersAreExplicit(t *testing.T) {
	t.Parallel()

	customProvider, err := GetProviderWithError(&model.Channel{
		Type:    config.ChannelTypeCustom,
		BaseURL: stringPointer("https://example.invalid"),
	}, nil)
	if err != nil {
		t.Fatalf("create custom provider: %v", err)
	}
	if _, ok := customProvider.(*openai.OpenAIProvider); !ok {
		t.Fatalf("expected explicit OpenAI compatibility provider, got %T", customProvider)
	}

	klingProvider, err := GetProviderWithError(&model.Channel{
		Type: config.ChannelTypeKling,
		Key:  "test-key",
	}, nil)
	if err != nil {
		t.Fatalf("create Kling provider: %v", err)
	}
	if _, ok := klingProvider.(*kling.KlingProvider); !ok {
		t.Fatalf("expected Kling provider, got %T", klingProvider)
	}
}

func TestProviderCapabilitiesMatchImplementedInterfaces(t *testing.T) {
	for _, definition := range ProviderDefinitions() {
		definition := definition
		t.Run(definition.ID, func(t *testing.T) {
			provider, err := GetProviderWithError(&model.Channel{
				Type:    int(definition.ChannelType),
				Key:     "contract-test",
				BaseURL: stringPointer("https://example.invalid"),
			}, nil)
			if err != nil {
				t.Fatalf("create provider: %v", err)
			}

			for _, capability := range definition.Capabilities {
				var implemented bool
				switch capability {
				case domain.CapabilityChat:
					_, implemented = provider.(providersbase.ChatInterface)
					if !implemented {
						_, implemented = provider.(claudeprovider.ClaudeChatInterface)
					}
				case domain.CapabilityCompletions:
					_, implemented = provider.(providersbase.CompletionInterface)
				case domain.CapabilityResponses:
					_, implemented = provider.(providersbase.ResponsesInterface)
				case domain.CapabilityEmbeddings:
					_, implemented = provider.(providersbase.EmbeddingsInterface)
				case domain.CapabilityModerations:
					_, implemented = provider.(providersbase.ModerationInterface)
				case domain.CapabilityImageGenerations:
					_, implemented = provider.(providersbase.ImageGenerationsInterface)
				case domain.CapabilityImageEdits:
					_, implemented = provider.(providersbase.ImageEditsInterface)
				case domain.CapabilityImageVariations:
					_, implemented = provider.(providersbase.ImageVariationsInterface)
				case domain.CapabilitySpeech:
					_, implemented = provider.(providersbase.SpeechInterface)
				case domain.CapabilityTranscriptions:
					_, implemented = provider.(providersbase.TranscriptionsInterface)
				case domain.CapabilityTranslations:
					_, implemented = provider.(providersbase.TranslationInterface)
				case domain.CapabilityRerank:
					_, implemented = provider.(providersbase.RerankInterface)
				case domain.CapabilityRealtime:
					_, implemented = provider.(providersbase.RealtimeInterface)
				case domain.CapabilityTask, domain.CapabilityPassthrough:
					continue
				}
				if !implemented {
					t.Errorf("declares %q but %T does not implement its contract", capability, provider)
				}
			}
		})
	}
}

func TestBackfillProtocolProfilesMigratesExistingConnections(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Channel{}); err != nil {
		t.Fatalf("migrate channels: %v", err)
	}
	if err := model.AutoMigrateGatewayResources(db); err != nil {
		t.Fatalf("migrate gateway resources: %v", err)
	}
	channels := []model.Channel{
		{Type: config.ChannelTypeOpenAI, Key: "openai-key", Status: 1, Name: "openai", Models: "gpt-test", Group: "default"},
		{Type: config.ChannelTypeAnthropic, Key: "anthropic-key", Status: 1, Name: "anthropic", Models: "claude-test", Group: "default"},
		{Type: config.ChannelTypeXunfei, Key: "native-key", Status: 1, Name: "native", Models: "spark-test", Group: "default"},
	}
	if err := db.Create(&channels).Error; err != nil {
		t.Fatalf("create channels: %v", err)
	}
	viper.Set("gateway_secret_key", "profile-backfill-test")
	defer viper.Set("gateway_secret_key", "")
	if err := BackfillProtocolProfiles(db); err != nil {
		t.Fatalf("backfill profiles: %v", err)
	}

	var got []model.Channel
	if err := db.Order("id ASC").Find(&got).Error; err != nil {
		t.Fatalf("reload channels: %v", err)
	}
	if got[0].ProtocolProfileID != string(domain.ProfileOpenAIChat) {
		t.Fatalf("unexpected OpenAI profile %q", got[0].ProtocolProfileID)
	}
	if got[1].ProtocolProfileID != string(domain.ProfileAnthropicMessages) {
		t.Fatalf("unexpected Anthropic profile %q", got[1].ProtocolProfileID)
	}
	if got[2].ProtocolProfileID != "" {
		t.Fatalf("native-only provider should not be forced into a mainstream profile: %q", got[2].ProtocolProfileID)
	}

	var endpoint model.GatewayEndpoint
	if err := db.Where("channel_id = ?", got[1].Id).First(&endpoint).Error; err != nil {
		t.Fatalf("load migrated endpoint: %v", err)
	}
	if endpoint.ProtocolProfileID != string(domain.ProfileAnthropicMessages) {
		t.Fatalf("gateway endpoint profile was not refreshed: %q", endpoint.ProtocolProfileID)
	}
}

func stringPointer(value string) *string {
	return &value
}
