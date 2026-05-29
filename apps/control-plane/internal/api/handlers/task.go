package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/superteam/control-plane/internal/task"
)

type TaskService interface {
	CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error)
	GetTask(ctx context.Context, taskID int64) (*task.Task, error)
	ListTasks(ctx context.Context, filter task.ListTasksFilter) ([]*task.Task, error)
	UpdateTaskStatus(ctx context.Context, req task.UpdateTaskStatusRequest) (*task.Task, error)
	CancelTask(ctx context.Context, taskID int64, cancelledBy *string, reason *string) (*task.Task, error)
}

type TaskHandler struct {
	taskService TaskService
}

func NewTaskHandler(taskService TaskService) *TaskHandler {
	return &TaskHandler{taskService: taskService}
}

func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title         string                 `json:"title"`
		Description   string                 `json:"description"`
		ProviderType  string                 `json:"provider_type"`
		TargetNodeID  string                 `json:"target_node_id"`
		WorkspacePath string                 `json:"workspace_path"`
		Params        map[string]interface{} `json:"params"`
		Priority      int32                  `json:"priority"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	paramsJSON, _ := json.Marshal(req.Params)

	task, err := h.taskService.CreateTask(r.Context(), task.CreateTaskRequest{
		Title:         req.Title,
		Description:   req.Description,
		ProviderType:  req.ProviderType,
		TargetNodeID:  req.TargetNodeID,
		WorkspacePath: req.WorkspacePath,
		Params:        paramsJSON,
		Priority:      req.Priority,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	task, err := h.taskService.GetTask(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func (h *TaskHandler) ListTasks(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	tasks, err := h.taskService.ListTasks(r.Context(), task.ListTasksFilter{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (h *TaskHandler) UpdateTaskStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	t, err := h.taskService.UpdateTaskStatus(r.Context(), task.UpdateTaskStatusRequest{
		TaskID:    id,
		NewStatus: task.TaskStatus(req.Status),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

func (h *TaskHandler) CancelTask(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid task id", http.StatusBadRequest)
		return
	}

	task, err := h.taskService.CancelTask(r.Context(), id, nil, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}
