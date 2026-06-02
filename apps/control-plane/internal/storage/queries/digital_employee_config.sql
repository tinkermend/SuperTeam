-- name: CreateDigitalEmployeeConfigRevision :one
INSERT INTO digital_employee_config_revisions (
    tenant_id,
    digital_employee_id,
    revision_number,
    role_profile,
    constitution_addendum,
    capability_selection,
    context_policy_override,
    approval_policy_override,
    output_contract_addendum,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('revision_number')::integer,
    COALESCE(sqlc.arg('role_profile')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('constitution_addendum')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('capability_selection')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('context_policy_override')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('approval_policy_override')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('output_contract_addendum')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING *;

-- name: GetLatestDigitalEmployeeConfigRevision :one
SELECT *
FROM digital_employee_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY revision_number DESC
LIMIT 1;

-- name: GetCurrentDigitalEmployeeConfigRevision :one
SELECT *
FROM digital_employee_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND status = 'active'
  AND archived_at IS NULL
ORDER BY revision_number DESC
LIMIT 1;

-- name: GetDigitalEmployeeConfigRevision :one
SELECT *
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

-- name: CreateDigitalEmployeeEffectiveConfig :one
INSERT INTO digital_employee_effective_configs (
    tenant_id,
    digital_employee_id,
    tenant_team_config_revision_id,
    employee_config_revision_id,
    effective_config_snapshot,
    validation_result,
    status,
    approved_by,
    approved_at
)
VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('tenant_team_config_revision_id')::uuid,
    sqlc.arg('employee_config_revision_id')::uuid,
    COALESCE(sqlc.arg('effective_config_snapshot')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('validation_result')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('approved_by')::uuid,
    sqlc.narg('approved_at')::timestamptz
)
RETURNING *;

-- name: GetLatestDigitalEmployeeEffectiveConfig :one
SELECT *
FROM digital_employee_effective_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
ORDER BY created_at DESC
LIMIT 1;

-- name: GetCurrentDigitalEmployeeEffectiveConfig :one
SELECT *
FROM digital_employee_effective_configs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND digital_employee_id = sqlc.arg('digital_employee_id')::uuid
  AND status = 'approved'
  AND revoked_at IS NULL
ORDER BY created_at DESC
LIMIT 1;
