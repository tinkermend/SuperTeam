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

// 验收卡:不给一键整单批准(防橡皮图章),但每条判据可逐条签署,证据紧邻按钮。
func TestDemandAcceptanceCardSignsPerCriterion(t *testing.T) {
	cardJSON := DecisionCard(map[string]any{
		"decision_type": "demand_acceptance",
		"title":         "需求验收:登录加固",
		"context": map[string]any{
			"demand_id": "d-1",
			"pending_criteria_detail": []any{
				map[string]any{"id": "c1", "statement": "接口通过安全扫描", "evidence": []any{
					map[string]any{"title": "安全扫描任务", "status": "completed", "conclusion": "扫描 0 高危"},
				}},
			},
		},
	}, "dec-2", "proj-1", "http://web.local:3000")
	mustValid(t, cardJSON)
	if strings.Contains(cardJSON, "resolve_decision") {
		t.Fatalf("判据签署卡不得有整单一键批准(防橡皮图章):\n%s", cardJSON)
	}
	for _, want := range []string{"sign_criterion", "satisfied", "unsatisfied", "接口通过安全扫描", "扫描 0 高危", "http://web.local:3000/inbox"} {
		if !strings.Contains(cardJSON, want) {
			t.Fatalf("acceptance card missing %q:\n%s", want, cardJSON)
		}
	}
	// 无 context 时退化为深链卡,不渲染签署按钮。
	bare := DecisionCard(map[string]any{"decision_type": "demand_acceptance", "title": "需求验收:登录加固"}, "dec-2", "proj-1", "http://web.local:3000")
	mustValid(t, bare)
	if strings.Contains(bare, "sign_criterion") {
		t.Fatalf("bare acceptance card must not render sign buttons:\n%s", bare)
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
	cardJSON := DecisionResolvedCard(map[string]any{"title": "计划评审", "resolved_status": "approved"}, "http://web.local:3000")
	if !strings.Contains(cardJSON, "批准") || !strings.Contains(cardJSON, "已处理") {
		t.Fatalf("resolved card unexpected:\n%s", cardJSON)
	}
}

// 终态卡必须保留原卡详情(摘要/项目/上下文)——批完不回控制台也能看清批的是什么。
func TestDecisionResolvedCardKeepsOriginalDetail(t *testing.T) {
	cardJSON := DecisionResolvedCard(map[string]any{
		"decision_type":      "plan_review",
		"title":              "登录接口加固计划",
		"summary":            "共 3 个任务",
		"risk_level":         "high",
		"project_name":       "支付网关",
		"resolved_status":    "approved",
		"resolved_by_name":   "张三",
		"resolution_comment": "按计划推进",
		"context": map[string]any{
			"tasks": []any{map[string]any{"title": "改造登录限流", "selected_employee_id": "emp-1"}},
		},
		"employee_names": map[string]any{"emp-1": "后端-小李"},
	}, "http://web.local:3000")
	mustValid(t, cardJSON)
	for _, want := range []string{"共 3 个任务", "支付网关", "改造登录限流", "后端-小李", "张三", "按计划推进", "批准"} {
		if !strings.Contains(cardJSON, want) {
			t.Fatalf("resolved card lost original detail %q:\n%s", want, cardJSON)
		}
	}
	if strings.Contains(cardJSON, "resolve_decision") {
		t.Fatalf("resolved card must not keep action buttons:\n%s", cardJSON)
	}
}

// 决策卡富上下文分级渲染:plan_review 带任务清单+判据;demand_acceptance 带待签判据原文。
func TestDecisionCardRendersRichContext(t *testing.T) {
	planCard := DecisionCard(map[string]any{
		"decision_type": "plan_review",
		"title":         "登录接口加固计划",
		"project_name":  "支付网关",
		"risk_level":    "medium",
		"context": map[string]any{
			"tasks": []any{
				map[string]any{"title": "改造登录限流", "selected_employee_id": "emp-1"},
				map[string]any{"title": "补充压测报告"},
			},
			"plan_acceptance_criteria": []any{
				map[string]any{"statement": "限流阈值可配置且有回归测试"},
			},
			"human_review": map[string]any{"required": true, "reasons": []any{"涉及生产限流参数"}},
		},
		"employee_names": map[string]any{"emp-1": "后端-小李"},
	}, "dec-9", "proj-1", "http://web.local:3000")
	mustValid(t, planCard)
	for _, want := range []string{"支付网关", "计划任务", "改造登录限流", "后端-小李", "限流阈值可配置", "涉及生产限流参数"} {
		if !strings.Contains(planCard, want) {
			t.Fatalf("plan_review rich card missing %q:\n%s", want, planCard)
		}
	}

	acceptanceCard := DecisionCard(map[string]any{
		"decision_type": "demand_acceptance",
		"title":         "需求验收:登录加固",
		"context": map[string]any{
			"pending_criteria_detail": []any{
				map[string]any{"id": "c1", "statement": "全部接口通过安全扫描", "verification_method": "human_judgment"},
			},
		},
	}, "dec-10", "proj-1", "http://web.local:3000")
	mustValid(t, acceptanceCard)
	for _, want := range []string{"待签署判据", "全部接口通过安全扫描", "人工判断"} {
		if !strings.Contains(acceptanceCard, want) {
			t.Fatalf("demand_acceptance rich card missing %q:\n%s", want, acceptanceCard)
		}
	}
}

// 结果通知带需求摘录与任务清单;失败时点名失败任务。
func TestResultNoticeCardRichContent(t *testing.T) {
	cardJSON := ResultNoticeCard(map[string]any{
		"title":              "登录加固",
		"status":             "failed",
		"demand_id":          "d-9",
		"project_name":       "支付网关",
		"content_excerpt":    "把登录接口的限流从固定阈值改为按租户动态配置",
		"task_total":         float64(3),
		"task_completed":     float64(2),
		"task_failed":        float64(1),
		"failed_task_titles": []any{"压测报告生成"},
	}, "http://web.local:3000")
	mustValid(t, cardJSON)
	for _, want := range []string{"支付网关", "按租户动态配置", "失败任务", "压测报告生成", "共 3 项"} {
		if !strings.Contains(cardJSON, want) {
			t.Fatalf("result notice missing %q:\n%s", want, cardJSON)
		}
	}
}
