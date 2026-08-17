package scenariotemplate

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/superteam/control-plane/internal/autonomypolicy"
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
	Step                   string              `json:"step"`
	Role                   string              `json:"role"`
	Title                  string              `json:"title,omitempty"`
	DependsOn              []string            `json:"depends_on,omitempty"`
	ProducesDefaults       []SpecProduce       `json:"produces_defaults,omitempty"`
	RequiredInputsDefaults []SpecRequiredInput `json:"required_inputs_defaults,omitempty"`
	NotesDefaults          []SpecNote          `json:"notes_defaults,omitempty"`
}

// SpecRequiredInput 是下游步骤声明的一个平台槽位（spec v3，2026-08-16 交接包
// 专稿 §3.3）。v2 的字符串形态（"head_commit"）在解析时归一为
// {name, required: true}；kind 走开放注册表，非封闭枚举。
type SpecRequiredInput struct {
	Name     string `json:"name"`
	Kind     string `json:"kind,omitempty"`
	Required *bool  `json:"required,omitempty"` // 缺省 true
}

// UnmarshalJSON 接受 v2 字符串形态与 v3 对象形态，归一到同一结构。
func (i *SpecRequiredInput) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		i.Name = strings.TrimSpace(text)
		i.Required = boolPtr(true)
		return nil
	}
	type alias SpecRequiredInput
	var object alias
	if err := json.Unmarshal(data, &object); err != nil {
		return fmt.Errorf("required_inputs_defaults 条目须为字符串或 {name, kind, required} 对象: %w", err)
	}
	*i = SpecRequiredInput(object)
	i.Name = strings.TrimSpace(i.Name)
	return nil
}

// SpecNote 是下游步骤声明的一个结论槽位：由上游按 name 写入
// result_contract.handoff_notes，平台按下游声明提取合并。散文不进闸
// （2026-08-16 拍板 8），缺失只投影不打回。
type SpecNote struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
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
	// VerificationMethod / Severity: optional; empty keeps today's defaults
	// (automated_test / blocking) via normalizeCriterionDefaults (F5 / §6.5.2).
	VerificationMethod string `json:"verification_method,omitempty"`
	Severity           string `json:"severity,omitempty"`
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
	// AutonomyDefault / AutonomyCeiling: playbook posture (P3). Empty = unset
	// (no extra tighten / default pause recommendation). Enum: pause_at_gate | full_auto.
	AutonomyDefault string `json:"autonomy_default,omitempty"`
	AutonomyCeiling string `json:"autonomy_ceiling,omitempty"`
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

// MissingSpecVersionForV3Shape reports whether raw carries v3-only skeleton
// shapes (notes_defaults, or object-shaped required_inputs_defaults entries)
// while declaring spec_version < 3 — the write-time guardrail for the 2026-08-16
// handoff package spec §3.3, mirroring MissingSpecVersionForV2Shape: registering
// such a spec without the version declaration would silently drop the v3
// semantics on read (see sanitizePreV3Spec).
func MissingSpecVersionForV3Shape(raw map[string]any) (bool, []string) {
	if specVersion(raw) >= 3 {
		return false, nil
	}
	var offending []string
	hasObjectInput := false
	hasNotes := false
	for _, stepAny := range toAnySlice(raw["skeleton"]) {
		stepMap, ok := stepAny.(map[string]any)
		if !ok {
			continue
		}
		if !hasNotes && len(toAnySlice(stepMap["notes_defaults"])) > 0 {
			hasNotes = true
		}
		if hasObjectInput {
			continue
		}
		for _, entry := range toAnySlice(stepMap["required_inputs_defaults"]) {
			if _, isObject := entry.(map[string]any); isObject {
				hasObjectInput = true
				break
			}
		}
	}
	if hasNotes {
		offending = append(offending, "skeleton.notes_defaults")
	}
	if hasObjectInput {
		offending = append(offending, "skeleton.required_inputs_defaults(对象条目)")
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
	if specVersion(raw) < 3 {
		sanitizePreV3Spec(&spec)
	}
	if err := validateSkeletonSlots(spec); err != nil {
		return SpecV2{}, err
	}
	if err := validateConstraints(spec, false); err != nil {
		return SpecV2{}, err
	}
	if err := validateAutonomyFields(spec); err != nil {
		return SpecV2{}, err
	}
	if err := validateAcceptanceCriteria(spec); err != nil {
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
			Title:                  asString(stepMap["title"]),
			DependsOn:              toStringSlice(stepMap["depends_on"]),
			RequiredInputsDefaults: requiredInputsFromStrings(toStringSlice(stepMap["required_inputs_defaults"])),
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
	if err := validateSkeletonSlots(spec); err != nil {
		return SpecV2{}, err
	}
	return spec, nil
}

// requiredInputsFromStrings 把 v1/v2 的字符串形态归一为 {name, required: true}。
func requiredInputsFromStrings(names []string) []SpecRequiredInput {
	if len(names) == 0 {
		return nil
	}
	inputs := make([]SpecRequiredInput, 0, len(names))
	for _, name := range names {
		inputs = append(inputs, SpecRequiredInput{Name: strings.TrimSpace(name), Required: boolPtr(true)})
	}
	return inputs
}

// sanitizePreV3Spec 把声明 spec_version < 3 却携带 v3 形态的 spec 收敛回 v2
// 语义：对象条目降为 name（kind 丢弃），notes_defaults 清空。写侧
// MissingSpecVersionForV3Shape 会拒绝注册这种 spec，这里是读侧兜底——
// 防止 v3 语义在未声明版本时被静默启用（对齐 v2OnlyTopLevelKeys 的哲学）。
func sanitizePreV3Spec(spec *SpecV2) {
	for i := range spec.Skeleton {
		step := &spec.Skeleton[i]
		step.NotesDefaults = nil
		if step.RequiredInputsDefaults == nil {
			continue
		}
		names := make([]string, 0, len(step.RequiredInputsDefaults))
		for _, input := range step.RequiredInputsDefaults {
			if input.Name != "" {
				names = append(names, input.Name)
			}
		}
		step.RequiredInputsDefaults = requiredInputsFromStrings(names)
	}
}

// validateSkeletonSlots 校验槽位声明的形态：required_inputs 与 notes 的 name
// 非空、步骤内不重名（同名会让运行期槽位解析歧义）。
func validateSkeletonSlots(spec SpecV2) error {
	for _, step := range spec.Skeleton {
		inputs := map[string]struct{}{}
		for _, input := range step.RequiredInputsDefaults {
			if input.Name == "" {
				return fmt.Errorf("skeleton step %q: required_inputs_defaults 含空 name", step.Step)
			}
			if _, exists := inputs[input.Name]; exists {
				return fmt.Errorf("skeleton step %q: required_inputs_defaults 重名 %q", step.Step, input.Name)
			}
			inputs[input.Name] = struct{}{}
		}
		notes := map[string]struct{}{}
		for _, note := range step.NotesDefaults {
			if strings.TrimSpace(note.Name) == "" {
				return fmt.Errorf("skeleton step %q: notes_defaults 含空 name", step.Step)
			}
			if _, exists := notes[note.Name]; exists {
				return fmt.Errorf("skeleton step %q: notes_defaults 重名 %q", step.Step, note.Name)
			}
			notes[note.Name] = struct{}{}
		}
	}
	return nil
}

func boolPtr(value bool) *bool {
	return &value
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

func validateAutonomyFields(spec SpecV2) error {
	def := strings.TrimSpace(spec.AutonomyDefault)
	ceil := strings.TrimSpace(spec.AutonomyCeiling)
	if def != "" {
		if _, err := autonomypolicy.Normalize(def); err != nil {
			return fmt.Errorf("autonomy_default: %w", err)
		}
	}
	if ceil != "" {
		if _, err := autonomypolicy.Normalize(ceil); err != nil {
			return fmt.Errorf("autonomy_ceiling: %w", err)
		}
	}
	if def != "" && ceil != "" && autonomypolicy.Rank(def) > autonomypolicy.Rank(ceil) {
		return fmt.Errorf("autonomy_default %q exceeds autonomy_ceiling %q", def, ceil)
	}
	return nil
}

// knownAcceptanceVerificationMethods / severities mirror
// projectcoordination.knownVerificationMethods (F5). Empty values are allowed
// and mean "use platform defaults" at instantiate time.
var knownAcceptanceVerificationMethods = map[string]bool{
	"automated_test":     true,
	"human_judgment":     true,
	"adversarial_review": true,
	"review_gate":        true,
}

var knownAcceptanceSeverities = map[string]bool{
	"blocking":     true,
	"non_blocking": true,
}

func validateAcceptanceCriteria(spec SpecV2) error {
	for i, c := range spec.DefaultAcceptanceCriteria {
		method := strings.TrimSpace(c.VerificationMethod)
		if method != "" && !knownAcceptanceVerificationMethods[method] {
			return fmt.Errorf("default_acceptance_criteria[%d]: unknown verification_method %q", i, method)
		}
		severity := strings.TrimSpace(c.Severity)
		if severity != "" && !knownAcceptanceSeverities[severity] {
			return fmt.Errorf("default_acceptance_criteria[%d]: unknown severity %q", i, severity)
		}
		if exit := strings.TrimSpace(c.AppliesFromExit); exit != "" && spec.ExitIndex(exit) == -1 {
			return fmt.Errorf("default_acceptance_criteria[%d]: applies_from_exit %q does not match a declared exit", i, exit)
		}
	}
	return nil
}
