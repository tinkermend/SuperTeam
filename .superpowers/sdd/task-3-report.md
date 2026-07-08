Task 3a Report

Status
- Implemented Task 3a only: employee repository team baseline reader.
- Kept existing GetCurrentTeamConfigRevision in place for compile continuity per controller resolution.
- Did not touch Task 3b service/handler/openapi changes.

Scope
- apps/control-plane/internal/employee/repository.go
- apps/control-plane/internal/employee/pg_repository.go
- apps/control-plane/internal/employee/types.go
- apps/control-plane/internal/employee/pg_repository_test.go
- apps/control-plane/internal/employee/service_test.go

Files Changed
- apps/control-plane/internal/employee/repository.go
- apps/control-plane/internal/employee/pg_repository.go
- apps/control-plane/internal/employee/types.go
- apps/control-plane/internal/employee/pg_repository_test.go
- apps/control-plane/internal/employee/service_test.go

Implementation Summary
- Added TeamBaseline type with Constitution, Skills, MCPServers.
- Added Repository.GetTeamBaseline(ctx, tenantID, teamID).
- Implemented PgRepository.GetTeamBaseline:
  - reads tenant_teams.constitution via existing sqlc GetTenantTeam
  - reads team skill identifiers from team_skill_bindings joined to skills, returning skills.slug
  - reads team MCP identifiers from existing sqlc ListTeamMCPBindings, returning mcp_servers.server_key
  - filters MCP bindings to active status
- Kept GetCurrentTeamConfigRevision unchanged.
- Added a focused repo integration test TestGetTeamBaseline with skip-safe DB handling.
- Added minimal memoryRepository.GetTeamBaseline test stub so package tests compile after interface expansion.

Identifier Decisions
- Skills: used skills.slug as the stable identifier.
- MCP servers: used mcp_servers.server_key as the stable identifier.
  - Reason: schema/migration comments describe server_key as the stable MCP identifier rendered into provider config.

RED Evidence
1. Before implementation:
   Command:
   go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v

   Result:
   - build failed
   - repo.GetTeamBaseline undefined (type Repository has no field or method GetTeamBaseline)

2. After interface addition, before memory test stub update:
   Command:
   go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v

   Result:
   - build failed
   - *memoryRepository does not implement Repository (missing method GetTeamBaseline)

GREEN Evidence
1. Targeted test command:
   go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v

   Result:
   - PASS at package level
   - TestGetTeamBaseline skipped because TEST_DATABASE_URL / DATABASE_URL and TEST_REDIS_URL / REDIS_URL were not set in this environment
   - skip path is intentional and keeps repo tests safe on machines without dedicated test DB env

2. Build command:
   go build ./apps/control-plane/...

   Result:
   - success

3. Hygiene command:
   git diff --check

   Result:
   - success

Commands Run
- go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v
- go build ./apps/control-plane/...
- git diff --check
- gofmt -w apps/control-plane/internal/employee/repository.go apps/control-plane/internal/employee/pg_repository.go apps/control-plane/internal/employee/types.go apps/control-plane/internal/employee/pg_repository_test.go apps/control-plane/internal/employee/service_test.go

Self Review
- The implementation is intentionally narrow and does not remove old revision-based code paths.
- The new repo method depends on PgRepository being constructed with a DBTX in the optional db arg when direct SQL is needed. Production wiring already does this in app.NewContainerWithConfig. The new integration test now also passes conn explicitly.
- I avoided sqlc/query regeneration churn for Task 3a by using one focused direct SQL read for skill slugs and existing sqlc for team/team-MCP reads.
- The test fixture asserts the contract requested in the brief: 2 skills, 1 MCP, constitution hard_rules ["r1"].

Concerns
- Real GREEN for TestGetTeamBaseline against a live test database was not possible in this shell because TEST_DATABASE_URL, DATABASE_URL, TEST_REDIS_URL, and REDIS_URL were all unset. Current evidence is compile-safe plus skip-safe, not live DB proof.
- GetTeamBaseline currently uses direct SQL for skill slug lookup instead of a generated sqlc query. This keeps scope minimal, but Task 3b or later cleanup may prefer moving it into storage queries for consistency.
- The method returns MCP server_key rather than display name. This matches the schema’s stable identifier semantics, but any downstream caller must expect the stable key, not the UI label.

---

Task 3a Fix Worker Follow-up

Status
- BLOCKED after fixing the test fixture issue reported by the controller.

What I changed
- Updated `apps/control-plane/internal/employee/pg_repository_test.go` to split the DB fixture setup into separate `conn.Exec(...)` calls so pgx does not send multiple semicolon-separated parameterized statements through one prepared statement.
- Kept the test DB-backed.
- Kept the no-env path skip-safe and narrowed the skip message to the actual requirement for this test: `TEST_DATABASE_URL` or `ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1` with `DATABASE_URL`.

Verification Evidence
1. Reproduced original fixture failure before the fix:
   - Command:
     `TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/postgres?sslmode=disable' go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v`
   - Result:
     `pg_repository_test.go:82`
     `ERROR: cannot insert multiple commands into a prepared statement (SQLSTATE 42601)`

2. After the fixture fix, the DB-backed test progressed past setup and failed inside repository SQL:
   - Same command as above
   - Result:
     `pg_repository_test.go:101`
     `ERROR: column stb.deleted_at does not exist (SQLSTATE 42703)`

3. Skip-safe verification without DB env:
   - Command:
     `go test ./apps/control-plane/internal/employee/... -run TestGetTeamBaseline -v`
   - Result:
     `--- SKIP: TestGetTeamBaseline`
     `set TEST_DATABASE_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL`

4. Package build:
   - Command:
     `go build ./apps/control-plane/...`
   - Result:
     success

5. Diff hygiene:
   - Command:
     `git diff --check`
   - Result:
     success

Blocker Evidence
- `apps/control-plane/internal/employee/pg_repository.go` currently queries:
  - `FROM team_skill_bindings stb`
  - `AND stb.deleted_at IS NULL`
- Migration `apps/control-plane/internal/storage/migrations/046_team_constitution_and_skill_binding_rename.sql` only renames `skill_team_bindings` to `team_skill_bindings`; it does not add a `deleted_at` column.
- That makes the remaining DB-backed failure a production query/schema mismatch, not a fixture problem.

Why I stopped
- The task instructions explicitly said not to change production code unless the fixed fixture revealed a production bug, and to report `BLOCKED` with evidence before making broader changes.
- The fixture is now executable; the remaining failure is a production bug requiring a separate fix decision.
