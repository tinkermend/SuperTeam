use std::time::Duration;

use anyhow::{Context, Result};
use futures_util::StreamExt;
use http::HeaderValue;
use tokio_tungstenite::connect_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::commands::executor::RuntimeCommandExecutor;
use crate::config::RuntimeConfig;
use crate::controlplane::ControlPlaneClient;
use crate::controlplane::models::RuntimeCommand;

const COMMAND_LOOP_RECONNECT_DELAY: Duration = Duration::from_secs(5);

pub async fn run_command_loop(config: RuntimeConfig, session_token: String) -> Result<()> {
    let ws_url = runtime_ws_url(&config.runtime.control_plane_url)?;
    let authorization = HeaderValue::from_str(&format!("Bearer {session_token}"))
        .context("runtime session token is not a valid websocket authorization header")?;
    let control_plane = ControlPlaneClient::with_session_token(
        config.runtime.control_plane_url.clone(),
        session_token,
        config.runtime.node_id.clone(),
    );
    let executor = RuntimeCommandExecutor::with_control_plane_client(config, control_plane);

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
    use crate::controlplane::ControlPlaneClient;
    use crate::controlplane::models::{RuntimeCommand, RuntimeCommandType};
    use crate::runs::{RunSnapshot, RunStatus, RuntimeRunStore};
    use axum::extract::{Path as AxumPath, State};
    use axum::http::{HeaderMap, StatusCode};
    use axum::routing::post;
    use axum::{Json, Router};
    use futures_util::SinkExt;
    use http::HeaderValue;
    use serde_json::Value;
    use serde_json::json;
    use std::fs;
    use std::os::unix::fs::PermissionsExt;
    use std::path::{Path, PathBuf};
    use std::sync::{Arc, Mutex};
    use std::time::Duration;
    use tempfile::TempDir;
    use tokio::net::TcpListener;
    use tokio_tungstenite::accept_hdr_async;
    use tokio_tungstenite::tungstenite::Message;
    use tokio_tungstenite::tungstenite::handshake::server::{Request, Response};

    const DIGITAL_EMPLOYEE_ID: &str = "11111111-1111-4111-8111-111111111111";
    const EXECUTION_INSTANCE_ID: &str = "22222222-2222-4222-8222-222222222222";
    const TENANT_ID: &str = "00000000-0000-4000-8000-000000000001";
    const TEAM_ID: &str = "33333333-3333-4333-8333-333333333333";
    const RUNTIME_NODE_ID: &str = "44444444-4444-4444-8444-444444444444";
    const AGENT_HOME_DIR: &str = "/tmp/superteam-runtime-agent/ws-test-agent";

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
        let digital_employee_id = "11111111-1111-4111-8111-111111111111";
        let team_id = "22222222-2222-4222-8222-222222222222";
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
                        r#"{{"id":"cmd-good","type":"ensure_instance","payload":{{"team_id":"{team_id}","digital_employee_id":"{digital_employee_id}"}}}}"#
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

        let agent_home_dir = temp.path().join("agents").join(digital_employee_id);
        assert!(agent_home_dir.join("state").is_dir());
        assert!(agent_home_dir.join("sessions").is_dir());
        assert!(agent_home_dir.join("runs").is_dir());
    }

    #[tokio::test]
    async fn command_loop_executes_start_session_commands() {
        let temp = TempDir::new().expect("tempdir");
        let capture = CommandWritebackCapture::default();
        let http_server = serve_command_writebacks(capture.clone()).await;
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
        config.runtime.control_plane_url = format!("http://{}", http_server.addr);
        config.providers.claude_code.enabled = true;
        config.providers.claude_code.binary_path = fake_claude;
        config.providers.opencode.enabled = false;
        config.providers.opencode.binary_path = temp.path().join("missing-opencode");

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
                .send(Message::Text(
                    json!({
                        "id": "cmd-ws-start",
                        "type": "start_session",
                        "payload": {
                            "command_id": "cmd-ws-start",
                            "tenant_id": TENANT_ID,
                            "team_id": TEAM_ID,
                            "digital_employee_id": DIGITAL_EMPLOYEE_ID,
                            "execution_instance_id": EXECUTION_INSTANCE_ID,
                            "runtime_node_id": RUNTIME_NODE_ID,
                            "provider_type": "claude-code",
                            "agent_home_dir": AGENT_HOME_DIR,
                            "workspace_files": [],
                            "skills": [],
                            "mcp_servers": [],
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

        let control_plane = ControlPlaneClient::with_session_token(
            format!("http://{}", http_server.addr),
            "session-token",
            "node-1",
        );
        let executor =
            RuntimeCommandExecutor::with_control_plane_client(config.clone(), control_plane);
        let authorization = HeaderValue::from_static("Bearer session-token");
        run_command_loop_once(
            &executor,
            &format!("ws://{addr}/api/v1/runtime/ws"),
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

        let complete = wait_for_writeback(capture.complete.clone()).await;
        assert_eq!(complete.command_id, "cmd-ws-start");
        assert_eq!(complete.payload["status"], "completed");
        assert_eq!(
            complete.payload["provider_session_external_id"],
            "session-from-ws-command"
        );
        let events = wait_for_event_writebacks(capture.events.clone(), 3).await;
        assert_eq!(events[0].payload["event_type"], "session_started");
        assert_eq!(events[1].payload["event_type"], "text_delta");
        assert_eq!(events[2].payload["event_type"], "turn_completed");
        assert_eq!(
            events[0].payload["provider_session_external_id"],
            "session-from-ws-command"
        );

        http_server.task.abort();
    }

    #[tokio::test]
    async fn command_loop_executes_provision_instance_and_writes_completion() {
        let temp = TempDir::new().expect("tempdir");
        let capture = CommandWritebackCapture::default();
        let http_server = serve_command_writebacks(capture.clone()).await;

        let mut config = RuntimeConfig::new("node-1").expect("config");
        config.runtime.control_plane_url = format!("http://{}", http_server.addr);
        config.workspace.base_dir = temp.path().join("workspaces");

        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
        let ws_addr = listener.local_addr().expect("local addr");
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
                        "id": "cmd-provision",
                        "type": "provision_instance",
                        "payload": {
                            "command_id": "cmd-provision",
                            "team_id": TEAM_ID,
                            "digital_employee_id": DIGITAL_EMPLOYEE_ID
                        }
                    })
                    .to_string()
                    .into(),
                ))
                .await
                .expect("send provision_instance command");
            tokio::time::sleep(Duration::from_millis(25)).await;
            socket.close(None).await.expect("close socket");
        });

        let control_plane = ControlPlaneClient::with_session_token(
            format!("http://{}", http_server.addr),
            "session-token",
            "node-1",
        );
        let executor =
            RuntimeCommandExecutor::with_control_plane_client(config.clone(), control_plane);
        let authorization = HeaderValue::from_static("Bearer session-token");
        run_command_loop_once(
            &executor,
            &format!("ws://{ws_addr}/api/v1/runtime/ws"),
            &authorization,
        )
        .await
        .expect("command loop once");
        server.await.expect("server task");

        let agent_home_dir = config
            .workspace
            .base_dir
            .join("agents")
            .join(DIGITAL_EMPLOYEE_ID);
        assert!(agent_home_dir.join("state").is_dir());
        assert!(agent_home_dir.join("sessions").is_dir());
        assert!(agent_home_dir.join("runs").is_dir());

        let complete = wait_for_writeback(capture.complete.clone()).await;
        assert_eq!(complete.command_id, "cmd-provision");
        assert_eq!(
            complete.authorization.as_deref(),
            Some("Bearer session-token")
        );
        assert_eq!(complete.node_id.as_deref(), Some("node-1"));
        assert_eq!(complete.payload["status"], "completed");
        assert_eq!(
            complete.payload["result"]["agent_home_dir"],
            Value::String(agent_home_dir.to_string_lossy().to_string())
        );

        http_server.task.abort();
    }

    #[tokio::test]
    async fn provision_instance_failure_writes_failed_terminal() {
        let temp = TempDir::new().expect("tempdir");
        let capture = CommandWritebackCapture::default();
        let http_server = serve_command_writebacks(capture.clone()).await;

        let mut config = RuntimeConfig::new("node-1").expect("config");
        config.runtime.control_plane_url = format!("http://{}", http_server.addr);
        config.workspace.base_dir = temp.path().join("workspaces");
        let control_plane = ControlPlaneClient::with_session_token(
            format!("http://{}", http_server.addr),
            "session-token",
            "node-1",
        );
        let executor = RuntimeCommandExecutor::with_control_plane_client(config, control_plane);

        let error = executor
            .handle_command(RuntimeCommand {
                id: "cmd-provision-bad".to_string(),
                command_type: RuntimeCommandType::ProvisionInstance,
                payload: json!({
                    "team_id": TEAM_ID,
                    "digital_employee_id": "not-a-uuid"
                }),
            })
            .await
            .expect_err("invalid execution instance id should fail");
        assert!(error.to_string().contains("invalid execution instance id"));

        let failed = wait_for_writeback(capture.fail.clone()).await;
        assert_eq!(failed.command_id, "cmd-provision-bad");
        assert_eq!(
            failed.authorization.as_deref(),
            Some("Bearer session-token")
        );
        assert_eq!(failed.node_id.as_deref(), Some("node-1"));
        assert_eq!(failed.payload["status"], "failed");
        assert_eq!(failed.payload["error_code"], "provision_instance_failed");
        assert_eq!(failed.payload["error_family"], "runtime_provisioning");

        http_server.task.abort();
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

    #[derive(Clone, Default)]
    struct CommandWritebackCapture {
        events: Arc<Mutex<Vec<CapturedWriteback>>>,
        complete: Arc<Mutex<Option<CapturedWriteback>>>,
        fail: Arc<Mutex<Option<CapturedWriteback>>>,
    }

    #[derive(Clone, Debug)]
    struct CapturedWriteback {
        command_id: String,
        authorization: Option<String>,
        node_id: Option<String>,
        payload: Value,
    }

    struct CommandWritebackServer {
        addr: std::net::SocketAddr,
        task: tokio::task::JoinHandle<()>,
    }

    async fn serve_command_writebacks(capture: CommandWritebackCapture) -> CommandWritebackServer {
        let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
        let addr = listener.local_addr().expect("local addr");
        let app = Router::new()
            .route(
                "/api/v1/runtime/commands/{command_id}/complete",
                post(capture_complete_writeback),
            )
            .route(
                "/api/v1/runtime/commands/{command_id}/events",
                post(capture_event_writeback),
            )
            .route(
                "/api/v1/runtime/commands/{command_id}/fail",
                post(capture_fail_writeback),
            )
            .with_state(capture);
        let task = tokio::spawn(async move {
            axum::serve(listener, app).await.expect("serve writebacks");
        });
        CommandWritebackServer { addr, task }
    }

    async fn capture_complete_writeback(
        AxumPath(command_id): AxumPath<String>,
        State(capture): State<CommandWritebackCapture>,
        headers: HeaderMap,
        Json(payload): Json<Value>,
    ) -> StatusCode {
        *capture.complete.lock().expect("complete lock") = Some(CapturedWriteback {
            command_id,
            authorization: header_value(&headers, "authorization"),
            node_id: header_value(&headers, "x-node-id"),
            payload,
        });
        StatusCode::ACCEPTED
    }

    async fn capture_event_writeback(
        AxumPath(command_id): AxumPath<String>,
        State(capture): State<CommandWritebackCapture>,
        headers: HeaderMap,
        Json(payload): Json<Value>,
    ) -> StatusCode {
        capture
            .events
            .lock()
            .expect("events lock")
            .push(CapturedWriteback {
                command_id,
                authorization: header_value(&headers, "authorization"),
                node_id: header_value(&headers, "x-node-id"),
                payload,
            });
        StatusCode::ACCEPTED
    }

    async fn capture_fail_writeback(
        AxumPath(command_id): AxumPath<String>,
        State(capture): State<CommandWritebackCapture>,
        headers: HeaderMap,
        Json(payload): Json<Value>,
    ) -> StatusCode {
        *capture.fail.lock().expect("fail lock") = Some(CapturedWriteback {
            command_id,
            authorization: header_value(&headers, "authorization"),
            node_id: header_value(&headers, "x-node-id"),
            payload,
        });
        StatusCode::ACCEPTED
    }

    async fn wait_for_writeback(slot: Arc<Mutex<Option<CapturedWriteback>>>) -> CapturedWriteback {
        for _ in 0..100 {
            if let Some(writeback) = slot.lock().expect("writeback lock").clone() {
                return writeback;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
        panic!("runtime command writeback was not received");
    }

    async fn wait_for_event_writebacks(
        slot: Arc<Mutex<Vec<CapturedWriteback>>>,
        count: usize,
    ) -> Vec<CapturedWriteback> {
        for _ in 0..100 {
            let writebacks = slot.lock().expect("events lock").clone();
            if writebacks.len() >= count {
                return writebacks;
            }
            tokio::time::sleep(Duration::from_millis(20)).await;
        }
        panic!("runtime command event writebacks were not received");
    }

    fn header_value(headers: &HeaderMap, key: &str) -> Option<String> {
        headers
            .get(key)
            .and_then(|value| value.to_str().ok())
            .map(ToString::to_string)
    }
}
