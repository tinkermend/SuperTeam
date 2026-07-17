use std::path::PathBuf;

use anyhow::Context;
use async_trait::async_trait;
use tokio::process::Command;

use crate::events::ProviderEvent;
use crate::providers::{
    ProviderAdapter, ProviderRequest, ProviderRun, apply_environment, stream_child_events,
};

#[derive(Debug, Clone)]
pub struct OpenCodeProvider {
    bin_path: PathBuf,
}

impl OpenCodeProvider {
    pub fn new(bin_path: impl Into<PathBuf>) -> Self {
        Self {
            bin_path: bin_path.into(),
        }
    }

    pub fn build_command(&self, request: &ProviderRequest) -> Command {
        let mut command = Command::new(&self.bin_path);
        command.current_dir(&request.workspace_path);
        apply_environment(&mut command, request);
        command.arg("run").arg("--format").arg("json");
        command.arg("--dir").arg(&request.workspace_path);
        if let Some(model) = &request.model {
            command.arg("--model").arg(model);
        }
        if let Some(session_id) = &request.session_id {
            command.arg("--session").arg(session_id);
        } else if request.continue_session {
            command.arg("--continue");
        }
        command.arg(&request.prompt);
        command
    }
}

#[async_trait]
impl ProviderAdapter for OpenCodeProvider {
    async fn start(
        &self,
        request: ProviderRequest,
        raw_sink: std::sync::Arc<dyn crate::raw_log::RawLineSink>,
    ) -> anyhow::Result<ProviderRun> {
        let mut command = self.build_command(&request);
        // `opencode run` appends piped stdin to the prompt and waits for EOF;
        // inheriting the daemon's never-closing stdin therefore hangs the run
        // forever with zero output. The prompt is always passed as an argv
        // argument, so stdin must be closed explicitly.
        command.stdin(std::process::Stdio::null());
        command.stdout(std::process::Stdio::piped());
        command.stderr(std::process::Stdio::piped());
        let mut child = command.spawn().context("failed to spawn opencode")?;
        let stdout = child
            .stdout
            .take()
            .context("failed to capture opencode stdout")?;
        let stderr = child
            .stderr
            .take()
            .context("failed to capture opencode stderr")?;
        Ok(stream_child_events(
            "opencode",
            parse_opencode_event,
            raw_sink,
            child,
            stdout,
            stderr,
        ))
    }
}

pub fn parse_opencode_event(value: &str) -> anyhow::Result<Vec<ProviderEvent>> {
    let Some(event) = crate::providers::parse_line_json("opencode", value) else {
        return Ok(Vec::new());
    };
    let event_type = event
        .get("type")
        .and_then(|v| v.as_str())
        .unwrap_or_default();
    match event_type {
        "session.updated" | "session" => {
            let session_id = event
                .get("sessionID")
                .or_else(|| event.get("session_id"))
                .or_else(|| event.get("sessionId"))
                .and_then(|v| v.as_str())
                .unwrap_or_default();
            if session_id.is_empty() {
                Ok(Vec::new())
            } else {
                Ok(vec![ProviderEvent::SessionStarted {
                    session_id: session_id.to_string(),
                    session_state: None,
                }])
            }
        }
        "message.part.updated" | "message.delta" | "text.delta" => {
            let text = event
                .get("text")
                .or_else(|| event.get("delta"))
                .or_else(|| event.get("content"))
                .and_then(|v| v.as_str())
                .unwrap_or_default();
            if text.is_empty() {
                Ok(Vec::new())
            } else {
                Ok(vec![ProviderEvent::TextDelta {
                    text: text.to_string(),
                }])
            }
        }
        "turn.completed" | "session.idle" => Ok(vec![ProviderEvent::TurnCompleted {
            summary: None,
            usage: crate::providers::usage::extract_usage(&event),
        }]),
        _ => Ok(Vec::new()),
    }
}
