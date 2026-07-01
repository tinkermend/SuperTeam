use std::path::Path;

use anyhow::Result;
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct CommandAttestation {
    pub attestation_type: String,
    pub status: String,
    pub command_argv: Vec<String>,
    pub exit_code: Option<i32>,
    pub duration_ms: i64,
    pub stdout_sha256: String,
    pub stderr_sha256: String,
}

impl CommandAttestation {
    pub fn from_logs(
        argv: Vec<String>,
        exit_code: Option<i32>,
        duration_ms: i64,
        stdout_path: &Path,
        stderr_path: &Path,
    ) -> Result<Self> {
        let status = match exit_code {
            Some(0) => "succeeded",
            Some(_) => "failed",
            None => "cancelled",
        };
        Ok(Self {
            attestation_type: "provider_session".to_string(),
            status: status.to_string(),
            command_argv: argv,
            exit_code,
            duration_ms,
            stdout_sha256: sha256_file(stdout_path)?,
            stderr_sha256: sha256_file(stderr_path)?,
        })
    }
}

pub fn sha256_file(path: &Path) -> Result<String> {
    let bytes = std::fs::read(path)?;
    let digest = Sha256::digest(bytes);
    let mut out = String::with_capacity(digest.len() * 2);
    for byte in digest {
        out.push(nibble_to_hex(byte >> 4));
        out.push(nibble_to_hex(byte & 0x0f));
    }
    Ok(out)
}

fn nibble_to_hex(value: u8) -> char {
    match value {
        0..=9 => (b'0' + value) as char,
        10..=15 => (b'a' + (value - 10)) as char,
        _ => unreachable!("sha256 nibble is always <= 15"),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::TempDir;

    #[test]
    fn hashes_log_files_for_attestation() {
        let temp = TempDir::new().unwrap();
        let stdout_path = temp.path().join("stdout.log");
        let stderr_path = temp.path().join("stderr.log");
        std::fs::write(&stdout_path, b"ok\n").unwrap();
        std::fs::write(&stderr_path, b"").unwrap();

        let record = CommandAttestation::from_logs(
            vec!["cargo".to_string(), "test".to_string()],
            Some(0),
            12,
            &stdout_path,
            &stderr_path,
        )
        .unwrap();

        assert_eq!(record.exit_code, Some(0));
        assert_eq!(record.status, "succeeded");
        assert_eq!(record.stdout_sha256.len(), 64);
        assert_eq!(record.stderr_sha256.len(), 64);
    }
}
