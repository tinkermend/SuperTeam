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
    },
}
