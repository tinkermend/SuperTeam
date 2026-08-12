package feishu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
)

// stubOutboxRepo is a minimal OutboxRepository for AckOutbox unit tests.
type stubOutboxRepo struct {
	item OutboxItem
	err  error
}

func (s *stubOutboxRepo) ListPendingOutbox(context.Context, uuid.UUID, int32) ([]OutboxItem, error) {
	return nil, nil
}
func (s *stubOutboxRepo) MarkOutboxSent(context.Context, uuid.UUID, uuid.UUID, string) (OutboxItem, error) {
	return s.item, s.err
}
func (s *stubOutboxRepo) MarkOutboxFailed(context.Context, uuid.UUID, uuid.UUID, string) (OutboxItem, error) {
	return s.item, s.err
}
func (s *stubOutboxRepo) ListOutboxByStatuses(context.Context, uuid.UUID, []string, int32, int32, *time.Time) ([]OutboxItem, error) {
	return nil, nil
}
func (s *stubOutboxRepo) CountOutboxByStatuses(context.Context, uuid.UUID, []string, *time.Time) (int64, error) {
	return 0, nil
}
func (s *stubOutboxRepo) RequeueOutbox(context.Context, uuid.UUID, uuid.UUID) (OutboxItem, error) {
	return OutboxItem{}, nil
}

type recordingTerminalizer struct {
	calls []struct {
		tenantID, projectID, decisionID uuid.UUID
	}
	err error
}

func (r *recordingTerminalizer) EnsureDecisionCardsTerminal(_ context.Context, tenantID, projectID, decisionID uuid.UUID) error {
	r.calls = append(r.calls, struct {
		tenantID, projectID, decisionID uuid.UUID
	}{tenantID, projectID, decisionID})
	return r.err
}

func ackRequest(t *testing.T, tenantID, outboxID uuid.UUID, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/ack", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, tenantID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("outboxId", outboxID.String())
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

func TestAckOutboxDecisionCardSentTriggersTerminalizer(t *testing.T) {
	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	projectID := uuid.MustParse("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	decisionID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	outboxID := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")

	repo := &stubOutboxRepo{item: OutboxItem{
		ID:           outboxID,
		TenantID:     tenantID,
		ProjectID:    &projectID,
		Kind:         "decision_card",
		ResourceType: "decision_request",
		ResourceID:   decisionID,
		Status:       "sent",
	}}
	terminalizer := &recordingTerminalizer{}
	handler := NewConnectorHTTPHandler(nil)
	handler.SetOutboxRepository(repo)
	handler.SetDecisionCardTerminalizer(terminalizer)

	rec := httptest.NewRecorder()
	handler.AckOutbox(rec, ackRequest(t, tenantID, outboxID, `{"result":"sent","feishu_message_id":"om_race_1"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(terminalizer.calls) != 1 {
		t.Fatalf("expected 1 terminalizer call, got %d", len(terminalizer.calls))
	}
	call := terminalizer.calls[0]
	if call.tenantID != tenantID || call.projectID != projectID || call.decisionID != decisionID {
		t.Fatalf("unexpected call %#v", call)
	}
}

func TestAckOutboxResultNoticeDoesNotTriggerTerminalizer(t *testing.T) {
	tenantID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	outboxID := uuid.New()
	repo := &stubOutboxRepo{item: OutboxItem{
		ID:           outboxID,
		TenantID:     tenantID,
		Kind:         "result_notice",
		ResourceType: "project_demand",
		ResourceID:   uuid.New(),
		Status:       "sent",
	}}
	terminalizer := &recordingTerminalizer{}
	handler := NewConnectorHTTPHandler(nil)
	handler.SetOutboxRepository(repo)
	handler.SetDecisionCardTerminalizer(terminalizer)

	rec := httptest.NewRecorder()
	handler.AckOutbox(rec, ackRequest(t, tenantID, outboxID, `{"result":"sent","feishu_message_id":"om_x"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(terminalizer.calls) != 0 {
		t.Fatalf("result_notice must not trigger terminalizer, calls=%d", len(terminalizer.calls))
	}
}

func TestAckOutboxTerminalizerErrorDoesNotFailAck(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	outboxID := uuid.New()
	repo := &stubOutboxRepo{item: OutboxItem{
		ID: outboxID, TenantID: tenantID, ProjectID: &projectID,
		Kind: "decision_card", ResourceType: "decision_request", ResourceID: decisionID, Status: "sent",
	}}
	handler := NewConnectorHTTPHandler(nil)
	handler.SetOutboxRepository(repo)
	handler.SetDecisionCardTerminalizer(&recordingTerminalizer{err: context.DeadlineExceeded})

	rec := httptest.NewRecorder()
	handler.AckOutbox(rec, ackRequest(t, tenantID, outboxID, `{"result":"sent","feishu_message_id":"om_y"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("terminalizer error must not fail ack: status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "sent" {
		t.Fatalf("body=%#v", body)
	}
}
