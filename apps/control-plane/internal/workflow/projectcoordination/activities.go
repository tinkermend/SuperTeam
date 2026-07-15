package projectcoordination

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
)

var ErrActivityStoreRequired = errors.New("project coordination activity store is required")

var ErrRoutePlannerRequired = errors.New("project coordination route planner is required")

// errTypeNoSuitableEmployee is the temporal ApplicationError Type stamped on a
// terminal ErrNoSuitableEmployee planning failure. The workflow matches on this
// Type to route the demand to its rejection/diagnosis surface instead of retrying.
const errTypeNoSuitableEmployee = "NoSuitableEmployee"

type Activities struct {
	store   ActivityStore
	planner RoutePlanner
}

type ActivityStore interface {
	LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error)
	CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error)
	PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error)
	PersistPlanRevision(ctx context.Context, input PersistPlanRevisionInput) (PlanRevisionResult, error)
	RequestPlanRevisionReview(ctx context.Context, input RequestPlanRevisionReviewInput) (DecisionRequestResult, error)
	ResolvePlanRevisionReview(ctx context.Context, input ResolvePlanRevisionReviewInput) (PlanRevisionResult, error)
	LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error)
	DecomposeAcceptedPlanRevision(ctx context.Context, input DecomposeAcceptedPlanRevisionInput) ([]ProjectTaskResult, error)
	ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error)
	ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error)
	IsProjectAcceptanceReady(ctx context.Context, input IsProjectAcceptanceReadyInput) (bool, error)
	RequestProjectAcceptanceReview(ctx context.Context, input RequestProjectAcceptanceReviewInput) (DecisionRequestResult, error)
	RequestProjectTaskIterationExhaustedReview(ctx context.Context, input RequestProjectTaskIterationExhaustedReviewInput) (DecisionRequestResult, error)
	RequestUpstreamSupplementReview(ctx context.Context, input RequestUpstreamSupplementReviewInput) (DecisionRequestResult, error)
	ApplyProjectAcceptanceDecision(ctx context.Context, input ApplyProjectAcceptanceDecisionInput) error
	HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error)
	ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) (ApplyFailureRecoveryDecisionResult, error)
	ApplyPreDispatchGateDecision(ctx context.Context, input ApplyPreDispatchGateDecisionInput) (ApplyPreDispatchGateDecisionResult, error)
	AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error)
	RejectDemandPlanning(ctx context.Context, input RejectDemandPlanningInput) error
	DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error
	RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error)
	FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error
}

func NewActivities(store ActivityStore, planner ...RoutePlanner) *Activities {
	var selected RoutePlanner
	if len(planner) > 0 && planner[0] != nil {
		selected = planner[0]
	}
	return &Activities{store: store, planner: selected}
}

func (a *Activities) LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error) {
	if a.store == nil {
		return CoordinationSnapshot{}, ErrActivityStoreRequired
	}
	return a.store.LoadProjectCoordinationSnapshot(ctx, input)
}

func (a *Activities) CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error) {
	if a.store == nil {
		return CoordinationJobResult{}, ErrActivityStoreRequired
	}
	return a.store.CreateCoordinationJob(ctx, input)
}

func (a *Activities) PlanDemandRoute(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	if a.planner == nil {
		return RouteDecisionPlan{}, ErrRoutePlannerRequired
	}
	decision, err := a.planner.Plan(ctx, snapshot)
	if err != nil && errors.Is(err, ErrNoSuitableEmployee) {
		// A no-suitable-employee failure is structural: the executor pool cannot
		// satisfy the plan and re-planning would reselect the same pool forever.
		// Every retry is a fresh, real reasoning-model call that cannot change the
		// outcome, so mark it non-retryable — Temporal escalates immediately
		// instead of burning MaximumAttempts planner calls. err.Error() carries the
		// human-readable diagnosis (with fix 3's structural ways-out hints) for the
		// workflow to surface on the demand.
		return RouteDecisionPlan{}, wrapNoSuitableEmployeeError(err)
	}
	return decision, err
}

// wrapNoSuitableEmployeeError stamps the terminal, non-retryable ApplicationError
// on an ErrNoSuitableEmployee-family planning failure. When err originates from
// governance's structural-gap channel (a *structuralGapError, from
// enforceRoleIndependence/structuralGapForPlan in template_governance.go), the
// PlanningGap it carries is attached as an ApplicationError detail so the workflow
// (noSuitableEmployeeDiagnosis) and, downstream, project_store.RejectDemandPlanning
// can act on structured gap data instead of only free-text diagnosis. err must
// satisfy errors.Is(err, ErrNoSuitableEmployee); the only call site already guards
// on that. Extracted from PlanDemandRoute so tests can exercise
// governance-error → wrap → detail-extraction without a live Temporal test
// environment: the SDK's ApplicationError detail values round-trip in-process by
// reflection (ErrorDetailsValues.Get), no data converter or workflow environment
// required.
func wrapNoSuitableEmployeeError(err error) error {
	var gapErr *structuralGapError
	if errors.As(err, &gapErr) {
		return temporal.NewNonRetryableApplicationError(err.Error(), errTypeNoSuitableEmployee, err, gapErr.gap)
	}
	return temporal.NewNonRetryableApplicationError(err.Error(), errTypeNoSuitableEmployee, err)
}

func (a *Activities) RejectDemandPlanning(ctx context.Context, input RejectDemandPlanningInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	return a.store.RejectDemandPlanning(ctx, input)
}

func (a *Activities) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	if a.store == nil {
		return RouteDecisionResult{}, ErrActivityStoreRequired
	}
	return a.store.PersistRouteDecision(ctx, input)
}

func (a *Activities) PersistPlanRevision(ctx context.Context, input PersistPlanRevisionInput) (PlanRevisionResult, error) {
	if a.store == nil {
		return PlanRevisionResult{}, ErrActivityStoreRequired
	}
	return a.store.PersistPlanRevision(ctx, input)
}

func (a *Activities) RequestPlanRevisionReview(ctx context.Context, input RequestPlanRevisionReviewInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.RequestPlanRevisionReview(ctx, input)
}

func (a *Activities) ResolvePlanRevisionReview(ctx context.Context, input ResolvePlanRevisionReviewInput) (PlanRevisionResult, error) {
	if a.store == nil {
		return PlanRevisionResult{}, ErrActivityStoreRequired
	}
	return a.store.ResolvePlanRevisionReview(ctx, input)
}

func (a *Activities) LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error) {
	if a.store == nil {
		return HumanDecisionRouteResult{}, ErrActivityStoreRequired
	}
	return a.store.LoadHumanDecisionRoute(ctx, input)
}

func (a *Activities) DecomposeAcceptedPlanRevision(ctx context.Context, input DecomposeAcceptedPlanRevisionInput) ([]ProjectTaskResult, error) {
	if a.store == nil {
		return nil, ErrActivityStoreRequired
	}
	return a.store.DecomposeAcceptedPlanRevision(ctx, input)
}

func (a *Activities) ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error) {
	if a.store == nil {
		return nil, ErrActivityStoreRequired
	}
	return a.store.ListDispatchableTasks(ctx, input)
}

func (a *Activities) ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error) {
	if a.store == nil {
		return nil, ErrActivityStoreRequired
	}
	return a.store.ResolveReadyDownstream(ctx, input)
}

func (a *Activities) IsProjectAcceptanceReady(ctx context.Context, input IsProjectAcceptanceReadyInput) (bool, error) {
	if a.store == nil {
		return false, ErrActivityStoreRequired
	}
	return a.store.IsProjectAcceptanceReady(ctx, input)
}

func (a *Activities) RequestProjectAcceptanceReview(ctx context.Context, input RequestProjectAcceptanceReviewInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.RequestProjectAcceptanceReview(ctx, input)
}

func (a *Activities) RequestProjectTaskIterationExhaustedReview(ctx context.Context, input RequestProjectTaskIterationExhaustedReviewInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.RequestProjectTaskIterationExhaustedReview(ctx, input)
}

func (a *Activities) RequestUpstreamSupplementReview(ctx context.Context, input RequestUpstreamSupplementReviewInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.RequestUpstreamSupplementReview(ctx, input)
}

func (a *Activities) ApplyProjectAcceptanceDecision(ctx context.Context, input ApplyProjectAcceptanceDecisionInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	return a.store.ApplyProjectAcceptanceDecision(ctx, input)
}

func (a *Activities) HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.HoldDownstreamForFailure(ctx, input)
}

func (a *Activities) ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) (ApplyFailureRecoveryDecisionResult, error) {
	if a.store == nil {
		return ApplyFailureRecoveryDecisionResult{}, ErrActivityStoreRequired
	}
	return a.store.ApplyFailureRecoveryDecision(ctx, input)
}

func (a *Activities) ApplyPreDispatchGateDecision(ctx context.Context, input ApplyPreDispatchGateDecisionInput) (ApplyPreDispatchGateDecisionResult, error) {
	if a.store == nil {
		return ApplyPreDispatchGateDecisionResult{}, ErrActivityStoreRequired
	}
	return a.store.ApplyPreDispatchGateDecision(ctx, input)
}

func (a *Activities) AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error) {
	if a.store == nil {
		return ProjectEventResult{}, ErrActivityStoreRequired
	}
	return a.store.AppendProjectEvent(ctx, input)
}

func (a *Activities) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	input.DispatchReason = defaultDispatchReason(input.DispatchReason)
	err := a.store.DispatchProjectTask(ctx, input)
	if err != nil && !dispatchErrorRetryable(err) {
		return temporal.NewNonRetryableApplicationError("project task dispatch rejected", "ProjectTaskDispatchTerminal", err)
	}
	return err
}

func (a *Activities) RecoverTaskDispatchFailure(ctx context.Context, input RecoverTaskDispatchFailureInput) (RecoverTaskDispatchFailureResult, error) {
	if a.store == nil {
		return RecoverTaskDispatchFailureResult{}, ErrActivityStoreRequired
	}
	return a.store.RecoverTaskDispatchFailure(ctx, input)
}

func (a *Activities) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	return a.store.FinishCoordinationJob(ctx, input)
}
