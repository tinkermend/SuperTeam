-- 076_team_lifecycle_convergence.sql
-- 团队生命周期收敛(spec 2026-07-18-team-lifecycle-convergence §1):撤销归档/停用,
-- 存活团队唯一状态 active,删除只经 deleted_at(软删+审计)。归档/停用端点与
-- SetTenantTeamStatus 写入路径已在代码层移除,本迁移收敛存量数据并加约束。

-- 1) 存量 status 残留刷平:archived/disabled(含已删行的尸体残留)一律回 active。
--    未删除的 archived/disabled 团队视为仍存活,删不删由管理员通过删除入口再决定。
UPDATE tenant_teams
SET status = 'active', disabled_at = NULL, archived_at = NULL, updated_at = NOW()
WHERE status <> 'active' OR disabled_at IS NOT NULL OR archived_at IS NOT NULL;

-- 2) 约束兜底:status 此后只允许 active(P2 引入 pending_delete 时重建此约束)。
ALTER TABLE tenant_teams
  ADD CONSTRAINT chk_tenant_teams_status CHECK (status = 'active');

-- 3) 存量悬空员工清理:team_id 指向已删团队的存活员工按 DeleteTeam 增量口径
--    解绑(置 NULL 入候岗);已删员工保留历史归属不动。
UPDATE digital_employees
SET team_id = NULL, updated_at = NOW()
WHERE deleted_at IS NULL
  AND team_id IN (SELECT id FROM tenant_teams WHERE deleted_at IS NOT NULL);
