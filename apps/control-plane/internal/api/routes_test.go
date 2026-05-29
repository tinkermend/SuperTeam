package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

func TestRuntimeRoutesAreRegistered(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/runtime/register", body: `{"node_id":"node-1","name":"node 1","supported_providers":["codex"],"max_slots":1}`},
		{method: http.MethodPost, path: "/api/v1/runtime/heartbeat", body: `{"current_load":0}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/claim"},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/1/events", body: `{"events":[]}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/1/complete", body: `{"result":{}}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/1/fail", body: `{"error":"failed"}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/1/lease"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes/node-1"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			server.ServeHTTP(rr, req)

			if rr.Code == http.StatusNotFound {
				t.Fatalf("expected runtime route to be registered, got 404")
			}
		})
	}
}

func TestLegacyRuntimeClaimRouteIsNotRegistered(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/claim", nil)
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected legacy runtime claim route to be removed, got %d", rr.Code)
	}
}

type routeRuntimeService struct{}

func (s *routeRuntimeService) RegisterNode(ctx context.Context, req runtime.RegisterNodeRequest) (*runtime.Node, error) {
	return &runtime.Node{NodeID: req.NodeID, Name: req.Name, SupportedProviders: req.SupportedProviders, MaxSlots: req.MaxSlots}, nil
}

func (s *routeRuntimeService) UpdateHeartbeat(ctx context.Context, req runtime.UpdateHeartbeatRequest) (*runtime.Node, error) {
	return &runtime.Node{NodeID: req.NodeID, CurrentLoad: req.CurrentLoad}, nil
}

func (s *routeRuntimeService) GetNode(ctx context.Context, nodeID string) (*runtime.Node, error) {
	return &runtime.Node{NodeID: nodeID, SupportedProviders: []string{"codex"}}, nil
}

func (s *routeRuntimeService) ListNodes(ctx context.Context, filter runtime.ListNodesFilter) ([]*runtime.Node, error) {
	return []*runtime.Node{}, nil
}

type routeTaskService struct{}

func (s *routeTaskService) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	return &task.Task{ID: 1, Title: req.Title, ProviderType: req.ProviderType}, nil
}

func (s *routeTaskService) GetTask(ctx context.Context, taskID int64) (*task.Task, error) {
	return &task.Task{ID: taskID, ProviderType: "codex"}, nil
}

func (s *routeTaskService) ListTasks(ctx context.Context, filter task.ListTasksFilter) ([]*task.Task, error) {
	return []*task.Task{}, nil
}

func (s *routeTaskService) AppendTaskEvent(ctx context.Context, req task.AppendTaskEventRequest) (*task.TaskEvent, error) {
	return &task.TaskEvent{TaskID: req.TaskID, EventType: req.EventType, Payload: req.Payload}, nil
}

func (s *routeTaskService) UpdateTaskStatus(ctx context.Context, req task.UpdateTaskStatusRequest) (*task.Task, error) {
	return &task.Task{ID: req.TaskID, Status: req.NewStatus}, nil
}

func (s *routeTaskService) CancelTask(ctx context.Context, taskID int64, cancelledBy *string, reason *string) (*task.Task, error) {
	return &task.Task{ID: taskID, Status: task.TaskStatusCancelled}, nil
}

func (s *routeTaskService) AssignTask(ctx context.Context, req task.AssignTaskRequest) (*task.Task, error) {
	return &task.Task{ID: req.TaskID, AssignedNodeID: &req.AssignedNodeID}, nil
}

type routePoller struct{}

func (p *routePoller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
