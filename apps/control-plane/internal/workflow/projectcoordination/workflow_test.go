package projectcoordination

import (
	"context"
	"errors"
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

func TestProjectCoordinatorDispatchesHumanResolvedTaskThroughGate(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	decisionRequestID := uuid.New()
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
		taskIDs:                   []uuid.UUID{staleReadyID, blockedTaskID},
		decisionRequestID:         decisionRequestID,
		dispatchableTaskIDBatches: [][]uuid.UUID{{readyAfterApprovalID}},
		dispatchEvent:             uuid.New(),
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
		jobID:             uuid.New(),
		routeID:           uuid.New(),
		routeEventID:      uuid.New(),
		taskID:            uuid.New(),
		decisionRequestID: decisionRequestID,
		dispatchEvent:     uuid.New(),
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
		"ResolvePlanRevisionReview",
		"AppendProjectEvent",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"PersistPlanRevision",
		"RequestPlanRevisionReview",
	}, store.calls)
	require.Len(t, store.persistPlanRevisionInputs, 2)
	require.False(t, store.persistPlanRevisionInputs[0].SupersedeOpen)
	require.True(t, store.persistPlanRevisionInputs[1].SupersedeOpen)
	require.NotNil(t, store.persistPlanRevisionInputs[1].SupersedeReason)
	require.Equal(t, "缩小上线范围", *store.persistPlanRevisionInputs[1].SupersedeReason)
	require.Len(t, store.requestPlanReviewInputs, 2)
	require.Empty(t, store.dispatchInputs)
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
	require.Equal(t, []string{"ApplyPreDispatchGateDecision", "DispatchProjectTask"}, store.calls)
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
	store := &recordingActivityStore{
		snapshot:      CoordinationSnapshot{ProjectID: projectID},
		dispatchEvent: uuid.New(),
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			ApprovalRequestID: uuid.New(),
			DecisionRequestID: uuid.New(),
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
	require.Equal(t, []string{"ApplyPreDispatchGateDecision", "AppendProjectEvent"}, store.calls)
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

func TestProjectCoordinatorReturnsUnrecordedDispatchFailure(t *testing.T) {
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

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.NotContains(t, store.calls, "FinishCoordinationJob")
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
	require.Contains(t, store.calls, "FinishCoordinationJob")
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
	calls                     []string
	snapshot                  CoordinationSnapshot
	jobID                     uuid.UUID
	routeID                   uuid.UUID
	routeEventID              uuid.UUID
	planRevisionID            uuid.UUID
	planFingerprint           string
	planRevisionStatus        string
	taskID                    uuid.UUID
	taskIDs                   []uuid.UUID
	decisionRequestID         uuid.UUID
	failureRecoveryDecisionID uuid.UUID
	dispatchEvent             uuid.UUID
	dispatchErr               error

	dispatchableTaskIDs          []uuid.UUID
	dispatchableTaskIDBatches    [][]uuid.UUID
	readyDownstreamIDs           []uuid.UUID
	resultDecision               InspectTaskResultDecisionResult
	revisionResult               CreateRevisionTaskForResultResult
	failureRecoveryResult        ApplyFailureRecoveryDecisionResult
	iterationExhaustedDecisionID uuid.UUID
	listDispatchableInputs       []ListDispatchableTasksInput
	resolveReadyInputs           []ResolveReadyDownstreamInput
	inspectResultInputs          []InspectTaskResultDecisionInput
	createRevisionInputs         []CreateRevisionTaskForResultInput
	iterationExhaustedInputs     []RequestProjectTaskIterationExhaustedReviewInput
	acceptanceReady              bool
	acceptanceDecisionRequestID  uuid.UUID
	acceptanceReviewInputs       []RequestProjectAcceptanceReviewInput
	applyAcceptanceInputs        []ApplyProjectAcceptanceDecisionInput
	applyAcceptanceErr           error
	holdFailureInputs            []HoldDownstreamForFailureInput
	applyFailureRecoveryInputs   []ApplyFailureRecoveryDecisionInput
	applyPreDispatchGateInputs   []ApplyPreDispatchGateDecisionInput
	dispatchInputs               []DispatchProjectTaskInput
	gateDecisionTaskID           *uuid.UUID
	persistPlanRevisionInputs    []PersistPlanRevisionInput
	requestPlanReviewInputs      []RequestPlanRevisionReviewInput
	resolvePlanReviewInputs      []ResolvePlanRevisionReviewInput
	decomposePlanInputs          []DecomposeAcceptedPlanRevisionInput
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
	return CoordinationJobResult{ID: s.jobID}, nil
}

func (s *recordingActivityStore) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	s.calls = append(s.calls, "PersistRouteDecision")
	return RouteDecisionResult{ID: s.routeID, CreatedEventID: s.routeEventID}, nil
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
	if planRevisionID == uuid.Nil {
		planRevisionID = uuid.New()
	}
	fingerprint := s.planFingerprint
	if fingerprint == "" {
		fingerprint = "fingerprint"
	}
	return PlanRevisionResult{ID: planRevisionID, Status: status, RevisionNumber: 1, PlanFingerprint: fingerprint, Payload: BuildPlanRevisionPayload(input.Decision), ReviewRequired: status == "pending_review", CreatedEventID: s.routeEventID}, nil
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

func (s *recordingActivityStore) RequestProjectTaskIterationExhaustedReview(ctx context.Context, input RequestProjectTaskIterationExhaustedReviewInput) (DecisionRequestResult, error) {
	s.calls = append(s.calls, "RequestProjectTaskIterationExhaustedReview")
	s.iterationExhaustedInputs = append(s.iterationExhaustedInputs, input)
	return DecisionRequestResult{ID: s.iterationExhaustedDecisionID}, nil
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
	return ProjectEventResult{ID: s.dispatchEvent}, nil
}

func (s *recordingActivityStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	s.calls = append(s.calls, "DispatchProjectTask")
	s.dispatchInputs = append(s.dispatchInputs, input)
	return s.dispatchErr
}

func (s *recordingActivityStore) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	s.calls = append(s.calls, "FinishCoordinationJob")
	return nil
}
