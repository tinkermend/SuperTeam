use std::collections::HashMap;
use std::path::{Path, PathBuf};

use futures::StreamExt;

use crate::commands::payload::{RuntimeSessionCommandPayload, SessionPolicyMode};
use crate::commands::registry::{ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding};
use crate::config::RuntimeConfig;
use crate::controlplane::ControlPlaneClient;
use crate::controlplane::models::{
    EnsureInstanceCommand, RuntimeCommand, RuntimeCommandEventWriteback,
    RuntimeCommandTerminalWriteback, RuntimeCommandType,
};
use crate::events::ProviderEvent;
use crate::instances::{EnsureInstanceRequest, ensure_instance};
use crate::providers::claude::ClaudeProvider;
use crate::providers::opencode::OpenCodeProvider;
use crate::providers::{ProviderAdapter, ProviderEventStream, ProviderRequest};
use crate::runs::{RunEventRecord, RunSpec, RunStatus, RuntimeCommandRunContext, RuntimeRunStore};

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
}

#[derive(Clone)]
pub struct RuntimeCommandExecutor {
    config: RuntimeConfig,
    runs: RuntimeRunStore,
    registry: RuntimeCommandRegistry,
    control_plane: Option<ControlPlaneClient>,
}

impl RuntimeCommandExecutor {
    pub fn new(config: RuntimeConfig) -> Self {
        Self {
            runs: RuntimeRunStore::new(config.runs.log_dir.clone()),
            registry: RuntimeCommandRegistry::default(),
            config,
            control_plane: None,
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
        let prompt = match payload.provider_prompt() {
            Some(prompt) => prompt,
            None => {
                let error = self
                    .recorded_error(&command.id, anyhow::anyhow!("prompt or input is required"));
                self.write_command_failure(&command.id, error.to_string())
                    .await?;
                return Err(error);
            }
        };
        let session_id = match self.input_session_id(&command, &payload) {
            Ok(session_id) => session_id,
            Err(error) => {
                self.write_command_failure(&command.id, error.to_string())
                    .await?;
                return Err(error);
            }
        };
        let reusable_provider_session = reusable_provider_session(&payload);
        let provider = match self.select_provider(&command.id, &payload) {
            Ok(provider) => provider,
            Err(error) => {
                self.write_command_failure(&command.id, error.to_string())
                    .await?;
                return Err(error);
            }
        };
        let workspace_path = match self.ensure_command_instance(&command.id, &payload) {
            Ok(workspace_path) => workspace_path,
            Err(error) => {
                self.write_command_failure(&command.id, error.to_string())
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
            });
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
        let result = match self.ensure_instance_from_command(&command, "provision_instance") {
            Ok(result) => result,
            Err(error) => {
                let message = error.to_string();
                self.write_provisioning_failure(&command.id, message)
                    .await?;
                return Err(error);
            }
        };

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
            execution_instance_id: request.execution_instance_id,
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
        ensure_instance(EnsureInstanceRequest {
            base_dir: self.config.workspace.base_dir.clone(),
            execution_instance_id: payload.execution_instance_id.clone(),
        })
        .map(|result| result.agent_home_dir)
        .map_err(|error| self.recorded_error(command_id, error))
    }

    fn select_provider(
        &self,
        command_id: &str,
        payload: &RuntimeSessionCommandPayload,
    ) -> anyhow::Result<Box<dyn ProviderAdapter>> {
        match payload.provider_kind() {
            "claude" => {
                if !self.config.providers.claude_code.enabled {
                    return Err(self.recorded_error(
                        command_id,
                        anyhow::anyhow!("Claude Code provider is disabled"),
                    ));
                }
                Ok(Box::new(ClaudeProvider::new(
                    self.config.providers.claude_code.binary_path.clone(),
                )))
            }
            "opencode" => {
                if !self.config.providers.opencode.enabled {
                    return Err(self.recorded_error(
                        command_id,
                        anyhow::anyhow!("OpenCode provider is disabled"),
                    ));
                }
                Ok(Box::new(OpenCodeProvider::new(
                    self.config.providers.opencode.binary_path.clone(),
                )))
            }
            _ => Err(self.recorded_error(
                command_id,
                anyhow::anyhow!("unsupported provider_type: {}", payload.provider_type),
            )),
        }
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

impl RuntimeCommandWritebackSink {
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
                &command_completed_terminal(summary, provider_session_id),
            )
            .await
    }

    async fn fail(&self, error_message: String) -> anyhow::Result<()> {
        self.client
            .fail_runtime_command(&self.command_id, &command_failed_terminal(error_message))
            .await
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
        let summary = match &event {
            ProviderEvent::TurnCompleted { summary } => summary.clone(),
            _ => None,
        };
        let failure = match &event {
            ProviderEvent::TurnError { message } => Some(message.clone()),
            _ => None,
        };
        let record = runs.record_event(&run_id, event).await?;
        if let Some(writeback) = &writeback {
            writeback
                .record_event(&record, latest_provider_session_id.as_deref())
                .await?;
            if let Some(message) = failure {
                writeback.fail(message).await?;
            } else if is_terminal {
                writeback
                    .complete(summary, latest_provider_session_id.clone())
                    .await?;
            }
        }
        if is_terminal {
            registry.record_run_finished(&run_id);
        }
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
