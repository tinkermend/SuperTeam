package employee

import (
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
		TeamRevisionNumber:     pgtype.Int4{Int32: 2, Valid: true},
		EmployeeRevisionNumber: pgtype.Int4{Int32: 3, Valid: true},
		SkillsCount:            4,
		McpServersCount:        2,
		ConstitutionRef:        "constitution://team/backend",
		BudgetUsageTokens30d:   pgtype.Int4{Int32: 1600, Valid: true},
		BudgetRunCount30d:      3,
	}
}
