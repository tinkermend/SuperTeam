package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

func TestClaimTaskAssignsFirstSupportedProviderTask(t *testing.T) {
	node := &runtime.Node{
		NodeID:             "node-1",
		SupportedProviders: []string{"codex"},
	}
	unsupportedTask := &task.Task{ID: 100, ProviderType: "claude-code"}
	supportedTask := &task.Task{ID: 200, ProviderType: "codex"}
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

	request := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/claim?timeout=1", nil)
	ctx := context.WithValue(request.Context(), middleware.NodeIDKey, node.NodeID)
	request = request.WithContext(ctx)
	response := httptest.NewRecorder()

	handler.ClaimTask(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if taskService.assignedTaskID != supportedTask.ID {
		t.Fatalf("expected supported task %d to be assigned, got %d", supportedTask.ID, taskService.assignedTaskID)
	}
	if len(taskService.listedProviders) != 1 || taskService.listedProviders[0] != "codex" {
		t.Fatalf("expected provider-filtered list for codex, got %#v", taskService.listedProviders)
	}

	var body task.Task
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected task JSON response: %v", err)
	}
	if body.ID != supportedTask.ID {
		t.Fatalf("expected response task %d, got %d", supportedTask.ID, body.ID)
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
	assignedTaskID  int64
}

func (s *claimTaskService) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	return nil, nil
}

func (s *claimTaskService) GetTask(ctx context.Context, taskID int64) (*task.Task, error) {
	return nil, nil
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
	return nil, nil
}

func (s *claimTaskService) CancelTask(ctx context.Context, taskID int64, cancelledBy *string, reason *string) (*task.Task, error) {
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

type claimPoller struct{}

func (p *claimPoller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
