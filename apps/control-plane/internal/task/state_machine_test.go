package task

import "testing"

func TestStateMachine_CanTransition(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		name string
		from TaskStatus
		to   TaskStatus
		want bool
	}{
		// Valid transitions from pending
		{"pending to claimed", TaskStatusPending, TaskStatusClaimed, true},
		{"pending to cancelled", TaskStatusPending, TaskStatusCancelled, true},
		{"pending to pending", TaskStatusPending, TaskStatusPending, true}, // Same state

		// Invalid transitions from pending
		{"pending to running", TaskStatusPending, TaskStatusRunning, false},
		{"pending to completed", TaskStatusPending, TaskStatusCompleted, false},
		{"pending to failed", TaskStatusPending, TaskStatusFailed, false},

		// Valid transitions from claimed
		{"claimed to running", TaskStatusClaimed, TaskStatusRunning, true},
		{"claimed to cancelled", TaskStatusClaimed, TaskStatusCancelled, true},
		{"claimed to completed", TaskStatusClaimed, TaskStatusCompleted, true},
		{"claimed to failed", TaskStatusClaimed, TaskStatusFailed, true},
		{"claimed to pending", TaskStatusClaimed, TaskStatusPending, true}, // Unclaim

		// Valid transitions from running
		{"running to completed", TaskStatusRunning, TaskStatusCompleted, true},
		{"running to failed", TaskStatusRunning, TaskStatusFailed, true},
		{"running to cancelled", TaskStatusRunning, TaskStatusCancelled, true},

		// Invalid transitions from running
		{"running to pending", TaskStatusRunning, TaskStatusPending, false},
		{"running to claimed", TaskStatusRunning, TaskStatusClaimed, false},

		// Terminal states - no transitions
		{"completed to any", TaskStatusCompleted, TaskStatusPending, false},
		{"failed to any", TaskStatusFailed, TaskStatusPending, false},
		{"cancelled to any", TaskStatusCancelled, TaskStatusPending, false},

		// Invalid statuses
		{"invalid from", TaskStatus("invalid"), TaskStatusPending, false},
		{"invalid to", TaskStatusPending, TaskStatus("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sm.CanTransition(tt.from, tt.to)
			if got != tt.want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}

func TestStateMachine_ValidateTransition(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		name    string
		from    TaskStatus
		to      TaskStatus
		wantErr bool
	}{
		{"valid transition", TaskStatusPending, TaskStatusClaimed, false},
		{"invalid transition", TaskStatusPending, TaskStatusCompleted, true},
		{"invalid from status", TaskStatus("invalid"), TaskStatusPending, true},
		{"invalid to status", TaskStatusPending, TaskStatus("invalid"), true},
		{"same status", TaskStatusPending, TaskStatusPending, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := sm.ValidateTransition(tt.from, tt.to)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTransition(%s, %s) error = %v, wantErr %v", tt.from, tt.to, err, tt.wantErr)
			}
		})
	}
}

func TestStateMachine_GetAllowedTransitions(t *testing.T) {
	sm := NewStateMachine()

	tests := []struct {
		name      string
		from      TaskStatus
		wantCount int
	}{
		{"pending transitions", TaskStatusPending, 2},     // claimed, cancelled
		{"claimed transitions", TaskStatusClaimed, 5},     // running, completed, failed, cancelled, pending
		{"running transitions", TaskStatusRunning, 3},     // completed, failed, cancelled
		{"completed transitions", TaskStatusCompleted, 0}, // terminal
		{"failed transitions", TaskStatusFailed, 0},       // terminal
		{"cancelled transitions", TaskStatusCancelled, 0}, // terminal
		{"invalid status", TaskStatus("invalid"), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sm.GetAllowedTransitions(tt.from)
			if len(got) != tt.wantCount {
				t.Errorf("GetAllowedTransitions(%s) returned %d transitions, want %d", tt.from, len(got), tt.wantCount)
			}
		})
	}
}

func TestTaskStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{"pending", TaskStatusPending, true},
		{"claimed", TaskStatusClaimed, true},
		{"running", TaskStatusRunning, true},
		{"completed", TaskStatusCompleted, true},
		{"failed", TaskStatusFailed, true},
		{"cancelled", TaskStatusCancelled, true},
		{"invalid", TaskStatus("invalid"), false},
		{"empty", TaskStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsValid()
			if got != tt.want {
				t.Errorf("TaskStatus(%s).IsValid() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestTaskStatus_IsTerminal(t *testing.T) {
	tests := []struct {
		name   string
		status TaskStatus
		want   bool
	}{
		{"pending", TaskStatusPending, false},
		{"claimed", TaskStatusClaimed, false},
		{"running", TaskStatusRunning, false},
		{"completed", TaskStatusCompleted, true},
		{"failed", TaskStatusFailed, true},
		{"cancelled", TaskStatusCancelled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.IsTerminal()
			if got != tt.want {
				t.Errorf("TaskStatus(%s).IsTerminal() = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
