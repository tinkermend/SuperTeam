pub mod loops;
pub mod retry;
pub mod task;
pub mod workspace;

use std::collections::HashMap;
use std::sync::Arc;
use std::time::{Duration, Instant};
use tokio::sync::{Mutex, Semaphore};
use tokio_util::sync::CancellationToken;

use crate::config::RuntimeConfig;
use crate::controlplane::ControlPlaneClient;

use loops::{ActiveTask, QueuedTask, execution_loop, lease_renewal_loop, polling_loop};

pub struct TaskExecutor {
    config: RuntimeConfig,
    control_plane: ControlPlaneClient,
    task_queue: Arc<Mutex<Vec<QueuedTask>>>,
    active_tasks: Arc<Mutex<HashMap<i64, ActiveTask>>>,
    semaphore: Arc<Semaphore>,
    shutdown_token: CancellationToken,
}

impl TaskExecutor {
    pub fn new(config: RuntimeConfig, control_plane: ControlPlaneClient) -> Self {
        let max_slots = config.runtime.max_concurrent_tasks as usize;

        Self {
            config,
            control_plane,
            task_queue: Arc::new(Mutex::new(Vec::new())),
            active_tasks: Arc::new(Mutex::new(HashMap::new())),
            semaphore: Arc::new(Semaphore::new(max_slots)),
            shutdown_token: CancellationToken::new(),
        }
    }

    pub async fn run(self) -> anyhow::Result<()> {
        let shutdown_token = self.shutdown_token.clone();

        let shutdown_signal = async {
            let mut sigterm = tokio::signal::unix::signal(
                tokio::signal::unix::SignalKind::terminate()
            ).expect("Failed to setup SIGTERM handler");
            let mut sigint = tokio::signal::unix::signal(
                tokio::signal::unix::SignalKind::interrupt()
            ).expect("Failed to setup SIGINT handler");

            tokio::select! {
                _ = sigterm.recv() => println!("Received SIGTERM"),
                _ = sigint.recv() => println!("Received SIGINT"),
            }
        };

        let polling = tokio::spawn(polling_loop(
            self.control_plane.clone(),
            self.task_queue.clone(),
            shutdown_token.clone(),
        ));

        let execution = tokio::spawn(execution_loop(
            self.control_plane.clone(),
            self.config.clone(),
            self.task_queue.clone(),
            self.active_tasks.clone(),
            self.semaphore.clone(),
            shutdown_token.clone(),
        ));

        let lease_renewal = tokio::spawn(lease_renewal_loop(
            self.control_plane.clone(),
            self.active_tasks.clone(),
            shutdown_token.clone(),
        ));

        shutdown_signal.await;
        println!("Shutdown signal received, starting graceful shutdown...");

        shutdown_token.cancel();
        let _ = tokio::join!(polling, lease_renewal);

        let shutdown_timeout = Duration::from_secs(60);
        let start = Instant::now();

        loop {
            let active_count = self.active_tasks.lock().await.len();

            if active_count == 0 {
                println!("All tasks completed, shutting down");
                break;
            }

            if start.elapsed() > shutdown_timeout {
                println!("Shutdown timeout reached, {} tasks still running", active_count);

                let active = self.active_tasks.lock().await;
                for (task_id, active_task) in active.iter() {
                    println!("Force cancelling task {}", task_id);
                    active_task.cancel_token.cancel();
                }

                tokio::time::sleep(Duration::from_secs(5)).await;
                break;
            }

            println!("Waiting for {} tasks to complete...", active_count);
            tokio::time::sleep(Duration::from_secs(5)).await;
        }

        let _ = execution.await;

        println!("TaskExecutor shutdown complete");
        Ok(())
    }
}
