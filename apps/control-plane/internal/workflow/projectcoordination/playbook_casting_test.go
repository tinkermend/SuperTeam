package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDeepestExitDeliverableWithCasting(t *testing.T) {
	dev := uuid.MustParse("0be393bb-9dfd-48c8-b010-4b5abb114f23")
	rev := uuid.MustParse("7a16f593-9a99-490e-bcab-77bb8b326afa")
	spec := map[string]any{
		"spec_version": 2,
		"roles": []any{
			map[string]any{"key": "developer", "title": "开发", "required_capabilities": []any{"code_implementation"}},
			map[string]any{"key": "reviewer", "title": "审查", "required_capabilities": []any{"code_review"}},
			map[string]any{"key": "tester", "title": "测试", "required_capabilities": []any{"test_execution"}},
		},
		"skeleton": []any{
			map[string]any{"step": "develop", "role": "developer", "produces_defaults": []any{map[string]any{"name": "branch_ref", "kind": "branch_ref"}, map[string]any{"name": "head_commit", "kind": "git_commit"}}},
			map[string]any{"step": "review", "role": "reviewer", "depends_on": []any{"develop"}, "produces_defaults": []any{map[string]any{"name": "review_verdict", "kind": "conclusion"}}, "required_inputs_defaults": []any{"head_commit"}},
			map[string]any{"step": "test", "role": "tester", "depends_on": []any{"develop"}, "produces_defaults": []any{map[string]any{"name": "test_report", "kind": "conclusion"}}, "required_inputs_defaults": []any{"branch_ref"}},
			map[string]any{"step": "release", "role": "developer", "depends_on": []any{"review", "test"}, "produces_defaults": []any{map[string]any{"name": "release_record", "kind": "evidence_ref"}}, "required_inputs_defaults": []any{"review_verdict", "test_report"}},
		},
		"exits": []any{
			map[string]any{"deliverable": "branch_ref", "label": "分支"},
			map[string]any{"deliverable": "review_verdict", "label": "审查"},
			map[string]any{"deliverable": "release_record", "label": "发布"},
		},
		"constraints": []any{},
	}
	// developer only → branch_ref
	snap := CoordinationSnapshot{
		ScenarioTemplate: &ScenarioTemplateSnapshot{Key: "software_delivery", Version: 1, Spec: spec},
		PlaybookCasting:  []PlaybookCastingAssignment{{RoleKey: "developer", DigitalEmployeeID: dev}},
	}
	require.Equal(t, "branch_ref", DeepestExitDeliverableWithCasting(snap))
	// + reviewer → review_verdict
	snap.PlaybookCasting = append(snap.PlaybookCasting, PlaybookCastingAssignment{RoleKey: "reviewer", DigitalEmployeeID: rev})
	require.Equal(t, "review_verdict", DeepestExitDeliverableWithCasting(snap))
}

func TestApplyPlaybookCasting_OverridesPlannerSelection(t *testing.T) {
	dev := uuid.MustParse("0be393bb-9dfd-48c8-b010-4b5abb114f23")
	other := uuid.MustParse("7a16f593-9a99-490e-bcab-77bb8b326afa")
	plan := RouteDecisionPlan{
		ExitDeliverable: "verification_result",
		Tasks: []PlannedTask{
			{
				Key:                "diagnose",
				SelectedEmployeeID: other,
				Produces:           []string{"root_cause"},
			},
			{
				Key:                "fix",
				SelectedEmployeeID: other,
				Produces:           []string{"fix_record"},
			},
			{
				Key:                "verify",
				SelectedEmployeeID: other,
				Produces:           []string{"verification_result"},
			},
		},
	}
	// Minimal incident-like skeleton in ScenarioTemplate.Spec as map (v2)
	spec := map[string]any{
		"spec_version": 2,
		"roles": []any{
			map[string]any{"key": "diagnostician", "title": "诊断", "required_capabilities": []any{"incident_triage"}},
			map[string]any{"key": "operator", "title": "处置", "required_capabilities": []any{"incident_triage"}},
			map[string]any{"key": "verifier", "title": "验证", "required_capabilities": []any{"incident_triage"}},
		},
		"skeleton": []any{
			map[string]any{"step": "diagnose", "role": "diagnostician", "produces_defaults": []any{map[string]any{"name": "root_cause", "kind": "conclusion"}}},
			map[string]any{"step": "fix", "role": "operator", "depends_on": []any{"diagnose"}, "produces_defaults": []any{map[string]any{"name": "fix_record", "kind": "evidence_ref"}}, "required_inputs_defaults": []any{"root_cause"}},
			map[string]any{"step": "verify", "role": "verifier", "depends_on": []any{"fix"}, "produces_defaults": []any{map[string]any{"name": "verification_result", "kind": "conclusion"}}, "required_inputs_defaults": []any{"fix_record"}},
		},
		"exits": []any{
			map[string]any{"deliverable": "root_cause", "label": "仅诊断"},
			map[string]any{"deliverable": "fix_record", "label": "修复"},
			map[string]any{"deliverable": "verification_result", "label": "验证"},
		},
		"constraints": []any{
			map[string]any{
				"kind":  "role_independence",
				"roles": []any{"verifier", "operator"},
				"when":  map[string]any{"exit_at_or_beyond": "verification_result"},
			},
		},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate: &ScenarioTemplateSnapshot{Key: "incident_response", Version: 1, Spec: spec},
		PlaybookCasting: []PlaybookCastingAssignment{
			{RoleKey: "diagnostician", DigitalEmployeeID: other},
			{RoleKey: "operator", DigitalEmployeeID: dev},
			{RoleKey: "verifier", DigitalEmployeeID: dev},
		},
	}
	// Align task keys with step names via produces matching produces_defaults names
	// stepTask matches produces_defaults name to task.Produces
	ApplyPlaybookCasting(snapshot, &plan)
	require.Equal(t, other, plan.Tasks[0].SelectedEmployeeID)
	require.Equal(t, dev, plan.Tasks[1].SelectedEmployeeID)
	require.Equal(t, dev, plan.Tasks[2].SelectedEmployeeID)

	// Governance must reject same operator+verifier on deep exit. Cast locks both
	// roles to the same employee → structural gap (ErrNoSuitableEmployee), not a
	// replan-able invalidRouteDecision (G12: replan cannot undo 编制).
	snapshot.DigitalEmployeePool = []ProjectMemberSnapshot{
		{PrincipalID: dev, ProjectRole: "executor", Status: "active"},
		{PrincipalID: other, ProjectRole: "executor", Status: "active"},
	}
	err := EnforceScenarioTemplateGovernance(snapshot, &plan)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoSuitableEmployee)
	require.Contains(t, err.Error(), "剧本编制")
	var gap *structuralGapError
	require.ErrorAs(t, err, &gap)
	require.Equal(t, "role_independence", gap.gap.ConstraintKind)
	require.ElementsMatch(t, []string{"verifier", "operator"}, gap.gap.Roles)
}
