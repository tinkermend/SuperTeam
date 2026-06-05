package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
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

func TestClaimTaskSkipsRuntimeCommandDrivenTask(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	commandDrivenTask := &task.Task{
		ID:           handlerTestUUID(100),
		ProviderType: "codex",
		Priority:     9,
		Params:       []byte(`{"provider_run_protocol":"provider-run/v1"}`),
	}
	regularTask := &task.Task{
		ID:           handlerTestUUID(200),
		ProviderType: "codex",
		Priority:     1,
		Params:       []byte(`{"kind":"legacy-task"}`),
	}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": {commandDrivenTask, regularTask},
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
	if taskService.assignedTaskID != regularTask.ID {
		t.Fatalf("expected regular task %s to be assigned, got %s", regularTask.ID, taskService.assignedTaskID)
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

func TestClaimTaskSkipsTaskOutsideRuntimeScope(t *testing.T) {
	tenantID := handlerTestUUID(1)
	teamID := handlerTestUUID(101)
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	blockedTask := &task.Task{
		ID:           handlerTestUUID(200),
		TenantID:     tenantID,
		TeamID:       &teamID,
		ProviderType: "codex",
	}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": {blockedTask},
		},
	}
	authorizer := &claimAuthorizer{allowed: false}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
		authorizer,
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected no content when task is outside scope, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected no assignment, got %s", taskService.assignedTaskID)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authz check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Actor.Type != authz.ActorRuntimeNode {
		t.Fatalf("expected runtime node actor type, got %#v", check)
	}
	if check.Actor.ID != node.NodeID {
		t.Fatalf("expected runtime node actor id %q, got %#v", node.NodeID, check)
	}
	if check.Action != authz.ActionTaskClaim {
		t.Fatalf("expected task.claim action, got %#v", check)
	}
	if check.Resource.Type != authz.ResourceTask {
		t.Fatalf("expected task resource type, got %#v", check)
	}
	if check.Resource.ID != blockedTask.ID.String() {
		t.Fatalf("expected task resource id %s, got %#v", blockedTask.ID, check)
	}
	if check.TenantID != tenantID {
		t.Fatalf("expected tenant id %s, got %#v", tenantID, check)
	}
	if check.TeamID == nil || *check.TeamID != teamID {
		t.Fatalf("expected team id %s, got %#v", teamID, check)
	}
	if check.AuditReason != "runtime task claim" {
		t.Fatalf("expected runtime task claim audit reason, got %#v", check)
	}
}

func TestClaimTaskAssignsAllowedCandidateWhenHigherPriorityDenied(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	blockedTask := &task.Task{ID: handlerTestUUID(100), ProviderType: "codex", Priority: 9}
	allowedTask := &task.Task{ID: handlerTestUUID(200), ProviderType: "codex", Priority: 1}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": {blockedTask, allowedTask},
		},
	}
	authorizer := &claimAuthorizer{
		allowedByTaskID: map[string]bool{
			blockedTask.ID.String(): false,
			allowedTask.ID.String(): true,
		},
	}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
		authorizer,
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != allowedTask.ID {
		t.Fatalf("expected allowed task %s to be assigned, got %s", allowedTask.ID, taskService.assignedTaskID)
	}
	if len(authorizer.checks) < 2 {
		t.Fatalf("expected authz checks for at least two tasks, got %#v", authorizer.checks)
	}
	checkedTaskIDs := map[string]bool{}
	for _, check := range authorizer.checks {
		checkedTaskIDs[check.Resource.ID] = true
	}
	if !checkedTaskIDs[blockedTask.ID.String()] || !checkedTaskIDs[allowedTask.ID.String()] {
		t.Fatalf("expected checks for blocked and allowed tasks, got %#v", authorizer.checks)
	}
}

func TestClaimTaskSkipsPolledRuntimeCommandDrivenTask(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	polledTask := &task.Task{
		ID:           handlerTestUUID(200),
		ProviderType: "codex",
		Params:       []byte(`{"provider_run_protocol":"provider-run/v1"}`),
	}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": nil,
		},
	}
	authorizer := &claimAuthorizer{allowed: true}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{task: polledTask},
		authorizer,
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected no content for command-driven task, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected no assignment, got %s", taskService.assignedTaskID)
	}
	if len(authorizer.checks) != 0 {
		t.Fatalf("expected command-driven task to skip authz claim checks, got %#v", authorizer.checks)
	}
}

func TestClaimTaskSkipsPolledTaskOutsideRuntimeScope(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	polledTask := &task.Task{ID: handlerTestUUID(200), ProviderType: "codex"}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": nil,
		},
	}
	authorizer := &claimAuthorizer{allowed: false}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{task: polledTask},
		authorizer,
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("expected no content when polled task is outside scope, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected no assignment, got %s", taskService.assignedTaskID)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authz check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionTaskClaim || check.Resource.ID != polledTask.ID.String() {
		t.Fatalf("expected task.claim check for polled task %s, got %#v", polledTask.ID, check)
	}
}

func TestClaimTaskReturnsInternalServerErrorWhenAuthzFails(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	claimableTask := &task.Task{ID: handlerTestUUID(200), ProviderType: "codex"}
	taskService := &claimTaskService{
		tasksByProvider: map[string][]*task.Task{
			"codex": {claimableTask},
		},
	}
	authorizer := &claimAuthorizer{err: errors.New("authz unavailable")}
	handler := NewRuntimeHandler(
		&claimRuntimeService{node: node},
		taskService,
		&claimPoller{},
		authorizer,
	)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected no assignment, got %s", taskService.assignedTaskID)
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

func (s *claimRuntimeService) EnrollHello(ctx context.Context, req runtime.EnrollHelloRequest) (*runtime.EnrollHelloResponse, error) {
	return &runtime.EnrollHelloResponse{
		Enrollment: runtime.RuntimeEnrollment{
			ID:             handlerTestUUID(901),
			TenantID:       runtime.DefaultTenantID,
			NodeID:         req.NodeID,
			BootstrapKeyID: handlerTestUUID(902),
			Status:         runtime.RuntimeEnrollmentStatusPending,
		},
	}, nil
}

func (s *claimRuntimeService) ListRuntimeEnrollments(ctx context.Context, filter runtime.ListRuntimeEnrollmentsFilter) ([]*runtime.RuntimeEnrollment, error) {
	return nil, nil
}

func (s *claimRuntimeService) ApproveEnrollment(ctx context.Context, req runtime.ApproveEnrollmentRequest) (*runtime.RuntimeEnrollment, error) {
	return &runtime.RuntimeEnrollment{ID: req.EnrollmentID, TenantID: runtime.DefaultTenantID, Status: runtime.RuntimeEnrollmentStatusApproved}, nil
}

func (s *claimRuntimeService) RejectEnrollment(ctx context.Context, req runtime.RejectEnrollmentRequest) (*runtime.RuntimeEnrollment, error) {
	return &runtime.RuntimeEnrollment{ID: req.EnrollmentID, TenantID: runtime.DefaultTenantID, Status: runtime.RuntimeEnrollmentStatusRejected}, nil
}

func (s *claimRuntimeService) RevokeEnrollment(ctx context.Context, req runtime.RevokeEnrollmentRequest) (*runtime.RuntimeEnrollment, error) {
	return &runtime.RuntimeEnrollment{ID: req.EnrollmentID, TenantID: runtime.DefaultTenantID, Status: runtime.RuntimeEnrollmentStatusRevoked}, nil
}

func (s *claimRuntimeService) RenewRuntimeSession(ctx context.Context, token string) (*runtime.RuntimeSession, error) {
	return &runtime.RuntimeSession{ID: handlerTestUUID(903), TenantID: runtime.DefaultTenantID, RuntimeNodeID: handlerTestUUID(904)}, nil
}

func (s *claimRuntimeService) UpsertCapabilities(ctx context.Context, token string, capabilities []runtime.RuntimeCapabilityInput) ([]runtime.RuntimeCapability, error) {
	return []runtime.RuntimeCapability{}, nil
}

func (s *claimRuntimeService) GetOverview(ctx context.Context, filter runtime.RuntimeOverviewFilter) (*runtime.RuntimeOverview, error) {
	return &runtime.RuntimeOverview{}, nil
}

func (s *claimRuntimeService) ListRuntimeEvents(ctx context.Context, filter runtime.ListRuntimeEventsFilter) ([]runtime.RuntimeEvent, error) {
	return []runtime.RuntimeEvent{}, nil
}

func (s *claimRuntimeService) ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtime.RuntimeCapability, error) {
	return []runtime.RuntimeCapability{}, nil
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

type claimPoller struct {
	task *task.Task
}

func (p *claimPoller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	if p.task != nil {
		return p.task, nil
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

type claimAuthorizer struct {
	allowed         bool
	allowedByTaskID map[string]bool
	err             error
	checks          []authz.CheckRequest
}

func (a *claimAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.err != nil {
		return authz.Decision{}, a.err
	}
	if a.allowedByTaskID != nil {
		return authz.Decision{Allowed: a.allowedByTaskID[req.Resource.ID], Reason: authz.ReasonAllowed}, nil
	}
	return authz.Decision{Allowed: a.allowed, Reason: authz.ReasonAllowed}, nil
}

func runtimeRequest(method string, target string, routePattern string, taskID uuid.UUID, body []byte) *http.Request {
	request := httptest.NewRequest(method, target, bytes.NewReader(body))
	routeContext := chi.NewRouteContext()
	routeContext.Routes = chi.NewRouter()
	routeContext.RoutePatterns = []string{routePattern}
	routeContext.URLParams.Add("id", taskID.String())
	return request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
}
