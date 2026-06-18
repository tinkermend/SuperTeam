-- 数字员工运营状态读模型索引
-- 支撑 overview operational facts 按租户、数字员工 assignee、任务状态和待决决策状态聚合。
-- 保持 tenant-first，不新增跨模块外键。

CREATE INDEX idx_project_tasks_tenant_assignee_status
    ON project_tasks(tenant_id, assigned_digital_employee_id, status)
    WHERE assigned_digital_employee_id IS NOT NULL;

CREATE INDEX idx_project_decision_requests_tenant_task_status_type
    ON project_decision_requests(tenant_id, project_task_id, status_snapshot, decision_type)
    WHERE project_task_id IS NOT NULL;
