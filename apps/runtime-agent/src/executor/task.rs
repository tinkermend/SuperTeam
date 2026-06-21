use anyhow::Result;
use futures::StreamExt;
use tokio_util::sync::CancellationToken;

use crate::config::RuntimeConfig;
use crate::controlplane::{ControlPlaneClient, models::TaskStatus};
use crate::providers::{ProviderAdapter, ProviderRequest, catalog};

use super::retry::push_event_with_retry;
use super::workspace::{cleanup_run_workspace, create_run_workspace};

pub async fn execute_task(
    task: crate::controlplane::models::Task,
    control_plane: ControlPlaneClient,
    config: RuntimeConfig,
    cancel_token: CancellationToken,
) -> Result<()> {
    // 使用新的工作目录结构，暂用 task_id 模拟 execution_instance_id
    let execution_instance_id = format!("instance-{}", task.id);
    let run_id = format!("run-{}", task.id);
    let workspace = create_run_workspace(&config, &execution_instance_id, &run_id)?;

    control_plane
        .update_task_status(task.id, TaskStatus::Running)
        .await?;

    let provider = select_provider(&task.provider_type, &config)?;

    let request = ProviderRequest {
        prompt: extract_prompt(&task.params)?,
        workspace_path: workspace.workspace_path.clone(),
        session_id: None,
        continue_session: false,
        model: extract_model(&task.params),
        environment: Default::default(),
    };

    let mut event_stream = provider.run(request).await?;

    while let Some(event_result) = event_stream.next().await {
        if cancel_token.is_cancelled() {
            let _ = control_plane
                .fail_task(task.id, "Task cancelled".to_string())
                .await;
            cleanup_run_workspace(&workspace, &config)?;
            return Err(anyhow::anyhow!("Task cancelled"));
        }

        match event_result {
            Ok(event) => {
                if let Err(e) = push_event_with_retry(&control_plane, task.id, event).await {
                    let _ = control_plane
                        .fail_task(task.id, format!("Failed to push events: {}", e))
                        .await;
                    cleanup_run_workspace(&workspace, &config)?;
                    return Err(e);
                }
            }
            Err(e) => {
                let _ = control_plane
                    .fail_task(task.id, format!("Provider execution failed: {}", e))
                    .await;
                cleanup_run_workspace(&workspace, &config)?;
                return Err(e);
            }
        }
    }

    control_plane
        .complete_task(task.id, serde_json::json!({"status": "success"}))
        .await?;
    cleanup_run_workspace(&workspace, &config)?;

    Ok(())
}

fn select_provider(
    provider_type: &str,
    config: &RuntimeConfig,
) -> Result<Box<dyn ProviderAdapter>> {
    catalog::select_provider(config, provider_type)
        .map_err(|error| anyhow::anyhow!("Unsupported provider type: {provider_type}: {error}"))
}

fn extract_prompt(params: &serde_json::Value) -> Result<String> {
    params
        .get("prompt")
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
        .ok_or_else(|| anyhow::anyhow!("Missing 'prompt' in task params"))
}

fn extract_model(params: &serde_json::Value) -> Option<String> {
    params
        .get("model")
        .and_then(|v| v.as_str())
        .map(|s| s.to_string())
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;

    fn runtime_config_with_codex(enabled: bool) -> RuntimeConfig {
        let mut config = RuntimeConfig::default();
        config.providers.claude_code.enabled = false;
        config.providers.opencode.enabled = false;
        config.providers.codex.enabled = enabled;
        config.providers.codex.binary_path = PathBuf::from("codex-test");
        config
    }

    #[test]
    fn select_provider_uses_catalog_for_enabled_codex() {
        let config = runtime_config_with_codex(true);

        let provider = select_provider("codex", &config);

        assert!(provider.is_ok());
    }

    #[test]
    fn select_provider_preserves_catalog_error_for_disabled_codex() {
        let config = runtime_config_with_codex(false);

        let error = match select_provider("codex", &config) {
            Ok(_) => panic!("disabled codex should fail"),
            Err(error) => error,
        };

        assert_eq!(
            error.to_string(),
            "Unsupported provider type: codex: Codex provider is disabled"
        );
    }
}
