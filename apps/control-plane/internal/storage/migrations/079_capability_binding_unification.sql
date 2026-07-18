-- 079_capability_binding_unification.sql
-- 能力逻辑绑定统一(spec: 员工能力=绑定表唯一权威源):
-- digital_employee_config_revisions.capability_bindings JSONB 中的 skills/mcp_servers
-- 声明退役。历史声明回填为真实绑定行(用户拍板:创建向导里选过但从未生效的选择在
-- 下次派发真实生效),随后从全量 revision 中剥离两 key。
-- skill_installations 表冻结(应用层停写停读),留存追溯,待懒同步稳定后另行 drop。

-- 1) Backfill 技能声明 → skill_agent_bindings。
--    取每员工最新 revision 的 capability_bindings->'skills',按租户内活跃 slug 解析;
--    员工仍存续(未删)、且未被团队绑定继承覆盖的组合才补行。不可解析的 slug 属死数据丢弃。
INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status)
SELECT DISTINCT r.tenant_id, s.id, r.digital_employee_id, 'enabled'
FROM (
    SELECT DISTINCT ON (tenant_id, digital_employee_id)
        tenant_id, digital_employee_id, capability_bindings
    FROM digital_employee_config_revisions
    ORDER BY tenant_id, digital_employee_id, revision_number DESC
) r
CROSS JOIN LATERAL jsonb_array_elements_text(
    CASE WHEN jsonb_typeof(r.capability_bindings->'skills') = 'array'
         THEN r.capability_bindings->'skills'
         ELSE '[]'::jsonb END
) AS slug(value)
JOIN skills s
    ON s.tenant_id = r.tenant_id
   AND s.slug = slug.value
   AND s.deleted_at IS NULL
JOIN digital_employees de
    ON de.id = r.digital_employee_id
   AND de.tenant_id = r.tenant_id
   AND de.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM team_skill_bindings tsb
    WHERE tsb.tenant_id = r.tenant_id
      AND tsb.team_id = de.team_id
      AND tsb.skill_id = s.id
)
ON CONFLICT (tenant_id, skill_id, digital_employee_id)
DO UPDATE SET status = 'enabled', updated_at = NOW();

-- 2) Backfill MCP 声明 → digital_employee_mcp_bindings_v2。
--    按租户内 active server_key 解析;已被团队 MCP 绑定继承覆盖的组合跳过。
INSERT INTO digital_employee_mcp_bindings_v2 (tenant_id, digital_employee_id, mcp_server_id)
SELECT DISTINCT r.tenant_id, r.digital_employee_id, m.id
FROM (
    SELECT DISTINCT ON (tenant_id, digital_employee_id)
        tenant_id, digital_employee_id, capability_bindings
    FROM digital_employee_config_revisions
    ORDER BY tenant_id, digital_employee_id, revision_number DESC
) r
CROSS JOIN LATERAL jsonb_array_elements_text(
    CASE WHEN jsonb_typeof(r.capability_bindings->'mcp_servers') = 'array'
         THEN r.capability_bindings->'mcp_servers'
         ELSE '[]'::jsonb END
) AS server_key(value)
JOIN mcp_servers m
    ON m.tenant_id = r.tenant_id
   AND m.server_key = server_key.value
   AND m.deleted_at IS NULL
   AND m.status = 'active'
JOIN digital_employees de
    ON de.id = r.digital_employee_id
   AND de.tenant_id = r.tenant_id
   AND de.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM team_mcp_bindings tmb
    WHERE tmb.tenant_id = r.tenant_id
      AND tmb.team_id = de.team_id
      AND tmb.mcp_server_id = m.id
      AND tmb.deleted_at IS NULL
      AND tmb.status = 'active'
)
ON CONFLICT (tenant_id, digital_employee_id, mcp_server_id) WHERE deleted_at IS NULL
DO NOTHING;

-- 3) Strip:全量 revision(含 archived)剥离 skills/mcp_servers 两 key,
--    保证不再有任何读到旧声明的路径。capability_bindings 只保留
--    external_capabilities / environment_variable_refs 职责。
UPDATE digital_employee_config_revisions
SET capability_bindings = capability_bindings - 'skills' - 'mcp_servers'
WHERE capability_bindings ? 'skills' OR capability_bindings ? 'mcp_servers';

COMMENT ON COLUMN digital_employee_config_revisions.capability_bindings IS
    '能力声明(仅 external_capabilities/environment_variable_refs);技能与 MCP 逻辑绑定的唯一权威源是 skill_agent_bindings/team_skill_bindings 与 digital_employee_mcp_bindings_v2/team_mcp_bindings';
COMMENT ON TABLE skill_installations IS
    '已冻结(2026-07-19 能力绑定统一):物理安装事实由派发时 runtime 懒收敛 + attestation 承载,本表停写停读留存追溯,待另行 drop';
