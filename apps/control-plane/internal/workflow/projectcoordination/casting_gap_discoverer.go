package projectcoordination

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/superteam/control-plane/internal/project"
)

// CastingGapDiscoverer is the optional LLM seam that reads a completed task's
// output and, constrained to the tenant role vocabulary, suggests whether
// casting should expand (design 2026-08-05 §3). Unwired → silent skip.
type CastingGapDiscoverer interface {
	DiscoverCastingGap(ctx context.Context, in project.CastingGapInput) (project.CastingGapSuggestion, error)
}

// castingGapDiscoverer is the production implementation over chatCompletionClient.
type castingGapDiscoverer struct {
	client chatCompletionClient
	model  string
}

// NewCastingGapDiscoverer wires the pure engine to a chat client. model may be
// empty (server-side default). nil client → nil discoverer (callers skip).
func NewCastingGapDiscoverer(client chatCompletionClient, model string) CastingGapDiscoverer {
	if client == nil {
		return nil
	}
	return &castingGapDiscoverer{client: client, model: strings.TrimSpace(model)}
}

func (d *castingGapDiscoverer) DiscoverCastingGap(ctx context.Context, in project.CastingGapInput) (project.CastingGapSuggestion, error) {
	if d == nil || d.client == nil {
		return project.CastingGapSuggestion{Needed: false}, nil
	}
	return runCastingGapDiscovery(ctx, d.client, d.model, in)
}

// runCastingGapDiscovery is the pure engine: one constrained LLM call + R1–R3
// server-side validation. DB-free and unit-testable with a fake client.
//
// R1: role_key not in the injected list → demote to external (do not drop).
// R2: already-participating role → discard (needed=false).
// R3: parse failure → needed=false (suggestion, not a gate).
func runCastingGapDiscovery(ctx context.Context, client chatCompletionClient, model string, in project.CastingGapInput) (project.CastingGapSuggestion, error) {
	if client == nil {
		return project.CastingGapSuggestion{Needed: false}, nil
	}
	if err := ctx.Err(); err != nil {
		return project.CastingGapSuggestion{}, err
	}
	allowed := make(map[string]bool, len(in.ActiveRoles))
	for _, r := range in.ActiveRoles {
		key := strings.TrimSpace(r.RoleKey)
		if key != "" {
			allowed[key] = true
		}
	}
	participating := make(map[string]bool, len(in.ParticipatingRoles))
	for _, r := range in.ParticipatingRoles {
		key := strings.TrimSpace(r)
		if key != "" {
			participating[key] = true
		}
	}
	content, err := client.CreateChatCompletion(ctx, OpenAICompatibleChatRequest{
		Model:  model,
		System: buildCastingGapSystemPrompt(in.ActiveRoles, in.ParticipatingRoles),
		User:   buildCastingGapUserPrompt(in),
	})
	if err != nil {
		// Transport failure is a genuine error; callers log+swallow so the
		// task graph is never blocked by a suggestion path.
		return project.CastingGapSuggestion{}, fmt.Errorf("casting gap discoverer call failed: %w", err)
	}
	return parseCastingGapResponse(content, allowed, participating), nil
}

func buildCastingGapSystemPrompt(roles []project.CastingGapRoleOption, participating []string) string {
	var b strings.Builder
	b.WriteString("你在评估一个刚完成的任务产出，判断为了把这一单继续做下去，\n")
	b.WriteString("是否还需要某个**当前未参与**的角色补充进来。\n\n")
	b.WriteString("可选角色仅限以下清单（role_key + 中文名 + 说明）：\n")
	if len(roles) == 0 {
		b.WriteString("  （无可用角色）\n")
	} else {
		for _, r := range roles {
			key := strings.TrimSpace(r.RoleKey)
			if key == "" {
				continue
			}
			title := strings.TrimSpace(r.Title)
			desc := strings.TrimSpace(r.Description)
			line := "  - " + key
			if title != "" {
				line += "（" + title + "）"
			}
			if desc != "" {
				line += "：" + desc
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n已参与本单的角色：")
	if len(participating) == 0 {
		b.WriteString("（无）\n")
	} else {
		b.WriteString(strings.Join(participating, ", "))
		b.WriteByte('\n')
	}
	b.WriteString("\n只返回一个 JSON 对象，不要包裹在 markdown 中：\n")
	b.WriteString(`  {"needed": false}` + "\n")
	b.WriteString(`  {"needed": true, "role_key": "<清单中的 key>", "reason": "一句话"}` + "\n")
	b.WriteString(`  {"needed": true, "role_key": "", "external": true, "reason": "一句话"}` + "\n")
	b.WriteString("role_key 必须在清单内；已参与的角色不得再被建议；不确定时返回 needed=false。\n")
	return b.String()
}

func buildCastingGapUserPrompt(in project.CastingGapInput) string {
	payload := castingGapUserPayload{
		TaskTitle:          strings.TrimSpace(in.TaskTitle),
		ConclusionSummary:  strings.TrimSpace(in.ConclusionSummary),
		DeliverableNames:   in.DeliverableNames,
		ParticipatingRoles: in.ParticipatingRoles,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte("{}")
	}
	return fmt.Sprintf("刚完成的任务产出如下。判断是否需要词表内的某个未参与角色补充参与。只返回 JSON。\n%s", string(body))
}

type castingGapUserPayload struct {
	TaskTitle          string   `json:"task_title"`
	ConclusionSummary  string   `json:"conclusion_summary"`
	DeliverableNames   []string `json:"deliverable_names,omitempty"`
	ParticipatingRoles []string `json:"participating_roles,omitempty"`
}

type castingGapModelResponse struct {
	Needed   bool   `json:"needed"`
	RoleKey  string `json:"role_key"`
	External bool   `json:"external"`
	Reason   string `json:"reason"`
}

// parseCastingGapResponse applies R1–R3. Never returns an error for bad content.
func parseCastingGapResponse(content string, allowed, participating map[string]bool) project.CastingGapSuggestion {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return project.CastingGapSuggestion{Needed: false} // R3
	}
	// Strip optional markdown fences the model sometimes still emits.
	if strings.HasPrefix(trimmed, "```") {
		trimmed = stripMarkdownFence(trimmed)
	}
	var raw castingGapModelResponse
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		// Try to extract a JSON object substring.
		if obj := extractJSONObject(trimmed); obj != "" {
			if err2 := json.Unmarshal([]byte(obj), &raw); err2 != nil {
				return project.CastingGapSuggestion{Needed: false} // R3
			}
		} else {
			return project.CastingGapSuggestion{Needed: false} // R3
		}
	}
	if !raw.Needed {
		return project.CastingGapSuggestion{Needed: false}
	}
	reason := strings.TrimSpace(raw.Reason)
	roleKey := strings.TrimSpace(raw.RoleKey)

	if raw.External || roleKey == "" {
		return project.CastingGapSuggestion{
			Needed:   true,
			External: true,
			Reason:   reason,
		}
	}

	// R2: already participating → discard.
	if participating[roleKey] {
		return project.CastingGapSuggestion{Needed: false}
	}

	// R1: not in vocabulary → demote to external (keep reason, drop fake key).
	if !allowed[roleKey] {
		return project.CastingGapSuggestion{
			Needed:   true,
			External: true,
			Reason:   reason,
		}
	}

	return project.CastingGapSuggestion{
		Needed:  true,
		RoleKey: roleKey,
		Reason:  reason,
	}
}

func stripMarkdownFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	// drop first line (``` or ```json)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	if j := strings.LastIndex(s, "```"); j >= 0 {
		s = s[:j]
	}
	return strings.TrimSpace(s)
}

func extractJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end <= start {
		return ""
	}
	return s[start : end+1]
}
