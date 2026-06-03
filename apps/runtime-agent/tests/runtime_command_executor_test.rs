use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::time::Duration;

use serde_json::json;
use superteam_runtime_agent::commands::executor::RuntimeCommandExecutor;
use superteam_runtime_agent::config::RuntimeConfig;
use superteam_runtime_agent::controlplane::models::{RuntimeCommand, RuntimeCommandType};
use superteam_runtime_agent::runs::{RunSnapshot, RunStatus, RuntimeRunStore};
use tempfile::TempDir;

const DIGITAL_EMPLOYEE_ID: &str = "11111111-1111-4111-8111-111111111111";
const EXECUTION_INSTANCE_ID: &str = "22222222-2222-4222-8222-222222222222";

fn make_script(dir: &Path, name: &str, body: &str) -> PathBuf {
    let path = dir.join(name);
    fs::write(&path, body).expect("write fake provider script");
    let mut permissions = fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}

fn configure_runtime(temp: &TempDir, claude_bin: PathBuf) -> RuntimeCommandExecutor {
    let mut config = RuntimeConfig::default();
    config.runs.log_dir = temp.path().join("run-logs");
    config.workspace.base_dir = temp.path().join("workspaces");
    config.providers.claude_code.enabled = true;
    config.providers.claude_code.binary_path = claude_bin;
    config.providers.opencode.enabled = false;
    config.providers.opencode.binary_path = temp.path().join("missing-opencode");
    RuntimeCommandExecutor::new(config)
}

fn session_command(
    command_id: &str,
    command_type: RuntimeCommandType,
    mode: &str,
    provider_session_id: Option<&str>,
    prompt: Option<&str>,
    input: Option<&str>,
) -> RuntimeCommand {
    session_command_with_refs(
        command_id,
        command_type,
        mode,
        provider_session_id,
        prompt,
        input,
        Vec::new(),
        Vec::new(),
    )
}

fn session_command_with_refs(
    command_id: &str,
    command_type: RuntimeCommandType,
    mode: &str,
    provider_session_id: Option<&str>,
    prompt: Option<&str>,
    input: Option<&str>,
    context_refs: Vec<serde_json::Value>,
    artifact_refs: Vec<serde_json::Value>,
) -> RuntimeCommand {
    session_command_full(
        command_id,
        command_type,
        mode,
        provider_session_id,
        prompt,
        input,
        context_refs,
        artifact_refs,
        true,
    )
}

fn session_command_with_recoverable(
    command_id: &str,
    command_type: RuntimeCommandType,
    mode: &str,
    provider_session_id: Option<&str>,
    prompt: Option<&str>,
    input: Option<&str>,
    recoverable: bool,
) -> RuntimeCommand {
    session_command_full(
        command_id,
        command_type,
        mode,
        provider_session_id,
        prompt,
        input,
        Vec::new(),
        Vec::new(),
        recoverable,
    )
}

fn session_command_full(
    command_id: &str,
    command_type: RuntimeCommandType,
    mode: &str,
    provider_session_id: Option<&str>,
    prompt: Option<&str>,
    input: Option<&str>,
    context_refs: Vec<serde_json::Value>,
    artifact_refs: Vec<serde_json::Value>,
    recoverable: bool,
) -> RuntimeCommand {
    RuntimeCommand {
        id: command_id.to_string(),
        command_type,
        payload: json!({
            "command_id": command_id,
            "digital_employee_id": DIGITAL_EMPLOYEE_ID,
            "execution_instance_id": EXECUTION_INSTANCE_ID,
            "provider_type": "claude-code",
            "session_policy": {
                "mode": mode,
                "provider_session_id": provider_session_id,
                "recoverable": recoverable
            },
            "prompt": prompt,
            "input": input,
            "context_refs": context_refs,
            "artifact_refs": artifact_refs,
            "model": null,
            "metadata": {"source": "executor-test"}
        }),
    }
}

async fn wait_for_status(runs: &RuntimeRunStore, run_id: &str, expected: RunStatus) -> RunSnapshot {
    for _ in 0..250 {
        if let Some(snapshot) = runs.get_run(run_id).await {
            if snapshot.status == expected {
                return snapshot;
            }
            if matches!(snapshot.status, RunStatus::Failed)
                && !matches!(expected, RunStatus::Failed)
            {
                panic!("run {run_id} failed unexpectedly: {:?}", snapshot.error);
            }
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }

    let snapshot = runs
        .get_run(run_id)
        .await
        .unwrap_or_else(|| panic!("run {run_id} not found"));
    panic!(
        "run {run_id} did not reach {:?}; latest status: {:?}",
        expected, snapshot.status
    );
}

async fn wait_for_latest_provider_session(
    executor: &RuntimeCommandExecutor,
    expected_session_id: &str,
) {
    for _ in 0..250 {
        if executor
            .registry()
            .latest_provider_session(EXECUTION_INSTANCE_ID, "claude-code")
            .as_deref()
            == Some(expected_session_id)
        {
            return;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }

    panic!("latest provider session did not become {expected_session_id}");
}

fn shell_quote(path: &Path) -> String {
    format!("'{}'", path.display().to_string().replace('\'', "'\\''"))
}

fn assert_tokens_in_order(args: &str, first: &str, second: &str) {
    let tokens: Vec<&str> = args.split_whitespace().collect();
    let first_index = tokens
        .iter()
        .position(|token| *token == first)
        .unwrap_or_else(|| panic!("missing token {first} in args: {args}"));
    let second_index = tokens
        .iter()
        .skip(first_index + 1)
        .position(|token| *token == second)
        .map(|relative_index| first_index + 1 + relative_index)
        .unwrap_or_else(|| panic!("missing token {second} after {first} in args: {args}"));
    assert!(
        first_index < second_index,
        "expected {first} before {second} in args: {args}"
    );
}

#[tokio::test]
async fn start_session_runs_provider_and_records_command_context() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-from-command"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"hello from executor"}]}}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);
    let context_refs = vec![json!({"type": "document", "id": "ctx-1"})];
    let artifact_refs = vec![json!({"type": "report", "id": "artifact-1"})];

    let outcome = executor
        .handle_command(session_command_with_refs(
            "cmd-start-001",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("write the summary"),
            None,
            context_refs.clone(),
            artifact_refs.clone(),
        ))
        .await
        .expect("start_session accepted");

    assert!(outcome.accepted);
    let run_id = outcome.run_id.expect("run id");
    let snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    assert_eq!(
        snapshot.workspace_path,
        temp.path()
            .join("workspaces/agents")
            .join(EXECUTION_INSTANCE_ID)
    );
    assert_eq!(
        snapshot.provider_session_id.as_deref(),
        Some("session-from-command")
    );
    let command_context = snapshot.command_context.expect("command context");
    assert_eq!(command_context.command_id, "cmd-start-001");
    assert_eq!(command_context.context_refs, context_refs);
    assert_eq!(command_context.artifact_refs, artifact_refs);
    assert_eq!(
        executor
            .registry()
            .latest_provider_session(EXECUTION_INSTANCE_ID, "claude-code")
            .as_deref(),
        Some("session-from-command")
    );
}

#[tokio::test]
async fn resume_session_sets_continue_session_and_session_id() {
    let temp = TempDir::new().expect("tempdir");
    let args_file = temp.path().join("resume-args.txt");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude",
        &format!(
            r#"#!/usr/bin/env bash
printf '%s\n' "$*" > {}
printf '%s\n' '{{"type":"system","session_id":"existing-session"}}'
printf '%s\n' '{{"type":"result","result":"resumed"}}'
"#,
            shell_quote(&args_file)
        ),
    );
    let executor = configure_runtime(&temp, fake_claude);

    let outcome = executor
        .handle_command(session_command(
            "cmd-resume-001",
            RuntimeCommandType::ResumeSession,
            "resume",
            Some("existing-session"),
            Some("continue the work"),
            None,
        ))
        .await
        .expect("resume_session accepted");
    let run_id = outcome.run_id.expect("run id");

    wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;
    let args = fs::read_to_string(args_file).expect("args file");
    assert_tokens_in_order(&args, "--resume", "existing-session");
}

#[tokio::test]
async fn send_input_reuses_latest_provider_session() {
    let temp = TempDir::new().expect("tempdir");
    let args_log = temp.path().join("args.log");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude",
        &format!(
            r#"#!/usr/bin/env bash
printf '%s\n' "$*" >> {}
printf '%s\n' '{{"type":"system","session_id":"latest-session"}}'
printf '%s\n' '{{"type":"result","result":"done"}}'
"#,
            shell_quote(&args_log)
        ),
    );
    let executor = configure_runtime(&temp, fake_claude);

    let first = executor
        .handle_command(session_command(
            "cmd-start-002",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("start work"),
            None,
        ))
        .await
        .expect("start_session accepted");
    wait_for_status(
        &executor.runs(),
        first.run_id.as_deref().expect("first run id"),
        RunStatus::Completed,
    )
    .await;

    let second = executor
        .handle_command(session_command(
            "cmd-send-001",
            RuntimeCommandType::SendInput,
            "reuse_latest",
            None,
            None,
            Some("append this turn"),
        ))
        .await
        .expect("send_input accepted");
    let second_run_id = second.run_id.expect("second run id");

    let snapshot = wait_for_status(&executor.runs(), &second_run_id, RunStatus::Completed).await;
    assert!(snapshot.continue_session);
    assert_eq!(snapshot.session_id.as_deref(), Some("latest-session"));

    let args = fs::read_to_string(args_log).expect("args log");
    let send_input_args = args.lines().last().expect("send_input args");
    assert_tokens_in_order(send_input_args, "--resume", "latest-session");
}

#[tokio::test]
async fn stop_session_cancels_active_run() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "slow-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"slow-session"}'
sleep 5
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);

    let start = executor
        .handle_command(session_command(
            "cmd-start-slow",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("start slow work"),
            None,
        ))
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");
    wait_for_latest_provider_session(&executor, "slow-session").await;

    let stop = executor
        .handle_command(session_command(
            "cmd-stop-001",
            RuntimeCommandType::StopSession,
            "resume",
            Some("slow-session"),
            Some(""),
            None,
        ))
        .await
        .expect("stop_session accepted");

    assert!(stop.accepted);
    assert_eq!(stop.run_id.as_deref(), Some(started_run_id.as_str()));
    let snapshot = wait_for_status(&executor.runs(), &started_run_id, RunStatus::Cancelled).await;
    assert_eq!(snapshot.status, RunStatus::Cancelled);
}

#[tokio::test(flavor = "current_thread")]
async fn stop_session_immediately_after_start_kills_provider_before_output() {
    let temp = TempDir::new().expect("tempdir");
    let marker_file = temp.path().join("provider-marker.txt");
    let fake_claude = make_script(
        temp.path(),
        "slow-start-claude",
        &format!(
            r#"#!/usr/bin/env bash
sleep 0.25
printf '%s\n' marker > {}
sleep 5
"#,
            shell_quote(&marker_file)
        ),
    );
    let executor = configure_runtime(&temp, fake_claude);

    let start = executor
        .handle_command(session_command(
            "cmd-start-racy",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("start cancellable work"),
            None,
        ))
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");

    let stop = executor
        .handle_command(session_command(
            "cmd-stop-racy",
            RuntimeCommandType::StopSession,
            "new",
            None,
            Some(""),
            None,
        ))
        .await
        .expect("stop_session accepted");

    assert_eq!(stop.run_id.as_deref(), Some(started_run_id.as_str()));
    wait_for_status(&executor.runs(), &started_run_id, RunStatus::Cancelled).await;

    tokio::time::sleep(Duration::from_millis(700)).await;
    assert!(
        !marker_file.exists(),
        "provider kept running after immediate stop_session"
    );
}

#[tokio::test]
async fn stop_session_after_turn_completed_does_not_cancel_completed_run() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "completed-but-open-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"completed-session"}'
printf '%s\n' '{"type":"result","result":"done"}'
sleep 5
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);

    let start = executor
        .handle_command(session_command(
            "cmd-start-completed-open",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("complete then stay open"),
            None,
        ))
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");
    wait_for_status(&executor.runs(), &started_run_id, RunStatus::Completed).await;

    let stop_error = executor
        .handle_command(session_command(
            "cmd-stop-completed-open",
            RuntimeCommandType::StopSession,
            "resume",
            Some("completed-session"),
            Some(""),
            None,
        ))
        .await
        .expect_err("stop_session should not target a completed run");

    assert!(
        stop_error.to_string().contains("no active run found"),
        "unexpected error: {stop_error}"
    );
    assert_eq!(
        executor
            .registry()
            .rejection("cmd-stop-completed-open")
            .as_deref(),
        Some(stop_error.to_string().as_str())
    );

    tokio::time::sleep(Duration::from_millis(100)).await;
    let snapshot = executor
        .runs()
        .get_run(&started_run_id)
        .await
        .expect("completed run snapshot");
    assert_eq!(snapshot.status, RunStatus::Completed);
}

#[tokio::test]
async fn send_input_reuse_latest_ignores_ephemeral_sessions() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "ephemeral-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"ephemeral-session"}'
printf '%s\n' '{"type":"result","result":"ephemeral done"}'
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);

    let start = executor
        .handle_command(session_command(
            "cmd-start-ephemeral",
            RuntimeCommandType::StartSession,
            "ephemeral",
            None,
            Some("temporary provider turn"),
            None,
        ))
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");
    let snapshot = wait_for_status(&executor.runs(), &started_run_id, RunStatus::Completed).await;

    assert_eq!(
        snapshot.provider_session_id.as_deref(),
        Some("ephemeral-session")
    );
    assert_eq!(
        executor
            .registry()
            .latest_provider_session(EXECUTION_INSTANCE_ID, "claude-code"),
        None
    );

    let error = executor
        .handle_command(session_command(
            "cmd-send-after-ephemeral",
            RuntimeCommandType::SendInput,
            "reuse_latest",
            None,
            None,
            Some("try to reuse ephemeral"),
        ))
        .await
        .expect_err("send_input should not reuse an ephemeral provider session");

    assert!(
        error.to_string().contains("provider session"),
        "unexpected error: {error}"
    );
    assert_eq!(
        executor
            .registry()
            .rejection("cmd-send-after-ephemeral")
            .as_deref(),
        Some(error.to_string().as_str())
    );
}

#[tokio::test]
async fn send_input_reuse_latest_ignores_non_recoverable_sessions() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "non-recoverable-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"non-recoverable-session"}'
printf '%s\n' '{"type":"result","result":"non-recoverable done"}'
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);

    let start = executor
        .handle_command(session_command_with_recoverable(
            "cmd-start-non-recoverable",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("non recoverable provider turn"),
            None,
            false,
        ))
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");
    let snapshot = wait_for_status(&executor.runs(), &started_run_id, RunStatus::Completed).await;

    assert_eq!(
        snapshot.provider_session_id.as_deref(),
        Some("non-recoverable-session")
    );
    assert_eq!(
        executor
            .registry()
            .latest_provider_session(EXECUTION_INSTANCE_ID, "claude-code"),
        None
    );

    let error = executor
        .handle_command(session_command(
            "cmd-send-after-non-recoverable",
            RuntimeCommandType::SendInput,
            "reuse_latest",
            None,
            None,
            Some("try to reuse non recoverable"),
        ))
        .await
        .expect_err("send_input should not reuse a non-recoverable provider session");

    assert!(
        error.to_string().contains("provider session"),
        "unexpected error: {error}"
    );
    assert_eq!(
        executor
            .registry()
            .rejection("cmd-send-after-non-recoverable")
            .as_deref(),
        Some(error.to_string().as_str())
    );
}

#[tokio::test]
async fn stop_session_targets_non_reusable_explicit_session_before_session_started() {
    let temp = TempDir::new().expect("tempdir");
    let marker_file = temp.path().join("ephemeral-explicit-marker.txt");
    let fake_claude = make_script(
        temp.path(),
        "multi-session-claude",
        &format!(
            r#"#!/usr/bin/env bash
case "$*" in
  *"ephemeral explicit work"*)
    sleep 0.25
    printf '%s\n' marker > {}
    printf '%s\n' '{{"type":"system","session_id":"ephemeral-explicit-session"}}'
    sleep 5
    ;;
  *"competing latest work"*)
    printf '%s\n' '{{"type":"system","session_id":"late-session"}}'
    sleep 5
    ;;
  *)
    printf '%s\n' '{{"type":"system","session_id":"other-session"}}'
    sleep 5
    ;;
esac
"#,
            shell_quote(&marker_file)
        ),
    );
    let executor = configure_runtime(&temp, fake_claude);

    let first = executor
        .handle_command(session_command(
            "cmd-start-other-active",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("other long work"),
            None,
        ))
        .await
        .expect("start_session accepted");
    let first_run_id = first.run_id.expect("first run id");
    wait_for_latest_provider_session(&executor, "other-session").await;

    let ephemeral = executor
        .handle_command(session_command(
            "cmd-start-ephemeral-explicit",
            RuntimeCommandType::StartSession,
            "ephemeral",
            Some("ephemeral-explicit-session"),
            Some("ephemeral explicit work"),
            None,
        ))
        .await
        .expect("ephemeral start_session accepted");
    let ephemeral_run_id = ephemeral.run_id.expect("ephemeral run id");

    assert_ne!(
        executor
            .registry()
            .latest_provider_session(EXECUTION_INSTANCE_ID, "claude-code")
            .as_deref(),
        Some("ephemeral-explicit-session")
    );

    let late = executor
        .handle_command(session_command(
            "cmd-start-late-active",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("competing latest work"),
            None,
        ))
        .await
        .expect("late start_session accepted");
    let late_run_id = late.run_id.expect("late run id");

    let stop = executor
        .handle_command(session_command(
            "cmd-stop-ephemeral-explicit",
            RuntimeCommandType::StopSession,
            "resume",
            Some("ephemeral-explicit-session"),
            Some(""),
            None,
        ))
        .await
        .expect("stop_session accepted");

    tokio::time::sleep(Duration::from_millis(700)).await;
    let marker_exists = marker_file.exists();
    let first_status = executor
        .runs()
        .get_run(&first_run_id)
        .await
        .expect("first run snapshot")
        .status;
    let late_status = executor
        .runs()
        .get_run(&late_run_id)
        .await
        .expect("late run snapshot")
        .status;

    for run_id in [&first_run_id, &ephemeral_run_id, &late_run_id] {
        if executor
            .runs()
            .get_run(run_id)
            .await
            .is_some_and(|snapshot| snapshot.status == RunStatus::Running)
        {
            let _ = executor
                .runs()
                .cancel_run(run_id, Some("test cleanup".to_string()))
                .await;
        }
    }

    assert_eq!(stop.run_id.as_deref(), Some(ephemeral_run_id.as_str()));
    assert_eq!(first_status, RunStatus::Running);
    assert_eq!(late_status, RunStatus::Running);
    assert!(
        !marker_exists,
        "non-reusable explicit provider session was not stopped before SessionStarted"
    );
}

#[tokio::test]
async fn send_input_without_session_or_reuse_latest_is_rejected() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"result","result":"should not run"}'
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);

    let error = executor
        .handle_command(session_command(
            "cmd-send-rejected",
            RuntimeCommandType::SendInput,
            "new",
            None,
            None,
            Some("append this turn"),
        ))
        .await
        .expect_err("send_input without a provider session should fail");

    assert!(
        error.to_string().contains("provider session"),
        "unexpected error: {error}"
    );
    assert_eq!(
        executor
            .registry()
            .rejection("cmd-send-rejected")
            .as_deref(),
        Some(error.to_string().as_str())
    );
}
