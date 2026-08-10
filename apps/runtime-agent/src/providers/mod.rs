pub mod catalog;
pub mod claude;
pub mod codex;
pub mod error_map;
pub mod opencode;
pub mod usage;

use std::collections::{BTreeMap, VecDeque};
use std::path::PathBuf;
use std::process::ExitStatus;
use std::sync::Arc;

use async_trait::async_trait;
use futures::StreamExt;
use futures::stream::{self, BoxStream};
use serde::{Deserialize, Serialize};
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::{Child, ChildStderr, ChildStdout};
use tokio::sync::Mutex;
use tokio::task::JoinHandle;

use crate::events::{ErrorNative, ProviderEvent};
use crate::providers::error_map::{
    ProviderStreamError, code as error_code, envelope_for_code, refine_exit_code,
};
use crate::raw_log::{RawLineSink, RawStream};

pub type ProviderEventStream = BoxStream<'static, anyhow::Result<ProviderEvent>>;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProviderRequest {
    pub prompt: String,
    pub workspace_path: PathBuf,
    /// Legacy alias for the employee capability cache. Do not use it as a
    /// provider auth home.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub agent_home_dir: Option<PathBuf>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub employee_capability_dir: Option<PathBuf>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub capability_manifest_version: Option<String>,
    #[serde(default = "default_provider_auth_mode")]
    pub provider_auth_mode: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mcp_config_path: Option<PathBuf>,
    pub session_id: Option<String>,
    pub continue_session: bool,
    pub model: Option<String>,
    #[serde(default)]
    pub environment: BTreeMap<String, String>,
}

fn default_provider_auth_mode() -> String {
    "host".to_string()
}

pub fn apply_environment(command: &mut tokio::process::Command, request: &ProviderRequest) {
    for (name, value) in &request.environment {
        command.env(name, value);
    }
}

#[async_trait]
pub trait ProviderAdapter: Send + Sync {
    async fn start(
        &self,
        request: ProviderRequest,
        raw_sink: Arc<dyn RawLineSink>,
    ) -> anyhow::Result<ProviderRun>;

    async fn run(
        &self,
        request: ProviderRequest,
        raw_sink: Arc<dyn RawLineSink>,
    ) -> anyhow::Result<ProviderEventStream> {
        Ok(self.start(request, raw_sink).await?.events)
    }
}

/// A single stdout line can carry one `text` block plus N `tool_use` blocks, so
/// a parser must be able to return more than one event per line.
type ProviderParser = fn(&str) -> anyhow::Result<Vec<ProviderEvent>>;

struct ChildStreamState {
    provider_name: &'static str,
    parser: ProviderParser,
    lines: tokio::io::Lines<BufReader<ChildStdout>>,
    pending: VecDeque<ProviderEvent>,
    raw_sink: Arc<dyn RawLineSink>,
    child: SharedChild,
    stderr_task: JoinHandle<std::io::Result<String>>,
    /// Non-JSON stdout lines (noise). Separate from unknown native types.
    unparseable_line_count: u64,
    /// Unknown native type lines (parser produced `native_unmapped`).
    unmapped_native_count: u64,
}

/// Default unmapped/unparseable lines per attempt before a WARN alert is emitted.
pub const DEFAULT_UNMAPPED_ALERT_THRESHOLD: u64 = 5;

/// Env `SUPERTEAM_PROVIDER_UNMAPPED_ALERT_THRESHOLD` (0 disables alerting).
pub fn unmapped_alert_threshold() -> u64 {
    match std::env::var("SUPERTEAM_PROVIDER_UNMAPPED_ALERT_THRESHOLD") {
        Ok(raw) => raw.trim().parse().unwrap_or(DEFAULT_UNMAPPED_ALERT_THRESHOLD),
        Err(_) => DEFAULT_UNMAPPED_ALERT_THRESHOLD,
    }
}

type SharedChild = Arc<Mutex<Child>>;

#[derive(Clone)]
pub struct ProviderRunHandle {
    child: SharedChild,
}

impl ProviderRunHandle {
    pub async fn cancel(&self) -> anyhow::Result<()> {
        let mut child = self.child.lock().await;
        child
            .kill()
            .await
            .map_err(|error| anyhow::anyhow!("failed to cancel provider process: {error}"))
    }
}

pub struct ProviderRun {
    pub events: ProviderEventStream,
    pub handle: ProviderRunHandle,
}

/// Cap on the stderr text retained for the exit message. The full stderr still
/// reaches the raw transcript line by line; only this in-memory tail is bounded,
/// so a provider spamming stderr cannot exhaust the agent's memory.
const STDERR_TAIL_LIMIT_BYTES: usize = 256 * 1024;

pub fn stream_child_events(
    provider_name: &'static str,
    parser: ProviderParser,
    raw_sink: Arc<dyn RawLineSink>,
    child: Child,
    stdout: ChildStdout,
    stderr: ChildStderr,
) -> ProviderRun {
    let child = Arc::new(Mutex::new(child));
    let handle = ProviderRunHandle {
        child: child.clone(),
    };
    let stderr_sink = raw_sink.clone();
    let stderr_task = tokio::spawn(async move {
        let mut stderr_text = String::new();
        let mut lines = BufReader::new(stderr).lines();
        while let Some(line) = lines.next_line().await? {
            // Interleaving stderr with stdout line by line preserves the real
            // ordering of the execution; reading it as one blob at exit does not.
            stderr_sink.write_line(RawStream::Stderr, &line);
            if stderr_text.len() < STDERR_TAIL_LIMIT_BYTES {
                stderr_text.push_str(&line);
                stderr_text.push('\n');
            }
        }
        Ok(stderr_text)
    });

    let state = ChildStreamState {
        provider_name,
        parser,
        lines: BufReader::new(stdout).lines(),
        pending: VecDeque::new(),
        raw_sink,
        child: child.clone(),
        stderr_task,
        unparseable_line_count: 0,
        unmapped_native_count: 0,
    };

    let events = stream::unfold(Some(state), |state| async move {
        let mut state = state?;

        loop {
            if let Some(event) = state.pending.pop_front() {
                return Some((Ok(event), Some(state)));
            }

            match state.lines.next_line().await {
                Ok(Some(line)) => {
                    // Before parsing: a parser error or an unknown event type
                    // must never cost us the original bytes.
                    state.raw_sink.write_line(RawStream::Stdout, &line);
                    if serde_json::from_str::<serde_json::Value>(&line).is_err() {
                        // Noise vs unknown type: non-JSON is unparseable, not unmapped.
                        state.unparseable_line_count =
                            state.unparseable_line_count.saturating_add(1);
                        // Still invoke parser so its skip log stays consistent.
                        let _ = (state.parser)(&line);
                        continue;
                    }
                    match (state.parser)(&line) {
                        Ok(events) => {
                            for event in events {
                                if event.is_native_unmapped() {
                                    state.unmapped_native_count =
                                        state.unmapped_native_count.saturating_add(1);
                                }
                                state.pending.push_back(event);
                            }
                        }
                        // `Err` means the provider reported a failure (e.g. codex
                        // `turn.failed`), not that a line was unreadable. Parsers
                        // drop unreadable lines themselves.
                        Err(error) => return Some((Err(error), Some(state))),
                    }
                }
                Ok(None) => {
                    if state.unmapped_native_count > 0 || state.unparseable_line_count > 0 {
                        let threshold = unmapped_alert_threshold();
                        let total = state
                            .unmapped_native_count
                            .saturating_add(state.unparseable_line_count);
                        if threshold > 0 && total >= threshold {
                            eprintln!(
                                "ALERT provider_stream_drift provider={} unmapped_native={} unparseable_line={} threshold={}",
                                state.provider_name,
                                state.unmapped_native_count,
                                state.unparseable_line_count,
                                threshold,
                            );
                        } else {
                            eprintln!(
                                "{}: stream diagnostics unmapped_native={} unparseable_line={}",
                                state.provider_name,
                                state.unmapped_native_count,
                                state.unparseable_line_count
                            );
                        }
                    }
                    let status = state.child.lock().await.wait().await;
                    let stderr = read_stderr(state.stderr_task).await;
                    return provider_exit_result(state.provider_name, status, stderr)
                        .map(|result| (result, None));
                }
                Err(error) => return Some((Err(error.into()), None)),
            }
        }
    })
    .boxed();

    ProviderRun { events, handle }
}

/// When true, L2 `native_unmapped` events are written back to Control Plane
/// (rate-limited per attempt). Diagnostics still count when false (default).
pub fn emit_native_unmapped_enabled() -> bool {
    match std::env::var("SUPERTEAM_PROVIDER_EMIT_NATIVE_UNMAPPED") {
        Ok(value) => {
            let normalized = value.trim().to_ascii_lowercase();
            normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
        }
        Err(_) => false,
    }
}

/// Max `native_unmapped` writebacks per attempt when emission is enabled.
pub const NATIVE_UNMAPPED_WRITEBACK_LIMIT: u32 = 20;

/// Providers interleave non-JSON noise into stdout. Such a line carries no
/// event, but it must not fail the run — the raw stream keeps it verbatim.
pub(crate) fn parse_line_json(provider_name: &str, line: &str) -> Option<serde_json::Value> {
    match serde_json::from_str(line) {
        Ok(value) => Some(value),
        Err(error) => {
            eprintln!("{provider_name}: skipping unparseable stdout line: {error}");
            None
        }
    }
}

async fn read_stderr(stderr_task: JoinHandle<std::io::Result<String>>) -> String {
    match stderr_task.await {
        Ok(Ok(stderr)) => stderr.trim().to_string(),
        Ok(Err(error)) => format!("failed to read stderr: {error}"),
        Err(error) => format!("failed to join stderr reader: {error}"),
    }
}

fn provider_exit_result(
    provider_name: &str,
    status: std::io::Result<ExitStatus>,
    stderr: String,
) -> Option<anyhow::Result<ProviderEvent>> {
    // stream_child_events still passes a short display name ("claude") for
    // human-readable exit messages ("claude exited with status N"). Envelope
    // provider_type is canonicalized inside envelope_for_code (registry type).
    match status {
        Ok(status) if status.success() => None,
        Ok(status) => {
            let exit_code = status.code();
            let code_label = exit_code
                .map(|code| code.to_string())
                .unwrap_or_else(|| "signal".to_string());
            let mut message = format!("{provider_name} exited with status {code_label}");
            if !stderr.is_empty() {
                message.push_str(": ");
                message.push_str(&stderr);
            }
            let error_code = refine_exit_code(&message);
            let mut envelope = envelope_for_code(error_code, message, provider_name);
            envelope.native = Some(ErrorNative {
                r#type: Some("process_exit".to_string()),
                exit_code,
                excerpt: (!stderr.is_empty()).then(|| {
                    let (excerpt, _) = crate::events::truncate_excerpt(&stderr);
                    excerpt
                }),
                subtype: None,
            });
            Some(Err(ProviderStreamError::new(envelope).into()))
        }
        Err(error) => {
            let message = format!("failed to wait for {provider_name}: {error}");
            Some(Err(ProviderStreamError::from_code(
                error_code::PROVIDER_PROTOCOL_ERROR,
                message,
                provider_name,
            )
            .into()))
        }
    }
}
