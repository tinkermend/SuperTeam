package scenariotemplate

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

const softwareDeliveryRoleViewLiteral = `{"spec_version":2,"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"branch_ref","kind":"branch_ref"},{"name":"head_commit","kind":"git_commit"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]},{"step":"release","role":"developer","depends_on":["review","test"],"required_inputs_defaults":["review_verdict","test_report"],"produces_defaults":[{"name":"release_record","kind":"evidence_ref"}]}],"exits":[{"deliverable":"branch_ref","label":"交付分支"},{"deliverable":"review_verdict","label":"审查通过并合入"},{"deliverable":"release_record","label":"发布上线"}],"constraints":[{"kind":"role_independence","roles":["reviewer","developer"],"when":{"exit_at_or_beyond":"review_verdict"}}],"collapse_rules":[],"default_acceptance_criteria":[]}`

func TestBuildRoleViewSoftwareDelivery(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(softwareDeliveryRoleViewLiteral), &raw); err != nil {
		t.Fatal(err)
	}
	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	template := ScenarioTemplate{Key: "software_delivery", Name: "软件开发", Spec: raw}
	view, err := buildRoleView(context.Background(), &Service{}, uuid.Nil, template, spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Roles) != 3 {
		t.Fatalf("roles = %d, want 3", len(view.Roles))
	}
	if len(view.Exits) != 3 {
		t.Fatalf("exits = %d, want 3", len(view.Exits))
	}
	// Shallow exit branch_ref: only developer
	if got := view.Exits[0].RequiredRoles; len(got) != 1 || got[0] != "developer" {
		t.Fatalf("branch_ref roles = %v, want [developer]", got)
	}
	// review_verdict: developer + reviewer, independence pair
	if got := view.Exits[1].RequiredRoles; len(got) != 2 {
		t.Fatalf("review_verdict roles = %v, want developer+reviewer", got)
	}
	if len(view.Exits[1].RoleIndependencePairs) != 1 {
		t.Fatalf("review_verdict independence pairs = %d, want 1", len(view.Exits[1].RoleIndependencePairs))
	}
	// release: all three
	if got := view.Exits[2].RequiredRoles; len(got) != 3 {
		t.Fatalf("release roles = %v, want 3", got)
	}
}

type stubHolderCounter struct {
	counts map[string]int
}

func (s stubHolderCounter) CountActiveHolders(_ context.Context, _ uuid.UUID, roleKey string) (int, error) {
	return s.counts[roleKey], nil
}

func TestBuildRoleViewHolderCounts(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(softwareDeliveryRoleViewLiteral), &raw); err != nil {
		t.Fatal(err)
	}
	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatal(err)
	}
	svc := &Service{roleHolderCounter: stubHolderCounter{counts: map[string]int{
		"developer": 2,
		"reviewer":  6,
		"tester":    2,
	}}}
	view, err := buildRoleView(context.Background(), svc, uuid.New(), ScenarioTemplate{Key: "software_delivery", Name: "软件开发"}, spec)
	if err != nil {
		t.Fatal(err)
	}
	byKey := map[string]int{}
	for _, r := range view.Roles {
		byKey[r.RoleKey] = r.HolderCount
	}
	if byKey["developer"] != 2 || byKey["reviewer"] != 6 || byKey["tester"] != 2 {
		t.Fatalf("holder counts = %v", byKey)
	}
}
