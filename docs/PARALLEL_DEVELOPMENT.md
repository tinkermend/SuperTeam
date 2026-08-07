# 多会话并行开发：worktree 隔离 + 单栈验证

> 状态：**已实测**（2026-08-08）。本文每条流程与每个坑都在本机真实跑通/踩到，非推演。
> 适用：多个会话（人或 agent）同时在 SuperTeam 上开发，且本机只跑一套开发服务。

## 1. 为什么需要这个

以前的做法是**所有会话挤在 `main` 的同一个 checkout 上**。原因不是没想过 worktree，而是 worktree 里的代码**跑不起来**——服务只有一套，从 worktree 执行 `dev-services.sh` 会失败或静默无效，导致"1 对 1 验证"做不了。

代价是 `CLAUDE.md` 里那一大段防御性 git 铁律（禁 `git add -A`、禁切分支、逐 hunk 暂存、ref 手术），以及真实发生过的事故：

- 会话 A 的 `make generate-sqlc` 把会话 B 未提交的 `inbox.sql` 新查询卷进 `querier.go`，差一步随 A 的提交带走（2026-08-07，提交前逐 hunk 核对时拦下）
- `git branch -D` 孤立并发提交（已恢复）
- 提交落到并发会话的分支上，靠 worktree + cherry-pick + `update-ref` 复位
- 并行会话 E2E 清库，冲掉另一会话的项目夹具

本文给出**验证过的**替代方案：每会话一个 worktree，服务仍只跑一套，谁验证谁把服务切到自己的代码上。

## 2. 一次性设置

### 2.1 所有会话共用的环境变量（写进 shell profile）

```bash
export SUPERTEAM_DEV_PID_DIR=$HOME/.superteam/dev-services/pids
export SUPERTEAM_DEV_LOG_DIR=$HOME/.superteam/dev-services/logs
```

**这是整套方案的关键。** `dev-services.sh` 默认把 pid 写在 `$PROJECT_ROOT/.scratch/dev-services/pids`，**每个 checkout 各记各的**，于是 worktree 里的脚本看不见主 checkout 起的服务（见 §4 坑 1 的严重后果）。挪到共享目录后，任何 checkout 的脚本都能看见并接管同一套服务。

### 2.2 新建 worktree

```bash
git worktree add ../SuperTeam-<主题> -b <分支名>
cd ../SuperTeam-<主题>
corepack pnpm install          # 实测约 3 秒（pnpm 硬链接复用全局 store）
```

### 2.3 配置文件

`apps/control-plane/config/config.yaml` 与 `apps/runtime-agent/config.yaml` 都被 `.gitignore`（`.gitignore:24` 的 `**/config.yaml`），新 worktree 里**只有 `config.example.yaml`**。两种做法任选：

**推荐（无需在 worktree 里放文件）**——指回主 checkout：

```bash
export SUPERTEAM_DEV_CONTROL_PLANE_CONFIG=/Users/tinker/src/singe/SuperTeam/apps/control-plane/config/config.yaml
export SUPERTEAM_DEV_RUNTIME_AGENT_CONFIG=/Users/tinker/src/singe/SuperTeam/apps/runtime-agent/config.yaml
```

这两个变量同时被 `scripts/dev-services.sh` 和 `package.json` 的 `dev:control-plane` / `dev:runtime-agent` 读取，所以"脚本解析 DB URL 的来源"与"进程真正加载的配置"始终一致，不会分叉。

> 注：`package.json` 支持这两个变量是 2026-08-08 的修复。在此之前 npm script 把路径写死成相对路径，设了变量也没用——见 §4 坑 3。

**备选**——在 worktree 里软链：

```bash
ln -sf /Users/tinker/src/singe/SuperTeam/apps/control-plane/config/config.yaml \
       apps/control-plane/config/config.yaml
```

## 3. 日常流程

```bash
# 在自己的 worktree 里写代码、跑单测，随便做，互不干扰

# 轮到自己做真实 E2E 时（同一时刻只应有一个会话在做）：
bash scripts/dev-services.sh restart control-plane web

# 此时 :8080 / :3100 跑的就是你这个 worktree 的代码
# 验证完成后，别的会话再 restart 到他们的 worktree
```

**验证服务当前跑的是哪份代码**（怀疑时必查）：

```bash
LP=$(lsof -nP -iTCP:8080 -sTCP:LISTEN -Fp | grep ^p | cut -c2- | head -1)
lsof -a -p $LP -d cwd -Fn | grep ^n | cut -c2-
```

输出的目录就是当前生效的 checkout。

## 4. 实测踩到的坑（每条都真实发生过）

### 坑 1：不设共享 pid 目录时，`restart` 是**静默空操作**

从 worktree 执行 `restart control-plane`，**退出码 0**，只打两行 WARN：

```
[WARN] control-plane is available at http://127.0.0.1:8080/health but was not started by this script; leaving it running
[WARN] control-plane already responds at http://127.0.0.1:8080/health but is not managed by this script; skipping start
```

服务纹丝不动，仍跑别人的代码（实测：`restart` 后监听进程 cwd 仍是主 checkout）。

**这比撞端口危险得多**——不报错、退出码正常，会话很容易以为自己验证了新代码，实际验的是旧的，**产出假的验证结论**。设了 §2.1 的共享 pid 目录即可根除。

### 坑 2：`config.yaml` 被 gitignore

新 worktree 缺配置 → `dev-services.sh` 报「无法解析数据库 URL」→ 且**它已经先把服务停了**，于是服务处于宕机状态。按 §2.3 处理。

### 坑 3：npm script 曾把配置路径写死（**已于 2026-08-08 修复**）

原 `package.json`：

```json
"dev:control-plane": "go run ./apps/control-plane/cmd/control-plane --config apps/control-plane/config/config.yaml"
```

相对路径，从 worktree 启动时读的是 worktree 里那个**不存在**的文件。`SUPERTEAM_DEV_CONTROL_PLANE_CONFIG` 只影响 shell 脚本解析 DB URL，**管不到二进制**，所以设了照样起不来（错误：`open apps/control-plane/config/config.yaml: no such file or directory`）。

已改为：

```json
"dev:control-plane": "go run ./apps/control-plane/cmd/control-plane --config ${SUPERTEAM_DEV_CONTROL_PLANE_CONFIG:-apps/control-plane/config/config.yaml}"
```

`dev:runtime-agent` 同样处理（`SUPERTEAM_DEV_RUNTIME_AGENT_CONFIG`）。默认行为不变；`pnpm dev:control-plane -- --config x.yaml` 的透传覆盖写法也仍然有效（Go `flag` 包重复 flag 后者胜出，已实测）。

### 坑 4：`restart` 会把**你分支的迁移**应用到共享库

`start|restart control-plane` 自动执行 Atlas 迁移，迁移目录是 `$CONTROL_PLANE_DIR/internal/storage/migrations`，即**你这个 worktree 的**（日志里能看到 `dir: internal/storage/migrations`）。

后果：你分支上的新迁移一旦被应用，库里就有了 `main` 不认识的结构，**且不会自动回滚**，别人切回 main 的代码可能直接坏掉。

**规矩**：只改代码不动 schema 的分支随便切；**一旦动 `apps/control-plane/internal/storage/migrations/`，restart 前必须跟其他会话打招呼**。`SUPERTEAM_DEV_SKIP_MIGRATIONS=1` 只能不应用，救不了已经应用过的。

## 5. 仍然共享、仍需约束的东西

worktree 解决的是**文件与 git**，解决不了运行时。以下始终只有一份：

| 资源 | 约束 |
|---|---|
| 开发数据库 | 造数/清库会影响所有会话。清库前必须问 |
| Control Plane `:8080` | 谁 restart 谁生效，串行使用 |
| Web `:3100` | 同上 |
| Temporal + runtime-agent | 项目协调线程 WorkflowID 为 `project-coordinator:{project_id}`，共库即共 workflow |

所以**「同一时刻只有一个会话做真实 E2E」这条约定不能省**，它是这套方案的前提而非可选项。

### 5.1 怎么保证只有一个会话在验证

纯靠自觉不够。真正伤人的不是"两人同时 restart"（罕见且会立刻暴露），而是：

> B 正在做 E2E，A 因正当理由 restart → **B 的结论在无声中失效，B 却照常报告"验证通过"**。

锁只能拦 A，但 A 有时确实需要重启。所以这里要的是**检测**而非互斥，三层递进：

**第一层 · 归属可见（已实现）**——`status` 显示服务实际跑的是哪个 checkout：

```
control-plane: running pid=97946 healthy=... owner=/path/to/other-worktree log=...
```

`owner=` 只在与当前 checkout 不一致时出现，同源时省略。它**从进程 cwd 观测得来，不是记账**，所以不存在与实际进程脱节的可能。

**第二层 · 接管告警（已实现）**——`stop` / `restart` 顶替他人服务前告警：

```
[WARN] control-plane 当前跑的是 /path/to/other-worktree 的代码，可能有会话正在用它做 E2E 验证
[WARN]   继续将接管该服务，对方验证结论会失效；确认无人占用再操作
```

不阻断（A 可能确实需要接管），只保证不会手滑。

**第三层 · 验证期自检（真正的保证，写入 `AGENTS.md` 硬规则）**——E2E 开始记 pid，收尾复核：

```bash
PID_BEFORE=$(cat $SUPERTEAM_DEV_PID_DIR/control-plane.pid)
# ... 执行 E2E ...
[ "$(cat $SUPERTEAM_DEV_PID_DIR/control-plane.pid)" = "$PID_BEFORE" ] \
  || echo "❌ 验证期间服务被接管，本轮结论作废"
```

**pid 变了 = 有人中途接管 = 结论不算数。** 这条不依赖任何人守规矩，是事后可判定的硬证据；即使前两层都被绕过，假结论也无法合规交付。

> 未采用硬锁（flock）：会话意外中断会留下死锁，需人工清理，代价大于收益。

## 6. 迁移到这套做法之后

`CLAUDE.md` 里为「共享同一 checkout」而立的防御性 git 规则，在各会话真正分处不同 worktree 后大部分不再需要：禁 `git add -A`、禁切分支、逐 hunk 暂存、ref 手术。

但**在完成迁移前不要提前放松**——只要还有会话在主 checkout 上直接改代码，那些规则依然是必需的。

另：生成物（sqlc / oapi-codegen 产物）是全仓重生成，**即便分处不同 worktree，若两人同时改契约仍会互相覆盖**。这条与 worktree 无关，`git add` 后逐 hunk 核对的纪律要长期保留。
