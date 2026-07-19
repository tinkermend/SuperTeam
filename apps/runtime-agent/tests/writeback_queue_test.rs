//! 写回持久重试队列(遗留缺陷#1)的行为测试:
//! - CP 返回 2xx → 重放成功,队列项删除(结果不丢)。
//! - CP 返回 409(attempt 已终态/租约失配,P1 看门狗已恢复)→ 确定性丢弃,不无限重试。
//! - CP 返回 5xx → 瞬时失败,队列项保留待下轮重试。

use std::time::Duration;

use superteam_runtime_agent::controlplane::client::ControlPlaneClient;
use superteam_runtime_agent::controlplane::models::ProjectTaskAttemptWritebackKind;
use superteam_runtime_agent::writeback_queue::{WritebackQueue, spawn_writeback_retry_worker};
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tokio_util::sync::CancellationToken;

async fn spawn_mock(status_line: &'static str) -> String {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    tokio::spawn(async move {
        if let Ok((mut socket, _)) = listener.accept().await {
            consume_request(&mut socket).await;
            let response = format!(
                "HTTP/1.1 {status_line}\r\nContent-Length: 2\r\nContent-Type: application/json\r\n\r\n{{}}"
            );
            let _ = socket.write_all(response.as_bytes()).await;
            let _ = socket.flush().await;
        }
    });
    format!("http://{addr}")
}

async fn consume_request(socket: &mut TcpStream) {
    // 读到 header 结束 + 按 Content-Length 读完 body,避免客户端写阻塞。
    let mut buffer = Vec::new();
    let header_end = loop {
        let mut chunk = [0u8; 1024];
        let n = socket.read(&mut chunk).await.unwrap_or(0);
        if n == 0 {
            return;
        }
        buffer.extend_from_slice(&chunk[..n]);
        if let Some(i) = buffer.windows(4).position(|w| w == b"\r\n\r\n") {
            break i + 4;
        }
    };
    let headers = String::from_utf8_lossy(&buffer[..header_end]).to_string();
    let content_length = headers
        .lines()
        .filter_map(|l| l.split_once(':'))
        .find_map(|(k, v)| {
            k.eq_ignore_ascii_case("content-length")
                .then(|| v.trim().parse::<usize>().unwrap_or(0))
        })
        .unwrap_or(0);
    while buffer.len() < header_end + content_length {
        let mut chunk = [0u8; 1024];
        let n = socket.read(&mut chunk).await.unwrap_or(0);
        if n == 0 {
            break;
        }
        buffer.extend_from_slice(&chunk[..n]);
    }
}

fn sample_body() -> serde_json::Value {
    serde_json::json!({
        "project_task_id": "11111111-1111-4111-8111-111111111111",
        "lease_token": "lease-abc",
        "runtime_node_id": "22222222-2222-4222-8222-222222222222",
        "idempotency_key": "project-task-attempt:att-1:complete:cmd-1",
        "digital_employee_id": "33333333-3333-4333-8333-333333333333",
        "conclusion": "done"
    })
}

async fn enqueue_one(dir: &std::path::Path) -> std::path::PathBuf {
    let queue = WritebackQueue::new(dir.to_path_buf());
    queue
        .enqueue(
            ProjectTaskAttemptWritebackKind::Complete,
            "att-1",
            sample_body(),
        )
        .await
        .unwrap();
    let path = dir.join("att-1.json");
    assert!(path.exists(), "queue item should be persisted on enqueue");
    path
}

#[tokio::test]
async fn drain_flushes_and_removes_on_success() {
    let tmp = tempfile::tempdir().unwrap();
    let path = enqueue_one(tmp.path()).await;
    let base = spawn_mock("202 Accepted").await;
    let client = ControlPlaneClient::with_node_id(base, "test-token", "node-1");
    let queue = WritebackQueue::new(tmp.path().to_path_buf());

    let flushed = queue.drain_once(&client, Duration::from_secs(3600)).await;

    assert_eq!(flushed, 1, "202 should count as flushed");
    assert!(!path.exists(), "flushed item must be removed");
}

#[tokio::test]
async fn drain_discards_terminal_conflict() {
    let tmp = tempfile::tempdir().unwrap();
    let path = enqueue_one(tmp.path()).await;
    let base = spawn_mock("409 Conflict").await;
    let client = ControlPlaneClient::with_node_id(base, "test-token", "node-1");
    let queue = WritebackQueue::new(tmp.path().to_path_buf());

    let flushed = queue.drain_once(&client, Duration::from_secs(3600)).await;

    assert_eq!(flushed, 0, "409 is not a flush");
    assert!(
        !path.exists(),
        "409 (attempt superseded/terminal) must be discarded, not retried forever"
    );
}

#[tokio::test]
async fn worker_recovers_on_startup_persisted_item() {
    // daemon 挂载的实际 worker:进程启动即扫一轮,重放上次遗留在盘上的未发写回。
    let tmp = tempfile::tempdir().unwrap();
    let path = enqueue_one(tmp.path()).await; // 模拟"上次进程崩溃前落盘、未发出"的写回
    let base = spawn_mock("202 Accepted").await;
    let client = ControlPlaneClient::with_node_id(base, "test-token", "node-1");
    let cancel = CancellationToken::new();

    let handle = spawn_writeback_retry_worker(
        tmp.path().to_path_buf(),
        client,
        Duration::from_secs(3600), // 长 interval:只考察启动即扫的那一轮
        Duration::from_secs(3600),
        cancel.clone(),
    );

    // 轮询等待启动扫描把盘上项发出并删除。
    for _ in 0..50 {
        if !path.exists() {
            break;
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    cancel.cancel();
    let _ = handle.await;

    assert!(
        !path.exists(),
        "worker must flush a pre-existing on-disk writeback on startup"
    );
}

#[tokio::test]
async fn drain_retains_on_transient_server_error() {
    let tmp = tempfile::tempdir().unwrap();
    let path = enqueue_one(tmp.path()).await;
    let base = spawn_mock("500 Internal Server Error").await;
    let client = ControlPlaneClient::with_node_id(base, "test-token", "node-1");
    let queue = WritebackQueue::new(tmp.path().to_path_buf());

    let flushed = queue.drain_once(&client, Duration::from_secs(3600)).await;

    assert_eq!(flushed, 0, "500 is not a flush");
    assert!(
        path.exists(),
        "5xx is transient; item must be retained for the next retry tick"
    );
}
