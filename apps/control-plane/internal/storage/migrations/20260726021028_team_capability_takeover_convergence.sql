-- 团队接管收敛（spec 2026-07-26-team-configuration-console-design §5.2.1）：
-- 同一 MCP / 技能在团队与员工两个维度只允许存在一条生效绑定，团队胜出。
--
-- 此前只有读路径靠 NOT EXISTS 静默屏蔽员工侧重复项，库里的重复行一直留着，造成：
--   ① 员工页「个人绑定」看得见、「生效配置」看不见，同页自相矛盾；
--   ② 员工那条的 credential_env_var 被无声丢弃；
--   ③ 团队一旦解绑，员工侧陈年旧绑定会自动"复活"进生效集合。
-- 这里把历史重复行一次性物理收敛掉；之后由写路径在绑定时接管，不再产生新的重复。
--
-- 验收判据：执行后下面两个 SELECT 都应返回 0 行（即读路径的 NOT EXISTS 兜底
-- 已无可屏蔽项）。

-- MCP：软删本团队成员的同 server 个人绑定。
UPDATE digital_employee_mcp_bindings_v2 eb
SET deleted_at = NOW(),
    updated_at = NOW()
FROM digital_employees de
JOIN team_mcp_bindings tmb
  ON tmb.tenant_id = de.tenant_id
 AND tmb.team_id = de.team_id
 AND tmb.deleted_at IS NULL
WHERE de.id = eb.digital_employee_id
  AND de.tenant_id = eb.tenant_id
  AND de.deleted_at IS NULL
  AND eb.deleted_at IS NULL
  AND tmb.mcp_server_id = eb.mcp_server_id;

-- 技能：skill_agent_bindings 是硬删模型，直接删除本团队成员的同 skill 个人绑定。
DELETE FROM skill_agent_bindings sab
USING digital_employees de,
      team_skill_bindings tsb
WHERE de.id = sab.digital_employee_id
  AND de.tenant_id = sab.tenant_id
  AND de.deleted_at IS NULL
  AND tsb.tenant_id = de.tenant_id
  AND tsb.team_id = de.team_id
  AND tsb.skill_id = sab.skill_id;
