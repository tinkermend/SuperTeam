package inbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

func TestApprovalProjectorAdapterUpsertsApprovalRequest(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewApprovalProjectorAdapter(service)
	summary := "Approve high risk deployment"
	risk := "high"
	createdAt := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	request := approval.ApprovalRequest{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ResourceType:   "project",
		ResourceID:     uuid.New(),
		TargetUserID:   uuid.New(),
		DecisionType:   "deploy",
		Title:          "Deployment approval",
		Summary:        &summary,
		RiskLevel:      &risk,
		Status:         approval.ApprovalStatusPending,
		ContextPayload: map[string]any{"change": "deploy"},
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}

	if err := adapter.UpsertApprovalRequest(context.Background(), request); err != nil {
		t.Fatalf("upsert approval request: %v", err)
	}
	item, err := repo.GetItem(context.Background(), request.TenantID, repo.itemsByApproval[request.ID])
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusOpen || item.ItemType != ItemTypeApproval || item.SourceType != SourceTypeApprovalRequest {
		t.Fatalf("unexpected projected item: %#v", item)
	}
	if item.SourceID != request.ID || item.SourceApprovalRequestID == nil || *item.SourceApprovalRequestID != request.ID {
		t.Fatalf("expected source approval request id %s, got source=%s approval=%#v", request.ID, item.SourceID, item.SourceApprovalRequestID)
	}
	if item.Summary == nil || *item.Summary != summary || item.RiskLevel == nil || *item.RiskLevel != risk {
		t.Fatalf("expected summary and risk from request, got summary=%#v risk=%#v", item.Summary, item.RiskLevel)
	}
	if !item.LastActivityAt.Equal(updatedAt) {
		t.Fatalf("expected last activity %s, got %s", updatedAt, item.LastActivityAt)
	}
	if item.DeepLink["route"] != "/approvals" || item.DeepLink["approval_request_id"] != request.ID.String() {
		t.Fatalf("unexpected deep link: %#v", item.DeepLink)
	}
	if item.ContextPayload["change"] != "deploy" || len(item.Actions) == 0 {
		t.Fatalf("expected context payload and default actions, got %#v actions=%#v", item.ContextPayload, item.Actions)
	}
}

func TestApprovalProjectorAdapterResolvesApprovalRequest(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewApprovalProjectorAdapter(service)
	resolvedAt := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	request := approval.ApprovalRequest{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ResourceID:     uuid.New(),
		TargetUserID:   uuid.New(),
		DecisionType:   "deploy",
		Title:          "Deployment approval",
		Status:         approval.ApprovalStatusApproved,
		ContextPayload: map[string]any{},
		CreatedAt:      resolvedAt.Add(-time.Hour),
		UpdatedAt:      resolvedAt,
		ResolvedAt:     &resolvedAt,
	}

	if err := adapter.ResolveApprovalRequest(context.Background(), request); err != nil {
		t.Fatalf("resolve approval request: %v", err)
	}
	item, err := repo.GetItem(context.Background(), request.TenantID, repo.itemsByApproval[request.ID])
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusResolved || item.ResolvedAt == nil || !item.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("expected resolved item at %s, got status=%s resolved=%#v", resolvedAt, item.Status, item.ResolvedAt)
	}
}

func TestApprovalProjectorAdapterSkipsWhenDecisionOwnsApproval(t *testing.T) {
	// §5.4.1: once DecisionProjector owns the card, approval resolve must not
	// overwrite kind/why/progress back to a bare approval projection.
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewApprovalProjectorAdapter(service)
	adapter.SetDecisionChecker(stubDecisionChecker{owned: true})

	tenantID := uuid.New()
	approvalID := uuid.New()
	decisionID := uuid.New()
	projectID := uuid.New()
	_, err = service.UpsertItem(context.Background(), UpsertItemRequest{
		TenantID:                tenantID,
		TargetUserID:            uuid.New(),
		Scope:                   "personal",
		ItemType:                ItemTypeProjectDecision,
		SourceType:              SourceTypeProjectDecisionRequest,
		SourceID:                decisionID,
		SourceProjectID:         &projectID,
		SourceApprovalRequestID: &approvalID,
		Title:                   "验收签署",
		ContextPayload:          map[string]any{"kind": "acceptance_sign", "why": "待人类签署"},
		Status:                  StatusOpen,
	})
	if err != nil {
		t.Fatalf("seed decision card: %v", err)
	}

	resolvedAt := time.Now().UTC()
	if err := adapter.ResolveApprovalRequest(context.Background(), approval.ApprovalRequest{
		ID:           approvalID,
		TenantID:     tenantID,
		ResourceID:   projectID,
		TargetUserID: uuid.New(),
		DecisionType: "demand_acceptance",
		Title:        "approval overwrite attempt",
		Status:       approval.ApprovalStatusApproved,
		ResolvedAt:   &resolvedAt,
		CreatedAt:    resolvedAt.Add(-time.Hour),
		UpdatedAt:    resolvedAt,
	}); err != nil {
		t.Fatalf("resolve approval: %v", err)
	}

	item, err := service.GetItemByApprovalSource(context.Background(), tenantID, approvalID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if item.ItemType != ItemTypeProjectDecision || item.SourceType != SourceTypeProjectDecisionRequest {
		t.Fatalf("approval projector overwrote decision ownership: %#v", item)
	}
	if item.ContextPayload["kind"] != "acceptance_sign" || item.ContextPayload["why"] != "待人类签署" {
		t.Fatalf("approval projector wiped decision context: %#v", item.ContextPayload)
	}
	if item.Status != StatusOpen {
		t.Fatalf("expected open decision card untouched, got %s", item.Status)
	}
}

type stubDecisionChecker struct{ owned bool }

func (s stubDecisionChecker) HasProjectDecisionForApproval(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
	return s.owned, nil
}

func TestDecisionProjectorAdapterUpsertsProjectDecision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	summary := "Needs owner approval"
	risk := "medium"
	taskID := uuid.New()
	approvalID := uuid.New()
	createdAt := time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: approvalID,
		ProjectTaskID:     &taskID,
		TargetUserID:      uuid.New(),
		DecisionType:      "route",
		TitleSnapshot:     "Review route decision",
		SummarySnapshot:   &summary,
		RiskLevelSnapshot: &risk,
		StatusSnapshot:    "pending",
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt.Add(time.Minute),
	}

	if err := adapter.UpsertProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("upsert project decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusOpen || item.ItemType != ItemTypeProjectDecision || item.SourceType != SourceTypeProjectDecisionRequest {
		t.Fatalf("unexpected projected item: %#v", item)
	}
	if item.SourceProjectID == nil || *item.SourceProjectID != decision.ProjectID || item.SourceTaskID == nil || *item.SourceTaskID != taskID {
		t.Fatalf("expected project/task source ids, got project=%#v task=%#v", item.SourceProjectID, item.SourceTaskID)
	}
	if item.SourceApprovalRequestID == nil || *item.SourceApprovalRequestID != approvalID {
		t.Fatalf("expected approval source id %s, got %#v", approvalID, item.SourceApprovalRequestID)
	}
	// F3(§5.4.3): server computes the single authoritative primary_surface. A
	// non-demand project decision lands on the project's approval tab focused on
	// the decision (mirrors the old web-side resolveProjectDecisionPath, now
	// server-owned). route mirrors primary_surface; anchor still carries the id.
	expectedSurface := "/projects/" + decision.ProjectID.String() + "?tab=approval&focus=" + decision.ID.String()
	if item.DeepLink["route"] != expectedSurface || item.DeepLink["primary_surface"] != expectedSurface || item.DeepLink["anchor"] != decision.ID.String() {
		t.Fatalf("unexpected deep link: %#v", item.DeepLink)
	}
	if item.Summary == nil || *item.Summary != summary || item.RiskLevel == nil || *item.RiskLevel != risk || len(item.Actions) == 0 {
		t.Fatalf("expected decision details, got summary=%#v risk=%#v actions=%#v", item.Summary, item.RiskLevel, item.Actions)
	}
}

func TestDecisionProjectorAdapterUsesInboxContextAndDemandDeepLink(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	demandID := uuid.New()
	createdAt := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ProjectID:      uuid.New(),
		TargetUserID:   uuid.New(),
		DecisionType:   "project_acceptance",
		TitleSnapshot:  "验收 · 帮我分析 Claude Code",
		StatusSnapshot: "pending",
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
		InboxContext: map[string]any{
			"decision_type":     "project_acceptance",
			"primary_demand_id": demandID.String(),
			"project_name":      "测试项目",
			"demands": []map[string]any{
				{"id": demandID.String(), "title": "帮我分析 Claude Code", "status": "completed"},
			},
		},
	}

	if err := adapter.UpsertProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("upsert project decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.DeepLink["route"] != "/projects/"+decision.ProjectID.String()+"?tab=closure" {
		t.Fatalf("expected closure tab deep link for project_acceptance, got %#v", item.DeepLink)
	}
	if item.ContextPayload["primary_demand_id"] != demandID.String() {
		t.Fatalf("expected InboxContext projected into ContextPayload, got %#v", item.ContextPayload)
	}
	if item.ContextPayload["kind"] != "closure_confirm" {
		t.Fatalf("expected kind=closure_confirm, got %#v", item.ContextPayload["kind"])
	}
	if item.ContextPayload["why"] == nil || item.ContextPayload["why"] == "" {
		t.Fatalf("expected why stamped on closure card, got %#v", item.ContextPayload["why"])
	}
	progress, _ := item.ContextPayload["progress"].(map[string]any)
	if progress == nil || progress["step"] != 4 {
		t.Fatalf("expected progress step=4 for closure_confirm, got %#v", item.ContextPayload["progress"])
	}
}

// TestDecisionProjectorAdapterProjectsHumanTaskKindAndLayer pins P1.6 §4.2: the
// projector stamps the canonical HumanTask kind + layer (and persists
// decision_type) into the item's context so the console can group/label by
// human-task semantics without the internal decision_type being renamed.
func TestDecisionProjectorAdapterProjectsHumanTaskKindAndLayer(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	cases := []struct {
		decisionType string
		kind         string
		layer        string
	}{
		{"demand_acceptance", "acceptance_sign", "demand"},
		{"project_task_approval", "dispatch_release", "task"},
		{"project_task_acceptance", "downstream_release", "task"},
		{"project_acceptance", "closure_confirm", "project"},
		{"plan_review", "plan_review", "demand"},
		{"planning_gap", "planning_gap", "demand"},
	}
	for _, tc := range cases {
		decision := project.DecisionRequest{
			ID:             uuid.New(),
			TenantID:       uuid.New(),
			ProjectID:      uuid.New(),
			TargetUserID:   uuid.New(),
			DecisionType:   tc.decisionType,
			TitleSnapshot:  "待办",
			StatusSnapshot: "pending",
		}
		if err := adapter.UpsertProjectDecisionRequest(context.Background(), decision); err != nil {
			t.Fatalf("upsert %s: %v", tc.decisionType, err)
		}
		itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
		item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
		if err != nil {
			t.Fatalf("get item %s: %v", tc.decisionType, err)
		}
		if item.ContextPayload["kind"] != tc.kind {
			t.Fatalf("%s kind = %v, want %v", tc.decisionType, item.ContextPayload["kind"], tc.kind)
		}
		if item.ContextPayload["layer"] != tc.layer {
			t.Fatalf("%s layer = %v, want %v", tc.decisionType, item.ContextPayload["layer"], tc.layer)
		}
		if item.ContextPayload["decision_type"] != tc.decisionType {
			t.Fatalf("%s decision_type = %v, want %v", tc.decisionType, item.ContextPayload["decision_type"], tc.decisionType)
		}
	}
}

func TestDecisionProjectorAdapterOmitsEmptyApprovalSource(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	taskID := uuid.New()
	decision := project.DecisionRequest{
		ID:             uuid.New(),
		TenantID:       uuid.New(),
		ProjectID:      uuid.New(),
		ProjectTaskID:  &taskID,
		TargetUserID:   uuid.New(),
		DecisionType:   "project_task_acceptance",
		TitleSnapshot:  "Review task result",
		StatusSnapshot: "pending",
		CreatedAt:      time.Date(2026, 6, 20, 15, 0, 0, 0, time.UTC),
		UpdatedAt:      time.Date(2026, 6, 20, 15, 0, 0, 0, time.UTC),
	}

	err = adapter.UpsertProjectDecisionRequest(context.Background(), decision)
	if err != nil {
		t.Fatalf("upsert project decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.SourceApprovalRequestID != nil {
		t.Fatalf("expected no approval source id, got %#v", item.SourceApprovalRequestID)
	}
}

func TestDecisionProjectorAdapterResolvesProjectDecision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	resolvedAt := time.Date(2026, 6, 12, 14, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: uuid.New(),
		TargetUserID:      uuid.New(),
		DecisionType:      "route",
		TitleSnapshot:     "Review route decision",
		StatusSnapshot:    "approved",
		CreatedAt:         resolvedAt.Add(-time.Hour),
		UpdatedAt:         resolvedAt,
		ResolvedAt:        &resolvedAt,
	}

	if err := adapter.ResolveProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("resolve project decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusResolved || item.ResolvedAt == nil || !item.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("expected resolved item at %s, got status=%s resolved=%#v", resolvedAt, item.Status, item.ResolvedAt)
	}
}

func TestDecisionProjectorAdapterResolvesRequestChangesDecision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	resolvedAt := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: uuid.New(),
		TargetUserID:      uuid.New(),
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "request_changes",
		CreatedAt:         resolvedAt.Add(-time.Hour),
		UpdatedAt:         resolvedAt,
		ResolvedAt:        &resolvedAt,
	}

	if err := adapter.ResolveProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("resolve project decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusResolved {
		t.Fatalf("expected request_changes decision to resolve inbox item, got status=%s", item.Status)
	}
}

// TestUpsertPlanningGapDecisionUsesCustomActions proves the decision projector
// gives a planning_gap decision item its own action vocabulary — 已补员/豁免/关闭 —
// instead of the generic approved/rejected/needs_more_evidence default. The web
// inbox-shell renders item.actions dynamically, so this is the sole source of the
// three buttons a human sees on a 规划缺口 item.
func TestUpsertPlanningGapDecisionUsesCustomActions(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	createdAt := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: uuid.New(),
		TargetUserID:      uuid.New(),
		DecisionType:      "planning_gap",
		TitleSnapshot:     "规划缺口：项目员工池无法满足审查独立性约束",
		StatusSnapshot:    "pending",
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}

	if err := adapter.UpsertProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("upsert planning_gap decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	keys := make([]string, 0, len(item.Actions))
	for _, action := range item.Actions {
		keys = append(keys, action.Key)
	}
	if len(keys) != 3 || keys[0] != "restaffed" || keys[1] != "exempted" || keys[2] != "rejected" {
		t.Fatalf("expected [restaffed exempted rejected] actions, got %#v", keys)
	}
	restaffed, ok := findAction(item.Actions, "restaffed")
	if !ok || restaffed.Label != "已补员，重新规划" {
		t.Fatalf("expected restaffed action labelled 已补员，重新规划, got %#v", restaffed)
	}
	exempted, ok := findAction(item.Actions, "exempted")
	if !ok || exempted.Label != "豁免约束并重规划" {
		t.Fatalf("expected exempted action labelled 豁免约束并重规划, got %#v", exempted)
	}
	closeAction, ok := findAction(item.Actions, "rejected")
	if !ok || closeAction.Label != "关闭" {
		t.Fatalf("expected rejected action labelled 关闭, got %#v", closeAction)
	}
}

// TestUpsertPlanningGapExemptedResolvesInboxItem proves a planning_gap decision
// resolved with exempted closes its inbox item exactly like restaffed — the
// exemption record is persisted and the demand is reopened + replanned, so
// nothing is left open here.
func TestUpsertPlanningGapExemptedResolvesInboxItem(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	resolvedAt := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: uuid.New(),
		TargetUserID:      uuid.New(),
		DecisionType:      "planning_gap",
		TitleSnapshot:     "规划缺口：豁免约束后重开",
		StatusSnapshot:    "exempted",
		CreatedAt:         resolvedAt.Add(-time.Hour),
		UpdatedAt:         resolvedAt,
		ResolvedAt:        &resolvedAt,
	}

	if err := adapter.ResolveProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("resolve planning_gap decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusResolved {
		t.Fatalf("expected exempted decision to resolve inbox item, got status=%s", item.Status)
	}
}

// TestUpsertPlanningGapRestaffedResolvesInboxItem proves a planning_gap decision
// resolved with restaffed closes its inbox item (the reopen replan opens fresh
// surfaces), rather than staying open the way an unknown status snapshot would.
func TestUpsertPlanningGapRestaffedResolvesInboxItem(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new inbox service: %v", err)
	}
	adapter := NewDecisionProjectorAdapter(service)
	resolvedAt := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          uuid.New(),
		ProjectID:         uuid.New(),
		ApprovalRequestID: uuid.New(),
		TargetUserID:      uuid.New(),
		DecisionType:      "planning_gap",
		TitleSnapshot:     "规划缺口：补员后重开",
		StatusSnapshot:    "restaffed",
		CreatedAt:         resolvedAt.Add(-time.Hour),
		UpdatedAt:         resolvedAt,
		ResolvedAt:        &resolvedAt,
	}

	if err := adapter.ResolveProjectDecisionRequest(context.Background(), decision); err != nil {
		t.Fatalf("resolve planning_gap decision: %v", err)
	}
	itemID := repo.itemsBySource[sourceKey(decision.TenantID, SourceTypeProjectDecisionRequest, decision.ID)]
	item, err := repo.GetItem(context.Background(), decision.TenantID, itemID)
	if err != nil {
		t.Fatalf("get projected item: %v", err)
	}
	if item.Status != StatusResolved {
		t.Fatalf("expected restaffed decision to resolve inbox item, got status=%s", item.Status)
	}
}

func TestApprovalActionAdapterResolvesApproval(t *testing.T) {
	repo := &approvalActionRepository{}
	service, err := approval.NewService(repo)
	if err != nil {
		t.Fatalf("new approval service: %v", err)
	}
	adapter := NewApprovalActionAdapter(service)
	sourceID := uuid.New()
	req := SourceActionRequest{
		TenantID:    uuid.New(),
		ActorUserID: uuid.New(),
		SourceID:    sourceID,
		Action:      "approved",
		Comment:     "ok",
		Payload:     map[string]any{"reason": "clear"},
	}

	result, err := adapter.ResolveApprovalAction(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve approval action: %v", err)
	}
	if repo.resolveInput.TenantID != req.TenantID || repo.resolveInput.ApprovalRequestID != sourceID || repo.resolveInput.DecidedByUserID != req.ActorUserID {
		t.Fatalf("unexpected approval resolve input: %#v", repo.resolveInput)
	}
	if repo.resolveInput.Decision != approval.ApprovalDecisionApproved || repo.resolveInput.Comment != "ok" || repo.resolveInput.Payload["reason"] != "clear" {
		t.Fatalf("unexpected approval decision input: %#v", repo.resolveInput)
	}
	if result.SourceType != string(SourceTypeApprovalRequest) || result.SourceID != sourceID || result.Status != "approved" {
		t.Fatalf("unexpected source result: %#v", result)
	}
}

func TestApprovalActionAdapterReturnsSourceUnavailableWithoutService(t *testing.T) {
	adapter := NewApprovalActionAdapter(nil)
	_, err := adapter.ResolveApprovalAction(context.Background(), SourceActionRequest{SourceID: uuid.New(), Action: "approved"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected source unavailable, got %v", err)
	}
}

func TestApprovalActionAdapterNormalizesSourceErrors(t *testing.T) {
	unknownErr := errors.New("approval store unavailable")
	tests := []struct {
		name    string
		source  error
		wantErr error
	}{
		{name: "invalid approval request", source: approval.ErrInvalidApprovalRequest, wantErr: ErrInvalidAction},
		{name: "approval not found", source: approval.ErrApprovalNotFound, wantErr: ErrSourceUnavailable},
		{name: "approval no rows", source: pgx.ErrNoRows, wantErr: ErrSourceUnavailable},
		{name: "approval already resolved", source: approval.ErrApprovalAlreadyResolved, wantErr: ErrInvalidAction},
		{name: "unknown", source: unknownErr, wantErr: unknownErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &approvalActionRepository{resolveErr: tt.source}
			service, err := approval.NewService(repo)
			if err != nil {
				t.Fatalf("new approval service: %v", err)
			}
			adapter := NewApprovalActionAdapter(service)

			_, err = adapter.ResolveApprovalAction(context.Background(), SourceActionRequest{
				TenantID:    uuid.New(),
				ActorUserID: uuid.New(),
				SourceID:    uuid.New(),
				Action:      "approved",
			})

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestProjectDecisionActionAdapterResolvesDecision(t *testing.T) {
	projectID := uuid.New()
	decisionID := uuid.New()
	repo := &projectActionRepository{
		project: project.Project{
			ID:               projectID,
			TenantID:         uuid.New(),
			Name:             "Customer rollout",
			Goal:             "Ship safely",
			HumanOwnerUserID: uuid.New(),
		},
		decision: project.DecisionRequest{
			ID:                decisionID,
			ProjectID:         projectID,
			ApprovalRequestID: uuid.New(),
			TargetUserID:      uuid.New(),
			DecisionType:      "route",
			TitleSnapshot:     "Review route decision",
			StatusSnapshot:    "pending",
		},
	}
	repo.decision.TenantID = repo.project.TenantID
	service, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, project.NoopCoordinatorSignalClient{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("new project service: %v", err)
	}
	adapter := NewProjectDecisionActionAdapter(service)
	req := SourceActionRequest{
		TenantID:        repo.project.TenantID,
		ActorUserID:     repo.project.HumanOwnerUserID,
		SourceID:        decisionID,
		SourceProjectID: &projectID,
		Action:          "approved",
		Comment:         "approved",
		Payload:         map[string]any{"reason": "clear"},
	}

	result, err := adapter.ResolveProjectDecisionAction(context.Background(), req)
	if err != nil {
		t.Fatalf("resolve project decision action: %v", err)
	}
	if repo.resolveReq.TenantID != req.TenantID || repo.resolveReq.ProjectID != projectID || repo.resolveReq.ID != decisionID || repo.resolveReq.StatusSnapshot != "approved" {
		t.Fatalf("unexpected project resolve request: %#v", repo.resolveReq)
	}
	if repo.event.ActorID != req.ActorUserID.String() || repo.event.Payload["decision"] != "approved" {
		t.Fatalf("unexpected decision event: %#v", repo.event)
	}
	if result.SourceType != string(SourceTypeProjectDecisionRequest) || result.SourceID != decisionID || result.Status != "approved" {
		t.Fatalf("unexpected source result: %#v", result)
	}
}

func TestProjectDecisionActionAdapterReturnsSourceUnavailableWithoutProjectID(t *testing.T) {
	service, err := project.NewServiceWithCoordinator(&projectActionRepository{}, project.NoopCoordinatorSignalClient{})
	if err != nil {
		t.Fatalf("new project service: %v", err)
	}
	adapter := NewProjectDecisionActionAdapter(service)
	_, err = adapter.ResolveProjectDecisionAction(context.Background(), SourceActionRequest{SourceID: uuid.New(), Action: "approved"})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("expected source unavailable, got %v", err)
	}
}

func TestProjectDecisionActionAdapterNormalizesSourceErrors(t *testing.T) {
	unknownErr := errors.New("project store unavailable")
	tests := []struct {
		name    string
		mutate  func(*projectActionRepository)
		wantErr error
	}{
		{
			name: "invalid decision request",
			mutate: func(repo *projectActionRepository) {
				repo.getDecisionErr = project.ErrInvalidProject
			},
			wantErr: ErrInvalidAction,
		},
		{
			name: "project not found",
			mutate: func(repo *projectActionRepository) {
				repo.getProjectErr = project.ErrProjectNotFound
			},
			wantErr: ErrSourceUnavailable,
		},
		{
			name: "project no rows",
			mutate: func(repo *projectActionRepository) {
				repo.getProjectErr = pgx.ErrNoRows
			},
			wantErr: ErrSourceUnavailable,
		},
		{
			name: "unknown",
			mutate: func(repo *projectActionRepository) {
				repo.getDecisionErr = unknownErr
			},
			wantErr: unknownErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectID := uuid.New()
			decisionID := uuid.New()
			repo := &projectActionRepository{
				project: project.Project{
					ID:               projectID,
					TenantID:         uuid.New(),
					Name:             "Customer rollout",
					Goal:             "Ship safely",
					HumanOwnerUserID: uuid.New(),
				},
				decision: project.DecisionRequest{
					ID:                decisionID,
					ProjectID:         projectID,
					ApprovalRequestID: uuid.New(),
					TargetUserID:      uuid.New(),
					DecisionType:      "route",
					TitleSnapshot:     "Review route decision",
					StatusSnapshot:    "pending",
				},
			}
			repo.decision.TenantID = repo.project.TenantID
			tt.mutate(repo)
			service, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, project.NoopCoordinatorSignalClient{}, nil, nil, nil)
			if err != nil {
				t.Fatalf("new project service: %v", err)
			}
			adapter := NewProjectDecisionActionAdapter(service)

			_, err = adapter.ResolveProjectDecisionAction(context.Background(), SourceActionRequest{
				TenantID:        repo.project.TenantID,
				ActorUserID:     repo.project.HumanOwnerUserID,
				SourceID:        decisionID,
				SourceProjectID: &projectID,
				Action:          "approved",
			})

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// any-of-N 资格由 project.ResolveDecision 判定:actor 既非负责人也非项目人类成员时,
// ErrProjectDecisionForbidden 必须映射为 403(ErrActionForbidden)而非 500。
func TestProjectDecisionActionAdapterMapsForbiddenForNonMember(t *testing.T) {
	projectID := uuid.New()
	decisionID := uuid.New()
	repo := &projectActionRepository{
		project: project.Project{
			ID:               projectID,
			TenantID:         uuid.New(),
			Name:             "Customer rollout",
			Goal:             "Ship safely",
			HumanOwnerUserID: uuid.New(),
		},
		decision: project.DecisionRequest{
			ID:             decisionID,
			ProjectID:      projectID,
			TargetUserID:   uuid.New(),
			DecisionType:   "plan_review",
			TitleSnapshot:  "确认项目计划版本",
			StatusSnapshot: "pending",
		},
	}
	repo.decision.TenantID = repo.project.TenantID
	service, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(repo, project.NoopCoordinatorSignalClient{}, nil, nil, nil)
	if err != nil {
		t.Fatalf("new project service: %v", err)
	}
	adapter := NewProjectDecisionActionAdapter(service)

	_, err = adapter.ResolveProjectDecisionAction(context.Background(), SourceActionRequest{
		TenantID:        repo.project.TenantID,
		ActorUserID:     uuid.New(), // 既非负责人也非项目成员
		SourceID:        decisionID,
		SourceProjectID: &projectID,
		Action:          "approved",
	})

	if !errors.Is(err, ErrActionForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
	var forbidden *DecisionForbiddenError
	if !errors.As(err, &forbidden) {
		t.Fatalf("expected DecisionForbiddenError, got %T", err)
	}
	if got := err.Error(); got != "只有该项目的人类成员（含负责人）可以处理该决策" {
		t.Fatalf("unexpected message: %q", got)
	}
}

type approvalActionRepository struct {
	resolveInput approval.ResolveRequestInput
	resolveErr   error
}

func (r *approvalActionRepository) CreateApprovalRequest(_ context.Context, input approval.CreateRequestInput, status approval.ApprovalStatus) (approval.ApprovalRequest, error) {
	return approval.ApprovalRequest{
		ID:           uuid.New(),
		TenantID:     input.TenantID,
		ResourceID:   input.ResourceID,
		TargetUserID: input.TargetUserID,
		Title:        input.Title,
		Status:       status,
	}, nil
}

func (r *approvalActionRepository) GetApprovalRequest(_ context.Context, tenantID, requestID uuid.UUID) (approval.ApprovalRequest, error) {
	return approval.ApprovalRequest{ID: requestID, TenantID: tenantID}, nil
}

func (r *approvalActionRepository) GetApprovalRequestByResource(_ context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) (approval.ApprovalRequest, error) {
	return approval.ApprovalRequest{
		ID:           resourceID,
		TenantID:     tenantID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Status:       approval.ApprovalStatusPending,
	}, nil
}

func (r *approvalActionRepository) ResolveApprovalRequest(_ context.Context, input approval.ResolveRequestInput, status approval.ApprovalStatus) (approval.ApprovalRequest, error) {
	if r.resolveErr != nil {
		return approval.ApprovalRequest{}, r.resolveErr
	}
	r.resolveInput = input
	return approval.ApprovalRequest{
		ID:           input.ApprovalRequestID,
		TenantID:     input.TenantID,
		TargetUserID: input.DecidedByUserID,
		Title:        "Resolved",
		Status:       status,
	}, nil
}

func (r *approvalActionRepository) CreateApprovalDecision(_ context.Context, input approval.ResolveRequestInput) (approval.ApprovalDecisionRecord, error) {
	return approval.ApprovalDecisionRecord{
		ID:                uuid.New(),
		TenantID:          input.TenantID,
		ApprovalRequestID: input.ApprovalRequestID,
		DecidedByUserID:   input.DecidedByUserID,
		Decision:          input.Decision,
		Payload:           input.Payload,
	}, nil
}

func (r *approvalActionRepository) ListPermissionApprovals(_ context.Context, _ approval.ListPermissionApprovalsInput) ([]approval.ApprovalRequest, error) {
	return nil, nil
}

func (r *approvalActionRepository) PermissionApprovalSummary(_ context.Context, _ approval.PermissionApprovalSummaryInput) (approval.PermissionApprovalSummary, error) {
	return approval.PermissionApprovalSummary{}, nil
}

type projectActionRepository struct {
	project.Repository
	project        project.Project
	decision       project.DecisionRequest
	event          project.AppendProjectEventRequest
	resolveReq     project.ResolveDecisionRequestRepositoryRequest
	getProjectErr  error
	getDecisionErr error
	resolveErr     error
}

func (r projectActionRepository) GetProject(_ context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	if r.getProjectErr != nil {
		return project.Project{}, r.getProjectErr
	}
	if r.project.TenantID != tenantID || r.project.ID != projectID {
		return project.Project{}, project.ErrProjectNotFound
	}
	return r.project, nil
}

func (r projectActionRepository) GetDecisionRequest(_ context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (project.DecisionRequest, error) {
	if r.getDecisionErr != nil {
		return project.DecisionRequest{}, r.getDecisionErr
	}
	if r.decision.TenantID != tenantID || r.decision.ProjectID != projectID || r.decision.ID != decisionRequestID {
		return project.DecisionRequest{}, project.ErrInvalidProject
	}
	return r.decision, nil
}

func (r *projectActionRepository) ListProjectMembers(_ context.Context, tenantID, projectID uuid.UUID) ([]project.ProjectMember, error) {
	return nil, nil
}

func (r *projectActionRepository) AppendProjectEvent(_ context.Context, event project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	r.event = event
	return project.ProjectEvent{
		ID:        uuid.New(),
		TenantID:  event.TenantID,
		ProjectID: event.ProjectID,
		EventType: event.EventType,
		ActorType: event.ActorType,
		ActorID:   event.ActorID,
		Payload:   event.Payload,
	}, nil
}

func (r *projectActionRepository) ResolveDecisionRequest(_ context.Context, req project.ResolveDecisionRequestRepositoryRequest) (project.DecisionRequest, error) {
	if r.resolveErr != nil {
		return project.DecisionRequest{}, r.resolveErr
	}
	r.resolveReq = req
	resolved := r.decision
	resolved.StatusSnapshot = req.StatusSnapshot
	resolved.ResolvedEventID = req.ResolvedEventID
	now := time.Now().UTC()
	resolved.ResolvedAt = &now
	return resolved, nil
}
