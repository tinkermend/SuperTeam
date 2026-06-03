use std::time::Duration;

use anyhow::{Context, Result};
use futures_util::StreamExt;
use http::HeaderValue;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::commands::executor::RuntimeCommandExecutor;
use crate::config::RuntimeConfig;
use crate::controlplane::models::RuntimeCommand;

const COMMAND_LOOP_RECONNECT_DELAY: Duration = Duration::from_secs(5);

pub async fn run_command_loop(config: RuntimeConfig, session_token: String) -> Result<()> {
    let ws_url = runtime_ws_url(&config.runtime.control_plane_url)?;
    let authorization = HeaderValue::from_str(&format!("Bearer {session_token}"))
        .context("runtime session token is not a valid websocket authorization header")?;
    let executor = RuntimeCommandExecutor::new(config);

    loop {
        if let Err(error) = run_command_loop_once(&executor, &ws_url, &authorization).await {
            eprintln!("Runtime command loop connection failed: {}", error);
        }
        tokio::time::sleep(COMMAND_LOOP_RECONNECT_DELAY).await;
    }
}

async fn run_command_loop_once(
    executor: &RuntimeCommandExecutor,
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
        if let Err(error) = handle_text_command(executor, text).await {
            eprintln!("Runtime command handling failed: {}", error);
        }
    }
    Ok(())
}

async fn handle_text_command(executor: &RuntimeCommandExecutor, text: &str) -> Result<()> {
    let command: RuntimeCommand = serde_json::from_str(text)?;
    executor.handle_command(command).await?;
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
    use crate::commands::executor::RuntimeCommandExecutor;
    use crate::config::RuntimeConfig;
    use crate::runs::{RunSnapshot, RunStatus, RuntimeRunStore};
    use futures_util::SinkExt;
    use http::HeaderValue;
    use serde_json::json;
    use std::fs;
    use std::os::unix::fs::PermissionsExt;
    use std::path::{Path, PathBuf};
    use std::time::Duration;
    use tempfile::TempDir;
    use tokio::net::TcpListener;
    use tokio_tungstenite::accept_hdr_async;
    use tokio_tungstenite::tungstenite::Message;
    use tokio_tungstenite::tungstenite::handshake::server::{Request, Response};

    const DIGITAL_EMPLOYEE_ID: &str = "11111111-1111-4111-8111-111111111111";
    const EXECUTION_INSTANCE_ID: &str = "22222222-2222-4222-8222-222222222222";

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

    #[tokio::test]
    async fn handle_text_command_ignores_unsupported_command_types() {
        let config = RuntimeConfig::new("node-1").expect("config");
        let executor = RuntimeCommandExecutor::new(config);

        handle_text_command(
            &executor,
            r#"{"id":"cmd-legacy","type":"task.claim","payload":{}}"#,
        )
        .await
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
        let executor = RuntimeCommandExecutor::new(config.clone());
        let authorization = HeaderValue::from_static("Bearer session-token");
        run_command_loop_once(
            &executor,
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

    #[tokio::test]
    async fn command_loop_executes_start_session_commands() {
        let temp = TempDir::new().expect("tempdir");
        let fake_claude = make_script(
            temp.path(),
            "fake-claude",
            r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-from-ws-command"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"hello from websocket command"}]}}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
        );
        let mut config = RuntimeConfig::new("node-1").expect("config");
        config.runs.log_dir = temp.path().join("run-logs");
        config.workspace.base_dir = temp.path().join("workspaces");
        config.providers.claude_code.enabled = true;
        config.providers.claude_code.binary_path = fake_claude;
        config.providers.opencode.enabled = false;
        config.providers.opencode.binary_path = temp.path().join("missing-opencode");

        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
        let addr = listener.local_addr().expect("local addr");
        config.runtime.control_plane_url = format!("http://{addr}");

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
                .send(Message::Text(
                    json!({
                        "id": "cmd-ws-start",
                        "type": "start_session",
                        "payload": {
                            "command_id": "cmd-ws-start",
                            "digital_employee_id": DIGITAL_EMPLOYEE_ID,
                            "execution_instance_id": EXECUTION_INSTANCE_ID,
                            "provider_type": "claude-code",
                            "session_policy": {
                                "mode": "new",
                                "provider_session_id": null,
                                "recoverable": true
                            },
                            "prompt": "write a websocket summary",
                            "input": null,
                            "context_refs": [],
                            "artifact_refs": [],
                            "model": null,
                            "metadata": {"source": "ws-test"}
                        }
                    })
                    .to_string()
                    .into(),
                ))
                .await
                .expect("send start_session command");
            tokio::time::sleep(Duration::from_millis(25)).await;
            socket.close(None).await.expect("close socket");
        });

        let executor = RuntimeCommandExecutor::new(config.clone());
        let authorization = HeaderValue::from_static("Bearer session-token");
        run_command_loop_once(
            &executor,
            &runtime_ws_url(&config.runtime.control_plane_url).expect("ws url"),
            &authorization,
        )
        .await
        .expect("command loop once");
        server.await.expect("server task");

        let run_id = wait_for_command_run(&executor, "cmd-ws-start").await;
        let snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;
        assert_eq!(
            snapshot.provider_session_id.as_deref(),
            Some("session-from-ws-command")
        );
        let command_context = snapshot.command_context.expect("command context");
        assert_eq!(command_context.command_id, "cmd-ws-start");
    }

    fn make_script(dir: &Path, name: &str, body: &str) -> PathBuf {
        let path = dir.join(name);
        fs::write(&path, body).expect("write fake provider script");
        let mut permissions = fs::metadata(&path).expect("metadata").permissions();
        permissions.set_mode(0o755);
        fs::set_permissions(&path, permissions).expect("chmod fake provider script");
        path
    }

    async fn wait_for_command_run(executor: &RuntimeCommandExecutor, command_id: &str) -> String {
        for _ in 0..100 {
            if let Some(run_id) = executor.registry().run_for_command(command_id) {
                return run_id;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }

        panic!("run was not registered for command {command_id}");
    }

    async fn wait_for_status(
        runs: &RuntimeRunStore,
        run_id: &str,
        expected: RunStatus,
    ) -> RunSnapshot {
        for _ in 0..100 {
            if let Some(snapshot) = runs.get_run(run_id).await {
                if snapshot.status == expected {
                    return snapshot;
                }
                if matches!(snapshot.status, RunStatus::Failed)
                    && !matches!(expected, RunStatus::Failed)
                {
                    panic!("run {run_id} failed unexpectedly: {:?}", snapshot.error);
                }
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }

        let snapshot = runs
            .get_run(run_id)
            .await
            .unwrap_or_else(|| panic!("run {run_id} not found"));
        panic!(
            "run {run_id} did not reach {:?}; latest status: {:?}",
            expected, snapshot.status
        );
    }
}
