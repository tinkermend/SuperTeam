use anyhow::Result;
use std::time::Duration;

use crate::controlplane::ControlPlaneClient;
use crate::events::ProviderEvent;
use crate::runtime_auth::is_runtime_auth_expired;

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
            Err(e) if is_runtime_auth_expired(e.as_ref()) => {
                eprintln!("Push event paused while runtime auth recovers: {}", e);
                continue;
            }
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
            Err(e) if is_runtime_auth_expired(e.as_ref()) => {
                eprintln!("Lease renewal paused while runtime auth recovers: {}", e);
                continue;
            }
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

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::{TcpListener, TcpStream};

    use super::*;
    use crate::runtime_auth::RuntimeAuthState;

    #[tokio::test]
    async fn push_event_waits_for_auth_recovery_before_failing_task() {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let (request_tx, mut request_rx) = tokio::sync::mpsc::channel(4);

        tokio::spawn(async move {
            let (mut first_socket, _) = listener.accept().await.unwrap();
            let first_request = read_http_request(&mut first_socket).await;
            request_tx.send(first_request).await.unwrap();
            write_status_response(
                &mut first_socket,
                "401 Unauthorized",
                serde_json::json!("invalid runtime authentication"),
            )
            .await;

            let (mut second_socket, _) = listener.accept().await.unwrap();
            let second_request = read_http_request(&mut second_socket).await;
            request_tx.send(second_request).await.unwrap();
            second_socket
                .write_all(b"HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n")
                .await
                .unwrap();
        });

        let auth = RuntimeAuthState::new("node-1");
        auth.set_session("session-1", "old-token", "2999-06-02T00:00:00Z")
            .await
            .expect("old session");
        let control_plane =
            ControlPlaneClient::with_runtime_auth(format!("http://{addr}"), auth.clone());
        let push = tokio::spawn({
            let control_plane = control_plane.clone();
            async move {
                push_event_with_retry(
                    &control_plane,
                    42,
                    ProviderEvent::TextDelta {
                        text: "still running".to_string(),
                    },
                )
                .await
            }
        });

        let first_request = tokio::time::timeout(Duration::from_secs(2), request_rx.recv())
            .await
            .expect("first request")
            .expect("first request");
        assert!(first_request.contains("authorization: Bearer old-token"));

        tokio::time::sleep(Duration::from_millis(100)).await;
        assert!(
            !push.is_finished(),
            "typed runtime auth 401 should pause event push instead of failing the task"
        );

        auth.set_session("session-2", "fresh-token", "2999-06-03T00:00:00Z")
            .await
            .expect("fresh session");

        let second_request = tokio::time::timeout(Duration::from_secs(2), request_rx.recv())
            .await
            .expect("second request")
            .expect("second request");
        assert!(second_request.contains("authorization: Bearer fresh-token"));
        push.await.expect("push task").expect("push should recover");
    }

    async fn read_http_request(socket: &mut TcpStream) -> String {
        let mut buf = vec![0_u8; 8192];
        let mut bytes = Vec::new();
        loop {
            let n = socket.read(&mut buf).await.unwrap();
            if n == 0 {
                break;
            }
            bytes.extend_from_slice(&buf[..n]);
            if bytes.windows(4).any(|window| window == b"\r\n\r\n") {
                break;
            }
        }
        String::from_utf8(bytes).unwrap()
    }

    async fn write_status_response(
        socket: &mut TcpStream,
        status_line: &str,
        body: serde_json::Value,
    ) {
        let body = body.to_string();
        socket
            .write_all(
                format!(
                    "HTTP/1.1 {}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                    status_line,
                    body.len(),
                    body
                )
                .as_bytes(),
            )
            .await
            .unwrap();
    }
}
