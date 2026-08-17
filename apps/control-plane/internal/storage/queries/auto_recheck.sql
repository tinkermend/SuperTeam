-- name: ListPendingRecoveryDecisionsForAutoRecheck :many
-- Autonomy F7: pending recovery-family decisions that auto_recheck may probe
-- and release without a human when the underlying fact has healed.
SELECT d.*
FROM project_decision_requests d
WHERE lower(btrim(d.status_snapshot)) IN ('pending', 'requested')
  AND d.decision_type IN (
    'project_task_runtime_recovery',
    'project_task_recovery',
    'project_task_budget_approval'
  )
ORDER BY d.created_at ASC, d.id ASC
LIMIT sqlc.arg('batch_limit')::integer;
