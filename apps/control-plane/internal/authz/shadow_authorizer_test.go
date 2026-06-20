package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type staticAuthorizer struct {
	decision Decision
	err      error
}

func (a staticAuthorizer) Check(ctx context.Context, req CheckRequest) (Decision, error) {
	return a.decision, a.err
}

type staticOpenFGAChecker struct {
	allowed bool
	err     error
	checks  []OpenFGACheck
}

func (c *staticOpenFGAChecker) Check(ctx context.Context, check OpenFGACheck) (bool, error) {
	c.checks = append(c.checks, check)
	return c.allowed, c.err
}

func TestShadowAuthorizerKeepsDBDecisionAndRecordsOpenFGADiff(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	recorder := &memoryRecorder{}
	checker := &staticOpenFGAChecker{allowed: false}
	authorizer := NewShadowAuthorizer(
		staticAuthorizer{decision: Decision{Allowed: true, Reason: ReasonAllowed, MatchedRule: "tenant.admin"}},
		checker,
		ShadowOptions{Recorder: recorder, StoreID: "store-1", ModelID: "model-1"},
	)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionAuthzCenterRead,
		Resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Len(t, checker.checks, 1)
	require.Len(t, recorder.records, 1)
	record := recorder.records[0]
	require.False(t, record.Allowed)
	require.Equal(t, "openfga_shadow", record.Engine)
	require.Equal(t, true, record.Snapshot["diff"])
	require.Equal(t, true, record.Snapshot["db_allowed"])
	require.Equal(t, false, record.Snapshot["openfga_allowed"])
	require.Equal(t, "store-1", record.Snapshot["openfga_store_id"])
	require.Equal(t, "model-1", record.Snapshot["openfga_model_id"])
}

func TestShadowAuthorizerKeepsDBDecisionWhenOpenFGAErrors(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	recorder := &memoryRecorder{}
	checker := &staticOpenFGAChecker{err: errors.New("openfga unavailable")}
	authorizer := NewShadowAuthorizer(
		staticAuthorizer{decision: Decision{Allowed: true, Reason: ReasonAllowed, MatchedRule: "tenant.admin"}},
		checker,
		ShadowOptions{Recorder: recorder, StoreID: "store-1", ModelID: "model-1"},
	)

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionAuthzCenterRead,
		Resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Len(t, recorder.records, 1)
	require.Equal(t, "openfga_shadow", recorder.records[0].Engine)
	require.Equal(t, "openfga unavailable", recorder.records[0].Snapshot["openfga_error"])
}
