use std::collections::HashMap;
use std::sync::{Arc, Mutex};

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuntimeRunBinding {
    pub command_id: String,
    pub run_id: String,
    pub execution_instance_id: String,
    pub provider_type: String,
    pub provider_session_id: Option<String>,
}

#[derive(Debug, Clone, Copy)]
pub struct ActiveRunLookup<'a> {
    pub command_id: Option<&'a str>,
    pub provider_session_id: Option<&'a str>,
    pub execution_instance_id: &'a str,
    pub provider_type: &'a str,
}

#[derive(Debug, Clone, Default)]
pub struct RuntimeCommandRegistry {
    state: Arc<Mutex<RuntimeCommandRegistryState>>,
}

#[derive(Debug, Default)]
struct RuntimeCommandRegistryState {
    command_runs: HashMap<String, String>,
    run_bindings: HashMap<String, RuntimeRunBinding>,
    latest_session_by_instance: HashMap<String, String>,
    active_runs_by_session: HashMap<String, Vec<String>>,
    active_runs_by_instance: HashMap<String, Vec<String>>,
    rejected_commands: HashMap<String, String>,
}

impl RuntimeCommandRegistry {
    pub fn record_run_started(&self, binding: RuntimeRunBinding) {
        let mut state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");
        let instance_key = instance_key(&binding.execution_instance_id, &binding.provider_type);

        state
            .command_runs
            .insert(binding.command_id.clone(), binding.run_id.clone());
        state
            .active_runs_by_instance
            .entry(instance_key.clone())
            .or_default();
        insert_active_run(
            state
                .active_runs_by_instance
                .get_mut(&instance_key)
                .expect("active instance list exists"),
            &binding.run_id,
        );

        if let Some(provider_session_id) = &binding.provider_session_id {
            state
                .latest_session_by_instance
                .insert(instance_key, provider_session_id.clone());
            state
                .active_runs_by_session
                .entry(provider_session_id.clone())
                .or_default();
            insert_active_run(
                state
                    .active_runs_by_session
                    .get_mut(provider_session_id)
                    .expect("active session list exists"),
                &binding.run_id,
            );
        }

        state.run_bindings.insert(binding.run_id.clone(), binding);
    }

    pub fn record_provider_session(&self, run_id: &str, provider_session_id: &str) {
        let mut state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");

        let Some(binding) = state.run_bindings.get(run_id).cloned() else {
            return;
        };

        if let Some(previous_session_id) = &binding.provider_session_id {
            remove_active_run(
                &mut state.active_runs_by_session,
                previous_session_id,
                run_id,
            );
        }

        let instance_key = instance_key(&binding.execution_instance_id, &binding.provider_type);
        state
            .latest_session_by_instance
            .insert(instance_key, provider_session_id.to_string());
        state
            .active_runs_by_session
            .entry(provider_session_id.to_string())
            .or_default();
        insert_active_run(
            state
                .active_runs_by_session
                .get_mut(provider_session_id)
                .expect("active session list exists"),
            run_id,
        );

        if let Some(binding) = state.run_bindings.get_mut(run_id) {
            binding.provider_session_id = Some(provider_session_id.to_string());
        }
    }

    pub fn record_run_finished(&self, run_id: &str) {
        let mut state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");

        let Some(binding) = state.run_bindings.get(run_id).cloned() else {
            return;
        };

        let instance_key = instance_key(&binding.execution_instance_id, &binding.provider_type);
        remove_active_run(&mut state.active_runs_by_instance, &instance_key, run_id);

        if let Some(provider_session_id) = &binding.provider_session_id {
            remove_active_run(
                &mut state.active_runs_by_session,
                provider_session_id,
                run_id,
            );
        }
    }

    pub fn run_for_command(&self, command_id: &str) -> Option<String> {
        let state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");
        state.command_runs.get(command_id).cloned()
    }

    pub fn latest_provider_session(
        &self,
        execution_instance_id: &str,
        provider_type: &str,
    ) -> Option<String> {
        let state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");
        state
            .latest_session_by_instance
            .get(&instance_key(execution_instance_id, provider_type))
            .cloned()
    }

    pub fn active_run(&self, lookup: ActiveRunLookup<'_>) -> Option<String> {
        let state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");

        if let Some(command_id) = lookup.command_id {
            if let Some(run_id) = state.command_runs.get(command_id) {
                if state.is_active_run(run_id) {
                    return Some(run_id.clone());
                }
            }
        }

        if let Some(provider_session_id) = lookup.provider_session_id {
            if let Some(run_id) = first_active_run_for_session(
                &state,
                provider_session_id,
                lookup.execution_instance_id,
                lookup.provider_type,
            ) {
                return Some(run_id);
            }
        }

        first_active_run(
            &state,
            &state.active_runs_by_instance,
            &instance_key(lookup.execution_instance_id, lookup.provider_type),
        )
    }

    pub fn record_rejection(&self, command_id: &str, message: &str) {
        let mut state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");
        state
            .rejected_commands
            .insert(command_id.to_string(), message.to_string());
    }

    pub fn rejection(&self, command_id: &str) -> Option<String> {
        let state = self
            .state
            .lock()
            .expect("runtime command registry poisoned");
        state.rejected_commands.get(command_id).cloned()
    }
}

impl RuntimeCommandRegistryState {
    fn is_active_run(&self, run_id: &str) -> bool {
        self.run_bindings.get(run_id).is_some_and(|binding| {
            self.active_runs_by_instance
                .get(&instance_key(
                    &binding.execution_instance_id,
                    &binding.provider_type,
                ))
                .is_some_and(|run_ids| run_ids.iter().any(|active_run_id| active_run_id == run_id))
        })
    }
}

fn first_active_run(
    state: &RuntimeCommandRegistryState,
    active_runs: &HashMap<String, Vec<String>>,
    key: &str,
) -> Option<String> {
    active_runs
        .get(key)?
        .iter()
        .rev()
        .find(|run_id| state.is_active_run(run_id))
        .cloned()
}

fn first_active_run_for_session(
    state: &RuntimeCommandRegistryState,
    provider_session_id: &str,
    execution_instance_id: &str,
    provider_type: &str,
) -> Option<String> {
    state
        .active_runs_by_session
        .get(provider_session_id)?
        .iter()
        .rev()
        .find(|run_id| {
            state.is_active_run(run_id)
                && state.run_bindings.get(*run_id).is_some_and(|binding| {
                    binding.execution_instance_id == execution_instance_id
                        && binding.provider_type == provider_type
                })
        })
        .cloned()
}

fn insert_active_run(run_ids: &mut Vec<String>, run_id: &str) {
    run_ids.retain(|active_run_id| active_run_id != run_id);
    run_ids.push(run_id.to_string());
}

fn remove_active_run(active_runs: &mut HashMap<String, Vec<String>>, key: &str, run_id: &str) {
    let Some(run_ids) = active_runs.get_mut(key) else {
        return;
    };

    run_ids.retain(|active_run_id| active_run_id != run_id);
    if run_ids.is_empty() {
        active_runs.remove(key);
    }
}

fn instance_key(execution_instance_id: &str, provider_type: &str) -> String {
    format!("{execution_instance_id}:{provider_type}")
}
