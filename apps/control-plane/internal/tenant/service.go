package tenant

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

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
	if len(slug) < 3 || len(slug) > 64 {
		return nil, fmt.Errorf("%w: slug must be between 3 and 64 characters", ErrInvalidInput)
	}
	matched, _ := regexp.MatchString(`^[a-z][a-z0-9-]*[a-z0-9]$`, slug)
	if !matched {
		return nil, fmt.Errorf("%w: invalid slug format", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	description, err := normalizeTeamDescription(req.Description)
	if err != nil {
		return nil, err
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
	if len(req.InitialDigitalEmployeeIDs) > MaxDigitalEmployeesPerTeam {
		return nil, fmt.Errorf("%w: digital employee capacity exceeded", ErrInvalidInput)
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
		TenantID:                  req.TenantID,
		ActorUserID:               req.ActorUserID,
		Slug:                      slug,
		Name:                      name,
		Description:               description,
		Status:                    status,
		OwnerUserIDs:              req.HumanOwnerUserIDs,
		InitialMembers:            initialMembers,
		InitialDigitalEmployeeIDs: req.InitialDigitalEmployeeIDs,
		Metadata:                  metadata,
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
	var description string
	if req.Description != nil {
		var err error
		description, err = normalizeTeamDescription(*req.Description)
		if err != nil {
			return nil, err
		}
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
	if req.HumanOwnerUserIDs == nil || req.Metadata == nil || req.Description == nil {
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
		if req.Description == nil {
			description = existing.Description
		}
	}
	record, err := s.repository.UpdateTeam(ctx, UpdateTeamParams{
		TenantID:          req.TenantID,
		TeamID:            req.TeamID,
		Slug:              slug,
		Name:              name,
		Description:       description,
		HumanOwnerUserIDs: humanOwnerUserIDs,
		Metadata:          metadata,
	})
	if err != nil {
		return nil, fmt.Errorf("update team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) UpdateTeamConstitution(ctx context.Context, tenantID, teamID uuid.UUID, constitution map[string]any) (*Team, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	record, err := s.repository.UpdateTeamConstitution(ctx, tenantID, teamID, cloneMap(constitution))
	if err != nil {
		return nil, fmt.Errorf("update team constitution: %w", err)
	}
	return teamFromRecord(record), nil
}

type DeleteTeamRequest struct {
	TenantID uuid.UUID
	TeamID   uuid.UUID
	// 删除操作者：团队唯一退出路径是删除，必须落审计（spec 2026-07-18-team-lifecycle-convergence）。
	ActorUserID uuid.UUID
}

func (s *Service) DeleteTeam(ctx context.Context, req DeleteTeamRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if err := s.repository.DeleteTeam(ctx, req.TenantID, req.TeamID, req.ActorUserID); err != nil {
		return fmt.Errorf("delete team: %w", err)
	}
	return nil
}

// ListPendingDeleteTeams 待确认删除队列(管理员)。
func (s *Service) ListPendingDeleteTeams(ctx context.Context, tenantID uuid.UUID) ([]PendingDeleteTeamRecord, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	records, err := s.repository.ListPendingDeleteTeams(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list pending delete teams: %w", err)
	}
	return records, nil
}

// ListStalePendingDeleteTeams 滞留催办扫描(跨租户,供周期提醒任务)。
func (s *Service) ListStalePendingDeleteTeams(ctx context.Context, staleBefore time.Time) ([]PendingDeleteTeamRecord, error) {
	records, err := s.repository.ListStalePendingDeleteTeams(ctx, staleBefore)
	if err != nil {
		return nil, fmt.Errorf("list stale pending delete teams: %w", err)
	}
	return records, nil
}

// ResolveOrphanPendingDeleteReminders 孤儿催办回收(供清扫任务每轮执行)。
func (s *Service) ResolveOrphanPendingDeleteReminders(ctx context.Context) error {
	if err := s.repository.ResolveOrphanPendingDeleteReminders(ctx); err != nil {
		return fmt.Errorf("resolve orphan pending delete reminders: %w", err)
	}
	return nil
}

// RestorePendingDeleteTeam 恢复待确认删除的团队;员工归属不回填。
func (s *Service) RestorePendingDeleteTeam(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) (*Team, error) {
	if tenantID == uuid.Nil || teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	record, err := s.repository.RestorePendingDeleteTeam(ctx, tenantID, teamID, actorUserID)
	if err != nil {
		return nil, fmt.Errorf("restore pending delete team: %w", err)
	}
	return teamFromRecord(record), nil
}

// ConfirmTeamDelete 确认物理删除待确认态团队。
func (s *Service) ConfirmTeamDelete(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) error {
	if tenantID == uuid.Nil || teamID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id and team_id are required", ErrInvalidInput)
	}
	if _, err := s.repository.ConfirmTeamDelete(ctx, tenantID, teamID, actorUserID); err != nil {
		return fmt.Errorf("confirm team delete: %w", err)
	}
	return nil
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
	return overview, nil
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

type BindTeamDigitalEmployeeRequest struct {
	TenantID    uuid.UUID
	TeamID      uuid.UUID
	EmployeeID  uuid.UUID
	ActorUserID uuid.UUID
}

// BindTeamDigitalEmployee 把候岗（无归属）数字员工收编进已有团队——团队归属
// 参与门禁的产品内归队入口。仅 active 团队可收编。
func (s *Service) BindTeamDigitalEmployee(ctx context.Context, req BindTeamDigitalEmployeeRequest) error {
	if req.TenantID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.EmployeeID == uuid.Nil {
		return fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	team, err := s.repository.GetTeam(ctx, req.TenantID, req.TeamID)
	if err != nil {
		return err
	}
	if team.Status != TeamStatusActive {
		return fmt.Errorf("%w: 团队状态为 %s，无法收编数字员工", ErrInvalidInput, team.Status)
	}
	if err := s.repository.BindTeamDigitalEmployee(ctx, BindTeamDigitalEmployeeParams{
		TenantID:    req.TenantID,
		TeamID:      req.TeamID,
		EmployeeID:  req.EmployeeID,
		ActorUserID: req.ActorUserID,
	}); err != nil {
		return fmt.Errorf("bind team digital employee: %w", err)
	}
	return nil
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

func teamFromRecord(record TeamRecord) *Team {
	return &Team{
		ID:                record.ID,
		TenantID:          record.TenantID,
		Slug:              record.Slug,
		Name:              record.Name,
		Description:       record.Description,
		Status:            record.Status,
		HumanOwnerUserIDs: record.HumanOwnerUserIDs,
		HumanOwners:       record.HumanOwners,
		Constitution:      cloneMap(record.Constitution),
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

func teamListItemFromRecord(record TeamListItemRecord) *TeamListItem {
	team := teamFromRecord(record.Team)
	return &TeamListItem{
		Team:                 *team,
		MemberCount:          record.MemberCount,
		DigitalEmployeeCount: record.DigitalEmployeeCount,
		CapabilityCount:      record.CapabilityCount,
		GovernanceStatus:     record.GovernanceStatus,
		PendingDraftCount:    record.PendingDraftCount,
		RiskSummary:          record.RiskSummary,
	}
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

func normalizeTeamDescription(value string) (string, error) {
	description := strings.TrimSpace(value)
	if utf8.RuneCountInString(description) > MaxTeamDescriptionLength {
		return "", fmt.Errorf("%w: description must be at most %d characters", ErrInvalidInput, MaxTeamDescriptionLength)
	}
	return description, nil
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
