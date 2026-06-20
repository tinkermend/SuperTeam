package authz

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOpenFGAAuthorizerAllowsMappedRequestWhenOpenFGAAllows(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	checker := &staticOpenFGAChecker{allowed: true}
	authorizer := NewOpenFGAAuthorizer(checker, OpenFGAAuthorizerOptions{StoreID: "store-1", ModelID: "model-1"})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionAuthzCenterRead,
		Resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
	})

	require.NoError(t, err)
	require.True(t, decision.Allowed)
	require.Equal(t, ReasonAllowed, decision.Reason)
	require.Equal(t, "openfga.admin", decision.MatchedRule)
	require.Equal(t, "openfga", decision.Snapshot["engine"])
	require.Equal(t, "store-1", decision.Snapshot["openfga_store_id"])
	require.Len(t, checker.checks, 1)
}

func TestOpenFGAAuthorizerFailsClosedWhenOpenFGAErrors(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	checker := &staticOpenFGAChecker{err: errors.New("openfga unavailable")}
	authorizer := NewOpenFGAAuthorizer(checker, OpenFGAAuthorizerOptions{StoreID: "store-1", ModelID: "model-1"})

	decision, err := authorizer.Check(context.Background(), CheckRequest{
		Actor:    ActorRef{Type: ActorUser, ID: userID.String()},
		Action:   ActionAuthzCenterRead,
		Resource: ResourceRef{Type: ResourceTenant, ID: tenantID.String()},
		TenantID: tenantID,
	})

	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.True(t, decision.RequiresAudit)
	require.Equal(t, "openfga check failed", decision.Reason)
	require.Equal(t, "openfga unavailable", decision.Snapshot["openfga_error"])
	require.Equal(t, "openfga", decision.Snapshot["engine"])
}
