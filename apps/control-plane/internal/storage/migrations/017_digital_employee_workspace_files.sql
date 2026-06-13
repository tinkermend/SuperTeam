CREATE TABLE IF NOT EXISTS digital_employee_workspace_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    path TEXT NOT NULL,
    file_role VARCHAR(50) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    sync_policy VARCHAR(50) NOT NULL DEFAULT 'auto',
    current_revision_id UUID,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_files_active_path
    ON digital_employee_workspace_files(tenant_id, digital_employee_id, path)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_files_active_entrypoint
    ON digital_employee_workspace_files(tenant_id, digital_employee_id)
    WHERE deleted_at IS NULL AND status = 'active' AND file_role = 'entrypoint';

CREATE INDEX IF NOT EXISTS idx_de_workspace_files_employee_status
    ON digital_employee_workspace_files(tenant_id, digital_employee_id, status, updated_at DESC)
    WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS digital_employee_workspace_file_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    file_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    content_text TEXT,
    content_hash VARCHAR(64) NOT NULL,
    size_bytes INTEGER NOT NULL,
    storage_backend VARCHAR(50) NOT NULL DEFAULT 'db',
    object_key TEXT,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    change_note TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_de_workspace_file_revisions_db_content
        CHECK (
            (storage_backend = 'db' AND content_text IS NOT NULL AND object_key IS NULL)
            OR
            (storage_backend = 'object_store' AND object_key IS NOT NULL)
        ),
    CONSTRAINT chk_de_workspace_file_revisions_sha256
        CHECK (content_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_de_workspace_file_revisions_size
        CHECK (size_bytes >= 0)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_revisions_number
    ON digital_employee_workspace_file_revisions(file_id, revision_number);

CREATE INDEX IF NOT EXISTS idx_de_workspace_file_revisions_tenant_file
    ON digital_employee_workspace_file_revisions(tenant_id, file_id, revision_number DESC);

CREATE TABLE IF NOT EXISTS digital_employee_workspace_file_syncs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    execution_instance_id UUID NOT NULL,
    file_id UUID NOT NULL,
    revision_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL,
    synced_hash VARCHAR(64),
    error_message TEXT,
    last_command_id VARCHAR(255),
    last_synced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_de_workspace_file_syncs_target
    ON digital_employee_workspace_file_syncs(tenant_id, digital_employee_id, execution_instance_id, file_id);

CREATE INDEX IF NOT EXISTS idx_de_workspace_file_syncs_status
    ON digital_employee_workspace_file_syncs(tenant_id, digital_employee_id, status, updated_at DESC);

COMMENT ON TABLE digital_employee_workspace_files IS '数字员工工作目录受控文件身份表';
COMMENT ON COLUMN digital_employee_workspace_files.id IS '受控文件主键 UUID';
COMMENT ON COLUMN digital_employee_workspace_files.tenant_id IS '文件所属租户 ID';
COMMENT ON COLUMN digital_employee_workspace_files.team_id IS '文件所属数字员工团队 ID，目录按团队归属组织';
COMMENT ON COLUMN digital_employee_workspace_files.digital_employee_id IS '文件所属数字员工 ID';
COMMENT ON COLUMN digital_employee_workspace_files.path IS '数字员工根目录下的安全相对路径';
COMMENT ON COLUMN digital_employee_workspace_files.file_role IS '文件角色，例如 entrypoint、supporting_doc、provider_config 或 generated，由应用层注册表校验';
COMMENT ON COLUMN digital_employee_workspace_files.mime_type IS '文件 MIME 类型';
COMMENT ON COLUMN digital_employee_workspace_files.sync_policy IS '同步策略，例如 auto、manual 或 disabled，由应用层注册表校验';
COMMENT ON COLUMN digital_employee_workspace_files.current_revision_id IS '当前激活的文件版本 ID';
COMMENT ON COLUMN digital_employee_workspace_files.status IS '文件状态，例如 active、archived 或 deleted';
COMMENT ON COLUMN digital_employee_workspace_files.metadata IS '文件扩展元数据 JSON';
COMMENT ON COLUMN digital_employee_workspace_files.created_by IS '创建文件的用户 ID，系统创建时可为空';
COMMENT ON COLUMN digital_employee_workspace_files.created_at IS '文件创建时间';
COMMENT ON COLUMN digital_employee_workspace_files.updated_at IS '文件更新时间';
COMMENT ON COLUMN digital_employee_workspace_files.archived_at IS '文件归档时间';
COMMENT ON COLUMN digital_employee_workspace_files.deleted_at IS '文件软删除时间';

COMMENT ON TABLE digital_employee_workspace_file_revisions IS '数字员工工作目录受控文件内容版本表';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.id IS '文件版本主键 UUID';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.tenant_id IS '文件版本所属租户 ID';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.file_id IS '所属受控文件 ID';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.revision_number IS '文件内递增版本号';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.content_text IS 'DB 存储模式下的文本正文';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.content_hash IS '文件正文 SHA-256 十六进制校验值';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.size_bytes IS '文件正文 UTF-8 字节数';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.storage_backend IS '正文存储后端，首版使用 db，预留 object_store';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.object_key IS '对象存储模式下的对象键';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.created_by IS '创建版本的用户 ID，系统创建时可为空';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.created_at IS '文件版本创建时间';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.change_note IS '版本变更说明';
COMMENT ON COLUMN digital_employee_workspace_file_revisions.metadata IS '文件版本扩展元数据 JSON';

COMMENT ON TABLE digital_employee_workspace_file_syncs IS '数字员工工作目录文件同步状态投影表';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.id IS '同步状态主键 UUID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.tenant_id IS '同步状态所属租户 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.digital_employee_id IS '同步状态所属数字员工 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.execution_instance_id IS '同步目标执行实例 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.file_id IS '同步的受控文件 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.revision_id IS '同步目标文件版本 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.runtime_node_id IS '同步目标 Runtime 节点 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.status IS '同步状态，例如 pending、synced 或 failed';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.synced_hash IS 'Runtime 回写的已同步文件校验值';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.error_message IS '同步失败时的错误摘要';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.last_command_id IS '最后一次同步命令 ID';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.last_synced_at IS '最后同步成功时间';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.created_at IS '同步状态创建时间';
COMMENT ON COLUMN digital_employee_workspace_file_syncs.updated_at IS '同步状态更新时间';
