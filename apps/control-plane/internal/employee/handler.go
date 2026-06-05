package employee

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	GetCreateOptions(ctx context.Context, req CreateOptionsRequest) (*CreateOptions, error)
	CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error)
	ListDigitalEmployees(ctx context.Context, req ListDigitalEmployeesRequest) ([]*DigitalEmployee, error)
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployee, error)
	UpdateStatus(ctx context.Context, req UpdateStatusRequest) (*DigitalEmployee, error)
	GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeExecutionInstance, error)
	BindExecutionInstance(ctx context.Context, req BindExecutionInstanceRequest) (*DigitalEmployeeExecutionInstance, error)
	CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error)
	PreviewEffectiveConfigByRevisionIDs(ctx context.Context, req PreviewEffectiveConfigByRevisionIDsRequest) (*EffectiveConfigPreview, error)
	ApproveEffectiveConfig(ctx context.Context, req ApproveEffectiveConfigRequest) (*DigitalEmployeeEffectiveConfig, error)
}

type HTTPHandler struct {
	service    HandlerService
	runService RunHandlerService
	authorizer authz.Authorizer
}

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func NewHandlerWithRunService(service HandlerService, runService RunHandlerService) *HTTPHandler {
	return &HTTPHandler{service: service, runService: runService}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) SetRunService(runService RunHandlerService) {
	h.runService = runService
}

func (h *HTTPHandler) ListDigitalEmployees(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, nil, "digital employee read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	status := DigitalEmployeeStatus(r.URL.Query().Get("status"))
	var teamID *uuid.UUID
	if rawTeamID := r.URL.Query().Get("team_id"); rawTeamID != "" {
		parsedTeamID, err := uuid.Parse(rawTeamID)
		if err != nil {
			http.Error(w, "invalid team_id", http.StatusBadRequest)
			return
		}
		teamID = &parsedTeamID
	}

	employees, err := service.ListDigitalEmployees(r.Context(), ListDigitalEmployeesRequest{
		TenantID: tenantID,
		TeamID:   teamID,
		Status:   status,
		Offset:   int32(offset),
		Limit:    int32(limit),
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeResponses(employees))
}

func (h *HTTPHandler) GetCreateOptions(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "digital employee create options")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	rawTeamID := r.URL.Query().Get("team_id")
	teamID, err := uuid.Parse(rawTeamID)
	if err != nil || teamID == uuid.Nil {
		http.Error(w, "invalid team_id", http.StatusBadRequest)
		return
	}
	options, err := service.GetCreateOptions(r.Context(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, createOptionsResponseFromDomain(options))
}

func (h *HTTPHandler) CreateDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "digital employee create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		TeamID                 *uuid.UUID     `json:"team_id"`
		EmployeeType           string         `json:"employee_type"`
		Name                   string         `json:"name"`
		Role                   string         `json:"role"`
		Description            *string        `json:"description"`
		PermissionPolicy       map[string]any `json:"permission_policy"`
		ContextPolicy          map[string]any `json:"context_policy"`
		ApprovalPolicy         map[string]any `json:"approval_policy"`
		RiskLevel              string         `json:"risk_level"`
		Metadata               map[string]any `json:"metadata"`
		RoleProfile            map[string]any `json:"role_profile"`
		ConstitutionAddendum   map[string]any `json:"constitution_addendum"`
		CapabilitySelection    map[string]any `json:"capability_selection"`
		ContextPolicyOverride  map[string]any `json:"context_policy_override"`
		ApprovalPolicyOverride map[string]any `json:"approval_policy_override"`
		OutputContractAddendum map[string]any `json:"output_contract_addendum"`
		RuntimeNodeID          uuid.UUID      `json:"runtime_node_id"`
		ProviderType           string         `json:"provider_type"`
		SessionPolicy          map[string]any `json:"session_policy"`
		WorkspacePolicy        map[string]any `json:"workspace_policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	employee, err := service.CreateDigitalEmployee(r.Context(), CreateDigitalEmployeeRequest{
		TenantID:               tenantID,
		TeamID:                 req.TeamID,
		OwnerUserID:            middleware.GetUserID(r.Context()),
		EmployeeType:           req.EmployeeType,
		Name:                   req.Name,
		Role:                   req.Role,
		Description:            req.Description,
		PermissionPolicy:       req.PermissionPolicy,
		ContextPolicy:          req.ContextPolicy,
		ApprovalPolicy:         req.ApprovalPolicy,
		RiskLevel:              req.RiskLevel,
		Metadata:               req.Metadata,
		RoleProfile:            req.RoleProfile,
		ConstitutionAddendum:   req.ConstitutionAddendum,
		CapabilitySelection:    req.CapabilitySelection,
		ContextPolicyOverride:  req.ContextPolicyOverride,
		ApprovalPolicyOverride: req.ApprovalPolicyOverride,
		OutputContractAddendum: req.OutputContractAddendum,
		RuntimeNodeID:          req.RuntimeNodeID,
		ProviderType:           req.ProviderType,
		SessionPolicy:          req.SessionPolicy,
		WorkspacePolicy:        req.WorkspacePolicy,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, employeeResponseFromDomain(employee))
}

func (h *HTTPHandler) GetDigitalEmployee(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	employee, err := service.GetDigitalEmployee(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeResponseFromDomain(employee))
}

func (h *HTTPHandler) UpdateDigitalEmployeeStatus(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeStatusUpdate, &employeeID, "digital employee status update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Status DigitalEmployeeStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	employee, err := service.UpdateStatus(r.Context(), UpdateStatusRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Status:            req.Status,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeResponseFromDomain(employee))
}

func (h *HTTPHandler) GetDigitalEmployeeExecutionInstance(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeRead, &employeeID, "digital employee execution instance read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	instance, err := service.GetExecutionInstance(r.Context(), tenantID, employeeID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionInstanceResponseFromDomain(instance))
}

func (h *HTTPHandler) UpsertDigitalEmployeeExecutionInstance(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeExecutionBind, &employeeID, "digital employee execution instance bind")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		RuntimeNodeID        uuid.UUID      `json:"runtime_node_id"`
		ProviderType         string         `json:"provider_type"`
		AgentHomeDir         string         `json:"agent_home_dir"`
		WorkspacePolicy      map[string]any `json:"workspace_policy"`
		SessionPolicy        map[string]any `json:"session_policy"`
		RuntimeSelector      map[string]any `json:"runtime_selector"`
		CapacityRequirements map[string]any `json:"capacity_requirements"`
		FallbackPolicy       map[string]any `json:"fallback_policy"`
		Metadata             map[string]any `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	instance, err := service.BindExecutionInstance(r.Context(), BindExecutionInstanceRequest{
		TenantID:             tenantID,
		DigitalEmployeeID:    employeeID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         req.ProviderType,
		AgentHomeDir:         req.AgentHomeDir,
		WorkspacePolicy:      req.WorkspacePolicy,
		SessionPolicy:        req.SessionPolicy,
		RuntimeSelector:      req.RuntimeSelector,
		CapacityRequirements: req.CapacityRequirements,
		FallbackPolicy:       req.FallbackPolicy,
		Metadata:             req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executionInstanceResponseFromDomain(instance))
}

func (h *HTTPHandler) CreateDigitalEmployeeConfigRevision(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigCreate, &employeeID, "digital employee config revision create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		RoleProfile            map[string]any       `json:"role_profile"`
		ConstitutionAddendum   map[string]any       `json:"constitution_addendum"`
		CapabilitySelection    map[string]any       `json:"capability_selection"`
		ContextPolicyOverride  map[string]any       `json:"context_policy_override"`
		ApprovalPolicyOverride map[string]any       `json:"approval_policy_override"`
		OutputContractAddendum map[string]any       `json:"output_contract_addendum"`
		Status                 ConfigRevisionStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	revision, err := service.CreateConfigRevision(r.Context(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:               tenantID,
		DigitalEmployeeID:      employeeID,
		RoleProfile:            req.RoleProfile,
		ConstitutionAddendum:   req.ConstitutionAddendum,
		CapabilitySelection:    req.CapabilitySelection,
		ContextPolicyOverride:  req.ContextPolicyOverride,
		ApprovalPolicyOverride: req.ApprovalPolicyOverride,
		OutputContractAddendum: req.OutputContractAddendum,
		Status:                 req.Status,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, configRevisionResponseFromDomain(revision))
}

func (h *HTTPHandler) PreviewDigitalEmployeeEffectiveConfig(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigPreview, &employeeID, "digital employee effective config preview")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		TeamConfig struct {
			ID uuid.UUID `json:"id"`
		} `json:"team_config"`
		EmployeeConfig struct {
			ID uuid.UUID `json:"id"`
		} `json:"employee_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	preview, err := service.PreviewEffectiveConfigByRevisionIDs(r.Context(), PreviewEffectiveConfigByRevisionIDsRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     req.TeamConfig.ID,
		EmployeeConfigRevisionID: req.EmployeeConfig.ID,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, effectiveConfigPreviewResponseFromDomain(preview))
}

func (h *HTTPHandler) ApproveDigitalEmployeeEffectiveConfig(w http.ResponseWriter, r *http.Request) {
	employeeID, ok := employeeIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeConfigApprove, &employeeID, "digital employee effective config approve")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req struct {
		Preview struct {
			TeamConfig struct {
				ID uuid.UUID `json:"id"`
			} `json:"team_config"`
			EmployeeConfig struct {
				ID uuid.UUID `json:"id"`
			} `json:"employee_config"`
		} `json:"preview"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	approvedBy := middleware.GetUserID(r.Context())
	effectiveConfig, err := service.ApproveEffectiveConfig(r.Context(), ApproveEffectiveConfigRequest{
		TenantID:                 tenantID,
		DigitalEmployeeID:        employeeID,
		TeamConfigRevisionID:     req.Preview.TeamConfig.ID,
		EmployeeConfigRevisionID: req.Preview.EmployeeConfig.ID,
		ApprovedBy:               approvedBy,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, effectiveConfigResponseFromDomain(effectiveConfig))
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		http.Error(w, "employee service is not configured", http.StatusServiceUnavailable)
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) authorizeDigitalEmployeeManagement(w http.ResponseWriter, r *http.Request, action string, employeeID *uuid.UUID, auditReason string) (uuid.UUID, bool) {
	if h == nil || h.authorizer == nil {
		http.Error(w, "digital employee authorization is not configured", http.StatusForbidden)
		return uuid.Nil, false
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return uuid.Nil, false
	}
	resource := authz.ResourceRef{
		Type: authz.ResourceTenant,
		ID:   tenantID.String(),
	}
	if employeeID != nil {
		resource = authz.ResourceRef{
			Type: authz.ResourceEmployee,
			ID:   employeeID.String(),
		}
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   userID.String(),
		},
		Action:      action,
		Resource:    resource,
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

type digitalEmployeeResponse struct {
	ID               string                `json:"id"`
	TenantID         string                `json:"tenant_id"`
	TeamID           *string               `json:"team_id,omitempty"`
	OwnerUserID      string                `json:"owner_user_id"`
	EmployeeType     string                `json:"employee_type"`
	Name             string                `json:"name"`
	Role             string                `json:"role"`
	Description      *string               `json:"description,omitempty"`
	Status           DigitalEmployeeStatus `json:"status"`
	PermissionPolicy map[string]any        `json:"permission_policy"`
	ContextPolicy    map[string]any        `json:"context_policy"`
	ApprovalPolicy   map[string]any        `json:"approval_policy"`
	RiskLevel        string                `json:"risk_level"`
	Metadata         map[string]any        `json:"metadata"`
	DisabledAt       *string               `json:"disabled_at,omitempty"`
	ArchivedAt       *string               `json:"archived_at,omitempty"`
	CreatedAt        string                `json:"created_at,omitempty"`
	UpdatedAt        string                `json:"updated_at,omitempty"`
}

type createOptionsResponse struct {
	TeamConfig             teamConfigCreateOptionResponse  `json:"team_config"`
	EmployeeTypes          []employeeTypeOptionResponse    `json:"employee_types"`
	CapabilityOptions      capabilityOptionsResponse       `json:"capability_options"`
	RuntimeProviderOptions []runtimeProviderOptionResponse `json:"runtime_provider_options"`
	PolicyDefaults         policyDefaultsResponse          `json:"policy_defaults"`
}

type teamConfigCreateOptionResponse struct {
	ID                          string         `json:"id"`
	TenantID                    string         `json:"tenant_id"`
	TeamID                      string         `json:"team_id"`
	RevisionNumber              int32          `json:"revision_number"`
	Status                      string         `json:"status"`
	AllowedEmployeeTypes        []string       `json:"allowed_employee_types"`
	AllowedProviderTypes        []string       `json:"allowed_provider_types"`
	AllowedSkills               []string       `json:"allowed_skills"`
	AllowedMCPServers           []string       `json:"allowed_mcp_servers"`
	AllowedExternalCapabilities []string       `json:"allowed_external_capabilities"`
	CapabilityPolicy            map[string]any `json:"capability_policy"`
	ContextPolicy               map[string]any `json:"context_policy"`
	ApprovalPolicy              map[string]any `json:"approval_policy"`
	ArtifactContract            map[string]any `json:"artifact_contract"`
	InternalCollaborationPolicy map[string]any `json:"internal_collaboration_policy"`
	RuntimeScopePolicy          map[string]any `json:"runtime_scope_policy"`
}

type employeeTypeOptionResponse struct {
	Type                         string         `json:"type"`
	Label                        string         `json:"label"`
	Description                  string         `json:"description"`
	DefaultRole                  string         `json:"default_role"`
	RecommendedSkills            []string       `json:"recommended_skills"`
	RecommendedMCPServers        []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes     []string       `json:"recommended_provider_types"`
	DefaultCapabilitySelection   map[string]any `json:"default_capability_selection"`
	DefaultContextPolicyOverride map[string]any `json:"default_context_policy_override"`
	DefaultApprovalPolicy        map[string]any `json:"default_approval_policy"`
	Metadata                     map[string]any `json:"metadata"`
}

type capabilityOptionsResponse struct {
	ProviderTypes        []string `json:"provider_types"`
	Skills               []string `json:"skills"`
	MCPServers           []string `json:"mcp_servers"`
	ExternalCapabilities []string `json:"external_capabilities"`
}

type runtimeProviderOptionResponse struct {
	RuntimeNodeID         string `json:"runtime_node_id"`
	NodeID                string `json:"node_id"`
	RuntimeName           string `json:"runtime_name"`
	ProviderType          string `json:"provider_type"`
	RuntimeStatus         string `json:"runtime_status"`
	ProviderStatus        string `json:"provider_status"`
	HealthStatus          string `json:"health_status"`
	CurrentLoad           int32  `json:"current_load"`
	MaxSlots              int32  `json:"max_slots"`
	AgentHomeDir          string `json:"agent_home_dir"`
	AgentHomeDirAvailable bool   `json:"agent_home_dir_available"`
	Available             bool   `json:"available"`
	DisabledReason        string `json:"disabled_reason,omitempty"`
}

type policyDefaultsResponse struct {
	PermissionPolicy      map[string]any `json:"permission_policy"`
	ContextPolicyOverride map[string]any `json:"context_policy_override"`
	ApprovalPolicy        map[string]any `json:"approval_policy"`
	CapabilitySelection   map[string]any `json:"capability_selection"`
	RuntimeSelector       map[string]any `json:"runtime_selector"`
	WorkspacePolicy       map[string]any `json:"workspace_policy"`
	SessionPolicy         map[string]any `json:"session_policy"`
	Metadata              map[string]any `json:"metadata"`
}

type executionInstanceResponse struct {
	ID                   string                  `json:"id"`
	TenantID             string                  `json:"tenant_id"`
	DigitalEmployeeID    string                  `json:"digital_employee_id"`
	RuntimeNodeID        string                  `json:"runtime_node_id"`
	ProviderType         string                  `json:"provider_type"`
	AgentHomeDir         string                  `json:"agent_home_dir"`
	WorkspacePolicy      map[string]any          `json:"workspace_policy"`
	SessionPolicy        map[string]any          `json:"session_policy"`
	RuntimeSelector      map[string]any          `json:"runtime_selector"`
	CapacityRequirements map[string]any          `json:"capacity_requirements"`
	FallbackPolicy       map[string]any          `json:"fallback_policy"`
	Status               ExecutionInstanceStatus `json:"status"`
	ReadyAt              *string                 `json:"ready_at,omitempty"`
	DisabledAt           *string                 `json:"disabled_at,omitempty"`
	ErrorAt              *string                 `json:"error_at,omitempty"`
	ErrorMessage         *string                 `json:"error_message,omitempty"`
	Metadata             map[string]any          `json:"metadata"`
	CreatedAt            string                  `json:"created_at,omitempty"`
	UpdatedAt            string                  `json:"updated_at,omitempty"`
}

type configRevisionResponse struct {
	ID                     string               `json:"id"`
	TenantID               string               `json:"tenant_id"`
	DigitalEmployeeID      string               `json:"digital_employee_id"`
	RevisionNumber         int32                `json:"revision_number"`
	RoleProfile            map[string]any       `json:"role_profile"`
	ConstitutionAddendum   map[string]any       `json:"constitution_addendum"`
	CapabilitySelection    map[string]any       `json:"capability_selection"`
	ContextPolicyOverride  map[string]any       `json:"context_policy_override"`
	ApprovalPolicyOverride map[string]any       `json:"approval_policy_override"`
	OutputContractAddendum map[string]any       `json:"output_contract_addendum"`
	Status                 ConfigRevisionStatus `json:"status"`
	ApprovedBy             *string              `json:"approved_by,omitempty"`
	ApprovedAt             *string              `json:"approved_at,omitempty"`
	ArchivedAt             *string              `json:"archived_at,omitempty"`
	CreatedAt              string               `json:"created_at,omitempty"`
	UpdatedAt              string               `json:"updated_at,omitempty"`
}

type effectiveConfigPreviewResponse struct {
	TeamConfigRevisionID     string                    `json:"team_config_revision_id"`
	EmployeeConfigRevisionID string                    `json:"employee_config_revision_id"`
	EffectiveConfig          map[string]any            `json:"effective_config"`
	Validation               EffectiveConfigValidation `json:"validation"`
}

type effectiveConfigResponse struct {
	ID                       string                `json:"id"`
	TenantID                 string                `json:"tenant_id"`
	DigitalEmployeeID        string                `json:"digital_employee_id"`
	TeamConfigRevisionID     string                `json:"team_config_revision_id"`
	EmployeeConfigRevisionID string                `json:"employee_config_revision_id"`
	EffectiveConfig          map[string]any        `json:"effective_config"`
	ValidationResult         map[string]any        `json:"validation_result"`
	Status                   EffectiveConfigStatus `json:"status"`
	ApprovedBy               *string               `json:"approved_by,omitempty"`
	ApprovedAt               *string               `json:"approved_at,omitempty"`
	RevokedAt                *string               `json:"revoked_at,omitempty"`
	CreatedAt                string                `json:"created_at,omitempty"`
	UpdatedAt                string                `json:"updated_at,omitempty"`
}

func employeeIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	employeeID, err := uuid.Parse(chi.URLParam(r, "employeeId"))
	if err != nil || employeeID == uuid.Nil {
		http.Error(w, "invalid employee id", http.StatusBadRequest)
		return uuid.Nil, false
	}
	return employeeID, true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrRuntimeUnavailable), errors.Is(err, ErrProviderUnavailable):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, ErrEffectiveConfigRequired):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, ErrInvalidInput):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, ErrConflict):
		http.Error(w, "conflict", http.StatusConflict)
	default:
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func employeeResponses(employees []*DigitalEmployee) []digitalEmployeeResponse {
	responses := make([]digitalEmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		responses = append(responses, employeeResponseFromDomain(employee))
	}
	return responses
}

func employeeResponseFromDomain(employee *DigitalEmployee) digitalEmployeeResponse {
	return digitalEmployeeResponse{
		ID:               employee.ID.String(),
		TenantID:         employee.TenantID.String(),
		TeamID:           uuidStringPtr(employee.TeamID),
		OwnerUserID:      employee.OwnerUserID.String(),
		EmployeeType:     employee.EmployeeType,
		Name:             employee.Name,
		Role:             employee.Role,
		Description:      employee.Description,
		Status:           employee.Status,
		PermissionPolicy: cloneMap(employee.PermissionPolicy),
		ContextPolicy:    cloneMap(employee.ContextPolicy),
		ApprovalPolicy:   cloneMap(employee.ApprovalPolicy),
		RiskLevel:        employee.RiskLevel,
		Metadata:         cloneMap(employee.Metadata),
		DisabledAt:       timeStringPtr(employee.DisabledAt),
		ArchivedAt:       timeStringPtr(employee.ArchivedAt),
		CreatedAt:        timeString(employee.CreatedAt),
		UpdatedAt:        timeString(employee.UpdatedAt),
	}
}

func createOptionsResponseFromDomain(options *CreateOptions) createOptionsResponse {
	runtimeOptions := make([]runtimeProviderOptionResponse, 0, len(options.RuntimeProviderOptions))
	for _, option := range options.RuntimeProviderOptions {
		runtimeOptions = append(runtimeOptions, runtimeProviderOptionResponse{
			RuntimeNodeID:         option.RuntimeNodeID.String(),
			NodeID:                option.NodeID,
			RuntimeName:           option.RuntimeName,
			ProviderType:          option.ProviderType,
			RuntimeStatus:         option.RuntimeStatus,
			ProviderStatus:        option.ProviderStatus,
			HealthStatus:          option.HealthStatus,
			CurrentLoad:           option.CurrentLoad,
			MaxSlots:              option.MaxSlots,
			AgentHomeDir:          option.AgentHomeDir,
			AgentHomeDirAvailable: option.AgentHomeDirAvailable,
			Available:             option.Available,
			DisabledReason:        option.DisabledReason,
		})
	}
	employeeTypes := make([]employeeTypeOptionResponse, 0, len(options.EmployeeTypes))
	for _, definition := range options.EmployeeTypes {
		employeeTypes = append(employeeTypes, employeeTypeOptionResponse{
			Type:                         definition.Type,
			Label:                        definition.Label,
			Description:                  definition.Description,
			DefaultRole:                  definition.DefaultRole,
			RecommendedSkills:            stringSliceForJSON(definition.RecommendedSkills),
			RecommendedMCPServers:        stringSliceForJSON(definition.RecommendedMCPServers),
			RecommendedProviderTypes:     stringSliceForJSON(definition.RecommendedProviderTypes),
			DefaultCapabilitySelection:   cloneMap(definition.DefaultCapabilitySelection),
			DefaultContextPolicyOverride: cloneMap(definition.DefaultContextPolicyOverride),
			DefaultApprovalPolicy:        cloneMap(definition.DefaultApprovalPolicy),
			Metadata:                     cloneMap(definition.Metadata),
		})
	}
	return createOptionsResponse{
		TeamConfig: teamConfigCreateOptionResponse{
			ID:                          options.TeamConfig.ID.String(),
			TenantID:                    options.TeamConfig.TenantID.String(),
			TeamID:                      options.TeamConfig.TeamID.String(),
			RevisionNumber:              options.TeamConfig.RevisionNumber,
			Status:                      string(options.TeamConfig.Status),
			AllowedEmployeeTypes:        stringSliceForJSON(options.TeamConfig.AllowedEmployeeTypes),
			AllowedProviderTypes:        stringSliceForJSON(options.TeamConfig.AllowedProviderTypes),
			AllowedSkills:               stringSliceForJSON(options.TeamConfig.AllowedSkills),
			AllowedMCPServers:           stringSliceForJSON(options.TeamConfig.AllowedMCPServers),
			AllowedExternalCapabilities: stringSliceForJSON(options.TeamConfig.AllowedExternalCaps),
			CapabilityPolicy:            cloneMap(options.TeamConfig.CapabilityPolicy),
			ContextPolicy:               cloneMap(options.TeamConfig.ContextPolicy),
			ApprovalPolicy:              cloneMap(options.TeamConfig.ApprovalPolicy),
			ArtifactContract:            cloneMap(options.TeamConfig.ArtifactContract),
			InternalCollaborationPolicy: cloneMap(options.TeamConfig.InternalCollaborationPolicy),
			RuntimeScopePolicy:          cloneMap(options.TeamConfig.RuntimeScopePolicy),
		},
		EmployeeTypes: employeeTypes,
		CapabilityOptions: capabilityOptionsResponse{
			ProviderTypes:        stringSliceForJSON(options.CapabilityOptions.ProviderTypes),
			Skills:               stringSliceForJSON(options.CapabilityOptions.Skills),
			MCPServers:           stringSliceForJSON(options.CapabilityOptions.MCPServers),
			ExternalCapabilities: stringSliceForJSON(options.CapabilityOptions.ExternalCapabilities),
		},
		RuntimeProviderOptions: runtimeOptions,
		PolicyDefaults: policyDefaultsResponse{
			PermissionPolicy:      cloneMap(options.PolicyDefaults.PermissionPolicy),
			ContextPolicyOverride: cloneMap(options.PolicyDefaults.ContextPolicyOverride),
			ApprovalPolicy:        cloneMap(options.PolicyDefaults.ApprovalPolicy),
			CapabilitySelection:   cloneMap(options.PolicyDefaults.CapabilitySelection),
			RuntimeSelector:       cloneMap(options.PolicyDefaults.RuntimeSelector),
			WorkspacePolicy:       cloneMap(options.PolicyDefaults.WorkspacePolicy),
			SessionPolicy:         cloneMap(options.PolicyDefaults.SessionPolicy),
			Metadata:              cloneMap(options.PolicyDefaults.Metadata),
		},
	}
}

func stringSliceForJSON(values []string) []string {
	if values == nil {
		return []string{}
	}
	return cloneStringSlice(values)
}

func executionInstanceResponseFromDomain(instance *DigitalEmployeeExecutionInstance) executionInstanceResponse {
	return executionInstanceResponse{
		ID:                   instance.ID.String(),
		TenantID:             instance.TenantID.String(),
		DigitalEmployeeID:    instance.DigitalEmployeeID.String(),
		RuntimeNodeID:        instance.RuntimeNodeID.String(),
		ProviderType:         instance.ProviderType,
		AgentHomeDir:         instance.AgentHomeDir,
		WorkspacePolicy:      cloneMap(instance.WorkspacePolicy),
		SessionPolicy:        cloneMap(instance.SessionPolicy),
		RuntimeSelector:      cloneMap(instance.RuntimeSelector),
		CapacityRequirements: cloneMap(instance.CapacityRequirements),
		FallbackPolicy:       cloneMap(instance.FallbackPolicy),
		Status:               instance.Status,
		ReadyAt:              timeStringPtr(instance.ReadyAt),
		DisabledAt:           timeStringPtr(instance.DisabledAt),
		ErrorAt:              timeStringPtr(instance.ErrorAt),
		ErrorMessage:         instance.ErrorMessage,
		Metadata:             cloneMap(instance.Metadata),
		CreatedAt:            timeString(instance.CreatedAt),
		UpdatedAt:            timeString(instance.UpdatedAt),
	}
}

func configRevisionResponseFromDomain(revision *DigitalEmployeeConfigRevision) configRevisionResponse {
	return configRevisionResponse{
		ID:                     revision.ID.String(),
		TenantID:               revision.TenantID.String(),
		DigitalEmployeeID:      revision.DigitalEmployeeID.String(),
		RevisionNumber:         revision.RevisionNumber,
		RoleProfile:            cloneMap(revision.RoleProfile),
		ConstitutionAddendum:   cloneMap(revision.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(revision.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(revision.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(revision.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(revision.OutputContractAddendum),
		Status:                 revision.Status,
		ApprovedBy:             uuidStringPtr(revision.ApprovedBy),
		ApprovedAt:             timeStringPtr(revision.ApprovedAt),
		ArchivedAt:             timeStringPtr(revision.ArchivedAt),
		CreatedAt:              timeString(revision.CreatedAt),
		UpdatedAt:              timeString(revision.UpdatedAt),
	}
}

func effectiveConfigPreviewResponseFromDomain(preview *EffectiveConfigPreview) effectiveConfigPreviewResponse {
	return effectiveConfigPreviewResponse{
		TeamConfigRevisionID:     preview.TeamConfigRevisionID.String(),
		EmployeeConfigRevisionID: preview.EmployeeConfigRevisionID.String(),
		EffectiveConfig:          cloneMap(preview.EffectiveConfig),
		Validation:               preview.Validation,
	}
}

func effectiveConfigResponseFromDomain(effectiveConfig *DigitalEmployeeEffectiveConfig) effectiveConfigResponse {
	return effectiveConfigResponse{
		ID:                       effectiveConfig.ID.String(),
		TenantID:                 effectiveConfig.TenantID.String(),
		DigitalEmployeeID:        effectiveConfig.DigitalEmployeeID.String(),
		TeamConfigRevisionID:     effectiveConfig.TeamConfigRevisionID.String(),
		EmployeeConfigRevisionID: effectiveConfig.EmployeeConfigRevisionID.String(),
		EffectiveConfig:          cloneMap(effectiveConfig.EffectiveConfig),
		ValidationResult:         cloneMap(effectiveConfig.ValidationResult),
		Status:                   effectiveConfig.Status,
		ApprovedBy:               uuidStringPtr(effectiveConfig.ApprovedBy),
		ApprovedAt:               timeStringPtr(effectiveConfig.ApprovedAt),
		RevokedAt:                timeStringPtr(effectiveConfig.RevokedAt),
		CreatedAt:                timeString(effectiveConfig.CreatedAt),
		UpdatedAt:                timeString(effectiveConfig.UpdatedAt),
	}
}

func uuidStringPtr(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func timeStringPtr(value *time.Time) *string {
	if value == nil || value.IsZero() {
		return nil
	}
	text := value.UTC().Format(time.RFC3339Nano)
	return &text
}

func timeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
