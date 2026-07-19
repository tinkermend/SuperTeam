use superteam_runtime_agent::controlplane::{
    ControlPlaneClient, EnrollHelloRequest, EnrollHelloResponse, EnrollmentStatus,
    HeartbeatRequest, HeartbeatResponse, NodeStatus, RegisterNodeRequest, RegisterNodeResponse,
    RuntimeCapabilityInput, RuntimeCommand, RuntimeCommandType,
};
use tokio::{
    io::{AsyncReadExt, AsyncWriteExt},
    net::{TcpListener, TcpStream},
    sync::oneshot,
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
            supports_platform_limits: true,
    };

    let json = serde_json::to_string(&req).unwrap();
    assert!(json.contains("\"current_load\":2"));
    assert!(json.contains("\"status\":\"online\""));
}

#[test]
fn test_enroll_hello_serializes_flat_control_plane_contract() {
    let req = EnrollHelloRequest {
        node_id: "node-hello".to_string(),
        bootstrap_key: "bootstrap-secret".to_string(),
        name: "runtime-node-hello".to_string(),
        version: Some("0.1.0".to_string()),
        supported_providers: vec!["claude-code".to_string()],
        max_slots: 3,
        metadata: None,
        capabilities: vec![RuntimeCapabilityInput {
            capability_type: "provider".to_string(),
            capability_key: "claude-code".to_string(),
            provider_type: "claude-code".to_string(),
            provider_version: None,
            binary_path: Some("claude".to_string()),
            available: true,
            workspace_base_dir: None,
            capacity: None,
            labels: None,
            status: "available".to_string(),
            details: None,
            health_status: "configured".to_string(),
            metadata: None,
        }],
    };

    let json = serde_json::to_value(&req).unwrap();

    assert_eq!(json["node_id"], "node-hello");
    assert_eq!(json["bootstrap_key"], "bootstrap-secret");
    assert_eq!(json["max_slots"], 3);
    assert_eq!(json["capabilities"][0]["capability_type"], "provider");
    assert!(json.get("providers").is_none());
    assert!(json.get("workspace").is_none());
    assert!(json.get("capacity").is_none());
}

#[test]
fn test_enroll_hello_response_reads_top_level_session_token() {
    let response: EnrollHelloResponse = serde_json::from_value(serde_json::json!({
        "enrollment": {
            "id": "11111111-1111-4111-8111-111111111111",
            "tenant_id": "22222222-2222-4222-8222-222222222222",
            "runtime_node_id": "33333333-3333-4333-8333-333333333333",
            "node_id": "node-hello",
            "bootstrap_key_id": "44444444-4444-4444-8444-444444444444",
            "status": "approved",
            "created_at": "2026-06-02T00:00:00Z",
            "updated_at": "2026-06-02T00:00:00Z"
        },
        "session": {
            "id": "55555555-5555-4555-8555-555555555555",
            "tenant_id": "22222222-2222-4222-8222-222222222222",
            "runtime_node_id": "33333333-3333-4333-8333-333333333333",
            "node_id": "node-hello",
            "enrollment_id": "11111111-1111-4111-8111-111111111111",
            "expires_at": "2026-06-02T12:00:00Z",
            "last_seen_at": "2026-06-02T00:00:00Z",
            "created_at": "2026-06-02T00:00:00Z",
            "updated_at": "2026-06-02T00:00:00Z"
        },
        "session_token": "runtime-session-token"
    }))
    .unwrap();

    assert_eq!(response.enrollment.status, EnrollmentStatus::Approved);
    assert_eq!(
        response.session_token.as_deref(),
        Some("runtime-session-token")
    );
    assert_eq!(
        response.session.as_ref().map(|session| session.id.as_str()),
        Some("55555555-5555-4555-8555-555555555555")
    );
}

#[test]
fn test_runtime_command_deserializes_ensure_instance() {
    let command: RuntimeCommand = serde_json::from_value(serde_json::json!({
        "id": "cmd-1",
        "type": "ensure_instance",
        "payload": {
            "execution_instance_id": "11111111-1111-4111-8111-111111111111"
        }
    }))
    .unwrap();

    assert_eq!(command.id, "cmd-1");
    assert_eq!(command.command_type, RuntimeCommandType::EnsureInstance);
    assert_eq!(
        command.payload["execution_instance_id"],
        "11111111-1111-4111-8111-111111111111"
    );
}

#[test]
fn test_runtime_command_deserializes_unknown_command_as_unsupported() {
    let command: RuntimeCommand = serde_json::from_value(serde_json::json!({
        "id": "cmd-legacy",
        "type": "task.claim",
        "payload": {}
    }))
    .unwrap();

    assert_eq!(
        command.command_type,
        RuntimeCommandType::Unsupported("task.claim".to_string())
    );
}

#[test]
fn test_register_response_deserializes_control_plane_contract() {
    let response: RegisterNodeResponse = serde_json::from_value(runtime_node_response()).unwrap();

    assert_eq!(response.node_id, "test-node-001");
    assert_eq!(response.status, NodeStatus::Online);
    assert_eq!(response.supported_providers, ["claude-code", "opencode"]);
}

#[test]
fn test_heartbeat_response_deserializes_control_plane_contract() {
    let response: HeartbeatResponse = serde_json::from_value(runtime_node_response()).unwrap();

    assert_eq!(response.node_id, "test-node-001");
    assert_eq!(response.current_load, 0);
    assert_eq!(response.status, NodeStatus::Online);
    assert!(response.required_tools.is_empty());
}

#[test]
fn test_heartbeat_response_deserializes_required_tools() {
    let mut value = runtime_node_response();
    value["required_tools"] = serde_json::json!(["gh", "terraform"]);

    let response: HeartbeatResponse = serde_json::from_value(value).unwrap();

    assert_eq!(response.required_tools, ["gh", "terraform"]);
}

#[test]
fn test_runtime_node_response_allows_optional_timestamps() {
    let response: RegisterNodeResponse = serde_json::from_value(serde_json::json!({
        "node_id": "test-node-001",
        "name": "Test Node",
        "supported_providers": ["claude-code"],
        "max_slots": 5,
        "current_load": 0,
        "status": "online",
        "metadata": null,
        "last_heartbeat_at": null
    }))
    .unwrap();

    assert_eq!(response.node_id, "test-node-001");
}

fn runtime_node_response() -> serde_json::Value {
    serde_json::json!({
        "node_id": "test-node-001",
        "name": "Test Node",
        "supported_providers": ["claude-code", "opencode"],
        "max_slots": 5,
        "current_load": 0,
        "status": "online",
        "metadata": null,
        "last_heartbeat_at": null,
        "created_at": "2026-05-29T00:00:00Z",
        "updated_at": "2026-05-29T00:00:00Z"
    })
}

// Integration tests that require a running Control Plane server
// These are marked with #[ignore] by default
#[tokio::test]
#[ignore]
async fn test_register_integration() {
    let timestamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs();
    let node_id = format!("test-node-{}", timestamp);
    let client = ControlPlaneClient::with_node_id("http://localhost:8080", "test-token", &node_id);

    let req = RegisterNodeRequest {
        node_id,
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
    let timestamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs();
    let node_id = format!("test-node-{}", timestamp);
    let client = ControlPlaneClient::with_node_id("http://localhost:8080", "test-token", &node_id);

    // First register
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
            supports_platform_limits: true,
    };

    let result = client.heartbeat(heartbeat_req).await;
    assert!(result.is_ok(), "Heartbeat should succeed: {:?}", result);

    let response = result.unwrap();
    assert_eq!(response.current_load, 1);
    assert_eq!(response.status, NodeStatus::Online);
}

#[tokio::test]
async fn controlplane_client_heartbeat_sends_runtime_identity_headers() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, request_rx) = oneshot::channel();

    tokio::spawn(async move {
        let (mut socket, _) = listener.accept().await.unwrap();
        let request = read_http_request(&mut socket).await;
        let _ = request_tx.send(request);

        write_json_response(
            &mut socket,
            serde_json::json!({
                "node_id": "node-1",
                "name": "runtime-node-1",
                "supported_providers": [],
                "required_tools": [],
                "max_slots": 5,
                "current_load": 0,
                "status": "online",
                "metadata": null,
                "last_heartbeat_at": "2026-06-02T00:00:00Z",
                "created_at": "2026-06-02T00:00:00Z",
                "updated_at": "2026-06-02T00:00:00Z"
            }),
        )
        .await;
    });

    let client =
        ControlPlaneClient::with_node_id(format!("http://{}", addr), "test-token", "node-1");
    let result = client
        .heartbeat(HeartbeatRequest {
            current_load: 0,
            status: NodeStatus::Online,
            supports_platform_limits: true,
        })
        .await
        .unwrap();

    assert_eq!(result.node_id, "node-1");

    let request = request_rx.await.unwrap();
    let request_line = request.lines().next().unwrap();
    assert_eq!(request_line, "POST /api/v1/runtime/heartbeat HTTP/1.1");
    assert!(request.contains("authorization: Bearer test-token"));
    assert!(request.contains("x-node-id: node-1"));
}

#[tokio::test]
async fn shared_runtime_auth_client_uses_latest_token_for_each_request() {
    use superteam_runtime_agent::runtime_auth::RuntimeAuthState;

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, mut request_rx) = tokio::sync::mpsc::channel(2);

    tokio::spawn(async move {
        for _ in 0..2 {
            let (mut socket, _) = listener.accept().await.unwrap();
            let request = read_http_request(&mut socket).await;
            request_tx.send(request).await.unwrap();
            write_json_response(
                &mut socket,
                serde_json::json!({
                    "node_id": "node-1",
                    "name": "runtime-node-1",
                    "supported_providers": [],
                    "required_tools": [],
                    "max_slots": 5,
                    "current_load": 0,
                    "status": "online",
                    "metadata": null,
                    "last_heartbeat_at": "2026-06-02T00:00:00Z",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                }),
            )
            .await;
        }
    });

    let auth = RuntimeAuthState::new("node-1");
    auth.set_session("session-1", "token-one", "2999-06-02T00:00:00Z")
        .await
        .expect("initial session");
    let client = ControlPlaneClient::with_runtime_auth(format!("http://{}", addr), auth.clone());

    client
        .heartbeat(HeartbeatRequest {
            current_load: 0,
            status: NodeStatus::Online,
            supports_platform_limits: true,
        })
        .await
        .unwrap();
    auth.set_session("session-2", "token-two", "2999-06-03T00:00:00Z")
        .await
        .expect("refreshed session");
    client
        .heartbeat(HeartbeatRequest {
            current_load: 0,
            status: NodeStatus::Online,
            supports_platform_limits: true,
        })
        .await
        .unwrap();

    let first = request_rx.recv().await.expect("first request");
    let second = request_rx.recv().await.expect("second request");
    assert!(first.contains("authorization: Bearer token-one"));
    assert!(second.contains("authorization: Bearer token-two"));
    assert!(first.contains("x-node-id: node-1"));
    assert!(second.contains("x-node-id: node-1"));
}

#[tokio::test]
async fn controlplane_client_classifies_runtime_auth_unauthorized() {
    use superteam_runtime_agent::runtime_auth::{RuntimeAuthState, is_runtime_auth_expired};

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    tokio::spawn(async move {
        let (mut socket, _) = listener.accept().await.unwrap();
        let _request = read_http_request(&mut socket).await;
        socket
            .write_all(
                b"HTTP/1.1 401 Unauthorized\r\nContent-Length: 30\r\n\r\ninvalid runtime authentication",
            )
            .await
            .unwrap();
    });

    let auth = RuntimeAuthState::new("node-1");
    auth.set_session("session-1", "expired-token", "2999-06-02T00:00:00Z")
        .await
        .expect("session");
    let client = ControlPlaneClient::with_runtime_auth(format!("http://{}", addr), auth.clone());

    let error = client
        .heartbeat(HeartbeatRequest {
            current_load: 0,
            status: NodeStatus::Online,
            supports_platform_limits: true,
        })
        .await
        .expect_err("heartbeat should fail");
    assert!(is_runtime_auth_expired(error.as_ref()));
    assert_eq!(
        auth.snapshot().await.status,
        superteam_runtime_agent::runtime_auth::RuntimeAuthStatus::Reauthenticating
    );
}

#[tokio::test]
async fn runtime_auth_reauth_wait_observes_fast_status_transitions() {
    use superteam_runtime_agent::runtime_auth::{RuntimeAuthState, RuntimeAuthStatus};

    let auth = RuntimeAuthState::new("node-1");
    auth.set_session("session-1", "token-one", "2999-06-02T00:00:00Z")
        .await
        .expect("session");

    let seen_generation = auth.snapshot().await.reauth_generation;
    auth.report_auth_failure(None).await;
    auth.mark_status(RuntimeAuthStatus::Connected).await;

    tokio::time::timeout(
        std::time::Duration::from_secs(2),
        auth.wait_for_next_reauth_event(seen_generation),
    )
    .await
    .expect("reauth event should not be missed");
}

#[tokio::test]
async fn shared_runtime_auth_blocks_business_requests_inside_safety_window() {
    use std::time::Duration;
    use superteam_runtime_agent::runtime_auth::RuntimeAuthState;

    let auth =
        RuntimeAuthState::with_windows("node-1", Duration::from_secs(60), Duration::from_secs(60));
    auth.set_session("session-1", "token-one", "1970-01-01T00:00:00Z")
        .await
        .expect("expired session");

    let wait = tokio::spawn({
        let auth = auth.clone();
        async move { auth.wait_for_business_credentials().await.unwrap() }
    });

    tokio::time::sleep(Duration::from_millis(100)).await;
    assert!(
        !wait.is_finished(),
        "business credentials should wait while unsafe"
    );

    auth.set_session("session-2", "token-two", "2999-06-02T00:00:00Z")
        .await
        .expect("fresh session");
    let credentials = tokio::time::timeout(Duration::from_secs(2), wait)
        .await
        .expect("wait should unblock")
        .expect("join");
    assert_eq!(credentials.token, "token-two");
}

#[test]
fn runtime_auth_expired_detection_walks_error_sources() {
    use reqwest::StatusCode;
    use superteam_runtime_agent::runtime_auth::{RuntimeAuthExpired, is_runtime_auth_expired};

    let error = anyhow::Error::new(RuntimeAuthExpired {
        operation: "Claim task",
        status: StatusCode::UNAUTHORIZED,
        body: "invalid runtime authentication".to_string(),
        generation: Some(1),
    })
    .context("claim loop failed");

    assert!(is_runtime_auth_expired(error.as_ref()));
}

#[tokio::test]
async fn controlplane_client_upsert_capabilities_sends_openapi_wrapper_body() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, request_rx) = oneshot::channel();

    tokio::spawn(async move {
        let (mut socket, _) = listener.accept().await.unwrap();
        let request = read_http_request(&mut socket).await;
        let _ = request_tx.send(request);

        socket
            .write_all(
                br#"HTTP/1.1 200 OK
Content-Type: application/json
Content-Length: 2

[]"#,
            )
            .await
            .unwrap();
    });

    let client = ControlPlaneClient::with_session_token(
        format!("http://{}", addr),
        "session-token",
        "node-1",
    );
    let result = client
        .upsert_capabilities(
            "node-1",
            vec![RuntimeCapabilityInput {
                capability_type: "provider".to_string(),
                capability_key: "claude-code".to_string(),
                provider_type: "claude-code".to_string(),
                provider_version: None,
                binary_path: Some("claude".to_string()),
                available: true,
                workspace_base_dir: None,
                capacity: None,
                labels: None,
                status: "available".to_string(),
                details: None,
                health_status: "configured".to_string(),
                metadata: None,
            }],
        )
        .await
        .unwrap();

    assert!(result.is_empty());

    let request = request_rx.await.unwrap();
    let request_line = request.lines().next().unwrap();
    assert_eq!(
        request_line,
        "PUT /api/v1/runtime/nodes/node-1/capabilities HTTP/1.1"
    );
    assert!(request.contains("authorization: Bearer session-token"));
    assert!(request.contains("x-node-id: node-1"));
    let (_, body) = request.split_once("\r\n\r\n").expect("http body");
    let body: serde_json::Value = serde_json::from_str(body).expect("json body");
    assert!(body.as_object().is_some());
    assert_eq!(
        body["capabilities"][0]["capability_type"],
        serde_json::json!("provider")
    );
}

async fn read_http_request(socket: &mut TcpStream) -> String {
    let mut buffer = Vec::new();
    let header_end = loop {
        let mut chunk = [0; 1024];
        let bytes_read = socket.read(&mut chunk).await.unwrap();
        assert!(bytes_read > 0, "socket closed before HTTP headers");
        buffer.extend_from_slice(&chunk[..bytes_read]);
        if let Some(index) = find_subsequence(&buffer, b"\r\n\r\n") {
            break index + 4;
        }
    };

    let headers = String::from_utf8_lossy(&buffer[..header_end]);
    let content_length = headers
        .lines()
        .filter_map(|line| line.split_once(':'))
        .find_map(|(name, value)| {
            name.eq_ignore_ascii_case("content-length")
                .then(|| value.trim().parse::<usize>().unwrap())
        })
        .unwrap_or(0);

    while buffer.len() < header_end + content_length {
        let mut chunk = [0; 1024];
        let bytes_read = socket.read(&mut chunk).await.unwrap();
        assert!(bytes_read > 0, "socket closed before HTTP body");
        buffer.extend_from_slice(&chunk[..bytes_read]);
    }

    String::from_utf8(buffer[..header_end + content_length].to_vec()).unwrap()
}

fn find_subsequence(haystack: &[u8], needle: &[u8]) -> Option<usize> {
    haystack
        .windows(needle.len())
        .position(|window| window == needle)
}

async fn write_json_response(socket: &mut TcpStream, body: serde_json::Value) {
    write_status_response(socket, "200 OK", body).await;
}

async fn write_status_response(socket: &mut TcpStream, status: &str, body: serde_json::Value) {
    let payload = body.to_string();
    let response = format!(
        "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
        payload.len(),
        payload
    );
    socket.write_all(response.as_bytes()).await.unwrap();
}
