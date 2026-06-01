package task

import "fmt"

// StateMachine defines the allowed state transitions for tasks
type StateMachine struct {
	transitions map[TaskStatus][]TaskStatus
}

// NewStateMachine creates a new state machine with predefined transitions
func NewStateMachine() *StateMachine {
	return &StateMachine{
		transitions: map[TaskStatus][]TaskStatus{
			TaskStatusPending: {
				TaskStatusClaimed,
				TaskStatusCancelled,
			},
			TaskStatusClaimed: {
				TaskStatusRunning,
				TaskStatusCompleted,
				TaskStatusFailed,
				TaskStatusCancelled,
				TaskStatusPending, // Allow unclaim
			},
			TaskStatusRunning: {
				TaskStatusCompleted,
				TaskStatusFailed,
				TaskStatusCancelled,
			},
			// Terminal states have no transitions
			TaskStatusCompleted: {},
			TaskStatusFailed:    {},
			TaskStatusCancelled: {},
		},
	}
}

// CanTransition checks if a transition from one status to another is allowed
func (sm *StateMachine) CanTransition(from, to TaskStatus) bool {
	// Validate statuses
	if !from.IsValid() || !to.IsValid() {
		return false
	}

	// No transition needed
	if from == to {
		return true
	}

	// Check if transition is allowed
	allowedTransitions, exists := sm.transitions[from]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == to {
			return true
		}
	}

	return false
}

// ValidateTransition validates a state transition and returns an error if invalid
func (sm *StateMachine) ValidateTransition(from, to TaskStatus) error {
	if !from.IsValid() {
		return fmt.Errorf("invalid source status: %s", from)
	}

	if !to.IsValid() {
		return fmt.Errorf("invalid target status: %s", to)
	}

	if from == to {
		return nil // No transition needed
	}

	if !sm.CanTransition(from, to) {
		return fmt.Errorf("invalid state transition from %s to %s", from, to)
	}

	return nil
}

// GetAllowedTransitions returns all allowed transitions from a given status
func (sm *StateMachine) GetAllowedTransitions(from TaskStatus) []TaskStatus {
	if !from.IsValid() {
		return nil
	}

	transitions, exists := sm.transitions[from]
	if !exists {
		return nil
	}

	// Return a copy to prevent modification
	result := make([]TaskStatus, len(transitions))
	copy(result, transitions)
	return result
}
