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

	created := mustRequestJSONMap(t, server, http.MethodPost, "/api/v1/tasks", map[string]any{
		"title":         "fake provider smoke",
		"provider_type": "fake",
		"params":        map[string]any{"prompt": "say hello"},
	})
	assertTaskResponseShape(t, created)

	registeredNode := mustRequestJSONMap(t, server, http.MethodPost, "/api/v1/runtime/register", map[string]any{
		"node_id":             "fake-node-1",
		"name":                "Fake Node",
		"supported_providers": []string{"fake"},
		"max_slots":           1,
	})
	assertRuntimeNodeResponseShape(t, registeredNode)

	heartbeatNode := mustRequestJSONMap(t, server, http.MethodPost, "/api/v1/runtime/heartbeat", map[string]any{
		"current_load": 1,
	})
	assertRuntimeNodeResponseShape(t, heartbeatNode)

	gotNode := mustRequestJSONMap(t, server, http.MethodGet, "/api/v1/runtime/nodes/fake-node-1", nil)
	assertRuntimeNodeResponseShape(t, gotNode)

	listedNodes := mustRequestJSONArray(t, server, http.MethodGet, "/api/v1/runtime/nodes", nil)
	if len(listedNodes) != 1 {
		t.Fatalf("expected 1 listed runtime node, got %d", len(listedNodes))
	}
	assertRuntimeNodeResponseShape(t, listedNodes[0])

	claimed := mustRequestJSONMap(t, server, http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	assertTaskResponseShape(t, claimed)
	if int64FromJSONNumber(t, claimed["id"]) != int64FromJSONNumber(t, created["id"]) {
		t.Fatalf("expected claimed task ID %v, got %v", created["id"], claimed["id"])
	}

	taskID := strconv.FormatInt(int64FromJSONNumber(t, created["id"]), 10)
	mustRequestStatus(t, server, http.MethodPost, "/api/v1/runtime/tasks/"+taskID+"/events", map[string]any{
		"events": []map[string]any{{"type": "text_delta", "text": "hello"}},
	}, http.StatusAccepted)

	completed := mustRequestJSONMap(t, server, http.MethodPost, "/api/v1/runtime/tasks/"+taskID+"/complete", map[string]any{
		"result": map[string]any{"ok": true},
	})
	assertTaskResponseShape(t, completed)
	if completed["status"] != string(task.TaskStatusCompleted) {
		t.Fatalf("expected complete response status completed, got %#v", completed["status"])
	}

	readBack := mustRequestJSONMap(t, server, http.MethodGet, "/api/v1/tasks/"+taskID, nil)
	assertTaskResponseShape(t, readBack)
	if readBack["status"] != string(task.TaskStatusCompleted) {
		t.Fatalf("expected readback status completed, got %#v", readBack["status"])
	}

	listed := mustRequestJSONArray(t, server, http.MethodGet, "/api/v1/tasks", nil)
	if len(listed) != 1 {
		t.Fatalf("expected 1 listed task, got %d", len(listed))
	}
	assertTaskResponseShape(t, listed[0])

	events := server.tasks.events[int64FromJSONNumber(t, created["id"])]
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

func mustRequestJSONMap(t *testing.T, server *e2eTestServer, method string, path string, body any) map[string]any {
	t.Helper()

	response := mustRequest(t, server, method, path, body)
	if response.Code < 200 || response.Code >= 300 {
		t.Fatalf("%s %s returned %d: %s", method, path, response.Code, response.Body.String())
	}

	var decoded map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
	return decoded
}

func mustRequestJSONArray(t *testing.T, server *e2eTestServer, method string, path string, body any) []map[string]any {
	t.Helper()

	response := mustRequest(t, server, method, path, body)
	if response.Code < 200 || response.Code >= 300 {
		t.Fatalf("%s %s returned %d: %s", method, path, response.Code, response.Body.String())
	}

	var decoded []map[string]any
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s %s response array: %v", method, path, err)
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

func assertTaskResponseShape(t *testing.T, got map[string]any) {
	t.Helper()

	for _, field := range []string{"provider_type", "created_at", "updated_at", "params"} {
		if _, ok := got[field]; !ok {
			t.Fatalf("expected task response field %q in %#v", field, got)
		}
	}
	for _, leakedField := range []string{"ProviderType", "CreatedAt", "UpdatedAt", "Params"} {
		if _, ok := got[leakedField]; ok {
			t.Fatalf("did not expect leaked Go field %q in %#v", leakedField, got)
		}
	}
	params, ok := got["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected params to be a JSON object, got %T (%#v)", got["params"], got["params"])
	}
	if params["prompt"] != "say hello" {
		t.Fatalf("expected params.prompt to be %q, got %#v", "say hello", params["prompt"])
	}
}

func assertRuntimeNodeResponseShape(t *testing.T, got map[string]any) {
	t.Helper()

	for _, field := range []string{"node_id", "supported_providers", "current_load", "created_at", "updated_at"} {
		if _, ok := got[field]; !ok {
			t.Fatalf("expected runtime node response field %q in %#v", field, got)
		}
	}
	for _, leakedField := range []string{"NodeID", "SupportedProviders", "CurrentLoad", "CreatedAt", "UpdatedAt"} {
		if _, ok := got[leakedField]; ok {
			t.Fatalf("did not expect leaked Go field %q in %#v", leakedField, got)
		}
	}
}

func int64FromJSONNumber(t *testing.T, value any) int64 {
	t.Helper()

	number, ok := value.(float64)
	if !ok {
		t.Fatalf("expected JSON number, got %T (%#v)", value, value)
	}
	return int64(number)
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
