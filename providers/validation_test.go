package providers

import (
	"strings"
	"testing"

	"done-hub/common/config"
	"done-hub/internal/gateway/domain"
	"done-hub/model"
)

func TestValidateChannelConfig(t *testing.T) {
	validBaseURL := "https://gateway.example.com/v1"
	validJSON := `{"gpt-public":"gpt-upstream"}`

	tests := []struct {
		name     string
		channel  *model.Channel
		creating bool
		wantPart string
	}{
		{
			name: "valid explicit provider",
			channel: &model.Channel{
				Type:         config.ChannelTypeCustom,
				Key:          "secret",
				BaseURL:      &validBaseURL,
				ModelMapping: &validJSON,
			},
			creating: true,
		},
		{
			name: "unknown provider",
			channel: &model.Channel{
				Type: 999,
				Key:  "secret",
			},
			creating: true,
			wantPart: "unknown provider",
		},
		{
			name: "missing credential",
			channel: &model.Channel{
				Type: config.ChannelTypeOpenAI,
			},
			creating: true,
			wantPart: "requires a credential",
		},
		{
			name: "invalid base url scheme",
			channel: &model.Channel{
				Type:    config.ChannelTypeCustom,
				Key:     "secret",
				BaseURL: stringPointerForValidation("file:///tmp/provider"),
			},
			creating: true,
			wantPart: "base URL",
		},
		{
			name: "required base url is missing",
			channel: &model.Channel{
				Type:    config.ChannelTypeCustom,
				Key:     "secret",
				BaseURL: stringPointerForValidation(" \n "),
			},
			creating: true,
			wantPart: "requires a base URL",
		},
		{
			name: "optional base url may be empty",
			channel: &model.Channel{
				Type: config.ChannelTypeOpenAI,
				Key:  "secret",
			},
			creating: true,
		},
		{
			name: "invalid policy json",
			channel: &model.Channel{
				Type:         config.ChannelTypeOpenAI,
				Key:          "secret",
				ModelMapping: stringPointerForValidation("[1,2]"),
			},
			creating: true,
			wantPart: "model_mapping",
		},
		{
			name: "protocol profile mismatch",
			channel: &model.Channel{
				Type:              config.ChannelTypeAnthropic,
				ProtocolProfileID: "google-gemini",
				Key:               "secret",
			},
			creating: true,
			wantPart: "does not support protocol profile",
		},
		{
			name: "native protocol rejects affinity",
			channel: &model.Channel{
				Type:            config.ChannelTypeXunfei,
				Key:             "secret",
				AffinityEnabled: true,
			},
			creating: true,
			wantPart: "does not support request affinity",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := ValidateChannelConfig(test.channel, test.creating)
			if test.wantPart == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantPart) {
				t.Fatalf("expected error containing %q, got %v", test.wantPart, err)
			}
		})
	}
}

func TestValidateChannelConfigAppliesProviderDefaults(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		wantOther   string
	}{
		{name: "Azure API version", channelType: config.ChannelTypeAzure, wantOther: "2024-05-01-preview"},
		{name: "Xunfei API version", channelType: config.ChannelTypeXunfei, wantOther: "v2.1"},
	}
	baseURL := "https://gateway.example.com"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			channel := &model.Channel{Type: test.channelType, Key: "secret"}
			if test.channelType == config.ChannelTypeAzure {
				channel.BaseURL = &baseURL
			}
			if err := ValidateChannelConfig(channel, true); err != nil {
				t.Fatalf("validate channel: %v", err)
			}
			if channel.Other != test.wantOther {
				t.Fatalf("default other: got %q want %q", channel.Other, test.wantOther)
			}

			channel.Other = "explicit"
			if err := ValidateChannelConfig(channel, false); err != nil {
				t.Fatalf("validate explicit value: %v", err)
			}
			if channel.Other != "explicit" {
				t.Fatalf("explicit value was overwritten: %q", channel.Other)
			}
		})
	}
}

func TestValidateChannelConfigRejectsForbiddenBaseURL(t *testing.T) {
	baseURL := "https://gateway.example.com"
	channel := &model.Channel{BaseURL: &baseURL}
	definition := domain.ProviderDefinition{ID: "forbidden", DisplayName: "Forbidden", BaseURLPolicy: domain.BaseURLPolicyForbidden}
	if err := validateProviderBaseURLs(channel, definition); err == nil || !strings.Contains(err.Error(), "does not allow") {
		t.Fatalf("expected forbidden base URL error, got %v", err)
	}
}

func TestValidateChannelConfigAssignsNativeDefaultProfile(t *testing.T) {
	channel := &model.Channel{Type: config.ChannelTypeAnthropic, Key: "secret"}
	if err := ValidateChannelConfig(channel, true); err != nil {
		t.Fatalf("validate channel: %v", err)
	}
	if channel.ProtocolProfileID != "anthropic-messages" {
		t.Fatalf("expected native Anthropic profile, got %q", channel.ProtocolProfileID)
	}
}

func TestAllowsEmptyCredential(t *testing.T) {
	if !AllowsEmptyCredential(config.ChannelTypeOllama) {
		t.Fatal("Ollama should allow an empty credential")
	}
	if AllowsEmptyCredential(config.ChannelTypeOpenAI) {
		t.Fatal("OpenAI should require a credential")
	}
}

func stringPointerForValidation(value string) *string {
	return &value
}
