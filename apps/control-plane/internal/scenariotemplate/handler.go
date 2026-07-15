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
	Create(ctx context.Context, req CreateScenarioTemplateRequest) (ScenarioTemplate, error)
	CreateVersion(ctx context.Context, req CreateScenarioTemplateVersionRequest) (ScenarioTemplate, error)
	ListVersions(ctx context.Context, tenantID uuid.UUID, key string) ([]ScenarioTemplateVersion, error)
	Patch(ctx context.Context, req PatchScenarioTemplateRequest) (ScenarioTemplate, error)
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
	tenantID, _, ok := h.authorize(w, r, authz.ActionScenarioTemplateRead, "scenario template list")
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
	tenantID, _, ok := h.authorize(w, r, authz.ActionScenarioTemplateRead, "scenario template get")
	if !ok {
		return
	}
	key := chi.URLParam(r, "templateKey")
	template, err := h.service.GetByKey(r.Context(), tenantID, key)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scenarioTemplateResponseFrom(template))
}

// CreateScenarioTemplate handles POST /scenario-templates: builds a
// template's v1 (main row + version row 1), validating the spec structure
// and capability vocabulary before writing.
func (h *HTTPHandler) CreateScenarioTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionScenarioTemplateManage, "scenario template create")
	if !ok {
		return
	}
	var body createScenarioTemplateRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	template, err := h.service.Create(r.Context(), CreateScenarioTemplateRequest{
		TenantID:    tenantID,
		ActorUserID: userID,
		Key:         body.TemplateKey,
		Name:        body.Name,
		Description: body.Description,
		Spec:        body.Spec,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, scenarioTemplateResponseFrom(template))
}

// CreateScenarioTemplateVersion handles POST /scenario-templates/{templateKey}/versions:
// bumps the template to a new spec version and mirrors it onto the main row.
func (h *HTTPHandler) CreateScenarioTemplateVersion(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionScenarioTemplateManage, "scenario template version create")
	if !ok {
		return
	}
	key := chi.URLParam(r, "templateKey")
	var body createScenarioTemplateVersionRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	template, err := h.service.CreateVersion(r.Context(), CreateScenarioTemplateVersionRequest{
		TenantID:    tenantID,
		ActorUserID: userID,
		Key:         key,
		Spec:        body.Spec,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, scenarioTemplateResponseFrom(template))
}

// ListScenarioTemplateVersions handles GET /scenario-templates/{templateKey}/versions:
// read-only version history, newest first.
func (h *HTTPHandler) ListScenarioTemplateVersions(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionScenarioTemplateRead, "scenario template version list")
	if !ok {
		return
	}
	key := chi.URLParam(r, "templateKey")
	versions, err := h.service.ListVersions(r.Context(), tenantID, key)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	responses := make([]scenarioTemplateVersionResponse, 0, len(versions))
	for _, version := range versions {
		responses = append(responses, scenarioTemplateVersionResponseFrom(version))
	}
	writeJSON(w, http.StatusOK, responses)
}

// PatchScenarioTemplate handles PATCH /scenario-templates/{templateKey}:
// partial update of status (active/disabled) and/or name/description.
func (h *HTTPHandler) PatchScenarioTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionScenarioTemplateManage, "scenario template patch")
	if !ok {
		return
	}
	key := chi.URLParam(r, "templateKey")
	var body patchScenarioTemplateRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	template, err := h.service.Patch(r.Context(), PatchScenarioTemplateRequest{
		TenantID:    tenantID,
		ActorUserID: userID,
		Key:         key,
		Status:      body.Status,
		Name:        body.Name,
		Description: body.Description,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, scenarioTemplateResponseFrom(template))
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "scenario template authorization is not configured", http.StatusForbidden)
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

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrScenarioTemplateNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

type scenarioTemplateResponse struct {
	ID            uuid.UUID      `json:"id"`
	TenantID      uuid.UUID      `json:"tenant_id"`
	TemplateKey   string         `json:"template_key"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Spec          map[string]any `json:"spec"`
	Status        string         `json:"status"`
	ActiveVersion int            `json:"active_version"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

func scenarioTemplateResponseFrom(template ScenarioTemplate) scenarioTemplateResponse {
	spec := template.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	return scenarioTemplateResponse{
		ID:            template.ID,
		TenantID:      template.TenantID,
		TemplateKey:   template.Key,
		Name:          template.Name,
		Description:   template.Description,
		Spec:          spec,
		Status:        template.Status,
		ActiveVersion: template.ActiveVersion,
		CreatedAt:     template.CreatedAt,
		UpdatedAt:     template.UpdatedAt,
	}
}

type scenarioTemplateVersionResponse struct {
	ID         uuid.UUID      `json:"id"`
	TemplateID uuid.UUID      `json:"template_id"`
	Version    int            `json:"version"`
	Spec       map[string]any `json:"spec"`
	CreatedAt  time.Time      `json:"created_at"`
}

func scenarioTemplateVersionResponseFrom(version ScenarioTemplateVersion) scenarioTemplateVersionResponse {
	spec := version.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	return scenarioTemplateVersionResponse{
		ID:         version.ID,
		TemplateID: version.TemplateID,
		Version:    version.Version,
		Spec:       spec,
		CreatedAt:  version.CreatedAt,
	}
}

type createScenarioTemplateRequestBody struct {
	TemplateKey string         `json:"template_key"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Spec        map[string]any `json:"spec"`
}

type createScenarioTemplateVersionRequestBody struct {
	Spec map[string]any `json:"spec"`
}

type patchScenarioTemplateRequestBody struct {
	Status      *string `json:"status"`
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
