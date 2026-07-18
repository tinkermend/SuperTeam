//! 技能懒收敛(capability-binding-unification):数字员工技能改为纯逻辑绑定后,
//! 物理物化延迟到任务派发。派发时以 payload 的全量 skills[] 为清单,对员工能力
//! 家目录做收敛:物化新增/更新的技能、prune 清单外的 stale 目录,并用家目录
//! stamp 快速短路已收敛的重复派发。runtime 从不复算 CP 指纹
//! (capability_manifest_version 只透传留痕);收敛判定用本地 skills-only 指纹。

use std::collections::BTreeSet;
use std::fs;
use std::io;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use time::OffsetDateTime;
use time::format_description::well_known::Rfc3339;

use crate::commands::payload::RuntimeSkillPayload;
use crate::skills::{
    SkillArchiveFetcher, materialize_skill_to_dir, remove_skill_dir_if_exists, sha256_hex,
    validate_skill_key,
};

pub const CAPABILITY_STAMP_VERSION: u32 = 1;

/// 家目录 stamp:上次收敛成功(含 prune)后的清单快照。命中 stamp + 每个技能的
/// `.skill-checksum` marker 复验一致时,收敛整体短路(fetcher 零调用、不扫描
/// prune)。
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CapabilityStamp {
    pub version: u32,
    pub provider_type: String,
    pub skills_fingerprint: String,
    #[serde(default)]
    pub capability_manifest_version: Option<String>,
    pub skills: Vec<CapabilityStampSkill>,
    pub converged_at: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CapabilityStampSkill {
    pub skill_key: String,
    pub archive_checksum_sha256: String,
}

/// 单次收敛的结构化报告,随 RunSpec 进 attestation metadata
/// (`capability_convergence`)留痕。
#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct SkillConvergenceReport {
    pub materialized: Vec<String>,
    pub reused: Vec<String>,
    pub pruned: Vec<String>,
    pub prune_skipped: bool,
    pub stamp_hit: bool,
}

/// Provider 专属技能根目录(员工能力家目录下),即 provider_skill_dir 去掉最后
/// 一段 skill key。物化与收敛路径共用这一映射,保证布局单一事实源。
pub fn provider_skills_root(agent_home_dir: &Path, provider_type: &str) -> Result<PathBuf> {
    let provider_root = match provider_type {
        "opencode" => ".opencode",
        "codex" => ".agents",
        "claude-code" => ".claude",
        _ => anyhow::bail!("unsupported provider_type for skill install: {provider_type}"),
    };
    Ok(agent_home_dir.join(provider_root).join("skills"))
}

/// 单个技能在员工能力家目录下的物化目录:`provider_skills_root/<skill_key>`。
pub fn provider_skill_dir(
    agent_home_dir: &Path,
    provider_type: &str,
    skill_key: &str,
) -> Result<PathBuf> {
    validate_skill_key(skill_key)?;
    Ok(provider_skills_root(agent_home_dir, provider_type)?.join(skill_key))
}

pub fn capability_stamp_path(agent_home_dir: &Path, provider_type: &str) -> PathBuf {
    agent_home_dir
        .join(".superteam")
        .join(format!("capability-stamp-{provider_type}.json"))
}

/// 本地 skills-only 指纹:按 skill_key 排序的 "key:checksum\n" 拼接取 sha256。
/// 与 CP 侧 capability_manifest_version 指纹无关,runtime 不复算后者。
pub fn skills_fingerprint(skills: &[RuntimeSkillPayload]) -> String {
    let mut entries: Vec<(&str, &str)> = skills
        .iter()
        .map(|skill| {
            (
                skill.skill_key.as_str(),
                skill.archive_checksum_sha256.as_str(),
            )
        })
        .collect();
    entries.sort();
    let mut buffer = String::new();
    for (key, checksum) in entries {
        buffer.push_str(key);
        buffer.push(':');
        buffer.push_str(checksum);
        buffer.push('\n');
    }
    sha256_hex(buffer.as_bytes())
}

/// 把员工能力家目录收敛到 payload 清单:
/// - stamp + marker 双重命中 → 整体短路;
/// - 未命中 → 逐 key 物化(marker 命中的归 reused,真下载的归 materialized);
/// - `manifest_version.is_some() && allow_prune` 时 prune 清单外条目并写 stamp;
///   prune 被并发防护跳过(allow_prune=false)时不写 stamp,下次派发补删;
///   `manifest_version=None` 为老 CP 兼容模式:只增不删、不写 stamp。
/// - 物化中途失败返回 Err;已成功 key 的目录与 marker 保留,stamp 不写。
pub async fn converge_provider_skills(
    agent_home_dir: &Path,
    provider_type: &str,
    skills: &[RuntimeSkillPayload],
    manifest_version: Option<&str>,
    fetcher: &dyn SkillArchiveFetcher,
    allow_prune: bool,
) -> Result<SkillConvergenceReport> {
    let skills_root = provider_skills_root(agent_home_dir, provider_type)?;
    let fingerprint = skills_fingerprint(skills);
    let stamp_path = capability_stamp_path(agent_home_dir, provider_type);

    if let Some(stamp) = read_capability_stamp(&stamp_path) {
        if stamp.version == CAPABILITY_STAMP_VERSION
            && stamp.skills_fingerprint == fingerprint
            && all_markers_current(&skills_root, skills)
        {
            return Ok(SkillConvergenceReport {
                stamp_hit: true,
                ..SkillConvergenceReport::default()
            });
        }
    }

    let mut report = SkillConvergenceReport::default();
    let temp_root = agent_home_dir.join(".skill-tmp");
    for skill in skills {
        validate_skill_key(&skill.skill_key)?;
        let target_dir = skills_root.join(&skill.skill_key);
        let reused = marker_matches(&target_dir, &skill.archive_checksum_sha256);
        materialize_skill_to_dir(&target_dir, &temp_root, skill, fetcher).await?;
        if reused {
            report.reused.push(skill.skill_key.clone());
        } else {
            report.materialized.push(skill.skill_key.clone());
        }
    }

    // Prune 仅在 CP 声明了能力清单指纹时执行;老 CP(无
    // capability_manifest_version)保持现状语义:只增不删、不写 stamp。
    if manifest_version.is_some() {
        if allow_prune {
            let manifest_keys: BTreeSet<&str> = skills
                .iter()
                .map(|skill| skill.skill_key.as_str())
                .collect();
            report.pruned = prune_unlisted_entries(&skills_root, &manifest_keys)?;
            write_capability_stamp(
                &stamp_path,
                &CapabilityStamp {
                    version: CAPABILITY_STAMP_VERSION,
                    provider_type: provider_type.to_string(),
                    skills_fingerprint: fingerprint,
                    capability_manifest_version: manifest_version.map(ToString::to_string),
                    skills: stamp_skills(skills),
                    converged_at: OffsetDateTime::now_utc()
                        .format(&Rfc3339)
                        .unwrap_or_default(),
                },
            )?;
        } else {
            report.prune_skipped = true;
        }
    }

    Ok(report)
}

fn stamp_skills(skills: &[RuntimeSkillPayload]) -> Vec<CapabilityStampSkill> {
    let mut entries: Vec<CapabilityStampSkill> = skills
        .iter()
        .map(|skill| CapabilityStampSkill {
            skill_key: skill.skill_key.clone(),
            archive_checksum_sha256: skill.archive_checksum_sha256.clone(),
        })
        .collect();
    entries.sort_by(|a, b| a.skill_key.cmp(&b.skill_key));
    entries
}

fn read_capability_stamp(path: &Path) -> Option<CapabilityStamp> {
    let bytes = fs::read(path).ok()?;
    serde_json::from_slice(&bytes).ok()
}

fn write_capability_stamp(path: &Path, stamp: &CapabilityStamp) -> Result<()> {
    let parent = path
        .parent()
        .context("capability stamp path has no parent")?;
    fs::create_dir_all(parent)
        .with_context(|| format!("create capability stamp dir: {}", parent.display()))?;
    let bytes = serde_json::to_vec_pretty(stamp).context("serialize capability stamp")?;
    crate::workspace_files::atomic_write(path, &bytes)
        .with_context(|| format!("write capability stamp: {}", path.display()))
}

fn marker_matches(target_dir: &Path, checksum: &str) -> bool {
    fs::read_to_string(target_dir.join(".skill-checksum"))
        .map(|existing| existing == checksum)
        .unwrap_or(false)
}

/// stamp 命中的复验:清单每个 key 的 marker 必须仍与清单 checksum 一致——
/// 手删/篡改技能目录后 stamp 失效,走重物化自愈。
fn all_markers_current(skills_root: &Path, skills: &[RuntimeSkillPayload]) -> bool {
    skills.iter().all(|skill| {
        marker_matches(
            &skills_root.join(&skill.skill_key),
            &skill.archive_checksum_sha256,
        )
    })
}

/// 列 skills_root 一层条目,名字不在清单 key 集合的目录/符号链接/散文件删除。
/// 以 `.` 开头的隐藏条目(如 `.DS_Store`)不当技能处理,直接跳过。空清单 = 全删。
fn prune_unlisted_entries(
    skills_root: &Path,
    manifest_keys: &BTreeSet<&str>,
) -> Result<Vec<String>> {
    let entries = match fs::read_dir(skills_root) {
        Ok(entries) => entries,
        Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(Vec::new()),
        Err(e) => {
            return Err(e)
                .with_context(|| format!("list provider skills root: {}", skills_root.display()));
        }
    };

    let mut pruned = Vec::new();
    for entry in entries {
        let entry = entry
            .with_context(|| format!("read provider skills entry: {}", skills_root.display()))?;
        let name = entry.file_name().to_string_lossy().into_owned();
        if name.starts_with('.') {
            continue;
        }
        if manifest_keys.contains(name.as_str()) {
            continue;
        }
        let path = entry.path();
        // DirEntry::file_type 不跟随符号链接:链接本身按文件删,不误入目标。
        let file_type = entry
            .file_type()
            .with_context(|| format!("inspect stale skill entry: {}", path.display()))?;
        if file_type.is_dir() {
            remove_skill_dir_if_exists(&path)?;
        } else {
            fs::remove_file(&path)
                .with_context(|| format!("remove stale skill entry: {}", path.display()))?;
        }
        pruned.push(name);
    }
    pruned.sort();
    Ok(pruned)
}

#[cfg(test)]
mod tests {
    use super::*;

    use std::collections::{HashMap, HashSet};
    use std::io::Write;
    use std::sync::atomic::{AtomicUsize, Ordering};

    use async_trait::async_trait;

    struct FakeFetcher {
        archives: HashMap<String, Vec<u8>>,
        calls: AtomicUsize,
        fail_keys: HashSet<String>,
    }

    impl FakeFetcher {
        fn new(archives: &[(&str, Vec<u8>)]) -> Self {
            Self {
                archives: archives
                    .iter()
                    .map(|(key, bytes)| (key.to_string(), bytes.clone()))
                    .collect(),
                calls: AtomicUsize::new(0),
                fail_keys: HashSet::new(),
            }
        }

        fn failing_on(mut self, key: &str) -> Self {
            self.fail_keys.insert(key.to_string());
            self
        }

        fn calls(&self) -> usize {
            self.calls.load(Ordering::SeqCst)
        }
    }

    #[async_trait]
    impl SkillArchiveFetcher for FakeFetcher {
        async fn fetch(&self, skill: &RuntimeSkillPayload) -> Result<Vec<u8>> {
            self.calls.fetch_add(1, Ordering::SeqCst);
            if self.fail_keys.contains(&skill.skill_key) {
                anyhow::bail!("fake fetch failure for {}", skill.skill_key);
            }
            self.archives
                .get(&skill.skill_key)
                .cloned()
                .ok_or_else(|| anyhow::anyhow!("no fake archive for {}", skill.skill_key))
        }
    }

    fn zip_archive(files: &[(&str, &str)]) -> Vec<u8> {
        let mut cursor = std::io::Cursor::new(Vec::new());
        {
            let mut writer = zip::ZipWriter::new(&mut cursor);
            let options = zip::write::SimpleFileOptions::default();
            for (name, content) in files {
                writer.start_file(*name, options).expect("start zip file");
                writer
                    .write_all(content.as_bytes())
                    .expect("write zip file");
            }
            writer.finish().expect("finish zip");
        }
        cursor.into_inner()
    }

    fn skill_payload(key: &str, archive: &[u8]) -> RuntimeSkillPayload {
        RuntimeSkillPayload {
            skill_id: format!("skill-{key}"),
            skill_key: key.to_string(),
            revision_id: None,
            archive_object_ref: format!("mem://{key}.zip"),
            archive_checksum_sha256: sha256_hex(archive),
            archive_size_bytes: archive.len() as i64,
            archive_file_count: 1,
        }
    }

    fn two_skill_fixture() -> (Vec<RuntimeSkillPayload>, FakeFetcher) {
        let alpha = zip_archive(&[("SKILL.md", "alpha instructions")]);
        let beta = zip_archive(&[("SKILL.md", "beta instructions")]);
        let skills = vec![skill_payload("alpha", &alpha), skill_payload("beta", &beta)];
        let fetcher = FakeFetcher::new(&[("alpha", alpha), ("beta", beta)]);
        (skills, fetcher)
    }

    #[tokio::test]
    async fn empty_home_materializes_manifest_and_writes_stamp() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();

        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:abc"),
            &fetcher,
            true,
        )
        .await
        .expect("converge");

        assert_eq!(report.materialized, vec!["alpha", "beta"]);
        assert!(report.reused.is_empty());
        assert!(report.pruned.is_empty());
        assert!(!report.prune_skipped);
        assert!(!report.stamp_hit);
        assert_eq!(fetcher.calls(), 2);
        let root = home.path().join(".claude/skills");
        assert!(root.join("alpha/SKILL.md").is_file());
        assert!(root.join("beta/SKILL.md").is_file());

        let stamp = read_capability_stamp(&capability_stamp_path(home.path(), "claude-code"))
            .expect("stamp written");
        assert_eq!(stamp.version, CAPABILITY_STAMP_VERSION);
        assert_eq!(stamp.skills_fingerprint, skills_fingerprint(&skills));
        assert_eq!(
            stamp.capability_manifest_version.as_deref(),
            Some("cmv1:sha256:abc")
        );
        assert_eq!(stamp.skills.len(), 2);
    }

    #[tokio::test]
    async fn same_manifest_hits_stamp_with_zero_fetches() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();
        converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:abc"),
            &fetcher,
            true,
        )
        .await
        .expect("first converge");
        let calls_after_first = fetcher.calls();

        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:abc"),
            &fetcher,
            true,
        )
        .await
        .expect("second converge");

        assert!(report.stamp_hit);
        assert!(report.materialized.is_empty());
        assert!(report.reused.is_empty());
        assert!(report.pruned.is_empty());
        assert_eq!(
            fetcher.calls(),
            calls_after_first,
            "stamp hit must not fetch"
        );
    }

    #[tokio::test]
    async fn removed_key_is_pruned_and_re_added_key_rematerializes() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();
        converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:v1"),
            &fetcher,
            true,
        )
        .await
        .expect("seed converge");

        let only_alpha = vec![skills[0].clone()];
        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &only_alpha,
            Some("cmv1:sha256:v2"),
            &fetcher,
            true,
        )
        .await
        .expect("prune converge");
        assert_eq!(report.reused, vec!["alpha"]);
        assert_eq!(report.pruned, vec!["beta"]);
        let root = home.path().join(".claude/skills");
        assert!(!root.join("beta").exists(), "stale skill must be pruned");
        let stamp = read_capability_stamp(&capability_stamp_path(home.path(), "claude-code"))
            .expect("stamp updated");
        assert_eq!(stamp.skills_fingerprint, skills_fingerprint(&only_alpha));

        // 加回 beta → 重新物化。
        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:v1"),
            &fetcher,
            true,
        )
        .await
        .expect("re-add converge");
        assert_eq!(report.materialized, vec!["beta"]);
        assert_eq!(report.reused, vec!["alpha"]);
        assert!(root.join("beta/SKILL.md").is_file());
    }

    #[tokio::test]
    async fn stamp_hit_with_deleted_skill_dir_self_heals() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();
        converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:abc"),
            &fetcher,
            true,
        )
        .await
        .expect("seed converge");
        let root = home.path().join(".claude/skills");
        fs::remove_dir_all(root.join("beta")).expect("simulate manual deletion");

        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:abc"),
            &fetcher,
            true,
        )
        .await
        .expect("healing converge");

        assert!(
            !report.stamp_hit,
            "marker recheck must invalidate the stamp"
        );
        assert_eq!(report.materialized, vec!["beta"]);
        assert_eq!(report.reused, vec!["alpha"]);
        assert!(root.join("beta/SKILL.md").is_file());
    }

    #[tokio::test]
    async fn prune_disallowed_keeps_stale_and_stamp_absent_until_next_converge() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();
        converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:v1"),
            &fetcher,
            true,
        )
        .await
        .expect("seed converge");
        let root = home.path().join(".claude/skills");
        let stamp_path = capability_stamp_path(home.path(), "claude-code");

        let only_alpha = vec![skills[0].clone()];
        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &only_alpha,
            Some("cmv1:sha256:v2"),
            &fetcher,
            false,
        )
        .await
        .expect("guarded converge");
        assert!(report.prune_skipped);
        assert!(report.pruned.is_empty());
        assert!(root.join("beta").exists(), "stale skill kept while guarded");
        let stale_stamp = read_capability_stamp(&stamp_path).expect("old stamp still present");
        assert_eq!(
            stale_stamp.skills_fingerprint,
            skills_fingerprint(&skills),
            "guarded converge must not update the stamp"
        );

        // 下次派发 allow_prune=true → 补删 + stamp 更新。
        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &only_alpha,
            Some("cmv1:sha256:v2"),
            &fetcher,
            true,
        )
        .await
        .expect("catch-up converge");
        assert!(!report.prune_skipped);
        assert_eq!(report.pruned, vec!["beta"]);
        assert!(!root.join("beta").exists());
        let stamp = read_capability_stamp(&stamp_path).expect("stamp updated");
        assert_eq!(stamp.skills_fingerprint, skills_fingerprint(&only_alpha));
    }

    #[tokio::test]
    async fn missing_manifest_version_never_prunes_and_never_stamps() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();
        converge_provider_skills(home.path(), "claude-code", &skills, None, &fetcher, true)
            .await
            .expect("seed converge");
        let root = home.path().join(".claude/skills");
        assert!(root.join("alpha/SKILL.md").is_file());
        assert!(
            read_capability_stamp(&capability_stamp_path(home.path(), "claude-code")).is_none(),
            "legacy mode must not write a stamp"
        );

        let only_alpha = vec![skills[0].clone()];
        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &only_alpha,
            None,
            &fetcher,
            true,
        )
        .await
        .expect("legacy converge");
        assert!(report.pruned.is_empty());
        assert!(!report.prune_skipped);
        assert!(root.join("beta").exists(), "legacy mode keeps stale skills");
    }

    #[tokio::test]
    async fn empty_manifest_with_version_prunes_everything_and_writes_empty_stamp() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();
        converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:v1"),
            &fetcher,
            true,
        )
        .await
        .expect("seed converge");

        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &[],
            Some("cmv1:sha256:empty"),
            &fetcher,
            true,
        )
        .await
        .expect("empty converge");

        assert_eq!(report.pruned, vec!["alpha", "beta"]);
        let root = home.path().join(".claude/skills");
        assert!(!root.join("alpha").exists());
        assert!(!root.join("beta").exists());
        let stamp = read_capability_stamp(&capability_stamp_path(home.path(), "claude-code"))
            .expect("empty stamp written");
        assert!(stamp.skills.is_empty());
        assert_eq!(stamp.skills_fingerprint, skills_fingerprint(&[]));
    }

    #[tokio::test]
    async fn mid_flight_fetch_failure_keeps_succeeded_keys_and_no_stamp() {
        let home = tempfile::tempdir().expect("tempdir");
        let alpha = zip_archive(&[("SKILL.md", "alpha instructions")]);
        let beta = zip_archive(&[("SKILL.md", "beta instructions")]);
        let skills = vec![skill_payload("alpha", &alpha), skill_payload("beta", &beta)];
        let fetcher = FakeFetcher::new(&[("alpha", alpha), ("beta", beta)]).failing_on("beta");

        let error = converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:v1"),
            &fetcher,
            true,
        )
        .await
        .expect_err("beta fetch failure must fail the convergence");

        assert!(error.to_string().contains("fake fetch failure"));
        let root = home.path().join(".claude/skills");
        assert!(
            root.join("alpha/SKILL.md").is_file(),
            "succeeded key survives (marker skips it next time)"
        );
        assert!(!root.join("beta").exists());
        assert!(
            read_capability_stamp(&capability_stamp_path(home.path(), "claude-code")).is_none(),
            "stamp must not be written after a partial failure"
        );
    }

    #[cfg(unix)]
    #[tokio::test]
    async fn prune_skips_hidden_entries_and_removes_unlisted_symlinks() {
        let home = tempfile::tempdir().expect("tempdir");
        let (skills, fetcher) = two_skill_fixture();
        // 老 CP 模式播种(无 stamp),避免后续同清单收敛走 stamp 短路跳过 prune 扫描。
        converge_provider_skills(home.path(), "claude-code", &skills, None, &fetcher, true)
            .await
            .expect("seed converge");
        let root = home.path().join(".claude/skills");
        fs::write(root.join(".DS_Store"), b"finder noise").expect("hidden file");
        std::os::unix::fs::symlink(root.join("alpha"), root.join("stale-link"))
            .expect("stale symlink");

        let report = converge_provider_skills(
            home.path(),
            "claude-code",
            &skills,
            Some("cmv1:sha256:v1"),
            &fetcher,
            true,
        )
        .await
        .expect("prune converge");

        assert_eq!(report.pruned, vec!["stale-link"]);
        assert!(
            root.join(".DS_Store").exists(),
            "hidden entries are not skills"
        );
        assert!(!root.join("stale-link").exists());
        assert!(
            root.join("alpha/SKILL.md").is_file(),
            "symlink removal must not touch the link target"
        );
    }

    #[test]
    fn skills_fingerprint_is_order_insensitive_and_content_sensitive() {
        let alpha = zip_archive(&[("SKILL.md", "alpha")]);
        let beta = zip_archive(&[("SKILL.md", "beta")]);
        let forward = vec![skill_payload("alpha", &alpha), skill_payload("beta", &beta)];
        let reversed = vec![skill_payload("beta", &beta), skill_payload("alpha", &alpha)];
        assert_eq!(skills_fingerprint(&forward), skills_fingerprint(&reversed));

        let changed = vec![skill_payload("alpha", &beta), skill_payload("beta", &beta)];
        assert_ne!(skills_fingerprint(&forward), skills_fingerprint(&changed));
    }

    #[test]
    fn provider_skills_root_matches_provider_skill_dir_parent() {
        let home = Path::new("/tmp/agent-home");
        for provider in ["claude-code", "codex", "opencode"] {
            let dir = provider_skill_dir(home, provider, "k").expect("skill dir");
            assert_eq!(
                provider_skills_root(home, provider).expect("skills root"),
                dir.parent().expect("parent").to_path_buf()
            );
        }
        assert!(provider_skills_root(home, "unknown").is_err());
    }

    #[test]
    fn provider_skill_dir_maps_official_provider_skill_directories() {
        let agent_home = Path::new("/tmp/superteam/agent-home");
        assert_eq!(
            provider_skill_dir(agent_home, "opencode", "code-review").unwrap(),
            agent_home.join(".opencode/skills/code-review")
        );
        assert_eq!(
            provider_skill_dir(agent_home, "codex", "code-review").unwrap(),
            agent_home.join(".agents/skills/code-review")
        );
        assert_eq!(
            provider_skill_dir(agent_home, "claude-code", "code-review").unwrap(),
            agent_home.join(".claude/skills/code-review")
        );
    }

    #[test]
    fn provider_skill_dir_rejects_unsupported_provider_and_unsafe_keys() {
        let agent_home = Path::new("/tmp/superteam/agent-home");
        let error = provider_skill_dir(agent_home, "unknown", "review")
            .expect_err("unsupported provider should fail");
        assert!(error.to_string().contains("unsupported provider_type"));
        for key in ["../review", "bad/key", ""] {
            assert!(
                provider_skill_dir(agent_home, "codex", key).is_err(),
                "expected unsafe skill key to fail: {key:?}"
            );
        }
    }
}
