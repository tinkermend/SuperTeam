package employee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository               Repository
	dispatcher               RuntimeCommandDispatcher
	provisioningTimeout      time.Duration
	provisioningPollInterval time.Duration
}

const (
	defaultProvisioningTimeout      = 10 * time.Second
	defaultProvisioningPollInterval = 250 * time.Millisecond
	defaultProvisioningAbortTimeout = 5 * time.Second
	maxWorkspaceFileInlineBytes     = 10 * 1024 * 1024
)

func NewService(repository Repository) (*Service, error) {
	return NewServiceWithProvisioning(repository, nil)
}

func NewServiceWithProvisioning(repository Repository, dispatcher RuntimeCommandDispatcher) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{
		repository:               repository,
		dispatcher:               dispatcher,
		provisioningTimeout:      defaultProvisioningTimeout,
		provisioningPollInterval: defaultProvisioningPollInterval,
	}, nil
}

func (s *Service) GetOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	req.Query = strings.TrimSpace(req.Query)
	if req.Status != "" && !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
	}
	if !req.ExecutionStatus.IsValid() {
		return nil, fmt.Errorf("%w: invalid execution_status", ErrInvalidInput)
	}
	if !req.RunStatus.IsValid() {
		return nil, fmt.Errorf("%w: invalid run_status", ErrInvalidInput)
	}
	if req.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must be non-negative", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	overview, err := s.repository.GetDigitalEmployeeOverview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get digital employee overview: %w", err)
	}
	if overview.Items == nil {
		overview.Items = []DigitalEmployeeOverviewItem{}
	}
	overview.Pagination.Limit = req.Limit
	overview.Pagination.Offset = req.Offset
	return overview, nil
}

func (s *Service) ListWorkspaceFiles(ctx context.Context, req ListWorkspaceFilesRequest) ([]WorkspaceFile, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	files, err := s.repository.ListWorkspaceFiles(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list workspace files: %w", err)
	}
	return append([]WorkspaceFile(nil), files...), nil
}

func (s *Service) UpsertWorkspaceFile(ctx context.Context, req UpsertWorkspaceFileRequest) (WorkspaceFile, error) {
	if req.TenantID == uuid.Nil {
		return WorkspaceFile{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return WorkspaceFile{}, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	normalizedPath, err := normalizeWorkspaceFilePath(req.Path)
	if err != nil {
		return WorkspaceFile{}, err
	}
	fileRole, err := normalizeWorkspaceFileRole(req.FileRole, normalizedPath)
	if err != nil {
		return WorkspaceFile{}, err
	}
	syncPolicy, err := normalizeWorkspaceFileSyncPolicy(req.SyncPolicy)
	if err != nil {
		return WorkspaceFile{}, err
	}
	mimeType := strings.TrimSpace(req.MimeType)
	if mimeType == "" {
		mimeType = inferWorkspaceFileMimeType(normalizedPath)
	}
	if strings.ContainsAny(mimeType, "\r\n\t") {
		return WorkspaceFile{}, fmt.Errorf("%w: invalid workspace file mime_type", ErrInvalidInput)
	}
	employee, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return WorkspaceFile{}, fmt.Errorf("get digital employee: %w", err)
	}
	if employee.TeamID == nil || *employee.TeamID == uuid.Nil {
		return WorkspaceFile{}, fmt.Errorf("%w: employee team_id is required for workspace files", ErrInvalidInput)
	}
	contentBytes := []byte(req.Content)
	if len(contentBytes) > maxWorkspaceFileInlineBytes {
		return WorkspaceFile{}, fmt.Errorf("%w: workspace file content is too large", ErrInvalidInput)
	}
	var file WorkspaceFile
	if err := s.repository.WithTransaction(ctx, func(repository Repository) error {
		var fileRecord WorkspaceFileRecord
		fileRecord, err = repository.GetWorkspaceFileByPath(ctx, req.TenantID, req.DigitalEmployeeID, normalizedPath)
		if err != nil {
			if !errors.Is(err, ErrNotFound) {
				return err
			}
			fileRecord, err = repository.CreateWorkspaceFile(ctx, CreateWorkspaceFileParams{
				TenantID:          req.TenantID,
				TeamID:            *employee.TeamID,
				DigitalEmployeeID: req.DigitalEmployeeID,
				Path:              normalizedPath,
				FileRole:          fileRole,
				MimeType:          mimeType,
				SyncPolicy:        syncPolicy,
				Status:            "active",
				Metadata: map[string]any{
					"source": "console",
				},
				CreatedBy: req.UpdatedBy,
			})
			if err != nil {
				return err
			}
		}
		revisionNumber, err := repository.GetNextWorkspaceFileRevisionNumber(ctx, req.TenantID, fileRecord.ID)
		if err != nil {
			return err
		}
		revision, err := repository.CreateWorkspaceFileRevision(ctx, CreateWorkspaceFileRevisionParams{
			TenantID:       req.TenantID,
			FileID:         fileRecord.ID,
			RevisionNumber: revisionNumber,
			ContentText:    req.Content,
			ContentHash:    sha256Hex(req.Content),
			SizeBytes:      int32(len(contentBytes)),
			StorageBackend: "db",
			CreatedBy:      req.UpdatedBy,
			ChangeNote:     req.ChangeNote,
			Metadata: map[string]any{
				"source": "console",
			},
		})
		if err != nil {
			return err
		}
		fileRecord, err = repository.ActivateWorkspaceFileRevision(ctx, req.TenantID, fileRecord.ID, revision.ID)
		if err != nil {
			return err
		}
		file = workspaceFileFromRecords(fileRecord, revision)
		return nil
	}); err != nil {
		return WorkspaceFile{}, fmt.Errorf("upsert workspace file: %w", err)
	}
	return file, nil
}

func (s *Service) GetCreateOptions(ctx context.Context, req CreateOptionsRequest) (*CreateOptions, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if err := s.repository.EnsureTeamExists(ctx, req.TenantID, req.TeamID); err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	teamConfig, err := s.repository.GetCurrentTeamConfigRevision(ctx, req.TenantID, req.TeamID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: active team governance config is required", ErrEffectiveConfigRequired)
		}
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	teamConfigOption, err := teamConfigCreateOption(teamConfig)
	if err != nil {
		return nil, err
	}
	employeeTypes, err := employeeTypesForTeamConfig(teamConfig)
	if err != nil {
		return nil, err
	}
	runtimeOptions, err := s.repository.ListRuntimeProviderOptionsForCreate(ctx, req.TenantID, req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("list runtime provider options: %w", err)
	}
	capabilityOptions := capabilityOptionsFromTeamConfig(teamConfig)

	return &CreateOptions{
		TeamConfig:             teamConfigOption,
		EmployeeTypes:          employeeTypes,
		CapabilityOptions:      capabilityOptions,
		RuntimeProviderOptions: append([]RuntimeProviderOption(nil), runtimeOptions...),
		CreationChecks: createOptionChecks(
			teamConfigOption,
			employeeTypes,
			capabilityOptions,
			runtimeOptions,
		),
		PolicyDefaults: emptyPolicyDefaults(),
	}, nil
}

func createOptionChecks(
	teamConfig TeamConfigCreateOption,
	employeeTypes []EmployeeTypeDefinition,
	capabilityOptions CapabilityOptions,
	runtimeOptions []RuntimeProviderOption,
) []CreateOptionCheck {
	availableRuntimeCount := 0
	for _, option := range runtimeOptions {
		if option.Available {
			availableRuntimeCount++
		}
	}

	capabilityCount := len(capabilityOptions.Skills) + len(capabilityOptions.MCPServers) + len(capabilityOptions.ExternalCapabilities)

	return []CreateOptionCheck{
		{
			Key:     "team_governance",
			Label:   "团队治理版本",
			Status:  checkStatus(teamConfig.Status == TeamConfigRevisionStatusActive, false),
			Message: fmt.Sprintf("#%d %s", teamConfig.RevisionNumber, teamConfig.Status),
		},
		{
			Key:     "employee_templates",
			Label:   "专业模板",
			Status:  checkStatus(len(employeeTypes) > 0, false),
			Message: fmt.Sprintf("%d 个可用模板", len(employeeTypes)),
		},
		{
			Key:     "capability_policy",
			Label:   "能力边界",
			Status:  checkStatus(capabilityCount > 0 || len(capabilityOptions.ProviderTypes) > 0, false),
			Message: fmt.Sprintf("技能 %d · MCP %d · 外部能力 %d", len(capabilityOptions.Skills), len(capabilityOptions.MCPServers), len(capabilityOptions.ExternalCapabilities)),
		},
		{
			Key:     "runtime_provider",
			Label:   "Runtime 可用",
			Status:  checkStatus(availableRuntimeCount > 0, false),
			Message: fmt.Sprintf("%d/%d 个运行绑定可用", availableRuntimeCount, len(runtimeOptions)),
		},
	}
}

func checkStatus(passed bool, warning bool) string {
	if passed {
		return "passed"
	}
	if warning {
		return "warning"
	}
	return "blocked"
}

func teamConfigCreateOption(teamConfig TeamConfigInput) (TeamConfigCreateOption, error) {
	allowedEmployeeTypes, err := allowedEmployeeTypesFromTeamConfig(teamConfig)
	if err != nil {
		return TeamConfigCreateOption{}, err
	}
	allowedProviderTypes := firstNonEmptyStringList(
		optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_provider_types"),
		optionalStringListFromPolicy(teamConfig.RuntimeScopePolicy, "allowed_provider_types", "provider_types"),
	)
	return TeamConfigCreateOption{
		ID:                          teamConfig.ID,
		TenantID:                    teamConfig.TenantID,
		TeamID:                      teamConfig.TeamID,
		RevisionNumber:              teamConfig.RevisionNumber,
		Status:                      teamConfig.Status,
		AllowedEmployeeTypes:        cloneStringSlice(allowedEmployeeTypes),
		AllowedProviderTypes:        cloneStringSlice(allowedProviderTypes),
		AllowedSkills:               optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_skills"),
		AllowedMCPServers:           optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_mcp_servers"),
		AllowedExternalCaps:         optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_external_capabilities"),
		CapabilityPolicy:            cloneMap(teamConfig.CapabilityPolicy),
		ContextPolicy:               cloneMap(teamConfig.ContextPolicy),
		ApprovalPolicy:              cloneMap(teamConfig.ApprovalPolicy),
		ArtifactContract:            cloneMap(teamConfig.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(teamConfig.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(teamConfig.RuntimeScopePolicy),
	}, nil
}

func capabilityOptionsFromTeamConfig(teamConfig TeamConfigInput) CapabilityOptions {
	return CapabilityOptions{
		ProviderTypes: firstNonEmptyStringList(
			optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_provider_types"),
			optionalStringListFromPolicy(teamConfig.RuntimeScopePolicy, "allowed_provider_types", "provider_types"),
		),
		Skills:               optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_skills"),
		MCPServers:           optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_mcp_servers"),
		ExternalCapabilities: optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_external_capabilities"),
	}
}

func employeeTypesForTeamConfig(teamConfig TeamConfigInput) ([]EmployeeTypeDefinition, error) {
	allowedTypes, err := allowedEmployeeTypesFromTeamConfig(teamConfig)
	if err != nil {
		return nil, err
	}
	defaultTypes := DefaultEmployeeTypeDefinitions()
	if len(allowedTypes) == 0 {
		filtered := make([]EmployeeTypeDefinition, 0, len(defaultTypes))
		for _, definition := range defaultTypes {
			filtered = append(filtered, employeeTypeDefinitionForTeamConfig(definition, teamConfig))
		}
		return filtered, nil
	}
	allowedSet := stringSet(allowedTypes)
	filtered := make([]EmployeeTypeDefinition, 0, len(defaultTypes))
	for _, definition := range defaultTypes {
		if allowedSet[definition.Type] {
			filtered = append(filtered, employeeTypeDefinitionForTeamConfig(definition, teamConfig))
		}
	}
	return filtered, nil
}

func employeeTypeDefinitionForTeamConfig(definition EmployeeTypeDefinition, teamConfig TeamConfigInput) EmployeeTypeDefinition {
	filtered := cloneEmployeeTypeDefinition(definition)
	filtered.DefaultCapabilitySelection = constrainedDefaultCapabilitySelection(definition.DefaultCapabilitySelection, teamConfig)
	filtered.DefaultContextPolicyOverride = constrainedDefaultContextPolicyOverride(definition.DefaultContextPolicyOverride, teamConfig)
	return filtered
}

func allowedEmployeeTypesFromTeamConfig(teamConfig TeamConfigInput) ([]string, error) {
	values, present, issues := stringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_employee_types")
	if len(issues) != 0 {
		return nil, fmt.Errorf("%w: invalid capability_policy.allowed_employee_types", ErrInvalidInput)
	}
	if present {
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: capability_policy.allowed_employee_types must not be empty", ErrInvalidInput)
		}
		return values, nil
	}
	values, present, issues = stringListFromPolicy(teamConfig.RuntimeScopePolicy, "allowed_employee_types", "employee_types")
	if len(issues) != 0 {
		return nil, fmt.Errorf("%w: invalid runtime_scope_policy employee type allowlist", ErrInvalidInput)
	}
	if present {
		if len(values) == 0 {
			return nil, fmt.Errorf("%w: runtime_scope_policy employee type allowlist must not be empty", ErrInvalidInput)
		}
		return values, nil
	}
	return nil, nil
}

func stringListFromAnyPolicy(value any) []string {
	return stringList(value)
}

func stringListFromPolicy(policy map[string]any, keys ...string) ([]string, bool, []ValidationIssue) {
	for _, key := range keys {
		if _, ok := policy[key]; !ok {
			continue
		}
		values, issues := stringListPolicyValue(policy, key, key)
		if len(issues) != 0 {
			return nil, true, issues
		}
		return stringListFromAnyPolicy(values), true, nil
	}
	return nil, false, nil
}

func optionalStringListFromPolicy(policy map[string]any, keys ...string) []string {
	values, _, issues := stringListFromPolicy(policy, keys...)
	if len(issues) != 0 {
		return nil
	}
	return values
}

func firstNonEmptyStringList(candidates ...[]string) []string {
	for _, candidate := range candidates {
		if len(candidate) != 0 {
			return cloneStringSlice(candidate)
		}
	}
	return nil
}

func emptyPolicyDefaults() PolicyDefaults {
	return PolicyDefaults{
		PermissionPolicy:      map[string]any{},
		ContextPolicyOverride: map[string]any{},
		ApprovalPolicy:        map[string]any{},
		CapabilitySelection:   map[string]any{},
		RuntimeSelector:       map[string]any{},
		WorkspacePolicy:       map[string]any{},
		SessionPolicy:         map[string]any{},
		Metadata:              map[string]any{},
	}
}

func (s *Service) CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error) {
	normalized, definition, err := normalizeCreateDigitalEmployeeRequest(req)
	if err != nil {
		return nil, err
	}
	teamID := *normalized.TeamID
	if err := s.repository.EnsureTeamExists(ctx, normalized.TenantID, teamID); err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	teamConfig, err := s.repository.GetCurrentTeamConfigRevision(ctx, normalized.TenantID, teamID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: active team governance config is required", ErrEffectiveConfigRequired)
		}
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	if err := validateEmployeeTypeAllowedByTeamConfig(normalized.EmployeeType, teamConfig); err != nil {
		return nil, err
	}
	if err := s.validateInitialEffectiveConfig(ctx, s.repository, normalized, definition, teamConfig, uuid.New()); err != nil {
		return nil, err
	}

	preflight, err := s.repository.GetRuntimeProvisioningPreflight(ctx, normalized.TenantID, teamID, normalized.RuntimeNodeID, normalized.ProviderType)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("%w: runtime provisioning preflight unavailable", ErrRuntimeUnavailable)
		}
		return nil, fmt.Errorf("get runtime provisioning preflight: %w", err)
	}
	if err := validateRuntimeProvisioningPreflight(preflight); err != nil {
		return nil, err
	}
	if s.dispatcher == nil {
		return nil, fmt.Errorf("%w: runtime command dispatcher is required", ErrRuntimeUnavailable)
	}
	if !s.dispatcher.IsConnected(preflight.NodeID) {
		return nil, fmt.Errorf("%w: runtime node is not connected", ErrRuntimeUnavailable)
	}

	var record DigitalEmployeeRecord
	var instance DigitalEmployeeExecutionInstanceRecord
	var commandID string
	var payload map[string]any
	if err := s.repository.WithTransaction(ctx, func(txRepo Repository) error {
		createdRecord, createdInstance, createdCommandID, createdPayload, err := s.createLocalReadyEmployeeFacts(ctx, txRepo, normalized, definition, teamConfig, preflight)
		if err != nil {
			return err
		}
		record = createdRecord
		instance = createdInstance
		commandID = createdCommandID
		payload = createdPayload
		return nil
	}); err != nil {
		return nil, err
	}

	if err := dispatchRuntimeProvisioningCommand(ctx, s.dispatcher, preflight.NodeID, commandID, payload); err != nil {
		abortErr := s.abortProvisioning(req.TenantID, record.ID, instance.ID, "dispatch provisioning command failed: "+err.Error())
		return nil, provisioningErrorWithAbort(err, abortErr)
	}

	receipt, err := s.waitForProvisioningCompletion(ctx, normalized.TenantID, commandID)
	if err != nil {
		abortErr := s.abortProvisioning(normalized.TenantID, record.ID, instance.ID, "wait for provisioning command completion failed: "+err.Error())
		return nil, provisioningErrorWithAbort(fmt.Errorf("%w: wait for provisioning command completion: %w", ErrRuntimeUnavailable, err), abortErr)
	}
	if receipt == nil {
		abortErr := s.abortProvisioning(normalized.TenantID, record.ID, instance.ID, "provisioning command receipt missing")
		return nil, provisioningErrorWithAbort(fmt.Errorf("%w: provisioning command receipt missing", ErrRuntimeUnavailable), abortErr)
	}
	switch receipt.Status {
	case string(DigitalEmployeeRunStatusCompleted):
		readyRecord, err := s.repository.GetDigitalEmployee(ctx, normalized.TenantID, record.ID)
		if err != nil {
			return nil, fmt.Errorf("get provisioned digital employee: %w", err)
		}
		return employeeFromRecord(readyRecord), nil
	case string(DigitalEmployeeRunStatusFailed), string(DigitalEmployeeRunStatusTimedOut), string(DigitalEmployeeRunStatusCancelled):
		abortErr := s.abortProvisioning(normalized.TenantID, record.ID, instance.ID, "provisioning command "+receipt.Status)
		return nil, provisioningErrorWithAbort(fmt.Errorf("%w: provisioning command %s", ErrRuntimeUnavailable, receipt.Status), abortErr)
	default:
		abortErr := s.abortProvisioning(normalized.TenantID, record.ID, instance.ID, "provisioning command did not reach terminal status")
		return nil, provisioningErrorWithAbort(fmt.Errorf("%w: provisioning command did not reach terminal status %q", ErrRuntimeUnavailable, receipt.Status), abortErr)
	}
}

func normalizeCreateDigitalEmployeeRequest(req CreateDigitalEmployeeRequest) (CreateDigitalEmployeeRequest, EmployeeTypeDefinition, error) {
	if req.TenantID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == nil || *req.TeamID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.OwnerUserID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: owner_user_id is required", ErrInvalidInput)
	}
	employeeType := strings.ToLower(strings.TrimSpace(req.EmployeeType))
	if employeeType == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: employee_type is required", ErrInvalidInput)
	}
	definition, ok := EmployeeTypeDefinitionByType(employeeType)
	if !ok {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: unknown employee_type %q", ErrInvalidInput, employeeType)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	avatarAssetID := normalizeAvatarAssetID(req.AvatarAssetID)
	if avatarAssetID == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: avatar_asset_id is required", ErrInvalidInput)
	}
	avatarAsset, ok := DigitalEmployeeAvatarAssetByID(avatarAssetID)
	if !ok {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: unknown avatar_asset_id %q", ErrInvalidInput, avatarAssetID)
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = strings.TrimSpace(definition.DefaultRole)
	}
	if role == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: role is required", ErrInvalidInput)
	}
	if req.RuntimeNodeID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: runtime_node_id is required", ErrInvalidInput)
	}
	providerType := strings.TrimSpace(req.ProviderType)
	if providerType == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
	}
	riskLevel := strings.TrimSpace(req.RiskLevel)
	if riskLevel == "" {
		riskLevel = defaultRiskLevelForEmployeeType(definition)
	}
	budgetPolicy, err := normalizeBudgetPolicy(req.BudgetPolicy)
	if err != nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, err
	}
	req.EmployeeType = employeeType
	req.Name = name
	req.AvatarAssetID = avatarAsset.ID
	req.Role = role
	req.Description = trimOptionalString(req.Description)
	req.RiskLevel = riskLevel
	req.ProviderType = providerType
	req.BudgetPolicy = budgetPolicy
	req.Metadata = metadataWithAvatarAsset(req.Metadata, avatarAsset)
	return req, definition, nil
}

func defaultRiskLevelForEmployeeType(definition EmployeeTypeDefinition) string {
	if value, ok := definition.DefaultApprovalPolicy["min_risk_for_human"].(string); ok && riskRank(value) > 0 {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return "medium"
}

func validateEmployeeTypeAllowedByTeamConfig(employeeType string, teamConfig TeamConfigInput) error {
	allowedTypes, err := allowedEmployeeTypesFromTeamConfig(teamConfig)
	if err != nil {
		return err
	}
	if len(allowedTypes) == 0 {
		return nil
	}
	if !stringSet(allowedTypes)[employeeType] {
		return fmt.Errorf("%w: employee_type %q is outside team policy", ErrInvalidInput, employeeType)
	}
	return nil
}

func (s *Service) validateInitialEffectiveConfig(ctx context.Context, repository Repository, req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput, employeeID uuid.UUID) error {
	configInput := initialEmployeeConfigInput(req, definition, teamConfig, employeeID, uuid.New(), 1)
	preview, err := s.previewEffectiveConfigWithRepository(ctx, repository, teamConfig, configInput)
	if err != nil {
		return err
	}
	if len(preview.Validation.BlockingErrors) > 0 {
		return fmt.Errorf("%w: effective config has blocking validation errors", ErrInvalidInput)
	}
	return nil
}

func (s *Service) previewEffectiveConfigWithRepository(ctx context.Context, repository Repository, teamConfig TeamConfigInput, employeeConfig EmployeeConfigInput) (*EffectiveConfigPreview, error) {
	txService := *s
	txService.repository = repository
	return txService.PreviewEffectiveConfig(ctx, PreviewEffectiveConfigRequest{
		TenantID:          employeeConfig.TenantID,
		DigitalEmployeeID: employeeConfig.DigitalEmployeeID,
		TeamConfig:        teamConfig,
		EmployeeConfig:    employeeConfig,
	})
}

func (s *Service) createLocalReadyEmployeeFacts(ctx context.Context, repository Repository, req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput, preflight RuntimeProvisioningPreflight) (DigitalEmployeeRecord, DigitalEmployeeExecutionInstanceRecord, string, map[string]any, error) {
	record, err := repository.CreateDigitalEmployee(ctx, createDigitalEmployeeParams(req))
	if err != nil {
		return DigitalEmployeeRecord{}, DigitalEmployeeExecutionInstanceRecord{}, "", nil, fmt.Errorf("create digital employee: %w", err)
	}
	configRevision, err := s.createInitialActiveConfigRevision(ctx, repository, record, req, definition, teamConfig)
	if err != nil {
		return DigitalEmployeeRecord{}, DigitalEmployeeExecutionInstanceRecord{}, "", nil, err
	}
	configInput := employeeConfigInputFromRecord(configRevision)
	preview, err := s.previewEffectiveConfigWithRepository(ctx, repository, teamConfig, configInput)
	if err != nil {
		return DigitalEmployeeRecord{}, DigitalEmployeeExecutionInstanceRecord{}, "", nil, err
	}
	if len(preview.Validation.BlockingErrors) > 0 {
		return DigitalEmployeeRecord{}, DigitalEmployeeExecutionInstanceRecord{}, "", nil, fmt.Errorf("%w: effective config has blocking validation errors", ErrInvalidInput)
	}
	if _, err := createApprovedEffectiveConfig(ctx, repository, record, teamConfig.ID, configRevision.ID, preview, req.OwnerUserID); err != nil {
		return DigitalEmployeeRecord{}, DigitalEmployeeExecutionInstanceRecord{}, "", nil, err
	}
	instance, commandID, payload, err := createProvisioningInstanceAndReceipt(ctx, repository, record, req, preflight, configInput, preview)
	if err != nil {
		return DigitalEmployeeRecord{}, DigitalEmployeeExecutionInstanceRecord{}, "", nil, err
	}
	return record, instance, commandID, payload, nil
}

func createDigitalEmployeeParams(req CreateDigitalEmployeeRequest) CreateDigitalEmployeeParams {
	return CreateDigitalEmployeeParams{
		TenantID:         req.TenantID,
		TeamID:           validUUIDPtr(req.TeamID),
		OwnerUserID:      req.OwnerUserID,
		EmployeeType:     req.EmployeeType,
		Name:             req.Name,
		Role:             req.Role,
		Description:      req.Description,
		Status:           DigitalEmployeeStatusDraft,
		PermissionPolicy: cloneMap(req.PermissionPolicy),
		ContextPolicy:    cloneMap(req.ContextPolicy),
		ApprovalPolicy:   cloneMap(req.ApprovalPolicy),
		RiskLevel:        req.RiskLevel,
		Metadata:         cloneMap(req.Metadata),
	}
}

func (s *Service) createInitialActiveConfigRevision(ctx context.Context, repository Repository, record DigitalEmployeeRecord, req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput) (DigitalEmployeeConfigRevisionRecord, error) {
	nextRevision, err := repository.GetNextDigitalEmployeeConfigRevisionNumber(ctx, req.TenantID, record.ID)
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, fmt.Errorf("get next digital employee config revision number: %w", err)
	}
	if nextRevision <= 0 {
		nextRevision = 1
	}
	approvedBy := req.OwnerUserID
	now := time.Now().UTC()
	params := initialEmployeeConfigParams(req, definition, teamConfig, record.ID, nextRevision, approvedBy, now)
	revision, err := repository.CreateDigitalEmployeeConfigRevision(ctx, params)
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, fmt.Errorf("create initial digital employee config revision: %w", err)
	}
	return revision, nil
}

func initialEmployeeConfigParams(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput, employeeID uuid.UUID, revisionNumber int32, approvedBy uuid.UUID, approvedAt time.Time) CreateConfigRevisionParams {
	return CreateConfigRevisionParams{
		TenantID:               req.TenantID,
		DigitalEmployeeID:      employeeID,
		RevisionNumber:         revisionNumber,
		RoleProfile:            initialRoleProfile(req),
		ConstitutionAddendum:   cloneMap(req.ConstitutionAddendum),
		CapabilitySelection:    initialCapabilitySelection(req, definition, teamConfig),
		ContextPolicyOverride:  initialContextPolicyOverride(req, definition, teamConfig),
		ApprovalPolicyOverride: mergePolicyMaps(definition.DefaultApprovalPolicy, req.ApprovalPolicyOverride),
		BudgetPolicy:           cloneMap(req.BudgetPolicy),
		OutputContractAddendum: cloneMap(req.OutputContractAddendum),
		Status:                 ConfigRevisionStatusActive,
		ApprovedBy:             &approvedBy,
		ApprovedAt:             &approvedAt,
	}
}

func initialEmployeeConfigInput(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput, employeeID, configID uuid.UUID, revisionNumber int32) EmployeeConfigInput {
	return EmployeeConfigInput{
		ID:                     configID,
		TenantID:               req.TenantID,
		DigitalEmployeeID:      employeeID,
		RevisionNumber:         revisionNumber,
		RoleProfile:            initialRoleProfile(req),
		ConstitutionAddendum:   cloneMap(req.ConstitutionAddendum),
		CapabilitySelection:    initialCapabilitySelection(req, definition, teamConfig),
		ContextPolicyOverride:  initialContextPolicyOverride(req, definition, teamConfig),
		ApprovalPolicyOverride: mergePolicyMaps(definition.DefaultApprovalPolicy, req.ApprovalPolicyOverride),
		BudgetPolicy:           cloneMap(req.BudgetPolicy),
		OutputContractAddendum: cloneMap(req.OutputContractAddendum),
	}
}

func createApprovedEffectiveConfig(ctx context.Context, repository Repository, record DigitalEmployeeRecord, teamConfigRevisionID, employeeConfigRevisionID uuid.UUID, preview *EffectiveConfigPreview, approvedBy uuid.UUID) (DigitalEmployeeEffectiveConfigRecord, error) {
	now := time.Now().UTC()
	params := CreateEffectiveConfigParams{
		TenantID:                 record.TenantID,
		DigitalEmployeeID:        record.ID,
		TeamConfigRevisionID:     teamConfigRevisionID,
		EmployeeConfigRevisionID: employeeConfigRevisionID,
		EffectiveConfig:          cloneMap(preview.EffectiveConfig),
		ValidationResult:         validationResultMap(preview.Validation),
		Status:                   EffectiveConfigStatusApproved,
		ApprovedBy:               &approvedBy,
		ApprovedAt:               &now,
	}
	effectiveConfig, err := repository.CreateDigitalEmployeeEffectiveConfig(ctx, params)
	if err != nil {
		return DigitalEmployeeEffectiveConfigRecord{}, fmt.Errorf("create approved digital employee effective config: %w", err)
	}
	return effectiveConfig, nil
}

func createProvisioningInstanceAndReceipt(ctx context.Context, repository Repository, record DigitalEmployeeRecord, req CreateDigitalEmployeeRequest, preflight RuntimeProvisioningPreflight, configInput EmployeeConfigInput, preview *EffectiveConfigPreview) (DigitalEmployeeExecutionInstanceRecord, string, map[string]any, error) {
	agentHomeDir := canonicalEmployeeHome(preflight.AgentHomeDir, preflight.TeamID, record.ID)
	instance, err := repository.UpsertDigitalEmployeeExecutionInstance(ctx, UpsertExecutionInstanceParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: record.ID,
		RuntimeNodeID:     req.RuntimeNodeID,
		ProviderType:      req.ProviderType,
		AgentHomeDir:      agentHomeDir,
		WorkspacePolicy:   cloneMap(req.WorkspacePolicy),
		SessionPolicy:     cloneMap(req.SessionPolicy),
		RuntimeSelector: map[string]any{
			"runtime_node_id": preflight.RuntimeNodeID.String(),
			"node_id":         preflight.NodeID,
		},
		Status: ExecutionInstanceStatusProvisioning,
		Metadata: map[string]any{
			"provisioned_by": "digital_employee_create",
		},
	})
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, "", nil, fmt.Errorf("create digital employee execution instance: %w", err)
	}

	workspaceFiles, err := createDefaultAgentsWorkspaceFile(ctx, repository, record, preflight.TeamID, configInput, preview)
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, "", nil, err
	}

	commandID := newRuntimeCommandID()
	payload := buildProvisionInstancePayload(commandID, record, instance, req.ProviderType, preflight, req, configInput, preview, workspaceFiles)
	if err := repository.CreateRuntimeCommandReceipt(ctx, CreateRuntimeCommandReceiptRequest{
		TenantID:      req.TenantID,
		CommandID:     commandID,
		CommandType:   "provision_instance",
		RuntimeNodeID: req.RuntimeNodeID,
		NodeID:        preflight.NodeID,
		ResourceType:  "digital_employee_execution_instance",
		ResourceID:    instance.ID,
		Status:        "pending",
		Payload:       payload,
	}); err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, "", nil, fmt.Errorf("create provisioning command receipt: %w", err)
	}
	return instance, commandID, payload, nil
}

func createDefaultAgentsWorkspaceFile(ctx context.Context, repository Repository, employee DigitalEmployeeRecord, teamID uuid.UUID, configInput EmployeeConfigInput, preview *EffectiveConfigPreview) ([]WorkspaceFileForSyncRecord, error) {
	agentsPath, err := normalizeWorkspaceFilePath("AGENTS.md")
	if err != nil {
		return nil, err
	}
	agentsContent := buildDefaultAgentsContent(employee, configInput, preview)
	agentsRevisionHash := sha256Hex(agentsContent)
	agentsFile, err := repository.CreateWorkspaceFile(ctx, CreateWorkspaceFileParams{
		TenantID:          employee.TenantID,
		TeamID:            teamID,
		DigitalEmployeeID: employee.ID,
		Path:              agentsPath,
		FileRole:          "entrypoint",
		MimeType:          "text/markdown",
		SyncPolicy:        "auto",
		Status:            "active",
		Metadata:          map[string]any{"created_by": "digital_employee_create"},
	})
	if err != nil {
		return nil, fmt.Errorf("create default workspace file: %w", err)
	}
	agentsRevision, err := repository.CreateWorkspaceFileRevision(ctx, CreateWorkspaceFileRevisionParams{
		TenantID:       employee.TenantID,
		FileID:         agentsFile.ID,
		RevisionNumber: 1,
		ContentText:    agentsContent,
		ContentHash:    agentsRevisionHash,
		SizeBytes:      int32(len([]byte(agentsContent))),
		StorageBackend: "db",
		Metadata:       map[string]any{"source": "default_agents"},
	})
	if err != nil {
		return nil, fmt.Errorf("create default workspace file revision: %w", err)
	}
	agentsFile, err = repository.ActivateWorkspaceFileRevision(ctx, employee.TenantID, agentsFile.ID, agentsRevision.ID)
	if err != nil {
		return nil, fmt.Errorf("activate default workspace file revision: %w", err)
	}
	return []WorkspaceFileForSyncRecord{workspaceFileForSyncFromDefault(agentsFile, agentsRevision)}, nil
}

func dispatchRuntimeProvisioningCommand(ctx context.Context, dispatcher RuntimeCommandDispatcher, nodeID, commandID string, payload map[string]any) error {
	command, err := runtimeCommand(commandID, "provision_instance", payload)
	if err != nil {
		return err
	}
	if err := dispatcher.Dispatch(ctx, nodeID, command); err != nil {
		return fmt.Errorf("%w: dispatch provision instance: %w", ErrRuntimeUnavailable, err)
	}
	return nil
}

func (s *Service) waitForProvisioningCompletion(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	waitCtx, cancel := context.WithTimeout(ctx, s.provisioningTimeout)
	defer cancel()
	return s.repository.WaitForRuntimeCommandCompletion(waitCtx, tenantID, commandID, s.provisioningPollInterval)
}

func initialCapabilitySelection(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput) map[string]any {
	defaults := constrainedDefaultCapabilitySelection(definition.DefaultCapabilitySelection, teamConfig)
	return mergePolicyMaps(defaults, req.CapabilitySelection)
}

func initialContextPolicyOverride(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput) map[string]any {
	defaults := constrainedDefaultContextPolicyOverride(definition.DefaultContextPolicyOverride, teamConfig)
	return mergePolicyMaps(defaults, req.ContextPolicyOverride)
}

func constrainedDefaultCapabilitySelection(defaults map[string]any, teamConfig TeamConfigInput) map[string]any {
	selection := cloneMap(defaults)
	filterDefaultStringListByPolicy(selection, "enabled_skills", teamConfig.CapabilityPolicy, "allowed_skills")
	filterDefaultStringListByPolicy(selection, "enabled_mcp_servers", teamConfig.CapabilityPolicy, "allowed_mcp_servers")
	filterDefaultStringListByPolicy(selection, "enabled_plugins", teamConfig.CapabilityPolicy, "allowed_plugins")
	filterDefaultStringListByPolicy(selection, "enabled_external_capabilities", teamConfig.CapabilityPolicy, "allowed_external_capabilities")
	filterDefaultStringListByPolicy(selection, "enabled_provider_types", teamConfig.CapabilityPolicy, "allowed_provider_types")
	return selection
}

func constrainedDefaultContextPolicyOverride(defaults map[string]any, teamConfig TeamConfigInput) map[string]any {
	override := cloneMap(defaults)
	filterDefaultStringListByPolicy(override, "sources", teamConfig.ContextPolicy, "sources", "allowed_sources")
	filterDefaultStringListByPolicy(override, "knowledge_bases", teamConfig.ContextPolicy, "knowledge_bases", "allowed_knowledge_bases")
	filterDefaultStringListByPolicy(override, "documents", teamConfig.ContextPolicy, "documents", "allowed_documents")
	filterDefaultStringListByPolicy(override, "repositories", teamConfig.ContextPolicy, "repositories", "allowed_repositories")
	filterDefaultStringListByPolicy(override, "log_sources", teamConfig.ContextPolicy, "log_sources", "allowed_log_sources")
	return override
}

func filterDefaultStringListByPolicy(values map[string]any, valueKey string, policy map[string]any, policyKeys ...string) {
	current, currentIssues := stringListPolicyValue(values, valueKey, valueKey)
	if len(currentIssues) != 0 || len(current) == 0 {
		delete(values, valueKey)
		return
	}
	allowed, present, allowedIssues := firstStringListPolicyValue(policy, policyKeys...)
	if !present || len(allowedIssues) != 0 || len(allowed) == 0 {
		delete(values, valueKey)
		return
	}
	allowedSet := stringSet(allowed)
	filtered := make([]string, 0, len(current))
	for _, item := range current {
		if allowedSet[item] {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		delete(values, valueKey)
		return
	}
	values[valueKey] = filtered
}

func mergePolicyMaps(base, override map[string]any) map[string]any {
	merged := cloneMap(base)
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func initialRoleProfile(req CreateDigitalEmployeeRequest) map[string]any {
	profile := cloneMap(req.RoleProfile)
	profile["employee_type"] = req.EmployeeType
	profile["role"] = req.Role
	return profile
}

func (s *Service) abortProvisioning(tenantID, employeeID, executionInstanceID uuid.UUID, reason string) error {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), defaultProvisioningAbortTimeout)
	defer cancel()
	return s.repository.AbortProvisionedDigitalEmployee(cleanupCtx, tenantID, employeeID, executionInstanceID, reason)
}

func validateRuntimeProvisioningPreflight(preflight RuntimeProvisioningPreflight) error {
	if preflight.TenantID == uuid.Nil {
		return fmt.Errorf("%w: provisioning tenant_id is required", ErrRuntimeUnavailable)
	}
	if preflight.TeamID == uuid.Nil {
		return fmt.Errorf("%w: provisioning team_id is required", ErrRuntimeUnavailable)
	}
	if preflight.RuntimeNodeID == uuid.Nil || strings.TrimSpace(preflight.NodeID) == "" {
		return fmt.Errorf("%w: runtime node is unavailable", ErrRuntimeUnavailable)
	}
	if !preflight.HasActiveTeamConfig {
		return fmt.Errorf("%w: active team governance config is required before provisioning", ErrEffectiveConfigRequired)
	}
	if !preflight.RuntimeOnline {
		return fmt.Errorf("%w: runtime node is not online", ErrRuntimeUnavailable)
	}
	if !preflight.EnrollmentApproved {
		return fmt.Errorf("%w: runtime enrollment is not approved", ErrRuntimeUnavailable)
	}
	if !preflight.RuntimeSessionActive {
		return fmt.Errorf("%w: runtime session is not active", ErrRuntimeUnavailable)
	}
	if !preflight.ProviderAvailable {
		return fmt.Errorf("%w: provider capability is unavailable", ErrProviderUnavailable)
	}
	if !preflight.ProviderPolicyAllowed {
		return fmt.Errorf("%w: provider type is outside team capability policy", ErrProviderUnavailable)
	}
	if !preflight.RuntimePolicyAllowed {
		return fmt.Errorf("%w: runtime node is outside team runtime policy", ErrRuntimeUnavailable)
	}
	if strings.TrimSpace(preflight.AgentHomeDir) == "" {
		return fmt.Errorf("%w: runtime agent home dir is unavailable", ErrProviderUnavailable)
	}
	return nil
}

func buildProvisionInstancePayload(commandID string, employee DigitalEmployeeRecord, instance DigitalEmployeeExecutionInstanceRecord, providerType string, preflight RuntimeProvisioningPreflight, req CreateDigitalEmployeeRequest, configInput EmployeeConfigInput, preview *EffectiveConfigPreview, workspaceFiles []WorkspaceFileForSyncRecord) map[string]any {
	return redactRuntimeEventPayload(map[string]any{
		"command_id":                  commandID,
		"digital_employee_id":         employee.ID.String(),
		"execution_instance_id":       instance.ID.String(),
		"tenant_id":                   employee.TenantID.String(),
		"team_id":                     preflight.TeamID.String(),
		"owner_user_id":               employee.OwnerUserID.String(),
		"employee_type":               employee.EmployeeType,
		"role":                        employee.Role,
		"risk_level":                  employee.RiskLevel,
		"runtime_node_id":             preflight.RuntimeNodeID.String(),
		"node_id":                     preflight.NodeID,
		"provider_type":               providerType,
		"provider_run_protocol":       providerRunProtocol,
		"agent_home_dir":              instance.AgentHomeDir,
		"team_config_revision_id":     preview.TeamConfigRevisionID.String(),
		"employee_config_revision_id": preview.EmployeeConfigRevisionID.String(),
		"governance_snapshot":         cloneMap(preflight.GovernanceSnapshot),
		"session_policy":              cloneMap(req.SessionPolicy),
		"workspace_policy":            cloneMap(req.WorkspacePolicy),
		"permission_policy":           cloneMap(employee.PermissionPolicy),
		"context_policy":              cloneMap(employee.ContextPolicy),
		"approval_policy":             cloneMap(employee.ApprovalPolicy),
		"role_profile":                cloneMap(configInput.RoleProfile),
		"context_policy_override":     cloneMap(configInput.ContextPolicyOverride),
		"approval_policy_override":    cloneMap(configInput.ApprovalPolicyOverride),
		"capability_selection":        cloneMap(configInput.CapabilitySelection),
		"budget_policy":               cloneMap(configInput.BudgetPolicy),
		"output_contract_addendum":    cloneMap(configInput.OutputContractAddendum),
		"employee_metadata":           cloneMap(employee.Metadata),
		"execution_instance_ref":      instance.ID.String(),
		"workspace_files":             runtimeWorkspaceFilesPayload(workspaceFiles),
		"skills":                      runtimeSkillsPayload(configInput.CapabilitySelection),
		"mcp_servers":                 runtimeMCPServersPayload(configInput.CapabilitySelection),
	})
}

func canonicalEmployeeHome(workspaceBaseDir string, teamID, digitalEmployeeID uuid.UUID) string {
	base := strings.TrimRight(strings.TrimSpace(workspaceBaseDir), "/")
	return base + "/teams/" + teamID.String() + "/employees/" + digitalEmployeeID.String()
}

func normalizeWorkspaceFilePath(path string) (string, error) {
	value := strings.TrimSpace(path)
	if value == "" || strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") {
		return "", fmt.Errorf("%w: invalid workspace file path", ErrInvalidInput)
	}
	if strings.Contains(value, "\\") || strings.Contains(value, "\x00") {
		return "", fmt.Errorf("%w: invalid workspace file path", ErrInvalidInput)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("%w: invalid workspace file path", ErrInvalidInput)
		}
	}
	if value == "CLAUDE.md" || strings.HasPrefix(value, ".claude/") || strings.HasPrefix(value, ".opencode/") || strings.HasPrefix(value, ".git/") || strings.HasPrefix(value, ".superteam/") {
		return "", fmt.Errorf("%w: workspace file path is reserved", ErrInvalidInput)
	}
	return value, nil
}

func normalizeWorkspaceFileRole(fileRole, filePath string) (string, error) {
	value := strings.TrimSpace(fileRole)
	if value == "" {
		if filePath == "AGENTS.md" {
			return "entrypoint", nil
		}
		return "supporting_doc", nil
	}
	switch value {
	case "entrypoint":
		if filePath != "AGENTS.md" {
			return "", fmt.Errorf("%w: entrypoint workspace file must be AGENTS.md", ErrInvalidInput)
		}
		return value, nil
	case "supporting_doc", "provider_config", "generated":
		if filePath == "AGENTS.md" && value != "entrypoint" {
			return "", fmt.Errorf("%w: AGENTS.md must be the entrypoint workspace file", ErrInvalidInput)
		}
		return value, nil
	default:
		return "", fmt.Errorf("%w: invalid workspace file role", ErrInvalidInput)
	}
}

func normalizeWorkspaceFileSyncPolicy(syncPolicy string) (string, error) {
	value := strings.TrimSpace(syncPolicy)
	if value == "" {
		return "auto", nil
	}
	switch value {
	case "auto", "manual", "disabled":
		return value, nil
	default:
		return "", fmt.Errorf("%w: invalid workspace file sync_policy", ErrInvalidInput)
	}
}

func inferWorkspaceFileMimeType(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".md", ".markdown":
		return "text/markdown"
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".txt":
		return "text/plain"
	default:
		return "text/plain"
	}
}

func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func buildDefaultAgentsContent(employee DigitalEmployeeRecord, configInput EmployeeConfigInput, preview *EffectiveConfigPreview) string {
	var builder strings.Builder
	builder.WriteString("You are an agent at SuperTeam.\n\n")
	builder.WriteString("# Execution Contract\n\n")
	builder.WriteString("- Work as digital employee: ")
	builder.WriteString(markdownInstructionDisplayValue(employee.Name))
	builder.WriteString("\n- Role: ")
	builder.WriteString(markdownInstructionDisplayValue(employee.Role))
	builder.WriteString("\n- Keep outputs aligned with the approved team and employee configuration.\n")
	builder.WriteString("- Ask for human approval before high-risk or ambiguous actions.\n")
	builder.WriteString("- Persist durable results through platform artifacts, evidence, or structured writeback.\n")
	if preview != nil {
		builder.WriteString("\n# Active Configuration\n\n")
		builder.WriteString("- Team config revision: ")
		builder.WriteString(preview.TeamConfigRevisionID.String())
		builder.WriteString("\n- Employee config revision: ")
		builder.WriteString(preview.EmployeeConfigRevisionID.String())
		builder.WriteString("\n")
	}
	if len(configInput.OutputContractAddendum) > 0 {
		builder.WriteString("\n# Output Contract Addendum\n\n")
		builder.WriteString("Additional output contract data is governed by the Control Plane effective configuration.\n")
	}
	return builder.String()
}

func markdownInstructionDisplayValue(value string) string {
	collapsed := strings.Join(strings.Fields(value), " ")
	return strconv.Quote(collapsed)
}

func workspaceFileForSyncFromDefault(file WorkspaceFileRecord, revision WorkspaceFileRevisionRecord) WorkspaceFileForSyncRecord {
	return WorkspaceFileForSyncRecord{
		FileID:            file.ID,
		TenantID:          file.TenantID,
		TeamID:            file.TeamID,
		DigitalEmployeeID: file.DigitalEmployeeID,
		Path:              file.Path,
		FileRole:          file.FileRole,
		MimeType:          file.MimeType,
		SyncPolicy:        file.SyncPolicy,
		FileMetadata:      cloneMap(file.Metadata),
		RevisionID:        revision.ID,
		RevisionNumber:    revision.RevisionNumber,
		ContentText:       revision.ContentText,
		ContentHash:       revision.ContentHash,
		SizeBytes:         revision.SizeBytes,
		StorageBackend:    revision.StorageBackend,
		ObjectKey:         cloneStringPtr(revision.ObjectKey),
		RevisionMetadata:  cloneMap(revision.Metadata),
	}
}

func workspaceFileFromRecords(file WorkspaceFileRecord, revision WorkspaceFileRevisionRecord) WorkspaceFile {
	return WorkspaceFile{
		ID:                file.ID,
		TenantID:          file.TenantID,
		TeamID:            file.TeamID,
		DigitalEmployeeID: file.DigitalEmployeeID,
		Path:              file.Path,
		FileRole:          file.FileRole,
		MimeType:          file.MimeType,
		SyncPolicy:        file.SyncPolicy,
		Status:            file.Status,
		CurrentRevisionID: revision.ID,
		RevisionNumber:    revision.RevisionNumber,
		Content:           revision.ContentText,
		ContentHash:       revision.ContentHash,
		SizeBytes:         revision.SizeBytes,
		StorageBackend:    revision.StorageBackend,
		ObjectKey:         cloneStringPtr(revision.ObjectKey),
		CreatedBy:         cloneUUIDPtr(file.CreatedBy),
		ChangeNote:        cloneStringPtr(revision.ChangeNote),
		CreatedAt:         file.CreatedAt,
		UpdatedAt:         file.UpdatedAt,
	}
}

type runtimeWorkspaceFilePayload struct {
	FileID         string
	RevisionID     string
	Path           string
	FileRole       string
	MimeType       string
	SyncPolicy     string
	ContentHash    string
	SizeBytes      int32
	StorageBackend string
	ContentText    string
	ObjectKey      *string
	Metadata       map[string]any
}

type runtimeSkillPayload struct {
	SkillID     string   `json:"skill_id"`
	SkillKey    string   `json:"skill_key"`
	RevisionID  string   `json:"revision_id"`
	Files       []string `json:"files"`
	ContentHash string   `json:"content_hash"`
}

type runtimeMCPServerPayload struct {
	ServerID        string
	ServerKey       string
	Transport       string
	ConfigRef       string
	PermissionScope map[string]any
}

func runtimeWorkspaceFilesPayload(files []WorkspaceFileForSyncRecord) []map[string]any {
	out := make([]map[string]any, 0, len(files))
	for _, file := range files {
		payload := runtimeWorkspaceFilePayload{
			FileID:         file.FileID.String(),
			RevisionID:     file.RevisionID.String(),
			Path:           file.Path,
			FileRole:       file.FileRole,
			MimeType:       file.MimeType,
			SyncPolicy:     file.SyncPolicy,
			ContentHash:    file.ContentHash,
			SizeBytes:      file.SizeBytes,
			StorageBackend: file.StorageBackend,
			ContentText:    file.ContentText,
			ObjectKey:      cloneStringPtr(file.ObjectKey),
			Metadata: map[string]any{
				"file":     cloneMap(file.FileMetadata),
				"revision": cloneMap(file.RevisionMetadata),
			},
		}
		item := map[string]any{
			"file_id":         payload.FileID,
			"revision_id":     payload.RevisionID,
			"path":            payload.Path,
			"file_role":       payload.FileRole,
			"mime_type":       payload.MimeType,
			"sync_policy":     payload.SyncPolicy,
			"content_hash":    payload.ContentHash,
			"size_bytes":      payload.SizeBytes,
			"storage_backend": payload.StorageBackend,
			"metadata":        payload.Metadata,
		}
		if payload.StorageBackend == "db" {
			item["content_text"] = payload.ContentText
		}
		if payload.ObjectKey != nil {
			item["object_key"] = *payload.ObjectKey
		}
		out = append(out, item)
	}
	return out
}

func runtimeSkillsPayload(capabilitySelection map[string]any) []map[string]any {
	keys := stringList(capabilitySelection["enabled_skills"])
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		payload := runtimeSkillPayload{
			SkillKey: key,
			Files:    []string{},
		}
		out = append(out, map[string]any{
			"skill_id":     payload.SkillID,
			"skill_key":    payload.SkillKey,
			"revision_id":  payload.RevisionID,
			"files":        payload.Files,
			"content_hash": payload.ContentHash,
		})
	}
	return out
}

func runtimeMCPServersPayload(capabilitySelection map[string]any) []map[string]any {
	keys := stringList(capabilitySelection["enabled_mcp_servers"])
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		payload := runtimeMCPServerPayload{
			ServerKey:       key,
			PermissionScope: map[string]any{},
		}
		out = append(out, map[string]any{
			"server_id":        payload.ServerID,
			"server_key":       payload.ServerKey,
			"transport":        payload.Transport,
			"config_ref":       payload.ConfigRef,
			"permission_scope": payload.PermissionScope,
		})
	}
	return out
}

func emptyRuntimeSkillsPayload() []map[string]any {
	return []map[string]any{}
}

func emptyRuntimeMCPServersPayload() []map[string]any {
	return []map[string]any{}
}

func provisioningErrorWithAbort(cause error, abortErr error) error {
	if abortErr != nil {
		return fmt.Errorf("%w; abort provisioning: %v", cause, abortErr)
	}
	return cause
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
	var latestConfig *EmployeeConfigInput
	latest, err := s.repository.GetLatestDigitalEmployeeConfigRevision(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("get latest digital employee config revision: %w", err)
		}
	} else {
		latestConfig = &latest
	}
	roleProfile := inheritedConfigMap(req.RoleProfile, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.RoleProfile
	})
	constitutionAddendum := inheritedConfigMap(req.ConstitutionAddendum, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.ConstitutionAddendum
	})
	capabilitySelection := inheritedConfigMap(req.CapabilitySelection, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.CapabilitySelection
	})
	contextPolicyOverride := inheritedConfigMap(req.ContextPolicyOverride, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.ContextPolicyOverride
	})
	approvalPolicyOverride := inheritedConfigMap(req.ApprovalPolicyOverride, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.ApprovalPolicyOverride
	})
	budgetPolicySource := inheritedConfigMap(req.BudgetPolicy, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.BudgetPolicy
	})
	budgetPolicy, err := normalizeBudgetPolicy(budgetPolicySource)
	if err != nil {
		return nil, err
	}
	outputContractAddendum := inheritedConfigMap(req.OutputContractAddendum, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.OutputContractAddendum
	})
	nextRevision, err := s.repository.GetNextDigitalEmployeeConfigRevisionNumber(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get next digital employee config revision number: %w", err)
	}
	record, err := s.repository.CreateDigitalEmployeeConfigRevision(ctx, CreateConfigRevisionParams{
		TenantID:               req.TenantID,
		DigitalEmployeeID:      req.DigitalEmployeeID,
		RevisionNumber:         nextRevision,
		RoleProfile:            roleProfile,
		ConstitutionAddendum:   constitutionAddendum,
		CapabilitySelection:    capabilitySelection,
		ContextPolicyOverride:  contextPolicyOverride,
		ApprovalPolicyOverride: approvalPolicyOverride,
		BudgetPolicy:           budgetPolicy,
		OutputContractAddendum: outputContractAddendum,
		Status:                 status,
	})
	if err != nil {
		return nil, fmt.Errorf("create digital employee config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func inheritedConfigMap(requested map[string]any, latest *EmployeeConfigInput, selectLatest func(EmployeeConfigInput) map[string]any) map[string]any {
	if requested != nil {
		return cloneMap(requested)
	}
	if latest == nil {
		return map[string]any{}
	}
	return cloneMap(selectLatest(*latest))
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
	if req.TeamConfig.Status != "" && req.TeamConfig.Status != TeamConfigRevisionStatusActive {
		return nil, fmt.Errorf("%w: team config revision must be active", ErrInvalidInput)
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
		"budget_policy":                 cloneMap(req.EmployeeConfig.BudgetPolicy),
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
	if teamConfig.Status != TeamConfigRevisionStatusActive {
		return nil, fmt.Errorf("%w: team config revision must be active", ErrInvalidInput)
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
		OwnerUserID:      record.OwnerUserID,
		EmployeeType:     record.EmployeeType,
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
		BudgetPolicy:           cloneMap(record.BudgetPolicy),
		OutputContractAddendum: cloneMap(record.OutputContractAddendum),
		Status:                 record.Status,
		ApprovedBy:             validUUIDPtr(record.ApprovedBy),
		ApprovedAt:             cloneTimePtr(record.ApprovedAt),
		ArchivedAt:             cloneTimePtr(record.ArchivedAt),
		CreatedAt:              record.CreatedAt,
		UpdatedAt:              record.UpdatedAt,
	}
}

func employeeConfigInputFromRecord(record DigitalEmployeeConfigRevisionRecord) EmployeeConfigInput {
	return EmployeeConfigInput{
		ID:                     record.ID,
		TenantID:               record.TenantID,
		DigitalEmployeeID:      record.DigitalEmployeeID,
		RevisionNumber:         record.RevisionNumber,
		RoleProfile:            cloneMap(record.RoleProfile),
		ConstitutionAddendum:   cloneMap(record.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(record.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(record.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(record.ApprovalPolicyOverride),
		BudgetPolicy:           cloneMap(record.BudgetPolicy),
		OutputContractAddendum: cloneMap(record.OutputContractAddendum),
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
	keys := []struct {
		overrideKey string
		teamKeys    []string
	}{
		{overrideKey: "sources", teamKeys: []string{"sources", "allowed_sources"}},
		{overrideKey: "knowledge_bases", teamKeys: []string{"knowledge_bases", "allowed_knowledge_bases"}},
		{overrideKey: "documents", teamKeys: []string{"documents", "allowed_documents"}},
		{overrideKey: "repositories", teamKeys: []string{"repositories", "allowed_repositories"}},
		{overrideKey: "log_sources", teamKeys: []string{"log_sources", "allowed_log_sources"}},
	}
	issues := []ValidationIssue{}
	for _, key := range keys {
		allowed, _, allowedIssues := firstStringListPolicyValue(teamPolicy, key.teamKeys...)
		requested, requestedIssues := stringListPolicyValue(employeeOverride, key.overrideKey, fmt.Sprintf("context_policy_override.%s", key.overrideKey))
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
			Path:    fmt.Sprintf("context_policy_override.%s", key.overrideKey),
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

func firstStringListPolicyValue(values map[string]any, keys ...string) ([]string, bool, []ValidationIssue) {
	for _, key := range keys {
		if _, ok := values[key]; !ok {
			continue
		}
		values, issues := stringListPolicyValue(values, key, key)
		return values, true, issues
	}
	return nil, false, nil
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

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func stringPtrValue(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func normalizeBudgetPolicy(input map[string]any) (map[string]any, error) {
	policy := cloneMap(input)
	if policy == nil {
		return map[string]any{}, nil
	}
	value, exists := policy["daily_token_limit"]
	if !exists || value == nil || value == "" {
		delete(policy, "daily_token_limit")
		return policy, nil
	}

	var limit int64
	switch typed := value.(type) {
	case int:
		limit = int64(typed)
	case int32:
		limit = int64(typed)
	case int64:
		limit = typed
	case float64:
		if typed != float64(int64(typed)) {
			return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
		}
		limit = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
		}
		limit = parsed
	default:
		return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
	}
	if limit <= 0 || limit > int64(^uint32(0)>>1) {
		return nil, fmt.Errorf("%w: budget_policy.daily_token_limit must be a positive integer", ErrInvalidInput)
	}
	policy["daily_token_limit"] = float64(limit)
	return policy, nil
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
