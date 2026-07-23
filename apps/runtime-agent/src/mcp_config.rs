//! Provider-specific MCP config materialization.
//!
//! Renders the effective MCP server payload (HTTP/streamable HTTP only) into provider config
//! projection files. Only env-var *names* are written, never token values. Task execution uses
//! `.superteam/mcp` under the task workspace so provider auth can keep using the host home.

use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result, anyhow, bail};
use serde::{Deserialize, Serialize};

use crate::commands::payload::RuntimeMCPServerPayload;
use crate::workspace_files::atomic_write;

/// home_mcp_config_target resolves the provider's home-dir MCP config path and defends against
/// the target ever landing outside the agent home directory.
fn home_mcp_config_target(agent_home_dir: &Path, provider_type: &str) -> Result<PathBuf> {
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

    Ok(target)
}

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

    let target = home_mcp_config_target(agent_home_dir, provider_type)?;

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

// ----------------------------------------------------------------------------
// Session-scoped injection + rollback (manifest-backed)
// ----------------------------------------------------------------------------

const SESSION_MANIFEST_DIR: &str = ".superteam";
const SESSION_MANIFEST_FILE: &str = "mcp-session-manifest.json";

#[derive(Debug, Serialize, Deserialize)]
struct McpSessionManifest {
    entries: Vec<McpManifestEntry>,
}

#[derive(Debug, Serialize, Deserialize)]
struct McpManifestEntry {
    path: PathBuf,
    existed: bool,
    #[serde(default)]
    previous_content: Option<String>,
}

pub fn manifest_path(agent_home_dir: &Path) -> PathBuf {
    agent_home_dir
        .join(SESSION_MANIFEST_DIR)
        .join(SESSION_MANIFEST_FILE)
}

/// Session-scoped home-dir MCP injection: roll back any residual manifest first
/// (crash fallback), snapshot the target file, persist the manifest, then materialize.
/// The manifest is written BEFORE the config: if materialization fails midway the
/// next session still restores the pre-session state.
pub fn inject_session_mcp_config(
    agent_home_dir: &Path,
    provider_type: &str,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Vec<PathBuf>> {
    rollback_session_mcp_config(agent_home_dir)
        .with_context(|| "rollback residual mcp session manifest before injecting")?;
    let target = home_mcp_config_target(agent_home_dir, provider_type)?;
    if servers.is_empty() {
        return Ok(Vec::new());
    }
    let existed = target.exists();
    let previous_content = if existed {
        Some(
            fs::read_to_string(&target)
                .with_context(|| format!("snapshot existing mcp config {}", target.display()))?,
        )
    } else {
        None
    };
    let manifest = McpSessionManifest {
        entries: vec![McpManifestEntry {
            path: target.clone(),
            existed,
            previous_content,
        }],
    };
    let mpath = manifest_path(agent_home_dir);
    if let Some(parent) = mpath.parent() {
        fs::create_dir_all(parent)
            .with_context(|| format!("create manifest dir {}", parent.display()))?;
    }
    let manifest_bytes = serde_json::to_vec_pretty(&manifest)?;
    atomic_write(&mpath, &manifest_bytes)?;
    materialize_mcp_config(agent_home_dir, provider_type, servers)
}

/// Restores every file recorded in the session manifest (previous content for files
/// that existed, deletion for files the injection created), then removes the manifest.
/// No manifest means nothing was injected: no-op.
pub fn rollback_session_mcp_config(agent_home_dir: &Path) -> Result<()> {
    let mpath = manifest_path(agent_home_dir);
    if !mpath.exists() {
        return Ok(());
    }
    let raw = fs::read_to_string(&mpath)
        .with_context(|| format!("read mcp session manifest {}", mpath.display()))?;
    let manifest: McpSessionManifest = serde_json::from_str(&raw)
        .with_context(|| format!("parse mcp session manifest {}", mpath.display()))?;
    for entry in &manifest.entries {
        if entry.existed {
            if let Some(content) = &entry.previous_content {
                if let Some(parent) = entry.path.parent() {
                    fs::create_dir_all(parent)?;
                }
                atomic_write(&entry.path, content.as_bytes())?;
            }
        } else if entry.path.exists() {
            fs::remove_file(&entry.path).with_context(|| {
                format!("remove injected mcp config {}", entry.path.display())
            })?;
        }
    }
    fs::remove_file(&mpath)
        .with_context(|| format!("remove mcp session manifest {}", mpath.display()))?;
    Ok(())
}

/// materialize_task_mcp_config writes task-level MCP projection files under
/// `<workspace>/.superteam/mcp`. These files are capability projection inputs,
/// not provider auth homes. Prefer `materialize_session_mcp_config` for
/// session-scoped load/unload (spec 2026-07-23 §6).
pub fn materialize_task_mcp_config(
    workspace_path: &Path,
    provider_type: &str,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Option<PathBuf>> {
    materialize_mcp_projection(workspace_path, &task_mcp_config_path(workspace_path, provider_type)?, servers)
}

/// Session-scoped MCP projection under
/// `{workspace}/.superteam/sessions/{command_id}/mcp/…` — unloaded with the
/// project session manifest (does not touch business files).
pub fn materialize_session_mcp_config(
    workspace_path: &Path,
    command_id: &str,
    provider_type: &str,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Option<PathBuf>> {
    let target = session_mcp_config_path(workspace_path, command_id, provider_type)?;
    materialize_mcp_projection(workspace_path, &target, servers)
}

fn materialize_mcp_projection(
    workspace_path: &Path,
    target: &Path,
    servers: &[RuntimeMCPServerPayload],
) -> Result<Option<PathBuf>> {
    for server in servers {
        validate_server(server)?;
    }
    if !target.starts_with(workspace_path) {
        bail!(
            "refusing to write mcp config outside workspace: {}",
            target.display()
        );
    }
    if servers.is_empty() {
        return Ok(None);
    }

    if let Some(parent) = target.parent() {
        fs::create_dir_all(parent).with_context(|| {
            format!("failed to create mcp config dir {}", parent.display())
        })?;
    }

    let provider_type = mcp_file_provider_type(target)?;
    let content = match provider_type.as_str() {
        "codex" => render_codex_mcp_config(servers)?,
        "claude-code" => render_claude_code_mcp_config(servers)?,
        "opencode" => render_opencode_task_mcp_config(servers)?,
        other => bail!("unsupported provider_type for mcp config: {other}"),
    };

    atomic_write(target, content.as_bytes())?;
    Ok(Some(target.to_path_buf()))
}

fn mcp_projection_file_name(provider_type: &str) -> Result<&'static str> {
    match provider_type {
        "codex" => Ok("codex.toml"),
        "claude-code" | "claude" => Ok("claude.mcp.json"),
        "opencode" => Ok("opencode.json"),
        other => bail!("unsupported provider_type for mcp config: {other}"),
    }
}

fn mcp_file_provider_type(target: &Path) -> Result<String> {
    match target
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("")
    {
        "codex.toml" => Ok("codex".to_string()),
        "claude.mcp.json" => Ok("claude-code".to_string()),
        "opencode.json" => Ok("opencode".to_string()),
        other => bail!("unknown mcp projection file name: {other}"),
    }
}

pub fn task_mcp_config_path(workspace_path: &Path, provider_type: &str) -> Result<PathBuf> {
    Ok(workspace_path
        .join(".superteam")
        .join("mcp")
        .join(mcp_projection_file_name(provider_type)?))
}

pub fn session_mcp_config_path(
    workspace_path: &Path,
    command_id: &str,
    provider_type: &str,
) -> Result<PathBuf> {
    let command_id = command_id.trim();
    if command_id.is_empty() {
        bail!("command_id is required for session mcp config");
    }
    if command_id.contains('/') || command_id.contains('\\') || command_id.contains("..") {
        bail!("command_id is not a safe path segment");
    }
    Ok(workspace_path
        .join(".superteam")
        .join("sessions")
        .join(command_id)
        .join("mcp")
        .join(mcp_projection_file_name(provider_type)?))
}

/// Build a session-local CODEX_HOME that carries MCP servers while auth stays
/// on the host (symlink auth.json). Does **not** point at employee capability
/// home — that remains a hard auth boundary (provider_command_test).
pub fn prepare_codex_session_overlay(
    overlay_home: &Path,
    servers: &[RuntimeMCPServerPayload],
) -> Result<()> {
    for server in servers {
        validate_server(server)?;
    }
    fs::create_dir_all(overlay_home)
        .with_context(|| format!("create codex overlay home {}", overlay_home.display()))?;

    let host_home = host_codex_home();
    let host_config = host_home.as_ref().map(|home| home.join("config.toml"));
    let content = match host_config.as_ref().filter(|path| path.exists()) {
        Some(path) => merge_codex_config(path, servers)?,
        None => render_codex_mcp_config(servers)?,
    };
    atomic_write(&overlay_home.join("config.toml"), content.as_bytes())?;

    if let Some(host) = host_home {
        for name in ["auth.json", "auth.json.bak"] {
            let src = host.join(name);
            let dst = overlay_home.join(name);
            if src.exists() && !dst.exists() {
                #[cfg(unix)]
                std::os::unix::fs::symlink(&src, &dst).with_context(|| {
                    format!(
                        "symlink host codex {} into overlay {}",
                        src.display(),
                        dst.display()
                    )
                })?;
            }
        }
    }
    Ok(())
}

fn host_codex_home() -> Option<PathBuf> {
    if let Some(explicit) = std::env::var_os("CODEX_HOME") {
        let path = PathBuf::from(explicit);
        if !path.as_os_str().is_empty() {
            return Some(path);
        }
    }
    std::env::var_os("HOME").map(|home| PathBuf::from(home).join(".codex"))
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

    #[test]
    fn inject_records_manifest_and_rollback_restores_prior_codex_config() {
        let dir = tempfile::tempdir().unwrap();
        let codex_dir = dir.path().join(".codex");
        std::fs::create_dir_all(&codex_dir).unwrap();
        std::fs::write(codex_dir.join("config.toml"), "theme = \"dark\"\n").unwrap();

        let written = inject_session_mcp_config(dir.path(), "codex", &[github_server()]).unwrap();
        assert_eq!(written.len(), 1);
        let injected = std::fs::read_to_string(&written[0]).unwrap();
        assert!(injected.contains("[mcp_servers.github]"));
        assert!(injected.contains("theme"));
        assert!(manifest_path(dir.path()).exists());

        rollback_session_mcp_config(dir.path()).unwrap();
        let restored = std::fs::read_to_string(codex_dir.join("config.toml")).unwrap();
        assert_eq!(restored, "theme = \"dark\"\n");
        assert!(!manifest_path(dir.path()).exists());
    }

    #[test]
    fn rollback_deletes_file_created_by_injection() {
        let dir = tempfile::tempdir().unwrap();
        inject_session_mcp_config(dir.path(), "claude-code", &[github_server()]).unwrap();
        assert!(dir.path().join(".mcp.json").exists());
        rollback_session_mcp_config(dir.path()).unwrap();
        assert!(!dir.path().join(".mcp.json").exists());
    }

    #[test]
    fn inject_rolls_back_residual_manifest_before_injecting() {
        let dir = tempfile::tempdir().unwrap();
        // 第一次注入后不回滚，模拟异常退出残留
        inject_session_mcp_config(dir.path(), "claude-code", &[github_server()]).unwrap();
        // 第二次注入应先回滚残留（.mcp.json 删除）再重新注入
        let written = inject_session_mcp_config(dir.path(), "claude-code", &[github_server()]).unwrap();
        assert_eq!(written.len(), 1);
        let manifest_raw = std::fs::read_to_string(manifest_path(dir.path())).unwrap();
        // 残留回滚后重拍快照：本次快照必须记录"文件不存在"，而不是把上次注入内容当原值
        assert!(manifest_raw.contains("\"existed\": false") || manifest_raw.contains("\"existed\":false"));
    }

    #[test]
    fn rollback_without_manifest_is_noop() {
        let dir = tempfile::tempdir().unwrap();
        rollback_session_mcp_config(dir.path()).unwrap();
    }

    #[test]
    fn inject_with_empty_servers_clears_residual_and_writes_nothing() {
        let dir = tempfile::tempdir().unwrap();
        inject_session_mcp_config(dir.path(), "opencode", &[github_server()]).unwrap();
        let written = inject_session_mcp_config(dir.path(), "opencode", &[]).unwrap();
        assert!(written.is_empty());
        assert!(!manifest_path(dir.path()).exists());
        assert!(!dir.path().join("opencode.json").exists());
    }

    #[test]
    fn inject_rejects_unknown_provider_even_with_empty_servers() {
        let dir = tempfile::tempdir().unwrap();
        let error = inject_session_mcp_config(dir.path(), "bogus-provider", &[]).unwrap_err();
        assert!(error.to_string().contains("unsupported provider_type"));
    }
}
