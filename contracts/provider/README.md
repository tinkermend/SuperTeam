# Provider Contract

Provider contracts describe the boundary between `apps/runtime-agent` and concrete executors such as Claude Code, OpenCode, and Codex.

The Control Plane must not depend on provider-specific request shapes. Runtime adapters translate this neutral contract into CLI, PTY, HTTP, or SDK calls.

## Design status

- Target architecture, L0–L4 layering, ErrorEnvelope / ProviderResult / Capability Matrix, phased delivery:
  - `docs/superpowers/specs/2026-08-09-provider-semantic-unification-design.md`
- Related debt / verify work: `docs/superpowers/specs/2026-07-19-runtime-provider-contract-verification.md` (P2).

### Phase 1 landed (error & terminal hardening)

- Runtime: `ErrorEnvelope` + `providers/error_map.rs` (`code → family/retryable` single table).
- Four terminal paths produce fail/complete writeback with `provider_terminal` attestation (stream `Err` synthesizes `turn_error` before fail).
- Claude `result.is_error` / `subtype` read correctly (no longer mis-classified as success).
- CP: `FailureFamilyBudgetFuse` routes to `waiting_human` + `budget_approval` with Chinese lead.
- Fail writeback `failure_family` / `retryable` are sourced from the envelope (not dialect `contains` on the main path).

### Phase 2 landed (contract freeze + goldens)

- `contracts/provider/schemas/*.json` + `failure-family.json` + fixtures; consumed by `scripts/verify-foundation-contracts.mjs`.
- Golden native→event samples under `golden/{claude-code,opencode,codex}/` (runtime `provider_golden_test.rs`).
- Web: `failureFamilyLabel` + guard on bare `failure_family` render.
- Catalog static `capability()` matrix (honest tool/structured_error flags).

### Phase 3 landed (capability honesty)

- Codex parser: explicit `type` branches only; unknown types → `native_unmapped` (no cross-type greedy keys).
- Parsers emit `native_unmapped` for unknown native types; CP writeback default **off** (`SUPERTEAM_PROVIDER_EMIT_NATIVE_UNMAPPED=1` to enable, max 20/attempt). Stream logs `unmapped_native` / `unparseable_line` diagnostics.
- Web execution trace: `stream_tools=false` providers show「摘要模式」and do not pretend to have tool trajectories.
- Onboarding checklist: `contracts/provider/ONBOARDING.md`.
- Activity labels: `turn_error` / `native_unmapped`.

### Phase 4 landed (error_code + ajv + start_session schema)

- Migration: `project_task_attempts.error_code` (nullable) + fail writeback / recovery / ledger / OpenAPI / Web attempt type.
- Root `ajv` validates provider fixtures in `verify:contracts` (S1).
- `start-session-payload.schema.json` + fixture; `CODEGEN.md` records **no** schema→type codegen this batch.
- Follow-up: ErrorEnvelope / attestation prefer registry `provider_type` over short `provider_kind` (kind kept one release).

### Still open

- Full `provider_kind` retirement; golden count target (≥5/provider); S2 ingest tagging; unmapped threshold alerts.
- Wire transport for process events remains `contracts/control-plane/openapi.yaml` (`payload`/`metadata` opaque objects).


The first Rust adapter baseline supports Claude Code and OpenCode through short-lived CLI processes per turn. Session continuity is represented by `ProviderSessionRef` and translated by the adapter:

- Claude Code: `claude -p <prompt> --output-format stream-json --verbose --include-partial-messages`, with `--session-id`, `--resume`, or `--continue` for session control.
- OpenCode: `opencode run --format json <prompt>`, with `--session` or `--continue` for session control.

Both adapters normalize provider JSON lines into the shared `ProviderEvent` stream and surface spawn failures, stderr, and non-zero exits as structured runtime errors.

## Baseline Objects

- `ProviderKind`: server-registered provider type string, not a closed business enum.
- `ProviderSessionRef`: provider session id plus workspace path managed by Runtime Agent.
- `ProviderInput`: task instruction, context slice refs, artifact refs, and execution limits.
- `ProviderEvent`: structured log, progress, artifact, blocker, decision request, or execution result.
- `ProviderResult`: final status, produced artifact refs, summary, and provider diagnostics.

## Baseline Commands

- `start(input) -> ProviderSessionRef`
- `stream(sessionRef) -> ProviderEvent[]`
- `cancel(sessionRef, reason)`
- `collect(sessionRef) -> ProviderResult`

Provider-specific capability details belong in adapter configuration and server-side registration, not in Control Plane business flow code.
