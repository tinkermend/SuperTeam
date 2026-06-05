-- name: CreateRuntimeCommandReceipt :one
WITH inserted AS (
    INSERT INTO runtime_command_receipts (
        tenant_id,
        command_id,
        command_type,
        runtime_node_id,
        node_id,
        resource_type,
        resource_id,
        status,
        payload,
        dispatched_at
    ) VALUES (
        sqlc.arg('tenant_id')::uuid,
        sqlc.arg('command_id')::varchar,
        sqlc.arg('command_type')::varchar,
        sqlc.arg('runtime_node_id')::uuid,
        sqlc.arg('node_id')::varchar,
        sqlc.arg('resource_type')::varchar,
        sqlc.arg('resource_id')::uuid,
        sqlc.arg('status')::varchar,
        COALESCE(sqlc.arg('payload')::jsonb, '{}'::jsonb),
        sqlc.narg('dispatched_at')::timestamptz
    )
    ON CONFLICT (tenant_id, command_id) DO UPDATE SET
        command_id = runtime_command_receipts.command_id
    RETURNING *
)
SELECT * FROM inserted;

-- name: GetRuntimeCommandReceiptByCommandID :one
SELECT *
FROM runtime_command_receipts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND command_id = sqlc.arg('command_id')::varchar;

-- name: GetRuntimeCommandReceiptByCommandIDForUpdate :one
SELECT *
FROM runtime_command_receipts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND command_id = sqlc.arg('command_id')::varchar
FOR UPDATE;

-- name: UpdateRuntimeCommandReceiptStatus :one
UPDATE runtime_command_receipts
SET status = sqlc.arg('status')::varchar,
    result = COALESCE(sqlc.arg('result')::jsonb, result),
    error_message = sqlc.narg('error_message')::text,
    completed_at = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'failed', 'cancelled', 'timed_out') THEN COALESCE(completed_at, NOW())
        ELSE completed_at
    END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND command_id = sqlc.arg('command_id')::varchar
RETURNING *;
