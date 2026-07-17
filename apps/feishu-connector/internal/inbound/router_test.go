package inbound

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/superteam/feishu-connector/internal/cpclient"
	"github.com/superteam/feishu-connector/internal/gateway"
	"github.com/superteam/feishu-connector/internal/session"
)

type fakeCP struct {
	identities map[string]*cpclient.Identity
	projects   []cpclient.MyProject
	submitted  []cpclient.SubmitDemandRequest
	resolved   []string
	conflict   bool
}

func (f *fakeCP) ResolveIdentity(_ context.Context, _, openID string) (*cpclient.Identity, error) {
	return f.identities[openID], nil
}

func (f *fakeCP) MyProjects(_ context.Context, _ cpclient.OnBehalfOf) ([]cpclient.MyProject, error) {
	return f.projects, nil
}

func (f *fakeCP) SubmitDemand(_ context.Context, _ cpclient.OnBehalfOf, req cpclient.SubmitDemandRequest) (*cpclient.SubmitDemandResponse, error) {
	f.submitted = append(f.submitted, req)
	return &cpclient.SubmitDemandResponse{DemandID: "d-1", Title: req.Title, Status: "planning_pending"}, nil
}

func (f *fakeCP) ResolveDecision(_ context.Context, _ cpclient.OnBehalfOf, decisionID string, req cpclient.ResolveDecisionRequest) (bool, error) {
	f.resolved = append(f.resolved, decisionID+":"+req.Decision)
	return f.conflict, nil
}

type fakeMessenger struct {
	texts []string
	cards []string
}

func (m *fakeMessenger) SendText(_ context.Context, _ string, text string) (string, error) {
	m.texts = append(m.texts, text)
	return "om_text", nil
}

func (m *fakeMessenger) SendCard(_ context.Context, _ string, card string) (string, error) {
	m.cards = append(m.cards, card)
	return "om_card", nil
}

func setup(bound bool) (*Router, *fakeCP, *fakeMessenger, *session.Store) {
	cp := &fakeCP{identities: map[string]*cpclient.Identity{}}
	if bound {
		cp.identities["ou_a"] = &cpclient.Identity{AuthUserID: "user-a", OpenID: "ou_a"}
	}
	sessions := session.NewStore(time.Minute)
	router := NewRouter(cp, sessions, "cfg-1", "http://web.local:3000")
	messenger := &fakeMessenger{}
	router.SetMessenger(messenger)
	return router, cp, messenger, sessions
}

func msg(text string) gateway.InboundMessage {
	return gateway.InboundMessage{EventID: "e1", AppConfigID: "cfg-1", OpenID: "ou_a", MsgType: "text", Text: text}
}

func TestUnboundUserGetsBindGuide(t *testing.T) {
	router, _, messenger, _ := setup(false)
	router.HandleMessage(context.Background(), msg("你好"))
	if len(messenger.texts) != 1 || !strings.Contains(messenger.texts[0], "绑定") {
		t.Fatalf("expected bind guide, got %#v", messenger.texts)
	}
}

func TestPlainMessageGetsGuideCardWithoutSideEffects(t *testing.T) {
	router, cp, messenger, _ := setup(true)
	router.HandleMessage(context.Background(), msg("随便聊聊"))
	if len(messenger.cards) != 1 || !strings.Contains(messenger.cards[0], "提需求") {
		t.Fatalf("expected guide card, got %#v", messenger.cards)
	}
	if len(cp.submitted) != 0 {
		t.Fatalf("plain chat must not create demands")
	}
}

func TestDemandFlowEndToEnd(t *testing.T) {
	router, cp, messenger, _ := setup(true)
	cp.projects = []cpclient.MyProject{{ID: "p-1", Name: "支付项目"}}

	// 1. 提需求 → 项目选择卡
	router.HandleMessage(context.Background(), msg("提需求"))
	if len(messenger.cards) != 1 || !strings.Contains(messenger.cards[0], "pick_project") {
		t.Fatalf("expected project pick card, got %#v", messenger.cards)
	}

	// 2. 选项目 → 模式卡
	reply := router.HandleCardAction(context.Background(), gateway.CardAction{
		AppConfigID: "cfg-1", OpenID: "ou_a",
		Value: map[string]any{"action": "pick_project", "project_id": "p-1", "project_name": "支付项目"},
	})
	if reply.ToastType != "success" {
		t.Fatalf("pick project toast: %#v", reply)
	}

	// 3. 选模式 → 等内容
	reply = router.HandleCardAction(context.Background(), gateway.CardAction{
		AppConfigID: "cfg-1", OpenID: "ou_a",
		Value: map[string]any{"action": "pick_mode", "mode": "plan"},
	})
	if reply.ToastType != "success" {
		t.Fatalf("pick mode toast: %#v", reply)
	}

	// 4. 发送内容 → 确认卡
	router.HandleMessage(context.Background(), msg("加固登录接口\n对登录接口做限流与验证码加固"))
	found := false
	for _, card := range messenger.cards {
		if strings.Contains(card, "confirm_demand") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected confirm card, got %#v", messenger.cards)
	}

	// 5. 确认 → 提交
	reply = router.HandleCardAction(context.Background(), gateway.CardAction{
		AppConfigID: "cfg-1", OpenID: "ou_a",
		Value: map[string]any{"action": "confirm_demand"},
	})
	if reply.ToastType != "success" {
		t.Fatalf("confirm toast: %#v", reply)
	}
	if len(cp.submitted) != 1 {
		t.Fatalf("expected one demand submitted, got %d", len(cp.submitted))
	}
	got := cp.submitted[0]
	if got.ProjectID != "p-1" || got.Title != "加固登录接口" || got.CoordinationMode != "plan" ||
		!strings.Contains(got.Content, "限流") {
		t.Fatalf("unexpected demand %#v", got)
	}
}

func TestExpiredSessionCardActionAsksRestart(t *testing.T) {
	router, _, _, _ := setup(true)
	reply := router.HandleCardAction(context.Background(), gateway.CardAction{
		AppConfigID: "cfg-1", OpenID: "ou_a",
		Value: map[string]any{"action": "pick_mode", "mode": "plan"},
	})
	if reply.ToastType != "warning" || !strings.Contains(reply.ToastContent, "提需求") {
		t.Fatalf("expected restart hint, got %#v", reply)
	}
}

func TestResolveDecisionActionAndConflict(t *testing.T) {
	router, cp, _, _ := setup(true)
	action := gateway.CardAction{
		AppConfigID: "cfg-1", OpenID: "ou_a",
		Value: map[string]any{"action": "resolve_decision", "decision_id": "dec-1", "project_id": "p-1", "decision": "approved"},
	}
	reply := router.HandleCardAction(context.Background(), action)
	if reply.ToastType != "success" || len(cp.resolved) != 1 || cp.resolved[0] != "dec-1:approved" {
		t.Fatalf("resolve failed: %#v resolved=%v", reply, cp.resolved)
	}

	cp.conflict = true
	reply = router.HandleCardAction(context.Background(), action)
	if reply.ToastType != "info" || !strings.Contains(reply.ToastContent, "已由他人处理") {
		t.Fatalf("expected conflict toast, got %#v", reply)
	}
}

func TestCancelClearsSession(t *testing.T) {
	router, cp, messenger, sessions := setup(true)
	cp.projects = []cpclient.MyProject{{ID: "p-1", Name: "支付项目"}}
	router.HandleMessage(context.Background(), msg("提需求"))
	router.HandleMessage(context.Background(), msg("取消"))
	if _, ok := sessions.Get("ou_a"); ok {
		t.Fatalf("expected session cleared")
	}
	last := messenger.texts[len(messenger.texts)-1]
	if !strings.Contains(last, "已取消") {
		t.Fatalf("expected cancel ack, got %q", last)
	}
}
