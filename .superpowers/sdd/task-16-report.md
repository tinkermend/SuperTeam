# Task 16: chat project-anchor backend (spec §13)

## Summary

Implemented §13's decision: chat runs (`run_kind=chat`) no longer preflight
through the standalone `digital_employee_execution_instances` path (which
`BindExecutionInstance` no longer populates). Instead they carry a required
`project_id` runtime anchor and resolve their dispatch node/preflight the same
way project task dispatch does. The anchor project receives **no** business
effect — no signal, ProjectTask, or RouteDecision; `anchor_project_id` is
persisted only in `tasks.params["metadata"]` for audit, not a new column.
`run_kind=task` requests are completely unaffected (still legacy
`GetRunPreflight` path); any `project_id` they send is ignored.

## Files changed

- `apps/control-plane/internal/employee/run_types.go` — `CreateDigitalEmployeeRunRequest.ProjectID *uuid.UUID`.
- `apps/control-plane/internal/employee/run_repository.go` — new `ChatAnchorProjectValidator` interface (mirrors `ProjectTaskNodeResolver`'s "declared here, implemented by project package" pattern so `employee` never imports `project`).
- `apps/control-plane/internal/employee/run_service.go`:
  - `DigitalEmployeeRunService.chatAnchorValidator` field + `SetChatAnchorProjectValidator` setter.
  - `CreateRun`: chat requires non-nil `project_id` (400 via `ErrInvalidInput`) or the request is rejected before touching any dependency; task-kind clears `req.ProjectID = nil` ("task 时忽略"); resume validation gained the anchor-match check (see below); chat requests are dispatched via the new `createChatRun`.
  - `createChatRun` (new): `ResolveProjectTaskNode` (with `ProjectTaskID: uuid.Nil`) → `GetProjectTaskRunPreflightForNode` → `validateProjectTaskRunPreflight` → `validateDailyTokenBudget` → `dispatcher.IsConnected` → derives a chat-specific compat `ExecutionInstanceID` + one-off `AgentHomeDir` → `projectTaskRunPreflightToRunPreflight` → sets `metadata["anchor_project_id"]` → reuses `createAndDispatchRun` unchanged.
  - New helpers: `chatDispatchNonce`, `chatAgentHomeDir`, `chatCompatibilityExecutionInstanceID`.
- `apps/control-plane/internal/employee/run_handler.go` — decode struct + request mapping gained `project_id`.
- `apps/control-plane/internal/employee/run_service_test.go` — see TDD section.
- `apps/control-plane/internal/project/chat_anchor_validator_adapter.go` (new) — `ChatAnchorProjectValidatorAdapter` delegating to the existing private `Service.requireActiveProject` (tenant-scoped `GetProject` + archived check), translating `ErrProjectNotFound`/`ErrInvalidProject`/`ErrProjectArchived` into `employee.ErrInvalidInput`-wrapped errors.
- `apps/control-plane/internal/app/app.go` — wires `runService.SetChatAnchorProjectValidator(project.NewChatAnchorProjectValidatorAdapter(projectService))` next to the existing `SetProjectTaskNodeResolver` line.
- `contracts/control-plane/openapi.yaml` — `CreateDigitalEmployeeRunRequest.project_id` (uuid, optional in schema; server enforces chat-required, task-ignored). Ran `generate:control-plane` (regenerates `internal/api/gen/control_plane.gen.go`) and `verify:contracts` (passed: "foundation contract guard passed").

## Key decisions

1. **`ProjectTaskID` for chat's node resolution: `uuid.Nil`.** Verified in
   `internal/project/node_resolver.go:104` — `ResolveProjectTaskNode` explicitly
   documents and treats `ProjectTaskID == uuid.Nil` as "skip the task hard-pin
   layer," proceeding straight to layer-1 eligibility/online/capacity + layer-2
   affinity/load. No dereference, no forced row lookup, no panic. Chat has no
   project task by construction, so this is the correct (not a workaround)
   value, not a hack.

2. **Project validation: new `ChatAnchorProjectValidator` interface + adapter,
   not reuse of `ResolveProjectTaskNode`'s error.** Research confirmed no
   existing project-existence/tenant/archived check is reachable from
   `employee`, and `ResolveProjectTaskNode` itself never validates project
   existence — an unresolvable/nonexistent project just falls through to a
   generic `no_eligible_online_node` reason, which is not distinguishable from
   "genuinely no capacity." So a dedicated interface was added, following the
   exact `ProjectTaskNodeResolver`/`ProjectTaskNodeResolverAdapter` composition
   pattern already in the codebase. The adapter reuses the private
   `Service.requireActiveProject` (same package, so directly callable) rather
   than reimplementing the archived check — `GetProject`'s repository lookup is
   already tenant+id scoped, so cross-tenant and not-found both collapse to
   `ErrProjectNotFound` (no separate tenant-mismatch sentinel exists in
   `project`, by design per the research).

3. **Chat's compat `ExecutionInstanceID` / one-off `AgentHomeDir`.** Chat has no
   natural per-attempt id (unlike project tasks, which have
   `project_task_attempt_id`). Path shape:
   `<workspace_base_dir>/chat/<project_id>/<employee_id>/<nonce>`; compat
   execution-instance id is `SHA1(tenant:project:employee:"chat":nonce)`. The
   `nonce` is a hash of the caller's `idempotency_key` when supplied, else a
   fresh value per call — this keeps genuine idempotent retries (same
   `idempotency_key`, active run still in flight) landing on the same
   directory/fingerprint, preserving `createAndDispatchRun`'s existing
   idempotent re-dispatch path; without an idempotency key, retries already had
   no such guarantee (`sameIdempotentRun` requires one). This is a
   from-scratch design choice (spec only said "pick a clean shape and document
   it") — flagged for human review if a different shape is wanted (e.g. a
   response field surfacing the workspace path, or literal per-run uniqueness
   at the cost of idempotent-retry safety).
4. **Resume anchor-match check** reads the prior run's anchor via
   `s.repository.GetRunTaskMetadata(ctx, tenantID, prior.TaskID)["anchor_project_id"]`
   (existing method, `run_repository.go:38`/`pg_run_repository.go:603`, reads
   `tasks.params["metadata"]`) and rejects (`ErrInvalidResumeRun`) if it's empty
   or doesn't match the request's `project_id` string-for-string.
5. Did **not** add `project_id`/anchor readback to the run response payload or
   touch any frontend code — out of scope per the task brief (backend-only);
   noted as a UI follow-up dependency (§13 "对话态恢复项目 chip" / "转为任务默认预选锚项目"
   will need a way to read the anchor back, either via this metadata or a new
   response field — a decision for the frontend task).

## TDD

RED (confirmed real, not vacuous, via mutation testing — see below) → GREEN.

New tests in `run_service_test.go`:
- `TestCreateRunChatRequiresProjectID` (a) — chat without `project_id` →
  `ErrInvalidInput`, before any chat-anchor dependency is even called.
- `TestCreateRunChatResolvesProjectAnchorNodeAndDispatches` (b) — asserts
  validator called with (tenant, project), resolver called once with
  `ProjectTaskID == uuid.Nil`, preflight looked up by the resolved node,
  `metadata["anchor_project_id"]` persisted, dispatch goes to the resolved
  node.
- `TestCreateRunChatResumeRejectsMismatchedAnchorProject` (c) — prior run's
  stubbed `anchor_project_id` differs from the request's `project_id` →
  `ErrInvalidResumeRun`, no run created.
- `TestCreateRunTaskKindIgnoresProjectIDAndUsesLegacyPreflight` (d) — task-kind
  run with a caller-supplied `project_id` still uses legacy `GetRunPreflight`;
  resolver/validator are never called.
- Updated the two pre-existing chat tests
  (`TestCreateRunChatResumeValidation`, `TestCreateRunChatResumeInjectsProviderSession`)
  to wire the new chat-anchor fakes (they previously exercised the now-dead
  standalone-preflight chat path).
- New fakes: `fakeChatAnchorProjectValidator`; `fakeRunServiceRepository.taskMetadata`
  map + updated `GetRunTaskMetadata`; helpers `chatAnchorRunServiceRepository`,
  `chatAnchorRunService`; `runServiceProjectID` fixture id.

Mutation-tested the two most safety-critical checks by temporarily deleting
them and re-running: (1) removing the `project_id` nil-check — the fixed
`TestCreateRunChatRequiresProjectID` (after correcting an initial version that
false-passed due to an earlier missing-dependency error masking it) now fails
with a nil-pointer-dereference panic, proving it's load-bearing. Reverted
immediately after confirming.

Final: `go build ./internal/... ./cmd/...` clean;
`go test ./internal/...` — all packages pass (employee, project, app, api,
runtime, workflow/projectcoordination, etc.), no skips beyond the pre-existing
Postgres-integration tests that require `TEST_DATABASE_URL`.
`corepack pnpm verify:contracts` — "foundation contract guard passed".

## Verification scope note

Per repo verification rules, this change touches interface/API surface
(request schema, service dispatch path) — the full bar is real end-to-end
verification (running services, real DB, real dispatch). This task's scope
was backend implementation + TDD; I did not start `dev-services.sh` or drive a
real chat run through a live runtime/provider. **Flagging as a verification
gap**, not silently declaring e2e-done: someone should run a real chat create
→ resolve → dispatch → resume cycle against live control-plane + runtime +
provider before this is considered fully shipped, per project policy.

## Concerns / follow-ups

- Frontend (§13 UI: project chip on chat, pre-selected anchor on "转为任务") is
  untouched — separate task.
- No real end-to-end run was executed (see verification note above).
- `chatDispatchNonce`'s idempotency-key-hash approach is a from-scratch design
  choice, not spec-mandated; revisit if product wants strictly per-run unique
  directories regardless of idempotency-retry safety.

## Fix: dry-run anchor + rejection matrix tests

Addressed two code-review findings on this task's implementation.

**Finding 1 (chat resolver must not persist node affinity).** `createChatRun`
called `ResolveProjectTaskNode` via `ProjectTaskNodeResolverAdapter` with
`DryRun` unset (zero value `false`), so every chat send persisted
`UpsertProjectEmployeeNodeAffinity` — silently steering future REAL task
placement for that employee. `ResolveProjectTaskNodeInput.DryRun` already
existed in `internal/project/node_resolver.go` (confirmed semantics at line
148: `if !in.DryRun { ...UpsertProjectEmployeeNodeAffinity... }` — DryRun
skips only the affinity write, resolution itself still runs and returns a
real node) and was already used elsewhere (`gateProjectTaskNodeResolverAdapter`
in `internal/app/planning_profile_adapter.go`), so this reuses an established
pattern rather than inventing a new one.

Fix:
- `internal/employee/run_repository.go`: added `DryRun bool` to
  `ResolveProjectTaskNodeRequest`.
- `internal/project/node_resolver_adapter.go`: `ProjectTaskNodeResolverAdapter.ResolveProjectTaskNode`
  now passes `DryRun: req.DryRun` into `ResolveProjectTaskNodeInput`.
- `internal/employee/run_service.go`: `createChatRun` now sets `DryRun: true`
  on its `ResolveProjectTaskNodeRequest`. `StartProjectTaskRun`'s request
  literal is unchanged (defaults to `false`), so project-task dispatch keeps
  persisting/updating affinity as before.

**Finding 2 (400 rejection matrix untested).** Spec §13 requires
`project_id` 缺失/跨租户/不存在/archived → 400; only "missing" had a test.
Added:
- `internal/employee/run_service_test.go`:
  `TestCreateRunChatRejectsInvalidAnchorProject` — a fake
  `ChatAnchorProjectValidator` returning an `ErrInvalidInput`-wrapped error
  causes `CreateRun` to propagate that error, with the node resolver never
  called (0 calls) and no run created (`createdRunCount == 0`).
- `internal/project/chat_anchor_validator_adapter_test.go` (new file):
  `ChatAnchorProjectValidatorAdapter.ValidateChatAnchorProject` mapping
  coverage — not-found project, archived project (both map to
  `employee.ErrInvalidInput`, confirmed the raw `project.ErrProjectNotFound`
  / `project.ErrProjectArchived` sentinels do NOT leak across the package
  boundary), and an explicit cross-tenant case (project exists for tenant A,
  validated against tenant B) — cross-tenant is not-found by construction
  since `memoryRepository.GetProject` (mirroring the production
  `pg_repository`) returns `ErrProjectNotFound` whenever
  `project.TenantID != tenantID`, not a distinct sentinel. Also added an
  active-project happy-path test as a baseline.
- Extended the two existing resolver-call assertions
  (`TestCreateRunChatResolvesProjectAnchorNodeAndDispatches` and
  `TestStartProjectTaskRunResolvesNodeThenUsesPreflightForNode`) to assert
  `resolver.lastReq.DryRun` is `true` for chat and `false` for project-task
  dispatch, directly covering Finding 1's fix.

### Test commands + output tails

```
$ go build ./internal/...
(clean, no output)

$ go test -count=1 ./internal/employee/ ./internal/project/
ok  	github.com/superteam/control-plane/internal/employee	0.225s
ok  	github.com/superteam/control-plane/internal/project	0.447s

$ go test ./internal/employee/ -run 'TestCreateRunChat|TestStartProjectTaskRunResolvesNodeThenUsesPreflightForNode' -v
--- PASS: TestStartProjectTaskRunResolvesNodeThenUsesPreflightForNode (0.00s)
--- PASS: TestCreateRunChatResumeValidation (0.00s)
--- PASS: TestCreateRunChatResumeInjectsProviderSession (0.00s)
--- PASS: TestCreateRunChatRequiresProjectID (0.00s)
--- PASS: TestCreateRunChatResolvesProjectAnchorNodeAndDispatches (0.00s)
--- PASS: TestCreateRunChatRejectsInvalidAnchorProject (0.00s)
--- PASS: TestCreateRunChatResumeRejectsMismatchedAnchorProject (0.00s)
PASS

$ go test ./internal/project/ -run 'TestChatAnchorProjectValidatorAdapter' -v
--- PASS: TestChatAnchorProjectValidatorAdapter_ApprovesActiveProject (0.00s)
--- PASS: TestChatAnchorProjectValidatorAdapter_RejectsUnknownProject (0.00s)
--- PASS: TestChatAnchorProjectValidatorAdapter_RejectsCrossTenantProject (0.00s)
--- PASS: TestChatAnchorProjectValidatorAdapter_RejectsArchivedProject (0.00s)
PASS
```

Scope note: this is unit-level Go verification only (`go build`/`go test`),
consistent with this task's original "backend implementation + TDD" scope and
the CLAUDE.md lightweight-verification allowance for this kind of pure
internal-logic/test change — no service restart or real end-to-end run was
performed for this fix. The original task-level e2e verification gap noted
above still stands and is unchanged by this fix.

Commits:
- `d27921ee` fix(employee): resolve chat anchor node in dry-run mode
- `79983237` test(project): cover chat anchor 400 rejection matrix

## Fix: default prompt to objective

**Issue.** Chat/task runs created with only `objective` and no `prompt` send an empty prompt in the start_session payload. Claude Code rejects empty prompts ("prompt or input is required"), failing every such run.

**Fix.** In `CreateRun`, after trimming objective/prompt (line 122), if `prompt == ""`, set `prompt = objective`. Applied to both standalone run kinds (chat + task). Does NOT touch `StartProjectTaskRun` (per spec).

**Tests (TDD).** Added three tests in `run_service_test.go`:
- `TestCreateRunTaskDefaultsPromptToObjective` — task run with empty prompt → dispatched payload has `"prompt" == objective`.
- `TestCreateRunChatDefaultsPromptToObjective` — chat run with empty prompt → dispatched payload has `"prompt" == objective`.
- `TestCreateRunPreservesExplicitPrompt` — explicit non-empty prompt is NOT overwritten.

**Verification.** Full employee package tests pass:

```
$ go test ./apps/control-plane/internal/employee/ -v
...
=== RUN   TestCreateRunTaskDefaultsPromptToObjective
--- PASS: TestCreateRunTaskDefaultsPromptToObjective (0.00s)
=== RUN   TestCreateRunChatDefaultsPromptToObjective
--- PASS: TestCreateRunChatDefaultsPromptToObjective (0.00s)
=== RUN   TestCreateRunPreservesExplicitPrompt
--- PASS: TestCreateRunPreservesExplicitPrompt (0.00s)
...
PASS
ok  	github.com/superteam/control-plane/internal/employee	0.140s
```

Commit: `7b28d573` fix(employee): default standalone run prompt to objective

## Fix: resume session source

**Issue.** Chat follow-up resume always 400s. Resume validation in `CreateRun`'s line 168 checked only `prior.ProviderSessionID` (from `task_runs.provider_session_id` column), but for standalone chat runs that column is NULL. The REAL provider session id (written back by runtime via events, injected into `metadata["provider_session_id"]`) lives in `task_runs.provider_session_external_id` → domain field `prior.ProviderSessionExternalID`. Live DB confirms external id present, internal one NULL.

**Fix.** In `CreateRun`'s resume validation block (lines 156-187):
- Changed the session id resolution logic to prefer `prior.ProviderSessionExternalID`, fall back to `prior.ProviderSessionID` if external ID is nil/blank
- If both are nil/blank, reject with `ErrInvalidResumeRun` (unchanged)
- Inject the resolved session id into `req.Metadata["provider_session_id"]` (key name unchanged — runtime contract)

**Tests (TDD RED → GREEN).** Expanded `TestCreateRunChatResumeInjectsProviderSession` from a single case to a comprehensive 4-subtest matrix:
- "prefers ProviderSessionExternalID when set" — standalone chat path (external ID only)
- "falls back to ProviderSessionID when external ID is blank" — legacy/project-task path fallback
- "uses ProviderSessionExternalID over ProviderSessionID" — both set, external ID wins
- "rejects when both ProviderSessionExternalID and ProviderSessionID are blank" — neither set → ErrInvalidResumeRun

Also updated the existing `TestCreateRunChatResumeValidation` subtest "上个 run 无 provider session" to set BOTH fields to nil before testing rejection (was only setting ProviderSessionID).

**Verification.** Full employee package tests pass:

```
$ go test ./apps/control-plane/internal/employee/ -v
...
=== RUN   TestCreateRunChatResumeInjectsProviderSession
=== RUN   TestCreateRunChatResumeInjectsProviderSession/prefers_ProviderSessionExternalID_when_set
--- PASS: ... (0.00s)
=== RUN   TestCreateRunChatResumeInjectsProviderSession/falls_back_to_ProviderSessionID_when_external_ID_is_blank
--- PASS: ... (0.00s)
=== RUN   TestCreateRunChatResumeInjectsProviderSession/uses_ProviderSessionExternalID_over_ProviderSessionID
--- PASS: ... (0.00s)
=== RUN   TestCreateRunChatResumeInjectsProviderSession/rejects_when_both_ProviderSessionExternalID_and_ProviderSessionID_are_blank
--- PASS: ... (0.00s)
...
PASS
ok  	github.com/superteam/control-plane/internal/employee	0.365s
```

Commit: `fix(employee): resume chat sessions from provider_session_external_id`

## Fix: resume command type

**Bug.** Chat follow-ups inject the prior turn's live provider session id into
`req.Metadata["provider_session_id"]` (`run_service.go` CreateRun resume
validation, ~line 197), but `dispatchStartSession` always dispatched runtime
command type `"start_session"` regardless. The runtime executor
(`apps/runtime-agent/src/commands/executor.rs:308-311`, not touched) only sets
`continue_session=true` for `ResumeSession | SendInput`, so a `start_session`
carrying an already-used session id made the runtime spawn
`claude --session-id <id>` as a *create*, and Claude rejected it: "Session ID
... is already in use."

**Runtime-acceptance evidence (verified before touching Go code).**
- `apps/runtime-agent/src/controlplane/models.rs:359-377`: manual
  `Deserialize` for `RuntimeCommandType` maps wire string `"resume_session"` →
  `RuntimeCommandType::ResumeSession` (alongside `"start_session"` →
  `StartSession`), so the control plane sending `"resume_session"` parses
  correctly — not `Unsupported`.
- `apps/runtime-agent/src/commands/executor.rs:215-218`: `StartSession |
  ResumeSession | SendInput` all route to the same `handle_input_command`
  handler — no divergent/skipped path for `ResumeSession`.
- `apps/runtime-agent/src/commands/executor.rs:308-311`: `continue_session:
  matches!(command.command_type, RuntimeCommandType::ResumeSession |
  RuntimeCommandType::SendInput)` — this is the exact switch the fix needs to
  flip on.
- `apps/runtime-agent/src/commands/payload.rs:390-394` and
  `executor.rs:944-951`: `ResumeSession` (like `StartSession`) requires a
  non-empty session id in the payload — already satisfied, since the chat
  resume path already injects `provider_session_id`.

Conclusion: `ResumeSession` is wire-parseable and flows through the identical
session-start handler; safe to switch command type without any runtime-agent
change.

**Fix (control-plane only).** `apps/control-plane/internal/employee/run_service.go`:
added `standaloneDispatchCommandType(req)` — returns `"resume_session"` when
`req.RunKind == RunKindChat` and `req.Metadata["provider_session_id"]` is a
non-empty string, else `"start_session"`. `dispatchStartSession` now uses this
for both the command-receipt `CommandType` and the dispatched
`runtimeCommand(...)` type, replacing the two hardcoded `"start_session"`
literals.

Scope: gated on `RunKind == RunKindChat`, so `StartProjectTaskRun`'s dispatch
(task-lineage resume via `FindProviderSessionForTaskRoot`, which also injects
`provider_session_id` but always dispatches with `RunKind == RunKindTask`) is
untouched and keeps the same latent bug — noted here, not fixed, per scope.

**Tests (TDD).** Extended `run_service_test.go`:
- `TestCreateRunChatResumeInjectsProviderSession`: asserts the dispatched
  command's `Type == "resume_session"` for all non-error cases (RED before
  the fix: got `"start_session"`; GREEN after).
- `TestCreateRunChatResolvesProjectAnchorNodeAndDispatches` (first chat
  message, no resume): asserts `Type == "start_session"` (unaffected).
- `TestRunServiceCreateRunDispatchesStartSession` (task-kind run): already
  asserted `"start_session"`, still passes.

`go test ./internal/employee/` (from `apps/control-plane`): PASS, no
regressions.

Commit: `fix(employee): dispatch chat resumes as resume_session commands`
