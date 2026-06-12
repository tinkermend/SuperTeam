package inbox

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	UpsertItem(ctx context.Context, req UpsertItemRequest) (Item, error)
	UpsertItemByApprovalSource(ctx context.Context, req UpsertItemRequest) (Item, error)
	GetItem(ctx context.Context, tenantID, itemID uuid.UUID) (Item, error)
	ListItems(ctx context.Context, req ListItemsRequest) ([]Item, error)
	CountOpenItems(ctx context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error)
	CountHighRiskOpenItems(ctx context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error)
}
