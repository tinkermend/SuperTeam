package projectcoordination

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func ProjectCoordinatorWorkflow(ctx workflow.Context, input ProjectCoordinatorInput) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		// Sized for the reasoning-model planner activity (PlanDemandRoute), which can
		// run well over a minute; it must exceed the planner's request timeout so the
		// HTTP call, not the activity, owns the deadline. Other activities finish fast,
		// so this is only a ceiling, not added latency.
		StartToCloseTimeout: 180 * time.Second,
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
	pendingReviews := map[string]pendingPlanRevisionReview{}
	pendingFailureRecoveries := map[string]pendingTaskFailureRecovery{}
	pendingAcceptance := map[string]pendingProjectAcceptance{}

	for {
		selector := workflow.NewSelector(ctx)
		var shouldStop bool
		var workflowErr error
		selector.AddReceive(demandCh, func(c workflow.ReceiveChannel, more bool) {
			var signal DemandSubmitted
			c.Receive(ctx, &signal)
			var pending *pendingPlanRevisionReview
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
			var pending taskCompletionPending
			pending, workflowErr = handleEmployeeTaskCompleted(ctx, input, signal)
			if workflowErr == nil && pending.Acceptance != nil {
				pendingAcceptance[pending.Acceptance.DecisionRequestID.String()] = *pending.Acceptance
			}
			if workflowErr == nil && pending.FailureRecovery != nil {
				pendingFailureRecoveries[pending.FailureRecovery.DecisionRequestID.String()] = *pending.FailureRecovery
			}
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
			if workflow.GetVersion(ctx, "route-human-decision-from-store", workflow.DefaultVersion, 1) == workflow.DefaultVersion {
				workflowErr = handleHumanDecisionSubmitted(ctx, input, signal, pendingReviews, pendingFailureRecoveries, pendingAcceptance)
				return
			}
			workflowErr = handleHumanDecisionSubmittedFromStore(ctx, input, signal)
		})
		selector.AddReceive(shutdownCh, func(c workflow.ReceiveChannel, more bool) {
			var signal ShutdownSignal
			c.Receive(ctx, &signal)
			_ = signal
			shouldStop = true
		})
		selector.Select(ctx)
		if workflowErr != nil {
			if workflow.GetVersion(ctx, "coordinator-survive-handler-error", workflow.DefaultVersion, 1) == workflow.DefaultVersion {
				return workflowErr
			}
			// A single signal handler failing — e.g. an activity exhausting its retry budget
			// during a transient DB/planner outage — must not terminate the long-lived
			// coordinator. If it did, every later signal (including the human decision that
			// would unblock the project) would be delivered to a closed workflow and lost,
			// deadlocking the project with no recovery path. Record the failure for audit and
			// keep the loop alive so subsequent signals are still processed.
			recordCoordinatorHandlerFailure(ctx, input, workflowErr)
			workflowErr = nil
		}
		if shouldStop {
			return nil
		}
		if shouldContinueAsNew(ctx) {
			return workflow.NewContinueAsNewError(ctx, ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
				TenantID:   input.TenantID,
				ProjectID:  input.ProjectID,
				WorkflowID: input.WorkflowID,
				Generation: input.Generation + 1,
			})
		}
	}
}

type pendingPlanRevisionReview struct {
	DecisionRequestID uuid.UUID
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	PlanRevisionID    uuid.UUID
	PlanFingerprint   string
	Payload           PlanRevisionPayload
	OutputEventIDs    []uuid.UUID
}

type pendingTaskFailureRecovery struct {
	DecisionRequestID uuid.UUID
	ProjectID         uuid.UUID
}

type pendingProjectAcceptance struct {
	DecisionRequestID uuid.UUID
	ProjectID         uuid.UUID
}

type taskCompletionPending struct {
	Acceptance      *pendingProjectAcceptance
	FailureRecovery *pendingTaskFailureRecovery
}

func handleDemandSubmitted(ctx workflow.Context, input ProjectCoordinatorInput, signal DemandSubmitted) (*pendingPlanRevisionReview, error) {
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

	var planRevision PlanRevisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).PersistPlanRevision, PersistPlanRevisionInput{
		TenantID:          input.TenantID,
		ProjectID:         signal.ProjectID,
		DemandID:          signal.DemandID,
		CoordinationJobID: job.ID,
		RouteDecisionID:   route.ID,
		Decision:          decision,
	}).Get(ctx, &planRevision); err != nil {
		return nil, err
	}

	outputEventIDs := []uuid.UUID{route.CreatedEventID}
	if planRevision.CreatedEventID != uuid.Nil && planRevision.CreatedEventID != route.CreatedEventID {
		outputEventIDs = append(outputEventIDs, planRevision.CreatedEventID)
	}
	pending := pendingPlanRevisionReview{
		ProjectID:         signal.ProjectID,
		DemandID:          signal.DemandID,
		CoordinationJobID: job.ID,
		RouteDecisionID:   route.ID,
		PlanRevisionID:    planRevision.ID,
		PlanFingerprint:   planRevision.PlanFingerprint,
		Payload:           planRevision.Payload,
		OutputEventIDs:    outputEventIDs,
	}
	switch planRevision.Status {
	case project.PlanRevisionStatusAccepted:
		return nil, decomposeAndDispatchAcceptedPlan(ctx, input, pending, project.DispatchReasonRootReady)
	case project.PlanRevisionStatusPendingReview:
		var review DecisionRequestResult
		if err := workflow.ExecuteActivity(ctx, (*Activities).RequestPlanRevisionReview, RequestPlanRevisionReviewInput{
			TenantID:          input.TenantID,
			ProjectID:         signal.ProjectID,
			CoordinationJobID: job.ID,
			DemandID:          signal.DemandID,
			PlanRevisionID:    planRevision.ID,
			PlanFingerprint:   planRevision.PlanFingerprint,
			Payload:           planRevision.Payload,
			CreatedEventID:    planRevision.CreatedEventID,
		}).Get(ctx, &review); err != nil {
			return nil, err
		}
		pending.DecisionRequestID = review.ID
		return &pending, nil
	default:
		return nil, finishCoordinationJob(ctx, input.TenantID, job.ID, planRevision.Status, outputEventIDs)
	}
}

func decomposeAndDispatchAcceptedPlan(ctx workflow.Context, input ProjectCoordinatorInput, pending pendingPlanRevisionReview, dispatchReason string) error {
	var tasks []ProjectTaskResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).DecomposeAcceptedPlanRevision, DecomposeAcceptedPlanRevisionInput{
		TenantID:          input.TenantID,
		ProjectID:         pending.ProjectID,
		DemandID:          pending.DemandID,
		CoordinationJobID: pending.CoordinationJobID,
		RouteDecisionID:   pending.RouteDecisionID,
		PlanRevisionID:    pending.PlanRevisionID,
		PlanFingerprint:   pending.PlanFingerprint,
		Payload:           pending.Payload,
	}).Get(ctx, &tasks); err != nil {
		return err
	}
	_ = tasks
	readyTaskIDs, err := listDispatchableTasks(ctx, input.TenantID, pending.ProjectID, pending.CoordinationJobID)
	if err != nil {
		return err
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, pending.ProjectID, readyTaskIDs, dispatchReason); err != nil {
		return err
	}
	return finishCoordinationJob(ctx, input.TenantID, pending.CoordinationJobID, "completed", pending.OutputEventIDs)
}

func handleHumanDecisionSubmitted(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted, pendingReviews map[string]pendingPlanRevisionReview, pendingFailureRecoveries map[string]pendingTaskFailureRecovery, pendingAcceptance map[string]pendingProjectAcceptance) error {
	if pending, ok := pendingReviews[signal.DecisionRequestID.String()]; ok {
		delete(pendingReviews, signal.DecisionRequestID.String())
		nextPending, err := handlePlanReviewDecision(ctx, input, signal, pending)
		if err == nil && nextPending != nil {
			pendingReviews[nextPending.DecisionRequestID.String()] = *nextPending
		}
		return err
	}
	if pending, ok := pendingFailureRecoveries[signal.DecisionRequestID.String()]; ok {
		delete(pendingFailureRecoveries, signal.DecisionRequestID.String())
		readyTaskIDs, err := applyFailureRecoveryDecision(ctx, input.TenantID, pending.ProjectID, signal)
		if err != nil {
			return err
		}
		return dispatchProjectTasks(ctx, input.TenantID, pending.ProjectID, readyTaskIDs, project.DispatchReasonRetry)
	}
	if pending, ok := pendingAcceptance[signal.DecisionRequestID.String()]; ok {
		delete(pendingAcceptance, signal.DecisionRequestID.String())
		return applyProjectAcceptanceDecision(ctx, input.TenantID, pending.ProjectID, signal)
	}
	if workflow.GetVersion(ctx, "predispatch-gate-decision-rerun", workflow.DefaultVersion, 1) == workflow.DefaultVersion {
		return appendSignalObservedEvent(ctx, input, "human decision submitted")
	}
	readyTaskIDs, err := applyPreDispatchGateDecision(ctx, input.TenantID, input.ProjectID, signal)
	if err != nil {
		return err
	}
	if len(readyTaskIDs) > 0 {
		return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonHumanResolved)
	}
	return appendSignalObservedEvent(ctx, input, "human decision submitted")
}

func handleHumanDecisionSubmittedFromStore(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted) error {
	route, err := loadHumanDecisionRoute(ctx, input.TenantID, input.ProjectID, signal.DecisionRequestID)
	if err != nil {
		return err
	}
	if route.Decision.ID == uuid.Nil {
		return appendSignalObservedEvent(ctx, input, "human decision submitted for unknown request")
	}
	projectID := route.Decision.ProjectID
	if projectID == uuid.Nil {
		projectID = input.ProjectID
	}
	switch route.Decision.DecisionType {
	case "plan_review":
		if route.PlanReview == nil {
			return temporal.NewNonRetryableApplicationError("human decision route missing plan review", "HumanDecisionRouteMissingPlanReview", project.ErrInvalidProject)
		}
		outputEventIDs := planReviewRouteOutputEventIDs(*route.PlanReview, route.Decision.CreatedEventID)
		pending := pendingPlanRevisionReview{
			DecisionRequestID: route.Decision.ID,
			ProjectID:         route.PlanReview.ProjectID,
			DemandID:          route.PlanReview.DemandID,
			CoordinationJobID: route.PlanReview.CoordinationJobID,
			RouteDecisionID:   route.PlanReview.RouteDecisionID,
			PlanRevisionID:    route.PlanReview.PlanRevisionID,
			PlanFingerprint:   route.PlanReview.PlanFingerprint,
			Payload:           route.PlanReview.Payload,
			OutputEventIDs:    outputEventIDs,
		}
		_, err := handlePlanReviewDecision(ctx, input, signal, pending)
		return err
	case "task_failure_recovery":
		readyTaskIDs, err := applyFailureRecoveryDecision(ctx, input.TenantID, projectID, signal)
		if err != nil {
			return err
		}
		return dispatchProjectTasks(ctx, input.TenantID, projectID, readyTaskIDs, project.DispatchReasonRetry)
	case "project_acceptance":
		return applyProjectAcceptanceDecision(ctx, input.TenantID, projectID, signal)
	case "project_task_approval":
		readyTaskIDs, err := applyPreDispatchGateDecision(ctx, input.TenantID, projectID, signal)
		if err != nil {
			return err
		}
		if len(readyTaskIDs) > 0 {
			return dispatchProjectTasks(ctx, input.TenantID, projectID, readyTaskIDs, project.DispatchReasonHumanResolved)
		}
		return appendSignalObservedEvent(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: projectID}, "human decision submitted")
	default:
		readyTaskIDs, err := applyPreDispatchGateDecision(ctx, input.TenantID, projectID, signal)
		if err != nil {
			return err
		}
		if len(readyTaskIDs) > 0 {
			return dispatchProjectTasks(ctx, input.TenantID, projectID, readyTaskIDs, project.DispatchReasonHumanResolved)
		}
		return appendSignalObservedEvent(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: projectID}, "human decision submitted")
	}
}

func planReviewRouteOutputEventIDs(route PlanReviewRoute, decisionCreatedEventID uuid.UUID) []uuid.UUID {
	if len(route.OutputEventIDs) > 0 {
		return append([]uuid.UUID{}, route.OutputEventIDs...)
	}
	outputEventIDs := make([]uuid.UUID, 0, 2)
	if route.RouteEventID != uuid.Nil {
		outputEventIDs = append(outputEventIDs, route.RouteEventID)
	}
	planEventID := route.PlanEventID
	if planEventID == uuid.Nil {
		planEventID = decisionCreatedEventID
	}
	if planEventID != uuid.Nil && planEventID != route.RouteEventID {
		outputEventIDs = append(outputEventIDs, planEventID)
	}
	return outputEventIDs
}

func handlePlanReviewDecision(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted, pending pendingPlanRevisionReview) (*pendingPlanRevisionReview, error) {
	outputEventIDs := append([]uuid.UUID{}, pending.OutputEventIDs...)
	if signal.ResolvedEventID != uuid.Nil {
		outputEventIDs = append(outputEventIDs, signal.ResolvedEventID)
	}
	var resolved PlanRevisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).ResolvePlanRevisionReview, ResolvePlanRevisionReviewInput{
		TenantID:          input.TenantID,
		ProjectID:         pending.ProjectID,
		DemandID:          pending.DemandID,
		CoordinationJobID: pending.CoordinationJobID,
		PlanRevisionID:    pending.PlanRevisionID,
		DecisionRequestID: signal.DecisionRequestID,
		Decision:          signal.Decision,
		Payload:           signal.Payload,
	}).Get(ctx, &resolved); err != nil {
		return nil, err
	}
	pending.OutputEventIDs = outputEventIDs
	if signal.Decision == project.PlanReviewDecisionAccept {
		if resolved.PlanFingerprint != "" {
			pending.PlanFingerprint = resolved.PlanFingerprint
		}
		return nil, decomposeAndDispatchAcceptedPlan(ctx, input, pending, project.DispatchReasonHumanResolved)
	}
	if signal.Decision == project.PlanReviewDecisionRequestChanges {
		nextPending, err := replanAfterPlanReviewChanges(ctx, input, signal, pending, outputEventIDs)
		return nextPending, err
	}
	if err := appendSignalObservedEvent(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: pending.ProjectID}, "human plan review rejected"); err != nil {
		return nil, err
	}
	return nil, finishCoordinationJob(ctx, input.TenantID, pending.CoordinationJobID, signal.Decision, outputEventIDs)
}

func replanAfterPlanReviewChanges(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted, pending pendingPlanRevisionReview, outputEventIDs []uuid.UUID) (*pendingPlanRevisionReview, error) {
	if err := appendSignalObservedEvent(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: pending.ProjectID}, "human plan review requested changes"); err != nil {
		return nil, err
	}
	var snapshot CoordinationSnapshot
	if err := workflow.ExecuteActivity(ctx, (*Activities).LoadProjectCoordinationSnapshot, LoadSnapshotInput{
		TenantID:  input.TenantID,
		ProjectID: pending.ProjectID,
		DemandID:  pending.DemandID,
	}).Get(ctx, &snapshot); err != nil {
		return nil, err
	}
	var decision RouteDecisionPlan
	if err := workflow.ExecuteActivity(ctx, (*Activities).PlanDemandRoute, snapshot).Get(ctx, &decision); err != nil {
		return nil, err
	}
	var routeDecision RouteDecisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).PersistRouteDecision, PersistRouteDecisionInput{
		TenantID:  input.TenantID,
		ProjectID: pending.ProjectID,
		JobID:     pending.CoordinationJobID,
		DemandID:  pending.DemandID,
		Decision:  decision,
	}).Get(ctx, &routeDecision); err != nil {
		return nil, err
	}
	if routeDecision.CreatedEventID != uuid.Nil {
		outputEventIDs = append(outputEventIDs, routeDecision.CreatedEventID)
	}
	supersedeReason := planReviewChangeReason(signal)
	var planRevision PlanRevisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).PersistPlanRevision, PersistPlanRevisionInput{
		TenantID:          input.TenantID,
		ProjectID:         pending.ProjectID,
		DemandID:          pending.DemandID,
		CoordinationJobID: pending.CoordinationJobID,
		RouteDecisionID:   routeDecision.ID,
		Decision:          decision,
		SupersedeOpen:     true,
		SupersedeReason:   supersedeReason,
	}).Get(ctx, &planRevision); err != nil {
		return nil, err
	}
	if planRevision.CreatedEventID != uuid.Nil {
		outputEventIDs = append(outputEventIDs, planRevision.CreatedEventID)
	}
	next := pendingPlanRevisionReview{
		ProjectID:         pending.ProjectID,
		DemandID:          pending.DemandID,
		CoordinationJobID: pending.CoordinationJobID,
		RouteDecisionID:   routeDecision.ID,
		PlanRevisionID:    planRevision.ID,
		PlanFingerprint:   planRevision.PlanFingerprint,
		Payload:           planRevision.Payload,
		OutputEventIDs:    outputEventIDs,
	}
	if planRevision.Status == project.PlanRevisionStatusAccepted {
		return nil, decomposeAndDispatchAcceptedPlan(ctx, input, next, project.DispatchReasonHumanResolved)
	}
	if planRevision.Status == project.PlanRevisionStatusPendingReview {
		var review DecisionRequestResult
		if err := workflow.ExecuteActivity(ctx, (*Activities).RequestPlanRevisionReview, RequestPlanRevisionReviewInput{
			TenantID:          input.TenantID,
			ProjectID:         pending.ProjectID,
			CoordinationJobID: pending.CoordinationJobID,
			DemandID:          pending.DemandID,
			PlanRevisionID:    planRevision.ID,
			PlanFingerprint:   planRevision.PlanFingerprint,
			Payload:           planRevision.Payload,
			CreatedEventID:    planRevision.CreatedEventID,
		}).Get(ctx, &review); err != nil {
			return nil, err
		}
		next.DecisionRequestID = review.ID
		return &next, nil
	}
	return nil, finishCoordinationJob(ctx, input.TenantID, pending.CoordinationJobID, planRevision.Status, outputEventIDs)
}

func planReviewChangeReason(signal HumanDecisionSubmitted) *string {
	for _, key := range []string{"reason", "comment", "summary"} {
		if value, ok := signal.Payload[key].(string); ok && value != "" {
			return &value
		}
	}
	reason := "human requested plan changes"
	return &reason
}

func handleEmployeeTaskCompleted(ctx workflow.Context, input ProjectCoordinatorInput, signal EmployeeTaskCompleted) (taskCompletionPending, error) {
	if err := appendSignalObservedEvent(ctx, input, "employee task completed"); err != nil {
		return taskCompletionPending{}, err
	}
	if workflow.GetVersion(ctx, "employee-task-result-decision", workflow.DefaultVersion, 1) == workflow.DefaultVersion {
		return handleEmployeeTaskCompletedLegacy(ctx, input, signal)
	}
	decision, err := inspectTaskResultDecision(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID)
	if err != nil {
		return taskCompletionPending{}, err
	}
	if decision.Decision == string(project.TaskResultDecisionRevisionAttempt) {
		if !decision.Exhausted && decision.ResultID != uuid.Nil {
			revision, err := createRevisionTaskForResult(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID, decision.ResultID)
			if err != nil {
				return taskCompletionPending{}, err
			}
			if !revision.Exhausted && revision.TaskID != uuid.Nil {
				return taskCompletionPending{}, dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, []uuid.UUID{revision.TaskID}, project.DispatchReasonRetry)
			}
		}
		decision, err := requestProjectTaskIterationExhaustedReview(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID, decision.ResultID, signal.CompletedEventID)
		if err != nil || decision.ID == uuid.Nil {
			return taskCompletionPending{}, err
		}
		return taskCompletionPending{FailureRecovery: &pendingTaskFailureRecovery{
			DecisionRequestID: decision.ID,
			ProjectID:         input.ProjectID,
		}}, nil
	}
	if decision.Decision == string(project.TaskResultDecisionBlockedResolvableUpstream) &&
		workflow.GetVersion(ctx, "upstream-supplement-task", workflow.DefaultVersion, 1) != workflow.DefaultVersion {
		if decision.Blocker == nil {
			return taskCompletionPending{}, project.ErrInvalidProject
		}
		supplement, err := createUpstreamSupplementTasks(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID, decision.Blocker.MissingInputs)
		if err != nil {
			return taskCompletionPending{}, err
		}
		if supplement.Exhausted {
			exhaustedDecision, err := requestProjectTaskIterationExhaustedReview(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID, decision.ResultID, signal.CompletedEventID)
			if err != nil || exhaustedDecision.ID == uuid.Nil {
				return taskCompletionPending{}, err
			}
			return taskCompletionPending{FailureRecovery: &pendingTaskFailureRecovery{
				DecisionRequestID: exhaustedDecision.ID,
				ProjectID:         input.ProjectID,
			}}, nil
		}
		return taskCompletionPending{}, dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, supplement.TaskIDs, project.DispatchReasonRetry)
	}
	if decision.Decision != "" && decision.Decision != string(project.TaskResultDecisionCompleteAccepted) {
		return taskCompletionPending{}, nil
	}
	readyTaskIDs, err := resolveReadyDownstream(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID)
	if err != nil {
		return taskCompletionPending{}, err
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonDependencyUnlocked); err != nil {
		return taskCompletionPending{}, err
	}
	// No further downstream to dispatch from this completion; if the whole project is
	// now terminal, open a human acceptance review. Idempotent on the store side.
	if len(readyTaskIDs) == 0 {
		ready, err := isProjectAcceptanceReady(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return taskCompletionPending{}, err
		}
		if !ready {
			return taskCompletionPending{}, nil
		}
		decision, err := requestProjectAcceptanceReview(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return taskCompletionPending{}, err
		}
		if decision.ID == uuid.Nil {
			return taskCompletionPending{}, nil
		}
		return taskCompletionPending{Acceptance: &pendingProjectAcceptance{
			DecisionRequestID: decision.ID,
			ProjectID:         input.ProjectID,
		}}, nil
	}
	return taskCompletionPending{}, nil
}

func handleEmployeeTaskCompletedLegacy(ctx workflow.Context, input ProjectCoordinatorInput, signal EmployeeTaskCompleted) (taskCompletionPending, error) {
	readyTaskIDs, err := resolveReadyDownstream(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID)
	if err != nil {
		return taskCompletionPending{}, err
	}
	if err := dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonDependencyUnlocked); err != nil {
		return taskCompletionPending{}, err
	}
	if len(readyTaskIDs) == 0 {
		ready, err := isProjectAcceptanceReady(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return taskCompletionPending{}, err
		}
		if !ready {
			return taskCompletionPending{}, nil
		}
		decision, err := requestProjectAcceptanceReview(ctx, input.TenantID, input.ProjectID)
		if err != nil {
			return taskCompletionPending{}, err
		}
		if decision.ID == uuid.Nil {
			return taskCompletionPending{}, nil
		}
		return taskCompletionPending{Acceptance: &pendingProjectAcceptance{
			DecisionRequestID: decision.ID,
			ProjectID:         input.ProjectID,
		}}, nil
	}
	return taskCompletionPending{}, nil
}

type taskResultDecisionInspector interface {
	InspectTaskResultDecision(ctx context.Context, input InspectTaskResultDecisionInput) (InspectTaskResultDecisionResult, error)
}

type revisionTaskCreator interface {
	CreateRevisionTaskForResult(ctx context.Context, input CreateRevisionTaskForResultInput) (CreateRevisionTaskForResultResult, error)
}

type upstreamSupplementTaskCreator interface {
	CreateUpstreamSupplementTasks(ctx context.Context, input CreateUpstreamSupplementInput) (CreateUpstreamSupplementResult, error)
}

func (a *Activities) InspectTaskResultDecision(ctx context.Context, input InspectTaskResultDecisionInput) (InspectTaskResultDecisionResult, error) {
	store, ok := a.store.(taskResultDecisionInspector)
	if a.store == nil || !ok {
		return InspectTaskResultDecisionResult{}, ErrActivityStoreRequired
	}
	return store.InspectTaskResultDecision(ctx, input)
}

func (a *Activities) CreateRevisionTaskForResult(ctx context.Context, input CreateRevisionTaskForResultInput) (CreateRevisionTaskForResultResult, error) {
	store, ok := a.store.(revisionTaskCreator)
	if a.store == nil || !ok {
		return CreateRevisionTaskForResultResult{}, ErrActivityStoreRequired
	}
	return store.CreateRevisionTaskForResult(ctx, input)
}

func (a *Activities) CreateUpstreamSupplementTasks(ctx context.Context, input CreateUpstreamSupplementInput) (CreateUpstreamSupplementResult, error) {
	store, ok := a.store.(upstreamSupplementTaskCreator)
	if a.store == nil || !ok {
		return CreateUpstreamSupplementResult{}, ErrActivityStoreRequired
	}
	return store.CreateUpstreamSupplementTasks(ctx, input)
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

func applyFailureRecoveryDecision(ctx workflow.Context, tenantID, projectID uuid.UUID, signal HumanDecisionSubmitted) ([]uuid.UUID, error) {
	var result ApplyFailureRecoveryDecisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).ApplyFailureRecoveryDecision, ApplyFailureRecoveryDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: signal.DecisionRequestID,
		Decision:          signal.Decision,
		Payload:           signal.Payload,
	}).Get(ctx, &result); err != nil {
		return nil, err
	}
	return result.ReadyTaskIDs, nil
}

func loadHumanDecisionRoute(ctx workflow.Context, tenantID, projectID, decisionRequestID uuid.UUID) (HumanDecisionRouteResult, error) {
	var result HumanDecisionRouteResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).LoadHumanDecisionRoute, LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionRequestID,
	}).Get(ctx, &result); err != nil {
		return HumanDecisionRouteResult{}, err
	}
	return result, nil
}

func applyPreDispatchGateDecision(ctx workflow.Context, tenantID, projectID uuid.UUID, signal HumanDecisionSubmitted) ([]uuid.UUID, error) {
	var result ApplyPreDispatchGateDecisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).ApplyPreDispatchGateDecision, ApplyPreDispatchGateDecisionInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: signal.DecisionRequestID,
		Decision:          signal.Decision,
		Payload:           signal.Payload,
	}).Get(ctx, &result); err != nil {
		return nil, err
	}
	return result.ReadyTaskIDs, nil
}

func inspectTaskResultDecision(ctx workflow.Context, tenantID, projectID, projectTaskID uuid.UUID) (InspectTaskResultDecisionResult, error) {
	var result InspectTaskResultDecisionResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).InspectTaskResultDecision, InspectTaskResultDecisionInput{
		TenantID:      tenantID,
		ProjectID:     projectID,
		ProjectTaskID: projectTaskID,
	}).Get(ctx, &result); err != nil {
		return InspectTaskResultDecisionResult{}, err
	}
	return result, nil
}

func createRevisionTaskForResult(ctx workflow.Context, tenantID, projectID, sourceTaskID, resultID uuid.UUID) (CreateRevisionTaskForResultResult, error) {
	var result CreateRevisionTaskForResultResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).CreateRevisionTaskForResult, CreateRevisionTaskForResultInput{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SourceTaskID: sourceTaskID,
		ResultID:     resultID,
	}).Get(ctx, &result); err != nil {
		return CreateRevisionTaskForResultResult{}, err
	}
	return result, nil
}

func createUpstreamSupplementTasks(ctx workflow.Context, tenantID, projectID, sourceTaskID uuid.UUID, missingInputs []string) (CreateUpstreamSupplementResult, error) {
	var result CreateUpstreamSupplementResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).CreateUpstreamSupplementTasks, CreateUpstreamSupplementInput{
		TenantID:      tenantID,
		ProjectID:     projectID,
		SourceTaskID:  sourceTaskID,
		MissingInputs: missingInputs,
	}).Get(ctx, &result); err != nil {
		return CreateUpstreamSupplementResult{}, err
	}
	return result, nil
}

func isProjectAcceptanceReady(ctx workflow.Context, tenantID, projectID uuid.UUID) (bool, error) {
	var ready bool
	if err := workflow.ExecuteActivity(ctx, (*Activities).IsProjectAcceptanceReady, IsProjectAcceptanceReadyInput{
		TenantID:  tenantID,
		ProjectID: projectID,
	}).Get(ctx, &ready); err != nil {
		return false, err
	}
	return ready, nil
}

func requestProjectAcceptanceReview(ctx workflow.Context, tenantID, projectID uuid.UUID) (DecisionRequestResult, error) {
	var decision DecisionRequestResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).RequestProjectAcceptanceReview, RequestProjectAcceptanceReviewInput{
		TenantID:  tenantID,
		ProjectID: projectID,
	}).Get(ctx, &decision); err != nil {
		return DecisionRequestResult{}, err
	}
	return decision, nil
}

func requestProjectTaskIterationExhaustedReview(ctx workflow.Context, tenantID, projectID, projectTaskID, resultID, completedEventID uuid.UUID) (DecisionRequestResult, error) {
	var decision DecisionRequestResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).RequestProjectTaskIterationExhaustedReview, RequestProjectTaskIterationExhaustedReviewInput{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskID:  projectTaskID,
		ResultID:       resultID,
		Reason:         "iteration_exhausted",
		Summary:        "同一失败重复出现，需要人类判断是否继续",
		CreatedEventID: completedEventID,
	}).Get(ctx, &decision); err != nil {
		return DecisionRequestResult{}, err
	}
	return decision, nil
}

func applyProjectAcceptanceDecision(ctx workflow.Context, tenantID, projectID uuid.UUID, signal HumanDecisionSubmitted) error {
	return workflow.ExecuteActivity(ctx, (*Activities).ApplyProjectAcceptanceDecision, ApplyProjectAcceptanceDecisionInput{
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

func dispatchProjectTasks(ctx workflow.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID, dispatchReason string) error {
	for _, taskID := range taskIDs {
		if err := workflow.ExecuteActivity(ctx, (*Activities).DispatchProjectTask, DispatchProjectTaskInput{
			TenantID:       tenantID,
			ProjectID:      projectID,
			TaskID:         taskID,
			DispatchReason: dispatchReason,
		}).Get(ctx, nil); err != nil {
			if !dispatchFailureRecorded(err) {
				return err
			}
			workflow.GetLogger(ctx).Warn("dispatch project task failed", "task_id", taskID.String(), "error", err.Error())
			var recovery RecoverTaskDispatchFailureResult
			if recoverErr := workflow.ExecuteActivity(ctx, (*Activities).RecoverTaskDispatchFailure, RecoverTaskDispatchFailureInput{
				TenantID:      tenantID,
				ProjectID:     projectID,
				ProjectTaskID: taskID,
			}).Get(ctx, &recovery); recoverErr != nil {
				return recoverErr
			}
			if recovery.Action == project.ProjectTaskRecoveryActionRetryScheduled && recovery.RetryNotBefore != nil {
				scheduleDispatchRetry(ctx, tenantID, projectID, taskID, *recovery.RetryNotBefore)
			}
			continue
		}
	}
	return nil
}

// scheduleDispatchRetry re-dispatches one task after its retry backoff.
// dispatchProjectTasks only runs from signal handlers, so without this timer a
// retry-scheduled task would wait for an unrelated signal. The loop is bounded:
// recovery stops returning retry_scheduled once dispatch failures reach
// max_attempts and moves the task to waiting_human instead.
func scheduleDispatchRetry(ctx workflow.Context, tenantID, projectID, taskID uuid.UUID, retryAt time.Time) {
	delay := retryAt.Sub(workflow.Now(ctx))
	if delay < 0 {
		delay = 0
	}
	workflow.Go(ctx, func(gctx workflow.Context) {
		if err := workflow.Sleep(gctx, delay); err != nil {
			return
		}
		if err := dispatchProjectTasks(gctx, tenantID, projectID, []uuid.UUID{taskID}, project.DispatchReasonRetry); err != nil {
			workflow.GetLogger(gctx).Error("retry dispatch failed", "task_id", taskID.String(), "error", err.Error())
		}
	})
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

// recordCoordinatorHandlerFailure leaves an audit trail when a signal handler fails but the
// coordinator deliberately survives. It is best-effort: if the audit activity itself fails
// (the same outage that broke the handler), the error is swallowed and only logged — the
// whole point is to never let a failed activity terminate the workflow.
func recordCoordinatorHandlerFailure(ctx workflow.Context, input ProjectCoordinatorInput, handlerErr error) {
	workflow.GetLogger(ctx).Error("coordinator signal handler failed; continuing",
		"project_id", input.ProjectID.String(), "error", handlerErr.Error())
	if err := workflow.ExecuteActivity(ctx, (*Activities).AppendProjectEvent, AppendProjectEventInput{
		TenantID:  input.TenantID,
		ProjectID: input.ProjectID,
		EventType: "workflow.signal_failed",
		Summary:   "coordinator signal handler failed: " + handlerErr.Error(),
	}).Get(ctx, nil); err != nil {
		workflow.GetLogger(ctx).Error("failed to record coordinator handler failure",
			"project_id", input.ProjectID.String(), "error", err.Error())
	}
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

func shouldContinueAsNew(ctx workflow.Context) bool {
	return workflow.GetInfo(ctx).GetContinueAsNewSuggested()
}

func defaultRetryPolicy() *temporal.RetryPolicy {
	return &temporal.RetryPolicy{
		InitialInterval:    time.Second,
		BackoffCoefficient: 2,
		MaximumInterval:    10 * time.Second,
		MaximumAttempts:    3,
	}
}
