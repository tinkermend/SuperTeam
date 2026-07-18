package systemconfig

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HTTPHandler struct {
	service    *Service
	authorizer authz.Authorizer
}

func NewHandler(service *Service) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) ListSystemConfigs(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionSystemConfigRead, "system config read")
	if !ok {
		return
	}
	items, err := h.service.List(r.Context(), tenantID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponse{Items: itemResponses(items)})
}

func (h *HTTPHandler) UpdateSystemConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionSystemConfigManage, "system config update")
	if !ok {
		return
	}
	key := chi.URLParam(r, "configKey")
	var body struct {
		Value *int64 `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Value == nil {
		http.Error(w, "value must be an integer", http.StatusBadRequest)
		return
	}
	item, err := h.service.Set(r.Context(), tenantID, key, *body.Value, userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemResponse(item))
}

func (h *HTTPHandler) ResetSystemConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionSystemConfigManage, "system config reset")
	if !ok {
		return
	}
	key := chi.URLParam(r, "configKey")
	item, err := h.service.Reset(r.Context(), tenantID, key, userID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, itemResponse(item))
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "system config service is not configured", http.StatusServiceUnavailable)
		return uuid.Nil, uuid.Nil, false
	}
	if h.authorizer == nil {
		http.Error(w, "system config authorization is not configured", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor:       authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
		Action:      action,
		Resource:    authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()},
		TenantID:    tenantID,
		AuditReason: auditReason,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

type listResponse struct {
	Items []configItemResponse `json:"items"`
}

type configItemResponse struct {
	Key            string     `json:"key"`
	Domain         string     `json:"domain"`
	Label          string     `json:"label"`
	Description    string     `json:"description"`
	ValueType      string     `json:"value_type"`
	DefaultValue   int64      `json:"default_value"`
	EffectiveValue int64      `json:"effective_value"`
	IsOverridden   bool       `json:"is_overridden"`
	MinValue       int64      `json:"min_value"`
	MaxValue       int64      `json:"max_value"`
	UpdatedAt      *time.Time `json:"updated_at,omitempty"`
	UpdatedByName  string     `json:"updated_by_name,omitempty"`
}

func itemResponse(item EffectiveConfig) configItemResponse {
	return configItemResponse{
		Key:            item.Key,
		Domain:         item.Domain,
		Label:          item.Label,
		Description:    item.Description,
		ValueType:      item.ValueType,
		DefaultValue:   item.DefaultValue,
		EffectiveValue: item.EffectiveValue,
		IsOverridden:   item.IsOverridden,
		MinValue:       item.MinValue,
		MaxValue:       item.MaxValue,
		UpdatedAt:      item.UpdatedAt,
		UpdatedByName:  item.UpdatedByName,
	}
}

func itemResponses(items []EffectiveConfig) []configItemResponse {
	out := make([]configItemResponse, 0, len(items))
	for _, item := range items {
		out = append(out, itemResponse(item))
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnknownKey):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrInvalidValue):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
