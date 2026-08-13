package autonomypolicy

import (
	"fmt"
	"strings"
)

// Binary autonomy tiers shared by automation rules, playbooks, and project ceilings (P2/P3).
const (
	TierPauseAtGate = "pause_at_gate"
	TierFullAuto    = "full_auto"
)

// Rank returns how autonomous a tier is (higher = more automatic). Unknown → -1.
func Rank(tier string) int {
	switch strings.TrimSpace(tier) {
	case TierPauseAtGate:
		return 0
	case TierFullAuto:
		return 1
	default:
		return -1
	}
}

// Normalize returns a known tier or error. Empty → pause_at_gate (safe default).
func Normalize(raw string) (string, error) {
	tier := strings.TrimSpace(raw)
	if tier == "" {
		return TierPauseAtGate, nil
	}
	if Rank(tier) < 0 {
		return "", fmt.Errorf("autonomy tier must be pause_at_gate or full_auto")
	}
	return tier, nil
}

// Min returns the stricter (less automatic) of two tiers. Invalid inputs treated as pause.
func Min(a, b string) string {
	ra, rb := Rank(a), Rank(b)
	if ra < 0 {
		ra = 0
		a = TierPauseAtGate
	}
	if rb < 0 {
		rb = 0
		b = TierPauseAtGate
	}
	if ra <= rb {
		return a
	}
	return b
}

// EffectiveCeiling composes playbook ceiling, optional project ceiling, and invoker tier.
// Absent ceilings (empty) do not tighten. Result is always a known tier.
func Effective(playbookCeiling, projectCeiling, invokerTier string) string {
	out, _ := Normalize(invokerTier)
	if c := strings.TrimSpace(playbookCeiling); c != "" {
		out = Min(out, c)
	}
	if c := strings.TrimSpace(projectCeiling); c != "" {
		out = Min(out, c)
	}
	return out
}

// CoordinationPolicyCeiling reads projects.coordination_policy.autonomy_ceiling.
func CoordinationPolicyCeiling(policy map[string]any) string {
	if policy == nil {
		return ""
	}
	raw, _ := policy["autonomy_ceiling"].(string)
	raw = strings.TrimSpace(raw)
	if Rank(raw) < 0 {
		return ""
	}
	return raw
}
