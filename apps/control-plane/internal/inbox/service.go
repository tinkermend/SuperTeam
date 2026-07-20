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

type RunRecoveryActionResolver interface {
	ResolveRunRecoveryAction(ctx context.Context, req SourceActionRequest) (SourceActionResult, error)
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
	repository   Repository
	approvals    ApprovalActionResolver
	decisions    ProjectDecisionActionResolver
	runRecovery  RunRecoveryActionResolver
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

func (s *Service) SetRunRecoveryActionResolver(resolver RunRecoveryActionResolver) {
	s.runRecovery = resolver
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
	if req.Status != nil && !validStatus(*req.Status) {
		return ListItemsResult{}, ErrInvalidItem
	}
	if req.View == ViewMine {
		req.TargetUserID = &req.ActorUserID
	}
	if req.View != ViewMine && req.View != ViewTeam {
		return ListItemsResult{}, ErrViewForbidden
	}
	if req.View == ViewTeam && !req.TeamViewAllowed {
		return ListItemsResult{}, ErrViewForbidden
	}
	fetchReq := req
	fetchReq.Limit = req.Limit + 1
	items, err := s.repository.ListItems(ctx, fetchReq)
	if err != nil {
		return ListItemsResult{}, err
	}
	hasMore := len(items) > int(req.Limit)
	if hasMore {
		items = items[:req.Limit]
	}
	openCount, err := s.repository.CountOpenItems(ctx, req.TenantID, req.TargetUserID)
	if err != nil {
		return ListItemsResult{}, err
	}
	highRiskCount, err := s.repository.CountHighRiskOpenItems(ctx, req.TenantID, req.TargetUserID)
	if err != nil {
		return ListItemsResult{}, err
	}
	if err := s.enrichSourceNames(ctx, req.TenantID, items); err != nil {
		return ListItemsResult{}, err
	}
	return ListItemsResult{Items: items, Limit: req.Limit, Offset: req.Offset, HasMore: hasMore, OpenCount: openCount, HighRiskCount: highRiskCount}, nil
}

// enrichSourceNames 就地为 items 批量补 source_project_name/source_task_name。
// 名称是展示字段:来源行已删除时保持 nil,由前端回退显示 id。
func (s *Service) enrichSourceNames(ctx context.Context, tenantID uuid.UUID, items []Item) error {
	projectIDs := make([]uuid.UUID, 0, len(items))
	taskIDs := make([]uuid.UUID, 0, len(items))
	seenProjects := map[uuid.UUID]struct{}{}
	seenTasks := map[uuid.UUID]struct{}{}
	for _, item := range items {
		if item.SourceProjectID != nil {
			if _, ok := seenProjects[*item.SourceProjectID]; !ok {
				seenProjects[*item.SourceProjectID] = struct{}{}
				projectIDs = append(projectIDs, *item.SourceProjectID)
			}
		}
		if item.SourceTaskID != nil {
			if _, ok := seenTasks[*item.SourceTaskID]; !ok {
				seenTasks[*item.SourceTaskID] = struct{}{}
				taskIDs = append(taskIDs, *item.SourceTaskID)
			}
		}
	}
	if len(projectIDs) == 0 && len(taskIDs) == 0 {
		return nil
	}
	projectNames, err := s.repository.ProjectNames(ctx, tenantID, projectIDs)
	if err != nil {
		return err
	}
	taskTitles, err := s.repository.ProjectTaskTitles(ctx, tenantID, taskIDs)
	if err != nil {
		return err
	}
	for i := range items {
		if items[i].SourceProjectID != nil {
			if name, ok := projectNames[*items[i].SourceProjectID]; ok {
				value := name
				items[i].SourceProjectName = &value
			}
		}
		if items[i].SourceTaskID != nil {
			if title, ok := taskTitles[*items[i].SourceTaskID]; ok {
				value := title
				items[i].SourceTaskName = &value
			}
		}
	}
	return nil
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
	// Team badge visibility is authorized by the caller; this method only applies
	// the already-authorized includeTeam projection.
	if includeTeam {
		team, err = s.repository.CountOpenItems(ctx, tenantID, nil)
		if err != nil {
			return Badge{}, err
		}
	}
	return Badge{MineOpenCount: mine, TeamOpenCount: team, HighRiskCount: high}, nil
}

// PeekChanges 探测 actor 可见范围内游标之后的最新变更;无变更返回 nil。
// req.TeamViewAllowed 的授权由调用方负责(同 GetBadge 的 includeTeam 约定)。
func (s *Service) PeekChanges(ctx context.Context, req PeekChangeRequest) (*ChangeCursor, error) {
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidItem
	}
	return s.repository.PeekChange(ctx, req)
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
	action, ok := findAction(item.Actions, req.Action)
	if !ok {
		return item, SourceActionResult{}, ErrInvalidAction
	}
	if action.RequiresComment && req.Comment == "" {
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
		if item.SourceProjectID == nil || *item.SourceProjectID == uuid.Nil {
			return item, SourceActionResult{}, ErrSourceUnavailable
		}
		if s.decisions == nil {
			return item, SourceActionResult{}, ErrSourceUnavailable
		}
		result, err = s.decisions.ResolveProjectDecisionAction(ctx, sourceReq)
	case SourceTypeDigitalEmployeeRun:
		if s.runRecovery == nil {
			return item, SourceActionResult{}, ErrSourceUnavailable
		}
		result, err = s.runRecovery.ResolveRunRecoveryAction(ctx, sourceReq)
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
	if updated.Status == StatusOpen {
		return updated, result, ErrProjectionNotApplied
	}
	// 补名失败不影响动作结果:动作在来源侧已成功,名称缺失由前端回退显示 id。
	single := []Item{updated}
	if err := s.enrichSourceNames(ctx, req.TenantID, single); err == nil {
		updated = single[0]
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
	if !validScope(req.Scope) || !validItemType(req.ItemType) || !validSourceType(req.SourceType) {
		return UpsertItemRequest{}, ErrInvalidItem
	}
	switch req.ItemType {
	case ItemTypeApproval:
		if req.SourceType != SourceTypeApprovalRequest {
			return UpsertItemRequest{}, ErrInvalidItem
		}
	case ItemTypeProjectDecision:
		if req.SourceType != SourceTypeProjectDecisionRequest {
			return UpsertItemRequest{}, ErrInvalidItem
		}
	case ItemTypeTeamPendingDelete:
		if req.SourceType != SourceTypeTeamPendingDelete {
			return UpsertItemRequest{}, ErrInvalidItem
		}
	case ItemTypeDigitalEmployeeRunRecovery:
		if req.SourceType != SourceTypeDigitalEmployeeRun {
			return UpsertItemRequest{}, ErrInvalidItem
		}
	}
	if req.SourceApprovalRequestID != nil && *req.SourceApprovalRequestID == uuid.Nil {
		return UpsertItemRequest{}, ErrInvalidItem
	}
	if req.SourceType == SourceTypeApprovalRequest {
		if req.SourceApprovalRequestID == nil {
			sourceID := req.SourceID
			req.SourceApprovalRequestID = &sourceID
		}
		if *req.SourceApprovalRequestID != req.SourceID {
			return UpsertItemRequest{}, ErrInvalidItem
		}
	}
	if req.ItemType == ItemTypeProjectDecision && (req.SourceProjectID == nil || *req.SourceProjectID == uuid.Nil) {
		return UpsertItemRequest{}, ErrInvalidItem
	}
	if req.Status == "" {
		req.Status = StatusOpen
	}
	if !validStatus(req.Status) {
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
	actions, err := normalizeActions(req.Actions)
	if err != nil {
		return UpsertItemRequest{}, err
	}
	req.Actions = actions
	req.ContextPayload = mapOrEmpty(req.ContextPayload)
	req.DeepLink = mapOrEmpty(req.DeepLink)
	return req, nil
}

func normalizeActions(actions []Action) ([]Action, error) {
	normalized := make([]Action, len(actions))
	seen := make(map[string]struct{}, len(actions))
	for i, action := range actions {
		action.Key = strings.TrimSpace(action.Key)
		action.Label = strings.TrimSpace(action.Label)
		action.Tone = strings.TrimSpace(action.Tone)
		if action.Key == "" || action.Label == "" {
			return nil, ErrInvalidItem
		}
		if _, ok := seen[action.Key]; ok {
			return nil, ErrInvalidItem
		}
		seen[action.Key] = struct{}{}
		normalized[i] = action
	}
	return normalized, nil
}

func DefaultActions(itemType ItemType) []Action {
	if itemType == ItemTypeDigitalEmployeeRunRecovery {
		return []Action{
			{Key: "retry", Label: "重试", Tone: "positive"},
			{Key: "acknowledge", Label: "确认关闭", Tone: "warning"},
		}
	}
	actions := []Action{
		{Key: "approved", Label: "同意", Tone: "positive"},
		{Key: "rejected", Label: "驳回", Tone: "destructive", RequiresComment: true},
		{Key: "needs_more_evidence", Label: "要求补证", Tone: "warning", RequiresComment: true},
	}
	if itemType == ItemTypeProjectDecision {
		actions[0].Metadata = map[string]any{"decision": "approved"}
		actions[1].Metadata = map[string]any{"decision": "rejected"}
		actions[2].Metadata = map[string]any{"decision": "needs_more_evidence"}
	}
	return actions
}

// DecisionActions returns the inbox action vocabulary for a project-decision item
// of the given decision type. Most decision types use the generic
// approved/rejected/needs_more_evidence set; a planning_gap decision instead gets
// 已补员/关闭 so the human can reopen-and-replan or close the gap. The web
// inbox-shell renders item.actions dynamically, so this is the sole source of the
// buttons shown per decision type.
func DecisionActions(decisionType string) []Action {
	switch decisionType {
	case "planning_gap":
		return []Action{
			{Key: "restaffed", Label: "已补员，重新规划", Tone: "positive", Metadata: map[string]any{"decision": "restaffed"}},
			{Key: "exempted", Label: "豁免约束并重规划", Tone: "positive", Metadata: map[string]any{"decision": "exempted"}},
			{Key: "rejected", Label: "关闭", Tone: "destructive", Metadata: map[string]any{"decision": "rejected"}},
		}
	case "task_failure_recovery":
		// 失败恢复需要显式 recovery_action;裸 approved 无法驱动 retry。
		// 改派需要选择新员工,收件箱快动作暂不暴露(项目详情可后续补选择器)。
		return []Action{
			{
				Key:   "retry",
				Label: "重试任务",
				Tone:  "positive",
				Metadata: map[string]any{
					"decision": "retry",
				},
			},
			{
				Key:             "cancel_downstream",
				Label:           "取消下游",
				Tone:            "destructive",
				RequiresComment: true,
				Metadata: map[string]any{
					"decision": "cancel_downstream",
				},
			},
		}
	case "demand_acceptance":
		// demand_acceptance items are a pure deep-link: signing individual
		// acceptance criteria is a per-criterion action that happens in the
		// demand detail UI, not a single inbox quick-action, so no buttons are
		// rendered here.
		return []Action{}
	}
	return DefaultActions(ItemTypeProjectDecision)
}

func findAction(actions []Action, action string) (Action, bool) {
	for _, candidate := range actions {
		if strings.TrimSpace(candidate.Key) == action {
			return candidate, true
		}
	}
	return Action{}, false
}

func validStatus(status Status) bool {
	switch status {
	case StatusOpen, StatusResolved, StatusCancelled:
		return true
	default:
		return false
	}
}

func validScope(scope string) bool {
	switch scope {
	case "personal", "team":
		return true
	default:
		return false
	}
}

func validItemType(itemType ItemType) bool {
	switch itemType {
	case ItemTypeApproval, ItemTypeProjectDecision, ItemTypeTeamPendingDelete, ItemTypeDigitalEmployeeRunRecovery:
		return true
	default:
		return false
	}
}

func validSourceType(sourceType SourceType) bool {
	switch sourceType {
	case SourceTypeApprovalRequest, SourceTypeProjectDecisionRequest, SourceTypeTeamPendingDelete, SourceTypeDigitalEmployeeRun:
		return true
	default:
		return false
	}
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
