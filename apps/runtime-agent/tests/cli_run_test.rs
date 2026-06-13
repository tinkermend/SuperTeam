use std::fs;
use std::io::Write;
use std::os::unix::fs::PermissionsExt;
use std::process::{Command, Stdio};

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

#[test]
fn cli_run_supports_codex_provider() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let script = make_script(
        &temp,
        "fake-codex",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session","session_id":"cli-codex-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from cli codex"}'
printf '%s\n' '{"type":"turn.completed","summary":"done"}'
"#,
    );

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("run")
        .arg("--provider")
        .arg("codex")
        .arg("--provider-bin")
        .arg(script)
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
    assert_eq!(events[0]["session_id"], "cli-codex-session");
    assert_eq!(events[1]["type"], "text_delta");
    assert_eq!(events[1]["text"], "hello from cli codex");
    assert_eq!(events[2]["type"], "turn_completed");
    assert_eq!(events[2]["summary"], "done");
}

#[test]
fn cli_run_codex_provider_does_not_inherit_parent_stdin() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let script = make_script(
        &temp,
        "fake-codex-stdin",
        r#"#!/usr/bin/env bash
if IFS= read -r -t 1 inherited; then
  printf 'inherited stdin: %s\n' "$inherited" >&2
  exit 42
fi
printf '%s\n' '{"type":"session","session_id":"cli-codex-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from cli codex"}'
printf '%s\n' '{"type":"turn.completed","summary":"done"}'
"#,
    );

    let mut child = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("run")
        .arg("--provider")
        .arg("codex")
        .arg("--provider-bin")
        .arg(script)
        .arg("--workspace")
        .arg(temp.path())
        .arg("--prompt")
        .arg("hello")
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .expect("spawn runtime-agent");

    {
        let mut stdin = child.stdin.take().expect("runtime-agent stdin");
        writeln!(stdin, "contaminated parent stdin").expect("write runtime-agent stdin");
    }

    let output = child.wait_with_output().expect("run runtime-agent");

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
    assert_eq!(events[0]["session_id"], "cli-codex-session");
    assert_eq!(events[1]["type"], "text_delta");
    assert_eq!(events[1]["text"], "hello from cli codex");
    assert_eq!(events[2]["type"], "turn_completed");
    assert_eq!(events[2]["summary"], "done");
}
