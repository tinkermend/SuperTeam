package projectcoordination

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"go.temporal.io/sdk/temporal"
)

var ErrActivityStoreRequired = errors.New("project coordination activity store is required")

type Activities struct {
	store   ActivityStore
	planner RoutePlanner
}

type ActivityStore interface {
	LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error)
	CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error)
	PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error)
	CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error)
	ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error)
	ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error)
	HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error)
	ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) error
	RequestRouteDecisionReview(ctx context.Context, input RequestRouteDecisionReviewInput) (DecisionRequestResult, error)
	AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error)
	DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error
	FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error
}

func NewActivities(store ActivityStore, planner ...RoutePlanner) *Activities {
	selected := RoutePlanner(HeuristicRoutePlanner{})
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
	planner := a.planner
	if planner == nil {
		planner = HeuristicRoutePlanner{}
	}
	plan, err := planner.Plan(ctx, snapshot)
	if err == nil {
		return plan, nil
	}
	if contextErr := terminalContextError(ctx, err); contextErr != nil {
		return RouteDecisionPlan{}, contextErr
	}
	return HeuristicRoutePlanner{}.Plan(ctx, snapshot)
}

func (a *Activities) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	if a.store == nil {
		return RouteDecisionResult{}, ErrActivityStoreRequired
	}
	return a.store.PersistRouteDecision(ctx, input)
}

func (a *Activities) CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error) {
	if a.store == nil {
		return nil, ErrActivityStoreRequired
	}
	return a.store.CreateProjectTasks(ctx, input)
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

func (a *Activities) HoldDownstreamForFailure(ctx context.Context, input HoldDownstreamForFailureInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.HoldDownstreamForFailure(ctx, input)
}

func (a *Activities) ApplyFailureRecoveryDecision(ctx context.Context, input ApplyFailureRecoveryDecisionInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	return a.store.ApplyFailureRecoveryDecision(ctx, input)
}

func (a *Activities) RequestRouteDecisionReview(ctx context.Context, input RequestRouteDecisionReviewInput) (DecisionRequestResult, error) {
	if a.store == nil {
		return DecisionRequestResult{}, ErrActivityStoreRequired
	}
	return a.store.RequestRouteDecisionReview(ctx, input)
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
	err := a.store.DispatchProjectTask(ctx, input)
	if err != nil && !dispatchErrorRetryable(err) {
		return temporal.NewNonRetryableApplicationError("project task dispatch rejected", "ProjectTaskDispatchTerminal", err)
	}
	return err
}

func (a *Activities) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	if a.store == nil {
		return ErrActivityStoreRequired
	}
	return a.store.FinishCoordinationJob(ctx, input)
}
