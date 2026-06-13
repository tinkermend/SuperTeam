use serde_json::json;
use superteam_runtime_agent::commands::payload::{
    RuntimeProvisionInstanceCommandPayload, RuntimeSessionCommandPayload, SessionPolicyMode,
};
use superteam_runtime_agent::controlplane::models::{RuntimeCommand, RuntimeCommandType};

fn command(payload: serde_json::Value) -> RuntimeCommand {
    RuntimeCommand {
        id: "cmd-001".to_string(),
        command_type: RuntimeCommandType::StartSession,
        payload,
    }
}

fn valid_payload() -> serde_json::Value {
    json!({
        "command_id": "cmd-001",
        "tenant_id": "00000000-0000-4000-8000-000000000001",
        "team_id": "11111111-1111-4111-8111-111111111111",
        "digital_employee_id": "11111111-1111-4111-8111-111111111111",
        "execution_instance_id": "22222222-2222-4222-8222-222222222222",
        "runtime_node_id": "44444444-4444-4444-8444-444444444444",
        "provider_type": "claude-code",
        "agent_home_dir": "/tmp/workspaces/teams/11111111-1111-4111-8111-111111111111/employees/11111111-1111-4111-8111-111111111111",
        "workspace_files": [],
        "skills": [],
        "mcp_servers": [],
        "session_policy": {"mode": "new", "provider_session_id": null, "recoverable": true},
        "prompt": "hello",
        "input": null,
        "context_refs": [],
        "artifact_refs": [],
        "model": null,
        "metadata": {}
    })
}

#[test]
fn parses_valid_start_session_payload() {
    let parsed = RuntimeSessionCommandPayload::from_command(&command(valid_payload()))
        .expect("valid command payload");

    assert_eq!(parsed.command_id, "cmd-001");
    assert_eq!(
        parsed.digital_employee_id,
        "11111111-1111-4111-8111-111111111111"
    );
    assert_eq!(
        parsed.execution_instance_id,
        "22222222-2222-4222-8222-222222222222"
    );
    assert_eq!(parsed.provider_type, "claude-code");
    assert_eq!(parsed.provider_kind(), "claude");
    assert_eq!(parsed.session_policy.mode, SessionPolicyMode::New);
    assert_eq!(parsed.provider_prompt().as_deref(), Some("hello"));
}

#[test]
fn parses_opencode_provider_kind() {
    let mut payload = valid_payload();
    payload["provider_type"] = json!("opencode");

    let parsed = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect("valid opencode payload");

    assert_eq!(parsed.provider_type, "opencode");
    assert_eq!(parsed.provider_kind(), "opencode");
}

#[test]
fn rejects_local_provider_kind_as_provider_type() {
    let mut payload = valid_payload();
    payload["provider_type"] = json!("claude");

    let error = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect_err("local provider_kind should not be accepted as provider_type");

    assert!(error.to_string().contains("unsupported provider_type"));
}

#[test]
fn rejects_command_id_mismatch() {
    let mut payload = valid_payload();
    payload["command_id"] = json!("different-command");

    let error = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect_err("mismatched command id should fail");

    assert!(
        error
            .to_string()
            .contains("command_id does not match runtime command id")
    );
}

#[test]
fn rejects_missing_required_arrays_even_when_empty_arrays_are_allowed() {
    let mut payload = valid_payload();
    payload.as_object_mut().unwrap().remove("context_refs");

    let error = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect_err("missing context_refs should fail");

    assert!(error.to_string().contains("context_refs is required"));
}

#[test]
fn rejects_empty_prompt_for_input_commands() {
    let mut payload = valid_payload();
    payload["prompt"] = json!("");
    payload["input"] = json!(null);

    let error = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect_err("empty provider input should fail");

    assert!(error.to_string().contains("prompt or input is required"));
}

#[test]
fn allows_stop_session_without_prompt_or_input() {
    let mut command = command(valid_payload());
    command.command_type = RuntimeCommandType::StopSession;
    command.payload["prompt"] = json!("");
    command.payload["input"] = json!(null);
    command.payload.as_object_mut().unwrap().remove("tenant_id");
    command.payload.as_object_mut().unwrap().remove("team_id");
    command
        .payload
        .as_object_mut()
        .unwrap()
        .remove("runtime_node_id");
    command
        .payload
        .as_object_mut()
        .unwrap()
        .remove("agent_home_dir");

    let parsed = RuntimeSessionCommandPayload::from_command(&command)
        .expect("stop_session can omit provider input");

    assert_eq!(parsed.provider_prompt(), None);
}

#[test]
fn resume_requires_provider_session_id() {
    let mut command = command(valid_payload());
    command.command_type = RuntimeCommandType::ResumeSession;
    command.payload["session_policy"] =
        json!({"mode": "resume", "provider_session_id": null, "recoverable": true});

    let error = RuntimeSessionCommandPayload::from_command(&command)
        .expect_err("resume without provider_session_id should fail");

    assert!(
        error
            .to_string()
            .contains("provider_session_id is required for resume")
    );
}

#[test]
fn resume_session_requires_explicit_provider_session_id_even_when_mode_is_new() {
    let mut command = command(valid_payload());
    command.command_type = RuntimeCommandType::ResumeSession;
    command.payload["session_policy"] =
        json!({"mode": "new", "provider_session_id": null, "recoverable": true});

    let error = RuntimeSessionCommandPayload::from_command(&command)
        .expect_err("resume_session without provider_session_id should fail");

    assert!(
        error
            .to_string()
            .contains("provider_session_id is required for resume_session")
    );
}

#[test]
fn parses_valid_provision_payload_with_workspace_file() {
    let command = RuntimeCommand {
        id: "cmd-provision".to_string(),
        command_type: RuntimeCommandType::ProvisionInstance,
        payload: serde_json::json!({
            "command_id": "cmd-provision",
            "tenant_id": "00000000-0000-4000-8000-000000000001",
            "team_id": "11111111-1111-4111-8111-111111111111",
            "digital_employee_id": "22222222-2222-4222-8222-222222222222",
            "execution_instance_id": "33333333-3333-4333-8333-333333333333",
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
            "provider_type": "claude-code",
            "agent_home_dir": "/tmp/workspaces/teams/11111111-1111-4111-8111-111111111111/employees/22222222-2222-4222-8222-222222222222",
            "workspace_files": [{
                "file_id": "55555555-5555-4555-8555-555555555555",
                "revision_id": "66666666-6666-4666-8666-666666666666",
                "path": "AGENTS.md",
                "file_role": "entrypoint",
                "mime_type": "text/markdown",
                "sync_policy": "auto",
                "content_hash": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
                "size_bytes": 0,
                "storage_backend": "db",
                "content_text": ""
            }],
            "skills": [],
            "mcp_servers": []
        }),
    };

    let parsed = RuntimeProvisionInstanceCommandPayload::from_command(&command).unwrap();
    assert_eq!(parsed.team_id, "11111111-1111-4111-8111-111111111111");
    assert_eq!(parsed.workspace_files[0].path, "AGENTS.md");
    assert!(parsed.skills.is_empty());
    assert!(parsed.mcp_servers.is_empty());
}

#[test]
fn parses_sync_workspace_files_command_type() {
    let raw = serde_json::json!({
        "id": "cmd-sync",
        "type": "sync_workspace_files",
        "payload": {}
    });
    let command: RuntimeCommand = serde_json::from_value(raw).unwrap();
    assert_eq!(command.command_type, RuntimeCommandType::SyncWorkspaceFiles);
}
