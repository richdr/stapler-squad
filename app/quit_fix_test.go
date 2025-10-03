package app

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestContextCancellationOnQuit verifies that handleQuit cancels the context
func TestContextCancellationOnQuit(t *testing.T) {
	// Create a cancellable context like Run() does
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create home instance and set cancel function like Run() does
	h := newHome(ctx, "test-program", true)
	h.cancelFunc = cancel

	// Verify context is not cancelled initially
	select {
	case <-h.ctx.Done():
		t.Fatal("Context should not be cancelled initially")
	default:
		// Good - context not cancelled
	}

	// Call handleQuit
	_, _ = h.handleQuit()

	// Verify context is cancelled
	select {
	case <-h.ctx.Done():
		// Good - context was cancelled
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context should be cancelled after handleQuit")
	}
}

// TestHealthCheckerCancellation verifies health checker can be interrupted
func TestHealthCheckerCancellation(t *testing.T) {
	// Create a cancellable context like Run() does
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create home instance and set cancel function like Run() does
	h := newHome(ctx, "test-program", true)
	h.cancelFunc = cancel

	// Start health checker in background
	healthCheckerStarted := make(chan struct{})
	healthCheckerExited := make(chan struct{})

	go func() {
		close(healthCheckerStarted)
		h.startBackgroundHealthChecker()
		close(healthCheckerExited)
	}()

	// Wait for health checker to start
	<-healthCheckerStarted
	time.Sleep(100 * time.Millisecond) // Give it time to enter select

	// Cancel context (simulating quit)
	startTime := time.Now()
	if h.cancelFunc != nil {
		h.cancelFunc()
	}

	// Wait for health checker to exit
	select {
	case <-healthCheckerExited:
		elapsed := time.Since(startTime)
		// Should exit quickly (within 1 second)
		assert.Less(t, elapsed, 1*time.Second,
			"Health checker should exit quickly after context cancellation")
		t.Logf("Health checker exited in %v", elapsed)
	case <-time.After(5 * time.Second):
		t.Fatal("Health checker did not exit after context cancellation")
	}
}

// TestHealthCheckerWithoutCancellation verifies old blocking behavior would have failed
func TestHealthCheckerWithoutCancellation(t *testing.T) {
	t.Skip("This test documents the old broken behavior - it would hang for 30s")

	// Old code would do:
	// time.Sleep(30 * time.Second)  // BLOCKS - can't be interrupted!
	//
	// New code does:
	// timer := time.NewTimer(30 * time.Second)
	// select {
	// case <-timer.C:
	//     // continue
	// case <-ctx.Done():
	//     return  // Exit immediately!
	// }
}

// TestQuitSequenceTiming measures how long handleQuit takes
func TestQuitSequenceTiming(t *testing.T) {
	// Create a cancellable context like Run() does
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create home instance and set cancel function like Run() does
	h := newHome(ctx, "test-program", true)
	h.cancelFunc = cancel

	// Measure handleQuit execution time
	startTime := time.Now()
	_, _ = h.handleQuit()
	elapsed := time.Since(startTime)

	// handleQuit should complete quickly (under 3 seconds)
	// It includes:
	// - Context cancellation (instant)
	// - SaveInstancesSync (should be fast with no sessions)
	// - storage.Close with 2s StateService timeout (might timeout)
	assert.Less(t, elapsed, 3*time.Second,
		"handleQuit should complete within 3 seconds")
	t.Logf("handleQuit took %v", elapsed)
}
