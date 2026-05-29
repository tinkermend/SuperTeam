package task

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// Repository defines the interface for task data access
type Repository interface {
	// Task operations
	CreateTask(ctx context.Context, params CreateTaskParams) (TaskRecord, error)
	GetTask(ctx context.Context, id int64) (TaskRecord, error)
	ListTasks(ctx context.Context, params ListTasksParams) ([]TaskRecord, error)
	UpdateTaskStatus(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error)
	UpdateTask(ctx context.Context, params UpdateTaskParams) (TaskRecord, error)
	DeleteTask(ctx context.Context, id int64) error

	// State history operations
	CreateTaskStateHistory(ctx context.Context, params CreateTaskStateHistoryParams) error
}

// CreateTaskParams represents parameters for creating a task
type CreateTaskParams struct {
	Title         string
	Description   pgtype.Text
	Status        string
	Priority      int32
	ProviderType  string
	CreatorID     pgtype.Int8
	TargetNodeID  pgtype.Text
	WorkspacePath pgtype.Text
	Params        []byte
}

// TaskRecord represents a task record from the database
type TaskRecord struct {
	ID             int64
	Title          string
	Description    pgtype.Text
	CreatorID      pgtype.Int8
	ProviderType   string
	TargetNodeID   pgtype.Text
	AssignedNodeID pgtype.Text
	Status         string
	WorkspacePath  pgtype.Text
	Params         []byte
	Priority       int32
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

// ListTasksParams represents parameters for listing tasks
type ListTasksParams struct {
	Status       pgtype.Text
	CreatorID    pgtype.Int8
	ProviderType pgtype.Text
	Offset       int32
	Limit        int32
}

// UpdateTaskStatusParams represents parameters for updating task status
type UpdateTaskStatusParams struct {
	ID     int64
	Status string
}

// UpdateTaskParams represents parameters for updating a task
type UpdateTaskParams struct {
	Title          pgtype.Text
	Description    pgtype.Text
	Status         pgtype.Text
	Priority       pgtype.Int4
	TargetNodeID   pgtype.Text
	AssignedNodeID pgtype.Text
	WorkspacePath  pgtype.Text
	Params         []byte
	ID             int64
}

// CreateTaskStateHistoryParams represents parameters for creating state history
type CreateTaskStateHistoryParams struct {
	TaskID     int64
	FromStatus pgtype.Text
	ToStatus   string
	ChangedBy  pgtype.Text
	Reason     pgtype.Text
}
