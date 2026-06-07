package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

type Service struct {
	repository  Repository
	auditReader TeamAuditReader
}

type TeamAuditReader interface {
	ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*audit.Event, error)
}

func NewService(repository Repository, auditReader TeamAuditReader) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	if auditReader == nil {
		return nil, fmt.Errorf("%w: team audit reader is required", ErrInvalidInput)
	}
	return &Service{repository: repository, auditReader: auditReader}, nil
}

func NewServiceWithoutAuditForTest(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository}, nil
}

func (s *Service) CreateTeam(ctx context.Context, req CreateTeamRequest) (*TeamOverview, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.ActorUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: actor_user_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if len(req.HumanOwnerUserIDs) == 0 {
		return nil, fmt.Errorf("%w: human_owner_user_ids is required", ErrInvalidInput)
	}
	for _, ownerID := range req.HumanOwnerUserIDs {
		if ownerID == uuid.Nil {
			return nil, fmt.Errorf("%w: owner_user_id cannot be nil", ErrInvalidInput)
		}
	}
	status := req.Status
	if status == "" {
		status = TeamStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	initialMembers, err := normalizeInitialMembers(req.HumanOwnerUserIDs, req.InitialMembers)
	if err != nil {
		return nil, err
	}
	metadata, err := normalizeTeamMetadata(req.Metadata)
	if err != nil {
		return nil, err
	}

	team, err := s.repository.CreateTeamWithInitialMembers(ctx, CreateTeamWithInitialMembersParams{
		TenantID:       req.TenantID,
		ActorUserID:    req.ActorUserID,
		Slug:           slug,
		Name:           name,
		Status:         status,
		OwnerUserIDs:   req.HumanOwnerUserIDs,
		InitialMembers: initialMembers,
		Metadata:       metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("create team with initial members: %w", err)
	}
	return s.GetOverview(ctx, team.TenantID, team.ID)
}

func normalizeInitialMembers(ownerUserIDs []uuid.UUID, members []InitialTeamMemberInput) ([]InitialTeamMemberInput, error) {
	seen := map[uuid.UUID]struct{}{}
	for _, id := range ownerUserIDs {
		seen[id] = struct{}{}
	}
	normalized := make([]InitialTeamMemberInput, 0, len(members))
	for _, member := range members {
		if member.UserID == uuid.Nil {
			return nil, fmt.Errorf("%w: initial member user_id is required", ErrInvalidInput)
		}
		if member.Role != TeamRoleMember && member.Role != TeamRoleViewer {
			return nil, fmt.Errorf("%w: initial member role must be member or viewer", ErrInvalidInput)
		}
		if _, ok := seen[member.UserID]; ok {
			return nil, fmt.Errorf("%w: duplicate initial member", ErrInvalidInput)
		}
		seen[member.UserID] = struct{}{}
		normalized = append(normalized, member)
	}
	return normalized, nil
}

func (s *Service) ListTeams(ctx context.Context, req ListTeamsRequest) ([]*Team, error) {
	req, err := normalizeListTeamsRequest(req)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeams(ctx, ListTeamsParams{
		TenantID:         req.TenantID,
		Status:           req.Status,
		GovernanceStatus: req.GovernanceStatus,
		Q:                req.Q,
		Offset:           req.Offset,
		Limit:            req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	teams := make([]*Team, 0, len(records))
	for _, record := range records {
		teams = append(teams, teamFromRecord(record))
	}
	return teams, nil
}

func (s *Service) ListTeamSummaries(ctx context.Context, req ListTeamsRequest) ([]*TeamListItem, error) {
	req, err := normalizeListTeamsRequest(req)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeamSummaries(ctx, ListTeamSummariesParams{
		TenantID:         req.TenantID,
		Status:           req.Status,
		GovernanceStatus: req.GovernanceStatus,
		Q:                req.Q,
		Offset:           req.Offset,
		Limit:            req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list team summaries: %w", err)
	}
	items := make([]*TeamListItem, 0, len(records))
	for _, record := range records {
		items = append(items, teamListItemFromRecord(record))
	}
	return items, nil
}

func (s *Service) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*Team, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetTeam(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) UpdateTeam(ctx context.Context, req UpdateTeamRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	humanOwnerUserIDs := req.HumanOwnerUserIDs
	var metadata map[string]any
	if req.Metadata != nil {
		var err error
		metadata, err = normalizeTeamMetadata(req.Metadata)
		if err != nil {
			return nil, err
		}
	}
	if req.HumanOwnerUserIDs == nil || req.Metadata == nil {
		existing, err := s.repository.GetTeam(ctx, req.TenantID, req.TeamID)
		if err != nil {
			return nil, fmt.Errorf("get team: %w", err)
		}
		if req.HumanOwnerUserIDs == nil {
			humanOwnerUserIDs = existing.HumanOwnerUserIDs
		}
		if req.Metadata == nil {
			metadata = cloneMap(existing.Metadata)
		}
	}
	record, err := s.repository.UpdateTeam(ctx, UpdateTeamParams{
		TenantID:         req.TenantID,
		TeamID:           req.TeamID,
		Slug:             slug,
		Name:             name,
		HumanOwnerUserIDs: humanOwnerUserIDs,
		Metadata:         metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("update team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) ChangeTeamStatus(ctx context.Context, req ChangeTeamStatusRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	record, err := s.repository.SetTeamStatus(ctx, SetTeamStatusParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		Status:   req.Status,
	})
	if err != nil {
		return nil, fmt.Errorf("set team status: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) GetOverview(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamOverview, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	summary, err := s.repository.GetTeamSummary(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team summary: %w", err)
	}
	item := teamListItemFromRecord(summary)
	overview := &TeamOverview{
		Team:                 teamFromRecord(summary.Team),
		MemberCount:          item.MemberCount,
		DigitalEmployeeCount: item.DigitalEmployeeCount,
		CapabilityCount:      item.CapabilityCount,
		PendingDraftCount:    item.PendingDraftCount,
		PendingItemCount:     item.PendingDraftCount,
	}
	if revision, err := s.GetCurrentConfigRevision(ctx, tenantID, teamID); err == nil {
		overview.CurrentRevision = revision
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	return overview, nil
}

func (s *Service) CreateConfigRevision(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if len(req.HumanOwnerUserIDs) == 0 {
		return nil, fmt.Errorf("%w: human_owner_user_ids is required", ErrInvalidInput)
	}
	for _, id := range req.HumanOwnerUserIDs {
		if id == uuid.Nil {
			return nil, fmt.Errorf("%w: human_owner_user_id cannot be nil", ErrInvalidInput)
		}
	}
	status := req.Status
	if status == "" {
		status = TeamConfigRevisionStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid config revision status", ErrInvalidInput)
	}

	if _, err := s.repository.GetTeam(ctx, req.TenantID, req.TeamID); err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	if status == TeamConfigRevisionStatusActive {
		if _, err := s.repository.GetCurrentTeamConfigRevision(ctx, req.TenantID, req.TeamID); err == nil {
			return nil, fmt.Errorf("%w: active config revision already exists", ErrInvalidInput)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("get current team config revision: %w", err)
		}
	}

	nextRevision, err := s.repository.GetNextTeamConfigRevisionNumber(ctx, req.TenantID, req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("get next team config revision number: %w", err)
	}
	approvedBy := validUUIDPtr(req.ApprovedBy)
	var approvedAt *time.Time
	if status == TeamConfigRevisionStatusActive {
		now := time.Now().UTC()
		approvedAt = &now
	} else {
		approvedBy = nil
	}
	record, err := s.repository.CreateTeamConfigRevision(ctx, CreateTeamConfigRevisionParams{
		TenantID:                    req.TenantID,
		TeamID:                      req.TeamID,
		RevisionNumber:              nextRevision,
		Constitution:                cloneMap(req.Constitution),
		CapabilityPolicy:            cloneMap(req.CapabilityPolicy),
		ContextPolicy:               cloneMap(req.ContextPolicy),
		ApprovalPolicy:              cloneMap(req.ApprovalPolicy),
		ArtifactContract:            cloneMap(req.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(req.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(req.RuntimeScopePolicy),
		HumanOwnerUserIDs:           req.HumanOwnerUserIDs,
		Status:                      status,
		ApprovedBy:                  approvedBy,
		ApprovedAt:                  approvedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) GetConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (*TeamConfigRevision, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if revisionID == uuid.Nil {
		return nil, fmt.Errorf("%w: config_revision_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetTeamConfigRevision(ctx, tenantID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("get team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) GetCurrentConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamConfigRevision, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetCurrentTeamConfigRevision(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) ListGovernanceDrafts(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamConfigRevision, error) {
	params, err := normalizeListTeamConfigDraftsRequest(tenantID, teamID, limit, offset)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeamConfigDrafts(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("list team config drafts: %w", err)
	}
	drafts := make([]*TeamConfigRevision, 0, len(records))
	for _, record := range records {
		drafts = append(drafts, configRevisionFromRecord(record))
	}
	return drafts, nil
}

func (s *Service) CreateGovernanceDraft(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error) {
	req.Status = TeamConfigRevisionStatusDraft
	req.ApprovedBy = nil
	return s.CreateConfigRevision(ctx, req)
}

func (s *Service) UpdateGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID, input GovernanceDraftInput) (*TeamConfigRevision, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	if issues := validateCapabilityBindingArrays(input.CapabilityPolicy); len(issues) > 0 {
		return nil, fmt.Errorf("%w: capability policy has invalid binding arrays", ErrInvalidInput)
	}
	record, err := s.repository.UpdateTeamConfigRevisionDraft(ctx, UpdateTeamConfigRevisionDraftParams{
		TenantID:                    tenantID,
		TeamID:                      teamID,
		RevisionID:                  draftID,
		Constitution:                cloneOptionalMap(input.Constitution),
		CapabilityPolicy:            cloneOptionalMap(input.CapabilityPolicy),
		ContextPolicy:               cloneOptionalMap(input.ContextPolicy),
		ApprovalPolicy:              cloneOptionalMap(input.ApprovalPolicy),
		ArtifactContract:            cloneOptionalMap(input.ArtifactContract),
		InternalCollaborationPolicy: cloneOptionalMap(input.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneOptionalMap(input.RuntimeScopePolicy),
		HumanOwnerUserIDs:           input.HumanOwnerUserIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("update governance draft: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) ApproveGovernanceDraft(ctx context.Context, tenantID, teamID, draftID, approvedBy uuid.UUID) (*TeamConfigRevision, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	if approvedBy == uuid.Nil {
		return nil, fmt.Errorf("%w: approved_by is required", ErrInvalidInput)
	}
	team, err := s.repository.GetTeam(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	if team.Status == TeamStatusDisabled || team.Status == TeamStatusArchived {
		return nil, fmt.Errorf("%w: disabled or archived team cannot approve governance drafts", ErrInvalidInput)
	}
	draft, err := s.repository.GetTeamConfigRevision(ctx, tenantID, draftID)
	if err != nil {
		return nil, fmt.Errorf("get governance draft: %w", err)
	}
	if draft.TeamID != teamID {
		return nil, fmt.Errorf("%w: governance draft does not belong to team", ErrInvalidInput)
	}
	if draft.Status != TeamConfigRevisionStatusDraft {
		return nil, fmt.Errorf("%w: governance revision must be draft", ErrInvalidInput)
	}
	_, blockingErrors := validateGovernancePolicies(draft.Constitution, draft.CapabilityPolicy, true)
	if len(blockingErrors) > 0 {
		return nil, fmt.Errorf("%w: governance draft has blocking validation errors", ErrInvalidInput)
	}
	record, err := s.repository.ApproveTeamConfigRevision(ctx, ActivateTeamConfigRevisionParams{
		TenantID:   tenantID,
		TeamID:     teamID,
		RevisionID: draftID,
		ApprovedBy: approvedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("approve governance draft: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) RejectGovernanceDraft(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*TeamConfigRevision, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	record, err := s.repository.RejectTeamConfigRevision(ctx, tenantID, teamID, draftID)
	if err != nil {
		return nil, fmt.Errorf("reject governance draft: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) PreviewGovernanceDiff(ctx context.Context, tenantID, teamID, draftID uuid.UUID) (*GovernanceDiffSummary, error) {
	if err := validateGovernanceRevisionIDs(tenantID, teamID, draftID); err != nil {
		return nil, err
	}
	draft, err := s.repository.GetTeamConfigRevision(ctx, tenantID, draftID)
	if err != nil {
		return nil, fmt.Errorf("get governance draft: %w", err)
	}
	if draft.TeamID != teamID {
		return nil, fmt.Errorf("%w: governance draft does not belong to team", ErrInvalidInput)
	}
	if draft.Status != TeamConfigRevisionStatusDraft {
		return nil, fmt.Errorf("%w: governance revision must be draft", ErrInvalidInput)
	}
	active, err := s.repository.GetCurrentTeamConfigRevision(ctx, tenantID, teamID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	warnings, blockingErrors := validateGovernancePolicies(draft.Constitution, draft.CapabilityPolicy, true)
	summary := &GovernanceDiffSummary{
		AddedHardRules: countAddedHardRules(active.Constitution, draft.Constitution),
		Warnings:       warnings,
		BlockingErrors: blockingErrors,
	}
	if jsonMapChanged(active.CapabilityPolicy, draft.CapabilityPolicy) {
		summary.ChangedCapabilities = 1
	}
	if jsonMapChanged(active.ApprovalPolicy, draft.ApprovalPolicy) {
		summary.ChangedApprovalRules = 1
	}
	return summary, nil
}

func (s *Service) ListTeamMembers(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*TeamMember, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	limit, offset, err := normalizeLimitOffset(limit, offset)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeamMembers(ctx, ListTeamMembersParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list team members: %w", err)
	}
	members := make([]*TeamMember, 0, len(records))
	for _, record := range records {
		members = append(members, teamMemberFromRecord(record))
	}
	return members, nil
}

func (s *Service) AddTeamMember(ctx context.Context, req AddTeamMemberRequest) (*TeamMember, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	role, err := normalizeTeamRole(req.Role, TeamRoleMember)
	if err != nil {
		return nil, err
	}
	if !isDirectTeamRole(role) {
		return nil, fmt.Errorf("%w: privileged role requires approval", ErrInvalidInput)
	}
	record, err := s.repository.AddTeamMember(ctx, AddTeamMemberParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		UserID:   req.UserID,
		Role:     role,
	})
	if err != nil {
		return nil, fmt.Errorf("add team member: %w", err)
	}
	return teamMemberFromRecord(record), nil
}

func (s *Service) RemoveTeamMember(ctx context.Context, req RemoveTeamMemberRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.MembershipID == uuid.Nil {
		return fmt.Errorf("%w: membership_id is required", ErrInvalidInput)
	}
	member, err := s.repository.GetTeamMember(ctx, req.TenantID, req.TeamID, req.MembershipID)
	if err != nil {
		return fmt.Errorf("get team member: %w", err)
	}
	if member.Role == TeamRoleOwner {
		ownerCount, err := s.repository.CountTeamOwners(ctx, req.TenantID, req.TeamID)
		if err != nil {
			return fmt.Errorf("count team owners: %w", err)
		}
		if ownerCount <= 1 {
			return fmt.Errorf("%w: cannot remove the final team owner", ErrInvalidInput)
		}
	}
	if _, err := s.repository.DisableTeamMemberRole(ctx, DisableTeamMemberRoleParams{
		TenantID:     req.TenantID,
		TeamID:       req.TeamID,
		MembershipID: req.MembershipID,
	}); err != nil {
		return fmt.Errorf("disable team member role: %w", err)
	}
	return nil
}

func (s *Service) CreateRoleRequest(ctx context.Context, req CreateRoleRequestRequest) (*TeamMemberRoleRequest, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.TargetUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: target_user_id is required", ErrInvalidInput)
	}
	if req.RequestedBy == uuid.Nil {
		return nil, fmt.Errorf("%w: requested_by is required", ErrInvalidInput)
	}
	role, err := normalizeTeamRole(req.RequestedRole, "")
	if err != nil {
		return nil, err
	}
	if !isPrivilegedTeamRole(role) {
		return nil, fmt.Errorf("%w: role request must target a privileged role", ErrInvalidInput)
	}
	record, err := s.repository.CreateTeamMemberRoleRequest(ctx, CreateTeamMemberRoleRequestParams{
		TenantID:      req.TenantID,
		TeamID:        req.TeamID,
		TargetUserID:  req.TargetUserID,
		RequestedRole: role,
		RequestedBy:   req.RequestedBy,
		Reason:        strings.TrimSpace(req.Reason),
	})
	if err != nil {
		return nil, fmt.Errorf("create team member role request: %w", err)
	}
	return roleRequestFromRecord(record), nil
}

func (s *Service) ListRoleRequests(ctx context.Context, tenantID, teamID uuid.UUID, status TeamMemberRoleRequestStatus, limit, offset int32) ([]*TeamMemberRoleRequest, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if status != "" && !isValidRoleRequestStatus(status) {
		return nil, fmt.Errorf("%w: invalid role request status", ErrInvalidInput)
	}
	limit, offset, err := normalizeLimitOffset(limit, offset)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeamMemberRoleRequests(ctx, ListTeamMemberRoleRequestsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Status:   status,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list team member role requests: %w", err)
	}
	requests := make([]*TeamMemberRoleRequest, 0, len(records))
	for _, record := range records {
		requests = append(requests, roleRequestFromRecord(record))
	}
	return requests, nil
}

func (s *Service) ApproveRoleRequest(ctx context.Context, req DecideRoleRequestRequest) (*TeamMemberRoleRequest, error) {
	return s.decideRoleRequest(ctx, req, TeamMemberRoleRequestStatusApproved)
}

func (s *Service) RejectRoleRequest(ctx context.Context, req DecideRoleRequestRequest) (*TeamMemberRoleRequest, error) {
	return s.decideRoleRequest(ctx, req, TeamMemberRoleRequestStatusRejected)
}

func (s *Service) decideRoleRequest(ctx context.Context, req DecideRoleRequestRequest, status TeamMemberRoleRequestStatus) (*TeamMemberRoleRequest, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.RequestID == uuid.Nil {
		return nil, fmt.Errorf("%w: request_id is required", ErrInvalidInput)
	}
	if req.DecidedBy == uuid.Nil {
		return nil, fmt.Errorf("%w: decided_by is required", ErrInvalidInput)
	}
	if status == TeamMemberRoleRequestStatusApproved {
		record, err := s.repository.ApproveTeamMemberRoleRequest(ctx, DecideTeamMemberRoleRequestParams{
			TenantID:       req.TenantID,
			TeamID:         req.TeamID,
			RequestID:      req.RequestID,
			Status:         TeamMemberRoleRequestStatusApproved,
			DecidedBy:      req.DecidedBy,
			DecisionReason: strings.TrimSpace(req.DecisionReason),
		})
		if err != nil {
			return nil, fmt.Errorf("approve team member role request: %w", err)
		}
		return roleRequestFromRecord(record), nil
	}
	record, err := s.repository.DecideTeamMemberRoleRequest(ctx, DecideTeamMemberRoleRequestParams{
		TenantID:       req.TenantID,
		TeamID:         req.TeamID,
		RequestID:      req.RequestID,
		Status:         status,
		DecidedBy:      req.DecidedBy,
		DecisionReason: strings.TrimSpace(req.DecisionReason),
	})
	if err != nil {
		return nil, fmt.Errorf("decide team member role request: %w", err)
	}
	return roleRequestFromRecord(record), nil
}

func (s *Service) ListTeamAuditEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]*audit.Event, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	limit, offset, err := normalizeLimitOffset(limit, offset)
	if err != nil {
		return nil, err
	}
	if s.auditReader == nil {
		return nil, errors.New("team audit reader is not configured")
	}
	return s.auditReader.ListTeamEvents(ctx, tenantID, teamID, int(limit), int(offset))
}

func normalizeListTeamsRequest(req ListTeamsRequest) (ListTeamsRequest, error) {
	if req.TenantID == uuid.Nil {
		return req, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.Status != "" && !req.Status.IsValid() {
		return req, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	if req.GovernanceStatus != "" && !req.GovernanceStatus.IsValid() {
		return req, fmt.Errorf("%w: invalid governance status", ErrInvalidInput)
	}
	if req.Offset < 0 {
		return req, fmt.Errorf("%w: offset must be non-negative", ErrInvalidInput)
	}
	if req.Limit < 0 {
		return req, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	req.Q = strings.TrimSpace(req.Q)
	return req, nil
}

func normalizeLimitOffset(limit, offset int32) (int32, int32, error) {
	if offset < 0 {
		return 0, 0, fmt.Errorf("%w: offset must be non-negative", ErrInvalidInput)
	}
	if limit < 0 {
		return 0, 0, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	return limit, offset, nil
}

func normalizeListTeamConfigDraftsRequest(tenantID, teamID uuid.UUID, limit, offset int32) (ListTeamConfigDraftsParams, error) {
	if tenantID == uuid.Nil {
		return ListTeamConfigDraftsParams{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return ListTeamConfigDraftsParams{}, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	limit, offset, err := normalizeLimitOffset(limit, offset)
	if err != nil {
		return ListTeamConfigDraftsParams{}, err
	}
	return ListTeamConfigDraftsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Limit:    limit,
		Offset:   offset,
	}, nil
}

func validateGovernanceRevisionIDs(tenantID, teamID, revisionID uuid.UUID) error {
	if tenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if revisionID == uuid.Nil {
		return fmt.Errorf("%w: governance_revision_id is required", ErrInvalidInput)
	}
	return nil
}

func validateGovernancePolicies(constitution, capabilityPolicy map[string]any, requireHardRules bool) ([]ValidationIssue, []ValidationIssue) {
	warnings := []ValidationIssue{}
	blockingErrors := []ValidationIssue{}
	if requireHardRules {
		rules, ok, hardRuleIssues := hardRulesFromConstitution(constitution)
		blockingErrors = append(blockingErrors, hardRuleIssues...)
		if len(hardRuleIssues) == 0 && (!ok || len(rules) == 0) {
			blockingErrors = append(blockingErrors, ValidationIssue{
				Field:    "constitution.hard_rules",
				Message:  "hard_rules must be an array with at least one non-empty string",
				Severity: "error",
			})
		}
	}
	blockingErrors = append(blockingErrors, validateCapabilityBindingArrays(capabilityPolicy)...)
	return warnings, blockingErrors
}

func validateCapabilityBindingArrays(capabilityPolicy map[string]any) []ValidationIssue {
	keys := []string{
		"skill_bindings",
		"mcp_bindings",
		"knowledge_base_bindings",
		"external_capability_bindings",
		"allowed_skills",
		"allowed_mcp_servers",
		"allowed_plugins",
		"allowed_provider_types",
	}
	issues := []ValidationIssue{}
	for _, key := range keys {
		value, ok := capabilityPolicy[key]
		if !ok {
			continue
		}
		path := fmt.Sprintf("capability_policy.%s", key)
		switch typed := value.(type) {
		case []string:
			continue
		case []any:
			for index, item := range typed {
				if _, ok := item.(string); !ok {
					issues = append(issues, invalidGovernanceIssue(path, fmt.Sprintf("binding item %d must be a string", index)))
				}
			}
		default:
			issues = append(issues, invalidGovernanceIssue(path, "binding value must be an array of strings"))
		}
	}
	return issues
}

func invalidGovernanceIssue(field, message string) ValidationIssue {
	return ValidationIssue{
		Field:    field,
		Message:  message,
		Severity: "error",
	}
}

func countAddedHardRules(activeConstitution, draftConstitution map[string]any) int32 {
	activeRules, _, _ := hardRulesFromConstitution(activeConstitution)
	draftRules, _, _ := hardRulesFromConstitution(draftConstitution)
	activeSet := map[string]bool{}
	for _, rule := range activeRules {
		activeSet[rule] = true
	}
	seenDraft := map[string]bool{}
	var added int32
	for _, rule := range draftRules {
		if activeSet[rule] || seenDraft[rule] {
			continue
		}
		seenDraft[rule] = true
		added++
	}
	return added
}

func hardRulesFromConstitution(constitution map[string]any) ([]string, bool, []ValidationIssue) {
	value, ok := constitution["hard_rules"]
	if !ok {
		return nil, false, nil
	}
	switch typed := value.(type) {
	case []string:
		return normalizedStringList(typed), true, nil
	case []any:
		rules := make([]string, 0, len(typed))
		issues := []ValidationIssue{}
		for index, item := range typed {
			text, ok := item.(string)
			if !ok {
				issues = append(issues, invalidGovernanceIssue("constitution.hard_rules", fmt.Sprintf("hard_rules item %d must be a string", index)))
				continue
			}
			trimmed := strings.TrimSpace(text)
			if trimmed != "" {
				rules = append(rules, trimmed)
			}
		}
		return rules, true, issues
	default:
		return nil, false, []ValidationIssue{invalidGovernanceIssue("constitution.hard_rules", "hard_rules must be an array of strings")}
	}
}

func normalizedStringList(values []string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func jsonMapChanged(left, right map[string]any) bool {
	leftJSON, leftErr := json.Marshal(cloneMap(left))
	rightJSON, rightErr := json.Marshal(cloneMap(right))
	if leftErr != nil || rightErr != nil {
		return true
	}
	return string(leftJSON) != string(rightJSON)
}

func normalizeTeamRole(role string, defaultRole string) (string, error) {
	role = strings.TrimSpace(role)
	if role == "" {
		role = defaultRole
	}
	if !isKnownTeamRole(role) {
		return "", fmt.Errorf("%w: invalid team role", ErrInvalidInput)
	}
	return role, nil
}

func isKnownTeamRole(role string) bool {
	return isDirectTeamRole(role) || isPrivilegedTeamRole(role)
}

func isDirectTeamRole(role string) bool {
	return role == TeamRoleMember || role == TeamRoleViewer
}

func isPrivilegedTeamRole(role string) bool {
	switch role {
	case TeamRoleOwner, TeamRoleAdmin, TeamRoleApprover:
		return true
	default:
		return false
	}
}

func isValidRoleRequestStatus(status TeamMemberRoleRequestStatus) bool {
	switch status {
	case TeamMemberRoleRequestStatusPending, TeamMemberRoleRequestStatusApproved, TeamMemberRoleRequestStatusRejected:
		return true
	default:
		return false
	}
}

func teamFromRecord(record TeamRecord) *Team {
	return &Team{
		ID:                record.ID,
		TenantID:          record.TenantID,
		Slug:              record.Slug,
		Name:              record.Name,
		Status:            record.Status,
		HumanOwnerUserIDs: record.HumanOwnerUserIDs,
		HumanOwners:       record.HumanOwners,
		Metadata:          cloneMap(record.Metadata),
		CreatedAt:         record.CreatedAt,
		UpdatedAt:         record.UpdatedAt,
	}
}

func teamMemberFromRecord(record TeamMemberRecord) *TeamMember {
	return &TeamMember{
		MembershipID:     record.MembershipID,
		TenantID:         record.TenantID,
		TeamID:           record.TeamID,
		UserID:           record.UserID,
		Username:         record.Username,
		DisplayName:      record.DisplayName,
		Email:            record.Email,
		AccountStatus:    record.AccountStatus,
		Avatar:           cloneUserAvatarConfig(record.Avatar),
		Role:             record.Role,
		MembershipStatus: record.MembershipStatus,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func roleRequestFromRecord(record TeamMemberRoleRequestRecord) *TeamMemberRoleRequest {
	return &TeamMemberRoleRequest{
		ID:             record.ID,
		TenantID:       record.TenantID,
		TeamID:         record.TeamID,
		TargetUserID:   record.TargetUserID,
		RequestedRole:  record.RequestedRole,
		RequestedBy:    record.RequestedBy,
		Status:         record.Status,
		Reason:         record.Reason,
		DecidedBy:      validUUIDPtr(record.DecidedBy),
		DecidedAt:      cloneTimePtr(record.DecidedAt),
		DecisionReason: record.DecisionReason,
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
}

func teamListItemFromRecord(record TeamListItemRecord) *TeamListItem {
	team := teamFromRecord(record.Team)
	return &TeamListItem{
		Team:                 *team,
		MemberCount:          record.MemberCount,
		DigitalEmployeeCount: record.DigitalEmployeeCount,
		CapabilityCount:      record.CapabilityCount,
		GovernanceStatus:     record.GovernanceStatus,
		CurrentRevision:      cloneInt32Ptr(record.CurrentRevision),
		PendingDraftCount:    record.PendingDraftCount,
		RiskSummary:          record.RiskSummary,
	}
}

func configRevisionFromRecord(record TeamConfigRevisionRecord) *TeamConfigRevision {
	return &TeamConfigRevision{
		ID:                          record.ID,
		TenantID:                    record.TenantID,
		TeamID:                      record.TeamID,
		RevisionNumber:              record.RevisionNumber,
		Constitution:                cloneMap(record.Constitution),
		CapabilityPolicy:            cloneMap(record.CapabilityPolicy),
		ContextPolicy:               cloneMap(record.ContextPolicy),
		ApprovalPolicy:              cloneMap(record.ApprovalPolicy),
		ArtifactContract:            cloneMap(record.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(record.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(record.RuntimeScopePolicy),
		HumanOwnerUserIDs:           record.HumanOwnerUserIDs,
		Status:                      record.Status,
		ApprovedBy:                  validUUIDPtr(record.ApprovedBy),
		ApprovedAt:                  cloneTimePtr(record.ApprovedAt),
		CreatedAt:                   record.CreatedAt,
		UpdatedAt:                   record.UpdatedAt,
	}
}

func cloneInt32Ptr(value *int32) *int32 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}

func validUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func normalizeTeamMetadata(metadata map[string]any) (map[string]any, error) {
	cloned := cloneMap(metadata)
	displayValue, ok := cloned["display"]
	if !ok || displayValue == nil {
		return cloned, nil
	}
	display, ok := displayValue.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: metadata.display must be object", ErrInvalidInput)
	}
	display = cloneMap(display)
	cloned["display"] = display
	for _, key := range []string{"icon_key", "color_tone"} {
		value, ok := display[key]
		if !ok || value == nil {
			continue
		}
		text, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%w: metadata.display.%s must be string", ErrInvalidInput, key)
		}
		if len(strings.TrimSpace(text)) > 40 {
			return nil, fmt.Errorf("%w: metadata.display.%s is too long", ErrInvalidInput, key)
		}
		display[key] = strings.TrimSpace(text)
	}
	return cloned, nil
}

func cloneUserAvatarConfig(avatar *UserAvatarConfig) *UserAvatarConfig {
	if avatar == nil {
		return nil
	}
	return &UserAvatarConfig{
		Provider: avatar.Provider,
		Style:    avatar.Style,
		Seed:     avatar.Seed,
		Options:  cloneMap(avatar.Options),
	}
}

func cloneOptionalMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	return cloneMap(value)
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
