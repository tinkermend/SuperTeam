package feishu

import (
	"testing"

	"github.com/google/uuid"
)

func TestToOutboxOpsItemPrefersHumanLabelsFromPayload(t *testing.T) {
	projectID := uuid.New()
	recipientID := uuid.New()
	item := OutboxItem{
		ID:              uuid.New(),
		Kind:            "card_update",
		Status:          "sent",
		ResourceType:    "decision_request",
		ResourceID:      uuid.New(),
		ProjectID:       &projectID,
		RecipientUserID: recipientID,
		RecipientOpenID: "ou_x",
		Payload: map[string]any{
			"project_name": "验收演示项目",
			"title":        "是否批准扩编",
		},
		Attempts: 1,
	}
	row := toOutboxOpsItem(item, map[uuid.UUID]string{recipientID: "开发管理员"})
	if row.ProjectName != "验收演示项目" {
		t.Fatalf("project_name=%q", row.ProjectName)
	}
	if row.ResourceTitle != "是否批准扩编" {
		t.Fatalf("resource_title=%q", row.ResourceTitle)
	}
	if row.RecipientDisplayName != "开发管理员" {
		t.Fatalf("recipient_display_name=%q", row.RecipientDisplayName)
	}
	if row.ProjectID != projectID.String() {
		t.Fatalf("project_id should remain for copy/debug")
	}
}
