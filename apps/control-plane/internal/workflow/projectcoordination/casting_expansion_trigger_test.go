package projectcoordination

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
	"go.temporal.io/sdk/testsuite"
)

type castingExpansionRecordingStore struct {
	*recordingActivityStore
	expansionInputs []MaybeRequestCastingExpansionInput
	expansionResult MaybeRequestCastingExpansionResult
}

func (s *castingExpansionRecordingStore) MaybeRequestCastingExpansionAfterTask(ctx context.Context, input MaybeRequestCastingExpansionInput) (MaybeRequestCastingExpansionResult, error) {
	s.calls = append(s.calls, "MaybeRequestCastingExpansionAfterTask")
	s.expansionInputs = append(s.expansionInputs, input)
	return s.expansionResult, nil
}

func TestMaybeRequestCastingExpansionActivityAfterAcceptedCompletion(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	taskID := uuid.New()
	decisionID := uuid.New()
	base := &recordingActivityStore{
		snapshot:        CoordinationSnapshot{ProjectID: projectID},
		resultDecision:  InspectTaskResultDecisionResult{ResultID: uuid.New(), Decision: string(project.TaskResultDecisionCompleteAccepted)},
		readyDownstreamIDs: []uuid.UUID{},
		dispatchEvent:   uuid.New(),
	}
	store := &castingExpansionRecordingStore{
		recordingActivityStore: base,
		expansionResult: MaybeRequestCastingExpansionResult{
			Requested:        true,
			DecisionID:       decisionID,
			SuggestedRoleKey: "reviewer",
		},
	}
	activities := newRawDispatchWorkflowActivities(store.recordingActivityStore)
	// Replace store so type-assert finds castingExpansionStore.
	activities.Activities = NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      taskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 5*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Contains(t, store.calls, "MaybeRequestCastingExpansionAfterTask")
	require.Len(t, store.expansionInputs, 1)
	require.Equal(t, taskID, store.expansionInputs[0].CompletedTaskID)
	require.Equal(t, projectID, store.expansionInputs[0].ProjectID)
}
