package codex

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

type SubscriptionQuotaResponse struct {
	RateLimit            *SubscriptionRateLimit        `json:"rate_limit"`
	AdditionalRateLimits []AdditionalSubscriptionLimit `json:"additional_rate_limits"`
}

type SubscriptionRateLimit struct {
	PrimaryWindow   *SubscriptionWindow `json:"primary_window"`
	SecondaryWindow *SubscriptionWindow `json:"secondary_window"`
}

type AdditionalSubscriptionLimit struct {
	LimitName string                 `json:"limit_name"`
	RateLimit *SubscriptionRateLimit `json:"rate_limit"`
}

type SubscriptionWindow struct {
	Label            string  `json:"label"`
	UsedPercent      float64 `json:"used_percent"`
	RemainingPercent float64 `json:"remaining_percent"`
	ResetAt          int64   `json:"reset_at"`
	WindowSeconds    int     `json:"window_seconds"`
}

func (p *CodexProvider) SubscriptionQuota() ([]SubscriptionWindow, error) {
	token, err := p.GetToken()
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Accept":     "application/json",
		"User-Agent": DefaultCodexUserAgent,
	}
	p.ApplyCustomHeaders(headers)
	p.SetHeader(headers, "Authorization", "Bearer "+token)
	if p.Credentials != nil && p.Credentials.AccountID != "" {
		p.SetHeader(headers, "ChatGPT-Account-Id", p.Credentials.AccountID)
	}

	req, err := p.Requester.NewRequest("GET", p.codexUsageURL(), p.Requester.WithHeader(headers))
	if err != nil {
		return nil, err
	}

	var usage SubscriptionQuotaResponse
	_, errWithCode := p.Requester.SendRequest(req, &usage, false)
	if errWithCode != nil {
		return nil, errors.New(errWithCode.OpenAIError.Message)
	}

	windows := usage.Windows()
	if len(windows) == 0 {
		return nil, errors.New("未获取到 Codex 订阅额度窗口")
	}
	return windows, nil
}

func (p *CodexProvider) codexUsageURL() string {
	baseURL := strings.TrimRight(p.GetBaseURL(), "/")
	switch {
	case strings.Contains(baseURL, "/backend-api"):
		return baseURL + "/wham/usage"
	case strings.Contains(baseURL, "api.openai.com"):
		return baseURL + "/api/codex/usage"
	default:
		return baseURL + "/backend-api/wham/usage"
	}
}

func (r SubscriptionQuotaResponse) Windows() []SubscriptionWindow {
	windows := make([]SubscriptionWindow, 0, 2+len(r.AdditionalRateLimits)*2)
	windows = appendWindows(windows, "", r.RateLimit)
	for _, additional := range r.AdditionalRateLimits {
		windows = appendWindows(windows, strings.TrimSpace(additional.LimitName), additional.RateLimit)
	}
	return windows
}

func appendWindows(windows []SubscriptionWindow, prefix string, limit *SubscriptionRateLimit) []SubscriptionWindow {
	if limit == nil {
		return windows
	}
	if limit.PrimaryWindow != nil {
		windows = append(windows, limit.PrimaryWindow.normalized(windowLabel(prefix, limit.PrimaryWindow.WindowSeconds, "5h")))
	}
	if limit.SecondaryWindow != nil {
		windows = append(windows, limit.SecondaryWindow.normalized(windowLabel(prefix, limit.SecondaryWindow.WindowSeconds, "1week")))
	}
	return windows
}

func (w *SubscriptionWindow) normalized(label string) SubscriptionWindow {
	used := clampPercent(w.UsedPercent)
	return SubscriptionWindow{
		Label:            label,
		UsedPercent:      used,
		RemainingPercent: 100 - used,
		ResetAt:          w.ResetAt,
		WindowSeconds:    w.WindowSeconds,
	}
}

func windowLabel(prefix string, seconds int, fallback string) string {
	label := fallback
	if seconds > 0 {
		switch {
		case seconds%604800 == 0:
			weeks := seconds / 604800
			if weeks == 1 {
				label = "1week"
			} else {
				label = fmt.Sprintf("%dweeks", weeks)
			}
		case seconds%3600 == 0:
			label = fmt.Sprintf("%dh", seconds/3600)
		case seconds%60 == 0:
			label = fmt.Sprintf("%dm", seconds/60)
		default:
			label = fmt.Sprintf("%ds", seconds)
		}
	}
	if prefix == "" {
		return label
	}
	return prefix + " " + label
}

func clampPercent(value float64) float64 {
	if math.IsNaN(value) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
