-- 数据保留清理查询(P1-B)。
--
-- 每张表一条静态语句而不是一条动态 SQL:表名不进参数,既避免注入面,也让 sqlc
-- 保持类型检查。保留策略的"表驱动"体现在 internal/retention 的注册表上,注册表项
-- 各自挂一条本文件里的语句。
--
-- 全部按批删除(LIMIT + ctid 子查询):远程库上一次性删几十万行会拖长事务、放大锁
-- 竞争,分批让每个事务都很短。调用方循环到单批删不满为止。
--
-- 业务事实表(project_events / execution_ledger_events / audit_events)不按时间删,
-- 只在其所属项目已软删且超期后随项目一起清(见文件末尾)。

-- name: DeleteExpiredRuntimeEvents :execrows
DELETE FROM runtime_events
WHERE ctid IN (
    SELECT ctid FROM runtime_events
    WHERE created_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: DeleteExpiredProviderSessionEvents :execrows
DELETE FROM provider_session_events
WHERE ctid IN (
    SELECT ctid FROM provider_session_events
    WHERE created_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: DeleteExpiredRuntimeCommandReceipts :execrows
DELETE FROM runtime_command_receipts
WHERE ctid IN (
    SELECT ctid FROM runtime_command_receipts
    WHERE created_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: DeleteExpiredTaskEvents :execrows
DELETE FROM task_events
WHERE ctid IN (
    SELECT ctid FROM task_events
    WHERE created_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: DeleteExpiredAuthzAllowOperationLogs :execrows
-- authz 放行日志单独一条更短的保留期:实测 177,090 条 succeeded 对 19 条 failed,
-- 放行记录几乎没有审计价值,拒绝记录才是信号。拒绝走下面的通用规则。
DELETE FROM web_operation_logs
WHERE ctid IN (
    SELECT ctid FROM web_operation_logs
    WHERE module = 'authz'
      AND result = 'succeeded'
      AND created_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: DeleteExpiredOperationLogs :execrows
DELETE FROM web_operation_logs
WHERE ctid IN (
    SELECT ctid FROM web_operation_logs
    WHERE NOT (module = 'authz' AND result = 'succeeded')
      AND created_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: DeleteExpiredAuthSessions :execrows
DELETE FROM auth_sessions
WHERE ctid IN (
    SELECT ctid FROM auth_sessions
    WHERE expires_at < NOW()
    LIMIT sqlc.arg('batch_size')
);

-- 以下三条清理"已软删且超期项目"的事实行。项目软删只置 deleted_at、不清行
-- (SoftDeleteProject),这些行在应用层已完全不可见,却是最大的一块死重。

-- name: DeleteProjectEventsForPurgedProjects :execrows
DELETE FROM project_events
WHERE ctid IN (
    SELECT e.ctid FROM project_events e
    JOIN projects p ON p.id = e.project_id
    WHERE p.deleted_at IS NOT NULL
      AND p.deleted_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: DeleteExecutionLedgerEventsForPurgedProjects :execrows
DELETE FROM execution_ledger_events
WHERE ctid IN (
    SELECT e.ctid FROM execution_ledger_events e
    JOIN projects p ON p.id = e.project_id
    WHERE p.deleted_at IS NOT NULL
      AND p.deleted_at < NOW() - make_interval(days => sqlc.arg('retention_days')::int)
    LIMIT sqlc.arg('batch_size')
);

-- name: CountRetentionCandidates :one
-- 观测用:不删,只报当前各类超期行的规模,供日志与人工核对。
SELECT
    (SELECT COUNT(*) FROM runtime_events WHERE created_at < NOW() - make_interval(days => sqlc.arg('runtime_days')::int))::bigint AS runtime_events,
    (SELECT COUNT(*) FROM web_operation_logs WHERE module = 'authz' AND result = 'succeeded')::bigint AS authz_allow_logs,
    (SELECT COUNT(*) FROM auth_sessions WHERE expires_at < NOW())::bigint AS expired_sessions;
