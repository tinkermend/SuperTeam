package projectcoordination

// Detector 抽象的设计边界（见 docs/superpowers/specs/2026-07-17-review-gate-violation-detection-design.md §1/§10）：
//
// 一个 ConditionDetector 只回答一个问题——"这个工件是否违反了一条已定义的条件"。
// 它检测的是某个可枚举缺陷/违规类别在一个真实工件中的"存在性"，而不是：
//   - 证明工件整体正确（correctness proof）；
//   - 给工件整体质量打分（quality scoring）。
//
// "未检出"（Detected == false）是默认放行方向：检测器保持沉默不代表工件被判定为"正确"或
// "优秀"，只代表它没有触发这条已定义的违规条件。审阅门是多个独立检测器的并集，不是一个
// 综合评分器。新增检测器时不要把它写成"给分/给结论"的形态，只回答"命中/未命中"。
//
// 本文件实现该抽象 + 第一个规则型检测器：密钥泄漏（secret_leak，block 级）。

import (
	"context"
	"fmt"
	"regexp"
)

// DetectionArtifact 是被审任务的真实工件切片：不是任务描述或计划，而是实际产出
// （摘要、交付物清单、证据引用、真实 diff 文本）。
type DetectionArtifact struct {
	Summary      string
	Deliverables []string
	EvidenceRefs []string
	DiffText     string
}

// DetectionResult 是单个检测器对单个工件的判定结果。Severity 取值："block" | "major" | "minor"。
// 零值（Detected == false）即"未检出"，是默认放行方向。
type DetectionResult struct {
	Detected     bool
	Severity     string
	ConditionKey string
	Finding      string
	EvidenceRefs []string
}

// ConditionDetector 是所有检测器（规则型、LLM 型等）共同实现的接口。
type ConditionDetector interface {
	// Key 返回该检测器对应的条件标识（与 DetectionResult.ConditionKey 一致）。
	Key() string
	// Detect 在给定工件上判定是否命中该检测器定义的违规条件。
	Detect(ctx context.Context, art DetectionArtifact) DetectionResult
}

// ruleMatch 是单条规则的最小描述：命中该正则即视为触发条件。
// label 用于在 Finding 中说明命中的是哪种模式（不回显命中的原文）。
type ruleMatch struct {
	label string
	re    *regexp.Regexp
}

// RuleDetector 是基于正则规则集合的 ConditionDetector 实现：扫描 DiffText 与每个
// Deliverables 条目，命中任一规则即判定 Detected。
type RuleDetector struct {
	key      string
	severity string
	rules    []ruleMatch
}

// Key 实现 ConditionDetector。
func (d *RuleDetector) Key() string {
	return d.key
}

// Detect 实现 ConditionDetector：扫描 DiffText 与 Deliverables，命中任一规则即返回
// Detected=true 的结果；未命中任何规则则返回零值（Detected=false）。
func (d *RuleDetector) Detect(_ context.Context, art DetectionArtifact) DetectionResult {
	texts := make([]string, 0, len(art.Deliverables)+1)
	if art.DiffText != "" {
		texts = append(texts, art.DiffText)
	}
	texts = append(texts, art.Deliverables...)

	for _, text := range texts {
		for _, rule := range d.rules {
			if loc := rule.re.FindStringIndex(text); loc != nil {
				match := text[loc[0]:loc[1]]
				return DetectionResult{
					Detected:     true,
					Severity:     d.severity,
					ConditionKey: d.key,
					Finding:      fmt.Sprintf("检测到疑似密钥泄漏（模式：%s），命中片段已脱敏：%s", rule.label, redactSecret(match)),
				}
			}
		}
	}

	return DetectionResult{}
}

// redactSecret 对命中的原文做脱敏：只保留一个短前缀（用于人工核对是哪类值），其余全部
// 替换为掩码字符，绝不回显完整的密钥/口令原文。
func redactSecret(matched string) string {
	const keepPrefix = 4
	prefix := matched
	if len(matched) > keepPrefix {
		prefix = matched[:keepPrefix]
	}
	return prefix + "****"
}

// secretLeakRules 是密钥泄漏检测器的正则规则集合，构造时一次性预编译（不在 Detect 内重复编译）。
var secretLeakRules = []ruleMatch{
	{label: "OpenAI-style API key (sk-...)", re: regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`)},
	{label: "AWS access key ID (AKIA...)", re: regexp.MustCompile(`AKIA[0-9A-Z]{12,}`)},
	{label: "PEM private key block", re: regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)},
	{label: "inline password assignment", re: regexp.MustCompile(`password\s*=\s*["'][^"']+["']`)},
}

// newSecretLeakDetector 构造密钥泄漏检测器（条件键 secret_leak，severity=block）。
func newSecretLeakDetector() *RuleDetector {
	return &RuleDetector{
		key:      "secret_leak",
		severity: "block",
		rules:    secretLeakRules,
	}
}
