package projectcoordination

// 检测门执行核心（Task 4，见 docs/superpowers/specs/2026-07-17-review-gate-violation-detection-design.md §1/§10）。
//
// 三段职责：
//   1. assembleDetectionArtifact：把被审任务的真实结果契约投影成 DetectionArtifact（真工件切片）；
//   2. runReviewGate：对已启用条件逐个 Detect，按 action 档 + minor 容忍聚合出一个 review_gate 判定（纯函数，可单测）；
//   3. projectReviewGateVerdict：把聚合判定写成一条 demand_criterion_verdicts 的 review_gate 聚合行（幂等）。
//
// 设计诚实边界（与 detector.go 顶部一致）：审阅门是多个独立检测器的并集，不是综合评分器；
// "全未检出"是默认放行方向，不代表工件被判定为"正确"。

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// detectionSeverityMinor 是检测结果的最低严重档；只有 minor 档命中才受 minorTolerance 容忍，
// block/major 档命中一律计入 HOLD。取值与 DetectionResult.Severity 的 "block"|"major"|"minor" 一致。
const detectionSeverityMinor = "minor"

// ReviewGateOutcome 是检测门对一个被审工件的聚合判定：是否 HOLD（Violated）、若 HOLD 取最严
// 动作档（Action，block > need_human），以及所有 Detected 命中的原始结果（Findings，无论其 action
// 档是否导致 HOLD 都记录，供审计/reason/evidence 使用）。
type ReviewGateOutcome struct {
	Violated bool
	Action   string
	Findings []DetectionResult
}

// assembleDetectionArtifact 把被审任务自己最新的结果契约投影成 DetectionArtifact：
//   - Summary/Deliverables(名)/EvidenceRefs 复用 adversarialEvidenceFromResult（与对抗评审同源）；
//   - DiffText 取契约里"内联可见"的真实改动文本：交付物的内联 Value、changes_made 的 Summary，
//     若都没有则回退到契约 Summary。
//
// P1 detectors scan inline artifact fields; fetching object-storage diff content is a
// follow-up. 也就是说，若真实 diff 只以对象存储 artifact ref（evidence_refs/artifact_refs 的
// ref/uri/sha256）形式存在、内联字段里没有内容，这里不会去拉取对象内容，DiffText 只由上述内联
// 来源填充（E2E 会把可检测内容放进内联字段）。
func assembleDetectionArtifact(result *project.ProjectTaskResult) DetectionArtifact {
	summary, deliverables, evidenceRefs := adversarialEvidenceFromResult(result)
	art := DetectionArtifact{
		Summary:      summary,
		Deliverables: deliverables,
		EvidenceRefs: evidenceRefs,
	}
	if result == nil {
		return art
	}
	art.DiffText = inlineDiffText(result.Contract)
	return art
}

// inlineDiffText 收集契约里内联可见的改动文本：交付物 Value（执行者把真实 diff/代码作为内联值
// 交付时）+ changes_made 的 Summary。若没有任何内联改动文本，回退到契约 Summary（散文兜底）。
// 只读内联字段，不解引用对象存储 artifact ref（见 assembleDetectionArtifact 的 P1 说明）。
func inlineDiffText(contract project.TaskResultContract) string {
	parts := make([]string, 0, len(contract.Deliverables)+len(contract.ChangesMade))
	for _, d := range contract.Deliverables {
		if v := strings.TrimSpace(d.Value); v != "" {
			parts = append(parts, v)
		}
	}
	for _, c := range contract.ChangesMade {
		if s := strings.TrimSpace(c.Summary); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return contract.Summary
	}
	return strings.Join(parts, "\n")
}

// runReviewGate 是聚合核心（纯函数，可单测）：对每个已启用条件跑 Detect，收集 Detected 命中，
// 按 action 档 + minor 容忍聚合出 ReviewGateOutcome。
//
// 聚合规则：
//   - 只有 action∈{block,need_human} 的命中才可能导致 HOLD；record_only 命中记入 Findings 但不
//     置 Violated。
//   - minor 严重档的命中受 minorTolerance 容忍：当"会 HOLD 的 minor 命中"数量不超过容忍值时，它们
//     不计入 HOLD；一旦超过容忍值，全部 minor 命中都计入。block/major 档命中一律计入。
//   - Violated = 存在一个 Detected 命中，其 action∈{block,need_human} 且（severity != minor 或
//     会 HOLD 的 minor 命中数超过容忍值）。
//   - Action = 计入 HOLD 的命中里最严的 action 档（block > need_human）；未 HOLD 时为空。
//   - 全未检出 → ReviewGateOutcome{Violated:false, Findings:nil}（默认放行）。
func runReviewGate(ctx context.Context, art DetectionArtifact, enabled []EnabledCondition, minorTolerance int) ReviewGateOutcome {
	type scoredFinding struct {
		result DetectionResult
		action string
	}
	scored := make([]scoredFinding, 0, len(enabled))
	findings := make([]DetectionResult, 0, len(enabled))
	minorHoldingCount := 0

	for _, cond := range enabled {
		res := cond.Spec.Detector.Detect(ctx, art)
		if !res.Detected {
			continue
		}
		findings = append(findings, res)
		scored = append(scored, scoredFinding{result: res, action: cond.Action})
		if isHoldingAction(cond.Action) && res.Severity == detectionSeverityMinor {
			minorHoldingCount++
		}
	}

	if len(findings) == 0 {
		return ReviewGateOutcome{Violated: false, Findings: nil}
	}

	minorExceeded := minorHoldingCount > minorTolerance
	violated := false
	strictest := ""
	for _, sf := range scored {
		if !isHoldingAction(sf.action) {
			continue // record_only（或任何非 block/need_human）永不 HOLD
		}
		// minor 档命中只有在超过容忍值时才计入；block/major 档一律计入。
		if sf.result.Severity == detectionSeverityMinor && !minorExceeded {
			continue
		}
		violated = true
		strictest = stricterReviewGateAction(strictest, sf.action)
	}

	if !violated {
		return ReviewGateOutcome{Violated: false, Findings: findings}
	}
	return ReviewGateOutcome{Violated: true, Action: strictest, Findings: findings}
}

// isHoldingAction 判断一个已解析动作档是否可能导致 HOLD：只有 block 和 need_human 会拦截，
// record_only（及任何其他值）只记录不拦截。
func isHoldingAction(action string) bool {
	return action == reviewGateActionBlock || action == reviewGateActionNeedHuman
}

// stricterReviewGateAction 在两个动作档中取更严的一个：block > need_human。空串视为"尚无"，
// 直接被任何具体档取代。
func stricterReviewGateAction(current, candidate string) string {
	if current == reviewGateActionBlock || candidate == reviewGateActionBlock {
		return reviewGateActionBlock
	}
	if current == "" {
		return candidate
	}
	return current
}

// ReviewGateVerdictInput 是把一条聚合判定写成 review_gate verdict 所需的坐标 + 判定本身。
// Task 4 不接工作流触发；调用方（Task 5/6）显式提供 demand/plan-revision/criterion 坐标。
type ReviewGateVerdictInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	DemandID       uuid.UUID
	PlanRevisionID uuid.UUID
	CriterionID    string
	Outcome        ReviewGateOutcome
}

// projectReviewGateVerdict 把一条 ReviewGateOutcome 投影成 demand_criterion_verdicts 的一条
// review_gate 聚合行（judge_type=review_gate，project_task_id NULL）：Violated → unsatisfied，
// 否则 satisfied。命中 uq_demand_verdicts_review_gate 唯一索引 upsert，重跑幂等。
func (s *ProjectStore) projectReviewGateVerdict(ctx context.Context, input ReviewGateVerdictInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	verdict := reviewGateVerdictSatisfied
	if input.Outcome.Violated {
		verdict = reviewGateVerdictUnsatisfied
	}
	return s.repository.CreateReviewGateVerdict(ctx, project.CreateReviewGateVerdictRequest{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		DemandID:       input.DemandID,
		PlanRevisionID: input.PlanRevisionID,
		CriterionID:    input.CriterionID,
		Verdict:        verdict,
		Reason:         reviewGateOutcomeReason(input.Outcome),
		EvidenceRefs:   reviewGateEvidenceRefs(input.Outcome),
	})
}

// reviewGateVerdictSatisfied / Unsatisfied 是 review_gate 聚合行的 verdict 取值，与
// demand_criterion_verdicts 的既有语义一致（satisfied 放行、unsatisfied HOLD）。
const (
	reviewGateVerdictSatisfied   = "satisfied"
	reviewGateVerdictUnsatisfied = "unsatisfied"
)

// reviewGateOutcomeReason 把聚合判定压成一句人读的 reason：未 HOLD 时说明默认放行；HOLD 时带上
// 动作档 + 每条命中的 finding 摘要。
func reviewGateOutcomeReason(outcome ReviewGateOutcome) string {
	if !outcome.Violated {
		if len(outcome.Findings) == 0 {
			return "检测门无命中：默认放行"
		}
		return "检测门命中均为记录级/在容忍范围内：默认放行"
	}
	findings := make([]string, 0, len(outcome.Findings))
	for _, f := range outcome.Findings {
		key := f.ConditionKey
		if key == "" {
			key = "unknown"
		}
		finding := strings.TrimSpace(f.Finding)
		if finding == "" {
			finding = "（无说明）"
		}
		findings = append(findings, fmt.Sprintf("%s: %s", key, finding))
	}
	return fmt.Sprintf("检测门 HOLD（动作档 %s）：%s", outcome.Action, strings.Join(findings, "；"))
}

// reviewGateEvidenceRefs 汇总所有命中的 evidence_refs 去重后返回，作为 verdict 的证据指针。
func reviewGateEvidenceRefs(outcome ReviewGateOutcome) []string {
	seen := make(map[string]struct{})
	refs := make([]string, 0)
	for _, f := range outcome.Findings {
		for _, ref := range f.EvidenceRefs {
			r := strings.TrimSpace(ref)
			if r == "" {
				continue
			}
			if _, ok := seen[r]; ok {
				continue
			}
			seen[r] = struct{}{}
			refs = append(refs, r)
		}
	}
	if len(refs) == 0 {
		return nil
	}
	return refs
}
