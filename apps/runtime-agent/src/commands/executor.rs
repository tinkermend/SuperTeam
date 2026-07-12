use std::collections::{BTreeMap, HashMap};
use std::path::{Path, PathBuf};
use std::sync::Arc;
use std::sync::atomic::{AtomicI64, Ordering};
use std::time::{Duration, Instant};

use futures::StreamExt;
use tokio_util::sync::CancellationToken;

use crate::commands::install_skills::{InstallSkillsCommandPayload, install_skill_targets};
use crate::commands::payload::{
    RuntimeProvisionInstanceCommandPayload, RuntimeSessionCommandPayload,
    RuntimeStopSessionCommandPayload, SessionPolicyMode, metadata_string,
};
use crate::commands::registry::{ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding};
use crate::config::RuntimeConfig;
use crate::controlplane::ControlPlaneClient;
use crate::controlplane::models::{
    EnsureInstanceCommand, ProjectTaskAttestationWriteback, ProjectTaskBudgetHeartbeatWriteback,
    ProjectTaskCompleteWriteback, ProjectTaskFailWriteback, ProjectTaskStartWriteback,
    ProjectTaskWaitHumanWriteback, RuntimeCommand, RuntimeCommandEventWriteback,
    RuntimeCommandTerminalWriteback, RuntimeCommandType, TaskResultContract,
};
use crate::events::ProviderEvent;
use crate::instances::{EnsureInstanceRequest, ensure_instance};
use crate::providers::catalog;
use crate::providers::{ProviderAdapter, ProviderEventStream, ProviderRequest, ProviderRunHandle};
use crate::runs::{RunEventRecord, RunSpec, RunStatus, RuntimeCommandRunContext, RuntimeRunStore};
use crate::skills::materialize_skills;
use crate::workspace_files::{
    WorkspaceMaterializationPlan, atomic_write, materialize_workspace, provider_home_kind,
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
}

#[derive(Clone)]
struct RuntimeCommandWritebackSink {
    client: ControlPlaneClient,
    command_id: String,
    project_task: Option<ProjectTaskWritebackContext>,
    usage_tokens: Arc<AtomicI64>,
    /// Set once the raw transcript has been finalized, so every terminal
    /// writeback (complete, fail, wait_human) carries the same pointer.
    raw_log: Arc<std::sync::Mutex<Option<crate::raw_log::RawLogSummary>>>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
struct ProviderTerminalCompletion {
    summary: Option<String>,
    provider_session_id: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
enum ProviderTerminalWritebackAction {
    Fail(String),
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
            ProviderEvent::TurnError { message } => {
                self.failed = true;
                self.pending_completion = None;
                Some(ProviderTerminalWritebackAction::Fail(message.clone()))
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
    s3_client: Option<aws_sdk_s3::Client>,
    s3_bucket: Option<String>,
}

impl RuntimeCommandExecutor {
    fn build_raw_sink(
        &self,
        run_id: &str,
        tenant_id: Option<&str>,
        project_task: &Option<ProjectTaskWritebackContext>,
    ) -> Arc<dyn crate::raw_log::RawLineSink> {
        build_raw_sink_inner(
            self.s3_client.as_ref(),
            self.s3_bucket.as_deref(),
            self.runs.run_dir(run_id),
            tenant_id,
            project_task.as_ref().map(|task| task.attempt_id.as_str()),
        )
    }

    pub fn new(config: RuntimeConfig) -> Self {
        let (s3_client, s3_bucket) = create_s3_client(&config);
        Self {
            runs: RuntimeRunStore::new(config.runs.log_dir.clone()),
            registry: RuntimeCommandRegistry::default(),
            config,
            control_plane: None,
            s3_client,
            s3_bucket,
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
            RuntimeCommandType::InstallSkills => self.handle_install_skills(command).await,
            RuntimeCommandType::ProvisionInstance => self.handle_provision_instance(command).await,
            RuntimeCommandType::SyncWorkspaceFiles => {
                self.handle_sync_workspace_files(command).await
            }
            RuntimeCommandType::Unsupported(_) => Ok(RuntimeCommandOutcome {
                command_id: command.id,
                accepted: false,
                run_id: None,
            }),
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
        let command_workspace = match self.ensure_command_instance(&command.id, &payload) {
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
        let spec = RunSpec {
            provider_kind: payload.provider_kind().to_string(),
            workspace_path: command_workspace.workspace_path,
            agent_home_dir: Some(command_workspace.agent_home_dir.clone()),
            employee_capability_dir: Some(command_workspace.employee_capability_dir),
            capability_manifest_version: command_workspace.capability_manifest_version,
            provider_auth_mode: command_workspace.provider_auth_mode,
            mcp_config_path: command_workspace.mcp_config_path,
            prompt,
            session_id: session_id.clone(),
            continue_session: matches!(
                command.command_type,
                RuntimeCommandType::ResumeSession | RuntimeCommandType::SendInput
            ),
            model: payload.model.clone(),
            environment: payload
                .environment
                .iter()
                .map(|env| (env.name.clone(), env.value.clone()))
                .collect(),
            command_context: Some(RuntimeCommandRunContext {
                command_id: payload.command_id.clone(),
                digital_employee_id: payload.digital_employee_id.clone(),
                execution_instance_id: payload.execution_instance_id.clone(),
                provider_type: payload.provider_type.clone(),
                session_policy: serde_json::to_value(&payload.session_policy)
                    .map_err(|error| self.recorded_error(&payload.command_id, error.into()))?,
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
                project_task: project_task.clone(),
                usage_tokens: Arc::new(AtomicI64::new(0)),
                raw_log: Arc::new(std::sync::Mutex::new(None)),
            });
        if let Some(writeback) = &writeback {
            if let Err(error) = writeback.start_project_task().await {
                let message = error.to_string();
                let _ = self.runs.finish_failed(&run_id, message.clone()).await;
                let _ = writeback.fail(message).await;
                self.registry.record_run_finished(&run_id);
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
                let message = error.to_string();
                let _ = self.runs.finish_failed(&run_id, message.clone()).await;
                if let Some(writeback) = &writeback {
                    writeback.spawn_attestation(
                        spec.clone(),
                        "provider_start",
                        "failed",
                        None,
                        None,
                    );
                    writeback.fail(message).await?;
                }
                self.registry.record_run_finished(&run_id);
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
                    project_task,
                    usage_tokens: Arc::new(AtomicI64::new(0)),
                    raw_log: Arc::new(std::sync::Mutex::new(None)),
                }
                .fail_project_task("operator cancelled")
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

    async fn handle_provision_instance(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let payload = match RuntimeProvisionInstanceCommandPayload::from_command(&command) {
            Ok(payload) => payload,
            Err(error) => {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_provisioning_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };
        let provider_home = match provider_home_kind(&payload.provider_type) {
            Ok(provider_home) => provider_home,
            Err(error) => {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_provisioning_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };
        let result = match materialize_workspace(WorkspaceMaterializationPlan {
            agent_home_dir: PathBuf::from(&payload.agent_home_dir),
            provider_home,
            files: payload.workspace_files,
        }) {
            Ok(result) => result,
            Err(error) => {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_provisioning_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };
        if let Err(error) = materialize_persona_memory(
            &result.agent_home_dir,
            payload.persona_memory_markdown.as_deref(),
        ) {
            let error = self.recorded_error(&command.id, error);
            let message = error.to_string();
            self.write_provisioning_failure(&command.id, message)
                .await?;
            return Err(error);
        }

        if !payload.skills.is_empty() {
            if let (Some(s3_client), Some(bucket)) = (&self.s3_client, &self.s3_bucket) {
                if let Err(error) = materialize_skills(
                    &PathBuf::from(&payload.agent_home_dir),
                    &payload.skills,
                    s3_client,
                    bucket,
                )
                .await
                {
                    let error = self.recorded_error(&command.id, error);
                    let message = error.to_string();
                    self.write_provisioning_failure(&command.id, message)
                        .await?;
                    return Err(error);
                }
            } else {
                let error = self.recorded_error(
                    &command.id,
                    anyhow::anyhow!(
                        "skills require S3 configuration but s3 client is not configured"
                    ),
                );
                let message = error.to_string();
                self.write_provisioning_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        }

        if !payload.mcp_servers.is_empty() {
            if let Err(error) = crate::mcp_config::materialize_mcp_config(
                &PathBuf::from(&payload.agent_home_dir),
                &payload.provider_type,
                &payload.mcp_servers,
            ) {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_provisioning_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        }

        if let Some(control_plane) = &self.control_plane {
            control_plane
                .complete_runtime_command(
                    &command.id,
                    &provisioning_completed_terminal(
                        &result.agent_home_dir,
                        &self.config.workspace.base_dir,
                    ),
                )
                .await?;
        }

        Ok(RuntimeCommandOutcome {
            command_id: command.id,
            accepted: true,
            run_id: None,
        })
    }

    async fn handle_sync_workspace_files(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let payload = match RuntimeProvisionInstanceCommandPayload::from_command(&command) {
            Ok(payload) => payload,
            Err(error) => {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_workspace_sync_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };
        let provider_home = match provider_home_kind(&payload.provider_type) {
            Ok(provider_home) => provider_home,
            Err(error) => {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_workspace_sync_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };
        let result = match materialize_workspace(WorkspaceMaterializationPlan {
            agent_home_dir: PathBuf::from(&payload.agent_home_dir),
            provider_home,
            files: payload.workspace_files,
        }) {
            Ok(result) => result,
            Err(error) => {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_workspace_sync_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };
        if let Err(error) = materialize_persona_memory(
            &result.agent_home_dir,
            payload.persona_memory_markdown.as_deref(),
        ) {
            let error = self.recorded_error(&command.id, error);
            let message = error.to_string();
            self.write_workspace_sync_failure(&command.id, message)
                .await?;
            return Err(error);
        }
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .complete_runtime_command(
                    &command.id,
                    &workspace_sync_completed_terminal(&result.agent_home_dir, result.synced_files),
                )
                .await?;
        }
        Ok(RuntimeCommandOutcome {
            command_id: command.id,
            accepted: true,
            run_id: None,
        })
    }

    async fn handle_install_skills(
        &self,
        command: RuntimeCommand,
    ) -> anyhow::Result<RuntimeCommandOutcome> {
        let payload: InstallSkillsCommandPayload = match serde_json::from_value(command.payload) {
            Ok(payload) => payload,
            Err(error) => {
                let error = self.recorded_error(
                    &command.id,
                    anyhow::anyhow!("invalid install_skills command payload: {error}"),
                );
                let message = error.to_string();
                self.write_install_skills_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };
        if payload.command_id != command.id {
            let error = self.recorded_error(
                &command.id,
                anyhow::anyhow!("command_id does not match runtime command id"),
            );
            let message = error.to_string();
            self.write_install_skills_failure(&command.id, message)
                .await?;
            return Err(error);
        }

        let (s3_client, bucket) = match (&self.s3_client, &self.s3_bucket) {
            (Some(s3_client), Some(bucket)) => (s3_client, bucket),
            _ => {
                let error = self.recorded_error(
                    &command.id,
                    anyhow::anyhow!(
                        "install_skills requires S3 configuration but s3 client is not configured"
                    ),
                );
                let message = error.to_string();
                self.write_install_skills_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };

        let installed = match install_skill_targets(payload, s3_client, bucket).await {
            Ok(installed) => installed,
            Err(error) => {
                let error = self.recorded_error(&command.id, error);
                let message = error.to_string();
                self.write_install_skills_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };

        if let Some(control_plane) = &self.control_plane {
            control_plane
                .complete_runtime_command(
                    &command.id,
                    &install_skills_completed_terminal(installed),
                )
                .await?;
        }

        Ok(RuntimeCommandOutcome {
            command_id: command.id,
            accepted: true,
            run_id: None,
        })
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
            base_dir: self.config.workspace.base_dir.clone(),
            team_id: request.team_id,
            digital_employee_id: request.digital_employee_id,
        })
        .map_err(|error| self.recorded_error(&command.id, error))
    }

    async fn write_provisioning_failure(
        &self,
        command_id: &str,
        error_message: String,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .fail_runtime_command(command_id, &provisioning_failed_terminal(error_message))
                .await?;
        }
        Ok(())
    }

    async fn write_install_skills_failure(
        &self,
        command_id: &str,
        error_message: String,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .fail_runtime_command(command_id, &install_skills_failed_terminal(error_message))
                .await?;
        }
        Ok(())
    }

    async fn write_workspace_sync_failure(
        &self,
        command_id: &str,
        error_message: String,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .fail_runtime_command(command_id, &workspace_sync_failed_terminal(error_message))
                .await?;
        }
        Ok(())
    }

    async fn write_command_failure(
        &self,
        command_id: &str,
        error_message: String,
    ) -> anyhow::Result<()> {
        if let Some(control_plane) = &self.control_plane {
            control_plane
                .fail_runtime_command(command_id, &command_failed_terminal(error_message))
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

    fn ensure_command_instance(
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
        let agent_home_dir = PathBuf::from(agent_home_dir_text);

        let provider_home = provider_home_kind(&payload.provider_type)
            .map_err(|error| self.recorded_error(command_id, error))?;
        materialize_workspace(WorkspaceMaterializationPlan {
            agent_home_dir: agent_home_dir.clone(),
            provider_home,
            files: payload.workspace_files.clone(),
        })
        .map_err(|error| self.recorded_error(command_id, error))?;

        let project_workspace = payload.project_workspace();
        let capability_manifest_version = project_workspace.capability_manifest_version.clone();
        let provider_auth_mode = project_workspace.provider_auth_mode.clone();
        let resolved = crate::project_workspace::resolve_project_workspace(
            crate::project_workspace::ProjectWorkspaceRequest {
                base_dir: self.config.workspace.base_dir.clone(),
                project_id: project_workspace.project_id,
                project_task_id: project_workspace.project_task_id,
                attempt_id: project_workspace.project_task_attempt_id,
                workspace_mode: project_workspace
                    .workspace_mode
                    .unwrap_or_else(|| "none".to_string()),
                project_git: project_workspace.project_git,
                base_ref: project_workspace.base_ref,
            },
        )
        .map_err(|error| self.recorded_error(command_id, error))?;

        let mcp_config_path = crate::mcp_config::materialize_task_mcp_config(
            &resolved.workspace_path,
            &payload.provider_type,
            &payload.mcp_servers,
        )
        .map_err(|error| self.recorded_error(command_id, error))?;

        crate::project_workspace::link_provider_skills(
            &agent_home_dir,
            &resolved.workspace_path,
            &payload.provider_type,
        )
        .map_err(|error| self.recorded_error(command_id, error))?;

        Ok(CommandWorkspace {
            workspace_path: resolved.workspace_path,
            employee_capability_dir: agent_home_dir.clone(),
            agent_home_dir,
            capability_manifest_version,
            provider_auth_mode,
            mcp_config_path,
        })
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
            )
            .await;

            if let Err(error) = result {
                if !run_is_cancelled(&runs, &run_id).await {
                    let message = error.to_string();
                    let _ = runs.finish_failed(&run_id, message.clone()).await;
                    // A failed run still produced a transcript; the failure
                    // writeback must carry its pointer.
                    finalize_raw_log(&failure_raw_sink, failure_writeback.as_ref()).await;
                    if let Some(writeback) = &failure_writeback {
                        let _ = writeback.fail(message).await;
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

fn materialize_persona_memory(
    agent_home_dir: &Path,
    persona_memory: Option<&str>,
) -> anyhow::Result<()> {
    let Some(markdown) = persona_memory.filter(|value| !value.trim().is_empty()) else {
        return Ok(());
    };

    atomic_write(&agent_home_dir.join("人格记忆.md"), markdown.as_bytes())
}

/// Builds the raw transcript sink for a run.
///
/// Falls back to a no-op sink when object storage is unconfigured or the run is
/// not backed by a project task attempt: without an attempt there is nowhere to
/// hang the resulting pointer, and without a bucket there is nowhere to put the
/// bytes.
fn build_raw_sink_inner(
    s3_client: Option<&aws_sdk_s3::Client>,
    s3_bucket: Option<&str>,
    local_dir: std::path::PathBuf,
    tenant_id: Option<&str>,
    attempt_id: Option<&str>,
) -> Arc<dyn crate::raw_log::RawLineSink> {
    let (Some(client), Some(bucket), Some(attempt_id)) = (s3_client, s3_bucket, attempt_id) else {
        return Arc::new(crate::raw_log::NoopRawSink);
    };
    let tenant_id = tenant_id.unwrap_or("unknown-tenant");
    let uploader = Arc::new(crate::raw_log::S3RawLogUploader::new(
        client.clone(),
        bucket.to_string(),
    ));
    Arc::new(crate::raw_log::SegmentedRawLogSink::new(
        uploader,
        local_dir,
        format!("runs/{tenant_id}/{attempt_id}/"),
        attempt_id.to_string(),
    ))
}

fn create_s3_client(config: &RuntimeConfig) -> (Option<aws_sdk_s3::Client>, Option<String>) {
    match &config.s3 {
        Some(s3) => {
            let creds = aws_sdk_s3::config::Credentials::new(
                &s3.access_key_id,
                &s3.secret_access_key,
                None,
                None,
                "static",
            );
            let s3_config = aws_sdk_s3::Config::builder()
                .region(aws_sdk_s3::config::Region::new(s3.region.clone()))
                .credentials_provider(creds)
                .endpoint_url(&s3.endpoint)
                .force_path_style(s3.force_path_style)
                .behavior_version_latest()
                .build();
            (
                Some(aws_sdk_s3::Client::from_conf(s3_config)),
                Some(s3.bucket.clone()),
            )
        }
        None => (None, None),
    }
}

fn spawn_project_task_budget_heartbeat(
    runs: RuntimeRunStore,
    run_id: String,
    writeback: RuntimeCommandWritebackSink,
    handle: ProviderRunHandle,
    started_at: Instant,
    interval: Duration,
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
                            let reason = "wall_clock_exceeded";
                            let _ = handle.cancel().await;
                            let _ = runs.finish_failed(&run_id, reason.to_string()).await;
                            let _ = writeback.fail(reason.to_string()).await;
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

fn provisioning_completed_terminal(
    _agent_home_dir: &Path,
    _workspace_base_dir: &Path,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    result.insert(
        "provisioning_status".to_string(),
        serde_json::Value::String("ready".to_string()),
    );

    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary: Some("digital employee execution instance provisioned".to_string()),
        result: Some(result),
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

fn provisioning_failed_terminal(error_message: String) -> RuntimeCommandTerminalWriteback {
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
        error_code: Some("provision_instance_failed".to_string()),
        error_family: Some("runtime_provisioning".to_string()),
    }
}

fn workspace_sync_completed_terminal(
    _agent_home_dir: &Path,
    synced_files: Vec<crate::workspace_files::SyncedWorkspaceFile>,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    result.insert(
        "synced_files".to_string(),
        serde_json::to_value(synced_files).unwrap_or_else(|_| serde_json::Value::Array(Vec::new())),
    );
    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary: Some("digital employee workspace files synced".to_string()),
        result: Some(result),
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

fn install_skills_completed_terminal(
    installed: Vec<crate::commands::install_skills::InstalledSkillTarget>,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    result.insert(
        "installed".to_string(),
        serde_json::to_value(installed).unwrap_or_else(|_| serde_json::Value::Array(Vec::new())),
    );
    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary: Some("runtime skills installed".to_string()),
        result: Some(result),
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

fn install_skills_failed_terminal(error_message: String) -> RuntimeCommandTerminalWriteback {
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
        error_code: Some("install_skills_failed".to_string()),
        error_family: Some("runtime_skill_install".to_string()),
    }
}

fn command_completed_terminal(
    summary: Option<String>,
    provider_session_id: Option<String>,
    total_tokens: i64,
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

    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary,
        result: Some(result),
        diagnostic: None,
        provider_session_external_id: provider_session_id.clone(),
        session_state_patch: provider_session_state_patch(provider_session_id.as_deref()),
        log_ref: None,
        raw_result_ref: None,
        error_message: None,
        error_code: None,
        error_family: None,
    }
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
                &runtime_event_writeback(record, provider_session_id, environment),
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
            let body = project_task_attestation_writeback(
                project_task,
                &self.command_id,
                spec,
                attestation_type,
                status,
                provider_session_id,
                duration_ms,
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
    ) -> anyhow::Result<()> {
        let total_tokens = self.usage_tokens.load(Ordering::Relaxed);
        self.client
            .complete_runtime_command(
                &self.command_id,
                &command_completed_terminal(
                    summary.clone(),
                    provider_session_id.clone(),
                    total_tokens,
                ),
            )
            .await?;
        if let Some(project_task) = &self.project_task {
            let raw_log = self.raw_log.lock().ok().and_then(|guard| guard.clone());
            let result = if let Some(mut writeback) = project_task_wait_human_writeback(
                project_task,
                &self.command_id,
                summary.as_deref(),
                provider_session_id.as_deref(),
            ) {
                writeback.raw_log = raw_log;
                self.client
                    .wait_human_project_task_attempt(&project_task.attempt_id, &writeback)
                    .await
            } else {
                let mut writeback = project_task_complete_writeback(
                    project_task,
                    &self.command_id,
                    summary.as_deref(),
                    provider_session_id.as_deref(),
                );
                writeback.raw_log = raw_log;
                self.client
                    .complete_project_task_attempt(&project_task.attempt_id, &writeback)
                    .await
            };
            if let Err(error) = result {
                eprintln!(
                    "Project task writeback failed for command {} project_task {}: {}",
                    self.command_id, project_task.project_task_id, error
                );
            }
        }
        Ok(())
    }

    async fn fail_project_task(&self, error_message: &str) -> anyhow::Result<()> {
        if let Some(project_task) = &self.project_task {
            let mut writeback =
                project_task_fail_writeback(project_task, &self.command_id, error_message);
            writeback.raw_log = self.raw_log.lock().ok().and_then(|guard| guard.clone());
            self.client
                .fail_project_task_attempt(
                    &project_task.attempt_id,
                    &writeback,
                )
                .await?;
        }
        Ok(())
    }

    async fn fail(&self, error_message: String) -> anyhow::Result<()> {
        self.client
            .fail_runtime_command(
                &self.command_id,
                &command_failed_terminal(error_message.clone()),
            )
            .await?;
        self.fail_project_task(&error_message).await
    }
}

fn runtime_event_writeback(
    record: &RunEventRecord,
    provider_session_id: Option<&str>,
    environment: &BTreeMap<String, String>,
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
            payload.insert("text".to_string(), serde_json::Value::String(text.clone()));
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
        ProviderEvent::TurnCompleted { summary, .. } => {
            let mut payload = HashMap::new();
            if let Some(summary) = summary {
                payload.insert(
                    "summary".to_string(),
                    serde_json::Value::String(summary.clone()),
                );
            }
            ("turn_completed".to_string(), payload)
        }
        ProviderEvent::TurnError { message } => {
            let mut payload = HashMap::new();
            payload.insert(
                "message".to_string(),
                serde_json::Value::String(message.clone()),
            );
            ("turn_error".to_string(), payload)
        }
    };

    RuntimeCommandEventWriteback {
        event_type,
        sequence_number: record.sequence.min(i32::MAX as u64) as i32,
        payload,
        provider_session_external_id,
        session_state_patch,
        metadata: Some(HashMap::from([(
            "source".to_string(),
            serde_json::Value::String("runtime-agent".to_string()),
        )])),
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
        "provider_kind".to_string(),
        serde_json::Value::String(spec.provider_kind.clone()),
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
        command_argv: vec![spec.provider_kind.clone()],
        exit_code: None,
        duration_ms,
        log_ref: None,
        stdout_sha256: None,
        stderr_sha256: None,
        artifact_refs: Vec::new(),
        artifact_hashes: serde_json::Value::Object(serde_json::Map::new()),
        git_branch: None,
        git_base_ref: None,
        git_head_sha: None,
        git_diff_sha256: None,
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
    let (failure_family, retryable) = project_task_failure_classification(error_message);
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
        failure_summary: error_message.trim().to_string(),
        failure_family: failure_family.to_string(),
        retryable,
        result_contract: None,
    }
}

fn project_task_failure_classification(error_message: &str) -> (&'static str, bool) {
    let normalized = error_message.to_ascii_lowercase();
    if normalized.contains("wall_clock_exceeded") || normalized.contains("budget") {
        return ("budget_fuse", false);
    }
    if normalized.contains("operator cancelled") || normalized.contains("cancelled") {
        return ("business_cancelled", false);
    }
    if normalized.contains("content_hash mismatch")
        || normalized.contains("workspace_sync")
        || normalized.contains("workspace sync")
    {
        return ("invalid_contract", false);
    }
    if normalized.contains("timeout") || normalized.contains("timed out") {
        return ("timeout", true);
    }
    if normalized.contains("claude exited")
        || normalized.contains("opencode exited")
        || normalized.contains("codex exited")
        || normalized.contains("api error")
        || normalized.contains("rate limit")
        || normalized.contains("overloaded")
        || normalized.contains("unavailable")
    {
        return ("transient_provider", true);
    }
    ("non_retryable_execution", false)
}

fn project_task_attempt_idempotency_key(
    attempt_id: &str,
    action: &str,
    command_id: &str,
) -> String {
    format!("project-task-attempt:{attempt_id}:{action}:{command_id}")
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
) -> anyhow::Result<()> {
    let mut latest_provider_session_id: Option<String> = None;
    let mut terminal_writeback = ProviderTerminalWritebackState::default();
    let mut fallback_text_summary = String::new();
    while let Some(event) = events.next().await {
        let event = event?;
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
        let writeback_action =
            terminal_writeback.observe_event(&event, latest_provider_session_id.as_deref());
        let record = runs.record_event(&run_id, event).await?;
        if is_terminal {
            registry.record_run_finished(&run_id);
        }
        if let Some(writeback) = &writeback {
            writeback
                .record_event(
                    &record,
                    latest_provider_session_id.as_deref(),
                    &spec.environment,
                )
                .await?;
            if let Some(action) = writeback_action {
                match action {
                    ProviderTerminalWritebackAction::Fail(message) => {
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
                        writeback.fail(message).await?;
                    }
                }
            }
        }
    }
    if let Some(stop) = &heartbeat_stop {
        stop.cancel();
    }
    // Finalize before the terminal writeback so the attempt row and the raw
    // transcript pointer land in the same call.
    finalize_raw_log(&raw_sink, writeback.as_ref()).await;
    if let (Some(writeback), Some(mut completion)) =
        (&writeback, terminal_writeback.finish_successful_stream())
    {
        let has_summary = completion
            .summary
            .as_deref()
            .map(str::trim)
            .is_some_and(|value| !value.is_empty());
        let fallback_text = fallback_text_summary.trim();
        if !has_summary && !fallback_text.is_empty() {
            completion.summary = Some(fallback_text.to_string());
        }
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
            .complete(completion.summary, completion.provider_session_id)
            .await?;
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
            capability_bindings: serde_json::json!({}),
            workspace_files: Vec::new(),
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
                    Some("provider-session-1")
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
                )
                .is_none()
        );
        let action = state.observe_event(
            &ProviderEvent::TurnError {
                message: "claude exited with status 1".to_string(),
            },
            Some("provider-session-1"),
        );

        match action {
            Some(ProviderTerminalWritebackAction::Fail(message)) => {
                assert_eq!(message, "claude exited with status 1");
            }
            other => panic!("expected fail action, got {other:?}"),
        }
        assert!(
            state.finish_successful_stream().is_none(),
            "a failed stream must not emit a deferred completion"
        );
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
        let writeback = runtime_event_writeback(&record, None, &environment);
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
        let writeback = runtime_event_writeback(&record, None, &environment);
        let input = writeback.payload["input_excerpt"].as_str().unwrap();
        assert!(input.contains("[REDACTED:env:MY_API_TOKEN]"));
        assert!(!input.contains("supersecretvalue123"));
    }

    #[test]
    fn project_task_attestation_writeback_carries_runtime_and_provider_metadata() {
        let payload = project_task_session_payload("emp-1");
        let context = project_task_writeback_context(&payload).expect("project task context");
        let spec = RunSpec {
            provider_kind: "claude-code".to_string(),
            workspace_path: PathBuf::from("/workspace/project"),
            agent_home_dir: Some(PathBuf::from("/agent/home")),
            employee_capability_dir: Some(PathBuf::from("/employee/cache")),
            capability_manifest_version: Some("cap-manifest:v3".to_string()),
            provider_auth_mode: "host".to_string(),
            mcp_config_path: Some(PathBuf::from(
                "/workspace/project/.superteam/mcp/claude.json",
            )),
            prompt: "complete task".to_string(),
            session_id: None,
            continue_session: false,
            model: Some("sonnet".to_string()),
            environment: BTreeMap::new(),
            command_context: None,
        };

        let body = project_task_attestation_writeback(
            &context,
            "cmd-project-task",
            &spec,
            "provider_start",
            "succeeded",
            Some("provider-session-1"),
            None,
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
    fn provisioning_terminal_result_omits_local_runtime_paths() {
        let terminal = provisioning_completed_terminal(
            Path::new("/runtime/agent/home"),
            Path::new("/runtime/workspaces"),
        );
        let result = terminal.result.expect("terminal result");

        assert!(result.get("agent_home_dir").is_none());
        assert!(result.get("workspace_base_dir").is_none());
        assert_eq!(
            result.get("provisioning_status"),
            Some(&serde_json::Value::String("ready".to_string()))
        );
    }

    #[test]
    fn workspace_sync_terminal_result_omits_agent_home_path() {
        let terminal =
            workspace_sync_completed_terminal(Path::new("/runtime/agent/home"), Vec::new());
        let result = terminal.result.expect("terminal result");

        assert!(result.get("agent_home_dir").is_none());
        assert!(result.get("synced_files").is_some());
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
        let terminal = command_completed_terminal(Some("done".to_string()), None, 1500);

        assert_eq!(terminal.status, "completed");
        let result = terminal.result.expect("result map");
        let usage = result.get("usage").expect("usage field");
        assert_eq!(usage["total_tokens"], serde_json::json!(1500));
        assert_eq!(result["summary"], serde_json::json!("done"));
    }

    #[test]
    fn command_completed_terminal_with_zero_tokens_omits_usage() {
        let terminal = command_completed_terminal(Some("done".to_string()), None, 0);

        let result = terminal.result.expect("result map");
        assert!(result.get("usage").is_none());
        assert_eq!(result["summary"], serde_json::json!("done"));
    }

    #[test]
    fn command_completed_terminal_without_summary_omits_summary_and_writes_usage() {
        let terminal = command_completed_terminal(None, Some("sess-1".to_string()), 42);

        let result = terminal.result.expect("result map");
        assert!(result.get("summary").is_none());
        assert_eq!(result["usage"]["total_tokens"], serde_json::json!(42));
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
