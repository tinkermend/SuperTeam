#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeConfig {
    pub node_id: String,
}

impl RuntimeConfig {
    pub fn new(node_id: impl Into<String>) -> anyhow::Result<Self> {
        let node_id = node_id.into().trim().to_string();
        if node_id.is_empty() {
            anyhow::bail!("node id is required");
        }
        Ok(Self { node_id })
    }
}
