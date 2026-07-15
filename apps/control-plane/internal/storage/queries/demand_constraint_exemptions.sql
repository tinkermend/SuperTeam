-- name: CreateDemandConstraintExemption :one
INSERT INTO project_demand_constraint_exemptions (
    tenant_id,
    project_id,
    demand_id,
    constraint_kind,
    roles,
    granted_by_user_id,
    decision_request_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('constraint_kind')::varchar,
    COALESCE(sqlc.narg('roles')::jsonb, '[]'::jsonb),
    sqlc.arg('granted_by_user_id')::uuid,
    sqlc.narg('decision_request_id')::uuid
)
ON CONFLICT (tenant_id, demand_id, constraint_kind) DO NOTHING
RETURNING *;

-- name: ListDemandConstraintExemptionsByDemand :many
SELECT * FROM project_demand_constraint_exemptions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
ORDER BY created_at ASC;
