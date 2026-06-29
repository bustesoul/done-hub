package claudecode

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

type SubscriptionWindow struct {
	Label            string  `json:"label"`
	UsedPercent      float64 `json:"used_percent"`
	RemainingPercent float64 `json:"remaining_percent"`
	ResetAt          int64   `json:"reset_at"`
	WindowSeconds    int     `json:"window_seconds"`
}

// SubscriptionQuota probes GET /v1/models and reads the unified rate-limit
// headers that Anthropic includes in every response for Claude Code OAuth users.
// Headers: anthropic-ratelimit-unified-{limit,remaining,reset}
func (p *ClaudeCodeProvider) SubscriptionQuota() ([]SubscriptionWindow, error) {
	token, err := p.GetToken()
	if err != nil {
		return nil, fmt.Errorf("获取 Token 失败: %w", err)
	}

	headers := map[string]string{
		"Authorization":     "Bearer " + token,
		"anthropic-version": "2023-06-01",
		"Accept":            "application/json",
	}

	url := p.GetFullRequestURL("/v1/models")
	req, err := p.Requester.NewRequest("GET", url, p.Requester.WithHeader(headers))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// Pass nil response to skip body parsing; we only need the response headers.
	resp, errWithCode := p.Requester.SendRequest(req, nil, false)
	if errWithCode != nil {
		return nil, errors.New(errWithCode.OpenAIError.Message)
	}

	limitStr := resp.Header.Get("anthropic-ratelimit-unified-limit")
	remainingStr := resp.Header.Get("anthropic-ratelimit-unified-remaining")
	resetStr := resp.Header.Get("anthropic-ratelimit-unified-reset")

	if limitStr == "" || remainingStr == "" {
		return nil, errors.New("未获取到 Claude Code 订阅额度信息（响应头缺失，账号可能未使用 Claude MAX 订阅）")
	}

	limit, err := strconv.ParseInt(limitStr, 10, 64)
	if err != nil || limit <= 0 {
		return nil, fmt.Errorf("解析订阅额度上限失败: %v", err)
	}

	remaining, err := strconv.ParseInt(remainingStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("解析剩余额度失败: %v", err)
	}
	if remaining < 0 {
		remaining = 0
	}

	var resetAt int64
	if resetStr != "" {
		resetAt, _ = strconv.ParseInt(resetStr, 10, 64)
	}

	used := limit - remaining
	if used < 0 {
		used = 0
	}
	usedPercent := ccClampPercent(float64(used) / float64(limit) * 100)

	now := time.Now().Unix()
	windowSeconds := 0
	label := "current"
	if resetAt > now {
		windowSeconds = int(resetAt - now)
		label = ccWindowLabel(windowSeconds)
	}

	return []SubscriptionWindow{{
		Label:            label,
		UsedPercent:      usedPercent,
		RemainingPercent: 100 - usedPercent,
		ResetAt:          resetAt,
		WindowSeconds:    windowSeconds,
	}}, nil
}

func ccWindowLabel(seconds int) string {
	switch {
	case seconds >= 604800:
		if weeks := seconds / 604800; weeks == 1 {
			return "1week"
		} else {
			return fmt.Sprintf("%dweeks", weeks)
		}
	case seconds >= 86400:
		return fmt.Sprintf("%dd", seconds/86400)
	case seconds >= 3600:
		return fmt.Sprintf("%dh", seconds/3600)
	case seconds >= 60:
		return fmt.Sprintf("%dm", seconds/60)
	default:
		return fmt.Sprintf("%ds", seconds)
	}
}

func ccClampPercent(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}
