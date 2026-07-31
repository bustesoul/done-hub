package stream

import (
	"errors"
	"testing"

	"done-hub/internal/gateway/domain"
)

func TestSessionStateAndFailoverGate(t *testing.T) {
	t.Parallel()

	session := NewSession()
	if !session.CanFailover() {
		t.Fatal("new session should allow failover")
	}
	if err := session.BeginHeaders(); err != nil {
		t.Fatalf("begin headers: %v", err)
	}
	if session.CanFailover() {
		t.Fatal("session must not allow failover after headers")
	}
	if err := session.RecordData(12); err != nil {
		t.Fatalf("record data: %v", err)
	}
	if err := session.AddUsage(domain.Usage{InputTokens: 3, OutputTokens: 5}); err != nil {
		t.Fatalf("add usage: %v", err)
	}
	terminalErr := errors.New("upstream stream failed")
	if err := session.Terminate(terminalErr); err != nil {
		t.Fatalf("terminate: %v", err)
	}

	snapshot := session.Snapshot()
	if snapshot.State != StateTerminal || snapshot.Bytes != 12 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.Usage.InputTokens != 3 || snapshot.Usage.OutputTokens != 5 {
		t.Fatalf("unexpected usage: %#v", snapshot.Usage)
	}
	if !errors.Is(snapshot.TerminalErr, terminalErr) {
		t.Fatalf("unexpected terminal error: %v", snapshot.TerminalErr)
	}

	if err := session.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := session.RecordData(1); !errors.Is(err, ErrStreamClosed) {
		t.Fatalf("expected closed error, got %v", err)
	}
}

func TestSessionRejectsDataAfterTerminal(t *testing.T) {
	t.Parallel()

	session := NewSession()
	if err := session.Terminate(nil); err != nil {
		t.Fatalf("terminate: %v", err)
	}
	if err := session.RecordData(1); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

func TestSessionAcceptanceBlocksFailoverWithoutOutput(t *testing.T) {
	t.Parallel()

	session := NewSession()
	if err := session.MarkAccepted(); err != nil {
		t.Fatalf("mark accepted: %v", err)
	}
	if session.CanFailover() {
		t.Fatal("accepted upstream side effect must block failover")
	}
	if !session.Snapshot().Accepted {
		t.Fatal("accepted state was not recorded")
	}
}
