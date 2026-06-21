use std::process::Command;
use superteam_runtime_agent::config::{RuntimeConfig, RuntimeConfigOverrides};
use superteam_runtime_agent::controlplane::{ControlPlaneClient, RuntimeCapabilityInput};
use superteam_runtime_agent::daemon::{
    RuntimeDaemon, connect_runtime_session, spawn_session_renewal_loop,
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::{TcpListener, TcpStream},
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
fn config_probe_baseline_defaults_empty() {
    let cfg = RuntimeConfig::new("node-a").expect("config");

    assert!(
        cfg.tools.probe_names.is_empty(),
        "baseline must default to empty; the authoritative probe set is injected by the control plane"
    );
}

#[test]
fn config_loads_optional_tool_probe_baseline_from_file_and_env() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.yaml");
    std::fs::write(
        &config_path,
        r#"
tools:
  probe_names:
    - gh
    - terraform
"#,
    )
    .expect("write config");

    let file_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        std::iter::empty::<(&str, &str)>(),
        Default::default(),
    )
    .expect("load file config");
    assert_eq!(file_config.tools.probe_names, ["gh", "terraform"]);

    let env_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_TOOL_PROBE_NAMES", "gh, ,kubectl,terraform")],
        Default::default(),
    )
    .expect("load env config");
    assert_eq!(env_config.tools.probe_names, ["gh", "kubectl", "terraform"]);
}

#[tokio::test]
async fn build_capabilities_probes_required_tool_set() {
    let cfg = RuntimeConfig::new("node-a").expect("config");
    let caps = superteam_runtime_agent::daemon::build_capabilities(
        &cfg,
        &["definitely-missing-superteam-tool".to_string()],
    )
    .await;

    let tool = caps
        .iter()
        .find(|c| {
            c.capability_type == "tool" && c.capability_key == "definitely-missing-superteam-tool"
        })
        .expect("tool capability probed from required set");
    assert_eq!(tool.provider_type, "tool");
    assert_eq!(tool.binary_path, None);
    assert!(!tool.available);
    assert_eq!(tool.status, "missing");
    assert_eq!(tool.health_status, "missing");
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
  codex:
    enabled: true
    binary_path: /usr/local/bin/file-codex
    timeout: 240

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
            (
                "RUNTIME_AGENT_PROVIDER_CODEX_BINARY",
                "/usr/local/bin/env-codex",
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
    assert!(config.providers.codex.enabled);
    assert_eq!(
        config.providers.codex.binary_path,
        std::path::PathBuf::from("/usr/local/bin/env-codex")
    );
    assert_eq!(config.providers.codex.timeout, 240);
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
fn config_ignores_legacy_auth_token_aliases() {
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

    assert_eq!(file_config.runtime.bootstrap_key, "local-dev-bootstrap-key");

    let env_config = RuntimeConfig::load_with_env(
        Some(&config_path),
        [("RUNTIME_AGENT_AUTH_TOKEN", "env-token")],
        Default::default(),
    )
    .expect("load env config");

    assert_eq!(env_config.runtime.bootstrap_key, "local-dev-bootstrap-key");
}

#[test]
fn cli_rejects_legacy_auth_token_flag() {
    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("--auth-token")
        .arg("legacy-token")
        .arg("--once")
        .output()
        .expect("run runtime-agent");

    assert!(
        !output.status.success(),
        "stdout: {}",
        String::from_utf8_lossy(&output.stdout)
    );
    let stderr = String::from_utf8_lossy(&output.stderr);
    assert!(
        stderr.contains("--auth-token"),
        "stderr should mention rejected flag, got: {stderr}"
    );
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
        let request = read_http_request(&mut socket).await;
        let _ = request_tx.send(request);

        write_json_response(
            &mut socket,
            serde_json::json!({
                "id": "55555555-5555-4555-8555-555555555555",
                "tenant_id": "22222222-2222-4222-8222-222222222222",
                "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                "node_id": "node-1",
                "enrollment_id": "11111111-1111-4111-8111-111111111111",
                "expires_at": "2026-06-03T00:00:00Z",
                "last_seen_at": "2026-06-02T00:00:00Z",
                "created_at": "2026-06-02T00:00:00Z",
                "updated_at": "2026-06-02T00:00:00Z"
            }),
        )
        .await;
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
        let hello_request = read_http_request(&mut hello_socket).await;
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
        let capabilities_request = read_http_request(&mut capabilities_socket).await;
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

#[tokio::test]
async fn runtime_daemon_reports_codex_provider_capability_when_enabled() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, request_rx) = oneshot::channel();

    tokio::spawn(async move {
        let (mut hello_socket, _) = listener.accept().await.unwrap();
        let hello_request = read_http_request(&mut hello_socket).await;
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
        let capabilities_request = read_http_request(&mut capabilities_socket).await;
        write_json_response(&mut capabilities_socket, serde_json::json!([])).await;

        let _ = request_tx.send((hello_request, capabilities_request));
    });

    let temp = tempfile::TempDir::new().expect("tempdir");
    let fake_codex = make_executable_script(
        &temp,
        "fake-codex",
        r#"#!/usr/bin/env bash
if [ "$1" = "--version" ]; then
  printf '%s\n' 'codex-cli 0.137.0'
fi
"#,
    );
    let mut config = RuntimeConfig::new("node-1").expect("valid config");
    config.runtime.control_plane_url = format!("http://{}", addr);
    config.runtime.bootstrap_key = "bootstrap-key".to_string();
    config.providers.claude_code.enabled = false;
    config.providers.opencode.enabled = false;
    config.providers.codex.enabled = true;
    config.providers.codex.binary_path = fake_codex;
    let expected_workspace_base_dir = config.workspace.base_dir.display().to_string();

    let daemon = RuntimeDaemon::new(config);
    let handle = tokio::spawn(async move { daemon.run().await });

    let (hello_request, capabilities_request) =
        tokio::time::timeout(std::time::Duration::from_secs(2), request_rx)
            .await
            .expect("server should capture requests")
            .expect("request pair");
    handle.abort();

    assert!(hello_request.contains(r#""supported_providers":["codex"]"#));
    let (_, capabilities_body) = capabilities_request
        .split_once("\r\n\r\n")
        .expect("capabilities body");
    let capabilities_body: serde_json::Value =
        serde_json::from_str(capabilities_body).expect("capabilities json");
    let capabilities = capabilities_body["capabilities"]
        .as_array()
        .expect("capabilities array");
    let codex = capabilities
        .iter()
        .find(|capability| capability["capability_key"] == "codex")
        .expect("codex capability");
    assert_eq!(codex["provider_type"], serde_json::json!("codex"));
    assert_eq!(codex["available"], serde_json::json!(true));
    assert_eq!(
        codex["provider_version"],
        serde_json::json!("codex-cli 0.137.0")
    );
    assert_eq!(
        codex["workspace_base_dir"],
        serde_json::json!(expected_workspace_base_dir)
    );
}

#[tokio::test]
async fn daemon_connect_runtime_session_does_not_start_renewal_when_capability_upsert_fails() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (renew_tx, renew_rx) = oneshot::channel();

    tokio::spawn(async move {
        let (mut hello_socket, _) = listener.accept().await.unwrap();
        let _hello_request = read_http_request(&mut hello_socket).await;
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
                    "expires_at": "1970-01-01T00:00:00Z",
                    "last_seen_at": "2026-06-02T00:00:00Z",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session_token": "session-token"
            }),
        )
        .await;

        let mut renew_attempted = false;
        loop {
            let (mut socket, _) = listener.accept().await.unwrap();
            let request = read_http_request(&mut socket).await;
            let request_line = request.lines().next().unwrap_or_default();
            if request_line == "PUT /api/v1/runtime/nodes/node-1/capabilities HTTP/1.1" {
                write_status_response(
                    &mut socket,
                    "500 Internal Server Error",
                    serde_json::json!({"error":"capability upsert failed"}),
                )
                .await;
                break;
            }

            if request_line
                == "POST /api/v1/runtime/sessions/55555555-5555-4555-8555-555555555555/renew HTTP/1.1"
            {
                renew_attempted = true;
                write_json_response(
                    &mut socket,
                    serde_json::json!({
                        "id": "55555555-5555-4555-8555-555555555555",
                        "tenant_id": "22222222-2222-4222-8222-222222222222",
                        "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                        "node_id": "node-1",
                        "enrollment_id": "11111111-1111-4111-8111-111111111111",
                        "expires_at": "2999-06-03T00:00:00Z",
                        "last_seen_at": "2026-06-02T00:00:00Z",
                        "created_at": "2026-06-02T00:00:00Z",
                        "updated_at": "2026-06-02T00:00:00Z"
                    }),
                )
                .await;
                continue;
            }

            panic!("unexpected request: {request_line}");
        }

        let late_renew =
            tokio::time::timeout(std::time::Duration::from_millis(200), listener.accept())
                .await
                .is_ok();
        let _ = renew_tx.send(renew_attempted || late_renew);
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

    let result = connect_runtime_session(&config, capabilities).await;

    assert!(result.is_err(), "capability upsert should fail");
    let renew_attempted = renew_rx.await.expect("renew observation");
    assert!(
        !renew_attempted,
        "session renewal must not start before capability upsert succeeds"
    );
}

fn make_executable_script(dir: &tempfile::TempDir, name: &str, body: &str) -> std::path::PathBuf {
    use std::os::unix::fs::PermissionsExt;

    let path = dir.path().join(name);
    std::fs::write(&path, body).expect("write fake provider script");
    let mut permissions = std::fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    std::fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}

async fn read_http_request(socket: &mut TcpStream) -> String {
    let mut buffer = Vec::new();
    let header_end = loop {
        let mut chunk = [0; 1024];
        let bytes_read = socket.read(&mut chunk).await.unwrap();
        assert!(bytes_read > 0, "socket closed before HTTP headers");
        buffer.extend_from_slice(&chunk[..bytes_read]);
        if let Some(index) = find_subsequence(&buffer, b"\r\n\r\n") {
            break index + 4;
        }
    };

    let headers = String::from_utf8_lossy(&buffer[..header_end]);
    let content_length = headers
        .lines()
        .filter_map(|line| line.split_once(':'))
        .find_map(|(name, value)| {
            name.eq_ignore_ascii_case("content-length")
                .then(|| value.trim().parse::<usize>().unwrap())
        })
        .unwrap_or(0);

    while buffer.len() < header_end + content_length {
        let mut chunk = [0; 1024];
        let bytes_read = socket.read(&mut chunk).await.unwrap();
        assert!(bytes_read > 0, "socket closed before HTTP body");
        buffer.extend_from_slice(&chunk[..bytes_read]);
    }

    String::from_utf8(buffer[..header_end + content_length].to_vec()).unwrap()
}

fn find_subsequence(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack
        .windows(needle.len())
        .position(|window| window == needle)
}

async fn write_json_response(socket: &mut TcpStream, body: serde_json::Value) {
    write_status_response(socket, "200 OK", body).await;
}

async fn write_status_response(socket: &mut TcpStream, status: &str, body: serde_json::Value) {
    let body = body.to_string();
    let response = format!(
        "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
        body.len(),
        body
    );
    socket.write_all(response.as_bytes()).await.unwrap();
}
