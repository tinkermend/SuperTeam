package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

type RuntimeService interface {
	RegisterNode(ctx context.Context, req runtime.RegisterNodeRequest) (*runtime.Node, error)
	UpdateHeartbeat(ctx context.Context, req runtime.UpdateHeartbeatRequest) (*runtime.Node, error)
	GetNode(ctx context.Context, nodeID string) (*runtime.Node, error)
	ListNodes(ctx context.Context, filter runtime.ListNodesFilter) ([]*runtime.Node, error)
}

type Poller interface {
	WaitForTask(ctx context.Context, nodeID string) (*task.Task, error)
}

type RuntimeHandler struct {
	runtimeService RuntimeService
	taskService    TaskService
	poller         Poller
}

func NewRuntimeHandler(runtimeService RuntimeService, taskService TaskService, poller Poller) *RuntimeHandler {
	return &RuntimeHandler{
		runtimeService: runtimeService,
		taskService:    taskService,
		poller:         poller,
	}
}

func (h *RuntimeHandler) RegisterNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID             string                 `json:"node_id"`
		Name               string                 `json:"name"`
		SupportedProviders []string               `json:"supported_providers"`
		MaxSlots           int32                  `json:"max_slots"`
		Metadata           map[string]interface{} `json:"metadata"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	node, err := h.runtimeService.RegisterNode(r.Context(), runtime.RegisterNodeRequest{
		NodeID:             req.NodeID,
		Name:               req.Name,
		SupportedProviders: req.SupportedProviders,
		MaxSlots:           req.MaxSlots,
		Metadata:           req.Metadata,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(node)
}

func (h *RuntimeHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID := middleware.GetNodeID(r.Context())
	if nodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return
	}

	var req struct {
		CurrentLoad int32  `json:"current_load"`
		Status      string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	node, err := h.runtimeService.UpdateHeartbeat(r.Context(), runtime.UpdateHeartbeatRequest{
		NodeID:      nodeID,
		CurrentLoad: req.CurrentLoad,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(node)
}

func (h *RuntimeHandler) ClaimTask(w http.ResponseWriter, r *http.Request) {
	nodeID := middleware.GetNodeID(r.Context())
	if nodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return
	}

	timeoutStr := r.URL.Query().Get("timeout")
	timeout := 30
	if timeoutStr != "" {
		if t, err := strconv.Atoi(timeoutStr); err == nil {
			timeout = t
		}
	}
	if timeout > 60 {
		timeout = 60
	}

	node, err := h.runtimeService.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	pendingStatus := task.TaskStatusPending
	for _, provider := range node.SupportedProviders {
		provider := provider
		tasks, err := h.taskService.ListTasks(ctx, task.ListTasksFilter{
			Status:       &pendingStatus,
			ProviderType: &provider,
			Limit:        1,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if len(tasks) > 0 {
			h.assignTask(ctx, w, tasks[0], nodeID)
			return
		}
	}

	t, err := h.poller.WaitForTask(ctx, nodeID)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if !node.SupportsProvider(t.ProviderType) {
		// The poller should only wake compatible nodes; reject stale or mismatched wakeups defensively.
		http.Error(w, "polled task provider is not supported by node", http.StatusInternalServerError)
		return
	}

	h.assignTask(ctx, w, t, nodeID)
}

func (h *RuntimeHandler) assignTask(ctx context.Context, w http.ResponseWriter, t *task.Task, nodeID string) {
	assignedTask, err := h.taskService.AssignTask(ctx, task.AssignTaskRequest{
		TaskID:         t.ID,
		AssignedNodeID: nodeID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assignedTask)
}

func (h *RuntimeHandler) GetNodeByID(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")

	node, err := h.runtimeService.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(node)
}

func (h *RuntimeHandler) ListNodes(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	nodes, err := h.runtimeService.ListNodes(r.Context(), runtime.ListNodesFilter{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(nodes)
}
