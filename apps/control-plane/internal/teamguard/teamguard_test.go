package teamguard

import (
	"errors"
	"strings"
	"testing"
)

func TestBlockedErrorReturnsNilWithoutBlockers(t *testing.T) {
	if err := BlockedError(nil, "移出团队"); err != nil {
		t.Fatalf("expected nil for empty blockers, got %v", err)
	}
}

func TestBlockedErrorIgnoresUnknownBlockerTypes(t *testing.T) {
	// 未知类型不该凭空造出一句"有 0 个…"的错误，否则任何脏数据都会把操作卡死。
	err := BlockedError([]DetachBlocker{{Type: "something_else", Name: "x"}}, "换队")
	if err != nil {
		t.Fatalf("expected nil for unknown blocker type, got %v", err)
	}
}

func TestBlockedErrorNamesBothCategoriesAndMatchesPrototype(t *testing.T) {
	err := BlockedError([]DetachBlocker{
		{Type: BlockerActiveRun, RefID: "run-1", Name: "巡检任务", Status: "running"},
		{Type: BlockerActiveProject, RefID: "proj-1", Name: "结算重构", Status: "running"},
	}, "移出团队")
	if err == nil {
		t.Fatal("expected blocked error")
	}
	if !errors.Is(err, ErrDetachBlocked) {
		t.Fatalf("expected error to match prototype by code, got %v", err)
	}
	message := err.Error()
	for _, want := range []string{
		"该数字员工有 1 个在役执行（巡检任务）",
		"且被 1 个进行中项目引用（结算重构）",
		"无法移出团队",
		"请先等待或取消在役执行、从相关项目中移除该成员后重试",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q missing %q", message, want)
		}
	}
}

// 只命中项目一类时，句子必须读得通（"该数字员工被 N 个…"，不是"该数字员工有 被 N 个…"），
// 且建议句不提没发生的在役执行。
func TestBlockedErrorReadsNaturallyWithASingleCategory(t *testing.T) {
	message := BlockedError([]DetachBlocker{
		{Type: BlockerActiveProject, RefID: "p1", Name: "结算重构"},
	}, "移出团队").Error()
	if !strings.Contains(message, "该数字员工被 1 个进行中项目引用（结算重构），无法移出团队。") {
		t.Fatalf("unexpected phrasing: %q", message)
	}
	if strings.Contains(message, "有 被") {
		t.Fatalf("broken phrasing survived: %q", message)
	}
	if strings.Contains(message, "在役执行") {
		t.Fatalf("advice mentions a blocker that did not occur: %q", message)
	}
}

func TestBlockedErrorTruncatesLongLists(t *testing.T) {
	blockers := []DetachBlocker{}
	for _, name := range []string{"A", "B", "C", "D", "E"} {
		blockers = append(blockers, DetachBlocker{Type: BlockerActiveProject, RefID: name, Name: name})
	}
	message := BlockedError(blockers, "换队").Error()
	if !strings.Contains(message, "被 5 个进行中项目引用") {
		t.Fatalf("expected total count in message, got %q", message)
	}
	if !strings.Contains(message, "A、B、C 等 5 个") {
		t.Fatalf("expected truncated sample, got %q", message)
	}
	if strings.Contains(message, "D") || strings.Contains(message, "E") {
		t.Fatalf("expected sample to stop at the limit, got %q", message)
	}
}

func TestBlockedErrorFallsBackToRefIDWhenNameMissing(t *testing.T) {
	message := BlockedError([]DetachBlocker{
		{Type: BlockerActiveRun, RefID: "run-9", Name: "   "},
	}, "移出团队").Error()
	if !strings.Contains(message, "run-9") {
		t.Fatalf("expected ref id fallback, got %q", message)
	}
}
