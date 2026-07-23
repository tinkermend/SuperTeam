-- name: CreateAutomationRule :one
INSERT INTO automation_rules (
    tenant_id,
    team_id,
    project_id,
    name,
    enabled,
    coordination_mode,
    demand_title_template,
    demand_body_template,
    scenario_template_key,
    digital_employee_id,
    chat_objective_template,
    schedule_kind,
    cron_expr,
    interval_seconds,
    timezone,
    overlap_policy,
    actor_user_id,
    disabled_reason,
    consecutive_failure_count,
    temporal_schedule_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('team_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('name')::varchar,
    sqlc.arg('enabled')::boolean,
    sqlc.arg('coordination_mode')::varchar,
    sqlc.narg('demand_title_template')::text,
    sqlc.narg('demand_body_template')::text,
    sqlc.narg('scenario_template_key')::varchar,
    sqlc.narg('digital_employee_id')::uuid,
    sqlc.narg('chat_objective_template')::text,
    sqlc.arg('schedule_kind')::varchar,
    sqlc.narg('cron_expr')::varchar,
    sqlc.narg('interval_seconds')::int,
    sqlc.arg('timezone')::varchar,
    sqlc.arg('overlap_policy')::varchar,
    sqlc.arg('actor_user_id')::uuid,
    sqlc.narg('disabled_reason')::varchar,
    sqlc.arg('consecutive_failure_count')::int,
    sqlc.narg('temporal_schedule_id')::varchar
)
RETURNING *;

-- name: GetAutomationRule :one
SELECT * FROM automation_rules
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ListAutomationRules :many
SELECT ar.* FROM automation_rules ar
JOIN projects p ON p.id = ar.project_id AND p.tenant_id = ar.tenant_id
WHERE ar.tenant_id = sqlc.arg('tenant_id')::uuid
  AND p.deleted_at IS NULL
  AND (
    sqlc.narg('project_id')::uuid IS NULL
    OR ar.project_id = sqlc.narg('project_id')::uuid
  )
  AND (
    sqlc.narg('enabled')::boolean IS NULL
    OR ar.enabled = sqlc.narg('enabled')::boolean
  )
ORDER BY ar.updated_at DESC
LIMIT sqlc.arg('limit_count')::int
OFFSET sqlc.arg('offset_count')::int;

-- name: UpdateAutomationRule :one
UPDATE automation_rules SET
    name = sqlc.arg('name')::varchar,
    demand_title_template = sqlc.narg('demand_title_template')::text,
    demand_body_template = sqlc.narg('demand_body_template')::text,
    scenario_template_key = sqlc.narg('scenario_template_key')::varchar,
    digital_employee_id = sqlc.narg('digital_employee_id')::uuid,
    chat_objective_template = sqlc.narg('chat_objective_template')::text,
    schedule_kind = sqlc.arg('schedule_kind')::varchar,
    cron_expr = sqlc.narg('cron_expr')::varchar,
    interval_seconds = sqlc.narg('interval_seconds')::int,
    timezone = sqlc.arg('timezone')::varchar,
    temporal_schedule_id = sqlc.narg('temporal_schedule_id')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: SetAutomationRuleEnabled :one
UPDATE automation_rules SET
    enabled = sqlc.arg('enabled')::boolean,
    disabled_reason = sqlc.narg('disabled_reason')::varchar,
    consecutive_failure_count = CASE
        WHEN sqlc.arg('enabled')::boolean THEN 0
        ELSE consecutive_failure_count
    END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: SetAutomationRuleScheduleID :one
UPDATE automation_rules SET
    temporal_schedule_id = sqlc.narg('temporal_schedule_id')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: IncrementAutomationRuleFailureCount :one
UPDATE automation_rules SET
    consecutive_failure_count = consecutive_failure_count + 1,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ResetAutomationRuleFailureCount :one
UPDATE automation_rules SET
    consecutive_failure_count = 0,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: DisableAutomationRuleSystem :one
UPDATE automation_rules SET
    enabled = FALSE,
    disabled_reason = sqlc.arg('disabled_reason')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: DeleteAutomationRule :execrows
DELETE FROM automation_rules
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ListAutomationRulesByProject :many
SELECT id, tenant_id, team_id, project_id, name, enabled, coordination_mode, demand_title_template, demand_body_template, scenario_template_key, digital_employee_id, chat_objective_template, schedule_kind, cron_expr, interval_seconds, timezone, overlap_policy, actor_user_id, disabled_reason, consecutive_failure_count, temporal_schedule_id, created_at, updated_at
FROM automation_rules
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: DeleteAutomationFiresByProject :execrows
DELETE FROM automation_fires
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND rule_id IN (
    SELECT id FROM automation_rules
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND project_id = sqlc.arg('project_id')::uuid
  );

-- name: DeleteAutomationRulesByProject :execrows
DELETE FROM automation_rules
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: ListEnabledAutomationRulesByActor :many
SELECT * FROM automation_rules
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND actor_user_id = sqlc.arg('actor_user_id')::uuid
  AND enabled = TRUE;

-- name: ListEnabledAutomationRulesByActorOnProject :many
SELECT * FROM automation_rules
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND actor_user_id = sqlc.arg('actor_user_id')::uuid
  AND enabled = TRUE;

-- name: CreateAutomationFire :one
INSERT INTO automation_fires (
    tenant_id,
    rule_id,
    scheduled_fire_at,
    idempotency_key,
    status,
    demand_id,
    run_id,
    error_code,
    error_message
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('rule_id')::uuid,
    sqlc.arg('scheduled_fire_at')::timestamptz,
    sqlc.arg('idempotency_key')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.narg('demand_id')::uuid,
    sqlc.narg('run_id')::uuid,
    sqlc.narg('error_code')::varchar,
    sqlc.narg('error_message')::text
)
RETURNING *;

-- name: GetAutomationFireByIdempotency :one
SELECT * FROM automation_fires
WHERE idempotency_key = sqlc.arg('idempotency_key')::varchar;

-- name: UpdateAutomationFire :one
UPDATE automation_fires SET
    status = sqlc.arg('status')::varchar,
    demand_id = sqlc.narg('demand_id')::uuid,
    run_id = sqlc.narg('run_id')::uuid,
    error_code = sqlc.narg('error_code')::varchar,
    error_message = sqlc.narg('error_message')::text
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
RETURNING *;

-- name: ListAutomationFires :many
SELECT * FROM automation_fires
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND rule_id = sqlc.arg('rule_id')::uuid
ORDER BY scheduled_fire_at DESC
LIMIT sqlc.arg('limit_count')::int
OFFSET sqlc.arg('offset_count')::int;

-- name: GetLatestNonTerminalAutomationFire :one
SELECT f.* FROM automation_fires f
WHERE f.tenant_id = sqlc.arg('tenant_id')::uuid
  AND f.rule_id = sqlc.arg('rule_id')::uuid
  AND f.status = 'succeeded'
  AND (
    (f.demand_id IS NOT NULL AND EXISTS (
      SELECT 1 FROM project_demands d
      WHERE d.id = f.demand_id
        AND d.tenant_id = f.tenant_id
        AND d.status NOT IN ('completed', 'failed', 'cancelled')
    ))
    OR (f.run_id IS NOT NULL AND EXISTS (
      SELECT 1 FROM task_runs tr
      WHERE tr.id = f.run_id
        AND tr.tenant_id = f.tenant_id
        AND tr.status NOT IN ('succeeded', 'failed', 'cancelled', 'timed_out')
    ))
  )
ORDER BY f.scheduled_fire_at DESC
LIMIT 1;
