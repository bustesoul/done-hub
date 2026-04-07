package controller

import (
	"done-hub/common"
	"done-hub/common/logger"
	"done-hub/providers/copilot"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ─── In-process session store ─────────────────────────────────────────────────

// copilotDeviceSession tracks a pending GitHub device OAuth flow.
type copilotDeviceSession struct {
	DeviceCode      string    `json:"device_code"`
	UserCode        string    `json:"user_code"`
	VerificationURI string    `json:"verification_uri"`
	ExpiresAt       time.Time `json:"expires_at"`
	Interval        int       `json:"interval"`
	NextPollAt      time.Time `json:"next_poll_at"`
}

var (
	copilotSessionMu sync.Mutex
	copilotSessions  = make(map[string]*copilotDeviceSession)
)

func copilotSessionID() string {
	return fmt.Sprintf("copilot_%d", time.Now().UnixNano())
}

func copilotCleanupExpired() {
	copilotSessionMu.Lock()
	defer copilotSessionMu.Unlock()
	now := time.Now()
	for id, s := range copilotSessions {
		if now.After(s.ExpiresAt) {
			delete(copilotSessions, id)
		}
	}
}

// ─── HTTP handler: start device flow ─────────────────────────────────────────

// StartCopilotOAuth initiates the GitHub device OAuth flow.
// POST /api/copilot/oauth/device-code
//
// Response JSON:
//
//	{
//	  "success": true,
//	  "data": {
//	    "session_id": "...",
//	    "user_code": "ABCD-1234",
//	    "verification_uri": "https://github.com/login/device",
//	    "expires_in": 900,
//	    "interval": 5
//	  }
//	}
func StartCopilotOAuth(c *gin.Context) {
	httpClient := &http.Client{Timeout: 30 * time.Second}

	resp, err := copilot.RequestDeviceCode(httpClient)
	if err != nil {
		logger.SysError(fmt.Sprintf("[Copilot OAuth] RequestDeviceCode failed: %s", err.Error()))
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("failed to start device flow: %w", err))
		return
	}

	sessionID := copilotSessionID()
	session := &copilotDeviceSession{
		DeviceCode:      resp.DeviceCode,
		UserCode:        resp.UserCode,
		VerificationURI: resp.VerificationURI,
		ExpiresAt:       time.Now().Add(time.Duration(resp.ExpiresIn) * time.Second),
		Interval:        resp.Interval,
		NextPollAt:      time.Now(),
	}

	copilotSessionMu.Lock()
	copilotSessions[sessionID] = session
	copilotSessionMu.Unlock()

	go copilotCleanupExpired()

	logger.SysLog(fmt.Sprintf("[Copilot OAuth] device flow started, session=%s user_code=%s", sessionID, resp.UserCode))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"session_id":       sessionID,
			"user_code":        resp.UserCode,
			"verification_uri": resp.VerificationURI,
			"expires_in":       resp.ExpiresIn,
			"interval":         resp.Interval,
		},
	})
}

// ─── HTTP handler: poll ───────────────────────────────────────────────────────

// PollCopilotOAuth polls for the access token after the user has authorized.
// POST /api/copilot/oauth/poll
//
// Request JSON:  { "session_id": "..." }
//
// Response JSON (pending):
//
//	{ "success": true, "data": { "status": "pending" } }
//
// Response JSON (success):
//
//	{
//	  "success": true,
//	  "data": {
//	    "status": "success",
//	    "github_token": "gho_...",
//	    "github_login": "username",
//	    "github_name":  "Display Name",
//	    "github_id":    12345
//	  }
//	}
func PollCopilotOAuth(c *gin.Context) {
	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.APIRespondWithError(c, http.StatusOK, err)
		return
	}
	if req.SessionID == "" {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("session_id is required"))
		return
	}

	copilotSessionMu.Lock()
	session, ok := copilotSessions[req.SessionID]
	copilotSessionMu.Unlock()

	if !ok {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("session not found or expired"))
		return
	}

	if time.Now().After(session.ExpiresAt) {
		copilotSessionMu.Lock()
		delete(copilotSessions, req.SessionID)
		copilotSessionMu.Unlock()
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("device code expired"))
		return
	}

	// Enforce polling interval.
	copilotSessionMu.Lock()
	nextPoll := session.NextPollAt
	copilotSessionMu.Unlock()

	if time.Now().Before(nextPoll) {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    gin.H{"status": "pending"},
		})
		return
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}

	tokenResp, err := copilot.PollAccessToken(httpClient, session.DeviceCode)
	if err != nil {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("poll failed: %w", err))
		return
	}

	// Handle GitHub error states.
	if tokenResp.Error != "" {
		switch tokenResp.Error {
		case "authorization_pending":
			copilotSessionMu.Lock()
			session.NextPollAt = time.Now().Add(time.Duration(session.Interval) * time.Second)
			copilotSessionMu.Unlock()
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "pending"}})
		case "slow_down":
			newInterval := session.Interval + 5
			if tokenResp.Interval > 0 {
				newInterval = tokenResp.Interval
			}
			copilotSessionMu.Lock()
			session.Interval = newInterval
			session.NextPollAt = time.Now().Add(time.Duration(newInterval) * time.Second)
			copilotSessionMu.Unlock()
			c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"status": "pending"}})
		case "expired_token":
			copilotSessionMu.Lock()
			delete(copilotSessions, req.SessionID)
			copilotSessionMu.Unlock()
			common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("device code expired"))
		default:
			common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("oauth error: %s: %s", tokenResp.Error, tokenResp.ErrorDesc))
		}
		return
	}

	if tokenResp.AccessToken == "" {
		common.APIRespondWithError(c, http.StatusOK, fmt.Errorf("empty access token in response"))
		return
	}

	// Clean up session.
	copilotSessionMu.Lock()
	delete(copilotSessions, req.SessionID)
	copilotSessionMu.Unlock()

	// Fetch GitHub user profile.
	user, userErr := copilot.GetGitHubUser(httpClient, tokenResp.AccessToken)
	if userErr != nil {
		logger.SysError(fmt.Sprintf("[Copilot OAuth] GetGitHubUser failed: %s", userErr.Error()))
	}

	// Verify Copilot access by performing a token exchange.
	_, exchangeErr := copilot.ExchangeToken(c.Request.Context(), httpClient, tokenResp.AccessToken)
	if exchangeErr != nil {
		login := ""
		if user != nil {
			login = user.Login
		}
		logger.SysError(fmt.Sprintf("[Copilot OAuth] Copilot token exchange failed for user %s: %s", login, exchangeErr.Error()))
		common.APIRespondWithError(c, http.StatusOK,
			fmt.Errorf("GitHub token obtained but Copilot access is not available: %w (user: %s)", exchangeErr, login))
		return
	}

	data := gin.H{
		"status":       "success",
		"github_token": tokenResp.AccessToken,
	}
	if user != nil {
		data["github_login"] = user.Login
		data["github_name"] = user.Name
		data["github_id"] = user.ID
		logger.SysLog(fmt.Sprintf("[Copilot OAuth] completed for user %s (id=%d)", user.Login, user.ID))
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "授权成功",
		"data":    data,
	})
}
