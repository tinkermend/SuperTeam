package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestAnnotateAndValidateExtraTaskRoles_UnknownDemoted(t *testing.T) {
	t.Parallel()
	emp := uuid.New()
	plan := RouteDecisionPlan{
		ExitDeliverable: "ops_report",
		Tasks: []PlannedTask{
			{
				Key:                "collect",
				Title:              "采集",
				SelectedEmployeeID: emp,
				Produces:           []string{"raw_logs"},
				RoleKey:            "collector", // skeleton-derived via produces
			},
			{
				Key:                "extra_net",
				Title:              "网络核查",
				SelectedEmployeeID: emp,
				Produces:           []string{"net_check"},
				RoleKey:            "network_diagnostics", // not in vocab
			},
		},
	}
	snap := CoordinationSnapshot{
		ScenarioTemplate: &ScenarioTemplateSnapshot{
			Key: "ops_analysis",
			Spec: map[string]any{
				"version": 2,
				"roles": []any{
					map[string]any{"key": "collector", "title": "采集"},
					map[string]any{"key": "analyst", "title": "分析"},
				},
				"skeleton": []any{
					map[string]any{
						"step":  "collect",
						"role":  "collector",
						"produces_defaults": []any{
							map[string]any{"name": "raw_logs"},
						},
					},
				},
				"exits": []any{
					map[string]any{"deliverable": "ops_report", "label": "运维报告"},
				},
			},
		},
		RoleVocabulary: []RoleVocabularyPromptEntry{
			{RoleKey: "collector", Title: "采集"},
			{RoleKey: "analyst", Title: "分析"},
			{RoleKey: "operator", Title: "处置"},
		},
	}
	AnnotateAndValidateExtraTaskRoles(snap, &plan)

	require.Equal(t, "collector", plan.Tasks[0].RoleKey)
	require.Equal(t, "", plan.Tasks[1].RoleKey, "unknown key demoted")
	require.NotNil(t, constraintNoteWithKind(plan.ConstraintNotes, "extra_task_unknown_role"))
}

func TestAnnotateAndValidateExtraTaskRoles_ValidExtraKept(t *testing.T) {
	t.Parallel()
	emp := uuid.New()
	plan := RouteDecisionPlan{
		Tasks: []PlannedTask{
			{
				Key:                "extra_op",
				Title:              "处置",
				SelectedEmployeeID: emp,
				Produces:           []string{"fix"},
				RoleKey:            "operator",
			},
		},
	}
	snap := CoordinationSnapshot{
		RoleVocabulary: []RoleVocabularyPromptEntry{
			{RoleKey: "operator", Title: "处置"},
		},
	}
	AnnotateAndValidateExtraTaskRoles(snap, &plan)
	require.Equal(t, "operator", plan.Tasks[0].RoleKey)
	require.Empty(t, plan.ConstraintNotes)
}

func TestAnnotateAndValidateExtraTaskRoles_MissingRoleNoted(t *testing.T) {
	t.Parallel()
	plan := RouteDecisionPlan{
		Tasks: []PlannedTask{
			{Key: "extra", Title: "额外", Produces: []string{"x"}},
		},
	}
	snap := CoordinationSnapshot{
		RoleVocabulary: []RoleVocabularyPromptEntry{{RoleKey: "developer"}},
	}
	AnnotateAndValidateExtraTaskRoles(snap, &plan)
	require.NotNil(t, constraintNoteWithKind(plan.ConstraintNotes, "extra_task_missing_role"))
}
