What you implemented

- Added forward migration `apps/control-plane/internal/storage/migrations/046_team_constitution_and_skill_binding_rename.sql`.
- The migration adds `tenant_teams.constitution JSONB NOT NULL DEFAULT '{}'::jsonb`.
- The migration backfills `tenant_teams.constitution` from the active row in `tenant_team_config_revisions`.
- The migration renames `skill_team_bindings` to `team_skill_bindings`.
- Added `TestMigration046TeamConstitutionAndSkillBindingRename` in `apps/control-plane/internal/storage/migrations_test.go` to assert the required SQL fragments.
- Updated `apps/control-plane/internal/storage/migrations/atlas.sum`.

Test results, including RED/GREEN evidence if you could run both

- RED:
  - Command: `go test ./apps/control-plane/internal/storage/... -run TestMigration046 -v`
  - Result: FAIL
  - Evidence: `read migration 046_team_constitution_and_skill_binding_rename.sql: open migrations/046_team_constitution_and_skill_binding_rename.sql: no such file or directory`
- GREEN:
  - Command: `go test ./apps/control-plane/internal/storage/... -run TestMigration046 -v`
  - Result: PASS
- Migration validation:
  - Command required by brief: `make -C apps/control-plane migrate-validate`
  - First result: FAIL because `atlas.sum` had not been rehashed yet after adding migration 046.
  - After `atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations`, reran validation.
  - Default Make target then failed because local `docker` was unavailable.
  - Final passing validation used the same repo target with explicit dev DB override:
    - `DEV_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' make -C apps/control-plane migrate-validate`
  - Result: PASS

Files changed

- `apps/control-plane/internal/storage/migrations/046_team_constitution_and_skill_binding_rename.sql`
- `apps/control-plane/internal/storage/migrations/atlas.sum`
- `apps/control-plane/internal/storage/migrations_test.go`

Self-review findings

- Kept the change forward-only; no historical migration was rewritten.
- Kept scope to assigned files only.
- Used `ADD COLUMN IF NOT EXISTS` to stay compatible with rebuild-era schemas where the column may already exist in `001_initial.sql`.
- Backfill copies only the `active` governance revision, matching the task brief.
- Rename is intentionally non-conditional because the migration stream already creates `skill_team_bindings` before 046.

Issues/concerns

- `make -C apps/control-plane migrate-validate` is environment-sensitive in this workspace because the default `DEV_URL` depends on `docker`, which is not installed here.
- Validation still completed successfully by overriding `DEV_URL` to a disposable local Postgres container started via `podman`.

---

Fix worker follow-up for reviewer Important issue

What changed

- Replaced the reviewer-gap coverage with an executable schema assertion path for migration 046 in `apps/control-plane/internal/storage/migrations_test.go`.
- Kept the existing SQL-fragment test `TestMigration046TeamConstitutionAndSkillBindingRename`.
- Added `TestMigration046TeamConstitutionAndSkillBindingRenameAppliedSchema` to apply the full migration set against a real PostgreSQL schema and assert:
  - `tenant_teams.constitution` exists
  - `team_skill_bindings` exists
  - `skill_team_bindings` is absent
- Added small local helpers:
  - `applyAllMigrations`
  - `assertTableExists`
  - `assertTableAbsent`
  - `assertColumnExists`
- The executable path safely skips when `TEST_DATABASE_URL` is unset, matching the file's existing integration-test pattern.

Commands run and exact pass/fail outcome

- `go test ./apps/control-plane/internal/storage/... -run TestMigration046 -v`
  - PASS for `TestMigration046TeamConstitutionAndSkillBindingRename`
  - SKIP for `TestMigration046TeamConstitutionAndSkillBindingRenameAppliedSchema`
  - Skip reason: `set TEST_DATABASE_URL to run executable migration schema tests`
- `TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' go test ./apps/control-plane/internal/storage/... -run TestMigration046 -v`
  - PASS
  - Evidence: both `TestMigration046TeamConstitutionAndSkillBindingRename` and `TestMigration046TeamConstitutionAndSkillBindingRenameAppliedSchema` passed
- `DEV_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' make -C apps/control-plane migrate-validate`
  - PASS
- `git diff --check`
  - PASS

Files changed

- `apps/control-plane/internal/storage/migrations_test.go`
- `.superpowers/sdd/task-1-report.md`

Self-review

- Scope stayed on the reviewer's Important issue only.
- The real-schema assertion uses a dedicated throwaway schema under the supplied test database, so it does not depend on or mutate application schemas.
- The helper is intentionally local to `migrations_test.go` and reuses the file's existing `TEST_DATABASE_URL` contract instead of introducing a new env var or broader test harness.
- Verification covered both the skip-safe path and the real migrated-schema path on PostgreSQL.
