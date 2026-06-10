-- name: CreateProject :one
INSERT INTO projects (
    id,
    tenant_id,
    team_id,
    name,
    description,
    goal,
    status,
    human_owner_user_id,
    leader_user_id,
    acceptance_user_id,
    coordination_workflow_id,
    coordination_status,
    coordination_policy,
    approval_policy,
    evidence_policy
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('name')::varchar,
    sqlc.narg('description')::text,
    sqlc.narg('goal')::text,
    sqlc.arg('status')::varchar,
    sqlc.arg('human_owner_user_id')::uuid,
    sqlc.narg('leader_user_id')::uuid,
    sqlc.narg('acceptance_user_id')::uuid,
    sqlc.narg('coordination_workflow_id')::varchar,
    sqlc.narg('coordination_status')::varchar,
    COALESCE(sqlc.narg('coordination_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('approval_policy')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('evidence_policy')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetProject :one
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ListProjects :many
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND (
    sqlc.narg('q')::text IS NULL
    OR name ILIKE '%' || sqlc.narg('q')::text || '%'
    OR COALESCE(goal, '') ILIKE '%' || sqlc.narg('q')::text || '%'
  )
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProject :one
UPDATE projects
SET
    name = COALESCE(sqlc.narg('name')::varchar, name),
    description = COALESCE(sqlc.narg('description')::text, description),
    goal = COALESCE(sqlc.narg('goal')::text, goal),
    status = COALESCE(sqlc.narg('status')::varchar, status),
    human_owner_user_id = COALESCE(sqlc.narg('human_owner_user_id')::uuid, human_owner_user_id),
    leader_user_id = sqlc.narg('leader_user_id')::uuid,
    acceptance_user_id = sqlc.narg('acceptance_user_id')::uuid,
    coordination_policy = COALESCE(sqlc.narg('coordination_policy')::jsonb, coordination_policy),
    approval_policy = COALESCE(sqlc.narg('approval_policy')::jsonb, approval_policy),
    evidence_policy = COALESCE(sqlc.narg('evidence_policy')::jsonb, evidence_policy),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND archived_at IS NULL
RETURNING *;

-- name: ArchiveProject :one
UPDATE projects
SET status = 'archived',
    archived_at = COALESCE(archived_at, NOW()),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ReplaceProjectMembersDelete :exec
DELETE FROM project_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: CreateProjectMember :one
INSERT INTO project_members (
    tenant_id,
    project_id,
    principal_type,
    principal_id,
    project_role,
    display_name_snapshot,
    status,
    settings
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('principal_type')::varchar,
    sqlc.arg('principal_id')::uuid,
    sqlc.arg('project_role')::varchar,
    sqlc.narg('display_name_snapshot')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('settings')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListProjectMembers :many
SELECT * FROM project_members
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at ASC;

-- name: ListProjectTasks :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectTask :one
INSERT INTO project_tasks (
    tenant_id,
    project_id,
    demand_id,
    title,
    summary,
    status,
    assigned_digital_employee_id,
    runtime_task_id,
    digital_employee_run_id,
    risk_level,
    requires_human_approval
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('demand_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.arg('status')::varchar,
    sqlc.narg('assigned_digital_employee_id')::uuid,
    sqlc.narg('runtime_task_id')::uuid,
    sqlc.narg('digital_employee_run_id')::uuid,
    sqlc.narg('risk_level')::varchar,
    COALESCE(sqlc.arg('requires_human_approval')::boolean, false)
) RETURNING *;

-- name: GetLatestProjectEventSequence :one
SELECT COALESCE(MAX(sequence_number), 0)::bigint AS max_sequence
FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: CreateProjectEvent :one
INSERT INTO project_events (
    tenant_id,
    project_id,
    sequence_number,
    event_type,
    actor_type,
    actor_id,
    resource_type,
    resource_id,
    summary,
    payload
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('sequence_number')::bigint,
    sqlc.arg('event_type')::varchar,
    sqlc.arg('actor_type')::varchar,
    sqlc.arg('actor_id')::varchar,
    sqlc.narg('resource_type')::varchar,
    sqlc.narg('resource_id')::varchar,
    sqlc.narg('summary')::text,
    COALESCE(sqlc.narg('payload')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: ListProjectEvents :many
SELECT * FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY sequence_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectDemand :one
INSERT INTO project_demands (
    tenant_id,
    project_id,
    submitted_by_user_id,
    title,
    content,
    source_type,
    source_refs,
    attachments,
    priority,
    risk_level,
    status,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('submitted_by_user_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('content')::text,
    sqlc.arg('source_type')::varchar,
    COALESCE(sqlc.narg('source_refs')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('attachments')::jsonb, '[]'::jsonb),
    sqlc.narg('priority')::varchar,
    sqlc.narg('risk_level')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectDemands :many
SELECT * FROM project_demands
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectConfigRevision :one
INSERT INTO project_config_revisions (
    tenant_id,
    project_id,
    revision_number,
    config_snapshot,
    change_summary,
    created_by_user_id,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('revision_number')::integer,
    sqlc.arg('config_snapshot')::jsonb,
    sqlc.narg('change_summary')::text,
    sqlc.arg('created_by_user_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: GetLatestProjectConfigRevisionNumber :one
SELECT COALESCE(MAX(revision_number), 0)::integer AS max_revision
FROM project_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: GetLatestProjectConfigRevision :one
SELECT * FROM project_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY revision_number DESC
LIMIT 1;
