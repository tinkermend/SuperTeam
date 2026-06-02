use serde::{Deserialize, Serialize};
use std::collections::HashMap;

/// Task status enum matching Go TaskStatus
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub enum TaskStatus {
    Pending,
    Claimed,
    Running,
    Completed,
    Failed,
    Cancelled,
}

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
}

/// Heartbeat response
#[derive(Debug, Clone, Deserialize)]
pub struct HeartbeatResponse {
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

/// Task from Control Plane
#[derive(Debug, Clone, Deserialize)]
pub struct Task {
    pub id: i64,
    pub title: String,
    pub description: Option<String>,
    pub creator_id: Option<i64>,
    pub provider_type: String,
    pub target_node_id: Option<String>,
    pub assigned_node_id: Option<String>,
    pub status: TaskStatus,
    pub workspace_path: Option<String>,
    pub params: serde_json::Value,
    pub priority: i32,
    pub created_at: String,
    pub updated_at: String,
}
