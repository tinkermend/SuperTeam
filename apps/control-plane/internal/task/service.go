package task

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrTaskNotFound       = errors.New("task not found")
	ErrInvalidStatus      = errors.New("invalid task status")
	ErrInvalidTransition  = errors.New("invalid state transition")
	ErrTaskAlreadyAssigned = errors.New("task already assigned")
)

type Service struct {
	repository   Repository
	stateMachine *StateMachine
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("task repository is required")
	}
	return &Service{
		repository:   repository,
		stateMachine: NewStateMachine(),
	}, nil
}

// CreateTask creates a new task
func (s *Service) CreateTask(ctx context.Context, req CreateTaskRequest) (*Task, error) {
	// Set default priority if not specified
	if req.Priority == 0 {
		req.Priority = 5 // Default medium priority
	}

	// Validate provider type is not empty
	if req.ProviderType == "" {
		return nil, errors.New("provider_type is required")
	}

	// Create task with pending status
	params := CreateTaskParams{
		Title:         req.Title,
		Description:   textFromString(req.Description),
		Status:        string(TaskStatusPending),
		Priority:      req.Priority,
		ProviderType:  req.ProviderType,
		CreatorID:     int8FromInt64(req.CreatorID),
		TargetNodeID:  textFromString(req.TargetNodeID),
		WorkspacePath: textFromString(req.WorkspacePath),
		Params:        req.Params,
	}

	record, err := s.repository.CreateTask(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return s.recordToTask(record), nil
}

// GetTask retrieves a task by ID
func (s *Service) GetTask(ctx context.Context, taskID int64) (*Task, error) {
	record, err := s.repository.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}

	return s.recordToTask(record), nil
}

// ListTasks lists tasks with optional filters
func (s *Service) ListTasks(ctx context.Context, filter ListTasksFilter) ([]*Task, error) {
	// Set default limit if not specified
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100 // Max limit
	}

	params := ListTasksParams{
		Status:       s.statusToText(filter.Status),
		CreatorID:    int8FromInt64(filter.CreatorID),
		ProviderType: textFromString(filter.ProviderType),
		Offset:       filter.Offset,
		Limit:        filter.Limit,
	}

	records, err := s.repository.ListTasks(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	tasks := make([]*Task, len(records))
	for i, record := range records {
		tasks[i] = s.recordToTask(record)
	}

	return tasks, nil
}

// UpdateTaskStatus updates the status of a task with state machine validation
func (s *Service) UpdateTaskStatus(ctx context.Context, req UpdateTaskStatusRequest) (*Task, error) {
	// Get current task
	currentTask, err := s.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}

	// Validate status
	if !req.NewStatus.IsValid() {
		return nil, ErrInvalidStatus
	}

	// Validate state transition
	if err := s.stateMachine.ValidateTransition(currentTask.Status, req.NewStatus); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTransition, err)
	}

	// Update status
	params := UpdateTaskStatusParams{
		ID:     req.TaskID,
		Status: string(req.NewStatus),
	}

	record, err := s.repository.UpdateTaskStatus(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to update task status: %w", err)
	}

	// Record state history
	historyParams := CreateTaskStateHistoryParams{
		TaskID:     req.TaskID,
		FromStatus: textFromString((*string)(&currentTask.Status)),
		ToStatus:   string(req.NewStatus),
		ChangedBy:  textFromString(req.ChangedBy),
		Reason:     textFromString(req.Reason),
	}

	if err := s.repository.CreateTaskStateHistory(ctx, historyParams); err != nil {
		// Log error but don't fail the operation
		// In production, use proper logging
		_ = err
	}

	return s.recordToTask(record), nil
}

// CancelTask cancels a task
func (s *Service) CancelTask(ctx context.Context, taskID int64, cancelledBy *string, reason *string) (*Task, error) {
	req := UpdateTaskStatusRequest{
		TaskID:    taskID,
		NewStatus: TaskStatusCancelled,
		ChangedBy: cancelledBy,
		Reason:    reason,
	}

	return s.UpdateTaskStatus(ctx, req)
}

// AssignTask assigns a task to a node
func (s *Service) AssignTask(ctx context.Context, req AssignTaskRequest) (*Task, error) {
	// Get current task
	currentTask, err := s.GetTask(ctx, req.TaskID)
	if err != nil {
		return nil, err
	}

	// Check if task is already assigned
	if currentTask.AssignedNodeID != nil && *currentTask.AssignedNodeID != "" {
		return nil, ErrTaskAlreadyAssigned
	}

	// Check if task is in a state that can be assigned
	if currentTask.Status != TaskStatusPending && currentTask.Status != TaskStatusClaimed {
		return nil, fmt.Errorf("task cannot be assigned in status: %s", currentTask.Status)
	}

	// Update task with assigned node
	claimedStatus := string(TaskStatusClaimed)
	updateParams := UpdateTaskParams{
		Title:          pgtype.Text{Valid: false},
		Description:    pgtype.Text{Valid: false},
		Status:         textFromString(&claimedStatus),
		Priority:       pgtype.Int4{Valid: false},
		TargetNodeID:   pgtype.Text{Valid: false},
		AssignedNodeID: pgtype.Text{String: req.AssignedNodeID, Valid: true},
		WorkspacePath:  pgtype.Text{Valid: false},
		Params:         nil,
		ID:             req.TaskID,
	}

	record, err := s.repository.UpdateTask(ctx, updateParams)
	if err != nil {
		return nil, fmt.Errorf("failed to assign task: %w", err)
	}

	// Record state history if status changed
	if currentTask.Status != TaskStatusClaimed {
		historyParams := CreateTaskStateHistoryParams{
			TaskID:     req.TaskID,
			FromStatus: textFromString((*string)(&currentTask.Status)),
			ToStatus:   string(TaskStatusClaimed),
			ChangedBy:  textFromString(&req.AssignedNodeID),
			Reason:     textFromString(stringPtr("Task assigned to node")),
		}

		if err := s.repository.CreateTaskStateHistory(ctx, historyParams); err != nil {
			// Log error but don't fail the operation
			_ = err
		}
	}

	return s.recordToTask(record), nil
}

// Helper methods

func (s *Service) recordToTask(record TaskRecord) *Task {
	return &Task{
		ID:             record.ID,
		Title:          record.Title,
		Description:    stringFromText(record.Description),
		CreatorID:      int64FromInt8(record.CreatorID),
		ProviderType:   record.ProviderType,
		TargetNodeID:   stringFromText(record.TargetNodeID),
		AssignedNodeID: stringFromText(record.AssignedNodeID),
		Status:         TaskStatus(record.Status),
		WorkspacePath:  stringFromText(record.WorkspacePath),
		Params:         record.Params,
		Priority:       record.Priority,
		CreatedAt:      timeFromTimestamptz(record.CreatedAt),
		UpdatedAt:      timeFromTimestamptz(record.UpdatedAt),
	}
}

func (s *Service) statusToText(status *TaskStatus) pgtype.Text {
	if status == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: string(*status), Valid: true}
}

func stringPtr(s string) *string {
	return &s
}
