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
    #[serde(default)]
    pub rollback_on_failure: bool,
    pub targets: Vec<InstallSkillTargetPayload>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct InstallSkillTargetPayload {
    pub agent_home_dir: String,
    pub provider_type: String,
    pub skill: RuntimeSkillPayload,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct InstalledSkillTarget {
    pub agent_home_dir: String,
    pub provider_type: String,
    pub skill_key: String,
    pub path: String,
    pub checksum: String,
    pub file_count: u64,
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
    let mut changed_dirs = Vec::new();

    for target in payload.targets {
        let agent_home_dir = PathBuf::from(&target.agent_home_dir);
        if !agent_home_dir.is_dir() {
            let error = anyhow::anyhow!(
                "agent_home_dir does not exist or is not a directory: {}",
                agent_home_dir.display()
            );
            rollback_if_requested(payload.rollback_on_failure, &changed_dirs)?;
            return Err(error);
        }

        let target_dir = match provider_skill_dir(
            &agent_home_dir,
            &target.provider_type,
            &target.skill.skill_key,
        ) {
            Ok(target_dir) => target_dir,
            Err(error) => {
                rollback_if_requested(payload.rollback_on_failure, &changed_dirs)?;
                return Err(error);
            }
        };
        let was_current = existing_checksum(&target_dir)
            .is_some_and(|checksum| checksum == target.skill.archive_checksum_sha256);
        let temp_root = agent_home_dir.join(".skill-tmp");

        let synced = match materialize_skill_to_dir(
            &target_dir,
            &temp_root,
            &target.skill,
            s3_client,
            bucket,
        )
        .await
        {
            Ok(synced) => synced,
            Err(error) => {
                rollback_if_requested(payload.rollback_on_failure, &changed_dirs)?;
                return Err(error);
            }
        };

        if !was_current {
            changed_dirs.push(target_dir.clone());
        }

        installed.push(InstalledSkillTarget {
            agent_home_dir: target.agent_home_dir,
            provider_type: target.provider_type,
            skill_key: synced.skill_key,
            path: target_dir.to_string_lossy().into_owned(),
            checksum: synced.content_hash,
            file_count: synced.file_count,
        });
    }

    Ok(installed)
}

fn existing_checksum(target_dir: &Path) -> Option<String> {
    fs::read_to_string(target_dir.join(".skill-checksum")).ok()
}

fn rollback_if_requested(rollback_on_failure: bool, changed_dirs: &[PathBuf]) -> Result<()> {
    if !rollback_on_failure {
        return Ok(());
    }

    for dir in changed_dirs.iter().rev() {
        remove_skill_dir_if_exists(dir)
            .with_context(|| format!("rollback installed skill directory: {}", dir.display()))?;
    }

    Ok(())
}
