-- Support team-less (tenant-level) digital employees.
-- The effective config snapshot no longer requires a team config revision source.

ALTER TABLE digital_employee_effective_configs
    ALTER COLUMN tenant_team_config_revision_id DROP NOT NULL;

COMMENT ON COLUMN digital_employee_effective_configs.tenant_team_config_revision_id IS '参与合成的团队配置版本ID；无团队数字员工为空表示使用租户级默认治理';

-- Workspace files and environment variables may belong to team-less employees.
ALTER TABLE digital_employee_workspace_files
    ALTER COLUMN team_id DROP NOT NULL;

ALTER TABLE digital_employee_environment_variables
    ALTER COLUMN team_id DROP NOT NULL;
ALTER TABLE digital_employee_environment_variables
    DROP CONSTRAINT IF EXISTS digital_employee_environment_variables_team_id_fkey;

COMMENT ON COLUMN digital_employee_workspace_files.team_id IS '文件所属数字员工团队 ID；无团队数字员工为空';
COMMENT ON COLUMN digital_employee_environment_variables.team_id IS '环境变量所属数字员工团队 ID；无团队数字员工为空';
