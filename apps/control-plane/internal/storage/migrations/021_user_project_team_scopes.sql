ALTER TABLE auth_users
    ADD COLUMN IF NOT EXISTS avatar_asset_id VARCHAR(100);

CREATE TABLE user_project_team_scopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    user_id UUID NOT NULL,
    team_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    granted_by_user_id UUID,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE user_project_team_scopes
    ADD CONSTRAINT chk_user_project_team_scopes_status
    CHECK (status IN ('active', 'revoked'));

CREATE UNIQUE INDEX uq_user_project_team_scopes_active
    ON user_project_team_scopes(tenant_id, user_id, team_id)
    WHERE status = 'active';

CREATE INDEX idx_user_project_team_scopes_tenant_user_status
    ON user_project_team_scopes(tenant_id, user_id, status);

CREATE INDEX idx_user_project_team_scopes_tenant_team_status
    ON user_project_team_scopes(tenant_id, team_id, status);

CREATE TRIGGER update_user_project_team_scopes_updated_at
    BEFORE UPDATE ON user_project_team_scopes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON COLUMN auth_users.avatar_asset_id IS '人类用户选择的内置头像资产 ID；创建用户流程以该字段作为头像事实来源';
COMMENT ON TABLE user_project_team_scopes IS '人类用户创建项目时可选择的团队授权范围';
COMMENT ON COLUMN user_project_team_scopes.id IS '授权记录 UUID';
COMMENT ON COLUMN user_project_team_scopes.tenant_id IS '租户 ID';
COMMENT ON COLUMN user_project_team_scopes.user_id IS '被授权的人类用户 ID';
COMMENT ON COLUMN user_project_team_scopes.team_id IS '用户创建项目时可选择的团队 ID';
COMMENT ON COLUMN user_project_team_scopes.status IS '授权状态：active 表示可用，revoked 表示已撤销';
COMMENT ON COLUMN user_project_team_scopes.granted_by_user_id IS '授予或最后替换该授权范围的管理员用户 ID';
COMMENT ON COLUMN user_project_team_scopes.revoked_at IS '授权撤销时间';
COMMENT ON COLUMN user_project_team_scopes.created_at IS '授权创建时间';
COMMENT ON COLUMN user_project_team_scopes.updated_at IS '授权最后更新时间';
