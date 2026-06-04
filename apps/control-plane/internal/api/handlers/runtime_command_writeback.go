package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/employee"
)

type RuntimeCommandWritebackService interface {
	RecordEvent(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, event employee.RuntimeCommandEventWriteback) error
	Complete(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
	Fail(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
	Cancel(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
	TimedOut(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error
}

type RuntimeCommandWritebackHandler struct {
	service RuntimeCommandWritebackService
}

func NewRuntimeCommandWritebackHandler(service RuntimeCommandWritebackService) *RuntimeCommandWritebackHandler {
	return &RuntimeCommandWritebackHandler{service: service}
}

func (h *RuntimeCommandWritebackHandler) RecordEvent(w http.ResponseWriter, r *http.Request) {
	identity, commandID, ok := runtimeCommandWritebackIdentity(w, r)
	if !ok {
		return
	}
	var event employee.RuntimeCommandEventWriteback
	if !decodeRuntimeCommandWritebackJSON(w, r, &event) {
		return
	}
	if !h.ensureService(w) {
		return
	}
	if err := h.service.RecordEvent(r.Context(), identity, commandID, event); err != nil {
		writeRuntimeCommandWritebackError(w, err)
		return
	}
	writeRuntimeCommandWritebackAccepted(w)
}

func (h *RuntimeCommandWritebackHandler) Complete(w http.ResponseWriter, r *http.Request) {
	h.handleTerminal(w, r, func(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
		return h.service.Complete(ctx, identity, commandID, terminal)
	})
}

func (h *RuntimeCommandWritebackHandler) Fail(w http.ResponseWriter, r *http.Request) {
	h.handleTerminal(w, r, func(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
		return h.service.Fail(ctx, identity, commandID, terminal)
	})
}

func (h *RuntimeCommandWritebackHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	h.handleTerminal(w, r, func(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
		return h.service.Cancel(ctx, identity, commandID, terminal)
	})
}

func (h *RuntimeCommandWritebackHandler) TimedOut(w http.ResponseWriter, r *http.Request) {
	h.handleTerminal(w, r, func(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
		return h.service.TimedOut(ctx, identity, commandID, terminal)
	})
}

func (h *RuntimeCommandWritebackHandler) handleTerminal(w http.ResponseWriter, r *http.Request, call func(context.Context, employee.RuntimeCommandWritebackIdentity, string, employee.RuntimeCommandTerminalWriteback) error) {
	identity, commandID, ok := runtimeCommandWritebackIdentity(w, r)
	if !ok {
		return
	}
	var terminal employee.RuntimeCommandTerminalWriteback
	if !decodeRuntimeCommandWritebackJSON(w, r, &terminal) {
		return
	}
	if !h.ensureService(w) {
		return
	}
	if err := call(r.Context(), identity, commandID, terminal); err != nil {
		writeRuntimeCommandWritebackError(w, err)
		return
	}
	writeRuntimeCommandWritebackAccepted(w)
}

func (h *RuntimeCommandWritebackHandler) ensureService(w http.ResponseWriter) bool {
	if h == nil || h.service == nil {
		http.Error(w, "runtime command writeback service is not configured", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func runtimeCommandWritebackIdentity(w http.ResponseWriter, r *http.Request) (employee.RuntimeCommandWritebackIdentity, string, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "tenant_id not found in context", http.StatusUnauthorized)
		return employee.RuntimeCommandWritebackIdentity{}, "", false
	}
	runtimeNodeID := middleware.GetRuntimeNodeID(r.Context())
	if runtimeNodeID == uuid.Nil {
		http.Error(w, "runtime_node_id not found in context", http.StatusUnauthorized)
		return employee.RuntimeCommandWritebackIdentity{}, "", false
	}
	nodeID := strings.TrimSpace(middleware.GetNodeID(r.Context()))
	if nodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return employee.RuntimeCommandWritebackIdentity{}, "", false
	}
	commandID := strings.TrimSpace(chi.URLParam(r, "commandId"))
	if commandID == "" {
		http.Error(w, "command_id is required", http.StatusBadRequest)
		return employee.RuntimeCommandWritebackIdentity{}, "", false
	}
	return employee.RuntimeCommandWritebackIdentity{
		TenantID:      tenantID,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        nodeID,
	}, commandID, true
}

func decodeRuntimeCommandWritebackJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

func writeRuntimeCommandWritebackError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, employee.ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, employee.ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, employee.ErrConflict):
		http.Error(w, "conflict", http.StatusConflict)
	case errors.Is(err, employee.ErrRuntimeIdentityMismatch):
		http.Error(w, "runtime identity mismatch", http.StatusForbidden)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeRuntimeCommandWritebackAccepted(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
