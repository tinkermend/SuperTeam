package capabilityprojection

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

// CapabilityProjectionSnapshot is the console-safe view of one attempt's
// dispatched skill/MCP set (three-layer model P3).
type CapabilityProjectionSnapshot struct {
	Available      bool                        `json:"available"`
	Skills         []ProjectedSkillItem        `json:"skills"`
	MCPServers     []ProjectedMcpItem          `json:"mcp_servers"`
	SkillConflicts []ProjectedSkillConflict    `json:"skill_conflicts"`
	Summary        CapabilityProjectionSummary `json:"summary"`
}

type CapabilityProjectionSummary struct {
	SkillCount    int            `json:"skill_count"`
	MCPCount      int            `json:"mcp_count"`
	ConflictCount int            `json:"conflict_count"`
	BySource      map[string]int `json:"by_source"`
}

type ProjectedSkillItem struct {
	SkillID     string `json:"skill_id"`
	SkillKey    string `json:"skill_key"`
	SkillName   string `json:"skill_name,omitempty"`
	Version     string `json:"version,omitempty"`
	SourceScope string `json:"source_scope"`
}

type ProjectedMcpItem struct {
	ServerID    string `json:"server_id"`
	ServerKey   string `json:"server_key"`
	ServerName  string `json:"server_name,omitempty"`
	SourceScope string `json:"source_scope"`
}

type ProjectedSkillConflict struct {
	Slug             string `json:"slug"`
	Source           string `json:"source"`
	WinningSkillID   string `json:"winning_skill_id,omitempty"`
	DroppedSkillID   string `json:"dropped_skill_id,omitempty"`
	WinningSource    string `json:"winning_source,omitempty"`
	DroppedSource    string `json:"dropped_source,omitempty"`
	WinningSkillName string `json:"winning_skill_name,omitempty"`
	DroppedSkillName string `json:"dropped_skill_name,omitempty"`
}

// Empty returns a stable unavailable snapshot.
func Empty() CapabilityProjectionSnapshot {
	return CapabilityProjectionSnapshot{
		Available:      false,
		Skills:         []ProjectedSkillItem{},
		MCPServers:     []ProjectedMcpItem{},
		SkillConflicts: []ProjectedSkillConflict{},
		Summary: CapabilityProjectionSummary{
			BySource: map[string]int{},
		},
	}
}

// Extract builds a safe snapshot from a start_session
// command receipt payload. Never copies environment, prompts, or secret fields.
// payloadJSON may be nil/empty → available=false.
func Extract(payloadJSON []byte, attestationConflictJSONList [][]byte) CapabilityProjectionSnapshot {
	snap := Empty()
	if len(payloadJSON) == 0 {
		return snap
	}
	var root map[string]any
	if err := json.Unmarshal(payloadJSON, &root); err != nil || root == nil {
		return snap
	}
	snap.Available = true

	if rawSkills, ok := root["skills"].([]any); ok {
		for _, raw := range rawSkills {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			skillID := strings.TrimSpace(stringFromAny(item["skill_id"]))
			skillKey := strings.TrimSpace(stringFromAny(item["skill_key"]))
			if skillID == "" && skillKey == "" {
				continue
			}
			snap.Skills = append(snap.Skills, ProjectedSkillItem{
				SkillID:     skillID,
				SkillKey:    skillKey,
				Version:     strings.TrimSpace(stringFromAny(item["version"])),
				SourceScope: strings.TrimSpace(stringFromAny(item["source_scope"])),
			})
		}
	}

	if rawMCP, ok := root["mcp_servers"].([]any); ok {
		for _, raw := range rawMCP {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			serverID := strings.TrimSpace(stringFromAny(item["server_id"]))
			serverKey := strings.TrimSpace(stringFromAny(item["server_key"]))
			if serverID == "" && serverKey == "" {
				continue
			}
			name := strings.TrimSpace(stringFromAny(item["name"]))
			if name == "" {
				name = strings.TrimSpace(stringFromAny(item["server_name"]))
			}
			snap.MCPServers = append(snap.MCPServers, ProjectedMcpItem{
				ServerID:    serverID,
				ServerKey:   serverKey,
				ServerName:  name,
				SourceScope: strings.TrimSpace(stringFromAny(item["source_scope"])),
			})
		}
	}

	var conflicts []ProjectedSkillConflict
	if meta, ok := root["metadata"].(map[string]any); ok {
		conflicts = append(conflicts, parseSkillConflicts(meta["skill_conflicts"])...)
	}
	for _, raw := range attestationConflictJSONList {
		if len(raw) == 0 {
			continue
		}
		var meta map[string]any
		if err := json.Unmarshal(raw, &meta); err != nil || meta == nil {
			// attestation metadata may be the whole object; try as array-bearing map
			continue
		}
		conflicts = append(conflicts, parseSkillConflicts(meta["skill_conflicts"])...)
	}
	snap.SkillConflicts = dedupeSkillConflicts(conflicts)
	recomputeCapabilityProjectionSummary(&snap)
	return snap
}

func parseSkillConflicts(raw any) []ProjectedSkillConflict {
	arr, ok := raw.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}
	out := make([]ProjectedSkillConflict, 0, len(arr))
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		slug := strings.TrimSpace(stringFromAny(m["slug"]))
		source := strings.TrimSpace(stringFromAny(m["source"]))
		if slug == "" && source == "" {
			continue
		}
		out = append(out, ProjectedSkillConflict{
			Slug:           slug,
			Source:         source,
			WinningSkillID: strings.TrimSpace(stringFromAny(m["winning_skill_id"])),
			DroppedSkillID: strings.TrimSpace(stringFromAny(m["dropped_skill_id"])),
			WinningSource:  strings.TrimSpace(stringFromAny(m["winning_source"])),
			DroppedSource:  strings.TrimSpace(stringFromAny(m["dropped_source"])),
		})
	}
	return out
}

func dedupeSkillConflicts(items []ProjectedSkillConflict) []ProjectedSkillConflict {
	if len(items) == 0 {
		return []ProjectedSkillConflict{}
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]ProjectedSkillConflict, 0, len(items))
	for _, item := range items {
		key := strings.Join([]string{
			item.Slug,
			item.Source,
			item.WinningSkillID,
			item.DroppedSkillID,
			item.WinningSource,
			item.DroppedSource,
		}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func recomputeCapabilityProjectionSummary(snap *CapabilityProjectionSnapshot) {
	bySource := map[string]int{}
	for _, s := range snap.Skills {
		scope := s.SourceScope
		if scope == "" {
			scope = "unknown"
		}
		bySource[scope]++
	}
	for _, m := range snap.MCPServers {
		scope := m.SourceScope
		if scope == "" {
			scope = "unknown"
		}
		bySource[scope]++
	}
	snap.Summary = CapabilityProjectionSummary{
		SkillCount:    len(snap.Skills),
		MCPCount:      len(snap.MCPServers),
		ConflictCount: len(snap.SkillConflicts),
		BySource:      bySource,
	}
}

// EnrichNames fills skill_name / conflict skill names from id→name.
// Missing names stay empty (UI falls back to skill_key).
func EnrichNames(snap *CapabilityProjectionSnapshot, namesByID map[uuid.UUID]string) {
	if snap == nil || len(namesByID) == 0 {
		return
	}
	for i := range snap.Skills {
		if id, err := uuid.Parse(snap.Skills[i].SkillID); err == nil {
			if name := strings.TrimSpace(namesByID[id]); name != "" {
				snap.Skills[i].SkillName = name
			}
		}
	}
	for i := range snap.SkillConflicts {
		if id, err := uuid.Parse(snap.SkillConflicts[i].WinningSkillID); err == nil {
			if name := strings.TrimSpace(namesByID[id]); name != "" {
				snap.SkillConflicts[i].WinningSkillName = name
			}
		}
		if id, err := uuid.Parse(snap.SkillConflicts[i].DroppedSkillID); err == nil {
			if name := strings.TrimSpace(namesByID[id]); name != "" {
				snap.SkillConflicts[i].DroppedSkillName = name
			}
		}
	}
}

// CollectSkillIDs gathers skill UUIDs for batch name lookup.
func CollectSkillIDs(snap CapabilityProjectionSnapshot) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	var out []uuid.UUID
	add := func(raw string) {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || id == uuid.Nil {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, s := range snap.Skills {
		add(s.SkillID)
	}
	for _, c := range snap.SkillConflicts {
		add(c.WinningSkillID)
		add(c.DroppedSkillID)
	}
	return out
}

// CapabilityProjectionSourceRow is one attempt's start_session receipt payload (may be empty).
type CapabilityProjectionSourceRow struct {
	AttemptID uuid.UUID
	CommandID string
	Payload   []byte
}

func stringFromAny(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}
