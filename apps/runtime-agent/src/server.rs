use std::net::SocketAddr;
use std::path::PathBuf;

use axum::Json;
use axum::Json as AxumJson;
use axum::Router;
use axum::extract::ws::{Message, WebSocket, WebSocketUpgrade};
use axum::extract::{Path, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use axum::routing::{get, post};
use futures::{SinkExt, StreamExt, TryStreamExt};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tokio::net::TcpListener;
use tokio::task::JoinHandle;

use crate::health::{ProviderHealth, ProviderHealthProbe, probe_provider_health};
use crate::providers::claude::ClaudeProvider;
use crate::providers::opencode::OpenCodeProvider;
use crate::providers::{ProviderAdapter, ProviderRequest};
use crate::runs::{RunSpec, RunStatus, RuntimeRunStore};

#[derive(Debug, Clone)]
pub struct RuntimeHttpConfig {
    pub node_id: String,
    pub run_log_dir: PathBuf,
    pub claude_bin: PathBuf,
    pub opencode_bin: PathBuf,
}

#[derive(Clone)]
struct RuntimeHttpState {
    config: RuntimeHttpConfig,
    runs: RuntimeRunStore,
}

pub struct RuntimeHttpServer {
    addr: SocketAddr,
    task: JoinHandle<()>,
}

impl RuntimeHttpServer {
    pub async fn bind_ephemeral(config: RuntimeHttpConfig) -> anyhow::Result<Self> {
        Self::bind(([127, 0, 0, 1], 0).into(), config).await
    }

    pub async fn bind(addr: SocketAddr, config: RuntimeHttpConfig) -> anyhow::Result<Self> {
        let listener = TcpListener::bind(addr).await?;
        let addr = listener.local_addr()?;
        let state = RuntimeHttpState {
            runs: RuntimeRunStore::new(config.run_log_dir.clone()),
            config,
        };
        let app = router(state);
        let task = tokio::spawn(async move {
            if let Err(error) = axum::serve(listener, app).await {
                eprintln!("runtime http server stopped: {error}");
            }
        });
        Ok(Self { addr, task })
    }

    pub fn addr(&self) -> SocketAddr {
        self.addr
    }
}

impl Drop for RuntimeHttpServer {
    fn drop(&mut self) {
        self.task.abort();
    }
}

fn router(state: RuntimeHttpState) -> Router {
    Router::new()
        .route("/health", get(health))
        .route("/providers", get(providers))
        .route("/runs", post(create_run))
        .route("/runs/{run_id}", get(get_run))
        .route("/runs/{run_id}/events", get(get_run_events))
        .route("/runs/{run_id}/cancel", post(cancel_run))
        .route("/ws", get(ws_events))
        .with_state(state)
}

#[derive(Debug, Serialize)]
struct HealthResponse {
    node_id: String,
    status: String,
}

async fn health(State(state): State<RuntimeHttpState>) -> Json<HealthResponse> {
    Json(HealthResponse {
        node_id: state.config.node_id,
        status: "ok".to_string(),
    })
}

async fn providers(State(state): State<RuntimeHttpState>) -> Json<Vec<ProviderHealth>> {
    let claude = probe_provider_health(ProviderHealthProbe {
        kind: "claude".to_string(),
        bin_path: state.config.claude_bin,
    });
    let opencode = probe_provider_health(ProviderHealthProbe {
        kind: "opencode".to_string(),
        bin_path: state.config.opencode_bin,
    });
    let (claude, opencode) = tokio::join!(claude, opencode);
    Json(vec![claude, opencode])
}

#[derive(Debug, Deserialize)]
struct CreateRunRequest {
    provider_kind: String,
    workspace_path: PathBuf,
    prompt: String,
    session_id: Option<String>,
    #[serde(default)]
    continue_session: bool,
    model: Option<String>,
}

async fn create_run(
    State(state): State<RuntimeHttpState>,
    Json(request): Json<CreateRunRequest>,
) -> Result<Response, ApiError> {
    let spec = RunSpec {
        provider_kind: request.provider_kind.trim().to_string(),
        workspace_path: request.workspace_path,
        prompt: request.prompt,
        session_id: request.session_id,
        continue_session: request.continue_session,
        model: request.model,
    };
    validate_run_spec(&spec)?;

    let snapshot = state
        .runs
        .start_run(spec.clone(), None)
        .await
        .map_err(ApiError::internal)?;
    spawn_provider_run(state.clone(), snapshot.id.clone(), spec);
    Ok((StatusCode::ACCEPTED, Json(snapshot)).into_response())
}

async fn get_run(
    State(state): State<RuntimeHttpState>,
    Path(run_id): Path<String>,
) -> Result<Json<crate::runs::RunSnapshot>, ApiError> {
    state
        .runs
        .get_run(&run_id)
        .await
        .map(Json)
        .ok_or_else(|| ApiError::not_found(format!("run not found: {run_id}")))
}

async fn get_run_events(
    State(state): State<RuntimeHttpState>,
    Path(run_id): Path<String>,
) -> Result<Json<Vec<crate::runs::RunEventRecord>>, ApiError> {
    state
        .runs
        .events(&run_id)
        .await
        .map(Json)
        .ok_or_else(|| ApiError::not_found(format!("run not found: {run_id}")))
}

#[derive(Debug, Deserialize)]
struct CancelRunRequest {
    reason: Option<String>,
}

async fn cancel_run(
    State(state): State<RuntimeHttpState>,
    Path(run_id): Path<String>,
    request: Option<AxumJson<CancelRunRequest>>,
) -> Result<StatusCode, ApiError> {
    state
        .runs
        .cancel_run(&run_id, request.and_then(|body| body.0.reason))
        .await
        .map_err(|error| {
            if error.to_string().contains("run not found") {
                ApiError::not_found(error.to_string())
            } else {
                ApiError::internal(error)
            }
        })?;
    Ok(StatusCode::NO_CONTENT)
}

async fn ws_events(State(state): State<RuntimeHttpState>, ws: WebSocketUpgrade) -> Response {
    ws.on_upgrade(move |socket| stream_ws_events(socket, state))
}

async fn stream_ws_events(socket: WebSocket, state: RuntimeHttpState) {
    let (mut sender, mut receiver) = socket.split();
    let mut events = state.runs.subscribe();
    loop {
        tokio::select! {
            event = events.recv() => {
                let Ok(event) = event else {
                    break;
                };
                let Ok(text) = serde_json::to_string(&json!({
                    "event": "run.event",
                    "data": event,
                })) else {
                    continue;
                };
                if sender.send(Message::Text(text.into())).await.is_err() {
                    break;
                }
            }
            message = receiver.next() => {
                match message {
                    Some(Ok(Message::Close(_))) | None => break,
                    Some(Ok(_)) => {}
                    Some(Err(_)) => break,
                }
            }
        }
    }
}

fn spawn_provider_run(state: RuntimeHttpState, run_id: String, spec: RunSpec) {
    tokio::spawn(async move {
        let result = match spec.provider_kind.as_str() {
            "claude" => {
                let provider = ClaudeProvider::new(state.config.claude_bin.clone());
                run_provider_stream(state.runs.clone(), run_id.clone(), provider, spec).await
            }
            "opencode" => {
                let provider = OpenCodeProvider::new(state.config.opencode_bin.clone());
                run_provider_stream(state.runs.clone(), run_id.clone(), provider, spec).await
            }
            _ => Err(anyhow::anyhow!(
                "unsupported provider kind: {}",
                spec.provider_kind
            )),
        };

        if let Err(error) = result {
            if let Some(snapshot) = state.runs.get_run(&run_id).await {
                if snapshot.status == RunStatus::Cancelled {
                    return;
                }
            }
            let _ = state.runs.finish_failed(&run_id, error.to_string()).await;
        }
    });
}

async fn run_provider_stream(
    runs: RuntimeRunStore,
    run_id: String,
    provider: impl ProviderAdapter,
    spec: RunSpec,
) -> anyhow::Result<()> {
    let provider_run = provider
        .start(ProviderRequest {
            prompt: spec.prompt,
            workspace_path: spec.workspace_path,
            session_id: spec.session_id,
            continue_session: spec.continue_session,
            model: spec.model,
        })
        .await?;

    runs.attach_handle(&run_id, provider_run.handle).await?;
    provider_run
        .events
        .try_for_each(|event| {
            let runs = runs.clone();
            let run_id = run_id.clone();
            async move {
                runs.record_event(&run_id, event).await?;
                Ok(())
            }
        })
        .await
}

fn validate_run_spec(spec: &RunSpec) -> Result<(), ApiError> {
    if spec.provider_kind != "claude" && spec.provider_kind != "opencode" {
        return Err(ApiError::bad_request(format!(
            "unsupported provider kind: {}",
            spec.provider_kind
        )));
    }
    if spec.prompt.trim().is_empty() {
        return Err(ApiError::bad_request("prompt is required"));
    }
    if !spec.workspace_path.is_absolute() {
        return Err(ApiError::bad_request("workspace_path must be absolute"));
    }
    Ok(())
}

struct ApiError {
    status: StatusCode,
    code: &'static str,
    message: String,
}

impl ApiError {
    fn bad_request(message: impl Into<String>) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            code: "bad_request",
            message: message.into(),
        }
    }

    fn not_found(message: impl Into<String>) -> Self {
        Self {
            status: StatusCode::NOT_FOUND,
            code: "not_found",
            message: message.into(),
        }
    }

    fn internal(error: impl std::fmt::Display) -> Self {
        Self {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            code: "internal_error",
            message: error.to_string(),
        }
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        (
            self.status,
            Json(json!({
                "error": self.message,
                "code": self.code,
            })),
        )
            .into_response()
    }
}
