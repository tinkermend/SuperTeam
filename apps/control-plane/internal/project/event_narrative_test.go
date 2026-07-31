package project

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// projectEventTypeLiteralPattern 抓 types.go 里 ProjectEventType 常量的字符串
// 字面量。别名常量(= 另一个常量而非字面量)天然不被匹配,正是想要的。
var projectEventTypeLiteralPattern = regexp.MustCompile(`ProjectEventType\s*=\s*"([^"]+)"`)

func allProjectEventTypeLiterals(t *testing.T) []ProjectEventType {
	t.Helper()
	source, err := os.ReadFile("types.go")
	if err != nil {
		t.Fatalf("读取 types.go 失败: %v", err)
	}
	matches := projectEventTypeLiteralPattern.FindAllStringSubmatch(string(source), -1)
	if len(matches) < 50 {
		t.Fatalf("只从 types.go 扫到 %d 个事件类型字面量,远少于预期——正则或常量写法变了,本护栏已失效", len(matches))
	}
	types := make([]ProjectEventType, 0, len(matches))
	for _, match := range matches {
		types = append(types, ProjectEventType(match[1]))
	}
	return types
}

// 全枚举护栏:任何 ProjectEventType 都必须有登记的中文叙事。新增事件类型而忘了
// 补表会在这里失败,而不是等用户在页面上看到英文蛇形串。
func TestProjectEventNarrativeCoversEveryEventType(t *testing.T) {
	for _, eventType := range allProjectEventTypeLiterals(t) {
		narrative, ok := projectEventNarratives[eventType]
		if !ok {
			t.Errorf("事件类型 %q 未登记叙事;请在 projectEventNarratives 补一条中文文案", eventType)
			continue
		}
		if strings.TrimSpace(narrative.Title) == "" {
			t.Errorf("事件类型 %q 的叙事标题为空", eventType)
		}
	}
}

// 机械判别器:中文标题里不该出现 "." 或 "_"——这两个字符正是 event_type 蛇形串
// 的特征,出现即说明有人把原始事件类型当文案漏了出去。
func TestProjectEventNarrativeTitlesAreChineseNotRawEventType(t *testing.T) {
	assertClean := func(t *testing.T, label, title string) {
		t.Helper()
		if strings.ContainsAny(title, "._") {
			t.Errorf("%s 的标题 %q 含 '.' 或 '_',疑似把 event_type 原串当文案", label, title)
		}
		for _, r := range title {
			if r >= 'a' && r <= 'z' {
				t.Errorf("%s 的标题 %q 含小写英文字母,面向用户文案必须是中文", label, title)
				return
			}
		}
	}

	for eventType, narrative := range projectEventNarratives {
		assertClean(t, string(eventType), narrative.Title)
	}
	assertClean(t, "未知事件兜底", unknownProjectEventNarrative.Title)
}

// 未登记类型必须回落通用中文,且落在 other,不得 panic 也不得回吐原串。
func TestNarrateProjectEventTypeFallsBackToGenericChinese(t *testing.T) {
	narrative := NarrateProjectEventType(ProjectEventType("some.brand_new.event_type"))
	if narrative.Kind != TimelineKindOther {
		t.Fatalf("未知事件应落 other,得到 %q", narrative.Kind)
	}
	if narrative.Title != "协调更新" {
		t.Fatalf("未知事件文案应为通用中文,得到 %q", narrative.Title)
	}
}

// 叙事 kind 与 severity 必须落在契约枚举内,否则前端 tone 映射会静默失配。
func TestProjectEventNarrativeUsesContractEnums(t *testing.T) {
	kinds := map[string]bool{
		TimelineKindDemandSubmitted: true, TimelineKindCoordinationStarted: true,
		TimelineKindPlanReady: true, TimelineKindPlanAccepted: true,
		TimelineKindPlanRejected: true, TimelineKindPlanChangeRequested: true,
		TimelineKindTaskCreated: true, TimelineKindTaskDispatched: true,
		TimelineKindTaskWaitingHuman: true, TimelineKindTaskCompleted: true,
		TimelineKindTaskFailed: true, TimelineKindTaskCancelled: true,
		TimelineKindResultRecorded: true, TimelineKindResultAccepted: true,
		TimelineKindResultRejected: true, TimelineKindDecisionOpened: true,
		TimelineKindDecisionResolved: true, TimelineKindDispatchBlocked: true,
		TimelineKindStaffingGap: true, TimelineKindCoordinationBlocked: true,
		TimelineKindOther: true,
	}
	severities := map[string]bool{
		NarrativeSeverityInfo: true, NarrativeSeveritySuccess: true,
		NarrativeSeverityWarn: true, NarrativeSeverityDanger: true,
		NarrativeSeverityMute: true,
	}
	for eventType, narrative := range projectEventNarratives {
		if !kinds[narrative.Kind] {
			t.Errorf("事件 %q 的 kind %q 不在契约枚举内", eventType, narrative.Kind)
		}
		if !severities[narrative.Severity] {
			t.Errorf("事件 %q 的 severity %q 不在契约枚举内", eventType, narrative.Severity)
		}
	}
}

// 关键业务事实不得被当噪音过滤掉(spec §5.4:blocking/gap/failed/waiting_human/
// decision/任务终态必须进时间线)。
func TestCriticalEventsAreNotMarkedNoise(t *testing.T) {
	critical := []ProjectEventType{
		ProjectEventCoordinationBlocked,
		ProjectEventTaskDispatchBlocked,
		ProjectEventTaskFailed,
		ProjectEventTaskCompleted,
		ProjectEventTaskWaitingHuman,
		ProjectEventDecisionRequested,
		ProjectEventDecisionSubmitted,
		ProjectEventTeamlessEmployeeSkipped,
		ProjectEventLendingEmployeeSkipped,
		ProjectEventDemandAcceptanceRejected,
	}
	for _, eventType := range critical {
		if NarrateProjectEventType(eventType).Noise {
			t.Errorf("关键事件 %q 被标为噪音,会从时间线消失", eventType)
		}
	}
}
