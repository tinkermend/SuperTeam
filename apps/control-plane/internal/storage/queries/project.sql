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

-- name: ListWorkflowInstances :many
WITH visible_demands AS (
    SELECT
        d.id AS demand_id,
        d.project_id,
        p.name AS project_name,
        d.title,
        d.submitted_by_user_id,
        d.status AS demand_status,
        d.created_at,
        d.source_refs,
        regexp_match(
          NULLIF(d.source_refs->>'sla_due_at', ''),
          '^([0-9]{4})-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])(?:[ T]([01][0-9]|2[0-3]):([0-5][0-9])(?::([0-5][0-9]))?)?$'
        ) AS sla_due_at_parts,
        COALESCE(d.updated_at, d.created_at) AS demand_updated_at
    FROM project_demands d
    JOIN projects p ON p.tenant_id = d.tenant_id AND p.id = d.project_id
    WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
      AND (sqlc.narg('project_id')::uuid IS NULL OR d.project_id = sqlc.narg('project_id')::uuid)
      AND (
        sqlc.narg('q')::text IS NULL
        OR d.title ILIKE '%' || sqlc.narg('q')::text || '%'
        OR COALESCE(d.content, '') ILIKE '%' || sqlc.narg('q')::text || '%'
        OR p.name ILIKE '%' || sqlc.narg('q')::text || '%'
      )
      AND (
        p.human_owner_user_id = sqlc.arg('actor_user_id')::uuid
        OR p.leader_user_id = sqlc.arg('actor_user_id')::uuid
        OR p.acceptance_user_id = sqlc.arg('actor_user_id')::uuid
        OR EXISTS (
          SELECT 1
          FROM project_members pm
          WHERE pm.tenant_id = p.tenant_id
            AND pm.project_id = p.id
            AND pm.principal_type = 'human_user'
            AND pm.principal_id = sqlc.arg('actor_user_id')::uuid
            AND pm.status = 'active'
        )
      )
),
demand_read_model AS (
    SELECT
        vd.*,
        CASE
          WHEN vd.sla_due_at_parts IS NOT NULL
           AND vd.sla_due_at_parts[1]::int BETWEEN 1 AND 9999
           AND vd.sla_due_at_parts[3]::int <= CASE
             WHEN vd.sla_due_at_parts[2]::int IN (1, 3, 5, 7, 8, 10, 12) THEN 31
             WHEN vd.sla_due_at_parts[2]::int IN (4, 6, 9, 11) THEN 30
             WHEN vd.sla_due_at_parts[2]::int = 2
              AND (
                vd.sla_due_at_parts[1]::int % 400 = 0
                OR (vd.sla_due_at_parts[1]::int % 4 = 0 AND vd.sla_due_at_parts[1]::int % 100 <> 0)
              ) THEN 29
             ELSE 28
           END
          THEN make_timestamptz(
            vd.sla_due_at_parts[1]::int,
            vd.sla_due_at_parts[2]::int,
            vd.sla_due_at_parts[3]::int,
            COALESCE(vd.sla_due_at_parts[4]::int, 0),
            COALESCE(vd.sla_due_at_parts[5]::int, 0),
            COALESCE(vd.sla_due_at_parts[6]::double precision, 0),
            'UTC'
          )
          ELSE NULL
        END AS safe_sla_due_at
    FROM visible_demands vd
),
task_counts AS (
    SELECT
        tenant_id,
        project_id,
        demand_id,
        COUNT(*)::int AS total_nodes,
        COUNT(*) FILTER (WHERE status IN ('completed', 'done', 'success'))::int AS completed_nodes,
        COUNT(*) FILTER (WHERE status IN ('assigned', 'running', 'in_progress'))::int AS running_nodes,
        COUNT(*) FILTER (WHERE status IN ('blocked'))::int AS blocked_nodes,
        COUNT(*) FILTER (WHERE requires_human_approval OR status IN ('waiting_human', 'pending_review'))::int AS waiting_human_nodes,
        COUNT(*) FILTER (WHERE status IN ('planned', 'pending'))::int AS planned_nodes,
        COUNT(*) FILTER (WHERE status IN ('failed'))::int AS failed_nodes,
        COUNT(*) FILTER (WHERE status IN ('cancelled'))::int AS cancelled_nodes,
        MAX(NULLIF(risk_level, '')) FILTER (WHERE status NOT IN ('completed', 'done', 'success', 'cancelled')) AS active_risk_level,
        MAX(updated_at) AS task_updated_at
    FROM project_tasks
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND demand_id IS NOT NULL
    GROUP BY tenant_id, project_id, demand_id
),
decision_counts AS (
    SELECT
        dr.tenant_id,
        dr.project_id,
        COALESCE(pt.demand_id, rd.demand_id) AS demand_id,
        COUNT(*) FILTER (WHERE dr.status_snapshot IN ('pending', 'requested'))::int AS pending_decisions,
        MAX(dr.updated_at) AS decision_updated_at
    FROM project_decision_requests dr
    LEFT JOIN project_tasks pt
      ON pt.tenant_id = dr.tenant_id
     AND pt.project_id = dr.project_id
     AND pt.id = dr.project_task_id
    LEFT JOIN project_route_decisions rd
      ON rd.tenant_id = dr.tenant_id
     AND rd.project_id = dr.project_id
     AND rd.coordination_job_id = dr.coordination_job_id
    WHERE dr.tenant_id = sqlc.arg('tenant_id')::uuid
      AND COALESCE(pt.demand_id, rd.demand_id) IS NOT NULL
    GROUP BY dr.tenant_id, dr.project_id, COALESCE(pt.demand_id, rd.demand_id)
),
latest_jobs AS (
    SELECT DISTINCT ON (j.tenant_id, j.project_id, rd.demand_id)
        j.tenant_id,
        j.project_id,
        rd.demand_id,
        j.id AS selected_coordination_job_id,
        GREATEST(COALESCE(j.finished_at, j.started_at), j.started_at, j.created_at) AS job_updated_at
    FROM project_coordination_jobs j
    JOIN project_route_decisions rd
      ON rd.tenant_id = j.tenant_id
     AND rd.project_id = j.project_id
     AND rd.coordination_job_id = j.id
    WHERE j.tenant_id = sqlc.arg('tenant_id')::uuid
      AND rd.demand_id IS NOT NULL
    ORDER BY j.tenant_id, j.project_id, rd.demand_id, j.created_at DESC
),
latest_events AS (
    SELECT DISTINCT ON (e.tenant_id, e.project_id, demand_id)
        e.tenant_id,
        e.project_id,
        demand_id,
        e.event_type,
        COALESCE(NULLIF(e.summary, ''), e.event_type)::text AS event_summary,
        e.created_at AS event_occurred_at
    FROM (
        SELECT
            pe.*,
            COALESCE(
                CASE
                  WHEN NULLIF(pe.payload->>'demand_id', '') ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$'
                  THEN NULLIF(pe.payload->>'demand_id', '')::uuid
                  ELSE NULL
                END,
                pt.demand_id,
                rd.demand_id
            ) AS demand_id
        FROM project_events pe
        LEFT JOIN project_tasks pt
          ON pt.tenant_id = pe.tenant_id
         AND pt.project_id = pe.project_id
         AND pt.id::text = pe.resource_id
        LEFT JOIN project_route_decisions rd
          ON rd.tenant_id = pe.tenant_id
         AND rd.project_id = pe.project_id
         AND rd.coordination_job_id::text = pe.resource_id
        WHERE pe.tenant_id = sqlc.arg('tenant_id')::uuid
    ) e
    WHERE demand_id IS NOT NULL
    ORDER BY e.tenant_id, e.project_id, demand_id, e.created_at DESC
),
decision_blockers AS (
    SELECT DISTINCT ON (item.tenant_id, item.project_id, item.demand_id)
        item.tenant_id,
        item.project_id,
        item.demand_id,
        'decision_request'::text AS blocker_type,
        item.title_snapshot::text AS blocker_title,
        item.id AS blocker_resource_id,
        item.updated_at AS blocker_updated_at
    FROM (
        SELECT
            dr.*,
            COALESCE(pt.demand_id, rd.demand_id) AS demand_id
        FROM project_decision_requests dr
        LEFT JOIN project_tasks pt
          ON pt.tenant_id = dr.tenant_id
         AND pt.project_id = dr.project_id
         AND pt.id = dr.project_task_id
        LEFT JOIN project_route_decisions rd
          ON rd.tenant_id = dr.tenant_id
         AND rd.project_id = dr.project_id
         AND rd.coordination_job_id = dr.coordination_job_id
        WHERE dr.tenant_id = sqlc.arg('tenant_id')::uuid
          AND dr.status_snapshot IN ('pending', 'requested')
    ) item
    WHERE item.demand_id IS NOT NULL
    ORDER BY item.tenant_id, item.project_id, item.demand_id, item.updated_at DESC, item.id DESC
),
task_blockers AS (
    SELECT DISTINCT ON (tenant_id, project_id, demand_id)
        tenant_id,
        project_id,
        demand_id,
        'project_task'::text AS blocker_type,
        title::text AS blocker_title,
        id AS blocker_resource_id,
        updated_at AS blocker_updated_at
    FROM project_tasks
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND demand_id IS NOT NULL
      AND (
        requires_human_approval
        OR status IN ('waiting_human', 'pending_review', 'failed', 'blocked')
      )
    ORDER BY
        tenant_id,
        project_id,
        demand_id,
        CASE
          WHEN requires_human_approval OR status IN ('waiting_human', 'pending_review') THEN 1
          ELSE 2
        END ASC,
        updated_at DESC,
        id DESC
)
SELECT
    vd.demand_id,
    vd.project_id,
    vd.project_name,
    vd.title,
    vd.submitted_by_user_id,
    COALESCE(NULLIF(vd.source_refs->>'submitted_by_display_name', ''), vd.submitted_by_user_id::text)::text AS submitted_by_display_name,
    CASE
      WHEN COALESCE(dc.pending_decisions, 0) > 0 OR COALESCE(tc.waiting_human_nodes, 0) > 0 THEN 'waiting_human'
      WHEN COALESCE(tc.failed_nodes, 0) > 0 THEN 'failed'
      WHEN COALESCE(tc.running_nodes, 0) > 0 THEN 'running'
      WHEN COALESCE(tc.cancelled_nodes, 0) > 0 OR vd.demand_status = 'cancelled' THEN 'cancelled'
      WHEN COALESCE(tc.total_nodes, 0) = 0 THEN 'planning'
      WHEN tc.completed_nodes = tc.total_nodes THEN 'completed'
      ELSE 'unknown'
    END::text AS status,
    CASE
      WHEN COALESCE(dc.pending_decisions, 0) > 0 THEN '等待人工决策'
      WHEN COALESCE(tc.waiting_human_nodes, 0) > 0 THEN '等待人工处理'
      WHEN COALESCE(tc.failed_nodes, 0) > 0 THEN '存在失败任务'
      WHEN COALESCE(tc.running_nodes, 0) > 0 THEN '任务执行中'
      WHEN COALESCE(tc.cancelled_nodes, 0) > 0 OR vd.demand_status = 'cancelled' THEN '已取消'
      WHEN COALESCE(tc.total_nodes, 0) = 0 THEN '任务正在规划'
      ELSE ''
    END::text AS status_reason,
    vd.created_at,
    GREATEST(
      vd.demand_updated_at,
      COALESCE(tc.task_updated_at, vd.demand_updated_at),
      COALESCE(dc.decision_updated_at, vd.demand_updated_at),
      COALESCE(lj.job_updated_at, vd.demand_updated_at)
    )::timestamptz AS updated_at,
    lj.selected_coordination_job_id,
    COALESCE(tc.total_nodes, 0)::int AS total_nodes,
    COALESCE(tc.completed_nodes, 0)::int AS completed_nodes,
    COALESCE(tc.running_nodes, 0)::int AS running_nodes,
    COALESCE(tc.blocked_nodes, 0)::int AS blocked_nodes,
    (COALESCE(tc.waiting_human_nodes, 0) + COALESCE(dc.pending_decisions, 0))::int AS waiting_human_nodes,
    COALESCE(tc.planned_nodes, 0)::int AS planned_nodes,
    COALESCE(tc.failed_nodes, 0)::int AS failed_nodes,
    COALESCE(tc.cancelled_nodes, 0)::int AS cancelled_nodes,
    COALESCE(NULLIF(vd.source_refs->>'priority', ''), NULLIF(vd.source_refs->>'severity', ''), '')::text AS priority_value,
    COALESCE(CASE
      WHEN COALESCE(NULLIF(vd.source_refs->>'priority', ''), NULLIF(vd.source_refs->>'severity', '')) IS NULL THEN NULL
      ELSE UPPER(COALESCE(NULLIF(vd.source_refs->>'priority', ''), NULLIF(vd.source_refs->>'severity', '')))
    END, '')::text AS priority_label,
    COALESCE(CASE
      WHEN NULLIF(vd.source_refs->>'priority', '') IS NOT NULL THEN 'source_refs.priority'
      WHEN NULLIF(vd.source_refs->>'severity', '') IS NOT NULL THEN 'source_refs.severity'
      ELSE NULL
    END, '')::text AS priority_source,
    COALESCE(NULLIF(tc.active_risk_level, ''), '')::text AS risk_level,
    COALESCE(CASE
      WHEN NULLIF(tc.active_risk_level, '') IS NULL THEN NULL
      ELSE NULLIF(tc.active_risk_level, '')
    END, '')::text AS risk_label,
    COALESCE(CASE
      WHEN NULLIF(tc.active_risk_level, '') IS NULL THEN NULL
      ELSE 'project_tasks.risk_level'
    END, '')::text AS risk_source,
    vd.safe_sla_due_at::timestamptz AS sla_due_at,
    COALESCE(CASE
      WHEN vd.safe_sla_due_at IS NULL THEN NULL
      ELSE LEAST(GREATEST(EXTRACT(EPOCH FROM (vd.safe_sla_due_at - NOW())), 0), 2147483647)::int
    END, 0)::int AS sla_remaining_seconds,
    COALESCE(CASE
      WHEN vd.safe_sla_due_at IS NULL THEN NULL
      ELSE (vd.safe_sla_due_at < NOW())
    END, false)::boolean AS sla_breached,
    COALESCE(CASE
      WHEN vd.safe_sla_due_at IS NULL THEN NULL
      WHEN vd.safe_sla_due_at < NOW() THEN '已超时'
      ELSE 'SLA 生效'
    END, '')::text AS sla_label,
    COALESCE(CASE
      WHEN vd.safe_sla_due_at IS NULL THEN NULL
      ELSE 'source_refs.sla_due_at'
    END, '')::text AS sla_source,
    COALESCE(le.event_type, '')::text AS recent_event_type,
    COALESCE(le.event_summary, '')::text AS recent_event_summary,
    le.event_occurred_at::timestamptz AS recent_event_occurred_at,
    COALESCE(db.blocker_type, tb.blocker_type, '')::text AS current_blocker_type,
    COALESCE(db.blocker_title, tb.blocker_title, '')::text AS current_blocker_title,
    COALESCE(db.blocker_resource_id, tb.blocker_resource_id, '00000000-0000-0000-0000-000000000000'::uuid) AS current_blocker_resource_id
FROM demand_read_model vd
LEFT JOIN task_counts tc
  ON tc.project_id = vd.project_id
 AND tc.demand_id = vd.demand_id
LEFT JOIN decision_counts dc
  ON dc.project_id = vd.project_id
 AND dc.demand_id = vd.demand_id
LEFT JOIN latest_jobs lj
  ON lj.project_id = vd.project_id
 AND lj.demand_id = vd.demand_id
LEFT JOIN latest_events le
  ON le.project_id = vd.project_id
 AND le.demand_id = vd.demand_id
LEFT JOIN decision_blockers db
  ON db.project_id = vd.project_id
 AND db.demand_id = vd.demand_id
LEFT JOIN task_blockers tb
  ON tb.project_id = vd.project_id
 AND tb.demand_id = vd.demand_id
ORDER BY
    CASE
      WHEN COALESCE(dc.pending_decisions, 0) > 0 OR COALESCE(tc.waiting_human_nodes, 0) > 0 THEN 1
      WHEN COALESCE(tc.failed_nodes, 0) > 0 THEN 2
      WHEN COALESCE(tc.running_nodes, 0) > 0 THEN 3
      WHEN COALESCE(tc.cancelled_nodes, 0) > 0 OR vd.demand_status = 'cancelled' THEN 6
      WHEN tc.completed_nodes = tc.total_nodes THEN 5
      ELSE 4
    END ASC,
    updated_at DESC
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

-- name: TransitionProjectStatus :one
-- Forward-guarded project status transition: only applied when the current status
-- is in from_statuses. No matching row (wrong current status) yields no rows so the
-- caller can treat it as an idempotent no-op via ErrNoRows.
UPDATE projects
SET status = sqlc.arg('to_status')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = ANY(sqlc.arg('from_statuses')::varchar[])
RETURNING *;

-- name: CountProjectDemandsByTerminality :one
-- Aggregates a project's demands into total / non-terminal counts so the coordinator
-- can decide whether the whole project is ready for human acceptance.
SELECT
    COUNT(*)::integer AS total_count,
    COUNT(*) FILTER (WHERE status NOT IN ('completed', 'failed', 'cancelled'))::integer AS non_terminal_count
FROM project_demands
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

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

-- name: ListProjectTasksByDemand :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
ORDER BY stage_index ASC NULLS LAST, created_at ASC;

-- name: CreateProjectTask :one
INSERT INTO project_tasks (
    tenant_id,
    project_id,
    demand_id,
    coordination_job_id,
    route_decision_id,
    planned_task_key,
    task_kind,
    stage_index,
    revision_of_task_id,
    accepted_plan_revision_id,
    decomposition_claim_key,
    title,
    summary,
    status,
    assigned_digital_employee_id,
    runtime_task_id,
    digital_employee_run_id,
    risk_level,
    requires_human_approval,
    expected_outputs,
    input_requirements,
    handoff_contract,
    planner_metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('demand_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('route_decision_id')::uuid,
    sqlc.narg('planned_task_key')::varchar,
    sqlc.narg('task_kind')::varchar,
    sqlc.narg('stage_index')::integer,
    sqlc.narg('revision_of_task_id')::uuid,
    sqlc.narg('accepted_plan_revision_id')::uuid,
    sqlc.narg('decomposition_claim_key')::varchar,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.arg('status')::varchar,
    sqlc.narg('assigned_digital_employee_id')::uuid,
    sqlc.narg('runtime_task_id')::uuid,
    sqlc.narg('digital_employee_run_id')::uuid,
    sqlc.narg('risk_level')::varchar,
    COALESCE(sqlc.arg('requires_human_approval')::boolean, false),
    COALESCE(sqlc.narg('expected_outputs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('input_requirements')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('handoff_contract')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('planner_metadata')::jsonb, '{}'::jsonb)
) RETURNING *;

-- name: GetLatestProjectEventSequence :one
SELECT COALESCE(MAX(sequence_number), 0)::bigint AS max_sequence
FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: LockProjectEventSequence :exec
SELECT pg_advisory_xact_lock(hashtextextended((sqlc.arg('tenant_id')::uuid)::text || ':' || (sqlc.arg('project_id')::uuid)::text, 0));

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

-- name: ListProjectTaskGraphEvents :many
SELECT * FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (
    actor_id = ANY(sqlc.arg('coordination_job_ids')::varchar[])
    OR actor_id = ANY(sqlc.arg('project_task_ids')::varchar[])
    OR resource_id = ANY(sqlc.arg('project_task_ids')::varchar[])
    OR resource_id = ANY(sqlc.arg('decision_request_ids')::varchar[])
    OR payload->>'coordination_job_id' = ANY(sqlc.arg('coordination_job_ids')::varchar[])
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

-- name: GetProjectEventByTypeAndActor :one
SELECT * FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND event_type = sqlc.arg('event_type')::varchar
  AND actor_id = sqlc.arg('actor_id')::varchar
ORDER BY sequence_number DESC
LIMIT 1;

-- name: ListProjectTaskGraphReplayEvents :many
SELECT * FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND event_type = ANY(sqlc.arg('event_types')::varchar[])
  AND (
    actor_id = sqlc.arg('coordination_job_id')::varchar
    OR actor_id = ANY(sqlc.arg('project_task_ids')::varchar[])
    OR resource_id = sqlc.arg('coordination_job_id')::varchar
    OR resource_id = ANY(sqlc.arg('project_task_ids')::varchar[])
    OR payload->>'coordination_job_id' = sqlc.arg('coordination_job_id')::varchar
    OR payload->>'project_task_id' = ANY(sqlc.arg('project_task_ids')::varchar[])
  )
ORDER BY sequence_number DESC;

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

-- name: UpdateProjectDemandStatus :one
UPDATE project_demands
SET status = sqlc.arg('status')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: CountProjectTaskStatusesByDemand :one
SELECT
    COUNT(*)::bigint AS total,
    COUNT(*) FILTER (WHERE status = 'completed')::bigint AS completed,
    COUNT(*) FILTER (WHERE status = 'failed')::bigint AS failed,
    COUNT(*) FILTER (WHERE status NOT IN ('completed', 'failed', 'cancelled'))::bigint AS active
FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid;

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

-- name: GetProjectCoordinationJobByTrigger :one
SELECT * FROM project_coordination_jobs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND workflow_id = sqlc.arg('workflow_id')::varchar
  AND trigger_event_id = sqlc.arg('trigger_event_id')::uuid
  AND job_type = sqlc.arg('job_type')::varchar;

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

-- name: GetProjectRouteDecisionByCoordinationJob :one
SELECT * FROM project_route_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND coordination_job_id = sqlc.arg('coordination_job_id')::uuid;

-- name: CreateProjectTaskDependency :one
INSERT INTO project_task_dependencies (
    tenant_id, project_id, coordination_job_id, dependent_task_id, blocker_task_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.arg('dependent_task_id')::uuid,
    sqlc.arg('blocker_task_id')::uuid
) RETURNING *;

-- name: ListProjectTaskDependencies :many
SELECT * FROM project_task_dependencies
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND dependent_task_id = ANY(sqlc.arg('dependent_task_ids')::uuid[])
ORDER BY created_at ASC;

-- name: RewireProjectTaskDependencies :many
WITH affected AS (
    SELECT id, tenant_id, project_id, coordination_job_id, dependent_task_id
    FROM project_task_dependencies
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND project_id = sqlc.arg('project_id')::uuid
      AND blocker_task_id = sqlc.arg('old_blocker_task_id')::uuid
      AND dependent_task_id = ANY(sqlc.arg('dependent_task_ids')::uuid[])
),
inserted AS (
    INSERT INTO project_task_dependencies (
        tenant_id, project_id, coordination_job_id, dependent_task_id, blocker_task_id
    )
    SELECT
        tenant_id,
        project_id,
        coordination_job_id,
        dependent_task_id,
        sqlc.arg('new_blocker_task_id')::uuid
    FROM affected
    ON CONFLICT (tenant_id, dependent_task_id, blocker_task_id) DO NOTHING
    RETURNING *
),
deleted AS (
    DELETE FROM project_task_dependencies d
    USING affected a
    WHERE d.id = a.id
    RETURNING d.*
)
SELECT * FROM inserted
UNION ALL
SELECT existing.*
FROM project_task_dependencies existing
JOIN affected a
  ON existing.tenant_id = a.tenant_id
 AND existing.dependent_task_id = a.dependent_task_id
WHERE existing.blocker_task_id = sqlc.arg('new_blocker_task_id')::uuid
ORDER BY created_at ASC;

-- name: ListDependentsOfTask :many
SELECT dependent_task_id
FROM project_task_dependencies
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND blocker_task_id = sqlc.arg('blocker_task_id')::uuid
ORDER BY created_at ASC;

-- name: ListUnresolvedBlockersForTasks :many
SELECT
    d.dependent_task_id,
    d.blocker_task_id,
    b.status AS blocker_status
FROM project_task_dependencies d
JOIN project_tasks b
  ON b.tenant_id = d.tenant_id
 AND b.id = d.blocker_task_id
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.project_id = sqlc.arg('project_id')::uuid
  AND d.dependent_task_id = ANY(sqlc.arg('dependent_task_ids')::uuid[])
  AND b.status <> 'completed'
ORDER BY d.dependent_task_id, d.created_at ASC;

-- name: ListProjectTasksByCoordinationJob :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND coordination_job_id = sqlc.arg('coordination_job_id')::uuid
ORDER BY stage_index ASC NULLS LAST, created_at ASC;

-- name: ListProjectTasksByAcceptedPlanRevision :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND accepted_plan_revision_id = sqlc.arg('accepted_plan_revision_id')::uuid
ORDER BY stage_index ASC NULLS LAST, created_at ASC;

-- name: CreateProjectPlanRevision :one
INSERT INTO project_plan_revisions (
    tenant_id,
    team_id,
    project_id,
    demand_id,
    coordination_job_id,
    route_decision_id,
    revision_number,
    status,
    payload,
    planner_provider,
    planner_model,
    planner_input_hash,
    plan_fingerprint,
    validation_errors,
    validation_warnings,
    review_required,
    review_reason
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('route_decision_id')::uuid,
    sqlc.arg('revision_number')::integer,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('payload')::jsonb, '{}'::jsonb),
    sqlc.narg('planner_provider')::varchar,
    sqlc.narg('planner_model')::varchar,
    sqlc.narg('planner_input_hash')::varchar,
    sqlc.arg('plan_fingerprint')::varchar,
    COALESCE(sqlc.narg('validation_errors')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('validation_warnings')::jsonb, '[]'::jsonb),
    sqlc.arg('review_required')::boolean,
    sqlc.narg('review_reason')::text
) RETURNING *;

-- name: GetProjectPlanRevision :one
SELECT * FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetProjectPlanRevisionByFingerprint :one
SELECT * FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND plan_fingerprint = sqlc.arg('plan_fingerprint')::varchar;

-- name: ListProjectPlanRevisions :many
SELECT * FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('demand_id')::uuid IS NULL OR demand_id = sqlc.narg('demand_id')::uuid)
ORDER BY demand_id ASC, revision_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: NextProjectPlanRevisionNumber :one
SELECT COALESCE(MAX(revision_number), 0)::integer + 1 AS revision_number
FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid;

-- name: SupersedeOpenProjectPlanRevisions :exec
UPDATE project_plan_revisions
SET status = 'superseded',
    superseded_by_revision_id = sqlc.arg('superseded_by_revision_id')::uuid,
    rejection_reason = sqlc.narg('reason')::text,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND id <> sqlc.arg('superseded_by_revision_id')::uuid
  AND status IN ('draft', 'validation_failed', 'pending_review');

-- name: AcceptProjectPlanRevision :one
UPDATE project_plan_revisions
SET status = 'accepted',
    accepted_by = sqlc.narg('accepted_by')::uuid,
    accepted_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('draft', 'pending_review')
RETURNING *;

-- name: RejectProjectPlanRevision :one
UPDATE project_plan_revisions
SET status = 'rejected',
    rejected_by = sqlc.narg('rejected_by')::uuid,
    rejected_at = NOW(),
    rejection_reason = sqlc.narg('rejection_reason')::text,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'pending_review'
RETURNING *;

-- name: MarkProjectPlanRevisionDecomposing :one
UPDATE project_plan_revisions
SET status = 'decomposing',
    decomposition_claim_id = sqlc.arg('decomposition_claim_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'accepted'
RETURNING *;

-- name: MarkProjectPlanRevisionDecomposed :one
UPDATE project_plan_revisions
SET status = 'decomposed',
    created_task_ids = sqlc.arg('created_task_ids')::uuid[],
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('decomposing', 'decomposed')
RETURNING *;

-- name: CreateProjectPlanDecompositionClaim :one
INSERT INTO project_plan_decomposition_claims (
    tenant_id,
    project_id,
    demand_id,
    accepted_plan_revision_id,
    plan_fingerprint,
    status
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('accepted_plan_revision_id')::uuid,
    sqlc.arg('plan_fingerprint')::varchar,
    'in_flight'
)
ON CONFLICT (tenant_id, project_id, demand_id, accepted_plan_revision_id)
DO UPDATE SET updated_at = project_plan_decomposition_claims.updated_at
RETURNING *;

-- name: CompleteProjectPlanDecompositionClaim :one
UPDATE project_plan_decomposition_claims
SET status = 'completed',
    created_task_ids = sqlc.arg('created_task_ids')::uuid[],
    error = '{}'::jsonb,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: FailProjectPlanDecompositionClaim :one
UPDATE project_plan_decomposition_claims
SET status = 'failed',
    error = COALESCE(sqlc.narg('error')::jsonb, '{}'::jsonb),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: GetProjectTaskCompletionContract :one
SELECT id, tenant_id, project_id, expected_outputs, handoff_contract, digital_employee_run_id
FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: CreateProjectTaskAttempt :one
INSERT INTO project_task_attempts (
    id,
    tenant_id,
    project_task_id,
    attempt_no,
    status,
    digital_employee_run_id,
    runtime_task_id,
    runtime_node_id,
    execution_context_packet,
    execution_context_packet_version,
    lease_token,
    lease_expires_at,
    idempotency_key,
    dispatch_gate_result_id,
    created_event_id
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.arg('attempt_no')::integer,
    sqlc.arg('status')::varchar,
    sqlc.narg('digital_employee_run_id')::uuid,
    sqlc.narg('runtime_task_id')::uuid,
    sqlc.narg('runtime_node_id')::uuid,
    sqlc.arg('execution_context_packet')::jsonb,
    sqlc.arg('execution_context_packet_version')::varchar,
    sqlc.arg('lease_token')::varchar,
    sqlc.narg('lease_expires_at')::timestamptz,
    sqlc.arg('idempotency_key')::varchar,
    sqlc.narg('dispatch_gate_result_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: GetProjectTaskAttempt :one
SELECT * FROM project_task_attempts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: SetProjectTaskAttemptDispatchGate :one
UPDATE project_task_attempts
SET dispatch_gate_result_id = sqlc.arg('dispatch_gate_result_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND EXISTS (
      SELECT 1
      FROM project_tasks
      WHERE project_tasks.tenant_id = project_task_attempts.tenant_id
        AND project_tasks.project_id = sqlc.arg('project_id')::uuid
        AND project_tasks.id = project_task_attempts.project_task_id
  )
  AND (
      project_task_attempts.dispatch_gate_result_id IS NULL
      OR project_task_attempts.dispatch_gate_result_id = sqlc.arg('dispatch_gate_result_id')::uuid
  )
RETURNING *;

-- name: GetProjectTaskAttemptByIdempotencyKey :one
SELECT * FROM project_task_attempts
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND idempotency_key = sqlc.arg('idempotency_key')::varchar;

-- name: GetCurrentProjectTaskAttempt :one
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt ON pt.current_attempt_id = pta.id
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.id = sqlc.arg('project_task_id')::uuid;

-- name: LockProjectTaskForQueue :one
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
FOR UPDATE;

-- name: QueueProjectTask :one
UPDATE project_tasks
SET status = 'queued',
    current_attempt_id = sqlc.arg('current_attempt_id')::uuid,
    runtime_task_id = COALESCE(sqlc.narg('runtime_task_id')::uuid, runtime_task_id),
    digital_employee_run_id = COALESCE(sqlc.narg('digital_employee_run_id')::uuid, digital_employee_run_id),
    attempt_count = attempt_count + 1,
    retry_not_before = NULL,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
RETURNING *;

-- name: ScheduleProjectTaskRetry :one
UPDATE project_tasks
SET status = 'queued',
    current_attempt_id = sqlc.arg('current_attempt_id')::uuid,
    attempt_count = attempt_count + 1,
    retry_not_before = sqlc.narg('retry_not_before')::timestamptz,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('running', 'waiting_human')
RETURNING *;

-- name: MoveProjectTaskToWaitingHuman :one
UPDATE project_tasks
SET status = 'waiting_human',
    waiting_reason = sqlc.arg('waiting_reason')::varchar,
    waiting_request_id = sqlc.narg('waiting_request_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('queued', 'running')
RETURNING *;

-- name: StartProjectTaskAttempt :one
UPDATE project_task_attempts
SET status = 'running',
    runtime_node_id = sqlc.arg('runtime_node_id')::uuid,
    provider_session_id = COALESCE(sqlc.narg('provider_session_id')::varchar, provider_session_id),
    started_at = COALESCE(started_at, NOW()),
    renewed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status = 'queued'
RETURNING *;

-- name: RenewProjectTaskAttemptLease :one
UPDATE project_task_attempts
SET lease_expires_at = sqlc.narg('lease_expires_at')::timestamptz,
    renewed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status IN ('queued', 'running')
RETURNING *;

-- name: FinishProjectTaskAttempt :one
UPDATE project_task_attempts
SET status = sqlc.arg('status')::varchar,
    provider_session_id = COALESCE(sqlc.narg('provider_session_id')::varchar, provider_session_id),
    finished_at = NOW(),
    retryable = sqlc.narg('retryable')::boolean,
    failure_family = sqlc.narg('failure_family')::varchar,
    failure_message = sqlc.narg('failure_message')::text,
    terminal_event_id = sqlc.narg('terminal_event_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status IN ('queued', 'running')
RETURNING *;

-- name: UpdateProjectTaskStatus :one
UPDATE project_tasks
SET status = sqlc.arg('status')::varchar,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    terminal_event_id = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'failed', 'cancelled')
        THEN COALESCE(sqlc.narg('latest_event_id')::uuid, terminal_event_id)
        ELSE terminal_event_id
    END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = ANY(sqlc.arg('current_statuses')::varchar[])
RETURNING *;

-- name: BindProjectTaskRun :one
UPDATE project_tasks
SET status = 'assigned',
    runtime_task_id = sqlc.arg('runtime_task_id')::uuid,
    digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND (
      status = ANY(sqlc.arg('current_statuses')::varchar[])
      OR (
          status = 'assigned'
          AND runtime_task_id = sqlc.arg('runtime_task_id')::uuid
          AND digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid
      )
  )
  AND (
      digital_employee_run_id IS NULL
      OR digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid
  )
RETURNING *;

-- name: ProjectTaskEventExists :one
SELECT EXISTS (
    SELECT 1 FROM project_events
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
      AND project_id = sqlc.arg('project_id')::uuid
      AND event_type = sqlc.arg('event_type')::varchar
      AND actor_id = sqlc.arg('actor_id')::varchar
) AS event_exists;

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

-- name: ListProjectExecutionSummariesByTaskIDs :many
SELECT * FROM project_execution_summaries
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
ORDER BY created_at DESC;

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

-- name: GetProjectDecisionRequestByApprovalAndTask :one
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND approval_request_id = sqlc.arg('approval_request_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
ORDER BY created_at DESC
LIMIT 1;

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

-- name: ListProjectTaskGraphDecisionRequests :many
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (
    coordination_job_id = ANY(sqlc.arg('coordination_job_ids')::uuid[])
    OR project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
  )
ORDER BY created_at DESC;

-- name: CreateProjectTaskResult :one
INSERT INTO project_task_results (
    tenant_id, project_id, project_task_id, attempt_id, execution_summary_id,
    result_status, validation_status, decision, contract_payload, validation_errors,
    validation_warnings, idempotency_key, human_review_request, replan_request,
    revision_request, created_event_id, decision_request_id, revision_task_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.narg('attempt_id')::uuid,
    sqlc.narg('execution_summary_id')::uuid,
    sqlc.arg('result_status')::varchar,
    sqlc.arg('validation_status')::varchar,
    sqlc.arg('decision')::varchar,
    sqlc.arg('contract_payload')::jsonb,
    COALESCE(sqlc.narg('validation_errors')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('validation_warnings')::jsonb, '[]'::jsonb),
    sqlc.arg('idempotency_key')::varchar,
    COALESCE(sqlc.narg('human_review_request')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('replan_request')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('revision_request')::jsonb, '{}'::jsonb),
    sqlc.narg('created_event_id')::uuid,
    sqlc.narg('decision_request_id')::uuid,
    sqlc.narg('revision_task_id')::uuid
) ON CONFLICT (
    tenant_id,
    project_task_id,
    COALESCE(attempt_id, '00000000-0000-0000-0000-000000000000'::uuid),
    idempotency_key
)
DO UPDATE SET updated_at = project_task_results.updated_at
RETURNING *;

-- name: LinkProjectTaskLatestResult :one
UPDATE project_tasks
SET latest_task_result_id = sqlc.arg('task_result_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('project_task_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
RETURNING *;

-- name: ListProjectTaskResults :many
SELECT * FROM project_task_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: LinkProjectTaskResultDecisionRequest :one
UPDATE project_task_results
SET decision_request_id = sqlc.arg('decision_request_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND (decision_request_id IS NULL OR decision_request_id = sqlc.arg('decision_request_id')::uuid)
  AND EXISTS (
    SELECT 1 FROM project_decision_requests
    WHERE project_decision_requests.tenant_id = sqlc.arg('tenant_id')::uuid
      AND project_decision_requests.project_id = sqlc.arg('project_id')::uuid
      AND project_decision_requests.id = sqlc.arg('decision_request_id')::uuid
  )
RETURNING *;

-- name: LinkDecisionRequestProjectTaskResult :one
UPDATE project_decision_requests
SET project_task_result_id = sqlc.arg('project_task_result_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('decision_request_id')::uuid
  AND (project_task_result_id IS NULL OR project_task_result_id = sqlc.arg('project_task_result_id')::uuid)
  AND EXISTS (
    SELECT 1 FROM project_task_results
    WHERE project_task_results.tenant_id = sqlc.arg('tenant_id')::uuid
      AND project_task_results.project_id = sqlc.arg('project_id')::uuid
      AND project_task_results.id = sqlc.arg('project_task_result_id')::uuid
  )
RETURNING *;

-- name: LinkProjectTaskResultRevisionTask :one
UPDATE project_task_results
SET revision_task_id = sqlc.arg('revision_task_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND (revision_task_id IS NULL OR revision_task_id = sqlc.arg('revision_task_id')::uuid)
  AND EXISTS (
    SELECT 1 FROM project_tasks
    WHERE project_tasks.tenant_id = sqlc.arg('tenant_id')::uuid
      AND project_tasks.project_id = sqlc.arg('project_id')::uuid
      AND project_tasks.id = sqlc.arg('revision_task_id')::uuid
  )
RETURNING *;

-- name: CreateProjectDemandSummary :one
INSERT INTO project_demand_summaries (
    tenant_id, project_id, demand_id, status, conclusion, summary_payload,
    report_ref_id, acceptance_required, idempotency_key, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('status')::varchar,
    sqlc.arg('conclusion')::text,
    sqlc.arg('summary_payload')::jsonb,
    sqlc.narg('report_ref_id')::uuid,
    sqlc.arg('acceptance_required')::boolean,
    sqlc.arg('idempotency_key')::varchar,
    sqlc.narg('created_event_id')::uuid
) ON CONFLICT (tenant_id, demand_id, idempotency_key)
DO UPDATE SET updated_at = project_demand_summaries.updated_at
RETURNING *;

-- name: GetLatestProjectDemandSummary :one
SELECT * FROM project_demand_summaries
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateProjectTaskAttemptContextUpdate :one
INSERT INTO project_task_attempt_context_updates (
    id,
    tenant_id,
    project_task_id,
    attempt_id,
    update_kind,
    payload,
    delivery_mode,
    created_event_id
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.narg('attempt_id')::uuid,
    sqlc.arg('update_kind')::varchar,
    sqlc.arg('payload')::jsonb,
    sqlc.arg('delivery_mode')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: CreateProjectTaskDispatchGateResult :one
INSERT INTO project_task_dispatch_gate_results (
    id,
    tenant_id,
    project_id,
    project_task_id,
    accepted_plan_revision_id,
    planned_task_key,
    selected_employee_id,
    attempt_no,
    dispatch_reason,
    idempotency_key,
    dispatch_token,
    status,
    checked_at,
    checks,
    blockers,
    human_action_request,
    retry_after,
    created_event_id
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.narg('accepted_plan_revision_id')::uuid,
    sqlc.narg('planned_task_key')::varchar,
    sqlc.arg('selected_employee_id')::uuid,
    sqlc.arg('attempt_no')::integer,
    sqlc.arg('dispatch_reason')::varchar,
    sqlc.arg('idempotency_key')::varchar,
    sqlc.arg('dispatch_token')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.arg('checked_at')::timestamptz,
    sqlc.arg('checks')::jsonb,
    sqlc.arg('blockers')::jsonb,
    sqlc.arg('human_action_request')::jsonb,
    sqlc.narg('retry_after')::timestamptz,
    sqlc.narg('created_event_id')::uuid
)
ON CONFLICT (tenant_id, project_task_id, idempotency_key)
DO UPDATE SET
    status = EXCLUDED.status,
    checked_at = EXCLUDED.checked_at,
    checks = EXCLUDED.checks,
    blockers = EXCLUDED.blockers,
    human_action_request = EXCLUDED.human_action_request,
    retry_after = EXCLUDED.retry_after,
    created_event_id = COALESCE(project_task_dispatch_gate_results.created_event_id, EXCLUDED.created_event_id),
    updated_at = NOW()
WHERE project_task_dispatch_gate_results.attempt_id IS NULL
  AND (
    project_task_dispatch_gate_results.decision_request_id IS NULL
    OR (
      project_task_dispatch_gate_results.status = 'waiting_human'
      AND EXCLUDED.status IN ('passed', 'blocked', 'retry_later', 'replan_required')
    )
  )
RETURNING *;

-- name: GetProjectTaskDispatchGateResult :one
SELECT * FROM project_task_dispatch_gate_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetProjectTaskDispatchGateResultByKey :one
SELECT * FROM project_task_dispatch_gate_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND idempotency_key = sqlc.arg('idempotency_key')::varchar;

-- name: ListProjectTaskDispatchGateResults :many
SELECT * FROM project_task_dispatch_gate_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit')::integer
OFFSET sqlc.arg('offset')::integer;

-- name: LinkProjectTaskDispatchGateAttempt :one
UPDATE project_task_dispatch_gate_results
SET attempt_id = sqlc.arg('attempt_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND (attempt_id IS NULL OR attempt_id = sqlc.arg('attempt_id')::uuid)
RETURNING *;

-- name: LinkProjectTaskDispatchGateDecisionRequest :one
UPDATE project_task_dispatch_gate_results
SET decision_request_id = sqlc.arg('decision_request_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND (decision_request_id IS NULL OR decision_request_id = sqlc.arg('decision_request_id')::uuid)
RETURNING *;

-- name: MarkProjectTaskLatestDispatchGate :one
UPDATE project_tasks
SET latest_dispatch_gate_result_id = sqlc.arg('latest_dispatch_gate_result_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: MovePlannedProjectTaskToWaitingHumanForGate :one
UPDATE project_tasks
SET status = 'waiting_human',
    waiting_reason = sqlc.arg('waiting_reason')::varchar,
    waiting_request_id = sqlc.narg('waiting_request_id')::uuid,
    latest_dispatch_gate_result_id = sqlc.arg('latest_dispatch_gate_result_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
RETURNING *;

-- name: SetProjectDecisionRequestDispatchGate :one
UPDATE project_decision_requests
SET dispatch_gate_result_id = sqlc.arg('dispatch_gate_result_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND (dispatch_gate_result_id IS NULL OR dispatch_gate_result_id = sqlc.arg('dispatch_gate_result_id')::uuid)
RETURNING *;
