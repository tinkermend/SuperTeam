package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

func TestFakeRuntimeTaskLifecycle(t *testing.T) {
	server := newTestServer(t)

	created := mustRequestJSON[task.Task](t, server, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":         "fake provider smoke",
		"provider_type": "fake",
		"params":        map[string]any{"prompt": "say hello"},
	})

	mustRequestJSON[runtime.Node](t, server, http.MethodPost, "/api/v1/runtime/register", map[string]any{
		"node_id":             "fake-node-1",
		"name":                "Fake Node",
		"supported_providers": []string{"fake"},
		"max_slots":           1,
	})

	claimed := mustRequestJSON[task.Task](t, server, http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	if claimed.ID != created.ID {
		t.Fatalf("expected claimed task ID %d, got %d", created.ID, claimed.ID)
	}

	mustRequestStatus(t, server, http.MethodPost, "/api/v1/runtime/tasks/"+strconv.FormatInt(created.ID, 10)+"/events", map[string]any{
		"events": []map[string]any{{"type": "text_delta", "text": "hello"}},
	}, http.StatusAccepted)

	completed := mustRequestJSON[task.Task](t, server, http.MethodPost, "/api/v1/runtime/tasks/"+strconv.FormatInt(created.ID, 10)+"/complete", map[string]any{
		"result": map[string]any{"ok": true},
	})
	if completed.Status != task.TaskStatusCompleted {
		t.Fatalf("expected complete response status completed, got %q", completed.Status)
	}

	readBack := mustRequestJSON[task.Task](t, server, http.MethodGet, "/api/v1/tasks/"+strconv.FormatInt(created.ID, 10), nil)
	if readBack.Status != task.TaskStatusCompleted {
		t.Fatalf("expected readback status completed, got %q", readBack.Status)
	}

	events := server.tasks.events[created.ID]
	if len(events) != 1 {
		t.Fatalf("expected 1 persisted event, got %d", len(events))
	}
	if events[0].EventType != "text_delta" {
		t.Fatalf("expected persisted event type text_delta, got %q", events[0].EventType)
	}
}

type e2eTestServer struct {
	handler http.Handler
	tasks   *fakeTaskService
}

func newTestServer(t *testing.T) *e2eTestServer {
	t.Helper()

	taskService := &fakeTaskService{
		tasks:  map[int64]*task.Task{},
		events: map[int64][]task.TaskEvent{},
	}
	runtimeService := &fakeRuntimeService{nodes: map[string]*runtime.Node{}}
	server := NewServer(
		handlers.NewTaskHandler(taskService),
		handlers.NewRuntimeHandler(runtimeService, taskService, fakePoller{}),
	)

	return &e2eTestServer{
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nodeID := r.Header.Get("X-Node-ID")
			if nodeID == "" {
				nodeID = "fake-node-1"
			}
			ctx := context.WithValue(r.Context(), middleware.NodeIDKey, nodeID)
			server.ServeHTTP(w, r.WithContext(ctx))
		}),
		tasks: taskService,
	}
}

func mustRequestJSON[T any](t *testing.T, server *e2eTestServer, method string, path string, body any) T {
	t.Helper()

	response := mustRequest(t, server, method, path, body)
	if response.Code < 200 || response.Code >= 300 {
		t.Fatalf("%s %s returned %d: %s", method, path, response.Code, response.Body.String())
	}

	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
	return decoded
}

func mustRequestStatus(t *testing.T, server *e2eTestServer, method string, path string, body any, status int) {
	t.Helper()

	response := mustRequest(t, server, method, path, body)
	if response.Code != status {
		t.Fatalf("%s %s returned %d, want %d: %s", method, path, response.Code, status, response.Body.String())
	}
}

func mustRequest(t *testing.T, server *e2eTestServer, method string, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&requestBody).Encode(body); err != nil {
			t.Fatalf("encode request body: %v", err)
		}
	}

	request := httptest.NewRequest(method, path, &requestBody)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Node-ID", "fake-node-1")
	response := httptest.NewRecorder()
	server.handler.ServeHTTP(response, request)
	return response
}

type fakeTaskService struct {
	nextTaskID  int64
	nextEventID int64
	tasks       map[int64]*task.Task
	events      map[int64][]task.TaskEvent
}

func (s *fakeTaskService) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	s.nextTaskID++
	now := time.Now().UTC()
	t := &task.Task{
		ID:           s.nextTaskID,
		Title:        req.Title,
		Description:  req.Description,
		CreatorID:    req.CreatorID,
		ProviderType: req.ProviderType,
		TargetNodeID: req.TargetNodeID,
		Status:       task.TaskStatusPending,
		Params:       req.Params,
		Priority:     req.Priority,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	s.tasks[t.ID] = t
	return cloneTask(t), nil
}

func (s *fakeTaskService) GetTask(ctx context.Context, taskID int64) (*task.Task, error) {
	t, ok := s.tasks[taskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	return cloneTask(t), nil
}

func (s *fakeTaskService) ListTasks(ctx context.Context, filter task.ListTasksFilter) ([]*task.Task, error) {
	result := make([]*task.Task, 0)
	for _, t := range s.tasks {
		if filter.Status != nil && t.Status != *filter.Status {
			continue
		}
		if filter.ProviderType != nil && t.ProviderType != *filter.ProviderType {
			continue
		}
		result = append(result, cloneTask(t))
	}
	return result, nil
}

func (s *fakeTaskService) AppendTaskEvent(ctx context.Context, req task.AppendTaskEventRequest) (*task.TaskEvent, error) {
	if _, ok := s.tasks[req.TaskID]; !ok {
		return nil, errors.New("task not found")
	}
	s.nextEventID++
	event := task.TaskEvent{
		ID:             s.nextEventID,
		TaskID:         req.TaskID,
		EventType:      req.EventType,
		SequenceNumber: int32(len(s.events[req.TaskID]) + 1),
		Payload:        req.Payload,
		CreatedAt:      time.Now().UTC(),
	}
	s.events[req.TaskID] = append(s.events[req.TaskID], event)
	return &event, nil
}

func (s *fakeTaskService) UpdateTaskStatus(ctx context.Context, req task.UpdateTaskStatusRequest) (*task.Task, error) {
	t, ok := s.tasks[req.TaskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	t.Status = req.NewStatus
	t.UpdatedAt = time.Now().UTC()
	return cloneTask(t), nil
}

func (s *fakeTaskService) CancelTask(ctx context.Context, taskID int64, cancelledBy *string, reason *string) (*task.Task, error) {
	return s.UpdateTaskStatus(ctx, task.UpdateTaskStatusRequest{TaskID: taskID, NewStatus: task.TaskStatusCancelled})
}

func (s *fakeTaskService) AssignTask(ctx context.Context, req task.AssignTaskRequest) (*task.Task, error) {
	t, ok := s.tasks[req.TaskID]
	if !ok {
		return nil, errors.New("task not found")
	}
	t.AssignedNodeID = &req.AssignedNodeID
	t.Status = task.TaskStatusClaimed
	t.UpdatedAt = time.Now().UTC()
	return cloneTask(t), nil
}

type fakeRuntimeService struct {
	nextID int64
	nodes  map[string]*runtime.Node
}

func (s *fakeRuntimeService) RegisterNode(ctx context.Context, req runtime.RegisterNodeRequest) (*runtime.Node, error) {
	s.nextID++
	now := time.Now().UTC()
	node := &runtime.Node{
		ID:                 s.nextID,
		NodeID:             req.NodeID,
		Name:               req.Name,
		SupportedProviders: req.SupportedProviders,
		MaxSlots:           req.MaxSlots,
		Status:             runtime.NodeStatusOnline,
		Metadata:           req.Metadata,
		LastHeartbeatAt:    now,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	s.nodes[node.NodeID] = node
	return cloneNode(node), nil
}

func (s *fakeRuntimeService) UpdateHeartbeat(ctx context.Context, req runtime.UpdateHeartbeatRequest) (*runtime.Node, error) {
	node, ok := s.nodes[req.NodeID]
	if !ok {
		return nil, errors.New("node not found")
	}
	node.CurrentLoad = req.CurrentLoad
	node.LastHeartbeatAt = time.Now().UTC()
	node.UpdatedAt = node.LastHeartbeatAt
	return cloneNode(node), nil
}

func (s *fakeRuntimeService) GetNode(ctx context.Context, nodeID string) (*runtime.Node, error) {
	node, ok := s.nodes[nodeID]
	if !ok {
		return nil, errors.New("node not found")
	}
	return cloneNode(node), nil
}

func (s *fakeRuntimeService) ListNodes(ctx context.Context, filter runtime.ListNodesFilter) ([]*runtime.Node, error) {
	result := make([]*runtime.Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		if filter.Status != nil && node.Status != *filter.Status {
			continue
		}
		result = append(result, cloneNode(node))
	}
	return result, nil
}

type fakePoller struct{}

func (fakePoller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func cloneTask(t *task.Task) *task.Task {
	copy := *t
	return &copy
}

func cloneNode(node *runtime.Node) *runtime.Node {
	copy := *node
	copy.SupportedProviders = append([]string(nil), node.SupportedProviders...)
	return &copy
}
