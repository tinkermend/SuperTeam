# 项目创建负责人池设计

日期：2026-06-29
状态：已确认，待实施计划

## 1. 背景

当前新建项目流程已经偏向复杂角色模型：Leader、验收负责人、审核人、单个授权团队，以及一个只有提示文案的最终确认 Tab。这个模型与本次确认的产品边界不一致。

项目创建阶段应该保持简单：项目有一个主负责人作为现有系统兼容锚点，也可以有多个项目人类负责人共同承担结果验收、审批和项目管理。创建项目只创建项目容器和明确选择的项目成员，不发起任务。

## 2. 目标

- 新建项目可以选择一个或多个项目人类负责人。
- 保留 `human_owner_user_id` 作为主负责人和系统兼容锚点。
- 多选团队只作为数字员工候选来源，不自动扩大项目授权。
- 只有被明确勾选的数字员工会进入项目数字员工池。
- 移除独立确认 Tab，由右侧常驻审阅面板承担最终核对。
- 任务发起不再要求用户选择审核人，后端默认把人类决策派给主负责人。

## 3. 非目标

- 本阶段不把 `projects.human_owner_user_id` 改成数组字段。
- 本阶段不实现多人审批、多人待办或负责人抢单机制。
- 来源团队不会作为 `team` 类型项目成员写入项目。
- 项目创建不会合并任务提交。
- 不删除 `leader_user_id`、`acceptance_user_id` 等后端兼容字段，只是在新建项目流程里停止主动使用。

## 4. 当前代码事实

- `CreateProjectInput` 和 `CreateProjectRequest` 当前包含 `human_owner_user_id`、可选 `leader_user_id`、可选 `acceptance_user_id` 和 `members`。
- `ProjectMemberInput` 支持 `principal_type`、`principal_id`、`project_role`、`display_name_snapshot` 和可选 `settings`。
- 后端创建项目时会自动把 `human_owner_user_id` 补成 active `owner` 成员。
- 项目和工作流列表可见性已经会检查 active human `project_members`，不是只依赖项目根字段。
- 当前任务发起表单要求审核人，并优先使用 `reviewer` 成员，找不到时才 fallback 到 `human_owner_user_id`。
- 当前项目验收和计划审查仍是单目标人类决策，`human_owner_user_id` 是最稳妥的默认目标。

## 5. 数据模型

`projects.human_owner_user_id` 保持必填、单值。新建项目时它始终等于当前登录用户。这个用户是主负责人，也是自动审批、需求确认、高风险暂停和最终验收的默认目标。

项目负责人池用 `project_members` 表达：

- 当前登录用户：由后端保证存在为 `principal_type = human_user`、`project_role = owner` 的成员。
- 额外选择的人类负责人：前端作为额外 `owner` 成员提交。
- 新建项目不再创建 `leader`、`acceptance` 或 `reviewer` 成员。

多选来源团队不持久化为项目团队成员。它们只用于创建时筛选候选数字员工。最终项目数字员工池仍然只由被勾选的数字员工成员组成，即 `principal_type = digital_employee`、`project_role = executor`。

## 6. 新建项目交互

创建入口保持当前 `/projects/new` split-console 页面，不回退抽屉。流程从 5 步收敛为 4 步：

1. **基础信息**：项目名称、目标、描述。
2. **人类负责人**：展示当前用户为主负责人，并允许搜索添加其他 active 人类用户作为项目负责人。
3. **数字员工池**：先多选数字员工来源团队，再从这些团队的候选数字员工中搜索和勾选项目执行员工。
4. **策略预设**：保留现有策略预设和开关，但文案中的审批、验收和确认对象统一描述为主负责人或项目负责人。

独立“确认创建”Tab 移除。右侧常驻审阅面板就是最终核对面板，应展示：

- 项目事实。
- 主负责人和额外项目负责人。
- 已选数字员工来源团队。
- 已选数字员工。
- 策略和审计摘要。
- 创建只创建项目容器、不发起任务的提示。

底部主操作为“创建项目”。必填项未就绪时禁用。必填项包括项目基础事实、当前用户、至少一个来源团队，以及来源团队授权有效。数字员工选择保持可选，因为可以先创建项目容器，后续再补齐执行池。

## 7. 提交载荷映射

新建项目提交载荷按以下规则构造：

- `human_owner_user_id`：当前登录用户 ID。
- `leader_user_id`：不提交。
- `acceptance_user_id`：不提交。
- `team_id`：本阶段不新增来源团队字段，固定使用第一个已选来源团队作为后端兼容值。
- `members`：额外人类负责人 + 已选数字员工。
- 策略对象：保留现有 preset/toggle 映射，但文案改成主负责人语义。

额外人类负责人按以下成员提交：

```json
{
  "principal_type": "human_user",
  "principal_id": "<user_id>",
  "project_role": "owner",
  "display_name_snapshot": "<display name>"
}
```

已选数字员工沿用当前 executor 映射：

```json
{
  "principal_type": "digital_employee",
  "principal_id": "<employee_id>",
  "project_role": "executor",
  "display_name_snapshot": "<employee name>",
  "settings": {
    "role": "<employee role>",
    "risk_level": "<risk level>"
  }
}
```

## 8. 任务发起

任务发起表单不再渲染或要求“审核人”字段。用户只需要选择项目、填写需求，并选择优先级和风险级别。

`SubmitProjectDemandInput` 应允许 reviewer 字段缺省。当前端没有提交 reviewer 时，后端将人类决策目标解析为 `project.human_owner_user_id`，从而保持现有单目标审批和 inbox 模型。

后端自动 fallback 时仍应记录 reviewer preference 或等价审计元数据，让后续协调和审计页面能解释为什么主负责人成为处理人。

## 9. 后端行为

项目创建需要接受负责人池载荷，不要求 leader、acceptance 或 reviewer 成员。

任务需求提交需要满足：

- 接受缺失的 `reviewer_user_id`。
- 默认目标用户为 `project.human_owner_user_id`。
- 只有当项目缺少有效主负责人时才拒绝提交；对合法项目这应当不会发生。
- 旧客户端显式提交 reviewer 时仍保持兼容。

项目协调仍保持计划审查、需求确认、高风险暂停和项目验收的单目标行为。多个项目负责人不会在本阶段生成多个 pending decision request。

## 10. 错误处理

- 当前用户加载失败时，禁止创建项目。
- 所选来源团队未被授权用于创建项目时，禁止创建项目。
- 已选来源团队失效时，从草稿中移除对应候选数字员工，并给出简洁提示。
- 某个来源团队的数字员工加载失败时，其他团队仍可继续使用，并在来源团队区域标记失败团队。
- 任务提交时如果后端无法解析主负责人，前端展示后端错误，不再要求用户选择审核人。

## 11. 测试

前端测试应覆盖：

- 新建项目页面展示“人类负责人”，不展示 Leader、验收负责人、审核人或旧确认 Tab。
- 当前用户作为主负责人固定包含。
- 额外人类负责人以 `owner` 成员提交。
- 多个来源团队的数字员工候选会合并展示。
- 选择来源团队不会提交 team 类型项目成员。
- 任务发起不渲染审核人选择器，并能在没有 reviewer 字段时提交。

后端测试应覆盖：

- 项目创建接受多个 owner 成员。
- 项目创建不要求 leader、acceptance 或 reviewer 成员。
- 需求提交缺少 reviewer 时默认使用 `human_owner_user_id`。
- 显式 reviewer 提交仍兼容旧客户端。
- 没有 reviewer 时，计划审查或验收目标仍落到主负责人。

如果 OpenAPI schema 或生成的前端类型仍把任务发起 reviewer 字段标记为必填，实施时必须同步契约和生成产物，并运行契约验证。

## 12. 真实验证

实现完成后，需要在运行中的真实链路验证：

1. 创建一个项目：当前用户为主负责人，至少一个额外人类负责人，多个来源团队，至少一个已选数字员工。
2. 项目详情能看到项目负责人池和已选数字员工池。
3. 进入任务发起，选择刚创建的项目，不选择审核人也能提交需求。
4. 创建出的需求或决策元数据能确认默认处理人是主负责人。
5. 确认项目创建本身没有发起任务。

实现收尾前必须使用项目内 `superteam-completion-check` skill，不能把 mock、组件测试或单元测试表述为真实链路已验证。
