package claudecode

import (
	"errors"
	"fmt"
	"math"
	"time"
)

type SubscriptionWindow struct {
	Label            string  `json:"label"`
	UsedPercent      float64 `json:"used_percent"`
	RemainingPercent float64 `json:"remaining_percent"`
	ResetAt          int64   `json:"reset_at"`
	WindowSeconds    int     `json:"window_seconds"`
}

// claudeUsageWindow 对应 /api/oauth/usage 返回的单个用量窗口。
// utilization 是 0-100 的使用率百分比；resets_at 是 RFC3339 时间戳。
type claudeUsageWindow struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// claudeUsageResponse 对应 Anthropic /api/oauth/usage 返回结构。
type claudeUsageResponse struct {
	FiveHour                claudeUsageWindow `json:"five_hour"`
	SevenDay                claudeUsageWindow `json:"seven_day"`
	SevenDaySonnet          claudeUsageWindow `json:"seven_day_sonnet"`
	SevenDayOverageIncluded claudeUsageWindow `json:"seven_day_overage_included"`
}

// SubscriptionQuota 调用 Claude Code CLI 的专属额度端点 /api/oauth/usage，
// 解析 JSON body 获取订阅额度。对齐真实 Claude Code CLI 行为。
func (p *ClaudeCodeProvider) SubscriptionQuota() ([]SubscriptionWindow, error) {
	token, err := p.GetToken()
	if err != nil {
		return nil, fmt.Errorf("获取 Token 失败: %w", err)
	}

	headers := map[string]string{
		"Authorization":  "Bearer " + token,
		"anthropic-beta": "oauth-2025-04-20",
		"User-Agent":     "claude-code/2.1.7",
		"Accept":         "application/json, text/plain, */*",
	}

	url := p.GetFullRequestURL("/api/oauth/usage")
	req, err := p.Requester.NewRequest("GET", url, p.Requester.WithHeader(headers))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	var usage claudeUsageResponse
	_, errWithCode := p.Requester.SendRequest(req, &usage, false)
	if errWithCode != nil {
		return nil, errors.New(errWithCode.OpenAIError.Message)
	}

	windows := buildClaudeWindows(&usage)
	if len(windows) == 0 {
		return nil, errors.New("未获取到 Claude Code 订阅额度信息")
	}
	return windows, nil
}

// buildClaudeWindows 将 usage 响应转换为 SubscriptionWindow 切片。
// 顺序：5h、7d、Sonnet 7d、Fable 7d；跳过 resets_at 为空的无效窗口。
func buildClaudeWindows(usage *claudeUsageResponse) []SubscriptionWindow {
	windows := make([]SubscriptionWindow, 0, 4)
	windows = appendClaudeWindow(windows, "", usage.FiveHour)
	windows = appendClaudeWindow(windows, "", usage.SevenDay)
	windows = appendClaudeWindow(windows, "Sonnet", usage.SevenDaySonnet)
	windows = appendClaudeWindow(windows, "Fable", usage.SevenDayOverageIncluded)
	return windows
}

func appendClaudeWindow(windows []SubscriptionWindow, prefix string, w claudeUsageWindow) []SubscriptionWindow {
	if w.ResetsAt == "" {
		return windows
	}

	resetAt, err := parseRFC3339(w.ResetsAt)
	if err != nil {
		return windows
	}
	resetAtUnix := resetAt.Unix()

	now := time.Now().Unix()
	windowSeconds := int(resetAtUnix - now)
	if windowSeconds < 0 {
		windowSeconds = 0
	}

	used := ccClampPercent(w.Utilization)
	return append(windows, SubscriptionWindow{
		Label:            ccWindowLabel(prefix, windowSeconds),
		UsedPercent:      used,
		RemainingPercent: 100 - used,
		ResetAt:          resetAtUnix,
		WindowSeconds:    windowSeconds,
	})
}

// ccWindowLabel 根据窗口剩余秒数生成 label，可选前缀区分不同窗口。
func ccWindowLabel(prefix string, seconds int) string {
	var label string
	switch {
	case seconds >= 604800:
		if weeks := seconds / 604800; weeks == 1 {
			label = "1week"
		} else {
			label = fmt.Sprintf("%dweeks", weeks)
		}
	case seconds >= 86400:
		label = fmt.Sprintf("%dd", seconds/86400)
	case seconds >= 3600:
		label = fmt.Sprintf("%dh", seconds/3600)
	case seconds >= 60:
		label = fmt.Sprintf("%dm", seconds/60)
	default:
		label = fmt.Sprintf("%ds", seconds)
	}
	if prefix == "" {
		return label
	}
	return prefix + " " + label
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

func parseRFC3339(s string) (time.Time, error) {
	for _, format := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间: %s", s)
}
