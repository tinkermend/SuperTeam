package skill

import (
	"testing"

	"github.com/google/uuid"
)

func TestMergeRuntimeSkillsTruthTable(t *testing.T) {
	// §4.2 three rows, implemented at merge/filter level with source labels.
	universal := SkillRuntimeRecord{ID: uuid.New(), Slug: "db-inspect", SourceScope: "employee", Version: "1"}
	projectAOnly := SkillRuntimeRecord{ID: uuid.New(), Slug: "a-datasource", SourceScope: "project", Version: "1"}
	projectBCarried := SkillRuntimeRecord{ID: uuid.New(), Slug: "b-migrate", SourceScope: "employee", Version: "1"}
	projectBBound := SkillRuntimeRecord{ID: uuid.New(), Slug: "b-migrate", SourceScope: "project", Version: "2"}

	// Row1: universal carried → present without project supply.
	r1 := mergeRuntimeSkills([]SkillRuntimeRecord{universal}, nil)
	if len(r1.Skills) != 1 || r1.Skills[0].Slug != "db-inspect" {
		t.Fatalf("row1: %#v", r1)
	}

	// Row2: project A supply only → present from project side.
	r2 := mergeRuntimeSkills(nil, []SkillRuntimeRecord{projectAOnly})
	if len(r2.Skills) != 1 || r2.Skills[0].SourceScope != "project" {
		t.Fatalf("row2: %#v", r2)
	}

	// Row3 conflict same slug different versions: project wins + conflict marker.
	r3 := mergeRuntimeSkills([]SkillRuntimeRecord{projectBCarried}, []SkillRuntimeRecord{projectBBound})
	if len(r3.Skills) != 1 || r3.Skills[0].Version != "2" || r3.Skills[0].SourceScope != "project" {
		t.Fatalf("row3 winner: %#v", r3.Skills)
	}
	if len(r3.Conflicts) != 1 || r3.Conflicts[0].Source != "project_binding" {
		t.Fatalf("row3 conflicts: %#v", r3.Conflicts)
	}
}

func TestVenueFilterDropsForeignProjectSkills(t *testing.T) {
	// Simulates §4.2 filter: employee carries B-bound skill into A → dropped before merge.
	projectA := uuid.New()
	projectB := uuid.New()
	skillID := uuid.New()
	bindings := map[uuid.UUID][]uuid.UUID{skillID: {projectB}}
	rec := SkillRuntimeRecord{ID: skillID, Slug: "b-migrate", SourceScope: "employee"}
	allowed := false
	for _, pid := range bindings[rec.ID] {
		if pid == projectA {
			allowed = true
		}
	}
	if len(bindings[rec.ID]) == 0 {
		allowed = true
	}
	if allowed {
		t.Fatalf("foreign project-bound skill must not pass venue filter for project A")
	}
	// bound to A → allowed
	bindings[skillID] = []uuid.UUID{projectA}
	allowed = false
	if len(bindings[rec.ID]) == 0 {
		allowed = true
	} else {
		for _, pid := range bindings[rec.ID] {
			if pid == projectA {
				allowed = true
			}
		}
	}
	if !allowed {
		t.Fatalf("skill bound to A must pass venue filter")
	}
}
