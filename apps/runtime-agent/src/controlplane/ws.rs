use anyhow::{Context, Result};
use futures_util::StreamExt;
use http::HeaderValue;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::config::RuntimeConfig;
use crate::controlplane::models::{EnsureInstanceCommand, RuntimeCommand, RuntimeCommandType};
use crate::instances::{EnsureInstanceRequest, ensure_instance};

pub async fn run_command_loop(config: RuntimeConfig, session_token: String) -> Result<()> {
    let ws_url = runtime_ws_url(&config.runtime.control_plane_url)?;
    let mut request = ws_url.into_client_request()?;
    request.headers_mut().insert(
        "Authorization",
        HeaderValue::from_str(&format!("Bearer {session_token}"))
            .context("runtime session token is not a valid websocket authorization header")?,
    );

    let (mut socket, _) = connect_async(request).await?;
    while let Some(message) = socket.next().await {
        let message = message?;
        if !message.is_text() {
            continue;
        }
        let command: RuntimeCommand = serde_json::from_str(message.to_text()?)?;
        handle_command(&config, command)?;
    }
    Ok(())
}

fn handle_command(config: &RuntimeConfig, command: RuntimeCommand) -> Result<()> {
    match command.command_type {
        RuntimeCommandType::EnsureInstance => {
            let payload: EnsureInstanceCommand = serde_json::from_value(command.payload)?;
            ensure_instance(EnsureInstanceRequest {
                base_dir: config.workspace.base_dir.clone(),
                execution_instance_id: payload.execution_instance_id,
            })?;
        }
        RuntimeCommandType::StartSession
        | RuntimeCommandType::ResumeSession
        | RuntimeCommandType::SendInput
        | RuntimeCommandType::StopSession => {}
    }
    Ok(())
}

fn runtime_ws_url(control_plane_url: &str) -> Result<String> {
    let base = control_plane_url.trim_end_matches('/');
    if let Some(rest) = base.strip_prefix("http://") {
        return Ok(format!("ws://{rest}/api/v1/runtime/ws"));
    }
    if let Some(rest) = base.strip_prefix("https://") {
        return Ok(format!("wss://{rest}/api/v1/runtime/ws"));
    }
    anyhow::bail!("control_plane_url must start with http:// or https://");
}

#[cfg(test)]
mod tests {
    use super::runtime_ws_url;

    #[test]
    fn runtime_ws_url_uses_runtime_ws_endpoint() {
        assert_eq!(
            runtime_ws_url("http://control-plane.local/").expect("ws url"),
            "ws://control-plane.local/api/v1/runtime/ws"
        );
        assert_eq!(
            runtime_ws_url("https://control-plane.local").expect("ws url"),
            "wss://control-plane.local/api/v1/runtime/ws"
        );
    }
}
