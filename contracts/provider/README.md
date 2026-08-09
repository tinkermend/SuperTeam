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

### Still open (later phases)

- Phase 3: codex greedy-fallback tighten, `native_unmapped` observability, UI honesty for `stream_tools=false`.
- Phase 4: schema codegen, optional `error_code` column for alerting stats.
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
