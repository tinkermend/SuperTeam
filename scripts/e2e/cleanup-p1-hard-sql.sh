#!/usr/bin/env bash
# Hard-delete all demands/tasks/inbox/events for P1 e2e project (history junk).
# Keeps project row, members, castings.
# Usage:
#   DATABASE_URL=postgres://... PROJECT_ID=ca82b054-... ./scripts/e2e/cleanup-p1-hard-sql.sh
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DATABASE_URL="${DATABASE_URL:-$(python3 -c "import re; t=open('$ROOT/apps/control-plane/config/config.yaml').read(); m=re.search(r'url:\\s*\\\"([^\\\"]+)\\\"', t); print(m.group(1) if m else '')")}"
PID="${PROJECT_ID:-ca82b054-de2d-4810-9a2b-dd41f5e50a2c}"

psql "$DATABASE_URL" -v ON_ERROR_STOP=1 <<SQL
BEGIN;
SET search_path TO superteam, public;
CREATE TEMP TABLE _p1_demands AS SELECT id FROM project_demands WHERE project_id = '$PID'::uuid;
CREATE TEMP TABLE _p1_tasks AS SELECT id FROM project_tasks WHERE project_id = '$PID'::uuid;
CREATE TEMP TABLE _p1_approvals AS
SELECT DISTINCT id FROM (
  SELECT id FROM approval_requests
  WHERE resource_type = 'project' AND resource_id = '$PID'::uuid
  UNION
  SELECT source_approval_request_id FROM inbox_items
  WHERE source_project_id = '$PID'::uuid AND source_approval_request_id IS NOT NULL
  UNION
  SELECT approval_request_id FROM project_decision_requests WHERE project_id = '$PID'::uuid
) x WHERE id IS NOT NULL;

UPDATE project_tasks SET
  current_attempt_id = NULL,
  latest_dispatch_gate_result_id = NULL,
  latest_task_result_id = NULL,
  revision_of_task_id = NULL
WHERE project_id = '$PID'::uuid;
UPDATE project_task_attempts SET dispatch_gate_result_id = NULL
WHERE project_task_id IN (SELECT id FROM _p1_tasks);
UPDATE provider_sessions SET project_task_root_id = NULL
WHERE project_task_root_id IN (SELECT id FROM _p1_tasks);

DELETE FROM inbox_items WHERE source_project_id = '$PID'::uuid;
DELETE FROM project_decision_requests WHERE project_id = '$PID'::uuid;
DELETE FROM approval_decisions WHERE approval_request_id IN (SELECT id FROM _p1_approvals);
DELETE FROM approval_requests WHERE id IN (SELECT id FROM _p1_approvals);
DELETE FROM demand_criterion_verdicts WHERE demand_id IN (SELECT id FROM _p1_demands);
DELETE FROM demand_adversarial_judgements WHERE demand_id IN (SELECT id FROM _p1_demands);
DELETE FROM demand_acceptance_criteria WHERE demand_id IN (SELECT id FROM _p1_demands);
DELETE FROM project_demand_constraint_exemptions WHERE demand_id IN (SELECT id FROM _p1_demands);
DELETE FROM project_demand_summaries WHERE demand_id IN (SELECT id FROM _p1_demands);
DELETE FROM project_task_results WHERE project_task_id IN (SELECT id FROM _p1_tasks);
DELETE FROM project_task_dispatch_gate_results WHERE project_id = '$PID'::uuid;
DELETE FROM project_task_attestations WHERE project_task_id IN (SELECT id FROM _p1_tasks);
DELETE FROM project_task_attempt_context_updates WHERE project_task_id IN (SELECT id FROM _p1_tasks);
DELETE FROM project_task_attempts WHERE project_task_id IN (SELECT id FROM _p1_tasks);
DELETE FROM project_task_dependencies WHERE coordination_job_id IN (
  SELECT id FROM project_coordination_jobs WHERE project_id = '$PID'::uuid
);
DELETE FROM project_tasks WHERE project_id = '$PID'::uuid;
DELETE FROM project_plan_decomposition_claims WHERE demand_id IN (SELECT id FROM _p1_demands);
UPDATE project_plan_revisions SET superseded_by_revision_id = NULL WHERE project_id = '$PID'::uuid;
DELETE FROM project_plan_revisions WHERE project_id = '$PID'::uuid;
DELETE FROM project_evidence_refs WHERE project_id = '$PID'::uuid;
DELETE FROM project_artifact_refs WHERE project_id = '$PID'::uuid;
DELETE FROM project_report_refs WHERE project_id = '$PID'::uuid;
DELETE FROM project_execution_summaries WHERE project_id = '$PID'::uuid;
DELETE FROM project_route_decisions WHERE project_id = '$PID'::uuid;
DELETE FROM project_budget_ledger WHERE project_id = '$PID'::uuid;
DELETE FROM project_coordination_jobs WHERE project_id = '$PID'::uuid;
DELETE FROM project_events WHERE project_id = '$PID'::uuid;
DELETE FROM automation_fires WHERE demand_id IN (SELECT id FROM _p1_demands);
DELETE FROM project_acceptance_records WHERE project_id = '$PID'::uuid;
UPDATE project_demands SET continues_demand_id = NULL WHERE project_id = '$PID'::uuid;
DELETE FROM project_demands WHERE project_id = '$PID'::uuid;
COMMIT;
SQL
echo "P1 hard cleanup done for $PID"
