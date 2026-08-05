//go:build live_e2e

package projectcoordination

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// H7: demand already has open casting_expansion → MaybeRequest returns already_pending
// and does not open a second card (active idempotency, not observational).
//
//	DATABASE_URL=... LIVE_PROJECT_ID=... LIVE_TASK_ID=... \
//	go test -tags=live_e2e ./internal/workflow/projectcoordination/ -run TestLiveH7AlreadyPending -count=1 -v
func TestLiveH7AlreadyPending(t *testing.T) {
	dbURL := firstNonEmpty(os.Getenv("DATABASE_URL"), os.Getenv("POSTGRES_URL"))
	projectID, err := uuid.Parse(strings.TrimSpace(os.Getenv("LIVE_PROJECT_ID")))
	if err != nil {
		t.Skip("LIVE_PROJECT_ID")
	}
	taskID, err := uuid.Parse(strings.TrimSpace(os.Getenv("LIVE_TASK_ID")))
	if err != nil {
		t.Skip("LIVE_TASK_ID")
	}
	if dbURL == "" {
		t.Skip("DATABASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, _ = pool.Exec(ctx, "SET search_path TO superteam, public")

	var tenantID, demandID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT tenant_id, demand_id FROM project_tasks WHERE id=$1`, taskID).Scan(&tenantID, &demandID); err != nil {
		t.Fatalf("task: %v", err)
	}

	var openN int
	_ = pool.QueryRow(ctx, `
		SELECT count(*) FROM inbox_items
		WHERE source_project_id=$1 AND status='open'
		  AND context_payload->>'demand_id'=$2
		  AND (context_payload->>'decision_type'='casting_expansion'
		       OR context_payload->>'kind'='casting_expansion')
	`, projectID, demandID.String()).Scan(&openN)
	if openN < 1 {
		// Try re-open latest decision for this demand
		_, _ = pool.Exec(ctx, `
			UPDATE project_decision_requests d
			SET status_snapshot='pending', updated_at=now()
			FROM approval_requests a
			WHERE d.approval_request_id=a.id
			  AND d.project_id=$1 AND d.decision_type='casting_expansion'
			  AND a.context_payload->>'demand_id'=$2
		`, projectID, demandID.String())
		_, _ = pool.Exec(ctx, `
			UPDATE approval_requests
			SET status='pending', updated_at=now()
			WHERE resource_id=$1 AND decision_type='casting_expansion'
			  AND context_payload->>'demand_id'=$2
		`, projectID, demandID.String())
		_, _ = pool.Exec(ctx, `
			UPDATE inbox_items SET status='open', updated_at=now()
			WHERE source_project_id=$1 AND context_payload->>'demand_id'=$2
			  AND (context_payload->>'decision_type'='casting_expansion'
			       OR context_payload->>'kind'='casting_expansion')
		`, projectID, demandID.String())
		_ = pool.QueryRow(ctx, `
			SELECT count(*) FROM inbox_items
			WHERE source_project_id=$1 AND status='open'
			  AND context_payload->>'demand_id'=$2
			  AND (context_payload->>'decision_type'='casting_expansion'
			       OR context_payload->>'kind'='casting_expansion')
		`, projectID, demandID.String()).Scan(&openN)
		if openN < 1 {
			t.Skip("no open casting_expansion for demand — need H1 card first")
		}
	}

	q := queries.New(pool)
	repo := project.NewPgRepository(q, pool)
	approvalSvc, err := approval.NewService(approval.NewPgRepository(q))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(
		repo,
		project.NoopCoordinatorSignalClient{},
		project.NewApprovalServiceAdapter(approvalSvc),
		liveFakeInbox{},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetCastingRepository(project.NewPgCastingRepository(q, pool))
	svc.SetScenarioTemplateSpecSource(liveSpecSource{q: q})
	svc.SetRoleVocabularyLister(liveRoleLister{q: q})
	// If hasOpen fails, discoverer would fire — use a trap.
	trap := &trapDiscoverer{}
	svc.SetCastingGapDiscoverer(trap)

	out, err := svc.MaybeRequestCastingExpansionForCompletedTask(ctx, tenantID, projectID, taskID)
	if err != nil {
		t.Fatalf("MaybeRequest: %v", err)
	}
	t.Logf("out=%+v trap_calls=%d", out, trap.calls)
	if out.Requested {
		t.Fatalf("must not open second expansion")
	}
	if out.SkippedReason != "already_pending" {
		t.Fatalf("want already_pending got %q", out.SkippedReason)
	}
	if trap.calls != 0 {
		t.Fatalf("discoverer must not run when expansion already pending (calls=%d)", trap.calls)
	}

	var after int
	_ = pool.QueryRow(ctx, `
		SELECT count(*) FROM inbox_items
		WHERE source_project_id=$1 AND status='open'
		  AND context_payload->>'demand_id'=$2
		  AND (context_payload->>'decision_type'='casting_expansion'
		       OR context_payload->>'kind'='casting_expansion')
	`, projectID, demandID.String()).Scan(&after)
	if after != openN {
		t.Fatalf("open cards changed %d → %d", openN, after)
	}
}

type trapDiscoverer struct{ calls int }

func (t *trapDiscoverer) DiscoverCastingGap(ctx context.Context, in project.CastingGapInput) (project.CastingGapSuggestion, error) {
	t.calls++
	return project.CastingGapSuggestion{Needed: true, RoleKey: "reviewer", Reason: "trap"}, nil
}

type liveFakeInbox struct{}

func (liveFakeInbox) UpsertProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	return nil
}

func (liveFakeInbox) ResolveProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	return nil
}
