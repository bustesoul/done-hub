package relay_util

import "testing"

func TestQuotaTransferReservationMovesOwnership(t *testing.T) {
	source := &Quota{
		preConsumedQuota: 120,
		HandelStatus:     true,
	}
	target := &Quota{}

	source.TransferReservationTo(target)

	if source.preConsumedQuota != 0 || source.HandelStatus {
		t.Fatalf("source still owns reservation: %#v", source)
	}
	if target.preConsumedQuota != 120 || !target.HandelStatus {
		t.Fatalf("target did not receive reservation: %#v", target)
	}
}
