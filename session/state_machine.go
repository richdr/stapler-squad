package session

// allowedTransitions defines the valid state machine transitions.
// Any transition not explicitly listed here is considered invalid.
//
// State machine diagram:
//
//	Creating  --> Running, Stopped
//	Ready     --> Running, Stopped
//	Running   --> Paused, NeedsApproval, Stopped
//	Paused    --> Running, Stopped
//	NeedsApproval --> Running, Paused, Stopped
//	Loading   --> Running, Stopped
//	Stopped   --> (terminal state, no outgoing transitions)
var allowedTransitions = map[Status][]Status{
	Creating:      {Running, Stopped},
	Ready:         {Running, Stopped},
	Running:       {Paused, NeedsApproval, Stopped},
	Paused:        {Running, Stopped},
	NeedsApproval: {Running, Paused, Stopped},
	Loading:       {Running, Stopped},
	Stopped:       {},
}

// CanTransition returns true if transitioning from -> to is a valid state transition.
func CanTransition(from, to Status) bool {
	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
