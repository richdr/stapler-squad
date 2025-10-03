package ui

import (
	"claude-squad/session"
	"claude-squad/ui/overlay"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestCreateSessionSetupOverlay verifies the coordinator properly creates and retrieves session setup overlays
func TestCreateSessionSetupOverlay(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.Initialize()

	// Test creating the session setup overlay with required callbacks
	err := coordinator.CreateSessionSetupOverlay(overlay.SessionSetupCallbacks{
		OnComplete: func(options session.InstanceOptions) {
			// Callback provided for test
		},
	})
	if err != nil {
		t.Fatalf("CreateSessionSetupOverlay failed: %v", err)
	}

	// CRITICAL TEST: Verify we can immediately retrieve the overlay after creation
	// This test would have caught the bug where GetSessionSetupOverlay returns nil
	sessionSetupOverlay := coordinator.GetSessionSetupOverlay()
	if sessionSetupOverlay == nil {
		t.Fatal("GetSessionSetupOverlay returned nil immediately after CreateSessionSetupOverlay - callbacks cannot be set!")
	}

	// IMPORTANT: The overlay should NOT be visible yet after CreateSessionSetupOverlay
	// This is intentional - it allows callbacks to be configured before showing
	if coordinator.IsOverlayVisible(ComponentSessionSetupOverlay) {
		t.Error("Session setup overlay should NOT be visible after creation (allows callback setup)")
	}

	// Now show the overlay explicitly
	err = coordinator.ShowOverlay(ComponentSessionSetupOverlay)
	if err != nil {
		t.Fatalf("ShowOverlay failed: %v", err)
	}

	// After ShowOverlay, it should be visible and active
	if !coordinator.IsOverlayVisible(ComponentSessionSetupOverlay) {
		t.Error("Session setup overlay should be visible after ShowOverlay")
	}

	activeOverlay, active := coordinator.GetActiveOverlay()
	if !active {
		t.Error("Expected overlay to be active after ShowOverlay")
	}
	if activeOverlay != ComponentSessionSetupOverlay {
		t.Errorf("Expected active overlay to be SessionSetupOverlay, got %v", activeOverlay)
	}
}

// TestSessionSetupOverlayCallbackSetup verifies that callbacks are properly set at construction time
func TestSessionSetupOverlayCallbackSetup(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.Initialize()

	// Create overlay with callbacks - the NEW way
	var completeCalled bool
	var cancelCalled bool

	err := coordinator.CreateSessionSetupOverlay(overlay.SessionSetupCallbacks{
		OnComplete: func(options session.InstanceOptions) {
			completeCalled = true
		},
		OnCancel: func() {
			cancelCalled = true
		},
	})
	if err != nil {
		t.Fatalf("CreateSessionSetupOverlay failed: %v", err)
	}

	// Get the overlay
	sessionSetupOverlay := coordinator.GetSessionSetupOverlay()
	if sessionSetupOverlay == nil {
		t.Fatal("Cannot get session setup overlay after creation")
	}

	// Callbacks should not be called yet (they're set but not invoked)
	if completeCalled {
		t.Error("Complete callback was called unexpectedly")
	}
	if cancelCalled {
		t.Error("Cancel callback was called unexpectedly")
	}
}

// TestSessionSetupOverlayWithoutCallback tests that creating without callback panics
func TestSessionSetupOverlayWithoutCallback(t *testing.T) {
	// This test verifies that the new design PREVENTS the "callback not set" error
	// by REQUIRING callbacks at construction time

	// Attempting to create without a callback should panic
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when creating SessionSetupOverlay without OnComplete callback")
		}
	}()

	// This should panic due to nil OnComplete
	_ = overlay.NewSessionSetupOverlay(overlay.SessionSetupCallbacks{
		OnComplete: nil, // This will cause a panic
	})
}

// TestSessionSetupOverlayCompleteFlow tests the complete flow with callback
func TestSessionSetupOverlayCompleteFlow(t *testing.T) {
	coordinator := NewCoordinator()
	coordinator.Initialize()

	var completeCalled bool
	var receivedOptions session.InstanceOptions

	// Create overlay with callbacks at construction time
	err := coordinator.CreateSessionSetupOverlay(overlay.SessionSetupCallbacks{
		OnComplete: func(options session.InstanceOptions) {
			completeCalled = true
			receivedOptions = options
		},
	})
	if err != nil {
		t.Fatalf("CreateSessionSetupOverlay failed: %v", err)
	}

	// Get the overlay
	sessionSetupOverlay := coordinator.GetSessionSetupOverlay()
	if sessionSetupOverlay == nil {
		t.Fatal("Cannot get session setup overlay")
	}

	// Focus the overlay
	sessionSetupOverlay.Focus()

	// Simulate completing the flow
	// Step 1: Enter name
	for _, r := range "test-session" {
		nameInput := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		sessionSetupOverlay.Update(nameInput)
	}

	// Advance through steps
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}

	// Basics -> Location
	sessionSetupOverlay.Update(enterKey)

	// Location (current) -> Branch
	sessionSetupOverlay.Update(enterKey)

	// Branch (new) -> Confirm
	sessionSetupOverlay.Update(enterKey)

	// Confirm -> Complete (should trigger callback)
	sessionSetupOverlay.Update(enterKey)

	// Verify callback was called
	if !completeCalled {
		t.Error("Complete callback was not called when pressing Enter at confirmation")
	}

	// Verify we received the session options
	if receivedOptions.Title != "test-session" {
		t.Errorf("Expected session title 'test-session', got '%s'", receivedOptions.Title)
	}
}
