package execution

import (
	"context"
	"net/http"
	"testing"
	"time"

	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/retry"
	"done-hub/internal/gateway/stream"
)

type runnerFake struct {
	calls         int
	streamOnFirst bool
	acceptOnFirst bool
}

func (r *runnerFake) Run(
	_ context.Context,
	attempt domain.Attempt,
	session *stream.Session,
) (domain.AttemptResult, *retry.UpstreamError) {
	r.calls++
	if r.calls == 1 {
		if r.streamOnFirst {
			_ = session.BeginHeaders()
		}
		if r.acceptOnFirst {
			_ = session.MarkAccepted()
		}
		return domain.AttemptResult{}, &retry.UpstreamError{
			Class:      retry.ErrorClassTransient,
			StatusCode: http.StatusBadGateway,
		}
	}
	return domain.AttemptResult{
		Attempt: attempt,
		Usage:   domain.Usage{OutputTokens: 2},
	}, nil
}

func testPlan() domain.RoutePlan {
	return domain.RoutePlan{
		Request: domain.RequestContext{
			RequestID:      "request-1",
			RequestedModel: "model-1",
		},
		Model: domain.ModelRoute{
			PublicModel:   "model-1",
			UpstreamModel: "upstream-model-1",
		},
		Endpoints: []domain.Endpoint{
			{ID: 1, ProviderID: 1, Enabled: true},
			{ID: 2, ProviderID: 1, Enabled: true},
		},
	}
}

func TestGatewayEngineRetriesBeforeOutput(t *testing.T) {
	t.Parallel()

	runner := &runnerFake{}
	coordinator := NewGatewayEngine(retry.DefaultPolicy())
	coordinator.Sleep = func(context.Context, time.Duration) error { return nil }
	result := coordinator.Run(context.Background(), testPlan(), runner)
	if result.Err != nil {
		t.Fatalf("expected success, got %v", result.Err)
	}
	if runner.calls != 2 || len(result.Attempts) != 2 {
		t.Fatalf("expected two attempts, calls=%d attempts=%d", runner.calls, len(result.Attempts))
	}
	if result.Final.Attempt.Endpoint.ID != 2 {
		t.Fatalf("expected second endpoint, got %d", result.Final.Attempt.Endpoint.ID)
	}
}

func TestGatewayEngineStopsAfterUpstreamAcceptance(t *testing.T) {
	t.Parallel()

	runner := &runnerFake{acceptOnFirst: true}
	coordinator := NewGatewayEngine(retry.DefaultPolicy())
	coordinator.Sleep = func(context.Context, time.Duration) error { return nil }
	result := coordinator.Run(context.Background(), testPlan(), runner)
	if result.Err == nil {
		t.Fatal("expected upstream error")
	}
	if runner.calls != 1 || result.Final.UpstreamAccepted != true {
		t.Fatalf("accepted attempt must not retry: calls=%d final=%#v", runner.calls, result.Final)
	}
}

func TestGatewayEngineConsumesCooldownAndDelay(t *testing.T) {
	t.Parallel()

	runner := &runnerFake{}
	cooldowns := &cooldownFake{}
	var slept time.Duration
	coordinator := NewGatewayEngine(retry.Policy{BaseCooldown: 3 * time.Second})
	coordinator.Cooldowns = cooldowns
	coordinator.Sleep = func(_ context.Context, delay time.Duration) error {
		slept = delay
		return nil
	}
	result := coordinator.Run(context.Background(), testPlan(), runner)
	if result.Err != nil {
		t.Fatalf("expected success, got %v", result.Err)
	}
	if cooldowns.calls != 1 || slept != 3*time.Second {
		t.Fatalf("decision not consumed: cooldowns=%d slept=%s", cooldowns.calls, slept)
	}
}

type cooldownFake struct {
	calls int
}

func (c *cooldownFake) Cooldown(context.Context, domain.Endpoint, string, *retry.UpstreamError, retry.Decision) error {
	c.calls++
	return nil
}

func TestGatewayEngineStopsAfterOutput(t *testing.T) {
	t.Parallel()

	runner := &runnerFake{streamOnFirst: true}
	coordinator := NewGatewayEngine(retry.DefaultPolicy())
	result := coordinator.Run(context.Background(), testPlan(), runner)
	if result.Err == nil {
		t.Fatal("expected upstream error")
	}
	if runner.calls != 1 || len(result.Attempts) != 1 {
		t.Fatalf("expected one attempt, calls=%d attempts=%d", runner.calls, len(result.Attempts))
	}
}

func TestGatewayEngineRejectsEmptyRoutePlan(t *testing.T) {
	t.Parallel()

	result := NewGatewayEngine(retry.DefaultPolicy()).Run(context.Background(), domain.RoutePlan{}, &runnerFake{})
	if result.Err == nil || result.Err.Class != retry.ErrorClassConfigInvalid {
		t.Fatalf("expected config error, got %#v", result.Err)
	}
}
