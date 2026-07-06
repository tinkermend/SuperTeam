use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::events::TurnUsage;
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
            summary: Some("done".to_string()),
            usage: None,
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
    assert_eq!(completed, ProviderEvent::TurnCompleted { summary: None, usage: None });
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
            summary: Some("done".to_string()),
            usage: None,
        }
    );
}

#[test]
fn codex_error_event_returns_error() {
    let error = parse_codex_event(r#"{"type":"error","message":"codex failed"}"#)
        .expect_err("codex error event should fail");

    assert!(error.to_string().contains("codex failed"));
}

#[test]
fn codex_turn_failed_event_returns_nested_error_message() {
    let error = parse_codex_event(r#"{"type":"turn.failed","error":{"message":"invalid model"}}"#)
        .expect_err("codex failed turn should fail");

    assert!(error.to_string().contains("invalid model"));
}

#[test]
fn parses_codex_realistic_thread_item_and_turn_events() {
    let lines = [
        r#"{"type":"thread.started","thread":{"id":"thread-1"}}"#,
        r#"{"type":"turn.started"}"#,
        r#"{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"OK"}}"#,
        r#"{"type":"turn.completed"}"#,
    ];

    let events: Vec<_> = lines
        .iter()
        .filter_map(|line| parse_codex_event(line).expect("valid codex json line"))
        .collect();

    assert_eq!(
        events,
        vec![
            ProviderEvent::SessionStarted {
                session_id: "thread-1".to_string(),
                session_state: None,
            },
            ProviderEvent::TextDelta {
                text: "OK".to_string()
            },
            ProviderEvent::TurnCompleted { summary: None, usage: None },
        ]
    );
}

#[test]
fn parses_claude_result_event_with_usage() {
    let completed = parse_claude_event(
        r#"{"type":"result","result":"done","usage":{"total_tokens":1500,"input_tokens":200,"output_tokens":1300}}"#,
    )
    .expect("valid json")
    .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: Some(TurnUsage {
                total_tokens: 1500,
                input_tokens: Some(200),
                output_tokens: Some(1300),
            }),
        }
    );
}

#[test]
fn parses_claude_result_event_without_usage_keeps_usage_none() {
    let completed = parse_claude_event(r#"{"type":"result","result":"done"}"#)
        .expect("valid json")
        .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: None,
        }
    );
}

#[test]
fn parses_codex_turn_completed_with_usage() {
    let completed = parse_codex_event(
        r#"{"type":"turn.completed","summary":"done","usage":{"input_tokens":400,"output_tokens":600}}"#,
    )
    .expect("valid json")
    .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string()),
            usage: Some(TurnUsage {
                total_tokens: 1000,
                input_tokens: Some(400),
                output_tokens: Some(600),
            }),
        }
    );
}

#[test]
fn parses_opencode_turn_completed_with_usage() {
    let completed = parse_opencode_event(
        r#"{"type":"turn.completed","usage":{"total_tokens":77}}"#,
    )
    .expect("valid json")
    .expect("event");

    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: None,
            usage: Some(TurnUsage {
                total_tokens: 77,
                input_tokens: None,
                output_tokens: None,
            }),
        }
    );
}
