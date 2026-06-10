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
    coordination_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    evidence_policy JSONB NOT NULL DEFAULT '{}'::jsonb,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE projects IS '项目核心事实容器';
COMMENT ON COLUMN projects.id IS '项目ID';
COMMENT ON COLUMN projects.tenant_id IS '租户ID';
COMMENT ON COLUMN projects.team_id IS '团队ID（可选）';
COMMENT ON COLUMN projects.name IS '项目名称';
COMMENT ON COLUMN projects.description IS '项目说明';
COMMENT ON COLUMN projects.goal IS '项目目标或问题场景闭环目标';
COMMENT ON COLUMN projects.status IS '项目状态，由应用层校验';
COMMENT ON COLUMN projects.human_owner_user_id IS '人类负责人ID';
COMMENT ON COLUMN projects.leader_user_id IS '项目负责人或推进人用户ID';
COMMENT ON COLUMN projects.acceptance_user_id IS '项目验收人用户ID';
COMMENT ON COLUMN projects.coordination_workflow_id IS '绑定的 Temporal 工作流ID';
COMMENT ON COLUMN projects.coordination_status IS '虚拟协调线程状态';
COMMENT ON COLUMN projects.coordination_policy IS '项目协调策略配置';
COMMENT ON COLUMN projects.approval_policy IS '项目审批策略配置';
COMMENT ON COLUMN projects.evidence_policy IS '项目证据与工件策略配置';
COMMENT ON COLUMN projects.archived_at IS '项目归档时间';
COMMENT ON COLUMN projects.created_at IS '项目创建时间';
COMMENT ON COLUMN projects.updated_at IS '项目更新时间';

CREATE INDEX idx_projects_tenant_team ON projects(tenant_id, team_id);
CREATE INDEX idx_projects_tenant_status_created ON projects(tenant_id, status, created_at DESC);
CREATE TRIGGER update_projects_updated_at BEFORE UPDATE ON projects FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

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
    settings JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_members IS '项目成员与数字员工池';
COMMENT ON COLUMN project_members.id IS '项目成员记录ID';
COMMENT ON COLUMN project_members.tenant_id IS '租户ID';
COMMENT ON COLUMN project_members.project_id IS '所属项目ID';
COMMENT ON COLUMN project_members.principal_type IS '成员类型：human_user / digital_employee / team';
COMMENT ON COLUMN project_members.principal_id IS '成员主体ID';
COMMENT ON COLUMN project_members.project_role IS '项目内角色：owner / leader / acceptance / executor / reviewer / observer';
COMMENT ON COLUMN project_members.display_name_snapshot IS '成员展示名称快照';
COMMENT ON COLUMN project_members.status IS '成员状态，由应用层校验';
COMMENT ON COLUMN project_members.settings IS '项目成员级配置';
COMMENT ON COLUMN project_members.created_at IS '项目成员创建时间';
COMMENT ON COLUMN project_members.updated_at IS '项目成员更新时间';

CREATE INDEX idx_project_members_tenant_project ON project_members(tenant_id, project_id);
CREATE UNIQUE INDEX uq_project_members_principal_role ON project_members(tenant_id, project_id, principal_type, principal_id, project_role) WHERE status = 'active';
CREATE TRIGGER update_project_members_updated_at BEFORE UPDATE ON project_members FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

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
COMMENT ON COLUMN project_tasks.id IS '项目任务ID';
COMMENT ON COLUMN project_tasks.tenant_id IS '租户ID';
COMMENT ON COLUMN project_tasks.project_id IS '所属项目ID';
COMMENT ON COLUMN project_tasks.demand_id IS '关联项目需求ID';
COMMENT ON COLUMN project_tasks.title IS '项目任务标题';
COMMENT ON COLUMN project_tasks.summary IS '项目任务摘要';
COMMENT ON COLUMN project_tasks.status IS '任务状态：pending, planned, assigned, running, waiting_human, completed, failed, cancelled';
COMMENT ON COLUMN project_tasks.assigned_digital_employee_id IS '当前分派的数字员工ID';
COMMENT ON COLUMN project_tasks.runtime_task_id IS '关联的底层 Runtime 任务ID（若有）';
COMMENT ON COLUMN project_tasks.digital_employee_run_id IS '关联的数字员工执行记录ID（若有）';
COMMENT ON COLUMN project_tasks.risk_level IS '任务风险等级，由应用层和策略校验';
COMMENT ON COLUMN project_tasks.requires_human_approval IS '是否需要人类审批后继续';
COMMENT ON COLUMN project_tasks.latest_event_id IS '最新关联项目事件ID';
COMMENT ON COLUMN project_tasks.created_at IS '项目任务创建时间';
COMMENT ON COLUMN project_tasks.updated_at IS '项目任务更新时间';

CREATE INDEX idx_project_tasks_tenant_project_status ON project_tasks(tenant_id, project_id, status);
CREATE TRIGGER update_project_tasks_updated_at BEFORE UPDATE ON project_tasks FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

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
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_events IS '项目生命周期内的关键事件流';
COMMENT ON COLUMN project_events.id IS '项目事件ID';
COMMENT ON COLUMN project_events.tenant_id IS '租户ID';
COMMENT ON COLUMN project_events.project_id IS '所属项目ID';
COMMENT ON COLUMN project_events.sequence_number IS '项目内的局部递增序号';
COMMENT ON COLUMN project_events.event_type IS '项目事件类型，由应用层注册和校验';
COMMENT ON COLUMN project_events.actor_type IS '事件发起者类型：human_user / digital_employee / project_coordinator / system';
COMMENT ON COLUMN project_events.actor_id IS '事件发起者ID或系统标识';
COMMENT ON COLUMN project_events.resource_type IS '事件关联资源类型';
COMMENT ON COLUMN project_events.resource_id IS '事件关联资源ID';
COMMENT ON COLUMN project_events.summary IS '事件摘要';
COMMENT ON COLUMN project_events.payload IS '事件结构化载荷';
COMMENT ON COLUMN project_events.created_at IS '事件创建时间';

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
    source_refs JSONB NOT NULL DEFAULT '{}'::jsonb,
    attachments JSONB NOT NULL DEFAULT '[]'::jsonb,
    priority VARCHAR(50),
    risk_level VARCHAR(50),
    status VARCHAR(50) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_demands IS '用户或外部系统提交到项目的需求';
COMMENT ON COLUMN project_demands.id IS '项目需求ID';
COMMENT ON COLUMN project_demands.tenant_id IS '租户ID';
COMMENT ON COLUMN project_demands.project_id IS '所属项目ID';
COMMENT ON COLUMN project_demands.submitted_by_user_id IS '提交需求的人类用户ID';
COMMENT ON COLUMN project_demands.title IS '需求标题';
COMMENT ON COLUMN project_demands.content IS '需求正文';
COMMENT ON COLUMN project_demands.source_type IS '来源类型：manual / github / ticket / document / log';
COMMENT ON COLUMN project_demands.source_refs IS '需求来源引用信息';
COMMENT ON COLUMN project_demands.attachments IS '需求附件引用列表';
COMMENT ON COLUMN project_demands.priority IS '需求优先级，由应用层校验';
COMMENT ON COLUMN project_demands.risk_level IS '需求风险等级，由应用层和策略校验';
COMMENT ON COLUMN project_demands.status IS '需求状态：submitted, recorded, planning_pending, cancelled';
COMMENT ON COLUMN project_demands.created_event_id IS '创建该需求时产生的项目事件ID';
COMMENT ON COLUMN project_demands.created_at IS '需求创建时间';
COMMENT ON COLUMN project_demands.updated_at IS '需求更新时间';

CREATE INDEX idx_project_demands_tenant_project_status ON project_demands(tenant_id, project_id, status);
CREATE TRIGGER update_project_demands_updated_at BEFORE UPDATE ON project_demands FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

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
COMMENT ON COLUMN project_config_revisions.id IS '项目配置修订ID';
COMMENT ON COLUMN project_config_revisions.tenant_id IS '租户ID';
COMMENT ON COLUMN project_config_revisions.project_id IS '所属项目ID';
COMMENT ON COLUMN project_config_revisions.revision_number IS '项目内的递增版本号';
COMMENT ON COLUMN project_config_revisions.config_snapshot IS '项目配置与策略快照';
COMMENT ON COLUMN project_config_revisions.change_summary IS '配置变更摘要';
COMMENT ON COLUMN project_config_revisions.created_by_user_id IS '创建该配置修订的人类用户ID';
COMMENT ON COLUMN project_config_revisions.created_event_id IS '创建该修订时产生的项目事件ID';
COMMENT ON COLUMN project_config_revisions.created_at IS '配置修订创建时间';

CREATE INDEX idx_project_config_revisions_tenant_project ON project_config_revisions(tenant_id, project_id);
CREATE UNIQUE INDEX uq_project_config_revisions_project_rev ON project_config_revisions(project_id, revision_number);
