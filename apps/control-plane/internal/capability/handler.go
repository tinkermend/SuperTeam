package capability

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
	CreateCredential(ctx context.Context, req CreateCredentialRequest) (Credential, error)
	ListCredentials(ctx context.Context, req ListCredentialsRequest) ([]Credential, error)
	CreateTeamMCPServer(ctx context.Context, req CreateTeamMCPServerRequest) (MCPServer, error)
	ListTeamMCPServers(ctx context.Context, req TeamScopedRequest) ([]MCPServer, error)
	DeleteTeamMCPServer(ctx context.Context, req DeleteTeamMCPServerRequest) error
	CreateEmployeeMCPBinding(ctx context.Context, req CreateEmployeeMCPBindingRequest) (MCPServer, error)
	ListEmployeeMCPBindings(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
	DeleteEmployeeMCPBinding(ctx context.Context, req DeleteEmployeeMCPBindingRequest) error
	ListEffectiveMCPServers(ctx context.Context, req EmployeeScopedRequest) ([]MCPServer, error)
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

func (h *HTTPHandler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionCredentialCreate, authz.ResourceRef{Type: authz.ResourceCredential, ID: middleware.GetUserID(r.Context()).String()}, "credential create", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var body struct {
		Name            string `json:"name"`
		CredentialType  string `json:"credential_type"`
		CredentialValue string `json:"credential_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	credential, err := service.CreateCredential(r.Context(), CreateCredentialRequest{
		TenantID:        tenantID,
		UserID:          userID,
		Name:            body.Name,
		CredentialType:  CredentialType(body.CredentialType),
		CredentialValue: body.CredentialValue,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, credentialResponseFromDomain(credential))
}

func (h *HTTPHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorize(w, r, authz.ActionCredentialRead, authz.ResourceRef{Type: authz.ResourceCredential, ID: middleware.GetUserID(r.Context()).String()}, "credential read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	credentials, err := service.ListCredentials(r.Context(), ListCredentialsRequest{
		TenantID:       tenantID,
		UserID:         userID,
		CredentialType: CredentialType(r.URL.Query().Get("credential_type")),
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, credentialResponses(credentials))
}

func (h *HTTPHandler) CreateTeamMCPServer(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionTeamCapabilityManage, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp server create", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var body mcpServerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	server, err := service.CreateTeamMCPServer(r.Context(), CreateTeamMCPServerRequest{
		TenantID:     tenantID,
		TeamID:       teamID,
		UserID:       userID,
		Name:         body.Name,
		URL:          body.URL,
		CredentialID: body.CredentialID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mcpServerResponseFromDomain(server))
}

func (h *HTTPHandler) ListTeamMCPServers(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionTeamCapabilityManage, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp server read", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	servers, err := service.ListTeamMCPServers(r.Context(), TeamScopedRequest{
		TenantID: tenantID,
		UserID:   userID,
		TeamID:   teamID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpServerResponses(servers))
}

func (h *HTTPHandler) DeleteTeamMCPServer(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	serverID, ok := uuidParam(w, r, "serverId", "invalid mcp server id")
	if !ok {
		return
	}
	tenantID, _, ok := h.authorize(w, r, authz.ActionTeamCapabilityManage, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp server delete", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteTeamMCPServer(r.Context(), DeleteTeamMCPServerRequest{TenantID: tenantID, TeamID: teamID, ServerID: serverID}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) CreateEmployeeMCPBinding(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee mcp binding create", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var body mcpServerRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	server, err := service.CreateEmployeeMCPBinding(r.Context(), CreateEmployeeMCPBindingRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		UserID:            userID,
		Name:              body.Name,
		URL:               body.URL,
		CredentialID:      body.CredentialID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mcpServerResponseFromDomain(server))
}

func (h *HTTPHandler) ListEmployeeMCPBindings(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee mcp binding read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	servers, err := service.ListEmployeeMCPBindings(r.Context(), EmployeeScopedRequest{
		TenantID:          tenantID,
		UserID:            userID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpServerResponses(servers))
}

func (h *HTTPHandler) DeleteEmployeeMCPBinding(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	bindingID, ok := uuidParam(w, r, "bindingId", "invalid mcp binding id")
	if !ok {
		return
	}
	tenantID, _, ok := h.authorize(w, r, authz.ActionEmployeeCapabilityEdit, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "employee mcp binding delete", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteEmployeeMCPBinding(r.Context(), DeleteEmployeeMCPBindingRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, BindingID: bindingID}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) ListEffectiveMCPServers(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionEmployeeRead, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "effective employee mcp server read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	servers, err := service.ListEffectiveMCPServers(r.Context(), EmployeeScopedRequest{
		TenantID:          tenantID,
		UserID:            userID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpServerResponses(servers))
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "capability service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) authorize(w http.ResponseWriter, r *http.Request, action string, resource authz.ResourceRef, auditReason string, teamID *uuid.UUID) (uuid.UUID, uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "capability authorization is not configured", http.StatusForbidden)
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
		Resource:    resource,
		TenantID:    tenantID,
		TeamID:      teamID,
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

type mcpServerRequest struct {
	Name         string     `json:"name"`
	URL          string     `json:"url"`
	CredentialID *uuid.UUID `json:"credential_id"`
}

type credentialResponse struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	UserID         string         `json:"user_id"`
	Name           string         `json:"name"`
	CredentialType CredentialType `json:"credential_type"`
	LastFour       string         `json:"last_four"`
	Status         string         `json:"status"`
	DisabledAt     string         `json:"disabled_at,omitempty"`
	CreatedAt      string         `json:"created_at,omitempty"`
	UpdatedAt      string         `json:"updated_at,omitempty"`
}

type mcpServerResponse struct {
	ID                 string         `json:"id"`
	TenantID           string         `json:"tenant_id"`
	TeamID             string         `json:"team_id,omitempty"`
	DigitalEmployeeID  string         `json:"digital_employee_id,omitempty"`
	Name               string         `json:"name"`
	URL                string         `json:"url"`
	CredentialID       string         `json:"credential_id,omitempty"`
	CredentialName     string         `json:"credential_name,omitempty"`
	CredentialType     CredentialType `json:"credential_type,omitempty"`
	CredentialLastFour string         `json:"credential_last_four,omitempty"`
	Status             string         `json:"status"`
	SourceScope        string         `json:"source_scope,omitempty"`
	Inherited          bool           `json:"inherited"`
	CreatedBy          string         `json:"created_by,omitempty"`
	DisabledAt         string         `json:"disabled_at,omitempty"`
	CreatedAt          string         `json:"created_at,omitempty"`
	UpdatedAt          string         `json:"updated_at,omitempty"`
}

func credentialResponses(credentials []Credential) []credentialResponse {
	responses := make([]credentialResponse, 0, len(credentials))
	for _, item := range credentials {
		responses = append(responses, credentialResponseFromDomain(item))
	}
	return responses
}

func credentialResponseFromDomain(item Credential) credentialResponse {
	return credentialResponse{
		ID:             item.ID.String(),
		TenantID:       item.TenantID.String(),
		UserID:         item.UserID.String(),
		Name:           item.Name,
		CredentialType: item.CredentialType,
		LastFour:       item.LastFour,
		Status:         item.Status,
		DisabledAt:     formatTime(item.DisabledAt),
		CreatedAt:      formatTime(item.CreatedAt),
		UpdatedAt:      formatTime(item.UpdatedAt),
	}
}

func mcpServerResponses(servers []MCPServer) []mcpServerResponse {
	responses := make([]mcpServerResponse, 0, len(servers))
	for _, item := range servers {
		responses = append(responses, mcpServerResponseFromDomain(item))
	}
	return responses
}

func mcpServerResponseFromDomain(item MCPServer) mcpServerResponse {
	response := mcpServerResponse{
		ID:                 item.ID.String(),
		TenantID:           item.TenantID.String(),
		Name:               item.Name,
		URL:                item.URL,
		CredentialName:     item.CredentialName,
		CredentialType:     item.CredentialType,
		CredentialLastFour: item.CredentialLastFour,
		Status:             item.Status,
		SourceScope:        item.SourceScope,
		Inherited:          item.Inherited,
		DisabledAt:         formatTime(item.DisabledAt),
		CreatedAt:          formatTime(item.CreatedAt),
		UpdatedAt:          formatTime(item.UpdatedAt),
	}
	if item.TeamID != nil {
		response.TeamID = item.TeamID.String()
	}
	if item.DigitalEmployeeID != nil {
		response.DigitalEmployeeID = item.DigitalEmployeeID.String()
	}
	if item.CredentialID != nil {
		response.CredentialID = item.CredentialID.String()
	}
	if item.CreatedBy != nil {
		response.CreatedBy = item.CreatedBy.String()
	}
	return response
}

func uuidParam(w http.ResponseWriter, r *http.Request, name, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil || id == uuid.Nil {
		http.Error(w, message, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrCredentialKeyMissing), errors.Is(err, ErrCredentialTypeInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
