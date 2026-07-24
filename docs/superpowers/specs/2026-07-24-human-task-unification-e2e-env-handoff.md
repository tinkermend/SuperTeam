# 交接：human-task-unification 真实 E2E 的环境前置（§11 第 5 项未满足）

- 日期：2026-07-24
- 来源：另一会话在做 spec 前置条件核查时的实测结论
- 对应 spec：`2026-07-24-human-task-unification.md` §8 门禁 G1–G8、§11 开工前置清单
- 结论：**跑 G1–G8 之前必须独占 dev 环境；当前不满足。** 人类已就方案拍板（见 §3）。

---

## 1. 为什么这是个前置问题

`scripts/dev-services.sh` 的 pid / log 目录是 `PROJECT_ROOT` 相对的：

```
scripts/dev-services.sh:10  PID_DIR="${SUPERTEAM_DEV_PID_DIR:-$PROJECT_ROOT/.scratch/dev-services/pids}"
scripts/dev-services.sh:11  LOG_DIR="${SUPERTEAM_DEV_LOG_DIR:-$PROJECT_ROOT/.scratch/dev-services/logs}"
```

后果：**在 worktree 里执行 `dev-services.sh start` 不会看到主 checkout 已经起着的那套服务**（各自的 `.scratch`），于是直接抢端口——CP `:8080`、Web `:3000`、Temporal `:8233`。而 CORS 只白名单 `:3000`，换端口绕不过去。

也就是说：**服务只能有一套，且它从哪个 checkout 起，加载的就是哪个 checkout 的代码。** G1–G8 全是真实链路门禁，跑的必须是本 spec 的代码，不能是 main 上的旧代码。

## 2. 当前占用事实（2026-07-24 16:0x 实测）

- 服务全部从**主 checkout** 起着：temporal / control-plane / web / runtime-agent / feishu-connector。
- 有会话正在同一 dev 环境跑 F1 复验：审计项目 `4627824a` 里出现了两条外来需求 `abab0c3d`（15:44:06）、`53262975`（15:54:35），且 15:55 有一张 open 的「确认项目计划版本」卡在收件箱里。
- 同一时段 planner（`deepseek-v4-pro`）三次上游挂起把 `PlanDemandRoute` 打超时（14:38、14:57、15:42），这是人类明确选择保留 v4-pro 后接受的成本，不是代码问题。

两件事叠加的效果：**任何真实 E2E 的断言（尤其 G5 规划失败可关闭、G6 等待人工信号）都会被别人的数据和别人的 planner 重试污染。**

## 3. 人类已拍板：走方案 A

| | 方案 | 说明 |
|---|---|---|
| ✅ | **A：独占** | 停掉主 checkout 的服务，改从本 spec 的 checkout / worktree 起 |
| | B：先合再验 | 代码合回 main 再跑 E2E——违反"验证通过再合并"，已否 |

如果本 spec 就是在主 checkout 的 main 上开发的（当前状态即如此），那 A 退化为：**确保没有别的会话在用这套服务，然后 `restart` 让服务加载当前代码**。

### 操作步骤

```bash
# 1. 确认没有别的会话在跑（问一声，或看 open 待办与在跑任务）
./scripts/dev-services.sh status

# 2. 让服务加载当前代码（CP 是 go run，重启即生效；web 是 vite dev 已热更）
./scripts/dev-services.sh restart control-plane
./scripts/dev-services.sh restart web        # 仅在改了 vite 配置/依赖时需要

# 3. 确认加载的是当前代码，不是缓存或旧进程
./scripts/dev-services.sh status
```

若改在 worktree 起服务，顺序必须是：**先 `stop` 主 checkout 的全部服务**，再在 worktree 里 `start`；否则端口冲突且 `status` 互相看不见。

注意：
- `start|restart control-plane` 会先跑 Atlas 迁移；本 spec 若无迁移（`planning_failed` 只是 varchar 值，`project_demands.status` **无 CHECK 约束**，已实测确认）则不涉及。
- runtime-agent 与 feishu-connector 不必跟着重启，但 provider 执行（claude-code）需要 runtime-agent 在线，跑 G1/G4 前用 `status` 确认。
- 重启 CP 会中断别人在飞的 chat 会话与心跳，这是"独占"的代价，切换前务必确认无人在用。

## 4. 现成夹具（不要误删）

| 对象 | 用途 |
|---|---|
| 需求 `dbd24727-…`（项目 `4627824a`） | F1 卡死现场：`acceptance_pending` + 决策已被点掉 + 签署 404。修好后可用来验证"能否恢复" |
| 需求 `84fd333d-…`（同项目） | G5 夹具：规划失败僵尸，等 `close_demand` API 做出来后用真实 API 关掉它就是 G5 证据 |
| 需求 `c6e894fc-…`（项目 `d52727ee` 流程空态验证） | 存量僵尸（非本次产生），已滞留 >4 小时，可作对照 |

审计项目 B / C 已删除。

## 5. 另一项未满足前置（§11 第 5 项）：G4 夹具

G4 需要"多任务且有依赖"的需求，三次试跑全被 planner 打掉，**尚未验证 planner 会按预期分解**。可复用的需求文案（同类措辞在 `流程空态验证` 里确实分解出过 2 个有依赖的任务）：

> 标题：先生成 /tmp 顶层清单，再基于清单统计扩展名占比
> 正文：分两步完成，第二步必须基于第一步的产出：第一步只读扫描 /tmp 顶层，生成条目清单（文件名+类型）作为交付物；第二步读取第一步的清单，统计各扩展名占比并输出中文报告。禁止修改或删除任何文件。

若 planner 仍只产出单任务，退路是按仓库既有先例直插 DB 造 task graph（`docs/superpowers` 里多份 spec 记录过这一手法），但那样 G4 就不是全真实链路，需在结论里如实标注。

---

## 6. 附：核查中发现的新缺陷（F1 的姊妹，spec 未覆盖，请决定是否纳入）

**现象**：任务进入 `waiting_human` 却**没有任何人类可见的入口**——不是"卡片动作无效"（F1），而是"根本没有卡片"。

**现场**（不是我造的夹具，是另一会话跑 F1 复验时自然产生的，实测已滞留 25 分钟以上）：

```
project_tasks   id=a144f12d-6fc0-49d5-8546-f078ef4074a9  "生成中文简报"
                status=waiting_human   waiting_reason=clarification
                waiting_request_id=NULL
project_decision_requests  where project_task_id=a144f12d…  →  0 行
inbox_items                where source_task_id=a144f12d…  →  0 行
project_events             project_task.failed @ 15:45:36（无 waiting_human 事件）
```

**机制**（`apps/control-plane/internal/project/service.go`）：

`RecoverProjectTaskAttemptFailure`（约 :4995-5053）在执行失败后：
- `projectTaskFailureAction`（:5342-5364）可能返回 `ProjectTaskStatusWaitingHuman`；
- `humanWaitReasonForFailureFamily`（:5876）对未识别的 failure family **默认落 `clarification`**；
- writeback 把任务置为 `waiting_human` + 该 reason，**但全程不创建 decision request、不投影 inbox**；
- 且 :5030 只有在 `result.Task.Status == failed` 时才 `SignalEmployeeTaskFailed`，落到 `waiting_human` 时**连协调线程信号都不发**。

对照 `WaitHumanProjectTaskAttempt`（:5055-5108）——同样是把任务置为 `waiting_human`，但它**建 decision request（:5085）并 upsert inbox（:5102）**。两条通往同一状态的路径，一条建卡一条不建，这正是本 spec §2 判定的根因（没有唯一的人类待办对象，只有各自为政的投影）的又一实例。

**后果**：任务永久静默阻塞——收件箱没有、流程编排的 `waiting_human` 计数也拿不到（该任务确实是 `waiting_human`，会被 §5.6 的口径统计到，但人类点进去无处可操作）。

**建议**：并入 spec §4.4 的 `(kind, action) → handler` 注册表与 CI 断言的**对偶断言**——不只是"每个动作必须有 handler"，还要"**每个进入 `waiting_human` 的路径必须产出一条 HumanTask**"。可加一条服务端不变量测试：任务状态置 `waiting_human` 后，必存在对应的 open 待办。

是否纳入本 spec 由你决定；若不纳入，建议单独立项，不要只留在对话里。
