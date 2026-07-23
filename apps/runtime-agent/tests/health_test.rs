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

#[tokio::test]
async fn probe_provider_health_resolves_stale_absolute_path_via_path() {
    let temp = TempDir::new().expect("tempdir");
    let bin_dir = temp.path().join("bin");
    fs::create_dir_all(&bin_dir).expect("mkdir bin");
    let real = bin_dir.join("stale-claude-probe");
    fs::write(
        &real,
        r#"#!/usr/bin/env bash
printf '%s\n' '9.9.9 (Claude Code)'
"#,
    )
    .expect("write probe binary");
    let mut permissions = fs::metadata(&real).expect("metadata").permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(&real, permissions).expect("chmod");

    let previous_path = std::env::var_os("PATH");
    let mut path_entries = vec![bin_dir];
    if let Some(existing) = previous_path.as_ref() {
        path_entries.extend(std::env::split_paths(existing));
    }
    // SAFETY: single-threaded test; PATH restored below.
    unsafe {
        std::env::set_var(
            "PATH",
            std::env::join_paths(&path_entries).expect("join PATH"),
        );
    }

    let health = probe_provider_health(ProviderHealthProbe {
        kind: "claude".to_string(),
        bin_path: std::path::PathBuf::from("/usr/local/bin/stale-claude-probe"),
    })
    .await;

    match previous_path {
        Some(value) => unsafe { std::env::set_var("PATH", value) },
        None => unsafe { std::env::remove_var("PATH") },
    }

    assert!(health.available, "health={health:?}");
    assert_eq!(health.version.as_deref(), Some("9.9.9 (Claude Code)"));
    assert_eq!(health.error, None);
}
