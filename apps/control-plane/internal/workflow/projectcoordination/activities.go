package projectcoordination

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
)

var ErrActivityStoreRequired = errors.New("project coordination activity store is required")

var ErrRoutePlannerRequired = errors.New("project coordination route planner is required")

// ErrJudgeClientRequired is returned by RunAdversarialReview when no judge chat
// client has been wired (mirrors ErrRoutePlannerRequired's nil-dep contract).
var ErrJudgeClientRequired = errors.New("project coordination adversarial judge client is required")

// errTypeNoSuitableEmployee is the temporal ApplicationError Type stamped on a
// terminal ErrNoSuitableEmployee planning failure. The workflow matches on this
// Type to route the demand to its rejection/diagnosis surface instead of retrying.
const errTypeNoSuitableEmployee = "NoSuitableEmployee"

type Activities struct {
	store       ActivityStore
	planner     RoutePlanner
	judgeClient chatCompletionClient
	judgeModel  string
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
	ReopenProjectDemandForReplanning(ctx context.Context, input ReopenProjectDemandForReplanningInput) error
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

// WithJudgeClient wires the adversarial-review judge chat client (the same
// chatCompletionClient seam the route planner uses). It is an optional,
// nil-tolerant wiring seam — mirroring NewActivities' variadic planner — so the
// many existing Activities constructions that predate adversarial review need no
// changes; only RunAdversarialReview requires it, and it errors clearly
// (ErrJudgeClientRequired) when it is missing. Returns a for chaining.
//
// It is deliberately a package-level function, NOT a method on Activities:
// worker.go registers the whole Activities struct with Temporal
// (w.RegisterActivity(activities)), which reflects over every EXPORTED METHOD and
// requires each to have a valid activity signature ((T, error) / error). An
// exported WithJudgeClient method returning *Activities would fail that
// registration. A package-level function is invisible to that reflection while
// still callable from the cross-package wiring in internal/app.
// The optional model argument sets the judge model id passed through to each
// judge call (RunAdversarialReviewInput.Model). It is variadic so existing
// two-argument callers (tests that only need the client) are unaffected.
func WithJudgeClient(a *Activities, client chatCompletionClient, model ...string) *Activities {
	if a != nil {
		a.judgeClient = client
		if len(model) > 0 {
			a.judgeModel = model[0]
		}
	}
	return a
}

// RunAdversarialReview decides one adversarial_review acceptance criterion by
// running N refute-style LLM judges (default 3, hard cap 7) and taking a
// majority-refute vote. Cost guardrail: when the reviewed task's revision/cost
// budget is already exhausted (Task 4 populates input.BudgetExhausted from
// (*ProjectStore).revisionBudgetExhausted), it short-circuits to an
// escalate_human result WITHOUT calling any judge, so Task 4 can route the
// criterion to a human tier-3 hold instead of burning more model spend. The
// engine itself (runAdversarialReview) is DB-free; this method only resolves the
// judge count, applies the budget guardrail, and delegates.
func (a *Activities) RunAdversarialReview(ctx context.Context, input RunAdversarialReviewInput) (AdversarialReviewResult, error) {
	if a.judgeClient == nil {
		return AdversarialReviewResult{}, ErrJudgeClientRequired
	}
	if input.BudgetExhausted {
		return AdversarialReviewResult{
			CriterionID:    input.CriterionID,
			ReviewedTaskID: input.ReviewedTaskID,
			Aggregate:      AdversarialAggregateEscalateHuman,
			RefutedCount:   0,
			JudgeCount:     0,
		}, nil
	}
	lenses := resolveAdversarialLenses(resolveJudgeCount(input.JudgeCountPolicy))
	return runAdversarialReview(ctx, a.judgeClient, lenses, input)
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

func (a *Activities) ReopenProjectDemandForReplanning(ctx context.Context, input ReopenProjectDemandForReplanningInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	return a.store.ReopenProjectDemandForReplanning(ctx, input)
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

// demandAcceptanceDecisionEnsurer is an optional ActivityStore capability
// (type-asserted, not part of the ActivityStore interface — mirrors
// taskResultDecisionInspector/revisionTaskCreator/upstreamSupplementTaskCreator
// in workflow.go) so the many existing ActivityStore test doubles that predate
// the demand-acceptance convergence gate need no changes. *ProjectStore
// implements it directly (see project_store.go).
type demandAcceptanceDecisionEnsurer interface {
	EnsureDemandAcceptanceDecisionForTask(ctx context.Context, input EnsureDemandAcceptanceDecisionForTaskInput) (DecisionRequestResult, error)
}

// EnsureDemandAcceptanceDecisionForTask probes whether the task's demand just
// converged to acceptance_pending and, if so, opens the demand_acceptance
// decision. Deliberately soft: an ActivityStore that doesn't implement
// demandAcceptanceDecisionEnsurer (a.store == nil, or a test double scoped to
// unrelated behavior) yields a no-op zero result rather than
// ErrActivityStoreRequired — unlike this file's other wrappers, opening this
// decision is an orthogonal add-on to task-completion routing, not a
// precondition for it, so a store that hasn't opted in must never fail the
// signal handler that's calling this as an extra step.
func (a *Activities) EnsureDemandAcceptanceDecisionForTask(ctx context.Context, input EnsureDemandAcceptanceDecisionForTaskInput) (DecisionRequestResult, error) {
	store, ok := a.store.(demandAcceptanceDecisionEnsurer)
	if a.store == nil || !ok {
		return DecisionRequestResult{}, nil
	}
	return store.EnsureDemandAcceptanceDecisionForTask(ctx, input)
}
