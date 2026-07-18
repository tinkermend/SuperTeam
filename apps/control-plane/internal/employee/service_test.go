package employee

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	cpruntime "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestBuiltinEmployeeTemplateFixturesAreEmptyByDefault(t *testing.T) {
	// The platform ships no default digital employee templates — every
	// tenant starts with an empty catalog and templates must be created
	// explicitly via CreateEmployeeTemplate.
	require.Empty(t, builtinEmployeeTemplateFixtures(uuid.New()))
}

func TestCustomAgentEmployeeTypeDefinitionIsAvailableForBlankCustomCreate(t *testing.T) {
	definition := customAgentEmployeeTypeDefinition()
	require.Equal(t, "custom_agent", definition.Type)
	require.Equal(t, "自定义数字员工", definition.Label)
	require.Empty(t, definition.DefaultRole)
	require.Empty(t, definition.RecommendedSkills)
	require.Empty(t, definition.RecommendedMCPServers)
	require.Empty(t, definition.PersonaMemoryMarkdown)
	require.Empty(t, definition.CapabilityBindings)
	require.Empty(t, definition.BudgetPolicy)
	require.Contains(t, definition.Metadata, "creation_mode")
	require.Equal(t, "blank_custom", definition.Metadata["creation_mode"])
}

func TestGetCreateOptionsReturnsTeamBaselineAndPlatformCandidates(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.teamBaselines[teamID] = TeamBaseline{
		Constitution: map[string]any{"mission": "keep services healthy"},
		Skills:       []string{"database-troubleshooting", "incident-diagnosis"},
		MCPServers:   []string{"postgres-readonly"},
	}
	repo.runtimeProviderOptions = []RuntimeProviderOption{{
		RuntimeNodeID:         runtimeNodeID,
		NodeID:                "node-ops-01",
		RuntimeName:           "运维节点 01",
		ProviderType:          "codex",
		RuntimeStatus:         "online",
		ProviderStatus:        "healthy",
		HealthStatus:          "healthy",
		CurrentLoad:           1,
		MaxSlots:              4,
		AgentHomeDir:          "/srv/superteam/agents",
		AgentHomeDirAvailable: true,
		Available:             true,
		DisabledReason:        "",
	}}

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})
	if err != nil {
		t.Fatalf("get create options: %v", err)
	}

	if options.TeamConfig.ID != uuid.Nil {
		t.Fatalf("unexpected team config option: %#v", options.TeamConfig)
	}
	if got := options.TeamConfig.Skills; len(got) != 2 || got[0] != "database-troubleshooting" {
		t.Fatalf("expected baseline skills, got %#v", got)
	}
	if len(options.EmployeeTypes) != len(builtinEmployeeTemplateFixtures(tenantID))+1 {
		t.Fatalf("expected full employee types, got %#v", options.EmployeeTypes)
	}
	if len(options.RuntimeProviderOptions) != 1 || !options.RuntimeProviderOptions[0].Available {
		t.Fatalf("expected available runtime provider option, got %#v", options.RuntimeProviderOptions)
	}
	if got := options.CapabilityOptions.ProviderTypes; len(got) != 3 {
		t.Fatalf("expected full provider types, got %#v", got)
	}
}

func TestCreateOptionChecksDescribeInactiveRuntimeSessions(t *testing.T) {
	runtimeOptions := []RuntimeProviderOption{
		{
			RuntimeNodeID:         uuid.New(),
			NodeID:                "runtime-a",
			RuntimeName:           "Runtime A",
			ProviderType:          "codex",
			RuntimeStatus:         "online",
			ProviderStatus:        "healthy",
			HealthStatus:          "healthy",
			AgentHomeDirAvailable: true,
			Available:             false,
			DisabledReason:        "runtime_session_inactive",
		},
	}

	checks := createOptionChecks(
		TeamConfigCreateOption{Skills: []string{"sql-review"}},
		[]EmployeeTypeDefinition{customAgentEmployeeTypeDefinition()},
		CapabilityOptions{ProviderTypes: []string{"codex"}},
		runtimeOptions,
	)

	require.Len(t, checks, 4)
	require.Equal(t, "runtime_provider", checks[3].Key)
	require.Equal(t, "warning", checks[3].Status)
	require.Equal(t, "0/1 个 Provider 候选当前可用于调度；1 个 Runtime 会话未激活", checks[3].Message)
	require.NotContains(t, checks[3].Message, "当前在线")
}

func TestGetCreateOptionsIgnoresEmptyAllowedEmployeeTypes(t *testing.T) {
	svc, _, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_employee_types": []any{},
	}, nil)

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.NoError(t, err)
	require.NotEmpty(t, options.EmployeeTypes)
}

func TestGetCreateOptionsSupportsTeamLessWithBuiltInDefaults(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.runtimeProviderOptions = []RuntimeProviderOption{{
		RuntimeNodeID:         runtimeNodeID,
		NodeID:                "node-team-less",
		RuntimeName:           "独立节点",
		ProviderType:          "codex",
		RuntimeStatus:         "online",
		ProviderStatus:        "healthy",
		HealthStatus:          "healthy",
		CurrentLoad:           0,
		MaxSlots:              2,
		AgentHomeDir:          "/srv/superteam/agents",
		AgentHomeDirAvailable: true,
		Available:             true,
		DisabledReason:        "",
	}}

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   nil,
	})
	if err != nil {
		t.Fatalf("get team-less create options: %v", err)
	}
	if options.TeamConfig.ID != uuid.Nil {
		t.Fatalf("expected nil team config id for team-less mode, got %s", options.TeamConfig.ID)
	}
	if len(options.EmployeeTypes) == 0 {
		t.Fatalf("expected built-in default employee types to be available in team-less mode, got %#v", options.EmployeeTypes)
	}
	if len(options.RuntimeProviderOptions) != 1 || !options.RuntimeProviderOptions[0].Available {
		t.Fatalf("expected team-less runtime provider option, got %#v", options.RuntimeProviderOptions)
	}
	if len(options.EmployeeTypes) == 0 {
		t.Fatalf("expected employee types, got %#v", options.EmployeeTypes)
	}
	definition := options.EmployeeTypes[0]
	if definition.CapabilityBindings == nil {
		t.Fatalf("expected create-options employee type capability bindings to use final field, got %#v", definition.CapabilityBindings)
	}
	if definition.BudgetPolicy == nil {
		t.Fatalf("expected create-options employee type budget policy to use final field, got %#v", definition.BudgetPolicy)
	}
	if options.PolicyDefaults.WorkspacePolicy == nil || options.PolicyDefaults.SessionPolicy == nil {
		t.Fatalf("expected create-options policy defaults to keep final fields, got %#v", options.PolicyDefaults)
	}
}

func TestGetCreateOptionsIgnoresMalformedAllowedEmployeeTypes(t *testing.T) {
	svc, _, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_employee_types": []any{"database_admin", 42},
	}, nil)

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.NoError(t, err)
	require.NotEmpty(t, options.EmployeeTypes)
}

func TestGetCreateOptionsIncludesCustomAgentForTeamLessCreate(t *testing.T) {
	svc, repo, tenantID, _ := newCreateOptionsTestService(t, map[string]any{}, map[string]any{})
	repo.currentTeamConfigByTeam = map[uuid.UUID]uuid.UUID{}

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{TenantID: tenantID})

	require.NoError(t, err)
	require.True(t, employeeTypeOptionExists(options.EmployeeTypes, "custom_agent"))
}

func TestCreateEmployeeWithoutTeamRevision(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	teamID := *req.TeamID
	delete(repo.currentTeamConfigByTeam, teamID)
	repo.teamBaselines[teamID] = TeamBaseline{
		Constitution: map[string]any{
			"mission": "protect production databases",
		},
		Skills:     []string{"database-troubleshooting", "incident-diagnosis"},
		MCPServers: []string{"postgres-readonly"},
	}
	repo.preflight.HasActiveTeamConfig = false
	delete(repo.preflight.GovernanceSnapshot, "team_config_revision_id")

	employee, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, employee)
	require.Equal(t, DigitalEmployeeStatusReady, employee.Status)
}

func TestCreateOptionsUsePlatformFullEmployeeTypes(t *testing.T) {
	svc, repo, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_employee_types": []any{"database_admin"},
		"allowed_provider_types": []any{"codex"},
		"allowed_skills":         []any{"database-troubleshooting"},
	}, nil)
	repo.teamBaselines[teamID] = TeamBaseline{
		Skills:     []string{"database-troubleshooting"},
		MCPServers: []string{"postgres-readonly"},
	}
	delete(repo.currentTeamConfigByTeam, teamID)

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.NoError(t, err)
	require.Len(t, options.EmployeeTypes, len(builtinEmployeeTemplateFixtures(tenantID))+1)
	require.True(t, employeeTypeOptionExists(options.EmployeeTypes, "custom_agent"))
	require.Contains(t, options.CapabilityOptions.ProviderTypes, "codex")
	require.Contains(t, options.CapabilityOptions.ProviderTypes, "opencode")
	require.Contains(t, options.CapabilityOptions.ProviderTypes, "claude-code")
}

func TestCreateOptionsUsesTeamBaseline(t *testing.T) {
	svc, repo, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_skills":      []any{"policy-only-skill"},
		"allowed_mcp_servers": []any{"policy-only-mcp"},
	}, nil)
	delete(repo.currentTeamConfigByTeam, teamID)
	repo.teamBaselines[teamID] = TeamBaseline{
		Constitution: map[string]any{
			"mission": "stabilize ops",
		},
		Skills:     []string{"baseline-skill", "shared-skill"},
		MCPServers: []string{"baseline-mcp"},
	}
	// Seed two tenant templates whose recommended skills/mcp servers differ
	// from the team baseline, to prove CapabilityOptions reflects the
	// tenant's template catalog rather than the team baseline.
	ctx := context.Background()
	_, err := repo.CreateEmployeeTemplate(ctx, CreateEmployeeTemplateParams{
		TenantID:              tenantID,
		Type:                  "database_admin",
		Label:                 "数据库管理",
		DefaultRole:           "database_admin",
		RecommendedSkills:     []string{"database-troubleshooting"},
		RecommendedMCPServers: []string{"postgres-readonly"},
	})
	require.NoError(t, err)
	_, err = repo.CreateEmployeeTemplate(ctx, CreateEmployeeTemplateParams{
		TenantID:              tenantID,
		Type:                  "frontend_engineer",
		Label:                 "前端开发",
		DefaultRole:           "frontend_engineer",
		RecommendedSkills:     []string{"frontend-implementation"},
		RecommendedMCPServers: []string{"browser"},
	})
	require.NoError(t, err)

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.NoError(t, err)
	require.NotNil(t, options.TeamConfig.TeamID)
	require.Equal(t, teamID, *options.TeamConfig.TeamID)
	require.Equal(t, tenantID, options.TeamConfig.TenantID)
	require.Equal(t, map[string]any{"mission": "stabilize ops"}, options.TeamConfig.Constitution)
	require.Equal(t, []string{"baseline-skill", "shared-skill"}, options.TeamConfig.Skills)
	require.Equal(t, []string{"baseline-mcp"}, options.TeamConfig.MCPServers)
	skillKeys := capabilityOptionKeys(options.CapabilityOptions.Skills)
	mcpKeys := capabilityOptionKeys(options.CapabilityOptions.MCPServers)
	require.Contains(t, skillKeys, "database-troubleshooting")
	require.Contains(t, skillKeys, "frontend-implementation")
	require.Contains(t, mcpKeys, "browser")
	require.Contains(t, mcpKeys, "postgres-readonly")
	require.NotContains(t, skillKeys, "baseline-skill")
	require.NotContains(t, mcpKeys, "baseline-mcp")
	// Template-recommended keys missing from the registry are surfaced as
	// unavailable rather than silently hidden.
	dbSkill := capabilityOptionByKey(t, options.CapabilityOptions.Skills, "database-troubleshooting")
	require.True(t, dbSkill.Recommended)
	require.False(t, dbSkill.Available)
}

func TestCreateOptionsIncludeRegistrySkillsAndMCPServers(t *testing.T) {
	svc, repo, tenantID, teamID := newCreateOptionsTestService(t, nil, nil)
	skillID := uuid.New()
	serverID := uuid.New()
	repo.registrySkills = []CapabilityRegistryOption{{
		ID:          skillID,
		Key:         "market-skill",
		Label:       "市场技能",
		Description: "来自技能市场",
		RiskLevel:   "normal",
	}}
	repo.registryMCPServers = []CapabilityRegistryOption{{
		ID:    serverID,
		Key:   "market-mcp",
		Label: "注册表 MCP",
	}}

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.NoError(t, err)
	marketSkill := capabilityOptionByKey(t, options.CapabilityOptions.Skills, "market-skill")
	require.True(t, marketSkill.Available)
	require.False(t, marketSkill.Recommended)
	require.NotNil(t, marketSkill.ID)
	require.Equal(t, skillID, *marketSkill.ID)
	require.Equal(t, "市场技能", marketSkill.Label)
	marketMCP := capabilityOptionByKey(t, options.CapabilityOptions.MCPServers, "market-mcp")
	require.True(t, marketMCP.Available)
	require.NotNil(t, marketMCP.ID)
	require.Equal(t, serverID, *marketMCP.ID)
}

func capabilityOptionKeys(items []CapabilityOptionItem) []string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	return keys
}

func capabilityOptionByKey(t *testing.T, items []CapabilityOptionItem, key string) CapabilityOptionItem {
	t.Helper()
	for _, item := range items {
		if item.Key == key {
			return item
		}
	}
	t.Fatalf("capability option %q not found in %#v", key, items)
	return CapabilityOptionItem{}
}

func TestGetCreateOptionsIncludesCustomAgentRegardlessOfTeamAllowlist(t *testing.T) {
	svc, repo, tenantID, teamID := newCreateOptionsTestService(t, map[string]any{
		"allowed_employee_types": []any{"custom_agent"},
		"allowed_provider_types": []any{"codex"},
	}, map[string]any{})
	// Seed a template the team's allow-list does NOT mention, to prove the
	// returned catalog is the tenant's full template set, not filtered down
	// to the allow-list.
	_, err := repo.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "评审员",
	})
	require.NoError(t, err)

	options, err := svc.GetCreateOptions(context.Background(), CreateOptionsRequest{
		TenantID: tenantID,
		TeamID:   &teamID,
	})

	require.NoError(t, err)
	require.True(t, employeeTypeOptionExists(options.EmployeeTypes, "custom_agent"))
	require.Greater(t, len(options.EmployeeTypes), 1)
}

func employeeTypeOptionExists(items []EmployeeTypeDefinition, employeeType string) bool {
	for _, item := range items {
		if item.Type == employeeType {
			return true
		}
	}
	return false
}

func TestEmployeeServiceGetOverviewAppliesDefaultsAndFilters(t *testing.T) {
	tenantID := uuid.New()
	teamID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := &overviewRepositoryStub{
		overview: &DigitalEmployeeOverview{
			Summary:    DigitalEmployeeOverviewSummary{TotalCount: 1, RunnableCount: 1},
			Items:      []DigitalEmployeeOverviewItem{},
			Filters:    DigitalEmployeeOverviewFilters{},
			Pagination: OverviewPagination{Limit: 50, Offset: 0, TotalCount: 1},
		},
	}
	service, err := NewService(repo)
	require.NoError(t, err)

	overview, err := service.GetOverview(context.Background(), GetDigitalEmployeeOverviewRequest{
		TenantID:        tenantID,
		Query:           "  需求  ",
		TeamID:          &teamID,
		Status:          DigitalEmployeeStatusActive,
		EmployeeType:    "requirements_analyst",
		ProviderType:    "codex",
		RuntimeNodeID:   &runtimeNodeID,
		RiskLevel:       "medium",
		ExecutionStatus: OverviewExecutionStatusMissing,
		RunStatus:       OverviewRunStatusNone,
	})

	require.NoError(t, err)
	require.Equal(t, int32(50), repo.req.Limit)
	require.Equal(t, int32(0), repo.req.Offset)
	require.Equal(t, "需求", repo.req.Query)
	require.Equal(t, teamID, *repo.req.TeamID)
	require.Equal(t, runtimeNodeID, *repo.req.RuntimeNodeID)
	require.Equal(t, int32(50), overview.Pagination.Limit)
}

func TestEmployeeServiceGetOverviewRejectsInvalidFilters(t *testing.T) {
	service, err := NewService(&overviewRepositoryStub{})
	require.NoError(t, err)
	_, err = service.GetOverview(context.Background(), GetDigitalEmployeeOverviewRequest{
		TenantID:        uuid.New(),
		Status:          DigitalEmployeeStatus("retired"),
		ExecutionStatus: OverviewExecutionStatusReady,
		RunStatus:       OverviewRunStatusNone,
	})
	require.ErrorIs(t, err, ErrInvalidInput)

	_, err = service.GetOverview(context.Background(), GetDigitalEmployeeOverviewRequest{
		TenantID:        uuid.New(),
		ExecutionStatus: OverviewExecutionStatus("lost"),
		RunStatus:       OverviewRunStatusNone,
	})
	require.ErrorIs(t, err, ErrInvalidInput)

	_, err = service.GetOverview(context.Background(), GetDigitalEmployeeOverviewRequest{
		TenantID:        uuid.New(),
		ExecutionStatus: OverviewExecutionStatusMissing,
		RunStatus:       OverviewRunStatus("paused"),
	})
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestServiceDeleteDigitalEmployeeBlocksActiveWorkAndRollsBack(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	employeeID := uuid.New()
	actorID := uuid.New()
	now := time.Now().UTC()
	repo.employees[employeeID] = DigitalEmployeeRecord{ID: employeeID, TenantID: tenantID, OwnerUserID: actorID, EmployeeType: "devops_engineer", ProviderType: "codex", Name: "阻断员工", Role: "devops", Status: DigitalEmployeeStatusReady, CreatedAt: now, UpdatedAt: now}
	repo.deleteBlockers = []DigitalEmployeeDeleteBlocker{{Type: DigitalEmployeeDeleteBlockerTypeRun, ID: uuid.New(), Status: "running", Title: "运行中的任务"}}

	err = svc.DeleteDigitalEmployee(context.Background(), DeleteDigitalEmployeeRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, ActorUserID: actorID})
	require.ErrorIs(t, err, ErrDigitalEmployeeDeleteBlocked)
	var blocked *DigitalEmployeeDeleteBlockedError
	require.ErrorAs(t, err, &blocked)
	require.Len(t, blocked.Blockers, 1)
	require.Nil(t, repo.employees[employeeID].DeletedAt)
	require.Equal(t, 0, repo.deleteCascadeCount)
	require.Len(t, repo.deleteAuditEvents, 0)
	require.Equal(t, 0, repo.transactionCommitCount)
}

func TestServiceDeleteDigitalEmployeeSoftDeletesCascadeAndAudits(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	teamID := uuid.New()
	employeeID := uuid.New()
	actorID := uuid.New()
	executionInstanceID := uuid.New()
	runtimeNodeID := uuid.New()
	now := time.Now().UTC()
	repo.employees[employeeID] = DigitalEmployeeRecord{ID: employeeID, TenantID: tenantID, TeamID: &teamID, OwnerUserID: actorID, EmployeeType: "devops_engineer", ProviderType: "codex", Name: "可删除员工", Role: "devops", Status: DigitalEmployeeStatusReady, CreatedAt: now, UpdatedAt: now}
	repo.deleteCascadeResult = DigitalEmployeeDeleteCascadeResult{ExecutionInstances: 1, EnvironmentVariables: 2, MCPBindingsV2: 1, SkillBindings: 1, ConfigRevisions: 1, WorkspaceFiles: 1, ProjectAffinities: 1, ExecutionInstanceID: &executionInstanceID, RuntimeNodeID: &runtimeNodeID, AgentHomeDir: "/srv/superteam/agents/emp", ProviderType: "codex", WorkspaceFileIDs: []uuid.UUID{uuid.New()}}

	err = svc.DeleteDigitalEmployee(context.Background(), DeleteDigitalEmployeeRequest{TenantID: tenantID, DigitalEmployeeID: employeeID, ActorUserID: actorID})
	require.NoError(t, err)
	require.NotNil(t, repo.employees[employeeID].DeletedAt)
	require.Equal(t, DigitalEmployeeStatusDisabled, repo.employees[employeeID].Status)
	require.Equal(t, 1, repo.deleteCascadeCount)
	require.Len(t, repo.deleteAuditEvents, 1)
	require.Equal(t, actorID, repo.deleteAuditEvents[0].ActorUserID)
	require.Equal(t, employeeID, repo.deleteAuditEvents[0].Employee.ID)
	require.Equal(t, int64(1), repo.deleteAuditEvents[0].CascadeResult.ExecutionInstances)
	require.Equal(t, 1, repo.transactionCommitCount)
}

func TestServiceDeleteDigitalEmployeeValidatesRequiredIDs(t *testing.T) {
	svc, err := NewService(newMemoryRepository())
	require.NoError(t, err)
	err = svc.DeleteDigitalEmployee(context.Background(), DeleteDigitalEmployeeRequest{})
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateDigitalEmployeeParamsAndDomainMappingKeepOwnerAndType(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	ownerUserID := uuid.New()

	record, err := repo.CreateDigitalEmployee(context.Background(), CreateDigitalEmployeeParams{
		TenantID:     tenantID,
		OwnerUserID:  ownerUserID,
		EmployeeType: "database_admin",
		ProviderType: "codex",
		Name:         "Database maintainer",
		Role:         "database_admin",
		Status:       DigitalEmployeeStatusDraft,
	})
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}

	if record.OwnerUserID != ownerUserID {
		t.Fatalf("expected owner_user_id %s, got %s", ownerUserID, record.OwnerUserID)
	}
	if record.EmployeeType != "database_admin" {
		t.Fatalf("expected employee_type database_admin, got %q", record.EmployeeType)
	}
	if record.ProviderType != "codex" {
		t.Fatalf("expected provider_type codex, got %q", record.ProviderType)
	}
	employee := employeeFromRecord(record)
	if employee.OwnerUserID != ownerUserID {
		t.Fatalf("expected domain owner_user_id %s, got %s", ownerUserID, employee.OwnerUserID)
	}
	if employee.EmployeeType != "database_admin" {
		t.Fatalf("expected domain employee_type database_admin, got %q", employee.EmployeeType)
	}
	if employee.ProviderType != "codex" {
		t.Fatalf("expected domain provider_type codex, got %q", employee.ProviderType)
	}
}

func TestCreateDigitalEmployeeDoesNotRequireRuntimeBinding(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryRepository()
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	repo.teams[teamID] = tenantID
	teamConfigID := uuid.New()
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:       teamConfigID,
		TenantID: tenantID,
		TeamID:   teamID,
		CapabilityPolicy: map[string]any{
			"allowed_employee_types": []any{"backend_engineer"},
			"allowed_provider_types": []any{"codex"},
		},
		RuntimeScopePolicy: map[string]any{
			"provider_types": []any{"codex"},
		},
	}
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	if _, err := repo.CreateEmployeeTemplate(ctx, CreateEmployeeTemplateParams{
		TenantID:    tenantID,
		Type:        "backend_engineer",
		Label:       "后端开发",
		DefaultRole: "backend_engineer",
	}); err != nil {
		t.Fatalf("seed backend_engineer template: %v", err)
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	employee, err := service.CreateDigitalEmployee(ctx, CreateDigitalEmployeeRequest{
		TenantID:      tenantID,
		TeamID:        &teamID,
		OwnerUserID:   ownerUserID,
		EmployeeType:  "backend_engineer",
		Name:          "需求分析员",
		AvatarAssetID: "engineer-m-01",
		ProviderType:  "codex",
		Role:          "负责需求澄清",
	})

	if err != nil {
		t.Fatalf("create digital employee without runtime binding: %v", err)
	}
	if employee.Status != DigitalEmployeeStatusReady {
		t.Fatalf("expected ready status, got %q", employee.Status)
	}
	if employee.ProviderType != "codex" {
		t.Fatalf("expected provider_type codex, got %q", employee.ProviderType)
	}
	if len(repo.instances) != 0 {
		t.Fatalf("expected no execution instances, got %#v", repo.instances)
	}
	if len(repo.commandReceipts) != 0 {
		t.Fatalf("expected no runtime command receipts, got %#v", repo.commandReceipts)
	}
}

func TestCreateDigitalEmployeeCreatesOwnerTypeConfigEffectiveConfigWithoutRuntimeBinding(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	req.BudgetPolicy = map[string]any{"daily_token_limit": 120000}
	dispatchDuringTransaction := false
	dispatchAfterCommit := false
	dispatcher.onDispatch = func(_ string, _ cpruntime.RuntimeCommand) {
		dispatchDuringTransaction = repo.inTransaction
		dispatchAfterCommit = repo.transactionCommitCount == 1
	}

	created, err := svc.CreateDigitalEmployee(context.Background(), req)
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}

	if created.TenantID != req.TenantID {
		t.Fatalf("expected tenant id %s, got %s", req.TenantID, created.TenantID)
	}
	if created.TeamID == nil || *created.TeamID != *req.TeamID {
		t.Fatalf("expected team id %s, got %#v", *req.TeamID, created.TeamID)
	}
	if created.OwnerUserID != req.OwnerUserID {
		t.Fatalf("expected owner_user_id %s, got %s", req.OwnerUserID, created.OwnerUserID)
	}
	if created.EmployeeType != "database_admin" {
		t.Fatalf("expected employee_type database_admin, got %q", created.EmployeeType)
	}
	if created.ProviderType != "codex" {
		t.Fatalf("expected provider_type codex, got %q", created.ProviderType)
	}
	if created.Name != "Main database admin" {
		t.Fatalf("expected trimmed name, got %q", created.Name)
	}
	if created.Role != "database_admin" {
		t.Fatalf("expected default database admin role, got %q", created.Role)
	}
	if created.Metadata["avatar_asset_id"] != "engineer-m-01" {
		t.Fatalf("expected avatar asset id metadata, got %#v", created.Metadata)
	}
	avatar, ok := created.Metadata["avatar"].(map[string]any)
	if !ok || avatar["id"] != "engineer-m-01" || avatar["thumbnail_url"] == "" {
		t.Fatalf("expected avatar metadata snapshot, got %#v", created.Metadata)
	}
	if created.Status != DigitalEmployeeStatusReady {
		t.Fatalf("expected ready status after identity creation, got %q", created.Status)
	}
	if repo.createdEmployeeCount != 1 {
		t.Fatalf("expected one employee to be created, got %d", repo.createdEmployeeCount)
	}
	if repo.transactionCount != 1 || repo.transactionCommitCount != 1 || repo.transactionRollbackCount != 0 {
		t.Fatalf("expected exactly one committed transaction, got tx=%d commit=%d rollback=%d", repo.transactionCount, repo.transactionCommitCount, repo.transactionRollbackCount)
	}
	if dispatchDuringTransaction || dispatchAfterCommit {
		t.Fatalf("expected no runtime dispatch, during_tx=%v after_commit=%v", dispatchDuringTransaction, dispatchAfterCommit)
	}

	if repo.createdConfigRevision.Status != ConfigRevisionStatusActive {
		t.Fatalf("expected initial config revision active, got %q", repo.createdConfigRevision.Status)
	}
	if repo.createdConfigRevision.ApprovedBy == nil || *repo.createdConfigRevision.ApprovedBy != req.OwnerUserID || repo.createdConfigRevision.ApprovedAt == nil {
		t.Fatalf("expected config revision approved by owner, got approved_by=%#v approved_at=%#v", repo.createdConfigRevision.ApprovedBy, repo.createdConfigRevision.ApprovedAt)
	}
	if repo.createdConfigRevision.PersonaMemoryMarkdown != "# postgres operator" {
		t.Fatalf("expected persona memory to be persisted, got %#v", repo.createdConfigRevision.PersonaMemoryMarkdown)
	}
	if !stringListContains(repo.createdConfigRevision.CapabilityBindings["external_capabilities"], "change-ticket") {
		t.Fatalf("expected request capability bindings to be merged, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
	if repo.createdConfigRevision.BudgetPolicy["daily_token_limit"] != float64(120000) {
		t.Fatalf("expected request budget policy to be persisted, got %#v", repo.createdConfigRevision.BudgetPolicy)
	}

	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected no runtime command, got %#v", dispatcher.commands)
	}
	if len(repo.instances) != 0 {
		t.Fatalf("expected no execution instances, got %#v", repo.instances)
	}
	if len(repo.commandReceipts) != 0 {
		t.Fatalf("expected no command receipt, got %#v", repo.commandReceipts)
	}
}

func TestCreateDigitalEmployeeRejectsTeamOverCapacityBeforeTransaction(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	repo.digitalEmployeeOverview = &DigitalEmployeeOverview{
		Items:      []DigitalEmployeeOverviewItem{},
		Filters:    DigitalEmployeeOverviewFilters{},
		Pagination: OverviewPagination{Limit: 1, Offset: 0, TotalCount: 10},
	}

	_, err := svc.CreateDigitalEmployee(context.Background(), req)

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if err == nil || !strings.Contains(err.Error(), "digital employee capacity") {
		t.Fatalf("expected digital employee capacity error, got %v", err)
	}
	if repo.lastOverviewRequest.TeamID == nil || *repo.lastOverviewRequest.TeamID != *req.TeamID {
		t.Fatalf("expected capacity check to query target team overview, got %#v", repo.lastOverviewRequest)
	}
	if repo.createdEmployeeCount != 0 || repo.transactionCount != 0 {
		t.Fatalf("expected rejection before create/transaction, created=%d tx=%d", repo.createdEmployeeCount, repo.transactionCount)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected no runtime dispatch, got %#v", dispatcher.commands)
	}
}

func TestCreateDigitalEmployeeSupportsTeamLessCreation(t *testing.T) {
	svc, _, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	req.TeamID = nil

	created, err := svc.CreateDigitalEmployee(context.Background(), req)
	if err != nil {
		t.Fatalf("create team-less digital employee without runtime binding: %v", err)
	}

	if created.TeamID != nil {
		t.Fatalf("expected created employee team_id nil, got %#v", created.TeamID)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected no runtime command, got %#v", dispatcher.commands)
	}
}

func TestCreateDigitalEmployeePersistsInitialEnvironmentVariablesInTransaction(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	svc.SetEnvironmentCodec(testCreateFlowEnvironmentCodec(t))
	req.EnvironmentVariables = []InitialEnvironmentVariable{{
		Name:      " GH_TOKEN ",
		Value:     "ghp_secret_value",
		Sensitive: true,
	}}

	created, err := svc.CreateDigitalEmployee(context.Background(), req)
	if err != nil {
		t.Fatalf("create digital employee: %v", err)
	}

	if repo.transactionCount != 1 || repo.transactionCommitCount != 1 || repo.transactionRollbackCount != 0 {
		t.Fatalf("expected one committed transaction, got tx=%d commit=%d rollback=%d", repo.transactionCount, repo.transactionCommitCount, repo.transactionRollbackCount)
	}
	if len(repo.envVars) != 1 {
		t.Fatalf("expected one env var, got %#v", repo.envVars)
	}
	record := repo.envVars["GH_TOKEN"]
	if record.TenantID != req.TenantID || record.TeamID == nil || *record.TeamID != *req.TeamID || record.DigitalEmployeeID != created.ID || record.Name != "GH_TOKEN" {
		t.Fatalf("unexpected env var scope: %#v", record)
	}
	if record.EncryptedValue == "" || strings.Contains(record.EncryptedValue, "ghp_secret_value") {
		t.Fatalf("stored env value leaked plaintext: %#v", record)
	}
	if record.EncryptionKeyID != "v1" || record.ValueFingerprint == "" || !record.Sensitive {
		t.Fatalf("expected encrypted env metadata, got %#v", record)
	}
}

func TestCreateDigitalEmployeeRollsBackInitialEnvironmentVariablesWhenNameInvalid(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	svc.SetEnvironmentCodec(testCreateFlowEnvironmentCodec(t))
	req.EnvironmentVariables = []InitialEnvironmentVariable{{
		Name:      "1BAD",
		Value:     "ghp_secret_value",
		Sensitive: true,
	}}

	_, err := svc.CreateDigitalEmployee(context.Background(), req)

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if repo.transactionCount != 1 || repo.transactionCommitCount != 0 || repo.transactionRollbackCount != 1 {
		t.Fatalf("expected one rolled-back transaction, got tx=%d commit=%d rollback=%d", repo.transactionCount, repo.transactionCommitCount, repo.transactionRollbackCount)
	}
	if len(repo.employees) != 0 || len(repo.envVars) != 0 || len(repo.instances) != 0 || len(repo.commandReceipts) != 0 {
		t.Fatalf("expected local facts rollback, employees=%#v env=%#v instances=%#v receipts=%#v", repo.employees, repo.envVars, repo.instances, repo.commandReceipts)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected no runtime dispatch after invalid env var, got %#v", dispatcher.commands)
	}
}

func TestCreateDigitalEmployeeRejectsUnknownEmployeeType(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	teamID := uuid.New()

	_, err = svc.CreateDigitalEmployee(context.Background(), CreateDigitalEmployeeRequest{
		TenantID:     uuid.New(),
		TeamID:       &teamID,
		OwnerUserID:  uuid.New(),
		EmployeeType: "project_coordinator",
		Name:         "Coordinator",
		ProviderType: "codex",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for unknown employee type, got %v", err)
	}
	if repo.createdEmployeeCount != 0 || repo.transactionCount != 0 {
		t.Fatalf("expected type rejection before creation, employees=%d transactions=%d", repo.createdEmployeeCount, repo.transactionCount)
	}
}

func TestCreateDigitalEmployeeRejectsUnknownAvatarAsset(t *testing.T) {
	svc, _, _, req := newCreateDigitalEmployeeReadyFixture(t)
	req.AvatarAssetID = "missing-avatar"

	_, err := svc.CreateDigitalEmployee(context.Background(), req)
	if err == nil {
		t.Fatalf("expected unknown avatar asset to fail")
	}
	if !strings.Contains(err.Error(), "unknown avatar_asset_id") {
		t.Fatalf("expected unknown avatar asset error, got %v", err)
	}
}

func TestCreateDigitalEmployeeAllowsCapabilityOutsideFormerTeamPolicyBeforeProvisioning(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	teamConfigID := repo.currentTeamConfigByTeam[*req.TeamID]
	teamConfig := repo.teamConfigs[teamConfigID]
	teamConfig.CapabilityPolicy = map[string]any{
		"allowed_skills":         []any{"incident-diagnosis"},
		"allowed_provider_types": []any{"codex"},
		"allowed_employee_types": []any{"database_admin"},
	}
	repo.teamConfigs[teamConfigID] = teamConfig

	created, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.NoError(t, err)
	require.NotNil(t, created)
	require.Empty(t, dispatcher.commands)
	require.Len(t, repo.employees, 1)
}

func TestCreateDigitalEmployeeAllowsProviderOutsideFormerTeamPolicyBeforeCreatingFacts(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	req.ProviderType = "opencode"
	teamConfigID := repo.currentTeamConfigByTeam[*req.TeamID]
	teamConfig := repo.teamConfigs[teamConfigID]
	teamConfig.CapabilityPolicy = map[string]any{
		"allowed_employee_types": []any{"database_admin"},
		"allowed_provider_types": []any{"codex"},
		"allowed_skills":         []any{"database-troubleshooting", "sql-review", "backup-restore", "performance-tuning"},
		"allowed_mcp_servers":    []any{"postgres-readonly", "mysql-readonly"},
	}
	teamConfig.RuntimeScopePolicy = map[string]any{
		"allowed_provider_types": []any{"codex"},
	}
	repo.teamConfigs[teamConfigID] = teamConfig

	created, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, "opencode", created.ProviderType)
	require.Empty(t, dispatcher.commands)
	require.Len(t, repo.employees, 1)
	require.Equal(t, 1, repo.transactionCount)
}

func TestCreateDigitalEmployeeProviderTypeMustBeSupportedEvenWithoutTeamAllowlist(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		wantError    bool
	}{
		{name: "claude code allowed", providerType: "claude-code"},
		{name: "opencode allowed", providerType: "opencode"},
		{name: "codex allowed", providerType: "codex"},
		{name: "unknown provider rejected", providerType: "foo", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
			req.ProviderType = tt.providerType
			teamConfigID := repo.currentTeamConfigByTeam[*req.TeamID]
			teamConfig := repo.teamConfigs[teamConfigID]
			delete(teamConfig.CapabilityPolicy, "allowed_provider_types")
			teamConfig.RuntimeScopePolicy = map[string]any{}
			repo.teamConfigs[teamConfigID] = teamConfig

			_, err := svc.CreateDigitalEmployee(context.Background(), req)

			if tt.wantError {
				require.ErrorIs(t, err, ErrInvalidInput)
				require.Contains(t, err.Error(), "provider_type")
				require.Empty(t, dispatcher.commands)
				require.Empty(t, repo.employees)
				require.Empty(t, repo.commandReceipts)
				require.Zero(t, repo.transactionCount)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCreateDigitalEmployeeNormalizesProviderTypeAliases(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	req.ProviderType = " CLAUDE_CODE "
	teamConfigID := repo.currentTeamConfigByTeam[*req.TeamID]
	teamConfig := repo.teamConfigs[teamConfigID]
	teamConfig.CapabilityPolicy["allowed_provider_types"] = []any{"claude-code"}
	teamConfig.RuntimeScopePolicy = map[string]any{"allowed_provider_types": []any{"claude-code"}}
	repo.teamConfigs[teamConfigID] = teamConfig

	created, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, "claude-code", created.ProviderType)
	require.Equal(t, "claude-code", repo.employees[created.ID].ProviderType)
}

func TestCreateDigitalEmployeeRejectsBlankProviderType(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)
	req.ProviderType = " "

	_, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Contains(t, err.Error(), "provider_type is required")
	require.Empty(t, dispatcher.commands)
	require.Empty(t, repo.employees)
	require.Empty(t, repo.commandReceipts)
}

func TestCreateDigitalEmployeeKeepsPlatformTypeDefaultsWithoutTeamPolicyClipping(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	teamConfigID := repo.currentTeamConfigByTeam[*req.TeamID]
	teamConfig := repo.teamConfigs[teamConfigID]
	teamConfig.CapabilityPolicy = map[string]any{
		"skill_bindings": []any{"security-capability-1"},
	}
	teamConfig.ContextPolicy = map[string]any{
		"allowed_sources": []any{"team-docs", "runtime-logs"},
	}
	teamConfig.ApprovalPolicy = map[string]any{
		"high_risk": "required",
	}
	teamConfig.RuntimeScopePolicy = map[string]any{
		"provider_types": []any{"codex"},
	}
	repo.teamConfigs[teamConfigID] = teamConfig
	req.CapabilityBindings = map[string]any{}

	created, err := svc.CreateDigitalEmployee(context.Background(), req)

	if err != nil {
		t.Fatalf("create digital employee should not fail on filtered type defaults: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatalf("expected created employee id")
	}
	// Template skill/MCP defaults are no longer silently merged into the
	// config revision: skills/mcp_servers live in binding tables only, and
	// template defaults surface as create-options recommendations.
	if _, ok := repo.createdConfigRevision.CapabilityBindings["skills"]; ok {
		t.Fatalf("expected skills key stripped from config revision, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
	if _, ok := repo.createdConfigRevision.CapabilityBindings["mcp_servers"]; ok {
		t.Fatalf("expected mcp_servers key stripped from config revision, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
}

func TestCreateDigitalEmployeeBindsSelectedCapabilities(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	skillID := uuid.New()
	serverID := uuid.New()
	repo.registrySkills = []CapabilityRegistryOption{{ID: skillID, Key: "market-skill", Label: "市场技能"}}
	repo.registryMCPServers = []CapabilityRegistryOption{{ID: serverID, Key: "market-mcp", Label: "注册表 MCP"}}
	req.Skills = []string{"market-skill", " market-skill "}
	req.MCPServers = []string{"market-mcp"}

	created, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{skillID}, repo.boundSkillIDs[created.ID])
	require.Equal(t, []uuid.UUID{serverID}, repo.boundMCPServerIDs[created.ID])
	if _, ok := repo.createdConfigRevision.CapabilityBindings["skills"]; ok {
		t.Fatalf("expected skills key stripped from stored revision, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
}

func TestCreateDigitalEmployeeRejectsUnknownCapabilityKeys(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	req.Skills = []string{"ghost-skill"}

	_, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.ErrorIs(t, err, ErrInvalidInput)
	require.Contains(t, err.Error(), `unknown skill slug "ghost-skill"`)
	require.Empty(t, repo.boundSkillIDs)
	require.Equal(t, 1, repo.transactionRollbackCount)
}

func TestCreateDigitalEmployeeSkipsTeamInheritedCapabilityKeys(t *testing.T) {
	svc, repo, _, req := newCreateDigitalEmployeeReadyFixture(t)
	repo.teamBaselines[*req.TeamID] = TeamBaseline{Skills: []string{"inherited-skill"}}
	req.Skills = []string{"inherited-skill"}

	created, err := svc.CreateDigitalEmployee(context.Background(), req)

	require.NoError(t, err)
	require.Empty(t, repo.boundSkillIDs[created.ID])
}

func TestCreateDigitalEmployeeDoesNotWaitForProvisioningTimeout(t *testing.T) {
	svc, repo, dispatcher, req := newCreateDigitalEmployeeReadyFixture(t)

	_, err := svc.CreateDigitalEmployee(context.Background(), req)

	if err != nil {
		t.Fatalf("create digital employee should not wait for runtime provisioning: %v", err)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("expected no runtime command, got %#v", dispatcher.commands)
	}
	if len(repo.abortReasons) != 0 {
		t.Fatalf("expected no provisioning abort, got %#v", repo.abortReasons)
	}
	visible, err := repo.ListDigitalEmployees(context.Background(), ListDigitalEmployeesParams{TenantID: req.TenantID})
	if err != nil {
		t.Fatalf("list employees: %v", err)
	}
	if len(visible) != 1 || visible[0].Status != DigitalEmployeeStatusReady {
		t.Fatalf("expected visible ready employee after identity creation, got %#v", visible)
	}
}

func TestBindExecutionInstanceRejectsEmployeeRuntimeBinding(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	runtimeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:           employeeID,
		TenantID:     tenantID,
		Status:       DigitalEmployeeStatusReady,
		ProviderType: "codex",
	}

	_, err = svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     runtimeID,
		ProviderType:      "codex",
		AgentHomeDir:      "/srv/superteam/employees/finance",
		SessionPolicy:     map[string]any{"max_turns": float64(5)},
	})
	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "digital employees are not runtime-bound") {
		t.Fatalf("expected runtime binding rejection, got %v", err)
	}
	if len(repo.instances) != 0 {
		t.Fatalf("expected no execution instance writes, got %#v", repo.instances)
	}
}

func TestBindExecutionInstanceRejectsProviderChangeThroughLegacyBinding(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:           employeeID,
		TenantID:     tenantID,
		Status:       DigitalEmployeeStatusReady,
		ProviderType: "codex",
	}

	_, err = svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RuntimeNodeID:     runtimeNodeID,
		ProviderType:      "opencode",
		AgentHomeDir:      "/tmp/opencode",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if !strings.Contains(err.Error(), "digital employees are not runtime-bound") {
		t.Fatalf("expected runtime binding rejection, got %v", err)
	}
	if repo.employees[employeeID].ProviderType != "codex" {
		t.Fatalf("expected employee provider to remain codex, got %q", repo.employees[employeeID].ProviderType)
	}
	if len(repo.instances) != 0 {
		t.Fatalf("expected no execution instance writes, got %#v", repo.instances)
	}
}

func TestServiceValidation(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	employeeID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	if _, err := repo.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:    tenantID,
		Type:        "backend_engineer",
		Label:       "后端开发",
		DefaultRole: "backend_engineer",
	}); err != nil {
		t.Fatalf("seed backend_engineer template: %v", err)
	}
	validCreateReq := func() CreateDigitalEmployeeRequest {
		return CreateDigitalEmployeeRequest{
			TenantID:      tenantID,
			TeamID:        &teamID,
			OwnerUserID:   ownerUserID,
			EmployeeType:  "backend_engineer",
			Name:          "employee",
			AvatarAssetID: "engineer-m-01",
			ProviderType:  "codex",
		}
	}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "create requires tenant",
			run: func() error {
				req := validCreateReq()
				req.TenantID = uuid.Nil
				_, err := svc.CreateDigitalEmployee(context.Background(), req)
				return err
			},
		},
		{
			name: "create requires owner",
			run: func() error {
				req := validCreateReq()
				req.OwnerUserID = uuid.Nil
				_, err := svc.CreateDigitalEmployee(context.Background(), req)
				return err
			},
		},
		{
			name: "create requires employee type",
			run: func() error {
				req := validCreateReq()
				req.EmployeeType = " "
				_, err := svc.CreateDigitalEmployee(context.Background(), req)
				return err
			},
		},
		{
			name: "create requires name",
			run: func() error {
				req := validCreateReq()
				req.Name = " "
				_, err := svc.CreateDigitalEmployee(context.Background(), req)
				return err
			},
		},
		{
			name: "create requires provider",
			run: func() error {
				req := validCreateReq()
				req.ProviderType = " "
				_, err := svc.CreateDigitalEmployee(context.Background(), req)
				return err
			},
		},
		{
			name: "bind requires provider",
			run: func() error {
				_, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
					TenantID:          tenantID,
					DigitalEmployeeID: employeeID,
					RuntimeNodeID:     runtimeNodeID,
					AgentHomeDir:      "/tmp/agent",
				})
				return err
			},
		},
		{
			name: "bind requires agent home dir",
			run: func() error {
				_, err := svc.BindExecutionInstance(context.Background(), BindExecutionInstanceRequest{
					TenantID:          tenantID,
					DigitalEmployeeID: employeeID,
					RuntimeNodeID:     runtimeNodeID,
					ProviderType:      "codex",
					AgentHomeDir:      " ",
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestCreateConfigRevisionDefaultsDraftAndRevisionNumber(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	teamID := uuid.New()
	spoofedApproverID := uuid.New()
	repo.nextConfigRevisionNumber = 3
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Finance reviewer",
		Role:     "finance_reviewer",
		Status:   DigitalEmployeeStatusDraft,
	}

	revision, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		BudgetPolicy:      map[string]any{"daily_token_limit": 25000},
		ApprovedBy:        &spoofedApproverID,
	})
	if err != nil {
		t.Fatalf("create config revision: %v", err)
	}

	if revision.RevisionNumber != 3 {
		t.Fatalf("expected revision number 3, got %d", revision.RevisionNumber)
	}
	if revision.Status != ConfigRevisionStatusDraft {
		t.Fatalf("expected draft status, got %q", revision.Status)
	}
	if revision.ApprovedAt != nil {
		t.Fatalf("expected draft revision approved_at to be nil, got %v", revision.ApprovedAt)
	}
	if revision.ApprovedBy != nil {
		t.Fatalf("expected draft revision approved_by to be nil, got %#v", revision.ApprovedBy)
	}
	if repo.createdConfigRevision.Status != ConfigRevisionStatusDraft {
		t.Fatalf("expected repository draft status, got %q", repo.createdConfigRevision.Status)
	}
	if repo.createdConfigRevision.ApprovedBy != nil || repo.createdConfigRevision.ApprovedAt != nil {
		t.Fatalf("expected repository draft approval metadata to be cleared, got %#v/%#v", repo.createdConfigRevision.ApprovedBy, repo.createdConfigRevision.ApprovedAt)
	}
	if repo.createdConfigRevision.BudgetPolicy["daily_token_limit"] != float64(25000) {
		t.Fatalf("expected repository budget policy from request, got %#v", repo.createdConfigRevision.BudgetPolicy)
	}
	if revision.BudgetPolicy["daily_token_limit"] != float64(25000) {
		t.Fatalf("expected response budget policy from repository record, got %#v", revision.BudgetPolicy)
	}
}

func TestNormalizeBudgetPolicyHandlesEmptyAndRemoval(t *testing.T) {
	t.Run("nil input returns empty policy", func(t *testing.T) {
		policy, err := normalizeBudgetPolicy(nil)
		if err != nil {
			t.Fatalf("normalize budget policy: %v", err)
		}
		if policy == nil || len(policy) != 0 {
			t.Fatalf("expected empty policy, got %#v", policy)
		}
	})

	t.Run("missing daily token limit preserves other keys", func(t *testing.T) {
		input := map[string]any{"mode": "capped"}
		policy, err := normalizeBudgetPolicy(input)
		if err != nil {
			t.Fatalf("normalize budget policy: %v", err)
		}
		if policy["mode"] != "capped" {
			t.Fatalf("expected other policy keys to be preserved, got %#v", policy)
		}
		if _, ok := policy["daily_token_limit"]; ok {
			t.Fatalf("expected missing daily_token_limit to stay absent, got %#v", policy)
		}
	})

	tests := []struct {
		name  string
		value any
	}{
		{name: "nil removes key", value: nil},
		{name: "empty string removes key", value: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]any{
				"daily_token_limit": tt.value,
				"mode":              "capped",
			}
			policy, err := normalizeBudgetPolicy(input)
			if err != nil {
				t.Fatalf("normalize budget policy: %v", err)
			}
			if _, ok := policy["daily_token_limit"]; ok {
				t.Fatalf("expected daily_token_limit to be removed, got %#v", policy)
			}
			if policy["mode"] != "capped" {
				t.Fatalf("expected other policy keys to be preserved, got %#v", policy)
			}
		})
	}
}

func TestNormalizeBudgetPolicyNormalizesNumericLimits(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  float64
	}{
		{name: "int", value: int(12000), want: float64(12000)},
		{name: "int32", value: int32(12000), want: float64(12000)},
		{name: "int64", value: int64(12000), want: float64(12000)},
		{name: "float64 integer", value: float64(12000), want: float64(12000)},
		{name: "json number integer", value: json.Number("12000"), want: float64(12000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := normalizeBudgetPolicy(map[string]any{"daily_token_limit": tt.value})
			if err != nil {
				t.Fatalf("normalize budget policy: %v", err)
			}
			if policy["daily_token_limit"] != tt.want {
				t.Fatalf("expected daily_token_limit %v, got %#v", tt.want, policy["daily_token_limit"])
			}
			if _, ok := policy["daily_token_limit"].(float64); !ok {
				t.Fatalf("expected daily_token_limit to normalize to float64, got %T", policy["daily_token_limit"])
			}
		})
	}
}

func TestNormalizeBudgetPolicyRejectsInvalidLimits(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "fractional float64", value: float64(12.5)},
		{name: "zero", value: float64(0)},
		{name: "negative", value: int64(-1)},
		{name: "non-number string", value: "12000"},
		{name: "json number fractional", value: json.Number("12.5")},
		{name: "json number invalid", value: json.Number("not-a-number")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeBudgetPolicy(map[string]any{"daily_token_limit": tt.value})
			if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "budget_policy.daily_token_limit") {
				t.Fatalf("expected budget policy validation error, got %v", err)
			}
		})
	}
}

func TestNormalizeBudgetPolicyDoesNotMutateCallerMap(t *testing.T) {
	t.Run("normalization does not replace caller value", func(t *testing.T) {
		input := map[string]any{
			"daily_token_limit": int64(12000),
			"mode":              "capped",
		}
		policy, err := normalizeBudgetPolicy(input)
		if err != nil {
			t.Fatalf("normalize budget policy: %v", err)
		}
		if policy["daily_token_limit"] != float64(12000) {
			t.Fatalf("expected normalized policy, got %#v", policy)
		}
		if input["daily_token_limit"] != int64(12000) {
			t.Fatalf("expected caller daily_token_limit to remain int64, got %#v", input["daily_token_limit"])
		}
	})

	t.Run("removal does not delete caller key", func(t *testing.T) {
		input := map[string]any{
			"daily_token_limit": "",
			"mode":              "capped",
		}
		policy, err := normalizeBudgetPolicy(input)
		if err != nil {
			t.Fatalf("normalize budget policy: %v", err)
		}
		if _, ok := policy["daily_token_limit"]; ok {
			t.Fatalf("expected normalized policy to remove daily_token_limit, got %#v", policy)
		}
		if input["daily_token_limit"] != "" {
			t.Fatalf("expected caller daily_token_limit to remain empty string, got %#v", input["daily_token_limit"])
		}
	})
}

func TestCreateConfigRevisionStoresFinalFields(t *testing.T) {
	svc, repo := newEmployeeServiceForTest(t)
	tenantID := uuid.New()
	employeeID := uuid.New()
	seedConfigRevisionEmployee(repo, tenantID, employeeID)
	persona := "# 人格画像\n证据优先"

	revision, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:              tenantID,
		DigitalEmployeeID:     employeeID,
		PersonaMemoryMarkdown: &persona,
		CapabilityBindings:    map[string]any{"external_capabilities": []any{"feishu-connector"}},
		BudgetPolicy:          map[string]any{"daily_token_limit": float64(12000)},
		Status:                ConfigRevisionStatusDraft,
	})

	if err != nil {
		t.Fatalf("create config revision: %v", err)
	}
	if revision.PersonaMemoryMarkdown != persona {
		t.Fatalf("expected persona memory on revision, got %#v", revision.PersonaMemoryMarkdown)
	}
	if !stringListContains(revision.CapabilityBindings["external_capabilities"], "feishu-connector") {
		t.Fatalf("expected capability bindings on revision, got %#v", revision.CapabilityBindings)
	}
	if revision.BudgetPolicy["daily_token_limit"] != float64(12000) {
		t.Fatalf("expected budget policy on revision, got %#v", revision.BudgetPolicy)
	}
	if repo.createdConfigRevision.PersonaMemoryMarkdown != persona {
		t.Fatalf("expected persona memory persisted, got %#v", repo.createdConfigRevision.PersonaMemoryMarkdown)
	}
	if !stringListContains(repo.createdConfigRevision.CapabilityBindings["external_capabilities"], "feishu-connector") {
		t.Fatalf("expected capability bindings persisted, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
	if repo.createdConfigRevision.BudgetPolicy["daily_token_limit"] != float64(12000) {
		t.Fatalf("expected budget policy persisted, got %#v", repo.createdConfigRevision.BudgetPolicy)
	}
}

func TestCreateConfigRevisionRejectsLegacySkillAndMCPKeys(t *testing.T) {
	svc, repo := newEmployeeServiceForTest(t)
	tenantID := uuid.New()
	employeeID := uuid.New()
	seedConfigRevisionEmployee(repo, tenantID, employeeID)

	for _, bindings := range []map[string]any{
		{"skills": []any{"incident-diagnosis"}},
		{"mcp_servers": []any{"postgres-readonly"}},
	} {
		_, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
			TenantID:           tenantID,
			DigitalEmployeeID:  employeeID,
			CapabilityBindings: bindings,
			Status:             ConfigRevisionStatusDraft,
		})
		require.ErrorIs(t, err, ErrInvalidInput)
		require.Contains(t, err.Error(), "no longer supported")
	}

	// Empty arrays are stripped residue, not an error.
	_, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:           tenantID,
		DigitalEmployeeID:  employeeID,
		CapabilityBindings: map[string]any{"skills": []any{}, "mcp_servers": []any{}},
		Status:             ConfigRevisionStatusDraft,
	})
	require.NoError(t, err)
	if _, ok := repo.createdConfigRevision.CapabilityBindings["skills"]; ok {
		t.Fatalf("expected skills key stripped, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
}

func TestCreateConfigRevisionPreservesOmittedMapFieldsFromLatestRevision(t *testing.T) {
	svc, repo := newEmployeeServiceForTest(t)
	tenantID := uuid.New()
	employeeID := uuid.New()
	seedConfigRevisionEmployee(repo, tenantID, employeeID)
	latestID := uuid.New()
	repo.employeeConfigs[latestID] = EmployeeConfigInput{
		ID:                    latestID,
		TenantID:              tenantID,
		DigitalEmployeeID:     employeeID,
		RevisionNumber:        7,
		PersonaMemoryMarkdown: "# security reviewer",
		CapabilityBindings:    map[string]any{"external_capabilities": []any{"release-review"}, "skills": []any{"legacy-skill"}},
		BudgetPolicy:          map[string]any{"daily_token_limit": float64(12000), "mode": "capped"},
	}

	revision, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		BudgetPolicy:      map[string]any{"daily_token_limit": int64(24000), "mode": "capped"},
		Status:            ConfigRevisionStatusDraft,
	})
	if err != nil {
		t.Fatalf("create config revision: %v", err)
	}

	if revision.BudgetPolicy["daily_token_limit"] != float64(24000) {
		t.Fatalf("expected budget change to be applied, got %#v", revision.BudgetPolicy)
	}
	if repo.createdConfigRevision.PersonaMemoryMarkdown != "# security reviewer" {
		t.Fatalf("expected persona_memory_markdown to be preserved, got %#v", repo.createdConfigRevision.PersonaMemoryMarkdown)
	}
	if !stringListContains(repo.createdConfigRevision.CapabilityBindings["external_capabilities"], "release-review") {
		t.Fatalf("expected capability_bindings to be preserved, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
	// Inherited legacy skills residue from pre-unification revisions is
	// silently stripped, not rejected.
	if _, ok := repo.createdConfigRevision.CapabilityBindings["skills"]; ok {
		t.Fatalf("expected inherited skills key stripped, got %#v", repo.createdConfigRevision.CapabilityBindings)
	}
}

func TestCreateConfigRevisionRejectsInvalidBudgetPolicy(t *testing.T) {
	svc, repo := newEmployeeServiceForTest(t)
	tenantID := uuid.New()
	employeeID := uuid.New()
	seedConfigRevisionEmployee(repo, tenantID, employeeID)

	_, err := svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		BudgetPolicy:      map[string]any{"daily_token_limit": float64(0)},
		Status:            ConfigRevisionStatusDraft,
	})

	if !errors.Is(err, ErrInvalidInput) || !strings.Contains(err.Error(), "budget_policy.daily_token_limit") {
		t.Fatalf("expected budget policy validation error, got %v", err)
	}
}

func TestCreateConfigRevisionRequiresExistingEmployee(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	employeeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: uuid.New(),
		Name:     "Wrong tenant employee",
		Role:     "reviewer",
		Status:   DigitalEmployeeStatusDraft,
	}

	_, err = svc.CreateConfigRevision(context.Background(), CreateDigitalEmployeeConfigRevisionRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: employeeID,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found for wrong-tenant employee, got %v", err)
	}
	if len(repo.employeeConfigs) != 0 {
		t.Fatalf("expected missing employee not to insert config revision, got %#v", repo.employeeConfigs)
	}
}

func TestPreviewEffectiveConfigIncludesBudgetPolicy(t *testing.T) {
	svc := newTestService(t)
	tenantID := uuid.New()
	employeeID := uuid.New()
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		TeamConfig: TeamConfigInput{
			ID:       uuid.New(),
			TenantID: tenantID,
			TeamID:   uuid.New(),
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                uuid.New(),
			TenantID:          tenantID,
			DigitalEmployeeID: employeeID,
			BudgetPolicy:      map[string]any{"daily_token_limit": float64(9000)},
		},
	})

	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	budgetPolicy, ok := preview.EffectiveConfig["budget_policy"].(map[string]any)
	if !ok || budgetPolicy["daily_token_limit"] != float64(9000) {
		t.Fatalf("expected budget policy in effective config, got %#v", preview.EffectiveConfig["budget_policy"])
	}
}

func TestPreviewEffectiveConfigAllowsCapabilityBindingsOutsideFormerTeamAllowlist(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:               uuid.New(),
			CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID:                 uuid.New(),
			CapabilityBindings: map[string]any{"skills": []any{"database-troubleshooting"}},
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no capability allowlist blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
}

func TestPreviewEffectiveConfigIgnoresFormerContextOverrideValidation(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:            uuid.New(),
			ContextPolicy: map[string]any{"sources": []any{"monitoring", "logs"}},
		},
		EmployeeConfig: EmployeeConfigInput{
			ID: uuid.New(),
		},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
}

func TestPreviewEffectiveConfigBlocksApprovalPolicyDowngrade(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID: uuid.New(),
			ApprovalPolicy: map[string]any{
				"min_risk_for_human":          "high",
				"write_actions_require_human": true,
			},
		},
		EmployeeConfig: EmployeeConfigInput{ID: uuid.New()},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
}

func TestPreviewEffectiveConfigAllowsTeamInternalCollaborationPolicy(t *testing.T) {
	svc := newTestService(t)
	policy := map[string]any{
		"mode":                       "team_internal",
		"allow_same_team_handoffs":   true,
		"requires_external_approval": false,
	}
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:                          uuid.New(),
			CapabilityPolicy:            map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
			ContextPolicy:               map[string]any{"sources": []any{"monitoring"}},
			ApprovalPolicy:              map[string]any{"min_risk_for_human": "high"},
			InternalCollaborationPolicy: policy,
		},
		EmployeeConfig: EmployeeConfigInput{ID: uuid.New()},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}
	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
	if _, ok := preview.EffectiveConfig["internal_collaboration_policy"]; ok {
		t.Fatalf("expected internal collaboration policy to be omitted, got %#v", preview.EffectiveConfig["internal_collaboration_policy"])
	}
}

func TestPreviewEffectiveConfigReportsMalformedPolicyValues(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:               uuid.New(),
			CapabilityPolicy: map[string]any{"allowed_skills": []any{"incident-diagnosis"}},
		},
		EmployeeConfig: EmployeeConfigInput{ID: uuid.New()},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
}

func TestPreviewEffectiveConfigReportsUnknownApprovalRisk(t *testing.T) {
	svc := newTestService(t)
	preview, err := svc.PreviewEffectiveConfig(context.Background(), PreviewEffectiveConfigRequest{
		TenantID:          uuid.New(),
		DigitalEmployeeID: uuid.New(),
		TeamConfig: TeamConfigInput{
			ID:             uuid.New(),
			ApprovalPolicy: map[string]any{"min_risk_for_human": "high"},
		},
		EmployeeConfig: EmployeeConfigInput{ID: uuid.New()},
	})
	if err != nil {
		t.Fatalf("preview effective config: %v", err)
	}

	if len(preview.Validation.BlockingErrors) != 0 {
		t.Fatalf("expected no blocking errors, got %#v", preview.Validation.BlockingErrors)
	}
}

func TestGetSchedulingReadinessPassesForReadyEmployee(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	teamID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:           employeeID,
		TenantID:     tenantID,
		TeamID:       &teamID,
		OwnerUserID:  uuid.New(),
		EmployeeType: "requirements_analyst",
		Name:         "需求分析员工",
		Role:         "requirements_analyst",
		Status:       DigitalEmployeeStatusReady,
	}
	repo.schedulingCapabilityFacts = SchedulingCapabilityFacts{
		PersonalSkillCount:      1,
		InheritedSkillCount:     2,
		MissingRequiredSkills:   []string{"incident-review"},
		PersonalMCPServerCount:  1,
		InheritedMCPServerCount: 1,
		ConfiguredEnvVarCount:   2,
		MissingEnvironmentNames: []string{"OPENAI_API_KEY"},
	}

	readiness, err := svc.GetSchedulingReadiness(context.Background(), tenantID, employeeID)
	if err != nil {
		t.Fatalf("get scheduling readiness: %v", err)
	}
	if !readiness.ReadyForProjectScheduling {
		t.Fatalf("expected employee to be schedulable, got %#v", readiness)
	}
	assertReadinessCheck(t, readiness, "employee_status", ReadinessCheckPassed)
	assertReadinessCheck(t, readiness, "project_runtime", ReadinessCheckInfo)
	if readiness.Capabilities.Skills.PersonalCount != 1 || readiness.Capabilities.Skills.InheritedCount != 2 {
		t.Fatalf("unexpected skill counts: %#v", readiness.Capabilities.Skills)
	}
	if !stringSlicesEqual(readiness.Capabilities.Skills.MissingRequired, []string{"incident-review"}) {
		t.Fatalf("unexpected missing required skills: %#v", readiness.Capabilities.Skills.MissingRequired)
	}
	if readiness.Capabilities.MCPServers.PersonalCount != 1 || readiness.Capabilities.MCPServers.InheritedCount != 1 {
		t.Fatalf("unexpected MCP server counts: %#v", readiness.Capabilities.MCPServers)
	}
	if readiness.Capabilities.EnvironmentVariables.ConfiguredCount != 2 {
		t.Fatalf("unexpected configured env var count: %#v", readiness.Capabilities.EnvironmentVariables)
	}
	if !stringSlicesEqual(readiness.Capabilities.EnvironmentVariables.MissingNames, []string{"OPENAI_API_KEY"}) {
		t.Fatalf("unexpected missing environment names: %#v", readiness.Capabilities.EnvironmentVariables.MissingNames)
	}
	if readiness.ProjectExecutionSource != "project_runtime_readiness" {
		t.Fatalf("expected project runtime readiness source, got %q", readiness.ProjectExecutionSource)
	}
}

func TestGetSchedulingReadinessBlocksWhenEmployeeStatusIsNotRunnable(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:           employeeID,
		TenantID:     tenantID,
		OwnerUserID:  uuid.New(),
		EmployeeType: "requirements_analyst",
		Name:         "需求分析员工",
		Role:         "requirements_analyst",
		Status:       DigitalEmployeeStatusDisabled,
	}
	readiness, err := svc.GetSchedulingReadiness(context.Background(), tenantID, employeeID)
	if err != nil {
		t.Fatalf("get scheduling readiness: %v", err)
	}
	if readiness.ReadyForProjectScheduling {
		t.Fatalf("expected disabled employee to be blocked, got %#v", readiness)
	}
	assertReadinessCheck(t, readiness, "employee_status", ReadinessCheckBlocked)
}

func TestGetSchedulingReadinessDoesNotBlockWhenEffectiveConfigMissing(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	employeeID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:           employeeID,
		TenantID:     tenantID,
		OwnerUserID:  uuid.New(),
		EmployeeType: "requirements_analyst",
		Name:         "需求分析员工",
		Role:         "requirements_analyst",
		Status:       DigitalEmployeeStatusReady,
	}

	readiness, err := svc.GetSchedulingReadiness(context.Background(), tenantID, employeeID)
	if err != nil {
		t.Fatalf("get scheduling readiness: %v", err)
	}
	if !readiness.ReadyForProjectScheduling {
		t.Fatalf("expected missing effective config not to block readiness, got %#v", readiness)
	}
	assertReadinessCheck(t, readiness, "employee_status", ReadinessCheckPassed)
}

func assertReadinessCheck(t *testing.T, readiness *DigitalEmployeeSchedulingReadiness, code string, status ReadinessCheckStatus) {
	t.Helper()
	for _, check := range readiness.Checks {
		if check.Code == code {
			if check.Status != status {
				t.Fatalf("expected check %s status %s, got %#v", code, status, check)
			}
			return
		}
	}
	t.Fatalf("missing readiness check %s in %#v", code, readiness.Checks)
}

func TestJSONBFromMapRejectsUnsupportedValues(t *testing.T) {
	_, err := jsonbFromMap(map[string]any{"bad": func() {}}, "metadata")
	if err == nil {
		t.Fatalf("expected JSONB encoding error")
	}
	if !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("expected field name in error, got %v", err)
	}
}

func TestDigitalEmployeeConfigRevisionQueryMappingKeepsBudgetPolicy(t *testing.T) {
	now := pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}
	revision := queries.GetDigitalEmployeeConfigRevisionRow{
		ID:                    uuid.New(),
		TenantID:              uuid.New(),
		DigitalEmployeeID:     uuid.New(),
		RevisionNumber:        4,
		PersonaMemoryMarkdown: "# finance reviewer",
		CapabilityBindings:    []byte(`{"skills":["finance-reviewer"]}`),
		BudgetPolicy:          []byte(`{"daily_token_limit":50000}`),
		Status:                string(ConfigRevisionStatusDraft),
		CreatedAt:             now,
		UpdatedAt:             now,
	}

	input, err := employeeConfigInputFromQuery(digitalEmployeeConfigRevisionQueryAdapter{
		id:                    revision.ID,
		tenantID:              revision.TenantID,
		digitalEmployeeID:     revision.DigitalEmployeeID,
		revisionNumber:        revision.RevisionNumber,
		personaMemoryMarkdown: revision.PersonaMemoryMarkdown,
		capabilityBindings:    revision.CapabilityBindings,
		budgetPolicy:          revision.BudgetPolicy,
	})
	if err != nil {
		t.Fatalf("map employee config input: %v", err)
	}
	if input.BudgetPolicy["daily_token_limit"] != float64(50000) {
		t.Fatalf("expected input budget policy from query row, got %#v", input.BudgetPolicy)
	}

	record, err := configRevisionRecordFromQuery(digitalEmployeeConfigRevisionRecordAdapter{
		digitalEmployeeConfigRevisionQueryAdapter: digitalEmployeeConfigRevisionQueryAdapter{
			id:                    revision.ID,
			tenantID:              revision.TenantID,
			digitalEmployeeID:     revision.DigitalEmployeeID,
			revisionNumber:        revision.RevisionNumber,
			personaMemoryMarkdown: revision.PersonaMemoryMarkdown,
			capabilityBindings:    revision.CapabilityBindings,
			budgetPolicy:          revision.BudgetPolicy,
		},
		status:     revision.Status,
		approvedBy: revision.ApprovedBy,
		approvedAt: revision.ApprovedAt,
		archivedAt: revision.ArchivedAt,
		createdAt:  revision.CreatedAt,
		updatedAt:  revision.UpdatedAt,
	})
	if err != nil {
		t.Fatalf("map config revision record: %v", err)
	}
	if record.BudgetPolicy["daily_token_limit"] != float64(50000) {
		t.Fatalf("expected record budget policy from query row, got %#v", record.BudgetPolicy)
	}
}

func assertEmptyMap(t *testing.T, value map[string]any, label string) {
	t.Helper()
	if value == nil {
		t.Fatalf("expected %s to default to empty map, got nil", label)
	}
	if len(value) != 0 {
		t.Fatalf("expected empty %s, got %#v", label, value)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	svc, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc
}

func newEmployeeServiceForTest(t *testing.T) (*Service, *memoryRepository) {
	t.Helper()
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, repo
}

func seedConfigRevisionEmployee(repo *memoryRepository, tenantID, employeeID uuid.UUID) {
	teamID := uuid.New()
	repo.employees[employeeID] = DigitalEmployeeRecord{
		ID:       employeeID,
		TenantID: tenantID,
		TeamID:   &teamID,
		Name:     "Budget analyst",
		Role:     "analyst",
		Status:   DigitalEmployeeStatusDraft,
	}
}

func ptrUUID(value uuid.UUID) *uuid.UUID {
	return &value
}

func newCreateOptionsTestService(t *testing.T, capabilityPolicy, runtimeScopePolicy map[string]any) (*Service, *memoryRepository, uuid.UUID, uuid.UUID) {
	t.Helper()
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	teamConfigID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:                 teamConfigID,
		TenantID:           tenantID,
		TeamID:             teamID,
		CapabilityPolicy:   cloneMap(capabilityPolicy),
		RuntimeScopePolicy: cloneMap(runtimeScopePolicy),
	}
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	return svc, repo, tenantID, teamID
}

func newCreateDigitalEmployeeReadyFixture(t *testing.T) (*Service, *memoryRepository, *fakeRuntimeCommandDispatcher, CreateDigitalEmployeeRequest) {
	t.Helper()
	repo := newMemoryRepository()
	dispatcher := newFakeRuntimeCommandDispatcher()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	runtimeNodeID := uuid.New()
	teamConfigID := uuid.New()
	repo.teams[teamID] = tenantID
	repo.teamConfigs[teamConfigID] = TeamConfigInput{
		ID:       teamConfigID,
		TenantID: tenantID,
		TeamID:   teamID,
		CapabilityPolicy: map[string]any{
			"allowed_employee_types": []any{"database_admin"},
			"allowed_provider_types": []any{"codex"},
			"allowed_skills":         []any{"database-troubleshooting", "sql-review", "backup-restore", "performance-tuning"},
			"allowed_mcp_servers":    []any{"postgres-readonly", "mysql-readonly"},
		},
		ContextPolicy: map[string]any{
			"sources": []any{"runbook", "monitoring", "database_schema"},
		},
		ApprovalPolicy: map[string]any{
			"min_risk_for_human":          "high",
			"write_actions_require_human": true,
		},
		RuntimeScopePolicy: map[string]any{
			"allowed_provider_types": []any{"codex"},
		},
	}
	repo.currentTeamConfigByTeam[teamID] = teamConfigID
	if _, err := repo.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:              tenantID,
		Type:                  "database_admin",
		Label:                 "数据库管理",
		DefaultRole:           "database_admin",
		RecommendedSkills:     []string{"database-troubleshooting", "sql-review"},
		RecommendedMCPServers: []string{"postgres-readonly"},
		CapabilityBindings:    map[string]any{"skills": []string{"database-troubleshooting", "sql-review"}},
	}); err != nil {
		t.Fatalf("seed database_admin template: %v", err)
	}
	repo.preflight = validRuntimeProvisioningPreflight(tenantID, teamID, runtimeNodeID)
	repo.preflight.GovernanceSnapshot = map[string]any{
		"authorization":     "Bearer raw-token",
		"capability_policy": map[string]any{"api_key": "raw-key"},
	}
	repo.waitStatus = string(DigitalEmployeeRunStatusCompleted)
	dispatcher.connected["runtime-node-1"] = true
	return svc, repo, dispatcher, CreateDigitalEmployeeRequest{
		TenantID:              tenantID,
		TeamID:                &teamID,
		OwnerUserID:           ownerUserID,
		EmployeeType:          "database_admin",
		Name:                  "  Main database admin  ",
		AvatarAssetID:         "engineer-m-01",
		PersonaMemoryMarkdown: "# postgres operator",
		CapabilityBindings:    map[string]any{"external_capabilities": []string{"change-ticket"}, "skills": []string{"database-troubleshooting"}},
		ProviderType:          "  codex  ",
	}
}

func testCreateFlowEnvironmentCodec(t *testing.T) *EnvironmentValueCodec {
	t.Helper()
	codec, err := NewEnvironmentValueCodec(EnvironmentValueCodecConfig{
		Keys:        "v1:" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{5}, 32)),
		ActiveKeyID: "v1",
	})
	if err != nil {
		t.Fatalf("new env codec: %v", err)
	}
	return codec
}

func assertBlockingIssue(t *testing.T, validation EffectiveConfigValidation, code string) {
	t.Helper()
	for _, issue := range validation.BlockingErrors {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("expected blocking issue %q, got %#v", code, validation.BlockingErrors)
}

func stringSlicesEqual(got any, want []string) bool {
	var values []string
	switch typed := got.(type) {
	case []string:
		values = typed
	case []any:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return false
			}
			values = append(values, text)
		}
	default:
		return false
	}
	if len(values) != len(want) {
		return false
	}
	for index := range values {
		if values[index] != want[index] {
			return false
		}
	}
	return true
}

func assertBlockingIssuePath(t *testing.T, validation EffectiveConfigValidation, code, path string) {
	t.Helper()
	for _, issue := range validation.BlockingErrors {
		if issue.Code == code && issue.Path == path {
			return
		}
	}
	t.Fatalf("expected blocking issue %q at %q, got %#v", code, path, validation.BlockingErrors)
}

func stringListContains(value any, expected string) bool {
	for _, item := range stringList(value) {
		if item == expected {
			return true
		}
	}
	return false
}

func firstEmployeeID(repo *memoryRepository) uuid.UUID {
	for id := range repo.employees {
		return id
	}
	return uuid.Nil
}

func validRuntimeProvisioningPreflight(tenantID, teamID, runtimeNodeID uuid.UUID) RuntimeProvisioningPreflight {
	return RuntimeProvisioningPreflight{
		TenantID:              tenantID,
		TeamID:                teamID,
		RuntimeNodeID:         runtimeNodeID,
		NodeID:                "runtime-node-1",
		AgentHomeDir:          "/runtime/reported/agent-home",
		GovernanceSnapshot:    map[string]any{},
		HasActiveTeamConfig:   true,
		RuntimeOnline:         true,
		EnrollmentApproved:    true,
		RuntimeSessionActive:  true,
		ProviderAvailable:     true,
		ProviderPolicyAllowed: true,
		RuntimePolicyAllowed:  true,
	}
}

type memoryRepository struct {
	teams                     map[uuid.UUID]uuid.UUID
	employees                 map[uuid.UUID]DigitalEmployeeRecord
	instances                 map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord
	preflight                 RuntimeProvisioningPreflight
	preflightErr              error
	commandReceipts           map[string]*RuntimeCommandReceipt
	waitStatus                string
	waitErr                   error
	abortReasons              []string
	abortContextErrors        []error
	createdEmployeeCount      int
	teamConfigs               map[uuid.UUID]TeamConfigInput
	teamBaselines             map[uuid.UUID]TeamBaseline
	currentTeamConfigByTeam   map[uuid.UUID]uuid.UUID
	runtimeProviderOptions    []RuntimeProviderOption
	employeeConfigs           map[uuid.UUID]EmployeeConfigInput
	schedulingCapabilityFacts SchedulingCapabilityFacts
	envVars                   map[string]EnvironmentVariableRecord
	nextConfigRevisionNumber  int32
	createdConfigRevision     CreateConfigRevisionParams
	digitalEmployeeOverview   *DigitalEmployeeOverview
	lastOverviewRequest       GetDigitalEmployeeOverviewRequest
	digitalEmployeeActivity   []DigitalEmployeeActivityItem
	lastActivityRequest       GetDigitalEmployeeActivityRequest
	deleteBlockers            []DigitalEmployeeDeleteBlocker
	deleteCascadeResult       DigitalEmployeeDeleteCascadeResult
	deleteCascadeCount        int
	deleteAuditEvents         []DigitalEmployeeDeleteAuditEventParams
	waitHook                  func(context.Context, uuid.UUID, string, time.Duration) (*RuntimeCommandReceipt, error)
	transactionCount          int
	transactionCommitCount    int
	transactionRollbackCount  int
	inTransaction             bool
	templates                 map[uuid.UUID][]EmployeeTemplateRecord
	registrySkills            []CapabilityRegistryOption
	registryMCPServers        []CapabilityRegistryOption
	boundSkillIDs             map[uuid.UUID][]uuid.UUID
	boundMCPServerIDs         map[uuid.UUID][]uuid.UUID
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		teams:                    make(map[uuid.UUID]uuid.UUID),
		employees:                make(map[uuid.UUID]DigitalEmployeeRecord),
		instances:                make(map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord),
		commandReceipts:          make(map[string]*RuntimeCommandReceipt),
		teamConfigs:              make(map[uuid.UUID]TeamConfigInput),
		teamBaselines:            make(map[uuid.UUID]TeamBaseline),
		currentTeamConfigByTeam:  make(map[uuid.UUID]uuid.UUID),
		employeeConfigs:          make(map[uuid.UUID]EmployeeConfigInput),
		envVars:                  make(map[string]EnvironmentVariableRecord),
		nextConfigRevisionNumber: 1,
		templates:                make(map[uuid.UUID][]EmployeeTemplateRecord),
	}
}

// builtinEmployeeTemplateFixtures intentionally returns no default templates.
// The platform does not ship any pre-seeded digital employee templates —
// every tenant starts with an empty template catalog and templates are
// created explicitly via CreateEmployeeTemplate. Callers still invoke this
// helper (e.g. via len(builtinEmployeeTemplateFixtures(tenantID))+1 for the
// custom_agent sentinel) so the signature is preserved for a length of 0.
func builtinEmployeeTemplateFixtures(tenantID uuid.UUID) []EmployeeTemplateRecord {
	return []EmployeeTemplateRecord{}
}

func (r *memoryRepository) templatesForTenant(tenantID uuid.UUID) []EmployeeTemplateRecord {
	if _, ok := r.templates[tenantID]; !ok {
		r.templates[tenantID] = builtinEmployeeTemplateFixtures(tenantID)
	}
	return r.templates[tenantID]
}

func (r *memoryRepository) ListEmployeeTemplates(ctx context.Context, params ListEmployeeTemplatesParams) ([]EmployeeTemplateRecord, error) {
	result := make([]EmployeeTemplateRecord, 0)
	for _, tmpl := range r.templatesForTenant(params.TenantID) {
		if params.ActiveOnly && tmpl.Status != "active" {
			continue
		}
		result = append(result, tmpl)
	}
	return result, nil
}

func (r *memoryRepository) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error) {
	for _, tmpl := range r.templatesForTenant(tenantID) {
		if tmpl.ID == templateID {
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) GetEmployeeTemplateByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTemplateRecord, error) {
	for _, tmpl := range r.templatesForTenant(tenantID) {
		if tmpl.Type == employeeType {
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	for _, tmpl := range r.templatesForTenant(params.TenantID) {
		if tmpl.Type == params.Type {
			return EmployeeTemplateRecord{}, fmt.Errorf("%w: template type already exists for this tenant", ErrInvalidInput)
		}
	}
	now := time.Now().UTC()
	record := EmployeeTemplateRecord{
		ID:                       uuid.New(),
		TenantID:                 params.TenantID,
		Type:                     params.Type,
		Label:                    params.Label,
		Description:              params.Description,
		DefaultRole:              params.DefaultRole,
		RecommendedSkills:        params.RecommendedSkills,
		RecommendedMCPServers:    params.RecommendedMCPServers,
		RecommendedProviderTypes: params.RecommendedProviderTypes,
		PersonaMemoryMarkdown:    params.PersonaMemoryMarkdown,
		CapabilityBindings:       cloneMap(params.CapabilityBindings),
		BudgetPolicy:             cloneMap(params.BudgetPolicy),
		Metadata:                 params.Metadata,
		Status:                   "active",
		IsSystem:                 false,
		CreatedAt:                now,
		UpdatedAt:                now,
	}
	r.templates[params.TenantID] = append(r.templatesForTenant(params.TenantID), record)
	return record, nil
}

func (r *memoryRepository) UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	templates := r.templatesForTenant(params.TenantID)
	for i, tmpl := range templates {
		if tmpl.ID == params.ID {
			tmpl.Label = params.Label
			tmpl.Description = params.Description
			tmpl.DefaultRole = params.DefaultRole
			tmpl.RecommendedSkills = params.RecommendedSkills
			tmpl.RecommendedMCPServers = params.RecommendedMCPServers
			tmpl.RecommendedProviderTypes = params.RecommendedProviderTypes
			tmpl.PersonaMemoryMarkdown = params.PersonaMemoryMarkdown
			tmpl.CapabilityBindings = cloneMap(params.CapabilityBindings)
			tmpl.BudgetPolicy = cloneMap(params.BudgetPolicy)
			tmpl.Metadata = params.Metadata
			tmpl.UpdatedAt = time.Now().UTC()
			templates[i] = tmpl
			r.templates[params.TenantID] = templates
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error) {
	templates := r.templatesForTenant(tenantID)
	for i, tmpl := range templates {
		if tmpl.ID == templateID {
			tmpl.Status = status
			tmpl.UpdatedAt = time.Now().UTC()
			templates[i] = tmpl
			r.templates[tenantID] = templates
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) SoftDeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	templates := r.templatesForTenant(tenantID)
	for i, tmpl := range templates {
		if tmpl.ID == templateID {
			r.templates[tenantID] = append(templates[:i], templates[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (r *memoryRepository) ListEmployeeTemplateLabels(ctx context.Context, tenantID uuid.UUID) (map[string]string, error) {
	labels := make(map[string]string)
	for _, tmpl := range r.templatesForTenant(tenantID) {
		labels[tmpl.Type] = tmpl.Label
	}
	return labels, nil
}

type overviewRepositoryStub struct {
	Repository
	req      GetDigitalEmployeeOverviewRequest
	overview *DigitalEmployeeOverview
	err      error
}

func (r *overviewRepositoryStub) GetDigitalEmployeeOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error) {
	r.req = req
	if r.err != nil {
		return nil, r.err
	}
	if r.overview != nil {
		return r.overview, nil
	}
	return &DigitalEmployeeOverview{
		Summary:    DigitalEmployeeOverviewSummary{},
		Items:      []DigitalEmployeeOverviewItem{},
		Filters:    DigitalEmployeeOverviewFilters{},
		Pagination: OverviewPagination{Limit: req.Limit, Offset: req.Offset, TotalCount: 0},
	}, nil
}

func (r *memoryRepository) WithTransaction(ctx context.Context, fn func(Repository) error) error {
	if r.inTransaction {
		return errors.New("nested transaction")
	}
	snapshot := r.snapshot()
	r.transactionCount++
	r.inTransaction = true
	err := fn(r)
	r.inTransaction = false
	if err != nil {
		r.restore(snapshot)
		r.transactionRollbackCount++
		return err
	}
	if err := ctx.Err(); err != nil {
		r.restore(snapshot)
		r.transactionRollbackCount++
		return err
	}
	r.transactionCommitCount++
	return nil
}

func (r *memoryRepository) CreateDigitalEmployee(_ context.Context, params CreateDigitalEmployeeParams) (DigitalEmployeeRecord, error) {
	now := time.Now().UTC()
	record := DigitalEmployeeRecord{
		ID:               uuid.New(),
		TenantID:         params.TenantID,
		TeamID:           params.TeamID,
		OwnerUserID:      params.OwnerUserID,
		EmployeeType:     params.EmployeeType,
		ProviderType:     params.ProviderType,
		Name:             params.Name,
		Role:             params.Role,
		Description:      params.Description,
		Status:           params.Status,
		PermissionPolicy: cloneMap(params.PermissionPolicy),
		ContextPolicy:    cloneMap(params.ContextPolicy),
		ApprovalPolicy:   cloneMap(params.ApprovalPolicy),
		RiskLevel:        params.RiskLevel,
		Metadata:         cloneMap(params.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.employees[record.ID] = record
	r.createdEmployeeCount++
	return record, nil
}

func (r *memoryRepository) ListDigitalEmployees(_ context.Context, params ListDigitalEmployeesParams) ([]DigitalEmployeeRecord, error) {
	records := make([]DigitalEmployeeRecord, 0, len(r.employees))
	for _, record := range r.employees {
		if record.TenantID != params.TenantID {
			continue
		}
		if record.DeletedAt != nil {
			continue
		}
		if params.Status != "" && record.Status != params.Status {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *memoryRepository) GetDigitalEmployee(_ context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	record, ok := r.employees[employeeID]
	if !ok || record.TenantID != tenantID || record.DeletedAt != nil {
		return DigitalEmployeeRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) GetDigitalEmployeeForDelete(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	return r.GetDigitalEmployee(ctx, tenantID, employeeID)
}

func (r *memoryRepository) ListDigitalEmployeeDeleteBlockers(_ context.Context, _, _ uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error) {
	return append([]DigitalEmployeeDeleteBlocker(nil), r.deleteBlockers...), nil
}

func (r *memoryRepository) SoftDeleteDigitalEmployeeCascade(_ context.Context, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error) {
	employee, ok := r.employees[params.DigitalEmployeeID]
	if !ok || employee.TenantID != params.TenantID || employee.DeletedAt != nil {
		return DigitalEmployeeDeleteCascadeResult{}, ErrNotFound
	}
	deletedAt := params.DeletedAt.UTC()
	employee.Status = DigitalEmployeeStatusDisabled
	employee.DisabledAt = &deletedAt
	employee.DeletedAt = &deletedAt
	employee.UpdatedAt = deletedAt
	r.employees[params.DigitalEmployeeID] = employee
	r.deleteCascadeCount++
	return r.deleteCascadeResult, nil
}

func (r *memoryRepository) CreateDigitalEmployeeDeleteAuditEvent(_ context.Context, params DigitalEmployeeDeleteAuditEventParams) error {
	r.deleteAuditEvents = append(r.deleteAuditEvents, params)
	return nil
}

func (r *memoryRepository) GetDigitalEmployeeActivity(_ context.Context, req GetDigitalEmployeeActivityRequest) ([]DigitalEmployeeActivityItem, error) {
	r.lastActivityRequest = req
	return r.digitalEmployeeActivity, nil
}

func (r *memoryRepository) GetDigitalEmployeeOverview(_ context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error) {
	r.lastOverviewRequest = req
	if r.digitalEmployeeOverview != nil {
		return r.digitalEmployeeOverview, nil
	}
	return &DigitalEmployeeOverview{
		Summary:    DigitalEmployeeOverviewSummary{},
		Items:      []DigitalEmployeeOverviewItem{},
		Filters:    DigitalEmployeeOverviewFilters{},
		Pagination: OverviewPagination{Limit: req.Limit, Offset: req.Offset, TotalCount: 0},
	}, nil
}

func (r *memoryRepository) AreRuntimeReady(_ context.Context, _ uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	ready := make(map[uuid.UUID]bool, len(employeeIDs))
	for _, id := range employeeIDs {
		ready[id] = true
	}
	return ready, nil
}

func (r *memoryRepository) EnsureTeamExists(_ context.Context, tenantID, teamID uuid.UUID) error {
	teamTenantID, ok := r.teams[teamID]
	if !ok || teamTenantID != tenantID {
		return ErrNotFound
	}
	return nil
}

func (r *memoryRepository) GetTeamBaseline(_ context.Context, tenantID, teamID uuid.UUID) (TeamBaseline, error) {
	if err := r.EnsureTeamExists(context.Background(), tenantID, teamID); err != nil {
		return TeamBaseline{}, err
	}
	baseline, ok := r.teamBaselines[teamID]
	if !ok {
		return TeamBaseline{}, nil
	}
	return TeamBaseline{
		Constitution: cloneMap(baseline.Constitution),
		Skills:       append([]string(nil), baseline.Skills...),
		MCPServers:   append([]string(nil), baseline.MCPServers...),
	}, nil
}

func (r *memoryRepository) ListSkillCapabilityOptions(_ context.Context, _ uuid.UUID) ([]CapabilityRegistryOption, error) {
	return append([]CapabilityRegistryOption(nil), r.registrySkills...), nil
}

func (r *memoryRepository) ListMCPCapabilityOptions(_ context.Context, _ uuid.UUID) ([]CapabilityRegistryOption, error) {
	return append([]CapabilityRegistryOption(nil), r.registryMCPServers...), nil
}

func (r *memoryRepository) ResolveSkillIDsBySlugs(_ context.Context, _ uuid.UUID, slugs []string) (map[string]uuid.UUID, error) {
	return resolveRegistryKeys(r.registrySkills, slugs), nil
}

func (r *memoryRepository) ResolveMCPServerIDsByKeys(_ context.Context, _ uuid.UUID, keys []string) (map[string]uuid.UUID, error) {
	return resolveRegistryKeys(r.registryMCPServers, keys), nil
}

func resolveRegistryKeys(registry []CapabilityRegistryOption, keys []string) map[string]uuid.UUID {
	byKey := make(map[string]uuid.UUID, len(registry))
	for _, option := range registry {
		byKey[option.Key] = option.ID
	}
	resolved := make(map[string]uuid.UUID, len(keys))
	for _, key := range keys {
		if id, ok := byKey[key]; ok {
			resolved[key] = id
		}
	}
	return resolved
}

func (r *memoryRepository) BindSkillsToEmployee(_ context.Context, _ uuid.UUID, employeeID uuid.UUID, skillIDs []uuid.UUID) error {
	if r.boundSkillIDs == nil {
		r.boundSkillIDs = make(map[uuid.UUID][]uuid.UUID)
	}
	r.boundSkillIDs[employeeID] = append(r.boundSkillIDs[employeeID], skillIDs...)
	return nil
}

func (r *memoryRepository) BindMCPServersToEmployee(_ context.Context, _ uuid.UUID, employeeID uuid.UUID, serverIDs []uuid.UUID) error {
	if r.boundMCPServerIDs == nil {
		r.boundMCPServerIDs = make(map[uuid.UUID][]uuid.UUID)
	}
	r.boundMCPServerIDs[employeeID] = append(r.boundMCPServerIDs[employeeID], serverIDs...)
	return nil
}

func (r *memoryRepository) ListRuntimeProviderOptionsForCreate(_ context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error) {
	if err := r.EnsureTeamExists(context.Background(), tenantID, teamID); err != nil {
		return nil, err
	}
	return append([]RuntimeProviderOption(nil), r.runtimeProviderOptions...), nil
}

func (r *memoryRepository) ListRuntimeProviderOptionsForTeamLessCreate(_ context.Context, _ uuid.UUID) ([]RuntimeProviderOption, error) {
	return append([]RuntimeProviderOption(nil), r.runtimeProviderOptions...), nil
}

func (r *memoryRepository) UpdateDigitalEmployeeStatus(_ context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	record, ok := r.employees[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeRecord{}, ErrNotFound
	}
	record.Status = status
	record.UpdatedAt = time.Now().UTC()
	r.employees[employeeID] = record
	return record, nil
}

func (r *memoryRepository) UpsertDigitalEmployeeExecutionInstance(_ context.Context, params UpsertExecutionInstanceParams) (DigitalEmployeeExecutionInstanceRecord, error) {
	if params.TenantID == uuid.Nil || params.DigitalEmployeeID == uuid.Nil {
		return DigitalEmployeeExecutionInstanceRecord{}, errors.New("tenant and employee are required")
	}
	now := time.Now().UTC()
	record, ok := r.instances[params.DigitalEmployeeID]
	if !ok {
		record.ID = uuid.New()
		record.CreatedAt = now
	}
	record.TenantID = params.TenantID
	record.DigitalEmployeeID = params.DigitalEmployeeID
	record.RuntimeNodeID = params.RuntimeNodeID
	record.ProviderType = params.ProviderType
	record.AgentHomeDir = params.AgentHomeDir
	record.WorkspacePolicy = cloneMap(params.WorkspacePolicy)
	record.SessionPolicy = cloneMap(params.SessionPolicy)
	record.RuntimeSelector = cloneMap(params.RuntimeSelector)
	record.CapacityRequirements = cloneMap(params.CapacityRequirements)
	record.FallbackPolicy = cloneMap(params.FallbackPolicy)
	record.Status = params.Status
	record.Metadata = cloneMap(params.Metadata)
	record.UpdatedAt = now
	r.instances[params.DigitalEmployeeID] = record
	return record, nil
}

func (r *memoryRepository) GetDigitalEmployeeExecutionInstanceByEmployeeID(_ context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeExecutionInstanceRecord, error) {
	record, ok := r.instances[employeeID]
	if !ok || record.TenantID != tenantID {
		return DigitalEmployeeExecutionInstanceRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) GetDigitalEmployeeOperationalSignals(_ context.Context, _ uuid.UUID, _ []uuid.UUID) (map[uuid.UUID]OperationalSignals, error) {
	return map[uuid.UUID]OperationalSignals{}, nil
}

func (r *memoryRepository) ListEnvironmentVariables(_ context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableRecord, error) {
	records := make([]EnvironmentVariableRecord, 0)
	for _, record := range r.envVars {
		if record.TenantID == req.TenantID && record.DigitalEmployeeID == req.DigitalEmployeeID && record.Status == EnvironmentVariableStatusActive {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r *memoryRepository) UpsertEnvironmentVariable(_ context.Context, req UpsertEnvironmentVariableStoreRequest) (EnvironmentVariableRecord, error) {
	now := time.Now().UTC()
	record, ok := r.envVars[req.Name]
	if !ok {
		record.ID = uuid.New()
		record.CreatedAt = now
		record.CreatedBy = validUUIDPtr(req.UpdatedBy)
	}
	record.TenantID = req.TenantID
	record.TeamID = req.TeamID
	record.DigitalEmployeeID = req.DigitalEmployeeID
	record.Name = req.Name
	record.EncryptedValue = req.EncryptedValue
	record.EncryptionKeyID = req.EncryptionKeyID
	record.ValueFingerprint = req.ValueFingerprint
	record.Sensitive = req.Sensitive
	record.Status = EnvironmentVariableStatusActive
	record.UpdatedBy = validUUIDPtr(req.UpdatedBy)
	record.UpdatedAt = now
	r.envVars[req.Name] = record
	return record, nil
}

func (r *memoryRepository) DeleteEnvironmentVariable(_ context.Context, req DeleteEnvironmentVariableRequest) error {
	record, ok := r.envVars[req.Name]
	if !ok || record.TenantID != req.TenantID || record.DigitalEmployeeID != req.DigitalEmployeeID {
		return nil
	}
	delete(r.envVars, req.Name)
	return nil
}

func (r *memoryRepository) ListRuntimeEnvironmentVariables(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]EnvironmentVariableRecord, error) {
	return r.ListEnvironmentVariables(ctx, ListEnvironmentVariablesRequest{TenantID: tenantID, DigitalEmployeeID: digitalEmployeeID})
}

func (r *memoryRepository) CreateDigitalEmployeeConfigRevision(_ context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error) {
	r.createdConfigRevision = params
	now := time.Now().UTC()
	approvedAt := params.ApprovedAt
	record := DigitalEmployeeConfigRevisionRecord{
		ID:                    uuid.New(),
		TenantID:              params.TenantID,
		DigitalEmployeeID:     params.DigitalEmployeeID,
		RevisionNumber:        params.RevisionNumber,
		PersonaMemoryMarkdown: params.PersonaMemoryMarkdown,
		CapabilityBindings:    cloneMap(params.CapabilityBindings),
		BudgetPolicy:          cloneMap(params.BudgetPolicy),
		Status:                params.Status,
		ApprovedBy:            validUUIDPtr(params.ApprovedBy),
		ApprovedAt:            cloneTimePtr(approvedAt),
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	r.employeeConfigs[record.ID] = EmployeeConfigInput{
		ID:                    record.ID,
		TenantID:              record.TenantID,
		DigitalEmployeeID:     record.DigitalEmployeeID,
		RevisionNumber:        record.RevisionNumber,
		PersonaMemoryMarkdown: record.PersonaMemoryMarkdown,
		CapabilityBindings:    cloneMap(record.CapabilityBindings),
		BudgetPolicy:          cloneMap(record.BudgetPolicy),
	}
	return record, nil
}

func (r *memoryRepository) GetDigitalEmployeeConfigRevision(_ context.Context, tenantID, digitalEmployeeID, employeeConfigRevisionID uuid.UUID) (EmployeeConfigInput, error) {
	record, ok := r.employeeConfigs[employeeConfigRevisionID]
	if !ok || record.TenantID != tenantID || record.DigitalEmployeeID != digitalEmployeeID {
		return EmployeeConfigInput{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) GetLatestDigitalEmployeeConfigRevision(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) (EmployeeConfigInput, error) {
	var latest EmployeeConfigInput
	found := false
	for _, record := range r.employeeConfigs {
		if record.TenantID != tenantID || record.DigitalEmployeeID != digitalEmployeeID {
			continue
		}
		if !found || record.RevisionNumber > latest.RevisionNumber {
			latest = record
			found = true
		}
	}
	if !found {
		return EmployeeConfigInput{}, ErrNotFound
	}
	return latest, nil
}

func (r *memoryRepository) GetNextDigitalEmployeeConfigRevisionNumber(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) (int32, error) {
	if tenantID == uuid.Nil || digitalEmployeeID == uuid.Nil {
		return 0, errors.New("tenant and employee are required")
	}
	return r.nextConfigRevisionNumber, nil
}

func (r *memoryRepository) GetSchedulingCapabilityFacts(_ context.Context, tenantID, digitalEmployeeID uuid.UUID) (SchedulingCapabilityFacts, error) {
	if _, ok := r.employees[digitalEmployeeID]; !ok {
		return SchedulingCapabilityFacts{}, ErrNotFound
	}
	if r.employees[digitalEmployeeID].TenantID != tenantID {
		return SchedulingCapabilityFacts{}, ErrNotFound
	}
	return r.schedulingCapabilityFacts, nil
}

func (r *memoryRepository) GetDigitalEmployeeRunStats(_ context.Context, _, _ uuid.UUID) (DigitalEmployeeRunStats, error) {
	return DigitalEmployeeRunStats{}, nil
}

func (r *memoryRepository) ListRunsDetailed(_ context.Context, _, _ uuid.UUID, _ DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error) {
	return &DigitalEmployeeRunListResult{}, nil
}

func (r *memoryRepository) GetRuntimeProvisioningPreflight(_ context.Context, tenantID, teamID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error) {
	if r.preflightErr != nil {
		return RuntimeProvisioningPreflight{}, r.preflightErr
	}
	if r.preflight.TenantID != tenantID || r.preflight.TeamID != teamID || r.preflight.RuntimeNodeID != runtimeNodeID || providerType == "" {
		return RuntimeProvisioningPreflight{}, ErrNotFound
	}
	return r.preflight, nil
}

func (r *memoryRepository) GetRuntimeProvisioningPreflightTeamLess(_ context.Context, tenantID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error) {
	if r.preflightErr != nil {
		return RuntimeProvisioningPreflight{}, r.preflightErr
	}
	if r.preflight.TenantID != tenantID || r.preflight.RuntimeNodeID != runtimeNodeID || providerType == "" {
		return RuntimeProvisioningPreflight{}, ErrNotFound
	}
	return r.preflight, nil
}

func (r *memoryRepository) CreateRuntimeCommandReceipt(_ context.Context, req CreateRuntimeCommandReceiptRequest) error {
	r.commandReceipts[req.CommandID] = &RuntimeCommandReceipt{
		ID:            uuid.New(),
		TenantID:      req.TenantID,
		CommandID:     req.CommandID,
		CommandType:   req.CommandType,
		RuntimeNodeID: req.RuntimeNodeID,
		NodeID:        req.NodeID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		Status:        req.Status,
		Payload:       redactRuntimeEventPayloadForPersistence(req.Payload),
		DispatchedAt:  cloneTimePtr(req.DispatchedAt),
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	return nil
}

func (r *memoryRepository) WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error) {
	if r.waitHook != nil {
		return r.waitHook(ctx, tenantID, commandID, interval)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.waitErr != nil {
		return nil, r.waitErr
	}
	receipt, ok := r.commandReceipts[commandID]
	if !ok || receipt.TenantID != tenantID {
		return nil, ErrNotFound
	}
	if r.waitStatus != "" {
		now := time.Now().UTC()
		receipt.Status = r.waitStatus
		receipt.CompletedAt = &now
	}
	if receipt.Status == string(DigitalEmployeeRunStatusCompleted) {
		instance, ok := r.instances[receipt.ResourceID]
		if !ok {
			for _, record := range r.instances {
				if record.ID == receipt.ResourceID {
					instance = record
					ok = true
					break
				}
			}
		}
		if ok {
			now := time.Now().UTC()
			instance.Status = ExecutionInstanceStatusReady
			instance.ReadyAt = &now
			r.instances[instance.DigitalEmployeeID] = instance
			employeeRecord := r.employees[instance.DigitalEmployeeID]
			employeeRecord.Status = DigitalEmployeeStatusReady
			employeeRecord.UpdatedAt = now
			r.employees[instance.DigitalEmployeeID] = employeeRecord
		}
	}
	return receipt, nil
}

func (r *memoryRepository) AbortProvisionedDigitalEmployee(ctx context.Context, tenantID, employeeID, executionInstanceID uuid.UUID, reason string) error {
	r.abortReasons = append(r.abortReasons, reason)
	r.abortContextErrors = append(r.abortContextErrors, ctx.Err())
	now := time.Now().UTC()
	employeeRecord, ok := r.employees[employeeID]
	if ok && employeeRecord.TenantID == tenantID {
		employeeRecord.Status = DigitalEmployeeStatusError
		employeeRecord.DeletedAt = &now
		employeeRecord.UpdatedAt = now
		r.employees[employeeID] = employeeRecord
	}
	instance, ok := r.instances[employeeID]
	if ok && instance.TenantID == tenantID && (executionInstanceID == uuid.Nil || instance.ID == executionInstanceID) {
		instance.Status = ExecutionInstanceStatusError
		instance.ErrorAt = &now
		instance.ErrorMessage = &reason
		instance.DeletedAt = &now
		instance.UpdatedAt = now
		r.instances[employeeID] = instance
	}
	for _, receipt := range r.commandReceipts {
		if receipt.TenantID == tenantID && (executionInstanceID == uuid.Nil || receipt.ResourceID == executionInstanceID) {
			receipt.Status = string(DigitalEmployeeRunStatusFailed)
			receipt.ErrorMessage = &reason
			receipt.CompletedAt = &now
			receipt.UpdatedAt = now
		}
	}
	return nil
}

type memoryRepositorySnapshot struct {
	employees                map[uuid.UUID]DigitalEmployeeRecord
	instances                map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord
	commandReceipts          map[string]*RuntimeCommandReceipt
	employeeConfigs          map[uuid.UUID]EmployeeConfigInput
	envVars                  map[string]EnvironmentVariableRecord
	nextConfigRevisionNumber int32
	createdEmployeeCount     int
	createdConfigRevision    CreateConfigRevisionParams
	deleteCascadeCount       int
	deleteAuditEvents        []DigitalEmployeeDeleteAuditEventParams
}

func (r *memoryRepository) snapshot() memoryRepositorySnapshot {
	return memoryRepositorySnapshot{
		employees:                cloneEmployeeRecordMap(r.employees),
		instances:                cloneExecutionInstanceRecordMap(r.instances),
		commandReceipts:          cloneCommandReceiptMap(r.commandReceipts),
		employeeConfigs:          cloneEmployeeConfigInputMap(r.employeeConfigs),
		envVars:                  cloneEnvironmentVariableRecordMap(r.envVars),
		nextConfigRevisionNumber: r.nextConfigRevisionNumber,
		createdEmployeeCount:     r.createdEmployeeCount,
		createdConfigRevision:    cloneCreateConfigRevisionParams(r.createdConfigRevision),
		deleteCascadeCount:       r.deleteCascadeCount,
		deleteAuditEvents:        append([]DigitalEmployeeDeleteAuditEventParams(nil), r.deleteAuditEvents...),
	}
}

func (r *memoryRepository) restore(snapshot memoryRepositorySnapshot) {
	r.employees = snapshot.employees
	r.instances = snapshot.instances
	r.commandReceipts = snapshot.commandReceipts
	r.employeeConfigs = snapshot.employeeConfigs
	r.envVars = snapshot.envVars
	r.nextConfigRevisionNumber = snapshot.nextConfigRevisionNumber
	r.createdEmployeeCount = snapshot.createdEmployeeCount
	r.createdConfigRevision = snapshot.createdConfigRevision
	r.deleteCascadeCount = snapshot.deleteCascadeCount
	r.deleteAuditEvents = snapshot.deleteAuditEvents
}

func cloneEmployeeRecordMap(values map[uuid.UUID]DigitalEmployeeRecord) map[uuid.UUID]DigitalEmployeeRecord {
	cloned := make(map[uuid.UUID]DigitalEmployeeRecord, len(values))
	for id, record := range values {
		record.TeamID = validUUIDPtr(record.TeamID)
		record.Description = cloneStringPtrForTest(record.Description)
		record.PermissionPolicy = cloneMap(record.PermissionPolicy)
		record.ContextPolicy = cloneMap(record.ContextPolicy)
		record.ApprovalPolicy = cloneMap(record.ApprovalPolicy)
		record.Metadata = cloneMap(record.Metadata)
		record.DisabledAt = cloneTimePtr(record.DisabledAt)
		record.ArchivedAt = cloneTimePtr(record.ArchivedAt)
		record.DeletedAt = cloneTimePtr(record.DeletedAt)
		cloned[id] = record
	}
	return cloned
}

func cloneExecutionInstanceRecordMap(values map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord) map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord {
	cloned := make(map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord, len(values))
	for id, record := range values {
		record.WorkspacePolicy = cloneMap(record.WorkspacePolicy)
		record.SessionPolicy = cloneMap(record.SessionPolicy)
		record.RuntimeSelector = cloneMap(record.RuntimeSelector)
		record.CapacityRequirements = cloneMap(record.CapacityRequirements)
		record.FallbackPolicy = cloneMap(record.FallbackPolicy)
		record.ReadyAt = cloneTimePtr(record.ReadyAt)
		record.DisabledAt = cloneTimePtr(record.DisabledAt)
		record.ErrorAt = cloneTimePtr(record.ErrorAt)
		record.ErrorMessage = cloneStringPtrForTest(record.ErrorMessage)
		record.DeletedAt = cloneTimePtr(record.DeletedAt)
		record.Metadata = cloneMap(record.Metadata)
		cloned[id] = record
	}
	return cloned
}

func cloneCommandReceiptMap(values map[string]*RuntimeCommandReceipt) map[string]*RuntimeCommandReceipt {
	cloned := make(map[string]*RuntimeCommandReceipt, len(values))
	for id, receipt := range values {
		if receipt == nil {
			cloned[id] = nil
			continue
		}
		copied := *receipt
		copied.Payload = cloneMap(receipt.Payload)
		copied.Result = cloneMap(receipt.Result)
		copied.ErrorMessage = cloneStringPtrForTest(receipt.ErrorMessage)
		copied.DispatchedAt = cloneTimePtr(receipt.DispatchedAt)
		copied.CompletedAt = cloneTimePtr(receipt.CompletedAt)
		cloned[id] = &copied
	}
	return cloned
}

func cloneEmployeeConfigInputMap(values map[uuid.UUID]EmployeeConfigInput) map[uuid.UUID]EmployeeConfigInput {
	cloned := make(map[uuid.UUID]EmployeeConfigInput, len(values))
	for id, record := range values {
		record.CapabilityBindings = cloneMap(record.CapabilityBindings)
		record.BudgetPolicy = cloneMap(record.BudgetPolicy)
		cloned[id] = record
	}
	return cloned
}

func cloneEnvironmentVariableRecordMap(values map[string]EnvironmentVariableRecord) map[string]EnvironmentVariableRecord {
	cloned := make(map[string]EnvironmentVariableRecord, len(values))
	for name, record := range values {
		record.CreatedBy = validUUIDPtr(record.CreatedBy)
		record.UpdatedBy = validUUIDPtr(record.UpdatedBy)
		cloned[name] = record
	}
	return cloned
}

func cloneCreateConfigRevisionParams(params CreateConfigRevisionParams) CreateConfigRevisionParams {
	params.CapabilityBindings = cloneMap(params.CapabilityBindings)
	params.BudgetPolicy = cloneMap(params.BudgetPolicy)
	params.ApprovedBy = validUUIDPtr(params.ApprovedBy)
	params.ApprovedAt = cloneTimePtr(params.ApprovedAt)
	return params
}

func cloneStringPtrForTest(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

type fakeRuntimeCommandDispatcher struct {
	connected  map[string]bool
	commands   []cpruntime.RuntimeCommand
	err        error
	onDispatch func(string, cpruntime.RuntimeCommand)
}

func newFakeRuntimeCommandDispatcher() *fakeRuntimeCommandDispatcher {
	return &fakeRuntimeCommandDispatcher{connected: make(map[string]bool)}
}

func (f *fakeRuntimeCommandDispatcher) IsConnected(nodeID string) bool {
	return f.connected[nodeID]
}

func (f *fakeRuntimeCommandDispatcher) Dispatch(_ context.Context, nodeID string, command cpruntime.RuntimeCommand) error {
	if f.err != nil {
		return f.err
	}
	if !f.IsConnected(nodeID) {
		return cpruntime.ErrRuntimeNotConnected
	}
	if f.onDispatch != nil {
		f.onDispatch(nodeID, command)
	}
	f.commands = append(f.commands, command)
	return nil
}
