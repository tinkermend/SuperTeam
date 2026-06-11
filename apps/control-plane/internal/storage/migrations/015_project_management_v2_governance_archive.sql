-- 015_project_management_v2_governance_archive.sql
-- 项目管理 V2：证据、工件、报告、预算、验收、归档快照和保留锁

CREATE TABLE project_evidence_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID,
    route_decision_id UUID,
    execution_summary_id UUID,
    evidence_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    source_type VARCHAR(100) NOT NULL,
    source_ref TEXT,
    artifact_ref_id UUID,
    submitted_by_type VARCHAR(50) NOT NULL,
    submitted_by_id UUID,
    verification_status VARCHAR(50) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_evidence_refs IS '项目证据引用表，保存任务、路由和执行摘要产出的可审计证据';
COMMENT ON COLUMN project_evidence_refs.id IS '项目证据引用ID';
COMMENT ON COLUMN project_evidence_refs.tenant_id IS '租户ID';
COMMENT ON COLUMN project_evidence_refs.project_id IS '所属项目ID';
COMMENT ON COLUMN project_evidence_refs.project_task_id IS '关联项目任务ID，可为空表示项目级证据';
COMMENT ON COLUMN project_evidence_refs.route_decision_id IS '关联路由决策ID';
COMMENT ON COLUMN project_evidence_refs.execution_summary_id IS '关联执行摘要ID';
COMMENT ON COLUMN project_evidence_refs.evidence_type IS '证据类型，由应用层注册和校验';
COMMENT ON COLUMN project_evidence_refs.title IS '证据标题快照';
COMMENT ON COLUMN project_evidence_refs.summary IS '证据摘要';
COMMENT ON COLUMN project_evidence_refs.source_type IS '证据来源类型，例如 artifact、report、manual、external';
COMMENT ON COLUMN project_evidence_refs.source_ref IS '证据来源引用，保存外部ID、对象地址或结构化引用摘要';
COMMENT ON COLUMN project_evidence_refs.artifact_ref_id IS '关联项目工件引用ID';
COMMENT ON COLUMN project_evidence_refs.submitted_by_type IS '提交者类型，例如 human_user、digital_employee、system';
COMMENT ON COLUMN project_evidence_refs.submitted_by_id IS '提交者ID，可为空表示系统提交';
COMMENT ON COLUMN project_evidence_refs.verification_status IS '证据核验状态，由应用层注册和校验';
COMMENT ON COLUMN project_evidence_refs.metadata IS '证据扩展元数据';
COMMENT ON COLUMN project_evidence_refs.created_event_id IS '创建该证据引用时产生的项目事件ID';
COMMENT ON COLUMN project_evidence_refs.created_at IS '证据引用创建时间';

CREATE INDEX idx_project_evidence_refs_tenant_project_created ON project_evidence_refs(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_evidence_refs_tenant_task ON project_evidence_refs(tenant_id, project_task_id);
CREATE INDEX idx_project_evidence_refs_tenant_execution_summary ON project_evidence_refs(tenant_id, execution_summary_id);
CREATE INDEX idx_project_evidence_refs_tenant_status ON project_evidence_refs(tenant_id, project_id, verification_status);

CREATE TABLE project_artifact_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID,
    artifact_id UUID,
    artifact_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    object_ref TEXT NOT NULL,
    content_type VARCHAR(255),
    size_bytes BIGINT,
    checksum VARCHAR(255),
    retention_status VARCHAR(50) NOT NULL DEFAULT 'unheld',
    retention_hold_id UUID,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_artifact_refs IS '项目工件引用表，保存项目内报告、附件、日志和执行产物的对象引用';
COMMENT ON COLUMN project_artifact_refs.id IS '项目工件引用ID';
COMMENT ON COLUMN project_artifact_refs.tenant_id IS '租户ID';
COMMENT ON COLUMN project_artifact_refs.project_id IS '所属项目ID';
COMMENT ON COLUMN project_artifact_refs.project_task_id IS '关联项目任务ID，可为空表示项目级工件';
COMMENT ON COLUMN project_artifact_refs.artifact_id IS '平台工件ID，可为空表示外部对象或尚未入库的对象';
COMMENT ON COLUMN project_artifact_refs.artifact_type IS '工件类型，由应用层注册和校验';
COMMENT ON COLUMN project_artifact_refs.title IS '工件标题快照';
COMMENT ON COLUMN project_artifact_refs.object_ref IS '对象存储引用或外部对象引用';
COMMENT ON COLUMN project_artifact_refs.content_type IS '工件内容类型';
COMMENT ON COLUMN project_artifact_refs.size_bytes IS '工件大小字节数';
COMMENT ON COLUMN project_artifact_refs.checksum IS '工件校验和';
COMMENT ON COLUMN project_artifact_refs.retention_status IS '保留状态，例如 unheld、held、released';
COMMENT ON COLUMN project_artifact_refs.retention_hold_id IS '当前关联的保留锁ID';
COMMENT ON COLUMN project_artifact_refs.metadata IS '工件扩展元数据';
COMMENT ON COLUMN project_artifact_refs.created_event_id IS '创建该工件引用时产生的项目事件ID';
COMMENT ON COLUMN project_artifact_refs.created_at IS '工件引用创建时间';

CREATE INDEX idx_project_artifact_refs_tenant_project_created ON project_artifact_refs(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_artifact_refs_tenant_task ON project_artifact_refs(tenant_id, project_task_id);
CREATE INDEX idx_project_artifact_refs_tenant_artifact ON project_artifact_refs(tenant_id, artifact_id);

CREATE TABLE project_report_refs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    report_type VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    summary TEXT,
    object_ref TEXT NOT NULL,
    format VARCHAR(100) NOT NULL,
    generated_by_type VARCHAR(50) NOT NULL,
    generated_by_id UUID,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_report_refs IS '项目报告引用表，保存阶段汇报、验收报告和归档报告的对象引用';
COMMENT ON COLUMN project_report_refs.id IS '项目报告引用ID';
COMMENT ON COLUMN project_report_refs.tenant_id IS '租户ID';
COMMENT ON COLUMN project_report_refs.project_id IS '所属项目ID';
COMMENT ON COLUMN project_report_refs.report_type IS '报告类型，由应用层注册和校验';
COMMENT ON COLUMN project_report_refs.title IS '报告标题快照';
COMMENT ON COLUMN project_report_refs.summary IS '报告摘要';
COMMENT ON COLUMN project_report_refs.object_ref IS '报告对象存储引用或外部对象引用';
COMMENT ON COLUMN project_report_refs.format IS '报告格式，例如 markdown、pdf、html、json';
COMMENT ON COLUMN project_report_refs.generated_by_type IS '生成者类型，例如 human_user、digital_employee、system';
COMMENT ON COLUMN project_report_refs.generated_by_id IS '生成者ID，可为空表示系统生成';
COMMENT ON COLUMN project_report_refs.created_event_id IS '创建该报告引用时产生的项目事件ID';
COMMENT ON COLUMN project_report_refs.created_at IS '报告引用创建时间';

CREATE INDEX idx_project_report_refs_tenant_project_created ON project_report_refs(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_report_refs_tenant_type_created ON project_report_refs(tenant_id, project_id, report_type, created_at DESC);

CREATE TABLE project_budget_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    coordination_job_id UUID,
    project_task_id UUID,
    digital_employee_id UUID,
    cost_type VARCHAR(100) NOT NULL,
    estimated_tokens BIGINT,
    actual_tokens BIGINT,
    estimated_cost NUMERIC(18,6),
    actual_cost NUMERIC(18,6),
    source VARCHAR(100) NOT NULL,
    reason TEXT,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_budget_ledger IS '项目预算流水表，记录协调、执行和外部能力调用的预算估算与实际消耗';
COMMENT ON COLUMN project_budget_ledger.id IS '项目预算流水ID';
COMMENT ON COLUMN project_budget_ledger.tenant_id IS '租户ID';
COMMENT ON COLUMN project_budget_ledger.project_id IS '所属项目ID';
COMMENT ON COLUMN project_budget_ledger.coordination_job_id IS '关联协调作业ID';
COMMENT ON COLUMN project_budget_ledger.project_task_id IS '关联项目任务ID';
COMMENT ON COLUMN project_budget_ledger.digital_employee_id IS '关联数字员工ID';
COMMENT ON COLUMN project_budget_ledger.cost_type IS '成本类型，由应用层注册和校验';
COMMENT ON COLUMN project_budget_ledger.estimated_tokens IS '预估 Token 数';
COMMENT ON COLUMN project_budget_ledger.actual_tokens IS '实际 Token 数';
COMMENT ON COLUMN project_budget_ledger.estimated_cost IS '预估费用';
COMMENT ON COLUMN project_budget_ledger.actual_cost IS '实际费用';
COMMENT ON COLUMN project_budget_ledger.source IS '预算流水来源，例如 coordinator、runtime、provider、capability';
COMMENT ON COLUMN project_budget_ledger.reason IS '记录该预算流水的原因';
COMMENT ON COLUMN project_budget_ledger.created_event_id IS '创建该预算流水时产生的项目事件ID';
COMMENT ON COLUMN project_budget_ledger.created_at IS '预算流水创建时间';

CREATE INDEX idx_project_budget_ledger_tenant_project_created ON project_budget_ledger(tenant_id, project_id, created_at DESC);
CREATE INDEX idx_project_budget_ledger_tenant_task ON project_budget_ledger(tenant_id, project_task_id);
CREATE INDEX idx_project_budget_ledger_tenant_employee ON project_budget_ledger(tenant_id, digital_employee_id, created_at DESC);
CREATE INDEX idx_project_budget_ledger_tenant_cost_type ON project_budget_ledger(tenant_id, project_id, cost_type);

CREATE TABLE project_acceptance_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    accepted_by_user_id UUID NOT NULL,
    status VARCHAR(50) NOT NULL,
    conclusion TEXT NOT NULL,
    summary TEXT,
    evidence_ref_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    report_ref_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    unresolved_risks JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_acceptance_records IS '项目验收记录表，保存人类验收结论、证据引用和未解决风险';
COMMENT ON COLUMN project_acceptance_records.id IS '项目验收记录ID';
COMMENT ON COLUMN project_acceptance_records.tenant_id IS '租户ID';
COMMENT ON COLUMN project_acceptance_records.project_id IS '所属项目ID';
COMMENT ON COLUMN project_acceptance_records.accepted_by_user_id IS '做出验收结论的人类用户ID';
COMMENT ON COLUMN project_acceptance_records.status IS '验收状态，由应用层注册和校验';
COMMENT ON COLUMN project_acceptance_records.conclusion IS '验收结论';
COMMENT ON COLUMN project_acceptance_records.summary IS '验收摘要';
COMMENT ON COLUMN project_acceptance_records.evidence_ref_ids IS '验收使用的证据引用ID数组';
COMMENT ON COLUMN project_acceptance_records.report_ref_ids IS '验收使用的报告引用ID数组';
COMMENT ON COLUMN project_acceptance_records.unresolved_risks IS '验收时仍未解决的风险数组';
COMMENT ON COLUMN project_acceptance_records.created_event_id IS '创建该验收记录时产生的项目事件ID';
COMMENT ON COLUMN project_acceptance_records.created_at IS '验收记录创建时间';

CREATE INDEX idx_project_acceptance_records_tenant_project_created ON project_acceptance_records(tenant_id, project_id, created_at DESC);

CREATE TABLE project_archive_snapshots (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    snapshot_type VARCHAR(100) NOT NULL,
    status VARCHAR(50) NOT NULL,
    object_ref TEXT NOT NULL,
    summary TEXT,
    included_counts JSONB NOT NULL DEFAULT '{}'::jsonb,
    retained_artifact_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    retention_lock_event_id UUID,
    created_by_user_id UUID,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_archive_snapshots IS '项目归档快照表，保存项目证据、报告和工件归档结果';
COMMENT ON COLUMN project_archive_snapshots.id IS '项目归档快照ID';
COMMENT ON COLUMN project_archive_snapshots.tenant_id IS '租户ID';
COMMENT ON COLUMN project_archive_snapshots.project_id IS '所属项目ID';
COMMENT ON COLUMN project_archive_snapshots.snapshot_type IS '归档快照类型，由应用层注册和校验';
COMMENT ON COLUMN project_archive_snapshots.status IS '归档快照状态，由应用层注册和校验';
COMMENT ON COLUMN project_archive_snapshots.object_ref IS '归档快照对象存储引用或外部对象引用';
COMMENT ON COLUMN project_archive_snapshots.summary IS '归档摘要';
COMMENT ON COLUMN project_archive_snapshots.included_counts IS '归档包含对象数量统计';
COMMENT ON COLUMN project_archive_snapshots.retained_artifact_ids IS '归档时被保留锁覆盖的工件ID数组';
COMMENT ON COLUMN project_archive_snapshots.retention_lock_event_id IS '创建归档保留锁时产生的项目事件ID';
COMMENT ON COLUMN project_archive_snapshots.created_by_user_id IS '创建归档快照的人类用户ID';
COMMENT ON COLUMN project_archive_snapshots.created_event_id IS '创建该归档快照时产生的项目事件ID';
COMMENT ON COLUMN project_archive_snapshots.created_at IS '归档快照创建时间';

CREATE INDEX idx_project_archive_snapshots_tenant_project_created ON project_archive_snapshots(tenant_id, project_id, created_at DESC);

CREATE TABLE artifact_retention_holds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    artifact_id UUID NOT NULL,
    hold_type VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,
    reason TEXT,
    status VARCHAR(50) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    released_at TIMESTAMPTZ
);

COMMENT ON TABLE artifact_retention_holds IS '工件保留锁表，记录归档、审计和人工决策产生的工件保留要求';
COMMENT ON COLUMN artifact_retention_holds.id IS '工件保留锁ID';
COMMENT ON COLUMN artifact_retention_holds.tenant_id IS '租户ID';
COMMENT ON COLUMN artifact_retention_holds.artifact_id IS '被保留的工件ID';
COMMENT ON COLUMN artifact_retention_holds.hold_type IS '保留锁类型，由应用层注册和校验';
COMMENT ON COLUMN artifact_retention_holds.resource_type IS '触发保留锁的资源类型';
COMMENT ON COLUMN artifact_retention_holds.resource_id IS '触发保留锁的资源ID';
COMMENT ON COLUMN artifact_retention_holds.reason IS '保留原因';
COMMENT ON COLUMN artifact_retention_holds.status IS '保留锁状态，例如 active、released';
COMMENT ON COLUMN artifact_retention_holds.created_event_id IS '创建该保留锁时产生的项目事件ID';
COMMENT ON COLUMN artifact_retention_holds.created_at IS '保留锁创建时间';
COMMENT ON COLUMN artifact_retention_holds.released_at IS '保留锁释放时间';

CREATE INDEX idx_artifact_retention_holds_tenant_artifact_active ON artifact_retention_holds(tenant_id, artifact_id) WHERE released_at IS NULL AND status = 'active';
CREATE INDEX idx_artifact_retention_holds_tenant_resource ON artifact_retention_holds(tenant_id, resource_type, resource_id, created_at DESC);

ALTER TABLE project_config_revisions
    ADD COLUMN changed_sections JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN previous_revision_id UUID,
    ADD COLUMN policy_fingerprint VARCHAR(128),
    ADD COLUMN diff_summary JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN project_config_revisions.changed_sections IS '配置修订涉及的配置分区数组';
COMMENT ON COLUMN project_config_revisions.previous_revision_id IS '上一版项目配置修订ID';
COMMENT ON COLUMN project_config_revisions.policy_fingerprint IS '配置策略指纹，用于识别策略内容是否变化';
COMMENT ON COLUMN project_config_revisions.diff_summary IS '配置变更差异摘要';
