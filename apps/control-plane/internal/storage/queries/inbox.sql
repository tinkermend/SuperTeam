-- name: UpsertInboxItem :one
INSERT INTO inbox_items (
    tenant_id,
    team_id,
    target_user_id,
    scope,
    item_type,
    source_type,
    source_id,
    source_project_id,
    source_task_id,
    source_approval_request_id,
    title,
    summary,
    risk_level,
    priority,
    status,
    action_schema,
    context_payload,
    deep_link,
    resolved_at,
    last_activity_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('scope')::varchar,
    sqlc.arg('item_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::uuid,
    sqlc.narg('source_project_id')::uuid,
    sqlc.narg('source_task_id')::uuid,
    sqlc.narg('source_approval_request_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.narg('risk_level')::varchar,
    sqlc.narg('priority')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('action_schema')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('context_payload')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('deep_link')::jsonb, '{}'::jsonb),
    sqlc.narg('resolved_at')::timestamptz,
    sqlc.arg('last_activity_at')::timestamptz
)
ON CONFLICT (tenant_id, source_type, source_id)
DO UPDATE SET
    team_id = EXCLUDED.team_id,
    target_user_id = EXCLUDED.target_user_id,
    scope = EXCLUDED.scope,
    item_type = EXCLUDED.item_type,
    -- F2(§5.4.2): resolve 投影不得清空已有上下文。source_project_id/source_task_id
    -- 为空(NULL)时保留原值;context_payload 为空对象({})时保留原值——一个闸门可能被
    -- 审批与决策两条管道写同一行,后写方(如 resolve 触发的 approval 投影)不带这些字段
    -- 时不得抹掉先写方(decision 投影)已填的项目归属与展示上下文。
    source_project_id = COALESCE(EXCLUDED.source_project_id, inbox_items.source_project_id),
    source_task_id = COALESCE(EXCLUDED.source_task_id, inbox_items.source_task_id),
    source_approval_request_id = EXCLUDED.source_approval_request_id,
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    risk_level = EXCLUDED.risk_level,
    priority = EXCLUDED.priority,
    status = EXCLUDED.status,
    action_schema = EXCLUDED.action_schema,
    context_payload = CASE
        WHEN EXCLUDED.context_payload = '{}'::jsonb THEN inbox_items.context_payload
        ELSE EXCLUDED.context_payload
    END,
    deep_link = EXCLUDED.deep_link,
    resolved_at = EXCLUDED.resolved_at,
    last_activity_at = EXCLUDED.last_activity_at,
    updated_at = NOW()
RETURNING *;

-- name: UpsertInboxItemByApprovalSource :one
INSERT INTO inbox_items (
    tenant_id,
    team_id,
    target_user_id,
    scope,
    item_type,
    source_type,
    source_id,
    source_project_id,
    source_task_id,
    source_approval_request_id,
    title,
    summary,
    risk_level,
    priority,
    status,
    action_schema,
    context_payload,
    deep_link,
    resolved_at,
    last_activity_at
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('scope')::varchar,
    sqlc.arg('item_type')::varchar,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_id')::uuid,
    sqlc.narg('source_project_id')::uuid,
    sqlc.narg('source_task_id')::uuid,
    sqlc.arg('source_approval_request_id')::uuid,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.narg('risk_level')::varchar,
    sqlc.narg('priority')::varchar,
    sqlc.arg('status')::varchar,
    COALESCE(sqlc.narg('action_schema')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('context_payload')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('deep_link')::jsonb, '{}'::jsonb),
    sqlc.narg('resolved_at')::timestamptz,
    sqlc.arg('last_activity_at')::timestamptz
)
ON CONFLICT (tenant_id, source_approval_request_id)
WHERE source_approval_request_id IS NOT NULL
DO UPDATE SET
    team_id = EXCLUDED.team_id,
    target_user_id = EXCLUDED.target_user_id,
    scope = EXCLUDED.scope,
    item_type = EXCLUDED.item_type,
    source_type = EXCLUDED.source_type,
    source_id = EXCLUDED.source_id,
    -- F2(§5.4.2): 审批与决策共用 (tenant_id, source_approval_request_id) 唯一行;resolve
    -- 触发的 approval 投影不带 source_project_id/source_task_id/上下文时,不得抹掉 decision
    -- 投影已填的项目归属与展示上下文(见上面 UpsertInboxItem 同理)。
    source_project_id = COALESCE(EXCLUDED.source_project_id, inbox_items.source_project_id),
    source_task_id = COALESCE(EXCLUDED.source_task_id, inbox_items.source_task_id),
    title = EXCLUDED.title,
    summary = EXCLUDED.summary,
    risk_level = EXCLUDED.risk_level,
    priority = EXCLUDED.priority,
    status = EXCLUDED.status,
    action_schema = EXCLUDED.action_schema,
    context_payload = CASE
        WHEN EXCLUDED.context_payload = '{}'::jsonb THEN inbox_items.context_payload
        ELSE EXCLUDED.context_payload
    END,
    deep_link = EXCLUDED.deep_link,
    resolved_at = EXCLUDED.resolved_at,
    last_activity_at = EXCLUDED.last_activity_at,
    updated_at = NOW()
RETURNING *;

-- name: GetInboxItem :one
SELECT * FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;

-- name: GetInboxItemByApprovalSource :one
-- §5.4.1: ApprovalProjectorAdapter uses this to detect when a project-decision
-- card already owns the (tenant_id, source_approval_request_id) unique row.
SELECT * FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND source_approval_request_id = sqlc.arg('source_approval_request_id')::uuid;

-- name: ListInboxItems :many
-- 分诊默认序：风险优先（blocked→high→medium→low），同级按最近活动。
-- NULL / 未登记 risk_level 排最后（ELSE 4），不得插队。
-- 契约：组内顺序由本查询承担；前端 groupInboxItems 不得二次排序。
-- WHERE 必须与 ListInboxItemsOldest 逐字同口径（TestInboxListQueriesShareWhereClause）。
SELECT * FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (
    sqlc.narg('status')::varchar IS NULL
    OR status = sqlc.narg('status')::varchar
  )
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
    OR (
      -- any-of-N: 项目决策类事项对该项目全部 active 人类成员可见(成员同等身份)
      item_type = 'project_decision'
      AND source_project_id IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM project_members pm
        WHERE pm.tenant_id = inbox_items.tenant_id
          AND pm.project_id = inbox_items.source_project_id
          AND pm.principal_type = 'human_user'
          AND pm.status = 'active'
          AND pm.principal_id = sqlc.narg('target_user_id')::uuid
      )
    )
  )
  AND (
    sqlc.narg('item_type')::varchar IS NULL
    OR item_type = sqlc.narg('item_type')::varchar
  )
  AND (
    sqlc.narg('risk_level')::varchar IS NULL
    OR risk_level = sqlc.narg('risk_level')::varchar
  )
  AND (
    sqlc.narg('source_project_id')::uuid IS NULL
    OR source_project_id = sqlc.narg('source_project_id')::uuid
  )
ORDER BY
  CASE risk_level
    WHEN 'blocked' THEN 0
    WHEN 'high'    THEN 1
    WHEN 'medium'  THEN 2
    WHEN 'low'     THEN 3
    ELSE 4
  END,
  last_activity_at DESC, created_at DESC, id DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: ListInboxItemsOldest :many
-- sort=oldest：积压视角，谁被晾最久。
-- WHERE 必须与 ListInboxItems 逐字同口径（含 any-of-N 注释）；改一处漏一处会静默破坏可见性。
-- 护栏：migrations_test.go TestInboxListQueriesShareWhereClause。
SELECT * FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (
    sqlc.narg('status')::varchar IS NULL
    OR status = sqlc.narg('status')::varchar
  )
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
    OR (
      -- any-of-N: 项目决策类事项对该项目全部 active 人类成员可见(成员同等身份)
      item_type = 'project_decision'
      AND source_project_id IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM project_members pm
        WHERE pm.tenant_id = inbox_items.tenant_id
          AND pm.project_id = inbox_items.source_project_id
          AND pm.principal_type = 'human_user'
          AND pm.status = 'active'
          AND pm.principal_id = sqlc.narg('target_user_id')::uuid
      )
    )
  )
  AND (
    sqlc.narg('item_type')::varchar IS NULL
    OR item_type = sqlc.narg('item_type')::varchar
  )
  AND (
    sqlc.narg('risk_level')::varchar IS NULL
    OR risk_level = sqlc.narg('risk_level')::varchar
  )
  AND (
    sqlc.narg('source_project_id')::uuid IS NULL
    OR source_project_id = sqlc.narg('source_project_id')::uuid
  )
ORDER BY created_at ASC, id ASC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CountInboxItems :one
SELECT COUNT(*)::bigint FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (
    sqlc.narg('status')::varchar IS NULL
    OR status = sqlc.narg('status')::varchar
  )
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
    OR (
      -- any-of-N: 项目决策类事项对该项目全部 active 人类成员可见(成员同等身份)
      item_type = 'project_decision'
      AND source_project_id IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM project_members pm
        WHERE pm.tenant_id = inbox_items.tenant_id
          AND pm.project_id = inbox_items.source_project_id
          AND pm.principal_type = 'human_user'
          AND pm.status = 'active'
          AND pm.principal_id = sqlc.narg('target_user_id')::uuid
      )
    )
  )
  AND (
    sqlc.narg('item_type')::varchar IS NULL
    OR item_type = sqlc.narg('item_type')::varchar
  )
  AND (
    sqlc.narg('risk_level')::varchar IS NULL
    OR risk_level = sqlc.narg('risk_level')::varchar
  )
  AND (
    sqlc.narg('source_project_id')::uuid IS NULL
    OR source_project_id = sqlc.narg('source_project_id')::uuid
  );

-- name: CountHighRiskInboxItems :one
SELECT COUNT(*)::bigint FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND status = 'open'
  AND risk_level = 'high'
  AND (
    sqlc.narg('target_user_id')::uuid IS NULL
    OR target_user_id = sqlc.narg('target_user_id')::uuid
    OR (
      -- any-of-N: 项目决策类事项对该项目全部 active 人类成员可见(成员同等身份)
      item_type = 'project_decision'
      AND source_project_id IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM project_members pm
        WHERE pm.tenant_id = inbox_items.tenant_id
          AND pm.project_id = inbox_items.source_project_id
          AND pm.principal_type = 'human_user'
          AND pm.status = 'active'
          AND pm.principal_id = sqlc.narg('target_user_id')::uuid
      )
    )
  );

-- name: PeekInboxChange :one
-- 收件箱 SSE 脏通知探测:返回 actor 可见范围内、(updated_at, id) 游标之后最新的一条变更行。
-- 可见性谓词必须与 ListInboxItems 的 target_user_id 分支同口径(含 any-of-N 项目决策成员可见);
-- team_view_allowed(具团队读权)时放宽到全租户。只取最新一行:两次探测间的多条变更折叠为一次通知。
SELECT id, updated_at FROM inbox_items
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND (
    sqlc.arg('team_view_allowed')::boolean
    OR target_user_id = sqlc.arg('actor_user_id')::uuid
    OR (
      item_type = 'project_decision'
      AND source_project_id IS NOT NULL
      AND EXISTS (
        SELECT 1 FROM project_members pm
        WHERE pm.tenant_id = inbox_items.tenant_id
          AND pm.project_id = inbox_items.source_project_id
          AND pm.principal_type = 'human_user'
          AND pm.status = 'active'
          AND pm.principal_id = sqlc.arg('actor_user_id')::uuid
      )
    )
  )
  AND (updated_at, id) > (sqlc.arg('cursor_updated_at')::timestamptz, sqlc.arg('cursor_id')::uuid)
ORDER BY updated_at DESC, id DESC
LIMIT 1;

-- name: CancelInboxItemsForProjectDelete :many
-- 取消项目挂接的 open 收件箱（source_project_id 显式关联）。
-- run 失败恢复卡分支已随「运行必须归属项目」spec（2026-07-26 A4）退役：
-- 该 item_type 不再产生，存量 open 态已由迁移 20260726170000 取消。
UPDATE inbox_items ii
SET status = 'cancelled',
    resolved_at = COALESCE(ii.resolved_at, NOW()),
    updated_at = NOW()
WHERE ii.tenant_id = sqlc.arg('tenant_id')::uuid
  AND ii.status = 'open'
  AND ii.source_project_id = sqlc.arg('project_id')::uuid
RETURNING ii.id;

-- name: ResolveOpenInboxItemsBySource :exec
-- 按来源关闭 open 收件箱(通道恢复告警、同类幂等告警回收)。
UPDATE inbox_items
SET status = 'resolved',
    resolved_at = COALESCE(resolved_at, NOW()),
    updated_at = NOW()
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND source_type = sqlc.arg('source_type')::varchar
  AND source_id = sqlc.arg('source_id')::uuid
  AND status = 'open';

-- name: ListInboxProjectNames :many
-- 收件箱来源补名:批量取项目名称(读时解析,不入库快照)。
SELECT id, name FROM projects
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListInboxProjectTaskTitles :many
-- 收件箱来源补名:批量取项目任务标题。
SELECT id, title FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = ANY(sqlc.arg('ids')::uuid[]);

-- name: ListInboxDemandTitles :many
-- 收件箱来源补名:批量取需求标题(读时解析,不入库快照)。
SELECT id, title FROM project_demands
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = ANY(sqlc.arg('ids')::uuid[]);
