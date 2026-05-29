use std::time::Duration;

use anyhow::Result;

use crate::config::RuntimeConfig;
use crate::controlplane::client::ControlPlaneClient;
use crate::controlplane::models::{HeartbeatRequest, NodeStatus, RegisterNodeRequest};
use crate::executor::TaskExecutor;

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
        let client = ControlPlaneClient::with_node_id(
            &self.config.runtime.control_plane_url,
            &self.config.runtime.auth_token,
            &self.config.runtime.node_id,
        );

        let supported_providers = build_supported_providers(&self.config);

        let register_req = RegisterNodeRequest {
            node_id: self.config.runtime.node_id.clone(),
            name: format!("runtime-{}", self.config.runtime.node_id),
            supported_providers,
            max_slots: self.config.runtime.max_concurrent_tasks as i32,
            metadata: None,
        };

        client.register(register_req).await?;
        println!("Node registered successfully");

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
