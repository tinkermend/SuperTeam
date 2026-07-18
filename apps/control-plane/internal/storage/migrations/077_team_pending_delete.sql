-- 077_team_pending_delete.sql
-- 团队生命周期收敛 P2(spec 2026-07-18-team-lifecycle-convergence §2):删除进入
-- pending_delete 待确认态(全站不可见),管理员恢复或确认后才物理删除。
-- 存量口径:P1 前已软删团队(status=active+deleted_at 置位)为遗留终态,不入待确认队列。

-- 1) status 约束重建:允许 pending_delete 中间态。
ALTER TABLE tenant_teams DROP CONSTRAINT chk_tenant_teams_status;
ALTER TABLE tenant_teams
  ADD CONSTRAINT chk_tenant_teams_status CHECK (status IN ('active', 'pending_delete'));

-- 2) 删除发起人:待确认队列展示与滞留催办的收件人。
ALTER TABLE tenant_teams ADD COLUMN delete_requested_by uuid;
COMMENT ON COLUMN tenant_teams.delete_requested_by IS '删除发起人(pending_delete 期间有值;恢复时清空)';

-- 3) 待确认队列扫描索引(队列查询与滞留催办均按此过滤)。
CREATE INDEX idx_tenant_teams_pending_delete
  ON tenant_teams (tenant_id, deleted_at)
  WHERE status = 'pending_delete';
