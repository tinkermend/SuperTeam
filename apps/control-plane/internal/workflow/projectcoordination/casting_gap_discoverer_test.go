package projectcoordination

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/superteam/control-plane/internal/project"
)

func gapTestRoles() []project.CastingGapRoleOption {
	return []project.CastingGapRoleOption{
		{RoleKey: "collector", Title: "采集", Description: "收集日志与指标"},
		{RoleKey: "analyst", Title: "分析", Description: "解读证据"},
		{RoleKey: "operator", Title: "处置", Description: "执行修复"},
	}
}

func gapTestInput() project.CastingGapInput {
	return project.CastingGapInput{
		TaskTitle:          "排查 API 超时",
		ConclusionSummary:  "应用侧无异常，疑似网络链路问题",
		DeliverableNames:   []string{"incident_report"},
		ActiveRoles:        gapTestRoles(),
		ParticipatingRoles: []string{"collector", "analyst"},
		Model:              "test-model",
	}
}

func TestCastingGapDiscoverer_NeededInVocab(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed": true, "role_key": "operator", "reason": "需要处置侧执行修复"}`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !got.Needed || got.External || got.RoleKey != "operator" {
		t.Fatalf("got %+v", got)
	}
	if !strings.Contains(got.Reason, "处置") {
		t.Fatalf("reason: %q", got.Reason)
	}
	if client.calls != 1 {
		t.Fatalf("calls=%d", client.calls)
	}
	if len(client.systems) == 0 || !strings.Contains(client.systems[0], "operator") {
		t.Fatalf("system prompt missing role list: %q", client.systems)
	}
}

func TestCastingGapDiscoverer_NotNeeded(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed": false}`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Needed {
		t.Fatalf("expected not needed, got %+v", got)
	}
}

func TestCastingGapDiscoverer_ExternalFlag(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed": true, "role_key": "", "external": true, "reason": "需要网络诊断"}`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !got.Needed || !got.External || got.RoleKey != "" {
		t.Fatalf("got %+v", got)
	}
	if got.Reason != "需要网络诊断" {
		t.Fatalf("reason: %q", got.Reason)
	}
}

func TestCastingGapDiscoverer_R1_UnknownKeyDemotedToExternal(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed": true, "role_key": "network_diagnostics", "reason": "需要网络侧核查"}`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Must not keep the fabricated key.
	if !got.Needed || !got.External || got.RoleKey != "" {
		t.Fatalf("R1 demotion failed: %+v", got)
	}
	if got.Reason != "需要网络侧核查" {
		t.Fatalf("reason must be preserved: %q", got.Reason)
	}
}

func TestCastingGapDiscoverer_R2_ParticipatingRoleDiscarded(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed": true, "role_key": "collector", "reason": "再采集一次"}`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got.Needed {
		t.Fatalf("R2 should discard: %+v", got)
	}
}

func TestCastingGapDiscoverer_R3_ParseFailureSilent(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `this is not json at all`},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err != nil {
		t.Fatalf("parse failure must not error: %v", err)
	}
	if got.Needed {
		t.Fatalf("R3 → needed=false, got %+v", got)
	}
}

func TestCastingGapDiscoverer_R3_MarkdownFence(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: "```json\n{\"needed\": true, \"role_key\": \"operator\", \"reason\": \"ok\"}\n```"},
	}}
	got, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !got.Needed || got.RoleKey != "operator" {
		t.Fatalf("got %+v", got)
	}
}

func TestCastingGapDiscoverer_TransportErrorPropagates(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{err: errors.New("upstream down")},
	}}
	_, err := runCastingGapDiscovery(context.Background(), client, "m", gapTestInput())
	if err == nil {
		t.Fatal("expected transport error")
	}
}

func TestNewCastingGapDiscoverer_NilClient(t *testing.T) {
	t.Parallel()
	if NewCastingGapDiscoverer(nil, "m") != nil {
		t.Fatal("nil client must yield nil discoverer")
	}
}

func TestCastingGapDiscoverer_Adapter(t *testing.T) {
	t.Parallel()
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: `{"needed": false}`},
	}}
	d := NewCastingGapDiscoverer(client, "m")
	got, err := d.DiscoverCastingGap(context.Background(), gapTestInput())
	if err != nil || got.Needed {
		t.Fatalf("got %+v err=%v", got, err)
	}
}
