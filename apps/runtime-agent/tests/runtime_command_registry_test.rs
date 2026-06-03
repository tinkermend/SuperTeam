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

fn binding_for(
    command_id: &str,
    run_id: &str,
    provider_session_id: Option<&str>,
) -> RuntimeRunBinding {
    binding_for_provider(
        command_id,
        run_id,
        "22222222-2222-4222-8222-222222222222",
        "claude-code",
        provider_session_id,
    )
}

fn binding_for_provider(
    command_id: &str,
    run_id: &str,
    execution_instance_id: &str,
    provider_type: &str,
    provider_session_id: Option<&str>,
) -> RuntimeRunBinding {
    RuntimeRunBinding {
        command_id: command_id.to_string(),
        run_id: run_id.to_string(),
        execution_instance_id: execution_instance_id.to_string(),
        provider_type: provider_type.to_string(),
        provider_session_id: provider_session_id.map(str::to_string),
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
fn registry_prioritizes_command_then_provider_session_then_instance_latest() {
    let registry = RuntimeCommandRegistry::default();

    registry.record_run_started(binding_for("cmd-target", "run-cmd", None));
    registry.record_run_started(binding_for("cmd-session", "run-session", Some("session-1")));
    registry.record_run_started(binding_for(
        "cmd-instance",
        "run-instance",
        Some("session-2"),
    ));

    assert_eq!(
        registry
            .active_run(ActiveRunLookup {
                command_id: Some("cmd-target"),
                provider_session_id: Some("session-1"),
                execution_instance_id: "22222222-2222-4222-8222-222222222222",
                provider_type: "claude-code",
            })
            .as_deref(),
        Some("run-cmd")
    );
    assert_eq!(
        registry
            .active_run(ActiveRunLookup {
                command_id: None,
                provider_session_id: Some("session-1"),
                execution_instance_id: "22222222-2222-4222-8222-222222222222",
                provider_type: "claude-code",
            })
            .as_deref(),
        Some("run-session")
    );
    assert_eq!(
        registry
            .active_run(ActiveRunLookup {
                command_id: None,
                provider_session_id: None,
                execution_instance_id: "22222222-2222-4222-8222-222222222222",
                provider_type: "claude-code",
            })
            .as_deref(),
        Some("run-instance")
    );
}

#[test]
fn registry_returns_latest_active_run_for_provider_session_deterministically() {
    for attempt in 0..64 {
        let registry = RuntimeCommandRegistry::default();
        let prior_run = format!("run-session-prior-{attempt}");
        let latest_run = format!("run-session-latest-{attempt}");

        registry.record_run_started(binding_for(
            &format!("cmd-session-prior-{attempt}"),
            &prior_run,
            Some("session-1"),
        ));
        registry.record_run_started(binding_for(
            &format!("cmd-session-latest-{attempt}"),
            &latest_run,
            Some("session-1"),
        ));

        assert_eq!(
            registry
                .active_run(ActiveRunLookup {
                    command_id: None,
                    provider_session_id: Some("session-1"),
                    execution_instance_id: "22222222-2222-4222-8222-222222222222",
                    provider_type: "claude-code",
                })
                .as_deref(),
            Some(latest_run.as_str())
        );

        registry.record_run_finished(&latest_run);

        assert_eq!(
            registry
                .active_run(ActiveRunLookup {
                    command_id: None,
                    provider_session_id: Some("session-1"),
                    execution_instance_id: "22222222-2222-4222-8222-222222222222",
                    provider_type: "claude-code",
                })
                .as_deref(),
            Some(prior_run.as_str())
        );
    }
}

#[test]
fn registry_scopes_provider_session_lookup_by_instance_and_provider() {
    let registry = RuntimeCommandRegistry::default();
    let execution_instance_id = "22222222-2222-4222-8222-222222222222";

    registry.record_run_started(binding_for_provider(
        "cmd-claude",
        "run-claude",
        execution_instance_id,
        "claude-code",
        Some("shared-session"),
    ));
    registry.record_run_started(binding_for_provider(
        "cmd-opencode",
        "run-opencode",
        execution_instance_id,
        "opencode",
        Some("shared-session"),
    ));

    assert_eq!(
        registry
            .active_run(ActiveRunLookup {
                command_id: None,
                provider_session_id: Some("shared-session"),
                execution_instance_id,
                provider_type: "claude-code",
            })
            .as_deref(),
        Some("run-claude")
    );
    assert_eq!(
        registry
            .active_run(ActiveRunLookup {
                command_id: None,
                provider_session_id: Some("shared-session"),
                execution_instance_id,
                provider_type: "opencode",
            })
            .as_deref(),
        Some("run-opencode")
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
