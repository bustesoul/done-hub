package copilot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ─── GitHub Copilot API constants ────────────────────────────────────────────

const (
	// TokenExchangeURL is the GitHub endpoint for exchanging a GitHub token for a Copilot token.
	TokenExchangeURL = "https://api.github.com/copilot_internal/v2/token"

	// CopilotAPIBase is the canonical Copilot API base URL.
	CopilotAPIBase = "https://api.githubcopilot.com"

	// Plan-specific base URLs used for /chat/completions routing.
	CopilotAPIBaseIndividual = "https://api.individual.githubcopilot.com"
	CopilotAPIBaseBusiness   = "https://api.business.githubcopilot.com"
	CopilotAPIBaseEnterprise = "https://api.enterprise.githubcopilot.com"
)

const (
	// PlanType constants for GitHub Copilot account plans.
	PlanTypeIndividual        = "individual"
	PlanTypeIndividualFree    = "individual_free"
	PlanTypeIndividualPro     = "individual_pro"
	PlanTypeIndividualProPlus = "individual_pro_plus"
	PlanTypeBusiness          = "business"
	PlanTypeEnterprise        = "enterprise"
)

const (
	// DefaultEditorVersion is the editor version header sent to the Copilot API.
	DefaultEditorVersion = "vscode/1.98.1"
	// DefaultEditorPluginVersion is the editor plugin version header sent to the Copilot API.
	DefaultEditorPluginVersion = "copilot-chat/0.26.7"
	// DefaultUserAgent is the user agent string sent to the Copilot API.
	DefaultUserAgent = "GitHubCopilotChat/0.26.7"
	// DefaultGitHubAPIVersion is the GitHub API version header.
	DefaultGitHubAPIVersion = "2025-04-01"
	// DefaultCopilotIntegrationID is the integration identifier sent to the Copilot API.
	DefaultCopilotIntegrationID = "vscode-chat"
	// DefaultOpenAIIntent is the OpenAI intent header sent to the Copilot API.
	DefaultOpenAIIntent = "conversation-panel"
)

// GitHub Device OAuth constants (VS Code's public client ID)
const (
	DeviceOAuthClientID = "Iv1.b507a08c87ecfe98"
	DeviceCodeURL       = "https://github.com/login/device/code"
	AccessTokenURL      = "https://github.com/login/oauth/access_token"
	GitHubUserURL       = "https://api.github.com/user"
)

// ─── Token types ─────────────────────────────────────────────────────────────

// TokenExchangeResponse is the response from the Copilot token exchange endpoint.
type TokenExchangeResponse struct {
	Token        string `json:"token"`
	ExpiresAt    int64  `json:"expires_at"`
	RefreshIn    int64  `json:"refresh_in"`
	ErrorMessage string `json:"error_description,omitempty"`
}

// CopilotToken holds a cached Copilot API token with its refresh metadata.
type CopilotToken struct {
	Token     string
	ExpiresAt time.Time
	RefreshAt time.Time
}

// IsExpired reports whether the token has expired (with 60s safety margin).
func (t *CopilotToken) IsExpired() bool {
	return time.Now().Add(60 * time.Second).After(t.ExpiresAt)
}

// ShouldRefresh reports whether the token should be proactively refreshed.
func (t *CopilotToken) ShouldRefresh() bool {
	return time.Now().After(t.RefreshAt)
}

// ─── Device OAuth types ───────────────────────────────────────────────────────

// DeviceCodeResponse is the response from GitHub's device code endpoint.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// AccessTokenResponse is the response from GitHub's access token endpoint.
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
	ErrorDesc   string `json:"error_description,omitempty"`
	Interval    int    `json:"interval,omitempty"`
}

// GitHubUser is a minimal GitHub user profile.
type GitHubUser struct {
	Login     string `json:"login"`
	ID        int64  `json:"id"`
	AvatarURL string `json:"avatar_url"`
	Name      string `json:"name"`
}

// ─── ChatBaseURLForPlan ───────────────────────────────────────────────────────

// ChatBaseURLForPlan returns the appropriate /chat/completions base URL for the given plan_type.
func ChatBaseURLForPlan(planType string) string {
	switch planType {
	case PlanTypeIndividual, PlanTypeIndividualFree, PlanTypeIndividualPro, PlanTypeIndividualProPlus:
		return CopilotAPIBaseIndividual
	case PlanTypeBusiness:
		return CopilotAPIBaseBusiness
	case PlanTypeEnterprise:
		return CopilotAPIBaseEnterprise
	default:
		return CopilotAPIBase
	}
}

// ─── Token exchange ───────────────────────────────────────────────────────────

// ExchangeToken exchanges a GitHub personal access token for a short-lived Copilot API token.
func ExchangeToken(ctx context.Context, httpClient *http.Client, githubToken string) (*CopilotToken, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, TokenExchangeURL, nil)
	if err != nil {
		return nil, fmt.Errorf("copilot token exchange: build request: %w", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("editor-version", DefaultEditorVersion)
	req.Header.Set("editor-plugin-version", DefaultEditorPluginVersion)
	req.Header.Set("User-Agent", DefaultUserAgent)
	req.Header.Set("x-github-api-version", DefaultGitHubAPIVersion)
	req.Header.Set("x-vscode-user-agent-library-version", "electron-fetch")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot token exchange: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("copilot token exchange: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("copilot token exchange: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenExchangeResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("copilot token exchange: parse response: %w", err)
	}

	if tokenResp.Token == "" {
		errMsg := tokenResp.ErrorMessage
		if errMsg == "" {
			errMsg = "empty token in response"
		}
		return nil, fmt.Errorf("copilot token exchange: %s", errMsg)
	}

	now := time.Now()
	expiresAt := now.Add(30 * time.Minute) // default fallback
	if tokenResp.ExpiresAt > 0 {
		expiresAt = time.Unix(tokenResp.ExpiresAt, 0)
	}

	refreshIn := tokenResp.RefreshIn
	if refreshIn <= 0 {
		refreshIn = int64(time.Until(expiresAt).Seconds()) - 60
	}
	if refreshIn < 30 {
		refreshIn = 30
	}
	refreshAt := now.Add(time.Duration(refreshIn-60) * time.Second)

	return &CopilotToken{
		Token:     tokenResp.Token,
		ExpiresAt: expiresAt,
		RefreshAt: refreshAt,
	}, nil
}

// ─── Device OAuth flow ────────────────────────────────────────────────────────

// RequestDeviceCode initiates the GitHub device code flow.
func RequestDeviceCode(httpClient *http.Client) (*DeviceCodeResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	data := url.Values{
		"client_id": {DeviceOAuthClientID},
		"scope":     {"read:user"},
	}

	req, err := http.NewRequest(http.MethodPost, DeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("device code request: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("device code request: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result DeviceCodeResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("device code request: parse response: %w", err)
	}

	return &result, nil
}

// PollAccessToken polls GitHub for the access token using the device code.
func PollAccessToken(httpClient *http.Client, deviceCode string) (*AccessTokenResponse, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	data := url.Values{
		"client_id":   {DeviceOAuthClientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequest(http.MethodPost, AccessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("poll access token: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll access token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("poll access token: read body: %w", err)
	}

	var result AccessTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("poll access token: parse response: %w (body=%s)", err, string(body))
	}

	return &result, nil
}

// GetGitHubUser fetches the authenticated user's profile.
func GetGitHubUser(httpClient *http.Client, accessToken string) (*GitHubUser, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	req, err := http.NewRequest(http.MethodGet, GitHubUserURL, nil)
	if err != nil {
		return nil, fmt.Errorf("get github user: build request: %w", err)
	}
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get github user: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("get github user: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get github user: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var user GitHubUser
	if err := json.Unmarshal(body, &user); err != nil {
		return nil, fmt.Errorf("get github user: parse response: %w", err)
	}

	return &user, nil
}
