use std::collections::{BTreeMap, HashMap};
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

use serde::{Deserialize, Serialize};
use tokio::fs::{self, OpenOptions};
use tokio::io::AsyncWriteExt;
use tokio::sync::{Mutex, broadcast};

use crate::commands::payload::RuntimeEnvironmentVariablePayload;
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
    /// Legacy alias for the employee capability cache. Do not use it as a
    /// provider auth home.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub agent_home_dir: Option<PathBuf>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub employee_capability_dir: Option<PathBuf>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub capability_manifest_version: Option<String>,
    #[serde(default = "default_provider_auth_mode")]
    pub provider_auth_mode: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mcp_config_path: Option<PathBuf>,
    /// 项目原生技能压过员工侧同 key 技能的冲突清单(spec §3.1),随 attestation
    /// 留痕。空表示无冲突。
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub skill_conflicts: Vec<String>,
    /// 技能懒收敛报告(capability-binding-unification):随 attestation metadata
    /// 落库(key `capability_convergence`)。
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skill_convergence: Option<crate::skills_convergence::SkillConvergenceReport>,
    /// 工作区检出的基线 ref(派发下发的 base_ref,不是从 git 反推)。随
    /// attestation 落 `git_base_ref`:没有它,HEAD 与 diff 都缺一个参照系。
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub workspace_base_ref: Option<String>,
    pub prompt: String,
    pub session_id: Option<String>,
    pub continue_session: bool,
    pub model: Option<String>,
    #[serde(default)]
    pub environment: BTreeMap<String, String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub command_context: Option<RuntimeCommandRunContext>,
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RedactedEnvironmentVariableView {
    pub name: String,
    pub value: String,
    pub sensitive: bool,
}

pub fn redacted_environment_view(
    environment: &[RuntimeEnvironmentVariablePayload],
) -> Vec<RedactedEnvironmentVariableView> {
    environment
        .iter()
        .map(|env| RedactedEnvironmentVariableView {
            name: env.name.clone(),
            value: "[redacted]".to_string(),
            sensitive: env.sensitive,
        })
        .collect()
}

fn default_provider_auth_mode() -> String {
    "host".to_string()
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RunSnapshot {
    pub id: String,
    pub provider_kind: String,
    pub workspace_path: PathBuf,
    /// Legacy alias for the employee capability cache. Do not use it as a
    /// provider auth home.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub agent_home_dir: Option<PathBuf>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub employee_capability_dir: Option<PathBuf>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub capability_manifest_version: Option<String>,
    #[serde(default = "default_provider_auth_mode")]
    pub provider_auth_mode: String,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub mcp_config_path: Option<PathBuf>,
    /// 项目原生技能压过员工侧同 key 技能的冲突清单(spec §3.1),随 attestation
    /// 留痕。空表示无冲突。
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub skill_conflicts: Vec<String>,
    /// 技能懒收敛报告(capability-binding-unification):随 attestation metadata
    /// 落库(key `capability_convergence`)。
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub skill_convergence: Option<crate::skills_convergence::SkillConvergenceReport>,
    pub prompt: String,
    pub session_id: Option<String>,
    pub continue_session: bool,
    pub model: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
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
            agent_home_dir: spec.agent_home_dir,
            employee_capability_dir: spec.employee_capability_dir,
            capability_manifest_version: spec.capability_manifest_version,
            provider_auth_mode: spec.provider_auth_mode,
            mcp_config_path: spec.mcp_config_path,
            skill_conflicts: spec.skill_conflicts,
            skill_convergence: spec.skill_convergence,
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

    pub async fn record_event(
        &self,
        run_id: &str,
        event: ProviderEvent,
    ) -> anyhow::Result<RunEventRecord> {
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
        let _ = self.event_sender.send(record.clone());
        Ok(record)
    }

    pub async fn finish_failed(&self, run_id: &str, message: String) -> anyhow::Result<()> {
        self.record_event(run_id, ProviderEvent::TurnError { message })
            .await
            .map(|_| ())
    }

    pub async fn cancel_run(&self, run_id: &str, reason: Option<String>) -> anyhow::Result<()> {
        let handle = {
            let mut runs = self.runs.lock().await;
            let state = runs
                .get_mut(run_id)
                .ok_or_else(|| anyhow::anyhow!("run not found: {run_id}"))?;
            if state.snapshot.status != RunStatus::Running {
                return Ok(());
            }
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

    /// 非终态 run 引用的工作区路径集合——后台清扫据此跳过活跃目录
    /// (目录与能力投影修订 spec §5)。
    pub async fn active_workspaces(&self) -> std::collections::HashSet<PathBuf> {
        let runs = self.runs.lock().await;
        runs.values()
            .filter(|state| state.snapshot.status == RunStatus::Running)
            .map(|state| state.snapshot.workspace_path.clone())
            .collect()
    }

    /// Running 状态 run 引用的员工能力家目录集合——技能懒收敛据此决定是否允许
    /// prune:有并发 run 正在读同一家目录时跳过删除(下次派发补删)。
    pub async fn active_capability_dirs(&self) -> std::collections::HashSet<PathBuf> {
        let runs = self.runs.lock().await;
        runs.values()
            .filter(|state| state.snapshot.status == RunStatus::Running)
            .filter_map(|state| {
                state
                    .snapshot
                    .employee_capability_dir
                    .clone()
                    .or_else(|| state.snapshot.agent_home_dir.clone())
            })
            .collect()
    }

    pub async fn events(&self, run_id: &str) -> Option<Vec<RunEventRecord>> {
        let runs = self.runs.lock().await;
        runs.get(run_id).map(|state| state.events.clone())
    }

    pub fn subscribe(&self) -> broadcast::Receiver<RunEventRecord> {
        self.event_sender.subscribe()
    }

    pub fn run_dir(&self, run_id: &str) -> PathBuf {
        self.log_dir.join(run_id)
    }

    async fn append_event(&self, record: &RunEventRecord) -> anyhow::Result<()> {
        fs::create_dir_all(self.run_dir(&record.run_id)).await?;
        let path = self.run_dir(&record.run_id).join("events.jsonl");
        let mut options = OpenOptions::new();
        options.create(true).append(true);
        #[cfg(unix)]
        options.mode(0o600);
        let mut file = options.open(path).await?;
        let line = serde_json::to_string(record)?;
        file.write_all(line.as_bytes()).await?;
        file.write_all(b"\n").await?;
        file.flush().await?;
        Ok(())
    }
}

fn apply_event_to_snapshot(snapshot: &mut RunSnapshot, event: &ProviderEvent) {
    match event {
        ProviderEvent::SessionStarted { session_id, .. } => {
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

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    #[cfg(unix)]
    async fn local_events_log_is_owner_only() {
        use std::os::unix::fs::PermissionsExt;
        let dir = tempfile::tempdir().unwrap();
        let store = RuntimeRunStore::new(dir.path());
        let spec = RunSpec {
            provider_kind: "claude".to_string(),
            workspace_path: dir.path().to_path_buf(),
            agent_home_dir: None,
            employee_capability_dir: None,
            capability_manifest_version: None,
            provider_auth_mode: "host".to_string(),
            mcp_config_path: None,
            workspace_base_ref: None,
            skill_conflicts: Vec::new(),
            skill_convergence: None,
            prompt: "test".to_string(),
            session_id: None,
            continue_session: false,
            model: None,
            environment: std::collections::BTreeMap::new(),
            command_context: None,
        };
        let snapshot = store.start_run(spec, None).await.unwrap();
        store
            .record_event(&snapshot.id, ProviderEvent::TurnStarted)
            .await
            .unwrap();

        let mode = std::fs::metadata(dir.path().join(&snapshot.id).join("events.jsonl"))
            .unwrap()
            .permissions()
            .mode();
        assert_eq!(mode & 0o777, 0o600);
    }

    fn capability_run_spec(workspace: &std::path::Path, home: Option<&str>) -> RunSpec {
        RunSpec {
            provider_kind: "claude".to_string(),
            workspace_path: workspace.to_path_buf(),
            agent_home_dir: home.map(PathBuf::from),
            employee_capability_dir: home.map(PathBuf::from),
            capability_manifest_version: None,
            provider_auth_mode: "host".to_string(),
            mcp_config_path: None,
            workspace_base_ref: None,
            skill_conflicts: Vec::new(),
            skill_convergence: None,
            prompt: "test".to_string(),
            session_id: None,
            continue_session: false,
            model: None,
            environment: std::collections::BTreeMap::new(),
            command_context: None,
        }
    }

    #[tokio::test]
    async fn active_capability_dirs_only_includes_running_runs() {
        let dir = tempfile::tempdir().unwrap();
        let store = RuntimeRunStore::new(dir.path());

        let running = store
            .start_run(
                capability_run_spec(dir.path(), Some("/homes/running")),
                None,
            )
            .await
            .unwrap();
        let finished = store
            .start_run(
                capability_run_spec(dir.path(), Some("/homes/finished")),
                None,
            )
            .await
            .unwrap();
        let homeless = store
            .start_run(capability_run_spec(dir.path(), None), None)
            .await
            .unwrap();
        store
            .record_event(
                &finished.id,
                ProviderEvent::TurnError {
                    message: "boom".to_string(),
                },
            )
            .await
            .unwrap();

        let dirs = store.active_capability_dirs().await;
        assert!(dirs.contains(&PathBuf::from("/homes/running")));
        assert!(
            !dirs.contains(&PathBuf::from("/homes/finished")),
            "terminal runs must not block pruning"
        );
        assert_eq!(dirs.len(), 1, "runs without a home contribute nothing");

        let _ = (running, homeless);
    }
}
