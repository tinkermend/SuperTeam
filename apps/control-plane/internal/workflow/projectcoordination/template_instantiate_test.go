package projectcoordination

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
)

func instantiatePool(ids ...uuid.UUID) []ProjectMemberSnapshot {
	pool := make([]ProjectMemberSnapshot, 0, len(ids))
	for _, id := range ids {
		member := ProjectMemberSnapshot{
			PrincipalID:     id,
			ProjectRole:     "executor",
			Status:          "active",
			PlanningProfile: openAITestExecutorProfile(id),
		}
		pool = append(pool, member)
	}
	return pool
}

func TestInstantiatePlanFromTemplateUsesCastingAndDeepestExit(t *testing.T) {
	developer := uuid.New()
	reviewer := uuid.New()
	tester := uuid.New()
	snapshot := CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "交付需求", Content: "实现功能"},
		DigitalEmployeePool: instantiatePool(developer, reviewer, tester),
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		PlaybookCasting: []PlaybookCastingAssignment{
			{RoleKey: "developer", DigitalEmployeeID: developer},
			{RoleKey: "reviewer", DigitalEmployeeID: reviewer},
			{RoleKey: "tester", DigitalEmployeeID: tester},
		},
	}

	raw, err := InstantiatePlanFromTemplate(snapshot)
	require.NoError(t, err)
	plan, err := finalizeInstantiatedPlan(snapshot, raw)
	require.NoError(t, err)
	require.Equal(t, "software_delivery", plan.TemplateKey)
	require.Equal(t, "release_record", plan.ExitDeliverable)
	require.Equal(t, []string{"develop", "review", "test", "release"}, taskKeys(plan))
	require.Equal(t, developer, taskByKey(t, plan, "develop").SelectedEmployeeID)
	require.Equal(t, reviewer, taskByKey(t, plan, "review").SelectedEmployeeID)
	require.Equal(t, tester, taskByKey(t, plan, "test").SelectedEmployeeID)
	require.Equal(t, developer, taskByKey(t, plan, "release").SelectedEmployeeID)
	require.NotEqual(t, taskByKey(t, plan, "develop").SelectedEmployeeID, taskByKey(t, plan, "review").SelectedEmployeeID)
}

func TestInstantiatePlanFromTemplateHonorsPinnedExit(t *testing.T) {
	developer := uuid.New()
	reviewer := uuid.New()
	snapshot := CoordinationSnapshot{
		Demand:              DemandSnapshot{Title: "浅出口", Content: "只交分支"},
		DigitalEmployeePool: instantiatePool(developer, reviewer),
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		PinnedExitDeliverable: "branch_ref",
		PlaybookCasting: []PlaybookCastingAssignment{
			{RoleKey: "developer", DigitalEmployeeID: developer},
			{RoleKey: "reviewer", DigitalEmployeeID: reviewer},
		},
	}

	raw, err := InstantiatePlanFromTemplate(snapshot)
	require.NoError(t, err)
	require.Equal(t, "branch_ref", raw.ExitDeliverable)
	require.Equal(t, []string{"develop"}, taskKeys(raw))
	plan, err := finalizeInstantiatedPlan(snapshot, raw)
	require.NoError(t, err)
	require.Equal(t, []string{"develop"}, taskKeys(plan))
}

func TestInstantiatePlanFromTemplateNoSkeletonFallsThrough(t *testing.T) {
	_, err := InstantiatePlanFromTemplate(CoordinationSnapshot{})
	require.ErrorIs(t, err, errNoTemplateSkeleton)
}

func TestPlanDemandRouteInstantiatesTemplateWithoutCallingPlanner(t *testing.T) {
	developer := uuid.New()
	reviewer := uuid.New()
	tester := uuid.New()
	snapshot := CoordinationSnapshot{
		Demand:              DemandSnapshot{Title: "模板规划", Content: "不要调模型"},
		DigitalEmployeePool: instantiatePool(developer, reviewer, tester),
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		PlaybookCasting: []PlaybookCastingAssignment{
			{RoleKey: "developer", DigitalEmployeeID: developer},
			{RoleKey: "reviewer", DigitalEmployeeID: reviewer},
			{RoleKey: "tester", DigitalEmployeeID: tester},
		},
	}
	planner := &errPlanner{err: errors.New("llm must not be called")}
	activities := NewActivities(nil, planner)

	plan, err := activities.PlanDemandRoute(context.Background(), snapshot)
	require.NoError(t, err)
	require.Equal(t, int32(0), planner.calls.Load())
	require.Equal(t, "release_record", plan.ExitDeliverable)
	require.Equal(t, "template_instantiate", plan.PlannerMetadata["planner"])
}

func TestPlanDemandRouteMarksPlannerTimeoutNonRetryable(t *testing.T) {
	planErr := errors.Join(ErrPlannerRequestTimeout, errors.New("context deadline exceeded"))
	activities := NewActivities(nil, &errPlanner{err: planErr})

	_, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{})

	require.Error(t, err)
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.True(t, appErr.NonRetryable())
	require.Equal(t, errTypePlannerRequestTimeout, appErr.Type())
	require.ErrorIs(t, err, ErrPlannerRequestTimeout)
}

func TestInstantiatePlanFromTemplateUsesSkeletonStepTitle(t *testing.T) {
	developer := uuid.New()
	releaser := uuid.New()
	snapshot := CoordinationSnapshot{
		Demand: DemandSnapshot{Title: "致命缺陷", Content: "修 bug"},
		DigitalEmployeePool: instantiatePool(developer, releaser),
		ScenarioTemplate: &ScenarioTemplateSnapshot{
			Key:  "ecc_code_delivery",
			Name: "ECC 工程交付",
			Spec: map[string]any{
				"spec_version": 2,
				"roles": []any{
					map[string]any{"key": "developer", "title": "开发", "required_capabilities": []any{"code_implementation"}},
					map[string]any{"key": "releaser", "title": "提交发布", "required_capabilities": []any{"code_implementation"}},
				},
				"skeleton": []any{
					map[string]any{
						"step": "develop", "role": "developer", "title": "开发实现",
						"produces_defaults": []any{map[string]any{"name": "head_commit"}},
					},
					map[string]any{
						"step": "commit", "role": "releaser", "title": "提交代码",
						"depends_on": []any{"develop"},
						"produces_defaults": []any{map[string]any{"name": "commit_ref"}},
					},
					map[string]any{
						"step": "push", "role": "releaser", "title": "推送远程",
						"depends_on": []any{"commit"},
						"produces_defaults": []any{map[string]any{"name": "push_receipt"}},
					},
				},
				"exits": []any{
					map[string]any{"deliverable": "head_commit", "label": "开发完成"},
					map[string]any{"deliverable": "commit_ref", "label": "已本地提交"},
					map[string]any{"deliverable": "push_receipt", "label": "已推送远程"},
				},
			},
		},
		PlaybookCasting: []PlaybookCastingAssignment{
			{RoleKey: "developer", DigitalEmployeeID: developer},
			{RoleKey: "releaser", DigitalEmployeeID: releaser},
		},
	}

	raw, err := InstantiatePlanFromTemplate(snapshot)
	require.NoError(t, err)
	plan, err := finalizeInstantiatedPlan(snapshot, raw)
	require.NoError(t, err)
	require.Equal(t, "致命缺陷 · 开发实现", taskByKey(t, plan, "develop").Title)
	require.Equal(t, "致命缺陷 · 提交代码", taskByKey(t, plan, "commit").Title)
	require.Equal(t, "致命缺陷 · 推送远程", taskByKey(t, plan, "push").Title)
	require.Equal(t, []string{"code_implementation"}, taskByKey(t, plan, "develop").RequiredCapabilities)
	require.NotContains(t, taskByKey(t, plan, "develop").RequiredCapabilities, "codebase.analysis")
}

func taskKeys(plan RouteDecisionPlan) []string {
	keys := make([]string, 0, len(plan.Tasks))
	for _, task := range plan.Tasks {
		keys = append(keys, task.Key)
	}
	return keys
}

func taskByKey(t *testing.T, plan RouteDecisionPlan, key string) PlannedTask {
	t.Helper()
	for _, task := range plan.Tasks {
		if task.Key == key {
			return task
		}
	}
	t.Fatalf("missing task %q", key)
	return PlannedTask{}
}
