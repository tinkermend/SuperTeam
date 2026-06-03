use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

use crate::controlplane::models::{RuntimeCommand, RuntimeCommandType};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SessionPolicyMode {
    New,
    Resume,
    ReuseLatest,
    Ephemeral,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RuntimeSessionPolicy {
    pub mode: SessionPolicyMode,
    #[serde(default)]
    pub provider_session_id: Option<String>,
    #[serde(default = "default_recoverable")]
    pub recoverable: bool,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeSessionCommandPayload {
    pub command_id: String,
    pub digital_employee_id: String,
    pub execution_instance_id: String,
    pub provider_type: String,
    pub session_policy: RuntimeSessionPolicy,
    pub prompt: Option<String>,
    pub input: Option<String>,
    pub context_refs: Vec<serde_json::Value>,
    pub artifact_refs: Vec<serde_json::Value>,
    #[serde(default)]
    pub model: Option<String>,
    #[serde(default = "default_metadata")]
    pub metadata: serde_json::Value,
}

impl RuntimeSessionCommandPayload {
    pub fn from_command(command: &RuntimeCommand) -> Result<Self> {
        if !matches!(
            command.command_type,
            RuntimeCommandType::StartSession
                | RuntimeCommandType::ResumeSession
                | RuntimeCommandType::SendInput
                | RuntimeCommandType::StopSession
        ) {
            anyhow::bail!(
                "runtime command type is not a session operation: {:?}",
                command.command_type
            );
        }

        if !command.payload.is_object() {
            anyhow::bail!("runtime command payload must be an object");
        }

        for field in REQUIRED_FIELDS {
            require_field(&command.payload, field)?;
        }

        let payload: Self = serde_json::from_value(command.payload.clone())
            .context("invalid runtime session command payload")?;
        payload.validate(command)?;
        Ok(payload)
    }

    pub fn provider_kind(&self) -> &'static str {
        let provider_type = self.provider_type.trim();
        if provider_type.eq_ignore_ascii_case("claude-code")
            || provider_type.eq_ignore_ascii_case("claude")
        {
            "claude"
        } else if provider_type.eq_ignore_ascii_case("opencode") {
            "opencode"
        } else {
            "unsupported"
        }
    }

    pub fn provider_prompt(&self) -> Option<String> {
        trimmed_text(&self.prompt).or_else(|| trimmed_text(&self.input))
    }

    fn validate(&self, command: &RuntimeCommand) -> Result<()> {
        if self.command_id != command.id {
            anyhow::bail!("command_id does not match runtime command id");
        }

        require_uuid_like("digital_employee_id", &self.digital_employee_id)?;
        require_uuid_like("execution_instance_id", &self.execution_instance_id)?;

        if self.provider_kind() == "unsupported" {
            anyhow::bail!("unsupported provider_type: {}", self.provider_type);
        }

        if self.session_policy.mode == SessionPolicyMode::Resume {
            let has_provider_session_id = self
                .session_policy
                .provider_session_id
                .as_deref()
                .map(str::trim)
                .is_some_and(|value| !value.is_empty());
            if !has_provider_session_id {
                anyhow::bail!("provider_session_id is required for resume");
            }
        }

        if !matches!(command.command_type, RuntimeCommandType::StopSession)
            && self.provider_prompt().is_none()
        {
            anyhow::bail!("prompt or input is required");
        }

        Ok(())
    }
}

const REQUIRED_FIELDS: &[&str] = &[
    "command_id",
    "digital_employee_id",
    "execution_instance_id",
    "provider_type",
    "session_policy",
    "prompt",
    "input",
    "context_refs",
    "artifact_refs",
];

fn require_field(payload: &serde_json::Value, field: &str) -> Result<()> {
    if payload.get(field).is_none() {
        anyhow::bail!("{field} is required");
    }
    Ok(())
}

fn require_uuid_like(field: &str, value: &str) -> Result<()> {
    if !is_uuid_like(value) {
        anyhow::bail!("{field} must be a UUID-like string");
    }
    Ok(())
}

fn is_uuid_like(value: &str) -> bool {
    let bytes = value.as_bytes();
    if bytes.len() != 36 {
        return false;
    }

    for (index, byte) in bytes.iter().enumerate() {
        match index {
            8 | 13 | 18 | 23 => {
                if *byte != b'-' {
                    return false;
                }
            }
            _ => {
                if !byte.is_ascii_hexdigit() {
                    return false;
                }
            }
        }
    }

    true
}

fn trimmed_text(value: &Option<String>) -> Option<String> {
    value
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn default_recoverable() -> bool {
    true
}

fn default_metadata() -> serde_json::Value {
    serde_json::json!({})
}
