# Task 3b Report

## Scope

- Modified `apps/control-plane/internal/employee/service.go`
- Modified `apps/control-plane/internal/employee/handler.go`
- Modified `apps/control-plane/internal/employee/service_test.go`
- Modified `apps/control-plane/internal/employee/types.go`
- Modified `contracts/control-plane/openapi.yaml`
- Regenerated `apps/control-plane/internal/api/gen/control_plane.gen.go`
- Regenerated `apps/control-plane/gen/control_plane.gen.go`

## RED evidence

### Added failing tests first

- `TestCreateEmployeeWithoutTeamRevision`
- `TestCreateOptionsUsePlatformFullEmployeeTypes`
- `TestCreateOptionsUsesTeamBaseline`

### RED command

```bash
go test ./apps/control-plane/internal/employee/... -run 'TestCreateEmployeeWithoutTeamRevision|TestCreateOptionsUsePlatformFullEmployeeTypes|TestCreateOptionsUsesTeamBaseline' -v
```

### RED result

- Failed at compile stage before implementation because `TeamConfigCreateOption` still exposed the old create-options shape and did not contain:
  - `constitution`
  - `skills`
  - `mcp_servers`

Key failure:

```text
options.TeamConfig.Constitution undefined
options.TeamConfig.Skills undefined
options.TeamConfig.MCPServers undefined
```

This established that the old contract/model shape was still active before the implementation changes.

## Implementation summary

- `GetCreateOptions` now:
  - calls `EnsureTeamExists`
  - uses `GetTeamBaseline`
  - does not require active team config revision
  - returns platform full `DefaultEmployeeTypeDefinitions()`
  - returns platform full provider pool
  - builds `team_config` from slim baseline fields only
- `CreateDigitalEmployee` now:
  - uses `GetTeamBaseline`
  - does not require active team config revision
  - no longer rejects employee type/provider type based on team allowlists in this create path
- create-options contract now:
  - removes `external_capabilities`
  - slims `team_config` to `{id, tenant_id, team_id, constitution, skills, mcp_servers}`
- compatibility path kept for effective-config preview/create by converting baseline into a minimal `TeamConfigInput` with empty policy blobs and no revision gate

## GREEN evidence

### Focused GREEN command

```bash
go test ./apps/control-plane/internal/employee/... -run 'TestCreateEmployeeWithoutTeamRevision|TestCreateOptionsUsePlatformFullEmployeeTypes|TestCreateOptionsUsesTeamBaseline' -v
```

### Focused GREEN result

```text
=== RUN   TestCreateEmployeeWithoutTeamRevision
--- PASS: TestCreateEmployeeWithoutTeamRevision (0.00s)
=== RUN   TestCreateOptionsUsePlatformFullEmployeeTypes
--- PASS: TestCreateOptionsUsePlatformFullEmployeeTypes (0.00s)
=== RUN   TestCreateOptionsUsesTeamBaseline
--- PASS: TestCreateOptionsUsesTeamBaseline (0.00s)
PASS
```

## Full verification

### Employee package

```bash
go test ./apps/control-plane/internal/employee/... -v
```

- Result: PASS

### Contract generation

```bash
corepack pnpm generate:control-plane
go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config ../../contracts/control-plane/oapi-codegen.server.yaml ../../contracts/control-plane/openapi.yaml
```

- Result:
  - regenerated `apps/control-plane/internal/api/gen/control_plane.gen.go`
  - regenerated `apps/control-plane/gen/control_plane.gen.go`

### Contract verification

```bash
corepack pnpm verify:contracts
```

- Result: PASS
- Output: `foundation contract guard passed`

### Build

```bash
go build ./apps/control-plane/...
```

- Result: PASS

### Diff hygiene

```bash
git diff --check
```

- Result: PASS

## Files changed

- `apps/control-plane/internal/employee/service.go`
- `apps/control-plane/internal/employee/handler.go`
- `apps/control-plane/internal/employee/service_test.go`
- `apps/control-plane/internal/employee/types.go`
- `contracts/control-plane/openapi.yaml`
- `apps/control-plane/internal/api/gen/control_plane.gen.go`
- `apps/control-plane/gen/control_plane.gen.go`
- `.superpowers/sdd/task-3b-report.md`

## Self-review

- Confirmed create-options no longer reads active team revision.
- Confirmed create flow no longer rejects on team employee/provider allowlists.
- Confirmed `team_config.skills` and `team_config.mcp_servers` come from baseline-derived compatibility input, not old policy blobs from an active revision fetch.
- Confirmed `external_capabilities` removed from create-options contract/handler response.
- Confirmed Task 4 removal of effective-config routes/subsystem was not attempted.

## Concerns

- `apps/control-plane/internal/employee/types.go` needed a shape update for `TeamConfigCreateOption` and `CapabilityOptions` so the new contract and handler mapping could compile. This is still within Task 3b behavior, but it is one additional touched file beyond the original short file list.
- Verification was local-scope as requested by the task. No real running service/browser smoke was attempted because this task was explicitly framed as a scoped control-plane/contract implementation with required command-level verification.
