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
) SELECT
    rn.tenant_id,
    rn.id,
    sqlc.arg('node_id')::varchar,
    rbk.id,
    'pending'::varchar,
    COALESCE(sqlc.arg('request_payload')::jsonb, '{}'::jsonb),
    sqlc.arg('last_hello_at')::timestamptz
FROM runtime_nodes rn
JOIN runtime_bootstrap_keys rbk
  ON rbk.id = sqlc.arg('bootstrap_key_id')::uuid
 AND rbk.tenant_id = rn.tenant_id
 AND rbk.status = 'active'
 AND rbk.revoked_at IS NULL
 AND (rbk.expires_at IS NULL OR rbk.expires_at > NOW())
WHERE rn.id = sqlc.arg('runtime_node_id')::uuid
  AND rn.tenant_id = sqlc.arg('tenant_id')::uuid
  AND rn.node_id = sqlc.arg('node_id')::varchar
  AND rn.disabled_at IS NULL
  AND rn.archived_at IS NULL
ON CONFLICT (tenant_id, node_id) DO UPDATE SET
    runtime_node_id = CASE
        WHEN runtime_enrollments.status IN ('approved', 'rejected', 'revoked') THEN runtime_enrollments.runtime_node_id
        ELSE EXCLUDED.runtime_node_id
    END,
    bootstrap_key_id = CASE
        WHEN runtime_enrollments.status IN ('approved', 'rejected', 'revoked') THEN runtime_enrollments.bootstrap_key_id
        ELSE EXCLUDED.bootstrap_key_id
    END,
    status = CASE
        WHEN runtime_enrollments.status IN ('approved', 'rejected', 'revoked') THEN runtime_enrollments.status
        ELSE 'pending'
    END,
    request_payload = CASE
        WHEN runtime_enrollments.status IN ('approved', 'rejected', 'revoked') THEN runtime_enrollments.request_payload
        ELSE EXCLUDED.request_payload
    END,
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
  AND status = 'pending'
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
  AND status = 'pending'
RETURNING *;

-- name: RevokeRuntimeEnrollment :one
WITH revoked_enrollment AS (
    UPDATE runtime_enrollments
    SET status = 'revoked',
        revoked_by = sqlc.narg('revoked_by')::uuid,
        revoked_at = NOW(),
        revoke_reason = sqlc.narg('revoke_reason')::text,
        updated_at = NOW()
    WHERE id = sqlc.arg('id')::uuid
      AND tenant_id = sqlc.arg('tenant_id')::uuid
      AND status IN ('pending', 'approved')
    RETURNING *
),
revoked_sessions AS (
    UPDATE runtime_sessions rs
    SET revoked_at = COALESCE(rs.revoked_at, NOW()),
        revoked_reason = COALESCE(sqlc.narg('revoke_reason')::text, rs.revoked_reason),
        updated_at = NOW()
    FROM revoked_enrollment re
    WHERE rs.tenant_id = re.tenant_id
      AND rs.runtime_node_id = re.runtime_node_id
      AND rs.expires_at > NOW()
      AND rs.revoked_at IS NULL
    RETURNING rs.id
)
SELECT *
FROM revoked_enrollment;

-- name: CreateRuntimeSession :one
INSERT INTO runtime_sessions (
    tenant_id,
    runtime_node_id,
    enrollment_id,
    token_lookup_hash,
    token_secret_hash,
    expires_at
) SELECT
    re.tenant_id,
    rn.id,
    re.id,
    sqlc.arg('token_lookup_hash')::varchar,
    sqlc.arg('token_secret_hash')::varchar,
    sqlc.arg('expires_at')::timestamptz
FROM runtime_enrollments re
JOIN runtime_nodes rn
  ON rn.id = re.runtime_node_id
 AND rn.tenant_id = re.tenant_id
 AND rn.archived_at IS NULL
WHERE re.id = sqlc.narg('enrollment_id')::uuid
  AND re.tenant_id = sqlc.arg('tenant_id')::uuid
  AND re.status = 'approved'
  AND re.runtime_node_id IS NOT NULL
  AND rn.id = sqlc.arg('runtime_node_id')::uuid
RETURNING *;

-- name: GetActiveRuntimeSessionByLookupHash :one
SELECT rs.*
FROM runtime_sessions rs
JOIN runtime_enrollments re
  ON re.id = rs.enrollment_id
 AND re.tenant_id = rs.tenant_id
 AND re.runtime_node_id = rs.runtime_node_id
 AND re.status = 'approved'
 AND re.rejected_at IS NULL
 AND re.revoked_at IS NULL
WHERE rs.tenant_id = sqlc.arg('tenant_id')::uuid
  AND rs.token_lookup_hash = sqlc.arg('token_lookup_hash')::varchar
  AND rs.expires_at > NOW()
  AND rs.revoked_at IS NULL;

-- name: RenewRuntimeSession :one
UPDATE runtime_sessions
SET expires_at = sqlc.arg('expires_at')::timestamptz,
    last_seen_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND expires_at > NOW()
  AND revoked_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM runtime_enrollments re
      WHERE re.id = runtime_sessions.enrollment_id
        AND re.tenant_id = runtime_sessions.tenant_id
        AND re.runtime_node_id = runtime_sessions.runtime_node_id
        AND re.status = 'approved'
        AND re.rejected_at IS NULL
        AND re.revoked_at IS NULL
  )
RETURNING *;

-- name: TouchRuntimeSessionLastSeen :one
UPDATE runtime_sessions
SET last_seen_at = NOW(),
    updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND expires_at > NOW()
  AND revoked_at IS NULL
  AND EXISTS (
      SELECT 1
      FROM runtime_enrollments re
      WHERE re.id = runtime_sessions.enrollment_id
        AND re.tenant_id = runtime_sessions.tenant_id
        AND re.runtime_node_id = runtime_sessions.runtime_node_id
        AND re.status = 'approved'
        AND re.rejected_at IS NULL
        AND re.revoked_at IS NULL
  )
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
) SELECT
    rn.tenant_id,
    rn.id,
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
FROM runtime_nodes rn
WHERE rn.id = sqlc.arg('runtime_node_id')::uuid
  AND rn.tenant_id = sqlc.arg('tenant_id')::uuid
  AND rn.archived_at IS NULL
ON CONFLICT (tenant_id, runtime_node_id, capability_type, capability_key) DO UPDATE SET
    provider_type = EXCLUDED.provider_type,
    provider_version = EXCLUDED.provider_version,
    binary_path = EXCLUDED.binary_path,
    available = EXCLUDED.available,
    workspace_base_dir = EXCLUDED.workspace_base_dir,
    capacity = EXCLUDED.capacity,
    labels = EXCLUDED.labels,
    status = EXCLUDED.status,
    details = EXCLUDED.details,
    health_status = EXCLUDED.health_status,
    metadata = EXCLUDED.metadata,
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
