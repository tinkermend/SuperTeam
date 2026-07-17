package project

import (
	"testing"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/storage/queries"
)

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
