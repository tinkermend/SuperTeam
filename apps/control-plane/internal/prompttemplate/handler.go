package prompttemplate

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/superteam/control-plane/internal/api/gen"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
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
	authorizer  authz.Authorizer
}

func NewHandler(service HandlerService, authService AuthService, authorizer ...authz.Authorizer) *HTTPHandler {
	h := &HTTPHandler{service: service, authService: authService}
	if len(authorizer) > 0 {
		h.authorizer = authorizer[0]
	}
	return h
}

func (h *HTTPHandler) ListPromptTemplates(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	authCtx, err := h.authService.GetCurrentUserContext(r.Context(), cookie.Value)
	if err != nil || authCtx == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	templates, err := h.service.ListTemplates(r.Context(), authCtx)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	
	writeJSON(w, http.StatusOK, promptTemplateResponses(templates))
}

func (h *HTTPHandler) CreatePromptTemplate(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	authCtx, err := h.authService.GetCurrentUserContext(r.Context(), cookie.Value)
	if err != nil || authCtx == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req gen.CreatePromptTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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

	isAdmin := false
	if h.authorizer != nil {
		decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
			Actor:    authz.ActorRef{Type: authz.ActorUser, ID: authCtx.User.ID.String()},
			Action:   authz.ActionManageSystemTemplates,
			Resource: authz.ResourceRef{Type: authz.ResourceTenant, ID: authCtx.TenantID.String()},
			TenantID: authCtx.TenantID,
		})
		if err == nil && decision.Allowed {
			isAdmin = true
		}
	}

	template, err := h.service.CreateTemplate(r.Context(), CreateTemplateInput{
		TenantID:     authCtx.TenantID,
		CreatorID:    authCtx.User.ID,
		IsAdmin:      isAdmin,
		Title:        req.Title,
		Content:      req.Content,
		CategoryCode: req.CategoryCode,
		Scope:        string(req.Scope),
		TeamID:       teamID,
		Variables:    variables,
	})
	if err != nil {
		log.Printf("ERROR creating template: %v", err)
		writeHandlerError(w, err)
		return
	}
	
	writeJSON(w, http.StatusCreated, promptTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) ApplyPromptTemplate(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	authCtx, err := h.authService.GetCurrentUserContext(r.Context(), cookie.Value)
	if err != nil || authCtx == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	
	idStr := chi.URLParam(r, "id")
	
	if idStr == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	
	err = h.service.ApplyTemplate(r.Context(), id, authCtx)
	if err != nil {
		writeHandlerError(w, err)
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

type ErrorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func writeHandlerError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnauthorized) {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if errors.Is(err, ErrForbidden) {
		writeError(w, http.StatusForbidden, err.Error())
		return
	}
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if errors.Is(err, ErrBadRequest) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, "internal server error")
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
