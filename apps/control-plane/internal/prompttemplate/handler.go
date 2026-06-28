package prompttemplate

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/superteam/control-plane/internal/api/gen"
	"github.com/superteam/control-plane/internal/auth"
)

type HandlerService interface {
	ListTemplates(ctx context.Context, authCtx *auth.CurrentUserContext) ([]PromptTemplate, error)
	CreateTemplate(ctx context.Context, input CreateTemplateInput) (PromptTemplate, error)
	ApplyTemplate(ctx context.Context, id uuid.UUID, authCtx *auth.CurrentUserContext) error
}

type AuthService interface {
	GetCurrentUserContext(ctx context.Context, sessionToken string) (*auth.CurrentUserContext, error)
}

type HTTPHandler struct {
	service     HandlerService
	authService AuthService
}

func NewHandler(service HandlerService, authService AuthService) *HTTPHandler {
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
	var req gen.CreatePromptTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	var teamID *uuid.UUID
	if req.TeamId != nil {
		id := uuid.UUID(*req.TeamId)
		teamID = &id
	}

	var variables []PromptTemplateVariable
	if req.Variables != nil {
		variables = make([]PromptTemplateVariable, len(*req.Variables))
		for i, v := range *req.Variables {
			variables[i] = PromptTemplateVariable{
				Name:        v.Name,
				Description: v.Description,
				Required:    v.Required,
			}
		}
	}

	template, err := h.service.CreateTemplate(r.Context(), CreateTemplateInput{
		TenantID:     authCtx.TenantID,
		CreatorID:    authCtx.User.ID,
		IsAdmin:      false,
		Title:        req.Title,
		Content:      req.Content,
		CategoryCode: req.CategoryCode,
		Scope:        string(req.Scope),
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

// DTO helpers

func promptTemplateResponses(templates []PromptTemplate) []gen.PromptTemplate {
	responses := make([]gen.PromptTemplate, len(templates))
	for i, t := range templates {
		responses[i] = promptTemplateResponseFromDomain(t)
	}
	return responses
}

func promptTemplateResponseFromDomain(t PromptTemplate) gen.PromptTemplate {
	vars := make([]gen.PromptTemplateVariable, len(t.Variables))
	for i, v := range t.Variables {
		vars[i] = gen.PromptTemplateVariable{
			Name:        v.Name,
			Description: v.Description,
			Required:    v.Required,
		}
	}
	var pVars *[]gen.PromptTemplateVariable
	if len(vars) > 0 {
		pVars = &vars
	} else {
		emptyVars := []gen.PromptTemplateVariable{}
		pVars = &emptyVars
	}
	var teamID *openapi_types.UUID
	if t.TeamID != nil {
		id := openapi_types.UUID(*t.TeamID)
		teamID = &id
	}
	return gen.PromptTemplate{
		Id:           openapi_types.UUID(t.ID),
		TenantId:     openapi_types.UUID(t.TenantID),
		Title:        t.Title,
		Content:      t.Content,
		CategoryCode: t.CategoryCode,
		Scope:        t.Scope,
		TeamId:       teamID,
		CreatorId:    openapi_types.UUID(t.CreatorID),
		Variables:    pVars,
		UseCount:     t.UseCount,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
	}
}
