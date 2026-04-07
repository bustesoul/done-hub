package copilot

import (
	"context"
	"crypto/tls"
	"done-hub/common/logger"
	"done-hub/common/requester"
	"done-hub/model"
	"done-hub/providers/base"
	"done-hub/providers/openai"
	"done-hub/types"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"
)

// ─── Token cache (process-local, shared across all CopilotProvider instances) ─

var (
	tokenCacheMu sync.RWMutex
	tokenCache   = make(map[int]*CopilotToken) // channelID → cached token
	sfGroup      singleflight.Group
)

// getOrExchangeToken returns a valid Copilot API token for the given channel.
// It uses a read-first + singleflight pattern to avoid thundering-herd.
func getOrExchangeToken(ctx context.Context, httpClient *http.Client, channelID int, githubToken string) (string, error) {
	// Fast path: valid cached token.
	tokenCacheMu.RLock()
	cached, ok := tokenCache[channelID]
	tokenCacheMu.RUnlock()

	if ok && cached != nil && !cached.IsExpired() && !cached.ShouldRefresh() {
		return cached.Token, nil
	}

	// Slow path: exchange under singleflight.
	key := fmt.Sprintf("exchange:%d", channelID)
	val, err, _ := sfGroup.Do(key, func() (any, error) {
		// Re-check under lock after entering singleflight.
		tokenCacheMu.RLock()
		cached, ok := tokenCache[channelID]
		tokenCacheMu.RUnlock()
		if ok && cached != nil && !cached.IsExpired() && !cached.ShouldRefresh() {
			return cached.Token, nil
		}

		// Keep fallback in case exchange fails but token is still alive.
		var fallback string
		if ok && cached != nil && !cached.IsExpired() {
			fallback = cached.Token
		}

		exchangeCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()

		newToken, err := ExchangeToken(exchangeCtx, httpClient, githubToken)
		if err != nil {
			logger.SysError(fmt.Sprintf("[Copilot] token exchange failed for channel %d: %s", channelID, err.Error()))
			if fallback != "" {
				return fallback, nil
			}
			return "", fmt.Errorf("copilot token exchange: %w", err)
		}

		tokenCacheMu.Lock()
		tokenCache[channelID] = newToken
		tokenCacheMu.Unlock()

		logger.SysLog(fmt.Sprintf("[Copilot] token refreshed for channel %d, expires at %s",
			channelID, newToken.ExpiresAt.Format(time.RFC3339)))

		return newToken.Token, nil
	})
	if err != nil {
		return "", err
	}

	token, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("copilot token provider: unexpected result type")
	}
	return token, nil
}

// invalidateToken removes the cached token for the given channel.
func invalidateToken(channelID int) {
	tokenCacheMu.Lock()
	delete(tokenCache, channelID)
	tokenCacheMu.Unlock()
}

// ─── HTTP client (HTTP/1.1 forced to avoid 421 Misdirected Request) ──────────

var copilotHTTPClient = &http.Client{
	Timeout: 5 * time.Minute,
	Transport: &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// Explicitly advertise only HTTP/1.1 to prevent h2 negotiation.
			NextProtos: []string{"http/1.1"},
		},
		ForceAttemptHTTP2:   false,
		DisableKeepAlives:   false,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
	},
}

// ─── Factory ──────────────────────────────────────────────────────────────────

type CopilotProviderFactory struct{}

func (f CopilotProviderFactory) Create(channel *model.Channel) base.ProviderInterface {
	provider := &CopilotProvider{
		OpenAIProvider: openai.OpenAIProvider{
			BaseProvider: base.BaseProvider{
				Config:          getConfig(),
				Channel:         channel,
				Requester:       requester.NewHTTPRequester(channel.GetProxy(), copilotErrorHandle),
				SupportResponse: true,
			},
			SupportStreamOptions: false,
		},
		githubToken: strings.TrimSpace(channel.Key),
		planType:    "",
	}

	// Parse optional plan_type from channel.Other JSON: {"plan_type": "business"}
	if channel.Other != "" {
		parsePlanType(provider, channel.Other)
	}

	return provider
}

func getConfig() base.ProviderConfig {
	return base.ProviderConfig{
		BaseURL:         CopilotAPIBase,
		ChatCompletions: "/chat/completions",
		Responses:       "/responses",
	}
}

// ─── Provider ─────────────────────────────────────────────────────────────────

// CopilotProvider implements ChatInterface and ResponsesInterface for GitHub Copilot.
type CopilotProvider struct {
	openai.OpenAIProvider

	// githubToken is the long-lived GitHub personal access token stored in the channel key.
	githubToken string
	// planType is an optional plan specifier ("individual", "business", "enterprise").
	planType string
}

// parsePlanType reads plan_type from the channel's Other JSON field.
func parsePlanType(p *CopilotProvider, otherJSON string) {
	var cfg struct {
		PlanType string `json:"plan_type"`
	}
	if err := json.Unmarshal([]byte(otherJSON), &cfg); err == nil && cfg.PlanType != "" {
		p.planType = cfg.PlanType
	}
}

// ─── Token access ─────────────────────────────────────────────────────────────

// GetCopilotToken returns a valid short-lived Copilot API token, exchanging
// it from the GitHub token when needed.
func (p *CopilotProvider) GetCopilotToken() (string, error) {
	if p.githubToken == "" {
		return "", fmt.Errorf("copilot: channel key (GitHub token) is empty")
	}

	var ctx context.Context
	if p.Context != nil {
		ctx = p.Context.Request.Context()
	} else {
		ctx = context.Background()
	}

	return getOrExchangeToken(ctx, copilotHTTPClient, p.Channel.Id, p.githubToken)
}

// ─── Request headers ──────────────────────────────────────────────────────────

// GetRequestHeaders builds Copilot-specific request headers (satisfies ProviderInterface).
func (p *CopilotProvider) GetRequestHeaders() map[string]string {
	headers, _ := p.buildCopilotHeaders("user")
	if headers == nil {
		headers = make(map[string]string)
		p.CommonRequestHeaders(headers)
	}
	return headers
}

// buildCopilotHeaders returns a fully-populated Copilot headers map with the
// given X-Initiator value, or an error if the token cannot be obtained.
func (p *CopilotProvider) buildCopilotHeaders(initiator string) (map[string]string, *types.OpenAIErrorWithStatusCode) {
	token, err := p.GetCopilotToken()
	if err != nil {
		if p.Context != nil {
			logger.LogError(p.Context.Request.Context(), "[Copilot] failed to get token: "+err.Error())
		} else {
			logger.SysError("[Copilot] failed to get token: " + err.Error())
		}
		return nil, &types.OpenAIErrorWithStatusCode{
			OpenAIError: types.OpenAIError{
				Message: err.Error(),
				Type:    "copilot_auth_error",
				Code:    "copilot_auth_error",
			},
			StatusCode: http.StatusUnauthorized,
		}
	}

	headers := make(map[string]string)
	p.CommonRequestHeaders(headers)

	headers["Authorization"] = "Bearer " + token
	headers["Content-Type"] = "application/json"
	headers["editor-version"] = DefaultEditorVersion
	headers["editor-plugin-version"] = DefaultEditorPluginVersion
	headers["User-Agent"] = DefaultUserAgent
	headers["x-github-api-version"] = DefaultGitHubAPIVersion
	headers["copilot-integration-id"] = DefaultCopilotIntegrationID
	headers["openai-intent"] = DefaultOpenAIIntent
	headers["x-vscode-user-agent-library-version"] = "electron-fetch"
	headers["x-request-id"] = uuid.New().String()
	headers["X-Initiator"] = initiator

	return headers, nil
}

// ─── URL helpers ──────────────────────────────────────────────────────────────

// chatBaseURL returns the appropriate base URL for /chat/completions based on plan_type.
func (p *CopilotProvider) chatBaseURL() string {
	if p.planType != "" {
		return ChatBaseURLForPlan(p.planType)
	}
	return CopilotAPIBase
}

// ─── Initiator detection ──────────────────────────────────────────────────────

// copilotInitiatorFromMessages detects whether the message list contains
// assistant or tool messages (multi-turn), returning "agent" or "user".
func copilotInitiatorFromMessages(messages []types.ChatCompletionMessage) string {
	for _, m := range messages {
		if m.Role == "assistant" || m.Role == "tool" {
			return "agent"
		}
	}
	return "user"
}

// ─── Error handler ────────────────────────────────────────────────────────────

// copilotErrorHandle is the requester error handler for the Copilot provider.
func copilotErrorHandle(resp *http.Response) *types.OpenAIError {
	return openai.RequestErrorHandle(resp)
}

// handleTokenError wraps a token-related error into an OpenAIErrorWithStatusCode.
func (p *CopilotProvider) handleTokenError(err error) *types.OpenAIErrorWithStatusCode {
	return &types.OpenAIErrorWithStatusCode{
		OpenAIError: types.OpenAIError{
			Message: err.Error(),
			Type:    "copilot_auth_error",
			Code:    "copilot_auth_error",
		},
		StatusCode: http.StatusUnauthorized,
		LocalError: false,
	}
}
