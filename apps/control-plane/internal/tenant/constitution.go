package tenant

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// 团队宪法（spec §5.3，D1 接通 / D9 仅文本注入）。
//
// 规则条目取代此前的裸字符串数组：分类让人一眼看出这条是禁止还是要求，也让注入到
// provider 的文本能按类分组。分类在服务端校验，不做前端封闭枚举（宪法「不依赖封闭
// 枚举」）。
const (
	ConstitutionCategoryForbid         = "forbid"
	ConstitutionCategoryMust           = "must"
	ConstitutionCategoryRequireApprove = "require_approval"
)

// constitutionCategories 是服务端注册的合法分类集合。新增分类在这里加。
var constitutionCategories = map[string]string{
	ConstitutionCategoryForbid:         "禁止",
	ConstitutionCategoryMust:           "必须",
	ConstitutionCategoryRequireApprove: "需审批",
}

// ConstitutionCategoryLabel 返回分类的中文名，未知值回退原文。
func ConstitutionCategoryLabel(category string) string {
	if label, ok := constitutionCategories[category]; ok {
		return label
	}
	return category
}

// ConstitutionRule 一条团队规则。
type ConstitutionRule struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Category string `json:"category"`
}

// TeamConstitutionRevision 一个宪法版本。
type TeamConstitutionRevision struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	TeamID         uuid.UUID
	RevisionNumber int32
	Rules          []ConstitutionRule
	ChangeNote     string
	CreatedBy      *uuid.UUID
	CreatedByName  string
	CreatedAt      string
}

// normalizeConstitutionRules 校验并规整规则条目：去空白、补 id、校验分类、拒绝空文本
// 与重复文本。返回规整后的条目与正文总字符数（用于字符预算）。
func normalizeConstitutionRules(rules []ConstitutionRule) ([]ConstitutionRule, int, error) {
	normalized := make([]ConstitutionRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	total := 0
	for _, rule := range rules {
		text := strings.TrimSpace(rule.Text)
		if text == "" {
			continue
		}
		if _, duplicate := seen[text]; duplicate {
			return nil, 0, fmt.Errorf("%w: duplicate constitution rule %q", ErrInvalidInput, text)
		}
		seen[text] = struct{}{}
		category := strings.TrimSpace(rule.Category)
		if category == "" {
			category = ConstitutionCategoryMust
		}
		if _, ok := constitutionCategories[category]; !ok {
			return nil, 0, fmt.Errorf("%w: unsupported constitution category %q", ErrInvalidInput, category)
		}
		id := strings.TrimSpace(rule.ID)
		if id == "" {
			id = uuid.NewString()
		}
		normalized = append(normalized, ConstitutionRule{ID: id, Text: text, Category: category})
		total += len([]rune(text))
	}
	return normalized, total, nil
}

// constitutionSnapshot 把规则条目转成 tenant_teams.constitution 的当前生效快照。
// 同时写 rules（结构化，新读者）与 hard_rules（纯文本，既有读者如员工创建基线），
// 两者由同一份 rules 派生，不会漂移。
func constitutionSnapshot(existing map[string]any, rules []ConstitutionRule) map[string]any {
	snapshot := cloneMap(existing)
	if snapshot == nil {
		snapshot = map[string]any{}
	}
	ruleItems := make([]any, 0, len(rules))
	hardRules := make([]any, 0, len(rules))
	for _, rule := range rules {
		ruleItems = append(ruleItems, map[string]any{
			"id":       rule.ID,
			"text":     rule.Text,
			"category": rule.Category,
		})
		hardRules = append(hardRules, rule.Text)
	}
	snapshot["rules"] = ruleItems
	snapshot["hard_rules"] = hardRules
	return snapshot
}

// ConstitutionRulesFromSnapshot 从当前生效快照里读回结构化规则。优先读 rules；
// 只有 hard_rules 的旧快照按「必须」归类回退，保证读路径对老数据也成立。
func ConstitutionRulesFromSnapshot(snapshot map[string]any) []ConstitutionRule {
	if snapshot == nil {
		return nil
	}
	if items, ok := snapshot["rules"].([]any); ok {
		rules := make([]ConstitutionRule, 0, len(items))
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			text, _ := entry["text"].(string)
			if strings.TrimSpace(text) == "" {
				continue
			}
			category, _ := entry["category"].(string)
			if category == "" {
				category = ConstitutionCategoryMust
			}
			id, _ := entry["id"].(string)
			rules = append(rules, ConstitutionRule{ID: id, Text: text, Category: category})
		}
		return rules
	}
	legacy, ok := snapshot["hard_rules"].([]any)
	if !ok {
		return nil
	}
	rules := make([]ConstitutionRule, 0, len(legacy))
	for _, item := range legacy {
		text, _ := item.(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		rules = append(rules, ConstitutionRule{Text: text, Category: ConstitutionCategoryMust})
	}
	return rules
}

// RenderConstitutionPrompt 把规则渲染成注入 provider 提示词的文本块。
// D9：只是约束文本，不参与任何门禁判定；分类只用于分组表达，不触发审批。
func RenderConstitutionPrompt(rules []ConstitutionRule) string {
	if len(rules) == 0 {
		return ""
	}
	grouped := map[string][]string{}
	order := []string{ConstitutionCategoryForbid, ConstitutionCategoryMust, ConstitutionCategoryRequireApprove}
	for _, rule := range rules {
		grouped[rule.Category] = append(grouped[rule.Category], rule.Text)
	}
	var builder strings.Builder
	builder.WriteString("# 团队宪法（必须遵守）\n")
	for _, category := range order {
		items := grouped[category]
		if len(items) == 0 {
			continue
		}
		builder.WriteString(fmt.Sprintf("\n## %s\n", ConstitutionCategoryLabel(category)))
		for _, text := range items {
			builder.WriteString(fmt.Sprintf("- %s\n", text))
		}
	}
	// 未知分类（历史数据/后续扩展）兜底附在最后，不静默丢弃。
	for category, items := range grouped {
		if category == ConstitutionCategoryForbid || category == ConstitutionCategoryMust || category == ConstitutionCategoryRequireApprove {
			continue
		}
		builder.WriteString(fmt.Sprintf("\n## %s\n", ConstitutionCategoryLabel(category)))
		for _, text := range items {
			builder.WriteString(fmt.Sprintf("- %s\n", text))
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}
