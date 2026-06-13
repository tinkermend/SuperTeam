use std::path::PathBuf;

use anyhow::Context;
use async_trait::async_trait;
use serde_json::Value;
use tokio::process::Command;

use crate::events::ProviderEvent;
use crate::providers::{ProviderAdapter, ProviderRequest, ProviderRun, stream_child_events};

#[derive(Debug, Clone)]
pub struct CodexProvider {
    bin_path: PathBuf,
}

impl CodexProvider {
    pub fn new(bin_path: impl Into<PathBuf>) -> Self {
        Self {
            bin_path: bin_path.into(),
        }
    }

    pub fn build_command(&self, request: &ProviderRequest) -> Command {
        let mut command = Command::new(&self.bin_path);
        command.current_dir(&request.workspace_path);
        command.arg("exec");
        if request.continue_session {
            command.arg("resume");
            if let Some(session_id) = &request.session_id {
                command.arg(session_id);
            }
            command.arg("--json");
            command.arg("--dangerously-bypass-approvals-and-sandbox");
        } else {
            command.arg("--json");
            command.arg("--cd").arg(&request.workspace_path);
            command.arg("--ask-for-approval").arg("never");
            command.arg("--sandbox").arg("danger-full-access");
        }
        if let Some(model) = &request.model {
            command.arg("--model").arg(model);
        }
        command.arg(&request.prompt);
        command
    }
}

#[async_trait]
impl ProviderAdapter for CodexProvider {
    async fn start(&self, request: ProviderRequest) -> anyhow::Result<ProviderRun> {
        let mut command = self.build_command(&request);
        command.stdout(std::process::Stdio::piped());
        command.stderr(std::process::Stdio::piped());
        let mut child = command.spawn().context("failed to spawn codex")?;
        let stdout = child
            .stdout
            .take()
            .context("failed to capture codex stdout")?;
        let stderr = child
            .stderr
            .take()
            .context("failed to capture codex stderr")?;
        Ok(stream_child_events(
            "codex",
            parse_codex_event,
            child,
            stdout,
            stderr,
        ))
    }
}

pub fn parse_codex_event(value: &str) -> anyhow::Result<Option<ProviderEvent>> {
    let event: Value = serde_json::from_str(value)?;
    let event_type = event
        .get("type")
        .and_then(|value| value.as_str())
        .unwrap_or_default();

    if matches!(event_type, "error" | "turn.error" | "failed" | "failure") {
        anyhow::bail!(
            "{}",
            first_string(&event, &["message", "error", "reason"]).unwrap_or("codex failed")
        );
    }

    if let Some(session_id) = extract_session_id(&event) {
        return Ok(Some(ProviderEvent::SessionStarted {
            session_id,
            session_state: None,
        }));
    }

    if let Some(text) = extract_text(&event) {
        return Ok(Some(ProviderEvent::TextDelta { text }));
    }

    if matches!(
        event_type,
        "turn.completed" | "turn_complete" | "completed" | "result" | "done"
    ) {
        return Ok(Some(ProviderEvent::TurnCompleted {
            summary: extract_summary(&event),
        }));
    }

    Ok(None)
}

fn extract_session_id(event: &Value) -> Option<String> {
    first_string(event, &["session_id", "sessionId", "thread_id", "threadId"])
        .or_else(|| nested_string(event, &["session", "id"]))
        .or_else(|| nested_string(event, &["thread", "id"]))
        .map(ToString::to_string)
}

fn extract_text(event: &Value) -> Option<String> {
    first_string(event, &["text", "delta", "content"])
        .or_else(|| nested_string(event, &["message", "content"]))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn extract_summary(event: &Value) -> Option<String> {
    first_string(event, &["summary", "result", "final_message"])
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn first_string<'a>(value: &'a Value, keys: &[&str]) -> Option<&'a str> {
    keys.iter()
        .find_map(|key| value.get(*key).and_then(|value| value.as_str()))
}

fn nested_string<'a>(value: &'a Value, path: &[&str]) -> Option<&'a str> {
    let mut current = value;
    for key in path {
        current = current.get(*key)?;
    }
    current.as_str()
}
