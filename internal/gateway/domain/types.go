package domain

import "time"

type ProviderID int

type Capability string

const (
	CapabilityChat             Capability = "chat"
	CapabilityCompletions      Capability = "completions"
	CapabilityResponses        Capability = "responses"
	CapabilityEmbeddings       Capability = "embeddings"
	CapabilityModerations      Capability = "moderations"
	CapabilityImageGenerations Capability = "image_generations"
	CapabilityImageEdits       Capability = "image_edits"
	CapabilityImageVariations  Capability = "image_variations"
	CapabilitySpeech           Capability = "speech"
	CapabilityTranscriptions   Capability = "transcriptions"
	CapabilityTranslations     Capability = "translations"
	CapabilityRerank           Capability = "rerank"
	CapabilityRealtime         Capability = "realtime"
	CapabilityTask             Capability = "task"
	CapabilityPassthrough      Capability = "passthrough"
)

type Protocol string

const (
	ProtocolOpenAIChat      Protocol = "openai_chat"
	ProtocolOpenAIResponses Protocol = "openai_responses"
	ProtocolClaudeMessages  Protocol = "claude_messages"
	ProtocolGemini          Protocol = "gemini"
	ProtocolNative          Protocol = "native"
)

type ProtocolProfileID string

const (
	ProfileOpenAIChat        ProtocolProfileID = "openai-chat-completions"
	ProfileOpenAIResponses   ProtocolProfileID = "openai-responses"
	ProfileAnthropicMessages ProtocolProfileID = "anthropic-messages"
	ProfileGoogleGemini      ProtocolProfileID = "google-gemini"
)

func ProtocolForProfile(profileID ProtocolProfileID) (Protocol, bool) {
	switch profileID {
	case ProfileOpenAIChat:
		return ProtocolOpenAIChat, true
	case ProfileOpenAIResponses:
		return ProtocolOpenAIResponses, true
	case ProfileAnthropicMessages:
		return ProtocolClaudeMessages, true
	case ProfileGoogleGemini:
		return ProtocolGemini, true
	default:
		return "", false
	}
}

type AuthMode string

const (
	AuthModeAPIKey         AuthMode = "api_key"
	AuthModeBearer         AuthMode = "bearer"
	AuthModeOAuth          AuthMode = "oauth"
	AuthModeServiceAccount AuthMode = "service_account"
	AuthModeSignedRequest  AuthMode = "signed_request"
	AuthModeOpaque         AuthMode = "opaque"
	AuthModeNone           AuthMode = "none"
)

type BaseURLPolicy string

const (
	BaseURLPolicyOptional  BaseURLPolicy = "optional"
	BaseURLPolicyRequired  BaseURLPolicy = "required"
	BaseURLPolicyForbidden BaseURLPolicy = "forbidden"
)

type OAuthFlow string

const (
	OAuthFlowBrowserCallback OAuthFlow = "browser_callback"
	OAuthFlowManualCallback  OAuthFlow = "manual_callback"
	OAuthFlowDeviceCode      OAuthFlow = "device_code"
)

type ProviderOAuthDefinition struct {
	Provider string    `json:"provider"`
	Flow     OAuthFlow `json:"flow"`
}

type ProviderDefinition struct {
	ID               string                   `json:"id"`
	DisplayName      string                   `json:"display_name"`
	ChannelType      ProviderID               `json:"channel_type"`
	Capabilities     []Capability             `json:"capabilities"`
	Protocols        []Protocol               `json:"protocols"`
	AuthModes        []AuthMode               `json:"auth_modes"`
	OpenAICompatible bool                     `json:"openai_compatible"`
	BaseURLPolicy    BaseURLPolicy            `json:"base_url_policy"`
	Defaults         map[string]string        `json:"defaults,omitempty"`
	OAuth            *ProviderOAuthDefinition `json:"oauth,omitempty"`
}

type ProviderVariant struct {
	ProviderID     string                   `json:"provider_id"`
	ChannelType    ProviderID               `json:"channel_type"`
	DisplayName    string                   `json:"display_name"`
	DefaultBaseURL string                   `json:"default_base_url,omitempty"`
	AuthModes      []AuthMode               `json:"auth_modes"`
	BaseURLPolicy  BaseURLPolicy            `json:"base_url_policy"`
	Defaults       map[string]string        `json:"defaults,omitempty"`
	OAuth          *ProviderOAuthDefinition `json:"oauth,omitempty"`
}

type ProtocolProbeDefinition struct {
	ModelDiscoveryPath string `json:"model_discovery_path,omitempty"`
	RequestPath        string `json:"request_path"`
	Streaming          bool   `json:"streaming"`
}

type ConnectionProfileDefinition struct {
	ID             ProtocolProfileID       `json:"id"`
	DisplayName    string                  `json:"display_name"`
	Description    string                  `json:"description"`
	Protocol       Protocol                `json:"protocol"`
	Featured       bool                    `json:"featured"`
	DisplayOrder   int                     `json:"display_order"`
	CatalogSection string                  `json:"catalog_section"`
	Variants       []ProviderVariant       `json:"variants"`
	Probe          ProtocolProbeDefinition `json:"probe"`
}

func (d ProviderDefinition) Supports(capability Capability) bool {
	for _, current := range d.Capabilities {
		if current == capability {
			return true
		}
	}
	return false
}

type CredentialRef struct {
	ID       string
	AuthMode AuthMode
}

type Endpoint struct {
	ID                int
	ProviderID        ProviderID
	Name              string
	BaseURL           string
	ProxyRef          string
	Group             string
	Tags              []string
	Weight            int
	Priority          int
	Enabled           bool
	ProtocolProfileID ProtocolProfileID
	CredentialRef     CredentialRef
}

type ModelRoute struct {
	PublicModel   string
	UpstreamModel string
	Capability    Capability
	Protocol      Protocol
	EndpointID    int
	Group         string
	PriceVersion  string
}

type RequestContext struct {
	RequestID      string
	UserID         int
	TokenID        int
	Group          string
	RequestedModel string
	Capability     Capability
	Protocol       Protocol
	Stream         bool
	StartedAt      time.Time
}

type RoutePlan struct {
	Request   RequestContext
	Model     ModelRoute
	Endpoints []Endpoint
}

type Attempt struct {
	RequestID string
	Number    int
	Endpoint  Endpoint
	Model     ModelRoute
	StartedAt time.Time
}

type Usage struct {
	InputTokens     int
	OutputTokens    int
	CachedTokens    int
	ReasoningTokens int
}

func (u Usage) Add(delta Usage) Usage {
	u.InputTokens += delta.InputTokens
	u.OutputTokens += delta.OutputTokens
	u.CachedTokens += delta.CachedTokens
	u.ReasoningTokens += delta.ReasoningTokens
	return u
}

type AttemptResult struct {
	Attempt          Attempt
	Usage            Usage
	UpstreamAccepted bool
	OutputStarted    bool
	CompletedAt      time.Time
}

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeCanceled  Outcome = "canceled"
)
