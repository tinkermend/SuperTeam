use std::path::PathBuf;

use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::runs::{RunSpec, RunStatus, RuntimeRunStore};
use tempfile::TempDir;

#[tokio::test]
async fn store_records_provider_session_events_and_replays_them() {
    let temp = TempDir::new().expect("tempdir");
    let store = RuntimeRunStore::new(temp.path().join("runs"));
    let run = store
        .start_run(
            RunSpec {
                provider_kind: "claude".to_string(),
                workspace_path: PathBuf::from("/tmp/workspace"),
                prompt: "hello".to_string(),
                session_id: None,
                continue_session: false,
                model: None,
                command_context: None,
            },
            None,
        )
        .await
        .expect("start run");

    store
        .record_event(
            &run.id,
            ProviderEvent::SessionStarted {
                session_id: "claude-session-1".to_string(),
            },
        )
        .await
        .expect("record session");
    store
        .record_event(
            &run.id,
            ProviderEvent::TextDelta {
                text: "hello from provider".to_string(),
            },
        )
        .await
        .expect("record text");
    store
        .record_event(
            &run.id,
            ProviderEvent::TurnCompleted {
                summary: Some("done".to_string()),
            },
        )
        .await
        .expect("record completion");

    let snapshot = store.get_run(&run.id).await.expect("run snapshot");
    assert_eq!(snapshot.status, RunStatus::Completed);
    assert_eq!(
        snapshot.provider_session_id.as_deref(),
        Some("claude-session-1")
    );
    let snapshot_json = serde_json::to_value(&snapshot).expect("snapshot json");
    assert!(snapshot_json.get("command_context").is_none());

    let events = store.events(&run.id).await.expect("events");
    assert_eq!(events.len(), 3);
    assert_eq!(events[0].sequence, 1);
    assert_eq!(
        events[0].event,
        ProviderEvent::SessionStarted {
            session_id: "claude-session-1".to_string()
        }
    );
    assert_eq!(events[2].sequence, 3);

    let event_log = temp.path().join("runs").join(&run.id).join("events.jsonl");
    let persisted = std::fs::read_to_string(event_log).expect("event log");
    assert!(persisted.contains("\"type\":\"session_started\""));
    assert!(persisted.contains("\"type\":\"turn_completed\""));
}

#[tokio::test]
async fn store_preserves_runtime_command_metadata_on_snapshot() {
    let temp = TempDir::new().expect("tempdir");
    let store = RuntimeRunStore::new(temp.path().join("runs"));
    let command_context = superteam_runtime_agent::runs::RuntimeCommandRunContext {
        command_id: "cmd-001".to_string(),
        digital_employee_id: "11111111-1111-4111-8111-111111111111".to_string(),
        execution_instance_id: "22222222-2222-4222-8222-222222222222".to_string(),
        provider_type: "claude-code".to_string(),
        session_policy: serde_json::json!({"mode":"new","recoverable":true}),
        context_refs: vec![serde_json::json!({"id":"ctx-1","kind":"memory"})],
        artifact_refs: vec![serde_json::json!({"id":"artifact-1","kind":"input"})],
        metadata: serde_json::json!({"source":"runtime-command-test"}),
    };

    let run = store
        .start_run(
            RunSpec {
                provider_kind: "claude".to_string(),
                workspace_path: PathBuf::from("/tmp/workspace"),
                prompt: "hello".to_string(),
                session_id: None,
                continue_session: false,
                model: None,
                command_context: Some(command_context.clone()),
            },
            None,
        )
        .await
        .expect("start run");

    let snapshot = store.get_run(&run.id).await.expect("run snapshot");
    assert_eq!(snapshot.command_context, Some(command_context));
}
