package scenariotemplate

import (
	"encoding/json"
	"fmt"
)

// knownConstraintKinds is the registry of constraint kinds this parser
// accepts. Adding a new kind requires wiring an evaluator in
// projectcoordination/template_governance.go.
var knownConstraintKinds = map[string]bool{
	"role_independence": true,
	"stage_required":    true,
	"human_gate":        true,
}

type SpecProduce struct {
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

type SpecRole struct {
	Key, Title           string
	RequiredCapabilities []string `json:"required_capabilities"`
}

type SpecSkeletonStep struct {
	Step                   string        `json:"step"`
	Role                   string        `json:"role"`
	DependsOn              []string      `json:"depends_on,omitempty"`
	ProducesDefaults       []SpecProduce `json:"produces_defaults,omitempty"`
	RequiredInputsDefaults []string      `json:"required_inputs_defaults,omitempty"`
}

type SpecExit struct {
	Deliverable string `json:"deliverable"`
	Label       string `json:"label"`
}

// SpecConstraintWhen conditions a constraint on the plan having selected an
// exit at or beyond the given deliverable. Empty = unconditional.
type SpecConstraintWhen struct {
	ExitAtOrBeyond string `json:"exit_at_or_beyond,omitempty"`
}

type SpecConstraint struct {
	Kind   string             `json:"kind"` // registered in knownConstraintKinds: role_independence | stage_required | human_gate
	Roles  []string           `json:"roles,omitempty"`
	Step   string             `json:"step,omitempty"`
	Target string             `json:"target,omitempty"`
	When   SpecConstraintWhen `json:"when,omitempty"`
}

type SpecCollapseRule struct {
	Roles []string `json:"roles"`
}

type SpecAcceptanceCriterion struct {
	Statement       string `json:"statement"`
	AppliesFromExit string `json:"applies_from_exit,omitempty"`
}

type SpecV2 struct {
	SpecVersion               int                       `json:"spec_version"`
	Roles                     []SpecRole                `json:"roles"`
	Skeleton                  []SpecSkeletonStep        `json:"skeleton"`
	Exits                     []SpecExit                `json:"exits"`
	Constraints               []SpecConstraint          `json:"constraints"`
	CollapseRules             []SpecCollapseRule        `json:"collapse_rules"`
	DefaultAcceptanceCriteria []SpecAcceptanceCriterion `json:"default_acceptance_criteria"`
	FeasibilityThresholds     map[string]float64        `json:"feasibility_thresholds,omitempty"`
	BudgetProfile             map[string]any            `json:"budget_profile,omitempty"`
}

// ExitIndex returns the position of deliverable within Exits, or -1 if it is
// not a declared exit.
func (s SpecV2) ExitIndex(deliverable string) int {
	for i, exit := range s.Exits {
		if exit.Deliverable == deliverable {
			return i
		}
	}
	return -1
}

// StepByProduce returns the skeleton step whose produces_defaults includes
// name, if any.
func (s SpecV2) StepByProduce(name string) (SpecSkeletonStep, bool) {
	for _, step := range s.Skeleton {
		for _, produce := range step.ProducesDefaults {
			if produce.Name == name {
				return step, true
			}
		}
	}
	return SpecSkeletonStep{}, false
}

// v2OnlyTopLevelKeys are the spec fields normalizeV1 silently drops: a spec
// carrying any of them while declaring spec_version < 2 is a v2-shaped spec
// missing its version declaration, and registering it would silently disable
// the governance the author wrote down (constraints/exits gone). See
// MissingSpecVersionForV2Shape.
var v2OnlyTopLevelKeys = []string{"constraints", "exits", "collapse_rules"}

// MissingSpecVersionForV2Shape reports whether raw is a v2-shaped spec that
// forgot to declare "spec_version": 2 — the write-time guardrail predicate
// (spec 2026-07-18-scenario-template-spec-version-guardrail §3). It fires when
// specVersion(raw) < 2 AND the spec carries any v2-only top-level key, or an
// object-shaped default_acceptance_criteria entry (normalizeV1 only keeps
// string entries). Genuine v1 shapes (no v2-only fields) never match, so the
// legacy write path stays open and the runtime read path is untouched.
func MissingSpecVersionForV2Shape(raw map[string]any) (bool, []string) {
	if specVersion(raw) >= 2 {
		return false, nil
	}
	var offending []string
	for _, key := range v2OnlyTopLevelKeys {
		if _, ok := raw[key]; ok {
			offending = append(offending, key)
		}
	}
	for _, criterionAny := range toAnySlice(raw["default_acceptance_criteria"]) {
		if _, isString := criterionAny.(string); !isString {
			offending = append(offending, "default_acceptance_criteria(对象条目)")
			break
		}
	}
	return len(offending) > 0, offending
}

// ParseSpec parses a scenario template's raw JSONB spec into a typed SpecV2.
// specs with spec_version < 2 (or absent, e.g. nil/empty raw) are normalized
// from the legacy v1 shape. ParseSpec(nil) returns a zero-value SpecV2 with
// no error (generic fallback semantics).
func ParseSpec(raw map[string]any) (SpecV2, error) {
	if specVersion(raw) < 2 {
		return normalizeV1(raw)
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return SpecV2{}, fmt.Errorf("marshal spec: %w", err)
	}
	var spec SpecV2
	if err := json.Unmarshal(data, &spec); err != nil {
		return SpecV2{}, fmt.Errorf("unmarshal spec: %w", err)
	}
	if err := validateConstraints(spec, false); err != nil {
		return SpecV2{}, err
	}
	return spec, nil
}

func specVersion(raw map[string]any) int {
	switch v := raw["spec_version"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// normalizeV1 translates the legacy v1 spec shape into SpecV2:
//   - roles[].independent_from -> role_independence constraints (unconditional).
//   - roles[].collapsible_with -> collapse_rules (symmetric pairs deduped).
//   - string entries in default_acceptance_criteria -> SpecAcceptanceCriterion{Statement}.
//   - risk_policy.release_requires_human=true -> human_gate targeting a step
//     named "release", only if such a step exists in the skeleton.
func normalizeV1(raw map[string]any) (SpecV2, error) {
	var spec SpecV2

	seenCollapsePairs := map[string]bool{}
	for _, roleAny := range toAnySlice(raw["roles"]) {
		roleMap, ok := roleAny.(map[string]any)
		if !ok {
			continue
		}
		key := asString(roleMap["key"])
		spec.Roles = append(spec.Roles, SpecRole{
			Key:                  key,
			Title:                asString(roleMap["title"]),
			RequiredCapabilities: toStringSlice(roleMap["required_capabilities"]),
		})

		for _, other := range toStringSlice(roleMap["independent_from"]) {
			spec.Constraints = append(spec.Constraints, SpecConstraint{
				Kind:  "role_independence",
				Roles: []string{key, other},
			})
		}

		for _, other := range toStringSlice(roleMap["collapsible_with"]) {
			pairKey := collapsePairKey(key, other)
			if seenCollapsePairs[pairKey] {
				continue
			}
			seenCollapsePairs[pairKey] = true
			spec.CollapseRules = append(spec.CollapseRules, SpecCollapseRule{Roles: sortedPair(key, other)})
		}
	}

	hasReleaseStep := false
	for _, stepAny := range toAnySlice(raw["skeleton"]) {
		stepMap, ok := stepAny.(map[string]any)
		if !ok {
			continue
		}
		step := SpecSkeletonStep{
			Step:                   asString(stepMap["step"]),
			Role:                   asString(stepMap["role"]),
			DependsOn:              toStringSlice(stepMap["depends_on"]),
			RequiredInputsDefaults: toStringSlice(stepMap["required_inputs_defaults"]),
		}
		for _, produceAny := range toAnySlice(stepMap["produces_defaults"]) {
			produceMap, ok := produceAny.(map[string]any)
			if !ok {
				continue
			}
			step.ProducesDefaults = append(step.ProducesDefaults, SpecProduce{
				Name: asString(produceMap["name"]),
				Kind: asString(produceMap["kind"]),
			})
		}
		if step.Step == "release" {
			hasReleaseStep = true
		}
		spec.Skeleton = append(spec.Skeleton, step)
	}

	for _, criterionAny := range toAnySlice(raw["default_acceptance_criteria"]) {
		if statement, ok := criterionAny.(string); ok {
			spec.DefaultAcceptanceCriteria = append(spec.DefaultAcceptanceCriteria, SpecAcceptanceCriterion{Statement: statement})
		}
	}

	if riskPolicy, ok := raw["risk_policy"].(map[string]any); ok {
		releaseRequiresHuman, _ := riskPolicy["release_requires_human"].(bool)
		if releaseRequiresHuman && hasReleaseStep {
			spec.Constraints = append(spec.Constraints, SpecConstraint{
				Kind:   "human_gate",
				Target: "release",
			})
		}
	}

	if thresholdsRaw, ok := raw["feasibility_thresholds"].(map[string]any); ok {
		thresholds := map[string]float64{}
		for k, v := range thresholdsRaw {
			if n, ok := v.(float64); ok {
				thresholds[k] = n
			}
		}
		if len(thresholds) > 0 {
			spec.FeasibilityThresholds = thresholds
		}
	}

	// v1-normalized specs have no declared exits, so exit_at_or_beyond
	// references cannot be validated against them.
	if err := validateConstraints(spec, true); err != nil {
		return SpecV2{}, err
	}
	return spec, nil
}

// validateConstraints checks that every constraint's kind is registered in
// knownConstraintKinds, and (unless skipExitCheck, which applies to
// v1-normalized specs that carry no declared exits) that a non-empty
// when.exit_at_or_beyond matches a declared exit deliverable.
func validateConstraints(spec SpecV2, skipExitCheck bool) error {
	for _, constraint := range spec.Constraints {
		if !knownConstraintKinds[constraint.Kind] {
			return fmt.Errorf("unknown constraint kind %q", constraint.Kind)
		}
		if skipExitCheck || constraint.When.ExitAtOrBeyond == "" {
			continue
		}
		if spec.ExitIndex(constraint.When.ExitAtOrBeyond) == -1 {
			return fmt.Errorf("constraint %q: when.exit_at_or_beyond %q does not match a declared exit", constraint.Kind, constraint.When.ExitAtOrBeyond)
		}
	}
	return nil
}

func toAnySlice(v any) []any {
	arr, _ := v.([]any)
	return arr
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// sortedPair returns [a, b] sorted lexicographically, used as a canonical,
// order-independent representation of a role pair.
func sortedPair(a, b string) []string {
	if a <= b {
		return []string{a, b}
	}
	return []string{b, a}
}

// collapsePairKey returns a canonical key for a role pair, independent of
// declaration order, used to dedupe symmetric collapsible_with entries.
func collapsePairKey(a, b string) string {
	pair := sortedPair(a, b)
	return pair[0] + "\x00" + pair[1]
}
