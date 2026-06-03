use serde_json::json;
use superteam_runtime_agent::commands::payload::{RuntimeSessionCommandPayload, SessionPolicyMode};
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
        "digital_employee_id": "11111111-1111-4111-8111-111111111111",
        "execution_instance_id": "22222222-2222-4222-8222-222222222222",
        "provider_type": "claude-code",
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
