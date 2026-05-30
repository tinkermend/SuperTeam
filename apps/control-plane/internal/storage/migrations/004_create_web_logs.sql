-- Web 控制台登录与操作日志表

CREATE TABLE IF NOT EXISTS web_login_logs (
    id BIGSERIAL PRIMARY KEY,
    event_type VARCHAR(50) NOT NULL CHECK (event_type IN ('login_succeeded', 'login_failed', 'logout_succeeded')),
    user_id BIGINT REFERENCES auth_users(id) ON DELETE SET NULL,
    username VARCHAR(255) NOT NULL,
    session_id VARCHAR(255),
    client_ip VARCHAR(255),
    user_agent TEXT,
    result VARCHAR(50) NOT NULL CHECK (result IN ('succeeded', 'failed')),
    failure_reason VARCHAR(100),
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_web_login_logs_event_type_created ON web_login_logs(event_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_web_login_logs_user_id_created ON web_login_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_web_login_logs_username_created ON web_login_logs(username, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_web_login_logs_created_at ON web_login_logs(created_at DESC);

CREATE TABLE IF NOT EXISTS web_operation_logs (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES auth_users(id) ON DELETE SET NULL,
    username VARCHAR(255),
    module VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100),
    resource_id VARCHAR(255),
    action VARCHAR(100) NOT NULL,
    result VARCHAR(50) NOT NULL CHECK (result IN ('succeeded', 'failed')),
    request_id VARCHAR(255),
    client_ip VARCHAR(255),
    user_agent TEXT,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_web_operation_logs_user_id_created ON web_operation_logs(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_web_operation_logs_module_action_created ON web_operation_logs(module, action, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_web_operation_logs_resource ON web_operation_logs(resource_type, resource_id);
CREATE INDEX IF NOT EXISTS idx_web_operation_logs_created_at ON web_operation_logs(created_at DESC);
