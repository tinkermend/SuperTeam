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

func softwareDeliveryV2Spec(t *testing.T) scenariotemplate.SpecV2 {
	t.Helper()
	var raw map[string]any
	require.NoError(t, json.Unmarshal([]byte(softwareDeliveryV2Literal), &raw))
	spec, err := scenariotemplate.ParseSpec(raw)
	require.NoError(t, err)
	return spec
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
