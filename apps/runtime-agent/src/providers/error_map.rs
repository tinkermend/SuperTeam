//! Structured provider error mapping (provider semantic unification Phase 1).
//!
//! Single table: `code → (family, retryable)`. Family decision must not re-parse
//! free-form English strings on the main path. Message-based classification is
//! only a fallback for legacy call sites that still only have a string.

use serde::{Deserialize, Serialize};

use crate::events::{ErrorEnvelope, ErrorEvidenceRef, ErrorNative};

/// Stable machine codes (UPPER_SNAKE). Closed set for v1.
pub mod code {
    pub const PROVIDER_EXIT_NON_ZERO: &str = "PROVIDER_EXIT_NON_ZERO";
    pub const PROVIDER_SPAWN_FAILED: &str = "PROVIDER_SPAWN_FAILED";
    pub const PROVIDER_PROTOCOL_ERROR: &str = "PROVIDER_PROTOCOL_ERROR";
    pub const PROVIDER_NO_TERMINAL_EVENT: &str = "PROVIDER_NO_TERMINAL_EVENT";
    pub const RATE_LIMIT: &str = "RATE_LIMIT";
    pub const AUTH_FAILED: &str = "AUTH_FAILED";
    pub const TIMEOUT: &str = "TIMEOUT";
    pub const BUDGET_FUSE: &str = "BUDGET_FUSE";
    pub const CANCELLED: &str = "CANCELLED";
    pub const WORKSPACE_INVALID: &str = "WORKSPACE_INVALID";
    pub const WAITING_HUMAN: &str = "WAITING_HUMAN";
    pub const UNKNOWN: &str = "UNKNOWN";
}

/// Open-string families written to `project_task_attempts.failure_family`.
/// Runtime production subset; CP owns additional families.
pub mod family {
    pub const BUDGET_FUSE: &str = "budget_fuse";
    pub const BUSINESS_CANCELLED: &str = "business_cancelled";
    pub const INVALID_CONTRACT: &str = "invalid_contract";
    pub const TIMEOUT: &str = "timeout";
    pub const TRANSIENT_PROVIDER: &str = "transient_provider";
    pub const NON_RETRYABLE_EXECUTION: &str = "non_retryable_execution";
    pub const PROVIDER_CONFIGURATION: &str = "provider_configuration";
}

pub const ERROR_SCHEMA_VERSION: &str = "provider.error.v1";

/// Map a stable error code to (family, retryable). Single source of truth.
pub fn map_code(code: &str) -> (&'static str, bool) {
    match code {
        code::PROVIDER_EXIT_NON_ZERO => (family::TRANSIENT_PROVIDER, true),
        code::PROVIDER_SPAWN_FAILED => (family::PROVIDER_CONFIGURATION, false),
        code::PROVIDER_PROTOCOL_ERROR => (family::NON_RETRYABLE_EXECUTION, false),
        // Schema drift / empty run. Retry was the original §13 #11 decision, but
        // real-chain E2E showed retries never re-reach the runtime (CP creates no
        // runtime command for the requeued attempt), so retrying only burned
        // ~12min and relabelled the task `runtime_recovery`, hiding this code.
        // Held at non-retryable until 派发再送 is fixed; flip back then.
        code::PROVIDER_NO_TERMINAL_EVENT => (family::TRANSIENT_PROVIDER, false),
        code::RATE_LIMIT => (family::TRANSIENT_PROVIDER, true),
        code::AUTH_FAILED => (family::PROVIDER_CONFIGURATION, false),
        code::TIMEOUT => (family::TIMEOUT, true),
        code::BUDGET_FUSE => (family::BUDGET_FUSE, false),
        code::CANCELLED => (family::BUSINESS_CANCELLED, false),
        code::WORKSPACE_INVALID => (family::INVALID_CONTRACT, false),
        code::WAITING_HUMAN => (family::INVALID_CONTRACT, false),
        _ => (family::NON_RETRYABLE_EXECUTION, false),
    }
}

#[derive(Debug, Clone)]
pub struct EnvelopeBuild {
    pub code: String,
    pub message: String,
    pub provider_type: String,
    pub native: Option<ErrorNative>,
    pub evidence_refs: Vec<ErrorEvidenceRef>,
}

/// Build a complete ErrorEnvelope; family/retryable always come from `map_code`.
/// `provider_type` is canonicalized to the registry value (`claude-code` / …)
/// so short kinds like `claude` never leak into L3 (spec §13 #7).
pub fn build_envelope(build: EnvelopeBuild) -> ErrorEnvelope {
    let (family, retryable) = map_code(&build.code);
    ErrorEnvelope {
        schema_version: ERROR_SCHEMA_VERSION.to_string(),
        code: build.code,
        family: family.to_string(),
        retryable,
        message: build.message,
        provider_type: crate::providers::catalog::canonical_provider_type(&build.provider_type)
            .to_string(),
        native: build.native,
        evidence_refs: if build.evidence_refs.is_empty() {
            None
        } else {
            Some(build.evidence_refs)
        },
    }
}

pub fn envelope_for_code(
    code: &str,
    message: impl Into<String>,
    provider_type: impl Into<String>,
) -> ErrorEnvelope {
    build_envelope(EnvelopeBuild {
        code: code.to_string(),
        message: message.into(),
        provider_type: provider_type.into(),
        native: None,
        evidence_refs: Vec::new(),
    })
}

/// Refine a non-zero exit into a more specific code when stderr/message allows.
/// Fallback stays PROVIDER_EXIT_NON_ZERO. Keyword scan is confined here.
pub fn refine_exit_code(message: &str) -> &'static str {
    let lower = message.to_ascii_lowercase();
    if lower.contains("rate limit")
        || lower.contains("rate_limit")
        || lower.contains("too many requests")
        || lower.contains("overloaded")
    {
        return code::RATE_LIMIT;
    }
    if lower.contains("auth") && (lower.contains("fail") || lower.contains("unauthor") || lower.contains("401") || lower.contains("403"))
        || lower.contains("invalid api key")
        || lower.contains("authentication")
    {
        return code::AUTH_FAILED;
    }
    if lower.contains("timeout") || lower.contains("timed out") {
        return code::TIMEOUT;
    }
    code::PROVIDER_EXIT_NON_ZERO
}

/// Infer a code from a free-form failure string. **Deprecated main path** —
/// prefer emitting a code at the failure site. Kept for dual-read / legacy
/// call sites until they all construct envelopes directly.
pub fn classify_message_fallback(message: &str) -> &'static str {
    let normalized = message.to_ascii_lowercase();
    if normalized.contains("wall_clock_exceeded") || normalized.contains("budget") {
        return code::BUDGET_FUSE;
    }
    if normalized.contains("operator cancelled")
        || (normalized.contains("cancelled") && !normalized.contains("canceling"))
    {
        // Keep "cancelled" broad to match historical classification, but avoid
        // matching unrelated "canceling" noise when possible.
        if normalized.contains("operator cancelled") || normalized.contains("cancelled") {
            return code::CANCELLED;
        }
    }
    if normalized.contains("content_hash mismatch")
        || normalized.contains("workspace_sync")
        || normalized.contains("workspace sync")
        || normalized.contains("workspace_sync_failed")
    {
        return code::WORKSPACE_INVALID;
    }
    if normalized.contains("timeout") || normalized.contains("timed out") {
        return code::TIMEOUT;
    }
    if normalized.contains("failed to spawn")
        || normalized.contains("no such file or directory")
            && (normalized.contains("claude")
                || normalized.contains("opencode")
                || normalized.contains("codex"))
    {
        return code::PROVIDER_SPAWN_FAILED;
    }
    if normalized.contains("provider exited without a terminal event") {
        return code::PROVIDER_NO_TERMINAL_EVENT;
    }
    if normalized.contains("rate limit")
        || normalized.contains("overloaded")
        || normalized.contains("api error")
        || normalized.contains("unavailable")
        || normalized.contains("claude exited")
        || normalized.contains("opencode exited")
        || normalized.contains("codex exited")
    {
        return refine_exit_code(message);
    }
    if normalized.contains("turn.failed")
        || normalized.contains("protocol")
        || normalized.contains("parse")
    {
        return code::PROVIDER_PROTOCOL_ERROR;
    }
    code::UNKNOWN
}

/// Prefer structured envelope; fall back to message classification.
pub fn classify_provider_error(
    envelope: Option<&ErrorEnvelope>,
    message: &str,
    provider_type: &str,
) -> ErrorEnvelope {
    if let Some(envelope) = envelope {
        return envelope.clone();
    }
    let code = classify_message_fallback(message);
    envelope_for_code(code, message.trim(), provider_type)
}

/// Stream/item error carrying a structured envelope so callers need not re-parse.
#[derive(Debug, Clone)]
pub struct ProviderStreamError {
    pub envelope: ErrorEnvelope,
}

impl ProviderStreamError {
    pub fn new(envelope: ErrorEnvelope) -> Self {
        Self { envelope }
    }

    pub fn from_code(
        code: &str,
        message: impl Into<String>,
        provider_type: impl Into<String>,
    ) -> Self {
        Self {
            envelope: envelope_for_code(code, message, provider_type),
        }
    }
}

impl std::fmt::Display for ProviderStreamError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.envelope.message)
    }
}

impl std::error::Error for ProviderStreamError {}

/// Extract envelope from anyhow, or classify message as last resort.
pub fn envelope_from_anyhow(error: &anyhow::Error, provider_type: &str) -> ErrorEnvelope {
    if let Some(stream_error) = error.downcast_ref::<ProviderStreamError>() {
        return stream_error.envelope.clone();
    }
    classify_provider_error(None, &error.to_string(), provider_type)
}

/// Optional JSON object form for writeback payload / metadata (Phase 1 optional).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorEnvelopeJson {
    #[serde(flatten)]
    pub envelope: ErrorEnvelope,
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashSet;
    use std::fs;
    use std::path::PathBuf;

    /// Runtime production family::* set. Must stay ⊆ failure-family.json
    /// known_values (provider semantic unification §5.3; mirrors CP vocab test).
    fn runtime_family_constants() -> &'static [&'static str] {
        &[
            family::BUDGET_FUSE,
            family::BUSINESS_CANCELLED,
            family::INVALID_CONTRACT,
            family::TIMEOUT,
            family::TRANSIENT_PROVIDER,
            family::NON_RETRYABLE_EXECUTION,
            family::PROVIDER_CONFIGURATION,
        ]
    }

    fn load_failure_family_known_values() -> HashSet<String> {
        let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
            .join("../../contracts/provider/schemas/failure-family.json");
        let raw = fs::read_to_string(&path)
            .unwrap_or_else(|e| panic!("read {}: {e}", path.display()));
        let doc: serde_json::Value =
            serde_json::from_str(&raw).unwrap_or_else(|e| panic!("parse failure-family.json: {e}"));
        let values = doc
            .get("known_values")
            .and_then(|v| v.as_array())
            .unwrap_or_else(|| panic!("failure-family.json missing known_values array"));
        values
            .iter()
            .filter_map(|v| v.as_str().map(str::to_string))
            .collect()
    }

    #[test]
    fn runtime_family_constants_subset_of_shared_vocab() {
        let known = load_failure_family_known_values();
        assert!(!known.is_empty(), "failure-family.json known_values empty");
        for family in runtime_family_constants() {
            assert!(
                known.contains(*family),
                "Runtime family::{family:?} missing from contracts/provider/schemas/failure-family.json known_values"
            );
        }
    }

    #[test]
    fn map_code_table_covers_v1_codes() {
        assert_eq!(
            map_code(code::BUDGET_FUSE),
            (family::BUDGET_FUSE, false)
        );
        // Non-retryable on purpose: see map_code comment (retry never re-reaches
        // the runtime today, so it would only mask this code behind
        // runtime_recovery). Flip to `true` together with the dispatch fix.
        assert_eq!(
            map_code(code::PROVIDER_NO_TERMINAL_EVENT),
            (family::TRANSIENT_PROVIDER, false)
        );
        assert_eq!(
            map_code(code::PROVIDER_SPAWN_FAILED),
            (family::PROVIDER_CONFIGURATION, false)
        );
        assert_eq!(map_code(code::RATE_LIMIT), (family::TRANSIENT_PROVIDER, true));
        assert_eq!(
            map_code(code::AUTH_FAILED),
            (family::PROVIDER_CONFIGURATION, false)
        );
        assert_eq!(map_code(code::TIMEOUT), (family::TIMEOUT, true));
        assert_eq!(
            map_code(code::CANCELLED),
            (family::BUSINESS_CANCELLED, false)
        );
        assert_eq!(
            map_code(code::WORKSPACE_INVALID),
            (family::INVALID_CONTRACT, false)
        );
        assert_eq!(
            map_code(code::UNKNOWN),
            (family::NON_RETRYABLE_EXECUTION, false)
        );
    }

    #[test]
    fn build_envelope_is_source_of_family_and_retryable() {
        let env = envelope_for_code(code::BUDGET_FUSE, "wall_clock_exceeded", "claude-code");
        assert_eq!(env.family, family::BUDGET_FUSE);
        assert_eq!(env.provider_type, "claude-code");
        // short kind must canonicalize
        let short = envelope_for_code(code::RATE_LIMIT, "rate limit", "claude");
        assert_eq!(short.provider_type, "claude-code");
        assert!(!env.retryable);
        assert_eq!(env.code, code::BUDGET_FUSE);
        assert_eq!(env.schema_version, ERROR_SCHEMA_VERSION);
    }

    #[test]
    fn refine_exit_detects_rate_limit() {
        assert_eq!(
            refine_exit_code("claude exited with status 1: rate limit exceeded"),
            code::RATE_LIMIT
        );
    }

    #[test]
    fn fallback_classifies_historical_messages() {
        assert_eq!(
            classify_message_fallback("wall_clock_exceeded"),
            code::BUDGET_FUSE
        );
        assert_eq!(
            classify_message_fallback("operator cancelled"),
            code::CANCELLED
        );
        assert_eq!(
            classify_message_fallback("provider exited without a terminal event"),
            code::PROVIDER_NO_TERMINAL_EVENT
        );
        assert_eq!(
            classify_message_fallback("failed to spawn claude: No such file"),
            code::PROVIDER_SPAWN_FAILED
        );
        assert_eq!(
            classify_message_fallback("content_hash mismatch for workspace"),
            code::WORKSPACE_INVALID
        );
    }

    #[test]
    fn classify_provider_error_prefers_envelope() {
        let existing = envelope_for_code(code::TIMEOUT, "timed out", "opencode");
        let got = classify_provider_error(
            Some(&existing),
            "wall_clock_exceeded",
            "opencode",
        );
        assert_eq!(got.code, code::TIMEOUT);
        assert_eq!(got.family, family::TIMEOUT);
    }

    #[test]
    fn provider_stream_error_display_is_message() {
        let err = ProviderStreamError::from_code(
            code::PROVIDER_EXIT_NON_ZERO,
            "claude exited with status 1",
            "claude-code",
        );
        assert_eq!(err.to_string(), "claude exited with status 1");
        assert_eq!(err.envelope.family, family::TRANSIENT_PROVIDER);
    }
}
