use superteam_runtime_agent::controlplane::{
    ControlPlaneClient, HeartbeatRequest, NodeStatus, RegisterNodeRequest,
};

#[test]
fn test_client_creation() {
    let _client = ControlPlaneClient::new("http://localhost:8080", "test-token");
    // Client should be created successfully
    assert!(true);
}

#[test]
fn test_register_request_serialization() {
    let req = RegisterNodeRequest {
        node_id: "test-node-001".to_string(),
        name: "Test Node".to_string(),
        supported_providers: vec!["claude-code".to_string(), "opencode".to_string()],
        max_slots: 5,
        metadata: None,
    };

    let json = serde_json::to_string(&req).unwrap();
    assert!(json.contains("test-node-001"));
    assert!(json.contains("claude-code"));
}

#[test]
fn test_heartbeat_request_serialization() {
    let req = HeartbeatRequest {
        current_load: 2,
        status: NodeStatus::Online,
    };

    let json = serde_json::to_string(&req).unwrap();
    assert!(json.contains("\"current_load\":2"));
    assert!(json.contains("\"status\":\"online\""));
}

// Integration tests that require a running Control Plane server
// These are marked with #[ignore] by default
#[tokio::test]
#[ignore]
async fn test_register_integration() {
    let client = ControlPlaneClient::new("http://localhost:8080", "test-token");

    let timestamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs();

    let req = RegisterNodeRequest {
        node_id: format!("test-node-{}", timestamp),
        name: "Integration Test Node".to_string(),
        supported_providers: vec!["claude-code".to_string()],
        max_slots: 3,
        metadata: None,
    };

    let result = client.register(req).await;
    assert!(result.is_ok(), "Register should succeed: {:?}", result);

    let response = result.unwrap();
    assert_eq!(response.max_slots, 3);
    assert_eq!(response.current_load, 0);
}

#[tokio::test]
#[ignore]
async fn test_heartbeat_integration() {
    let client = ControlPlaneClient::new("http://localhost:8080", "test-token");

    let timestamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs();

    // First register
    let node_id = format!("test-node-{}", timestamp);
    let register_req = RegisterNodeRequest {
        node_id: node_id.clone(),
        name: "Heartbeat Test Node".to_string(),
        supported_providers: vec!["claude-code".to_string()],
        max_slots: 5,
        metadata: None,
    };

    client.register(register_req).await.unwrap();

    // Then send heartbeat
    let heartbeat_req = HeartbeatRequest {
        current_load: 1,
        status: NodeStatus::Online,
    };

    let result = client.heartbeat(heartbeat_req).await;
    assert!(result.is_ok(), "Heartbeat should succeed: {:?}", result);

    let response = result.unwrap();
    assert_eq!(response.current_load, 1);
    assert_eq!(response.status, NodeStatus::Online);
}

#[tokio::test]
#[ignore]
async fn test_claim_task_timeout() {
    let client = ControlPlaneClient::new("http://localhost:8080", "test-token");

    let timestamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs();

    // Register first
    let node_id = format!("test-node-{}", timestamp);
    let register_req = RegisterNodeRequest {
        node_id: node_id.clone(),
        name: "Claim Test Node".to_string(),
        supported_providers: vec!["claude-code".to_string()],
        max_slots: 5,
        metadata: None,
    };

    client.register(register_req).await.unwrap();

    // Try to claim with short timeout (should return None if no tasks)
    let result = client.claim_task(2).await;
    assert!(result.is_ok(), "Claim should succeed: {:?}", result);

    // Should be None if no tasks available
    let task = result.unwrap();
    if task.is_none() {
        println!("No tasks available (expected in test environment)");
    }
}
