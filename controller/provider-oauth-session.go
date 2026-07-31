package controller

import (
	"done-hub/common"
	"done-hub/common/cache"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type providerOAuthSessionStrategy struct {
	Start    gin.HandlerFunc
	Exchange gin.HandlerFunc
	Status   gin.HandlerFunc
	Callback gin.HandlerFunc
	Cancel   func(string)
}

var providerOAuthSessionStrategies = map[string]providerOAuthSessionStrategy{
	"gemini-cli": {
		Start: StartGeminiCliOAuth, Status: GetGeminiCliOAuthStatus, Callback: GeminiCliOAuthCallback,
		Cancel: func(sessionID string) {
			cache.DeleteCache(OAuthStateCachePrefix + sessionID)
			cache.DeleteCache(OAuthResultCachePrefix + sessionID)
		},
	},
	"antigravity": {
		Start: StartAntigravityOAuth, Status: GetAntigravityOAuthStatus, Callback: AntigravityOAuthCallback,
		Cancel: func(sessionID string) {
			cache.DeleteCache(AntigravityOAuthStateCachePrefix + sessionID)
			cache.DeleteCache(AntigravityOAuthResultCachePrefix + sessionID)
		},
	},
	"claude-code": {
		Start: StartClaudeCodeOAuth, Exchange: ClaudeCodeOAuthCallback,
		Cancel: func(sessionID string) {
			cache.DeleteCache(ClaudeCodeOAuthStateCachePrefix + sessionID)
		},
	},
	"codex": {
		Start: StartCodexOAuth, Exchange: CodexOAuthCallback,
		Cancel: func(sessionID string) {
			cache.DeleteCache(CodexOAuthStateCachePrefix + sessionID)
		},
	},
	"copilot": {
		Start: StartCopilotOAuth, Status: PollCopilotOAuth,
		Cancel: func(sessionID string) {
			copilotSessionMu.Lock()
			delete(copilotSessions, sessionID)
			copilotSessionMu.Unlock()
		},
	},
}

func providerOAuthSessionStrategyFor(c *gin.Context, operation string) (gin.HandlerFunc, bool) {
	provider := c.Param("provider")
	strategy, exists := providerOAuthSessionStrategies[provider]
	if !exists {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("OAuth provider %q is not supported", provider))
		return nil, false
	}

	var handler gin.HandlerFunc
	switch operation {
	case "start":
		handler = strategy.Start
	case "exchange":
		handler = strategy.Exchange
	case "status":
		handler = strategy.Status
	case "callback":
		handler = strategy.Callback
	}
	if handler == nil {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("OAuth provider %q does not support %s", provider, operation))
		return nil, false
	}
	return handler, true
}

// StartProviderOAuthSession starts the descriptor-selected OAuth strategy.
func StartProviderOAuthSession(c *gin.Context) {
	if handler, ok := providerOAuthSessionStrategyFor(c, "start"); ok {
		handler(c)
	}
}

// ExchangeProviderOAuthSession completes a manual-callback OAuth strategy.
func ExchangeProviderOAuthSession(c *gin.Context) {
	if handler, ok := providerOAuthSessionStrategyFor(c, "exchange"); ok {
		handler(c)
	}
}

// GetProviderOAuthSession polls browser-callback and device-code strategies.
func GetProviderOAuthSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	c.Params = append(c.Params, gin.Param{Key: "state", Value: sessionID})
	if handler, ok := providerOAuthSessionStrategyFor(c, "status"); ok {
		handler(c)
	}
}

// CancelProviderOAuthSession idempotently removes server-side session state.
func CancelProviderOAuthSession(c *gin.Context) {
	provider := c.Param("provider")
	strategy, exists := providerOAuthSessionStrategies[provider]
	if !exists {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("OAuth provider %q is not supported", provider))
		return
	}
	if strategy.Cancel == nil {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("OAuth provider %q does not support cancellation", provider))
		return
	}
	strategy.Cancel(c.Param("session_id"))
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    gin.H{"status": "canceled"},
	})
}

// ProviderOAuthSessionCallback is the sole unauthenticated callback entry.
func ProviderOAuthSessionCallback(c *gin.Context) {
	if handler, ok := providerOAuthSessionStrategyFor(c, "callback"); ok {
		handler(c)
	}
}

// ProviderOAuthSessionCallbackFor keeps externally registered redirect URIs
// stable while still dispatching through the unified session controller.
func ProviderOAuthSessionCallbackFor(provider string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Params = append(c.Params, gin.Param{Key: "provider", Value: provider})
		ProviderOAuthSessionCallback(c)
	}
}
