package project

import (
	"testing"

	"github.com/google/uuid"
)

func TestProjectCapabilityProjectionWrappers(t *testing.T) {
	snap := ExtractCapabilityProjection(nil, nil)
	if snap.Available {
		t.Fatal("expected unavailable")
	}
	id := uuid.New()
	snap.Skills = []ProjectedSkillItem{{SkillID: id.String(), SkillKey: "k", SourceScope: "project"}}
	EnrichCapabilityProjectionNames(&snap, map[uuid.UUID]string{id: "Name"})
	if snap.Skills[0].SkillName != "Name" {
		t.Fatalf("name %#v", snap.Skills[0].SkillName)
	}
	if len(CollectSkillIDsFromProjection(snap)) != 1 {
		t.Fatal("ids")
	}
	_ = emptyCapabilityProjection()
}
