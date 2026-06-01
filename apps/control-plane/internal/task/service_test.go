package task

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func taskTestUUID(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", n))
}

// Mock repository for testing
type mockRepository struct {
	createTaskFunc                 func(ctx context.Context, params CreateTaskParams) (TaskRecord, error)
	getTaskFunc                    func(ctx context.Context, params GetTaskParams) (TaskRecord, error)
	listTasksFunc                  func(ctx context.Context, params ListTasksParams) ([]TaskRecord, error)
	updateTaskStatusFunc           func(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error)
	updateTaskFunc                 func(ctx context.Context, params UpdateTaskParams) (TaskRecord, error)
	deleteTaskFunc                 func(ctx context.Context, params DeleteTaskParams) error
	createTaskStateHistoryFunc     func(ctx context.Context, params CreateTaskStateHistoryParams) error
	createTaskEventFunc            func(ctx context.Context, params CreateTaskEventParams) (TaskEventRecord, error)
	getLatestTaskEventSequenceFunc func(ctx context.Context, params GetLatestTaskEventSequenceParams) (int32, error)
}

func (m *mockRepository) CreateTask(ctx context.Context, params CreateTaskParams) (TaskRecord, error) {
	if m.createTaskFunc != nil {
		return m.createTaskFunc(ctx, params)
	}
	return TaskRecord{}, errors.New("not implemented")
}

func (m *mockRepository) GetTask(ctx context.Context, params GetTaskParams) (TaskRecord, error) {
	if m.getTaskFunc != nil {
		return m.getTaskFunc(ctx, params)
	}
	return TaskRecord{}, errors.New("not implemented")
}

func (m *mockRepository) ListTasks(ctx context.Context, params ListTasksParams) ([]TaskRecord, error) {
	if m.listTasksFunc != nil {
		return m.listTasksFunc(ctx, params)
	}
	return nil, errors.New("not implemented")
}

func (m *mockRepository) UpdateTaskStatus(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error) {
	if m.updateTaskStatusFunc != nil {
		return m.updateTaskStatusFunc(ctx, params)
	}
	return TaskRecord{}, errors.New("not implemented")
}

func (m *mockRepository) UpdateTask(ctx context.Context, params UpdateTaskParams) (TaskRecord, error) {
	if m.updateTaskFunc != nil {
		return m.updateTaskFunc(ctx, params)
	}
	return TaskRecord{}, errors.New("not implemented")
}

func (m *mockRepository) DeleteTask(ctx context.Context, params DeleteTaskParams) error {
	if m.deleteTaskFunc != nil {
		return m.deleteTaskFunc(ctx, params)
	}
	return errors.New("not implemented")
}

func (m *mockRepository) CreateTaskStateHistory(ctx context.Context, params CreateTaskStateHistoryParams) error {
	if m.createTaskStateHistoryFunc != nil {
		return m.createTaskStateHistoryFunc(ctx, params)
	}
	return nil // State history is optional
}

func (m *mockRepository) CreateTaskEvent(ctx context.Context, params CreateTaskEventParams) (TaskEventRecord, error) {
	if m.createTaskEventFunc != nil {
		return m.createTaskEventFunc(ctx, params)
	}
	return TaskEventRecord{}, errors.New("not implemented")
}

func (m *mockRepository) GetLatestTaskEventSequence(ctx context.Context, params GetLatestTaskEventSequenceParams) (int32, error) {
	if m.getLatestTaskEventSequenceFunc != nil {
		return m.getLatestTaskEventSequenceFunc(ctx, params)
	}
	return 0, errors.New("not implemented")
}

// Test NewService
func TestNewServiceRequiresRepository(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected nil repository to fail")
	}
}

func TestNewServiceAcceptsRepository(t *testing.T) {
	service, err := NewService(&mockRepository{})
	if err != nil {
		t.Fatalf("expected service: %v", err)
	}
	if service == nil {
		t.Fatal("expected service")
	}
}

func TestServiceAppendTaskEvent(t *testing.T) {
	ctx := context.Background()
	taskID := taskTestUUID(42)
	payload := []byte(`{"delta":"hello"}`)

	repo := &mockRepository{
		getLatestTaskEventSequenceFunc: func(ctx context.Context, params GetLatestTaskEventSequenceParams) (int32, error) {
			if params.TaskID != taskID {
				t.Fatalf("expected latest sequence lookup for task %s, got %s", taskID, params.TaskID)
			}
			return 0, nil
		},
		createTaskEventFunc: func(ctx context.Context, params CreateTaskEventParams) (TaskEventRecord, error) {
			if params.TaskID != taskID {
				t.Fatalf("expected task id %s, got %s", taskID, params.TaskID)
			}
			if params.EventType != "text_delta" {
				t.Fatalf("expected event type text_delta, got %q", params.EventType)
			}
			if params.SequenceNumber != 1 {
				t.Fatalf("expected sequence number 1, got %d", params.SequenceNumber)
			}
			if string(params.Payload) != string(payload) {
				t.Fatalf("expected payload %s, got %s", payload, params.Payload)
			}
			return TaskEventRecord{
				TaskID:         params.TaskID,
				EventType:      params.EventType,
				SequenceNumber: params.SequenceNumber,
				Payload:        params.Payload,
				CreatedAt:      pgtype.Timestamptz{Valid: true},
			}, nil
		},
	}
	service, _ := NewService(repo)

	event, err := service.AppendTaskEvent(ctx, AppendTaskEventRequest{
		TaskID:    taskID,
		EventType: "text_delta",
		Payload:   payload,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.TaskID != taskID {
		t.Fatalf("expected task id %s, got %s", taskID, event.TaskID)
	}
	if event.EventType != "text_delta" {
		t.Fatalf("expected event type text_delta, got %q", event.EventType)
	}
	if event.SequenceNumber != 1 {
		t.Fatalf("expected sequence number 1, got %d", event.SequenceNumber)
	}
	if string(event.Payload) != string(payload) {
		t.Fatalf("expected payload %s, got %s", payload, event.Payload)
	}
}

// Test CreateTask
func TestCreateTask(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		req         CreateTaskRequest
		mockFunc    func(ctx context.Context, params CreateTaskParams) (TaskRecord, error)
		wantErr     bool
		errContains string
	}{
		{
			name: "successful creation",
			req: CreateTaskRequest{
				Title:        "Test Task",
				ProviderType: "claude-code",
				Priority:     5,
			},
			mockFunc: func(ctx context.Context, params CreateTaskParams) (TaskRecord, error) {
				return TaskRecord{
					ID:           taskTestUUID(1),
					Title:        params.Title,
					Status:       params.Status,
					ProviderType: params.ProviderType,
					Priority:     params.Priority,
					CreatedAt:    pgtype.Timestamptz{Valid: true},
					UpdatedAt:    pgtype.Timestamptz{Valid: true},
				}, nil
			},
			wantErr: false,
		},
		{
			name: "missing provider type",
			req: CreateTaskRequest{
				Title: "Test Task",
			},
			wantErr:     true,
			errContains: "provider_type is required",
		},
		{
			name: "repository error",
			req: CreateTaskRequest{
				Title:        "Test Task",
				ProviderType: "claude-code",
			},
			mockFunc: func(ctx context.Context, params CreateTaskParams) (TaskRecord, error) {
				return TaskRecord{}, errors.New("database error")
			},
			wantErr:     true,
			errContains: "failed to create task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				createTaskFunc: tt.mockFunc,
			}
			service, _ := NewService(repo)

			task, err := service.CreateTask(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if task == nil {
				t.Fatal("expected task but got nil")
			}

			if task.Status != TaskStatusPending {
				t.Errorf("expected status %s, got %s", TaskStatusPending, task.Status)
			}
		})
	}
}

// Test GetTask
func TestGetTask(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		taskID   uuid.UUID
		mockFunc func(ctx context.Context, params GetTaskParams) (TaskRecord, error)
		wantErr  bool
	}{
		{
			name:   "successful get",
			taskID: taskTestUUID(1),
			mockFunc: func(ctx context.Context, params GetTaskParams) (TaskRecord, error) {
				return TaskRecord{
					ID:           params.ID,
					Title:        "Test Task",
					Status:       string(TaskStatusPending),
					ProviderType: "claude-code",
					Priority:     5,
					CreatedAt:    pgtype.Timestamptz{Valid: true},
					UpdatedAt:    pgtype.Timestamptz{Valid: true},
				}, nil
			},
			wantErr: false,
		},
		{
			name:   "task not found",
			taskID: taskTestUUID(999),
			mockFunc: func(ctx context.Context, params GetTaskParams) (TaskRecord, error) {
				return TaskRecord{}, errors.New("not found")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getTaskFunc: tt.mockFunc,
			}
			service, _ := NewService(repo)

			task, err := service.GetTask(ctx, tt.taskID)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if task == nil {
				t.Fatal("expected task but got nil")
			}

			if task.ID != tt.taskID {
				t.Errorf("expected task ID %s, got %s", tt.taskID, task.ID)
			}
		})
	}
}

// Test ListTasks
func TestListTasks(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		filter    ListTasksFilter
		mockFunc  func(ctx context.Context, params ListTasksParams) ([]TaskRecord, error)
		wantCount int
		wantErr   bool
	}{
		{
			name: "list all tasks",
			filter: ListTasksFilter{
				Limit: 10,
			},
			mockFunc: func(ctx context.Context, params ListTasksParams) ([]TaskRecord, error) {
				return []TaskRecord{
					{ID: taskTestUUID(1), Title: "Task 1", Status: string(TaskStatusPending), ProviderType: "claude-code"},
					{ID: taskTestUUID(2), Title: "Task 2", Status: string(TaskStatusRunning), ProviderType: "claude-code"},
				}, nil
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name: "filter by status",
			filter: ListTasksFilter{
				Status: func() *TaskStatus { s := TaskStatusPending; return &s }(),
				Limit:  10,
			},
			mockFunc: func(ctx context.Context, params ListTasksParams) ([]TaskRecord, error) {
				return []TaskRecord{
					{ID: taskTestUUID(1), Title: "Task 1", Status: string(TaskStatusPending), ProviderType: "claude-code"},
				}, nil
			},
			wantCount: 1,
			wantErr:   false,
		},
		{
			name: "default limit applied",
			filter: ListTasksFilter{
				Limit: 0, // Should default to 50
			},
			mockFunc: func(ctx context.Context, params ListTasksParams) ([]TaskRecord, error) {
				if params.Limit != 50 {
					t.Errorf("expected default limit 50, got %d", params.Limit)
				}
				return []TaskRecord{}, nil
			},
			wantCount: 0,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				listTasksFunc: tt.mockFunc,
			}
			service, _ := NewService(repo)

			tasks, err := service.ListTasks(ctx, tt.filter)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(tasks) != tt.wantCount {
				t.Errorf("expected %d tasks, got %d", tt.wantCount, len(tasks))
			}
		})
	}
}

// Test UpdateTaskStatus
func TestUpdateTaskStatus(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		req         UpdateTaskStatusRequest
		currentTask TaskRecord
		mockUpdate  func(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error)
		wantErr     bool
		errContains string
	}{
		{
			name: "valid transition pending to claimed",
			req: UpdateTaskStatusRequest{
				TaskID:    taskTestUUID(1),
				NewStatus: TaskStatusClaimed,
			},
			currentTask: TaskRecord{
				ID:     taskTestUUID(1),
				Status: string(TaskStatusPending),
			},
			mockUpdate: func(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error) {
				return TaskRecord{
					ID:     params.ID,
					Status: params.Status,
				}, nil
			},
			wantErr: false,
		},
		{
			name: "valid transition claimed to running",
			req: UpdateTaskStatusRequest{
				TaskID:    taskTestUUID(1),
				NewStatus: TaskStatusRunning,
			},
			currentTask: TaskRecord{
				ID:     taskTestUUID(1),
				Status: string(TaskStatusClaimed),
			},
			mockUpdate: func(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error) {
				return TaskRecord{
					ID:     params.ID,
					Status: params.Status,
				}, nil
			},
			wantErr: false,
		},
		{
			name: "invalid transition pending to completed",
			req: UpdateTaskStatusRequest{
				TaskID:    taskTestUUID(1),
				NewStatus: TaskStatusCompleted,
			},
			currentTask: TaskRecord{
				ID:     taskTestUUID(1),
				Status: string(TaskStatusPending),
			},
			wantErr:     true,
			errContains: "invalid state transition",
		},
		{
			name: "invalid status",
			req: UpdateTaskStatusRequest{
				TaskID:    taskTestUUID(1),
				NewStatus: TaskStatus("invalid"),
			},
			currentTask: TaskRecord{
				ID:     taskTestUUID(1),
				Status: string(TaskStatusPending),
			},
			wantErr:     true,
			errContains: "invalid task status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getTaskFunc: func(ctx context.Context, params GetTaskParams) (TaskRecord, error) {
					return tt.currentTask, nil
				},
				updateTaskStatusFunc: tt.mockUpdate,
				createTaskStateHistoryFunc: func(ctx context.Context, params CreateTaskStateHistoryParams) error {
					return nil
				},
			}
			service, _ := NewService(repo)

			task, err := service.UpdateTaskStatus(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if task == nil {
				t.Fatal("expected task but got nil")
			}

			if task.Status != tt.req.NewStatus {
				t.Errorf("expected status %s, got %s", tt.req.NewStatus, task.Status)
			}
		})
	}
}

// Test CancelTask
func TestCancelTask(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		taskID      uuid.UUID
		currentTask TaskRecord
		wantErr     bool
	}{
		{
			name:   "cancel pending task",
			taskID: taskTestUUID(1),
			currentTask: TaskRecord{
				ID:     taskTestUUID(1),
				Status: string(TaskStatusPending),
			},
			wantErr: false,
		},
		{
			name:   "cancel running task",
			taskID: taskTestUUID(1),
			currentTask: TaskRecord{
				ID:     taskTestUUID(1),
				Status: string(TaskStatusRunning),
			},
			wantErr: false,
		},
		{
			name:   "cannot cancel completed task",
			taskID: taskTestUUID(1),
			currentTask: TaskRecord{
				ID:     taskTestUUID(1),
				Status: string(TaskStatusCompleted),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getTaskFunc: func(ctx context.Context, params GetTaskParams) (TaskRecord, error) {
					return tt.currentTask, nil
				},
				updateTaskStatusFunc: func(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error) {
					return TaskRecord{
						ID:          params.ID,
						Status:      params.Status,
						CancelledAt: pgtype.Timestamptz{Time: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC), Valid: params.Status == string(TaskStatusCancelled)},
					}, nil
				},
				createTaskStateHistoryFunc: func(ctx context.Context, params CreateTaskStateHistoryParams) error {
					return nil
				},
			}
			service, _ := NewService(repo)

			task, err := service.CancelTask(ctx, tt.taskID, nil, nil)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if task.Status != TaskStatusCancelled {
				t.Errorf("expected status %s, got %s", TaskStatusCancelled, task.Status)
			}
			if task.CancelledAt == nil || !task.CancelledAt.Equal(time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)) {
				t.Fatalf("expected cancelled_at to be mapped, got %#v", task.CancelledAt)
			}
		})
	}
}

// Test AssignTask
func TestAssignTask(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		req         AssignTaskRequest
		currentTask TaskRecord
		wantErr     bool
		errContains string
	}{
		{
			name: "assign pending task",
			req: AssignTaskRequest{
				TaskID:         taskTestUUID(1),
				AssignedNodeID: "node-001",
			},
			currentTask: TaskRecord{
				ID:             taskTestUUID(1),
				Status:         string(TaskStatusPending),
				AssignedNodeID: pgtype.Text{Valid: false},
			},
			wantErr: false,
		},
		{
			name: "cannot assign already assigned task",
			req: AssignTaskRequest{
				TaskID:         taskTestUUID(1),
				AssignedNodeID: "node-002",
			},
			currentTask: TaskRecord{
				ID:             taskTestUUID(1),
				Status:         string(TaskStatusPending),
				AssignedNodeID: pgtype.Text{String: "node-001", Valid: true},
			},
			wantErr:     true,
			errContains: "already assigned",
		},
		{
			name: "cannot assign completed task",
			req: AssignTaskRequest{
				TaskID:         taskTestUUID(1),
				AssignedNodeID: "node-001",
			},
			currentTask: TaskRecord{
				ID:             taskTestUUID(1),
				Status:         string(TaskStatusCompleted),
				AssignedNodeID: pgtype.Text{Valid: false},
			},
			wantErr:     true,
			errContains: "cannot be assigned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockRepository{
				getTaskFunc: func(ctx context.Context, params GetTaskParams) (TaskRecord, error) {
					return tt.currentTask, nil
				},
				updateTaskFunc: func(ctx context.Context, params UpdateTaskParams) (TaskRecord, error) {
					return TaskRecord{
						ID:             params.ID,
						Status:         params.Status.String,
						AssignedNodeID: params.AssignedNodeID,
					}, nil
				},
				createTaskStateHistoryFunc: func(ctx context.Context, params CreateTaskStateHistoryParams) error {
					return nil
				},
			}
			service, _ := NewService(repo)

			task, err := service.AssignTask(ctx, tt.req)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error to contain %q, got %q", tt.errContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if task.AssignedNodeID == nil || *task.AssignedNodeID != tt.req.AssignedNodeID {
				t.Errorf("expected assigned node %s, got %v", tt.req.AssignedNodeID, task.AssignedNodeID)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
