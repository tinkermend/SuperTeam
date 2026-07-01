\echo 'This script clears SuperTeam development project coordination data.'
\echo 'It preserves auth_users, auth_sessions, web_login_logs, user_project_team_scopes, tenants, tenant_teams, and login/account data.'
\echo 'Run only against the confirmed development database.'

BEGIN;

SELECT 'project_task_attestations' AS table_name, count(*) FROM project_task_attestations
UNION ALL SELECT 'project_placements', count(*) FROM project_placements
UNION ALL SELECT 'project_task_results', count(*) FROM project_task_results
UNION ALL SELECT 'project_demand_summaries', count(*) FROM project_demand_summaries
UNION ALL SELECT 'project_task_dispatch_gate_results', count(*) FROM project_task_dispatch_gate_results
UNION ALL SELECT 'project_task_attempt_context_updates', count(*) FROM project_task_attempt_context_updates
UNION ALL SELECT 'project_task_attempts', count(*) FROM project_task_attempts
UNION ALL SELECT 'project_plan_decomposition_claims', count(*) FROM project_plan_decomposition_claims
UNION ALL SELECT 'project_plan_revisions', count(*) FROM project_plan_revisions
UNION ALL SELECT 'project_decision_requests', count(*) FROM project_decision_requests
UNION ALL SELECT 'project_transfer_requests', count(*) FROM project_transfer_requests
UNION ALL SELECT 'project_execution_summaries', count(*) FROM project_execution_summaries
UNION ALL SELECT 'execution_ledger_events', count(*) FROM execution_ledger_events
UNION ALL SELECT 'project_route_decisions', count(*) FROM project_route_decisions
UNION ALL SELECT 'project_coordination_jobs', count(*) FROM project_coordination_jobs
UNION ALL SELECT 'project_acceptance_records', count(*) FROM project_acceptance_records
UNION ALL SELECT 'project_archive_snapshots', count(*) FROM project_archive_snapshots
UNION ALL SELECT 'project_budget_ledger', count(*) FROM project_budget_ledger
UNION ALL SELECT 'project_report_refs', count(*) FROM project_report_refs
UNION ALL SELECT 'project_artifact_refs', count(*) FROM project_artifact_refs
UNION ALL SELECT 'project_evidence_refs', count(*) FROM project_evidence_refs
UNION ALL SELECT 'project_task_dependencies', count(*) FROM project_task_dependencies
UNION ALL SELECT 'team_lending_request', count(*) FROM team_lending_request
UNION ALL SELECT 'project_events', count(*) FROM project_events
UNION ALL SELECT 'project_demands', count(*) FROM project_demands
UNION ALL SELECT 'project_members', count(*) FROM project_members
UNION ALL SELECT 'project_config_revisions', count(*) FROM project_config_revisions
UNION ALL SELECT 'project_tasks', count(*) FROM project_tasks
UNION ALL SELECT 'projects', count(*) FROM projects
UNION ALL SELECT 'project approvals', count(*) FROM approval_requests WHERE requester_type = 'project_coordinator'
UNION ALL SELECT 'project inbox items', count(*) FROM inbox_items WHERE source_project_id IS NOT NULL;

DELETE FROM inbox_items WHERE source_project_id IS NOT NULL;
DELETE FROM approval_decisions
WHERE approval_request_id IN (
    SELECT id FROM approval_requests WHERE requester_type = 'project_coordinator'
);
DELETE FROM approval_requests WHERE requester_type = 'project_coordinator';

TRUNCATE TABLE
    project_task_attestations,
    project_placements,
    project_task_results,
    project_demand_summaries,
    project_task_dispatch_gate_results,
    project_task_attempt_context_updates,
    project_task_attempts,
    project_plan_decomposition_claims,
    project_plan_revisions,
    project_decision_requests,
    project_transfer_requests,
    project_execution_summaries,
    execution_ledger_events,
    project_route_decisions,
    project_coordination_jobs,
    project_acceptance_records,
    project_archive_snapshots,
    project_budget_ledger,
    project_report_refs,
    project_artifact_refs,
    project_evidence_refs,
    project_task_dependencies,
    team_lending_request,
    project_events,
    project_demands,
    project_members,
    project_config_revisions,
    project_tasks,
    projects;

SELECT 'auth_users preserved' AS check_name, count(*) FROM auth_users
UNION ALL SELECT 'auth_sessions preserved', count(*) FROM auth_sessions
UNION ALL SELECT 'web_login_logs preserved', count(*) FROM web_login_logs
UNION ALL SELECT 'user_project_team_scopes preserved', count(*) FROM user_project_team_scopes
UNION ALL SELECT 'tenants preserved', count(*) FROM tenants
UNION ALL SELECT 'tenant_teams preserved', count(*) FROM tenant_teams;

COMMIT;
