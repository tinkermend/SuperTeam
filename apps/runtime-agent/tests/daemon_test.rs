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
