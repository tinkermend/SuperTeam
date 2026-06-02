-- name: CreateRuntimeBootstrapKey :one
INSERT INTO runtime_bootstrap_keys (
    tenant_id,
    name,
    key_hash,
    status,
    description,
    expires_at,
    created_by,
    metadata
) VALUES (
    COALESCE(sqlc.narg('tenant_id')::uuid, '00000000-0000-0000-0000-000000000001'::uuid),
    sqlc.arg('name')::varchar,
    sqlc.arg('key_hash')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.narg('description')::text,
    sqlc.arg('expires_at')::timestamptz,
    sqlc.narg('created_by')::uuid,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetActiveRuntimeBootstrapKeyByHash :one
SELECT *
FROM runtime_bootstrap_keys
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND key_hash = sqlc.arg('key_hash')::varchar
  AND status = 'active'
  AND revoked_at IS NULL
  AND (expires_at IS NULL OR expires_at > NOW());

-- name: RevokeRuntimeBootstrapKey :one
UPDATE runtime_bootstrap_keys
SET status = 'revoked',
    revoked_at = COALESCE(revoked_at, NOW()),
    revoked_by = sqlc.narg('revoked_by')::uuid,
    revoked_reason = sqlc.narg('revoked_reason')::text,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: UpsertRuntimeEnrollment :one
INSERT INTO runtime_enrollments (
    tenant_id,
    runtime_node_id,
    node_id,
    bootstrap_key_id,
    status,
    request_payload,
    last_hello_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('node_id')::varchar,
    sqlc.narg('bootstrap_key_id')::uuid,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('request_payload')::jsonb, '{}'::jsonb),
    sqlc.arg('last_hello_at')::timestamptz
)
ON CONFLICT (tenant_id, node_id) DO UPDATE SET
    runtime_node_id = EXCLUDED.runtime_node_id,
    bootstrap_key_id = EXCLUDED.bootstrap_key_id,
    status = CASE
        WHEN runtime_enrollments.status IN ('approved', 'rejected', 'revoked') THEN runtime_enrollments.status
        ELSE EXCLUDED.status
    END,
    request_payload = EXCLUDED.request_payload,
    last_hello_at = EXCLUDED.last_hello_at,
    updated_at = NOW()
RETURNING *;

-- name: GetRuntimeEnrollmentByNodeID :one
SELECT *
FROM runtime_enrollments
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND node_id = sqlc.arg('node_id')::varchar;

-- name: ListRuntimeEnrollments :many
SELECT *
FROM runtime_enrollments
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ApproveRuntimeEnrollment :one
UPDATE runtime_enrollments
SET status = 'approved',
    approved_by = sqlc.narg('approved_by')::uuid,
    approved_at = NOW(),
    rejected_by = NULL,
    rejected_at = NULL,
    reject_reason = NULL,
    revoked_by = NULL,
    revoked_at = NULL,
    revoke_reason = NULL,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: RejectRuntimeEnrollment :one
UPDATE runtime_enrollments
SET status = 'rejected',
    rejected_by = sqlc.narg('rejected_by')::uuid,
    rejected_at = NOW(),
    reject_reason = sqlc.narg('reject_reason')::text,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: RevokeRuntimeEnrollment :one
UPDATE runtime_enrollments
SET status = 'revoked',
    revoked_by = sqlc.narg('revoked_by')::uuid,
    revoked_at = NOW(),
    revoke_reason = sqlc.narg('revoke_reason')::text,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: CreateRuntimeSession :one
INSERT INTO runtime_sessions (
    tenant_id,
    runtime_node_id,
    enrollment_id,
    token_lookup_hash,
    token_secret_hash,
    expires_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.narg('enrollment_id')::uuid,
    sqlc.arg('token_lookup_hash')::varchar,
    sqlc.arg('token_secret_hash')::varchar,
    sqlc.arg('expires_at')::timestamptz
) RETURNING *;

-- name: GetActiveRuntimeSessionByLookupHash :one
SELECT *
FROM runtime_sessions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND token_lookup_hash = sqlc.arg('token_lookup_hash')::varchar
  AND expires_at > NOW()
  AND revoked_at IS NULL;

-- name: RenewRuntimeSession :one
UPDATE runtime_sessions
SET expires_at = sqlc.arg('expires_at')::timestamptz,
    last_seen_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND expires_at > NOW()
  AND revoked_at IS NULL
RETURNING *;

-- name: TouchRuntimeSessionLastSeen :one
UPDATE runtime_sessions
SET last_seen_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND expires_at > NOW()
  AND revoked_at IS NULL
RETURNING *;

-- name: RevokeRuntimeSession :one
UPDATE runtime_sessions
SET revoked_at = COALESCE(revoked_at, NOW()),
    revoked_reason = sqlc.narg('revoked_reason')::text,
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: UpsertRuntimeCapability :one
INSERT INTO runtime_capabilities (
    tenant_id,
    runtime_node_id,
    capability_type,
    capability_key,
    provider_type,
    provider_version,
    binary_path,
    available,
    workspace_base_dir,
    capacity,
    labels,
    status,
    details,
    health_status,
    metadata,
    last_seen_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('runtime_node_id')::uuid,
    sqlc.arg('capability_type')::varchar,
    sqlc.arg('capability_key')::varchar,
    sqlc.arg('provider_type')::varchar,
    sqlc.narg('provider_version')::varchar,
    sqlc.narg('binary_path')::text,
    sqlc.arg('available')::boolean,
    sqlc.narg('workspace_base_dir')::text,
    COALESCE(sqlc.arg('capacity')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.arg('labels')::jsonb, '{}'::jsonb),
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.arg('details')::jsonb, '{}'::jsonb),
    sqlc.arg('health_status')::varchar,
    COALESCE(sqlc.arg('metadata')::jsonb, '{}'::jsonb),
    sqlc.arg('last_seen_at')::timestamptz
)
ON CONFLICT (tenant_id, runtime_node_id, capability_type, capability_key) DO UPDATE SET
    status = EXCLUDED.status,
    details = EXCLUDED.details,
    last_seen_at = EXCLUDED.last_seen_at,
    updated_at = NOW()
RETURNING *;

-- name: ListRuntimeCapabilities :many
SELECT *
FROM runtime_capabilities
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND runtime_node_id = sqlc.arg('runtime_node_id')::uuid
  AND archived_at IS NULL
ORDER BY provider_type ASC;

-- name: GetRuntimeCapability :one
SELECT *
FROM runtime_capabilities
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND runtime_node_id = sqlc.arg('runtime_node_id')::uuid
  AND capability_type = sqlc.arg('capability_type')::varchar
  AND capability_key = sqlc.arg('capability_key')::varchar
  AND archived_at IS NULL;
