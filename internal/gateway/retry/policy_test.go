package retry

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestPolicyDecisionTable(t *testing.T) {
	t.Parallel()

	policy := Policy{
		BaseCooldown: 2 * time.Second,
		MaxCooldown:  10 * time.Second,
	}
	tests := []struct {
		name          string
		upstreamError *UpstreamError
		context       DecisionContext
		retry         bool
		cooldown      bool
		reason        string
		delay         time.Duration
	}{
		{
			name:    "success",
			context: DecisionContext{Attempt: 1, MaxAttempts: 2},
			reason:  "success",
		},
		{
			name:          "local validation",
			upstreamError: &UpstreamError{Class: ErrorClassLocalValidation, StatusCode: http.StatusBadRequest},
			context:       DecisionContext{Attempt: 1, MaxAttempts: 2},
			reason:        string(ErrorClassLocalValidation),
		},
		{
			name:          "rate limited",
			upstreamError: &UpstreamError{Class: ErrorClassRateLimited, StatusCode: http.StatusTooManyRequests, RetryAfter: 30 * time.Second},
			context:       DecisionContext{Attempt: 1, MaxAttempts: 2},
			retry:         true,
			cooldown:      true,
			reason:        string(ErrorClassRateLimited),
			delay:         10 * time.Second,
		},
		{
			name:          "transient",
			upstreamError: &UpstreamError{Class: ErrorClassTransient, StatusCode: http.StatusBadGateway},
			context:       DecisionContext{Attempt: 1, MaxAttempts: 2},
			retry:         true,
			cooldown:      true,
			reason:        string(ErrorClassTransient),
			delay:         2 * time.Second,
		},
		{
			name:          "stream started",
			upstreamError: &UpstreamError{Class: ErrorClassTransient, StatusCode: http.StatusBadGateway},
			context:       DecisionContext{Attempt: 1, MaxAttempts: 2, OutputStarted: true},
			reason:        "output_started",
		},
		{
			name:          "upstream accepted",
			upstreamError: &UpstreamError{Class: ErrorClassTransient, StatusCode: http.StatusBadGateway},
			context:       DecisionContext{Attempt: 1, MaxAttempts: 2, UpstreamAccepted: true},
			reason:        "upstream_accepted",
		},
		{
			name:          "attempts exhausted",
			upstreamError: &UpstreamError{Class: ErrorClassTransient, StatusCode: http.StatusBadGateway},
			context:       DecisionContext{Attempt: 2, MaxAttempts: 2},
			reason:        "attempts_exhausted",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			decision := policy.Decide(test.upstreamError, test.context)
			if decision.Retry != test.retry || decision.Cooldown != test.cooldown {
				t.Fatalf("unexpected retry decision: %#v", decision)
			}
			if decision.Reason != test.reason {
				t.Fatalf("expected reason %q, got %q", test.reason, decision.Reason)
			}
			if decision.Delay != test.delay {
				t.Fatalf("expected delay %s, got %s", test.delay, decision.Delay)
			}
		})
	}
}

func TestClassifyHTTP(t *testing.T) {
	t.Parallel()

	if got := ClassifyHTTP(http.StatusTooManyRequests, nil); got.Class != ErrorClassRateLimited {
		t.Fatalf("expected rate limited, got %s", got.Class)
	}
	if got := ClassifyHTTP(http.StatusBadGateway, nil); got.Class != ErrorClassTransient {
		t.Fatalf("expected transient, got %s", got.Class)
	}
	if got := ClassifyHTTP(0, context.Canceled); got.Class != ErrorClassClientCanceled {
		t.Fatalf("expected canceled, got %s", got.Class)
	}
}
