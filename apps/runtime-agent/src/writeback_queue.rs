//! 任务结果写回的本地持久化重试队列(卡死任务收敛 spec §3.2 / 遗留缺陷#1)。
//!
//! runtime 执行完项目任务尝试后把 complete/fail/wait-human 结果 POST 回控制平面。
//! 该 POST 若因网络失联/CP 5xx 瞬时失败,此前只 `eprintln!` 吞掉、结果永久丢失,
//! 任务在 CP 侧永远卡 running(直到 P1 CP 侧 attempt 看门狗按租约过期恢复,但那已把
//! 结果丢了、任务被判失败)。本队列把失败的终态写回落盘,后台 worker 按退避重放:
//!
//! - 2xx → 重放成功,任务正常完成/失败,结果不丢。删除队列项。
//! - 4xx(含 409 attempt 已终态/租约失配,通常是 P1 看门狗已恢复该 attempt)→ 确定性
//!   不可成功、且下游已被 CP 接管,丢弃队列项(不无限重试)。
//! - 5xx / 网络 / 超时 → 保留,下一轮退避重试;超过 max_age 兜底放弃(loud log,CP 看门狗覆盖)。
//!
//! 与 P1 互补:短瞬时失败由本队列重放保住结果;长失联由 P1 看门狗恢复,本队列的迟到
//! 写回拿到 409 后安全丢弃。请求体已含确定性 idempotency_key/lease_token,重放即幂等。

use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use tokio_util::sync::CancellationToken;

use crate::config::RuntimeConfig;
use crate::controlplane::client::{ControlPlaneClient, RuntimeApiError};
use crate::controlplane::models::ProjectTaskAttemptWritebackKind;

/// 写回重试队列目录:放在 runs 日志目录同级的 `writeback-queue/`(即 `.superteam/` 下)。
/// 入队方(executor)与重放 worker(daemon)共用同一目录。
pub fn queue_dir(config: &RuntimeConfig) -> PathBuf {
    let log_dir = &config.runs.log_dir;
    match log_dir.parent() {
        Some(parent) => parent.join("writeback-queue"),
        None => log_dir.join("writeback-queue"),
    }
}

/// 一条待重放的终态写回。body 是原始请求体(已含 idempotency_key/lease_token/
/// runtime_node_id),按 kind 分派到对应 CP 端点原样重发。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PendingWriteback {
    pub kind: ProjectTaskAttemptWritebackKind,
    pub attempt_id: String,
    pub body: serde_json::Value,
    pub enqueued_at_unix: i64,
}

/// 落盘的写回重试队列。目录下每个 attempt 至多一条(一个 attempt 只有一个终态写回),
/// 文件名以 attempt_id 命名,重复入队幂等覆盖。
#[derive(Clone)]
pub struct WritebackQueue {
    dir: PathBuf,
}

impl WritebackQueue {
    pub fn new(dir: PathBuf) -> Self {
        Self { dir }
    }

    fn item_path(&self, attempt_id: &str) -> PathBuf {
        let sanitized: String = attempt_id
            .chars()
            .map(|c| if c.is_ascii_alphanumeric() || c == '-' { c } else { '_' })
            .collect();
        self.dir.join(format!("{sanitized}.json"))
    }

    /// 把一条失败的终态写回落盘(先写 .tmp 再 rename,避免半截文件被 worker 读到)。
    pub async fn enqueue(
        &self,
        kind: ProjectTaskAttemptWritebackKind,
        attempt_id: &str,
        body: serde_json::Value,
    ) -> anyhow::Result<()> {
        let item = PendingWriteback {
            kind,
            attempt_id: attempt_id.to_string(),
            body,
            enqueued_at_unix: now_unix(),
        };
        tokio::fs::create_dir_all(&self.dir).await?;
        let path = self.item_path(attempt_id);
        let tmp = path.with_extension("json.tmp");
        let bytes = serde_json::to_vec(&item)?;
        write_private(&tmp, &bytes).await?;
        tokio::fs::rename(&tmp, &path).await?;
        Ok(())
    }

    /// 扫描队列目录,重放每一项一次。成功/确定性失败/超龄 → 删项;瞬时失败 → 留待下轮。
    /// 逐项失败只记日志不中断。返回本轮实际重放成功(flush)的条数。
    pub async fn drain_once(
        &self,
        client: &ControlPlaneClient,
        max_age: Duration,
    ) -> usize {
        let mut read_dir = match tokio::fs::read_dir(&self.dir).await {
            Ok(rd) => rd,
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => return 0,
            Err(err) => {
                eprintln!("writeback retry: read queue dir failed: {err}");
                return 0;
            }
        };
        let mut flushed = 0usize;
        loop {
            let entry = match read_dir.next_entry().await {
                Ok(Some(entry)) => entry,
                Ok(None) => break,
                Err(err) => {
                    eprintln!("writeback retry: iterate queue dir failed: {err}");
                    break;
                }
            };
            let path = entry.path();
            if path.extension().and_then(|e| e.to_str()) != Some("json") {
                continue; // 跳过 .tmp 等
            }
            match self.replay_one(&path, client, max_age).await {
                ReplayOutcome::Flushed => flushed += 1,
                ReplayOutcome::Discarded | ReplayOutcome::Retained => {}
            }
        }
        if flushed > 0 {
            eprintln!("writeback retry: flushed {flushed} pending task writeback(s)");
        }
        flushed
    }

    async fn replay_one(
        &self,
        path: &Path,
        client: &ControlPlaneClient,
        max_age: Duration,
    ) -> ReplayOutcome {
        let bytes = match tokio::fs::read(path).await {
            Ok(b) => b,
            Err(err) => {
                eprintln!("writeback retry: read {} failed: {err}", path.display());
                return ReplayOutcome::Retained;
            }
        };
        let item: PendingWriteback = match serde_json::from_slice(&bytes) {
            Ok(item) => item,
            Err(err) => {
                // 损坏项无法重放,删除避免卡死队列。
                eprintln!(
                    "writeback retry: corrupt queue item {} dropped: {err}",
                    path.display()
                );
                remove_quietly(path).await;
                return ReplayOutcome::Discarded;
            }
        };
        match client
            .resend_project_task_attempt_writeback(item.kind, &item.attempt_id, &item.body)
            .await
        {
            Ok(()) => {
                remove_quietly(path).await;
                ReplayOutcome::Flushed
            }
            Err(err) => {
                if is_deterministic_failure(&err) {
                    // 4xx:attempt 已终态/租约失配(多为 P1 看门狗已恢复),重放永不会成功,
                    // 且任务已被 CP 接管。丢弃,不无限重试。
                    eprintln!(
                        "writeback retry: attempt {} writeback superseded ({err}); discarding",
                        item.attempt_id
                    );
                    remove_quietly(path).await;
                    ReplayOutcome::Discarded
                } else if age_exceeded(&item, max_age) {
                    // 瞬时失败但已超龄兜底放弃:结果丢失由 CP 侧 attempt 看门狗覆盖(任务不会永卡)。
                    eprintln!(
                        "writeback retry: giving up on attempt {} after {}s (last error: {err}); \
                         CP-side stuck-task watchdog will recover the task",
                        item.attempt_id,
                        max_age.as_secs()
                    );
                    remove_quietly(path).await;
                    ReplayOutcome::Discarded
                } else {
                    // 瞬时失败:保留,下一轮重试。
                    ReplayOutcome::Retained
                }
            }
        }
    }
}

enum ReplayOutcome {
    Flushed,
    Discarded,
    Retained,
}

/// 4xx(客户端错误)= 确定性、重放不会成功(400 请求非法 / 403 绑定失配 / 404 不存在 /
/// 409 attempt 已终态或租约失配)。5xx/网络/超时/鉴权失效(RuntimeAuthExpired,会话续期后
/// 可恢复)不在此列 → 视为瞬时可重试。
fn is_deterministic_failure(err: &anyhow::Error) -> bool {
    err.downcast_ref::<RuntimeApiError>()
        .is_some_and(|api| api.status.is_client_error())
}

fn age_exceeded(item: &PendingWriteback, max_age: Duration) -> bool {
    let now = now_unix();
    now.saturating_sub(item.enqueued_at_unix) as u64 > max_age.as_secs()
}

fn now_unix() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs() as i64)
        .unwrap_or(0)
}

async fn write_private(path: &Path, bytes: &[u8]) -> std::io::Result<()> {
    #[cfg(unix)]
    {
        use tokio::io::AsyncWriteExt;
        let mut file = tokio::fs::OpenOptions::new()
            .create(true)
            .write(true)
            .truncate(true)
            .mode(0o600)
            .open(path)
            .await?;
        file.write_all(bytes).await?;
        file.flush().await?;
        Ok(())
    }
    #[cfg(not(unix))]
    {
        tokio::fs::write(path, bytes).await
    }
}

async fn remove_quietly(path: &Path) {
    if let Err(err) = tokio::fs::remove_file(path).await {
        if err.kind() != std::io::ErrorKind::NotFound {
            eprintln!("writeback retry: remove {} failed: {err}", path.display());
        }
    }
}

/// 启动写回重试后台 worker:启动即扫一轮(补偿上次进程遗留的未发写回),此后每
/// `interval` 巡检一次,直到 `cancel` 触发。
pub fn spawn_writeback_retry_worker(
    queue_dir: PathBuf,
    client: ControlPlaneClient,
    interval: Duration,
    max_age: Duration,
    cancel: CancellationToken,
) -> tokio::task::JoinHandle<()> {
    tokio::spawn(async move {
        let queue = WritebackQueue::new(queue_dir);
        // 启动即补账:重放上次进程崩溃/退出时遗留在盘上的未发写回。
        queue.drain_once(&client, max_age).await;
        loop {
            tokio::select! {
                _ = cancel.cancelled() => break,
                _ = tokio::time::sleep(interval) => {
                    queue.drain_once(&client, max_age).await;
                }
            }
        }
    })
}
