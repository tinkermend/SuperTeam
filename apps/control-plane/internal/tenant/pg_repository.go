package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q  *queries.Queries
	db txBeginner
}

type txBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func NewPgRepository(q *queries.Queries, db ...txBeginner) Repository {
	var txDB txBeginner
	if len(db) > 0 {
		txDB = db[0]
	}
	return &PgRepository{q: q, db: txDB}
}

func (r *PgRepository) CreateTeam(ctx context.Context, params CreateTeamParams) (TeamRecord, error) {
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return TeamRecord{}, err
	}
	team, err := r.q.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:          params.TenantID,
		Slug:              params.Slug,
		Name:              params.Name,
		Status:            string(params.Status),
		HumanOwnerUserIds: params.HumanOwnerUserIDs,
		Metadata:          metadata,
	})
	if err != nil {
		return TeamRecord{}, mapConstraintError(err)
	}
	return teamRecordFromQuery(team)
}

func (r *PgRepository) CreateTeamWithInitialMembers(ctx context.Context, params CreateTeamWithInitialMembersParams) (TeamRecord, error) {
	if r.db == nil {
		return TeamRecord{}, fmt.Errorf("%w: transaction starter is required", ErrInvalidInput)
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return TeamRecord{}, err
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return TeamRecord{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	qtx := r.q.WithTx(tx)
	for _, ownerID := range params.OwnerUserIDs {
		if _, err := qtx.GetActiveTenantUserForTeamCreate(ctx, queries.GetActiveTenantUserForTeamCreateParams{
			ID:       ownerID,
			TenantID: params.TenantID,
		}); err != nil {
			return TeamRecord{}, mapNoRows(err)
		}
	}
	for _, member := range params.InitialMembers {
		if _, err := qtx.GetActiveTenantUserForTeamCreate(ctx, queries.GetActiveTenantUserForTeamCreateParams{
			ID:       member.UserID,
			TenantID: params.TenantID,
		}); err != nil {
			return TeamRecord{}, mapNoRows(err)
		}
	}
	team, err := qtx.CreateTenantTeam(ctx, queries.CreateTenantTeamParams{
		TenantID:          params.TenantID,
		Slug:              params.Slug,
		Name:              params.Name,
		Status:            string(params.Status),
		HumanOwnerUserIds: params.OwnerUserIDs,
		Metadata:          metadata,
	})
	if err != nil {
		return TeamRecord{}, mapConstraintError(err)
	}
	if err := createTeamAuditEvent(ctx, qtx, params, team.ID); err != nil {
		return TeamRecord{}, err
	}
	for _, ownerID := range params.OwnerUserIDs {
		ownerMembership, err := qtx.AddTeamOwnerMembership(ctx, queries.AddTeamOwnerMembershipParams{
			TenantID: params.TenantID,
			TeamID:   team.ID,
			UserID:   ownerID,
		})
		if err != nil {
			return TeamRecord{}, mapConstraintError(err)
		}
		if err := createTeamMemberAuditEvent(ctx, qtx, params, team.ID, ownerMembership.ID, ownerID, TeamRoleOwner); err != nil {
			return TeamRecord{}, err
		}
	}
	for _, member := range params.InitialMembers {
		membership, err := qtx.AddTeamMember(ctx, queries.AddTeamMemberParams{
			TenantID: params.TenantID,
			TeamID:   team.ID,
			UserID:   member.UserID,
			Role:     member.Role,
		})
		if err != nil {
			return TeamRecord{}, mapConstraintError(err)
		}
		if err := createTeamMemberAuditEvent(ctx, qtx, params, team.ID, membership.ID, member.UserID, member.Role); err != nil {
			return TeamRecord{}, err
		}
	}
	for _, employeeID := range params.InitialDigitalEmployeeIDs {
		_, err := qtx.BindDigitalEmployeeToTeam(ctx, queries.BindDigitalEmployeeToTeamParams{
			TeamID:     team.ID,
			EmployeeID: employeeID,
			TenantID:   params.TenantID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return TeamRecord{}, fmt.Errorf("digital employee %s not found or already assigned: %w", employeeID, ErrInvalidInput)
			}
			return TeamRecord{}, err
		}
	}
	record, err := teamRecordFromQuery(team)
	if err != nil {
		return TeamRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamRecord{}, err
	}
	committed = true
	return record, nil
}

func createTeamAuditEvent(ctx context.Context, q *queries.Queries, params CreateTeamWithInitialMembersParams, teamID uuid.UUID) error {
	var ownerIDStrs []string
	for _, id := range params.OwnerUserIDs {
		ownerIDStrs = append(ownerIDStrs, id.String())
	}
	details, err := json.Marshal(map[string]any{
		"team_id":              teamID.String(),
		"slug":                 params.Slug,
		"human_owner_user_ids": ownerIDStrs,
		"initial_members":      len(params.InitialMembers),
	})
	if err != nil {
		return err
	}
	_, err = q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: params.TenantID, Valid: params.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      params.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "team.create",
		Details:      details,
	})
	return err
}

func createTeamMemberAuditEvent(ctx context.Context, q *queries.Queries, params CreateTeamWithInitialMembersParams, teamID, membershipID, userID uuid.UUID, role string) error {
	details, err := json.Marshal(map[string]any{
		"team_id":       teamID.String(),
		"membership_id": membershipID.String(),
		"user_id":       userID.String(),
		"role":          role,
	})
	if err != nil {
		return err
	}
	_, err = q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: params.TenantID, Valid: params.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      params.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "team_member", Valid: true},
		ResourceID:   pgtype.Text{String: membershipID.String(), Valid: true},
		Action:       "team.member.add",
		Details:      details,
	})
	return err
}

// BindTeamDigitalEmployee 收编一名候岗（无归属）数字员工进团队。底层 SQL 带
// team_id IS NULL 守卫：已有归属的员工不会被抢占，返回 ErrInvalidInput。
func (r *PgRepository) BindTeamDigitalEmployee(ctx context.Context, params BindTeamDigitalEmployeeParams) error {
	_, err := r.q.BindDigitalEmployeeToTeam(ctx, queries.BindDigitalEmployeeToTeamParams{
		TeamID:     params.TeamID,
		EmployeeID: params.EmployeeID,
		TenantID:   params.TenantID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: 数字员工不存在或已有团队归属", ErrInvalidInput)
		}
		return err
	}
	details, err := json.Marshal(map[string]any{
		"team_id":             params.TeamID.String(),
		"digital_employee_id": params.EmployeeID.String(),
	})
	if err != nil {
		return err
	}
	_, err = r.q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: params.TenantID, Valid: params.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      params.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: params.TeamID.String(), Valid: true},
		Action:       "team.digital_employee.bind",
		Details:      details,
	})
	return err
}

func (r *PgRepository) ListTeams(ctx context.Context, params ListTeamsParams) ([]TeamRecord, error) {
	teams, err := r.q.ListTenantTeams(ctx, queries.ListTenantTeamsParams{
		TenantID: params.TenantID,
		Status:   textFromTeamStatus(params.Status),
		Offset:   params.Offset,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]TeamRecord, 0, len(teams))
	for _, team := range teams {
		record, err := teamRecordFromQuery(team)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) ListTeamSummaries(ctx context.Context, params ListTeamSummariesParams) ([]TeamListItemRecord, error) {
	rows, err := r.q.ListTenantTeamSummaries(ctx, queries.ListTenantTeamSummariesParams{
		TenantID:         params.TenantID,
		Status:           textFromTeamStatus(params.Status),
		GovernanceStatus: textFromGovernanceSummaryStatus(params.GovernanceStatus),
		Q:                textFromString(params.Q),
		Offset:           params.Offset,
		Limit:            params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]TeamListItemRecord, 0, len(rows))
	for _, row := range rows {
		record, err := teamListItemRecordFromQuery(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) GetTeamSummary(ctx context.Context, tenantID, teamID uuid.UUID) (TeamListItemRecord, error) {
	row, err := r.q.GetTenantTeamSummary(ctx, queries.GetTenantTeamSummaryParams{
		ID:       teamID,
		TenantID: tenantID,
	})
	if err != nil {
		return TeamListItemRecord{}, mapNoRows(err)
	}
	return teamListItemRecordFromGetSummaryQuery(row)
}

func (r *PgRepository) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error) {
	team, err := r.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{
		ID:       teamID,
		TenantID: tenantID,
	})
	if err != nil {
		return TeamRecord{}, mapConstraintError(mapNoRows(err))
	}
	return teamRecordFromQuery(team)
}

func (r *PgRepository) UpdateTeam(ctx context.Context, params UpdateTeamParams) (TeamRecord, error) {
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return TeamRecord{}, err
	}
	team, err := r.q.UpdateTenantTeam(ctx, queries.UpdateTenantTeamParams{
		ID:                params.TeamID,
		TenantID:          params.TenantID,
		Slug:              params.Slug,
		Name:              params.Name,
		HumanOwnerUserIds: params.HumanOwnerUserIDs,
		Metadata:          metadata,
	})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	return teamRecordFromQuery(team)
}

func (r *PgRepository) UpdateTeamConstitution(ctx context.Context, tenantID, teamID uuid.UUID, constitution map[string]any) (TeamRecord, error) {
	constitutionJSON, err := jsonbFromMap(constitution, "constitution")
	if err != nil {
		return TeamRecord{}, err
	}
	team, err := r.q.UpdateTenantTeamConstitution(ctx, queries.UpdateTenantTeamConstitutionParams{
		ID:           teamID,
		TenantID:     tenantID,
		Constitution: constitutionJSON,
	})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	return teamRecordFromQuery(team)
}

func (r *PgRepository) SetTeamStatus(ctx context.Context, params SetTeamStatusParams) (TeamRecord, error) {
	team, err := r.q.SetTenantTeamStatus(ctx, queries.SetTenantTeamStatusParams{
		ID:       params.TeamID,
		TenantID: params.TenantID,
		Status:   string(params.Status),
	})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	return teamRecordFromQuery(team)
}

func (r *PgRepository) DeleteTeam(ctx context.Context, tenantID, teamID uuid.UUID) error {
	if r.db == nil {
		return fmt.Errorf("%w: transaction starter is required", ErrInvalidInput)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	qtx := r.q.WithTx(tx)
	if err := qtx.UnbindTeamDigitalEmployees(ctx, queries.UnbindTeamDigitalEmployeesParams{
		TeamID:   teamID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("unbind team employees: %w", err)
	}
	if err := qtx.DeleteTeamSkillBindings(ctx, queries.DeleteTeamSkillBindingsParams{
		TenantID: tenantID,
		TeamID:   teamID,
	}); err != nil {
		return fmt.Errorf("delete team skill bindings: %w", err)
	}
	if err := qtx.SoftDeleteTeamMCPBindings(ctx, queries.SoftDeleteTeamMCPBindingsParams{
		TenantID: tenantID,
		TeamID:   teamID,
	}); err != nil {
		return fmt.Errorf("soft delete team mcp bindings: %w", err)
	}
	if _, err := qtx.SoftDeleteTeam(ctx, queries.SoftDeleteTeamParams{
		TeamID:   teamID,
		TenantID: tenantID,
	}); err != nil {
		return mapNoRows(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

func (r *PgRepository) ListTeamMembers(ctx context.Context, params ListTeamMembersParams) ([]TeamMemberRecord, error) {
	rows, err := r.q.ListTeamMembers(ctx, queries.ListTeamMembersParams{
		TenantID: params.TenantID,
		TeamID:   params.TeamID,
		Offset:   params.Offset,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]TeamMemberRecord, 0, len(rows))
	for _, row := range rows {
		record, err := teamMemberRecordFromListRow(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) GetTeamMember(ctx context.Context, tenantID, teamID, membershipID uuid.UUID) (TeamMemberRecord, error) {
	row, err := r.q.GetTeamMember(ctx, queries.GetTeamMemberParams{
		MembershipID: membershipID,
		TenantID:     tenantID,
		TeamID:       teamID,
	})
	if err != nil {
		return TeamMemberRecord{}, mapNoRows(err)
	}
	return teamMemberRecordFromGetRow(row)
}

func (r *PgRepository) AddTeamMember(ctx context.Context, params AddTeamMemberParams) (TeamMemberRecord, error) {
	member, err := r.q.AddTeamMember(ctx, queries.AddTeamMemberParams{
		TenantID: params.TenantID,
		TeamID:   params.TeamID,
		UserID:   params.UserID,
		Role:     params.Role,
	})
	if err != nil {
		return TeamMemberRecord{}, mapConstraintError(err)
	}
	return teamMemberRecordFromTenantMember(member)
}

func (r *PgRepository) DisableTeamMemberRole(ctx context.Context, params DisableTeamMemberRoleParams) (TeamMemberRecord, error) {
	member, err := r.q.DisableTeamMemberRole(ctx, queries.DisableTeamMemberRoleParams{
		MembershipID: params.MembershipID,
		TenantID:     params.TenantID,
		TeamID:       params.TeamID,
	})
	if err != nil {
		return TeamMemberRecord{}, mapNoRows(err)
	}
	return teamMemberRecordFromTenantMember(member)
}

func (r *PgRepository) CountTeamOwners(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error) {
	return r.q.CountTeamOwners(ctx, queries.CountTeamOwnersParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
}

func (r *PgRepository) CreateTeamMemberRoleRequest(ctx context.Context, params CreateTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error) {
	request, err := r.q.CreateTeamMemberRoleRequest(ctx, queries.CreateTeamMemberRoleRequestParams{
		TenantID:      params.TenantID,
		TeamID:        params.TeamID,
		TargetUserID:  params.TargetUserID,
		RequestedRole: params.RequestedRole,
		RequestedBy:   params.RequestedBy,
		Reason:        params.Reason,
	})
	if err != nil {
		return TeamMemberRoleRequestRecord{}, mapConstraintError(err)
	}
	return roleRequestRecordFromQuery(request), nil
}

func (r *PgRepository) GetTeamMemberRoleRequest(ctx context.Context, tenantID, teamID, requestID uuid.UUID) (TeamMemberRoleRequestRecord, error) {
	request, err := r.q.GetTeamMemberRoleRequest(ctx, queries.GetTeamMemberRoleRequestParams{
		ID:       requestID,
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return TeamMemberRoleRequestRecord{}, mapNoRows(err)
	}
	return roleRequestRecordFromQuery(request), nil
}

func (r *PgRepository) ListTeamMemberRoleRequests(ctx context.Context, params ListTeamMemberRoleRequestsParams) ([]TeamMemberRoleRequestRecord, error) {
	requests, err := r.q.ListTeamMemberRoleRequests(ctx, queries.ListTeamMemberRoleRequestsParams{
		TenantID: params.TenantID,
		TeamID:   params.TeamID,
		Status:   textFromRoleRequestStatus(params.Status),
		Offset:   params.Offset,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]TeamMemberRoleRequestRecord, 0, len(requests))
	for _, request := range requests {
		records = append(records, roleRequestRecordFromQuery(request))
	}
	return records, nil
}

func (r *PgRepository) ApproveTeamMemberRoleRequest(ctx context.Context, params DecideTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error) {
	if r.db == nil {
		return r.approveTeamMemberRoleRequestWithQueries(ctx, r.q, params)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return TeamMemberRoleRequestRecord{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	record, err := r.approveTeamMemberRoleRequestWithQueries(ctx, r.q.WithTx(tx), params)
	if err != nil {
		return TeamMemberRoleRequestRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamMemberRoleRequestRecord{}, err
	}
	committed = true
	return record, nil
}

func (r *PgRepository) approveTeamMemberRoleRequestWithQueries(ctx context.Context, q *queries.Queries, params DecideTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error) {
	pending, err := q.GetTeamMemberRoleRequest(ctx, queries.GetTeamMemberRoleRequestParams{
		ID:       params.RequestID,
		TenantID: params.TenantID,
		TeamID:   params.TeamID,
	})
	if err != nil {
		return TeamMemberRoleRequestRecord{}, mapNoRows(err)
	}
	if _, err := q.AddTeamMember(ctx, queries.AddTeamMemberParams{
		TenantID: pending.TenantID,
		TeamID:   pending.TeamID,
		UserID:   pending.TargetUserID,
		Role:     pending.RequestedRole,
	}); err != nil {
		return TeamMemberRoleRequestRecord{}, mapConstraintError(err)
	}
	decided, err := q.DecideTeamMemberRoleRequest(ctx, queries.DecideTeamMemberRoleRequestParams{
		ID:             params.RequestID,
		TenantID:       params.TenantID,
		TeamID:         params.TeamID,
		Status:         string(TeamMemberRoleRequestStatusApproved),
		DecidedBy:      params.DecidedBy,
		DecisionReason: params.DecisionReason,
	})
	if err != nil {
		return TeamMemberRoleRequestRecord{}, mapNoRows(err)
	}
	return roleRequestRecordFromQuery(decided), nil
}

func (r *PgRepository) DecideTeamMemberRoleRequest(ctx context.Context, params DecideTeamMemberRoleRequestParams) (TeamMemberRoleRequestRecord, error) {
	request, err := r.q.DecideTeamMemberRoleRequest(ctx, queries.DecideTeamMemberRoleRequestParams{
		ID:             params.RequestID,
		TenantID:       params.TenantID,
		TeamID:         params.TeamID,
		Status:         string(params.Status),
		DecidedBy:      params.DecidedBy,
		DecisionReason: params.DecisionReason,
	})
	if err != nil {
		return TeamMemberRoleRequestRecord{}, mapNoRows(err)
	}
	return roleRequestRecordFromQuery(request), nil
}

func teamRecordFromQuery(team queries.TenantTeam) (TeamRecord, error) {
	metadata, err := mapFromJSONB(team.Metadata, "metadata")
	if err != nil {
		return TeamRecord{}, err
	}
	constitution, err := mapFromJSONB(team.Constitution, "constitution")
	if err != nil {
		return TeamRecord{}, err
	}
	return TeamRecord{
		ID:                team.ID,
		TenantID:          team.TenantID,
		Slug:              team.Slug,
		Name:              team.Name,
		Status:            TeamStatus(team.Status),
		HumanOwnerUserIDs: team.HumanOwnerUserIds,
		Constitution:      constitution,
		Metadata:          metadata,
		CreatedAt:         timeFromTimestamptz(team.CreatedAt),
		UpdatedAt:         timeFromTimestamptz(team.UpdatedAt),
	}, nil
}

func teamListItemRecordFromQuery(row queries.ListTenantTeamSummariesRow) (TeamListItemRecord, error) {
	return teamListItemRecordFromSummaryParts(
		queries.TenantTeam{
			ID:                row.ID,
			TenantID:          row.TenantID,
			Slug:              row.Slug,
			Name:              row.Name,
			Status:            row.Status,
			HumanOwnerUserIds: row.HumanOwnerUserIds,
			Constitution:      row.Constitution,
			Metadata:          row.Metadata,
			ArchivedAt:        row.ArchivedAt,
			DisabledAt:        row.DisabledAt,
			DeletedAt:         row.DeletedAt,
			CreatedAt:         row.CreatedAt,
			UpdatedAt:         row.UpdatedAt,
		},
		row.MemberCount,
		row.DigitalEmployeeCount,
		row.CapabilityCount,
		row.PendingDraftCount,
		row.GovernanceStatus,
		row.RiskSummary,
		parseTeamHumanOwners(row.HumanOwners),
	)
}

func teamListItemRecordFromGetSummaryQuery(row queries.GetTenantTeamSummaryRow) (TeamListItemRecord, error) {
	return teamListItemRecordFromSummaryParts(
		queries.TenantTeam{
			ID:                row.ID,
			TenantID:          row.TenantID,
			Slug:              row.Slug,
			Name:              row.Name,
			Status:            row.Status,
			HumanOwnerUserIds: row.HumanOwnerUserIds,
			Constitution:      row.Constitution,
			Metadata:          row.Metadata,
			ArchivedAt:        row.ArchivedAt,
			DisabledAt:        row.DisabledAt,
			DeletedAt:         row.DeletedAt,
			CreatedAt:         row.CreatedAt,
			UpdatedAt:         row.UpdatedAt,
		},
		row.MemberCount,
		row.DigitalEmployeeCount,
		row.CapabilityCount,
		row.PendingDraftCount,
		row.GovernanceStatus,
		row.RiskSummary,
		parseTeamHumanOwners(row.HumanOwners),
	)
}

func teamListItemRecordFromSummaryParts(
	tenantTeam queries.TenantTeam,
	memberCount int32,
	digitalEmployeeCount int32,
	capabilityCount int32,
	pendingDraftCount int32,
	governanceStatus string,
	riskSummary string,
	humanOwners []TeamHumanOwner,
) (TeamListItemRecord, error) {
	team, err := teamRecordFromQuery(tenantTeam)
	if err != nil {
		return TeamListItemRecord{}, err
	}
	team.HumanOwners = humanOwners
	return TeamListItemRecord{
		Team:                 team,
		MemberCount:          memberCount,
		DigitalEmployeeCount: digitalEmployeeCount,
		CapabilityCount:      capabilityCount,
		PendingDraftCount:    pendingDraftCount,
		GovernanceStatus:     GovernanceSummaryStatus(governanceStatus),
		RiskSummary:          riskSummary,
	}, nil
}

func parseTeamHumanOwners(b []byte) []TeamHumanOwner {
	if len(b) == 0 {
		return nil
	}
	var rawOwners []struct {
		ID             uuid.UUID      `json:"id"`
		Username       string         `json:"username"`
		DisplayName    *string        `json:"display_name"`
		Email          *string        `json:"email"`
		Status         string         `json:"status"`
		AvatarProvider *string        `json:"avatar_provider"`
		AvatarStyle    *string        `json:"avatar_style"`
		AvatarSeed     *string        `json:"avatar_seed"`
		AvatarOptions  map[string]any `json:"avatar_options"`
	}
	if err := json.Unmarshal(b, &rawOwners); err != nil {
		return nil
	}
	var owners []TeamHumanOwner
	for _, ro := range rawOwners {
		o := TeamHumanOwner{
			UserID:   ro.ID,
			Username: ro.Username,
			Status:   ro.Status,
		}
		if ro.DisplayName != nil {
			o.DisplayName = *ro.DisplayName
		}
		if ro.Email != nil {
			o.Email = *ro.Email
		}
		var provider, style, seed pgtype.Text
		if ro.AvatarProvider != nil {
			provider = pgtype.Text{String: *ro.AvatarProvider, Valid: true}
		}
		if ro.AvatarStyle != nil {
			style = pgtype.Text{String: *ro.AvatarStyle, Valid: true}
		}
		if ro.AvatarSeed != nil {
			seed = pgtype.Text{String: *ro.AvatarSeed, Valid: true}
		}
		var opts []byte
		if ro.AvatarOptions != nil {
			opts, _ = json.Marshal(ro.AvatarOptions)
		}
		o.Avatar = avatarFromFields(ro.Username, provider, style, seed, opts)
		owners = append(owners, o)
	}
	return owners
}

func teamMemberRecordFromListRow(row queries.ListTeamMembersRow) (TeamMemberRecord, error) {
	return teamMemberRecordFromParts(
		row.MembershipID,
		row.TenantID,
		row.TeamID,
		row.UserID,
		row.Username,
		stringFromText(row.DisplayName),
		stringFromText(row.Email),
		row.AccountStatus,
		avatarFromMemberFields(row.Username, row.AvatarProvider, row.AvatarStyle, row.AvatarSeed, row.AvatarOptions),
		row.Role,
		row.MembershipStatus,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func teamMemberRecordFromGetRow(row queries.GetTeamMemberRow) (TeamMemberRecord, error) {
	return teamMemberRecordFromParts(
		row.MembershipID,
		row.TenantID,
		row.TeamID,
		row.UserID,
		row.Username,
		stringFromText(row.DisplayName),
		stringFromText(row.Email),
		row.AccountStatus,
		avatarFromMemberFields(row.Username, row.AvatarProvider, row.AvatarStyle, row.AvatarSeed, row.AvatarOptions),
		row.Role,
		row.MembershipStatus,
		row.CreatedAt,
		row.UpdatedAt,
	)
}

func teamMemberRecordFromTenantMember(member queries.TenantMember) (TeamMemberRecord, error) {
	return teamMemberRecordFromParts(
		member.ID,
		member.TenantID,
		member.TeamID,
		member.PrincipalID,
		"",
		"",
		"",
		"",
		nil,
		member.Role,
		member.Status,
		member.CreatedAt,
		member.UpdatedAt,
	)
}

func teamMemberRecordFromParts(
	membershipID uuid.UUID,
	tenantID uuid.UUID,
	teamID uuid.NullUUID,
	userID uuid.UUID,
	username string,
	displayName string,
	email string,
	accountStatus string,
	avatar *UserAvatarConfig,
	role string,
	membershipStatus string,
	createdAt pgtype.Timestamptz,
	updatedAt pgtype.Timestamptz,
) (TeamMemberRecord, error) {
	if !teamID.Valid || teamID.UUID == uuid.Nil {
		return TeamMemberRecord{}, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	return TeamMemberRecord{
		MembershipID:     membershipID,
		TenantID:         tenantID,
		TeamID:           teamID.UUID,
		UserID:           userID,
		Username:         username,
		DisplayName:      displayName,
		Email:            email,
		AccountStatus:    accountStatus,
		Avatar:           cloneUserAvatarConfig(avatar),
		Role:             role,
		MembershipStatus: membershipStatus,
		CreatedAt:        timeFromTimestamptz(createdAt),
		UpdatedAt:        timeFromTimestamptz(updatedAt),
	}, nil
}

func avatarFromFields(username string, provider, style, seed pgtype.Text, options []byte) *UserAvatarConfig {
	if !provider.Valid || !style.Valid {
		return nil
	}
	return avatarFromValues(username, provider.String, style.String, stringFromText(seed), options)
}

func avatarFromMemberFields(username, provider, style string, seed pgtype.Text, options []byte) *UserAvatarConfig {
	if provider == "" || style == "" {
		return nil
	}
	return avatarFromValues(username, provider, style, stringFromText(seed), options)
}

func avatarFromValues(username, provider, style, seed string, options []byte) *UserAvatarConfig {
	if strings.TrimSpace(seed) == "" {
		seed = "user:" + strings.TrimSpace(username)
	}
	avatar := &UserAvatarConfig{
		Provider: provider,
		Style:    style,
		Seed:     seed,
	}
	if len(options) > 0 {
		var parsed map[string]any
		if err := json.Unmarshal(options, &parsed); err == nil && parsed != nil {
			avatar.Options = parsed
		}
	}
	return avatar
}

func roleRequestRecordFromQuery(request queries.TenantTeamMemberRoleRequest) TeamMemberRoleRequestRecord {
	return TeamMemberRoleRequestRecord{
		ID:             request.ID,
		TenantID:       request.TenantID,
		TeamID:         request.TeamID,
		TargetUserID:   request.TargetUserID,
		RequestedRole:  request.RequestedRole,
		RequestedBy:    request.RequestedBy,
		Status:         TeamMemberRoleRequestStatus(request.Status),
		Reason:         request.Reason,
		DecidedBy:      uuidPtrFromNull(request.DecidedBy),
		DecidedAt:      timePtrFromTimestamptz(request.DecidedAt),
		DecisionReason: request.DecisionReason,
		CreatedAt:      timeFromTimestamptz(request.CreatedAt),
		UpdatedAt:      timeFromTimestamptz(request.UpdatedAt),
	}
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func mapConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: unique constraint violation", ErrInvalidInput)
	}
	return err
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid || value.UUID == uuid.Nil {
		return nil
	}
	copied := value.UUID
	return &copied
}

func textFromTeamStatus(status TeamStatus) pgtype.Text {
	if status == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(status), Valid: true}
}

func textFromGovernanceSummaryStatus(status GovernanceSummaryStatus) pgtype.Text {
	if status == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(status), Valid: true}
}

func textFromString(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func textFromRoleRequestStatus(status TeamMemberRoleRequestStatus) pgtype.Text {
	if status == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(status), Valid: true}
}

func stringFromText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func int32PtrFromInt4(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copied := value.Int32
	return &copied
}

func timestamptzFromPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil || value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func timePtrFromTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func jsonbFromMap(value map[string]any, field string) ([]byte, error) {
	encoded, err := json.Marshal(cloneMap(value))
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func jsonbFromOptionalMap(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return jsonbFromMap(value, field)
}

func mapFromJSONB(raw []byte, field string) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}
