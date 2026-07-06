use serde_json::Value;

use crate::events::TurnUsage;

/// Best-effort 解析 Provider 事件中的 token 用量。
/// 找不到或字段非数字时返回 None，不抛错。
pub fn extract_usage(value: &Value) -> Option<TurnUsage> {
    if let Some(usage) = value.get("usage")
        && let Some(parsed) = parse_usage_object(usage)
    {
        return Some(parsed);
    }
    if let Some(usage) = value.get("token_usage")
        && let Some(parsed) = parse_usage_object(usage)
    {
        return Some(parsed);
    }
    parse_usage_object(value)
}

fn parse_usage_object(value: &Value) -> Option<TurnUsage> {
    let total = value.get("total_tokens").and_then(|v| v.as_i64());
    let input = value.get("input_tokens").and_then(|v| v.as_i64());
    let output = value.get("output_tokens").and_then(|v| v.as_i64());
    let cache_read = value
        .get("cache_read_input_tokens")
        .and_then(|v| v.as_i64())
        .unwrap_or(0);
    let cache_creation = value
        .get("cache_creation_input_tokens")
        .and_then(|v| v.as_i64())
        .unwrap_or(0);

    let total_tokens = match (total, input, output) {
        (Some(t), _, _) => Some(t),
        (None, Some(i), Some(o)) => Some(i + o + cache_read + cache_creation),
        _ => None,
    }?;

    Some(TurnUsage {
        total_tokens,
        input_tokens: input,
        output_tokens: output,
    })
}
