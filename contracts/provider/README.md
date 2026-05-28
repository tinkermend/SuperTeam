# Provider Contract

Provider contracts describe the boundary between `apps/runtime-agent` and concrete executors such as Claude Code, OpenCode, Codex, and Pi.

The Control Plane must not depend on provider-specific request shapes. Runtime adapters translate this neutral contract into CLI, PTY, HTTP, or SDK calls.

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
