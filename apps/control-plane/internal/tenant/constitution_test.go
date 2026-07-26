package tenant

import (
	"strings"
	"testing"
)

func TestNormalizeConstitutionRulesTrimsFillsAndCounts(t *testing.T) {
	rules, total, err := normalizeConstitutionRules([]ConstitutionRule{
		{Text: "  不得直连生产库  "},
		{Text: "变更必须登记", Category: ConstitutionCategoryMust},
		{Text: "   "},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected blank rule dropped, got %#v", rules)
	}
	if rules[0].Text != "不得直连生产库" {
		t.Fatalf("expected trimmed text, got %q", rules[0].Text)
	}
	// 未填分类默认「必须」，并自动补 id，避免前端漏传导致条目无法定位。
	if rules[0].Category != ConstitutionCategoryMust || rules[0].ID == "" {
		t.Fatalf("expected default category and generated id, got %#v", rules[0])
	}
	if total != len([]rune("不得直连生产库"))+len([]rune("变更必须登记")) {
		t.Fatalf("unexpected char total %d", total)
	}
}

func TestNormalizeConstitutionRulesRejectsUnknownCategoryAndDuplicates(t *testing.T) {
	if _, _, err := normalizeConstitutionRules([]ConstitutionRule{
		{Text: "x", Category: "whatever"},
	}); err == nil {
		t.Fatal("expected unknown category to be rejected")
	}
	if _, _, err := normalizeConstitutionRules([]ConstitutionRule{
		{Text: "同一条"}, {Text: " 同一条 "},
	}); err == nil {
		t.Fatal("expected duplicate rule text to be rejected")
	}
}

func TestConstitutionSnapshotKeepsHardRulesInSyncWithRules(t *testing.T) {
	// hard_rules 是既有读者（员工创建基线）的入口，必须与结构化 rules 同源派生，
	// 否则两个事实源会漂移。
	snapshot := constitutionSnapshot(map[string]any{"other": "keep"}, []ConstitutionRule{
		{ID: "r1", Text: "不得直连生产库", Category: ConstitutionCategoryForbid},
	})
	if snapshot["other"] != "keep" {
		t.Fatalf("expected unrelated keys preserved, got %#v", snapshot)
	}
	hardRules, ok := snapshot["hard_rules"].([]any)
	if !ok || len(hardRules) != 1 || hardRules[0] != "不得直连生产库" {
		t.Fatalf("unexpected hard_rules %#v", snapshot["hard_rules"])
	}
	if rules, ok := snapshot["rules"].([]any); !ok || len(rules) != 1 {
		t.Fatalf("unexpected rules %#v", snapshot["rules"])
	}
}

func TestConstitutionRulesFromSnapshotFallsBackToLegacyHardRules(t *testing.T) {
	rules := ConstitutionRulesFromSnapshot(map[string]any{
		"hard_rules": []any{"旧规则", "  ", "另一条"},
	})
	if len(rules) != 2 || rules[0].Category != ConstitutionCategoryMust {
		t.Fatalf("expected legacy fallback to must-category rules, got %#v", rules)
	}
}

func TestRenderConstitutionPromptGroupsByCategory(t *testing.T) {
	prompt := RenderConstitutionPrompt([]ConstitutionRule{
		{Text: "不得直连生产库", Category: ConstitutionCategoryForbid},
		{Text: "变更必须登记", Category: ConstitutionCategoryMust},
		{Text: "上线需审批", Category: ConstitutionCategoryRequireApprove},
	})
	// 提示词标题走 constitutionPromptHeaders，故意比人类看到的 ConstitutionCategoryLabel
	// 更强硬（forbid/must 保留"禁止/必须"）；唯独 require_approval 连对模型也不写"需审批"
	// ——那会让模型误以为存在真实的审批流程，改用"重点关注"。
	for _, want := range []string{"# 团队宪法（必须遵守）", "## 禁止", "## 必须", "## 重点关注", "- 不得直连生产库"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt %q missing %q", prompt, want)
		}
	}
	// 只检查标题不写"需审批"；规则正文是自由文本，用户完全可以自己写这几个字
	// （比如这条规则的原文就是"上线需审批"），那不是本测试要拦的东西。
	if strings.Contains(prompt, "## 需审批") {
		t.Fatalf("prompt header must not claim an approval workflow exists: %q", prompt)
	}
	if strings.Index(prompt, "## 禁止") > strings.Index(prompt, "## 必须") {
		t.Fatalf("expected forbid section first: %q", prompt)
	}
}

// ConstitutionCategoryLabel 是给人看的（控制台 UI），必须诚实：不能暗示存在门禁或
// 审批流程（D9）。
func TestConstitutionCategoryLabelDoesNotOverclaimEnforcement(t *testing.T) {
	for _, category := range []string{ConstitutionCategoryForbid, ConstitutionCategoryMust, ConstitutionCategoryRequireApprove} {
		label := ConstitutionCategoryLabel(category)
		for _, forbidden := range []string{"禁止", "必须", "需审批"} {
			if label == forbidden {
				t.Fatalf("category %q label %q overclaims enforcement strength", category, label)
			}
		}
	}
}

func TestRenderConstitutionPromptEmptyForNoRules(t *testing.T) {
	if got := RenderConstitutionPrompt(nil); got != "" {
		t.Fatalf("expected empty prompt, got %q", got)
	}
}

// 未知分类（历史数据或后续扩展）必须兜底输出，不能静默丢规则。
func TestRenderConstitutionPromptKeepsUnknownCategories(t *testing.T) {
	prompt := RenderConstitutionPrompt([]ConstitutionRule{{Text: "冷门规则", Category: "custom"}})
	if !strings.Contains(prompt, "冷门规则") {
		t.Fatalf("unknown category rule dropped: %q", prompt)
	}
}
