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
	"github.com/superteam/control-plane/internal/oplog"
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
		Description:       params.Description,
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
		Description:       params.Description,
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
	_, err = oplog.InsertAudit(ctx, q, queries.CreateAuditEventParams{
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
	_, err = oplog.InsertAudit(ctx, q, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: params.TenantID, Valid: params.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      params.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
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
	_, err = oplog.InsertAudit(ctx, r.q, queries.CreateAuditEventParams{
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

// UnbindTeamDigitalEmployee 把数字员工移出团队，回候岗大厅（team_id = NULL）。
// 与 BindTeamDigitalEmployee 对称：底层 SQL 带 team_id 守卫，员工已被换到别的团队
// 时返回 ErrInvalidInput 而不是误解绑。
func (r *PgRepository) UnbindTeamDigitalEmployee(ctx context.Context, params BindTeamDigitalEmployeeParams) error {
	if _, err := r.q.UnbindTeamDigitalEmployee(ctx, queries.UnbindTeamDigitalEmployeeParams{
		TeamID:     params.TeamID,
		EmployeeID: params.EmployeeID,
		TenantID:   params.TenantID,
	}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: 数字员工不存在或不属于该团队", ErrInvalidInput)
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
	_, err = oplog.InsertAudit(ctx, r.q, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: params.TenantID, Valid: params.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      params.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: params.TeamID.String(), Valid: true},
		Action:       "team.digital_employee.unbind",
		Details:      details,
	})
	return err
}

func (r *PgRepository) ListDigitalEmployeeDetachBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]DetachBlocker, error) {
	rows, err := r.q.ListDigitalEmployeeDetachBlockers(ctx, queries.ListDigitalEmployeeDetachBlockersParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}
	blockers := make([]DetachBlocker, 0, len(rows))
	for _, row := range rows {
		blockers = append(blockers, DetachBlocker{
			Type:   row.BlockerType,
			RefID:  row.RefID,
			Name:   row.RefName,
			Status: row.RefStatus,
		})
	}
	return blockers, nil
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
		Description:       params.Description,
		HumanOwnerUserIds: params.HumanOwnerUserIDs,
		Metadata:          metadata,
	})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	if err := r.writeTeamMemberAudit(ctx, r.q, params.TenantID, params.TeamID, params.ActorUserID, "team.update", map[string]any{
		"team_id":              params.TeamID.String(),
		"slug":                 params.Slug,
		"name":                 params.Name,
		"human_owner_user_ids": uuidStrings(params.HumanOwnerUserIDs),
	}); err != nil {
		return TeamRecord{}, err
	}
	return teamRecordFromQuery(team)
}

func uuidStrings(ids []uuid.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

func (r *PgRepository) UpdateTeamConstitution(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID, constitution map[string]any) (TeamRecord, error) {
	constitutionJSON, err := jsonbFromMap(constitution, "constitution")
	if err != nil {
		return TeamRecord{}, err
	}
	// 宪法是团队级行为约束，改动影响全队；此前完全不记审计。
	previous, prevErr := r.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{ID: teamID, TenantID: tenantID})
	team, err := r.q.UpdateTenantTeamConstitution(ctx, queries.UpdateTenantTeamConstitutionParams{
		ID:           teamID,
		TenantID:     tenantID,
		Constitution: constitutionJSON,
	})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	details := map[string]any{
		"team_id":         teamID.String(),
		"hard_rule_count": len(stringListFromAny(constitution["hard_rules"])),
	}
	if prevErr == nil {
		if prior, mapErr := mapFromJSONB(previous.Constitution, "constitution"); mapErr == nil {
			details["prior_hard_rule_count"] = len(stringListFromAny(prior["hard_rules"]))
		}
	}
	if err := r.writeTeamMemberAudit(ctx, r.q, tenantID, teamID, actorUserID, "team.constitution.update", details); err != nil {
		return TeamRecord{}, err
	}
	return teamRecordFromQuery(team)
}

func stringListFromAny(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			out = append(out, text)
		}
	}
	return out
}

func (r *PgRepository) DeleteTeam(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) error {
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
		TeamID:            teamID,
		TenantID:          tenantID,
		DeleteRequestedBy: actorUserID,
	}); err != nil {
		return mapNoRows(err)
	}
	// 团队唯一退出路径是删除：审计与软删同事务，删除必有日志（生命周期收敛 spec §1）。
	deleteDetails, err := json.Marshal(map[string]any{"team_id": teamID.String(), "phase": "pending_delete"})
	if err != nil {
		return fmt.Errorf("marshal team delete audit details: %w", err)
	}
	if _, err := oplog.InsertAudit(ctx, qtx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      actorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "team.delete",
		Details:      deleteDetails,
	}); err != nil {
		return fmt.Errorf("record team delete audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	committed = true
	return nil
}

// ListPendingDeleteTeams 待确认删除队列(按删除时间升序,滞留最久优先)。
func (r *PgRepository) ListPendingDeleteTeams(ctx context.Context, tenantID uuid.UUID) ([]PendingDeleteTeamRecord, error) {
	rows, err := r.q.ListPendingDeleteTeams(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	records := make([]PendingDeleteTeamRecord, 0, len(rows))
	for _, row := range rows {
		record, err := pendingDeleteTeamRecordFromQuery(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// ListStalePendingDeleteTeams 滞留催办扫描(跨租户):待确认超过阈值仍无人处理的团队。
func (r *PgRepository) ListStalePendingDeleteTeams(ctx context.Context, staleBefore time.Time) ([]PendingDeleteTeamRecord, error) {
	rows, err := r.q.ListStalePendingDeleteTeams(ctx, pgtype.Timestamptz{Time: staleBefore, Valid: true})
	if err != nil {
		return nil, err
	}
	records := make([]PendingDeleteTeamRecord, 0, len(rows))
	for _, row := range rows {
		record, err := pendingDeleteTeamRecordFromQuery(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// ResolveOrphanPendingDeleteReminders 孤儿催办回收:团队离开 pending_delete 后
// 其滞留催办收件箱条目自动关闭。
func (r *PgRepository) ResolveOrphanPendingDeleteReminders(ctx context.Context) error {
	return r.q.ResolveOrphanTeamPendingDeleteReminders(ctx)
}

// RestorePendingDeleteTeam 恢复待确认删除的团队(status→active, deleted_at 清空),
// 审计与状态翻转同事务;员工归属不回填(已入候岗,由人工重新编排)。
func (r *PgRepository) RestorePendingDeleteTeam(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) (TeamRecord, error) {
	if r.db == nil {
		return TeamRecord{}, fmt.Errorf("%w: transaction starter is required", ErrInvalidInput)
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
	team, err := qtx.RestorePendingDeleteTeam(ctx, queries.RestorePendingDeleteTeamParams{
		TeamID:   teamID,
		TenantID: tenantID,
	})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	restoreDetails, err := json.Marshal(map[string]any{"team_id": teamID.String(), "slug": team.Slug, "name": team.Name})
	if err != nil {
		return TeamRecord{}, fmt.Errorf("marshal team restore audit details: %w", err)
	}
	if _, err := oplog.InsertAudit(ctx, qtx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      actorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "team.restore",
		Details:      restoreDetails,
	}); err != nil {
		return TeamRecord{}, fmt.Errorf("record team restore audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamRecord{}, err
	}
	committed = true
	return teamRecordFromQuery(team)
}

// ConfirmTeamDelete 确认物理删除:级联清理归属型数据、清引用型 team_id、删除团队行,
// 审计带团队名/slug 快照(物理删后审计仍可读)。仅允许 pending_delete 态。
func (r *PgRepository) ConfirmTeamDelete(ctx context.Context, tenantID, teamID, actorUserID uuid.UUID) (TeamRecord, error) {
	if r.db == nil {
		return TeamRecord{}, fmt.Errorf("%w: transaction starter is required", ErrInvalidInput)
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
	// 归属型数据物理清理(级联口径见 spec §2)。
	if err := qtx.HardDeleteTeamMembers(ctx, queries.HardDeleteTeamMembersParams{TenantID: tenantID, TeamID: teamID}); err != nil {
		return TeamRecord{}, fmt.Errorf("delete team members: %w", err)
	}
	if err := qtx.DeleteTeamSkillBindings(ctx, queries.DeleteTeamSkillBindingsParams{TenantID: tenantID, TeamID: teamID}); err != nil {
		return TeamRecord{}, fmt.Errorf("delete team skill bindings: %w", err)
	}
	if err := qtx.HardDeleteTeamMCPBindings(ctx, queries.HardDeleteTeamMCPBindingsParams{TenantID: tenantID, TeamID: teamID}); err != nil {
		return TeamRecord{}, fmt.Errorf("delete team mcp bindings: %w", err)
	}
	if err := qtx.HardDeleteTeamRuntimeNodeScopes(ctx, teamID); err != nil {
		return TeamRecord{}, fmt.Errorf("delete team runtime node scopes: %w", err)
	}
	if err := qtx.HardDeleteTeamUserProjectTeamScopes(ctx, teamID); err != nil {
		return TeamRecord{}, fmt.Errorf("delete team user project scopes: %w", err)
	}
	// 引用型 team_id 置 NULL(历史/审计型表保留悬空引用,可读性由审计快照兜底)。
	if err := qtx.ClearProjectsTeamRef(ctx, queries.ClearProjectsTeamRefParams{TenantID: tenantID, TeamID: teamID}); err != nil {
		return TeamRecord{}, fmt.Errorf("clear projects team ref: %w", err)
	}
	if err := qtx.ClearDigitalEmployeesTeamRef(ctx, queries.ClearDigitalEmployeesTeamRefParams{TenantID: tenantID, TeamID: teamID}); err != nil {
		return TeamRecord{}, fmt.Errorf("clear digital employees team ref: %w", err)
	}
	team, err := qtx.HardDeleteTeam(ctx, queries.HardDeleteTeamParams{TeamID: teamID, TenantID: tenantID})
	if err != nil {
		return TeamRecord{}, mapNoRows(err)
	}
	confirmDetails, err := json.Marshal(map[string]any{"team_id": teamID.String(), "slug": team.Slug, "name": team.Name, "phase": "confirmed"})
	if err != nil {
		return TeamRecord{}, fmt.Errorf("marshal team delete confirm audit details: %w", err)
	}
	if _, err := oplog.InsertAudit(ctx, qtx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      actorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       "team.delete.confirmed",
		Details:      confirmDetails,
	}); err != nil {
		return TeamRecord{}, fmt.Errorf("record team delete confirm audit: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamRecord{}, err
	}
	committed = true
	return teamRecordFromQuery(team)
}

func pendingDeleteTeamRecordFromQuery(team queries.TenantTeam) (PendingDeleteTeamRecord, error) {
	record, err := teamRecordFromQuery(team)
	if err != nil {
		return PendingDeleteTeamRecord{}, err
	}
	pending := PendingDeleteTeamRecord{Team: record, DeletedAt: timeFromTimestamptz(team.DeletedAt)}
	if team.DeleteRequestedBy.Valid {
		requestedBy := team.DeleteRequestedBy.UUID
		pending.DeleteRequestedBy = &requestedBy
	}
	return pending, nil
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

func (r *PgRepository) RequireActiveTenantLevelMembership(ctx context.Context, tenantID, userID uuid.UUID) error {
	_, err := r.q.GetActiveTenantLevelMembership(ctx, queries.GetActiveTenantLevelMembershipParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("%w: user must have tenant membership before joining a team", ErrInvalidInput)
		}
		return err
	}
	return nil
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
	// 建团时的初始成员会记审计，但事后单独加人此前不记——团队审计流里"人是什么时候
	// 进来的"因此缺一段。补齐后与移除/角色变更同口径。
	if err := r.writeTeamMemberAudit(ctx, r.q, params.TenantID, params.TeamID, params.ActorUserID, "team.member.add", map[string]any{
		"team_id":        params.TeamID.String(),
		"membership_id":  member.ID.String(),
		"target_user_id": params.UserID.String(),
		"role":           params.Role,
	}); err != nil {
		return TeamMemberRecord{}, err
	}
	return teamMemberRecordFromTenantMember(member)
}

// GrantTeamMemberRole grants a privileged role to a member AFTER permission-center
// approval (S2 apply). tenant_members is a multi-role model (unique key includes
// role), so this adds/reactivates the role row and records a team audit event.
func (r *PgRepository) GrantTeamMemberRole(ctx context.Context, in GrantTeamRoleInput) (TeamMemberRecord, error) {
	member, err := r.q.AddTeamMember(ctx, queries.AddTeamMemberParams{
		TenantID: in.TenantID,
		TeamID:   in.TeamID,
		UserID:   in.TargetUserID,
		Role:     in.Role,
	})
	if err != nil {
		return TeamMemberRecord{}, mapConstraintError(err)
	}
	details, err := json.Marshal(map[string]any{
		"team_id":        in.TeamID.String(),
		"membership_id":  member.ID.String(),
		"target_user_id": in.TargetUserID.String(),
		"role":           in.Role,
		"granted_by":     in.GrantedBy.String(),
		"via":            "permission_center_approval",
	})
	if err != nil {
		return TeamMemberRecord{}, err
	}
	if _, err := oplog.InsertAudit(ctx, r.q, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: in.TenantID, Valid: in.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      in.GrantedBy.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: in.TeamID.String(), Valid: true},
		Action:       "team.member.grant_privileged_role",
		Details:      details,
	}); err != nil {
		return TeamMemberRecord{}, err
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
	if err := r.writeTeamMemberAudit(ctx, r.q, params.TenantID, params.TeamID, params.ActorUserID, "team.member.remove", map[string]any{
		"team_id":        params.TeamID.String(),
		"membership_id":  params.MembershipID.String(),
		"target_user_id": member.PrincipalID.String(),
		"role":           member.Role,
	}); err != nil {
		return TeamMemberRecord{}, err
	}
	return teamMemberRecordFromTenantMember(member)
}

// ChangeTeamMemberRole 直接角色变更（member ⇄ viewer）。tenant_members 的唯一键
// 含 role，所以"改角色"落地为：停用旧角色行 + upsert 目标角色行（既有停用行会被
// 复活）。两步同事务，避免中途失败留下没有任何生效角色的成员。
func (r *PgRepository) ChangeTeamMemberRole(ctx context.Context, params ChangeTeamMemberRoleParams) (TeamMemberRecord, error) {
	if r.db == nil {
		return TeamMemberRecord{}, fmt.Errorf("%w: transaction starter is required", ErrInvalidInput)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return TeamMemberRecord{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()
	qtx := r.q.WithTx(tx)

	previous, err := qtx.GetTeamMember(ctx, queries.GetTeamMemberParams{
		MembershipID: params.MembershipID,
		TenantID:     params.TenantID,
		TeamID:       params.TeamID,
	})
	if err != nil {
		return TeamMemberRecord{}, mapNoRows(err)
	}
	if _, err := qtx.DisableTeamMemberRole(ctx, queries.DisableTeamMemberRoleParams{
		MembershipID: params.MembershipID,
		TenantID:     params.TenantID,
		TeamID:       params.TeamID,
	}); err != nil {
		return TeamMemberRecord{}, mapNoRows(err)
	}
	member, err := qtx.AddTeamMember(ctx, queries.AddTeamMemberParams{
		TenantID: params.TenantID,
		TeamID:   params.TeamID,
		UserID:   previous.UserID,
		Role:     params.Role,
	})
	if err != nil {
		return TeamMemberRecord{}, mapConstraintError(err)
	}
	if err := r.writeTeamMemberAudit(ctx, qtx, params.TenantID, params.TeamID, params.ActorUserID, "team.member.change_role", map[string]any{
		"team_id":             params.TeamID.String(),
		"membership_id":       member.ID.String(),
		"prior_membership_id": params.MembershipID.String(),
		"target_user_id":      previous.UserID.String(),
		"from_role":           previous.Role,
		"to_role":             params.Role,
	}); err != nil {
		return TeamMemberRecord{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TeamMemberRecord{}, err
	}
	committed = true
	return teamMemberRecordFromTenantMember(member)
}

// writeTeamMemberAudit 落成员相关的团队审计事件。资源维度统一为 team，
// membership_id / target_user_id 进 details——团队审计流按 resource_type='team'
// 过滤，写在 team_member 上等于在团队视角不可见。
func (r *PgRepository) writeTeamMemberAudit(
	ctx context.Context,
	q *queries.Queries,
	tenantID, teamID, actorUserID uuid.UUID,
	action string,
	details map[string]any,
) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = oplog.InsertAudit(ctx, q, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      actorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       action,
		Details:      payload,
	})
	return err
}

func (r *PgRepository) CountTeamOwners(ctx context.Context, tenantID, teamID uuid.UUID) (int32, error) {
	return r.q.CountTeamOwners(ctx, queries.CountTeamOwnersParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
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
		Description:       team.Description,
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
			Description:       row.Description,
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
			Description:       row.Description,
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

func (r *PgRepository) CreateTeamConstitutionRevision(ctx context.Context, params CreateTeamConstitutionRevisionParams) (TeamConstitutionRevision, error) {
	rules, err := json.Marshal(params.Rules)
	if err != nil {
		return TeamConstitutionRevision{}, err
	}
	row, err := r.q.CreateTeamConstitutionRevision(ctx, queries.CreateTeamConstitutionRevisionParams{
		TenantID:   params.TenantID,
		TeamID:     params.TeamID,
		Rules:      rules,
		ChangeNote: params.ChangeNote,
		CreatedBy:  uuid.NullUUID{UUID: params.ActorUserID, Valid: params.ActorUserID != uuid.Nil},
	})
	if err != nil {
		return TeamConstitutionRevision{}, err
	}
	return constitutionRevisionFromRow(row.ID, row.TenantID, row.TeamID, row.RevisionNumber, row.Rules, row.ChangeNote, row.CreatedBy, "", row.CreatedAt)
}

func (r *PgRepository) ListTeamConstitutionRevisions(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int32) ([]TeamConstitutionRevision, error) {
	rows, err := r.q.ListTeamConstitutionRevisions(ctx, queries.ListTeamConstitutionRevisionsParams{
		TenantID: tenantID,
		TeamID:   teamID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}
	revisions := make([]TeamConstitutionRevision, 0, len(rows))
	for _, row := range rows {
		revision, err := constitutionRevisionFromRow(row.ID, row.TenantID, row.TeamID, row.RevisionNumber, row.Rules, row.ChangeNote, row.CreatedBy, row.CreatedByName, row.CreatedAt)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, nil
}

func (r *PgRepository) GetTeamConstitutionRevision(ctx context.Context, tenantID, teamID uuid.UUID, revisionNumber int32) (TeamConstitutionRevision, error) {
	row, err := r.q.GetTeamConstitutionRevision(ctx, queries.GetTeamConstitutionRevisionParams{
		TenantID:       tenantID,
		TeamID:         teamID,
		RevisionNumber: revisionNumber,
	})
	if err != nil {
		return TeamConstitutionRevision{}, mapNoRows(err)
	}
	return constitutionRevisionFromRow(row.ID, row.TenantID, row.TeamID, row.RevisionNumber, row.Rules, row.ChangeNote, row.CreatedBy, row.CreatedByName, row.CreatedAt)
}

func constitutionRevisionFromRow(
	id, tenantID, teamID uuid.UUID,
	revisionNumber int32,
	rawRules []byte,
	changeNote string,
	createdBy uuid.NullUUID,
	createdByName string,
	createdAt pgtype.Timestamptz,
) (TeamConstitutionRevision, error) {
	var rules []ConstitutionRule
	if len(rawRules) > 0 {
		if err := json.Unmarshal(rawRules, &rules); err != nil {
			return TeamConstitutionRevision{}, fmt.Errorf("decode constitution rules: %w", err)
		}
	}
	revision := TeamConstitutionRevision{
		ID:             id,
		TenantID:       tenantID,
		TeamID:         teamID,
		RevisionNumber: revisionNumber,
		Rules:          rules,
		ChangeNote:     changeNote,
		CreatedByName:  createdByName,
	}
	if createdBy.Valid {
		actor := createdBy.UUID
		revision.CreatedBy = &actor
	}
	if createdAt.Valid {
		revision.CreatedAt = createdAt.Time.UTC().Format(time.RFC3339)
	}
	return revision, nil
}
