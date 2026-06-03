use superteam_runtime_agent::commands::registry::{
    ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding,
};

fn binding() -> RuntimeRunBinding {
    RuntimeRunBinding {
        command_id: "cmd-001".to_string(),
        run_id: "run-001".to_string(),
        execution_instance_id: "22222222-2222-4222-8222-222222222222".to_string(),
        provider_type: "claude-code".to_string(),
        provider_session_id: None,
    }
}

#[test]
fn registry_resolves_command_and_latest_provider_session() {
    let registry = RuntimeCommandRegistry::default();
    registry.record_run_started(binding());
    registry.record_provider_session("run-001", "provider-session-1");

    assert_eq!(
        registry.run_for_command("cmd-001").as_deref(),
        Some("run-001")
    );
    assert_eq!(
        registry
            .latest_provider_session("22222222-2222-4222-8222-222222222222", "claude-code")
            .as_deref(),
        Some("provider-session-1")
    );
}

#[test]
fn registry_returns_active_runs_for_stop_priority() {
    let registry = RuntimeCommandRegistry::default();
    registry.record_run_started(binding());
    registry.record_provider_session("run-001", "provider-session-1");

    assert_eq!(
        registry
            .active_run(ActiveRunLookup {
                command_id: Some("cmd-001"),
                provider_session_id: None,
                execution_instance_id: "22222222-2222-4222-8222-222222222222",
                provider_type: "claude-code",
            })
            .as_deref(),
        Some("run-001")
    );

    registry.record_run_finished("run-001");
    assert!(
        registry
            .active_run(ActiveRunLookup {
                command_id: Some("cmd-001"),
                provider_session_id: Some("provider-session-1"),
                execution_instance_id: "22222222-2222-4222-8222-222222222222",
                provider_type: "claude-code",
            })
            .is_none()
    );
}

#[test]
fn registry_records_rejected_commands() {
    let registry = RuntimeCommandRegistry::default();
    registry.record_rejection("cmd-bad", "prompt or input is required");

    assert_eq!(
        registry.rejection("cmd-bad").as_deref(),
        Some("prompt or input is required")
    );
}
