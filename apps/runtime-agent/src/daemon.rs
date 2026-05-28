use crate::config::RuntimeConfig;

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
            node_id: self.config.node_id.clone(),
            status: "idle".to_string(),
        }
    }
}
