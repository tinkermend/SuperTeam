# SuperTeam casting E2E

不进 `verify:*`。需要真实 Control Plane + DB；`sod` 还需要真实 planner。

## 前置

```bash
./scripts/dev-services.sh status
# Web http://127.0.0.1:3100  CP http://127.0.0.1:8080
# 登录 admin / admin
```

改了 Control Plane 代码要先 `./scripts/dev-services.sh restart control-plane`，
否则跑的是旧进程。

## 入口

```bash
# 默认全 stage（不含 sod）
node scripts/e2e/casting-suite.mjs

# 选 stage
node scripts/e2e/casting-suite.mjs --stage=cascade,graph-assert

# 含 opt-in 的慢 stage
node scripts/e2e/casting-suite.mjs --stage=everything
node scripts/e2e/casting-suite.mjs --stage=sod

# C10 反向：断言库必须抓到「blocker 已解仍 blocked」
SUPERTEAM_ASSERT_GRAPH_REVERSE=1 node scripts/e2e/casting-suite.mjs --stage=graph-assert
```

### Provider 语义失败分类（opt-in，不进 verify:*）

假 binary rate-limit 路径核对 `error_code` / `failure_family` / envelope `provider_type`
（claude-code + 可选 opencode）。Runtime 需挂假 provider binary。

```bash
export SUPERTEAM_E2E_DB_URL='postgres://…'   # 与 control-plane config 一致
node scripts/e2e/provider-semantic-fail-classification.mjs
SUPERTEAM_E2E_SKIP_OPENCODE=1 node scripts/e2e/provider-semantic-fail-classification.mjs

# 终态路径（turn_error 标记 / attestation / 分类与路由），一次一条腿
node scripts/e2e/provider-semantic-terminal-paths.mjs PROVIDER_NO_TERMINAL_EVENT transient_provider true
node scripts/e2e/provider-semantic-terminal-paths.mjs BUDGET_FUSE budget_fuse false --budget
```

上面两组都需要把 `providers.claude_code.binary_path` 指到 `scripts/e2e/fake-providers/` 下对应脚本再重启 runtime-agent；**改之前先确认全机只有一个 agent 进程**（曾有孤儿 agent 抢单导致整轮结论作废）。用法与踩坑见 `scripts/e2e/fake-providers/README.md`。

## Stages

| stage | 覆盖 | 默认跑 |
|---|---|---|
| `smoke` | 登录 + 按名解析项目/员工 fixture | ✅ |
| `role-impact` | C1 预检端点；C2 未确认移除 → 400 带影响面且**零副作用** | ✅ |
| `cascade` | C3 确认移除 → 编制删除 + 可达收口出现缺口 + 负责人收到「编制失效」；C8 重新编制 → 告警自动关闭。**自恢复** | ✅ |
| `graph-assert` | C10 图终态：扫全租户 demand 的 `task-graph`，无「blocker 已全解仍 blocked」；见零边则判失败（拒绝真空通过） | ✅ |
| `automation-fire` | G7 编制完整时规则可保存；G8 运行期编制失效 → fire failed + 负责人 `automation_alert`（文案含缺哪个角色）；删规则关掉其告警。**自恢复** | ✅ |
| `sod` | G12 把同一人编制到 role_independence 两侧 → **耐久** `planning_gap` 点名「职责分离」+ demand 终态。需真实 planner，数分钟 | opt-in |

`sod` 会幂等创建探针剧本 `e2e_sod_probe`：现网已无可用 SoD 夹具——`incident_response`
的 SoD 对是 `operator+verifier` 而全库无人持有 `operator`（批二 G2 判别条件，**不得补发**），
`software_delivery` 的 developer+reviewer 已迁到 `adversarial_review` 不再走经典
`role_independence`。探针用 `diagnostician+verifier`（两侧都有持有人，且有人同时持有两者）。

## 浏览器 GATE 脚本

| 脚本 | 覆盖 |
|---|---|
| `capability-binding-console.mjs` | 能力绑定控制台 U1–U9（spec `2026-08-06-capability-binding-console-design.md` §8）：tab/两区/说明条、**草稿态不即时写入**、依赖闭包预览（含**已保存**的绑定行）、员工侧场地标记**逐条对照 API**、技能详情页文案与项目绑定卡、控制台无真实错误 |

```bash
node scripts/e2e/capability-binding-console.mjs
node scripts/e2e/capability-binding-console.mjs --project=<uuid>
```

改了 Web 代码要先 `restart web`，改了 CP 要先 `restart control-plane`。

## 共享库 `scripts/e2e/lib/`

- `cp-client.mjs` — login / fetch / assert
- `fixtures.mjs` — 按名字解析项目与员工（可用 env 覆盖 UUID）
- `browser.mjs` — **浏览器 E2E 统一入口**：`launchLoggedIn({ chromium })` 返回已登录的
  page + 控制台错误采集。封了两条反复踩的坑：
  1. 登录必须用 placeholder 选择器（表单没有 name/id，`input[type=text]` 会命中错元素），
     且 `waitForURL` 之后要再等一下，否则紧接着的硬导航会被弹回 `/sign-in`；
  2. `realErrors()` 会滤掉**已知良性噪音**，见下。别再用裸 `consoleErrors.length === 0`。
- `assert-graph.mjs` — 图终态断言（C10 承重）。**必须吃 `/task-graph` 的 nodes+edges**：
  `/projects/{id}/tasks` 的 `ProjectTask` 没有任何依赖字段，用它做断言时
  「blocker 已全解仍 blocked」永远无法求值，合法 blocked 反而被误报——
  批三就是靠零依赖断言"绿着"漏过去的。

## 语义扩编脚本（保留）

- `semantic-casting-expansion.mjs`（H1/H4/H7/H9a/H9b）
- `semantic-casting-gates-closeout.mjs`（H9/H7/H8/H9c）
- `semantic-casting-h1-h4-full.mjs`
- `semantic-casting-h12-graph-terminal.mjs`
- `cleanup-p1-inbox-demands.mjs`

## 本批 suite 覆盖边界（诚实说明）

历史 12 个 `browser-casting-*` 与 `run-casting-design-full-suite.mjs` 已删（见 git 历史）。
其中 4 个自带 `@deprecated` 标记、1 个是编排器，删除无损；其余场景的接管情况：

| 原覆盖 | 现在由谁接 |
|---|---|
| G5 编制自动入池同事务 | Go 单测 `project/casting_readiness_test.go` |
| G6 成员移除保护 | Go 单测 `project/casting_closure_test.go` |
| G7 规则保存期编制闸 | Go 单测 `automation/casting_gate_test.go` + 本 suite `automation-fire` |
| G8 fire 期编制失效 | 本 suite `automation-fire`（真实链路） |
| G10 已完成 `planned_task_key` 复用 | `semantic-casting-expansion.mjs` / `h1-h4-full` / `h12` |
| G11 越界 → `pending_review` 不自动跑 | Temporal workflow 测试（`workflow_test.go`，断言 `ForcePendingReview` + `RequestPlanRevisionReview` + 不 decompose） |
| G12 SoD 判定逻辑 | Go 单测 `projectcoordination/activities_test.go` |
| G12 SoD **耐久产品面** | 本 suite `sod`（真实链路） |
| **G9 协调线程自动提请（真路径，非手工 POST）** | **暂无自动化覆盖**——见 TODO |
| 扩编卡浏览器审批 UI | `semantic-casting-gates-closeout.mjs`（H9/H9a 浏览器） |

`assert-graph` 的 C10 反向模式用**真实边结构**（`dependent_task_id`/`blocker_task_id`）
在内存里合成滞留任务，不往真库插脏数据。

## 造数与清理

- 默认项目名：`批二基线项目 P1`（`SUPERTEAM_PROJECT_ID` / `SUPERTEAM_PROJECT_NAME` 可覆盖）
- 员工按显示名 `开发-A` 等解析；**不要给无人持有的 `operator` 补绑定**（批二 G2 判别条件）
- `cascade` / `automation-fire` / `sod` 都在 `finally` 里恢复角色、编制并收掉自己产生的
  待办；`sod` 每跑一次会留下一条终态探针 demand（终态 demand 不可 close，属正常残留）

## 控制台断言：已知良性噪音

`browser.mjs` 的 `BENIGN_CONSOLE_PATTERNS` 是**累积词表**，新增噪音请加进去而不是在脚本里各自绕过：

| 噪音 | 为什么是良性的 |
|---|---|
| `/api/auth/me` 401 | 会话 cookie 是 httpOnly，前端读不到，只能问服务端"我有会话吗"，未登录时答案就是 401。且 `auth-provider` 的启动探测是裸 `useEffect`，**开发模式**下被 StrictMode 双调 ⇒ 每次应用启动固定两条（生产构建实测只发一次）。 |
| `net::ERR_ABORTED` on `/stream`、`/events` | 收件箱与运行总览有常驻 SSE，每次页面导航都会被浏览器 abort，是导航的正常副作用。 |

采集侧会把 `location().url` 拼进错误字符串——**资源类错误的 `text()` 里没有 URL**，不拼就按 URL 过滤不到（踩过）。

另：**不要用 `waitUntil: "networkidle"`**，常驻 SSE 会让它永不触发、只等到超时；用 `SAFE_WAIT_UNTIL`。

## 断言要对 API 真值，不要对"页面上有没有某个词"

`capability-binding-console.mjs` 的 U7/U8 是个范例：早期版本断言"页面出现「通用」"，
结果租户里所有技能都被项目绑定之后就永远绿不了（也可能反过来真空通过）。
现在改成**逐条拿 `project_bindings` 真值对照 UI**，并在只覆盖到一种形态时显式提示，
而不是让它悄悄算过。

失真编制普查：

```bash
node scripts/casting-stale-census.mjs
```
