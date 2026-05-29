use std::env;
use std::net::SocketAddr;
use std::path::{Path, PathBuf};

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeConfig {
    pub runtime: RuntimeSection,
    pub http: HttpSection,
    pub runs: RunsSection,
    pub workspace: WorkspaceSection,
    pub providers: ProvidersSection,
    pub logging: LoggingSection,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeSection {
    pub node_id: String,
    pub control_plane_url: String,
    pub heartbeat_interval: u64,
    pub max_concurrent_tasks: u16,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HttpSection {
    pub addr: SocketAddr,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RunsSection {
    pub log_dir: PathBuf,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WorkspaceSection {
    pub base_dir: PathBuf,
    pub cleanup_policy: String,
    pub max_retained: u16,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProvidersSection {
    pub claude_code: ProviderSection,
    pub opencode: ProviderSection,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProviderSection {
    pub enabled: bool,
    pub binary_path: PathBuf,
    pub timeout: u64,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct LoggingSection {
    pub level: String,
    pub format: String,
    pub output: String,
    pub file_path: Option<PathBuf>,
}

#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct RuntimeConfigOverrides {
    pub node_id: Option<String>,
    pub http_addr: Option<SocketAddr>,
    pub run_log_dir: Option<PathBuf>,
    pub claude_bin: Option<PathBuf>,
    pub opencode_bin: Option<PathBuf>,
}

#[derive(Debug, Deserialize, Default)]
struct FileConfig {
    runtime: Option<FileRuntimeSection>,
    http: Option<FileHttpSection>,
    runs: Option<FileRunsSection>,
    workspace: Option<FileWorkspaceSection>,
    providers: Option<FileProvidersSection>,
    logging: Option<FileLoggingSection>,
}

#[derive(Debug, Deserialize, Default)]
struct FileRuntimeSection {
    node_id: Option<String>,
    control_plane_url: Option<String>,
    heartbeat_interval: Option<u64>,
    max_concurrent_tasks: Option<u16>,
}

#[derive(Debug, Deserialize, Default)]
struct FileHttpSection {
    addr: Option<SocketAddr>,
}

#[derive(Debug, Deserialize, Default)]
struct FileRunsSection {
    log_dir: Option<PathBuf>,
}

#[derive(Debug, Deserialize, Default)]
struct FileWorkspaceSection {
    base_dir: Option<PathBuf>,
    cleanup_policy: Option<String>,
    max_retained: Option<u16>,
}

#[derive(Debug, Deserialize, Default)]
struct FileProvidersSection {
    claude_code: Option<FileProviderSection>,
    opencode: Option<FileProviderSection>,
}

#[derive(Debug, Deserialize, Default)]
struct FileProviderSection {
    enabled: Option<bool>,
    binary_path: Option<PathBuf>,
    timeout: Option<u64>,
}

#[derive(Debug, Deserialize, Default)]
struct FileLoggingSection {
    level: Option<String>,
    format: Option<String>,
    output: Option<String>,
    file_path: Option<PathBuf>,
}

impl RuntimeConfig {
    pub fn new(node_id: impl Into<String>) -> anyhow::Result<Self> {
        let mut cfg = Self::default();
        cfg.runtime.node_id = node_id.into().trim().to_string();
        cfg.validate()?;
        Ok(cfg)
    }

    pub fn load(path: Option<&Path>, overrides: RuntimeConfigOverrides) -> anyhow::Result<Self> {
        Self::load_with_env(path, env::vars(), overrides)
    }

    pub fn load_with_env<I, K, V>(
        path: Option<&Path>,
        env_vars: I,
        overrides: RuntimeConfigOverrides,
    ) -> anyhow::Result<Self>
    where
        I: IntoIterator<Item = (K, V)>,
        K: AsRef<str>,
        V: AsRef<str>,
    {
        let mut cfg = Self::default();
        if let Some(path) = path {
            cfg.apply_file(path)?;
        }
        cfg.apply_env(env_vars)?;
        cfg.apply_overrides(overrides);
        cfg.validate()?;
        Ok(cfg)
    }

    pub fn node_id(&self) -> &str {
        &self.runtime.node_id
    }

    pub fn http_config(&self) -> crate::server::RuntimeHttpConfig {
        crate::server::RuntimeHttpConfig {
            node_id: self.runtime.node_id.clone(),
            run_log_dir: self.runs.log_dir.clone(),
            claude_bin: self.providers.claude_code.binary_path.clone(),
            opencode_bin: self.providers.opencode.binary_path.clone(),
        }
    }

    fn apply_file(&mut self, path: &Path) -> anyhow::Result<()> {
        let body = std::fs::read_to_string(path)?;
        let file: FileConfig = toml::from_str(&body)?;

        if let Some(runtime) = file.runtime {
            apply_string(&mut self.runtime.node_id, runtime.node_id);
            apply_string(
                &mut self.runtime.control_plane_url,
                runtime.control_plane_url,
            );
            apply_copy(
                &mut self.runtime.heartbeat_interval,
                runtime.heartbeat_interval,
            );
            apply_copy(
                &mut self.runtime.max_concurrent_tasks,
                runtime.max_concurrent_tasks,
            );
        }

        if let Some(http) = file.http {
            apply_copy(&mut self.http.addr, http.addr);
        }

        if let Some(runs) = file.runs {
            apply_path(&mut self.runs.log_dir, runs.log_dir);
        }

        if let Some(workspace) = file.workspace {
            apply_path(&mut self.workspace.base_dir, workspace.base_dir);
            apply_string(&mut self.workspace.cleanup_policy, workspace.cleanup_policy);
            apply_copy(&mut self.workspace.max_retained, workspace.max_retained);
        }

        if let Some(providers) = file.providers {
            if let Some(claude_code) = providers.claude_code {
                self.providers.claude_code.apply_file(claude_code);
            }
            if let Some(opencode) = providers.opencode {
                self.providers.opencode.apply_file(opencode);
            }
        }

        if let Some(logging) = file.logging {
            apply_string(&mut self.logging.level, logging.level);
            apply_string(&mut self.logging.format, logging.format);
            apply_string(&mut self.logging.output, logging.output);
            if logging.file_path.is_some() {
                self.logging.file_path = logging.file_path;
            }
        }

        Ok(())
    }

    fn apply_env<I, K, V>(&mut self, env_vars: I) -> anyhow::Result<()>
    where
        I: IntoIterator<Item = (K, V)>,
        K: AsRef<str>,
        V: AsRef<str>,
    {
        for (key, value) in env_vars {
            self.apply_env_value(key.as_ref(), value.as_ref())?;
        }
        Ok(())
    }

    fn apply_env_value(&mut self, key: &str, value: &str) -> anyhow::Result<()> {
        if value.is_empty() {
            return Ok(());
        }

        match key {
            "RUNTIME_AGENT_NODE_ID" => self.runtime.node_id = value.to_string(),
            "RUNTIME_AGENT_CONTROL_PLANE_URL" => {
                self.runtime.control_plane_url = value.to_string();
            }
            "RUNTIME_AGENT_HEARTBEAT_INTERVAL" => {
                self.runtime.heartbeat_interval = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_MAX_CONCURRENT_TASKS" => {
                self.runtime.max_concurrent_tasks = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_HTTP_ADDR" => self.http.addr = parse_env(key, value)?,
            "RUNTIME_AGENT_RUN_LOG_DIR" => self.runs.log_dir = PathBuf::from(value),
            "RUNTIME_AGENT_WORKSPACE_DIR" => self.workspace.base_dir = PathBuf::from(value),
            "RUNTIME_AGENT_CLEANUP_POLICY" => self.workspace.cleanup_policy = value.to_string(),
            "RUNTIME_AGENT_MAX_RETAINED_WORKSPACES" => {
                self.workspace.max_retained = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_PROVIDER_CLAUDE_CODE_ENABLED" => {
                self.providers.claude_code.enabled = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_PROVIDER_CLAUDE_CODE_BINARY" => {
                self.providers.claude_code.binary_path = PathBuf::from(value);
            }
            "RUNTIME_AGENT_PROVIDER_CLAUDE_CODE_TIMEOUT" => {
                self.providers.claude_code.timeout = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_PROVIDER_OPENCODE_ENABLED" => {
                self.providers.opencode.enabled = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_PROVIDER_OPENCODE_BINARY" => {
                self.providers.opencode.binary_path = PathBuf::from(value);
            }
            "RUNTIME_AGENT_PROVIDER_OPENCODE_TIMEOUT" => {
                self.providers.opencode.timeout = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_LOG_LEVEL" => self.logging.level = value.to_string(),
            "RUNTIME_AGENT_LOG_FORMAT" => self.logging.format = value.to_string(),
            "RUNTIME_AGENT_LOG_OUTPUT" => self.logging.output = value.to_string(),
            "RUNTIME_AGENT_LOG_FILE_PATH" => {
                self.logging.file_path = Some(PathBuf::from(value));
            }
            _ => {}
        }
        Ok(())
    }

    fn apply_overrides(&mut self, overrides: RuntimeConfigOverrides) {
        apply_string(&mut self.runtime.node_id, overrides.node_id);
        apply_copy(&mut self.http.addr, overrides.http_addr);
        apply_path(&mut self.runs.log_dir, overrides.run_log_dir);
        apply_path(
            &mut self.providers.claude_code.binary_path,
            overrides.claude_bin,
        );
        apply_path(
            &mut self.providers.opencode.binary_path,
            overrides.opencode_bin,
        );
    }

    fn validate(&self) -> anyhow::Result<()> {
        if self.runtime.node_id.trim().is_empty() {
            anyhow::bail!("node id is required");
        }
        if self.runtime.control_plane_url.trim().is_empty() {
            anyhow::bail!("control plane url is required");
        }
        if self.runtime.heartbeat_interval == 0 {
            anyhow::bail!("heartbeat interval must be greater than 0");
        }
        if self.runtime.max_concurrent_tasks == 0 {
            anyhow::bail!("max concurrent tasks must be greater than 0");
        }
        if self.workspace.cleanup_policy.trim().is_empty() {
            anyhow::bail!("workspace cleanup policy is required");
        }
        if self
            .providers
            .claude_code
            .binary_path
            .as_os_str()
            .is_empty()
        {
            anyhow::bail!("claude code binary path is required");
        }
        if self.providers.opencode.binary_path.as_os_str().is_empty() {
            anyhow::bail!("opencode binary path is required");
        }
        if self.logging.level.trim().is_empty() {
            anyhow::bail!("log level is required");
        }
        Ok(())
    }
}

impl Default for RuntimeConfig {
    fn default() -> Self {
        Self {
            runtime: RuntimeSection {
                node_id: "local-dev-node".to_string(),
                control_plane_url: "http://localhost:8080".to_string(),
                heartbeat_interval: 30,
                max_concurrent_tasks: 3,
            },
            http: HttpSection {
                addr: ([127, 0, 0, 1], 7077).into(),
            },
            runs: RunsSection {
                log_dir: PathBuf::from(".superteam/runtime-runs"),
            },
            workspace: WorkspaceSection {
                base_dir: PathBuf::from(".superteam/workspaces"),
                cleanup_policy: "on_success".to_string(),
                max_retained: 10,
            },
            providers: ProvidersSection {
                claude_code: ProviderSection {
                    enabled: true,
                    binary_path: PathBuf::from("claude"),
                    timeout: 3600,
                },
                opencode: ProviderSection {
                    enabled: false,
                    binary_path: PathBuf::from("opencode"),
                    timeout: 3600,
                },
            },
            logging: LoggingSection {
                level: "info".to_string(),
                format: "pretty".to_string(),
                output: "stdout".to_string(),
                file_path: None,
            },
        }
    }
}

impl ProviderSection {
    fn apply_file(&mut self, file: FileProviderSection) {
        apply_copy(&mut self.enabled, file.enabled);
        apply_path(&mut self.binary_path, file.binary_path);
        apply_copy(&mut self.timeout, file.timeout);
    }
}

fn apply_string(target: &mut String, value: Option<String>) {
    if let Some(value) = value {
        *target = value;
    }
}

fn apply_path(target: &mut PathBuf, value: Option<PathBuf>) {
    if let Some(value) = value {
        *target = value;
    }
}

fn apply_copy<T: Copy>(target: &mut T, value: Option<T>) {
    if let Some(value) = value {
        *target = value;
    }
}

fn parse_env<T>(key: &str, value: &str) -> anyhow::Result<T>
where
    T: std::str::FromStr,
    T::Err: std::fmt::Display,
{
    value
        .parse()
        .map_err(|err| anyhow::anyhow!("invalid {key}: {err}"))
}
