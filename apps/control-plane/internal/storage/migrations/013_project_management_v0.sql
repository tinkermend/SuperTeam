-- 013_project_management_v0.sql
-- 创建项目管理模块 V0 阶段相关的基础表结构

-- 1. 项目主表 (projects)
CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID,
    name VARCHAR(255) NOT NULL,
    description TEXT,
    goal TEXT,
    status VARCHAR(50) NOT NULL,
    human_owner_user_id UUID NOT NULL,
    leader_user_id UUID,
    acceptance_user_id UUID,
    coordination_workflow_id VARCHAR(255),
    coordination_status VARCHAR(50),
    coordination_policy JSONB,
    approval_policy JSONB,
    evidence_policy JSONB,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE projects IS '项目核心事实容器';
COMMENT ON COLUMN projects.tenant_id IS '租户ID';
COMMENT ON COLUMN projects.team_id IS '团队ID（可选）';
COMMENT ON COLUMN projects.human_owner_user_id IS '人类负责人ID';
COMMENT ON COLUMN projects.coordination_workflow_id IS '绑定的 Temporal 工作流ID';

CREATE INDEX idx_projects_tenant_team ON projects(tenant_id, team_id);
CREATE INDEX idx_projects_tenant_status_created ON projects(tenant_id, status, created_at DESC);

-- 2. 项目成员/数字员工池 (project_members)
CREATE TABLE project_members (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    principal_type VARCHAR(50) NOT NULL,
    principal_id UUID NOT NULL,
    project_role VARCHAR(50) NOT NULL,
    display_name_snapshot VARCHAR(255),
    status VARCHAR(50) NOT NULL,
    settings JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_members IS '项目成员与数字员工池';
COMMENT ON COLUMN project_members.principal_type IS '成员类型：human_user / digital_employee / team';
COMMENT ON COLUMN project_members.project_role IS '项目内角色：owner / leader / acceptance / executor / reviewer / observer';

CREATE INDEX idx_project_members_tenant_project ON project_members(tenant_id, project_id);
CREATE UNIQUE INDEX uq_project_members_principal_role ON project_members(tenant_id, project_id, principal_type, principal_id, project_role) WHERE status = 'active';

-- 3. 项目业务任务表 (project_tasks)
CREATE TABLE project_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    status VARCHAR(50) NOT NULL,
    assigned_digital_employee_id UUID,
    runtime_task_id UUID,
    digital_employee_run_id UUID,
    risk_level VARCHAR(50),
    requires_human_approval BOOLEAN NOT NULL DEFAULT false,
    latest_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_tasks IS '项目内可分派、可执行的工作项';
COMMENT ON COLUMN project_tasks.runtime_task_id IS '关联的底层 Runtime 任务ID（若有）';
COMMENT ON COLUMN project_tasks.status IS '任务状态：pending, planned, assigned, running, waiting_human, completed, failed, cancelled';

CREATE INDEX idx_project_tasks_tenant_project_status ON project_tasks(tenant_id, project_id, status);

-- 4. 项目事件流 (project_events)
CREATE TABLE project_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    sequence_number BIGINT NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    actor_type VARCHAR(50) NOT NULL,
    actor_id VARCHAR(255) NOT NULL,
    resource_type VARCHAR(50),
    resource_id VARCHAR(255),
    summary TEXT,
    payload JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_events IS '项目生命周期内的关键事件流';
COMMENT ON COLUMN project_events.sequence_number IS '项目内的局部递增序号';
COMMENT ON COLUMN project_events.actor_type IS '事件发起者类型：human_user / digital_employee / project_coordinator / system';

CREATE INDEX idx_project_events_tenant_project ON project_events(tenant_id, project_id);
CREATE UNIQUE INDEX uq_project_events_project_sequence ON project_events(project_id, sequence_number);
CREATE INDEX idx_project_events_tenant_project_sequence ON project_events(tenant_id, project_id, sequence_number DESC);

-- 5. 项目需求表 (project_demands)
CREATE TABLE project_demands (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    submitted_by_user_id UUID NOT NULL,
    title VARCHAR(255) NOT NULL,
    content TEXT,
    source_type VARCHAR(50) NOT NULL,
    source_refs JSONB,
    attachments JSONB,
    priority VARCHAR(50),
    risk_level VARCHAR(50),
    status VARCHAR(50) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_demands IS '用户或外部系统提交到项目的需求';
COMMENT ON COLUMN project_demands.source_type IS '来源类型：manual / github / ticket / document / log';
COMMENT ON COLUMN project_demands.status IS '需求状态：submitted, recorded, planning_pending, cancelled';

CREATE INDEX idx_project_demands_tenant_project_status ON project_demands(tenant_id, project_id, status);

-- 6. 项目配置修订历史 (project_config_revisions)
CREATE TABLE project_config_revisions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    revision_number INTEGER NOT NULL,
    config_snapshot JSONB NOT NULL,
    change_summary TEXT,
    created_by_user_id UUID NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_config_revisions IS '项目配置与策略变更历史';
COMMENT ON COLUMN project_config_revisions.revision_number IS '项目内的递增版本号';

CREATE INDEX idx_project_config_revisions_tenant_project ON project_config_revisions(tenant_id, project_id);
CREATE UNIQUE INDEX uq_project_config_revisions_project_rev ON project_config_revisions(project_id, revision_number);
