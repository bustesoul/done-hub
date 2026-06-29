package relay_util

import (
	"done-hub/model"
	"testing"
)

func TestServiceTierPriorityRatio(t *testing.T) {
	if got := NormalizeServiceTier(" Priority "); got != ServiceTierPriority {
		t.Fatalf("NormalizeServiceTier() = %q, want %q", got, ServiceTierPriority)
	}
	if got := ServiceTierRatio(ServiceTierPriority); got != ServiceTierPriorityRatio {
		t.Fatalf("ServiceTierRatio(priority) = %v, want %v", got, ServiceTierPriorityRatio)
	}
	if got := ServiceTierRatio("auto"); got != 1 {
		t.Fatalf("ServiceTierRatio(auto) = %v, want 1", got)
	}
}

func TestPriorityServiceTierIncreasesQuota(t *testing.T) {
	normal := &Quota{
		price:            model.Price{Type: model.TokensPriceType},
		groupRatio:       1,
		inputRatio:       2,
		outputRatio:      4,
		serviceTierRatio: 1,
	}
	fast := &Quota{
		price:            model.Price{Type: model.TokensPriceType},
		groupRatio:       1,
		inputRatio:       2 * ServiceTierPriorityRatio,
		outputRatio:      4 * ServiceTierPriorityRatio,
		serviceTierRatio: ServiceTierPriorityRatio,
	}

	normalQuota := normal.GetTotalQuota(100, 50, nil)
	fastQuota := fast.GetTotalQuota(100, 50, nil)

	if normalQuota != 400 {
		t.Fatalf("normal quota = %d, want 400", normalQuota)
	}
	if fastQuota != 600 {
		t.Fatalf("fast quota = %d, want 600", fastQuota)
	}
}
