package approval

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
	inbox      InboxProjector
}

func NewService(repository Repository) (*Service, error) {
	return NewServiceWithInboxProjector(repository, nil)
}

func NewServiceWithInboxProjector(repository Repository, inbox InboxProjector) (*Service, error) {
	if repository == nil {
		return nil, errors.New("approval repository is required")
	}
	return &Service{repository: repository, inbox: inbox}, nil
}

func (s *Service) CreateRequest(ctx context.Context, input CreateRequestInput) (*ApprovalRequest, error) {
	input.ResourceType = strings.TrimSpace(input.ResourceType)
	input.RequesterType = strings.TrimSpace(input.RequesterType)
	input.DecisionType = strings.TrimSpace(input.DecisionType)
	input.Title = strings.TrimSpace(input.Title)
	input.Summary = strings.TrimSpace(input.Summary)
	input.RiskLevel = strings.TrimSpace(input.RiskLevel)
	if input.ContextPayload == nil {
		input.ContextPayload = map[string]any{}
	}
	if input.Options == nil {
		input.Options = []any{}
	}
	if input.TenantID == uuid.Nil ||
		input.ResourceID == uuid.Nil ||
		input.TargetUserID == uuid.Nil ||
		input.ResourceType == "" ||
		input.RequesterType == "" ||
		input.DecisionType == "" ||
		input.Title == "" {
		return nil, ErrInvalidApprovalRequest
	}
	request, err := s.repository.CreateApprovalRequest(ctx, input, ApprovalStatusPending)
	if err != nil {
		return nil, err
	}
	if s.inbox != nil {
		if err := s.inbox.UpsertApprovalRequest(ctx, request); err != nil {
			return nil, err
		}
	}
	return &request, nil
}

func (s *Service) GetRequest(ctx context.Context, tenantID, requestID uuid.UUID) (*ApprovalRequest, error) {
	if tenantID == uuid.Nil || requestID == uuid.Nil {
		return nil, ErrInvalidApprovalRequest
	}
	request, err := s.repository.GetApprovalRequest(ctx, tenantID, requestID)
	if err != nil {
		return nil, err
	}
	return &request, nil
}

func (s *Service) ResolveRequest(ctx context.Context, input ResolveRequestInput) (*ApprovalDecisionRecord, error) {
	input.Comment = strings.TrimSpace(input.Comment)
	if input.Payload == nil {
		input.Payload = map[string]any{}
	}
	if input.TenantID == uuid.Nil ||
		input.ApprovalRequestID == uuid.Nil ||
		input.DecidedByUserID == uuid.Nil ||
		!validDecision(input.Decision) {
		return nil, ErrInvalidApprovalRequest
	}
	status := statusFromDecision(input.Decision)
	request, err := s.repository.ResolveApprovalRequest(ctx, input, status)
	if err != nil {
		return nil, err
	}
	decision, err := s.repository.CreateApprovalDecision(ctx, input)
	if err != nil {
		return nil, err
	}
	if s.inbox != nil {
		if err := s.inbox.ResolveApprovalRequest(ctx, request); err != nil {
			return nil, err
		}
	}
	return &decision, nil
}

func validDecision(decision ApprovalDecision) bool {
	switch decision {
	case ApprovalDecisionApproved, ApprovalDecisionRejected, ApprovalDecisionNeedsMoreEvidence:
		return true
	default:
		return false
	}
}

func statusFromDecision(decision ApprovalDecision) ApprovalStatus {
	switch decision {
	case ApprovalDecisionApproved:
		return ApprovalStatusApproved
	case ApprovalDecisionNeedsMoreEvidence:
		return ApprovalStatusNeedsMoreEvidence
	default:
		return ApprovalStatusRejected
	}
}
