use crate::config::{ProviderSection, RuntimeConfig};
use crate::providers::ProviderAdapter;
use crate::providers::claude::ClaudeProvider;
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
        CODEX_PROVIDER_TYPE => Err(anyhow::anyhow!("Codex provider is not implemented yet")),
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
