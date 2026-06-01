package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
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
	if nodeID := middleware.GetNodeID(r.Context()); nodeID != "" && nodeID != req.NodeID {
		http.Error(w, "authenticated node_id does not match request node_id", http.StatusForbidden)
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
	json.NewEncoder(w).Encode(newRuntimeNodeResponse(node))
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
	json.NewEncoder(w).Encode(newRuntimeNodeResponse(node))
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
	var candidate *task.Task
	for _, provider := range node.SupportedProviders {
		provider := provider
		tasks, err := h.taskService.ListTasks(ctx, task.ListTasksFilter{
			Status:       &pendingStatus,
			ProviderType: &provider,
			Limit:        10,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for _, t := range tasks {
			if bestClaimCandidate(candidate, t) == t {
				candidate = t
			}
		}
	}
	if candidate != nil {
		h.assignTask(ctx, w, candidate, nodeID)
		return
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
	json.NewEncoder(w).Encode(newTaskResponse(assignedTask))
}

func (h *RuntimeHandler) PushEvents(w http.ResponseWriter, r *http.Request) {
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}

	var req struct {
		Events []json.RawMessage `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	for _, rawEvent := range req.Events {
		var event struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(rawEvent, &event); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if event.Type == "" {
			http.Error(w, "event type is required", http.StatusBadRequest)
			return
		}
		if _, err := h.taskService.AppendTaskEvent(r.Context(), task.AppendTaskEventRequest{
			TaskID:    taskID,
			EventType: event.Type,
			Payload:   []byte(rawEvent),
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusAccepted)
}

func (h *RuntimeHandler) CompleteTask(w http.ResponseWriter, r *http.Request) {
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body = bytes.TrimSpace(body)
		if len(body) > 0 && !json.Valid(body) {
			err := errors.New("invalid JSON body")
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	reason := "runtime completed task"
	updatedTask, err := h.taskService.UpdateTaskStatus(r.Context(), task.UpdateTaskStatusRequest{
		TaskID:    taskID,
		NewStatus: task.TaskStatusCompleted,
		Reason:    &reason,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newTaskResponse(updatedTask))
}

func (h *RuntimeHandler) FailTask(w http.ResponseWriter, r *http.Request) {
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}

	var req struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Error == "" {
		http.Error(w, "error is required", http.StatusBadRequest)
		return
	}

	updatedTask, err := h.taskService.UpdateTaskStatus(r.Context(), task.UpdateTaskStatusRequest{
		TaskID:    taskID,
		NewStatus: task.TaskStatusFailed,
		Reason:    &req.Error,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newTaskResponse(updatedTask))
}

func (h *RuntimeHandler) RenewLease(w http.ResponseWriter, r *http.Request) {
	taskID, ok := taskIDFromRequest(w, r)
	if !ok {
		return
	}

	if _, err := h.taskService.GetTask(r.Context(), taskID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Persistent lease records are not modeled in this foundation stage.
	w.WriteHeader(http.StatusNoContent)
}

func taskIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func bestClaimCandidate(current *task.Task, candidate *task.Task) *task.Task {
	if current == nil {
		return candidate
	}
	if candidate == nil {
		return current
	}
	if candidate.Priority > current.Priority {
		return candidate
	}
	if candidate.Priority < current.Priority {
		return current
	}
	if candidate.CreatedAt.After(current.CreatedAt) {
		return candidate
	}
	return current
}

func (h *RuntimeHandler) GetNodeByID(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "id")

	node, err := h.runtimeService.GetNode(r.Context(), nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeNodeResponse(node))
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
	json.NewEncoder(w).Encode(newRuntimeNodeResponses(nodes))
}
