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

type ApprovalProjectorAdapter struct {
	service *Service
}

func NewApprovalProjectorAdapter(service *Service) *ApprovalProjectorAdapter {
	return &ApprovalProjectorAdapter{service: service}
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
	contextPayload := decision.InboxContext
	if contextPayload == nil {
		contextPayload = map[string]any{}
	}
	deepLink := map[string]any{
		"route":  "/projects/" + decision.ProjectID.String(),
		"anchor": decision.ID.String(),
	}
	if primaryDemandID, ok := contextPayload["primary_demand_id"].(string); ok && strings.TrimSpace(primaryDemandID) != "" {
		deepLink["route"] = "/workflows/" + strings.TrimSpace(primaryDemandID)
	} else if demandID, ok := contextPayload["demand_id"].(string); ok && strings.TrimSpace(demandID) != "" {
		deepLink["route"] = "/workflows/" + strings.TrimSpace(demandID)
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
