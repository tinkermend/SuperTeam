package projectcoordination

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// softwareDeliveryV2Literal is the v2 spec literal for the software_delivery
// scenario template, byte-identical to migration 061 and to
// internal/scenariotemplate/spec_test.go's constant of the same name.
const softwareDeliveryV2Literal = `{"spec_version":2,"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"branch_ref","kind":"branch_ref"},{"name":"head_commit","kind":"git_commit"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]},{"step":"release","role":"developer","depends_on":["review","test"],"required_inputs_defaults":["review_verdict","test_report"],"produces_defaults":[{"name":"release_record","kind":"evidence_ref"}]}],"exits":[{"deliverable":"branch_ref","label":"交付分支（不合入）"},{"deliverable":"review_verdict","label":"审查通过并合入"},{"deliverable":"release_record","label":"发布上线"}],"constraints":[{"kind":"role_independence","roles":["reviewer","developer"],"when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"review","when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"test","when":{"exit_at_or_beyond":"release_record"}},{"kind":"human_gate","target":"release","when":{"exit_at_or_beyond":"release_record"}}],"collapse_rules":[{"roles":["developer","tester"]}],"default_acceptance_criteria":[{"statement":"变更以 branch+commit 交付","applies_from_exit":"branch_ref"},{"statement":"通过独立审查","applies_from_exit":"review_verdict"},{"statement":"测试报告覆盖主路径且结论可判","applies_from_exit":"release_record"}],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}`

func softwareDeliveryRawSpec(t *testing.T) map[string]any {
	t.Helper()
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(softwareDeliveryV2Literal), &raw))
	return raw
}

func softwareDeliveryV2Spec(t *testing.T) scenariotemplate.SpecV2 {
	t.Helper()
	spec, err := scenariotemplate.ParseSpec(softwareDeliveryRawSpec(t))
	require.NoError(t, err)
	return spec
}

func softwareDeliveryTemplateSnapshot(t *testing.T) *ScenarioTemplateSnapshot {
	t.Helper()
	return &ScenarioTemplateSnapshot{
		Key:     "software_delivery",
		Name:    "软件交付",
		Version: 2,
		Spec:    softwareDeliveryRawSpec(t),
	}
}

func activeExecutorPool(ids ...uuid.UUID) []ProjectMemberSnapshot {
	pool := make([]ProjectMemberSnapshot, 0, len(ids))
	for _, id := range ids {
		pool = append(pool, ProjectMemberSnapshot{PrincipalID: id, ProjectRole: "executor", Status: "active"})
	}
	return pool
}

func findTaskByKey(tasks []PlannedTask, key string) *PlannedTask {
	for i := range tasks {
		if tasks[i].Key == key {
			return &tasks[i]
		}
	}
	return nil
}

func constraintNoteWithKind(notes []PlanConstraintNote, kind string) *PlanConstraintNote {
	for i := range notes {
		if notes[i].Kind == kind {
			return &notes[i]
		}
	}
	return nil
}

func TestPruneSkeletonForExit(t *testing.T) {
	spec := softwareDeliveryV2Spec(t)

	branchOnly, err := pruneSkeletonForExit(spec, "branch_ref")
	require.NoError(t, err)
	require.Len(t, branchOnly, 1)
	require.Equal(t, "develop", branchOnly[0].Step)

	throughReview, err := pruneSkeletonForExit(spec, "review_verdict")
	require.NoError(t, err)
	require.Len(t, throughReview, 2)
	steps := []string{throughReview[0].Step, throughReview[1].Step}
	require.ElementsMatch(t, []string{"develop", "review"}, steps)

	all, err := pruneSkeletonForExit(spec, "release_record")
	require.NoError(t, err)
	require.Len(t, all, 4)

	_, err = pruneSkeletonForExit(spec, "not_a_real_exit")
	require.Error(t, err)
}

func TestValidateSkeletonAdherenceMissingStep(t *testing.T) {
	spec := softwareDeliveryV2Spec(t)
	plan := RouteDecisionPlan{
		Reason:          "valid",
		ExitDeliverable: "review_verdict",
		Tasks: []PlannedTask{
			planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil),
		},
	}

	err := validateSkeletonAdherence(spec, plan)

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), `skeleton step "review"`)
}

func TestValidateSkeletonAdherencePasses(t *testing.T) {
	spec := softwareDeliveryV2Spec(t)
	plan := RouteDecisionPlan{
		Reason:          "valid",
		ExitDeliverable: "review_verdict",
		Tasks: []PlannedTask{
			planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil),
			planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"}),
		},
	}

	err := validateSkeletonAdherence(spec, plan)

	require.NoError(t, err)
}

func TestValidateSkeletonAdherenceRequiresExit(t *testing.T) {
	spec := softwareDeliveryV2Spec(t)
	plan := validGraphPlan(uuid.New())
	plan.ExitDeliverable = ""

	err := validateSkeletonAdherence(spec, plan)

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "exit_deliverable required")
}

func TestValidateSkeletonAdherenceGenericNoop(t *testing.T) {
	var spec scenariotemplate.SpecV2
	plan := validGraphPlan(uuid.New())

	err := validateSkeletonAdherence(spec, plan)

	require.NoError(t, err)
}

// --- EnforceScenarioTemplateGovernance ---

func TestGovernanceRoleIndependenceViolation(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "role independence violation",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "role_independence")
	require.Contains(t, err.Error(), "reviewer")
	require.Contains(t, err.Error(), "developer")
}

// TestGovernanceSkipsExemptedRoleIndependence proves a DemandConstraintExemption
// carried on the snapshot (a human negotiator's first-class豁免决策, loaded from
// project_demand_constraint_exemptions) causes the matching role_independence
// constraint to be skipped instead of rejected — even though the same
// employeeA-fills-both-roles setup would otherwise violate it (see
// TestGovernanceRoleIndependenceViolation) — and that the skip is made visible via
// an "exemption" PlanConstraintNote.
func TestGovernanceSkipsExemptedRoleIndependence(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "role independence exempted",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
		DemandConstraintExemptions: []DemandConstraintExemption{
			{ConstraintKind: "role_independence", Roles: []string{"reviewer", "developer"}},
		},
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	note := constraintNoteWithKind(plan.ConstraintNotes, "exemption")
	require.NotNil(t, note, "expected an exemption constraint note, got %#v", plan.ConstraintNotes)
	require.Contains(t, note.Message, "role_independence")
}

// TestGovernanceExemptionScopedByKind proves an exemption only covers its own
// recorded ConstraintKind: an exemption granted for a different constraint kind
// (e.g. stage_required) must not suppress a role_independence violation on the
// same roles — the demand-scoped exemption table is per constraint_kind, not a
// blanket "ignore this demand's governance" switch.
func TestGovernanceExemptionScopedByKind(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "role independence violation despite unrelated exemption",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
		DemandConstraintExemptions: []DemandConstraintExemption{
			{ConstraintKind: "stage_required", Roles: []string{"reviewer", "developer"}},
		},
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "role_independence")
	require.Nil(t, constraintNoteWithKind(plan.ConstraintNotes, "exemption"))
}

func TestGovernanceRoleIndependencePassesWithTwoEmployees(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeB
	plan := RouteDecisionPlan{
		Reason:          "role independence satisfied",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
}

func TestGovernanceRoleIndependenceNotTriggeredBelowExit(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "below exit threshold",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "branch_ref",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
}

// TestGovernanceRoleIndependenceStructuralGapEscalates: with only one active
// executor in the pool, the reviewer/developer overlap can never be resolved
// by re-planning — replanning would just reselect the same sole employee for
// both roles forever. This must escalate through the ErrNoSuitableEmployee
// family (terminate to human), not ErrInvalidRouteDecision (which would spin
// the project back into a doomed re-plan loop).
func TestGovernanceRoleIndependenceStructuralGapEscalates(t *testing.T) {
	employeeA := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "structural gap",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA),
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.ErrorIs(t, err, ErrNoSuitableEmployee)
	require.NotErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "补充员工")
}

// structuralGapForPlan preference (fix: 结构缺口出路提示优先)
//
// When ValidateRouteDecisionPlan returns a low-confidence ErrNoSuitableEmployee
// (the planner honestly scored the sole employee low for the review role), the
// raw score text ("scored 0.30") is useless to a human. structuralGapForPlan must
// detect that this is really a pool structural gap (template bound, chosen exit
// keeps role_independence in force, <2 active executors) and prefer the actionable
// message carrying the ways-out hints (补充员工/改浅出口/换模板).

func TestStructuralGapForPlanReturnsActionableMessageForSingleExecutorMerge(t *testing.T) {
	employeeA := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "single executor merge",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{develop, review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA),
	}

	err := structuralGapForPlan(snapshot, plan)

	require.ErrorIs(t, err, ErrNoSuitableEmployee)
	require.NotErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "补充员工")
	require.Contains(t, err.Error(), "改选更浅出口")
	require.Contains(t, err.Error(), "换用模板")
}

func TestStructuralGapForPlanNilWhenTwoExecutors(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "two executors available",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks:           []PlannedTask{review},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
	}

	require.NoError(t, structuralGapForPlan(snapshot, plan))
}

func TestStructuralGapForPlanNilBelowIndependenceExit(t *testing.T) {
	employeeA := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "branch only, no review independence in force",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "branch_ref",
		Tasks:           []PlannedTask{develop},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA),
	}

	require.NoError(t, structuralGapForPlan(snapshot, plan))
}

func TestStructuralGapForPlanNilWhenTemplateUnbound(t *testing.T) {
	plan := validGraphPlan(uuid.New())
	snapshot := CoordinationSnapshot{DigitalEmployeePool: activeExecutorPool(uuid.New())}

	require.NoError(t, structuralGapForPlan(snapshot, plan))
}

func TestGovernanceHumanGateForcesApproval(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeB
	test := planTaskWithIO("test", []string{"develop"}, []string{"test_report"}, []string{"branch_ref"})
	test.SelectedEmployeeID = employeeB
	release := planTaskWithIO("release", []string{"review", "test"}, []string{"release_record"}, []string{"review_verdict", "test_report"})
	release.SelectedEmployeeID = employeeA
	release.RequiresHumanApproval = false
	plan := RouteDecisionPlan{
		Reason:          "full chain to release",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "release_record",
		Tasks:           []PlannedTask{develop, review, test, release},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	releaseTask := findTaskByKey(plan.Tasks, "release")
	require.NotNil(t, releaseTask)
	require.True(t, releaseTask.RequiresHumanApproval)
	note := constraintNoteWithKind(plan.ConstraintNotes, "human_gate")
	require.NotNil(t, note)
	require.Equal(t, "发布任务已强制人类审批：由 human_gate@software_delivery v2 触发", note.Message)
}

func TestGovernanceCollapseNoteAnnotates(t *testing.T) {
	employeeA := uuid.New()
	employeeB := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	review := planTaskWithIO("review", []string{"develop"}, []string{"review_verdict"}, []string{"head_commit"})
	review.SelectedEmployeeID = employeeB
	test := planTaskWithIO("test", []string{"develop"}, []string{"test_report"}, []string{"branch_ref"})
	test.SelectedEmployeeID = employeeA
	release := planTaskWithIO("release", []string{"review", "test"}, []string{"release_record"}, []string{"review_verdict", "test_report"})
	release.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "developer/tester collapse",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "release_record",
		Tasks:           []PlannedTask{develop, review, test, release},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA, employeeB),
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	require.True(t, plan.RequiresHumanReview)
	note := constraintNoteWithKind(plan.ConstraintNotes, "collapse")
	require.NotNil(t, note)
	require.Contains(t, note.Message, "开发")
	require.Contains(t, note.Message, "测试")
}

func TestGovernancePopulatesVersionAndExits(t *testing.T) {
	employeeA := uuid.New()
	develop := planTaskWithIO("develop", nil, []string{"branch_ref", "head_commit"}, nil)
	develop.SelectedEmployeeID = employeeA
	plan := RouteDecisionPlan{
		Reason:          "populate metadata",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "branch_ref",
		Tasks:           []PlannedTask{develop},
	}
	snapshot := CoordinationSnapshot{
		ScenarioTemplate:    softwareDeliveryTemplateSnapshot(t),
		DigitalEmployeePool: activeExecutorPool(employeeA),
	}
	spec := softwareDeliveryV2Spec(t)

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	require.Equal(t, snapshot.ScenarioTemplate.Version, plan.TemplateVersion)
	require.Len(t, plan.AvailableExits, len(spec.Exits))
	for i, exit := range spec.Exits {
		require.Equal(t, exit.Deliverable, plan.AvailableExits[i].Deliverable)
		require.Equal(t, exit.Label, plan.AvailableExits[i].Label)
	}
}

// TestGovernanceUnboundStripsHallucinatedTemplateLineage: an unbound/generic
// demand (nil ScenarioTemplate) has no template, so any template lineage the
// planner echoed back — exit_deliverable, available_exits, constraint_notes,
// template_version — is hallucinated and must be stripped before the payload
// reaches the confirmation card. TemplateKey is deliberately left intact: it is
// a pre-existing planner label consumed by plan fingerprints, not a binding
// marker.
func TestGovernanceUnboundStripsHallucinatedTemplateLineage(t *testing.T) {
	develop := planTaskWithIO("analyze", nil, []string{"risk_report"}, nil)
	develop.SelectedEmployeeID = uuid.New()
	// High enough to stay clear of the low_feasibility degrade note (Task 3 of
	// scenario-template P2b) — this test is about stripping hallucinated
	// template lineage, not about the score gate.
	develop.SelectionScore = 100
	plan := RouteDecisionPlan{
		Reason:          "unbound demand with hallucinated lineage",
		TemplateKey:     "tech_risk_analysis",
		TemplateVersion: 7,
		ExitDeliverable: "risk_report",
		AvailableExits:  []PlanExitOption{{Deliverable: "risk_report", Label: "风险报告"}},
		ConstraintNotes: []PlanConstraintNote{{Kind: "human_gate", Message: "幻觉约束"}},
		Tasks:           []PlannedTask{develop},
	}
	snapshot := CoordinationSnapshot{DigitalEmployeePool: activeExecutorPool(uuid.New())}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	require.Equal(t, "", plan.ExitDeliverable)
	require.Empty(t, plan.AvailableExits)
	require.Empty(t, plan.ConstraintNotes)
	require.Equal(t, 0, plan.TemplateVersion)
	// TemplateKey is a pre-existing planner label used in fingerprints; keep as-is.
	require.Equal(t, "tech_risk_analysis", plan.TemplateKey)
}

// --- Low-feasibility score degrade (Task 3 of scenario-template P2b: 事实性可行性三档) ---

// TestGovernanceLowFeasibilityScoreAddsDegradeNote: a task whose
// server-computed SelectionScore is below the default threshold (40) gets a
// low_feasibility ConstraintNote naming the score, threshold, and missing
// capabilities, and forces plan.RequiresHumanReview — without rejecting the
// plan. Runs on an unbound plan to prove the degrade is template-independent.
func TestGovernanceLowFeasibilityScoreAddsDegradeNote(t *testing.T) {
	employeeID := uuid.New()
	task := planTaskWithIO("analyze", nil, []string{"risk_report"}, nil)
	task.SelectedEmployeeID = employeeID
	task.SelectionScore = 25
	task.MissingCapabilities = []string{"database.write"}
	plan := RouteDecisionPlan{
		Reason: "low feasibility",
		Tasks:  []PlannedTask{task},
	}
	snapshot := CoordinationSnapshot{DigitalEmployeePool: activeExecutorPool(employeeID)}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	require.True(t, plan.RequiresHumanReview)
	note := constraintNoteWithKind(plan.ConstraintNotes, "low_feasibility")
	require.NotNil(t, note)
	require.Contains(t, note.Message, "analyze")
	require.Contains(t, note.Message, "25")
	require.Contains(t, note.Message, "40")
	require.Contains(t, note.Message, "database.write")
}

// TestGovernanceHighScoreNoNote: a task whose SelectionScore clears the
// threshold gets no low_feasibility note and RequiresHumanReview stays false.
func TestGovernanceHighScoreNoNote(t *testing.T) {
	employeeID := uuid.New()
	task := planTaskWithIO("analyze", nil, []string{"risk_report"}, nil)
	task.SelectedEmployeeID = employeeID
	task.SelectionScore = 85
	plan := RouteDecisionPlan{
		Reason: "high feasibility",
		Tasks:  []PlannedTask{task},
	}
	snapshot := CoordinationSnapshot{DigitalEmployeePool: activeExecutorPool(employeeID)}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	require.False(t, plan.RequiresHumanReview)
	require.Nil(t, constraintNoteWithKind(plan.ConstraintNotes, "low_feasibility"))
}

// TestSelectionScoreThresholdPolicyOverride: a project's coordination_policy
// can raise the score floor above the 40-point default, so a task that would
// otherwise clear the default (55) still degrades under the stricter (60)
// project policy.
func TestSelectionScoreThresholdPolicyOverride(t *testing.T) {
	employeeID := uuid.New()
	task := planTaskWithIO("analyze", nil, []string{"risk_report"}, nil)
	task.SelectedEmployeeID = employeeID
	task.SelectionScore = 55
	plan := RouteDecisionPlan{
		Reason: "policy override",
		Tasks:  []PlannedTask{task},
	}
	snapshot := CoordinationSnapshot{
		DigitalEmployeePool: activeExecutorPool(employeeID),
		CoordinationPolicy:  map[string]any{"selection_score_threshold": 60.0},
	}

	err := EnforceScenarioTemplateGovernance(snapshot, &plan)

	require.NoError(t, err)
	require.True(t, plan.RequiresHumanReview)
	note := constraintNoteWithKind(plan.ConstraintNotes, "low_feasibility")
	require.NotNil(t, note)
	require.Contains(t, note.Message, "55")
	require.Contains(t, note.Message, "60")
}

// --- ValidateRouteDecisionPlan integration coverage (template-governance-adjacent) ---

func TestValidateRouteDecisionPlanRejectsPinnedExitMismatch(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	snapshot.ScenarioTemplate = softwareDeliveryTemplateSnapshot(t)
	snapshot.PinnedExitDeliverable = "release_record"
	plan := RouteDecisionPlan{
		Reason:          "pin mismatch",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "branch_ref",
		Tasks: []PlannedTask{{
			Key: "develop", Title: "Develop", Summary: "Develop branch",
			SelectedEmployeeID: employeeID, EmployeeSelectionReason: "only executor in pool", SelectionConfidence: 0.9,
			ExpectedOutputs:   []string{"branch"},
			Produces:          []string{"branch_ref", "head_commit"},
			InputRequirements: map[string]any{}, HandoffContract: map[string]any{},
		}},
	}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "human-pinned exit")
}

func TestValidateRouteDecisionPlanRejectsUnparsableTemplateSpec(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	snapshot.ScenarioTemplate = &ScenarioTemplateSnapshot{
		Key: "broken_template",
		Spec: map[string]any{
			"spec_version": float64(2),
			"constraints":  []any{map[string]any{"kind": "made_up"}},
		},
	}
	plan := validGraphPlan(employeeID)
	plan.TemplateKey = "broken_template"

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), "unparsable")
}

func TestValidateRouteDecisionPlanRejectsSkeletonNonConformance(t *testing.T) {
	employeeID := uuid.New()
	snapshot := validationSnapshotWithProfile(employeeID)
	snapshot.ScenarioTemplate = softwareDeliveryTemplateSnapshot(t)
	plan := RouteDecisionPlan{
		Reason:          "missing review step",
		TemplateKey:     "software_delivery",
		ExitDeliverable: "review_verdict",
		Tasks: []PlannedTask{{
			Key: "develop", Title: "Develop", Summary: "Develop branch",
			SelectedEmployeeID: employeeID, EmployeeSelectionReason: "only executor in pool", SelectionConfidence: 0.9,
			ExpectedOutputs:   []string{"branch"},
			Produces:          []string{"branch_ref", "head_commit"},
			InputRequirements: map[string]any{}, HandoffContract: map[string]any{},
		}},
	}

	err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 10})

	require.ErrorIs(t, err, ErrInvalidRouteDecision)
	require.Contains(t, err.Error(), `skeleton step "review"`)
}
