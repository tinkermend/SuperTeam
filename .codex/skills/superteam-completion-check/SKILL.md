---
name: superteam-completion-check
description: SuperTeam completion gate for truthfully verifying work before final answers, commits, merges, or claims that a task is complete. Use in /Users/tinker/src/singe/SuperTeam whenever finishing a feature, bugfix, refactor, merge, frontend change, Control Plane change, database migration, Runtime integration, approval/inbox/project coordination workflow, documentation update, or any task where Codex might say "done", "fixed", "真实测试过", "功能可用", or "已点通".
---

# SuperTeam Completion Check

Use this as the final gate before declaring SuperTeam work complete. The goal is to prevent mock-only validation, stale services, missing migrations, generated-code drift, UI regressions, missing changelog entries, and overclaiming.

## Non-Negotiable Real-Chain Gate

Real-chain verification is the default completion requirement for feature work, bug fixes, merges, frontend/backend integration, Runtime/Provider integration, database or migration changes, and any task where the final answer would imply "usable", "done", "fixed", "merged", "真实测试过", or "功能可用".

Do not turn missing real-chain verification into a harmless disclaimer. If real-chain verification is required and cannot be completed because services are down, auth is missing, an external Provider is unavailable, migrations are unsafe, or the environment is unclear, the task is blocked until the dependency is resolved or the human explicitly narrows the scope to local-only verification.

Unit tests, component tests, mock API tests, fake Provider scripts, typechecks, builds, and code review are supporting evidence only. They cannot replace real-chain verification for work that changes a user-visible, API-visible, cross-layer, Runtime/Provider, persistence, or merge result path.

## Workflow

1. Identify the actual change surface:
   - Web only
   - Control Plane/API only
   - database migration/read model
   - Runtime/Provider integration
   - cross-layer workflow such as inbox, approval, project coordination, or task dispatch
   - documentation, prototype, or design-only change

2. Separate verification types:
   - Unit/component tests prove local behavior only.
   - Mock API/browser tests prove UI behavior against a fake service only.
   - Build/typecheck prove compilation only.
   - Real-chain verification means the current running Web talks to the current running Control Plane and the target database/runtime dependency responds as expected.
   - For any real-chain-required surface, write down the real path that must be exercised before running local-only gates.
   - Low-risk local changes such as pure copy replacements, one-class visual alignment fixes, and design or constitution wording updates do not require service restarts, full test suites, browser verification, or real-chain smoke by default when they do not change interaction, data submission, routing, API contracts, state flow, authorization, persistence, or Runtime/Provider behavior.

3. Verify current code is what is running:
   - Inspect `scripts/dev-services.sh status` or process commands when local services are involved.
   - After merge, migration, generated code, or a 404/old behavior, restart only the relevant `scripts/dev-services.sh` managed service before real-chain testing.
   - For branch-finish work, verify the checkout and running services are on merged `main` before the post-merge smoke; feature-branch evidence does not prove the merged result.
   - Do not kill unmanaged user processes without inspecting them first.

4. Run the right gates:
   - Low-risk local change: use the smallest proof that matches the edit, typically `rg` before/after checks, `git diff --check`, and only the affected targeted test when an existing assertion or selector depends on the changed text. Do not expand to full tests, service restart, or browser/real-chain verification unless the change crosses into behavior, layout risk, data flow, or a claim that the feature path is usable.
   - Web change: targeted Vitest/browser test, typecheck when risk justifies it, and real browser verification against the running Web and real Control Plane for visible UI or data behavior. For in-app navigation changes, click the actual entry in Chrome plug/browser automation and confirm the result is a TanStack Router transition, not a document-level reload.
   - Control Plane change: targeted `go test` for the affected package and curl/API smoke against the running Control Plane with real auth when route behavior matters.
   - Contract change: regenerate OpenAPI output and run `pnpm verify:contracts`.
   - sqlc/schema change: run sqlc generation when queries/schema changed.
   - Database migration/read model: run migration status/apply against the intended development database and confirm the new table/column/index exists by status, schema inspect, or an equivalent read-only DB check.
   - Cross-layer workflow: verify at least one real path from UI or curl through Control Plane to the backing store, then confirm the page or API final state.
   - Runtime/Provider integration: verify local unit/fake-Provider behavior, then run a real Runtime/Provider smoke when claiming the execution path is usable. If a real Provider binary, credentials, or safe execution workspace is unavailable, stop as blocked instead of claiming completion.
   - Merge completion: after merging, rerun the relevant real-chain smoke on merged `main`, not only on the feature branch. For Web-visible flows, use the Chrome plug/browser automation against the running Web and real Control Plane, then record the page or API evidence.

5. Run the common project hygiene checks:
   - Add a `CHANGELOG.md` entry for completed feature work using `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`; do not hand-write the timestamp.
   - Keep generated files generated: after OpenAPI or sqlc source changes, run the generator instead of hand-editing generated outputs.
   - Keep documentation in Simplified Chinese unless the touched file has a stronger existing convention.
   - Confirm new files live inside the expected directory boundary, such as HTML prototypes under `docs/prototypes/`.
   - Run `git diff --check` before final response when files changed.
   - For branch-finish work, remove the branch worktree, prune worktrees, and delete the branch only after post-merge real-chain verification passes. If merge or verification is blocked, keep the branch/worktree available and report the blocker.

6. Report honestly:
   - If only mock/component/unit/build verification ran for work that does not require real-chain verification, explicitly say that real-chain verification was not done.
   - If real-chain verification ran, name the concrete service/API/page/database checks.
   - If any required real-chain verification is blocked, do not call the task complete; state the blocker and the exact command, service, auth, Provider, migration, or dependency needed.
   - Do not use `局部验证：...；未做真实链路验证，因为 ...` as a completion statement for real-chain-required work. Use `阻塞：...；尚不能声明完成` instead.

## Scope-Specific Checks

### Frontend and UI

- Read `DESIGN.md` before layout or style changes.
- For internal Web routes, use TanStack Router `Link` or `navigate`; reserve native `<a href>`, `window.location.href`, `location.assign`, or `location.replace` for external links, downloads, same-page anchors, or explicitly intended full-page navigation.
- Use real browser verification for visible UI, especially layout/style changes.
- For lists, tables, card workbenches, filters, sorting, pagination, or tabs, preserve existing data during background refresh; do not unload the main content when data already exists.
- For React Query queryKey changes, keep previous data unless the business explicitly requires clearing it.
- Preserve local UI state such as selected, expanded, and current view state across refetch; only fall back when the object no longer exists.
- Check that text does not overflow or overlap at relevant desktop/mobile widths.

### Control Plane, Contracts, and DB

- Respect `DATABASE_DESIGN.md` for schema, UUID-first modeling, tenant/team fields, indexes, migrations, sqlc, and OpenAPI.
- After `contracts/control-plane/openapi.yaml` changes, run `pnpm generate:control-plane` and `pnpm verify:contracts`.
- After sqlc query or schema changes, run the appropriate sqlc generation command; `pnpm generate:control-plane` is not a substitute.
- Validate migrations with `DATABASE_URL=... make -C apps/control-plane migrate-status` or `migrate-up` only against an intended database.
- Do not set `TEST_DATABASE_URL` or `TEST_REDIS_URL` unless the test database is confirmed safe to migrate and clean.

### Runtime, Provider, and Capability Integration

- Control Plane must not directly execute local commands.
- Runtime Agent owns node execution concerns, leases, provider processes, workspace/log/artifact handling, and slots; it must not own business policy or long-term platform state.
- Provider integration should go through the language-neutral `provider` contract before falling back to CLI, stdio, JSON stream, PTY, or HTTP adapters.
- External capability integration belongs in registration, authorization, HTTP invocation, and audit boundaries, not ad hoc business-core calls.

### Project, Approval, and Human Decision Workflows

- Keep Project as the business closure container; do not replace it with workflow templates.
- Keep the project coordinator as a virtual Temporal coordination thread, not a digital employee.
- Human responsibility, approval, rejection, evidence requests, reports, and acceptance decisions must remain first-class human decisions.
- Digital employees must not bypass human decisions or receive human management responsibilities as digital-employee capabilities.
- Agent collaboration facts should be structured objects with durable evidence, decisions, artifacts, or handoff packages, not free-form agent chat.
- Customer-specific differences should live in Tenant Profile, Connector, Semantic Mapping, Capability config, or Policy, not hard-coded core flow branches.

## Minimum Real-Chain Smoke

For SuperTeam cross-layer features, complete all of these before saying the feature is usable. If any item cannot be completed, the task is blocked unless the human explicitly narrows the task to local-only verification:

- Confirm Web's actual Control Plane URL.
- Confirm the backend route returns the expected non-5xx status with real auth if the route is authenticated.
- Confirm required migrations are applied and the database object exists when persistence changed.
- Open the real page in a browser when visible UI changed and check the final state is not stuck loading.
- Check logs or response bodies enough to distinguish success from silent fallback or mock data.
- For Runtime/Provider paths, confirm the Runtime Agent is running current code and execute at least one real Provider smoke when the claim is that the provider execution path works.

## Final Answer Contract

Include a short verification section or sentence with one of these shapes:

- `真实链路验证：...`
- `局部验证：...；未做真实链路验证，因为 ...`
- `阻塞：...；尚不能声明完成`

Never imply a broader verification scope than the commands and browser/API checks actually performed.
Use the `局部验证` shape only for explicitly local-only work. For real-chain-required work without real-chain evidence, use the `阻塞` shape.
