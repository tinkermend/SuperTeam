// Package outbound 消费控制平面 feishu_outbox:渲染卡片→投递→ack。
// 投递失败 ack failed(控制平面计数,3 次终态);connector 崩溃未 ack 的行
// 保持 pending,重启后重新消费——at-least-once,由飞书侧幂等容忍重发。
package outbound

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/superteam/feishu-connector/internal/cards"
	"github.com/superteam/feishu-connector/internal/cpclient"
)

// ControlPlane 轮询与回执所需的控制平面动作。
type ControlPlane interface {
	ListOutbox(ctx context.Context, limit int) ([]cpclient.OutboxItem, error)
	// ListOutboxWait 空队列时在 CP 侧挂起 waitMs,有 pending 或超时后返回。
	ListOutboxWait(ctx context.Context, limit, waitMs int) ([]cpclient.OutboxItem, error)
	AckOutbox(ctx context.Context, id, result, feishuMessageID, errText string) error
}

// Messenger 投递能力(gateway 实现)。
type Messenger interface {
	SendCard(ctx context.Context, openID, cardJSON string) (string, error)
	UpdateCard(ctx context.Context, messageID, cardJSON string) error
}

// defaultLongPollWaitMs:空队列在 CP 挂起的上限。NOTIFY 到达会提前返回。
const defaultLongPollWaitMs = 2000

// errorBackoff:ListOutbox 失败时短暂退避,避免 tight-loop 打满日志/CP。
const errorBackoff = 2 * time.Second

type Poller struct {
	cp          ControlPlane
	messenger   Messenger
	webOrigin   string
	waitMs      int
	mu          sync.Mutex
	lastPollAt  time.Time
	lastPollErr error
}

func NewPoller(cp ControlPlane, messenger Messenger, webOrigin string) *Poller {
	return &Poller{cp: cp, messenger: messenger, webOrigin: webOrigin, waitMs: defaultLongPollWaitMs}
}

// SetWaitMs 测试/调优:长轮询 wait_ms;0 表示不 wait(立即返回)。
func (p *Poller) SetWaitMs(waitMs int) { p.waitMs = waitMs }

// SetInterval 兼容旧测试名:将 interval 映射为 wait_ms(秒级→毫秒)。
// Deprecated: 优先 SetWaitMs。
func (p *Poller) SetInterval(interval time.Duration) {
	if interval <= 0 {
		p.waitMs = 0
		return
	}
	p.waitMs = int(interval / time.Millisecond)
}

// LastPollAt 供心跳上报最近一次 outbox 轮询时刻。
func (p *Poller) LastPollAt() *time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastPollAt.IsZero() {
		return nil
	}
	t := p.lastPollAt
	return &t
}

// Run 持续长轮询:有 pending 立即投递;空队列在 CP wait 直到 NOTIFY 或超时。
// 不再用固定 2s ticker——card_update 延迟由「写库→NOTIFY→返回」主导。
func (p *Poller) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		p.drainOnce(ctx)
	}
}

func (p *Poller) drainOnce(ctx context.Context) {
	waitMs := p.waitMs
	var items []cpclient.OutboxItem
	var err error
	if waitMs > 0 {
		items, err = p.cp.ListOutboxWait(ctx, 20, waitMs)
	} else {
		items, err = p.cp.ListOutbox(ctx, 20)
	}
	p.mu.Lock()
	p.lastPollAt = time.Now().UTC()
	p.lastPollErr = err
	p.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		log.Printf("[outbound] list outbox: %v", err)
		select {
		case <-ctx.Done():
		case <-time.After(errorBackoff):
		}
		return
	}
	for _, item := range items {
		p.deliver(ctx, item)
	}
}

// Deliver 单条投递(导出供测试直接驱动)。
func (p *Poller) deliver(ctx context.Context, item cpclient.OutboxItem) {
	switch item.Kind {
	case "decision_card":
		cardJSON := cards.DecisionCard(item.Payload, item.ResourceID, item.ProjectID, p.webOrigin)
		messageID, err := p.messenger.SendCard(ctx, item.RecipientOpenID, cardJSON)
		p.ack(ctx, item.ID, messageID, err)
	case "result_notice":
		cardJSON := cards.ResultNoticeCard(item.Payload, p.webOrigin)
		messageID, err := p.messenger.SendCard(ctx, item.RecipientOpenID, cardJSON)
		p.ack(ctx, item.ID, messageID, err)
	case "card_update":
		messageID, _ := item.Payload["feishu_message_id"].(string)
		if messageID == "" {
			p.ack(ctx, item.ID, "", nil) // 无可更新目标,直接消化
			return
		}
		err := p.messenger.UpdateCard(ctx, messageID, cards.DecisionResolvedCard(item.Payload, p.webOrigin))
		p.ack(ctx, item.ID, messageID, err)
	default:
		log.Printf("[outbound] unknown outbox kind %q, acking to avoid poison-pill", item.Kind)
		p.ack(ctx, item.ID, "", nil)
	}
}

func (p *Poller) ack(ctx context.Context, id, messageID string, deliverErr error) {
	result := "sent"
	errText := ""
	if deliverErr != nil {
		result = "failed"
		errText = deliverErr.Error()
		log.Printf("[outbound] deliver %s failed: %v", id, deliverErr)
	}
	if err := p.cp.AckOutbox(ctx, id, result, messageID, errText); err != nil {
		log.Printf("[outbound] ack %s: %v", id, err)
	}
}
