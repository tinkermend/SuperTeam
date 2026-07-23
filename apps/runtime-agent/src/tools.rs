use std::env;
use std::path::{Path, PathBuf};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ToolProbeResult {
    pub name: String,
    pub binary_path: Option<PathBuf>,
    pub available: bool,
}

pub fn probe_tool(name: &str) -> ToolProbeResult {
    let name = name.trim();
    if name.is_empty() {
        return ToolProbeResult {
            name: String::new(),
            binary_path: None,
            available: false,
        };
    }

    let binary_path = find_on_path(name);
    ToolProbeResult {
        name: name.to_string(),
        available: binary_path.is_some(),
        binary_path,
    }
}

/// Resolve a configured provider/tool binary to an executable path.
///
/// - If `configured` is already an executable file, keep it.
/// - Otherwise look up the basename on `PATH` (covers bare names like `claude`
///   and stale absolute paths such as `/usr/local/bin/claude` when the real
///   binary lives elsewhere on PATH).
/// - If nothing is found, return the original path so callers still surface a
///   clear "not found" / probe failure (never invent a healthy binary).
pub fn resolve_binary_path(configured: &Path) -> PathBuf {
    if is_executable_file(configured) {
        return configured.to_path_buf();
    }

    let Some(name) = configured.file_name().and_then(|value| value.to_str()) else {
        return configured.to_path_buf();
    };
    let name = name.trim();
    if name.is_empty() {
        return configured.to_path_buf();
    }

    find_on_path(name).unwrap_or_else(|| configured.to_path_buf())
}

pub fn find_on_path(name: &str) -> Option<PathBuf> {
    let path = env::var_os("PATH")?;
    for dir in env::split_paths(&path) {
        let candidate = dir.join(name);
        if is_executable_file(&candidate) {
            return Some(candidate);
        }
    }
    None
}

#[cfg(unix)]
fn is_executable_file(path: &Path) -> bool {
    use std::os::unix::fs::PermissionsExt;

    let Ok(metadata) = std::fs::metadata(path) else {
        return false;
    };
    metadata.is_file() && metadata.permissions().mode() & 0o111 != 0
}

#[cfg(not(unix))]
fn is_executable_file(path: &Path) -> bool {
    std::fs::metadata(path)
        .map(|metadata| metadata.is_file())
        .unwrap_or(false)
}
