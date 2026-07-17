// Package gateway 封装飞书长连接与消息 API:SDK 事件在此翻译成本进程自有结构,
// 业务处理(inbound/outbound)不直接依赖 lark SDK 类型,便于测试与替换。
//
// 长连接事实(官方口径):集群模式随机分发不广播、at-least-once 必须按
// event_id 去重、事件 3 秒内须处理完否则重推——因此 handler 只做
// 去重+入队+立即返回,业务在 worker goroutine 池异步消费。
package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/superteam/feishu-connector/internal/dedup"
)

// InboundMessage 是收到的私聊消息(已去重)。
type InboundMessage struct {
	EventID     string
	AppConfigID string
	OpenID      string
	ChatID      string
	MsgType     string
	Text        string
}

// CardAction 是卡片按钮回调(已去重)。Value 携带业务路由键(action/decision_id 等)。
type CardAction struct {
	EventID     string
	AppConfigID string
	OpenID      string
	MessageID   string
	Value       map[string]any
	InputValue  string
	FormValue   map[string]any
}

// CardActionReply 是回调的同步应答(toast 提示)。
type CardActionReply struct {
	ToastType    string
	ToastContent string
}

// Handler 由 inbound 路由实现;必须快速返回(重活自行异步)。
type Handler interface {
	HandleMessage(ctx context.Context, msg InboundMessage)
	HandleCardAction(ctx context.Context, action CardAction) CardActionReply
}

// Gateway 维护一个飞书 app 的长连接与出站消息能力。
type Gateway struct {
	appConfigID string
	wsClient    *larkws.Client
	apiClient   *lark.Client
	dedupSet    *dedup.Set
	handler     Handler
	queue       chan func()
}

const inboundQueueSize = 256

func New(appConfigID, appID, appSecret string, handler Handler) *Gateway {
	g := &Gateway{
		appConfigID: appConfigID,
		apiClient:   lark.NewClient(appID, appSecret),
		dedupSet:    dedup.New(8192),
		handler:     handler,
		queue:       make(chan func(), inboundQueueSize),
	}
	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(func(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
			g.enqueueMessage(event)
			return nil
		}).
		OnP2CardActionTrigger(func(ctx context.Context, event *callback.CardActionTriggerEvent) (*callback.CardActionTriggerResponse, error) {
			return g.handleCardAction(ctx, event), nil
		})
	g.wsClient = larkws.NewClient(appID, appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)
	return g
}

// Start 建立长连接并启动异步消费(阻塞;断线由 SDK 自动重连)。
func (g *Gateway) Start(ctx context.Context, workers int) error {
	if workers <= 0 {
		workers = 4
	}
	for i := 0; i < workers; i++ {
		go func() {
			for job := range g.queue {
				job()
			}
		}()
	}
	return g.wsClient.Start(ctx)
}

func (g *Gateway) enqueueMessage(event *larkim.P2MessageReceiveV1) {
	if event == nil || event.Event == nil || event.Event.Message == nil || event.Event.Sender == nil ||
		event.Event.Sender.SenderId == nil || event.Event.Sender.SenderId.OpenId == nil {
		return
	}
	eventID := ""
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		eventID = event.EventV2Base.Header.EventID
	}
	if g.dedupSet.Seen(eventID) {
		return
	}
	msg := InboundMessage{
		EventID:     eventID,
		AppConfigID: g.appConfigID,
		OpenID:      *event.Event.Sender.SenderId.OpenId,
	}
	if event.Event.Message.ChatId != nil {
		msg.ChatID = *event.Event.Message.ChatId
	}
	if event.Event.Message.MessageType != nil {
		msg.MsgType = *event.Event.Message.MessageType
	}
	if event.Event.Message.Content != nil {
		msg.Text = extractText(*event.Event.Message.Content)
	}
	select {
	case g.queue <- func() { g.handler.HandleMessage(context.Background(), msg) }:
	default:
		log.Printf("[gateway] inbound queue full, dropping event %s", eventID)
	}
}

// handleCardAction 同步应答 toast(3 秒窗口内轻量),业务动作在 handler 内完成——
// resolve 是一次控制平面 HTTP 调用,量级安全;卡片更新走 outbox 的 card_update。
func (g *Gateway) handleCardAction(ctx context.Context, event *callback.CardActionTriggerEvent) *callback.CardActionTriggerResponse {
	if event == nil || event.Event == nil || event.Event.Operator == nil || event.Event.Action == nil {
		return &callback.CardActionTriggerResponse{}
	}
	eventID := ""
	if event.EventV2Base != nil && event.EventV2Base.Header != nil {
		eventID = event.EventV2Base.Header.EventID
	}
	if g.dedupSet.Seen(eventID) {
		return &callback.CardActionTriggerResponse{}
	}
	action := CardAction{
		EventID:     eventID,
		AppConfigID: g.appConfigID,
		OpenID:      event.Event.Operator.OpenID,
		Value:       event.Event.Action.Value,
		InputValue:  event.Event.Action.InputValue,
		FormValue:   event.Event.Action.FormValue,
	}
	if event.Event.Context != nil {
		action.MessageID = event.Event.Context.OpenMessageID
	}
	reply := g.handler.HandleCardAction(ctx, action)
	resp := &callback.CardActionTriggerResponse{}
	if reply.ToastContent != "" {
		toastType := reply.ToastType
		if toastType == "" {
			toastType = "info"
		}
		resp.Toast = &callback.Toast{Type: toastType, Content: reply.ToastContent}
	}
	return resp
}

// SendCard 发送交互卡片,返回 message_id(用于后续更新)。
func (g *Gateway) SendCard(ctx context.Context, openID, cardJSON string) (string, error) {
	return g.sendMessage(ctx, openID, "interactive", cardJSON)
}

// SendText 发送文本消息。
func (g *Gateway) SendText(ctx context.Context, openID, text string) (string, error) {
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", err
	}
	return g.sendMessage(ctx, openID, "text", string(content))
}

func (g *Gateway) sendMessage(ctx context.Context, openID, msgType, content string) (string, error) {
	resp, err := g.apiClient.Im.Message.Create(ctx, larkim.NewCreateMessageReqBuilder().
		ReceiveIdType("open_id").
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(openID).
			MsgType(msgType).
			Content(content).
			Build()).
		Build())
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", fmt.Errorf("feishu send message failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.MessageId == nil {
		return "", fmt.Errorf("feishu send message: missing message id")
	}
	return *resp.Data.MessageId, nil
}

// UpdateCard 整卡替换更新(决策已处理态)。
func (g *Gateway) UpdateCard(ctx context.Context, messageID, cardJSON string) error {
	resp, err := g.apiClient.Im.Message.Patch(ctx, larkim.NewPatchMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(cardJSON).
			Build()).
		Build())
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("feishu update card failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
}

func extractText(content string) string {
	var parsed struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return ""
	}
	return parsed.Text
}
