use std::fs;
use std::os::unix::fs::PermissionsExt;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use axum::extract::{Path as AxumPath, State};
use axum::http::{HeaderMap, StatusCode};
use axum::routing::post;
use axum::{Json, Router};
use serde_json::Value;
use serde_json::json;
use superteam_runtime_agent::commands::executor::RuntimeCommandExecutor;
use superteam_runtime_agent::config::RuntimeConfig;
use superteam_runtime_agent::controlplane::ControlPlaneClient;
use superteam_runtime_agent::controlplane::models::{RuntimeCommand, RuntimeCommandType};
use superteam_runtime_agent::runs::{RunSnapshot, RunStatus, RuntimeRunStore};
use tempfile::TempDir;
use tokio::net::TcpListener;

const DIGITAL_EMPLOYEE_ID: &str = "11111111-1111-4111-8111-111111111111";
const EXECUTION_INSTANCE_ID: &str = "22222222-2222-4222-8222-222222222222";
const TENANT_ID: &str = "00000000-0000-4000-8000-000000000001";
const TEAM_ID: &str = "33333333-3333-4333-8333-333333333333";
const RUNTIME_NODE_ID: &str = "44444444-4444-4444-8444-444444444444";

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

fn configure_runtime_with_control_plane(
    temp: &TempDir,
    claude_bin: PathBuf,
    control_plane: ControlPlaneClient,
) -> RuntimeCommandExecutor {
    let mut config = RuntimeConfig::default();
    config.runs.log_dir = temp.path().join("run-logs");
    config.workspace.base_dir = temp.path().join("workspaces");
    config.providers.claude_code.enabled = true;
    config.providers.claude_code.binary_path = claude_bin;
    config.providers.opencode.enabled = false;
    config.providers.opencode.binary_path = temp.path().join("missing-opencode");
    RuntimeCommandExecutor::with_control_plane_client(config, control_plane)
}

fn employee_home(temp: &TempDir) -> PathBuf {
    temp.path()
        .join("workspaces")
        .join("teams")
        .join(TEAM_ID)
        .join("employees")
        .join(DIGITAL_EMPLOYEE_ID)
}

fn prepare_employee_home(temp: &TempDir) -> PathBuf {
    let home = employee_home(temp);
    fs::create_dir_all(&home).expect("create employee home");
    home
}

fn session_command_in_home(
    agent_home_dir: &Path,
    command_id: &str,
    command_type: RuntimeCommandType,
    mode: &str,
    provider_session_id: Option<&str>,
    prompt: Option<&str>,
    input: Option<&str>,
) -> RuntimeCommand {
    session_command_full(
        command_id,
        command_type,
        mode,
        provider_session_id,
        prompt,
        input,
        agent_home_dir.to_str().expect("agent home dir is utf-8"),
        Vec::new(),
        Vec::new(),
        true,
    )
}

fn session_command_with_refs_in_home(
    agent_home_dir: &Path,
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
        agent_home_dir.to_str().expect("agent home dir is utf-8"),
        context_refs,
        artifact_refs,
        true,
    )
}

fn session_command_with_recoverable_in_home(
    agent_home_dir: &Path,
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
        agent_home_dir.to_str().expect("agent home dir is utf-8"),
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
    agent_home_dir: &str,
    context_refs: Vec<serde_json::Value>,
    artifact_refs: Vec<serde_json::Value>,
    recoverable: bool,
) -> RuntimeCommand {
    RuntimeCommand {
        id: command_id.to_string(),
        command_type,
        payload: json!({
            "command_id": command_id,
            "tenant_id": TENANT_ID,
            "team_id": TEAM_ID,
            "digital_employee_id": DIGITAL_EMPLOYEE_ID,
            "execution_instance_id": EXECUTION_INSTANCE_ID,
            "runtime_node_id": RUNTIME_NODE_ID,
            "provider_type": "claude-code",
            "agent_home_dir": agent_home_dir,
            "workspace_files": [],
            "skills": [],
            "mcp_servers": [],
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

fn workspace_file(content: &str) -> serde_json::Value {
    serde_json::json!({
        "file_id": "55555555-5555-4555-8555-555555555555",
        "revision_id": "66666666-6666-4666-8666-666666666666",
        "path": "AGENTS.md",
        "file_role": "entrypoint",
        "mime_type": "text/markdown",
        "sync_policy": "auto",
        "content_hash": superteam_runtime_agent::workspace_files::sha256_hex(content.as_bytes()),
        "size_bytes": content.len() as i32,
        "storage_backend": "db",
        "content_text": content
    })
}

fn workspace_file_with_hash(content: &str, content_hash: &str) -> serde_json::Value {
    let mut file = workspace_file(content);
    file["content_hash"] = serde_json::Value::String(content_hash.to_string());
    file
}

fn provision_command(
    command_id: &str,
    team_id: &str,
    employee_id: &str,
    agent_home_dir: &str,
    content: &str,
) -> RuntimeCommand {
    RuntimeCommand {
        id: command_id.to_string(),
        command_type: RuntimeCommandType::ProvisionInstance,
        payload: json!({
            "command_id": command_id,
            "tenant_id": "00000000-0000-4000-8000-000000000001",
            "team_id": team_id,
            "digital_employee_id": employee_id,
            "execution_instance_id": EXECUTION_INSTANCE_ID,
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
            "provider_type": "claude-code",
            "agent_home_dir": agent_home_dir,
            "workspace_files": [workspace_file(content)],
            "skills": [],
            "mcp_servers": []
        }),
    }
}

fn start_session_command_with_home(
    command_id: &str,
    team_id: &str,
    employee_id: &str,
    agent_home_dir: &str,
    content: &str,
) -> RuntimeCommand {
    RuntimeCommand {
        id: command_id.to_string(),
        command_type: RuntimeCommandType::StartSession,
        payload: json!({
            "command_id": command_id,
            "tenant_id": "00000000-0000-4000-8000-000000000001",
            "team_id": team_id,
            "digital_employee_id": employee_id,
            "execution_instance_id": EXECUTION_INSTANCE_ID,
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
            "provider_type": "claude-code",
            "agent_home_dir": agent_home_dir,
            "workspace_files": [workspace_file(content)],
            "skills": [],
            "mcp_servers": [],
            "session_policy": {
                "mode": "new",
                "provider_session_id": null,
                "recoverable": true
            },
            "prompt": "write the summary",
            "input": null,
            "context_refs": [],
            "artifact_refs": [],
            "model": null,
            "metadata": {"source": "executor-test"}
        }),
    }
}

fn workspace_materialization_payload(
    command_id: &str,
    agent_home_dir: &Path,
    content: &str,
) -> serde_json::Value {
    json!({
        "command_id": command_id,
        "tenant_id": TENANT_ID,
        "team_id": TEAM_ID,
        "digital_employee_id": DIGITAL_EMPLOYEE_ID,
        "execution_instance_id": EXECUTION_INSTANCE_ID,
        "runtime_node_id": RUNTIME_NODE_ID,
        "provider_type": "claude-code",
        "agent_home_dir": agent_home_dir,
        "workspace_files": [workspace_file(content)],
        "skills": [],
        "mcp_servers": []
    })
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

#[derive(Clone, Default)]
struct CommandFailureCapture {
    fail: Arc<Mutex<Option<CapturedWriteback>>>,
}

#[derive(Clone, Debug)]
struct CapturedWriteback {
    command_id: String,
    authorization: Option<String>,
    node_id: Option<String>,
    payload: Value,
}

struct CommandWritebackServer {
    addr: std::net::SocketAddr,
    task: tokio::task::JoinHandle<()>,
}

async fn serve_command_failures(capture: CommandFailureCapture) -> CommandWritebackServer {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
    let addr = listener.local_addr().expect("local addr");
    let app = Router::new()
        .route(
            "/api/v1/runtime/commands/{command_id}/fail",
            post(capture_fail_writeback),
        )
        .with_state(capture);
    let task = tokio::spawn(async move {
        axum::serve(listener, app).await.expect("serve writebacks");
    });
    CommandWritebackServer { addr, task }
}

async fn serve_failing_command_failures() -> CommandWritebackServer {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
    let addr = listener.local_addr().expect("local addr");
    let app = Router::new().route(
        "/api/v1/runtime/commands/{command_id}/fail",
        post(reject_fail_writeback),
    );
    let task = tokio::spawn(async move {
        axum::serve(listener, app)
            .await
            .expect("serve failing writebacks");
    });
    CommandWritebackServer { addr, task }
}

async fn capture_fail_writeback(
    AxumPath(command_id): AxumPath<String>,
    State(capture): State<CommandFailureCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    *capture.fail.lock().expect("fail lock") = Some(CapturedWriteback {
        command_id,
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::ACCEPTED
}

async fn reject_fail_writeback() -> StatusCode {
    StatusCode::INTERNAL_SERVER_ERROR
}

async fn wait_for_writeback(slot: Arc<Mutex<Option<CapturedWriteback>>>) -> CapturedWriteback {
    for _ in 0..100 {
        if let Some(writeback) = slot.lock().expect("writeback lock").clone() {
            return writeback;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    panic!("runtime command writeback was not received");
}

fn header_value(headers: &HeaderMap, key: &str) -> Option<String> {
    headers
        .get(key)
        .and_then(|value| value.to_str().ok())
        .map(ToString::to_string)
}

#[tokio::test]
async fn provision_instance_materializes_team_employee_home() {
    let temp = tempfile::tempdir().unwrap();
    let mut config = RuntimeConfig::default();
    config.workspace.base_dir = temp.path().join("workspaces");
    let executor = RuntimeCommandExecutor::new(config.clone());

    let team_id = "11111111-1111-4111-8111-111111111111";
    let employee_id = "22222222-2222-4222-8222-222222222222";
    let home = config
        .workspace
        .base_dir
        .join("teams")
        .join(team_id)
        .join("employees")
        .join(employee_id);
    let content = "# Execution Contract\n";
    let command = provision_command(
        "cmd-provision",
        team_id,
        employee_id,
        home.to_str().unwrap(),
        content,
    );

    executor
        .handle_command(command)
        .await
        .expect("provision accepted");

    assert_eq!(
        std::fs::read_to_string(home.join("AGENTS.md")).unwrap(),
        content
    );
    assert!(home.join(".claude").is_dir());
    assert!(home.join("CLAUDE.md").exists());
    assert!(!home.join("state").exists());
}

#[tokio::test]
async fn start_session_uses_agent_home_dir_as_provider_cwd() {
    let temp = tempfile::tempdir().unwrap();
    let cwd_file = temp.path().join("provider-cwd.txt");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-cwd",
        &format!(
            r#"#!/usr/bin/env bash
printf '%s\n' "$PWD" > {}
printf '%s\n' '{{"type":"system","session_id":"session-from-cwd-test"}}'
printf '%s\n' '{{"type":"result","result":"done"}}'
"#,
            shell_quote(&cwd_file)
        ),
    );
    let executor = configure_runtime(&temp, fake_claude);

    let team_id = "11111111-1111-4111-8111-111111111111";
    let employee_id = "22222222-2222-4222-8222-222222222222";
    let home = temp
        .path()
        .join("workspaces")
        .join("teams")
        .join(team_id)
        .join("employees")
        .join(employee_id);
    std::fs::create_dir_all(&home).unwrap();

    let content = "# Execution Contract\n";
    let command = start_session_command_with_home(
        "cmd-start",
        team_id,
        employee_id,
        home.to_str().unwrap(),
        content,
    );
    let outcome = executor
        .handle_command(command)
        .await
        .expect("start_session accepted");

    let run_id = outcome.run_id.as_deref().unwrap();
    wait_for_status(&executor.runs(), run_id, RunStatus::Completed).await;
    let run = executor.runs().get_run(run_id).await.unwrap();
    assert_eq!(run.workspace_path, home);
    assert_eq!(
        std::fs::canonicalize(std::fs::read_to_string(cwd_file).unwrap().trim_end()).unwrap(),
        std::fs::canonicalize(&home).unwrap()
    );
    assert_eq!(
        std::fs::read_to_string(run.workspace_path.join("AGENTS.md")).unwrap(),
        content
    );
}

#[tokio::test]
async fn start_session_workspace_sync_failure_writes_workspace_terminal() {
    let temp = tempfile::tempdir().unwrap();
    let capture = CommandFailureCapture::default();
    let http_server = serve_command_failures(capture.clone()).await;
    let marker_file = temp.path().join("provider-ran.txt");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-should-not-run",
        &format!(
            r#"#!/usr/bin/env bash
printf '%s\n' ran > {}
printf '%s\n' '{{"type":"result","result":"done"}}'
"#,
            shell_quote(&marker_file)
        ),
    );
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        "node-1",
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);

    let team_id = "11111111-1111-4111-8111-111111111111";
    let employee_id = "22222222-2222-4222-8222-222222222222";
    let home = temp
        .path()
        .join("workspaces")
        .join("teams")
        .join(team_id)
        .join("employees")
        .join(employee_id);
    std::fs::create_dir_all(&home).unwrap();

    let content = "# Execution Contract\n";
    let mut command = start_session_command_with_home(
        "cmd-start-bad-workspace",
        team_id,
        employee_id,
        home.to_str().unwrap(),
        content,
    );
    command.payload["workspace_files"] =
        json!([workspace_file_with_hash(content, "not-the-content-hash")]);

    let error = executor
        .handle_command(command)
        .await
        .expect_err("bad workspace file hash should reject before provider start");
    assert!(
        error.to_string().contains("content_hash mismatch"),
        "unexpected error: {error}"
    );
    assert_eq!(
        executor
            .registry()
            .rejection("cmd-start-bad-workspace")
            .as_deref(),
        Some(error.to_string().as_str())
    );

    let failed = wait_for_writeback(capture.fail.clone()).await;
    assert_eq!(failed.command_id, "cmd-start-bad-workspace");
    assert_eq!(
        failed.authorization.as_deref(),
        Some("Bearer session-token")
    );
    assert_eq!(failed.node_id.as_deref(), Some("node-1"));
    assert_eq!(failed.payload["status"], "failed");
    assert_eq!(failed.payload["error_code"], "workspace_sync_failed");
    assert_eq!(failed.payload["error_family"], "workspace_materialization");
    assert!(
        !marker_file.exists(),
        "provider started after workspace failure"
    );

    http_server.task.abort();
}

#[tokio::test]
async fn provision_failure_records_rejection_when_fail_writeback_fails() {
    let temp = tempfile::tempdir().unwrap();
    let http_server = serve_failing_command_failures().await;
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-unused",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"result","result":"unused"}'
"#,
    );
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        "node-1",
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);

    let command_id = "cmd-provision-writeback-fails";
    let error = executor
        .handle_command(RuntimeCommand {
            id: command_id.to_string(),
            command_type: RuntimeCommandType::ProvisionInstance,
            payload: json!({
                "command_id": command_id,
                "tenant_id": TENANT_ID,
                "team_id": TEAM_ID,
                "digital_employee_id": "not-a-uuid",
                "execution_instance_id": EXECUTION_INSTANCE_ID,
                "runtime_node_id": RUNTIME_NODE_ID,
                "provider_type": "claude-code",
                "agent_home_dir": temp.path().join("workspaces").join("teams").join(TEAM_ID).join("employees").join("not-a-uuid"),
                "workspace_files": [],
                "skills": [],
                "mcp_servers": []
            }),
        })
        .await
        .expect_err("invalid provision payload should reject");

    assert!(
        error.to_string().contains("Fail runtime command failed"),
        "unexpected error: {error}"
    );
    let rejection = executor
        .registry()
        .rejection(command_id)
        .expect("original rejection recorded");
    assert!(
        rejection.contains("digital_employee_id must be a UUID-like string"),
        "unexpected rejection: {rejection}"
    );

    http_server.task.abort();
}

#[tokio::test]
async fn sync_workspace_files_materializes_team_employee_home() {
    let temp = TempDir::new().expect("tempdir");
    let mut config = RuntimeConfig::default();
    config.workspace.base_dir = temp.path().join("workspaces");
    let executor = RuntimeCommandExecutor::new(config.clone());
    let home = employee_home(&temp);
    let content = "# Synced Contract\n";

    let outcome = executor
        .handle_command(RuntimeCommand {
            id: "cmd-sync-001".to_string(),
            command_type: RuntimeCommandType::SyncWorkspaceFiles,
            payload: workspace_materialization_payload("cmd-sync-001", &home, content),
        })
        .await
        .expect("sync_workspace_files accepted");

    assert!(outcome.accepted);
    assert_eq!(
        std::fs::read_to_string(home.join("AGENTS.md")).unwrap(),
        content
    );
    assert!(home.join(".claude").is_dir());
    assert!(home.join("CLAUDE.md").exists());
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
    let home = prepare_employee_home(&temp);
    let context_refs = vec![json!({"type": "document", "id": "ctx-1"})];
    let artifact_refs = vec![json!({"type": "report", "id": "artifact-1"})];

    let outcome = executor
        .handle_command(session_command_with_refs_in_home(
            &home,
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

    assert_eq!(snapshot.workspace_path, home);
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
    let home = prepare_employee_home(&temp);

    let outcome = executor
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let first = executor
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let start = executor
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let start = executor
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let start = executor
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let start = executor
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let start = executor
        .handle_command(session_command_with_recoverable_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let first = executor
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
        .handle_command(session_command_in_home(
            &home,
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
    let home = prepare_employee_home(&temp);

    let error = executor
        .handle_command(session_command_in_home(
            &home,
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
