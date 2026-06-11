package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
)

const defaultAuditEventLimit = 50

type HandlerService interface {
	ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int) ([]*Event, error)
}

type HTTPHandler struct {
	service HandlerService
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return
	}

	resourceType := r.URL.Query().Get("resource_type")
	if resourceType != "project" {
		http.Error(w, "unsupported resource_type", http.StatusBadRequest)
		return
	}
	projectID, ok := projectIDFromQuery(w, r)
	if !ok {
		return
	}
	limit, ok := nonNegativeIntQueryParam(w, r, "limit", defaultAuditEventLimit)
	if !ok {
		return
	}
	offset, ok := nonNegativeIntQueryParam(w, r, "offset", 0)
	if !ok {
		return
	}

	events, err := service.ListProjectEvents(r.Context(), tenantID, projectID, limit, offset)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, auditEventResponses(events))
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "audit service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func projectIDFromQuery(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := r.URL.Query().Get("resource_id")
	if raw == "" {
		http.Error(w, "missing resource_id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	projectID, err := uuid.Parse(raw)
	if err != nil || projectID == uuid.Nil {
		http.Error(w, "invalid resource_id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return projectID, true
}

func nonNegativeIntQueryParam(w http.ResponseWriter, r *http.Request, name string, fallback int) (int, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		http.Error(w, "invalid "+name, http.StatusBadRequest)
		return 0, false
	}
	return parsed, true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, context.Canceled):
		http.Error(w, "request canceled", http.StatusRequestTimeout)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type auditEventResponse struct {
	ID           string         `json:"id"`
	TenantID     string         `json:"tenant_id"`
	EventType    string         `json:"event_type"`
	ActorType    string         `json:"actor_type"`
	ActorID      string         `json:"actor_id"`
	ResourceType string         `json:"resource_type"`
	ResourceID   string         `json:"resource_id"`
	Action       string         `json:"action"`
	Details      map[string]any `json:"details"`
	IPAddress    string         `json:"ip_address"`
	CreatedAt    string         `json:"created_at,omitempty"`
}

func auditEventResponses(events []*Event) []auditEventResponse {
	responses := make([]auditEventResponse, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		responses = append(responses, auditEventResponseFromDomain(event))
	}
	return responses
}

func auditEventResponseFromDomain(event *Event) auditEventResponse {
	return auditEventResponse{
		ID:           event.ID.String(),
		TenantID:     event.TenantID.String(),
		EventType:    event.EventType,
		ActorType:    event.ActorType,
		ActorID:      event.ActorID,
		ResourceType: event.ResourceType,
		ResourceID:   event.ResourceID,
		Action:       event.Action,
		Details:      cloneMap(event.Details),
		IPAddress:    event.IPAddress,
		CreatedAt:    timeString(event.CreatedAt),
	}
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func timeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
