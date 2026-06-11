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

	for {
		selector := workflow.NewSelector(ctx)
		var shouldStop bool
		var workflowErr error
		selector.AddReceive(demandCh, func(c workflow.ReceiveChannel, more bool) {
			var signal DemandSubmitted
			c.Receive(ctx, &signal)
			workflowErr = handleDemandSubmitted(ctx, input, signal)
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
			workflowErr = appendSignalObservedEvent(ctx, input, "human decision submitted")
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

func handleDemandSubmitted(ctx workflow.Context, input ProjectCoordinatorInput, signal DemandSubmitted) error {
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
		return err
	}

	var snapshot CoordinationSnapshot
	if err := workflow.ExecuteActivity(ctx, (*Activities).LoadProjectCoordinationSnapshot, LoadSnapshotInput{
		TenantID:  input.TenantID,
		ProjectID: signal.ProjectID,
		DemandID:  signal.DemandID,
	}).Get(ctx, &snapshot); err != nil {
		return err
	}

	var decision RouteDecisionPlan
	if err := workflow.ExecuteActivity(ctx, (*Activities).PlanDemandRoute, snapshot).Get(ctx, &decision); err != nil {
		return err
	}

	var route RouteDecisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).PersistRouteDecision, PersistRouteDecisionInput{
		TenantID:  input.TenantID,
		ProjectID: signal.ProjectID,
		JobID:     job.ID,
		DemandID:  signal.DemandID,
		Decision:  decision,
	}).Get(ctx, &route); err != nil {
		return err
	}

	var tasks []ProjectTaskResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).CreateProjectTasks, CreateProjectTasksInput{
		TenantID:  input.TenantID,
		ProjectID: signal.ProjectID,
		DemandID:  signal.DemandID,
		Decision:  decision,
	}).Get(ctx, &tasks); err != nil {
		return err
	}

	outputEventIDs := []uuid.UUID{route.CreatedEventID}
	for _, task := range tasks {
		if err := workflow.ExecuteActivity(ctx, (*Activities).DispatchProjectTask, DispatchProjectTaskInput{
			TenantID:  input.TenantID,
			ProjectID: signal.ProjectID,
			TaskID:    task.ID,
		}).Get(ctx, nil); err != nil {
			return err
		}
	}
	finishInput := FinishCoordinationJobInput{
		TenantID:       input.TenantID,
		JobID:          job.ID,
		Status:         "completed",
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
