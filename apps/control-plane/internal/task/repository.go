package task

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository defines the interface for task data access
type Repository interface {
	// Task operations
	CreateTask(ctx context.Context, params CreateTaskParams) (TaskRecord, error)
	GetTask(ctx context.Context, params GetTaskParams) (TaskRecord, error)
	ListTasks(ctx context.Context, params ListTasksParams) ([]TaskRecord, error)
	UpdateTaskStatus(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error)
	UpdateTask(ctx context.Context, params UpdateTaskParams) (TaskRecord, error)
	DeleteTask(ctx context.Context, params DeleteTaskParams) error

	// State history operations
	CreateTaskStateHistory(ctx context.Context, params CreateTaskStateHistoryParams) error

	// Event operations
	CreateTaskEvent(ctx context.Context, params CreateTaskEventParams) (TaskEventRecord, error)
	GetLatestTaskEventSequence(ctx context.Context, params GetLatestTaskEventSequenceParams) (int32, error)
}

// CreateTaskParams represents parameters for creating a task
type CreateTaskParams struct {
	TenantID      uuid.NullUUID
	TeamID        uuid.NullUUID
	Title         string
	Description   pgtype.Text
	Status        string
	Priority      int32
	ProviderType  string
	CreatorID     uuid.NullUUID
	TargetNodeID  pgtype.Text
	WorkspacePath pgtype.Text
	Params        []byte
}

// TaskRecord represents a task record from the database
type TaskRecord struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	TeamID         uuid.NullUUID
	Title          string
	Description    pgtype.Text
	CreatorID      uuid.NullUUID
	ProviderType   string
	TargetNodeID   pgtype.Text
	AssignedNodeID pgtype.Text
	Status         string
	WorkspacePath  pgtype.Text
	Params         []byte
	Priority       int32
	CancelledAt    pgtype.Timestamptz
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

// GetTaskParams represents parameters for reading a task in a tenant scope.
type GetTaskParams struct {
	TenantID uuid.NullUUID
	ID       uuid.UUID
}

// DeleteTaskParams represents parameters for deleting a task in a tenant scope.
type DeleteTaskParams struct {
	TenantID uuid.NullUUID
	ID       uuid.UUID
}

// CreateTaskEventParams represents parameters for creating a task event
type CreateTaskEventParams struct {
	TenantID       uuid.NullUUID
	TaskID         uuid.UUID
	RunID          uuid.NullUUID
	EventType      string
	SequenceNumber int32
	Payload        []byte
}

// GetLatestTaskEventSequenceParams represents parameters for reading an event sequence in a tenant scope.
type GetLatestTaskEventSequenceParams struct {
	TenantID uuid.NullUUID
	TaskID   uuid.UUID
}

// TaskEventRecord represents a task event record from the database
type TaskEventRecord struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	TaskID         uuid.UUID
	RunID          uuid.NullUUID
	EventType      string
	SequenceNumber int32
	Payload        []byte
	CreatedAt      pgtype.Timestamptz
}

// ListTasksParams represents parameters for listing tasks
type ListTasksParams struct {
	TenantID     uuid.NullUUID
	Status       pgtype.Text
	CreatorID    uuid.NullUUID
	ProviderType pgtype.Text
	Offset       int32
	Limit        int32
}

// UpdateTaskStatusParams represents parameters for updating task status
type UpdateTaskStatusParams struct {
	TenantID uuid.NullUUID
	ID       uuid.UUID
	Status   string
}

// UpdateTaskParams represents parameters for updating a task
type UpdateTaskParams struct {
	TenantID       uuid.NullUUID
	Title          pgtype.Text
	Description    pgtype.Text
	Status         pgtype.Text
	Priority       pgtype.Int4
	TargetNodeID   pgtype.Text
	AssignedNodeID pgtype.Text
	WorkspacePath  pgtype.Text
	Params         []byte
	ID             uuid.UUID
}

// CreateTaskStateHistoryParams represents parameters for creating state history
type CreateTaskStateHistoryParams struct {
	TenantID   uuid.NullUUID
	TaskID     uuid.UUID
	FromStatus pgtype.Text
	ToStatus   string
	ChangedBy  pgtype.Text
	Reason     pgtype.Text
}
