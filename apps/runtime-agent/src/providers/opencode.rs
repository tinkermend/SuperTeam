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
        // opencode 1.17.x `run --format json` 实测事件流(GATE smoke 现场抓取):
        // step_start / text / step_finish,会话 id 在顶层 sessionID,正文与
        // tokens 嵌在 part 里。旧的 session.updated/message.part.updated 分支
        // 保留以兼容其他版本。
        "step_start" => {
            let session_id = event
                .get("sessionID")
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
        "text" => {
            let text = event
                .get("part")
                .and_then(|part| part.get("text"))
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
        "step_finish" => {
            let usage = event
                .get("part")
                .and_then(|part| part.get("tokens"))
                .and_then(|tokens| {
                    let total = tokens.get("total").and_then(|v| v.as_i64())?;
                    Some(crate::events::TurnUsage {
                        total_tokens: total,
                        input_tokens: tokens.get("input").and_then(|v| v.as_i64()),
                        output_tokens: tokens.get("output").and_then(|v| v.as_i64()),
                    })
                });
            Ok(vec![ProviderEvent::TurnCompleted {
                summary: None,
                usage,
            }])
        }
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

#[cfg(test)]
mod tests {
    use super::*;

    // 三条样本均为 opencode 1.17.16 `run --format json` 真实输出
    // (2026-07-17 GATE smoke 现场抓取,仅截短无关字段)。
    #[test]
    fn parses_real_step_start_as_session_started() {
        let line = r#"{"type":"step_start","timestamp":1784276621339,"sessionID":"ses_090d3c9a1ffexFN3DtImBH4e0M","part":{"id":"prt_1","type":"step-start"}}"#;
        let events = parse_opencode_event(line).unwrap();
        assert_eq!(
            events,
            vec![ProviderEvent::SessionStarted {
                session_id: "ses_090d3c9a1ffexFN3DtImBH4e0M".to_string(),
                session_state: None,
            }]
        );
    }

    #[test]
    fn parses_real_text_part_as_text_delta() {
        let line = r#"{"type":"text","timestamp":1784276621584,"sessionID":"ses_1","part":{"id":"prt_2","type":"text","text":"ok"}}"#;
        let events = parse_opencode_event(line).unwrap();
        assert_eq!(
            events,
            vec![ProviderEvent::TextDelta {
                text: "ok".to_string()
            }]
        );
    }

    #[test]
    fn parses_real_step_finish_as_turn_completed_with_usage() {
        let line = r#"{"type":"step_finish","timestamp":1784276621631,"sessionID":"ses_1","part":{"id":"prt_3","reason":"stop","type":"step-finish","tokens":{"total":10848,"input":10717,"output":3,"reasoning":0,"cache":{"write":0,"read":128}},"cost":0.015}}"#;
        let events = parse_opencode_event(line).unwrap();
        match &events[..] {
            [ProviderEvent::TurnCompleted { usage: Some(usage), .. }] => {
                assert_eq!(usage.total_tokens, 10848);
                assert_eq!(usage.input_tokens, Some(10717));
                assert_eq!(usage.output_tokens, Some(3));
            }
            other => panic!("expected turn completed with usage, got {other:?}"),
        }
    }

    #[test]
    fn legacy_event_names_still_parse() {
        let line = r#"{"type":"session.updated","sessionID":"ses_legacy"}"#;
        let events = parse_opencode_event(line).unwrap();
        assert_eq!(events.len(), 1);
    }
}
