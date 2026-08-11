use anyhow::{Context, Result};
use serde::{Deserialize, Deserializer, Serialize};

use crate::controlplane::models::{RuntimeCommand, RuntimeCommandType};
use crate::providers::catalog;

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
pub struct RuntimeSkillPayload {
    pub skill_id: String,
    pub skill_key: String,
    #[serde(default)]
    pub revision_id: Option<String>,
    pub archive_object_ref: String,
    pub archive_checksum_sha256: String,
    pub archive_size_bytes: i64,
    pub archive_file_count: i64,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeMCPServerPayload {
    pub server_id: String,
    pub server_key: String,
    #[serde(default)]
    pub name: Option<String>,
    pub transport: String,
    #[serde(default)]
    pub url: Option<String>,
    #[serde(default)]
    pub auth_strategy: Option<String>,
    #[serde(default)]
    pub credential_env_var: Option<String>,
    #[serde(default)]
    pub required_env_vars: Vec<String>,
    #[serde(default)]
    pub headers_env: std::collections::BTreeMap<String, String>,
    #[serde(default)]
    pub source_scope: Option<String>,
    #[serde(default)]
    pub config_ref: Option<String>,
    #[serde(default = "default_metadata")]
    pub permission_scope: serde_json::Value,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct RuntimeEnvironmentVariablePayload {
    pub name: String,
    pub value: String,
    #[serde(default)]
    pub sensitive: bool,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize, Default)]
pub struct RuntimeProjectGitPayload {
    pub url: String,
    #[serde(default)]
    pub default_branch: Option<String>,
    #[serde(default)]
    pub git_credential_ref: Option<String>,
    #[serde(default)]
    pub scope: Vec<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct RuntimeProjectWorkspacePayload {
    pub project_id: Option<String>,
    pub project_task_id: Option<String>,
    pub project_task_attempt_id: Option<String>,
    /// Chat dispatch only (目录与能力投影修订 spec §4): keys the chat working
    /// directory by (project, thread) instead of (project, task, attempt).
    pub chat_thread_id: Option<String>,
    /// 稳定项目目录名(spec 2026-07-23);有值时 CWD = `{base}/{project_name}`。
    pub project_name: Option<String>,
    pub workspace_mode: Option<String>,
    pub base_ref: Option<String>,
    pub project_git: Option<RuntimeProjectGitPayload>,
    pub capability_manifest_version: Option<String>,
    pub provider_auth_mode: String,
    pub attestation_policy: Option<serde_json::Value>,
    pub budget: Option<serde_json::Value>,
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
    pub persona_memory_markdown: Option<String>,
    /// 团队宪法（control-plane spec §5.3，D9 仅约束文本注入，不参与门禁判定）。
    /// 已由 CP 渲染成 markdown 文本块，这里只负责拼进 provider 实际收到的提示词。
    #[serde(default)]
    pub team_constitution: Option<String>,
    #[serde(default = "default_json_object")]
    pub capability_bindings: serde_json::Value,
    #[serde(default)]
    pub skills: Vec<RuntimeSkillPayload>,
    #[serde(default)]
    pub mcp_servers: Vec<RuntimeMCPServerPayload>,
    #[serde(default)]
    pub environment: Vec<RuntimeEnvironmentVariablePayload>,
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

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeStopSessionCommandPayload {
    pub command_id: String,
    #[serde(default)]
    pub start_command_id: Option<String>,
    #[serde(default)]
    pub run_id: Option<String>,
    #[serde(default)]
    pub task_id: Option<String>,
    #[serde(default)]
    pub reason: Option<String>,
    #[serde(default)]
    pub grace_sec: Option<i32>,
}

impl RuntimeStopSessionCommandPayload {
    pub fn from_command(command: &RuntimeCommand) -> Result<Self> {
        if !matches!(command.command_type, RuntimeCommandType::StopSession) {
            anyhow::bail!(
                "runtime command type is not stop_session: {:?}",
                command.command_type
            );
        }
        if !command.payload.is_object() {
            anyhow::bail!("runtime stop command payload must be an object");
        }
        let payload: Self = serde_json::from_value(command.payload.clone())
            .context("invalid runtime stop session command payload")?;
        if payload.command_id != command.id {
            anyhow::bail!("command_id does not match runtime command id");
        }
        if payload
            .start_command_id
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .is_none()
        {
            anyhow::bail!("start_command_id is required");
        }
        Ok(payload)
    }

    pub fn start_command_id(&self) -> &str {
        self.start_command_id
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .expect("validated start_command_id")
    }
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
        catalog::provider_kind(&self.provider_type)
    }

    /// provider 实际收到的提示词。
    ///
    /// 团队宪法与员工人格在这里拼进任务提示词之前。此前这两个字段虽然进了 payload，
    /// 但没有任何地方读它们——provider 只拿到 prompt/input，等于"存了不用"。顺序：
    /// 团队宪法（团队级约束，优先级最高）→ 员工人格 → 任务本身。
    pub fn provider_prompt(&self) -> Option<String> {
        let task = trimmed_text(&self.prompt).or_else(|| trimmed_text(&self.input))?;
        let mut sections = Vec::new();
        if let Some(constitution) = trimmed_text(&self.team_constitution) {
            sections.push(constitution);
        }
        if let Some(persona) = trimmed_text(&self.persona_memory_markdown) {
            sections.push(format!("# 数字员工人格与记忆\n{persona}"));
        }
        if sections.is_empty() {
            return Some(task);
        }
        sections.push(format!("# 本次任务\n{task}"));
        Some(sections.join("\n\n"))
    }

    /// 团队宪法，作为未来 system 级注入的取值入口。
    ///
    /// 当前**没有任何 provider 消费它**：宪法仍由 `provider_prompt()`（见上）
    /// 作为普通 user prompt 前置投递。二者目前**不互斥**——本访问器只是把宪法
    /// 单独取出，供 codex/opencode 原生 system-prompt flag 落地后各
    /// `build_command` 使用。届时需同步把宪法段从 `provider_prompt()` 移除、
    /// 改由 `ProviderRequest.system_prompt` 投递，避免双注入。
    pub fn system_prompt(&self) -> Option<String> {
        trimmed_text(&self.team_constitution)
    }

    pub fn project_workspace(&self) -> RuntimeProjectWorkspacePayload {
        RuntimeProjectWorkspacePayload {
            project_id: metadata_string(&self.metadata, "project_id"),
            project_task_id: metadata_string(&self.metadata, "project_task_id"),
            project_task_attempt_id: metadata_string(&self.metadata, "project_task_attempt_id"),
            chat_thread_id: metadata_string(&self.metadata, "chat_thread_id"),
            project_name: metadata_string(&self.metadata, "project_name"),
            workspace_mode: metadata_string(&self.metadata, "workspace_mode"),
            base_ref: metadata_string(&self.metadata, "base_ref"),
            project_git: project_git_metadata(&self.metadata),
            capability_manifest_version: metadata_string(
                &self.metadata,
                "capability_manifest_version",
            ),
            provider_auth_mode: metadata_string(&self.metadata, "provider_auth_mode")
                .unwrap_or_else(|| "host".to_string()),
            attestation_policy: metadata_object(&self.metadata, "attestation_policy"),
            budget: metadata_object(&self.metadata, "budget"),
        }
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

        for env in &self.environment {
            if !valid_env_name(&env.name) {
                anyhow::bail!("invalid environment variable name: {}", env.name);
            }
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

fn valid_env_name(name: &str) -> bool {
    let mut chars = name.chars();
    match chars.next() {
        Some(c) if c == '_' || c.is_ascii_alphabetic() => {}
        _ => return false,
    }
    chars.all(|c| c == '_' || c.is_ascii_alphanumeric())
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

pub(crate) fn metadata_string(metadata: &serde_json::Value, key: &str) -> Option<String> {
    metadata
        .get(key)
        .and_then(|value| value.as_str())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn project_git_metadata(metadata: &serde_json::Value) -> Option<RuntimeProjectGitPayload> {
    let project_git = metadata.get("project_git")?.as_object()?;
    let url = project_git
        .get("url")
        .and_then(|value| value.as_str())
        .map(str::trim)
        .filter(|value| !value.is_empty())?
        .to_string();
    let default_branch = project_git
        .get("default_branch")
        .and_then(|value| value.as_str())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string);
    let git_credential_ref = project_git
        .get("git_credential_ref")
        .and_then(|value| value.as_str())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string);
    let scope = project_git
        .get("scope")
        .and_then(|value| value.as_array())
        .map(|values| {
            values
                .iter()
                .filter_map(|value| {
                    value
                        .as_str()
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                        .map(ToString::to_string)
                })
                .collect()
        })
        .unwrap_or_default();

    Some(RuntimeProjectGitPayload {
        url,
        default_branch,
        git_credential_ref,
        scope,
    })
}

fn metadata_object(metadata: &serde_json::Value, key: &str) -> Option<serde_json::Value> {
    metadata.get(key).filter(|value| value.is_object()).cloned()
}

fn default_recoverable() -> bool {
    true
}

fn default_metadata() -> serde_json::Value {
    serde_json::json!({})
}

fn default_json_object() -> serde_json::Value {
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

    #[test]
    fn test_project_workspace_metadata_deserializes() {
        let raw = r#"{
            "command_id":"cmd-test",
            "tenant_id":"00000000-0000-4000-8000-000000000001",
            "team_id":"11111111-1111-4111-8111-111111111111",
            "digital_employee_id":"35a3799b-7665-4913-9097-35ee53d30e74",
            "execution_instance_id":"8e64dd8c-d70d-417d-b8bf-fe57a61f4205",
            "runtime_node_id":"44444444-4444-4444-8444-444444444444",
            "provider_type":"codex",
            "agent_home_dir":"/tmp/workspaces/employees/35a3799b-7665-4913-9097-35ee53d30e74",
            "workspace_files":[],
            "skills":[],
            "mcp_servers":[],
            "environment":[],
            "session_policy":{"mode":"new"},
            "prompt":"hello",
            "input":"hello",
            "context_refs":[],
            "artifact_refs":[],
            "metadata":{
                "workspace_mode":"branch",
                "base_ref":"main",
                "project_id":"11111111-1111-4111-8111-111111111111",
                "project_task_id":"22222222-2222-4222-8222-222222222222",
                "project_task_attempt_id":"33333333-3333-4333-8333-333333333333",
                "project_git":{
                    "url":"https://github.com/acme/app.git",
                    "default_branch":"main",
                    "git_credential_ref":"git-creds",
                    "scope":["apps/web"]
                }
            }
        }"#;
        let command = RuntimeCommand {
            id: "cmd-test".to_string(),
            command_type: RuntimeCommandType::StartSession,
            payload: serde_json::from_str(raw).unwrap(),
        };
        let payload = RuntimeSessionCommandPayload::from_command(&command).unwrap();
        let project_workspace = payload.project_workspace();

        assert_eq!(project_workspace.workspace_mode.as_deref(), Some("branch"));
        assert_eq!(project_workspace.base_ref.as_deref(), Some("main"));
        assert_eq!(
            project_workspace.project_id.as_deref(),
            Some("11111111-1111-4111-8111-111111111111")
        );
        assert_eq!(
            project_workspace.project_task_id.as_deref(),
            Some("22222222-2222-4222-8222-222222222222")
        );
        assert_eq!(
            project_workspace.project_task_attempt_id.as_deref(),
            Some("33333333-3333-4333-8333-333333333333")
        );
        let git = project_workspace.project_git.unwrap();
        assert_eq!(git.url, "https://github.com/acme/app.git");
        assert_eq!(git.default_branch.as_deref(), Some("main"));
        assert_eq!(git.git_credential_ref.as_deref(), Some("git-creds"));
        assert_eq!(git.scope, vec!["apps/web".to_string()]);
    }

    fn prompt_payload(
        constitution: Option<&str>,
        persona: Option<&str>,
        task: &str,
    ) -> RuntimeSessionCommandPayload {
        RuntimeSessionCommandPayload {
            command_id: "cmd-test".to_string(),
            tenant_id: Some("00000000-0000-4000-8000-000000000001".to_string()),
            team_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            digital_employee_id: "35a3799b-7665-4913-9097-35ee53d30e74".to_string(),
            execution_instance_id: "8e64dd8c-d70d-417d-b8bf-fe57a61f4205".to_string(),
            runtime_node_id: None,
            provider_type: "codex".to_string(),
            agent_home_dir: None,
            persona_memory_markdown: persona.map(ToString::to_string),
            team_constitution: constitution.map(ToString::to_string),
            capability_bindings: serde_json::json!({}),
            skills: Vec::new(),
            mcp_servers: Vec::new(),
            environment: Vec::new(),
            session_policy: RuntimeSessionPolicy {
                mode: SessionPolicyMode::New,
                provider_session_id: None,
                recoverable: true,
            },
            prompt: Some(task.to_string()),
            input: None,
            context_refs: Vec::new(),
            artifact_refs: Vec::new(),
            model: None,
            metadata: serde_json::json!({}),
        }
    }

    /// 团队宪法必须真的出现在 provider 收到的提示词里。此前 team_constitution 与
    /// persona_memory_markdown 都只进了 payload、没有任何读者，等于存了不用。
    #[test]
    fn provider_prompt_prepends_constitution_and_persona_before_task() {
        let payload = prompt_payload(
            Some("# 团队宪法（必须遵守）\n\n## 禁止\n- 不得直连生产库"),
            Some("我是数据库运维员工。"),
            "巡检慢查询",
        );

        let prompt = payload.provider_prompt().expect("prompt");

        assert!(prompt.contains("不得直连生产库"), "constitution missing: {prompt}");
        assert!(prompt.contains("我是数据库运维员工。"), "persona missing: {prompt}");
        assert!(prompt.contains("巡检慢查询"), "task missing: {prompt}");
        // 顺序：团队约束 → 员工人格 → 任务本身。
        let constitution_at = prompt.find("不得直连生产库").unwrap();
        let persona_at = prompt.find("我是数据库运维员工。").unwrap();
        let task_at = prompt.find("巡检慢查询").unwrap();
        assert!(constitution_at < persona_at && persona_at < task_at, "unexpected order: {prompt}");
    }

    /// 没有宪法与人格时提示词保持原样，不加任何包裹——避免给裸任务凭空套标题。
    #[test]
    fn provider_prompt_is_unchanged_without_constitution_or_persona() {
        let payload = prompt_payload(None, None, "巡检慢查询");
        assert_eq!(payload.provider_prompt().as_deref(), Some("巡检慢查询"));
    }

    /// 空白宪法等同于没有，不产生空的宪法段落。
    #[test]
    fn provider_prompt_ignores_blank_constitution() {
        let payload = prompt_payload(Some("   \n  "), None, "巡检慢查询");
        assert_eq!(payload.provider_prompt().as_deref(), Some("巡检慢查询"));
    }

    /// `system_prompt()` 是未来 system 级注入的取值入口：返回宪法原文。当前
    /// `provider_prompt()` 仍独立前置宪法（见上三例），二者暂不互斥——本访问器
    /// 只把宪法单独取出，待 codex/opencode 原生 system flag 落地后由各
    /// `build_command` 消费。见 `ProviderRequest::system_prompt` 的 dormant 注释。
    #[test]
    fn system_prompt_returns_constitution() {
        let payload = prompt_payload(
            Some("# 团队宪法（必须遵守）\n\n## 禁止\n- 不得直连生产库"),
            None,
            "巡检慢查询",
        );
        let system = payload.system_prompt().expect("constitution");
        assert!(system.contains("不得直连生产库"), "constitution missing: {system}");
    }

    #[test]
    fn system_prompt_none_when_absent() {
        let payload = prompt_payload(None, Some("人格"), "巡检慢查询");
        assert!(payload.system_prompt().is_none());
    }

    #[test]
    fn system_prompt_none_when_blank() {
        let payload = prompt_payload(Some("   \n  "), None, "巡检慢查询");
        assert!(payload.system_prompt().is_none());
    }

    #[test]
    fn test_project_workspace_project_git_metadata_normalizes_strings() {
        let payload = RuntimeSessionCommandPayload {
            command_id: "cmd-test".to_string(),
            tenant_id: Some("00000000-0000-4000-8000-000000000001".to_string()),
            team_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            digital_employee_id: "35a3799b-7665-4913-9097-35ee53d30e74".to_string(),
            execution_instance_id: "8e64dd8c-d70d-417d-b8bf-fe57a61f4205".to_string(),
            runtime_node_id: Some("44444444-4444-4444-8444-444444444444".to_string()),
            provider_type: "codex".to_string(),
            agent_home_dir: Some("/tmp/workspaces/employees/35a3799b".to_string()),
            persona_memory_markdown: None,
            team_constitution: None,
            capability_bindings: serde_json::json!({}),
            skills: Vec::new(),
            mcp_servers: Vec::new(),
            environment: Vec::new(),
            session_policy: RuntimeSessionPolicy {
                mode: SessionPolicyMode::New,
                provider_session_id: None,
                recoverable: true,
            },
            prompt: Some("hello".to_string()),
            input: Some("hello".to_string()),
            context_refs: Vec::new(),
            artifact_refs: Vec::new(),
            model: None,
            metadata: serde_json::json!({
                "project_git": {
                    "url": " https://github.com/acme/app.git ",
                    "default_branch": " main ",
                    "git_credential_ref": " git-creds ",
                    "scope": [" apps/web ", "", null, 7, " packages/api "]
                }
            }),
        };

        let git = payload.project_workspace().project_git.unwrap();

        assert_eq!(git.url, "https://github.com/acme/app.git");
        assert_eq!(git.default_branch.as_deref(), Some("main"));
        assert_eq!(git.git_credential_ref.as_deref(), Some("git-creds"));
        assert_eq!(
            git.scope,
            vec!["apps/web".to_string(), "packages/api".to_string()]
        );
    }

    #[test]
    fn test_project_workspace_omits_invalid_project_git_metadata() {
        for project_git in [
            serde_json::Value::Null,
            serde_json::json!("https://github.com/acme/app.git"),
            serde_json::json!(["https://github.com/acme/app.git"]),
            serde_json::json!({}),
            serde_json::json!({"url": null}),
            serde_json::json!({"url": 7}),
            serde_json::json!({"url": "   "}),
        ] {
            let payload = RuntimeSessionCommandPayload {
                command_id: "cmd-test".to_string(),
                tenant_id: Some("00000000-0000-4000-8000-000000000001".to_string()),
                team_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
                digital_employee_id: "35a3799b-7665-4913-9097-35ee53d30e74".to_string(),
                execution_instance_id: "8e64dd8c-d70d-417d-b8bf-fe57a61f4205".to_string(),
                runtime_node_id: Some("44444444-4444-4444-8444-444444444444".to_string()),
                provider_type: "codex".to_string(),
                agent_home_dir: Some("/tmp/workspaces/employees/35a3799b".to_string()),
                persona_memory_markdown: None,
                team_constitution: None,
                capability_bindings: serde_json::json!({}),
                skills: Vec::new(),
                mcp_servers: Vec::new(),
                environment: Vec::new(),
                session_policy: RuntimeSessionPolicy {
                    mode: SessionPolicyMode::New,
                    provider_session_id: None,
                    recoverable: true,
                },
                prompt: Some("hello".to_string()),
                input: Some("hello".to_string()),
                context_refs: Vec::new(),
                artifact_refs: Vec::new(),
                model: None,
                metadata: serde_json::json!({
                    "project_git": project_git,
                }),
            };

            assert_eq!(payload.project_workspace().project_git, None);
        }

        let missing_project_git = RuntimeSessionCommandPayload {
            command_id: "cmd-test".to_string(),
            tenant_id: Some("00000000-0000-4000-8000-000000000001".to_string()),
            team_id: Some("11111111-1111-4111-8111-111111111111".to_string()),
            digital_employee_id: "35a3799b-7665-4913-9097-35ee53d30e74".to_string(),
            execution_instance_id: "8e64dd8c-d70d-417d-b8bf-fe57a61f4205".to_string(),
            runtime_node_id: Some("44444444-4444-4444-8444-444444444444".to_string()),
            provider_type: "codex".to_string(),
            agent_home_dir: Some("/tmp/workspaces/employees/35a3799b".to_string()),
            persona_memory_markdown: None,
            team_constitution: None,
            capability_bindings: serde_json::json!({}),
            skills: Vec::new(),
            mcp_servers: Vec::new(),
            environment: Vec::new(),
            session_policy: RuntimeSessionPolicy {
                mode: SessionPolicyMode::New,
                provider_session_id: None,
                recoverable: true,
            },
            prompt: Some("hello".to_string()),
            input: Some("hello".to_string()),
            context_refs: Vec::new(),
            artifact_refs: Vec::new(),
            model: None,
            metadata: serde_json::json!({}),
        };

        assert_eq!(missing_project_git.project_workspace().project_git, None);
    }
}
