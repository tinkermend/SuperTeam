# Runtime Session Auth Self-Healing Implementation Plan
> 复核状态：已实现（2026-06-28完成）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep Runtime Agent business traffic on a valid runtime session by refreshing before expiry, re-enrolling with `bootstrap_key` when needed, and treating runtime auth 401 as a fallback signal instead of the normal refresh path.

**Architecture:** Add a shared `RuntimeAuthState` that every Runtime Control Plane client reads at request time. A daemon-level session supervisor owns hello, renewal, safety-window pausing, and re-authentication. Existing heartbeat, task claim, websocket, lease, and writeback loops keep running, but they block on safe credentials and automatically use the newest token.

**Tech Stack:** Rust 2024, `tokio`, `reqwest`, `tokio-tungstenite`, `anyhow`, existing Go Control Plane runtime session APIs, `corepack pnpm verify:runtime-agent`.

---

## File Structure

- Create `apps/runtime-agent/src/runtime_auth.rs`
  - Owns shared auth snapshot, business credential waiting, renewal credential access, generation tracking, safety-window checks, and typed auth errors.
- Modify `apps/runtime-agent/src/lib.rs`
  - Exports the new `runtime_auth` module.
- Modify `apps/runtime-agent/src/controlplane/client.rs`
  - Adds shared-auth client construction, runtime-auth error classification, and request-time credential lookup.
- Modify `apps/runtime-agent/src/controlplane/ws.rs`
  - Uses shared auth for websocket Authorization headers and reports handshake 401 as runtime auth expiry.
- Modify `apps/runtime-agent/src/daemon.rs`
  - Replaces fire-and-forget renewal with a session supervisor that enrolls, renews, re-enrolls, and updates `RuntimeAuthState`.
- Modify `apps/runtime-agent/src/executor/loops.rs`
  - Keeps task polling behavior simple, relying on shared auth to pause when credentials are unsafe. Adjust error logging so auth pauses are not noisy retry loops.
- Modify `apps/runtime-agent/tests/controlplane_client_test.rs`
  - Covers request-time token replacement and typed runtime auth 401.
- Modify `apps/runtime-agent/tests/daemon_test.rs`
  - Covers renewal 401 re-enrollment, proactive renewal before safety window, and pending enrollment behavior.
- Modify `apps/runtime-agent/src/controlplane/ws.rs` tests
  - Covers websocket handshake 401 reporting rather than stale-token reconnect.

## Task 1: Add Shared Runtime Auth State

**Files:**
- Create: `apps/runtime-agent/src/runtime_auth.rs`
- Modify: `apps/runtime-agent/src/lib.rs`
- Test: `apps/runtime-agent/tests/controlplane_client_test.rs`

- [ ] **Step 1: Write failing tests for request-time credential updates**

Add this test to `apps/runtime-agent/tests/controlplane_client_test.rs` near the existing identity header tests:

```rust
#[tokio::test]
async fn shared_runtime_auth_client_uses_latest_token_for_each_request() {
    use superteam_runtime_agent::runtime_auth::RuntimeAuthState;

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, mut request_rx) = tokio::sync::mpsc::channel(2);

    tokio::spawn(async move {
        for _ in 0..2 {
            let (mut socket, _) = listener.accept().await.unwrap();
            let request = read_http_request(&mut socket).await;
            request_tx.send(request).await.unwrap();
            socket
                .write_all(b"HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
                .await
                .unwrap();
        }
    });

    let auth = RuntimeAuthState::new("node-1");
    auth.set_session(
        "session-1",
        "token-one",
        "2999-06-02T00:00:00Z",
    )
    .await
    .expect("initial session");
    let client = ControlPlaneClient::with_runtime_auth(format!("http://{}", addr), auth.clone());

    client.claim_task(1).await.unwrap();
    auth.set_session(
        "session-2",
        "token-two",
        "2999-06-03T00:00:00Z",
    )
    .await
    .expect("refreshed session");
    client.claim_task(1).await.unwrap();

    let first = request_rx.recv().await.expect("first request");
    let second = request_rx.recv().await.expect("second request");
    assert!(first.contains("authorization: Bearer token-one"));
    assert!(second.contains("authorization: Bearer token-two"));
    assert!(first.contains("x-node-id: node-1"));
    assert!(second.contains("x-node-id: node-1"));
}
```

- [ ] **Step 2: Run the failing test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml shared_runtime_auth_client_uses_latest_token_for_each_request --test controlplane_client_test -- --nocapture
```

Expected: FAIL because `runtime_auth` and `ControlPlaneClient::with_runtime_auth` do not exist.

- [ ] **Step 3: Implement `RuntimeAuthState`**

Create `apps/runtime-agent/src/runtime_auth.rs` with:

```rust
use std::fmt;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use reqwest::StatusCode;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};
use tokio::sync::{Notify, RwLock};

pub const DEFAULT_AUTH_REFRESH_MARGIN: Duration = Duration::from_secs(30 * 60);
pub const DEFAULT_AUTH_SAFETY_WINDOW: Duration = Duration::from_secs(2 * 60);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RuntimeAuthStatus {
    Empty,
    Connected,
    Refreshing,
    Reauthenticating,
    PendingApproval,
    Shutdown,
}

#[derive(Debug, Clone)]
pub struct RuntimeCredentials {
    pub node_id: String,
    pub session_id: String,
    pub token: String,
    pub expires_at: OffsetDateTime,
    pub generation: u64,
}

#[derive(Debug, Clone)]
pub struct RuntimeAuthSnapshot {
    pub node_id: String,
    pub session_id: Option<String>,
    pub token: Option<String>,
    pub expires_at: Option<OffsetDateTime>,
    pub generation: u64,
    pub status: RuntimeAuthStatus,
}

#[derive(Clone)]
pub struct RuntimeAuthState {
    inner: Arc<RuntimeAuthInner>,
}

struct RuntimeAuthInner {
    node_id: String,
    state: RwLock<RuntimeAuthSnapshot>,
    changed: Notify,
    safety_window: Duration,
    refresh_margin: Duration,
}

#[derive(Debug, Clone)]
pub struct RuntimeAuthExpired {
    pub operation: &'static str,
    pub status: StatusCode,
    pub body: String,
    pub generation: Option<u64>,
}

impl fmt::Display for RuntimeAuthExpired {
    fn fmt(&self, formatter: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            formatter,
            "{} failed with runtime auth error {}: {}",
            self.operation, self.status, self.body
        )
    }
}

impl std::error::Error for RuntimeAuthExpired {}

impl RuntimeAuthState {
    pub fn new(node_id: impl Into<String>) -> Self {
        Self::with_windows(
            node_id,
            DEFAULT_AUTH_REFRESH_MARGIN,
            DEFAULT_AUTH_SAFETY_WINDOW,
        )
    }

    pub fn with_windows(
        node_id: impl Into<String>,
        refresh_margin: Duration,
        safety_window: Duration,
    ) -> Self {
        let node_id = node_id.into();
        Self {
            inner: Arc::new(RuntimeAuthInner {
                state: RwLock::new(RuntimeAuthSnapshot {
                    node_id: node_id.clone(),
                    session_id: None,
                    token: None,
                    expires_at: None,
                    generation: 0,
                    status: RuntimeAuthStatus::Empty,
                }),
                node_id,
                changed: Notify::new(),
                safety_window,
                refresh_margin,
            }),
        }
    }

    pub async fn snapshot(&self) -> RuntimeAuthSnapshot {
        self.inner.state.read().await.clone()
    }

    pub async fn set_session(
        &self,
        session_id: impl Into<String>,
        token: impl Into<String>,
        expires_at: impl AsRef<str>,
    ) -> Result<RuntimeCredentials> {
        let expires_at = OffsetDateTime::parse(expires_at.as_ref(), &Rfc3339)
            .context("runtime session expires_at is not RFC3339")?;
        let mut state = self.inner.state.write().await;
        state.session_id = Some(session_id.into());
        state.token = Some(token.into());
        state.expires_at = Some(expires_at);
        state.generation += 1;
        state.status = RuntimeAuthStatus::Connected;
        let credentials = credentials_from_snapshot(&state)?;
        drop(state);
        self.inner.changed.notify_waiters();
        Ok(credentials)
    }

    pub async fn mark_status(&self, status: RuntimeAuthStatus) {
        let mut state = self.inner.state.write().await;
        state.status = status;
        drop(state);
        self.inner.changed.notify_waiters();
    }

    pub async fn report_auth_failure(&self, generation: Option<u64>) {
        let mut state = self.inner.state.write().await;
        if generation.is_none() || generation == Some(state.generation) {
            state.status = RuntimeAuthStatus::Reauthenticating;
        }
        drop(state);
        self.inner.changed.notify_waiters();
    }

    pub async fn wait_for_business_credentials(&self) -> Result<RuntimeCredentials> {
        loop {
            let snapshot = self.snapshot().await;
            if matches!(
                snapshot.status,
                RuntimeAuthStatus::Connected | RuntimeAuthStatus::Refreshing
            ) && snapshot.expires_at.is_some_and(|expires_at| {
                expires_at - time::Duration::seconds(self.inner.safety_window.as_secs() as i64)
                    > OffsetDateTime::now_utc()
            }) {
                return credentials_from_snapshot(&snapshot);
            }
            if snapshot.status == RuntimeAuthStatus::Shutdown {
                anyhow::bail!("runtime auth is shut down");
            }
            self.inner.changed.notified().await;
        }
    }

    pub async fn renewal_credentials(&self) -> Result<RuntimeCredentials> {
        credentials_from_snapshot(&self.snapshot().await)
    }

    pub async fn wait_until_reauthenticating(&self) {
        loop {
            if self.snapshot().await.status == RuntimeAuthStatus::Reauthenticating {
                return;
            }
            self.inner.changed.notified().await;
        }
    }

    pub fn refresh_margin(&self) -> Duration {
        self.inner.refresh_margin
    }

    pub fn safety_window(&self) -> Duration {
        self.inner.safety_window
    }

    pub fn node_id(&self) -> &str {
        &self.inner.node_id
    }
}

pub fn credentials_from_snapshot(snapshot: &RuntimeAuthSnapshot) -> Result<RuntimeCredentials> {
    Ok(RuntimeCredentials {
        node_id: snapshot.node_id.clone(),
        session_id: snapshot
            .session_id
            .clone()
            .context("runtime session id is not available")?,
        token: snapshot
            .token
            .clone()
            .context("runtime session token is not available")?,
        expires_at: snapshot
            .expires_at
            .context("runtime session expiry is not available")?,
        generation: snapshot.generation,
    })
}

pub fn is_runtime_auth_failure(status: StatusCode, body: &str) -> bool {
    status == StatusCode::UNAUTHORIZED
        && (body.contains("invalid runtime session")
            || body.contains("invalid runtime authentication")
            || body.contains("runtime session token"))
}

pub fn is_runtime_auth_expired(error: &(dyn std::error::Error + 'static)) -> bool {
    error.downcast_ref::<RuntimeAuthExpired>().is_some()
}
```

- [ ] **Step 4: Export the module**

Add this line to `apps/runtime-agent/src/lib.rs`:

```rust
pub mod runtime_auth;
```

- [ ] **Step 5: Add shared-auth construction and request-time credential lookup to the client**

In `apps/runtime-agent/src/controlplane/client.rs`, change the client struct to hold an auth mode:

```rust
use anyhow::{Context, Result, anyhow};
use reqwest::{Client, StatusCode};
use std::time::Duration;

use crate::runtime_auth::{
    RuntimeAuthExpired, RuntimeAuthState, RuntimeCredentials, is_runtime_auth_failure,
};

#[derive(Clone)]
enum RuntimeAuthMode {
    None,
    Static { token: String, node_id: Option<String> },
    Shared(RuntimeAuthState),
}

#[derive(Clone)]
pub struct ControlPlaneClient {
    base_url: String,
    auth: RuntimeAuthMode,
    client: Client,
}
```

Update constructors:

```rust
pub fn new(base_url: impl Into<String>, token: impl Into<String>) -> Self {
    let token = token.into();
    let auth = if token.is_empty() {
        RuntimeAuthMode::None
    } else {
        RuntimeAuthMode::Static {
            token,
            node_id: None,
        }
    };
    Self::with_auth(base_url, auth)
}

fn with_auth(base_url: impl Into<String>, auth: RuntimeAuthMode) -> Self {
    let client = Client::builder()
        .timeout(Duration::from_secs(65))
        .build()
        .expect("Failed to build HTTP client");

    Self {
        base_url: base_url.into(),
        auth,
        client,
    }
}

pub fn with_node_id(
    base_url: impl Into<String>,
    token: impl Into<String>,
    node_id: impl Into<String>,
) -> Self {
    Self::with_auth(
        base_url,
        RuntimeAuthMode::Static {
            token: token.into(),
            node_id: Some(node_id.into()),
        },
    )
}

pub fn with_session_token(
    base_url: impl Into<String>,
    token: impl Into<String>,
    node_id: impl Into<String>,
) -> Self {
    Self::with_node_id(base_url, token, node_id)
}

pub fn with_runtime_auth(base_url: impl Into<String>, auth: RuntimeAuthState) -> Self {
    Self::with_auth(base_url, RuntimeAuthMode::Shared(auth))
}
```

Add helpers:

```rust
async fn runtime_credentials(&self) -> Result<RuntimeCredentials> {
    match &self.auth {
        RuntimeAuthMode::Shared(auth) => auth.wait_for_business_credentials().await,
        RuntimeAuthMode::Static { token, node_id } => Ok(RuntimeCredentials {
            node_id: node_id
                .clone()
                .filter(|node_id| !node_id.trim().is_empty())
                .context("Runtime node_id is required for authenticated Runtime API requests")?,
            session_id: String::new(),
            token: token.clone(),
            expires_at: time::OffsetDateTime::UNIX_EPOCH,
            generation: 0,
        }),
        RuntimeAuthMode::None => anyhow::bail!(
            "runtime auth is required for authenticated Runtime API requests"
        ),
    }
}

async fn renewal_credentials(&self) -> Result<RuntimeCredentials> {
    match &self.auth {
        RuntimeAuthMode::Shared(auth) => auth.renewal_credentials().await,
        _ => self.runtime_credentials().await,
    }
}

async fn runtime_headers_for(node_id: &str) -> Result<reqwest::header::HeaderMap> {
    let mut headers = reqwest::header::HeaderMap::new();
    headers.insert(
        "X-Node-ID",
        reqwest::header::HeaderValue::from_str(node_id)
            .context("Runtime node_id is not a valid header value")?,
    );
    Ok(headers)
}

async fn runtime_error(
    &self,
    operation: &'static str,
    status: StatusCode,
    body: String,
    generation: Option<u64>,
) -> anyhow::Error {
    if is_runtime_auth_failure(status, &body) {
        if let RuntimeAuthMode::Shared(auth) = &self.auth {
            auth.report_auth_failure(generation).await;
        }
        RuntimeAuthExpired {
            operation,
            status,
            body,
            generation,
        }
        .into()
    } else {
        anyhow!("{} failed with status {}: {}", operation, status, body)
    }
}
```

Then update each runtime-authenticated request to use:

```rust
let auth = self.runtime_credentials().await?;
let response = self
    .client
    .post(&url)
    .bearer_auth(&auth.token)
    .headers(runtime_headers_for(&auth.node_id).await?)
    .send()
    .await
    .context("Failed to send claim task request")?;
```

For `renew_session`, use `self.renewal_credentials().await?` instead of `runtime_credentials()`.

For every non-success response, replace `anyhow::bail!(...)` with:

```rust
let status = response.status();
let body = response.text().await.unwrap_or_default();
return Err(self.runtime_error("Claim task", status, body, Some(auth.generation)).await);
```

Use operation names matching existing methods: `Runtime session renew`, `Runtime capabilities upsert`, `Heartbeat`, `Claim task`, `Update task status`, `Push event`, `Complete task`, `Fail task`, `Complete runtime command`, `Record runtime command event`, `Fail runtime command`, `Cancel runtime command`, `Complete project task attempt`, `Start project task attempt`, `Fail project task attempt`, `Wait-human project task attempt`, and `Renew lease`.

- [ ] **Step 6: Run the new test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml shared_runtime_auth_client_uses_latest_token_for_each_request --test controlplane_client_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 7: Run existing client tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test controlplane_client_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 8: Commit Task 1**

```bash
git add apps/runtime-agent/src/runtime_auth.rs apps/runtime-agent/src/lib.rs apps/runtime-agent/src/controlplane/client.rs apps/runtime-agent/tests/controlplane_client_test.rs
git commit -m "feat(runtime-agent): add shared runtime auth state"
```

## Task 2: Add Typed Runtime Auth 401 Classification Coverage

**Files:**
- Modify: `apps/runtime-agent/tests/controlplane_client_test.rs`
- Modify: `apps/runtime-agent/src/controlplane/client.rs`

- [ ] **Step 1: Write failing tests for typed auth errors**

Add this test to `apps/runtime-agent/tests/controlplane_client_test.rs`:

```rust
#[tokio::test]
async fn controlplane_client_classifies_runtime_auth_unauthorized() {
    use superteam_runtime_agent::runtime_auth::{RuntimeAuthState, is_runtime_auth_expired};

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    tokio::spawn(async move {
        let (mut socket, _) = listener.accept().await.unwrap();
        let _request = read_http_request(&mut socket).await;
        socket
            .write_all(
                b"HTTP/1.1 401 Unauthorized\r\nContent-Length: 30\r\n\r\ninvalid runtime authentication",
            )
            .await
            .unwrap();
    });

    let auth = RuntimeAuthState::new("node-1");
    auth.set_session("session-1", "expired-token", "2999-06-02T00:00:00Z")
        .await
        .expect("session");
    let client = ControlPlaneClient::with_runtime_auth(format!("http://{}", addr), auth.clone());

    let error = client.claim_task(1).await.expect_err("claim should fail");
    assert!(is_runtime_auth_expired(error.as_ref()));
    assert_eq!(
        auth.snapshot().await.status,
        superteam_runtime_agent::runtime_auth::RuntimeAuthStatus::Reauthenticating
    );
}
```

- [ ] **Step 2: Run the failing test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml controlplane_client_classifies_runtime_auth_unauthorized --test controlplane_client_test -- --nocapture
```

Expected before Task 1 completion: FAIL. Expected after Task 1 completion: PASS. If it fails after Task 1, fix `runtime_error` so 401 bodies containing `invalid runtime authentication` create `RuntimeAuthExpired` and mark shared auth as `Reauthenticating`.

- [ ] **Step 3: Run targeted tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test controlplane_client_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 4: Commit Task 2**

```bash
git add apps/runtime-agent/src/controlplane/client.rs apps/runtime-agent/tests/controlplane_client_test.rs
git commit -m "test(runtime-agent): classify runtime auth failures"
```

## Task 3: Replace Fire-and-Forget Renewal with Session Supervisor

**Files:**
- Modify: `apps/runtime-agent/src/daemon.rs`
- Modify: `apps/runtime-agent/tests/daemon_test.rs`

- [ ] **Step 1: Write failing test for renewal 401 re-enrollment**

Replace the old `daemon_runtime_session_renewal_loop_attempts_renew_before_expiry` focus with a supervisor test in `apps/runtime-agent/tests/daemon_test.rs`:

```rust
#[tokio::test]
async fn session_supervisor_reenrolls_when_renew_returns_runtime_auth_401() {
    use superteam_runtime_agent::runtime_auth::RuntimeAuthState;

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, mut request_rx) = tokio::sync::mpsc::channel(4);

    tokio::spawn(async move {
        let (mut renew_socket, _) = listener.accept().await.unwrap();
        let renew_request = read_http_request(&mut renew_socket).await;
        request_tx.send(renew_request).await.unwrap();
        write_status_response(
            &mut renew_socket,
            "401 Unauthorized",
            serde_json::json!("invalid runtime session"),
        )
        .await;

        let (mut hello_socket, _) = listener.accept().await.unwrap();
        let hello_request = read_http_request(&mut hello_socket).await;
        request_tx.send(hello_request).await.unwrap();
        write_json_response(
            &mut hello_socket,
            serde_json::json!({
                "enrollment": {
                    "id": "11111111-1111-4111-8111-111111111111",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "bootstrap_key_id": "44444444-4444-4444-8444-444444444444",
                    "status": "approved",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session": {
                    "id": "66666666-6666-4666-8666-666666666666",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "enrollment_id": "11111111-1111-4111-8111-111111111111",
                    "expires_at": "2999-06-03T00:00:00Z",
                    "last_seen_at": "2026-06-02T00:00:00Z",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session_token": "fresh-session-token"
            }),
        )
        .await;
    });

    let mut config = RuntimeConfig::new("node-1").expect("config");
    config.runtime.control_plane_url = format!("http://{}", addr);
    config.runtime.bootstrap_key = "bootstrap-key".to_string();

    let auth = RuntimeAuthState::new("node-1");
    auth.set_session("old-session", "old-token", "1970-01-01T00:00:00Z")
        .await
        .expect("old session");
    superteam_runtime_agent::daemon::renew_or_reenroll_once(&config, auth.clone(), vec![])
        .await
        .expect("renew or re-enroll");

    let renew_request = request_rx.recv().await.expect("renew request");
    let hello_request = request_rx.recv().await.expect("hello request");
    assert_eq!(
        renew_request.lines().next().unwrap(),
        "POST /api/v1/runtime/sessions/old-session/renew HTTP/1.1"
    );
    assert!(renew_request.contains("authorization: Bearer old-token"));
    assert_eq!(
        hello_request.lines().next().unwrap(),
        "POST /api/v1/runtime/enrollments/hello HTTP/1.1"
    );

    let snapshot = auth.snapshot().await;
    assert_eq!(snapshot.session_id.as_deref(), Some("66666666-6666-4666-8666-666666666666"));
    assert_eq!(snapshot.token.as_deref(), Some("fresh-session-token"));
}
```

- [ ] **Step 2: Run the failing test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml session_supervisor_reenrolls_when_renew_returns_runtime_auth_401 --test daemon_test -- --nocapture
```

Expected: FAIL because `renew_or_reenroll_once` does not exist.

- [ ] **Step 3: Add supervisor helpers in `daemon.rs`**

Add imports:

```rust
use crate::runtime_auth::{RuntimeAuthState, RuntimeAuthStatus, is_runtime_auth_expired};
```

Add these helper functions near the current session renewal functions:

```rust
pub async fn establish_runtime_auth_session(
    config: &RuntimeConfig,
    capabilities: Vec<RuntimeCapabilityInput>,
    auth: RuntimeAuthState,
) -> Result<bool> {
    let bootstrap_client = ControlPlaneClient::new(&config.runtime.control_plane_url, "");
    let supported_providers = build_supported_providers(config);
    let hello = bootstrap_client
        .enroll_hello(EnrollHelloRequest {
            node_id: config.runtime.node_id.clone(),
            name: format!("runtime-{}", config.runtime.node_id),
            supported_providers,
            max_slots: config.runtime.max_concurrent_tasks as i32,
            bootstrap_key: config.runtime.bootstrap_key.clone(),
            version: Some(env!("CARGO_PKG_VERSION").to_string()),
            metadata: Some(runtime_metadata()),
            capabilities: capabilities.clone(),
        })
        .await?;

    if hello.enrollment.status != EnrollmentStatus::Approved {
        auth.mark_status(RuntimeAuthStatus::PendingApproval).await;
        println!(
            "runtime-agent node={} enrollment={}; waiting for approval",
            config.runtime.node_id,
            enrollment_status_label(&hello.enrollment.status)
        );
        return Ok(false);
    }

    let session_response = hello
        .session
        .as_ref()
        .context("approved runtime enrollment did not return a session")?;
    let session_token = hello
        .session_token
        .as_deref()
        .filter(|token| !token.trim().is_empty())
        .context("approved runtime enrollment did not return a session token")?;

    auth.set_session(
        session_response.id.clone(),
        session_token.to_string(),
        &session_response.expires_at,
    )
    .await?;

    let client = ControlPlaneClient::with_runtime_auth(
        config.runtime.control_plane_url.clone(),
        auth.clone(),
    );
    client
        .upsert_capabilities(&config.runtime.node_id, capabilities)
        .await?;

    println!("Runtime session established");
    Ok(true)
}

pub async fn renew_or_reenroll_once(
    config: &RuntimeConfig,
    auth: RuntimeAuthState,
    capabilities: Vec<RuntimeCapabilityInput>,
) -> Result<()> {
    auth.mark_status(RuntimeAuthStatus::Refreshing).await;
    let client = ControlPlaneClient::with_runtime_auth(
        config.runtime.control_plane_url.clone(),
        auth.clone(),
    );
    let credentials = auth.renewal_credentials().await?;
    match client.renew_session(&credentials.session_id).await {
        Ok(session) => {
            auth.set_session(credentials.session_id, credentials.token, session.expires_at)
                .await?;
            Ok(())
        }
        Err(error) if is_runtime_auth_expired(error.as_ref()) => {
            auth.mark_status(RuntimeAuthStatus::Reauthenticating).await;
            establish_runtime_auth_session(config, capabilities, auth).await?;
            Ok(())
        }
        Err(error) => {
            auth.mark_status(RuntimeAuthStatus::Connected).await;
            Err(error)
        }
    }
}
```

Keep the old `connect_runtime_session` wrapper temporarily by implementing it with the new auth state:

```rust
pub async fn connect_runtime_session(
    config: &RuntimeConfig,
    capabilities: Vec<RuntimeCapabilityInput>,
) -> Result<Option<ControlPlaneClient>> {
    let auth = RuntimeAuthState::new(config.runtime.node_id.clone());
    if establish_runtime_auth_session(config, capabilities, auth.clone()).await? {
        Ok(Some(ControlPlaneClient::with_runtime_auth(
            config.runtime.control_plane_url.clone(),
            auth,
        )))
    } else {
        Ok(None)
    }
}
```

- [ ] **Step 4: Run the supervisor test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml session_supervisor_reenrolls_when_renew_returns_runtime_auth_401 --test daemon_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 5: Run daemon tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test daemon_test -- --nocapture
```

Expected: PASS. If the old renewal test races with the new supervisor, replace it with a direct `renew_or_reenroll_once` success-path test that asserts a 200 renew updates `expires_at` without calling hello.

- [ ] **Step 6: Commit Task 3**

```bash
git add apps/runtime-agent/src/daemon.rs apps/runtime-agent/tests/daemon_test.rs
git commit -m "feat(runtime-agent): re-enroll after invalid runtime session"
```

## Task 4: Wire the Daemon Runtime to Shared Auth and Supervisor

**Files:**
- Modify: `apps/runtime-agent/src/daemon.rs`
- Modify: `apps/runtime-agent/src/controlplane/ws.rs`
- Modify: `apps/runtime-agent/src/executor/loops.rs`
- Test: `apps/runtime-agent/tests/daemon_test.rs`

- [ ] **Step 1: Write failing test for startup, renew, and continued shared token use**

Add a daemon test that establishes an initial session, serves one claim with the initial token, forces a re-auth signal by returning 401, then serves a second claim with the new token. Use the same local TCP HTTP helper style already present in `daemon_test.rs`:

```rust
#[tokio::test]
async fn daemon_shared_auth_resumes_business_requests_after_reenroll() {
    use superteam_runtime_agent::runtime_auth::RuntimeAuthState;

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, mut request_rx) = tokio::sync::mpsc::channel(6);

    tokio::spawn(async move {
        let (mut first_claim_socket, _) = listener.accept().await.unwrap();
        let first_claim = read_http_request(&mut first_claim_socket).await;
        request_tx.send(first_claim).await.unwrap();
        write_status_response(
            &mut first_claim_socket,
            "401 Unauthorized",
            serde_json::json!("invalid runtime authentication"),
        )
        .await;

        let (mut hello_socket, _) = listener.accept().await.unwrap();
        let hello = read_http_request(&mut hello_socket).await;
        request_tx.send(hello).await.unwrap();
        write_json_response(
            &mut hello_socket,
            serde_json::json!({
                "enrollment": {
                    "id": "11111111-1111-4111-8111-111111111111",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "bootstrap_key_id": "44444444-4444-4444-8444-444444444444",
                    "status": "approved",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session": {
                    "id": "77777777-7777-4777-8777-777777777777",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "enrollment_id": "11111111-1111-4111-8111-111111111111",
                    "expires_at": "2999-06-03T00:00:00Z",
                    "last_seen_at": "2026-06-02T00:00:00Z",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session_token": "fresh-token"
            }),
        )
        .await;

        let (mut caps_socket, _) = listener.accept().await.unwrap();
        let caps = read_http_request(&mut caps_socket).await;
        request_tx.send(caps).await.unwrap();
        write_json_response(&mut caps_socket, serde_json::json!([])).await;

        let (mut second_claim_socket, _) = listener.accept().await.unwrap();
        let second_claim = read_http_request(&mut second_claim_socket).await;
        request_tx.send(second_claim).await.unwrap();
        second_claim_socket
            .write_all(b"HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
            .await
            .unwrap();
    });

    let mut config = RuntimeConfig::new("node-1").expect("config");
    config.runtime.control_plane_url = format!("http://{}", addr);
    config.runtime.bootstrap_key = "bootstrap-key".to_string();
    let auth = RuntimeAuthState::new("node-1");
    auth.set_session("old-session", "old-token", "2999-06-02T00:00:00Z")
        .await
        .expect("old session");

    let client = ControlPlaneClient::with_runtime_auth(config.runtime.control_plane_url.clone(), auth.clone());
    let first_result = client.claim_task(1).await;
    assert!(first_result.is_err());
    superteam_runtime_agent::daemon::reenroll_runtime_session(&config, auth.clone(), vec![])
        .await
        .expect("re-enroll");
    client.claim_task(1).await.expect("second claim");

    let first_claim = request_rx.recv().await.expect("first claim");
    let hello = request_rx.recv().await.expect("hello");
    let caps = request_rx.recv().await.expect("capabilities");
    let second_claim = request_rx.recv().await.expect("second claim");
    assert!(first_claim.contains("authorization: Bearer old-token"));
    assert_eq!(hello.lines().next().unwrap(), "POST /api/v1/runtime/enrollments/hello HTTP/1.1");
    assert!(caps.contains("authorization: Bearer fresh-token"));
    assert!(second_claim.contains("authorization: Bearer fresh-token"));
}
```

- [ ] **Step 2: Run the failing test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml daemon_shared_auth_resumes_business_requests_after_reenroll --test daemon_test -- --nocapture
```

Expected: FAIL because `reenroll_runtime_session` does not exist.

- [ ] **Step 3: Add explicit re-enroll helper**

In `apps/runtime-agent/src/daemon.rs`, add:

```rust
pub async fn reenroll_runtime_session(
    config: &RuntimeConfig,
    auth: RuntimeAuthState,
    capabilities: Vec<RuntimeCapabilityInput>,
) -> Result<bool> {
    auth.mark_status(RuntimeAuthStatus::Reauthenticating).await;
    establish_runtime_auth_session(config, capabilities, auth).await
}
```

- [ ] **Step 4: Replace `RuntimeDaemon::run` session wiring**

Change `RuntimeDaemon::run` to:

```rust
pub async fn run(self) -> Result<()> {
    let capabilities = build_capabilities(&self.config, &self.config.tools.probe_names).await;
    let auth = RuntimeAuthState::new(self.config.runtime.node_id.clone());

    loop {
        if establish_runtime_auth_session(&self.config, capabilities.clone(), auth.clone()).await? {
            break;
        }
        tokio::time::sleep(Duration::from_secs(30)).await;
    }

    let supervisor_config = self.config.clone();
    let supervisor_capabilities = capabilities.clone();
    let supervisor_auth = auth.clone();
    tokio::spawn(async move {
        session_supervisor_loop(supervisor_config, supervisor_capabilities, supervisor_auth).await;
    });

    let control_plane = ControlPlaneClient::with_runtime_auth(
        self.config.runtime.control_plane_url.clone(),
        auth.clone(),
    );

    let command_config = self.config.clone();
    let command_client = control_plane.clone();
    tokio::spawn(async move {
        if let Err(error) = run_command_loop(command_config, command_client).await {
            eprintln!("Runtime command loop failed: {}", error);
        }
    });

    let heartbeat_client = control_plane.clone();
    let heartbeat_config = self.config.clone();
    tokio::spawn(async move {
        heartbeat_loop(heartbeat_client, heartbeat_config).await;
    });

    let executor = TaskExecutor::new(self.config, control_plane);
    executor.run().await?;
    Ok(())
}
```

Add supervisor loop:

```rust
async fn session_supervisor_loop(
    config: RuntimeConfig,
    capabilities: Vec<RuntimeCapabilityInput>,
    auth: RuntimeAuthState,
) {
    loop {
        let snapshot = auth.snapshot().await;
        let delay = snapshot
            .expires_at
            .map(|expires_at| refresh_delay(expires_at, auth.refresh_margin()))
            .unwrap_or(Duration::from_secs(30));

        tokio::select! {
            _ = tokio::time::sleep(delay) => {
                if let Err(error) = renew_or_reenroll_once(&config, auth.clone(), capabilities.clone()).await {
                    eprintln!("Runtime session refresh failed: {}", error);
                    if auth.snapshot().await.status == RuntimeAuthStatus::Refreshing {
                        auth.mark_status(RuntimeAuthStatus::Connected).await;
                    }
                    tokio::time::sleep(Duration::from_secs(30)).await;
                }
            }
            _ = auth.wait_until_reauthenticating() => {
                loop {
                    match reenroll_runtime_session(&config, auth.clone(), capabilities.clone()).await {
                        Ok(true) => break,
                        Ok(false) => tokio::time::sleep(Duration::from_secs(30)).await,
                        Err(error) => {
                            eprintln!("Runtime session re-authentication failed: {}", error);
                            tokio::time::sleep(Duration::from_secs(30)).await;
                        }
                    }
                }
            }
        }
    }
}

fn refresh_delay(expires_at: time::OffsetDateTime, refresh_margin: Duration) -> Duration {
    let renew_at = expires_at - time::Duration::seconds(refresh_margin.as_secs() as i64);
    let now = time::OffsetDateTime::now_utc();
    if renew_at <= now {
        Duration::ZERO
    } else {
        (renew_at - now).try_into().unwrap_or(Duration::ZERO)
    }
}
```

Remove the old `spawn_session_renewal_loop` use from the normal path. Keep a compatibility wrapper only if existing tests still import it, or update those tests to use `renew_or_reenroll_once`.

- [ ] **Step 5: Update command loop signature**

Change `apps/runtime-agent/src/controlplane/ws.rs`:

```rust
pub async fn run_command_loop(config: RuntimeConfig, control_plane: ControlPlaneClient) -> Result<()> {
    let ws_url = runtime_ws_url(&config.runtime.control_plane_url)?;
    let executor = RuntimeCommandExecutor::with_control_plane_client(config, control_plane.clone());

    loop {
        match control_plane.runtime_authorization_header().await {
            Ok(authorization) => {
                if let Err(error) = run_command_loop_once(&executor, &ws_url, &authorization).await {
                    if control_plane.report_websocket_auth_error(error.as_ref()).await {
                        eprintln!("Runtime command loop auth expired; waiting for re-authentication");
                    } else {
                        eprintln!("Runtime command loop connection failed: {}", error);
                    }
                }
            }
            Err(error) => {
                eprintln!("Runtime command loop waiting for runtime auth: {}", error);
            }
        }
        tokio::time::sleep(COMMAND_LOOP_RECONNECT_DELAY).await;
    }
}
```

Add `runtime_authorization_header` and `report_websocket_auth_error` to `ControlPlaneClient`:

```rust
pub async fn runtime_authorization_header(&self) -> Result<http::HeaderValue> {
    let auth = self.runtime_credentials().await?;
    http::HeaderValue::from_str(&format!("Bearer {}", auth.token))
        .context("runtime session token is not a valid websocket authorization header")
}

pub async fn report_websocket_auth_error(
    &self,
    error: &(dyn std::error::Error + 'static),
) -> bool {
    let is_auth = error
        .downcast_ref::<tokio_tungstenite::tungstenite::Error>()
        .is_some_and(|error| matches!(error, tokio_tungstenite::tungstenite::Error::Http(response) if response.status() == http::StatusCode::UNAUTHORIZED));
    if is_auth {
        if let RuntimeAuthMode::Shared(auth) = &self.auth {
            auth.report_auth_failure(None).await;
        }
    }
    is_auth
}
```

- [ ] **Step 6: Run targeted daemon and websocket tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml daemon_shared_auth_resumes_business_requests_after_reenroll --test daemon_test -- --nocapture
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test daemon_test -- --nocapture
cargo test --manifest-path apps/runtime-agent/Cargo.toml controlplane::ws --lib -- --nocapture
```

Expected: PASS.

- [ ] **Step 7: Commit Task 4**

```bash
git add apps/runtime-agent/src/daemon.rs apps/runtime-agent/src/controlplane/ws.rs apps/runtime-agent/src/executor/loops.rs apps/runtime-agent/tests/daemon_test.rs
git commit -m "feat(runtime-agent): supervise runtime auth lifecycle"
```

## Task 5: Add Safety-Window Pause Coverage

**Files:**
- Modify: `apps/runtime-agent/src/runtime_auth.rs`
- Modify: `apps/runtime-agent/tests/controlplane_client_test.rs`

- [ ] **Step 1: Write failing test for business credential pause near expiry**

Add this test to `apps/runtime-agent/tests/controlplane_client_test.rs`:

```rust
#[tokio::test]
async fn shared_runtime_auth_blocks_business_requests_inside_safety_window() {
    use std::time::Duration;
    use superteam_runtime_agent::runtime_auth::RuntimeAuthState;

    let auth = RuntimeAuthState::with_windows(
        "node-1",
        Duration::from_secs(60),
        Duration::from_secs(60),
    );
    auth.set_session("session-1", "token-one", "1970-01-01T00:00:00Z")
        .await
        .expect("expired session");

    let wait = tokio::spawn({
        let auth = auth.clone();
        async move { auth.wait_for_business_credentials().await.unwrap() }
    });

    tokio::time::sleep(Duration::from_millis(100)).await;
    assert!(!wait.is_finished(), "business credentials should wait while unsafe");

    auth.set_session("session-2", "token-two", "2999-06-02T00:00:00Z")
        .await
        .expect("fresh session");
    let credentials = tokio::time::timeout(Duration::from_secs(2), wait)
        .await
        .expect("wait should unblock")
        .expect("join")
        ;
    assert_eq!(credentials.token, "token-two");
}
```

- [ ] **Step 2: Run the safety-window test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml shared_runtime_auth_blocks_business_requests_inside_safety_window --test controlplane_client_test -- --nocapture
```

Expected: PASS if Task 1 implemented `wait_for_business_credentials` correctly. If it fails, fix the safety-window condition so expired or near-expired sessions do not return credentials.

- [ ] **Step 3: Run runtime auth tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml shared_runtime_auth --test controlplane_client_test -- --nocapture
```

Expected: PASS.

- [ ] **Step 4: Commit Task 5**

```bash
git add apps/runtime-agent/src/runtime_auth.rs apps/runtime-agent/tests/controlplane_client_test.rs
git commit -m "test(runtime-agent): pause business traffic near auth expiry"
```

## Task 6: Verify Control Plane Session Semantics Stay Intact

**Files:**
- No planned source edits.

- [ ] **Step 1: Run existing Control Plane runtime session tests**

Run:

```bash
go test ./apps/control-plane/internal/runtime -run 'Test.*RuntimeSession|Test.*Enroll' -count=1
go test ./apps/control-plane/internal/api -run 'Test.*Runtime.*Session|Test.*Runtime.*Enroll|Test.*Runtime.*Unauthorized' -count=1
```

Expected: PASS. If the regex misses all tests in one package, rerun the package without `-run` and record the actual relevant test names in the implementation report.

- [ ] **Step 2: Run full relevant Control Plane packages**

Run:

```bash
go test ./apps/control-plane/internal/runtime -count=1
go test ./apps/control-plane/internal/api -count=1
```

Expected: PASS.

## Task 7: Run Runtime Verification Gates

**Files:**
- No intentional source edits.

- [ ] **Step 1: Run runtime agent verification**

Run:

```bash
corepack pnpm verify:runtime-agent
```

Expected: PASS.

- [ ] **Step 2: Run diff hygiene**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 3: Restart live runtime services**

Run:

```bash
scripts/dev-services.sh restart runtime-agent
scripts/dev-services.sh status
```

Expected: runtime-agent starts successfully, Control Plane remains healthy, and the runtime-agent log contains `Runtime session established`.

- [ ] **Step 4: Verify active runtime session in DB**

Run:

```bash
url=$(sed -n 's/^  url: "\(.*\)"/\1/p' apps/control-plane/config/config.yaml | head -1)
psql "$url" -X -v ON_ERROR_STOP=1 -c "select rn.node_id, count(*) filter (where rs.expires_at > now() and rs.revoked_at is null) as active_sessions, max(rs.expires_at) as latest_expires_at from runtime_nodes rn left join runtime_sessions rs on rs.runtime_node_id = rn.id where rn.node_id = 'local-dev-node' group by rn.node_id;"
```

Expected: `active_sessions` is at least `1` for `local-dev-node`.

## Task 8: Real Auth Self-Healing Smoke

**Files:**
- No committed source edits unless the smoke exposes a bug.

- [ ] **Step 1: Force one runtime session invalidation**

Run:

```bash
url=$(sed -n 's/^  url: "\(.*\)"/\1/p' apps/control-plane/config/config.yaml | head -1)
psql "$url" -X -v ON_ERROR_STOP=1 -c "update runtime_sessions rs set expires_at = now() - interval '1 minute', updated_at = now() from runtime_nodes rn where rs.runtime_node_id = rn.id and rn.node_id = 'local-dev-node' and rs.revoked_at is null;"
```

Expected: at least one row updated.

- [ ] **Step 2: Watch runtime-agent re-authenticate**

Run:

```bash
sleep 10
tail -n 120 .scratch/dev-services/logs/runtime-agent.log
```

Expected:

- log includes a runtime auth refresh or re-authentication message;
- log includes `Runtime session established` after the forced invalidation;
- repeated heartbeat, claim, or websocket 401s do not continue after the new session is established.

- [ ] **Step 3: Verify DB active session recovered**

Run:

```bash
url=$(sed -n 's/^  url: "\(.*\)"/\1/p' apps/control-plane/config/config.yaml | head -1)
psql "$url" -X -v ON_ERROR_STOP=1 -c "select rn.node_id, count(*) filter (where rs.expires_at > now() and rs.revoked_at is null) as active_sessions, max(rs.expires_at) as latest_expires_at, max(rs.last_seen_at) as latest_seen_at from runtime_nodes rn left join runtime_sessions rs on rs.runtime_node_id = rn.id where rn.node_id = 'local-dev-node' group by rn.node_id;"
```

Expected: `active_sessions >= 1` and `latest_seen_at` is after the forced invalidation.

- [ ] **Step 4: Run one real Runtime/Provider smoke**

Run the repo's existing runtime provider gate:

```bash
corepack pnpm verify:runtime-agent
```

Then run the live smoke command already used for this repo if available:

```bash
apps/runtime-agent/target/debug/runtime-agent run --provider codex --provider-bin /opt/homebrew/bin/codex --prompt "Return exactly: runtime auth self-healing smoke"
```

Expected: provider output includes a completed run event such as `turn_completed`, and no runtime auth 401 loop appears in `.scratch/dev-services/logs/runtime-agent.log` after the smoke.

- [ ] **Step 5: Use completion gate before final claim**

Read and follow:

```bash
sed -n '1,220p' .codex/skills/superteam-completion-check/SKILL.md
```

Expected: final report distinguishes unit tests, runtime verification, and real auth self-healing smoke evidence.

## Self-Review

- Spec coverage:
  - Shared mutable auth state: Tasks 1, 4, 5.
  - Proactive refresh before expiry: Tasks 3, 4, 5.
  - 401 fallback: Tasks 2, 3, 4, 8.
  - Active task continuity by request-time token lookup: Tasks 1 and 4.
  - Control Plane short-session semantics retained: Task 6.
  - Real verification: Tasks 7 and 8.
- Placeholder scan: no unfinished markers or unspecified test steps remain.
- Type consistency:
  - `RuntimeAuthState`, `RuntimeAuthStatus`, `RuntimeCredentials`, and `RuntimeAuthExpired` are introduced in Task 1 and reused consistently.
  - `ControlPlaneClient::with_runtime_auth` is introduced before daemon and websocket tasks use it.
  - `renew_or_reenroll_once`, `reenroll_runtime_session`, and `establish_runtime_auth_session` are introduced before tests depend on them.
