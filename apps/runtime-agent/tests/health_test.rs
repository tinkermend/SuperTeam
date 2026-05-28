use std::fs;
use std::os::unix::fs::PermissionsExt;

use superteam_runtime_agent::health::{ProviderHealthProbe, probe_provider_health};
use tempfile::TempDir;

fn make_script(dir: &TempDir, name: &str, body: &str) -> std::path::PathBuf {
    let path = dir.path().join(name);
    fs::write(&path, body).expect("write fake provider script");
    let mut permissions = fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}

#[tokio::test]
async fn probe_provider_health_reports_available_version() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '2.1.153 (Claude Code)'
"#,
    );

    let health = probe_provider_health(ProviderHealthProbe {
        kind: "claude".to_string(),
        bin_path: fake_claude,
    })
    .await;

    assert_eq!(health.kind, "claude");
    assert!(health.available);
    assert_eq!(health.version.as_deref(), Some("2.1.153 (Claude Code)"));
    assert_eq!(health.error, None);
}

#[tokio::test]
async fn probe_provider_health_reports_missing_binary() {
    let temp = TempDir::new().expect("tempdir");
    let health = probe_provider_health(ProviderHealthProbe {
        kind: "opencode".to_string(),
        bin_path: temp.path().join("missing-opencode"),
    })
    .await;

    assert_eq!(health.kind, "opencode");
    assert!(!health.available);
    assert_eq!(health.version, None);
    assert!(
        health
            .error
            .as_deref()
            .expect("error")
            .contains("failed to run opencode --version")
    );
}
