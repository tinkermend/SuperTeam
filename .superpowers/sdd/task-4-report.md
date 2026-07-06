# Task 4 Report — Sync `contracts/control-plane/openapi.yaml`

## Status: DONE_WITH_CONCERNS (one deviation from brief, see Concerns)

## What was added

All four pieces from the brief were applied to `contracts/control-plane/openapi.yaml`, plus the regenerated tracked `control_plane.gen.go`.

### 1. New schema: `DigitalEmployeeRunStats`
Inserted in `components.schemas` immediately after the existing `DigitalEmployeeRun` schema block (so the related run schemas stay grouped). Exact properties from the brief: `total_count`, `succeeded_count`, `failed_count`, `cancelled_count`, `success_rate?`, `avg_duration_sec?`, `p90_duration_sec?`, `last_7d_count`, `prev_7d_count`.

### 2. New path: `GET /api/v1/digital-employees/{employeeId}/run-stats`
Inserted immediately before the existing `/runs` path. References `#/components/parameters/EmployeeId` and returns `#/components/schemas/DigitalEmployeeRunStats`.

### 3. New path: `GET /api/v1/digital-employees/{employeeId}/effective-config`
Inserted between `/effective-configs/approve` (POST) and `/status` (PUT), matching the brief's "after approve" placement. Reuses the existing `#/components/schemas/DigitalEmployeeEffectiveConfig` schema and adds a `404` for the no-approved-config case.

### 4. Enriched path: `GET /api/v1/digital-employees/{employeeId}/runs`
Replaced the prior bare-array response (`type: array / items: $ref DigitalEmployeeRun`) with `$ref: "#/components/schemas/DigitalEmployeeRunList"`. Added four new optional query params (`status`, `project_id` uuid, `from` date-time, `to` date-time). The POST handler on the same path was left untouched.

Plus three new schemas grouped right after `DigitalEmployeeRunStats`: `DigitalEmployeeRunListItem` (allOf over `DigitalEmployeeRun` + `task_title`, `project_id?`, `project_name?`, `work_product_count`, `duration_sec?`), `DigitalEmployeeRunFilterOption` (`value`, `label`), `DigitalEmployeeRunList` (`items`, `total_count`, `filters.{statuses,projects}`).

## Validation — `pnpm generate:control-plane` (run from repo root)

Full output:
```
> superteam@ generate:control-plane /Users/tinker/src/singe/SuperTeam/.worktrees/digital-employee-detail-redesign
> cd apps/control-plane && go generate ./internal/api

---EXIT: 0---
```
Exit 0, no unresolved-ref or syntax errors. Re-running after commit produced zero further changes (generation is idempotent — clean tree afterwards).

## Files changed

- `contracts/control-plane/openapi.yaml` — 148 insertions, 3 deletions (the 3 deletions are the 3 lines of the old bare-array response shape on `/runs`).
- `apps/control-plane/internal/api/gen/control_plane.gen.go` — 204 insertions, 0 deletions (pure additive regeneration; diff contains only the new types `DigitalEmployeeRunFilterOption`, `DigitalEmployeeRunList`, `DigitalEmployeeRunListItem`, `DigitalEmployeeRunStats` and the new request/params structs).

Commit: `4799919a docs(contracts): sync openapi with run stats, effective-config read, and enriched run list`

## Self-review

- All three endpoint changes applied: run-stats (path + schema), effective-config (path, reuses existing schema), runs enrichment (new query params + `$ref` to `DigitalEmployeeRunList`). All four new schemas (`DigitalEmployeeRunStats`, `DigitalEmployeeRunListItem`, `DigitalEmployeeRunFilterOption`, `DigitalEmployeeRunList`) added.
- `pnpm generate:control-plane` exits 0 with no errors; generation is idempotent.
- Indentation preserved: schema names at 4 spaces, paths at 2 spaces, methods at 4 spaces, parameters/properties at 6 — matches surrounding blocks. Verified by reading the surrounding context before each edit and by the zero-deletion gen diff.
- `git diff --check` clean (no whitespace errors).

## Concerns

1. **Brief inaccuracy on gen-file tracking (deviation from "only openapi.yaml").** The brief states the generated `control_plane.gen.go` is gitignored and that "only `contracts/control-plane/openapi.yaml` should show as changed." Reality: `.gitignore` does contain `*.gen.go`, but this specific file was committed to the index before that rule existed, so git still treats it as tracked. Running generation modified it. Every recent commit that touched `openapi.yaml` (`937aab63`, `632bd2e9`, `d86038e2`, `38391ce9`, etc.) also committed the regenerated `control_plane.gen.go`, so the repo convention is to commit both together. I followed that convention and committed both; leaving the gen file modified-but-uncommitted would drift the committed contract from the committed Go types and hand Task 5 a dirty tree. The gen diff is purely additive (only the new symbols from this task). Flagging because it contradicts the brief's literal instruction — if you'd prefer the gen file left out, a `git rm --cached` + force-ignore change would need to be a separate, intentional decision.

2. **No other concerns.** Indentation, placement, ref resolution, and generation all pass.
