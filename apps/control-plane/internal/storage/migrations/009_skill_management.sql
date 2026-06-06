CREATE TABLE IF NOT EXISTS skills (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    slug VARCHAR(160) NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    version VARCHAR(80) NOT NULL DEFAULT 'v0.1.0',
    source VARCHAR(80) NOT NULL DEFAULT 'upload',
    risk_level VARCHAR(40) NOT NULL DEFAULT 'medium',
    status VARCHAR(40) NOT NULL DEFAULT 'installed',
    icon_key VARCHAR(80) NOT NULL DEFAULT 'blocks',
    color_token VARCHAR(40) NOT NULL DEFAULT 'teal',
    tags TEXT[] NOT NULL DEFAULT '{}'::text[],
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_skills_tenant_slug_active
    ON skills(tenant_id, slug)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_skills_tenant_status_updated
    ON skills(tenant_id, status, updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS skill_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    path TEXT NOT NULL,
    file_type VARCHAR(40) NOT NULL DEFAULT 'file',
    content TEXT NOT NULL DEFAULT '',
    size_bytes BIGINT NOT NULL DEFAULT 0,
    checksum_sha256 VARCHAR(64) NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_files_tenant_skill_path
    ON skill_files(tenant_id, skill_id, path);

CREATE INDEX IF NOT EXISTS idx_skill_files_tenant_skill_path
    ON skill_files(tenant_id, skill_id, path);

CREATE TABLE IF NOT EXISTS skill_team_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    team_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_team_bindings_tenant_skill_team
    ON skill_team_bindings(tenant_id, skill_id, team_id);

CREATE INDEX IF NOT EXISTS idx_skill_team_bindings_tenant_team
    ON skill_team_bindings(tenant_id, team_id);

CREATE TABLE IF NOT EXISTS skill_agent_bindings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'enabled',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_skill_agent_bindings_tenant_skill_employee
    ON skill_agent_bindings(tenant_id, skill_id, digital_employee_id);

CREATE INDEX IF NOT EXISTS idx_skill_agent_bindings_tenant_employee
    ON skill_agent_bindings(tenant_id, digital_employee_id);

COMMENT ON TABLE skills IS '技能包主表，记录可上传、安装和绑定到数字员工的技能定义';
COMMENT ON COLUMN skills.id IS '技能主键 UUID';
COMMENT ON COLUMN skills.tenant_id IS '技能所属租户 ID';
COMMENT ON COLUMN skills.slug IS '租户内技能稳定标识，由上传名称规范化生成';
COMMENT ON COLUMN skills.name IS '技能显示名称';
COMMENT ON COLUMN skills.description IS '技能描述，由上传表单或 SKILL.md 摘要提供';
COMMENT ON COLUMN skills.version IS '技能版本，来自上传包元数据或默认版本';
COMMENT ON COLUMN skills.source IS '技能来源，例如 upload、internal_market 或 marketplace';
COMMENT ON COLUMN skills.risk_level IS '技能风险等级，由服务端和上传表单校验';
COMMENT ON COLUMN skills.status IS '技能状态，例如 installed 或 available';
COMMENT ON COLUMN skills.icon_key IS '技能市场彩色图标键，前端映射到图标库';
COMMENT ON COLUMN skills.color_token IS '技能图标语义色标识';
COMMENT ON COLUMN skills.tags IS '上传定义的技能标签数组，技能市场展示只使用此字段';
COMMENT ON COLUMN skills.metadata IS '技能扩展元数据 JSON';
COMMENT ON COLUMN skills.created_by IS '上传或创建技能的用户 ID';
COMMENT ON COLUMN skills.deleted_at IS '技能软删除时间';
COMMENT ON COLUMN skills.created_at IS '技能创建时间';
COMMENT ON COLUMN skills.updated_at IS '技能更新时间';

COMMENT ON TABLE skill_files IS '技能包文件表，保存 SKILL.md、脚本和附加资源的可编辑文本内容';
COMMENT ON COLUMN skill_files.id IS '技能文件主键 UUID';
COMMENT ON COLUMN skill_files.tenant_id IS '技能文件所属租户 ID';
COMMENT ON COLUMN skill_files.skill_id IS '所属技能 ID';
COMMENT ON COLUMN skill_files.path IS '技能包内规范化相对路径';
COMMENT ON COLUMN skill_files.file_type IS '文件节点类型，当前为 file';
COMMENT ON COLUMN skill_files.content IS '文本文件内容，支持在线编辑';
COMMENT ON COLUMN skill_files.size_bytes IS '文件内容字节数';
COMMENT ON COLUMN skill_files.checksum_sha256 IS '文件内容 SHA256 校验值';
COMMENT ON COLUMN skill_files.metadata IS '技能文件扩展元数据 JSON';
COMMENT ON COLUMN skill_files.created_at IS '技能文件创建时间';
COMMENT ON COLUMN skill_files.updated_at IS '技能文件更新时间';

COMMENT ON TABLE skill_team_bindings IS '技能与团队归属绑定表';
COMMENT ON COLUMN skill_team_bindings.id IS '技能团队绑定主键 UUID';
COMMENT ON COLUMN skill_team_bindings.tenant_id IS '绑定所属租户 ID';
COMMENT ON COLUMN skill_team_bindings.skill_id IS '绑定的技能 ID';
COMMENT ON COLUMN skill_team_bindings.team_id IS '归属团队 ID';
COMMENT ON COLUMN skill_team_bindings.created_at IS '绑定创建时间';

COMMENT ON TABLE skill_agent_bindings IS '技能安装到数字员工的绑定表';
COMMENT ON COLUMN skill_agent_bindings.id IS '技能数字员工绑定主键 UUID';
COMMENT ON COLUMN skill_agent_bindings.tenant_id IS '绑定所属租户 ID';
COMMENT ON COLUMN skill_agent_bindings.skill_id IS '绑定的技能 ID';
COMMENT ON COLUMN skill_agent_bindings.digital_employee_id IS '安装该技能的数字员工 ID';
COMMENT ON COLUMN skill_agent_bindings.status IS '安装状态，例如 enabled 或 disabled';
COMMENT ON COLUMN skill_agent_bindings.metadata IS '安装关系扩展元数据 JSON';
COMMENT ON COLUMN skill_agent_bindings.created_at IS '安装关系创建时间';
COMMENT ON COLUMN skill_agent_bindings.updated_at IS '安装关系更新时间';

INSERT INTO skills (
    id,
    tenant_id,
    slug,
    name,
    description,
    version,
    source,
    risk_level,
    status,
    icon_key,
    color_token,
    tags
)
VALUES
(
    '00000000-0000-0000-0000-000000000301'::uuid,
    '00000000-0000-0000-0000-000000000001'::uuid,
    'diagnose',
    'diagnose',
    '用于任务失败或异常时，系统化地进行问题复现、证据收集、根因分析和回归验证。',
    'v1.0.0',
    'internal_market',
    'low',
    'installed',
    'stethoscope',
    'cyan',
    ARRAY['诊断','测试','自动化']::text[]
),
(
    '00000000-0000-0000-0000-000000000302'::uuid,
    '00000000-0000-0000-0000-000000000001'::uuid,
    'tdd',
    'tdd',
    '测试优先的红绿重构流程，适合执行型数字员工开发功能或修复缺陷。',
    'v1.0.0',
    'internal_market',
    'medium',
    'installed',
    'flask',
    'emerald',
    ARRAY['测试','代码审查']::text[]
)
ON CONFLICT (id) DO NOTHING;

INSERT INTO skill_files (tenant_id, skill_id, path, file_type, content, size_bytes, checksum_sha256)
VALUES
(
    '00000000-0000-0000-0000-000000000001'::uuid,
    '00000000-0000-0000-0000-000000000301'::uuid,
    'SKILL.md',
    'file',
    '# diagnose

触发：当任务失败、日志异常或运行结果与预期不一致时使用。

## 工作流

1. 复现问题
2. 收集证据
3. 最小化场景
4. 修复并回归验证
',
    188,
    ''
),
(
    '00000000-0000-0000-0000-000000000001'::uuid,
    '00000000-0000-0000-0000-000000000301'::uuid,
    'scripts/reproduce.sh',
    'file',
    '#!/usr/bin/env bash
set -euo pipefail
',
    37,
    ''
),
(
    '00000000-0000-0000-0000-000000000001'::uuid,
    '00000000-0000-0000-0000-000000000302'::uuid,
    'SKILL.md',
    'file',
    '# tdd

触发：实现功能或修复缺陷前使用。

## 工作流

1. 写失败测试
2. 实现最小代码
3. 重构并保持测试通过
',
    137,
    ''
)
ON CONFLICT (tenant_id, skill_id, path) DO NOTHING;

INSERT INTO skill_team_bindings (tenant_id, skill_id, team_id)
VALUES
(
    '00000000-0000-0000-0000-000000000001'::uuid,
    '00000000-0000-0000-0000-000000000301'::uuid,
    '00000000-0000-0000-0000-000000000101'::uuid
),
(
    '00000000-0000-0000-0000-000000000001'::uuid,
    '00000000-0000-0000-0000-000000000302'::uuid,
    '00000000-0000-0000-0000-000000000101'::uuid
)
ON CONFLICT DO NOTHING;
