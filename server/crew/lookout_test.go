package crew

import (
	"context"
	"testing"
	"time"
)

// newTestLookout creates a Lookout with a buffered doneCh for test use.
func newTestLookout(t *testing.T, cfg LookoutConfig) (*Lookout, chan LookoutResult) {
	t.Helper()
	doneCh := make(chan LookoutResult, 4)
	l := NewLookout(context.Background(), cfg, doneCh)
	return l, doneCh
}

// TestLookoutInitialState verifies a new Lookout starts in LookoutIdle.
func TestLookoutInitialState(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-session", MaxRetries: 3})
	if l.State() != LookoutIdle {
		t.Errorf("expected LookoutIdle, got %v", l.State())
	}
}

// TestLookoutRetryCountInitial verifies RetryCount starts at 0.
func TestLookoutRetryCountInitial(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-session", MaxRetries: 3})
	if l.RetryCount() != 0 {
		t.Errorf("expected RetryCount=0, got %d", l.RetryCount())
	}
}

// TestLookoutMaxRetriesDefaulted verifies MaxRetries defaults to defaultMaxRetries when ≤0.
func TestLookoutMaxRetriesDefaulted(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-session", MaxRetries: 0})
	if l.MaxRetries() != defaultMaxRetries {
		t.Errorf("expected MaxRetries=%d, got %d", defaultMaxRetries, l.MaxRetries())
	}
}

// TestLookoutMaxRetriesRespected verifies a custom MaxRetries value is honoured.
func TestLookoutMaxRetriesRespected(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-session", MaxRetries: 7})
	if l.MaxRetries() != 7 {
		t.Errorf("expected MaxRetries=7, got %d", l.MaxRetries())
	}
}

// TestLookoutEarpieceChClosedOnStop verifies that Stop() causes EarpieceCh to close,
// which allows any watchEarpiece goroutine in the Fixer to exit without leaking.
func TestLookoutEarpieceChClosedOnStop(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-earpiece", MaxRetries: 3})
	l.Start()
	l.Stop()

	select {
	case _, open := <-l.EarpieceCh():
		if open {
			t.Error("expected earpieceCh to be closed after Stop(), got open channel")
		}
	case <-time.After(time.Second):
		t.Error("timed out waiting for earpieceCh to close after Stop()")
	}
}

// TestLookoutStateAfterStop verifies State() is LookoutStopped after Stop().
func TestLookoutStateAfterStop(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-stop", MaxRetries: 3})
	l.Start()
	l.Stop()

	if l.State() != LookoutStopped {
		t.Errorf("expected LookoutStopped after Stop(), got %v", l.State())
	}
}

// TestLookoutStopIsIdempotent verifies calling Stop() twice does not panic.
func TestLookoutStopIsIdempotent(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-idempotent", MaxRetries: 3})
	l.Start()
	l.Stop()
	// Second Stop should not deadlock or panic (context is already cancelled).
	done := make(chan struct{})
	go func() {
		l.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("second Stop() call blocked — potential deadlock")
	}
}

// TestLookoutBackoffDuration verifies backoff durations grow as expected.
func TestLookoutBackoffDuration(t *testing.T) {
	tests := []struct {
		attempt   int
		wantMin   time.Duration
		wantExact time.Duration
	}{
		{0, 5 * time.Second, 5 * time.Second},
		{1, 5 * time.Second, 5 * time.Second},
		{2, 10 * time.Second, 10 * time.Second},
		{3, 20 * time.Second, 20 * time.Second},
		{99, 20 * time.Second, 20 * time.Second},
	}
	for _, tt := range tests {
		got := backoffDuration(tt.attempt)
		if got != tt.wantExact {
			t.Errorf("backoffDuration(%d) = %v, want %v", tt.attempt, got, tt.wantExact)
		}
	}
}

// TestLookoutLookoutStateEnumValues verifies the explicit integer values of LookoutState
// constants match the proto wire encoding expectations.
// These values must not change without a corresponding proto enum update.
func TestLookoutLookoutStateEnumValues(t *testing.T) {
	tests := []struct {
		state LookoutState
		want  int
	}{
		{LookoutIdle, 0},
		{LookoutSweeping, 2},
		{LookoutAwaitingRetry, 3},
		{LookoutFallen, 4},
		{LookoutStopped, 5},
	}
	for _, tt := range tests {
		if int(tt.state) != tt.want {
			t.Errorf("LookoutState %v = %d, want %d", tt.state, int(tt.state), tt.want)
		}
	}
}

// TestLookoutOnTaskCompleteNonBlocking verifies OnTaskComplete does not block when
// the channel is already signalled (buffered channel, capacity 1).
func TestLookoutOnTaskCompleteNonBlocking(t *testing.T) {
	l, _ := newTestLookout(t, LookoutConfig{SessionID: "test-nonblocking", MaxRetries: 3})
	done := make(chan struct{})
	go func() {
		l.OnTaskComplete()
		l.OnTaskComplete() // second call must not block
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("OnTaskComplete blocked — possible channel deadlock")
	}
}
