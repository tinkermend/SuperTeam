use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Node status enum matching Go NodeStatus
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum NodeStatus {
    Online,
    Offline,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum EnrollmentStatus {
    Pending,
    Approved,
    Rejected,
    Revoked,
}

#[derive(Debug, Clone, Serialize)]
pub struct EnrollHelloRequest {
    pub node_id: String,
    pub bootstrap_key: String,
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub version: Option<String>,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub supported_providers: Vec<String>,
    pub max_slots: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<HashMap<String, serde_json::Value>>,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub capabilities: Vec<RuntimeCapabilityInput>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct EnrollHelloResponse {
    pub enrollment: RuntimeEnrollmentResponse,
    #[serde(default)]
    pub session: Option<RuntimeSessionResponse>,
    #[serde(default)]
    pub session_token: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RuntimeEnrollmentResponse {
    pub id: String,
    pub tenant_id: String,
    #[serde(default)]
    pub runtime_node_id: Option<String>,
    pub node_id: String,
    pub bootstrap_key_id: String,
    pub status: EnrollmentStatus,
    #[serde(default)]
    pub request_payload: Option<HashMap<String, serde_json::Value>>,
    #[serde(default)]
    pub approved_by: Option<String>,
    #[serde(default)]
    pub approved_at: Option<String>,
    #[serde(default)]
    pub rejected_by: Option<String>,
    #[serde(default)]
    pub rejected_at: Option<String>,
    #[serde(default)]
    pub reject_reason: Option<String>,
    #[serde(default)]
    pub revoked_by: Option<String>,
    #[serde(default)]
    pub revoked_at: Option<String>,
    #[serde(default)]
    pub revoke_reason: Option<String>,
    #[serde(default)]
    pub last_hello_at: Option<String>,
    #[serde(default)]
    pub created_at: Option<String>,
    #[serde(default)]
    pub updated_at: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct RuntimeSessionResponse {
    pub id: String,
    pub tenant_id: String,
    pub runtime_node_id: String,
    #[serde(default)]
    pub node_id: Option<String>,
    #[serde(default)]
    pub enrollment_id: Option<String>,
    pub expires_at: String,
    pub last_seen_at: String,
    #[serde(default)]
    pub revoked_at: Option<String>,
    #[serde(default)]
    pub revoked_reason: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeCapabilitiesRequest {
    pub capabilities: Vec<RuntimeCapabilityInput>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RuntimeCapabilityInput {
    pub capability_type: String,
    pub capability_key: String,
    pub provider_type: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_version: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub binary_path: Option<String>,
    pub available: bool,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub workspace_base_dir: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub capacity: Option<HashMap<String, serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub labels: Option<HashMap<String, serde_json::Value>>,
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub details: Option<HashMap<String, serde_json::Value>>,
    pub health_status: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub metadata: Option<HashMap<String, serde_json::Value>>,
}

pub type RuntimeProviderCapability = RuntimeCapabilityInput;
pub type RuntimeWorkspaceCapability = RuntimeCapabilityInput;
pub type RuntimeCapacityCapability = RuntimeCapabilityInput;

#[derive(Debug, Clone, Deserialize)]
pub struct RuntimeCommand {
    pub id: String,
    #[serde(rename = "type")]
    pub command_type: RuntimeCommandType,
    pub payload: serde_json::Value,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum RuntimeCommandType {
    EnsureInstance,
    StartSession,
    ResumeSession,
    SendInput,
    StopSession,
    Unsupported(String),
}

#[derive(Debug, Clone, Deserialize)]
pub struct EnsureInstanceCommand {
    #[serde(default)]
    pub team_id: Option<String>,
    pub digital_employee_id: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeCommandTerminalWriteback {
    pub status: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub summary: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub result: Option<HashMap<String, serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub diagnostic: Option<HashMap<String, serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_session_external_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub session_state_patch: Option<HashMap<String, serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub log_ref: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub raw_result_ref: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub error_message: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub error_code: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub error_family: Option<String>,
}

#[derive(Debug, Clone, Serialize)]
pub struct RuntimeCommandEventWriteback {
    pub event_type: String,
    pub sequence_number: i32,
    pub payload: HashMap<String, serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_session_external_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub session_state_patch: Option<HashMap<String, serde_json::Value>>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub metadata: Option<HashMap<String, serde_json::Value>>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectTaskStartWriteback {
    pub project_task_id: String,
    pub lease_token: String,
    pub runtime_node_id: String,
    pub idempotency_key: String,
    /// 执行该 attempt 的 runtime 命令 id;控制平面用它回填 dispatch 冲突
    /// 路径遗留的 NULL run 关联(否则 provider 事件静默不进 ledger)。
    pub command_id: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_session_id: Option<String>,
}

#[derive(Debug, Clone, Serialize, Default)]
pub struct TaskResultContract {
    pub status: String,
    pub summary: String,
    #[serde(default)]
    pub acceptance_results: Vec<serde_json::Value>,
    #[serde(default)]
    pub evidence_refs: Vec<serde_json::Value>,
    #[serde(default)]
    pub artifact_refs: Vec<serde_json::Value>,
    #[serde(default)]
    pub changes_made: Vec<serde_json::Value>,
    #[serde(default)]
    pub deliverables: Vec<serde_json::Value>,
    #[serde(default)]
    pub verification: Vec<serde_json::Value>,
    #[serde(default)]
    pub risks: Vec<serde_json::Value>,
    #[serde(default)]
    pub follow_up_requests: Vec<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub human_review_request: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub revision_request: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub blocker: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub failure: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub replan_request: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub cancellation: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectTaskCompleteWriteback {
    pub project_task_id: String,
    pub lease_token: String,
    pub runtime_node_id: String,
    pub idempotency_key: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_session_id: Option<String>,
    pub conclusion: String,
    pub evidence_refs: Vec<serde_json::Value>,
    pub artifact_refs: Vec<serde_json::Value>,
    pub confidence_factors: HashMap<String, serde_json::Value>,
    pub uncertainty: String,
    pub missing_information: Vec<serde_json::Value>,
    pub recommended_next_action: String,
    pub requires_human_review: bool,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub result_contract: Option<TaskResultContract>,
    /// Pointer to the unparsed provider transcript uploaded by this runtime.
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub raw_log: Option<crate::raw_log::RawLogSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectTaskFailWriteback {
    pub project_task_id: String,
    pub lease_token: String,
    pub runtime_node_id: String,
    pub idempotency_key: String,
    pub failure_summary: String,
    pub failure_family: String,
    pub retryable: bool,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub result_contract: Option<TaskResultContract>,
    /// Pointer to the unparsed provider transcript uploaded by this runtime.
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub raw_log: Option<crate::raw_log::RawLogSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectTaskWaitHumanWriteback {
    pub project_task_id: String,
    pub lease_token: String,
    pub runtime_node_id: String,
    pub idempotency_key: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_session_id: Option<String>,
    pub digital_employee_id: String,
    pub reason: String,
    pub summary: String,
    pub missing_context_refs: Vec<serde_json::Value>,
    pub suggested_resolution_options: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub result_contract: Option<TaskResultContract>,
    /// Pointer to the unparsed provider transcript uploaded by this runtime.
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub raw_log: Option<crate::raw_log::RawLogSummary>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectTaskAttestationWriteback {
    pub project_id: String,
    pub project_task_id: String,
    pub attempt_id: String,
    pub runtime_node_id: String,
    pub digital_employee_id: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub capability_manifest_version: Option<String>,
    pub provider_auth_mode: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_session_id: Option<String>,
    pub attestation_type: String,
    pub status: String,
    pub command_argv: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub exit_code: Option<i32>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub duration_ms: Option<i64>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub log_ref: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub stdout_sha256: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub stderr_sha256: Option<String>,
    #[serde(default)]
    pub artifact_refs: Vec<serde_json::Value>,
    #[serde(default)]
    pub artifact_hashes: serde_json::Value,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub git_branch: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub git_base_ref: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub git_head_sha: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub git_diff_sha256: Option<String>,
    #[serde(default)]
    pub metadata: serde_json::Value,
    pub idempotency_key: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectTaskBudgetHeartbeatWriteback {
    pub project_id: String,
    pub project_task_id: String,
    pub consumed_wall_clock_sec: i32,
    pub consumed_tokens: i32,
}

#[derive(Debug, Clone, Deserialize)]
pub struct ProjectTaskBudgetHeartbeatResponse {
    pub tripped: bool,
    #[serde(default)]
    pub trip_reason: Option<String>,
}

impl<'de> Deserialize<'de> for RuntimeCommandType {
    fn deserialize<D>(deserializer: D) -> Result<Self, D::Error>
    where
        D: serde::Deserializer<'de>,
    {
        let value = String::deserialize(deserializer)?;
        Ok(match value.as_str() {
            "ensure_instance" => Self::EnsureInstance,
            "start_session" => Self::StartSession,
            "resume_session" => Self::ResumeSession,
            "send_input" => Self::SendInput,
            "stop_session" => Self::StopSession,
            _ => Self::Unsupported(value),
        })
    }
}

#[derive(Debug, Clone, Deserialize)]
pub struct RuntimeCapabilityResponse {
    pub id: String,
    pub tenant_id: String,
    pub runtime_node_id: String,
    pub capability_type: String,
    pub capability_key: String,
    pub provider_type: String,
    pub available: bool,
    pub status: String,
    pub health_status: String,
    #[serde(default)]
    pub last_seen_at: Option<String>,
    #[serde(default)]
    pub created_at: Option<String>,
    #[serde(default)]
    pub updated_at: Option<String>,
}

/// Register node request
#[derive(Debug, Clone, Serialize)]
pub struct RegisterNodeRequest {
    pub node_id: String,
    pub name: String,
    pub supported_providers: Vec<String>,
    pub max_slots: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<HashMap<String, serde_json::Value>>,
}

/// Register node response
#[derive(Debug, Clone, Deserialize)]
pub struct RegisterNodeResponse {
    pub node_id: String,
    pub name: String,
    pub supported_providers: Vec<String>,
    pub max_slots: i32,
    pub current_load: i32,
    pub status: NodeStatus,
    pub metadata: Option<HashMap<String, serde_json::Value>>,
    pub last_heartbeat_at: Option<String>,
    pub created_at: Option<String>,
    pub updated_at: Option<String>,
}

/// Heartbeat request
#[derive(Debug, Clone, Serialize)]
pub struct HeartbeatRequest {
    pub current_load: i32,
    pub status: NodeStatus,
    /// 能力自报(平台限额下发 P2 spec §5):声明本 agent 消费心跳携带的
    /// platform_limits 快照,控制平面据此解除工件上限的版本偏斜 clamp。
    pub supports_platform_limits: bool,
}

/// 平台限额快照(P2 spec §3),控制平面经心跳响应下发。
/// 字段全部可选:CP 老版本不发时保持本地默认值,任何一侧缺失都不破坏现状。
#[derive(Debug, Clone, Default, Deserialize, PartialEq, Eq)]
pub struct PlatformLimits {
    #[serde(default)]
    pub version: Option<String>,
    #[serde(default)]
    pub artifact_max_file_size_bytes: Option<u64>,
    #[serde(default)]
    pub attachment_max_file_size_bytes: Option<u64>,
    #[serde(default)]
    pub attachment_max_count: Option<u64>,
    #[serde(default)]
    pub attachment_total_max_bytes: Option<u64>,
    #[serde(default)]
    pub skill_archive_max_bytes: Option<u64>,
    #[serde(default)]
    pub skill_archive_max_file_count: Option<u64>,
}

/// Heartbeat response
#[derive(Debug, Clone, Deserialize)]
pub struct HeartbeatResponse {
    pub node_id: String,
    pub name: String,
    pub supported_providers: Vec<String>,
    #[serde(default)]
    pub required_tools: Vec<String>,
    pub max_slots: i32,
    pub current_load: i32,
    pub status: NodeStatus,
    pub metadata: Option<HashMap<String, serde_json::Value>>,
    pub last_heartbeat_at: Option<String>,
    pub created_at: Option<String>,
    pub updated_at: Option<String>,
    /// 平台限额快照;CP 老版本不携带该字段(serde default → None)。
    #[serde(default)]
    pub platform_limits: Option<PlatformLimits>,
}

/// 证据地基(spec 2026-07-09 §8 修订 1):runtime 零对象存储凭证,
/// 所有对象直传/直取都先向控制平面换取 presigned URL。

/// Presign request for a content-addressed artifact upload.
#[derive(Debug, Clone, Serialize)]
pub struct PresignArtifactUploadRequest {
    pub sha256: String,
    pub size_bytes: i64,
    pub content_type: String,
}

/// Presign request for a raw transcript segment or manifest upload.
#[derive(Debug, Clone, Serialize)]
pub struct PresignRawLogUploadRequest {
    pub attempt_id: String,
    /// "part" or "manifest".
    pub object: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub part_index: Option<i32>,
    pub size_bytes: i64,
}

/// Presigned upload target returned by the control plane.
#[derive(Debug, Clone, Deserialize)]
pub struct PresignUploadResponse {
    pub object_key: String,
    #[serde(default)]
    pub upload_url: Option<String>,
    #[serde(default)]
    pub expires_at: Option<String>,
    #[serde(default)]
    pub already_exists: bool,
}

/// Presign request for downloading a skill archive.
#[derive(Debug, Clone, Serialize)]
pub struct PresignSkillArchiveDownloadRequest {
    pub archive_object_ref: String,
}

/// Presigned download target returned by the control plane.
#[derive(Debug, Clone, Deserialize)]
pub struct PresignDownloadResponse {
    pub download_url: String,
    #[serde(default)]
    pub expires_at: Option<String>,
}
