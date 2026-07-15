package projectcoordination

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/temporal"
)

// errPlanner is a RoutePlanner stub that returns a fixed error (or plan), and
// counts how many times it was invoked — enough to prove the ErrNoSuitableEmployee
// family is not retried.
type errPlanner struct {
	err   error
	plan  RouteDecisionPlan
	calls atomic.Int32
}

func (p *errPlanner) Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	p.calls.Add(1)
	return p.plan, p.err
}

// sequencedPlanner returns a fixed error on the Nth (1-indexed) planning call and
// delegates to inner on every other call. It is used to drive a first plan that
// succeeds and a later replan that fails terminally.
type sequencedPlanner struct {
	inner    RoutePlanner
	failOn   int
	failWith error
	calls    atomic.Int32
}

func (p *sequencedPlanner) Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	if int(p.calls.Add(1)) == p.failOn {
		return RouteDecisionPlan{}, p.failWith
	}
	return p.inner.Plan(ctx, snapshot)
}

func TestPlanDemandRouteMarksNoSuitableEmployeeNonRetryable(t *testing.T) {
	planErr := fmt.Errorf("%w: task %q: employee scored 0.30", ErrNoSuitableEmployee, "review")
	activities := NewActivities(nil, &errPlanner{err: planErr})

	_, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{})

	require.Error(t, err)
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.True(t, appErr.NonRetryable())
	require.Equal(t, errTypeNoSuitableEmployee, appErr.Type())
	require.ErrorIs(t, err, ErrNoSuitableEmployee)
}

// TestStructuralGapErrorCarriesDetails proves the structured PlanningGap survives
// the real governance-detection path (EnforceScenarioTemplateGovernance on a
// single-employee software_delivery fixture, same as
// TestGovernanceRoleIndependenceStructuralGapEscalates) through
// wrapNoSuitableEmployeeError's ApplicationError Details attachment, without a live
// Temporal test environment.
func TestStructuralGapErrorCarriesDetails(t *testing.T) {
	employeeA := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "structural gap",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA),
	}
	planErr := EnforceScenarioTemplateGovernance(snapshot, &plan)
	require.ErrorIs(t, planErr, ErrNoSuitableEmployee)

	activities := NewActivities(nil, &errPlanner{err: planErr})

	_, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{})

	require.Error(t, err)
	require.ErrorIs(t, err, ErrNoSuitableEmployee)
	var appErr *temporal.ApplicationError
	require.True(t, errors.As(err, &appErr))
	require.True(t, appErr.HasDetails())
	var gap PlanningGap
	require.NoError(t, appErr.Details(&gap))
	require.Equal(t, "role_independence", gap.ConstraintKind)
	require.Equal(t, []string{"reviewer", "developer"}, gap.Roles)
	require.ElementsMatch(t, []string{"code_review", "code_implementation"}, gap.RequiredCapabilities)
	require.Equal(t, 1, gap.ActiveExecutorCount)
	require.Equal(t, []string{"restaff", "exempt", "lending"}, gap.Options)
}

func TestPlanDemandRoutePassesThroughNonFamilyErrorsRetryable(t *testing.T) {
	planErr := errors.New("planner request timeout")
	activities := NewActivities(nil, &errPlanner{err: planErr})

	_, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{})

	require.ErrorIs(t, err, planErr)
	var appErr *temporal.ApplicationError
	require.False(t, errors.As(err, &appErr))
}
