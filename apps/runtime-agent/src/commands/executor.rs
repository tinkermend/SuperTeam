use std::collections::HashMap;
use std::path::{Path, PathBuf};

use futures::StreamExt;

use crate::commands::payload::{
    RuntimeProvisionInstanceCommandPayload, RuntimeSessionCommandPayload,
    RuntimeStopSessionCommandPayload, SessionPolicyMode,
};
use crate::commands::registry::{ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding};
use crate::config::RuntimeConfig;
use crate::controlplane::ControlPlaneClient;
use crate::controlplane::models::{
    EnsureInstanceCommand, ProjectTaskCompleteWriteback, ProjectTaskFailWriteback,
    ProjectTaskStartWriteback, ProjectTaskWaitHumanWriteback, RuntimeCommand,
    RuntimeCommandEventWriteback, RuntimeCommandTerminalWriteback, RuntimeCommandType,
    TaskResultContract,
};
use crate::events::ProviderEvent;
use crate::instances::{EnsureInstanceRequest, ensure_instance};
use crate::providers::catalog;
use crate::providers::{ProviderAdapter, ProviderEventStream, ProviderRequest};
use crate::runs::{RunEventRecord, RunSpec, RunStatus, RuntimeCommandRunContext, RuntimeRunStore};
use crate::skills::materialize_skills;
use crate::workspace_files::{
    WorkspaceMaterializationPlan, materialize_workspace, provider_home_kind,
};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeCommandOutcome {
    pub command_id: String,
    pub accepted: bool,
    pub run_id: Option<String>,
}

#[derive(Clone)]
struct RuntimeCommandWritebackSink {
    client: ControlPlaneClient,
    command_id: String,
    project_task: Option<ProjectTaskWritebackContext>,
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
    failed: bool,
}

impl ProviderTerminalWritebackState {
    fn observe_event(
        &mut self,
        event: &ProviderEvent,
        provider_session_id: Option<&str>,
    ) -> Option<ProviderTerminalWritebackAction> {
        match event {
            ProviderEvent::TurnCompleted { summary } => {
                if !self.failed {
                    self.pending_completion = Some(ProviderTerminalCompletion {
                        summary: summary.clone(),
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
            self.pending_completion
        }
    }
}

#[derive(Clone, Debug)]
struct ProjectTaskWritebackContext {
    project_task_id: String,
    attempt_id: String,
    lease_token: String,
    runtime_node_id: String,
    digital_employee_id: String,
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
        let workspace_path = match self.ensure_command_instance(&command.id, &payload) {
            Ok(workspace_path) => workspace_path,
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
            workspace_path,
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
        let provider_run = match provider.start(provider_request(&spec)).await {
            Ok(provider_run) => provider_run,
            Err(error) => {
                let message = error.to_string();
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
        };

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
        self.spawn_provider_event_drain(
            run_id.clone(),
            provider_run.events,
            reusable_provider_session,
            writeback,
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
    ) -> anyhow::Result<PathBuf> {
        let agent_home_dir_text = payload
            .agent_home_dir
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .ok_or_else(|| {
                self.recorded_error(command_id, anyhow::anyhow!("agent_home_dir is required"))
            })?;
        let agent_home_dir = PathBuf::from(agent_home_dir_text);
        if !agent_home_dir.exists() {
            return Err(self.recorded_error(
                command_id,
                anyhow::anyhow!(
                    "agent_home_dir does not exist: {}",
                    agent_home_dir.display()
                ),
            ));
        }

        let provider_home = provider_home_kind(&payload.provider_type)
            .map_err(|error| self.recorded_error(command_id, error))?;
        materialize_workspace(WorkspaceMaterializationPlan {
            agent_home_dir: agent_home_dir.clone(),
            provider_home,
            files: payload.workspace_files.clone(),
        })
        .map(|_| agent_home_dir)
        .map_err(|error| self.recorded_error(command_id, error))
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
    ) {
        let runs = self.runs.clone();
        let registry = self.registry.clone();
        let failure_writeback = writeback.clone();
        tokio::spawn(async move {
            let result = drain_provider_events(
                runs.clone(),
                registry.clone(),
                run_id.clone(),
                events,
                reusable_provider_session,
                writeback,
            )
            .await;

            if let Err(error) = result {
                if !run_is_cancelled(&runs, &run_id).await {
                    let message = error.to_string();
                    let _ = runs.finish_failed(&run_id, message.clone()).await;
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

fn provisioning_completed_terminal(
    agent_home_dir: &Path,
    workspace_base_dir: &Path,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    result.insert(
        "agent_home_dir".to_string(),
        serde_json::Value::String(path_to_string(agent_home_dir)),
    );
    result.insert(
        "workspace_base_dir".to_string(),
        serde_json::Value::String(path_to_string(workspace_base_dir)),
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
    agent_home_dir: &Path,
    synced_files: Vec<crate::workspace_files::SyncedWorkspaceFile>,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    result.insert(
        "agent_home_dir".to_string(),
        serde_json::Value::String(path_to_string(agent_home_dir)),
    );
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

fn command_completed_terminal(
    summary: Option<String>,
    provider_session_id: Option<String>,
) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    if let Some(summary) = summary.as_ref().filter(|value| !value.trim().is_empty()) {
        result.insert(
            "summary".to_string(),
            serde_json::Value::String(summary.clone()),
        );
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
    ) -> anyhow::Result<()> {
        self.client
            .record_runtime_command_event(
                &self.command_id,
                &runtime_event_writeback(record, provider_session_id),
            )
            .await
    }

    async fn complete(
        &self,
        summary: Option<String>,
        provider_session_id: Option<String>,
    ) -> anyhow::Result<()> {
        self.client
            .complete_runtime_command(
                &self.command_id,
                &command_completed_terminal(summary.clone(), provider_session_id.clone()),
            )
            .await?;
        if let Some(project_task) = &self.project_task {
            let result = if let Some(writeback) = project_task_wait_human_writeback(
                project_task,
                &self.command_id,
                summary.as_deref(),
                provider_session_id.as_deref(),
            ) {
                self.client
                    .wait_human_project_task_attempt(&project_task.attempt_id, &writeback)
                    .await
            } else {
                self.client
                    .complete_project_task_attempt(
                        &project_task.attempt_id,
                        &project_task_complete_writeback(
                            project_task,
                            &self.command_id,
                            summary.as_deref(),
                            provider_session_id.as_deref(),
                        ),
                    )
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
            self.client
                .fail_project_task_attempt(
                    &project_task.attempt_id,
                    &project_task_fail_writeback(project_task, &self.command_id, error_message),
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
        ProviderEvent::ToolStarted { tool_id, name } => {
            let mut payload = HashMap::new();
            payload.insert(
                "tool_id".to_string(),
                serde_json::Value::String(tool_id.clone()),
            );
            payload.insert("name".to_string(), serde_json::Value::String(name.clone()));
            ("tool_started".to_string(), payload)
        }
        ProviderEvent::ToolCompleted { tool_id } => {
            let mut payload = HashMap::new();
            payload.insert(
                "tool_id".to_string(),
                serde_json::Value::String(tool_id.clone()),
            );
            ("tool_completed".to_string(), payload)
        }
        ProviderEvent::TurnCompleted { summary } => {
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
    Some(ProjectTaskWritebackContext {
        project_task_id: project_task_id.to_string(),
        attempt_id: attempt_id.to_string(),
        lease_token: lease_token.to_string(),
        runtime_node_id: runtime_node_id.to_string(),
        digital_employee_id: digital_employee_id.to_string(),
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
    let result_contract =
        parsed_result_contract(parsed.as_ref(), &conclusion, &evidence_refs, &artifact_refs);
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
) -> Option<TaskResultContract> {
    if let Some(contract) = value
        .and_then(|value| value.get("result_contract"))
        .and_then(serde_json::Value::as_object)
    {
        return Some(TaskResultContract {
            status: contract
                .get("status")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("completed")
                .to_string(),
            summary: contract
                .get("summary")
                .and_then(serde_json::Value::as_str)
                .unwrap_or(fallback_summary)
                .to_string(),
            acceptance_results: contract
                .get("acceptance_results")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            evidence_refs: contract
                .get("evidence_refs")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_else(|| normalized_result_refs(evidence_refs, "evidence")),
            artifact_refs: contract
                .get("artifact_refs")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_else(|| normalized_result_refs(artifact_refs, "artifact")),
            changes_made: contract
                .get("changes_made")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            verification: contract
                .get("verification")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
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

fn path_to_string(path: &Path) -> String {
    path.to_string_lossy().to_string()
}

async fn drain_provider_events(
    runs: RuntimeRunStore,
    registry: RuntimeCommandRegistry,
    run_id: String,
    mut events: ProviderEventStream,
    reusable_provider_session: bool,
    writeback: Option<RuntimeCommandWritebackSink>,
) -> anyhow::Result<()> {
    let mut latest_provider_session_id: Option<String> = None;
    let mut terminal_writeback = ProviderTerminalWritebackState::default();
    while let Some(event) = events.next().await {
        let event = event?;
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
                .record_event(&record, latest_provider_session_id.as_deref())
                .await?;
            if let Some(action) = writeback_action {
                match action {
                    ProviderTerminalWritebackAction::Fail(message) => {
                        writeback.fail(message).await?;
                    }
                }
            }
        }
    }
    if let (Some(writeback), Some(completion)) =
        (&writeback, terminal_writeback.finish_successful_stream())
    {
        writeback
            .complete(completion.summary, completion.provider_session_id)
            .await?;
    }
    Ok(())
}

fn provider_request(spec: &RunSpec) -> ProviderRequest {
    ProviderRequest {
        prompt: spec.prompt.clone(),
        workspace_path: spec.workspace_path.clone(),
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
    payload
        .session_policy
        .provider_session_id
        .as_deref()
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
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
                "project_task_id": "55555555-5555-4555-8555-555555555555",
                "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
                "project_task_lease_token": "lease-token-1",
                "runtime_node_id": "44444444-4444-4444-8444-444444444444",
                "execution_context_packet_version": "v1",
                "expected_outputs": ["execution_summary", "evidence_refs", "recommended_next_action"],
                "handoff_contract": {"completion_path": "project_task_attempt_writeback"}
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
            "project_task_id": "55555555-5555-4555-8555-555555555555",
            "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
            "project_task_lease_token": "lease-token-1",
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
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
            "project_task_id": "55555555-5555-4555-8555-555555555555",
            "project_task_attempt_id": "66666666-6666-4666-8666-666666666666",
            "project_task_lease_token": "lease-token-1",
            "runtime_node_id": "44444444-4444-4444-8444-444444444444",
            "handoff_contract": {"completion_path": "manual_review"}
        });
        assert!(project_task_writeback_context(&other).is_none());
    }

    #[test]
    fn provider_terminal_writeback_defers_completion_until_stream_finishes() {
        let mut state = ProviderTerminalWritebackState::default();

        let action = state.observe_event(
            &ProviderEvent::TurnCompleted {
                summary: Some("looks successful".to_string()),
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
    fn provider_terminal_writeback_prefers_later_failure_over_prior_completion() {
        let mut state = ProviderTerminalWritebackState::default();

        assert!(
            state
                .observe_event(
                    &ProviderEvent::TurnCompleted {
                        summary: Some("API Error: 529 provider overloaded".to_string()),
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
}
