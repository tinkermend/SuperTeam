-- 剧本可落地化 批一（对齐讨论 2026-08-04）：清测试残留、统一能力词表、
-- 给"对外生效"的步骤补人类闸、清死字段、给空租户补内置剧本。
--
-- 背景：能力词表(capability_vocabulary)是模板 required_capabilities 与员工
-- external_capabilities 的共用词表，但此前只有模板侧被校验，员工侧随便写，
-- 于是出现"词表里注册的键 0 人声明 / 员工在用的键根本没注册"的两头落空。

-- ── 1. 退役 E2E 残留 ─────────────────────────────────────────────────
-- 这些是历史 E2E 造出来的，却是 active，会出现在用户的剧本选择器里。
-- 只置 disabled 不删除：历史 demand 仍引用它们，删了会让卷宗解析不出剧本名。
UPDATE scenario_templates
SET status = 'disabled', updated_at = NOW()
WHERE template_key IN ('adv_review_e2e_123656', 'g4_sod_staffgap')
  AND status = 'active';

-- 同理退役 E2E 词表键（g4_sod_staffgap 专用，随模板一起下线）。
UPDATE capability_vocabulary
SET status = 'disabled', updated_at = NOW()
WHERE vocab_key IN ('change_request', 'change_approval')
  AND status = 'active';

-- ── 2. 注册员工实际在用、却从未进词表的能力键 ────────────────────────
-- 员工侧写入不过词表校验（本批同步补上代码校验），已产生这些"野生"键。
-- 先把真实在用的登记为正式词汇，再开校验，否则存量员工一保存就 400。
INSERT INTO capability_vocabulary (id, tenant_id, vocab_key, title, description, status)
SELECT gen_random_uuid(), t.id, v.vocab_key, v.title, v.description, 'active'
FROM tenants t
CROSS JOIN (VALUES
    ('research',              '资料检索',   '检索并甄别外部资料，产出可追溯的来源清单'),
    ('writing',               '成稿写作',   '把结论与证据组织成面向人的稿件'),
    ('report_synthesis',      '报告综合',   '跨来源归纳结论并形成结构化报告'),
    ('information_retrieval', '信息提取',   '从既有系统或文档中定位并提取所需信息')
) AS v(vocab_key, title, description)
WHERE t.id = '00000000-0000-0000-0000-000000000001'::uuid
  AND NOT EXISTS (
    SELECT 1 FROM capability_vocabulary c
    WHERE c.tenant_id = t.id AND c.vocab_key = v.vocab_key
  );

-- ── 3. 统一命名风格：点号 → 下划线 ───────────────────────────────────
-- 词表内部同时存在 code_implementation 与 log.analysis 两种风格；不统一的话
-- 每新增一个键都要猜该用哪种。这两个点号键 0 名员工声明，改名无存量影响。
UPDATE capability_vocabulary SET vocab_key = 'log_analysis', updated_at = NOW()
WHERE vocab_key = 'log.analysis';
UPDATE capability_vocabulary SET vocab_key = 'incident_triage', updated_at = NOW()
WHERE vocab_key = 'incident.triage';

UPDATE scenario_templates
SET spec = replace(replace(spec::text, '"log.analysis"', '"log_analysis"'),
                   '"incident.triage"', '"incident_triage"')::jsonb,
    updated_at = NOW()
WHERE spec::text LIKE '%log.analysis%' OR spec::text LIKE '%incident.triage%';

UPDATE scenario_template_versions
SET spec = replace(replace(spec::text, '"log.analysis"', '"log_analysis"'),
                   '"incident.triage"', '"incident_triage"')::jsonb
WHERE spec::text LIKE '%log.analysis%' OR spec::text LIKE '%incident.triage%';

-- 员工侧同名回填（当前无人声明这两个键，写成幂等以防并发新增）。
UPDATE digital_employee_config_revisions
SET capability_bindings = replace(replace(capability_bindings::text, '"log.analysis"', '"log_analysis"'),
                                  '"incident.triage"', '"incident_triage"')::jsonb
WHERE capability_bindings::text LIKE '%log.analysis%'
   OR capability_bindings::text LIKE '%incident.triage%';

-- ── 4. 给"对外生效且难回滚"的步骤补人类闸 ────────────────────────────
-- 基线 §1：打断人的时机是**权力边界变化**（从建议到对外生效、从可逆到难回滚），
-- 不是每个任务完成。故障排查的 fix 是真的去改生产系统，却一直没有闸；而
-- 运维分析的 collect 是只读采集，**故意不加**——给只读动作加闸正是基线否决的
-- 「每个动作都要人授权」。
UPDATE scenario_templates
SET spec = jsonb_set(
        spec,
        '{constraints}',
        coalesce(spec->'constraints', '[]'::jsonb) ||
        '[{"kind":"human_gate","target":"fix","when":{"exit_at_or_beyond":"fix_record"}}]'::jsonb
    ),
    updated_at = NOW()
WHERE template_key = 'incident_response'
  AND NOT (spec::text LIKE '%"human_gate"%');

-- ── 5. 清死字段 feasibility_thresholds ───────────────────────────────
-- 解析了但从不使用：真实阈值取自项目 coordination_policy
-- (selectionScoreThreshold)。留在模板里会让人以为改它有用。
UPDATE scenario_templates
SET spec = spec - 'feasibility_thresholds', updated_at = NOW()
WHERE spec ? 'feasibility_thresholds';

UPDATE scenario_template_versions
SET spec = spec - 'feasibility_thresholds'
WHERE spec ? 'feasibility_thresholds';

-- ── 6. 给没有任何剧本的租户补内置剧本 ────────────────────────────────
-- 内置剧本此前只写给默认租户，其余租户开箱 0 个剧本、连 generic 兜底都没有，
-- 「剧本」这套能力对它们等于不存在。租户目前没有创建 API（只在迁移里产生），
-- 因此这里按现存空租户补齐；未来若引入租户开通流程，需在那里挂 seed 钩子。
INSERT INTO scenario_templates (id, tenant_id, template_key, name, description, spec, status, active_version)
SELECT gen_random_uuid(), t.id, src.template_key, src.name, src.description, src.spec, src.status, src.active_version
FROM tenants t
CROSS JOIN (
    SELECT template_key, name, description, spec, status, active_version
    FROM scenario_templates
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
      AND template_key IN ('generic', 'software_delivery', 'ops_analysis', 'incident_response', 'research_report')
      AND deleted_at IS NULL
) src
WHERE NOT EXISTS (
    SELECT 1 FROM scenario_templates existing
    WHERE existing.tenant_id = t.id AND existing.deleted_at IS NULL
);

-- 同步内置能力词表，否则新补的剧本引用的键在该租户不存在，
-- 模板一经编辑就会被 validateSpecVocabulary 拒掉。
INSERT INTO capability_vocabulary (id, tenant_id, vocab_key, title, description, status)
SELECT gen_random_uuid(), t.id, src.vocab_key, src.title, src.description, src.status
FROM tenants t
CROSS JOIN (
    SELECT vocab_key, title, description, status
    FROM capability_vocabulary
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'::uuid
      AND status = 'active'
) src
WHERE NOT EXISTS (
    SELECT 1 FROM capability_vocabulary existing
    WHERE existing.tenant_id = t.id AND existing.vocab_key = src.vocab_key
);
