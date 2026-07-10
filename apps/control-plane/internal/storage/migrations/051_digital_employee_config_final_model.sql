-- Development-only destructive reset for the final digital-employee config model.
-- Current development data is intentionally discarded instead of migrated from the
-- old personal-governance config shape.

DELETE FROM digital_employee_environment_variables;
DELETE FROM skill_installations;
DELETE FROM digital_employee_mcp_bindings_v2;
DELETE FROM digital_employee_mcp_bindings;
DELETE FROM skill_agent_bindings;
DELETE FROM project_employee_node_affinity;

UPDATE project_task_attempts
SET digital_employee_id = NULL
WHERE digital_employee_id IS NOT NULL;

UPDATE project_tasks
SET assigned_digital_employee_id = NULL,
    digital_employee_run_id = NULL
WHERE assigned_digital_employee_id IS NOT NULL
   OR digital_employee_run_id IS NOT NULL;

DELETE FROM task_runs
WHERE digital_employee_id IS NOT NULL;

DELETE FROM digital_employee_workspace_file_syncs;
DELETE FROM digital_employee_workspace_file_revisions
WHERE file_id IN (SELECT id FROM digital_employee_workspace_files);
DELETE FROM digital_employee_workspace_files;
DELETE FROM runtime_command_receipts
WHERE resource_type = 'digital_employee_execution_instance';
DELETE FROM provider_session_events
WHERE digital_employee_id IS NOT NULL;
DELETE FROM provider_sessions
WHERE digital_employee_id IS NOT NULL;
DELETE FROM digital_employee_execution_instances;
DELETE FROM digital_employee_config_revisions;
DELETE FROM digital_employees;

ALTER TABLE digital_employee_config_revisions
    ADD COLUMN IF NOT EXISTS persona_memory_markdown TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE digital_employee_config_revisions
    DROP COLUMN IF EXISTS role_profile,
    DROP COLUMN IF EXISTS constitution_addendum,
    DROP COLUMN IF EXISTS capability_selection,
    DROP COLUMN IF EXISTS context_policy_override,
    DROP COLUMN IF EXISTS approval_policy_override,
    DROP COLUMN IF EXISTS output_contract_addendum;

COMMENT ON TABLE digital_employee_config_revisions IS '数字员工个人配置版本表，保存人格记忆、能力绑定和预算策略';
COMMENT ON COLUMN digital_employee_config_revisions.persona_memory_markdown IS '数字员工人格记忆 Markdown，描述人格画像、专业边界、工作方式和表达偏好';
COMMENT ON COLUMN digital_employee_config_revisions.capability_bindings IS '数字员工能力绑定，保存 Skill、MCP、外部能力和环境变量引用';
COMMENT ON COLUMN digital_employee_config_revisions.budget_policy IS '数字员工预算策略，包含每日 token 上限；空对象表示无预算上限';
