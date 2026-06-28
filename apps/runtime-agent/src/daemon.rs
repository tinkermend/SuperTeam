use std::collections::BTreeSet;
use std::time::Duration;

use anyhow::{Context, Result};
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
use crate::health::{ProviderHealth, ProviderHealthProbe, probe_provider_health};
use crate::providers::catalog;
use crate::runtime_auth::{RuntimeAuthState, RuntimeAuthStatus, is_runtime_auth_expired};
use crate::tools::probe_tool;

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
        let capabilities = build_capabilities(&self.config, &self.config.tools.probe_names).await;
        let auth = RuntimeAuthState::new(self.config.runtime.node_id.clone());

        loop {
            if establish_runtime_auth_session(&self.config, capabilities.clone(), auth.clone())
                .await?
            {
                break;
            }
            tokio::time::sleep(Duration::from_secs(30)).await;
        }

        let supervisor_config = self.config.clone();
        let supervisor_capabilities = capabilities.clone();
        let supervisor_auth = auth.clone();
        tokio::spawn(async move {
            session_supervisor_loop(supervisor_config, supervisor_capabilities, supervisor_auth)
                .await;
        });

        let control_plane = ControlPlaneClient::with_runtime_auth(
            self.config.runtime.control_plane_url.clone(),
            auth,
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
}

pub async fn reenroll_runtime_session(
    config: &RuntimeConfig,
    auth: RuntimeAuthState,
    capabilities: Vec<RuntimeCapabilityInput>,
) -> Result<bool> {
    auth.mark_status(RuntimeAuthStatus::Reauthenticating).await;
    establish_runtime_auth_session(config, capabilities, auth).await
}

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
            auth.set_session(
                credentials.session_id,
                credentials.token,
                session.expires_at,
            )
            .await?;
            Ok(())
        }
        Err(error) if is_runtime_auth_expired(error.as_ref()) => {
            if !reenroll_runtime_session(config, auth, capabilities).await? {
                anyhow::bail!("runtime enrollment is not approved");
            }
            Ok(())
        }
        Err(error) => {
            auth.mark_status(RuntimeAuthStatus::Connected).await;
            Err(error)
        }
    }
}

async fn session_supervisor_loop(
    config: RuntimeConfig,
    capabilities: Vec<RuntimeCapabilityInput>,
    auth: RuntimeAuthState,
) {
    let mut last_seen_reauth_generation = auth.snapshot().await.reauth_generation;
    loop {
        let snapshot = auth.snapshot().await;
        let delay = snapshot
            .expires_at
            .map(|expires_at| refresh_delay(expires_at, auth.refresh_margin()))
            .unwrap_or(Duration::from_secs(30));

        tokio::select! {
            _ = tokio::time::sleep(delay) => {
                if let Err(error) =
                    renew_or_reenroll_once(&config, auth.clone(), capabilities.clone()).await
                {
                    eprintln!("Runtime session refresh failed: {}", error);
                    if auth.snapshot().await.status == RuntimeAuthStatus::Refreshing {
                        auth.mark_status(RuntimeAuthStatus::Connected).await;
                    }
                    tokio::time::sleep(Duration::from_secs(30)).await;
                }
            }
            _ = auth.wait_for_next_reauth_event(last_seen_reauth_generation) => {
                loop {
                    match reenroll_runtime_session(&config, auth.clone(), capabilities.clone()).await {
                        Ok(true) => {
                            last_seen_reauth_generation = auth.snapshot().await.reauth_generation;
                            break;
                        }
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

fn refresh_delay(expires_at: OffsetDateTime, refresh_margin: Duration) -> Duration {
    let renew_at = expires_at - time::Duration::seconds(refresh_margin.as_secs() as i64);
    let now = OffsetDateTime::now_utc();
    if renew_at <= now {
        return Duration::ZERO;
    }

    (renew_at - now).try_into().unwrap_or(Duration::ZERO)
}

pub fn spawn_runtime_auth_renewal_loop(
    config: RuntimeConfig,
    auth: RuntimeAuthState,
    capabilities: Vec<RuntimeCapabilityInput>,
) -> JoinHandle<()> {
    tokio::spawn(async move {
        runtime_auth_renewal_loop(config, auth, capabilities).await;
    })
}

async fn runtime_auth_renewal_loop(
    config: RuntimeConfig,
    auth: RuntimeAuthState,
    capabilities: Vec<RuntimeCapabilityInput>,
) {
    loop {
        let expires_at = auth
            .snapshot()
            .await
            .expires_at
            .map(|value| value.format(&Rfc3339).expect("RFC3339 format"));
        let delay = session_renewal_delay(expires_at.as_deref());
        if !delay.is_zero() {
            tokio::time::sleep(delay).await;
        }

        if let Err(error) =
            renew_or_reenroll_once(&config, auth.clone(), capabilities.clone()).await
        {
            eprintln!("Runtime session renew failed: {}", error);
            tokio::time::sleep(SESSION_RENEWAL_RETRY_DELAY).await;
        }
    }
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
    catalog::supported_provider_types(config)
}

pub async fn build_capabilities(
    config: &RuntimeConfig,
    probe_tools: &[String],
) -> Vec<RuntimeCapabilityInput> {
    let mut capabilities = Vec::new();

    for descriptor in catalog::configured_provider_descriptors() {
        let section = catalog::provider_section(config, descriptor.provider_type)
            .expect("catalog descriptor must have config section");
        let health = if section.enabled {
            Some(
                probe_provider_health(ProviderHealthProbe {
                    kind: descriptor.health_kind.to_string(),
                    bin_path: section.binary_path.clone(),
                })
                .await,
            )
        } else {
            None
        };
        capabilities.push(provider_capability(
            descriptor.provider_type,
            section.enabled,
            section.binary_path.display().to_string(),
            config.workspace.base_dir.display().to_string(),
            health,
        ));
    }

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

    for name in normalize_tool_set(probe_tools.iter().cloned()) {
        let probe = probe_tool(&name);
        let status = if probe.available {
            "available"
        } else {
            "missing"
        }
        .to_string();
        capabilities.push(RuntimeCapabilityInput {
            capability_type: "tool".to_string(),
            capability_key: probe.name,
            provider_type: "tool".to_string(),
            provider_version: None,
            binary_path: probe
                .binary_path
                .as_ref()
                .map(|path| path.display().to_string()),
            available: probe.available,
            workspace_base_dir: None,
            capacity: None,
            labels: None,
            status: status.clone(),
            details: None,
            health_status: status,
            metadata: None,
        });
    }

    capabilities
}

fn provider_capability(
    provider_type: &str,
    enabled: bool,
    binary_path: String,
    workspace_base_dir: String,
    health: Option<ProviderHealth>,
) -> RuntimeCapabilityInput {
    let available = enabled && health.as_ref().is_some_and(|probe| probe.available);
    let provider_version = health.as_ref().and_then(|probe| probe.version.clone());
    let mut details = std::collections::HashMap::new();
    if let Some(error) = health.as_ref().and_then(|probe| probe.error.clone()) {
        details.insert("error".to_string(), json!(error));
    }

    RuntimeCapabilityInput {
        capability_type: "provider".to_string(),
        capability_key: provider_type.to_string(),
        provider_type: provider_type.to_string(),
        provider_version,
        binary_path: Some(binary_path),
        available,
        workspace_base_dir: Some(workspace_base_dir),
        capacity: None,
        labels: None,
        status: if !enabled {
            "disabled"
        } else if available {
            "healthy"
        } else {
            "unavailable"
        }
        .to_string(),
        details: if details.is_empty() {
            None
        } else {
            Some(details)
        },
        health_status: if !enabled {
            "disabled"
        } else if available {
            "healthy"
        } else {
            "unhealthy"
        }
        .to_string(),
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
    let mut last_required_tools = normalize_tool_set(std::iter::empty::<String>());

    loop {
        interval.tick().await;

        let req = HeartbeatRequest {
            current_load: 0,
            status: NodeStatus::Online,
        };

        match client.heartbeat(req).await {
            Ok(response) => {
                let required_tools = normalize_tool_set(response.required_tools);
                if required_tools != last_required_tools {
                    let probe_tools = merge_tool_sets(&config.tools.probe_names, &required_tools);
                    let capabilities = build_capabilities(&config, &probe_tools).await;
                    match client
                        .upsert_capabilities(&config.runtime.node_id, capabilities)
                        .await
                    {
                        Ok(_) => {
                            last_required_tools = required_tools;
                        }
                        Err(e) => {
                            eprintln!("Runtime capability upsert failed after heartbeat: {}", e);
                        }
                    }
                }
            }
            Err(e) => {
                eprintln!("Heartbeat failed: {}", e);
            }
        }
    }
}

fn normalize_tool_set<I>(names: I) -> Vec<String>
where
    I: IntoIterator<Item = String>,
{
    names
        .into_iter()
        .map(|name| name.trim().to_string())
        .filter(|name| !name.is_empty())
        .collect::<BTreeSet<_>>()
        .into_iter()
        .collect()
}

fn merge_tool_sets(baseline: &[String], required: &[String]) -> Vec<String> {
    normalize_tool_set(baseline.iter().cloned().chain(required.iter().cloned()))
}
