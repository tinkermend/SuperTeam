package projectcoordination

import (
	"context"
	"testing"

	"github.com/superteam/control-plane/internal/project"
)

// H3/H5/H6 as end-to-end engine path with scripted model (not just parse helpers):
// scripted client → runCastingGapDiscovery → R1/R2/R3 outcomes.

func TestScriptedDiscoverer_H3_UnknownKeyExternal(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed":true,"role_key":"totally_invented_role_xyz","reason":"编造角色"}`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", project.CastingGapInput{
		TaskTitle:          "t",
		ConclusionSummary:  "c",
		ActiveRoles:        []project.CastingGapRoleOption{{RoleKey: "developer"}},
		ParticipatingRoles: []string{"developer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Needed || !got.External || got.RoleKey != "" {
		t.Fatalf("H3 want external empty key, got %+v", got)
	}
}

func TestScriptedDiscoverer_H5_ParticipatingDiscarded(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed":true,"role_key":"developer","reason":"再要一个开发"}`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", project.CastingGapInput{
		TaskTitle:          "t",
		ConclusionSummary:  "c",
		ActiveRoles:        []project.CastingGapRoleOption{{RoleKey: "developer"}, {RoleKey: "reviewer"}},
		ParticipatingRoles: []string{"developer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Needed {
		t.Fatalf("H5 R2 must discard, got %+v", got)
	}
}

func TestScriptedDiscoverer_H6_GarbageSilent(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `<<<not-json>>>`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", project.CastingGapInput{
		TaskTitle:   "t",
		ActiveRoles: []project.CastingGapRoleOption{{RoleKey: "developer"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Needed {
		t.Fatalf("H6 silent, got %+v", got)
	}
}
