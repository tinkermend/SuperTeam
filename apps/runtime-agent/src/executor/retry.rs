use anyhow::Result;
use std::time::Duration;

use crate::controlplane::ControlPlaneClient;
use crate::events::ProviderEvent;

pub async fn push_event_with_retry(
    control_plane: &ControlPlaneClient,
    task_id: i64,
    event: ProviderEvent,
) -> Result<()> {
    let max_retries = 3;
    let mut attempt = 0;

    loop {
        match control_plane.push_event(task_id, &event).await {
            Ok(_) => return Ok(()),
            Err(e) if is_retryable_error(&e) && attempt < max_retries => {
                attempt += 1;
                let backoff = Duration::from_millis(100 * 2_u64.pow(attempt - 1));
                eprintln!(
                    "Push event failed (attempt {}): {}, retrying in {:?}",
                    attempt, e, backoff
                );
                tokio::time::sleep(backoff).await;
            }
            Err(e) => {
                return Err(anyhow::anyhow!(
                    "Push event failed after {} retries: {}",
                    max_retries,
                    e
                ));
            }
        }
    }
}

pub async fn renew_lease_with_retry(
    control_plane: &ControlPlaneClient,
    task_id: i64,
) -> Result<()> {
    let max_retries = 3;
    let mut attempt = 0;

    loop {
        match control_plane.renew_lease(task_id).await {
            Ok(_) => return Ok(()),
            Err(e) if is_retryable_error(&e) && attempt < max_retries => {
                attempt += 1;
                let backoff = Duration::from_millis(200 * 2_u64.pow(attempt - 1));
                tokio::time::sleep(backoff).await;
            }
            Err(e) => return Err(e),
        }
    }
}

pub fn is_retryable_error(error: &anyhow::Error) -> bool {
    let error_str = error.to_string().to_lowercase();
    error_str.contains("timeout")
        || error_str.contains("connection")
        || error_str.contains("network")
        || error_str.contains("502")
        || error_str.contains("503")
        || error_str.contains("504")
}
