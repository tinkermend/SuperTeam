use anyhow::Result;
use futures::StreamExt;
use tokio_util::sync::CancellationToken;

use crate::config::RuntimeConfig;
use crate::controlplane::{ControlPlaneClient, models::TaskStatus};
use crate::providers::{
    ProviderAdapter, ProviderRequest, claude::ClaudeProvider, opencode::OpenCodeProvider,
};

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
    match provider_type {
        "claude-code" => {
            if !config.providers.claude_code.enabled {
                anyhow::bail!("Claude Code provider is disabled");
            }
            Ok(Box::new(ClaudeProvider::new(
                &config.providers.claude_code.binary_path,
            )))
        }
        "opencode" => {
            if !config.providers.opencode.enabled {
                anyhow::bail!("OpenCode provider is disabled");
            }
            Ok(Box::new(OpenCodeProvider::new(
                &config.providers.opencode.binary_path,
            )))
        }
        _ => anyhow::bail!("Unsupported provider type: {}", provider_type),
    }
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
