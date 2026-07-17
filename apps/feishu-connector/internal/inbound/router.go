// Package inbound 处理入站:私聊消息与卡片动作。显式意图规则——普通消息只回
// 引导,不隐式创建任何业务对象;提需求必须经 项目选择→模式选择→内容→确认 四步。
package inbound

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/superteam/feishu-connector/internal/cards"
	"github.com/superteam/feishu-connector/internal/cpclient"
	"github.com/superteam/feishu-connector/internal/gateway"
	"github.com/superteam/feishu-connector/internal/session"
)

// Messenger 是出站消息的最小接口(gateway 实现;测试用假实现)。
type Messenger interface {
	SendCard(ctx context.Context, openID, cardJSON string) (string, error)
	SendText(ctx context.Context, openID, text string) (string, error)
}

// ControlPlane 是路由依赖的控制平面动作(cpclient 实现;测试用假实现)。
type ControlPlane interface {
	ResolveIdentity(ctx context.Context, appConfigID, openID string) (*cpclient.Identity, error)
	MyProjects(ctx context.Context, obo cpclient.OnBehalfOf) ([]cpclient.MyProject, error)
	SubmitDemand(ctx context.Context, obo cpclient.OnBehalfOf, req cpclient.SubmitDemandRequest) (*cpclient.SubmitDemandResponse, error)
	ResolveDecision(ctx context.Context, obo cpclient.OnBehalfOf, decisionID string, req cpclient.ResolveDecisionRequest) (bool, error)
}

type Router struct {
	cp          ControlPlane
	sessions    *session.Store
	appConfigID string
	webOrigin   string
	messenger   Messenger
}

func NewRouter(cp ControlPlane, sessions *session.Store, appConfigID, webOrigin string) *Router {
	return &Router{cp: cp, sessions: sessions, appConfigID: appConfigID, webOrigin: strings.TrimRight(webOrigin, "/")}
}

func (r *Router) SetMessenger(m Messenger) { r.messenger = m }

const (
	cmdNewDemand = "提需求"
	cmdCancel    = "取消"
)

// HandleMessage 私聊消息入口。
func (r *Router) HandleMessage(ctx context.Context, msg gateway.InboundMessage) {
	if r.messenger == nil {
		return
	}
	identity, err := r.cp.ResolveIdentity(ctx, msg.AppConfigID, msg.OpenID)
	if err != nil {
		log.Printf("[inbound] resolve identity: %v", err)
		return
	}
	if identity == nil {
		r.sendText(ctx, msg.OpenID, "你的飞书还未绑定平台账号。请登录 Console 的用户管理页,点击「绑定我的飞书」完成绑定后再来找我。")
		return
	}
	obo := cpclient.OnBehalfOf{UserID: identity.AuthUserID, OpenID: msg.OpenID}
	text := strings.TrimSpace(msg.Text)

	if text == cmdCancel {
		r.sessions.Clear(msg.OpenID)
		r.sendText(ctx, msg.OpenID, "已取消当前操作。")
		return
	}

	state, hasSession := r.sessions.Get(msg.OpenID)
	if hasSession && state.Stage == session.StageAwaitContent {
		r.captureDemandContent(ctx, msg.OpenID, state, text)
		return
	}

	if strings.HasPrefix(text, cmdNewDemand) || strings.HasPrefix(text, "/"+cmdNewDemand) {
		r.startDemandFlow(ctx, msg.OpenID, obo)
		return
	}

	// 显式意图规则:普通消息只回引导,不创建业务对象。
	r.sendCard(ctx, msg.OpenID, cards.GuideCard())
}

func (r *Router) startDemandFlow(ctx context.Context, openID string, obo cpclient.OnBehalfOf) {
	projects, err := r.cp.MyProjects(ctx, obo)
	if err != nil {
		log.Printf("[inbound] list projects: %v", err)
		r.sendText(ctx, openID, "拉取你的项目列表失败,请稍后再试或改用 Console。")
		return
	}
	if len(projects) == 0 {
		r.sendText(ctx, openID, "你还不是任何项目的人类成员,无法发起需求。请先在 Console 加入项目。")
		return
	}
	r.sessions.Put(openID, session.FormState{Stage: session.StagePickProject, UserID: obo.UserID})
	r.sendCard(ctx, openID, cards.ProjectPickCard(projects))
}

func (r *Router) captureDemandContent(ctx context.Context, openID string, state session.FormState, text string) {
	if text == "" {
		r.sendText(ctx, openID, "需求内容不能为空。第一行是标题,其余是详细内容;发送「取消」放弃。")
		return
	}
	lines := strings.SplitN(text, "\n", 2)
	state.Title = strings.TrimSpace(lines[0])
	if len(lines) > 1 {
		state.Content = strings.TrimSpace(lines[1])
	} else {
		state.Content = state.Title
	}
	state.Stage = session.StageConfirm
	r.sessions.Put(openID, state)
	r.sendCard(ctx, openID, cards.DemandConfirmCard(state.ProjectName, state.Mode, state.Title, state.Content))
}

// HandleCardAction 卡片按钮回调入口。
func (r *Router) HandleCardAction(ctx context.Context, action gateway.CardAction) gateway.CardActionReply {
	kind, _ := action.Value["action"].(string)
	identity, err := r.cp.ResolveIdentity(ctx, action.AppConfigID, action.OpenID)
	if err != nil {
		return gateway.CardActionReply{ToastType: "error", ToastContent: "系统繁忙,请稍后再试"}
	}
	if identity == nil {
		return gateway.CardActionReply{ToastType: "warning", ToastContent: "飞书未绑定平台账号,请先到 Console 绑定"}
	}
	obo := cpclient.OnBehalfOf{UserID: identity.AuthUserID, OpenID: action.OpenID}

	switch kind {
	case "pick_project":
		return r.onPickProject(action)
	case "pick_mode":
		return r.onPickMode(ctx, action)
	case "confirm_demand":
		return r.onConfirmDemand(ctx, action, obo)
	case "cancel_demand":
		r.sessions.Clear(action.OpenID)
		return gateway.CardActionReply{ToastType: "info", ToastContent: "已取消"}
	case "resolve_decision":
		return r.onResolveDecision(ctx, action, obo)
	default:
		return gateway.CardActionReply{}
	}
}

func (r *Router) onPickProject(action gateway.CardAction) gateway.CardActionReply {
	projectID, _ := action.Value["project_id"].(string)
	projectName, _ := action.Value["project_name"].(string)
	state, ok := r.sessions.Get(action.OpenID)
	if !ok || state.Stage != session.StagePickProject || projectID == "" {
		return gateway.CardActionReply{ToastType: "warning", ToastContent: "会话已过期,请重新发送「提需求」"}
	}
	state.ProjectID = projectID
	state.ProjectName = projectName
	state.Stage = session.StagePickMode
	r.sessions.Put(action.OpenID, state)
	go r.sendCard(context.Background(), action.OpenID, cards.ModePickCard(projectName))
	return gateway.CardActionReply{ToastType: "success", ToastContent: "已选择项目 " + projectName}
}

func (r *Router) onPickMode(ctx context.Context, action gateway.CardAction) gateway.CardActionReply {
	mode, _ := action.Value["mode"].(string)
	state, ok := r.sessions.Get(action.OpenID)
	if !ok || state.Stage != session.StagePickMode || (mode != "plan" && mode != "loop") {
		return gateway.CardActionReply{ToastType: "warning", ToastContent: "会话已过期,请重新发送「提需求」"}
	}
	state.Mode = mode
	state.Stage = session.StageAwaitContent
	r.sessions.Put(action.OpenID, state)
	go r.sendText(context.Background(), action.OpenID, "请直接发送需求内容:第一行是标题,其余是详细描述。发送「取消」放弃。")
	return gateway.CardActionReply{ToastType: "success", ToastContent: "协调模式:" + mode}
}

func (r *Router) onConfirmDemand(ctx context.Context, action gateway.CardAction, obo cpclient.OnBehalfOf) gateway.CardActionReply {
	state, ok := r.sessions.Get(action.OpenID)
	if !ok || state.Stage != session.StageConfirm {
		return gateway.CardActionReply{ToastType: "warning", ToastContent: "会话已过期,请重新发送「提需求」"}
	}
	resp, err := r.cp.SubmitDemand(ctx, obo, cpclient.SubmitDemandRequest{
		ProjectID:        state.ProjectID,
		Title:            state.Title,
		Content:          state.Content,
		CoordinationMode: state.Mode,
	})
	if err != nil {
		log.Printf("[inbound] submit demand: %v", err)
		return gateway.CardActionReply{ToastType: "error", ToastContent: "需求提交失败,请稍后再试或改用 Console"}
	}
	r.sessions.Clear(action.OpenID)
	deepLink := fmt.Sprintf("%s/workflows/%s", r.webOrigin, resp.DemandID)
	go r.sendCard(context.Background(), action.OpenID, cards.DemandReceiptCard(state.Title, resp.DemandID, deepLink))
	return gateway.CardActionReply{ToastType: "success", ToastContent: "需求已提交"}
}

func (r *Router) onResolveDecision(ctx context.Context, action gateway.CardAction, obo cpclient.OnBehalfOf) gateway.CardActionReply {
	decisionID, _ := action.Value["decision_id"].(string)
	projectID, _ := action.Value["project_id"].(string)
	decision, _ := action.Value["decision"].(string)
	if decisionID == "" || projectID == "" || decision == "" {
		return gateway.CardActionReply{ToastType: "error", ToastContent: "卡片数据缺失"}
	}
	comment := strings.TrimSpace(action.InputValue)
	if comment == "" && action.FormValue != nil {
		if v, ok := action.FormValue["comment"].(string); ok {
			comment = strings.TrimSpace(v)
		}
	}
	conflict, err := r.cp.ResolveDecision(ctx, obo, decisionID, cpclient.ResolveDecisionRequest{
		ProjectID: projectID,
		Decision:  decision,
		Comment:   comment,
	})
	if err != nil {
		log.Printf("[inbound] resolve decision: %v", err)
		return gateway.CardActionReply{ToastType: "error", ToastContent: "处理失败,请稍后再试或到 Console 处理"}
	}
	title, _ := action.Value["title"].(string)
	if conflict {
		// 已由他人(或本人此前)处理:同步置换为已处理卡,终结可点状态。
		return gateway.CardActionReply{
			ToastType:    "info",
			ToastContent: "该决策已被处理",
			NewCardJSON:  cards.DecisionResolvedCard(map[string]any{"title": title}),
		}
	}
	// 点击瞬间同步置换已处理卡,消除按钮重复可点的时间窗;
	// 其他收件人的卡由 outbox card_update 兜底更新(any-of-N)。
	return gateway.CardActionReply{
		ToastType:    "success",
		ToastContent: "已提交:" + decision,
		NewCardJSON:  cards.DecisionResolvedCard(map[string]any{"title": title, "resolved_status": decision}),
	}
}

func (r *Router) sendText(ctx context.Context, openID, text string) {
	if _, err := r.messenger.SendText(ctx, openID, text); err != nil {
		log.Printf("[inbound] send text: %v", err)
	}
}

func (r *Router) sendCard(ctx context.Context, openID, card string) {
	if _, err := r.messenger.SendCard(ctx, openID, card); err != nil {
		log.Printf("[inbound] send card: %v", err)
	}
}
