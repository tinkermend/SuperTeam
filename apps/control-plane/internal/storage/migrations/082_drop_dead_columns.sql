-- 082: Drop columns that never gained a write path.
--
-- Each column below was declared with terminal/audit semantics that no code
-- ever implemented: no INSERT/UPDATE anywhere names them, live data is 100%
-- NULL, and read sides only surfaced permanently-nil struct fields with no
-- branching logic attached.
--
-- Deliberately NOT touched: project_task_attempts.lease_expires_at (live
-- attempt-lease reaping), runtime enrollment/session revoked_at (live),
-- execution_ledger_events columns (written via create_execution_ledger_event),
-- and the policy knobs project_tasks.max_attempts /
-- project_task_attempts.budget_wall_clock_limit_sec — those anchor real
-- retry-cap and budget-trip logic that only lacks a writer today.

ALTER TABLE auth_users DROP COLUMN last_login_at;
ALTER TABLE auth_sessions DROP COLUMN revoked_at;
ALTER TABLE task_runs DROP COLUMN lease_expires_at;
ALTER TABLE project_tasks
    DROP COLUMN terminal_reason,
    DROP COLUMN cancelled_by,
    DROP COLUMN failed_by;
ALTER TABLE project_task_attempts
    DROP COLUMN lost_at,
    DROP COLUMN timeout_at;
ALTER TABLE digital_employee_environment_variables DROP COLUMN metadata;
