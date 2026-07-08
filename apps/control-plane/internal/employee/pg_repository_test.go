package employee

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestOverviewInt32FromJSONString(t *testing.T) {
	require.Equal(t, int32(1600), int32FromJSONString("1600"))
	require.Equal(t, int32(0), int32FromJSONString(""))
	require.Equal(t, int32(0), int32FromJSONString("not-a-number"))
}

func TestGetTeamBaseline(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRepoIntegrationTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_team_baseline_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepoTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	teamID := uuid.MustParse("00000000-0000-0000-0000-000000000101")
	skillAID := uuid.New()
	skillBID := uuid.New()
	activeMCPID := uuid.New()
	disabledServerMCPID := uuid.New()
	disabledBindingMCPID := uuid.New()

	_, err = conn.Exec(ctx, `
		INSERT INTO tenants (id, slug, name, status)
		VALUES ($1, 'default', '默认租户', 'active')
		ON CONFLICT (id) DO NOTHING
	`, tenantID)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `
		INSERT INTO tenant_teams (id, tenant_id, slug, name, status, constitution)
		VALUES ($2, $1, 'default', '默认团队', 'active', '{"hard_rules":["r1"]}'::jsonb)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			constitution = EXCLUDED.constitution
	`, tenantID, teamID)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `
		INSERT INTO skills (
			id, tenant_id, slug, name, description,
			archive_object_ref, archive_filename, archive_size_bytes, archive_checksum_sha256, archive_file_count
		) VALUES
			($2, $1, 'skill-a', 'skill-a', '', 's3://skills/skill-a', 'skill-a.zip', 1, repeat('a', 64), 1),
			($3, $1, 'skill-b', 'skill-b', '', 's3://skills/skill-b', 'skill-b.zip', 1, repeat('b', 64), 1)
	`, tenantID, skillAID, skillBID)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `
		INSERT INTO team_skill_bindings (tenant_id, skill_id, team_id)
		VALUES ($1, $2, $4), ($1, $3, $4)
	`, tenantID, skillAID, skillBID, teamID)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `
		INSERT INTO mcp_servers (
			id, tenant_id, name, server_key, description, transport, url, auth_strategy,
			required_env_vars, optional_env_vars, provider_visibility, tool_allowlist, risk_level, metadata, status
		) VALUES (
			$2, $1, 'Postgres Readonly', 'postgres-readonly', '', 'http', 'https://mcp.local/postgres', 'none',
			ARRAY[]::text[], ARRAY[]::text[], '{"codex":true}'::jsonb, ARRAY[]::text[], 'medium', '{}'::jsonb, 'active'
		), (
			$3, $1, 'Disabled Server', 'disabled-server', '', 'http', 'https://mcp.local/disabled-server', 'none',
			ARRAY[]::text[], ARRAY[]::text[], '{"codex":true}'::jsonb, ARRAY[]::text[], 'medium', '{}'::jsonb, 'disabled'
		), (
			$4, $1, 'Disabled Binding', 'disabled-binding', '', 'http', 'https://mcp.local/disabled-binding', 'none',
			ARRAY[]::text[], ARRAY[]::text[], '{"codex":true}'::jsonb, ARRAY[]::text[], 'medium', '{}'::jsonb, 'active'
		)
	`, tenantID, activeMCPID, disabledServerMCPID, disabledBindingMCPID)
	require.NoError(t, err)

	_, err = conn.Exec(ctx, `
		INSERT INTO team_mcp_bindings (tenant_id, team_id, mcp_server_id, metadata, status)
		VALUES
			($1, $2, $3, '{}'::jsonb, 'active'),
			($1, $2, $4, '{}'::jsonb, 'active'),
			($1, $2, $5, '{}'::jsonb, 'disabled')
	`, tenantID, teamID, activeMCPID, disabledServerMCPID, disabledBindingMCPID)
	require.NoError(t, err)

	repo := NewPgRepository(queries.New(conn), conn)
	got, err := repo.GetTeamBaseline(ctx, tenantID, teamID)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"skill-a", "skill-b"}, got.Skills)
	require.ElementsMatch(t, []string{"postgres-readonly"}, got.MCPServers)
	require.Equal(t, []any{"r1"}, got.Constitution["hard_rules"])
}

type employeeRepoIntegrationConfig struct {
	databaseURL string
}

func employeeRepoIntegrationTestConfig() (employeeRepoIntegrationConfig, bool) {
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if employeeRunRepositoryEnvBool("ALLOW_DATABASE_URL_FOR_QUERY_TESTS") && databaseURL == "" {
		databaseURL = strings.TrimSpace(os.Getenv("DATABASE_URL"))
	}
	if databaseURL == "" {
		return employeeRepoIntegrationConfig{}, false
	}
	return employeeRepoIntegrationConfig{databaseURL: databaseURL}, true
}

func runEmployeeRepoTestMigrations(ctx context.Context, conn *pgx.Conn) error {
	files, err := filepath.Glob(filepath.Join("..", "storage", "migrations", "*.sql"))
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, string(content)); err != nil {
			return err
		}
	}
	return nil
}

func TestOverviewInt32PtrFromJSONString(t *testing.T) {
	require.Nil(t, int32PtrFromJSONString(""))
	require.Nil(t, int32PtrFromJSONString("not-a-number"))
	require.Nil(t, int32PtrFromJSONString("2147483648"))

	value := int32PtrFromJSONString("1600")
	require.NotNil(t, value)
	require.Equal(t, int32(1600), *value)
}

func TestOverviewExecutionStatus(t *testing.T) {
	require.Equal(t, OverviewExecutionStatusMissing, overviewExecutionStatus(""))
	require.Equal(t, OverviewExecutionStatusReady, overviewExecutionStatus("ready"))
}

func TestSchedulingCapabilityEnvFactsIncludesCredentialEnvVar(t *testing.T) {
	configuredCount, missingNames := schedulingCapabilityEnvFacts([]queries.ListEffectiveMCPBindingsV2ForEmployeeRow{
		{
			RequiredEnvVars:  []string{" EXISTING_ENV ", "MISSING_REQUIRED", "DUPLICATE_MISSING"},
			CredentialEnvVar: pgtype.Text{String: " BEARER_TOKEN ", Valid: true},
		},
		{
			RequiredEnvVars:  []string{"", "DUPLICATE_MISSING"},
			CredentialEnvVar: pgtype.Text{String: "DUPLICATE_MISSING", Valid: true},
		},
		{
			CredentialEnvVar: pgtype.Text{String: "IGNORED_INVALID", Valid: false},
		},
	}, []string{"EXISTING_ENV", "  CONFIGURED_ONLY "})

	require.Equal(t, 2, configuredCount)
	require.Equal(t, []string{"BEARER_TOKEN", "DUPLICATE_MISSING", "MISSING_REQUIRED"}, missingNames)
}

func TestOverviewRunStatus(t *testing.T) {
	require.Equal(t, OverviewRunStatusNone, overviewRunStatus(""))
	require.Equal(t, OverviewRunStatusRunning, overviewRunStatus("running"))
}

func TestOverviewBudgetSource(t *testing.T) {
	require.Equal(t, "unavailable", overviewBudgetSource(0, 0))
	require.Equal(t, "run_usage_projection", overviewBudgetSource(3, 1600))
}

func TestOverviewSummarySQLCountsStaleConfigQueue(t *testing.T) {
	normalizedSQL := normalizeSQL(queries.GetDigitalEmployeeOverviewSummary)

	require.NotContains(t, normalizedSQL, "0::integer AS stale_config_count")
	require.Contains(t, normalizedSQL, "governance_status IN ('missing', 'pending_approval', 'stale') ))::integer AS stale_config_count")
}

func TestOverviewSummarySQLSeparatesRuntimeBindingAndConfigQueues(t *testing.T) {
	normalizedSQL := normalizeSQL(queries.GetDigitalEmployeeOverviewSummary)
	runtimeBindingExpr := summaryAggregateExpression(t, normalizedSQL, "pending_runtime_binding_count")
	configExpr := summaryAggregateExpression(t, normalizedSQL, "pending_config_approval_count")
	staleConfigExpr := summaryAggregateExpression(t, normalizedSQL, "stale_config_count")

	require.Contains(t, runtimeBindingExpr, "execution_status IN ('missing', 'provisioning')")
	require.Contains(t, runtimeBindingExpr, "runtime_node_id IS NULL")
	require.Contains(t, runtimeBindingExpr, "provider_type = ''")
	require.Contains(t, runtimeBindingExpr, "agent_home_dir_available = false")
	require.NotContains(t, runtimeBindingExpr, "governance_status")
	require.Contains(t, configExpr, "governance_status IN ('missing', 'pending_approval', 'stale')")
	require.Contains(t, staleConfigExpr, "governance_status IN ('missing', 'pending_approval', 'stale')")
}

func TestOverviewSummarySQLRequiresDispatchReadyAgentHome(t *testing.T) {
	normalizedSQL := normalizeSQL(queries.GetDigitalEmployeeOverviewSummary)

	require.Contains(t, normalizedSQL, "(NULLIF(BTRIM(COALESCE(dei.agent_home_dir, '')), '') IS NOT NULL)::boolean AS agent_home_dir_available")
	require.Contains(t, normalizedSQL, "agent_home_dir_available = true")
}

func TestOverviewItemsSQLCarriesProviderAvailabilityForWorkbenchStatus(t *testing.T) {
	normalizedSQL := normalizeSQL(queries.ListDigitalEmployeeOverviewItems)

	require.Contains(t, normalizedSQL, "COALESCE(pc.available, false)::boolean AS provider_available")
}

func TestOverviewItemsSQLUsesLatestGovernanceStatusForCard(t *testing.T) {
	normalizedSQL := normalizeSQL(queries.ListDigitalEmployeeOverviewItems)

	require.Contains(t, normalizedSQL, "employee_config_states AS")
	require.Contains(t, normalizedSQL, "COALESCE(ecs.governance_status, 'missing')::text AS governance_status")
}

func TestOverviewItemsSQLExcludesDeletedTaskEvents(t *testing.T) {
	normalizedSQL := normalizeSQL(queries.ListDigitalEmployeeOverviewItems)

	require.Contains(t, normalizedSQL, "JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id AND t.deleted_at IS NULL")
}

func TestEmployeeOverviewSQLCarriesOperationalStatusFacts(t *testing.T) {
	for name, sql := range map[string]string{
		"items":             queries.ListDigitalEmployeeOverviewItems,
		"operational_facts": queries.ListDigitalEmployeeOverviewOperationalFacts,
	} {
		t.Run(name, func(t *testing.T) {
			assertEmployeeOverviewOperationalFactsSQL(t, sql)
		})
	}
}

func TestOverviewOperationalFactsSQL(t *testing.T) {
	normalizedSQL := normalizeSQL(queries.ListDigitalEmployeeOverviewOperationalFacts)

	for _, expected := range []string{
		"AS q",
		"AS team_id",
		"AS status",
		"AS employee_type",
		"AS provider_type",
		"AS runtime_node_id",
		"AS risk_level",
		"AS execution_status",
		"AS run_status",
		"args.q IS NULL",
		"args.team_id IS NULL",
		"args.status IS NULL",
		"args.employee_type IS NULL",
		"args.provider_type IS NULL",
		"args.runtime_node_id IS NULL",
		"args.risk_level IS NULL",
		"args.execution_status IS NULL",
		"args.run_status IS NULL",
	} {
		require.Contains(t, normalizedSQL, expected)
	}

	upperSQL := strings.ToUpper(normalizedSQL)
	require.NotContains(t, upperSQL, " LIMIT ")
	require.NotContains(t, upperSQL, " OFFSET ")
}

func TestOverviewFiltersFromQueryMapsStableLabels(t *testing.T) {
	filters := overviewFiltersFromQuery([]queries.ListDigitalEmployeeOverviewFilterOptionsRow{
		{FilterType: "status", Value: "active", Label: "active"},
		{FilterType: "risk_level", Value: "medium", Label: "medium"},
		{FilterType: "execution_status", Value: "missing", Label: "missing"},
		{FilterType: "run_status", Value: "none", Label: "none"},
		{FilterType: "provider", Value: "codex", Label: "codex"},
		{FilterType: "provider", Value: "custom-provider", Label: "custom-provider"},
	})

	require.Equal(t, []OverviewFilterOption{{Value: "active", Label: "活跃中"}}, filters.Statuses)
	require.Equal(t, []OverviewFilterOption{{Value: "medium", Label: "中风险"}}, filters.RiskLevels)
	require.Equal(t, []OverviewFilterOption{{Value: "missing", Label: "未绑定 Runtime"}}, filters.ExecutionStatuses)
	require.Equal(t, []OverviewFilterOption{{Value: "none", Label: "暂无运行"}}, filters.RunStatuses)
	require.Equal(t, []OverviewFilterOption{
		{Value: "codex", Label: "Codex"},
		{Value: "custom-provider", Label: "custom-provider"},
	}, filters.Providers)
}

func TestOverviewItemFromQueryHandlesMissingExecutionInstance(t *testing.T) {
	row := baseOverviewItemRow()
	row.ExecutionInstanceID = uuid.NullUUID{}
	row.ExecutionStatus = "missing"
	row.RuntimeNodeID = uuid.NullUUID{}

	item := overviewItemFromQuery(row)

	require.Nil(t, item.ExecutionSummary.ExecutionInstanceID)
	require.Equal(t, OverviewExecutionStatusMissing, item.ExecutionSummary.Status)
}

func TestOverviewItemFromQueryHandlesMissingLatestRun(t *testing.T) {
	row := baseOverviewItemRow()
	row.LatestRunID = uuid.NullUUID{}
	row.LatestRunStatus = "none"

	item := overviewItemFromQuery(row)

	require.Nil(t, item.LatestRunSummary)
}

func TestOverviewItemFromQueryHandlesMissingEffectiveConfig(t *testing.T) {
	row := baseOverviewItemRow()
	row.EffectiveConfigID = uuid.NullUUID{}
	row.GovernanceStatus = "missing"

	item := overviewItemFromQuery(row)

	require.Nil(t, item.GovernanceSummary.EffectiveConfigID)
	require.Equal(t, "missing", item.GovernanceSummary.Status)
}

func TestOverviewItemFromQueryTreatsMissingAgentHomeAsPendingBinding(t *testing.T) {
	row := baseOverviewItemRow()
	row.AgentHomeDirAvailable = false

	item := overviewItemFromQuery(row)

	require.Equal(t, WorkbenchStatusPendingBinding, item.WorkbenchStatus)
}

func TestOverviewItemFromQueryTreatsUnavailableProviderAsError(t *testing.T) {
	row := baseOverviewItemRow()
	row.ProviderAvailable = false

	item := overviewItemFromQuery(row)

	require.Equal(t, WorkbenchStatusError, item.WorkbenchStatus)
}

func TestOverviewItemFromQueryTreatsDisabledExecutionAsError(t *testing.T) {
	row := baseOverviewItemRow()
	row.ExecutionStatus = "disabled"

	item := overviewItemFromQuery(row)

	require.Equal(t, WorkbenchStatusError, item.WorkbenchStatus)
}

func TestOverviewItemFromQueryTreatsPendingGovernanceAsPendingBinding(t *testing.T) {
	row := baseOverviewItemRow()
	row.GovernanceStatus = "pending_approval"

	item := overviewItemFromQuery(row)

	require.Equal(t, "pending_approval", item.GovernanceSummary.Status)
	require.Equal(t, WorkbenchStatusPendingBinding, item.WorkbenchStatus)
}

func TestOverviewItemFromQueryMapsWorkbenchBudgetAndEvents(t *testing.T) {
	row := baseOverviewItemRow()

	item := overviewItemFromQuery(row)

	require.Equal(t, WorkbenchStatusReady, item.WorkbenchStatus)
	require.NotNil(t, item.BudgetSummary.DailyTokenLimit)
	require.Equal(t, int32(10000), *item.BudgetSummary.DailyTokenLimit)
	require.Equal(t, int32(2500), item.BudgetSummary.UsageTokensToday)
	require.NotNil(t, item.BudgetSummary.UsagePercentToday)
	require.Equal(t, int32(25), *item.BudgetSummary.UsagePercentToday)
	require.False(t, item.BudgetSummary.LimitExceeded)
	require.Len(t, item.RecentEvents, 2)
	require.Equal(t, "命令已下发", item.RecentEvents[0].Label)
	require.Equal(t, "running", item.RecentEvents[0].Status)
	require.NotNil(t, item.RecentEvents[0].OccurredAt)
}

func TestOverviewItemFromQueryMapsOperationalState(t *testing.T) {
	row := baseOverviewItemRow()
	row.OperationalHasEmployeeScopedHumanBlocker = true
	row.OperationalHasProjectAcceptanceBlocker = true
	row.OperationalHasQueuedWork = true

	item := overviewItemFromQuery(row)

	require.Equal(t, WorkbenchStatusReady, item.WorkbenchStatus)
	assertDigitalEmployeeOperationalState(t, item.OperationalState, DigitalEmployeeOperationalStatusWaitingHuman, false, []DigitalEmployeeOperationalReason{
		{Code: "approval_blocked", Message: "等待人工确认后继续执行"},
	})
}

func TestOverviewItemFromQueryDoesNotMapProjectAcceptanceToEmployeeWaiting(t *testing.T) {
	row := baseOverviewItemRow()
	row.OperationalHasProjectAcceptanceBlocker = true

	item := overviewItemFromQuery(row)

	assertDigitalEmployeeOperationalState(t, item.OperationalState, DigitalEmployeeOperationalStatusIdle, true, []DigitalEmployeeOperationalReason{})
}

func TestOverviewItemFromQueryMapsRunStatusFacts(t *testing.T) {
	tests := []struct {
		name       string
		runStatus  string
		wantStatus DigitalEmployeeOperationalStatus
	}{
		{name: "dispatching latest run is queued", runStatus: string(OverviewRunStatusDispatching), wantStatus: DigitalEmployeeOperationalStatusQueued},
		{name: "cancelling latest run is working", runStatus: string(OverviewRunStatusCancelling), wantStatus: DigitalEmployeeOperationalStatusWorking},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := baseOverviewItemRow()
			row.LatestRunStatus = tt.runStatus

			item := overviewItemFromQuery(row)

			assertDigitalEmployeeOperationalState(t, item.OperationalState, tt.wantStatus, true, []DigitalEmployeeOperationalReason{})
		})
	}
}

func TestOverviewItemFromQueryMapsLatestProviderFailure(t *testing.T) {
	row := baseOverviewItemRow()
	row.LatestRunStatus = string(OverviewRunStatusFailed)
	row.LatestRunErrorFamily = "provider_timeout"

	item := overviewItemFromQuery(row)

	assertDigitalEmployeeOperationalState(t, item.OperationalState, DigitalEmployeeOperationalStatusError, false, []DigitalEmployeeOperationalReason{
		{Code: "provider_failure", Message: "Provider 执行失败或不可用"},
	})
}

func TestOverviewItemFromQueryMapsLatestTaskFailure(t *testing.T) {
	tests := []struct {
		name        string
		errorFamily string
	}{
		{name: "empty error family"},
		{name: "other error family", errorFamily: "task_contract"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := baseOverviewItemRow()
			row.LatestRunStatus = string(OverviewRunStatusFailed)
			row.LatestRunErrorFamily = tt.errorFamily

			item := overviewItemFromQuery(row)

			assertDigitalEmployeeOperationalState(t, item.OperationalState, DigitalEmployeeOperationalStatusError, false, []DigitalEmployeeOperationalReason{
				{Code: "task_failed", Message: "任务失败，等待恢复策略或后续处理"},
			})
		})
	}
}

func TestOverviewItemFromQueryMapsConfigurationBeforeUnavailable(t *testing.T) {
	row := baseOverviewItemRow()
	row.ExecutionInstanceID = uuid.NullUUID{}
	row.ExecutionStatus = string(OverviewExecutionStatusMissing)
	row.RuntimeNodeID = uuid.NullUUID{}
	row.NodeID = ""
	row.ProviderType = ""
	row.RuntimeStatus = "offline"
	row.AgentHomeDirAvailable = false

	item := overviewItemFromQuery(row)

	require.Equal(t, WorkbenchStatusPendingBinding, item.WorkbenchStatus)
	assertDigitalEmployeeOperationalState(t, item.OperationalState, DigitalEmployeeOperationalStatusNeedsConfiguration, false, []DigitalEmployeeOperationalReason{
		{Code: "configuration_missing", Message: "缺少执行所需配置"},
	})
}

func TestOverviewOperationalStatusCountsFromStatesInitializesMap(t *testing.T) {
	counts := overviewOperationalStatusCountsFromStates([]DigitalEmployeeOperationalState{
		{Status: DigitalEmployeeOperationalStatusIdle},
		{Status: DigitalEmployeeOperationalStatusIdle},
		{Status: DigitalEmployeeOperationalStatusError},
	})

	require.Equal(t, map[DigitalEmployeeOperationalStatus]int32{
		DigitalEmployeeOperationalStatusIdle:  2,
		DigitalEmployeeOperationalStatusError: 1,
	}, counts)

	emptyCounts := overviewOperationalStatusCountsFromStates(nil)
	require.NotNil(t, emptyCounts)
	require.Empty(t, emptyCounts)
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func assertEmployeeOverviewOperationalFactsSQL(t *testing.T, sql string) {
	t.Helper()

	normalizedSQL := normalizeSQL(sql)
	terminalGuard := "pt.requires_human_approval AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')"
	decisionTypeWhereAllowlist := "AND pdr.decision_type IN ('task_failure_recovery', 'route_review', 'project_acceptance')"
	joinStatusNarrowing := "AND ( pt.status IN ('pending', 'planned', 'queued', 'blocked', 'running', 'in_progress', 'waiting_human', 'pending_review', 'failed') OR ( pt.requires_human_approval AND pt.status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed') ) )"
	queuedTaskStatusNarrowing := "count(pt.id) FILTER (WHERE pt.status IN ('queued')) > 0 AS operational_has_queued_work"
	queuedTaskStatusBroadening := "count(pt.id) FILTER (WHERE pt.status IN ('pending', 'planned', 'queued', 'blocked', 'assigned')) > 0 AS operational_has_queued_work"

	for _, expected := range []string{
		"employee_operational_facts",
		"pending_employee_decisions",
		"project_acceptance",
		"latest_run_error_family",
		"latest_run_error_code",
		"operational_has_employee_scoped_human_blocker",
		"operational_has_project_acceptance_blocker",
		"task_failure_recovery",
		"route_review",
		"project_task_attempts pta",
		"pta.id = pt.current_attempt_id",
		terminalGuard,
		decisionTypeWhereAllowlist,
		joinStatusNarrowing,
		queuedTaskStatusNarrowing,
		"completed",
		"done",
		"success",
		"cancelled",
		"failed",
	} {
		require.Contains(t, normalizedSQL, expected)
	}
	require.NotContains(t, normalizedSQL, queuedTaskStatusBroadening)
	require.NotContains(t, normalizedSQL, "pt.status IN ('planned', 'assigned')")
	require.NotContains(t, sql, "<> 'project_acceptance'")
	require.Equal(t, 3, strings.Count(normalizedSQL, terminalGuard))
}

func summaryAggregateExpression(t *testing.T, normalizedSQL, alias string) string {
	t.Helper()
	aliasMarker := " AS " + alias
	aliasIndex := strings.Index(normalizedSQL, aliasMarker)
	require.NotEqual(t, -1, aliasIndex, "missing summary aggregate %s", alias)
	startIndex := strings.LastIndex(normalizedSQL[:aliasIndex], "(COUNT(*) FILTER")
	require.NotEqual(t, -1, startIndex, "missing count filter for %s", alias)
	return normalizedSQL[startIndex : aliasIndex+len(aliasMarker)]
}

func baseOverviewItemRow() queries.ListDigitalEmployeeOverviewItemsRow {
	tenantID := uuid.New()
	teamID := uuid.New()
	ownerUserID := uuid.New()
	employeeID := uuid.New()
	executionInstanceID := uuid.New()
	runtimeNodeID := uuid.New()
	runID := uuid.New()
	taskID := uuid.New()
	effectiveConfigID := uuid.New()

	return queries.ListDigitalEmployeeOverviewItemsRow{
		ID:                                       employeeID,
		TenantID:                                 tenantID,
		TeamID:                                   uuid.NullUUID{UUID: teamID, Valid: true},
		TeamName:                                 "平台团队",
		OwnerUserID:                              ownerUserID,
		OwnerDisplayName:                         "Owner",
		EmployeeType:                             "backend_engineer",
		Name:                                     "后端执行员",
		Role:                                     "backend_engineer",
		Description:                              pgtype.Text{String: "负责后端任务", Valid: true},
		Status:                                   "active",
		RiskLevel:                                "medium",
		ExecutionInstanceID:                      uuid.NullUUID{UUID: executionInstanceID, Valid: true},
		ExecutionStatus:                          "ready",
		RuntimeNodeID:                            uuid.NullUUID{UUID: runtimeNodeID, Valid: true},
		NodeID:                                   "runtime-1",
		RuntimeName:                              "Runtime 1",
		RuntimeStatus:                            "online",
		ProviderType:                             "codex",
		ProviderAvailable:                        true,
		ProviderStatus:                           "healthy",
		HealthStatus:                             "healthy",
		AgentHomeDirAvailable:                    true,
		LatestRunID:                              uuid.NullUUID{UUID: runID, Valid: true},
		LatestRunTaskID:                          uuid.NullUUID{UUID: taskID, Valid: true},
		LatestRunStatus:                          "completed",
		LatestRunTitle:                           "执行任务",
		LatestRunStartedAt:                       pgtype.Timestamptz{},
		LatestRunFinishedAt:                      pgtype.Timestamptz{},
		LatestRunUpdatedAt:                       pgtype.Timestamptz{},
		LatestRunDurationSec:                     "15",
		LatestRunTokenUsage:                      "1600",
		LatestRunErrorFamily:                     "",
		LatestRunErrorCode:                       "",
		EffectiveConfigID:                        uuid.NullUUID{UUID: effectiveConfigID, Valid: true},
		GovernanceStatus:                         "approved",
		DailyTokenLimitText:                      "10000",
		TeamRevisionNumber:                       pgtype.Int4{Int32: 2, Valid: true},
		EmployeeRevisionNumber:                   pgtype.Int4{Int32: 3, Valid: true},
		SkillsCount:                              4,
		McpServersCount:                          2,
		ConstitutionRef:                          "constitution://team/backend",
		TodayBudgetUsageTokens:                   2500,
		BudgetUsageTokens30d:                     pgtype.Int4{Int32: 1600, Valid: true},
		BudgetRunCount30d:                        3,
		OperationalHasEmployeeScopedHumanBlocker: false,
		OperationalHasProjectAcceptanceBlocker:   false,
		OperationalHasQueuedWork:                 false,
		OperationalHasWorkingTask:                false,
		OperationalHasActiveWork:                 false,
		OperationalHasTaskFailure:                false,
		RecentEventsJson:                         []byte(`[{"label":"命令已下发","status":"running","occurred_at":"2026-06-08T01:00:00Z"},{"label":"Provider 输出中","status":"running","occurred_at":"2026-06-08T00:59:00Z"}]`),
	}
}
