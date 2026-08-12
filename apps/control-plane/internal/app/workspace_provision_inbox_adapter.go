package app

import (
	"context"

	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/project"
)

// workspaceProvisionInboxAdapter maps project-local DTOs into inbox.Service
// without creating a project↔inbox import cycle in either package.
type workspaceProvisionInboxAdapter struct {
	inbox *inbox.Service
}

func newWorkspaceProvisionInboxAdapter(inboxService *inbox.Service) project.WorkspaceProvisionInbox {
	if inboxService == nil {
		return nil
	}
	return &workspaceProvisionInboxAdapter{inbox: inboxService}
}

func (a *workspaceProvisionInboxAdapter) UpsertWorkspaceProvisionTodo(ctx context.Context, item project.WorkspaceProvisionInboxItem) error {
	sourceProjectID := item.SourceProjectID
	_, err := a.inbox.UpsertItem(ctx, inbox.UpsertItemRequest{
		TenantID:        item.TenantID,
		TargetUserID:    item.TargetUserID,
		Scope:           "personal",
		ItemType:        inbox.ItemTypeProjectWorkspaceProvisionPending,
		SourceType:      inbox.SourceTypeProjectWorkspaceProvisionPending,
		SourceID:        item.SourceID,
		SourceProjectID: &sourceProjectID,
		Title:           item.Title,
		Summary:         item.Summary,
		Priority:        "high",
		Status:          inbox.StatusOpen,
		DeepLink:        item.DeepLink,
		ContextPayload:  item.ContextPayload,
	})
	return err
}
