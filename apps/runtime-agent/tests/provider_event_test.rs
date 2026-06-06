use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::providers::claude::parse_claude_event;
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
