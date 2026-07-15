-- 能力词汇注册表：模板角色要求（required_capabilities）与员工能力声明共享的键词汇；
-- 场景差异走注册表插行，不走核心代码枚举。
CREATE TABLE IF NOT EXISTS capability_vocabulary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    vocab_key TEXT NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_capability_vocabulary_key_not_blank CHECK (btrim(vocab_key) <> ''),
    CONSTRAINT ck_capability_vocabulary_status_supported CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_capability_vocabulary_tenant_key_active
    ON capability_vocabulary(tenant_id, vocab_key)
    WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_capability_vocabulary_updated_at'
    ) THEN
        CREATE TRIGGER update_capability_vocabulary_updated_at
        BEFORE UPDATE ON capability_vocabulary
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE capability_vocabulary IS '租户级能力词汇注册表：场景模板角色 required_capabilities 与员工能力声明共享的键，插行即扩展，不建代码枚举';
COMMENT ON COLUMN capability_vocabulary.id IS '能力词汇主键 UUID';
COMMENT ON COLUMN capability_vocabulary.tenant_id IS '能力词汇所属租户 ID';
COMMENT ON COLUMN capability_vocabulary.vocab_key IS '能力键稳定标识（如 code_review），租户内未删除时唯一';
COMMENT ON COLUMN capability_vocabulary.title IS '能力键显示名（中文）';
COMMENT ON COLUMN capability_vocabulary.description IS '能力键含义说明';
COMMENT ON COLUMN capability_vocabulary.status IS '能力键状态：active/disabled';
COMMENT ON COLUMN capability_vocabulary.deleted_at IS '能力词汇软删除时间';
COMMENT ON COLUMN capability_vocabulary.created_at IS '能力词汇创建时间';
COMMENT ON COLUMN capability_vocabulary.updated_at IS '能力词汇更新时间';

INSERT INTO capability_vocabulary (id, tenant_id, vocab_key, title, description) VALUES
('00000000-0000-0000-0000-000000000501'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'code_implementation', '代码实现', '编写或修改代码以实现需求变更'),
('00000000-0000-0000-0000-000000000502'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'code_review', '代码审查', '独立评审代码变更，只审不改，结论须附证据指针'),
('00000000-0000-0000-0000-000000000503'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'test_execution', '测试执行', '编写并运行测试以验证变更行为符合预期'),
('00000000-0000-0000-0000-000000000504'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'log.analysis', '日志分析', '采集与分析系统运行日志、指标等原始证据'),
('00000000-0000-0000-0000-000000000505'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'incident.triage', '故障分诊', '对故障进行诊断、定位根因或验证修复效果')
ON CONFLICT (id) DO NOTHING;

-- 治理约束豁免记录：人类负责人对单需求豁免某条模板约束（如四眼审查、独立验证）的一等决策留痕；
-- 重规划时治理评估器消费该表以跳过已豁免的约束检查。
CREATE TABLE IF NOT EXISTS project_demand_constraint_exemptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    constraint_kind VARCHAR(64) NOT NULL,
    roles JSONB NOT NULL DEFAULT '[]'::jsonb,
    granted_by_user_id UUID NOT NULL,
    decision_request_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_demand_constraint_exemption UNIQUE (tenant_id, demand_id, constraint_kind)
);

CREATE INDEX IF NOT EXISTS idx_demand_constraint_exemptions_tenant_demand
    ON project_demand_constraint_exemptions(tenant_id, demand_id);

COMMENT ON TABLE project_demand_constraint_exemptions IS '治理约束豁免记录：人类负责人对单个需求豁免某条模板约束的决策留痕，重规划治理评估器消费';
COMMENT ON COLUMN project_demand_constraint_exemptions.id IS '豁免记录主键 UUID';
COMMENT ON COLUMN project_demand_constraint_exemptions.tenant_id IS '豁免记录所属租户 ID';
COMMENT ON COLUMN project_demand_constraint_exemptions.project_id IS '豁免所属项目 ID';
COMMENT ON COLUMN project_demand_constraint_exemptions.demand_id IS '被豁免约束所属的需求 ID';
COMMENT ON COLUMN project_demand_constraint_exemptions.constraint_kind IS '被豁免的约束种类（如 role_independence、stage_required）';
COMMENT ON COLUMN project_demand_constraint_exemptions.roles IS '豁免涉及的角色 key 列表';
COMMENT ON COLUMN project_demand_constraint_exemptions.granted_by_user_id IS '批准豁免的人类负责人用户 ID';
COMMENT ON COLUMN project_demand_constraint_exemptions.decision_request_id IS '关联的人类决策请求 ID，可空';
COMMENT ON COLUMN project_demand_constraint_exemptions.created_at IS '豁免记录创建时间';

-- 标准员工模板种子：系统内置的最小可用数字员工模板，覆盖场景模板 v2 最常见的审查/测试两类角色需求；
-- 租户可直接按模板一键创建数字员工，不必从零填写能力绑定与人格记忆。
INSERT INTO digital_employee_templates (
    id, tenant_id, type, label, description, default_role,
    recommended_skills, recommended_mcp_servers, recommended_provider_types,
    capability_bindings, budget_policy, metadata, persona_memory_markdown,
    status, is_system
) VALUES
('00000000-0000-0000-0000-000000000511'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'standard_code_reviewer', '标准代码审查员', '系统内置的标准代码审查员模板：独立评审代码变更，只审不改', '代码审查',
 '[]'::jsonb, '[]'::jsonb, '["claude-code"]'::jsonb,
 '{"skills":[],"mcp_servers":[],"external_capabilities":["code_review"],"environment_variable_refs":[]}'::jsonb,
 '{}'::jsonb, '{}'::jsonb,
 '# 标准代码审查员

## 角色定位
你是独立的代码审查员，只负责评审代码变更是否满足需求与质量要求，不代替开发者修改代码。

## 工作原则
- 保持独立性：不与被审查变更的开发者是同一人（四眼原则）。
- 只审不改：发现问题在评审结论中指出，不直接提交修复。
- 结论须可追溯：每条评审意见必须附具体证据指针（文件路径、行号、commit/diff 位置）。
- 模糊或高风险判断（如是否阻断发布）应交由人类负责人确认，不擅自拍板。',
 'active', true),
('00000000-0000-0000-0000-000000000512'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'standard_tester', '标准测试员', '系统内置的标准测试员模板：编写并执行测试以验证变更行为', '测试',
 '[]'::jsonb, '[]'::jsonb, '["claude-code"]'::jsonb,
 '{"skills":[],"mcp_servers":[],"external_capabilities":["test_execution"],"environment_variable_refs":[]}'::jsonb,
 '{}'::jsonb, '{}'::jsonb,
 '# 标准测试员

## 角色定位
你是测试员，负责针对变更编写并执行测试，验证行为是否符合预期。

## 工作原则
- 覆盖主路径：测试至少覆盖变更引入或影响的主要执行路径。
- 结论须可追溯：测试报告必须给出可复现的执行方式与证据指针（测试文件、运行日志）。
- 失败即阻断：测试失败时如实报告，不弱化结论、不擅自判定为可接受的已知问题。
- 模糊或高风险判断应交由人类负责人确认，不擅自拍板。',
 'active', true)
ON CONFLICT (id) DO NOTHING;
