package providers

import (
	"done-hub/common/config"
	"done-hub/internal/gateway/catalog"
	"done-hub/internal/gateway/domain"
	gatewayregistry "done-hub/internal/gateway/registry"
	"done-hub/model"
	"done-hub/providers/ali"
	"done-hub/providers/antigravity"
	"done-hub/providers/azure"
	azurespeech "done-hub/providers/azureSpeech"
	"done-hub/providers/azure_v1"
	"done-hub/providers/azuredatabricks"
	"done-hub/providers/baichuan"
	"done-hub/providers/baidu"
	"done-hub/providers/base"
	"done-hub/providers/bedrock"
	"done-hub/providers/bedrockmessages"
	"done-hub/providers/claude"
	"done-hub/providers/claudecode"
	"done-hub/providers/cloudflareAI"
	"done-hub/providers/codex"
	"done-hub/providers/cohere"
	"done-hub/providers/copilot"
	"done-hub/providers/coze"
	"done-hub/providers/deepseek"
	"done-hub/providers/gemini"
	"done-hub/providers/geminicli"
	"done-hub/providers/github"
	"done-hub/providers/groq"
	"done-hub/providers/hunyuan"
	"done-hub/providers/jina"
	"done-hub/providers/kling"
	"done-hub/providers/lingyi"
	"done-hub/providers/midjourney"
	"done-hub/providers/minimax"
	"done-hub/providers/mistral"
	"done-hub/providers/moonshot"
	"done-hub/providers/ollama"
	"done-hub/providers/openai"
	"done-hub/providers/openrouter"
	"done-hub/providers/palm"
	"done-hub/providers/recraftAI"
	"done-hub/providers/replicate"
	"done-hub/providers/siliconflow"
	"done-hub/providers/stabilityAI"
	"done-hub/providers/suno"
	"done-hub/providers/tencent"
	"done-hub/providers/vertexai"
	vertexai_express "done-hub/providers/vertexai_express"
	"done-hub/providers/xAI"
	"done-hub/providers/xunfei"
	"done-hub/providers/zhipu"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

type AdapterFactory interface {
	CreateAdapter(channel *model.Channel) base.ProviderRuntime
}

var providerRegistry = gatewayregistry.NewProviderRegistry[AdapterFactory]()

func init() {
	registerProvider(config.ChannelTypeOpenAI, "openai", "OpenAI", openai.OpenAIProviderFactory{}, openAICapabilities(), []domain.Protocol{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeAzure, "azure", "Azure OpenAI", azure.AzureProviderFactory{}, openAICapabilities(), []domain.Protocol{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses}, []domain.AuthMode{domain.AuthModeAPIKey}, true)
	registerProvider(config.ChannelTypeCustom, "openai-compatible", "OpenAI Compatible", openai.OpenAIProviderFactory{}, openAICapabilities(), []domain.Protocol{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses}, []domain.AuthMode{domain.AuthModeBearer, domain.AuthModeAPIKey}, true)
	registerProvider(config.ChannelTypePaLM, "palm", "Google PaLM", palm.PalmProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeAnthropic, "anthropic", "Anthropic", claude.ClaudeProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolClaudeMessages, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeBaidu, "baidu", "Baidu", baidu.BaiduProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityEmbeddings), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeZhipu, "zhipu", "Zhipu", zhipu.ZhipuProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityEmbeddings, domain.CapabilityImageGenerations), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeAli, "ali", "Alibaba Qwen", ali.AliProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityEmbeddings), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeXunfei, "xunfei", "Xunfei Spark", xunfei.XunfeiProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeSignedRequest}, false)
	registerProvider(config.ChannelType360, "360-compatible", "360 OpenAI Compatible", openai.OpenAIProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityEmbeddings), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeOpenRouter, "openrouter", "OpenRouter", openrouter.OpenRouterProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeTencent, "tencent", "Tencent", tencent.TencentProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeSignedRequest}, false)
	registerProvider(config.ChannelTypeAzureSpeech, "azure-speech", "Azure Speech", azurespeech.AzureSpeechProviderFactory{}, capabilities(domain.CapabilitySpeech), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeGemini, "gemini", "Google Gemini", gemini.GeminiProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityImageGenerations, domain.CapabilityTask), []domain.Protocol{domain.ProtocolGemini, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey, domain.AuthModeServiceAccount}, false)
	registerProvider(config.ChannelTypeBaichuan, "baichuan", "Baichuan", baichuan.BaichuanProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeMiniMax, "minimax", "MiniMax", minimax.MiniMaxProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilitySpeech), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeDeepseek, "deepseek", "DeepSeek", deepseek.DeepseekProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat, domain.ProtocolClaudeMessages}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeMoonshot, "moonshot", "Moonshot", moonshot.MoonshotProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeMistral, "mistral", "Mistral", mistral.MistralProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeGroq, "groq", "Groq", groq.GroqProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeBedrock, "bedrock", "AWS Bedrock", bedrock.BedrockProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat, domain.ProtocolClaudeMessages}, []domain.AuthMode{domain.AuthModeSignedRequest, domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeLingyi, "lingyi", "Lingyi", lingyi.LingyiProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeMidjourney, "midjourney", "Midjourney", midjourney.MidjourneyProviderFactory{}, capabilities(domain.CapabilityTask), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeCloudflareAI, "cloudflare-ai", "Cloudflare AI", cloudflareAI.CloudflareAIProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityImageGenerations, domain.CapabilityTranscriptions), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeCohere, "cohere", "Cohere", cohere.CohereProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityRerank), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeStabilityAI, "stability-ai", "Stability AI", stabilityAI.StabilityAIProviderFactory{}, capabilities(domain.CapabilityImageGenerations), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeCoze, "coze", "Coze", coze.CozeProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeOllama, "ollama", "Ollama", ollama.OllamaProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityEmbeddings), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeNone, domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeHunyuan, "hunyuan", "Hunyuan", hunyuan.HunyuanProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeSignedRequest}, false)
	registerProvider(config.ChannelTypeSuno, "suno", "Suno", suno.SunoProviderFactory{}, capabilities(domain.CapabilityTask), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeVertexAI, "vertex-ai", "Vertex AI", vertexai.VertexAIProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityImageGenerations), []domain.Protocol{domain.ProtocolGemini, domain.ProtocolClaudeMessages, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeServiceAccount}, false)
	registerProvider(config.ChannelTypeLLAMA, "llama-compatible", "Meta Llama Compatible", openai.OpenAIProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeIdeogram, "ideogram-compatible", "Ideogram Compatible", openai.OpenAIProviderFactory{}, capabilities(domain.CapabilityImageGenerations), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeSiliconflow, "siliconflow", "SiliconFlow", siliconflow.SiliconflowProviderFactory{}, capabilities(domain.CapabilityImageGenerations, domain.CapabilityRerank), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeFlux, "flux-compatible", "Flux Compatible", openai.OpenAIProviderFactory{}, capabilities(domain.CapabilityImageGenerations), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeJina, "jina", "Jina", jina.JinaProviderFactory{}, capabilities(domain.CapabilityRerank), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeGithub, "github-models", "GitHub Models", github.GithubProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeRecraft, "recraft", "Recraft", recraftAI.RecraftProviderFactory{}, capabilities(domain.CapabilityImageGenerations, domain.CapabilityPassthrough), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeReplicate, "replicate", "Replicate", replicate.ReplicateProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityImageGenerations, domain.CapabilityTask), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeBearer}, false)
	registerProvider(config.ChannelTypeKling, "kling", "Kling", kling.KlingProviderFactory{}, capabilities(domain.CapabilityTask), []domain.Protocol{domain.ProtocolNative}, []domain.AuthMode{domain.AuthModeSignedRequest}, false)
	registerProvider(config.ChannelTypeAzureDatabricks, "azure-databricks", "Azure Databricks", azuredatabricks.AzureDatabricksProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeAzureV1, "azure-v1", "Azure OpenAI v1", azure_v1.AzureV1ProviderFactory{}, openAICapabilities(), []domain.Protocol{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses}, []domain.AuthMode{domain.AuthModeAPIKey}, true)
	registerProvider(config.ChannelTypeXAI, "xai", "xAI", xAI.XAIProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityImageGenerations, domain.CapabilityImageEdits), []domain.Protocol{domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeBearer}, true)
	registerProvider(config.ChannelTypeGeminiCli, "gemini-cli", "Gemini CLI", geminicli.GeminiCliProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityImageGenerations), []domain.Protocol{domain.ProtocolGemini, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeOAuth}, false)
	registerProvider(config.ChannelTypeClaudeCode, "claude-code", "Claude Code", claudecode.ClaudeCodeProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolClaudeMessages, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeOAuth}, false)
	registerProvider(config.ChannelTypeCodex, "codex", "OpenAI Codex", codex.CodexProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityResponses, domain.CapabilityImageGenerations, domain.CapabilityImageEdits), []domain.Protocol{domain.ProtocolOpenAIResponses, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeOAuth}, false)
	registerProvider(config.ChannelTypeAntigravity, "antigravity", "Antigravity", antigravity.AntigravityProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolGemini, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeOAuth}, false)
	registerProvider(config.ChannelTypeVertexAIExpress, "vertex-ai-express", "Vertex AI Express", vertexai_express.VertexAIExpressProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolGemini, domain.ProtocolOpenAIChat}, []domain.AuthMode{domain.AuthModeAPIKey}, false)
	registerProvider(config.ChannelTypeCopilot, "copilot", "GitHub Copilot", copilot.CopilotProviderFactory{}, capabilities(domain.CapabilityChat, domain.CapabilityResponses), []domain.Protocol{domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses}, []domain.AuthMode{domain.AuthModeOAuth}, false)
	registerProvider(config.ChannelTypeBedrockMessages, "bedrock-messages", "AWS Bedrock Messages", bedrockmessages.BedrockMessagesProviderFactory{}, capabilities(domain.CapabilityChat), []domain.Protocol{domain.ProtocolClaudeMessages}, []domain.AuthMode{domain.AuthModeSignedRequest, domain.AuthModeAPIKey}, false)

	if err := providerRegistry.Freeze(); err != nil {
		panic(fmt.Errorf("freeze provider registry: %w", err))
	}
	if _, err := catalog.ConnectionProfiles(providerRegistry.Definitions()); err != nil {
		panic(fmt.Errorf("validate connection profile catalog: %w", err))
	}
}

func capabilities(values ...domain.Capability) []domain.Capability {
	return values
}

func openAICapabilities() []domain.Capability {
	return capabilities(
		domain.CapabilityChat,
		domain.CapabilityCompletions,
		domain.CapabilityResponses,
		domain.CapabilityEmbeddings,
		domain.CapabilityModerations,
		domain.CapabilityImageGenerations,
		domain.CapabilityImageEdits,
		domain.CapabilityImageVariations,
		domain.CapabilitySpeech,
		domain.CapabilityTranscriptions,
		domain.CapabilityTranslations,
		domain.CapabilityRealtime,
		domain.CapabilityPassthrough,
	)
}

func registerProvider(
	channelType int,
	id string,
	displayName string,
	factory AdapterFactory,
	providerCapabilities []domain.Capability,
	protocols []domain.Protocol,
	authModes []domain.AuthMode,
	openAICompatible bool,
) {
	baseURLPolicy, defaults, oauth := providerManagementMetadata(channelType)
	providerRegistry.MustRegister(gatewayregistry.ProviderEntry[AdapterFactory]{
		Definition: domain.ProviderDefinition{
			ID:               id,
			DisplayName:      displayName,
			ChannelType:      domain.ProviderID(channelType),
			Capabilities:     providerCapabilities,
			Protocols:        protocols,
			AuthModes:        authModes,
			OpenAICompatible: openAICompatible,
			BaseURLPolicy:    baseURLPolicy,
			Defaults:         defaults,
			OAuth:            oauth,
		},
		Factory: factory,
	})
}

func providerManagementMetadata(channelType int) (domain.BaseURLPolicy, map[string]string, *domain.ProviderOAuthDefinition) {
	policy := domain.BaseURLPolicyOptional
	var defaults map[string]string
	var oauth *domain.ProviderOAuthDefinition

	switch channelType {
	case config.ChannelTypeCustom, config.ChannelTypeAzure:
		policy = domain.BaseURLPolicyRequired
	}
	switch channelType {
	case config.ChannelTypeAzure:
		defaults = map[string]string{"other": "2024-05-01-preview"}
	case config.ChannelTypeXunfei:
		defaults = map[string]string{"other": "v2.1"}
	}
	switch channelType {
	case config.ChannelTypeGeminiCli:
		oauth = &domain.ProviderOAuthDefinition{Provider: "gemini-cli", Flow: domain.OAuthFlowBrowserCallback}
	case config.ChannelTypeAntigravity:
		oauth = &domain.ProviderOAuthDefinition{Provider: "antigravity", Flow: domain.OAuthFlowBrowserCallback}
	case config.ChannelTypeClaudeCode:
		oauth = &domain.ProviderOAuthDefinition{Provider: "claude-code", Flow: domain.OAuthFlowManualCallback}
	case config.ChannelTypeCodex:
		oauth = &domain.ProviderOAuthDefinition{Provider: "codex", Flow: domain.OAuthFlowManualCallback}
	case config.ChannelTypeCopilot:
		oauth = &domain.ProviderOAuthDefinition{Provider: "copilot", Flow: domain.OAuthFlowDeviceCode}
	}
	return policy, defaults, oauth
}

func ProviderDefinitions() []domain.ProviderDefinition {
	return providerRegistry.Definitions()
}

func ConnectionProfiles() ([]domain.ConnectionProfileDefinition, error) {
	return catalog.ConnectionProfiles(providerRegistry.Definitions())
}

func DefaultProtocolProfile(channelType int) (domain.ProtocolProfileID, error) {
	definition, err := GetProviderDefinition(channelType)
	if err != nil {
		return "", err
	}
	return catalog.DefaultProfileForProvider(definition), nil
}

func ValidateProtocolProfile(channelType int, profileID domain.ProtocolProfileID) error {
	definition, err := GetProviderDefinition(channelType)
	if err != nil {
		return err
	}
	return catalog.ValidateProfileForProvider(profileID, definition)
}

func BackfillProtocolProfiles(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is nil")
	}
	var channels []model.Channel
	if err := db.Where("protocol_profile_id = ? OR protocol_profile_id IS NULL", "").Find(&channels).Error; err != nil {
		return err
	}
	if len(channels) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		changedIDs := make([]int, 0, len(channels))
		for index := range channels {
			channel := &channels[index]
			profileID, err := DefaultProtocolProfile(channel.Type)
			if err != nil {
				return fmt.Errorf("channel %d provider profile: %w", channel.Id, err)
			}
			if profileID == "" {
				continue
			}
			if err := tx.Model(&model.Channel{}).
				Where("id = ?", channel.Id).
				Update("protocol_profile_id", string(profileID)).Error; err != nil {
				return err
			}
			changedIDs = append(changedIDs, channel.Id)
		}
		return model.SyncGatewayChannels(tx, changedIDs)
	})
}

func GetProviderDefinition(channelType int) (domain.ProviderDefinition, error) {
	entry, err := providerRegistry.Get(domain.ProviderID(channelType))
	if err != nil {
		return domain.ProviderDefinition{}, err
	}
	return entry.Definition, nil
}

func GetProviderWithError(channel *model.Channel, c *base.RequestContext) (base.ProviderRuntime, error) {
	if channel == nil {
		return nil, errors.New("channel is nil")
	}
	entry, err := providerRegistry.Get(domain.ProviderID(channel.Type))
	if err != nil {
		return nil, err
	}

	provider := entry.Factory.CreateAdapter(channel)
	if provider == nil {
		return nil, fmt.Errorf("provider factory %q returned nil", entry.Definition.ID)
	}
	provider.SetContext(c)
	return provider, nil
}

func GetProvider(channel *model.Channel, c *base.RequestContext) base.ProviderRuntime {
	provider, _ := GetProviderWithError(channel, c)
	return provider
}
