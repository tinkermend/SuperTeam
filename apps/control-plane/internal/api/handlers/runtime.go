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
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
	"nhooyr.io/websocket"
)

const runtimeCommandDrivenProviderRunProtocol = "provider-run/v1"

type RuntimeService interface {
	RegisterNode(ctx context.Context, req runtime.RegisterNodeRequest) (*runtime.Node, error)
	UpdateHeartbeat(ctx context.Context, req runtime.UpdateHeartbeatRequest) (*runtime.Node, error)
	GetNode(ctx context.Context, nodeID string) (*runtime.Node, error)
	ListNodes(ctx context.Context, filter runtime.ListNodesFilter) ([]*runtime.Node, error)
	EnrollHello(ctx context.Context, req runtime.EnrollHelloRequest) (*runtime.EnrollHelloResponse, error)
	ListRuntimeEnrollments(ctx context.Context, filter runtime.ListRuntimeEnrollmentsFilter) ([]*runtime.RuntimeEnrollment, error)
	ApproveEnrollment(ctx context.Context, req runtime.ApproveEnrollmentRequest) (*runtime.RuntimeEnrollment, error)
	RejectEnrollment(ctx context.Context, req runtime.RejectEnrollmentRequest) (*runtime.RuntimeEnrollment, error)
	RevokeEnrollment(ctx context.Context, req runtime.RevokeEnrollmentRequest) (*runtime.RuntimeEnrollment, error)
	RenewRuntimeSession(ctx context.Context, token string) (*runtime.RuntimeSession, error)
	UpsertCapabilities(ctx context.Context, token string, capabilities []runtime.RuntimeCapabilityInput) ([]runtime.RuntimeCapability, error)
	GetOverview(ctx context.Context, filter runtime.RuntimeOverviewFilter) (*runtime.RuntimeOverview, error)
	ListRuntimeEvents(ctx context.Context, filter runtime.ListRuntimeEventsFilter) ([]runtime.RuntimeEvent, error)
	ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtime.RuntimeCapability, error)
}

type Poller interface {
	WaitForTask(ctx context.Context, nodeID string) (*task.Task, error)
}

type RuntimeHandler struct {
	runtimeService     RuntimeService
	taskService        TaskService
	poller             Poller
	authorizer         authz.Authorizer
	connectionRegistry *runtime.ConnectionRegistry
}

func NewRuntimeHandler(runtimeService RuntimeService, taskService TaskService, poller Poller, authorizer ...authz.Authorizer) *RuntimeHandler {
	var az authz.Authorizer
	if len(authorizer) > 0 {
		az = authorizer[0]
	}
	return &RuntimeHandler{
		runtimeService: runtimeService,
		taskService:    taskService,
		poller:         poller,
		authorizer:     az,
	}
}

func (h *RuntimeHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *RuntimeHandler) SetConnectionRegistry(registry *runtime.ConnectionRegistry) {
	h.connectionRegistry = registry
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

func (h *RuntimeHandler) EnrollHello(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID             string                           `json:"node_id"`
		BootstrapKey       string                           `json:"bootstrap_key"`
		Name               string                           `json:"name"`
		Version            string                           `json:"version"`
		SupportedProviders []string                         `json:"supported_providers"`
		MaxSlots           int32                            `json:"max_slots"`
		Metadata           map[string]interface{}           `json:"metadata"`
		Capabilities       []runtime.RuntimeCapabilityInput `json:"capabilities"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.runtimeService.EnrollHello(r.Context(), runtime.EnrollHelloRequest{
		NodeID:             req.NodeID,
		BootstrapKey:       req.BootstrapKey,
		Name:               req.Name,
		Version:            req.Version,
		SupportedProviders: req.SupportedProviders,
		MaxSlots:           req.MaxSlots,
		Metadata:           req.Metadata,
		Capabilities:       req.Capabilities,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeEnrollmentHelloResponse(resp))
}

func (h *RuntimeHandler) ListRuntimeEnrollments(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime enrollment manage")
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	var status *runtime.RuntimeEnrollmentStatus
	if statusText := r.URL.Query().Get("status"); statusText != "" {
		parsed := runtime.RuntimeEnrollmentStatus(statusText)
		status = &parsed
	}

	enrollments, err := h.runtimeService.ListRuntimeEnrollments(r.Context(), runtime.ListRuntimeEnrollmentsFilter{
		TenantID: tenantID,
		Status:   status,
		Limit:    int32(limit),
		Offset:   int32(offset),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeEnrollmentResponses(enrollments))
}

func (h *RuntimeHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime overview read")
	if !ok {
		return
	}

	overview, err := h.runtimeService.GetOverview(r.Context(), runtime.RuntimeOverviewFilter{
		TenantID: tenantID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeOverviewResponse(overview))
}

func (h *RuntimeHandler) ListRuntimeEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime events read")
	if !ok {
		return
	}
	limit, offset := runtimeListPaginationFromRequest(r)
	filter := runtime.ListRuntimeEventsFilter{
		TenantID: tenantID,
		Limit:    limit,
		Offset:   offset,
	}
	if eventType := r.URL.Query().Get("event_type"); eventType != "" {
		parsed := runtime.RuntimeEventType(eventType)
		if !parsed.IsValid() {
			http.Error(w, "invalid event_type", http.StatusBadRequest)
			return
		}
		filter.EventType = &parsed
	}
	if severity := r.URL.Query().Get("severity"); severity != "" {
		parsed := runtime.RuntimeEventSeverity(severity)
		if !parsed.IsValid() {
			http.Error(w, "invalid severity", http.StatusBadRequest)
			return
		}
		filter.Severity = &parsed
	}
	if nodeID := r.URL.Query().Get("node_id"); nodeID != "" {
		filter.NodeID = &nodeID
	}
	if providerType := r.URL.Query().Get("provider_type"); providerType != "" {
		filter.ProviderType = &providerType
	}

	events, err := h.runtimeService.ListRuntimeEvents(r.Context(), filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeEventListResponse(events, limit, offset))
}

func (h *RuntimeHandler) ListRuntimeCapabilitiesForNode(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime capabilities read")
	if !ok {
		return
	}
	nodeID := chi.URLParam(r, "nodeId")
	if nodeID == "" {
		nodeID = chi.URLParam(r, "id")
	}
	if nodeID == "" {
		http.Error(w, "node id is required", http.StatusBadRequest)
		return
	}

	capabilities, err := h.runtimeService.ListRuntimeCapabilitiesForNode(r.Context(), tenantID, nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeCapabilityResponses(capabilities))
}

func (h *RuntimeHandler) ApproveEnrollment(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime enrollment approve")
	if !ok {
		return
	}
	enrollmentID, ok := enrollmentIDFromRequest(w, r)
	if !ok {
		return
	}
	enrollment, err := h.runtimeService.ApproveEnrollment(r.Context(), runtime.ApproveEnrollmentRequest{
		TenantID:     tenantID,
		EnrollmentID: enrollmentID,
		ApprovedBy:   userID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeEnrollmentResponse(enrollment))
}

func (h *RuntimeHandler) RejectEnrollment(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime enrollment reject")
	if !ok {
		return
	}
	enrollmentID, ok := enrollmentIDFromRequest(w, r)
	if !ok {
		return
	}
	reason, ok := decisionReasonFromRequest(w, r)
	if !ok {
		return
	}
	enrollment, err := h.runtimeService.RejectEnrollment(r.Context(), runtime.RejectEnrollmentRequest{
		TenantID:     tenantID,
		EnrollmentID: enrollmentID,
		RejectedBy:   userID,
		Reason:       reason,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeEnrollmentResponse(enrollment))
}

func (h *RuntimeHandler) RevokeEnrollment(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime enrollment revoke")
	if !ok {
		return
	}
	enrollmentID, ok := enrollmentIDFromRequest(w, r)
	if !ok {
		return
	}
	reason, ok := decisionReasonFromRequest(w, r)
	if !ok {
		return
	}
	enrollment, err := h.runtimeService.RevokeEnrollment(r.Context(), runtime.RevokeEnrollmentRequest{
		TenantID:     tenantID,
		EnrollmentID: enrollmentID,
		RevokedBy:    userID,
		Reason:       reason,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeEnrollmentResponse(enrollment))
}

func (h *RuntimeHandler) authorizeRuntimeEnrollmentManagement(w http.ResponseWriter, r *http.Request, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	if h.authorizer == nil {
		http.Error(w, "runtime enrollment authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   userID.String(),
		},
		Action: authz.ActionRuntimeScopeManage,
		Resource: authz.ResourceRef{
			Type: authz.ResourceTenant,
			ID:   tenantID.String(),
		},
		TenantID:    tenantID,
		AuditReason: auditReason,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

func (h *RuntimeHandler) RenewRuntimeSession(w http.ResponseWriter, r *http.Request) {
	if sessionIDText := chi.URLParam(r, "sessionId"); sessionIDText != "" {
		sessionID, err := uuid.Parse(sessionIDText)
		if err != nil {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}
		if sessionID != middleware.GetRuntimeSessionID(r.Context()) {
			http.Error(w, "authenticated session does not match path session_id", http.StatusForbidden)
			return
		}
	}
	token := middleware.GetRuntimeToken(r.Context())
	if token == "" {
		http.Error(w, "runtime session token not found in context", http.StatusUnauthorized)
		return
	}
	session, err := h.runtimeService.RenewRuntimeSession(r.Context(), token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeSessionResponse(session))
}

func (h *RuntimeHandler) UpsertCapabilities(w http.ResponseWriter, r *http.Request) {
	pathNodeID := chi.URLParam(r, "nodeId")
	if pathNodeID == "" {
		pathNodeID = chi.URLParam(r, "nodeID")
	}
	authNodeID := middleware.GetNodeID(r.Context())
	if authNodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return
	}
	if pathNodeID != "" && pathNodeID != authNodeID {
		http.Error(w, "authenticated node_id does not match path node_id", http.StatusForbidden)
		return
	}
	token := middleware.GetRuntimeToken(r.Context())
	if token == "" {
		http.Error(w, "runtime session token not found in context", http.StatusUnauthorized)
		return
	}
	capabilities, ok := capabilityInputsFromRequest(w, r)
	if !ok {
		return
	}
	result, err := h.runtimeService.UpsertCapabilities(r.Context(), token, capabilities)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newRuntimeCapabilityResponses(result))
}

func (h *RuntimeHandler) WebSocket(w http.ResponseWriter, r *http.Request) {
	if h.connectionRegistry == nil {
		http.Error(w, "runtime command registry is not configured", http.StatusServiceUnavailable)
		return
	}
	nodeID := middleware.GetNodeID(r.Context())
	if nodeID == "" {
		http.Error(w, "node_id not found in context", http.StatusUnauthorized)
		return
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusNormalClosure, "runtime websocket closed")

	connection := h.connectionRegistry.Register(nodeID)
	defer h.connectionRegistry.Unregister(nodeID, connection.ID)

	ctx := conn.CloseRead(r.Context())
	for {
		select {
		case command, ok := <-connection.Commands:
			if !ok {
				return
			}
			data, err := json.Marshal(command)
			if err != nil {
				conn.Close(websocket.StatusInternalError, "failed to encode runtime command")
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		case <-connection.Done():
			return
		case <-ctx.Done():
			return
		}
	}
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
			if isRuntimeCommandDrivenTask(t) {
				continue
			}
			allowed, err := h.runtimeCanClaim(ctx, nodeID, t)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if !allowed {
				continue
			}
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

	if isRuntimeCommandDrivenTask(t) {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	allowed, err := h.runtimeCanClaim(ctx, nodeID, t)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !allowed {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	h.assignTask(ctx, w, t, nodeID)
}

func isRuntimeCommandDrivenTask(t *task.Task) bool {
	if t == nil || len(t.Params) == 0 {
		return false
	}
	var params map[string]any
	if err := json.Unmarshal(t.Params, &params); err != nil {
		return false
	}
	protocol, ok := params["provider_run_protocol"].(string)
	return ok && protocol == runtimeCommandDrivenProviderRunProtocol
}

func (h *RuntimeHandler) runtimeCanClaim(ctx context.Context, nodeID string, t *task.Task) (bool, error) {
	if h.authorizer == nil {
		return true, nil
	}
	decision, err := h.authorizer.Check(ctx, authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorRuntimeNode,
			ID:   nodeID,
		},
		Action: authz.ActionTaskClaim,
		Resource: authz.ResourceRef{
			Type: authz.ResourceTask,
			ID:   t.ID.String(),
		},
		TenantID:    t.TenantID,
		TeamID:      t.TeamID,
		AuditReason: "runtime task claim",
	})
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
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

func enrollmentIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := chi.URLParam(r, "enrollmentId")
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid enrollment id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func decisionReasonFromRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Body == nil {
		return "", true
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", false
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		return "", true
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return "", false
	}
	return req.Reason, true
}

func runtimeListPaginationFromRequest(r *http.Request) (int32, int32) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return int32(limit), int32(offset)
}

type runtimeOverviewResponse struct {
	Summary              runtimeOverviewSummaryResponse             `json:"summary"`
	PendingEnrollments   []runtimeEnrollmentResponse                `json:"pending_enrollments"`
	Nodes                []runtimeNodeResponse                      `json:"nodes"`
	ProviderCapabilities []runtimeProviderCapabilitySummaryResponse `json:"provider_capabilities"`
	RecentEvents         []runtimeEventResponse                     `json:"recent_events"`
}

type runtimeOverviewSummaryResponse struct {
	OnlineNodes            int64 `json:"online_nodes"`
	TotalNodes             int64 `json:"total_nodes"`
	PendingEnrollments     int64 `json:"pending_enrollments"`
	ActiveProviderSessions int64 `json:"active_provider_sessions"`
	BlockedEvents          int64 `json:"blocked_events"`
}

type runtimeProviderCapabilitySummaryResponse struct {
	ProviderType   string `json:"provider_type"`
	NodeCount      int64  `json:"node_count"`
	AvailableCount int64  `json:"available_count"`
	HealthyCount   int64  `json:"healthy_count"`
	LastSeenAt     string `json:"last_seen_at,omitempty"`
}

type runtimeEventResponse struct {
	ID              string                       `json:"id"`
	TenantID        string                       `json:"tenant_id"`
	RuntimeNodeID   *string                      `json:"runtime_node_id,omitempty"`
	NodeID          string                       `json:"node_id,omitempty"`
	EventType       runtime.RuntimeEventType     `json:"event_type"`
	Severity        runtime.RuntimeEventSeverity `json:"severity"`
	Source          runtime.RuntimeEventSource   `json:"source"`
	Title           string                       `json:"title"`
	Description     string                       `json:"description,omitempty"`
	ProviderType    string                       `json:"provider_type,omitempty"`
	CorrelationType string                       `json:"correlation_type,omitempty"`
	CorrelationID   string                       `json:"correlation_id,omitempty"`
	Payload         map[string]interface{}       `json:"payload,omitempty"`
	CreatedAt       string                       `json:"created_at,omitempty"`
}

type runtimeEventListResponse struct {
	Items  []runtimeEventResponse `json:"items"`
	Limit  int32                  `json:"limit"`
	Offset int32                  `json:"offset"`
}

func newRuntimeOverviewResponse(overview *runtime.RuntimeOverview) runtimeOverviewResponse {
	if overview == nil {
		return runtimeOverviewResponse{
			PendingEnrollments:   []runtimeEnrollmentResponse{},
			Nodes:                []runtimeNodeResponse{},
			ProviderCapabilities: []runtimeProviderCapabilitySummaryResponse{},
			RecentEvents:         []runtimeEventResponse{},
		}
	}
	return runtimeOverviewResponse{
		Summary: runtimeOverviewSummaryResponse{
			OnlineNodes:            overview.Summary.OnlineNodes,
			TotalNodes:             overview.Summary.TotalNodes,
			PendingEnrollments:     overview.Summary.PendingEnrollments,
			ActiveProviderSessions: overview.Summary.ActiveProviderSessions,
			BlockedEvents:          overview.Summary.BlockedEvents,
		},
		PendingEnrollments:   newRuntimeEnrollmentResponses(overview.PendingEnrollments),
		Nodes:                newRuntimeNodeResponses(overview.Nodes),
		ProviderCapabilities: newRuntimeProviderCapabilitySummaryResponses(overview.ProviderCapabilities),
		RecentEvents:         newRuntimeEventResponses(overview.RecentEvents),
	}
}

func newRuntimeProviderCapabilitySummaryResponses(summaries []runtime.RuntimeProviderCapabilitySummary) []runtimeProviderCapabilitySummaryResponse {
	responses := make([]runtimeProviderCapabilitySummaryResponse, 0, len(summaries))
	for _, summary := range summaries {
		response := runtimeProviderCapabilitySummaryResponse{
			ProviderType:   summary.ProviderType,
			NodeCount:      summary.NodeCount,
			AvailableCount: summary.AvailableCount,
			HealthyCount:   summary.HealthyCount,
		}
		if !summary.LastSeenAt.IsZero() {
			response.LastSeenAt = summary.LastSeenAt.UTC().Format(timeRFC3339Nano)
		}
		responses = append(responses, response)
	}
	return responses
}

func newRuntimeEventListResponse(events []runtime.RuntimeEvent, limit int32, offset int32) runtimeEventListResponse {
	return runtimeEventListResponse{
		Items:  newRuntimeEventResponses(events),
		Limit:  limit,
		Offset: offset,
	}
}

func newRuntimeEventResponses(events []runtime.RuntimeEvent) []runtimeEventResponse {
	responses := make([]runtimeEventResponse, 0, len(events))
	for _, event := range events {
		responses = append(responses, newRuntimeEventResponse(event))
	}
	return responses
}

func newRuntimeEventResponse(event runtime.RuntimeEvent) runtimeEventResponse {
	response := runtimeEventResponse{
		ID:              event.ID.String(),
		TenantID:        event.TenantID.String(),
		NodeID:          event.NodeID,
		EventType:       event.EventType,
		Severity:        event.Severity,
		Source:          event.Source,
		Title:           event.Title,
		Description:     event.Description,
		ProviderType:    event.ProviderType,
		CorrelationType: event.CorrelationType,
		CorrelationID:   event.CorrelationID,
		Payload:         event.Payload,
	}
	if event.RuntimeNodeID != uuid.Nil {
		runtimeNodeID := event.RuntimeNodeID.String()
		response.RuntimeNodeID = &runtimeNodeID
	}
	if !event.CreatedAt.IsZero() {
		response.CreatedAt = event.CreatedAt.UTC().Format(timeRFC3339Nano)
	}
	return response
}

func capabilityInputsFromRequest(w http.ResponseWriter, r *http.Request) ([]runtime.RuntimeCapabilityInput, bool) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 {
		http.Error(w, "capabilities body is required", http.StatusBadRequest)
		return nil, false
	}
	var direct []runtime.RuntimeCapabilityInput
	if err := json.Unmarshal(body, &direct); err == nil {
		return direct, true
	}
	var wrapped struct {
		Capabilities []runtime.RuntimeCapabilityInput `json:"capabilities"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, false
	}
	return wrapped.Capabilities, true
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
