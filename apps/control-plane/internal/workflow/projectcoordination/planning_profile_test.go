package projectcoordination

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
)

func TestBuildDigitalEmployeePlanningProfileFallsBackToUnknownFacts(t *testing.T) {
	employeeID := uuid.New()
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("数据分析员工"),
		Settings:            map[string]any{"planning_role": "data_analyst"},
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{}, true)

	require.Equal(t, employeeID, profile.DigitalEmployeeID)
	require.Equal(t, "数据分析员工", profile.DisplayName)
	require.Equal(t, "data_analyst", profile.RoleProfile.PrimaryRole)
	require.Equal(t, "unknown", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, "unknown", profile.ProfileFreshness.SourceState)
	require.Contains(t, profile.SelectionWarnings, "profile_source_missing")
	require.Empty(t, profile.HardFailures)
}

func TestBuildDigitalEmployeePlanningProfileTreatsIdentityOnlySourceAsMissing(t *testing.T) {
	employeeID := uuid.New()
	fetchedAt := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("执行员工"),
		Settings:            map[string]any{"planning_role": "executor"},
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{
		DigitalEmployeeID: employeeID,
		FetchedAt:         fetchedAt,
	}, true)

	require.Equal(t, employeeID, profile.DigitalEmployeeID)
	require.Equal(t, "unknown", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, "unknown", profile.ProfileFreshness.SourceState)
	require.Contains(t, profile.SelectionWarnings, "profile_source_missing")
}

func TestBuildDigitalEmployeePlanningProfileTreatsEmptySourceFactsAsMissing(t *testing.T) {
	employeeID := uuid.New()
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("执行员工"),
		Settings:            map[string]any{"planning_role": "executor"},
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{
		DigitalEmployeeID: employeeID,
		EmployeeStatus:    "   ",
		ProviderType:      "   ",
		CapabilitySelection: map[string]any{
			"enabled_skills":                []any{"", "   "},
			"enabled_mcp_servers":           []any{},
			"enabled_external_capabilities": []any{123},
			"enabled_provider_types":        []any{"   "},
		},
	}, true)

	require.Equal(t, "unknown", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, "unknown", profile.ProfileFreshness.SourceState)
	require.Contains(t, profile.SelectionWarnings, "profile_source_missing")
}

func TestBuildDigitalEmployeePlanningProfileUsesSourceFacts(t *testing.T) {
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("执行员工"),
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{
		DigitalEmployeeID: employeeID,
		EmployeeType:      "database_admin",
		Role:              "数据库分析",
		EmployeeStatus:    "active",
		RoleProfile:       map[string]any{"primary_role": "data_analyst", "description": "数据库分析"},
		CapabilitySelection: map[string]any{
			"enabled_skills":                []any{"sql.analysis", "data.quality.check"},
			"enabled_mcp_servers":           []any{"postgres.readonly"},
			"enabled_external_capabilities": []any{"database.read"},
			"enabled_provider_types":        []any{"codex"},
		},
		PermissionPolicy: map[string]any{
			"grants": []any{"database.read:dev_database"},
		},
		ContextPolicy:         map[string]any{"max_context_classification": "internal"},
		RuntimeNodeID:         runtimeNodeID,
		ProviderType:          "codex",
		ExecutionStatus:       "ready",
		EffectiveConfigStatus: "approved",
		LoadState: map[string]any{
			"running_tasks":   2,
			"available_slots": 3,
			"lendable":        true,
		},
		ReliabilitySignals: map[string]any{
			"recent_success_count":      7,
			"recent_failure_count":      1,
			"recent_human_reject_count": 2,
		},
		FetchedAt: now,
	}, true)

	require.Equal(t, "data_analyst", profile.RoleProfile.PrimaryRole)
	require.Equal(t, []PlanningCapability{{Key: "database.read", Level: "strong", Source: "capability_selection.enabled_external_capabilities", Confidence: 0.9}}, profile.Capabilities)
	require.Equal(t, []PlanningSkill{{Key: "sql.analysis", Source: "capability_selection.enabled_skills"}, {Key: "data.quality.check", Source: "capability_selection.enabled_skills"}}, profile.Skills)
	require.Equal(t, []PlanningToolBinding{{Type: "mcp", Key: "postgres.readonly", Status: "available"}}, profile.ToolBindings)
	require.Equal(t, []string{"codex"}, profile.RuntimeRequirements.ProviderTypes)
	require.Equal(t, "ready", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, runtimeNodeID.String(), profile.RuntimeRequirements.RuntimeNodeID)
	require.Equal(t, []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}}, profile.Permissions)
	require.Equal(t, "internal", profile.ContextPolicy.MaxContextClassification)
	require.Equal(t, PlanningLoadState{AvailableSlots: 3, InFlightTasks: 2, Lendable: true}, profile.LoadState)
	require.Equal(t, PlanningReliabilitySignals{RecentSuccessCount: 7, RecentFailureCount: 1, RecentHumanRejectCount: 2}, profile.ReliabilitySignals)
	require.Equal(t, "ready", profile.ProfileFreshness.SourceState)
	require.Equal(t, "approved", profile.ProfileFreshness.SourceVersions["effective_config_status"])
}

func TestBuildDigitalEmployeePlanningProfileKeepsRuntimeStatusUnknownWithoutExecutionFacts(t *testing.T) {
	employeeID := uuid.New()
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("执行员工"),
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{
		DigitalEmployeeID: employeeID,
		EmployeeType:      "database_admin",
		EmployeeStatus:    "active",
		RoleProfile:       map[string]any{"primary_role": "data_analyst"},
	}, true)

	require.Equal(t, "unknown", profile.RuntimeRequirements.ProviderStatus)
}

func TestBuildDigitalEmployeePlanningProfileMarksDispatchNotReadyWithoutHardFailure(t *testing.T) {
	employeeID := uuid.New()
	member := project.ProjectMember{
		PrincipalID:         employeeID,
		ProjectRole:         project.ProjectRoleExecutor,
		Status:              "active",
		DisplayNameSnapshot: strPtr("执行员工"),
	}

	profile := BuildDigitalEmployeePlanningProfile(member, DigitalEmployeePlanningProfileSourceRecord{
		DigitalEmployeeID: employeeID,
		EmployeeType:      "implementation",
		EmployeeStatus:    "active",
		CapabilitySelection: map[string]any{
			"enabled_provider_types": []any{"codex"},
		},
		ProviderType:    "codex",
		ExecutionStatus: "ready",
	}, false)

	require.Equal(t, "ready", profile.RuntimeRequirements.ProviderStatus)
	require.Equal(t, "not_ready", profile.RuntimeRequirements.DispatchReadinessStatus)
	require.Equal(t, []string{"runtime_not_ready"}, profile.RuntimeRequirements.DispatchBlockingReasons)
	require.Empty(t, profile.HardFailures)
}

func TestScorePlanningProfileRecordsHardFailures(t *testing.T) {
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.New(),
		RoleProfile:       PlanningRoleProfile{PrimaryRole: "data_analyst"},
		Capabilities:      []PlanningCapability{{Key: "database.read", Level: "strong", Source: "test", Confidence: 1}, {Key: "sql.analysis", Level: "strong", Source: "test", Confidence: 1}},
		Skills:            []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
		ToolBindings:      []PlanningToolBinding{{Type: "mcp", Key: "postgres.readonly", Status: "available"}},
		RuntimeRequirements: PlanningRuntimeRequirements{
			ProviderTypes:  []string{"codex"},
			ProviderStatus: "ready",
		},
		Permissions: []PlanningPermission{{Scope: "database.read", Resource: "dev_database", Status: "granted"}},
		LoadState:   PlanningLoadState{AvailableSlots: 1, Lendable: true},
	}

	result := ScorePlanningProfile(profile, PlanningTaskRequirements{
		TaskType:               "database_analysis",
		RequiredCapabilities:   []string{"database.read", "sql.analysis"},
		PermissionRequirements: []string{"database.read:dev_database"},
		ToolRequirements:       []string{"mcp:postgres.readonly"},
		RuntimeRequirements:    []string{"provider:codex"},
	})

	require.Equal(t, 100, result.Score)
	require.Equal(t, []string{"database.read", "sql.analysis"}, result.MatchedCapabilities)
	require.Empty(t, result.MissingCapabilities)
	require.Empty(t, result.HardFailures)

	result = ScorePlanningProfile(profile, PlanningTaskRequirements{
		TaskType:             "database_analysis",
		RequiredCapabilities: []string{"database.write"},
	})

	require.Equal(t, []string{"database.write"}, result.MissingCapabilities)
	require.Contains(t, result.HardFailures, "missing_capability:database.write")
	require.Equal(t, 0, result.Score)

	runtimeMismatchProfile := profile
	runtimeMismatchProfile.RuntimeRequirements.ProviderTypes = []string{"claude"}
	result = ScorePlanningProfile(runtimeMismatchProfile, PlanningTaskRequirements{
		TaskType:            "database_analysis",
		RuntimeRequirements: []string{"provider:codex"},
	})

	require.Contains(t, result.HardFailures, "runtime_requirement_unsatisfied")
	require.Equal(t, 0, result.Score)

	runtimeUnknownProfile := profile
	runtimeUnknownProfile.RuntimeRequirements.ProviderStatus = "unknown"
	result = ScorePlanningProfile(runtimeUnknownProfile, PlanningTaskRequirements{
		TaskType:            "database_analysis",
		RuntimeRequirements: []string{"provider:codex"},
	})

	require.Contains(t, result.HardFailures, "runtime_requirement_unsatisfied")
	require.Equal(t, 0, result.Score)

	result = ScorePlanningProfile(runtimeUnknownProfile, PlanningTaskRequirements{
		TaskType: "database_analysis",
	})

	require.Contains(t, result.HardFailures, "runtime_requirement_unsatisfied")
	require.Equal(t, 0, result.Score)

	result = ScorePlanningProfile(profile, PlanningTaskRequirements{
		TaskType:               "database_analysis",
		PermissionRequirements: []string{"database.write:dev_database"},
		ToolRequirements:       []string{"mcp:postgres.admin"},
	})

	require.Contains(t, result.HardFailures, "permission_or_tool_requirement_unsatisfied")
	require.Equal(t, 0, result.Score)

	hardFailedProfile := profile
	hardFailedProfile.HardFailures = []string{"runtime_contract_missing"}
	result = ScorePlanningProfile(hardFailedProfile, PlanningTaskRequirements{
		TaskType:               "database_analysis",
		RequiredCapabilities:   []string{"database.read"},
		PermissionRequirements: []string{"database.read:dev_database"},
		ToolRequirements:       []string{"mcp:postgres.readonly"},
		RuntimeRequirements:    []string{"provider:codex"},
	})

	require.Equal(t, 0, result.Score)
	require.Contains(t, result.HardFailures, "runtime_contract_missing")
}

func TestPlanningProfileSnapshotHashIsStable(t *testing.T) {
	generatedAt := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	profile := DigitalEmployeePlanningProfile{
		DigitalEmployeeID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		DisplayName:       "A",
		RoleProfile:       PlanningRoleProfile{PrimaryRole: "executor"},
		Skills:            []PlanningSkill{{Key: "sql.analysis", Source: "test"}},
		ProfileFreshness:  PlanningProfileFreshness{SourceState: "ready", GeneratedAt: generatedAt},
		GeneratedAt:       generatedAt,
	}
	differentGeneratedAt := profile
	differentGeneratedAt.ProfileFreshness.GeneratedAt = generatedAt.Add(time.Hour)
	differentGeneratedAt.GeneratedAt = generatedAt.Add(2 * time.Hour)

	first := PlanningProfileSnapshotHash(profile)
	second := PlanningProfileSnapshotHash(differentGeneratedAt)

	require.Len(t, first, 64)
	require.Equal(t, first, second)
}
