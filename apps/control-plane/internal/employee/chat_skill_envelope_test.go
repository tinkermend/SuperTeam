package employee

import (
	"testing"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/skill"
)

func TestApplyChatSkillEnvelopeEmptySelectionUsesBindings(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	c := uuid.New() // employee-only, not in bindings
	supply := []skill.SkillRuntimeRecord{
		{ID: a, Slug: "a"},
		{ID: b, Slug: "b"},
		{ID: c, Slug: "c"},
	}
	allow := skillIDSet([]uuid.UUID{a, b})
	out, err := applyChatSkillEnvelope(supply, allow, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(out))
	}
	got := map[uuid.UUID]bool{out[0].ID: true, out[1].ID: true}
	if !got[a] || !got[b] || got[c] {
		t.Fatalf("unexpected set %#v", got)
	}
}

func TestApplyChatSkillEnvelopeRejectsOutsideSurface(t *testing.T) {
	a := uuid.New()
	outsider := uuid.New()
	allow := skillIDSet([]uuid.UUID{a})
	_, err := applyChatSkillEnvelope(
		[]skill.SkillRuntimeRecord{{ID: a, Slug: "a"}},
		allow,
		[]uuid.UUID{outsider},
	)
	if err == nil {
		t.Fatal("expected error for outsider skill")
	}
}

func TestApplyChatSkillEnvelopeSelectionIntersectsSupply(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	allow := skillIDSet([]uuid.UUID{a, b})
	out, err := applyChatSkillEnvelope(
		[]skill.SkillRuntimeRecord{{ID: a, Slug: "a"}, {ID: b, Slug: "b"}},
		allow,
		[]uuid.UUID{a},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != a {
		t.Fatalf("expected only a, got %#v", out)
	}
}

func TestApplyChatSkillEnvelopeNoBindingsYieldsEmpty(t *testing.T) {
	a := uuid.New()
	out, err := applyChatSkillEnvelope(
		[]skill.SkillRuntimeRecord{{ID: a, Slug: "a"}},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty, got %#v", out)
	}
}
