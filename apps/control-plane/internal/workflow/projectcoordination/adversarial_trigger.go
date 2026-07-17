package projectcoordination

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// Adversarial-review trigger + verdict projection (autonomy posture Phase B).
//
// When a reviewed task completes and is accepted, the coordinator (guarded by a
// GetVersion fence in handleEmployeeTaskCompleted) runs ONE orchestrating
// Activity — AdversarialReviewForTask — per completion. That Activity:
//   1. asks the store to prepare the review (load the completed task, find the
//      adversarial_review criteria it satisfies, assemble evidence + budget +
//      judge-count from policy);
//   2. runs the pure judge engine IN-PROCESS (a.RunAdversarialReview, NOT a
//      nested Temporal activity) for each criterion;
//   3. persists the aggregate verdict (judge_type=adversarial) + per-lens detail
//      rows through the store.
//
// It returns a compact summary so the workflow can decide, deterministically,
// whether to unlock downstream (AllSatisfied) or hold the demand for the
// convergence gate / human tier-3 path (unsatisfied or escalate_human).

// adversarialReviewStore is the narrow store seam the orchestrating Activity
// needs. It is type-asserted off a.store (mirroring taskResultDecisionInspector
// et al.) so the ActivityStore interface — and its many fakes — need not grow.
type adversarialReviewStore interface {
	PrepareAdversarialReview(ctx context.Context, input PrepareAdversarialReviewInput) (AdversarialReviewPlan, error)
	PersistAdversarialOutcome(ctx context.Context, input PersistAdversarialOutcomeInput) error
}

// AdversarialReviewForTaskInput identifies the just-completed, accepted task.
type AdversarialReviewForTaskInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CompletedTaskID uuid.UUID
}

// AdversarialReviewForTaskResult is the deterministic summary the workflow
// branches on. Reviewed is false when the completed task satisfies no
// adversarial_review criterion (nothing to judge). AllSatisfied is true only
// when every reviewed criterion aggregated to satisfied. AnyEscalated is true
// when any criterion hit the budget short-circuit (escalate_human).
//
// Task 4 (Phase C1) additions: ReviewedTaskID/DemandID/PlanRevisionID carry the
// coordinates the workflow needs to dispatch an auto-rework activity WITHOUT an
// extra store lookup; HeldCriteria lists every criterion the judges REFUTED
// (Aggregate==unsatisfied) — the rework-eligible set. escalate_human criteria
// are deliberately EXCLUDED from HeldCriteria (they route to the human path via
// AnyEscalated, never to auto-rework).
type AdversarialReviewForTaskResult struct {
	Reviewed       bool
	AllSatisfied   bool
	AnyEscalated   bool
	ReviewedTaskID uuid.UUID
	DemandID       uuid.UUID
	PlanRevisionID uuid.UUID
	HeldCriteria   []HeldAdversarialCriterion
}

// PrepareAdversarialReviewInput asks the store to assemble the per-criterion
// judge inputs for a completed task.
type PrepareAdversarialReviewInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CompletedTaskID uuid.UUID
}

// AdversarialReviewItem is one criterion's fully-assembled judge input plus the
// demand/plan-revision coordinates needed to persist its verdict.
type AdversarialReviewItem struct {
	DemandID       uuid.UUID
	PlanRevisionID uuid.UUID
	Input          RunAdversarialReviewInput
}

// AdversarialReviewPlan is the store's answer: whether this task is on the hook
// for any adversarial_review criterion, and the assembled inputs if so.
type AdversarialReviewPlan struct {
	Reviewed bool
	Items    []AdversarialReviewItem
}

// PersistAdversarialOutcomeInput is one criterion's judged outcome to persist:
// the aggregate row (judge_type=adversarial) plus the per-lens detail rows.
type PersistAdversarialOutcomeInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	DemandID       uuid.UUID
	PlanRevisionID uuid.UUID
	CriterionID    string
	ReviewedTaskID uuid.UUID
	Aggregate      string
	Reason         string
	EvidenceRefs   []string
	Judgements     []AdversarialJudgement
}

// AdversarialReviewForTask is the orchestrating Activity. On any engine error it
// propagates (persisting NOTHING for the failing criterion) so the workflow
// holds the demand for human tier-3 escalation — a review that cannot complete
// must NEVER release the gate.
func (a *Activities) AdversarialReviewForTask(ctx context.Context, input AdversarialReviewForTaskInput) (AdversarialReviewForTaskResult, error) {
	store, ok := a.store.(adversarialReviewStore)
	if a.store == nil || !ok {
		return AdversarialReviewForTaskResult{}, ErrActivityStoreRequired
	}
	plan, err := store.PrepareAdversarialReview(ctx, PrepareAdversarialReviewInput{
		TenantID:        input.TenantID,
		ProjectID:       input.ProjectID,
		CompletedTaskID: input.CompletedTaskID,
	})
	if err != nil {
		return AdversarialReviewForTaskResult{}, err
	}
	if !plan.Reviewed || len(plan.Items) == 0 {
		return AdversarialReviewForTaskResult{Reviewed: false}, nil
	}
	result := AdversarialReviewForTaskResult{Reviewed: true, AllSatisfied: true}
	for _, item := range plan.Items {
		judgeInput := item.Input
		if judgeInput.Model == "" {
			judgeInput.Model = a.judgeModel
		}
		// All items belong to the SAME completed task / demand / plan-revision;
		// carry those coordinates so the workflow can dispatch the auto-rework
		// activity without a second store lookup (Task 4).
		result.ReviewedTaskID = judgeInput.ReviewedTaskID
		result.DemandID = item.DemandID
		result.PlanRevisionID = item.PlanRevisionID
		review, err := a.RunAdversarialReview(ctx, judgeInput)
		if err != nil {
			// Terminal error handling (spec author requirement): do NOT persist a
			// satisfied verdict, do NOT swallow. Propagating leaves the criterion
			// verdict-less so the convergence gate holds it (see
			// ResolveUnsatisfiedBlockingCriteria's adversarial branch) and the
			// workflow routes the demand to a human tier-3 hold.
			return AdversarialReviewForTaskResult{}, fmt.Errorf("adversarial review for criterion %q failed: %w", judgeInput.CriterionID, err)
		}
		if err := store.PersistAdversarialOutcome(ctx, PersistAdversarialOutcomeInput{
			TenantID:       input.TenantID,
			ProjectID:      input.ProjectID,
			DemandID:       item.DemandID,
			PlanRevisionID: item.PlanRevisionID,
			CriterionID:    judgeInput.CriterionID,
			ReviewedTaskID: judgeInput.ReviewedTaskID,
			Aggregate:      review.Aggregate,
			Reason:         adversarialOutcomeReason(review),
			EvidenceRefs:   judgeInput.EvidenceRefs,
			Judgements:     review.Judgements,
		}); err != nil {
			return AdversarialReviewForTaskResult{}, err
		}
		switch review.Aggregate {
		case AdversarialAggregateSatisfied:
			// released for this criterion
		case AdversarialAggregateEscalateHuman:
			// Budget short-circuit → human path. NOT a held (rework-eligible)
			// criterion: AnyEscalated flags it, and it is excluded from
			// HeldCriteria so it never drives an auto-rework.
			result.AllSatisfied = false
			result.AnyEscalated = true
		default: // unsatisfied (majority refute) → rework-eligible held criterion
			result.AllSatisfied = false
			result.HeldCriteria = append(result.HeldCriteria, HeldAdversarialCriterion{
				CriterionID: judgeInput.CriterionID,
				Statement:   judgeInput.Assertion,
			})
		}
	}
	return result, nil
}

func adversarialOutcomeReason(r AdversarialReviewResult) string {
	if r.Aggregate == AdversarialAggregateEscalateHuman {
		return "预算触顶：转人类档3审查"
	}
	return fmt.Sprintf("%d/%d 判官证伪", r.RefutedCount, r.JudgeCount)
}

// PrepareAdversarialReview loads the completed task, scopes the demand's
// adversarial_review criteria to those it satisfies, and assembles one judge
// input per criterion. Evidence comes from the reviewed task's OWN latest
// result contract (not upstream results). Judge count comes from coordination
// policy; the budget short-circuit comes from revisionBudgetExhausted.
func (s *ProjectStore) PrepareAdversarialReview(ctx context.Context, input PrepareAdversarialReviewInput) (AdversarialReviewPlan, error) {
	if s.repository == nil {
		return AdversarialReviewPlan{}, ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.CompletedTaskID)
	if err != nil {
		return AdversarialReviewPlan{}, err
	}
	if task.ProjectID != input.ProjectID {
		return AdversarialReviewPlan{}, project.ErrProjectNotFound
	}
	criteria, err := s.listAdversarialCriteriaForTask(ctx, task)
	if err != nil {
		return AdversarialReviewPlan{}, err
	}
	if len(criteria) == 0 {
		return AdversarialReviewPlan{Reviewed: false}, nil
	}
	latest, err := s.latestTaskResult(ctx, task)
	if err != nil {
		return AdversarialReviewPlan{}, err
	}
	evidenceSummary, deliverables, evidenceRefs := adversarialEvidenceFromResult(latest)

	judgeCountPolicy := 0
	if projectRecord, perr := s.repository.GetProject(ctx, input.TenantID, input.ProjectID); perr == nil {
		judgeCountPolicy = adversarialJudgeCount(projectRecord.CoordinationPolicy)
	}
	budgetExhausted := s.revisionBudgetExhausted(ctx, input.TenantID, input.ProjectID, task)

	plan := AdversarialReviewPlan{Reviewed: true, Items: make([]AdversarialReviewItem, 0, len(criteria))}
	for _, c := range criteria {
		plan.Items = append(plan.Items, AdversarialReviewItem{
			DemandID:       c.DemandID,
			PlanRevisionID: c.PlanRevisionID,
			Input: RunAdversarialReviewInput{
				TenantID:         input.TenantID,
				ProjectID:        input.ProjectID,
				CriterionID:      c.CriterionID,
				ReviewedTaskID:   task.ID,
				Assertion:        c.Statement,
				EvidenceSummary:  evidenceSummary,
				Deliverables:     deliverables,
				EvidenceRefs:     evidenceRefs,
				JudgeCountPolicy: judgeCountPolicy,
				BudgetExhausted:  budgetExhausted,
			},
		})
	}
	return plan, nil
}

// listAdversarialCriteriaForTask narrows the demand+revision criteria snapshot
// to the blocking-or-not adversarial_review criteria this task is on the hook
// for: VerificationMethod==adversarial_review AND SatisfiedBy names the task's
// planned KEY (not UUID) — the same identity rule as
// project.criteriaSatisfiedByTask. A task with no demand/plan-revision/planned
// key is on the hook for nothing.
//
// A rework task minted by reworkTask/reviseTask gets a NEW PlannedTaskKey
// (revisionTaskKey: "<base>#revision-<resultID>") distinct from the task it
// revises, but the demand's adversarial_review criteria still name the
// ORIGINAL (revision-root) task's key in SatisfiedBy — that snapshot is fixed
// at planning time and is never rewritten as revisions chain. So a rework
// task is also on the hook for any criterion whose SatisfiedBy names the
// revision-root task's planned key, not just its own — otherwise a completed
// rework never re-fires the judges and the self-iteration loop silently
// breaks after one round.
func (s *ProjectStore) listAdversarialCriteriaForTask(ctx context.Context, task project.ProjectTask) ([]project.DemandAcceptanceCriterion, error) {
	return s.listCriteriaForTaskByMethod(ctx, task, VerificationMethodAdversarialReview)
}

// listCriteriaForTaskByMethod is the method-parameterized generalization of the
// per-task criterion scoping rule shared by the adversarial-review and
// review-gate triggers: it narrows the demand+revision criteria snapshot to the
// criteria whose VerificationMethod == method AND whose SatisfiedBy names the
// task's planned KEY (its own key, or — for a rework task — the revision-root
// ancestor's key; see revisionRootPlannedTaskKey and the doc on
// listAdversarialCriteriaForTask). A task with no demand/plan-revision/planned
// key is on the hook for nothing.
func (s *ProjectStore) listCriteriaForTaskByMethod(ctx context.Context, task project.ProjectTask, method string) ([]project.DemandAcceptanceCriterion, error) {
	if task.DemandID == nil || task.AcceptedPlanRevisionID == nil || task.PlannedTaskKey == nil {
		return nil, nil
	}
	taskKey := strings.TrimSpace(*task.PlannedTaskKey)
	if taskKey == "" {
		return nil, nil
	}
	matchKeys := map[string]struct{}{taskKey: {}}
	if rootKey := s.revisionRootPlannedTaskKey(ctx, task); rootKey != "" {
		matchKeys[rootKey] = struct{}{}
	}
	criteria, err := s.repository.ListDemandAcceptanceCriteria(ctx, task.TenantID, *task.DemandID, *task.AcceptedPlanRevisionID)
	if err != nil {
		return nil, err
	}
	scoped := make([]project.DemandAcceptanceCriterion, 0, len(criteria))
	for _, c := range criteria {
		if c.VerificationMethod != method {
			continue
		}
		for _, satisfiedBy := range c.SatisfiedBy {
			if _, ok := matchKeys[strings.TrimSpace(satisfiedBy)]; ok {
				scoped = append(scoped, c)
				break
			}
		}
	}
	return scoped, nil
}

// revisionRootPlannedTaskKey resolves the PlannedTaskKey of task's
// revision-root ancestor, so listAdversarialCriteriaForTask can match
// SatisfiedBy entries that still name the original (pre-revision) task. It
// returns "" — falling back to matching the task's own key only — when task
// is not a revision, the root cannot be resolved, or the root has no planned
// key. Root lookup failure must never error the whole trigger: a review that
// under-matches degrades to "not re-triggered," which is the pre-fix
// behavior, not a new failure mode.
func (s *ProjectStore) revisionRootPlannedTaskKey(ctx context.Context, task project.ProjectTask) string {
	if task.RevisionOfTaskID == nil {
		if _, hasMetadataRoot := task.PlannerMetadata["revision_root_task_id"]; !hasMetadataRoot {
			return ""
		}
	}
	if s.repository == nil {
		return ""
	}
	rootIDValue := revisionRootTaskID(task)
	if rootIDValue == "" || rootIDValue == task.ID.String() {
		return ""
	}
	rootID, err := uuid.Parse(rootIDValue)
	if err != nil {
		return ""
	}
	root, err := s.repository.GetProjectTask(ctx, task.TenantID, rootID)
	if err != nil {
		return ""
	}
	if root.PlannedTaskKey == nil {
		return ""
	}
	return strings.TrimSpace(*root.PlannedTaskKey)
}

// PersistAdversarialOutcome writes one criterion's aggregate verdict
// (judge_type=adversarial, project_task_id nil) plus its per-lens detail rows.
// Both are upserts, so a task retry re-running the judges is idempotent.
func (s *ProjectStore) PersistAdversarialOutcome(ctx context.Context, input PersistAdversarialOutcomeInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	if err := s.repository.CreateAdversarialVerdict(ctx, project.CreateAdversarialVerdictRequest{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		DemandID:       input.DemandID,
		PlanRevisionID: input.PlanRevisionID,
		CriterionID:    input.CriterionID,
		Verdict:        input.Aggregate,
		JudgeID:        uuid.Nil,
		Reason:         input.Reason,
		EvidenceRefs:   input.EvidenceRefs,
	}); err != nil {
		return err
	}
	if len(input.Judgements) == 0 {
		return nil
	}
	reqs := make([]project.CreateAdversarialJudgementRequest, 0, len(input.Judgements))
	for _, j := range input.Judgements {
		reqs = append(reqs, project.CreateAdversarialJudgementRequest{
			TenantID:       input.TenantID,
			ProjectID:      input.ProjectID,
			DemandID:       input.DemandID,
			PlanRevisionID: input.PlanRevisionID,
			CriterionID:    input.CriterionID,
			ReviewedTaskID: input.ReviewedTaskID,
			Lens:           j.Lens,
			Verdict:        j.Verdict,
			Reason:         j.Reason,
		})
	}
	return s.repository.CreateAdversarialJudgements(ctx, reqs)
}

// adversarialJudgeCount reads coordination_policy.adversarial_review_judges as a
// three-state int (int/float64/json.Number via int32FromAny). Returns 0 when
// unset or non-positive so the engine applies its default (3, hard cap 7).
func adversarialJudgeCount(policy map[string]any) int {
	if value, ok := int32FromAny(policy["adversarial_review_judges"]); ok && value > 0 {
		return int(value)
	}
	return 0
}

// adversarialEvidenceFromResult projects the reviewed task's OWN latest result
// contract into judge evidence: summary, deliverable names, and evidence refs.
func adversarialEvidenceFromResult(result *project.ProjectTaskResult) (summary string, deliverables []string, evidenceRefs []string) {
	if result == nil {
		return "", nil, nil
	}
	contract := result.Contract
	summary = contract.Summary
	for _, d := range contract.Deliverables {
		name := strings.TrimSpace(d.Name)
		if name != "" {
			deliverables = append(deliverables, name)
		}
	}
	for _, ref := range contract.EvidenceRefs {
		if v := firstNonEmptyRef(ref.Ref, ref.URI, ref.URL, ref.ID); v != "" {
			evidenceRefs = append(evidenceRefs, v)
		}
	}
	return summary, deliverables, evidenceRefs
}

func firstNonEmptyRef(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
