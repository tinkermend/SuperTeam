package project

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectiveProjectTaskMaxAttempts(t *testing.T) {
	three := int32(3)
	one := int32(1)
	ten := int32(10)
	zero := int32(0)

	require.Equal(t, int32(3), EffectiveProjectTaskMaxAttempts(nil, 0))
	require.Equal(t, int32(3), EffectiveProjectTaskMaxAttempts(&zero, 0))
	require.Equal(t, int32(1), EffectiveProjectTaskMaxAttempts(&one, 0))
	require.Equal(t, int32(3), EffectiveProjectTaskMaxAttempts(&three, 0))
	require.Equal(t, int32(5), EffectiveProjectTaskMaxAttempts(&ten, 0), "clamp upper bound")
	require.Equal(t, int32(2), EffectiveProjectTaskMaxAttempts(nil, 2))
	require.Equal(t, int32(4), EffectiveProjectTaskMaxAttempts(&zero, 4))
	// explicit task wins over platform default
	require.Equal(t, int32(1), EffectiveProjectTaskMaxAttempts(&one, 4))
}

func TestProjectTaskFailureActionUsesEffectiveMaxAttempts(t *testing.T) {
	task := ProjectTask{AttemptCount: 1}
	// B-layer (provider actually ran): first failure may auto-requeue within attempt budget
	require.Equal(t, ProjectTaskStatusQueued, projectTaskFailureAction(task, FailureFamilyTransientProvider, nil, 3))
	task.AttemptCount = 3
	require.Equal(t, ProjectTaskStatusWaitingHuman, projectTaskFailureAction(task, FailureFamilyTransientProvider, nil, 3))
	// legacy null max with platform 3 behaves as 3, not silent 1
	task.AttemptCount = 1
	task.MaxAttempts = nil
	require.Equal(t, ProjectTaskStatusQueued, projectTaskFailureAction(task, FailureFamilyTransientProvider, nil, 0))
	// A-layer (never started / runtime drop): never auto-requeue — human only
	require.Equal(t, ProjectTaskStatusWaitingHuman, projectTaskFailureAction(task, FailureFamilyTransientRuntime, nil, 3))
	require.Equal(t, ProjectTaskStatusWaitingHuman, projectTaskFailureAction(task, FailureFamilyDispatchTransient, nil, 3))
	require.Equal(t, ProjectTaskStatusWaitingHuman, projectTaskFailureAction(task, FailureFamilyRuntimeStartTimeout, nil, 3))
}

func TestProjectTaskFailureActionBudgetFuseWaitsHuman(t *testing.T) {
	// Spec 2026-08-09 §13 #12: budget_fuse must not fall through to default failed.
	task := ProjectTask{AttemptCount: 1}
	retryableFalse := false
	require.Equal(t, ProjectTaskStatusWaitingHuman, projectTaskFailureAction(task, FailureFamilyBudgetFuse, &retryableFalse, 3))
	require.Equal(t, HumanWaitReasonBudgetApproval, humanWaitReasonForFailureFamily(FailureFamilyBudgetFuse))
	summary := humanReadableFailureSummary(FailureFamilyBudgetFuse, "wall_clock_exceeded")
	require.Contains(t, summary, "任务预算熔断")
	require.Contains(t, summary, "wall_clock_exceeded")
}
