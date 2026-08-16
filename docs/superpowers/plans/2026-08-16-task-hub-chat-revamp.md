# 任务中枢对话面（Chat）体验重构立项方案

> 状态：批次一、批次二已落地；批次三（Composer Enter 发送、复制/时间 footer、失败词表、完成后过程折叠）施工中。Markdown 继续 react-markdown + GFM/breaks + 流式块切分。
> 日期：2026-08-16
> 范围：`apps/web` 为主；T4 视决策可能触及 `contracts/control-plane`（事件增量参数）；T10 跨 `contracts/provider` + `apps/runtime-agent` + `apps/control-plane`，**触发式另立不在本项内实施**
> 依据：对照参考实现 desktop-cc-gui（`/Users/tinker/src/github/Tools/desktop-cc-gui`，对话流/滚动跟随/过程渲染同构可借鉴）；代码核证落点见 §1
> 并行边界：`chat-panel.tsx` / `chat-panel.test.tsx` 当前有他人在途改动；执行时按宪法共享 checkout 约定，只 `git add` 显式路径、交织文件只暂存自己的 hunk，优先一子任务一 worktree

**Goal:** 把任务中枢 Chat 面从「提交-等待-看最终字符串」升级为可滚动、可渲染、过程可见、可停止的真实对话体验，并修掉会话切换丢轮次的数据可见性 bug。

**非目标：**

- 不重做 Chat 面信息架构（左轨员工/会话、右轨项目现场维持现状）
- 不引入 token 级流式（平台语义事件是事件级，不发明 delta 通道；desktop-cc-gui 的增量 markdown 切块不照搬）
- 不做 contenteditable 富输入框、fork/rewind（CLI 会话语义，与本平台 run 模型不符）
- 不把任务中枢 Chat 做成思考链时间线：产品只展示工具调用与回答。Claude Code 原生流里可以有 thinking 块，当前不映射、也不在文案里写成「过程数据没有思考链」。
- 不改 chat run 的后端语义（thread 继承、resume 降级、技能包络均不动）

---

## 1. 问题清单与核证

| 编号 | 优先级 | 问题 | 核证结论（代码落点） |
|---|---|---|---|
| C1 | P0 | 会话窗口无法滚动 | `task-hub-chat.css:370` `.hub-thread:has(.hub-entry){justify-content:flex-end}`：column flex + `overflow-y:auto` 下溢出内容伸向顶端不可滚动区（flexbox 经典坑）。另：组件无任何贴底跟随逻辑 |
| C2 | P0 | 切换数字员工/会话后只剩第一轮，后发轮次不可见 | 前端双真相水合 bug，服务端无损（`run_service.go:240` 续聊继承 `ChatThreadID`）。详见 §2.2 |
| C3 | P0 | Markdown 不渲染 | `chat-panel.tsx` 完成态直出 `<p>{entry.answer}</p>` + `pre-wrap`；仓库已有免 XSS 的 `components/superteam/markdown-prose.tsx` 未接入 |
| C4 | P1 | 工具调用/流式回答不可见 | 面板只轮询 run 状态（2.5s）取最终字符串。平台已有：契约 `listDigitalEmployeeRunEvents`、web 已封装、`run-event-timeline.tsx` 可归并 tool/text。Chat 产品面只投影工具与回答。 |
| C5 | P1 | 运行中无法停止 | 契约有 `stopDigitalEmployeeRun`（openapi `/runs/{runId}/stop`），web `employees.ts` 未封装，UI 无按钮 |
| C6 | P1 | 一轮进行中锁死全面板 | `canSend`、`handleSelectThread`、`handleNewConversation` 在 `activeEntry` 存在时拦截。**切员工 / 切项目不拦**（这正是 C2 切走再切回的路径）。T5 只放开切会话/新会话的只读浏览，不与 T2 抢发送锁 |
| C7 | P1 | 进行中零反馈 | 仅等待文案，无当前工具名、无流式回答 |
| C8 | P2 | 人类发言无头像 | 人类回合只渲染 `creatorDisplayName` 文本；员工侧有 `EmployeeAvatar`，不对称。多人共享会话场景（会话列表已分「我发起的」）比单人 GUI 更需要发起人身份 |
| C9 | P2 | 发送快捷键语义反常 | Shift+Enter 发送 / Enter 换行，与业界惯例相反；且 IME 组合检查只在 Shift+Enter 分支，改默认时需一并处理 |
| C10 | P2 | 无消息级操作/时间戳 | 无复制按钮；逐轮无时间/耗时（run 数据里 `created_at`/duration 现成） |
| C11 | P2 | 历史恢复静默截断 | `restoreQuery` 固定 `limit:100`，更早轮次消失无提示 |
| C12 | P2 | 输入框不自动增高 | `resize:vertical` 仅手动拖拽 |
| C13 | P2 | 错误直出 `error_message` | 未走 `status-labels.ts` 的 `failure_family` 口径（宪法要求） |
| C14 | 后置 | 轮询无推送 | threads 5s + run 2.5s 轮询可用；平台已有 SSE 基础设施（收件箱/员工动态流），统一时机另定 |
| C15 | 触发式 | 思考块未映射到平台事件 | **不是「过程没有思考」**。Claude Code stdout 可含 `content[].type=thinking`；OpenCode JSON 常只有 reasoning token 计数。Chat 明确不展示思考。若将来要展示，再立项映射，且动 parser 前先做 L0 重放。 |

## 2. 关键根因详解

### 2.1 C1 滚动

`justify-content:flex-end` 本意是「消息少时贴底」，副作用是溢出区在容器顶端、浏览器只在末端方向生成可滚动区。修法：删该规则，改 `.hub-entry:first-child{margin-top:auto}` 达成同样贴底且不破坏滚动。跟随逻辑另建（T1），借鉴 desktop-cc-gui `useMessagesCanvasFollow`：距底阈值内视为在底、仅滚轮上滚暂停跟随、发送/切会话强制钉底、防「markdown 渲染撑高误判离底」。

### 2.2 C2 水合丢轮（本次新发现 bug）

复现序列（代码级推演，T2 验收须真实复现）：

1. 选员工 A → 自动钉最新 thread T → `restoreQuery`（key=`[chat-restore,A,proj,T]`）拉取，缓存 = 当时轮次（如仅首轮）
2. 用户续发第 2/3 轮 → `sendMutation.onSuccess` 只 append 本地 `thread` state，**从不 `setQueryData`/`invalidateQueries`**，缓存仍是旧的
3. 切员工 B → `handleEmployeeChange` 清 `thread=[]`
4. 切回 A → 自动钉回 T，同 queryKey 命中**陈旧缓存**；`useLayoutEffect` 以旧缓存水合 `setThread((prev)=>prev.length===0?data:prev)`
5. `refetchOnMount:"always"` 后台刷新拿回全量轮次 → effect 再跑，但 `prev.length>0` 闸门拒绝覆盖 → **后发轮次永久不可见**

切会话（不切员工）、切项目再切回同机制可复现。根因是「本地 state 与 query 缓存双真相 + 单向一次性水合闸」。

修法方向（T2）必须同时做三件事，缺一会在另一侧丢消息：

1. **去掉** `setThread(prev.length===0 ? data : prev)` 水合闸与 `restoreDataRef`。
2. **按 `runId` 合并** 服务端恢复列表与本地/缓存已有条目（`mergeChatThread`）：refetch 不得丢掉发送成功但 list 尚未包含（或测试 fetcher 对 GET `/runs` 返回空）的轮次；也不得用陈旧 list 覆盖已有后发轮次。
3. 发送成功 / `getRun` 轮询终态 **`setQueryData` 写回** 对应 `["chat-restore", employeeId, projectId, threadId]`。

新会话（`selectedThreadId=null`）restore query 禁用：首条 `onSuccess` 用 `run.chat_thread_id ?? run.id` 写入该 key 再 `setSelectedThreadId`。

## 3. 子任务拆分

### T1（P0）滚动修复 + 贴底跟随

- 范围：去掉 `.hub-thread:has(.hub-entry){justify-content:flex-end}`，改顶部弹性占位（`flex:1 0 0` 的 grow 条，短内容贴底、长内容占位为 0 从而可向下滚）。轻量 stick-to-bottom：距底 ~100px 视为在底；**仅 wheel 上滚**暂停跟随；发送 / 切会话 / 恢复完成强制钉底。不跟「回到底部」浮标（批次三可加，避免和 T7 抢 composer）。
- 落点：`task-hub-chat.css`、`use-stick-to-bottom.ts`、`chat-panel.tsx`
- 验收：长会话可滚到最早消息；新消息到达时在底则跟随；上滚阅读不被拽回；发送后钉底
- 依赖：无

### T2（P0）会话状态模型重构（修 C2）

- 范围：§2.2 修法（合并 + setQueryData + 删水合闸）
- 落点：`chat-panel.tsx` 状态模型、`chat-panel.test.tsx`（须新增「A 多轮→切 B→切回 A 全量可见」；进行中切员工再切回后继续轮询至完成）
- 验收：真实链路复现步骤跑通——员工 A 发 ≥2 轮，切员工 B 再切回 A，全部轮次可见；进行中轮次切回后状态继续推进；现有测试全过
- 依赖：无（建议先于 T4）

### T3（P0）Markdown 渲染

- 范围：完成态回答换现成 `MarkdownProse`（**不**为本项加 `remark-gfm`；GFM 表格/删除线不在本批验收）。`extractAnswerText` 的 JSON.stringify 兜底走 `<pre>`。`.hub-a` 去掉 `white-space:pre-wrap`。
- 落点：`chat-panel.tsx`、`task-hub-chat.css`、`chat-panel.test.tsx`（加粗渲染、JSON 兜底）
- 验收：标题/加粗/列表/围栏代码块按 markdown 渲染；JSON 兜底不被当成 markdown
- 依赖：无

### T4（P1）过程可见性：工具调用 + 逐回合正文 + 进行中指示器

- 范围：活动 run 增量拉 `listDigitalEmployeeRunEvents` 渐进渲染；归并逻辑从 `run-event-timeline.tsx` 的 `buildTimeline` 抽取共享（原详情抽屉不回归）；聊天气泡样式渲染 tool（工具名+状态+输入/输出摘录折叠）与 text；进行中指示器 = spinner + 计时 + 当前工具名（参考 desktop-cc-gui `WorkingIndicator`）；工具行默认折叠
- 落点：`chat-panel.tsx`、共享归并模块（新文件）、`run-event-timeline.tsx`（抽取）、`task-hub-chat.css`；视 §6-1 决策可能触及 `contracts/control-plane/openapi.yaml` + 生成链
- 验收：真实 Provider 链路下发起一轮带工具调用的 chat，进行中能看到工具行出现与完成、正文逐回合出现；完成后与终稿一致
- 依赖：T2（状态模型先收敛）；设计决策 §6-1 先拍板

### T5（P1）停止运行 + 运行中解锁只读浏览

- 范围：`employees.ts` 封装 `stopDigitalEmployeeRun`；进行中回合出停止按钮（`cancelling` 过渡态展示）；`handleSelectThread` 放开运行中切会话浏览（发送仍锁）
- 落点：`lib/api/employees.ts`（+测试）、`chat-panel.tsx`、`chat-panel.test.tsx`
- 验收：真实链路停止一个长跑 chat run，状态走到 `cancelled` 且会话可继续新一轮；运行中可切到其它会话查看历史
- 依赖：无硬依赖，建议与 T4 同批

### T6（P2）人类身份头像

- 范围：首字缩写圆形头像组件（人类无头像资产体系），人类回合与员工侧布局对称
- 验收：多人会话中不同发起人可区分；名称缺失回退占位
- 依赖：无

### T7（P2）Composer 体验

- 范围：Enter 发送（Shift+Enter 换行）为默认并处理 IME 组合防误发；textarea 自动增高（上限内）；完成态回答加复制按钮；终稿 footer 一行小字（时间 · 耗时）
- 验收：中文输入法组合期 Enter 不误发；长文本输入框随内容增高
- 依赖：无

### T8（P2）历史「加载更早」

- 范围：恢复超过 limit 时给「仅显示最近 N 轮」提示 + 加载更早按钮（offset 或时间游标，按现有 list 端点能力）
- 依赖：T2（列表真相收敛后再做分页）

### T9（P2）错误口径统一

- 范围：失败卡片经 `status-labels.ts` `failure_family` 词表；原始 `error_message` 降级为次要详情
- 验收：轻量例外（纯文案），diff + 定向测试即可
- 依赖：无

### T10（触发式，另立；Chat 当前不展示思考）

- 产品口径：任务中枢只展示工具与回答。
- 触发条件：若将来要展示思考折叠行再立项。**不要把现状写成过程数据没有思考链**（Claude 原生 thinking 块存在，当前选择不映射）。动 parser 前先做 L0 raw 离线重放。
- 范围（届时）：契约 + 三家 parser 透出 + CP 存储/API + Web 折叠行
- 门禁：`verify:contracts` 全链

## 4. 批次与依赖

| 批次 | 子任务 | 说明 |
|---|---|---|
| 批次一（P0 止血） | T1 + T2 + T3 | **同一次改动**（三处都改 `chat-panel.tsx`）；T2 是 T4/T8 前置 |
| 批次二（P1 质变） | T4 + T5 | §6-1 拍板后启动 |
| 批次三（P2 补齐） | T6 + T7 + T8 + T9 | 顺序无关，可拆散跟车 |
| 项外 | T10 | 触发式，另立 |

## 5. 验证要求（宪法口径）

- T2 / T4 / T5 触及真实执行链路：验收须 Web + Control Plane + DB + Runtime + Provider 真实 E2E，记录并复核 `control-plane`/`web` pid 与 `owner=`（worktree 并行时严防静默空操作 restart）
- T1 / T3 / T6 / T7 / T8 为纯前端交互：`corepack pnpm --filter @superteam/web test` + 浏览器人工验证
- T9 走轻量例外
- 每子任务收尾走 `superteam-completion-check`

## 6. 设计决策点（实施前拍板）

1. **T4 事件拉取方式**：事件端点仅 `limit`/`offset`。候选 a) 客户端全量拉+按 `sequence_number` 合并（chat 单轮事件量小，需先确认 limit 上限够用）；b) 契约加 `since_sequence` 增量参数（改 openapi + 生成链 + `verify:contracts`）。默认建议 a) 先行，量证明不够再走 b)
2. **C9 Enter 语义**：直接换默认（Enter 发送）还是做用户偏好项。默认建议直接换 + IME 防误发，不加配置面
3. **T4 事件轮询频率**：与现有 run 状态轮询 2.5s 合并为同一节奏还是独立更快节奏（建议合并，避免双轮询）
