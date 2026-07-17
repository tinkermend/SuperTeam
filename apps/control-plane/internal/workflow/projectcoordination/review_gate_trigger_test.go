package projectcoordination

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
	"go.temporal.io/sdk/testsuite"
)

// stubReviewGateRepo is a minimal project.Repository for RunReviewGateForTask
// end-to-end unit tests: it answers GetProjectTask / ListDemandAcceptanceCriteria
// / GetProject / ListProjectTaskResults and CAPTURES the review_gate verdict the
// projection writes (CreateReviewGateVerdict), inheriting nil (panic-on-call) for
// everything else via the embedded interface.
type stubReviewGateRepo struct {
	project.Repository
	task     project.ProjectTask
	criteria []project.DemandAcceptanceCriterion
	result   project.ProjectTaskResult
	policy   map[string]any
	verdicts []project.CreateReviewGateVerdictRequest
}

func (r *stubReviewGateRepo) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (project.ProjectTask, error) {
	return r.task, nil
}

func (r *stubReviewGateRepo) ListDemandAcceptanceCriteria(ctx context.Context, tenantID, demandID, planRevisionID uuid.UUID) ([]project.DemandAcceptanceCriterion, error) {
	return r.criteria, nil
}

func (r *stubReviewGateRepo) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	return project.Project{TenantID: tenantID, ID: projectID, CoordinationPolicy: r.policy}, nil
}

func (r *stubReviewGateRepo) ListProjectTaskResults(ctx context.Context, req project.ListProjectTaskResultsRequest) ([]project.ProjectTaskResult, error) {
	return []project.ProjectTaskResult{r.result}, nil
}

func (r *stubReviewGateRepo) CreateReviewGateVerdict(ctx context.Context, req project.CreateReviewGateVerdictRequest) error {
	r.verdicts = append(r.verdicts, req)
	return nil
}

func reviewGateCriterionRow(demandID, planRevisionID uuid.UUID, id, satisfiedBy string) project.DemandAcceptanceCriterion {
	return project.DemandAcceptanceCriterion{
		DemandID:           demandID,
		PlanRevisionID:     planRevisionID,
		CriterionID:        id,
		Statement:          "交付未引入违规 " + id,
		VerificationMethod: VerificationMethodReviewGate,
		Severity:           CriterionSeverityBlocking,
		SatisfiedBy:        []string{satisfiedBy},
	}
}

func reviewGateTaskWithResult(tenantID, projectID, demandID, planRevisionID uuid.UUID, plannedKey, deliverableValue string) (project.ProjectTask, project.ProjectTaskResult) {
	key := plannedKey
	resultID := uuid.New()
	task := project.ProjectTask{
		ID:                     uuid.New(),
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               &demandID,
		AcceptedPlanRevisionID: &planRevisionID,
		PlannedTaskKey:         &key,
		LatestTaskResultID:     &resultID,
	}
	result := project.ProjectTaskResult{
		ID: resultID,
		Contract: project.TaskResultContract{
			Summary: "实现登录",
			Deliverables: []project.TaskResultDeliverable{
				{Name: "auth.go", Value: deliverableValue},
			},
		},
	}
	return task, result
}

// TestRunReviewGateForTaskProjectsVerdict drives the whole Activity end-to-end
// through a real ProjectStore + stub repo: a completed task on the hook for a
// review_gate criterion whose real artifact leaks a secret projects an
// UNSATISFIED review_gate verdict; a clean artifact projects SATISFIED. The judge
// client is deliberately nil (the LLM code_review detector fails open); only the
// rule secret_leak detector fires, proving the wiring runs the standard detectors.
func TestRunReviewGateForTaskProjectsVerdict(t *testing.T) {
	tenantID, projectID := uuid.New(), uuid.New()
	demandID, planRevisionID := uuid.New(), uuid.New()

	t.Run("violation projects unsatisfied", func(t *testing.T) {
		// Inline deliverable value carries a leaked OpenAI-style key → secret_leak
		// (block) fires → gate HOLDs.
		task, result := reviewGateTaskWithResult(tenantID, projectID, demandID, planRevisionID, "impl_task",
			"diff --git a/auth.go\n+const apiKey = \"sk-ABCDEFGHIJKLMNOP1234\"")
		repo := &stubReviewGateRepo{
			task:     task,
			criteria: []project.DemandAcceptanceCriterion{reviewGateCriterionRow(demandID, planRevisionID, "crit_rg", "impl_task")},
			result:   result,
		}
		activities := &Activities{store: &ProjectStore{repository: repo}}
		out, err := activities.RunReviewGateForTask(context.Background(), RunReviewGateForTaskInput{
			TenantID: tenantID, ProjectID: projectID, CompletedTaskID: task.ID,
		})
		require.NoError(t, err)
		require.True(t, out.Reviewed)
		require.True(t, out.AnyViolation)
		require.Len(t, repo.verdicts, 1)
		require.Equal(t, "crit_rg", repo.verdicts[0].CriterionID)
		require.Equal(t, reviewGateVerdictUnsatisfied, repo.verdicts[0].Verdict)
	})

	t.Run("clean projects satisfied", func(t *testing.T) {
		task, result := reviewGateTaskWithResult(tenantID, projectID, demandID, planRevisionID, "impl_task",
			"diff --git a/auth.go\n+const timeout = 30")
		repo := &stubReviewGateRepo{
			task:     task,
			criteria: []project.DemandAcceptanceCriterion{reviewGateCriterionRow(demandID, planRevisionID, "crit_rg", "impl_task")},
			result:   result,
		}
		activities := &Activities{store: &ProjectStore{repository: repo}}
		out, err := activities.RunReviewGateForTask(context.Background(), RunReviewGateForTaskInput{
			TenantID: tenantID, ProjectID: projectID, CompletedTaskID: task.ID,
		})
		require.NoError(t, err)
		require.True(t, out.Reviewed)
		require.False(t, out.AnyViolation)
		require.Len(t, repo.verdicts, 1)
		require.Equal(t, reviewGateVerdictSatisfied, repo.verdicts[0].Verdict)
	})

	t.Run("no review_gate criterion is a no-op", func(t *testing.T) {
		task, result := reviewGateTaskWithResult(tenantID, projectID, demandID, planRevisionID, "unrelated",
			"diff --git a/auth.go\n+const apiKey = \"sk-ABCDEFGHIJKLMNOP1234\"")
		repo := &stubReviewGateRepo{
			task:     task,
			criteria: []project.DemandAcceptanceCriterion{reviewGateCriterionRow(demandID, planRevisionID, "crit_rg", "impl_task")},
			result:   result,
		}
		activities := &Activities{store: &ProjectStore{repository: repo}}
		out, err := activities.RunReviewGateForTask(context.Background(), RunReviewGateForTaskInput{
			TenantID: tenantID, ProjectID: projectID, CompletedTaskID: task.ID,
		})
		require.NoError(t, err)
		require.False(t, out.Reviewed)
		require.Empty(t, repo.verdicts)
	})
}

// TestReviewGateTriggerReplaySafe: on a NEW workflow the review-gate fence fires
// RunReviewGateForTask, and — even when the gate reports a violation — the
// workflow NEVER early-returns to block downstream: it falls through to
// resolveReadyDownstream and dispatches the ready downstream task (the review_gate
// hold lives at the acceptance gate, not the task graph). The mandatory
// TestReplayRealCoordinatorHistory proves the new fence keeps recorded histories
// replay-green (the fence returns DefaultVersion → no new command on old
// histories).
func TestReviewGateTriggerReplaySafe(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	rootTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := newHeldAdversarialStore(rootTaskID, downstreamTaskID)
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	// Adversarial has nothing to review → falls through to the review-gate fence.
	env.OnActivity(activities.AdversarialReviewForTask, mock.Anything, mock.Anything).
		Return(AdversarialReviewForTaskResult{Reviewed: false}, nil)
	// Review gate DETECTS a violation. The verdict is persisted by the activity;
	// the workflow must still fall through and never block downstream.
	reviewGateCalled := false
	env.OnActivity(activities.RunReviewGateForTask, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { reviewGateCalled = true }).
		Return(RunReviewGateForTaskResult{Reviewed: true, AnyViolation: true}, nil)

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
	require.True(t, reviewGateCalled, "new workflow must fire RunReviewGateForTask")

	// A detected review_gate violation did NOT early-return: downstream still
	// unlocked. The hold is at the acceptance gate (persisted verdict), never here.
	require.Len(t, store.resolveReadyInputs, 1, "review-gate violation must still reach resolveReadyDownstream")
	require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
	require.Contains(t, store.dispatchInputs, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         downstreamTaskID,
		DispatchReason: project.DispatchReasonDependencyUnlocked,
	}, "review-gate violation must still unlock the downstream task")
}
