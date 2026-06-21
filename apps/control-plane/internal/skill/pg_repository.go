package skill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PgRepository struct {
	db *pgxpool.Pool
}

func NewPgRepository(db *pgxpool.Pool) Repository {
	return &PgRepository{db: db}
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
		if err != nil {
			_ = tx.Rollback(ctx)
		}
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
	if _, err = tx.Exec(ctx, `DELETE FROM skill_team_bindings WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, skillID); err != nil {
		return nil, err
	}
	for _, teamID := range req.TeamIDs {
		if _, err = tx.Exec(ctx, `
INSERT INTO skill_team_bindings (tenant_id, skill_id, team_id)
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
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if _, err = tx.Exec(ctx, `DELETE FROM skill_team_bindings WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, req.SkillID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM skill_agent_bindings WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, req.SkillID); err != nil {
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

func (r *PgRepository) IsSkillBoundToEmployeeTeam(ctx context.Context, req BindEmployeeSkillRequest) (bool, error) {
	if r == nil || r.db == nil {
		return false, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	var exists bool
	err := r.db.QueryRow(ctx, `
SELECT EXISTS(
    SELECT 1
    FROM skill_team_bindings stb
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
	if _, err := r.db.Exec(ctx, `
INSERT INTO skill_team_bindings (tenant_id, skill_id, team_id)
VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING`, req.TenantID, req.SkillID, req.TeamID); err != nil {
		return nil, err
	}
	return r.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
}

func (r *PgRepository) UnbindSkillFromTeam(ctx context.Context, req BindTeamSkillRequest) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	_, err := r.db.Exec(ctx, `
DELETE FROM skill_team_bindings
WHERE tenant_id = $1 AND skill_id = $2 AND team_id = $3`, req.TenantID, req.SkillID, req.TeamID)
	return err
}

func (r *PgRepository) ListTeamSkills(ctx context.Context, req ListTeamSkillsRequest) ([]*Skill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	rows, err := r.db.Query(ctx, `
SELECT `+skillSelectColumns+`
FROM skill_team_bindings stb
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
JOIN skill_team_bindings stb ON stb.tenant_id = target_employee.tenant_id
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
    FROM skill_team_bindings inherited_binding
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

func (r *PgRepository) ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]SkillRuntimeRecord, error) {
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
    s.id,
    s.slug,
    COALESCE(s.metadata, '{}'::jsonb) AS metadata,
    s.archive_object_ref,
    s.archive_checksum_sha256,
    s.archive_size_bytes,
    s.archive_file_count
FROM target_employee
JOIN skills s ON s.tenant_id = target_employee.tenant_id
    AND s.deleted_at IS NULL
    AND s.archive_object_ref IS NOT NULL
    AND s.archive_object_ref <> ''
WHERE EXISTS (
    SELECT 1 FROM skill_team_bindings stb
    WHERE stb.tenant_id = target_employee.tenant_id
      AND stb.team_id = target_employee.team_id
      AND stb.skill_id = s.id
) OR EXISTS (
    SELECT 1 FROM skill_agent_bindings sab
    WHERE sab.tenant_id = target_employee.tenant_id
      AND sab.digital_employee_id = target_employee.digital_employee_id
      AND sab.skill_id = s.id
      AND sab.status = 'enabled'
)
ORDER BY s.slug ASC`, tenantID, digitalEmployeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []SkillRuntimeRecord
	for rows.Next() {
		var rec SkillRuntimeRecord
		var metadataBytes []byte
		if err := rows.Scan(&rec.ID, &rec.Slug, &metadataBytes, &rec.ArchiveObjectRef, &rec.ArchiveChecksum, &rec.ArchiveSizeBytes, &rec.ArchiveFileCount); err != nil {
			return nil, err
		}
		if err := applyRuntimeRecordMetadata(&rec, metadataBytes); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}
	return records, rows.Err()
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
	return nil
}

func (r *PgRepository) listTeamBindings(ctx context.Context, tenantID, skillID uuid.UUID) ([]*SkillTeamBinding, error) {
	rows, err := r.db.Query(ctx, `
SELECT stb.team_id, COALESCE(tt.name, '')
FROM skill_team_bindings stb
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

func nullUUID(value uuid.UUID) any {
	if value == uuid.Nil {
		return nil
	}
	return value
}
