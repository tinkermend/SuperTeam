//go:build adversarial_live

// Package projectcoordination_test's live gate harness (autonomy posture Phase B).
//
// This is a REAL end-to-end verification of the adversarial AI-review mechanism.
// It is guarded by BOTH a build tag (adversarial_live) AND an env flag
// (ADVERSARIAL_LIVE=1) so it never runs in normal CI. It makes REAL calls to a
// real deepseek model and writes REAL rows to a real (throwaway) Postgres.
//
// Run:
//
//	ADVERSARIAL_LIVE=1 \
//	ADV_BASE_URL=https://api.deepseek.com/v1 \
//	ADV_API_KEY=sk-... \
//	ADV_MODEL=deepseek-v4-pro \
//	ADV_PG_DSN='postgres://postgres@/superteam_adv_test?host=/tmp&port=55432' \
//	go test -tags adversarial_live -run TestAdversarialLive -v ./internal/workflow/projectcoordination/
package projectcoordination_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/storage/queries"
	pc "github.com/superteam/control-plane/internal/workflow/projectcoordination"
)

const liveCriterion = "所有新增导出函数都有覆盖边界条件的单元测试，且测试真实断言行为"

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("ADVERSARIAL_LIVE") != "1" {
		t.Skip("set ADVERSARIAL_LIVE=1 to run the live adversarial-review harness")
	}
}

// --- Part 1: real judge discrimination via real deepseek ---

func TestAdversarialLive_DeepseekDiscrimination(t *testing.T) {
	requireLive(t)
	baseURL := envOr("ADV_BASE_URL", "https://api.deepseek.com/v1")
	apiKey := os.Getenv("ADV_API_KEY")
	model := envOr("ADV_MODEL", "deepseek-v4-pro")
	if apiKey == "" {
		t.Fatal("ADV_API_KEY is required (do not hardcode the secret in the test)")
	}

	client := pc.NewOpenAICompatibleChatCompletionClient(baseURL, apiKey, 120*time.Second)
	activities := pc.WithJudgeClient(pc.NewActivities(nil), client, model)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// BAD output: no tests at all; a "test" that asserts nothing.
	badInput := pc.RunAdversarialReviewInput{
		TenantID:       uuid.New(),
		ProjectID:      uuid.New(),
		CriterionID:    "adv_tests_boundary",
		ReviewedTaskID: uuid.New(),
		Assertion:      liveCriterion,
		EvidenceSummary: "新增导出函数 ParseRange(s string) (int,int,error) 与 NormalizeScore(v float64) int。" +
			"提交里没有任何 *_test.go 文件；唯一的 TestParseRange 只调用了函数、丢弃了返回值、没有任何 assert/require/if err!=nil 断言，" +
			"边界情况（空字符串、越界、NaN、负数）完全没有覆盖。没有测试命令输出，没有覆盖率报告。",
		Deliverables:     []string{"parser.go", "score.go"},
		EvidenceRefs:     []string{},
		JudgeCountPolicy: 3,
		Model:            model,
	}
	badRes, err := activities.RunAdversarialReview(ctx, badInput)
	if err != nil {
		t.Fatalf("BAD scenario judge call failed (deepseek unreachable / auth?): %v", err)
	}
	logResult(t, "BAD", badRes)
	if badRes.Aggregate != pc.AdversarialAggregateUnsatisfied {
		t.Errorf("BAD: expected aggregate %q, got %q (RefutedCount=%d/%d)",
			pc.AdversarialAggregateUnsatisfied, badRes.Aggregate, badRes.RefutedCount, badRes.JudgeCount)
	}
	if badRes.RefutedCount < 2 {
		t.Errorf("BAD: expected RefutedCount>=2, got %d/%d", badRes.RefutedCount, badRes.JudgeCount)
	}

	// GOOD output: real tests present, asserting behavior, covering boundaries.
	goodInput := badInput
	goodInput.ReviewedTaskID = uuid.New()
	goodInput.EvidenceSummary = "新增导出函数 ParseRange(s string) (int,int,error) 与 NormalizeScore(v float64) int。" +
		"提交包含 parser_test.go 与 score_test.go：TestParseRange 用表驱动断言了正常区间(\"3-7\"→3,7)、" +
		"空字符串返回 ErrEmpty、越界\"10-2\"返回 ErrInverted、非数字\"a-b\"返回 err，均用 require.ErrorIs / require.Equal 真实断言返回值；" +
		"TestNormalizeScore 断言了 NaN→0、-5→0、150→100、边界 0 与 100 原样返回。go test ./... 全绿，覆盖率 96%。"
	goodInput.Deliverables = []string{"parser.go", "score.go", "parser_test.go", "score_test.go"}
	goodInput.EvidenceRefs = []string{"ci://run/8842/go-test.log", "artifact://coverage/parser.html"}
	goodRes, err := activities.RunAdversarialReview(ctx, goodInput)
	if err != nil {
		t.Fatalf("GOOD scenario judge call failed: %v", err)
	}
	logResult(t, "GOOD", goodRes)
	if goodRes.Aggregate != pc.AdversarialAggregateSatisfied {
		t.Errorf("GOOD: expected aggregate %q, got %q (RefutedCount=%d/%d) -- real finding about judge leniency/prompt strength if this fails",
			pc.AdversarialAggregateSatisfied, goodRes.Aggregate, goodRes.RefutedCount, goodRes.JudgeCount)
	}
	if goodRes.RefutedCount >= 2 {
		t.Errorf("GOOD: expected RefutedCount<2, got %d/%d", goodRes.RefutedCount, goodRes.JudgeCount)
	}
}

func logResult(t *testing.T, label string, r pc.AdversarialReviewResult) {
	t.Helper()
	t.Logf("=== %s scenario: aggregate=%s refuted=%d/%d ===", label, r.Aggregate, r.RefutedCount, r.JudgeCount)
	for i, j := range r.Judgements {
		t.Logf("  [%s judge %d] lens=%-16s verdict=%-8s reason=%s", label, i+1, j.Lens, j.Verdict, j.Reason)
	}
}

// --- Part 2: projection + gate + human override via real Postgres ---

func TestAdversarialLive_GatePersistenceAndOverride(t *testing.T) {
	requireLive(t)
	dsn := os.Getenv("ADV_PG_DSN")
	if dsn == "" {
		t.Fatal("ADV_PG_DSN is required (throwaway local Postgres, NOT the shared dev DB)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect throwaway pg: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping throwaway pg: %v", err)
	}

	repo := project.NewPgRepository(queries.New(pool))

	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	planRevID := uuid.New()
	reviewedTaskID := uuid.New()

	const (
		cBad = "c_bad_unsatisfied"
		cGd  = "c_good_satisfied"
		cEsc = "c_escalate_human"
	)

	// Seed three blocking adversarial_review criteria.
	mkCriterion := func(id string) project.CreateDemandAcceptanceCriterionRequest {
		return project.CreateDemandAcceptanceCriterionRequest{
			TenantID:           tenantID,
			ProjectID:          projectID,
			DemandID:           demandID,
			PlanRevisionID:     planRevID,
			CriterionID:        id,
			Statement:          liveCriterion,
			VerificationMethod: "adversarial_review",
			Severity:           "blocking",
			SatisfiedBy:        []string{"impl_task"},
		}
	}
	if err := repo.CreateDemandAcceptanceCriteria(ctx, []project.CreateDemandAcceptanceCriterionRequest{
		mkCriterion(cBad), mkCriterion(cGd), mkCriterion(cEsc),
	}); err != nil {
		t.Fatalf("seed criteria: %v", err)
	}

	// gate returns the current set of pending (held) blocking criteria, read
	// back from the real tables through the real gate resolver.
	gate := func() map[string]bool {
		criteria, err := repo.ListDemandAcceptanceCriteria(ctx, tenantID, demandID, planRevID)
		if err != nil {
			t.Fatalf("list criteria: %v", err)
		}
		verdicts, err := repo.ListDemandCriterionVerdicts(ctx, tenantID, demandID, planRevID)
		if err != nil {
			t.Fatalf("list verdicts: %v", err)
		}
		pending := project.ResolveUnsatisfiedBlockingCriteria(criteria, verdicts)
		set := map[string]bool{}
		for _, id := range pending {
			set[id] = true
		}
		t.Logf("gate pending(held)=%v", pending)
		return set
	}

	persistAgg := func(criterionID, aggregate string, judgements []pc.AdversarialJudgement) {
		if err := repo.CreateAdversarialVerdict(ctx, project.CreateAdversarialVerdictRequest{
			TenantID:       tenantID,
			ProjectID:      projectID,
			DemandID:       demandID,
			PlanRevisionID: planRevID,
			CriterionID:    criterionID,
			Verdict:        aggregate,
			JudgeID:        uuid.Nil,
			Reason:         aggregate,
			EvidenceRefs:   []string{},
		}); err != nil {
			t.Fatalf("persist adversarial verdict %s=%s: %v", criterionID, aggregate, err)
		}
		reqs := make([]project.CreateAdversarialJudgementRequest, 0, len(judgements))
		for _, j := range judgements {
			reqs = append(reqs, project.CreateAdversarialJudgementRequest{
				TenantID: tenantID, ProjectID: projectID, DemandID: demandID,
				PlanRevisionID: planRevID, CriterionID: criterionID, ReviewedTaskID: reviewedTaskID,
				Lens: j.Lens, Verdict: j.Verdict, Reason: j.Reason,
			})
		}
		if len(reqs) > 0 {
			if err := repo.CreateAdversarialJudgements(ctx, reqs); err != nil {
				t.Fatalf("persist judgements %s: %v", criterionID, err)
			}
		}
	}

	// Baseline: no verdicts yet -> all three blocking criteria held.
	if g := gate(); !(g[cBad] && g[cGd] && g[cEsc]) {
		t.Fatalf("baseline: expected all three held, got %v", g)
	}

	// BAD -> unsatisfied aggregate -> held.
	persistAgg(cBad, pc.AdversarialAggregateUnsatisfied, []pc.AdversarialJudgement{
		{Lens: "correctness", Verdict: "refuted", Reason: "无测试断言"},
		{Lens: "security", Verdict: "refuted", Reason: "无法核对"},
		{Lens: "reproducibility", Verdict: "accepted", Reason: "有列出文件"},
	})
	if g := gate(); !g[cBad] {
		t.Errorf("BAD unsatisfied: expected criterion HELD, but gate released it")
	}

	// GOOD -> satisfied aggregate -> released.
	persistAgg(cGd, pc.AdversarialAggregateSatisfied, []pc.AdversarialJudgement{
		{Lens: "correctness", Verdict: "accepted", Reason: "边界断言充分"},
		{Lens: "security", Verdict: "accepted", Reason: "无风险"},
		{Lens: "reproducibility", Verdict: "accepted", Reason: "有CI日志"},
	})
	if g := gate(); g[cGd] {
		t.Errorf("GOOD satisfied: expected criterion RELEASED, but gate held it")
	}

	// ESCALATE_HUMAN -> budget-exhausted aggregate -> held.
	persistAgg(cEsc, pc.AdversarialAggregateEscalateHuman, nil)
	if g := gate(); !g[cEsc] {
		t.Errorf("escalate_human: expected criterion HELD, but gate released it")
	}

	// Human override on the held escalate_human criterion -> released (human precedence).
	if err := repo.CreateDemandCriterionVerdict(ctx, project.CreateDemandCriterionVerdictRequest{
		TenantID:       tenantID,
		ProjectID:      projectID,
		DemandID:       demandID,
		PlanRevisionID: planRevID,
		CriterionID:    cEsc,
		Verdict:        "satisfied",
		JudgeType:      "human",
		JudgeID:        uuid.New(),
		Reason:         "人类负责人复核后判定满足，覆盖对抗预算熔断",
		EvidenceRefs:   []string{"manual-review://owner/ok"},
		ProjectTaskID:  nil,
	}); err != nil {
		t.Fatalf("human override verdict: %v", err)
	}
	if g := gate(); g[cEsc] {
		t.Errorf("human override: expected escalate_human criterion RELEASED by human satisfied verdict, but gate still held it")
	}

	// Final: only the BAD unsatisfied criterion should remain held.
	final := gate()
	if !final[cBad] {
		t.Errorf("final: BAD criterion should still be held")
	}
	if final[cGd] || final[cEsc] {
		t.Errorf("final: good/escalate-then-overridden criteria should be released, got held=%v", final)
	}
}
