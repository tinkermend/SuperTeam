CREATE TABLE IF NOT EXISTS tenant_team_member_role_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    target_user_id UUID NOT NULL,
    requested_role VARCHAR(100) NOT NULL,
    requested_by UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    reason TEXT NOT NULL DEFAULT '',
    decided_by UUID,
    decided_at TIMESTAMPTZ,
    decision_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tenant_team_member_role_requests_team
    ON tenant_team_member_role_requests(tenant_id, team_id, status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_tenant_team_member_role_requests_target
    ON tenant_team_member_role_requests(tenant_id, target_user_id, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tenant_team_member_role_requests_pending_role
    ON tenant_team_member_role_requests(tenant_id, team_id, target_user_id, requested_role)
    WHERE status = 'pending';

COMMENT ON TABLE tenant_team_member_role_requests IS '团队高权限角色变更申请表';
COMMENT ON COLUMN tenant_team_member_role_requests.id IS '角色变更申请ID';
COMMENT ON COLUMN tenant_team_member_role_requests.tenant_id IS '租户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.team_id IS '团队ID';
COMMENT ON COLUMN tenant_team_member_role_requests.target_user_id IS '目标用户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.requested_role IS '申请授予的团队角色';
COMMENT ON COLUMN tenant_team_member_role_requests.requested_by IS '申请人用户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.status IS '申请状态：pending、approved、rejected';
COMMENT ON COLUMN tenant_team_member_role_requests.reason IS '申请原因';
COMMENT ON COLUMN tenant_team_member_role_requests.decided_by IS '审批人用户ID';
COMMENT ON COLUMN tenant_team_member_role_requests.decided_at IS '审批时间';
COMMENT ON COLUMN tenant_team_member_role_requests.decision_reason IS '审批说明';
COMMENT ON COLUMN tenant_team_member_role_requests.created_at IS '创建时间';
COMMENT ON COLUMN tenant_team_member_role_requests.updated_at IS '更新时间';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_tenant_team_member_role_requests_updated_at'
    ) THEN
        CREATE TRIGGER update_tenant_team_member_role_requests_updated_at
        BEFORE UPDATE ON tenant_team_member_role_requests
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;
