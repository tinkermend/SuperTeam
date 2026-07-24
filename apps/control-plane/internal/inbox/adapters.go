package inbox

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/project"
)

// DecisionByApprovalChecker reports whether a project_decision_request already
// owns the given approval_request_id (spec §5.4.1). When true, ApprovalProjector
// must not write/overwrite the inbox card — DecisionProjectorAdapter owns it.
type DecisionByApprovalChecker interface {
	HasProjectDecisionForApproval(ctx context.Context, tenantID, approvalRequestID uuid.UUID) (bool, error)
}

type ApprovalProjectorAdapter struct {
	service         *Service
	decisionChecker DecisionByApprovalChecker
}

func NewApprovalProjectorAdapter(service *Service) *ApprovalProjectorAdapter {
	return &ApprovalProjectorAdapter{service: service}
}

// SetDecisionChecker wires the §5.4.1 ownership check. Optional in unit tests;
// production wiring (app.go) always sets it.
func (a *ApprovalProjectorAdapter) SetDecisionChecker(checker DecisionByApprovalChecker) {
	if a != nil {
		a.decisionChecker = checker
	}
}

func (a *ApprovalProjectorAdapter) UpsertApprovalRequest(ctx context.Context, request approval.ApprovalRequest) error {
	return a.upsert(ctx, request)
}

func (a *ApprovalProjectorAdapter) ResolveApprovalRequest(ctx context.Context, request approval.ApprovalRequest) error {
	return a.upsert(ctx, request)
}

func (a *ApprovalProjectorAdapter) upsert(ctx context.Context, request approval.ApprovalRequest) error {
	if a == nil || a.service == nil {
		return ErrSourceUnavailable
	}
	// §5.4.1: one gate → one inbox card owned by one projector. When a project
	// decision request already links this approval, skip — otherwise resolve-time
	// approval projection overwrites decision kind/why/progress/context (F2).
	if a.decisionChecker != nil {
		owned, err := a.decisionChecker.HasProjectDecisionForApproval(ctx, request.TenantID, request.ID)
		if err != nil {
			return err
		}
		if owned {
			return nil
		}
	} else if existing, err := a.service.GetItemByApprovalSource(ctx, request.TenantID, request.ID); err == nil {
		if existing.SourceType == SourceTypeProjectDecisionRequest {
			return nil
		}
	} else if err != nil && !errors.Is(err, pgx.ErrNoRows) && !errors.Is(err, ErrItemNotFound) {
		return err
	}
	approvalID := request.ID
	_, err := a.service.UpsertItem(ctx, UpsertItemRequest{
		TenantID:                request.TenantID,
		TargetUserID:            request.TargetUserID,
		Scope:                   "personal",
		ItemType:                ItemTypeApproval,
		SourceType:              SourceTypeApprovalRequest,
		SourceID:                request.ID,
		SourceApprovalRequestID: &approvalID,
		Title:                   request.Title,
		Summary:                 stringValue(request.Summary),
		RiskLevel:               stringValue(request.RiskLevel),
		Status:                  statusFromApproval(request.Status),
		Actions:                 DefaultActions(ItemTypeApproval),
		ContextPayload:          request.ContextPayload,
		DeepLink: map[string]any{
			"route":               "/approvals",
			"approval_request_id": request.ID.String(),
		},
		ResolvedAt:     request.ResolvedAt,
		LastActivityAt: lastActivityAt(request.UpdatedAt, request.CreatedAt),
	})
	return err
}

type DecisionProjectorAdapter struct {
	service *Service
}

func NewDecisionProjectorAdapter(service *Service) *DecisionProjectorAdapter {
	return &DecisionProjectorAdapter{service: service}
}

func (a *DecisionProjectorAdapter) UpsertProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	return a.upsert(ctx, decision)
}

func (a *DecisionProjectorAdapter) ResolveProjectDecisionRequest(ctx context.Context, decision project.DecisionRequest) error {
	return a.upsert(ctx, decision)
}

func (a *DecisionProjectorAdapter) upsert(ctx context.Context, decision project.DecisionRequest) error {
	if a == nil || a.service == nil {
		return ErrSourceUnavailable
	}
	projectID := decision.ProjectID
	var approvalID *uuid.UUID
	if decision.ApprovalRequestID != uuid.Nil {
		id := decision.ApprovalRequestID
		approvalID = &id
	}
	// Copy the caller's InboxContext before augmenting so we never mutate the
	// decision's map in place.
	contextPayload := map[string]any{}
	for k, v := range decision.InboxContext {
		contextPayload[k] = v
	}
	// P1.6 (§4.2): expose the canonical HumanTask kind + layer as additive
	// read-model metadata so the console can group/label by human-task semantics
	// (计划确认 / 执行放行 / 阶段放行 / 验收签署 / 结项确认 / 异常处理) without renaming the
	// internal decision_type values. Also persist decision_type so it is reliably
	// present on the card (F2 noted demand_acceptance open state lacked it, which
	// broke the web decisionFraming copy).
	kind, layer := humanTaskKindAndLayer(decision.DecisionType)
	if kind != "" {
		contextPayload["kind"] = kind
	}
	if layer != "" {
		contextPayload["layer"] = layer
	}
	if strings.TrimSpace(decision.DecisionType) != "" {
		contextPayload["decision_type"] = decision.DecisionType
	}
	// P1.6 (§4.1): additive why / evidence / progress on the HumanTask read model.
	stampHumanTaskReadModel(contextPayload, kind, stringValue(decision.SummarySnapshot))
	// F3(§5.4.3): primary_surface 是唯一权威落点,服务端算一次。前端不得再各自推导深链
	// (删除 web 的 reviewHref / resolveWorkflowInstanceHref / resolveWorkflowTemplateHref),
	// 只读这里下发的 primary_surface,避免"同一待办在不同入口跳不同页"。
	surface := primarySurfaceForDecision(decision, contextPayload)
	deepLink := map[string]any{
		"route":           surface,
		"anchor":          decision.ID.String(),
		"primary_surface": surface,
	}
	_, err := a.service.UpsertItem(ctx, UpsertItemRequest{
		TenantID:                decision.TenantID,
		TargetUserID:            decision.TargetUserID,
		Scope:                   "personal",
		ItemType:                ItemTypeProjectDecision,
		SourceType:              SourceTypeProjectDecisionRequest,
		SourceID:                decision.ID,
		SourceProjectID:         &projectID,
		SourceTaskID:            decision.ProjectTaskID,
		SourceApprovalRequestID: approvalID,
		Title:                   decision.TitleSnapshot,
		Summary:                 stringValue(decision.SummarySnapshot),
		RiskLevel:               stringValue(decision.RiskLevelSnapshot),
		Status:                  statusFromDecisionSnapshot(decision.StatusSnapshot),
		Actions:                 DecisionActions(decision.DecisionType),
		ContextPayload:          contextPayload,
		DeepLink:                deepLink,
		ResolvedAt:              decision.ResolvedAt,
		LastActivityAt:          lastActivityAt(decision.UpdatedAt, decision.CreatedAt),
	})
	return err
}

// primarySurfaceForDecision computes THE single authoritative deep link (primary
// surface) for a project-decision inbox card, server-side (spec §5.4.3, treating
// F3). This is the sole source of truth for where a human-task card lands; the
// web inbox must not re-derive its own href from context:
//   - closure_confirm → project closure tab (never demand page; G3 / §4.2);
//   - dispatch/downstream release with a task id → project tasks tab focused;
//   - demand-linked decisions land on the demand's workflow page;
//   - other project decisions land on the project's approval tab focused on the
//     decision;
//   - anything without a project id falls back to the project page.
func primarySurfaceForDecision(decision project.DecisionRequest, contextPayload map[string]any) string {
	if decision.DecisionType == "project_acceptance" && decision.ProjectID != uuid.Nil {
		return "/projects/" + decision.ProjectID.String() + "?tab=closure"
	}
	if decision.ProjectTaskID != nil && *decision.ProjectTaskID != uuid.Nil && decision.ProjectID != uuid.Nil {
		switch decision.DecisionType {
		case "project_task_approval", "project_task_acceptance":
			return "/projects/" + decision.ProjectID.String() + "?tab=tasks&focus=" + decision.ProjectTaskID.String()
		}
	}
	if primaryDemandID, ok := contextPayload["primary_demand_id"].(string); ok && strings.TrimSpace(primaryDemandID) != "" {
		return "/workflows/" + strings.TrimSpace(primaryDemandID)
	}
	if demandID, ok := contextPayload["demand_id"].(string); ok && strings.TrimSpace(demandID) != "" {
		return "/workflows/" + strings.TrimSpace(demandID)
	}
	if decision.ProjectID != uuid.Nil {
		return "/projects/" + decision.ProjectID.String() + "?tab=approval&focus=" + decision.ID.String()
	}
	return "/projects/" + decision.ProjectID.String()
}

// stampHumanTaskReadModel fills additive HumanTask fields (§4.1): why (one Chinese
// sentence), evidence (criteria/demand excerpts already on the card context), and
// progress {step,total,label} for the closed-loop bar (§6.1).
func stampHumanTaskReadModel(contextPayload map[string]any, kind, summary string) {
	if contextPayload == nil {
		return
	}
	if why := humanTaskWhy(kind, summary); why != "" {
		contextPayload["why"] = why
	}
	if evidence := humanTaskEvidence(contextPayload); evidence != nil {
		contextPayload["evidence"] = evidence
	}
	if progress := humanTaskProgress(kind); progress != nil {
		contextPayload["progress"] = progress
	}
}

func humanTaskWhy(kind, summary string) string {
	switch kind {
	case "plan_review":
		return "计划版本需要你确认后才能开始执行"
	case "dispatch_release":
		return "高风险动作派发前需要你放行"
	case "downstream_release":
		return "本阶段产出需你确认后，下游任务才能继续"
	case "acceptance_sign":
		return "需求已执行完成，需要你签署验收判据以收敛本需求"
	case "closure_confirm":
		return "项目全部需求已终态，需要你确认结项并归档"
	case "planning_failed":
		return "规划失败，需要你重试、换人或关闭需求"
	case "planning_gap":
		return "规划缺口需要你补员或豁免后才能继续"
	case "task_failure_recovery":
		return "任务失败，需要你选择重试或取消下游"
	}
	if trimmed := strings.TrimSpace(summary); trimmed != "" {
		return trimmed
	}
	return ""
}

// humanTaskEvidence prefers pending_criteria_detail (acceptance_sign), else the
// demands list on closure cards. Returns nil when neither is present.
func humanTaskEvidence(contextPayload map[string]any) any {
	if detail, ok := contextPayload["pending_criteria_detail"]; ok && detail != nil {
		return detail
	}
	if demands, ok := contextPayload["demands"]; ok && demands != nil {
		return demands
	}
	return nil
}

// humanTaskProgress returns the closed-loop step marker for §6.1 progress bars.
// Steps: 1 计划确认 → 2 执行/阶段放行 → 3 验收签署 → 4 结项确认.
func humanTaskProgress(kind string) map[string]any {
	switch kind {
	case "plan_review":
		return map[string]any{"step": 1, "total": 4, "label": "计划确认 待你 → 执行 未开始 → 验收 未开始 → 结项 未开始"}
	case "dispatch_release":
		return map[string]any{"step": 2, "total": 4, "label": "计划 已过 → 执行放行 待你 → 验收 未开始 → 结项 未开始"}
	case "downstream_release":
		return map[string]any{"step": 2, "total": 4, "label": "计划 已过 → 阶段放行 待你 → 验收 未开始 → 结项 未开始"}
	case "acceptance_sign":
		return map[string]any{"step": 3, "total": 4, "label": "任务完成 → 验收签署 待你 → 结项 未开始"}
	case "closure_confirm":
		return map[string]any{"step": 4, "total": 4, "label": "任务完成 → 验收签署 已过 → 结项确认 待你"}
	default:
		return nil
	}
}

// humanTaskKindAndLayer maps an internal decision_type to the canonical HumanTask
// kind name and layer (spec §4.2). This is ADDITIVE read-model metadata: the
// internal decision_type values are intentionally left unchanged (renaming them
// touches the coordinator switch, DB values and many tests — a separate change);
// the console instead groups and labels cards by these canonical kind/layer.
//   task | demand | project layers per §4.1.
func humanTaskKindAndLayer(decisionType string) (kind string, layer string) {
	switch strings.TrimSpace(decisionType) {
	case "plan_review":
		return "plan_review", "demand"
	case "project_task_approval":
		return "dispatch_release", "task"
	case "project_task_acceptance":
		return "downstream_release", "task"
	case "demand_acceptance":
		return "acceptance_sign", "demand"
	case "project_acceptance":
		return "closure_confirm", "project"
	case "planning_failed":
		return "planning_failed", "demand"
	case "planning_gap":
		return "planning_gap", "demand"
	case "task_failure_recovery":
		return "task_failure_recovery", "task"
	case "":
		return "", ""
	default:
		// Other project_task_* gates (recovery/runtime_recovery/clarification/…)
		// stay at the task layer with their own decision type as the kind.
		if strings.HasPrefix(decisionType, "project_task_") {
			return decisionType, "task"
		}
		return decisionType, ""
	}
}

type ApprovalActionAdapter struct {
	service *approval.Service
}

func NewApprovalActionAdapter(service *approval.Service) *ApprovalActionAdapter {
	return &ApprovalActionAdapter{service: service}
}

func (a *ApprovalActionAdapter) ResolveApprovalAction(ctx context.Context, req SourceActionRequest) (SourceActionResult, error) {
	if a == nil || a.service == nil {
		return SourceActionResult{}, ErrSourceUnavailable
	}
	_, err := a.service.ResolveRequest(ctx, approval.ResolveRequestInput{
		TenantID:          req.TenantID,
		ApprovalRequestID: req.SourceID,
		DecidedByUserID:   req.ActorUserID,
		Decision:          approval.ApprovalDecision(req.Action),
		Comment:           req.Comment,
		Payload:           req.Payload,
	})
	if err != nil {
		return SourceActionResult{}, normalizeSourceActionError(err)
	}
	return SourceActionResult{SourceType: string(SourceTypeApprovalRequest), SourceID: req.SourceID, Status: req.Action}, nil
}

type ProjectDecisionActionAdapter struct {
	service *project.Service
}

func NewProjectDecisionActionAdapter(service *project.Service) *ProjectDecisionActionAdapter {
	return &ProjectDecisionActionAdapter{service: service}
}

func (a *ProjectDecisionActionAdapter) ResolveProjectDecisionAction(ctx context.Context, req SourceActionRequest) (SourceActionResult, error) {
	if a == nil || a.service == nil || req.SourceProjectID == nil || *req.SourceProjectID == uuid.Nil {
		return SourceActionResult{}, ErrSourceUnavailable
	}
	resolved, err := a.service.ResolveDecision(ctx, project.ResolveDecisionRequest{
		TenantID:          req.TenantID,
		ProjectID:         *req.SourceProjectID,
		DecisionRequestID: req.SourceID,
		DecidedByUserID:   req.ActorUserID,
		Decision:          req.Action,
		Comment:           req.Comment,
		Payload:           req.Payload,
	})
	if err != nil {
		return SourceActionResult{}, normalizeSourceActionError(err)
	}
	if resolved == nil {
		return SourceActionResult{}, ErrSourceUnavailable
	}
	return SourceActionResult{SourceType: string(SourceTypeProjectDecisionRequest), SourceID: resolved.ID, Status: req.Action}, nil
}

func normalizeSourceActionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, approval.ErrInvalidApprovalRequest), errors.Is(err, approval.ErrApprovalAlreadyResolved), errors.Is(err, project.ErrInvalidProject):
		return ErrInvalidAction
	// 项目决策 any-of-N 资格由 project.ResolveDecision 判定;不合格映射为 403 而非 500。
	case errors.Is(err, project.ErrProjectDecisionForbidden):
		return &DecisionForbiddenError{}
	case errors.Is(err, approval.ErrApprovalNotFound), errors.Is(err, project.ErrProjectNotFound), errors.Is(err, pgx.ErrNoRows):
		return ErrSourceUnavailable
	default:
		return err
	}
}

func statusFromApproval(status approval.ApprovalStatus) Status {
	switch status {
	case approval.ApprovalStatusPending:
		return StatusOpen
	case approval.ApprovalStatusCancelled:
		return StatusCancelled
	case approval.ApprovalStatusApproved, approval.ApprovalStatusRejected, approval.ApprovalStatusNeedsMoreEvidence:
		return StatusResolved
	default:
		return StatusOpen
	}
}

func statusFromDecisionSnapshot(status string) Status {
	// Plan-review request_changes resolves the decision request (the replan
	// opens a fresh one), so the inbox item must not stay open.
	if status == project.PlanReviewDecisionRequestChanges {
		return StatusResolved
	}
	// planning_gap restaffed/exempted likewise resolve the decision — the demand
	// is reopened and a fresh planning cycle (with its own review) begins.
	if status == project.PlanningGapDecisionRestaffed || status == project.PlanningGapDecisionExempted {
		return StatusResolved
	}
	// task_failure_recovery explicit actions also close the card.
	if status == "retry" || status == "cancel_downstream" || status == "reassign" {
		return StatusResolved
	}
	switch approval.ApprovalStatus(status) {
	case approval.ApprovalStatusPending:
		return StatusOpen
	case approval.ApprovalStatusCancelled:
		return StatusCancelled
	case approval.ApprovalStatusApproved, approval.ApprovalStatusRejected, approval.ApprovalStatusNeedsMoreEvidence:
		return StatusResolved
	default:
		return StatusOpen
	}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func lastActivityAt(updatedAt, createdAt time.Time) time.Time {
	switch {
	case !updatedAt.IsZero():
		return updatedAt.UTC()
	case !createdAt.IsZero():
		return createdAt.UTC()
	default:
		return time.Now().UTC()
	}
}
