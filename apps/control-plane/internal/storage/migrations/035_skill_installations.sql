CREATE TABLE skill_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    target_scope VARCHAR(40) NOT NULL,
    team_id UUID,
    digital_employee_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    provider_type VARCHAR(80) NOT NULL,
    installed_path TEXT NOT NULL,
    archive_checksum_sha256 VARCHAR(64) NOT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'installed',
    installed_by UUID,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT skill_installations_scope_supported CHECK (target_scope IN ('team', 'employee')),
    CONSTRAINT skill_installations_provider_supported CHECK (provider_type IN ('opencode', 'codex', 'claude-code')),
    CONSTRAINT skill_installations_status_supported CHECK (status = 'installed'),
    CONSTRAINT skill_installations_installed_path_not_blank CHECK (btrim(installed_path) <> ''),
    CONSTRAINT skill_installations_checksum_not_blank CHECK (btrim(archive_checksum_sha256) <> '')
);

CREATE UNIQUE INDEX uq_skill_installations_active_employee
    ON skill_installations (tenant_id, skill_id, digital_employee_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_skill_installations_skill
    ON skill_installations (tenant_id, skill_id, installed_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_skill_installations_employee
    ON skill_installations (tenant_id, digital_employee_id, installed_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_skill_installations_team
    ON skill_installations (tenant_id, team_id, installed_at DESC)
    WHERE deleted_at IS NULL;

COMMENT ON TABLE skill_installations IS '技能物理安装记录，只保存已成功写入数字员工 workspace 的安装事实';
COMMENT ON COLUMN skill_installations.target_scope IS '安装请求目标范围，team 表示团队批量安装，employee 表示单个数字员工安装';
COMMENT ON COLUMN skill_installations.installed_path IS 'Runtime Agent 实际写入的 provider 官方技能目录';
COMMENT ON COLUMN skill_installations.metadata IS '安装命令、Runtime 回执和排障扩展信息';
