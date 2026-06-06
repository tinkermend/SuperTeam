use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use futures::TryStreamExt;
use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
use superteam_runtime_agent::providers::opencode::OpenCodeProvider;
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
        session_id: Some("session-1".to_string()),
        continue_session: false,
        model: None,
    }
}

#[tokio::test]
async fn claude_provider_streams_fake_cli_events() {
    let temp = TempDir::new().expect("tempdir");
    let script = make_script(
        temp.path(),
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"claude-session"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"hello from claude"}]}}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
    );
    let provider = ClaudeProvider::new(script);

    let events: Vec<ProviderEvent> = provider
        .run(request(temp.path()))
        .await
        .expect("run fake claude")
        .try_collect()
        .await
        .expect("collect fake claude events");

    assert_eq!(
        events,
        vec![
            ProviderEvent::SessionStarted {
                session_id: "claude-session".to_string(),
                session_state: None,
            },
            ProviderEvent::TextDelta {
                text: "hello from claude".to_string()
            },
            ProviderEvent::TurnCompleted {
                summary: Some("done".to_string())
            },
        ]
    );
}

#[tokio::test]
async fn opencode_provider_streams_fake_cli_events() {
    let temp = TempDir::new().expect("tempdir");
    let script = make_script(
        temp.path(),
        "fake-opencode",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session.updated","sessionID":"opencode-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from opencode"}'
printf '%s\n' '{"type":"turn.completed"}'
"#,
    );
    let provider = OpenCodeProvider::new(script);

    let events: Vec<ProviderEvent> = provider
        .run(request(temp.path()))
        .await
        .expect("run fake opencode")
        .try_collect()
        .await
        .expect("collect fake opencode events");

    assert_eq!(
        events,
        vec![
            ProviderEvent::SessionStarted {
                session_id: "opencode-session".to_string(),
                session_state: None,
            },
            ProviderEvent::TextDelta {
                text: "hello from opencode".to_string()
            },
            ProviderEvent::TurnCompleted { summary: None },
        ]
    );
}
