use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::process::Command;

use serde_json::Value;
use tempfile::TempDir;

fn make_script(dir: &TempDir, name: &str, body: &str) -> std::path::PathBuf {
    let path = dir.path().join(name);
    fs::write(&path, body).expect("write fake provider script");
    let mut permissions = fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}

#[test]
fn run_command_streams_provider_events_as_json_lines() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"cli-session"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"hello from cli"}]}}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
    );

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("run")
        .arg("--provider")
        .arg("claude")
        .arg("--provider-bin")
        .arg(fake_claude)
        .arg("--workspace")
        .arg(temp.path())
        .arg("--prompt")
        .arg("hello")
        .output()
        .expect("run runtime-agent");

    assert!(
        output.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    let stdout = String::from_utf8(output.stdout).expect("utf8 stdout");
    let events: Vec<Value> = stdout
        .lines()
        .map(|line| serde_json::from_str(line).expect("json line"))
        .collect();

    assert_eq!(events.len(), 3);
    assert_eq!(events[0]["type"], "session_started");
    assert_eq!(events[0]["session_id"], "cli-session");
    assert_eq!(events[1]["type"], "text_delta");
    assert_eq!(events[1]["text"], "hello from cli");
    assert_eq!(events[2]["type"], "turn_completed");
    assert_eq!(events[2]["summary"], "done");
}

#[test]
fn run_command_reports_provider_exit_errors_as_json_lines() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "fake-claude-fails",
        r#"#!/usr/bin/env bash
printf '%s\n' 'provider unavailable' >&2
exit 12
"#,
    );

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("run")
        .arg("--provider")
        .arg("claude")
        .arg("--provider-bin")
        .arg(fake_claude)
        .arg("--workspace")
        .arg(temp.path())
        .arg("--prompt")
        .arg("hello")
        .output()
        .expect("run runtime-agent");

    assert!(!output.status.success());
    let stdout = String::from_utf8(output.stdout).expect("utf8 stdout");
    let events: Vec<Value> = stdout
        .lines()
        .map(|line| serde_json::from_str(line).expect("json line"))
        .collect();

    assert_eq!(events.len(), 1);
    assert_eq!(events[0]["type"], "turn_error");
    assert!(
        events[0]["message"]
            .as_str()
            .expect("error message")
            .contains("claude exited with status 12: provider unavailable")
    );
}
