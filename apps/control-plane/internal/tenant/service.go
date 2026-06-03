package tenant

import (
	"context"
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

func (s *Service) CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = TeamStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}

	record, err := s.repository.CreateTeam(ctx, CreateTeamParams{
		TenantID:         req.TenantID,
		Slug:             slug,
		Name:             name,
		Status:           status,
		HumanOwnerUserID: validUUIDPtr(req.HumanOwnerUserID),
		Metadata:         cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) ListTeams(ctx context.Context, req ListTeamsRequest) ([]*Team, error) {
	req, err := normalizeListTeamsRequest(req)
	if err != nil {
		return nil, err
	}
	records, err := s.repository.ListTeams(ctx, ListTeamsParams{
		TenantID: req.TenantID,
		Status:   req.Status,
		Q:        req.Q,
		Offset:   req.Offset,
		Limit:    req.Limit,
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
		TenantID: req.TenantID,
		Status:   req.Status,
		Q:        req.Q,
		Offset:   req.Offset,
		Limit:    req.Limit,
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
	humanOwnerUserID := validUUIDPtr(req.HumanOwnerUserID)
	metadata := cloneMap(req.Metadata)
	if req.HumanOwnerUserID == nil || req.Metadata == nil {
		existing, err := s.repository.GetTeam(ctx, req.TenantID, req.TeamID)
		if err != nil {
			return nil, fmt.Errorf("get team: %w", err)
		}
		if req.HumanOwnerUserID == nil {
			humanOwnerUserID = validUUIDPtr(existing.HumanOwnerUserID)
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
		HumanOwnerUserID: humanOwnerUserID,
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
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
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
		HumanOwnerUserID:            validUUIDPtr(req.HumanOwnerUserID),
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
		ID:               record.ID,
		TenantID:         record.TenantID,
		Slug:             record.Slug,
		Name:             record.Name,
		Status:           record.Status,
		HumanOwnerUserID: validUUIDPtr(record.HumanOwnerUserID),
		Metadata:         cloneMap(record.Metadata),
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
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
		HumanOwnerUserID:            validUUIDPtr(record.HumanOwnerUserID),
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

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
