package projectcoordination

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// noSuitableEmployeeStructuralGapMessage is the fixed message surfaced when a
// role_independence violation cannot be repaired by re-planning because the
// active executor pool is too small to make the two roles independent — see
// EnforceScenarioTemplateGovernance.
const noSuitableEmployeeStructuralGapMessage = "项目员工池无法满足审查独立性约束（需≥2名可调度员工）；可改选更浅出口、为项目补充员工、或换用模板"

// noSuitableEmployeeCastLockedGapMessage is surfaced when playbook casting (编制)
// forces the same employee onto both sides of a role_independence pair. Re-planning
// cannot help because ApplyPlaybookCasting re-applies the cast every attempt — the
// human must change casting, restaff, pick a shallower exit, or exempt (G12).
const noSuitableEmployeeCastLockedGapMessage = "剧本编制将职责分离角色指派给同一数字员工，重规划无法解除；请改编制、补员、改选更浅出口、豁免约束或换用模板"

// structuralGapError wraps ErrNoSuitableEmployee with the actionable PlanningGap a
// human needs to resolve a structural pool gap (补充员工/改浅出口/换模板), so the
// terminal activities.go wrapping (wrapNoSuitableEmployeeError) can attach the gap
// to the ApplicationError's Details without any caller having to reconstruct it
// from CoordinationSnapshot after the fact — the knowledge (constraint kind, roles,
// required capabilities, pool size) only exists here, at the point of detection.
// Unwrap returns ErrNoSuitableEmployee so errors.Is(err, ErrNoSuitableEmployee)
// keeps working unchanged for every existing caller.
type structuralGapError struct {
	msg string
	gap PlanningGap
}

func (e *structuralGapError) Error() string { return e.msg }
func (e *structuralGapError) Unwrap() error { return ErrNoSuitableEmployee }

// roleCapabilitiesFromSpec indexes a template spec's declared roles by key, for
// looking up the required_capabilities a PlanningGap should report for the roles
// named by a violated constraint.
func roleCapabilitiesFromSpec(spec scenariotemplate.SpecV2) map[string][]string {
	capabilities := make(map[string][]string, len(spec.Roles))
	for _, role := range spec.Roles {
		capabilities[role.Key] = role.RequiredCapabilities
	}
	return capabilities
}

// buildRoleIndependenceGap constructs the PlanningGap for a role_independence
// structural escalation: RequiredCapabilities is the order-preserving union (no
// duplicates) of constraint.Roles' declared required_capabilities. Options is
// always the fixed three-way-out set — restaff (补充员工), exempt (改选更浅出口),
// — regardless of which constraint triggered the gap.
func buildRoleIndependenceGap(constraint scenariotemplate.SpecConstraint, roleCapabilities map[string][]string, activeExecutorCount int) PlanningGap {
	var capabilities []string
	seen := make(map[string]bool, len(constraint.Roles))
	for _, role := range constraint.Roles {
		for _, capability := range roleCapabilities[role] {
			if seen[capability] {
				continue
			}
			seen[capability] = true
			capabilities = append(capabilities, capability)
		}
	}
	return PlanningGap{
		ConstraintKind:       constraint.Kind,
		Roles:                append([]string(nil), constraint.Roles...),
		RequiredCapabilities: capabilities,
		ActiveExecutorCount:  activeExecutorCount,
		Options:              []string{"restaff", "exempt"},
	}
}

// newStructuralGapError builds the structuralGapError shared by
// enforceRoleIndependence and structuralGapForPlan: same fixed diagnosis message
// (noSuitableEmployeeStructuralGapMessage, prefixed exactly as the historical
// fmt.Errorf("%w: %s", ErrNoSuitableEmployee, ...) formatted it, so
// humanizeNoSuitableEmployeeDiagnosis in workflow.go keeps stripping the same
// prefix), carrying the constraint's PlanningGap.
func newStructuralGapError(constraint scenariotemplate.SpecConstraint, roleCapabilities map[string][]string, activeExecutorCount int) error {
	return newStructuralGapErrorWithMessage(constraint, roleCapabilities, activeExecutorCount, noSuitableEmployeeStructuralGapMessage)
}

func newStructuralGapErrorWithMessage(constraint scenariotemplate.SpecConstraint, roleCapabilities map[string][]string, activeExecutorCount int, message string) error {
	return &structuralGapError{
		msg: ErrNoSuitableEmployee.Error() + ": " + message,
		gap: buildRoleIndependenceGap(constraint, roleCapabilities, activeExecutorCount),
	}
}

// pruneSkeletonForExit delegates to scenariotemplate.PruneSkeletonForExit so
// playbook-readiness and planning governance share one implementation.
func pruneSkeletonForExit(spec scenariotemplate.SpecV2, exitDeliverable string) ([]scenariotemplate.SpecSkeletonStep, error) {
	return scenariotemplate.PruneSkeletonForExit(spec, exitDeliverable)
}

// validateSkeletonAdherence checks that plan's tasks collectively produce
// every deliverable required by the scenario template's skeleton, pruned to
// the steps reachable from plan.ExitDeliverable. A spec with no skeleton
// (generic/unbound fallback) is a no-op.
func validateSkeletonAdherence(spec scenariotemplate.SpecV2, plan RouteDecisionPlan) error {
	if len(spec.Skeleton) == 0 {
		return nil
	}
	if len(spec.Exits) > 0 && strings.TrimSpace(plan.ExitDeliverable) == "" {
		names := make([]string, 0, len(spec.Exits))
		for _, e := range spec.Exits {
			names = append(names, e.Deliverable)
		}
		return invalidRouteDecision("exit_deliverable required: template declares exits %v", names)
	}
	steps := spec.Skeleton
	if len(spec.Exits) > 0 {
		pruned, err := pruneSkeletonForExit(spec, plan.ExitDeliverable)
		if err != nil {
			return invalidRouteDecision("invalid exit_deliverable %q: %v", plan.ExitDeliverable, err)
		}
		steps = pruned
	}
	producedBy := map[string]string{} // produce name -> task key（全局唯一性已由 ValidateRouteDecisionGraph 保证）
	for _, task := range plan.Tasks {
		for _, p := range task.Produces {
			producedBy[p] = task.Key
		}
	}
	for _, step := range steps {
		for _, p := range step.ProducesDefaults {
			if _, ok := producedBy[p.Name]; !ok {
				return invalidRouteDecision("skeleton step %q deliverable %q missing from plan produces", step.Step, p.Name)
			}
		}
	}
	return nil
}

// exitCondMet delegates to scenariotemplate.ExitCondMet so playbook-readiness
// and planning governance share one implementation.
func exitCondMet(spec scenariotemplate.SpecV2, cond scenariotemplate.SpecConstraintWhen, exit string) bool {
	return scenariotemplate.ExitCondMet(spec, cond, exit)
}

// stepTask resolves the plan task that fulfils a skeleton step, via the
// step's first declared produces_defaults name. Steps with no
// produces_defaults, or whose declared output has no producing task in the
// plan, resolve to (nil, false).
func stepTask(step scenariotemplate.SpecSkeletonStep, producedBy map[string]string, taskByKey map[string]*PlannedTask) (*PlannedTask, bool) {
	if len(step.ProducesDefaults) == 0 {
		return nil, false
	}
	key, ok := producedBy[step.ProducesDefaults[0].Name]
	if !ok {
		return nil, false
	}
	task, ok := taskByKey[key]
	return task, ok
}

// enforceRoleIndependence checks every pair of roles named by a
// role_independence constraint for a shared employee. A shared employee is
// only ever a violation to reject-and-replan (invalidRouteDecision) when the
// project's active executor pool has at least 2 members — meaning a
// different assignment could in principle satisfy the constraint. When the
// pool cannot possibly make the two roles independent (fewer than 2 active
// executors), rejecting-and-replanning would just have the planner reselect
// the same sole employee forever; this is a structural pool gap, not a plan
// defect, so it escalates straight to a human via newStructuralGapError,
// carrying an actionable PlanningGap (roles/capabilities/ways-out) instead of
// a raw diagnosis.
//
// Playbook casting (编制) is assignment authority: ApplyPlaybookCasting re-forces
// role→employee on every plan. When both roles of a SoD pair are cast to the
// same employee, re-planning cannot repair the violation either — escalate to
// the same structural gap (restaff/exempt) so G12 does not burn planner retries
// and leave the demand in planning_pending for many minutes (design §7.3 / G12).
//
// This is the only production channel that raises ErrNoSuitableEmployee:
// ValidateRouteDecisionPlan no longer gates on planner self-reported confidence —
// see graph_validation.go's selectionScoreThreshold for the server-computed gate
// that replaced it (a low score degrades the plan via a low_feasibility note
// instead of rejecting it).
func enforceRoleIndependence(constraint scenariotemplate.SpecConstraint, roleEmployees map[string]map[uuid.UUID]bool, roleCapabilities map[string][]string, activeExecutorCount int, castByRole map[string]uuid.UUID) error {
	for i := 0; i < len(constraint.Roles); i++ {
		for j := i + 1; j < len(constraint.Roles); j++ {
			leftRole := constraint.Roles[i]
			rightRole := constraint.Roles[j]
			left := roleEmployees[leftRole]
			right := roleEmployees[rightRole]
			for id := range left {
				if !right[id] {
					continue
				}
				// Cast-locked SoD: both roles force the same employee → not a planner defect.
				if castByRole != nil {
					leftCast, leftOK := castByRole[leftRole]
					rightCast, rightOK := castByRole[rightRole]
					if leftOK && rightOK && leftCast == id && rightCast == id {
						return newStructuralGapErrorWithMessage(constraint, roleCapabilities, activeExecutorCount, noSuitableEmployeeCastLockedGapMessage)
					}
				}
				if activeExecutorCount >= 2 {
					return invalidRouteDecision("constraint role_independence violated: roles %v share employee %s", constraint.Roles, id)
				}
				return newStructuralGapError(constraint, roleCapabilities, activeExecutorCount)
			}
		}
	}
	return nil
}

// roleHasReviewCapability reports whether role declares at least one review-class
// required capability — any capability key CONTAINING the substring "review"
// (case-insensitive), e.g. code_review, design_review, peer_review.
//
// This is a HEURISTIC review signal, deliberately part of the migration
// discriminator (see reviewedStepForIndependence): a skeleton dependency edge alone
// does NOT mark a review relationship — a non-review separation-of-duties constraint
// (e.g. role_independence[approver, requester] where approve depends_on request)
// also has a dependency edge, yet migrating it would inject a spurious
// adversarial_review criterion AND silently disable the four-eyes
// enforceRoleIndependence enforcement, defeating separation of duties. Requiring the
// depending ("reviewer") role to carry a review-class capability keeps that
// migration scoped to genuine review pairs.
//
// TODO(governance): a future explicit constraint field/kind (e.g. a review_of
// pointer or a dedicated review_independence constraint kind) would be more robust
// than substring-matching a capability key; this heuristic is the interim signal
// until that lands.
func roleHasReviewCapability(role scenariotemplate.SpecRole) bool {
	for _, capability := range role.RequiredCapabilities {
		if strings.Contains(strings.ToLower(capability), "review") {
			return true
		}
	}
	return false
}

// reviewedStepForIndependence identifies the reviewed skeleton step of a
// role_independence constraint from the skeleton DEPENDENCY plus a REVIEW SIGNAL: of
// the two roles the constraint names, the one whose step depends_on the other role's
// step is the reviewer, and — for the pair to count as a genuine review relationship —
// that reviewer role must carry a review-class required capability (see
// roleHasReviewCapability); the depended-on (reviewed) role's step is what an
// adversarial review judges. It only resolves the classic two-role review pair — a
// constraint naming other than exactly two roles, two roles with no dependency edge
// between their steps (so reviewer/reviewed is undeterminable), or a depending role
// with NO review-class capability (a non-review separation-of-duties constraint)
// returns ok=false, and the caller must fall back to the original
// enforceRoleIndependence behavior. steps is the pruned skeleton, so only dependency
// edges reachable from the chosen exit count.
func reviewedStepForIndependence(constraint scenariotemplate.SpecConstraint, steps []scenariotemplate.SpecSkeletonStep, roleByKey map[string]scenariotemplate.SpecRole) (scenariotemplate.SpecSkeletonStep, bool) {
	if len(constraint.Roles) != 2 {
		return scenariotemplate.SpecSkeletonStep{}, false
	}
	stepByName := make(map[string]scenariotemplate.SpecSkeletonStep, len(steps))
	for _, s := range steps {
		stepByName[s.Step] = s
	}
	// reviewedFor returns the reviewed role's depended-on step when some step of
	// reviewerRole depends_on a step of reviewedRole AND reviewerRole carries a
	// review-class capability (so the dependency edge really is a review, not a
	// generic separation-of-duties ordering).
	reviewedFor := func(reviewerRole, reviewedRole string) (scenariotemplate.SpecSkeletonStep, bool) {
		if !roleHasReviewCapability(roleByKey[reviewerRole]) {
			return scenariotemplate.SpecSkeletonStep{}, false
		}
		for _, s := range steps {
			if s.Role != reviewerRole {
				continue
			}
			for _, dep := range s.DependsOn {
				d, ok := stepByName[dep]
				if ok && d.Role == reviewedRole {
					return d, true
				}
			}
		}
		return scenariotemplate.SpecSkeletonStep{}, false
	}
	if reviewed, ok := reviewedFor(constraint.Roles[0], constraint.Roles[1]); ok {
		return reviewed, true
	}
	if reviewed, ok := reviewedFor(constraint.Roles[1], constraint.Roles[0]); ok {
		return reviewed, true
	}
	return scenariotemplate.SpecSkeletonStep{}, false
}

// migrateReviewerRoleToAdversarial implements Part A of the adversarial-review
// rollout: a template's independent-review declaration (a role_independence
// constraint whose reviewer/reviewed relation is determinable from the skeleton
// dependency) is REPLACED by a blocking adversarial_review criterion on the
// reviewed task — N AI judges, independent by construction, stand in for the single
// human reviewer, so the four-eyes requirement is satisfied without demanding a
// distinct reviewer employee. Returns migrated=true when the constraint's
// independence is now provided by the judges (so the caller skips the
// shared-employee enforceRoleIndependence check); false when the relation could not
// be determined (caller keeps the original behavior).
//
// The injected criterion is self-consistently valid (verification_method=
// adversarial_review with a non-empty SatisfiedBy pointing at the reviewed task's
// planned key) because governance runs AFTER validateAcceptanceCriteriaSemantics —
// nothing re-validates what we inject here. Injection is idempotent: if an
// adversarial_review criterion already satisfied_by the reviewed task exists (a
// prior governance pass), we report migrated without appending a duplicate.
func migrateReviewerRoleToAdversarial(constraint scenariotemplate.SpecConstraint, steps []scenariotemplate.SpecSkeletonStep, producedBy map[string]string, taskByKey map[string]*PlannedTask, roleByKey map[string]scenariotemplate.SpecRole, plan *RouteDecisionPlan) bool {
	reviewedStep, ok := reviewedStepForIndependence(constraint, steps, roleByKey)
	if !ok {
		return false
	}
	reviewedTask, ok := stepTask(reviewedStep, producedBy, taskByKey)
	if !ok {
		// Reviewed step produces nothing in this plan: no task to hang the
		// adversarial criterion on. Leave the original constraint behavior intact.
		return false
	}
	for _, existing := range plan.PlanAcceptanceCriteria {
		if existing.VerificationMethod == VerificationMethodAdversarialReview && containsString(existing.SatisfiedBy, reviewedTask.Key) {
			return true // idempotent: already injected
		}
	}
	plan.PlanAcceptanceCriteria = append(plan.PlanAcceptanceCriteria, PlanAcceptanceCriterion{
		ID:                 "adversarial_review:" + reviewedTask.Key,
		Statement:          fmt.Sprintf("独立对抗评审：%s 的产出须经多判官证伪评审通过", reviewedStep.Step),
		SatisfiedBy:        []string{reviewedTask.Key},
		VerificationMethod: VerificationMethodAdversarialReview,
		Severity:           CriterionSeverityBlocking,
	})
	plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
		Kind:    "adversarial_review",
		Message: fmt.Sprintf("独立审查已迁为 adversarial_review（被审步骤 %s），独立性由判官组保证；不再要求独立 reviewer 员工", reviewedStep.Step),
	})
	return true
}

// enforceAutonomousStageExitEvidence implements Part B: an autonomous stage must
// not gate solely on opinion. A skeleton step is treated as autonomous when it is
// NOT the target of an in-force human_gate constraint — an approximation of a true
// per-stage checkpoint declaration (the Phase A Task 4 human_checkpoint field has
// not landed; when it does this should key on that instead of human_gate).
//
// For each autonomous step, its exit criteria are the plan acceptance criteria
// whose satisfied_by names the step's produced task. It is rejected (replan) only
// when the step HAS exit criteria and every one is human_judgment — a step with no
// exit criteria (a non-deliverable/uncriteriaed step) is not an error. This runs
// AFTER Part A's injection, so a reviewed step made non-opinion by an injected
// adversarial_review criterion passes.
func enforceAutonomousStageExitEvidence(spec scenariotemplate.SpecV2, steps []scenariotemplate.SpecSkeletonStep, producedBy map[string]string, taskByKey map[string]*PlannedTask, plan *RouteDecisionPlan) error {
	humanGated := map[string]bool{}
	for _, constraint := range spec.Constraints {
		if constraint.Kind == "human_gate" && exitCondMet(spec, constraint.When, plan.ExitDeliverable) {
			humanGated[constraint.Target] = true
		}
	}
	for _, step := range steps {
		if humanGated[step.Step] {
			continue // human-gated stage: a human at the gate is the evidence
		}
		task, ok := stepTask(step, producedBy, taskByKey)
		if !ok {
			continue
		}
		hasExitCriteria := false
		allOpinion := true
		for _, criterion := range plan.PlanAcceptanceCriteria {
			if !containsString(criterion.SatisfiedBy, task.Key) {
				continue
			}
			hasExitCriteria = true
			if criterion.VerificationMethod != VerificationMethodHumanJudgment {
				allOpinion = false
				break
			}
		}
		if hasExitCriteria && allOpinion {
			return invalidRouteDecision("autonomous_stage_opinion_only_exit: 无人自治阶段 %q（任务 %s）的出口判据不得全为意见类，需至少一条 automated_test 或 adversarial_review", step.Step, task.Key)
		}
	}
	return nil
}

// demandConstraintExempted reports whether snapshot's first-class exemptions
// (project_demand_constraint_exemptions, loaded per-demand by
// LoadProjectCoordinationSnapshot) cover the given role_independence constraint,
// and — when they do — the roles to name in the resulting exemption note. The
// match rule is deliberately simple: same ConstraintKind, and either the
// exemption carries no explicit Roles (a blanket exemption for that kind on this
// demand) or its Roles set is exactly the constraint's Roles set (order-
// independent; sorted-slice equality, no partial/superset matching).
func demandConstraintExempted(exemptions []DemandConstraintExemption, constraint scenariotemplate.SpecConstraint) (bool, []string) {
	for _, exemption := range exemptions {
		if exemption.ConstraintKind != constraint.Kind {
			continue
		}
		if len(exemption.Roles) == 0 || roleSetsEqual(exemption.Roles, constraint.Roles) {
			return true, constraint.Roles
		}
	}
	return false, nil
}

// roleSetsEqual reports whether a and b contain the same role keys, ignoring
// order (each is sorted into a fresh copy before comparing; neither input slice
// is mutated).
func roleSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	sortedA := append([]string(nil), a...)
	sortedB := append([]string(nil), b...)
	sort.Strings(sortedA)
	sort.Strings(sortedB)
	for i := range sortedA {
		if sortedA[i] != sortedB[i] {
			return false
		}
	}
	return true
}

// playbookCastByRole indexes PlaybookCasting by role_key → employee.
func playbookCastByRole(casts []PlaybookCastingAssignment) map[string]uuid.UUID {
	if len(casts) == 0 {
		return nil
	}
	out := make(map[string]uuid.UUID, len(casts))
	for _, c := range casts {
		role := strings.TrimSpace(c.RoleKey)
		if role == "" || c.DigitalEmployeeID == uuid.Nil {
			continue
		}
		out[role] = c.DigitalEmployeeID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// structuralGapForPlan returns the actionable ErrNoSuitableEmployee-family error
// (noSuitableEmployeeStructuralGapMessage) when a plan's failure is really a pool
// structural gap the planner cannot re-plan around, else nil. The gap holds when:
// the project is bound to a scenario template with a skeleton, the plan's chosen
// exit keeps a role_independence constraint in force, and the active executor pool
// has fewer than 2 members (so no assignment could ever make the two roles
// independent).
//
// It is called when ValidateRouteDecisionPlan has already returned an
// ErrNoSuitableEmployee error (an honest low-confidence selection for the review
// role): both are the same terminal family, but this message tells the human how to
// get out (补充员工/改浅出口/换模板) instead of surfacing a raw "scored 0.30".
func structuralGapForPlan(snapshot CoordinationSnapshot, plan RouteDecisionPlan) error {
	if snapshot.ScenarioTemplate == nil {
		return nil
	}
	spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
	if err != nil || len(spec.Skeleton) == 0 {
		return nil
	}
	activeExecutorCount := len(activeExecutorIDs(snapshot.DigitalEmployeePool))
	if activeExecutorCount >= 2 {
		return nil
	}
	roleCapabilities := roleCapabilitiesFromSpec(spec)
	for _, constraint := range spec.Constraints {
		if constraint.Kind != "role_independence" || len(constraint.Roles) < 2 {
			continue
		}
		if !exitCondMet(spec, constraint.When, plan.ExitDeliverable) {
			continue
		}
		return newStructuralGapError(constraint, roleCapabilities, activeExecutorCount)
	}
	return nil
}

// EnforceScenarioTemplateGovernance applies a bound scenario template's
// constraints and collapse rules to a plan that has already passed
// ValidateRouteDecisionPlan. It must be called immediately after every
// production ValidateRouteDecisionPlan call succeeds.
//
//   - A constraint violation is a plan defect: returns an error wrapping
//     ErrInvalidRouteDecision (or, for a role_independence violation that no
//     re-plan could fix, ErrNoSuitableEmployee — see enforceRoleIndependence).
//   - human_gate forces the target step's task RequiresHumanApproval=true and
//     appends a ConstraintNote.
//   - A collapse_rules hit is server-detected (not planner-self-reported —
//     stronger than requiring the planner to declare it, and cannot be
//     omitted by an uncooperative or careless planner output) and sets
//     plan.RequiresHumanReview, with a ConstraintNote naming both role titles.
//   - plan.TemplateVersion and plan.AvailableExits are always populated from
//     the bound template, for payload/human-review context — this is why the
//     function takes a pointer.
//
// A nil ScenarioTemplate (unbound/generic project) or an empty spec.Skeleton
// (generic fallback spec) is a no-op beyond populating TemplateVersion/
// AvailableExits, matching validateSkeletonAdherence's own generic-noop rule.
// The function is mode-agnostic: it applies identically to plan-mode and
// loop-mode plans, since neither RouteDecisionPlan nor CoordinationSnapshot
// carries a "mode" the evaluator branches on.
func EnforceScenarioTemplateGovernance(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) error {
	if plan == nil {
		return nil
	}
	unbound := snapshot.ScenarioTemplate == nil
	if unbound {
		// Unbound/generic demand: there is no template, so any template lineage
		// the planner echoed back is hallucinated and must not reach the
		// confirmation card. Strip the binding markers — exit_deliverable,
		// available_exits, constraint_notes, template_version — before this
		// function's own low-feasibility notes (below) are appended.
		// TemplateKey is deliberately left intact: it is a pre-existing planner
		// label consumed by plan fingerprints, not a template-binding marker.
		plan.ExitDeliverable = ""
		plan.AvailableExits = nil
		plan.ConstraintNotes = nil
		plan.TemplateVersion = 0
	}

	// Feasibility degrade is template-independent: even an unbound/generic
	// plan can carry a task whose server-computed SelectionScore is low, and a
	// human still needs to see that. Runs before the unbound early-return so
	// unbound plans get the note too.
	appendLowFeasibilityNotes(snapshot, plan)

	if unbound {
		return nil
	}
	spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
	if err != nil {
		return invalidRouteDecision("scenario template spec unparsable: %v", err)
	}

	plan.TemplateVersion = snapshot.ScenarioTemplate.Version
	plan.AvailableExits = make([]PlanExitOption, 0, len(spec.Exits))
	for _, exit := range spec.Exits {
		plan.AvailableExits = append(plan.AvailableExits, PlanExitOption{Deliverable: exit.Deliverable, Label: exit.Label})
	}

	if len(spec.Skeleton) == 0 {
		return nil
	}

	steps := spec.Skeleton
	if len(spec.Exits) > 0 {
		pruned, err := pruneSkeletonForExit(spec, plan.ExitDeliverable)
		if err != nil {
			return invalidRouteDecision("invalid exit_deliverable %q: %v", plan.ExitDeliverable, err)
		}
		steps = pruned
	}
	prunedSet := make(map[string]scenariotemplate.SpecSkeletonStep, len(steps))
	for _, s := range steps {
		prunedSet[s.Step] = s
	}

	producedBy := map[string]string{}
	taskByKey := make(map[string]*PlannedTask, len(plan.Tasks))
	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		taskByKey[task.Key] = task
		for _, p := range task.Produces {
			producedBy[p] = task.Key
		}
	}

	roleTitle := make(map[string]string, len(spec.Roles))
	roleByKey := make(map[string]scenariotemplate.SpecRole, len(spec.Roles))
	for _, role := range spec.Roles {
		roleTitle[role.Key] = role.Title
		roleByKey[role.Key] = role
	}
	roleCapabilities := roleCapabilitiesFromSpec(spec)

	// A role may span multiple steps within the pruned skeleton (e.g.
	// "developer" both develops and releases); union all employees observed
	// for each role.
	roleEmployees := map[string]map[uuid.UUID]bool{}
	for _, step := range steps {
		task, ok := stepTask(step, producedBy, taskByKey)
		if !ok {
			continue
		}
		if roleEmployees[step.Role] == nil {
			roleEmployees[step.Role] = map[uuid.UUID]bool{}
		}
		roleEmployees[step.Role][task.SelectedEmployeeID] = true
	}

	activeExecutorCount := len(activeExecutorIDs(snapshot.DigitalEmployeePool))
	castByRole := playbookCastByRole(snapshot.PlaybookCasting)

	for _, constraint := range spec.Constraints {
		if !exitCondMet(spec, constraint.When, plan.ExitDeliverable) {
			continue
		}
		switch constraint.Kind {
		case "role_independence":
			if exempted, roles := demandConstraintExempted(snapshot.DemandConstraintExemptions, constraint); exempted {
				plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
					Kind:    "exemption",
					Message: fmt.Sprintf("约束 role_independence 已由人类负责人豁免（角色：%s）", strings.Join(roles, "、")),
				})
				continue
			}
			// Part A: 模板的独立审查声明 → 被审任务上的 adversarial_review 判据（AI
			// 判官替换单一 reviewer 员工）。迁移成功即视为判官组天然提供四眼独立，
			// 不再因 reviewer/developer 共享员工 reject。关系不可判（无依赖边）的约束
			// 保持原 enforceRoleIndependence 行为。
			if migrateReviewerRoleToAdversarial(constraint, steps, producedBy, taskByKey, roleByKey, plan) {
				continue
			}
			if err := enforceRoleIndependence(constraint, roleEmployees, roleCapabilities, activeExecutorCount, castByRole); err != nil {
				return err
			}
		case "stage_required":
			step, ok := prunedSet[constraint.Step]
			if !ok {
				return invalidRouteDecision("constraint stage_required violated: step %q is not reachable from exit %q", constraint.Step, plan.ExitDeliverable)
			}
			if _, ok := stepTask(step, producedBy, taskByKey); !ok {
				return invalidRouteDecision("constraint stage_required violated: step %q has no corresponding task in the plan", constraint.Step)
			}
		case "human_gate":
			step, ok := prunedSet[constraint.Target]
			if !ok {
				return invalidRouteDecision("constraint human_gate violated: target step %q is not reachable from exit %q", constraint.Target, plan.ExitDeliverable)
			}
			task, ok := stepTask(step, producedBy, taskByKey)
			if !ok {
				return invalidRouteDecision("constraint human_gate violated: target step %q has no corresponding task in the plan", constraint.Target)
			}
			task.RequiresHumanApproval = true
			plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
				Kind:    "human_gate",
				Message: fmt.Sprintf("发布任务已强制人类审批：由 human_gate@%s v%d 触发", snapshot.ScenarioTemplate.Key, snapshot.ScenarioTemplate.Version),
			})
		}
	}

	// Part B: after Part A's adversarial injection, reject any autonomous stage
	// whose exit gates on opinion alone. Runs after the constraint loop so the
	// injected adversarial_review criteria count toward "non-opinion exit".
	if err := enforceAutonomousStageExitEvidence(spec, steps, producedBy, taskByKey, plan); err != nil {
		return err
	}

	// Collapse annotation is server-generated, unconditionally, from the
	// plan's actual employee assignments — see the doc comment above for why
	// this is deliberately stronger than requiring the planner to self-report
	// a collapse.
	for _, rule := range spec.CollapseRules {
		if len(rule.Roles) != 2 {
			continue
		}
		left, right := roleEmployees[rule.Roles[0]], roleEmployees[rule.Roles[1]]
		shared := false
		for id := range left {
			if right[id] {
				shared = true
				break
			}
		}
		if !shared {
			continue
		}
		plan.RequiresHumanReview = true
		plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
			Kind:    "collapse",
			Message: fmt.Sprintf("角色折叠：%s 与 %s 由同一员工承担，已自动标注待人工复核", roleTitleOrKey(roleTitle, rule.Roles[0]), roleTitleOrKey(roleTitle, rule.Roles[1])),
		})
	}

	return nil
}

// appendLowFeasibilityNotes flags every task whose server-computed
// SelectionScore falls below the coordination-policy threshold
// (selectionScoreThreshold): a low_feasibility ConstraintNote naming the
// score, the threshold, and (when present) the missing capabilities, plus
// plan.RequiresHumanReview=true. This is a degrade, not a rejection — the
// plan still proceeds; a human reviews the honest, server-verified fact
// instead of the plan being rejected-and-replanned (which would just cause
// the planner to reselect the same weak candidate again). Called from
// EnforceScenarioTemplateGovernance for both templated and unbound plans.
func appendLowFeasibilityNotes(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) {
	threshold := selectionScoreThreshold(snapshot.CoordinationPolicy)
	for i := range plan.Tasks {
		task := &plan.Tasks[i]
		if float64(task.SelectionScore) >= threshold {
			continue
		}
		message := fmt.Sprintf("任务 %s 选角事实性评分 %d 低于阈值 %d", task.Key, task.SelectionScore, int(threshold))
		if len(task.MissingCapabilities) > 0 {
			message += fmt.Sprintf("：缺失 %s", strings.Join(task.MissingCapabilities, "、"))
		}
		plan.ConstraintNotes = append(plan.ConstraintNotes, PlanConstraintNote{
			Kind:    "low_feasibility",
			Message: message,
		})
		plan.RequiresHumanReview = true
	}
}

func roleTitleOrKey(titles map[string]string, key string) string {
	if title, ok := titles[key]; ok && strings.TrimSpace(title) != "" {
		return title
	}
	return key
}
