use std::path::PathBuf;

use futures::StreamExt;

use crate::commands::payload::{RuntimeSessionCommandPayload, SessionPolicyMode};
use crate::commands::registry::{ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding};
use crate::config::RuntimeConfig;
use crate::controlplane::models::{EnsureInstanceCommand, RuntimeCommand, RuntimeCommandType};
use crate::events::ProviderEvent;
use crate::instances::{EnsureInstanceRequest, ensure_instance};
use crate::providers::claude::ClaudeProvider;
use crate::providers::opencode::OpenCodeProvider;
use crate::providers::{ProviderAdapter, ProviderEventStream, ProviderRequest};
use crate::runs::{RunSpec, RunStatus, RuntimeCommandRunContext, RuntimeRunStore};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeCommandOutcome {
    pub command_id: String,
    pub accepted: bool,
    pub run_id: Option<String>,
}

#[derive(Clone)]
pub struct RuntimeCommandExecutor {
    config: RuntimeConfig,
    runs: RuntimeRunStore,
    registry: RuntimeCommandRegistry,
}

impl RuntimeCommandExecutor {
    pub fn new(config: RuntimeConfig) -> Self {
        Self {
            runs: RuntimeRunStore::new(config.runs.log_dir.clone()),
            registry: RuntimeCommandRegistry::default(),
            config,
        }
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
        let payload = self.parse_session_payload(&command)?;
        let prompt = payload.provider_prompt().ok_or_else(|| {
            self.recorded_error(&command.id, anyhow::anyhow!("prompt or input is required"))
        })?;
        let session_id = self.input_session_id(&command, &payload)?;
        let provider = self.select_provider(&command.id, &payload)?;
        let workspace_path = self.ensure_command_instance(&command.id, &payload)?;
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

        let snapshot = self
            .runs
            .start_run(spec.clone(), None)
            .await
            .map_err(|error| self.recorded_error(&payload.command_id, error))?;
        self.registry.record_run_started(RuntimeRunBinding {
            command_id: payload.command_id.clone(),
            run_id: snapshot.id.clone(),
            execution_instance_id: payload.execution_instance_id.clone(),
            provider_type: payload.provider_type.clone(),
            provider_session_id: session_id,
        });
        let run_id = snapshot.id.clone();
        let provider_run = match provider.start(provider_request(&spec)).await {
            Ok(provider_run) => provider_run,
            Err(error) => {
                let message = error.to_string();
                let _ = self.runs.finish_failed(&run_id, message).await;
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
            let _ = self.runs.finish_failed(&run_id, message).await;
            self.registry.record_run_finished(&run_id);
            return Ok(RuntimeCommandOutcome {
                command_id: payload.command_id,
                accepted: true,
                run_id: Some(run_id),
            });
        }
        self.spawn_provider_event_drain(run_id.clone(), provider_run.events);

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
        let request: EnsureInstanceCommand = serde_json::from_value(command.payload.clone())
            .map_err(|error| {
                self.recorded_error(
                    &command.id,
                    anyhow::anyhow!("invalid ensure_instance command payload: {error}"),
                )
            })?;
        ensure_instance(EnsureInstanceRequest {
            base_dir: self.config.workspace.base_dir.clone(),
            execution_instance_id: request.execution_instance_id,
        })
        .map_err(|error| self.recorded_error(&command.id, error))?;

        Ok(RuntimeCommandOutcome {
            command_id: command.id,
            accepted: true,
            run_id: None,
        })
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

    fn spawn_provider_event_drain(&self, run_id: String, events: ProviderEventStream) {
        let runs = self.runs.clone();
        let registry = self.registry.clone();
        tokio::spawn(async move {
            let result =
                drain_provider_events(runs.clone(), registry.clone(), run_id.clone(), events).await;

            if let Err(error) = result {
                if !run_is_cancelled(&runs, &run_id).await {
                    let _ = runs.finish_failed(&run_id, error.to_string()).await;
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

async fn drain_provider_events(
    runs: RuntimeRunStore,
    registry: RuntimeCommandRegistry,
    run_id: String,
    mut events: ProviderEventStream,
) -> anyhow::Result<()> {
    while let Some(event) = events.next().await {
        let event = event?;
        if let ProviderEvent::SessionStarted { session_id } = &event {
            registry.record_provider_session(&run_id, session_id);
        }
        runs.record_event(&run_id, event).await?;
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
