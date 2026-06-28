# Runtime Session Auth Self-Healing Design

## Context

Runtime Agent currently uses a two-layer credential model:

- `bootstrap_key` is the long-lived environment credential. It is used only to call `POST /api/v1/runtime/enrollments/hello` and obtain or refresh an approved runtime enrollment.
- `runtime_session_token` is the short-lived business credential. It authorizes heartbeat, task claim, lease renewal, command websocket, provider writeback, and capability updates.

The original runtime enrollment design intentionally made runtime sessions short-lived: the Runtime Agent should not persist a long-lived business token, and Control Plane operators should be able to revoke runtime enrollments or sessions. The current implementation only renews the active session before expiry. If renewal misses the valid window or the token is invalidated, cloned clients and websocket loops keep using the old token and repeatedly receive `401 Unauthorized`.

The fix is not to wait for 401 and then recover. The normal path must keep every business request away from the expiry boundary. A 401 handler is only an emergency fallback.

## Goals

- Keep the short-lived runtime session model.
- Prevent normal heartbeat, claim, websocket, lease renewal, and writeback requests from hitting an expired token.
- Refresh the runtime session before it enters an unsafe expiry window.
- Use `bootstrap_key` to obtain a fresh session when renewal cannot keep the session safe.
- Treat `401 invalid runtime session` and `401 invalid runtime authentication` as auth-state signals, not generic retryable transport errors.
- Let active provider tasks continue across token refresh whenever the runtime enrollment is still approved.
- Stop old session loops from continuing to use stale tokens after the auth state moves to a new session.

## Non-Goals

- Do not remove `runtime_sessions.expires_at`.
- Do not turn `runtime_session_token` into a long-lived credential.
- Do not make `bootstrap_key` authorize business endpoints such as task claim or provider writeback.
- Do not add UI changes in this slice.
- Do not make `scripts/dev-services.sh status` part of the core fix.

## Recommended Approach

Use a session supervisor plus shared auth state.

The Runtime Agent owns one `RuntimeAuthState` shared by all Control Plane clients. Each request reads the latest session token from that state instead of storing a fixed token string inside every cloned client. A daemon-level supervisor keeps the auth state fresh:

1. Enroll with `bootstrap_key`.
2. Store the issued session id, token, and expiry in shared auth state.
3. Start runtime loops for heartbeat, task polling, lease renewal, websocket commands, and execution.
4. Renew the session before the safety window.
5. If renewal succeeds, update expiry in shared auth state.
6. If renewal cannot keep the token safe, pause new business intake and re-enroll with `bootstrap_key`.
7. If any business endpoint returns runtime auth 401, mark the current session generation invalid and re-enroll.

This keeps 401 recovery as a fallback while the steady state is proactive refresh.

## Auth State

`RuntimeAuthState` should be a small shared structure with interior synchronization:

- `node_id`
- `session_id`
- `session_token`
- `expires_at`
- `generation`
- `status`: `Connected`, `Refreshing`, `Reauthenticating`, `PendingApproval`, `Shutdown`

The `generation` prevents stale loops from mutating state after a new session is established. A loop that observes an auth error reports it with the generation it was using. The supervisor ignores reports from older generations.

`ControlPlaneClient` should hold an auth provider instead of a copied token. For runtime-authenticated requests it asks the provider for the current token and node id immediately before sending the request. Bootstrap requests still use an unauthenticated client.

## Refresh Policy

The refresh loop has two thresholds:

- Refresh window: start renewal well before expiry, for example at 75 percent of TTL elapsed or at least 30 minutes before expiry for a 12 hour session.
- Safety window: stop using the old token for new business intake when expiry is too close, for example 2 minutes before expiry.

Within the refresh window:

- First try `POST /api/v1/runtime/sessions/{id}/renew`.
- On success, update `expires_at`.
- On network or 5xx failure, retry with bounded backoff while the token remains outside the safety window.

At or inside the safety window:

- Pause new task claims and websocket reconnects.
- Re-enroll with `bootstrap_key` to obtain a fresh session.
- Keep active provider tasks running locally when possible.
- Resume business traffic once the shared auth state contains the fresh token.

## 401 Fallback

Runtime auth 401 is a session health signal:

- `invalid runtime session`
- `invalid runtime authentication`
- missing or rejected runtime session token on runtime-only endpoints

When one of these occurs:

1. The client returns a typed `RuntimeAuthExpired` or equivalent error.
2. The loop reports the auth error to the supervisor with its generation.
3. The supervisor marks the generation invalid.
4. The supervisor pauses new business intake.
5. The supervisor re-enrolls with `bootstrap_key`.
6. The shared auth state is updated with the new session.
7. Loops resume with the new token.

If the enrollment is no longer approved, the runtime should stop business traffic and periodically retry hello. It should not keep claiming tasks or opening websockets with the stale token.

## Loop Behavior

Heartbeat:

- Uses current auth state for every request.
- During `Refreshing`, it may continue if the token is outside the safety window.
- During `Reauthenticating` or `PendingApproval`, it pauses authenticated heartbeat and lets the supervisor own hello retries.

Task polling:

- Must pause inside the safety window and during re-authentication.
- Existing queued tasks can remain queued, but no new tasks should be claimed until auth is safe.

Execution and writeback:

- Active provider processes should not be killed solely because auth is refreshing.
- Writebacks, lease renewal, and event upload should read the latest token at send time.
- If writeback hits runtime auth 401, report auth invalid and retry after re-auth where the operation is idempotent. Terminal writebacks must preserve existing idempotency semantics.

Command websocket:

- Opens with the current token.
- On auth handshake failure, reports runtime auth invalid instead of reconnecting forever with the same Authorization header.
- Reconnects only after the supervisor has safe auth state.

Session renewal:

- No longer runs as an isolated fire-and-forget task with a copied client.
- It belongs to the supervisor and updates shared auth state directly.

## Control Plane Semantics

Control Plane can keep the current short-lived session model:

- New sessions are still issued by approved `enrollments/hello`.
- Session renewal still requires a valid active session.
- Runtime endpoints still reject expired or revoked sessions.
- Bootstrap key remains unable to access business endpoints.

The main Control Plane change is optional but useful: keep runtime auth failure bodies stable enough for the Runtime Agent to classify them. If the error body changes, the status code plus route context should still allow typed auth handling.

## Failure Handling

Network failure before safety window:

- Retry renewal with backoff.
- Continue existing business traffic while token remains safe.

Network failure inside safety window:

- Pause new business traffic.
- Keep active provider tasks running locally.
- Retry renewal or re-enrollment until auth is safe or shutdown occurs.

Enrollment revoked or rejected:

- Stop authenticated runtime business loops.
- Do not kill provider processes immediately unless active writeback can no longer be made safe after a timeout.
- Surface logs that distinguish pending approval, revoked enrollment, invalid bootstrap key, and transient Control Plane failure.

Invalid bootstrap key:

- Stop business traffic.
- Retry slowly or exit with a clear error, depending on existing daemon conventions.

## Testing Strategy

Unit and integration-style Rust tests:

- A shared auth client uses the newest token after `RuntimeAuthState` is updated.
- Renewal success updates expiry without restarting the daemon.
- Renewal 401 triggers re-enrollment.
- Heartbeat 401 triggers re-enrollment.
- Task claim 401 triggers re-enrollment and pauses further claim attempts until a new session exists.
- Websocket auth failure does not reconnect forever with the stale token.
- Active task writeback uses a refreshed token after the auth state changes.
- Pending enrollment stops business loops and retries hello instead of using the old token.

Control Plane tests:

- Runtime session TTL remains enforced.
- Expired session renewal is rejected.
- Approved hello can issue a fresh session for the same node after an old session expires.
- Revoked enrollment prevents hello from issuing a usable business session.

Real verification:

- Start the local stack.
- Shorten session TTL in a test-only path or use a controlled test server to force expiry behavior.
- Confirm Runtime Agent renews before expiry without any heartbeat, claim, or websocket 401 in the normal path.
- Force a runtime auth 401 and confirm the agent re-enrolls and resumes with a new session.
- Run one real Runtime/Provider smoke after re-auth to prove task execution and writeback still work.

## Acceptance Criteria

- A long-running Runtime Agent remains available past the runtime session TTL without manual restart.
- Normal renewal does not produce 401s on heartbeat, claim, websocket, lease renewal, or writeback.
- Forced session invalidation causes automatic re-authentication through `bootstrap_key`.
- After re-authentication, all runtime business requests use the new token.
- Existing active provider tasks are not cancelled solely because the runtime token refreshed.
- Revocation still works: revoked enrollment or session cannot continue business access.
