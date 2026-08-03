package outbound

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/superteam/feishu-connector/internal/cpclient"
)

type fakeCP struct {
	items []cpclient.OutboxItem
	acks  []string
}

func (f *fakeCP) ListOutbox(_ context.Context, _ int) ([]cpclient.OutboxItem, error) {
	return f.items, nil
}

func (f *fakeCP) ListOutboxWait(ctx context.Context, limit, waitMs int) ([]cpclient.OutboxItem, error) {
	return f.ListOutbox(ctx, limit)
}

func (f *fakeCP) AckOutbox(_ context.Context, id, result, feishuMessageID, errText string) error {
	f.acks = append(f.acks, id+":"+result+":"+feishuMessageID)
	return nil
}

type fakeMessenger struct {
	sent    []string
	updated []string
	fail    bool
}

func (m *fakeMessenger) SendCard(_ context.Context, openID, cardJSON string) (string, error) {
	if m.fail {
		return "", errors.New("network down")
	}
	m.sent = append(m.sent, openID+":"+cardJSON)
	return "om_1", nil
}

func (m *fakeMessenger) UpdateCard(_ context.Context, messageID, cardJSON string) error {
	m.updated = append(m.updated, messageID)
	return nil
}

func TestDeliverDecisionCardAcksSentWithMessageID(t *testing.T) {
	cp := &fakeCP{}
	messenger := &fakeMessenger{}
	poller := NewPoller(cp, messenger, "http://web.local:3000")

	poller.deliver(context.Background(), cpclient.OutboxItem{
		ID: "ob-1", Kind: "decision_card", ResourceID: "dec-1", ProjectID: "p-1",
		RecipientOpenID: "ou_a",
		Payload:         map[string]any{"decision_type": "plan_review", "title": "计划评审"},
	})
	if len(messenger.sent) != 1 || !strings.Contains(messenger.sent[0], "resolve_decision") {
		t.Fatalf("expected decision card sent, got %#v", messenger.sent)
	}
	if len(cp.acks) != 1 || cp.acks[0] != "ob-1:sent:om_1" {
		t.Fatalf("expected sent ack with message id, got %#v", cp.acks)
	}
}

func TestDeliverFailureAcksFailed(t *testing.T) {
	cp := &fakeCP{}
	messenger := &fakeMessenger{fail: true}
	poller := NewPoller(cp, messenger, "http://web.local:3000")

	poller.deliver(context.Background(), cpclient.OutboxItem{
		ID: "ob-2", Kind: "result_notice", RecipientOpenID: "ou_a",
		Payload: map[string]any{"title": "需求", "status": "completed", "demand_id": "d-1"},
	})
	if len(cp.acks) != 1 || !strings.HasPrefix(cp.acks[0], "ob-2:failed:") {
		t.Fatalf("expected failed ack, got %#v", cp.acks)
	}
}

func TestDeliverCardUpdateUsesStoredMessageID(t *testing.T) {
	cp := &fakeCP{}
	messenger := &fakeMessenger{}
	poller := NewPoller(cp, messenger, "http://web.local:3000")

	poller.deliver(context.Background(), cpclient.OutboxItem{
		ID: "ob-3", Kind: "card_update", ResourceID: "dec-1", RecipientOpenID: "ou_a",
		Payload: map[string]any{"title": "计划评审", "resolved_status": "approved", "feishu_message_id": "om_9"},
	})
	if len(messenger.updated) != 1 || messenger.updated[0] != "om_9" {
		t.Fatalf("expected card update on om_9, got %#v", messenger.updated)
	}
	if len(cp.acks) != 1 || cp.acks[0] != "ob-3:sent:om_9" {
		t.Fatalf("expected sent ack, got %#v", cp.acks)
	}
}

func TestUnknownKindIsConsumedNotPoisoned(t *testing.T) {
	cp := &fakeCP{}
	poller := NewPoller(cp, &fakeMessenger{}, "http://web.local:3000")
	poller.deliver(context.Background(), cpclient.OutboxItem{ID: "ob-4", Kind: "mystery"})
	if len(cp.acks) != 1 || !strings.HasPrefix(cp.acks[0], "ob-4:sent") {
		t.Fatalf("unknown kind must be consumed, got %#v", cp.acks)
	}
}
