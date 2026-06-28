package prompttemplate

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/auth"
)

type HandlerService interface {
	ListTemplates(ctx context.Context, authCtx *auth.CurrentUserContext) ([]PromptTemplate, error)
	CreateTemplate(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error)
	ApplyTemplate(ctx context.Context, id uuid.UUID, authCtx *auth.CurrentUserContext) error
}

type HTTPHandler struct {
	service     HandlerService
	authService *auth.Service
}

func NewHandler(service HandlerService, authService *auth.Service) *HTTPHandler {
	return &HTTPHandler{service: service, authService: authService}
}

func (h *HTTPHandler) ListPromptTemplates(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		http.Error(w, "missing session", http.StatusUnauthorized)
		return
	}
	authCtx, err := h.authService.GetCurrentUserContext(r.Context(), cookie.Value)
	if err != nil || authCtx == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	templates, err := h.service.ListTemplates(r.Context(), authCtx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	writeJSON(w, http.StatusOK, promptTemplateResponses(templates))
}

func (h *HTTPHandler) CreatePromptTemplate(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		http.Error(w, "missing session", http.StatusUnauthorized)
		return
	}
	authCtx, err := h.authService.GetCurrentUserContext(r.Context(), cookie.Value)
	if err != nil || authCtx == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req createPromptTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	var teamID *uuid.UUID
	if req.TeamID != "" {
		parsed, err := uuid.Parse(req.TeamID)
		if err != nil {
			http.Error(w, "invalid team_id", http.StatusBadRequest)
			return
		}
		teamID = &parsed
	}

	variables := make([]PromptTemplateVariable, len(req.Variables))
	for i, v := range req.Variables {
		variables[i] = PromptTemplateVariable{
			Name:        v.Name,
			Description: v.Description,
			Required:    v.Required,
		}
	}

	template, err := h.service.CreateTemplate(r.Context(), CreateTemplateInput{
		TenantID:     authCtx.TenantID,
		CreatorID:    authCtx.User.ID,
		IsAdmin:      false,
		Title:        req.Title,
		Content:      req.Content,
		CategoryCode: req.CategoryCode,
		Scope:        req.Scope,
		TeamID:       teamID,
		Variables:    variables,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	writeJSON(w, http.StatusCreated, promptTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) ApplyPromptTemplate(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		http.Error(w, "missing session", http.StatusUnauthorized)
		return
	}
	authCtx, err := h.authService.GetCurrentUserContext(r.Context(), cookie.Value)
	if err != nil || authCtx == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	
	idStr := chi.URLParam(r, "id")
	
	if idStr == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	
	err = h.service.ApplyTemplate(r.Context(), id, authCtx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// DTOs

type createPromptTemplateRequest struct {
	Title        string                       `json:"title"`
	Content      string                       `json:"content"`
	CategoryCode string                       `json:"category_code"`
	Scope        string                       `json:"scope"`
	TeamID       string                       `json:"team_id,omitempty"`
	Variables    []promptTemplateVariableDTO  `json:"variables,omitempty"`
}

type promptTemplateVariableDTO struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type promptTemplateResponse struct {
	ID           string                       `json:"id"`
	TenantID     string                       `json:"tenant_id"`
	Title        string                       `json:"title"`
	Content      string                       `json:"content"`
	CategoryCode string                       `json:"category_code"`
	Scope        string                       `json:"scope"`
	TeamID       string                       `json:"team_id,omitempty"`
	CreatorID    string                       `json:"creator_id"`
	Variables    []promptTemplateVariableDTO  `json:"variables"`
	UseCount     int32                        `json:"use_count"`
	CreatedAt    string                       `json:"created_at"`
	UpdatedAt    string                       `json:"updated_at"`
}

func promptTemplateResponses(templates []PromptTemplate) []promptTemplateResponse {
	responses := make([]promptTemplateResponse, len(templates))
	for i, t := range templates {
		responses[i] = promptTemplateResponseFromDomain(t)
	}
	return responses
}

func promptTemplateResponseFromDomain(t PromptTemplate) promptTemplateResponse {
	vars := make([]promptTemplateVariableDTO, len(t.Variables))
	for i, v := range t.Variables {
		vars[i] = promptTemplateVariableDTO{
			Name:        v.Name,
			Description: v.Description,
			Required:    v.Required,
		}
	}
	var teamID string
	if t.TeamID != nil {
		teamID = t.TeamID.String()
	}
	return promptTemplateResponse{
		ID:           t.ID.String(),
		TenantID:     t.TenantID.String(),
		Title:        t.Title,
		Content:      t.Content,
		CategoryCode: t.CategoryCode,
		Scope:        t.Scope,
		TeamID:       teamID,
		CreatorID:    t.CreatorID.String(),
		Variables:    vars,
		UseCount:     t.UseCount,
		CreatedAt:    t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    t.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
