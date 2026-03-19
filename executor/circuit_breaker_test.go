package executor

import (
	"errors"
	"os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Executor ---

type mockExecutor struct {
	runFunc            func(cmd *exec.Cmd) error
	outputFunc         func(cmd *exec.Cmd) ([]byte, error)
	combinedOutputFunc func(cmd *exec.Cmd) ([]byte, error)
}

func (m *mockExecutor) Run(cmd *exec.Cmd) error {
	if m.runFunc != nil {
		return m.runFunc(cmd)
	}
	return nil
}

func (m *mockExecutor) Output(cmd *exec.Cmd) ([]byte, error) {
	if m.outputFunc != nil {
		return m.outputFunc(cmd)
	}
	return []byte("ok"), nil
}

func (m *mockExecutor) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	if m.combinedOutputFunc != nil {
		return m.combinedOutputFunc(cmd)
	}
	if m.outputFunc != nil {
		return m.outputFunc(cmd)
	}
	return []byte("ok"), nil
}

// --- Mock Clock ---

type mockClock struct {
	mu  sync.Mutex
	now time.Time
}

func newMockClock(t time.Time) *mockClock {
	return &mockClock{now: t}
}

func (c *mockClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *mockClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// --- Helpers ---

var errSimulated = errors.New("simulated failure")

func failingExecutor() *mockExecutor {
	return &mockExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return errSimulated
		},
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return nil, errSimulated
		},
	}
}

func succeedingExecutor() *mockExecutor {
	return &mockExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			return nil
		},
		outputFunc: func(cmd *exec.Cmd) ([]byte, error) {
			return []byte("success"), nil
		},
	}
}

func dummyCmd(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}

// --- commandClass tests ---

func TestCommandClass(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *exec.Cmd
		expected string
	}{
		{
			name:     "nil command",
			cmd:      nil,
			expected: "unknown",
		},
		{
			name:     "simple command",
			cmd:      exec.Command("git"),
			expected: "git",
		},
		{
			name:     "command with subcommand",
			cmd:      exec.Command("git", "diff"),
			expected: "git-diff",
		},
		{
			name:     "command with flags before subcommand",
			cmd:      exec.Command("git", "--no-pager", "log"),
			expected: "git-log",
		},
		{
			name:     "command with only flags",
			cmd:      exec.Command("ls", "-la", "-R"),
			expected: "ls",
		},
		{
			name:     "tmux capture-pane",
			cmd:      exec.Command("tmux", "capture-pane", "-t", "session"),
			expected: "tmux-capture-pane",
		},
		{
			name:     "full path binary",
			cmd:      exec.Command("/usr/bin/git", "status"),
			expected: "git-status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := commandClass(tc.cmd)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// --- CircuitState String tests ---

func TestCircuitStateString(t *testing.T) {
	assert.Equal(t, "CLOSED", CircuitClosed.String())
	assert.Equal(t, "OPEN", CircuitOpen.String())
	assert.Equal(t, "HALF-OPEN", CircuitHalfOpen.String())
	assert.Equal(t, "UNKNOWN", CircuitState(99).String())
}

// --- DefaultCircuitBreakerConfig tests ---

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	assert.Equal(t, 3, cfg.FailureThreshold)
	assert.Equal(t, 30*time.Second, cfg.RecoveryTimeout)
}

// --- Core state transition tests (table-driven) ---

func TestCircuitBreakerStateTransitions(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock)
	}{
		{
			name: "Closed to Open: consecutive failures trip the breaker",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// 3 consecutive failures should trip the breaker (threshold = 3).
				for i := 0; i < 3; i++ {
					err := cbe.Run(dummyCmd("git", "diff"))
					assert.ErrorIs(t, err, errSimulated, "failure %d should propagate", i+1)
				}

				// The 4th call should be rejected by the open breaker.
				err := cbe.Run(dummyCmd("git", "diff"))
				assert.ErrorIs(t, err, ErrCircuitOpen, "breaker should be open after 3 failures")

				// Verify state via snapshot.
				snaps := cbe.AllBreakers()
				snap, ok := snaps["git-diff"]
				require.True(t, ok, "should have a breaker for git-diff")
				assert.Equal(t, CircuitOpen, snap.State)
				assert.Equal(t, 3, snap.ConsecutiveFailures)
			},
		},
		{
			name: "Open rejects calls with ErrCircuitOpen",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "status"))
				}

				// Multiple calls should all be rejected immediately.
				for i := 0; i < 5; i++ {
					err := cbe.Run(dummyCmd("git", "status"))
					assert.ErrorIs(t, err, ErrCircuitOpen, "call %d should be rejected", i+1)
				}
			},
		},
		{
			name: "Open to Half-Open: recovery timeout allows one probe",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "diff"))
				}

				// Verify open.
				err := cbe.Run(dummyCmd("git", "diff"))
				assert.ErrorIs(t, err, ErrCircuitOpen)

				// Advance clock past recovery timeout.
				clock.Advance(31 * time.Second)

				// Swap delegate to a succeeding executor so the probe succeeds.
				cbe.delegate = succeedingExecutor()

				// The next call should be allowed as a half-open probe.
				err = cbe.Run(dummyCmd("git", "diff"))
				assert.NoError(t, err, "probe should be allowed after recovery timeout")
			},
		},
		{
			name: "Half-Open to Closed: successful probe resets breaker",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "log"))
				}

				// Advance past recovery.
				clock.Advance(31 * time.Second)

				// Swap to succeeding executor.
				cbe.delegate = succeedingExecutor()

				// Probe succeeds -> should transition to CLOSED.
				err := cbe.Run(dummyCmd("git", "log"))
				assert.NoError(t, err)

				// Verify breaker is now CLOSED.
				snaps := cbe.AllBreakers()
				snap := snaps["git-log"]
				assert.Equal(t, CircuitClosed, snap.State)
				assert.Equal(t, 0, snap.ConsecutiveFailures)

				// Subsequent calls should work normally.
				err = cbe.Run(dummyCmd("git", "log"))
				assert.NoError(t, err)
			},
		},
		{
			name: "Half-Open to Open: failed probe re-opens breaker",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "push"))
				}

				// Advance past recovery.
				clock.Advance(31 * time.Second)

				// Delegate still fails -- probe will fail.
				err := cbe.Run(dummyCmd("git", "push"))
				assert.ErrorIs(t, err, errSimulated, "probe should execute and fail")

				// Breaker should be OPEN again.
				snaps := cbe.AllBreakers()
				snap := snaps["git-push"]
				assert.Equal(t, CircuitOpen, snap.State)

				// Calls should be rejected.
				err = cbe.Run(dummyCmd("git", "push"))
				assert.ErrorIs(t, err, ErrCircuitOpen)
			},
		},
		{
			name: "Closed stays Closed: intermittent failures do not trip",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Alternate: fail, succeed, fail, succeed, fail, succeed.
				// Consecutive failures never reach threshold of 3.
				fail := failingExecutor()
				succeed := succeedingExecutor()

				for i := 0; i < 3; i++ {
					cbe.delegate = fail
					_ = cbe.Run(dummyCmd("git", "fetch"))

					cbe.delegate = succeed
					err := cbe.Run(dummyCmd("git", "fetch"))
					assert.NoError(t, err)
				}

				// Breaker should still be CLOSED.
				snaps := cbe.AllBreakers()
				snap := snaps["git-fetch"]
				assert.Equal(t, CircuitClosed, snap.State)
				assert.Equal(t, 0, snap.ConsecutiveFailures)
			},
		},
		{
			name: "Command-class isolation: independent breakers for different classes",
			setup: func(t *testing.T, cbe *CircuitBreakerExecutor, clock *mockClock) {
				// Trip the breaker for "git-diff" only.
				for i := 0; i < 3; i++ {
					_ = cbe.Run(dummyCmd("git", "diff"))
				}

				// git-diff should be open.
				err := cbe.Run(dummyCmd("git", "diff"))
				assert.ErrorIs(t, err, ErrCircuitOpen)

				// git-status should still work (independent breaker).
				cbe.delegate = succeedingExecutor()
				err = cbe.Run(dummyCmd("git", "status"))
				assert.NoError(t, err)

				// tmux-capture-pane should also work.
				err = cbe.Run(dummyCmd("tmux", "capture-pane"))
				assert.NoError(t, err)

				// Verify independent states.
				snaps := cbe.AllBreakers()
				assert.Equal(t, CircuitOpen, snaps["git-diff"].State)
				assert.Equal(t, CircuitClosed, snaps["git-status"].State)
				assert.Equal(t, CircuitClosed, snaps["tmux-capture-pane"].State)
			},
		},
		{
			name: "Config validation: custom thresholds respected",
			setup: func(t *testing.T, _ *CircuitBreakerExecutor, clock *mockClock) {
				// Create a new executor with custom config.
				customCfg := CircuitBreakerConfig{
					FailureThreshold: 5,
					RecoveryTimeout:  10 * time.Second,
				}
				customCBE := NewCircuitBreakerExecutorWithClock(failingExecutor(), customCfg, clock)

				// 4 failures should NOT trip (threshold is 5).
				for i := 0; i < 4; i++ {
					err := customCBE.Run(dummyCmd("git", "pull"))
					assert.ErrorIs(t, err, errSimulated)
				}
				snaps := customCBE.AllBreakers()
				assert.Equal(t, CircuitClosed, snaps["git-pull"].State)

				// 5th failure trips it.
				_ = customCBE.Run(dummyCmd("git", "pull"))
				snaps = customCBE.AllBreakers()
				assert.Equal(t, CircuitOpen, snaps["git-pull"].State)

				// 10s recovery (not 30s default).
				clock.Advance(11 * time.Second)
				customCBE.delegate = succeedingExecutor()
				err := customCBE.Run(dummyCmd("git", "pull"))
				assert.NoError(t, err, "should recover after custom timeout")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
			cfg := CircuitBreakerConfig{
				FailureThreshold: 3,
				RecoveryTimeout:  30 * time.Second,
			}
			cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)
			tc.setup(t, cbe, clock)
		})
	}
}

// --- Output method tests ---

func TestCircuitBreakerExecutor_Output(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 2,
		RecoveryTimeout:  10 * time.Second,
	}

	t.Run("Output succeeds through closed breaker", func(t *testing.T) {
		cbe := NewCircuitBreakerExecutorWithClock(succeedingExecutor(), cfg, clock)
		out, err := cbe.Output(dummyCmd("git", "log"))
		assert.NoError(t, err)
		assert.Equal(t, []byte("success"), out)
	})

	t.Run("Output returns ErrCircuitOpen when breaker is open", func(t *testing.T) {
		cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

		// Trip breaker (threshold=2).
		for i := 0; i < 2; i++ {
			_, _ = cbe.Output(dummyCmd("git", "show"))
		}

		out, err := cbe.Output(dummyCmd("git", "show"))
		assert.ErrorIs(t, err, ErrCircuitOpen)
		assert.Nil(t, out)
	})

	t.Run("Output probe recovers breaker", func(t *testing.T) {
		cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

		// Trip breaker.
		for i := 0; i < 2; i++ {
			_, _ = cbe.Output(dummyCmd("git", "blame"))
		}

		// Advance past recovery.
		clock.Advance(11 * time.Second)
		cbe.delegate = succeedingExecutor()

		out, err := cbe.Output(dummyCmd("git", "blame"))
		assert.NoError(t, err)
		assert.Equal(t, []byte("success"), out)

		// Verify closed.
		snaps := cbe.AllBreakers()
		assert.Equal(t, CircuitClosed, snaps["git-blame"].State)
	})
}

// --- Concurrent access test ---

func TestCircuitBreakerExecutor_ConcurrentAccess(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}
	cbe := NewCircuitBreakerExecutorWithClock(succeedingExecutor(), cfg, clock)

	const numGoroutines = 50
	const numCallsPerGoroutine = 20
	var wg sync.WaitGroup
	var successCount atomic.Int64
	var errorCount atomic.Int64

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			// Use a mix of command classes.
			cmds := [][]string{
				{"git", "diff"},
				{"git", "status"},
				{"tmux", "capture-pane"},
			}
			for j := 0; j < numCallsPerGoroutine; j++ {
				cmdArgs := cmds[j%len(cmds)]
				cmd := dummyCmd(cmdArgs[0], cmdArgs[1:]...)
				err := cbe.Run(cmd)
				if err == nil {
					successCount.Add(1)
				} else {
					errorCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	// All calls should succeed since the delegate always succeeds.
	total := numGoroutines * numCallsPerGoroutine
	assert.Equal(t, int64(total), successCount.Load(), "all calls should succeed")
	assert.Equal(t, int64(0), errorCount.Load(), "no errors expected")

	// All breakers should be in CLOSED state.
	snaps := cbe.AllBreakers()
	for class, snap := range snaps {
		assert.Equal(t, CircuitClosed, snap.State, "breaker %s should be CLOSED", class)
	}
}

// --- Half-Open serialization test (BUG-003 prevention) ---

func TestCircuitBreakerExecutor_HalfOpenSerializesProbes(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}

	// The probe executor blocks until we release it.
	probeStarted := make(chan struct{}, 1)
	probeRelease := make(chan struct{})
	var probeCount atomic.Int64

	blockingExec := &mockExecutor{
		runFunc: func(cmd *exec.Cmd) error {
			probeCount.Add(1)
			probeStarted <- struct{}{}
			<-probeRelease
			return nil // success
		},
	}

	cbe := NewCircuitBreakerExecutorWithClock(failingExecutor(), cfg, clock)

	// Trip the breaker.
	for i := 0; i < 3; i++ {
		_ = cbe.Run(dummyCmd("git", "diff"))
	}

	// Advance past recovery timeout.
	clock.Advance(31 * time.Second)

	// Switch to blocking executor for the probe.
	cbe.delegate = blockingExec

	// Launch the probe goroutine.
	var probeErr error
	var probeDone sync.WaitGroup
	probeDone.Add(1)
	go func() {
		defer probeDone.Done()
		probeErr = cbe.Run(dummyCmd("git", "diff"))
	}()

	// Wait for the probe to start executing.
	<-probeStarted

	// While the probe is in flight, launch concurrent requests.
	// They should all be rejected since exactly one probe is allowed.
	const concurrentAttempts = 10
	rejectedCount := atomic.Int64{}
	var attemptWg sync.WaitGroup
	attemptWg.Add(concurrentAttempts)
	for i := 0; i < concurrentAttempts; i++ {
		go func() {
			defer attemptWg.Done()
			err := cbe.Run(dummyCmd("git", "diff"))
			if errors.Is(err, ErrCircuitOpen) {
				rejectedCount.Add(1)
			}
		}()
	}

	attemptWg.Wait()

	// All concurrent attempts should have been rejected.
	assert.Equal(t, int64(concurrentAttempts), rejectedCount.Load(),
		"all concurrent attempts during HALF-OPEN probe should be rejected")

	// Only one probe should have started.
	assert.Equal(t, int64(1), probeCount.Load(), "exactly one probe should execute")

	// Release the probe.
	close(probeRelease)
	probeDone.Wait()

	assert.NoError(t, probeErr, "probe should succeed")

	// Breaker should now be CLOSED.
	snaps := cbe.AllBreakers()
	assert.Equal(t, CircuitClosed, snaps["git-diff"].State)
}

// --- AllBreakers snapshot test ---

func TestCircuitBreakerExecutor_AllBreakers(t *testing.T) {
	clock := newMockClock(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	cfg := CircuitBreakerConfig{
		FailureThreshold: 3,
		RecoveryTimeout:  30 * time.Second,
	}
	cbe := NewCircuitBreakerExecutorWithClock(succeedingExecutor(), cfg, clock)

	// No breakers initially.
	snaps := cbe.AllBreakers()
	assert.Empty(t, snaps)

	// Create some breakers.
	_ = cbe.Run(dummyCmd("git", "diff"))
	_ = cbe.Run(dummyCmd("git", "status"))
	_ = cbe.Run(dummyCmd("tmux", "capture-pane"))

	snaps = cbe.AllBreakers()
	assert.Len(t, snaps, 3)
	assert.Contains(t, snaps, "git-diff")
	assert.Contains(t, snaps, "git-status")
	assert.Contains(t, snaps, "tmux-capture-pane")

	for _, snap := range snaps {
		assert.Equal(t, CircuitClosed, snap.State)
		assert.Equal(t, 0, snap.ConsecutiveFailures)
		assert.Equal(t, cfg.FailureThreshold, snap.Config.FailureThreshold)
		assert.Equal(t, cfg.RecoveryTimeout, snap.Config.RecoveryTimeout)
	}
}

// --- NewCircuitBreakerExecutor (real clock) ---

func TestNewCircuitBreakerExecutor_UsesRealClock(t *testing.T) {
	cbe := NewCircuitBreakerExecutor(succeedingExecutor(), DefaultCircuitBreakerConfig())
	assert.NotNil(t, cbe)
	assert.NotNil(t, cbe.clock)

	// Should work with real time.
	err := cbe.Run(dummyCmd("echo", "hello"))
	assert.NoError(t, err)
}
