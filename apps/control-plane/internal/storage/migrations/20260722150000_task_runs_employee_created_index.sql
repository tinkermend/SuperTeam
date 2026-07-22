-- 员工日历看板按 created_at 窗口扫描;现有 idx_task_runs_employee_status 中间夹 status,
-- 无状态过滤时用本索引更贴。
CREATE INDEX IF NOT EXISTS idx_task_runs_employee_created
    ON task_runs (tenant_id, digital_employee_id, created_at DESC)
    WHERE digital_employee_id IS NOT NULL;
