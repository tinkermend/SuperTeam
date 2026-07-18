-- 075_team_delete_binding_cleanup.sql
-- 团队删除后技能/能力绑定悬空修复(存量数据清理)。
-- 根因:DeleteTeam 事务只解绑数字员工+软删团队,未处置 team_skill_bindings 与
-- team_mcp_bindings,技能详情与 KPI 统计因此出现幽灵团队。
-- 增量路径由 DeleteTeam 事务内清理保证(见 tenant/pg_repository.go),本迁移只回收存量。

-- 1) 技能绑定表无软删列,团队已软删或缺失的绑定行直接物理删除。
DELETE FROM team_skill_bindings stb
WHERE NOT EXISTS (
    SELECT 1 FROM tenant_teams tt
    WHERE tt.tenant_id = stb.tenant_id
      AND tt.id = stb.team_id
      AND tt.deleted_at IS NULL
);

-- 2) 能力绑定表自带软删语义,按其自身删除口径软删,保留审计痕迹。
UPDATE team_mcp_bindings tb
SET deleted_at = NOW(),
    updated_at = NOW()
WHERE tb.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM tenant_teams tt
    WHERE tt.tenant_id = tb.tenant_id
      AND tt.id = tb.team_id
      AND tt.deleted_at IS NULL
);
