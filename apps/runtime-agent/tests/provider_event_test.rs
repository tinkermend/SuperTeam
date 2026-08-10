use superteam_runtime_agent::events::ProviderEvent;
use superteam_runtime_agent::events::TurnUsage;
use superteam_runtime_agent::providers::claude::parse_claude_event;
use superteam_runtime_agent::providers::codex::parse_codex_event;
use superteam_runtime_agent::providers::opencode::parse_opencode_event;

#[test]
fn parses_claude_session_and_text_and_completion_events() {
    let session = parse_claude_event(r#"{"type":"system","session_id":"abc"}"#)
        .expect("valid json")
        .into_iter()
        .next()
        .expect("event");
    let text = parse_claude_event(
        r#"{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}"#,
    )
    .expect("valid json")
    .into_iter()
    .next()
    .expect("event");
    let completed = parse_claude_event(r#"{"type":"result","result":"done"}"#)
        .expect("valid json")
        .into_iter()
        .next()
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
        .into_iter()
        .next()
        .expect("event");
    let text = parse_opencode_event(r#"{"type":"message.delta","delta":"hello"}"#)
        .expect("valid json")
        .into_iter()
        .next()
        .expect("event");
    let completed = parse_opencode_event(r#"{"type":"turn.completed"}"#)
        .expect("valid json")
        .into_iter()
        .next()
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
    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: None,
            usage: None
        }
    );
}

#[test]
fn parses_codex_session_text_and_completion_events() {
    let session = parse_codex_event(r#"{"type":"session","session_id":"codex-session"}"#)
        .expect("valid json")
        .into_iter()
        .next()
        .expect("event");
    let text = parse_codex_event(r#"{"type":"message.delta","delta":"hello from codex"}"#)
        .expect("valid json")
        .into_iter()
        .next()
        .expect("event");
    let completed = parse_codex_event(r#"{"type":"turn.completed","summary":"done"}"#)
        .expect("valid json")
        .into_iter()
        .next()
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
        .flat_map(|line| parse_codex_event(line).expect("valid codex json line"))
        .collect();

    assert_eq!(
        events,
        vec![
            ProviderEvent::SessionStarted {
                session_id: "thread-1".to_string(),
                session_state: None,
            },
            ProviderEvent::TurnStarted,
            ProviderEvent::TextDelta {
                text: "OK".to_string()
            },
            ProviderEvent::TurnCompleted {
                summary: None,
                usage: None
            },
        ]
    );
}

#[test]
fn parses_claude_result_event_with_usage() {
    let completed = parse_claude_event(
        r#"{"type":"result","result":"done","usage":{"total_tokens":1500,"input_tokens":200,"output_tokens":1300}}"#,
    )
    .expect("valid json")
    .into_iter()
    .next()
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
        .into_iter()
        .next()
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
    .into_iter()
    .next()
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
    let completed =
        parse_opencode_event(r#"{"type":"turn.completed","usage":{"total_tokens":77}}"#)
            .expect("valid json")
            .into_iter()
            .next()
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

#[test]
fn parses_multiple_events_from_one_assistant_message() {
    let events = parse_claude_event(
        r#"{"type":"assistant","message":{"content":[
            {"type":"text","text":"running two tools"},
            {"type":"tool_use","id":"t1","name":"Bash","input":{"command":"git status"}},
            {"type":"tool_use","id":"t2","name":"Bash","input":{"command":"false"}}
        ]}}"#,
    )
    .expect("valid json");

    assert_eq!(events.len(), 3);
    assert_eq!(
        events[0],
        ProviderEvent::TextDelta {
            text: "running two tools".to_string()
        }
    );
    match &events[1] {
        ProviderEvent::ToolStarted {
            tool_id,
            name,
            input_excerpt,
            input_truncated,
        } => {
            assert_eq!(tool_id, "t1");
            assert_eq!(name, "Bash");
            assert!(input_excerpt.contains("git status"));
            assert!(!input_truncated);
        }
        other => panic!("expected ToolStarted, got {other:?}"),
    }
    match &events[2] {
        ProviderEvent::ToolStarted { tool_id, .. } => assert_eq!(tool_id, "t2"),
        other => panic!("expected ToolStarted, got {other:?}"),
    }
}

#[test]
fn parses_tool_result_error_flag_from_user_message() {
    let events = parse_claude_event(
        r#"{"type":"user","message":{"content":[
            {"type":"tool_result","tool_use_id":"t2","is_error":true,"content":"exit code 1"}
        ]}}"#,
    )
    .expect("valid json");

    assert_eq!(
        events,
        vec![ProviderEvent::ToolCompleted {
            tool_id: "t2".to_string(),
            is_error: true,
            output_excerpt: "exit code 1".to_string(),
            output_truncated: false,
        }]
    );
}

#[test]
fn tool_result_is_error_defaults_to_false_when_absent() {
    let events = parse_claude_event(
        r#"{"type":"user","message":{"content":[
            {"type":"tool_result","tool_use_id":"t1","content":"ok"}
        ]}}"#,
    )
    .expect("valid json");

    match &events[0] {
        ProviderEvent::ToolCompleted { is_error, .. } => assert!(!is_error),
        other => panic!("expected ToolCompleted, got {other:?}"),
    }
}

#[test]
fn normalizes_tool_result_content_given_as_block_array() {
    let events = parse_claude_event(
        r#"{"type":"user","message":{"content":[
            {"type":"tool_result","tool_use_id":"t1","content":[
                {"type":"text","text":"line one"},
                {"type":"text","text":"line two"}
            ]}
        ]}}"#,
    )
    .expect("valid json");

    match &events[0] {
        ProviderEvent::ToolCompleted { output_excerpt, .. } => {
            assert_eq!(output_excerpt, "line one\nline two");
        }
        other => panic!("expected ToolCompleted, got {other:?}"),
    }
}

#[test]
fn truncates_oversized_tool_input() {
    let payload = "x".repeat(10_000);
    let line = serde_json::json!({
        "type": "assistant",
        "message": {"content": [
            {"type": "tool_use", "id": "t1", "name": "Write", "input": {"content": payload}}
        ]}
    })
    .to_string();

    let events = parse_claude_event(&line).expect("valid json");
    match &events[0] {
        ProviderEvent::ToolStarted {
            input_excerpt,
            input_truncated,
            ..
        } => {
            assert!(input_truncated);
            assert!(input_excerpt.ends_with("…[truncated]"));
            assert!(input_excerpt.len() < 10_000);
        }
        other => panic!("expected ToolStarted, got {other:?}"),
    }
}

#[test]
fn unparseable_line_yields_no_events_instead_of_failing_the_run() {
    let events = parse_claude_event("this is not json").expect("must not fail the run");
    assert!(events.is_empty());

    let events = parse_opencode_event("<<< warning from provider").expect("must not fail the run");
    assert!(events.is_empty());
}

#[test]
fn codex_failure_events_still_propagate_as_errors() {
    // A provider-reported failure is not a parse failure; it must not be
    // swallowed alongside unreadable lines.
    let result = parse_codex_event(r#"{"type":"turn.failed","error":{"message":"boom"}}"#);
    assert!(
        result.is_err(),
        "codex turn.failed must surface as an error"
    );
}

#[test]
fn claude_result_is_error_emits_turn_error_not_completed() {
    // Spec 1.3.4: top-level is_error on result must fail the attempt.
    let events = parse_claude_event(
        r#"{"type":"result","is_error":true,"subtype":"error_during_execution","result":"rate limit exceeded"}"#,
    )
    .expect("valid json");
    assert_eq!(events.len(), 1);
    match &events[0] {
        ProviderEvent::TurnError { message, error } => {
            assert!(message.contains("rate limit"), "message={message}");
            let envelope = error.as_ref().expect("structured error required");
            assert_eq!(envelope.code, "RATE_LIMIT");
            assert_eq!(envelope.family, "transient_provider");
            assert!(envelope.retryable);
            assert_eq!(
                envelope.native.as_ref().and_then(|n| n.subtype.as_deref()),
                Some("error_during_execution")
            );
        }
        other => panic!("expected TurnError, got {other:?}"),
    }
}

#[test]
fn claude_result_is_error_false_still_completes() {
    let events = parse_claude_event(
        r#"{"type":"result","is_error":false,"result":"done","usage":{"input_tokens":1,"output_tokens":2}}"#,
    )
    .expect("valid json");
    assert_eq!(events.len(), 1);
    assert!(matches!(
        &events[0],
        ProviderEvent::TurnCompleted {
            summary: Some(s),
            ..
        } if s == "done"
    ));
}
