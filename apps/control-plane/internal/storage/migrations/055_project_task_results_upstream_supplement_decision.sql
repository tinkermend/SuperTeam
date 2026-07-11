-- The blocked_resolvable_upstream TaskResultDecision (task_result_contract.go)
-- shipped in the upstream-supplement graph extension, but this table's decision
-- check constraint (034_project_task_results.sql) was never updated to allow it.
-- Every real SubmitProjectTaskAttemptResult call for a resolvable-upstream block
-- fails at the database layer until this value is added. See the 2026-07-10
-- plan-phase refactor spec §4.6(a).
ALTER TABLE project_task_results
    DROP CONSTRAINT chk_project_task_results_decision;

ALTER TABLE project_task_results
    ADD CONSTRAINT chk_project_task_results_decision CHECK (
        decision IN (
            'validation_failed',
            'complete_accepted',
            'waiting_human_review',
            'revision_attempt',
            'revision_task',
            'blocked_waiting_human',
            'blocked_resolvable_upstream',
            'failed_retryable',
            'failed_recovery',
            'cancelled_terminal',
            'replan_requested'
        )
    );
