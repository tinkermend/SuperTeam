package scenariotemplate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPreviewExitAutonomyMarksDeepHumanJudgment(t *testing.T) {
	spec := SpecV2{
		Exits: []SpecExit{
			{Deliverable: "branch_ref", Label: "分支"},
			{Deliverable: "release_record", Label: "发布"},
		},
		DefaultAcceptanceCriteria: []SpecAcceptanceCriterion{
			{Statement: "机器测过", AppliesFromExit: "branch_ref"},
			{Statement: "负责人确认发布", AppliesFromExit: "release_record", VerificationMethod: "human_judgment"},
		},
		Constraints: []SpecConstraint{
			{Kind: "human_gate", Target: "ship", When: SpecConstraintWhen{ExitAtOrBeyond: "release_record"}},
		},
	}

	preview := PreviewExitAutonomy(spec)
	require.Len(t, preview, 2)
	require.False(t, preview[0].StopsHuman)
	require.True(t, preview[1].StopsHuman)
	require.Contains(t, preview[1].Reasons[0], "acceptance_criterion:")
	require.Contains(t, preview[1].Reasons, "human_gate:ship")
	require.True(t, RequiresFullAutoExitPin(spec))
}

func TestValidatePinnedExitDeliverable(t *testing.T) {
	spec := SpecV2{Exits: []SpecExit{{Deliverable: "branch_ref"}, {Deliverable: "release_record"}}}
	require.NoError(t, ValidatePinnedExitDeliverable(spec, ""))
	require.NoError(t, ValidatePinnedExitDeliverable(spec, "branch_ref"))
	require.Error(t, ValidatePinnedExitDeliverable(spec, "missing"))
}
