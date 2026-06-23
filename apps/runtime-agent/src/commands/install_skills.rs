use std::fs;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use aws_sdk_s3::Client as S3Client;
use serde::{Deserialize, Serialize};

use crate::commands::payload::RuntimeSkillPayload;
use crate::skills::{
    materialize_skill_to_dir, remove_skill_dir_if_exists, validate_skill_key as validate_key,
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

pub async fn install_skill_targets(
    payload: InstallSkillsCommandPayload,
    s3_client: &S3Client,
    bucket: &str,
) -> Result<Vec<InstalledSkillTarget>> {
    if payload.targets.is_empty() {
        anyhow::bail!("install_skills targets must not be empty");
    }

    let mut installed = Vec::with_capacity(payload.targets.len());
    let mut rollback = SkillInstallRollback::new(payload.rollback_on_failure);

    for target in payload.targets {
        let agent_home_dir = PathBuf::from(&target.agent_home_dir);
        if !agent_home_dir.is_dir() {
            let error = anyhow::anyhow!(
                "agent_home_dir does not exist or is not a directory: {}",
                agent_home_dir.display()
            );
            rollback.rollback()?;
            return Err(error);
        }

        let target_dir = match provider_skill_dir(
            &agent_home_dir,
            &target.provider_type,
            &payload.skill.skill_key,
        ) {
            Ok(target_dir) => target_dir,
            Err(error) => {
                rollback.rollback()?;
                return Err(error);
            }
        };
        let temp_root = agent_home_dir.join(".skill-tmp");
        let was_current = existing_checksum(&target_dir)
            .is_some_and(|checksum| checksum == payload.skill.archive_checksum_sha256);
        if !was_current {
            rollback.prepare_target(&target_dir)?;
        }

        let synced = match materialize_skill_to_dir(
            &target_dir,
            &temp_root,
            &payload.skill,
            s3_client,
            bucket,
        )
        .await
        {
            Ok(synced) => synced,
            Err(error) => {
                rollback.rollback()?;
                return Err(error);
            }
        };

        installed.push(InstalledSkillTarget {
            team_id: target.team_id,
            digital_employee_id: target.digital_employee_id,
            agent_home_dir: target.agent_home_dir,
            provider_type: target.provider_type,
            skill_key: synced.skill_key,
            installed_path: target_dir.to_string_lossy().into_owned(),
            archive_checksum_sha256: synced.content_hash,
            archive_file_count: payload.skill.archive_file_count,
        });
    }

    rollback.commit()?;
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
}

#[derive(Debug)]
struct RollbackEntry {
    target_dir: PathBuf,
    backup_dir: Option<PathBuf>,
}

impl SkillInstallRollback {
    pub fn new(enabled: bool) -> Self {
        Self {
            enabled,
            entries: Vec::new(),
            committed: false,
        }
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
            let backup_dir = rollback_backup_dir(target_dir, self.entries.len())?;
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
            remove_skill_dir_if_exists(&entry.target_dir)?;
            if let Some(backup_dir) = &entry.backup_dir {
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
        self.committed = true;
        Ok(())
    }

    pub fn commit(mut self) -> Result<()> {
        for entry in &self.entries {
            if let Some(backup_dir) = &entry.backup_dir {
                remove_skill_dir_if_exists(backup_dir).with_context(|| {
                    format!("cleanup skill rollback backup: {}", backup_dir.display())
                })?;
            }
        }
        self.committed = true;
        Ok(())
    }
}

fn rollback_backup_dir(target_dir: &Path, index: usize) -> Result<PathBuf> {
    let parent = target_dir
        .parent()
        .context("skill target directory has no parent")?;
    fs::create_dir_all(parent)?;
    Ok(parent.join(format!(
        ".{}-rollback-{}-{}-{}",
        target_dir
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("skill"),
        std::process::id(),
        index,
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or(0)
    )))
}
