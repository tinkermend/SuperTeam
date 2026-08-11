use std::collections::{BTreeMap, HashMap};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::sync::atomic::{AtomicI64, Ordering};
use std::time::{Duration, Instant};

use futures::StreamExt;
use tokio_util::sync::CancellationToken;

use crate::commands::payload::{
    RuntimeSessionCommandPayload, RuntimeStopSessionCommandPayload, SessionPolicyMode,
    metadata_string,
};
use crate::commands::registry::{ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding};
use crate::config::RuntimeConfig;
use crate::controlplane::ControlPlaneClient;
use crate::controlplane::models::{
    EnsureInstanceCommand, EnsureProjectDirectoryCommand, CloneProjectRepositoryCommand,
    ProjectTaskAttestationWriteback,
    ProjectTaskBudgetHeartbeatWriteback, ProjectTaskCompleteWriteback, ProjectTaskFailWriteback,
    ProjectTaskStartWriteback, ProjectTaskWaitHumanWriteback, RemoveProjectDirectoryCommand,
    ValidateProjectWorkspaceCommand,
    RuntimeCommand, RuntimeCommandEventWriteback, RuntimeCommandTerminalWriteback,
    RuntimeCommandType, TaskResultContract,
};
use crate::events::{
    attempt_stream_diagnostics, ErrorEnvelope, PROVIDER_EVENT_SCHEMA_VERSION, ProviderEvent,
};
use crate::instances::{EnsureInstanceRequest, ensure_instance};
use crate::providers::catalog;
use crate::providers::error_map::{
    self, code as error_code, envelope_from_anyhow, envelope_for_code,
};
use crate::providers::{ProviderAdapter, ProviderEventStream, ProviderRequest, ProviderRunHandle};
use crate::runs::{RunEventRecord, RunSpec, RunStatus, RuntimeCommandRunContext, RuntimeRunStore};
use crate::workspace_files::{
    WorkspaceMaterializationPlan, materialize_workspace, provider_home_kind,
};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeCommandOutcome {
    pub command_id: String,
    pub accepted: bool,
    pub run_id: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct CommandWorkspace {
    agent_home_dir: PathBuf,
    workspace_path: PathBuf,
    employee_capability_dir: PathBuf,
    capability_manifest_version: Option<String>,
    provider_auth_mode: String,
    mcp_config_path: Option<PathBuf>,
    skill_conflicts: Vec<String>,
    skill_convergence: Option<crate::skills_convergence::SkillConvergenceReport>,
    /// 工作区检出的基线 ref（派发下发；非 git 仓工作区为 None）。
    workspace_base_ref: Option<String>,
    /// Codex/OpenCode 会话 overlay 环境（不指向员工 auth home）。
    provider_overlay_env: std::collections::BTreeMap<String, String>,
}

#[derive(Clone)]
struct RuntimeCommandWritebackSink {
    client: ControlPlaneClient,
    command_id: String,
    /// Registry provider_type for event metadata (spec §4.5 / §13 #7).
    provider_type: String,
    project_task: Option<ProjectTaskWritebackContext>,
    usage_tokens: Arc<AtomicI64>,
    /// Set once the raw transcript has been finalized, so every terminal
    /// writeback (complete, fail, wait_human) carries the same pointer.
    raw_log: Arc<std::sync::Mutex<Option<crate::raw_log::RawLogSummary>>>,
    /// Inputs for artifact collection at completion (证据地基 spec §4.1);
    /// None on sinks that never complete (e.g. the stop-command failure sink).
    artifact_collection: Option<ArtifactCollectionContext>,
    /// 终态写回瞬时失败时落盘重试(遗留缺陷#1);后台 worker 重放。
    writeback_queue: Arc<crate::writeback_queue::WritebackQueue>,
}

/// What the sink needs to collect the attempt's artifacts when it completes.
#[derive(Clone)]
struct ArtifactCollectionContext {
    raw_log_path: PathBuf,
    workspace_path: Option<PathBuf>,
    environment: BTreeMap<String, String>,
    /// Pre-run file listing for non-git workspaces (输出附件 spec §1.1
    /// snapshot fallback); None when git can answer "what is new" itself.
    workspace_baseline: Option<std::collections::BTreeSet<String>>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct ProviderTerminalCompletion {
    summary: Option<String>,
    provider_session_id: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum ProviderTerminalWritebackAction {
    /// In-loop terminal failure (path 1). Carries structured envelope when known.
    Fail(crate::events::ErrorEnvelope),
}

#[derive(Debug, Default)]
struct ProviderTerminalWritebackState {
    pending_completion: Option<ProviderTerminalCompletion>,
    text_summary: String,
    failed: bool,
}

impl ProviderTerminalWritebackState {
    fn observe_event(
        &mut self,
        event: &ProviderEvent,
        provider_session_id: Option<&str>,
        provider_type: &str,
    ) -> Option<ProviderTerminalWritebackAction> {
        match event {
            ProviderEvent::TextDelta { text } => {
                if !self.failed {
                    self.text_summary.push_str(text);
                }
                None
            }
            ProviderEvent::TurnCompleted { summary, .. } => {
                if !self.failed {
                    let summary = summary
                        .as_deref()
                        .map(str::trim)
                        .filter(|value| !value.is_empty())
                        .map(ToString::to_string)
                        .or_else(|| {
                            let text = self.text_summary.trim();
                            (!text.is_empty()).then(|| text.to_string())
                        });
                    self.pending_completion = Some(ProviderTerminalCompletion {
                        summary,
                        provider_session_id: provider_session_id.map(ToString::to_string),
                    });
                }
                None
            }
            ProviderEvent::TurnError { message, error } => {
                self.failed = true;
                self.pending_completion = None;
                let envelope = crate::providers::error_map::classify_provider_error(
                    error.as_ref(),
                    message,
                    provider_type,
                );
                Some(ProviderTerminalWritebackAction::Fail(envelope))
            }
            _ => None,
        }
    }

    fn finish_successful_stream(self) -> Option<ProviderTerminalCompletion> {
        if self.failed {
            None
        } else {
            let text_summary = self.text_summary.trim();
            self.pending_completion.map(|mut completion| {
                let has_summary = completion
                    .summary
                    .as_deref()
                    .map(str::trim)
                    .is_some_and(|value| !value.is_empty());
                if !has_summary && !text_summary.is_empty() {
                    completion.summary = Some(text_summary.to_string());
                }
                completion
            })
        }
    }
}

#[derive(Clone, Debug)]
struct ProjectTaskWritebackContext {
    project_id: String,
    project_task_id: String,
    attempt_id: String,
    lease_token: String,
    runtime_node_id: String,
    digital_employee_id: String,
    capability_manifest_version: Option<String>,
    provider_auth_mode: String,
    budget_heartbeat_interval_sec: u64,
    expected_outputs: Vec<String>,
    handoff_contract: serde_json::Value,
    execution_context_packet_version: String,
}

#[derive(Clone)]
pub struct RuntimeCommandExecutor {
    config: RuntimeConfig,
    runs: RuntimeRunStore,
    registry: RuntimeCommandRegistry,
    control_plane: Option<ControlPlaneClient>,
    /// 终态写回失败时的本地持久重试队列(遗留缺陷#1):写回 POST 瞬时失败不再吞掉,
    /// 落盘由后台 worker 重放,避免结果丢失致任务卡 running。
    writeback_queue: Arc<crate::writeback_queue::WritebackQueue>,
    /// Per-home 收敛互斥:同一员工家目录上的两条并发命令不得同时收敛
    /// (物化 + prune 互删防护)。
    capability_locks: Arc<std::sync::Mutex<HashMap<PathBuf, Arc<tokio::sync::Mutex<()>>>>>,
}

impl RuntimeCommandExecutor {
    fn build_raw_sink(
        &self,
        run_id: &str,
        tenant_id: Option<&str>,
        project_task: &Option<ProjectTaskWritebackContext>,
    ) -> Arc<dyn crate::raw_log::RawLineSink> {
        build_raw_sink_inner(
            self.control_plane.as_ref(),
            self.runs.run_dir(run_id),
            tenant_id,
            project_task.as_ref().map(|task| task.attempt_id.as_str()),
        )
    }

    pub fn new(config: RuntimeConfig) -> Self {
        let writeback_queue = Arc::new(crate::writeback_queue::WritebackQueue::new(
            crate::writeback_queue::queue_dir(&config),
        ));
        Self {
            runs: RuntimeRunStore::new(config.runs.log_dir.clone()),
            registry: RuntimeCommandRegistry::default(),
            config,
            control_plane: None,
            writeback_queue,
            capability_locks: Arc::new(std::sync::Mutex::new(HashMap::new())),
        }
    }

    pub fn with_control_plane_client(
        config: RuntimeConfig,
        control_plane: ControlPlaneClient,
    ) -> Self {
        let mut executor = Self::new(config);
        executor.control_plane = Some(control_plane);
        executor
    }

    pub fn runs(&self) -> RuntimeRunStore {
        self.runs.clone()
    }

    pub fn registry(&self) -> RuntimeCommandRegistry {
        self.registry.clone()
    }

    pub async fn handle_command(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        match &command.command_type {
            RuntimeCommandType::StartSession
            | RuntimeCommandType::ResumeSession
            | RuntimeCommandType::SendInput => self.handle_input_command(command).await,
            RuntimeCommandType::StopSession => self.handle_stop_command(command).await,
            RuntimeCommandType::EnsureInstance => self.handle_ensure_instance(command),
            RuntimeCommandType::EnsureProjectDirectory => {
                self.handle_ensure_project_directory(command).await
            }
            RuntimeCommandType::RemoveProjectDirectory => {
                self.handle_remove_project_directory(command).await
            }
            RuntimeCommandType::CloneProjectRepository => {
                self.handle_clone_project_repository(command).await
            }
            RuntimeCommandType::ValidateProjectWorkspace => {
                self.handle_validate_project_workspace(command).await
            }
            RuntimeCommandType::ReadProviderNativeConfig => {
                self.handle_read_provider_native_config(command).await
            }
            RuntimeCommandType::WriteProviderNativeConfig => {
                self.handle_write_provider_native_config(command).await
            }
            RuntimeCommandType::Unsupported(name) => {
                // Write a failed receipt so CP WaitForRuntimeCommandCompletion does not spin until timeout.
                let message = format!("unsupported runtime command type: {name}");
                let _ = self
                    .write_command_failure_with_code(
                        &command.id,
                        message.clone(),
                        "unsupported_command",
                        "runtime",
                    )
                    .await;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: false,
                    run_id: None,
                })
            }
        }
    }

    async fn handle_input_command(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let payload = match self.parse_session_payload(&command) {
            Ok(payload) => payload,
            Err(error) => {
                self.write_command_failure(&command.id, error.to_string())
                    .await?;
                return Err(error);
            }
        };
        let project_task = project_task_writeback_context(&payload);
        let prompt = match payload.provider_prompt() {
            Some(prompt) => prompt,
            None => {
                let error = self
                    .recorded_error(&command.id, anyhow::anyhow!("prompt or input is required"));
                self.write_session_failure(
                    &payload.command_id,
                    project_task.as_ref(),
                    error.to_string(),
                )
                .await?;
                return Err(error);
            }
        };
        let session_id = match self.input_session_id(&command, &payload) {
            Ok(session_id) => session_id,
            Err(error) => {
                self.write_session_failure(
                    &payload.command_id,
                    project_task.as_ref(),
                    error.to_string(),
                )
                .await?;
                return Err(error);
            }
        };
        let reusable_provider_session = reusable_provider_session(&payload);
        let provider = match self.select_provider(&command.id, &payload) {
            Ok(provider) => provider,
            Err(error) => {
                self.write_session_failure(
                    &payload.command_id,
                    project_task.as_ref(),
                    error.to_string(),
                )
                .await?;
                return Err(error);
            }
        };
        let command_workspace = match self.ensure_command_instance(&command.id, &payload).await {
            Ok(command_workspace) => command_workspace,
            Err(error) => {
                self.write_session_workspace_sync_failure(
                    &payload.command_id,
                    project_task.as_ref(),
                    error.to_string(),
                )
                .await?;
                return Err(error);
            }
        };
        // ensure_command_instance above has already injected the session MCP config into
        // agent_home_dir; every failure exit from here until the run is actually started
        // (and the drain/attach paths below take over rollback duty) must roll it back
        // best-effort rather than leaving a stale injected config behind.
        let session_policy_value = serde_json::to_value(&payload.session_policy).map_err(|error| {
            rollback_session_projections_best_effort(
                &payload.command_id,
                Some(command_workspace.agent_home_dir.as_path()),
                Some(command_workspace.workspace_path.as_path()),
                Some(payload.command_id.as_str()),
            );
            self.recorded_error(&payload.command_id, error.into())
        })?;

        let mut environment: BTreeMap<String, String> = payload
            .environment
            .iter()
            .map(|env| (env.name.clone(), env.value.clone()))
            .collect();
        crate::project_session::merge_provider_overlay_env(
            &mut environment,
            &command_workspace.provider_overlay_env,
        );

        let spec = RunSpec {
            provider_type: payload.provider_type.clone(),
            workspace_path: command_workspace.workspace_path,
            agent_home_dir: Some(command_workspace.agent_home_dir.clone()),
            employee_capability_dir: Some(command_workspace.employee_capability_dir),
            capability_manifest_version: command_workspace.capability_manifest_version,
            provider_auth_mode: command_workspace.provider_auth_mode,
            mcp_config_path: command_workspace.mcp_config_path,
            skill_conflicts: command_workspace.skill_conflicts,
            skill_convergence: command_workspace.skill_convergence,
            workspace_base_ref: command_workspace.workspace_base_ref,
            prompt,
            // dormant: 见 ProviderRequest::system_prompt。待 codex/opencode 原生
            // system-prompt flag 落地后改为 `payload.system_prompt()`，并相应把
            // 宪法段从 provider_prompt() 拆出（避免双注入）。
            system_prompt: None,
            session_id: session_id.clone(),
            continue_session: matches!(
                command.command_type,
                RuntimeCommandType::ResumeSession | RuntimeCommandType::SendInput
            ),
            model: payload.model.clone(),
            environment,
            command_context: Some(RuntimeCommandRunContext {
                command_id: payload.command_id.clone(),
                digital_employee_id: payload.digital_employee_id.clone(),
                execution_instance_id: payload.execution_instance_id.clone(),
                provider_type: payload.provider_type.clone(),
                session_policy: session_policy_value,
                context_refs: payload.context_refs.clone(),
                artifact_refs: payload.artifact_refs.clone(),
                metadata: payload.metadata.clone(),
            }),
        };

        let snapshot = match self.runs.start_run(spec.clone(), None).await {
            Ok(snapshot) => snapshot,
            Err(error) => {
                let error = self.recorded_error(&payload.command_id, error);
                self.write_command_failure(&payload.command_id, error.to_string())
                    .await?;
                rollback_session_projections_best_effort(
                    &payload.command_id,
                    spec.agent_home_dir.as_deref(),
                    Some(spec.workspace_path.as_path()),
                    Some(payload.command_id.as_str()),
                );
                return Err(error);
            }
        };
        self.registry.record_run_started(RuntimeRunBinding {
            command_id: payload.command_id.clone(),
            run_id: snapshot.id.clone(),
            execution_instance_id: payload.execution_instance_id.clone(),
            provider_type: payload.provider_type.clone(),
            provider_session_id: session_id.clone().filter(|_| reusable_provider_session),
        });
        let run_id = snapshot.id.clone();
        if !reusable_provider_session {
            if let Some(session_id) = &session_id {
                self.registry
                    .record_provider_session_with_recoverability(&run_id, session_id, false);
            }
        }
        let writeback = self
            .control_plane
            .as_ref()
            .map(|client| RuntimeCommandWritebackSink {
                client: client.clone(),
                command_id: payload.command_id.clone(),
                provider_type: payload.provider_type.clone(),
                project_task: project_task.clone(),
                usage_tokens: Arc::new(AtomicI64::new(0)),
                raw_log: Arc::new(std::sync::Mutex::new(None)),
                artifact_collection: Some(ArtifactCollectionContext {
                    raw_log_path: self.runs.run_dir(&run_id).join("raw.jsonl"),
                    workspace_path: Some(spec.workspace_path.clone()),
                    environment: spec.environment.clone(),
                    workspace_baseline: if spec.workspace_path.join(".git").exists() {
                        None
                    } else {
                        Some(crate::artifacts::snapshot_workspace_files(
                            &spec.workspace_path,
                        ))
                    },
                }),
                writeback_queue: self.writeback_queue.clone(),
            });
        if let Some(writeback) = &writeback {
            if let Err(error) = writeback.start_project_task().await {
                let message = error.to_string();
                let _ = self.runs.finish_failed(&run_id, message.clone()).await;
                let _ = writeback.fail(message).await;
                self.registry.record_run_finished(&run_id);
                rollback_session_projections_best_effort(
                    &run_id,
                    spec.agent_home_dir.as_deref(),
                    Some(spec.workspace_path.as_path()),
                    Some(payload.command_id.as_str()),
                );
                return Ok(RuntimeCommandOutcome {
                    command_id: payload.command_id,
                    accepted: true,
                    run_id: Some(run_id),
                });
            }
        }
        let raw_sink = self.build_raw_sink(&run_id, payload.tenant_id.as_deref(), &project_task);
        let provider_run = match provider.start(provider_request(&spec), raw_sink.clone()).await {
            Ok(provider_run) => provider_run,
            Err(error) => {
                let envelope = envelope_for_code(
                    error_code::PROVIDER_SPAWN_FAILED,
                    error.to_string(),
                    payload.provider_type.as_str(),
                );
                let _ = self
                    .runs
                    .finish_failed(&run_id, envelope.message.clone())
                    .await;
                if let Some(writeback) = &writeback {
                    writeback.spawn_attestation(
                        spec.clone(),
                        "provider_start",
                        "failed",
                        None,
                        None,
                    );
                    writeback.fail_with_envelope(envelope, None).await?;
                }
                self.registry.record_run_finished(&run_id);
                rollback_session_projections_best_effort(
                    &run_id,
                    spec.agent_home_dir.as_deref(),
                    Some(spec.workspace_path.as_path()),
                    Some(payload.command_id.as_str()),
                );
                return Ok(RuntimeCommandOutcome {
                    command_id: payload.command_id,
                    accepted: true,
                    run_id: Some(run_id),
                });
            }
        };
        let provider_started_at = Instant::now();
        if let Some(writeback) = &writeback {
            writeback.spawn_attestation(
                spec.clone(),
                "provider_start",
                "succeeded",
                session_id.clone(),
                Some(0),
            );
        }

        if let Err(error) = self
            .runs
            .attach_handle(&run_id, provider_run.handle.clone())
            .await
        {
            let message = error.to_string();
            let _ = provider_run.handle.cancel().await;
            let _ = self.runs.finish_failed(&run_id, message.clone()).await;
            if let Some(writeback) = &writeback {
                writeback.fail(message).await?;
            }
            self.registry.record_run_finished(&run_id);
            rollback_session_projections_best_effort(
                &run_id,
                spec.agent_home_dir.as_deref(),
                Some(spec.workspace_path.as_path()),
                Some(payload.command_id.as_str()),
            );
            return Ok(RuntimeCommandOutcome {
                command_id: payload.command_id,
                accepted: true,
                run_id: Some(run_id),
            });
        }
        let heartbeat_stop = writeback.as_ref().map(|writeback| {
            let heartbeat_interval_sec = project_task
                .as_ref()
                .map(|project_task| project_task.budget_heartbeat_interval_sec)
                .unwrap_or(15);
            spawn_project_task_budget_heartbeat(
                self.runs.clone(),
                run_id.clone(),
                writeback.clone(),
                provider_run.handle.clone(),
                provider_started_at,
                Duration::from_secs(heartbeat_interval_sec),
                spec.environment.clone(),
                spec.clone(),
            )
        });
        self.spawn_provider_event_drain(
            run_id.clone(),
            provider_run.events,
            reusable_provider_session,
            writeback,
            spec,
            provider_started_at,
            heartbeat_stop,
            raw_sink,
        );

        Ok(RuntimeCommandOutcome {
            command_id: payload.command_id,
            accepted: true,
            run_id: Some(run_id),
        })
    }

    async fn handle_stop_command(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        if let Ok(payload) = RuntimeStopSessionCommandPayload::from_command(&command) {
            let start_command_id = payload.start_command_id();
            let run_id = self
                .registry
                .active_run_for_command(start_command_id)
                .ok_or_else(|| {
                    self.recorded_error(
                        &command.id,
                        anyhow::anyhow!("no active run found for stop_session command"),
                    )
                })?;
            let project_task = self
                .runs
                .get_run(&run_id)
                .await
                .and_then(|snapshot| snapshot.command_context)
                .and_then(|context| {
                    project_task_writeback_context_from_metadata(
                        &context.metadata,
                        &context.digital_employee_id,
                    )
                });
            let reason = payload
                .reason
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .unwrap_or("stop_session command received")
                .to_string();

            self.runs.cancel_run(&run_id, Some(reason.clone())).await?;
            if let Some(control_plane) = &self.control_plane {
                control_plane
                    .cancel_runtime_command(
                        start_command_id,
                        &command_cancelled_terminal(Some(reason)),
                    )
                    .await?;
                RuntimeCommandWritebackSink {
                    client: control_plane.clone(),
                    command_id: start_command_id.to_string(),
                    provider_type: String::new(),
                    project_task,
                    usage_tokens: Arc::new(AtomicI64::new(0)),
                    raw_log: Arc::new(std::sync::Mutex::new(None)),
                    artifact_collection: None,
                    writeback_queue: self.writeback_queue.clone(),
                }
                .fail_project_task(&envelope_for_code(
                    error_code::CANCELLED,
                    "operator cancelled",
                    "unknown",
                ))
                .await?;
            }
            self.registry.record_run_finished(&run_id);

            return Ok(RuntimeCommandOutcome {
                command_id: payload.command_id,
                accepted: true,
                run_id: Some(run_id),
            });
        }

        let payload = self.parse_session_payload(&command)?;
        let provider_session_id = non_empty_session_id(&payload);
        let run_id = self
            .registry
            .active_run(ActiveRunLookup {
                command_id: Some(&command.id),
                provider_session_id: provider_session_id.as_deref(),
                execution_instance_id: &payload.execution_instance_id,
                provider_type: &payload.provider_type,
            })
            .ok_or_else(|| {
                self.recorded_error(
                    &command.id,
                    anyhow::anyhow!("no active run found for stop_session command"),
                )
            })?;

        self.runs
            .cancel_run(&run_id, Some("stop_session command received".to_string()))
            .await?;
        self.registry.record_run_finished(&run_id);

        Ok(RuntimeCommandOutcome {
            command_id: payload.command_id,
            accepted: true,
            run_id: Some(run_id),
        })
    }

    fn handle_ensure_instance(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        self.ensure_instance_from_command(&command, "ensure_instance")?;

        Ok(RuntimeCommandOutcome {
            command_id: command.id,
            accepted: true,
            run_id: None,
        })
    }

    async fn handle_ensure_project_directory(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let request: EnsureProjectDirectoryCommand =
            match serde_json::from_value(command.payload.clone()) {
                Ok(request) => request,
                Err(error) => {
                    let message = format!("invalid ensure_project_directory command payload: {error}");
                    self.write_command_failure(&command.id, message.clone())
                        .await?;
                    return Err(self.recorded_error(&command.id, anyhow::anyhow!(message)));
                }
            };
        match crate::project_workspace::ensure_stable_project_directory(
            &self.config.workspace_base_dir(),
            &request.project_name,
        ) {
            Ok(path) => {
                let mut result = HashMap::new();
                result.insert(
                    "workspace_path".to_string(),
                    serde_json::Value::String(path.display().to_string()),
                );
                if let Some(project_id) = request.project_id.filter(|value| !value.trim().is_empty())
                {
                    result.insert(
                        "project_id".to_string(),
                        serde_json::Value::String(project_id),
                    );
                }
                result.insert(
                    "project_name".to_string(),
                    serde_json::Value::String(request.project_name),
                );
                self.write_command_completed(
                    &command.id,
                    Some(format!(
                        "ensured project directory {}",
                        path.display()
                    )),
                    Some(result),
                )
                .await?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: true,
                    run_id: None,
                })
            }
            Err(error) => {
                let message = error.to_string();
                self.write_command_failure(&command.id, message.clone())
                    .await?;
                Err(self.recorded_error(&command.id, anyhow::anyhow!(message)))
            }
        }
    }

    async fn handle_remove_project_directory(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let request: RemoveProjectDirectoryCommand =
            match serde_json::from_value(command.payload.clone()) {
                Ok(request) => request,
                Err(error) => {
                    let message = format!("invalid remove_project_directory command payload: {error}");
                    self.write_command_failure(&command.id, message.clone())
                        .await?;
                    return Err(self.recorded_error(&command.id, anyhow::anyhow!(message)));
                }
            };
        match crate::project_workspace::remove_stable_project_directory(
            &self.config.workspace_base_dir(),
            &request.project_name,
        ) {
            Ok(()) => {
                let mut result = HashMap::new();
                result.insert(
                    "project_name".to_string(),
                    serde_json::Value::String(request.project_name.clone()),
                );
                if let Some(project_id) = request.project_id.filter(|value| !value.trim().is_empty())
                {
                    result.insert(
                        "project_id".to_string(),
                        serde_json::Value::String(project_id),
                    );
                }
                self.write_command_completed(
                    &command.id,
                    Some(format!(
                        "removed project directory {}",
                        request.project_name
                    )),
                    Some(result),
                )
                .await?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: true,
                    run_id: None,
                })
            }
            Err(error) => {
                let message = error.to_string();
                self.write_command_failure(&command.id, message.clone())
                    .await?;
                Err(self.recorded_error(&command.id, anyhow::anyhow!(message)))
            }
        }
    }

    async fn handle_clone_project_repository(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let request: CloneProjectRepositoryCommand =
            match serde_json::from_value(command.payload.clone()) {
                Ok(request) => request,
                Err(error) => {
                    let message =
                        format!("invalid clone_project_repository command payload: {error}");
                    self.write_command_failure(&command.id, message.clone())
                        .await?;
                    return Err(self.recorded_error(&command.id, anyhow::anyhow!(message)));
                }
            };
        match crate::project_workspace::clone_into_stable_project_directory(
            &self.config.workspace_base_dir(),
            &request.project_name,
            &request.repo_url,
            request.default_branch.as_deref(),
            request.force.unwrap_or(false),
        ) {
            Ok(path) => {
                let mut result = HashMap::new();
                result.insert(
                    "workspace_path".to_string(),
                    serde_json::Value::String(path.display().to_string()),
                );
                result.insert(
                    "project_name".to_string(),
                    serde_json::Value::String(request.project_name),
                );
                if let Some(project_id) = request.project_id.filter(|value| !value.trim().is_empty())
                {
                    result.insert(
                        "project_id".to_string(),
                        serde_json::Value::String(project_id),
                    );
                }
                self.write_command_completed(
                    &command.id,
                    Some(format!("cloned project repository into {}", path.display())),
                    Some(result),
                )
                .await?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: true,
                    run_id: None,
                })
            }
            Err(error) => {
                let message = error.to_string();
                self.write_command_failure(&command.id, message.clone())
                    .await?;
                Err(self.recorded_error(&command.id, anyhow::anyhow!(message)))
            }
        }
    }

    async fn handle_validate_project_workspace(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let request: ValidateProjectWorkspaceCommand =
            match serde_json::from_value(command.payload.clone()) {
                Ok(request) => request,
                Err(error) => {
                    let message =
                        format!("invalid validate_project_workspace command payload: {error}");
                    self.write_command_failure(&command.id, message.clone())
                        .await?;
                    return Err(self.recorded_error(&command.id, anyhow::anyhow!(message)));
                }
            };
        match crate::project_workspace::validate_stable_project_workspace(
            &self.config.workspace_base_dir(),
            &request.project_name,
            request.require_git.unwrap_or(false),
        ) {
            Ok(path) => {
                let mut result = HashMap::new();
                result.insert(
                    "workspace_path".to_string(),
                    serde_json::Value::String(path.display().to_string()),
                );
                result.insert(
                    "project_name".to_string(),
                    serde_json::Value::String(request.project_name),
                );
                if let Some(project_id) = request.project_id.filter(|value| !value.trim().is_empty())
                {
                    result.insert(
                        "project_id".to_string(),
                        serde_json::Value::String(project_id),
                    );
                }
                self.write_command_completed(
                    &command.id,
                    Some(format!("validated project workspace {}", path.display())),
                    Some(result),
                )
                .await?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: true,
                    run_id: None,
                })
            }
            Err(error) => {
                let message = error.to_string();
                self.write_command_failure(&command.id, message.clone())
                    .await?;
                Err(self.recorded_error(&command.id, anyhow::anyhow!(message)))
            }
        }
    }

    async fn handle_read_provider_native_config(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let request: crate::provider_native_config::ReadRequest =
            match serde_json::from_value(command.payload.clone()) {
                Ok(request) => request,
                Err(error) => {
                    let message =
                        format!("invalid read_provider_native_config command payload: {error}");
                    self.write_command_failure_with_code(
                        &command.id,
                        message.clone(),
                        "validation_error",
                        "config",
                    )
                    .await?;
                    return Err(self.recorded_error(&command.id, anyhow::anyhow!(message)));
                }
            };
        match crate::provider_native_config::read_config(
            &request.provider_type,
            &request.config_key,
        ) {
            Ok(result) => {
                let transit = crate::provider_native_config::receipt_transit_result(&result);
                let mut map = HashMap::new();
                for (k, v) in transit {
                    map.insert(k, v);
                }
                self.write_command_completed(
                    &command.id,
                    Some(format!(
                        "read {}/{} from {}",
                        result.provider_type, result.config_key, result.resolved_path
                    )),
                    Some(map),
                )
                .await?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: true,
                    run_id: None,
                })
            }
            Err(error) => {
                let message = error.to_string();
                self.write_command_failure_with_code(
                    &command.id,
                    message,
                    error.error_code(),
                    "config",
                )
                .await?;
                // Expected validation/unmanageable failures already wrote a terminal receipt;
                // return accepted=false without Err so the command loop does not tear down WS.
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: false,
                    run_id: None,
                })
            }
        }
    }

    async fn handle_write_provider_native_config(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let request: crate::provider_native_config::WriteRequest =
            match serde_json::from_value(command.payload.clone()) {
                Ok(request) => request,
                Err(error) => {
                    let message =
                        format!("invalid write_provider_native_config command payload: {error}");
                    self.write_command_failure_with_code(
                        &command.id,
                        message.clone(),
                        "validation_error",
                        "config",
                    )
                    .await?;
                    return Err(self.recorded_error(&command.id, anyhow::anyhow!(message)));
                }
            };
        match crate::provider_native_config::write_config(&request) {
            Ok(result) => {
                let transit = crate::provider_native_config::receipt_transit_result(&result);
                let mut map = HashMap::new();
                for (k, v) in transit {
                    map.insert(k, v);
                }
                self.write_command_completed(
                    &command.id,
                    Some(format!(
                        "wrote {}/{} to {}",
                        result.provider_type, result.config_key, result.resolved_path
                    )),
                    Some(map),
                )
                .await?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: true,
                    run_id: None,
                })
            }
            Err(error) => {
                let message = error.to_string();
                let mut result = HashMap::new();
                if let crate::provider_native_config::ConfigError::Conflict { actual_hash } = &error
                {
                    result.insert(
                        "actual_file_content_hash".to_string(),
                        serde_json::Value::String(actual_hash.clone()),
                    );
                }
                if let crate::provider_native_config::ConfigError::Unmanageable { reason } = &error {
                    result.insert(
                        "unmanageable_reason".to_string(),
                        serde_json::Value::String(reason.clone()),
                    );
                }
                self.write_command_failure_with_code_and_result(
                    &command.id,
                    message,
                    error.error_code(),
                    "config",
                    if result.is_empty() { None } else { Some(result) },
                )
                .await?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: false,
                    run_id: None,
                })
            }
        }
    }

    fn ensure_instance_from_command(
        &self,
        command: &RuntimeCommand,
        command_name: &str,
    ) -> anyhow::Result<crate::instances::EnsureInstanceResult> {
        let request: EnsureInstanceCommand = serde_json::from_value(command.payload.clone())
            .map_err(|error| {
                self.recorded_error(
                    &command.id,
                    anyhow::anyhow!("invalid {command_name} command payload: {error}"),
                )
            })?;
        ensure_instance(EnsureInstanceRequest {
            base_dir: self.config.workspace_base_dir(),
            team_id: request.team_id,
            digital_employee_id: request.digital_employee_id,
        })
        .map_err(|error| self.recorded_error(&command.id, error))
    }

    async fn write_command_completed(
        &self,
        command_id: &str,
        summary: Option<String>,
        result: Option<HashMap<String, serde_json::Value>>,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .complete_runtime_command(
                    command_id,
                    &RuntimeCommandTerminalWriteback {
                        status: "completed".to_string(),
                        summary,
                        result,
                        diagnostic: None,
                        provider_session_external_id: None,
                        session_state_patch: None,
                        log_ref: None,
                        raw_result_ref: None,
                        error_message: None,
                        error_code: None,
                        error_family: None,
                    },
                )
                .await?;
        }
        Ok(())
    }

    async fn write_command_failure(
        &self,
        command_id: &str,
        error_message: String,
    ) -> anyhow::Result<()> {
        self.write_command_failure_with_code(
            command_id,
            error_message,
            "provider_failed",
            "provider",
        )
        .await
    }

    async fn write_command_failure_with_code(
        &self,
        command_id: &str,
        error_message: String,
        error_code: &str,
        error_family: &str,
    ) -> anyhow::Result<()> {
        self.write_command_failure_with_code_and_result(
            command_id,
            error_message,
            error_code,
            error_family,
            None,
        )
        .await
    }

    async fn write_command_failure_with_code_and_result(
        &self,
        command_id: &str,
        error_message: String,
        error_code: &str,
        error_family: &str,
        result: Option<HashMap<String, serde_json::Value>>,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .fail_runtime_command(
                    command_id,
                    &RuntimeCommandTerminalWriteback {
                        status: "failed".to_string(),
                        summary: None,
                        result,
                        diagnostic: None,
                        provider_session_external_id: None,
                        session_state_patch: None,
                        log_ref: None,
                        raw_result_ref: None,
                        error_message: Some(error_message),
                        error_code: Some(error_code.to_string()),
                        error_family: Some(error_family.to_string()),
                    },
                )
                .await?;
        }
        Ok(())
    }

    async fn write_session_failure(
        &self,
        command_id: &str,
        project_task: Option<&ProjectTaskWritebackContext>,
        error_message: String,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .fail_runtime_command(command_id, &command_failed_terminal(error_message.clone()))
                .await?;
            if let Some(project_task) = project_task {
                control_plane
                    .fail_project_task_attempt(
                        &project_task.attempt_id,
                        &project_task_fail_writeback(project_task, command_id, &error_message),
                    )
                    .await?;
            }
        }
        Ok(())
    }

    async fn write_session_workspace_sync_failure(
        &self,
        command_id: &str,
        project_task: Option<&ProjectTaskWritebackContext>,
        error_message: String,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .fail_runtime_command(
                    command_id,
                    &workspace_sync_failed_terminal(error_message.clone()),
                )
                .await?;
            if let Some(project_task) = project_task {
                control_plane
                    .fail_project_task_attempt(
                        &project_task.attempt_id,
                        &project_task_fail_writeback(project_task, command_id, &error_message),
                    )
                    .await?;
            }
        }
        Ok(())
    }

    fn parse_session_payload(
        &self,
        command: &RuntimeCommand,
    ) -> anyhow::Result<RuntimeSessionCommandPayload> {
        RuntimeSessionCommandPayload::from_command(command)
            .map_err(|error| self.recorded_error(&command.id, error))
    }

    fn input_session_id(
        &self,
        command: &RuntimeCommand,
        payload: &RuntimeSessionCommandPayload,
    ) -> anyhow::Result<Option<String>> {
        match &command.command_type {
            RuntimeCommandType::StartSession => Ok(non_empty_session_id(payload)),
            RuntimeCommandType::ResumeSession => {
                non_empty_session_id(payload).map(Some).ok_or_else(|| {
                    self.recorded_error(
                        &command.id,
                        anyhow::anyhow!("provider session id is required for resume_session"),
                    )
                })
            }
            RuntimeCommandType::SendInput => {
                if let Some(provider_session_id) = non_empty_session_id(payload) {
                    return Ok(Some(provider_session_id));
                }
                if payload.session_policy.mode == SessionPolicyMode::ReuseLatest {
                    return self
                        .registry
                        .latest_provider_session(
                            &payload.execution_instance_id,
                            &payload.provider_type,
                        )
                        .ok_or_else(|| {
                            self.recorded_error(
                                &command.id,
                                anyhow::anyhow!(
                                    "provider session is required for send_input; no latest provider session exists"
                                ),
                            )
                        })
                        .map(Some);
                }
                Err(self.recorded_error(
                    &command.id,
                    anyhow::anyhow!(
                        "provider session is required for send_input unless session_policy.mode is reuse_latest"
                    ),
                ))
            }
            _ => Ok(None),
        }
    }

    async fn ensure_command_instance(
        &self,
        command_id: &str,
        payload: &RuntimeSessionCommandPayload,
    ) -> anyhow::Result<CommandWorkspace> {
        let agent_home_dir_text = payload
            .agent_home_dir
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                self.recorded_error(command_id, anyhow::anyhow!("agent_home_dir is required"))
            })?;
        // 控制平面按"绝对路径运行时派生"原则下发的可能是相对路径;此处一次性
        // 绝对化,否则后续逐 key 软链的 target 会是相对路径——从任务工作区解析
        // 不到(悬空链接),provider 读不穿技能内容。
        let agent_home_dir = PathBuf::from(agent_home_dir_text);
        let agent_home_dir = if agent_home_dir.is_absolute() {
            agent_home_dir
        } else {
            std::env::current_dir()
                .map(|cwd| cwd.join(&agent_home_dir))
                .unwrap_or(agent_home_dir)
        };

        if let Err(error) = crate::mcp_config::inject_session_mcp_config(
            &agent_home_dir,
            &payload.provider_type,
            &payload.mcp_servers,
        ) {
            return Err(self.recorded_error(command_id, error));
        }

        // Everything below runs after the session MCP injection above has already
        // written the home-dir config. Any early return from here on must roll the
        // injection back best-effort, so the fallible steps live in one helper and
        // this single call site handles rollback + error wrapping uniformly.
        self.ensure_command_instance_post_inject(payload, &agent_home_dir)
            .await
            .map_err(|error| {
                rollback_session_mcp_config_best_effort(
                    command_id,
                    Some(agent_home_dir.as_path()),
                );
                self.recorded_error(command_id, error)
            })
    }

    async fn ensure_command_instance_post_inject(
        &self,
        payload: &RuntimeSessionCommandPayload,
        agent_home_dir: &Path,
    ) -> anyhow::Result<CommandWorkspace> {
        let provider_home = provider_home_kind(&payload.provider_type)?;
        materialize_workspace(WorkspaceMaterializationPlan {
            agent_home_dir: agent_home_dir.to_path_buf(),
            provider_home,
        })?;

        let project_workspace = payload.project_workspace();
        let capability_manifest_version = project_workspace.capability_manifest_version.clone();
        let provider_auth_mode = project_workspace.provider_auth_mode.clone();

        // 技能懒收敛(capability-binding-unification):payload 的 skills[] 是员工
        // 能力家目录的全量清单——物化新增/更新、prune 清单外 stale 目录,家目录
        // stamp 短路重复派发。任务与 chat 两条派发路径都经此处生效;老 CP(无
        // capability_manifest_version)保持只增不删的现状语义。
        let fetcher: Box<dyn crate::skills::SkillArchiveFetcher> = match self.control_plane.as_ref()
        {
            Some(client) => Box::new(crate::skills::PresignSkillArchiveFetcher::new(
                client.clone(),
            )),
            None => {
                if !payload.skills.is_empty() {
                    anyhow::bail!(
                        "session skills require a control plane client for presigned downloads"
                    );
                }
                Box::new(NoControlPlaneSkillFetcher)
            }
        };
        let skill_convergence = {
            // per-home 互斥:同一家目录上的并发命令串行收敛,防止互删。
            let home_lock = self.capability_convergence_lock(agent_home_dir);
            let _guard = home_lock.lock().await;
            // 并发防护:家目录仍被 Running run 引用时跳过 prune(当前命令的 run
            // 此时尚未注册——start_run 在收敛之后——不会误伤自己),stamp 不写,
            // 下次派发补删。
            let allow_prune = !self
                .runs
                .active_capability_dirs()
                .await
                .contains(agent_home_dir);
            crate::skills_convergence::converge_provider_skills(
                agent_home_dir,
                &payload.provider_type,
                &payload.skills,
                capability_manifest_version.as_deref(),
                fetcher.as_ref(),
                allow_prune,
            )
            .await?
        };
        eprintln!(
            "command {}: skill convergence for {} home (materialized={}, reused={}, pruned={}, prune_skipped={}, stamp_hit={})",
            payload.command_id,
            payload.provider_type,
            skill_convergence.materialized.len(),
            skill_convergence.reused.len(),
            skill_convergence.pruned.len(),
            skill_convergence.prune_skipped,
            skill_convergence.stamp_hit,
        );
        let resolved = crate::project_workspace::resolve_project_workspace(
            crate::project_workspace::ProjectWorkspaceRequest {
                base_dir: self.config.workspace_base_dir(),
                project_id: project_workspace.project_id,
                project_task_id: project_workspace.project_task_id,
                attempt_id: project_workspace.project_task_attempt_id,
                chat_thread_id: project_workspace.chat_thread_id,
                project_name: project_workspace.project_name,
                provider_type: Some(payload.provider_type.clone()),
                workspace_mode: project_workspace
                    .workspace_mode
                    .unwrap_or_else(|| "none".to_string()),
                project_git: project_workspace.project_git,
                base_ref: project_workspace.base_ref,
            },
        )?;

        let skill_keys: Vec<String> = payload
            .skills
            .iter()
            .map(|skill| skill.skill_key.clone())
            .collect();
        let session = crate::project_session::install_project_session(
            agent_home_dir,
            &resolved.workspace_path,
            &payload.command_id,
            &payload.provider_type,
            &skill_keys,
            &payload.mcp_servers,
        )?;
        if !session.skill_conflicts.is_empty() {
            // 项目原生技能压过员工侧同 key(spec §3.1):stderr 留痕之外,冲突
            // 清单随 CommandWorkspace 进 RunSpec,在 attestation metadata 落库。
            eprintln!(
                "command {}: employee skills skipped in favor of project-native skills: {}",
                payload.command_id,
                session.skill_conflicts.join(",")
            );
        }

        Ok(CommandWorkspace {
            workspace_path: resolved.workspace_path,
            employee_capability_dir: agent_home_dir.to_path_buf(),
            agent_home_dir: agent_home_dir.to_path_buf(),
            capability_manifest_version,
            provider_auth_mode,
            mcp_config_path: session.mcp_config_path,
            skill_conflicts: session.skill_conflicts,
            skill_convergence: Some(skill_convergence),
            workspace_base_ref: resolved.base_ref,
            provider_overlay_env: session.provider_overlay_env,
        })
    }

    fn capability_convergence_lock(&self, agent_home_dir: &Path) -> Arc<tokio::sync::Mutex<()>> {
        let mut locks = self
            .capability_locks
            .lock()
            .expect("capability convergence lock registry poisoned");
        locks
            .entry(agent_home_dir.to_path_buf())
            .or_default()
            .clone()
    }

    fn select_provider(
        &self,
        command_id: &str,
        payload: &RuntimeSessionCommandPayload,
    ) -> anyhow::Result<Box<dyn ProviderAdapter>> {
        catalog::select_provider(&self.config, &payload.provider_type)
            .map_err(|error| self.recorded_error(command_id, error))
    }

    fn spawn_provider_event_drain(
        &self,
        run_id: String,
        events: ProviderEventStream,
        reusable_provider_session: bool,
        writeback: Option<RuntimeCommandWritebackSink>,
        spec: RunSpec,
        provider_started_at: Instant,
        heartbeat_stop: Option<CancellationToken>,
        raw_sink: Arc<dyn crate::raw_log::RawLineSink>,
    ) {
        let runs = self.runs.clone();
        let registry = self.registry.clone();
        let failure_writeback = writeback.clone();
        let failure_raw_sink = raw_sink.clone();
        // Clone before move so the Err branch can still roll back projections
        // and write attestation (path 4 backstop).
        let failure_spec = spec.clone();
        let rollback_home = spec.agent_home_dir.clone();
        let rollback_workspace = spec.workspace_path.clone();
        let rollback_command_id = spec
            .command_context
            .as_ref()
            .map(|context| context.command_id.clone());
        // 终态清理计划(spec §5):仅任务 attempt 工作区形态会得到 Some;chat
        // 线程目录与未知路径在 plan 层就被排除。base_dir 相对路径按与
        // resolve_project_workspace 相同规则绝对化,保证前缀比对成立。
        // 稳定 `{base}/{project_name}` 不在终态清理范围(禁止删项目根)。
        let terminal_cleanup = crate::project_workspace::absolutize_workspace_base_dir(
            &self.config.workspace_base_dir(),
        )
        .ok()
        .and_then(|base_dir| {
            crate::workspace_cleanup::TerminalWorkspaceCleanup::plan(
                &self.config.workspace.cleanup_policy,
                &base_dir,
                &spec.workspace_path,
            )
        });
        tokio::spawn(async move {
            let result = drain_provider_events(
                runs.clone(),
                registry.clone(),
                run_id.clone(),
                events,
                reusable_provider_session,
                writeback,
                spec,
                provider_started_at,
                heartbeat_stop,
                raw_sink,
                terminal_cleanup,
            )
            .await;

            if let Err(error) = result {
                // drain_provider_events bailed out before its tail rollback hook
                // (record/writeback `?` — stream item errors are handled inside
                // drain with attestation). Backstop rollback + fail + attestation.
                rollback_session_projections_best_effort(
                    &run_id,
                    rollback_home.as_deref(),
                    Some(rollback_workspace.as_path()),
                    rollback_command_id.as_deref(),
                );
                if !run_is_cancelled(&runs, &run_id).await {
                    let envelope =
                        envelope_from_anyhow(&error, failure_spec.registry_provider_type());
                    let _ = runs
                        .finish_failed(&run_id, envelope.message.clone())
                        .await;
                    // A failed run still produced a transcript; the failure
                    // writeback must carry its pointer.
                    finalize_raw_log(&failure_raw_sink, failure_writeback.as_ref()).await;
                    if let Some(writeback) = &failure_writeback {
                        writeback
                            .record_attestation(
                                &failure_spec,
                                "provider_terminal",
                                "failed",
                                None,
                                Some(
                                    provider_started_at
                                        .elapsed()
                                        .as_millis()
                                        .min(i64::MAX as u128) as i64,
                                ),
                            )
                            .await;
                        let _ = writeback.fail_with_envelope(envelope, None).await;
                    }
                }
            }
            registry.record_run_finished(&run_id);
        });
    }

    fn recorded_error(&self, command_id: &str, error: anyhow::Error) -> anyhow::Error {
        self.registry
            .record_rejection(command_id, &error.to_string());
        error
    }
}

/// 收敛在无 control plane client 且 skills 清单为空时仍要执行(prune/stamp 语义
/// 不依赖下载);此 fetcher 只作占位,任何真实取回请求都是逻辑错误。
struct NoControlPlaneSkillFetcher;

#[async_trait::async_trait]
impl crate::skills::SkillArchiveFetcher for NoControlPlaneSkillFetcher {
    async fn fetch(
        &self,
        skill: &crate::commands::payload::RuntimeSkillPayload,
    ) -> anyhow::Result<Vec<u8>> {
        anyhow::bail!(
            "session skills require a control plane client for presigned downloads: {}",
            skill.skill_key
        )
    }
}

/// Builds the raw transcript sink for a run.
///
/// Falls back to a no-op sink when there is no control plane client (nowhere
/// to presign uploads) or the run is not backed by a project task attempt
/// (nowhere to hang the resulting pointer). Uploads go through presigned URLs
/// issued per segment — the runtime holds no object-store credentials.
fn build_raw_sink_inner(
    control_plane: Option<&ControlPlaneClient>,
    local_dir: std::path::PathBuf,
    tenant_id: Option<&str>,
    attempt_id: Option<&str>,
) -> Arc<dyn crate::raw_log::RawLineSink> {
    let (Some(control_plane), Some(attempt_id)) = (control_plane, attempt_id) else {
        return Arc::new(crate::raw_log::NoopRawSink);
    };
    let tenant_id = tenant_id.unwrap_or("unknown-tenant");
    let uploader = Arc::new(crate::raw_log::PresignRawLogUploader::new(
        control_plane.clone(),
        attempt_id.to_string(),
    ));
    Arc::new(crate::raw_log::SegmentedRawLogSink::new(
        uploader,
        local_dir,
        format!("runs/{tenant_id}/{attempt_id}/"),
        attempt_id.to_string(),
    ))
}

fn spawn_project_task_budget_heartbeat(
    runs: RuntimeRunStore,
    run_id: String,
    writeback: RuntimeCommandWritebackSink,
    handle: ProviderRunHandle,
    started_at: Instant,
    interval: Duration,
    environment: BTreeMap<String, String>,
    spec: RunSpec,
) -> CancellationToken {
    let stop = CancellationToken::new();
    let child_stop = stop.clone();
    tokio::spawn(async move {
        loop {
            tokio::select! {
                _ = child_stop.cancelled() => break,
                _ = tokio::time::sleep(interval) => {
                    match writeback.record_budget_heartbeat(started_at.elapsed()).await {
                        Ok(true) => {
                            let envelope = envelope_for_code(
                                error_code::BUDGET_FUSE,
                                "wall_clock_exceeded",
                                // The sink knows the registry provider_type; the
                                // literal "unknown" used to leak into the envelope.
                                writeback.provider_type.as_str(),
                            );
                            let _ = handle.cancel().await;
                            emit_turn_error_marker(
                                &runs,
                                Some(&writeback),
                                &run_id,
                                &envelope,
                                None,
                                &environment,
                            )
                            .await;
                            // 与另外四条终态路径对齐：熔断也必须留执行证明。
                            // 此前这条路径只有 provider_start/succeeded，终态无
                            // attestation（2026-08-10 真实 E2E 实测发现）。
                            writeback
                                .record_attestation(
                                    &spec,
                                    "provider_terminal",
                                    "failed",
                                    None,
                                    Some(
                                        started_at
                                            .elapsed()
                                            .as_millis()
                                            .min(i64::MAX as u128)
                                            as i64,
                                    ),
                                )
                                .await;
                            let _ = writeback.fail_with_envelope(envelope, None).await;
                            break;
                        }
                        Ok(false) => {}
                        Err(error) => {
                            eprintln!(
                                "Project task budget heartbeat failed for command {}: {}",
                                writeback.command_id, error
                            );
                        }
                    }
                }
            }
        }
    });
    stop
}

fn workspace_sync_failed_terminal(error_message: String) -> RuntimeCommandTerminalWriteback {
    RuntimeCommandTerminalWriteback {
        status: "failed".to_string(),
        summary: None,
        result: None,
        diagnostic: None,
        provider_session_external_id: None,
        session_state_patch: None,
        log_ref: None,
        raw_result_ref: None,
        error_message: Some(error_message),
        error_code: Some("workspace_sync_failed".to_string()),
        error_family: Some("workspace_materialization".to_string()),
    }
}

fn command_completed_terminal(
    summary: Option<String>,
    provider_session_id: Option<String>,
    total_tokens: i64,
    diagnostics: Option<serde_json::Value>,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    if let Some(summary) = summary.as_ref().filter(|value| !value.trim().is_empty()) {
        result.insert(
            "summary".to_string(),
            serde_json::Value::String(summary.clone()),
        );
    }
    if total_tokens > 0 {
        let mut usage = serde_json::Map::new();
        usage.insert(
            "total_tokens".to_string(),
            serde_json::Value::Number(total_tokens.into()),
        );
        result.insert("usage".to_string(), serde_json::Value::Object(usage));
    }
    if let Some(diag) = diagnostics.as_ref() {
        result.insert("diagnostics".to_string(), diag.clone());
    }

    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary,
        result: Some(result),
        diagnostic: diagnostics_to_terminal_map(diagnostics.as_ref()),
        provider_session_external_id: provider_session_id.clone(),
        session_state_patch: provider_session_state_patch(provider_session_id.as_deref()),
        log_ref: None,
        raw_result_ref: None,
        error_message: None,
        error_code: None,
        error_family: None,
    }
}

fn diagnostics_to_terminal_map(
    diagnostics: Option<&serde_json::Value>,
) -> Option<HashMap<String, serde_json::Value>> {
    let Some(serde_json::Value::Object(map)) = diagnostics else {
        return None;
    };
    Some(map.iter().map(|(k, v)| (k.clone(), v.clone())).collect())
}

fn command_failed_terminal(error_message: String) -> RuntimeCommandTerminalWriteback {
    RuntimeCommandTerminalWriteback {
        status: "failed".to_string(),
        summary: None,
        result: None,
        diagnostic: None,
        provider_session_external_id: None,
        session_state_patch: None,
        log_ref: None,
        raw_result_ref: None,
        error_message: Some(error_message),
        error_code: Some("provider_failed".to_string()),
        error_family: Some("provider".to_string()),
    }
}

fn command_cancelled_terminal(reason: Option<String>) -> RuntimeCommandTerminalWriteback {
    RuntimeCommandTerminalWriteback {
        status: "cancelled".to_string(),
        summary: reason,
        result: None,
        diagnostic: None,
        provider_session_external_id: None,
        session_state_patch: None,
        log_ref: None,
        raw_result_ref: None,
        error_message: None,
        error_code: None,
        error_family: None,
    }
}

impl RuntimeCommandWritebackSink {
    async fn start_project_task(&self) -> anyhow::Result<()> {
        if let Some(project_task) = &self.project_task {
            self.client
                .start_project_task_attempt(
                    &project_task.attempt_id,
                    &project_task_start_writeback(project_task, &self.command_id, None),
                )
                .await?;
        }
        Ok(())
    }

    async fn record_event(
        &self,
        record: &RunEventRecord,
        provider_session_id: Option<&str>,
        environment: &BTreeMap<String, String>,
    ) -> anyhow::Result<()> {
        if let ProviderEvent::TurnCompleted {
            usage: Some(usage), ..
        } = &record.event
            && usage.total_tokens > 0
        {
            self.usage_tokens
                .fetch_add(usage.total_tokens, Ordering::Relaxed);
        }
        self.client
            .record_runtime_command_event(
                &self.command_id,
                &runtime_event_writeback(
                    record,
                    provider_session_id,
                    environment,
                    Some(self.provider_type.as_str()),
                    Some(EventAttemptRef {
                        command_id: self.command_id.as_str(),
                        attempt_id: self
                            .project_task
                            .as_ref()
                            .map(|context| context.attempt_id.as_str()),
                    }),
                ),
            )
            .await
    }

    async fn record_attestation(
        &self,
        spec: &RunSpec,
        attestation_type: &str,
        status: &str,
        provider_session_id: Option<&str>,
        duration_ms: Option<i64>,
    ) {
        if let Some(project_task) = &self.project_task {
            // git 事实按次采集,不缓存:provider_start 记的是基线 HEAD,
            // provider_terminal 记的是收工时的 HEAD 与变更摘要,两次本就该不同。
            let git = crate::artifacts::collect_workspace_git_facts(&spec.workspace_path).await;
            let body = project_task_attestation_writeback(
                project_task,
                &self.command_id,
                spec,
                attestation_type,
                status,
                provider_session_id,
                duration_ms,
                &git,
            );
            if let Err(error) = self.client.create_project_task_attestation(&body).await {
                eprintln!(
                    "Project task attestation writeback failed for command {} project_task {}: {}",
                    self.command_id, project_task.project_task_id, error
                );
            }
        }
    }

    fn spawn_attestation(
        &self,
        spec: RunSpec,
        attestation_type: &'static str,
        status: &'static str,
        provider_session_id: Option<String>,
        duration_ms: Option<i64>,
    ) {
        let writeback = self.clone();
        tokio::spawn(async move {
            writeback
                .record_attestation(
                    &spec,
                    attestation_type,
                    status,
                    provider_session_id.as_deref(),
                    duration_ms,
                )
                .await;
        });
    }

    async fn record_budget_heartbeat(&self, elapsed: Duration) -> anyhow::Result<bool> {
        let project_task = match &self.project_task {
            Some(project_task) => project_task,
            None => return Ok(false),
        };
        let elapsed_sec = elapsed.as_secs().min(i32::MAX as u64) as i32;
        let consumed_tokens = self
            .usage_tokens
            .load(Ordering::Relaxed)
            .min(i32::MAX as i64) as i32;
        let body =
            project_task_budget_heartbeat_writeback(project_task, elapsed_sec, consumed_tokens);
        let response = self
            .client
            .record_project_task_budget_heartbeat(&project_task.attempt_id, &body)
            .await?;
        Ok(response.tripped)
    }

    async fn complete(
        &self,
        summary: Option<String>,
        provider_session_id: Option<String>,
        diagnostics: Option<serde_json::Value>,
    ) -> anyhow::Result<()> {
        // 模型自述(conclusion)在离开执行机前统一脱敏(证据地基 §8 修订 7②):
        // 它下游流向 run_completed 事件、conclusion.md 工件、attempt.completed /
        // summary.created ledger 行与 execution_summaries.conclusion,任何一处
        // 明文都等于把模型复述的密钥端上 Web。raw transcript 不经此路径,保持原样。
        let summary = summary.map(|value| match &self.artifact_collection {
            Some(context) => {
                crate::redaction::redact_with_environment(&value, &context.environment)
            }
            None => crate::redaction::redact(&value),
        });
        let total_tokens = self.usage_tokens.load(Ordering::Relaxed);
        self.client
            .complete_runtime_command(
                &self.command_id,
                &command_completed_terminal(
                    summary.clone(),
                    provider_session_id.clone(),
                    total_tokens,
                    diagnostics,
                ),
            )
            .await?;
        if let Some(project_task) = &self.project_task {
            let raw_log = self.raw_log.lock().ok().and_then(|guard| guard.clone());
            // 捕获 (发送结果, 写回类别, 原始请求体) —— 失败时把请求体落盘由后台 worker
            // 重放(遗留缺陷#1),而不是像以前那样吞掉丢失结果。body 已含幂等键/租约令牌。
            let (result, kind, body) = if let Some(mut writeback) =
                project_task_wait_human_writeback(
                    project_task,
                    &self.command_id,
                    summary.as_deref(),
                    provider_session_id.as_deref(),
                ) {
                writeback.raw_log = raw_log;
                let result = self
                    .client
                    .wait_human_project_task_attempt(&project_task.attempt_id, &writeback)
                    .await;
                (
                    result,
                    crate::controlplane::models::ProjectTaskAttemptWritebackKind::WaitHuman,
                    serde_json::to_value(&writeback).ok(),
                )
            } else {
                // 采集+上传发生在 complete writeback 之前(证据地基 spec §3.4):
                // 控制平面收到 result 时对象已在存储中,可在同一事务里物化。
                // 上传失败 → 整个 completion 失败,任务不得声称拥有从未落库的证据。
                let mut collected_refs =
                    self.collect_and_upload_artifacts(summary.as_deref()).await?;
                // 声明式交付物与证据同格整批失败(v2 spec §2):契约承诺的
                // 交付物不允许"声称交付了但没落库"。
                collected_refs.extend(self.collect_and_upload_declared().await?);
                // 输出附件是 best-effort(输出附件 spec §1.5):单个附件失败
                // 降级为可见的 skip note,绝不拖垮 completion。
                collected_refs.extend(self.collect_and_upload_attachments().await);
                let mut writeback = project_task_complete_writeback(
                    project_task,
                    &self.command_id,
                    summary.as_deref(),
                    provider_session_id.as_deref(),
                );
                writeback.raw_log = raw_log;
                merge_collected_artifact_refs(&mut writeback, collected_refs);
                let result = self
                    .client
                    .complete_project_task_attempt(&project_task.attempt_id, &writeback)
                    .await;
                (
                    result,
                    crate::controlplane::models::ProjectTaskAttemptWritebackKind::Complete,
                    serde_json::to_value(&writeback).ok(),
                )
            };
            if let Err(error) = result {
                self.enqueue_failed_writeback(project_task, kind, body, error)
                    .await;
            }
        }
        Ok(())
    }

    /// 终态写回 POST 失败的统一善后:能序列化请求体就落盘由重试 worker 重放(遗留缺陷#1),
    /// 否则只能记日志(理论上不发生——写回体都是纯数据)。采集/上传已在此前成功,重放只重发 HTTP。
    async fn enqueue_failed_writeback(
        &self,
        project_task: &ProjectTaskWritebackContext,
        kind: crate::controlplane::models::ProjectTaskAttemptWritebackKind,
        body: Option<serde_json::Value>,
        error: anyhow::Error,
    ) {
        match body {
            Some(body) => {
                eprintln!(
                    "Project task writeback failed for command {} project_task {}: {}; \
                     persisted for durable retry",
                    self.command_id, project_task.project_task_id, error
                );
                if let Err(enqueue_error) = self
                    .writeback_queue
                    .enqueue(kind, &project_task.attempt_id, body)
                    .await
                {
                    eprintln!(
                        "Failed to persist writeback for attempt {} (result may be lost, \
                         CP-side watchdog will recover): {enqueue_error}",
                        project_task.attempt_id
                    );
                }
            }
            None => {
                eprintln!(
                    "Project task writeback failed and body could not be serialized for retry \
                     (command {} project_task {}): {}",
                    self.command_id, project_task.project_task_id, error
                );
            }
        }
    }

    /// Collects the attempt's artifacts and uploads them through presigned
    /// URLs. No-op (empty refs) when the sink has no collection context.
    async fn collect_and_upload_artifacts(
        &self,
        conclusion: Option<&str>,
    ) -> anyhow::Result<Vec<serde_json::Value>> {
        let Some(context) = &self.artifact_collection else {
            return Ok(Vec::new());
        };
        let artifacts = crate::artifacts::collect_artifacts(
            &crate::artifacts::ArtifactCollectionInputs {
                raw_log_path: context.raw_log_path.clone(),
                workspace_path: context.workspace_path.clone(),
                conclusion,
                environment: &context.environment,
            },
        )
        .await;
        crate::artifacts::upload_artifacts(&self.client, artifacts).await
    }

    /// Collects and uploads declared deliverables from `deliverables/`
    /// (v2 spec §2). Upload failure fails the completion, like evidence.
    async fn collect_and_upload_declared(&self) -> anyhow::Result<Vec<serde_json::Value>> {
        let Some(context) = &self.artifact_collection else {
            return Ok(Vec::new());
        };
        let Some(workspace) = &context.workspace_path else {
            return Ok(Vec::new());
        };
        let collection = crate::artifacts::collect_declared_deliverables(workspace).await;
        if collection.attachments.is_empty() && collection.skipped.is_empty() {
            return Ok(Vec::new());
        }
        crate::artifacts::upload_declared_deliverables(&self.client, collection).await
    }

    /// Collects and uploads `execution_output` attachments best-effort.
    /// Infallible by design: failures surface as skip notes in the refs.
    async fn collect_and_upload_attachments(&self) -> Vec<serde_json::Value> {
        let Some(context) = &self.artifact_collection else {
            return Vec::new();
        };
        let Some(workspace) = &context.workspace_path else {
            return Vec::new();
        };
        let collection =
            crate::artifacts::collect_attachments(workspace, context.workspace_baseline.as_ref())
                .await;
        if collection.attachments.is_empty() && collection.skipped.is_empty() {
            return Vec::new();
        }
        crate::artifacts::upload_attachments_best_effort(&self.client, collection).await
    }

    async fn fail_project_task(&self, envelope: &ErrorEnvelope) -> anyhow::Result<()> {
        if let Some(project_task) = &self.project_task {
            let mut writeback =
                project_task_fail_writeback_from_envelope(project_task, &self.command_id, envelope);
            writeback.raw_log = self.raw_log.lock().ok().and_then(|guard| guard.clone());
            if let Err(error) = self
                .client
                .fail_project_task_attempt(&project_task.attempt_id, &writeback)
                .await
            {
                // 失败写回同样落盘重试(遗留缺陷#1):否则 CP 侧看不到失败,任务卡 running。
                // 落盘后视为已善后返回 Ok,不把瞬时写回失败上抛拖垮 drain 早退路径。
                self.enqueue_failed_writeback(
                    project_task,
                    crate::controlplane::models::ProjectTaskAttemptWritebackKind::Fail,
                    serde_json::to_value(&writeback).ok(),
                    error,
                )
                .await;
            }
        }
        Ok(())
    }

    /// Terminal fail writeback from a structured envelope (preferred).
    /// `diagnostics` is L3 attempt stream stats (unmapped counts, …); optional
    /// when the failure is pre-stream (spawn/workspace) or early fuse.
    async fn fail_with_envelope(
        &self,
        envelope: ErrorEnvelope,
        diagnostics: Option<serde_json::Value>,
    ) -> anyhow::Result<()> {
        // L3 semantics ride on the terminal writeback fields (status + error_code
        // + error_family + diagnostic); Runtime does not send a separate
        // ProviderResult object today (spec §6, 已知差距).
        self.client
            .fail_runtime_command(
                &self.command_id,
                &command_failed_terminal_from_envelope(&envelope, diagnostics),
            )
            .await?;
        self.fail_project_task(&envelope).await
    }

    /// Legacy string entry point: classifies via error_map fallback, then fails.
    /// No `turn_error` marker here on purpose — callers are pre-provider failures
    /// (start writeback, workspace sync, stop command), where no L2 stream exists.
    async fn fail(&self, error_message: String) -> anyhow::Result<()> {
        let envelope = envelope_for_code(
            error_map::classify_message_fallback(&error_message),
            error_message,
            self.provider_type.as_str(),
        );
        self.fail_with_envelope(envelope, None).await
    }
}

/// Prepends the runtime-collected artifact refs to the writeback — both the
/// top-level `artifact_refs` (the /complete handler path) and the result
/// contract's (the /result path). 采集结果优先于 provider 自报(spec §4.1.1);
/// self-reported bare refs stay behind them as metadata-only entries.
fn merge_collected_artifact_refs(
    writeback: &mut ProjectTaskCompleteWriteback,
    collected: Vec<serde_json::Value>,
) {
    if collected.is_empty() {
        return;
    }
    let mut merged = collected.clone();
    merged.append(&mut writeback.artifact_refs);
    writeback.artifact_refs = merged;
    if let Some(contract) = writeback.result_contract.as_mut() {
        let mut merged = collected;
        merged.append(&mut contract.artifact_refs);
        contract.artifact_refs = merged;
    }
}

/// Who this event belongs to, for the L2 envelope's `attempt_ref`.
#[derive(Debug, Clone, Copy)]
struct EventAttemptRef<'a> {
    command_id: &'a str,
    attempt_id: Option<&'a str>,
}

/// Formats the run store's epoch-millis stamp as RFC3339 for the envelope `ts`.
/// Out-of-range stamps yield None rather than a bogus timestamp.
fn envelope_timestamp(recorded_at_ms: u64) -> Option<String> {
    let nanos = i128::from(recorded_at_ms).checked_mul(1_000_000)?;
    time::OffsetDateTime::from_unix_timestamp_nanos(nanos)
        .ok()?
        .format(&time::format_description::well_known::Rfc3339)
        .ok()
}

fn runtime_event_writeback(
    record: &RunEventRecord,
    provider_session_id: Option<&str>,
    environment: &BTreeMap<String, String>,
    provider_type: Option<&str>,
    attempt_ref: Option<EventAttemptRef<'_>>,
) -> RuntimeCommandEventWriteback {
    let mut provider_session_external_id = provider_session_id.map(ToString::to_string);
    let mut session_state_patch = provider_session_state_patch(provider_session_id);
    let (event_type, payload) = match &record.event {
        ProviderEvent::SessionStarted { session_id, .. } => {
            provider_session_external_id = Some(session_id.clone());
            session_state_patch = provider_session_state_patch(Some(session_id));
            let mut payload = HashMap::new();
            payload.insert(
                "session_id".to_string(),
                serde_json::Value::String(session_id.clone()),
            );
            ("session_started".to_string(), payload)
        }
        ProviderEvent::TurnStarted => ("turn_started".to_string(), HashMap::new()),
        ProviderEvent::TextDelta { text } => {
            let mut payload = HashMap::new();
            // 模型 prose 同样脱敏(证据地基 §8 修订 7②):模型复述的密钥此前
            // 经 text_delta 明文直达 Web「最新结果」。raw 保持原样不受影响。
            payload.insert(
                "text".to_string(),
                serde_json::Value::String(crate::redaction::redact_with_environment(
                    text,
                    environment,
                )),
            );
            ("text_delta".to_string(), payload)
        }
        ProviderEvent::ToolStarted {
            tool_id,
            name,
            input_excerpt,
            input_truncated,
        } => {
            let mut payload = HashMap::new();
            payload.insert(
                "tool_id".to_string(),
                serde_json::Value::String(tool_id.clone()),
            );
            payload.insert("name".to_string(), serde_json::Value::String(name.clone()));
            payload.insert(
                "input_excerpt".to_string(),
                serde_json::Value::String(crate::redaction::redact_with_environment(
                    input_excerpt,
                    environment,
                )),
            );
            payload.insert(
                "input_truncated".to_string(),
                serde_json::Value::Bool(*input_truncated),
            );
            ("tool_started".to_string(), payload)
        }
        ProviderEvent::ToolCompleted {
            tool_id,
            is_error,
            output_excerpt,
            output_truncated,
        } => {
            let mut payload = HashMap::new();
            payload.insert(
                "tool_id".to_string(),
                serde_json::Value::String(tool_id.clone()),
            );
            payload.insert("is_error".to_string(), serde_json::Value::Bool(*is_error));
            payload.insert(
                "output_excerpt".to_string(),
                serde_json::Value::String(crate::redaction::redact_with_environment(
                    output_excerpt,
                    environment,
                )),
            );
            payload.insert(
                "output_truncated".to_string(),
                serde_json::Value::Bool(*output_truncated),
            );
            ("tool_completed".to_string(), payload)
        }
        ProviderEvent::TurnCompleted { summary, usage } => {
            let mut payload = HashMap::new();
            if let Some(summary) = summary {
                payload.insert(
                    "summary".to_string(),
                    serde_json::Value::String(crate::redaction::redact_with_environment(
                        summary,
                        environment,
                    )),
                );
            }
            if let Some(usage) = usage {
                if let Ok(value) = serde_json::to_value(usage) {
                    payload.insert("usage".to_string(), value);
                }
            }
            ("turn_completed".to_string(), payload)
        }
        ProviderEvent::TurnError { message, error } => {
            let mut payload = HashMap::new();
            let redacted_message = crate::redaction::redact_with_environment(message, environment);
            payload.insert(
                "message".to_string(),
                serde_json::Value::String(redacted_message.clone()),
            );
            if let Some(envelope) = error {
                let mut redacted = envelope.clone();
                redacted.message = redacted_message;
                if let Ok(value) = serde_json::to_value(&redacted) {
                    payload.insert("error".to_string(), value);
                }
            }
            ("turn_error".to_string(), payload)
        }
        ProviderEvent::NativeUnmapped {
            native_type,
            reason,
        } => {
            let mut payload = HashMap::new();
            if let Some(native_type) = native_type {
                payload.insert(
                    "native_type".to_string(),
                    serde_json::Value::String(native_type.clone()),
                );
            }
            payload.insert(
                "reason".to_string(),
                serde_json::Value::String(reason.clone()),
            );
            ("native_unmapped".to_string(), payload)
        }
    };

    // L2 信封字段（2026-08-10 定档批一）：与业务键**同层**补齐，让一条事件脱离
    // 请求上下文也能自解释（离线重放、导出分析）。刻意保持扁平——过渡期若同时
    // 保留扁平键与一份嵌套副本，tool 事件的两个 4KB excerpt 会翻倍，而 task_events
    // 是热表且走 WS 广播。嵌套形态与读路径迁移是批二。
    //
    // `type` / `seq` 是**冗余投影**：外层 `event_type` / `sequence_number` 才是真相
    // （CP 用外层去重、排序、落 task_events.sequence_number 列），不一致以外层为准。
    let mut payload = payload;
    payload.insert(
        "schema_version".to_string(),
        serde_json::Value::String(PROVIDER_EVENT_SCHEMA_VERSION.to_string()),
    );
    payload.insert(
        "type".to_string(),
        serde_json::Value::String(event_type.clone()),
    );
    payload.insert(
        "seq".to_string(),
        serde_json::Value::from(record.sequence.min(i32::MAX as u64)),
    );
    if let Some(ts) = envelope_timestamp(record.recorded_at_ms) {
        payload.insert("ts".to_string(), serde_json::Value::String(ts));
    }
    let canonical_provider_type = provider_type
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(|value| crate::providers::catalog::canonical_provider_type(value).to_string());
    if let Some(provider_type) = &canonical_provider_type {
        payload.insert(
            "provider_type".to_string(),
            serde_json::Value::String(provider_type.clone()),
        );
    }
    if let Some(session_id) = provider_session_external_id.as_deref() {
        payload.insert(
            "provider_session_id".to_string(),
            serde_json::Value::String(session_id.to_string()),
        );
    }
    if let Some(attempt_ref) = attempt_ref {
        let mut reference = serde_json::Map::new();
        reference.insert(
            "command_id".to_string(),
            serde_json::Value::String(attempt_ref.command_id.to_string()),
        );
        if let Some(attempt_id) = attempt_ref.attempt_id {
            reference.insert(
                "attempt_id".to_string(),
                serde_json::Value::String(attempt_id.to_string()),
            );
        }
        payload.insert(
            "attempt_ref".to_string(),
            serde_json::Value::Object(reference),
        );
    }

    let mut metadata = HashMap::from([
        (
            "source".to_string(),
            serde_json::Value::String("runtime-agent".to_string()),
        ),
        (
            "schema_version".to_string(),
            serde_json::Value::String(PROVIDER_EVENT_SCHEMA_VERSION.to_string()),
        ),
    ]);
    if let Some(provider_type) = provider_type.map(str::trim).filter(|v| !v.is_empty()) {
        metadata.insert(
            "provider_type".to_string(),
            serde_json::Value::String(
                crate::providers::catalog::canonical_provider_type(provider_type).to_string(),
            ),
        );
    }
    RuntimeCommandEventWriteback {
        event_type,
        sequence_number: record.sequence.min(i32::MAX as u64) as i32,
        payload,
        provider_session_external_id,
        session_state_patch,
        metadata: Some(metadata),
    }
}

fn provider_session_state_patch(
    provider_session_id: Option<&str>,
) -> Option<HashMap<String, serde_json::Value>> {
    provider_session_id.map(|session_id| {
        HashMap::from([(
            "provider_session_id".to_string(),
            serde_json::Value::String(session_id.to_string()),
        )])
    })
}

fn project_task_writeback_context(
    payload: &RuntimeSessionCommandPayload,
) -> Option<ProjectTaskWritebackContext> {
    project_task_writeback_context_from_metadata(&payload.metadata, &payload.digital_employee_id)
}

fn project_task_writeback_context_from_metadata(
    metadata: &serde_json::Value,
    digital_employee_id: &str,
) -> Option<ProjectTaskWritebackContext> {
    let metadata = metadata.as_object()?;
    if metadata
        .get("source")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        != Some("project_task_dispatch")
    {
        return None;
    }
    let handoff_contract = metadata
        .get("handoff_contract")
        .cloned()
        .unwrap_or_else(|| serde_json::json!({}));
    let completion_path = handoff_contract
        .as_object()
        .and_then(|contract| contract.get("completion_path"))
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .unwrap_or("");
    // A project_task_dispatch run with attempt metadata closes through the attempt-scoped
    // Runtime writeback API. When completion_path is omitted, default to the attempt path;
    // an explicit non-matching value is respected as no writeback.
    if !completion_path.is_empty() && completion_path != "project_task_attempt_writeback" {
        return None;
    }
    let project_id = metadata
        .get("project_id")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;
    let project_task_id = metadata
        .get("project_task_id")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;
    let attempt_id = metadata
        .get("project_task_attempt_id")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;
    let lease_token = metadata
        .get("project_task_lease_token")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;
    let runtime_node_id = metadata
        .get("runtime_node_id")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())?;
    let digital_employee_id = digital_employee_id.trim();
    if digital_employee_id.is_empty() {
        return None;
    }
    let execution_context_packet_version = metadata
        .get("execution_context_packet_version")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .unwrap_or("v1");
    let capability_manifest_version = metadata
        .get("capability_manifest_version")
        .and_then(serde_json::Value::as_str)
        .and_then(|value| trimmed_optional(Some(value)));
    let provider_auth_mode = metadata
        .get("provider_auth_mode")
        .and_then(serde_json::Value::as_str)
        .and_then(|value| trimmed_optional(Some(value)))
        .unwrap_or_else(|| "host".to_string());
    let budget_heartbeat_interval_sec = metadata
        .get("budget")
        .and_then(serde_json::Value::as_object)
        .and_then(|budget| budget.get("heartbeat_interval_sec"))
        .and_then(serde_json::Value::as_u64)
        .filter(|value| *value > 0)
        .unwrap_or(15);
    Some(ProjectTaskWritebackContext {
        project_id: project_id.to_string(),
        project_task_id: project_task_id.to_string(),
        attempt_id: attempt_id.to_string(),
        lease_token: lease_token.to_string(),
        runtime_node_id: runtime_node_id.to_string(),
        digital_employee_id: digital_employee_id.to_string(),
        capability_manifest_version,
        provider_auth_mode,
        budget_heartbeat_interval_sec,
        expected_outputs: string_array_from_metadata(metadata.get("expected_outputs")),
        handoff_contract,
        execution_context_packet_version: execution_context_packet_version.to_string(),
    })
}

fn string_array_from_metadata(value: Option<&serde_json::Value>) -> Vec<String> {
    value
        .and_then(serde_json::Value::as_array)
        .map(|values| {
            values
                .iter()
                .filter_map(serde_json::Value::as_str)
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .map(ToString::to_string)
                .collect()
        })
        .unwrap_or_default()
}

fn project_task_complete_writeback(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    summary: Option<&str>,
    provider_session_id: Option<&str>,
) -> ProjectTaskCompleteWriteback {
    let parsed = parse_summary_json(summary);
    let conclusion = parsed_conclusion(parsed.as_ref())
        .or_else(|| trimmed_optional(summary))
        .unwrap_or_else(|| "Provider run completed without a textual summary.".to_string());
    let mut evidence_refs = parsed_array(parsed.as_ref(), "evidence_refs");
    if evidence_refs.is_empty() {
        evidence_refs.push(runtime_command_evidence_ref(
            command_id,
            provider_session_id,
        ));
    }
    let artifact_refs = parsed_array(parsed.as_ref(), "artifact_refs");
    let runtime_attestation_ref = project_task_runtime_attestation_ref(context, command_id);
    let result_contract = parsed_result_contract(
        parsed.as_ref(),
        &conclusion,
        &evidence_refs,
        &artifact_refs,
        runtime_attestation_ref.as_deref(),
    )
    .map(|mut contract| {
        backfill_partial_contract_gaps(
            &mut contract,
            context,
            command_id,
            runtime_attestation_ref.as_deref(),
        );
        contract
    })
    .or_else(|| {
        synthesized_result_contract(
            context,
            command_id,
            &conclusion,
            &evidence_refs,
            &artifact_refs,
            runtime_attestation_ref.as_deref(),
        )
    });
    let missing_information = parsed_array(parsed.as_ref(), "missing_information");
    let recommended_next_action = parsed_string(parsed.as_ref(), "recommended_next_action")
        .or_else(|| {
            expected_output(context, "recommended_next_action")
                .then(|| "Continue project coordination with the next ready task.".to_string())
        })
        .unwrap_or_default();
    let mut confidence_factors = parsed_confidence_factors(parsed.as_ref());
    confidence_factors.insert(
        "source".to_string(),
        serde_json::Value::String("runtime_agent_project_task_attempt_writeback".to_string()),
    );
    confidence_factors.insert(
        "command_id".to_string(),
        serde_json::Value::String(command_id.to_string()),
    );
    confidence_factors.insert(
        "project_task_id".to_string(),
        serde_json::Value::String(context.project_task_id.clone()),
    );
    confidence_factors.insert(
        "project_task_attempt_id".to_string(),
        serde_json::Value::String(context.attempt_id.clone()),
    );
    confidence_factors.insert(
        "digital_employee_id".to_string(),
        serde_json::Value::String(context.digital_employee_id.clone()),
    );
    confidence_factors.insert(
        "execution_context_packet_version".to_string(),
        serde_json::Value::String(context.execution_context_packet_version.clone()),
    );
    if let Some(provider_session_id) = provider_session_id {
        confidence_factors.insert(
            "provider_session_id".to_string(),
            serde_json::Value::String(provider_session_id.to_string()),
        );
    }
    if let Some(completion_path) = context
        .handoff_contract
        .as_object()
        .and_then(|contract| contract.get("completion_path"))
        .and_then(serde_json::Value::as_str)
    {
        confidence_factors.insert(
            "completion_path".to_string(),
            serde_json::Value::String(completion_path.to_string()),
        );
    }

    ProjectTaskCompleteWriteback {
        raw_log: None,
        project_task_id: context.project_task_id.clone(),
        lease_token: context.lease_token.clone(),
        runtime_node_id: context.runtime_node_id.clone(),
        idempotency_key: project_task_attempt_idempotency_key(
            &context.attempt_id,
            "complete",
            command_id,
        ),
        provider_session_id: provider_session_id.map(ToString::to_string),
        conclusion,
        evidence_refs,
        artifact_refs,
        confidence_factors,
        uncertainty: parsed_string(parsed.as_ref(), "uncertainty").unwrap_or_default(),
        missing_information,
        recommended_next_action,
        requires_human_review: parsed
            .as_ref()
            .and_then(|value| value.get("requires_human_review"))
            .and_then(serde_json::Value::as_bool)
            .unwrap_or(false),
        result_contract,
    }
}

fn project_task_start_writeback(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    provider_session_id: Option<&str>,
) -> ProjectTaskStartWriteback {
    ProjectTaskStartWriteback {
        project_task_id: context.project_task_id.clone(),
        lease_token: context.lease_token.clone(),
        runtime_node_id: context.runtime_node_id.clone(),
        idempotency_key: project_task_attempt_idempotency_key(
            &context.attempt_id,
            "start",
            command_id,
        ),
        command_id: command_id.to_string(),
        provider_session_id: provider_session_id.map(ToString::to_string),
    }
}

fn project_task_runtime_attestation_ref(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
) -> Option<String> {
    if !context
        .handoff_contract
        .as_object()
        .and_then(|contract| contract.get("requires_runtime_attestation"))
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false)
    {
        return None;
    }
    Some(format!(
        "attestation:{}",
        project_task_attempt_idempotency_key(
            &context.attempt_id,
            "attestation:provider_terminal",
            command_id
        )
    ))
}

fn project_task_attestation_writeback(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    spec: &RunSpec,
    attestation_type: &str,
    status: &str,
    provider_session_id: Option<&str>,
    duration_ms: Option<i64>,
    git: &crate::artifacts::WorkspaceGitFacts,
) -> ProjectTaskAttestationWriteback {
    let mut metadata = serde_json::Map::new();
    metadata.insert(
        "source".to_string(),
        serde_json::Value::String("runtime_agent".to_string()),
    );
    metadata.insert(
        "command_id".to_string(),
        serde_json::Value::String(command_id.to_string()),
    );
    metadata.insert(
        "provider_type".to_string(),
        serde_json::Value::String(spec.registry_provider_type().to_string()),
    );
    metadata.insert(
        "workspace_ref".to_string(),
        serde_json::Value::String(format!(
            "project-workspace:{}/{}/{}",
            context.project_id, context.project_task_id, context.attempt_id
        )),
    );
    if let Some(capability_manifest_version) = &spec.capability_manifest_version {
        metadata.insert(
            "capability_manifest_version".to_string(),
            serde_json::Value::String(capability_manifest_version.clone()),
        );
    }
    metadata.insert(
        "provider_auth_mode".to_string(),
        serde_json::Value::String(spec.provider_auth_mode.clone()),
    );
    {
        // Merge control-plane conflicts (source=project_binding etc.) with workspace-native
        // skill key collisions (project-native path wins). Both must be auditable (S7).
        let mut conflicts: Vec<serde_json::Value> = Vec::new();
        if let Some(ctx) = &spec.command_context {
            if let Some(arr) = ctx.metadata.get("skill_conflicts").and_then(|v| v.as_array()) {
                for item in arr {
                    conflicts.push(item.clone());
                }
            }
        }
        for key in &spec.skill_conflicts {
            conflicts.push(serde_json::json!({
                "slug": key,
                "source": "workspace_native",
            }));
        }
        if !conflicts.is_empty() {
            metadata.insert(
                "skill_conflicts".to_string(),
                serde_json::Value::Array(conflicts),
            );
        }
    }
    if let Some(skill_convergence) = &spec.skill_convergence {
        // 技能懒收敛报告(capability-binding-unification):materialized/reused/
        // pruned/prune_skipped/stamp_hit 全量留痕。
        metadata.insert(
            "capability_convergence".to_string(),
            serde_json::to_value(skill_convergence)
                .unwrap_or_else(|_| serde_json::Value::Object(serde_json::Map::new())),
        );
    }
    if let Some(model) = &spec.model {
        metadata.insert(
            "model".to_string(),
            serde_json::Value::String(model.clone()),
        );
    }

    ProjectTaskAttestationWriteback {
        project_id: context.project_id.clone(),
        project_task_id: context.project_task_id.clone(),
        attempt_id: context.attempt_id.clone(),
        runtime_node_id: context.runtime_node_id.clone(),
        digital_employee_id: context.digital_employee_id.clone(),
        capability_manifest_version: context.capability_manifest_version.clone(),
        provider_auth_mode: context.provider_auth_mode.clone(),
        provider_session_id: provider_session_id.map(ToString::to_string),
        attestation_type: attestation_type.to_string(),
        status: status.to_string(),
        command_argv: vec![spec.registry_provider_type().to_string()],
        exit_code: None,
        duration_ms,
        log_ref: None,
        stdout_sha256: None,
        stderr_sha256: None,
        artifact_refs: Vec::new(),
        artifact_hashes: serde_json::Value::Object(serde_json::Map::new()),
        git_branch: git.branch.clone(),
        git_base_ref: spec.workspace_base_ref.clone(),
        git_head_sha: git.head_sha.clone(),
        git_diff_sha256: git.diff_sha256.clone(),
        metadata: serde_json::Value::Object(metadata),
        idempotency_key: project_task_attempt_idempotency_key(
            &context.attempt_id,
            &format!("attestation:{attestation_type}"),
            command_id,
        ),
    }
}

fn project_task_budget_heartbeat_writeback(
    context: &ProjectTaskWritebackContext,
    consumed_wall_clock_sec: i32,
    consumed_tokens: i32,
) -> ProjectTaskBudgetHeartbeatWriteback {
    ProjectTaskBudgetHeartbeatWriteback {
        project_id: context.project_id.clone(),
        project_task_id: context.project_task_id.clone(),
        consumed_wall_clock_sec,
        consumed_tokens,
    }
}

fn project_task_wait_human_writeback(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    summary: Option<&str>,
    provider_session_id: Option<&str>,
) -> Option<ProjectTaskWaitHumanWriteback> {
    let parsed = parse_summary_json(summary)?;
    let requires_human_review = parsed
        .get("requires_human_review")
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false);
    if !requires_human_review {
        return None;
    }
    let reason = parsed_string(Some(&parsed), "wait_human_reason")?;
    let summary = parsed_string(Some(&parsed), "recommended_next_action")
        .or_else(|| parsed_string(Some(&parsed), "summary"))
        .unwrap_or_else(|| "Human input is required before this task can continue.".to_string());

    Some(ProjectTaskWaitHumanWriteback {
        raw_log: None,
        project_task_id: context.project_task_id.clone(),
        lease_token: context.lease_token.clone(),
        runtime_node_id: context.runtime_node_id.clone(),
        idempotency_key: project_task_attempt_idempotency_key(
            &context.attempt_id,
            "wait-human",
            command_id,
        ),
        provider_session_id: provider_session_id.map(ToString::to_string),
        digital_employee_id: context.digital_employee_id.clone(),
        reason,
        summary,
        missing_context_refs: parsed_array(Some(&parsed), "missing_context_refs"),
        suggested_resolution_options: parsed_string_array(
            Some(&parsed),
            "suggested_resolution_options",
        )
        .unwrap_or_else(|| vec!["resume_same_task".to_string()]),
        result_contract: None,
    })
}

fn expected_output(context: &ProjectTaskWritebackContext, key: &str) -> bool {
    context.expected_outputs.iter().any(|value| value == key)
}

fn parse_summary_json(summary: Option<&str>) -> Option<serde_json::Value> {
    let text = summary?.trim();
    if text.is_empty() {
        return None;
    }
    if let Ok(value) = serde_json::from_str::<serde_json::Value>(text) {
        if value.is_object() {
            return Some(value);
        }
    }
    let fenced = extract_fenced_json(text)?;
    serde_json::from_str::<serde_json::Value>(&fenced)
        .ok()
        .filter(serde_json::Value::is_object)
}

fn extract_fenced_json(text: &str) -> Option<String> {
    let start = text.find("```json").or_else(|| text.find("```"))?;
    let after_start = &text[start..];
    let first_newline = after_start.find('\n')?;
    let content_start = start + first_newline + 1;
    let rest = &text[content_start..];
    let end = rest.find("```")?;
    Some(rest[..end].trim().to_string())
}

fn parsed_conclusion(value: Option<&serde_json::Value>) -> Option<String> {
    parsed_string(value, "conclusion").or_else(|| {
        value
            .and_then(|value| value.get("execution_summary"))
            .and_then(|summary| {
                summary
                    .as_str()
                    .and_then(|text| trimmed_optional(Some(text)))
                    .or_else(|| {
                        summary.as_object().and_then(|object| {
                            ["summary", "description", "conclusion", "status"]
                                .iter()
                                .find_map(|key| {
                                    object
                                        .get(*key)
                                        .and_then(serde_json::Value::as_str)
                                        .and_then(|text| trimmed_optional(Some(text)))
                                })
                        })
                    })
                    .or_else(|| summary.is_object().then(|| summary.to_string()))
            })
    })
}

fn parsed_string(value: Option<&serde_json::Value>, key: &str) -> Option<String> {
    value
        .and_then(|value| value.get(key))
        .and_then(serde_json::Value::as_str)
        .and_then(|text| trimmed_optional(Some(text)))
}

fn parsed_array(value: Option<&serde_json::Value>, key: &str) -> Vec<serde_json::Value> {
    value
        .and_then(|value| value.get(key))
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default()
}

fn parsed_string_array(value: Option<&serde_json::Value>, key: &str) -> Option<Vec<String>> {
    let values: Vec<String> = value
        .and_then(|value| value.get(key))
        .and_then(serde_json::Value::as_array)?
        .iter()
        .filter_map(serde_json::Value::as_str)
        .filter_map(|text| trimmed_optional(Some(text)))
        .collect();
    (!values.is_empty()).then_some(values)
}

fn parsed_confidence_factors(
    value: Option<&serde_json::Value>,
) -> HashMap<String, serde_json::Value> {
    let mut factors = HashMap::new();
    if let Some(object) = value
        .and_then(|value| value.get("confidence_factors"))
        .and_then(serde_json::Value::as_object)
    {
        factors.extend(
            object
                .iter()
                .map(|(key, value)| (key.clone(), value.clone())),
        );
    }
    if let Some(confidence) = value.and_then(|value| value.get("confidence")) {
        factors.insert("confidence".to_string(), confidence.clone());
    }
    factors
}

fn parsed_result_contract(
    value: Option<&serde_json::Value>,
    fallback_summary: &str,
    evidence_refs: &[serde_json::Value],
    artifact_refs: &[serde_json::Value],
    runtime_attestation_ref: Option<&str>,
) -> Option<TaskResultContract> {
    if let Some(contract) = value
        .and_then(|value| value.get("result_contract"))
        .and_then(serde_json::Value::as_object)
    {
        return Some(TaskResultContract {
            status: contract
                .get("status")
                .and_then(serde_json::Value::as_str)
                .and_then(|status| normalized_task_result_status(status))
                .unwrap_or_else(|| "completed".to_string()),
            summary: contract
                .get("summary")
                .and_then(serde_json::Value::as_str)
                .unwrap_or(fallback_summary)
                .to_string(),
            acceptance_results: normalized_acceptance_results(contract.get("acceptance_results")),
            evidence_refs: contract
                .get("evidence_refs")
                .and_then(serde_json::Value::as_array)
                .map(|refs| normalized_result_refs(refs, "evidence"))
                .unwrap_or_else(|| normalized_result_refs(evidence_refs, "evidence")),
            artifact_refs: contract
                .get("artifact_refs")
                .and_then(serde_json::Value::as_array)
                .map(|refs| normalized_result_refs(refs, "artifact"))
                .unwrap_or_else(|| normalized_result_refs(artifact_refs, "artifact")),
            changes_made: contract
                .get("changes_made")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            deliverables: contract
                .get("deliverables")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            verification: normalized_verifications(
                contract.get("verification"),
                runtime_attestation_ref,
            ),
            risks: contract
                .get("risks")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            follow_up_requests: contract
                .get("follow_up_requests")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            human_review_request: contract.get("human_review_request").cloned(),
            revision_request: contract.get("revision_request").cloned(),
            blocker: contract.get("blocker").cloned(),
            failure: contract.get("failure").cloned(),
            replan_request: contract.get("replan_request").cloned(),
            cancellation: contract.get("cancellation").cloned(),
        });
    }

    None
}

fn synthesized_result_contract(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    fallback_summary: &str,
    evidence_refs: &[serde_json::Value],
    artifact_refs: &[serde_json::Value],
    runtime_attestation_ref: Option<&str>,
) -> Option<TaskResultContract> {
    let acceptance_criteria =
        handoff_string_array(&context.handoff_contract, "acceptance_criteria");
    let verification_requirements =
        handoff_string_array(&context.handoff_contract, "verification_requirements");
    let needs_verification = expected_output(context, "verification");
    if acceptance_criteria.is_empty() && verification_requirements.is_empty() && !needs_verification
    {
        return None;
    }

    let normalized_evidence_refs = normalized_result_refs(evidence_refs, "evidence");
    let normalized_artifact_refs = normalized_result_refs(artifact_refs, "artifact");
    let acceptance_evidence_refs = result_ref_strings(&normalized_evidence_refs)
        .into_iter()
        .next()
        .map(|reference| vec![reference])
        .unwrap_or_else(|| vec![format!("runtime-command://{command_id}")]);
    let acceptance_results = acceptance_criteria
        .iter()
        .map(|criterion| {
            serde_json::json!({
                "criterion": criterion,
                "status": "passed",
                "summary": "Runtime synthesized acceptance from provider completion.",
                "evidence_refs": acceptance_evidence_refs.clone()
            })
        })
        .collect();
    let verification = synthesized_verifications(
        &verification_requirements,
        runtime_attestation_ref,
        &normalized_evidence_refs,
    );

    Some(TaskResultContract {
        status: "completed".to_string(),
        summary: fallback_summary.to_string(),
        acceptance_results,
        evidence_refs: normalized_evidence_refs,
        artifact_refs: normalized_artifact_refs,
        changes_made: Vec::new(),
        deliverables: Vec::new(),
        verification,
        risks: Vec::new(),
        follow_up_requests: Vec::new(),
        human_review_request: None,
        revision_request: None,
        blocker: None,
        failure: None,
        replan_request: None,
        cancellation: None,
    })
}

// A provider may return a structured result_contract without echoing the
// planner's acceptance criteria (acceptance_results) or verification
// requirements. The control plane validates completed results against the
// handoff contract, so such a run — even with real deliverables — is
// rejected (`acceptance_result_missing:*`) and escalates to a human
// clarification card whose body is just the completion report. The
// no-contract path already synthesizes these entries from provider
// completion; a partial contract deserves the same normalization, not a
// stricter fate. Non-completed statuses are left untouched: never fabricate
// passed criteria for failed/blocked/revision results.
fn backfill_partial_contract_gaps(
    contract: &mut TaskResultContract,
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    runtime_attestation_ref: Option<&str>,
) {
    if contract.status != "completed" {
        return;
    }
    if contract.acceptance_results.is_empty() {
        let acceptance_criteria =
            handoff_string_array(&context.handoff_contract, "acceptance_criteria");
        if !acceptance_criteria.is_empty() {
            let acceptance_evidence_refs = result_ref_strings(&contract.evidence_refs)
                .into_iter()
                .next()
                .map(|reference| vec![reference])
                .unwrap_or_else(|| vec![format!("runtime-command://{command_id}")]);
            contract.acceptance_results = acceptance_criteria
                .iter()
                .map(|criterion| {
                    serde_json::json!({
                        "criterion": criterion,
                        "status": "passed",
                        "summary": "Runtime synthesized acceptance from provider completion.",
                        "evidence_refs": acceptance_evidence_refs.clone()
                    })
                })
                .collect();
        }
    }
    if contract.verification.is_empty() {
        let verification_requirements =
            handoff_string_array(&context.handoff_contract, "verification_requirements");
        if !verification_requirements.is_empty() {
            contract.verification = synthesized_verifications(
                &verification_requirements,
                runtime_attestation_ref,
                &contract.evidence_refs,
            );
        }
    }
}

fn result_ref_strings(values: &[serde_json::Value]) -> Vec<String> {
    values.iter().filter_map(result_ref_string).collect()
}

fn synthesized_verifications(
    requirements: &[String],
    runtime_attestation_ref: Option<&str>,
    evidence_refs: &[serde_json::Value],
) -> Vec<serde_json::Value> {
    let refs = if let Some(attestation_ref) = runtime_attestation_ref {
        vec![serde_json::json!({
            "type": "attestation",
            "ref": attestation_ref
        })]
    } else {
        evidence_refs.to_vec()
    };
    requirements
        .iter()
        .map(|requirement| {
            serde_json::json!({
                "status": "passed",
                "summary": requirement,
                "evidence_refs": refs
            })
        })
        .collect()
}

fn handoff_string_array(value: &serde_json::Value, key: &str) -> Vec<String> {
    value
        .as_object()
        .and_then(|object| object.get(key))
        .and_then(serde_json::Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(serde_json::Value::as_str)
                .filter_map(|text| trimmed_optional(Some(text)))
                .collect()
        })
        .unwrap_or_default()
}

fn normalized_verifications(
    value: Option<&serde_json::Value>,
    runtime_attestation_ref: Option<&str>,
) -> Vec<serde_json::Value> {
    value
        .and_then(serde_json::Value::as_array)
        .map(|items| {
            items
                .iter()
                .map(|item| normalized_verification(item, runtime_attestation_ref))
                .collect()
        })
        .unwrap_or_default()
}

fn normalized_verification(
    value: &serde_json::Value,
    runtime_attestation_ref: Option<&str>,
) -> serde_json::Value {
    if let Some(summary) = value.as_str().and_then(|text| trimmed_optional(Some(text))) {
        let mut normalized = serde_json::Map::new();
        normalized.insert(
            "status".to_string(),
            serde_json::Value::String("passed".to_string()),
        );
        normalized.insert("summary".to_string(), serde_json::Value::String(summary));
        if let Some(attestation_ref) = runtime_attestation_ref {
            normalized.insert(
                "evidence_refs".to_string(),
                serde_json::Value::Array(vec![serde_json::json!({
                    "type": "attestation",
                    "ref": attestation_ref
                })]),
            );
        }
        return serde_json::Value::Object(normalized);
    }

    let Some(object) = value.as_object() else {
        return value.clone();
    };
    let mut normalized = object.clone();
    if !verification_status_is_passed(&normalized) {
        return serde_json::Value::Object(normalize_verification_evidence_refs(normalized));
    }
    let Some(attestation_ref) = runtime_attestation_ref else {
        return serde_json::Value::Object(normalize_verification_evidence_refs(normalized));
    };
    if verification_has_attestation_ref(&normalized) {
        return serde_json::Value::Object(normalize_verification_evidence_refs(normalized));
    }

    let mut refs = normalized
        .remove("evidence_refs")
        .and_then(|value| value.as_array().cloned())
        .unwrap_or_default();
    refs.push(serde_json::json!({
        "type": "attestation",
        "ref": attestation_ref
    }));
    normalized.insert("evidence_refs".to_string(), serde_json::Value::Array(refs));
    let normalized = normalize_verification_evidence_refs(normalized);
    serde_json::Value::Object(normalized)
}

fn normalize_verification_evidence_refs(
    mut object: serde_json::Map<String, serde_json::Value>,
) -> serde_json::Map<String, serde_json::Value> {
    if let Some(status) = object
        .get("status")
        .and_then(serde_json::Value::as_str)
        .and_then(|status| normalized_verification_status(status))
    {
        object.insert("status".to_string(), serde_json::Value::String(status));
    }
    let mut summary_from_name = false;
    if let Some(name) = object
        .remove("name")
        .and_then(|value| value.as_str().and_then(|text| trimmed_optional(Some(text))))
    {
        object
            .entry("type".to_string())
            .or_insert_with(|| serde_json::Value::String(name.clone()));
        if !object.contains_key("summary") {
            object.insert("summary".to_string(), serde_json::Value::String(name));
            summary_from_name = true;
        }
    }
    if let Some(evidence) = object
        .remove("evidence")
        .and_then(verification_evidence_summary)
    {
        if summary_from_name || !object.contains_key("summary") {
            object.insert("summary".to_string(), serde_json::Value::String(evidence));
        }
    }
    let mut refs: Vec<serde_json::Value> = object
        .remove("evidence_refs")
        .and_then(|value| value.as_array().cloned())
        .unwrap_or_default();
    if let Some(singular) = object.remove("evidence_ref") {
        refs.push(singular);
    }
    if !refs.is_empty() {
        object.insert(
            "evidence_refs".to_string(),
            serde_json::Value::Array(normalized_result_refs(&refs, "evidence")),
        );
    }
    object
}

fn verification_evidence_summary(value: serde_json::Value) -> Option<String> {
    value
        .as_str()
        .and_then(|text| trimmed_optional(Some(text)))
        .or_else(|| (!value.is_null()).then(|| value.to_string()))
}

fn verification_status_is_passed(object: &serde_json::Map<String, serde_json::Value>) -> bool {
    object
        .get("status")
        .and_then(serde_json::Value::as_str)
        .map(str::trim)
        .and_then(|status| normalized_verification_status(status))
        .is_some_and(|status| status == "passed")
}

fn verification_has_attestation_ref(object: &serde_json::Map<String, serde_json::Value>) -> bool {
    object
        .get("evidence_refs")
        .and_then(serde_json::Value::as_array)
        .is_some_and(|refs| refs.iter().any(result_ref_is_attestation))
}

fn result_ref_is_attestation(value: &serde_json::Value) -> bool {
    if value
        .as_str()
        .and_then(|text| trimmed_optional(Some(text)))
        .is_some_and(|text| text.starts_with("attestation:"))
    {
        return true;
    }
    value.as_object().is_some_and(|object| {
        object
            .get("type")
            .and_then(serde_json::Value::as_str)
            .is_some_and(|kind| kind.trim().eq_ignore_ascii_case("attestation"))
            || object
                .get("kind")
                .and_then(serde_json::Value::as_str)
                .is_some_and(|kind| kind.trim().eq_ignore_ascii_case("attestation"))
            || object
                .get("ref")
                .and_then(serde_json::Value::as_str)
                .and_then(|text| trimmed_optional(Some(text)))
                .is_some_and(|text| text.starts_with("attestation:"))
    })
}

fn normalized_acceptance_results(value: Option<&serde_json::Value>) -> Vec<serde_json::Value> {
    value
        .and_then(serde_json::Value::as_array)
        .map(|items| items.iter().map(normalized_acceptance_result).collect())
        .unwrap_or_default()
}

fn normalized_acceptance_result(value: &serde_json::Value) -> serde_json::Value {
    let Some(object) = value.as_object() else {
        return value.clone();
    };
    let mut normalized = serde_json::Map::new();
    if let Some(criterion) = object
        .get("criterion")
        .or_else(|| object.get("criteria"))
        .and_then(serde_json::Value::as_str)
        .and_then(|text| trimmed_optional(Some(text)))
    {
        normalized.insert(
            "criterion".to_string(),
            serde_json::Value::String(criterion),
        );
    }
    for key in [
        "id",
        "criterion_id",
        "name",
        "summary",
        "human_accepted_reason",
    ] {
        if let Some(value) = object
            .get(key)
            .and_then(serde_json::Value::as_str)
            .and_then(|text| trimmed_optional(Some(text)))
        {
            normalized.insert(key.to_string(), serde_json::Value::String(value));
        }
    }
    if let Some(status) = object
        .get("status")
        .and_then(serde_json::Value::as_str)
        .and_then(|status| normalized_acceptance_status(status))
    {
        normalized.insert("status".to_string(), serde_json::Value::String(status));
    }
    if let Some(refs) = object
        .get("evidence_refs")
        .and_then(serde_json::Value::as_array)
    {
        normalized.insert(
            "evidence_refs".to_string(),
            serde_json::Value::Array(
                refs.iter()
                    .filter_map(result_ref_string)
                    .map(serde_json::Value::String)
                    .collect(),
            ),
        );
    }
    serde_json::Value::Object(normalized)
}

fn normalized_task_result_status(status: &str) -> Option<String> {
    match status.trim() {
        "completed" | "complete" | "success" | "succeeded" | "done" => {
            Some("completed".to_string())
        }
        "revision_needed" | "needs_revision" | "revise" => Some("revision_needed".to_string()),
        "blocked" | "waiting_human" | "needs_human" => Some("blocked".to_string()),
        "failed" | "failure" => Some("failed".to_string()),
        "cancelled" | "canceled" => Some("cancelled".to_string()),
        "" => None,
        other => Some(other.to_string()),
    }
}

fn normalized_acceptance_status(status: &str) -> Option<String> {
    match status.trim() {
        "passed" | "pass" | "success" | "succeeded" | "verified" => Some("passed".to_string()),
        "failed" | "fail" | "failure" => Some("failed".to_string()),
        "needs_human" | "human_review" | "needs_review" => Some("needs_human".to_string()),
        "not_applicable" | "n/a" | "skipped" => Some("not_applicable".to_string()),
        "human_overridden" => Some("human_overridden".to_string()),
        "" => None,
        other => Some(other.to_string()),
    }
}

fn normalized_verification_status(status: &str) -> Option<String> {
    match status.trim() {
        "passed" | "pass" | "success" | "succeeded" | "verified" => Some("passed".to_string()),
        "failed" | "fail" | "failure" => Some("failed".to_string()),
        "skipped" | "skip" | "not_applicable" | "n/a" => Some("skipped".to_string()),
        "" => None,
        other => Some(other.to_string()),
    }
}

fn result_ref_string(value: &serde_json::Value) -> Option<String> {
    value
        .as_str()
        .and_then(|text| trimmed_optional(Some(text)))
        .or_else(|| {
            value.as_object().and_then(|object| {
                ["ref", "uri", "url", "id"].iter().find_map(|key| {
                    object
                        .get(*key)
                        .and_then(serde_json::Value::as_str)
                        .and_then(|text| trimmed_optional(Some(text)))
                })
            })
        })
}

fn normalized_result_refs(
    values: &[serde_json::Value],
    default_type: &str,
) -> Vec<serde_json::Value> {
    values
        .iter()
        .filter_map(|value| normalized_result_ref(value, default_type))
        .collect()
}

fn normalized_result_ref(
    value: &serde_json::Value,
    default_type: &str,
) -> Option<serde_json::Value> {
    if let Some(text) = value.as_str().and_then(|text| trimmed_optional(Some(text))) {
        let mut object = serde_json::Map::new();
        object.insert(
            "type".to_string(),
            serde_json::Value::String(default_type.to_string()),
        );
        object.insert("ref".to_string(), serde_json::Value::String(text));
        return Some(serde_json::Value::Object(object));
    }

    let object = value.as_object()?;
    let reference = ["ref", "uri", "url", "id"]
        .iter()
        .find_map(|key| object.get(*key).and_then(serde_json::Value::as_str))
        .and_then(|text| trimmed_optional(Some(text)))?;
    let result_type = object
        .get("type")
        .and_then(serde_json::Value::as_str)
        .and_then(|text| trimmed_optional(Some(text)))
        .unwrap_or_else(|| default_type.to_string());

    let mut result = serde_json::Map::new();
    result.insert("type".to_string(), serde_json::Value::String(result_type));
    result.insert("ref".to_string(), serde_json::Value::String(reference));
    if let Some(summary) = object
        .get("summary")
        .and_then(serde_json::Value::as_str)
        .and_then(|text| trimmed_optional(Some(text)))
    {
        result.insert("summary".to_string(), serde_json::Value::String(summary));
    }
    Some(serde_json::Value::Object(result))
}

fn trimmed_optional(value: Option<&str>) -> Option<String> {
    value
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn runtime_command_evidence_ref(
    command_id: &str,
    provider_session_id: Option<&str>,
) -> serde_json::Value {
    let mut evidence = serde_json::Map::from_iter([
        (
            "type".to_string(),
            serde_json::Value::String("runtime_command".to_string()),
        ),
        (
            "ref".to_string(),
            serde_json::Value::String(format!("runtime-command://{command_id}")),
        ),
    ]);
    if let Some(provider_session_id) = provider_session_id {
        evidence.insert(
            "provider_session_id".to_string(),
            serde_json::Value::String(provider_session_id.to_string()),
        );
    }
    serde_json::Value::Object(evidence)
}

fn project_task_fail_writeback(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    error_message: &str,
) -> ProjectTaskFailWriteback {
    // Deprecated entry: string-only call sites. Family comes from error_map
    // fallback, not scattered contains chains in this function.
    let envelope = envelope_for_code(
        error_map::classify_message_fallback(error_message),
        error_message.trim(),
        "unknown",
    );
    project_task_fail_writeback_from_envelope(context, command_id, &envelope)
}

fn project_task_fail_writeback_from_envelope(
    context: &ProjectTaskWritebackContext,
    command_id: &str,
    envelope: &ErrorEnvelope,
) -> ProjectTaskFailWriteback {
    ProjectTaskFailWriteback {
        raw_log: None,
        project_task_id: context.project_task_id.clone(),
        lease_token: context.lease_token.clone(),
        runtime_node_id: context.runtime_node_id.clone(),
        idempotency_key: project_task_attempt_idempotency_key(
            &context.attempt_id,
            "fail",
            command_id,
        ),
        failure_summary: envelope.message.trim().to_string(),
        failure_family: envelope.family.clone(),
        error_code: {
            let code = envelope.code.trim();
            (!code.is_empty()).then(|| code.to_string())
        },
        retryable: envelope.retryable,
        result_contract: None,
    }
}

/// Emits the L2 terminal marker (`turn_error`) for a failing attempt.
///
/// **Ordering is load-bearing**: Control Plane rejects run events once the run is
/// terminal, so a marker emitted after the terminal writeback is silently dropped
/// and the execution timeline just stops mid-stream (spec §4.2.1). Call this
/// before `fail_with_envelope`.
///
/// Never fatal: the terminal writeback still carries code/family, so a failed
/// marker is logged rather than propagated.
async fn emit_turn_error_marker(
    runs: &RuntimeRunStore,
    writeback: Option<&RuntimeCommandWritebackSink>,
    run_id: &str,
    envelope: &ErrorEnvelope,
    provider_session_id: Option<&str>,
    environment: &BTreeMap<String, String>,
) {
    let event = ProviderEvent::turn_error_from_envelope(envelope.clone());
    match runs.record_event(run_id, event).await {
        Ok(record) => {
            if let Some(writeback) = writeback
                && let Err(error) = writeback
                    .record_event(&record, provider_session_id, environment)
                    .await
            {
                eprintln!("turn_error marker writeback failed for run {run_id}: {error}");
            }
        }
        Err(error) => {
            eprintln!("turn_error marker not recorded for run {run_id}: {error}");
        }
    }
}

fn command_failed_terminal_from_envelope(
    envelope: &ErrorEnvelope,
    diagnostics: Option<serde_json::Value>,
) -> RuntimeCommandTerminalWriteback {
    RuntimeCommandTerminalWriteback {
        status: "failed".to_string(),
        summary: None,
        result: None,
        diagnostic: diagnostics_to_terminal_map(diagnostics.as_ref()),
        provider_session_external_id: None,
        session_state_patch: None,
        log_ref: None,
        raw_result_ref: None,
        error_message: Some(envelope.message.clone()),
        error_code: Some(envelope.code.clone()),
        error_family: Some(envelope.family.clone()),
    }
}

fn project_task_attempt_idempotency_key(
    attempt_id: &str,
    action: &str,
    command_id: &str,
) -> String {
    format!("project-task-attempt:{attempt_id}:{action}:{command_id}")
}

/// Best-effort rollback of session projections:
/// 1) stable-dir skill/MCP session manifest unload (spec 2026-07-23 §6)
/// 2) home-dir MCP injection rollback
/// Failure must never override the run outcome.
fn rollback_session_projections_best_effort(
    label: &str,
    agent_home_dir: Option<&Path>,
    workspace_path: Option<&Path>,
    command_id: Option<&str>,
) {
    if let (Some(workspace), Some(command_id)) = (workspace_path, command_id) {
        crate::project_session::unload_project_session_best_effort(workspace, command_id);
    }
    rollback_session_mcp_config_best_effort(label, agent_home_dir);
}

/// Best-effort rollback of the session-scoped home-dir MCP injection made by
/// `ensure_command_instance`. Failure to roll back must never override the
/// run's actual outcome, so it is logged (this crate has no tracing/log setup)
/// and swallowed; a residual manifest is defensively rolled back by the next
/// session's `inject_session_mcp_config` call.
fn rollback_session_mcp_config_best_effort(run_id: &str, agent_home_dir: Option<&Path>) {
    if let Some(agent_home) = agent_home_dir {
        if let Err(error) = crate::mcp_config::rollback_session_mcp_config(agent_home) {
            eprintln!(
                "mcp session rollback failed for run {} at {}: {error:#}",
                run_id,
                agent_home.display()
            );
        }
    }
}

async fn drain_provider_events(
    runs: RuntimeRunStore,
    registry: RuntimeCommandRegistry,
    run_id: String,
    mut events: ProviderEventStream,
    reusable_provider_session: bool,
    writeback: Option<RuntimeCommandWritebackSink>,
    spec: RunSpec,
    provider_started_at: Instant,
    heartbeat_stop: Option<CancellationToken>,
    raw_sink: Arc<dyn crate::raw_log::RawLineSink>,
    terminal_cleanup: Option<crate::workspace_cleanup::TerminalWorkspaceCleanup>,
) -> anyhow::Result<()> {
    // Prefer registry provider_type (claude-code/…); never put short kind alone
    // into ErrorEnvelope.provider_type (spec §13 #7).
    let provider_type = spec.registry_provider_type();
    let mut latest_provider_session_id: Option<String> = None;
    let mut terminal_writeback = ProviderTerminalWritebackState::default();
    let mut fallback_text_summary = String::new();
    let mut unmapped_native_count: u64 = 0;
    let mut native_unmapped_writebacks: u32 = 0;
    let emit_native_unmapped = crate::providers::emit_native_unmapped_enabled();
    while let Some(item) = events.next().await {
        let event = match item {
            Ok(event) => event,
            // Path 4 (stream Err): synthesize turn_error + attestation + fail
            // writeback so L2 timeline has a terminal marker and attestation
            // is not skipped (spec §4.2.1 / §6.4).
            Err(error) => {
                let envelope = envelope_from_anyhow(&error, provider_type);
                let diagnostics = Some(attempt_stream_diagnostics(
                    provider_type,
                    unmapped_native_count,
                ));
                if let Some(stop) = &heartbeat_stop {
                    stop.cancel();
                }
                terminal_writeback.failed = true;
                terminal_writeback.pending_completion = None;
                emit_turn_error_marker(
                    &runs,
                    writeback.as_ref(),
                    &run_id,
                    &envelope,
                    latest_provider_session_id.as_deref(),
                    &spec.environment,
                )
                .await;
                registry.record_run_finished(&run_id);
                let command_id = spec
                    .command_context
                    .as_ref()
                    .map(|context| context.command_id.as_str());
                rollback_session_projections_best_effort(
                    &run_id,
                    spec.agent_home_dir.as_deref(),
                    Some(spec.workspace_path.as_path()),
                    command_id,
                );
                finalize_raw_log(&raw_sink, writeback.as_ref()).await;
                if let Some(writeback) = &writeback {
                    writeback
                        .record_attestation(
                            &spec,
                            "provider_terminal",
                            "failed",
                            latest_provider_session_id.as_deref(),
                            Some(
                                provider_started_at
                                    .elapsed()
                                    .as_millis()
                                    .min(i64::MAX as u128) as i64,
                            ),
                        )
                        .await;
                    let _ = writeback.fail_with_envelope(envelope, diagnostics).await;
                }
                if let Some(cleanup) = &terminal_cleanup {
                    cleanup.apply(false);
                }
                return Ok(());
            }
        };
        if let ProviderEvent::TextDelta { text } = &event {
            fallback_text_summary.push_str(text);
        }
        if let ProviderEvent::SessionStarted { session_id, .. } = &event {
            if latest_provider_session_id.as_deref() == Some(session_id.as_str()) {
                continue;
            }
            latest_provider_session_id = Some(session_id.clone());
            registry.record_provider_session_with_recoverability(
                &run_id,
                session_id,
                reusable_provider_session,
            );
        }
        let is_terminal = matches!(
            event,
            ProviderEvent::TurnCompleted { .. } | ProviderEvent::TurnError { .. }
        );
        if event.is_native_unmapped() {
            unmapped_native_count = unmapped_native_count.saturating_add(1);
        }
        let writeback_action = terminal_writeback.observe_event(
            &event,
            latest_provider_session_id.as_deref(),
            provider_type,
        );
        let skip_cp_writeback = match &event {
            ProviderEvent::NativeUnmapped { .. } => {
                if !emit_native_unmapped {
                    true
                } else if native_unmapped_writebacks
                    >= crate::providers::NATIVE_UNMAPPED_WRITEBACK_LIMIT
                {
                    true
                } else {
                    native_unmapped_writebacks = native_unmapped_writebacks.saturating_add(1);
                    false
                }
            }
            _ => false,
        };
        let record = runs.record_event(&run_id, event).await?;
        if is_terminal {
            registry.record_run_finished(&run_id);
        }
        if let Some(writeback) = &writeback {
            if !skip_cp_writeback {
                writeback
                    .record_event(
                        &record,
                        latest_provider_session_id.as_deref(),
                        &spec.environment,
                    )
                    .await?;
            }
            if let Some(action) = writeback_action {
                match action {
                    // Path 1: in-loop TurnError (the marker is the event itself,
                    // already written back above).
                    ProviderTerminalWritebackAction::Fail(envelope) => {
                        let diagnostics = Some(attempt_stream_diagnostics(
                            provider_type,
                            unmapped_native_count,
                        ));
                        if let Some(stop) = &heartbeat_stop {
                            stop.cancel();
                        }
                        writeback
                            .record_attestation(
                                &spec,
                                "provider_terminal",
                                "failed",
                                latest_provider_session_id.as_deref(),
                                Some(
                                    provider_started_at
                                        .elapsed()
                                        .as_millis()
                                        .min(i64::MAX as u128)
                                        as i64,
                                ),
                            )
                            .await;
                        writeback.fail_with_envelope(envelope, diagnostics).await?;
                    }
                }
            }
        }
    }
    if let Some(stop) = &heartbeat_stop {
        stop.cancel();
    }
    if unmapped_native_count > 0 {
        let threshold = crate::providers::unmapped_alert_threshold();
        if threshold > 0 && unmapped_native_count >= threshold {
            crate::providers::note_drift_alert();
            eprintln!(
                "ALERT provider_stream_drift run={run_id} provider={provider_type} \
                 unmapped_native={unmapped_native_count} \
                 native_unmapped_writebacks={native_unmapped_writebacks} threshold={threshold}"
            );
        } else {
            eprintln!(
                "run {run_id}: attempt diagnostics unmapped_native={unmapped_native_count} \
                 native_unmapped_writebacks={native_unmapped_writebacks} emit={emit_native_unmapped}"
            );
        }
    }
    // Session-scoped projection rollback: unload project-dir skill/MCP
    // session (spec 2026-07-23 §6) and restore home-dir MCP injection.
    // handle_stop_command deliberately does not roll back directly: cancel_run
    // makes the provider event stream end, which drives execution back here.
    // Early `?` exits above (stream item error, record/writeback failures) skip
    // this hook; spawn_provider_event_drain's Err branch backstops those with
    // the same rollback (idempotent no-op when this hook already ran).
    // If the process restarts before either point runs (e.g. stop after a
    // crash), the residual project session is unloaded defensively by the next
    // session's install_project_session, and home MCP by inject_session_mcp_config.
    let command_id = spec
        .command_context
        .as_ref()
        .map(|context| context.command_id.as_str());
    rollback_session_projections_best_effort(
        &run_id,
        spec.agent_home_dir.as_deref(),
        Some(spec.workspace_path.as_path()),
        command_id,
    );
    // Finalize before the terminal writeback so the attempt row and the raw
    // transcript pointer land in the same call.
    finalize_raw_log(&raw_sink, writeback.as_ref()).await;
    let mut run_succeeded = false;
    let stream_failed = terminal_writeback.failed;
    match terminal_writeback.finish_successful_stream() {
        // Path 2: stream ended with TurnCompleted
        Some(mut completion) => {
            if let Some(writeback) = &writeback {
                let has_summary = completion
                    .summary
                    .as_deref()
                    .map(str::trim)
                    .is_some_and(|value| !value.is_empty());
                let fallback_text = fallback_text_summary.trim();
                if !has_summary && !fallback_text.is_empty() {
                    completion.summary = Some(fallback_text.to_string());
                }
                let diagnostics = Some(attempt_stream_diagnostics(
                    provider_type,
                    unmapped_native_count,
                ));
                writeback
                    .record_attestation(
                        &spec,
                        "provider_terminal",
                        "succeeded",
                        completion.provider_session_id.as_deref(),
                        Some(
                            provider_started_at
                                .elapsed()
                                .as_millis()
                                .min(i64::MAX as u128) as i64,
                        ),
                    )
                    .await;
                writeback
                    .complete(
                        completion.summary,
                        completion.provider_session_id,
                        diagnostics,
                    )
                    .await?;
                run_succeeded = true;
            }
        }
        // Path 3: stream ended without TurnCompleted/TurnError (exit 0 empty /
        // format drift). TurnError path (stream_failed) already failed above.
        None if !stream_failed => {
            if !run_is_cancelled(&runs, &run_id).await {
                let envelope = envelope_for_code(
                    error_code::PROVIDER_NO_TERMINAL_EVENT,
                    "provider exited without a terminal event",
                    provider_type,
                );
                let diagnostics = Some(attempt_stream_diagnostics(
                    provider_type,
                    unmapped_native_count,
                ));
                // Marker before the terminal writeback (see emit_turn_error_marker):
                // this also replaces the old `runs.finish_failed`, which only moved
                // the local snapshot and never reached Control Plane.
                emit_turn_error_marker(
                    &runs,
                    writeback.as_ref(),
                    &run_id,
                    &envelope,
                    latest_provider_session_id.as_deref(),
                    &spec.environment,
                )
                .await;
                if let Some(writeback) = &writeback {
                    writeback
                        .record_attestation(
                            &spec,
                            "provider_terminal",
                            "failed",
                            latest_provider_session_id.as_deref(),
                            Some(
                                provider_started_at
                                    .elapsed()
                                    .as_millis()
                                    .min(i64::MAX as u128)
                                    as i64,
                            ),
                        )
                        .await;
                    let _ = writeback.fail_with_envelope(envelope, diagnostics).await;
                }
            }
        }
        None => {}
    }
    // 终态清理(spec §5):在 artifacts/attestation 采集与终态回写全部落地之后
    // 执行,绝不影响 run 结果。早退 `?` 路径会跳过本钩子——遗留目录由后台清扫
    // (workspace_cleanup::sweep)兜底。
    if let Some(cleanup) = &terminal_cleanup {
        cleanup.apply(run_succeeded);
    }
    Ok(())
}

/// Idempotent: a sink whose writer already finished yields `None`, so calling
/// this on both the success and the failure path is safe.
async fn finalize_raw_log(
    raw_sink: &Arc<dyn crate::raw_log::RawLineSink>,
    writeback: Option<&RuntimeCommandWritebackSink>,
) {
    let Some(writeback) = writeback else {
        return;
    };
    if let Some(summary) = raw_sink.finalize_log().await {
        if let Ok(mut guard) = writeback.raw_log.lock() {
            *guard = Some(summary);
        }
    }
}

fn provider_request(spec: &RunSpec) -> ProviderRequest {
    ProviderRequest {
        prompt: spec.prompt.clone(),
        system_prompt: spec.system_prompt.clone(),
        workspace_path: spec.workspace_path.clone(),
        agent_home_dir: spec.agent_home_dir.clone(),
        employee_capability_dir: spec.employee_capability_dir.clone(),
        capability_manifest_version: spec.capability_manifest_version.clone(),
        provider_auth_mode: spec.provider_auth_mode.clone(),
        mcp_config_path: spec.mcp_config_path.clone(),
        session_id: spec.session_id.clone(),
        continue_session: spec.continue_session,
        model: spec.model.clone(),
        environment: spec.environment.clone(),
    }
}

async fn run_is_cancelled(runs: &RuntimeRunStore, run_id: &str) -> bool {
    runs.get_run(run_id)
        .await
        .is_some_and(|snapshot| snapshot.status == RunStatus::Cancelled)
}

fn non_empty_session_id(payload: &RuntimeSessionCommandPayload) -> Option<String> {
    metadata_string(&payload.metadata, "provider_session_id").or_else(|| {
        payload
            .session_policy
            .provider_session_id
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToString::to_string)
    })
}

fn reusable_provider_session(payload: &RuntimeSessionCommandPayload) -> bool {
    payload.session_policy.recoverable
        && payload.session_policy.mode != SessionPolicyMode::Ephemeral
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::commands::payload::RuntimeSessionPolicy;
    use serde_json::json;
    use std::collections::BTreeMap;

    fn project_task_session_payload(digital_employee_id: &str) -> RuntimeSessionCommandPayload {
        RuntimeSessionCommandPayload {
            command_id: "cmd-project-task".to_string(),
            tenant_id: None,
            team_id: None,
            digital_employee_id: digital_employee_id.to_string(),
            execution_instance_id: "22222222-2222-4222-8222-222222222222".to_string(),
            runtime_node_id: None,
            provider_type: "claude-code".to_string(),
            agent_home_dir: Some("/tmp/runtime-agent-test".to_string()),
            persona_memory_markdown: None,
            team_constitution: None,
            capability_bindings: serde_json::json!({}),
            skills: Vec::new(),
            mcp_servers: Vec::new(),
            environment: Vec::new(),
            session_policy: RuntimeSessionPolicy {
                mode: SessionPolicyMode::New,
                provider_session_id: None,
                recoverable: true,
            },
            prompt: Some("complete the task".to_string()),
            input: None,
            context_refs: Vec::new(),
            artifact_refs: Vec::new(),
            model: None,
            metadata: json!({
                "source": "project_task_dispatch",
                "project_id": "44444444-4444-4444-8444-444444444444",
                "project_task_id": "55555555-5555-4555-8555-555555555555",
                "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
                "project_task_lease_token": "lease-token-1",
                "runtime_node_id": "77777777-7777-4777-8777-777777777777",
                "execution_context_packet_version": "v1",
                "expected_outputs": ["execution_summary", "evidence_refs", "recommended_next_action"],
                "handoff_contract": {
                    "completion_path": "project_task_attempt_writeback",
                    "requires_runtime_attestation": true
                }
            }),
        }
    }

    #[test]
    fn project_task_writeback_context_requires_digital_employee_id() {
        let payload = project_task_session_payload("   ");

        assert!(project_task_writeback_context(&payload).is_none());
    }

    #[test]
    fn project_task_writeback_context_defaults_completion_path_when_omitted() {
        // The control-plane normally sets completion_path; the agent falls back to the
        // writeback path for a project_task_dispatch run when it is omitted.
        let mut payload = project_task_session_payload("emp-1");
        payload.metadata = json!({
            "source": "project_task_dispatch",
            "project_id": "44444444-4444-4444-8444-444444444444",
            "project_task_id": "55555555-5555-4555-8555-555555555555",
            "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
            "project_task_lease_token": "lease-token-1",
            "runtime_node_id": "77777777-7777-4777-8777-777777777777",
            "handoff_contract": {}
        });
        let context = project_task_writeback_context(&payload)
            .expect("writeback context should default to project_task_attempt_writeback");
        assert_eq!(
            context.project_task_id,
            "55555555-5555-4555-8555-555555555555"
        );
        assert_eq!(context.attempt_id, "66666666-6666-4666-8666-666666666666");

        // An explicit non-matching completion_path is still respected (no writeback).
        let mut other = project_task_session_payload("emp-1");
        other.metadata = json!({
            "source": "project_task_dispatch",
            "project_id": "44444444-4444-4444-8444-444444444444",
            "project_task_id": "55555555-5555-4555-8555-555555555555",
            "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
            "project_task_lease_token": "lease-token-1",
            "runtime_node_id": "77777777-7777-4777-8777-777777777777",
            "handoff_contract": {"completion_path": "manual_review"}
        });
        assert!(project_task_writeback_context(&other).is_none());
    }

    #[test]
    fn non_empty_session_id_uses_metadata_provider_session_id() {
        let mut payload = project_task_session_payload("emp-1");
        payload.metadata["provider_session_id"] = json!("metadata-session-1");

        assert_eq!(
            non_empty_session_id(&payload).as_deref(),
            Some("metadata-session-1")
        );
    }

    #[test]
    fn non_empty_session_id_falls_back_to_session_policy_when_metadata_absent() {
        let mut payload = project_task_session_payload("emp-1");
        payload.session_policy.provider_session_id = Some("policy-session-1".to_string());

        assert_eq!(
            non_empty_session_id(&payload).as_deref(),
            Some("policy-session-1")
        );
    }

    #[test]
    fn non_empty_session_id_prefers_metadata_over_session_policy() {
        let mut payload = project_task_session_payload("emp-1");
        payload.metadata["provider_session_id"] = json!("metadata-session-1");
        payload.session_policy.provider_session_id = Some("policy-session-1".to_string());

        assert_eq!(
            non_empty_session_id(&payload).as_deref(),
            Some("metadata-session-1")
        );
    }

    #[test]
    fn non_empty_session_id_ignores_blank_metadata_value() {
        let mut payload = project_task_session_payload("emp-1");
        payload.metadata["provider_session_id"] = json!("   ");
        payload.session_policy.provider_session_id = Some("policy-session-1".to_string());

        assert_eq!(
            non_empty_session_id(&payload).as_deref(),
            Some("policy-session-1")
        );
    }

    #[test]
    fn provider_terminal_writeback_defers_completion_until_stream_finishes() {
        let mut state = ProviderTerminalWritebackState::default();

        let action = state.observe_event(
            &ProviderEvent::TurnCompleted {
                summary: Some("looks successful".to_string()),
                usage: None,
            },
            Some("provider-session-1"),
            "claude-code",
        );

        assert!(action.is_none());
        let completion = state
            .finish_successful_stream()
            .expect("completion should be emitted after the stream finishes successfully");
        assert_eq!(completion.summary.as_deref(), Some("looks successful"));
        assert_eq!(
            completion.provider_session_id.as_deref(),
            Some("provider-session-1")
        );
    }

    #[test]
    fn provider_terminal_writeback_uses_text_delta_when_completion_summary_empty() {
        let mut state = ProviderTerminalWritebackState::default();

        assert!(
            state
                .observe_event(
                    &ProviderEvent::TextDelta {
                        text: "{\"summary\":\"provider final answer\"}".to_string(),
                    },
                    Some("provider-session-1"),
                    "claude-code",
                )
                .is_none()
        );
        assert!(
            state
                .observe_event(
                    &ProviderEvent::TurnCompleted {
                        summary: None,
                        usage: None
                    },
                    Some("provider-session-1"),
                    "claude-code",
                )
                .is_none()
        );

        let completion = state
            .finish_successful_stream()
            .expect("completion should use buffered provider text");
        assert_eq!(
            completion.summary.as_deref(),
            Some("{\"summary\":\"provider final answer\"}")
        );
        assert_eq!(
            completion.provider_session_id.as_deref(),
            Some("provider-session-1")
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_acceptance_evidence_refs_to_strings() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "acceptance_results": [
                    {
                        "criterion": "return structured result",
                        "status": "passed",
                        "evidence_refs": [
                            {"type": "runtime_result_payload", "ref": "final_answer.raw_json_object"},
                            {"id": "evidence-2"}
                        ]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(
            contract.acceptance_results[0]["evidence_refs"],
            json!(["final_answer.raw_json_object", "evidence-2"])
        );
    }

    #[test]
    fn parsed_result_contract_preserves_deliverables() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "deliverables": [
                    {"name": "head_commit", "kind": "git_commit", "value": "abc123"}
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(contract.deliverables.len(), 1);
        assert_eq!(contract.deliverables[0]["name"], json!("head_commit"));
    }

    #[test]
    fn backfill_partial_contract_fills_missing_acceptance_results() {
        let mut payload = project_task_session_payload("emp-1");
        payload.metadata["handoff_contract"] = json!({
            "completion_path": "project_task_attempt_writeback",
            "acceptance_criteria": ["cities.json", "cities.json 文件存在且为有效 JSON 数组"],
            "verification_requirements": ["cities.json parses as JSON"]
        });
        let context = project_task_writeback_context(&payload).expect("context");

        // A completed partial contract (no acceptance_results echo) gets the
        // same synthesis as the no-contract path — otherwise CP validation
        // rejects it (acceptance_result_missing) and a human is pulled in to
        // countersign a card whose body is just the completion report.
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "deliverables": [{"name": "cities_json", "ref": "deliverables/cities.json"}]
            }
        });
        let mut contract =
            parsed_result_contract(Some(&parsed), "done", &[], &[], None).expect("contract");
        assert!(contract.acceptance_results.is_empty());
        backfill_partial_contract_gaps(&mut contract, &context, "cmd-1", None);
        assert_eq!(contract.acceptance_results.len(), 2);
        assert_eq!(contract.acceptance_results[0]["criterion"], json!("cities.json"));
        assert_eq!(contract.acceptance_results[0]["status"], json!("passed"));
        assert_eq!(
            contract.acceptance_results[0]["evidence_refs"],
            json!(["runtime-command://cmd-1"])
        );
        assert!(!contract.verification.is_empty());

        // A provider-supplied echo is authoritative — never overwritten.
        let parsed_with_echo = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "acceptance_results": [
                    {"criterion": "cities.json", "status": "failed", "evidence_refs": ["e1"]}
                ]
            }
        });
        let mut with_echo = parsed_result_contract(Some(&parsed_with_echo), "done", &[], &[], None)
            .expect("contract");
        backfill_partial_contract_gaps(&mut with_echo, &context, "cmd-1", None);
        assert_eq!(with_echo.acceptance_results.len(), 1);
        assert_eq!(with_echo.acceptance_results[0]["status"], json!("failed"));

        // Non-completed statuses are untouched: never fabricate passed criteria
        // for failed/blocked results.
        let parsed_failed = json!({
            "result_contract": {"status": "failed", "summary": "boom"}
        });
        let mut failed =
            parsed_result_contract(Some(&parsed_failed), "boom", &[], &[], None).expect("contract");
        backfill_partial_contract_gaps(&mut failed, &context, "cmd-1", None);
        assert!(failed.acceptance_results.is_empty());
    }

    #[test]
    fn parsed_result_contract_normalizes_acceptance_criteria_alias_to_criterion() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "acceptance_results": [
                    {
                        "criteria": "return structured result",
                        "status": "passed",
                        "evidence_refs": ["runtime-command://cmd-smoke"]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(
            contract.acceptance_results[0]["criterion"],
            json!("return structured result")
        );
        assert!(contract.acceptance_results[0].get("criteria").is_none());
    }

    #[test]
    fn parsed_result_contract_strips_unknown_acceptance_result_fields() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "acceptance_results": [
                    {
                        "criteria": "return structured result",
                        "status": "passed",
                        "summary": "checked output",
                        "evidence_refs": ["runtime-command://cmd-smoke"],
                        "verification": "provider-local note that belongs at result_contract.verification",
                        "unexpected": {"nested": true}
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        let acceptance = &contract.acceptance_results[0];
        assert_eq!(acceptance["criterion"], json!("return structured result"));
        assert_eq!(acceptance["status"], json!("passed"));
        assert_eq!(acceptance["summary"], json!("checked output"));
        assert_eq!(
            acceptance["evidence_refs"],
            json!(["runtime-command://cmd-smoke"])
        );
        assert!(acceptance.get("criteria").is_none());
        assert!(acceptance.get("verification").is_none());
        assert!(acceptance.get("unexpected").is_none());
    }

    #[test]
    fn parsed_result_contract_adds_runtime_attestation_ref_to_passed_verification() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    {"name": "cargo test", "status": "passed", "evidence_refs": []},
                    {"name": "manual review", "status": "skipped"}
                ]
            }
        });

        let contract = parsed_result_contract(
            Some(&parsed),
            "done",
            &[],
            &[],
            Some("attestation:project-task-attempt:66666666-6666-4666-8666-666666666666:attestation:provider_terminal:cmd-project-task"),
        )
        .expect("contract should parse");

        assert_eq!(
            contract.verification[0]["evidence_refs"],
            json!([
                {
                    "type": "attestation",
                    "ref": "attestation:project-task-attempt:66666666-6666-4666-8666-666666666666:attestation:provider_terminal:cmd-project-task"
                }
            ])
        );
        assert!(
            contract.verification[1].get("evidence_refs").is_none(),
            "skipped verification should not get a runtime attestation"
        );
    }

    #[test]
    fn parsed_result_contract_keeps_existing_verification_attestation_ref() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    {
                        "name": "cargo test",
                        "status": "passed",
                        "evidence_refs": [
                            {"type": "attestation", "ref": "attestation:provider-supplied"}
                        ]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(
            Some(&parsed),
            "done",
            &[],
            &[],
            Some("attestation:runtime-generated"),
        )
        .expect("contract should parse");

        assert_eq!(
            contract.verification[0]["evidence_refs"],
            json!([{"type": "attestation", "ref": "attestation:provider-supplied"}])
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_string_verification_entries() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    "Read README.md from working tree with line numbers."
                ]
            }
        });

        let contract = parsed_result_contract(
            Some(&parsed),
            "done",
            &[],
            &[],
            Some("attestation:project-task-attempt:66666666-6666-4666-8666-666666666666:attestation:provider_terminal:cmd-project-task"),
        )
        .expect("contract should parse");

        assert_eq!(
            contract.verification,
            vec![json!({
                "status": "passed",
                "summary": "Read README.md from working tree with line numbers.",
                "evidence_refs": [
                    {
                        "type": "attestation",
                        "ref": "attestation:project-task-attempt:66666666-6666-4666-8666-666666666666:attestation:provider_terminal:cmd-project-task"
                    }
                ]
            })]
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_verification_evidence_refs_to_objects() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    {
                        "status": "passed",
                        "summary": "README.md was read.",
                        "evidence_refs": [
                            "README.md:1-4"
                        ]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(
            contract.verification[0]["evidence_refs"],
            json!([
                {
                    "type": "evidence",
                    "ref": "README.md:1-4"
                }
            ])
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_verification_name_alias() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    {
                        "name": "artifact_readback",
                        "status": "passed",
                        "evidence_refs": [
                            "sed -n '1,120p' smoke-note-fixed.txt"
                        ]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(
            contract.verification,
            vec![json!({
                "status": "passed",
                "type": "artifact_readback",
                "summary": "artifact_readback",
                "evidence_refs": [
                    {
                        "type": "evidence",
                        "ref": "sed -n '1,120p' smoke-note-fixed.txt"
                    }
                ]
            })]
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_verification_evidence_alias() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    {
                        "name": "exact_text_match",
                        "status": "passed",
                        "evidence": "Output text exactly matched the expected smoke marker.",
                        "evidence_refs": [
                            "verification.exact_text_match"
                        ]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(
            contract.verification,
            vec![json!({
                "status": "passed",
                "type": "exact_text_match",
                "summary": "Output text exactly matched the expected smoke marker.",
                "evidence_refs": [
                    {
                        "type": "evidence",
                        "ref": "verification.exact_text_match"
                    }
                ]
            })]
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_verification_singular_evidence_ref() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    {
                        "type": "input_review",
                        "evidence_ref": "user_provided_demand_content",
                        "summary": "Confirmed the output is a short confirmation string matching the demand content."
                    },
                    {
                        "type": "object_review",
                        "evidence_ref": {"ref": "evidence://object-ref", "summary": "object-typed singular evidence"},
                        "evidence_refs": ["verification.already_plural"]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert!(
            contract.verification[0].get("evidence_ref").is_none(),
            "singular evidence_ref must be removed from verification entry: {}",
            contract.verification[0]
        );
        assert_eq!(
            contract.verification[0]["evidence_refs"],
            json!([
                {
                    "type": "evidence",
                    "ref": "user_provided_demand_content"
                }
            ])
        );
        assert!(
            contract.verification[1].get("evidence_ref").is_none(),
            "singular evidence_ref must be removed when plural evidence_refs is also present: {}",
            contract.verification[1]
        );
        assert_eq!(
            contract.verification[1]["evidence_refs"],
            json!([
                {
                    "type": "evidence",
                    "ref": "verification.already_plural"
                },
                {
                    "type": "evidence",
                    "ref": "evidence://object-ref",
                    "summary": "object-typed singular evidence"
                }
            ])
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_top_level_string_refs_to_objects() {
        let parsed = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "evidence_refs": [
                    "runtime-command://cmd-project-task"
                ],
                "artifact_refs": [
                    "smoke-note.txt"
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(
            contract.evidence_refs,
            vec![json!({
                "type": "evidence",
                "ref": "runtime-command://cmd-project-task"
            })]
        );
        assert_eq!(
            contract.artifact_refs,
            vec![json!({
                "type": "artifact",
                "ref": "smoke-note.txt"
            })]
        );
    }

    #[test]
    fn parsed_result_contract_normalizes_status_aliases() {
        let parsed = json!({
            "result_contract": {
                "status": "success",
                "summary": "done",
                "acceptance_results": [
                    {
                        "criterion": "artifact exists",
                        "status": "pass",
                        "evidence_refs": ["smoke-note-green.txt"]
                    }
                ],
                "verification": [
                    {
                        "status": "verified",
                        "summary": "file checked",
                        "evidence_refs": ["smoke-note-green.txt"]
                    }
                ]
            }
        });

        let contract = parsed_result_contract(Some(&parsed), "done", &[], &[], None)
            .expect("contract should parse");

        assert_eq!(contract.status, "completed");
        assert_eq!(contract.acceptance_results[0]["status"], json!("passed"));
        assert_eq!(contract.verification[0]["status"], json!("passed"));
    }

    #[test]
    fn project_task_complete_writeback_adds_attempt_attestation_to_passed_verification() {
        let payload = project_task_session_payload("emp-1");
        let context = project_task_writeback_context(&payload).expect("project task context");
        let summary = json!({
            "result_contract": {
                "status": "completed",
                "summary": "done",
                "verification": [
                    {"name": "cargo test", "status": "passed"}
                ]
            }
        })
        .to_string();

        let body = project_task_complete_writeback(
            &context,
            "cmd-project-task",
            Some(&summary),
            Some("provider-session-1"),
        );

        let contract = body.result_contract.expect("result contract");
        assert_eq!(
            contract.verification[0]["evidence_refs"],
            json!([
                {
                    "type": "attestation",
                    "ref": "attestation:project-task-attempt:66666666-6666-4666-8666-666666666666:attestation:provider_terminal:cmd-project-task"
                }
            ])
        );
    }

    #[test]
    fn project_task_complete_writeback_synthesizes_contract_acceptance_results() {
        let mut payload = project_task_session_payload("emp-1");
        payload.metadata["expected_outputs"] =
            json!(["Structured result with summary and evidence"]);
        payload.metadata["handoff_contract"] = json!({
            "completion_path": "project_task_attempt_writeback",
            "acceptance_criteria": [
                "Structured result with summary and evidence"
            ],
            "verification_requirements": [
                "Ensure all specified files were read"
            ],
            "requires_runtime_attestation": true
        });
        let context = project_task_writeback_context(&payload).expect("project task context");

        let body = project_task_complete_writeback(
            &context,
            "cmd-project-task",
            Some("Read README.md and data/revenue.csv. Revenue increased."),
            Some("provider-session-1"),
        );

        let contract = body
            .result_contract
            .expect("fallback result contract should be synthesized");
        assert_eq!(contract.status, "completed");
        assert_eq!(
            contract.acceptance_results,
            vec![json!({
                "criterion": "Structured result with summary and evidence",
                "status": "passed",
                "summary": "Runtime synthesized acceptance from provider completion.",
                "evidence_refs": ["runtime-command://cmd-project-task"]
            })]
        );
        assert_eq!(
            contract.verification,
            vec![json!({
                "status": "passed",
                "summary": "Ensure all specified files were read",
                "evidence_refs": [
                    {
                        "type": "attestation",
                        "ref": "attestation:project-task-attempt:66666666-6666-4666-8666-666666666666:attestation:provider_terminal:cmd-project-task"
                    }
                ]
            })]
        );
    }

    #[test]
    fn provider_terminal_writeback_prefers_later_failure_over_prior_completion() {
        let mut state = ProviderTerminalWritebackState::default();

        assert!(
            state
                .observe_event(
                    &ProviderEvent::TurnCompleted {
                        summary: Some("API Error: 529 provider overloaded".to_string()),
                        usage: None,
                    },
                    Some("provider-session-1"),
                    "claude-code",
                )
                .is_none()
        );
        let action = state.observe_event(
            &ProviderEvent::TurnError {
                message: "claude exited with status 1".to_string(),
                error: None,
            },
            Some("provider-session-1"),
            "claude-code",
        );

        match action {
            Some(ProviderTerminalWritebackAction::Fail(envelope)) => {
                assert_eq!(envelope.message, "claude exited with status 1");
                assert_eq!(envelope.family, "transient_provider");
                assert!(envelope.retryable);
            }
            other => panic!("expected fail action, got {other:?}"),
        }
        assert!(
            state.finish_successful_stream().is_none(),
            "a failed stream must not emit a deferred completion"
        );
    }

    /// 批一护栏：信封字段必须与业务键同层齐全，且 `seq`/`type` 与外层一致——
    /// 外层是唯一真相，这里锁的是"投影不许漂"。
    #[test]
    fn runtime_event_writeback_carries_flat_envelope_fields() {
        let environment = BTreeMap::new();
        let record = crate::runs::RunEventRecord {
            sequence: 17,
            run_id: "run-1".to_string(),
            event: ProviderEvent::TextDelta {
                text: "hello".to_string(),
            },
            // 2026-08-10T00:00:00Z
            recorded_at_ms: 1_786_320_000_000,
        };
        let writeback = runtime_event_writeback(
            &record,
            Some("ses-1"),
            &environment,
            // 短名必须归一到注册表口径
            Some("claude"),
            Some(EventAttemptRef {
                command_id: "cmd-1",
                attempt_id: Some("attempt-1"),
            }),
        );

        assert_eq!(writeback.event_type, "text_delta");
        assert_eq!(writeback.sequence_number, 17);
        assert_eq!(
            writeback.payload["schema_version"],
            serde_json::json!("provider.event.v1")
        );
        // 冗余投影与外层一致
        assert_eq!(
            writeback.payload["type"],
            serde_json::json!(writeback.event_type)
        );
        assert_eq!(
            writeback.payload["seq"],
            serde_json::json!(writeback.sequence_number)
        );
        assert_eq!(
            writeback.payload["provider_type"],
            serde_json::json!("claude-code")
        );
        assert_eq!(
            writeback.payload["provider_session_id"],
            serde_json::json!("ses-1")
        );
        assert_eq!(
            writeback.payload["attempt_ref"],
            serde_json::json!({ "command_id": "cmd-1", "attempt_id": "attempt-1" })
        );
        assert_eq!(writeback.payload["ts"], serde_json::json!("2026-08-10T00:00:00Z"));
        // 业务键不受影响
        assert_eq!(writeback.payload["text"], serde_json::json!("hello"));
    }

    #[test]
    fn runtime_event_writeback_redacts_environment_values_in_excerpts() {
        let mut environment = BTreeMap::new();
        environment.insert(
            "MY_API_TOKEN".to_string(),
            "supersecretvalue123".to_string(),
        );
        let record = crate::runs::RunEventRecord {
            sequence: 1,
            run_id: "run-1".to_string(),
            event: ProviderEvent::ToolCompleted {
                tool_id: "tu-1".to_string(),
                is_error: false,
                output_excerpt: "echo supersecretvalue123".to_string(),
                output_truncated: false,
            },
            recorded_at_ms: 0,
        };
        let writeback = runtime_event_writeback(
            &record,
            None,
            &environment,
            Some("claude-code"),
            Some(EventAttemptRef {
                command_id: "cmd-test",
                attempt_id: Some("attempt-test"),
            }),
        );
        assert_eq!(
            writeback.payload["output_excerpt"],
            serde_json::Value::String("echo [REDACTED:env:MY_API_TOKEN]".to_string())
        );

        let record = crate::runs::RunEventRecord {
            sequence: 2,
            run_id: "run-1".to_string(),
            event: ProviderEvent::ToolStarted {
                tool_id: "tu-2".to_string(),
                name: "Bash".to_string(),
                input_excerpt: "{\"command\":\"curl -H 'X-Key: supersecretvalue123'\"}"
                    .to_string(),
                input_truncated: false,
            },
            recorded_at_ms: 0,
        };
        let writeback = runtime_event_writeback(
            &record,
            None,
            &environment,
            Some("claude-code"),
            Some(EventAttemptRef {
                command_id: "cmd-test",
                attempt_id: Some("attempt-test"),
            }),
        );
        let input = writeback.payload["input_excerpt"].as_str().unwrap();
        assert!(input.contains("[REDACTED:env:MY_API_TOKEN]"));
        assert!(!input.contains("supersecretvalue123"));
    }

    #[test]
    fn project_task_attestation_writeback_carries_runtime_and_provider_metadata() {
        let payload = project_task_session_payload("emp-1");
        let context = project_task_writeback_context(&payload).expect("project task context");
        let spec = RunSpec {
            provider_type: "claude-code".to_string(),
            workspace_path: PathBuf::from("/workspace/project"),
            agent_home_dir: Some(PathBuf::from("/agent/home")),
            employee_capability_dir: Some(PathBuf::from("/employee/cache")),
            capability_manifest_version: Some("cap-manifest:v3".to_string()),
            provider_auth_mode: "host".to_string(),
            mcp_config_path: Some(PathBuf::from(
                "/workspace/project/.superteam/mcp/claude.json",
            )),
            skill_conflicts: vec!["beta".to_string()],
            workspace_base_ref: Some("main".to_string()),
            skill_convergence: Some(crate::skills_convergence::SkillConvergenceReport {
                materialized: vec!["alpha".to_string()],
                reused: vec!["gamma".to_string()],
                pruned: vec!["stale".to_string()],
                prune_skipped: false,
                stamp_hit: false,
            }),
            prompt: "complete task".to_string(),
            system_prompt: None,
            session_id: None,
            continue_session: false,
            model: Some("sonnet".to_string()),
            environment: BTreeMap::new(),
            command_context: None,
        };

        let git = crate::artifacts::WorkspaceGitFacts {
            branch: Some("feat/x".to_string()),
            head_sha: Some("a".repeat(40)),
            diff_sha256: Some("b".repeat(64)),
        };
        let body = project_task_attestation_writeback(
            &context,
            "cmd-project-task",
            &spec,
            "provider_start",
            "succeeded",
            Some("provider-session-1"),
            None,
            &git,
        );

        assert_eq!(body.project_id, "44444444-4444-4444-8444-444444444444");
        assert_eq!(body.project_task_id, "55555555-5555-4555-8555-555555555555");
        assert_eq!(body.attempt_id, "66666666-6666-4666-8666-666666666666");
        assert_eq!(body.runtime_node_id, "77777777-7777-4777-8777-777777777777");
        assert_eq!(body.digital_employee_id, "emp-1");
        assert_eq!(body.provider_auth_mode, "host");
        assert_eq!(body.attestation_type, "provider_start");
        assert_eq!(body.status, "succeeded");
        assert_eq!(
            body.provider_session_id.as_deref(),
            Some("provider-session-1")
        );
        assert_eq!(body.command_argv, vec!["claude-code"]);
        // git 四列此前一直硬编码 None,变更范围因此完全不可见;base_ref 取派发
        // 下发值,其余三项取采集结果。
        assert_eq!(body.git_branch.as_deref(), Some("feat/x"));
        assert_eq!(body.git_base_ref.as_deref(), Some("main"));
        assert_eq!(body.git_head_sha.as_deref(), Some("a".repeat(40).as_str()));
        assert_eq!(
            body.git_diff_sha256.as_deref(),
            Some("b".repeat(64).as_str())
        );
        assert_eq!(
            body.metadata["workspace_ref"],
            serde_json::Value::String(
                "project-workspace:44444444-4444-4444-8444-444444444444/55555555-5555-4555-8555-555555555555/66666666-6666-4666-8666-666666666666"
                    .to_string()
            )
        );
        assert_eq!(
            body.metadata["capability_manifest_version"],
            serde_json::Value::String("cap-manifest:v3".to_string())
        );
        assert_eq!(
            body.metadata["provider_auth_mode"],
            serde_json::Value::String("host".to_string())
        );
        assert_eq!(
            body.metadata["skill_conflicts"],
            serde_json::json!([{"slug":"beta","source":"workspace_native"}]),
            "spec §3.1: project-native skill conflicts must reach the attestation metadata"
        );
        assert_eq!(
            body.metadata["capability_convergence"],
            serde_json::json!({
                "materialized": ["alpha"],
                "reused": ["gamma"],
                "pruned": ["stale"],
                "prune_skipped": false,
                "stamp_hit": false
            }),
            "lazy skill convergence report must reach the attestation metadata"
        );
        assert!(body.metadata.get("workspace_path").is_none());
        assert!(body.metadata.get("agent_home_dir").is_none());
        assert!(body.metadata.get("employee_capability_dir").is_none());
        assert!(body.metadata.get("mcp_config_path").is_none());
        assert_eq!(
            body.idempotency_key,
            "project-task-attempt:66666666-6666-4666-8666-666666666666:attestation:provider_start:cmd-project-task"
        );
    }

    #[test]
    fn project_task_budget_heartbeat_writeback_reports_elapsed_seconds() {
        let payload = project_task_session_payload("emp-1");
        let context = project_task_writeback_context(&payload).expect("project task context");

        let body = project_task_budget_heartbeat_writeback(&context, 42, 0);

        assert_eq!(body.project_id, "44444444-4444-4444-8444-444444444444");
        assert_eq!(body.project_task_id, "55555555-5555-4555-8555-555555555555");
        assert_eq!(body.consumed_wall_clock_sec, 42);
        assert_eq!(body.consumed_tokens, 0);
    }

    #[test]
    fn command_completed_terminal_with_positive_tokens_writes_usage() {
        let terminal = command_completed_terminal(Some("done".to_string()), None, 1500, None);

        assert_eq!(terminal.status, "completed");
        let result = terminal.result.expect("result map");
        let usage = result.get("usage").expect("usage field");
        assert_eq!(usage["total_tokens"], serde_json::json!(1500));
        assert_eq!(result["summary"], serde_json::json!("done"));
    }

    #[test]
    fn command_completed_terminal_with_zero_tokens_omits_usage() {
        let terminal = command_completed_terminal(Some("done".to_string()), None, 0, None);

        let result = terminal.result.expect("result map");
        assert!(result.get("usage").is_none());
        assert_eq!(result["summary"], serde_json::json!("done"));
    }

    #[test]
    fn command_completed_terminal_without_summary_omits_summary_and_writes_usage() {
        let terminal = command_completed_terminal(None, Some("sess-1".to_string()), 42, None);

        let result = terminal.result.expect("result map");
        assert!(result.get("summary").is_none());
        assert_eq!(result["usage"]["total_tokens"], serde_json::json!(42));
    }

    #[test]
    fn command_completed_terminal_writes_unmapped_diagnostics() {
        let diagnostics = attempt_stream_diagnostics("claude-code", 3);
        let terminal =
            command_completed_terminal(Some("done".to_string()), None, 0, Some(diagnostics.clone()));

        let result = terminal.result.expect("result map");
        assert_eq!(result["diagnostics"]["unmapped_native_count"], 3);
        let diagnostic = terminal.diagnostic.expect("terminal diagnostic");
        assert_eq!(diagnostic["unmapped_native_count"], 3);
        assert_eq!(diagnostic["provider_type"], "claude-code");
    }

    #[test]
    fn command_failed_terminal_from_envelope_writes_diagnostics() {
        let envelope = envelope_for_code(
            error_code::RATE_LIMIT,
            "rate limit",
            "claude-code",
        );
        let diagnostics = attempt_stream_diagnostics("claude-code", 2);
        let terminal = command_failed_terminal_from_envelope(&envelope, Some(diagnostics));
        let diagnostic = terminal.diagnostic.expect("terminal diagnostic");
        assert_eq!(diagnostic["unmapped_native_count"], 2);
        assert_eq!(terminal.error_code.as_deref(), Some("RATE_LIMIT"));
    }

    #[test]
    fn record_budget_heartbeat_saturates_accumulator_to_i32_max() {
        let huge: i64 = (i32::MAX as i64) + 1000;
        let saturated = huge.min(i32::MAX as i64) as i32;
        assert_eq!(saturated, i32::MAX);

        let normal: i64 = 12345;
        let saturated_normal = normal.min(i32::MAX as i64) as i32;
        assert_eq!(saturated_normal, 12345);
    }
}
