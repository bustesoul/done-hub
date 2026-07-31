package billing

import (
	"context"
	"errors"
	"testing"

	"done-hub/internal/gateway/domain"
)

type ledgerFake struct {
	precharges int
	usageCalls int
	settles    int
	refunds    int
}

func (l *ledgerFake) Precharge(context.Context, string, Estimate) (string, error) {
	l.precharges++
	return "reservation-1", nil
}

func (l *ledgerFake) AddUsage(context.Context, string, domain.Usage) error {
	l.usageCalls++
	return nil
}

func (l *ledgerFake) Settle(context.Context, string, FinalCharge) error {
	l.settles++
	return nil
}

func (l *ledgerFake) Refund(context.Context, string, string) error {
	l.refunds++
	return nil
}

func TestSessionSettlesExactlyOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ledger := &ledgerFake{}
	session := NewSession("request-1", ledger)
	if err := session.Precharge(ctx, Estimate{Model: "gpt-test", PromptTokens: 10}); err != nil {
		t.Fatalf("precharge: %v", err)
	}
	if err := session.AddUsage(ctx, domain.Usage{InputTokens: 10, OutputTokens: 3}); err != nil {
		t.Fatalf("add usage: %v", err)
	}
	if err := session.Settle(ctx, domain.Usage{InputTokens: 10, OutputTokens: 5}, domain.OutcomeSucceeded); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := session.Settle(ctx, domain.Usage{InputTokens: 10, OutputTokens: 5}, domain.OutcomeSucceeded); err != nil {
		t.Fatalf("idempotent settle: %v", err)
	}
	if ledger.precharges != 1 || ledger.usageCalls != 1 || ledger.settles != 1 || ledger.refunds != 0 {
		t.Fatalf("unexpected ledger calls: %#v", ledger)
	}
}

func TestSessionAggregatesAttemptUsageAndRejectsConflictingSettlement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ledger := &ledgerFake{}
	session := NewSession("request-aggregate", ledger)
	if err := session.Precharge(ctx, Estimate{}); err != nil {
		t.Fatalf("precharge: %v", err)
	}
	if err := session.AddUsage(ctx, domain.Usage{InputTokens: 3, OutputTokens: 1}); err != nil {
		t.Fatalf("add failed attempt usage: %v", err)
	}
	if err := session.Settle(ctx, domain.Usage{InputTokens: 5, OutputTokens: 2}, domain.OutcomeSucceeded); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if got := session.Usage(); got != (domain.Usage{InputTokens: 8, OutputTokens: 3}) {
		t.Fatalf("unexpected aggregate usage: %#v", got)
	}
	if err := session.Settle(ctx, domain.Usage{InputTokens: 1}, domain.OutcomeSucceeded); !errors.Is(err, ErrFinalChargeConflict) {
		t.Fatalf("expected final charge conflict, got %v", err)
	}
}

func TestSessionRefundsExactlyOnce(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ledger := &ledgerFake{}
	session := NewSession("request-2", ledger)
	if err := session.Precharge(ctx, Estimate{}); err != nil {
		t.Fatalf("precharge: %v", err)
	}
	if err := session.Refund(ctx, "no output"); err != nil {
		t.Fatalf("refund: %v", err)
	}
	if err := session.Refund(ctx, "no output"); err != nil {
		t.Fatalf("idempotent refund: %v", err)
	}
	if ledger.refunds != 1 {
		t.Fatalf("expected one refund, got %d", ledger.refunds)
	}
}
