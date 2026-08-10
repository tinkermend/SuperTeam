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
            command.arg("--dangerously-bypass-approvals-and-sandbox");
            command.arg("--skip-git-repo-check");
        } else {
            command.arg("--json");
            command
                .arg("--cd")
                .arg(codex_cd_arg(&request.workspace_path));
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
    async fn start(
        &self,
        request: ProviderRequest,
        raw_sink: std::sync::Arc<dyn crate::raw_log::RawLineSink>,
    ) -> anyhow::Result<ProviderRun> {
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
            raw_sink,
            child,
            stdout,
            stderr,
        ))
    }
}

/// Explicit type-branch mapping (Phase 3). Cross-type greedy key matching is
/// intentionally removed so unknown types can surface as `native_unmapped`.
pub fn parse_codex_event(value: &str) -> anyhow::Result<Vec<ProviderEvent>> {
    let Some(event) = crate::providers::parse_line_json("codex", value) else {
        return Ok(Vec::new());
    };
    let event_type = event
        .get("type")
        .and_then(|value| value.as_str())
        .unwrap_or_default();

    match event_type {
        "error" | "turn.error" | "turn.failed" | "failed" | "failure" => {
            anyhow::bail!(
                "{}",
                extract_error_message(&event).unwrap_or("codex failed")
            );
        }
        // Session / thread lifecycle.
        "session" | "session.started" | "session.created" | "thread.started" | "thread.created" => {
            if let Some(session_id) = extract_session_id_for_session_event(&event) {
                Ok(vec![ProviderEvent::SessionStarted {
                    session_id,
                    session_state: None,
                }])
            } else {
                // Known type but no usable id — not "unknown type".
                Ok(Vec::new())
            }
        }
        // Text deltas and completed agent messages.
        "message.delta" | "text.delta" | "agent.message" | "message" => {
            if let Some(text) = extract_text_for_message_event(&event) {
                Ok(vec![ProviderEvent::TextDelta { text }])
            } else {
                Ok(Vec::new())
            }
        }
        "item.completed" => {
            if let Some(text) = extract_completed_agent_message_text(&event) {
                Ok(vec![ProviderEvent::TextDelta { text }])
            } else {
                // item.completed for non-agent_message kinds (tools, etc.) —
                // no platform tool mapping yet; leave empty rather than unmapped
                // so tool-less capability stays honest without noise.
                Ok(Vec::new())
            }
        }
        "turn.started" => Ok(vec![ProviderEvent::TurnStarted]),
        "turn.completed" | "turn_complete" | "completed" | "result" | "done" => {
            Ok(vec![ProviderEvent::TurnCompleted {
                summary: extract_summary(&event),
                usage: crate::providers::usage::extract_usage(&event),
            }])
        }
        // Known noise / protocol types with no L2 event.
        "token_count" | "rate_limits" | "response.created" | "response.in_progress" => {
            Ok(Vec::new())
        }
        "" => Ok(vec![ProviderEvent::native_unmapped(
            None,
            "unrecognized_type",
        )]),
        other => Ok(vec![ProviderEvent::native_unmapped(
            Some(other.to_string()),
            "unrecognized_type",
        )]),
    }
}

fn extract_session_id_for_session_event(event: &Value) -> Option<String> {
    first_string(event, &["session_id", "sessionId", "thread_id", "threadId"])
        .or_else(|| nested_string(event, &["session", "id"]))
        .or_else(|| nested_string(event, &["thread", "id"]))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn extract_text_for_message_event(event: &Value) -> Option<String> {
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn unknown_type_is_native_unmapped_not_greedy_session() {
        // Pre-Phase-3 greedy extract_session_id would have treated any object
        // with session_id as SessionStarted regardless of type.
        let events = parse_codex_event(
            r#"{"type":"metrics.sample","session_id":"should-not-become-session","text":"noise"}"#,
        )
        .expect("parse");
        assert_eq!(
            events,
            vec![ProviderEvent::native_unmapped(
                Some("metrics.sample".to_string()),
                "unrecognized_type"
            )]
        );
    }

    #[test]
    fn thread_started_reads_nested_thread_id() {
        let events =
            parse_codex_event(r#"{"type":"thread.started","thread":{"id":"thread-1"}}"#)
                .expect("parse");
        assert_eq!(
            events,
            vec![ProviderEvent::SessionStarted {
                session_id: "thread-1".to_string(),
                session_state: None,
            }]
        );
    }
}
