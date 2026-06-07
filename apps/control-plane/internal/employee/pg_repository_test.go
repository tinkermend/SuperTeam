package employee

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestOverviewInt32FromJSONString(t *testing.T) {
	require.Equal(t, int32(1600), int32FromJSONString("1600"))
	require.Equal(t, int32(0), int32FromJSONString(""))
	require.Equal(t, int32(0), int32FromJSONString("not-a-number"))
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

func TestOverviewRunStatus(t *testing.T) {
	require.Equal(t, OverviewRunStatusNone, overviewRunStatus(""))
	require.Equal(t, OverviewRunStatusRunning, overviewRunStatus("running"))
}

func TestOverviewBudgetSource(t *testing.T) {
	require.Equal(t, "unavailable", overviewBudgetSource(0, 0))
	require.Equal(t, "run_usage_projection", overviewBudgetSource(3, 1600))
}

func TestOverviewSummarySQLCountsStaleConfigQueue(t *testing.T) {
	normalizedSQL := strings.Join(strings.Fields(queries.GetDigitalEmployeeOverviewSummary), " ")

	require.NotContains(t, normalizedSQL, "0::integer AS stale_config_count")
	require.Contains(t, normalizedSQL, "governance_status IN ('missing', 'pending_approval', 'stale') ))::integer AS stale_config_count")
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
		ID:                     employeeID,
		TenantID:               tenantID,
		TeamID:                 uuid.NullUUID{UUID: teamID, Valid: true},
		TeamName:               "平台团队",
		OwnerUserID:            ownerUserID,
		OwnerDisplayName:       "Owner",
		EmployeeType:           "backend_engineer",
		Name:                   "后端执行员",
		Role:                   "backend_engineer",
		Description:            pgtype.Text{String: "负责后端任务", Valid: true},
		Status:                 "active",
		RiskLevel:              "medium",
		ExecutionInstanceID:    uuid.NullUUID{UUID: executionInstanceID, Valid: true},
		ExecutionStatus:        "ready",
		RuntimeNodeID:          uuid.NullUUID{UUID: runtimeNodeID, Valid: true},
		NodeID:                 "runtime-1",
		RuntimeName:            "Runtime 1",
		RuntimeStatus:          "online",
		ProviderType:           "codex",
		ProviderStatus:         "healthy",
		HealthStatus:           "healthy",
		AgentHomeDirAvailable:  true,
		LatestRunID:            uuid.NullUUID{UUID: runID, Valid: true},
		LatestRunTaskID:        uuid.NullUUID{UUID: taskID, Valid: true},
		LatestRunStatus:        "completed",
		LatestRunTitle:         "执行任务",
		LatestRunStartedAt:     pgtype.Timestamptz{},
		LatestRunFinishedAt:    pgtype.Timestamptz{},
		LatestRunUpdatedAt:     pgtype.Timestamptz{},
		LatestRunDurationSec:   "15",
		LatestRunTokenUsage:    "1600",
		EffectiveConfigID:      uuid.NullUUID{UUID: effectiveConfigID, Valid: true},
		GovernanceStatus:       "approved",
		DailyTokenLimitText:    "10000",
		TeamRevisionNumber:     pgtype.Int4{Int32: 2, Valid: true},
		EmployeeRevisionNumber: pgtype.Int4{Int32: 3, Valid: true},
		SkillsCount:            4,
		McpServersCount:        2,
		ConstitutionRef:        "constitution://team/backend",
		TodayBudgetUsageTokens: 2500,
		BudgetUsageTokens30d:   pgtype.Int4{Int32: 1600, Valid: true},
		BudgetRunCount30d:      3,
		RecentEventsJson:       []byte(`[{"label":"命令已下发","status":"running","occurred_at":"2026-06-08T01:00:00Z"},{"label":"Provider 输出中","status":"running","occurred_at":"2026-06-08T00:59:00Z"}]`),
	}
}
