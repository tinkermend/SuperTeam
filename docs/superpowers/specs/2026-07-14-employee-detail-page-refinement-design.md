# 数字员工详情页优化设计
> 复核状态：已实施，现状与本设计一致：事件流语义化时间线、指标条精简、状态中文化、技能入口改抽屉等全部落地，含遗留三瑕疵07-14当日修复。

日期:2026-07-14
状态:已与人类确认设计,待实施
范围:仅 `apps/web`(Console 层),不改契约、不改 Control Plane / Runtime。

## 背景与问题

数字员工详情页(`apps/web/src/features/employees/detail.tsx` 及其子组件)存在以下问题:

1. 任务详情抽屉(`run-detail-drawer.tsx`)的事件流按 `#序号 | 英文 event_type | 原始 JSON` 三列直出,不可读。
2. 顶部指标条(`employee-metrics-strip.tsx`)中「Runtime 执行位置」「当前状态」两卡冗余:执行位置是调度事实逻辑,当前状态在头部已展示。
3. 员工状态(`ready` 等)在头部、生效上下文面板、指标条均直出英文;仓库已有 `employeeStatusLabel`(`src/lib/status-labels.ts`)未接入。
4. 「下次任务会注入的上下文包」流程图(`context-injection-chain.tsx`)无必要。
5. 生效上下文面板中技能「查看全部」跳 `/skills`(全局技能管理页),不是该员工自己的技能配置入口;MCP 跳 `/mcp` 是对的。

分析中额外发现并纳入本次范围:

6. 生效上下文面板「状态」行与头部重复且为英文。
7. 抽屉「更新时间」直出 ISO 时间戳;「结果」块原始 JSON 直出。
8. `providerDisplayName` 在 `detail.tsx`、`employee-detail-header.tsx`、`effective-context-panel.tsx` 三处复制;抽屉内 `runStatusLabel` 与 `lib/status-labels.ts` 重复。
9. 页面底部四张 JSON 快照卡(人格记忆 / 能力绑定 / 预算策略 / 运行与缓存状态)原始 JSON 直出且与其他区块重复。

## 设计决策(人类已确认)

- 事件流采用**语义化时间线**样式(非对话式 transcript),视觉模式对齐 `project-execution-trace-panel.tsx`。
- 移除两张指标卡后,命令通道信号改为**仅断开时显示页面顶部警示条**,正常时不占版面。
- 技能「查看全部」改为**打开「管理技能与 MCP」抽屉**(`onManageCapabilities`),MCP 链接保持 `/mcp`。
- 附加问题 6–9 全部纳入本次改造;底部快照卡中「能力绑定」「运行与缓存状态」两卡**删除**。

## 方案明细

### 1. 事件流语义化时间线

从 `run-detail-drawer.tsx` 拆出新组件 `apps/web/src/features/employees/components/run-event-timeline.tsx`,输入 `DigitalEmployeeRunEvent[]`。

事件类型全集(来源:`apps/runtime-agent/src/events.rs` ProviderEvent + `apps/control-plane/internal/employee/run_writeback.go` 生命周期事件):

| event_type | 展示 | 说明 |
|---|---|---|
| `session_started` | 时间线节点「会话已建立」 | 附 session id(等宽、截断) |
| `turn_started` | 时间线节点「回合开始」 | |
| `text_delta` | 正文段落 | **连续** `text_delta` 按序合并 `payload.text` 为一段正文直接展示 |
| `tool_started` / `tool_completed` | 工具调用行 | 按 `payload.tool_id` 配对为一行:工具名 + 状态(运行中/成功/失败,`is_error` 判定)+ 可折叠输入/输出摘录;`input_truncated`/`output_truncated` 为 true 时显示截断提示 |
| `turn_completed` | 时间线节点「回合完成」 | 显示 `summary` 与 `usage` token 数 |
| `turn_error` | 危险色块「回合出错」 | 显示 `message` |
| `run_dispatched` | 时间线节点「命令已下发」 | |
| `run_completed` / `run_failed` / `run_cancelled` / `run_timed_out` / `run_reaped_stale` | 终态时间线节点 | 中文标签复用 `lib/status-labels.ts` 语义 |
| 其他未知类型 | 通用回退行 | 显示原 event_type + 原始 JSON 折叠,保证新增类型不白屏 |

- 每条事件(含合并后的正文段与工具行)提供「原始 JSON」`<details>` 折叠入口;序号弱化为辅助信息。
- 事件数达到查询上限 50 条时,底部显示「仅显示前 50 条事件」提示。
- 工具配对规则:`tool_completed` 与最近一个同 `tool_id` 的 `tool_started` 合并;孤儿 `tool_completed`(前 50 条截断导致)按独立工具行展示。

### 2. 指标条精简 + 通道警示条

- `employee-metrics-strip.tsx`:删除「Runtime 执行位置」「当前状态」两卡及 `runtimeNodeLabel`、`commandChannelConnected`、`currentStatusLabel` props,剩 8 卡(Provider / 累计执行 / 近7天 / 成功率 / 平均耗时 / 成功 / 失败 / 人工停止)。
- `detail.tsx`:`runtimeCommandChannelDisconnected` 为 true 时,内容区顶部渲染危险色 Alert「Runtime 命令通道未连接,暂不能开始任务」。开始任务抽屉中的禁用原因逻辑保持不变。

### 3. 状态中文化

详情页所有直出 `employee.status` 的位置接入 `employeeStatusLabel`:

- `employee-detail-header.tsx:52` 头部 StatusPill。
- 其余详情页内直出位置随第 6 项一并处理(「状态」行直接删除)。

### 4. 删除上下文包流程图

删除 `context-injection-chain.tsx`、`context-injection-chain.test.tsx` 及 `detail.tsx` 中的引用;`detail.tsx` 中仅为该组件服务的派生数据(如 `hasPersonaMemory` 传参)一并清理,技能/MCP/环境变量计数仍被生效上下文面板使用,保留。

### 5. 技能跳转修正

`effective-context-panel.tsx` 技能区「查看全部」由 `<Link to="/skills">` 改为触发 `onManageCapabilities`(与环境变量「查看详情」一致的 ghost button)。MCP「查看全部」保持 `/mcp`。

### 6. 生效上下文去重

删除基本信息中「状态」行;保留 Provider / 角色 / 工作目录。

### 7. 抽屉格式化

- 「更新时间」用本地化时间格式(与 `project-execution-trace-panel.tsx` 的 `formatTimestamp` 同规则,可抽共享)。
- 「结果」块:优先提取结果对象中的文本型结论字段(`summary` / `conclusion` / `text` / `message` 中首个字符串值)作为正文展示,完整原始 JSON 收进折叠;无可提取字段时保持 JSON 折叠展示。
- 「命令」「节点」值用等宽字体。

### 8. 重复代码收敛

- `providerDisplayName` 抽为 `apps/web/src/features/employees/provider-label.ts`(或就近共享模块)单一实现,三处调用点改为引用。
- 抽屉内本地 `runStatusLabel` 删除,改用 `lib/status-labels.ts` 的 `runStatusLabel`。

### 9. 底部快照卡改造

`EmployeeConfigSnapshotSection`(位于 `detail.tsx`):

- **保留「人格记忆」**:内容为 markdown 文本,改为正文排版(`whitespace-pre-wrap` 普通文本样式,非 JSON `<pre>`),空态「未设置」。
- **保留「预算策略」**:渲染为可读字段「每日 Token 上限:N」(`daily_token_limit`,空态「未设置」);出现未知额外字段时以键值对列表回退展示。
- **删除「能力绑定」卡**(与生效上下文面板计数及能力抽屉重复)。
- **删除「运行与缓存状态」卡**(runtime 事实信息不常驻展示,与第 2 项同理);随之删除 `runtimeState` 派生逻辑。

## 错误处理与边界

- 事件流:payload 字段缺失/类型不符时逐字段容错(取不到就不渲染该部分),未知 event_type 走通用回退行;不因单条脏数据崩整个时间线。
- 通道警示条仅在 `runtimeOverview` 查询成功且 `command_channel_connected === false` 时显示,查询失败/加载中不显示(与现有 `canStartTask` 判定一致)。
- 结果结论提取只接受字符串类型字段,防注入依赖 React 默认转义,不做 HTML 渲染。

## 测试与验证

- 更新/新增组件测试:`run-detail-drawer.test.tsx`(或新增 `run-event-timeline.test.tsx`,覆盖各事件类型、text_delta 合并、工具配对、孤儿 completed、未知类型回退、50 条提示)、`employee-metrics-strip.test.tsx`、`employee-detail-header.test.tsx`、`effective-context-panel.test.tsx`、`detail.test.tsx`;删除 `context-injection-chain.test.tsx`。
- 分层门禁:`corepack pnpm --filter @superteam/web test` → `corepack pnpm verify:web`。
- 端到端真实验证(默认完成条件):`scripts/dev-services.sh status` 确认服务在跑并加载当前代码后,浏览器打开真实员工详情页,验证:真实运行的事件流时间线渲染、状态中文、指标条 8 卡、无上下文包流程图、技能「查看全部」打开抽屉、断开 Runtime 命令通道时警示条出现、底部两卡展示。
