use serde::{Deserialize, Serialize};

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
    },
    ToolCompleted {
        tool_id: String,
    },
    TurnCompleted {
        summary: Option<String>,
    },
    TurnError {
        message: String,
    },
}
