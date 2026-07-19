-- name: ListSkillMCPDependencies :many
SELECT d.id, d.tenant_id, d.skill_id, d.mcp_server_id, d.note, d.created_at,
       m.server_key, m.name AS server_name, m.auth_strategy, m.risk_level
FROM skill_mcp_dependencies d
JOIN mcp_servers m ON m.id = d.mcp_server_id AND m.deleted_at IS NULL AND m.tenant_id = d.tenant_id
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.skill_id = sqlc.arg('skill_id')::uuid
ORDER BY m.server_key ASC;

-- name: DeleteSkillMCPDependenciesForSkill :exec
DELETE FROM skill_mcp_dependencies
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND skill_id = sqlc.arg('skill_id')::uuid;

-- name: InsertSkillMCPDependency :exec
INSERT INTO skill_mcp_dependencies (tenant_id, skill_id, mcp_server_id, note)
VALUES (sqlc.arg('tenant_id')::uuid, sqlc.arg('skill_id')::uuid,
        sqlc.arg('mcp_server_id')::uuid, sqlc.arg('note')::text);

-- name: ListDependentSkillsForMCPServer :many
SELECT d.skill_id, s.slug, s.name
FROM skill_mcp_dependencies d
JOIN skills s ON s.id = d.skill_id AND s.deleted_at IS NULL AND s.tenant_id = d.tenant_id
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.mcp_server_id = sqlc.arg('mcp_server_id')::uuid
ORDER BY s.slug ASC;

-- name: ListSkillMCPDependenciesForSkills :many
SELECT d.id, d.tenant_id, d.skill_id, d.mcp_server_id, d.note, d.created_at,
       m.server_key, m.name AS server_name, m.auth_strategy, m.risk_level
FROM skill_mcp_dependencies d
JOIN mcp_servers m ON m.id = d.mcp_server_id AND m.deleted_at IS NULL AND m.tenant_id = d.tenant_id
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.skill_id = ANY(sqlc.arg('skill_ids')::uuid[])
ORDER BY d.skill_id, m.server_key ASC;

-- name: SkillExistsForTenant :one
SELECT EXISTS(
    SELECT 1 FROM skills
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND id = sqlc.arg('id')::uuid
      AND deleted_at IS NULL
) AS exists;
