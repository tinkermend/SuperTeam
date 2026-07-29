package cards

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestHumanTaskKindLabelsMatchSharedFixture asserts Feishu kind → 中文 values
// match contracts/control-plane/human-task-kind-labels.json key-by-key
// (2026-07-25 §5.4 option 2).
func TestHumanTaskKindLabelsMatchSharedFixture(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixturePath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..", "contracts", "control-plane", "human-task-kind-labels.json"))
	raw, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}
	var fixture map[string]string
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(fixture) == 0 {
		t.Fatal("fixture empty")
	}
	for kind, want := range fixture {
		got, ok := humanTaskKindLabels[kind]
		if !ok {
			t.Errorf("feishu missing kind %q (want %q)", kind, want)
			continue
		}
		if got != want {
			t.Errorf("kind %q: feishu=%q fixture=%q", kind, got, want)
		}
	}
	for kind := range humanTaskKindLabels {
		if _, ok := fixture[kind]; !ok {
			t.Errorf("feishu has extra kind %q not in fixture", kind)
		}
	}
}

func TestDispatchAndDownstreamReleaseCardsHaveApproveReject(t *testing.T) {
	for _, kind := range []string{"dispatch_release", "downstream_release"} {
		cardJSON := DecisionCard(map[string]any{
			"kind":          kind,
			"decision_type": "project_task_approval",
			"title":         "高风险派发",
			"summary":       "写入生产配置",
			"risk_level":    "high",
		}, "dec-x", "proj-1", "http://web.local:3000")
		mustValid(t, cardJSON)
		for _, want := range []string{"approved", "rejected", "resolve_decision", "放行", "驳回"} {
			if !strings.Contains(cardJSON, want) {
				t.Fatalf("%s card missing %q:\n%s", kind, want, cardJSON)
			}
		}
	}
}

func TestClosureConfirmDemandListCarriesStatus(t *testing.T) {
	closure := DecisionCard(map[string]any{
		"kind":          "closure_confirm",
		"decision_type": "project_acceptance",
		"title":         "结项确认 · 支付网关",
		"project_name":  "支付网关",
		"context": map[string]any{
			"demands": []any{
				map[string]any{"title": "接入渠道对账", "status": "completed", "status_label": "已完成"},
				map[string]any{"title": "遗留 E2E 夹具需求", "status": "cancelled", "status_label": "已取消"},
				// 老卡片快照没有 status_label,必须由 status 兜底。
				map[string]any{"title": "旧快照需求", "status": "failed"},
			},
		},
	}, "dec-c", "proj-1", "http://web.local:3000")
	mustValid(t, closure)
	for _, want := range []string{"接入渠道对账 · 已完成", "遗留 E2E 夹具需求 · 已取消", "旧快照需求 · 失败"} {
		if !strings.Contains(closure, want) {
			t.Fatalf("closure card missing %q:\n%s", want, closure)
		}
	}
}

func TestClosureConfirmAndPlanningFailedCards(t *testing.T) {
	closure := DecisionCard(map[string]any{
		"kind":          "closure_confirm",
		"decision_type": "project_acceptance",
		"title":         "结项确认 · 支付网关",
		"project_name":  "支付网关",
	}, "dec-c", "proj-1", "http://web.local:3000")
	mustValid(t, closure)
	for _, want := range []string{"确认结项并归档", "退回返工", "要求补证", "结项确认"} {
		if !strings.Contains(closure, want) {
			t.Fatalf("closure card missing %q:\n%s", want, closure)
		}
	}

	failed := DecisionCard(map[string]any{
		"kind":          "planning_failed",
		"decision_type": "planning_failed",
		"title":         "规划失败",
		"summary":       "planner 超时",
	}, "dec-f", "proj-1", "http://web.local:3000")
	mustValid(t, failed)
	for _, want := range []string{"retry_planning", "close_demand", "重新规划", "关闭需求", "规划失败"} {
		if !strings.Contains(failed, want) {
			t.Fatalf("planning_failed card missing %q:\n%s", want, failed)
		}
	}
}
