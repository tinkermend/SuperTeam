use std::path::PathBuf;

use anyhow::Context;
use async_trait::async_trait;
use tokio::process::Command;

use crate::events::ProviderEvent;
use crate::providers::{ProviderAdapter, ProviderRequest, ProviderRun, stream_child_events};

#[derive(Debug, Clone)]
pub struct ClaudeProvider {
    bin_path: PathBuf,
}

impl ClaudeProvider {
    pub fn new(bin_path: impl Into<PathBuf>) -> Self {
        Self {
            bin_path: bin_path.into(),
        }
    }

    pub fn build_command(&self, request: &ProviderRequest) -> Command {
        let mut command = Command::new(&self.bin_path);
        command.current_dir(&request.workspace_path);
        command.arg("-p").arg(&request.prompt);
        command.arg("--output-format").arg("stream-json");
        command.arg("--verbose");
        command.arg("--include-partial-messages");
        if request.continue_session {
            if let Some(session_id) = &request.session_id {
                command.arg("--resume").arg(session_id);
            } else {
                command.arg("--continue");
            }
        } else if let Some(session_id) = &request.session_id {
            command.arg("--session-id").arg(session_id);
        }
        if let Some(model) = &request.model {
            command.arg("--model").arg(model);
        }
        command
    }
}

#[async_trait]
impl ProviderAdapter for ClaudeProvider {
    async fn start(&self, request: ProviderRequest) -> anyhow::Result<ProviderRun> {
        let mut command = self.build_command(&request);
        command.stdout(std::process::Stdio::piped());
        command.stderr(std::process::Stdio::piped());
        let mut child = command.spawn().context("failed to spawn claude")?;
        let stdout = child
            .stdout
            .take()
            .context("failed to capture claude stdout")?;
        let stderr = child
            .stderr
            .take()
            .context("failed to capture claude stderr")?;
        Ok(stream_child_events(
            "claude",
            parse_claude_event,
            child,
            stdout,
            stderr,
        ))
    }
}

pub fn parse_claude_event(value: &str) -> anyhow::Result<Option<ProviderEvent>> {
    let event: serde_json::Value = serde_json::from_str(value)?;
    let event_type = event
        .get("type")
        .and_then(|v| v.as_str())
        .unwrap_or_default();
    match event_type {
        "system" => {
            let session_id = event
                .get("session_id")
                .or_else(|| event.get("sessionId"))
                .and_then(|v| v.as_str())
                .unwrap_or_default();
            if session_id.is_empty() {
                Ok(None)
            } else {
                Ok(Some(ProviderEvent::SessionStarted {
                    session_id: session_id.to_string(),
                    session_state: None,
                }))
            }
        }
        "assistant" => {
            let text = event
                .get("message")
                .and_then(|message| message.get("content"))
                .and_then(|content| content.as_array())
                .and_then(|content| content.iter().find_map(|block| block.get("text")))
                .and_then(|text| text.as_str())
                .unwrap_or_default();
            if text.is_empty() {
                Ok(None)
            } else {
                Ok(Some(ProviderEvent::TextDelta {
                    text: text.to_string(),
                }))
            }
        }
        "result" => Ok(Some(ProviderEvent::TurnCompleted {
            summary: event
                .get("result")
                .and_then(|v| v.as_str())
                .map(ToString::to_string),
        })),
        _ => Ok(None),
    }
}
