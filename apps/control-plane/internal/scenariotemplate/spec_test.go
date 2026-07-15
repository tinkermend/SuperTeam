package scenariotemplate

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// softwareDeliveryV2Literal is the v2 spec literal for the software_delivery
// seed template, copied verbatim from migration 061
// (scenario_template_versions_and_demand_binding.sql).
const softwareDeliveryV2Literal = `{"spec_version":2,"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"branch_ref","kind":"branch_ref"},{"name":"head_commit","kind":"git_commit"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]},{"step":"release","role":"developer","depends_on":["review","test"],"required_inputs_defaults":["review_verdict","test_report"],"produces_defaults":[{"name":"release_record","kind":"evidence_ref"}]}],"exits":[{"deliverable":"branch_ref","label":"交付分支（不合入）"},{"deliverable":"review_verdict","label":"审查通过并合入"},{"deliverable":"release_record","label":"发布上线"}],"constraints":[{"kind":"role_independence","roles":["reviewer","developer"],"when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"review","when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"test","when":{"exit_at_or_beyond":"release_record"}},{"kind":"human_gate","target":"release","when":{"exit_at_or_beyond":"release_record"}}],"collapse_rules":[{"roles":["developer","tester"]}],"default_acceptance_criteria":[{"statement":"变更以 branch+commit 交付","applies_from_exit":"branch_ref"},{"statement":"通过独立审查","applies_from_exit":"review_verdict"},{"statement":"测试报告覆盖主路径且结论可判","applies_from_exit":"release_record"}],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}`

// softwareDeliveryV1Literal is the v1 spec literal for the software_delivery
// seed template, copied verbatim from migration 058 (scenario_templates.sql).
const softwareDeliveryV1Literal = `{"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"],"collapsible_with":["tester"],"independent_from":[]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"],"collapsible_with":[],"independent_from":["developer"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"],"collapsible_with":["developer"],"independent_from":[]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"head_commit","kind":"git_commit"},{"name":"branch_ref","kind":"branch_ref"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]}],"default_acceptance_criteria":["变更以 branch+commit 交付且通过独立审查","测试报告覆盖主路径且结论可判"],"risk_policy":{"release_requires_human":true},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}`

func literalToMap(t *testing.T, literal string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(literal), &m); err != nil {
		t.Fatalf("unmarshal literal: %v", err)
	}
	return m
}

func TestParseSpecV2(t *testing.T) {
	raw := literalToMap(t, softwareDeliveryV2Literal)

	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec returned error: %v", err)
	}

	if spec.SpecVersion != 2 {
		t.Fatalf("expected SpecVersion 2, got %d", spec.SpecVersion)
	}
	if len(spec.Skeleton) != 4 {
		t.Fatalf("expected 4 skeleton steps, got %d", len(spec.Skeleton))
	}
	if len(spec.Exits) != 3 {
		t.Fatalf("expected 3 exits, got %d", len(spec.Exits))
	}
	if len(spec.Constraints) != 4 {
		t.Fatalf("expected 4 constraints, got %d", len(spec.Constraints))
	}
	if idx := spec.ExitIndex("review_verdict"); idx != 1 {
		t.Fatalf("expected ExitIndex(review_verdict) == 1, got %d", idx)
	}
	step, ok := spec.StepByProduce("release_record")
	if !ok {
		t.Fatalf("expected StepByProduce(release_record) to be found")
	}
	if step.Step != "release" {
		t.Fatalf("expected StepByProduce(release_record).Step == release, got %q", step.Step)
	}
}

func TestParseSpecV1Normalizes(t *testing.T) {
	raw := literalToMap(t, softwareDeliveryV1Literal)

	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec returned error: %v", err)
	}

	if len(spec.Roles) != 3 {
		t.Fatalf("expected 3 roles, got %d", len(spec.Roles))
	}
	if len(spec.Skeleton) != 3 {
		t.Fatalf("expected 3 skeleton steps (v1 has no release step), got %d", len(spec.Skeleton))
	}

	// roles[].independent_from -> role_independence constraint, unconditional.
	var independenceConstraints []SpecConstraint
	for _, c := range spec.Constraints {
		if c.Kind == "role_independence" {
			independenceConstraints = append(independenceConstraints, c)
		}
	}
	if len(independenceConstraints) != 1 {
		t.Fatalf("expected 1 role_independence constraint, got %d: %#v", len(independenceConstraints), independenceConstraints)
	}
	got := independenceConstraints[0]
	if len(got.Roles) != 2 || got.Roles[0] != "reviewer" || got.Roles[1] != "developer" {
		t.Fatalf("expected role_independence Roles [reviewer developer], got %#v", got.Roles)
	}
	if got.When != (SpecConstraintWhen{}) {
		t.Fatalf("expected unconditional When, got %#v", got.When)
	}

	// roles[].collapsible_with -> collapse_rules, symmetric pairs deduped.
	if len(spec.CollapseRules) != 1 {
		t.Fatalf("expected 1 collapse rule (developer/tester deduped), got %d: %#v", len(spec.CollapseRules), spec.CollapseRules)
	}
	collapseRoles := spec.CollapseRules[0].Roles
	if len(collapseRoles) != 2 || collapseRoles[0] != "developer" || collapseRoles[1] != "tester" {
		t.Fatalf("expected collapse rule roles [developer tester], got %#v", collapseRoles)
	}

	// string criteria -> SpecAcceptanceCriterion{Statement}.
	wantCriteria := []string{
		"变更以 branch+commit 交付且通过独立审查",
		"测试报告覆盖主路径且结论可判",
	}
	if len(spec.DefaultAcceptanceCriteria) != len(wantCriteria) {
		t.Fatalf("expected %d acceptance criteria, got %d: %#v", len(wantCriteria), len(spec.DefaultAcceptanceCriteria), spec.DefaultAcceptanceCriteria)
	}
	for i, want := range wantCriteria {
		c := spec.DefaultAcceptanceCriteria[i]
		if c.Statement != want {
			t.Fatalf("criterion %d: expected statement %q, got %q", i, want, c.Statement)
		}
		if c.AppliesFromExit != "" {
			t.Fatalf("criterion %d: expected empty AppliesFromExit, got %q", i, c.AppliesFromExit)
		}
	}

	// risk_policy.release_requires_human=true but no step named "release" in
	// the v1 skeleton -> no human_gate constraint produced.
	for _, c := range spec.Constraints {
		if c.Kind == "human_gate" {
			t.Fatalf("expected no human_gate constraint (v1 seed has no release step), got %#v", c)
		}
	}
}

func TestParseSpecUnknownConstraintKind(t *testing.T) {
	raw := map[string]any{
		"spec_version": 2,
		"constraints": []any{
			map[string]any{"kind": "made_up"},
		},
	}

	_, err := ParseSpec(raw)
	if err == nil {
		t.Fatalf("expected error for unknown constraint kind, got nil")
	}
	if !strings.Contains(err.Error(), "unknown constraint kind") {
		t.Fatalf("expected error to contain %q, got %q", "unknown constraint kind", err.Error())
	}
}

func TestParseSpecEmptyIsGeneric(t *testing.T) {
	spec, err := ParseSpec(map[string]any{})
	if err != nil {
		t.Fatalf("ParseSpec({}) returned error: %v", err)
	}
	if !reflect.DeepEqual(spec, SpecV2{}) {
		t.Fatalf("expected zero-value SpecV2, got %#v", spec)
	}
}
