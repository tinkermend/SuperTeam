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

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct CollectedArtifact {
    pub artifact_type: &'static str,
    pub name: &'static str,
    pub content_type: &'static str,
    pub is_evidence: bool,
    pub truncated: bool,
    pub redaction_count: usize,
    pub sha256: String,
    pub bytes: Vec<u8>,
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
                "raw.jsonl",
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
                    "changes.diff",
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
                "conclusion.md",
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
    name: &'static str,
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
}
