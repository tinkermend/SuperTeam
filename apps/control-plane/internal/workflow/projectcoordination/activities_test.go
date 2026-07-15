package projectcoordination

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"

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

func TestPlanDemandRoutePassesThroughNonFamilyErrorsRetryable(t *testing.T) {
	planErr := errors.New("planner request timeout")
	activities := NewActivities(nil, &errPlanner{err: planErr})

	_, err := activities.PlanDemandRoute(context.Background(), CoordinationSnapshot{})

	require.ErrorIs(t, err, planErr)
	var appErr *temporal.ApplicationError
	require.False(t, errors.As(err, &appErr))
}
