use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use aws_sdk_s3::Client as S3Client;
use serde::{Deserialize, Serialize};

use crate::commands::payload::RuntimeSkillPayload;
use crate::skills::{
    ensure_safe_install_path, materialize_skill_to_dir, remove_skill_dir_if_exists,
    validate_skill_key as validate_key,
};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct InstallSkillsCommandPayload {
    pub command_id: String,
    pub tenant_id: String,
    pub skill: RuntimeSkillPayload,
    #[serde(default)]
    pub rollback_on_failure: bool,
    pub targets: Vec<InstallSkillTargetPayload>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct InstallSkillTargetPayload {
    pub team_id: String,
    pub digital_employee_id: String,
    pub agent_home_dir: String,
    pub provider_type: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct InstalledSkillTarget {
    pub team_id: String,
    pub digital_employee_id: String,
    pub agent_home_dir: String,
    pub provider_type: String,
    pub skill_key: String,
    pub installed_path: String,
    pub archive_checksum_sha256: String,
    pub archive_file_count: i64,
}

pub fn provider_skill_dir(
    agent_home_dir: &Path,
    provider_type: &str,
    skill_key: &str,
) -> Result<PathBuf> {
    validate_skill_key(skill_key)?;
    let provider_root = match provider_type {
        "opencode" => ".opencode",
        "codex" => ".agents",
        "claude-code" => ".claude",
        _ => anyhow::bail!("unsupported provider_type for skill install: {provider_type}"),
    };
    Ok(agent_home_dir
        .join(provider_root)
        .join("skills")
        .join(skill_key))
}

pub fn validate_skill_key(key: &str) -> Result<()> {
    validate_key(key)
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProviderSkillInstallPaths {
    pub canonical_agent_home: PathBuf,
    pub target_dir: PathBuf,
    pub temp_root: PathBuf,
    pub rollback_root: PathBuf,
}

pub fn prepare_provider_skill_install_paths(
    agent_home_dir: &Path,
    provider_type: &str,
    skill_key: &str,
) -> Result<ProviderSkillInstallPaths> {
    validate_skill_key(skill_key)?;
    reject_symlink(agent_home_dir)?;
    let canonical_agent_home = agent_home_dir
        .canonicalize()
        .with_context(|| format!("canonicalize agent_home_dir: {}", agent_home_dir.display()))?;
    if !canonical_agent_home.is_dir() {
        anyhow::bail!(
            "agent_home_dir does not exist or is not a directory: {}",
            agent_home_dir.display()
        );
    }

    let provider_root_name = provider_root_name(provider_type)?;
    let provider_root = canonical_agent_home.join(provider_root_name);
    let skills_root = provider_root.join("skills");
    let target_dir = skills_root.join(skill_key);
    let temp_root = canonical_agent_home.join(".skill-tmp");
    let rollback_root = canonical_agent_home.join(".skill-rollback");

    reject_symlink_if_exists(&provider_root)?;
    reject_symlink_if_exists(&skills_root)?;
    reject_symlink_if_exists(&temp_root)?;
    reject_symlink_if_exists(&rollback_root)?;
    ensure_under_home(&canonical_agent_home, &target_dir)?;
    ensure_under_home(&canonical_agent_home, &temp_root)?;
    ensure_under_home(&canonical_agent_home, &rollback_root)?;
    ensure_safe_install_path(&target_dir, &temp_root)?;

    Ok(ProviderSkillInstallPaths {
        canonical_agent_home,
        target_dir,
        temp_root,
        rollback_root,
    })
}

pub async fn install_skill_targets(
    payload: InstallSkillsCommandPayload,
    s3_client: &S3Client,
    bucket: &str,
) -> Result<Vec<InstalledSkillTarget>> {
    if payload.targets.is_empty() {
        anyhow::bail!("install_skills targets must not be empty");
    }

    let mut installed = Vec::with_capacity(payload.targets.len());
    let mut rollbacks: Vec<SkillInstallRollback> = Vec::new();

    for target in payload.targets {
        let agent_home_dir = PathBuf::from(&target.agent_home_dir);
        let paths = match prepare_provider_skill_install_paths(
            &agent_home_dir,
            &target.provider_type,
            &payload.skill.skill_key,
        ) {
            Ok(paths) => paths,
            Err(error) => {
                rollback_all(rollbacks)?;
                return Err(error);
            }
        };
        let rollback_index =
            rollback_index_for_home(&mut rollbacks, payload.rollback_on_failure, &paths)?;
        let was_current = existing_checksum(&paths.target_dir)
            .is_some_and(|checksum| checksum == payload.skill.archive_checksum_sha256);
        if !was_current {
            rollbacks[rollback_index].prepare_target(&paths.target_dir)?;
        }

        let synced = match materialize_skill_to_dir(
            &paths.target_dir,
            &paths.temp_root,
            &payload.skill,
            s3_client,
            bucket,
        )
        .await
        {
            Ok(synced) => synced,
            Err(error) => {
                rollback_all(rollbacks)?;
                return Err(error);
            }
        };

        installed.push(InstalledSkillTarget {
            team_id: target.team_id,
            digital_employee_id: target.digital_employee_id,
            agent_home_dir: target.agent_home_dir,
            provider_type: target.provider_type,
            skill_key: synced.skill_key,
            installed_path: paths.target_dir.to_string_lossy().into_owned(),
            archive_checksum_sha256: synced.content_hash,
            archive_file_count: payload.skill.archive_file_count,
        });
    }

    for rollback in rollbacks {
        rollback.commit()?;
    }
    Ok(installed)
}

fn existing_checksum(target_dir: &Path) -> Option<String> {
    fs::read_to_string(target_dir.join(".skill-checksum")).ok()
}

#[derive(Debug)]
pub struct SkillInstallRollback {
    enabled: bool,
    entries: Vec<RollbackEntry>,
    committed: bool,
    canonical_agent_home: PathBuf,
    rollback_root: PathBuf,
}

#[derive(Debug)]
struct RollbackEntry {
    target_dir: PathBuf,
    backup_dir: Option<PathBuf>,
}

impl SkillInstallRollback {
    pub fn new(enabled: bool, agent_home_dir: &Path) -> Result<Self> {
        let canonical_agent_home = agent_home_dir
            .canonicalize()
            .with_context(|| format!("canonicalize rollback home: {}", agent_home_dir.display()))?;
        let rollback_root = canonical_agent_home.join(".skill-rollback");
        ensure_under_home(&canonical_agent_home, &rollback_root)?;
        reject_symlink_if_exists(&rollback_root)?;
        if enabled {
            match fs::create_dir(&rollback_root) {
                Ok(()) => {}
                Err(e) if e.kind() == std::io::ErrorKind::AlreadyExists => {
                    anyhow::bail!(
                        "skill rollback root already exists: {}",
                        rollback_root.display()
                    );
                }
                Err(e) => {
                    return Err(e).with_context(|| {
                        format!("create skill rollback root: {}", rollback_root.display())
                    });
                }
            }
        }
        Ok(Self {
            enabled,
            entries: Vec::new(),
            committed: false,
            canonical_agent_home,
            rollback_root,
        })
    }

    pub fn prepare_target(&mut self, target_dir: &Path) -> Result<()> {
        if !self.enabled {
            return Ok(());
        }
        if self
            .entries
            .iter()
            .any(|entry| entry.target_dir == target_dir)
        {
            return Ok(());
        }

        let backup_dir = if target_dir.exists() {
            self.ensure_owned_target(target_dir)?;
            let backup_dir = self.rollback_backup_dir(target_dir)?;
            fs::rename(target_dir, &backup_dir)
                .with_context(|| format!("backup skill directory: {}", target_dir.display()))?;
            Some(backup_dir)
        } else {
            None
        };
        self.entries.push(RollbackEntry {
            target_dir: target_dir.to_path_buf(),
            backup_dir,
        });
        Ok(())
    }

    pub fn rollback(mut self) -> Result<()> {
        if !self.enabled || self.committed {
            return Ok(());
        }

        for entry in self.entries.iter().rev() {
            self.ensure_owned_target(&entry.target_dir)?;
            remove_skill_dir_if_exists(&entry.target_dir)?;
            if let Some(backup_dir) = &entry.backup_dir {
                self.ensure_owned_backup(backup_dir)?;
                if let Some(parent) = entry.target_dir.parent() {
                    fs::create_dir_all(parent).with_context(|| {
                        format!("create rollback parent: {}", entry.target_dir.display())
                    })?;
                }
                fs::rename(backup_dir, &entry.target_dir).with_context(|| {
                    format!("restore skill directory: {}", entry.target_dir.display())
                })?;
            }
        }
        self.cleanup_owned_rollback_root();
        self.committed = true;
        Ok(())
    }

    pub fn commit(mut self) -> Result<()> {
        for entry in &self.entries {
            if let Some(backup_dir) = &entry.backup_dir {
                self.ensure_owned_backup(backup_dir)?;
                remove_skill_dir_if_exists(backup_dir).with_context(|| {
                    format!("cleanup skill rollback backup: {}", backup_dir.display())
                })?;
            }
        }
        self.cleanup_owned_rollback_root();
        self.committed = true;
        Ok(())
    }

    fn rollback_backup_dir(&self, target_dir: &Path) -> Result<PathBuf> {
        reject_symlink_if_exists(target_dir)?;
        for attempt in 0..16 {
            let backup_dir =
                self.rollback_root
                    .join(format!("target-{}-{}", self.entries.len(), attempt));
            if !backup_dir.exists() {
                return Ok(backup_dir);
            }
        }
        anyhow::bail!(
            "failed to reserve skill rollback backup under {}",
            self.rollback_root.display()
        )
    }

    fn cleanup_owned_rollback_root(&self) {
        if self.enabled {
            let _ = fs::remove_dir_all(&self.rollback_root);
        }
    }

    fn ensure_owned_target(&self, path: &Path) -> Result<()> {
        let normalized = normalize_existing_path_for_home(path)?;
        ensure_under_home(&self.canonical_agent_home, &normalized)?;
        reject_symlink_if_exists(path)
    }

    fn ensure_owned_backup(&self, path: &Path) -> Result<()> {
        ensure_under_home(&self.rollback_root, path)?;
        reject_symlink_if_exists(path)
    }
}

fn rollback_index_for_home(
    rollbacks: &mut Vec<SkillInstallRollback>,
    enabled: bool,
    paths: &ProviderSkillInstallPaths,
) -> Result<usize> {
    if let Some(index) = rollbacks
        .iter()
        .position(|rollback| rollback.canonical_agent_home == paths.canonical_agent_home)
    {
        return Ok(index);
    }

    rollbacks.push(SkillInstallRollback::new(
        enabled,
        &paths.canonical_agent_home,
    )?);
    Ok(rollbacks.len() - 1)
}

fn rollback_all(mut rollbacks: Vec<SkillInstallRollback>) -> Result<()> {
    while let Some(rollback) = rollbacks.pop() {
        rollback.rollback()?;
    }
    Ok(())
}

fn provider_root_name(provider_type: &str) -> Result<&'static str> {
    match provider_type {
        "opencode" => Ok(".opencode"),
        "codex" => Ok(".agents"),
        "claude-code" => Ok(".claude"),
        _ => anyhow::bail!("unsupported provider_type for skill install: {provider_type}"),
    }
}

fn reject_symlink(path: &Path) -> Result<()> {
    let metadata = fs::symlink_metadata(path)
        .with_context(|| format!("inspect skill path: {}", path.display()))?;
    if metadata.file_type().is_symlink() {
        anyhow::bail!(
            "skill path component must not be a symlink: {}",
            path.display()
        );
    }
    Ok(())
}

fn reject_symlink_if_exists(path: &Path) -> Result<()> {
    match fs::symlink_metadata(path) {
        Ok(metadata) if metadata.file_type().is_symlink() => {
            anyhow::bail!(
                "skill path component must not be a symlink: {}",
                path.display()
            );
        }
        Ok(_) => Ok(()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(e).with_context(|| format!("inspect skill path: {}", path.display())),
    }
}

fn ensure_under_home(home: &Path, path: &Path) -> Result<()> {
    if path.starts_with(home) {
        Ok(())
    } else {
        anyhow::bail!(
            "skill path must stay under agent_home_dir: {}",
            path.display()
        )
    }
}

fn normalize_existing_path_for_home(path: &Path) -> Result<PathBuf> {
    match path.canonicalize() {
        Ok(path) => Ok(path),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
            let parent = path
                .parent()
                .context("skill path has no parent for normalization")?;
            let parent = parent
                .canonicalize()
                .with_context(|| format!("canonicalize skill path parent: {}", parent.display()))?;
            let file_name = path
                .file_name()
                .context("skill path has no file name for normalization")?;
            Ok(parent.join(file_name))
        }
        Err(e) => Err(e).with_context(|| format!("canonicalize skill path: {}", path.display())),
    }
}
