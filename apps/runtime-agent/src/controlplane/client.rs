use anyhow::{Context, Result};
use reqwest::{Client, StatusCode};
use std::time::Duration;

use super::models::{
    HeartbeatRequest, HeartbeatResponse, RegisterNodeRequest, RegisterNodeResponse, Task,
};

/// Control Plane HTTP client
#[derive(Clone)]
pub struct ControlPlaneClient {
    base_url: String,
    token: String,
    node_id: Option<String>,
    client: Client,
}

impl ControlPlaneClient {
    /// Create a new Control Plane client
    pub fn new(base_url: impl Into<String>, token: impl Into<String>) -> Self {
        let client = Client::builder()
            .timeout(Duration::from_secs(65)) // Slightly longer than max poll timeout
            .build()
            .expect("Failed to build HTTP client");

        Self {
            base_url: base_url.into(),
            token: token.into(),
            node_id: None,
            client,
        }
    }

    pub fn with_node_id(
        base_url: impl Into<String>,
        token: impl Into<String>,
        node_id: impl Into<String>,
    ) -> Self {
        let mut client = Self::new(base_url, token);
        client.node_id = Some(node_id.into());
        client
    }

    /// Register this runtime node with the Control Plane
    pub async fn register(&self, req: RegisterNodeRequest) -> Result<RegisterNodeResponse> {
        let url = format!("{}/api/v1/runtime/register", self.base_url);

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
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

    /// Send heartbeat to Control Plane
    pub async fn heartbeat(&self, req: HeartbeatRequest) -> Result<HeartbeatResponse> {
        let url = format!("{}/api/v1/runtime/heartbeat", self.base_url);

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
            .headers(self.runtime_headers()?)
            .json(&req)
            .send()
            .await
            .context("Failed to send heartbeat request")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Heartbeat failed with status {}: {}", status, body);
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

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
            .headers(self.runtime_headers()?)
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
                anyhow::bail!("Claim task failed with status {}: {}", status, body);
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

        let response = self
            .client
            .put(&url)
            .bearer_auth(&self.token)
            .headers(self.runtime_headers()?)
            .json(&serde_json::json!({"status": status}))
            .send()
            .await
            .context("Failed to update task status")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Update task status failed with {}: {}", status, body);
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

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
            .headers(self.runtime_headers()?)
            .json(&serde_json::json!({"events": [event]}))
            .send()
            .await
            .context("Failed to push event")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Push event failed with {}: {}", status, body);
        }

        Ok(())
    }

    /// Complete task
    pub async fn complete_task(&self, task_id: i64, result: serde_json::Value) -> Result<()> {
        let url = self.task_complete_url(task_id);

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
            .headers(self.runtime_headers()?)
            .json(&serde_json::json!({"result": result}))
            .send()
            .await
            .context("Failed to complete task")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Complete task failed with {}: {}", status, body);
        }

        Ok(())
    }

    /// Fail task
    pub async fn fail_task(&self, task_id: i64, error: String) -> Result<()> {
        let url = self.task_fail_url(task_id);

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
            .headers(self.runtime_headers()?)
            .json(&serde_json::json!({"error": error}))
            .send()
            .await
            .context("Failed to fail task")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Fail task failed with {}: {}", status, body);
        }

        Ok(())
    }

    /// Renew task lease
    pub async fn renew_lease(&self, task_id: i64) -> Result<()> {
        let url = self.task_lease_url(task_id);

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
            .headers(self.runtime_headers()?)
            .send()
            .await
            .context("Failed to renew lease")?;

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            anyhow::bail!("Renew lease failed with {}: {}", status, body);
        }

        Ok(())
    }

    fn claim_task_url(&self, timeout_secs: u64) -> String {
        format!(
            "{}/api/v1/runtime/tasks/claim?timeout={}",
            self.base_url, timeout_secs
        )
    }

    fn runtime_headers(&self) -> Result<reqwest::header::HeaderMap> {
        let node_id = self
            .node_id
            .as_ref()
            .filter(|node_id| !node_id.trim().is_empty())
            .context("Runtime node_id is required for authenticated Runtime API requests")?;
        let mut headers = reqwest::header::HeaderMap::new();
        headers.insert(
            "X-Node-ID",
            reqwest::header::HeaderValue::from_str(node_id)
                .context("Runtime node_id is not a valid header value")?,
        );
        Ok(headers)
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
        assert_eq!(client.token, "test-token");
        assert_eq!(client.node_id, None);
    }

    #[test]
    fn controlplane_client_builds_canonical_runtime_task_paths() {
        let client = ControlPlaneClient::new("http://localhost:8080", "test-token");

        assert_eq!(
            client.claim_task_url(30),
            "http://localhost:8080/api/v1/runtime/tasks/claim?timeout=30"
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
            client.task_lease_url(1),
            "http://localhost:8080/api/v1/runtime/tasks/1/lease"
        );
    }
}
