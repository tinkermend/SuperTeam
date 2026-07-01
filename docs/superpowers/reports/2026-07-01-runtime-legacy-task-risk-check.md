# Runtime Legacy Task Risk Check

日期：2026-07-01

## Scope

Read-only check for active non-command-driven rows in `tasks` before disabling legacy Runtime polling.

## Service Status

- Temporal: `temporal: running-external healthy=http://127.0.0.1:8233/`
- Control Plane: `control-plane: running-external healthy=http://127.0.0.1:8081/health`
- Web: `web: running-external healthy=http://127.0.0.1:3000/`
- Runtime Agent: `runtime-agent: stopped`
- Database: the running Control Plane process was started from `/Users/tinker/src/singe/SuperTeam` with `apps/control-plane/config/config.yaml`; that gitignored config points at a remote Postgres host, not the local Docker Compose Postgres service, so no safe development database URL was confirmed for SQL inspection.

## Query

```sql
SELECT status, count(*) AS count
FROM tasks
WHERE deleted_at IS NULL
  AND status IN ('pending', 'claimed', 'running')
  AND COALESCE(params->>'provider_run_protocol', '') <> 'provider-run/v1'
GROUP BY status
ORDER BY status;
```

## Result

- `Blocked: no safe development database URL was confirmed, so SQL was not run.`

## Recommendation

- Confirm an intended development database before running the read-only SQL.
- If zero rows are found after confirmation: proceed with default no-op.
- If rows exist after confirmation: decide whether to cancel, migrate to DigitalEmployee Run, or temporarily enable explicit legacy compatibility.
