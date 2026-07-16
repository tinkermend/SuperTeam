package projectcoordination

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Adversarial AI review engine (autonomy posture Phase B).
//
// A criterion of kind adversarial_review is decided by N independent LLM
// "judges", each of which is instructed to REFUTE — not confirm — that the
// reviewed task's output satisfies the criterion. A majority of refutals marks
// the criterion unsatisfied. This is deliberately conservative: the default
// posture of each judge is refuted unless the evidence convinces it otherwise,
// and a parse failure is also counted as refuted (宁严勿漏).
//
// HONESTY / KNOWN LIMITATION (spec §7, tier-2): the N judges are N calls to the
// SAME underlying model with different lens (perspective) system prompts. This
// buys *perspective diversity*, not *model independence* — same-model judges
// share failure modes and blind spots in a way genuinely independent human
// reviewers do not. Do not overclaim this as independent verification.

const (
	// AdversarialVerdictRefuted is emitted when a judge is not convinced the
	// output satisfies the criterion (the conservative default).
	AdversarialVerdictRefuted = "refuted"
	// AdversarialVerdictAccepted is emitted only when a judge is convinced.
	AdversarialVerdictAccepted = "accepted"

	// AdversarialAggregateSatisfied means a minority (or none) of judges refuted.
	AdversarialAggregateSatisfied = "satisfied"
	// AdversarialAggregateUnsatisfied means a majority of judges refuted.
	AdversarialAggregateUnsatisfied = "unsatisfied"
	// AdversarialAggregateEscalateHuman is returned when the revision/cost budget
	// is exhausted; Task 4 turns this into a human tier-3 hold rather than
	// spending more model calls on an already-over-budget task.
	AdversarialAggregateEscalateHuman = "escalate_human"

	// adversarialReasonParseFailed is the reason attached to a judgement whose
	// model response could not be parsed into a structured verdict.
	adversarialReasonParseFailed = "parse_failed"

	// defaultAdversarialJudgeCount is used when coordination policy does not set
	// adversarial_review_judges.
	defaultAdversarialJudgeCount = 3
	// maxAdversarialJudgeCount is the hard cap on judges, regardless of policy, to
	// bound per-criterion model spend.
	maxAdversarialJudgeCount = 7
)

// resolveJudgeCount maps the raw coordination-policy value to an effective judge
// count: unset or non-positive → default 3; values above the hard cap → 7.
func resolveJudgeCount(policyValue int) int {
	if policyValue <= 0 {
		return defaultAdversarialJudgeCount
	}
	if policyValue > maxAdversarialJudgeCount {
		return maxAdversarialJudgeCount
	}
	return policyValue
}

// baseAdversarialLenses are the spec's three default adversarial perspectives.
// Order is stable: correctness, security, reproducibility.
func baseAdversarialLenses() []AdversarialLens {
	return []AdversarialLens{
		{
			Key: "correctness",
			SystemPrompt: buildJudgeSystemPrompt(
				"正确性",
				"审查产出是否真正满足判据断言：逻辑是否成立、边界与异常情况是否覆盖、是否存在与断言相矛盾的事实。",
			),
		},
		{
			Key: "security",
			SystemPrompt: buildJudgeSystemPrompt(
				"安全",
				"审查产出是否引入安全、权限或数据泄漏问题：越权、凭据外泄、注入、绕过审批、破坏隔离等；任何此类风险都足以判定未满足。",
			),
		},
		{
			Key: "reproducibility",
			SystemPrompt: buildJudgeSystemPrompt(
				"可复现性",
				"审查结论是否有可复现的证据支撑：是否有测试、命令输出、工件或链接可核对，而非空口断言；证据缺失或无法核对即判未满足。",
			),
		},
	}
}

// resolveAdversarialLenses returns exactly count lenses. When count is at or
// below the number of base lenses, it returns the first count base lenses. When
// count exceeds the base set, it extends DETERMINISTICALLY by cycling the base
// perspectives, giving each extra judge a distinct key suffix (e.g.
// correctness#2) while reusing the same base perspective prompt — this is an
// honest "more of the same perspectives", not a claim of new independent
// viewpoints. The system prompt is identical to the cycled base lens (same
// perspective), so perspective diversity is bounded by the base set.
func resolveAdversarialLenses(count int) []AdversarialLens {
	base := baseAdversarialLenses()
	if count <= 0 {
		count = defaultAdversarialJudgeCount
	}
	if count > maxAdversarialJudgeCount {
		count = maxAdversarialJudgeCount
	}
	if count <= len(base) {
		return base[:count]
	}
	lenses := make([]AdversarialLens, 0, count)
	lenses = append(lenses, base...)
	for i := len(base); i < count; i++ {
		src := base[i%len(base)]
		occurrence := i/len(base) + 1
		lenses = append(lenses, AdversarialLens{
			Key:          src.Key + "#" + strconv.Itoa(occurrence),
			SystemPrompt: src.SystemPrompt,
		})
	}
	return lenses
}

func buildJudgeSystemPrompt(perspective, focus string) string {
	return strings.Join([]string{
		fmt.Sprintf("你是 SuperTeam 对抗式验收判官，视角：%s。", perspective),
		"你的任务是**证伪**——主动找出这份产出不满足判据的理由，而不是礼貌地确认它满足。",
		"默认判定为 refuted，除非工件与证据确实说服你该判据已被满足，才判 accepted。",
		focus,
		`只返回一个 JSON 对象，不要包裹在 markdown 中，形如 {"verdict":"refuted|accepted","reason":"一句话理由"}。`,
		"verdict 只能是 refuted 或 accepted；reason 用一句话给出判定依据。",
	}, "\n")
}

func buildJudgeUserPrompt(input RunAdversarialReviewInput) string {
	payload := judgePromptPayload{
		Assertion:       input.Assertion,
		EvidenceSummary: input.EvidenceSummary,
		Deliverables:    input.Deliverables,
		EvidenceRefs:    input.EvidenceRefs,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte("{}")
	}
	return fmt.Sprintf("判据与被评审任务的产出证据如下，判断该判据是否被满足。只返回 JSON。\n%s", string(body))
}

type judgePromptPayload struct {
	Assertion       string   `json:"assertion"`
	EvidenceSummary string   `json:"evidence_summary"`
	Deliverables    []string `json:"deliverables,omitempty"`
	EvidenceRefs    []string `json:"evidence_refs,omitempty"`
}

// runAdversarialReview is the pure engine: it runs one refute-style LLM call per
// lens, parses each {verdict, reason} response, and aggregates. It is DB-free and
// fully unit-testable with a fake chatCompletionClient. A parse failure yields a
// conservative refuted judgement with reason=parse_failed. A transport error from
// the client fails the whole review (propagated to the caller) — an unavailable
// judge model is a genuine failure, not a silent refute.
func runAdversarialReview(ctx context.Context, client chatCompletionClient, lenses []AdversarialLens, input RunAdversarialReviewInput) (AdversarialReviewResult, error) {
	result := AdversarialReviewResult{
		CriterionID:    input.CriterionID,
		ReviewedTaskID: input.ReviewedTaskID,
		JudgeCount:     len(lenses),
		Judgements:     make([]AdversarialJudgement, 0, len(lenses)),
	}
	user := buildJudgeUserPrompt(input)
	for _, lens := range lenses {
		if err := ctx.Err(); err != nil {
			return AdversarialReviewResult{}, err
		}
		content, err := client.CreateChatCompletion(ctx, OpenAICompatibleChatRequest{
			Model:  input.Model,
			System: lens.SystemPrompt,
			User:   user,
		})
		if err != nil {
			return AdversarialReviewResult{}, fmt.Errorf("adversarial judge %q call failed: %w", lens.Key, err)
		}
		judgement := parseJudgeResponse(lens.Key, content)
		if judgement.Verdict == AdversarialVerdictRefuted {
			result.RefutedCount++
		}
		result.Judgements = append(result.Judgements, judgement)
	}
	if result.RefutedCount >= adversarialRefuteThreshold(result.JudgeCount) {
		result.Aggregate = AdversarialAggregateUnsatisfied
	} else {
		result.Aggregate = AdversarialAggregateSatisfied
	}
	return result, nil
}

// adversarialRefuteThreshold is ceil(judgeCount/2): the number of refutals that
// constitutes a majority. For N=2 this is 1, so any single refute kills
// (conservative, matching the "2 判官偏保守" posture).
func adversarialRefuteThreshold(judgeCount int) int {
	return (judgeCount + 1) / 2
}

// parseJudgeResponse decodes one judge's {verdict, reason} response. Anything
// that is not a clean, explicit "accepted" verdict is treated as refuted:
//   - a JSON parse failure → refuted, reason=parse_failed
//   - a parsed but unknown/blank verdict → refuted, keeping the model's reason
//
// This is the 宁严勿漏 default: only an explicit accepted verdict passes a judge.
func parseJudgeResponse(lensKey, content string) AdversarialJudgement {
	var decoded struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(content)), &decoded); err != nil {
		return AdversarialJudgement{Lens: lensKey, Verdict: AdversarialVerdictRefuted, Reason: adversarialReasonParseFailed}
	}
	reason := strings.TrimSpace(decoded.Reason)
	switch strings.ToLower(strings.TrimSpace(decoded.Verdict)) {
	case AdversarialVerdictAccepted:
		return AdversarialJudgement{Lens: lensKey, Verdict: AdversarialVerdictAccepted, Reason: reason}
	case AdversarialVerdictRefuted:
		return AdversarialJudgement{Lens: lensKey, Verdict: AdversarialVerdictRefuted, Reason: reason}
	default:
		// Unknown verdict token: conservative refute, but preserve the reason so
		// the ambiguity is diagnosable.
		if reason == "" {
			reason = adversarialReasonParseFailed
		}
		return AdversarialJudgement{Lens: lensKey, Verdict: AdversarialVerdictRefuted, Reason: reason}
	}
}
