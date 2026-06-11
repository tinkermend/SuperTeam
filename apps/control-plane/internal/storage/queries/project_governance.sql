-- name: CreateProjectEvidenceRef :one
INSERT INTO project_evidence_refs (
    tenant_id,
    project_id,
    project_task_id,
    route_decision_id,
    execution_summary_id,
    evidence_type,
    title,
    summary,
    source_type,
    source_ref,
    artifact_ref_id,
    submitted_by_type,
    submitted_by_id,
    verification_status,
    metadata,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.narg('route_decision_id')::uuid,
    sqlc.narg('execution_summary_id')::uuid,
    sqlc.arg('evidence_type')::varchar,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.arg('source_type')::varchar,
    sqlc.arg('source_ref')::text,
    sqlc.narg('artifact_ref_id')::uuid,
    sqlc.arg('submitted_by_type')::varchar,
    sqlc.narg('submitted_by_id')::uuid,
    sqlc.arg('verification_status')::varchar,
    COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectEvidenceRefs :many
SELECT * FROM project_evidence_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('verification_status')::varchar IS NULL OR verification_status = sqlc.narg('verification_status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProjectEvidenceVerificationStatus :one
UPDATE project_evidence_refs
SET verification_status = sqlc.arg('verification_status')::varchar,
    metadata = COALESCE(sqlc.narg('metadata')::jsonb, metadata)
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: CreateProjectArtifactRef :one
INSERT INTO project_artifact_refs (
    tenant_id,
    project_id,
    project_task_id,
    artifact_id,
    artifact_type,
    title,
    object_ref,
    content_type,
    size_bytes,
    checksum,
    retention_status,
    retention_hold_id,
    metadata,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.narg('artifact_id')::uuid,
    sqlc.arg('artifact_type')::varchar,
    sqlc.arg('title')::varchar,
    sqlc.arg('object_ref')::text,
    sqlc.narg('content_type')::varchar,
    sqlc.narg('size_bytes')::bigint,
    sqlc.narg('checksum')::varchar,
    COALESCE(sqlc.narg('retention_status')::varchar, 'unheld'::varchar),
    sqlc.narg('retention_hold_id')::uuid,
    COALESCE(sqlc.narg('metadata')::jsonb, '{}'::jsonb),
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectArtifactRefs :many
SELECT * FROM project_artifact_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('artifact_type')::varchar IS NULL OR artifact_type = sqlc.narg('artifact_type')::varchar)
  AND (sqlc.narg('retention_status')::varchar IS NULL OR retention_status = sqlc.narg('retention_status')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: UpdateProjectArtifactRetention :one
UPDATE project_artifact_refs
SET retention_status = sqlc.arg('retention_status')::varchar,
    retention_hold_id = sqlc.narg('retention_hold_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: CreateProjectReportRef :one
INSERT INTO project_report_refs (
    tenant_id,
    project_id,
    report_type,
    title,
    summary,
    object_ref,
    format,
    generated_by_type,
    generated_by_id,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('report_type')::varchar,
    sqlc.arg('title')::varchar,
    sqlc.narg('summary')::text,
    sqlc.arg('object_ref')::text,
    sqlc.arg('format')::varchar,
    sqlc.arg('generated_by_type')::varchar,
    sqlc.narg('generated_by_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectReportRefs :many
SELECT * FROM project_report_refs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('report_type')::varchar IS NULL OR report_type = sqlc.narg('report_type')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectBudgetLedgerEntry :one
INSERT INTO project_budget_ledger (
    tenant_id,
    project_id,
    coordination_job_id,
    project_task_id,
    digital_employee_id,
    cost_type,
    estimated_tokens,
    actual_tokens,
    estimated_cost,
    actual_cost,
    source,
    reason,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.narg('digital_employee_id')::uuid,
    sqlc.arg('cost_type')::varchar,
    sqlc.narg('estimated_tokens')::bigint,
    sqlc.narg('actual_tokens')::bigint,
    sqlc.narg('estimated_cost')::numeric,
    sqlc.narg('actual_cost')::numeric,
    sqlc.arg('source')::varchar,
    sqlc.narg('reason')::text,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectBudgetLedger :many
SELECT * FROM project_budget_ledger
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('cost_type')::varchar IS NULL OR cost_type = sqlc.narg('cost_type')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetProjectBudgetSummary :one
SELECT
    COALESCE(SUM(estimated_tokens), 0)::bigint AS estimated_tokens,
    COALESCE(SUM(actual_tokens), 0)::bigint AS actual_tokens,
    COALESCE(SUM(estimated_cost), 0)::numeric AS estimated_cost,
    COALESCE(SUM(actual_cost), 0)::numeric AS actual_cost,
    COUNT(*)::integer AS ledger_count
FROM project_budget_ledger
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid;

-- name: CreateProjectAcceptanceRecord :one
INSERT INTO project_acceptance_records (
    tenant_id,
    project_id,
    accepted_by_user_id,
    status,
    conclusion,
    summary,
    evidence_ref_ids,
    report_ref_ids,
    unresolved_risks,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('accepted_by_user_id')::uuid,
    sqlc.arg('status')::varchar,
    sqlc.arg('conclusion')::text,
    sqlc.narg('summary')::text,
    COALESCE(sqlc.narg('evidence_ref_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('report_ref_ids')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('unresolved_risks')::jsonb, '[]'::jsonb),
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: GetLatestProjectAcceptanceRecord :one
SELECT * FROM project_acceptance_records
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY created_at DESC
LIMIT 1;

-- name: CreateProjectArchiveSnapshot :one
INSERT INTO project_archive_snapshots (
    tenant_id,
    project_id,
    snapshot_type,
    status,
    object_ref,
    summary,
    included_counts,
    retained_artifact_ids,
    retention_lock_event_id,
    created_by_user_id,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('snapshot_type')::varchar,
    sqlc.arg('status')::varchar,
    sqlc.narg('object_ref')::text,
    sqlc.narg('summary')::text,
    COALESCE(sqlc.narg('included_counts')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('retained_artifact_ids')::jsonb, '[]'::jsonb),
    sqlc.narg('retention_lock_event_id')::uuid,
    sqlc.arg('created_by_user_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectArchiveSnapshots :many
SELECT * FROM project_archive_snapshots
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND (sqlc.narg('snapshot_type')::varchar IS NULL OR snapshot_type = sqlc.narg('snapshot_type')::varchar)
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: CreateProjectConfigRevisionWithGovernanceFields :one
INSERT INTO project_config_revisions (
    tenant_id,
    project_id,
    revision_number,
    config_snapshot,
    change_summary,
    changed_sections,
    previous_revision_id,
    policy_fingerprint,
    diff_summary,
    created_by_user_id,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('revision_number')::integer,
    sqlc.arg('config_snapshot')::jsonb,
    sqlc.narg('change_summary')::text,
    COALESCE(sqlc.narg('changed_sections')::jsonb, '[]'::jsonb),
    sqlc.narg('previous_revision_id')::uuid,
    sqlc.narg('policy_fingerprint')::varchar,
    COALESCE(sqlc.narg('diff_summary')::jsonb, '{}'::jsonb),
    sqlc.arg('created_by_user_id')::uuid,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;

-- name: ListProjectConfigRevisions :many
SELECT * FROM project_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
ORDER BY revision_number DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: GetProjectConfigRevision :one
SELECT * FROM project_config_revisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid;
