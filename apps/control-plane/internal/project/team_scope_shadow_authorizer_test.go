package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/authz"
)

type staticTeamScopeAuthorizer struct {
	allowed bool
	err     error
}

func (a staticTeamScopeAuthorizer) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	return a.allowed, a.err
}

type staticTeamScopeOpenFGAChecker struct {
	allowed bool
	checks  []authz.OpenFGACheck
}

func (c *staticTeamScopeOpenFGAChecker) Check(ctx context.Context, check authz.OpenFGACheck) (bool, error) {
	c.checks = append(c.checks, check)
	return c.allowed, nil
}

type recordingDecisionRecorder struct {
	records []authz.DecisionRecord
}

func (r *recordingDecisionRecorder) RecordDecision(ctx context.Context, record authz.DecisionRecord) error {
	r.records = append(r.records, record)
	return nil
}

func TestTeamScopeShadowAuthorizerKeepsDBDecisionAndRecordsDiff(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	teamID := uuid.MustParse("00000000-0000-4000-8000-000000000010")
	recorder := &recordingDecisionRecorder{}
	checker := &staticTeamScopeOpenFGAChecker{allowed: false}
	authorizer := NewTeamScopeShadowAuthorizer(
		staticTeamScopeAuthorizer{allowed: true},
		checker,
		TeamScopeShadowOptions{Recorder: recorder, StoreID: "store-1", ModelID: "model-1"},
	)

	allowed, err := authorizer.CanUseTeamForProject(context.Background(), tenantID, userID, teamID)

	require.NoError(t, err)
	require.True(t, allowed)
	require.Len(t, checker.checks, 1)
	require.Equal(t, "user:"+userID.String(), checker.checks[0].User)
	require.Equal(t, authz.OpenFGARelationProjectScopeUser, checker.checks[0].Relation)
	require.Equal(t, "team:"+teamID.String(), checker.checks[0].Object)
	require.Len(t, recorder.records, 1)
	require.Equal(t, "openfga_shadow", recorder.records[0].Engine)
	require.Equal(t, true, recorder.records[0].Snapshot["diff"])
	require.Equal(t, true, recorder.records[0].Snapshot["db_allowed"])
	require.Equal(t, false, recorder.records[0].Snapshot["openfga_allowed"])
}
