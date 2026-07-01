//! Provider-specific MCP config materialization.
//!
//! Renders the effective MCP server payload (HTTP/streamable HTTP only) into provider config
//! projection files. Only env-var *names* are written, never token values. Task execution uses
//! `.superteam/mcp` under the task workspace so provider auth can keep using the host home.

use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result, anyhow, bail};

use crate::commands::payload::RuntimeMCPServerPayload;
use crate::workspace_files::atomic_write;

/// materialize_mcp_config writes provider config for the given servers and returns the files
/// written. An empty server list still validates the provider type but writes nothing.
pub fn materialize_mcp_config(
    agent_home_dir: &Path,
    provider_type: &str,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Vec<PathBuf>> {
    for server in servers {
        validate_server(server)?;
    }

    let target = match provider_type {
        "codex" => agent_home_dir.join(".codex").join("config.toml"),
        "claude-code" => agent_home_dir.join(".mcp.json"),
        "opencode" => agent_home_dir.join("opencode.json"),
        other => bail!("unsupported provider_type for mcp config: {other}"),
    };

    // Defense in depth: the target is built from fixed joins, but never let a config land
    // outside the agent home directory.
    if !target.starts_with(agent_home_dir) {
        bail!(
            "refusing to write mcp config outside agent home dir: {}",
            target.display()
        );
    }

    if servers.is_empty() {
        return Ok(Vec::new());
    }

    if let Some(parent) = target.parent() {
        fs::create_dir_all(parent)
            .with_context(|| format!("failed to create mcp config dir {}", parent.display()))?;
    }

    let content = match provider_type {
        "codex" => merge_codex_config(&target, servers)?,
        "claude-code" => render_claude_code_mcp_config(servers)?,
        "opencode" => merge_opencode_config(&target, servers)?,
        other => bail!("unsupported provider_type for mcp config: {other}"),
    };

    atomic_write(&target, content.as_bytes())?;
    Ok(vec![target])
}

/// materialize_task_mcp_config writes task-level MCP projection files under
/// `<workspace>/.superteam/mcp`. These files are capability projection inputs,
/// not provider auth homes.
pub fn materialize_task_mcp_config(
    workspace_path: &Path,
    provider_type: &str,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Option<PathBuf>> {
    for server in servers {
        validate_server(server)?;
    }

    let target = task_mcp_config_path(workspace_path, provider_type)?;
    if !target.starts_with(workspace_path) {
        bail!(
            "refusing to write task mcp config outside workspace: {}",
            target.display()
        );
    }
    if servers.is_empty() {
        return Ok(None);
    }

    if let Some(parent) = target.parent() {
        fs::create_dir_all(parent).with_context(|| {
            format!("failed to create task mcp config dir {}", parent.display())
        })?;
    }

    let content = match provider_type {
        "codex" => render_codex_mcp_config(servers)?,
        "claude-code" => render_claude_code_mcp_config(servers)?,
        "opencode" => render_opencode_task_mcp_config(servers)?,
        other => bail!("unsupported provider_type for mcp config: {other}"),
    };

    atomic_write(&target, content.as_bytes())?;
    Ok(Some(target))
}

pub fn task_mcp_config_path(workspace_path: &Path, provider_type: &str) -> Result<PathBuf> {
    let file_name = match provider_type {
        "codex" => "codex.toml",
        "claude-code" => "claude.mcp.json",
        "opencode" => "opencode.json",
        other => bail!("unsupported provider_type for mcp config: {other}"),
    };
    Ok(workspace_path
        .join(".superteam")
        .join("mcp")
        .join(file_name))
}

// ----------------------------------------------------------------------------
// Validation
// ----------------------------------------------------------------------------

fn validate_server(server: &RuntimeMCPServerPayload) -> Result<()> {
    if !is_valid_server_key(&server.server_key) {
        bail!("invalid mcp server_key: {}", server.server_key);
    }
    if server.transport != "http" && server.transport != "streamable_http" {
        bail!(
            "unsupported mcp transport for {}: {}",
            server.server_key,
            server.transport
        );
    }
    server_url(server)?;
    if let Some(name) = server.credential_env_var.as_deref() {
        if !name.is_empty() && !is_valid_env_name(name) {
            bail!(
                "invalid credential_env_var for {}: {}",
                server.server_key,
                name
            );
        }
    }
    for name in &server.required_env_vars {
        if !is_valid_env_name(name) {
            bail!(
                "invalid required env var for {}: {}",
                server.server_key,
                name
            );
        }
    }
    for value in server.headers_env.values() {
        if !is_valid_env_name(value) {
            bail!(
                "invalid headers_env value for {}: {}",
                server.server_key,
                value
            );
        }
    }
    Ok(())
}

fn server_url(server: &RuntimeMCPServerPayload) -> Result<&str> {
    server
        .url
        .as_deref()
        .filter(|url| !url.is_empty())
        .ok_or_else(|| {
            anyhow!(
                "mcp server {} requires a url for http transport",
                server.server_key
            )
        })
}

fn is_valid_server_key(key: &str) -> bool {
    !key.is_empty()
        && key
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '_' || c == '-')
}

fn is_valid_env_name(name: &str) -> bool {
    let mut chars = name.chars();
    match chars.next() {
        Some(c) if c.is_ascii_alphabetic() || c == '_' => {}
        _ => return false,
    }
    chars.all(|c| c.is_ascii_alphanumeric() || c == '_')
}

fn is_bearer_env(server: &RuntimeMCPServerPayload) -> bool {
    server.auth_strategy.as_deref() == Some("bearer_env")
        && server
            .credential_env_var
            .as_deref()
            .map(|c| !c.is_empty())
            .unwrap_or(false)
}

// ----------------------------------------------------------------------------
// Codex (config.toml, read-merge-preserve)
// ----------------------------------------------------------------------------

/// render_codex_mcp_config renders a fresh Codex config containing only the `mcp_servers` table.
pub fn render_codex_mcp_config(servers: &[RuntimeMCPServerPayload]) -> Result<String> {
    let mut root = toml::value::Table::new();
    root.insert(
        "mcp_servers".to_string(),
        toml::Value::Table(codex_mcp_servers_table(servers)?),
    );
    toml::to_string_pretty(&toml::Value::Table(root)).context("serialize codex mcp config")
}

fn codex_mcp_servers_table(servers: &[RuntimeMCPServerPayload]) -> Result<toml::value::Table> {
    let mut table = toml::value::Table::new();
    for server in servers {
        let mut entry = toml::value::Table::new();
        entry.insert(
            "url".to_string(),
            toml::Value::String(server_url(server)?.to_string()),
        );
        if is_bearer_env(server) {
            entry.insert(
                "bearer_token_env_var".to_string(),
                toml::Value::String(server.credential_env_var.clone().unwrap_or_default()),
            );
        }
        if !server.headers_env.is_empty() {
            let mut headers = toml::value::Table::new();
            for (header, env_name) in &server.headers_env {
                headers.insert(header.clone(), toml::Value::String(env_name.clone()));
            }
            entry.insert("env_http_headers".to_string(), toml::Value::Table(headers));
        }
        table.insert(server.server_key.clone(), toml::Value::Table(entry));
    }
    Ok(table)
}

fn merge_codex_config(target: &Path, servers: &[RuntimeMCPServerPayload]) -> Result<String> {
    let mcp_servers = toml::Value::Table(codex_mcp_servers_table(servers)?);
    if target.exists() {
        let existing = fs::read_to_string(target)
            .with_context(|| format!("read existing codex config {}", target.display()))?;
        let mut doc: toml::Value = toml::from_str(&existing)
            .with_context(|| format!("parse existing codex config {}", target.display()))?;
        let root = doc
            .as_table_mut()
            .ok_or_else(|| anyhow!("codex config {} is not a table", target.display()))?;
        root.insert("mcp_servers".to_string(), mcp_servers);
        return toml::to_string_pretty(&doc).context("serialize merged codex config");
    }
    render_codex_mcp_config(servers)
}

// ----------------------------------------------------------------------------
// Claude Code (.mcp.json, MCP-only file)
// ----------------------------------------------------------------------------

pub fn render_claude_code_mcp_config(servers: &[RuntimeMCPServerPayload]) -> Result<String> {
    let mut entries = serde_json::Map::new();
    for server in servers {
        let mut entry = serde_json::Map::new();
        entry.insert(
            "type".to_string(),
            serde_json::Value::String("http".to_string()),
        );
        entry.insert(
            "url".to_string(),
            serde_json::Value::String(server_url(server)?.to_string()),
        );
        let headers = claude_code_headers(server);
        if !headers.is_empty() {
            entry.insert("headers".to_string(), serde_json::Value::Object(headers));
        }
        entries.insert(server.server_key.clone(), serde_json::Value::Object(entry));
    }
    let mut root = serde_json::Map::new();
    root.insert("mcpServers".to_string(), serde_json::Value::Object(entries));
    serde_json::to_string_pretty(&serde_json::Value::Object(root))
        .context("serialize claude code mcp config")
}

fn claude_code_headers(
    server: &RuntimeMCPServerPayload,
) -> serde_json::Map<String, serde_json::Value> {
    let mut headers = serde_json::Map::new();
    if is_bearer_env(server) {
        let env = server.credential_env_var.clone().unwrap_or_default();
        headers.insert(
            "Authorization".to_string(),
            serde_json::Value::String(format!("Bearer ${{{env}}}")),
        );
    }
    for (header, env_name) in &server.headers_env {
        headers.insert(
            header.clone(),
            serde_json::Value::String(format!("${{{env_name}}}")),
        );
    }
    headers
}

// ----------------------------------------------------------------------------
// OpenCode (opencode.json, read-merge-preserve `mcp` key)
// ----------------------------------------------------------------------------

pub fn render_opencode_mcp_value(servers: &[RuntimeMCPServerPayload]) -> Result<serde_json::Value> {
    let mut entries = serde_json::Map::new();
    for server in servers {
        let mut entry = serde_json::Map::new();
        entry.insert(
            "type".to_string(),
            serde_json::Value::String("remote".to_string()),
        );
        entry.insert(
            "url".to_string(),
            serde_json::Value::String(server_url(server)?.to_string()),
        );
        entry.insert("enabled".to_string(), serde_json::Value::Bool(true));
        let headers = opencode_headers(server);
        if !headers.is_empty() {
            entry.insert("headers".to_string(), serde_json::Value::Object(headers));
        }
        entries.insert(server.server_key.clone(), serde_json::Value::Object(entry));
    }
    Ok(serde_json::Value::Object(entries))
}

fn opencode_headers(
    server: &RuntimeMCPServerPayload,
) -> serde_json::Map<String, serde_json::Value> {
    let mut headers = serde_json::Map::new();
    if is_bearer_env(server) {
        let env = server.credential_env_var.clone().unwrap_or_default();
        headers.insert(
            "Authorization".to_string(),
            serde_json::Value::String(format!("Bearer {{env:{env}}}")),
        );
    }
    for (header, env_name) in &server.headers_env {
        headers.insert(
            header.clone(),
            serde_json::Value::String(format!("{{env:{env_name}}}")),
        );
    }
    headers
}

fn merge_opencode_config(target: &Path, servers: &[RuntimeMCPServerPayload]) -> Result<String> {
    let mcp = render_opencode_mcp_value(servers)?;
    if target.exists() {
        let existing = fs::read_to_string(target)
            .with_context(|| format!("read existing opencode config {}", target.display()))?;
        let mut doc: serde_json::Value = serde_json::from_str(&existing)
            .with_context(|| format!("parse existing opencode config {}", target.display()))?;
        let root = doc
            .as_object_mut()
            .ok_or_else(|| anyhow!("opencode config {} is not a json object", target.display()))?;
        root.insert("mcp".to_string(), mcp);
        return serde_json::to_string_pretty(&doc).context("serialize merged opencode config");
    }
    let mut root = serde_json::Map::new();
    root.insert("mcp".to_string(), mcp);
    serde_json::to_string_pretty(&serde_json::Value::Object(root))
        .context("serialize opencode config")
}

fn render_opencode_task_mcp_config(servers: &[RuntimeMCPServerPayload]) -> Result<String> {
    let mut root = serde_json::Map::new();
    root.insert("mcp".to_string(), render_opencode_mcp_value(servers)?);
    serde_json::to_string_pretty(&serde_json::Value::Object(root))
        .context("serialize opencode task mcp config")
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::BTreeMap;

    fn github_server() -> RuntimeMCPServerPayload {
        RuntimeMCPServerPayload {
            server_id: "mcp-1".to_string(),
            server_key: "github".to_string(),
            name: Some("GitHub MCP".to_string()),
            transport: "streamable_http".to_string(),
            url: Some("https://api.githubcopilot.com/mcp/".to_string()),
            auth_strategy: Some("bearer_env".to_string()),
            credential_env_var: Some("GITHUB_TOKEN".to_string()),
            required_env_vars: vec!["GITHUB_TOKEN".to_string()],
            headers_env: BTreeMap::new(),
            source_scope: Some("employee".to_string()),
            config_ref: None,
            permission_scope: serde_json::json!({}),
        }
    }

    #[test]
    fn renders_codex_remote_mcp_with_bearer_env() {
        let servers = vec![github_server()];
        let rendered = render_codex_mcp_config(&servers).expect("render codex mcp");

        assert!(rendered.contains("[mcp_servers.github]"), "got: {rendered}");
        assert!(rendered.contains("url = \"https://api.githubcopilot.com/mcp/\""));
        assert!(rendered.contains("bearer_token_env_var = \"GITHUB_TOKEN\""));
        // never write a token value
        assert!(!rendered.to_lowercase().contains("bearer "));
    }

    #[test]
    fn renders_claude_code_http_mcp() {
        let rendered =
            render_claude_code_mcp_config(&[github_server()]).expect("render claude mcp");
        assert!(rendered.contains("\"mcpServers\""));
        assert!(rendered.contains("\"type\": \"http\""));
        assert!(rendered.contains("Bearer ${GITHUB_TOKEN}"));
    }

    #[test]
    fn opencode_merge_preserves_unrelated_keys() {
        let dir = tempfile::tempdir().unwrap();
        let target = dir.path().join("opencode.json");
        std::fs::write(&target, r#"{"theme":"dark","mcp":{"old":{}}}"#).unwrap();

        let merged = merge_opencode_config(&target, &[github_server()]).expect("merge opencode");
        let value: serde_json::Value = serde_json::from_str(&merged).unwrap();
        assert_eq!(value["theme"], "dark");
        assert!(value["mcp"].get("github").is_some());
        assert!(
            value["mcp"].get("old").is_none(),
            "mcp key should be replaced"
        );
    }

    #[test]
    fn materialize_writes_codex_config_under_agent_home() {
        let dir = tempfile::tempdir().unwrap();
        let written = materialize_mcp_config(dir.path(), "codex", &[github_server()]).unwrap();
        assert_eq!(written.len(), 1);
        let content = std::fs::read_to_string(&written[0]).unwrap();
        assert!(content.contains("[mcp_servers.github]"));
        assert!(written[0].ends_with(".codex/config.toml"));
    }

    #[test]
    fn materialize_task_mcp_config_writes_projection_under_workspace() {
        let dir = tempfile::tempdir().unwrap();
        let workspace = dir.path().join("workspaces/project/task/attempt");

        let written =
            materialize_task_mcp_config(&workspace, "claude-code", &[github_server()]).unwrap();

        let written = written.expect("task mcp config path");
        assert!(written.ends_with(".superteam/mcp/claude.mcp.json"));
        assert!(written.starts_with(&workspace));
        let content = std::fs::read_to_string(written).unwrap();
        assert!(content.contains("\"mcpServers\""));
    }

    #[test]
    fn rejects_non_http_transport() {
        let mut server = github_server();
        server.transport = "stdio".to_string();
        let err = materialize_mcp_config(tempfile::tempdir().unwrap().path(), "codex", &[server])
            .unwrap_err();
        assert!(err.to_string().contains("unsupported mcp transport"));
    }

    #[test]
    fn rejects_unknown_provider() {
        let err = materialize_mcp_config(
            tempfile::tempdir().unwrap().path(),
            "unknown",
            &[github_server()],
        )
        .unwrap_err();
        assert!(err.to_string().contains("unsupported provider_type"));
    }
}
