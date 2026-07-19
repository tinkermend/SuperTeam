package permission

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/tenant"
)

// ResourceTypeTeamPrivilegedRole is the S2 subject resource_type: a team member
// requesting a privileged team role (approver/admin/owner). The standalone
// role-request feature (migration 004 / tenant_team_member_role_requests) was
// retired in migration 085; this re-homes the capability onto the unified
// ApprovalRequest model with no separate table.
const ResourceTypeTeamPrivilegedRole = "team_privileged_role_request"

// privilegedRoles are the roles that must go through permission-center approval;
// member/viewer are granted directly by the existing team member API.
var privilegedRoles = map[string]bool{
	tenant.TeamRoleApprover: true,
	tenant.TeamRoleAdmin:    true,
	tenant.TeamRoleOwner:    true,
}

// TeamRoleGranter is the tenant surface the S2 apply uses to grant a privileged
// role AFTER approval (bypassing the direct-assignment rejection). tenant.Service
// implements it.
type TeamRoleGranter interface {
	GrantTeamMemberRole(ctx context.Context, in tenant.GrantTeamRoleInput) error
}

// PrivilegedRoleRequestInput is the S2 producing-side input.
type PrivilegedRoleRequestInput struct {
	TenantID      uuid.UUID
	TeamID        uuid.UUID
	RequestedBy   uuid.UUID
	TargetUserID  uuid.UUID
	RequestedRole string
	Reason        string
}

// PrivilegedRoleProducer creates a category=permission approval request for a
// privileged-role grant, routed to a team approver.
type PrivilegedRoleProducer struct {
	approvals *approval.Service
	router    *ApproverRouter
	registry  *Registry
	names     NameResolver
}

func NewPrivilegedRoleProducer(approvals *approval.Service, router *ApproverRouter, registry *Registry, names NameResolver) *PrivilegedRoleProducer {
	return &PrivilegedRoleProducer{approvals: approvals, router: router, registry: registry, names: names}
}

func (p *PrivilegedRoleProducer) Request(ctx context.Context, in PrivilegedRoleRequestInput) (*View, error) {
	role := strings.TrimSpace(in.RequestedRole)
	if in.TenantID == uuid.Nil || in.TeamID == uuid.Nil || in.RequestedBy == uuid.Nil || in.TargetUserID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	if !privilegedRoles[role] {
		return nil, fmt.Errorf("%w: requested_role must be one of approver/admin/owner", ErrInvalidRequest)
	}
	approver, err := p.router.ResolveTeamApprover(ctx, in.TenantID, in.TeamID, in.RequestedBy)
	if err != nil {
		return nil, err
	}
	requester := in.RequestedBy
	payload := map[string]any{
		"team_id":        in.TeamID.String(),
		"target_user_id": in.TargetUserID.String(),
		"requested_role": role,
		"requested_by":   in.RequestedBy.String(),
	}
	if reason := strings.TrimSpace(in.Reason); reason != "" {
		payload["reason"] = reason
	}
	title := fmt.Sprintf("特权角色申请:授予 %s 角色", tenant.RoleDisplayName(role))
	created, err := p.approvals.CreateRequest(ctx, approval.CreateRequestInput{
		TenantID:       in.TenantID,
		ResourceType:   ResourceTypeTeamPrivilegedRole,
		ResourceID:     in.TargetUserID,
		RequesterType:  "human_user",
		RequesterID:    &requester,
		TargetUserID:   approver,
		DecisionType:   "team_privileged_role_grant",
		Title:          title,
		RiskLevel:      "high",
		Category:       approval.ApprovalCategoryPermission,
		ContextPayload: payload,
	})
	if err != nil {
		return nil, err
	}
	view := View{
		Request: *created,
		Actions: p.registry.ActionsFor(ResourceTypeTeamPrivilegedRole),
	}
	if p.names != nil && created.RequesterID != nil {
		view.RequesterName = p.names.DisplayName(ctx, created.TenantID, *created.RequesterID)
	}
	return &view, nil
}

// teamRoleSubject is the S2 subject: on approval it grants the requested
// privileged role to the target member.
type teamRoleSubject struct {
	granter TeamRoleGranter
}

// NewTeamRoleSubject builds the S2 subject.
func NewTeamRoleSubject(granter TeamRoleGranter) Subject {
	return &teamRoleSubject{granter: granter}
}

func (s *teamRoleSubject) ResourceType() string { return ResourceTypeTeamPrivilegedRole }
func (s *teamRoleSubject) Actions() []Action    { return DefaultActions() }

func (s *teamRoleSubject) Apply(ctx context.Context, in ApplyInput) error {
	payload := in.Request.ContextPayload
	teamID, err := uuidFromPayload(payload, "team_id")
	if err != nil {
		return err
	}
	targetUserID, err := uuidFromPayload(payload, "target_user_id")
	if err != nil {
		return err
	}
	role, _ := payload["requested_role"].(string)
	if !privilegedRoles[role] {
		return fmt.Errorf("permission: invalid requested_role %q in approval context", role)
	}
	return s.granter.GrantTeamMemberRole(ctx, tenant.GrantTeamRoleInput{
		TenantID:     in.Request.TenantID,
		TeamID:       teamID,
		TargetUserID: targetUserID,
		Role:         role,
		GrantedBy:    in.DecidedBy,
	})
}

func uuidFromPayload(payload map[string]any, key string) (uuid.UUID, error) {
	raw, ok := payload[key].(string)
	if !ok || raw == "" {
		return uuid.Nil, fmt.Errorf("permission: missing %q in approval context", key)
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fmt.Errorf("permission: invalid %q in approval context: %w", key, err)
	}
	return id, nil
}
