package cards

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/superteam/feishu-connector/internal/cpclient"
)

func mustValid(t *testing.T, cardJSON string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(cardJSON), &parsed); err != nil {
		t.Fatalf("card must be valid json: %v\n%s", err, cardJSON)
	}
	return parsed
}

func TestPlanReviewCardHasApproveAndRequestChanges(t *testing.T) {
	cardJSON := DecisionCard(map[string]any{
		"decision_type": "plan_review",
		"title":         "登录接口加固计划",
		"summary":       "共 3 个任务,判据 4 条(2 automated / 1 human / 1 adversarial)",
		"risk_level":    "high",
	}, "dec-1", "proj-1", "http://web.local:3000")
	mustValid(t, cardJSON)
	for _, want := range []string{"approved", "request_changes", "resolve_decision", "dec-1", "proj-1", "登录接口加固计划", "计划评审", "red"} {
		if !strings.Contains(cardJSON, want) {
			t.Fatalf("plan_review card missing %q:\n%s", want, cardJSON)
		}
	}
}

func TestDemandAcceptanceCardIsDeepLinkOnly(t *testing.T) {
	cardJSON := DecisionCard(map[string]any{
		"decision_type": "demand_acceptance",
		"title":         "需求验收:登录加固",
	}, "dec-2", "proj-1", "http://web.local:3000")
	mustValid(t, cardJSON)
	if strings.Contains(cardJSON, "resolve_decision") {
		t.Fatalf("判据签署卡不得有卡内签署按钮(防橡皮图章):\n%s", cardJSON)
	}
	if !strings.Contains(cardJSON, "http://web.local:3000/inbox") {
		t.Fatalf("expected deep link, got:\n%s", cardJSON)
	}
}

func TestPlanningGapCardHasThreeVerbs(t *testing.T) {
	cardJSON := DecisionCard(map[string]any{
		"decision_type": "planning_gap",
		"title":         "缺后端执行员工",
	}, "dec-3", "proj-1", "http://web.local:3000")
	for _, want := range []string{"restaffed", "exempted", "rejected"} {
		if !strings.Contains(cardJSON, want) {
			t.Fatalf("planning_gap card missing %q", want)
		}
	}
}

func TestResultNoticeCardStates(t *testing.T) {
	done := ResultNoticeCard(map[string]any{"title": "登录加固", "status": "completed", "demand_id": "d-1"}, "http://web.local:3000")
	if !strings.Contains(done, "green") || !strings.Contains(done, "/workflows/d-1") {
		t.Fatalf("completed notice unexpected:\n%s", done)
	}
	failed := ResultNoticeCard(map[string]any{"title": "登录加固", "status": "failed", "demand_id": "d-2"}, "http://web.local:3000")
	if !strings.Contains(failed, "red") {
		t.Fatalf("failed notice must be red:\n%s", failed)
	}
	if strings.Contains(done, "resolve_decision") {
		t.Fatalf("result notice must be read-only")
	}
}

func TestProjectPickCardCapsAtTen(t *testing.T) {
	projects := make([]cpclient.MyProject, 0, 12)
	for i := 0; i < 12; i++ {
		projects = append(projects, cpclient.MyProject{ID: "p", Name: "项目"})
	}
	cardJSON := ProjectPickCard(projects)
	if strings.Count(cardJSON, "pick_project") != 10 {
		t.Fatalf("expected 10 project buttons, got %d", strings.Count(cardJSON, "pick_project"))
	}
	if !strings.Contains(cardJSON, "共 12 个") {
		t.Fatalf("expected overflow hint")
	}
}

func TestDecisionResolvedCardMentionsOutcome(t *testing.T) {
	cardJSON := DecisionResolvedCard(map[string]any{"title": "计划评审", "resolved_status": "approved"})
	if !strings.Contains(cardJSON, "批准") || !strings.Contains(cardJSON, "已处理") {
		t.Fatalf("resolved card unexpected:\n%s", cardJSON)
	}
}
