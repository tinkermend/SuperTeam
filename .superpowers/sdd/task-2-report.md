# Task 2 Report

## What changed

- Renamed runtime query references from `skill_team_bindings` to `team_skill_bindings` in:
  - `apps/control-plane/internal/storage/queries/skill_runtime.sql`
  - `apps/control-plane/internal/storage/queries/employee_execution.sql`
- Regenerated the owned sqlc outputs:
  - `apps/control-plane/internal/storage/queries/skill_runtime.sql.go`
  - `apps/control-plane/internal/storage/queries/employee_execution.sql.go`
- Updated owned tests to use the renamed table:
  - `apps/control-plane/internal/storage/queries/queries_test.go`
  - `apps/control-plane/internal/storage/queries/employee_execution_static_test.go`

## Commands and results

1. Red-step check before regeneration:
   - Command: `go test ./apps/control-plane/internal/storage/queries -run TestGeneratedSchedulingSkillCountQueryMatchesEffectiveSkillPrecedence -v`
   - Result: failed as expected before regeneration, with:
     - `scheduling skill count query must count team bindings as inherited skills`

2. sqlc regeneration:
   - Command: `make -C apps/control-plane generate-sqlc`
   - Result: passed

3. Build verification:
   - Command: `go build ./apps/control-plane/...`
   - Result: passed

4. Query package tests:
   - Command: `go test ./apps/control-plane/internal/storage/queries/... -v`
   - Result: passed for the package; integration tests were skipped because `TEST_DATABASE_URL` and `TEST_REDIS_URL` were not set. Static test path still ran and passed.

5. Diff sanity:
   - Command: `git diff --check`
   - Result: passed

6. Scoped old-name check on Task 2 owned files:
   - Command: `rg -n "skill_team_bindings" apps/control-plane/internal/storage/queries/{skill_runtime.sql,employee_execution.sql,skill_runtime.sql.go,employee_execution.sql.go,queries_test.go,employee_execution_static_test.go}`
   - Result: no matches

7. Scoped new-name check on Task 2 owned files:
   - Command: `rg -n "team_skill_bindings" apps/control-plane/internal/storage/queries/{skill_runtime.sql,employee_execution.sql,skill_runtime.sql.go,employee_execution.sql.go,queries_test.go,employee_execution_static_test.go}`
   - Result: matches present in all expected owned source/generated/test files

## Files changed

- `apps/control-plane/internal/storage/queries/skill_runtime.sql`
- `apps/control-plane/internal/storage/queries/employee_execution.sql`
- `apps/control-plane/internal/storage/queries/skill_runtime.sql.go`
- `apps/control-plane/internal/storage/queries/employee_execution.sql.go`
- `apps/control-plane/internal/storage/queries/queries_test.go`
- `apps/control-plane/internal/storage/queries/employee_execution_static_test.go`
- `/.superpowers/sdd/task-2-report.md`

## Self-review

- Query logic was not changed beyond the table rename.
- The red/green static test cycle was observed for the generated scheduling-skill query.
- Generated sqlc output matches the renamed table in the owned runtime query surfaces.
- Old table-name references were removed from the owned non-migration query/generated/test files for Task 2.

## Concerns

- `make -C apps/control-plane generate-sqlc` also refreshed unrelated generated files already drifting in this branch:
  - `apps/control-plane/internal/storage/queries/models.go`
  - `apps/control-plane/internal/storage/queries/tenant_team.sql.go`
  - `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go`
- Those files were intentionally left out of this task’s commit to respect the ownership boundary.
- Full storage-query integration coverage was not exercised because the dedicated test DB and Redis env vars were unset in this workspace.

---

## Task 2 generated-output consistency fix

### Investigation

- Checked the dirty generated diffs in:
  - `apps/control-plane/internal/storage/queries/models.go`
  - `apps/control-plane/internal/storage/queries/tenant_team.sql.go`
  - `apps/control-plane/internal/storage/queries/tenant_team_config.sql.go`
- Confirmed they are legitimate `sqlc` output drift caused by schema/query sources already present on this branch:
  - migration `apps/control-plane/internal/storage/migrations/046_team_constitution_and_skill_binding_rename.sql` adds `tenant_teams.constitution`
  - `apps/control-plane/internal/storage/queries/tenant_team.sql` uses `RETURNING *`
  - `apps/control-plane/internal/storage/queries/tenant_team_config.sql` uses `SELECT *` and `tt.*`
- Regenerating with `make -C apps/control-plane generate-sqlc` reproduced the same dirty output, proving the files are generated from current sources rather than hand edits.

### What the generated diffs contain

- `models.go`
  - replaces the stale `SkillTeamBinding` model with `TeamSkillBinding`
  - adds `Constitution []byte` to `TenantTeam`
- `tenant_team.sql.go`
  - widens `SoftDeleteTeam` scan/return shape to include `constitution`
- `tenant_team_config.sql.go`
  - widens generated scans/results for `tenant_teams`-backed queries to include `constitution`

### Verification

1. `make -C apps/control-plane generate-sqlc`
   - passed
2. `go build ./apps/control-plane/...`
   - passed
3. `go test ./apps/control-plane/internal/storage/queries/... -v`
   - passed; integration tests were skipped because `TEST_DATABASE_URL` and `TEST_REDIS_URL` were not set
4. `git diff --check`
   - passed

### Resolution

- Included the three generated files in the Task 2 checkpoint so the branch no longer carries sqlc generated-code drift from migration 046 / team constitution / team-skill-binding rename.

---

## Controller follow-up fix: raw SQL rename completion

### What changed

- Replaced the remaining raw SQL table references in `apps/control-plane/internal/skill/pg_repository.go` from `skill_team_bindings` to `team_skill_bindings`.
- Updated static assertions in `apps/control-plane/internal/skill/service_test.go` to check the renamed table in repository source.

### Verification

1. `go test ./apps/control-plane/internal/skill/... -v`
2. `go build ./apps/control-plane/...`
3. `rg -n "skill_team_bindings" apps/control-plane/internal/skill`
4. `git diff --check`

### Notes

- No migration files or migration tests were changed.
- The fix was intentionally limited to the controller-flagged runtime raw SQL and matching static tests.
