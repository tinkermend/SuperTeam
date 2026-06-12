-- 016_inbox_items.sql
-- 收件箱：可操作事项队列 read model

CREATE TABLE inbox_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID,
    target_user_id UUID NOT NULL,
    scope VARCHAR(50) NOT NULL,
    item_type VARCHAR(100) NOT NULL,
    source_type VARCHAR(100) NOT NULL,
    source_id UUID NOT NULL,
    source_project_id UUID,
    source_task_id UUID,
    source_approval_request_id UUID,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    risk_level VARCHAR(50),
    priority VARCHAR(50),
    status VARCHAR(50) NOT NULL,
    action_schema JSONB NOT NULL DEFAULT '[]'::jsonb,
    context_payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    deep_link JSONB NOT NULL DEFAULT '{}'::jsonb,
    resolved_at TIMESTAMPTZ,
    last_activity_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE inbox_items IS '收件箱可操作事项 read model，聚合需要人类用户处理的审批和项目决策待办';
COMMENT ON COLUMN inbox_items.id IS '收件箱事项ID';
COMMENT ON COLUMN inbox_items.tenant_id IS '租户ID';
COMMENT ON COLUMN inbox_items.team_id IS '团队ID，可为空；第一版主要用于后续团队范围过滤';
COMMENT ON COLUMN inbox_items.target_user_id IS '目标处理人用户ID；第一版只有目标处理人可以执行动作';
COMMENT ON COLUMN inbox_items.scope IS '事项范围：personal 或 team，由应用层校验';
COMMENT ON COLUMN inbox_items.item_type IS '收件箱事项类型，例如 approval 或 project_decision，由应用层注册校验';
COMMENT ON COLUMN inbox_items.source_type IS '来源对象类型，例如 approval_request 或 project_decision_request';
COMMENT ON COLUMN inbox_items.source_id IS '来源对象ID，与 source_type 共同定位事实源';
COMMENT ON COLUMN inbox_items.source_project_id IS '来源项目ID，用于项目筛选和上下文跳转';
COMMENT ON COLUMN inbox_items.source_task_id IS '来源项目任务ID，用于后续任务上下文筛选';
COMMENT ON COLUMN inbox_items.source_approval_request_id IS '关联全局审批请求ID，用于防止审批和项目决策重复投影';
COMMENT ON COLUMN inbox_items.title IS '事项标题快照';
COMMENT ON COLUMN inbox_items.summary IS '事项摘要快照';
COMMENT ON COLUMN inbox_items.risk_level IS '风险等级快照，例如 low、medium、high';
COMMENT ON COLUMN inbox_items.priority IS '队列优先级快照，由应用层校验';
COMMENT ON COLUMN inbox_items.status IS '收件箱状态：open、resolved、cancelled，由来源状态驱动';
COMMENT ON COLUMN inbox_items.action_schema IS '前端可展示动作 schema，最终合法性仍由来源服务校验';
COMMENT ON COLUMN inbox_items.context_payload IS '展示上下文快照，不作为业务事实源';
COMMENT ON COLUMN inbox_items.deep_link IS '前端上下文跳转信息';
COMMENT ON COLUMN inbox_items.resolved_at IS '事项关闭时间';
COMMENT ON COLUMN inbox_items.last_activity_at IS '最后活动时间，用于收件箱排序';
COMMENT ON COLUMN inbox_items.created_at IS '事项创建时间';
COMMENT ON COLUMN inbox_items.updated_at IS '事项更新时间';

CREATE INDEX idx_inbox_items_tenant_target_status_activity
    ON inbox_items(tenant_id, target_user_id, status, last_activity_at DESC);

CREATE INDEX idx_inbox_items_tenant_status_activity
    ON inbox_items(tenant_id, status, last_activity_at DESC);

CREATE INDEX idx_inbox_items_tenant_team_status_activity
    ON inbox_items(tenant_id, team_id, status, last_activity_at DESC)
    WHERE team_id IS NOT NULL;

CREATE INDEX idx_inbox_items_tenant_project_status_activity
    ON inbox_items(tenant_id, source_project_id, status, last_activity_at DESC)
    WHERE source_project_id IS NOT NULL;

CREATE UNIQUE INDEX uq_inbox_items_tenant_source
    ON inbox_items(tenant_id, source_type, source_id);

CREATE UNIQUE INDEX uq_inbox_items_tenant_approval_source
    ON inbox_items(tenant_id, source_approval_request_id)
    WHERE source_approval_request_id IS NOT NULL;

CREATE TRIGGER update_inbox_items_updated_at
    BEFORE UPDATE ON inbox_items
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
