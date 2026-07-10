//! Redaction for values leaving the execution host over the ledger/WS path.
//!
//! Raw provider output is deliberately NOT redacted — see §3.5 of
//! `docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md`.
//! Only excerpts entering `execution_ledger_events` pass through here.

use std::sync::LazyLock;

use regex::Regex;

struct Pattern {
    reason: &'static str,
    regex: Regex,
}

static PATTERNS: LazyLock<Vec<Pattern>> = LazyLock::new(|| {
    let specs: &[(&str, &str)] = &[
        (
            "jwt",
            r"eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}",
        ),
        ("bearer", r"(?i)bearer\s+[A-Za-z0-9._~+/-]{16,}=*"),
        ("anthropic_key", r"sk-[A-Za-z0-9_-]{20,}"),
        ("github_token", r"gh[pousr]_[A-Za-z0-9]{20,}"),
        ("aws_access_key", r"(?:AKIA|ASIA)[A-Z0-9]{16}"),
    ];
    specs
        .iter()
        .map(|(reason, pattern)| Pattern {
            reason,
            regex: Regex::new(pattern).expect("redaction pattern must compile"),
        })
        .collect()
});

/// Replaces known credential shapes with `[REDACTED:{reason}]`.
pub fn redact(value: &str) -> String {
    let mut result = value.to_string();
    for pattern in PATTERNS.iter() {
        let placeholder = format!("[REDACTED:{}]", pattern.reason);
        result = pattern.regex.replace_all(&result, placeholder).into_owned();
    }
    result
}

/// Replaces the literal values of the provider process environment. Only values
/// long enough to be secrets are considered; short values such as `1` or `true`
/// would shred unrelated text.
pub fn redact_with_environment<'a, I>(value: &str, environment: I) -> String
where
    I: IntoIterator<Item = (&'a String, &'a String)>,
{
    let mut result = redact(value);
    for (name, secret) in environment {
        if secret.len() < 8 {
            continue;
        }
        if result.contains(secret.as_str()) {
            result = result.replace(secret.as_str(), &format!("[REDACTED:env:{name}]"));
        }
    }
    result
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn redacts_anthropic_style_key() {
        let redacted = redact("token is sk-ant-api03-abcdefghijklmnopqrstuvwxyz012345 done");
        assert!(redacted.contains("[REDACTED:anthropic_key]"));
        assert!(!redacted.contains("abcdefghijklmnopqrstuvwxyz"));
    }

    #[test]
    fn redacts_github_and_aws_shapes() {
        let redacted = redact("ghp_abcdefghijklmnopqrstuvwxyz0123 and AKIAIOSFODNN7EXAMPLE");
        assert!(redacted.contains("[REDACTED:github_token]"));
        assert!(redacted.contains("[REDACTED:aws_access_key]"));
    }

    #[test]
    fn redacts_bearer_and_jwt() {
        let jwt = "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U";
        let redacted = redact(&format!("Authorization: Bearer {jwt}"));
        assert!(redacted.contains("[REDACTED:"));
        assert!(!redacted.contains("dozjgNryP4J3jVmNHl0w5N"));
    }

    #[test]
    fn leaves_ordinary_text_untouched() {
        let text = "exit code 0; stdout was final-sanity-20260709014431";
        assert_eq!(redact(text), text);
    }

    #[test]
    fn redacts_environment_values_by_name() {
        let name = "MY_API_TOKEN".to_string();
        let secret = "supersecretvalue123".to_string();
        let environment = vec![(&name, &secret)];
        let redacted = redact_with_environment("echo supersecretvalue123", environment);
        assert_eq!(redacted, "echo [REDACTED:env:MY_API_TOKEN]");
    }

    #[test]
    fn ignores_short_environment_values() {
        let name = "DEBUG".to_string();
        let secret = "1".to_string();
        let environment = vec![(&name, &secret)];
        assert_eq!(
            redact_with_environment("step 1 of 3", environment),
            "step 1 of 3"
        );
    }
}
