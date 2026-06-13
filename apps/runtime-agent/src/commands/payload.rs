use anyhow::{Context, Result};
use serde::{Deserialize, Deserializer, Serialize};

use crate::controlplane::models::{RuntimeCommand, RuntimeCommandType};

fn null_as_empty_vec<'de, D>(d: D) -> std::result::Result<Vec<serde_json::Value>, D::Error>
where
    D: Deserializer<'de>,
{
    Ok(Option::<Vec<serde_json::Value>>::deserialize(d)?.unwrap_or_default())
}

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
pub struct RuntimeWorkspaceFilePayload {
    pub file_id: String,
    pub revision_id: String,
    pub path: String,
    pub file_role: String,
    pub mime_type: String,
    pub sync_policy: String,
    pub content_hash: String,
    pub size_bytes: i32,
    pub storage_backend: String,
    #[serde(default)]
    pub content_text: Option<String>,
    #[serde(default)]
    pub object_key: Option<String>,
    #[serde(default = "default_metadata")]
    pub metadata: serde_json::Value,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeSkillPayload {
    pub skill_id: String,
    pub skill_key: String,
    #[serde(default)]
    pub revision_id: Option<String>,
    #[serde(default)]
    pub files: Vec<serde_json::Value>,
    #[serde(default)]
    pub content_hash: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeMCPServerPayload {
    pub server_id: String,
    pub server_key: String,
    pub transport: String,
    #[serde(default)]
    pub config_ref: Option<String>,
    #[serde(default = "default_metadata")]
    pub permission_scope: serde_json::Value,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeProvisionInstanceCommandPayload {
    pub command_id: String,
    pub tenant_id: String,
    pub team_id: String,
    pub digital_employee_id: String,
    pub execution_instance_id: String,
    pub runtime_node_id: String,
    pub provider_type: String,
    pub agent_home_dir: String,
    #[serde(default)]
    pub workspace_files: Vec<RuntimeWorkspaceFilePayload>,
    #[serde(default)]
    pub skills: Vec<RuntimeSkillPayload>,
    #[serde(default)]
    pub mcp_servers: Vec<RuntimeMCPServerPayload>,
}

impl RuntimeProvisionInstanceCommandPayload {
    pub fn from_command(command: &RuntimeCommand) -> Result<Self> {
        if !matches!(
            command.command_type,
            RuntimeCommandType::ProvisionInstance | RuntimeCommandType::SyncWorkspaceFiles
        ) {
            anyhow::bail!(
                "runtime command type is not a workspace materialization operation: {:?}",
                command.command_type
            );
        }

        if !command.payload.is_object() {
            anyhow::bail!("runtime command payload must be an object");
        }

        let payload: Self = serde_json::from_value(command.payload.clone())
            .context("invalid runtime provision instance command payload")?;
        payload.validate(command)?;
        Ok(payload)
    }

    fn validate(&self, command: &RuntimeCommand) -> Result<()> {
        if self.command_id != command.id {
            anyhow::bail!("command_id does not match runtime command id");
        }

        require_uuid_like("tenant_id", &self.tenant_id)?;
        require_uuid_like("team_id", &self.team_id)?;
        require_uuid_like("digital_employee_id", &self.digital_employee_id)?;
        require_uuid_like("execution_instance_id", &self.execution_instance_id)?;
        require_uuid_like("runtime_node_id", &self.runtime_node_id)?;

        if self.provider_type.trim().is_empty() {
            anyhow::bail!("provider_type is required");
        }

        if self.agent_home_dir.trim().is_empty() {
            anyhow::bail!("agent_home_dir is required");
        }

        for file in &self.workspace_files {
            require_uuid_like("workspace_files.file_id", &file.file_id)?;
            require_uuid_like("workspace_files.revision_id", &file.revision_id)?;
            if file.storage_backend == "db" && file.content_text.is_none() {
                anyhow::bail!("content_text is required for db-backed workspace files");
            }
        }

        Ok(())
    }
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeSessionCommandPayload {
    pub command_id: String,
    #[serde(default)]
    pub tenant_id: Option<String>,
    #[serde(default)]
    pub team_id: Option<String>,
    pub digital_employee_id: String,
    pub execution_instance_id: String,
    #[serde(default)]
    pub runtime_node_id: Option<String>,
    pub provider_type: String,
    #[serde(default)]
    pub agent_home_dir: Option<String>,
    #[serde(default)]
    pub workspace_files: Vec<RuntimeWorkspaceFilePayload>,
    #[serde(default)]
    pub skills: Vec<RuntimeSkillPayload>,
    #[serde(default)]
    pub mcp_servers: Vec<RuntimeMCPServerPayload>,
    pub session_policy: RuntimeSessionPolicy,
    pub prompt: Option<String>,
    pub input: Option<String>,
    #[serde(default, deserialize_with = "null_as_empty_vec")]
    pub context_refs: Vec<serde_json::Value>,
    #[serde(default, deserialize_with = "null_as_empty_vec")]
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
        match self.provider_type.as_str() {
            "claude-code" => "claude",
            "opencode" => "opencode",
            _ => "unsupported",
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

        if !matches!(command.command_type, RuntimeCommandType::StopSession) {
            require_optional_uuid_like("tenant_id", &self.tenant_id)?;
            require_optional_uuid_like("team_id", &self.team_id)?;
            require_optional_uuid_like("runtime_node_id", &self.runtime_node_id)?;
            if self
                .agent_home_dir
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .is_none()
            {
                anyhow::bail!("agent_home_dir is required");
            }
        }

        if self.provider_kind() == "unsupported" {
            anyhow::bail!("unsupported provider_type: {}", self.provider_type);
        }

        if self.session_policy.mode == SessionPolicyMode::Resume {
            if !self.has_provider_session_id() {
                anyhow::bail!("provider_session_id is required for resume");
            }
        }

        if matches!(command.command_type, RuntimeCommandType::ResumeSession)
            && !self.has_provider_session_id()
        {
            anyhow::bail!("provider_session_id is required for resume_session");
        }

        if !matches!(command.command_type, RuntimeCommandType::StopSession)
            && self.provider_prompt().is_none()
        {
            anyhow::bail!("prompt or input is required");
        }

        Ok(())
    }

    fn has_provider_session_id(&self) -> bool {
        self.session_policy
            .provider_session_id
            .as_deref()
            .map(str::trim)
            .is_some_and(|value| !value.is_empty())
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

fn require_optional_uuid_like(field: &str, value: &Option<String>) -> Result<()> {
    match value
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
    {
        Some(value) => require_uuid_like(field, value),
        None => anyhow::bail!("{field} is required"),
    }
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

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn test_null_refs_deserialize() {
        let raw = r#"{"command_id":"cmd-test","digital_employee_id":"35a3799b-7665-4913-9097-35ee53d30e74","execution_instance_id":"8e64dd8c-d70d-417d-b8bf-fe57a61f4205","provider_type":"claude-code","session_policy":{"mode":"new"},"prompt":"hello","input":"hello","context_refs":null,"artifact_refs":null,"metadata":{}}"#;
        let v: serde_json::Value = serde_json::from_str(raw).unwrap();
        let result = serde_json::from_value::<RuntimeSessionCommandPayload>(v);
        assert!(result.is_ok(), "expected ok, got: {:?}", result.err());
    }
}
