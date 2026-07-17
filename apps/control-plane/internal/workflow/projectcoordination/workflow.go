package projectcoordination

import (
	"context"
	"errors"
	"strings"
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
			if workflowErr == nil {
				workflowErr = ensureDemandAcceptanceDecisionForTask(ctx, input, signal.ProjectTaskID)
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
			if workflowErr == nil {
				workflowErr = ensureDemandAcceptanceDecisionForTask(ctx, input, signal.ProjectTaskID)
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
		if diagnosis, gap, ok := noSuitableEmployeeDiagnosis(err); ok {
			// Terminal, non-retryable planning failure: the executor pool cannot
			// satisfy the demand. Route it to the demand rejection/diagnosis surface
			// instead of letting the error fall through to the generic signal_failed
			// audit event, which is invisible to a human.
			//
			// Deliberately NOT version-fenced. Replay determinism across the
			// introduction of this branch is guaranteed by the error type itself:
			// the typed non-retryable NoSuitableEmployee ApplicationError only
			// exists in histories recorded by code that also had this reject
			// branch (both shipped in the same commit); older histories carry
			// untyped retryable planner errors, which fail the type check above
			// and fall through to the old raw-error path. A retroactive
			// GetVersion fence here is actively harmful: histories that already
			// recorded RejectDemandPlanning carry no version marker, so replay
			// returns DefaultVersion, diverges from the recorded command, and
			// kills the coordinator (this bricked project-coordinator:b4226c24
			// on 2026-07-15; see replay_test.go). gap threads through unfenced for
			// the same reason: it is nil on any history whose recorded
			// ApplicationError predates the Details attachment, and non-nil is only
			// ever reached alongside this same reject branch.
			return nil, rejectDemandPlanning(ctx, input, signal, job.ID, diagnosis, gap, []uuid.UUID{signal.CreatedEventID})
		}
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
		CoordinationMode:  snapshot.Demand.CoordinationMode,
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
	case "planning_gap":
		return handlePlanningGapDecision(ctx, input, signal, route, projectID)
	case "task_failure_recovery", "upstream_supplement_review":
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

// handlePlanningGapDecision routes a resolved planning_gap decision. On restaffed
// or exempted the demand is unblocked — either the executor pool was supplemented,
// or the violated constraint was waived for this demand (a DemandConstraintExemption
// record is already persisted by Service.ResolveDecision before this signal fires)
// — so either way the demand is reopened (failed→planning_pending) and replanned in
// place by re-running the same planning path as a fresh demand submission — a
// failed replan flows through that path's own terminal reject handling, which may
// open a new planning_gap decision. Any other resolution (关闭/rejected) is
// terminal: the decision is already resolved by ResolveDecision and the demand
// stays failed.
//
// Temporal safety: planning_gap decisions are created only by new code, so this
// switch case (and the activities it drives) is unreachable on any pre-feature
// history — the decision type is the natural discriminator, no GetVersion fence.
// exempted is likewise new-code-only (same decision type), so it needs no fence
// either.
func handlePlanningGapDecision(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted, route HumanDecisionRouteResult, projectID uuid.UUID) error {
	if route.PlanningGap == nil {
		return temporal.NewNonRetryableApplicationError("human decision route missing planning gap", "HumanDecisionRouteMissingPlanningGap", project.ErrInvalidProject)
	}
	if signal.Decision != project.PlanningGapDecisionRestaffed && signal.Decision != project.PlanningGapDecisionExempted {
		return appendSignalObservedEvent(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: projectID}, "planning gap decision closed")
	}
	demandID := route.PlanningGap.DemandID
	if err := reopenProjectDemandForReplanning(ctx, input.TenantID, projectID, demandID); err != nil {
		return err
	}
	_, err := handleDemandSubmitted(ctx, ProjectCoordinatorInput{TenantID: input.TenantID, ProjectID: projectID, WorkflowID: input.WorkflowID}, DemandSubmitted{
		ProjectID:      projectID,
		DemandID:       demandID,
		CreatedEventID: signal.ResolvedEventID,
	})
	return err
}

func reopenProjectDemandForReplanning(ctx workflow.Context, tenantID, projectID, demandID uuid.UUID) error {
	return workflow.ExecuteActivity(ctx, (*Activities).ReopenProjectDemandForReplanning, ReopenProjectDemandForReplanningInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	}).Get(ctx, nil)
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
	snapshot.PinnedExitDeliverable = strings.TrimSpace(signal.TargetExitDeliverable)
	var decision RouteDecisionPlan
	if err := workflow.ExecuteActivity(ctx, (*Activities).PlanDemandRoute, snapshot).Get(ctx, &decision); err != nil {
		if diagnosis, gap, ok := noSuitableEmployeeDiagnosis(err); ok {
			// Terminal, non-retryable replan failure (e.g. the reselected exit
			// pulls in a stage the pool cannot staff): route the demand to the
			// rejection/diagnosis surface — same treatment as the initial
			// handleDemandSubmitted path — instead of letting the error fall
			// through to the generic signal_failed audit event, which strands
			// the demand in planning_pending with no human-visible diagnosis.
			//
			// Deliberately NOT version-fenced (see the matching comment in
			// handleDemandSubmitted). Unfenced executions of this branch already
			// wrote RejectDemandPlanning into live histories with no version
			// marker (project-coordinator:9bb61e95, 2026-07-15 18:02); a
			// retroactive fence replays DefaultVersion there and diverges —
			// replay_test.go pins this against the real exported history. The one
			// history recorded in the gap where the typed error existed but this
			// branch did not (project-coordinator:b4226c24) is unreplayable by any
			// code version and was remediated operationally. gap is nil-safe for
			// the same reason (see handleDemandSubmitted's matching comment).
			return nil, rejectDemandPlanningByID(ctx, input.TenantID, pending.ProjectID, pending.DemandID, pending.CoordinationJobID, diagnosis, gap, outputEventIDs)
		}
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
		CoordinationMode:  snapshot.Demand.CoordinationMode,
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
		if workflow.GetVersion(ctx, "coordination-mode-branch", workflow.DefaultVersion, 1) != workflow.DefaultVersion &&
			decision.CoordinationMode == project.CoordinationModePlan {
			review, err := requestUpstreamSupplementReview(ctx, input.TenantID, input.ProjectID,
				signal.ProjectTaskID, decision.ResultID, signal.CompletedEventID, decision.Blocker.MissingInputs)
			if err != nil || review.ID == uuid.Nil {
				return taskCompletionPending{}, err
			}
			return taskCompletionPending{FailureRecovery: &pendingTaskFailureRecovery{
				DecisionRequestID: review.ID,
				ProjectID:         input.ProjectID,
			}}, nil
		}
		// loop 模式:以下为既有自动补链路径,原样保留
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
	// Adversarial-review trigger (autonomy posture Phase B). GetVersion fence
	// from birth: this issues a NEW Activity command absent from old histories,
	// so it is NOT a "naturally unreachable" decode — it must be fenced. On old
	// histories GetVersion returns DefaultVersion (no marker written), taking the
	// original direct path straight to resolveReadyDownstream — replay-safe (see
	// TestReplayRealCoordinatorHistory). The legacy branch
	// (handleEmployeeTaskCompletedLegacy) is deliberately NOT fenced/triggered:
	// legacy plans predate the adversarial_review method, so they never carry
	// such criteria.
	//
	// v1 → v2 evolution (real full-stack E2E finding): v1 HELD the whole task
	// graph on a held/errored review — it early-returned taskCompletionPending
	// BEFORE resolveReadyDownstream, so downstream never dispatched, the graph
	// never went terminal, and requestProjectAcceptanceReview never fired. The
	// demand got STUCK in `executing` and never reached `acceptance_pending`,
	// which made the human tier-3 override (Task 4.5, gated on acceptance_pending)
	// unreachable — a held review became a dead end instead of a human hand-off.
	// Product decision: a held adversarial review must NOT block the task graph.
	// v2 lets downstream proceed so the graph can reach the acceptance gate, where
	// the persisted adversarial verdict (satisfied=false / escalate_human) — or, on
	// review error, the verdict-LESS blocking criterion — HOLDS the demand at
	// acceptance_pending via the convergence gate, and a human can override it.
	// The hold moves from the workflow (task graph) to the acceptance gate.
	//
	// Why a NEW version, not an edit of v1/v2: each shipped and ran on a live dev
	// workflow. Temporal replay discipline forbids retroactively editing a shipped
	// version. GetVersion(..., DefaultVersion, 3) returns: DefaultVersion for old
	// histories (marker absent → original direct path), 1 for the shipped v1
	// history (marker=1 → v1 block-downstream, preserved below), 2 for v2 histories
	// (unblock-to-acceptance, preserved below), 3 for new executions (v3
	// auto-rework). minSupported stays DefaultVersion so old histories — including
	// TestReplayRealCoordinatorHistory — remain valid, and the NEW auto-rework
	// ExecuteActivity command fires ONLY on v3 (never on replayed v1/v2/Default).
	adversarialVersion := workflow.GetVersion(ctx, "adversarial-review-trigger", workflow.DefaultVersion, 3)
	if adversarialVersion != workflow.DefaultVersion {
		var review AdversarialReviewForTaskResult
		reviewErr := workflow.ExecuteActivity(ctx, (*Activities).AdversarialReviewForTask, AdversarialReviewForTaskInput{
			TenantID:        input.TenantID,
			ProjectID:       input.ProjectID,
			CompletedTaskID: signal.ProjectTaskID,
		}).Get(ctx, &review)
		if reviewErr != nil {
			// Spec author requirement: a review that cannot complete (LLM
			// transport failure after retries exhaust) must NEVER auto-pass. The
			// blocking adversarial criterion stays verdict-less, so the convergence
			// gate holds it for the human tier-3 path. Swallow the error (do not
			// fail the workflow task) so it does not fail-and-retry into a hot loop.
			workflow.GetLogger(ctx).Error("adversarial review failed; holding demand for human escalation",
				"completed_task_id", signal.ProjectTaskID.String(), "error", reviewErr.Error(),
				"adversarial_version", adversarialVersion)
			if adversarialVersion == 1 {
				// v1 (preserved for replay): block downstream — early-return before
				// resolveReadyDownstream. Kept verbatim so v1 histories replay.
				return taskCompletionPending{}, nil
			}
			// v2/v3: fall through to resolveReadyDownstream. The verdict-less blocking
			// criterion holds the demand at the acceptance gate (not here), so the
			// graph can reach acceptance_pending for the human tier-3 override.
		} else if review.Reviewed && (!review.AllSatisfied || review.AnyEscalated) {
			// Adversarial criterion unsatisfied (majority refute) or escalate_human
			// (budget exhausted). The verdict is persisted, so the convergence gate
			// holds the demand; the acceptance/human tier-3 machinery owns it.
			workflow.GetLogger(ctx).Info("adversarial review held demand",
				"completed_task_id", signal.ProjectTaskID.String(),
				"all_satisfied", review.AllSatisfied, "any_escalated", review.AnyEscalated,
				"held_criteria", len(review.HeldCriteria),
				"adversarial_version", adversarialVersion)
			if adversarialVersion == 1 {
				// v1 (preserved for replay): block downstream. Kept verbatim.
				return taskCompletionPending{}, nil
			}
			// v3 auto-rework: a held review with rework-eligible criteria (judges
			// REFUTED, NOT escalate_human) and remaining revision budget dispatches
			// ONE rework task feeding the refutations back as input; downstream then
			// WAITS for convergence (no resolveReadyDownstream). Budget-exhausted or
			// escalate_human or a rework-activity error all fall through to
			// resolveReadyDownstream — release to the human acceptance gate. NEVER a
			// silent auto-pass. Fenced on v3 so the new ExecuteActivity command is
			// absent from replayed v1/v2 histories.
			if adversarialVersion >= 3 && len(review.HeldCriteria) > 0 && !review.AnyEscalated {
				var rework CreateReworkTaskFromAdversarialResult
				reworkErr := workflow.ExecuteActivity(ctx, (*Activities).CreateReworkTaskFromAdversarial, CreateReworkTaskFromAdversarialInput{
					TenantID:       input.TenantID,
					ProjectID:      input.ProjectID,
					ReviewedTaskID: signal.ProjectTaskID,
					DemandID:       review.DemandID,
					PlanRevisionID: review.PlanRevisionID,
					HeldCriteria:   review.HeldCriteria,
				}).Get(ctx, &rework)
				if reworkErr != nil {
					// Rework could not be created (transport/store failure, retries
					// exhausted): release to the human acceptance gate; do NOT auto-pass.
					workflow.GetLogger(ctx).Error("adversarial auto-rework failed; releasing to acceptance gate",
						"completed_task_id", signal.ProjectTaskID.String(), "error", reworkErr.Error())
					// fall through to resolveReadyDownstream
				} else if !rework.Exhausted && rework.TaskID != uuid.Nil {
					// Rework dispatched: downstream waits for convergence.
					workflow.GetLogger(ctx).Info("adversarial held → auto-rework dispatched",
						"completed_task_id", signal.ProjectTaskID.String(), "rework_task_id", rework.TaskID.String())
					return taskCompletionPending{}, dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, []uuid.UUID{rework.TaskID}, project.DispatchReasonRetry)
				}
				// Budget-exhausted (rework.Exhausted): fall through to
				// resolveReadyDownstream — release to acceptance_pending for a human.
			}
			// v2 (and v3 escalate/exhausted/error): fall through to
			// resolveReadyDownstream. The persisted verdict holds the demand at the
			// acceptance gate; downstream proceeds so the graph can reach the gate
			// rather than stalling in `executing`.
		}
	}
	// Review-gate trigger (violation-detection gate, Task 6). INDEPENDENT fence,
	// separate from the adversarial one above: it drives the DETECTION path
	// (secret_leak/code_review detectors → review_gate verdict), not the
	// adversarial JUDGE path. GetVersion from birth: this issues a NEW
	// ExecuteActivity command absent from old histories, so on replay of any
	// pre-Task-6 history GetVersion returns DefaultVersion (no marker written) →
	// the block is skipped → no new command → replay-safe (see
	// TestReplayRealCoordinatorHistory). It fires only on the common fall-through
	// path (i.e. when the adversarial block did not early-return to hold/rework),
	// which is exactly when the demand is heading to the acceptance gate.
	//
	// It NEVER blocks downstream: on a detected violation the review_gate verdict
	// is persisted `unsatisfied` and the Task-5 default-reversal convergence gate
	// holds the demand at acceptance_pending, where the human sees it at final
	// acceptance (the "human final acceptance" model, spec §6). An Activity error
	// (store load/persist failure) is logged and swallowed — since the P1.1
	// placeholder-race fix the criterion is NOT verdict-less at this point: the
	// completion writeback already wrote a `pending` placeholder, so a gate that
	// was triggered but never concluded leaves the demand HELD for the human
	// (fail-toward-oversight), not default-released. Either way we fall through
	// to resolveReadyDownstream.
	//
	// Ordering matters for the placeholder fix: this activity is AWAITED here,
	// and ensureDemandAcceptanceDecisionForTask runs only after this handler
	// returns — so in the clean case (verdict flipped to satisfied + activity-side
	// recompute → demand completed) no demand_acceptance decision is ever opened
	// for the transient placeholder hold; in the violation case the demand is
	// still acceptance_pending when the ensure probe runs, and the human decision
	// three-piece opens exactly as before.
	if workflow.GetVersion(ctx, "review-gate-trigger", workflow.DefaultVersion, 1) != workflow.DefaultVersion {
		var gate RunReviewGateForTaskResult
		gateErr := workflow.ExecuteActivity(ctx, (*Activities).RunReviewGateForTask, RunReviewGateForTaskInput{
			TenantID:        input.TenantID,
			ProjectID:       input.ProjectID,
			CompletedTaskID: signal.ProjectTaskID,
		}).Get(ctx, &gate)
		if gateErr != nil {
			workflow.GetLogger(ctx).Error("review gate failed; falling through (verdict-less review_gate → default release)",
				"completed_task_id", signal.ProjectTaskID.String(), "error", gateErr.Error())
		} else if gate.Reviewed && gate.AnyViolation {
			workflow.GetLogger(ctx).Info("review gate detected violation; demand held at acceptance gate for human final acceptance",
				"completed_task_id", signal.ProjectTaskID.String())
		}
		// Always fall through: a review_gate hold lives at the acceptance gate, never here.
	}
	// v3 fence: when a REVISION (rework) task completes — whether the loop
	// converged (judges satisfied) or exhausted/escalated to the human gate — its
	// downstream dependents are blocked on the revision ROOT (anchored round 0),
	// not on this rework. Resolve downstream of the root so the reviewed work's
	// terminal state finally flips blocked→planned. DefaultVersion/v1/v2 keep
	// resolving downstream-of-the-completing-task exactly as recorded (replay
	// safety); GetVersion is sticky, so a v3 rework completion re-enters here and
	// re-reads adversarialVersion==3.
	resolveRevisionRoot := adversarialVersion >= 3
	readyTaskIDs, err := resolveReadyDownstream(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID, resolveRevisionRoot)
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
	readyTaskIDs, err := resolveReadyDownstream(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID, false)
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

// reworkFromAdversarialCreator is the narrow store seam for turning a held
// adversarial criterion into an automatic rework task (Phase C1 Task 3). Like
// revisionTaskCreator it is type-asserted off a.store, not folded into the
// monolithic ActivityStore, so the many store fakes need not grow. The workflow
// trigger that calls it is Task 4.
type reworkFromAdversarialCreator interface {
	CreateReworkTaskFromAdversarial(ctx context.Context, input CreateReworkTaskFromAdversarialInput) (CreateReworkTaskFromAdversarialResult, error)
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

func (a *Activities) CreateReworkTaskFromAdversarial(ctx context.Context, input CreateReworkTaskFromAdversarialInput) (CreateReworkTaskFromAdversarialResult, error) {
	store, ok := a.store.(reworkFromAdversarialCreator)
	if a.store == nil || !ok {
		return CreateReworkTaskFromAdversarialResult{}, ErrActivityStoreRequired
	}
	return store.CreateReworkTaskFromAdversarial(ctx, input)
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

// ensureDemandAcceptanceDecisionForTask probes, after every task
// completion/failure signal, whether the task's demand just converged to
// acceptance_pending (the demand-acceptance criteria gate — see
// recomputeProjectDemandStatusWithQueries) and if so opens the
// demand_acceptance human-decision three-piece. Gated behind GetVersion so
// histories recorded before this gate existed replay unchanged; new
// executions always probe (the activity itself is a cheap, idempotent no-op
// on the common case where the demand isn't at acceptance_pending).
func ensureDemandAcceptanceDecisionForTask(ctx workflow.Context, input ProjectCoordinatorInput, projectTaskID uuid.UUID) error {
	if workflow.GetVersion(ctx, "demand-acceptance-gate", workflow.DefaultVersion, 1) == workflow.DefaultVersion {
		return nil
	}
	if projectTaskID == uuid.Nil {
		return nil
	}
	var result DecisionRequestResult
	return workflow.ExecuteActivity(ctx, (*Activities).EnsureDemandAcceptanceDecisionForTask, EnsureDemandAcceptanceDecisionForTaskInput{
		TenantID:      input.TenantID,
		ProjectID:     input.ProjectID,
		ProjectTaskID: projectTaskID,
	}).Get(ctx, &result)
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

func requestUpstreamSupplementReview(ctx workflow.Context, tenantID, projectID, projectTaskID, resultID, completedEventID uuid.UUID, missingInputs []string) (DecisionRequestResult, error) {
	var result DecisionRequestResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).RequestUpstreamSupplementReview, RequestUpstreamSupplementReviewInput{
		TenantID:         tenantID,
		ProjectID:        projectID,
		ProjectTaskID:    projectTaskID,
		ResultID:         resultID,
		CompletedEventID: completedEventID,
		MissingInputs:    missingInputs,
	}).Get(ctx, &result); err != nil {
		return DecisionRequestResult{}, err
	}
	return result, nil
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

func resolveReadyDownstream(ctx workflow.Context, tenantID, projectID, completedTaskID uuid.UUID, resolveRevisionRoot bool) ([]uuid.UUID, error) {
	var taskIDs []uuid.UUID
	if err := workflow.ExecuteActivity(ctx, (*Activities).ResolveReadyDownstream, ResolveReadyDownstreamInput{
		TenantID:            tenantID,
		ProjectID:           projectID,
		CompletedTaskID:     completedTaskID,
		ResolveRevisionRoot: resolveRevisionRoot,
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

// noSuitableEmployeeDiagnosis reports whether err is the terminal, non-retryable
// ErrNoSuitableEmployee escalation stamped by the PlanDemandRoute activity, and if
// so returns the human-readable diagnosis (the planner's reason with its structural
// ways-out hint) to surface on the demand, plus the structured PlanningGap when the
// ApplicationError carries one as a Details value (wrapNoSuitableEmployeeError in
// activities.go attaches it only for the structural role_independence gap channel).
//
// The gap is nil whenever Details is absent — including on old histories recorded
// before this field existed, where the replayed ApplicationError has no Details at
// all. That absence is itself the discriminator between old and new: no GetVersion
// fence is needed because appErr.HasDetails() is a deterministic decode of data
// already present in the recorded history event, not a live side effect (same
// reasoning as appErr.Message() below, which this function already replayed
// unfenced).
func noSuitableEmployeeDiagnosis(err error) (string, *PlanningGap, bool) {
	var appErr *temporal.ApplicationError
	if errors.As(err, &appErr) && appErr.Type() == errTypeNoSuitableEmployee {
		var gap *PlanningGap
		if appErr.HasDetails() {
			var decoded PlanningGap
			if detailsErr := appErr.Details(&decoded); detailsErr == nil {
				gap = &decoded
			}
		}
		return humanizeNoSuitableEmployeeDiagnosis(appErr.Message()), gap, true
	}
	return "", nil, false
}

// humanizeNoSuitableEmployeeDiagnosis strips the ErrNoSuitableEmployee sentinel
// prefix ("no suitable employee: ") so the surfaced diagnosis reads as a plain
// message rather than an error chain.
func humanizeNoSuitableEmployeeDiagnosis(message string) string {
	trimmed := strings.TrimSpace(message)
	trimmed = strings.TrimPrefix(trimmed, ErrNoSuitableEmployee.Error()+": ")
	return strings.TrimSpace(trimmed)
}

func rejectDemandPlanning(ctx workflow.Context, input ProjectCoordinatorInput, signal DemandSubmitted, jobID uuid.UUID, diagnosis string, gap *PlanningGap, outputEventIDs []uuid.UUID) error {
	return rejectDemandPlanningByID(ctx, input.TenantID, signal.ProjectID, signal.DemandID, jobID, diagnosis, gap, outputEventIDs)
}

func rejectDemandPlanningByID(ctx workflow.Context, tenantID, projectID, demandID, jobID uuid.UUID, diagnosis string, gap *PlanningGap, outputEventIDs []uuid.UUID) error {
	return workflow.ExecuteActivity(ctx, (*Activities).RejectDemandPlanning, RejectDemandPlanningInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		Diagnosis:         diagnosis,
		Gap:               gap,
		OutputEventIDs:    outputEventIDs,
	}).Get(ctx, nil)
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
