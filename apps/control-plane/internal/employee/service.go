package employee

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository}, nil
}

func (s *Service) CreateDraft(ctx context.Context, req CreateDraftRequest) (*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == nil || *req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		return nil, fmt.Errorf("%w: role is required", ErrInvalidInput)
	}
	description := trimOptionalString(req.Description)
	riskLevel := strings.TrimSpace(req.RiskLevel)
	if riskLevel == "" {
		riskLevel = "medium"
	}

	record, err := s.repository.CreateDigitalEmployee(ctx, CreateDigitalEmployeeParams{
		TenantID:         req.TenantID,
		TeamID:           validUUIDPtr(req.TeamID),
		Name:             name,
		Role:             role,
		Description:      description,
		Status:           DigitalEmployeeStatusDraft,
		PermissionPolicy: cloneMap(req.PermissionPolicy),
		ContextPolicy:    cloneMap(req.ContextPolicy),
		ApprovalPolicy:   cloneMap(req.ApprovalPolicy),
		RiskLevel:        riskLevel,
		Metadata:         cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create digital employee: %w", err)
	}
	return employeeFromRecord(record), nil
}

func (s *Service) ListDigitalEmployees(ctx context.Context, req ListDigitalEmployeesRequest) ([]*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.Status != "" && !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	records, err := s.repository.ListDigitalEmployees(ctx, ListDigitalEmployeesParams{
		TenantID: req.TenantID,
		TeamID:   validUUIDPtr(req.TeamID),
		Status:   req.Status,
		Offset:   req.Offset,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list digital employees: %w", err)
	}
	employees := make([]*DigitalEmployee, 0, len(records))
	for _, record := range records {
		employees = append(employees, employeeFromRecord(record))
	}
	return employees, nil
}

func (s *Service) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployee, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetDigitalEmployee(ctx, tenantID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	return employeeFromRecord(record), nil
}

func (s *Service) UpdateStatus(ctx context.Context, req UpdateStatusRequest) (*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	if !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	record, err := s.repository.UpdateDigitalEmployeeStatus(ctx, req.TenantID, req.DigitalEmployeeID, req.Status)
	if err != nil {
		return nil, fmt.Errorf("update digital employee status: %w", err)
	}
	return employeeFromRecord(record), nil
}

func (s *Service) BindExecutionInstance(ctx context.Context, req BindExecutionInstanceRequest) (*DigitalEmployeeExecutionInstance, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	if req.RuntimeNodeID == uuid.Nil {
		return nil, fmt.Errorf("%w: runtime_node_id is required", ErrInvalidInput)
	}
	providerType := strings.TrimSpace(req.ProviderType)
	if providerType == "" {
		return nil, fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
	}
	agentHomeDir := strings.TrimSpace(req.AgentHomeDir)
	if agentHomeDir == "" {
		return nil, fmt.Errorf("%w: agent_home_dir is required", ErrInvalidInput)
	}

	record, err := s.repository.UpsertDigitalEmployeeExecutionInstance(ctx, UpsertExecutionInstanceParams{
		TenantID:             req.TenantID,
		DigitalEmployeeID:    req.DigitalEmployeeID,
		RuntimeNodeID:        req.RuntimeNodeID,
		ProviderType:         providerType,
		AgentHomeDir:         agentHomeDir,
		WorkspacePolicy:      cloneMap(req.WorkspacePolicy),
		SessionPolicy:        cloneMap(req.SessionPolicy),
		RuntimeSelector:      cloneMap(req.RuntimeSelector),
		CapacityRequirements: cloneMap(req.CapacityRequirements),
		FallbackPolicy:       cloneMap(req.FallbackPolicy),
		Status:               ExecutionInstanceStatusReady,
		Metadata:             cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("upsert digital employee execution instance: %w", err)
	}
	return executionInstanceFromRecord(record), nil
}

func (s *Service) GetExecutionInstance(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeExecutionInstance, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, tenantID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee execution instance: %w", err)
	}
	return executionInstanceFromRecord(record), nil
}

func (s *Service) CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = ConfigRevisionStatusDraft
	}
	if status != ConfigRevisionStatusDraft {
		return nil, fmt.Errorf("%w: invalid config revision status", ErrInvalidInput)
	}
	if _, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID); err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	nextRevision, err := s.repository.GetNextDigitalEmployeeConfigRevisionNumber(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get next digital employee config revision number: %w", err)
	}
	record, err := s.repository.CreateDigitalEmployeeConfigRevision(ctx, CreateConfigRevisionParams{
		TenantID:               req.TenantID,
		DigitalEmployeeID:      req.DigitalEmployeeID,
		RevisionNumber:         nextRevision,
		RoleProfile:            cloneMap(req.RoleProfile),
		ConstitutionAddendum:   cloneMap(req.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(req.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(req.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(req.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(req.OutputContractAddendum),
		Status:                 status,
	})
	if err != nil {
		return nil, fmt.Errorf("create digital employee config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) PreviewEffectiveConfig(ctx context.Context, req PreviewEffectiveConfigRequest) (*EffectiveConfigPreview, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.TeamConfig.ID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_config_revision_id is required", ErrInvalidInput)
	}
	if req.EmployeeConfig.ID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_config_revision_id is required", ErrInvalidInput)
	}

	effectiveConfig := map[string]any{
		"team_config_revision_id":     req.TeamConfig.ID.String(),
		"employee_config_revision_id": req.EmployeeConfig.ID.String(),
		"constitution": map[string]any{
			"team":     cloneMap(req.TeamConfig.Constitution),
			"addendum": cloneMap(req.EmployeeConfig.ConstitutionAddendum),
		},
		"capability_policy":             cloneMap(req.TeamConfig.CapabilityPolicy),
		"capability_selection":          cloneMap(req.EmployeeConfig.CapabilitySelection),
		"context_policy":                cloneMap(req.TeamConfig.ContextPolicy),
		"context_policy_override":       cloneMap(req.EmployeeConfig.ContextPolicyOverride),
		"approval_policy":               cloneMap(req.TeamConfig.ApprovalPolicy),
		"approval_policy_override":      cloneMap(req.EmployeeConfig.ApprovalPolicyOverride),
		"artifact_contract":             cloneMap(req.TeamConfig.ArtifactContract),
		"output_contract_addendum":      cloneMap(req.EmployeeConfig.OutputContractAddendum),
		"internal_collaboration_policy": cloneMap(req.TeamConfig.InternalCollaborationPolicy),
		"runtime_scope_policy":          cloneMap(req.TeamConfig.RuntimeScopePolicy),
	}
	validation := EffectiveConfigValidation{
		BlockingErrors: []ValidationIssue{},
		Warnings:       []ValidationIssue{},
	}
	validation.BlockingErrors = append(validation.BlockingErrors, validateCapabilitySubset(req.TeamConfig.CapabilityPolicy, req.EmployeeConfig.CapabilitySelection)...)
	validation.BlockingErrors = append(validation.BlockingErrors, validateContextSubset(req.TeamConfig.ContextPolicy, req.EmployeeConfig.ContextPolicyOverride)...)
	validation.BlockingErrors = append(validation.BlockingErrors, validateApprovalOverride(req.TeamConfig.ApprovalPolicy, req.EmployeeConfig.ApprovalPolicyOverride)...)

	return &EffectiveConfigPreview{
		TeamConfigRevisionID:     req.TeamConfig.ID,
		EmployeeConfigRevisionID: req.EmployeeConfig.ID,
		EffectiveConfig:          effectiveConfig,
		Validation:               validation,
	}, nil
}

func (s *Service) PreviewEffectiveConfigByRevisionIDs(ctx context.Context, req PreviewEffectiveConfigByRevisionIDsRequest) (*EffectiveConfigPreview, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.TeamConfigRevisionID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_config_revision_id is required", ErrInvalidInput)
	}
	if req.EmployeeConfigRevisionID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_config_revision_id is required", ErrInvalidInput)
	}
	teamConfig, err := s.repository.GetTeamConfigRevision(ctx, req.TenantID, req.TeamConfigRevisionID)
	if err != nil {
		return nil, fmt.Errorf("get team config revision: %w", err)
	}
	employee, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	if employee.TeamID == nil || *employee.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital employee team_id is required for effective config preview", ErrInvalidInput)
	}
	if teamConfig.TeamID != *employee.TeamID {
		return nil, fmt.Errorf("%w: team config revision does not belong to digital employee team", ErrInvalidInput)
	}
	employeeConfig, err := s.repository.GetDigitalEmployeeConfigRevision(ctx, req.TenantID, req.DigitalEmployeeID, req.EmployeeConfigRevisionID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee config revision: %w", err)
	}
	return s.PreviewEffectiveConfig(ctx, PreviewEffectiveConfigRequest{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		TeamConfig:        teamConfig,
		EmployeeConfig:    employeeConfig,
	})
}

func (s *Service) ApproveEffectiveConfig(ctx context.Context, req ApproveEffectiveConfigRequest) (*DigitalEmployeeEffectiveConfig, error) {
	if req.ApprovedBy == uuid.Nil {
		return nil, fmt.Errorf("%w: approved_by is required", ErrInvalidInput)
	}
	preview, err := s.PreviewEffectiveConfigByRevisionIDs(ctx, PreviewEffectiveConfigByRevisionIDsRequest{
		TenantID:                 req.TenantID,
		DigitalEmployeeID:        req.DigitalEmployeeID,
		TeamConfigRevisionID:     req.TeamConfigRevisionID,
		EmployeeConfigRevisionID: req.EmployeeConfigRevisionID,
	})
	if err != nil {
		return nil, err
	}
	if len(preview.Validation.BlockingErrors) > 0 {
		return nil, fmt.Errorf("%w: effective config has blocking validation errors", ErrInvalidInput)
	}
	instance, err := s.repository.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee execution instance: %w", err)
	}
	if instance.Status != ExecutionInstanceStatusReady && instance.Status != ExecutionInstanceStatusActive {
		return nil, fmt.Errorf("%w: execution instance must be ready or active", ErrInvalidInput)
	}
	if _, err := s.repository.GetCurrentDigitalEmployeeEffectiveConfig(ctx, req.TenantID, req.DigitalEmployeeID); err == nil {
		return nil, fmt.Errorf("%w: approved effective config already exists", ErrConflict)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get current digital employee effective config: %w", err)
	}
	now := time.Now().UTC()
	approvedBy := req.ApprovedBy
	record, err := s.repository.CreateDigitalEmployeeEffectiveConfig(ctx, CreateEffectiveConfigParams{
		TenantID:                 req.TenantID,
		DigitalEmployeeID:        req.DigitalEmployeeID,
		TeamConfigRevisionID:     req.TeamConfigRevisionID,
		EmployeeConfigRevisionID: req.EmployeeConfigRevisionID,
		EffectiveConfig:          cloneMap(preview.EffectiveConfig),
		ValidationResult:         validationResultMap(preview.Validation),
		Status:                   EffectiveConfigStatusApproved,
		ApprovedBy:               &approvedBy,
		ApprovedAt:               &now,
	})
	if err != nil {
		return nil, fmt.Errorf("create digital employee effective config: %w", err)
	}
	return effectiveConfigFromRecord(record), nil
}

func employeeFromRecord(record DigitalEmployeeRecord) *DigitalEmployee {
	return &DigitalEmployee{
		ID:               record.ID,
		TenantID:         record.TenantID,
		TeamID:           validUUIDPtr(record.TeamID),
		Name:             record.Name,
		Role:             record.Role,
		Description:      trimOptionalString(record.Description),
		Status:           record.Status,
		PermissionPolicy: cloneMap(record.PermissionPolicy),
		ContextPolicy:    cloneMap(record.ContextPolicy),
		ApprovalPolicy:   cloneMap(record.ApprovalPolicy),
		RiskLevel:        record.RiskLevel,
		Metadata:         cloneMap(record.Metadata),
		DisabledAt:       cloneTimePtr(record.DisabledAt),
		ArchivedAt:       cloneTimePtr(record.ArchivedAt),
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func configRevisionFromRecord(record DigitalEmployeeConfigRevisionRecord) *DigitalEmployeeConfigRevision {
	return &DigitalEmployeeConfigRevision{
		ID:                     record.ID,
		TenantID:               record.TenantID,
		DigitalEmployeeID:      record.DigitalEmployeeID,
		RevisionNumber:         record.RevisionNumber,
		RoleProfile:            cloneMap(record.RoleProfile),
		ConstitutionAddendum:   cloneMap(record.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(record.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(record.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(record.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(record.OutputContractAddendum),
		Status:                 record.Status,
		ApprovedBy:             validUUIDPtr(record.ApprovedBy),
		ApprovedAt:             cloneTimePtr(record.ApprovedAt),
		ArchivedAt:             cloneTimePtr(record.ArchivedAt),
		CreatedAt:              record.CreatedAt,
		UpdatedAt:              record.UpdatedAt,
	}
}

func effectiveConfigFromRecord(record DigitalEmployeeEffectiveConfigRecord) *DigitalEmployeeEffectiveConfig {
	return &DigitalEmployeeEffectiveConfig{
		ID:                       record.ID,
		TenantID:                 record.TenantID,
		DigitalEmployeeID:        record.DigitalEmployeeID,
		TeamConfigRevisionID:     record.TeamConfigRevisionID,
		EmployeeConfigRevisionID: record.EmployeeConfigRevisionID,
		EffectiveConfig:          cloneMap(record.EffectiveConfig),
		ValidationResult:         cloneMap(record.ValidationResult),
		Status:                   record.Status,
		ApprovedBy:               validUUIDPtr(record.ApprovedBy),
		ApprovedAt:               cloneTimePtr(record.ApprovedAt),
		RevokedAt:                cloneTimePtr(record.RevokedAt),
		CreatedAt:                record.CreatedAt,
		UpdatedAt:                record.UpdatedAt,
	}
}

func executionInstanceFromRecord(record DigitalEmployeeExecutionInstanceRecord) *DigitalEmployeeExecutionInstance {
	return &DigitalEmployeeExecutionInstance{
		ID:                   record.ID,
		TenantID:             record.TenantID,
		DigitalEmployeeID:    record.DigitalEmployeeID,
		RuntimeNodeID:        record.RuntimeNodeID,
		ProviderType:         record.ProviderType,
		AgentHomeDir:         record.AgentHomeDir,
		WorkspacePolicy:      cloneMap(record.WorkspacePolicy),
		SessionPolicy:        cloneMap(record.SessionPolicy),
		RuntimeSelector:      cloneMap(record.RuntimeSelector),
		CapacityRequirements: cloneMap(record.CapacityRequirements),
		FallbackPolicy:       cloneMap(record.FallbackPolicy),
		Status:               record.Status,
		ReadyAt:              cloneTimePtr(record.ReadyAt),
		DisabledAt:           cloneTimePtr(record.DisabledAt),
		ErrorAt:              cloneTimePtr(record.ErrorAt),
		ErrorMessage:         trimOptionalString(record.ErrorMessage),
		Metadata:             cloneMap(record.Metadata),
		CreatedAt:            record.CreatedAt,
		UpdatedAt:            record.UpdatedAt,
	}
}

func validateCapabilitySubset(teamPolicy, employeeSelection map[string]any) []ValidationIssue {
	pairs := []struct {
		teamKey     string
		employeeKey string
	}{
		{teamKey: "allowed_mcp_servers", employeeKey: "enabled_mcp_servers"},
		{teamKey: "allowed_skills", employeeKey: "enabled_skills"},
		{teamKey: "allowed_plugins", employeeKey: "enabled_plugins"},
		{teamKey: "allowed_external_capabilities", employeeKey: "enabled_external_capabilities"},
		{teamKey: "allowed_provider_types", employeeKey: "enabled_provider_types"},
	}
	issues := []ValidationIssue{}
	for _, pair := range pairs {
		allowed, allowedIssues := stringListPolicyValue(teamPolicy, pair.teamKey, fmt.Sprintf("capability_policy.%s", pair.teamKey))
		enabled, enabledIssues := stringListPolicyValue(employeeSelection, pair.employeeKey, fmt.Sprintf("capability_selection.%s", pair.employeeKey))
		issues = append(issues, allowedIssues...)
		issues = append(issues, enabledIssues...)
		if len(enabled) == 0 {
			continue
		}
		allowedSet := stringSet(allowed)
		var outside []string
		for _, item := range enabled {
			if !allowedSet[item] {
				outside = append(outside, item)
			}
		}
		if len(outside) == 0 {
			continue
		}
		issues = append(issues, ValidationIssue{
			Code:    "capability_outside_team_allowlist",
			Path:    fmt.Sprintf("capability_selection.%s", pair.employeeKey),
			Message: fmt.Sprintf("capabilities are outside team allowlist: %s", strings.Join(outside, ", ")),
		})
	}
	return issues
}

func validateContextSubset(teamPolicy, employeeOverride map[string]any) []ValidationIssue {
	keys := []string{"sources", "knowledge_bases", "documents", "repositories", "log_sources"}
	issues := []ValidationIssue{}
	for _, key := range keys {
		allowed, allowedIssues := stringListPolicyValue(teamPolicy, key, fmt.Sprintf("context_policy.%s", key))
		requested, requestedIssues := stringListPolicyValue(employeeOverride, key, fmt.Sprintf("context_policy_override.%s", key))
		issues = append(issues, allowedIssues...)
		issues = append(issues, requestedIssues...)
		if len(requested) == 0 {
			continue
		}
		allowedSet := stringSet(allowed)
		var outside []string
		for _, item := range requested {
			if !allowedSet[item] {
				outside = append(outside, item)
			}
		}
		if len(outside) == 0 {
			continue
		}
		issues = append(issues, ValidationIssue{
			Code:    "context_outside_team_scope",
			Path:    fmt.Sprintf("context_policy_override.%s", key),
			Message: fmt.Sprintf("context refs are outside team scope: %s", strings.Join(outside, ", ")),
		})
	}
	return issues
}

func validateApprovalOverride(teamPolicy, employeeOverride map[string]any) []ValidationIssue {
	issues := []ValidationIssue{}
	teamRank, _, teamRiskIssues := riskPolicyValue(teamPolicy, "min_risk_for_human", "approval_policy.min_risk_for_human")
	overrideRank, _, overrideRiskIssues := riskPolicyValue(employeeOverride, "min_risk_for_human", "approval_policy_override.min_risk_for_human")
	issues = append(issues, teamRiskIssues...)
	issues = append(issues, overrideRiskIssues...)
	if teamRank > 0 && overrideRank > teamRank {
		issues = append(issues, ValidationIssue{
			Code:    "approval_policy_downgrade",
			Path:    "approval_policy_override.min_risk_for_human",
			Message: "approval override cannot lower team approval requirements",
		})
	}
	teamWriteRequired, teamWriteSet, teamWriteIssues := boolPolicyValue(teamPolicy, "write_actions_require_human", "approval_policy.write_actions_require_human")
	overrideWriteRequired, overrideWriteSet, overrideWriteIssues := boolPolicyValue(employeeOverride, "write_actions_require_human", "approval_policy_override.write_actions_require_human")
	issues = append(issues, teamWriteIssues...)
	issues = append(issues, overrideWriteIssues...)
	if teamWriteSet && teamWriteRequired && overrideWriteSet && !overrideWriteRequired {
		issues = append(issues, ValidationIssue{
			Code:    "approval_policy_downgrade",
			Path:    "approval_policy_override.write_actions_require_human",
			Message: "approval override cannot remove human approval for write actions",
		})
	}
	return issues
}

func stringListPolicyValue(values map[string]any, key, path string) ([]string, []ValidationIssue) {
	value, ok := values[key]
	if !ok {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		return stringList(typed), nil
	case []any:
		items := make([]string, 0, len(typed))
		issues := []ValidationIssue{}
		for index, item := range typed {
			text, ok := item.(string)
			if !ok {
				issues = append(issues, invalidPolicyValueIssue(path, fmt.Sprintf("policy list item %d must be a string", index)))
				continue
			}
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items, issues
	case string:
		return stringList(typed), nil
	default:
		return nil, []ValidationIssue{invalidPolicyValueIssue(path, "policy value must be a string list")}
	}
}

func riskPolicyValue(values map[string]any, key, path string) (int, bool, []ValidationIssue) {
	value, ok := values[key]
	if !ok {
		return 0, false, nil
	}
	text, ok := value.(string)
	if !ok {
		return 0, true, []ValidationIssue{invalidPolicyValueIssue(path, "risk value must be a string")}
	}
	rank := riskRank(text)
	if rank == 0 {
		return 0, true, []ValidationIssue{invalidPolicyValueIssue(path, "risk value must be one of low, medium, high, critical")}
	}
	return rank, true, nil
}

func boolPolicyValue(values map[string]any, key, path string) (bool, bool, []ValidationIssue) {
	value, ok := values[key]
	if !ok {
		return false, false, nil
	}
	typed, ok := value.(bool)
	if !ok {
		return false, true, []ValidationIssue{invalidPolicyValueIssue(path, "policy value must be a boolean")}
	}
	return typed, true, nil
}

func invalidPolicyValueIssue(path, message string) ValidationIssue {
	return ValidationIssue{
		Code:    "invalid_policy_value",
		Path:    path,
		Message: message,
	}
}

func riskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = true
	}
	return set
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			trimmed := strings.TrimSpace(item)
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				continue
			}
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	default:
		return nil
	}
}

func validationResultMap(validation EffectiveConfigValidation) map[string]any {
	return map[string]any{
		"blocking_errors": validation.BlockingErrors,
		"warnings":        validation.Warnings,
	}
}

func validUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copied := *value
	return &copied
}

func trimOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
