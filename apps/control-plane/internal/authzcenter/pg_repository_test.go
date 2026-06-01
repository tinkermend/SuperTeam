package authzcenter

import (
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestDecisionFromOperationLogExtractsNestedDecisionDetails(t *testing.T) {
	record := decisionFromOperationLog(queries.WebOperationLog{
		ID:       uuid.MustParse("00000000-0000-0000-0000-000000000101"),
		TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Module:   OperationModuleAuthz,
		Action:   authz.ActionTaskClaim,
		Result:   OperationResultFailed,
		Details: []byte(`{
			"actor": {"type": "runtime_node", "id": "node-1"},
			"decision": {
				"engine": "db",
				"reason": "runtime scope does not cover task",
				"matched_rule": "runtime.scope"
			}
		}`),
	})

	assertStringPtr(t, record.ActorType, authz.ActorRuntimeNode)
	assertStringPtr(t, record.ActorID, "node-1")
	assertStringPtr(t, record.Engine, "db")
	assertStringPtr(t, record.Reason, authz.ReasonRuntimeScopeMissing)
	assertStringPtr(t, record.MatchedRule, "runtime.scope")
}

func TestDecisionFromOperationLogPrefersFlatDecisionDetails(t *testing.T) {
	record := decisionFromOperationLog(queries.WebOperationLog{
		ID:       uuid.MustParse("00000000-0000-0000-0000-000000000102"),
		TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Module:   OperationModuleAuthz,
		Action:   authz.ActionTenantAccess,
		Result:   OperationResultSucceeded,
		Details: []byte(`{
			"engine": "flat-db",
			"reason": "flat-reason",
			"matched_rule": "flat-rule",
			"decision": {
				"engine": "nested-db",
				"reason": "nested-reason",
				"matched_rule": "nested-rule"
			}
		}`),
	})

	assertStringPtr(t, record.Engine, "flat-db")
	assertStringPtr(t, record.Reason, "flat-reason")
	assertStringPtr(t, record.MatchedRule, "flat-rule")
}

func assertStringPtr(t *testing.T, value *string, want string) {
	t.Helper()
	if value == nil {
		t.Fatalf("expected %q, got nil", want)
	}
	if *value != want {
		t.Fatalf("expected %q, got %q", want, *value)
	}
}
