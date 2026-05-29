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
            client,
        }
    }

    /// Register this runtime node with the Control Plane
    pub async fn register(&self, req: RegisterNodeRequest) -> Result<RegisterNodeResponse> {
        let url = format!("{}/api/v1/runtime/register", self.base_url);

        let response = self
            .client
            .post(&url)
            .bearer_auth(&self.token)
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
        let url = format!(
            "{}/api/v1/runtime/tasks/poll?timeout={}",
            self.base_url, timeout_secs
        );

        let response = self
            .client
            .get(&url)
            .bearer_auth(&self.token)
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
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_client_creation() {
        let client = ControlPlaneClient::new("http://localhost:8080", "test-token");
        assert_eq!(client.base_url, "http://localhost:8080");
        assert_eq!(client.token, "test-token");
    }
}
