package projectcoordination

import (
	"time"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func ProjectCoordinatorWorkflow(ctx workflow.Context, input ProjectCoordinatorInput) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy:         defaultRetryPolicy(),
	})
	demandCh := workflow.GetSignalChannel(ctx, SignalDemandSubmitted)
	policyCh := workflow.GetSignalChannel(ctx, SignalProjectPolicyChanged)
	memberCh := workflow.GetSignalChannel(ctx, SignalProjectMemberChanged)
	completedCh := workflow.GetSignalChannel(ctx, SignalEmployeeTaskCompleted)
	failedCh := workflow.GetSignalChannel(ctx, SignalEmployeeTaskFailed)
	transferCh := workflow.GetSignalChannel(ctx, SignalEmployeeTransferRequested)
	humanCh := workflow.GetSignalChannel(ctx, SignalHumanDecisionSubmitted)
	shutdownCh := workflow.GetSignalChannel(ctx, SignalShutdown)
	pendingReviews := map[string]pendingRouteDecisionReview{}
	pendingFailureRecoveries := map[string]pendingTaskFailureRecovery{}

	for {
		selector := workflow.NewSelector(ctx)
		var shouldStop bool
		var workflowErr error
		selector.AddReceive(demandCh, func(c workflow.ReceiveChannel, more bool) {
			var signal DemandSubmitted
			c.Receive(ctx, &signal)
			var pending *pendingRouteDecisionReview
			pending, workflowErr = handleDemandSubmitted(ctx, input, signal)
			if workflowErr == nil && pending != nil {
				pendingReviews[pending.DecisionRequestID.String()] = *pending
			}
		})
		selector.AddReceive(policyCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ProjectPolicyChanged
			c.Receive(ctx, &signal)
			workflowErr = appendSignalObservedEvent(ctx, input, "project policy changed")
		})
		selector.AddReceive(memberCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ProjectMemberChanged
			c.Receive(ctx, &signal)
			workflowErr = appendSignalObservedEvent(ctx, input, "project member changed")
		})
		selector.AddReceive(completedCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTaskCompleted
			c.Receive(ctx, &signal)
			workflowErr = handleEmployeeTaskCompleted(ctx, input, signal)
		})
		selector.AddReceive(failedCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTaskFailed
			c.Receive(ctx, &signal)
			var pending *pendingTaskFailureRecovery
			pending, workflowErr = handleEmployeeTaskFailed(ctx, input, signal)
			if workflowErr == nil && pending != nil {
				pendingFailureRecoveries[pending.DecisionRequestID.String()] = *pending
			}
		})
		selector.AddReceive(transferCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTransferRequested
			c.Receive(ctx, &signal)
			workflowErr = appendSignalObservedEvent(ctx, input, "employee transfer requested")
		})
		selector.AddReceive(humanCh, func(c workflow.ReceiveChannel, more bool) {
			var signal HumanDecisionSubmitted
			c.Receive(ctx, &signal)
			workflowErr = handleHumanDecisionSubmitted(ctx, input, signal, pendingReviews, pendingFailureRecoveries)
		})
		selector.AddReceive(shutdownCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ShutdownSignal
			c.Receive(ctx, &signal)
			_ = signal
			shouldStop = true
		})
		selector.Select(ctx)
		if workflowErr != nil {
			return workflowErr
		}
		if shouldStop {
			return nil
		}
	}
}

type pendingRouteDecisionReview struct {
	DecisionRequestID uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID uuid.UUID
	OutputEventIDs    []uuid.UUID
}

type pendingTaskFailureRecovery struct {
	DecisionRequestID uuid.UUID
	ProjectID         uuid.UUID
}

func handleDemandSubmitted(ctx workflow.Context, input ProjectCoordinatorInput, signal DemandSubmitted) (*pendingRouteDecisionReview, error) {
	workflowID := input.WorkflowID
	if workflowID == "" {
		workflowID = "project-coordinator:" + input.ProjectID.String()
	}
	jobInput := CreateCoordinationJobInput{
		TenantID:       input.TenantID,
		ProjectID:      signal.ProjectID,
		WorkflowID:     workflowID,
		TriggerEventID: signal.CreatedEventID,
		JobType:        "demand_route",
	}
	var job CoordinationJobResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).CreateCoordinationJob, jobInput).Get(ctx, &job); err != nil {
		return nil, err
	}

	var snapshot CoordinationSnapshot
	if err := workflow.ExecuteActivity(ctx, (*Activities).LoadProjectCoordinationSnapshot, LoadSnapshotInput{
		TenantID:  input.TenantID,
		ProjectID: signal.ProjectID,
		DemandID:  signal.DemandID,
	}).Get(ctx, &snapshot); err != nil {
		return nil, err
	}

	var decision RouteDecisionPlan
	if err := workflow.ExecuteActivity(ctx, (*Activities).PlanDemandRoute, snapshot).Get(ctx, &decision); err != nil {
		return nil, err
	}

	var route RouteDecisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).PersistRouteDecision, PersistRouteDecisionInput{
		TenantID:  input.TenantID,
		ProjectID: signal.ProjectID,
		JobID:     job.ID,
		DemandID:  signal.DemandID,
		Decision:  decision,
	}).Get(ctx, &route); err != nil {
		return nil, err
	}

	var tasks []ProjectTaskResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).CreateProjectTasks, CreateProjectTasksInput{
		TenantID:          input.TenantID,
		ProjectID:         signal.ProjectID,
		DemandID:          signal.DemandID,
		CoordinationJobID: job.ID,
		RouteDecisionID:   route.ID,
		Decision:          decision,
	}).Get(ctx, &tasks); err != nil {
		return nil, err
	}

	outputEventIDs := []uuid.UUID{route.CreatedEventID}
	taskIDs := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}
	readyTaskIDs, err := listDispatchableTasks(ctx, input.TenantID, signal.ProjectID, job.ID)
	if err != nil {
		return nil, err
	}
	if decision.RequiresHumanReview {
		var review DecisionRequestResult
		if err := workflow.ExecuteActivity(ctx, (*Activities).RequestRouteDecisionReview, RequestRouteDecisionReviewInput{
			TenantID:            input.TenantID,
			ProjectID:           signal.ProjectID,
			CoordinationJobID:   job.ID,
			DemandID:            signal.DemandID,
			RouteDecisionID:     route.ID,
			Decision:            decision,
			ProjectTaskIDs:      taskIDs,
			RouteCreatedEventID: route.CreatedEventID,
		}).Get(ctx, &review); err != nil {
			return nil, err
		}
		return &pendingRouteDecisionReview{
			DecisionRequestID: review.ID,
			ProjectID:         signal.ProjectID,
			CoordinationJobID: job.ID,
			OutputEventIDs:    outputEventIDs,
		}, nil
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, signal.ProjectID, readyTaskIDs); err != nil {
		return nil, err
	}
	return nil, finishCoordinationJob(ctx, input.TenantID, job.ID, "completed", outputEventIDs)
}

func handleHumanDecisionSubmitted(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted, pendingReviews map[string]pendingRouteDecisionReview, pendingFailureRecoveries map[string]pendingTaskFailureRecovery) error {
	if pending, ok := pendingReviews[signal.DecisionRequestID.String()]; ok {
		delete(pendingReviews, signal.DecisionRequestID.String())
		return handleRouteReviewDecision(ctx, input, signal, pending)
	}
	if pending, ok := pendingFailureRecoveries[signal.DecisionRequestID.String()]; ok {
		delete(pendingFailureRecoveries, signal.DecisionRequestID.String())
		return applyFailureRecoveryDecision(ctx, input.TenantID, pending.ProjectID, signal)
	}
	return appendSignalObservedEvent(ctx, input, "human decision submitted")
}

func handleRouteReviewDecision(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted, pending pendingRouteDecisionReview) error {
	outputEventIDs := append([]uuid.UUID{}, pending.OutputEventIDs...)
	if signal.ResolvedEventID != uuid.Nil {
		outputEventIDs = append(outputEventIDs, signal.ResolvedEventID)
	}
	if signal.Decision != "approved" {
		if err := appendSignalObservedEvent(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: pending.ProjectID}, "human route review rejected"); err != nil {
			return err
		}
		return finishCoordinationJob(ctx, input.TenantID, pending.CoordinationJobID, signal.Decision, outputEventIDs)
	}
	readyTaskIDs, err := listDispatchableTasks(ctx, input.TenantID, pending.ProjectID, pending.CoordinationJobID)
	if err != nil {
		return err
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, pending.ProjectID, readyTaskIDs); err != nil {
		return err
	}
	return finishCoordinationJob(ctx, input.TenantID, pending.CoordinationJobID, "completed", outputEventIDs)
}

func handleEmployeeTaskCompleted(ctx workflow.Context, input ProjectCoordinatorInput, signal EmployeeTaskCompleted) error {
	if err := appendSignalObservedEvent(ctx, input, "employee task completed"); err != nil {
		return err
	}
	readyTaskIDs, err := resolveReadyDownstream(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID)
	if err != nil {
		return err
	}
	return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs)
}

func handleEmployeeTaskFailed(ctx workflow.Context, input ProjectCoordinatorInput, signal EmployeeTaskFailed) (*pendingTaskFailureRecovery, error) {
	if err := appendSignalObservedEvent(ctx, input, "employee task failed"); err != nil {
		return nil, err
	}
	var decision DecisionRequestResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).HoldDownstreamForFailure, HoldDownstreamForFailureInput{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		FailedTaskID:   signal.ProjectTaskID,
		FailureSummary: signal.FailureSummary,
		FailedEventID:  signal.FailedEventID,
	}).Get(ctx, &decision); err != nil {
		return nil, err
	}
	if decision.ID == uuid.Nil {
		return nil, nil
	}
	return &pendingTaskFailureRecovery{
		DecisionRequestID: decision.ID,
		ProjectID:         input.ProjectID,
	}, nil
}

func applyFailureRecoveryDecision(ctx workflow.Context, tenantID, projectID uuid.UUID, signal HumanDecisionSubmitted) error {
	return workflow.ExecuteActivity(ctx, (*Activities).ApplyFailureRecoveryDecision, ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: signal.DecisionRequestID,
		Decision:          signal.Decision,
		Payload:           signal.Payload,
	}).Get(ctx, nil)
}

func listDispatchableTasks(ctx workflow.Context, tenantID, projectID, coordinationJobID uuid.UUID) ([]uuid.UUID, error) {
	var taskIDs []uuid.UUID
	if err := workflow.ExecuteActivity(ctx, (*Activities).ListDispatchableTasks, ListDispatchableTasksInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: coordinationJobID,
	}).Get(ctx, &taskIDs); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

func resolveReadyDownstream(ctx workflow.Context, tenantID, projectID, completedTaskID uuid.UUID) ([]uuid.UUID, error) {
	var taskIDs []uuid.UUID
	if err := workflow.ExecuteActivity(ctx, (*Activities).ResolveReadyDownstream, ResolveReadyDownstreamInput{
		TenantID:        tenantID,
		ProjectID:       projectID,
		CompletedTaskID: completedTaskID,
	}).Get(ctx, &taskIDs); err != nil {
		return nil, err
	}
	return taskIDs, nil
}

func dispatchProjectTasks(ctx workflow.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) error {
	for _, taskID := range taskIDs {
		if err := workflow.ExecuteActivity(ctx, (*Activities).DispatchProjectTask, DispatchProjectTaskInput{
			TenantID:  tenantID,
			ProjectID: projectID,
			TaskID:    taskID,
		}).Get(ctx, nil); err != nil {
			if !dispatchFailureRecorded(err) {
				return err
			}
			workflow.GetLogger(ctx).Warn("dispatch project task failed", "task_id", taskID.String(), "error", err.Error())
			continue
		}
	}
	return nil
}

func finishCoordinationJob(ctx workflow.Context, tenantID, jobID uuid.UUID, status string, outputEventIDs []uuid.UUID) error {
	finishInput := FinishCoordinationJobInput{
		TenantID:       tenantID,
		JobID:          jobID,
		Status:         status,
		OutputEventIDs: outputEventIDs,
	}
	return workflow.ExecuteActivity(ctx, (*Activities).FinishCoordinationJob, finishInput).Get(ctx, nil)
}

func appendSignalObservedEvent(ctx workflow.Context, input ProjectCoordinatorInput, summary string) error {
	var event ProjectEventResult
	return workflow.ExecuteActivity(ctx, (*Activities).AppendProjectEvent, AppendProjectEventInput{
		TenantID:  input.TenantID,
		ProjectID: input.ProjectID,
		EventType: "workflow.signaled",
		Summary:   summary,
	}).Get(ctx, &event)
}

func defaultRetryPolicy() *temporal.RetryPolicy {
	return &temporal.RetryPolicy{
		InitialInterval:    time.Second,
		BackoffCoefficient: 2,
		MaximumInterval:    10 * time.Second,
		MaximumAttempts:    3,
	}
}
