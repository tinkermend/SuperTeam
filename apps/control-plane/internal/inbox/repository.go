package inbox

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	UpsertItem(ctx context.Context, req UpsertItemRequest) (Item, error)
	UpsertItemByApprovalSource(ctx context.Context, req UpsertItemRequest) (Item, error)
	GetItem(ctx context.Context, tenantID, itemID uuid.UUID) (Item, error)
	GetItemByApprovalSource(ctx context.Context, tenantID, approvalRequestID uuid.UUID) (Item, error)
	ListItems(ctx context.Context, req ListItemsRequest) ([]Item, error)
	CountOpenItems(ctx context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error)
	CountHighRiskOpenItems(ctx context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error)
	// PeekChange 返回可见范围内游标之后最新的变更行;无变更时返回 nil。
	PeekChange(ctx context.Context, req PeekChangeRequest) (*ChangeCursor, error)
	// 来源补名:批量解析项目名/任务标题,缺失的 id 不出现在结果里。
	ProjectNames(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
	ProjectTaskTitles(ctx context.Context, tenantID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]string, error)
	// 指定处理人补名:仅用于 403 提示文案;用户不存在时返回空串(不报错)。
	UserDisplayName(ctx context.Context, userID uuid.UUID) (string, error)
}
