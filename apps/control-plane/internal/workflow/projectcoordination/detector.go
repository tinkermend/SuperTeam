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
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
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

// LLMPromptDetector 是基于 LLM prompt 的 ConditionDetector 实现，复用 route planner
// 的 chatCompletionClient 通道（openai_compatible_planner.go:62/54）。
//
// 诚实边界（spec §1/§10，与本文件顶部的抽象注释一致）：这个检测器只回答"这份真实工件
// 是否存在某一条已定义的违反类别"，它不证明工件整体正确，也不对工件打总体质量结论。
// systemPrompt 必须被框成"检测违反"而不是"评估质量/给结论"——LLMPromptDetector 本身不
// 强制这一点，写新的 system prompt 时必须遵守这个框架。
//
// 放行方向（与 RuleDetector 及规则型检测器一致，但理由不同）：这是审阅质量类检测器，不
// 是安全类检测器。任何模糊场景——LLM 调用失败、回复不是期望的 JSON 形状、或者回复里没有
// 显式给出 detected 字段——一律返回 Detected=false（默认放行），绝不把"我们看不懂模型的
// 回复"伪造成一次真实命中，也绝不让整个 Detect 调用向上抛错。这与安全类检测器"不确定就拦
// 截"的失败方向相反；那个失败关闭策略是按条件的 action-tier 关注点，属于 Task 3，不在这
// 里处理。
type LLMPromptDetector struct {
	key          string
	severity     string
	systemPrompt string
	model        string
	client       chatCompletionClient
}

// Key 实现 ConditionDetector。
func (d *LLMPromptDetector) Key() string {
	return d.key
}

// llmDetectorMaxTokens 限制检测回复的长度：检测器只需要一个短 JSON 判定，不需要长篇推理。
const llmDetectorMaxTokens = 512

// llmDetectorTemperature 使用确定性温度：检测器判定"存在/不存在"，不是创造性任务。
const llmDetectorTemperature = 0

// maxDetectorArtifactBytes 是喂给 LLM 的 diff/deliverables 文本上限，镜像
// openai_compatible_planner.go 里 maxChatCompletionResponseBytes 的"设上限、不让单次调用
// 无界增长"思路，但这里限的是我们自己发送的 user 消息大小，不是收到的响应体大小。
const maxDetectorArtifactBytes = 40000

// Detect 实现 ConditionDetector：把真实工件（art.DiffText / art.Deliverables /
// art.Summary——真实代码/产出，不是任何"已完成"的叙述性声明）交给 LLM 做违反检测，解析回
// 复为 {"detected": bool, "finding": string}。
//
// 任何解析或调用异常都收敛为 Detected=false（见类型注释的放行方向说明），不向上传播为
// error——一次检测器判定失败不应该让整个审阅门崩溃。
func (d *LLMPromptDetector) Detect(ctx context.Context, art DetectionArtifact) DetectionResult {
	if d == nil || d.client == nil {
		return DetectionResult{}
	}

	content, err := d.client.CreateChatCompletion(ctx, OpenAICompatibleChatRequest{
		Model:       d.model,
		System:      d.systemPrompt,
		User:        buildLLMDetectorUserPrompt(art),
		MaxTokens:   llmDetectorMaxTokens,
		Temperature: llmDetectorTemperature,
	})
	if err != nil {
		// Client/transport failure: unreadable, not a detection. Fail open toward
		// release (see type comment).
		return DetectionResult{}
	}

	reply, ok := decodeLLMDetectionReply(content)
	if !ok || !reply.Detected {
		// Either the reply did not parse as the expected JSON shape, or it parsed
		// but detected was absent/false. Both collapse to no-detection: we never
		// fabricate a hit from a reply we cannot confidently read as one.
		return DetectionResult{}
	}

	return DetectionResult{
		Detected:     true,
		Severity:     d.severity,
		ConditionKey: d.key,
		Finding:      reply.Finding,
	}
}

// llmDetectionReply 是 LLM 检测器回复的期望 JSON 形状。
type llmDetectionReply struct {
	Detected bool   `json:"detected"`
	Finding  string `json:"finding"`
}

// decodeLLMDetectionReply 解析 LLM 回复为 llmDetectionReply。容忍模型把 JSON 包在
// ```json ... ``` 代码块里的常见偏差；除此之外不做宽松兜底——解析失败就是解析失败，交给
// 调用方按放行方向处理，不在这里猜测。
func decodeLLMDetectionReply(content string) (llmDetectionReply, bool) {
	trimmed := strings.TrimSpace(content)
	trimmed = strings.TrimPrefix(trimmed, "```json")
	trimmed = strings.TrimPrefix(trimmed, "```")
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	var reply llmDetectionReply
	if err := json.Unmarshal([]byte(trimmed), &reply); err != nil {
		return llmDetectionReply{}, false
	}
	return reply, true
}

// buildLLMDetectorUserPrompt 把真实工件（不是任务描述、不是"已完成"的叙述性声明）组装成
// user 消息：真实 diff 文本 + 交付物清单 + 摘要。diff 文本超出 maxDetectorArtifactBytes 时
// 截断，避免单次检测调用的请求体无界增长。
func buildLLMDetectorUserPrompt(art DetectionArtifact) string {
	var b strings.Builder
	b.WriteString("以下是需要检测的真实工件（实际代码/产出，不是任务描述或计划，也不是任何“已完成”的叙述性声明）：\n\n")

	if art.DiffText != "" {
		b.WriteString("真实 diff：\n")
		b.WriteString(truncateForDetectorPrompt(art.DiffText))
		b.WriteString("\n\n")
	}

	if len(art.Deliverables) > 0 {
		b.WriteString("交付物清单：\n")
		for _, deliverable := range art.Deliverables {
			b.WriteString("- ")
			b.WriteString(truncateForDetectorPrompt(deliverable))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if art.Summary != "" {
		b.WriteString("摘要（仅供参考，判定必须基于上面的真实 diff/交付物，不能仅凭这段摘要下结论）：\n")
		b.WriteString(truncateForDetectorPrompt(art.Summary))
		b.WriteString("\n\n")
	}

	b.WriteString("只输出 JSON：{\"detected\": bool, \"finding\": string}")
	return b.String()
}

// truncateForDetectorPrompt 把单段文本截断到 maxDetectorArtifactBytes 以内。
func truncateForDetectorPrompt(text string) string {
	if len(text) <= maxDetectorArtifactBytes {
		return text
	}
	return text[:maxDetectorArtifactBytes] + "\n…(truncated)"
}

// codeReviewSystemPrompt 是首个 LLM 条件（code_review）的 system prompt：框成"检测违反"
// 而不是"评估质量/给结论"——不要求整体好坏判断、不要求打分、不要求判断改动是否"做对了"，
// 只回答"是否存在这些已定义的违反类别"。
var codeReviewSystemPrompt = strings.Join([]string{
	"你是代码审查违反检测器（violation detector），不是整体质量评估器。",
	"任务：只判断这份改动是否**存在**以下违反类别之一：明显的代码缺陷、逻辑错误、安全漏洞（如注入、越权、密钥硬编码）、会导致运行时崩溃或数据损坏的问题。",
	"只输出 JSON：{\"detected\": bool, \"finding\": string}。不要输出任何其他文字，不要用 markdown 代码块包裹。",
	"如果不确定、证据不足、或未发现上述任何违反类别，输出 detected=false，finding 留空字符串。",
	"不要评价这份改动整体好坏，不要给出总体结论，不要判断它是否“做对了”或已经达成需求——只检测是否**存在**上面列出的违反类别。",
}, "\n")

// newCodeReviewDetector 构造代码审查检测器（条件键 code_review，severity=major）：扫描真
// 实 diff 有无明显缺陷/漏洞类问题。
func newCodeReviewDetector(client chatCompletionClient, model string) *LLMPromptDetector {
	return &LLMPromptDetector{
		key:          "code_review",
		severity:     "major",
		systemPrompt: codeReviewSystemPrompt,
		model:        model,
		client:       client,
	}
}
