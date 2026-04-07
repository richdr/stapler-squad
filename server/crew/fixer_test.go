package crew

import (
	"context"
	"testing"

	"github.com/tstapler/stapler-squad/session"
)

// mockInstanceFinder is a test double for InstanceFinder that returns a fixed Instance.
type mockInstanceFinder struct {
	inst *session.Instance
}

func (m *mockInstanceFinder) FindInstance(sessionID string) *session.Instance {
	return m.inst
}

// mockTmuxPaneChecker is a test double for TmuxPaneChecker.
type mockTmuxPaneChecker struct{}

func (m *mockTmuxPaneChecker) PaneCurrentCommand(sessionName string) (string, error) {
	return "claude", nil
}

func (m *mockTmuxPaneChecker) CapturePaneContent(sessionName string) (string, error) {
	return "", nil
}

// newTestFixer creates a Fixer with a no-op queue and optional InstanceFinder.
func newTestFixer(finder InstanceFinder) *Fixer {
	q := session.NewReviewQueue()
	return NewFixer(q, finder, &mockTmuxPaneChecker{}, 0)
}

// TestLookoutStateFor_NoActiveLookout verifies that LookoutStateFor returns 0 (LookoutIdle)
// when no Lookout has been registered for the given session.
func TestLookoutStateFor_NoActiveLookout(t *testing.T) {
	f := newTestFixer(nil)
	got := f.LookoutStateFor("nonexistent-session")
	if got != int(LookoutIdle) {
		t.Errorf("expected LookoutIdle (%d), got %d", int(LookoutIdle), got)
	}
}

// TestLookoutStateFor_ActiveLookout verifies that LookoutStateFor returns the Lookout's
// current state integer when a Lookout is registered for the session.
func TestLookoutStateFor_ActiveLookout(t *testing.T) {
	f := newTestFixer(nil)

	// Manually insert a Lookout in a known state into the map.
	ctx := context.Background()
	cfg := LookoutConfig{
		SessionID:  "test-session",
		GoingDark:  false,
		MaxRetries: defaultMaxRetries,
		WorkingDir: "/tmp",
	}
	doneCh := make(chan LookoutResult, 1)
	l := NewLookout(ctx, cfg, doneCh)
	// Force the Lookout into LookoutSweeping state so we can distinguish from Idle.
	l.setState(LookoutSweeping)

	f.mu.Lock()
	f.lookouts["test-session"] = l
	f.mu.Unlock()

	got := f.LookoutStateFor("test-session")
	if got != int(LookoutSweeping) {
		t.Errorf("expected LookoutSweeping (%d), got %d", int(LookoutSweeping), got)
	}

	// Clean up.
	l.Stop()
}

// capturingFixer wraps a Fixer and captures the LookoutConfig of the first Lookout spawned.
// It hooks into spawnLookout by subscribing to the queue: when spawnLookout runs it inserts
// the Lookout into the map, and we read cfg immediately after OnItemAdded returns (which is
// synchronous up to the point of inserting into the map and then calling Start()).
//
// Because spawnLookout acquires and releases f.mu before calling lookout.Start(), the cfg
// is set in the Lookout before any goroutines begin.  We capture it before the Lookout's
// run() goroutine can complete the sweep and remove itself from the map.

// captureGoingDarkCfg calls OnItemAdded and captures the GoingDark flag from the newly
// registered Lookout.  It reads the map without any sleep because spawnLookout inserts
// into the map synchronously (under the write-lock) before returning.
func captureGoingDarkCfg(f *Fixer, item *session.ReviewItem) (goingDark bool, found bool) {
	// Cancel the fixer context immediately after the call so the Lookout's run()
	// goroutine exits before it can complete a sweep and remove itself from the map.
	f.OnItemAdded(item)

	// spawnLookout holds f.mu for its entire duration; the Lookout is in the map
	// by the time OnItemAdded returns.
	f.mu.RLock()
	l, ok := f.lookouts[item.SessionID]
	if ok {
		goingDark = l.cfg.GoingDark
	}
	f.mu.RUnlock()
	return goingDark, ok
}

// TestGoingDarkCap_BelowLimit verifies that when fewer than defaultMaxGoingDarkSessions GoingDark
// Lookouts are active, a new session with AutonomousMode=true spawns in autonomous mode.
func TestGoingDarkCap_BelowLimit(t *testing.T) {
	// defaultMaxGoingDarkSessions - 1 = 4 GoingDark lookouts already active.
	// The 5th spawn (count = 4 at the time of spawn) should stay autonomous.
	inst := &session.Instance{AutonomousMode: true}
	finder := &mockInstanceFinder{inst: inst}

	q := session.NewReviewQueue()

	// Use a cancelled context so Lookout goroutines exit immediately, preventing
	// them from completing sweeps and removing themselves from the map before we
	// can read the cfg.
	ctx, cancel := context.WithCancel(context.Background())
	f := NewFixer(q, finder, &mockTmuxPaneChecker{}, 0)
	f.Start(ctx)

	// Inject maxGoingDarkSessions-1 fake GoingDark Lookouts directly.
	for i := 0; i < defaultMaxGoingDarkSessions-1; i++ {
		sessionID := "existing-dark-session-" + string(rune('A'+i))
		cfg := LookoutConfig{
			SessionID:  sessionID,
			GoingDark:  true, // counts toward the cap
			MaxRetries: defaultMaxRetries,
			WorkingDir: "/tmp",
		}
		l := NewLookout(ctx, cfg, f.doneCh)

		f.mu.Lock()
		f.lookouts[sessionID] = l
		f.mu.Unlock()
	}

	item := &session.ReviewItem{
		SessionID:   "new-autonomous-session",
		SessionName: "new-autonomous-session",
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		WorkingDir:  "/tmp",
	}

	// Cancel before OnItemAdded so spawned Lookout goroutines exit immediately.
	cancel()

	goingDark, ok := captureGoingDarkCfg(f, item)
	f.Stop()

	if !ok {
		t.Fatal("expected a Lookout for 'new-autonomous-session' to be registered")
	}
	if !goingDark {
		t.Errorf("expected Lookout GoingDark=true (autonomous mode below cap), got false")
	}
}

// TestGoingDarkCap_SupervisedSession verifies that a session with AutonomousMode=false spawns in
// supervised mode (GoingDark=false) regardless of how much capacity is available.
func TestGoingDarkCap_SupervisedSession(t *testing.T) {
	inst := &session.Instance{AutonomousMode: false}
	finder := &mockInstanceFinder{inst: inst}

	ctx, cancel := context.WithCancel(context.Background())
	f := newTestFixer(finder)
	f.Start(ctx)

	item := &session.ReviewItem{
		SessionID:   "supervised-session",
		SessionName: "supervised-session",
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		WorkingDir:  "/tmp",
	}

	cancel() // exit goroutines immediately

	goingDark, ok := captureGoingDarkCfg(f, item)
	f.Stop()

	if !ok {
		t.Fatal("expected a Lookout for 'supervised-session' to be registered")
	}
	if goingDark {
		t.Errorf("expected Lookout GoingDark=false for supervised session (AutonomousMode=false), got true")
	}
}

// TestGoingDarkCap_AtLimit verifies that when defaultMaxGoingDarkSessions GoingDark Lookouts are
// already active, a new session with AutonomousMode=true is demoted to supervised mode.
func TestGoingDarkCap_AtLimit(t *testing.T) {
	inst := &session.Instance{AutonomousMode: true}
	finder := &mockInstanceFinder{inst: inst}

	q := session.NewReviewQueue()

	ctx, cancel := context.WithCancel(context.Background())
	f := NewFixer(q, finder, &mockTmuxPaneChecker{}, 0)
	f.Start(ctx)

	// Inject exactly defaultMaxGoingDarkSessions fake GoingDark Lookouts.
	for i := 0; i < defaultMaxGoingDarkSessions; i++ {
		sessionID := "dark-session-" + string(rune('A'+i))
		cfg := LookoutConfig{
			SessionID:  sessionID,
			GoingDark:  true,
			MaxRetries: defaultMaxRetries,
			WorkingDir: "/tmp",
		}
		l := NewLookout(ctx, cfg, f.doneCh)

		f.mu.Lock()
		f.lookouts[sessionID] = l
		f.mu.Unlock()
	}

	item := &session.ReviewItem{
		SessionID:   "capped-session",
		SessionName: "capped-session",
		Reason:      session.ReasonTaskComplete,
		Priority:    session.PriorityLow,
		WorkingDir:  "/tmp",
	}

	// Cancel before OnItemAdded so spawned Lookout goroutines exit immediately.
	cancel()

	goingDark, ok := captureGoingDarkCfg(f, item)
	f.Stop()

	if !ok {
		t.Fatal("expected a Lookout for 'capped-session' to be registered")
	}
	if goingDark {
		t.Errorf("expected Lookout GoingDark=false (capped), got true")
	}
}
