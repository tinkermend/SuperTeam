-- 场景模板注册表：租户级"这类场景该怎么干"的沉淀层（角色契约集合 + 分解骨架 + 默认交接契约）。
-- 内容有限、机制开放：加一类场景 = 插一行数据，核心代码不建枚举。
-- 项目在创建时绑定 template_key（可空 = generic 兜底，行为与无模板一致）；
-- 规划快照按 key 装载 spec 注入 planner，planner 输出的 template_key 必须与之一致。

CREATE TABLE IF NOT EXISTS scenario_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    template_key TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_scenario_templates_key_not_blank CHECK (btrim(template_key) <> ''),
    CONSTRAINT ck_scenario_templates_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT ck_scenario_templates_status_supported CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_scenario_templates_tenant_key_active
    ON scenario_templates(tenant_id, template_key)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_scenario_templates_tenant_status
    ON scenario_templates(tenant_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_scenario_templates_updated_at'
    ) THEN
        CREATE TRIGGER update_scenario_templates_updated_at
        BEFORE UPDATE ON scenario_templates
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

ALTER TABLE projects ADD COLUMN IF NOT EXISTS scenario_template_key TEXT;

COMMENT ON TABLE scenario_templates IS '租户级场景模板注册表，驱动规划分解与交接契约实例化';
COMMENT ON COLUMN scenario_templates.id IS '场景模板主键 UUID';
COMMENT ON COLUMN scenario_templates.tenant_id IS '场景模板所属租户 ID';
COMMENT ON COLUMN scenario_templates.template_key IS '模板稳定标识（如 software_delivery），租户内未删除时唯一';
COMMENT ON COLUMN scenario_templates.name IS '模板显示名';
COMMENT ON COLUMN scenario_templates.description IS '模板适用场景说明';
COMMENT ON COLUMN scenario_templates.spec IS '模板内容：roles/skeleton/default_acceptance_criteria/risk_policy/feasibility_thresholds';
COMMENT ON COLUMN scenario_templates.status IS '模板状态：active/disabled';
COMMENT ON COLUMN scenario_templates.deleted_at IS '模板软删除时间';
COMMENT ON COLUMN scenario_templates.created_by IS '创建模板的用户 ID';
COMMENT ON COLUMN scenario_templates.created_at IS '模板创建时间';
COMMENT ON COLUMN scenario_templates.updated_at IS '模板更新时间';
COMMENT ON COLUMN projects.scenario_template_key IS '项目绑定的场景模板 key，可空 = generic 兜底（行为同无模板）';

INSERT INTO scenario_templates (id, tenant_id, template_key, name, description, spec) VALUES
('00000000-0000-0000-0000-000000000401'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'software_delivery', '软件开发',
 '开发→审查→测试的软件交付场景；审查者与开发者必须不同人（四眼原则），开发与测试可同人。',
 '{"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"],"collapsible_with":["tester"],"independent_from":[]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"],"collapsible_with":[],"independent_from":["developer"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"],"collapsible_with":["developer"],"independent_from":[]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"head_commit","kind":"git_commit"},{"name":"branch_ref","kind":"branch_ref"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]}],"default_acceptance_criteria":["变更以 branch+commit 交付且通过独立审查","测试报告覆盖主路径且结论可判"],"risk_policy":{"release_requires_human":true},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000402'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'ops_analysis', '运维分析',
 '采集→分析的系统运行分析场景；分析可与采集同人。',
 '{"roles":[{"key":"collector","title":"采集","required_capabilities":["log.analysis"],"collapsible_with":["analyst"],"independent_from":[]},{"key":"analyst","title":"分析","required_capabilities":["incident.triage"],"collapsible_with":["collector"],"independent_from":[]}],"skeleton":[{"step":"collect","role":"collector","produces_defaults":[{"name":"raw_metrics","kind":"evidence_ref"}]},{"step":"analyze","role":"analyst","depends_on":["collect"],"required_inputs_defaults":["raw_metrics"],"produces_defaults":[{"name":"analysis_conclusion","kind":"conclusion"}]}],"default_acceptance_criteria":["结论附证据指针，可追溯到采集数据"],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000403'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'incident_response', '故障排查',
 '诊断→修复→验证的故障处置场景；验证者与修复者必须不同人。',
 '{"roles":[{"key":"diagnostician","title":"诊断","required_capabilities":["incident.triage"],"collapsible_with":["operator"],"independent_from":[]},{"key":"operator","title":"修复","required_capabilities":["incident.triage"],"collapsible_with":["diagnostician"],"independent_from":[]},{"key":"verifier","title":"验证","required_capabilities":["incident.triage"],"collapsible_with":[],"independent_from":["operator"]}],"skeleton":[{"step":"diagnose","role":"diagnostician","produces_defaults":[{"name":"root_cause","kind":"conclusion"}]},{"step":"fix","role":"operator","depends_on":["diagnose"],"required_inputs_defaults":["root_cause"],"produces_defaults":[{"name":"fix_record","kind":"evidence_ref"}]},{"step":"verify","role":"verifier","depends_on":["fix"],"required_inputs_defaults":["fix_record"],"produces_defaults":[{"name":"verification_result","kind":"conclusion"}]}],"default_acceptance_criteria":["根因结论与修复记录可相互印证","验证结果由非修复者出具"],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000404'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'research_report', '调研报告',
 '检索→综合成稿的调研场景；两阶段可同人。',
 '{"roles":[{"key":"researcher","title":"检索","required_capabilities":[],"collapsible_with":["writer"],"independent_from":[]},{"key":"writer","title":"成稿","required_capabilities":[],"collapsible_with":["researcher"],"independent_from":[]}],"skeleton":[{"step":"search","role":"researcher","produces_defaults":[{"name":"source_list","kind":"evidence_ref"}]},{"step":"synthesize","role":"writer","depends_on":["search"],"required_inputs_defaults":["source_list"],"produces_defaults":[{"name":"final_report","kind":"artifact_ref"}]}],"default_acceptance_criteria":["报告结论均有来源清单支撑"],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000405'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'generic', '通用兜底',
 '无场景约束的兜底模板：不注入骨架，规划行为与未绑定模板完全一致。',
 '{"roles":[],"skeleton":[],"default_acceptance_criteria":[],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb)
ON CONFLICT (id) DO NOTHING;
