-- Add team governance and digital employee effective configuration tables.
-- This forward migration backfills existing 002 databases after the early
-- rebuild-only initial schema absorbed these tables.

ALTER TABLE tenant_teams
    ADD COLUMN IF NOT EXISTS human_owner_user_id UUID;

CREATE TABLE IF NOT EXISTS tenant_team_config_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    constitution JSONB NOT NULL DEFAULT '{}'::jsonb,
    capability_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    context_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    artifact_contract JSONB NOT NULL DEFAULT '{}'::jsonb,
    internal_collaboration_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    runtime_scope_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    human_owner_user_id UUID,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, team_id, revision_number)
);

CREATE TABLE IF NOT EXISTS digital_employee_config_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    role_profile JSONB NOT NULL DEFAULT '{}'::jsonb,
    constitution_addendum JSONB NOT NULL DEFAULT '{}'::jsonb,
    capability_selection JSONB NOT NULL DEFAULT '{}'::jsonb,
    context_policy_override JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_policy_override JSONB NOT NULL DEFAULT '{}'::jsonb,
    output_contract_addendum JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'draft',
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, digital_employee_id, revision_number)
);

CREATE TABLE IF NOT EXISTS digital_employee_effective_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    tenant_team_config_revision_id UUID NOT NULL,
    employee_config_revision_id UUID NOT NULL,
    effective_config_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    validation_result JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(50) NOT NULL DEFAULT 'pending_approval',
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_tenant_team_config_revisions_current
    ON tenant_team_config_revisions(tenant_id, team_id)
    WHERE status = 'active' AND archived_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_tenant_team_config_revisions_team
    ON tenant_team_config_revisions(tenant_id, team_id, revision_number DESC);

CREATE INDEX IF NOT EXISTS idx_digital_employee_config_revisions_employee
    ON digital_employee_config_revisions(tenant_id, digital_employee_id, revision_number DESC);
CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_config_revisions_active
    ON digital_employee_config_revisions(tenant_id, digital_employee_id)
    WHERE status = 'active' AND archived_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_digital_employee_effective_configs_active
    ON digital_employee_effective_configs(tenant_id, digital_employee_id)
    WHERE status = 'approved' AND revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_digital_employee_effective_configs_employee
    ON digital_employee_effective_configs(tenant_id, digital_employee_id, created_at DESC);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_tenant_team_config_revisions_updated_at'
    ) THEN
        CREATE TRIGGER update_tenant_team_config_revisions_updated_at
        BEFORE UPDATE ON tenant_team_config_revisions
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_digital_employee_config_revisions_updated_at'
    ) THEN
        CREATE TRIGGER update_digital_employee_config_revisions_updated_at
        BEFORE UPDATE ON digital_employee_config_revisions
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_digital_employee_effective_configs_updated_at'
    ) THEN
        CREATE TRIGGER update_digital_employee_effective_configs_updated_at
        BEFORE UPDATE ON digital_employee_effective_configs
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON COLUMN tenant_teams.human_owner_user_id IS '团队负责人用户ID，第一版用于团队级审批、升级和跨团队交接决策';

COMMENT ON TABLE tenant_team_config_revisions IS '团队治理配置版本表';
COMMENT ON COLUMN tenant_team_config_revisions.id IS '团队治理配置版本ID';
COMMENT ON COLUMN tenant_team_config_revisions.tenant_id IS '租户ID';
COMMENT ON COLUMN tenant_team_config_revisions.team_id IS '团队ID';
COMMENT ON COLUMN tenant_team_config_revisions.revision_number IS '团队配置版本号，同一团队内递增';
COMMENT ON COLUMN tenant_team_config_revisions.constitution IS '团队公共宪法，定义硬性规则、工作原则和禁止行为';
COMMENT ON COLUMN tenant_team_config_revisions.capability_policy IS '团队公共能力策略，定义允许的MCP、技能、插件和Provider类型';
COMMENT ON COLUMN tenant_team_config_revisions.context_policy IS '团队公共上下文策略，定义可注入上下文来源、范围和保留规则';
COMMENT ON COLUMN tenant_team_config_revisions.approval_policy IS '团队公共审批策略，定义风险阈值和必须人类审批的动作';
COMMENT ON COLUMN tenant_team_config_revisions.artifact_contract IS '团队工件契约，定义交接时必须产出的结构化对象';
COMMENT ON COLUMN tenant_team_config_revisions.internal_collaboration_policy IS '团队内部协作策略，定义同团队数字员工自动问询的边界';
COMMENT ON COLUMN tenant_team_config_revisions.runtime_scope_policy IS '团队Runtime范围策略，定义可使用的执行节点、Provider和环境边界';
COMMENT ON COLUMN tenant_team_config_revisions.human_owner_user_id IS '该版本配置的团队负责人用户ID';
COMMENT ON COLUMN tenant_team_config_revisions.status IS '配置状态：draft、active、archived';
COMMENT ON COLUMN tenant_team_config_revisions.approved_by IS '批准该配置版本的用户ID';
COMMENT ON COLUMN tenant_team_config_revisions.approved_at IS '配置版本批准时间';
COMMENT ON COLUMN tenant_team_config_revisions.archived_at IS '配置版本归档时间';
COMMENT ON COLUMN tenant_team_config_revisions.created_at IS '创建时间';
COMMENT ON COLUMN tenant_team_config_revisions.updated_at IS '更新时间';

COMMENT ON TABLE digital_employee_config_revisions IS '数字员工个人治理配置版本表';
COMMENT ON COLUMN digital_employee_config_revisions.id IS '数字员工个人配置版本ID';
COMMENT ON COLUMN digital_employee_config_revisions.tenant_id IS '租户ID';
COMMENT ON COLUMN digital_employee_config_revisions.digital_employee_id IS '数字员工ID';
COMMENT ON COLUMN digital_employee_config_revisions.revision_number IS '个人配置版本号，同一数字员工内递增';
COMMENT ON COLUMN digital_employee_config_revisions.role_profile IS '角色画像，描述数字员工专业方向和职责';
COMMENT ON COLUMN digital_employee_config_revisions.constitution_addendum IS '个人宪法补充，只能收紧或补充团队宪法';
COMMENT ON COLUMN digital_employee_config_revisions.capability_selection IS '个人能力选择，只能从团队允许范围内启用MCP、技能和插件';
COMMENT ON COLUMN digital_employee_config_revisions.context_policy_override IS '个人上下文策略覆盖，只能收紧团队上下文策略';
COMMENT ON COLUMN digital_employee_config_revisions.approval_policy_override IS '个人审批策略覆盖，只能收紧团队审批策略';
COMMENT ON COLUMN digital_employee_config_revisions.output_contract_addendum IS '个人输出契约补充，定义额外交接工件要求';
COMMENT ON COLUMN digital_employee_config_revisions.status IS '配置状态：draft、pending_approval、active、archived';
COMMENT ON COLUMN digital_employee_config_revisions.approved_by IS '批准该个人配置版本的用户ID';
COMMENT ON COLUMN digital_employee_config_revisions.approved_at IS '个人配置版本批准时间';
COMMENT ON COLUMN digital_employee_config_revisions.archived_at IS '个人配置版本归档时间';
COMMENT ON COLUMN digital_employee_config_revisions.created_at IS '创建时间';
COMMENT ON COLUMN digital_employee_config_revisions.updated_at IS '更新时间';

COMMENT ON TABLE digital_employee_effective_configs IS '数字员工生效治理配置快照表';
COMMENT ON COLUMN digital_employee_effective_configs.id IS '生效配置快照ID';
COMMENT ON COLUMN digital_employee_effective_configs.tenant_id IS '租户ID';
COMMENT ON COLUMN digital_employee_effective_configs.digital_employee_id IS '数字员工ID';
COMMENT ON COLUMN digital_employee_effective_configs.tenant_team_config_revision_id IS '参与合成的团队配置版本ID';
COMMENT ON COLUMN digital_employee_effective_configs.employee_config_revision_id IS '参与合成的个人配置版本ID';
COMMENT ON COLUMN digital_employee_effective_configs.effective_config_snapshot IS '团队配置与个人配置合成后的生效治理配置快照';
COMMENT ON COLUMN digital_employee_effective_configs.validation_result IS '生效配置校验结果，包含阻断错误和警告';
COMMENT ON COLUMN digital_employee_effective_configs.status IS '生效配置状态：pending_approval、approved、revoked';
COMMENT ON COLUMN digital_employee_effective_configs.approved_by IS '批准生效配置的用户ID';
COMMENT ON COLUMN digital_employee_effective_configs.approved_at IS '生效配置批准时间';
COMMENT ON COLUMN digital_employee_effective_configs.revoked_at IS '生效配置撤销时间';
COMMENT ON COLUMN digital_employee_effective_configs.created_at IS '创建时间';
COMMENT ON COLUMN digital_employee_effective_configs.updated_at IS '更新时间';
