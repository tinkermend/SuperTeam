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
