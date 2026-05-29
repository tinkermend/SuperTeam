use std::process::Command;
use superteam_runtime_agent::config::RuntimeConfig;
use superteam_runtime_agent::daemon::RuntimeDaemon;

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
fn config_loads_runtime_toml_and_env_overrides() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.toml");
    std::fs::write(
        &config_path,
        r#"
[runtime]
node_id = "file-node"
control_plane_url = "http://control-plane-from-file:8080"
heartbeat_interval = 15
max_concurrent_tasks = 2

[http]
addr = "127.0.0.1:9099"

[runs]
log_dir = "/tmp/file-runtime-runs"

[workspace]
base_dir = "/tmp/file-workspaces"
cleanup_policy = "manual"
max_retained = 4

[providers.claude_code]
enabled = false
binary_path = "/usr/local/bin/file-claude"
timeout = 120

[providers.opencode]
enabled = true
binary_path = "/usr/local/bin/file-opencode"
timeout = 180

[logging]
level = "debug"
format = "json"
output = "file"
file_path = "/tmp/runtime-agent.log"
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
fn cli_loads_config_file_and_allows_explicit_overrides() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let config_path = temp.path().join("runtime-agent.toml");
    std::fs::write(
        &config_path,
        r#"
[runtime]
node_id = "file-cli-node"
"#,
    )
    .expect("write config");

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("--config")
        .arg(&config_path)
        .arg("--node-id")
        .arg("arg-cli-node")
        .arg("--once")
        .env("RUNTIME_AGENT_NODE_ID", "env-cli-node")
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
