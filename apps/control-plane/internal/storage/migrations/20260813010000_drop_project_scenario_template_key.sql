-- P0（2026-08-13-autonomy-envelope-policy-playbook-design §6）：
-- 删除项目级剧本绑定。剧本仅在发起时（demand / 规则 / 接续）显式选择，
-- 解析顺序收敛为 demand 显式 > generic；编制表 project_playbook_casting 不动。

COMMENT ON COLUMN project_demands.scenario_template_key IS
  '需求级场景模板 key；解析顺序：需求显式 > generic 兜底（已无项目默认回落）';

ALTER TABLE projects DROP COLUMN IF EXISTS scenario_template_key;
