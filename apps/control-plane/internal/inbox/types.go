package inbox

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidItem          = errors.New("invalid inbox item")
	ErrItemNotFound         = errors.New("inbox item not found")
	ErrActionForbidden      = errors.New("inbox action forbidden")
	ErrInvalidAction        = errors.New("invalid inbox action")
	ErrSourceUnavailable    = errors.New("inbox source unavailable")
	ErrViewForbidden        = errors.New("inbox view forbidden")
	ErrProjectionNotApplied = errors.New("inbox projection not applied")
)

// ActionForbiddenError 表示 actor 不是事项的指定处理人;携带处理人展示名,
// 让 API 层能返回可读提示而非裸 403。errors.Is 仍匹配 ErrActionForbidden。
type ActionForbiddenError struct {
	TargetUserName string
}

func (e *ActionForbiddenError) Error() string {
	if e.TargetUserName != "" {
		return fmt.Sprintf("该事项由 %s 处理，只有指定处理人可以执行此操作", e.TargetUserName)
	}
	return "该事项由其他指定处理人处理，只有指定处理人可以执行此操作"
}

func (e *ActionForbiddenError) Unwrap() error { return ErrActionForbidden }

// DecisionForbiddenError 表示 actor 不在项目决策的 any-of-N 资格集合内
// (非项目人类成员/负责人)。errors.Is 仍匹配 ErrActionForbidden。
type DecisionForbiddenError struct{}

func (e *DecisionForbiddenError) Error() string {
	return "只有该项目的人类成员（含负责人）可以处理该决策"
}

func (e *DecisionForbiddenError) Unwrap() error { return ErrActionForbidden }

type Status string

const (
	StatusOpen      Status = "open"
	StatusResolved  Status = "resolved"
	StatusCancelled Status = "cancelled"
)

type View string

const (
	ViewMine View = "mine"
	ViewTeam View = "team"
)

type ItemType string

const (
	ItemTypeApproval        ItemType = "approval"
	ItemTypeProjectDecision ItemType = "project_decision"
	// 团队待确认删除滞留催办(生命周期收敛 P2:永不自动物理删,超时提醒管理员处理)。
	ItemTypeTeamPendingDelete ItemType = "team_pending_delete"
	// 飞书通道失联告警(接入管理 P1:只进 Console 收件箱,不推飞书,防自指)。
	ItemTypeChannelAlert ItemType = "channel_alert"
	// 自动化规则 fire 失败告警（收口批：无人值守失败必须推到人）。
	ItemTypeAutomationAlert ItemType = "automation_alert"
	// 编制因角色移除/词表停用被级联解除。
	ItemTypeCastingInvalidated ItemType = "casting_invalidated"
)

type SourceType string

const (
	SourceTypeApprovalRequest        SourceType = "approval_request"
	SourceTypeProjectDecisionRequest SourceType = "project_decision_request"
	SourceTypeTeamPendingDelete      SourceType = "team_pending_delete"
	SourceTypeChannelAlert           SourceType = "feishu_channel"
	SourceTypeAutomationRule         SourceType = "automation_rule"
	SourceTypeProjectCasting         SourceType = "project_casting"
)

type Action struct {
	Key             string         `json:"key"`
	Label           string         `json:"label"`
	Tone            string         `json:"tone"`
	RequiresComment bool           `json:"requires_comment"`
	Metadata        map[string]any `json:"metadata,omitempty"`
}

type Item struct {
	ID                      uuid.UUID
	TenantID                uuid.UUID
	TeamID                  *uuid.UUID
	TargetUserID            uuid.UUID
	Scope                   string
	ItemType                ItemType
	SourceType              SourceType
	SourceID                uuid.UUID
	SourceProjectID         *uuid.UUID
	SourceTaskID            *uuid.UUID
	SourceApprovalRequestID *uuid.UUID
	// SourceProjectName/SourceTaskName 是读时批量补名的展示字段,不落库;
	// 来源已删除或跨租户时为 nil。
	SourceProjectName *string
	SourceTaskName    *string
	Title             string
	Summary           *string
	RiskLevel         *string
	Priority          *string
	Status            Status
	Actions           []Action
	ContextPayload    map[string]any
	DeepLink          map[string]any
	ResolvedAt        *time.Time
	LastActivityAt    time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type UpsertItemRequest struct {
	TenantID                uuid.UUID
	TeamID                  *uuid.UUID
	TargetUserID            uuid.UUID
	Scope                   string
	ItemType                ItemType
	SourceType              SourceType
	SourceID                uuid.UUID
	SourceProjectID         *uuid.UUID
	SourceTaskID            *uuid.UUID
	SourceApprovalRequestID *uuid.UUID
	Title                   string
	Summary                 string
	RiskLevel               string
	Priority                string
	Status                  Status
	Actions                 []Action
	ContextPayload          map[string]any
	DeepLink                map[string]any
	ResolvedAt              *time.Time
	LastActivityAt          time.Time
}

type ListItemsRequest struct {
	TenantID        uuid.UUID
	ActorUserID     uuid.UUID
	View            View
	TeamViewAllowed bool
	Status          *Status
	ItemType        *ItemType
	RiskLevel       *string
	ProjectID       *uuid.UUID
	TargetUserID    *uuid.UUID
	Limit           int32
	Offset          int32
}

type ListItemsResult struct {
	Items         []Item
	Limit         int32
	Offset        int32
	HasMore       bool
	OpenCount     int64
	HighRiskCount int64
}

type Badge struct {
	MineOpenCount int64
	TeamOpenCount int64
	HighRiskCount int64
}

// PeekChangeRequest 是 SSE 脏通知探测的入参:探测 actor 可见范围内 (updated_at, id)
// 游标之后是否有变更行。TeamViewAllowed 由调用方(handler)判权后传入,与 GetBadge 的
// includeTeam 同一授权投影约定。
type PeekChangeRequest struct {
	TenantID        uuid.UUID
	ActorUserID     uuid.UUID
	TeamViewAllowed bool
	CursorUpdatedAt time.Time
	CursorID        uuid.UUID
}

// ChangeCursor 是变更游标:可见范围内最新变更行的 (updated_at, id)。
type ChangeCursor struct {
	UpdatedAt time.Time
	ID        uuid.UUID
}

type ExecuteActionRequest struct {
	TenantID    uuid.UUID
	ActorUserID uuid.UUID
	ItemID      uuid.UUID
	Action      string
	Comment     string
	Payload     map[string]any
}

type SourceActionResult struct {
	SourceType string
	SourceID   uuid.UUID
	Status     string
}
