# Runtime Command Execution Layer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 Runtime Agent 通过 WebSocket 收到 `start_session`、`resume_session`、`send_input`、`stop_session` 后能在本地真实驱动 Provider 执行或取消。

**Architecture:** 新增 Runtime command execution layer，放在 `controlplane/ws.rs` 和 `ProviderAdapter` 之间。命令 payload 强类型解析，executor 复用现有 `RuntimeRunStore` 与 Claude/OpenCode adapter，并用内存 registry 维护 `command_id`、`execution_instance_id`、`provider_session_id` 与 active run 的关系。

**Tech Stack:** Rust 2024、Tokio、serde/serde_json、anyhow、现有 `ProviderAdapter`、`RuntimeRunStore`、Runtime WebSocket command loop。

---

## File Structure

- Create: `apps/runtime-agent/src/commands/mod.rs`
  - Runtime command layer 对外入口，导出 payload、registry、executor。
- Create: `apps/runtime-agent/src/commands/payload.rs`
  - 强类型 payload、session policy、字段校验、`provider_type` 到本地 provider kind 的规范化。
- Create: `apps/runtime-agent/src/commands/registry.rs`
  - 内存 command/session/run 映射，支持 `reuse_latest` 与 `stop_session` 查找 active run。
- Create: `apps/runtime-agent/src/commands/executor.rs`
  - `RuntimeCommandExecutor`，负责处理四类命令、创建 run、启动 provider、记录事件和取消 run。
- Modify: `apps/runtime-agent/src/lib.rs`
  - 导出 `commands` module。
- Modify: `apps/runtime-agent/src/runs.rs`
  - 扩展 `RunSpec` 与 `RunSnapshot` 的 command metadata。
- Modify: `apps/runtime-agent/src/server.rs`
  - 保持 `/runs` HTTP path 兼容，补齐新增 `RunSpec` 字段默认值。
- Modify: `apps/runtime-agent/src/controlplane/ws.rs`
  - 使用 `RuntimeCommandExecutor` 处理 runtime commands，保留坏命令不中断连接的行为。
- Test: `apps/runtime-agent/tests/runtime_command_payload_test.rs`
- Test: `apps/runtime-agent/tests/runtime_command_registry_test.rs`
- Test: `apps/runtime-agent/tests/runtime_command_executor_test.rs`
- Modify tests: `apps/runtime-agent/tests/run_registry_test.rs`、`apps/runtime-agent/tests/http_server_test.rs`、`apps/runtime-agent/tests/controlplane_client_test.rs`
- Modify: `CHANGELOG.md`

---

### Task 1: Extend Runtime Run Metadata

**Files:**
- Modify: `apps/runtime-agent/src/runs.rs`
- Modify: `apps/runtime-agent/tests/run_registry_test.rs`
- Modify: `apps/runtime-agent/src/server.rs`
- Test: `apps/runtime-agent/tests/run_registry_test.rs`

- [ ] **Step 1: Write the failing run metadata test**

Append this test to `apps/runtime-agent/tests/run_registry_test.rs`:

```rust
#[tokio::test]
async fn store_preserves_runtime_command_metadata_on_snapshot() {
    let temp = TempDir::new().expect("tempdir");
    let store = RuntimeRunStore::new(temp.path().join("runs"));
    let command_context = superteam_runtime_agent::runs::RuntimeCommandRunContext {
        command_id: "cmd-001".to_string(),
        digital_employee_id: "11111111-1111-4111-8111-111111111111".to_string(),
        execution_instance_id: "22222222-2222-4222-8222-222222222222".to_string(),
        provider_type: "claude-code".to_string(),
        session_policy: serde_json::json!({"mode":"new","recoverable":true}),
        context_refs: vec![serde_json::json!({"id":"ctx-1","kind":"memory"})],
        artifact_refs: vec![serde_json::json!({"id":"artifact-1","kind":"input"})],
        metadata: serde_json::json!({"source":"runtime-command-test"}),
    };

    let run = store
        .start_run(
            RunSpec {
                provider_kind: "claude".to_string(),
                workspace_path: PathBuf::from("/tmp/workspace"),
                prompt: "hello".to_string(),
                session_id: None,
                continue_session: false,
                model: None,
                command_context: Some(command_context.clone()),
            },
            None,
        )
        .await
        .expect("start run");

    let snapshot = store.get_run(&run.id).await.expect("run snapshot");
    assert_eq!(snapshot.command_context, Some(command_context));
}
```

- [ ] **Step 2: Run the new test to verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml store_preserves_runtime_command_metadata_on_snapshot
```

Expected: FAIL because `RuntimeCommandRunContext` and `RunSpec.command_context` do not exist.

- [ ] **Step 3: Add command metadata types to `runs.rs`**

In `apps/runtime-agent/src/runs.rs`, add this struct near `RunSpec`:

```rust
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
```

Change the derives on `RunSpec` and `RunSnapshot` from:

```rust
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
```

to:

```rust
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
```

Add this field to both `RunSpec` and `RunSnapshot`:

```rust
pub command_context: Option<RuntimeCommandRunContext>,
```

In `RuntimeRunStore::start_run`, copy the field into the snapshot:

```rust
command_context: spec.command_context,
```

- [ ] **Step 4: Preserve compatibility for existing run creation**

In `apps/runtime-agent/src/server.rs`, update `create_run` so the `RunSpec` uses:

```rust
command_context: None,
```

In existing test setup in `apps/runtime-agent/tests/run_registry_test.rs`, update the existing `RunSpec` literal with:

```rust
command_context: None,
```

- [ ] **Step 5: Run focused tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml store_preserves_runtime_command_metadata_on_snapshot store_records_provider_session_events_and_replays_them
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/runtime-agent/src/runs.rs apps/runtime-agent/src/server.rs apps/runtime-agent/tests/run_registry_test.rs
git commit -m "feat(runtime-agent): track command metadata on runs"
```

---

### Task 2: Add Runtime Command Payload Parser

**Files:**
- Create: `apps/runtime-agent/src/commands/mod.rs`
- Create: `apps/runtime-agent/src/commands/payload.rs`
- Modify: `apps/runtime-agent/src/lib.rs`
- Test: `apps/runtime-agent/tests/runtime_command_payload_test.rs`

- [ ] **Step 1: Write failing payload parser tests**

Create `apps/runtime-agent/tests/runtime_command_payload_test.rs`:

```rust
use serde_json::json;
use superteam_runtime_agent::commands::payload::{
    RuntimeSessionCommandPayload, SessionPolicyMode,
};
use superteam_runtime_agent::controlplane::models::{
    RuntimeCommand, RuntimeCommandType,
};

fn command(payload: serde_json::Value) -> RuntimeCommand {
    RuntimeCommand {
        id: "cmd-001".to_string(),
        command_type: RuntimeCommandType::StartSession,
        payload,
    }
}

fn valid_payload() -> serde_json::Value {
    json!({
        "command_id": "cmd-001",
        "digital_employee_id": "11111111-1111-4111-8111-111111111111",
        "execution_instance_id": "22222222-2222-4222-8222-222222222222",
        "provider_type": "claude-code",
        "session_policy": {"mode": "new", "provider_session_id": null, "recoverable": true},
        "prompt": "hello",
        "input": null,
        "context_refs": [],
        "artifact_refs": [],
        "model": null,
        "metadata": {}
    })
}

#[test]
fn parses_valid_start_session_payload() {
    let parsed = RuntimeSessionCommandPayload::from_command(&command(valid_payload()))
        .expect("valid command payload");

    assert_eq!(parsed.command_id, "cmd-001");
    assert_eq!(parsed.digital_employee_id, "11111111-1111-4111-8111-111111111111");
    assert_eq!(parsed.execution_instance_id, "22222222-2222-4222-8222-222222222222");
    assert_eq!(parsed.provider_type, "claude-code");
    assert_eq!(parsed.provider_kind(), "claude");
    assert_eq!(parsed.session_policy.mode, SessionPolicyMode::New);
    assert_eq!(parsed.provider_prompt().as_deref(), Some("hello"));
}

#[test]
fn rejects_command_id_mismatch() {
    let mut payload = valid_payload();
    payload["command_id"] = json!("different-command");

    let error = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect_err("mismatched command id should fail");

    assert!(error.to_string().contains("command_id does not match runtime command id"));
}

#[test]
fn rejects_missing_required_arrays_even_when_empty_arrays_are_allowed() {
    let mut payload = valid_payload();
    payload.as_object_mut().unwrap().remove("context_refs");

    let error = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect_err("missing context_refs should fail");

    assert!(error.to_string().contains("context_refs is required"));
}

#[test]
fn rejects_empty_prompt_for_input_commands() {
    let mut payload = valid_payload();
    payload["prompt"] = json!("");
    payload["input"] = json!(null);

    let error = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect_err("empty provider input should fail");

    assert!(error.to_string().contains("prompt or input is required"));
}

#[test]
fn allows_stop_session_without_prompt_or_input() {
    let mut command = command(valid_payload());
    command.command_type = RuntimeCommandType::StopSession;
    command.payload["prompt"] = json!("");
    command.payload["input"] = json!(null);

    let parsed = RuntimeSessionCommandPayload::from_command(&command)
        .expect("stop_session can omit provider input");

    assert_eq!(parsed.provider_prompt(), None);
}

#[test]
fn resume_requires_provider_session_id() {
    let mut command = command(valid_payload());
    command.command_type = RuntimeCommandType::ResumeSession;
    command.payload["session_policy"] = json!({"mode": "resume", "provider_session_id": null, "recoverable": true});

    let error = RuntimeSessionCommandPayload::from_command(&command)
        .expect_err("resume without provider_session_id should fail");

    assert!(error.to_string().contains("provider_session_id is required for resume"));
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_payload_test
```

Expected: FAIL because `commands::payload` is missing.

- [ ] **Step 3: Create module exports**

Create `apps/runtime-agent/src/commands/mod.rs`:

```rust
pub mod payload;
```

Modify `apps/runtime-agent/src/lib.rs`:

```rust
pub mod commands;
pub mod config;
pub mod controlplane;
pub mod daemon;
pub mod events;
pub mod executor;
pub mod health;
pub mod instances;
pub mod providers;
pub mod runs;
pub mod server;
pub mod session;
```

- [ ] **Step 4: Implement payload parsing**

Create `apps/runtime-agent/src/commands/payload.rs`:

```rust
use serde::{Deserialize, Serialize};

use crate::controlplane::models::{RuntimeCommand, RuntimeCommandType};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum SessionPolicyMode {
    New,
    Resume,
    ReuseLatest,
    Ephemeral,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct RuntimeSessionPolicy {
    pub mode: SessionPolicyMode,
    #[serde(default)]
    pub provider_session_id: Option<String>,
    #[serde(default = "default_recoverable")]
    pub recoverable: bool,
}

#[derive(Debug, Clone, PartialEq, Deserialize)]
pub struct RuntimeSessionCommandPayload {
    pub command_id: String,
    pub digital_employee_id: String,
    pub execution_instance_id: String,
    pub provider_type: String,
    pub session_policy: RuntimeSessionPolicy,
    pub prompt: Option<String>,
    pub input: Option<String>,
    pub context_refs: Vec<serde_json::Value>,
    pub artifact_refs: Vec<serde_json::Value>,
    #[serde(default)]
    pub model: Option<String>,
    #[serde(default)]
    pub metadata: serde_json::Value,
}

fn default_recoverable() -> bool {
    true
}

impl RuntimeSessionCommandPayload {
    pub fn from_command(command: &RuntimeCommand) -> anyhow::Result<Self> {
        require_field(&command.payload, "command_id")?;
        require_field(&command.payload, "digital_employee_id")?;
        require_field(&command.payload, "execution_instance_id")?;
        require_field(&command.payload, "provider_type")?;
        require_field(&command.payload, "session_policy")?;
        require_field(&command.payload, "prompt")?;
        require_field(&command.payload, "input")?;
        require_field(&command.payload, "context_refs")?;
        require_field(&command.payload, "artifact_refs")?;

        let parsed: Self = serde_json::from_value(command.payload.clone())?;
        parsed.validate(command)?;
        Ok(parsed)
    }

    pub fn provider_kind(&self) -> &'static str {
        match self.provider_type.as_str() {
            "claude-code" | "claude" => "claude",
            "opencode" => "opencode",
            _ => "unsupported",
        }
    }

    pub fn provider_prompt(&self) -> Option<String> {
        self.prompt
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
            .map(ToString::to_string)
            .or_else(|| {
                self.input
                    .as_deref()
                    .map(str::trim)
                    .filter(|value| !value.is_empty())
                    .map(ToString::to_string)
            })
    }

    fn validate(&self, command: &RuntimeCommand) -> anyhow::Result<()> {
        if self.command_id.trim().is_empty() {
            anyhow::bail!("command_id is required");
        }
        if self.command_id != command.id {
            anyhow::bail!("command_id does not match runtime command id");
        }
        require_uuid_like("digital_employee_id", &self.digital_employee_id)?;
        require_uuid_like("execution_instance_id", &self.execution_instance_id)?;
        if self.provider_kind() == "unsupported" {
            anyhow::bail!("unsupported provider_type: {}", self.provider_type);
        }
        if matches!(
            command.command_type,
            RuntimeCommandType::StartSession
                | RuntimeCommandType::ResumeSession
                | RuntimeCommandType::SendInput
        ) && self.provider_prompt().is_none()
        {
            anyhow::bail!("prompt or input is required");
        }
        if matches!(self.session_policy.mode, SessionPolicyMode::Resume)
            && self
                .session_policy
                .provider_session_id
                .as_deref()
                .map(str::trim)
                .filter(|value| !value.is_empty())
                .is_none()
        {
            anyhow::bail!("provider_session_id is required for resume");
        }
        Ok(())
    }
}

fn require_field(value: &serde_json::Value, field: &str) -> anyhow::Result<()> {
    if value.get(field).is_none() {
        anyhow::bail!("{field} is required");
    }
    Ok(())
}

fn require_uuid_like(field: &str, value: &str) -> anyhow::Result<()> {
    if !is_uuid_like(value) {
        anyhow::bail!("{field} must be a UUID");
    }
    Ok(())
}

fn is_uuid_like(value: &str) -> bool {
    if value.len() != 36 {
        return false;
    }
    for (index, ch) in value.chars().enumerate() {
        match index {
            8 | 13 | 18 | 23 => {
                if ch != '-' {
                    return false;
                }
            }
            _ => {
                if !ch.is_ascii_hexdigit() {
                    return false;
                }
            }
        }
    }
    true
}
```

- [ ] **Step 5: Run payload parser tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_payload_test
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/runtime-agent/src/lib.rs apps/runtime-agent/src/commands/mod.rs apps/runtime-agent/src/commands/payload.rs apps/runtime-agent/tests/runtime_command_payload_test.rs
git commit -m "feat(runtime-agent): parse runtime session commands"
```

---

### Task 3: Add Runtime Command Registry

**Files:**
- Create: `apps/runtime-agent/src/commands/registry.rs`
- Modify: `apps/runtime-agent/src/commands/mod.rs`
- Test: `apps/runtime-agent/tests/runtime_command_registry_test.rs`

- [ ] **Step 1: Write failing registry tests**

Create `apps/runtime-agent/tests/runtime_command_registry_test.rs`:

```rust
use superteam_runtime_agent::commands::registry::{
    ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding,
};

fn binding() -> RuntimeRunBinding {
    RuntimeRunBinding {
        command_id: "cmd-001".to_string(),
        run_id: "run-001".to_string(),
        execution_instance_id: "22222222-2222-4222-8222-222222222222".to_string(),
        provider_type: "claude-code".to_string(),
        provider_session_id: None,
    }
}

#[test]
fn registry_resolves_command_and_latest_provider_session() {
    let registry = RuntimeCommandRegistry::default();
    registry.record_run_started(binding());
    registry.record_provider_session("run-001", "provider-session-1");

    assert_eq!(
        registry.run_for_command("cmd-001").as_deref(),
        Some("run-001")
    );
    assert_eq!(
        registry
            .latest_provider_session("22222222-2222-4222-8222-222222222222", "claude-code")
            .as_deref(),
        Some("provider-session-1")
    );
}

#[test]
fn registry_returns_active_runs_for_stop_priority() {
    let registry = RuntimeCommandRegistry::default();
    registry.record_run_started(binding());
    registry.record_provider_session("run-001", "provider-session-1");

    assert_eq!(
        registry.active_run(ActiveRunLookup {
            command_id: Some("cmd-001"),
            provider_session_id: None,
            execution_instance_id: "22222222-2222-4222-8222-222222222222",
            provider_type: "claude-code",
        }).as_deref(),
        Some("run-001")
    );

    registry.record_run_finished("run-001");
    assert!(
        registry.active_run(ActiveRunLookup {
            command_id: Some("cmd-001"),
            provider_session_id: Some("provider-session-1"),
            execution_instance_id: "22222222-2222-4222-8222-222222222222",
            provider_type: "claude-code",
        }).is_none()
    );
}

#[test]
fn registry_records_rejected_commands() {
    let registry = RuntimeCommandRegistry::default();
    registry.record_rejection("cmd-bad", "prompt or input is required");

    assert_eq!(
        registry.rejection("cmd-bad").as_deref(),
        Some("prompt or input is required")
    );
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_registry_test
```

Expected: FAIL because `commands::registry` is missing.

- [ ] **Step 3: Export registry module**

Modify `apps/runtime-agent/src/commands/mod.rs`:

```rust
pub mod payload;
pub mod registry;
```

- [ ] **Step 4: Implement registry**

Create `apps/runtime-agent/src/commands/registry.rs`:

```rust
use std::collections::{HashMap, HashSet};
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

#[derive(Clone, Default)]
pub struct RuntimeCommandRegistry {
    inner: Arc<Mutex<RuntimeCommandRegistryState>>,
}

#[derive(Default)]
struct RuntimeCommandRegistryState {
    command_runs: HashMap<String, String>,
    run_bindings: HashMap<String, RuntimeRunBinding>,
    latest_session_by_instance: HashMap<String, String>,
    active_runs_by_session: HashMap<String, HashSet<String>>,
    active_runs_by_instance: HashMap<String, HashSet<String>>,
    rejected_commands: HashMap<String, String>,
}

impl RuntimeCommandRegistry {
    pub fn record_run_started(&self, binding: RuntimeRunBinding) {
        let instance_key = instance_key(&binding.execution_instance_id, &binding.provider_type);
        let mut state = self.inner.lock().expect("runtime command registry lock");
        state
            .command_runs
            .insert(binding.command_id.clone(), binding.run_id.clone());
        state
            .active_runs_by_instance
            .entry(instance_key)
            .or_default()
            .insert(binding.run_id.clone());
        if let Some(provider_session_id) = binding.provider_session_id.as_ref() {
            state
                .active_runs_by_session
                .entry(provider_session_id.clone())
                .or_default()
                .insert(binding.run_id.clone());
        }
        state.run_bindings.insert(binding.run_id.clone(), binding);
    }

    pub fn record_provider_session(&self, run_id: &str, provider_session_id: &str) {
        let mut state = self.inner.lock().expect("runtime command registry lock");
        let Some(binding) = state.run_bindings.get_mut(run_id) else {
            return;
        };
        binding.provider_session_id = Some(provider_session_id.to_string());
        let instance = instance_key(&binding.execution_instance_id, &binding.provider_type);
        state
            .latest_session_by_instance
            .insert(instance, provider_session_id.to_string());
        state
            .active_runs_by_session
            .entry(provider_session_id.to_string())
            .or_default()
            .insert(run_id.to_string());
    }

    pub fn record_run_finished(&self, run_id: &str) {
        let mut state = self.inner.lock().expect("runtime command registry lock");
        let Some(binding) = state.run_bindings.get(run_id).cloned() else {
            return;
        };
        let instance = instance_key(&binding.execution_instance_id, &binding.provider_type);
        remove_active(&mut state.active_runs_by_instance, &instance, run_id);
        if let Some(provider_session_id) = binding.provider_session_id {
            remove_active(&mut state.active_runs_by_session, &provider_session_id, run_id);
        }
    }

    pub fn run_for_command(&self, command_id: &str) -> Option<String> {
        self.inner
            .lock()
            .expect("runtime command registry lock")
            .command_runs
            .get(command_id)
            .cloned()
    }

    pub fn latest_provider_session(
        &self,
        execution_instance_id: &str,
        provider_type: &str,
    ) -> Option<String> {
        self.inner
            .lock()
            .expect("runtime command registry lock")
            .latest_session_by_instance
            .get(&instance_key(execution_instance_id, provider_type))
            .cloned()
    }

    pub fn active_run(&self, lookup: ActiveRunLookup<'_>) -> Option<String> {
        let state = self.inner.lock().expect("runtime command registry lock");
        if let Some(command_id) = lookup.command_id {
            if let Some(run_id) = state.command_runs.get(command_id) {
                if is_active(&state, run_id) {
                    return Some(run_id.clone());
                }
            }
        }
        if let Some(provider_session_id) = lookup.provider_session_id {
            if let Some(run_id) = first_active(&state.active_runs_by_session, provider_session_id) {
                return Some(run_id);
            }
        }
        first_active(
            &state.active_runs_by_instance,
            &instance_key(lookup.execution_instance_id, lookup.provider_type),
        )
    }

    pub fn record_rejection(&self, command_id: &str, message: impl Into<String>) {
        self.inner
            .lock()
            .expect("runtime command registry lock")
            .rejected_commands
            .insert(command_id.to_string(), message.into());
    }

    pub fn rejection(&self, command_id: &str) -> Option<String> {
        self.inner
            .lock()
            .expect("runtime command registry lock")
            .rejected_commands
            .get(command_id)
            .cloned()
    }
}

fn instance_key(execution_instance_id: &str, provider_type: &str) -> String {
    format!("{execution_instance_id}:{provider_type}")
}

fn remove_active(map: &mut HashMap<String, HashSet<String>>, key: &str, run_id: &str) {
    if let Some(runs) = map.get_mut(key) {
        runs.remove(run_id);
        if runs.is_empty() {
            map.remove(key);
        }
    }
}

fn first_active(map: &HashMap<String, HashSet<String>>, key: &str) -> Option<String> {
    map.get(key).and_then(|runs| runs.iter().next().cloned())
}

fn is_active(state: &RuntimeCommandRegistryState, run_id: &str) -> bool {
    state
        .active_runs_by_instance
        .values()
        .any(|runs| runs.contains(run_id))
        || state
            .active_runs_by_session
            .values()
            .any(|runs| runs.contains(run_id))
}
```

- [ ] **Step 5: Run registry tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_registry_test
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/runtime-agent/src/commands/mod.rs apps/runtime-agent/src/commands/registry.rs apps/runtime-agent/tests/runtime_command_registry_test.rs
git commit -m "feat(runtime-agent): track runtime command runs"
```

---

### Task 4: Add Runtime Command Executor

**Files:**
- Create: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/commands/mod.rs`
- Test: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Write failing executor tests**

Create `apps/runtime-agent/tests/runtime_command_executor_test.rs`:

```rust
use std::fs;
use std::os::unix::fs::PermissionsExt;

use serde_json::json;
use superteam_runtime_agent::commands::executor::RuntimeCommandExecutor;
use superteam_runtime_agent::config::RuntimeConfig;
use superteam_runtime_agent::controlplane::models::{
    RuntimeCommand, RuntimeCommandType,
};
use superteam_runtime_agent::runs::RunStatus;
use tempfile::TempDir;

fn make_script(dir: &TempDir, name: &str, body: &str) -> std::path::PathBuf {
    let path = dir.path().join(name);
    fs::write(&path, body).expect("write fake provider script");
    let mut permissions = fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}

fn configure_runtime(temp: &TempDir, provider_bin: std::path::PathBuf) -> RuntimeConfig {
    let mut config = RuntimeConfig::new("node-command").expect("config");
    config.workspace.base_dir = temp.path().join("workspaces");
    config.runs.log_dir = temp.path().join("run-logs");
    config.providers.claude_code.enabled = true;
    config.providers.claude_code.binary_path = provider_bin;
    config.providers.opencode.enabled = false;
    config
}

fn command(command_type: RuntimeCommandType, command_id: &str, session_policy: serde_json::Value) -> RuntimeCommand {
    RuntimeCommand {
        id: command_id.to_string(),
        command_type,
        payload: json!({
            "command_id": command_id,
            "digital_employee_id": "11111111-1111-4111-8111-111111111111",
            "execution_instance_id": "22222222-2222-4222-8222-222222222222",
            "provider_type": "claude-code",
            "session_policy": session_policy,
            "prompt": "hello from command",
            "input": null,
            "context_refs": [{"id":"ctx-1"}],
            "artifact_refs": [{"id":"artifact-1"}],
            "model": null,
            "metadata": {"test":"runtime-command"}
        }),
    }
}

async fn wait_for_status(
    executor: &RuntimeCommandExecutor,
    run_id: &str,
    status: RunStatus,
) -> serde_json::Value {
    for _ in 0..50 {
        let snapshot = executor.runs().get_run(run_id).await.expect("run snapshot");
        if snapshot.status == status {
            return serde_json::to_value(snapshot).expect("snapshot json");
        }
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
    }
    panic!("run did not reach status {status:?}");
}

#[tokio::test]
async fn start_session_runs_provider_and_records_command_context() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"session-from-command"}'
printf '%s\n' '{"type":"assistant","message":{"content":[{"type":"text","text":"hello"}]}}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
    );
    let executor = RuntimeCommandExecutor::new(configure_runtime(&temp, fake_claude));

    let outcome = executor
        .handle_command(command(
            RuntimeCommandType::StartSession,
            "cmd-start",
            json!({"mode":"new","provider_session_id":null,"recoverable":true}),
        ))
        .await
        .expect("handle start_session");

    let snapshot = wait_for_status(&executor, outcome.run_id.as_deref().unwrap(), RunStatus::Completed).await;
    assert_eq!(snapshot["provider_session_id"], "session-from-command");
    assert_eq!(snapshot["command_context"]["command_id"], "cmd-start");
    assert_eq!(snapshot["command_context"]["context_refs"][0]["id"], "ctx-1");
    assert_eq!(
        executor
            .registry()
            .latest_provider_session("22222222-2222-4222-8222-222222222222", "claude-code")
            .as_deref(),
        Some("session-from-command")
    );
}

#[tokio::test]
async fn resume_session_sets_continue_session_and_session_id() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' "$*" > "$FAKE_CLAUDE_ARGS_FILE"
printf '%s\n' '{"type":"system","session_id":"existing-session"}'
printf '%s\n' '{"type":"result","result":"done"}'
"#,
    );
    let args_file = temp.path().join("args.txt");
    let mut config = configure_runtime(&temp, fake_claude);
    config.providers.claude_code.enabled = true;
    let executor = RuntimeCommandExecutor::new(config);
    std::env::set_var("FAKE_CLAUDE_ARGS_FILE", &args_file);

    let outcome = executor
        .handle_command(command(
            RuntimeCommandType::ResumeSession,
            "cmd-resume",
            json!({"mode":"resume","provider_session_id":"existing-session","recoverable":true}),
        ))
        .await
        .expect("handle resume_session");

    wait_for_status(&executor, outcome.run_id.as_deref().unwrap(), RunStatus::Completed).await;
    let args = std::fs::read_to_string(args_file).expect("args");
    assert!(args.contains("--resume existing-session"));
}

#[tokio::test]
async fn stop_session_cancels_active_run() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        &temp,
        "slow-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"slow-session"}'
sleep 5
"#,
    );
    let executor = RuntimeCommandExecutor::new(configure_runtime(&temp, fake_claude));
    let outcome = executor
        .handle_command(command(
            RuntimeCommandType::StartSession,
            "cmd-slow",
            json!({"mode":"new","provider_session_id":null,"recoverable":true}),
        ))
        .await
        .expect("handle start_session");
    let run_id = outcome.run_id.expect("run id");

    for _ in 0..20 {
        if executor
            .registry()
            .latest_provider_session("22222222-2222-4222-8222-222222222222", "claude-code")
            .as_deref()
            == Some("slow-session")
        {
            break;
        }
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
    }

    let mut stop = command(
        RuntimeCommandType::StopSession,
        "cmd-stop",
        json!({"mode":"resume","provider_session_id":"slow-session","recoverable":true}),
    );
    stop.payload["prompt"] = json!("");
    stop.payload["input"] = json!(null);
    executor.handle_command(stop).await.expect("handle stop_session");

    let snapshot = wait_for_status(&executor, &run_id, RunStatus::Cancelled).await;
    assert_eq!(snapshot["status"], "cancelled");
}
```

- [ ] **Step 2: Run executor tests to verify they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_executor_test
```

Expected: FAIL because `commands::executor` is missing.

- [ ] **Step 3: Export executor module**

Modify `apps/runtime-agent/src/commands/mod.rs`:

```rust
pub mod executor;
pub mod payload;
pub mod registry;
```

- [ ] **Step 4: Implement executor outcome and constructor**

Create `apps/runtime-agent/src/commands/executor.rs` with these definitions:

```rust
use futures::TryStreamExt;

use crate::commands::payload::{
    RuntimeSessionCommandPayload, SessionPolicyMode,
};
use crate::commands::registry::{
    ActiveRunLookup, RuntimeCommandRegistry, RuntimeRunBinding,
};
use crate::config::RuntimeConfig;
use crate::controlplane::models::{RuntimeCommand, RuntimeCommandType};
use crate::events::ProviderEvent;
use crate::instances::{ensure_instance, EnsureInstanceRequest};
use crate::providers::{
    claude::ClaudeProvider, opencode::OpenCodeProvider, ProviderAdapter, ProviderRequest,
};
use crate::runs::{
    RunSpec, RunStatus, RuntimeCommandRunContext, RuntimeRunStore,
};

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

    pub async fn handle_command(&self, command: RuntimeCommand) -> anyhow::Result<RuntimeCommandOutcome> {
        match command.command_type {
            RuntimeCommandType::StartSession
            | RuntimeCommandType::ResumeSession
            | RuntimeCommandType::SendInput => self.handle_input_command(command).await,
            RuntimeCommandType::StopSession => self.handle_stop_command(command).await,
            RuntimeCommandType::EnsureInstance => {
                let payload: crate::controlplane::models::EnsureInstanceCommand =
                    serde_json::from_value(command.payload)?;
                ensure_instance(EnsureInstanceRequest {
                    base_dir: self.config.workspace.base_dir.clone(),
                    execution_instance_id: payload.execution_instance_id,
                })?;
                Ok(RuntimeCommandOutcome {
                    command_id: command.id,
                    accepted: true,
                    run_id: None,
                })
            }
            RuntimeCommandType::Unsupported(_) => Ok(RuntimeCommandOutcome {
                command_id: command.id,
                accepted: false,
                run_id: None,
            }),
        }
    }
}
```

- [ ] **Step 5: Implement input command handling**

Add these methods to `RuntimeCommandExecutor` in `executor.rs`:

```rust
impl RuntimeCommandExecutor {
    async fn handle_input_command(&self, command: RuntimeCommand) -> anyhow::Result<RuntimeCommandOutcome> {
        let command_id = command.id.clone();
        let payload = match RuntimeSessionCommandPayload::from_command(&command) {
            Ok(payload) => payload,
            Err(error) => {
                self.registry.record_rejection(&command_id, error.to_string());
                return Err(error);
            }
        };
        let provider_prompt = payload
            .provider_prompt()
            .ok_or_else(|| anyhow::anyhow!("prompt or input is required"))?;

        let instance = ensure_instance(EnsureInstanceRequest {
            base_dir: self.config.workspace.base_dir.clone(),
            execution_instance_id: payload.execution_instance_id.clone(),
        })?;

        let session_id = self.resolve_session_id(&payload)?;
        let continue_session = matches!(
            payload.session_policy.mode,
            SessionPolicyMode::Resume | SessionPolicyMode::ReuseLatest
        ) || matches!(command.command_type, RuntimeCommandType::SendInput);

        let spec = RunSpec {
            provider_kind: payload.provider_kind().to_string(),
            workspace_path: instance.agent_home_dir,
            prompt: provider_prompt,
            session_id: session_id.clone(),
            continue_session,
            model: payload.model.clone(),
            command_context: Some(RuntimeCommandRunContext {
                command_id: payload.command_id.clone(),
                digital_employee_id: payload.digital_employee_id.clone(),
                execution_instance_id: payload.execution_instance_id.clone(),
                provider_type: payload.provider_type.clone(),
                session_policy: serde_json::to_value(&payload.session_policy)?,
                context_refs: payload.context_refs.clone(),
                artifact_refs: payload.artifact_refs.clone(),
                metadata: payload.metadata.clone(),
            }),
        };

        let snapshot = self.runs.start_run(spec.clone(), None).await?;
        self.registry.record_run_started(RuntimeRunBinding {
            command_id: payload.command_id.clone(),
            run_id: snapshot.id.clone(),
            execution_instance_id: payload.execution_instance_id.clone(),
            provider_type: payload.provider_type.clone(),
            provider_session_id: session_id,
        });
        self.spawn_provider_run(snapshot.id.clone(), spec, payload).await;

        Ok(RuntimeCommandOutcome {
            command_id,
            accepted: true,
            run_id: Some(snapshot.id),
        })
    }

    fn resolve_session_id(&self, payload: &RuntimeSessionCommandPayload) -> anyhow::Result<Option<String>> {
        if let Some(provider_session_id) = payload
            .session_policy
            .provider_session_id
            .as_deref()
            .map(str::trim)
            .filter(|value| !value.is_empty())
        {
            return Ok(Some(provider_session_id.to_string()));
        }
        if matches!(payload.session_policy.mode, SessionPolicyMode::ReuseLatest) {
            return self
                .registry
                .latest_provider_session(&payload.execution_instance_id, &payload.provider_type)
                .map(Some)
                .ok_or_else(|| anyhow::anyhow!("no latest provider session for reuse_latest"));
        }
        if matches!(payload.session_policy.mode, SessionPolicyMode::Resume) {
            anyhow::bail!("provider_session_id is required for resume");
        }
        Ok(None)
    }
}
```

- [ ] **Step 6: Implement provider spawn and event recording**

Add this code to `executor.rs`:

```rust
impl RuntimeCommandExecutor {
    async fn spawn_provider_run(
        &self,
        run_id: String,
        spec: RunSpec,
        payload: RuntimeSessionCommandPayload,
    ) {
        let runs = self.runs.clone();
        let registry = self.registry.clone();
        let config = self.config.clone();
        tokio::spawn(async move {
            let result = run_provider_stream(&config, runs.clone(), registry.clone(), run_id.clone(), spec).await;
            if let Err(error) = result {
                if let Some(snapshot) = runs.get_run(&run_id).await {
                    if snapshot.status != RunStatus::Cancelled {
                        let _ = runs.finish_failed(&run_id, error.to_string()).await;
                    }
                }
            }
            registry.record_run_finished(&run_id);
            let _ = payload;
        });
    }
}

async fn run_provider_stream(
    config: &RuntimeConfig,
    runs: RuntimeRunStore,
    registry: RuntimeCommandRegistry,
    run_id: String,
    spec: RunSpec,
) -> anyhow::Result<()> {
    let provider = select_provider(&spec.provider_kind, config)?;
    let provider_run = provider
        .start(ProviderRequest {
            prompt: spec.prompt,
            workspace_path: spec.workspace_path,
            session_id: spec.session_id,
            continue_session: spec.continue_session,
            model: spec.model,
        })
        .await?;

    runs.attach_handle(&run_id, provider_run.handle).await?;
    provider_run
        .events
        .try_for_each(|event| {
            let runs = runs.clone();
            let registry = registry.clone();
            let run_id = run_id.clone();
            async move {
                if let ProviderEvent::SessionStarted { session_id } = &event {
                    registry.record_provider_session(&run_id, session_id);
                }
                runs.record_event(&run_id, event).await?;
                Ok(())
            }
        })
        .await
}

fn select_provider(provider_kind: &str, config: &RuntimeConfig) -> anyhow::Result<Box<dyn ProviderAdapter>> {
    match provider_kind {
        "claude" => {
            if !config.providers.claude_code.enabled {
                anyhow::bail!("Claude Code provider is disabled");
            }
            Ok(Box::new(ClaudeProvider::new(
                config.providers.claude_code.binary_path.clone(),
            )))
        }
        "opencode" => {
            if !config.providers.opencode.enabled {
                anyhow::bail!("OpenCode provider is disabled");
            }
            Ok(Box::new(OpenCodeProvider::new(
                config.providers.opencode.binary_path.clone(),
            )))
        }
        _ => anyhow::bail!("unsupported provider kind: {provider_kind}"),
    }
}
```

- [ ] **Step 7: Implement stop command handling**

Add this method to `executor.rs`:

```rust
impl RuntimeCommandExecutor {
    async fn handle_stop_command(&self, command: RuntimeCommand) -> anyhow::Result<RuntimeCommandOutcome> {
        let command_id = command.id.clone();
        let payload = match RuntimeSessionCommandPayload::from_command(&command) {
            Ok(payload) => payload,
            Err(error) => {
                self.registry.record_rejection(&command_id, error.to_string());
                return Err(error);
            }
        };
        let run_id = self.registry.active_run(ActiveRunLookup {
            command_id: Some(&payload.command_id),
            provider_session_id: payload.session_policy.provider_session_id.as_deref(),
            execution_instance_id: &payload.execution_instance_id,
            provider_type: &payload.provider_type,
        });
        let Some(run_id) = run_id else {
            self.registry
                .record_rejection(&payload.command_id, "active run not found for stop_session");
            anyhow::bail!("active run not found for stop_session");
        };
        self.runs
            .cancel_run(&run_id, Some(format!("stopped by command {}", payload.command_id)))
            .await?;
        self.registry.record_run_finished(&run_id);
        Ok(RuntimeCommandOutcome {
            command_id,
            accepted: true,
            run_id: Some(run_id),
        })
    }
}
```

- [ ] **Step 8: Run executor tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_executor_test
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add apps/runtime-agent/src/commands/mod.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat(runtime-agent): execute runtime session commands"
```

---

### Task 5: Wire Executor Into Runtime WebSocket Loop

**Files:**
- Modify: `apps/runtime-agent/src/controlplane/ws.rs`
- Modify: `apps/runtime-agent/tests/controlplane_client_test.rs`
- Test: existing unit tests inside `apps/runtime-agent/src/controlplane/ws.rs`

- [ ] **Step 1: Update WebSocket tests to expect real command handling path**

In `apps/runtime-agent/src/controlplane/ws.rs`, update the test module imports:

```rust
use super::{handle_text_command, run_command_loop_once, runtime_ws_url};
use crate::commands::executor::RuntimeCommandExecutor;
```

In `handle_text_command_ignores_unsupported_command_types`, replace:

```rust
handle_text_command(
    &config,
    r#"{"id":"cmd-legacy","type":"task.claim","payload":{}}"#,
)
.expect("unsupported command should be ignored");
```

with:

```rust
let executor = RuntimeCommandExecutor::new(config);
handle_text_command(
    &executor,
    r#"{"id":"cmd-legacy","type":"task.claim","payload":{}}"#,
)
.await
.expect("unsupported command should be ignored");
```

Change the test from `#[test]` to:

```rust
#[tokio::test]
```

- [ ] **Step 2: Run the WebSocket tests to verify compile failure**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml controlplane::ws
```

Expected: FAIL because `handle_text_command` still takes `&RuntimeConfig` and is not async.

- [ ] **Step 3: Update WebSocket command loop to use executor**

In `apps/runtime-agent/src/controlplane/ws.rs`, replace the old imports:

```rust
use crate::controlplane::models::{EnsureInstanceCommand, RuntimeCommand, RuntimeCommandType};
use crate::instances::{EnsureInstanceRequest, ensure_instance};
```

with:

```rust
use crate::commands::executor::RuntimeCommandExecutor;
use crate::controlplane::models::RuntimeCommand;
```

In `run_command_loop`, create one executor before reconnect loop:

```rust
let executor = RuntimeCommandExecutor::new(config);
loop {
    if let Err(error) = run_command_loop_once(&executor, &ws_url, &authorization).await {
        eprintln!("Runtime command loop connection failed: {}", error);
    }
    tokio::time::sleep(COMMAND_LOOP_RECONNECT_DELAY).await;
}
```

Change `run_command_loop_once` signature to:

```rust
async fn run_command_loop_once(
    executor: &RuntimeCommandExecutor,
    ws_url: &str,
    authorization: &HeaderValue,
) -> Result<()>
```

Inside the message loop, replace:

```rust
if let Err(error) = handle_text_command(config, text) {
    eprintln!("Runtime command handling failed: {}", error);
}
```

with:

```rust
if let Err(error) = handle_text_command(executor, text).await {
    eprintln!("Runtime command handling failed: {}", error);
}
```

Replace `handle_text_command` and remove the old `handle_command` function:

```rust
async fn handle_text_command(executor: &RuntimeCommandExecutor, text: &str) -> Result<()> {
    let command: RuntimeCommand = serde_json::from_str(text)?;
    executor.handle_command(command).await?;
    Ok(())
}
```

- [ ] **Step 4: Update `command_loop_continues_after_bad_commands`**

Inside the test after config setup, create executor:

```rust
let executor = RuntimeCommandExecutor::new(config.clone());
```

Change the call:

```rust
run_command_loop_once(
    &executor,
    &runtime_ws_url(&config.runtime.control_plane_url).expect("ws url"),
    &authorization,
)
.await
.expect("command loop once");
```

- [ ] **Step 5: Run WebSocket tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml controlplane::ws
```

Expected: PASS.

- [ ] **Step 6: Run runtime-agent test suite**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/runtime-agent/src/controlplane/ws.rs apps/runtime-agent/tests/controlplane_client_test.rs
git commit -m "feat(runtime-agent): handle websocket runtime commands"
```

---

### Task 6: Changelog and Verification

**Files:**
- Modify: `CHANGELOG.md`
- Verify: `apps/runtime-agent` tests

- [ ] **Step 1: Get local Asia/Shanghai timestamp**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Expected: prints a timestamp like `2026-06-04 16:30`.

- [ ] **Step 2: Update changelog**

Add this entry near the top of `CHANGELOG.md` with the exact timestamp printed in Step 1. If Step 1 prints `2026-06-04 02:17`, add:

```markdown
- 2026-06-04 02:17 Runtime Agent 新增 runtime command execution layer，支持 `start_session`、`resume_session`、`send_input`、`stop_session` 在本地解析 payload、驱动 Provider run、维护 command/session/run 映射并取消 active run。
```

- [ ] **Step 3: Run focused command tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_payload_test --test runtime_command_registry_test --test runtime_command_executor_test
```

Expected: PASS.

- [ ] **Step 4: Run full runtime-agent test suite**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: PASS.

- [ ] **Step 5: Inspect worktree scope**

Run:

```bash
git status --short
```

Expected: only files touched by this runtime command implementation plus any pre-existing unrelated user changes. Do not stage `.scratch/` or unrelated team-management plan files.

- [ ] **Step 6: Commit changelog**

```bash
git add CHANGELOG.md
git commit -m "docs: record runtime command execution layer"
```

---

## Self-Review Checklist

- Spec coverage:
  - Runtime Agent only: covered by Tasks 1-6.
  - Payload required fields: covered by Task 2 tests.
  - `start_session`、`resume_session`、`send_input`: covered by Task 4 executor tests and implementation.
  - `stop_session`: covered by Task 4 cancellation test.
  - Runtime does not judge tenant/team/approval: executor only validates field shape, provider availability and local execution state.
  - Reference repos as experience only: captured in the design spec; plan does not import external project code.
- Placeholder scan:
  - No unresolved placeholder markers or vague "add tests" steps.
  - Each code-changing step includes concrete code or exact replacement snippets.
- Type consistency:
  - `RuntimeCommandRunContext` is used in `RunSpec` and `RunSnapshot`.
  - `RuntimeSessionCommandPayload` is used by `RuntimeCommandExecutor`.
  - `RuntimeCommandRegistry` methods used in tests match the implementation snippets.
  - `RuntimeCommandExecutor::runs()` and `RuntimeCommandExecutor::registry()` are exposed for tests.
