package task

import (
	"time"

	"github.com/google/uuid"
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
	ID             uuid.UUID
	TenantID       uuid.UUID
	TeamID         *uuid.UUID
	Title          string
	Description    *string
	CreatorID      *uuid.UUID
	ProviderType   string
	TargetNodeID   *string
	AssignedNodeID *string
	Status         TaskStatus
	WorkspacePath  *string
	Params         []byte
	Priority       int32
	CancelledAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TaskEvent represents a structured runtime event for a task.
type TaskEvent struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	TaskID         uuid.UUID
	RunID          *uuid.UUID
	EventType      string
	SequenceNumber int32
	Payload        []byte
	CreatedAt      time.Time
}

// CreateTaskRequest represents a request to create a task
type CreateTaskRequest struct {
	TenantID      *uuid.UUID
	TeamID        *uuid.UUID
	Title         string
	Description   *string
	CreatorID     *uuid.UUID
	ProviderType  string
	TargetNodeID  *string
	WorkspacePath *string
	Params        []byte
	Priority      int32
}

// AppendTaskEventRequest represents a request to append a runtime task event.
type AppendTaskEventRequest struct {
	TenantID  *uuid.UUID
	TaskID    uuid.UUID
	RunID     *uuid.UUID
	EventType string
	Payload   []byte
}

// UpdateTaskStatusRequest represents a request to update task status
type UpdateTaskStatusRequest struct {
	TenantID  *uuid.UUID
	TaskID    uuid.UUID
	NewStatus TaskStatus
	ChangedBy *string
	Reason    *string
}

// CompleteTaskRequest represents a runtime task completion request.
type CompleteTaskRequest struct {
	TaskID uuid.UUID
	Result []byte
}

// FailTaskRequest represents a runtime task failure request.
type FailTaskRequest struct {
	TaskID uuid.UUID
	Error  string
}

// AssignTaskRequest represents a request to assign a task to a node
type AssignTaskRequest struct {
	TenantID       *uuid.UUID
	TaskID         uuid.UUID
	AssignedNodeID string
}

// ListTasksFilter represents filters for listing tasks
type ListTasksFilter struct {
	TenantID     *uuid.UUID
	Status       *TaskStatus
	CreatorID    *uuid.UUID
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

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func ptrFromNullUUID(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	return &value.UUID
}

func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func timePtrFromTimestamptz(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
