package services

import (
	"testing"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// mockFixerStateProvider is a test double for FixerStateProvider.
type mockFixerStateProvider struct {
	state int
}

func (m *mockFixerStateProvider) LookoutStateFor(sessionID string) int {
	return m.state
}

func (m *mockFixerStateProvider) LookoutRetryFor(sessionID string) (int, int) {
	return 0, 0
}

// newTestSessionServiceForEnrich builds the minimal SessionService needed to test
// enrichLookoutState — only the fixer field is relevant for these tests.
func newTestSessionServiceForEnrich(fixer FixerStateProvider) *SessionService {
	svc := &SessionService{
		fixer: fixer,
	}
	return svc
}

// TestEnrichLookoutState_NilSession verifies that a nil protoSess is returned unchanged.
func TestEnrichLookoutState_NilSession(t *testing.T) {
	svc := newTestSessionServiceForEnrich(&mockFixerStateProvider{state: 1})
	result := svc.enrichLookoutState(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

// TestEnrichLookoutState_NilFixer verifies that protoSess is returned unchanged when no fixer is wired.
func TestEnrichLookoutState_NilFixer(t *testing.T) {
	svc := newTestSessionServiceForEnrich(nil)
	sess := &sessionv1.Session{Id: "test-session"}
	result := svc.enrichLookoutState(sess)
	if result != sess {
		t.Errorf("expected same pointer returned when fixer is nil")
	}
	if result.LookoutState != sessionv1.LookoutState_LOOKOUT_STATE_UNSPECIFIED {
		t.Errorf("expected UNSPECIFIED (0), got %v", result.LookoutState)
	}
}

// TestEnrichLookoutState_IdleState verifies that crewState=0 (LookoutIdle) leaves
// LookoutState at its zero value (LOOKOUT_STATE_UNSPECIFIED), because idle is the default
// and we only emit non-zero when a Lookout is actively running.
func TestEnrichLookoutState_IdleState(t *testing.T) {
	// crew.LookoutIdle = 0 → should NOT set proto field (stays UNSPECIFIED = 0)
	svc := newTestSessionServiceForEnrich(&mockFixerStateProvider{state: 0})
	sess := &sessionv1.Session{Id: "session-idle"}
	result := svc.enrichLookoutState(sess)
	if result.LookoutState != sessionv1.LookoutState_LOOKOUT_STATE_UNSPECIFIED {
		t.Errorf("crewState=0 (Idle): expected UNSPECIFIED, got %v", result.LookoutState)
	}
}

// TestEnrichLookoutState_ReservedValue1 verifies that crewState=1 (reserved proto value,
// not produced by Go code after LookoutActive removal) maps to UNSPECIFIED.
func TestEnrichLookoutState_ReservedValue1(t *testing.T) {
	// Value 1 is reserved in the proto LookoutState enum but not produced by Go code.
	svc := newTestSessionServiceForEnrich(&mockFixerStateProvider{state: 1})
	sess := &sessionv1.Session{Id: "session-reserved"}
	result := svc.enrichLookoutState(sess)
	if result.LookoutState != sessionv1.LookoutState_LOOKOUT_STATE_UNSPECIFIED {
		t.Errorf("crewState=1 (reserved): expected LOOKOUT_STATE_UNSPECIFIED, got %v", result.LookoutState)
	}
}

// TestEnrichLookoutState_SweepingState verifies that crewState=2 (LookoutSweeping) maps to
// proto LOOKOUT_STATE_SWEEPING.
func TestEnrichLookoutState_SweepingState(t *testing.T) {
	svc := newTestSessionServiceForEnrich(&mockFixerStateProvider{state: 2})
	sess := &sessionv1.Session{Id: "session-sweeping"}
	result := svc.enrichLookoutState(sess)
	if result.LookoutState != sessionv1.LookoutState_LOOKOUT_STATE_SWEEPING {
		t.Errorf("crewState=2 (Sweeping): expected LOOKOUT_STATE_SWEEPING (%d), got %v",
			sessionv1.LookoutState_LOOKOUT_STATE_SWEEPING, result.LookoutState)
	}
}

// TestEnrichLookoutState_FallenState verifies that crewState=4 (LookoutFallen) maps to
// proto LOOKOUT_STATE_FALLEN (5) via the +1 offset.
func TestEnrichLookoutState_FallenState(t *testing.T) {
	// crew.LookoutFallen = 4 → proto offset +1 → LOOKOUT_STATE_FALLEN = 5
	svc := newTestSessionServiceForEnrich(&mockFixerStateProvider{state: 4})
	sess := &sessionv1.Session{Id: "session-fallen"}
	result := svc.enrichLookoutState(sess)
	if result.LookoutState != sessionv1.LookoutState_LOOKOUT_STATE_FALLEN {
		t.Errorf("crewState=4 (Fallen): expected LOOKOUT_STATE_FALLEN (%d), got %v",
			sessionv1.LookoutState_LOOKOUT_STATE_FALLEN, result.LookoutState)
	}
}

// TestEnrichLookoutState_AwaitingRetryState verifies that crewState=3 (LookoutAwaitingRetry)
// maps to proto LOOKOUT_STATE_AWAITING_RETRY (4).
// This was missing from the original test suite; it covers the retry-cooldown window.
func TestEnrichLookoutState_AwaitingRetryState(t *testing.T) {
	// crew.LookoutAwaitingRetry = 3 → proto LOOKOUT_STATE_AWAITING_RETRY = 4
	svc := newTestSessionServiceForEnrich(&mockFixerStateProvider{state: 3})
	sess := &sessionv1.Session{Id: "session-awaiting-retry"}
	result := svc.enrichLookoutState(sess)
	if result.LookoutState != sessionv1.LookoutState_LOOKOUT_STATE_AWAITING_RETRY {
		t.Errorf("crewState=3 (AwaitingRetry): expected LOOKOUT_STATE_AWAITING_RETRY (%d), got %v",
			sessionv1.LookoutState_LOOKOUT_STATE_AWAITING_RETRY, result.LookoutState)
	}
}

// TestEnrichLookoutState_StoppedState verifies that crewState=5 (LookoutStopped)
// maps to proto LOOKOUT_STATE_STOPPED (6).
func TestEnrichLookoutState_StoppedState(t *testing.T) {
	// crew.LookoutStopped = 5 → proto LOOKOUT_STATE_STOPPED = 6
	svc := newTestSessionServiceForEnrich(&mockFixerStateProvider{state: 5})
	sess := &sessionv1.Session{Id: "session-stopped"}
	result := svc.enrichLookoutState(sess)
	if result.LookoutState != sessionv1.LookoutState_LOOKOUT_STATE_STOPPED {
		t.Errorf("crewState=5 (Stopped): expected LOOKOUT_STATE_STOPPED (%d), got %v",
			sessionv1.LookoutState_LOOKOUT_STATE_STOPPED, result.LookoutState)
	}
}

// TestEnrichLookoutState_UsesSessionID verifies that the correct session ID is passed to
// LookoutStateFor — i.e. protoSess.Id is used, not some other field.
func TestEnrichLookoutState_UsesSessionID(t *testing.T) {
	// Use a state provider that captures the sessionID it was called with.
	var captured string
	svc := newTestSessionServiceForEnrich(FixerStateProviderFunc(func(sessionID string) int {
		captured = sessionID
		return 0
	}))
	sess := &sessionv1.Session{Id: "specific-session-id"}
	svc.enrichLookoutState(sess)
	if captured != "specific-session-id" {
		t.Errorf("expected LookoutStateFor to be called with 'specific-session-id', got %q", captured)
	}
}

// FixerStateProviderFunc is a function adapter for FixerStateProvider, used in tests.
type FixerStateProviderFunc func(sessionID string) int

func (f FixerStateProviderFunc) LookoutStateFor(sessionID string) int {
	return f(sessionID)
}

func (f FixerStateProviderFunc) LookoutRetryFor(sessionID string) (int, int) {
	return 0, 0
}
