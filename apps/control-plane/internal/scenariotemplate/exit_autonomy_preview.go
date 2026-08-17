package scenariotemplate

import (
	"fmt"
	"strconv"
	"strings"
)

// ExitAutonomyPreview summarizes whether selecting a given exit would stop a
// human under declarative acceptance / human_gate rules (F6 / §6.5.6).
type ExitAutonomyPreview struct {
	Deliverable string   `json:"deliverable"`
	Label       string   `json:"label,omitempty"`
	StopsHuman  bool     `json:"stops_human"`
	Reasons     []string `json:"reasons,omitempty"`
}

// PreviewExitAutonomy expands each exit in the playbook and reports which
// ones carry declarative human stops (human_judgment acceptance criteria or
// human_gate constraints at/after that exit).
func PreviewExitAutonomy(spec SpecV2) []ExitAutonomyPreview {
	out := make([]ExitAutonomyPreview, 0, len(spec.Exits))
	for _, exit := range spec.Exits {
		preview := ExitAutonomyPreview{
			Deliverable: exit.Deliverable,
			Label:       exit.Label,
		}
		for i, criterion := range spec.DefaultAcceptanceCriteria {
			method := strings.TrimSpace(criterion.VerificationMethod)
			if method == "" {
				method = "automated_test"
			}
			if method != "human_judgment" {
				continue
			}
			severity := strings.TrimSpace(criterion.Severity)
			if severity == "" {
				severity = "blocking"
			}
			if severity != "blocking" {
				continue
			}
			if !ExitCondMet(spec, SpecConstraintWhen{ExitAtOrBeyond: criterion.AppliesFromExit}, exit.Deliverable) {
				continue
			}
			preview.StopsHuman = true
			preview.Reasons = append(preview.Reasons, "acceptance_criterion:"+criterionIDHint(criterion, i))
		}
		for _, constraint := range spec.Constraints {
			if strings.TrimSpace(constraint.Kind) != "human_gate" {
				continue
			}
			if !ExitCondMet(spec, constraint.When, exit.Deliverable) {
				continue
			}
			preview.StopsHuman = true
			target := strings.TrimSpace(constraint.Target)
			if target == "" {
				target = "human_gate"
			}
			preview.Reasons = append(preview.Reasons, "human_gate:"+target)
		}
		out = append(out, preview)
	}
	return out
}

func criterionIDHint(criterion SpecAcceptanceCriterion, index int) string {
	stmt := strings.TrimSpace(criterion.Statement)
	if stmt == "" {
		return "criterion_" + strconv.Itoa(index)
	}
	runes := []rune(stmt)
	if len(runes) > 24 {
		return string(runes[:24])
	}
	return stmt
}

// RequiresFullAutoExitPin reports whether a full_auto binding to this playbook
// needs either a pinned exit or an explicit acknowledge_exit_tier_semantics.
func RequiresFullAutoExitPin(spec SpecV2) bool {
	return len(spec.Exits) > 1
}

// ValidatePinnedExitDeliverable checks the pin is a declared exit when set.
func ValidatePinnedExitDeliverable(spec SpecV2, pinned string) error {
	pinned = strings.TrimSpace(pinned)
	if pinned == "" {
		return nil
	}
	if spec.ExitIndex(pinned) < 0 {
		return fmt.Errorf("pinned_exit_deliverable %q is not a declared exit", pinned)
	}
	return nil
}
