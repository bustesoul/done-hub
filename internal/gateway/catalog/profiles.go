package catalog

import (
	"fmt"
	"sort"

	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
)

type profileSpec struct {
	ID                 domain.ProtocolProfileID
	DisplayName        string
	Description        string
	Protocol           domain.Protocol
	DisplayOrder       int
	ModelDiscoveryPath string
	RequestPath        string
	Streaming          bool
	Variants           []variantSpec
}

type variantSpec struct {
	ChannelType    int
	DisplayName    string
	Description    string
	Advanced       bool
	DefaultBaseURL string
}

var featuredProfileSpecs = []profileSpec{
	{
		ID:                 domain.ProfileOpenAIChat,
		DisplayName:        "OpenAI Chat Completions",
		Description:        "上游实际提供 /v1/chat/completions；Responses 客户端可由 DoneHub 转换",
		Protocol:           domain.ProtocolOpenAIChat,
		DisplayOrder:       10,
		ModelDiscoveryPath: "/v1/models",
		RequestPath:        "/v1/chat/completions",
		Streaming:          true,
		Variants: []variantSpec{
			{ChannelType: config.ChannelTypeOpenAI, DefaultBaseURL: "https://api.openai.com"},
			{
				ChannelType: config.ChannelTypeCustom,
				DisplayName: "高级兼容模式",
				Description: "仅用于需要自定义接口路径或无法接受 stream_options 的上游；普通 OpenAI 兼容服务无需开启",
				Advanced:    true,
			},
		},
	},
	{
		ID:                 domain.ProfileOpenAIResponses,
		DisplayName:        "OpenAI Responses API",
		Description:        "上游原生提供完整 /v1/responses；适合 OpenAI 官方等完整实现",
		Protocol:           domain.ProtocolOpenAIResponses,
		DisplayOrder:       20,
		ModelDiscoveryPath: "/v1/models",
		RequestPath:        "/v1/responses",
		Streaming:          true,
		Variants: []variantSpec{
			{ChannelType: config.ChannelTypeOpenAI, DefaultBaseURL: "https://api.openai.com"},
			{
				ChannelType: config.ChannelTypeCustom,
				DisplayName: "高级兼容模式",
				Description: "仅用于需要自定义接口路径或无法接受 stream_options 的上游；普通 OpenAI 兼容服务无需开启",
				Advanced:    true,
			},
		},
	},
	{
		ID:                 domain.ProfileAnthropicMessages,
		DisplayName:        "Anthropic Messages API",
		Description:        "Anthropic Claude 原生 /v1/messages 协议",
		Protocol:           domain.ProtocolClaudeMessages,
		DisplayOrder:       30,
		ModelDiscoveryPath: "/v1/models",
		RequestPath:        "/v1/messages",
		Streaming:          true,
		Variants: []variantSpec{
			{ChannelType: config.ChannelTypeAnthropic, DefaultBaseURL: "https://api.anthropic.com"},
			{ChannelType: config.ChannelTypeBedrockMessages},
			{ChannelType: config.ChannelTypeVertexAI},
		},
	},
	{
		ID:                 domain.ProfileGoogleGemini,
		DisplayName:        "Google Gemini",
		Description:        "Google AI Studio 或 Vertex AI 原生 GenerateContent 协议",
		Protocol:           domain.ProtocolGemini,
		DisplayOrder:       40,
		ModelDiscoveryPath: "/v1beta/models",
		RequestPath:        "/v1beta/models/{model}:generateContent",
		Streaming:          true,
		Variants: []variantSpec{
			{ChannelType: config.ChannelTypeGemini, DefaultBaseURL: "https://generativelanguage.googleapis.com"},
			{ChannelType: config.ChannelTypeVertexAI},
		},
	},
}

func ConnectionProfiles(providerDefinitions []domain.ProviderDefinition) ([]domain.ConnectionProfileDefinition, error) {
	providersByType := make(map[domain.ProviderID]domain.ProviderDefinition, len(providerDefinitions))
	for _, definition := range providerDefinitions {
		providersByType[definition.ChannelType] = definition
	}

	profiles := make([]domain.ConnectionProfileDefinition, 0, len(featuredProfileSpecs))
	for _, spec := range featuredProfileSpecs {
		profile := domain.ConnectionProfileDefinition{
			ID:               spec.ID,
			DisplayName:      spec.DisplayName,
			Description:      spec.Description,
			Protocol:         spec.Protocol,
			Featured:         true,
			DisplayOrder:     spec.DisplayOrder,
			CatalogSection:   "mainstream",
			SupportsAffinity: SupportsAffinityProtocol(spec.Protocol),
			Probe: domain.ProtocolProbeDefinition{
				ModelDiscoveryPath: spec.ModelDiscoveryPath,
				RequestPath:        spec.RequestPath,
				Streaming:          spec.Streaming,
			},
		}
		for _, variant := range spec.Variants {
			channelType := domain.ProviderID(variant.ChannelType)
			definition, exists := providersByType[channelType]
			if !exists {
				return nil, fmt.Errorf("connection profile %q references unknown channel type %d", spec.ID, variant.ChannelType)
			}
			if !supportsProtocol(definition, spec.Protocol) {
				return nil, fmt.Errorf("provider %q does not support profile protocol %q", definition.ID, spec.Protocol)
			}
			displayName := variant.DisplayName
			if displayName == "" {
				displayName = definition.DisplayName
			}
			profile.Variants = append(profile.Variants, domain.ProviderVariant{
				ProviderID:     definition.ID,
				ChannelType:    definition.ChannelType,
				DisplayName:    displayName,
				Description:    variant.Description,
				Advanced:       variant.Advanced,
				DefaultBaseURL: variant.DefaultBaseURL,
				AuthModes:      append([]domain.AuthMode(nil), definition.AuthModes...),
				BaseURLPolicy:  definition.BaseURLPolicy,
				Defaults:       cloneStringMap(definition.Defaults),
				OAuth:          cloneOAuthDefinition(definition.OAuth),
			})
		}
		profiles = append(profiles, profile)
	}
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].DisplayOrder < profiles[j].DisplayOrder
	})
	return profiles, nil
}

func SupportsAffinityProtocol(protocol domain.Protocol) bool {
	switch protocol {
	case domain.ProtocolOpenAIChat, domain.ProtocolOpenAIResponses, domain.ProtocolClaudeMessages, domain.ProtocolGemini:
		return true
	default:
		return false
	}
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

func cloneOAuthDefinition(value *domain.ProviderOAuthDefinition) *domain.ProviderOAuthDefinition {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func DefaultProfileForProvider(definition domain.ProviderDefinition) domain.ProtocolProfileID {
	for _, protocol := range definition.Protocols {
		switch protocol {
		case domain.ProtocolOpenAIChat:
			return domain.ProfileOpenAIChat
		case domain.ProtocolOpenAIResponses:
			return domain.ProfileOpenAIResponses
		case domain.ProtocolClaudeMessages:
			return domain.ProfileAnthropicMessages
		case domain.ProtocolGemini:
			return domain.ProfileGoogleGemini
		}
	}
	return ""
}

func ValidateProfileForProvider(profileID domain.ProtocolProfileID, definition domain.ProviderDefinition) error {
	if profileID == "" {
		return nil
	}
	protocol, ok := domain.ProtocolForProfile(profileID)
	if !ok {
		return fmt.Errorf("unknown protocol profile %q", profileID)
	}
	if !supportsProtocol(definition, protocol) {
		return fmt.Errorf("provider %q does not support protocol profile %q", definition.ID, profileID)
	}
	return nil
}

func supportsProtocol(definition domain.ProviderDefinition, protocol domain.Protocol) bool {
	for _, current := range definition.Protocols {
		if current == protocol {
			return true
		}
	}
	return false
}
