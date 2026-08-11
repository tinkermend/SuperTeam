-- name: CreateProject :one
INSERT INTO projects (
    id,
    tenant_id,
    team_id,
    name,
    directory_name,
    description,
    goal,
    status,
    human_owner_user_id,
    human_owner_user_ids,
    coordination_workflow_id,
    coordination_status,
    coordination_policy,
    repo_url,
    repo_default_branch,
    repo_git_credential_ref,
    repo_scope,
    repo_binding_status,
    scenario_template_key,
    workspace_ready_status,
    workspace_ready_at
) VALUES (
    sqlc.arg('id')::uuid,
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('name')::varchar,
    sqlc.arg('directory_name')::varchar,
    sqlc.narg('description')::text,
    sqlc.narg('goal')::text,
    sqlc.arg('status')::varchar,
    sqlc.arg('human_owner_user_id')::uuid,
    sqlc.arg('human_owner_user_ids')::uuid[],
    sqlc.narg('coordination_workflow_id')::varchar,
    sqlc.narg('coordination_status')::varchar,
    COALESCE(sqlc.narg('coordination_policy')::jsonb, '{}'::jsonb),
    sqlc.narg('repo_url')::text,
    sqlc.narg('repo_default_branch')::varchar,
    sqlc.narg('repo_git_credential_ref')::varchar,
    COALESCE(sqlc.narg('repo_scope')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('repo_binding_status')::varchar, 'unbound'),
    sqlc.narg('scenario_template_key')::text,
    COALESCE(sqlc.narg('workspace_ready_status')::varchar, 'ready'),
    sqlc.narg('workspace_ready_at')::timestamptz
) RETURNING *;

-- name: GetProject :one
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: SetProjectHumanOwners :exec
-- 多负责人:成员变更后按 owner 角色人类成员重同步负责人集合(数组权威,scalar=首个过渡镜像)。
UPDATE projects
SET human_owner_user_ids = sqlc.arg('human_owner_user_ids')::uuid[],
    human_owner_user_id = sqlc.arg('human_owner_user_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL;

-- name: ListProjects :many
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (sqlc.narg('status')::varchar IS NULL OR status = sqlc.narg('status')::varchar)
  AND (
    sqlc.narg('q')::text IS NULL
    OR name ILIKE '%' || sqlc.narg('q')::text || '%'
    OR directory_name ILIKE '%' || sqlc.narg('q')::text || '%'
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
        p.archived_at AS project_archived_at,
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
      AND p.deleted_at IS NULL
      AND (sqlc.narg('project_id')::uuid IS NULL OR d.project_id = sqlc.narg('project_id')::uuid)
      AND (
        sqlc.narg('q')::text IS NULL
        OR d.title ILIKE '%' || sqlc.narg('q')::text || '%'
        OR COALESCE(d.content, '') ILIKE '%' || sqlc.narg('q')::text || '%'
        OR p.name ILIKE '%' || sqlc.narg('q')::text || '%'
      )
      AND (
        sqlc.arg('actor_user_id')::uuid = ANY(p.human_owner_user_ids)
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
        -- F5(§5.6): requires_human_approval 是粘性标志,任务终态后仍为 true。只有当任务
        -- 尚未到终态时它才代表真的"等待人工";否则(如 completed 但 requires_human_approval=t)
        -- 会把早已完成的实例误判成 waiting_human。
        COUNT(*) FILTER (
          WHERE status IN ('waiting_human', 'pending_review')
             OR (requires_human_approval AND status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed'))
        )::int AS waiting_human_nodes,
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
        -- F5(§5.6): 终态任务的粘性 requires_human_approval 不再当作 blocker,否则已完成
        -- 但曾 requires_human_approval 的任务会永久制造假"等待人工"blocker。failed/blocked
        -- 仍是真实 blocker,由下面 status IN 覆盖。
        status IN ('waiting_human', 'pending_review', 'failed', 'blocked')
        OR (requires_human_approval AND status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed'))
      )
    ORDER BY
        tenant_id,
        project_id,
        demand_id,
        CASE
          WHEN status IN ('waiting_human', 'pending_review')
            OR (requires_human_approval AND status NOT IN ('completed', 'done', 'success', 'cancelled', 'failed')) THEN 1
          ELSE 2
        END ASC,
        updated_at DESC,
        id DESC
),
instances AS (
SELECT
    vd.demand_id,
    vd.project_id,
    vd.project_name,
    vd.title,
    vd.submitted_by_user_id,
    -- 提交人补名三级优先: source_refs 里的名(飞书等外部通道写入)最高, 其次 auth_users
    -- 的 display_name/username(Web 提交的需求不带 source_refs 名), UUID 仅作最终兜底
    -- (CLAUDE.md: 不得裸 UUID, 名称由服务端读路径补名)。
    COALESCE(
      NULLIF(vd.source_refs->>'submitted_by_display_name', ''),
      NULLIF(submitter.display_name, ''),
      NULLIF(submitter.username, ''),
      vd.submitted_by_user_id::text
    )::text AS submitted_by_display_name,
    CASE
      -- F5(§5.6): demand_status 优先。收敛闸挂起的需求(acceptance_pending)即便所有任务已
      -- completed 也必须显示为 waiting_human/待验收,不能被任务计数判成 completed 而从运行视图消失;
      -- 规划失败(planning_failed,§5.5)显示为 failed。其余状态维持任务推导。
      WHEN vd.demand_status = 'acceptance_pending' THEN 'waiting_human'
      WHEN vd.demand_status = 'planning_failed' THEN 'failed'
      WHEN COALESCE(dc.pending_decisions, 0) > 0 OR COALESCE(tc.waiting_human_nodes, 0) > 0 THEN 'waiting_human'
      WHEN COALESCE(tc.failed_nodes, 0) > 0 THEN 'failed'
      WHEN COALESCE(tc.running_nodes, 0) > 0 THEN 'running'
      WHEN COALESCE(tc.cancelled_nodes, 0) > 0 OR vd.demand_status = 'cancelled' THEN 'cancelled'
      WHEN COALESCE(tc.total_nodes, 0) = 0 THEN 'planning'
      WHEN tc.completed_nodes = tc.total_nodes THEN 'completed'
      ELSE 'unknown'
    END::text AS status,
    CASE
      WHEN vd.demand_status = 'acceptance_pending' THEN '待验收'
      WHEN vd.demand_status = 'planning_failed' THEN '规划失败'
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
    COALESCE(db.blocker_resource_id, tb.blocker_resource_id, '00000000-0000-0000-0000-000000000000'::uuid) AS current_blocker_resource_id,
    vd.project_archived_at
FROM demand_read_model vd
LEFT JOIN auth_users submitter
  ON submitter.id = vd.submitted_by_user_id
 AND submitter.deleted_at IS NULL
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
)
-- scope 语义（默认 active）：
--   active   = 未归档项目 且 非终态实例（排除 completed/cancelled；failed 仍属"需要介入"保留在运行视图）
--   archived = 已归档项目 或 终态实例（completed/cancelled），供"已归档/已完成"页签回看
--   all      = 不过滤（调试/兜底）
SELECT *
FROM instances
WHERE (
    sqlc.arg('scope')::text = 'all'
    OR (
      sqlc.arg('scope')::text = 'active'
      AND project_archived_at IS NULL
      AND status NOT IN ('completed', 'cancelled')
    )
    OR (
      sqlc.arg('scope')::text = 'archived'
      AND (project_archived_at IS NOT NULL OR status IN ('completed', 'cancelled'))
    )
)
ORDER BY
    CASE status
      WHEN 'waiting_human' THEN 1
      WHEN 'failed' THEN 2
      WHEN 'running' THEN 3
      WHEN 'cancelled' THEN 6
      WHEN 'completed' THEN 5
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
    human_owner_user_ids = COALESCE(sqlc.narg('human_owner_user_ids')::uuid[], human_owner_user_ids),
    coordination_policy = COALESCE(sqlc.narg('coordination_policy')::jsonb, coordination_policy),
    repo_url = CASE
        WHEN sqlc.narg('repo_binding_status')::varchar IS NULL THEN repo_url
        WHEN sqlc.narg('repo_binding_status')::varchar = 'unbound' THEN NULL
        ELSE sqlc.narg('repo_url')::text
    END,
    repo_default_branch = CASE
        WHEN sqlc.narg('repo_binding_status')::varchar IS NULL THEN repo_default_branch
        WHEN sqlc.narg('repo_binding_status')::varchar = 'unbound' THEN NULL
        ELSE sqlc.narg('repo_default_branch')::varchar
    END,
    repo_git_credential_ref = CASE
        WHEN sqlc.narg('repo_binding_status')::varchar IS NULL THEN repo_git_credential_ref
        WHEN sqlc.narg('repo_binding_status')::varchar = 'unbound' THEN NULL
        ELSE sqlc.narg('repo_git_credential_ref')::varchar
    END,
    repo_scope = CASE
        WHEN sqlc.narg('repo_binding_status')::varchar IS NULL THEN repo_scope
        WHEN sqlc.narg('repo_binding_status')::varchar = 'unbound' THEN '[]'::jsonb
        ELSE COALESCE(sqlc.narg('repo_scope')::jsonb, '[]'::jsonb)
    END,
    repo_binding_status = COALESCE(sqlc.narg('repo_binding_status')::varchar, repo_binding_status),
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

-- name: UnarchiveProject :one
-- 归档回拨：仅 status=archived 的行生效；清 archived_at，回到 running（「已就绪」）。
UPDATE projects
SET status = 'running',
    archived_at = NULL,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'archived'
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
  AND (
    COALESCE(sqlc.narg('include_dismissed')::boolean, false) = true
    OR dismissed_at IS NULL
  )
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetProjectTaskStatusCounts :one
-- 项目概览任务计数：必须走全表聚合，不能在 ListProjectTasks 的分页片上循环统计
-- （原实现在"最近更新的 20 条"上数数，任务超过 20 条即漏计，且窗口随更新漂移会让
-- 计数非单调抖动）。dismissed 任务与 ListProjectTasks 默认窗口同样排除。
--
-- ActiveTasks 闸门口径冻结（spec 2026-08-11 portfolio §5.2.1）：
--   status NOT IN ('completed','done','success','failed','cancelled')
-- blocked 与 error 在此定义下算活跃——这是归档闸门既有行为，不得改成展示桶之和。
--
-- 展示桶与 ActiveTasks 并列、互不派生。
-- 展示桶单一事实源：project_task_portfolio_bucket()（迁移 20260811180000）
-- 与 Go ClassifyProjectTaskPortfolioBucket 同源。展示 failed 仅 failed/error；
-- ListProjectRunSummaries.failed_count 仍含 blocked（运行总览宽失败）。
WITH task_rows AS (
  SELECT
    status,
    project_task_portfolio_bucket(status, requires_human_approval) AS portfolio_bucket
  FROM project_tasks
  WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    AND project_id = sqlc.arg('project_id')::uuid
    AND dismissed_at IS NULL
)
SELECT
    COUNT(*)::integer AS total_tasks,
    -- GATE (frozen): do not rewrite as non-terminal display-bucket sum.
    COUNT(*) FILTER (
        WHERE status NOT IN ('completed', 'done', 'success', 'failed', 'cancelled')
    )::integer AS active_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'pending')::integer AS pending_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'queued')::integer AS queued_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'running')::integer AS running_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'waiting_human')::integer AS pending_human_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'blocked')::integer AS blocked_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'failed')::integer AS failed_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'completed')::integer AS completed_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'cancelled')::integer AS cancelled_tasks,
    COUNT(*) FILTER (WHERE portfolio_bucket = 'other')::integer AS other_tasks
FROM task_rows;

-- name: ListDemandLaunchProjectTasks :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND dismissed_at IS NULL
ORDER BY updated_at DESC
LIMIT sqlc.arg('limit');

-- name: ListProjectTasksByDemand :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND dismissed_at IS NULL
ORDER BY stage_index ASC NULLS LAST, created_at ASC;

-- name: DismissProjectTask :one
-- 仅终态 failed/cancelled、尚未了结、且无 pending/requested 决策时可清理。
UPDATE project_tasks pt
SET dismissed_at = NOW(),
    dismissed_by = sqlc.arg('dismissed_by')::uuid,
    updated_at = NOW()
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.project_id = sqlc.arg('project_id')::uuid
  AND pt.id = sqlc.arg('id')::uuid
  AND pt.status IN ('failed', 'cancelled')
  AND pt.dismissed_at IS NULL
  AND NOT EXISTS (
    SELECT 1
    FROM project_decision_requests pdr
    WHERE pdr.tenant_id = pt.tenant_id
      AND pdr.project_task_id = pt.id
      AND COALESCE(pdr.status_snapshot, '') IN ('pending', 'requested', 'waiting', 'open')
  )
RETURNING pt.*;

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
    planner_metadata,
    plan_iteration,
    max_attempts
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
    COALESCE(sqlc.narg('planner_metadata')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('plan_iteration')::integer, 0),
    sqlc.arg('max_attempts')::integer
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
    created_event_id,
    coordination_mode,
    scenario_template_key,
    continues_demand_id
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
    sqlc.narg('created_event_id')::uuid,
    sqlc.arg('coordination_mode')::varchar,
    sqlc.narg('scenario_template_key')::text,
    sqlc.narg('continues_demand_id')::uuid
) RETURNING *;

-- name: ListProjectDemandContinuationChain :many
-- 一条接续链的全部 demand，时间正序（链头在前）。从任意成员出发都返回同一条链：
-- 先沿 continues_demand_id 上溯到链头，再从链头向下展开后代。
-- depth 上限由调用方传入（spec §5.2 D3：数据被手工改出环时必须能停）。
WITH RECURSIVE ancestors AS (
    SELECT d.id, d.continues_demand_id, 0 AS depth
    FROM project_demands d
    WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
      AND d.id = sqlc.arg('demand_id')::uuid
    UNION ALL
    SELECT parent.id, parent.continues_demand_id, a.depth + 1
    FROM project_demands parent
    JOIN ancestors a ON parent.id = a.continues_demand_id
    WHERE parent.tenant_id = sqlc.arg('tenant_id')::uuid
      AND a.depth < sqlc.arg('max_depth')::int
), head AS (
    SELECT id FROM ancestors ORDER BY depth DESC LIMIT 1
), chain AS (
    SELECT d.id, 0 AS depth
    FROM project_demands d
    JOIN head h ON h.id = d.id
    WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
    UNION ALL
    SELECT child.id, c.depth + 1
    FROM project_demands child
    JOIN chain c ON child.continues_demand_id = c.id
    WHERE child.tenant_id = sqlc.arg('tenant_id')::uuid
      AND c.depth < sqlc.arg('max_depth')::int
)
SELECT d.*, c.depth AS chain_depth
FROM project_demands d
JOIN chain c ON c.id = d.id
ORDER BY c.depth ASC, d.created_at ASC;

-- name: CountProjectDemandContinuationDepth :one
-- 从该 demand 上溯到链头的代数（链头返回 0）。写入期用它拦"链太深"。
WITH RECURSIVE ancestors AS (
    SELECT d.id, d.continues_demand_id, 0 AS depth
    FROM project_demands d
    WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
      AND d.id = sqlc.arg('demand_id')::uuid
    UNION ALL
    SELECT parent.id, parent.continues_demand_id, a.depth + 1
    FROM project_demands parent
    JOIN ancestors a ON parent.id = a.continues_demand_id
    WHERE parent.tenant_id = sqlc.arg('tenant_id')::uuid
      AND a.depth < sqlc.arg('max_depth')::int
)
SELECT COALESCE(MAX(depth), 0)::int AS depth FROM ancestors;

-- name: ResolveTaskLineageRootFromDemandChain :one
-- 接续场景的会话血缘根（spec §5.1 第 3 条 / §5.2 D1–D7）：
-- 从本任务所属 demand 沿 continues_demand_id **逐代上溯**，在每一代里找同一个
-- digital_employee 的任务，取最近一条的血缘根。
--
-- D1 排序钉死：先按代数（距离近的一代优先），同代内 created_at DESC, id DESC。
-- D2 逐代：靠 depth 排序天然实现，不会跳过中间代。
-- D3 depth 上限由调用方传入，成环也能停。
-- D4 返回的是那条任务自己的**根**（planner_metadata > revision_of > 自身 id），
--    不是它的 id —— 会话是按根存的。
-- D5 employee 完全相等才匹配；换人查不到，调用方落回任务自身 id。
-- D6 整个上溯是这一条查询，不在 Go 里循环往返。
-- D7 未定人的任务不参与匹配（assigned_digital_employee_id IS NOT NULL）。
WITH RECURSIVE ancestors AS (
    SELECT d.continues_demand_id AS demand_id, 1 AS depth
    FROM project_demands d
    WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
      AND d.id = sqlc.arg('demand_id')::uuid
      AND d.continues_demand_id IS NOT NULL
    UNION ALL
    SELECT parent.continues_demand_id, a.depth + 1
    FROM project_demands parent
    JOIN ancestors a ON parent.id = a.demand_id
    WHERE parent.tenant_id = sqlc.arg('tenant_id')::uuid
      AND parent.continues_demand_id IS NOT NULL
      AND a.depth < sqlc.arg('max_depth')::int
)
SELECT COALESCE(
    NULLIF(t.planner_metadata->>'revision_root_task_id', ''),
    NULLIF(t.revision_of_task_id::text, ''),
    t.id::text
)::uuid AS root_task_id
FROM ancestors a
JOIN project_tasks t
  ON t.demand_id = a.demand_id
 AND t.tenant_id = sqlc.arg('tenant_id')::uuid
 AND t.assigned_digital_employee_id = sqlc.arg('digital_employee_id')::uuid
WHERE t.assigned_digital_employee_id IS NOT NULL
ORDER BY a.depth ASC, t.created_at DESC, t.id DESC
LIMIT 1;

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

-- name: GetProjectTaskSessionLineage :one
-- Minimal projection for resolving a task's session-lineage root (see
-- employee.PgRunRepository.ResolveProjectTaskLineageRoot): only the two
-- fields that participate in root resolution, not the full row.
SELECT revision_of_task_id, planner_metadata, demand_id
FROM project_tasks
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

-- name: GetProjectRouteDecision :one
SELECT * FROM project_route_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

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
    b.status AS blocker_status,
    b.latest_task_result_id,
    latest_result.result_status AS latest_result_status,
    latest_result.decision AS latest_result_decision,
    latest_result.validation_status AS latest_result_validation_status,
    CASE
        WHEN b.status = 'completed'
         AND latest_result.id IS NOT NULL
         AND latest_result.result_status = 'completed'
         AND latest_result.decision = 'complete_accepted'
         AND latest_result.validation_status = 'accepted'
        THEN true
        ELSE false
    END AS acceptance_satisfied
FROM project_task_dependencies d
JOIN project_tasks b
  ON b.tenant_id = d.tenant_id
 AND b.id = d.blocker_task_id
LEFT JOIN project_task_results latest_result
  ON latest_result.tenant_id = b.tenant_id
 AND latest_result.project_id = b.project_id
 AND latest_result.project_task_id = b.id
 AND latest_result.id = b.latest_task_result_id
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.project_id = sqlc.arg('project_id')::uuid
  AND d.dependent_task_id = ANY(sqlc.arg('dependent_task_ids')::uuid[])
  AND NOT (
        b.status = 'completed'
        AND latest_result.id IS NOT NULL
        AND latest_result.result_status = 'completed'
        AND latest_result.decision = 'complete_accepted'
        AND latest_result.validation_status = 'accepted'
    )
ORDER BY d.dependent_task_id, d.created_at ASC;

-- name: ListProjectTasksByCoordinationJob :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND coordination_job_id = sqlc.arg('coordination_job_id')::uuid
  AND dismissed_at IS NULL
ORDER BY stage_index ASC NULLS LAST, created_at ASC;

-- name: ListProjectTasksByAcceptedPlanRevision :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND accepted_plan_revision_id = sqlc.arg('accepted_plan_revision_id')::uuid
  AND dismissed_at IS NULL
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
    review_reason,
    created_event_id,
    coordination_mode
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
    sqlc.narg('review_reason')::text,
    sqlc.narg('created_event_id')::uuid,
    sqlc.narg('coordination_mode')::varchar
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
ORDER BY demand_id ASC, revision_number ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListProjectPlanRevisionsForDemand :many
SELECT * FROM project_plan_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
ORDER BY revision_number ASC;

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

-- name: CancelOpenPlanReviewDecisionsForDemandExceptRevision :many
-- When a newer plan supersedes open revisions (casting expansion / replan),
-- cancel plan_review decisions still pointing at those superseded revisions so
-- humans cannot approve a dead plan (Accept would 409 and strand the demand).
UPDATE project_decision_requests pdr
SET status_snapshot = 'cancelled',
    resolved_at = COALESCE(pdr.resolved_at, NOW()),
    updated_at = NOW()
FROM project_plan_revisions ppr
WHERE pdr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pdr.project_id = sqlc.arg('project_id')::uuid
  AND pdr.plan_revision_id = ppr.id
  AND ppr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND ppr.project_id = sqlc.arg('project_id')::uuid
  AND ppr.demand_id = sqlc.arg('demand_id')::uuid
  AND ppr.id <> sqlc.arg('except_revision_id')::uuid
  AND pdr.decision_type = 'plan_review'
  AND pdr.status_snapshot IN ('pending', 'requested')
RETURNING pdr.id, pdr.tenant_id, pdr.project_id, pdr.approval_request_id, pdr.coordination_job_id, pdr.project_task_id, pdr.plan_revision_id, pdr.dispatch_gate_result_id, pdr.target_user_id, pdr.decision_type, pdr.title_snapshot, pdr.summary_snapshot, pdr.risk_level_snapshot, pdr.status_snapshot, pdr.created_event_id, pdr.resolved_event_id, pdr.created_at, pdr.updated_at, pdr.resolved_at;

-- name: CancelApprovalRequestsByIDs :exec
UPDATE approval_requests
SET status = 'cancelled',
    resolved_at = COALESCE(resolved_at, NOW()),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = ANY (sqlc.arg('ids')::uuid[])
  AND status = 'pending';

-- name: SupersedeCurrentAcceptedProjectPlanRevisions :exec
-- Clears the partial unique index uq_project_plan_revisions_current_accepted so a
-- newer pending_review revision can be accepted (casting-expansion replan / request_changes).
UPDATE project_plan_revisions
SET status = 'superseded',
    superseded_by_revision_id = sqlc.arg('superseded_by_revision_id')::uuid,
    rejection_reason = sqlc.narg('reason')::text,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
  AND id <> sqlc.arg('superseded_by_revision_id')::uuid
  AND status IN ('accepted', 'decomposing', 'decomposed');

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
    digital_employee_id,
    provider_type,
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
    sqlc.narg('digital_employee_id')::uuid,
    sqlc.narg('provider_type')::varchar,
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

-- name: BindProjectTaskAttemptRun :one
UPDATE project_task_attempts
SET digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid,
    runtime_task_id = sqlc.arg('runtime_task_id')::uuid,
    runtime_node_id = sqlc.arg('runtime_node_id')::uuid,
    provider_type = sqlc.arg('provider_type')::varchar,
    execution_context_packet = sqlc.arg('execution_context_packet')::jsonb,
    execution_context_packet_version = sqlc.arg('execution_context_packet_version')::varchar,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'queued'
  AND (digital_employee_run_id IS NULL OR digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid)
  AND (runtime_task_id IS NULL OR runtime_task_id = sqlc.arg('runtime_task_id')::uuid)
  AND (runtime_node_id IS NULL OR runtime_node_id = sqlc.arg('runtime_node_id')::uuid)
  AND (provider_type IS NULL OR provider_type = sqlc.arg('provider_type')::varchar)
RETURNING *;

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

-- name: BindQueuedProjectTaskRun :one
UPDATE project_tasks
SET runtime_task_id = sqlc.arg('runtime_task_id')::uuid,
    digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND current_attempt_id = sqlc.arg('current_attempt_id')::uuid
  AND status = 'queued'
  AND (runtime_task_id IS NULL OR runtime_task_id = sqlc.arg('runtime_task_id')::uuid)
  AND (digital_employee_run_id IS NULL OR digital_employee_run_id = sqlc.arg('digital_employee_run_id')::uuid)
RETURNING *;

-- name: MarkQueuedProjectTaskAttemptDispatchStartFailed :one
UPDATE project_task_attempts
SET status = sqlc.arg('status')::varchar,
    finished_at = NOW(),
    retryable = sqlc.arg('retryable')::boolean,
    failure_family = sqlc.arg('failure_family')::varchar,
    failure_message = sqlc.arg('failure_message')::text,
    error_code = sqlc.narg('error_code')::varchar,
    terminal_event_id = sqlc.narg('terminal_event_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status = 'queued'
RETURNING *;

-- name: RestoreProjectTaskAfterDispatchStartFailure :one
UPDATE project_tasks
SET status = sqlc.arg('status')::varchar,
    current_attempt_id = CASE
        WHEN sqlc.arg('clear_current_attempt')::boolean THEN NULL
        ELSE current_attempt_id
    END,
    retry_not_before = sqlc.narg('retry_not_before')::timestamptz,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND current_attempt_id = sqlc.arg('current_attempt_id')::uuid
  AND status = 'queued'
RETURNING *;

-- name: ScheduleProjectTaskRetry :one
-- Clear prior dispatch identity so the coordinator re-enters StartProjectTaskRun
-- for the new attempt (see projectTaskQueuedWithoutRunBinding). Keeping the old
-- runtime_task_id/digital_employee_run_id short-circuits Dispatch as "already dispatched".
UPDATE project_tasks
SET status = 'queued',
    current_attempt_id = sqlc.arg('current_attempt_id')::uuid,
    runtime_task_id = NULL,
    digital_employee_run_id = NULL,
    attempt_count = attempt_count + 1,
    retry_not_before = sqlc.narg('retry_not_before')::timestamptz,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('queued', 'running', 'waiting_human')
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

-- name: GetProjectTaskLatestDispatchFailureEvent :one
SELECT *
FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND event_type = 'project_task.dispatch_failed'
  AND actor_id = sqlc.arg('project_task_id')::uuid::text
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: CountProjectTaskDispatchFailureEvents :one
SELECT COUNT(*)
FROM project_events
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND event_type = 'project_task.dispatch_failed'
  AND actor_id = sqlc.arg('project_task_id')::uuid::text;

-- name: ScheduleProjectTaskDispatchRetry :one
UPDATE project_tasks
SET status = 'planned',
    retry_not_before = sqlc.arg('retry_not_before')::timestamptz,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
  AND current_attempt_id IS NULL
RETURNING *;

-- name: MoveProjectTaskDispatchFailureToWaitingHuman :one
UPDATE project_tasks
SET status = 'waiting_human',
    waiting_reason = sqlc.arg('waiting_reason')::varchar,
    waiting_request_id = sqlc.narg('waiting_request_id')::uuid,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status IN ('planned', 'waiting_human')
  AND current_attempt_id IS NULL
RETURNING *;

-- name: ListStaleQueuedProjectTaskAttempts :many
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt ON pt.tenant_id = pta.tenant_id AND pt.id = pta.project_task_id
WHERE pta.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pta.status = 'queued'
  AND pt.status = 'queued'
  AND pta.started_at IS NULL
  AND pta.created_at < sqlc.arg('started_before')::timestamptz
ORDER BY pta.created_at ASC
LIMIT sqlc.arg('limit')::integer;

-- name: ListExpiredRunningProjectTaskAttempts :many
SELECT pta.*
FROM project_task_attempts pta
JOIN project_tasks pt ON pt.tenant_id = pta.tenant_id AND pt.id = pta.project_task_id
WHERE pta.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pta.status = 'running'
  AND pt.status = 'running'
  AND pta.lease_expires_at IS NOT NULL
  AND pta.lease_expires_at < sqlc.arg('now')::timestamptz
ORDER BY pta.lease_expires_at ASC
LIMIT sqlc.arg('limit')::integer;

-- name: ListStuckOrphanProjectTasks :many
-- 僵尸/孤儿任务:停留在 running/in_progress 但没有当前活跃 attempt(current_attempt_id
-- 为空),且已滞留超过阈值。正常派发会在秒级内建 attempt 并回填 current_attempt_id,
-- 故长时间 running 而无 attempt 的任务只可能是 runtime 整体失联未落 attempt、协调线程
-- 死亡、或异常/夹具数据直插。跨租户列出交由看门狗逐条系统收敛(置 failed + 发失败信号,
-- 由协调线程开失败恢复决策卡)。阈值(stale_before)由调用方按系统配置 task.stuck_running_timeout 计算。
SELECT * FROM project_tasks
WHERE status IN ('running', 'in_progress')
  AND current_attempt_id IS NULL
  AND updated_at < sqlc.arg('stale_before')::timestamptz
ORDER BY updated_at ASC
LIMIT sqlc.arg('batch_limit')::integer;

-- name: ListOrphanWaitingHumanProjectTasks :many
-- waiting_human 且 waiting_request_id 为空，或指向的决策已非 open。
-- 看门狗：若任务上另有 open decision 则只补绑指针；否则补建决策卡。
-- 例外（spec 2026-08-11）：仍处「预检闸审批形态」的任务交给下面的 zombie 扫描 heal，
-- 不得当「缺卡」进补建列表。两个列表的条件严格互补，任务不会两边都落空。
SELECT t.*
FROM project_tasks t
WHERE t.status = 'waiting_human'
  AND t.dismissed_at IS NULL
  AND (
    t.waiting_request_id IS NULL
    OR NOT EXISTS (
      SELECT 1
      FROM project_decision_requests d
      WHERE d.tenant_id = t.tenant_id
        AND d.id = t.waiting_request_id
        AND lower(d.status_snapshot) IN ('pending', 'waiting', 'requested', 'open')
    )
  )
  AND NOT (
    EXISTS (
      SELECT 1
      FROM project_decision_requests g
      WHERE g.tenant_id = t.tenant_id
        AND g.project_id = t.project_id
        AND g.project_task_id = t.id
        AND g.decision_type = 'project_task_approval'
        AND lower(g.status_snapshot) = 'approved'
        AND g.dispatch_gate_result_id IS NOT NULL
    )
    AND (
      (t.waiting_request_id IS NULL AND t.waiting_reason = 'approval_required')
      OR EXISTS (
        SELECT 1
        FROM project_decision_requests w
        WHERE w.tenant_id = t.tenant_id
          AND w.id = t.waiting_request_id
          AND w.decision_type = 'project_task_approval'
      )
    )
  )
ORDER BY t.updated_at ASC
LIMIT sqlc.arg('batch_limit')::integer;

-- name: ListZombieGateApprovalWaitingHumanProjectTasks :many
-- waiting_human + 同 task 已有 approved gate 链接真卡，**且当前等待仍是预检闸审批形态**
-- （spec 2026-08-11 §4.2/§4.4）。两种入选形态：
--   a) 指针空 + waiting_reason=approval_required：批准后中间态，orphan 尚未抢走指针；
--   b) 指针指向 project_task_approval：零 approval 僵尸补建卡，或根因 A 的「指针挂已批真卡」。
-- 必须排除「已收敛到诚实恢复卡」（指针指 project_task_recovery/clarification 等）：那是
-- 人类的真实待办，heal 回 planned 只会撞回同一堵墙、再开一张新恢复卡并改挂指针，下一轮
-- 看门狗又扫到——每分钟自激一次的无限循环（2026-08-11 实测单任务积到 99 张卡）。
-- 收敛后自然离列：新指针指向非 approval 卡，形态判据不再命中。
SELECT t.*
FROM project_tasks t
WHERE t.status = 'waiting_human'
  AND t.dismissed_at IS NULL
  AND EXISTS (
    SELECT 1
    FROM project_decision_requests g
    WHERE g.tenant_id = t.tenant_id
      AND g.project_id = t.project_id
      AND g.project_task_id = t.id
      AND g.decision_type = 'project_task_approval'
      AND lower(g.status_snapshot) = 'approved'
      AND g.dispatch_gate_result_id IS NOT NULL
  )
  AND (
    (t.waiting_request_id IS NULL AND t.waiting_reason = 'approval_required')
    OR EXISTS (
      SELECT 1
      FROM project_decision_requests w
      WHERE w.tenant_id = t.tenant_id
        AND w.id = t.waiting_request_id
        AND w.decision_type = 'project_task_approval'
    )
  )
ORDER BY t.updated_at ASC
LIMIT sqlc.arg('batch_limit')::integer;

-- name: GetOpenProjectDecisionRequestByTask :one
SELECT *
FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
  AND lower(status_snapshot) IN ('pending', 'waiting', 'requested', 'open')
ORDER BY created_at DESC, id DESC
LIMIT 1;

-- name: BindProjectTaskWaitingRequest :one
-- 给已处于 waiting_human 的任务补挂/改挂 waiting_request_id（不改 status）。
UPDATE project_tasks
SET waiting_request_id = sqlc.arg('waiting_request_id')::uuid,
    waiting_reason = COALESCE(sqlc.narg('waiting_reason')::varchar, waiting_reason),
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'waiting_human'
RETURNING *;

-- name: ListTenantsWithRecoverableProjectTaskAttempts :many
-- 跨租户列出"存在可恢复卡死 attempt"的租户,供看门狗逐租户调用 per-tenant 的
-- SweepStaleQueuedProjectTaskAttempts / SweepExpiredRunningProjectTaskAttempts。
-- 阈值放宽以避免漏选(per-tenant sweep 内部再按精确阈值过滤,过选无害):
-- 只要有 queued attempt 未开始、或 running attempt 租约已过期即入选。
SELECT DISTINCT pta.tenant_id
FROM project_task_attempts pta
JOIN project_tasks pt ON pt.tenant_id = pta.tenant_id AND pt.id = pta.project_task_id
WHERE (pta.status = 'queued' AND pt.status = 'queued' AND pta.started_at IS NULL)
   OR (pta.status = 'running' AND pt.status = 'running'
       AND pta.lease_expires_at IS NOT NULL
       AND pta.lease_expires_at < sqlc.arg('now')::timestamptz);

-- name: StartProjectTaskAttempt :one
-- digital_employee_run_id 回填:dispatch 冲突路径可能留下 NULL 的 run 关联
-- (命令已送达但派发簿记失败),导致 provider 事件按 run_id 关联不到 attempt
-- 而静默不进 ledger(07-13 记档缺陷)。runtime 在 started 回写带 command_id,
-- 此处按 task_runs.command_id(全局唯一)补上缺失的关联。
UPDATE project_task_attempts
SET status = 'running',
    runtime_node_id = sqlc.arg('runtime_node_id')::uuid,
    provider_session_id = COALESCE(sqlc.narg('provider_session_id')::varchar, provider_session_id),
    digital_employee_run_id = COALESCE(
        digital_employee_run_id,
        (SELECT tr.id FROM task_runs tr
          WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
            AND tr.command_id = NULLIF(sqlc.narg('command_id')::varchar, ''))
    ),
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
    error_code = COALESCE(sqlc.narg('error_code')::varchar, error_code),
    terminal_event_id = sqlc.narg('terminal_event_id')::uuid,
    log_store = COALESCE(sqlc.narg('log_store')::varchar, log_store),
    log_ref = COALESCE(sqlc.narg('log_ref')::text, log_ref),
    log_bytes = COALESCE(sqlc.narg('log_bytes')::bigint, log_bytes),
    log_sha256 = COALESCE(sqlc.narg('log_sha256')::varchar, log_sha256),
    log_compressed = COALESCE(sqlc.narg('log_compressed')::boolean, log_compressed),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND lease_token = sqlc.arg('lease_token')::varchar
  AND status IN ('queued', 'running')
RETURNING *;

-- name: ReleaseProjectTaskWaitingHumanForRedispatch :one
-- 人类解决任务等待(批准继续)后,任务回到 planned 由协调线程走正常派发管线
-- (gate 评估 + run 启动 + attempt 创建)。直接由释放方创建 queued attempt 是
-- 死路:没有 run,runtime 永远不会来领,最终被 stale-queued 看门狗回收。
-- 旧执行的 run 绑定必须一并清除:DispatchProjectTask 对带 run 绑定且已有
-- dispatched 事件的任务按"已派发"幂等短路,残留绑定会让重派发静默 no-op。
UPDATE project_tasks
SET status = 'planned',
    current_attempt_id = NULL,
    digital_employee_run_id = NULL,
    runtime_task_id = NULL,
    retry_not_before = sqlc.arg('retry_not_before')::timestamptz,
    waiting_reason = NULL,
    waiting_request_id = NULL,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    status_changed_at = NOW(),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'waiting_human'
RETURNING *;

-- name: SupersedeWaitingHumanProjectTaskAttempt :execrows
-- 人类解决等待、任务转入重派发或终态时,旧 waiting_human attempt 必须先出让
-- 活跃位(uq_project_task_attempts_active 把 waiting_human 计入活跃),终态取
-- cancelled(被人类决策取代,词表内唯一贴切的终态)。attempt 已被其他恢复路径
-- 置为终态(如 lost)时命中 0 行,属合法情形,调用方不视为错误。
UPDATE project_task_attempts
SET status = 'cancelled',
    finished_at = COALESCE(finished_at, NOW()),
    failure_message = COALESCE(sqlc.narg('failure_message')::text, failure_message),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'waiting_human';

-- name: RestoreProjectTaskHumanWait :one
-- 人类验收写回的**补偿动作**：把任务从 completed 退回 waiting_human，并把
-- UpdateProjectTaskStatus 终态分支清掉的等待指针一并还原。
-- 必须还原指针，否则 ResolveProjectTaskHumanWait 的 approve 守卫
-- （waiting_reason 必须是 acceptance_required）会让重试永久 409：验收写回提交后、
-- 记录任务结果那几步（非同事务）若失败，任务会退回 waiting_human 但指针已空，
-- 人类再也点不动"验收通过"。补偿动作必须还原它清掉的每一样东西。
UPDATE project_tasks
SET status = 'waiting_human',
    waiting_reason = sqlc.narg('waiting_reason')::varchar,
    waiting_request_id = sqlc.narg('waiting_request_id')::uuid,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND status = 'completed'
RETURNING *;

-- name: UpdateProjectTaskStatus :one
-- 进终态时一并清掉等待指针：waiting_reason / waiting_request_id 只描述"当前在等什么"，
-- 四条"回活跃"的查询（QueueProjectTask / ScheduleProjectTaskRetry /
-- ScheduleProjectTaskDispatchRetry / ReleaseProjectTaskWaitingHumanForRedispatch）
-- 都会清它们，唯独终态这条不清，会让已完成/已取消的任务永久带着上一次等待的决策 id。
-- 人类决策溯源不依赖该列：结项摘要与执行上下文包都从 project_decision_requests 取。
UPDATE project_tasks
SET status = sqlc.arg('status')::varchar,
    latest_event_id = COALESCE(sqlc.narg('latest_event_id')::uuid, latest_event_id),
    terminal_event_id = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'failed', 'cancelled')
        THEN COALESCE(sqlc.narg('latest_event_id')::uuid, terminal_event_id)
        ELSE terminal_event_id
    END,
    waiting_reason = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'done', 'success', 'failed', 'cancelled')
        THEN NULL
        ELSE waiting_reason
    END,
    waiting_request_id = CASE
        WHEN sqlc.arg('status')::varchar IN ('completed', 'done', 'success', 'failed', 'cancelled')
        THEN NULL
        ELSE waiting_request_id
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

-- name: ListProjectDeclaredArtifactsByTaskIDs :many
-- 验收判据卡深链(声明式交付物 v2 §4 P2):按任务批量取 declared 交付物,
-- 只返回内容寻址(artifacts/ 前缀=可经平台取回)的行;declared_skipped
-- 与外部引用天然被排除,不下发不可点击的 chip。
SELECT * FROM project_artifact_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
  AND artifact_type = 'declared'
  AND object_ref LIKE 'artifacts/%'
ORDER BY created_at ASC;

-- name: ListProjectEvidenceRefsByTaskIDs :many
-- 一单卷宗右轨(spec 2026-07-29 R2 §5.3-5):按任务批量取证据,走
-- idx_project_evidence_refs_tenant_task。刻意不带 limit/offset——按项目分页
-- 再在内存里过滤会被分页截断,把"证据齐全"渲染成"交付缺失"(假阴性比不显示更坏)。
SELECT * FROM project_evidence_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
ORDER BY created_at DESC;

-- name: ListProjectArtifactRefsByTaskIDs :many
-- 同上,取本单任务的全部工件引用,走 idx_project_artifact_refs_tenant_task。
-- 刻意不套用 ListProjectDeclaredArtifactsByTaskIDs 的 declared + artifacts/
-- 前缀过滤:那是验收深链"可点击才下发"的口径;右轨要回答的是"这一单产出了
-- 什么",漏掉兜底附件与外部引用同样构成假阴性。
SELECT * FROM project_artifact_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
ORDER BY created_at DESC;

-- name: ListProjectTaskGraphNodeTimings :many
SELECT
  pta.project_task_id AS project_task_id,
  MIN(pta.started_at)::timestamptz AS started_at,
  MAX(pta.finished_at)::timestamptz AS finished_at
FROM project_task_attempts pta
WHERE pta.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pta.project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
GROUP BY pta.project_task_id;

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
    plan_revision_id,
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
    sqlc.narg('plan_revision_id')::uuid,
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

-- name: ExistsProjectDecisionRequestByApproval :one
-- §5.4.1: true when a project decision request already owns this approval —
-- ApprovalProjectorAdapter must not also project/overwrite the inbox card.
SELECT EXISTS(
  SELECT 1 FROM project_decision_requests
  WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    AND approval_request_id = sqlc.arg('approval_request_id')::uuid
);

-- name: GetProjectDecisionRequestByPlanRevision :one
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND plan_revision_id = sqlc.arg('plan_revision_id')::uuid
  AND decision_type = 'plan_review'
ORDER BY created_at DESC
LIMIT 1;

-- name: GetPendingDemandAcceptanceDecisionByPlanRevision :one
SELECT * FROM project_decision_requests
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND plan_revision_id = sqlc.arg('plan_revision_id')::uuid
  AND decision_type = 'demand_acceptance'
  AND status_snapshot = 'pending'
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
  AND status_snapshot IN ('pending', 'requested')
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
    revision_request, created_event_id
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
    sqlc.narg('created_event_id')::uuid
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
      AND project_decision_requests.project_task_id = project_task_results.project_task_id
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
      AND project_task_results.project_task_id = project_decision_requests.project_task_id
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
      AND project_tasks.revision_of_task_id = project_task_results.project_task_id
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

-- name: ListProjectTaskLatestDispatchGates :many
-- 每个任务只取**最新一条**闸门结果：闸门结果按 (task, idempotency_key=分派原因+尝试序号)
-- 唯一，重试重新评估会新增一行，因此最新一行就是当前闸门裁决。
-- 注意：不能改用 project_events 的闸门事件来判断"当前是否被闸住"——闸门事件按
-- (任务, 事件类型) 至多发一次（见 predispatch_gate.go 的 ProjectTaskEventExists
-- 去重），任务二次卡人工时不会有新事件，按事件推断会永远看不到第二次阻塞。
SELECT DISTINCT ON (project_task_id) *
FROM project_task_dispatch_gate_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = ANY(sqlc.arg('project_task_ids')::uuid[])
ORDER BY project_task_id, created_at DESC, id DESC;

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

-- name: GetProjectForDelete :one
SELECT * FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
FOR UPDATE;

-- name: SoftDeleteProject :one
UPDATE projects
SET deleted_at = COALESCE(deleted_at, sqlc.arg('deleted_at')::timestamptz),
    updated_at = sqlc.arg('deleted_at')::timestamptz,
    coordination_status = 'terminated'
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: SetProjectWorkspaceReady :one
-- CAS: ready 仅从 pending|error|ready；error 仅从 pending；pending 可从 pending|error|ready（reclone）。
UPDATE projects
SET workspace_ready_status = sqlc.arg('workspace_ready_status')::varchar,
    primary_runtime_node_id = sqlc.narg('primary_runtime_node_id')::uuid,
    workspace_ready_error = sqlc.narg('workspace_ready_error')::text,
    workspace_ready_at = CASE
        WHEN sqlc.arg('workspace_ready_status')::varchar = 'ready'
            THEN COALESCE(workspace_ready_at, NOW())
        ELSE workspace_ready_at
    END,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
  AND (
    CASE sqlc.arg('workspace_ready_status')::varchar
      WHEN 'ready' THEN workspace_ready_status IN ('pending', 'error', 'ready')
      WHEN 'error' THEN workspace_ready_status = 'pending'
      WHEN 'pending' THEN workspace_ready_status IN ('pending', 'error', 'ready')
      ELSE TRUE
    END
  )
RETURNING *;

-- name: ListProjectDeleteTaskBlockers :many
SELECT
    'project_task'::text AS blocker_type,
    pt.id,
    pt.status,
    (COALESCE(NULLIF(pt.title, ''), pt.id::text))::text AS title
FROM project_tasks pt
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.project_id = sqlc.arg('project_id')::uuid
  AND pt.status IN ('queued', 'running', 'in_progress')
ORDER BY pt.updated_at DESC
LIMIT 20;

-- name: ListProjectDeleteRunBlockers :many
SELECT
    'run'::text AS blocker_type,
    tr.id,
    tr.status,
    COALESCE(t.title, tr.task_id::text) AS title
FROM task_runs tr
INNER JOIN project_tasks pt
  ON pt.tenant_id = tr.tenant_id
 AND pt.digital_employee_run_id = tr.id
LEFT JOIN tasks t
  ON t.tenant_id = tr.tenant_id
 AND t.id = tr.task_id
WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pt.project_id = sqlc.arg('project_id')::uuid
  AND tr.status IN ('queued', 'dispatching', 'running', 'cancelling')
ORDER BY tr.updated_at DESC
LIMIT 20;

-- name: GetProjectDeletePreviewCounts :one
SELECT
  (SELECT COUNT(*)::int FROM project_decision_requests
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
      AND status_snapshot IN ('pending', 'requested')) AS pending_decision_count,
  (SELECT COUNT(*)::int FROM project_tasks
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
      AND status IN ('waiting_human', 'pending_review')) AS waiting_human_task_count,
  (SELECT COUNT(*)::int FROM inbox_items ii
    WHERE ii.tenant_id = sqlc.arg('tenant_id')::uuid
      AND ii.status = 'open'
      AND ii.source_project_id = sqlc.arg('project_id')::uuid) AS open_inbox_count,
  (SELECT COUNT(*)::int FROM project_members
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
      AND status = 'active') AS active_member_count,
  (SELECT COUNT(*)::int FROM project_members
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid
      AND status = 'active' AND principal_type = 'digital_employee') AS digital_employee_member_count,
  (SELECT COUNT(*)::int FROM project_runtime_nodes
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid) AS runtime_node_binding_count,
  (SELECT COUNT(*)::int FROM project_employee_node_affinity
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid) AS affinity_count,
  (SELECT COUNT(*)::int FROM automation_rules
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid AND project_id = sqlc.arg('project_id')::uuid) AS automation_rule_count;

-- name: DeactivateProjectMembersForDelete :many
UPDATE project_members
SET status = 'removed', updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND status = 'active'
RETURNING id;

-- name: CancelProjectTasksForDelete :many
-- Soft-delete cascade: cancel any task that could still light employee overview
-- blockers (active/waiting/failed). Keep completed/success/cancelled historical rows.
-- 与 UpdateProjectTaskStatus 的终态分支同口径：进终态即清等待指针，
-- 否则被级联取消的任务会永久带着上一次等待的决策 id。
UPDATE project_tasks
SET status = 'cancelled',
    waiting_reason = NULL,
    waiting_request_id = NULL,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND status NOT IN ('completed', 'cancelled', 'done', 'success')
RETURNING id;

-- name: AcknowledgeTaskRunsForProjectDelete :many
-- Soft-delete cascade: auto-ack failed/timed_out runs anchored to this project so
-- employee overview no longer stays in 异常 waiting for unreachable recovery.
UPDATE task_runs tr
SET failure_acknowledged_at = COALESCE(tr.failure_acknowledged_at, NOW()),
    failure_acknowledged_by = COALESCE(tr.failure_acknowledged_by, sqlc.narg('acknowledged_by')::uuid),
    updated_at = NOW()
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND tr.status IN ('failed', 'timed_out')
  AND tr.failure_acknowledged_at IS NULL
  AND tr.project_id = sqlc.arg('project_id')::uuid
RETURNING tr.id;

-- name: CancelProjectDecisionRequestsForDelete :many
UPDATE project_decision_requests
SET status_snapshot = 'cancelled',
    resolved_at = COALESCE(resolved_at, NOW()),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND status_snapshot IN ('pending', 'requested')
RETURNING id;

-- name: CancelApprovalRequestsForProjectDelete :many
UPDATE approval_requests ar
SET status = 'cancelled',
    resolved_at = COALESCE(ar.resolved_at, NOW()),
    updated_at = NOW()
FROM project_decision_requests pdr
WHERE ar.tenant_id = sqlc.arg('tenant_id')::uuid
  AND ar.id = pdr.approval_request_id
  AND pdr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND pdr.project_id = sqlc.arg('project_id')::uuid
  AND ar.status = 'pending'
RETURNING ar.id;

-- name: SumProjectConsumedTokens :one
-- 项目 token 已消耗:对项目下所有任务的所有 attempt 的心跳累加值求和(P1-A 预算熔断)。
-- budget_consumed_tokens 由 runtime 心跳单调累加,天然把失败与返工的消耗算进去。
SELECT COALESCE(SUM(a.budget_consumed_tokens), 0)::bigint AS consumed_tokens
FROM project_task_attempts a
JOIN project_tasks t
    ON t.tenant_id = a.tenant_id AND t.id = a.project_task_id
WHERE a.tenant_id = sqlc.arg('tenant_id')::uuid
  AND t.project_id = sqlc.arg('project_id')::uuid;

-- name: SetProjectBudgetTokenLimit :one
-- 提额/设限/清限(P1-A):直接置列而非 COALESCE——列本身可空(NULL=不限),
-- 需要能显式清回不限,不能用 COALESCE 区分"不改"与"设为不限"。
UPDATE projects
SET budget_token_limit = sqlc.narg('budget_token_limit')::bigint,
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid
  AND deleted_at IS NULL
RETURNING *;

-- name: ListProjectRunSummaries :many
-- 运行总览项目运行带 + 项目管理首页组合计数:跨项目一次聚合,避免逐项目 N+1。
-- 「今日」口径与员工 today token 一致(Asia/Shanghai 日窗);今日完成按 task_runs 执行完成计
-- (执行口径,用户拍板,见 spec 2026-07-26-run-overview-display-mode §8-3)。
-- 2026-08-10: failed 排除 dismissed;扩 open_decision_count / evidence_pending_count
-- (spec 2026-08-10-projects-home-portfolio-hygiene-design §7)。
-- 2026-08-11: waiting_human / failed 状态集对齐 Web deriveProjectRiskSummary。
-- 2026-08-11 复审修:等人拆两个字段,一个聚合不能同时服务两种展示语义——
--   waiting_human_count          = 所有在等人的任务(运行总览大屏「待人工」badge 与 hasActive 用);
--   waiting_human_unlinked_count = 其中无 open decision 挂同 project_task_id 的 orphan
--                                  (项目首页用:同屏另有「待决」桶,不去重会「1 待决 · 1 等人」双计,
--                                   与明细 deriveProjectRiskSummary 的 sister-F1 去重同源)。
-- 曾把去重口径直接写进 waiting_human_count,实测大屏 21→2、4 个项目 hasActive 翻 false。
-- 排序:有失败/待人工的项目优先(用宽口径),其次按最新任务活动时间。
SELECT
    p.id AS project_id,
    p.name,
    p.status,
    COALESCE(t.running_count, 0)::integer AS running_count,
    COALESCE(t.queued_count, 0)::integer AS queued_count,
    COALESCE(t.waiting_human_count, 0)::integer AS waiting_human_count,
    COALESCE(t.waiting_human_unlinked_count, 0)::integer AS waiting_human_unlinked_count,
    COALESCE(t.failed_count, 0)::integer AS failed_count,
    COALESCE(t.unassigned_count, 0)::integer AS unassigned_count,
    COALESCE(t.participant_employee_count, 0)::integer AS participant_employee_count,
    COALESCE(r.completed_today_count, 0)::integer AS completed_today_count,
    COALESCE(d.open_decision_count, 0)::integer AS open_decision_count,
    COALESCE(e.evidence_pending_count, 0)::integer AS evidence_pending_count,
    t.last_activity_at::timestamptz AS last_activity_at
FROM projects p
LEFT JOIN (
    SELECT
        pt.project_id,
        COUNT(*) FILTER (WHERE pt.status = 'running')::integer AS running_count,
        COUNT(*) FILTER (WHERE pt.status = 'queued')::integer AS queued_count,
        -- 宽口径:与 web waitingHumanTaskStatuses + isAwaitingHumanApproval 对齐。
        -- 不去重——大屏「待人工」问的是「有几个任务卡在人身上」,与是否已建决策卡无关。
        COUNT(*) FILTER (
            WHERE pt.status IN ('waiting_human', 'pending_human', 'pending_review', 'approval_required')
               OR (
                   pt.requires_human_approval
                   AND pt.status NOT IN (
                       'completed', 'done', 'success', 'cancelled',
                       'failed', 'error', 'blocked'
                   )
               )
        )::integer AS waiting_human_count,
        -- orphan 口径:再排除已有 open decision 的任务(该任务已计入 open_decision_count)。
        COUNT(*) FILTER (
            WHERE (
                pt.status IN ('waiting_human', 'pending_human', 'pending_review', 'approval_required')
                OR (
                    pt.requires_human_approval
                    AND pt.status NOT IN (
                        'completed', 'done', 'success', 'cancelled',
                        'failed', 'error', 'blocked'
                    )
                )
            )
            AND NOT EXISTS (
                SELECT 1
                FROM project_decision_requests dr
                WHERE dr.tenant_id = pt.tenant_id
                  AND dr.project_task_id = pt.id
                  AND lower(COALESCE(dr.status_snapshot, '')) IN (
                      'pending', 'waiting', 'requested', 'open'
                  )
            )
        )::integer AS waiting_human_unlinked_count,
        -- 与 web failedTaskStatuses 对齐(failed / error / blocked)
        COUNT(*) FILTER (
            WHERE pt.status IN ('failed', 'error', 'blocked')
        )::integer AS failed_count,
        -- 活跃口径与 web isTerminalTaskStatus 同源(cancelled/completed/done/failed/success)。
        COUNT(*) FILTER (
            WHERE pt.status NOT IN ('completed', 'done', 'success', 'failed', 'cancelled')
              AND pt.assigned_digital_employee_id IS NULL
        )::integer AS unassigned_count,
        COUNT(DISTINCT pt.assigned_digital_employee_id) FILTER (
            WHERE pt.status NOT IN ('completed', 'done', 'success', 'failed', 'cancelled')
        )::integer AS participant_employee_count,
        MAX(pt.updated_at) AS last_activity_at
    FROM project_tasks pt
    WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
      AND pt.dismissed_at IS NULL
    GROUP BY pt.project_id
) t ON t.project_id = p.id
LEFT JOIN (
    -- chat run 挂项目锚仅为运行时落点(节点解析/预算边界),其产出不进项目流转
    -- (tri-mode spec §5 不变量 2 + §13),故不得计入项目业务口径的「今日完成」。
    -- run_kind 在 tasks 表(迁移 059),task_runs.task_id 非空,用 INNER JOIN 取。
    SELECT tr.project_id, COUNT(*)::integer AS completed_today_count
    FROM task_runs tr
    JOIN tasks t
      ON t.tenant_id = tr.tenant_id
     AND t.id = tr.task_id
    WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
      AND t.run_kind <> 'chat'
      AND tr.status = 'completed'
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
      AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai')
    GROUP BY tr.project_id
) r ON r.project_id = p.id
LEFT JOIN (
    SELECT project_id,
           COUNT(*) FILTER (
               WHERE lower(COALESCE(status_snapshot, '')) IN ('pending', 'waiting', 'requested', 'open')
           )::integer AS open_decision_count
    FROM project_decision_requests
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    GROUP BY project_id
) d ON d.project_id = p.id
LEFT JOIN (
    SELECT project_id,
           COUNT(*) FILTER (
               WHERE verification_status IN ('submitted', 'rejected')
           )::integer AS evidence_pending_count
    FROM project_evidence_refs
    WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    GROUP BY project_id
) e ON e.project_id = p.id
WHERE p.tenant_id = sqlc.arg('tenant_id')::uuid
  AND p.deleted_at IS NULL
  AND p.status != 'archived'
ORDER BY
    ((COALESCE(t.failed_count, 0) + COALESCE(t.waiting_human_count, 0)) > 0) DESC,
    COALESCE(t.last_activity_at, p.updated_at) DESC
LIMIT sqlc.arg('limit');

-- name: CountTaskRunsCompletedToday :one
-- 大屏 KPI「今日完成运行」的租户级总数:独立于项目状态过滤(归档项目当日完成也计入),
-- 保证 KPI 与运行带逐项目求和的口径差异是显式的(前者全租户,后者仅活跃项目)。
-- 与 ListProjectRunSummaries 同口径排除 chat run:对话不是业务运行,不进 KPI
-- (tri-mode spec §5 不变量 2)。两处必须同增同减,否则大屏总数与运行带求和口径再次分叉。
SELECT COUNT(*)::integer AS completed_today_count
FROM task_runs tr
JOIN tasks t
  ON t.tenant_id = tr.tenant_id
 AND t.id = tr.task_id
WHERE tr.tenant_id = sqlc.arg('tenant_id')::uuid
  AND t.run_kind <> 'chat'
  AND tr.status = 'completed'
  AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) >= (date_trunc('day', timezone('Asia/Shanghai', now())) AT TIME ZONE 'Asia/Shanghai')
  AND COALESCE(tr.finished_at, tr.updated_at, tr.created_at) < ((date_trunc('day', timezone('Asia/Shanghai', now())) + INTERVAL '1 day') AT TIME ZONE 'Asia/Shanghai');

-- ---------------------------------------------------------------------------
-- Project portfolio home (spec 2026-08-11-project-portfolio-layered-status)
-- ---------------------------------------------------------------------------
-- mine_only 谓词与 ListWorkflowInstances 同源（owner ∪ active human member）。
-- summary 仅受 mine_only 收窄；q/project_status/owner/task_state 只影响 items+total。
-- 展示桶：project_task_portfolio_bucket() 单一事实源（与 Go 同源）。
-- 注意 sort=attention 必须先对 filtered 全集算 attention 再 LIMIT，故 task_agg 挂
-- filtered_projects 而非 candidate 页（§4.2 与「先 LIMIT 再聚合」互斥；spec 已勘误）。
-- summary 任务层按定义扫可见集合非归档项目（全量聚合，非逐卡扇出）。

-- name: GetProjectPortfolioSummary :one
WITH visible_projects AS (
  SELECT p.id, p.status
  FROM projects p
  WHERE p.tenant_id = sqlc.arg('tenant_id')::uuid
    AND p.deleted_at IS NULL
    AND (
      NOT sqlc.arg('mine_only')::bool
      OR sqlc.arg('actor_user_id')::uuid = ANY(p.human_owner_user_ids)
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
task_buckets AS (
  SELECT
    project_task_portfolio_bucket(pt.status, pt.requires_human_approval) AS portfolio_bucket
  FROM project_tasks pt
  JOIN visible_projects vp ON vp.id = pt.project_id
  WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
    AND pt.dismissed_at IS NULL
    AND vp.status <> 'archived'
)
SELECT
  (SELECT COUNT(*)::integer FROM visible_projects) AS total_projects,
  (SELECT COUNT(*) FILTER (WHERE status = 'draft')::integer FROM visible_projects) AS status_draft,
  (SELECT COUNT(*) FILTER (WHERE status = 'configuring')::integer FROM visible_projects) AS status_configuring,
  (SELECT COUNT(*) FILTER (WHERE status = 'running')::integer FROM visible_projects) AS status_running,
  (SELECT COUNT(*) FILTER (WHERE status = 'paused')::integer FROM visible_projects) AS status_paused,
  (SELECT COUNT(*) FILTER (WHERE status = 'acceptance')::integer FROM visible_projects) AS status_acceptance,
  (SELECT COUNT(*) FILTER (WHERE status = 'archived')::integer FROM visible_projects) AS status_archived,
  (SELECT COUNT(*)::integer FROM task_buckets) AS task_total,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'pending')::integer FROM task_buckets) AS task_pending,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'queued')::integer FROM task_buckets) AS task_queued,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'running')::integer FROM task_buckets) AS task_running,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'waiting_human')::integer FROM task_buckets) AS task_waiting_human,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'blocked')::integer FROM task_buckets) AS task_blocked,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'failed')::integer FROM task_buckets) AS task_failed,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'completed')::integer FROM task_buckets) AS task_completed,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'cancelled')::integer FROM task_buckets) AS task_cancelled,
  (SELECT COUNT(*) FILTER (WHERE portfolio_bucket = 'other')::integer FROM task_buckets) AS task_other;

-- name: CountProjectPortfolioItems :one
WITH visible_projects AS (
  SELECT p.*
  FROM projects p
  WHERE p.tenant_id = sqlc.arg('tenant_id')::uuid
    AND p.deleted_at IS NULL
    AND (
      NOT sqlc.arg('mine_only')::bool
      OR sqlc.arg('actor_user_id')::uuid = ANY(p.human_owner_user_ids)
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
filtered_projects AS (
  SELECT vp.id
  FROM visible_projects vp
  WHERE (
      sqlc.narg('q')::text IS NULL
      OR vp.name ILIKE '%' || sqlc.narg('q')::text || '%'
      OR vp.directory_name ILIKE '%' || sqlc.narg('q')::text || '%'
      OR COALESCE(vp.goal, '') ILIKE '%' || sqlc.narg('q')::text || '%'
    )
    AND (
      cardinality(sqlc.arg('project_statuses')::text[]) = 0
      OR vp.status = ANY(sqlc.arg('project_statuses')::text[])
    )
    AND (
      sqlc.narg('owner_user_id')::uuid IS NULL
      OR sqlc.narg('owner_user_id')::uuid = ANY(vp.human_owner_user_ids)
      OR vp.human_owner_user_id = sqlc.narg('owner_user_id')::uuid
    )
    AND (
      sqlc.narg('task_state')::text IS NULL
      OR EXISTS (
        SELECT 1
        FROM project_tasks pt
        WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
          AND pt.project_id = vp.id
          AND pt.dismissed_at IS NULL
          AND (
            project_task_portfolio_bucket(pt.status, pt.requires_human_approval)
          ) = sqlc.narg('task_state')::text
      )
    )
)
SELECT COUNT(*)::integer AS total
FROM filtered_projects;

-- name: ListProjectPortfolioItems :many
WITH visible_projects AS (
  SELECT p.*
  FROM projects p
  WHERE p.tenant_id = sqlc.arg('tenant_id')::uuid
    AND p.deleted_at IS NULL
    AND (
      NOT sqlc.arg('mine_only')::bool
      OR sqlc.arg('actor_user_id')::uuid = ANY(p.human_owner_user_ids)
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
filtered_projects AS (
  SELECT vp.*
  FROM visible_projects vp
  WHERE (
      sqlc.narg('q')::text IS NULL
      OR vp.name ILIKE '%' || sqlc.narg('q')::text || '%'
      OR vp.directory_name ILIKE '%' || sqlc.narg('q')::text || '%'
      OR COALESCE(vp.goal, '') ILIKE '%' || sqlc.narg('q')::text || '%'
    )
    AND (
      cardinality(sqlc.arg('project_statuses')::text[]) = 0
      OR vp.status = ANY(sqlc.arg('project_statuses')::text[])
    )
    AND (
      sqlc.narg('owner_user_id')::uuid IS NULL
      OR sqlc.narg('owner_user_id')::uuid = ANY(vp.human_owner_user_ids)
      OR vp.human_owner_user_id = sqlc.narg('owner_user_id')::uuid
    )
    AND (
      sqlc.narg('task_state')::text IS NULL
      OR EXISTS (
        SELECT 1
        FROM project_tasks pt
        WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
          AND pt.project_id = vp.id
          AND pt.dismissed_at IS NULL
          AND (
            project_task_portfolio_bucket(pt.status, pt.requires_human_approval)
          ) = sqlc.narg('task_state')::text
      )
    )
),
task_agg AS (
  SELECT
    pt.project_id,
    COUNT(*)::integer AS task_total,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'pending')::integer AS task_pending,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'queued')::integer AS task_queued,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'running')::integer AS task_running,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'waiting_human')::integer AS task_waiting_human,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'blocked')::integer AS task_blocked,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'failed')::integer AS task_failed,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'completed')::integer AS task_completed,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'cancelled')::integer AS task_cancelled,
    COUNT(*) FILTER (WHERE b.portfolio_bucket = 'other')::integer AS task_other,
    -- orphan 等人：宽口径 waiting_human 再排除已有 open decision（§3.3）
    COUNT(*) FILTER (
      WHERE b.portfolio_bucket = 'waiting_human'
        AND NOT EXISTS (
          SELECT 1
          FROM project_decision_requests dr
          WHERE dr.tenant_id = pt.tenant_id
            AND dr.project_task_id = pt.id
            AND lower(COALESCE(dr.status_snapshot, '')) IN (
                'pending', 'waiting', 'requested', 'open'
            )
        )
    )::integer AS waiting_human_unlinked_count,
    COUNT(*) FILTER (
      WHERE pt.status NOT IN ('completed', 'done', 'success', 'failed', 'cancelled')
        AND pt.assigned_digital_employee_id IS NULL
    )::integer AS unassigned_count,
    COUNT(DISTINCT pt.assigned_digital_employee_id) FILTER (
      WHERE pt.status NOT IN ('completed', 'done', 'success', 'failed', 'cancelled')
        AND pt.assigned_digital_employee_id IS NOT NULL
    )::integer AS active_digital_employee_count,
    MAX(pt.updated_at) AS last_activity_at
  FROM project_tasks pt
  JOIN filtered_projects fp ON fp.id = pt.project_id
  CROSS JOIN LATERAL (
    SELECT project_task_portfolio_bucket(pt.status, pt.requires_human_approval) AS portfolio_bucket
  ) b
  WHERE pt.tenant_id = sqlc.arg('tenant_id')::uuid
    AND pt.dismissed_at IS NULL
  GROUP BY pt.project_id
),
decision_agg AS (
  SELECT
    project_id,
    COUNT(*) FILTER (
      WHERE lower(COALESCE(status_snapshot, '')) IN ('pending', 'waiting', 'requested', 'open')
    )::integer AS open_decision_count
  FROM project_decision_requests
  WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    AND project_id IN (SELECT id FROM filtered_projects)
  GROUP BY project_id
),
evidence_agg AS (
  SELECT
    project_id,
    COUNT(*) FILTER (
      WHERE verification_status IN ('submitted', 'rejected')
    )::integer AS evidence_pending_count
  FROM project_evidence_refs
  WHERE tenant_id = sqlc.arg('tenant_id')::uuid
    AND project_id IN (SELECT id FROM filtered_projects)
  GROUP BY project_id
),
candidate_projects AS (
  SELECT
    fp.*,
    COALESCE(t.task_total, 0)::integer AS task_total,
    COALESCE(t.task_pending, 0)::integer AS task_pending,
    COALESCE(t.task_queued, 0)::integer AS task_queued,
    COALESCE(t.task_running, 0)::integer AS task_running,
    COALESCE(t.task_waiting_human, 0)::integer AS task_waiting_human,
    COALESCE(t.task_blocked, 0)::integer AS task_blocked,
    COALESCE(t.task_failed, 0)::integer AS task_failed,
    COALESCE(t.task_completed, 0)::integer AS task_completed,
    COALESCE(t.task_cancelled, 0)::integer AS task_cancelled,
    COALESCE(t.task_other, 0)::integer AS task_other,
    COALESCE(t.waiting_human_unlinked_count, 0)::integer AS waiting_human_unlinked_count,
    COALESCE(t.unassigned_count, 0)::integer AS unassigned_count,
    COALESCE(t.active_digital_employee_count, 0)::integer AS active_digital_employee_count,
    COALESCE(d.open_decision_count, 0)::integer AS open_decision_count,
    COALESCE(e.evidence_pending_count, 0)::integer AS evidence_pending_count,
    (COALESCE(t.last_activity_at, fp.updated_at))::timestamptz AS effective_last_activity_at
  FROM filtered_projects fp
  LEFT JOIN task_agg t ON t.project_id = fp.id
  LEFT JOIN decision_agg d ON d.project_id = fp.id
  LEFT JOIN evidence_agg e ON e.project_id = fp.id
  ORDER BY
    CASE
      WHEN sqlc.arg('sort')::text = 'attention' THEN
        CASE
          WHEN (
            COALESCE(t.task_failed, 0)
            + COALESCE(t.task_blocked, 0)
            + COALESCE(t.waiting_human_unlinked_count, 0)
            + COALESCE(d.open_decision_count, 0)
          ) > 0 THEN 0
          ELSE 1
        END
      ELSE 0
    END ASC,
    CASE
      WHEN sqlc.arg('sort')::text = 'created' THEN fp.created_at
      ELSE COALESCE(t.last_activity_at, fp.updated_at)
    END DESC NULLS LAST,
    fp.id DESC
  LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset')
)
SELECT
  cp.id AS project_id,
  cp.name,
  COALESCE(cp.goal, '')::text AS goal,
  cp.status,
  cp.human_owner_user_id,
  cp.human_owner_user_ids,
  COALESCE(cp.coordination_status, '')::text AS coordination_status,
  cp.updated_at,
  cp.created_at,
  cp.archived_at,
  COALESCE(NULLIF(BTRIM(owner.display_name), ''), owner.username, '')::text AS owner_display_name,
  cp.task_total,
  cp.task_pending,
  cp.task_queued,
  cp.task_running,
  cp.task_waiting_human,
  cp.task_blocked,
  cp.task_failed,
  cp.task_completed,
  cp.task_cancelled,
  cp.task_other,
  cp.waiting_human_unlinked_count,
  cp.unassigned_count,
  cp.active_digital_employee_count,
  cp.open_decision_count,
  cp.evidence_pending_count,
  cp.effective_last_activity_at
FROM candidate_projects cp
LEFT JOIN auth_users owner ON owner.id = cp.human_owner_user_id
ORDER BY
  CASE
    WHEN sqlc.arg('sort')::text = 'attention' THEN
      CASE
        WHEN (
          cp.task_failed
          + cp.task_blocked
          + cp.waiting_human_unlinked_count
          + cp.open_decision_count
        ) > 0 THEN 0
        ELSE 1
      END
    ELSE 0
  END ASC,
  CASE
    WHEN sqlc.arg('sort')::text = 'created' THEN cp.created_at
    ELSE cp.effective_last_activity_at
  END DESC NULLS LAST,
  cp.id DESC;
