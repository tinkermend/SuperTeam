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
	identities  map[string]*cpclient.Identity
	projects    []cpclient.MyProject
	submitted   []cpclient.SubmitDemandRequest
	resolved    []string
	conflict    bool
	resolveCard map[string]any
	signed      []string
	signOutcome *cpclient.SignCriterionOutcome
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

func (f *fakeCP) ResolveDecision(_ context.Context, _ cpclient.OnBehalfOf, decisionID string, req cpclient.ResolveDecisionRequest) (map[string]any, bool, error) {
	f.resolved = append(f.resolved, decisionID+":"+req.Decision)
	return f.resolveCard, f.conflict, nil
}

func (f *fakeCP) SignCriterion(_ context.Context, _ cpclient.OnBehalfOf, demandID string, req cpclient.SignCriterionRequest) (*cpclient.SignCriterionOutcome, error) {
	f.signed = append(f.signed, demandID+":"+req.CriterionID+":"+req.Verdict+":"+req.Reason)
	if f.signOutcome != nil {
		return f.signOutcome, nil
	}
	return &cpclient.SignCriterionOutcome{DemandStatus: "acceptance_pending", Signed: 1, Total: 2, Remaining: 1}, nil
}

type fakeMessenger struct {
	texts   []string
	cards   []string
	updates map[string]string
}

func (m *fakeMessenger) UpdateCard(_ context.Context, messageID, card string) error {
	if m.updates == nil {
		m.updates = map[string]string{}
	}
	m.updates[messageID] = card
	return nil
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
	// 点击瞬间同步置换已处理卡:按钮不可再点(修复"批准可来回点")。
	if reply.NewCardJSON == "" || !strings.Contains(reply.NewCardJSON, "已处理") || strings.Contains(reply.NewCardJSON, "resolve_decision") {
		t.Fatalf("expected inert resolved card in sync reply, got %s", reply.NewCardJSON)
	}

	cp.conflict = true
	reply = router.HandleCardAction(context.Background(), action)
	if reply.ToastType != "info" || !strings.Contains(reply.ToastContent, "已被处理") {
		t.Fatalf("expected conflict toast, got %#v", reply)
	}
	if reply.NewCardJSON == "" {
		t.Fatalf("conflict reply must also replace the card")
	}
}

// 卡内签署「通过」:直接提交并整卡重渲染进度。
func TestSignCriterionSatisfiedRerendersProgress(t *testing.T) {
	router, cp, _, _ := setup(true)
	cp.signOutcome = &cpclient.SignCriterionOutcome{
		DemandStatus: "acceptance_pending", Signed: 1, Total: 2, Remaining: 1,
		CriterionVerdicts: map[string]string{"c1": "satisfied"},
		CardPayload: map[string]any{
			"decision_type": "demand_acceptance", "title": "需求验收:登录加固",
			"context": map[string]any{
				"demand_id": "d-1",
				"pending_criteria_detail": []any{
					map[string]any{"id": "c1", "statement": "接口通过安全扫描"},
					map[string]any{"id": "c2", "statement": "限流开关可回滚"},
				},
			},
		},
	}
	reply := router.HandleCardAction(context.Background(), gateway.CardAction{
		AppConfigID: "cfg-1", OpenID: "ou_a", MessageID: "om_1",
		Value: map[string]any{"action": "sign_criterion", "demand_id": "d-1", "project_id": "p-1", "decision_id": "dec-1", "criterion_id": "c1", "verdict": "satisfied"},
	})
	if reply.ToastType != "success" || len(cp.signed) != 1 || cp.signed[0] != "d-1:c1:satisfied:" {
		t.Fatalf("sign failed: %#v signed=%v", reply, cp.signed)
	}
	// 重渲染:已签 ✅、未签保留按钮、进度行在卡上。
	for _, want := range []string{"1/2", "✅", "接口通过安全扫描", "限流开关可回滚", "sign_criterion"} {
		if !strings.Contains(reply.NewCardJSON, want) {
			t.Fatalf("progress card missing %q:\n%s", want, reply.NewCardJSON)
		}
	}
}

// 卡内签署「不通过」:理由必填——先入会话态,回复文本后提交并回头更新原卡。
func TestSignCriterionUnsatisfiedRequiresReason(t *testing.T) {
	router, cp, messenger, sessions := setup(true)
	cp.signOutcome = &cpclient.SignCriterionOutcome{
		DemandStatus: "failed", Signed: 2, Total: 2, Remaining: 0,
		CriterionVerdicts: map[string]string{"c2": "unsatisfied"},
		CardPayload: map[string]any{
			"decision_type": "demand_acceptance", "title": "需求验收:登录加固",
			"context": map[string]any{"demand_id": "d-1", "pending_criteria_detail": []any{
				map[string]any{"id": "c2", "statement": "限流开关可回滚"},
			}},
		},
	}
	reply := router.HandleCardAction(context.Background(), gateway.CardAction{
		AppConfigID: "cfg-1", OpenID: "ou_a", MessageID: "om_1",
		Value: map[string]any{"action": "sign_criterion", "demand_id": "d-1", "project_id": "p-1", "decision_id": "dec-1", "criterion_id": "c2", "verdict": "unsatisfied", "statement": "限流开关可回滚"},
	})
	if len(cp.signed) != 0 {
		t.Fatalf("unsatisfied must not sign before reason, signed=%v", cp.signed)
	}
	if state, ok := sessions.Get("ou_a"); !ok || state.Stage != session.StageAwaitRejectReason || state.CardMessageID != "om_1" {
		t.Fatalf("expected await-reason session, got %#v ok=%v", state, ok)
	}
	if reply.ToastType != "info" {
		t.Fatalf("expected info toast, got %#v", reply)
	}

	// 空理由被拒
	router.HandleMessage(context.Background(), msg(""))
	if len(cp.signed) != 0 {
		t.Fatalf("empty reason must not sign")
	}
	// 回复理由 → 提交 unsatisfied + 理由;原卡经 UpdateCard 更新为终态;会话清空。
	router.HandleMessage(context.Background(), msg("回滚开关没接入配置中心"))
	if len(cp.signed) != 1 || cp.signed[0] != "d-1:c2:unsatisfied:回滚开关没接入配置中心" {
		t.Fatalf("expected unsatisfied sign with reason, signed=%v", cp.signed)
	}
	if card, ok := messenger.updates["om_1"]; !ok || !strings.Contains(card, "验收未通过") {
		t.Fatalf("expected original card updated to failed state, updates=%v", messenger.updates)
	}
	if _, ok := sessions.Get("ou_a"); ok {
		t.Fatalf("expected session cleared after reason submitted")
	}
	last := messenger.texts[len(messenger.texts)-1]
	if !strings.Contains(last, "不通过") {
		t.Fatalf("expected reject confirmation text, got %q", last)
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
