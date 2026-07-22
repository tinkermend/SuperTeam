-- 自动化任务（定时规则）：配置事实源在 Control Plane；Temporal Schedule 只持时钟。
-- 见 docs/superpowers/specs/2026-07-22-automation-tasks-design.md

CREATE TABLE automation_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    team_id UUID NOT NULL,
    project_id UUID NOT NULL,
    name VARCHAR(200) NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    coordination_mode VARCHAR(20) NOT NULL,
    demand_title_template TEXT,
    demand_body_template TEXT,
    scenario_template_key VARCHAR(128),
    digital_employee_id UUID,
    chat_objective_template TEXT,
    schedule_kind VARCHAR(20) NOT NULL,
    cron_expr VARCHAR(128),
    interval_seconds INTEGER,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    overlap_policy VARCHAR(32) NOT NULL DEFAULT 'skip',
    actor_user_id UUID NOT NULL,
    disabled_reason VARCHAR(64),
    consecutive_failure_count INTEGER NOT NULL DEFAULT 0,
    temporal_schedule_id VARCHAR(200),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_automation_rules_tenant_enabled
    ON automation_rules (tenant_id, enabled);

CREATE INDEX idx_automation_rules_tenant_project
    ON automation_rules (tenant_id, project_id);

CREATE INDEX idx_automation_rules_tenant_actor
    ON automation_rules (tenant_id, actor_user_id);

CREATE TABLE automation_fires (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    rule_id UUID NOT NULL,
    scheduled_fire_at TIMESTAMPTZ NOT NULL,
    idempotency_key VARCHAR(200) NOT NULL,
    status VARCHAR(32) NOT NULL,
    demand_id UUID,
    run_id UUID,
    error_code VARCHAR(64),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX uq_automation_fires_idempotency
    ON automation_fires (idempotency_key);

CREATE INDEX idx_automation_fires_tenant_rule_scheduled
    ON automation_fires (tenant_id, rule_id, scheduled_fire_at DESC);

COMMENT ON TABLE automation_rules IS '自动化定时规则：到点触发任务中枢 SubmitDemand 或员工 chat run；发起人固定为创建者';
COMMENT ON COLUMN automation_rules.tenant_id IS '租户 ID';
COMMENT ON COLUMN automation_rules.team_id IS '项目所属团队 ID（创建时从项目快照）';
COMMENT ON COLUMN automation_rules.project_id IS '锚点项目；创建/启用/触发均校验发起人对该项目有发起权';
COMMENT ON COLUMN automation_rules.name IS '规则显示名称';
COMMENT ON COLUMN automation_rules.enabled IS '是否启用；失权或连败 3 次时由系统置 false';
COMMENT ON COLUMN automation_rules.coordination_mode IS 'plan | loop | chat';
COMMENT ON COLUMN automation_rules.demand_title_template IS 'plan/loop 需求标题模板，支持 {{date}} 等变量';
COMMENT ON COLUMN automation_rules.demand_body_template IS 'plan/loop 需求正文模板';
COMMENT ON COLUMN automation_rules.scenario_template_key IS '可选场景模板 key';
COMMENT ON COLUMN automation_rules.digital_employee_id IS 'chat 模式目标数字员工';
COMMENT ON COLUMN automation_rules.chat_objective_template IS 'chat 对话目标模板';
COMMENT ON COLUMN automation_rules.schedule_kind IS 'cron | interval';
COMMENT ON COLUMN automation_rules.cron_expr IS 'schedule_kind=cron 时的表达式';
COMMENT ON COLUMN automation_rules.interval_seconds IS 'schedule_kind=interval 时的间隔秒数';
COMMENT ON COLUMN automation_rules.timezone IS '日程时区，默认 Asia/Shanghai';
COMMENT ON COLUMN automation_rules.overlap_policy IS '重叠策略；P1 固定 skip（上一次未终态则跳过）';
COMMENT ON COLUMN automation_rules.actor_user_id IS '名义发起人，固定为规则创建者';
COMMENT ON COLUMN automation_rules.disabled_reason IS '系统或用户禁用原因码；启用中为 NULL';
COMMENT ON COLUMN automation_rules.consecutive_failure_count IS '连续发起失败次数；成功归零；达 3 自动禁用';
COMMENT ON COLUMN automation_rules.temporal_schedule_id IS 'Temporal Schedule ID，与规则对账';

COMMENT ON TABLE automation_fires IS '自动化规则每次触发审计行；幂等键 rule_id+scheduled_fire_at';
COMMENT ON COLUMN automation_fires.tenant_id IS '租户 ID';
COMMENT ON COLUMN automation_fires.rule_id IS '所属规则 ID';
COMMENT ON COLUMN automation_fires.scheduled_fire_at IS '计划触发时刻（调度日历时间）';
COMMENT ON COLUMN automation_fires.idempotency_key IS '全局唯一幂等键';
COMMENT ON COLUMN automation_fires.status IS 'pending | succeeded | failed | skipped_overlap | skipped_disabled';
COMMENT ON COLUMN automation_fires.demand_id IS 'plan/loop 成功时写入的 demand ID';
COMMENT ON COLUMN automation_fires.run_id IS 'chat 成功时写入的 run ID';
COMMENT ON COLUMN automation_fires.error_code IS '失败/跳过机器可读码';
COMMENT ON COLUMN automation_fires.error_message IS '失败/跳过可读摘要';
