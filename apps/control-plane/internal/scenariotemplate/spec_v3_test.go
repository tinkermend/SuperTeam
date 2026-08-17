package scenariotemplate

import (
	"reflect"
	"strings"
	"testing"
)

func v3SkeletonStep(input any, notes []any) map[string]any {
	step := map[string]any{
		"step": "review",
		"role": "reviewer",
		"required_inputs_defaults": []any{input},
	}
	if notes != nil {
		step["notes_defaults"] = notes
	}
	return step
}

func TestParseSpecV3ObjectRequiredInputs(t *testing.T) {
	raw := map[string]any{
		"spec_version": 3,
		"roles":        []any{map[string]any{"key": "reviewer", "title": "评审"}},
		"skeleton": []any{
			map[string]any{
				"step": "develop",
				"role": "reviewer",
				"produces_defaults": []any{
					map[string]any{"name": "head_commit", "kind": "git_commit"},
				},
			},
			v3SkeletonStep(map[string]any{"name": "head_commit", "kind": "git_commit", "required": false}, []any{
				map[string]any{"name": "risk_notes", "description": "风险备注"},
			}),
		},
	}
	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	if len(spec.Skeleton) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(spec.Skeleton))
	}
	inputs := spec.Skeleton[1].RequiredInputsDefaults
	if len(inputs) != 1 || inputs[0].Name != "head_commit" || inputs[0].Kind != "git_commit" {
		t.Fatalf("structured required input not parsed: %#v", inputs)
	}
	if inputs[0].Required == nil || *inputs[0].Required {
		t.Fatalf("required=false not honored: %#v", inputs[0])
	}
	notes := spec.Skeleton[1].NotesDefaults
	if len(notes) != 1 || notes[0].Name != "risk_notes" || notes[0].Description != "风险备注" {
		t.Fatalf("notes_defaults not parsed: %#v", notes)
	}
}

func TestParseSpecV2StringInputsNormalized(t *testing.T) {
	raw := map[string]any{
		"spec_version": 2,
		"roles":        []any{map[string]any{"key": "reviewer", "title": "评审"}},
		"skeleton":     []any{v3SkeletonStep("head_commit", nil)},
	}
	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	inputs := spec.Skeleton[0].RequiredInputsDefaults
	if len(inputs) != 1 || inputs[0].Name != "head_commit" {
		t.Fatalf("string form not normalized: %#v", inputs)
	}
	if inputs[0].Required == nil || !*inputs[0].Required {
		t.Fatalf("string form must normalize to required=true: %#v", inputs[0])
	}
	if spec.Skeleton[0].NotesDefaults != nil {
		t.Fatalf("v2 spec must not carry notes: %#v", spec.Skeleton[0].NotesDefaults)
	}
}

func TestParseSpecPreV3ShapeSanitized(t *testing.T) {
	// 声明 spec_version 2 却携带 v3 形态：读侧收敛回 v2 语义（写侧 guardrail
	// 会拒绝注册，这里是兜底）。
	raw := map[string]any{
		"spec_version": 2,
		"roles":        []any{map[string]any{"key": "reviewer", "title": "评审"}},
		"skeleton": []any{v3SkeletonStep(
			map[string]any{"name": "head_commit", "kind": "git_commit"},
			[]any{map[string]any{"name": "risk_notes"}},
		)},
	}
	spec, err := ParseSpec(raw)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	step := spec.Skeleton[0]
	if step.NotesDefaults != nil {
		t.Fatalf("notes must be dropped on pre-v3 spec: %#v", step.NotesDefaults)
	}
	if len(step.RequiredInputsDefaults) != 1 || step.RequiredInputsDefaults[0].Name != "head_commit" {
		t.Fatalf("object entry must degrade to name: %#v", step.RequiredInputsDefaults)
	}
	if step.RequiredInputsDefaults[0].Kind != "" {
		t.Fatalf("kind must be dropped on pre-v3 spec: %#v", step.RequiredInputsDefaults[0])
	}
}

func TestMissingSpecVersionForV3Shape(t *testing.T) {
	cases := []struct {
		name    string
		raw     map[string]any
		missing bool
	}{
		{
			name: "对象形态 required_inputs 且未声明版本",
			raw: map[string]any{
				"skeleton": []any{v3SkeletonStep(map[string]any{"name": "head_commit"}, nil)},
			},
			missing: true,
		},
		{
			name: "notes_defaults 且未声明版本",
			raw: map[string]any{
				"skeleton": []any{v3SkeletonStep("head_commit", []any{map[string]any{"name": "risk_notes"}})},
			},
			missing: true,
		},
		{
			name: "v3 已声明",
			raw: map[string]any{
				"spec_version": 3,
				"skeleton":     []any{v3SkeletonStep(map[string]any{"name": "head_commit"}, []any{map[string]any{"name": "risk_notes"}})},
			},
			missing: false,
		},
		{
			name: "纯字符串 v2",
			raw: map[string]any{
				"spec_version": 2,
				"skeleton":     []any{v3SkeletonStep("head_commit", nil)},
			},
			missing: false,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			missing, _ := MissingSpecVersionForV3Shape(testCase.raw)
			if missing != testCase.missing {
				t.Fatalf("MissingSpecVersionForV3Shape = %v, want %v", missing, testCase.missing)
			}
		})
	}
}

func TestParseSpecV3DuplicateSlotNames(t *testing.T) {
	raw := map[string]any{
		"spec_version": 3,
		"roles":        []any{map[string]any{"key": "reviewer", "title": "评审"}},
		"skeleton": []any{map[string]any{
			"step": "review",
			"role": "reviewer",
			"required_inputs_defaults": []any{
				map[string]any{"name": "head_commit"},
				map[string]any{"name": "head_commit"},
			},
		}},
	}
	_, err := ParseSpec(raw)
	if err == nil || !strings.Contains(err.Error(), "重名") {
		t.Fatalf("expected duplicate slot name error, got %v", err)
	}
}

func TestRequiredInputsFromStringsZeroValue(t *testing.T) {
	if !reflect.DeepEqual(requiredInputsFromStrings(nil), []SpecRequiredInput(nil)) {
		t.Fatal("nil input must stay nil")
	}
}
