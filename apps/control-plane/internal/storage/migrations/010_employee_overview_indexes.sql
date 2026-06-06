CREATE INDEX IF NOT EXISTS idx_task_runs_latest_per_employee
ON task_runs(tenant_id, digital_employee_id, updated_at DESC, created_at DESC)
WHERE digital_employee_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_task_runs_budget_30d
ON task_runs(tenant_id, created_at DESC, digital_employee_id)
WHERE digital_employee_id IS NOT NULL;
