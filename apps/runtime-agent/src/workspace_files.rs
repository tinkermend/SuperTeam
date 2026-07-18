use std::fs::{self, OpenOptions};
use std::io::{ErrorKind, Write};
use std::path::{Component, Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use sha2::{Digest, Sha256};


#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProviderHomeKind {
    ClaudeCode,
    OpenCode,
    Codex,
}

#[derive(Debug, Clone)]
pub struct WorkspaceMaterializationPlan {
    pub agent_home_dir: PathBuf,
    pub provider_home: ProviderHomeKind,
}

#[derive(Debug, Clone)]
pub struct WorkspaceMaterializationResult {
    pub agent_home_dir: PathBuf,
}

pub fn provider_home_kind(provider_type: &str) -> Result<ProviderHomeKind> {
    match provider_type {
        "claude-code" => Ok(ProviderHomeKind::ClaudeCode),
        "opencode" => Ok(ProviderHomeKind::OpenCode),
        "codex" => Ok(ProviderHomeKind::Codex),
        _ => anyhow::bail!(
            "unsupported provider_type for workspace materialization: {provider_type}"
        ),
    }
}

pub fn materialize_workspace(
    plan: WorkspaceMaterializationPlan,
) -> Result<WorkspaceMaterializationResult> {
    ensure_real_workspace_root(&plan.agent_home_dir)?;
    ensure_workspace_directory(
        &plan.agent_home_dir,
        provider_private_dir(plan.provider_home),
    )?;

    Ok(WorkspaceMaterializationResult {
        agent_home_dir: plan.agent_home_dir,
    })
}


pub fn sha256_hex(bytes: &[u8]) -> String {
    let digest = Sha256::digest(bytes);
    let mut out = String::with_capacity(digest.len() * 2);
    for byte in digest {
        out.push(nibble_to_hex(byte >> 4));
        out.push(nibble_to_hex(byte & 0x0f));
    }
    out
}

/// atomic_write writes bytes to a temp file in the same directory and renames it into place,
/// so readers never observe a partially written file. Exposed for provider config materializers.
pub fn atomic_write(path: &Path, bytes: &[u8]) -> Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| anyhow::anyhow!("workspace file path has no parent: {}", path.display()))?;
    if !parent.is_dir() {
        anyhow::bail!(
            "workspace file parent is not a directory: {}",
            parent.display()
        );
    }

    let temp_path = unique_temp_path(path);
    let write_result = (|| -> Result<()> {
        let mut temp_file = OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&temp_path)
            .with_context(|| format!("failed to create temp file {}", temp_path.display()))?;
        temp_file.write_all(bytes)?;
        temp_file.sync_all()?;
        fs::rename(&temp_path, path).with_context(|| {
            format!(
                "failed to rename temp file {} to {}",
                temp_path.display(),
                path.display()
            )
        })?;
        Ok(())
    })();

    if write_result.is_err() {
        let _ = fs::remove_file(&temp_path);
    }

    write_result
}



fn ensure_real_workspace_root(agent_home_dir: &Path) -> Result<()> {
    match fs::symlink_metadata(agent_home_dir) {
        Ok(metadata) => {
            if metadata.file_type().is_symlink() {
                anyhow::bail!(
                    "agent home directory must not be a symlink: {}",
                    agent_home_dir.display()
                );
            }
            if !metadata.is_dir() {
                anyhow::bail!(
                    "agent home path is not a directory: {}",
                    agent_home_dir.display()
                );
            }
            Ok(())
        }
        Err(error) if error.kind() == ErrorKind::NotFound => {
            fs::create_dir_all(agent_home_dir).with_context(|| {
                format!(
                    "failed to create agent home directory {}",
                    agent_home_dir.display()
                )
            })?;
            ensure_real_workspace_root(agent_home_dir)
        }
        Err(error) => Err(error).with_context(|| {
            format!(
                "failed to inspect agent home directory {}",
                agent_home_dir.display()
            )
        }),
    }
}

fn ensure_workspace_directory(agent_home_dir: &Path, relative_dir: &str) -> Result<()> {
    ensure_workspace_directory_components(agent_home_dir, Path::new(relative_dir))
}

fn ensure_workspace_directory_components(agent_home_dir: &Path, relative_dir: &Path) -> Result<()> {
    let mut current = agent_home_dir.to_path_buf();
    for component in relative_dir.components() {
        match component {
            Component::Normal(segment) => {
                current.push(segment);
                ensure_real_directory_component(&current)?;
            }
            Component::CurDir => {}
            _ => anyhow::bail!(
                "workspace directory path contains an unsafe component: {}",
                relative_dir.display()
            ),
        }
    }
    Ok(())
}


fn ensure_real_directory_component(path: &Path) -> Result<()> {
    match fs::symlink_metadata(path) {
        Ok(metadata) => {
            if metadata.file_type().is_symlink() {
                anyhow::bail!(
                    "workspace directory component must not be a symlink: {}",
                    path.display()
                );
            }
            if !metadata.is_dir() {
                anyhow::bail!(
                    "workspace directory component is not a directory: {}",
                    path.display()
                );
            }
            Ok(())
        }
        Err(error) if error.kind() == ErrorKind::NotFound => match fs::create_dir(path) {
            Ok(()) => Ok(()),
            Err(error) if error.kind() == ErrorKind::AlreadyExists => {
                ensure_real_directory_component(path)
            }
            Err(error) => Err(error).with_context(|| {
                format!(
                    "failed to create workspace directory component {}",
                    path.display()
                )
            }),
        },
        Err(error) => Err(error).with_context(|| {
            format!(
                "failed to inspect workspace directory component {}",
                path.display()
            )
        }),
    }
}



fn provider_private_dir(provider_home: ProviderHomeKind) -> &'static str {
    match provider_home {
        ProviderHomeKind::ClaudeCode => ".claude",
        ProviderHomeKind::OpenCode => ".opencode",
        ProviderHomeKind::Codex => ".codex",
    }
}

fn unique_temp_path(path: &Path) -> PathBuf {
    let file_name = path
        .file_name()
        .and_then(|value| value.to_str())
        .unwrap_or("workspace-file");
    let nanos = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_nanos())
        .unwrap_or_default();
    path.with_file_name(format!(".{file_name}.tmp-{}-{nanos}", std::process::id()))
}

fn nibble_to_hex(value: u8) -> char {
    match value {
        0..=9 => (b'0' + value) as char,
        10..=15 => (b'a' + value - 10) as char,
        _ => unreachable!("nibble out of range"),
    }
}
