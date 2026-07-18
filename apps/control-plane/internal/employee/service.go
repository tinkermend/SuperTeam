package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/skill"
	"github.com/superteam/control-plane/internal/tenant"
)

type Service struct {
	repository Repository
	envCodec   *EnvironmentValueCodec
}

const defaultProvisioningPollInterval = 250 * time.Millisecond

var supportedDigitalEmployeeProviderTypes = map[string]struct{}{
	"claude-code": {},
	"opencode":    {},
	"codex":       {},
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository}, nil
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

// GetActivity 返回跨员工运行动态流（时间倒序）。NextSince 指向最新事件，
// 供客户端下次以 since 增量拉取；无新事件时由 handler 回显请求游标。
func (s *Service) GetActivity(ctx context.Context, req GetDigitalEmployeeActivityRequest) (*DigitalEmployeeActivity, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	items, err := s.repository.GetDigitalEmployeeActivity(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get digital employee activity: %w", err)
	}
	activity := &DigitalEmployeeActivity{Items: items}
	if len(items) > 0 && items[0].OccurredAt != nil {
		activity.NextSince = encodeActivityCursor(*items[0].OccurredAt, items[0].EventID)
	}
	return activity, nil
}

func (s *Service) GetCreateOptions(ctx context.Context, req CreateOptionsRequest) (*CreateOptions, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	teamLess := req.TeamID == nil || *req.TeamID == uuid.Nil
	var teamConfig TeamConfigInput
	if teamLess {
		teamConfig = defaultTeamLessConfigInput(req.TenantID)
	} else {
		if err := s.repository.EnsureTeamExists(ctx, req.TenantID, *req.TeamID); err != nil {
			return nil, fmt.Errorf("get team: %w", err)
		}
		baseline, err := s.repository.GetTeamBaseline(ctx, req.TenantID, *req.TeamID)
		if err != nil {
			return nil, fmt.Errorf("get team baseline: %w", err)
		}
		teamConfig = teamConfigInputFromBaseline(req.TenantID, *req.TeamID, baseline)
	}
	teamConfigOption, err := teamConfigCreateOption(teamConfig)
	if err != nil {
		return nil, err
	}
	templates, err := s.repository.ListEmployeeTemplates(ctx, ListEmployeeTemplatesParams{TenantID: req.TenantID, ActiveOnly: true})
	if err != nil {
		return nil, fmt.Errorf("list employee templates: %w", err)
	}
	employeeTypes := make([]EmployeeTypeDefinition, 0, len(templates)+1)
	employeeTypes = append(employeeTypes, customAgentEmployeeTypeDefinition())
	for _, template := range templates {
		employeeTypes = append(employeeTypes, template.ToDefinition())
	}
	var runtimeOptions []RuntimeProviderOption
	if teamLess {
		runtimeOptions, err = s.repository.ListRuntimeProviderOptionsForTeamLessCreate(ctx, req.TenantID)
	} else {
		runtimeOptions, err = s.repository.ListRuntimeProviderOptionsForCreate(ctx, req.TenantID, *req.TeamID)
	}
	if err != nil {
		return nil, fmt.Errorf("list runtime provider options: %w", err)
	}
	capabilityOptions := capabilityOptionsForCreate(employeeTypes)

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

func defaultTeamLessConfigInput(tenantID uuid.UUID) TeamConfigInput {
	return TeamConfigInput{
		ID:                          uuid.Nil,
		TenantID:                    tenantID,
		TeamID:                      uuid.Nil,
		CapabilityPolicy:            map[string]any{},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
	}
}

func teamConfigInputFromBaseline(tenantID, teamID uuid.UUID, baseline TeamBaseline) TeamConfigInput {
	return TeamConfigInput{
		ID:           uuid.Nil,
		TenantID:     tenantID,
		TeamID:       teamID,
		Constitution: cloneMap(baseline.Constitution),
		CapabilityPolicy: map[string]any{
			"allowed_skills":      append([]string(nil), baseline.Skills...),
			"allowed_mcp_servers": append([]string(nil), baseline.MCPServers...),
		},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
	}
}

func createOptionChecks(
	teamConfig TeamConfigCreateOption,
	employeeTypes []EmployeeTypeDefinition,
	capabilityOptions CapabilityOptions,
	runtimeOptions []RuntimeProviderOption,
) []CreateOptionCheck {
	availableRuntimeCount := 0
	inactiveRuntimeSessionCount := 0
	for _, option := range runtimeOptions {
		if option.Available {
			availableRuntimeCount++
			continue
		}
		if option.DisabledReason == "runtime_session_inactive" {
			inactiveRuntimeSessionCount++
		}
	}

	capabilityCount := len(capabilityOptions.Skills) + len(capabilityOptions.MCPServers)

	return []CreateOptionCheck{
		{
			Key:     "team_baseline",
			Label:   "团队继承基线",
			Status:  "passed",
			Message: fmt.Sprintf("skills %d · MCP %d", len(teamConfig.Skills), len(teamConfig.MCPServers)),
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
			Message: fmt.Sprintf("技能 %d · MCP %d", len(capabilityOptions.Skills), len(capabilityOptions.MCPServers)),
		},
		{
			Key:     "runtime_provider",
			Label:   "Provider 类型预览",
			Status:  checkStatus(availableRuntimeCount > 0, true),
			Message: runtimeProviderCreateOptionMessage(availableRuntimeCount, len(runtimeOptions), inactiveRuntimeSessionCount),
		},
	}
}

func runtimeProviderCreateOptionMessage(availableRuntimeCount, totalRuntimeCount, inactiveRuntimeSessionCount int) string {
	message := fmt.Sprintf("%d/%d 个 Provider 候选当前可用于调度；创建时不绑定 Runtime 节点", availableRuntimeCount, totalRuntimeCount)
	if availableRuntimeCount == 0 && inactiveRuntimeSessionCount > 0 {
		message = fmt.Sprintf("%d/%d 个 Provider 候选当前可用于调度；%d 个 Runtime 会话未激活", availableRuntimeCount, totalRuntimeCount, inactiveRuntimeSessionCount)
	}
	return message
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
	var teamID *uuid.UUID
	if teamConfig.TeamID != uuid.Nil {
		id := teamConfig.TeamID
		teamID = &id
	}
	return TeamConfigCreateOption{
		ID:           teamConfig.ID,
		TenantID:     teamConfig.TenantID,
		TeamID:       teamID,
		Constitution: cloneMap(teamConfig.Constitution),
		Skills:       optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_skills"),
		MCPServers:   optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_mcp_servers"),
	}, nil
}

func capabilityOptionsForCreate(employeeTypes []EmployeeTypeDefinition) CapabilityOptions {
	return CapabilityOptions{
		ProviderTypes: supportedProviderTypes(),
		Skills:        platformSkillOptions(employeeTypes),
		MCPServers:    platformMCPServerOptions(employeeTypes),
	}
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

func emptyPolicyDefaults() PolicyDefaults {
	return PolicyDefaults{
		PermissionPolicy: map[string]any{},
		ApprovalPolicy:   map[string]any{},
		WorkspacePolicy:  map[string]any{},
		SessionPolicy:    map[string]any{},
		Metadata:         map[string]any{},
	}
}

func (s *Service) CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error) {
	normalized, definition, err := s.normalizeCreateDigitalEmployeeRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	teamLess := normalized.TeamID == nil
	var teamConfig TeamConfigInput
	if teamLess {
		teamConfig = defaultTeamLessConfigInput(normalized.TenantID)
	} else {
		teamID := *normalized.TeamID
		if err := s.repository.EnsureTeamExists(ctx, normalized.TenantID, teamID); err != nil {
			return nil, fmt.Errorf("get team: %w", err)
		}
		if err := s.ensureTeamDigitalEmployeeCapacity(ctx, normalized.TenantID, teamID); err != nil {
			return nil, err
		}
		baseline, err := s.repository.GetTeamBaseline(ctx, normalized.TenantID, teamID)
		if err != nil {
			return nil, fmt.Errorf("get team baseline: %w", err)
		}
		teamConfig = teamConfigInputFromBaseline(normalized.TenantID, teamID, baseline)
	}
	if err := s.validateInitialEffectiveConfig(ctx, s.repository, normalized, definition, teamConfig, uuid.New()); err != nil {
		return nil, err
	}

	var record DigitalEmployeeRecord
	if err := s.repository.WithTransaction(ctx, func(txRepo Repository) error {
		createdRecord, err := s.createLocalReadyEmployeeFacts(ctx, txRepo, normalized, definition, teamConfig)
		if err != nil {
			return err
		}
		record = createdRecord
		return nil
	}); err != nil {
		return nil, err
	}

	employee := employeeFromRecord(record)
	if err := s.attachLatestConfigRevision(ctx, employee); err != nil {
		return nil, err
	}
	return employee, nil
}

func (s *Service) ensureTeamDigitalEmployeeCapacity(ctx context.Context, tenantID, teamID uuid.UUID) error {
	overview, err := s.repository.GetDigitalEmployeeOverview(ctx, GetDigitalEmployeeOverviewRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
		Limit:    1,
	})
	if err != nil {
		return fmt.Errorf("get digital employee overview: %w", err)
	}
	if overview.Pagination.TotalCount >= tenant.MaxDigitalEmployeesPerTeam {
		return fmt.Errorf("%w: digital employee capacity exceeded", ErrInvalidInput)
	}
	return nil
}

func (s *Service) normalizeCreateDigitalEmployeeRequest(ctx context.Context, req CreateDigitalEmployeeRequest) (CreateDigitalEmployeeRequest, EmployeeTypeDefinition, error) {
	if req.TenantID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID != nil && *req.TeamID == uuid.Nil {
		req.TeamID = nil
	}
	if req.OwnerUserID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: owner_user_id is required", ErrInvalidInput)
	}
	employeeType := strings.ToLower(strings.TrimSpace(req.EmployeeType))
	if employeeType == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: employee_type is required", ErrInvalidInput)
	}
	definition, err := s.employeeTypeDefinitionByType(ctx, req.TenantID, employeeType)
	if err != nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, err
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
	providerType := normalizeProviderType(req.ProviderType)
	if providerType == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
	}
	if !isSupportedDigitalEmployeeProviderType(providerType) {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: unsupported provider_type %q", ErrInvalidInput, providerType)
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

// employeeTypeDefinitionByType resolves an employee_type string to its
// EmployeeTypeDefinition. custom_agent is a hardcoded sentinel (never
// persisted); every other type is looked up in digital_employee_templates,
// scoped to the tenant, and must be active to be usable for creation.
func (s *Service) employeeTypeDefinitionByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTypeDefinition, error) {
	if employeeType == "custom_agent" {
		return customAgentEmployeeTypeDefinition(), nil
	}
	template, err := s.repository.GetEmployeeTemplateByType(ctx, tenantID, employeeType)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return EmployeeTypeDefinition{}, fmt.Errorf("%w: unknown employee_type %q", ErrInvalidInput, employeeType)
		}
		return EmployeeTypeDefinition{}, err
	}
	if template.Status != "active" {
		return EmployeeTypeDefinition{}, fmt.Errorf("%w: employee_type %q is disabled", ErrInvalidInput, employeeType)
	}
	return template.ToDefinition(), nil
}

func normalizeProviderType(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "claude_code" {
		return "claude-code"
	}
	return normalized
}

func isSupportedDigitalEmployeeProviderType(providerType string) bool {
	_, ok := supportedDigitalEmployeeProviderTypes[providerType]
	return ok
}

func defaultRiskLevelForEmployeeType(definition EmployeeTypeDefinition) string {
	if value, ok := definition.DefaultApprovalPolicy["min_risk_for_human"].(string); ok && riskRank(value) > 0 {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return "medium"
}

func supportedProviderTypes() []string {
	types := make([]string, 0, len(supportedDigitalEmployeeProviderTypes))
	for providerType := range supportedDigitalEmployeeProviderTypes {
		types = append(types, providerType)
	}
	sort.Strings(types)
	return types
}

func platformSkillOptions(employeeTypes []EmployeeTypeDefinition) []string {
	values := make(map[string]struct{})
	for _, definition := range employeeTypes {
		for _, skill := range definition.RecommendedSkills {
			if skill == "" {
				continue
			}
			values[skill] = struct{}{}
		}
		for _, skill := range stringList(definition.CapabilityBindings["skills"]) {
			if skill == "" {
				continue
			}
			values[skill] = struct{}{}
		}
	}
	return sortedKeys(values)
}

func platformMCPServerOptions(employeeTypes []EmployeeTypeDefinition) []string {
	values := make(map[string]struct{})
	for _, definition := range employeeTypes {
		for _, serverID := range definition.RecommendedMCPServers {
			if serverID == "" {
				continue
			}
			values[serverID] = struct{}{}
		}
		for _, serverID := range stringList(definition.CapabilityBindings["mcp_servers"]) {
			if serverID == "" {
				continue
			}
			values[serverID] = struct{}{}
		}
	}
	return sortedKeys(values)
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
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

func (s *Service) createLocalReadyEmployeeFacts(ctx context.Context, repository Repository, req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput) (DigitalEmployeeRecord, error) {
	record, err := repository.CreateDigitalEmployee(ctx, createDigitalEmployeeParams(req))
	if err != nil {
		return DigitalEmployeeRecord{}, fmt.Errorf("create digital employee: %w", err)
	}
	if err := s.createInitialEnvironmentVariables(ctx, repository, record, req); err != nil {
		return DigitalEmployeeRecord{}, err
	}
	configRevision, err := s.createInitialActiveConfigRevision(ctx, repository, record, req, definition, teamConfig)
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	configInput := employeeConfigInputFromRecord(configRevision)
	preview, err := s.previewEffectiveConfigWithRepository(ctx, repository, teamConfig, configInput)
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	if len(preview.Validation.BlockingErrors) > 0 {
		return DigitalEmployeeRecord{}, fmt.Errorf("%w: effective config has blocking validation errors", ErrInvalidInput)
	}
	record.Status = DigitalEmployeeStatusReady
	if updated, err := repository.UpdateDigitalEmployeeStatus(ctx, req.TenantID, record.ID, DigitalEmployeeStatusReady); err == nil {
		record = updated
	} else if !errors.Is(err, ErrNotFound) {
		return DigitalEmployeeRecord{}, fmt.Errorf("mark digital employee ready: %w", err)
	}
	return record, nil
}

func (s *Service) createInitialEnvironmentVariables(ctx context.Context, repository Repository, record DigitalEmployeeRecord, req CreateDigitalEmployeeRequest) error {
	if len(req.EnvironmentVariables) == 0 {
		return nil
	}
	for _, item := range req.EnvironmentVariables {
		name, err := normalizeEnvName(item.Name)
		if err != nil {
			return err
		}
		if _, err := s.upsertEncryptedEnvironmentVariable(ctx, repository, UpsertEnvironmentVariableStoreInput{
			TenantID:          req.TenantID,
			TeamID:            req.TeamID,
			DigitalEmployeeID: record.ID,
			Name:              name,
			Value:             item.Value,
			Sensitive:         item.Sensitive,
			UpdatedBy:         &req.OwnerUserID,
		}); err != nil {
			return err
		}
	}
	return nil
}

func createDigitalEmployeeParams(req CreateDigitalEmployeeRequest) CreateDigitalEmployeeParams {
	return CreateDigitalEmployeeParams{
		TenantID:         req.TenantID,
		TeamID:           validUUIDPtr(req.TeamID),
		OwnerUserID:      req.OwnerUserID,
		EmployeeType:     req.EmployeeType,
		ProviderType:     req.ProviderType,
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
		TenantID:              req.TenantID,
		DigitalEmployeeID:     employeeID,
		RevisionNumber:        revisionNumber,
		PersonaMemoryMarkdown: strings.TrimSpace(req.PersonaMemoryMarkdown),
		CapabilityBindings:    normalizeCapabilityBindings(initialCapabilitySelection(req, definition, teamConfig)),
		BudgetPolicy:          cloneMap(req.BudgetPolicy),
		Status:                ConfigRevisionStatusActive,
		ApprovedBy:            &approvedBy,
		ApprovedAt:            &approvedAt,
	}
}

func initialEmployeeConfigInput(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput, employeeID, configID uuid.UUID, revisionNumber int32) EmployeeConfigInput {
	return EmployeeConfigInput{
		ID:                    configID,
		TenantID:              req.TenantID,
		DigitalEmployeeID:     employeeID,
		RevisionNumber:        revisionNumber,
		PersonaMemoryMarkdown: strings.TrimSpace(req.PersonaMemoryMarkdown),
		CapabilityBindings:    normalizeCapabilityBindings(initialCapabilitySelection(req, definition, teamConfig)),
		BudgetPolicy:          cloneMap(req.BudgetPolicy),
	}
}

func initialCapabilitySelection(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput) map[string]any {
	defaults := cloneMap(definition.CapabilityBindings)
	return mergePolicyMaps(defaults, req.CapabilityBindings)
}
func mergePolicyMaps(base, override map[string]any) map[string]any {
	merged := cloneMap(base)
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func normalizeCapabilityBindings(input map[string]any) map[string]any {
	bindings := cloneMap(input)
	if bindings == nil {
		bindings = map[string]any{}
	}
	for _, key := range []string{"skills", "mcp_servers", "external_capabilities", "environment_variable_refs"} {
		if _, ok := bindings[key]; !ok {
			bindings[key] = []any{}
		}
	}
	return bindings
}

type runtimeSkillPayload struct {
	SkillID               string `json:"skill_id"`
	SkillKey              string `json:"skill_key"`
	RevisionID            string `json:"revision_id"`
	ArchiveObjectRef      string `json:"archive_object_ref"`
	ArchiveChecksumSHA256 string `json:"archive_checksum_sha256"`
	ArchiveSizeBytes      int64  `json:"archive_size_bytes"`
	ArchiveFileCount      int    `json:"archive_file_count"`
}

func runtimeSkillsPayload(skills []skill.SkillRuntimeRecord) []map[string]any {
	out := make([]map[string]any, 0, len(skills))
	for _, s := range skills {
		payload := runtimeSkillPayload{
			SkillID:               s.ID.String(),
			SkillKey:              s.Slug,
			RevisionID:            s.ArchiveChecksum,
			ArchiveObjectRef:      s.ArchiveObjectRef,
			ArchiveChecksumSHA256: s.ArchiveChecksum,
			ArchiveSizeBytes:      s.ArchiveSizeBytes,
			ArchiveFileCount:      s.ArchiveFileCount,
		}
		out = append(out, map[string]any{
			"skill_id":                payload.SkillID,
			"skill_key":               payload.SkillKey,
			"revision_id":             payload.RevisionID,
			"archive_object_ref":      payload.ArchiveObjectRef,
			"archive_checksum_sha256": payload.ArchiveChecksumSHA256,
			"archive_size_bytes":      payload.ArchiveSizeBytes,
			"archive_file_count":      payload.ArchiveFileCount,
		})
	}
	return out
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
		TenantID:   req.TenantID,
		TeamID:     validUUIDPtr(req.TeamID),
		Status:     req.Status,
		Assignment: req.Assignment,
		Offset:     req.Offset,
		Limit:      req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list digital employees: %w", err)
	}
	employees := make([]*DigitalEmployee, 0, len(records))
	for _, record := range records {
		employee := employeeFromRecord(record)
		if err := s.attachLatestConfigRevision(ctx, employee); err != nil {
			return nil, err
		}
		employees = append(employees, employee)
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
	employee := employeeFromRecord(record)
	if err := s.attachLatestConfigRevision(ctx, employee); err != nil {
		return nil, err
	}
	return employee, nil
}

func (s *Service) DeleteDigitalEmployee(ctx context.Context, req DeleteDigitalEmployeeRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.ActorUserID == uuid.Nil {
		return fmt.Errorf("%w: actor_user_id is required", ErrInvalidInput)
	}
	deletedAt := time.Now().UTC()
	return s.repository.WithTransaction(ctx, func(repository Repository) error {
		employee, err := repository.GetDigitalEmployeeForDelete(ctx, req.TenantID, req.DigitalEmployeeID)
		if err != nil {
			return err
		}
		blockers, err := repository.ListDigitalEmployeeDeleteBlockers(ctx, req.TenantID, req.DigitalEmployeeID)
		if err != nil {
			return err
		}
		if len(blockers) > 0 {
			return &DigitalEmployeeDeleteBlockedError{Blockers: append([]DigitalEmployeeDeleteBlocker(nil), blockers...)}
		}
		cascade, err := repository.SoftDeleteDigitalEmployeeCascade(ctx, SoftDeleteDigitalEmployeeCascadeParams{
			TenantID:          req.TenantID,
			DigitalEmployeeID: req.DigitalEmployeeID,
			DeletedAt:         deletedAt,
		})
		if err != nil {
			return err
		}
		return repository.CreateDigitalEmployeeDeleteAuditEvent(ctx, DigitalEmployeeDeleteAuditEventParams{
			TenantID:      req.TenantID,
			ActorUserID:   req.ActorUserID,
			Employee:      employee,
			CascadeResult: cascade,
			DeletedAt:     deletedAt,
		})
	})
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

type ReassignDigitalEmployeeTeamRequest struct {
	TenantID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	TeamID            uuid.UUID
	ActorUserID       uuid.UUID
}

// TeamReassignRepository is the optional repository capability behind
// 员工换队/首次归队：把员工绑到目标团队（目标团队必须存在且 active），
// 已有归属的员工允许跨队转移。pg 仓储实现；不实现的 fake 视为不支持。
type TeamReassignRepository interface {
	ReassignDigitalEmployeeTeam(ctx context.Context, params ReassignDigitalEmployeeTeamRequest) (DigitalEmployeeRecord, error)
}

// ReassignTeam 换队/首次归队。副作用提示：员工的 agent home dir 按
// (team, employee) 键，换队后下次派发落新家目录（provider 会话连续性重置）；
// 团队级技能与 MCP 绑定继承随之切换。
func (s *Service) ReassignTeam(ctx context.Context, req ReassignDigitalEmployeeTeamRequest) (*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	reassigner, ok := s.repository.(TeamReassignRepository)
	if !ok {
		return nil, fmt.Errorf("%w: team reassignment is not supported by this repository", ErrInvalidInput)
	}
	record, err := reassigner.ReassignDigitalEmployeeTeam(ctx, req)
	if err != nil {
		return nil, err
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
	return nil, fmt.Errorf("%w: digital employees are not runtime-bound; bind runtime nodes to projects and dispatch project tasks instead", ErrInvalidInput)
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
	personaMemoryMarkdown := ""
	if latestConfig != nil {
		personaMemoryMarkdown = latestConfig.PersonaMemoryMarkdown
	}
	if req.PersonaMemoryMarkdown != nil {
		personaMemoryMarkdown = strings.TrimSpace(*req.PersonaMemoryMarkdown)
	}
	capabilityBindings := inheritedConfigMap(req.CapabilityBindings, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.CapabilityBindings
	})
	capabilityBindings = normalizeCapabilityBindings(capabilityBindings)
	budgetPolicySource := inheritedConfigMap(req.BudgetPolicy, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.BudgetPolicy
	})
	budgetPolicy, err := normalizeBudgetPolicy(budgetPolicySource)
	if err != nil {
		return nil, err
	}
	nextRevision, err := s.repository.GetNextDigitalEmployeeConfigRevisionNumber(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get next digital employee config revision number: %w", err)
	}
	record, err := s.repository.CreateDigitalEmployeeConfigRevision(ctx, CreateConfigRevisionParams{
		TenantID:              req.TenantID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		RevisionNumber:        nextRevision,
		PersonaMemoryMarkdown: personaMemoryMarkdown,
		CapabilityBindings:    capabilityBindings,
		BudgetPolicy:          budgetPolicy,
		Status:                status,
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

func (s *Service) GetSchedulingReadiness(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeSchedulingReadiness, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	employee, err := s.repository.GetDigitalEmployee(ctx, tenantID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	facts, err := s.repository.GetSchedulingCapabilityFacts(ctx, tenantID, employeeID)
	if err != nil {
		return nil, fmt.Errorf("get scheduling capability facts: %w", err)
	}

	checks := make([]SchedulingReadinessCheck, 0, 3)
	checks = append(checks, employeeStatusReadinessCheck(employee.Status))

	checks = append(checks, SchedulingReadinessCheck{
		Code:    "project_runtime",
		Status:  ReadinessCheckInfo,
		Label:   "项目运行准备",
		Message: "真实 Runtime/Provider 可执行性由项目运行准备检查判断。",
	})

	ready := true
	for _, check := range checks {
		if check.Status == ReadinessCheckBlocked {
			ready = false
			break
		}
	}

	return &DigitalEmployeeSchedulingReadiness{
		EmployeeID:                employee.ID,
		Status:                    employee.Status,
		ReadyForProjectScheduling: ready,
		Checks:                    checks,
		Capabilities: SchedulingReadinessCapabilities{
			Skills: SchedulingReadinessSkillSummary{
				PersonalCount:   facts.PersonalSkillCount,
				InheritedCount:  facts.InheritedSkillCount,
				MissingRequired: append([]string(nil), facts.MissingRequiredSkills...),
			},
			MCPServers: SchedulingReadinessMCPSummary{
				PersonalCount:  facts.PersonalMCPServerCount,
				InheritedCount: facts.InheritedMCPServerCount,
			},
			EnvironmentVariables: SchedulingReadinessEnvironmentSummary{
				ConfiguredCount: facts.ConfiguredEnvVarCount,
				MissingNames:    append([]string(nil), facts.MissingEnvironmentNames...),
			},
		},
		ProjectExecutionSource: "project_runtime_readiness",
	}, nil
}

func employeeStatusReadinessCheck(status DigitalEmployeeStatus) SchedulingReadinessCheck {
	if status == DigitalEmployeeStatusReady || status == DigitalEmployeeStatusActive {
		return SchedulingReadinessCheck{
			Code:    "employee_status",
			Status:  ReadinessCheckPassed,
			Label:   "员工状态",
			Message: fmt.Sprintf("员工状态为 %s，可进入项目调度池。", status),
		}
	}
	return SchedulingReadinessCheck{
		Code:    "employee_status",
		Status:  ReadinessCheckBlocked,
		Label:   "员工状态",
		Message: fmt.Sprintf("员工状态为 %s，不能进入项目调度池。", status),
	}
}

func (s *Service) PreviewEffectiveConfig(ctx context.Context, req PreviewEffectiveConfigRequest) (*EffectiveConfigPreview, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.EmployeeConfig.ID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_config_revision_id is required", ErrInvalidInput)
	}
	effectiveConfig := map[string]any{
		"employee_config_revision_id": req.EmployeeConfig.ID.String(),
		"persona_memory_markdown":     req.EmployeeConfig.PersonaMemoryMarkdown,
		"capability_bindings":         cloneMap(req.EmployeeConfig.CapabilityBindings),
		"budget_policy":               cloneMap(req.EmployeeConfig.BudgetPolicy),
	}
	validation := EffectiveConfigValidation{
		BlockingErrors: []ValidationIssue{},
		Warnings:       []ValidationIssue{},
	}

	return &EffectiveConfigPreview{
		EmployeeConfigRevisionID: req.EmployeeConfig.ID,
		EffectiveConfig:          effectiveConfig,
		Validation:               validation,
	}, nil
}

func employeeFromRecord(record DigitalEmployeeRecord) *DigitalEmployee {
	return &DigitalEmployee{
		ID:                 record.ID,
		TenantID:           record.TenantID,
		TeamID:             validUUIDPtr(record.TeamID),
		OwnerUserID:        record.OwnerUserID,
		EmployeeType:       record.EmployeeType,
		ProviderType:       record.ProviderType,
		Name:               record.Name,
		Role:               record.Role,
		Description:        trimOptionalString(record.Description),
		Status:             record.Status,
		PermissionPolicy:   cloneMap(record.PermissionPolicy),
		ContextPolicy:      cloneMap(record.ContextPolicy),
		ApprovalPolicy:     cloneMap(record.ApprovalPolicy),
		RiskLevel:          record.RiskLevel,
		Metadata:           cloneMap(record.Metadata),
		CapabilityBindings: map[string]any{},
		BudgetPolicy:       map[string]any{},
		DisabledAt:         cloneTimePtr(record.DisabledAt),
		ArchivedAt:         cloneTimePtr(record.ArchivedAt),
		CreatedAt:          record.CreatedAt,
		UpdatedAt:          record.UpdatedAt,
	}
}

func (s *Service) attachLatestConfigRevision(ctx context.Context, employee *DigitalEmployee) error {
	if employee == nil {
		return nil
	}
	config, err := s.repository.GetLatestDigitalEmployeeConfigRevision(ctx, employee.TenantID, employee.ID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			employee.PersonaMemoryMarkdown = ""
			employee.CapabilityBindings = map[string]any{}
			employee.BudgetPolicy = map[string]any{}
			return nil
		}
		return fmt.Errorf("get latest digital employee config revision: %w", err)
	}
	employee.PersonaMemoryMarkdown = config.PersonaMemoryMarkdown
	employee.CapabilityBindings = cloneMap(config.CapabilityBindings)
	employee.BudgetPolicy = cloneMap(config.BudgetPolicy)
	return nil
}

func configRevisionFromRecord(record DigitalEmployeeConfigRevisionRecord) *DigitalEmployeeConfigRevision {
	return &DigitalEmployeeConfigRevision{
		ID:                    record.ID,
		TenantID:              record.TenantID,
		DigitalEmployeeID:     record.DigitalEmployeeID,
		RevisionNumber:        record.RevisionNumber,
		PersonaMemoryMarkdown: record.PersonaMemoryMarkdown,
		CapabilityBindings:    cloneMap(record.CapabilityBindings),
		BudgetPolicy:          cloneMap(record.BudgetPolicy),
		Status:                record.Status,
		ApprovedBy:            validUUIDPtr(record.ApprovedBy),
		ApprovedAt:            cloneTimePtr(record.ApprovedAt),
		ArchivedAt:            cloneTimePtr(record.ArchivedAt),
		CreatedAt:             record.CreatedAt,
		UpdatedAt:             record.UpdatedAt,
	}
}

func employeeConfigInputFromRecord(record DigitalEmployeeConfigRevisionRecord) EmployeeConfigInput {
	return EmployeeConfigInput{
		ID:                    record.ID,
		TenantID:              record.TenantID,
		DigitalEmployeeID:     record.DigitalEmployeeID,
		RevisionNumber:        record.RevisionNumber,
		PersonaMemoryMarkdown: record.PersonaMemoryMarkdown,
		CapabilityBindings:    cloneMap(record.CapabilityBindings),
		BudgetPolicy:          cloneMap(record.BudgetPolicy),
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
