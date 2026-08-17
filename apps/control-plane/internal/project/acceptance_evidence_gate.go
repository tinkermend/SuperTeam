package project

import "strings"

// Acceptance evidence-gate outcomes (autonomy inventory F4 / §6.3 E1–E6).
const (
	AcceptanceEvidencePass              = ""
	AcceptanceEvidenceFailNoMachineProof = "e1_no_machine_proof"
	AcceptanceEvidenceFailUnsatisfied   = "e3_unsatisfied"
	AcceptanceEvidenceFailIncomplete    = "e4_incomplete"
	AcceptanceEvidenceFailDeclarativeHJ = "e6_declarative_human_judgment"
)

// CriterionSource mirrors projectcoordination criterion source tags at the
// persistence layer. Empty / unknown is treated as planner_authored for E6
// (pre-sign invariant: runtime-authored human_judgment must not park).
const (
	CriterionSourceTemplateDeclared = "template_declared"
	CriterionSourcePlatformInjected = "platform_injected"
	CriterionSourcePlannerAuthored  = "planner_authored"
)

// InferCriterionSource recovers a source tag when the snapshot row has none
// (pre-F4 rows, or heuristics while the DB column rolls out).
func InferCriterionSource(criterionID, verificationMethod, storedSource string) string {
	if s := strings.TrimSpace(storedSource); s != "" {
		return s
	}
	id := strings.TrimSpace(criterionID)
	switch {
	case id == "human_final_confirmation":
		return CriterionSourcePlatformInjected
	case strings.HasPrefix(id, "produces_delivered:"):
		return CriterionSourcePlatformInjected
	case strings.HasPrefix(id, "adversarial_review:"):
		return CriterionSourcePlatformInjected
	case strings.HasPrefix(id, "template_criterion_"):
		return CriterionSourceTemplateDeclared
	case verificationMethod == demandCriterionVerificationMethodHumanJudgment:
		return CriterionSourcePlannerAuthored
	default:
		return CriterionSourcePlannerAuthored
	}
}

// IsDeclarativeHumanJudgment reports whether a blocking human_judgment
// criterion is a pre-sign stop (template/platform) rather than planner noise.
func IsDeclarativeHumanJudgment(c DemandAcceptanceCriterion) bool {
	if c.VerificationMethod != demandCriterionVerificationMethodHumanJudgment {
		return false
	}
	if c.Severity != "" && c.Severity != demandAcceptanceCriterionSeverityBlocking {
		return false
	}
	src := InferCriterionSource(c.CriterionID, c.VerificationMethod, c.Source)
	return src == CriterionSourceTemplateDeclared || src == CriterionSourcePlatformInjected
}

// IsMachineEvidenceCriterion is true for blocking criteria that can supply
// E1 machine proof when satisfied (automated_test, adversarial_review,
// platform produces_delivered).
func IsMachineEvidenceCriterion(c DemandAcceptanceCriterion) bool {
	if c.Severity != "" && c.Severity != demandAcceptanceCriterionSeverityBlocking {
		return false
	}
	switch c.VerificationMethod {
	case demandAcceptanceVerificationMethodAutomatedTest,
		demandCriterionVerificationMethodAdversarialReview:
		return true
	default:
		return false
	}
}

// EvaluateAcceptanceEvidenceGate implements §6.3 E1–E6 for policy auto-resolve.
// Returns AcceptanceEvidencePass ("") when the demand may be signed by policy.
func EvaluateAcceptanceEvidenceGate(criteria []DemandAcceptanceCriterion, verdicts []DemandCriterionVerdict) string {
	if len(criteria) == 0 {
		return AcceptanceEvidenceFailNoMachineProof
	}
	pending := ResolveUnsatisfiedBlockingCriteria(criteria, verdicts)
	pendingSet := map[string]bool{}
	for _, id := range pending {
		pendingSet[id] = true
	}

	hasMachineProof := false
	for _, c := range criteria {
		if !IsMachineEvidenceCriterion(c) {
			continue
		}
		if pendingSet[c.CriterionID] {
			continue
		}
		verdict, _, _, has := criterionEffectiveVerdict(verdicts, c.CriterionID)
		if !has || verdict != demandCriterionVerdictSatisfied {
			continue
		}
		// E5: not_applicable never reaches here (held in Resolve…); satisfied only.
		hasMachineProof = true
		break
	}
	if !hasMachineProof {
		return AcceptanceEvidenceFailNoMachineProof
	}

	for _, c := range criteria {
		if !pendingSet[c.CriterionID] {
			continue
		}
		if IsDeclarativeHumanJudgment(c) {
			return AcceptanceEvidenceFailDeclarativeHJ
		}
		verdict, _, _, has := criterionEffectiveVerdict(verdicts, c.CriterionID)
		if !has || verdict == demandCriterionVerdictReviewGatePending || verdict == "escalate_human" {
			return AcceptanceEvidenceFailIncomplete
		}
		if verdict == demandCriterionVerdictUnsatisfied {
			return AcceptanceEvidenceFailUnsatisfied
		}
		// Remaining pending (e.g. N/A held) counts as incomplete evidence.
		return AcceptanceEvidenceFailIncomplete
	}
	return AcceptanceEvidencePass
}

// AutonomyTierSnapshotSourceRefKey is written into ProjectDemand.SourceRefs at
// create time for automation/external demands so the acceptance gate can read
// the effective tier without live rule lookups (F4 / §6.7).
const AutonomyTierSnapshotSourceRefKey = "autonomy_tier_snapshot"

// PinnedExitDeliverableSourceRefKey stamps a create-time exit pin onto the
// demand so coordination LoadSnapshot can honor it (F6 / §6.5.6).
const PinnedExitDeliverableSourceRefKey = "pinned_exit_deliverable"

// DemandAutonomyTierSnapshot returns the create-time effective autonomy tier
// snapshot from demand SourceRefs, or empty when absent (manual demands —
// keep legacy acceptance gate behaviour).
func DemandAutonomyTierSnapshot(demand ProjectDemand) string {
	if demand.SourceRefs == nil {
		return ""
	}
	raw, _ := demand.SourceRefs[AutonomyTierSnapshotSourceRefKey].(string)
	return strings.TrimSpace(raw)
}

// DemandPinnedExitDeliverable returns the create-time pinned exit, if any.
func DemandPinnedExitDeliverable(demand ProjectDemand) string {
	if demand.SourceRefs == nil {
		return ""
	}
	raw, _ := demand.SourceRefs[PinnedExitDeliverableSourceRefKey].(string)
	return strings.TrimSpace(raw)
}
