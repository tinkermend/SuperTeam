-- SuperTeam Control Plane Initial Schema
-- Migration: 001_initial
-- Created: 2026-05-29
-- Rebuilt: 2026-06-01 UUID-first rebuild-only schema
--
-- Development defaults:
-- default tenant: 00000000-0000-0000-0000-000000000001
-- default team: 00000000-0000-0000-0000-000000000101

-- ============================================================================
-- Tenant Module
-- ============================================================================

CREATE TABLE tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(100) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    archived_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tenant_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    profile_key VARCHAR(100) NOT NULL,
    profile_value JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, profile_key)
);

CREATE TABLE tenant_teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug VARCHAR(100) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    archived_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

-- ============================================================================
-- Auth Module
-- ============================================================================

CREATE TABLE auth_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username VARCHAR(255) NOT NULL,
    display_name VARCHAR(255),
    email VARCHAR(255),
    password_hash VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    last_login_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tenant_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    team_id UUID REFERENCES tenant_teams(id) ON DELETE CASCADE,
    principal_type VARCHAR(50) NOT NULL,
    principal_id UUID NOT NULL,
    role VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, team_id, principal_type, principal_id, role)
);

-- ============================================================================
-- Runtime Module
-- ============================================================================

CREATE TABLE runtime_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    node_id VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    supported_providers JSONB NOT NULL,
    max_slots INTEGER NOT NULL DEFAULT 1,
    current_load INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL,
    metadata JSONB,
    last_heartbeat_at TIMESTAMPTZ,
    disabled_at TIMESTAMPTZ,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE runtime_node_scopes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    runtime_node_id UUID NOT NULL REFERENCES runtime_nodes(id) ON DELETE CASCADE,
    team_id UUID REFERENCES tenant_teams(id),
    scope_type VARCHAR(100) NOT NULL,
    scope_value VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    disabled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (runtime_node_id, scope_type, scope_value)
);

CREATE TABLE auth_runtime_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid REFERENCES tenants(id),
    node_id VARCHAR(255) NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    client_ip VARCHAR(45),
    user_agent TEXT,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- Task Module
-- ============================================================================

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    team_id UUID,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    creator_id UUID,
    provider_type VARCHAR(100) NOT NULL,
    target_node_id VARCHAR(255),
    assigned_node_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    workspace_path TEXT,
    params JSONB NOT NULL DEFAULT '{}'::jsonb,
    priority INTEGER NOT NULL DEFAULT 0,
    idempotency_key VARCHAR(255),
    risk_level VARCHAR(50) NOT NULL DEFAULT 'normal',
    cancelled_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    task_id UUID NOT NULL,
    node_id VARCHAR(255) NOT NULL,
    runtime_node_id UUID,
    provider_session_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    lease_expires_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE runtime_leases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    task_id UUID NOT NULL,
    run_id UUID,
    runtime_node_id UUID,
    node_id VARCHAR(255) NOT NULL,
    lease_token VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    expires_at TIMESTAMPTZ NOT NULL,
    renewed_at TIMESTAMPTZ,
    released_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (lease_token)
);

CREATE TABLE task_state_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    task_id UUID NOT NULL,
    from_status VARCHAR(50),
    to_status VARCHAR(50) NOT NULL,
    changed_by VARCHAR(255),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    task_id UUID NOT NULL,
    run_id UUID,
    event_type VARCHAR(100) NOT NULL,
    sequence_number INTEGER NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE task_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    task_id UUID NOT NULL,
    run_id UUID,
    artifact_type VARCHAR(100) NOT NULL,
    name VARCHAR(500) NOT NULL,
    storage_url TEXT NOT NULL,
    size_bytes BIGINT,
    metadata JSONB,
    archived_at TIMESTAMPTZ,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- Audit Module
-- ============================================================================

CREATE TABLE audit_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    event_type VARCHAR(100) NOT NULL,
    actor_type VARCHAR(50) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    resource_type VARCHAR(50),
    resource_id VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    details JSONB,
    ip_address INET,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE web_login_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('login_succeeded', 'login_failed', 'logout_succeeded')),
    user_id UUID,
    username VARCHAR(100) NOT NULL,
    session_id UUID,
    client_ip VARCHAR(45),
    user_agent TEXT,
    result VARCHAR(50) NOT NULL CHECK (result IN ('succeeded', 'failed')),
    failure_reason VARCHAR(255),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE web_operation_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000001'::uuid,
    user_id UUID,
    username VARCHAR(100),
    module VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    result VARCHAR(50) NOT NULL CHECK (result IN ('succeeded', 'failed')),
    request_id VARCHAR(255),
    client_ip VARCHAR(45),
    user_agent TEXT,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- Indexes
-- ============================================================================

CREATE INDEX idx_tenant_profiles_tenant_id ON tenant_profiles(tenant_id);
CREATE INDEX idx_tenant_teams_tenant_id ON tenant_teams(tenant_id);
CREATE INDEX idx_tenant_members_tenant_principal ON tenant_members(tenant_id, principal_type, principal_id);
CREATE UNIQUE INDEX uq_auth_users_active_username ON auth_users(username) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_auth_users_active_email ON auth_users(email) WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_auth_users_username ON auth_users(username);
CREATE INDEX idx_auth_users_email ON auth_users(email);
CREATE INDEX idx_auth_users_status ON auth_users(status);
CREATE INDEX idx_runtime_nodes_tenant_id ON runtime_nodes(tenant_id);
CREATE INDEX idx_runtime_nodes_status ON runtime_nodes(status);
CREATE INDEX idx_runtime_nodes_last_heartbeat ON runtime_nodes(last_heartbeat_at);
CREATE INDEX idx_runtime_nodes_supported_providers ON runtime_nodes USING GIN (supported_providers);
CREATE INDEX idx_runtime_nodes_status_heartbeat ON runtime_nodes(status, last_heartbeat_at DESC);
CREATE INDEX idx_runtime_node_scopes_tenant_id ON runtime_node_scopes(tenant_id);
CREATE UNIQUE INDEX uq_auth_runtime_tokens_active_node_id ON auth_runtime_tokens(node_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_auth_runtime_tokens_node_id ON auth_runtime_tokens(node_id);
CREATE INDEX idx_auth_sessions_user_id ON auth_sessions(user_id);
CREATE INDEX idx_auth_sessions_token_hash ON auth_sessions(token_hash);
CREATE INDEX idx_auth_sessions_expires_at ON auth_sessions(expires_at);
CREATE INDEX idx_tasks_tenant_id ON tasks(tenant_id);
CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_provider_type ON tasks(provider_type);
CREATE INDEX idx_tasks_assigned_node_id ON tasks(assigned_node_id);
CREATE INDEX idx_tasks_creator_id ON tasks(creator_id);
CREATE INDEX idx_tasks_params ON tasks USING GIN (params);
CREATE INDEX idx_tasks_status_priority_created ON tasks(status, priority DESC, created_at DESC);
CREATE INDEX idx_task_runs_task_id ON task_runs(task_id);
CREATE INDEX idx_task_runs_runtime_node_id ON task_runs(runtime_node_id);
CREATE INDEX idx_task_runs_status ON task_runs(status);
CREATE INDEX idx_runtime_leases_task_id ON runtime_leases(task_id);
CREATE INDEX idx_runtime_leases_runtime_node_id ON runtime_leases(runtime_node_id);
CREATE INDEX idx_runtime_leases_expires_at ON runtime_leases(expires_at);
CREATE INDEX idx_task_state_history_task_id ON task_state_history(task_id);
CREATE INDEX idx_task_events_task_id ON task_events(task_id);
CREATE INDEX idx_task_events_run_id ON task_events(run_id);
CREATE UNIQUE INDEX uq_task_events_task_sequence ON task_events(task_id, sequence_number);
CREATE UNIQUE INDEX uq_task_events_run_sequence ON task_events(run_id, sequence_number) WHERE run_id IS NOT NULL;
CREATE INDEX idx_task_artifacts_task_id ON task_artifacts(task_id);
CREATE INDEX idx_task_artifacts_run_id ON task_artifacts(run_id);
CREATE INDEX idx_task_artifacts_type ON task_artifacts(artifact_type);
CREATE INDEX idx_audit_events_actor ON audit_events(actor_type, actor_id);
CREATE INDEX idx_audit_events_resource ON audit_events(resource_type, resource_id);
CREATE INDEX idx_audit_events_created_at ON audit_events(created_at);
CREATE INDEX idx_web_login_logs_event_type_created ON web_login_logs(event_type, created_at DESC);
CREATE INDEX idx_web_login_logs_user_id_created ON web_login_logs(user_id, created_at DESC);
CREATE INDEX idx_web_login_logs_username_created ON web_login_logs(username, created_at DESC);
CREATE INDEX idx_web_login_logs_created_at ON web_login_logs(created_at DESC);
CREATE INDEX idx_web_operation_logs_user_id_created ON web_operation_logs(user_id, created_at DESC);
CREATE INDEX idx_web_operation_logs_module_action_created ON web_operation_logs(module, action, created_at DESC);
CREATE INDEX idx_web_operation_logs_resource ON web_operation_logs(resource_type, resource_id);
CREATE INDEX idx_web_operation_logs_created_at ON web_operation_logs(created_at DESC);

-- ============================================================================
-- Triggers for auto-updating updated_at
-- ============================================================================

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_tenants_updated_at BEFORE UPDATE ON tenants
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_tenant_profiles_updated_at BEFORE UPDATE ON tenant_profiles
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_tenant_teams_updated_at BEFORE UPDATE ON tenant_teams
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_auth_users_updated_at BEFORE UPDATE ON auth_users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_tenant_members_updated_at BEFORE UPDATE ON tenant_members
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_runtime_nodes_updated_at BEFORE UPDATE ON runtime_nodes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_runtime_node_scopes_updated_at BEFORE UPDATE ON runtime_node_scopes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_auth_sessions_updated_at BEFORE UPDATE ON auth_sessions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_tasks_updated_at BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_task_runs_updated_at BEFORE UPDATE ON task_runs
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
CREATE TRIGGER update_runtime_leases_updated_at BEFORE UPDATE ON runtime_leases
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Comments
-- ============================================================================

COMMENT ON TABLE tenants IS '租户表';
COMMENT ON COLUMN tenants.id IS '租户主键 UUID';
COMMENT ON COLUMN tenants.slug IS '租户唯一标识';
COMMENT ON COLUMN tenants.name IS '租户名称';
COMMENT ON COLUMN tenants.status IS '租户状态';
COMMENT ON COLUMN tenants.metadata IS '租户扩展元数据';
COMMENT ON COLUMN tenants.archived_at IS '租户归档时间';
COMMENT ON COLUMN tenants.disabled_at IS '租户禁用时间';
COMMENT ON COLUMN tenants.deleted_at IS '租户软删除时间';
COMMENT ON COLUMN tenants.created_at IS '租户创建时间';
COMMENT ON COLUMN tenants.updated_at IS '租户最后更新时间';

COMMENT ON TABLE tenant_profiles IS '租户配置画像表';
COMMENT ON COLUMN tenant_profiles.id IS '租户配置主键 UUID';
COMMENT ON COLUMN tenant_profiles.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN tenant_profiles.profile_key IS '配置键';
COMMENT ON COLUMN tenant_profiles.profile_value IS '配置值';
COMMENT ON COLUMN tenant_profiles.created_at IS '配置创建时间';
COMMENT ON COLUMN tenant_profiles.updated_at IS '配置最后更新时间';

COMMENT ON TABLE tenant_teams IS '租户团队表';
COMMENT ON COLUMN tenant_teams.id IS '团队主键 UUID';
COMMENT ON COLUMN tenant_teams.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN tenant_teams.slug IS '团队唯一标识';
COMMENT ON COLUMN tenant_teams.name IS '团队名称';
COMMENT ON COLUMN tenant_teams.status IS '团队状态';
COMMENT ON COLUMN tenant_teams.metadata IS '团队扩展元数据';
COMMENT ON COLUMN tenant_teams.archived_at IS '团队归档时间';
COMMENT ON COLUMN tenant_teams.disabled_at IS '团队禁用时间';
COMMENT ON COLUMN tenant_teams.deleted_at IS '团队软删除时间';
COMMENT ON COLUMN tenant_teams.created_at IS '团队创建时间';
COMMENT ON COLUMN tenant_teams.updated_at IS '团队最后更新时间';

COMMENT ON TABLE auth_users IS 'Web 控制台平台用户表';
COMMENT ON COLUMN auth_users.id IS '用户主键 UUID';
COMMENT ON COLUMN auth_users.username IS '登录账号，平台内唯一';
COMMENT ON COLUMN auth_users.display_name IS '用户展示名称，当前 MVP 可为空';
COMMENT ON COLUMN auth_users.email IS '用户邮箱，当前 MVP 可为空';
COMMENT ON COLUMN auth_users.password_hash IS '用户密码哈希，禁止存储明文密码';
COMMENT ON COLUMN auth_users.status IS '用户状态：active 表示启用，disabled 表示禁用';
COMMENT ON COLUMN auth_users.last_login_at IS '用户最后登录时间';
COMMENT ON COLUMN auth_users.disabled_at IS '用户禁用时间';
COMMENT ON COLUMN auth_users.deleted_at IS '用户软删除时间';
COMMENT ON COLUMN auth_users.created_at IS '用户创建时间';
COMMENT ON COLUMN auth_users.updated_at IS '用户最后更新时间';

COMMENT ON TABLE tenant_members IS '租户成员关系表';
COMMENT ON COLUMN tenant_members.id IS '成员关系主键 UUID';
COMMENT ON COLUMN tenant_members.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN tenant_members.team_id IS '所属团队 ID，可为空表示租户级成员';
COMMENT ON COLUMN tenant_members.principal_type IS '成员主体类型';
COMMENT ON COLUMN tenant_members.principal_id IS '成员主体 ID';
COMMENT ON COLUMN tenant_members.role IS '成员角色';
COMMENT ON COLUMN tenant_members.status IS '成员关系状态';
COMMENT ON COLUMN tenant_members.disabled_at IS '成员关系禁用时间';
COMMENT ON COLUMN tenant_members.created_at IS '成员关系创建时间';
COMMENT ON COLUMN tenant_members.updated_at IS '成员关系最后更新时间';

COMMENT ON TABLE runtime_nodes IS 'Runtime Agent 节点注册表';
COMMENT ON COLUMN runtime_nodes.id IS 'Runtime 节点主键 UUID';
COMMENT ON COLUMN runtime_nodes.tenant_id IS '默认所属租户 ID';
COMMENT ON COLUMN runtime_nodes.node_id IS 'Runtime 外部业务节点 ID';
COMMENT ON COLUMN runtime_nodes.name IS 'Runtime 节点名称';
COMMENT ON COLUMN runtime_nodes.supported_providers IS 'Runtime 支持的 Provider 列表';
COMMENT ON COLUMN runtime_nodes.max_slots IS 'Runtime 最大并发槽位数';
COMMENT ON COLUMN runtime_nodes.current_load IS 'Runtime 当前负载';
COMMENT ON COLUMN runtime_nodes.status IS 'Runtime 节点状态';
COMMENT ON COLUMN runtime_nodes.metadata IS 'Runtime 节点扩展元数据';
COMMENT ON COLUMN runtime_nodes.last_heartbeat_at IS 'Runtime 最后心跳时间';
COMMENT ON COLUMN runtime_nodes.disabled_at IS 'Runtime 节点禁用时间';
COMMENT ON COLUMN runtime_nodes.archived_at IS 'Runtime 节点归档时间';
COMMENT ON COLUMN runtime_nodes.created_at IS 'Runtime 节点注册时间';
COMMENT ON COLUMN runtime_nodes.updated_at IS 'Runtime 节点最后更新时间';

COMMENT ON TABLE runtime_node_scopes IS 'Runtime 节点可执行范围表';
COMMENT ON COLUMN runtime_node_scopes.id IS '节点范围主键 UUID';
COMMENT ON COLUMN runtime_node_scopes.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN runtime_node_scopes.runtime_node_id IS 'Runtime 节点 ID';
COMMENT ON COLUMN runtime_node_scopes.team_id IS '授权团队 ID';
COMMENT ON COLUMN runtime_node_scopes.scope_type IS '范围类型';
COMMENT ON COLUMN runtime_node_scopes.scope_value IS '范围值';
COMMENT ON COLUMN runtime_node_scopes.status IS '范围状态';
COMMENT ON COLUMN runtime_node_scopes.disabled_at IS '范围禁用时间';
COMMENT ON COLUMN runtime_node_scopes.created_at IS '范围创建时间';
COMMENT ON COLUMN runtime_node_scopes.updated_at IS '范围最后更新时间';

COMMENT ON TABLE auth_runtime_tokens IS 'Runtime Agent 认证令牌表';
COMMENT ON COLUMN auth_runtime_tokens.id IS 'Runtime 令牌主键 UUID';
COMMENT ON COLUMN auth_runtime_tokens.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN auth_runtime_tokens.node_id IS 'Runtime 外部业务节点 ID';
COMMENT ON COLUMN auth_runtime_tokens.token_hash IS 'Runtime 令牌哈希';
COMMENT ON COLUMN auth_runtime_tokens.expires_at IS 'Runtime 令牌过期时间';
COMMENT ON COLUMN auth_runtime_tokens.revoked_at IS 'Runtime 令牌撤销时间';
COMMENT ON COLUMN auth_runtime_tokens.created_at IS 'Runtime 令牌创建时间';

COMMENT ON TABLE auth_sessions IS 'Web 控制台用户会话表';
COMMENT ON COLUMN auth_sessions.id IS '会话主键 UUID';
COMMENT ON COLUMN auth_sessions.user_id IS '会话所属用户 ID';
COMMENT ON COLUMN auth_sessions.token_hash IS '会话令牌哈希';
COMMENT ON COLUMN auth_sessions.expires_at IS '会话过期时间';
COMMENT ON COLUMN auth_sessions.last_seen_at IS '会话最后访问时间';
COMMENT ON COLUMN auth_sessions.client_ip IS '客户端 IP';
COMMENT ON COLUMN auth_sessions.user_agent IS '客户端 User-Agent';
COMMENT ON COLUMN auth_sessions.revoked_at IS '会话撤销时间';
COMMENT ON COLUMN auth_sessions.created_at IS '会话创建时间';
COMMENT ON COLUMN auth_sessions.updated_at IS '会话最后更新时间';

COMMENT ON TABLE tasks IS '任务主表';
COMMENT ON COLUMN tasks.id IS '任务主键 UUID';
COMMENT ON COLUMN tasks.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN tasks.team_id IS '所属团队 ID';
COMMENT ON COLUMN tasks.title IS '任务标题';
COMMENT ON COLUMN tasks.description IS '任务描述';
COMMENT ON COLUMN tasks.creator_id IS '任务创建用户 ID';
COMMENT ON COLUMN tasks.provider_type IS '任务目标 Provider 类型';
COMMENT ON COLUMN tasks.target_node_id IS '指定 Runtime 外部业务节点 ID';
COMMENT ON COLUMN tasks.assigned_node_id IS '已分配 Runtime 外部业务节点 ID';
COMMENT ON COLUMN tasks.status IS '任务状态';
COMMENT ON COLUMN tasks.workspace_path IS '任务工作目录路径';
COMMENT ON COLUMN tasks.params IS '任务参数';
COMMENT ON COLUMN tasks.priority IS '任务优先级';
COMMENT ON COLUMN tasks.idempotency_key IS '任务幂等键';
COMMENT ON COLUMN tasks.risk_level IS '任务风险级别';
COMMENT ON COLUMN tasks.cancelled_at IS '任务取消时间';
COMMENT ON COLUMN tasks.deleted_at IS '任务软删除时间';
COMMENT ON COLUMN tasks.created_at IS '任务创建时间';
COMMENT ON COLUMN tasks.updated_at IS '任务最后更新时间';

COMMENT ON TABLE task_runs IS '任务运行记录表';
COMMENT ON COLUMN task_runs.id IS '任务运行主键 UUID';
COMMENT ON COLUMN task_runs.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN task_runs.task_id IS '所属任务 ID';
COMMENT ON COLUMN task_runs.node_id IS '执行 Runtime 外部业务节点 ID';
COMMENT ON COLUMN task_runs.runtime_node_id IS '执行 Runtime 节点 UUID';
COMMENT ON COLUMN task_runs.provider_session_id IS 'Provider 会话 ID';
COMMENT ON COLUMN task_runs.status IS '任务运行状态';
COMMENT ON COLUMN task_runs.lease_expires_at IS '任务运行租约过期时间';
COMMENT ON COLUMN task_runs.started_at IS '任务运行开始时间';
COMMENT ON COLUMN task_runs.completed_at IS '任务运行完成时间';
COMMENT ON COLUMN task_runs.finished_at IS '任务运行终止时间，包含完成、失败或取消';
COMMENT ON COLUMN task_runs.result IS '任务运行结果';
COMMENT ON COLUMN task_runs.error_message IS '任务运行错误信息';
COMMENT ON COLUMN task_runs.created_at IS '任务运行创建时间';
COMMENT ON COLUMN task_runs.updated_at IS '任务运行最后更新时间';

COMMENT ON TABLE runtime_leases IS 'Runtime 任务租约表';
COMMENT ON COLUMN runtime_leases.id IS '租约主键 UUID';
COMMENT ON COLUMN runtime_leases.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN runtime_leases.task_id IS '租约所属任务 ID';
COMMENT ON COLUMN runtime_leases.run_id IS '租约所属任务运行 ID';
COMMENT ON COLUMN runtime_leases.runtime_node_id IS '持有租约的 Runtime 节点 UUID';
COMMENT ON COLUMN runtime_leases.node_id IS '持有租约的 Runtime 外部业务节点 ID';
COMMENT ON COLUMN runtime_leases.lease_token IS '租约令牌';
COMMENT ON COLUMN runtime_leases.status IS '租约状态';
COMMENT ON COLUMN runtime_leases.expires_at IS '租约过期时间';
COMMENT ON COLUMN runtime_leases.renewed_at IS '租约续约时间';
COMMENT ON COLUMN runtime_leases.released_at IS '租约释放时间';
COMMENT ON COLUMN runtime_leases.cancelled_at IS '租约取消时间';
COMMENT ON COLUMN runtime_leases.created_at IS '租约创建时间';
COMMENT ON COLUMN runtime_leases.updated_at IS '租约最后更新时间';

COMMENT ON TABLE task_state_history IS '任务状态变更历史表';
COMMENT ON COLUMN task_state_history.id IS '状态历史主键 UUID';
COMMENT ON COLUMN task_state_history.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN task_state_history.task_id IS '所属任务 ID';
COMMENT ON COLUMN task_state_history.from_status IS '变更前任务状态';
COMMENT ON COLUMN task_state_history.to_status IS '变更后任务状态';
COMMENT ON COLUMN task_state_history.changed_by IS '状态变更触发者';
COMMENT ON COLUMN task_state_history.reason IS '状态变更原因';
COMMENT ON COLUMN task_state_history.created_at IS '状态变更记录时间';

COMMENT ON TABLE task_events IS '任务事件流表';
COMMENT ON COLUMN task_events.id IS '任务事件主键 UUID';
COMMENT ON COLUMN task_events.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN task_events.task_id IS '所属任务 ID';
COMMENT ON COLUMN task_events.run_id IS '所属任务运行 ID';
COMMENT ON COLUMN task_events.event_type IS '任务事件类型';
COMMENT ON COLUMN task_events.sequence_number IS '任务内事件序号';
COMMENT ON COLUMN task_events.payload IS '任务事件负载';
COMMENT ON COLUMN task_events.created_at IS '任务事件创建时间';

COMMENT ON TABLE task_artifacts IS '任务工件表';
COMMENT ON COLUMN task_artifacts.id IS '任务工件主键 UUID';
COMMENT ON COLUMN task_artifacts.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN task_artifacts.task_id IS '所属任务 ID';
COMMENT ON COLUMN task_artifacts.run_id IS '所属任务运行 ID';
COMMENT ON COLUMN task_artifacts.artifact_type IS '工件类型';
COMMENT ON COLUMN task_artifacts.name IS '工件名称';
COMMENT ON COLUMN task_artifacts.storage_url IS '工件存储地址';
COMMENT ON COLUMN task_artifacts.size_bytes IS '工件大小字节数';
COMMENT ON COLUMN task_artifacts.metadata IS '工件扩展元数据';
COMMENT ON COLUMN task_artifacts.archived_at IS '工件归档时间';
COMMENT ON COLUMN task_artifacts.deleted_at IS '工件软删除时间';
COMMENT ON COLUMN task_artifacts.created_at IS '工件创建时间';

COMMENT ON TABLE audit_events IS '审计事件表';
COMMENT ON COLUMN audit_events.id IS '审计事件主键 UUID';
COMMENT ON COLUMN audit_events.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN audit_events.event_type IS '审计事件类型';
COMMENT ON COLUMN audit_events.actor_type IS '操作者类型';
COMMENT ON COLUMN audit_events.actor_id IS '操作者 ID';
COMMENT ON COLUMN audit_events.resource_type IS '资源类型';
COMMENT ON COLUMN audit_events.resource_id IS '资源 ID';
COMMENT ON COLUMN audit_events.action IS '审计动作';
COMMENT ON COLUMN audit_events.details IS '审计扩展信息';
COMMENT ON COLUMN audit_events.ip_address IS '操作者 IP 地址';
COMMENT ON COLUMN audit_events.created_at IS '审计事件创建时间';

COMMENT ON TABLE web_login_logs IS 'Web 控制台登录日志表';
COMMENT ON COLUMN web_login_logs.id IS '登录日志主键 UUID';
COMMENT ON COLUMN web_login_logs.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN web_login_logs.event_type IS '登录事件类型';
COMMENT ON COLUMN web_login_logs.user_id IS '登录用户 ID，失败登录可为空';
COMMENT ON COLUMN web_login_logs.username IS '登录账号快照';
COMMENT ON COLUMN web_login_logs.session_id IS '登录会话 ID';
COMMENT ON COLUMN web_login_logs.client_ip IS '客户端 IP';
COMMENT ON COLUMN web_login_logs.user_agent IS '客户端 User-Agent';
COMMENT ON COLUMN web_login_logs.result IS '登录结果：succeeded 或 failed';
COMMENT ON COLUMN web_login_logs.failure_reason IS '登录失败原因';
COMMENT ON COLUMN web_login_logs.details IS '登录上下文扩展信息';
COMMENT ON COLUMN web_login_logs.created_at IS '登录事件发生时间';

COMMENT ON TABLE web_operation_logs IS 'Web 控制台操作日志表';
COMMENT ON COLUMN web_operation_logs.id IS '操作日志主键 UUID';
COMMENT ON COLUMN web_operation_logs.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN web_operation_logs.user_id IS '执行操作的用户 ID，用户删除后保留日志';
COMMENT ON COLUMN web_operation_logs.username IS '执行操作的用户账号快照';
COMMENT ON COLUMN web_operation_logs.module IS '操作所属模块';
COMMENT ON COLUMN web_operation_logs.resource_type IS '被操作资源类型';
COMMENT ON COLUMN web_operation_logs.resource_id IS '被操作资源 ID';
COMMENT ON COLUMN web_operation_logs.action IS '操作动作';
COMMENT ON COLUMN web_operation_logs.result IS '操作结果：succeeded 或 failed';
COMMENT ON COLUMN web_operation_logs.request_id IS '请求 ID，便于链路追踪';
COMMENT ON COLUMN web_operation_logs.client_ip IS '客户端 IP';
COMMENT ON COLUMN web_operation_logs.user_agent IS '客户端 User-Agent';
COMMENT ON COLUMN web_operation_logs.details IS '操作上下文扩展信息';
COMMENT ON COLUMN web_operation_logs.created_at IS '操作发生时间';
