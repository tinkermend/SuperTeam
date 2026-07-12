# Provider Transcript 工具事件捕获——收尾落地计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把 `docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md` 从"主体已落地（commit `8cd076c4`）、Phase 2 仅单测"推进到 spec 定义的完成态：补齐三处代码缺口，并完成 §6 真实 E2E（items 1–6）。

**Architecture:** spec 的绝大部分已在 main（commit `8cd076c4`，2026-07-10）：parser 返回 `Vec`、claude adapter 全量解析 tool_use/tool_result、`RawLineSink` 分段上传、迁移 053、契约扩展、ledger payload 合并、Web 执行追踪面板。本计划只做**增量收尾**，不重做已落地部分。

**Tech Stack:** Rust (tokio, aws-sdk-s3, sha2, regex)、Go control-plane（本计划不改 Go 代码）、真实火山引擎 TOS bucket `superteam`。

## Global Constraints

- 验证一律走仓库脚本：`corepack pnpm verify:runtime-agent`（= verify:contracts + `cargo test --manifest-path apps/runtime-agent/Cargo.toml`）。单测内循环可用 `cargo test --manifest-path apps/runtime-agent/Cargo.toml <test_name>`。
- 服务启停只用 `scripts/dev-services.sh start|status|restart|stop`；改 runtime-agent 代码后 `restart runtime-agent`。
- 不新增表、不新增端点、不改契约（迁移 053 与 openapi 扩展已落地）。
- raw（本地缓冲与对象存储）**永不脱敏**；脱敏只发生在进 ledger/WS 的 excerpt（spec §3.5/§4.5）。
- `is_error` 只来自 claude-code 进程写出的字节，任何情况下不得从模型文本解析（spec §3.3）。
- E2E 在真实 TOS bucket 上执行：**不得为验证目的删除或覆盖既有对象**；写入前确认 `runs/{tenant_id}/{attempt_id}/` 前缀不与既有数据冲突。
- git 提交信息结尾带 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

## 已落地基线（不要重做）

| spec 要求 | 状态 | 位置 |
|---|---|---|
| parser 签名 `Vec<ProviderEvent>`、三 adapter 改造 | ✅ | `providers/mod.rs:78`、claude/codex/opencode |
| claude tool_use/tool_result 解析、`is_error` 取字节 | ✅ | `providers/claude.rs:85-202` |
| 4KB 对称截断 `truncate_excerpt` | ✅ | `events.rs:11` |
| `RawLineSink` 注入、stderr 逐行 tee + 256KB 尾部上限 | ✅ | `providers/mod.rs:117-189` |
| 分段上传（8MB 封段、重试 3 次、manifest、complete:false 降级） | ✅ | `raw_log.rs` |
| 迁移 053 `project_task_attempts` log_* 五列 | ✅ | `migrations/053_project_task_attempt_raw_log.sql` |
| openapi 扩展 + 回写链（runtime→CP handler→repo） | ✅ | `controlplane/models.rs` / `project/handler.go:1854` |
| `execution_ledger.sql` payload 并入 metadata（COALESCE） | ✅ | `queries/execution_ledger.sql:128` |
| Web 执行追踪面板渲染 tool 节点、失败标红 | ✅ | `project-execution-trace-panel.tsx` |
| 凭证形态正则脱敏（ledger 路径） | ✅ | `redaction.rs` |
| runtime `s3` 配置段（dev config.yaml 已填真实 TOS） | ✅ | `config.rs:16-20`、`apps/runtime-agent/config.yaml:44`（gitignored） |
| Phase 1 真实 E2E（`false` → `is_error:true` 标红） | ✅ | commit `8cd076c4` 提交信息记录 |

## 遗留缺口（本计划范围）

1. **env 值脱敏未接线**：`redaction.rs:49 redact_with_environment` 是死代码，executor 两处 excerpt 只调 `redact()`（spec §4.5 要求"已知敏感 env 变量名的值"参与脱敏）。
2. **本地文件权限 0600 未做**（spec §4.5）：`raw_log.rs:311 append_local` 与 `runs.rs:278 append_event` 均未设 mode。
3. **无时间维度封段**（spec §4.4"每累积 8MB **或 30s** 封一个分段"）：`raw_log.rs Writer::run` 只按大小轮转，安静小流量 run 在节点断电时对象存储侧一无所有。
4. **Phase 2 真实 E2E 未做**（spec §6 items 3/4/6 从未跑过；items 1/2/5 需在当前 main 复核）。
5. **TOS Object Lock / 版本控制支持性未核实**（spec §3.5.3 "落地前须核实"）。
6. spec 状态头仍写"待实现"，与事实不符；CHANGELOG 无条目。

**明确不在范围（防误扩）：** §6 item 7（控制平面读 raw 时重算 sha256）依赖证据地基 spec 的读路径——控制平面目前不存在任何 raw 读端点，该项作为证据地基 spec 的首要验收项顺延，本计划只在 spec 里记下这笔账。codex/opencode 的 tool 事件解析为 spec 非目标。

---

### Task 1: env 值脱敏接线进 ledger excerpt

**Files:**
- Modify: `apps/runtime-agent/src/commands/executor.rs`（`runtime_event_writeback` :1583、`record_event` :1418、调用点 :2825、tests mod :2949）

**Interfaces:**
- Consumes: `crate::redaction::redact_with_environment(value: &str, environment: I) -> String`（已存在，`I: IntoIterator<Item = (&'a String, &'a String)>`，`&BTreeMap<String,String>` 直接满足）；`RunSpec.environment: BTreeMap<String, String>`（`runs.rs:40`，drain 循环的 `spec` 在作用域内）。
- Produces: `fn runtime_event_writeback(record: &RunEventRecord, provider_session_id: Option<&str>, environment: &BTreeMap<String, String>) -> RuntimeCommandEventWriteback`——Task 4 的 E2E 依赖该行为（ledger excerpt 中 env 秘密值被替换为 `[REDACTED:env:{NAME}]`）。

- [ ] **Step 1: 写失败测试**

在 `executor.rs` 的 `mod tests`（:2949 起）内追加（与 `project_task_attestation_writeback_carries_runtime_and_provider_metadata` :3696 同级）：

```rust
#[test]
fn runtime_event_writeback_redacts_environment_values_in_excerpts() {
    let mut environment = std::collections::BTreeMap::new();
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
            input_excerpt: "{\"command\":\"curl -H 'X-Key: supersecretvalue123'\"}".to_string(),
            input_truncated: false,
        },
        recorded_at_ms: 0,
    };
    let writeback = runtime_event_writeback(&record, None, &environment);
    let input = writeback.payload["input_excerpt"].as_str().unwrap();
    assert!(input.contains("[REDACTED:env:MY_API_TOKEN]"));
    assert!(!input.contains("supersecretvalue123"));
}
```

注意：tests mod 若未 `use super::*` 覆盖到 `RunEventRecord`/`ProviderEvent`，按文件内既有 import 风格补齐。

- [ ] **Step 2: 跑测试确认编译失败**

Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_event_writeback_redacts_environment_values_in_excerpts`
Expected: 编译错误——`runtime_event_writeback` 只接受 2 个参数。

- [ ] **Step 3: 实现**

`executor.rs:1583` 改签名（文件顶部若无 `use std::collections::BTreeMap;` 则补上）：

```rust
fn runtime_event_writeback(
    record: &RunEventRecord,
    provider_session_id: Option<&str>,
    environment: &BTreeMap<String, String>,
) -> RuntimeCommandEventWriteback {
```

两处 excerpt（:1620、:1642）改为：

```rust
serde_json::Value::String(crate::redaction::redact_with_environment(
    input_excerpt,
    environment,
)),
```

```rust
serde_json::Value::String(crate::redaction::redact_with_environment(
    output_excerpt,
    environment,
)),
```

`record_event`（:1418）签名加参并透传：

```rust
    async fn record_event(
        &self,
        record: &RunEventRecord,
        provider_session_id: Option<&str>,
        environment: &BTreeMap<String, String>,
    ) -> anyhow::Result<()> {
```

其内部调用（:1434）改为 `&runtime_event_writeback(record, provider_session_id, environment)`。

唯一调用点 `drain_provider_events` 内（:2825）改为：

```rust
                .record_event(&record, latest_provider_session_id.as_deref(), &spec.environment)
```

（`spec: RunSpec` 已在 `drain_provider_events` 参数中，:2789。注意 :2825 处 `spec` 是否已被 move——若 drain 中 `spec` 先于事件循环被部分移动，改为在循环前 `let environment = spec.environment.clone();` 并传 `&environment`。）

- [ ] **Step 4: 跑测试确认通过**

Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_event_writeback`
Expected: PASS（含既有相邻测试）。

- [ ] **Step 5: 提交**

```bash
git add apps/runtime-agent/src/commands/executor.rs
git commit -m "feat(runtime): redact provider env values in ledger excerpts

redact_with_environment existed but was dead code; the two excerpt sites
only ran pattern redaction, so a secret injected via employee environment
reached execution_ledger_events verbatim. Spec §4.5 requires env-value
redaction on the ledger/WS path (raw stays verbatim by design).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: 本地 transcript 文件权限 0600

**Files:**
- Modify: `apps/runtime-agent/src/raw_log.rs`（`append_local` :311、tests mod）
- Modify: `apps/runtime-agent/src/runs.rs`（`append_event` :278，文件尾追加 tests mod）

**Interfaces:**
- Consumes: `tokio::fs::OpenOptions::mode(u32)`（unix only）。
- Produces: `{log_dir}/{run_id}/raw.jsonl` 与 `events.jsonl` 创建时 mode 0600（spec §4.5）。仅创建时生效，既有文件不回改。

- [ ] **Step 1: 写失败测试**

`raw_log.rs` tests mod 追加：

```rust
    #[tokio::test]
    #[cfg(unix)]
    async fn local_raw_log_is_owner_only() {
        use std::os::unix::fs::PermissionsExt;
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader::default());
        let s = sink(uploader, dir.path(), 1 << 20);

        s.write_line(RawStream::Stdout, "line");
        s.finalize().await.unwrap();

        let mode = std::fs::metadata(dir.path().join("raw.jsonl"))
            .unwrap()
            .permissions()
            .mode();
        assert_eq!(mode & 0o777, 0o600);
    }
```

`runs.rs` 文件尾追加：

```rust
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
}
```

（若 `RunSpec` 字段与 main 有出入，以 `runs.rs:40` 实际定义为准逐字段补全；`tempfile` 已是 dev-dependency——`raw_log.rs` 测试在用。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml owner_only`
Expected: 两个测试 FAIL，实际 mode 是 0644（受 umask 影响的默认值）。

- [ ] **Step 3: 实现**

`raw_log.rs append_local`：

```rust
async fn append_local(path: &Path, bytes: &[u8]) -> Result<()> {
    let mut options = OpenOptions::new();
    options.create(true).append(true);
    // Raw transcripts hold unredacted provider output; keep them owner-only.
    #[cfg(unix)]
    options.mode(0o600);
    let mut file = options
        .open(path)
        .await
        .with_context(|| format!("failed to open raw log {path:?}"))?;
    file.write_all(bytes).await?;
    file.flush().await?;
    Ok(())
}
```

`runs.rs append_event` 同型改造：

```rust
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
```

- [ ] **Step 4: 跑测试确认通过**

Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml owner_only`
Expected: 2 passed。

- [ ] **Step 5: 提交**

```bash
git add apps/runtime-agent/src/raw_log.rs apps/runtime-agent/src/runs.rs
git commit -m "fix(runtime): create local transcript files with mode 0600

raw.jsonl holds unredacted provider output and events.jsonl the parsed
stream; spec §4.5 requires owner-only permissions on {log_dir} files.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: 按时间封段（30s），补齐 spec §4.4 的"或 M 秒"

**Files:**
- Modify: `apps/runtime-agent/src/raw_log.rs`（`Writer` 结构与 `run` 循环、构造函数、tests）

**Interfaces:**
- Consumes: Task 2 后的 `raw_log.rs`。
- Produces: `SegmentedRawLogSink::with_options(uploader, local_dir, key_prefix, attempt_id, segment_bytes, segment_interval: std::time::Duration)`；`new` 与 `with_segment_bytes` 语义不变（默认 30s）。调用方（executor.rs `build_raw_sink_inner` 用 `new`）无需改动。

**理由：** 分段上传的存在意义是"进程被 kill、节点断电时证据已在对象存储"（spec §4.4）。当前只按 8MB 封段，一个安静的小流量 run 在崩溃场景下对象存储侧为空，恰好丢掉最需要证据的场景。

- [ ] **Step 1: 写失败测试**

`raw_log.rs` tests mod 追加：

```rust
    #[tokio::test(start_paused = true)]
    async fn rotates_segments_by_time() {
        let dir = tempfile::tempdir().unwrap();
        let uploader = Arc::new(RecordingUploader::default());
        let s = SegmentedRawLogSink::with_options(
            uploader.clone(),
            dir.path().to_path_buf(),
            "runs/t1/a1/".to_string(),
            "a1".to_string(),
            1 << 20,                                  // 大小阈值取不到
            std::time::Duration::from_secs(30),
        );

        s.write_line(RawStream::Stdout, "tiny");
        // 让 writer task 消费掉这一行
        tokio::task::yield_now().await;
        tokio::time::advance(std::time::Duration::from_secs(31)).await;
        tokio::task::yield_now().await;

        {
            let objects = uploader.objects.lock().unwrap();
            assert!(
                objects.iter().any(|(k, _)| k.contains("raw.part-0001")),
                "a small quiet run must still reach object storage within the interval"
            );
        }
        s.finalize().await.unwrap();
    }
```

（`start_paused` 下 `tokio::time::advance` 推进虚拟时钟；上传是内存 RecordingUploader，无真实 IO 等待。若 yield 不足以驱动 writer task，改用 `tokio::time::sleep(Duration::from_millis(1)).await`——paused 模式下 sleep 也走虚拟时钟并让出调度。）

- [ ] **Step 2: 跑测试确认失败**

Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml rotates_segments_by_time`
Expected: 编译错误——`with_options` 不存在。

- [ ] **Step 3: 实现**

```rust
/// Rotate a non-empty segment at least this often, so a killed process or a
/// powered-off node loses at most one interval of transcript (spec §4.4).
const DEFAULT_SEGMENT_INTERVAL: std::time::Duration = std::time::Duration::from_secs(30);
```

`Writer` 加字段 `segment_interval: std::time::Duration`；构造函数改为：

```rust
    pub fn new(
        uploader: Arc<dyn RawLogUploader>,
        local_dir: PathBuf,
        key_prefix: String,
        attempt_id: String,
    ) -> Self {
        Self::with_options(
            uploader,
            local_dir,
            key_prefix,
            attempt_id,
            DEFAULT_SEGMENT_BYTES,
            DEFAULT_SEGMENT_INTERVAL,
        )
    }

    pub fn with_segment_bytes(
        uploader: Arc<dyn RawLogUploader>,
        local_dir: PathBuf,
        key_prefix: String,
        attempt_id: String,
        segment_bytes: usize,
    ) -> Self {
        Self::with_options(
            uploader,
            local_dir,
            key_prefix,
            attempt_id,
            segment_bytes,
            DEFAULT_SEGMENT_INTERVAL,
        )
    }

    pub fn with_options(
        uploader: Arc<dyn RawLogUploader>,
        local_dir: PathBuf,
        key_prefix: String,
        attempt_id: String,
        segment_bytes: usize,
        segment_interval: std::time::Duration,
    ) -> Self {
        let (tx, rx) = mpsc::unbounded_channel();
        let writer = Writer {
            uploader,
            local_dir,
            key_prefix,
            attempt_id,
            segment_bytes,
            segment_interval,
        };
        let task = tokio::spawn(writer.run(rx));
        Self {
            tx,
            task: Mutex::new(Some(task)),
        }
    }
```

`Writer::run` 的接收循环改为 select（其余不变——从 `while let Some(msg) = rx.recv().await` 到 `Msg::Finish => break` 的整块替换）：

```rust
        let mut ticker = tokio::time::interval(self.segment_interval);
        ticker.set_missed_tick_behavior(tokio::time::MissedTickBehavior::Delay);
        ticker.tick().await; // 首个 tick 立即到期，先消费掉

        loop {
            tokio::select! {
                msg = rx.recv() => match msg {
                    Some(Msg::Line { stream, line }) => {
                        let encoded = encode_line(stream, &line);
                        // Local write failure means evidence cannot be recorded at
                        // all; the run must not continue pretending otherwise.
                        append_local(&local_path, &encoded).await?;
                        total.update(&encoded);
                        total_bytes += encoded.len() as i64;
                        segment.extend_from_slice(&encoded);
                        if segment.len() >= self.segment_bytes {
                            match self.upload_segment(parts.len() + 1, &segment).await {
                                Ok(part) => parts.push(part),
                                Err(_) => complete = false,
                            }
                            segment.clear();
                        }
                    }
                    Some(Msg::Finish) | None => break,
                },
                _ = ticker.tick() => {
                    if !segment.is_empty() {
                        match self.upload_segment(parts.len() + 1, &segment).await {
                            Ok(part) => parts.push(part),
                            Err(_) => complete = false,
                        }
                        segment.clear();
                    }
                }
            }
        }
```

- [ ] **Step 4: 全量跑 raw_log 测试**

Run: `cargo test --manifest-path apps/runtime-agent/Cargo.toml raw_log`
Expected: 全部 PASS（既有 6 个 + 新增 2 个）。尤其确认 `failed_segment_upload_marks_manifest_incomplete_without_failing_the_run` 未被 select 改动破坏。

- [ ] **Step 5: 门禁 + 提交**

Run: `corepack pnpm verify:runtime-agent`
Expected: contracts 校验 + cargo test 全绿。

```bash
git add apps/runtime-agent/src/raw_log.rs
git commit -m "feat(runtime): rotate raw transcript segments by time as well as size

Size-only rotation meant a quiet run under 8MB uploaded nothing until
finalize — exactly the crash/power-loss window segments exist for. A
non-empty segment now uploads at least every 30s (spec §4.4).

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: 真实 E2E（spec §6 items 1–6）

**Files:**
- 无代码改动。产出验证证据，写入 `CHANGELOG.md` 与 spec 附录（Task 6）。

**Interfaces:**
- Consumes: Tasks 1–3 的行为；运行中的全套 dev 服务；`apps/runtime-agent/config.yaml` 的 s3 段（真实 TOS，已配置）；`apps/control-plane/config/config.yaml` 的 database DSN（psql 用）。
- Produces: spec §6 items 1–6 的逐条证据（SQL 输出、TOS 对象清单、sha256 比对、浏览器截图）。

**阻塞规则（CLAUDE.md）：** 任一环节无法真实验证（runtime 节点连不上、planner 超时、审批流走不通）→ 按系统化排障处理；排不掉则本任务标记**阻塞**并报告缺失依赖，不得以未验证状态声明完成，也不得用 mock/单测替代。

- [ ] **Step 1: 服务就位**

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart runtime-agent   # 加载 Tasks 1-3 的代码
scripts/dev-services.sh restart control-plane   # 自动跑 Atlas 迁移，确认 053 已应用
```

确认 runtime 节点真实在线（不是仅进程存活）：

```bash
tail -50 .scratch/dev-services/logs/control-plane.log | grep -E "runtime/(heartbeat|ws|tasks/claim)"
```
Expected: heartbeat 持续 200。runtime-agent 日志近期曾有 `Connection refused` 波动——若 401/connection refused 持续，先排障（见 memory `runtime-agent-401-not-connected` 的 triage：restart runtime-agent 后复查），排不掉即阻塞上报。

确认迁移 053 生效（DSN 取自 `apps/control-plane/config/config.yaml`）：

```sql
select column_name from information_schema.columns
 where table_name='project_task_attempts' and column_name like 'log_%';
```
Expected: `log_store` / `log_ref` / `log_bytes` / `log_sha256` / `log_compressed` 五行。

- [ ] **Step 2: 派发真实任务**

用 admin/admin 登录 CP（:8081），在测试项目上提交一条需求，走完整协调链（demand → planner → plan-revision 人审通过 → dispatch → claude-code 真实执行）。需求描述要求数字员工在工作目录**真实执行**：

1. `git status`（成功命令）
2. `false`（必定失败命令）
3. `echo sk-test20260713abcdefghijklmnopqrstuv`（假 token，验证脱敏分叉）

并复述每步输出。记录得到的 `demand_id`、`project_task` id、`attempt_id`、`tenant_id`。

（planner 走 deepseek 真实调用约 47s；plan-revision review 是人审门，用 admin 会话通过 API approve——该链路 2026-06-25 验证过可用。）

- [ ] **Step 3: 验证 ledger 工具事件（§6 items 1–2）**

```sql
-- item 1：事件类型分布，必须出现 tool_started / tool_completed
select input_summary, count(*)
  from execution_ledger_events
 where event_type='provider.event'
   and project_task_attempt_id='<attempt_id>'
 group by 1;

-- item 2：is_error 来自字节而非模型自述
select input_summary,
       metadata->>'name'      as tool_name,
       metadata->>'is_error'  as is_error,
       left(metadata->>'output_excerpt', 120) as output_excerpt
  from execution_ledger_events
 where event_type='provider.event'
   and project_task_attempt_id='<attempt_id>'
   and input_summary in ('tool_started','tool_completed')
 order by occurred_at;
```
Expected: `false` 对应的 `tool_completed` 行 `is_error='true'`；`git status` 对应行 `is_error='false'`。（列名以实际 schema 为准；若事件子类型不在 `input_summary` 而在别列，用 `\d execution_ledger_events` 先确认再查。）

- [ ] **Step 4: 验证 TOS raw 分段与 manifest（§6 item 3）**

凭证取自 `apps/runtime-agent/config.yaml` s3 段（勿写进任何提交物）：

```bash
export AWS_ACCESS_KEY_ID=<config.yaml s3.access_key_id>
export AWS_SECRET_ACCESS_KEY=<config.yaml s3.secret_access_key>
aws s3 ls "s3://superteam/runs/<tenant_id>/<attempt_id>/" \
  --endpoint-url https://tos-s3-cn-guangzhou.volces.com --region cn-guangzhou
```
Expected: `raw.part-0001.jsonl`（或多段）+ `manifest.json`。

```bash
cd "$SCRATCHPAD" && mkdir -p raw && cd raw
aws s3 cp "s3://superteam/runs/<tenant_id>/<attempt_id>/" . --recursive \
  --endpoint-url https://tos-s3-cn-guangzhou.volces.com --region cn-guangzhou
cat raw.part-*.jsonl | shasum -a 256        # 与 manifest.total_sha256 一致
python3 -c "import json,sys; [json.loads(l) for l in open('raw.part-0001.jsonl')]"  # 逐行可解析
grep -c '"type":"user"' raw.part-*.jsonl    # 含 tool_result 行
cat manifest.json                            # complete: true
```

- [ ] **Step 5: 验证 attempt 行回写（§6 item 4）**

```sql
select log_store, log_ref, log_bytes, log_sha256, log_compressed
  from project_task_attempts where id='<attempt_id>';
```
Expected: `log_store='object_store'`，`log_ref='runs/<tenant_id>/<attempt_id>/manifest.json'`，`log_sha256`/`log_bytes` 与 manifest 及 Step 4 实测一致。

- [ ] **Step 6: 验证脱敏分叉（§6 item 6）**

```sql
select metadata->>'output_excerpt'
  from execution_ledger_events
 where project_task_attempt_id='<attempt_id>'
   and metadata->>'output_excerpt' like '%REDACTED%';
```
Expected: echo 命令的 excerpt 含 `[REDACTED:anthropic_key]`，不含 `sk-test2026...` 原文。

```bash
grep -c "sk-test20260713" "$SCRATCHPAD"/raw/raw.part-*.jsonl
```
Expected: ≥1——**raw 保持原样**（同一字节两条路径分叉，这条是 §3.5 决策的验收）。

- [ ] **Step 7: 浏览器验证（§6 item 5）**

打开 `http://localhost:3000` 项目详情 → 执行追踪面板：工具调用节点（名字+input 摘要）与工具结果节点可见，`false` 的结果节点标红，`text_delta` 旁白视觉弱化。截图存证到 scratchpad。（前端本计划零改动，只验证；若需动前端，先读 `DESIGN.md`。）

- [ ] **Step 8: 记录证据**

把 items 1–6 的实测输出（SQL 结果、对象清单、sha256、截图路径）整理成一段验证记录，供 Task 6 写入 CHANGELOG 与 spec 附录。**不在证据中留任何真实凭证。**

---

### Task 5: 核实 TOS Object Lock / 版本控制支持性（spec §3.5.3 / §7）

**Files:**
- 产出结论，写入 spec（Task 6 一并提交）。**本任务不改 bucket 配置**——bucket 由多方共用（skills 也在），开启 versioning/Object Lock 是运维决策，查明支持性后交人类拍板。

- [ ] **Step 1: 实测 bucket 当前状态**

```bash
aws s3api get-bucket-versioning --bucket superteam \
  --endpoint-url https://tos-s3-cn-guangzhou.volces.com --region cn-guangzhou
aws s3api get-object-lock-configuration --bucket superteam \
  --endpoint-url https://tos-s3-cn-guangzhou.volces.com --region cn-guangzhou
```
记录原样输出（`NotImplemented`/`ObjectLockConfigurationNotFoundError`/正常返回，各自含义不同）。

- [ ] **Step 2: 查火山引擎 TOS 官方文档**

WebSearch「火山引擎 TOS Object Lock WORM 保留策略」「TOS bucket versioning S3 兼容 API」，确认：① 是否支持版本控制；② 是否支持 Object Lock/WORM 及是否要求建桶时开启；③ S3 兼容层是否暴露这两个 API。

- [ ] **Step 3: 写结论**

三选一记入 spec §3.5.3：支持（附开启路径与建议）/ 不支持（sha256 事后比对成为唯一完整性防线，§6 item 7 优先级进一步上升）/ 无法确认（列出缺失信息）。

---

### Task 6: 文档收尾

**Files:**
- Modify: `docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md`（状态头 + 落地记录附录）
- Modify: `CHANGELOG.md`（Unreleased 段）

- [ ] **Step 1: 更新 spec 状态头**

```markdown
- 状态：已落地（commit 8cd076c4 主体 + <本计划收尾 commits>；§6 items 1–6 已于 2026-07-13 真实 E2E 验证）
- 遗留：§6 item 7（控制平面读 raw 重算 sha256）依赖证据地基 spec 的读路径，作为该 spec 首要验收项；Object Lock 结论见 §3.5.3
```

并在文末追加「落地记录（2026-07-13）」附录：Task 4 的逐条证据摘要 + Task 5 的 TOS 结论。

- [ ] **Step 2: CHANGELOG**

在 `CHANGELOG.md` Unreleased 段记录：env 值脱敏接线、本地日志 0600、30s 时间封段、Phase 2 真实 E2E 通过（引用 attempt_id 与验证日期）。

- [ ] **Step 3: 提交**

```bash
git add docs/superpowers/specs/2026-07-09-provider-transcript-tool-event-capture-design.md CHANGELOG.md
git commit -m "docs: record provider transcript capture E2E evidence and close spec

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

- [ ] **Step 4: 收尾检查**

跑项目内 skill `$superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）后再声明完成。

## Self-Review

- **Spec 覆盖：** §4.1–§4.4/§4.6/§4.7 已由 `8cd076c4` 落地（见基线表）；本计划补 §4.5 的两处缺口（env 脱敏、0600）、§4.4 的时间封段、§6 E2E items 1–6、§3.5.3/§7 的 TOS 核实。§6 item 7 显式记为证据地基 spec 的首要验收项（控制平面无 raw 读路径，物理上无法在本期验收）。
- **占位符扫描：** 无 TBD/TODO;Task 4 SQL 的列名给出了"以实际 schema 为准"的核对指令而非猜测断言，属验证步骤的必要弹性。
- **类型一致性：** `runtime_event_writeback` 三参签名在 Task 1 测试与实现一致；`with_options` 六参签名在 Task 3 测试与实现一致；`RawLogSummary`/`RunEventRecord`/`RunSpec` 字段均对照 main 现状抄录。
