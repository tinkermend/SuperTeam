package project

import "github.com/google/uuid"

// demandAcceptanceCriterionSeverityBlocking mirrors the planner-side severity
// vocabulary (projectcoordination.CriterionSeverityBlocking) at the
// persistence layer: the convergence gate only cares whether a snapshotted
// criterion is "blocking" — non_blocking criteria never hold a demand at
// acceptance_pending.
const demandAcceptanceCriterionSeverityBlocking = "blocking"

const (
	demandCriterionVerdictSatisfied   = "satisfied"
	demandCriterionVerdictUnsatisfied = "unsatisfied"
	// demandCriterionVerdictNotApplicable is the third verdict value, projected
	// only from an executor's contract-valid not_applicable acceptance result
	// (which requires a human_accepted_reason) for an automated_test criterion.
	// It RELEASES the convergence gate for THAT criterion — a blocking
	// automated_test criterion the executor justifiably marked N/A must not
	// deadlock the demand. A human verdict still overrides it (see
	// criterionEffectiveVerdict). Humans never sign not_applicable directly
	// (SignDemandCriterionVerdict rejects it), and the executor-verdict
	// projection path (Service.projectDemandCriterionVerdicts) explicitly
	// skips human_judgment criteria — so not_applicable can only ever apply
	// to automated_test criteria, never to the human backstop itself.
	//
	// The human_judgment fallback criterion is NOT mandatory since the
	// autonomy-posture default flip (Task 1 of the 2026-07 autonomy posture
	// calibration): a low-risk demand may legitimately carry no human
	// criterion at all, in which case not_applicable-ing every automated_test
	// criterion genuinely completes the demand with zero human touchpoints —
	// that is the intended, bounded outcome for low-risk work.
	// The bound holds for high-risk work: every high-risk-classified demand
	// (projectcoordination.planTouchesHighRisk — constitutional, never
	// policy-exemptable) gets a human_judgment blocking criterion injected
	// onto its snapshot (projectcoordination.ensureHumanJudgmentCriterion,
	// re-run in the planner AFTER every high-risk flag is final — so a plan
	// pushed high-risk only by governance or scoring still carries it, not
	// just plans high-risk at decode time), and an executor cannot
	// self-satisfy or self-N/A that criterion (see above). So an executor
	// N/A-ing every automated_test criterion on a high-risk demand still
	// leaves the injected human criterion unresolved, holding the gate at
	// acceptance_pending. See TestNotApplicableDoesNotEscapeHighRiskOversight
	// (pg_repository_test.go) for the pinned gate-layer proof, and
	// TestGovernanceDerivedHighRiskInjectsHumanCriterion
	// (openai_compatible_planner_test.go) for the injection-timing proof.
	demandCriterionVerdictNotApplicable = "not_applicable"
	demandCriterionJudgeTypeHuman       = "human"
	// demandCriterionVerificationMethodHumanJudgment mirrors
	// projectcoordination.VerificationMethodHumanJudgment at the persistence
	// layer (see acceptance_criteria.go's knownVerificationMethods registry).
	// Service.SignDemandCriterionVerdict only accepts sign-off against
	// criteria snapshotted with this verification method — Phase 1 leaves
	// automated_test criteria to the executor-projection verdict path.
	demandCriterionVerificationMethodHumanJudgment = "human_judgment"
	demandCriterionJudgeTypeExecutor               = "executor"
	// demandCriterionVerificationMethodAdversarialReview mirrors
	// projectcoordination.VerificationMethodAdversarialReview at the
	// persistence layer (see acceptance_criteria.go's knownVerificationMethods
	// registry). Registered here for the persistence-side mirror; the judge
	// engine that produces verdicts against this method is a later task.
	demandCriterionVerificationMethodAdversarialReview = "adversarial_review"
	// demandCriterionJudgeTypeAdversarial mirrors the judge_type recorded
	// against a DemandCriterionVerdict produced by the adversarial-review
	// judge (a later task); registered here alongside its verification
	// method so both persistence-side mirrors land together.
	demandCriterionJudgeTypeAdversarial = "adversarial"
)

// criterionEffectiveVerdict resolves a single criterion's effective verdict
// under the demand-acceptance precedence rule, the single source shared by the
// convergence gate and the acceptance panel read model:
//
//   - A human verdict (satisfied or unsatisfied) always wins.
//   - Absent a human verdict, the criterion is satisfied iff at least one
//     executor verdict for it is satisfied; else not_applicable iff at least
//     one executor verdict for it is not_applicable (both release the gate);
//     with only unsatisfied executor verdicts the effective verdict is
//     unsatisfied.
//   - With no verdict at all it is unresolved (hasVerdict=false).
//
// It also returns the judge type and evidence refs behind the winning verdict
// so callers can surface provenance without re-scanning.
func criterionEffectiveVerdict(verdicts []DemandCriterionVerdict, criterionID string) (verdict string, judgeType string, evidenceRefs []string, hasVerdict bool) {
	var human *DemandCriterionVerdict
	var adversarial *DemandCriterionVerdict
	var executorSatisfied *DemandCriterionVerdict
	var executorNotApplicable *DemandCriterionVerdict
	var executorAny *DemandCriterionVerdict
	for i := range verdicts {
		v := &verdicts[i]
		if v.CriterionID != criterionID {
			continue
		}
		if v.JudgeType == demandCriterionJudgeTypeHuman {
			human = v // last human verdict wins (arrives in created_at,id order)
			continue
		}
		if v.JudgeType == demandCriterionJudgeTypeAdversarial {
			// The adversarial aggregate row (one global row per criterion,
			// project_task_id NULL) is NOT an executor verdict — catch it here
			// before the executor accumulators so it is never miscounted as one.
			adversarial = v
			continue
		}
		if executorAny == nil {
			executorAny = v
		}
		if v.Verdict == demandCriterionVerdictSatisfied && executorSatisfied == nil {
			executorSatisfied = v
		}
		if v.Verdict == demandCriterionVerdictNotApplicable && executorNotApplicable == nil {
			executorNotApplicable = v
		}
	}
	switch {
	case human != nil:
		return human.Verdict, demandCriterionJudgeTypeHuman, human.EvidenceRefs, true
	case adversarial != nil:
		// An adversarial_review criterion's effective verdict is its adversarial
		// aggregate verdict (satisfied | unsatisfied | escalate_human). A human
		// verdict still overrides it (handled above); the executor never decides
		// an adversarial_review criterion.
		return adversarial.Verdict, demandCriterionJudgeTypeAdversarial, adversarial.EvidenceRefs, true
	case executorSatisfied != nil:
		return demandCriterionVerdictSatisfied, demandCriterionJudgeTypeExecutor, executorSatisfied.EvidenceRefs, true
	case executorNotApplicable != nil:
		return demandCriterionVerdictNotApplicable, demandCriterionJudgeTypeExecutor, executorNotApplicable.EvidenceRefs, true
	case executorAny != nil:
		return executorAny.Verdict, demandCriterionJudgeTypeExecutor, executorAny.EvidenceRefs, true
	default:
		return "", "", nil, false
	}
}

// ResolveUnsatisfiedBlockingCriteria returns the criterion_ids (snapshot
// order) of blocking criteria not satisfied under the demand-acceptance
// convergence gate's resolution rule:
//
//   - A human verdict for a criterion — satisfied or unsatisfied — always
//     takes precedence over any executor verdict for the same criterion.
//   - Absent a human verdict, the criterion is released iff at least one
//     executor verdict for it is satisfied or not_applicable (the executor's
//     justified N/A on an automated_test criterion releases the gate for
//     that criterion only; see demandCriterionVerdictNotApplicable for why
//     this cannot escape oversight on high-risk demands — which always
//     carry an injected human_judgment criterion the executor cannot
//     touch — and legitimately needs no human backstop on low-risk
//     demands that carry no human criterion at all).
//   - A blocking criterion with no verdict at all is unsatisfied (awaiting
//     sign-off).
//   - non_blocking criteria are never included, regardless of verdicts.
//
// Shared by PgRepository.CountUnsatisfiedBlockingCriteria (the recompute
// gate's hold/release signal) and projectcoordination's
// ensureDemandAcceptanceDecision (the demand_acceptance decision's
// pending_criteria payload) so both read the identical rule.
func ResolveUnsatisfiedBlockingCriteria(criteria []DemandAcceptanceCriterion, verdicts []DemandCriterionVerdict) []string {
	pending := make([]string, 0)
	for _, c := range criteria {
		if c.Severity != demandAcceptanceCriterionSeverityBlocking {
			continue
		}
		verdict, judgeType, _, hasVerdict := criterionEffectiveVerdict(verdicts, c.CriterionID)
		if c.VerificationMethod == demandCriterionVerificationMethodAdversarialReview {
			// An adversarial_review criterion is released ONLY by its adversarial
			// aggregate verdict (satisfied) or by a human override (satisfied). It
			// holds when: no verdict yet (review not run / in progress), the
			// adversarial aggregate is unsatisfied or escalate_human (budget
			// exhausted), or an engine error left it verdict-less. A stray
			// executor self-report must never release it — treat any non-human,
			// non-adversarial winning verdict as unresolved. This is the
			// escalate_human / engine-error hold: a review that cannot conclude
			// never lets the demand through, keeping the human tier-3 path owner.
			if !hasVerdict ||
				(judgeType != demandCriterionJudgeTypeHuman && judgeType != demandCriterionJudgeTypeAdversarial) ||
				verdict != demandCriterionVerdictSatisfied {
				pending = append(pending, c.CriterionID)
			}
			continue
		}
		if !hasVerdict || (verdict != demandCriterionVerdictSatisfied && verdict != demandCriterionVerdictNotApplicable) {
			pending = append(pending, c.CriterionID)
		}
	}
	return pending
}

// CurrentEffectivePlanRevisionID returns the highest-numbered plan revision
// for a demand that is still "open" — accepted, mid-decompose, or decomposed
// — i.e. not rejected/superseded. This is the plan revision the convergence
// gate and the demand_acceptance decision both read criteria/verdicts
// against. Demands with no open plan revision (legacy demands that predate
// the acceptance-criteria snapshot rollout, or any demand whose plan
// revisions are all rejected/superseded) return uuid.Nil, the signal both
// callers treat as "no gate, no snapshot".
func CurrentEffectivePlanRevisionID(revisions []PlanRevision) uuid.UUID {
	var bestID uuid.UUID
	var bestNumber int32 = -1
	for _, revision := range revisions {
		switch revision.Status {
		case PlanRevisionStatusAccepted, PlanRevisionStatusDecomposing, PlanRevisionStatusDecomposed:
			if revision.RevisionNumber > bestNumber {
				bestNumber = revision.RevisionNumber
				bestID = revision.ID
			}
		}
	}
	return bestID
}
