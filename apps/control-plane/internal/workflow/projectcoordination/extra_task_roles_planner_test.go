package projectcoordination

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// H10: full OpenAICompatibleRoutePlanner.Plan path — scripted model emits a
// skeleton-matching task plus a beyond-skeleton task with an unregistered
// role_key; server demotes it and records constraint_notes without rejecting.
func TestPlannerExtraTaskUnknownRoleDemotedThroughPlan(t *testing.T) {
	t.Parallel()
	emp := uuid.New()
	// Minimal software_delivery-like template: one skeleton step → release_record exit.
	spec := map[string]any{
		"version": 2,
		"roles": []any{
			map[string]any{"key": "developer", "title": "开发", "required_capabilities": []any{"code_implementation"}},
		},
		"skeleton": []any{
			map[string]any{
				"step": "develop",
				"role": "developer",
				"produces_defaults": []any{
					map[string]any{"name": "release_record", "kind": "artifact"},
				},
			},
		},
		"exits": []any{
			map[string]any{"deliverable": "release_record", "label": "发布记录"},
		},
	}

	// Model returns skeleton task + extra with invented role.
	content := fmt.Sprintf(`{
		"reason":"skeleton plus extra",
		"requires_human_review":false,
		"exit_deliverable":"release_record",
		"template_key":"software_delivery",
		"tasks":[
			{
				"key":"develop",
				"title":"开发",
				"summary":"实现",
				"selected_employee_id":%q,
				"employee_selection_reason":"cast",
				"required_capabilities":["code_implementation"],
				"matched_capabilities":["code_implementation"],
				"missing_capabilities":[],
				"permission_requirements":[],
				"tool_requirements":[],
				"runtime_requirements":[],
				"verification_requirements":["done"],
				"selection_score":80,
				"selection_confidence":0.9,
				"expected_outputs":["release_record"],
				"produces":["release_record"],
				"input_requirements":{},
				"handoff_contract":{},
				"blocked_by_keys":[],
				"risk_level":"medium",
				"task_kind":"feature_development",
				"role_key":"developer"
			},
			{
				"key":"extra_legal",
				"title":"法务审查",
				"summary":"额外任务",
				"selected_employee_id":%q,
				"employee_selection_reason":"guess",
				"required_capabilities":[],
				"matched_capabilities":[],
				"missing_capabilities":[],
				"permission_requirements":[],
				"tool_requirements":[],
				"runtime_requirements":[],
				"verification_requirements":["done"],
				"selection_score":50,
				"selection_confidence":0.5,
				"expected_outputs":["legal_note"],
				"produces":["legal_note"],
				"input_requirements":{"required_inputs":["release_record"]},
				"handoff_contract":{},
				"blocked_by_keys":["develop"],
				"risk_level":"medium",
				"task_kind":"feature_development",
				"role_key":"legal_compliance_invented"
			}
		],
		"budget_estimate":{},
		"planner_metadata":{},
		"plan_acceptance_criteria":[
			{"id":"build_ok","statement":"构建通过","satisfied_by":["develop"],"verification_method":"automated_test","severity":"blocking"},
			{"id":"human_ok","statement":"业务可接受","satisfied_by":[],"verification_method":"human_judgment","severity":"blocking"}
		]
	}`, emp.String(), emp.String())

	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey:      "k",
		BaseURL:     "https://example.invalid",
		Model:       "m",
		MaxAttempts: 1,
	}, fakeChatCompletionClient{content: content})

	snap := CoordinationSnapshot{
		ProjectID: uuid.New(),
		Demand:    DemandSnapshot{ID: uuid.New(), Title: "H10", Content: "test"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestExecutorMember(emp),
		},
		ScenarioTemplate: &ScenarioTemplateSnapshot{
			Key:  "software_delivery",
			Name: "软件交付",
			Spec: spec,
		},
		RoleVocabulary: []RoleVocabularyPromptEntry{
			{RoleKey: "developer", Title: "开发"},
			{RoleKey: "reviewer", Title: "审查"},
		},
		PlaybookCasting: []PlaybookCastingAssignment{
			{RoleKey: "developer", DigitalEmployeeID: emp},
		},
	}

	plan, err := planner.Plan(context.Background(), snap)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(plan.Tasks), 2)

	var develop, extra *PlannedTask
	for i := range plan.Tasks {
		t0 := &plan.Tasks[i]
		switch t0.Key {
		case "develop":
			develop = t0
		case "extra_legal":
			extra = t0
		}
	}
	require.NotNil(t, develop, "skeleton task present")
	require.Equal(t, "developer", develop.RoleKey, "skeleton keeps template role")

	require.NotNil(t, extra, "extra task present")
	require.Equal(t, "", extra.RoleKey, "unknown role demoted")
	require.NotNil(t, constraintNoteWithKind(plan.ConstraintNotes, "extra_task_unknown_role"))
	// Plan must not be rejected solely for unknown extra role.
	require.NotEmpty(t, plan.Tasks)
}

// H10 companion: missing role_key on extra task → constraint note, plan kept.
func TestPlannerExtraTaskMissingRoleNotedThroughPlan(t *testing.T) {
	t.Parallel()
	emp := uuid.New()
	content := fmt.Sprintf(`{
		"reason":"extra only",
		"requires_human_review":false,
		"tasks":[
			{
				"key":"solo",
				"title":"额外",
				"summary":"无角色",
				"selected_employee_id":%q,
				"employee_selection_reason":"only",
				"required_capabilities":[],
				"matched_capabilities":[],
				"missing_capabilities":[],
				"permission_requirements":[],
				"tool_requirements":[],
				"runtime_requirements":[],
				"verification_requirements":["x"],
				"selection_score":10,
				"selection_confidence":0.5,
				"expected_outputs":["out"],
				"produces":["out"],
				"input_requirements":{},
				"handoff_contract":{},
				"blocked_by_keys":[],
				"risk_level":"low",
				"task_kind":"feature_development"
			}
		],
		"budget_estimate":{},
		"template_key":"generic",
		"planner_metadata":{},
		"plan_acceptance_criteria":[
			{"id":"c1","statement":"自动化验证通过","satisfied_by":["solo"],"verification_method":"automated_test","severity":"blocking"},
			{"id":"c2","statement":"业务可接受","satisfied_by":[],"verification_method":"human_judgment","severity":"blocking"}
		]
	}`, emp.String())

	planner := NewOpenAICompatibleRoutePlanner(OpenAICompatiblePlannerConfig{
		APIKey: "k", BaseURL: "https://x", Model: "m", MaxAttempts: 1,
	}, fakeChatCompletionClient{content: content})

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "t", Content: "c"},
		DigitalEmployeePool: []ProjectMemberSnapshot{
			openAITestExecutorMember(emp),
		},
		RoleVocabulary: []RoleVocabularyPromptEntry{{RoleKey: "developer"}},
	})
	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, "", plan.Tasks[0].RoleKey)
	require.NotNil(t, constraintNoteWithKind(plan.ConstraintNotes, "extra_task_missing_role"))
}
