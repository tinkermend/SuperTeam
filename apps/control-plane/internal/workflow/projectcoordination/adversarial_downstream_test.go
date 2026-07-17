package projectcoordination

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
	"go.temporal.io/sdk/testsuite"
)

// newHeldAdversarialStore builds a recording store whose completed root task is
// COMPLETE_ACCEPTED and whose graph has one downstream task ready once the root
// unlocks. The adversarial trigger's held/error verdict is injected per-test via
// env.OnActivity on AdversarialReviewForTask, so these tests do not depend on the
// activity's judge internals — they assert only the workflow-level control flow.
func newHeldAdversarialStore(rootTaskID, downstreamTaskID uuid.UUID) *recordingActivityStore {
	return &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "对抗held不阻塞任务图",
				Content: "held 复审必须放行下游至验收闸",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: uuid.New(), ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:               uuid.New(),
		routeID:             uuid.New(),
		routeEventID:        uuid.New(),
		taskIDs:             []uuid.UUID{rootTaskID, downstreamTaskID},
		dispatchableTaskIDs: []uuid.UUID{rootTaskID},
		readyDownstreamIDs:  []uuid.UUID{downstreamTaskID},
		resultDecision:      InspectTaskResultDecisionResult{ResultID: uuid.New(), Decision: string(project.TaskResultDecisionCompleteAccepted)},
		dispatchEvent:       uuid.New(),
	}
}

// TestAdversarialHeldV2UnlocksDownstream: under version 2 (new workflow →
// GetVersion returns max=2), a review that HOLDS the demand
// (Reviewed=true, AllSatisfied=false) must NOT early-return
// taskCompletionPending. The workflow falls through to resolveReadyDownstream and
// dispatches the ready downstream task, so the graph can reach the acceptance
// gate where the persisted verdict holds it for the human tier-3 override.
func TestAdversarialHeldV2UnlocksDownstream(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	rootTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := newHeldAdversarialStore(rootTaskID, downstreamTaskID)
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	// Held: reviewed, majority-refute → unsatisfied. No error.
	env.OnActivity(activities.AdversarialReviewForTask, mock.Anything, mock.Anything).
		Return(AdversarialReviewForTaskResult{Reviewed: true, AllSatisfied: false, AnyEscalated: false}, nil)

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      rootTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())

	// The held review did NOT early-return: resolveReadyDownstream ran on the
	// completed root, and the ready downstream was dispatched.
	require.Len(t, store.resolveReadyInputs, 1, "held v2 review must still reach resolveReadyDownstream")
	require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
	require.Contains(t, store.dispatchInputs, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         downstreamTaskID,
		DispatchReason: project.DispatchReasonDependencyUnlocked,
	}, "held v2 review must still unlock the downstream task")
}

// TestAdversarialReviewErrorV2DoesNotBlockGraph: under version 2, a review that
// ERRORS (transport failure, retries exhausted) must NOT early-return
// taskCompletionPending. The workflow swallows the error and falls through to
// resolveReadyDownstream; the verdict-less blocking criterion holds the demand at
// the acceptance gate, not here, so the graph is not stalled in `executing`.
func TestAdversarialReviewErrorV2DoesNotBlockGraph(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	rootTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := newHeldAdversarialStore(rootTaskID, downstreamTaskID)
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	// Review cannot complete: activity errors on every attempt (retries exhaust,
	// MaximumAttempts=3), so .Get returns reviewErr != nil.
	env.OnActivity(activities.AdversarialReviewForTask, mock.Anything, mock.Anything).
		Return(AdversarialReviewForTaskResult{}, errors.New("llm transport unavailable"))

	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      rootTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError(), "a swallowed review error must not fail the workflow")

	// The errored review did NOT early-return: resolveReadyDownstream ran and the
	// downstream was dispatched. The gate — not the workflow — holds the demand.
	require.Len(t, store.resolveReadyInputs, 1, "errored v2 review must still reach resolveReadyDownstream")
	require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
	require.Contains(t, store.dispatchInputs, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         downstreamTaskID,
		DispatchReason: project.DispatchReasonDependencyUnlocked,
	}, "errored v2 review must still unlock the downstream task")
}
