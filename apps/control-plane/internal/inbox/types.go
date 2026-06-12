package inbox

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidItem       = errors.New("invalid inbox item")
	ErrItemNotFound      = errors.New("inbox item not found")
	ErrActionForbidden   = errors.New("inbox action forbidden")
	ErrInvalidAction     = errors.New("invalid inbox action")
	ErrSourceUnavailable = errors.New("inbox source unavailable")
	ErrViewForbidden     = errors.New("inbox view forbidden")
)

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
)

type SourceType string

const (
	SourceTypeApprovalRequest        SourceType = "approval_request"
	SourceTypeProjectDecisionRequest SourceType = "project_decision_request"
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
	Title                   string
	Summary                 *string
	RiskLevel               *string
	Priority                *string
	Status                  Status
	Actions                 []Action
	ContextPayload          map[string]any
	DeepLink                map[string]any
	ResolvedAt              *time.Time
	LastActivityAt          time.Time
	CreatedAt               time.Time
	UpdatedAt               time.Time
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
	TenantID     uuid.UUID
	ActorUserID  uuid.UUID
	View         View
	Status       Status
	ItemType     *ItemType
	RiskLevel    *string
	ProjectID    *uuid.UUID
	TargetUserID *uuid.UUID
	Limit        int32
	Offset       int32
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
