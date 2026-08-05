//go:build live_e2e

package projectcoordination

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/superteam/control-plane/internal/project"
)

// Live H4-style smoke: real LLM + pure discoverer engine.
// Run:
//
//	DEEPSEEK_API_KEY=... go test -tags=live_e2e ./internal/workflow/projectcoordination/ \
//	  -run TestLiveCastingGapDiscoverer_ExternalNetwork -count=1 -timeout 3m
//
// Uses planner config defaults from env (same as production wiring).
func TestLiveCastingGapDiscoverer_ExternalNetwork(t *testing.T) {
	apiKey := firstNonEmpty(os.Getenv("DEEPSEEK_API_KEY"), os.Getenv("PLANNER_API_KEY"), os.Getenv("OPENAI_API_KEY"))
	baseURL := firstNonEmpty(os.Getenv("PLANNER_BASE_URL"), "https://api.deepseek.com/v1")
	model := firstNonEmpty(os.Getenv("PLANNER_MODEL"), "deepseek-v4-pro")
	if strings.TrimSpace(apiKey) == "" {
		t.Skip("no LLM api key in env")
	}

	client := NewOpenAICompatibleChatCompletionClient(baseURL, apiKey, 90*time.Second)
	in := project.CastingGapInput{
		TaskTitle:         "排查昨日 API 超时",
		ConclusionSummary: "应用侧无异常，疑似网络链路问题，需要网络侧进一步核查",
		DeliverableNames:  []string{"analysis_conclusion"},
		ActiveRoles: []project.CastingGapRoleOption{
			{RoleKey: "collector", Title: "采集"},
			{RoleKey: "analyst", Title: "分析"},
			{RoleKey: "developer", Title: "开发"},
			{RoleKey: "reviewer", Title: "审查"},
			{RoleKey: "tester", Title: "测试"},
			{RoleKey: "operator", Title: "处置"},
			{RoleKey: "diagnostician", Title: "诊断"},
			{RoleKey: "verifier", Title: "核验"},
			{RoleKey: "researcher", Title: "研究"},
			{RoleKey: "writer", Title: "写作"},
		},
		ParticipatingRoles: []string{"collector", "analyst"},
		Model:              model,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	got, err := runCastingGapDiscovery(ctx, client, model, in)
	if err != nil {
		t.Fatalf("discoverer call failed: %v", err)
	}
	t.Logf("suggestion=%+v", got)

	// Word表 has no network_* role → R1/external path is the correct product outcome.
	if !got.Needed {
		t.Fatalf("expected needed=true for clear network gap, got %+v", got)
	}
	if got.RoleKey != "" && !got.External {
		// Model mapped to an in-vocab role (H2-like). Accept if key is in list and not participating.
		allowed := map[string]bool{}
		for _, r := range in.ActiveRoles {
			allowed[r.RoleKey] = true
		}
		if !allowed[got.RoleKey] {
			t.Fatalf("suggested role_key %q not in vocab and not external", got.RoleKey)
		}
		if got.RoleKey == "collector" || got.RoleKey == "analyst" {
			t.Fatalf("R2 violated: suggested participating role %q", got.RoleKey)
		}
		t.Logf("H2-like in-vocab hit: %s", got.RoleKey)
		return
	}
	if !got.External {
		t.Fatalf("expected external=true (no network role in vocab), got %+v", got)
	}
	if got.RoleKey != "" {
		t.Fatalf("external path must not keep fabricated key, got %q", got.RoleKey)
	}
	if strings.TrimSpace(got.Reason) == "" {
		t.Fatalf("expected natural-language reason")
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
