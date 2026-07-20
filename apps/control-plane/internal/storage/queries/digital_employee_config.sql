-- name: CreateDigitalEmployeeConfigRevision :one
INSERT INTO digital_employee_config_revisions (
    tenant_id,
    digital_employee_id,
    revision_number,
    persona_memory_markdown,
    capability_bindings,
    budget_policy,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('revision_number')::integer,
    COALESCE(sqlc.arg('persona_memory_markdown')::text, ''),
    COALESCE(sqlc.arg('capability_bindings')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('budget_policy')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING id,
    tenant_id,
    digital_employee_id,
    revision_number,
    persona_memory_markdown,
    capability_bindings,
    budget_policy,
    status,
    approved_by,
    approved_at,
    archived_at,
    created_at,
    updated_at;

-- name: GetLatestDigitalEmployeeConfigRevision :one
SELECT id,
    tenant_id,
    digital_employee_id,
    revision_number,
    persona_memory_markdown,
    capability_bindings,
    budget_policy,
    status,
    approved_by,
    approved_at,
    archived_at,
    created_at,
    updated_at
FROM digital_employee_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY revision_number DESC
LIMIT 1;

-- name: GetCurrentDigitalEmployeeConfigRevision :one
SELECT id,
    tenant_id,
    digital_employee_id,
    revision_number,
    persona_memory_markdown,
    capability_bindings,
    budget_policy,
    status,
    approved_by,
    approved_at,
    archived_at,
    created_at,
    updated_at
FROM digital_employee_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND status = 'active'
  AND archived_at IS NULL
ORDER BY revision_number DESC
LIMIT 1;

-- name: GetDigitalEmployeeConfigRevision :one
SELECT id,
    tenant_id,
    digital_employee_id,
    revision_number,
    persona_memory_markdown,
    capability_bindings,
    budget_policy,
    status,
    approved_by,
    approved_at,
    archived_at,
    created_at,
    updated_at
FROM digital_employee_config_revisions
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND archived_at IS NULL;

-- name: GetNextDigitalEmployeeConfigRevisionNumber :one
SELECT (COALESCE(MAX(revision_number), 0) + 1)::integer
FROM digital_employee_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid;

-- name: ArchivePriorActiveDigitalEmployeeConfigRevisions :exec
-- 归档某员工当前所有已生效(active、未归档)修订,让位给新激活的修订。
-- 配合偏唯一索引 uq_digital_employee_config_revisions_active(每员工至多一条 active 未归档)。
UPDATE digital_employee_config_revisions
SET status = 'archived',
    archived_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND status = 'active'
  AND archived_at IS NULL;
