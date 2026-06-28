use std::fmt;
use std::sync::Arc;
use std::time::Duration;

use anyhow::{Context, Result};
use reqwest::StatusCode;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};
use tokio::sync::watch;

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
    state: watch::Sender<RuntimeAuthSnapshot>,
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
        let (state, _) = watch::channel(RuntimeAuthSnapshot {
            node_id: node_id.clone(),
            session_id: None,
            token: None,
            expires_at: None,
            generation: 0,
            status: RuntimeAuthStatus::Empty,
        });
        Self {
            inner: Arc::new(RuntimeAuthInner {
                state,
                node_id,
                safety_window,
                refresh_margin,
            }),
        }
    }

    pub async fn snapshot(&self) -> RuntimeAuthSnapshot {
        self.inner.state.borrow().clone()
    }

    pub async fn set_session(
        &self,
        session_id: impl Into<String>,
        token: impl Into<String>,
        expires_at: impl AsRef<str>,
    ) -> Result<RuntimeCredentials> {
        let expires_at = OffsetDateTime::parse(expires_at.as_ref(), &Rfc3339)
            .context("runtime session expires_at is not RFC3339")?;
        let session_id = session_id.into();
        let token = token.into();
        let mut credentials = None;
        self.inner.state.send_modify(|state| {
            state.session_id = Some(session_id);
            state.token = Some(token);
            state.expires_at = Some(expires_at);
            state.generation += 1;
            state.status = RuntimeAuthStatus::Connected;
            credentials = Some(
                credentials_from_snapshot(state)
                    .expect("runtime credentials are complete after set_session"),
            );
        });
        Ok(credentials.expect("set_session send_modify should always assign credentials"))
    }

    pub async fn mark_status(&self, status: RuntimeAuthStatus) {
        self.inner.state.send_modify(|state| {
            state.status = status;
        });
    }

    pub async fn report_auth_failure(&self, generation: Option<u64>) {
        self.inner.state.send_modify(|state| {
            if generation.is_none() || generation == Some(state.generation) {
                state.status = RuntimeAuthStatus::Reauthenticating;
            }
        });
    }

    pub async fn wait_for_business_credentials(&self) -> Result<RuntimeCredentials> {
        let mut state = self.inner.state.subscribe();
        loop {
            let snapshot = state.borrow_and_update().clone();
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
            state
                .changed()
                .await
                .context("runtime auth state sender closed")?;
        }
    }

    pub async fn renewal_credentials(&self) -> Result<RuntimeCredentials> {
        credentials_from_snapshot(&self.snapshot().await)
    }

    pub async fn wait_until_reauthenticating(&self) {
        let mut state = self.inner.state.subscribe();
        loop {
            if state.borrow_and_update().status == RuntimeAuthStatus::Reauthenticating {
                return;
            }
            if state.changed().await.is_err() {
                return;
            }
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
