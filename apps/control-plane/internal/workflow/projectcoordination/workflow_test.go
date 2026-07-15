package projectcoordination

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/testsuite"
)

func TestProjectCoordinatorHandlesDemandSubmitted(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "验证 Runtime",
				Content: "检查心跳",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:         uuid.New(),
		routeID:       uuid.New(),
		routeEventID:  uuid.New(),
		taskID:        uuid.New(),
		dispatchEvent: uuid.New(),
	}
	store.dispatchableTaskIDs = []uuid.UUID{store.taskID}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
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
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"DecomposeAcceptedPlanRevision",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
}

func TestProjectCoordinatorSurvivesHandlerActivityFailure(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	store := &recordingActivityStore{
		snapshot:     CoordinationSnapshot{ProjectID: projectID},
		createJobErr: temporal.NewNonRetryableApplicationError("db down", "TransientOutage", errors.New("db down")),
	}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:      projectID,
			DemandID:       uuid.New(),
			CreatedEventID: uuid.New(),
		})
	}, time.Millisecond)
	// A later signal must still be processed: the coordinator did not die on the failed
	// demand handler, so shutdown drains cleanly instead of the workflow being long gone.
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	// The failed handler is recorded as an audit event, then the workflow keeps looping.
	require.Equal(t, []string{"CreateCoordinationJob", "AppendProjectEvent"}, store.calls)
}

// A terminal ErrNoSuitableEmployee planning failure must route the demand to the
// rejection/diagnosis surface (RejectDemandPlanning) exactly once — not spin the
// planner 3× (fix: 不可重试) and not fall through to the generic signal_failed audit
// event (fix: 需求驳回诊断可见). The diagnosis carried to the surface is the planner's
// human-readable reason with its ways-out hint.
func TestProjectCoordinatorRejectsDemandWhenNoSuitableEmployee(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	demandID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: projectID,
			Demand:    DemandSnapshot{ID: demandID, Title: "通过审查合入"},
		},
		jobID: uuid.New(),
	}
	planner := &errPlanner{err: fmt.Errorf("%w: 项目员工池无法满足审查独立性约束（需≥2名可调度员工）；可改选更浅出口、为项目补充员工、或换用模板", ErrNoSuitableEmployee)}
	activities := NewActivities(store, planner)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:      projectID,
			DemandID:       demandID,
			CreatedEventID: uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	// One planner call only: the family error is non-retryable.
	require.Equal(t, int32(1), planner.calls.Load())
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"RejectDemandPlanning",
	}, store.calls)
	require.Len(t, store.rejectDemandInputs, 1)
	require.Equal(t, demandID, store.rejectDemandInputs[0].DemandID)
	require.Contains(t, store.rejectDemandInputs[0].Diagnosis, "补充员工")
}

// TestProjectCoordinatorUntypedPlannerErrorTakesLegacySignalFailedPath pins the
// replay-compatibility discriminator for the terminal-reject branches: only the
// typed non-retryable NoSuitableEmployee ApplicationError routes to
// RejectDemandPlanning. An UNTYPED planner error — which is the only kind old
// (pre-reject-branch) histories can contain in their recorded
// ActivityTaskFailed events — must keep falling through to the legacy path
// (error escapes the handler, the survive-handler-error loop records
// workflow.signal_failed). This error-type check, not a GetVersion fence, is
// what keeps old histories replaying deterministically; see replay_test.go for
// the real-history pin.
func TestProjectCoordinatorUntypedPlannerErrorTakesLegacySignalFailedPath(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	demandID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: projectID,
			Demand:    DemandSnapshot{ID: demandID, Title: "通过审查合入"},
		},
		jobID: uuid.New(),
	}
	// Plain wrapped error, NOT the typed non-retryable family error: this is
	// what pre-a9a4b8a9 histories carry (e.g. "wrapError" failures).
	planner := &errPlanner{err: errors.New("no suitable employee: task \"review\": employee scored 0.30")}
	activities := NewActivities(store, planner)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:      projectID,
			DemandID:       demandID,
			CreatedEventID: uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	// Legacy path: the untyped error escapes the handler and the coordinator's
	// survive-handler-error loop records workflow.signal_failed via
	// AppendProjectEvent — the demand is NOT routed through RejectDemandPlanning.
	require.Contains(t, store.appendEventTypes, "workflow.signal_failed")
	require.Empty(t, store.rejectDemandInputs)
	require.NotContains(t, store.calls, "RejectDemandPlanning")
}

// TestProjectCoordinatorSurvivesRejectDemandPlanningFailure pins that a failure
// of the RejectDemandPlanning activity itself (e.g. a DB outage while marking
// the demand failed) degrades to the survive-handler-error audit path instead
// of killing the long-lived coordinator: the workflow must stay alive, record
// workflow.signal_failed, and keep processing subsequent signals.
func TestProjectCoordinatorSurvivesRejectDemandPlanningFailure(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	demandID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: projectID,
			Demand:    DemandSnapshot{ID: demandID, Title: "通过审查合入"},
		},
		jobID:           uuid.New(),
		rejectDemandErr: errors.New("store unavailable"),
	}
	planner := &errPlanner{err: fmt.Errorf("%w: 项目员工池无法满足审查独立性约束", ErrNoSuitableEmployee)}
	activities := NewActivities(store, planner)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:      projectID,
			DemandID:       demandID,
			CreatedEventID: uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	// The reject-activity failure must NOT terminate the coordinator: the
	// shutdown signal is still processed and the workflow returns cleanly.
	require.NoError(t, env.GetWorkflowError())
	// The reject path was attempted (and retried per policy)...
	require.Contains(t, store.calls, "RejectDemandPlanning")
	// ...and its exhaustion degraded to the signal_failed audit event.
	require.Contains(t, store.appendEventTypes, "workflow.signal_failed")
}

func TestProjectCoordinatorDispatchesRootReadyReasonForRootTasks(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	rootTaskID := uuid.New()
	blockedTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "并行任务图",
				Content: "根任务完成后再处理下游",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:               uuid.New(),
		routeID:             uuid.New(),
		routeEventID:        uuid.New(),
		taskIDs:             []uuid.UUID{rootTaskID, blockedTaskID},
		dispatchableTaskIDs: []uuid.UUID{rootTaskID},
		dispatchEvent:       uuid.New(),
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
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
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"DecomposeAcceptedPlanRevision",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, []DispatchProjectTaskInput{{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         rootTaskID,
		DispatchReason: project.DispatchReasonRootReady,
	}}, store.dispatchInputs)
}

func TestProjectCoordinatorPausesDispatchWhenRouteRequiresHumanReview(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "高风险发布",
				Content: "触发高风险策略，需要负责人确认",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
			CoordinationPolicy: map[string]any{
				"require_human_review_for_new_demands": true,
			},
		},
		jobID:             uuid.New(),
		routeID:           uuid.New(),
		routeEventID:      uuid.New(),
		taskID:            uuid.New(),
		decisionRequestID: uuid.New(),
		dispatchEvent:     uuid.New(),
	}
	store.dispatchableTaskIDs = []uuid.UUID{store.taskID}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
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
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"RequestPlanRevisionReview",
	}, store.calls)
	require.Empty(t, store.dispatchInputs)
}

func TestProjectCoordinatorRoutesPlanReviewFromStoreWithoutPendingMap(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	decisionRequestID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	planRevisionID := uuid.New()
	routeEventID := uuid.New()
	planEventID := uuid.New()
	resolvedEventID := uuid.New()
	demandID := uuid.New()
	readyTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot:                  CoordinationSnapshot{ProjectID: projectID},
		dispatchableTaskIDBatches: [][]uuid.UUID{{readyTaskID}},
		humanDecisionRoutes: map[uuid.UUID]HumanDecisionRouteResult{
			decisionRequestID: {
				Decision: ProjectDecisionSnapshot{
					ID:             decisionRequestID,
					ProjectID:      projectID,
					DecisionType:   "plan_review",
					StatusSnapshot: "resolved",
					CreatedEventID: planEventID,
				},
				PlanReview: &PlanReviewRoute{
					ProjectID:         projectID,
					DemandID:          demandID,
					CoordinationJobID: coordinationJobID,
					RouteDecisionID:   routeDecisionID,
					PlanRevisionID:    planRevisionID,
					PlanFingerprint:   "fingerprint",
					Payload:           PlanRevisionPayload{Summary: "approved plan"},
					RouteEventID:      routeEventID,
					PlanEventID:       planEventID,
				},
			},
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			DecisionRequestID: decisionRequestID,
			Decision:          project.PlanReviewDecisionAccept,
			ResolvedEventID:   resolvedEventID,
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"LoadHumanDecisionRoute",
		"ResolvePlanRevisionReview",
		"DecomposeAcceptedPlanRevision",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
	require.Len(t, store.resolvePlanReviewInputs, 1)
	require.Equal(t, planRevisionID, store.resolvePlanReviewInputs[0].PlanRevisionID)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, readyTaskID, store.dispatchInputs[0].TaskID)
	require.Equal(t, project.DispatchReasonHumanResolved, store.dispatchInputs[0].DispatchReason)
	require.Len(t, store.finishJobInputs, 1)
	require.Equal(t, []uuid.UUID{routeEventID, planEventID, resolvedEventID}, store.finishJobInputs[0].OutputEventIDs)
}

func TestProjectCoordinatorRejectsPlanReviewRouteWithoutPlanReview(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	decisionRequestID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{ProjectID: projectID},
		humanDecisionRoutes: map[uuid.UUID]HumanDecisionRouteResult{
			decisionRequestID: {
				Decision: ProjectDecisionSnapshot{
					ID:             decisionRequestID,
					ProjectID:      projectID,
					DecisionType:   "plan_review",
					StatusSnapshot: "resolved",
				},
			},
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			DecisionRequestID: decisionRequestID,
			Decision:          project.PlanReviewDecisionAccept,
			ResolvedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	// A malformed plan_review route is a data-integrity fault, but it must not kill the
	// coordinator: the handler failure is recorded as an audit event and the loop survives
	// so later signals (and shutdown) are still processed.
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{"LoadHumanDecisionRoute", "AppendProjectEvent"}, store.calls)
}

func TestProjectCoordinatorDispatchesHumanResolvedTaskThroughGate(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	decisionRequestID := uuid.New()
	planRevisionID := uuid.New()
	planEventID := uuid.New()
	readyAfterApprovalID := uuid.New()
	staleReadyID := uuid.New()
	blockedTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "高风险发布",
				Content: "触发高风险策略，需要负责人确认",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
			CoordinationPolicy: map[string]any{
				"require_human_review_for_new_demands": true,
			},
		},
		jobID:                     uuid.New(),
		routeID:                   uuid.New(),
		routeEventID:              uuid.New(),
		planRevisionID:            planRevisionID,
		taskIDs:                   []uuid.UUID{staleReadyID, blockedTaskID},
		decisionRequestID:         decisionRequestID,
		dispatchableTaskIDBatches: [][]uuid.UUID{{readyAfterApprovalID}},
		dispatchEvent:             uuid.New(),
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		decisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             decisionRequestID,
				ProjectID:      store.snapshot.ProjectID,
				DecisionType:   "plan_review",
				StatusSnapshot: "resolved",
				CreatedEventID: planEventID,
			},
			PlanReview: &PlanReviewRoute{
				ProjectID:         store.snapshot.ProjectID,
				DemandID:          store.snapshot.Demand.ID,
				CoordinationJobID: store.jobID,
				RouteDecisionID:   store.routeID,
				PlanRevisionID:    planRevisionID,
				PlanFingerprint:   "fingerprint",
				Payload:           PlanRevisionPayload{Summary: "approved plan"},
			},
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: decisionRequestID,
			Decision:          "approved",
			ResolvedEventID:   uuid.New(),
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
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"RequestPlanRevisionReview",
		"LoadHumanDecisionRoute",
		"ResolvePlanRevisionReview",
		"DecomposeAcceptedPlanRevision",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, []DispatchProjectTaskInput{{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      store.snapshot.ProjectID,
		TaskID:         readyAfterApprovalID,
		DispatchReason: project.DispatchReasonHumanResolved,
	}}, store.dispatchInputs)
	_ = staleReadyID
	require.Len(t, store.listDispatchableInputs, 1)
	require.Equal(t, store.jobID, store.listDispatchableInputs[0].CoordinationJobID)
}

func TestProjectCoordinatorReplansAfterPlanReviewRequestChanges(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	decisionRequestID := uuid.New()
	secondDecisionRequestID := uuid.New()
	planRevisionID := uuid.New()
	secondPlanRevisionID := uuid.New()
	initialRouteEventID := uuid.New()
	initialPlanEventID := uuid.New()
	requestChangesEventID := uuid.New()
	replanRouteEventID := uuid.New()
	replanPlanEventID := uuid.New()
	finalResolvedEventID := uuid.New()
	readyTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "调整发布计划",
				Content: "负责人要求缩小上线范围后重新规划",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
			CoordinationPolicy: map[string]any{
				"require_human_review_for_new_demands": true,
			},
		},
		jobID:                        uuid.New(),
		routeID:                      uuid.New(),
		routeEventID:                 initialRouteEventID,
		planRevisionID:               planRevisionID,
		taskID:                       uuid.New(),
		decisionRequestID:            decisionRequestID,
		planRevisionIDs:              []uuid.UUID{planRevisionID, secondPlanRevisionID},
		planRevisionIDsForRoute:      []uuid.UUID{planRevisionID, secondPlanRevisionID},
		routeEventIDs:                []uuid.UUID{initialRouteEventID, replanRouteEventID},
		planEventIDs:                 []uuid.UUID{initialPlanEventID, replanPlanEventID},
		routeEventIDsForRoute:        []uuid.UUID{initialRouteEventID, replanRouteEventID},
		planEventIDsForRoute:         []uuid.UUID{initialPlanEventID, replanPlanEventID},
		planResolvedEventIDsForRoute: []uuid.UUID{requestChangesEventID},
		dispatchableTaskIDBatches: [][]uuid.UUID{
			{readyTaskID},
		},
		dispatchEvent: uuid.New(),
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		decisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             decisionRequestID,
				ProjectID:      store.snapshot.ProjectID,
				DecisionType:   "plan_review",
				StatusSnapshot: "resolved",
			},
			PlanReview: &PlanReviewRoute{
				ProjectID:         store.snapshot.ProjectID,
				DemandID:          store.snapshot.Demand.ID,
				CoordinationJobID: store.jobID,
				RouteDecisionID:   store.routeID,
				PlanRevisionID:    planRevisionID,
				PlanFingerprint:   "fingerprint",
				Payload:           PlanRevisionPayload{Summary: "original plan"},
				RouteEventID:      initialRouteEventID,
			},
		},
		secondDecisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             secondDecisionRequestID,
				ProjectID:      store.snapshot.ProjectID,
				DecisionType:   "plan_review",
				StatusSnapshot: "resolved",
				CreatedEventID: replanPlanEventID,
			},
			PlanReview: &PlanReviewRoute{
				ProjectID:         store.snapshot.ProjectID,
				DemandID:          store.snapshot.Demand.ID,
				CoordinationJobID: store.jobID,
				RouteDecisionID:   store.routeID,
				PlanRevisionID:    secondPlanRevisionID,
				PlanFingerprint:   "fingerprint",
				Payload:           PlanRevisionPayload{Summary: "replanned plan"},
				RouteEventID:      replanRouteEventID,
				PlanEventID:       replanPlanEventID,
			},
		},
	}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: decisionRequestID,
			Decision:          project.PlanReviewDecisionRequestChanges,
			Payload:           map[string]any{"comment": "缩小上线范围"},
			ResolvedEventID:   requestChangesEventID,
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: secondDecisionRequestID,
			Decision:          project.PlanReviewDecisionAccept,
			ResolvedEventID:   finalResolvedEventID,
		})
	}, 8*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 15*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"RequestPlanRevisionReview",
		"LoadHumanDecisionRoute",
		"ResolvePlanRevisionReview",
		"AppendProjectEvent",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"RequestPlanRevisionReview",
		"LoadHumanDecisionRoute",
		"ResolvePlanRevisionReview",
		"DecomposeAcceptedPlanRevision",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
	require.Len(t, store.persistPlanRevisionInputs, 2)
	require.False(t, store.persistPlanRevisionInputs[0].SupersedeOpen)
	require.True(t, store.persistPlanRevisionInputs[1].SupersedeOpen)
	require.NotNil(t, store.persistPlanRevisionInputs[1].SupersedeReason)
	require.Equal(t, "缩小上线范围", *store.persistPlanRevisionInputs[1].SupersedeReason)
	require.Len(t, store.requestPlanReviewInputs, 2)
	require.Len(t, store.resolvePlanReviewInputs, 2)
	require.Equal(t, planRevisionID, store.resolvePlanReviewInputs[0].PlanRevisionID)
	require.Equal(t, secondPlanRevisionID, store.resolvePlanReviewInputs[1].PlanRevisionID)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, readyTaskID, store.dispatchInputs[0].TaskID)
	require.Len(t, store.finishJobInputs, 1)
	require.Equal(t, []uuid.UUID{
		initialRouteEventID,
		initialPlanEventID,
		requestChangesEventID,
		replanRouteEventID,
		replanPlanEventID,
		finalResolvedEventID,
	}, store.finishJobInputs[0].OutputEventIDs)
}

// TestProjectCoordinatorRejectsDemandWhenReplanHasNoSuitableEmployee mirrors
// TestProjectCoordinatorRejectsDemandWhenNoSuitableEmployee for the replan path:
// an exit-reselect request_changes triggers a replan whose planner returns a
// terminal ErrNoSuitableEmployee. That failure must route through the demand
// rejection/diagnosis surface (RejectDemandPlanning) exactly once — not fall
// through to the generic signal_failed audit event, which is invisible to a
// human.
func TestProjectCoordinatorRejectsDemandWhenReplanHasNoSuitableEmployee(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	decisionRequestID := uuid.New()
	planRevisionID := uuid.New()
	initialRouteEventID := uuid.New()
	initialPlanEventID := uuid.New()
	requestChangesEventID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "改选发布出口",
				Content: "负责人改选到 release_record 后重新规划",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
			CoordinationPolicy: map[string]any{
				"require_human_review_for_new_demands": true,
			},
		},
		jobID:                   uuid.New(),
		routeID:                 uuid.New(),
		routeEventID:            initialRouteEventID,
		planRevisionID:          planRevisionID,
		taskID:                  uuid.New(),
		decisionRequestID:       decisionRequestID,
		planRevisionIDs:         []uuid.UUID{planRevisionID},
		planRevisionIDsForRoute: []uuid.UUID{planRevisionID},
		routeEventIDs:           []uuid.UUID{initialRouteEventID},
		planEventIDs:            []uuid.UUID{initialPlanEventID},
		routeEventIDsForRoute:   []uuid.UUID{initialRouteEventID},
		planEventIDsForRoute:    []uuid.UUID{initialPlanEventID},
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		decisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             decisionRequestID,
				ProjectID:      store.snapshot.ProjectID,
				DecisionType:   "plan_review",
				StatusSnapshot: "resolved",
			},
			PlanReview: &PlanReviewRoute{
				ProjectID:         store.snapshot.ProjectID,
				DemandID:          store.snapshot.Demand.ID,
				CoordinationJobID: store.jobID,
				RouteDecisionID:   store.routeID,
				PlanRevisionID:    planRevisionID,
				PlanFingerprint:   "fingerprint",
				Payload:           PlanRevisionPayload{Summary: "original plan"},
				RouteEventID:      initialRouteEventID,
			},
		},
	}
	planner := &sequencedPlanner{
		inner:    HeuristicRoutePlanner{},
		failOn:   2,
		failWith: fmt.Errorf("%w: task %q: employee scored 0.50", ErrNoSuitableEmployee, "test"),
	}
	activities := NewActivities(store, planner)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID:     uuid.New(),
			DecisionRequestID:     decisionRequestID,
			Decision:              project.PlanReviewDecisionRequestChanges,
			TargetExitDeliverable: "release_record",
			Payload:               map[string]any{"comment": "改选到发布出口"},
			ResolvedEventID:       requestChangesEventID,
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 12*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	// Two planner calls only: the initial plan, then the replan whose family
	// error is non-retryable.
	require.Equal(t, int32(2), planner.calls.Load())
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"RequestPlanRevisionReview",
		"LoadHumanDecisionRoute",
		"ResolvePlanRevisionReview",
		"AppendProjectEvent",
		"LoadProjectCoordinationSnapshot",
		"RejectDemandPlanning",
	}, store.calls)
	// The terminal replan failure never falls through to the generic
	// signal_failed audit event.
	require.NotContains(t, store.appendEventTypes, "workflow.signal_failed")
	require.Len(t, store.rejectDemandInputs, 1)
	require.Equal(t, store.snapshot.Demand.ID, store.rejectDemandInputs[0].DemandID)
	require.Equal(t, store.jobID, store.rejectDemandInputs[0].CoordinationJobID)
	require.Contains(t, store.rejectDemandInputs[0].Diagnosis, "scored 0.50")
}

// TestPlanningGapRestaffedTriggersReplan proves the planning_gap decision loop
// closes: an initial demand whose route cannot be planned terminally rejects
// (RejectDemandPlanning), and when the human resolves the resulting planning_gap
// decision with restaffed, the coordinator reopens the demand (failed→planning_pending
// via the ReopenProjectDemandForReplanning activity) and replans it in place — a
// second planner call runs and a fresh plan revision review opens, with no
// signal_failed audit event.
func TestPlanningGapRestaffedTriggersReplan(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	planningGapDecisionID := uuid.New()
	restaffedEventID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: projectID,
			Demand: DemandSnapshot{
				ID:      demandID,
				Title:   "补员后重新规划",
				Content: "负责人补充员工后重开需求",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
			CoordinationPolicy: map[string]any{
				"require_human_review_for_new_demands": true,
			},
		},
		jobID:              uuid.New(),
		routeID:            uuid.New(),
		routeEventID:       uuid.New(),
		planRevisionID:     uuid.New(),
		decisionRequestID:  uuid.New(),
		planRevisionStatus: "pending_review",
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		planningGapDecisionID: {
			Decision: ProjectDecisionSnapshot{
				ID:             planningGapDecisionID,
				ProjectID:      projectID,
				DecisionType:   "planning_gap",
				StatusSnapshot: "restaffed",
			},
			PlanningGap: &PlanningGapRoute{ProjectID: projectID, DemandID: demandID},
		},
	}
	// failOn:1 — the initial plan fails structurally (→ RejectDemandPlanning), the
	// post-restaffed replan (call 2) succeeds.
	planner := &sequencedPlanner{
		inner:    HeuristicRoutePlanner{},
		failOn:   1,
		failWith: fmt.Errorf("%w: task %q: employee scored 0.40", ErrNoSuitableEmployee, "review"),
	}
	activities := NewActivities(store, planner)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         projectID,
			DemandID:          demandID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: planningGapDecisionID,
			Decision:          "restaffed",
			ResolvedEventID:   restaffedEventID,
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 12*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	// Two planner calls: the failed initial plan, then the successful replan.
	require.Equal(t, int32(2), planner.calls.Load())
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"RejectDemandPlanning",
		"LoadHumanDecisionRoute",
		"ReopenProjectDemandForReplanning",
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"RequestPlanRevisionReview",
	}, store.calls)
	require.Len(t, store.reopenDemandInputs, 1)
	require.Equal(t, demandID, store.reopenDemandInputs[0].DemandID)
	require.Equal(t, projectID, store.reopenDemandInputs[0].ProjectID)
	require.NotContains(t, store.appendEventTypes, "workflow.signal_failed")
}

// exitCapturingRoutePlanner wraps another RoutePlanner and records every
// CoordinationSnapshot it is asked to plan for, so tests can assert what
// PinnedExitDeliverable looked like on each planning pass (initial vs replan).
type exitCapturingRoutePlanner struct {
	inner     RoutePlanner
	snapshots []CoordinationSnapshot
}

func (p *exitCapturingRoutePlanner) Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	p.snapshots = append(p.snapshots, snapshot)
	return p.inner.Plan(ctx, snapshot)
}

func TestReplanAfterExitOverridePinsExit(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	decisionRequestID := uuid.New()
	secondDecisionRequestID := uuid.New()
	planRevisionID := uuid.New()
	secondPlanRevisionID := uuid.New()
	initialRouteEventID := uuid.New()
	initialPlanEventID := uuid.New()
	requestChangesEventID := uuid.New()
	replanRouteEventID := uuid.New()
	replanPlanEventID := uuid.New()
	finalResolvedEventID := uuid.New()
	readyTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "调整发布计划",
				Content: "负责人改选出口后重新规划",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
			CoordinationPolicy: map[string]any{
				"require_human_review_for_new_demands": true,
			},
		},
		jobID:                        uuid.New(),
		routeID:                      uuid.New(),
		routeEventID:                 initialRouteEventID,
		planRevisionID:               planRevisionID,
		taskID:                       uuid.New(),
		decisionRequestID:            decisionRequestID,
		planRevisionIDs:              []uuid.UUID{planRevisionID, secondPlanRevisionID},
		planRevisionIDsForRoute:      []uuid.UUID{planRevisionID, secondPlanRevisionID},
		routeEventIDs:                []uuid.UUID{initialRouteEventID, replanRouteEventID},
		planEventIDs:                 []uuid.UUID{initialPlanEventID, replanPlanEventID},
		routeEventIDsForRoute:        []uuid.UUID{initialRouteEventID, replanRouteEventID},
		planEventIDsForRoute:         []uuid.UUID{initialPlanEventID, replanPlanEventID},
		planResolvedEventIDsForRoute: []uuid.UUID{requestChangesEventID},
		dispatchableTaskIDBatches: [][]uuid.UUID{
			{readyTaskID},
		},
		dispatchEvent: uuid.New(),
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		decisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             decisionRequestID,
				ProjectID:      store.snapshot.ProjectID,
				DecisionType:   "plan_review",
				StatusSnapshot: "resolved",
			},
			PlanReview: &PlanReviewRoute{
				ProjectID:         store.snapshot.ProjectID,
				DemandID:          store.snapshot.Demand.ID,
				CoordinationJobID: store.jobID,
				RouteDecisionID:   store.routeID,
				PlanRevisionID:    planRevisionID,
				PlanFingerprint:   "fingerprint",
				Payload:           PlanRevisionPayload{Summary: "original plan"},
				RouteEventID:      initialRouteEventID,
			},
		},
		secondDecisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             secondDecisionRequestID,
				ProjectID:      store.snapshot.ProjectID,
				DecisionType:   "plan_review",
				StatusSnapshot: "resolved",
				CreatedEventID: replanPlanEventID,
			},
			PlanReview: &PlanReviewRoute{
				ProjectID:         store.snapshot.ProjectID,
				DemandID:          store.snapshot.Demand.ID,
				CoordinationJobID: store.jobID,
				RouteDecisionID:   store.routeID,
				PlanRevisionID:    secondPlanRevisionID,
				PlanFingerprint:   "fingerprint",
				Payload:           PlanRevisionPayload{Summary: "replanned plan"},
				RouteEventID:      replanRouteEventID,
				PlanEventID:       replanPlanEventID,
			},
		},
	}
	planner := &exitCapturingRoutePlanner{inner: HeuristicRoutePlanner{}}
	activities := NewActivities(store, planner)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID:     uuid.New(),
			DecisionRequestID:     decisionRequestID,
			Decision:              project.PlanReviewDecisionRequestChanges,
			Payload:               map[string]any{"comment": "改选交付出口"},
			TargetExitDeliverable: "branch_ref",
			ResolvedEventID:       requestChangesEventID,
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: secondDecisionRequestID,
			Decision:          project.PlanReviewDecisionAccept,
			ResolvedEventID:   finalResolvedEventID,
		})
	}, 8*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 15*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Len(t, planner.snapshots, 2, "expected an initial plan pass and a replan pass")
	require.Equal(t, "", planner.snapshots[0].PinnedExitDeliverable, "initial plan must not carry a pin")
	require.Equal(t, "branch_ref", planner.snapshots[1].PinnedExitDeliverable, "replan snapshot must carry the human's chosen exit as a pin")
}

func TestProjectCoordinatorDispatchesDependencyUnlockedReasonOnCompletion(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	rootTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "任务图执行",
				Content: "根任务完成后唤醒下游",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
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
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
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
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"DecomposeAcceptedPlanRevision",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"ResolveReadyDownstream",
		"DispatchProjectTask",
	}, store.calls)
	require.Len(t, store.resolveReadyInputs, 1)
	require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
	require.Equal(t, []DispatchProjectTaskInput{
		{TenantID: store.dispatchInputs[0].TenantID, ProjectID: store.snapshot.ProjectID, TaskID: rootTaskID, DispatchReason: project.DispatchReasonRootReady},
		{TenantID: store.dispatchInputs[0].TenantID, ProjectID: store.snapshot.ProjectID, TaskID: downstreamTaskID, DispatchReason: project.DispatchReasonDependencyUnlocked},
	}, store.dispatchInputs)
}

func TestProjectCoordinatorCreatesRevisionTaskOnRevisionResult(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	revisionTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot:       CoordinationSnapshot{ProjectID: projectID},
		dispatchEvent:  uuid.New(),
		resultDecision: InspectTaskResultDecisionResult{ResultID: resultID, Decision: string(project.TaskResultDecisionRevisionAttempt)},
		revisionResult: CreateRevisionTaskForResultResult{TaskID: revisionTaskID},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"CreateRevisionTaskForResult",
		"DispatchProjectTask",
	}, store.calls)
	require.Len(t, store.inspectResultInputs, 1)
	require.Equal(t, sourceTaskID, store.inspectResultInputs[0].ProjectTaskID)
	require.Len(t, store.createRevisionInputs, 1)
	require.Equal(t, CreateRevisionTaskForResultInput{
		TenantID:     store.createRevisionInputs[0].TenantID,
		ProjectID:    projectID,
		SourceTaskID: sourceTaskID,
		ResultID:     resultID,
	}, store.createRevisionInputs[0])
	require.Equal(t, []DispatchProjectTaskInput{{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      projectID,
		TaskID:         revisionTaskID,
		DispatchReason: project.DispatchReasonRetry,
	}}, store.dispatchInputs)
	require.Empty(t, store.resolveReadyInputs)
}

func TestProjectCoordinatorSupplementsUpstreamOwnerForResolvableBlockedResult(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	ownerSupplementTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{ProjectID: projectID},
		resultDecision: InspectTaskResultDecisionResult{
			Decision:         string(project.TaskResultDecisionBlockedResolvableUpstream),
			CoordinationMode: project.CoordinationModeLoop,
			Blocker:          &project.TaskResultBlocker{MissingInputs: []string{"load_test_report"}},
		},
		upstreamSupplementResult: CreateUpstreamSupplementResult{TaskIDs: []uuid.UUID{ownerSupplementTaskID}},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"CreateUpstreamSupplementTasks",
		"DispatchProjectTask",
	}, store.calls)
	require.Equal(t, []CreateUpstreamSupplementInput{{
		TenantID:      store.upstreamSupplementInputs[0].TenantID,
		ProjectID:     projectID,
		SourceTaskID:  sourceTaskID,
		MissingInputs: []string{"load_test_report"},
	}}, store.upstreamSupplementInputs)
	require.Equal(t, []DispatchProjectTaskInput{{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      projectID,
		TaskID:         ownerSupplementTaskID,
		DispatchReason: project.DispatchReasonRetry,
	}}, store.dispatchInputs)
	require.Empty(t, store.resolveReadyInputs)
}

func TestProjectCoordinatorRequestsHumanDecisionWhenUpstreamSupplementExhausted(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	decisionRequestID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{ProjectID: projectID},
		resultDecision: InspectTaskResultDecisionResult{
			ResultID:         resultID,
			Decision:         string(project.TaskResultDecisionBlockedResolvableUpstream),
			CoordinationMode: project.CoordinationModeLoop,
			Blocker:          &project.TaskResultBlocker{MissingInputs: []string{"load_test_report"}},
		},
		upstreamSupplementResult:     CreateUpstreamSupplementResult{Exhausted: true},
		iterationExhaustedDecisionID: decisionRequestID,
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"CreateUpstreamSupplementTasks",
		"RequestProjectTaskIterationExhaustedReview",
	}, store.calls)
	require.Len(t, store.iterationExhaustedInputs, 1)
	require.Equal(t, sourceTaskID, store.iterationExhaustedInputs[0].ProjectTaskID)
	require.Equal(t, resultID, store.iterationExhaustedInputs[0].ResultID)
	require.Equal(t, "iteration_exhausted", store.iterationExhaustedInputs[0].Reason)
	require.Empty(t, store.dispatchInputs)
}

func TestProjectCoordinatorRequestsSupplementReviewInPlanMode(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	reviewDecisionID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{ProjectID: projectID},
		resultDecision: InspectTaskResultDecisionResult{
			ResultID:         resultID,
			Decision:         string(project.TaskResultDecisionBlockedResolvableUpstream),
			CoordinationMode: project.CoordinationModePlan,
			Blocker:          &project.TaskResultBlocker{MissingInputs: []string{"load_test_report"}},
		},
		upstreamSupplementReviewDecisionID: reviewDecisionID,
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"RequestUpstreamSupplementReview",
	}, store.calls)
	require.Len(t, store.upstreamSupplementReviewInputs, 1)
	require.Equal(t, sourceTaskID, store.upstreamSupplementReviewInputs[0].ProjectTaskID)
	require.Equal(t, resultID, store.upstreamSupplementReviewInputs[0].ResultID)
	require.Equal(t, []string{"load_test_report"}, store.upstreamSupplementReviewInputs[0].MissingInputs)
	require.Empty(t, store.upstreamSupplementInputs)
	require.Empty(t, store.dispatchInputs)
}

func TestProjectCoordinatorAutoSupplementsInLoopMode(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	ownerSupplementTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{ProjectID: projectID},
		resultDecision: InspectTaskResultDecisionResult{
			Decision:         string(project.TaskResultDecisionBlockedResolvableUpstream),
			CoordinationMode: project.CoordinationModeLoop,
			Blocker:          &project.TaskResultBlocker{MissingInputs: []string{"load_test_report"}},
		},
		upstreamSupplementResult: CreateUpstreamSupplementResult{TaskIDs: []uuid.UUID{ownerSupplementTaskID}},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"CreateUpstreamSupplementTasks",
		"DispatchProjectTask",
	}, store.calls)
	require.Equal(t, []CreateUpstreamSupplementInput{{
		TenantID:      store.upstreamSupplementInputs[0].TenantID,
		ProjectID:     projectID,
		SourceTaskID:  sourceTaskID,
		MissingInputs: []string{"load_test_report"},
	}}, store.upstreamSupplementInputs)
	require.Equal(t, []DispatchProjectTaskInput{{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      projectID,
		TaskID:         ownerSupplementTaskID,
		DispatchReason: project.DispatchReasonRetry,
	}}, store.dispatchInputs)
	require.Empty(t, store.upstreamSupplementReviewInputs)
}

func TestProjectCoordinatorDispatchesSupplementAfterApproval(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	reviewDecisionID := uuid.New()
	replacementTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{ProjectID: projectID},
		resultDecision: InspectTaskResultDecisionResult{
			ResultID:         resultID,
			Decision:         string(project.TaskResultDecisionBlockedResolvableUpstream),
			CoordinationMode: project.CoordinationModePlan,
			Blocker:          &project.TaskResultBlocker{MissingInputs: []string{"load_test_report"}},
		},
		upstreamSupplementReviewDecisionID: reviewDecisionID,
		failureRecoveryResult:              ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{replacementTaskID}},
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		reviewDecisionID: {
			Decision: ProjectDecisionSnapshot{
				ID:             reviewDecisionID,
				ProjectID:      projectID,
				DecisionType:   "upstream_supplement_review",
				StatusSnapshot: "resolved",
				ProjectTaskID:  sourceTaskID,
			},
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: reviewDecisionID,
			Decision:          "approved",
			Payload:           map[string]any{"review_note": "supplement approved"},
			ResolvedEventID:   uuid.New(),
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	tenantID := uuid.New()
	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   tenantID,
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"RequestUpstreamSupplementReview",
		"LoadHumanDecisionRoute",
		"ApplyFailureRecoveryDecision",
		"DispatchProjectTask",
	}, store.calls)
	require.Equal(t, []DispatchProjectTaskInput{{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         replacementTaskID,
		DispatchReason: project.DispatchReasonRetry,
	}}, store.dispatchInputs)
}

func TestProjectCoordinatorRequestsHumanDecisionWhenRevisionIterationExhausted(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	resultID := uuid.New()
	decisionRequestID := uuid.New()
	store := &recordingActivityStore{
		snapshot:                     CoordinationSnapshot{ProjectID: projectID},
		resultDecision:               InspectTaskResultDecisionResult{ResultID: resultID, Decision: string(project.TaskResultDecisionRevisionAttempt)},
		revisionResult:               CreateRevisionTaskForResultResult{Exhausted: true},
		iterationExhaustedDecisionID: decisionRequestID,
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"InspectTaskResultDecision",
		"CreateRevisionTaskForResult",
		"RequestProjectTaskIterationExhaustedReview",
	}, store.calls)
	require.Len(t, store.iterationExhaustedInputs, 1)
	require.Equal(t, sourceTaskID, store.iterationExhaustedInputs[0].ProjectTaskID)
	require.Equal(t, resultID, store.iterationExhaustedInputs[0].ResultID)
	require.Equal(t, "iteration_exhausted", store.iterationExhaustedInputs[0].Reason)
	require.Equal(t, "同一失败重复出现，需要人类判断是否继续", store.iterationExhaustedInputs[0].Summary)
}

func TestProjectCoordinatorDoesNotUnlockDownstreamForHumanReviewResult(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	sourceTaskID := uuid.New()
	downstreamTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot:           CoordinationSnapshot{ProjectID: projectID},
		dispatchEvent:      uuid.New(),
		readyDownstreamIDs: []uuid.UUID{downstreamTaskID},
		resultDecision:     InspectTaskResultDecisionResult{ResultID: uuid.New(), Decision: string(project.TaskResultDecisionWaitingHumanReview)},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
			ProjectTaskID:      sourceTaskID,
			ExecutionSummaryID: uuid.New(),
			CompletedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{"AppendProjectEvent", "InspectTaskResultDecision"}, store.calls)
	require.Empty(t, store.resolveReadyInputs)
	require.Empty(t, store.dispatchInputs)
}

func TestProjectCoordinatorRedispatchesApprovedPreDispatchGateDecision(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	taskID := uuid.New()
	decisionRequestID := uuid.New()
	store := &recordingActivityStore{
		snapshot:           CoordinationSnapshot{ProjectID: projectID},
		dispatchEvent:      uuid.New(),
		gateDecisionTaskID: &taskID,
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		decisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             decisionRequestID,
				ProjectID:      projectID,
				DecisionType:   "project_task_approval",
				StatusSnapshot: "resolved",
				ProjectTaskID:  taskID,
			},
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: decisionRequestID,
			Decision:          "approved",
			ResolvedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{"LoadHumanDecisionRoute", "ApplyPreDispatchGateDecision", "DispatchProjectTask"}, store.calls)
	require.Len(t, store.applyPreDispatchGateInputs, 1)
	require.Equal(t, decisionRequestID, store.applyPreDispatchGateInputs[0].DecisionRequestID)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, DispatchProjectTaskInput{
		TenantID:       store.dispatchInputs[0].TenantID,
		ProjectID:      projectID,
		TaskID:         taskID,
		DispatchReason: project.DispatchReasonHumanResolved,
	}, store.dispatchInputs[0])
}

func TestProjectCoordinatorKeepsNonGateDecisionObservedOnly(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	decisionRequestID := uuid.New()
	store := &recordingActivityStore{
		snapshot:      CoordinationSnapshot{ProjectID: projectID},
		dispatchEvent: uuid.New(),
		unknownHumanDecisionRouteIDs: map[uuid.UUID]bool{
			decisionRequestID: true,
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: decisionRequestID,
			Decision:          "approved",
			ResolvedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Empty(t, store.dispatchInputs)
	require.Equal(t, []string{"LoadHumanDecisionRoute", "AppendProjectEvent"}, store.calls)
}

func TestProjectCoordinatorRequestsAcceptanceReviewAndAppliesDecision(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	rootTaskID := uuid.New()
	acceptanceID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand:    DemandSnapshot{ID: uuid.New(), Title: "验收触发", Content: "全部完成后进入验收"},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:                       uuid.New(),
		routeID:                     uuid.New(),
		routeEventID:                uuid.New(),
		taskIDs:                     []uuid.UUID{rootTaskID},
		dispatchableTaskIDs:         []uuid.UUID{rootTaskID},
		readyDownstreamIDs:          nil,
		resultDecision:              InspectTaskResultDecisionResult{ResultID: uuid.New(), Decision: string(project.TaskResultDecisionCompleteAccepted)},
		dispatchEvent:               uuid.New(),
		acceptanceReady:             true,
		acceptanceDecisionRequestID: acceptanceID,
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		acceptanceID: {
			Decision: ProjectDecisionSnapshot{
				ID:             acceptanceID,
				ProjectID:      store.snapshot.ProjectID,
				DecisionType:   "project_acceptance",
				StatusSnapshot: "resolved",
			},
		},
	}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
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
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			DecisionRequestID: acceptanceID,
			Decision:          "accepted",
			ResolvedEventID:   uuid.New(),
		})
	}, 10*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 15*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Contains(t, store.calls, "IsProjectAcceptanceReady")
	require.Contains(t, store.calls, "RequestProjectAcceptanceReview")
	require.Contains(t, store.calls, "LoadHumanDecisionRoute")
	require.Len(t, store.acceptanceReviewInputs, 1)
	require.Equal(t, store.snapshot.ProjectID, store.acceptanceReviewInputs[0].ProjectID)
	require.Len(t, store.applyAcceptanceInputs, 1)
	require.Equal(t, "accepted", store.applyAcceptanceInputs[0].Decision)
}

func TestProjectCoordinatorRequestsFailureRecoveryWhenTaskFails(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	failedTaskID := uuid.New()
	failedEventID := uuid.New()
	decisionRequestID := uuid.New()
	store := &recordingActivityStore{
		snapshot:                  CoordinationSnapshot{ProjectID: projectID},
		failureRecoveryDecisionID: decisionRequestID,
		dispatchEvent:             uuid.New(),
	}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskFailed, EmployeeTaskFailed{
			ProjectTaskID:  failedTaskID,
			FailureSummary: "runtime execution failed",
			FailedEventID:  failedEventID,
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	tenantID := uuid.New()
	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   tenantID,
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"HoldDownstreamForFailure",
	}, store.calls)
	require.Len(t, store.holdFailureInputs, 1)
	require.Equal(t, HoldDownstreamForFailureInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		FailedTaskID:   failedTaskID,
		FailureSummary: "runtime execution failed",
		FailedEventID:  failedEventID,
	}, store.holdFailureInputs[0])
}

func TestProjectCoordinatorDispatchesRetryReasonAfterHumanRecoveryDecision(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	failedTaskID := uuid.New()
	replacementTaskID := uuid.New()
	decisionRequestID := uuid.New()
	store := &recordingActivityStore{
		snapshot:                  CoordinationSnapshot{ProjectID: projectID},
		failureRecoveryDecisionID: decisionRequestID,
		failureRecoveryResult:     ApplyFailureRecoveryDecisionResult{ReadyTaskIDs: []uuid.UUID{replacementTaskID}},
		dispatchEvent:             uuid.New(),
	}
	store.humanDecisionRoutes = map[uuid.UUID]HumanDecisionRouteResult{
		decisionRequestID: {
			Decision: ProjectDecisionSnapshot{
				ID:             decisionRequestID,
				ProjectID:      projectID,
				DecisionType:   "task_failure_recovery",
				StatusSnapshot: "resolved",
				ProjectTaskID:  failedTaskID,
			},
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalEmployeeTaskFailed, EmployeeTaskFailed{
			ProjectTaskID:  failedTaskID,
			FailureSummary: "runtime execution failed",
			FailedEventID:  uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: decisionRequestID,
			Decision:          "approved",
			Payload:           map[string]any{"review_note": "retry after operator approval"},
			ResolvedEventID:   uuid.New(),
		})
	}, 5*time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	tenantID := uuid.New()
	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   tenantID,
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"AppendProjectEvent",
		"HoldDownstreamForFailure",
		"LoadHumanDecisionRoute",
		"ApplyFailureRecoveryDecision",
		"DispatchProjectTask",
	}, store.calls)
	require.Equal(t, []DispatchProjectTaskInput{{
		TenantID:       tenantID,
		ProjectID:      projectID,
		TaskID:         replacementTaskID,
		DispatchReason: project.DispatchReasonRetry,
	}}, store.dispatchInputs)
	require.Len(t, store.applyFailureRecoveryInputs, 1)
	require.Equal(t, ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionRequestID,
		Decision:          "approved",
		Payload:           map[string]any{"review_note": "retry after operator approval"},
	}, store.applyFailureRecoveryInputs[0])
}

func TestProjectCoordinatorSurvivesUnrecordedDispatchFailure(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "验证 Runtime",
				Content: "检查心跳",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:         uuid.New(),
		routeID:       uuid.New(),
		routeEventID:  uuid.New(),
		taskID:        uuid.New(),
		dispatchEvent: uuid.New(),
		dispatchErr:   errors.New("append dispatch failure event failed"),
	}
	store.dispatchableTaskIDs = []uuid.UUID{store.taskID}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	// An unrecorded dispatch failure aborts this demand (the job is left unfinished) but
	// must not terminate the coordinator: the failure is captured as an audit event and the
	// loop survives to process the shutdown signal.
	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.NotContains(t, store.calls, "FinishCoordinationJob")
	require.Contains(t, store.calls, "AppendProjectEvent")
}

func TestProjectCoordinatorContinuesAfterRecordedDispatchFailure(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "验证 Runtime",
				Content: "检查心跳",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:         uuid.New(),
		routeID:       uuid.New(),
		routeEventID:  uuid.New(),
		taskID:        uuid.New(),
		dispatchEvent: uuid.New(),
		dispatchErr:   &ProjectTaskDispatchError{FailureRecorded: true, Err: project.ErrInvalidProject},
	}
	store.dispatchableTaskIDs = []uuid.UUID{store.taskID}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
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
	require.Contains(t, store.calls, "RecoverTaskDispatchFailure")
	require.Contains(t, store.calls, "FinishCoordinationJob")
}

func TestProjectCoordinatorRedispatchesAfterRetryBackoff(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	retryAt := time.Now().Add(2 * time.Minute)
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "验证 Runtime",
				Content: "检查心跳",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:         uuid.New(),
		routeID:       uuid.New(),
		routeEventID:  uuid.New(),
		taskID:        uuid.New(),
		dispatchEvent: uuid.New(),
		dispatchErr:   &ProjectTaskDispatchError{FailureRecorded: true, Err: project.ErrInvalidProject},
		recoverResults: []RecoverTaskDispatchFailureResult{
			{Action: project.ProjectTaskRecoveryActionRetryScheduled, RetryNotBefore: &retryAt},
			{Action: project.ProjectTaskRecoveryActionWaitingHuman},
		},
	}
	store.dispatchableTaskIDs = []uuid.UUID{store.taskID}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Minute)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Len(t, store.recoverInputs, 2)
}

func TestProjectCoordinatorContinuesAsNewWhenSuggestedAfterSignal(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetContinueAsNewSuggested(true)
	projectID := uuid.New()
	store := &recordingActivityStore{
		snapshot:      CoordinationSnapshot{ProjectID: projectID},
		dispatchEvent: uuid.New(),
	}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalProjectPolicyChanged, ProjectPolicyChanged{
			ProjectID:        projectID,
			ConfigRevisionID: uuid.New(),
			ChangedEventID:   uuid.New(),
		})
	}, time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
		Generation: 3,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "continue as new")
	require.Equal(t, []string{"AppendProjectEvent"}, store.calls)
}

func TestActivitiesDispatchProjectTaskWrapsTerminalErrorAsNonRetryable(t *testing.T) {
	store := &recordingActivityStore{dispatchErr: &ProjectTaskDispatchError{FailureRecorded: true, Err: project.ErrInvalidProject}}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	err := activities.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: uuid.New(), ProjectID: uuid.New(), TaskID: uuid.New()})
	var appErr *temporal.ApplicationError
	if !errors.As(err, &appErr) || !appErr.NonRetryable() {
		t.Fatalf("expected non-retryable application error, got %#v", err)
	}
	require.Equal(t, "ProjectTaskDispatchTerminal", appErr.Type())
	require.True(t, errors.Is(err, project.ErrInvalidProject) || errors.Is(appErr.Unwrap(), project.ErrInvalidProject))
}

func TestActivitiesDispatchProjectTaskKeepsTransientErrorRetryable(t *testing.T) {
	store := &recordingActivityStore{dispatchErr: errors.New("db timeout")}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	err := activities.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: uuid.New(), ProjectID: uuid.New(), TaskID: uuid.New()})
	if err == nil {
		t.Fatal("expected error")
	}
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.NonRetryable() {
		t.Fatalf("expected retryable error, got non-retryable %#v", err)
	}
}

func TestActivitiesDispatchProjectTaskDefaultsDispatchReason(t *testing.T) {
	store := &recordingActivityStore{}
	activities := NewActivities(store, HeuristicRoutePlanner{})

	err := activities.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: uuid.New(), ProjectID: uuid.New(), TaskID: uuid.New()})
	require.NoError(t, err)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, project.DispatchReasonRootReady, store.dispatchInputs[0].DispatchReason)
}

func TestActivitiesDispatchProjectTaskKeepsRetryLaterGateRetryable(t *testing.T) {
	store := &recordingActivityStore{dispatchErr: ErrProjectTaskDispatchRetryLater}
	activities := NewActivities(store, HeuristicRoutePlanner{})

	err := activities.DispatchProjectTask(context.Background(), DispatchProjectTaskInput{TenantID: uuid.New(), ProjectID: uuid.New(), TaskID: uuid.New()})
	require.ErrorIs(t, err, ErrProjectTaskDispatchRetryLater)
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.NonRetryable() {
		t.Fatalf("expected retryable retry-later error, got non-retryable %#v", err)
	}
}

type recordingActivityStore struct {
	calls                        []string
	snapshot                     CoordinationSnapshot
	jobID                        uuid.UUID
	routeID                      uuid.UUID
	routeEventID                 uuid.UUID
	routeEventIDs                []uuid.UUID
	planRevisionID               uuid.UUID
	planRevisionIDs              []uuid.UUID
	planRevisionIDsForRoute      []uuid.UUID
	planEventIDs                 []uuid.UUID
	routeEventIDsForRoute        []uuid.UUID
	planEventIDsForRoute         []uuid.UUID
	planResolvedEventIDsForRoute []uuid.UUID
	planFingerprint              string
	planRevisionStatus           string
	taskID                       uuid.UUID
	taskIDs                      []uuid.UUID
	decisionRequestID            uuid.UUID
	failureRecoveryDecisionID    uuid.UUID
	dispatchEvent                uuid.UUID
	dispatchErr                  error
	createJobErr                 error
	recoverInputs                []RecoverTaskDispatchFailureInput
	recoverResults               []RecoverTaskDispatchFailureResult
	recoverErr                   error

	dispatchableTaskIDs                []uuid.UUID
	dispatchableTaskIDBatches          [][]uuid.UUID
	readyDownstreamIDs                 []uuid.UUID
	resultDecision                     InspectTaskResultDecisionResult
	revisionResult                     CreateRevisionTaskForResultResult
	upstreamSupplementResult           CreateUpstreamSupplementResult
	failureRecoveryResult              ApplyFailureRecoveryDecisionResult
	iterationExhaustedDecisionID       uuid.UUID
	listDispatchableInputs             []ListDispatchableTasksInput
	resolveReadyInputs                 []ResolveReadyDownstreamInput
	inspectResultInputs                []InspectTaskResultDecisionInput
	createRevisionInputs               []CreateRevisionTaskForResultInput
	upstreamSupplementInputs           []CreateUpstreamSupplementInput
	iterationExhaustedInputs           []RequestProjectTaskIterationExhaustedReviewInput
	upstreamSupplementReviewInputs     []RequestUpstreamSupplementReviewInput
	upstreamSupplementReviewDecisionID uuid.UUID
	acceptanceReady                    bool
	acceptanceDecisionRequestID        uuid.UUID
	acceptanceReviewInputs             []RequestProjectAcceptanceReviewInput
	applyAcceptanceInputs              []ApplyProjectAcceptanceDecisionInput
	applyAcceptanceErr                 error
	holdFailureInputs                  []HoldDownstreamForFailureInput
	applyFailureRecoveryInputs         []ApplyFailureRecoveryDecisionInput
	applyPreDispatchGateInputs         []ApplyPreDispatchGateDecisionInput
	dispatchInputs                     []DispatchProjectTaskInput
	gateDecisionTaskID                 *uuid.UUID
	humanDecisionRoutes                map[uuid.UUID]HumanDecisionRouteResult
	unknownHumanDecisionRouteIDs       map[uuid.UUID]bool
	persistPlanRevisionInputs          []PersistPlanRevisionInput
	requestPlanReviewInputs            []RequestPlanRevisionReviewInput
	resolvePlanReviewInputs            []ResolvePlanRevisionReviewInput
	decomposePlanInputs                []DecomposeAcceptedPlanRevisionInput
	finishJobInputs                    []FinishCoordinationJobInput
	rejectDemandInputs                 []RejectDemandPlanningInput
	rejectDemandErr                    error
	reopenDemandInputs                 []ReopenProjectDemandForReplanningInput
	reopenDemandErr                    error
	appendEventTypes                   []string
}

type rawDispatchWorkflowActivities struct {
	*Activities
	store *recordingActivityStore
}

func newRawDispatchWorkflowActivities(store *recordingActivityStore) *rawDispatchWorkflowActivities {
	return &rawDispatchWorkflowActivities{
		Activities: NewActivities(store, HeuristicRoutePlanner{}),
		store:      store,
	}
}

func (a *rawDispatchWorkflowActivities) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	err := a.store.DispatchProjectTask(ctx, input)
	if err != nil && !dispatchErrorRetryable(err) {
		return temporal.NewNonRetryableApplicationError("project task dispatch rejected", "ProjectTaskDispatchTerminal", err)
	}
	return err
}

func (s *recordingActivityStore) LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error) {
	s.calls = append(s.calls, "LoadProjectCoordinationSnapshot")
	return s.snapshot, nil
}

func (s *recordingActivityStore) CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error) {
	s.calls = append(s.calls, "CreateCoordinationJob")
	if s.createJobErr != nil {
		return CoordinationJobResult{}, s.createJobErr
	}
	return CoordinationJobResult{ID: s.jobID}, nil
}

func (s *recordingActivityStore) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	s.calls = append(s.calls, "PersistRouteDecision")
	createdEventID := s.routeEventID
	if len(s.routeEventIDs) > 0 {
		createdEventID = s.routeEventIDs[0]
		s.routeEventIDs = s.routeEventIDs[1:]
	}
	return RouteDecisionResult{ID: s.routeID, CreatedEventID: createdEventID}, nil
}

func (s *recordingActivityStore) PersistPlanRevision(ctx context.Context, input PersistPlanRevisionInput) (PlanRevisionResult, error) {
	s.calls = append(s.calls, "PersistPlanRevision")
	s.persistPlanRevisionInputs = append(s.persistPlanRevisionInputs, input)
	status := s.planRevisionStatus
	if status == "" {
		status = "accepted"
		if input.Decision.RequiresHumanReview {
			status = "pending_review"
		}
	}
	planRevisionID := s.planRevisionID
	if len(s.planRevisionIDs) > 0 {
		planRevisionID = s.planRevisionIDs[0]
		s.planRevisionIDs = s.planRevisionIDs[1:]
	}
	if planRevisionID == uuid.Nil {
		planRevisionID = uuid.New()
	}
	createdEventID := s.routeEventID
	if len(s.planEventIDs) > 0 {
		createdEventID = s.planEventIDs[0]
		s.planEventIDs = s.planEventIDs[1:]
	}
	fingerprint := s.planFingerprint
	if fingerprint == "" {
		fingerprint = "fingerprint"
	}
	return PlanRevisionResult{ID: planRevisionID, Status: status, RevisionNumber: 1, PlanFingerprint: fingerprint, Payload: BuildPlanRevisionPayload(input.Decision), ReviewRequired: status == "pending_review", CreatedEventID: createdEventID}, nil
}

func (s *recordingActivityStore) RequestPlanRevisionReview(ctx context.Context, input RequestPlanRevisionReviewInput) (DecisionRequestResult, error) {
	s.calls = append(s.calls, "RequestPlanRevisionReview")
	s.requestPlanReviewInputs = append(s.requestPlanReviewInputs, input)
	return DecisionRequestResult{ID: s.decisionRequestID}, nil
}

func (s *recordingActivityStore) ResolvePlanRevisionReview(ctx context.Context, input ResolvePlanRevisionReviewInput) (PlanRevisionResult, error) {
	s.calls = append(s.calls, "ResolvePlanRevisionReview")
	s.resolvePlanReviewInputs = append(s.resolvePlanReviewInputs, input)
	return PlanRevisionResult{ID: input.PlanRevisionID, Status: "accepted", PlanFingerprint: "fingerprint"}, nil
}

func (s *recordingActivityStore) LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error) {
	s.calls = append(s.calls, "LoadHumanDecisionRoute")
	if route, ok := s.humanDecisionRoutes[input.DecisionRequestID]; ok {
		if route.PlanReview != nil && len(route.PlanReview.OutputEventIDs) == 0 {
			route.PlanReview.OutputEventIDs = s.planReviewOutputEventIDs(route.Decision.ID, *route.PlanReview)
		}
		return route, nil
	}
	if s.unknownHumanDecisionRouteIDs[input.DecisionRequestID] {
		return HumanDecisionRouteResult{}, nil
	}
	return HumanDecisionRouteResult{}, project.ErrInvalidProject
}

func (s *recordingActivityStore) planReviewOutputEventIDs(decisionRequestID uuid.UUID, route PlanReviewRoute) []uuid.UUID {
	if route.PlanRevisionID == uuid.Nil {
		return nil
	}
	outputEventIDs := []uuid.UUID{}
	planIndex := -1
	for index, planRevisionID := range s.planRevisionIDsForRoute {
		if planRevisionID == route.PlanRevisionID {
			planIndex = index
			break
		}
	}
	if planIndex < 0 {
		outputEventIDs = appendUniqueUUID(outputEventIDs, route.RouteEventID)
		outputEventIDs = appendUniqueUUID(outputEventIDs, route.PlanEventID)
		return outputEventIDs
	}
	for index := 0; index <= planIndex; index++ {
		if index < len(s.routeEventIDsForRoute) {
			outputEventIDs = appendUniqueUUID(outputEventIDs, s.routeEventIDsForRoute[index])
		}
		if index < len(s.planEventIDsForRoute) {
			outputEventIDs = appendUniqueUUID(outputEventIDs, s.planEventIDsForRoute[index])
		}
		if index == planIndex {
			break
		}
		if index < len(s.planResolvedEventIDsForRoute) {
			outputEventIDs = appendUniqueUUID(outputEventIDs, s.planResolvedEventIDsForRoute[index])
		}
	}
	_ = decisionRequestID
	return outputEventIDs
}

func (s *recordingActivityStore) DecomposeAcceptedPlanRevision(ctx context.Context, input DecomposeAcceptedPlanRevisionInput) ([]ProjectTaskResult, error) {
	s.calls = append(s.calls, "DecomposeAcceptedPlanRevision")
	s.decomposePlanInputs = append(s.decomposePlanInputs, input)
	taskIDs := s.taskIDs
	if len(taskIDs) == 0 {
		taskIDs = []uuid.UUID{s.taskID}
	}
	results := make([]ProjectTaskResult, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		results = append(results, ProjectTaskResult{ID: taskID})
	}
	return results, nil
}

func (s *recordingActivityStore) ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error) {
	s.calls = append(s.calls, "ListDispatchableTasks")
	s.listDispatchableInputs = append(s.listDispatchableInputs, input)
	if len(s.dispatchableTaskIDBatches) > 0 {
		result := s.dispatchableTaskIDBatches[0]
		s.dispatchableTaskIDBatches = s.dispatchableTaskIDBatches[1:]
		return result, nil
	}
	return s.dispatchableTaskIDs, nil
}

func (s *recordingActivityStore) ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error) {
	s.calls = append(s.calls, "ResolveReadyDownstream")
	s.resolveReadyInputs = append(s.resolveReadyInputs, input)
	return s.readyDownstreamIDs, nil
}

func (s *recordingActivityStore) InspectTaskResultDecision(ctx context.Context, input InspectTaskResultDecisionInput) (InspectTaskResultDecisionResult, error) {
	s.calls = append(s.calls, "InspectTaskResultDecision")
	s.inspectResultInputs = append(s.inspectResultInputs, input)
	return s.resultDecision, nil
}

func (s *recordingActivityStore) CreateRevisionTaskForResult(ctx context.Context, input CreateRevisionTaskForResultInput) (CreateRevisionTaskForResultResult, error) {
	s.calls = append(s.calls, "CreateRevisionTaskForResult")
	s.createRevisionInputs = append(s.createRevisionInputs, input)
	return s.revisionResult, nil
}

func (s *recordingActivityStore) CreateUpstreamSupplementTasks(ctx context.Context, input CreateUpstreamSupplementInput) (CreateUpstreamSupplementResult, error) {
	s.calls = append(s.calls, "CreateUpstreamSupplementTasks")
	s.upstreamSupplementInputs = append(s.upstreamSupplementInputs, input)
	return s.upstreamSupplementResult, nil
}

func (s *recordingActivityStore) RequestProjectTaskIterationExhaustedReview(ctx context.Context, input RequestProjectTaskIterationExhaustedReviewInput) (DecisionRequestResult, error) {
	s.calls = append(s.calls, "RequestProjectTaskIterationExhaustedReview")
	s.iterationExhaustedInputs = append(s.iterationExhaustedInputs, input)
	return DecisionRequestResult{ID: s.iterationExhaustedDecisionID}, nil
}

func (s *recordingActivityStore) RequestUpstreamSupplementReview(ctx context.Context, input RequestUpstreamSupplementReviewInput) (DecisionRequestResult, error) {
	s.calls = append(s.calls, "RequestUpstreamSupplementReview")
	s.upstreamSupplementReviewInputs = append(s.upstreamSupplementReviewInputs, input)
	return DecisionRequestResult{ID: s.upstreamSupplementReviewDecisionID}, nil
}

func (s *recordingActivityStore) IsProjectAcceptanceReady(ctx context.Context, input IsProjectAcceptanceReadyInput) (bool, error) {
	s.calls = append(s.calls, "IsProjectAcceptanceReady")
	return s.acceptanceReady, nil
}

func (s *recordingActivityStore) RequestProjectAcceptanceReview(ctx context.Context, input RequestProjectAcceptanceReviewInput) (DecisionRequestResult, error) {
	s.calls = append(s.calls, "RequestProjectAcceptanceReview")
	s.acceptanceReviewInputs = append(s.acceptanceReviewInputs, input)
	return DecisionRequestResult{ID: s.acceptanceDecisionRequestID}, nil
}

func (s *recordingActivityStore) ApplyProjectAcceptanceDecision(ctx context.Context, input ApplyProjectAcceptanceDecisionInput) error {
	s.calls = append(s.calls, "ApplyProjectAcceptanceDecision")
	s.applyAcceptanceInputs = append(s.applyAcceptanceInputs, input)
	return s.applyAcceptanceErr
}

func (s *recordingActivityStore) HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error) {
	s.calls = append(s.calls, "HoldDownstreamForFailure")
	s.holdFailureInputs = append(s.holdFailureInputs, input)
	return DecisionRequestResult{ID: s.failureRecoveryDecisionID}, nil
}

func (s *recordingActivityStore) ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) (ApplyFailureRecoveryDecisionResult, error) {
	s.calls = append(s.calls, "ApplyFailureRecoveryDecision")
	s.applyFailureRecoveryInputs = append(s.applyFailureRecoveryInputs, input)
	return s.failureRecoveryResult, nil
}

func (s *recordingActivityStore) ApplyPreDispatchGateDecision(ctx context.Context, input ApplyPreDispatchGateDecisionInput) (ApplyPreDispatchGateDecisionResult, error) {
	s.calls = append(s.calls, "ApplyPreDispatchGateDecision")
	s.applyPreDispatchGateInputs = append(s.applyPreDispatchGateInputs, input)
	if s.gateDecisionTaskID == nil {
		return ApplyPreDispatchGateDecisionResult{}, nil
	}
	return ApplyPreDispatchGateDecisionResult{ReadyTaskIDs: []uuid.UUID{*s.gateDecisionTaskID}}, nil
}

func (s *recordingActivityStore) AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error) {
	s.calls = append(s.calls, "AppendProjectEvent")
	s.appendEventTypes = append(s.appendEventTypes, input.EventType)
	return ProjectEventResult{ID: s.dispatchEvent}, nil
}

func (s *recordingActivityStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	s.calls = append(s.calls, "DispatchProjectTask")
	s.dispatchInputs = append(s.dispatchInputs, input)
	return s.dispatchErr
}

func (s *recordingActivityStore) RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error) {
	s.calls = append(s.calls, "RecoverTaskDispatchFailure")
	s.recoverInputs = append(s.recoverInputs, input)
	if s.recoverErr != nil {
		return RecoverTaskDispatchFailureResult{}, s.recoverErr
	}
	if len(s.recoverResults) > 0 {
		result := s.recoverResults[0]
		s.recoverResults = s.recoverResults[1:]
		return result, nil
	}
	return RecoverTaskDispatchFailureResult{Action: project.ProjectTaskRecoveryActionWaitingHuman}, nil
}

func (s *recordingActivityStore) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	s.calls = append(s.calls, "FinishCoordinationJob")
	s.finishJobInputs = append(s.finishJobInputs, input)
	return nil
}

func (s *recordingActivityStore) RejectDemandPlanning(ctx context.Context, input RejectDemandPlanningInput) error {
	s.calls = append(s.calls, "RejectDemandPlanning")
	s.rejectDemandInputs = append(s.rejectDemandInputs, input)
	return s.rejectDemandErr
}

func (s *recordingActivityStore) ReopenProjectDemandForReplanning(ctx context.Context, input ReopenProjectDemandForReplanningInput) error {
	s.calls = append(s.calls, "ReopenProjectDemandForReplanning")
	s.reopenDemandInputs = append(s.reopenDemandInputs, input)
	return s.reopenDemandErr
}
