package project

import (
	"context"

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
