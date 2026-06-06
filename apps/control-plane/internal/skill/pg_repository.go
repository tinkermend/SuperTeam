package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

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

func (r *PgRepository) ListSkills(ctx context.Context, req ListSkillsRequest) ([]*Skill, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	conditions := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{req.TenantID}
	if req.Status != "" {
		args = append(args, string(req.Status))
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if strings.TrimSpace(req.Q) != "" {
		args = append(args, "%"+strings.TrimSpace(req.Q)+"%")
		conditions = append(conditions, fmt.Sprintf("(name ILIKE $%d OR description ILIKE $%d OR slug ILIKE $%d)", len(args), len(args), len(args)))
	}
	rows, err := r.db.Query(ctx, `
SELECT id, tenant_id, slug, name, description, version, source, risk_level, status, icon_key, color_token, tags, created_at, updated_at
FROM skills
WHERE `+strings.Join(conditions, " AND ")+`
ORDER BY updated_at DESC, name ASC`, args...)
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
SELECT id, tenant_id, slug, name, description, version, source, risk_level, status, icon_key, color_token, tags, created_at, updated_at
FROM skills
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL`, req.TenantID, req.SkillID)
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
	err = tx.QueryRow(ctx, `
INSERT INTO skills (
    tenant_id, slug, name, description, version, source, risk_level, status, icon_key, color_token, tags, created_by, updated_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW())
ON CONFLICT (tenant_id, slug) WHERE deleted_at IS NULL
DO UPDATE SET
    name = EXCLUDED.name,
    description = EXCLUDED.description,
    version = EXCLUDED.version,
    source = EXCLUDED.source,
    risk_level = EXCLUDED.risk_level,
    status = EXCLUDED.status,
    icon_key = EXCLUDED.icon_key,
    color_token = EXCLUDED.color_token,
    tags = EXCLUDED.tags,
    updated_at = NOW()
RETURNING id`, req.TenantID, req.Slug, req.Name, req.Description, req.Version, req.Source, req.RiskLevel, string(req.Status), req.IconKey, req.ColorToken, req.Tags, nullUUID(req.ActorUserID)).Scan(&skillID)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM skill_files WHERE tenant_id = $1 AND skill_id = $2`, req.TenantID, skillID); err != nil {
		return nil, err
	}
	for _, file := range req.Files {
		if _, err = tx.Exec(ctx, `
INSERT INTO skill_files (tenant_id, skill_id, path, file_type, content, size_bytes, checksum_sha256)
VALUES ($1,$2,$3,$4,$5,$6,$7)`, req.TenantID, skillID, file.Path, string(file.FileType), file.Content, file.SizeBytes, file.ChecksumSHA256); err != nil {
			return nil, err
		}
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

func (r *PgRepository) UpdateSkillFile(ctx context.Context, req UpdateSkillFileRequest) (*SkillFile, error) {
	sum := sha256.Sum256([]byte(req.Content))
	row := r.db.QueryRow(ctx, `
UPDATE skill_files
SET content = $1,
    size_bytes = $2,
    checksum_sha256 = $3,
    updated_at = NOW()
WHERE tenant_id = $4 AND skill_id = $5 AND path = $6
RETURNING id, tenant_id, skill_id, path, file_type, content, size_bytes, checksum_sha256, created_at, updated_at`,
		req.Content, len([]byte(req.Content)), hex.EncodeToString(sum[:]), req.TenantID, req.SkillID, req.Path)
	file, err := scanSkillFile(row)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return file, nil
}

func (r *PgRepository) loadChildren(ctx context.Context, item *Skill) error {
	files, err := r.listFiles(ctx, item.TenantID, item.ID)
	if err != nil {
		return err
	}
	item.Files = files
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

func (r *PgRepository) listFiles(ctx context.Context, tenantID, skillID uuid.UUID) ([]*SkillFile, error) {
	rows, err := r.db.Query(ctx, `
SELECT id, tenant_id, skill_id, path, file_type, content, size_bytes, checksum_sha256, created_at, updated_at
FROM skill_files
WHERE tenant_id = $1 AND skill_id = $2
ORDER BY CASE WHEN path = 'SKILL.md' THEN 0 ELSE 1 END, path ASC`, tenantID, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []*SkillFile
	for rows.Next() {
		file, err := scanSkillFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
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
	if err := row.Scan(
		&item.ID,
		&item.TenantID,
		&item.Slug,
		&item.Name,
		&item.Description,
		&item.Version,
		&item.Source,
		&item.RiskLevel,
		&item.Status,
		&item.IconKey,
		&item.ColorToken,
		&item.Tags,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return item, nil
}

func scanSkillFile(row skillScanner) (*SkillFile, error) {
	file := &SkillFile{}
	if err := row.Scan(
		&file.ID,
		&file.TenantID,
		&file.SkillID,
		&file.Path,
		&file.FileType,
		&file.Content,
		&file.SizeBytes,
		&file.ChecksumSHA256,
		&file.CreatedAt,
		&file.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return file, nil
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

var _ Repository = (*PgRepository)(nil)

var _ = time.Time{}
