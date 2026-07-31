package providers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"done-hub/internal/gateway/catalog"
	"done-hub/internal/gateway/domain"
	"done-hub/model"
)

func ValidateChannelConfig(channel *model.Channel, creating bool) error {
	if channel == nil {
		return fmt.Errorf("channel is nil")
	}
	definition, err := GetProviderDefinition(channel.Type)
	if err != nil {
		return fmt.Errorf("unknown provider type %d: %w", channel.Type, err)
	}
	if len(definition.Capabilities) == 0 {
		return fmt.Errorf("provider %q has no declared capability", definition.ID)
	}
	if err := applyProviderDefaults(channel, definition); err != nil {
		return err
	}
	if strings.TrimSpace(channel.ProtocolProfileID) == "" {
		channel.ProtocolProfileID = string(catalog.DefaultProfileForProvider(definition))
	}
	if err := catalog.ValidateProfileForProvider(domain.ProtocolProfileID(channel.ProtocolProfileID), definition); err != nil {
		return err
	}

	if creating && strings.TrimSpace(channel.Key) == "" && !supportsAuthMode(definition, domain.AuthModeNone) {
		return fmt.Errorf("provider %q requires a credential", definition.DisplayName)
	}
	if err := validateProviderBaseURLs(channel, definition); err != nil {
		return err
	}

	for field, value := range map[string]*string{
		"model_mapping":    channel.ModelMapping,
		"model_headers":    channel.ModelHeaders,
		"custom_parameter": channel.CustomParameter,
		"header_override":  channel.HeaderOverride,
	} {
		if err := validateJSONObject(field, value); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderBaseURLs(channel *model.Channel, definition domain.ProviderDefinition) error {
	baseURLConfigured := false
	for _, rawBaseURL := range strings.Split(channel.GetBaseURL(), "\n") {
		rawBaseURL = strings.TrimSpace(rawBaseURL)
		if rawBaseURL != "" {
			baseURLConfigured = true
			parsed, parseErr := url.ParseRequestURI(rawBaseURL)
			if parseErr != nil || parsed.Host == "" {
				return fmt.Errorf("provider %q base URL is invalid", definition.DisplayName)
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return fmt.Errorf("provider %q base URL must use http or https", definition.DisplayName)
			}
			if parsed.User != nil || parsed.Fragment != "" {
				return fmt.Errorf("provider %q base URL must not contain user info or fragment", definition.DisplayName)
			}
		}
	}
	switch definition.BaseURLPolicy {
	case "", domain.BaseURLPolicyOptional:
	case domain.BaseURLPolicyRequired:
		if !baseURLConfigured {
			return fmt.Errorf("provider %q requires a base URL", definition.DisplayName)
		}
	case domain.BaseURLPolicyForbidden:
		if baseURLConfigured {
			return fmt.Errorf("provider %q does not allow a base URL", definition.DisplayName)
		}
	default:
		return fmt.Errorf("provider %q has invalid base URL policy %q", definition.ID, definition.BaseURLPolicy)
	}
	return nil
}

func applyProviderDefaults(channel *model.Channel, definition domain.ProviderDefinition) error {
	for field, value := range definition.Defaults {
		switch field {
		case "other":
			if strings.TrimSpace(channel.Other) == "" {
				channel.Other = value
			}
		case "base_url":
			if strings.TrimSpace(channel.GetBaseURL()) == "" {
				defaultValue := value
				channel.BaseURL = &defaultValue
			}
		case "test_model":
			if strings.TrimSpace(channel.TestModel) == "" {
				channel.TestModel = value
			}
		case "protocol_profile_id":
			if strings.TrimSpace(channel.ProtocolProfileID) == "" {
				channel.ProtocolProfileID = value
			}
		case "name":
			if strings.TrimSpace(channel.Name) == "" {
				channel.Name = value
			}
		default:
			return fmt.Errorf("provider %q has unsupported default field %q", definition.ID, field)
		}
	}
	return nil
}

func AllowsEmptyCredential(channelType int) bool {
	definition, err := GetProviderDefinition(channelType)
	return err == nil && supportsAuthMode(definition, domain.AuthModeNone)
}

func ResolveCredentialAuthMode(channelType int, requested string) (domain.AuthMode, error) {
	definition, err := GetProviderDefinition(channelType)
	if err != nil {
		return "", err
	}
	mode := domain.AuthMode(strings.TrimSpace(requested))
	if mode == "" || mode == domain.AuthModeOpaque {
		for _, candidate := range definition.AuthModes {
			if candidate != domain.AuthModeNone {
				return candidate, nil
			}
		}
		return "", fmt.Errorf("provider %q has no credential auth mode", definition.ID)
	}
	if !supportsAuthMode(definition, mode) || mode == domain.AuthModeNone {
		return "", fmt.Errorf("provider %q does not support credential auth mode %q", definition.ID, mode)
	}
	return mode, nil
}

func supportsAuthMode(definition domain.ProviderDefinition, mode domain.AuthMode) bool {
	for _, current := range definition.AuthModes {
		if current == mode {
			return true
		}
	}
	return false
}

func validateJSONObject(field string, value *string) error {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	var object map[string]any
	if err := json.Unmarshal([]byte(*value), &object); err != nil {
		return fmt.Errorf("%s must be a JSON object: %w", field, err)
	}
	if object == nil {
		return fmt.Errorf("%s must be a JSON object", field)
	}
	return nil
}
