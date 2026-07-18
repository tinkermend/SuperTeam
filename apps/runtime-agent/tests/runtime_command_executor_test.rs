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
const RUNTIME_NODE_EXTERNAL_ID: &str = "node-1";
const PROJECT_ID: &str = "77777777-7777-4777-8777-777777777777";
const PROJECT_TASK_ID: &str = "55555555-5555-4555-8555-555555555555";
const PROJECT_TASK_ATTEMPT_ID: &str = "66666666-6666-4666-8666-666666666666";
const PROJECT_TASK_LEASE_TOKEN: &str = "lease-token-1";

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

fn with_provider_type(mut command: RuntimeCommand, provider_type: &str) -> RuntimeCommand {
    command.payload["provider_type"] = serde_json::Value::String(provider_type.to_string());
    command
}

fn workspace_file(content: &str) -> serde_json::Value {
    serde_json::json!({
        "file_id": "55555555-5555-4555-8555-555555555555",
        "revision_id": "66666666-6666-4666-8666-666666666666",
        "path": "context.md",
        "file_role": "supporting_doc",
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

fn shell_quote_str(value: &str) -> String {
    format!("'{}'", value.replace('\'', "'\\''"))
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
    cancelled: Arc<Mutex<Option<CapturedWriteback>>>,
    project_task_fail: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
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
        .route(
            "/api/v1/runtime/commands/{command_id}/cancelled",
            post(capture_cancelled_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/started",
            post(accept_project_task_started_for_failures),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/fail",
            post(capture_project_task_fail_writeback),
        )
        .with_state(capture);
    let task = tokio::spawn(async move {
        axum::serve(listener, app).await.expect("serve writebacks");
    });
    CommandWritebackServer { addr, task }
}

#[derive(Clone, Default)]
struct CommandCompletionCapture {
    events: Arc<Mutex<Vec<CapturedWriteback>>>,
    complete: Arc<Mutex<Option<CapturedWriteback>>>,
    fail: Arc<Mutex<Option<CapturedWriteback>>>,
    project_task_started: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
    project_task_complete: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
    project_task_fail: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
    project_task_wait_human: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
    project_task_attestations: Arc<Mutex<Vec<CapturedWriteback>>>,
    project_task_budget_heartbeats: Arc<Mutex<Vec<CapturedProjectTaskBudgetHeartbeat>>>,
    terminal_attestation_release: Arc<tokio::sync::Notify>,
    trip_budget_on_heartbeat: bool,
}

#[derive(Clone, Debug)]
struct CapturedProjectTaskWriteback {
    attempt_id: String,
    project_task_id: String,
    lease_token: String,
    runtime_node_id: String,
    idempotency_key: String,
    authorization: Option<String>,
    node_id: Option<String>,
    payload: Value,
}

#[derive(Clone, Debug)]
struct CapturedProjectTaskBudgetHeartbeat {
    attempt_id: String,
    authorization: Option<String>,
    node_id: Option<String>,
    payload: Value,
}

/// 证据地基:completion 路径现在要求 presign+上传成功;这个桩把
/// presign 指回本服务器的 PUT 端点并一律 200,让既有 writeback 断言不变。
fn with_object_store_stub<S: Clone + Send + Sync + 'static>(
    router: Router<S>,
    addr: std::net::SocketAddr,
) -> Router<S> {
    let artifact_addr = addr;
    let raw_addr = addr;
    router
        .route(
            "/api/v1/runtime/artifacts/presign",
            post(move |Json(body): Json<serde_json::Value>| async move {
                let sha = body["sha256"].as_str().unwrap_or_default().to_string();
                Json(serde_json::json!({
                    "object_key": format!("artifacts/test-tenant/sha256/{sha}"),
                    "upload_url": format!("http://{artifact_addr}/mock-upload/{sha}"),
                    "already_exists": false,
                }))
            }),
        )
        .route(
            "/api/v1/runtime/raw-logs/presign",
            post(move |Json(body): Json<serde_json::Value>| async move {
                let object = body["object"].as_str().unwrap_or_default().to_string();
                Json(serde_json::json!({
                    "object_key": format!("runs/test-tenant/test-attempt/{object}"),
                    "upload_url": format!("http://{raw_addr}/mock-upload/raw-{object}"),
                    "already_exists": false,
                }))
            }),
        )
        .route(
            "/mock-upload/{key}",
            axum::routing::put(|| async { StatusCode::OK }),
        )
}

async fn serve_command_completion_writebacks(
    capture: CommandCompletionCapture,
) -> CommandWritebackServer {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
    let addr = listener.local_addr().expect("local addr");
    let app = Router::new()
        .route(
            "/api/v1/runtime/commands/{command_id}/complete",
            post(capture_complete_writeback),
        )
        .route(
            "/api/v1/runtime/commands/{command_id}/fail",
            post(capture_completion_fail_writeback),
        )
        .route(
            "/api/v1/runtime/commands/{command_id}/events",
            post(capture_event_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/started",
            post(capture_project_task_started_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/complete",
            post(capture_project_task_complete_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/fail",
            post(capture_completion_project_task_fail_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/wait-human",
            post(capture_project_task_wait_human_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attestations",
            post(capture_project_task_attestation_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/budget-heartbeat",
            post(capture_project_task_budget_heartbeat),
        );
    let app = with_object_store_stub(app, addr).with_state(capture);
    let task = tokio::spawn(async move {
        axum::serve(listener, app)
            .await
            .expect("serve completion writebacks");
    });
    CommandWritebackServer { addr, task }
}

fn command_completion_capture_with_budget_trip() -> CommandCompletionCapture {
    CommandCompletionCapture {
        trip_budget_on_heartbeat: true,
        ..CommandCompletionCapture::default()
    }
}

async fn serve_command_cancel_writebacks(
    capture: CommandCompletionCapture,
) -> CommandWritebackServer {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
    let addr = listener.local_addr().expect("local addr");
    let app = Router::new()
        .route(
            "/api/v1/runtime/commands/{command_id}/cancelled",
            post(capture_complete_writeback),
        )
        .with_state(capture);
    let task = tokio::spawn(async move {
        axum::serve(listener, app)
            .await
            .expect("serve cancel writebacks");
    });
    CommandWritebackServer { addr, task }
}

async fn serve_failing_project_task_completion(
    capture: CommandCompletionCapture,
) -> CommandWritebackServer {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
    let addr = listener.local_addr().expect("local addr");
    let app = Router::new()
        .route(
            "/api/v1/runtime/commands/{command_id}/complete",
            post(capture_complete_writeback),
        )
        .route(
            "/api/v1/runtime/commands/{command_id}/events",
            post(capture_event_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/started",
            post(capture_project_task_started_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/complete",
            post(reject_project_task_complete_writeback),
        )
        .with_state(capture);
    let task = tokio::spawn(async move {
        axum::serve(listener, app)
            .await
            .expect("serve failing project task writeback");
    });
    CommandWritebackServer { addr, task }
}

async fn serve_delayed_terminal_attestation_writebacks(
    capture: CommandCompletionCapture,
) -> CommandWritebackServer {
    let listener = TcpListener::bind("127.0.0.1:0").await.expect("listener");
    let addr = listener.local_addr().expect("local addr");
    let app = Router::new()
        .route(
            "/api/v1/runtime/commands/{command_id}/complete",
            post(capture_complete_writeback),
        )
        .route(
            "/api/v1/runtime/commands/{command_id}/events",
            post(capture_event_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/started",
            post(capture_project_task_started_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attempts/{attempt_id}/complete",
            post(capture_project_task_complete_writeback),
        )
        .route(
            "/api/v1/runtime/project-task-attestations",
            post(delayed_terminal_attestation_writeback),
        );
    let app = with_object_store_stub(app, addr).with_state(capture);
    let task = tokio::spawn(async move {
        axum::serve(listener, app)
            .await
            .expect("serve delayed terminal attestation writeback");
    });
    CommandWritebackServer { addr, task }
}

async fn capture_complete_writeback(
    AxumPath(command_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    *capture.complete.lock().expect("complete lock") = Some(CapturedWriteback {
        command_id,
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::ACCEPTED
}

async fn capture_completion_fail_writeback(
    AxumPath(command_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
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

async fn capture_event_writeback(
    AxumPath(command_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    capture
        .events
        .lock()
        .expect("events lock")
        .push(CapturedWriteback {
            command_id,
            authorization: header_value(&headers, "authorization"),
            node_id: header_value(&headers, "x-node-id"),
            payload,
        });
    StatusCode::ACCEPTED
}

async fn capture_project_task_attestation_writeback(
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    capture
        .project_task_attestations
        .lock()
        .expect("attestations lock")
        .push(CapturedWriteback {
            command_id: payload
                .get("metadata")
                .and_then(|metadata| metadata.get("command_id"))
                .and_then(Value::as_str)
                .unwrap_or_default()
                .to_string(),
            authorization: header_value(&headers, "authorization"),
            node_id: header_value(&headers, "x-node-id"),
            payload,
        });
    StatusCode::CREATED
}

async fn delayed_terminal_attestation_writeback(
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    let attestation_type = payload
        .get("attestation_type")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string();
    if attestation_type == "provider_terminal" {
        capture.terminal_attestation_release.notified().await;
    }
    capture_project_task_attestation_writeback(State(capture), headers, Json(payload)).await
}

async fn capture_completion_project_task_fail_writeback(
    AxumPath(attempt_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    let _ = required_string_field(&payload, "failure_family");
    assert!(
        payload.get("retryable").is_some_and(Value::is_boolean),
        "missing boolean retryable in payload: {payload}"
    );
    *capture
        .project_task_fail
        .lock()
        .expect("project task fail lock") = Some(CapturedProjectTaskWriteback {
        attempt_id,
        project_task_id: required_string_field(&payload, "project_task_id"),
        lease_token: required_string_field(&payload, "lease_token"),
        runtime_node_id: required_string_field(&payload, "runtime_node_id"),
        idempotency_key: required_string_field(&payload, "idempotency_key"),
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::ACCEPTED
}

async fn capture_project_task_started_writeback(
    AxumPath(attempt_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    *capture
        .project_task_started
        .lock()
        .expect("project task started lock") = Some(CapturedProjectTaskWriteback {
        attempt_id,
        project_task_id: required_string_field(&payload, "project_task_id"),
        lease_token: required_string_field(&payload, "lease_token"),
        runtime_node_id: required_string_field(&payload, "runtime_node_id"),
        idempotency_key: required_string_field(&payload, "idempotency_key"),
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::ACCEPTED
}

async fn capture_project_task_complete_writeback(
    AxumPath(attempt_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    *capture
        .project_task_complete
        .lock()
        .expect("project task complete lock") = Some(CapturedProjectTaskWriteback {
        attempt_id,
        project_task_id: required_string_field(&payload, "project_task_id"),
        lease_token: required_string_field(&payload, "lease_token"),
        runtime_node_id: required_string_field(&payload, "runtime_node_id"),
        idempotency_key: required_string_field(&payload, "idempotency_key"),
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::ACCEPTED
}

async fn capture_project_task_wait_human_writeback(
    AxumPath(attempt_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    *capture
        .project_task_wait_human
        .lock()
        .expect("project task wait human lock") = Some(CapturedProjectTaskWriteback {
        attempt_id,
        project_task_id: required_string_field(&payload, "project_task_id"),
        lease_token: required_string_field(&payload, "lease_token"),
        runtime_node_id: required_string_field(&payload, "runtime_node_id"),
        idempotency_key: required_string_field(&payload, "idempotency_key"),
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::ACCEPTED
}

async fn capture_project_task_budget_heartbeat(
    AxumPath(attempt_id): AxumPath<String>,
    State(capture): State<CommandCompletionCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> Json<Value> {
    capture
        .project_task_budget_heartbeats
        .lock()
        .expect("budget heartbeats lock")
        .push(CapturedProjectTaskBudgetHeartbeat {
            attempt_id,
            authorization: header_value(&headers, "authorization"),
            node_id: header_value(&headers, "x-node-id"),
            payload,
        });
    Json(json!({
        "tripped": capture.trip_budget_on_heartbeat,
        "trip_reason": if capture.trip_budget_on_heartbeat {
            Some("wall_clock_exceeded")
        } else {
            None
        }
    }))
}

async fn reject_project_task_complete_writeback() -> StatusCode {
    StatusCode::INTERNAL_SERVER_ERROR
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

async fn capture_cancelled_writeback(
    AxumPath(command_id): AxumPath<String>,
    State(capture): State<CommandFailureCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    *capture.cancelled.lock().expect("cancelled lock") = Some(CapturedWriteback {
        command_id,
        authorization: header_value(&headers, "authorization"),
        node_id: header_value(&headers, "x-node-id"),
        payload,
    });
    StatusCode::ACCEPTED
}

async fn accept_project_task_started_for_failures(
    AxumPath(_attempt_id): AxumPath<String>,
    State(_capture): State<CommandFailureCapture>,
    Json(payload): Json<Value>,
) -> StatusCode {
    let _ = required_string_field(&payload, "project_task_id");
    let _ = required_string_field(&payload, "lease_token");
    let _ = required_string_field(&payload, "runtime_node_id");
    let _ = required_string_field(&payload, "idempotency_key");
    StatusCode::ACCEPTED
}

async fn capture_project_task_fail_writeback(
    AxumPath(attempt_id): AxumPath<String>,
    State(capture): State<CommandFailureCapture>,
    headers: HeaderMap,
    Json(payload): Json<Value>,
) -> StatusCode {
    let _ = required_string_field(&payload, "failure_family");
    assert!(
        payload.get("retryable").is_some_and(Value::is_boolean),
        "missing boolean retryable in payload: {payload}"
    );
    *capture
        .project_task_fail
        .lock()
        .expect("project task fail lock") = Some(CapturedProjectTaskWriteback {
        attempt_id,
        project_task_id: required_string_field(&payload, "project_task_id"),
        lease_token: required_string_field(&payload, "lease_token"),
        runtime_node_id: required_string_field(&payload, "runtime_node_id"),
        idempotency_key: required_string_field(&payload, "idempotency_key"),
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

async fn wait_for_project_task_writeback(
    slot: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
) -> CapturedProjectTaskWriteback {
    for _ in 0..100 {
        if let Some(writeback) = slot.lock().expect("project task writeback lock").clone() {
            return writeback;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    panic!("project task complete writeback was not received");
}

async fn assert_no_project_task_writeback(
    slot: Arc<Mutex<Option<CapturedProjectTaskWriteback>>>,
    duration: Duration,
) {
    tokio::time::sleep(duration).await;
    assert!(
        slot.lock().expect("project task writeback lock").is_none(),
        "project task complete writeback was sent before prerequisite writeback completed"
    );
}

async fn assert_no_command_writeback(
    slot: Arc<Mutex<Option<CapturedWriteback>>>,
    duration: Duration,
) {
    tokio::time::sleep(duration).await;
    assert!(
        slot.lock().expect("command writeback lock").is_none(),
        "runtime command writeback was sent before prerequisite writeback completed"
    );
}

async fn wait_for_captured_events(
    slot: Arc<Mutex<Vec<CapturedWriteback>>>,
    expected_count: usize,
) -> Vec<CapturedWriteback> {
    for _ in 0..100 {
        let events = slot.lock().expect("events lock").clone();
        if events.len() >= expected_count {
            return events;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    panic!("runtime command events were not received");
}

async fn wait_for_captured_attestations(
    slot: Arc<Mutex<Vec<CapturedWriteback>>>,
    expected_count: usize,
) -> Vec<CapturedWriteback> {
    for _ in 0..100 {
        let attestations = slot.lock().expect("attestations lock").clone();
        if attestations.len() >= expected_count {
            return attestations;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    panic!("project task attestations were not received");
}

fn header_value(headers: &HeaderMap, key: &str) -> Option<String> {
    headers
        .get(key)
        .and_then(|value| value.to_str().ok())
        .map(ToString::to_string)
}

fn required_string_field(payload: &Value, key: &str) -> String {
    payload
        .get(key)
        .and_then(Value::as_str)
        .unwrap_or_else(|| panic!("missing string field {key} in payload: {payload}"))
        .to_string()
}

fn project_task_attempt_metadata(expected_outputs: Vec<&str>) -> Value {
    json!({
        "source": "project_task_dispatch",
        "project_id": PROJECT_ID,
        "project_task_id": PROJECT_TASK_ID,
        "project_task_attempt_id": PROJECT_TASK_ATTEMPT_ID,
        "project_task_lease_token": PROJECT_TASK_LEASE_TOKEN,
        "runtime_node_id": RUNTIME_NODE_ID,
        "execution_context_packet_version": "v1",
        "expected_outputs": expected_outputs,
        "input_requirements": {},
        "handoff_contract": {"completion_path": "project_task_attempt_writeback"}
    })
}

fn project_task_attempt_metadata_with_budget_trip() -> Value {
    let mut metadata = project_task_attempt_metadata(vec!["execution_summary"]);
    metadata["budget"] = json!({
        "heartbeat_interval_sec": 1,
        "wall_clock_limit_sec": 1
    });
    metadata
}

async fn run_project_task_completion_and_capture_writeback(summary: Option<String>) -> Value {
    let temp = TempDir::new().expect("tempdir");
    let result_line = serde_json::json!({
        "type": "result",
        "result": summary.unwrap_or_else(|| "provider completed".to_string())
    })
    .to_string();
    let script = format!(
        "#!/usr/bin/env bash\nprintf '%s\\n' '{{\"type\":\"system\",\"session_id\":\"session-result-contract\"}}'\nprintf '%s\\n' '{}'\n",
        result_line
    );
    let fake_claude = make_script(temp.path(), "fake-claude-result-contract", &script);
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task-result-contract",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "result_contract",
        "acceptance_results",
        "evidence_refs",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    let captured = wait_for_project_task_writeback(capture.project_task_complete.clone()).await;
    http_server.task.abort();
    captured.payload
}

#[tokio::test]
async fn project_task_completion_writeback_includes_structured_result_contract() {
    let summary = serde_json::json!({
        "result_contract": {
            "status": "completed",
            "summary": "完成分析",
            "acceptance_results": [
                {"criterion": "输出结论", "status": "passed", "evidence_refs": ["artifact:report"]}
            ],
            "evidence_refs": [{"type": "report", "ref": "artifact:report"}],
            "artifact_refs": [],
            "verification": [{"type": "command", "status": "passed", "summary": "命令通过"}],
            "risks": []
        }
    })
    .to_string();

    let captured = run_project_task_completion_and_capture_writeback(Some(summary)).await;

    let contract = captured
        .get("result_contract")
        .expect("result_contract is sent");
    assert_eq!(contract["status"], "completed");
    assert_eq!(contract["summary"], "完成分析");
    assert_eq!(contract["acceptance_results"][0]["criterion"], "输出结论");
}

#[tokio::test]
async fn project_task_completion_writeback_omits_result_contract_for_legacy_summary() {
    let summary = serde_json::json!({
        "conclusion": "legacy complete",
        "evidence_refs": ["artifact:report"],
        "artifact_refs": ["artifact:analysis-report"]
    })
    .to_string();

    let captured = run_project_task_completion_and_capture_writeback(Some(summary)).await;

    assert!(captured.get("result_contract").is_none());
    assert_eq!(captured["conclusion"], "legacy complete");
    assert_eq!(captured["evidence_refs"][0], "artifact:report");
    // 采集的 transcript artifact 前插(证据地基),自报引用保留在其后。
    assert_eq!(captured["artifact_refs"][0]["type"], "execution_transcript");
    assert_eq!(captured["artifact_refs"][0]["is_evidence"], true);
    let self_reported = captured["artifact_refs"]
        .as_array()
        .expect("artifact_refs array")
        .iter()
        .any(|entry| entry == "artifact:analysis-report");
    assert!(self_reported, "self-reported ref must be preserved");
}

#[tokio::test]
async fn start_session_completes_project_task_when_metadata_requests_writeback() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-project-task",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-from-project-task"}'
printf '%s\n' '{"type":"result","result":"provider produced the requested execution summary"}'
"#,
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "execution_summary",
        "evidence_refs",
        "recommended_next_action",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    let snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;
    let command_context = snapshot.command_context.expect("command context");
    assert_eq!(command_context.digital_employee_id, DIGITAL_EMPLOYEE_ID);
    assert_eq!(command_context.provider_type, "claude-code");
    assert_eq!(
        command_context
            .metadata
            .get("project_id")
            .and_then(Value::as_str),
        Some(PROJECT_ID)
    );
    assert_eq!(
        command_context
            .metadata
            .get("project_task_id")
            .and_then(Value::as_str),
        Some(PROJECT_TASK_ID)
    );
    assert_eq!(
        command_context
            .metadata
            .get("project_task_attempt_id")
            .and_then(Value::as_str),
        Some(PROJECT_TASK_ATTEMPT_ID)
    );
    assert_eq!(
        command_context
            .metadata
            .get("runtime_node_id")
            .and_then(Value::as_str),
        Some(RUNTIME_NODE_ID)
    );

    let command_complete = wait_for_writeback(capture.complete.clone()).await;
    assert_eq!(command_complete.command_id, "cmd-project-task");
    assert_eq!(
        command_complete.authorization.as_deref(),
        Some("Bearer session-token")
    );

    let project_started =
        wait_for_project_task_writeback(capture.project_task_started.clone()).await;
    assert_eq!(project_started.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(project_started.project_task_id, PROJECT_TASK_ID);
    assert_eq!(project_started.lease_token, PROJECT_TASK_LEASE_TOKEN);
    assert_eq!(project_started.runtime_node_id, RUNTIME_NODE_ID);
    assert_eq!(
        project_started.idempotency_key,
        format!("project-task-attempt:{PROJECT_TASK_ATTEMPT_ID}:start:cmd-project-task")
    );
    assert_eq!(
        project_started.authorization.as_deref(),
        Some("Bearer session-token")
    );
    assert_eq!(
        project_started.node_id.as_deref(),
        Some(RUNTIME_NODE_EXTERNAL_ID)
    );

    let project_complete =
        wait_for_project_task_writeback(capture.project_task_complete.clone()).await;
    assert_eq!(project_complete.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(project_complete.project_task_id, PROJECT_TASK_ID);
    assert_eq!(project_complete.lease_token, PROJECT_TASK_LEASE_TOKEN);
    assert_eq!(project_complete.runtime_node_id, RUNTIME_NODE_ID);
    assert_eq!(
        project_complete.idempotency_key,
        format!("project-task-attempt:{PROJECT_TASK_ATTEMPT_ID}:complete:cmd-project-task")
    );
    assert_eq!(
        project_complete.authorization.as_deref(),
        Some("Bearer session-token")
    );
    assert_eq!(
        project_complete.node_id.as_deref(),
        Some(RUNTIME_NODE_EXTERNAL_ID)
    );
    assert!(
        project_complete
            .payload
            .get("digital_employee_id")
            .is_none()
    );
    assert_eq!(
        project_complete.payload["conclusion"],
        "provider produced the requested execution summary"
    );
    assert!(
        project_complete
            .payload
            .get("evidence_refs")
            .and_then(Value::as_array)
            .is_some_and(|refs| !refs.is_empty())
    );
    assert!(
        project_complete
            .payload
            .get("recommended_next_action")
            .and_then(Value::as_str)
            .is_some_and(|value| !value.trim().is_empty())
    );
    assert_eq!(
        project_complete.payload["confidence_factors"]["execution_context_packet_version"],
        "v1"
    );

    let attestations =
        wait_for_captured_attestations(capture.project_task_attestations.clone(), 2).await;
    assert!(
        attestations.iter().any(|attestation| {
            attestation.payload["attestation_type"] == "provider_start"
                && attestation.payload["status"] == "succeeded"
                && attestation.payload["project_id"] == PROJECT_ID
                && attestation.payload["project_task_id"] == PROJECT_TASK_ID
                && attestation.payload["attempt_id"] == PROJECT_TASK_ATTEMPT_ID
                && attestation.payload["runtime_node_id"] == RUNTIME_NODE_ID
                && attestation.payload["digital_employee_id"] == DIGITAL_EMPLOYEE_ID
                && attestation.payload["provider_auth_mode"] == "host"
        }),
        "provider_start attestation missing from {attestations:#?}"
    );
    assert!(
        attestations.iter().any(|attestation| {
            attestation.payload["attestation_type"] == "provider_terminal"
                && attestation.payload["status"] == "succeeded"
                && attestation.payload["provider_session_id"] == "session-from-project-task"
        }),
        "provider_terminal attestation missing from {attestations:#?}"
    );

    http_server.task.abort();
}

#[tokio::test]
async fn terminal_writeback_waits_for_terminal_attestation_before_project_completion() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-delayed-terminal-attestation",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-hanging-attestation"}'
printf '%s\n' '{"type":"result","result":"provider completed before attestation returned"}'
"#,
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_delayed_terminal_attestation_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task-hanging-attestation",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "execution_summary",
        "evidence_refs",
        "recommended_next_action",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    assert_no_project_task_writeback(
        capture.project_task_complete.clone(),
        Duration::from_millis(200),
    )
    .await;
    assert_no_command_writeback(capture.complete.clone(), Duration::from_millis(20)).await;
    capture.terminal_attestation_release.notify_waiters();
    let attestations =
        wait_for_captured_attestations(capture.project_task_attestations.clone(), 2).await;
    assert!(
        attestations
            .iter()
            .any(|attestation| attestation.payload["attestation_type"] == "provider_terminal"),
        "provider_terminal attestation missing from {attestations:#?}"
    );
    let command_complete = wait_for_writeback(capture.complete.clone()).await;
    assert_eq!(
        command_complete.command_id,
        "cmd-project-task-hanging-attestation"
    );
    let project_complete =
        wait_for_project_task_writeback(capture.project_task_complete.clone()).await;
    assert_eq!(project_complete.attempt_id, PROJECT_TASK_ATTEMPT_ID);

    http_server.task.abort();
}

#[tokio::test]
async fn zero_event_provider_exit_fails_run_instead_of_hanging() {
    // 残债交接 §1:provider exit 0 且零可解析输出(格式漂移被解析层全量丢弃、
    // 包装脚本吞输出等)曾使 run 永滞 dispatching 且无任何回写。流结束而无
    // 终局事件必须按失败收尾。
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-zero-events",
        "#!/usr/bin/env bash\nexit 0\n",
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let command = session_command_in_home(
        &home,
        "cmd-zero-events",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("run something"),
        None,
    );

    let outcome = executor
        .handle_command(command)
        .await
        .expect("command accepted");
    let run_id = outcome.run_id.expect("run id");
    let snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Failed).await;
    assert!(
        snapshot
            .error
            .as_deref()
            .is_some_and(|error| error.contains("without a terminal event")),
        "expected zero-event early-exit diagnosis, got {:?}",
        snapshot.error
    );
    let command_fail = wait_for_writeback(capture.fail.clone()).await;
    assert_eq!(command_fail.command_id, "cmd-zero-events");

    http_server.task.abort();
}

#[tokio::test]
async fn provider_failure_after_result_fails_project_task_without_prior_completion() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-project-task-provider-failure",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-provider-failure"}'
printf '%s\n' '{"type":"result","result":"API Error: 529 provider overloaded"}'
exit 1
"#,
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task-provider-failure",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "execution_summary",
        "evidence_refs",
        "recommended_next_action",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    let snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Failed).await;
    assert!(
        snapshot
            .error
            .as_deref()
            .is_some_and(|error| error.contains("claude exited with status 1")),
        "expected provider exit failure, got {:?}",
        snapshot.error
    );

    let command_fail = wait_for_writeback(capture.fail.clone()).await;
    assert_eq!(command_fail.command_id, "cmd-project-task-provider-failure");
    assert_eq!(command_fail.payload["status"], "failed");

    let project_fail = wait_for_project_task_writeback(capture.project_task_fail.clone()).await;
    assert_eq!(project_fail.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(project_fail.project_task_id, PROJECT_TASK_ID);
    assert_eq!(project_fail.lease_token, PROJECT_TASK_LEASE_TOKEN);
    assert_eq!(project_fail.runtime_node_id, RUNTIME_NODE_ID);
    assert_eq!(
        project_fail.idempotency_key,
        format!(
            "project-task-attempt:{PROJECT_TASK_ATTEMPT_ID}:fail:cmd-project-task-provider-failure"
        )
    );
    assert!(project_fail.payload.get("digital_employee_id").is_none());
    assert_eq!(
        project_fail.payload["failure_summary"],
        "claude exited with status 1"
    );
    assert_eq!(project_fail.payload["failure_family"], "transient_provider");
    assert_eq!(project_fail.payload["retryable"], true);

    tokio::time::sleep(Duration::from_millis(50)).await;
    assert!(
        capture.complete.lock().expect("complete lock").is_none(),
        "provider failure must not complete the runtime command first"
    );
    assert!(
        capture
            .project_task_complete
            .lock()
            .expect("project task complete lock")
            .is_none(),
        "provider failure must not complete the project task first"
    );

    http_server.task.abort();
}

#[tokio::test]
async fn start_session_does_not_complete_project_task_without_dispatch_source() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-non-project-task",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-from-ordinary-command"}'
printf '%s\n' '{"type":"result","result":"ordinary command completed"}'
"#,
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-ordinary",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete an ordinary command"),
        None,
    );
    command.payload["metadata"] = json!({
        "source": "manual",
        "project_task_id": PROJECT_TASK_ID,
        "project_task_attempt_id": PROJECT_TASK_ATTEMPT_ID,
        "project_task_lease_token": PROJECT_TASK_LEASE_TOKEN,
        "runtime_node_id": RUNTIME_NODE_ID,
        "handoff_contract": {"completion_path": "project_task_attempt_writeback"}
    });

    let outcome = executor
        .handle_command(command)
        .await
        .expect("ordinary command accepted");
    let run_id = outcome.run_id.expect("run id");
    wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    let command_complete = wait_for_writeback(capture.complete.clone()).await;
    assert_eq!(command_complete.command_id, "cmd-ordinary");
    tokio::time::sleep(Duration::from_millis(50)).await;
    assert!(
        capture
            .project_task_complete
            .lock()
            .expect("project task complete lock")
            .is_none(),
        "ordinary command metadata must not trigger project task writeback"
    );

    http_server.task.abort();
}

#[tokio::test]
async fn start_session_keeps_run_completed_when_project_task_writeback_fails() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-project-task-writeback-fails",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-project-task-writeback-fails"}'
printf '%s\n' '{"type":"result","result":"provider finished before project task writeback failed"}'
"#,
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_failing_project_task_completion(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task-writeback-fails",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "execution_summary",
        "evidence_refs",
        "recommended_next_action",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    let snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    assert_eq!(snapshot.error, None);
    let command_complete = wait_for_writeback(capture.complete.clone()).await;
    assert_eq!(
        command_complete.command_id,
        "cmd-project-task-writeback-fails"
    );

    http_server.task.abort();
}

#[tokio::test]
async fn start_session_preserves_structured_project_task_writeback_fields() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-project-task-structured",
        r#"#!/usr/bin/env bash
cat <<'EOF'
{"type":"system","session_id":"session-from-structured-project-task"}
{"type":"result","result":"{\"execution_summary\":{\"summary\":\"structured task complete\"},\"evidence_refs\":[{\"type\":\"artifact\",\"ref\":\"artifact://evidence-one\"}],\"artifact_refs\":[{\"type\":\"file\",\"ref\":\"file://artifact-one\"}],\"confidence_factors\":{\"provider_confidence\":\"high\",\"score\":0.82},\"uncertainty\":\"low\",\"missing_information\":[{\"field\":\"none\"}],\"recommended_next_action\":\"ready for review\",\"requires_human_review\":true}"}
EOF
"#,
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task-structured",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "execution_summary",
        "evidence_refs",
        "recommended_next_action",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    let project_complete =
        wait_for_project_task_writeback(capture.project_task_complete.clone()).await;
    assert_eq!(
        project_complete.payload["conclusion"],
        "structured task complete"
    );
    assert_eq!(
        project_complete.payload["evidence_refs"][0]["ref"],
        "artifact://evidence-one"
    );
    // 采集的 transcript artifact 前插;自报的 file:// 引用保留在其后。
    assert_eq!(
        project_complete.payload["artifact_refs"][0]["type"],
        "execution_transcript"
    );
    let self_reported = project_complete.payload["artifact_refs"]
        .as_array()
        .expect("artifact_refs array")
        .iter()
        .any(|entry| entry["ref"] == "file://artifact-one");
    assert!(self_reported, "self-reported artifact ref must be preserved");
    assert_eq!(
        project_complete.payload["confidence_factors"]["provider_confidence"],
        "high"
    );
    assert_eq!(
        project_complete.payload["confidence_factors"]["score"],
        0.82
    );
    assert_eq!(project_complete.payload["uncertainty"], "low");
    assert_eq!(
        project_complete.payload["missing_information"][0]["field"],
        "none"
    );
    assert_eq!(
        project_complete.payload["recommended_next_action"],
        "ready for review"
    );
    assert_eq!(project_complete.payload["requires_human_review"], true);

    http_server.task.abort();
}

#[tokio::test]
async fn start_session_wait_human_when_provider_reports_missing_context() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-project-task-wait-human",
        r#"#!/usr/bin/env bash
cat <<'EOF'
{"type":"system","session_id":"session-from-wait-human-project-task"}
{"type":"result","result":"{\"requires_human_review\":true,\"wait_human_reason\":\"missing_context\",\"missing_context_refs\":[\"customer_scope\"],\"recommended_next_action\":\"Ask the human owner for the missing customer scope.\"}"}
EOF
"#,
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task-wait-human",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "execution_summary",
        "evidence_refs",
        "recommended_next_action",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    let wait = wait_for_project_task_writeback(capture.project_task_wait_human.clone()).await;
    assert_eq!(wait.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(wait.project_task_id, PROJECT_TASK_ID);
    assert_eq!(wait.lease_token, PROJECT_TASK_LEASE_TOKEN);
    assert_eq!(wait.runtime_node_id, RUNTIME_NODE_ID);
    assert_eq!(
        wait.idempotency_key,
        format!(
            "project-task-attempt:{PROJECT_TASK_ATTEMPT_ID}:wait-human:cmd-project-task-wait-human"
        )
    );
    assert_eq!(wait.payload["digital_employee_id"], DIGITAL_EMPLOYEE_ID);
    assert_eq!(wait.payload["reason"], "missing_context");
    assert_eq!(
        wait.payload["summary"],
        "Ask the human owner for the missing customer scope."
    );
    assert_eq!(wait.payload["missing_context_refs"][0], "customer_scope");
    assert!(
        capture
            .project_task_complete
            .lock()
            .expect("complete lock")
            .is_none()
    );

    http_server.task.abort();
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
        std::fs::read_to_string(home.join("context.md")).unwrap(),
        content
    );
    assert!(home.join(".claude").is_dir());
    assert!(!home.join("state").exists());
}

#[tokio::test]
async fn provision_instance_projects_persona_memory_into_employee_home() {
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
    let mut command = provision_command(
        "cmd-provision-persona",
        team_id,
        employee_id,
        home.to_str().unwrap(),
        "# Execution Contract\n",
    );
    command.payload["persona_memory_markdown"] = json!("# 人格画像\n证据优先");

    executor
        .handle_command(command)
        .await
        .expect("provision accepted");

    assert_eq!(
        std::fs::read_to_string(home.join("人格记忆.md")).unwrap(),
        "# 人格画像\n证据优先"
    );
}

#[tokio::test]
async fn provision_instance_preserves_persona_memory_content_verbatim() {
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
    let persona_memory_markdown = " \n\n# 人格画像\n证据优先\n\n ";
    let mut command = provision_command(
        "cmd-provision-persona-verbatim",
        team_id,
        employee_id,
        home.to_str().unwrap(),
        "# Execution Contract\n",
    );
    command.payload["persona_memory_markdown"] = json!(persona_memory_markdown);

    executor
        .handle_command(command)
        .await
        .expect("provision accepted");

    assert_eq!(
        std::fs::read_to_string(home.join("人格记忆.md")).unwrap(),
        persona_memory_markdown
    );
}

#[tokio::test]
async fn start_session_rejects_disabled_codex_provider() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"unused"}'
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);
    let home = prepare_employee_home(&temp);
    let command = with_provider_type(
        session_command_in_home(
            &home,
            "cmd-codex-disabled",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("hello"),
            None,
        ),
        "codex",
    );

    let error = executor
        .handle_command(command)
        .await
        .expect_err("disabled codex should fail");

    assert!(error.to_string().contains("Codex provider is disabled"));
}

#[tokio::test]
async fn start_session_runs_codex_provider() {
    let temp = TempDir::new().expect("tempdir");
    let fake_codex = make_script(
        temp.path(),
        "fake-codex",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session","session_id":"codex-runtime-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from codex runtime"}'
printf '%s\n' '{"type":"turn.completed","summary":"done"}'
"#,
    );
    let mut config = RuntimeConfig::default();
    config.runs.log_dir = temp.path().join("run-logs");
    config.workspace.base_dir = temp.path().join("workspaces");
    config.providers.claude_code.enabled = false;
    config.providers.claude_code.binary_path = temp.path().join("missing-claude");
    config.providers.opencode.enabled = false;
    config.providers.opencode.binary_path = temp.path().join("missing-opencode");
    config.providers.codex.enabled = true;
    config.providers.codex.binary_path = fake_codex;
    let executor = RuntimeCommandExecutor::new(config);
    let home = prepare_employee_home(&temp);
    let command = with_provider_type(
        session_command_in_home(
            &home,
            "cmd-codex-start",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("hello"),
            None,
        ),
        "codex",
    );

    let outcome = executor
        .handle_command(command)
        .await
        .expect("codex command accepted");
    let run_id = outcome.run_id.expect("run id");
    let final_snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    assert_eq!(
        final_snapshot.provider_session_id.as_deref(),
        Some("codex-runtime-session")
    );
}

#[tokio::test]
async fn codex_project_task_completion_uses_item_text_when_turn_summary_is_empty() {
    let temp = TempDir::new().expect("tempdir");
    let task_result = json!({
        "conclusion": "Codex produced a structured task result.",
        "result_contract": {
            "status": "completed",
            "summary": "Codex produced a structured task result.",
            "acceptance_results": [
                {
                    "criteria": "execution_summary",
                    "status": "passed",
                    "summary": "Provider asserted the execution summary.",
                    "verification": "provider-local note that should not be sent inside acceptance_results",
                    "evidence_refs": [
                        {
                            "type": "log",
                            "ref": "runtime-command://cmd-codex-project-task"
                        }
                    ]
                }
            ],
            "evidence_refs": [
                {
                    "type": "log",
                    "ref": "runtime-command://cmd-codex-project-task"
                }
            ],
            "artifact_refs": [],
            "verification": [
                {
                    "type": "provider_output",
                    "status": "passed",
                    "summary": "Fake Codex item.completed output was parsed."
                }
            ],
            "risks": []
        },
        "evidence_refs": [
            {
                "type": "log",
                "ref": "runtime-command://cmd-codex-project-task"
            }
        ],
        "artifact_refs": [],
        "recommended_next_action": "Continue project coordination with the next ready task."
    })
    .to_string();
    let session_line = json!({
        "type": "thread.started",
        "thread": {"id": "codex-project-session"}
    })
    .to_string();
    let item_line = json!({
        "type": "item.completed",
        "item": {
            "id": "item_0",
            "type": "agent_message",
            "text": task_result
        }
    })
    .to_string();
    let completion_line = json!({"type": "turn.completed"}).to_string();
    let fake_codex = make_script(
        temp.path(),
        "fake-codex-project-task",
        &format!(
            "#!/usr/bin/env bash\nprintf '%s\\n' {}\nprintf '%s\\n' {}\nprintf '%s\\n' {}\n",
            shell_quote_str(&session_line),
            shell_quote_str(&item_line),
            shell_quote_str(&completion_line)
        ),
    );
    let capture = CommandCompletionCapture::default();
    let http_server = serve_command_completion_writebacks(capture.clone()).await;
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", http_server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let mut config = RuntimeConfig::default();
    config.runs.log_dir = temp.path().join("run-logs");
    config.workspace.base_dir = temp.path().join("workspaces");
    config.providers.claude_code.enabled = false;
    config.providers.claude_code.binary_path = temp.path().join("missing-claude");
    config.providers.opencode.enabled = false;
    config.providers.opencode.binary_path = temp.path().join("missing-opencode");
    config.providers.codex.enabled = true;
    config.providers.codex.binary_path = fake_codex;
    let executor = RuntimeCommandExecutor::with_control_plane_client(config, control_plane);
    let home = prepare_employee_home(&temp);
    let mut command = with_provider_type(
        session_command_in_home(
            &home,
            "cmd-codex-project-task",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("complete the project task"),
            None,
        ),
        "codex",
    );
    command.payload["metadata"] = project_task_attempt_metadata(vec![
        "result_contract",
        "acceptance_results",
        "evidence_refs",
    ]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("codex project task command accepted");
    let run_id = outcome.run_id.expect("run id");
    wait_for_status(&executor.runs(), &run_id, RunStatus::Completed).await;

    let events = wait_for_captured_events(capture.events.clone(), 3).await;
    assert_eq!(events[0].payload["event_type"], "session_started");
    assert_eq!(events[1].payload["event_type"], "text_delta");
    assert!(
        events[1].payload["payload"]["text"]
            .as_str()
            .is_some_and(|text| text.contains("Codex produced a structured task result.")),
        "text_delta event should carry the Codex item.completed text: {}",
        events[1].payload
    );
    assert_eq!(events[2].payload["event_type"], "turn_completed");
    assert!(
        events[2].payload["payload"]
            .as_object()
            .is_some_and(|payload| payload.is_empty()),
        "turn_completed event should have an empty payload: {}",
        events[2].payload
    );

    let command_complete = wait_for_writeback(capture.complete.clone()).await;
    assert_eq!(command_complete.command_id, "cmd-codex-project-task");
    assert!(
        command_complete
            .payload
            .get("summary")
            .and_then(Value::as_str)
            .is_some_and(|summary| summary.contains("Codex produced a structured task result.")),
        "command completion should use Codex item.completed text as summary: {}",
        command_complete.payload
    );

    let project_complete =
        wait_for_project_task_writeback(capture.project_task_complete.clone()).await;
    assert_eq!(
        project_complete.payload["conclusion"],
        "Codex produced a structured task result."
    );
    assert_eq!(
        project_complete.payload["result_contract"]["summary"],
        "Codex produced a structured task result."
    );
    assert_eq!(
        project_complete.payload["result_contract"]["acceptance_results"][0]["evidence_refs"][0],
        "runtime-command://cmd-codex-project-task"
    );
    assert_eq!(
        project_complete.payload["result_contract"]["acceptance_results"][0]["criterion"],
        "execution_summary"
    );
    assert!(
        project_complete.payload["result_contract"]["acceptance_results"][0]
            .get("criteria")
            .is_none(),
        "criteria alias must be normalized before Control Plane decode: {}",
        project_complete.payload
    );
    assert!(
        project_complete.payload["result_contract"]["acceptance_results"][0]
            .get("verification")
            .is_none(),
        "acceptance result unknown fields must not reach Control Plane: {}",
        project_complete.payload
    );
    assert!(
        project_complete.payload.get("verification").is_none(),
        "verification must stay nested under result_contract: {}",
        project_complete.payload
    );
    assert_eq!(
        project_complete.payload["result_contract"]["verification"][0]["status"],
        "passed"
    );

    http_server.task.abort();
}

#[tokio::test]
async fn start_session_uses_project_workspace_as_provider_cwd_and_agent_home_for_files() {
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
    let mut command = start_session_command_with_home(
        "cmd-start",
        team_id,
        employee_id,
        home.to_str().unwrap(),
        content,
    );
    command.payload["metadata"] = json!({
        "source": "executor-test",
        "workspace_mode": "none",
        "project_id": "77777777-7777-4777-8777-777777777777",
        "project_task_id": "88888888-8888-4888-8888-888888888888",
        "project_task_attempt_id": "99999999-9999-4999-8999-999999999999"
    });
    let outcome = executor
        .handle_command(command)
        .await
        .expect("start_session accepted");

    let run_id = outcome.run_id.as_deref().unwrap();
    wait_for_status(&executor.runs(), run_id, RunStatus::Completed).await;
    let run = executor.runs().get_run(run_id).await.unwrap();
    let expected_workspace = temp
        .path()
        .join("workspaces")
        .join("workspaces")
        .join("77777777-7777-4777-8777-777777777777")
        .join("88888888-8888-4888-8888-888888888888")
        .join("99999999-9999-4999-8999-999999999999");
    assert_eq!(run.workspace_path, expected_workspace);
    assert_eq!(run.agent_home_dir.as_deref(), Some(home.as_path()));
    assert_eq!(
        std::fs::canonicalize(std::fs::read_to_string(cwd_file).unwrap().trim_end()).unwrap(),
        std::fs::canonicalize(&expected_workspace).unwrap()
    );
    assert_eq!(
        std::fs::read_to_string(home.join("context.md")).unwrap(),
        content
    );
    assert!(!run.workspace_path.join("context.md").exists());
}

#[tokio::test]
async fn start_session_creates_missing_agent_home_for_project_task_dispatch() {
    let temp = tempfile::tempdir().unwrap();
    let cwd_file = temp.path().join("provider-cwd-missing-home.txt");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-missing-home",
        &format!(
            r#"#!/usr/bin/env bash
printf '%s\n' "$PWD" > {}
printf '%s\n' '{{"type":"system","session_id":"session-from-missing-home-test"}}'
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
        .join("project-tasks")
        .join(PROJECT_ID)
        .join(PROJECT_TASK_ID)
        .join(PROJECT_TASK_ATTEMPT_ID)
        .join("employees")
        .join(employee_id);
    assert!(!home.exists(), "test requires missing derived agent home");

    let content = "# Execution Contract\n";
    let mut command = start_session_command_with_home(
        "cmd-start-missing-home",
        team_id,
        employee_id,
        home.to_str().unwrap(),
        content,
    );
    command.payload["metadata"] = json!({
        "source": "project_task_dispatch",
        "workspace_mode": "none",
        "project_id": PROJECT_ID,
        "project_task_id": PROJECT_TASK_ID,
        "project_task_attempt_id": PROJECT_TASK_ATTEMPT_ID
    });

    let outcome = executor
        .handle_command(command)
        .await
        .expect("start_session should create missing derived agent home");

    let run_id = outcome.run_id.as_deref().unwrap();
    wait_for_status(&executor.runs(), run_id, RunStatus::Completed).await;
    let run = executor.runs().get_run(run_id).await.unwrap();
    assert_eq!(run.agent_home_dir.as_deref(), Some(home.as_path()));
    assert!(home.is_dir(), "derived agent home should be created");
    assert_eq!(
        std::fs::read_to_string(home.join("context.md")).unwrap(),
        content
    );
    assert_eq!(
        std::fs::canonicalize(std::fs::read_to_string(cwd_file).unwrap().trim_end()).unwrap(),
        std::fs::canonicalize(run.workspace_path).unwrap()
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
        RUNTIME_NODE_EXTERNAL_ID,
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
    assert_eq!(failed.node_id.as_deref(), Some(RUNTIME_NODE_EXTERNAL_ID));
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
async fn project_task_workspace_sync_failure_also_fails_project_task_writeback() {
    let temp = tempfile::tempdir().unwrap();
    let capture = CommandFailureCapture::default();
    let http_server = serve_command_failures(capture.clone()).await;
    let marker_file = temp.path().join("provider-ran.txt");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-should-not-run-project-task",
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
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);

    let home = temp
        .path()
        .join("workspaces")
        .join("teams")
        .join(TEAM_ID)
        .join("employees")
        .join(DIGITAL_EMPLOYEE_ID);
    std::fs::create_dir_all(&home).unwrap();

    let content = "# Project Task Execution Contract\n";
    let mut command = session_command_in_home(
        &home,
        "cmd-project-task-bad-workspace",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("complete the project task"),
        None,
    );
    command.payload["workspace_files"] =
        json!([workspace_file_with_hash(content, "not-the-content-hash")]);
    command.payload["metadata"] =
        project_task_attempt_metadata(vec!["execution_summary", "evidence_refs"]);

    let error = executor
        .handle_command(command)
        .await
        .expect_err("bad workspace file hash should reject before provider start");
    assert!(
        error.to_string().contains("content_hash mismatch"),
        "unexpected error: {error}"
    );

    let failed = wait_for_writeback(capture.fail.clone()).await;
    assert_eq!(failed.command_id, "cmd-project-task-bad-workspace");

    let project_task_failed =
        wait_for_project_task_writeback(capture.project_task_fail.clone()).await;
    assert_eq!(project_task_failed.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(project_task_failed.project_task_id, PROJECT_TASK_ID);
    assert_eq!(project_task_failed.lease_token, PROJECT_TASK_LEASE_TOKEN);
    assert_eq!(project_task_failed.runtime_node_id, RUNTIME_NODE_ID);
    assert_eq!(
        project_task_failed.idempotency_key,
        format!(
            "project-task-attempt:{PROJECT_TASK_ATTEMPT_ID}:fail:cmd-project-task-bad-workspace"
        )
    );
    assert_eq!(
        project_task_failed.authorization.as_deref(),
        Some("Bearer session-token")
    );
    assert_eq!(
        project_task_failed.node_id.as_deref(),
        Some(RUNTIME_NODE_EXTERNAL_ID)
    );
    assert!(
        project_task_failed
            .payload
            .get("digital_employee_id")
            .is_none()
    );
    assert!(
        project_task_failed
            .payload
            .get("failure_summary")
            .and_then(Value::as_str)
            .is_some_and(|value| value.contains("content_hash mismatch"))
    );
    assert_eq!(
        project_task_failed.payload["failure_family"],
        "invalid_contract"
    );
    assert_eq!(project_task_failed.payload["retryable"], false);
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
        std::fs::read_to_string(home.join("context.md")).unwrap(),
        content
    );
    assert!(home.join(".claude").is_dir());
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

    assert_eq!(
        snapshot.workspace_path,
        temp.path()
            .join("workspaces")
            .join("workspaces")
            .join("unscoped")
            .join("manual")
            .join("attempt")
    );
    assert_eq!(snapshot.agent_home_dir.as_deref(), Some(home.as_path()));
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
async fn stop_session_accepts_control_plane_lightweight_payload() {
    let temp = TempDir::new().expect("tempdir");
    let marker_file = temp.path().join("lightweight-stop-marker.txt");
    let fake_claude = make_script(
        temp.path(),
        "lightweight-stop-claude",
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
            "cmd-start-lightweight-stop",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("start work that will be stopped by control plane payload"),
            None,
        ))
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");

    let stop = executor
        .handle_command(RuntimeCommand {
            id: "cmd-stop-lightweight".to_string(),
            command_type: RuntimeCommandType::StopSession,
            payload: json!({
                "provider_run_protocol": "provider-run/v1",
                "run_id": "77777777-7777-4777-8777-777777777777",
                "task_id": "88888888-8888-4888-8888-888888888888",
                "command_id": "cmd-stop-lightweight",
                "start_command_id": "cmd-start-lightweight-stop",
                "reason": "test cleanup",
                "grace_sec": null
            }),
        })
        .await
        .expect("stop_session accepted");

    assert_eq!(stop.run_id.as_deref(), Some(started_run_id.as_str()));
    wait_for_status(&executor.runs(), &started_run_id, RunStatus::Cancelled).await;
    tokio::time::sleep(Duration::from_millis(700)).await;
    assert!(
        !marker_file.exists(),
        "lightweight stop_session did not kill provider process"
    );
}

#[tokio::test(flavor = "current_thread")]
async fn lightweight_stop_session_writes_cancelled_terminal_for_start_command() {
    let temp = TempDir::new().expect("tempdir");
    let capture = CommandCompletionCapture::default();
    let server = serve_command_cancel_writebacks(capture.clone()).await;
    let fake_claude = make_script(
        temp.path(),
        "cancel-writeback-claude",
        r#"#!/usr/bin/env bash
sleep 5
"#,
    );
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);

    let start = executor
        .handle_command(session_command_in_home(
            &home,
            "cmd-start-cancel-writeback",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("start work that will be cancelled"),
            None,
        ))
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");

    executor
        .handle_command(RuntimeCommand {
            id: "cmd-stop-cancel-writeback".to_string(),
            command_type: RuntimeCommandType::StopSession,
            payload: json!({
                "provider_run_protocol": "provider-run/v1",
                "run_id": "77777777-7777-4777-8777-777777777777",
                "task_id": "88888888-8888-4888-8888-888888888888",
                "command_id": "cmd-stop-cancel-writeback",
                "start_command_id": "cmd-start-cancel-writeback",
                "reason": "operator cancelled",
                "grace_sec": null
            }),
        })
        .await
        .expect("stop_session accepted");

    wait_for_status(&executor.runs(), &started_run_id, RunStatus::Cancelled).await;
    let cancel = wait_for_writeback(capture.complete.clone()).await;
    assert_eq!(cancel.command_id, "cmd-start-cancel-writeback");
    assert_eq!(cancel.payload["status"], "cancelled");
    assert_eq!(cancel.payload["summary"], "operator cancelled");
    assert_eq!(
        cancel.authorization.as_deref(),
        Some("Bearer session-token")
    );
    assert_eq!(cancel.node_id.as_deref(), Some(RUNTIME_NODE_EXTERNAL_ID));
    server.task.abort();
}

#[tokio::test(flavor = "current_thread")]
async fn stop_session_for_project_task_dispatch_also_fails_project_task_writeback() {
    let temp = TempDir::new().expect("tempdir");
    let capture = CommandFailureCapture::default();
    let server = serve_command_failures(capture.clone()).await;
    let fake_claude = make_script(
        temp.path(),
        "project-task-stop-claude",
        r#"#!/usr/bin/env bash
sleep 5
"#,
    );
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);

    let mut start = session_command_in_home(
        &home,
        "cmd-start-project-task-stop",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("start work that will be cancelled"),
        None,
    );
    start.payload["metadata"] = project_task_attempt_metadata(vec!["execution_summary"]);
    let start = executor
        .handle_command(start)
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");

    executor
        .handle_command(RuntimeCommand {
            id: "cmd-stop-project-task-stop".to_string(),
            command_type: RuntimeCommandType::StopSession,
            payload: json!({
                "provider_run_protocol": "provider-run/v1",
                "run_id": "77777777-7777-4777-8777-777777777777",
                "task_id": "88888888-8888-4888-8888-888888888888",
                "command_id": "cmd-stop-project-task-stop",
                "start_command_id": "cmd-start-project-task-stop",
                "reason": "operator cancelled",
                "grace_sec": null
            }),
        })
        .await
        .expect("stop_session accepted");

    wait_for_status(&executor.runs(), &started_run_id, RunStatus::Cancelled).await;
    let cancel = wait_for_writeback(capture.cancelled.clone()).await;
    assert_eq!(cancel.command_id, "cmd-start-project-task-stop");
    assert_eq!(cancel.payload["status"], "cancelled");

    let project_task_failed =
        wait_for_project_task_writeback(capture.project_task_fail.clone()).await;
    assert_eq!(project_task_failed.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(project_task_failed.project_task_id, PROJECT_TASK_ID);
    assert_eq!(project_task_failed.lease_token, PROJECT_TASK_LEASE_TOKEN);
    assert_eq!(project_task_failed.runtime_node_id, RUNTIME_NODE_ID);
    assert_eq!(
        project_task_failed.idempotency_key,
        format!("project-task-attempt:{PROJECT_TASK_ATTEMPT_ID}:fail:cmd-start-project-task-stop")
    );
    assert_eq!(
        project_task_failed.authorization.as_deref(),
        Some("Bearer session-token")
    );
    assert_eq!(
        project_task_failed.node_id.as_deref(),
        Some(RUNTIME_NODE_EXTERNAL_ID)
    );
    assert!(
        project_task_failed
            .payload
            .get("digital_employee_id")
            .is_none()
    );
    assert_eq!(
        project_task_failed.payload["failure_summary"],
        "operator cancelled"
    );
    assert_eq!(
        project_task_failed.payload["failure_family"],
        "business_cancelled"
    );
    assert_eq!(project_task_failed.payload["retryable"], false);
    server.task.abort();
}

#[tokio::test(flavor = "current_thread")]
async fn project_task_budget_trip_cancels_provider_and_fails_project_task_writeback() {
    let temp = TempDir::new().expect("tempdir");
    let capture = command_completion_capture_with_budget_trip();
    let server = serve_command_completion_writebacks(capture.clone()).await;
    let marker_file = temp.path().join("budget-cancelled.txt");
    let fake_claude = make_script(
        temp.path(),
        "budget-fuse-claude",
        &format!(
            r#"#!/usr/bin/env bash
sleep 3
printf '%s\n' survived > {}
sleep 30
"#,
            shell_quote(&marker_file)
        ),
    );
    let control_plane = ControlPlaneClient::with_session_token(
        format!("http://{}", server.addr),
        "session-token",
        RUNTIME_NODE_EXTERNAL_ID,
    );
    let executor = configure_runtime_with_control_plane(&temp, fake_claude, control_plane);
    let home = prepare_employee_home(&temp);

    let mut start = session_command_in_home(
        &home,
        "cmd-project-task-budget-trip",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("start work that will exceed budget"),
        None,
    );
    start.payload["metadata"] = project_task_attempt_metadata_with_budget_trip();
    let start = executor
        .handle_command(start)
        .await
        .expect("start_session accepted");
    let started_run_id = start.run_id.expect("started run id");

    wait_for_project_task_writeback(capture.project_task_fail.clone()).await;
    let runtime_fail = wait_for_writeback(capture.fail.clone()).await;
    assert_eq!(runtime_fail.command_id, "cmd-project-task-budget-trip");
    assert_eq!(runtime_fail.payload["status"], "failed");

    let project_task_failed =
        wait_for_project_task_writeback(capture.project_task_fail.clone()).await;
    assert_eq!(project_task_failed.attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(
        project_task_failed.idempotency_key,
        format!("project-task-attempt:{PROJECT_TASK_ATTEMPT_ID}:fail:cmd-project-task-budget-trip")
    );
    assert_eq!(
        project_task_failed.payload["failure_summary"],
        "wall_clock_exceeded"
    );
    assert_eq!(project_task_failed.payload["failure_family"], "budget_fuse");
    assert_eq!(project_task_failed.payload["retryable"], false);
    wait_for_status(&executor.runs(), &started_run_id, RunStatus::Failed).await;
    let heartbeats = capture
        .project_task_budget_heartbeats
        .lock()
        .expect("budget heartbeats lock")
        .clone();
    assert!(!heartbeats.is_empty(), "budget heartbeat was not sent");
    assert_eq!(heartbeats[0].attempt_id, PROJECT_TASK_ATTEMPT_ID);
    assert_eq!(
        heartbeats[0].authorization.as_deref(),
        Some("Bearer session-token")
    );
    assert_eq!(
        heartbeats[0].node_id.as_deref(),
        Some(RUNTIME_NODE_EXTERNAL_ID)
    );
    assert_eq!(heartbeats[0].payload["project_id"], PROJECT_ID);
    assert_eq!(heartbeats[0].payload["project_task_id"], PROJECT_TASK_ID);
    tokio::time::sleep(Duration::from_millis(2500)).await;
    assert!(
        !marker_file.exists(),
        "provider process was not cancelled before it wrote its survival marker"
    );
    server.task.abort();
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

#[tokio::test]
async fn stream_error_drain_early_exit_still_rolls_back_session_mcp_config() {
    let temp = TempDir::new().expect("tempdir");
    let home = prepare_employee_home(&temp);
    let captured_mcp = temp.path().join("captured-mcp.json");
    // The fake CLI copies the injected home MCP config while the session is
    // live (proving the injection happened), then exits nonzero. The nonzero
    // exit surfaces as an Err item on the provider event stream, so
    // drain_provider_events bails via `?` before its tail rollback hook —
    // this exercises spawn_provider_event_drain's Err-branch backstop.
    let fake_claude = make_script(
        temp.path(),
        "fake-claude-stream-error-mcp",
        &format!(
            r#"#!/usr/bin/env bash
cp {mcp_config} {captured}
printf '%s\n' '{{"type":"system","session_id":"session-stream-error-mcp"}}'
exit 1
"#,
            mcp_config = home.join(".mcp.json").display(),
            captured = captured_mcp.display(),
        ),
    );
    let executor = configure_runtime(&temp, fake_claude);
    let mut command = session_command_in_home(
        &home,
        "cmd-stream-error-mcp-rollback",
        RuntimeCommandType::StartSession,
        "new",
        None,
        Some("trigger an infra-level stream failure"),
        None,
    );
    command.payload["mcp_servers"] = json!([{
        "server_id": "88888888-8888-4888-8888-888888888888",
        "server_key": "github",
        "transport": "streamable_http",
        "url": "https://api.githubcopilot.com/mcp/"
    }]);

    let outcome = executor
        .handle_command(command)
        .await
        .expect("start_session accepted");
    let run_id = outcome.run_id.expect("run id");
    let snapshot = wait_for_status(&executor.runs(), &run_id, RunStatus::Failed).await;
    assert!(
        snapshot
            .error
            .as_deref()
            .is_some_and(|error| error.contains("claude exited")),
        "expected provider exit failure, got {:?}",
        snapshot.error
    );

    let captured = fs::read_to_string(&captured_mcp)
        .expect("provider must have seen the injected home mcp config");
    assert!(
        captured.contains("github"),
        "injected mcp config should list the server, got: {captured}"
    );

    // finish_failed runs after the backstop rollback in the same task, so once
    // the run is Failed the session MCP config and its manifest must be gone.
    assert!(
        !home.join(".mcp.json").exists(),
        "session mcp config must be rolled back after an infra-failure drain"
    );
    assert!(
        !superteam_runtime_agent::mcp_config::manifest_path(&home).exists(),
        "session mcp manifest must be removed after rollback"
    );
}
