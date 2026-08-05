//go:build live_e2e

package projectcoordination

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/scenariotemplate"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// Plant conclusion + re-run discoverer with REAL approval+inbox projectors so the
// judge casting_expansion appears in the shared inbox (same DB as running CP).
//
//	DATABASE_URL=... PLANNER_API_KEY=... LIVE_PROJECT_ID=... LIVE_TASK_ID=... \
//	go test -tags=live_e2e ./internal/workflow/projectcoordination/ \
//	  -run TestLiveRetriggerCastingGapDiscoverer -count=1 -timeout 3m -v
func TestLiveRetriggerCastingGapDiscoverer(t *testing.T) {
	dbURL := firstNonEmpty(os.Getenv("DATABASE_URL"), os.Getenv("POSTGRES_URL"))
	apiKey := firstNonEmpty(os.Getenv("PLANNER_API_KEY"), os.Getenv("DEEPSEEK_API_KEY"))
	projectID, err := uuid.Parse(strings.TrimSpace(os.Getenv("LIVE_PROJECT_ID")))
	if err != nil {
		t.Skip("LIVE_PROJECT_ID required")
	}
	taskID, err := uuid.Parse(strings.TrimSpace(os.Getenv("LIVE_TASK_ID")))
	if err != nil {
		t.Skip("LIVE_TASK_ID required")
	}
	if dbURL == "" || apiKey == "" {
		t.Skip("DATABASE_URL and PLANNER_API_KEY required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("pg: %v", err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, "SET search_path TO superteam, public"); err != nil {
		t.Fatalf("search_path: %v", err)
	}

	var tenantID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT tenant_id FROM project_tasks WHERE id=$1`, taskID).Scan(&tenantID); err != nil {
		t.Fatalf("tenant: %v", err)
	}

	// Close any open casting_expansion for this demand so re-request can open a new one.
	// hasOpenCastingExpansionForDemand blocks when any pending casting_expansion
	// still points at the demand (approval context or leftover rows).
	var demandID uuid.UUID
	_ = pool.QueryRow(ctx, `SELECT demand_id FROM project_tasks WHERE id=$1`, taskID).Scan(&demandID)
	if demandID != uuid.Nil {
		// Only clear THIS demand's open expansions so H1 coordinator cards on other
		// demands remain for H9a side-by-side UI checks.
		_, _ = pool.Exec(ctx, `
			UPDATE project_decision_requests d
			SET status_snapshot = 'rejected', updated_at = now()
			FROM approval_requests a
			WHERE d.approval_request_id = a.id
			  AND d.project_id = $1
			  AND d.decision_type = 'casting_expansion'
			  AND d.status_snapshot = 'pending'
			  AND a.context_payload->>'demand_id' = $2
		`, projectID, demandID.String())
		_, _ = pool.Exec(ctx, `
			UPDATE inbox_items
			SET status = 'done', updated_at = now()
			WHERE source_project_id = $1
			  AND status = 'open'
			  AND (
			    context_payload->>'kind' = 'casting_expansion'
			    OR context_payload->>'decision_type' = 'casting_expansion'
			  )
			  AND context_payload->>'demand_id' = $2
		`, projectID, demandID.String())
		_, _ = pool.Exec(ctx, `
			UPDATE approval_requests
			SET status = 'rejected', updated_at = now()
			WHERE tenant_id = $1
			  AND resource_id = $2
			  AND decision_type = 'casting_expansion'
			  AND status = 'pending'
			  AND context_payload->>'demand_id' = $3
		`, tenantID, projectID, demandID.String())
		// Reset call budget so re-trigger can invoke the LLM again (dev-only).
		_, _ = pool.Exec(ctx, `
			DELETE FROM project_events
			WHERE project_id = $1
			  AND event_type = 'project.casting.gap_discovery'
			  AND payload->>'demand_id' = $2
		`, projectID, demandID.String())
	}

	conclusion := "应用侧无异常，疑似网络链路问题，需要法务合规侧进一步审查合同与数据出境条款"
	tag, err := pool.Exec(ctx, `
		UPDATE project_execution_summaries SET conclusion = $2 WHERE project_task_id = $1
	`, taskID, conclusion)
	if err != nil {
		t.Fatalf("plant: %v", err)
	}
	if tag.RowsAffected() == 0 {
		t.Fatalf("no summary for task %s", taskID)
	}

	q := queries.New(pool)
	projectRepo := project.NewPgRepository(q, pool)

	inboxRepo := inbox.NewPgRepository(q)
	inboxService, err := inbox.NewService(inboxRepo)
	if err != nil {
		t.Fatalf("inbox: %v", err)
	}
	decisionProjector := inbox.NewDecisionProjectorAdapter(inboxService)

	approvalRepo := approval.NewPgRepository(q)
	// Approval create also projects to inbox for approval-sourced items; casting_expansion
	// is decision-sourced and uses DecisionProjectorAdapter on project service.
	approvalService, err := approval.NewService(approvalRepo)
	if err != nil {
		t.Fatalf("approval: %v", err)
	}

	svc, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(
		projectRepo,
		project.NoopCoordinatorSignalClient{},
		project.NewApprovalServiceAdapter(approvalService),
		decisionProjector,
		nil,
	)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	svc.SetCastingRepository(project.NewPgCastingRepository(q, pool))
	svc.SetRoleVocabularyLister(liveRoleLister{q: q})
	svc.SetScenarioTemplateSpecSource(liveSpecSource{q: q})
	svc.SetCastingGapDiscoverer(NewCastingGapDiscoverer(
		NewOpenAICompatibleChatCompletionClient(
			firstNonEmpty(os.Getenv("PLANNER_BASE_URL"), "https://api.deepseek.com/v1"),
			apiKey,
			90*time.Second,
		),
		firstNonEmpty(os.Getenv("PLANNER_MODEL"), "deepseek-v4-pro"),
	))

	out, err := svc.MaybeRequestCastingExpansionForCompletedTask(ctx, tenantID, projectID, taskID)
	if err != nil {
		t.Fatalf("MaybeRequest: %v", err)
	}
	t.Logf("result=%+v", out)
	if !out.Requested {
		t.Fatalf("expected Requested, skip=%q", out.SkippedReason)
	}

	// Prove real inbox row exists (same DB the web CP reads).
	var inboxCount int
	err = pool.QueryRow(ctx, `
		SELECT count(*) FROM inbox_items
		WHERE source_project_id = $1
		  AND status = 'open'
		  AND (
		    context_payload->>'kind' = 'casting_expansion'
		    OR context_payload->>'decision_type' = 'casting_expansion'
		  )
		  AND (
		    context_payload->>'actor_type' = 'judge'
		    OR context_payload->>'demand_id' = $2
		  )
	`, projectID, out.DemandID.String()).Scan(&inboxCount)
	if err != nil {
		t.Fatalf("inbox count: %v", err)
	}
	if inboxCount < 1 {
		t.Fatalf("expected open casting_expansion inbox item for demand %s (count=%d)", out.DemandID, inboxCount)
	}
	t.Logf("inbox open casting_expansion count=%d decision=%s suggested=%s", inboxCount, out.DecisionID, out.SuggestedRoleKey)
}

type liveRoleLister struct{ q *queries.Queries }

func (r liveRoleLister) ListActiveRoleRows(ctx context.Context, tenantID uuid.UUID) ([]project.RoleVocabularyRow, error) {
	rows, err := r.q.ListActiveRoleVocabulary(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]project.RoleVocabularyRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, project.RoleVocabularyRow{
			RoleKey:     row.RoleKey,
			Title:       row.Title,
			Description: row.Description,
		})
	}
	return out, nil
}

type liveSpecSource struct{ q *queries.Queries }

func (s liveSpecSource) GetParsedSpec(ctx context.Context, tenantID uuid.UUID, key string) (scenariotemplate.SpecV2, string, error) {
	tpl, err := s.q.GetScenarioTemplateByKey(ctx, queries.GetScenarioTemplateByKeyParams{
		TenantID:    tenantID,
		TemplateKey: key,
	})
	if err != nil {
		return scenariotemplate.SpecV2{}, "", err
	}
	var raw map[string]any
	if len(tpl.Spec) > 0 {
		if err := json.Unmarshal(tpl.Spec, &raw); err != nil {
			return scenariotemplate.SpecV2{}, "", err
		}
	}
	spec, err := scenariotemplate.ParseSpec(raw)
	if err != nil {
		return scenariotemplate.SpecV2{}, "", err
	}
	return spec, tpl.Name, nil
}
