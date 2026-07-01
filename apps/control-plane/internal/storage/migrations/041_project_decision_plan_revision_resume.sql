ALTER TABLE project_decision_requests
    ADD COLUMN plan_revision_id UUID;

CREATE INDEX idx_project_decision_requests_plan_revision
    ON project_decision_requests(tenant_id, project_id, plan_revision_id)
    WHERE plan_revision_id IS NOT NULL;

COMMENT ON COLUMN project_decision_requests.plan_revision_id IS '该人类决策关联的计划版本ID，用于 ProjectCoordinator Continue-As-New 后恢复 plan_review 路由。';
