# 日志管理三页面重设计说明

> UI Designer · 2026-07-06 · v3 Soft-Flat
> 覆盖：登录日志 / 操作日志 / 平台事件

## 一、现状诊断

三个日志页面当前完全同构（`ShellPageHeader + WorkSurface + LogFilterBar + V3Table + LogPagination`），没有体现各自场景差异：

| 问题 | 影响 |
| --- | --- |
| 纯表格平铺，无概览 | 用户进来无法快速判断"最近发生了什么"，只能逐行扫读 |
| 无主从详情 | 日志是工作对象（可查看、可判断、可追踪），但当前点不开详情 |
| 危险/失败无视觉突出 | 失败登录、错误事件淹没在普通行里，违反"默认安静、例外强调" |
| 筛选交互弱 | 网格 Select/Input，无快捷 chips，常用筛选要点 3-4 次才能到达 |
| 模块/动作用文本输入 | 实际是枚举（authz/users/user.create），文本输入易输错 |
| severity 散落表格 | 平台事件的核心维度没有分布概览，error/warning 不突出 |

## 二、三页面共同改进（符合 v3 工作对象界面规则）

1. **顶部紧凑概览带（≤80px）**：4 张指标卡，黑粗大数字 + 灰小标签，危险项左侧 3px accent bar + 淡 soft 底
2. **快捷 chips + 完整筛选行**：高频维度一键切换（带 count badge），低频维度折叠到筛选行
3. **主从详情优先**：左表格 + 右详情面板（380–400px），预选中首行展示详情态，未选时展示空状态/摘要
4. **危险行左侧 accent bar**：`danger-row` / `warn-row` 整行首列 inset 3px 实色条；选中后让位 brand 蓝条
5. **等宽字体**用于 IP / UUID / resource_id / event_type / module.action / payload
6. **StatusPill 语义化**：ok/danger/warn/info/mute/artifact 六色，圆点 + soft 底 + text 层（≥4.5:1）
7. **时间双行**：绝对时间（tabular-nums）+ 相对时间（3 分钟前，灰小字）
8. **导出 CSV + 刷新**主操作，平台事件额外加"实时"心跳指示
9. **四态覆盖**：loading（骨架）/ empty（带筛选区分文案）/ error（带重试）/ permission denied

## 三、三页面差异化设计

### 1. 登录日志 · 安全审计台
`docs/prototypes/logs-login-security-audit.html`

**用户场景**：安全审计、排查异常登录、确认账号是否被盗

**概览带**：24h 登录次数 / 24h 登录失败（danger 高亮）/ 24h 独立 IP / 最近异常（相对时间 + 用户 + 原因）

**快捷 chips**：全部 / 登录成功 / 登录失败（danger 色）/ 登出成功，各带 count

**表格列**：时间 · 事件 · 结果 · 用户（头像+用户名+user_id）· 来源 IP · 设备/UA · 失败原因

**详情面板**：
- 事件摘要（类型/时间/结果）
- **失败警示卡**（danger soft 底，提示同 IP 失败次数与暴力破解建议）
- 来源信息（IP / 归属推测 / 会话 ID / 完整 UA）
- 账号信息（用户名 / user_id / 账号状态）
- **同 IP 近 1 小时**关联列表（可点击跳转）

**视觉重点**：失败行 danger accent bar；境外/异常 IP 在详情中用 alert-box 强调

### 2. 操作日志 · 操作追溯台
`docs/prototypes/logs-operation-trace.html`

**用户场景**：追溯谁在什么时候改了什么、排查误操作、合规审计

**概览带**：24h 操作总数 / 24h 失败操作（danger）/ 活跃模块（Top1）/ 活跃操作者（Top1）

**快捷 chips**：模块枚举（authz / users / teams / projects / skills），mono 字体 + count，替代原来的文本输入

**表格列**：时间 · 模块（artifact pill）· 动作（mute pill）· 结果 · 操作者 · 资源（`resource_type:resource_id` 等宽）· 来源 IP

**详情面板**：
- 操作摘要（模块.动作 / 时间 / 结果）
- **资源对象卡**（artifact 色图标 + 类型 + ID）
- 操作者（用户名 / user_id / IP / UA）
- **请求追踪**（request_id 可复制，说明可定位网关链路）
- **操作上下文 Payload**（深色 JSON 预览，键/字符串/数字着色）
- **同操作者近 1 小时**关联列表

**视觉重点**：资源用 artifact 紫表示"工件/对象"类别；payload 深色代码块突出可追溯性

### 3. 平台事件 · 运维监控台
`docs/prototypes/logs-platform-monitor.html`

**用户场景**：监控 Runtime 节点健康、发现能力降级、排查平台异常

**概览带**：severity 四分布卡（info 蓝 / ok 绿 / warn 橙 / danger 红），每张带占比条形图，danger/warn 用 soft 渐变底高亮，可点击作为级别筛选

**快捷 chips**：全部 / 错误 / 预警 / 成功 / 信息，对应语义色，替代原 Select

**表格列**：时间 · 级别 · 事件类型（mono）· 来源（runtime 等带图标）· 节点（mono）· 标题+描述（双行）

**详情面板**：
- 事件摘要（类型/时间/级别/来源）
- **错误警示卡**（提示同节点同类事件次数与处置建议）
- **节点信息卡**（mute 图标 + node_id + 状态 pill）
- 关联追踪（correlation_type + correlation_id 可复制）
- **Payload**（深色 JSON，含 signal/exit_code/stack_trace 等）
- **同节点近 1 小时**关联列表

**视觉重点**：error/warn 双 accent bar；顶栏"实时"心跳点（pulse 动画）；severity 卡可点击联动筛选

## 四、组件复用对照（落地到代码）

原型中的视觉结构全部对应 `apps/web/src/components/superteam/` 现有组件，不引入新 token：

| 原型元素 | 复用组件 |
| --- | --- |
| 外壳白卡 | `WorkSurface`（软壳装脆数据） |
| 顶部指标卡 | `V3MetricCard`（danger 项加 accent bar 由 `tone` 控制） |
| 页头 | `ShellPageHeader`（已有） |
| 表格 | `V3Table / V3Th / V3Td / V3Tr`（`tone="danger"` 给 accent bar） |
| 状态胶囊 | `StatusPill`（`tone` 表达 ok/danger/warn/info/mute） |
| 资源/模块类别 | `IconTile`（artifact tone）|
| 按钮 | `V3Button`（primary/outline/ghost） |
| 快捷 chips | `V3Chip`（带 count badge） |
| 筛选 | `LogSelectFilter / LogTextFilter`（已有于 `-shared.tsx`，需补 chip 行） |
| 分页 | `LogPagination`（已有） |
| 四态 | `V3StateSurface`（loading/empty/error/permission） |
| 主从详情 | 需新增轻量 `MasterDetailPanel`（左表 + 右抽屉），或复用 `Sheet` 改右侧固定面板 |

**Token**：全部使用 `apps/web/src/styles/theme.css` 的 `--v3-*`，无新增。等宽用 `font-mono`，IP/UUID 用 `tabular-nums`。

## 五、数据真实性说明

原型中的字段全部来自真实 API 定义，未编造接口：

- **登录日志**：`LoginLogRecord`（`apps/web/src/lib/api/auth.ts`）— event_type / result / username / client_ip / failure_reason / session_id / user_agent / user_id
- **操作日志**：`OperationLogRecord`（同上）— module / action / result / username / resource_type / resource_id / client_ip / request_id / user_agent
- **平台事件**：`RuntimeEvent`（`apps/web/src/lib/api/runtime.ts`）— event_type / severity / source / title / description / node_id / runtime_node_id / provider_type / correlation_type / correlation_id / payload

原型中的示例数据（用户名 zhanghua/admin/linjie、IP、时间、模块名、payload）为合理占位，落地时应来自 `listLoginLogs / listOperationLogs / listRuntimeEvents` 真实返回。

**概览带指标**：当前接口（`listLoginLogs` 等）只返回 `items[]`，不返回聚合计数。概览带的"24h 失败 7 次""独立 IP 38"等数字在原型中为示意，落地时需要：
- 方案 A：前端对当前页 items 做本地聚合（不精确，仅当页）
- 方案 B：后端补 `/api/auth/login-logs/summary` 等聚合接口（推荐，符合"概览带 ≤80px 且数据真实"）
- 方案 C：概览带暂只展示"本页统计"+ 最近一条异常（无需新接口）

**同 IP / 同操作者 / 同节点关联**：当前接口不支持按 IP/user_id/node_id 反查关联。原型中关联列表为设计示意，落地需后端支持 `?related_to=<ip|user_id|node_id>` 或前端本地过滤。

## 六、落地建议（分阶段）

**阶段 1（低风险，纯前端）**：
- 三个页面统一加顶部概览带（先用本页聚合，标注"本页统计"）
- 快捷 chips 行（替代/补充原 LogFilterBar 的网格 Select）
- 失败/错误行 `V3Tr tone="danger"` accent bar
- 时间列双行（相对时间）
- 资源列等宽 + `resource_type:resource_id` 格式

**阶段 2（中风险，需组件沉淀）**：
- 主从详情面板（右侧 380–400px 抽屉），复用现有 Sheet 或新增 MasterDetailPanel
- 详情面板填充真实字段（登录详情/操作详情/事件详情）
- payload 深色代码块组件沉淀

**阶段 3（需后端配合）**：
- 概览带聚合接口（`/summary`）
- 关联查询（`?related_to=`）
- severity 分布接口（平台事件）

## 七、验证清单

- [ ] 三页面顶部概览带 ≤80px，danger 项有 accent bar
- [ ] 失败/错误行左侧 accent bar，选中后让位 brand 蓝条
- [ ] IP / UUID / resource_id / event_type 全部等宽 + tabular-nums
- [ ] StatusPill 六色语义正确，soft 底文字用 text 层（≥4.5:1）
- [ ] 主从详情：选中行高亮，右侧面板展示对应详情
- [ ] 四态：loading / empty（区分有/无筛选）/ error / permission
- [ ] 移动端：主从详情降级为单列 + Sheet 抽屉
- [ ] 桌面端无横向溢出（表格 min-w + 横向滚动仅在窄屏）

---

**设计语言**：v3 Soft-Flat（DESIGN.md 当前唯一基线）
**Token 事实源**：`apps/web/src/styles/theme.css` 的 `--v3-*`
**组件事实源**：`apps/web/src/components/superteam/`
