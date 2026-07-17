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

// runHeldAdversarialWorkflow drives one root-task completion through the v3
// adversarial trigger with the injected review result, returning the recording
// store for assertions. The rework activity runs for real, delegating to the
// recording store's CreateReworkTaskFromAdversarial seam (result/err set by the
// caller before invoking).
func runHeldAdversarialWorkflow(t *testing.T, store *recordingActivityStore, rootTaskID uuid.UUID, review AdversarialReviewForTaskResult) *testsuite.TestWorkflowEnvironment {
	t.Helper()
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.OnActivity(activities.AdversarialReviewForTask, mock.Anything, mock.Anything).Return(review, nil)

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
	}, 10*time.Second)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	return env
}

// TestAdversarialHeldWithBudgetTriggersRework (v3): a held review with
// rework-eligible criteria and remaining budget dispatches an auto-rework task
// (CreateReworkTaskFromAdversarial invoked) and does NOT release downstream —
// resolveReadyDownstream is not called.
func TestAdversarialHeldWithBudgetTriggersRework(t *testing.T) {
	rootTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	reworkTaskID := uuid.New()
	demandID := uuid.New()
	planRevisionID := uuid.New()
	store := newHeldAdversarialStore(rootTaskID, downstreamTaskID)
	store.reworkFromAdversarialResult = CreateReworkTaskFromAdversarialResult{TaskID: reworkTaskID}

	runHeldAdversarialWorkflow(t, store, rootTaskID, AdversarialReviewForTaskResult{
		Reviewed:       true,
		AllSatisfied:   false,
		AnyEscalated:   false,
		ReviewedTaskID: rootTaskID,
		DemandID:       demandID,
		PlanRevisionID: planRevisionID,
		HeldCriteria:   []HeldAdversarialCriterion{{CriterionID: "crit-secure", Statement: "登录必须防重放"}},
	})

	require.Len(t, store.reworkFromAdversarialInputs, 1, "held+budget must invoke CreateReworkTaskFromAdversarial")
	require.Equal(t, rootTaskID, store.reworkFromAdversarialInputs[0].ReviewedTaskID)
	require.Equal(t, demandID, store.reworkFromAdversarialInputs[0].DemandID)
	require.Equal(t, planRevisionID, store.reworkFromAdversarialInputs[0].PlanRevisionID)
	require.Len(t, store.reworkFromAdversarialInputs[0].HeldCriteria, 1)

	require.Empty(t, store.resolveReadyInputs, "auto-rework must NOT release downstream")

	// The rework task itself is dispatched for execution.
	require.Contains(t, store.dispatchInputs, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         reworkTaskID,
		DispatchReason: project.DispatchReasonRetry,
	})
}

// TestAdversarialHeldExhaustedReleasesToAcceptance (v3): a held review whose
// rework is budget-exhausted releases to the acceptance gate —
// resolveReadyDownstream IS called (v2/4.6 behavior for a human override).
func TestAdversarialHeldExhaustedReleasesToAcceptance(t *testing.T) {
	rootTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := newHeldAdversarialStore(rootTaskID, downstreamTaskID)
	store.reworkFromAdversarialResult = CreateReworkTaskFromAdversarialResult{Exhausted: true}

	runHeldAdversarialWorkflow(t, store, rootTaskID, AdversarialReviewForTaskResult{
		Reviewed:       true,
		AllSatisfied:   false,
		AnyEscalated:   false,
		ReviewedTaskID: rootTaskID,
		DemandID:       uuid.New(),
		PlanRevisionID: uuid.New(),
		HeldCriteria:   []HeldAdversarialCriterion{{CriterionID: "crit-secure", Statement: "登录必须防重放"}},
	})

	require.Len(t, store.reworkFromAdversarialInputs, 1, "exhausted path still consults CreateReworkTaskFromAdversarial")
	require.Len(t, store.resolveReadyInputs, 1, "budget-exhausted held review must release to acceptance gate")
	require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
	require.Contains(t, store.dispatchInputs, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         downstreamTaskID,
		DispatchReason: project.DispatchReasonDependencyUnlocked,
	})
}

// TestAdversarialEscalateOrErrorReleasesToHuman (v3): escalate_human (budget
// short-circuit) OR a rework-activity error releases to the human acceptance gate
// via resolveReadyDownstream, and NEVER auto-reworks / auto-passes.
func TestAdversarialEscalateOrErrorReleasesToHuman(t *testing.T) {
	t.Run("escalate_human", func(t *testing.T) {
		rootTaskID := uuid.New()
		downstreamTaskID := uuid.New()
		store := newHeldAdversarialStore(rootTaskID, downstreamTaskID)

		runHeldAdversarialWorkflow(t, store, rootTaskID, AdversarialReviewForTaskResult{
			Reviewed:       true,
			AllSatisfied:   false,
			AnyEscalated:   true, // budget escalate_human
			ReviewedTaskID: rootTaskID,
			DemandID:       uuid.New(),
			PlanRevisionID: uuid.New(),
			HeldCriteria:   nil,
		})

		require.Empty(t, store.reworkFromAdversarialInputs, "escalate_human must NOT auto-rework")
		require.Len(t, store.resolveReadyInputs, 1, "escalate_human must release to the acceptance gate")
		require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
	})

	t.Run("rework_error", func(t *testing.T) {
		rootTaskID := uuid.New()
		downstreamTaskID := uuid.New()
		store := newHeldAdversarialStore(rootTaskID, downstreamTaskID)
		store.reworkFromAdversarialErr = errors.New("store unavailable")

		runHeldAdversarialWorkflow(t, store, rootTaskID, AdversarialReviewForTaskResult{
			Reviewed:       true,
			AllSatisfied:   false,
			AnyEscalated:   false,
			ReviewedTaskID: rootTaskID,
			DemandID:       uuid.New(),
			PlanRevisionID: uuid.New(),
			HeldCriteria:   []HeldAdversarialCriterion{{CriterionID: "crit-secure", Statement: "登录必须防重放"}},
		})

		require.NotEmpty(t, store.reworkFromAdversarialInputs, "rework was attempted")
		require.Len(t, store.resolveReadyInputs, 1, "a rework error must release to the human acceptance gate")
		require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
		// The failed rework task was never created, so no retry-reason dispatch fired.
		for _, dispatched := range store.dispatchInputs {
			require.NotEqual(t, project.DispatchReasonRetry, dispatched.DispatchReason, "no rework task must be dispatched on error")
		}
	})
}
