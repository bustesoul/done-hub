package billing

import (
	"context"
	"errors"
	"sync"

	"done-hub/internal/gateway/domain"
)

type State uint8

const (
	StateNew State = iota
	StateReserved
	StateSettled
	StateRefunded
)

var (
	ErrAlreadyFinalized    = errors.New("billing session is already finalized")
	ErrNotReserved         = errors.New("billing session has not been reserved")
	ErrFinalChargeConflict = errors.New("billing session received a conflicting final charge")
)

type Estimate struct {
	Model        string
	PromptTokens int
	ServiceTier  string
	PriceVersion string
}

type FinalCharge struct {
	Usage   domain.Usage
	Outcome domain.Outcome
}

type Ledger interface {
	Precharge(ctx context.Context, requestID string, estimate Estimate) (string, error)
	AddUsage(ctx context.Context, reservationID string, delta domain.Usage) error
	Settle(ctx context.Context, reservationID string, charge FinalCharge) error
	Refund(ctx context.Context, reservationID string, reason string) error
}

type Session struct {
	mu            sync.Mutex
	requestID     string
	ledger        Ledger
	state         State
	reservationID string
	usage         domain.Usage
	finalCharge   *FinalCharge
	settleUsage   domain.Usage
}

func NewSession(requestID string, ledger Ledger) *Session {
	return &Session{
		requestID: requestID,
		ledger:    ledger,
		state:     StateNew,
	}
}

func (s *Session) Precharge(ctx context.Context, estimate Estimate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != StateNew {
		return ErrAlreadyFinalized
	}

	reservationID, err := s.ledger.Precharge(ctx, s.requestID, estimate)
	if err != nil {
		return err
	}
	s.reservationID = reservationID
	s.state = StateReserved
	return nil
}

func (s *Session) AddUsage(ctx context.Context, delta domain.Usage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateSettled || s.state == StateRefunded {
		return ErrAlreadyFinalized
	}
	if s.state != StateReserved {
		return ErrNotReserved
	}
	if err := s.ledger.AddUsage(ctx, s.reservationID, delta); err != nil {
		return err
	}
	s.usage = s.usage.Add(delta)
	return nil
}

func (s *Session) Settle(ctx context.Context, finalUsage domain.Usage, outcome domain.Outcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateSettled {
		if s.finalCharge == nil || s.finalCharge.Outcome != outcome ||
			s.settleUsage != finalUsage {
			return ErrFinalChargeConflict
		}
		return nil
	}
	if s.state == StateRefunded {
		return ErrAlreadyFinalized
	}
	if s.state != StateReserved {
		return ErrNotReserved
	}
	totalUsage := s.usage.Add(finalUsage)
	charge := FinalCharge{Usage: totalUsage, Outcome: outcome}
	if err := s.ledger.Settle(ctx, s.reservationID, charge); err != nil {
		return err
	}
	s.usage = totalUsage
	s.finalCharge = &charge
	s.settleUsage = finalUsage
	s.state = StateSettled
	return nil
}

func (s *Session) Refund(ctx context.Context, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateRefunded {
		return nil
	}
	if s.state == StateSettled {
		return ErrAlreadyFinalized
	}
	if s.state != StateReserved {
		return ErrNotReserved
	}
	if err := s.ledger.Refund(ctx, s.reservationID, reason); err != nil {
		return err
	}
	s.state = StateRefunded
	return nil
}

func (s *Session) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Session) Usage() domain.Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.usage
}
