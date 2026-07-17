package projectcoordination

import "testing"

func TestStandardConditionsPresent(t *testing.T) {
	all := standardConditions(fakeChatCompletionClient{content: `{"detected":false}`}, "test-model")

	byKey := make(map[string]ConditionSpec, len(all))
	for _, spec := range all {
		byKey[spec.Key] = spec
	}

	secretLeak, ok := byKey["secret_leak"]
	if !ok {
		t.Fatalf("expected secret_leak in standardConditions, got keys=%v", keysOf(byKey))
	}
	if secretLeak.Kind != "rule" {
		t.Fatalf("expected secret_leak.Kind=rule, got %q", secretLeak.Kind)
	}
	if secretLeak.DefaultAction != reviewGateActionBlock {
		t.Fatalf("expected secret_leak.DefaultAction=block, got %q", secretLeak.DefaultAction)
	}
	if secretLeak.Detector == nil || secretLeak.Detector.Key() != "secret_leak" {
		t.Fatalf("expected secret_leak.Detector wired to secret_leak, got %v", secretLeak.Detector)
	}

	codeReview, ok := byKey["code_review"]
	if !ok {
		t.Fatalf("expected code_review in standardConditions, got keys=%v", keysOf(byKey))
	}
	if codeReview.Kind != "llm" {
		t.Fatalf("expected code_review.Kind=llm, got %q", codeReview.Kind)
	}
	if codeReview.DefaultAction != reviewGateActionNeedHuman {
		t.Fatalf("expected code_review.DefaultAction=need_human, got %q", codeReview.DefaultAction)
	}
	if codeReview.Detector == nil || codeReview.Detector.Key() != "code_review" {
		t.Fatalf("expected code_review.Detector wired to code_review, got %v", codeReview.Detector)
	}

	if len(all) != 2 {
		t.Fatalf("expected exactly 2 preset conditions in P1, got %d", len(all))
	}
}

func keysOf(m map[string]ConditionSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func testConditions() []ConditionSpec {
	return standardConditions(fakeChatCompletionClient{content: `{"detected":false}`}, "test-model")
}

func findEnabled(t *testing.T, enabled []EnabledCondition, key string) EnabledCondition {
	t.Helper()
	for _, e := range enabled {
		if e.Spec.Key == key {
			return e
		}
	}
	t.Fatalf("expected %q to be enabled, got enabled=%v", key, enabledKeys(enabled))
	return EnabledCondition{}
}

func enabledKeys(enabled []EnabledCondition) []string {
	keys := make([]string, 0, len(enabled))
	for _, e := range enabled {
		keys = append(keys, e.Spec.Key)
	}
	return keys
}

func TestEnabledConditionsDefaultWhenNoPolicy(t *testing.T) {
	all := testConditions()

	cases := []map[string]any{
		nil,
		{},
		{"other_key": "irrelevant"},
	}

	for i, policy := range cases {
		enabled := enabledConditions(policy, all)
		if len(enabled) != len(all) {
			t.Fatalf("case %d: expected all %d conditions enabled, got %d (%v)", i, len(all), len(enabled), enabledKeys(enabled))
		}
		secretLeak := findEnabled(t, enabled, "secret_leak")
		if secretLeak.Action != reviewGateActionBlock {
			t.Fatalf("case %d: expected secret_leak default action=block, got %q", i, secretLeak.Action)
		}
		codeReview := findEnabled(t, enabled, "code_review")
		if codeReview.Action != reviewGateActionNeedHuman {
			t.Fatalf("case %d: expected code_review default action=need_human, got %q", i, codeReview.Action)
		}
	}
}

func TestEnabledConditionsOffDisables(t *testing.T) {
	all := testConditions()
	policy := map[string]any{
		"review_gate_conditions": map[string]any{
			"code_review": "off",
		},
	}

	enabled := enabledConditions(policy, all)

	for _, e := range enabled {
		if e.Spec.Key == "code_review" {
			t.Fatalf("expected code_review to be disabled by off, got enabled with action=%q", e.Action)
		}
	}

	secretLeak := findEnabled(t, enabled, "secret_leak")
	if secretLeak.Action != reviewGateActionBlock {
		t.Fatalf("expected secret_leak to remain enabled at default action=block, got %q", secretLeak.Action)
	}

	if len(enabled) != 1 {
		t.Fatalf("expected exactly 1 enabled condition, got %d (%v)", len(enabled), enabledKeys(enabled))
	}
}

func TestEnabledConditionsActionOverride(t *testing.T) {
	all := testConditions()
	policy := map[string]any{
		"review_gate_conditions": map[string]any{
			"secret_leak": "need_human",
		},
	}

	enabled := enabledConditions(policy, all)

	secretLeak := findEnabled(t, enabled, "secret_leak")
	if secretLeak.Action != reviewGateActionNeedHuman {
		t.Fatalf("expected secret_leak action overridden to need_human, got %q", secretLeak.Action)
	}

	// code_review not mentioned: stays enabled at its own DefaultAction.
	codeReview := findEnabled(t, enabled, "code_review")
	if codeReview.Action != reviewGateActionNeedHuman {
		t.Fatalf("expected code_review default action=need_human, got %q", codeReview.Action)
	}
}

func TestEnabledConditionsUnknownActionFallsBack(t *testing.T) {
	all := testConditions()
	policy := map[string]any{
		"review_gate_conditions": map[string]any{
			"secret_leak": "bogus",
		},
	}

	enabled := enabledConditions(policy, all)

	secretLeak := findEnabled(t, enabled, "secret_leak")
	if secretLeak.Action != reviewGateActionBlock {
		t.Fatalf("expected unknown action to fall back to DefaultAction=block, got %q", secretLeak.Action)
	}
}

func TestReviewGateMinorToleranceReads(t *testing.T) {
	if got := reviewGateMinorTolerance(nil); got != 0 {
		t.Fatalf("expected default tolerance=0 for nil policy, got %d", got)
	}
	if got := reviewGateMinorTolerance(map[string]any{}); got != 0 {
		t.Fatalf("expected default tolerance=0 for empty policy, got %d", got)
	}
	if got := reviewGateMinorTolerance(map[string]any{"review_gate_minor_tolerance": 2}); got != 2 {
		t.Fatalf("expected tolerance=2 for int literal, got %d", got)
	}
	if got := reviewGateMinorTolerance(map[string]any{"review_gate_minor_tolerance": float64(3)}); got != 3 {
		t.Fatalf("expected tolerance=3 for float64 (decoded-JSON shape), got %d", got)
	}
	if got := reviewGateMinorTolerance(map[string]any{"review_gate_minor_tolerance": -1}); got != 0 {
		t.Fatalf("expected negative tolerance to fall back to default 0, got %d", got)
	}
}
