use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use std::sync::Arc;

use futures::StreamExt;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
use superteam_runtime_agent::providers::{ProviderAdapter, ProviderRequest};
use superteam_runtime_agent::raw_log::NoopRawSink;
use tempfile::TempDir;

fn make_script(dir: &Path, name: &str, body: &str) -> std::path::PathBuf {
    let path = dir.join(name);
    fs::write(&path, body).expect("write fake provider script");
    let mut permissions = fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}

fn request(workspace_path: &Path) -> ProviderRequest {
    ProviderRequest {
        prompt: "hello".to_string(),
        system_prompt: None,
        workspace_path: workspace_path.to_path_buf(),
        agent_home_dir: None,
        employee_capability_dir: None,
        capability_manifest_version: None,
        provider_auth_mode: "host".to_string(),
        mcp_config_path: None,
        session_id: None,
        continue_session: false,
        model: None,
        environment: Default::default(),
    }
}

#[tokio::test]
async fn claude_provider_reports_nonzero_exit_with_stderr() {
    let temp = TempDir::new().expect("tempdir");
    let script = make_script(
        temp.path(),
        "fake-claude-fails",
        r#"#!/usr/bin/env bash
printf '%s\n' 'auth token missing' >&2
exit 7
"#,
    );
    let provider = ClaudeProvider::new(script);

    let mut stream = provider
        .run(request(temp.path()), Arc::new(NoopRawSink))
        .await
        .expect("spawn fake claude");
    let error = stream
        .next()
        .await
        .expect("exit error")
        .expect_err("provider should surface nonzero exit");

    let message = error.to_string();
    assert!(message.contains("claude exited with status 7"));
    assert!(message.contains("auth token missing"));
    assert!(stream.next().await.is_none());
}

/// Parent exits 0 while a grandchild keeps stdout open. The stream must finish
/// via try_wait instead of hanging on next_line EOF.
#[tokio::test]
async fn claude_provider_finishes_when_parent_exits_with_open_stdout() {
    let temp = TempDir::new().expect("tempdir");
    let script = make_script(
        temp.path(),
        "fake-claude-zombie-stdout",
        r#"#!/usr/bin/env python3
import os, sys, time
# Grandchild keeps the stdout write-end open after this PID exits.
if os.fork() == 0:
    time.sleep(30)
    os._exit(0)
os._exit(0)
"#,
    );
    let provider = ClaudeProvider::new(script);

    let mut stream = provider
        .run(request(temp.path()), Arc::new(NoopRawSink))
        .await
        .expect("spawn fake claude");
    let finished = tokio::time::timeout(std::time::Duration::from_secs(6), stream.next())
        .await
        .expect("stream must not hang after parent exit");
    assert!(
        finished.is_none(),
        "successful parent exit with leftover stdout should complete the stream"
    );
}
