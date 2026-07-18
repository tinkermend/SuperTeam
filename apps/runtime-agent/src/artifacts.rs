//! Artifact collection & content-addressed upload (证据地基 spec §4.1/§8).
//!
//! After the provider finishes and before the terminal writeback, the runtime
//! collects up to three artifacts per attempt:
//!
//! | type                   | source                       | evidence? |
//! |------------------------|------------------------------|-----------|
//! | `execution_transcript` | local raw.jsonl buffer       | yes       |
//! | `diff`                 | `git diff HEAD` in workspace | yes       |
//! | `conclusion`           | provider summary text        | no (self report) |
//!
//! Unlike the raw objects (uploaded verbatim, spec §3.5 of the transcript
//! spec), the transcript artifact is the human-retrievable copy: it is
//! REDACTED before hashing, so the stored bytes are the redacted bytes.
//! Uploads go through control-plane presigned URLs; the runtime holds no
//! object-store credentials.

use std::collections::BTreeMap;
use std::path::{Path, PathBuf};

use anyhow::{Context, Result};
use sha2::{Digest, Sha256};

use crate::controlplane::client::ControlPlaneClient;
use crate::controlplane::models::PresignArtifactUploadRequest;

/// Single-file cap (spec §4.1 限额); the control plane enforces the same cap
/// at presign time.
pub const MAX_ARTIFACT_FILE_BYTES: usize = 10 * 1024 * 1024;

/// Attachment caps (输出附件 spec §1.4): attachments are best-effort captures,
/// not evidence — oversized files are skipped (not truncated) with a note.
pub const MAX_ATTACHMENT_FILE_BYTES: u64 = 5 * 1024 * 1024;
pub const MAX_ATTACHMENT_COUNT: usize = 20;
pub const MAX_ATTACHMENT_TOTAL_BYTES: u64 = 50 * 1024 * 1024;

/// Extension whitelist (输出附件 spec §1.2, 用户拍板 2026-07-19). Files the
/// attempt newly created that match land as `execution_output` attachments.
const ATTACHMENT_EXTENSIONS: &[(&str, &str)] = &[
    ("html", "text/html"),
    ("md", "text/markdown"),
    ("txt", "text/plain"),
    ("csv", "text/csv"),
    ("json", "application/json"),
    (
        "docx",
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    ),
    ("doc", "application/msword"),
    (
        "xlsx",
        "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    ),
];

/// Noise directories (输出附件 spec §1.3) — second line of defence behind
/// .gitignore for vendored/build trees that are not ignored. Hidden (dot)
/// path components are excluded wholesale on top of this list: runtime and
/// provider metadata dirs (.superteam/.claude/...) can carry sensitive
/// config, and hidden files are never deliverables (E2E 2026-07-19 caught
/// .superteam/mcp/claude.mcp.json being captured as an attachment).
const ATTACHMENT_EXCLUDED_DIRS: &[&str] = &[
    "node_modules",
    "target",
    "dist",
    "build",
    "vendor",
    "__pycache__",
];

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CollectedArtifact {
    pub artifact_type: &'static str,
    pub name: String,
    pub content_type: &'static str,
    pub is_evidence: bool,
    pub truncated: bool,
    pub redaction_count: usize,
    pub sha256: String,
    pub bytes: Vec<u8>,
}

/// A workspace file captured as an `execution_output` attachment.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CollectedAttachment {
    pub artifact: CollectedArtifact,
    pub relative_path: String,
}

/// A candidate file dropped by a cap — surfaced as a self-report artifact row
/// so the drop is visible to humans (no silent caps, spec §1.4).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SkippedAttachment {
    pub relative_path: String,
    pub reason: String,
}

#[derive(Debug, Default)]
pub struct AttachmentCollection {
    pub attachments: Vec<CollectedAttachment>,
    pub skipped: Vec<SkippedAttachment>,
}

pub struct ArtifactCollectionInputs<'a> {
    /// Local raw transcript buffer written by the raw log sink.
    pub raw_log_path: PathBuf,
    /// Git worktree the attempt executed in, when one exists.
    pub workspace_path: Option<PathBuf>,
    /// Provider summary text; becomes conclusion.md (self report, not evidence).
    pub conclusion: Option<&'a str>,
    /// Provider process environment for value-based redaction.
    pub environment: &'a BTreeMap<String, String>,
}

/// Collects the attempt's artifacts from local state. Pure collection — no
/// network. A missing raw.jsonl yields no transcript artifact; the control
/// plane's zero-evidence gate then rejects the completion (spec §5).
pub async fn collect_artifacts(inputs: &ArtifactCollectionInputs<'_>) -> Vec<CollectedArtifact> {
    let mut collected = Vec::with_capacity(3);

    match tokio::fs::read(&inputs.raw_log_path).await {
        Ok(raw) if !raw.is_empty() => {
            let (redacted, redaction_count) = redact_transcript(&raw, inputs.environment);
            let (body, truncated) = truncate_keep_tail(redacted, MAX_ARTIFACT_FILE_BYTES);
            collected.push(build_artifact(
                "execution_transcript",
                "raw.jsonl".to_string(),
                "application/x-ndjson",
                true,
                truncated,
                redaction_count,
                body,
            ));
        }
        Ok(_) => {}
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => {}
        Err(error) => {
            eprintln!(
                "artifact collection: failed to read raw transcript {:?}: {error}",
                inputs.raw_log_path
            );
        }
    }

    if let Some(workspace) = &inputs.workspace_path {
        // v1 采集未提交的 tracked 变更;员工已 commit 的变更依赖
        // attestation 的 git 证据字段(后续接线),transcript 仍是核心证据。
        match git_diff_head(workspace).await {
            Ok(Some(diff)) => {
                let (redacted, redaction_count) = redact_transcript(&diff, inputs.environment);
                let (body, truncated) = truncate_keep_head(redacted, MAX_ARTIFACT_FILE_BYTES);
                collected.push(build_artifact(
                    "diff",
                    "changes.diff".to_string(),
                    "text/x-diff",
                    true,
                    truncated,
                    redaction_count,
                    body,
                ));
            }
            Ok(None) => {}
            Err(error) => {
                eprintln!("artifact collection: git diff failed in {workspace:?}: {error}");
            }
        }
    }

    if let Some(conclusion) = inputs.conclusion {
        let trimmed = conclusion.trim();
        if !trimmed.is_empty() {
            let (body, truncated) =
                truncate_keep_head(trimmed.as_bytes().to_vec(), MAX_ARTIFACT_FILE_BYTES);
            collected.push(build_artifact(
                "conclusion",
                "conclusion.md".to_string(),
                "text/markdown",
                false,
                truncated,
                0,
                body,
            ));
        }
    }

    collected
}

/// Uploads each artifact through a presigned URL (skipping content the store
/// already has) and returns the object-form `artifact_refs` entries for the
/// result contract. Any upload failure fails the whole batch: the completion
/// must not claim artifacts that never landed (spec §3.4/§5).
pub async fn upload_artifacts(
    control_plane: &ControlPlaneClient,
    artifacts: Vec<CollectedArtifact>,
) -> Result<Vec<serde_json::Value>> {
    let http = reqwest::Client::new();
    let mut refs = Vec::with_capacity(artifacts.len());
    for artifact in artifacts {
        let object_key = upload_one(control_plane, &http, &artifact)
            .await
            .with_context(|| format!("upload artifact {}", artifact.name))?;
        refs.push(serde_json::json!({
            "type": artifact.artifact_type,
            "name": artifact.name,
            "ref": object_key,
            "sha256": artifact.sha256,
            "size_bytes": artifact.bytes.len() as i64,
            "content_type": artifact.content_type,
            "truncated": artifact.truncated,
            "is_evidence": artifact.is_evidence,
            "redaction_count": artifact.redaction_count,
        }));
    }
    Ok(refs)
}

/// Collects `execution_output` attachments (输出附件 spec §1): files the
/// attempt newly created — untracked per git, or new vs. the pre-run snapshot
/// in non-git workspaces — filtered by the extension whitelist and noise-dir
/// exclusions, capped per §1.4. Attachments are NOT redacted (spec §1.5: agent
/// 原样产出, UI 标注) and oversized candidates are skipped, never truncated.
pub async fn collect_attachments(
    workspace: &Path,
    baseline: Option<&std::collections::BTreeSet<String>>,
) -> AttachmentCollection {
    let mut collection = AttachmentCollection::default();

    let candidates = match git_untracked_files(workspace).await {
        Some(paths) => paths,
        None => match baseline {
            // Non-git workspace: everything not present at run start is new.
            Some(baseline) => snapshot_workspace_files(workspace)
                .into_iter()
                .filter(|path| !baseline.contains(path))
                .collect(),
            None => return collection,
        },
    };

    // (relative path, size, mtime) for whitelist survivors within caps.
    let mut eligible: Vec<(String, u64, std::time::SystemTime)> = Vec::new();
    for relative in candidates {
        if attachment_content_type(&relative).is_none() || has_excluded_component(&relative) {
            continue;
        }
        let absolute = workspace.join(&relative);
        let Ok(metadata) = tokio::fs::metadata(&absolute).await else {
            continue;
        };
        if !metadata.is_file() {
            continue;
        }
        if metadata.len() > MAX_ATTACHMENT_FILE_BYTES {
            collection.skipped.push(SkippedAttachment {
                relative_path: relative,
                reason: format!(
                    "file is {} bytes, exceeds the {}MiB attachment cap",
                    metadata.len(),
                    MAX_ATTACHMENT_FILE_BYTES / (1024 * 1024)
                ),
            });
            continue;
        }
        let modified = metadata
            .modified()
            .unwrap_or(std::time::SystemTime::UNIX_EPOCH);
        eligible.push((relative, metadata.len(), modified));
    }

    // Newest first: when a cap bites, the most recent outputs win (§1.4).
    eligible.sort_by(|a, b| b.2.cmp(&a.2));

    let mut total_bytes = 0u64;
    for (relative, size, _) in eligible {
        if collection.attachments.len() >= MAX_ATTACHMENT_COUNT {
            collection.skipped.push(SkippedAttachment {
                relative_path: relative,
                reason: format!("attachment count cap ({MAX_ATTACHMENT_COUNT}) reached"),
            });
            continue;
        }
        if total_bytes + size > MAX_ATTACHMENT_TOTAL_BYTES {
            collection.skipped.push(SkippedAttachment {
                relative_path: relative,
                reason: format!(
                    "total attachment cap ({}MiB) reached",
                    MAX_ATTACHMENT_TOTAL_BYTES / (1024 * 1024)
                ),
            });
            continue;
        }
        let bytes = match tokio::fs::read(workspace.join(&relative)).await {
            Ok(bytes) => bytes,
            Err(error) => {
                collection.skipped.push(SkippedAttachment {
                    relative_path: relative,
                    reason: format!("read failed: {error}"),
                });
                continue;
            }
        };
        // Re-check after the read: the file may have grown since stat.
        if bytes.len() as u64 > MAX_ATTACHMENT_FILE_BYTES {
            collection.skipped.push(SkippedAttachment {
                relative_path: relative,
                reason: format!(
                    "file grew to {} bytes, exceeds the {}MiB attachment cap",
                    bytes.len(),
                    MAX_ATTACHMENT_FILE_BYTES / (1024 * 1024)
                ),
            });
            continue;
        }
        total_bytes += bytes.len() as u64;
        let content_type = attachment_content_type(&relative)
            .expect("eligible entries passed the whitelist filter");
        let name = Path::new(&relative)
            .file_name()
            .map(|name| name.to_string_lossy().into_owned())
            .unwrap_or_else(|| relative.clone());
        collection.attachments.push(CollectedAttachment {
            artifact: build_artifact("execution_output", name, content_type, false, false, 0, bytes),
            relative_path: relative,
        });
    }

    collection
}

/// Uploads attachments best-effort (spec §1.5): a failed attachment becomes a
/// visible skip note instead of failing the completion — unlike evidence,
/// which stays all-or-nothing. Returns ref entries for the writeback: uploaded
/// object refs first, then self-report skip notes (no sha256 → the control
/// plane keeps them as non-retrievable artifact rows for humans).
pub async fn upload_attachments_best_effort(
    control_plane: &ControlPlaneClient,
    collection: AttachmentCollection,
) -> Vec<serde_json::Value> {
    let http = reqwest::Client::new();
    let mut refs = Vec::new();
    let mut skipped = collection.skipped;
    for attachment in collection.attachments {
        match upload_one(control_plane, &http, &attachment.artifact).await {
            Ok(object_key) => refs.push(serde_json::json!({
                "type": attachment.artifact.artifact_type,
                "name": attachment.artifact.name,
                "ref": object_key,
                "sha256": attachment.artifact.sha256,
                "size_bytes": attachment.artifact.bytes.len() as i64,
                "content_type": attachment.artifact.content_type,
                "truncated": false,
                "is_evidence": false,
                "redaction_count": 0,
                "relative_path": attachment.relative_path,
            })),
            Err(error) => skipped.push(SkippedAttachment {
                relative_path: attachment.relative_path,
                reason: format!("upload failed: {error:#}"),
            }),
        }
    }
    for skip in skipped {
        eprintln!(
            "attachment skipped: {} ({})",
            skip.relative_path, skip.reason
        );
        refs.push(serde_json::json!({
            "type": "execution_output_skipped",
            "ref": skip.relative_path.clone(),
            "title": format!("{} — {}", skip.relative_path, skip.reason),
        }));
    }
    refs
}

fn attachment_content_type(relative_path: &str) -> Option<&'static str> {
    let extension = Path::new(relative_path).extension()?.to_str()?;
    let extension = extension.to_ascii_lowercase();
    ATTACHMENT_EXTENSIONS
        .iter()
        .find(|(candidate, _)| *candidate == extension)
        .map(|(_, content_type)| *content_type)
}

fn has_excluded_component(relative_path: &str) -> bool {
    Path::new(relative_path).components().any(|component| {
        component.as_os_str().to_str().is_some_and(|name| {
            name.starts_with('.') || ATTACHMENT_EXCLUDED_DIRS.contains(&name)
        })
    })
}

/// Untracked files per git (`--exclude-standard` respects .gitignore — spec
/// §1.1 known limitation: deliverables written into ignored paths are not
/// captured). None when the workspace is not a git worktree.
async fn git_untracked_files(workspace: &Path) -> Option<Vec<String>> {
    let output = tokio::process::Command::new("git")
        .arg("-C")
        .arg(workspace)
        .args(["ls-files", "--others", "--exclude-standard", "-z"])
        .output()
        .await
        .ok()?;
    if !output.status.success() {
        return None;
    }
    Some(
        output
            .stdout
            .split(|byte| *byte == 0)
            .filter(|path| !path.is_empty())
            .map(|path| String::from_utf8_lossy(path).into_owned())
            .collect(),
    )
}

/// Bounded relative-path listing for non-git workspaces: the pre-run baseline
/// and its post-run counterpart (spec §1.1 snapshot fallback). Skips noise
/// dirs; stops at 5000 entries — beyond that a scratch dir is pathological
/// and the attachment count cap would bite anyway.
pub fn snapshot_workspace_files(workspace: &Path) -> std::collections::BTreeSet<String> {
    const MAX_SNAPSHOT_ENTRIES: usize = 5000;
    let mut files = std::collections::BTreeSet::new();
    let mut stack = vec![workspace.to_path_buf()];
    while let Some(dir) = stack.pop() {
        let Ok(entries) = std::fs::read_dir(&dir) else {
            continue;
        };
        for entry in entries.flatten() {
            if files.len() >= MAX_SNAPSHOT_ENTRIES {
                return files;
            }
            let path = entry.path();
            let Ok(file_type) = entry.file_type() else {
                continue;
            };
            if file_type.is_dir() {
                let name = entry.file_name();
                let excluded = name.to_str().is_some_and(|name| {
                    name.starts_with('.') || ATTACHMENT_EXCLUDED_DIRS.contains(&name)
                });
                if !excluded {
                    stack.push(path);
                }
            } else if file_type.is_file() {
                if let Ok(relative) = path.strip_prefix(workspace) {
                    files.insert(relative.to_string_lossy().into_owned());
                }
            }
        }
    }
    files
}

async fn upload_one(
    control_plane: &ControlPlaneClient,
    http: &reqwest::Client,
    artifact: &CollectedArtifact,
) -> Result<String> {
    let mut delay = std::time::Duration::from_millis(200);
    let mut last_error = None;
    for _ in 0..3 {
        match try_upload_once(control_plane, http, artifact).await {
            Ok(key) => return Ok(key),
            Err(error) => {
                last_error = Some(error);
                tokio::time::sleep(delay).await;
                delay *= 2;
            }
        }
    }
    Err(last_error.unwrap_or_else(|| anyhow::anyhow!("unknown artifact upload failure")))
}

async fn try_upload_once(
    control_plane: &ControlPlaneClient,
    http: &reqwest::Client,
    artifact: &CollectedArtifact,
) -> Result<String> {
    let presigned = control_plane
        .presign_artifact_upload(&PresignArtifactUploadRequest {
            sha256: artifact.sha256.clone(),
            size_bytes: artifact.bytes.len() as i64,
            content_type: artifact.content_type.to_string(),
        })
        .await?;
    if presigned.already_exists {
        return Ok(presigned.object_key);
    }
    let upload_url = presigned
        .upload_url
        .clone()
        .context("presign response carries no upload_url")?;
    let response = http
        .put(&upload_url)
        // Signed over this exact Content-Type; must match the presign input.
        .header(reqwest::header::CONTENT_TYPE, artifact.content_type)
        .body(artifact.bytes.clone())
        .send()
        .await
        .context("artifact PUT failed")?;
    if !response.status().is_success() {
        let status = response.status();
        let detail = response.text().await.unwrap_or_default();
        anyhow::bail!("artifact upload rejected: {status} {detail}");
    }
    Ok(presigned.object_key)
}

fn build_artifact(
    artifact_type: &'static str,
    name: String,
    content_type: &'static str,
    is_evidence: bool,
    truncated: bool,
    redaction_count: usize,
    bytes: Vec<u8>,
) -> CollectedArtifact {
    let sha256 = hex(&Sha256::digest(&bytes));
    CollectedArtifact {
        artifact_type,
        name,
        content_type,
        is_evidence,
        truncated,
        redaction_count,
        sha256,
        bytes,
    }
}

/// Line-based redaction; the hash is computed AFTER redaction so the stored
/// bytes equal the hashed bytes (spec §4.1.1: 脱敏先于 sha256).
fn redact_transcript(raw: &[u8], environment: &BTreeMap<String, String>) -> (Vec<u8>, usize) {
    let text = String::from_utf8_lossy(raw);
    let mut out = String::with_capacity(text.len());
    let mut redaction_count = 0usize;
    for line in text.split_inclusive('\n') {
        let redacted = crate::redaction::redact_with_environment(line, environment);
        redaction_count += redacted.matches("[REDACTED:").count();
        out.push_str(&redacted);
    }
    (out.into_bytes(), redaction_count)
}

/// Keeps the TAIL on overflow — the end of a transcript holds the final tool
/// results and the conclusion (spec §5). Cuts on a line boundary.
fn truncate_keep_tail(bytes: Vec<u8>, limit: usize) -> (Vec<u8>, bool) {
    if bytes.len() <= limit {
        return (bytes, false);
    }
    let start = bytes.len() - limit;
    let start = bytes[start..]
        .iter()
        .position(|byte| *byte == b'\n')
        .map(|offset| start + offset + 1)
        .unwrap_or(start);
    (bytes[start..].to_vec(), true)
}

fn truncate_keep_head(bytes: Vec<u8>, limit: usize) -> (Vec<u8>, bool) {
    if bytes.len() <= limit {
        return (bytes, false);
    }
    (bytes[..limit].to_vec(), true)
}

/// Uncommitted tracked changes in the attempt worktree; None when clean or
/// not a git worktree.
async fn git_diff_head(workspace: &Path) -> Result<Option<Vec<u8>>> {
    let output = tokio::process::Command::new("git")
        .arg("-C")
        .arg(workspace)
        .args(["diff", "HEAD"])
        .output()
        .await
        .context("spawn git diff")?;
    if !output.status.success() {
        // Not a git worktree (or no HEAD yet) — not an error for collection.
        return Ok(None);
    }
    if output.stdout.is_empty() {
        return Ok(None);
    }
    Ok(Some(output.stdout))
}

fn hex(bytes: &[u8]) -> String {
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push_str(&format!("{byte:02x}"));
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn no_env() -> BTreeMap<String, String> {
        BTreeMap::new()
    }

    #[tokio::test]
    async fn collects_transcript_with_redaction_before_hash() {
        let dir = tempfile::tempdir().unwrap();
        let raw_path = dir.path().join("raw.jsonl");
        let secret_line = "token sk-ant-api03-abcdefghijklmnopqrstuvwxyz012345 end\n";
        std::fs::write(&raw_path, secret_line).unwrap();

        let env = no_env();
        let collected = collect_artifacts(&ArtifactCollectionInputs {
            raw_log_path: raw_path,
            workspace_path: None,
            conclusion: None,
            environment: &env,
        })
        .await;

        assert_eq!(collected.len(), 1);
        let transcript = &collected[0];
        assert_eq!(transcript.artifact_type, "execution_transcript");
        assert!(transcript.is_evidence);
        assert_eq!(transcript.redaction_count, 1);
        let body = String::from_utf8(transcript.bytes.clone()).unwrap();
        assert!(body.contains("[REDACTED:anthropic_key]"));
        assert!(!body.contains("abcdefghijklmnopqrstuvwxyz012345"));
        // 脱敏先于 sha256:哈希覆盖的是脱敏后的字节。
        assert_eq!(transcript.sha256, hex(&Sha256::digest(&transcript.bytes)));
    }

    #[tokio::test]
    async fn conclusion_is_not_evidence() {
        let dir = tempfile::tempdir().unwrap();
        let env = no_env();
        let collected = collect_artifacts(&ArtifactCollectionInputs {
            raw_log_path: dir.path().join("missing.jsonl"),
            workspace_path: None,
            conclusion: Some("一切正常,已完成。"),
            environment: &env,
        })
        .await;

        assert_eq!(collected.len(), 1);
        assert_eq!(collected[0].artifact_type, "conclusion");
        assert!(!collected[0].is_evidence);
    }

    #[tokio::test]
    async fn missing_raw_yields_no_transcript() {
        let dir = tempfile::tempdir().unwrap();
        let env = no_env();
        let collected = collect_artifacts(&ArtifactCollectionInputs {
            raw_log_path: dir.path().join("missing.jsonl"),
            workspace_path: None,
            conclusion: None,
            environment: &env,
        })
        .await;
        assert!(collected.is_empty());
    }

    #[test]
    fn transcript_truncation_keeps_tail_on_line_boundary() {
        let mut body = Vec::new();
        for index in 0..1000 {
            body.extend_from_slice(format!("line-{index:04}\n").as_bytes());
        }
        let (kept, truncated) = truncate_keep_tail(body, 100);
        assert!(truncated);
        assert!(kept.len() <= 100);
        let text = String::from_utf8(kept).unwrap();
        assert!(
            text.starts_with("line-"),
            "must cut on a line boundary: {text}"
        );
        assert!(text.trim_end().ends_with("line-0999"));
    }

    #[test]
    fn small_content_is_not_truncated() {
        let (kept, truncated) = truncate_keep_tail(b"short\n".to_vec(), 100);
        assert!(!truncated);
        assert_eq!(kept, b"short\n");
    }

    async fn git(dir: &Path, args: &[&str]) {
        let status = tokio::process::Command::new("git")
            .arg("-C")
            .arg(dir)
            .args(args)
            .env("GIT_CONFIG_GLOBAL", "/dev/null")
            .env("GIT_CONFIG_SYSTEM", "/dev/null")
            .status()
            .await
            .unwrap();
        assert!(status.success(), "git {args:?} failed");
    }

    async fn init_repo(dir: &Path) {
        git(dir, &["init", "-q"]).await;
        git(dir, &["config", "user.email", "test@example.com"]).await;
        git(dir, &["config", "user.name", "test"]).await;
    }

    #[tokio::test]
    async fn attachments_collect_untracked_whitelisted_files_only() {
        let dir = tempfile::tempdir().unwrap();
        let root = dir.path();
        init_repo(root).await;
        // Pre-existing tracked file — must never be captured (spec §1.1).
        std::fs::write(root.join("README.md"), "# tracked").unwrap();
        git(root, &["add", "."]).await;
        git(root, &["commit", "-qm", "base"]).await;

        std::fs::write(root.join("report.html"), "<h1>report</h1>").unwrap();
        std::fs::write(root.join("notes.md"), "notes").unwrap();
        std::fs::write(root.join("data.bin"), [0u8; 8]).unwrap(); // not whitelisted
        std::fs::create_dir_all(root.join("node_modules/pkg")).unwrap();
        std::fs::write(root.join("node_modules/pkg/README.md"), "noise").unwrap();
        // Hidden runtime/provider metadata must never be captured (E2E 07-19).
        std::fs::create_dir_all(root.join(".superteam/mcp")).unwrap();
        std::fs::write(root.join(".superteam/mcp/claude.mcp.json"), "{}").unwrap();
        std::fs::write(root.join(".hidden-notes.md"), "hidden").unwrap();
        // Modified tracked file — covered by the diff artifact, not captured.
        std::fs::write(root.join("README.md"), "# tracked (edited)").unwrap();

        let collection = collect_attachments(root, None).await;
        let mut paths: Vec<&str> = collection
            .attachments
            .iter()
            .map(|attachment| attachment.relative_path.as_str())
            .collect();
        paths.sort();
        assert_eq!(paths, vec!["notes.md", "report.html"]);
        assert!(collection.skipped.is_empty());
        let report = collection
            .attachments
            .iter()
            .find(|attachment| attachment.relative_path == "report.html")
            .unwrap();
        assert_eq!(report.artifact.artifact_type, "execution_output");
        assert_eq!(report.artifact.content_type, "text/html");
        assert!(!report.artifact.is_evidence);
    }

    #[tokio::test]
    async fn attachments_respect_gitignore() {
        let dir = tempfile::tempdir().unwrap();
        let root = dir.path();
        init_repo(root).await;
        std::fs::write(root.join(".gitignore"), "ignored/\n").unwrap();
        git(root, &["add", "."]).await;
        git(root, &["commit", "-qm", "base"]).await;
        std::fs::create_dir_all(root.join("ignored")).unwrap();
        std::fs::write(root.join("ignored/report.html"), "<p>hidden</p>").unwrap();
        std::fs::write(root.join("visible.md"), "seen").unwrap();

        let collection = collect_attachments(root, None).await;
        let paths: Vec<&str> = collection
            .attachments
            .iter()
            .map(|attachment| attachment.relative_path.as_str())
            .collect();
        assert_eq!(paths, vec!["visible.md"]);
    }

    #[tokio::test]
    async fn oversized_attachment_is_skipped_with_note_not_truncated() {
        let dir = tempfile::tempdir().unwrap();
        let root = dir.path();
        init_repo(root).await;
        std::fs::write(
            root.join("big.txt"),
            vec![b'x'; MAX_ATTACHMENT_FILE_BYTES as usize + 1],
        )
        .unwrap();
        std::fs::write(root.join("small.txt"), "ok").unwrap();

        let collection = collect_attachments(root, None).await;
        assert_eq!(collection.attachments.len(), 1);
        assert_eq!(collection.attachments[0].relative_path, "small.txt");
        assert_eq!(collection.skipped.len(), 1);
        assert_eq!(collection.skipped[0].relative_path, "big.txt");
        assert!(collection.skipped[0].reason.contains("5MiB"));
    }

    #[tokio::test]
    async fn non_git_workspace_uses_baseline_snapshot() {
        let dir = tempfile::tempdir().unwrap();
        let root = dir.path();
        std::fs::write(root.join("existing.md"), "before").unwrap();
        let baseline = snapshot_workspace_files(root);
        std::fs::write(root.join("new-report.md"), "after").unwrap();

        let collection = collect_attachments(root, Some(&baseline)).await;
        let paths: Vec<&str> = collection
            .attachments
            .iter()
            .map(|attachment| attachment.relative_path.as_str())
            .collect();
        assert_eq!(paths, vec!["new-report.md"]);
    }

    #[tokio::test]
    async fn non_git_workspace_without_baseline_collects_nothing() {
        let dir = tempfile::tempdir().unwrap();
        std::fs::write(dir.path().join("orphan.md"), "no baseline").unwrap();
        let collection = collect_attachments(dir.path(), None).await;
        assert!(collection.attachments.is_empty());
        assert!(collection.skipped.is_empty());
    }
}
