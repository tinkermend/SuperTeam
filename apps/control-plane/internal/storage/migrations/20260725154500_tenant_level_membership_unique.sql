-- 同一租户下每个用户至多一条活跃的租户级成员（team_id IS NULL），作为 console.access 事实源。
CREATE UNIQUE INDEX idx_tenant_members_active_tenant_level_principal
    ON tenant_members (tenant_id, principal_type, principal_id)
    WHERE team_id IS NULL AND disabled_at IS NULL;
