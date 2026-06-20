-- 团队借调：供给侧借调策略 + 需求侧借调请求/审批。
-- 团队是受治理的能力供给单元；项目通过借调获得团队数字员工的使用授权。
-- D1=团队级 (project, team) 粒度；D2=超纲强制转人工；D3=请求自带 status + inbox/audit。

CREATE TABLE team_lending_policy (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    team_id UUID NOT NULL,
    allow_lending BOOLEAN NOT NULL DEFAULT false,
    approval_mode VARCHAR(50) NOT NULL DEFAULT 'manual',
    budget_ceiling NUMERIC,
    capability_ceiling JSONB NOT NULL DEFAULT '{}'::jsonb,
    project_match JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_by_user_id UUID,
    updated_by_user_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE team_lending_policy
    ADD CONSTRAINT chk_team_lending_policy_approval_mode
    CHECK (approval_mode IN ('auto', 'manual'));

ALTER TABLE team_lending_policy
    ADD CONSTRAINT chk_team_lending_policy_status
    CHECK (status IN ('active', 'archived'));

CREATE UNIQUE INDEX uq_team_lending_policy_active
    ON team_lending_policy(tenant_id, team_id)
    WHERE status = 'active';

CREATE INDEX idx_team_lending_policy_tenant_team
    ON team_lending_policy(tenant_id, team_id);

CREATE TRIGGER update_team_lending_policy_updated_at
    BEFORE UPDATE ON team_lending_policy
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE team_lending_policy IS '团队借调策略：团队负责人设置本团队员工是否/在什么条件下可被项目借调（每团队一条 active）';
COMMENT ON COLUMN team_lending_policy.id IS '借调策略 UUID';
COMMENT ON COLUMN team_lending_policy.tenant_id IS '租户 ID';
COMMENT ON COLUMN team_lending_policy.team_id IS '所属团队 ID（供给侧）';
COMMENT ON COLUMN team_lending_policy.allow_lending IS '是否允许本团队员工被项目借调';
COMMENT ON COLUMN team_lending_policy.approval_mode IS '审批模式：auto 符合策略自动放行；manual 每次借调需团队负责人审批';
COMMENT ON COLUMN team_lending_policy.budget_ceiling IS '单次借调预算上限；请求超过则强制转人工审批';
COMMENT ON COLUMN team_lending_policy.capability_ceiling IS '可被借调时允许的能力/runtime scope 天花板，不可超出团队治理外壳';
COMMENT ON COLUMN team_lending_policy.project_match IS '可被哪些项目调用的匹配条件（标签 / owner 范围等，registry-first）';
COMMENT ON COLUMN team_lending_policy.status IS '策略状态：active 生效，archived 历史版本';
COMMENT ON COLUMN team_lending_policy.created_by_user_id IS '创建该策略的用户 ID';
COMMENT ON COLUMN team_lending_policy.updated_by_user_id IS '最后更新该策略的用户 ID';
COMMENT ON COLUMN team_lending_policy.created_at IS '创建时间';
COMMENT ON COLUMN team_lending_policy.updated_at IS '最后更新时间';

CREATE TABLE team_lending_request (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    team_id UUID NOT NULL,
    project_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    requested_by_user_id UUID NOT NULL,
    request_reason TEXT NOT NULL DEFAULT '',
    requested_budget NUMERIC,
    requested_capability JSONB NOT NULL DEFAULT '{}'::jsonb,
    granted_budget NUMERIC,
    granted_capability JSONB NOT NULL DEFAULT '{}'::jsonb,
    is_exception BOOLEAN NOT NULL DEFAULT false,
    decided_by_user_id UUID,
    decided_at TIMESTAMPTZ,
    decision_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE team_lending_request
    ADD CONSTRAINT chk_team_lending_request_status
    CHECK (status IN ('pending', 'auto_approved', 'approved', 'rejected', 'revoked'));

CREATE UNIQUE INDEX uq_team_lending_request_active
    ON team_lending_request(tenant_id, project_id, team_id)
    WHERE status IN ('pending', 'auto_approved', 'approved');

CREATE INDEX idx_team_lending_request_tenant_team_status
    ON team_lending_request(tenant_id, team_id, status);

CREATE INDEX idx_team_lending_request_tenant_project_status
    ON team_lending_request(tenant_id, project_id, status);

CREATE TRIGGER update_team_lending_request_updated_at
    BEFORE UPDATE ON team_lending_request
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE team_lending_request IS '团队借调请求：项目向团队申请借调数字员工，团队负责人裁决（D1 团队级 (project, team) 授权）';
COMMENT ON COLUMN team_lending_request.id IS '借调请求 UUID';
COMMENT ON COLUMN team_lending_request.tenant_id IS '租户 ID';
COMMENT ON COLUMN team_lending_request.team_id IS '被借调的团队 ID（供给侧）';
COMMENT ON COLUMN team_lending_request.project_id IS '发起借调的项目 ID（需求侧）';
COMMENT ON COLUMN team_lending_request.status IS '请求状态：pending 待审批，auto_approved 策略自动放行，approved 人工通过，rejected 驳回，revoked 撤销';
COMMENT ON COLUMN team_lending_request.requested_by_user_id IS '发起借调的用户 ID（项目负责人）';
COMMENT ON COLUMN team_lending_request.request_reason IS '借调事由';
COMMENT ON COLUMN team_lending_request.requested_budget IS '申请的借调预算';
COMMENT ON COLUMN team_lending_request.requested_capability IS '申请使用的能力范围';
COMMENT ON COLUMN team_lending_request.granted_budget IS '实际授予的借调预算（≤ 策略天花板）';
COMMENT ON COLUMN team_lending_request.granted_capability IS '实际授予的能力范围（≤ 策略天花板）';
COMMENT ON COLUMN team_lending_request.is_exception IS '是否因超出策略预算/能力天花板而强制转人工审批';
COMMENT ON COLUMN team_lending_request.decided_by_user_id IS '裁决该请求的团队负责人用户 ID';
COMMENT ON COLUMN team_lending_request.decided_at IS '裁决时间';
COMMENT ON COLUMN team_lending_request.decision_reason IS '裁决说明';
COMMENT ON COLUMN team_lending_request.created_at IS '创建时间';
COMMENT ON COLUMN team_lending_request.updated_at IS '最后更新时间';
