package task

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TaskStatus represents the status of a task
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusClaimed   TaskStatus = "claimed"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

// IsValid checks if the status is valid
func (s TaskStatus) IsValid() bool {
	switch s {
	case TaskStatusPending, TaskStatusClaimed, TaskStatusRunning,
		TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	}
	return false
}

// IsTerminal checks if the status is terminal (no further transitions)
func (s TaskStatus) IsTerminal() bool {
	switch s {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	}
	return false
}

// Task represents a task in the domain model
type Task struct {
	ID             int64
	Title          string
	Description    *string
	CreatorID      *int64
	ProviderType   string
	TargetNodeID   *string
	AssignedNodeID *string
	Status         TaskStatus
	WorkspacePath  *string
	Params         []byte
	Priority       int32
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CreateTaskRequest represents a request to create a task
type CreateTaskRequest struct {
	Title         string
	Description   *string
	CreatorID     *int64
	ProviderType  string
	TargetNodeID  *string
	WorkspacePath *string
	Params        []byte
	Priority      int32
}

// UpdateTaskStatusRequest represents a request to update task status
type UpdateTaskStatusRequest struct {
	TaskID    int64
	NewStatus TaskStatus
	ChangedBy *string
	Reason    *string
}

// AssignTaskRequest represents a request to assign a task to a node
type AssignTaskRequest struct {
	TaskID         int64
	AssignedNodeID string
}

// ListTasksFilter represents filters for listing tasks
type ListTasksFilter struct {
	Status       *TaskStatus
	CreatorID    *int64
	ProviderType *string
	Limit        int32
	Offset       int32
}

// Helper functions to convert between pgtype and domain types

func textFromString(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func stringFromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func int8FromInt64(i *int64) pgtype.Int8 {
	if i == nil {
		return pgtype.Int8{Valid: false}
	}
	return pgtype.Int8{Int64: *i, Valid: true}
}

func int64FromInt8(i pgtype.Int8) *int64 {
	if !i.Valid {
		return nil
	}
	return &i.Int64
}

func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}
