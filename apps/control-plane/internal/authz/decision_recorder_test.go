package authz

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type fakeOperationLogQueries struct {
	params queries.CreateWebOperationLogParams
	called bool
}

func (q *fakeOperationLogQueries) CreateWebOperationLog(ctx context.Context, params queries.CreateWebOperationLogParams) (queries.WebOperationLog, error) {
	q.params = params
	q.called = true
	return queries.WebOperationLog{}, nil
}

func TestOperationLogDecisionRecorderPersistsDecision(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	query := &fakeOperationLogQueries{}
	recorder := NewOperationLogDecisionRecorder(query)

	err := recorder.RecordDecision(context.Background(), DecisionRecord{
		TenantID:     tenantID,
		ActorType:    ActorUser,
		ActorID:      "00000000-0000-4000-8000-000000000001",
		Action:       ActionConsoleAccess,
		ResourceType: ResourceConsole,
		ResourceID:   "web",
		Allowed:      false,
		Reason:       ReasonNoMembership,
		MatchedRule:  "",
		Engine:       "db",
		Snapshot: map[string]any{
			"engine": "db",
		},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !query.called {
		t.Fatal("expected operation log query to be called")
	}
	if query.params.Module != ModuleAuthz {
		t.Fatalf("expected authz module, got %q", query.params.Module)
	}
	if query.params.Action != ActionConsoleAccess {
		t.Fatalf("expected console access action, got %q", query.params.Action)
	}
	if query.params.Result != ResultFailed {
		t.Fatalf("expected failed result for denied decision, got %q", query.params.Result)
	}
	if len(query.params.Details) == 0 {
		t.Fatal("expected details json to be present")
	}
	var details map[string]any
	if err := json.Unmarshal(query.params.Details, &details); err != nil {
		t.Fatalf("decode details: %v", err)
	}
	if details["reason"] != ReasonNoMembership || details["engine"] != "db" {
		t.Fatalf("unexpected details: %#v", details)
	}
}

func TestOperationLogDecisionRecorderUsesSucceededResultForAllowedDecision(t *testing.T) {
	query := &fakeOperationLogQueries{}
	recorder := NewOperationLogDecisionRecorder(query)

	err := recorder.RecordDecision(context.Background(), DecisionRecord{
		TenantID:     uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		ActorType:    ActorRuntimeNode,
		ActorID:      "node-1",
		Action:       ActionTaskClaim,
		ResourceType: ResourceTask,
		ResourceID:   "00000000-0000-4000-8000-000000000042",
		Allowed:      true,
		Reason:       ReasonAllowed,
		MatchedRule:  "runtime.scope",
		Engine:       "db",
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if query.params.Result != ResultSucceeded {
		t.Fatalf("expected succeeded result, got %q", query.params.Result)
	}
}

func TestOperationLogDecisionRecorderNilQueryIsNoop(t *testing.T) {
	recorder := NewOperationLogDecisionRecorder(nil)
	err := recorder.RecordDecision(context.Background(), DecisionRecord{
		TenantID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Action:   ActionConsoleAccess,
	})
	if err != nil {
		t.Fatalf("expected nil query recorder to be a no-op, got %v", err)
	}
}
