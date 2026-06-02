use std::time::Duration;

use anyhow::Result;
use serde_json::json;

use crate::config::RuntimeConfig;
use crate::controlplane::client::ControlPlaneClient;
use crate::controlplane::models::{
    EnrollHelloRequest, EnrollmentStatus, HeartbeatRequest, NodeStatus, RuntimeCapabilityInput,
};
use crate::executor::TaskExecutor;
use crate::session::RuntimeSession;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeSnapshot {
    pub node_id: String,
    pub status: String,
}

#[derive(Debug, Clone)]
pub struct RuntimeDaemon {
    config: RuntimeConfig,
}

impl RuntimeDaemon {
    pub fn new(config: RuntimeConfig) -> Self {
        Self { config }
    }

    pub fn snapshot(&self) -> RuntimeSnapshot {
        RuntimeSnapshot {
            node_id: self.config.node_id().to_string(),
            status: "idle".to_string(),
        }
    }

    pub async fn run(self) -> Result<()> {
        let bootstrap_client = ControlPlaneClient::new(&self.config.runtime.control_plane_url, "");
        let supported_providers = build_supported_providers(&self.config);
        let capabilities = build_capabilities(&self.config);

        let hello_req = EnrollHelloRequest {
            node_id: self.config.runtime.node_id.clone(),
            name: format!("runtime-{}", self.config.runtime.node_id),
            supported_providers,
            max_slots: self.config.runtime.max_concurrent_tasks as i32,
            bootstrap_key: self.config.runtime.bootstrap_key.clone(),
            version: Some(env!("CARGO_PKG_VERSION").to_string()),
            metadata: Some(runtime_metadata()),
            capabilities,
        };

        let hello = bootstrap_client.enroll_hello(hello_req).await?;
        if hello.enrollment.status != EnrollmentStatus::Approved {
            println!(
                "runtime-agent node={} enrollment={}; waiting for approval",
                self.config.runtime.node_id,
                enrollment_status_label(&hello.enrollment.status)
            );
            return Ok(());
        }

        let session = RuntimeSession::new(
            hello.session_token.unwrap_or_default(),
            hello
                .session
                .as_ref()
                .map(|session| session.expires_at.clone()),
        );
        if hello.session.is_none() || session.is_empty() {
            anyhow::bail!("approved runtime enrollment did not return a session token");
        }

        let client = ControlPlaneClient::with_session_token(
            &self.config.runtime.control_plane_url,
            &session.token,
            &self.config.runtime.node_id,
        );
        println!("Runtime session established");

        let heartbeat_client = client.clone();
        let heartbeat_config = self.config.clone();
        tokio::spawn(async move {
            heartbeat_loop(heartbeat_client, heartbeat_config).await;
        });

        let executor = TaskExecutor::new(self.config, client);
        executor.run().await?;

        Ok(())
    }
}

fn build_supported_providers(config: &RuntimeConfig) -> Vec<String> {
    let mut providers = Vec::new();
    if config.providers.claude_code.enabled {
        providers.push("claude-code".to_string());
    }
    if config.providers.opencode.enabled {
        providers.push("opencode".to_string());
    }
    providers
}

fn build_capabilities(config: &RuntimeConfig) -> Vec<RuntimeCapabilityInput> {
    let mut capabilities = Vec::new();
    capabilities.push(provider_capability(
        "claude-code",
        config.providers.claude_code.enabled,
        config
            .providers
            .claude_code
            .binary_path
            .display()
            .to_string(),
    ));
    capabilities.push(provider_capability(
        "opencode",
        config.providers.opencode.enabled,
        config.providers.opencode.binary_path.display().to_string(),
    ));

    let mut workspace_labels = std::collections::HashMap::new();
    workspace_labels.insert(
        "cleanup_policy".to_string(),
        json!(config.workspace.cleanup_policy),
    );
    workspace_labels.insert(
        "max_retained".to_string(),
        json!(config.workspace.max_retained),
    );
    capabilities.push(RuntimeCapabilityInput {
        capability_type: "workspace".to_string(),
        capability_key: "base-dir".to_string(),
        provider_type: "workspace".to_string(),
        provider_version: None,
        binary_path: None,
        available: true,
        workspace_base_dir: Some(config.workspace.base_dir.display().to_string()),
        capacity: None,
        labels: Some(workspace_labels),
        status: "available".to_string(),
        details: None,
        health_status: "configured".to_string(),
        metadata: None,
    });

    let mut capacity = std::collections::HashMap::new();
    capacity.insert(
        "max_slots".to_string(),
        json!(config.runtime.max_concurrent_tasks),
    );
    capabilities.push(RuntimeCapabilityInput {
        capability_type: "capacity".to_string(),
        capability_key: "execution-slots".to_string(),
        provider_type: "runtime".to_string(),
        provider_version: None,
        binary_path: None,
        available: true,
        workspace_base_dir: None,
        capacity: Some(capacity),
        labels: None,
        status: "available".to_string(),
        details: None,
        health_status: "configured".to_string(),
        metadata: None,
    });

    capabilities
}

fn provider_capability(
    provider_type: &str,
    enabled: bool,
    binary_path: String,
) -> RuntimeCapabilityInput {
    RuntimeCapabilityInput {
        capability_type: "provider".to_string(),
        capability_key: provider_type.to_string(),
        provider_type: provider_type.to_string(),
        provider_version: None,
        binary_path: Some(binary_path),
        available: enabled,
        workspace_base_dir: None,
        capacity: None,
        labels: None,
        status: if enabled { "available" } else { "disabled" }.to_string(),
        details: None,
        health_status: if enabled { "configured" } else { "disabled" }.to_string(),
        metadata: None,
    }
}

fn runtime_metadata() -> std::collections::HashMap<String, serde_json::Value> {
    let mut metadata = std::collections::HashMap::new();
    metadata.insert("runtime".to_string(), json!("runtime-agent"));
    metadata
}

fn enrollment_status_label(status: &EnrollmentStatus) -> &'static str {
    match status {
        EnrollmentStatus::Pending => "pending",
        EnrollmentStatus::Approved => "approved",
        EnrollmentStatus::Rejected => "rejected",
        EnrollmentStatus::Revoked => "revoked",
    }
}

async fn heartbeat_loop(client: ControlPlaneClient, config: RuntimeConfig) {
    let mut interval =
        tokio::time::interval(Duration::from_secs(config.runtime.heartbeat_interval));

    loop {
        interval.tick().await;

        let req = HeartbeatRequest {
            current_load: 0,
            status: NodeStatus::Online,
        };

        if let Err(e) = client.heartbeat(req).await {
            eprintln!("Heartbeat failed: {}", e);
        }
    }
}
