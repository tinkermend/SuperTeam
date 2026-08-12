-- 项目工作区 git 状态快照（spec 2026-08-12 P1）：1:1 侧表，不往 projects 主表加列。
CREATE TABLE IF NOT EXISTS project_workspace_git_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    is_git_repo BOOLEAN,
    is_clean BOOLEAN,
    head_commit VARCHAR(64),
    current_branch VARCHAR(255),
    detached BOOLEAN NOT NULL DEFAULT false,
    repo_state VARCHAR(32),
    uncommitted_count INTEGER NOT NULL DEFAULT 0,
    uncommitted_entries JSONB NOT NULL DEFAULT '[]'::jsonb,
    uncommitted_truncated BOOLEAN NOT NULL DEFAULT false,
    uncommitted_omitted INTEGER NOT NULL DEFAULT 0,
    sampled_at TIMESTAMPTZ,
    sampled_runtime_node_id UUID,
    sampled_node_id VARCHAR(128),
    sample_error TEXT,
    last_attempt_at TIMESTAMPTZ,
    inflight_at TIMESTAMPTZ,
    inflight_command_id VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_project_workspace_git_snapshots_project UNIQUE (project_id)
);

CREATE INDEX IF NOT EXISTS idx_project_workspace_git_snapshots_tenant_sampled
    ON project_workspace_git_snapshots (tenant_id, sampled_at);

COMMENT ON TABLE project_workspace_git_snapshots IS
  '项目工作区最近一次观测到的 git 状态快照；覆盖写，清单截断后落 JSONB。';
COMMENT ON COLUMN project_workspace_git_snapshots.tenant_id IS '租户 ID。';
COMMENT ON COLUMN project_workspace_git_snapshots.project_id IS '项目 ID；与 projects 1:1。';
COMMENT ON COLUMN project_workspace_git_snapshots.is_git_repo IS '是否 git 仓库；NULL 表示尚未成功观测。非 git 为不适用，不是干净。';
COMMENT ON COLUMN project_workspace_git_snapshots.is_clean IS '相对 HEAD 无未提交业务改动；非 git 时为 NULL。';
COMMENT ON COLUMN project_workspace_git_snapshots.head_commit IS 'rev-parse HEAD 实际提交 ID。';
COMMENT ON COLUMN project_workspace_git_snapshots.current_branch IS '当前分支名；detached 时为空。';
COMMENT ON COLUMN project_workspace_git_snapshots.detached IS 'HEAD 是否 detached。';
COMMENT ON COLUMN project_workspace_git_snapshots.repo_state IS 'ok / detached / rebase / merge；中间态不得糊成脏。';
COMMENT ON COLUMN project_workspace_git_snapshots.uncommitted_count IS '未提交条目总数（含截断未列出）。';
COMMENT ON COLUMN project_workspace_git_snapshots.uncommitted_entries IS '截断后的未提交清单（path + category）。';
COMMENT ON COLUMN project_workspace_git_snapshots.uncommitted_truncated IS '清单是否被截断。';
COMMENT ON COLUMN project_workspace_git_snapshots.uncommitted_omitted IS '截断未列出的条数。';
COMMENT ON COLUMN project_workspace_git_snapshots.sampled_at IS '最近一次成功写入快照的时间；失败不覆盖。';
COMMENT ON COLUMN project_workspace_git_snapshots.sampled_runtime_node_id IS '采自主节点 UUID。';
COMMENT ON COLUMN project_workspace_git_snapshots.sampled_node_id IS '采自主节点对外 node_id。';
COMMENT ON COLUMN project_workspace_git_snapshots.sample_error IS '最近一次未采到原因；成功时清空。失败保留上次快照。';
COMMENT ON COLUMN project_workspace_git_snapshots.last_attempt_at IS '最近一次探测尝试时间（含失败）。';
COMMENT ON COLUMN project_workspace_git_snapshots.inflight_at IS '在飞探测开始时间；用于同项目节流。';
COMMENT ON COLUMN project_workspace_git_snapshots.inflight_command_id IS '在飞探测 command_id。';
COMMENT ON COLUMN project_workspace_git_snapshots.created_at IS '行创建时间。';
COMMENT ON COLUMN project_workspace_git_snapshots.updated_at IS '行更新时间。';
