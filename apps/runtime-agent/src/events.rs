use serde::{Deserialize, Serialize};

/// Ledger rows are hot and broadcast over WS; excerpts are capped so a single
/// `Write` tool call carrying a whole file cannot blow up the row.
pub const EXCERPT_LIMIT_BYTES: usize = 4096;

const TRUNCATION_SUFFIX: &str = "…[truncated]";

/// Returns the excerpt and whether truncation occurred. Never splits a UTF-8
/// character.
pub fn truncate_excerpt(value: &str) -> (String, bool) {
    if value.len() <= EXCERPT_LIMIT_BYTES {
        return (value.to_string(), false);
    }
    let mut end = EXCERPT_LIMIT_BYTES;
    while end > 0 && !value.is_char_boundary(end) {
        end -= 1;
    }
    let mut excerpt = value[..end].to_string();
    excerpt.push_str(TRUNCATION_SUFFIX);
    (excerpt, true)
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct TurnUsage {
    pub total_tokens: i64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub input_tokens: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub output_tokens: Option<i64>,
}

/// Structured provider error (L3 ErrorEnvelope). Family/retryable must be
/// produced by `providers::error_map::map_code`, never re-derived from message
/// on the main path.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ErrorEnvelope {
    pub schema_version: String,
    pub code: String,
    pub family: String,
    pub retryable: bool,
    pub message: String,
    pub provider_type: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub native: Option<ErrorNative>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub evidence_refs: Option<Vec<ErrorEvidenceRef>>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ErrorNative {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub r#type: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub exit_code: Option<i32>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub excerpt: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub subtype: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ErrorEvidenceRef {
    pub r#type: String,
    #[serde(rename = "ref")]
    pub reference: String,
}

// L3 ProviderResult 没有 Rust 模型：Runtime 把终态语义投影到终态 writeback 的
// status / error_code / error_family / diagnostic 字段，不再额外传一个结果对象。
// 曾经存在的 ProviderResult 结构体每次都被构造后丢弃（`let _ =`），是纯装饰，
// 已删除；`contracts/provider/schemas/provider-result.schema.json` 与其 fixtures
// 保留为对外契约描述，是否让它真正上行见 spec §6 与后续立项。

/// Stream/attempt diagnostics carried on the terminal writeback (spec §4.4 / §6.1).
/// Always populated on terminal paths when counts are known; unmapped is
/// counted even when L2 `native_unmapped` writeback is off.
pub fn attempt_stream_diagnostics(
    provider_type: &str,
    unmapped_native_count: u64,
) -> serde_json::Value {
    serde_json::json!({
        "provider_type": provider_type,
        "unmapped_native_count": unmapped_native_count,
        "event_counts": {
            "native_unmapped": unmapped_native_count,
        },
    })
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(tag = "type", rename_all = "snake_case")]
pub enum ProviderEvent {
    SessionStarted {
        session_id: String,
        #[serde(skip_serializing_if = "Option::is_none")]
        session_state: Option<serde_json::Value>,
    },
    TurnStarted,
    TextDelta {
        text: String,
    },
    ToolStarted {
        tool_id: String,
        name: String,
        input_excerpt: String,
        #[serde(default)]
        input_truncated: bool,
    },
    ToolCompleted {
        tool_id: String,
        is_error: bool,
        output_excerpt: String,
        #[serde(default)]
        output_truncated: bool,
    },
    TurnCompleted {
        #[serde(skip_serializing_if = "Option::is_none")]
        summary: Option<String>,
        #[serde(skip_serializing_if = "Option::is_none")]
        usage: Option<TurnUsage>,
    },
    TurnError {
        message: String,
        /// Structured error when available (Phase 1+). Flat `message` remains
        /// for compatibility; when both are present they share the same text.
        #[serde(default, skip_serializing_if = "Option::is_none")]
        error: Option<ErrorEnvelope>,
    },
    /// Optional observability for unknown native types (Phase 3). Emission to
    /// Control Plane is gated by SUPERTEAM_PROVIDER_EMIT_NATIVE_UNMAPPED;
    /// parsers always produce these so goldens and diagnostics stay testable.
    NativeUnmapped {
        #[serde(default, skip_serializing_if = "Option::is_none")]
        native_type: Option<String>,
        reason: String,
    },
}

impl ProviderEvent {
    pub fn turn_error(message: impl Into<String>, error: Option<ErrorEnvelope>) -> Self {
        let message = message.into();
        Self::TurnError { message, error }
    }

    pub fn turn_error_from_envelope(envelope: ErrorEnvelope) -> Self {
        Self::TurnError {
            message: envelope.message.clone(),
            error: Some(envelope),
        }
    }

    pub fn native_unmapped(
        native_type: Option<String>,
        reason: impl Into<String>,
    ) -> Self {
        Self::NativeUnmapped {
            native_type,
            reason: reason.into(),
        }
    }

    pub fn is_native_unmapped(&self) -> bool {
        matches!(self, Self::NativeUnmapped { .. })
    }
}
