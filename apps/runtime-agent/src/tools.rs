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

fn find_on_path(name: &str) -> Option<PathBuf> {
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
