package projectcoordination

// Review-gate trigger + verdict projection (violation-detection gate, Task 6; see
// docs/superpowers/specs/2026-07-17-review-gate-violation-detection-design.md §6).
//
// This mirrors the adversarial-review trigger (adversarial_trigger.go) but drives
// the violation-DETECTION path instead of the adversarial JUDGE path. When a
// reviewed task completes and is accepted, the coordinator (guarded by an
// INDEPENDENT GetVersion fence in handleEmployeeTaskCompleted) runs ONE
// orchestrating Activity — RunReviewGateForTask — per completion. That Activity:
//   1. asks the store to prepare the gate (load the completed task, find the
//      review_gate criteria it satisfies, assemble the real detection artifact,
//      read coordination_policy);
//   2. resolves the enabled conditions + minor tolerance from policy and runs the
//      pure aggregation core (runReviewGate) IN-PROCESS over the platform's
//      standard detectors (secret_leak rule + code_review LLM);
//   3. persists ONE review_gate aggregate verdict per criterion (satisfied when no
//      violation, unsatisfied when a violation HELD) through the store.
//
// Honesty boundary (detector.go / review_gate.go top): the gate is a UNION of
// independent detectors, not a correctness/quality scorer. "No violation" is the
// DEFAULT-RELEASE direction — it does NOT judge the artifact "correct".
//
// Failure posture: LLM-detector failures fail OPEN inside the detectors (return
// Detected=false) — a detector that cannot run must not manufacture a violation.
// A store-level Activity error propagates and the workflow logs + falls through.
// Since the P1.1 placeholder-race fix, "falls through" no longer means default
// release: the reviewed task's completion already wrote a `pending` placeholder
// verdict (project.Service.projectReviewGatePlaceholderVerdicts), so a gate that
// was TRIGGERED but never concluded leaves the demand HELD at acceptance_pending
// for the human — deliberately failing toward oversight rather than releasing an
// artifact whose detection round never finished. A criterion whose task never
// completed (gate never triggered) still has no verdict and still default-releases.

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// reviewGateStore is the narrow store seam the orchestrating Activity needs. It is
// type-asserted off a.store (mirroring adversarialReviewStore) so the
// ActivityStore interface — and its many fakes — need not grow.
type reviewGateStore interface {
	PrepareReviewGate(ctx context.Context, input PrepareReviewGateInput) (ReviewGatePlan, error)
	PersistReviewGateOutcome(ctx context.Context, input PersistReviewGateOutcomeInput) error
}

// RunReviewGateForTaskInput identifies the just-completed, accepted task.
type RunReviewGateForTaskInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CompletedTaskID uuid.UUID
}

// RunReviewGateForTaskResult is the deterministic summary the workflow logs on.
// Reviewed is false when the completed task satisfies no review_gate criterion
// (nothing to detect). AnyViolation is true when the aggregated gate outcome
// HELD (a block/need_human violation survived minor tolerance) — the workflow
// does NOT branch on it (it never blocks downstream on a review_gate hold); it is
// carried for logging/observability only.
type RunReviewGateForTaskResult struct {
	Reviewed     bool
	AnyViolation bool
}

// PrepareReviewGateInput asks the store to assemble the detection artifact +
// policy + review_gate criterion coordinates for a completed task.
type PrepareReviewGateInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CompletedTaskID uuid.UUID
}

// ReviewGateItem is one review_gate criterion's demand/plan-revision coordinates
// needed to persist its verdict. The artifact + policy are SHARED across all
// items (they belong to the same completed task), so they live on the plan, not
// the item.
type ReviewGateItem struct {
	DemandID       uuid.UUID
	PlanRevisionID uuid.UUID
	CriterionID    string
}

// ReviewGatePlan is the store's answer: whether this task is on the hook for any
// review_gate criterion, and — if so — the assembled real detection artifact, the
// project's coordination policy (for enabledConditions + minor tolerance), and
// one item per criterion.
type ReviewGatePlan struct {
	Reviewed bool
	Artifact DetectionArtifact
	Policy   map[string]any
	Items    []ReviewGateItem
}

// PersistReviewGateOutcomeInput is one criterion's gate outcome to persist as a
// review_gate aggregate verdict row.
type PersistReviewGateOutcomeInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	DemandID       uuid.UUID
	PlanRevisionID uuid.UUID
	CriterionID    string
	Outcome        ReviewGateOutcome
}

// RunReviewGateForTask is the orchestrating Activity. Unlike AdversarialReviewForTask
// it does NOT propagate detector failures as a hold: the detectors fail OPEN
// internally, and only a store load/persist error propagates here — the workflow
// swallows it and falls through (leaving the completion-time `pending` placeholder
// holding the demand for the human; see the failure-posture note at the top of this
// file). It runs the standard detectors once over the artifact and writes the
// SAME aggregate outcome to every review_gate criterion the task satisfies.
func (a *Activities) RunReviewGateForTask(ctx context.Context, input RunReviewGateForTaskInput) (RunReviewGateForTaskResult, error) {
	store, ok := a.store.(reviewGateStore)
	if a.store == nil || !ok {
		return RunReviewGateForTaskResult{}, ErrActivityStoreRequired
	}
	plan, err := store.PrepareReviewGate(ctx, PrepareReviewGateInput{
		TenantID:        input.TenantID,
		ProjectID:       input.ProjectID,
		CompletedTaskID: input.CompletedTaskID,
	})
	if err != nil {
		return RunReviewGateForTaskResult{}, err
	}
	if !plan.Reviewed || len(plan.Items) == 0 {
		return RunReviewGateForTaskResult{Reviewed: false}, nil
	}
	// The detector chat client is the SAME wiring as the adversarial judge client
	// (a.judgeClient / a.judgeModel; see app.go WithJudgeClient). When it is nil
	// (unwired), the code_review LLM detector fails open (Detected=false) and only
	// the rule detectors (secret_leak) fire — a safe, fail-open degradation.
	enabled := enabledConditions(plan.Policy, standardConditions(a.judgeClient, a.judgeModel))
	tolerance := reviewGateMinorTolerance(plan.Policy)
	outcome := runReviewGate(ctx, plan.Artifact, enabled, tolerance)

	for _, item := range plan.Items {
		if err := store.PersistReviewGateOutcome(ctx, PersistReviewGateOutcomeInput{
			TenantID:       input.TenantID,
			ProjectID:      input.ProjectID,
			DemandID:       item.DemandID,
			PlanRevisionID: item.PlanRevisionID,
			CriterionID:    item.CriterionID,
			Outcome:        outcome,
		}); err != nil {
			return RunReviewGateForTaskResult{}, err
		}
	}
	return RunReviewGateForTaskResult{Reviewed: true, AnyViolation: outcome.Violated}, nil
}

// PrepareReviewGate loads the completed task, scopes the demand's review_gate
// criteria to those it satisfies (listCriteriaForTaskByMethod, the same identity
// rule the adversarial trigger uses), assembles the real detection artifact from
// the task's OWN latest result contract, and reads the project's coordination
// policy. Returns Reviewed=false (no artifact/policy work) when the task is on the
// hook for no review_gate criterion.
func (s *ProjectStore) PrepareReviewGate(ctx context.Context, input PrepareReviewGateInput) (ReviewGatePlan, error) {
	if s.repository == nil {
		return ReviewGatePlan{}, ErrActivityStoreRequired
	}
	task, err := s.repository.GetProjectTask(ctx, input.TenantID, input.CompletedTaskID)
	if err != nil {
		return ReviewGatePlan{}, err
	}
	if task.ProjectID != input.ProjectID {
		return ReviewGatePlan{}, project.ErrProjectNotFound
	}
	criteria, err := s.listCriteriaForTaskByMethod(ctx, task, VerificationMethodReviewGate)
	if err != nil {
		return ReviewGatePlan{}, err
	}
	if len(criteria) == 0 {
		return ReviewGatePlan{Reviewed: false}, nil
	}
	latest, err := s.latestTaskResult(ctx, task)
	if err != nil {
		return ReviewGatePlan{}, err
	}
	artifact := assembleDetectionArtifact(latest)

	var policy map[string]any
	if projectRecord, perr := s.repository.GetProject(ctx, input.TenantID, input.ProjectID); perr == nil {
		policy = projectRecord.CoordinationPolicy
	}

	plan := ReviewGatePlan{Reviewed: true, Artifact: artifact, Policy: policy, Items: make([]ReviewGateItem, 0, len(criteria))}
	for _, c := range criteria {
		plan.Items = append(plan.Items, ReviewGateItem{
			DemandID:       c.DemandID,
			PlanRevisionID: c.PlanRevisionID,
			CriterionID:    c.CriterionID,
		})
	}
	return plan, nil
}

// PersistReviewGateOutcome writes one criterion's aggregate gate outcome as a
// review_gate demand_criterion_verdicts row (judge_type=review_gate,
// project_task_id NULL) via the shared projection, then recomputes the demand's
// lifecycle status. The recompute is the second half of the placeholder-race
// fix (project.Service.projectReviewGatePlaceholderVerdicts): the reviewed
// task's completion synchronously wrote a `pending` placeholder that held the
// demand at acceptance_pending, and NOTHING else recomputes after this
// asynchronous verdict lands — without it a clean artifact (verdict flipped to
// satisfied) would stay held forever instead of default-releasing. The
// recompute is forward-only and idempotent: on unsatisfied it re-derives
// acceptance_pending (no change), on satisfied it converges the demand to
// completed exactly as the pre-placeholder default release did, only ~13s
// later. Upsert-idempotent, so a task retry re-running the gate is safe.
func (s *ProjectStore) PersistReviewGateOutcome(ctx context.Context, input PersistReviewGateOutcomeInput) error {
	if err := s.projectReviewGateVerdict(ctx, ReviewGateVerdictInput{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		DemandID:       input.DemandID,
		PlanRevisionID: input.PlanRevisionID,
		CriterionID:    input.CriterionID,
		Outcome:        input.Outcome,
	}); err != nil {
		return err
	}
	return s.repository.RecomputeProjectDemandStatus(ctx, input.TenantID, input.ProjectID, input.DemandID)
}
