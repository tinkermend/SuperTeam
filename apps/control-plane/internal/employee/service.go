package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/permission"
	"github.com/superteam/control-plane/internal/skill"
	"github.com/superteam/control-plane/internal/systemconfig"
	"github.com/superteam/control-plane/internal/teamguard"
)

type Service struct {
	repository   Repository
	envCodec     *EnvironmentValueCodec
	systemConfig systemconfig.Reader
	// 权限审批接缝(方案2):提交治理变更产生 category=permission 审批请求;
	// 批准时权限中心调 ActivateConfigRevision 写回员工行。均为可选,未注入(测试)时提交即返回 ErrPermissionApprovalNotConfigured。
	approvals *approval.Service
	router    *permission.ApproverRouter
	// vocabulary 校验员工声明的 external_capabilities 是否在租户能力词表里
	// 注册且 active。此前只有场景模板侧过词表校验、员工侧随便写，于是出现
	// 「词表里注册的键 0 人声明 / 员工在用的键根本没注册」的两头落空 ——
	// 而这两侧本来就该抽同一份词表。可选:未注入(测试)时放行。
	vocabulary CapabilityVocabularyValidator
	// roleVocabulary 校验员工 role_keys 是否在租户角色词表中 active。
	roleVocabulary RoleVocabularyValidator
	// roleStore 读写 digital_employee_roles 多值角色绑定。
	roleStore EmployeeRoleStore
	// castingImpact 预检/级联解除编制（可选；未注入则移除角色不查编制）。
	castingImpact CastingImpactGateway
}

// CastingImpactGateway previews and cascades casting rows when employee roles shrink.
type CastingImpactGateway interface {
	ListEmployeeRoleImpact(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) (CastingRoleImpact, error)
	// CommitRoleReplaceWithCascade replaces role bindings and deletes affected
	// castings in one DB transaction, then emits project events / owner alerts.
	CommitRoleReplaceWithCascade(ctx context.Context, req RoleReplaceCascadeRequest) error
}

// RoleReplaceCascadeRequest is the same-txn write for role shrink + casting cascade.
type RoleReplaceCascadeRequest struct {
	TenantID      uuid.UUID
	EmployeeID    uuid.UUID
	ActorUserID   uuid.UUID
	NewRoleKeys   []string
	RemovedKeys   []string
	EmployeeName  string
}

// CastingRoleImpact is the employee-facing impact snapshot (mirrors project impact).
type CastingRoleImpact struct {
	AffectedCastings []CastingImpactRow
	AffectedCount    int
}

// CastingImpactRow is one casting row affected by a role removal.
type CastingImpactRow struct {
	ProjectID           uuid.UUID
	ProjectName         string
	ScenarioTemplateKey string
	TemplateName        string
	RoleKey             string
}

// ErrCastingImpactRequiresConfirm is returned when role removal would drop castings
// without confirm_impact=true. Handler maps it to HTTP 400 with impact body.
type ErrCastingImpactRequiresConfirm struct {
	Impact CastingRoleImpact
}

func (e *ErrCastingImpactRequiresConfirm) Error() string {
	return fmt.Sprintf("removing roles would invalidate %d casting row(s); pass confirm_impact=true", e.Impact.AffectedCount)
}

// RoleVocabularyValidator returns role keys not registered as active.
type RoleVocabularyValidator interface {
	UnknownKeys(ctx context.Context, tenantID uuid.UUID, keys []string) ([]string, error)
}

// EmployeeRoleStore persists multi-value role bindings for digital employees.
type EmployeeRoleStore interface {
	ListRoleKeys(ctx context.Context, tenantID, employeeID uuid.UUID) ([]string, error)
	ReplaceRoleKeys(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) error
	ListRoleKeysByEmployees(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID][]string, error)
}

// SetRoleVocabularyValidator injects the role vocabulary checker.
func (s *Service) SetRoleVocabularyValidator(validator RoleVocabularyValidator) {
	s.roleVocabulary = validator
}

// SetEmployeeRoleStore injects the multi-value role binding store.
func (s *Service) SetEmployeeRoleStore(store EmployeeRoleStore) {
	s.roleStore = store
}

func (s *Service) SetCastingImpactGateway(g CastingImpactGateway) {
	s.castingImpact = g
}

func (s *Service) validateRoleKeys(ctx context.Context, tenantID uuid.UUID, roleKeys []string) error {
	if s.roleVocabulary == nil || len(roleKeys) == 0 {
		return nil
	}
	unknown, err := s.roleVocabulary.UnknownKeys(ctx, tenantID, roleKeys)
	if err != nil {
		return err
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: 角色键未在词表中注册: %s", ErrInvalidInput, strings.Join(unknown, ", "))
	}
	return nil
}

// ReplaceEmployeeRolesRequest is the write shape for PUT .../roles.
type ReplaceEmployeeRolesRequest struct {
	TenantID       uuid.UUID
	EmployeeID     uuid.UUID
	ActorUserID    uuid.UUID
	RoleKeys       []string
	ConfirmImpact  bool
}

// ReplaceEmployeeRoles replaces the multi-value role set for one employee.
// Removing roles that still appear in project castings requires ConfirmImpact.
func (s *Service) ReplaceEmployeeRoles(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) ([]string, error) {
	return s.ReplaceEmployeeRolesWithImpact(ctx, ReplaceEmployeeRolesRequest{
		TenantID:   tenantID,
		EmployeeID: employeeID,
		RoleKeys:   roleKeys,
	})
}

// ReplaceEmployeeRolesWithImpact is the confirm_impact-aware role replace.
func (s *Service) ReplaceEmployeeRolesWithImpact(ctx context.Context, req ReplaceEmployeeRolesRequest) ([]string, error) {
	if s.roleStore == nil {
		return nil, fmt.Errorf("employee role store not configured")
	}
	if req.TenantID == uuid.Nil || req.EmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and employee_id are required", ErrInvalidInput)
	}
	emp, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.EmployeeID)
	if err != nil {
		return nil, err
	}
	normalized := normalizeRoleKeys(req.RoleKeys)
	if err := s.validateRoleKeys(ctx, req.TenantID, normalized); err != nil {
		return nil, err
	}

	// Diff removed keys against current bindings.
	current, err := s.roleStore.ListRoleKeys(ctx, req.TenantID, req.EmployeeID)
	if err != nil {
		return nil, err
	}
	kept := map[string]struct{}{}
	for _, k := range normalized {
		kept[k] = struct{}{}
	}
	var removed []string
	for _, k := range current {
		if _, ok := kept[k]; !ok {
			removed = append(removed, k)
		}
	}

	var impact CastingRoleImpact
	if len(removed) > 0 && s.castingImpact != nil {
		impact, err = s.castingImpact.ListEmployeeRoleImpact(ctx, req.TenantID, req.EmployeeID, removed)
		if err != nil {
			return nil, err
		}
		if impact.AffectedCount > 0 && !req.ConfirmImpact {
			return nil, &ErrCastingImpactRequiresConfirm{Impact: impact}
		}
	}

	if impact.AffectedCount > 0 && s.castingImpact != nil {
		// Same transaction: role replace + casting deletes; events/alerts after commit.
		if err := s.castingImpact.CommitRoleReplaceWithCascade(ctx, RoleReplaceCascadeRequest{
			TenantID:     req.TenantID,
			EmployeeID:   req.EmployeeID,
			ActorUserID:  req.ActorUserID,
			NewRoleKeys:  normalized,
			RemovedKeys:  removed,
			EmployeeName: emp.Name,
		}); err != nil {
			return nil, err
		}
		return normalized, nil
	}

	if err := s.roleStore.ReplaceRoleKeys(ctx, req.TenantID, req.EmployeeID, normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// GetEmployeeRoleImpact previews casting rows that would drop if roleKeys were removed.
// Empty roleKeys = impact of removing ALL currently held roles.
func (s *Service) GetEmployeeRoleImpact(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) (CastingRoleImpact, error) {
	if tenantID == uuid.Nil || employeeID == uuid.Nil {
		return CastingRoleImpact{}, fmt.Errorf("%w: tenant_id and employee_id are required", ErrInvalidInput)
	}
	if _, err := s.repository.GetDigitalEmployee(ctx, tenantID, employeeID); err != nil {
		return CastingRoleImpact{}, err
	}
	if s.castingImpact == nil {
		return CastingRoleImpact{AffectedCastings: []CastingImpactRow{}, AffectedCount: 0}, nil
	}
	keys := normalizeRoleKeys(roleKeys)
	if len(keys) == 0 && s.roleStore != nil {
		// role_keys omitted → all current roles
		current, err := s.roleStore.ListRoleKeys(ctx, tenantID, employeeID)
		if err != nil {
			return CastingRoleImpact{}, err
		}
		keys = current
	}
	if len(keys) == 0 {
		return CastingRoleImpact{AffectedCastings: []CastingImpactRow{}, AffectedCount: 0}, nil
	}
	return s.castingImpact.ListEmployeeRoleImpact(ctx, tenantID, employeeID, keys)
}

func normalizeRoleKeys(keys []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(keys))
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// CapabilityVocabularyValidator 返回未在租户词表中注册为 active 的能力键子集。
// 由 scenariotemplate.Service 实现,app.go 注入(employee 不依赖该包)。
type CapabilityVocabularyValidator interface {
	ValidateCapabilityKeys(ctx context.Context, tenantID uuid.UUID, keys []string) ([]string, error)
}

// SetCapabilityVocabularyValidator 注入能力词表校验器。
func (s *Service) SetCapabilityVocabularyValidator(validator CapabilityVocabularyValidator) {
	s.vocabulary = validator
}

// validateDeclaredCapabilities 拒绝未注册的能力键。词表是模板 required_capabilities
// 与员工声明的共用词表(能力差异走注册表,不进代码枚举);单边校验等于没有词表。
func (s *Service) validateDeclaredCapabilities(ctx context.Context, tenantID uuid.UUID, bindings map[string]any) error {
	if s.vocabulary == nil {
		return nil
	}
	keys := stringList(bindings["external_capabilities"])
	if len(keys) == 0 {
		return nil
	}
	unknown, err := s.vocabulary.ValidateCapabilityKeys(ctx, tenantID, keys)
	if err != nil {
		return err
	}
	if len(unknown) > 0 {
		return fmt.Errorf("%w: 能力键未在词表中注册: %s", ErrInvalidInput, strings.Join(unknown, ", "))
	}
	return nil
}

// SetSystemConfigReader 注入配置中心读取器；未注入（测试）时使用注册表默认值。
func (s *Service) SetSystemConfigReader(reader systemconfig.Reader) {
	s.systemConfig = reader
}

// SetPermissionApprovalDependencies 注入权限审批接缝:产生 role/permission 治理审批请求(approvals)
// + 解析团队审批人(router)。由 app.go 在 approvalService/permissionRouter 构造后调用(setter 注入,
// 与 SetSystemConfigReader 同模式,避免构造顺序耦合)。
func (s *Service) SetPermissionApprovalDependencies(approvals *approval.Service, router *permission.ApproverRouter) {
	s.approvals = approvals
	s.router = router
}

// maxDigitalEmployeesPerTeam 单团队在册数字员工上限（配置中心 employee.max_per_team）。
func (s *Service) maxDigitalEmployeesPerTeam(ctx context.Context, tenantID uuid.UUID) int32 {
	if s.systemConfig == nil {
		return int32(systemconfig.DefaultFor(systemconfig.KeyEmployeeMaxPerTeam))
	}
	return int32(s.systemConfig.Int64(ctx, tenantID, systemconfig.KeyEmployeeMaxPerTeam))
}

const defaultProvisioningPollInterval = 250 * time.Millisecond

var supportedDigitalEmployeeProviderTypes = map[string]struct{}{
	"claude-code": {},
	"opencode":    {},
	"codex":       {},
}

// supportedDigitalEmployeeRoles 收敛提交治理变更时可接受的 role(防任意 role)。
// TODO: 以后改为从 employee type 注册表的 DefaultRole 动态取,当前封闭集合是务实过渡。
var supportedDigitalEmployeeRoles = map[string]struct{}{
	"requirements_analyst": {},
	"backend_engineer":     {},
	"frontend_engineer":    {},
	"qa_engineer":          {},
	"code_reviewer":        {},
	"devops_engineer":      {},
	"postgres_operator":    {},
	"finance_reviewer":     {},
	"e2e-capability-probe": {},
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
	registrySkills, err := s.repository.ListSkillCapabilityOptions(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list skill capability options: %w", err)
	}
	registryMCPServers, err := s.repository.ListMCPCapabilityOptions(ctx, req.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list mcp capability options: %w", err)
	}
	capabilityOptions := capabilityOptionsForCreate(employeeTypes, registrySkills, registryMCPServers)

	creationChecks := createOptionChecks(
		teamConfigOption,
		employeeTypes,
		capabilityOptions,
		runtimeOptions,
	)
	if !teamLess {
		capacityCheck, err := s.teamCapacityCheck(ctx, req.TenantID, *req.TeamID)
		if err != nil {
			return nil, err
		}
		creationChecks = append(creationChecks, capacityCheck)
	}

	return &CreateOptions{
		TeamConfig:             teamConfigOption,
		EmployeeTypes:          employeeTypes,
		CapabilityOptions:      capabilityOptions,
		RuntimeProviderOptions: append([]RuntimeProviderOption(nil), runtimeOptions...),
		CreationChecks:         creationChecks,
		PolicyDefaults:         emptyPolicyDefaults(),
	}, nil
}

// teamCapacityCheck 团队容量预检：满员时 blocked，让创建向导在选完团队的
// 那一刻即可见，而不是走完三步在最终提交时才失败。
func (s *Service) teamCapacityCheck(ctx context.Context, tenantID, teamID uuid.UUID) (CreateOptionCheck, error) {
	overview, err := s.repository.GetDigitalEmployeeOverview(ctx, GetDigitalEmployeeOverviewRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
		Limit:    1,
	})
	if err != nil {
		return CreateOptionCheck{}, fmt.Errorf("get digital employee overview: %w", err)
	}
	limit := s.maxDigitalEmployeesPerTeam(ctx, tenantID)
	count := overview.Pagination.TotalCount
	if count >= limit {
		return CreateOptionCheck{
			Key:     "team_capacity",
			Label:   "团队容量",
			Status:  "blocked",
			Message: fmt.Sprintf("团队已满员（%d/%d），请在系统配置调大上限或更换团队。", count, limit),
		}, nil
	}
	return CreateOptionCheck{
		Key:     "team_capacity",
		Label:   "团队容量",
		Status:  "passed",
		Message: fmt.Sprintf("已有 %d / 上限 %d", count, limit),
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

	availableSkillCount := countAvailableCapabilityOptions(capabilityOptions.Skills)
	availableMCPCount := countAvailableCapabilityOptions(capabilityOptions.MCPServers)
	capabilityCount := availableSkillCount + availableMCPCount

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
			Message: fmt.Sprintf("%d 个可用模板", countRealTemplates(employeeTypes)),
		},
		{
			Key:     "capability_policy",
			Label:   "能力边界",
			Status:  checkStatus(capabilityCount > 0 || len(capabilityOptions.ProviderTypes) > 0, false),
			Message: fmt.Sprintf("技能 %d · MCP %d", availableSkillCount, availableMCPCount),
		},
		{
			Key:     "runtime_provider",
			Label:   "Provider 类型预览",
			Status:  checkStatus(availableRuntimeCount > 0, true),
			Message: runtimeProviderCreateOptionMessage(availableRuntimeCount, len(runtimeOptions), inactiveRuntimeSessionCount),
		},
	}
}

// countRealTemplates 排除 custom_agent 哨兵（空白自定义的内部类型），
// 与模板选择表格的口径一致。
func countRealTemplates(employeeTypes []EmployeeTypeDefinition) int {
	count := 0
	for _, definition := range employeeTypes {
		if definition.Type != "custom_agent" {
			count++
		}
	}
	return count
}

func runtimeProviderCreateOptionMessage(availableRuntimeCount, totalRuntimeCount, inactiveRuntimeSessionCount int) string {
	message := fmt.Sprintf("%d/%d 个 Provider 候选当前可用于调度；创建时不绑定 Runtime 节点", availableRuntimeCount, totalRuntimeCount)
	if availableRuntimeCount == 0 && inactiveRuntimeSessionCount > 0 {
		message = fmt.Sprintf("%d/%d 个 Provider 候选当前可用于调度；%d 个 Runtime 会话未激活", availableRuntimeCount, totalRuntimeCount, inactiveRuntimeSessionCount)
	}
	return message
}

func countAvailableCapabilityOptions(items []CapabilityOptionItem) int {
	count := 0
	for _, item := range items {
		if item.Available {
			count++
		}
	}
	return count
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

// capabilityOptionsForCreate merges the tenant registry (the authoritative
// candidate source) with employee-type template recommendations. Registry
// entries are Available; template-recommended keys missing from the registry
// are appended with Available=false so the console can explain instead of
// silently hiding them.
func capabilityOptionsForCreate(employeeTypes []EmployeeTypeDefinition, registrySkills, registryMCPServers []CapabilityRegistryOption) CapabilityOptions {
	return CapabilityOptions{
		ProviderTypes: supportedProviderTypes(),
		Skills:        mergeCapabilityOptionItems(registrySkills, platformSkillOptions(employeeTypes)),
		MCPServers:    mergeCapabilityOptionItems(registryMCPServers, platformMCPServerOptions(employeeTypes)),
	}
}

func mergeCapabilityOptionItems(registry []CapabilityRegistryOption, recommendedKeys []string) []CapabilityOptionItem {
	recommended := make(map[string]struct{}, len(recommendedKeys))
	for _, key := range recommendedKeys {
		recommended[key] = struct{}{}
	}
	items := make([]CapabilityOptionItem, 0, len(registry)+len(recommendedKeys))
	seen := make(map[string]struct{}, len(registry))
	for _, option := range registry {
		id := option.ID
		label := strings.TrimSpace(option.Label)
		if label == "" {
			label = option.Key
		}
		_, isRecommended := recommended[option.Key]
		items = append(items, CapabilityOptionItem{
			Key:         option.Key,
			ID:          &id,
			Label:       label,
			Description: option.Description,
			Recommended: isRecommended,
			Available:   true,
			RiskLevel:   option.RiskLevel,
		})
		seen[option.Key] = struct{}{}
	}
	for _, key := range recommendedKeys {
		if _, ok := seen[key]; ok {
			continue
		}
		items = append(items, CapabilityOptionItem{
			Key:         key,
			Label:       key,
			Description: "模板推荐,注册表未上架",
			Recommended: true,
			Available:   false,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Key < items[j].Key })
	return items
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

// ListAvatarAssets returns the built-in avatar library with per-tenant
// in-use flags: an asset already assigned to a live digital employee is
// exclusive and cannot be picked again at creation time.
func (s *Service) ListAvatarAssets(ctx context.Context, tenantID uuid.UUID) ([]DigitalEmployeeAvatarAsset, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	used, err := s.repository.ListUsedAvatarAssetIDs(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list used avatar asset ids: %w", err)
	}
	assets := ListDigitalEmployeeAvatarAssets()
	for i := range assets {
		if _, ok := used[assets[i].ID]; ok {
			assets[i].InUse = true
		}
	}
	return assets, nil
}

func (s *Service) ensureAvatarAssetAvailable(ctx context.Context, tenantID uuid.UUID, avatarAssetID string) error {
	used, err := s.repository.ListUsedAvatarAssetIDs(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("list used avatar asset ids: %w", err)
	}
	if _, ok := used[avatarAssetID]; ok {
		return ErrEmployeeAvatarInUse
	}
	return nil
}

func (s *Service) CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error) {
	normalized, definition, err := s.normalizeCreateDigitalEmployeeRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := s.ensureAvatarAssetAvailable(ctx, normalized.TenantID, normalized.AvatarAssetID); err != nil {
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

	roleKeys := normalizeRoleKeys(normalized.RoleKeys)
	if err := s.validateRoleKeys(ctx, normalized.TenantID, roleKeys); err != nil {
		return nil, err
	}

	var record DigitalEmployeeRecord
	if err := s.repository.WithTransaction(ctx, func(txRepo Repository) error {
		createdRecord, err := s.createLocalReadyEmployeeFacts(ctx, txRepo, normalized, definition, teamConfig)
		if err != nil {
			return err
		}
		record = createdRecord
		if s.roleStore != nil && len(roleKeys) > 0 {
			if err := s.roleStore.ReplaceRoleKeys(ctx, normalized.TenantID, createdRecord.ID, roleKeys); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	employee := employeeFromRecord(record)
	employee.RoleKeys = roleKeys
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
	if overview.Pagination.TotalCount >= s.maxDigitalEmployeesPerTeam(ctx, tenantID) {
		return ErrEmployeeTeamCapacityExceeded
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
	req.Skills = normalizeCapabilityKeys(req.Skills)
	req.MCPServers = normalizeCapabilityKeys(req.MCPServers)
	return req, definition, nil
}

func normalizeCapabilityKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]string, 0, len(keys))
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
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

// defaultRiskLevelForEmployeeType 恒返回 medium：曾经的 min_risk_for_human
// 推导依赖模板 DefaultApprovalPolicy，而该字段在 ToDefinition 中恒为空，属
// 已证实的死路径（员工个体策略字段随迁移 20260719124500 一并下线）。
func defaultRiskLevelForEmployeeType(EmployeeTypeDefinition) string {
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
	if err := bindInitialCapabilities(ctx, repository, record, req, teamConfig); err != nil {
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

// bindInitialCapabilities persists the creation-time skill/MCP selections as
// logical binding rows inside the create transaction. Keys already covered by
// the team baseline are skipped silently (the employee inherits them); keys
// missing from the tenant registry reject the whole creation with 400.
func bindInitialCapabilities(ctx context.Context, repository Repository, record DigitalEmployeeRecord, req CreateDigitalEmployeeRequest, teamConfig TeamConfigInput) error {
	teamSkills := toStringSet(optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_skills"))
	teamMCPServers := toStringSet(optionalStringListFromPolicy(teamConfig.CapabilityPolicy, "allowed_mcp_servers"))

	skillSlugs := withoutKeys(req.Skills, teamSkills)
	if len(skillSlugs) > 0 {
		resolved, err := repository.ResolveSkillIDsBySlugs(ctx, req.TenantID, skillSlugs)
		if err != nil {
			return fmt.Errorf("resolve skill slugs: %w", err)
		}
		skillIDs := make([]uuid.UUID, 0, len(skillSlugs))
		for _, slug := range skillSlugs {
			id, ok := resolved[slug]
			if !ok {
				return fmt.Errorf("%w: unknown skill slug %q", ErrInvalidInput, slug)
			}
			skillIDs = append(skillIDs, id)
		}
		if err := repository.BindSkillsToEmployee(ctx, req.TenantID, record.ID, skillIDs); err != nil {
			return fmt.Errorf("bind initial skills: %w", err)
		}
	}

	serverKeys := withoutKeys(req.MCPServers, teamMCPServers)
	if len(serverKeys) > 0 {
		resolved, err := repository.ResolveMCPServerIDsByKeys(ctx, req.TenantID, serverKeys)
		if err != nil {
			return fmt.Errorf("resolve mcp server keys: %w", err)
		}
		serverIDs := make([]uuid.UUID, 0, len(serverKeys))
		for _, key := range serverKeys {
			id, ok := resolved[key]
			if !ok {
				return fmt.Errorf("%w: unknown mcp server key %q", ErrInvalidInput, key)
			}
			serverIDs = append(serverIDs, id)
		}
		if err := repository.BindMCPServersToEmployee(ctx, req.TenantID, record.ID, serverIDs); err != nil {
			return fmt.Errorf("bind initial mcp servers: %w", err)
		}
	}
	return nil
}

func toStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func withoutKeys(keys []string, excluded map[string]struct{}) []string {
	filtered := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := excluded[key]; ok {
			continue
		}
		filtered = append(filtered, key)
	}
	return filtered
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

// initialCapabilitySelection composes the non-binding capability declaration
// stored on the config revision. Skill and MCP logical bindings live in the
// binding tables (see bindInitialCapabilities); template defaults for those
// two keys are expressed as create-options recommendations, not silently
// merged server-side.
func initialCapabilitySelection(req CreateDigitalEmployeeRequest, definition EmployeeTypeDefinition, teamConfig TeamConfigInput) map[string]any {
	defaults := cloneMap(definition.CapabilityBindings)
	delete(defaults, "skills")
	delete(defaults, "mcp_servers")
	return mergePolicyMaps(defaults, req.CapabilityBindings)
}
func mergePolicyMaps(base, override map[string]any) map[string]any {
	merged := cloneMap(base)
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

// normalizeCapabilityBindings keeps only the non-binding declaration keys.
// skills/mcp_servers are always stripped: their authoritative source is the
// binding tables (skill_agent_bindings / digital_employee_mcp_bindings_v2),
// never the config revision JSON.
func normalizeCapabilityBindings(input map[string]any) map[string]any {
	bindings := cloneMap(input)
	if bindings == nil {
		bindings = map[string]any{}
	}
	delete(bindings, "skills")
	delete(bindings, "mcp_servers")
	for _, key := range []string{"external_capabilities", "environment_variable_refs"} {
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
		item := map[string]any{
			"skill_id":                payload.SkillID,
			"skill_key":               payload.SkillKey,
			"revision_id":             payload.RevisionID,
			"archive_object_ref":      payload.ArchiveObjectRef,
			"archive_checksum_sha256": payload.ArchiveChecksumSHA256,
			"archive_size_bytes":      payload.ArchiveSizeBytes,
			"archive_file_count":      payload.ArchiveFileCount,
		}
		if s.SourceScope != "" {
			item["source_scope"] = s.SourceScope
		}
		if s.Version != "" {
			item["version"] = s.Version
		}
		out = append(out, item)
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
	if s.roleStore != nil {
		if keys, err := s.roleStore.ListRoleKeys(ctx, tenantID, employeeID); err != nil {
			slog.Default().Warn("attach employee role keys failed", "employee_id", employeeID, "error", err)
		} else {
			employee.RoleKeys = keys
		}
	}
	// 附上与总览/列表同源的 operational_state(跨视图一致性 P2 3.3a);裁决失败不阻断
	// 员工读取,仅记录并留空(前端回退到既有本地判断),避免详情页因运行态查询挂掉打不开。
	if state, stateErr := s.repository.GetDigitalEmployeeOperationalState(ctx, tenantID, employeeID); stateErr != nil {
		slog.Default().Warn("attach operational state failed", "employee_id", employeeID, "error", stateErr)
	} else {
		employee.OperationalState = &state
	}
	if affiliation, affiliationErr := s.repository.GetDigitalEmployeeDetailAffiliation(ctx, tenantID, employeeID); affiliationErr != nil {
		slog.Default().Warn("attach detail affiliation failed", "employee_id", employeeID, "error", affiliationErr)
		employee.ProjectSummary = DigitalEmployeeProjectSummary{Projects: []DigitalEmployeeProjectLinkSummary{}}
	} else {
		employee.TeamName = affiliation.TeamName
		employee.ProjectSummary = affiliation.ProjectSummary
		if employee.ProjectSummary.Projects == nil {
			employee.ProjectSummary.Projects = []DigitalEmployeeProjectLinkSummary{}
		}
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

func (s *Service) UpdateProfile(ctx context.Context, req UpdateProfileRequest) (*DigitalEmployee, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: employee_id is required", ErrInvalidInput)
	}
	description := trimOptionalString(req.Description)
	record, err := s.repository.UpdateDigitalEmployeeProfile(ctx, req.TenantID, req.DigitalEmployeeID, description)
	if err != nil {
		return nil, fmt.Errorf("update digital employee profile: %w", err)
	}
	employee := employeeFromRecord(record)
	if err := s.attachLatestConfigRevision(ctx, employee); err != nil {
		return nil, err
	}
	if affiliation, affiliationErr := s.repository.GetDigitalEmployeeDetailAffiliation(ctx, req.TenantID, req.DigitalEmployeeID); affiliationErr != nil {
		slog.Default().Warn("attach detail affiliation failed", "employee_id", req.DigitalEmployeeID, "error", affiliationErr)
		employee.ProjectSummary = DigitalEmployeeProjectSummary{Projects: []DigitalEmployeeProjectLinkSummary{}}
	} else {
		employee.TeamName = affiliation.TeamName
		employee.ProjectSummary = affiliation.ProjectSummary
		if employee.ProjectSummary.Projects == nil {
			employee.ProjectSummary.Projects = []DigitalEmployeeProjectLinkSummary{}
		}
	}
	return employee, nil
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
	// ListDigitalEmployeeDetachBlockers 与 tenant 包移出团队走同一条 sqlc 查询，
	// 两处判据必须一致，否则"移出"拦得住、"换队"绕得过。
	ListDigitalEmployeeDetachBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]teamguard.DetachBlocker, error)
}

// ReassignTeam 换队/首次归队。副作用提示：员工的 agent home dir 按
// (team, employee) 键，换队后下次派发落新家目录（provider 会话连续性重置）；
// 团队级技能与 MCP 绑定继承随之切换。
//
// 两道守卫：
//   - 目标团队员工数限额（与创建路径同一判据，此前换队绕过它）。
//   - 已有归属的员工换队 = 先脱离原团队，套用与"移出团队"相同的在役/项目阻断；
//     从候岗大厅首次归队不涉及脱离，不套。
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
	current, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	if current.TeamID != nil && *current.TeamID == req.TeamID {
		// 目标就是当前团队：幂等返回，不做限额与阻断判定。
		return employeeFromRecord(current), nil
	}
	if err := s.ensureTeamDigitalEmployeeCapacity(ctx, req.TenantID, req.TeamID); err != nil {
		return nil, err
	}
	if current.TeamID != nil {
		blockers, err := reassigner.ListDigitalEmployeeDetachBlockers(ctx, req.TenantID, req.DigitalEmployeeID)
		if err != nil {
			return nil, fmt.Errorf("list detach blockers: %w", err)
		}
		if err := teamguard.BlockedError(blockers, "换队"); err != nil {
			return nil, err
		}
	}
	record, err := reassigner.ReassignDigitalEmployeeTeam(ctx, req)
	if err != nil {
		return nil, err
	}
	return employeeFromRecord(record), nil
}

func (s *Service) CreateConfigRevision(ctx context.Context, req CreateDigitalEmployeeConfigRevisionRequest) (*DigitalEmployeeConfigRevision, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if err := rejectLegacyCapabilityBindingKeys(req.CapabilityBindings); err != nil {
		return nil, err
	}
	employee, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	// 员工配置页 spec §6.2(A2 前身):当前修订只承载非权限治理字段(persona/能力/预算),保存即生效——
	// 创建即 active(自动批准),配合派发只认 active(GetCurrent)后才真正生效。待迁移为修订加 role/
	// permission_policy 列后,再把这些权限项分支为 draft→权限中心审批→ActivateConfigRevision 翻 active。
	// 自动批准人固定为员工 owner:客户端提供的 approved_by 一律忽略(防伪造审批人),
	// 与 handler 剥离 approved_by 的既有安全护栏一致。
	approvedBy := employee.OwnerUserID
	approvedAt := time.Now().UTC()
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
	if err := s.validateDeclaredCapabilities(ctx, req.TenantID, capabilityBindings); err != nil {
		return nil, err
	}
	budgetPolicySource := inheritedConfigMap(req.BudgetPolicy, latestConfig, func(config EmployeeConfigInput) map[string]any {
		return config.BudgetPolicy
	})
	budgetPolicy, err := normalizeBudgetPolicy(budgetPolicySource)
	if err != nil {
		return nil, err
	}
	params := CreateConfigRevisionParams{
		TenantID:              req.TenantID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		PersonaMemoryMarkdown: personaMemoryMarkdown,
		CapabilityBindings:    capabilityBindings,
		BudgetPolicy:          budgetPolicy,
		Status:                ConfigRevisionStatusActive,
		ApprovedBy:            &approvedBy,
		ApprovedAt:            &approvedAt,
	}
	// 非权限治理字段保存即生效:同事务归档旧 active 让位,再创建新 active,原子维持「每员工至多一条
	// active」不变式(偏唯一索引 uq_digital_employee_config_revisions_active)。
	var record DigitalEmployeeConfigRevisionRecord
	if txErr := s.repository.WithTransaction(ctx, func(txRepo Repository) error {
		nextRevision, err := txRepo.GetNextDigitalEmployeeConfigRevisionNumber(ctx, req.TenantID, req.DigitalEmployeeID)
		if err != nil {
			return fmt.Errorf("get next digital employee config revision number: %w", err)
		}
		params.RevisionNumber = nextRevision
		if err := txRepo.ArchivePriorActiveConfigRevisions(ctx, req.TenantID, req.DigitalEmployeeID); err != nil {
			return fmt.Errorf("archive prior active config revision: %w", err)
		}
		created, err := txRepo.CreateDigitalEmployeeConfigRevision(ctx, params)
		if err != nil {
			return fmt.Errorf("create digital employee config revision: %w", err)
		}
		record = created
		return nil
	}); txErr != nil {
		return nil, txErr
	}
	return configRevisionFromRecord(record), nil
}

// SubmitPermissionChange 提交 role/permission_policy 治理变更:校验→解析审批人→产生 category=permission
// 审批请求(目标值随 ContextPayload 承载)。批准时权限中心调 ActivateConfigRevision 写回员工行。
// 方案2:权限变更不进 config_revision;目标值由审批请求承载。
func (s *Service) SubmitPermissionChange(ctx context.Context, req SubmitPermissionChangeRequest) (*approval.ApprovalRequest, error) {
	if s.approvals == nil || s.router == nil {
		return nil, ErrPermissionApprovalNotConfigured
	}
	if req.TenantID == uuid.Nil || req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and digital_employee_id are required", ErrInvalidInput)
	}
	if req.Role == nil && req.PermissionPolicy == nil {
		return nil, ErrPermissionChangeEmpty
	}
	if req.Role != nil {
		if trimmed := strings.TrimSpace(*req.Role); trimmed == "" {
			return nil, fmt.Errorf("%w: role must not be blank", ErrInvalidInput)
		} else if !s.isValidRole(trimmed) {
			return nil, fmt.Errorf("%w: role not recognized", ErrInvalidInput)
		}
	}

	employee, err := s.repository.GetDigitalEmployee(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee: %w", err)
	}
	if employee.TeamID == nil {
		return nil, fmt.Errorf("%w: employee must belong to a team before permission change", ErrInvalidInput)
	}
	// 提交即拒护栏(务实版):员工当前有进行中工作,role 变更影响在役执行,拒绝提交。
	// 完整在役项目 SoD/role_independence 校验需跨 project 域,作为后续独立项。
	if busy, _ := s.employeeBusy(ctx, req.TenantID, req.DigitalEmployeeID); busy {
		return nil, ErrPermissionChangeBusy
	}

	approver, err := s.router.ResolveTeamApprover(ctx, req.TenantID, *employee.TeamID, req.RequesterUserID)
	if err != nil {
		return nil, fmt.Errorf("resolve team approver: %w", err)
	}

	// 目标值 + current/after diff 随 ContextPayload 承载,供权限中心弹窗渲染 + 批准时写回。
	payload := map[string]any{
		"employee_id":   req.DigitalEmployeeID.String(),
		"employee_name": employee.Name,
		"current_role":  employee.Role,
		"requested_by":  req.RequesterUserID.String(),
	}
	if req.Role != nil {
		payload["target_role"] = *req.Role
	}
	if req.PermissionPolicy != nil {
		payload["target_permission_policy"] = req.PermissionPolicy
		// 保留当前 permission_policy 供 diff 渲染(map 克隆避免引用)。
		payload["current_permission_policy"] = cloneMap(employee.PermissionPolicy)
	}

	risk := "high"
	summary := permissionChangeSummary(employee.Role, employee.Name, req.Role, req.PermissionPolicy != nil)
	return s.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       req.TenantID,
		ResourceType:   permission.ResourceTypeEmployeeConfigRevision,
		ResourceID:     req.DigitalEmployeeID,
		RequesterType:  "human_user",
		RequesterID:    &req.RequesterUserID,
		TargetUserID:   approver,
		DecisionType:   "approved",
		Title:          fmt.Sprintf("审批数字员工 %s 的权限变更", employee.Name),
		Summary:        summary,
		RiskLevel:      risk,
		Category:       approval.ApprovalCategoryPermission,
		ContextPayload: payload,
	})
}

// ActivateConfigRevision 实现权限中心接缝 permission.ConfigRevisionActivator:
// 权限审批批准时调用,把 ContextPayload 承载的目标 role/permission_policy 写回员工行(幂等)。
func (s *Service) ActivateConfigRevision(ctx context.Context, in permission.ActivateConfigRevisionInput) error {
	if in.TenantID == uuid.Nil || in.EmployeeID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id and employee_id are required", ErrInvalidInput)
	}
	role, hasRole := stringFromPayload(in.ContextPayload, "target_role")
	permissionPolicy, hasPolicy := mapFromPayload(in.ContextPayload, "target_permission_policy")
	if !hasRole && !hasPolicy {
		return fmt.Errorf("%w: permission approval payload carries no target role/policy", ErrInvalidInput)
	}
	// 单侧变更时,另一侧从员工行回填,避免把未变更侧覆盖为空(UPDATE 两列都写)。
	targetRole := role
	if !hasRole || !hasPolicy {
		employee, err := s.repository.GetDigitalEmployee(ctx, in.TenantID, in.EmployeeID)
		if err != nil {
			return fmt.Errorf("get digital employee for activation: %w", err)
		}
		if !hasRole {
			targetRole = employee.Role
		}
		if !hasPolicy {
			permissionPolicy = cloneMap(employee.PermissionPolicy)
		}
	}
	if _, err := s.repository.UpdateDigitalEmployeeRolePermission(ctx, in.TenantID, in.EmployeeID, targetRole, permissionPolicy); err != nil {
		return fmt.Errorf("write back role/permission on activation: %w", err)
	}
	return nil
}

// isValidRole 校验 role 在已知 employee type 默认角色集合内(防任意 role)。
func (s *Service) isValidRole(role string) bool {
	_, ok := supportedDigitalEmployeeRoles[role]
	return ok
}

// employeeBusy 报告员工是否有进行中工作(派发运行态)。完整在役项目占用校验见后续独立项。
func (s *Service) employeeBusy(ctx context.Context, tenantID, employeeID uuid.UUID) (bool, error) {
	state, err := s.repository.GetDigitalEmployeeOperationalState(ctx, tenantID, employeeID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return state.Status == DigitalEmployeeOperationalStatusWorking || state.Status == DigitalEmployeeOperationalStatusQueued, nil
}

func permissionChangeSummary(currentRole, name string, role *string, hasPolicy bool) string {
	parts := []string{}
	if role != nil {
		if currentRole == *role {
			parts = append(parts, fmt.Sprintf("角色保持 %s", *role))
		} else {
			parts = append(parts, fmt.Sprintf("角色 %s → %s", currentRole, *role))
		}
	}
	if hasPolicy {
		parts = append(parts, "更新权限策略(permission_policy)")
	}
	if len(parts) == 0 {
		return name
	}
	return name + ":" + strings.Join(parts, "、")
}

func stringFromPayload(payload map[string]any, key string) (string, bool) {
	value, ok := payload[key]
	if !ok {
		return "", false
	}
	str, ok := value.(string)
	if !ok {
		return "", false
	}
	return str, true
}

func mapFromPayload(payload map[string]any, key string) (map[string]any, bool) {
	value, ok := payload[key]
	if !ok {
		return nil, false
	}
	m, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	return m, true
}

// ConfigRevisionActivatorAdapter 把 employee.Service 包成 permission.ConfigRevisionActivator。
type ConfigRevisionActivatorAdapter struct {
	service *Service
}

func NewConfigRevisionActivatorAdapter(service *Service) *ConfigRevisionActivatorAdapter {
	return &ConfigRevisionActivatorAdapter{service: service}
}

func (a *ConfigRevisionActivatorAdapter) ActivateConfigRevision(ctx context.Context, in permission.ActivateConfigRevisionInput) error {
	return a.service.ActivateConfigRevision(ctx, in)
}

// rejectLegacyCapabilityBindingKeys blocks config revisions that still try to
// declare skills/mcp_servers in capability_bindings. Empty arrays (inherited
// residue from stripped revisions) are tolerated and silently dropped by
// normalizeCapabilityBindings.
func rejectLegacyCapabilityBindingKeys(bindings map[string]any) error {
	for _, key := range []string{"skills", "mcp_servers"} {
		if values := stringList(bindings[key]); len(values) > 0 {
			return fmt.Errorf("%w: capability_bindings.%s is no longer supported; manage %s bindings via the dedicated binding endpoints", ErrInvalidInput, key, key)
		}
	}
	return nil
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
