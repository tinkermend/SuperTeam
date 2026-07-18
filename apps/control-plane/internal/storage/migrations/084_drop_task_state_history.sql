-- 079: Drop task_state_history.
--
-- The table never worked as an audit trail: its only writer sat behind a
-- swallowed-error path in task.Service (errors ignored since 001), no API,
-- contract, or UI ever read it, and the live task activity trail is
-- task_events. Removing the table together with its write path instead of
-- fixing a writer nothing consumes.

DROP TABLE IF EXISTS task_state_history;
