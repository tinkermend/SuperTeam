use crate::config::{ProviderSection, RuntimeConfig};
use crate::providers::ProviderAdapter;
use crate::providers::claude::ClaudeProvider;
use crate::providers::codex::CodexProvider;
use crate::providers::opencode::OpenCodeProvider;

pub const CLAUDE_CODE_PROVIDER_TYPE: &str = "claude-code";
pub const OPENCODE_PROVIDER_TYPE: &str = "opencode";
pub const CODEX_PROVIDER_TYPE: &str = "codex";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ProviderDescriptor {
    pub provider_type: &'static str,
    pub provider_kind: &'static str,
    pub health_kind: &'static str,
}

pub const PROVIDERS: &[ProviderDescriptor] = &[
    ProviderDescriptor {
        provider_type: CLAUDE_CODE_PROVIDER_TYPE,
        provider_kind: "claude",
        health_kind: "claude",
    },
    ProviderDescriptor {
        provider_type: OPENCODE_PROVIDER_TYPE,
        provider_kind: "opencode",
        health_kind: "opencode",
    },
    ProviderDescriptor {
        provider_type: CODEX_PROVIDER_TYPE,
        provider_kind: "codex",
        health_kind: "codex",
    },
];

pub fn configured_provider_descriptors() -> &'static [ProviderDescriptor] {
    PROVIDERS
}

pub fn provider_descriptor(provider_type: &str) -> Option<&'static ProviderDescriptor> {
    PROVIDERS
        .iter()
        .find(|descriptor| descriptor.provider_type == provider_type)
}

pub fn provider_kind(provider_type: &str) -> &'static str {
    provider_descriptor(provider_type)
        .map(|descriptor| descriptor.provider_kind)
        .unwrap_or("unsupported")
}

/// Resolve a registry `provider_type` (`claude-code` / `opencode` / `codex`) from
/// either a registry type or a short `provider_kind` (`claude` / …).
///
/// ErrorEnvelope and writeback metadata must use this (or an already-stored
/// registry type), never the short kind alone (spec §13 #7).
pub fn canonical_provider_type(name: &str) -> &str {
    let n = name.trim();
    if n.is_empty() {
        return n;
    }
    if let Some(descriptor) = PROVIDERS.iter().find(|d| d.provider_type == n) {
        return descriptor.provider_type;
    }
    if let Some(descriptor) = PROVIDERS.iter().find(|d| d.provider_kind == n) {
        return descriptor.provider_type;
    }
    n
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn canonical_provider_type_maps_kind_and_type() {
        assert_eq!(canonical_provider_type("claude"), CLAUDE_CODE_PROVIDER_TYPE);
        assert_eq!(
            canonical_provider_type("claude-code"),
            CLAUDE_CODE_PROVIDER_TYPE
        );
        assert_eq!(canonical_provider_type("opencode"), OPENCODE_PROVIDER_TYPE);
        assert_eq!(canonical_provider_type("codex"), CODEX_PROVIDER_TYPE);
        assert_eq!(canonical_provider_type("unknown-x"), "unknown-x");
    }
}

/// Static capability matrix for a registered provider (Phase 2). Values are
/// intentional honesty flags — not product promises of feature parity.
#[derive(Debug, Clone, PartialEq, Eq, serde::Serialize)]
pub struct ProviderCapability {
    pub schema_version: &'static str,
    pub provider_type: &'static str,
    pub session_resume: bool,
    pub stream_text: bool,
    pub stream_tools: bool,
    pub stream_usage: bool,
    pub structured_error: bool,
    pub mcp_native: bool,
    pub mcp_isolation: &'static str,
}

pub fn capability(provider_type: &str) -> Option<ProviderCapability> {
    match provider_type {
        CLAUDE_CODE_PROVIDER_TYPE => Some(ProviderCapability {
            schema_version: "provider.capability.v1",
            provider_type: CLAUDE_CODE_PROVIDER_TYPE,
            session_resume: true,
            stream_text: true,
            stream_tools: true,
            stream_usage: true,
            // After Phase 1 is_error fix: can map some structured failures.
            structured_error: true,
            mcp_native: true,
            mcp_isolation: "argv",
        }),
        OPENCODE_PROVIDER_TYPE => Some(ProviderCapability {
            schema_version: "provider.capability.v1",
            provider_type: OPENCODE_PROVIDER_TYPE,
            session_resume: true,
            stream_text: true,
            stream_tools: false,
            stream_usage: true,
            structured_error: false,
            mcp_native: true,
            mcp_isolation: "home_file",
        }),
        CODEX_PROVIDER_TYPE => Some(ProviderCapability {
            schema_version: "provider.capability.v1",
            provider_type: CODEX_PROVIDER_TYPE,
            session_resume: true,
            stream_text: true,
            stream_tools: false,
            stream_usage: true,
            structured_error: false,
            mcp_native: true,
            mcp_isolation: "home_file",
        }),
        _ => None,
    }
}

pub fn provider_section<'a>(
    config: &'a RuntimeConfig,
    provider_type: &str,
) -> Option<&'a ProviderSection> {
    match provider_type {
        CLAUDE_CODE_PROVIDER_TYPE => Some(&config.providers.claude_code),
        OPENCODE_PROVIDER_TYPE => Some(&config.providers.opencode),
        CODEX_PROVIDER_TYPE => Some(&config.providers.codex),
        _ => None,
    }
}

pub fn supported_provider_types(config: &RuntimeConfig) -> Vec<String> {
    PROVIDERS
        .iter()
        .filter_map(|descriptor| {
            let section = provider_section(config, descriptor.provider_type)?;
            section
                .enabled
                .then(|| descriptor.provider_type.to_string())
        })
        .collect()
}

pub fn select_provider(
    config: &RuntimeConfig,
    provider_type: &str,
) -> anyhow::Result<Box<dyn ProviderAdapter>> {
    let section = provider_section(config, provider_type)
        .ok_or_else(|| anyhow::anyhow!("unsupported provider_type: {provider_type}"))?;
    if !section.enabled {
        return Err(anyhow::anyhow!(
            "{} provider is disabled",
            provider_type_label(provider_type)
        ));
    }

    match provider_type {
        CLAUDE_CODE_PROVIDER_TYPE => Ok(Box::new(ClaudeProvider::new(section.binary_path.clone()))),
        OPENCODE_PROVIDER_TYPE => Ok(Box::new(OpenCodeProvider::new(section.binary_path.clone()))),
        CODEX_PROVIDER_TYPE => Ok(Box::new(CodexProvider::new(section.binary_path.clone()))),
        _ => Err(anyhow::anyhow!(
            "unsupported provider_type: {provider_type}"
        )),
    }
}

fn provider_type_label(provider_type: &str) -> &'static str {
    match provider_type {
        CLAUDE_CODE_PROVIDER_TYPE => "Claude Code",
        OPENCODE_PROVIDER_TYPE => "OpenCode",
        CODEX_PROVIDER_TYPE => "Codex",
        _ => "Unknown",
    }
}
