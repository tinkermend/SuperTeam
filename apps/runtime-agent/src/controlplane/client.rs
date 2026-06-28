use anyhow::{Context, Result, anyhow};
use reqwest::{Client, Method, RequestBuilder, StatusCode};
use std::time::Duration;

use crate::runtime_auth::{
    RuntimeAuthExpired, RuntimeAuthState, RuntimeCredentials, is_runtime_auth_failure,
};

use super::models::{
    EnrollHelloRequest, EnrollHelloResponse, HeartbeatRequest, HeartbeatResponse,
    ProjectTaskCompleteWriteback, ProjectTaskFailWriteback, ProjectTaskStartWriteback,
    ProjectTaskWaitHumanWriteback, RegisterNodeRequest, RegisterNodeResponse,
    RuntimeCapabilitiesRequest, RuntimeCapabilityInput, RuntimeCapabilityResponse,
    RuntimeCommandEventWriteback, RuntimeCommandTerminalWriteback, RuntimeSessionResponse, Task,
};

#[derive(Clone)]
enum RuntimeAuthMode {
    None,
    Static {
        token: String,
        node_id: Option<String>,
    },
    Shared(RuntimeAuthState),
}

/// Control Plane HTTP client
#[derive(Clone)]
pub struct ControlPlaneClient {
    base_url: String,
    auth: RuntimeAuthMode,
    client: Client,
}

pub struct RuntimeAuthorization {
    pub header: http::HeaderValue,
    pub generation: u64,
}

impl ControlPlaneClient {
    /// Create a new Control Plane client
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
            .timeout(Duration::from_secs(65)) // Slightly longer than max poll timeout
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

    pub async fn runtime_authorization_header(&self) -> Result<http::HeaderValue> {
        Ok(self.runtime_authorization().await?.header)
    }

    pub async fn runtime_authorization(&self) -> Result<RuntimeAuthorization> {
        let auth = self.runtime_credentials().await?;
        let header = http::HeaderValue::from_str(&format!("Bearer {}", auth.token))
            .context("runtime session token is not a valid websocket authorization header")?;
        Ok(RuntimeAuthorization {
            header,
            generation: auth.generation,
        })
    }

    pub async fn report_websocket_auth_error(
        &self,
        error: &(dyn std::error::Error + Send + Sync + 'static),
    ) -> bool {
        self.report_websocket_auth_error_with_generation(error, None)
            .await
    }

    pub async fn report_websocket_auth_error_for_generation(
        &self,
        error: &(dyn std::error::Error + Send + Sync + 'static),
        generation: u64,
    ) -> bool {
        self.report_websocket_auth_error_with_generation(error, Some(generation))
            .await
    }

    async fn report_websocket_auth_error_with_generation(
        &self,
        error: &(dyn std::error::Error + Send + Sync + 'static),
        generation: Option<u64>,
    ) -> bool {
        let is_auth = error
            .downcast_ref::<tokio_tungstenite::tungstenite::Error>()
            .is_some_and(|error| {
                matches!(
                    error,
                    tokio_tungstenite::tungstenite::Error::Http(response)
                        if response.status().as_u16() == StatusCode::UNAUTHORIZED.as_u16()
                )
        });
        if is_auth {
            if let RuntimeAuthMode::Shared(auth) = &self.auth {
                auth.report_auth_failure(generation).await;
            }
        }
        is_auth
    }

    pub async fn enroll_hello(&self, req: EnrollHelloRequest) -> Result<EnrollHelloResponse> {
        let url = self.enroll_hello_url();

        let response = self
            .client
            .post(&url)
            .json(&req)
            .send()
            .await
            .context("Failed to send enrollment hello request")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Enrollment hello failed with status {}: {}", status, body);
        }

        let enrollment = response
            .json::<EnrollHelloResponse>()
            .await
            .context("Failed to parse enrollment hello response")?;

        Ok(enrollment)
    }

    /// Register this runtime node with the Control Plane
    pub async fn register(&self, req: RegisterNodeRequest) -> Result<RegisterNodeResponse> {
        let url = format!("{}/api/v1/runtime/register", self.base_url);
        let token = self.bearer_token().await?;

        let response = self
            .client
            .post(&url)
            .bearer_auth(token)
            .header("X-Node-ID", &req.node_id)
            .json(&req)
            .send()
            .await
            .context("Failed to send register request")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Register failed with status {}: {}", status, body);
        }

        let node = response
            .json::<RegisterNodeResponse>()
            .await
            .context("Failed to parse register response")?;

        Ok(node)
    }

    pub async fn renew_session(&self, session_id: &str) -> Result<RuntimeSessionResponse> {
        let url = self.session_renew_url(session_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, true).await?;

        let response = request
            .json(&serde_json::json!({}))
            .send()
            .await
            .context("Failed to send runtime session renew request")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Runtime session renew", status, body, Some(auth.generation))
                .await);
        }

        let session = response
            .json::<RuntimeSessionResponse>()
            .await
            .context("Failed to parse runtime session renew response")?;

        Ok(session)
    }

    pub async fn upsert_capabilities(
        &self,
        node_id: &str,
        capabilities: Vec<RuntimeCapabilityInput>,
    ) -> Result<Vec<RuntimeCapabilityResponse>> {
        let url = self.capabilities_url(node_id);
        let (request, auth) = self.runtime_request(Method::PUT, &url, false).await?;

        let response = request
            .json(&RuntimeCapabilitiesRequest { capabilities })
            .send()
            .await
            .context("Failed to send runtime capabilities request")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Runtime capabilities upsert",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        let capabilities = response
            .json::<Vec<RuntimeCapabilityResponse>>()
            .await
            .context("Failed to parse runtime capabilities response")?;

        Ok(capabilities)
    }

    /// Send heartbeat to Control Plane
    pub async fn heartbeat(&self, req: HeartbeatRequest) -> Result<HeartbeatResponse> {
        let url = format!("{}/api/v1/runtime/heartbeat", self.base_url);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(&req)
            .send()
            .await
            .context("Failed to send heartbeat request")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Heartbeat", status, body, Some(auth.generation))
                .await);
        }

        let node = response
            .json::<HeartbeatResponse>()
            .await
            .context("Failed to parse heartbeat response")?;

        Ok(node)
    }

    /// Claim a task from Control Plane (long polling)
    ///
    /// This will block for up to `timeout` seconds waiting for a task.
    /// Returns `Ok(None)` if no task is available within the timeout.
    pub async fn claim_task(&self, timeout_secs: u64) -> Result<Option<Task>> {
        let url = self.claim_task_url(timeout_secs);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .send()
            .await
            .context("Failed to send claim task request")?;

        match response.status() {
            StatusCode::OK => {
                let task = response
                    .json::<Task>()
                    .await
                    .context("Failed to parse task response")?;
                Ok(Some(task))
            }
            StatusCode::NO_CONTENT => Ok(None),
            status => {
                let body = response.text().await.unwrap_or_default();
                Err(self
                    .runtime_error("Claim task", status, body, Some(auth.generation))
                    .await)
            }
        }
    }

    /// Update task status
    pub async fn update_task_status(
        &self,
        task_id: i64,
        status: super::models::TaskStatus,
    ) -> Result<()> {
        let url = self.task_status_url(task_id);
        let (request, auth) = self.runtime_request(Method::PUT, &url, false).await?;

        let response = request
            .json(&serde_json::json!({"status": status}))
            .send()
            .await
            .context("Failed to update task status")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Update task status", status, body, Some(auth.generation))
                .await);
        }

        Ok(())
    }

    /// Push event to Control Plane
    pub async fn push_event(
        &self,
        task_id: i64,
        event: &crate::events::ProviderEvent,
    ) -> Result<()> {
        let url = self.task_events_url(task_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(&serde_json::json!({"events": [event]}))
            .send()
            .await
            .context("Failed to push event")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Push event", status, body, Some(auth.generation))
                .await);
        }

        Ok(())
    }

    /// Complete task
    pub async fn complete_task(&self, task_id: i64, result: serde_json::Value) -> Result<()> {
        let url = self.task_complete_url(task_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(&serde_json::json!({"result": result}))
            .send()
            .await
            .context("Failed to complete task")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Complete task", status, body, Some(auth.generation))
                .await);
        }

        Ok(())
    }

    /// Fail task
    pub async fn fail_task(&self, task_id: i64, error: String) -> Result<()> {
        let url = self.task_fail_url(task_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(&serde_json::json!({"error": error}))
            .send()
            .await
            .context("Failed to fail task")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Fail task", status, body, Some(auth.generation))
                .await);
        }

        Ok(())
    }

    pub async fn complete_runtime_command(
        &self,
        command_id: &str,
        terminal: &RuntimeCommandTerminalWriteback,
    ) -> Result<()> {
        let url = self.runtime_command_complete_url(command_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(terminal)
            .send()
            .await
            .context("Failed to complete runtime command")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Complete runtime command",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        Ok(())
    }

    pub async fn record_runtime_command_event(
        &self,
        command_id: &str,
        event: &RuntimeCommandEventWriteback,
    ) -> Result<()> {
        let url = self.runtime_command_events_url(command_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(event)
            .send()
            .await
            .context("Failed to record runtime command event")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Record runtime command event",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        Ok(())
    }

    pub async fn fail_runtime_command(
        &self,
        command_id: &str,
        terminal: &RuntimeCommandTerminalWriteback,
    ) -> Result<()> {
        let url = self.runtime_command_fail_url(command_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(terminal)
            .send()
            .await
            .context("Failed to fail runtime command")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Fail runtime command", status, body, Some(auth.generation))
                .await);
        }

        Ok(())
    }

    pub async fn cancel_runtime_command(
        &self,
        command_id: &str,
        terminal: &RuntimeCommandTerminalWriteback,
    ) -> Result<()> {
        let url = self.runtime_command_cancelled_url(command_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(terminal)
            .send()
            .await
            .context("Failed to cancel runtime command")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Cancel runtime command",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        Ok(())
    }

    pub async fn complete_project_task_attempt(
        &self,
        attempt_id: &str,
        writeback: &ProjectTaskCompleteWriteback,
    ) -> Result<()> {
        let url = self.project_task_attempt_complete_url(attempt_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(writeback)
            .send()
            .await
            .context("Failed to complete project task attempt")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Complete project task attempt",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        Ok(())
    }

    pub async fn start_project_task_attempt(
        &self,
        attempt_id: &str,
        writeback: &ProjectTaskStartWriteback,
    ) -> Result<()> {
        let url = self.project_task_attempt_started_url(attempt_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(writeback)
            .send()
            .await
            .context("Failed to start project task attempt")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Start project task attempt",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        Ok(())
    }

    pub async fn fail_project_task_attempt(
        &self,
        attempt_id: &str,
        writeback: &ProjectTaskFailWriteback,
    ) -> Result<()> {
        let url = self.project_task_attempt_fail_url(attempt_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(writeback)
            .send()
            .await
            .context("Failed to fail project task attempt")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Fail project task attempt",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        Ok(())
    }

    pub async fn wait_human_project_task_attempt(
        &self,
        attempt_id: &str,
        writeback: &ProjectTaskWaitHumanWriteback,
    ) -> Result<()> {
        let url = self.project_task_attempt_wait_human_url(attempt_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request
            .json(writeback)
            .send()
            .await
            .context("Failed to wait-human project task attempt")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error(
                    "Wait-human project task attempt",
                    status,
                    body,
                    Some(auth.generation),
                )
                .await);
        }

        Ok(())
    }

    /// Renew task lease
    pub async fn renew_lease(&self, task_id: i64) -> Result<()> {
        let url = self.task_lease_url(task_id);
        let (request, auth) = self.runtime_request(Method::POST, &url, false).await?;

        let response = request.send().await.context("Failed to renew lease")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(self
                .runtime_error("Renew lease", status, body, Some(auth.generation))
                .await);
        }

        Ok(())
    }

    fn claim_task_url(&self, timeout_secs: u64) -> String {
        format!(
            "{}/api/v1/runtime/tasks/claim?timeout={}",
            self.base_url, timeout_secs
        )
    }

    fn enroll_hello_url(&self) -> String {
        format!("{}/api/v1/runtime/enrollments/hello", self.base_url)
    }

    fn session_renew_url(&self, session_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/sessions/{}/renew",
            self.base_url, session_id
        )
    }

    fn capabilities_url(&self, node_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/nodes/{}/capabilities",
            self.base_url, node_id
        )
    }

    async fn bearer_token(&self) -> Result<String> {
        match &self.auth {
            RuntimeAuthMode::Shared(auth) => Ok(auth.renewal_credentials().await?.token),
            RuntimeAuthMode::Static { token, .. } => Ok(token.clone()),
            RuntimeAuthMode::None => {
                anyhow::bail!("runtime auth is required for authenticated Runtime API requests")
            }
        }
    }

    async fn runtime_credentials(&self) -> Result<RuntimeCredentials> {
        match &self.auth {
            RuntimeAuthMode::Shared(auth) => auth.wait_for_business_credentials().await,
            RuntimeAuthMode::Static { token, node_id } => Ok(RuntimeCredentials {
                node_id: node_id
                    .clone()
                    .filter(|node_id| !node_id.trim().is_empty())
                    .context(
                        "Runtime node_id is required for authenticated Runtime API requests",
                    )?,
                session_id: String::new(),
                token: token.clone(),
                expires_at: time::OffsetDateTime::UNIX_EPOCH,
                generation: 0,
            }),
            RuntimeAuthMode::None => {
                anyhow::bail!("runtime auth is required for authenticated Runtime API requests")
            }
        }
    }

    async fn renewal_credentials(&self) -> Result<RuntimeCredentials> {
        match &self.auth {
            RuntimeAuthMode::Shared(auth) => auth.renewal_credentials().await,
            _ => self.runtime_credentials().await,
        }
    }

    async fn runtime_request(
        &self,
        method: Method,
        url: &str,
        allow_unsafe_for_renewal: bool,
    ) -> Result<(RequestBuilder, RuntimeCredentials)> {
        let auth = if allow_unsafe_for_renewal {
            self.renewal_credentials().await?
        } else {
            self.runtime_credentials().await?
        };
        let request = self
            .client
            .request(method, url)
            .bearer_auth(&auth.token)
            .headers(Self::runtime_headers_for(&auth.node_id)?);
        Ok((request, auth))
    }

    fn runtime_headers_for(node_id: &str) -> Result<reqwest::header::HeaderMap> {
        let node_id = (!node_id.trim().is_empty())
            .then_some(node_id)
            .context("Runtime node_id is required for authenticated Runtime API requests")?;
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

    fn task_events_url(&self, task_id: i64) -> String {
        format!("{}/api/v1/runtime/tasks/{}/events", self.base_url, task_id)
    }

    fn task_status_url(&self, task_id: i64) -> String {
        format!("{}/api/v1/tasks/{}/status", self.base_url, task_id)
    }

    fn task_complete_url(&self, task_id: i64) -> String {
        format!(
            "{}/api/v1/runtime/tasks/{}/complete",
            self.base_url, task_id
        )
    }

    fn task_fail_url(&self, task_id: i64) -> String {
        format!("{}/api/v1/runtime/tasks/{}/fail", self.base_url, task_id)
    }

    fn runtime_command_complete_url(&self, command_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/commands/{}/complete",
            self.base_url, command_id
        )
    }

    fn runtime_command_events_url(&self, command_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/commands/{}/events",
            self.base_url, command_id
        )
    }

    fn runtime_command_fail_url(&self, command_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/commands/{}/fail",
            self.base_url, command_id
        )
    }

    fn runtime_command_cancelled_url(&self, command_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/commands/{}/cancelled",
            self.base_url, command_id
        )
    }

    fn project_task_attempt_complete_url(&self, attempt_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/project-task-attempts/{}/complete",
            self.base_url, attempt_id
        )
    }

    fn project_task_attempt_started_url(&self, attempt_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/project-task-attempts/{}/started",
            self.base_url, attempt_id
        )
    }

    fn project_task_attempt_fail_url(&self, attempt_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/project-task-attempts/{}/fail",
            self.base_url, attempt_id
        )
    }

    fn project_task_attempt_wait_human_url(&self, attempt_id: &str) -> String {
        format!(
            "{}/api/v1/runtime/project-task-attempts/{}/wait-human",
            self.base_url, attempt_id
        )
    }

    fn task_lease_url(&self, task_id: i64) -> String {
        format!("{}/api/v1/runtime/tasks/{}/lease", self.base_url, task_id)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_client_creation() {
        let client = ControlPlaneClient::new("http://localhost:8080", "test-token");
        assert_eq!(client.base_url, "http://localhost:8080");
        match client.auth {
            RuntimeAuthMode::Static { token, node_id } => {
                assert_eq!(token, "test-token");
                assert_eq!(node_id, None);
            }
            RuntimeAuthMode::None | RuntimeAuthMode::Shared(_) => {
                panic!("expected static auth mode")
            }
        }
    }

    #[test]
    fn controlplane_client_builds_canonical_runtime_task_paths() {
        let client = ControlPlaneClient::new("http://localhost:8080", "test-token");

        assert_eq!(
            client.claim_task_url(30),
            "http://localhost:8080/api/v1/runtime/tasks/claim?timeout=30"
        );
        assert_eq!(
            client.enroll_hello_url(),
            "http://localhost:8080/api/v1/runtime/enrollments/hello"
        );
        assert_eq!(
            client.session_renew_url("session-1"),
            "http://localhost:8080/api/v1/runtime/sessions/session-1/renew"
        );
        assert_eq!(
            client.capabilities_url("node-1"),
            "http://localhost:8080/api/v1/runtime/nodes/node-1/capabilities"
        );
        assert_eq!(
            client.task_events_url(1),
            "http://localhost:8080/api/v1/runtime/tasks/1/events"
        );
        assert_eq!(
            client.task_status_url(1),
            "http://localhost:8080/api/v1/tasks/1/status"
        );
        assert_eq!(
            client.task_complete_url(1),
            "http://localhost:8080/api/v1/runtime/tasks/1/complete"
        );
        assert_eq!(
            client.task_fail_url(1),
            "http://localhost:8080/api/v1/runtime/tasks/1/fail"
        );
        assert_eq!(
            client.runtime_command_complete_url("cmd-1"),
            "http://localhost:8080/api/v1/runtime/commands/cmd-1/complete"
        );
        assert_eq!(
            client.runtime_command_events_url("cmd-1"),
            "http://localhost:8080/api/v1/runtime/commands/cmd-1/events"
        );
        assert_eq!(
            client.runtime_command_fail_url("cmd-1"),
            "http://localhost:8080/api/v1/runtime/commands/cmd-1/fail"
        );
        assert_eq!(
            client.project_task_attempt_complete_url("attempt-1"),
            "http://localhost:8080/api/v1/runtime/project-task-attempts/attempt-1/complete"
        );
        assert_eq!(
            client.project_task_attempt_fail_url("attempt-1"),
            "http://localhost:8080/api/v1/runtime/project-task-attempts/attempt-1/fail"
        );
        assert_eq!(
            client.project_task_attempt_wait_human_url("attempt-1"),
            "http://localhost:8080/api/v1/runtime/project-task-attempts/attempt-1/wait-human"
        );
        assert_eq!(
            client.task_lease_url(1),
            "http://localhost:8080/api/v1/runtime/tasks/1/lease"
        );
    }
}
