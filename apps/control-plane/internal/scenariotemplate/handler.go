package scenariotemplate

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	List(ctx context.Context, tenantID uuid.UUID) ([]ScenarioTemplate, error)
	GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplate, error)
}

type HTTPHandler struct {
	service    HandlerService
	authorizer authz.Authorizer
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) ListScenarioTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionScenarioTemplateRead, "scenario template list")
	if !ok {
		return
	}
	templates, err := h.service.List(r.Context(), tenantID)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	responses := make([]scenarioTemplateResponse, 0, len(templates))
	for _, template := range templates {
		responses = append(responses, scenarioTemplateResponseFrom(template))
	}
	writeJSON(w, http.StatusOK, responses)
}

func (h *HTTPHandler) GetScenarioTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorize(w, r, authz.ActionScenarioTemplateRead, "scenario template get")
	if !ok {
		return
	}
	key := chi.URLParam(r, "templateKey")
	template, err := h.service.GetByKey(r.Context(), tenantID, key)
	if err != nil {
		if errors.Is(err, ErrScenarioTemplateNotFound) {
			http.Error(w, "scenario template not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, scenarioTemplateResponseFrom(template))
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string, auditReason string) (uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "scenario template authorization is not configured", http.StatusForbidden)
		return uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, false
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
		return uuid.Nil, false
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return uuid.Nil, false
	}
	return tenantID, true
}

type scenarioTemplateResponse struct {
	ID          uuid.UUID      `json:"id"`
	TenantID    uuid.UUID      `json:"tenant_id"`
	TemplateKey string         `json:"template_key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Spec        map[string]any `json:"spec"`
	Status      string         `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func scenarioTemplateResponseFrom(template ScenarioTemplate) scenarioTemplateResponse {
	spec := template.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	return scenarioTemplateResponse{
		ID:          template.ID,
		TenantID:    template.TenantID,
		TemplateKey: template.Key,
		Name:        template.Name,
		Description: template.Description,
		Spec:        spec,
		Status:      template.Status,
		CreatedAt:   template.CreatedAt,
		UpdatedAt:   template.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
