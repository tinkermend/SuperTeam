CREATE TABLE runtime_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    runtime_node_id UUID,
    node_id VARCHAR(255),
    event_type VARCHAR(100) NOT NULL,
    severity VARCHAR(32) NOT NULL,
    source VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    provider_type VARCHAR(100),
    correlation_type VARCHAR(100),
    correlation_id VARCHAR(255),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_runtime_events_type CHECK (event_type IN (
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
        'command_timed_out'
    )),
    CONSTRAINT chk_runtime_events_severity CHECK (severity IN ('info', 'success', 'warning', 'error')),
    CONSTRAINT chk_runtime_events_source CHECK (source IN (
        'runtime_enrollment',
        'runtime_node',
        'runtime_capability',
        'runtime_command',
        'provider_session'
    ))
);

CREATE INDEX idx_runtime_events_tenant_created
    ON runtime_events(tenant_id, created_at DESC);

CREATE INDEX idx_runtime_events_node_created
    ON runtime_events(tenant_id, runtime_node_id, created_at DESC);

CREATE INDEX idx_runtime_events_node_id_created
    ON runtime_events(tenant_id, node_id, created_at DESC);

CREATE INDEX idx_runtime_events_type_created
    ON runtime_events(tenant_id, event_type, created_at DESC);

CREATE INDEX idx_runtime_events_severity_created
    ON runtime_events(tenant_id, severity, created_at DESC);

CREATE INDEX idx_runtime_events_provider_created
    ON runtime_events(tenant_id, provider_type, created_at DESC);

CREATE INDEX idx_runtime_events_correlation
    ON runtime_events(tenant_id, correlation_type, correlation_id);

CREATE INDEX idx_runtime_nodes_tenant_online_heartbeat
    ON runtime_nodes(tenant_id, status, last_heartbeat_at DESC)
    WHERE disabled_at IS NULL AND archived_at IS NULL;

CREATE TRIGGER update_runtime_events_updated_at BEFORE UPDATE ON runtime_events
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE runtime_events IS 'Runtime 管理面统一事件流，用于 Runtime 总览和事件审计';
COMMENT ON COLUMN runtime_events.id IS 'Runtime 管理面事件主键 UUID';
COMMENT ON COLUMN runtime_events.tenant_id IS '所属租户 ID，用于 Runtime 事件租户隔离';
COMMENT ON COLUMN runtime_events.runtime_node_id IS '关联 Runtime 节点 UUID，可为空以支持仅有 node_id 的接入请求';
COMMENT ON COLUMN runtime_events.node_id IS 'Runtime 外部业务节点 ID';
COMMENT ON COLUMN runtime_events.event_type IS 'Runtime 管理面事件类型';
COMMENT ON COLUMN runtime_events.severity IS '事件严重级别：info、success、warning 或 error';
COMMENT ON COLUMN runtime_events.source IS '事件来源模块';
COMMENT ON COLUMN runtime_events.title IS '事件列表展示标题';
COMMENT ON COLUMN runtime_events.description IS '事件摘要描述';
COMMENT ON COLUMN runtime_events.provider_type IS '关联 Provider 类型';
COMMENT ON COLUMN runtime_events.correlation_type IS '原始事实类型';
COMMENT ON COLUMN runtime_events.correlation_id IS '原始事实 ID 或命令 ID';
COMMENT ON COLUMN runtime_events.payload IS '脱敏后的事件扩展数据';
COMMENT ON COLUMN runtime_events.created_at IS 'Runtime 管理面事件创建时间';
COMMENT ON COLUMN runtime_events.updated_at IS 'Runtime 管理面事件最后更新时间';
