use std::path::PathBuf;

use anyhow::Context;
use async_trait::async_trait;
use tokio::process::Command;

use crate::events::{ProviderEvent, truncate_excerpt};
use crate::providers::{
    ProviderAdapter, ProviderRequest, ProviderRun, apply_environment, stream_child_events,
};

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
        apply_environment(&mut command, request);
        command.arg("-p").arg(&request.prompt);
        command.arg("--output-format").arg("stream-json");
        command.arg("--verbose");
        command.arg("--include-partial-messages");
        command.arg("--permission-mode").arg("bypassPermissions");
        if let Some(mcp_config) = &request.mcp_config_path {
            if mcp_config.exists() {
                command.arg("--mcp-config").arg(mcp_config);
                command.arg("--strict-mcp-config");
            }
        }
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
    async fn start(
        &self,
        request: ProviderRequest,
        raw_sink: std::sync::Arc<dyn crate::raw_log::RawLineSink>,
    ) -> anyhow::Result<ProviderRun> {
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
            raw_sink,
            child,
            stdout,
            stderr,
        ))
    }
}

pub fn parse_claude_event(value: &str) -> anyhow::Result<Vec<ProviderEvent>> {
    let Some(event) = crate::providers::parse_line_json("claude", value) else {
        return Ok(Vec::new());
    };
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
                Ok(Vec::new())
            } else {
                Ok(vec![ProviderEvent::SessionStarted {
                    session_id: session_id.to_string(),
                    session_state: None,
                }])
            }
        }
        "assistant" => Ok(parse_assistant_blocks(&event)),
        "user" => Ok(parse_user_blocks(&event)),
        "result" => Ok(vec![ProviderEvent::TurnCompleted {
            summary: event
                .get("result")
                .and_then(|v| v.as_str())
                .map(ToString::to_string),
            usage: crate::providers::usage::extract_usage(&event),
        }]),
        _ => Ok(Vec::new()),
    }
}

fn message_content(event: &serde_json::Value) -> &[serde_json::Value] {
    event
        .get("message")
        .and_then(|message| message.get("content"))
        .and_then(|content| content.as_array())
        .map(Vec::as_slice)
        .unwrap_or_default()
}

fn parse_assistant_blocks(event: &serde_json::Value) -> Vec<ProviderEvent> {
    let mut events = Vec::new();
    for block in message_content(event) {
        match block.get("type").and_then(|v| v.as_str()) {
            Some("text") => {
                let text = block
                    .get("text")
                    .and_then(|v| v.as_str())
                    .unwrap_or_default();
                if !text.is_empty() {
                    events.push(ProviderEvent::TextDelta {
                        text: text.to_string(),
                    });
                }
            }
            Some("tool_use") => {
                let tool_id = block.get("id").and_then(|v| v.as_str()).unwrap_or_default();
                if tool_id.is_empty() {
                    continue;
                }
                let input = block
                    .get("input")
                    .map(|input| input.to_string())
                    .unwrap_or_default();
                let (input_excerpt, input_truncated) = truncate_excerpt(&input);
                events.push(ProviderEvent::ToolStarted {
                    tool_id: tool_id.to_string(),
                    name: block
                        .get("name")
                        .and_then(|v| v.as_str())
                        .unwrap_or_default()
                        .to_string(),
                    input_excerpt,
                    input_truncated,
                });
            }
            _ => {}
        }
    }
    events
}

/// claude-code wraps `tool_result` blocks inside a `type: "user"` message.
fn parse_user_blocks(event: &serde_json::Value) -> Vec<ProviderEvent> {
    let mut events = Vec::new();
    for block in message_content(event) {
        if block.get("type").and_then(|v| v.as_str()) != Some("tool_result") {
            continue;
        }
        let tool_id = block
            .get("tool_use_id")
            .and_then(|v| v.as_str())
            .unwrap_or_default();
        if tool_id.is_empty() {
            continue;
        }
        let output = block.get("content").map(flatten_tool_result_content);
        let (output_excerpt, output_truncated) = truncate_excerpt(&output.unwrap_or_default());
        events.push(ProviderEvent::ToolCompleted {
            tool_id: tool_id.to_string(),
            // Written by the claude-code process, never parsed out of model
            // prose. Absent means the tool succeeded.
            is_error: block
                .get("is_error")
                .and_then(|v| v.as_bool())
                .unwrap_or(false),
            output_excerpt,
            output_truncated,
        });
    }
    events
}

/// `tool_result.content` is either a bare string or an array of blocks.
fn flatten_tool_result_content(content: &serde_json::Value) -> String {
    match content {
        serde_json::Value::String(text) => text.clone(),
        serde_json::Value::Array(blocks) => blocks
            .iter()
            .map(|block| match block.get("text").and_then(|v| v.as_str()) {
                Some(text) => text.to_string(),
                None => block.to_string(),
            })
            .collect::<Vec<_>>()
            .join("\n"),
        other => other.to_string(),
    }
}
