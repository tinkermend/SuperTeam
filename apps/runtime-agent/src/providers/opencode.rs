use std::path::PathBuf;

use anyhow::Context;
use async_trait::async_trait;
use futures::Stream;
use tokio::io::{AsyncBufReadExt, BufReader};
use tokio::process::Command;
use tokio_stream::StreamExt;

use crate::events::ProviderEvent;
use crate::providers::{ProviderAdapter, ProviderRequest};

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
        command.arg("run").arg("--format").arg("json");
        if let Some(model) = &request.model {
            command.arg("--model").arg(model);
        }
        if request.continue_session {
            if let Some(session_id) = &request.session_id {
                command.arg("--session").arg(session_id);
            } else {
                command.arg("--continue");
            }
        }
        command.arg(&request.prompt);
        command
    }
}

#[async_trait]
impl ProviderAdapter for OpenCodeProvider {
    async fn run(
        &self,
        request: ProviderRequest,
    ) -> anyhow::Result<Box<dyn Stream<Item = anyhow::Result<ProviderEvent>> + Send + Unpin>> {
        let mut command = self.build_command(&request);
        command.stdout(std::process::Stdio::piped());
        let mut child = command.spawn().context("failed to spawn opencode")?;
        let stdout = child.stdout.take().context("failed to capture opencode stdout")?;
        let reader = BufReader::new(stdout);
        let lines = tokio_stream::wrappers::LinesStream::new(reader.lines());
        let stream = lines.filter_map(|line| match line {
            Ok(line) => parse_opencode_event(&line).transpose(),
            Err(error) => Some(Err(error.into())),
        });
        Ok(Box::new(stream))
    }
}

pub fn parse_opencode_event(value: &str) -> anyhow::Result<Option<ProviderEvent>> {
    let event: serde_json::Value = serde_json::from_str(value)?;
    let event_type = event.get("type").and_then(|v| v.as_str()).unwrap_or_default();
    match event_type {
        "session.updated" | "session" => {
            let session_id = event
                .get("sessionID")
                .or_else(|| event.get("session_id"))
                .or_else(|| event.get("sessionId"))
                .and_then(|v| v.as_str())
                .unwrap_or_default();
            if session_id.is_empty() {
                Ok(None)
            } else {
                Ok(Some(ProviderEvent::SessionStarted {
                    session_id: session_id.to_string(),
                }))
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
                Ok(None)
            } else {
                Ok(Some(ProviderEvent::TextDelta {
                    text: text.to_string(),
                }))
            }
        }
        "turn.completed" | "session.idle" => Ok(Some(ProviderEvent::TurnCompleted {
            summary: None,
        })),
        _ => Ok(None),
    }
}
