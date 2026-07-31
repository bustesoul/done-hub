package execution

import (
	"context"
	"errors"
	"time"

	"done-hub/internal/gateway/domain"
	"done-hub/internal/gateway/retry"
	"done-hub/internal/gateway/stream"
)

var ErrNoCandidates = errors.New("route plan has no candidate endpoints")
var ErrAttemptDeadlineExceeded = errors.New("gateway attempt deadline exceeded")

type Runner interface {
	Run(
		ctx context.Context,
		attempt domain.Attempt,
		session *stream.Session,
	) (domain.AttemptResult, *retry.UpstreamError)
}

type Observer interface {
	AttemptFinished(result domain.AttemptResult, upstreamErr *retry.UpstreamError, decision retry.Decision)
}

type SelectionState struct {
	Attempt  int
	Previous *domain.AttemptResult
	Skip     map[int]struct{}
}

type Selector interface {
	Next(ctx context.Context, plan domain.RoutePlan, state SelectionState) (domain.Endpoint, error)
}

type CooldownStore interface {
	Cooldown(ctx context.Context, endpoint domain.Endpoint, model string, upstreamErr *retry.UpstreamError, decision retry.Decision) error
}

type Result struct {
	Final    domain.AttemptResult
	Attempts []domain.AttemptResult
	Err      *retry.UpstreamError
}

// GatewayEngine owns the complete attempt lifecycle for every gateway
// transport: selection, execution, output gating, retry, cooldown and
// observation. Protocol-specific runners contain no retry loop.
type GatewayEngine struct {
	Policy      retry.Policy
	Observer    Observer
	Selector    Selector
	Cooldowns   CooldownStore
	MaxAttempts int
	Deadline    time.Duration
	Now         func() time.Time
	Sleep       func(context.Context, time.Duration) error
}

func NewGatewayEngine(policy retry.Policy) *GatewayEngine {
	return &GatewayEngine{
		Policy: policy,
		Now:    time.Now,
		Sleep:  sleepContext,
	}
}

func (c *GatewayEngine) Run(ctx context.Context, plan domain.RoutePlan, runner Runner) Result {
	if len(plan.Endpoints) == 0 && c.Selector == nil {
		return Result{
			Err: &retry.UpstreamError{
				Class:       retry.ErrorClassConfigInvalid,
				Local:       true,
				Cause:       ErrNoCandidates,
				Description: ErrNoCandidates.Error(),
			},
		}
	}

	now := c.Now
	if now == nil {
		now = time.Now
	}
	maxAttempts := len(plan.Endpoints)
	if c.Selector != nil {
		maxAttempts = c.MaxAttempts
	}
	if c.MaxAttempts > 0 && (maxAttempts == 0 || c.MaxAttempts < maxAttempts) {
		maxAttempts = c.MaxAttempts
	}
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	result := Result{
		Attempts: make([]domain.AttemptResult, 0, maxAttempts),
	}

	runCtx := ctx
	cancel := func() {}
	if c.Deadline > 0 {
		runCtx, cancel = context.WithTimeout(ctx, c.Deadline)
	}
	defer cancel()

	selectionState := SelectionState{Skip: make(map[int]struct{}, maxAttempts)}
	for index := 0; index < maxAttempts; index++ {
		if err := runCtx.Err(); err != nil {
			result.Err = retry.ClassifyHTTP(0, err)
			if errors.Is(err, context.DeadlineExceeded) {
				result.Err.Cause = ErrAttemptDeadlineExceeded
				result.Err.Description = ErrAttemptDeadlineExceeded.Error()
			}
			return result
		}
		var endpoint domain.Endpoint
		if c.Selector != nil {
			selectionState.Attempt = index + 1
			selected, err := c.Selector.Next(runCtx, plan, selectionState)
			if err != nil {
				if len(result.Attempts) == 0 {
					result.Err = &retry.UpstreamError{
						Class:       retry.ErrorClassConfigInvalid,
						Local:       true,
						Cause:       err,
						Description: err.Error(),
					}
				}
				return result
			}
			endpoint = selected
		} else {
			endpoint = plan.Endpoints[index]
		}
		attempt := domain.Attempt{
			RequestID: plan.Request.RequestID,
			Number:    index + 1,
			Endpoint:  endpoint,
			Model:     plan.Model,
			StartedAt: now(),
		}
		session := stream.NewSession()
		attemptResult, upstreamErr := runner.Run(runCtx, attempt, session)
		attemptResult.Attempt = attempt
		if attemptResult.CompletedAt.IsZero() {
			attemptResult.CompletedAt = now()
		}
		snapshot := session.Snapshot()
		attemptResult.OutputStarted = attemptResult.OutputStarted || snapshot.State != stream.StateNotStarted
		attemptResult.UpstreamAccepted = attemptResult.UpstreamAccepted || snapshot.Accepted
		attemptResult.Usage = attemptResult.Usage.Add(snapshot.Usage)
		result.Attempts = append(result.Attempts, attemptResult)
		result.Final = attemptResult
		result.Err = upstreamErr
		selectionState.Previous = &result.Final
		selectionState.Skip[endpoint.ID] = struct{}{}

		decision := c.Policy.Decide(upstreamErr, retry.DecisionContext{
			Attempt:          index + 1,
			MaxAttempts:      maxAttempts,
			OutputStarted:    attemptResult.OutputStarted,
			UpstreamAccepted: attemptResult.UpstreamAccepted,
		})
		if c.Observer != nil {
			c.Observer.AttemptFinished(attemptResult, upstreamErr, decision)
		}
		if upstreamErr == nil || !decision.Retry {
			return result
		}
		if decision.Cooldown && c.Cooldowns != nil {
			_ = c.Cooldowns.Cooldown(runCtx, endpoint, plan.Model.PublicModel, upstreamErr, decision)
		}
		if decision.Delay > 0 {
			sleep := c.Sleep
			if sleep == nil {
				sleep = sleepContext
			}
			if err := sleep(runCtx, decision.Delay); err != nil {
				result.Err = retry.ClassifyHTTP(0, err)
				return result
			}
		}
	}

	return result
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
