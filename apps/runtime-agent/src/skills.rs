use std::fs;
use std::io::{self, Cursor, Read};
use std::path::{Component, Path, PathBuf};

use anyhow::{Context, Result};
use aws_sdk_s3::Client as S3Client;
use sha2::{Digest, Sha256};
use zip::ZipArchive;

use crate::commands::payload::RuntimeSkillPayload;

const MAX_ARCHIVE_SIZE: u64 = 200 * 1024 * 1024;
const MAX_FILE_COUNT: usize = 10_000;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SyncedSkill {
    pub skill_id: String,
    pub skill_key: String,
    pub content_hash: String,
    pub file_count: u64,
}

pub async fn materialize_skills(
    agent_home_dir: &Path,
    skills: &[RuntimeSkillPayload],
    s3_client: &S3Client,
    bucket: &str,
) -> Result<Vec<SyncedSkill>> {
    let mut synced = Vec::with_capacity(skills.len());

    for skill in skills {
        let target_dir = agent_home_dir.join("skills").join(&skill.skill_key);
        let temp_root = agent_home_dir.join(".skill-tmp");
        synced.push(
            materialize_skill_to_dir(&target_dir, &temp_root, skill, s3_client, bucket).await?,
        );
    }

    Ok(synced)
}

pub async fn materialize_skill_to_dir(
    target_dir: &Path,
    temp_root: &Path,
    skill: &RuntimeSkillPayload,
    s3_client: &S3Client,
    bucket: &str,
) -> Result<SyncedSkill> {
    validate_skill_key(&skill.skill_key)?;
    ensure_safe_target_path(target_dir)?;

    let marker_path = target_dir.join(".skill-checksum");
    if let Ok(existing_hash) = fs::read_to_string(&marker_path) {
        if existing_hash == skill.archive_checksum_sha256 {
            return Ok(SyncedSkill {
                skill_id: skill.skill_id.clone(),
                skill_key: skill.skill_key.clone(),
                content_hash: skill.archive_checksum_sha256.clone(),
                file_count: count_materialized_files(target_dir)?,
            });
        }
    }

    let object_key = extract_object_key(&skill.archive_object_ref)?;

    let response = s3_client
        .get_object()
        .bucket(bucket)
        .key(&object_key)
        .send()
        .await
        .with_context(|| format!("failed to fetch skill archive from s3: {bucket}/{object_key}"))?;

    let body = response
        .body
        .collect()
        .await
        .map_err(|e| anyhow::anyhow!("read s3 body: {e}"))?;
    let archive_bytes = body.into_bytes();

    if archive_bytes.len() as u64 > MAX_ARCHIVE_SIZE {
        anyhow::bail!(
            "skill archive exceeds size limit: {} > {} bytes",
            archive_bytes.len(),
            MAX_ARCHIVE_SIZE
        );
    }

    let computed_hash = sha256_hex(&archive_bytes);
    if !computed_hash.eq_ignore_ascii_case(&skill.archive_checksum_sha256) {
        anyhow::bail!(
            "skill archive checksum mismatch for {}: expected {}, got {computed_hash}",
            skill.skill_key,
            skill.archive_checksum_sha256
        );
    }

    fs::create_dir_all(temp_root)?;
    let temp_dir = temp_root.join(format!(
        "{}-{}-{}",
        std::process::id(),
        skill.skill_key,
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or(0)
    ));
    if temp_dir.exists() {
        fs::remove_dir_all(&temp_dir)?;
    }
    fs::create_dir_all(&temp_dir)?;

    let cursor = Cursor::new(&archive_bytes);
    let mut archive = ZipArchive::new(cursor)
        .with_context(|| format!("invalid zip archive for skill {}", skill.skill_key))?;

    if archive.len() > MAX_FILE_COUNT {
        anyhow::bail!(
            "skill archive exceeds file count limit: {} > {}",
            archive.len(),
            MAX_FILE_COUNT
        );
    }

    let entry_names: Vec<String> = (0..archive.len())
        .filter_map(|i| {
            archive
                .by_index(i)
                .ok()
                .map(|entry| entry.name().to_string())
        })
        .collect();
    let root_prefix = common_root_prefix(&entry_names);

    let mut file_count = 0u64;
    for i in 0..archive.len() {
        let mut entry = archive.by_index(i)?;
        let entry_name = entry.name().to_string();
        if entry.is_dir() {
            continue;
        }

        let relative = normalize_zip_path(&entry_name, &root_prefix)?;
        let target = temp_dir.join(&relative);
        if let Some(parent) = target.parent() {
            fs::create_dir_all(parent)?;
        }

        let mut buf = Vec::new();
        entry.read_to_end(&mut buf)?;
        atomic_write(&target, &buf)?;
        file_count += 1;
    }

    if file_count == 0 {
        fs::remove_dir_all(&temp_dir)?;
        anyhow::bail!("skill archive contains no files: {}", skill.skill_key);
    }

    fs::write(
        temp_dir.join(".skill-checksum"),
        &skill.archive_checksum_sha256,
    )?;
    atomic_replace_dir(&temp_dir, target_dir)?;

    Ok(SyncedSkill {
        skill_id: skill.skill_id.clone(),
        skill_key: skill.skill_key.clone(),
        content_hash: skill.archive_checksum_sha256.clone(),
        file_count,
    })
}

pub fn validate_skill_key(key: &str) -> Result<()> {
    if key.is_empty() {
        anyhow::bail!("skill_key must not be empty");
    }
    if key.contains('/') || key.contains('\\') || key.contains('\0') {
        anyhow::bail!("skill_key must not contain path separators: {key}");
    }
    if key == "." || key == ".." {
        anyhow::bail!("skill_key must not be a path traversal: {key}");
    }
    Ok(())
}

pub fn remove_skill_dir_if_exists(path: &Path) -> Result<()> {
    match fs::symlink_metadata(path) {
        Ok(metadata) => {
            if !metadata.file_type().is_dir() {
                anyhow::bail!("skill target path is not a directory: {}", path.display());
            }
            fs::remove_dir_all(path)
                .with_context(|| format!("remove skill directory: {}", path.display()))?;
            Ok(())
        }
        Err(e) if e.kind() == io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(e).with_context(|| format!("inspect skill directory: {}", path.display())),
    }
}

fn extract_object_key(uri: &str) -> Result<String> {
    if let Some(stripped) = uri.strip_prefix("s3://") {
        if let Some(slash_pos) = stripped.find('/') {
            return Ok(stripped[slash_pos + 1..].to_string());
        }
        return Ok(stripped.to_string());
    }
    Ok(uri.to_string())
}

fn normalize_zip_path(entry_name: &str, root_prefix: &str) -> Result<PathBuf> {
    let stripped = entry_name.strip_prefix(root_prefix).unwrap_or(entry_name);
    let path = Path::new(stripped);

    if path.is_absolute() {
        anyhow::bail!("zip entry path must be relative: {entry_name}");
    }

    let mut result = PathBuf::new();
    for component in path.components() {
        match component {
            Component::Normal(segment) => result.push(segment),
            Component::CurDir => {}
            _ => anyhow::bail!("zip entry path contains unsafe component: {entry_name}"),
        }
    }

    if result.as_os_str().is_empty() {
        anyhow::bail!("zip entry path is empty after normalization: {entry_name}");
    }

    Ok(result)
}

fn common_root_prefix(entry_names: &[String]) -> String {
    let mut root = "";
    for name in entry_names {
        let parts: Vec<&str> = name.trim_end_matches('/').splitn(2, '/').collect();
        if parts.len() < 2 {
            return String::new();
        }
        if root.is_empty() {
            root = parts[0];
        } else if root != parts[0] {
            return String::new();
        }
    }
    if root.is_empty() {
        String::new()
    } else {
        format!("{root}/")
    }
}

fn ensure_safe_target_path(path: &Path) -> Result<()> {
    match fs::symlink_metadata(path) {
        Ok(metadata) => {
            if !metadata.file_type().is_dir() {
                anyhow::bail!("skill target path is not a directory: {}", path.display());
            }
            Ok(())
        }
        Err(e) if e.kind() == io::ErrorKind::NotFound => {
            if let Some(parent) = path.parent() {
                fs::create_dir_all(parent).with_context(|| {
                    format!("create skill directory parent: {}", path.display())
                })?;
            }
            Ok(())
        }
        Err(e) => Err(e).with_context(|| format!("inspect skill directory: {}", path.display())),
    }
}

fn atomic_replace_dir(temp_dir: &Path, target_dir: &Path) -> Result<()> {
    let parent = target_dir
        .parent()
        .context("skill target directory has no parent")?;
    fs::create_dir_all(parent)?;
    let backup_dir = parent.join(format!(
        ".{}-backup-{}-{}",
        target_dir
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("skill"),
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or(0)
    ));

    let had_target = target_dir.exists();
    if had_target {
        fs::rename(target_dir, &backup_dir)
            .with_context(|| format!("backup skill directory: {}", target_dir.display()))?;
    }

    match fs::rename(temp_dir, target_dir) {
        Ok(()) => {
            if backup_dir.exists() {
                let _ = fs::remove_dir_all(&backup_dir);
            }
            Ok(())
        }
        Err(error) => {
            if had_target && backup_dir.exists() {
                let _ = fs::rename(&backup_dir, target_dir);
            }
            Err(error).with_context(|| format!("install skill directory: {}", target_dir.display()))
        }
    }
}

fn count_materialized_files(path: &Path) -> Result<u64> {
    let mut count = 0u64;
    for entry in fs::read_dir(path)? {
        let entry = entry?;
        let file_type = entry.file_type()?;
        if file_type.is_dir() {
            count += count_materialized_files(&entry.path())?;
        } else if file_type.is_file() && entry.file_name() != ".skill-checksum" {
            count += 1;
        }
    }
    Ok(count)
}

fn atomic_write(path: &Path, bytes: &[u8]) -> Result<()> {
    let parent = path.parent().context("zip entry path has no parent")?;
    if !parent.is_dir() {
        fs::create_dir_all(parent)?;
    }

    let temp_path = parent.join(format!(
        ".{}-tmp-{}-{}",
        path.file_name().and_then(|n| n.to_str()).unwrap_or("file"),
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.as_nanos())
            .unwrap_or(0)
    ));

    fs::write(&temp_path, bytes)?;
    fs::rename(&temp_path, path)?;
    Ok(())
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

fn nibble_to_hex(value: u8) -> char {
    match value {
        0..=9 => (b'0' + value) as char,
        10..=15 => (b'a' + value - 10) as char,
        _ => unreachable!("nibble out of range"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn validate_skill_key_rejects_path_separators() {
        assert!(validate_skill_key("good-key").is_ok());
        assert!(validate_skill_key("").is_err());
        assert!(validate_skill_key("bad/key").is_err());
        assert!(validate_skill_key("bad\\key").is_err());
        assert!(validate_skill_key("..").is_err());
        assert!(validate_skill_key(".").is_err());
    }

    #[test]
    fn extract_object_key_parses_s3_uri() {
        assert_eq!(
            extract_object_key("s3://bucket/skills/tenant/diagnose/abc.zip").unwrap(),
            "skills/tenant/diagnose/abc.zip"
        );
        assert_eq!(
            extract_object_key("skills/tenant/diagnose/abc.zip").unwrap(),
            "skills/tenant/diagnose/abc.zip"
        );
    }

    #[test]
    fn normalize_zip_path_strips_root_prefix() {
        let result = normalize_zip_path("diagnose/SKILL.md", "diagnose/").unwrap();
        assert_eq!(result, PathBuf::from("SKILL.md"));

        let result = normalize_zip_path("diagnose/scripts/run.sh", "diagnose/").unwrap();
        assert_eq!(result, PathBuf::from("scripts/run.sh"));
    }

    #[test]
    fn normalize_zip_path_rejects_traversal() {
        assert!(normalize_zip_path("../escape.md", "").is_err());
        assert!(normalize_zip_path("/absolute.md", "").is_err());
    }
}
