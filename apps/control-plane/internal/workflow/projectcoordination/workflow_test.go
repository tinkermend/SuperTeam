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
		"CreateProjectTasks",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
}

func TestProjectCoordinatorDispatchesOnlyRootTasks(t *testing.T) {
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
		"CreateProjectTasks",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, rootTaskID, store.dispatchInputs[0].TaskID)
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
		"CreateProjectTasks",
		"ListDispatchableTasks",
		"RequestRouteDecisionReview",
	}, store.calls)
}

func TestProjectCoordinatorDispatchesPendingTasksAfterHumanApproval(t *testing.T) {
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
		jobID:             uuid.New(),
		routeID:           uuid.New(),
		routeEventID:      uuid.New(),
		taskIDs:           []uuid.UUID{staleReadyID, blockedTaskID},
		decisionRequestID: decisionRequestID,
		dispatchableTaskIDBatches: [][]uuid.UUID{
			{staleReadyID},
			{readyAfterApprovalID},
		},
		dispatchEvent: uuid.New(),
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
		"CreateProjectTasks",
		"ListDispatchableTasks",
		"RequestRouteDecisionReview",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, readyAfterApprovalID, store.dispatchInputs[0].TaskID)
	require.Len(t, store.listDispatchableInputs, 2)
	require.Equal(t, store.jobID, store.listDispatchableInputs[0].CoordinationJobID)
	require.Equal(t, store.jobID, store.listDispatchableInputs[1].CoordinationJobID)
}

func TestProjectCoordinatorWakesDownstreamOnCompletion(t *testing.T) {
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
		dispatchEvent:       uuid.New(),
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
		"CreateProjectTasks",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
		"AppendProjectEvent",
		"ResolveReadyDownstream",
		"DispatchProjectTask",
	}, store.calls)
	require.Len(t, store.resolveReadyInputs, 1)
	require.Equal(t, rootTaskID, store.resolveReadyInputs[0].CompletedTaskID)
	require.Equal(t, []DispatchProjectTaskInput{
		{TenantID: store.dispatchInputs[0].TenantID, ProjectID: store.snapshot.ProjectID, TaskID: rootTaskID},
		{TenantID: store.dispatchInputs[0].TenantID, ProjectID: store.snapshot.ProjectID, TaskID: downstreamTaskID},
	}, store.dispatchInputs)
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

func TestProjectCoordinatorRoutesHumanDecisionToFailureRecovery(t *testing.T) {
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
	activities := NewActivities(store, HeuristicRoutePlanner{})
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
			Payload:           map[string]any{"recovery_action": "retry"},
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
		TenantID:  tenantID,
		ProjectID: projectID,
		TaskID:    replacementTaskID,
	}}, store.dispatchInputs)
	require.Len(t, store.applyFailureRecoveryInputs, 1)
	require.Equal(t, ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionRequestID,
		Decision:          "approved",
		Payload:           map[string]any{"recovery_action": "retry"},
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

type recordingActivityStore struct {
	calls                     []string
	snapshot                  CoordinationSnapshot
	jobID                     uuid.UUID
	routeID                   uuid.UUID
	routeEventID              uuid.UUID
	taskID                    uuid.UUID
	taskIDs                   []uuid.UUID
	decisionRequestID         uuid.UUID
	failureRecoveryDecisionID uuid.UUID
	dispatchEvent             uuid.UUID
	dispatchErr               error

	dispatchableTaskIDs        []uuid.UUID
	dispatchableTaskIDBatches  [][]uuid.UUID
	readyDownstreamIDs         []uuid.UUID
	failureRecoveryResult      ApplyFailureRecoveryDecisionResult
	listDispatchableInputs     []ListDispatchableTasksInput
	resolveReadyInputs         []ResolveReadyDownstreamInput
	acceptanceReady            bool
	acceptanceDecisionRequestID uuid.UUID
	acceptanceReviewInputs     []RequestProjectAcceptanceReviewInput
	applyAcceptanceInputs      []ApplyProjectAcceptanceDecisionInput
	applyAcceptanceErr         error
	holdFailureInputs          []HoldDownstreamForFailureInput
	applyFailureRecoveryInputs []ApplyFailureRecoveryDecisionInput
	dispatchInputs             []DispatchProjectTaskInput
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

func (s *recordingActivityStore) CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error) {
	s.calls = append(s.calls, "CreateProjectTasks")
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

func (s *recordingActivityStore) RequestRouteDecisionReview(ctx context.Context, input RequestRouteDecisionReviewInput) (DecisionRequestResult, error) {
	s.calls = append(s.calls, "RequestRouteDecisionReview")
	return DecisionRequestResult{ID: s.decisionRequestID}, nil
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
