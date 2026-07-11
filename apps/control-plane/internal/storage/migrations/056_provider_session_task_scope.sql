-- Provider session is scoped to (employee, task lineage root), not just employee.
-- A revision task B' shares its root (B) with the original, so it resumes B's
-- session. A different task D under the same employee gets a fresh session. See
-- the 2026-07-10 plan-phase refactor spec §4.9.
ALTER TABLE provider_sessions
    ADD COLUMN project_task_root_id UUID;

CREATE INDEX idx_provider_sessions_task_root
    ON provider_sessions (tenant_id, digital_employee_id, project_task_root_id);

COMMENT ON COLUMN provider_sessions.project_task_root_id IS 'Task lineage root this session is scoped to; null for pre-refactor employee-level sessions.';
