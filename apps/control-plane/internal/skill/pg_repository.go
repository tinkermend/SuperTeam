package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	db *pgxpool.Pool
	q  *queries.Queries
}

func NewPgRepository(db *pgxpool.Pool, querySources ...*queries.Queries) *PgRepository {
	var q *queries.Queries
	if len(querySources) > 0 {
		q = querySources[0]
	}
	if q == nil && db != nil {
		q = queries.New(db)
	}
	return &PgRepository{db: db, q: q}
}

const skillSelectColumns = `s.id, s.tenant_id, s.slug, s.name, s.description, s.version, s.source, s.risk_level, s.icon_key, s.color_token, s.tags, COALESCE(s.metadata, '{}'::jsonb) AS metadata, s.archive_object_ref, s.archive_filename, s.archive_size_bytes, s.archive_checksum_sha256, s.archive_file_count, COALESCE(s.created_by::text, '') AS created_by, COALESCE(au.display_name, au.username, '') AS created_by_name, s.created_at, s.updated_at`

const skillJoinClause = `LEFT JOIN auth_users au ON au.id = s.created_by`

func (r *PgRepository) ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	conditions := []string{"s.tenant_id = $1", "s.deleted_at IS NULL"}
	args := []any{req.TenantID}
	if strings.TrimSpace(req.Q) != "" {
		args = append(args, "%"+strings.TrimSpace(req.Q)+"%")
		conditions = append(conditions, fmt.Sprintf("(s.name ILIKE $%d OR s.description ILIKE $%d OR s.slug ILIKE $%d)", len(args), len(args), len(args)))
	}
	rows, err := r.db.Query(ctx, `
SELECT `+skillSelectColumns+`
FROM skills s `+skillJoinClause+`
WHERE `+strings.Join(conditions, " AND ")+`
ORDER BY s.updated_at DESC, s.name ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var skills []*Skill
	for rows.Next() {
		item, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, item := range skills {
		if err := r.loadChildren(ctx, item); err != nil {
			return nil, err
		}
	}
	return skills, nil
}

func (r *PgRepository) GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error) {
	row := r.db.QueryRow(ctx, `
SELECT `+skillSelectColumns+`
FROM skills s `+skillJoinClause+`
WHERE s.tenant_id = $1 AND s.id = $2 AND s.deleted_at IS NULL`, req.TenantID, req.SkillID)
	item, err := scanSkill(row)
	if err != nil {
		return nil, mapNoRows(err)
	}
	if err := r.loadChildren(ctx, item); err != nil {
		return nil, err
	}
	return item, nil
}

func (r *PgRepository) UpsertSkillPackage(ctx context.Context, req UpsertSkillPackageRequest) (*Skill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		// Roll back unconditionally on any early return or panic; it is a no-op
		// once Commit succeeds, so a leaked open tx can never strand a pooled conn.
		_ = tx.Rollback(ctx)
	}()
	var skillID uuid.UUID
	metadata, err := marshalSkillMetadata(req.Metadata, req.RuntimeDependencies)
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, `
INSERT INTO skills (
    tenant_id, slug, name, description, version, source, risk_level,
    icon_key, color_token, tags, metadata, created_by, updated_at,
    archive_object_ref, archive_filename, archive_size_bytes, archive_checksum_sha256, archive_file_count
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW(),$13,$14,$15,$16,$17)
ON CONFLICT (tenant_id, slug) WHERE deleted_at IS NULL
DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    version = EXCLUDED.version,
    source = EXCLUDED.source,
    risk_level = EXCLUDED.risk_level,
    icon_key = EXCLUDED.icon_key,
    color_token = EXCLUDED.color_token,
    tags = EXCLUDED.tags,
    metadata = COALESCE(skills.metadata, '{}'::jsonb) || EXCLUDED.metadata,
    archive_object_ref = EXCLUDED.archive_object_ref,
    archive_filename = EXCLUDED.archive_filename,
    archive_size_bytes = EXCLUDED.archive_size_bytes,
    archive_checksum_sha256 = EXCLUDED.archive_checksum_sha256,
    archive_file_count = EXCLUDED.archive_file_count,
    updated_at = NOW()
RETURNING id`,
		req.TenantID, req.Slug, req.Name, req.Description, req.Version, req.Source, req.RiskLevel,
		req.IconKey, req.ColorToken, req.Tags, metadata, nullUUID(req.ActorUserID),
		req.ArchiveObjectRef, req.ArchiveFilename, req.ArchiveSizeBytes, req.ArchiveChecksum, req.ArchiveFileCount,
	).Scan(&skillID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM team_skill_bindings WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, skillID); err != nil {
		return nil, err
	}
	for _, teamID := range req.TeamIDs {
		if _, err = tx.Exec(ctx, `
INSERT INTO team_skill_bindings (tenant_id, skill_id, team_id)
VALUES ($1,$2,$3)
ON CONFLICT DO NOTHING`, req.TenantID, skillID, teamID); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	err = nil
	return r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: skillID})
}

func (r *PgRepository) DeleteSkill(ctx context.Context, req DeleteSkillRequest) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// Roll back unconditionally on any early return or panic; it is a no-op
		// once Commit succeeds, so a leaked open tx can never strand a pooled conn.
		_ = tx.Rollback(ctx)
	}()
	if _, err = tx.Exec(ctx, `DELETE FROM team_skill_bindings WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, req.SkillID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM skill_agent_bindings WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, req.SkillID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM project_skill_bindings WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, req.SkillID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE skills SET deleted_at = NOW() WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`, req.TenantID, req.SkillID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		err = ErrNotFound
		return err
	}
	return tx.Commit(ctx)
}

func (r *PgRepository) DeleteSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) error {
	if r == nil || r.q == nil {
		return fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	return r.q.DeleteSkillMCPDependenciesForSkill(ctx, queries.DeleteSkillMCPDependenciesForSkillParams{
		TenantID: tenantID,
		SkillID:  skillID,
	})
}

func (r *PgRepository) IsSkillBoundToEmployeeTeam(ctx context.Context, req BindEmployeeSkillRequest) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	var exists bool
	err := r.db.QueryRow(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM team_skill_bindings stb
    JOIN digital_employees de ON de.tenant_id = stb.tenant_id AND de.team_id = stb.team_id
    WHERE stb.tenant_id = $1
      AND stb.skill_id = $2
      AND de.id = $3
      AND de.deleted_at IS NULL
)`, req.TenantID, req.SkillID, req.DigitalEmployeeID).Scan(&exists)
	return exists, err
}

func (r *PgRepository) BindSkillToTeam(ctx context.Context, req BindTeamSkillRequest) (*Skill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	if _, err := r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID}); err != nil {
		return nil, err
	}
	if err := r.ensureTeamExists(ctx, req.TenantID, req.TeamID); err != nil {
		return nil, err
	}
	tag, err := r.db.Exec(ctx, `
INSERT INTO team_skill_bindings (tenant_id, skill_id, team_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING`, req.TenantID, req.SkillID, req.TeamID)
	if err != nil {
		return nil, err
	}
	item, err := r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
	if err != nil {
		return nil, err
	}
	// 只有真插入了才记审计与接管：重复绑定是幂等 no-op，不该在团队审计流里刷屏。
	if tag.RowsAffected() > 0 {
		if err := r.writeTeamCapabilityAudit(ctx, req, "team.skill.bind", item); err != nil {
			return nil, err
		}
		// 团队接管：同一技能只留一份，成员各自的个人绑定就地物理收敛。
		// 不做只留读时屏蔽——那会留下"个人技能列表看得见、生效列表看不见"的幽灵行。
		takenOver, err := r.takeoverTeamSkill(ctx, req)
		if err != nil {
			return nil, err
		}
		if len(takenOver) > 0 {
			if err := r.writeTeamSkillTakeoverAudit(ctx, req, item, takenOver); err != nil {
				return nil, err
			}
		}
	}
	return item, nil
}

// ListTeamSkillTakeoverTargets 列出本团队成员里已自行安装该技能的个人绑定。
// 预览与执行共用这一条，保证"看到的"和"接管的"是同一批。
func (r *PgRepository) ListTeamSkillTakeoverTargets(ctx context.Context, req BindTeamSkillRequest) ([]TeamSkillTakeover, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	rows, err := r.db.Query(ctx, `
SELECT de.id, de.name
FROM digital_employees de
JOIN skill_agent_bindings sab
  ON sab.tenant_id = de.tenant_id
 AND sab.digital_employee_id = de.id
 AND sab.status = 'enabled'
WHERE de.tenant_id = $1
  AND de.team_id = $2
  AND de.deleted_at IS NULL
  AND sab.skill_id = $3
ORDER BY de.name ASC`, req.TenantID, req.TeamID, req.SkillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	targets := make([]TeamSkillTakeover, 0)
	for rows.Next() {
		var item TeamSkillTakeover
		if err := rows.Scan(&item.DigitalEmployeeID, &item.EmployeeName); err != nil {
			return nil, err
		}
		targets = append(targets, item)
	}
	return targets, rows.Err()
}

func (r *PgRepository) takeoverTeamSkill(ctx context.Context, req BindTeamSkillRequest) ([]TeamSkillTakeover, error) {
	targets, err := r.ListTeamSkillTakeoverTargets(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}
	if _, err := r.db.Exec(ctx, `
DELETE FROM skill_agent_bindings sab
USING digital_employees de
WHERE de.id = sab.digital_employee_id
  AND de.tenant_id = sab.tenant_id
  AND de.team_id = $2
  AND de.deleted_at IS NULL
  AND sab.tenant_id = $1
  AND sab.skill_id = $3`, req.TenantID, req.TeamID, req.SkillID); err != nil {
		return nil, err
	}
	return targets, nil
}

// TeamProvidesSkill 员工侧安装前的冲突判据：该员工所属团队是否已提供同一技能。
func (r *PgRepository) TeamProvidesSkill(ctx context.Context, tenantID, employeeID, skillID uuid.UUID) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	var provides bool
	if err := r.db.QueryRow(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM digital_employees de
    JOIN team_skill_bindings tsb
      ON tsb.tenant_id = de.tenant_id
     AND tsb.team_id = de.team_id
    WHERE de.tenant_id = $1
      AND de.id = $2
      AND de.deleted_at IS NULL
      AND tsb.skill_id = $3
)`, tenantID, employeeID, skillID).Scan(&provides); err != nil {
		return false, err
	}
	return provides, nil
}

func (r *PgRepository) writeTeamSkillTakeoverAudit(ctx context.Context, req BindTeamSkillRequest, item *Skill, targets []TeamSkillTakeover) error {
	employees := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		employees = append(employees, map[string]any{
			"digital_employee_id": target.DigitalEmployeeID.String(),
			"employee_name":       target.EmployeeName,
		})
	}
	details := map[string]any{
		"team_id":             req.TeamID.String(),
		"skill_id":            req.SkillID.String(),
		"converged_count":     len(targets),
		"converged_employees": employees,
	}
	if item != nil {
		details["skill_slug"] = item.Slug
		details["skill_name"] = item.Name
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = r.q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: req.TenantID, Valid: req.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      req.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: req.TeamID.String(), Valid: true},
		Action:       "team.skill.takeover",
		Details:      payload,
	})
	return err
}

func (r *PgRepository) UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	// 先取技能（用于审计详情里的 slug/name），取不到就退化为只记 id。
	item, _ := r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
	tag, err := r.db.Exec(ctx, `
DELETE FROM team_skill_bindings
WHERE tenant_id = $1 AND skill_id = $2 AND team_id = $3`, req.TenantID, req.SkillID, req.TeamID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil
	}
	return r.writeTeamCapabilityAudit(ctx, req, "team.skill.unbind", item)
}

// writeTeamCapabilityAudit 落一条 resource_type=team 的团队审计事件。资源维度必须是
// team：团队审计流(GET /teams/{id}/audit)按 resource_type='team' 过滤，落在 skill 上
// 等于在团队视角不可见。
func (r *PgRepository) writeTeamCapabilityAudit(ctx context.Context, req BindTeamSkillRequest, action string, item *Skill) error {
	details := map[string]any{
		"team_id":  req.TeamID.String(),
		"skill_id": req.SkillID.String(),
	}
	if item != nil {
		details["skill_slug"] = item.Slug
		details["skill_name"] = item.Name
		details["skill_version"] = item.Version
		details["risk_level"] = item.RiskLevel
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = r.q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: req.TenantID, Valid: req.TenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      req.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: req.TeamID.String(), Valid: true},
		Action:       action,
		Details:      payload,
	})
	return err
}

func (r *PgRepository) ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	rows, err := r.db.Query(ctx, `
SELECT `+skillSelectColumns+`
FROM team_skill_bindings stb
JOIN skills s ON s.tenant_id = stb.tenant_id
    AND s.id = stb.skill_id
    AND s.deleted_at IS NULL
`+skillJoinClause+`
JOIN tenant_teams tt ON tt.tenant_id = stb.tenant_id
    AND tt.id = stb.team_id
    AND tt.deleted_at IS NULL
WHERE stb.tenant_id = $1 AND stb.team_id = $2
ORDER BY s.name ASC, s.updated_at DESC`, req.TenantID, req.TeamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var skills []*Skill
	for rows.Next() {
		item, err := scanSkill(rows)
		if err != nil {
			return nil, err
		}
		skills = append(skills, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, item := range skills {
		if err := r.loadChildren(ctx, item); err != nil {
			return nil, err
		}
	}
	return skills, nil
}

func (r *PgRepository) BindSkillToEmployee(ctx context.Context, req BindEmployeeSkillRequest) (*Skill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	if _, err := r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID}); err != nil {
		return nil, err
	}
	if err := r.ensureEmployeeExists(ctx, req.TenantID, req.DigitalEmployeeID); err != nil {
		return nil, err
	}
	if _, err := r.db.Exec(ctx, `
INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status)
VALUES ($1, $2, $3, 'enabled')
ON CONFLICT (tenant_id, skill_id, digital_employee_id)
DO UPDATE SET status = 'enabled', updated_at = NOW()`, req.TenantID, req.SkillID, req.DigitalEmployeeID); err != nil {
		return nil, err
	}
	return r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
}

func (r *PgRepository) UnbindSkillFromEmployee(ctx context.Context, req BindEmployeeSkillRequest) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	_, err := r.db.Exec(ctx, `
DELETE FROM skill_agent_bindings
WHERE tenant_id = $1 AND skill_id = $2 AND digital_employee_id = $3`, req.TenantID, req.SkillID, req.DigitalEmployeeID)
	return err
}

func (r *PgRepository) ListEffectiveEmployeeSkills(ctx context.Context, req ListEffectiveEmployeeSkillsRequest) ([]EffectiveEmployeeSkill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	rows, err := r.db.Query(ctx, `
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = $1
      AND id = $2
      AND deleted_at IS NULL
)
SELECT
    s.id, s.tenant_id, s.slug, s.name, s.description, s.version, s.source, s.risk_level,
    s.icon_key, s.color_token, s.tags, COALESCE(s.metadata, '{}'::jsonb) AS metadata, s.archive_object_ref, s.archive_filename,
    s.archive_size_bytes, s.archive_checksum_sha256, s.archive_file_count,
    COALESCE(s.created_by::text, '') AS created_by,
    COALESCE(au.display_name, au.username, '') AS created_by_name,
    s.created_at, s.updated_at,
    'team'::text AS source_scope, true AS inherited, true AS read_only
FROM target_employee
JOIN team_skill_bindings stb ON stb.tenant_id = target_employee.tenant_id
    AND stb.team_id = target_employee.team_id
JOIN skills s ON s.tenant_id = stb.tenant_id
    AND s.id = stb.skill_id
    AND s.deleted_at IS NULL
LEFT JOIN auth_users au ON au.id = s.created_by
UNION ALL
SELECT
    s.id, s.tenant_id, s.slug, s.name, s.description, s.version, s.source, s.risk_level,
    s.icon_key, s.color_token, s.tags, COALESCE(s.metadata, '{}'::jsonb) AS metadata, s.archive_object_ref, s.archive_filename,
    s.archive_size_bytes, s.archive_checksum_sha256, s.archive_file_count,
    COALESCE(s.created_by::text, '') AS created_by,
    COALESCE(au.display_name, au.username, '') AS created_by_name,
    s.created_at, s.updated_at,
    'employee'::text AS source_scope, false AS inherited, false AS read_only
FROM target_employee
JOIN skill_agent_bindings sab ON sab.tenant_id = target_employee.tenant_id
    AND sab.digital_employee_id = target_employee.digital_employee_id
    AND sab.status = 'enabled'
JOIN skills s ON s.tenant_id = sab.tenant_id
    AND s.id = sab.skill_id
    AND s.deleted_at IS NULL
LEFT JOIN auth_users au ON au.id = s.created_by
WHERE NOT EXISTS (
    SELECT 1
    FROM team_skill_bindings inherited_binding
    WHERE inherited_binding.tenant_id = target_employee.tenant_id
      AND inherited_binding.team_id = target_employee.team_id
      AND inherited_binding.skill_id = sab.skill_id
)
ORDER BY inherited DESC, name ASC`, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var skills []EffectiveEmployeeSkill
	for rows.Next() {
		item := EffectiveEmployeeSkill{}
		var createdByStr string
		var metadataBytes []byte
		if err := rows.Scan(
			&item.Skill.ID,
			&item.Skill.TenantID,
			&item.Skill.Slug,
			&item.Skill.Name,
			&item.Skill.Description,
			&item.Skill.Version,
			&item.Skill.Source,
			&item.Skill.RiskLevel,
			&item.Skill.IconKey,
			&item.Skill.ColorToken,
			&item.Skill.Tags,
			&metadataBytes,
			&item.Skill.ArchiveObjectRef,
			&item.Skill.ArchiveFilename,
			&item.Skill.ArchiveSizeBytes,
			&item.Skill.ArchiveChecksum,
			&item.Skill.ArchiveFileCount,
			&createdByStr,
			&item.Skill.CreatedByName,
			&item.Skill.CreatedAt,
			&item.Skill.UpdatedAt,
			&item.SourceScope,
			&item.Inherited,
			&item.ReadOnly,
		); err != nil {
			return nil, err
		}
		if createdByStr != "" {
			item.Skill.CreatedBy, _ = uuid.Parse(createdByStr)
		}
		if err := applySkillMetadata(&item.Skill, metadataBytes); err != nil {
			return nil, err
		}
		if err := r.loadChildren(ctx, &item.Skill); err != nil {
			return nil, err
		}
		skills = append(skills, item)
	}
	return skills, rows.Err()
}

func (r *PgRepository) ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, projectID *uuid.UUID) (RuntimeSkillsResult, error) {
	if r == nil || r.db == nil {
		return RuntimeSkillsResult{}, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	// Employee-carried skills (team ∪ personal). Source scopes preserved for conflict tracing.
	rows, err := r.db.Query(ctx, `
WITH target_employee AS (
    SELECT tenant_id, id AS digital_employee_id, team_id
    FROM digital_employees
    WHERE tenant_id = $1
      AND id = $2
      AND deleted_at IS NULL
)
SELECT
    s.id,
    s.slug,
    s.version,
    COALESCE(s.metadata, '{}'::jsonb) AS metadata,
    s.archive_object_ref,
    s.archive_checksum_sha256,
    s.archive_size_bytes,
    s.archive_file_count,
    'team'::text AS source_scope
FROM target_employee
JOIN team_skill_bindings stb ON stb.tenant_id = target_employee.tenant_id
    AND stb.team_id = target_employee.team_id
JOIN skills s ON s.tenant_id = stb.tenant_id
    AND s.id = stb.skill_id
    AND s.deleted_at IS NULL
    AND s.archive_object_ref IS NOT NULL
    AND s.archive_object_ref <> ''
UNION ALL
SELECT
    s.id,
    s.slug,
    s.version,
    COALESCE(s.metadata, '{}'::jsonb) AS metadata,
    s.archive_object_ref,
    s.archive_checksum_sha256,
    s.archive_size_bytes,
    s.archive_file_count,
    'employee'::text AS source_scope
FROM target_employee
JOIN skill_agent_bindings sab ON sab.tenant_id = target_employee.tenant_id
    AND sab.digital_employee_id = target_employee.digital_employee_id
    AND sab.status = 'enabled'
JOIN skills s ON s.tenant_id = sab.tenant_id
    AND s.id = sab.skill_id
    AND s.deleted_at IS NULL
    AND s.archive_object_ref IS NOT NULL
    AND s.archive_object_ref <> ''
WHERE NOT EXISTS (
    SELECT 1 FROM team_skill_bindings inherited_binding
    WHERE inherited_binding.tenant_id = target_employee.tenant_id
      AND inherited_binding.team_id = target_employee.team_id
      AND inherited_binding.skill_id = sab.skill_id
)
ORDER BY slug ASC`, tenantID, digitalEmployeeID)
	if err != nil {
		return RuntimeSkillsResult{}, err
	}
	defer rows.Close()
	employeeSide := make([]SkillRuntimeRecord, 0)
	for rows.Next() {
		rec, scanErr := scanSkillRuntimeRecord(rows)
		if scanErr != nil {
			return RuntimeSkillsResult{}, scanErr
		}
		employeeSide = append(employeeSide, rec)
	}
	if err := rows.Err(); err != nil {
		return RuntimeSkillsResult{}, err
	}

	if projectID == nil || *projectID == uuid.Nil {
		// No project context: keep prior behavior (employee-carried only, dedupe by id).
		return RuntimeSkillsResult{Skills: dedupeSkillsByID(employeeSide)}, nil
	}

	projectSide, err := r.listProjectBoundSkillsForRuntime(ctx, tenantID, *projectID)
	if err != nil {
		return RuntimeSkillsResult{}, err
	}

	// Venue filter (§4.2 ①): drop employee-carried skills that are bound to other projects only.
	skillIDs := make([]uuid.UUID, 0, len(employeeSide)+len(projectSide))
	for _, rec := range employeeSide {
		skillIDs = append(skillIDs, rec.ID)
	}
	for _, rec := range projectSide {
		skillIDs = append(skillIDs, rec.ID)
	}
	bindingsBySkill, err := r.ListSkillIDsWithAnyProjectBinding(ctx, tenantID, skillIDs)
	if err != nil {
		return RuntimeSkillsResult{}, err
	}

	filteredEmployee := make([]SkillRuntimeRecord, 0, len(employeeSide))
	for _, rec := range employeeSide {
		boundProjects := bindingsBySkill[rec.ID]
		if len(boundProjects) == 0 {
			// universal skill
			filteredEmployee = append(filteredEmployee, rec)
			continue
		}
		allowed := false
		for _, pid := range boundProjects {
			if pid == *projectID {
				allowed = true
				break
			}
		}
		if allowed {
			filteredEmployee = append(filteredEmployee, rec)
		}
	}

	// Union project supply (§4.2 ②) with filtered employee-carried; project wins on slug conflict.
	return mergeRuntimeSkills(filteredEmployee, projectSide), nil
}

func scanSkillRuntimeRecord(rows interface {
	Scan(dest ...any) error
}) (SkillRuntimeRecord, error) {
	var rec SkillRuntimeRecord
	var metadataBytes []byte
	if err := rows.Scan(
		&rec.ID,
		&rec.Slug,
		&rec.Version,
		&metadataBytes,
		&rec.ArchiveObjectRef,
		&rec.ArchiveChecksum,
		&rec.ArchiveSizeBytes,
		&rec.ArchiveFileCount,
		&rec.SourceScope,
	); err != nil {
		return SkillRuntimeRecord{}, err
	}
	if err := applyRuntimeRecordMetadata(&rec, metadataBytes); err != nil {
		return SkillRuntimeRecord{}, err
	}
	return rec, nil
}

func dedupeSkillsByID(records []SkillRuntimeRecord) []SkillRuntimeRecord {
	seen := make(map[uuid.UUID]struct{}, len(records))
	out := make([]SkillRuntimeRecord, 0, len(records))
	for _, rec := range records {
		if _, ok := seen[rec.ID]; ok {
			continue
		}
		seen[rec.ID] = struct{}{}
		out = append(out, rec)
	}
	return out
}

// mergeRuntimeSkills unions employee-side and project-side skills. Same slug prefers project
// (source_scope=project) and records a conflict with source=project_binding.
func mergeRuntimeSkills(employeeSide, projectSide []SkillRuntimeRecord) RuntimeSkillsResult {
	type ranked struct {
		rec      SkillRuntimeRecord
		priority int // lower wins: project=0, team=1, employee=2
	}
	priorityOf := func(scope string) int {
		switch scope {
		case "project":
			return 0
		case "team":
			return 1
		default:
			return 2
		}
	}
	bySlug := map[string]ranked{}
	conflicts := make([]SkillRuntimeConflict, 0)
	order := make([]string, 0)

	consider := func(rec SkillRuntimeRecord) {
		slug := rec.Slug
		p := priorityOf(rec.SourceScope)
		existing, ok := bySlug[slug]
		if !ok {
			bySlug[slug] = ranked{rec: rec, priority: p}
			order = append(order, slug)
			return
		}
		if existing.rec.ID == rec.ID {
			// same skill from multiple sources; keep higher-priority scope label
			if p < existing.priority {
				bySlug[slug] = ranked{rec: rec, priority: p}
			}
			return
		}
		// different skill ids, same slug → conflict
		if p < existing.priority {
			conflicts = append(conflicts, SkillRuntimeConflict{
				Slug:           slug,
				WinningSkillID: rec.ID,
				DroppedSkillID: existing.rec.ID,
				WinningSource:  rec.SourceScope,
				DroppedSource:  existing.rec.SourceScope,
				Source:         conflictSourceMarker(rec.SourceScope),
			})
			bySlug[slug] = ranked{rec: rec, priority: p}
			return
		}
		if p > existing.priority {
			conflicts = append(conflicts, SkillRuntimeConflict{
				Slug:           slug,
				WinningSkillID: existing.rec.ID,
				DroppedSkillID: rec.ID,
				WinningSource:  existing.rec.SourceScope,
				DroppedSource:  rec.SourceScope,
				Source:         conflictSourceMarker(existing.rec.SourceScope),
			})
			return
		}
		// equal priority: keep first
	}

	for _, rec := range employeeSide {
		consider(rec)
	}
	for _, rec := range projectSide {
		consider(rec)
	}

	skills := make([]SkillRuntimeRecord, 0, len(order))
	for _, slug := range order {
		skills = append(skills, bySlug[slug].rec)
	}
	return RuntimeSkillsResult{Skills: skills, Conflicts: conflicts}
}

func conflictSourceMarker(winningScope string) string {
	if winningScope == "project" {
		return "project_binding"
	}
	return winningScope
}

func (r *PgRepository) listProjectBoundSkillsForRuntime(ctx context.Context, tenantID, projectID uuid.UUID) ([]SkillRuntimeRecord, error) {
	rows, err := r.db.Query(ctx, `
SELECT
    s.id,
    s.slug,
    s.version,
    COALESCE(s.metadata, '{}'::jsonb) AS metadata,
    s.archive_object_ref,
    s.archive_checksum_sha256,
    s.archive_size_bytes,
    s.archive_file_count,
    'project'::text AS source_scope
FROM project_skill_bindings psb
JOIN skills s ON s.tenant_id = psb.tenant_id
    AND s.id = psb.skill_id
    AND s.deleted_at IS NULL
    AND s.archive_object_ref IS NOT NULL
    AND s.archive_object_ref <> ''
WHERE psb.tenant_id = $1
  AND psb.project_id = $2
ORDER BY s.slug ASC`, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SkillRuntimeRecord, 0)
	for rows.Next() {
		rec, scanErr := scanSkillRuntimeRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (r *PgRepository) ListSkillIDsWithAnyProjectBinding(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	result := make(map[uuid.UUID][]uuid.UUID)
	if len(skillIDs) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `
SELECT skill_id, project_id
FROM project_skill_bindings
WHERE tenant_id = $1
  AND skill_id = ANY($2::uuid[])`, tenantID, skillIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var skillID, projectID uuid.UUID
		if err := rows.Scan(&skillID, &projectID); err != nil {
			return nil, err
		}
		result[skillID] = append(result[skillID], projectID)
	}
	return result, rows.Err()
}

func (r *PgRepository) ListProjectSkillBindings(ctx context.Context, req ListProjectSkillBindingsRequest) ([]ProjectSkillBinding, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	rows, err := r.db.Query(ctx, `
SELECT
    psb.id, psb.tenant_id, psb.project_id, psb.skill_id, psb.created_by_user_id, psb.created_at,
    s.id, s.tenant_id, s.slug, s.name, s.description, s.version, s.source, s.risk_level,
    s.icon_key, s.color_token, s.tags, COALESCE(s.metadata, '{}'::jsonb), s.archive_object_ref, s.archive_filename,
    s.archive_size_bytes, s.archive_checksum_sha256, s.archive_file_count,
    COALESCE(s.created_by::text, ''), s.created_at, s.updated_at
FROM project_skill_bindings psb
JOIN skills s ON s.tenant_id = psb.tenant_id AND s.id = psb.skill_id AND s.deleted_at IS NULL
WHERE psb.tenant_id = $1 AND psb.project_id = $2
ORDER BY s.name ASC`, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ProjectSkillBinding, 0)
	for rows.Next() {
		var b ProjectSkillBinding
		var skill Skill
		var createdByUser *uuid.UUID
		var metadataBytes []byte
		var createdByStr string
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.ProjectID, &b.SkillID, &createdByUser, &b.CreatedAt,
			&skill.ID, &skill.TenantID, &skill.Slug, &skill.Name, &skill.Description, &skill.Version, &skill.Source, &skill.RiskLevel,
			&skill.IconKey, &skill.ColorToken, &skill.Tags, &metadataBytes, &skill.ArchiveObjectRef, &skill.ArchiveFilename,
			&skill.ArchiveSizeBytes, &skill.ArchiveChecksum, &skill.ArchiveFileCount,
			&createdByStr, &skill.CreatedAt, &skill.UpdatedAt,
		); err != nil {
			return nil, err
		}
		b.CreatedByUserID = createdByUser
		if createdByStr != "" {
			skill.CreatedBy, _ = uuid.Parse(createdByStr)
		}
		if err := applySkillMetadata(&skill, metadataBytes); err != nil {
			return nil, err
		}
		b.Skill = &skill
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *PgRepository) ReplaceProjectSkillBindings(ctx context.Context, req PutProjectSkillBindingsRequest) ([]ProjectSkillBinding, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
DELETE FROM project_skill_bindings
WHERE tenant_id = $1 AND project_id = $2`, req.TenantID, req.ProjectID); err != nil {
		return nil, err
	}
	for _, item := range req.Items {
		if _, err := tx.Exec(ctx, `
INSERT INTO project_skill_bindings (tenant_id, project_id, skill_id, created_by_user_id)
VALUES ($1, $2, $3, $4)`, req.TenantID, req.ProjectID, item.SkillID, req.UserID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.ListProjectSkillBindings(ctx, ListProjectSkillBindingsRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
	})
}

func (r *PgRepository) ensureTeamExists(ctx context.Context, tenantID, teamID uuid.UUID) error {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
SELECT id
FROM tenant_teams
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`, tenantID, teamID).Scan(&id)
	return mapNoRows(err)
}

func (r *PgRepository) ensureEmployeeExists(ctx context.Context, tenantID, employeeID uuid.UUID) error {
	var id uuid.UUID
	err := r.db.QueryRow(ctx, `
SELECT id
FROM digital_employees
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`, tenantID, employeeID).Scan(&id)
	return mapNoRows(err)
}

func (r *PgRepository) loadChildren(ctx context.Context, item *Skill) error {
	teams, err := r.listTeamBindings(ctx, item.TenantID, item.ID)
	if err != nil {
		return err
	}
	item.TeamBindings = teams
	item.TeamIDs = make([]uuid.UUID, 0, len(teams))
	for _, team := range teams {
		item.TeamIDs = append(item.TeamIDs, team.TeamID)
	}
	agents, err := r.listAgentBindings(ctx, item.TenantID, item.ID)
	if err != nil {
		return err
	}
	item.AgentBindings = agents
	projects, err := r.listProjectBindings(ctx, item.TenantID, item.ID)
	if err != nil {
		return err
	}
	item.ProjectBindings = projects
	mcpDeps, err := r.listSkillMCPDependencyRefs(ctx, item.TenantID, item.ID)
	if err != nil {
		return err
	}
	if item.RuntimeDependencies.Tools == nil {
		item.RuntimeDependencies.Tools = []string{}
	}
	if item.RuntimeDependencies.Env == nil {
		item.RuntimeDependencies.Env = []string{}
	}
	item.RuntimeDependencies.MCPServers = mcpDeps
	return nil
}

func (r *PgRepository) listProjectBindings(ctx context.Context, tenantID, skillID uuid.UUID) ([]*SkillProjectBinding, error) {
	rows, err := r.db.Query(ctx, `
SELECT psb.project_id, COALESCE(p.name, '') AS project_name
FROM project_skill_bindings psb
LEFT JOIN projects p ON p.id = psb.project_id AND p.tenant_id = psb.tenant_id AND p.deleted_at IS NULL
WHERE psb.tenant_id = $1 AND psb.skill_id = $2
ORDER BY project_name ASC, psb.project_id ASC`, tenantID, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*SkillProjectBinding, 0)
	for rows.Next() {
		var b SkillProjectBinding
		if err := rows.Scan(&b.ProjectID, &b.ProjectName); err != nil {
			return nil, err
		}
		out = append(out, &b)
	}
	return out, rows.Err()
}

func (r *PgRepository) listSkillMCPDependencyRefs(ctx context.Context, tenantID, skillID uuid.UUID) ([]SkillRuntimeMCPServerRef, error) {
	rows, err := r.db.Query(ctx, `
SELECT d.mcp_server_id, COALESCE(m.server_key, ''), COALESCE(m.name, '')
FROM skill_mcp_dependencies d
LEFT JOIN mcp_servers m ON m.id = d.mcp_server_id AND m.tenant_id = d.tenant_id AND m.deleted_at IS NULL
WHERE d.tenant_id = $1 AND d.skill_id = $2
ORDER BY COALESCE(m.server_key, d.mcp_server_id::text) ASC`, tenantID, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]SkillRuntimeMCPServerRef, 0)
	for rows.Next() {
		var ref SkillRuntimeMCPServerRef
		if err := rows.Scan(&ref.MCPServerID, &ref.ServerKey, &ref.ServerName); err != nil {
			return nil, err
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}


func (r *PgRepository) listTeamBindings(ctx context.Context, tenantID, skillID uuid.UUID) ([]*SkillTeamBinding, error) {
	rows, err := r.db.Query(ctx, `
SELECT stb.team_id, COALESCE(tt.name, '')
FROM team_skill_bindings stb
LEFT JOIN tenant_teams tt ON tt.tenant_id = stb.tenant_id AND tt.id = stb.team_id
WHERE stb.tenant_id = $1 AND stb.skill_id = $2
ORDER BY tt.name ASC NULLS LAST`, tenantID, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bindings []*SkillTeamBinding
	for rows.Next() {
		binding := &SkillTeamBinding{}
		if err := rows.Scan(&binding.TeamID, &binding.TeamName); err != nil {
			return nil, err
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

func (r *PgRepository) listAgentBindings(ctx context.Context, tenantID, skillID uuid.UUID) ([]*SkillAgentBinding, error) {
	rows, err := r.db.Query(ctx, `
SELECT sab.digital_employee_id, de.name, COALESCE(de.team_id::text, ''), COALESCE(tt.name, ''), sab.status
FROM skill_agent_bindings sab
JOIN digital_employees de ON de.tenant_id = sab.tenant_id AND de.id = sab.digital_employee_id
LEFT JOIN tenant_teams tt ON tt.tenant_id = de.tenant_id AND tt.id = de.team_id
WHERE sab.tenant_id = $1 AND sab.skill_id = $2
ORDER BY de.name ASC`, tenantID, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bindings []*SkillAgentBinding
	for rows.Next() {
		var teamIDText string
		binding := &SkillAgentBinding{}
		if err := rows.Scan(&binding.AgentID, &binding.AgentName, &teamIDText, &binding.TeamName, &binding.Status); err != nil {
			return nil, err
		}
		if teamIDText != "" {
			parsed, err := uuid.Parse(teamIDText)
			if err == nil {
				binding.TeamID = &parsed
			}
		}
		bindings = append(bindings, binding)
	}
	return bindings, rows.Err()
}

type skillScanner interface {
	Scan(dest ...any) error
}

func scanSkill(row skillScanner) (*Skill, error) {
	item := &Skill{}
	var createdByStr string
	var metadataBytes []byte
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.Slug,
		&item.Name,
		&item.Description,
		&item.Version,
		&item.Source,
		&item.RiskLevel,
		&item.IconKey,
		&item.ColorToken,
		&item.Tags,
		&metadataBytes,
		&item.ArchiveObjectRef,
		&item.ArchiveFilename,
		&item.ArchiveSizeBytes,
		&item.ArchiveChecksum,
		&item.ArchiveFileCount,
		&createdByStr,
		&item.CreatedByName,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if createdByStr != "" {
		item.CreatedBy, _ = uuid.Parse(createdByStr)
	}
	if err := applySkillMetadata(item, metadataBytes); err != nil {
		return nil, err
	}
	return item, nil
}

const runtimeDependenciesMetadataKey = "runtime_dependencies"

func marshalSkillMetadata(metadata map[string]any, deps SkillRuntimeDependencies) ([]byte, error) {
	merged := make(map[string]any, len(metadata)+1)
	for key, value := range metadata {
		merged[key] = value
	}
	merged[runtimeDependenciesMetadataKey] = deps
	return json.Marshal(merged)
}

func applySkillMetadata(item *Skill, raw []byte) error {
	metadata, deps, err := decodeSkillMetadata(raw)
	if err != nil {
		return err
	}
	item.Metadata = metadata
	item.RuntimeDependencies = deps
	return nil
}

func applyRuntimeRecordMetadata(record *SkillRuntimeRecord, raw []byte) error {
	metadata, deps, err := decodeSkillMetadata(raw)
	if err != nil {
		return err
	}
	record.Metadata = metadata
	record.RuntimeDependencies = deps
	return nil
}

func decodeSkillMetadata(raw []byte) (map[string]any, SkillRuntimeDependencies, error) {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, SkillRuntimeDependencies{}, fmt.Errorf("%w: invalid skill metadata", ErrInvalidInput)
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	deps := SkillRuntimeDependencies{
		Tools: []string{},
		Env:   []string{},
	}
	value, ok := metadata[runtimeDependenciesMetadataKey]
	if !ok || value == nil {
		return metadata, deps, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, SkillRuntimeDependencies{}, fmt.Errorf("%w: invalid skill runtime dependencies", ErrInvalidInput)
	}
	if err := json.Unmarshal(encoded, &deps); err != nil {
		return nil, SkillRuntimeDependencies{}, fmt.Errorf("%w: invalid skill runtime dependencies", ErrInvalidInput)
	}
	normalized, err := normalizeRuntimeDependencies(deps)
	if err != nil {
		return nil, SkillRuntimeDependencies{}, err
	}
	return metadata, normalized, nil
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func textFromPg(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}
