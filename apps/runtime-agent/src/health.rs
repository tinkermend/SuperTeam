use std::path::PathBuf;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tokio::process::Command;
use tokio::time::timeout;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProviderHealthProbe {
    pub kind: String,
    pub bin_path: PathBuf,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ProviderHealth {
    pub kind: String,
    pub available: bool,
    pub version: Option<String>,
    pub error: Option<String>,
}

pub async fn probe_provider_health(probe: ProviderHealthProbe) -> ProviderHealth {
    let output = timeout(
        Duration::from_secs(3),
        Command::new(&probe.bin_path).arg("--version").output(),
    )
    .await;

    match output {
        Ok(Ok(output)) if output.status.success() => {
            let version = String::from_utf8_lossy(&output.stdout).trim().to_string();
            ProviderHealth {
                kind: probe.kind,
                available: true,
                version: if version.is_empty() {
                    None
                } else {
                    Some(version)
                },
                error: None,
            }
        }
        Ok(Ok(output)) => {
            let stderr = String::from_utf8_lossy(&output.stderr).trim().to_string();
            ProviderHealth {
                kind: probe.kind.clone(),
                available: false,
                version: None,
                error: Some(format!(
                    "{} --version exited with status {}{}",
                    probe.kind,
                    output
                        .status
                        .code()
                        .map(|code| code.to_string())
                        .unwrap_or_else(|| "signal".to_string()),
                    if stderr.is_empty() {
                        String::new()
                    } else {
                        format!(": {stderr}")
                    }
                )),
            }
        }
        Ok(Err(error)) => ProviderHealth {
            kind: probe.kind.clone(),
            available: false,
            version: None,
            error: Some(format!("failed to run {} --version: {error}", probe.kind)),
        },
        Err(_) => ProviderHealth {
            kind: probe.kind.clone(),
            available: false,
            version: None,
            error: Some(format!("{} --version timed out", probe.kind)),
        },
    }
}
