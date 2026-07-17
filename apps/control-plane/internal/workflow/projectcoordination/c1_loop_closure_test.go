package projectcoordination

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
)

// TestReworkTaskCarriesPlanRevisionForJudgeRefire (DEFECT 1, real round-trip):
// drives the ACTUAL rework construction (CreateReworkTaskFromAdversarial →
// buildRevisionTask) and then feeds the CREATED rework task back through
// PrepareAdversarialReview. The adversarial_review criterion MUST be matched
// (Reviewed=true) so the judges re-fire on the rework — which requires the
// created rework task to carry a non-nil AcceptedPlanRevisionID (the field the
// guard in listAdversarialCriteriaForTask checks). Without the fix the rework's
// accepted_plan_revision_id is NULL, the guard returns nil, Reviewed=false, and
// the self-iteration loop silently never closes. This drives the REAL
// construction (not a hand-set fixture) so it would have caught the bug.
func TestReworkTaskCarriesPlanRevisionForJudgeRefire(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	rootTaskID := uuid.New()
	rootKey := "develop"
	maxAttempts := int32(3)

	repo := &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{{
			ID:                     rootTaskID,
			TenantID:               tenantID,
			ProjectID:              projectID,
			DemandID:               &demandID,
			AcceptedPlanRevisionID: &planRevisionID,
			PlannedTaskKey:         &rootKey,
			CoordinationJobID:      &coordinationJobID,
			Title:                  "Implement login",
			Status:                 project.ProjectTaskStatusCompleted,
			AttemptCount:           1,
			MaxAttempts:            &maxAttempts,
			PlannerMetadata:        map[string]any{"iteration_key": "wi-login"},
			// NO LatestTaskResultID → adversarial path seeds a deterministic key.
		}},
		adversarialJudgements: []project.DemandAdversarialJudgement{{
			TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: planRevisionID,
			CriterionID: "crit_adv", ReviewedTaskID: rootTaskID,
			Lens: "security", Verdict: AdversarialVerdictRefuted, Reason: "缺少 nonce 校验",
		}},
		// The demand's adversarial_review criterion still names the ROOT key.
		demandAcceptanceCriteria: []project.DemandAcceptanceCriterion{{
			TenantID:           tenantID,
			DemandID:           demandID,
			PlanRevisionID:     planRevisionID,
			CriterionID:        "crit_adv",
			Statement:          "登录流程必须防重放",
			VerificationMethod: VerificationMethodAdversarialReview,
			Severity:           CriterionSeverityBlocking,
			SatisfiedBy:        []string{rootKey},
		}},
	}
	store := NewProjectStore(repo)

	// (a) create the rework via the REAL shared construction core.
	created, err := store.CreateReworkTaskFromAdversarial(context.Background(), CreateReworkTaskFromAdversarialInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ReviewedTaskID: rootTaskID,
		DemandID:       demandID,
		PlanRevisionID: planRevisionID,
		HeldCriteria:   []HeldAdversarialCriterion{{CriterionID: "crit_adv", Statement: "登录流程必须防重放"}},
	})
	require.NoError(t, err)
	require.False(t, created.Exhausted)
	require.NotEqual(t, uuid.Nil, created.TaskID)

	rework := repo.mustTask(created.TaskID)
	// The created rework carries the source's accepted plan revision (defect 1 fix).
	require.NotNil(t, rework.AcceptedPlanRevisionID, "rework must carry AcceptedPlanRevisionID so the judges can re-fire")
	require.Equal(t, planRevisionID, *rework.AcceptedPlanRevisionID)

	// (b) feed the CREATED rework back through the trigger: the criterion IS
	// matched via the revision-root key, so the judges would re-fire.
	plan, err := store.PrepareAdversarialReview(context.Background(), PrepareAdversarialReviewInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: rework.ID,
	})
	require.NoError(t, err)
	require.True(t, plan.Reviewed, "the rework must re-fire the judges; a NULL AcceptedPlanRevisionID silently breaks the loop")
	require.Len(t, plan.Items, 1)
	require.Equal(t, "crit_adv", plan.Items[0].Input.CriterionID)
}

// newRootDownstreamRevisionRepo builds a graph where downstream D is anchored on
// the revision ROOT T0 (blocker_task_id = T0), T0 is completed+accepted (the
// reviewed work has converged), and Rn is a rework of T0. It models the exact
// post-convergence state ResolveReadyDownstream faces when the completing task is
// the rework Rn rather than the root.
func newRootDownstreamRevisionRepo(tenantID, projectID, rootTaskID, reworkTaskID, downstreamTaskID, rootResultID uuid.UUID) *projectStoreMemoryRepository {
	return &projectStoreMemoryRepository{
		tasks: []project.ProjectTask{
			{
				ID: rootTaskID, TenantID: tenantID, ProjectID: projectID,
				Title: "root reviewed task", Status: project.ProjectTaskStatusCompleted,
				LatestTaskResultID: &rootResultID,
			},
			{
				ID: reworkTaskID, TenantID: tenantID, ProjectID: projectID,
				Title: "rework of root", Status: project.ProjectTaskStatusCompleted,
				RevisionOfTaskID: &rootTaskID,
				PlannerMetadata:  map[string]any{"revision_root_task_id": rootTaskID.String()},
			},
			{
				ID: downstreamTaskID, TenantID: tenantID, ProjectID: projectID,
				Title: "downstream dependent", Status: "blocked",
			},
		},
		projectTaskResults: []project.ProjectTaskResult{{
			ID: rootResultID, TenantID: tenantID, ProjectID: projectID, ProjectTaskID: rootTaskID,
			ResultStatus: project.TaskResultStatusCompleted, Decision: project.TaskResultDecisionCompleteAccepted,
			ValidationStatus: "accepted",
		}},
		taskDependencies: []project.ProjectTaskDependency{{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			DependentTaskID: downstreamTaskID, BlockerTaskID: rootTaskID,
		}},
	}
}

// TestResolveReadyDownstreamRootAnchorForRevision (DEFECT 2, store mechanism):
// downstream D is anchored on the revision ROOT T0. When the rework Rn completes,
// resolving downstream-of-Rn finds NOTHING (D is blocked on T0, not Rn) — the
// deadlock. With ResolveRevisionRoot=true the activity re-anchors on T0 and
// flips D blocked→planned. The flag=false path proves the pre-fix / v1/v2 /
// non-revision behavior (downstream-of-the-completing-task) is preserved.
func TestResolveReadyDownstreamRootAnchorForRevision(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	rootTaskID := uuid.New()
	reworkTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	rootResultID := uuid.New()

	// flag=false: anchors on the completing rework → finds nothing, D stays blocked.
	repoOld := newRootDownstreamRevisionRepo(tenantID, projectID, rootTaskID, reworkTaskID, downstreamTaskID, rootResultID)
	readyOld, err := NewProjectStore(repoOld).ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: reworkTaskID, ResolveRevisionRoot: false,
	})
	require.NoError(t, err)
	require.Empty(t, readyOld, "downstream-of-the-rework must find nothing (the deadlock)")
	require.Equal(t, "blocked", repoOld.mustTask(downstreamTaskID).Status)

	// flag=true: re-anchors on the revision root → releases the root's downstream.
	repoNew := newRootDownstreamRevisionRepo(tenantID, projectID, rootTaskID, reworkTaskID, downstreamTaskID, rootResultID)
	readyNew, err := NewProjectStore(repoNew).ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: reworkTaskID, ResolveRevisionRoot: true,
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{downstreamTaskID}, readyNew, "root-anchored resolution must release the root's downstream")
	require.Equal(t, project.ProjectTaskStatusPlanned, repoNew.mustTask(downstreamTaskID).Status)

	// A non-revision completing task under the same flag is a no-op re-anchor
	// (revisionRootTaskID(self)==self): resolving downstream of the root task
	// itself still releases D, exactly as before.
	repoSelf := newRootDownstreamRevisionRepo(tenantID, projectID, rootTaskID, reworkTaskID, downstreamTaskID, rootResultID)
	readySelf, err := NewProjectStore(repoSelf).ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: rootTaskID, ResolveRevisionRoot: true,
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{downstreamTaskID}, readySelf)
}

// TestRevisionConvergedReleasesRootDownstream (DEFECT 2, v3 workflow): a held
// root is reworked; when the rework completes and the judges are SATISFIED
// (converged), the workflow falls through to resolveReadyDownstream under the v3
// fence with ResolveRevisionRoot=true, so the real activity re-anchors on the
// revision root and the root's (round-0-blocked) downstream is released.
func TestRevisionConvergedReleasesRootDownstream(t *testing.T) {
	reworkTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := newHeldAdversarialStore(reworkTaskID, downstreamTaskID)

	env := runHeldAdversarialWorkflow(t, store, reworkTaskID, AdversarialReviewForTaskResult{
		Reviewed:     true,
		AllSatisfied: true, // judges satisfied → converged
		AnyEscalated: false,
	})
	require.True(t, env.IsWorkflowCompleted())

	require.Empty(t, store.reworkFromAdversarialInputs, "a converged review must not auto-rework")
	require.Len(t, store.resolveReadyInputs, 1, "converged revision completion must resolve downstream")
	require.True(t, store.resolveReadyInputs[0].ResolveRevisionRoot,
		"v3 must resolve downstream of the revision ROOT, not the completing rework")
	require.Contains(t, store.dispatchInputs, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         downstreamTaskID,
		DispatchReason: project.DispatchReasonDependencyUnlocked,
	}, "the root's downstream dependent must be dispatched once the rework converges")
}

// TestRevisionExhaustedReleasesRootDownstream (DEFECT 2, v3 workflow): a held
// root is reworked but the rework is budget-exhausted; the workflow releases to
// the acceptance gate via resolveReadyDownstream with ResolveRevisionRoot=true,
// so the root's downstream is released and the demand can reach
// acceptance_pending (where the human tier-3 override lives).
func TestRevisionExhaustedReleasesRootDownstream(t *testing.T) {
	reworkTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := newHeldAdversarialStore(reworkTaskID, downstreamTaskID)
	store.reworkFromAdversarialResult = CreateReworkTaskFromAdversarialResult{Exhausted: true}

	env := runHeldAdversarialWorkflow(t, store, reworkTaskID, AdversarialReviewForTaskResult{
		Reviewed:       true,
		AllSatisfied:   false,
		AnyEscalated:   false,
		ReviewedTaskID: reworkTaskID,
		DemandID:       uuid.New(),
		PlanRevisionID: uuid.New(),
		HeldCriteria:   []HeldAdversarialCriterion{{CriterionID: "crit-secure", Statement: "登录必须防重放"}},
	})
	require.True(t, env.IsWorkflowCompleted())

	require.Len(t, store.reworkFromAdversarialInputs, 1, "exhausted path still consults the rework activity")
	require.Len(t, store.resolveReadyInputs, 1, "budget-exhausted rework must release to the acceptance gate")
	require.True(t, store.resolveReadyInputs[0].ResolveRevisionRoot,
		"v3 exhaust path must also resolve downstream of the revision ROOT")
	require.Contains(t, store.dispatchInputs, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         downstreamTaskID,
		DispatchReason: project.DispatchReasonDependencyUnlocked,
	})
}

// TestNonRevisionCompletionKeepsCompletingTaskAnchorAtV2 (DEFECT 2, regression):
// under adversarial version < 3 (no v3 fence marker) a completion resolves
// downstream-of-the-COMPLETING-task with ResolveRevisionRoot=false — the exact
// replay-preserving behavior for DefaultVersion/v1/v2 histories. Driven at v2 via
// a raw GetVersion of the same change id in an isolated workflow whose only marker
// is written by an explicit v2 SetVersion probe is not available here, so this
// asserts the store-side contract directly: flag=false anchors on the completing
// task regardless of whether it is a revision.
func TestNonRevisionCompletionKeepsCompletingTaskAnchorAtV2(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	rootTaskID := uuid.New()
	reworkTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	rootResultID := uuid.New()

	repo := newRootDownstreamRevisionRepo(tenantID, projectID, rootTaskID, reworkTaskID, downstreamTaskID, rootResultID)
	// flag=false (v1/v2/Default): even though the completing task is a revision,
	// resolution stays anchored on it — replay-identical to old histories.
	ready, err := NewProjectStore(repo).ResolveReadyDownstream(context.Background(), ResolveReadyDownstreamInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: reworkTaskID, ResolveRevisionRoot: false,
	})
	require.NoError(t, err)
	require.Empty(t, ready, "v1/v2/Default must resolve downstream-of-the-completing-task exactly as recorded")
	require.Equal(t, "blocked", repo.mustTask(downstreamTaskID).Status)
}
