use std::collections::HashMap;
use std::sync::Arc;
use std::time::Duration;
use tokio::sync::Mutex;
use tokio::sync::Semaphore;
use tokio_util::sync::CancellationToken;

use crate::config::RuntimeConfig;
use crate::controlplane::ControlPlaneClient;

use super::retry::renew_lease_with_retry;
use super::task::execute_task;

pub struct QueuedTask {
    pub task: crate::controlplane::models::Task,
    pub priority: i32,
}

pub struct ActiveTask {
    pub cancel_token: CancellationToken,
}

pub async fn polling_loop(
    control_plane: ControlPlaneClient,
    task_queue: Arc<Mutex<Vec<QueuedTask>>>,
    shutdown_token: CancellationToken,
) {
    loop {
        tokio::select! {
            _ = shutdown_token.cancelled() => {
                println!("Polling loop shutting down");
                break;
            }
            result = control_plane.claim_task(30) => {
                match result {
                    Ok(Some(task)) => {
                        let mut queue = task_queue.lock().await;
                        queue.push(QueuedTask {
                            priority: task.priority,
                            task,
                        });
                        queue.sort_by(|a, b| b.priority.cmp(&a.priority));
                    }
                    Ok(None) => {}
                    Err(e) => {
                        eprintln!("Poll failed: {}, retrying in 5s", e);
                        tokio::time::sleep(Duration::from_secs(5)).await;
                    }
                }
            }
        }
    }
}

pub async fn execution_loop(
    control_plane: ControlPlaneClient,
    config: RuntimeConfig,
    task_queue: Arc<Mutex<Vec<QueuedTask>>>,
    active_tasks: Arc<Mutex<HashMap<i64, ActiveTask>>>,
    semaphore: Arc<Semaphore>,
    shutdown_token: CancellationToken,
) {
    loop {
        tokio::select! {
            _ = shutdown_token.cancelled() => {
                println!("Execution loop shutting down");
                break;
            }
            _ = tokio::time::sleep(Duration::from_millis(100)) => {
                let queued_task = {
                    let mut queue = task_queue.lock().await;
                    queue.pop()
                };

                if let Some(queued_task) = queued_task {
                    if let Ok(permit) = semaphore.clone().try_acquire_owned() {
                        let task = queued_task.task;
                        let task_id = task.id;
                        let cancel_token = CancellationToken::new();

                        let cp = control_plane.clone();
                        let cfg = config.clone();
                        let ct = cancel_token.clone();
                        let active = active_tasks.clone();

                        active_tasks.lock().await.insert(task_id, ActiveTask {
                            cancel_token: cancel_token.clone(),
                        });

                        tokio::spawn(async move {
                            let result = execute_task(task, cp, cfg, ct).await;
                            drop(permit);
                            active.lock().await.remove(&task_id);

                            if let Err(e) = result {
                                eprintln!("Task {} failed: {}", task_id, e);
                            }
                        });
                    } else {
                        let mut queue = task_queue.lock().await;
                        queue.push(queued_task);
                    }
                }
            }
        }
    }
}

pub async fn lease_renewal_loop(
    control_plane: ControlPlaneClient,
    active_tasks: Arc<Mutex<HashMap<i64, ActiveTask>>>,
    shutdown_token: CancellationToken,
) {
    let mut interval = tokio::time::interval(Duration::from_secs(30));

    loop {
        tokio::select! {
            _ = shutdown_token.cancelled() => {
                println!("Lease renewal loop shutting down");
                break;
            }
            _ = interval.tick() => {
                let task_ids: Vec<i64> = {
                    let active = active_tasks.lock().await;
                    active.keys().copied().collect()
                };

                for task_id in task_ids {
                    if let Err(e) = renew_lease_with_retry(&control_plane, task_id).await {
                        eprintln!("Failed to renew lease for task {}: {}", task_id, e);
                        if let Some(active_task) = active_tasks.lock().await.get(&task_id) {
                            active_task.cancel_token.cancel();
                        }
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::runtime_auth::RuntimeAuthState;
    use tokio::io::{AsyncReadExt, AsyncWriteExt};
    use tokio::net::{TcpListener, TcpStream};

    #[tokio::test]
    async fn lease_renewal_waits_for_auth_recovery_without_canceling_task() {
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
        let active_tasks = Arc::new(Mutex::new(HashMap::new()));
        let cancel_token = CancellationToken::new();
        active_tasks.lock().await.insert(
            42,
            ActiveTask {
                cancel_token: cancel_token.clone(),
            },
        );
        let shutdown = CancellationToken::new();
        let loop_task = tokio::spawn(lease_renewal_loop(
            control_plane,
            active_tasks.clone(),
            shutdown.clone(),
        ));

        let first_request = tokio::time::timeout(Duration::from_secs(2), request_rx.recv())
            .await
            .expect("first request")
            .expect("first request");
        assert!(first_request.contains("authorization: Bearer old-token"));

        tokio::time::sleep(Duration::from_millis(100)).await;
        assert!(
            !cancel_token.is_cancelled(),
            "typed runtime auth 401 should pause lease renewal instead of canceling active task"
        );

        auth.set_session("session-2", "fresh-token", "2999-06-03T00:00:00Z")
            .await
            .expect("fresh session");

        let second_request = tokio::time::timeout(Duration::from_secs(2), request_rx.recv())
            .await
            .expect("second request")
            .expect("second request");
        assert!(second_request.contains("authorization: Bearer fresh-token"));
        assert!(!cancel_token.is_cancelled());

        shutdown.cancel();
        let _ = loop_task.await;
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
