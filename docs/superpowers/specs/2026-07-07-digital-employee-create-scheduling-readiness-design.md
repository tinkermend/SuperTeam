# 数字员工创建到调度就绪闭环设计

日期：2026-07-07
状态：已确认，待实现计划

## 1. 背景

当前数字员工创建页已经具备“选择创建路径与模板/员工类型 -> 配置预检 -> 身份/能力/治理/执行器配置 -> 确认创建”的主流程。后端 `POST /api/v1/digital-employees` 会创建员工事实、初始环境变量、初始员工配置修订、已批准有效配置，并将员工标记为 `ready`。

本轮要解决的是创建成功之后的能力边界和用户心智：`ready` 只能表示员工画像、治理配置和上下文注入事实已经准备好，不能表示该员工已经直接绑定 Runtime 或可以从员工详情页独立执行任务。当前服务层已经明确拒绝数字员工直接绑定 Runtime：Runtime 节点应绑定到 Project，并由 Project Task 调度数字员工执行。

因此，本设计不把数字员工创建扩展为“创建即绑定 Runtime”。正确闭环是：创建流程产出可调度员工画像；员工详情展示调度前置条件；项目侧 Runtime placement/readiness 决定某个项目是否具备真实执行能力；最终通过 Project Task 做真实 Runtime/Provider smoke。

## 2. 当前代码事实

- Web 创建页已有模板/空白自定义、预检、身份、能力、治理、执行器和环境变量输入，提交到 `POST /api/v1/digital-employees`。
- Control Plane 创建服务会落员工、初始环境变量、配置修订、有效配置，并把员工状态更新为 `ready`。
- `BindExecutionInstance` 当前返回错误：数字员工不直接绑定 Runtime，Runtime 节点绑定到项目并通过项目任务调度。
- 员工详情页已经读取员工、执行实例、运行历史、Runtime overview、有效配置、技能、MCP 和环境变量，并展示“开始任务”入口。
- 项目侧已有 `GET /api/v1/projects/{projectId}/runtime-readiness`，用于判断项目 Runtime placement、Provider、员工状态和调度阻断原因。

## 3. 目标

- 将“创建数字员工”收敛为员工画像、治理边界、能力上下文和 Provider 偏好的创建，不再暗示员工创建时会绑定 Runtime。
- 创建成功后，在员工详情页提供“调度就绪检查”，让用户看到员工自身是否具备进入项目调度池的前置条件。
- 将 Runtime 可执行性明确交给项目 Runtime placement/readiness，不在员工详情页制造第二套执行事实源。
- 保留现有技能、MCP、环境变量和有效配置入口，但以统一 checklist 呈现缺失项、继承项和下一步动作。
- 固定验收 smoke：创建员工 -> 员工调度就绪检查 -> 项目员工池/项目任务关联 -> 项目 Runtime readiness -> Project Task 真实执行或明确阻断。

## 4. 非目标

- 不实现数字员工直接 Runtime 绑定。
- 不恢复或扩展 `PUT /api/v1/digital-employees/{employeeId}/execution-instance` 的员工绑定语义。
- 不在创建时物化 Runtime agent home、Provider 会话或本地工作目录。
- 不让 Control Plane 直接执行本地命令。
- 不新增任意自定义员工类型。
- 不把项目 Runtime placement 的判断复制成员工详情页里的第二套规则。
- 不把创建流程改成项目任务发起流程。

## 5. 产品口径

数字员工创建完成后，系统只承诺以下事实：

- 员工身份、职责、头像、类型和归属已经创建。
- 员工有效配置已通过团队治理和员工配置校验。
- 能力上下文可以由员工自有配置和团队继承配置组成。
- 环境变量可以作为运行时注入材料被管理和审计。
- Provider 类型是员工可使用执行器能力的偏好或约束，不是 Runtime 节点绑定。

数字员工是否能真实执行任务，需要在项目上下文中判断：

- 项目是否有 Runtime placement。
- Runtime 节点是否在线、已批准、session active、命令通道可用。
- Provider capability 是否可用。
- 项目员工池中是否包含状态可用的数字员工。
- Project Task 是否能被调度并收到 Runtime/Provider 回执。

## 6. 能力缺口矩阵

| 环节 | 当前已有 | 缺口 | 设计处理 |
| --- | --- | --- | --- |
| 创建入口 | 模板/空白自定义、预检、四步配置、环境变量输入 | 执行器步骤容易被理解为 Runtime 绑定 | 改为 Provider 偏好/能力约束，不展示 Runtime 绑定承诺 |
| 创建提交 | `POST /api/v1/digital-employees` 创建员工和配置事实 | handler 接收 `runtime_node_id`，但 service 不创建执行实例 | 前端不再依赖 `runtime_node_id` 作为创建语义；后端后续可清理无效字段或保持兼容但不宣称生效 |
| 员工详情 | 有效配置、技能、MCP、环境变量、运行历史、Runtime overview 查询 | “未绑定 Runtime，不能开始任务”与当前项目调度模型冲突 | 替换为调度就绪检查和项目任务入口指引 |
| 能力配置 | 技能、MCP、环境变量接口和管理抽屉已有 | 没有统一视图说明自有/继承/缺失 | 详情页聚合展示自有、团队继承、未配置和下一步操作 |
| 调度就绪 | 项目侧已有 runtime readiness | 员工侧没有“可进入项目调度池”的只读 readiness | 增加员工调度就绪读模型，限定为员工自身前置条件 |
| 真实验证 | 项目 Runtime readiness 与 Project Task 链路已有 | 创建流程验收没有固定真实 smoke | 把 Project Task Runtime/Provider smoke 作为最终验收路径 |

## 7. 后端设计

### 7.1 新增员工调度就绪读模型

新增一个只读能力，建议路径：

```http
GET /api/v1/digital-employees/{employeeId}/scheduling-readiness
```

该接口只回答“该员工自身是否满足进入项目调度池的前置条件”，不判断某个项目的 Runtime 是否可执行。响应字段建议：

```json
{
  "employee_id": "uuid",
  "status": "ready",
  "ready_for_project_scheduling": true,
  "checks": [
    {
      "code": "employee_status",
      "status": "passed",
      "label": "员工状态",
      "message": "员工状态为 ready"
    }
  ],
  "capabilities": {
    "skills": { "personal_count": 1, "inherited_count": 2, "missing_required": [] },
    "mcp_servers": { "personal_count": 0, "inherited_count": 1 },
    "environment_variables": { "configured_count": 2, "missing_names": [] }
  },
  "project_execution_source": "project_runtime_readiness"
}
```

检查项初版只聚合现有事实：

- `employee_status`：员工状态必须是 `ready` 或 `active`。
- `effective_config`：必须存在已批准有效配置。
- `team_assignment`：团队归属为可选；无团队员工可以作为租户级员工存在，但需要在项目选择时显式纳入。
- `skills`：展示自有和继承数量，不把“0 个技能”默认判为阻断。
- `mcp_servers`：展示自有和继承数量，不把“0 个 MCP”默认判为阻断。
- `environment_variables`：展示配置数量和缺失变量名；只有已声明为必需但未配置的变量才阻断。
- `project_runtime`：固定返回 `info`，提示最终执行能力由项目 Runtime readiness 判断。

### 7.2 不改变 Runtime 绑定模型

保持 `BindExecutionInstance` 的当前业务边界：数字员工不直接绑定 Runtime。若现有 OpenAPI 仍暴露 execution-instance 读写接口，本轮不扩展它；实现时可以只在前端停止把它作为创建后必需项。

后续如要清理契约，应另开兼容性设计，评估已有详情页、测试和历史数据，不在本轮混入。

### 7.3 复用项目 Runtime readiness

项目侧 `GET /api/v1/projects/{projectId}/runtime-readiness` 仍是执行准备事实源。员工调度就绪接口不得复制 Runtime 节点在线、session、Provider capability、project placement 等项目上下文判断。

## 8. 前端设计

### 8.1 创建页

创建页继续保留四步配置，但调整执行器步骤语义：

- 标题从“执行器”收敛为“Provider 偏好”或“执行能力约束”。
- 说明文案明确：此处选择的是员工可使用的 Provider 类型；Runtime 节点会在项目 Runtime placement 中决定。
- 创建确认页不展示“Runtime 已绑定”或类似承诺。
- 若 `create-options.runtime_provider_options` 全部不可用，创建页可以展示风险提示，但不应把它表述为员工创建一定失败；最终是否阻断由后端创建校验和项目 readiness 决定。

创建提交仍使用现有 `POST /api/v1/digital-employees`。前端不新增必需字段，不要求创建时 provision Runtime。

### 8.2 员工详情页

将详情页核心行动从“开始任务”调整为“用于项目任务”：

- 详情头主操作引导到项目选择或项目任务创建入口，而不是直接创建员工 run。
- 当前员工 run 历史可以保留为历史兼容视图，但不能作为新模型的主要执行入口。
- 删除或改写“未绑定 Runtime，暂不能开始任务”这类员工直接绑定口径。
- 新增“调度就绪检查”面板，展示员工状态、有效配置、技能、MCP、环境变量、团队/项目使用指引。
- 面板中加入“下一步”动作：管理技能与 MCP、编辑环境变量、查看可使用项目、进入项目 Runtime readiness。

详情页仍使用 v3 Soft-Flat 工作台风格：信息密度高、状态可解释、优先复用现有 `SoftCard`、`StatusPill`、`V3Button` 和现有能力面板结构。

### 8.3 项目页衔接

项目页已有 Runtime placement/readiness 组件。本轮只需要在员工详情提供跳转或提示，不重新设计项目 Runtime placement。

如果项目选择员工池时发现员工自身 readiness 阻断，项目页应展示相同 code/message，避免员工详情和项目页出现两套解释。

## 9. 数据流

```text
CreateEmployeeView
  -> GET /api/v1/digital-employees/create-options
  -> POST /api/v1/digital-employees
  -> EmployeeDetailView
  -> GET /api/v1/digital-employees/{employeeId}/scheduling-readiness
  -> user selects or opens a Project
  -> GET /api/v1/projects/{projectId}/runtime-readiness
  -> Project Task dispatch
  -> Runtime Agent / Provider execution
  -> Project Task attempt writeback
```

创建页只负责产生员工事实。员工详情负责解释员工是否具备进入项目调度池的前置条件。项目 readiness 负责解释某个项目当前是否能真实执行。

## 10. 错误处理

- 创建选项加载失败：阻止继续配置，展示后端错误。
- 创建提交失败：展示后端错误，不吞掉 validation detail。
- 员工调度就绪接口 404：展示“员工不存在或无权访问”。
- 有效配置缺失：检查项 `effective_config` 为 blocked，提示进入配置修订/审批流程。
- 环境变量缺失：展示缺失变量名；只有后端标记为 required 的缺失项才 blocked。
- 项目 Runtime readiness 阻断：在项目上下文展示阻断原因，不在员工详情页替用户推断。
- Runtime/Provider 不可用：不影响员工创建事实，但影响项目执行 readiness。

## 11. 验收标准

- 创建页不再暗示数字员工会直接绑定 Runtime 节点。
- 创建成功后员工详情显示调度就绪检查，而不是以 execution instance 作为唯一可执行条件。
- 员工详情能解释有效配置、技能、MCP、环境变量的自有/继承/缺失状态。
- 员工详情把执行引导到项目任务或项目运行准备，而不是直接从员工详情发起新的主要执行路径。
- 新增员工调度就绪读模型不复制项目 Runtime readiness 的判断。
- 项目 Runtime readiness 仍是 Runtime/Provider 是否可执行的事实源。
- 验收 smoke 能证明：创建员工成功、调度就绪检查可读、项目 readiness 给出明确 ready 或阻断、真实 Project Task 能执行或明确阻断。

## 12. 测试计划

后端测试：

- 员工 `ready` 且存在有效配置时，scheduling readiness 返回 passed。
- 员工状态不是 `ready/active` 时，返回 blocked。
- 缺少有效配置时，返回 blocked。
- 技能、MCP、环境变量数量来自现有服务或 repository 聚合。
- 接口不检查 Runtime node online、session active 或 project placement。

前端测试：

- 创建页执行器步骤文案不再使用 Runtime 绑定口径。
- 创建确认页不展示 Runtime 已绑定承诺。
- 员工详情展示调度就绪检查。
- 员工详情在 execution instance 404 时不再把“未绑定 Runtime”作为主阻断文案。
- 调度就绪 blocked 项展示下一步动作。

局部验证命令：

```bash
corepack pnpm --filter ./apps/web run test -- src/features/employees/create.test.tsx src/features/employees/detail.test.tsx
go test ./apps/control-plane/internal/employee ./apps/control-plane/internal/api
git diff --check
```

真实链路验证：

- 使用真实登录会话访问 `/employees/new`。
- 完成一次模板或空白自定义员工创建。
- 进入员工详情并确认调度就绪检查来自真实接口。
- 将该员工纳入一个真实项目员工池，或选择已有包含该员工的项目。
- 调用或打开项目 Runtime readiness，确认 ready 或阻断原因来自真实项目上下文。
- 如果本地 Runtime/Provider 可用，派发一次 Project Task 并确认 Runtime/Provider 回执和任务结果写回。

## 13. 实现范围

预期修改：

- `contracts/control-plane/openapi.yaml`：新增员工调度就绪只读接口和 schema。
- `apps/control-plane/internal/employee/**`：新增 readiness 聚合服务、handler、repository 适配和测试。
- `apps/control-plane/internal/api/**`：注册路由并补 route 测试。
- `apps/web/src/lib/api/employees.ts`：新增 API client 类型和函数。
- `apps/web/src/features/employees/create.tsx`：调整执行器步骤和确认页口径。
- `apps/web/src/features/employees/detail.tsx` 及相关组件：新增调度就绪检查面板，调整主操作和阻断文案。

不修改：

- Runtime Agent 执行模型。
- Provider adapter。
- 项目 Runtime placement 核心语义。
- 数字员工直接 execution-instance 绑定服务语义。
- 数据库迁移，除非实现时发现 readiness 聚合需要持久化新事实；当前设计不需要。

## 14. 后续计划边界

本设计确认后，下一步进入实现计划。实现计划应拆成后端只读 readiness、前端创建页口径调整、员工详情就绪面板、项目 smoke 验证四个阶段。

如果后续决定移除或废弃数字员工 execution-instance 接口，需要另写兼容性设计，处理历史数据、OpenAPI 兼容和现有详情页调用，不与本轮创建闭环混做。
