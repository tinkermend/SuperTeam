use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;

use futures::TryStreamExt;
use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::providers::claude::ClaudeProvider;
use superteam_runtime_agent::providers::codex::CodexProvider;
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
        system_prompt: None,
        workspace_path: workspace_path.to_path_buf(),
        agent_home_dir: None,
        employee_capability_dir: None,
        capability_manifest_version: None,
        provider_auth_mode: "host".to_string(),
        mcp_config_path: None,
        session_id: Some("session-1".to_string()),
        continue_session: false,
        model: None,
        environment: Default::default(),
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
        .run(request(temp.path()), std::sync::Arc::new(superteam_runtime_agent::raw_log::NoopRawSink))
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
                summary: Some("done".to_string()),
                usage: None,
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
        .run(request(temp.path()), std::sync::Arc::new(superteam_runtime_agent::raw_log::NoopRawSink))
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
            ProviderEvent::TurnCompleted {
                summary: None,
                usage: None
            },
        ]
    );
}

#[tokio::test]
async fn codex_provider_streams_fake_cli_events() {
    let temp = TempDir::new().expect("tempdir");
    let script = make_script(
        temp.path(),
        "fake-codex",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session","session_id":"codex-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from codex"}'
printf '%s\n' '{"type":"turn.completed","summary":"done"}'
"#,
    );
    let provider = CodexProvider::new(script);

    let events: Vec<ProviderEvent> = provider
        .run(request(temp.path()), std::sync::Arc::new(superteam_runtime_agent::raw_log::NoopRawSink))
        .await
        .expect("run fake codex")
        .try_collect()
        .await
        .expect("collect fake codex events");

    assert_eq!(
        events,
        vec![
            ProviderEvent::SessionStarted {
                session_id: "codex-session".to_string(),
                session_state: None,
            },
            ProviderEvent::TextDelta {
                text: "hello from codex".to_string()
            },
            ProviderEvent::TurnCompleted {
                summary: Some("done".to_string()),
                usage: None,
            },
        ]
    );
}

/// Captures what the provider layer hands to the raw transcript.
#[derive(Default)]
struct RecordingSink {
    lines: std::sync::Mutex<Vec<(String, String)>>,
}

#[async_trait::async_trait]
impl superteam_runtime_agent::raw_log::RawLineSink for RecordingSink {
    fn write_line(&self, stream: superteam_runtime_agent::raw_log::RawStream, line: &str) {
        let stream = match stream {
            superteam_runtime_agent::raw_log::RawStream::Stdout => "stdout",
            superteam_runtime_agent::raw_log::RawStream::Stderr => "stderr",
        };
        self.lines
            .lock()
            .unwrap()
            .push((stream.to_string(), line.to_string()));
    }
}

#[tokio::test]
async fn raw_sink_receives_every_stdout_line_including_unparseable_ones() {
    let temp = TempDir::new().expect("tempdir");
    let script = make_script(
        temp.path(),
        "fake-claude-noise",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"s1"}'
printf '%s\n' 'not json at all'
printf '%s\n' 'warning to stderr' >&2
printf '%s\n' '{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","is_error":true,"content":"Exit code 1"}]}}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
    );
    let provider = ClaudeProvider::new(script);
    let sink = std::sync::Arc::new(RecordingSink::default());

    let events: Vec<ProviderEvent> = provider
        .run(request(temp.path()), sink.clone())
        .await
        .expect("run fake claude")
        .try_collect()
        .await
        .expect("unparseable line must not fail the run");

    // The malformed line produced no event but was still captured.
    assert!(events.iter().any(|event| matches!(
        event,
        ProviderEvent::ToolCompleted { is_error: true, .. }
    )));

    let lines = sink.lines.lock().unwrap();
    let stdout: Vec<&str> = lines
        .iter()
        .filter(|(stream, _)| stream == "stdout")
        .map(|(_, line)| line.as_str())
        .collect();
    assert_eq!(stdout.len(), 4, "every stdout line reaches the raw sink");
    assert!(stdout.contains(&"not json at all"));
    assert!(lines.iter().any(|(stream, line)| stream == "stderr" && line == "warning to stderr"));
}
