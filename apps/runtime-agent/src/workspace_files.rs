use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use anyhow::{Context, Result};
use serde::Serialize;
use sha2::{Digest, Sha256};

use crate::commands::payload::RuntimeWorkspaceFilePayload;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProviderHomeKind {
    ClaudeCode,
    OpenCode,
}

#[derive(Debug, Clone)]
pub struct WorkspaceMaterializationPlan {
    pub agent_home_dir: PathBuf,
    pub provider_home: ProviderHomeKind,
    pub files: Vec<RuntimeWorkspaceFilePayload>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct SyncedWorkspaceFile {
    pub file_id: String,
    pub revision_id: String,
    pub path: String,
    pub content_hash: String,
}

#[derive(Debug, Clone)]
pub struct WorkspaceMaterializationResult {
    pub agent_home_dir: PathBuf,
    pub synced_files: Vec<SyncedWorkspaceFile>,
}

pub fn provider_home_kind(provider_type: &str) -> Result<ProviderHomeKind> {
    match provider_type {
        "claude-code" => Ok(ProviderHomeKind::ClaudeCode),
        "opencode" => Ok(ProviderHomeKind::OpenCode),
        _ => anyhow::bail!(
            "unsupported provider_type for workspace materialization: {provider_type}"
        ),
    }
}

pub fn materialize_workspace(
    plan: WorkspaceMaterializationPlan,
) -> Result<WorkspaceMaterializationResult> {
    fs::create_dir_all(&plan.agent_home_dir).with_context(|| {
        format!(
            "failed to create agent home directory {}",
            plan.agent_home_dir.display()
        )
    })?;
    fs::create_dir_all(
        plan.agent_home_dir
            .join(provider_private_dir(plan.provider_home)),
    )?;

    let mut synced_files = Vec::new();
    let mut has_agents_file = false;

    for file in plan.files {
        if file.sync_policy == "disabled" {
            continue;
        }

        let path = validate_workspace_path(&file.path)?;

        if file.storage_backend == "object_store" {
            anyhow::bail!("object_store workspace files are not supported yet: {path}");
        }
        if file.storage_backend != "db" {
            anyhow::bail!(
                "unsupported workspace file storage_backend '{}': {path}",
                file.storage_backend
            );
        }

        let content = file.content_text.as_ref().ok_or_else(|| {
            anyhow::anyhow!("content_text is required for db-backed workspace file: {path}")
        })?;
        let computed_hash = sha256_hex(content.as_bytes());
        if !computed_hash.eq_ignore_ascii_case(&file.content_hash) {
            anyhow::bail!(
                "workspace file content_hash mismatch for {path}: expected {}, got {computed_hash}",
                file.content_hash
            );
        }

        atomic_write(&plan.agent_home_dir.join(&path), content.as_bytes())?;
        if path == "AGENTS.md" {
            has_agents_file = true;
        }
        synced_files.push(SyncedWorkspaceFile {
            file_id: file.file_id,
            revision_id: file.revision_id,
            path,
            content_hash: computed_hash,
        });
    }

    if has_agents_file {
        let agents_content = fs::read(plan.agent_home_dir.join("AGENTS.md"))?;
        atomic_write(&plan.agent_home_dir.join("CLAUDE.md"), &agents_content)?;
    }

    Ok(WorkspaceMaterializationResult {
        agent_home_dir: plan.agent_home_dir,
        synced_files,
    })
}

pub fn validate_workspace_path(path: &str) -> Result<String> {
    if path.is_empty() {
        anyhow::bail!("workspace file path must not be empty");
    }
    if path.starts_with('/') || Path::new(path).is_absolute() {
        anyhow::bail!("workspace file path must be relative: {path}");
    }
    if path.ends_with('/') {
        anyhow::bail!("workspace file path must not end with a slash: {path}");
    }
    if path.contains('\\') || path.contains('\0') {
        anyhow::bail!("workspace file path contains an unsafe character: {path}");
    }
    if path == "CLAUDE.md" {
        anyhow::bail!("CLAUDE.md is generated compatibility material");
    }

    let mut components = path.split('/');
    let first = components
        .next()
        .ok_or_else(|| anyhow::anyhow!("workspace file path must not be empty"))?;
    reject_component(first, path)?;
    if matches!(first, ".claude" | ".opencode" | ".git" | ".superteam") {
        anyhow::bail!("workspace file path uses a reserved top-level directory: {path}");
    }
    for component in components {
        reject_component(component, path)?;
    }

    Ok(path.to_string())
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

pub fn atomic_write(path: &Path, bytes: &[u8]) -> Result<()> {
    let parent = path
        .parent()
        .ok_or_else(|| anyhow::anyhow!("workspace file path has no parent: {}", path.display()))?;
    fs::create_dir_all(parent)?;

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

fn reject_component(component: &str, full_path: &str) -> Result<()> {
    if component.is_empty() || component == "." || component == ".." {
        anyhow::bail!("workspace file path contains an unsafe component: {full_path}");
    }
    Ok(())
}

fn provider_private_dir(provider_home: ProviderHomeKind) -> &'static str {
    match provider_home {
        ProviderHomeKind::ClaudeCode => ".claude",
        ProviderHomeKind::OpenCode => ".opencode",
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
