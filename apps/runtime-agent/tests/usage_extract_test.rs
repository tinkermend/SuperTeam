use superteam_runtime_agent::events::TurnUsage;
use superteam_runtime_agent::providers::usage::extract_usage;

#[test]
fn extract_usage_prefers_top_level_total_tokens() {
    let value = serde_json::json!({
        "usage": {"total_tokens": 12345, "input_tokens": 1000, "output_tokens": 11345}
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 12345,
            input_tokens: Some(1000),
            output_tokens: Some(11345),
        })
    );
}

#[test]
fn extract_usage_sums_input_and_output_when_no_total() {
    let value = serde_json::json!({
        "usage": {"input_tokens": 700, "output_tokens": 300}
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 1000,
            input_tokens: Some(700),
            output_tokens: Some(300),
        })
    );
}

#[test]
fn extract_usage_includes_cache_tokens_in_total() {
    let value = serde_json::json!({
        "usage": {
            "input_tokens": 100,
            "output_tokens": 200,
            "cache_read_input_tokens": 50,
            "cache_creation_input_tokens": 25
        }
    });
    let usage = extract_usage(&value).expect("usage");
    assert_eq!(usage.total_tokens, 375);
    assert_eq!(usage.input_tokens, Some(100));
    assert_eq!(usage.output_tokens, Some(200));
}

#[test]
fn extract_usage_reads_token_usage_object() {
    let value = serde_json::json!({
        "token_usage": {"total_tokens": 4242, "input_tokens": 1000, "output_tokens": 3242}
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 4242,
            input_tokens: Some(1000),
            output_tokens: Some(3242),
        })
    );
}

#[test]
fn extract_usage_reads_top_level_fields() {
    let value = serde_json::json!({
        "total_tokens": 99,
        "input_tokens": 40,
        "output_tokens": 59
    });
    assert_eq!(
        extract_usage(&value),
        Some(TurnUsage {
            total_tokens: 99,
            input_tokens: Some(40),
            output_tokens: Some(59),
        })
    );
}

#[test]
fn extract_usage_returns_none_when_no_token_fields() {
    let value = serde_json::json!({"summary": "done"});
    assert_eq!(extract_usage(&value), None);
}

#[test]
fn extract_usage_returns_none_for_non_numeric_tokens() {
    let value = serde_json::json!({"usage": {"total_tokens": "many"}});
    assert_eq!(extract_usage(&value), None);
}

#[test]
fn extract_usage_handles_null_usage_object() {
    let value = serde_json::json!({"usage": null});
    assert_eq!(extract_usage(&value), None);
}

#[test]
fn extract_usage_parses_real_codex_turn_completed() {
    let value = serde_json::json!({
        "type": "turn.completed",
        "usage": {
            "input_tokens": 64511,
            "cached_input_tokens": 25216,
            "output_tokens": 398,
            "reasoning_output_tokens": 225
        }
    });
    let usage = extract_usage(&value).expect("usage");
    assert_eq!(usage.total_tokens, 64511 + 398 + 25216 + 225);
    assert_eq!(usage.input_tokens, Some(64511));
    assert_eq!(usage.output_tokens, Some(398));
}
