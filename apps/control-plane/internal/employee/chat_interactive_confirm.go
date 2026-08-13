package employee

import (
	"fmt"
	"strings"

	"github.com/superteam/control-plane/internal/autonomypolicy"
)

// isInteractiveChatInvoker is true for Console/workbench CreateRun paths.
// Non-interactive invokers (automation, external API integrations — no human
// present, governance happened at rule save / binding grant) stamp identifying
// metadata and must never hit the light-confirm gate (spec §2/§7/§8).
func isInteractiveChatInvoker(req CreateDigitalEmployeeRunRequest) bool {
	if req.Metadata == nil {
		return true
	}
	if raw, _ := req.Metadata["automation_rule_id"].(string); strings.TrimSpace(raw) != "" {
		return false
	}
	if raw, _ := req.Metadata["external_integration_id"].(string); strings.TrimSpace(raw) != "" {
		return false
	}
	if src, _ := req.Metadata["source_type"].(string); strings.EqualFold(strings.TrimSpace(src), "automation") ||
		strings.EqualFold(strings.TrimSpace(src), "external_integration") {
		return false
	}
	if inv, _ := req.Metadata["invoker"].(string); strings.EqualFold(strings.TrimSpace(inv), "automation") {
		return false
	}
	return true
}

// chatInteractiveConfirmRequired decides whether interactive Chat must acknowledge
// before CreateRun (autonomy P4 / B2). No skill risk tags: triggers are explicit
// skill selection and/or a project autonomy ceiling of pause_at_gate.
func chatInteractiveConfirmRequired(skillIDsCount int, projectCeiling string) bool {
	if skillIDsCount > 0 {
		return true
	}
	return strings.TrimSpace(projectCeiling) == autonomypolicy.TierPauseAtGate
}

func enforceInteractiveLightConfirm(req CreateDigitalEmployeeRunRequest, projectCeiling string) error {
	if !isInteractiveChatInvoker(req) {
		return nil
	}
	if !chatInteractiveConfirmRequired(len(req.SkillIDs), projectCeiling) {
		return nil
	}
	if req.InteractiveConfirmed {
		return nil
	}
	return fmt.Errorf("%w: interactive light confirm required (set interactive_confirmed=true after user acknowledgment)", ErrInvalidInput)
}
