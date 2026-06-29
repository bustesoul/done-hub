package codex

import "testing"

func TestSubscriptionQuotaResponseWindows(t *testing.T) {
	response := SubscriptionQuotaResponse{
		RateLimit: &SubscriptionRateLimit{
			PrimaryWindow: &SubscriptionWindow{
				UsedPercent:   12,
				ResetAt:       1766948068,
				WindowSeconds: 18000,
			},
			SecondaryWindow: &SubscriptionWindow{
				UsedPercent:   35,
				ResetAt:       1767380068,
				WindowSeconds: 604800,
			},
		},
	}

	windows := response.Windows()
	if len(windows) != 2 {
		t.Fatalf("len(windows) = %d, want 2", len(windows))
	}
	if windows[0].Label != "5h" || windows[0].RemainingPercent != 88 {
		t.Fatalf("primary window = %#v", windows[0])
	}
	if windows[1].Label != "1week" || windows[1].RemainingPercent != 65 {
		t.Fatalf("secondary window = %#v", windows[1])
	}
}

func TestSubscriptionQuotaResponseWindowsIncludesAdditionalLimits(t *testing.T) {
	response := SubscriptionQuotaResponse{
		AdditionalRateLimits: []AdditionalSubscriptionLimit{
			{
				LimitName: "Codex Spark",
				RateLimit: &SubscriptionRateLimit{
					PrimaryWindow: &SubscriptionWindow{
						UsedPercent:   125,
						WindowSeconds: 3600,
					},
				},
			},
		},
	}

	windows := response.Windows()
	if len(windows) != 1 {
		t.Fatalf("len(windows) = %d, want 1", len(windows))
	}
	if windows[0].Label != "Codex Spark 1h" || windows[0].UsedPercent != 100 || windows[0].RemainingPercent != 0 {
		t.Fatalf("additional window = %#v", windows[0])
	}
}
