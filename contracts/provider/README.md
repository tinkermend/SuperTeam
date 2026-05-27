# Provider Contract

Provider contracts describe the boundary between `apps/runtime-agent` and concrete executors such as Claude Code, OpenCode, Codex, and Pi.

The Control Plane must not depend on provider-specific request shapes. Runtime adapters translate this neutral contract into CLI, PTY, HTTP, or SDK calls.

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
