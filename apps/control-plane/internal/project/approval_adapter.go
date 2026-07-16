package project

import (
	"context"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/approval"
)

type ApprovalServiceAdapter struct {
	service *approval.Service
}

func NewApprovalServiceAdapter(service *approval.Service) ApprovalResolver {
	if service == nil {
		return nil
	}
	return ApprovalServiceAdapter{service: service}
}

func (a ApprovalServiceAdapter) ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error {
	_, err := a.service.ResolveRequest(ctx, approval.ResolveRequestInput{
		TenantID:          req.TenantID,
		ApprovalRequestID: req.ApprovalRequestID,
		DecidedByUserID:   req.DecidedByUserID,
		Decision:          approval.ApprovalDecision(req.Decision),
		Comment:           req.Comment,
		Payload:           req.Payload,
	})
	return err
}

// GetRequestContextPayload returns the approval request's ContextPayload as
// recorded at creation time (e.g. a planning_gap decision's structured gap under
// "gap") — the source of truth ResolveDecision reads for decision-type vocabulary
// that must come from the original record, not from the resolving caller.
func (a ApprovalServiceAdapter) GetRequestContextPayload(ctx context.Context, tenantID, approvalRequestID uuid.UUID) (map[string]any, error) {
	request, err := a.service.GetRequest(ctx, tenantID, approvalRequestID)
	if err != nil {
		return nil, err
	}
	return request.ContextPayload, nil
}

// CreateRequest opens a new approval request via the approval service and returns
// its ID — used to open the project acceptance review when a criterion sign-off
// makes the last demand terminal (Service.maybeOpenProjectAcceptanceReview).
func (a ApprovalServiceAdapter) CreateRequest(ctx context.Context, req CreateApprovalRequestInput) (uuid.UUID, error) {
	request, err := a.service.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:      req.TenantID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		RequesterType: req.RequesterType,
		TargetUserID:  req.TargetUserID,
		DecisionType:  req.DecisionType,
		Title:         req.Title,
		Summary:       req.Summary,
		RiskLevel:     req.RiskLevel,
		Options:       req.Options,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return request.ID, nil
}
