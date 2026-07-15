-- 场景模板版本表：模板 spec 的不可变历史快照；主表 spec 始终镜像 active 版本，
-- 版本表供审计血缘与（后续）计划钉住回读。exits 数组按由浅到深声明，
-- 约束条件 exit_at_or_beyond 按 exits 下标比较。
CREATE TABLE IF NOT EXISTS scenario_template_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    template_id UUID NOT NULL,
    version INT NOT NULL,
    spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_scenario_template_versions_template_version UNIQUE (template_id, version)
);
CREATE INDEX IF NOT EXISTS idx_scenario_template_versions_tenant_template
    ON scenario_template_versions(tenant_id, template_id, version DESC);

COMMENT ON TABLE scenario_template_versions IS '场景模板 spec 的不可变版本历史，供审计血缘与计划钉住';
COMMENT ON COLUMN scenario_template_versions.id IS '版本记录主键 UUID';
COMMENT ON COLUMN scenario_template_versions.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN scenario_template_versions.template_id IS '所属场景模板 ID';
COMMENT ON COLUMN scenario_template_versions.version IS '版本号，从 1 单调递增';
COMMENT ON COLUMN scenario_template_versions.spec IS '该版本的完整模板 spec JSONB';
COMMENT ON COLUMN scenario_template_versions.created_by IS '创建该版本的用户 ID，迁移生成为 NULL';
COMMENT ON COLUMN scenario_template_versions.created_at IS '版本创建时间';

ALTER TABLE scenario_templates ADD COLUMN IF NOT EXISTS active_version INT NOT NULL DEFAULT 1;
COMMENT ON COLUMN scenario_templates.active_version IS '当前生效版本号，主表 spec 始终镜像该版本内容';

ALTER TABLE project_demands ADD COLUMN IF NOT EXISTS scenario_template_key TEXT;
COMMENT ON COLUMN project_demands.scenario_template_key IS '需求级场景模板 key；解析顺序：需求显式 > 项目默认 > generic 兜底';

-- 存量模板 spec 归档为 v1
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 1, spec FROM scenario_templates WHERE deleted_at IS NULL
ON CONFLICT (template_id, version) DO NOTHING;

-- 五个种子模板升级到 v2：spec 补齐 exits（由浅到深声明的交付出口）、
-- constraints（role_independence 四眼原则 / stage_required 阶段必经 / human_gate 人类关卡，
-- 均以 exit_at_or_beyond 相对 exits 下标比较触发）、collapse_rules（角色可合并同人执行）、
-- default_acceptance_criteria（statement + applies_from_exit，验收标准按出口深度累加生效）。
-- roles 去除 v1 的 collapsible_with/independent_from（语义迁移到 constraints/collapse_rules）；
-- risk_policy 语义迁移到 constraints 的 human_gate。software_delivery v2 新增 release 骨架步骤。

-- software_delivery（00000000-0000-0000-0000-000000000401）
UPDATE scenario_templates SET active_version = 2, spec =
'{"spec_version":2,"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"branch_ref","kind":"branch_ref"},{"name":"head_commit","kind":"git_commit"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]},{"step":"release","role":"developer","depends_on":["review","test"],"required_inputs_defaults":["review_verdict","test_report"],"produces_defaults":[{"name":"release_record","kind":"evidence_ref"}]}],"exits":[{"deliverable":"branch_ref","label":"交付分支（不合入）"},{"deliverable":"review_verdict","label":"审查通过并合入"},{"deliverable":"release_record","label":"发布上线"}],"constraints":[{"kind":"role_independence","roles":["reviewer","developer"],"when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"review","when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"test","when":{"exit_at_or_beyond":"release_record"}},{"kind":"human_gate","target":"release","when":{"exit_at_or_beyond":"release_record"}}],"collapse_rules":[{"roles":["developer","tester"]}],"default_acceptance_criteria":[{"statement":"变更以 branch+commit 交付","applies_from_exit":"branch_ref"},{"statement":"通过独立审查","applies_from_exit":"review_verdict"},{"statement":"测试报告覆盖主路径且结论可判","applies_from_exit":"release_record"}],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000401'::uuid;
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 2, spec FROM scenario_templates WHERE id = '00000000-0000-0000-0000-000000000401'::uuid
ON CONFLICT (template_id, version) DO NOTHING;

-- ops_analysis（00000000-0000-0000-0000-000000000402）
UPDATE scenario_templates SET active_version = 2, spec =
'{"spec_version":2,"roles":[{"key":"collector","title":"采集","required_capabilities":["log.analysis"]},{"key":"analyst","title":"分析","required_capabilities":["incident.triage"]}],"skeleton":[{"step":"collect","role":"collector","produces_defaults":[{"name":"raw_metrics","kind":"evidence_ref"}]},{"step":"analyze","role":"analyst","depends_on":["collect"],"required_inputs_defaults":["raw_metrics"],"produces_defaults":[{"name":"analysis_conclusion","kind":"conclusion"}]}],"exits":[{"deliverable":"raw_metrics","label":"仅采集数据"},{"deliverable":"analysis_conclusion","label":"给出分析结论"}],"constraints":[],"collapse_rules":[{"roles":["collector","analyst"]}],"default_acceptance_criteria":[{"statement":"结论附证据指针，可追溯到采集数据","applies_from_exit":"analysis_conclusion"}],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000402'::uuid;
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 2, spec FROM scenario_templates WHERE id = '00000000-0000-0000-0000-000000000402'::uuid
ON CONFLICT (template_id, version) DO NOTHING;

-- incident_response（00000000-0000-0000-0000-000000000403）
UPDATE scenario_templates SET active_version = 2, spec =
'{"spec_version":2,"roles":[{"key":"diagnostician","title":"诊断","required_capabilities":["incident.triage"]},{"key":"operator","title":"修复","required_capabilities":["incident.triage"]},{"key":"verifier","title":"验证","required_capabilities":["incident.triage"]}],"skeleton":[{"step":"diagnose","role":"diagnostician","produces_defaults":[{"name":"root_cause","kind":"conclusion"}]},{"step":"fix","role":"operator","depends_on":["diagnose"],"required_inputs_defaults":["root_cause"],"produces_defaults":[{"name":"fix_record","kind":"evidence_ref"}]},{"step":"verify","role":"verifier","depends_on":["fix"],"required_inputs_defaults":["fix_record"],"produces_defaults":[{"name":"verification_result","kind":"conclusion"}]}],"exits":[{"deliverable":"root_cause","label":"仅诊断根因"},{"deliverable":"fix_record","label":"实施修复"},{"deliverable":"verification_result","label":"修复并独立验证"}],"constraints":[{"kind":"role_independence","roles":["verifier","operator"],"when":{"exit_at_or_beyond":"verification_result"}},{"kind":"stage_required","step":"verify","when":{"exit_at_or_beyond":"verification_result"}}],"collapse_rules":[{"roles":["diagnostician","operator"]}],"default_acceptance_criteria":[{"statement":"根因结论与修复记录可相互印证","applies_from_exit":"fix_record"},{"statement":"验证结果由非修复者出具","applies_from_exit":"verification_result"}],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000403'::uuid;
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 2, spec FROM scenario_templates WHERE id = '00000000-0000-0000-0000-000000000403'::uuid
ON CONFLICT (template_id, version) DO NOTHING;

-- research_report（00000000-0000-0000-0000-000000000404）
UPDATE scenario_templates SET active_version = 2, spec =
'{"spec_version":2,"roles":[{"key":"researcher","title":"检索","required_capabilities":[]},{"key":"writer","title":"成稿","required_capabilities":[]}],"skeleton":[{"step":"search","role":"researcher","produces_defaults":[{"name":"source_list","kind":"evidence_ref"}]},{"step":"synthesize","role":"writer","depends_on":["search"],"required_inputs_defaults":["source_list"],"produces_defaults":[{"name":"final_report","kind":"artifact_ref"}]}],"exits":[{"deliverable":"source_list","label":"仅出来源清单"},{"deliverable":"final_report","label":"成稿"}],"constraints":[],"collapse_rules":[{"roles":["researcher","writer"]}],"default_acceptance_criteria":[{"statement":"报告结论均有来源清单支撑","applies_from_exit":"final_report"}],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000404'::uuid;
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 2, spec FROM scenario_templates WHERE id = '00000000-0000-0000-0000-000000000404'::uuid
ON CONFLICT (template_id, version) DO NOTHING;

-- generic（00000000-0000-0000-0000-000000000405）
UPDATE scenario_templates SET active_version = 2, spec =
'{"spec_version":2,"roles":[],"skeleton":[],"exits":[],"constraints":[],"collapse_rules":[],"default_acceptance_criteria":[],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000405'::uuid;
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 2, spec FROM scenario_templates WHERE id = '00000000-0000-0000-0000-000000000405'::uuid
ON CONFLICT (template_id, version) DO NOTHING;
