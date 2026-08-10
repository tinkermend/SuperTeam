-- Provider 原生配置快照（键级白名单管理面）。
-- 见 docs/design/runtimeAgent/provider-native-config-management.md

CREATE TABLE IF NOT EXISTS runtime_provider_native_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    node_id VARCHAR(255) NOT NULL,
    provider_type VARCHAR(100) NOT NULL,
    config_key VARCHAR(100) NOT NULL,
    resolved_path TEXT,
    format VARCHAR(32) NOT NULL DEFAULT 'json',
    managed_values JSONB NOT NULL DEFAULT '{}'::jsonb,
    file_content_hash VARCHAR(128),
    exists_on_node BOOLEAN NOT NULL DEFAULT false,
    manageable BOOLEAN NOT NULL DEFAULT true,
    unmanageable_reason VARCHAR(100),
    source VARCHAR(32) NOT NULL DEFAULT 'pulled',
    node_mtime TIMESTAMPTZ,
    snapshot_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_pulled_at TIMESTAMPTZ,
    last_pushed_at TIMESTAMPTZ,
    last_pushed_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_runtime_provider_native_configs UNIQUE (tenant_id, runtime_node_id, provider_type, config_key),
    CONSTRAINT chk_runtime_provider_native_configs_source CHECK (source IN ('pulled', 'pushed')),
    CONSTRAINT chk_runtime_provider_native_configs_format CHECK (format IN ('json', 'toml'))
);

CREATE INDEX IF NOT EXISTS idx_runtime_provider_native_configs_node
    ON runtime_provider_native_configs(tenant_id, node_id, provider_type, config_key);

CREATE INDEX IF NOT EXISTS idx_runtime_provider_native_configs_runtime_node
    ON runtime_provider_native_configs(tenant_id, runtime_node_id, snapshot_at DESC);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_trigger WHERE tgname = 'update_runtime_provider_native_configs_updated_at'
    ) THEN
        CREATE TRIGGER update_runtime_provider_native_configs_updated_at
        BEFORE UPDATE ON runtime_provider_native_configs
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

COMMENT ON TABLE runtime_provider_native_configs IS 'Runtime 节点上 Provider 原生配置的受管键快照（不含全文）；敏感键值 AES-GCM 加密后写入 managed_values';
COMMENT ON COLUMN runtime_provider_native_configs.id IS '快照主键 UUID';
COMMENT ON COLUMN runtime_provider_native_configs.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN runtime_provider_native_configs.runtime_node_id IS '内部 runtime_nodes.id';
COMMENT ON COLUMN runtime_provider_native_configs.node_id IS '对外 node 字符串，冗余便于查询';
COMMENT ON COLUMN runtime_provider_native_configs.provider_type IS 'Provider 类型：claude-code / opencode / codex';
COMMENT ON COLUMN runtime_provider_native_configs.config_key IS '逻辑配置面：model_profile / auth';
COMMENT ON COLUMN runtime_provider_native_configs.resolved_path IS '节点侧解析出的绝对路径（展示与审计；由节点回报）';
COMMENT ON COLUMN runtime_provider_native_configs.format IS '文件格式：json 或 toml';
COMMENT ON COLUMN runtime_provider_native_configs.managed_values IS '白名单内键值；敏感键值为 aesgcm:v1: 密文';
COMMENT ON COLUMN runtime_provider_native_configs.file_content_hash IS '整文件 sha256 指纹，用于乐观锁与漂移提示';
COMMENT ON COLUMN runtime_provider_native_configs.exists_on_node IS '上次探测/读写时文件是否存在';
COMMENT ON COLUMN runtime_provider_native_configs.manageable IS '该平台该面是否可经文件管理';
COMMENT ON COLUMN runtime_provider_native_configs.unmanageable_reason IS 'manageable=false 时的原因码';
COMMENT ON COLUMN runtime_provider_native_configs.source IS '快照来源：pulled 或 pushed';
COMMENT ON COLUMN runtime_provider_native_configs.node_mtime IS '节点侧文件 mtime（可选）';
COMMENT ON COLUMN runtime_provider_native_configs.snapshot_at IS '快照生成时间';
COMMENT ON COLUMN runtime_provider_native_configs.last_pulled_at IS '最近一次成功拉取时间';
COMMENT ON COLUMN runtime_provider_native_configs.last_pushed_at IS '最近一次成功下发时间';
COMMENT ON COLUMN runtime_provider_native_configs.last_pushed_by IS '最近一次下发操作者用户 ID';
COMMENT ON COLUMN runtime_provider_native_configs.created_at IS '行创建时间';
COMMENT ON COLUMN runtime_provider_native_configs.updated_at IS '行更新时间';

-- 放宽 runtime_events 事件类型与 source，支持原生配置 pull/push 审计。
ALTER TABLE runtime_events DROP CONSTRAINT IF EXISTS chk_runtime_events_type;
ALTER TABLE runtime_events ADD CONSTRAINT chk_runtime_events_type CHECK (event_type IN (
    'enrollment_requested',
    'enrollment_approved',
    'enrollment_rejected',
    'enrollment_revoked',
    'node_online',
    'node_offline',
    'capability_reported',
    'capability_degraded',
    'command_event',
    'command_completed',
    'command_failed',
    'command_cancelled',
    'command_timed_out',
    'provider_native_config_pull',
    'provider_native_config_push'
));

ALTER TABLE runtime_events DROP CONSTRAINT IF EXISTS chk_runtime_events_source;
ALTER TABLE runtime_events ADD CONSTRAINT chk_runtime_events_source CHECK (source IN (
    'runtime_enrollment',
    'runtime_node',
    'runtime_capability',
    'runtime_command',
    'provider_session',
    'provider_native_config'
));
