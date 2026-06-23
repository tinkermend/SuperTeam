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
COMMENT ON COLUMN skill_installations.id IS '技能物理安装记录 ID';
COMMENT ON COLUMN skill_installations.tenant_id IS '安装记录所属租户 ID';
COMMENT ON COLUMN skill_installations.skill_id IS '被安装的技能包 ID';
COMMENT ON COLUMN skill_installations.target_scope IS '安装请求目标范围，team 表示团队批量安装，employee 表示单个数字员工安装';
COMMENT ON COLUMN skill_installations.team_id IS '团队批量安装来源团队 ID，单员工安装时可为空';
COMMENT ON COLUMN skill_installations.digital_employee_id IS '实际写入技能目录的数字员工 ID';
COMMENT ON COLUMN skill_installations.runtime_node_id IS '执行本次物理安装的 Runtime 节点 ID';
COMMENT ON COLUMN skill_installations.provider_type IS 'Provider 类型，由服务端注册表和安装前置校验控制';
COMMENT ON COLUMN skill_installations.installed_path IS 'Runtime Agent 实际写入的 provider 官方技能目录';
COMMENT ON COLUMN skill_installations.archive_checksum_sha256 IS '安装时使用的技能 zip 包 SHA256 校验值';
COMMENT ON COLUMN skill_installations.status IS '安装事实状态；此表只保存 installed 成功记录';
COMMENT ON COLUMN skill_installations.installed_by IS '触发安装的人类用户 ID 或系统操作者 ID';
COMMENT ON COLUMN skill_installations.installed_at IS 'Runtime 确认物理安装成功的时间';
COMMENT ON COLUMN skill_installations.metadata IS '安装命令、Runtime 回执和排障扩展信息';
COMMENT ON COLUMN skill_installations.created_at IS '安装记录创建时间';
COMMENT ON COLUMN skill_installations.updated_at IS '安装记录最后更新时间';
COMMENT ON COLUMN skill_installations.deleted_at IS '安装记录软删除时间；为空表示当前有效';
