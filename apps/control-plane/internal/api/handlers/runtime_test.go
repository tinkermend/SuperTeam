package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

func handlerTestUUID(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-4000-8000-%012d", n))
}

func TestPushEventsPersistsTypedEvent(t *testing.T) {
	taskID := handlerTestUUID(42)
	taskService := &claimTaskService{}
	handler := NewRuntimeHandler(&claimRuntimeService{}, taskService, &claimPoller{})

	request := runtimeRequest(http.MethodPost, "/api/v1/runtime/tasks/"+taskID.String()+"/events", "/api/v1/runtime/tasks/{id}/events", taskID, []byte(`{"events":[{"type":"text_delta","text":"hello"}]}`))
	response := httptest.NewRecorder()

	handler.PushEvents(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected status 202, got %d: %s", response.Code, response.Body.String())
	}
	if len(taskService.appendedEvents) != 1 {
		t.Fatalf("expected 1 appended event, got %d", len(taskService.appendedEvents))
	}
	event := taskService.appendedEvents[0]
	if event.TaskID != taskID {
		t.Fatalf("expected task id %s, got %s", taskID, event.TaskID)
	}
	if event.EventType != "text_delta" {
		t.Fatalf("expected event type text_delta, got %q", event.EventType)
	}
	if string(event.Payload) != `{"type":"text_delta","text":"hello"}` {
		t.Fatalf("expected whole event JSON payload, got %s", event.Payload)
	}
}

func TestCompleteTaskTransitionsToCompleted(t *testing.T) {
	taskID := handlerTestUUID(42)
	completedTask := &task.Task{
		ID:           taskID,
		Title:        "done",
		ProviderType: "codex",
		Status:       task.TaskStatusCompleted,
		CreatedAt:    time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 5, 29, 10, 1, 0, 0, time.UTC),
	}
	taskService := &claimTaskService{updatedTask: completedTask}
	handler := NewRuntimeHandler(&claimRuntimeService{}, taskService, &claimPoller{})

	request := runtimeRequest(http.MethodPost, "/api/v1/runtime/tasks/"+taskID.String()+"/complete", "/api/v1/runtime/tasks/{id}/complete", taskID, nil)
	response := httptest.NewRecorder()

	handler.CompleteTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.updatedStatus == nil || *taskService.updatedStatus != task.TaskStatusCompleted {
		t.Fatalf("expected completed status update, got %#v", taskService.updatedStatus)
	}
	if taskService.updateReason == nil || *taskService.updateReason != "runtime completed task" {
		t.Fatalf("expected runtime completed task reason, got %#v", taskService.updateReason)
	}

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected task JSON response: %v", err)
	}
	if body["id"] != taskID.String() || body["status"] != string(task.TaskStatusCompleted) {
		t.Fatalf("expected completed task %s, got %#v", taskID, body)
	}
	if _, ok := body["provider_type"]; !ok {
		t.Fatalf("expected snake_case task response, got %#v", body)
	}
	if _, ok := body["Status"]; ok {
		t.Fatalf("did not expect Go field names in task response: %#v", body)
	}
}

func TestFailTaskRejectsMissingError(t *testing.T) {
	taskID := handlerTestUUID(42)
	handler := NewRuntimeHandler(&claimRuntimeService{}, &claimTaskService{}, &claimPoller{})

	request := runtimeRequest(http.MethodPost, "/api/v1/runtime/tasks/"+taskID.String()+"/fail", "/api/v1/runtime/tasks/{id}/fail", taskID, []byte(`{}`))
	response := httptest.NewRecorder()

	handler.FailTask(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestFailTaskAcceptsValidError(t *testing.T) {
	taskID := handlerTestUUID(42)
	failedTask := &task.Task{
		ID:           taskID,
		Title:        "failed",
		ProviderType: "codex",
		Status:       task.TaskStatusFailed,
		CreatedAt:    time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC),
		UpdatedAt:    time.Date(2026, 5, 29, 10, 1, 0, 0, time.UTC),
	}
	taskService := &claimTaskService{updatedTask: failedTask}
	handler := NewRuntimeHandler(&claimRuntimeService{}, taskService, &claimPoller{})

	request := runtimeRequest(http.MethodPost, "/api/v1/runtime/tasks/"+taskID.String()+"/fail", "/api/v1/runtime/tasks/{id}/fail", taskID, []byte(`{"error":"provider exited"}`))
	response := httptest.NewRecorder()

	handler.FailTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.updatedStatus == nil || *taskService.updatedStatus != task.TaskStatusFailed {
		t.Fatalf("expected failed status update, got %#v", taskService.updatedStatus)
	}
	if taskService.updateReason == nil || *taskService.updateReason != "provider exited" {
		t.Fatalf("expected provider error reason, got %#v", taskService.updateReason)
	}
}

func TestRenewLeaseReturnsNoContentWhenTaskExists(t *testing.T) {
	taskID := handlerTestUUID(42)
	taskService := &claimTaskService{taskByID: map[uuid.UUID]*task.Task{taskID: {ID: taskID}}}
	handler := NewRuntimeHandler(&claimRuntimeService{}, taskService, &claimPoller{})

	request := runtimeRequest(http.MethodPost, "/api/v1/runtime/tasks/"+taskID.String()+"/lease", "/api/v1/runtime/tasks/{id}/lease", taskID, nil)
	response := httptest.NewRecorder()

	handler.RenewLease(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.gotTaskID != taskID {
		t.Fatalf("expected task lookup for %s, got %s", taskID, taskService.gotTaskID)
	}
}

func TestClaimTaskAssignsFirstSupportedProviderTask(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	unsupportedTask := &task.Task{ID: handlerTestUUID(100), ProviderType: "claude-code"}
	supportedTask := &task.Task{ID: handlerTestUUID(200), ProviderType: "codex"}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"":      {unsupportedTask},
			"codex": {supportedTask},
		},
	}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != supportedTask.ID {
		t.Fatalf("expected supported task %s to be assigned, got %s", supportedTask.ID, taskService.assignedTaskID)
	}
	if len(taskService.listedProviders) != 1 || taskService.listedProviders[0] != "codex" {
		t.Fatalf("expected provider-filtered list for codex, got %#v", taskService.listedProviders)
	}

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected task JSON response: %v", err)
	}
	if body["id"] != supportedTask.ID.String() {
		t.Fatalf("expected response task %s, got %#v", supportedTask.ID, body["id"])
	}
	if _, ok := body["provider_type"]; !ok {
		t.Fatalf("expected snake_case task response, got %#v", body)
	}
}

func TestClaimTaskAssignsHighestPriorityAcrossSupportedProviders(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex", "opencode"},
	}
	lowerPriorityTask := &task.Task{ID: handlerTestUUID(100), ProviderType: "codex", Priority: 1}
	higherPriorityTask := &task.Task{ID: handlerTestUUID(200), ProviderType: "opencode", Priority: 9}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex":    {lowerPriorityTask},
			"opencode": {higherPriorityTask},
		},
	}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != higherPriorityTask.ID {
		t.Fatalf("expected highest-priority task %s to be assigned, got %s", higherPriorityTask.ID, taskService.assignedTaskID)
	}
}

func TestClaimTaskTieBreaksByNewestCreatedAtAcrossSupportedProviders(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex", "opencode"},
	}
	olderTask := &task.Task{ID: handlerTestUUID(100), ProviderType: "codex", Priority: 5, CreatedAt: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	newerTask := &task.Task{ID: handlerTestUUID(200), ProviderType: "opencode", Priority: 5, CreatedAt: time.Date(2026, 5, 29, 11, 0, 0, 0, time.UTC)}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex":    {olderTask},
			"opencode": {newerTask},
		},
	}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != newerTask.ID {
		t.Fatalf("expected newest task %s to be assigned, got %s", newerTask.ID, taskService.assignedTaskID)
	}
}

type claimRuntimeService struct {
	node *runtime.Node
}

func (s *claimRuntimeService) RegisterNode(ctx context.Context, req runtime.RegisterNodeRequest) (*runtime.Node, error) {
	return nil, nil
}

func (s *claimRuntimeService) UpdateHeartbeat(ctx context.Context, req runtime.UpdateHeartbeatRequest) (*runtime.Node, error) {
	return nil, nil
}

func (s *claimRuntimeService) GetNode(ctx context.Context, nodeID string) (*runtime.Node, error) {
	return s.node, nil
}

func (s *claimRuntimeService) ListNodes(ctx context.Context, filter runtime.ListNodesFilter) ([]*runtime.Node, error) {
	return nil, nil
}

type claimTaskService struct {
	tasksByProvider map[string][]*task.Task
	listedProviders []string
	assignedTaskID  uuid.UUID
	appendedEvents  []task.AppendTaskEventRequest
	updatedStatus   *task.TaskStatus
	updateReason    *string
	updatedTask     *task.Task
	taskByID        map[uuid.UUID]*task.Task
	gotTaskID       uuid.UUID
}

func (s *claimTaskService) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	return nil, nil
}

func (s *claimTaskService) GetTask(ctx context.Context, taskID uuid.UUID) (*task.Task, error) {
	s.gotTaskID = taskID
	if s.taskByID != nil {
		return s.taskByID[taskID], nil
	}
	return &task.Task{ID: taskID}, nil
}

func (s *claimTaskService) ListTasks(ctx context.Context, filter task.ListTasksFilter) ([]*task.Task, error) {
	provider := ""
	if filter.ProviderType != nil {
		provider = *filter.ProviderType
		s.listedProviders = append(s.listedProviders, provider)
	}
	return s.tasksByProvider[provider], nil
}

func (s *claimTaskService) UpdateTaskStatus(ctx context.Context, req task.UpdateTaskStatusRequest) (*task.Task, error) {
	s.updatedStatus = &req.NewStatus
	s.updateReason = req.Reason
	if s.updatedTask != nil {
		return s.updatedTask, nil
	}
	return &task.Task{ID: req.TaskID, Status: req.NewStatus}, nil
}

func (s *claimTaskService) CancelTask(ctx context.Context, taskID uuid.UUID, cancelledBy *string, reason *string) (*task.Task, error) {
	return nil, nil
}

func (s *claimTaskService) AssignTask(ctx context.Context, req task.AssignTaskRequest) (*task.Task, error) {
	s.assignedTaskID = req.TaskID
	for _, tasks := range s.tasksByProvider {
		for _, t := range tasks {
			if t.ID == req.TaskID {
				return t, nil
			}
		}
	}
	return &task.Task{ID: req.TaskID}, nil
}

func (s *claimTaskService) AppendTaskEvent(ctx context.Context, req task.AppendTaskEventRequest) (*task.TaskEvent, error) {
	s.appendedEvents = append(s.appendedEvents, req)
	return &task.TaskEvent{
		TaskID:         req.TaskID,
		EventType:      req.EventType,
		SequenceNumber: int32(len(s.appendedEvents)),
		Payload:        req.Payload,
	}, nil
}

type claimPoller struct{}

func (p *claimPoller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func runtimeRequest(method string, target string, routePattern string, taskID uuid.UUID, body []byte) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	routeContext := chi.NewRouteContext()
	routeContext.Routes = chi.NewRouter()
	routeContext.RoutePatterns = []string{routePattern}
	routeContext.URLParams.Add("id", taskID.String())
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
