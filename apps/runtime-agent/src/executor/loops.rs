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
