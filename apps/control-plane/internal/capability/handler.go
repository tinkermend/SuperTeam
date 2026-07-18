package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	CreateMCPServerDefinition(ctx context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error)
	ListMCPServerDefinitions(ctx context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error)
	DeleteMCPServerDefinition(ctx context.Context, req DeleteMCPServerDefinitionRequest) error
	CreateTeamMCPBinding(ctx context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error)
	ListTeamMCPBindings(ctx context.Context, req TeamScopedRequest) ([]MCPBinding, error)
	DeleteTeamMCPBinding(ctx context.Context, req DeleteTeamMCPBindingRequest) error
	CreateEmployeeMCPBindingV2(ctx context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error)
	ListEmployeeMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]MCPBinding, error)
	DeleteEmployeeMCPBindingV2(ctx context.Context, req DeleteEmployeeMCPBindingV2Request) error
	ListEffectiveMCPConfig(ctx context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error)
	ListProjectMCPBindings(ctx context.Context, req ProjectScopedRequest) ([]MCPBinding, error)
	PutProjectMCPBindings(ctx context.Context, req PutProjectMCPBindingsRequest) ([]MCPBinding, error)

	ListSkillMCPDependencies(ctx context.Context, req ListSkillMCPDependenciesRequest) ([]SkillMCPDependency, error)
	ReplaceSkillMCPDependencies(ctx context.Context, req ReplaceSkillMCPDependenciesRequest) ([]SkillMCPDependency, error)
	ListDependentSkills(ctx context.Context, req ListDependentSkillsRequest) ([]DependentSkill, error)
	EvaluateEmployeeSkillMCPDependencies(ctx context.Context, req EvaluateEmployeeSkillMCPDependenciesRequest) ([]EmployeeSkillMCPDependencyStatus, error)
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

func (h *HTTPHandler) ListTeamMCPBindings(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionTeamCapabilityManage, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp binding read", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	bindings, err := service.ListTeamMCPBindings(r.Context(), TeamScopedRequest{TenantID: tenantID, UserID: userID, TeamID: teamID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpBindingResponses(bindings))
}

func (h *HTTPHandler) DeleteTeamMCPBinding(w http.ResponseWriter, r *http.Request) {
	teamID, ok := uuidParam(w, r, "teamId", "invalid team id")
	if !ok {
		return
	}
	bindingID, ok := uuidParam(w, r, "bindingId", "invalid mcp binding id")
	if !ok {
		return
	}
	tenantID, _, ok := h.authorize(w, r, authz.ActionTeamCapabilityManage, authz.ResourceRef{Type: authz.ResourceTeam, ID: teamID.String()}, "team mcp binding delete", &teamID)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	if err := service.DeleteTeamMCPBinding(r.Context(), DeleteTeamMCPBindingRequest{TenantID: tenantID, TeamID: teamID, BindingID: bindingID}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HTTPHandler) ListEmployeeMCPBindingsV2(w http.ResponseWriter, r *http.Request) {
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
	bindings, err := service.ListEmployeeMCPBindingsV2(r.Context(), EmployeeScopedRequest{TenantID: tenantID, UserID: userID, DigitalEmployeeID: employeeID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpBindingResponses(bindings))
}

func (h *HTTPHandler) DeleteEmployeeMCPBindingV2(w http.ResponseWriter, r *http.Request) {
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
	if err := service.DeleteEmployeeMCPBindingV2(r.Context(), DeleteEmployeeMCPBindingV2Request{TenantID: tenantID, DigitalEmployeeID: employeeID, BindingID: bindingID}); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ----------------------------------------------------------------------------
// 项目级 MCP 绑定（迁移 072，目录与能力投影修订 spec §3.2）
// ----------------------------------------------------------------------------

// ListProjectMCPBindings 走项目级 authz（ResourceProject + project.config.read，项目成员
// 可读）：项目绑定属于项目配置面，归属边界由 checkProjectAccess 按项目成员关系裁决，
// 与 team 绑定端点用 team 维度动作（ResourceTeam + team.capability.manage）同构。
func (h *HTTPHandler) ListProjectMCPBindings(w http.ResponseWriter, r *http.Request) {
	projectID, ok := uuidParam(w, r, "projectId", "invalid project id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectConfigRead, authz.ResourceRef{Type: authz.ResourceProject, ID: projectID.String()}, "project mcp binding read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	bindings, err := service.ListProjectMCPBindings(r.Context(), ProjectScopedRequest{TenantID: tenantID, UserID: userID, ProjectID: projectID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpBindingResponses(bindings))
}

// PutProjectMCPBindings 声明式全量替换项目绑定集合；写路径要求 project.config.edit
// （项目 human_owner 或租户管理员），项目成员只读不可改。
func (h *HTTPHandler) PutProjectMCPBindings(w http.ResponseWriter, r *http.Request) {
	projectID, ok := uuidParam(w, r, "projectId", "invalid project id")
	if !ok {
		return
	}
	tenantID, userID, ok := h.authorize(w, r, authz.ActionProjectConfigEdit, authz.ResourceRef{Type: authz.ResourceProject, ID: projectID.String()}, "project mcp binding replace", nil)
	if !ok {
		return
	}
	var body putProjectMCPBindingsRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeHandlerError(w, fmt.Errorf("%w: invalid json body", ErrInvalidInput))
		return
	}
	// 声明式 PUT 里"清空"必须是显式的 []:字段缺失(nil)多半是键名手滑,
	// 宽容成清空会静默抹掉全部绑定(残债交接 §3),按契约 required 拒绝。
	if body.Items == nil {
		writeHandlerError(w, fmt.Errorf("%w: items is required (use [] to clear all bindings)", ErrInvalidInput))
		return
	}
	items := make([]ProjectMCPBindingInput, 0, len(body.Items))
	for _, item := range body.Items {
		serverID, err := uuid.Parse(item.MCPServerID)
		if err != nil {
			writeHandlerError(w, fmt.Errorf("%w: invalid mcp_server_id", ErrInvalidInput))
			return
		}
		items = append(items, ProjectMCPBindingInput{MCPServerID: serverID, CredentialEnvVar: item.CredentialEnvVar})
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	bindings, err := service.PutProjectMCPBindings(r.Context(), PutProjectMCPBindingsRequest{TenantID: tenantID, ProjectID: projectID, UserID: userID, Items: items})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mcpBindingResponses(bindings))
}

// ----------------------------------------------------------------------------
// Skill <-> MCP dependency declarations
// ----------------------------------------------------------------------------

func (h *HTTPHandler) ListSkillMCPDependencies(w http.ResponseWriter, r *http.Request) {
	skillID, ok := uuidParam(w, r, "skillId", "skill id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryRead, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "skill mcp dependencies read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	deps, err := service.ListSkillMCPDependencies(r.Context(), ListSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, SkillID: skillID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillMCPDependencyResponses(deps))
}

func (h *HTTPHandler) ReplaceSkillMCPDependencies(w http.ResponseWriter, r *http.Request) {
	skillID, ok := uuidParam(w, r, "skillId", "skill id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryManage, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "skill mcp dependencies replace", nil)
	if !ok {
		return
	}
	var body replaceSkillMCPDependenciesRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeHandlerError(w, fmt.Errorf("%w: invalid json body", ErrInvalidInput))
		return
	}
	items := make([]SkillMCPDependencyInput, 0, len(body.Items))
	for _, item := range body.Items {
		serverID, err := uuid.Parse(item.MCPServerID)
		if err != nil {
			writeHandlerError(w, fmt.Errorf("%w: invalid mcp_server_id", ErrInvalidInput))
			return
		}
		items = append(items, SkillMCPDependencyInput{MCPServerID: serverID, Note: item.Note})
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	deps, err := service.ReplaceSkillMCPDependencies(r.Context(), ReplaceSkillMCPDependenciesRequest{TenantID: tenantID, UserID: userID, SkillID: skillID, Items: items})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, skillMCPDependencyResponses(deps))
}

func (h *HTTPHandler) ListDependentSkills(w http.ResponseWriter, r *http.Request) {
	serverID, ok := uuidParam(w, r, "serverId", "server id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryRead, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "mcp dependent skills read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	skills, err := service.ListDependentSkills(r.Context(), ListDependentSkillsRequest{TenantID: tenantID, UserID: userID, ServerID: serverID})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, dependentSkillResponses(skills))
}

// ListEmployeeSkillMCPDependencyStatus is the employee panel data source: per-skill MCP
// dependency satisfaction status (satisfied | missing_binding | blocked_missing_env) for the
// skills runtime-effective on a digital employee. Read-only; gated by the same registry-read
// action as the other MCP registry list endpoints.
func (h *HTTPHandler) ListEmployeeSkillMCPDependencyStatus(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := uuidParam(w, r, "employeeId", "invalid employee id")
	if !ok {
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	tenantID, userID, ok := h.authorize(w, r, authz.ActionMCPRegistryRead, authz.ResourceRef{Type: authz.ResourceTenant, ID: tenantID.String()}, "employee skill mcp dependency status read", nil)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	statuses, err := service.EvaluateEmployeeSkillMCPDependencies(r.Context(), EvaluateEmployeeSkillMCPDependenciesRequest{
		TenantID:          tenantID,
		UserID:            userID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeSkillMCPDependencyStatusResponses(statuses))
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
	ProjectID         string   `json:"project_id,omitempty"`
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

func mcpBindingResponses(items []MCPBinding) []mcpBindingResponse {
	responses := make([]mcpBindingResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, mcpBindingResponseFromDomain(item))
	}
	return responses
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
	if item.ProjectID != nil {
		response.ProjectID = item.ProjectID.String()
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

type putProjectMCPBindingsRequestBody struct {
	Items []struct {
		MCPServerID      string `json:"mcp_server_id"`
		CredentialEnvVar string `json:"credential_env_var"`
	} `json:"items"`
}

type replaceSkillMCPDependenciesRequestBody struct {
	Items []struct {
		MCPServerID string `json:"mcp_server_id"`
		Note        string `json:"note"`
	} `json:"items"`
}

type skillMCPDependencyResponse struct {
	ID           string `json:"id"`
	SkillID      string `json:"skill_id"`
	MCPServerID  string `json:"mcp_server_id"`
	Note         string `json:"note"`
	ServerKey    string `json:"server_key"`
	ServerName   string `json:"server_name"`
	AuthStrategy string `json:"auth_strategy"`
	RiskLevel    string `json:"risk_level"`
	ServerStatus string `json:"server_status"`
	CreatedAt    string `json:"created_at"`
}

type dependentSkillResponse struct {
	SkillID string `json:"skill_id"`
	Slug    string `json:"slug"`
	Name    string `json:"name"`
}

func skillMCPDependencyResponses(deps []SkillMCPDependency) []skillMCPDependencyResponse {
	out := make([]skillMCPDependencyResponse, 0, len(deps))
	for _, d := range deps {
		out = append(out, skillMCPDependencyResponse{
			ID: d.ID.String(), SkillID: d.SkillID.String(), MCPServerID: d.MCPServerID.String(),
			Note: d.Note, ServerKey: d.ServerKey, ServerName: d.ServerName,
			AuthStrategy: string(d.AuthStrategy), RiskLevel: d.RiskLevel, ServerStatus: d.ServerStatus,
			CreatedAt: formatTime(d.CreatedAt),
		})
	}
	return out
}

func dependentSkillResponses(skills []DependentSkill) []dependentSkillResponse {
	out := make([]dependentSkillResponse, 0, len(skills))
	for _, s := range skills {
		out = append(out, dependentSkillResponse{SkillID: s.SkillID.String(), Slug: s.Slug, Name: s.Name})
	}
	return out
}

type employeeSkillMCPDependencyItemResponse struct {
	MCPServerID    string   `json:"mcp_server_id"`
	ServerKey      string   `json:"server_key"`
	ServerName     string   `json:"server_name"`
	Status         string   `json:"status"`
	MissingEnvVars []string `json:"missing_env_vars"`
}

type employeeSkillMCPDependencyStatusResponse struct {
	SkillID      string                                   `json:"skill_id"`
	SkillSlug    string                                   `json:"skill_slug"`
	Dependencies []employeeSkillMCPDependencyItemResponse `json:"dependencies"`
}

func employeeSkillMCPDependencyStatusResponses(statuses []EmployeeSkillMCPDependencyStatus) []employeeSkillMCPDependencyStatusResponse {
	out := make([]employeeSkillMCPDependencyStatusResponse, 0, len(statuses))
	for _, status := range statuses {
		deps := make([]employeeSkillMCPDependencyItemResponse, 0, len(status.Dependencies))
		for _, dep := range status.Dependencies {
			missingEnvVars := dep.MissingEnvVars
			if missingEnvVars == nil {
				missingEnvVars = []string{}
			}
			deps = append(deps, employeeSkillMCPDependencyItemResponse{
				MCPServerID:    dep.MCPServerID.String(),
				ServerKey:      dep.ServerKey,
				ServerName:     dep.ServerName,
				Status:         dep.Status,
				MissingEnvVars: missingEnvVars,
			})
		}
		out = append(out, employeeSkillMCPDependencyStatusResponse{
			SkillID:      status.SkillID.String(),
			SkillSlug:    status.SkillSlug,
			Dependencies: deps,
		})
	}
	return out
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
	case errors.Is(err, ErrInvalidInput), errors.Is(err, ErrCredentialKeyMissing):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrConflict):
		http.Error(w, err.Error(), http.StatusConflict)
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
