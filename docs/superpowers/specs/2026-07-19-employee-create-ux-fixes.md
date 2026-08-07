# 创建数字员工 UX 遗留修复专项

> 复核状态：实施中（基于锚点抽查）

日期：2026-07-19 · 状态：实施中 · 来源：创建流程浏览器 walkthrough 发现的 7 项遗留（见记忆 employee-create-page-overhaul）

排除项（原）：错误体结构化 `{code,message}` 治本——**已单独实现**，见下「② 治本落地」。

## ② 治本落地：结构化错误码（apierror）

**机制层（新建，可复用于全 handler）**：
- 后端 `internal/apierror`：`Error{Code, Status, Message, cause}` + `New/WithCause/Is/Write`。`Write` 输出 `{code, message}` JSON（`application/json`），`Is` 按 code 匹配（WithCause 副本/包装仍匹配原型）。message 是 zh-first 权威用户文本单一源。
- 前端 `lib/api/api-error.ts`：`apiErrorMessage(error, fallback)` 有 code 时取后端 `error.detail`（=message），否则 fallback；`apiErrorCode` 取 code。**不再对 `.message` 英文壳做关键词匹配**。

**员工创建链路示范落地**：`internal/employee/api_errors.go` 定义 `ErrEmployeeNameConflict` / `ErrEmployeeAvatarInUse` / `ErrEmployeeTeamCapacityExceeded`（code + 409 + 中文）；pg_repository 唯一索引冲突、service 头像前置/容量检查返回它们；`writeHandlerError` 先 `apierror.Write` 命中即输出结构化。create.tsx mutationFn 用 `apiErrorMessage`，横幅与全局 toast 共用后端中文。

**其余 handler 增量迁移**：存量 `http.Error` 纯文本错误按需迁移，不要求一次性全改；新错误直接用 apierror。这是全 handler 铺开的机制底座，非一次性重构。

## 修复清单

| # | 问题 | 方案 | 层 |
|---|---|---|---|
| 1 | 团队限额硬编码 10 且走完三步才报错 | 限额进系统配置中心 key `employee.max_per_team`（租户可配）；create-options 新增团队容量预检项（当前人数/限额，满员 blocked）；创建接口超限从 400 invalid 改 409 conflict | CP+Web |
| 2 | 错误英文透传 + 失败横幅不清除 | 前端按 status+响应体关键词映射中文（重名/头像占用/配额/兜底）；draft 变更与步骤切换时 mutation.reset() | Web |
| 3 | 进入配置后无法返回创建方式 | 身份步「上一步」启用为「返回创建方式」（setFlowStep('template')，保留草稿）；顶部步骤条已完成步可点回退 | Web |
| 4 | 模板路径自动选中第一个模板 | 删自动选中 effect，默认无选择、未选禁用「进入完成配置」；保留 ?template= 深链自动选中（明确意图） | Web |
| 5 | 预检「3 个可用模板」vs 列表「2 个」 | createOptionChecks 计数排除 custom_agent 哨兵 | CP |
| 6 | 列表卡「待配置」vs 详情「就绪」同位两口径 | 已由并发会话跨视图一致性 3.3a 交付：GetDigitalEmployeeOperationalState 按 employee_id 同源裁决 + 详情头身份态/运营态双 pill + operationalStatusLabel 抽到 features/employees/operational-status.ts 单一源。本专项不重复实现，仅在口径约定处对齐。 | （并发 3.3a） |
| 7 | writeHandlerError 500 吞日志 | default 分支记录原始 error | CP |

## 口径约定

- 身份态（status: ready/active/…）与运营态（operational_state: idle/working/needs_configuration/…）是两个维度，允许并存展示、不允许同一视觉位置互换口径（对齐 643a2f24 跨视图一致性方向）。
- 运营态词表唯一定义处 lib/status-labels.ts；overview 是运营态唯一装配源，详情页经 employee_id 过滤复用，不复刻装配。
- 团队容量的唯一事实源是 systemconfig（默认 10，registry 定义处），tenant/types.go 常量退役。

## 验证

真实浏览器 E2E：满员团队选择即预检 blocked；重名/头像冲突提示中文且改配置后横幅消失；身份步可返回创建方式且草稿保留；模板页默认无选中、深链仍自动选中；预检模板计数与列表一致；详情头双 pill 与列表卡口径可解释；500 有服务端日志。
