use std::path::PathBuf;
use std::process::Stdio;

use anyhow::Context;
use async_trait::async_trait;
use serde_json::Value;
use tokio::process::Command;

use crate::events::ProviderEvent;
use crate::providers::{
    ProviderAdapter, ProviderRequest, ProviderRun, apply_environment, stream_child_events,
};

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
        apply_environment(&mut command, request);
        command.arg("exec");
        if request.continue_session {
            command.arg("resume");
            if let Some(session_id) = &request.session_id {
                command.arg(session_id);
            } else {
                command.arg("--last");
            }
            command.arg("--json");
            command.arg("--cd").arg(&request.workspace_path);
            command.arg("--dangerously-bypass-approvals-and-sandbox");
            command.arg("--skip-git-repo-check");
        } else {
            command.arg("--json");
            command.arg("--cd").arg(codex_cd_arg(&request.workspace_path));
            command.arg("--dangerously-bypass-approvals-and-sandbox");
            command.arg("--skip-git-repo-check");
        }
        if let Some(model) = &request.model {
            command.arg("--model").arg(model);
        }
        command.arg(&request.prompt);
        command
    }
}

fn codex_cd_arg(workspace_path: &std::path::Path) -> PathBuf {
    if workspace_path.is_absolute() {
        workspace_path.to_path_buf()
    } else {
        PathBuf::from(".")
    }
}

#[async_trait]
impl ProviderAdapter for CodexProvider {
    async fn start(&self, request: ProviderRequest) -> anyhow::Result<ProviderRun> {
        let mut command = self.build_command(&request);
        command.stdin(Stdio::null());
        command.stdout(Stdio::piped());
        command.stderr(Stdio::piped());
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

    if matches!(
        event_type,
        "error" | "turn.error" | "turn.failed" | "failed" | "failure"
    ) {
        anyhow::bail!(
            "{}",
            extract_error_message(&event).unwrap_or("codex failed")
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

    if event_type == "item.completed" {
        if let Some(text) = extract_completed_agent_message_text(&event) {
            return Ok(Some(ProviderEvent::TextDelta { text }));
        }
    }

    if matches!(
        event_type,
        "turn.completed" | "turn_complete" | "completed" | "result" | "done"
    ) {
        return Ok(Some(ProviderEvent::TurnCompleted {
            summary: extract_summary(&event),
            usage: crate::providers::usage::extract_usage(&event),
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

fn extract_completed_agent_message_text(event: &Value) -> Option<String> {
    let item = event.get("item")?;
    let item_type = item.get("type").and_then(|value| value.as_str())?;
    if item_type != "agent_message" {
        return None;
    }
    item.get("text")
        .and_then(|value| value.as_str())
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

fn extract_error_message(event: &Value) -> Option<&str> {
    first_string(event, &["message", "error", "reason"])
        .or_else(|| nested_string(event, &["error", "message"]))
        .or_else(|| nested_string(event, &["error", "reason"]))
        .or_else(|| nested_string(event, &["error", "details"]))
        .or_else(|| nested_string(event, &["error", "detail"]))
        .or_else(|| nested_string(event, &["failure", "message"]))
        .or_else(|| nested_string(event, &["failure", "reason"]))
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
