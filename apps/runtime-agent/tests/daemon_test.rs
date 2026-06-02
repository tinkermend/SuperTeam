use std::process::Command;
use superteam_runtime_agent::config::{RuntimeConfig, RuntimeConfigOverrides};
use superteam_runtime_agent::controlplane::{ControlPlaneClient, RuntimeCapabilityInput};
use superteam_runtime_agent::daemon::{
    RuntimeDaemon, connect_runtime_session, spawn_session_renewal_loop,
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::TcpListener,
    sync::oneshot,
};

#[test]
fn snapshot_uses_configured_node_id() {
    let config = RuntimeConfig::new("node-1").expect("valid config");
    let daemon = RuntimeDaemon::new(config);

    let snapshot = daemon.snapshot();

    assert_eq!(snapshot.node_id, "node-1");
    assert_eq!(snapshot.status, "idle");
}

#[test]
fn config_rejects_blank_node_id() {
    let error = RuntimeConfig::new("  ").expect_err("blank node id must fail");

    assert!(error.to_string().contains("node id is required"));
}

#[test]
fn config_loads_runtime_yaml_and_env_overrides() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.yaml");
    std::fs::write(
        &config_path,
        r#"
runtime:
  node_id: file-node
  control_plane_url: http://control-plane-from-file:8080
  heartbeat_interval: 15
  max_concurrent_tasks: 2

http:
  addr: 127.0.0.1:9099

runs:
  log_dir: /tmp/file-runtime-runs

workspace:
  base_dir: /tmp/file-workspaces
  cleanup_policy: manual
  max_retained: 4

providers:
  claude_code:
    enabled: false
    binary_path: /usr/local/bin/file-claude
    timeout: 120
  opencode:
    enabled: true
    binary_path: /usr/local/bin/file-opencode
    timeout: 180

logging:
  level: debug
  format: json
  output: file
  file_path: /tmp/runtime-agent.log
"#,
    )
    .expect("write config");

    let config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [
            ("RUNTIME_AGENT_NODE_ID", "env-node"),
            ("RUNTIME_AGENT_HTTP_ADDR", "127.0.0.1:9191"),
            (
                "RUNTIME_AGENT_PROVIDER_CLAUDE_CODE_BINARY",
                "/usr/local/bin/env-claude",
            ),
        ],
        Default::default(),
    )
    .expect("load config");

    assert_eq!(config.runtime.node_id, "env-node");
    assert_eq!(
        config.runtime.control_plane_url,
        "http://control-plane-from-file:8080"
    );
    assert_eq!(config.runtime.heartbeat_interval, 15);
    assert_eq!(config.runtime.max_concurrent_tasks, 2);
    assert_eq!(
        config.http.addr,
        "127.0.0.1:9191"
            .parse::<std::net::SocketAddr>()
            .expect("socket addr")
    );
    assert_eq!(
        config.runs.log_dir,
        std::path::PathBuf::from("/tmp/file-runtime-runs")
    );
    assert_eq!(
        config.workspace.base_dir,
        std::path::PathBuf::from("/tmp/file-workspaces")
    );
    assert_eq!(config.workspace.cleanup_policy, "manual");
    assert_eq!(config.workspace.max_retained, 4);
    assert!(!config.providers.claude_code.enabled);
    assert_eq!(
        config.providers.claude_code.binary_path,
        std::path::PathBuf::from("/usr/local/bin/env-claude")
    );
    assert_eq!(config.providers.claude_code.timeout, 120);
    assert!(config.providers.opencode.enabled);
    assert_eq!(
        config.providers.opencode.binary_path,
        std::path::PathBuf::from("/usr/local/bin/file-opencode")
    );
    assert_eq!(config.providers.opencode.timeout, 180);
    assert_eq!(config.logging.level, "debug");
    assert_eq!(config.logging.format, "json");
    assert_eq!(config.logging.output, "file");
    assert_eq!(
        config.logging.file_path,
        Some(std::path::PathBuf::from("/tmp/runtime-agent.log"))
    );
}

#[test]
fn runtime_config_loads_bootstrap_key_from_file_and_env() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.yaml");
    std::fs::write(
        &config_path,
        r#"
runtime:
  bootstrap_key: file-bootstrap-key
"#,
    )
    .expect("write config");

    let file_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        std::iter::empty::<(&str, &str)>(),
        Default::default(),
    )
    .expect("load file config");

    assert_eq!(file_config.runtime.bootstrap_key, "file-bootstrap-key");

    let env_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_BOOTSTRAP_KEY", "env-bootstrap-key")],
        Default::default(),
    )
    .expect("load env config");

    assert_eq!(env_config.runtime.bootstrap_key, "env-bootstrap-key");
}

#[test]
fn config_loads_runtime_bootstrap_key_from_env_and_cli_override() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.yaml");
    std::fs::write(
        &config_path,
        r#"
runtime:
  bootstrap_key: file-bootstrap-key
"#,
    )
    .expect("write config");

    let file_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        std::iter::empty::<(&str, &str)>(),
        Default::default(),
    )
    .expect("load file config");

    assert_eq!(file_config.runtime.bootstrap_key, "file-bootstrap-key");

    let env_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_BOOTSTRAP_KEY", "env-bootstrap-key")],
        Default::default(),
    )
    .expect("load env config");

    assert_eq!(env_config.runtime.bootstrap_key, "env-bootstrap-key");

    let cli_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_BOOTSTRAP_KEY", "env-bootstrap-key")],
        RuntimeConfigOverrides {
            bootstrap_key: Some("cli-bootstrap-key".to_string()),
            ..Default::default()
        },
    )
    .expect("load config");

    assert_eq!(cli_config.runtime.bootstrap_key, "cli-bootstrap-key");
}

#[test]
fn config_accepts_legacy_auth_token_alias_when_bootstrap_key_absent() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.yaml");
    std::fs::write(
        &config_path,
        r#"
runtime:
  auth_token: file-token
"#,
    )
    .expect("write config");

    let file_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        std::iter::empty::<(&str, &str)>(),
        Default::default(),
    )
    .expect("load file config");

    assert_eq!(file_config.runtime.bootstrap_key, "file-token");

    let env_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_AUTH_TOKEN", "env-token")],
        Default::default(),
    )
    .expect("load env config");

    assert_eq!(env_config.runtime.bootstrap_key, "env-token");
}

#[test]
fn cli_loads_config_file_and_allows_explicit_overrides() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.yaml");
    std::fs::write(
        &config_path,
        r#"
runtime:
  node_id: file-cli-node
  bootstrap_key: file-cli-bootstrap-key
"#,
    )
    .expect("write config");

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("--config")
        .arg(&config_path)
        .arg("--node-id")
        .arg("arg-cli-node")
        .arg("--bootstrap-key")
        .arg("arg-cli-bootstrap-key")
        .arg("--once")
        .env("RUNTIME_AGENT_NODE_ID", "env-cli-node")
        .env("RUNTIME_AGENT_BOOTSTRAP_KEY", "env-cli-bootstrap-key")
        .env_remove("RUNTIME_NODE_ID")
        .output()
        .expect("run runtime-agent");

    assert!(
        output.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    let stdout = String::from_utf8(output.stdout).expect("utf8 stdout");
    assert!(stdout.contains("runtime-agent node=arg-cli-node status=idle"));
}

#[tokio::test]
async fn daemon_runtime_session_renewal_loop_attempts_renew_before_expiry() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, request_rx) = oneshot::channel();

    tokio::spawn(async move {
        let (mut socket, _) = listener.accept().await.unwrap();
        let mut buffer = vec![0; 4096];
        let bytes_read = socket.read(&mut buffer).await.unwrap();
        let request = String::from_utf8_lossy(&buffer[..bytes_read]).to_string();
        let _ = request_tx.send(request);

        socket
            .write_all(
                br#"HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 396

{"id":"55555555-5555-4555-8555-555555555555","tenant_id":"22222222-2222-4222-8222-222222222222","runtime_node_id":"33333333-3333-4333-8333-333333333333","node_id":"node-1","enrollment_id":"11111111-1111-4111-8111-111111111111","expires_at":"2026-06-03T00:00:00Z","last_seen_at":"2026-06-02T00:00:00Z","created_at":"2026-06-02T00:00:00Z","updated_at":"2026-06-02T00:00:00Z"}"#,
            )
            .await
            .unwrap();
    });

    let client = ControlPlaneClient::with_session_token(
        format!("http://{}", addr),
        "session-token",
        "node-1",
    );
    let handle = spawn_session_renewal_loop(
        client,
        "55555555-5555-4555-8555-555555555555".to_string(),
        Some("2026-06-01T00:00:00Z".to_string()),
    );

    let request = tokio::time::timeout(std::time::Duration::from_secs(2), request_rx)
        .await
        .expect("renew request should be attempted")
        .expect("renew request should be captured");
    handle.abort();

    let request_line = request.lines().next().unwrap();
    assert_eq!(
        request_line,
        "POST /api/v1/runtime/sessions/55555555-5555-4555-8555-555555555555/renew HTTP/1.1"
    );
    assert!(request.contains("authorization: Bearer session-token"));
    assert!(request.contains("x-node-id: node-1"));
}

#[tokio::test]
async fn daemon_connect_runtime_session_reports_capabilities_after_approved_hello() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, request_rx) = oneshot::channel();

    tokio::spawn(async move {
        let (mut hello_socket, _) = listener.accept().await.unwrap();
        let mut hello_buffer = vec![0; 4096];
        let hello_bytes = hello_socket.read(&mut hello_buffer).await.unwrap();
        let hello_request = String::from_utf8_lossy(&hello_buffer[..hello_bytes]).to_string();
        write_json_response(
            &mut hello_socket,
            serde_json::json!({
                "enrollment": {
                    "id": "11111111-1111-4111-8111-111111111111",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "bootstrap_key_id": "44444444-4444-4444-8444-444444444444",
                    "status": "approved",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session": {
                    "id": "55555555-5555-4555-8555-555555555555",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "enrollment_id": "11111111-1111-4111-8111-111111111111",
                    "expires_at": "2999-06-02T00:00:00Z",
                    "last_seen_at": "2026-06-02T00:00:00Z",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session_token": "session-token"
            }),
        )
        .await;

        let (mut capabilities_socket, _) = listener.accept().await.unwrap();
        let mut capabilities_buffer = vec![0; 4096];
        let capabilities_bytes = capabilities_socket
            .read(&mut capabilities_buffer)
            .await
            .unwrap();
        let capabilities_request =
            String::from_utf8_lossy(&capabilities_buffer[..capabilities_bytes]).to_string();
        write_json_response(&mut capabilities_socket, serde_json::json!([])).await;

        let _ = request_tx.send((hello_request, capabilities_request));
    });

    let mut config = RuntimeConfig::new("node-1").expect("valid config");
    config.runtime.control_plane_url = format!("http://{}", addr);
    config.runtime.bootstrap_key = "bootstrap-key".to_string();
    let capabilities = vec![RuntimeCapabilityInput {
        capability_type: "provider".to_string(),
        capability_key: "claude-code".to_string(),
        provider_type: "claude-code".to_string(),
        provider_version: None,
        binary_path: Some("claude".to_string()),
        available: true,
        workspace_base_dir: None,
        capacity: None,
        labels: None,
        status: "available".to_string(),
        details: None,
        health_status: "configured".to_string(),
        metadata: None,
    }];

    let client = connect_runtime_session(&config, capabilities)
        .await
        .expect("connect runtime session")
        .expect("approved session client");

    drop(client);

    let (hello_request, capabilities_request) =
        tokio::time::timeout(std::time::Duration::from_secs(2), request_rx)
            .await
            .expect("server should capture requests")
            .expect("request pair");
    let hello_line = hello_request.lines().next().unwrap();
    assert_eq!(
        hello_line,
        "POST /api/v1/runtime/enrollments/hello HTTP/1.1"
    );
    assert!(hello_request.contains(r#""capabilities":[{"capability_type":"provider""#));

    let capabilities_line = capabilities_request.lines().next().unwrap();
    assert_eq!(
        capabilities_line,
        "PUT /api/v1/runtime/nodes/node-1/capabilities HTTP/1.1"
    );
    assert!(capabilities_request.contains("authorization: Bearer session-token"));
    assert!(capabilities_request.contains("x-node-id: node-1"));
    let (_, capabilities_body) = capabilities_request
        .split_once("\r\n\r\n")
        .expect("capabilities body");
    let capabilities_body: serde_json::Value =
        serde_json::from_str(capabilities_body).expect("capabilities json");
    assert_eq!(
        capabilities_body["capabilities"][0]["capability_key"],
        serde_json::json!("claude-code")
    );
}

async fn write_json_response(socket: &mut tokio::net::TcpStream, body: serde_json::Value) {
    let body = body.to_string();
    let response = format!(
        "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
        body.len(),
        body
    );
    socket.write_all(response.as_bytes()).await.unwrap();
}
