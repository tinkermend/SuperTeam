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
			workflowErr = appendSignalObservedEvent(ctx, input, "employee task completed")
		})
		selector.AddReceive(failedCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTaskFailed
			c.Receive(ctx, &signal)
			workflowErr = appendSignalObservedEvent(ctx, input, "employee task failed")
		})
		selector.AddReceive(transferCh, func(c workflow.ReceiveChannel, more bool) {
			var signal EmployeeTransferRequested
			c.Receive(ctx, &signal)
			workflowErr = appendSignalObservedEvent(ctx, input, "employee transfer requested")
		})
		selector.AddReceive(humanCh, func(c workflow.ReceiveChannel, more bool) {
			var signal HumanDecisionSubmitted
			c.Receive(ctx, &signal)
			workflowErr = handleHumanDecisionSubmitted(ctx, input, signal, pendingReviews)
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
	JobID             uuid.UUID
	TaskIDs           []uuid.UUID
	OutputEventIDs    []uuid.UUID
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
		TenantID:  input.TenantID,
		ProjectID: signal.ProjectID,
		DemandID:  signal.DemandID,
		Decision:  decision,
	}).Get(ctx, &tasks); err != nil {
		return nil, err
	}

	outputEventIDs := []uuid.UUID{route.CreatedEventID}
	taskIDs := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
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
			JobID:             job.ID,
			TaskIDs:           taskIDs,
			OutputEventIDs:    outputEventIDs,
		}, nil
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, signal.ProjectID, taskIDs); err != nil {
		return nil, err
	}
	return nil, finishCoordinationJob(ctx, input.TenantID, job.ID, "completed", outputEventIDs)
}

func handleHumanDecisionSubmitted(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted, pendingReviews map[string]pendingRouteDecisionReview) error {
	pending, ok := pendingReviews[signal.DecisionRequestID.String()]
	if !ok {
		return appendSignalObservedEvent(ctx, input, "human decision submitted")
	}
	delete(pendingReviews, signal.DecisionRequestID.String())
	outputEventIDs := append([]uuid.UUID{}, pending.OutputEventIDs...)
	if signal.ResolvedEventID != uuid.Nil {
		outputEventIDs = append(outputEventIDs, signal.ResolvedEventID)
	}
	if signal.Decision != "approved" {
		if err := appendSignalObservedEvent(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: pending.ProjectID}, "human route review rejected"); err != nil {
			return err
		}
		return finishCoordinationJob(ctx, input.TenantID, pending.JobID, signal.Decision, outputEventIDs)
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, pending.ProjectID, pending.TaskIDs); err != nil {
		return err
	}
	return finishCoordinationJob(ctx, input.TenantID, pending.JobID, "completed", outputEventIDs)
}

func dispatchProjectTasks(ctx workflow.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) error {
	for _, taskID := range taskIDs {
		if err := workflow.ExecuteActivity(ctx, (*Activities).DispatchProjectTask, DispatchProjectTaskInput{
			TenantID:  tenantID,
			ProjectID: projectID,
			TaskID:    taskID,
		}).Get(ctx, nil); err != nil {
			return err
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
