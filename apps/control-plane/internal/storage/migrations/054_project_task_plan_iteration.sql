-- Graph-level extension counter. 0 = original plan; N = appended in the Nth
-- extension round. Per-task rework (revision of the same task) is tracked via
-- RevisionOfTaskID lineage; this counts whole-graph extension rounds, which
-- lineage cannot. See the 2026-07-10 plan-phase refactor spec §4.8.
ALTER TABLE project_tasks
    ADD COLUMN plan_iteration INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN project_tasks.plan_iteration IS 'Graph extension round: 0 for the original plan, N for tasks appended in the Nth round.';
