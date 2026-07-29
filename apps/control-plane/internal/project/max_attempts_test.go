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
	// platform default 3: first failure should retry
	require.Equal(t, ProjectTaskStatusQueued, projectTaskFailureAction(task, FailureFamilyTransientProvider, nil, 3))
	task.AttemptCount = 3
	require.Equal(t, ProjectTaskStatusWaitingHuman, projectTaskFailureAction(task, FailureFamilyTransientProvider, nil, 3))
	// legacy null max with platform 3 behaves as 3, not silent 1
	task.AttemptCount = 1
	task.MaxAttempts = nil
	require.Equal(t, ProjectTaskStatusQueued, projectTaskFailureAction(task, FailureFamilyTransientProvider, nil, 0))
}
