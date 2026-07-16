package project

import "github.com/google/uuid"

// demandAcceptanceCriterionSeverityBlocking mirrors the planner-side severity
// vocabulary (projectcoordination.CriterionSeverityBlocking) at the
// persistence layer: the convergence gate only cares whether a snapshotted
// criterion is "blocking" — non_blocking criteria never hold a demand at
// acceptance_pending.
const demandAcceptanceCriterionSeverityBlocking = "blocking"

const (
	demandCriterionVerdictSatisfied = "satisfied"
	demandCriterionJudgeTypeHuman   = "human"
)

// ResolveUnsatisfiedBlockingCriteria returns the criterion_ids (snapshot
// order) of blocking criteria not satisfied under the demand-acceptance
// convergence gate's resolution rule:
//
//   - A human verdict for a criterion — satisfied or unsatisfied — always
//     takes precedence over any executor verdict for the same criterion.
//   - Absent a human verdict, the criterion is satisfied iff at least one
//     executor verdict for it is satisfied.
//   - A blocking criterion with no verdict at all is unsatisfied (awaiting
//     sign-off).
//   - non_blocking criteria are never included, regardless of verdicts.
//
// Shared by PgRepository.CountUnsatisfiedBlockingCriteria (the recompute
// gate's hold/release signal) and projectcoordination's
// ensureDemandAcceptanceDecision (the demand_acceptance decision's
// pending_criteria payload) so both read the identical rule.
func ResolveUnsatisfiedBlockingCriteria(criteria []DemandAcceptanceCriterion, verdicts []DemandCriterionVerdict) []string {
	type resolvedVerdict struct {
		human             *string
		executorSatisfied bool
	}
	states := make(map[string]*resolvedVerdict, len(verdicts))
	for _, v := range verdicts {
		st := states[v.CriterionID]
		if st == nil {
			st = &resolvedVerdict{}
			states[v.CriterionID] = st
		}
		if v.JudgeType == demandCriterionJudgeTypeHuman {
			value := v.Verdict
			st.human = &value
			continue
		}
		if v.Verdict == demandCriterionVerdictSatisfied {
			st.executorSatisfied = true
		}
	}
	pending := make([]string, 0)
	for _, c := range criteria {
		if c.Severity != demandAcceptanceCriterionSeverityBlocking {
			continue
		}
		satisfied := false
		if st, ok := states[c.CriterionID]; ok {
			if st.human != nil {
				satisfied = *st.human == demandCriterionVerdictSatisfied
			} else {
				satisfied = st.executorSatisfied
			}
		}
		if !satisfied {
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
