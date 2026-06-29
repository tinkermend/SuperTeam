ALTER TABLE projects
    ADD COLUMN repo_url TEXT,
    ADD COLUMN repo_default_branch VARCHAR(255),
    ADD COLUMN repo_git_credential_ref VARCHAR(255),
    ADD COLUMN repo_scope JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN repo_binding_status VARCHAR(32) NOT NULL DEFAULT 'unbound';

ALTER TABLE projects
    ADD CONSTRAINT chk_projects_repo_binding_status
    CHECK (repo_binding_status IN ('unbound', 'bound'));

ALTER TABLE projects
    ADD CONSTRAINT chk_projects_repo_binding_consistent
    CHECK (
        (repo_binding_status = 'unbound' AND repo_url IS NULL AND repo_default_branch IS NULL)
        OR
        (repo_binding_status = 'bound' AND repo_url IS NOT NULL AND repo_default_branch IS NOT NULL)
    );

CREATE INDEX idx_projects_repo_binding_status
    ON projects(tenant_id, repo_binding_status)
    WHERE repo_binding_status = 'bound';

-- Required target for the project placement FK. PostgreSQL requires composite
-- foreign keys to reference a unique or primary-key-backed column set.
CREATE UNIQUE INDEX uq_projects_tenant_id
    ON projects(tenant_id, id);

CREATE TABLE project_placements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    placement_status VARCHAR(32) NOT NULL,
    placement_reason VARCHAR(100),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_placements_project
        FOREIGN KEY (tenant_id, project_id)
        REFERENCES projects(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_project_placements_status
        CHECK (placement_status IN ('active', 'released', 'lost'))
);

CREATE UNIQUE INDEX uq_project_placements_active
    ON project_placements(tenant_id, project_id)
    WHERE placement_status = 'active';

CREATE INDEX idx_project_placements_runtime_node
    ON project_placements(tenant_id, runtime_node_id, placement_status);

CREATE TABLE project_task_attestations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    attempt_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    provider_session_id VARCHAR(255),
    attestation_type VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL,
    command_argv JSONB NOT NULL DEFAULT '[]'::jsonb,
    exit_code INTEGER,
    duration_ms BIGINT,
    log_ref TEXT,
    stdout_sha256 VARCHAR(64),
    stderr_sha256 VARCHAR(64),
    artifact_refs JSONB NOT NULL DEFAULT '[]'::jsonb,
    artifact_hashes JSONB NOT NULL DEFAULT '{}'::jsonb,
    git_branch VARCHAR(255),
    git_base_ref VARCHAR(255),
    git_head_sha VARCHAR(64),
    git_diff_sha256 VARCHAR(64),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_task_attestations_task
        FOREIGN KEY (tenant_id, project_id, project_task_id)
        REFERENCES project_tasks(tenant_id, project_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_project_task_attestations_attempt
        FOREIGN KEY (tenant_id, project_task_id, attempt_id)
        REFERENCES project_task_attempts(tenant_id, project_task_id, id) ON DELETE CASCADE,
    CONSTRAINT chk_project_task_attestations_status
        CHECK (status IN ('succeeded', 'failed', 'cancelled', 'timed_out'))
);

CREATE UNIQUE INDEX uq_project_task_attestations_idempotency
    ON project_task_attestations(tenant_id, attempt_id, idempotency_key);

CREATE INDEX idx_project_task_attestations_task_created
    ON project_task_attestations(tenant_id, project_id, project_task_id, created_at DESC);

ALTER TABLE project_task_attempts
    ADD COLUMN budget_wall_clock_limit_sec INTEGER,
    ADD COLUMN budget_last_heartbeat_at TIMESTAMPTZ,
    ADD COLUMN budget_consumed_wall_clock_sec INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN budget_consumed_tokens INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN budget_tripped_at TIMESTAMPTZ,
    ADD COLUMN budget_trip_reason VARCHAR(100);

CREATE TRIGGER update_project_placements_updated_at
    BEFORE UPDATE ON project_placements
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_project_task_attestations_updated_at
    BEFORE UPDATE ON project_task_attestations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON COLUMN projects.repo_url IS '项目 Phase 1 源码仓库 URL；为空表示项目不绑定源码。';
COMMENT ON COLUMN projects.repo_default_branch IS '项目源码默认 base ref，不保存本地绝对路径。';
COMMENT ON COLUMN projects.repo_git_credential_ref IS '项目级 git 凭据引用，与员工 MCP 凭据分离。';
COMMENT ON COLUMN projects.repo_scope IS '仓库 sparse checkout scope，必须由业务侧保证包含传递依赖闭包。';
COMMENT ON TABLE project_placements IS '项目到 Runtime 节点的动态亲和放置状态，不保存本地绝对路径。';
COMMENT ON TABLE project_task_attestations IS 'Runtime 对项目任务执行的结构化证明，保存命令、退出码、日志哈希、产物哈希和 git 证据引用。';
COMMENT ON COLUMN project_task_attempts.budget_wall_clock_limit_sec IS '该尝试的墙钟预算上限，空表示未设置。';
COMMENT ON COLUMN project_task_attempts.budget_trip_reason IS '预算熔断原因，例如 wall_clock_exceeded、token_limit_exceeded。';
