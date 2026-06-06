use anyhow::{Context, Result};
use aws_sdk_s3::Client as S3Client;
use std::path::Path;
use tokio::fs;

#[derive(Debug, Clone)]
pub struct ArtifactRef {
    pub key: String,
    pub uri: String,
    pub size: u64,
}

pub struct ArtifactCollector {
    s3_client: S3Client,
    bucket: String,
}

impl ArtifactCollector {
    pub fn new(s3_client: S3Client, bucket: String) -> Self {
        Self { s3_client, bucket }
    }

    pub async fn upload_directory(
        &self,
        tenant_id: &str,
        run_id: &str,
        dir_path: &Path,
        artifact_type: &str,
    ) -> Result<Vec<ArtifactRef>> {
        if !dir_path.exists() {
            return Ok(Vec::new());
        }

        let mut refs = Vec::new();
        let mut entries = fs::read_dir(dir_path).await?;

        while let Some(entry) = entries.next_entry().await? {
            let path = entry.path();
            if path.is_file() {
                let file_name = path
                    .file_name()
                    .and_then(|n| n.to_str())
                    .context("Invalid filename")?;

                let key = format!("runtime/{}/{}/{}/{}", tenant_id, run_id, artifact_type, file_name);
                let artifact_ref = self.upload_file(&key, &path).await?;
                refs.push(artifact_ref);
            }
        }

        Ok(refs)
    }

    async fn upload_file(&self, key: &str, file_path: &Path) -> Result<ArtifactRef> {
        let body = aws_sdk_s3::primitives::ByteStream::from_path(file_path)
            .await
            .context("Failed to read file")?;

        let metadata = fs::metadata(file_path).await?;
        let size = metadata.len();

        self.s3_client
            .put_object()
            .bucket(&self.bucket)
            .key(key)
            .body(body)
            .send()
            .await
            .context("Failed to upload to S3")?;

        let uri = format!("s3://{}/{}", self.bucket, key);

        Ok(ArtifactRef {
            key: key.to_string(),
            uri,
            size,
        })
    }
}
