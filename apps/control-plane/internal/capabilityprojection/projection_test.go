package capabilityprojection

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestExtractCapabilityProjectionWhitelistAndSources(t *testing.T) {
	skillA := uuid.New()
	skillB := uuid.New()
	mcpID := uuid.New()
	payload := map[string]any{
		"skills": []any{
			map[string]any{
				"skill_id":           skillA.String(),
				"skill_key":          "linux",
				"version":            "2",
				"source_scope":       "project",
				"archive_object_ref": "s3://secret/should-not-leak",
			},
			map[string]any{
				"skill_id":     skillB.String(),
				"skill_key":    "db-inspect",
				"source_scope": "employee",
			},
		},
		"mcp_servers": []any{
			map[string]any{
				"server_id":          mcpID.String(),
				"server_key":         "github-mcp",
				"name":               "GitHub",
				"source_scope":       "dependency_closure",
				"url":                "https://evil.example",
				"headers_env":        map[string]any{"Authorization": "secret"},
				"credential_env_var": "GH_TOKEN",
			},
		},
		"environment": []any{
			map[string]any{"name": "GH_TOKEN", "value": "super-secret", "sensitive": true},
		},
		"prompt": "do not leak",
		"metadata": map[string]any{
			"skill_conflicts": []any{
				map[string]any{
					"slug":             "linux",
					"source":           "project_binding",
					"winning_skill_id": skillA.String(),
					"dropped_skill_id": skillB.String(),
					"winning_source":   "project",
					"dropped_source":   "employee",
				},
			},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	attMeta, _ := json.Marshal(map[string]any{
		"skill_conflicts": []any{
			map[string]any{"slug": "beta", "source": "workspace_native"},
			// duplicate CP conflict should dedupe
			map[string]any{
				"slug":             "linux",
				"source":           "project_binding",
				"winning_skill_id": skillA.String(),
				"dropped_skill_id": skillB.String(),
				"winning_source":   "project",
				"dropped_source":   "employee",
			},
		},
	})

	snap := Extract(raw, [][]byte{attMeta})
	if !snap.Available {
		t.Fatal("expected available")
	}
	if snap.Summary.SkillCount != 2 || snap.Summary.MCPCount != 1 || snap.Summary.ConflictCount != 2 {
		t.Fatalf("summary %#v", snap.Summary)
	}
	if snap.Summary.BySource["project"] != 1 || snap.Summary.BySource["dependency_closure"] != 1 {
		t.Fatalf("by_source %#v", snap.Summary.BySource)
	}
	if snap.MCPServers[0].ServerName != "GitHub" || snap.MCPServers[0].SourceScope != "dependency_closure" {
		t.Fatalf("mcp %#v", snap.MCPServers[0])
	}

	// Serialize and ensure secrets never appear.
	out, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{"super-secret", "s3://secret", "evil.example", "GH_TOKEN", "do not leak", "Authorization"} {
		if strings.Contains(s, bad) {
			t.Fatalf("leaked %q in %s", bad, s)
		}
	}
}

func TestExtractCapabilityProjectionUnavailable(t *testing.T) {
	snap := Extract(nil, nil)
	if snap.Available {
		t.Fatal("expected unavailable")
	}
	if snap.Skills == nil || snap.MCPServers == nil || snap.SkillConflicts == nil {
		t.Fatal("slices must be non-nil for stable JSON")
	}
}

func TestEnrichNames(t *testing.T) {
	id := uuid.New()
	snap := CapabilityProjectionSnapshot{
		Available: true,
		Skills:    []ProjectedSkillItem{{SkillID: id.String(), SkillKey: "linux", SourceScope: "project"}},
		SkillConflicts: []ProjectedSkillConflict{
			{Slug: "linux", Source: "project_binding", WinningSkillID: id.String()},
		},
		Summary: CapabilityProjectionSummary{BySource: map[string]int{}},
	}
	EnrichNames(&snap, map[uuid.UUID]string{id: "Linux 排障"})
	if snap.Skills[0].SkillName != "Linux 排障" {
		t.Fatalf("skill name %#v", snap.Skills[0].SkillName)
	}
	if snap.SkillConflicts[0].WinningSkillName != "Linux 排障" {
		t.Fatalf("conflict name %#v", snap.SkillConflicts[0].WinningSkillName)
	}
}
