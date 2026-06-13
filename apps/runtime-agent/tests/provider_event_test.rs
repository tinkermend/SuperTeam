use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::providers::claude::parse_claude_event;
use superteam_runtime_agent::providers::codex::parse_codex_event;
use superteam_runtime_agent::providers::opencode::parse_opencode_event;

#[test]
fn parses_claude_session_and_text_and_completion_events() {
    let session = parse_claude_event(r#"{"type":"system","session_id":"abc"}"#)
        .expect("valid json")
        .expect("event");
    let text = parse_claude_event(
        r#"{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}"#,
    )
    .expect("valid json")
    .expect("event");
    let completed = parse_claude_event(r#"{"type":"result","result":"done"}"#)
        .expect("valid json")
        .expect("event");

    assert_eq!(
        session,
        ProviderEvent::SessionStarted {
            session_id: "abc".to_string(),
            session_state: None,
        }
    );
    assert_eq!(
        text,
        ProviderEvent::TextDelta {
            text: "hello".to_string()
        }
    );
    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string())
        }
    );
}

#[test]
fn parses_opencode_session_text_and_completion_events() {
    let session = parse_opencode_event(r#"{"type":"session.updated","sessionID":"oc-1"}"#)
        .expect("valid json")
        .expect("event");
    let text = parse_opencode_event(r#"{"type":"message.delta","delta":"hello"}"#)
        .expect("valid json")
        .expect("event");
    let completed = parse_opencode_event(r#"{"type":"turn.completed"}"#)
        .expect("valid json")
        .expect("event");

    assert_eq!(
        session,
        ProviderEvent::SessionStarted {
            session_id: "oc-1".to_string(),
            session_state: None,
        }
    );
    assert_eq!(
        text,
        ProviderEvent::TextDelta {
            text: "hello".to_string()
        }
    );
    assert_eq!(completed, ProviderEvent::TurnCompleted { summary: None });
}

#[test]
fn parses_codex_session_text_and_completion_events() {
    let session = parse_codex_event(r#"{"type":"session","session_id":"codex-session"}"#)
        .expect("valid json")
        .expect("event");
    let text = parse_codex_event(r#"{"type":"message.delta","delta":"hello from codex"}"#)
        .expect("valid json")
        .expect("event");
    let completed = parse_codex_event(r#"{"type":"turn.completed","summary":"done"}"#)
        .expect("valid json")
        .expect("event");

    assert_eq!(
        session,
        ProviderEvent::SessionStarted {
            session_id: "codex-session".to_string(),
            session_state: None,
        }
    );
    assert_eq!(
        text,
        ProviderEvent::TextDelta {
            text: "hello from codex".to_string()
        }
    );
    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string())
        }
    );
}

#[test]
fn codex_error_event_returns_error() {
    let error = parse_codex_event(r#"{"type":"error","message":"codex failed"}"#)
        .expect_err("codex error event should fail");

    assert!(error.to_string().contains("codex failed"));
}
