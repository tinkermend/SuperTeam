//! Raw provider transcript capture.
//!
//! Every stdout/stderr line the provider writes is appended verbatim to a local
//! buffer and uploaded to object storage in segments. The bytes are NOT
//! redacted — see §3.5 of
//! `docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md`.
//! Only the excerpts that enter the ledger pass through `crate::redaction`.
//!
//! Segments are uploaded as they fill rather than once at the end: a killed
//! process or a powered-off node is exactly the case where the evidence matters
//! most, and a finalize-only upload loses all of it.

use std::path::{Path, PathBuf};
use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use serde::Serialize;
use sha2::{Digest, Sha256};
use tokio::fs::{self, OpenOptions};
use tokio::io::AsyncWriteExt;
use tokio::sync::{Mutex, mpsc};
use tokio::task::JoinHandle;

/// Rotate a segment once it reaches this many bytes.
const DEFAULT_SEGMENT_BYTES: usize = 8 * 1024 * 1024;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RawStream {
    Stdout,
    Stderr,
}

/// Sink for verbatim provider output lines.
///
/// The provider layer depends only on this narrow trait: it knows nothing about
/// run ids, object storage or the run store.
#[async_trait]
pub trait RawLineSink: Send + Sync {
    fn write_line(&self, stream: RawStream, line: &str);

    /// Flushes and uploads whatever is buffered. Returns the pointer to store on
    /// the attempt, or `None` when this sink captures nothing.
    async fn finalize_log(&self) -> Option<RawLogSummary> {
        None
    }
}

/// Used by tests and by runs that have no object storage configured.
pub struct NoopRawSink;

#[async_trait]
impl RawLineSink for NoopRawSink {
    fn write_line(&self, _stream: RawStream, _line: &str) {}
}

/// Where the raw transcript ended up, reported back to the control plane.
#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
pub struct RawLogSummary {
    pub log_store: String,
    pub log_ref: String,
    pub log_bytes: i64,
    pub log_sha256: String,
    pub log_compressed: bool,
}

#[async_trait]
pub trait RawLogUploader: Send + Sync {
    async fn put(&self, key: &str, body: Vec<u8>) -> Result<()>;
}

pub struct S3RawLogUploader {
    client: aws_sdk_s3::Client,
    bucket: String,
}

impl S3RawLogUploader {
    pub fn new(client: aws_sdk_s3::Client, bucket: String) -> Self {
        Self { client, bucket }
    }
}

#[async_trait]
impl RawLogUploader for S3RawLogUploader {
    async fn put(&self, key: &str, body: Vec<u8>) -> Result<()> {
        self.client
            .put_object()
            .bucket(&self.bucket)
            .key(key)
            .body(body.into())
            .send()
            .await
            .with_context(|| format!("failed to upload raw log segment {key}"))?;
        Ok(())
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
struct ManifestPart {
    key: String,
    bytes: i64,
    sha256: String,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize)]
struct Manifest {
    attempt_id: String,
    parts: Vec<ManifestPart>,
    total_bytes: i64,
    total_sha256: String,
    /// False when a segment upload exhausted its retries. The run is not killed
    /// for this — the bytes are still on the node and can be re-sent.
    complete: bool,
}

enum Msg {
    Line { stream: RawStream, line: String },
    Finish,
}

pub struct SegmentedRawLogSink {
    tx: mpsc::UnboundedSender<Msg>,
    task: Mutex<Option<JoinHandle<Result<RawLogSummary>>>>,
}

impl SegmentedRawLogSink {
    pub fn new(
        uploader: Arc<dyn RawLogUploader>,
        local_dir: PathBuf,
        key_prefix: String,
        attempt_id: String,
    ) -> Self {
        Self::with_segment_bytes(uploader, local_dir, key_prefix, attempt_id, DEFAULT_SEGMENT_BYTES)
    }

    pub fn with_segment_bytes(
        uploader: Arc<dyn RawLogUploader>,
        local_dir: PathBuf,
        key_prefix: String,
        attempt_id: String,
        segment_bytes: usize,
    ) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        let writer = Writer {
            uploader,
            local_dir,
            key_prefix,
            attempt_id,
            segment_bytes,
        };
        let task = tokio::spawn(writer.run(rx));
        Self {
            tx,
            task: Mutex::new(Some(task)),
        }
    }

    /// Flushes the last segment, uploads the manifest and returns the pointer.
    pub async fn finalize(&self) -> Result<RawLogSummary> {
        let _ = self.tx.send(Msg::Finish);
        let task = self.task.lock().await.take();
        match task {
            Some(task) => task.await.context("raw log writer panicked")?,
            None => anyhow::bail!("raw log sink already finalized"),
        }
    }
}

#[async_trait]
impl RawLineSink for SegmentedRawLogSink {
    async fn finalize_log(&self) -> Option<RawLogSummary> {
        match self.finalize().await {
            Ok(summary) => Some(summary),
            Err(error) => {
                // Losing the pointer must not lose the run; the bytes remain on
                // the node and the attempt simply has no raw_log reference.
                eprintln!("failed to finalize raw log: {error}");
                None
            }
        }
    }

    fn write_line(&self, stream: RawStream, line: &str) {
        // A closed channel means the writer already finished; dropping the line
        // is correct, and must never block or fail the provider stream.
        let _ = self.tx.send(Msg::Line {
            stream,
            line: line.to_string(),
        });
    }
}

struct Writer {
    uploader: Arc<dyn RawLogUploader>,
    local_dir: PathBuf,
    key_prefix: String,
    attempt_id: String,
    segment_bytes: usize,
}

impl Writer {
    async fn run(self, mut rx: mpsc::UnboundedReceiver<Msg>) -> Result<RawLogSummary> {
        fs::create_dir_all(&self.local_dir)
            .await
            .with_context(|| format!("failed to create raw log dir {:?}", self.local_dir))?;
        let local_path = self.local_dir.join("raw.jsonl");

        let mut segment: Vec<u8> = Vec::with_capacity(self.segment_bytes.min(1 << 20));
        let mut parts: Vec<ManifestPart> = Vec::new();
        let mut total = Sha256::new();
        let mut total_bytes: i64 = 0;
        let mut complete = true;

        while let Some(msg) = rx.recv().await {
            match msg {
                Msg::Line { stream, line } => {
                    let encoded = encode_line(stream, &line);
                    // Local write failure means evidence cannot be recorded at
                    // all; the run must not continue pretending otherwise.
                    append_local(&local_path, &encoded).await?;
                    total.update(&encoded);
                    total_bytes += encoded.len() as i64;
                    segment.extend_from_slice(&encoded);
                    if segment.len() >= self.segment_bytes {
                        let part = self.upload_segment(parts.len() + 1, &segment).await;
                        match part {
                            Ok(part) => parts.push(part),
                            Err(_) => complete = false,
                        }
                        segment.clear();
                    }
                }
                Msg::Finish => break,
            }
        }

        if !segment.is_empty() {
            match self.upload_segment(parts.len() + 1, &segment).await {
                Ok(part) => parts.push(part),
                Err(_) => complete = false,
            }
        }

        let total_sha256 = hex(&total.finalize());
        let manifest = Manifest {
            attempt_id: self.attempt_id.clone(),
            parts,
            total_bytes,
            total_sha256: total_sha256.clone(),
            complete,
        };
        let manifest_key = format!("{}manifest.json", self.key_prefix);
        let manifest_body = serde_json::to_vec(&manifest)?;
        self.uploader.put(&manifest_key, manifest_body).await?;

        Ok(RawLogSummary {
            log_store: "object_store".to_string(),
            log_ref: manifest_key,
            log_bytes: total_bytes,
            log_sha256: total_sha256,
            log_compressed: false,
        })
    }

    /// Retries with a bounded backoff. A failed segment degrades the manifest to
    /// `complete: false` instead of killing the run: the bytes are still on the
    /// node, so the evidence is delayed rather than lost.
    async fn upload_segment(&self, index: usize, body: &[u8]) -> Result<ManifestPart> {
        let key = format!("{}raw.part-{index:04}.jsonl", self.key_prefix);
        let sha256 = hex(&Sha256::digest(body));

        let mut delay = std::time::Duration::from_millis(200);
        let mut last_error = None;
        for _ in 0..3 {
            match self.uploader.put(&key, body.to_vec()).await {
                Ok(()) => {
                    return Ok(ManifestPart {
                        key,
                        bytes: body.len() as i64,
                        sha256,
                    });
                }
                Err(error) => {
                    last_error = Some(error);
                    tokio::time::sleep(delay).await;
                    delay *= 2;
                }
            }
        }
        let error = last_error.unwrap_or_else(|| anyhow::anyhow!("unknown upload failure"));
        eprintln!("raw log segment {key} upload failed after retries: {error}");
        Err(error)
    }
}

fn encode_line(stream: RawStream, line: &str) -> Vec<u8> {
    let mut encoded = match stream {
        // stdout lines are the provider's own JSON; keep them byte-for-byte so
        // the transcript stays replayable by the provider's own tooling.
        RawStream::Stdout => line.as_bytes().to_vec(),
        RawStream::Stderr => serde_json::to_vec(&serde_json::json!({
            "__stream": "stderr",
            "line": line,
        }))
        .unwrap_or_else(|_| Vec::new()),
    };
    encoded.push(b'\n');
    encoded
}

async fn append_local(path: &Path, bytes: &[u8]) -> Result<()> {
    let mut file = OpenOptions::new()
        .create(true)
        .append(true)
        .open(path)
        .await
        .with_context(|| format!("failed to open raw log {path:?}"))?;
    file.write_all(bytes).await?;
    file.flush().await?;
    Ok(())
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
    use std::sync::Mutex as StdMutex;

    #[derive(Default)]
    struct RecordingUploader {
        objects: StdMutex<Vec<(String, Vec<u8>)>>,
        fail_parts: bool,
    }

    #[async_trait]
    impl RawLogUploader for RecordingUploader {
        async fn put(&self, key: &str, body: Vec<u8>) -> Result<()> {
            if self.fail_parts && key.contains("raw.part-") {
                anyhow::bail!("simulated upload failure");
            }
            self.objects.lock().unwrap().push((key.to_string(), body));
            Ok(())
        }
    }

    fn sink(uploader: Arc<RecordingUploader>, dir: &Path, segment_bytes: usize) -> SegmentedRawLogSink {
        SegmentedRawLogSink::with_segment_bytes(
            uploader,
            dir.to_path_buf(),
            "runs/t1/a1/".to_string(),
            "a1".to_string(),
            segment_bytes,
        )
    }

    #[tokio::test]
    async fn writes_lines_verbatim_and_uploads_manifest() {
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader::default());
        let s = sink(uploader.clone(), dir.path(), 1 << 20);

        s.write_line(RawStream::Stdout, r#"{"type":"user"}"#);
        let summary = s.finalize().await.unwrap();

        let local = std::fs::read_to_string(dir.path().join("raw.jsonl")).unwrap();
        assert_eq!(local, "{\"type\":\"user\"}\n");
        assert_eq!(summary.log_store, "object_store");
        assert_eq!(summary.log_ref, "runs/t1/a1/manifest.json");
        assert_eq!(summary.log_bytes, local.len() as i64);

        let objects = uploader.objects.lock().unwrap();
        let keys: Vec<_> = objects.iter().map(|(k, _)| k.as_str()).collect();
        assert!(keys.contains(&"runs/t1/a1/raw.part-0001.jsonl"));
        assert!(keys.contains(&"runs/t1/a1/manifest.json"));
    }

    #[tokio::test]
    async fn secrets_are_not_redacted_in_raw() {
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader::default());
        let s = sink(uploader.clone(), dir.path(), 1 << 20);

        let token = "sk-ant-api03-abcdefghijklmnopqrstuvwxyz012345";
        s.write_line(RawStream::Stdout, token);
        s.finalize().await.unwrap();

        let local = std::fs::read_to_string(dir.path().join("raw.jsonl")).unwrap();
        assert!(local.contains(token), "raw must stay byte-for-byte");
    }

    #[tokio::test]
    async fn stderr_lines_are_tagged() {
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader::default());
        let s = sink(uploader.clone(), dir.path(), 1 << 20);

        s.write_line(RawStream::Stderr, "boom");
        s.finalize().await.unwrap();

        let local = std::fs::read_to_string(dir.path().join("raw.jsonl")).unwrap();
        let value: serde_json::Value = serde_json::from_str(local.trim()).unwrap();
        assert_eq!(value["__stream"], "stderr");
        assert_eq!(value["line"], "boom");
    }

    #[tokio::test]
    async fn rotates_segments_by_size() {
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader::default());
        let s = sink(uploader.clone(), dir.path(), 8);

        s.write_line(RawStream::Stdout, "aaaaaaaaaa");
        s.write_line(RawStream::Stdout, "bbbbbbbbbb");
        s.finalize().await.unwrap();

        let objects = uploader.objects.lock().unwrap();
        let parts = objects
            .iter()
            .filter(|(k, _)| k.contains("raw.part-"))
            .count();
        assert_eq!(parts, 2);
    }

    #[tokio::test]
    async fn failed_segment_upload_marks_manifest_incomplete_without_failing_the_run() {
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader {
            objects: StdMutex::new(Vec::new()),
            fail_parts: true,
        });
        let s = sink(uploader.clone(), dir.path(), 1 << 20);

        s.write_line(RawStream::Stdout, "line");
        let summary = s.finalize().await.expect("run must survive upload failure");
        assert_eq!(summary.log_ref, "runs/t1/a1/manifest.json");

        let objects = uploader.objects.lock().unwrap();
        let (_, manifest) = objects
            .iter()
            .find(|(k, _)| k.ends_with("manifest.json"))
            .expect("manifest uploaded");
        let value: serde_json::Value = serde_json::from_slice(manifest).unwrap();
        assert_eq!(value["complete"], false);
        assert_eq!(value["parts"].as_array().unwrap().len(), 0);

        // The bytes survive locally, so the evidence is delayed, not lost.
        let local = std::fs::read_to_string(dir.path().join("raw.jsonl")).unwrap();
        assert_eq!(local, "line\n");
    }

    #[tokio::test]
    async fn total_sha256_covers_concatenated_bytes() {
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader::default());
        let s = sink(uploader.clone(), dir.path(), 1 << 20);

        s.write_line(RawStream::Stdout, "one");
        s.write_line(RawStream::Stdout, "two");
        let summary = s.finalize().await.unwrap();

        let local = std::fs::read(dir.path().join("raw.jsonl")).unwrap();
        assert_eq!(summary.log_sha256, hex(&Sha256::digest(&local)));
    }
}
