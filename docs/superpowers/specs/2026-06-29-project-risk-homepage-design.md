# 项目管理风险优先首页设计
> 复核状态：已实现（2026-06-30完成）

## 背景

当前项目管理已经具备项目创建、项目详情、项目治理面板、任务发起和多项项目级 API。下一步不是从零建设项目管理，而是把 `/projects` 首页从“项目列表入口”优化成“风险优先的日常运营入口”。

参考原型为 `docs/prototypes/project-homepage-concepts/risk-first.html`。设计目标是让人类负责人进入项目管理后，优先看到需要处理的阻塞、失败、补证、等待超时和协调异常，同时保留稳定的项目浏览、筛选和详情跳转能力。

## 目标

- 首屏优先展示会阻断项目推进的风险项目，而不是平均展示所有项目。
- 汇总数字与明细队列使用同一口径，避免“总数和列表对不上”。
- 第一版使用现有真实接口落地，允许前端对当前页项目做有限补强。
- 风险识别失败时不影响项目列表、搜索、分页和详情跳转。
- 高风险审批、补证写入、验收等写操作不在首页直接完成，首页只提供筛选、排序和导航。
- 为第二阶段后端聚合接口预留稳定的数据模型和 deepLink 边界。

## 非目标

- 不在第一版新增首页直接审批发布、批量验收、负责人重新分派、补证写请求。
- 不把项目详情页、审批中心或证据审计页的完整能力复制到首页。
- 不把项目级管理入口放成全局无上下文入口；项目级入口必须依附具体项目。
- 不在第一版新增新的后端聚合接口；只在设计里定义后续接口边界。

## 信息架构

`/projects` 首页改为风险优先项目首页，结构如下：

1. 页面标题区保留项目管理定位、`新建项目` 主操作、搜索和状态筛选。
2. 顶部增加紧凑风险汇总条，展示阻塞项目、待人类决策、执行失败、补证等待、SLA/等待、Runtime/协调异常。
3. 主体为项目队列表格，默认按风险优先排序。支持全部、待我处理、高风险、失败、补证、Runtime 异常等筛选。
4. 桌面宽屏显示右侧“选中项目上下文”面板；窄屏隐藏右侧面板，项目级入口进入行内更多菜单或详情页。
5. 风险时间线不作为第一版首页主模块。选中项目上下文可展示最近 1-3 条关键事件，更多内容进入项目详情。

首页所有行内 `详情` 第一版统一进入 `/projects/$projectId`。未来后端聚合接口可返回 `deepLink`，用于精确落到项目详情的审批、证据、任务或协调段落。

## 风险口径

第一版固定 5 类风险：

| 风险类型 | 含义 | 首页表现 |
| --- | --- | --- |
| 待人类决策 | 审批、验收、驳回、需求不清，需要 `human_owner` 或验收人判断 | danger/warn 状态、排序靠前 |
| 执行失败 | 任务失败、测试失败、Runtime 写回失败证据 | danger 状态、显示失败任务或证据摘要 |
| 补证等待 | 证据不足、范围说明缺失、权限理由不足 | warn 状态、显示补证落点 |
| SLA / 等待超时 | 等待时间超过阈值或长时间无推进 | warn 状态、显示等待时长 |
| Runtime / 协调异常 | 协调线程、任务分派、Runtime 状态异常 | danger/warn 状态、显示协调或 Runtime 原因 |

风险排序规则按以下优先级处理：

1. 有 danger 风险的项目排在最前。
2. 需要人类决策的项目优先于纯信息类异常。
3. 等待时间越长越靠前。
4. 选中项目在刷新后尽量保持选中；如果项目不在当前结果中，再选当前页第一条。
5. 没有风险信号的项目按更新时间或现有排序稳定展示。

## 数据流

第一版采用“当前页轻量补强 + 选中项目深补强”。

基础层：

- 调用现有 `listProjects(filters)` 获取项目列表。
- 基础字段用于展示项目名、目标、状态、负责人 ID、协调状态、更新时间、归档状态。
- 基础列表 loading 和 error 仍然按页面级状态处理。

当前页轻量补强：

- 只对当前页项目执行轻量补强，不对全量项目做 N+1。
- 轻量补强建议使用已有接口：
  - `listProjectTasks(projectId)`：识别失败任务、待审批任务、`requires_human_approval`。
  - `listProjectDecisionRequests(projectId)`：识别待人类决策。
  - `listProjectEvidence(projectId, { limit })`：识别 rejected / submitted 证据和补证等待。
  - `listProjectEvents(projectId, { limit })`：提供最近风险事件摘要。
- 轻量补强 loading 时，表格先显示基础项目列表，风险列显示“识别中”。
- 单项目补强失败时，该行显示“风险待确认”，并保留 `详情` 跳转。
- 补强请求应限制在当前页范围内，并保持 previous data，避免搜索、分页、筛选时表格闪烁。

选中项目深补强：

- 用户选中某行后，右侧上下文面板再拉更完整的详情数据。
- 深补强可复用现有项目详情接口，例如 overview、tasks、decisions、evidence、events 中对上下文必要的部分。
- 深补强只影响右侧面板，不反向阻塞整张表。
- 右侧面板 loading、error、empty 均在面板内部展示。

第二阶段聚合接口：

- 后续由后端提供 `ProjectHomeOverview` 类聚合接口。
- 聚合接口统一返回风险汇总、项目队列行、排序键、风险原因、动作落点、`deepLink`、更新时间和权限可见性。
- 第一版前端 `ProjectRiskSummary` 类型应尽量贴近后续服务端响应，方便替换数据源。

## 前端模型

新增前端风险模型建议放在项目功能边界内，例如 `apps/web/src/features/projects/project-risk.ts`。

核心类型：

```ts
type ProjectRiskLevel = "none" | "info" | "warn" | "danger";

type ProjectRiskReasonType =
  | "human_decision"
  | "execution_failed"
  | "evidence_required"
  | "sla_waiting"
  | "runtime_or_coordination";

type ProjectRiskReason = {
  type: ProjectRiskReasonType;
  level: ProjectRiskLevel;
  label: string;
  detail?: string;
  waitingSince?: string;
  source: "project" | "tasks" | "decisions" | "evidence" | "events";
};

type ProjectRiskSummary = {
  projectId: string;
  level: ProjectRiskLevel;
  primaryReason?: ProjectRiskReason;
  reasons: ProjectRiskReason[];
  requiresHuman: boolean;
  waitingSince?: string;
  isPending: boolean;
  isPartial: boolean;
  error?: string;
  deepLink?: {
    route: "/projects/$projectId";
    tab?: string;
    targetId?: string;
  };
};
```

第一版 `deepLink` 只作为预留字段，实际点击统一进入 `/projects/$projectId`。

## 组件拆分

避免继续扩大 `ProjectsView` 的职责，建议拆分为以下单元：

- `ProjectHomeRiskSummaryBar`
  - 展示风险汇总条。
  - 只接收计算好的 counts，不直接请求数据。

- `ProjectRiskQueue`
  - 展示项目队列表格、筛选 chip、排序、分页、行选中、行内入口。
  - 接收 `Project[]` 和 `ProjectRiskSummaryMap`。
  - 不直接请求接口。

- `ProjectSelectedContextPanel`
  - 展示桌面右侧选中项目上下文。
  - 接收选中项目、轻量风险摘要、深补强数据状态。
  - 展示负责人、协调线程、主风险、最近事件和项目级入口。

- `project-risk.ts`
  - 放纯函数和类型。
  - 包含 `deriveRiskFromProject`、`mergeRiskSignals`、`sortProjectsByRisk` 等逻辑。
  - 承载第一版前端派生口径，后续可切换为服务端聚合数据。

- `useProjectRiskSignals`
  - 负责当前页轻量补强请求。
  - 只对当前页项目工作。
  - 支持失败降级和 keep previous data。

## 操作与导航

第一版首页动作范围为筛选、排序和跳转：

- `详情`：统一进入 `/projects/$projectId`。
- `发起任务`：进入现有任务发起或项目需求入口，并尽量带上项目上下文。
- `查看证据/审批`：第一版可进入项目详情，由详情页承接具体位置。
- `更多`：窄屏或低频项目级入口，包括成员与角色、数字员工池、策略与预算、协调线程、证据与审计。

不在首页直接执行：

- 审批发布。
- 请求补证写入。
- 处理失败任务写入。
- 验收通过或驳回。
- 批量审批、批量验收、负责人重新分派。

高风险动作不得使用普通蓝色实心主按钮。首页若展示风险入口，应使用 warn/danger 语义、明确文字和结构提示。

## 状态与错误处理

- 基础项目列表 loading：展示项目首页加载态或骨架。
- 基础项目列表 error：展示页面级错误，因为没有项目列表就没有首页。
- 风险补强 loading：保留基础列表，风险列显示“识别中”。
- 单项目补强 error：该行显示“风险待确认”，操作仍可进详情。
- 右侧深补强 loading：只在右侧面板内部显示。
- 右侧深补强 error：面板显示“上下文暂不可用”，保留详情入口。
- 无项目：引导 `新建项目`。
- 筛选无结果：保留筛选区，提供清空筛选。
- 窄屏：隐藏右侧上下文面板，不产生横向溢出。

## 测试与验收

前端测试：

- `project-risk.ts` 覆盖 5 类风险派生、合并和排序。
- `ProjectsView` 或拆分后的页面测试覆盖：
  - 基础列表加载。
  - 风险补强 loading。
  - 单项目补强 error 降级。
  - 筛选和排序。
  - 行选中。
  - `详情` 跳转到 `/projects/$projectId`。
  - 窄屏不渲染右侧上下文或不产生横向溢出。

项目验证：

- Web 测试使用 `corepack pnpm --filter ./apps/web run test`。
- 涉及真实页面行为时，需要用浏览器或真实接口验证当前 `/projects` 页面加载的是当前代码，不是 mock、缓存或旧服务。
- 第一版不得把 mock、组件测试或单元测试表述为真实链路已验证。

验收标准：

- `/projects` 能加载真实 `listProjects` 数据。
- 默认排序能把有风险信号的项目排在前面。
- 顶部风险汇总与当前队列口径一致。
- 当前页轻量补强失败不影响基础项目列表、分页、搜索和详情跳转。
- 详情跳转统一进入 `/projects/$projectId`。
- 宽屏显示右侧选中项目上下文；窄屏隐藏。
- 首页没有可误触的高风险写操作按钮。

## 分阶段落地

第一阶段：

- 风险优先首页 UI。
- 当前页轻量补强。
- 选中项目深补强。
- 风险汇总与队列一致。
- 统一详情跳转。

第二阶段：

- 后端 `ProjectHomeOverview` 聚合接口。
- 服务端统一风险口径、排序、deepLink 和权限可见动作。
- 前端把 `ProjectRiskSummary` 数据源从本地派生切换到聚合响应。

后续阶段：

- 在明确权限、审计和二次确认后，再评估首页是否允许补证提醒、负责人分派或其他低风险写操作。
