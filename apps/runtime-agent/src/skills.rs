use std::fs;
use std::io::{self, Cursor, Read};
use std::path::{Component, Path, PathBuf};

use anyhow::{Context, Result};
use async_trait::async_trait;
use sha2::{Digest, Sha256};
use zip::ZipArchive;

use crate::commands::payload::RuntimeSkillPayload;

const MAX_ARCHIVE_SIZE: u64 = 200 * 1024 * 1024;
const MAX_FILE_COUNT: usize = 10_000;

/// Fetches a skill archive's bytes. The only production implementation goes
/// through control-plane-issued presigned GET URLs (证据地基 spec §8 修订 1:
/// runtime 零对象存储凭证);下载完整性由 archive_checksum_sha256 复核保证,
/// 与取回通道无关。
#[async_trait]
pub trait SkillArchiveFetcher: Send + Sync {
    async fn fetch(&self, skill: &RuntimeSkillPayload) -> Result<Vec<u8>>;
}

pub struct PresignSkillArchiveFetcher {
    control_plane: crate::controlplane::client::ControlPlaneClient,
    http: reqwest::Client,
}

impl PresignSkillArchiveFetcher {
    pub fn new(control_plane: crate::controlplane::client::ControlPlaneClient) -> Self {
        Self {
            control_plane,
            http: reqwest::Client::new(),
        }
    }
}

#[async_trait]
impl SkillArchiveFetcher for PresignSkillArchiveFetcher {
    async fn fetch(&self, skill: &RuntimeSkillPayload) -> Result<Vec<u8>> {
        let presigned = self
            .control_plane
            .presign_skill_archive_download(
                &crate::controlplane::models::PresignSkillArchiveDownloadRequest {
                    archive_object_ref: skill.archive_object_ref.clone(),
                },
            )
            .await
            .with_context(|| {
                format!("failed to presign skill archive download: {}", skill.skill_key)
            })?;
        let response = self
            .http
            .get(&presigned.download_url)
            .send()
            .await
            .with_context(|| format!("failed to fetch skill archive: {}", skill.skill_key))?;
        if !response.status().is_success() {
            let status = response.status();
            anyhow::bail!("skill archive download rejected for {}: {status}", skill.skill_key);
        }
        let bytes = response
            .bytes()
            .await
            .with_context(|| format!("failed to read skill archive body: {}", skill.skill_key))?;
        Ok(bytes.to_vec())
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SyncedSkill {
    pub skill_id: String,
    pub skill_key: String,
    pub content_hash: String,
    pub file_count: u64,
}

/// Materializes one skill into the employee capability cache under the
/// provider-specific skills root (目录与能力投影修订 spec §2); session-time
/// convergence is the sole entry point. Per-item checksum markers make this
/// an incremental no-op when the cache is already current.
pub async fn materialize_skill_to_dir(
    target_dir: &Path,
    temp_root: &Path,
    skill: &RuntimeSkillPayload,
    fetcher: &dyn SkillArchiveFetcher,
) -> Result<SyncedSkill> {
    validate_skill_key(&skill.skill_key)?;
    ensure_safe_install_path(target_dir, temp_root)?;

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

    let archive_bytes = fetcher.fetch(skill).await?;

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

    let mut temp_dir = create_skill_temp_dir(temp_root, &skill.skill_key)?;

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
        let target = temp_dir.path().join(&relative);
        if let Some(parent) = target.parent() {
            fs::create_dir_all(parent)?;
        }

        let mut buf = Vec::new();
        entry.read_to_end(&mut buf)?;
        atomic_write(&target, &buf)?;
        file_count += 1;
    }

    if file_count == 0 {
        anyhow::bail!("skill archive contains no files: {}", skill.skill_key);
    }

    fs::write(
        temp_dir.path().join(".skill-checksum"),
        &skill.archive_checksum_sha256,
    )?;
    atomic_replace_dir(temp_dir.path(), target_dir)?;
    temp_dir.persist();

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

pub fn ensure_safe_install_path(target_dir: &Path, temp_root: &Path) -> Result<()> {
    let root = common_path_prefix(target_dir, temp_root)
        .context("skill target and temp root do not share an agent home")?;
    ensure_no_symlink_ancestors_under(&root, target_dir)?;
    ensure_no_symlink_ancestors_under(&root, temp_root)?;
    ensure_safe_target_path(target_dir)?;
    ensure_safe_target_path(temp_root)?;
    Ok(())
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
        let trimmed = name.trim_end_matches('/');
        if trimmed.is_empty() {
            continue;
        }
        let is_dir_entry = name.ends_with('/');
        let mut parts = trimmed.splitn(2, '/');
        let first = parts.next().unwrap_or_default();
        if parts.next().is_none() && !is_dir_entry {
            // 顶层裸文件(平铺 zip,如 SKILL.md 直接在根):没有公共根可剥。
            return String::new();
        }
        // 根目录自身的显式条目("root/")不否决公共根——此前它会让判定直接
        // 放弃,导致带目录条目的归档整层嵌套落盘(残债交接 §4)。
        if root.is_empty() {
            root = first;
        } else if root != first {
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

fn common_path_prefix(left: &Path, right: &Path) -> Option<PathBuf> {
    let mut prefix = PathBuf::new();
    for (left_component, right_component) in left.components().zip(right.components()) {
        if left_component == right_component {
            prefix.push(left_component.as_os_str());
        } else {
            break;
        }
    }
    (!prefix.as_os_str().is_empty()).then_some(prefix)
}

fn ensure_no_symlink_ancestors_under(root: &Path, path: &Path) -> Result<()> {
    let relative = path.strip_prefix(root).with_context(|| {
        format!(
            "skill path must stay under install root: {}",
            path.display()
        )
    })?;
    let mut current = root.to_path_buf();
    for component in relative.components() {
        current.push(component.as_os_str());
        match fs::symlink_metadata(&current) {
            Ok(metadata) if metadata.file_type().is_symlink() => {
                anyhow::bail!(
                    "skill path component must not be a symlink: {}",
                    current.display()
                );
            }
            Ok(_) => {}
            Err(e) if e.kind() == io::ErrorKind::NotFound => {}
            Err(e) => {
                return Err(e).with_context(|| {
                    format!("inspect skill path component: {}", current.display())
                });
            }
        }
    }
    Ok(())
}

pub struct SkillTempDir {
    path: PathBuf,
    persist: bool,
}

impl SkillTempDir {
    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn persist(&mut self) {
        self.persist = true;
    }
}

impl Drop for SkillTempDir {
    fn drop(&mut self) {
        if !self.persist {
            let _ = fs::remove_dir_all(&self.path);
        }
    }
}

pub fn create_skill_temp_dir(temp_root: &Path, skill_key: &str) -> Result<SkillTempDir> {
    validate_skill_key(skill_key)?;
    if let Some(parent) = temp_root.parent() {
        ensure_no_symlink_ancestors_under(parent, temp_root)?;
    }
    fs::create_dir_all(temp_root)?;

    for attempt in 0..16 {
        let temp_dir = temp_root.join(format!(
            "{}-{}-{}-{}",
            std::process::id(),
            skill_key,
            attempt,
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|d| d.as_nanos())
                .unwrap_or(0)
        ));
        match fs::create_dir(&temp_dir) {
            Ok(()) => {
                return Ok(SkillTempDir {
                    path: temp_dir,
                    persist: false,
                });
            }
            Err(e) if e.kind() == io::ErrorKind::AlreadyExists => continue,
            Err(e) => {
                return Err(e).with_context(|| {
                    format!("create skill temp directory: {}", temp_dir.display())
                });
            }
        }
    }

    anyhow::bail!(
        "failed to create unique skill temp directory under {}",
        temp_root.display()
    )
}

fn atomic_replace_dir(temp_dir: &Path, target_dir: &Path) -> Result<()> {
    let parent = target_dir
        .parent()
        .context("skill target directory has no parent")?;
    fs::create_dir_all(parent)?;
    let backup_dir = reserve_atomic_replace_backup(parent, target_dir)?;

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

fn reserve_atomic_replace_backup(parent: &Path, target_dir: &Path) -> Result<PathBuf> {
    for attempt in 0..16 {
        let backup_dir = parent.join(format!(
            ".{}-backup-{}-{}-{}",
            target_dir
                .file_name()
                .and_then(|name| name.to_str())
                .unwrap_or("skill"),
            std::process::id(),
            attempt,
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .map(|d| d.as_nanos())
                .unwrap_or(0)
        ));
        match fs::create_dir(&backup_dir) {
            Ok(()) => {
                fs::remove_dir(&backup_dir).with_context(|| {
                    format!(
                        "release reserved skill replace backup: {}",
                        backup_dir.display()
                    )
                })?;
                return Ok(backup_dir);
            }
            Err(e) if e.kind() == io::ErrorKind::AlreadyExists => continue,
            Err(e) => {
                return Err(e).with_context(|| {
                    format!("reserve skill replace backup: {}", backup_dir.display())
                });
            }
        }
    }

    anyhow::bail!(
        "failed to reserve skill replace backup under {}",
        parent.display()
    )
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
    fn normalize_zip_path_strips_root_prefix() {
        let result = normalize_zip_path("diagnose/SKILL.md", "diagnose/").unwrap();
        assert_eq!(result, PathBuf::from("SKILL.md"));

        let result = normalize_zip_path("diagnose/scripts/run.sh", "diagnose/").unwrap();
        assert_eq!(result, PathBuf::from("scripts/run.sh"));
    }

    #[test]
    fn common_root_prefix_survives_explicit_directory_entries() {
        // 残债交接 §4:带显式目录条目的归档(macOS zip 常见)此前不剥根。
        let names = vec![
            "probe/".to_string(),
            "probe/SKILL.md".to_string(),
            "probe/scripts/".to_string(),
            "probe/scripts/run.sh".to_string(),
        ];
        assert_eq!(common_root_prefix(&names), "probe/");
    }

    #[test]
    fn common_root_prefix_flat_and_multi_root_archives_do_not_strip() {
        // 平铺 zip:顶层裸文件,不剥。
        let flat = vec!["SKILL.md".to_string(), "scripts/run.sh".to_string()];
        assert_eq!(common_root_prefix(&flat), "");
        // 多根:不剥。
        let multi = vec!["a/SKILL.md".to_string(), "b/x.md".to_string()];
        assert_eq!(common_root_prefix(&multi), "");
    }

    #[test]
    fn normalize_zip_path_rejects_traversal() {
        assert!(normalize_zip_path("../escape.md", "").is_err());
        assert!(normalize_zip_path("/absolute.md", "").is_err());
    }

    #[test]
    fn skill_temp_dir_guard_removes_temp_dir_on_failure() {
        let temp = tempfile::tempdir().expect("tempdir");
        let temp_root = temp.path().join(".skill-tmp");
        let temp_dir = create_skill_temp_dir(&temp_root, "code-review").expect("temp dir");
        fs::write(temp_dir.path().join("partial"), "partial").expect("partial file");
        let leaked_path = temp_dir.path().to_path_buf();

        drop(temp_dir);

        assert!(
            !leaked_path.exists(),
            "temporary skill directory should be removed when not persisted"
        );
    }

    #[test]
    fn skill_temp_dir_guard_persists_after_success() {
        let temp = tempfile::tempdir().expect("tempdir");
        let temp_root = temp.path().join(".skill-tmp");
        let mut temp_dir = create_skill_temp_dir(&temp_root, "code-review").expect("temp dir");
        fs::write(temp_dir.path().join("complete"), "complete").expect("complete file");
        let path = temp_dir.path().to_path_buf();

        temp_dir.persist();
        drop(temp_dir);

        assert!(path.exists(), "persisted skill temp dir should remain");
    }
}
