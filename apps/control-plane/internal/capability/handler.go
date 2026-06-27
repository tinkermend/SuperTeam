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

	CreateMCPServerDefinition(ctx context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error)
	ListMCPServerDefinitions(ctx context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error)
	DeleteMCPServerDefinition(ctx context.Context, req DeleteMCPServerDefinitionRequest) error
	CreateTeamMCPBinding(ctx context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error)
	CreateEmployeeMCPBindingV2(ctx context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error)
	ListEffectiveMCPConfig(ctx context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error)
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

// ----------------------------------------------------------------------------
// MCP HTTP capability registry (migration 037)
// ----------------------------------------------------------------------------

func (h *HTTPHandler) ListMCPServerDefinitions(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryRead, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "mcp registry read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	definitions, err := service.ListMCPServerDefinitions(r.Context(), ListMCPServerDefinitionsRequest{TenantID: tenantID, UserID: userID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpDefinitionResponses(definitions))
}

func (h *HTTPHandler) CreateMCPServerDefinition(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryManage, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "mcp registry create", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var body mcpDefinitionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	definition, err := service.CreateMCPServerDefinition(r.Context(), CreateMCPServerDefinitionRequest{
		TenantID:           tenantID,
		UserID:             userID,
		Name:               body.Name,
		ServerKey:          body.ServerKey,
		Description:        body.Description,
		Transport:          MCPTransport(body.Transport),
		URL:                body.URL,
		AuthStrategy:       MCPAuthStrategy(body.AuthStrategy),
		RequiredEnvVars:    body.RequiredEnvVars,
		OptionalEnvVars:    body.OptionalEnvVars,
		ProviderVisibility: body.ProviderVisibility,
		ToolAllowlist:      body.ToolAllowlist,
		RiskLevel:          body.RiskLevel,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mcpDefinitionResponseFromDomain(definition))
}

func (h *HTTPHandler) DeleteMCPServerDefinition(w http.ResponseWriter, r *http.Request) {
	serverID, ok := uuidParam(w, r, "serverId", "invalid mcp server id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, _, ok = h.authorize(w, r, authz.ActionMCPRegistryManage, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "mcp registry delete", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteMCPServerDefinition(r.Context(), DeleteMCPServerDefinitionRequest{TenantID: tenantID, ServerID: serverID}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) CreateTeamMCPBinding(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionTeamCapabilityManage, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp binding create", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var body mcpBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	binding, err := service.CreateTeamMCPBinding(r.Context(), CreateTeamMCPBindingRequest{
		TenantID:         tenantID,
		TeamID:           teamID,
		UserID:           userID,
		MCPServerID:      body.MCPServerID,
		CredentialEnvVar: body.CredentialEnvVar,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mcpBindingResponseFromDomain(binding))
}

func (h *HTTPHandler) CreateEmployeeMCPBindingV2(w http.ResponseWriter, r *http.Request) {
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
	var body mcpBindingRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	binding, err := service.CreateEmployeeMCPBindingV2(r.Context(), CreateEmployeeMCPBindingV2Request{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		UserID:            userID,
		MCPServerID:       body.MCPServerID,
		CredentialEnvVar:  body.CredentialEnvVar,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, mcpBindingResponseFromDomain(binding))
}

func (h *HTTPHandler) ListEffectiveMCPConfig(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionEmployeeRead, authz.ResourceRef{Type: authz.ResourceEmployee, ID: employeeID.String()}, "effective employee mcp config read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	servers, err := service.ListEffectiveMCPConfig(r.Context(), EmployeeScopedRequest{
		TenantID:          tenantID,
		UserID:            userID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, effectiveMCPServerResponses(servers))
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

type mcpDefinitionRequest struct {
	Name               string          `json:"name"`
	ServerKey          string          `json:"server_key"`
	Description        string          `json:"description"`
	Transport          string          `json:"transport"`
	URL                string          `json:"url"`
	AuthStrategy       string          `json:"auth_strategy"`
	RequiredEnvVars    []string        `json:"required_env_vars"`
	OptionalEnvVars    []string        `json:"optional_env_vars"`
	ProviderVisibility map[string]bool `json:"provider_visibility"`
	ToolAllowlist      []string        `json:"tool_allowlist"`
	RiskLevel          string          `json:"risk_level"`
}

type mcpBindingRequest struct {
	MCPServerID      uuid.UUID `json:"mcp_server_id"`
	CredentialEnvVar string    `json:"credential_env_var"`
}

type mcpDefinitionResponse struct {
	ID                 string          `json:"id"`
	TenantID           string          `json:"tenant_id"`
	Name               string          `json:"name"`
	ServerKey          string          `json:"server_key"`
	Description        string          `json:"description"`
	Transport          string          `json:"transport"`
	URL                string          `json:"url"`
	AuthStrategy       string          `json:"auth_strategy"`
	RequiredEnvVars    []string        `json:"required_env_vars"`
	OptionalEnvVars    []string        `json:"optional_env_vars"`
	ProviderVisibility map[string]bool `json:"provider_visibility,omitempty"`
	ToolAllowlist      []string        `json:"tool_allowlist"`
	RiskLevel          string          `json:"risk_level"`
	Status             string          `json:"status"`
	CreatedAt          string          `json:"created_at,omitempty"`
	UpdatedAt          string          `json:"updated_at,omitempty"`
}

type mcpBindingResponse struct {
	ID                string   `json:"id"`
	TenantID          string   `json:"tenant_id"`
	TeamID            string   `json:"team_id,omitempty"`
	DigitalEmployeeID string   `json:"digital_employee_id,omitempty"`
	MCPServerID       string   `json:"mcp_server_id"`
	ServerKey         string   `json:"server_key,omitempty"`
	ServerName        string   `json:"server_name,omitempty"`
	URL               string   `json:"url,omitempty"`
	Transport         string   `json:"transport,omitempty"`
	AuthStrategy      string   `json:"auth_strategy,omitempty"`
	CredentialEnvVar  string   `json:"credential_env_var,omitempty"`
	RequiredEnvVars   []string `json:"required_env_vars,omitempty"`
	MissingEnvVars    []string `json:"missing_env_vars,omitempty"`
	SourceScope       string   `json:"source_scope,omitempty"`
	Status            string   `json:"status"`
	RiskLevel         string   `json:"risk_level,omitempty"`
	CreatedAt         string   `json:"created_at,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
}

type effectiveMCPServerResponse struct {
	ServerID         string   `json:"server_id"`
	ServerKey        string   `json:"server_key"`
	Name             string   `json:"name"`
	Transport        string   `json:"transport"`
	URL              string   `json:"url"`
	AuthStrategy     string   `json:"auth_strategy"`
	CredentialEnvVar string   `json:"credential_env_var,omitempty"`
	RequiredEnvVars  []string `json:"required_env_vars,omitempty"`
	MissingEnvVars   []string `json:"missing_env_vars,omitempty"`
	ToolAllowlist    []string `json:"tool_allowlist,omitempty"`
	RiskLevel        string   `json:"risk_level,omitempty"`
	SourceScope      string   `json:"source_scope"`
	Status           string   `json:"status"`
}

func mcpDefinitionResponses(definitions []MCPDefinition) []mcpDefinitionResponse {
	responses := make([]mcpDefinitionResponse, 0, len(definitions))
	for _, item := range definitions {
		responses = append(responses, mcpDefinitionResponseFromDomain(item))
	}
	return responses
}

func mcpDefinitionResponseFromDomain(item MCPDefinition) mcpDefinitionResponse {
	return mcpDefinitionResponse{
		ID:                 item.ID.String(),
		TenantID:           item.TenantID.String(),
		Name:               item.Name,
		ServerKey:          item.ServerKey,
		Description:        item.Description,
		Transport:          string(item.Transport),
		URL:                item.URL,
		AuthStrategy:       string(item.AuthStrategy),
		RequiredEnvVars:    nonNilStrings(item.RequiredEnvVars),
		OptionalEnvVars:    nonNilStrings(item.OptionalEnvVars),
		ProviderVisibility: item.ProviderVisibility,
		ToolAllowlist:      nonNilStrings(item.ToolAllowlist),
		RiskLevel:          item.RiskLevel,
		Status:             item.Status,
		CreatedAt:          formatTime(item.CreatedAt),
		UpdatedAt:          formatTime(item.UpdatedAt),
	}
}

func mcpBindingResponseFromDomain(item MCPBinding) mcpBindingResponse {
	response := mcpBindingResponse{
		ID:               item.ID.String(),
		TenantID:         item.TenantID.String(),
		MCPServerID:      item.MCPServerID.String(),
		ServerKey:        item.ServerKey,
		ServerName:       item.ServerName,
		URL:              item.URL,
		Transport:        string(item.Transport),
		AuthStrategy:     string(item.AuthStrategy),
		CredentialEnvVar: item.CredentialEnvVar,
		RequiredEnvVars:  item.RequiredEnvVars,
		MissingEnvVars:   item.MissingEnvVars,
		SourceScope:      item.SourceScope,
		Status:           item.PreflightStatus(),
		RiskLevel:        item.RiskLevel,
		CreatedAt:        formatTime(item.CreatedAt),
		UpdatedAt:        formatTime(item.UpdatedAt),
	}
	if item.TeamID != nil {
		response.TeamID = item.TeamID.String()
	}
	if item.DigitalEmployeeID != nil {
		response.DigitalEmployeeID = item.DigitalEmployeeID.String()
	}
	return response
}

func effectiveMCPServerResponses(servers []EffectiveMCPServer) []effectiveMCPServerResponse {
	responses := make([]effectiveMCPServerResponse, 0, len(servers))
	for _, item := range servers {
		responses = append(responses, effectiveMCPServerResponse{
			ServerID:         item.ServerID.String(),
			ServerKey:        item.ServerKey,
			Name:             item.Name,
			Transport:        string(item.Transport),
			URL:              item.URL,
			AuthStrategy:     string(item.AuthStrategy),
			CredentialEnvVar: item.CredentialEnvVar,
			RequiredEnvVars:  item.RequiredEnvVars,
			MissingEnvVars:   item.MissingEnvVars,
			ToolAllowlist:    item.ToolAllowlist,
			RiskLevel:        item.RiskLevel,
			SourceScope:      item.SourceScope,
			Status:           item.BindingStatus(),
		})
	}
	return responses
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
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
