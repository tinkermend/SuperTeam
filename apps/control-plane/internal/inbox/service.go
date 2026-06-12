package inbox

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultListLimit int32 = 50
	maxListLimit     int32 = 100
)

type ApprovalActionResolver interface {
	ResolveApprovalAction(ctx context.Context, req SourceActionRequest) (SourceActionResult, error)
}

type ProjectDecisionActionResolver interface {
	ResolveProjectDecisionAction(ctx context.Context, req SourceActionRequest) (SourceActionResult, error)
}

type SourceActionRequest struct {
	TenantID        uuid.UUID
	ActorUserID     uuid.UUID
	SourceID        uuid.UUID
	SourceProjectID *uuid.UUID
	Action          string
	Comment         string
	Payload         map[string]any
}

type Service struct {
	repository Repository
	approvals  ApprovalActionResolver
	decisions  ProjectDecisionActionResolver
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, ErrInvalidItem
	}
	return &Service{repository: repository}, nil
}

func (s *Service) SetApprovalActionResolver(resolver ApprovalActionResolver) {
	s.approvals = resolver
}

func (s *Service) SetProjectDecisionActionResolver(resolver ProjectDecisionActionResolver) {
	s.decisions = resolver
}

func (s *Service) UpsertItem(ctx context.Context, req UpsertItemRequest) (Item, error) {
	normalized, err := normalizeUpsert(req)
	if err != nil {
		return Item{}, err
	}
	if normalized.SourceApprovalRequestID != nil {
		return s.repository.UpsertItemByApprovalSource(ctx, normalized)
	}
	return s.repository.UpsertItem(ctx, normalized)
}

func (s *Service) ListItems(ctx context.Context, req ListItemsRequest) (ListItemsResult, error) {
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return ListItemsResult{}, ErrInvalidItem
	}
	if req.View == "" {
		req.View = ViewMine
	}
	if req.Status == "" {
		req.Status = StatusOpen
	}
	if req.View == ViewMine {
		req.TargetUserID = &req.ActorUserID
	}
	if req.View != ViewMine && req.View != ViewTeam {
		return ListItemsResult{}, ErrViewForbidden
	}
	items, err := s.repository.ListItems(ctx, req)
	if err != nil {
		return ListItemsResult{}, err
	}
	openCount, err := s.repository.CountOpenItems(ctx, req.TenantID, req.TargetUserID)
	if err != nil {
		return ListItemsResult{}, err
	}
	highRiskCount, err := s.repository.CountHighRiskOpenItems(ctx, req.TenantID, req.TargetUserID)
	if err != nil {
		return ListItemsResult{}, err
	}
	return ListItemsResult{Items: items, Limit: req.Limit, Offset: req.Offset, HasMore: len(items) == int(req.Limit), OpenCount: openCount, HighRiskCount: highRiskCount}, nil
}

func (s *Service) GetBadge(ctx context.Context, tenantID, actorUserID uuid.UUID, includeTeam bool) (Badge, error) {
	if tenantID == uuid.Nil || actorUserID == uuid.Nil {
		return Badge{}, ErrInvalidItem
	}
	mine, err := s.repository.CountOpenItems(ctx, tenantID, &actorUserID)
	if err != nil {
		return Badge{}, err
	}
	high, err := s.repository.CountHighRiskOpenItems(ctx, tenantID, &actorUserID)
	if err != nil {
		return Badge{}, err
	}
	var team int64
	if includeTeam {
		team, err = s.repository.CountOpenItems(ctx, tenantID, nil)
		if err != nil {
			return Badge{}, err
		}
	}
	return Badge{MineOpenCount: mine, TeamOpenCount: team, HighRiskCount: high}, nil
}

func (s *Service) ExecuteAction(ctx context.Context, req ExecuteActionRequest) (Item, SourceActionResult, error) {
	req.Action = strings.TrimSpace(req.Action)
	req.Comment = strings.TrimSpace(req.Comment)
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil || req.ItemID == uuid.Nil || req.Action == "" {
		return Item{}, SourceActionResult{}, ErrInvalidAction
	}
	item, err := s.repository.GetItem(ctx, req.TenantID, req.ItemID)
	if err != nil {
		return Item{}, SourceActionResult{}, err
	}
	if item.Status != StatusOpen {
		return item, SourceActionResult{}, ErrInvalidAction
	}
	if item.TargetUserID != req.ActorUserID {
		return item, SourceActionResult{}, ErrActionForbidden
	}
	if !actionAllowed(item.Actions, req.Action) {
		return item, SourceActionResult{}, ErrInvalidAction
	}
	sourceReq := SourceActionRequest{TenantID: req.TenantID, ActorUserID: req.ActorUserID, SourceID: item.SourceID, SourceProjectID: item.SourceProjectID, Action: req.Action, Comment: req.Comment, Payload: mapOrEmpty(req.Payload)}
	var result SourceActionResult
	switch item.SourceType {
	case SourceTypeApprovalRequest:
		if s.approvals == nil {
			return item, SourceActionResult{}, ErrSourceUnavailable
		}
		result, err = s.approvals.ResolveApprovalAction(ctx, sourceReq)
	case SourceTypeProjectDecisionRequest:
		if s.decisions == nil {
			return item, SourceActionResult{}, ErrSourceUnavailable
		}
		result, err = s.decisions.ResolveProjectDecisionAction(ctx, sourceReq)
	default:
		return item, SourceActionResult{}, ErrSourceUnavailable
	}
	if err != nil {
		return item, SourceActionResult{}, err
	}
	// Inbox is a read model. The source action resolver synchronously updates the source,
	// which then synchronously calls the Inbox projector to update the item state in the DB.
	// We just fetch the updated item to return.
	updated, err := s.repository.GetItem(ctx, req.TenantID, req.ItemID)
	if err != nil {
		return item, result, err
	}
	return updated, result, nil
}

func normalizePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func normalizeUpsert(req UpsertItemRequest) (UpsertItemRequest, error) {
	req.Scope = strings.TrimSpace(req.Scope)
	req.Title = strings.TrimSpace(req.Title)
	req.Summary = strings.TrimSpace(req.Summary)
	req.RiskLevel = strings.TrimSpace(req.RiskLevel)
	req.Priority = strings.TrimSpace(req.Priority)
	if req.TenantID == uuid.Nil || req.TargetUserID == uuid.Nil || req.SourceID == uuid.Nil || req.Title == "" {
		return UpsertItemRequest{}, ErrInvalidItem
	}
	if req.Scope == "" {
		req.Scope = "personal"
	}
	if req.ItemType == "" || req.SourceType == "" {
		return UpsertItemRequest{}, ErrInvalidItem
	}
	switch req.Status {
	case "":
		req.Status = StatusOpen
	case StatusOpen:
	case StatusResolved:
	case StatusCancelled:
	default:
		return UpsertItemRequest{}, ErrInvalidItem
	}
	if req.ResolvedAt == nil && req.Status != StatusOpen {
		now := time.Now().UTC()
		req.ResolvedAt = &now
	}
	if req.Status == StatusOpen {
		req.ResolvedAt = nil
	}
	if req.LastActivityAt.IsZero() {
		req.LastActivityAt = time.Now().UTC()
	} else {
		req.LastActivityAt = req.LastActivityAt.UTC()
	}
	if len(req.Actions) == 0 {
		req.Actions = DefaultActions(req.ItemType)
	}
	req.ContextPayload = mapOrEmpty(req.ContextPayload)
	req.DeepLink = mapOrEmpty(req.DeepLink)
	return req, nil
}

func DefaultActions(itemType ItemType) []Action {
	actions := []Action{
		{Key: "approve", Label: "Approve", Tone: "positive"},
		{Key: "reject", Label: "Reject", Tone: "destructive", RequiresComment: true},
		{Key: "needs_more_evidence", Label: "Request evidence", Tone: "warning", RequiresComment: true},
	}
	if itemType == ItemTypeProjectDecision {
		actions[0].Metadata = map[string]any{"decision": "approved"}
		actions[1].Metadata = map[string]any{"decision": "rejected"}
		actions[2].Metadata = map[string]any{"decision": "needs_more_evidence"}
	}
	return actions
}

func actionAllowed(actions []Action, action string) bool {
	for _, candidate := range actions {
		if strings.TrimSpace(candidate.Key) == action {
			return true
		}
	}
	return false
}

func mapOrEmpty(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func stringValue(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
