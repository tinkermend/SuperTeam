package permission

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/approval"
)

var (
	ErrInvalidRequest  = errors.New("permission: invalid request")
	ErrNotFound        = errors.New("permission: approval not found")
	ErrInvalidDecision = errors.New("permission: invalid decision")
	ErrAlreadyResolved = errors.New("permission: approval already resolved")
)

// NameResolver enriches a permission approval with the requester's display name
// (中文优先「名称 (id)」惯例的服务端补名). Optional — a nil resolver leaves names blank
// and the frontend falls back to the id.
type NameResolver interface {
	DisplayName(ctx context.Context, tenantID, userID uuid.UUID) string
}

// View is the domain projection of a permission approval the handler maps to the
// gen.PermissionApproval contract type.
type View struct {
	Request       approval.ApprovalRequest
	RequesterName string
	Actions       []Action
}

// Service owns the permission-center read path and decision lifecycle. It reads
// the approval domain directly (never the inbox) and, on approval, triggers the
// registered subject's Apply side-effect (§4.4 apply seam).
type Service struct {
	approvals *approval.Service
	registry  *Registry
	names     NameResolver
}

func NewService(approvals *approval.Service, registry *Registry, names NameResolver) (*Service, error) {
	if approvals == nil {
		return nil, errors.New("permission: approval service is required")
	}
	if registry == nil {
		return nil, errors.New("permission: registry is required")
	}
	return &Service{approvals: approvals, registry: registry, names: names}, nil
}

// Registry exposes the subject registry so app wiring can register subjects.
func (s *Service) Registry() *Registry { return s.registry }

type ListInput struct {
	TenantID     uuid.UUID
	ActorUserID  uuid.UUID
	View         string // "mine" (default) | "team"
	Status       string
	RiskLevel    string
	ResourceType string
	Limit        int32
	Offset       int32
}

// List reads the permission-center queue plus the view-scoped metric summary.
func (s *Service) List(ctx context.Context, in ListInput) ([]View, approval.PermissionApprovalSummary, bool, error) {
	if in.TenantID == uuid.Nil {
		return nil, approval.PermissionApprovalSummary{}, false, ErrInvalidRequest
	}
	limit := in.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var target *uuid.UUID
	if in.View != "team" && in.ActorUserID != uuid.Nil {
		// view=mine (default): only requests routed to the actor.
		actor := in.ActorUserID
		target = &actor
	}
	repoInput := approval.ListPermissionApprovalsInput{
		TenantID:     in.TenantID,
		Status:       optStr(in.Status),
		RiskLevel:    optStr(in.RiskLevel),
		ResourceType: optStr(in.ResourceType),
		TargetUserID: target,
		Limit:        limit + 1, // fetch one extra to compute has_more
		Offset:       in.Offset,
	}
	requests, err := s.approvals.ListPermissionApprovals(ctx, repoInput)
	if err != nil {
		return nil, approval.PermissionApprovalSummary{}, false, err
	}
	hasMore := false
	if int32(len(requests)) > limit {
		hasMore = true
		requests = requests[:limit]
	}
	views := make([]View, 0, len(requests))
	for _, req := range requests {
		views = append(views, s.toView(ctx, req))
	}
	summary, err := s.approvals.PermissionApprovalSummary(ctx, approval.PermissionApprovalSummaryInput{
		TenantID:     in.TenantID,
		TargetUserID: target,
	})
	if err != nil {
		return nil, approval.PermissionApprovalSummary{}, false, err
	}
	return views, summary, hasMore, nil
}

type DecideInput struct {
	TenantID     uuid.UUID
	ApprovalID   uuid.UUID
	DecidedBy    uuid.UUID
	Decision     string
	Note         string
	EvidenceRefs []string
}

// Decide resolves a permission approval and, for approved decisions, triggers the
// subject's idempotent Apply BEFORE marking the request resolved — so an approval
// is only recorded once its side-effect has succeeded.
func (s *Service) Decide(ctx context.Context, in DecideInput) (*View, error) {
	if in.TenantID == uuid.Nil || in.ApprovalID == uuid.Nil || in.DecidedBy == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	decision := approval.ApprovalDecision(strings.TrimSpace(in.Decision))
	if !isPermissionDecision(decision) {
		return nil, ErrInvalidDecision
	}
	request, err := s.approvals.GetRequest(ctx, in.TenantID, in.ApprovalID)
	if err != nil {
		if errors.Is(err, approval.ErrApprovalNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	// Domain guard: the permission endpoint only touches category=permission rows.
	if request.Category != approval.ApprovalCategoryPermission {
		return nil, ErrNotFound
	}
	if request.Status != approval.ApprovalStatusPending {
		return nil, ErrAlreadyResolved
	}

	if decision == approval.ApprovalDecisionApproved {
		if err := s.registry.Apply(ctx, ApplyInput{
			Request:   *request,
			Decision:  decision,
			DecidedBy: in.DecidedBy,
			Comment:   in.Note,
		}); err != nil {
			return nil, err
		}
	}

	payload := map[string]any{}
	if len(in.EvidenceRefs) > 0 {
		payload["evidence_refs"] = in.EvidenceRefs
	}
	if _, err := s.approvals.ResolveRequest(ctx, approval.ResolveRequestInput{
		TenantID:          in.TenantID,
		ApprovalRequestID: in.ApprovalID,
		DecidedByUserID:   in.DecidedBy,
		Decision:          decision,
		Comment:           in.Note,
		Payload:           payload,
	}); err != nil {
		if errors.Is(err, approval.ErrApprovalAlreadyResolved) {
			return nil, ErrAlreadyResolved
		}
		if errors.Is(err, approval.ErrApprovalNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	updated, err := s.approvals.GetRequest(ctx, in.TenantID, in.ApprovalID)
	if err != nil {
		return nil, err
	}
	view := s.toView(ctx, *updated)
	return &view, nil
}

func (s *Service) toView(ctx context.Context, req approval.ApprovalRequest) View {
	name := ""
	if s.names != nil && req.RequesterID != nil && *req.RequesterID != uuid.Nil {
		name = s.names.DisplayName(ctx, req.TenantID, *req.RequesterID)
	}
	return View{
		Request:       req,
		RequesterName: name,
		Actions:       s.registry.ActionsFor(req.ResourceType),
	}
}

func isPermissionDecision(d approval.ApprovalDecision) bool {
	switch d {
	case approval.ApprovalDecisionApproved, approval.ApprovalDecisionRejected, approval.ApprovalDecisionNeedsMoreEvidence:
		return true
	default:
		return false
	}
}

func optStr(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
