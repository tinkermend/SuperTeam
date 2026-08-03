package project

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/storage/queries"
)

func TestCardUpdateMessageIDsFromColumnAndPayload(t *testing.T) {
	rows := []queries.FeishuOutbox{
		{FeishuMessageID: pgtype.Text{String: "om_col", Valid: true}},
		{Payload: mustJSON(t, map[string]any{"feishu_message_id": "om_payload"})},
		{Payload: mustJSON(t, map[string]any{"title": "no mid"})},
		{},
	}
	ids := cardUpdateMessageIDs(rows)
	if _, ok := ids["om_col"]; !ok {
		t.Fatalf("expected column message id, got %#v", ids)
	}
	if _, ok := ids["om_payload"]; !ok {
		t.Fatalf("expected payload message id, got %#v", ids)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %#v", ids)
	}
}

func TestMergeDecisionResolvedPayload(t *testing.T) {
	resolvedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	decision := DecisionRequest{
		DecisionType:  "plan_review",
		TitleSnapshot: "计划评审",
		StatusSnapshot: "approved",
		ResolvedAt:    &resolvedAt,
	}
	payload := map[string]any{"summary": "keep me", "title": "old"}
	resolvedBy := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	mergeDecisionResolvedPayload(payload, decision, "om_1", resolvedBy, "张三", "LGTM")

	if payload["summary"] != "keep me" {
		t.Fatalf("original fields must be preserved, got %#v", payload)
	}
	if payload["title"] != "计划评审" {
		t.Fatalf("title overwritten by snapshot, got %#v", payload["title"])
	}
	if payload["resolved_status"] != "approved" {
		t.Fatalf("resolved_status=%v", payload["resolved_status"])
	}
	if payload["feishu_message_id"] != "om_1" {
		t.Fatalf("message_id=%v", payload["feishu_message_id"])
	}
	if payload["resolved_by_name"] != "张三" || payload["resolution_comment"] != "LGTM" {
		t.Fatalf("resolver fields missing: %#v", payload)
	}
	if payload["resolved_at"] != resolvedAt.Format(time.RFC3339) {
		t.Fatalf("resolved_at=%v", payload["resolved_at"])
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestExpandFeishuRecipients(t *testing.T) {
	owner := uuid.New()
	member := uuid.New()
	inactive := uuid.New()
	employee := uuid.New()
	outsider := uuid.New()

	members := []queries.ProjectMember{
		{PrincipalType: "human_user", PrincipalID: member, Status: "active"},
		{PrincipalType: "human_user", PrincipalID: inactive, Status: "removed"},
		{PrincipalType: "digital_employee", PrincipalID: employee, Status: "active"},
	}
	identities := []queries.UserFeishuIdentity{
		{AuthUserID: owner, OpenID: "ou_owner"},
		{AuthUserID: member, OpenID: "ou_member"},
		{AuthUserID: inactive, OpenID: "ou_inactive"},
		{AuthUserID: employee, OpenID: "ou_employee"},
		{AuthUserID: outsider, OpenID: "ou_outsider"},
		{AuthUserID: member, OpenID: "ou_member_dup"},
	}

	recipients := expandFeishuRecipients(owner, members, identities)
	byUser := map[uuid.UUID]string{}
	for _, r := range recipients {
		byUser[r.UserID] = r.OpenID
	}
	if len(recipients) != 2 {
		t.Fatalf("expected owner+member only, got %#v", recipients)
	}
	if byUser[owner] != "ou_owner" || byUser[member] != "ou_member" {
		t.Fatalf("unexpected recipients %#v", byUser)
	}
}

func TestExpandFeishuRecipientsEmptyWhenNoneBound(t *testing.T) {
	owner := uuid.New()
	if got := expandFeishuRecipients(owner, nil, nil); len(got) != 0 {
		t.Fatalf("expected empty, got %#v", got)
	}
}
