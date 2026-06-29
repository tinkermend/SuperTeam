package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDefaultRequiredCapabilitiesForKnownKinds(t *testing.T) {
	require.Equal(t,
		[]string{"database.read", "sql.analysis", "data.quality.check", "business.metric.interpretation"},
		DefaultRequiredCapabilities("database_analysis"))
	require.Equal(t,
		[]string{"incident.triage", "log.analysis", "metrics.analysis", "code.path.tracing", "runtime.diagnostics"},
		DefaultRequiredCapabilities("incident_triage"))
	require.Equal(t,
		[]string{"codebase.analysis", "implementation", "testing.verification", "artifact.reporting"},
		DefaultRequiredCapabilities("feature_development"))
}

func TestDefaultRequiredCapabilitiesResolvesAliases(t *testing.T) {
	// Casing and whitespace are normalized.
	require.Equal(t, DefaultRequiredCapabilities("database_analysis"), DefaultRequiredCapabilities("  Database_Analysis "))
	// Aliases resolve to the canonical defaults.
	require.Equal(t, DefaultRequiredCapabilities("database_analysis"), DefaultRequiredCapabilities("db_analysis"))
	require.Equal(t, DefaultRequiredCapabilities("incident_triage"), DefaultRequiredCapabilities("incident_investigation"))
	require.Equal(t, DefaultRequiredCapabilities("feature_development"), DefaultRequiredCapabilities("feature_dev"))
}

func TestDefaultRequiredCapabilitiesUnknownKindReturnsNil(t *testing.T) {
	require.Nil(t, DefaultRequiredCapabilities(""))
	require.Nil(t, DefaultRequiredCapabilities("execution"))
	require.Nil(t, DefaultRequiredCapabilities("some_custom_kind"))
}

func TestWorkspaceModeForTaskKind(t *testing.T) {
	tests := []struct {
		name string
		kind string
		want string
	}{
		{name: "feature development uses branch workspace", kind: "feature_development", want: WorkspaceModeBranch},
		{name: "feature uses branch workspace", kind: "feature", want: WorkspaceModeBranch},
		{name: "software uses branch workspace", kind: "software", want: WorkspaceModeBranch},
		{name: "code uses branch workspace", kind: "code", want: WorkspaceModeBranch},
		{name: "bugfix uses branch workspace", kind: "bugfix", want: WorkspaceModeBranch},
		{name: "code review uses diff workspace", kind: "code_review", want: WorkspaceModeDiff},
		{name: "test verification uses detached run workspace", kind: "test_verification", want: WorkspaceModeDetachedRun},
		{name: "test uses detached run workspace", kind: "test", want: WorkspaceModeDetachedRun},
		{name: "build verification uses detached run workspace", kind: "build_verification", want: WorkspaceModeDetachedRun},
		{name: "build uses detached run workspace", kind: "build", want: WorkspaceModeDetachedRun},
		{name: "incident triage uses readonly workspace", kind: "incident_triage", want: WorkspaceModeReadonly},
		{name: "analysis uses readonly workspace", kind: "analysis", want: WorkspaceModeReadonly},
		{name: "incident uses readonly workspace", kind: "incident", want: WorkspaceModeReadonly},
		{name: "database analysis uses readonly workspace", kind: "database analysis", want: WorkspaceModeReadonly},
		{name: "status report uses no workspace", kind: "status_report", want: WorkspaceModeNone},
		{name: "status uses no workspace", kind: "status", want: WorkspaceModeNone},
		{name: "human uses no workspace", kind: "human", want: WorkspaceModeNone},
		{name: "acceptance uses no workspace", kind: "acceptance", want: WorkspaceModeNone},
		{name: "unknown kind uses no workspace", kind: "some_custom_kind", want: WorkspaceModeNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, WorkspaceModeForTaskKind(tt.kind))
		})
	}
}

func TestDefaultRequiredCapabilitiesReturnsCopy(t *testing.T) {
	first := DefaultRequiredCapabilities("database_analysis")
	first[0] = "mutated"
	second := DefaultRequiredCapabilities("database_analysis")
	require.Equal(t, "database.read", second[0], "mutation must not leak across calls")
}

func TestApplyTaskTypeDefaultsFillsMissingCapabilities(t *testing.T) {
	plan := RouteDecisionPlan{
		Reason: "reason",
		Tasks: []PlannedTask{{
			Key:                     "analyze-db",
			TaskKind:                "database_analysis",
			SelectedEmployeeID:      uuid.New(),
			EmployeeSelectionReason: "db employee",
			RequiredCapabilities:    []string{"database.read"},
		}},
	}

	ApplyTaskTypeDefaults(&plan)

	require.Equal(t,
		[]string{"database.read", "sql.analysis", "data.quality.check", "business.metric.interpretation"},
		plan.Tasks[0].RequiredCapabilities)
	require.Equal(t, []string{"analyze-db"}, plan.PlannerMetadata["task_type_defaults_applied"])
}

func TestApplyTaskTypeDefaultsPreservesExplicitCapabilities(t *testing.T) {
	plan := RouteDecisionPlan{
		Reason: "reason",
		Tasks: []PlannedTask{{
			Key:                     "analyze-db",
			TaskKind:                "database_analysis",
			SelectedEmployeeID:      uuid.New(),
			EmployeeSelectionReason: "db employee",
			// Custom capability plus one that overlaps a default — union must dedupe.
			RequiredCapabilities: []string{"custom.cap", "sql.analysis"},
		}},
	}

	ApplyTaskTypeDefaults(&plan)

	require.Equal(t,
		[]string{"custom.cap", "sql.analysis", "database.read", "data.quality.check", "business.metric.interpretation"},
		plan.Tasks[0].RequiredCapabilities)
}

func TestApplyTaskTypeDefaultsSkipsUnknownKind(t *testing.T) {
	plan := RouteDecisionPlan{
		Reason: "reason",
		Tasks: []PlannedTask{{
			Key:                     "do-thing",
			TaskKind:                "custom_kind",
			SelectedEmployeeID:      uuid.New(),
			EmployeeSelectionReason: "custom",
			RequiredCapabilities:    []string{"only.this"},
		}},
	}

	ApplyTaskTypeDefaults(&plan)

	require.Equal(t, []string{"only.this"}, plan.Tasks[0].RequiredCapabilities)
	require.Nil(t, plan.PlannerMetadata["task_type_defaults_applied"])
}

func TestApplyTaskTypeDefaultsNilPlanIsNoop(t *testing.T) {
	require.NotPanics(t, func() { ApplyTaskTypeDefaults(nil) })
}

func TestApplyTaskTypeDefaultsDoesNotRecordWhenAlreadyComplete(t *testing.T) {
	// Task already declares every default — metadata should not record it as "applied".
	plan := RouteDecisionPlan{
		Reason: "reason",
		Tasks: []PlannedTask{{
			Key:                     "analyze-db",
			TaskKind:                "database_analysis",
			SelectedEmployeeID:      uuid.New(),
			EmployeeSelectionReason: "db employee",
			RequiredCapabilities:    DefaultRequiredCapabilities("database_analysis"),
		}},
	}

	ApplyTaskTypeDefaults(&plan)

	require.Nil(t, plan.PlannerMetadata["task_type_defaults_applied"])
	require.Len(t, plan.Tasks[0].RequiredCapabilities, 4)
}
