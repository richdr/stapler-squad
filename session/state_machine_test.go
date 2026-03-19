package session

import (
	"errors"
	"testing"
)

func TestCanTransition_ValidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
	}{
		// Creating transitions
		{"Creating -> Running", Creating, Running},
		{"Creating -> Stopped", Creating, Stopped},
		// Ready transitions
		{"Ready -> Running", Ready, Running},
		{"Ready -> Stopped", Ready, Stopped},
		// Running transitions
		{"Running -> Paused", Running, Paused},
		{"Running -> NeedsApproval", Running, NeedsApproval},
		{"Running -> Stopped", Running, Stopped},
		// Paused transitions
		{"Paused -> Running", Paused, Running},
		{"Paused -> Stopped", Paused, Stopped},
		// NeedsApproval transitions
		{"NeedsApproval -> Running", NeedsApproval, Running},
		{"NeedsApproval -> Paused", NeedsApproval, Paused},
		{"NeedsApproval -> Stopped", NeedsApproval, Stopped},
		// Loading transitions
		{"Loading -> Running", Loading, Running},
		{"Loading -> Stopped", Loading, Stopped},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !CanTransition(tt.from, tt.to) {
				t.Errorf("CanTransition(%s, %s) = false, want true", tt.from, tt.to)
			}
		})
	}
}

func TestCanTransition_InvalidTransitions(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
	}{
		// Stopped is terminal - cannot transition out
		{"Stopped -> Running", Stopped, Running},
		{"Stopped -> Paused", Stopped, Paused},
		// Paused cannot go directly to NeedsApproval
		{"Paused -> NeedsApproval", Paused, NeedsApproval},
		// Running cannot go back to Ready or Creating
		{"Running -> Ready", Running, Ready},
		{"Running -> Creating", Running, Creating},
		// Self-transitions are not allowed
		{"Running -> Running", Running, Running},
		{"Paused -> Paused", Paused, Paused},
		// Loading cannot go to Paused directly
		{"Loading -> Paused", Loading, Paused},
		// Ready cannot go to Paused directly
		{"Ready -> Paused", Ready, Paused},
		// Creating cannot go to Paused directly
		{"Creating -> Paused", Creating, Paused},
		// Paused cannot go to Creating
		{"Paused -> Creating", Paused, Creating},
		// Running cannot go to Loading
		{"Running -> Loading", Running, Loading},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if CanTransition(tt.from, tt.to) {
				t.Errorf("CanTransition(%s, %s) = true, want false", tt.from, tt.to)
			}
		})
	}
}

func TestCanTransition_UnknownStatus(t *testing.T) {
	unknownStatus := Status(999)
	if CanTransition(unknownStatus, Running) {
		t.Error("CanTransition with unknown from status should return false")
	}
	if CanTransition(Running, unknownStatus) {
		t.Error("CanTransition with unknown to status should return false")
	}
}

func TestErrInvalidTransition(t *testing.T) {
	err := ErrInvalidTransition{From: Paused, To: NeedsApproval}

	// Verify the error message format
	expected := "invalid transition: Paused -> NeedsApproval"
	if err.Error() != expected {
		t.Errorf("ErrInvalidTransition.Error() = %q, want %q", err.Error(), expected)
	}

	// Verify it can be detected with errors.As
	var target ErrInvalidTransition
	if !errors.As(err, &target) {
		t.Error("errors.As should match ErrInvalidTransition")
	}
	if target.From != Paused || target.To != NeedsApproval {
		t.Errorf("errors.As target = {%s, %s}, want {Paused, NeedsApproval}", target.From, target.To)
	}
}

func TestAllowedTransitions_StoppedIsTerminal(t *testing.T) {
	// Verify that Stopped has no outgoing transitions
	allowed, ok := allowedTransitions[Stopped]
	if !ok {
		t.Fatal("Stopped should be present in allowedTransitions map")
	}
	if len(allowed) != 0 {
		t.Errorf("Stopped should have 0 allowed transitions, got %d: %v", len(allowed), allowed)
	}
}

func TestAllowedTransitions_AllStatusesCovered(t *testing.T) {
	// Every defined status should have an entry in the transition table
	statuses := []Status{Creating, Ready, Running, Loading, Paused, NeedsApproval, Stopped}
	for _, s := range statuses {
		if _, ok := allowedTransitions[s]; !ok {
			t.Errorf("Status %s is not covered in allowedTransitions", s)
		}
	}
}
