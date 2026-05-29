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
    pub id: i64,
    pub node_id: String,
    pub name: String,
    pub supported_providers: Vec<String>,
    pub max_slots: i32,
    pub current_load: i32,
    pub status: NodeStatus,
    pub metadata: Option<HashMap<String, serde_json::Value>>,
    pub last_heartbeat_at: Option<String>,
    pub created_at: String,
    pub updated_at: String,
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
    pub id: i64,
    pub node_id: String,
    pub name: String,
    pub supported_providers: Vec<String>,
    pub max_slots: i32,
    pub current_load: i32,
    pub status: NodeStatus,
    pub metadata: Option<HashMap<String, serde_json::Value>>,
    pub last_heartbeat_at: Option<String>,
    pub created_at: String,
    pub updated_at: String,
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
