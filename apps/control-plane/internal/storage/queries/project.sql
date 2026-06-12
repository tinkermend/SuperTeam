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
    leader_user_id = COALESCE(sqlc.narg('leader_user_id')::uuid, leader_user_id),
    acceptance_user_id = COALESCE(sqlc.narg('acceptance_user_id')::uuid, acceptance_user_id),
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

-- name: ListDemandLaunchProjectTasks :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit');

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

-- name: ListDemandLaunchProjectEvents :many
SELECT * FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (
    (sqlc.narg('created_event_id')::uuid IS NOT NULL AND id = sqlc.narg('created_event_id')::uuid)
    OR resource_id = (sqlc.arg('demand_id')::uuid)::text
    OR resource_id = ANY(sqlc.arg('project_task_ids')::varchar[])
    OR resource_id = ANY(sqlc.arg('decision_request_ids')::varchar[])
    OR payload->>'demand_id' = (sqlc.arg('demand_id')::uuid)::text
    OR payload->>'project_task_id' = ANY(sqlc.arg('project_task_ids')::varchar[])
    OR payload->>'decision_request_id' = ANY(sqlc.arg('decision_request_ids')::varchar[])
  )
ORDER BY sequence_number DESC
LIMIT sqlc.arg('limit');

-- name: GetProjectEvent :one
SELECT * FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;

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

-- name: GetProjectDemand :one
SELECT * FROM project_demands
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetProjectTask :one
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetProjectTaskRunRuntimeNodeID :one
SELECT tr.runtime_node_id
FROM project_tasks pt
JOIN task_runs tr
  ON tr.tenant_id = pt.tenant_id
 AND tr.id = sqlc.arg('run_id')::uuid
 AND tr.id = pt.digital_employee_run_id
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.id = sqlc.arg('project_task_id')::uuid;

-- name: CreateProjectCoordinationJob :one
INSERT INTO project_coordination_jobs (
    tenant_id,
    project_id,
    workflow_id,
    trigger_event_id,
    job_type,
    status,
    input_snapshot_ref,
    started_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('workflow_id')::varchar,
    sqlc.narg('trigger_event_id')::uuid,
    sqlc.arg('job_type')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('input_snapshot_ref')::jsonb, '{}'::jsonb),
    NOW()
) RETURNING *;

-- name: FinishProjectCoordinationJob :one
UPDATE project_coordination_jobs
SET status = sqlc.arg('status')::varchar,
    output_event_ids = COALESCE(sqlc.narg('output_event_ids')::jsonb, output_event_ids),
    finished_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: ListProjectCoordinationJobs :many
SELECT * FROM project_coordination_jobs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListDemandLaunchCoordinationJobs :many
SELECT * FROM project_coordination_jobs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (
    (sqlc.narg('created_event_id')::uuid IS NOT NULL AND trigger_event_id = sqlc.narg('created_event_id')::uuid)
    OR input_snapshot_ref->>'demand_id' = (sqlc.arg('demand_id')::uuid)::text
  )
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- name: CreateProjectRouteDecision :one
INSERT INTO project_route_decisions (
    tenant_id,
    project_id,
    coordination_job_id,
    demand_id,
    candidate_digital_employee_ids,
    selected_digital_employee_ids,
    reason,
    input_requirements,
    expected_outputs,
    budget_estimate,
    requires_human_review,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('coordination_job_id')::uuid,
    sqlc.narg('demand_id')::uuid,
    COALESCE(sqlc.narg('candidate_digital_employee_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('selected_digital_employee_ids')::jsonb, '[]'::jsonb),
    sqlc.arg('reason')::text,
    COALESCE(sqlc.narg('input_requirements')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('expected_outputs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('budget_estimate')::jsonb, '{}'::jsonb),
    sqlc.arg('requires_human_review')::boolean,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectRouteDecisions :many
SELECT * FROM project_route_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListDemandLaunchRouteDecisions :many
SELECT * FROM project_route_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');

-- name: UpdateProjectTaskStatus :one
UPDATE project_tasks
SET status = sqlc.arg('status')::varchar,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = ANY(sqlc.arg('current_statuses')::varchar[])
RETURNING *;

-- name: AssignProjectTask :one
UPDATE project_tasks
SET status = sqlc.arg('status')::varchar,
    assigned_digital_employee_id = COALESCE(sqlc.narg('assigned_digital_employee_id')::uuid, assigned_digital_employee_id),
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'pending')
RETURNING *;

-- name: CreateProjectExecutionSummary :one
INSERT INTO project_execution_summaries (
    tenant_id,
    project_id,
    project_task_id,
    digital_employee_id,
    conclusion,
    evidence_refs,
    artifact_refs,
    confidence_factors,
    uncertainty,
    missing_information,
    recommended_next_action,
    requires_human_review,
    transfer_request_id,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('digital_employee_id')::uuid,
    sqlc.arg('conclusion')::text,
    COALESCE(sqlc.narg('evidence_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('artifact_refs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('confidence_factors')::jsonb, '{}'::jsonb),
    sqlc.narg('uncertainty')::text,
    COALESCE(sqlc.narg('missing_information')::jsonb, '[]'::jsonb),
    sqlc.narg('recommended_next_action')::text,
    sqlc.arg('requires_human_review')::boolean,
    sqlc.narg('transfer_request_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectExecutionSummaries :many
SELECT * FROM project_execution_summaries
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectTransferRequest :one
INSERT INTO project_transfer_requests (
    tenant_id,
    project_id,
    project_task_id,
    requested_by_digital_employee_id,
    reason,
    suggested_employee_type,
    suggested_digital_employee_ids,
    missing_context_refs,
    status,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('requested_by_digital_employee_id')::uuid,
    sqlc.arg('reason')::text,
    sqlc.narg('suggested_employee_type')::varchar,
    COALESCE(sqlc.narg('suggested_digital_employee_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('missing_context_refs')::jsonb, '[]'::jsonb),
    sqlc.arg('status')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectTransferRequests :many
SELECT * FROM project_transfer_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectDecisionRequest :one
INSERT INTO project_decision_requests (
    tenant_id,
    project_id,
    approval_request_id,
    coordination_job_id,
    project_task_id,
    target_user_id,
    decision_type,
    title_snapshot,
    summary_snapshot,
    risk_level_snapshot,
    status_snapshot,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('approval_request_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('decision_type')::varchar,
    sqlc.arg('title_snapshot')::varchar,
    sqlc.narg('summary_snapshot')::text,
    sqlc.narg('risk_level_snapshot')::varchar,
    sqlc.arg('status_snapshot')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: GetProjectDecisionRequest :one
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: ResolveProjectDecisionRequest :one
UPDATE project_decision_requests
SET status_snapshot = sqlc.arg('status_snapshot')::varchar,
    resolved_event_id = sqlc.narg('resolved_event_id')::uuid,
    resolved_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status_snapshot = 'pending'
RETURNING *;

-- name: ListProjectDecisionRequests :many
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListDemandLaunchDecisionRequests :many
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (
    coordination_job_id = ANY(sqlc.arg('coordination_job_ids')::uuid[])
    OR project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
  )
ORDER BY created_at DESC
LIMIT sqlc.arg('limit');
