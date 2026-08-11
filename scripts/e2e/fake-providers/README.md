# 假 Provider（Provider 语义链路 E2E 用）

真实 provider 无法按需产出特定失败形态（限流、空跑、长跑触发预算熔断），这些脚本用来**确定性地**触发每一类 `ErrorEnvelope.code`。它们不是 mock 测试——Runtime、Control Plane、DB、协调线程全都是真的，只有最末端的 provider 进程被替换。

| 脚本 | 模拟 | 期望分类 |
|---|---|---|
| `claude-rate-limit-exit1.sh` | 打一行 system，stderr 写 rate limit，exit 1 | `RATE_LIMIT` / `transient_provider` / retryable=true |
| `claude-no-terminal-event.sh` | 打一行 system 后 **exit 0 且无 result** | `PROVIDER_NO_TERMINAL_EVENT` / `transient_provider` / **retryable=true** |
| `claude-sleep.sh` | 打一行 system 后长睡 | 配合把 attempt 的 `budget_wall_clock_limit_sec` 改小 → `BUDGET_FUSE` / `budget_fuse` |
| `claude-success-with-artifacts.sh` | 成功 result + 写 `deliverables/`（可选读 `.scratch/e2e/fake-produces.json` / `fake-acceptance.json`） | 任务 completed；工件经 CP presign 上本地对象存储（见 `scripts/ops/smoke-runtime-artifact-rustfs.mjs`） |

## 用法

```bash
# 1. 备份并改 provider 路径（config.yaml 已被 gitignore，是本机文件）
cp apps/runtime-agent/config.yaml /tmp/config.yaml.bak
#    把 providers.claude_code.binary_path 指向本目录下某个脚本（绝对路径）

# 2. 重启（只重启 runtime-agent；先确认全机只有一个 agent 进程）
ps -ax | grep 'runtime-agent --config' | grep -v grep   # 必须只有一行
./scripts/dev-services.sh restart runtime-agent

# 3. 跑用例（见 scripts/e2e/provider-semantic-fail-classification.mjs）

# 4. 收尾：还原 config 并重启
cp /tmp/config.yaml.bak apps/runtime-agent/config.yaml
./scripts/dev-services.sh restart runtime-agent
```

## 踩过的坑（2026-08-10 实测）

- **假 provider 必须应答 `--version`**（三个脚本已内置）：Runtime 每次派发前用 `<bin> --version` 探活，3 秒超时。不应答 → 提供方被判不可用 → 预检闸以 `reason_code=provider_unavailable` 拦下派发 → 任务停在 `waiting_human/attempts=0` 并**每次批准都补建一张新的等待卡**。现场极像"人类批准了却不放行"，实际与审批无关。判别器：`project_events` 里的 `project_task.dispatch_blocked` payload 直接写着 `reason_code`。

- **先数进程**：曾出现一个孤儿 runtime-agent（原会话已退出、进程被 reparent 到 launchd）与受管进程同 node_id 抢单。它加载的是旧配置，于是跑的是**真** claude，任务"成功完成"，整轮结论作废。改 provider 路径前必须确认全机只有一个 agent。
- **预算熔断腿**：dispatch 后直接改库里 attempt 的 `budget_wall_clock_limit_sec=1`，CP 在心跳时按库里的值判超（`RecordProjectTaskAttemptBudgetHeartbeat` 每次重读），默认心跳间隔 15s，所以 provider 睡够 ~20s 即可。
- **预检闸**：规划器有时把任务判为高风险，任务会卡在 `waiting_human/approval_required`、`attempts=0`，此时 E2E 会在"等 running attempt"处超时——先看 `project_decision_requests` 再决定是批准还是换需求重来。
