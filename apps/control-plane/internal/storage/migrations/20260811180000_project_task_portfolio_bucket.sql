-- Project portfolio exclusive task-status bucket (spec 2026-08-11 §3.2 / §5.2).
-- Single SQL fact source for GetProjectTaskStatusCounts + portfolio queries.
-- Priority matches ClassifyProjectTaskPortfolioBucket in Go — keep them in lockstep.

CREATE OR REPLACE FUNCTION project_task_portfolio_bucket(
    status text,
    requires_human_approval boolean
) RETURNS text
LANGUAGE sql
IMMUTABLE
PARALLEL SAFE
AS $$
    SELECT CASE
        WHEN lower(btrim(COALESCE(status, ''))) = 'cancelled' THEN 'cancelled'
        WHEN lower(btrim(COALESCE(status, ''))) IN ('completed', 'done', 'success') THEN 'completed'
        WHEN lower(btrim(COALESCE(status, ''))) IN ('failed', 'error') THEN 'failed'
        WHEN lower(btrim(COALESCE(status, ''))) = 'blocked' THEN 'blocked'
        WHEN lower(btrim(COALESCE(status, ''))) IN (
            'waiting_human', 'pending_human', 'pending_review', 'approval_required'
        )
          OR (
              COALESCE(requires_human_approval, false)
              AND lower(btrim(COALESCE(status, ''))) NOT IN (
                  'cancelled', 'completed', 'done', 'success',
                  'failed', 'error', 'blocked'
              )
          ) THEN 'waiting_human'
        WHEN lower(btrim(COALESCE(status, ''))) IN ('running', 'in_progress') THEN 'running'
        WHEN lower(btrim(COALESCE(status, ''))) = 'queued' THEN 'queued'
        WHEN lower(btrim(COALESCE(status, ''))) IN ('pending', 'planned', 'assigned') THEN 'pending'
        ELSE 'other'
    END;
$$;

COMMENT ON FUNCTION project_task_portfolio_bucket(text, boolean) IS
  'Exclusive portfolio display bucket for project_tasks (cancelled>completed>failed>blocked>waiting_human>running>queued>pending>other). Shared by overview counts and GET /projects/portfolio.';
