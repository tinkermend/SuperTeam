package rolevocab

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

func (h *HTTPHandler) ListRoleVocabulary(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionScenarioTemplateRead, "role vocabulary list")
	if !ok {
		return
	}
	entries, err := h.service.List(r.Context(), tenantID)
	if err != nil {
		writeError(w, err)
		return
	}
	out := make([]entryResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, entryResponseFrom(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *HTTPHandler) CreateRoleVocabulary(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionScenarioTemplateManage, "role vocabulary create")
	if !ok {
		return
	}
	var body struct {
		RoleKey     string `json:"role_key"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Status      string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry, err := h.service.Create(r.Context(), CreateRequest{
		TenantID:    tenantID,
		ActorUserID: userID,
		RoleKey:     body.RoleKey,
		Title:       body.Title,
		Description: body.Description,
		Status:      body.Status,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, entryResponseFrom(entry))
}

func (h *HTTPHandler) PatchRoleVocabulary(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionScenarioTemplateManage, "role vocabulary patch")
	if !ok {
		return
	}
	roleKey := chi.URLParam(r, "roleKey")
	var body struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	entry, err := h.service.Patch(r.Context(), PatchRequest{
		TenantID:    tenantID,
		ActorUserID: userID,
		RoleKey:     roleKey,
		Title:       body.Title,
		Description: body.Description,
		Status:      body.Status,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, entryResponseFrom(entry))
}

// GetRoleVocabularyReferences handles GET /role-vocabulary/{roleKey}/references.
func (h *HTTPHandler) GetRoleVocabularyReferences(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorize(w, r, authz.ActionScenarioTemplateRead, "role vocabulary references")
	if !ok {
		return
	}
	roleKey := chi.URLParam(r, "roleKey")
	refs, err := h.service.GetReferences(r.Context(), tenantID, roleKey)
	if err != nil {
		writeError(w, err)
		return
	}
	templates := make([]templateRefResponse, 0, len(refs.ScenarioTemplates))
	for _, t := range refs.ScenarioTemplates {
		templates = append(templates, templateRefResponse{Key: t.Key, Name: t.Name})
	}
	employees := make([]employeeRefResponse, 0, len(refs.Employees))
	for _, e := range refs.Employees {
		employees = append(employees, employeeRefResponse{ID: e.ID, Name: e.Name})
	}
	writeJSON(w, http.StatusOK, referencesResponse{
		ScenarioTemplates: templates,
		Employees:         employees,
		EmployeeCount:     refs.EmployeeCount,
		CastingCount:      refs.CastingCount,
	})
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action, auditReason string) (uuid.UUID, uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "role vocabulary authorization is not configured", http.StatusForbidden)
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

type entryResponse struct {
	ID          uuid.UUID `json:"id"`
	TenantID    uuid.UUID `json:"tenant_id"`
	RoleKey     string    `json:"role_key"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type templateRefResponse struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type employeeRefResponse struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type referencesResponse struct {
	ScenarioTemplates []templateRefResponse `json:"scenario_templates"`
	Employees         []employeeRefResponse `json:"employees"`
	EmployeeCount     int                   `json:"employee_count"`
	CastingCount      int                   `json:"casting_count"`
}

func entryResponseFrom(e Entry) entryResponse {
	return entryResponse{
		ID:          e.ID,
		TenantID:    e.TenantID,
		RoleKey:     e.RoleKey,
		Title:       e.Title,
		Description: e.Description,
		Status:      e.Status,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrConflict):
		http.Error(w, err.Error(), http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
