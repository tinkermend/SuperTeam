package inbox

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestServiceProjectsApprovalRequestsIntoOpenItems(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	targetUserID := uuid.New()
	approvalID := uuid.New()

	item, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:                tenantID,
		TargetUserID:            targetUserID,
		Scope:                   "personal",
		ItemType:                ItemTypeApproval,
		SourceType:              SourceTypeApprovalRequest,
		SourceID:                approvalID,
		SourceApprovalRequestID: &approvalID,
		Title:                   "  确认高风险审批  ",
		Summary:                 "需要负责人确认是否继续",
		RiskLevel:               "high",
		Priority:                "urgent",
		ContextPayload:          map[string]any{"approval_request_id": approvalID.String()},
	})
	if err != nil {
		t.Fatalf("upsert approval item: %v", err)
	}
	if item.Status != StatusOpen {
		t.Fatalf("expected open item, got %s", item.Status)
	}
	if item.SourceApprovalRequestID == nil || *item.SourceApprovalRequestID != approvalID {
		t.Fatalf("expected approval source %s, got %#v", approvalID, item.SourceApprovalRequestID)
	}
	if item.Title != "确认高风险审批" {
		t.Fatalf("expected trimmed title, got %q", item.Title)
	}
	if item.Summary == nil || *item.Summary != "需要负责人确认是否继续" {
		t.Fatalf("expected summary snapshot, got %#v", item.Summary)
	}
	if len(item.Actions) == 0 {
		t.Fatal("expected default actions")
	}
	if _, ok := repo.itemsByApproval[approvalID]; !ok {
		t.Fatal("expected approval-source upsert path")
	}
}

func TestServiceRejectsZeroApprovalSourceID(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	approvalID := uuid.Nil

	_, err = service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:                uuid.New(),
		TargetUserID:            uuid.New(),
		Scope:                   "personal",
		ItemType:                ItemTypeApproval,
		SourceType:              SourceTypeApprovalRequest,
		SourceID:                uuid.New(),
		SourceApprovalRequestID: &approvalID,
		Title:                   "审批待处理",
	})
	if !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("expected invalid item, got %v", err)
	}
	if repo.upsertItemCalls != 0 || repo.upsertByApprovalSourceCalls != 0 {
		t.Fatalf("expected rejection before repository upsert, got generic=%d approval=%d", repo.upsertItemCalls, repo.upsertByApprovalSourceCalls)
	}
}

func TestServiceUpgradesProjectDecisionByApprovalSource(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	targetUserID := uuid.New()
	approvalID := uuid.New()
	decisionID := uuid.New()
	projectID := uuid.New()

	approvalItem, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:                tenantID,
		TargetUserID:            targetUserID,
		Scope:                   "personal",
		ItemType:                ItemTypeApproval,
		SourceType:              SourceTypeApprovalRequest,
		SourceID:                approvalID,
		SourceApprovalRequestID: &approvalID,
		Title:                   "审批待处理",
	})
	if err != nil {
		t.Fatalf("upsert approval item: %v", err)
	}

	decisionItem, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:                tenantID,
		TargetUserID:            targetUserID,
		Scope:                   "personal",
		ItemType:                ItemTypeProjectDecision,
		SourceType:              SourceTypeProjectDecisionRequest,
		SourceID:                decisionID,
		SourceProjectID:         &projectID,
		SourceApprovalRequestID: &approvalID,
		Title:                   "项目决策待处理",
		RiskLevel:               "high",
	})
	if err != nil {
		t.Fatalf("upsert decision item: %v", err)
	}
	if decisionItem.ID != approvalItem.ID {
		t.Fatalf("expected same item upgraded by approval source, got %s and %s", approvalItem.ID, decisionItem.ID)
	}
	if decisionItem.Status != StatusOpen {
		t.Fatalf("expected open item, got %s", decisionItem.Status)
	}
	if decisionItem.SourceApprovalRequestID == nil || *decisionItem.SourceApprovalRequestID != approvalID {
		t.Fatalf("expected approval source %s, got %#v", approvalID, decisionItem.SourceApprovalRequestID)
	}
	if decisionItem.ItemType != ItemTypeProjectDecision || decisionItem.SourceType != SourceTypeProjectDecisionRequest {
		t.Fatalf("expected project decision upgrade, got item=%s source=%s", decisionItem.ItemType, decisionItem.SourceType)
	}
	if decisionItem.SourceID != decisionID {
		t.Fatalf("expected decision source %s, got %s", decisionID, decisionItem.SourceID)
	}
	if decisionItem.SourceProjectID == nil || *decisionItem.SourceProjectID != projectID {
		t.Fatalf("expected project source %s, got %#v", projectID, decisionItem.SourceProjectID)
	}
	if len(repo.itemsByID) != 1 {
		t.Fatalf("expected single upgraded inbox item, got %d", len(repo.itemsByID))
	}
	if repo.upsertItemCalls != 0 {
		t.Fatalf("expected approval-source upserts to bypass generic source upsert, got %d generic calls", repo.upsertItemCalls)
	}
	if repo.upsertByApprovalSourceCalls != 2 {
		t.Fatalf("expected two approval-source upserts, got %d", repo.upsertByApprovalSourceCalls)
	}
}

func TestServiceRejectsInvalidListStatus(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.ListItems(context.Background(), ListItemsRequest{
		TenantID:    uuid.New(),
		ActorUserID: uuid.New(),
		View:        ViewMine,
		Status:      Status("archived"),
	})
	if !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("expected invalid item, got %v", err)
	}
}

func TestServiceRejectsProjectDecisionUpsertWithoutProjectSource(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:     uuid.New(),
		TargetUserID: uuid.New(),
		Scope:        "personal",
		ItemType:     ItemTypeProjectDecision,
		SourceType:   SourceTypeProjectDecisionRequest,
		SourceID:     uuid.New(),
		Title:        "项目决策待处理",
	})
	if !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("expected invalid item, got %v", err)
	}
}

func TestServiceRejectsInvalidUpsertBoundaries(t *testing.T) {
	projectID := uuid.New()
	tests := []struct {
		name   string
		mutate func(*UpsertItemRequest)
	}{
		{
			name: "invalid scope",
			mutate: func(req *UpsertItemRequest) {
				req.Scope = "workspace"
			},
		},
		{
			name: "invalid item type",
			mutate: func(req *UpsertItemRequest) {
				req.ItemType = ItemType("runtime_escalation")
			},
		},
		{
			name: "invalid source type",
			mutate: func(req *UpsertItemRequest) {
				req.SourceType = SourceType("runtime_request")
			},
		},
		{
			name: "approval item with project decision source",
			mutate: func(req *UpsertItemRequest) {
				req.ItemType = ItemTypeApproval
				req.SourceType = SourceTypeProjectDecisionRequest
				req.SourceProjectID = &projectID
			},
		},
		{
			name: "project decision item with approval source",
			mutate: func(req *UpsertItemRequest) {
				req.ItemType = ItemTypeProjectDecision
				req.SourceType = SourceTypeApprovalRequest
				req.SourceProjectID = &projectID
			},
		},
		{
			name: "approval source id does not match source id",
			mutate: func(req *UpsertItemRequest) {
				mismatch := uuid.New()
				req.SourceApprovalRequestID = &mismatch
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMemoryRepository()
			service, err := NewService(repo)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			req := validApprovalUpsertRequest()
			tc.mutate(&req)

			_, err = service.UpsertItem(context.Background(), req)
			if !errors.Is(err, ErrInvalidItem) {
				t.Fatalf("expected invalid item, got %v", err)
			}
			if repo.upsertItemCalls != 0 || repo.upsertByApprovalSourceCalls != 0 {
				t.Fatalf("expected rejection before repository upsert, got generic=%d approval=%d", repo.upsertItemCalls, repo.upsertByApprovalSourceCalls)
			}
		})
	}
}

func TestServiceNormalizesMissingApprovalSourceIdentity(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	req := validApprovalUpsertRequest()
	req.SourceApprovalRequestID = nil

	item, err := service.UpsertItem(context.Background(), req)
	if err != nil {
		t.Fatalf("upsert approval item: %v", err)
	}
	if item.SourceApprovalRequestID == nil || *item.SourceApprovalRequestID != req.SourceID {
		t.Fatalf("expected approval source to normalize to source id %s, got %#v", req.SourceID, item.SourceApprovalRequestID)
	}
	if repo.upsertItemCalls != 0 || repo.upsertByApprovalSourceCalls != 1 {
		t.Fatalf("expected approval-source upsert path, got generic=%d approval=%d", repo.upsertItemCalls, repo.upsertByApprovalSourceCalls)
	}
}

func TestServiceRejectsActionsFromNonTargetUser(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resolver := &fakeApprovalResolver{}
	service.SetApprovalActionResolver(resolver)
	tenantID := uuid.New()
	targetUserID := uuid.New()
	actorUserID := uuid.New()
	approvalID := uuid.New()
	item, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:     tenantID,
		TargetUserID: targetUserID,
		Scope:        "personal",
		ItemType:     ItemTypeApproval,
		SourceType:   SourceTypeApprovalRequest,
		SourceID:     approvalID,
		Title:        "审批待处理",
		Actions:      []Action{{Key: "approve", Label: "同意"}},
	})
	if err != nil {
		t.Fatalf("upsert item: %v", err)
	}

	_, _, err = service.ExecuteAction(context.Background(), ExecuteActionRequest{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		ItemID:      item.ID,
		Action:      "approve",
	})
	if !errors.Is(err, ErrActionForbidden) {
		t.Fatalf("expected forbidden action, got %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("expected source resolver not to be called, got %d calls", resolver.calls)
	}
}

func TestServiceRejectsRequiredCommentActionsWithoutComment(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resolver := &fakeApprovalResolver{}
	service.SetApprovalActionResolver(resolver)
	tenantID := uuid.New()
	actorUserID := uuid.New()
	item, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:     tenantID,
		TargetUserID: actorUserID,
		Scope:        "personal",
		ItemType:     ItemTypeApproval,
		SourceType:   SourceTypeApprovalRequest,
		SourceID:     uuid.New(),
		Title:        "审批待处理",
		Actions:      []Action{{Key: "reject", Label: "驳回", RequiresComment: true}},
	})
	if err != nil {
		t.Fatalf("upsert item: %v", err)
	}

	_, _, err = service.ExecuteAction(context.Background(), ExecuteActionRequest{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		ItemID:      item.ID,
		Action:      "reject",
		Comment:     "   ",
	})
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("expected invalid action, got %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("expected source resolver not to be called, got %d calls", resolver.calls)
	}
}

func TestServiceExecutesDefaultApprovalActionsWithDecisionKeys(t *testing.T) {
	tests := []struct {
		action  string
		comment string
	}{
		{action: "approved"},
		{action: "rejected", comment: "风险过高"},
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			repo := newMemoryRepository()
			service, err := NewService(repo)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			resolver := &fakeApprovalResolver{}
			service.SetApprovalActionResolver(resolver)
			tenantID := uuid.New()
			actorUserID := uuid.New()
			item, err := service.UpsertItem(context.Background(), UpsertItemRequest{
				TenantID:     tenantID,
				TargetUserID: actorUserID,
				Scope:        "personal",
				ItemType:     ItemTypeApproval,
				SourceType:   SourceTypeApprovalRequest,
				SourceID:     uuid.New(),
				Title:        "审批待处理",
			})
			if err != nil {
				t.Fatalf("upsert item: %v", err)
			}
			resolver.onResolve = func(req SourceActionRequest) {
				repo.resolveItem(t, tenantID, item.ID, req.Action)
			}

			_, _, err = service.ExecuteAction(context.Background(), ExecuteActionRequest{
				TenantID:    tenantID,
				ActorUserID: actorUserID,
				ItemID:      item.ID,
				Action:      tc.action,
				Comment:     tc.comment,
			})
			if err != nil {
				t.Fatalf("execute default action %q: %v", tc.action, err)
			}
			if resolver.calls != 1 {
				t.Fatalf("expected resolver call, got %d", resolver.calls)
			}
			if resolver.last.Action != tc.action {
				t.Fatalf("expected resolver action %q, got %q", tc.action, resolver.last.Action)
			}
			if resolver.last.Action == "approve" || resolver.last.Action == "reject" {
				t.Fatalf("expected downstream decision key, got legacy action %q", resolver.last.Action)
			}
		})
	}
}

func TestDefaultActionsRequireCommentsForNegativeDecisions(t *testing.T) {
	actions := DefaultActions(ItemTypeApproval)
	byKey := make(map[string]Action, len(actions))
	for _, action := range actions {
		byKey[action.Key] = action
	}

	if _, ok := byKey["approved"]; !ok {
		t.Fatalf("expected approved default action, got %#v", actions)
	}
	if rejected, ok := byKey["rejected"]; !ok || !rejected.RequiresComment {
		t.Fatalf("expected rejected action requiring comment, got %#v", rejected)
	}
	if needsMoreEvidence, ok := byKey["needs_more_evidence"]; !ok || !needsMoreEvidence.RequiresComment {
		t.Fatalf("expected needs_more_evidence action requiring comment, got %#v", needsMoreEvidence)
	}
}

func TestServiceRoutesProjectDecisionActions(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorUserID := uuid.New()
	decisionID := uuid.New()
	projectID := uuid.New()
	resolver := &fakeProjectDecisionResolver{
		result: SourceActionResult{
			SourceType: string(SourceTypeProjectDecisionRequest),
			SourceID:   decisionID,
			Status:     string(StatusResolved),
		},
	}
	service.SetProjectDecisionActionResolver(resolver)
	item, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:        tenantID,
		TargetUserID:    actorUserID,
		Scope:           "personal",
		ItemType:        ItemTypeProjectDecision,
		SourceType:      SourceTypeProjectDecisionRequest,
		SourceID:        decisionID,
		SourceProjectID: &projectID,
		Title:           "项目决策待处理",
		Actions:         []Action{{Key: "approve", Label: "同意"}},
	})
	if err != nil {
		t.Fatalf("upsert item: %v", err)
	}
	resolver.onResolve = func(req SourceActionRequest) {
		repo.resolveItem(t, tenantID, item.ID, req.Action)
	}

	updated, result, err := service.ExecuteAction(context.Background(), ExecuteActionRequest{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		ItemID:      item.ID,
		Action:      " approve ",
		Comment:     " 同意继续 ",
		Payload:     map[string]any{"accepted": true},
	})
	if err != nil {
		t.Fatalf("execute project decision action: %v", err)
	}
	if updated.Status != StatusResolved {
		t.Fatalf("expected resolved item, got %s", updated.Status)
	}
	if result.SourceType != string(SourceTypeProjectDecisionRequest) || result.SourceID != decisionID {
		t.Fatalf("unexpected source result: %#v", result)
	}
	if resolver.calls != 1 {
		t.Fatalf("expected one project decision resolver call, got %d", resolver.calls)
	}
	if resolver.last.SourceID != decisionID || resolver.last.SourceProjectID == nil || *resolver.last.SourceProjectID != projectID {
		t.Fatalf("unexpected source request: %#v", resolver.last)
	}
	if resolver.last.Action != "approve" || resolver.last.Comment != "同意继续" {
		t.Fatalf("expected normalized action/comment, got action=%q comment=%q", resolver.last.Action, resolver.last.Comment)
	}
}

func TestServiceRejectsMalformedProjectDecisionActionsWithoutProjectSource(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	resolver := &fakeProjectDecisionResolver{}
	service.SetProjectDecisionActionResolver(resolver)
	tenantID := uuid.New()
	actorUserID := uuid.New()
	itemID := uuid.New()
	now := time.Now().UTC()
	repo.itemsByID[itemID] = Item{
		ID:             itemID,
		TenantID:       tenantID,
		TargetUserID:   actorUserID,
		Scope:          "personal",
		ItemType:       ItemTypeProjectDecision,
		SourceType:     SourceTypeProjectDecisionRequest,
		SourceID:       uuid.New(),
		Title:          "缺少项目来源的决策",
		Status:         StatusOpen,
		Actions:        []Action{{Key: "approve", Label: "同意"}},
		ContextPayload: map[string]any{},
		DeepLink:       map[string]any{},
		LastActivityAt: now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	_, _, err = service.ExecuteAction(context.Background(), ExecuteActionRequest{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		ItemID:      itemID,
		Action:      "approve",
	})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected source unavailable, got %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("expected source resolver not to be called, got %d calls", resolver.calls)
	}
}

func TestServiceListItemsHasMoreRequiresExtraFetchedItem(t *testing.T) {
	tests := []struct {
		name        string
		totalItems  int
		wantHasMore bool
	}{
		{name: "exact limit", totalItems: 2, wantHasMore: false},
		{name: "limit plus one", totalItems: 3, wantHasMore: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMemoryRepository()
			service, err := NewService(repo)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}
			tenantID := uuid.New()
			actorUserID := uuid.New()
			baseTime := time.Now().UTC().Add(-time.Hour)
			for i := 0; i < tc.totalItems; i++ {
				_, err := service.UpsertItem(context.Background(), UpsertItemRequest{
					TenantID:       tenantID,
					TargetUserID:   actorUserID,
					Scope:          "personal",
					ItemType:       ItemTypeApproval,
					SourceType:     SourceTypeApprovalRequest,
					SourceID:       uuid.New(),
					Title:          "审批待处理",
					LastActivityAt: baseTime.Add(time.Duration(i) * time.Minute),
				})
				if err != nil {
					t.Fatalf("upsert item %d: %v", i, err)
				}
			}

			result, err := service.ListItems(context.Background(), ListItemsRequest{
				TenantID:    tenantID,
				ActorUserID: actorUserID,
				View:        ViewMine,
				Limit:       2,
			})
			if err != nil {
				t.Fatalf("list items: %v", err)
			}
			if result.HasMore != tc.wantHasMore {
				t.Fatalf("expected hasMore=%v, got %v", tc.wantHasMore, result.HasMore)
			}
			if result.Limit != 2 {
				t.Fatalf("expected result limit 2, got %d", result.Limit)
			}
			if len(result.Items) != 2 {
				t.Fatalf("expected returned item count trimmed to 2, got %d", len(result.Items))
			}
		})
	}
}

func TestServiceListsMineAndTeamItems(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorUserID := uuid.New()
	otherUserID := uuid.New()
	projectID := uuid.New()
	baseTime := time.Now().UTC().Add(-time.Hour)

	mineOpen, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:       tenantID,
		TargetUserID:   actorUserID,
		Scope:          "personal",
		ItemType:       ItemTypeApproval,
		SourceType:     SourceTypeApprovalRequest,
		SourceID:       uuid.New(),
		Title:          "我的高风险审批",
		RiskLevel:      "high",
		LastActivityAt: baseTime.Add(3 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert mine open: %v", err)
	}
	teamOpen, err := service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:        tenantID,
		TargetUserID:    otherUserID,
		Scope:           "personal",
		ItemType:        ItemTypeProjectDecision,
		SourceType:      SourceTypeProjectDecisionRequest,
		SourceID:        uuid.New(),
		SourceProjectID: &projectID,
		Title:           "团队项目决策",
		RiskLevel:       "high",
		LastActivityAt:  baseTime.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert team open: %v", err)
	}
	resolvedAt := baseTime.Add(4 * time.Minute)
	_, err = service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:       tenantID,
		TargetUserID:   actorUserID,
		Scope:          "personal",
		ItemType:       ItemTypeApproval,
		SourceType:     SourceTypeApprovalRequest,
		SourceID:       uuid.New(),
		Title:          "已处理审批",
		Status:         StatusResolved,
		ResolvedAt:     &resolvedAt,
		LastActivityAt: baseTime.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("upsert resolved: %v", err)
	}

	mine, err := service.ListItems(context.Background(), ListItemsRequest{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		View:        ViewMine,
	})
	if err != nil {
		t.Fatalf("list mine: %v", err)
	}
	if len(mine.Items) != 1 || mine.Items[0].ID != mineOpen.ID {
		t.Fatalf("expected only mine open item, got %#v", mine.Items)
	}
	if mine.OpenCount != 1 || mine.HighRiskCount != 1 {
		t.Fatalf("expected mine counts open=1 high=1, got open=%d high=%d", mine.OpenCount, mine.HighRiskCount)
	}

	team, err := service.ListItems(context.Background(), ListItemsRequest{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
		View:        ViewTeam,
	})
	if err != nil {
		t.Fatalf("list team: %v", err)
	}
	if len(team.Items) != 2 {
		t.Fatalf("expected two team open items, got %d", len(team.Items))
	}
	if team.Items[0].ID != mineOpen.ID || team.Items[1].ID != teamOpen.ID {
		t.Fatalf("expected stable activity ordering, got %#v", team.Items)
	}
	if team.OpenCount != 2 || team.HighRiskCount != 2 {
		t.Fatalf("expected team counts open=2 high=2, got open=%d high=%d", team.OpenCount, team.HighRiskCount)
	}
}

func validApprovalUpsertRequest() UpsertItemRequest {
	approvalID := uuid.New()
	return UpsertItemRequest{
		TenantID:                uuid.New(),
		TargetUserID:            uuid.New(),
		Scope:                   "personal",
		ItemType:                ItemTypeApproval,
		SourceType:              SourceTypeApprovalRequest,
		SourceID:                approvalID,
		SourceApprovalRequestID: &approvalID,
		Title:                   "审批待处理",
	}
}

type memoryRepository struct {
	mu                          sync.Mutex
	itemsByID                   map[uuid.UUID]Item
	itemsBySource               map[string]uuid.UUID
	itemsByApproval             map[uuid.UUID]uuid.UUID
	upsertItemCalls             int
	upsertByApprovalSourceCalls int
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		itemsByID:       map[uuid.UUID]Item{},
		itemsBySource:   map[string]uuid.UUID{},
		itemsByApproval: map[uuid.UUID]uuid.UUID{},
	}
}

func (r *memoryRepository) UpsertItem(_ context.Context, req UpsertItemRequest) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.upsertItemCalls++
	sourceKey := sourceKey(req.TenantID, req.SourceType, req.SourceID)
	item, ok := r.itemByID(r.itemsBySource[sourceKey])
	if !ok {
		item.ID = uuid.New()
		item.CreatedAt = time.Now().UTC()
	}
	item = applyUpsert(item, req)
	r.itemsByID[item.ID] = item
	r.itemsBySource[sourceKey] = item.ID
	if req.SourceApprovalRequestID != nil {
		r.itemsByApproval[*req.SourceApprovalRequestID] = item.ID
	}
	return item, nil
}

func (r *memoryRepository) UpsertItemByApprovalSource(_ context.Context, req UpsertItemRequest) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.upsertByApprovalSourceCalls++
	if req.SourceApprovalRequestID == nil {
		return Item{}, ErrInvalidItem
	}
	item, ok := r.itemByID(r.itemsByApproval[*req.SourceApprovalRequestID])
	if !ok {
		item.ID = uuid.New()
		item.CreatedAt = time.Now().UTC()
	}
	item = applyUpsert(item, req)
	r.itemsByID[item.ID] = item
	r.itemsBySource[sourceKey(req.TenantID, req.SourceType, req.SourceID)] = item.ID
	r.itemsByApproval[*req.SourceApprovalRequestID] = item.ID
	return item, nil
}

func (r *memoryRepository) GetItem(_ context.Context, tenantID, itemID uuid.UUID) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.itemsByID[itemID]
	if !ok || item.TenantID != tenantID {
		return Item{}, ErrItemNotFound
	}
	return item, nil
}

func (r *memoryRepository) ListItems(_ context.Context, req ListItemsRequest) ([]Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]Item, 0, len(r.itemsByID))
	for _, item := range r.itemsByID {
		if !matchesListRequest(item, req) {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].LastActivityAt.Equal(items[j].LastActivityAt) {
			return items[i].LastActivityAt.After(items[j].LastActivityAt)
		}
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID.String() > items[j].ID.String()
	})
	if req.Offset > int32(len(items)) {
		return []Item{}, nil
	}
	end := int(req.Offset + req.Limit)
	if end > len(items) {
		end = len(items)
	}
	return append([]Item(nil), items[req.Offset:end]...), nil
}

func (r *memoryRepository) CountOpenItems(_ context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	for _, item := range r.itemsByID {
		if item.TenantID != tenantID || item.Status != StatusOpen {
			continue
		}
		if targetUserID != nil && item.TargetUserID != *targetUserID {
			continue
		}
		count++
	}
	return count, nil
}

func (r *memoryRepository) CountHighRiskOpenItems(_ context.Context, tenantID uuid.UUID, targetUserID *uuid.UUID) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	for _, item := range r.itemsByID {
		if item.TenantID != tenantID || item.Status != StatusOpen || item.RiskLevel == nil || *item.RiskLevel != "high" {
			continue
		}
		if targetUserID != nil && item.TargetUserID != *targetUserID {
			continue
		}
		count++
	}
	return count, nil
}

func (r *memoryRepository) resolveItem(t *testing.T, tenantID, itemID uuid.UUID, action string) {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.itemsByID[itemID]
	if !ok || item.TenantID != tenantID {
		t.Fatalf("expected item %s to resolve", itemID)
	}
	now := time.Now().UTC()
	item.Status = StatusResolved
	item.ResolvedAt = &now
	item.LastActivityAt = now
	item.UpdatedAt = now
	item.ContextPayload["resolved_action"] = action
	r.itemsByID[item.ID] = item
}

func (r *memoryRepository) itemByID(itemID uuid.UUID) (Item, bool) {
	if itemID == uuid.Nil {
		return Item{}, false
	}
	item, ok := r.itemsByID[itemID]
	return item, ok
}

func applyUpsert(item Item, req UpsertItemRequest) Item {
	now := time.Now().UTC()
	if item.ID == uuid.Nil {
		item.ID = uuid.New()
		item.CreatedAt = now
	}
	item.TenantID = req.TenantID
	item.TeamID = req.TeamID
	item.TargetUserID = req.TargetUserID
	item.Scope = req.Scope
	item.ItemType = req.ItemType
	item.SourceType = req.SourceType
	item.SourceID = req.SourceID
	item.SourceProjectID = req.SourceProjectID
	item.SourceTaskID = req.SourceTaskID
	item.SourceApprovalRequestID = req.SourceApprovalRequestID
	item.Title = req.Title
	item.Summary = testStringValue(req.Summary)
	item.RiskLevel = testStringValue(req.RiskLevel)
	item.Priority = testStringValue(req.Priority)
	item.Status = req.Status
	item.Actions = append([]Action(nil), req.Actions...)
	item.ContextPayload = cloneMap(req.ContextPayload)
	item.DeepLink = cloneMap(req.DeepLink)
	item.ResolvedAt = req.ResolvedAt
	item.LastActivityAt = req.LastActivityAt
	if item.LastActivityAt.IsZero() {
		item.LastActivityAt = now
	}
	item.UpdatedAt = now
	return item
}

func matchesListRequest(item Item, req ListItemsRequest) bool {
	if item.TenantID != req.TenantID || item.Status != req.Status {
		return false
	}
	if req.TargetUserID != nil && item.TargetUserID != *req.TargetUserID {
		return false
	}
	if req.ItemType != nil && item.ItemType != *req.ItemType {
		return false
	}
	if req.RiskLevel != nil && (item.RiskLevel == nil || *item.RiskLevel != *req.RiskLevel) {
		return false
	}
	if req.ProjectID != nil && (item.SourceProjectID == nil || *item.SourceProjectID != *req.ProjectID) {
		return false
	}
	return true
}

func sourceKey(tenantID uuid.UUID, sourceType SourceType, sourceID uuid.UUID) string {
	return tenantID.String() + ":" + string(sourceType) + ":" + sourceID.String()
}

type fakeApprovalResolver struct {
	calls     int
	last      SourceActionRequest
	onResolve func(SourceActionRequest)
}

func (r *fakeApprovalResolver) ResolveApprovalAction(_ context.Context, req SourceActionRequest) (SourceActionResult, error) {
	r.calls++
	r.last = req
	if r.onResolve != nil {
		r.onResolve(req)
	}
	return SourceActionResult{SourceType: string(SourceTypeApprovalRequest), SourceID: req.SourceID, Status: string(StatusResolved)}, nil
}

type fakeProjectDecisionResolver struct {
	calls     int
	last      SourceActionRequest
	result    SourceActionResult
	onResolve func(SourceActionRequest)
}

func (r *fakeProjectDecisionResolver) ResolveProjectDecisionAction(_ context.Context, req SourceActionRequest) (SourceActionResult, error) {
	r.calls++
	r.last = req
	if r.onResolve != nil {
		r.onResolve(req)
	}
	return r.result, nil
}

func testStringValue(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cloneMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
