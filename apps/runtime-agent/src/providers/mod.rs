pub mod claude;
pub mod opencode;

use std::path::PathBuf;

use async_trait::async_trait;
use futures::Stream;

use crate::events::ProviderEvent;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProviderRequest {
    pub prompt: String,
    pub workspace_path: PathBuf,
    pub session_id: Option<String>,
    pub continue_session: bool,
    pub model: Option<String>,
}

#[async_trait]
pub trait ProviderAdapter {
    async fn run(
        &self,
        request: ProviderRequest,
    ) -> anyhow::Result<Box<dyn Stream<Item = anyhow::Result<ProviderEvent>> + Send + Unpin>>;
}
