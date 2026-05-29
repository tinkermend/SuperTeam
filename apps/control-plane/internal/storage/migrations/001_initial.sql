-- SuperTeam Control Plane Initial Schema
-- Migration: 001_initial
-- Created: 2026-05-29

-- ============================================================================
-- Runtime Module
-- ============================================================================

-- Runtime Agent 节点注册表
CREATE TABLE runtime_nodes (
    id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    supported_providers JSONB NOT NULL,
    max_slots INTEGER NOT NULL DEFAULT 1,
    current_load INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(50) NOT NULL,
    metadata JSONB,
    last_heartbeat_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_runtime_nodes_status ON runtime_nodes(status);
CREATE INDEX idx_runtime_nodes_last_heartbeat ON runtime_nodes(last_heartbeat_at);
CREATE INDEX idx_runtime_nodes_supported_providers ON runtime_nodes USING GIN (supported_providers);
CREATE INDEX idx_runtime_nodes_status_heartbeat ON runtime_nodes(status, last_heartbeat_at DESC);

-- ============================================================================
-- Auth Module
-- ============================================================================

-- 用户表
CREATE TABLE auth_users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    display_name VARCHAR(255),
    email VARCHAR(255) UNIQUE,
    password_hash VARCHAR(255),
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_users_username ON auth_users(username);
CREATE INDEX idx_auth_users_email ON auth_users(email);
CREATE INDEX idx_auth_users_status ON auth_users(status);

-- Runtime Agent 认证 token 表
CREATE TABLE auth_runtime_tokens (
    id BIGSERIAL PRIMARY KEY,
    node_id VARCHAR(255) UNIQUE NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_auth_runtime_tokens_node_id ON auth_runtime_tokens(node_id);

-- ============================================================================
-- Task Module
-- ============================================================================

-- 任务主表
CREATE TABLE tasks (
    id BIGSERIAL PRIMARY KEY,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    creator_id BIGINT REFERENCES auth_users(id),
    provider_type VARCHAR(50) NOT NULL,
    target_node_id VARCHAR(255),
    assigned_node_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    workspace_path TEXT,
    params JSONB NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tasks_status ON tasks(status);
CREATE INDEX idx_tasks_provider_type ON tasks(provider_type);
CREATE INDEX idx_tasks_assigned_node_id ON tasks(assigned_node_id);
CREATE INDEX idx_tasks_creator_id ON tasks(creator_id);
CREATE INDEX idx_tasks_params ON tasks USING GIN (params);
CREATE INDEX idx_tasks_status_priority_created ON tasks(status, priority DESC, created_at DESC);

-- 任务执行记录
CREATE TABLE task_executions (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    node_id VARCHAR(255) NOT NULL,
    provider_session_id VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    result JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_executions_task_id ON task_executions(task_id);
CREATE INDEX idx_task_executions_status ON task_executions(status);

-- 任务状态变更历史
CREATE TABLE task_state_history (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    from_status VARCHAR(50),
    to_status VARCHAR(50) NOT NULL,
    changed_by VARCHAR(255),
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_state_history_task_id ON task_state_history(task_id);

-- 任务事件流
CREATE TABLE task_events (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    execution_id BIGINT REFERENCES task_executions(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    sequence_number INTEGER NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_events_task_id ON task_events(task_id);
CREATE INDEX idx_task_events_execution_id ON task_events(execution_id);
CREATE INDEX idx_task_events_sequence ON task_events(task_id, sequence_number);

-- 任务工件
CREATE TABLE task_artifacts (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    execution_id BIGINT REFERENCES task_executions(id) ON DELETE CASCADE,
    artifact_type VARCHAR(100) NOT NULL,
    name VARCHAR(500) NOT NULL,
    storage_url TEXT NOT NULL,
    size_bytes BIGINT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_task_artifacts_task_id ON task_artifacts(task_id);
CREATE INDEX idx_task_artifacts_type ON task_artifacts(artifact_type);

-- ============================================================================
-- Audit Module
-- ============================================================================

-- 审计事件
CREATE TABLE audit_events (
    id BIGSERIAL PRIMARY KEY,
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

CREATE INDEX idx_audit_events_actor ON audit_events(actor_type, actor_id);
CREATE INDEX idx_audit_events_resource ON audit_events(resource_type, resource_id);
CREATE INDEX idx_audit_events_created_at ON audit_events(created_at);

-- ============================================================================
-- Triggers for auto-updating updated_at
-- ============================================================================

-- Trigger function for auto-updating updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply trigger to tables with updated_at
CREATE TRIGGER update_runtime_nodes_updated_at BEFORE UPDATE ON runtime_nodes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_auth_users_updated_at BEFORE UPDATE ON auth_users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_tasks_updated_at BEFORE UPDATE ON tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
