use std::time::Duration;

use anyhow::Result;
use serde_json::json;
use time::{OffsetDateTime, format_description::well_known::Rfc3339};
use tokio::task::JoinHandle;

use crate::config::RuntimeConfig;
use crate::controlplane::client::ControlPlaneClient;
use crate::controlplane::models::{
    EnrollHelloRequest, EnrollmentStatus, HeartbeatRequest, NodeStatus, RuntimeCapabilityInput,
};
use crate::controlplane::ws::run_command_loop;
use crate::executor::TaskExecutor;
use crate::session::RuntimeSession;

const SESSION_RENEWAL_MARGIN: Duration = Duration::from_secs(5 * 60);
const SESSION_RENEWAL_RETRY_DELAY: Duration = Duration::from_secs(30);
const SESSION_RENEWAL_FALLBACK_DELAY: Duration = Duration::from_secs(60 * 60);

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeSnapshot {
    pub node_id: String,
    pub status: String,
}

#[derive(Debug, Clone)]
pub struct RuntimeDaemon {
    config: RuntimeConfig,
}

struct RuntimeSessionContext {
    client: ControlPlaneClient,
    session_token: String,
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
        let capabilities = build_capabilities(&self.config);
        let Some(session_context) = establish_runtime_session(&self.config, capabilities).await?
        else {
            return Ok(());
        };

        let command_config = self.config.clone();
        let command_session_token = session_context.session_token.clone();
        tokio::spawn(async move {
            if let Err(error) = run_command_loop(command_config, command_session_token).await {
                eprintln!("Runtime command loop failed: {}", error);
            }
        });

        let heartbeat_client = session_context.client.clone();
        let heartbeat_config = self.config.clone();
        tokio::spawn(async move {
            heartbeat_loop(heartbeat_client, heartbeat_config).await;
        });

        let executor = TaskExecutor::new(self.config, session_context.client);
        executor.run().await?;

        Ok(())
    }
}

pub async fn connect_runtime_session(
    config: &RuntimeConfig,
    capabilities: Vec<RuntimeCapabilityInput>,
) -> Result<Option<ControlPlaneClient>> {
    Ok(establish_runtime_session(config, capabilities)
        .await?
        .map(|context| context.client))
}

async fn establish_runtime_session(
    config: &RuntimeConfig,
    capabilities: Vec<RuntimeCapabilityInput>,
) -> Result<Option<RuntimeSessionContext>> {
    let bootstrap_client = ControlPlaneClient::new(&config.runtime.control_plane_url, "");
    let supported_providers = build_supported_providers(config);

    let hello_req = EnrollHelloRequest {
        node_id: config.runtime.node_id.clone(),
        name: format!("runtime-{}", config.runtime.node_id),
        supported_providers,
        max_slots: config.runtime.max_concurrent_tasks as i32,
        bootstrap_key: config.runtime.bootstrap_key.clone(),
        version: Some(env!("CARGO_PKG_VERSION").to_string()),
        metadata: Some(runtime_metadata()),
        capabilities: capabilities.clone(),
    };

    let hello = bootstrap_client.enroll_hello(hello_req).await?;
    if hello.enrollment.status != EnrollmentStatus::Approved {
        println!(
            "runtime-agent node={} enrollment={}; waiting for approval",
            config.runtime.node_id,
            enrollment_status_label(&hello.enrollment.status)
        );
        return Ok(None);
    }

    let Some(session_response) = hello.session.as_ref() else {
        anyhow::bail!("approved runtime enrollment did not return a session");
    };
    let session = RuntimeSession::new(
        hello.session_token.unwrap_or_default(),
        Some(session_response.expires_at.clone()),
    );
    if session.is_empty() {
        anyhow::bail!("approved runtime enrollment did not return a session token");
    }

    let client = ControlPlaneClient::with_session_token(
        &config.runtime.control_plane_url,
        &session.token,
        &config.runtime.node_id,
    );
    client
        .upsert_capabilities(&config.runtime.node_id, capabilities)
        .await?;
    spawn_session_renewal_loop(
        client.clone(),
        session_response.id.clone(),
        session.expires_at.clone(),
    );
    let session_token = session.token.clone();
    println!("Runtime session established");

    Ok(Some(RuntimeSessionContext {
        client,
        session_token,
    }))
}

pub fn spawn_session_renewal_loop(
    client: ControlPlaneClient,
    session_id: String,
    expires_at: Option<String>,
) -> JoinHandle<()> {
    tokio::spawn(async move {
        session_renewal_loop(client, session_id, expires_at).await;
    })
}

async fn session_renewal_loop(
    client: ControlPlaneClient,
    session_id: String,
    mut expires_at: Option<String>,
) {
    loop {
        let delay = session_renewal_delay(expires_at.as_deref());
        if !delay.is_zero() {
            tokio::time::sleep(delay).await;
        }

        match client.renew_session(&session_id).await {
            Ok(session) => {
                expires_at = Some(session.expires_at);
            }
            Err(error) => {
                eprintln!("Runtime session renew failed: {}", error);
                tokio::time::sleep(SESSION_RENEWAL_RETRY_DELAY).await;
            }
        }
    }
}

fn session_renewal_delay(expires_at: Option<&str>) -> Duration {
    let Some(expires_at) = expires_at else {
        return SESSION_RENEWAL_FALLBACK_DELAY;
    };
    let Ok(expires_at) = OffsetDateTime::parse(expires_at, &Rfc3339) else {
        return SESSION_RENEWAL_FALLBACK_DELAY;
    };

    let renew_at = expires_at - time::Duration::seconds(SESSION_RENEWAL_MARGIN.as_secs() as i64);
    let now = OffsetDateTime::now_utc();
    if renew_at <= now {
        return Duration::ZERO;
    }

    (renew_at - now)
        .try_into()
        .unwrap_or(SESSION_RENEWAL_FALLBACK_DELAY)
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
