use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use tokio::fs::{self, OpenOptions};
use tokio::io::AsyncWriteExt;
use tokio::sync::{Mutex, broadcast};

use crate::events::ProviderEvent;
use crate::providers::ProviderRunHandle;

static RUN_COUNTER: AtomicU64 = AtomicU64::new(1);

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum RunStatus {
    Running,
    Completed,
    Failed,
    Cancelled,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeCommandRunContext {
    pub command_id: String,
    pub digital_employee_id: String,
    pub execution_instance_id: String,
    pub provider_type: String,
    pub session_policy: serde_json::Value,
    pub context_refs: Vec<serde_json::Value>,
    pub artifact_refs: Vec<serde_json::Value>,
    pub metadata: serde_json::Value,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RunSpec {
    pub provider_kind: String,
    pub workspace_path: PathBuf,
    pub prompt: String,
    pub session_id: Option<String>,
    pub continue_session: bool,
    pub model: Option<String>,
    pub command_context: Option<RuntimeCommandRunContext>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RunSnapshot {
    pub id: String,
    pub provider_kind: String,
    pub workspace_path: PathBuf,
    pub prompt: String,
    pub session_id: Option<String>,
    pub continue_session: bool,
    pub model: Option<String>,
    pub command_context: Option<RuntimeCommandRunContext>,
    pub provider_session_id: Option<String>,
    pub status: RunStatus,
    pub started_at_ms: u64,
    pub finished_at_ms: Option<u64>,
    pub error: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RunEventRecord {
    pub sequence: u64,
    pub run_id: String,
    pub event: ProviderEvent,
    pub recorded_at_ms: u64,
}

struct RunState {
    snapshot: RunSnapshot,
    events: Vec<RunEventRecord>,
    next_sequence: u64,
    handle: Option<ProviderRunHandle>,
}

#[derive(Clone)]
pub struct RuntimeRunStore {
    log_dir: PathBuf,
    runs: Arc<Mutex<HashMap<String, RunState>>>,
    event_sender: broadcast::Sender<RunEventRecord>,
}

impl RuntimeRunStore {
    pub fn new(log_dir: impl Into<PathBuf>) -> Self {
        let (event_sender, _) = broadcast::channel(1024);
        Self {
            log_dir: log_dir.into(),
            runs: Arc::new(Mutex::new(HashMap::new())),
            event_sender,
        }
    }

    pub async fn start_run(
        &self,
        spec: RunSpec,
        handle: Option<ProviderRunHandle>,
    ) -> anyhow::Result<RunSnapshot> {
        let id = new_run_id();
        let snapshot = RunSnapshot {
            id: id.clone(),
            provider_kind: spec.provider_kind,
            workspace_path: spec.workspace_path,
            prompt: spec.prompt,
            session_id: spec.session_id,
            continue_session: spec.continue_session,
            model: spec.model,
            command_context: spec.command_context,
            provider_session_id: None,
            status: RunStatus::Running,
            started_at_ms: now_ms(),
            finished_at_ms: None,
            error: None,
        };
        fs::create_dir_all(self.run_dir(&id)).await?;

        let mut runs = self.runs.lock().await;
        runs.insert(
            id,
            RunState {
                snapshot: snapshot.clone(),
                events: Vec::new(),
                next_sequence: 1,
                handle,
            },
        );
        Ok(snapshot)
    }

    pub async fn attach_handle(
        &self,
        run_id: &str,
        handle: ProviderRunHandle,
    ) -> anyhow::Result<()> {
        let mut runs = self.runs.lock().await;
        let state = runs
            .get_mut(run_id)
            .ok_or_else(|| anyhow::anyhow!("run not found: {run_id}"))?;
        state.handle = Some(handle);
        Ok(())
    }

    pub async fn record_event(&self, run_id: &str, event: ProviderEvent) -> anyhow::Result<()> {
        let record = {
            let mut runs = self.runs.lock().await;
            let state = runs
                .get_mut(run_id)
                .ok_or_else(|| anyhow::anyhow!("run not found: {run_id}"))?;

            apply_event_to_snapshot(&mut state.snapshot, &event);
            let record = RunEventRecord {
                sequence: state.next_sequence,
                run_id: run_id.to_string(),
                event,
                recorded_at_ms: now_ms(),
            };
            state.next_sequence += 1;
            state.events.push(record.clone());
            record
        };

        self.append_event(&record).await?;
        let _ = self.event_sender.send(record);
        Ok(())
    }

    pub async fn finish_failed(&self, run_id: &str, message: String) -> anyhow::Result<()> {
        self.record_event(run_id, ProviderEvent::TurnError { message })
            .await
    }

    pub async fn cancel_run(&self, run_id: &str, reason: Option<String>) -> anyhow::Result<()> {
        let handle = {
            let mut runs = self.runs.lock().await;
            let state = runs
                .get_mut(run_id)
                .ok_or_else(|| anyhow::anyhow!("run not found: {run_id}"))?;
            state.snapshot.status = RunStatus::Cancelled;
            state.snapshot.finished_at_ms = Some(now_ms());
            state.snapshot.error = reason.clone();
            state.handle.clone()
        };

        if let Some(handle) = handle {
            handle.cancel().await?;
        }
        Ok(())
    }

    pub async fn get_run(&self, run_id: &str) -> Option<RunSnapshot> {
        let runs = self.runs.lock().await;
        runs.get(run_id).map(|state| state.snapshot.clone())
    }

    pub async fn events(&self, run_id: &str) -> Option<Vec<RunEventRecord>> {
        let runs = self.runs.lock().await;
        runs.get(run_id).map(|state| state.events.clone())
    }

    pub fn subscribe(&self) -> broadcast::Receiver<RunEventRecord> {
        self.event_sender.subscribe()
    }

    fn run_dir(&self, run_id: &str) -> PathBuf {
        self.log_dir.join(run_id)
    }

    async fn append_event(&self, record: &RunEventRecord) -> anyhow::Result<()> {
        fs::create_dir_all(self.run_dir(&record.run_id)).await?;
        let path = self.run_dir(&record.run_id).join("events.jsonl");
        let mut file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(path)
            .await?;
        let line = serde_json::to_string(record)?;
        file.write_all(line.as_bytes()).await?;
        file.write_all(b"\n").await?;
        file.flush().await?;
        Ok(())
    }
}

fn apply_event_to_snapshot(snapshot: &mut RunSnapshot, event: &ProviderEvent) {
    match event {
        ProviderEvent::SessionStarted { session_id } => {
            snapshot.provider_session_id = Some(session_id.clone());
        }
        ProviderEvent::TurnCompleted { .. } => {
            if snapshot.status == RunStatus::Running {
                snapshot.status = RunStatus::Completed;
                snapshot.finished_at_ms = Some(now_ms());
            }
        }
        ProviderEvent::TurnError { message } => {
            if snapshot.status != RunStatus::Cancelled {
                snapshot.status = RunStatus::Failed;
                snapshot.finished_at_ms = Some(now_ms());
                snapshot.error = Some(message.clone());
            }
        }
        _ => {}
    }
}

fn new_run_id() -> String {
    let timestamp = now_ms();
    let sequence = RUN_COUNTER.fetch_add(1, Ordering::Relaxed);
    format!("run-{timestamp}-{sequence}")
}

fn now_ms() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis().min(u128::from(u64::MAX)) as u64)
        .unwrap_or_default()
}
