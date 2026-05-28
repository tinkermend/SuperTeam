use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use futures::StreamExt;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
use superteam_runtime_agent::providers::{ProviderAdapter, ProviderRequest};
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
        workspace_path: workspace_path.to_path_buf(),
        session_id: None,
        continue_session: false,
        model: None,
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
        .run(request(temp.path()))
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
