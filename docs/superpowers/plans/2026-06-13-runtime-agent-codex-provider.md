# Runtime Agent Codex Provider Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Codex as a first-class Runtime Agent provider with configuration, capability reporting, command execution, true resume support, workspace materialization, HTTP smoke support, CLI run support, and runtime-agent verification.

**Architecture:** Add a small provider catalog/selector inside `apps/runtime-agent/src/providers/` so provider metadata and adapter selection stop being duplicated across daemon, command executor, task executor, HTTP server, and CLI paths. Add `providers/codex.rs` as a CLI adapter that maps `codex exec --json` JSONL into existing `ProviderEvent` values and uses Runtime-governed local execution flags. Keep Control Plane contracts unchanged; Codex is introduced through existing string provider type and capability reporting.

**Tech Stack:** Rust 2024, Tokio process streaming, serde/serde_json, existing `ProviderAdapter` trait, existing Runtime Agent integration tests, `cargo test --manifest-path apps/runtime-agent/Cargo.toml`, `corepack pnpm verify:runtime-agent`.

---

## Scope Check

The spec is one subsystem: Runtime Agent provider support. It touches several runtime-agent paths because provider support is currently duplicated, but every task produces working, testable Runtime Agent behavior without changing Control Plane contracts or Web UI.

## File Structure

- Create `apps/runtime-agent/src/providers/catalog.rs`
  - Owns provider constants, provider descriptors, provider config access helpers, provider kind lookup, adapter selection, and provider health input data.
- Create `apps/runtime-agent/src/providers/codex.rs`
  - Owns Codex command construction, JSONL event parsing, and `ProviderAdapter` implementation.
- Modify `apps/runtime-agent/src/providers/mod.rs`
  - Exports `catalog` and `codex`.
- Modify `apps/runtime-agent/src/config.rs`
  - Adds `providers.codex`, env overrides, CLI override storage, and HTTP config fields.
- Modify `apps/runtime-agent/config.example.yaml`
  - Documents Codex provider config.
- Modify `apps/runtime-agent/src/commands/payload.rs`
  - Uses catalog for provider type to provider kind mapping.
- Modify `apps/runtime-agent/src/commands/executor.rs`
  - Uses catalog selector instead of local Claude/OpenCode match.
- Modify `apps/runtime-agent/src/executor/task.rs`
  - Uses catalog selector instead of local Claude/OpenCode match.
- Modify `apps/runtime-agent/src/daemon.rs`
  - Builds supported providers and provider capabilities from catalog.
- Modify `apps/runtime-agent/src/server.rs`
  - Extends HTTP config and smoke run path to Codex, preferably through catalog helpers.
- Modify `apps/runtime-agent/src/main.rs`
  - Adds `--codex-bin` and `run --provider codex`.
- Modify `apps/runtime-agent/src/workspace_files.rs`
  - Adds `ProviderHomeKind::Codex`, `.codex` private dir creation, and reserved path protection.
- Modify tests under `apps/runtime-agent/tests/`
  - Add focused tests before each implementation slice.

## Task 1: Config And Provider Catalog

**Files:**
- Create: `apps/runtime-agent/src/providers/catalog.rs`
- Modify: `apps/runtime-agent/src/providers/mod.rs`
- Modify: `apps/runtime-agent/src/config.rs`
- Modify: `apps/runtime-agent/src/main.rs`
- Modify: `apps/runtime-agent/config.example.yaml`
- Test: `apps/runtime-agent/tests/daemon_test.rs`

- [ ] **Step 1: Add failing config test for Codex file/env/override loading**

Append this assertion block to `config_loads_runtime_yaml_and_env_overrides` in `apps/runtime-agent/tests/daemon_test.rs` after the existing `opencode` assertions:

```rust
    assert!(config.providers.codex.enabled);
    assert_eq!(
        config.providers.codex.binary_path,
        std::path::PathBuf::from("/usr/local/bin/env-codex")
    );
    assert_eq!(config.providers.codex.timeout, 240);
```

Update the YAML in the same test under `providers:`:

```yaml
  codex:
    enabled: true
    binary_path: /usr/local/bin/file-codex
    timeout: 240
```

Add this env override to the `RuntimeConfig::load_with_env` call in that test:

```rust
            (
                "RUNTIME_AGENT_PROVIDER_CODEX_BINARY",
                "/usr/local/bin/env-codex",
            ),
```

- [ ] **Step 2: Run the config test and verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test daemon_test config_loads_runtime_yaml_and_env_overrides
```

Expected: FAIL with errors that `ProvidersSection` has no `codex` field or equivalent compile errors.

- [ ] **Step 3: Add Codex config fields**

Modify `apps/runtime-agent/src/config.rs`:

```rust
pub struct ProvidersSection {
    pub claude_code: ProviderSection,
    pub opencode: ProviderSection,
    pub codex: ProviderSection,
}
```

```rust
pub struct RuntimeConfigOverrides {
    pub node_id: Option<String>,
    pub bootstrap_key: Option<String>,
    pub http_addr: Option<SocketAddr>,
    pub run_log_dir: Option<PathBuf>,
    pub claude_bin: Option<PathBuf>,
    pub opencode_bin: Option<PathBuf>,
    pub codex_bin: Option<PathBuf>,
}
```

```rust
struct FileProvidersSection {
    claude_code: Option<FileProviderSection>,
    opencode: Option<FileProviderSection>,
    codex: Option<FileProviderSection>,
}
```

In `apply_file`, add:

```rust
            if let Some(codex) = providers.codex {
                self.providers.codex.apply_file(codex);
            }
```

In `apply_env_value`, add:

```rust
            "RUNTIME_AGENT_PROVIDER_CODEX_ENABLED" => {
                self.providers.codex.enabled = parse_env(key, value)?;
            }
            "RUNTIME_AGENT_PROVIDER_CODEX_BINARY" => {
                self.providers.codex.binary_path = PathBuf::from(value);
            }
            "RUNTIME_AGENT_PROVIDER_CODEX_TIMEOUT" => {
                self.providers.codex.timeout = parse_env(key, value)?;
            }
```

In `apply_overrides`, add:

```rust
        apply_path(&mut self.providers.codex.binary_path, overrides.codex_bin);
```

In `validate`, add:

```rust
        if self.providers.codex.binary_path.as_os_str().is_empty() {
            anyhow::bail!("codex binary path is required");
        }
```

In `Default for RuntimeConfig`, add:

```rust
                codex: ProviderSection {
                    enabled: false,
                    binary_path: PathBuf::from("codex"),
                    timeout: 3600,
                },
```

Do not change `http_config` in this task. Task 6 adds `codex_bin` there at the same time it extends `RuntimeHttpConfig`, so Task 1 remains compileable on its own.

Modify the `RuntimeConfigOverrides` struct literal in `apps/runtime-agent/src/main.rs` so the new field is initialized before the CLI flag exists:

```rust
            codex_bin: None,
```

- [ ] **Step 4: Add provider catalog skeleton**

Create `apps/runtime-agent/src/providers/catalog.rs`:

```rust
use std::path::PathBuf;

use crate::config::{ProviderSection, RuntimeConfig};
use crate::health::ProviderHealthProbe;
use crate::providers::claude::ClaudeProvider;
use crate::providers::opencode::OpenCodeProvider;
use crate::providers::ProviderAdapter;

pub const CLAUDE_CODE_PROVIDER_TYPE: &str = "claude-code";
pub const OPENCODE_PROVIDER_TYPE: &str = "opencode";
pub const CODEX_PROVIDER_TYPE: &str = "codex";

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct ProviderDescriptor {
    pub provider_type: &'static str,
    pub provider_kind: &'static str,
    pub health_kind: &'static str,
}

pub const PROVIDERS: &[ProviderDescriptor] = &[
    ProviderDescriptor {
        provider_type: CLAUDE_CODE_PROVIDER_TYPE,
        provider_kind: "claude",
        health_kind: "claude",
    },
    ProviderDescriptor {
        provider_type: OPENCODE_PROVIDER_TYPE,
        provider_kind: "opencode",
        health_kind: "opencode",
    },
    ProviderDescriptor {
        provider_type: CODEX_PROVIDER_TYPE,
        provider_kind: "codex",
        health_kind: "codex",
    },
];

pub fn provider_descriptor(provider_type: &str) -> Option<&'static ProviderDescriptor> {
    PROVIDERS
        .iter()
        .find(|descriptor| descriptor.provider_type == provider_type)
}

pub fn provider_kind(provider_type: &str) -> &'static str {
    provider_descriptor(provider_type)
        .map(|descriptor| descriptor.provider_kind)
        .unwrap_or("unsupported")
}

pub fn provider_section<'a>(
    config: &'a RuntimeConfig,
    provider_type: &str,
) -> Option<&'a ProviderSection> {
    match provider_type {
        CLAUDE_CODE_PROVIDER_TYPE => Some(&config.providers.claude_code),
        OPENCODE_PROVIDER_TYPE => Some(&config.providers.opencode),
        CODEX_PROVIDER_TYPE => Some(&config.providers.codex),
        _ => None,
    }
}

pub fn supported_provider_types(config: &RuntimeConfig) -> Vec<String> {
    PROVIDERS
        .iter()
        .filter_map(|descriptor| {
            let section = provider_section(config, descriptor.provider_type)?;
            section.enabled.then(|| descriptor.provider_type.to_string())
        })
        .collect()
}

pub fn provider_health_probe(
    config: &RuntimeConfig,
    descriptor: &ProviderDescriptor,
) -> Option<ProviderHealthProbe> {
    let section = provider_section(config, descriptor.provider_type)?;
    section.enabled.then(|| ProviderHealthProbe {
        kind: descriptor.health_kind.to_string(),
        bin_path: section.binary_path.clone(),
    })
}

pub fn provider_binary_path(config: &RuntimeConfig, provider_type: &str) -> Option<PathBuf> {
    provider_section(config, provider_type).map(|section| section.binary_path.clone())
}

pub fn select_provider(
    config: &RuntimeConfig,
    provider_type: &str,
) -> anyhow::Result<Box<dyn ProviderAdapter>> {
    let section = provider_section(config, provider_type)
        .ok_or_else(|| anyhow::anyhow!("unsupported provider_type: {provider_type}"))?;
    if !section.enabled {
        return Err(anyhow::anyhow!(
            "{} provider is disabled",
            provider_type_label(provider_type)
        ));
    }

    match provider_type {
        CLAUDE_CODE_PROVIDER_TYPE => Ok(Box::new(ClaudeProvider::new(section.binary_path.clone()))),
        OPENCODE_PROVIDER_TYPE => Ok(Box::new(OpenCodeProvider::new(section.binary_path.clone()))),
        CODEX_PROVIDER_TYPE => Err(anyhow::anyhow!("Codex provider is not implemented yet")),
        _ => Err(anyhow::anyhow!("unsupported provider_type: {provider_type}")),
    }
}

fn provider_type_label(provider_type: &str) -> &'static str {
    match provider_type {
        CLAUDE_CODE_PROVIDER_TYPE => "Claude Code",
        OPENCODE_PROVIDER_TYPE => "OpenCode",
        CODEX_PROVIDER_TYPE => "Codex",
        _ => "Unknown",
    }
}
```

Modify `apps/runtime-agent/src/providers/mod.rs`:

```rust
pub mod catalog;
pub mod claude;
pub mod opencode;
```

- [ ] **Step 5: Document Codex config example**

Modify `apps/runtime-agent/config.example.yaml`:

```yaml
  codex:
    enabled: false
    binary_path: codex
    timeout: 3600
```

- [ ] **Step 6: Run the config test and verify it passes**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test daemon_test config_loads_runtime_yaml_and_env_overrides
```

Expected: PASS.

- [ ] **Step 7: Commit Task 1**

```bash
git add apps/runtime-agent/src/config.rs apps/runtime-agent/src/main.rs apps/runtime-agent/src/providers/mod.rs apps/runtime-agent/src/providers/catalog.rs apps/runtime-agent/config.example.yaml apps/runtime-agent/tests/daemon_test.rs
git commit -m "feat: add runtime provider catalog config"
```

## Task 2: Codex Adapter Command Construction And JSONL Parsing

**Files:**
- Create: `apps/runtime-agent/src/providers/codex.rs`
- Modify: `apps/runtime-agent/src/providers/mod.rs`
- Modify: `apps/runtime-agent/src/providers/catalog.rs`
- Test: `apps/runtime-agent/tests/provider_command_test.rs`
- Test: `apps/runtime-agent/tests/provider_event_test.rs`
- Test: `apps/runtime-agent/tests/provider_spawn_test.rs`

- [ ] **Step 1: Add failing command construction tests**

Modify imports in `apps/runtime-agent/tests/provider_command_test.rs`:

```rust
use superteam_runtime_agent::providers::codex::CodexProvider;
```

Append tests:

```rust
#[test]
fn codex_new_turn_uses_runtime_governed_exec_flags() {
    let provider = CodexProvider::new("codex");
    let command = provider.build_command(&request(None, false));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert_eq!(args[0], "exec");
    assert!(args.iter().any(|arg| arg == "--json"));
    assert!(
        args.windows(2)
            .any(|window| window == ["--cd", "/tmp/workspace"])
    );
    assert!(
        args.windows(2)
            .any(|window| window == ["--ask-for-approval", "never"])
    );
    assert!(
        args.windows(2)
            .any(|window| window == ["--sandbox", "danger-full-access"])
    );
    assert!(args.windows(2).any(|window| window == ["--model", "model-a"]));
    assert_eq!(args.last().map(String::as_str), Some("hello"));
}

#[test]
fn codex_resume_uses_resume_subcommand_and_bypass_flag() {
    let provider = CodexProvider::new("codex");
    let command = provider.build_command(&request(Some("codex-session"), true));
    let args: Vec<_> = command
        .as_std()
        .get_args()
        .map(|arg| arg.to_string_lossy().to_string())
        .collect();

    assert_eq!(args[0], "exec");
    assert_eq!(args[1], "resume");
    assert_eq!(args[2], "codex-session");
    assert!(args.iter().any(|arg| arg == "--json"));
    assert!(
        args.iter()
            .any(|arg| arg == "--dangerously-bypass-approvals-and-sandbox")
    );
    assert!(!args.iter().any(|arg| arg == "--cd"));
    assert!(args.windows(2).any(|window| window == ["--model", "model-a"]));
    assert_eq!(args.last().map(String::as_str), Some("hello"));
}
```

- [ ] **Step 2: Run command tests and verify they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test provider_command_test codex_
```

Expected: FAIL because `providers::codex::CodexProvider` does not exist.

- [ ] **Step 3: Add failing Codex event parsing tests**

Modify imports in `apps/runtime-agent/tests/provider_event_test.rs`:

```rust
use superteam_runtime_agent::providers::codex::parse_codex_event;
```

Append tests:

```rust
#[test]
fn parses_codex_session_text_and_completion_events() {
    let session = parse_codex_event(r#"{"type":"session","session_id":"codex-session"}"#)
        .expect("valid json")
        .expect("event");
    let text = parse_codex_event(r#"{"type":"message.delta","delta":"hello from codex"}"#)
        .expect("valid json")
        .expect("event");
    let completed = parse_codex_event(r#"{"type":"turn.completed","summary":"done"}"#)
        .expect("valid json")
        .expect("event");

    assert_eq!(
        session,
        ProviderEvent::SessionStarted {
            session_id: "codex-session".to_string(),
            session_state: None,
        }
    );
    assert_eq!(
        text,
        ProviderEvent::TextDelta {
            text: "hello from codex".to_string()
        }
    );
    assert_eq!(
        completed,
        ProviderEvent::TurnCompleted {
            summary: Some("done".to_string())
        }
    );
}

#[test]
fn codex_error_event_returns_error() {
    let error = parse_codex_event(r#"{"type":"error","message":"codex failed"}"#)
        .expect_err("codex error event should fail");

    assert!(error.to_string().contains("codex failed"));
}
```

- [ ] **Step 4: Add Codex adapter implementation**

Create `apps/runtime-agent/src/providers/codex.rs`:

```rust
use std::path::PathBuf;

use anyhow::Context;
use async_trait::async_trait;
use serde_json::Value;
use tokio::process::Command;

use crate::events::ProviderEvent;
use crate::providers::{ProviderAdapter, ProviderRequest, ProviderRun, stream_child_events};

#[derive(Debug, Clone)]
pub struct CodexProvider {
    bin_path: PathBuf,
}

impl CodexProvider {
    pub fn new(bin_path: impl Into<PathBuf>) -> Self {
        Self {
            bin_path: bin_path.into(),
        }
    }

    pub fn build_command(&self, request: &ProviderRequest) -> Command {
        let mut command = Command::new(&self.bin_path);
        command.current_dir(&request.workspace_path);
        command.arg("exec");
        if request.continue_session {
            command.arg("resume");
            if let Some(session_id) = &request.session_id {
                command.arg(session_id);
            }
            command.arg("--json");
            command.arg("--dangerously-bypass-approvals-and-sandbox");
        } else {
            command.arg("--json");
            command.arg("--cd").arg(&request.workspace_path);
            command.arg("--ask-for-approval").arg("never");
            command.arg("--sandbox").arg("danger-full-access");
        }
        if let Some(model) = &request.model {
            command.arg("--model").arg(model);
        }
        command.arg(&request.prompt);
        command
    }
}

#[async_trait]
impl ProviderAdapter for CodexProvider {
    async fn start(&self, request: ProviderRequest) -> anyhow::Result<ProviderRun> {
        let mut command = self.build_command(&request);
        command.stdout(std::process::Stdio::piped());
        command.stderr(std::process::Stdio::piped());
        let mut child = command.spawn().context("failed to spawn codex")?;
        let stdout = child
            .stdout
            .take()
            .context("failed to capture codex stdout")?;
        let stderr = child
            .stderr
            .take()
            .context("failed to capture codex stderr")?;
        Ok(stream_child_events(
            "codex",
            parse_codex_event,
            child,
            stdout,
            stderr,
        ))
    }
}

pub fn parse_codex_event(value: &str) -> anyhow::Result<Option<ProviderEvent>> {
    let event: Value = serde_json::from_str(value)?;
    let event_type = event
        .get("type")
        .and_then(|value| value.as_str())
        .unwrap_or_default();

    if matches!(event_type, "error" | "turn.error" | "failed" | "failure") {
        anyhow::bail!(
            "{}",
            first_string(&event, &["message", "error", "reason"]).unwrap_or("codex failed")
        );
    }

    if let Some(session_id) = extract_session_id(&event) {
        return Ok(Some(ProviderEvent::SessionStarted {
            session_id,
            session_state: None,
        }));
    }

    if let Some(text) = extract_text(&event) {
        return Ok(Some(ProviderEvent::TextDelta { text }));
    }

    if matches!(
        event_type,
        "turn.completed" | "turn_complete" | "completed" | "result" | "done"
    ) {
        return Ok(Some(ProviderEvent::TurnCompleted {
            summary: extract_summary(&event),
        }));
    }

    Ok(None)
}

fn extract_session_id(event: &Value) -> Option<String> {
    first_string(event, &["session_id", "sessionId", "thread_id", "threadId"])
        .or_else(|| nested_string(event, &["session", "id"]))
        .or_else(|| nested_string(event, &["thread", "id"]))
        .map(ToString::to_string)
}

fn extract_text(event: &Value) -> Option<String> {
    first_string(event, &["text", "delta", "content"])
        .or_else(|| nested_string(event, &["message", "content"]))
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn extract_summary(event: &Value) -> Option<String> {
    first_string(event, &["summary", "result", "final_message"])
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(ToString::to_string)
}

fn first_string<'a>(value: &'a Value, keys: &[&str]) -> Option<&'a str> {
    keys.iter()
        .find_map(|key| value.get(*key).and_then(|value| value.as_str()))
}

fn nested_string<'a>(value: &'a Value, path: &[&str]) -> Option<&'a str> {
    let mut current = value;
    for key in path {
        current = current.get(*key)?;
    }
    current.as_str()
}
```

Modify `apps/runtime-agent/src/providers/mod.rs`:

```rust
pub mod catalog;
pub mod claude;
pub mod codex;
pub mod opencode;
```

- [ ] **Step 5: Wire Codex into catalog selector**

Modify imports in `apps/runtime-agent/src/providers/catalog.rs`:

```rust
use crate::providers::codex::CodexProvider;
```

Replace the Codex arm in `select_provider`:

```rust
        CODEX_PROVIDER_TYPE => Ok(Box::new(CodexProvider::new(section.binary_path.clone()))),
```

- [ ] **Step 6: Add Codex fake spawn test**

Modify imports in `apps/runtime-agent/tests/provider_spawn_test.rs`:

```rust
use superteam_runtime_agent::providers::codex::CodexProvider;
```

Append:

```rust
#[tokio::test]
async fn codex_provider_streams_fake_cli_events() {
    let temp = TempDir::new().expect("tempdir");
    let script = make_script(
        temp.path(),
        "fake-codex",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session","session_id":"codex-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from codex"}'
printf '%s\n' '{"type":"turn.completed","summary":"done"}'
"#,
    );
    let provider = CodexProvider::new(script);

    let events: Vec<ProviderEvent> = provider
        .run(request(temp.path()))
        .await
        .expect("run fake codex")
        .try_collect()
        .await
        .expect("collect fake codex events");

    assert_eq!(
        events,
        vec![
            ProviderEvent::SessionStarted {
                session_id: "codex-session".to_string(),
                session_state: None,
            },
            ProviderEvent::TextDelta {
                text: "hello from codex".to_string()
            },
            ProviderEvent::TurnCompleted {
                summary: Some("done".to_string())
            },
        ]
    );
}
```

- [ ] **Step 7: Run adapter tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test provider_command_test --test provider_event_test --test provider_spawn_test
```

Expected: PASS.

- [ ] **Step 8: Commit Task 2**

```bash
git add apps/runtime-agent/src/providers/mod.rs apps/runtime-agent/src/providers/catalog.rs apps/runtime-agent/src/providers/codex.rs apps/runtime-agent/tests/provider_command_test.rs apps/runtime-agent/tests/provider_event_test.rs apps/runtime-agent/tests/provider_spawn_test.rs
git commit -m "feat: add codex provider adapter"
```

## Task 3: Runtime Command Payload And Executor Selection

**Files:**
- Modify: `apps/runtime-agent/src/commands/payload.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Test: `apps/runtime-agent/tests/runtime_command_payload_test.rs`
- Test: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Add failing payload test for Codex provider kind**

Append to `apps/runtime-agent/tests/runtime_command_payload_test.rs`:

```rust
#[test]
fn parses_codex_provider_kind() {
    let mut payload = valid_payload();
    payload["provider_type"] = json!("codex");

    let parsed = RuntimeSessionCommandPayload::from_command(&command(payload))
        .expect("valid codex payload");

    assert_eq!(parsed.provider_type, "codex");
    assert_eq!(parsed.provider_kind(), "codex");
}
```

- [ ] **Step 2: Run payload test and verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_payload_test parses_codex_provider_kind
```

Expected: FAIL with `unsupported provider_type: codex`.

- [ ] **Step 3: Use catalog in session payload provider kind**

Modify imports in `apps/runtime-agent/src/commands/payload.rs`:

```rust
use crate::providers::catalog;
```

Replace `provider_kind`:

```rust
    pub fn provider_kind(&self) -> &'static str {
        catalog::provider_kind(&self.provider_type)
    }
```

- [ ] **Step 4: Add failing executor test for Codex disabled error**

In `apps/runtime-agent/tests/runtime_command_executor_test.rs`, add this helper below `session_command_full`:

```rust
fn with_provider_type(mut command: RuntimeCommand, provider_type: &str) -> RuntimeCommand {
    command.payload["provider_type"] = serde_json::Value::String(provider_type.to_string());
    command
}
```

Append test:

```rust
#[tokio::test]
async fn start_session_rejects_disabled_codex_provider() {
    let temp = TempDir::new().expect("tempdir");
    let fake_claude = make_script(
        temp.path(),
        "fake-claude",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"system","session_id":"unused"}'
"#,
    );
    let executor = configure_runtime(&temp, fake_claude);
    let home = prepare_employee_home(&temp);
    let command = with_provider_type(
        session_command_in_home(
            &home,
            "cmd-codex-disabled",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("hello"),
            None,
        ),
        "codex",
    );

    let error = executor
        .handle_command(command)
        .await
        .expect_err("disabled codex should fail");

    assert!(error.to_string().contains("Codex provider is disabled"));
}
```

- [ ] **Step 5: Add failing executor test for enabled Codex run**

Append test:

```rust
#[tokio::test]
async fn start_session_runs_codex_provider() {
    let temp = TempDir::new().expect("tempdir");
    let fake_codex = make_script(
        temp.path(),
        "fake-codex",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session","session_id":"codex-runtime-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from codex runtime"}'
printf '%s\n' '{"type":"turn.completed","summary":"done"}'
"#,
    );
    let mut config = RuntimeConfig::default();
    config.runs.log_dir = temp.path().join("run-logs");
    config.workspace.base_dir = temp.path().join("workspaces");
    config.providers.claude_code.enabled = false;
    config.providers.claude_code.binary_path = temp.path().join("missing-claude");
    config.providers.opencode.enabled = false;
    config.providers.opencode.binary_path = temp.path().join("missing-opencode");
    config.providers.codex.enabled = true;
    config.providers.codex.binary_path = fake_codex;
    let executor = RuntimeCommandExecutor::new(config);
    let home = prepare_employee_home(&temp);
    let command = with_provider_type(
        session_command_in_home(
            &home,
            "cmd-codex-start",
            RuntimeCommandType::StartSession,
            "new",
            None,
            Some("hello"),
            None,
        ),
        "codex",
    );

    let outcome = executor
        .handle_command(command)
        .await
        .expect("codex command accepted");
    let run_id = outcome.run_id.expect("run id");
    let final_snapshot = wait_for_run_status(&executor.runs(), &run_id, RunStatus::Completed)
        .await
        .expect("completed codex run");

    assert_eq!(
        final_snapshot.provider_session_id.as_deref(),
        Some("codex-runtime-session")
    );
}
```

If `wait_for_run_status` does not exist, add it near other helpers:

```rust
async fn wait_for_run_status(
    runs: &RuntimeRunStore,
    run_id: &str,
    status: RunStatus,
) -> Option<RunSnapshot> {
    for _ in 0..150 {
        let snapshot = runs.get_run(run_id).await?;
        if snapshot.status == status {
            return Some(snapshot);
        }
        tokio::time::sleep(Duration::from_millis(20)).await;
    }
    None
}
```

- [ ] **Step 6: Use catalog selector in command executor**

Modify imports in `apps/runtime-agent/src/commands/executor.rs`:

```rust
use crate::providers::catalog;
use crate::providers::{ProviderAdapter, ProviderEventStream, ProviderRequest};
```

Remove direct imports:

```rust
use crate::providers::claude::ClaudeProvider;
use crate::providers::opencode::OpenCodeProvider;
```

Replace `select_provider` body:

```rust
        catalog::select_provider(&self.config, &payload.provider_type)
            .map_err(|error| self.recorded_error(command_id, error))
```

- [ ] **Step 7: Run command payload and executor tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test runtime_command_payload_test --test runtime_command_executor_test
```

Expected: PASS.

- [ ] **Step 8: Commit Task 3**

```bash
git add apps/runtime-agent/src/commands/payload.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/tests/runtime_command_payload_test.rs apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat: route runtime commands to codex provider"
```

## Task 4: Workspace Materialization For Codex

**Files:**
- Modify: `apps/runtime-agent/src/workspace_files.rs`
- Test: `apps/runtime-agent/tests/workspace_files_test.rs`

- [ ] **Step 1: Add failing workspace path and materialization tests**

In `apps/runtime-agent/tests/workspace_files_test.rs`, add `.codex/config.toml` to the reserved path list:

```rust
        ".codex/config.toml",
```

Append test:

```rust
#[test]
fn materialize_workspace_creates_codex_provider_dir() {
    let temp = tempfile::tempdir().unwrap();
    let home = temp.path().join("employee");
    std::fs::create_dir_all(&home).unwrap();

    let result = materialize_workspace(WorkspaceMaterializationPlan {
        agent_home_dir: home.clone(),
        provider_home: ProviderHomeKind::Codex,
        files: vec![agents_file("# Contract\n")],
    })
    .unwrap();

    assert_eq!(result.synced_files.len(), 1);
    assert!(home.join(".codex").is_dir());
    assert!(home.join("CLAUDE.md").exists());
}
```

- [ ] **Step 2: Run workspace tests and verify they fail**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test workspace_files_test
```

Expected: FAIL because `ProviderHomeKind::Codex` is missing and `.codex` is not reserved.

- [ ] **Step 3: Add Codex provider home support**

Modify `apps/runtime-agent/src/workspace_files.rs`:

```rust
pub enum ProviderHomeKind {
    ClaudeCode,
    OpenCode,
    Codex,
}
```

Update `provider_home_kind`:

```rust
        "codex" => Ok(ProviderHomeKind::Codex),
```

Update reserved first path component:

```rust
    if matches!(first, ".claude" | ".opencode" | ".codex" | ".git" | ".superteam") {
```

Update `provider_private_dir`:

```rust
        ProviderHomeKind::Codex => ".codex",
```

- [ ] **Step 4: Run workspace tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test workspace_files_test
```

Expected: PASS.

- [ ] **Step 5: Commit Task 4**

```bash
git add apps/runtime-agent/src/workspace_files.rs apps/runtime-agent/tests/workspace_files_test.rs
git commit -m "feat: materialize codex provider workspace"
```

## Task 5: Capability Reporting From Catalog

**Files:**
- Modify: `apps/runtime-agent/src/daemon.rs`
- Modify: `apps/runtime-agent/src/providers/catalog.rs`
- Test: `apps/runtime-agent/tests/daemon_test.rs`

- [ ] **Step 1: Add failing daemon capability test for Codex**

Add helper near other test helpers in `apps/runtime-agent/tests/daemon_test.rs`:

```rust
fn make_executable_script(dir: &tempfile::TempDir, name: &str, body: &str) -> std::path::PathBuf {
    use std::os::unix::fs::PermissionsExt;

    let path = dir.path().join(name);
    std::fs::write(&path, body).expect("write fake provider script");
    let mut permissions = std::fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    std::fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}
```

Append test:

```rust
#[tokio::test]
async fn runtime_daemon_reports_codex_provider_capability_when_enabled() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let (request_tx, request_rx) = oneshot::channel();

    tokio::spawn(async move {
        let (mut hello_socket, _) = listener.accept().await.unwrap();
        let hello_request = read_http_request(&mut hello_socket).await;
        write_json_response(
            &mut hello_socket,
            serde_json::json!({
                "enrollment": {
                    "id": "11111111-1111-4111-8111-111111111111",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "bootstrap_key_id": "44444444-4444-4444-8444-444444444444",
                    "status": "approved",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session": {
                    "id": "55555555-5555-4555-8555-555555555555",
                    "tenant_id": "22222222-2222-4222-8222-222222222222",
                    "runtime_node_id": "33333333-3333-4333-8333-333333333333",
                    "node_id": "node-1",
                    "enrollment_id": "11111111-1111-4111-8111-111111111111",
                    "expires_at": "2999-06-02T00:00:00Z",
                    "last_seen_at": "2026-06-02T00:00:00Z",
                    "created_at": "2026-06-02T00:00:00Z",
                    "updated_at": "2026-06-02T00:00:00Z"
                },
                "session_token": "session-token"
            }),
        )
        .await;

        let (mut capabilities_socket, _) = listener.accept().await.unwrap();
        let capabilities_request = read_http_request(&mut capabilities_socket).await;
        write_json_response(&mut capabilities_socket, serde_json::json!([])).await;

        let _ = request_tx.send((hello_request, capabilities_request));
    });

    let temp = tempfile::TempDir::new().expect("tempdir");
    let fake_codex = make_executable_script(
        &temp,
        "fake-codex",
        r#"#!/usr/bin/env bash
if [ "$1" = "--version" ]; then
  printf '%s\n' 'codex-cli 0.137.0'
fi
"#,
    );
    let mut config = RuntimeConfig::new("node-1").expect("valid config");
    config.runtime.control_plane_url = format!("http://{}", addr);
    config.runtime.bootstrap_key = "bootstrap-key".to_string();
    config.providers.claude_code.enabled = false;
    config.providers.opencode.enabled = false;
    config.providers.codex.enabled = true;
    config.providers.codex.binary_path = fake_codex;

    let daemon = RuntimeDaemon::new(config);
    let handle = tokio::spawn(async move { daemon.run().await });

    let (hello_request, capabilities_request) =
        tokio::time::timeout(std::time::Duration::from_secs(2), request_rx)
            .await
            .expect("server should capture requests")
            .expect("request pair");
    handle.abort();

    assert!(hello_request.contains(r#""supported_providers":["codex"]"#));
    let (_, capabilities_body) = capabilities_request
        .split_once("\r\n\r\n")
        .expect("capabilities body");
    let capabilities_body: serde_json::Value =
        serde_json::from_str(capabilities_body).expect("capabilities json");
    let capabilities = capabilities_body["capabilities"]
        .as_array()
        .expect("capabilities array");
    let codex = capabilities
        .iter()
        .find(|capability| capability["capability_key"] == "codex")
        .expect("codex capability");
    assert_eq!(codex["provider_type"], serde_json::json!("codex"));
    assert_eq!(codex["available"], serde_json::json!(true));
    assert_eq!(codex["provider_version"], serde_json::json!("codex-cli 0.137.0"));
}
```

- [ ] **Step 2: Run daemon Codex test and verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test daemon_test runtime_daemon_reports_codex_provider_capability_when_enabled
```

Expected: FAIL because daemon capability building is still hard-coded or `build_capabilities` is private and not catalog-driven.

- [ ] **Step 3: Add catalog capability source helpers**

Extend `apps/runtime-agent/src/providers/catalog.rs`:

```rust
pub fn configured_provider_descriptors() -> &'static [ProviderDescriptor] {
    PROVIDERS
}
```

- [ ] **Step 4: Refactor daemon supported providers**

Modify imports in `apps/runtime-agent/src/daemon.rs`:

```rust
use crate::providers::catalog;
```

Replace `build_supported_providers`:

```rust
fn build_supported_providers(config: &RuntimeConfig) -> Vec<String> {
    catalog::supported_provider_types(config)
}
```

- [ ] **Step 5: Refactor daemon provider capability building**

Replace the provider-health section at the top of `build_capabilities` in `apps/runtime-agent/src/daemon.rs`:

```rust
    for descriptor in catalog::configured_provider_descriptors() {
        let section = catalog::provider_section(config, descriptor.provider_type)
            .expect("catalog descriptor must have config section");
        let health = if section.enabled {
            Some(
                probe_provider_health(ProviderHealthProbe {
                    kind: descriptor.health_kind.to_string(),
                    bin_path: section.binary_path.clone(),
                })
                .await,
            )
        } else {
            None
        };
        capabilities.push(provider_capability(
            descriptor.provider_type,
            section.enabled,
            section.binary_path.display().to_string(),
            health,
        ));
    }
```

Remove the old `claude_health`, `opencode_health`, and two hard-coded `capabilities.push(provider_capability(...))` blocks.

- [ ] **Step 6: Run daemon tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test daemon_test
```

Expected: PASS.

- [ ] **Step 7: Commit Task 5**

```bash
git add apps/runtime-agent/src/daemon.rs apps/runtime-agent/src/providers/catalog.rs apps/runtime-agent/tests/daemon_test.rs
git commit -m "feat: report codex runtime capability"
```

## Task 6: HTTP Server And CLI Run Path

**Files:**
- Modify: `apps/runtime-agent/src/server.rs`
- Modify: `apps/runtime-agent/src/config.rs`
- Modify: `apps/runtime-agent/src/main.rs`
- Test: `apps/runtime-agent/tests/http_server_test.rs`
- Test: `apps/runtime-agent/tests/cli_run_test.rs`

- [ ] **Step 1: Add failing HTTP Codex run test**

Modify server config construction in existing `http_server_test.rs` tests by adding:

```rust
        codex_bin: temp.path().join("missing-codex"),
```

Append test:

```rust
#[tokio::test]
async fn http_server_creates_codex_run_and_replays_events() {
    let temp = TempDir::new().expect("tempdir");
    let fake_codex = make_script(
        &temp,
        "fake-codex",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session","session_id":"http-codex-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello over codex http"}'
printf '%s\n' '{"type":"turn.completed","summary":"codex http done"}'
"#,
    );

    let server = RuntimeHttpServer::bind_ephemeral(RuntimeHttpConfig {
        node_id: "node-http".to_string(),
        run_log_dir: temp.path().join("run-logs"),
        claude_bin: temp.path().join("missing-claude"),
        opencode_bin: temp.path().join("missing-opencode"),
        codex_bin: fake_codex,
    })
    .await
    .expect("bind server");
    let client = reqwest::Client::new();

    let response = client
        .post(format!("http://{}/runs", server.addr()))
        .json(&json!({
            "provider_kind": "codex",
            "workspace_path": temp.path(),
            "prompt": "hello",
            "continue_session": false
        }))
        .send()
        .await
        .expect("post run");
    assert_eq!(response.status(), StatusCode::ACCEPTED);
    let run: serde_json::Value = response.json().await.expect("run json");
    let run_id = run["id"].as_str().expect("run id");

    let mut final_run = serde_json::Value::Null;
    for _ in 0..150 {
        let snapshot: serde_json::Value = client
            .get(format!("http://{}/runs/{run_id}", server.addr()))
            .send()
            .await
            .expect("get run")
            .json()
            .await
            .expect("snapshot json");
        if snapshot["status"] == "completed" {
            final_run = snapshot;
            break;
        }
        tokio::time::sleep(std::time::Duration::from_millis(20)).await;
    }

    assert_eq!(final_run["status"], "completed");
    assert_eq!(final_run["provider_session_id"], "http-codex-session");
}
```

- [ ] **Step 2: Update HTTP config and provider handling**

Modify `apps/runtime-agent/src/server.rs` imports:

```rust
use crate::providers::catalog::{self, CODEX_PROVIDER_TYPE};
use crate::providers::codex::CodexProvider;
```

Add field:

```rust
pub struct RuntimeHttpConfig {
    pub node_id: String,
    pub run_log_dir: PathBuf,
    pub claude_bin: PathBuf,
    pub opencode_bin: PathBuf,
    pub codex_bin: PathBuf,
}
```

Update `/providers`:

```rust
    let codex = probe_provider_health(ProviderHealthProbe {
        kind: "codex".to_string(),
        bin_path: state.config.codex_bin,
    });
    let (claude, opencode, codex) = tokio::join!(claude, opencode, codex);
    Json(vec![claude, opencode, codex])
```

Update `spawn_provider_run`:

```rust
            "codex" => {
                let provider = CodexProvider::new(state.config.codex_bin.clone());
                run_provider_stream(state.runs.clone(), run_id.clone(), provider, spec).await
            }
```

Update `validate_run_spec`:

```rust
    if !matches!(spec.provider_kind.as_str(), "claude" | "opencode" | "codex") {
```

Modify `apps/runtime-agent/src/config.rs` `http_config`:

```rust
            codex_bin: self.providers.codex.binary_path.clone(),
```

- [ ] **Step 3: Add failing CLI run test for Codex**

Open `apps/runtime-agent/tests/cli_run_test.rs` and append:

```rust
#[test]
fn cli_run_supports_codex_provider() {
    let temp = tempfile::TempDir::new().expect("tempdir");
    let script = make_script(
        &temp,
        "fake-codex",
        r#"#!/usr/bin/env bash
printf '%s\n' '{"type":"session","session_id":"cli-codex-session"}'
printf '%s\n' '{"type":"message.delta","delta":"hello from cli codex"}'
printf '%s\n' '{"type":"turn.completed","summary":"done"}'
"#,
    );

    let output = Command::new(env!("CARGO_BIN_EXE_runtime-agent"))
        .arg("run")
        .arg("--provider")
        .arg("codex")
        .arg("--provider-bin")
        .arg(script)
        .arg("--workspace")
        .arg(temp.path())
        .arg("--prompt")
        .arg("hello")
        .output()
        .expect("run runtime-agent");

    assert!(
        output.status.success(),
        "stderr: {}",
        String::from_utf8_lossy(&output.stderr)
    );
    let stdout = String::from_utf8_lossy(&output.stdout);
    assert!(stdout.contains(r#""type":"session_started""#));
    assert!(stdout.contains("hello from cli codex"));
}
```

If `make_script` is not already present in `cli_run_test.rs`, copy the existing helper shape from `provider_spawn_test.rs`:

```rust
fn make_script(dir: &tempfile::TempDir, name: &str, body: &str) -> std::path::PathBuf {
    use std::os::unix::fs::PermissionsExt;

    let path = dir.path().join(name);
    std::fs::write(&path, body).expect("write fake provider script");
    let mut permissions = std::fs::metadata(&path).expect("metadata").permissions();
    permissions.set_mode(0o755);
    std::fs::set_permissions(&path, permissions).expect("chmod fake provider script");
    path
}
```

- [ ] **Step 4: Update CLI args and run provider selection**

Modify `apps/runtime-agent/src/main.rs`:

```rust
use superteam_runtime_agent::providers::codex::CodexProvider;
```

Add top-level arg:

```rust
    #[arg(long)]
    codex_bin: Option<PathBuf>,
```

Add override:

```rust
            codex_bin: args.codex_bin,
```

Add enum variant:

```rust
enum ProviderKind {
    Claude,
    Opencode,
    Codex,
}
```

Update default bin:

```rust
            Self::Codex => "codex",
```

Update `run_provider` match:

```rust
        ProviderKind::Codex => {
            let provider = CodexProvider::new(provider_bin);
            stream_provider_events(&provider, request).await
        }
```

- [ ] **Step 5: Run HTTP and CLI tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test http_server_test --test cli_run_test
```

Expected: PASS.

- [ ] **Step 6: Commit Task 6**

```bash
git add apps/runtime-agent/src/server.rs apps/runtime-agent/src/config.rs apps/runtime-agent/src/main.rs apps/runtime-agent/tests/http_server_test.rs apps/runtime-agent/tests/cli_run_test.rs
git commit -m "feat: expose codex in runtime smoke paths"
```

## Task 7: Legacy Task Executor Uses Catalog

**Files:**
- Modify: `apps/runtime-agent/src/executor/task.rs`
- Test: `apps/runtime-agent/tests/provider_spawn_test.rs`

- [ ] **Step 1: Refactor legacy task executor provider selection**

Modify imports in `apps/runtime-agent/src/executor/task.rs`:

```rust
use crate::providers::{ProviderAdapter, ProviderRequest, catalog};
```

Remove direct provider imports:

```rust
use crate::providers::{
    ProviderAdapter, ProviderRequest, claude::ClaudeProvider, opencode::OpenCodeProvider,
};
```

Replace `select_provider`:

```rust
fn select_provider(
    provider_type: &str,
    config: &RuntimeConfig,
) -> Result<Box<dyn ProviderAdapter>> {
    catalog::select_provider(config, provider_type)
        .map_err(|error| anyhow::anyhow!("Unsupported provider type: {provider_type}: {error}"))
}
```

- [ ] **Step 2: Run Rust tests that cover provider selection**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml --test provider_spawn_test --test runtime_command_executor_test
```

Expected: PASS.

- [ ] **Step 3: Commit Task 7**

```bash
git add apps/runtime-agent/src/executor/task.rs
git commit -m "refactor: centralize task provider selection"
```

## Task 8: Full Runtime Agent Verification And Completion Check

**Files:**
- Modify only if preceding verification exposes a compile/test failure in files already touched by this plan.

- [ ] **Step 1: Format Rust code**

Run:

```bash
cargo fmt --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: exits 0 and formats only runtime-agent Rust files touched by this plan.

- [ ] **Step 2: Run full runtime-agent verification**

Run:

```bash
corepack pnpm verify:runtime-agent
```

Expected: exits 0. This runs foundation contract verification and `cargo test --manifest-path apps/runtime-agent/Cargo.toml`.

- [ ] **Step 3: Inspect git diff for unrelated changes**

Run:

```bash
git status --short
git diff --stat
```

Expected: only files from this plan plus any pre-existing unrelated dirty file remain. Do not stage unrelated dirty files such as `docs/superpowers/plans/2026-06-13-digital-employee-directory-management-refactor.md`.

- [ ] **Step 4: Run project completion gate**

Read and follow:

```bash
sed -n '1,260p' .codex/skills/superteam-completion-check/SKILL.md
```

Expected: completion check evidence distinguishes unit/integration/fake CLI verification from any untested real Codex CLI end-to-end path.

- [ ] **Step 5: Final implementation commit**

If Task 8 produced formatting or small verification fixes, commit them:

```bash
git add apps/runtime-agent
git commit -m "test: verify codex runtime provider"
```

If Task 8 produced no file changes, do not create an empty commit.

## Self-Review Notes

- Spec coverage: configuration, adapter, true resume, capability reporting, workspace home, command execution, legacy task executor, HTTP smoke path, CLI run path, tests, and final verification all map to tasks above.
- Placeholder scan: the plan contains no open-ended implementation placeholders; every change step identifies concrete files, functions, code snippets, commands, and expected results.
- Type consistency: the plan uses `provider_type = "codex"`, `provider_kind = "codex"`, `ProviderHomeKind::Codex`, `providers.codex`, and `CodexProvider` consistently.
