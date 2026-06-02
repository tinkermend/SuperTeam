use std::time::Duration;

use anyhow::{Context, Result};
use futures_util::StreamExt;
use http::HeaderValue;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::config::RuntimeConfig;
use crate::controlplane::models::{EnsureInstanceCommand, RuntimeCommand, RuntimeCommandType};
use crate::instances::{EnsureInstanceRequest, ensure_instance};

const COMMAND_LOOP_RECONNECT_DELAY: Duration = Duration::from_secs(5);

pub async fn run_command_loop(config: RuntimeConfig, session_token: String) -> Result<()> {
    let ws_url = runtime_ws_url(&config.runtime.control_plane_url)?;
    let authorization = HeaderValue::from_str(&format!("Bearer {session_token}"))
        .context("runtime session token is not a valid websocket authorization header")?;

    loop {
        if let Err(error) = run_command_loop_once(&config, &ws_url, &authorization).await {
            eprintln!("Runtime command loop connection failed: {}", error);
        }
        tokio::time::sleep(COMMAND_LOOP_RECONNECT_DELAY).await;
    }
}

async fn run_command_loop_once(
    config: &RuntimeConfig,
    ws_url: &str,
    authorization: &HeaderValue,
) -> Result<()> {
    let mut request = ws_url.into_client_request()?;
    request
        .headers_mut()
        .insert("Authorization", authorization.clone());

    let (mut socket, _) = connect_async(request).await?;
    while let Some(message) = socket.next().await {
        let message = match message {
            Ok(message) => message,
            Err(error) => {
                eprintln!("Runtime command websocket read failed: {}", error);
                break;
            }
        };
        if !message.is_text() {
            continue;
        }
        let text = match message.to_text() {
            Ok(text) => text,
            Err(error) => {
                eprintln!("Runtime command websocket text decode failed: {}", error);
                continue;
            }
        };
        if let Err(error) = handle_text_command(config, text) {
            eprintln!("Runtime command handling failed: {}", error);
        }
    }
    Ok(())
}

fn handle_text_command(config: &RuntimeConfig, text: &str) -> Result<()> {
    let command: RuntimeCommand = serde_json::from_str(text)?;
    handle_command(config, command)
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
        | RuntimeCommandType::StopSession
        | RuntimeCommandType::Unsupported(_) => {}
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
    use super::{handle_text_command, run_command_loop_once, runtime_ws_url};
    use crate::config::RuntimeConfig;
    use futures_util::SinkExt;
    use http::HeaderValue;
    use tokio::net::TcpListener;
    use tokio_tungstenite::accept_hdr_async;
    use tokio_tungstenite::tungstenite::Message;
    use tokio_tungstenite::tungstenite::handshake::server::{Request, Response};

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

    #[test]
    fn handle_text_command_ignores_unsupported_command_types() {
        let config = RuntimeConfig::new("node-1").expect("config");

        handle_text_command(
            &config,
            r#"{"id":"cmd-legacy","type":"task.claim","payload":{}}"#,
        )
        .expect("unsupported command should be ignored");
    }

    #[tokio::test]
    async fn command_loop_continues_after_bad_commands() {
        let temp = tempfile::tempdir().expect("tempdir");
        let execution_instance_id = "11111111-1111-4111-8111-111111111111";
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
        let addr = listener.local_addr().expect("local addr");

        let server = tokio::spawn(async move {
            let (stream, _) = listener.accept().await.expect("accept");
            let callback = |request: &Request, response: Response| {
                assert_eq!(request.uri().path(), "/api/v1/runtime/ws");
                assert_eq!(
                    request.headers().get("Authorization"),
                    Some(&HeaderValue::from_static("Bearer session-token"))
                );
                Ok(response)
            };
            let mut socket = accept_hdr_async(stream, callback).await.expect("ws accept");
            socket
                .send(Message::Text("{bad-json".into()))
                .await
                .expect("send bad json");
            socket
                .send(Message::Text(
                    r#"{"id":"cmd-bad","type":"ensure_instance","payload":{}}"#.into(),
                ))
                .await
                .expect("send bad command");
            socket
                .send(Message::Text(
                    format!(
                        r#"{{"id":"cmd-good","type":"ensure_instance","payload":{{"execution_instance_id":"{execution_instance_id}"}}}}"#
                    )
                    .into(),
                ))
                .await
                .expect("send valid command");
            socket.close(None).await.expect("close socket");
        });

        let mut config = RuntimeConfig::new("node-1").expect("config");
        config.runtime.control_plane_url = format!("http://{addr}");
        config.workspace.base_dir = temp.path().to_path_buf();
        let authorization = HeaderValue::from_static("Bearer session-token");
        run_command_loop_once(
            &config,
            &runtime_ws_url(&config.runtime.control_plane_url).expect("ws url"),
            &authorization,
        )
        .await
        .expect("command loop once");
        server.await.expect("server task");

        let agent_home_dir = temp.path().join("agents").join(execution_instance_id);
        assert!(agent_home_dir.join("state").is_dir());
        assert!(agent_home_dir.join("sessions").is_dir());
        assert!(agent_home_dir.join("runs").is_dir());
    }
}
