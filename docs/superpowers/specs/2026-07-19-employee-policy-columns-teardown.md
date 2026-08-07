# 数字员工个体策略字段下线（第二步 teardown）

> 复核状态：未找到明确实施记录

日期：2026-07-19 · 状态：已实施（迁移 20260719124500，全链真实 E2E PASS）· 前置：第一步（创建页 UI 与提交字段移除）已完成

## 背景与结论

数字员工个体上的 `context_policy` / `approval_policy` 是无消费方的死字段：

- 数据事实：dev 库全部员工两列恒为 `{}`；创建页曾以只读框展示恒 `{}`，无任何编辑路径（该 UI 已随第一步移除）。
- 消费方追查：员工个体 `approval_policy` 零运行期消费——真正生效的审批策略在团队配置与项目层（项目 `approval_policy` 有真实消费链，勿混淆）。`context_policy` 仅剩一个消费点：协调线程规划画像透传 `max_context_classification`（`internal/workflow/projectcoordination/planning_profile.go`），实践恒空。
- 与平台愿景不冲突：「执行时注入上下文切片」是团队/项目/任务层机制，不依赖员工个体静态 JSON。

第一步已让写入路径归零（前端不再提交，后端 nil→列默认 `{}`）；本 teardown 把列与残余引用清干净。

## 拆除清单（087 教训：删列必须全链扫，含手写内联 SQL）

1. **DB**：迁移删除 `digital_employees.context_policy` / `approval_policy` 两列（NOT NULL DEFAULT '{}'，无索引依赖；删列前全库 grep 两列名，重点覆盖不走 sqlc 的手写 SQL）。
2. **后端 employee 模块**：`CreateDigitalEmployeeParams` / `DigitalEmployeeRecord` / handler 请求响应结构 / `createLocalReadyEmployeeFacts` / 投影映射中的两字段；sqlc 查询与重新生成。
3. **模板层**：`digital_employee_templates` 的默认策略与 `default_context_policy_override` / `default_approval_policy_override`（template_handler.go legacy 字段拒收清单同步收敛）。注意：模板 `DefaultApprovalPolicy.min_risk_for_human` 目前还参与创建时风险等级默认推导（service.go `defaultRiskLevelForEmployeeType` 与前端 `applyTypeDefaults` 的 `policy_defaults`）——需先决定风险默认改从何处来（建议直接落模板顶层字段或常量 medium）。
4. **规划画像**：`planning_profile.go` 的 `PlanningContextPolicy` / `max_context_classification` 字段与 source 装配、selection warning 分支。
5. **契约**：`contracts/control-plane/openapi.yaml` 中 CreateDigitalEmployeeRequest / DigitalEmployee 响应等处两字段；`generate:control-plane` + `verify:contracts`。
6. **验证**：真实 E2E 走创建（模板路径 + 空白路径）+ 员工详情/overview 读路径 + 协调线程规划一次真实分派（确认画像装配不因缺字段报错）。

## 明确不动

- 团队配置（tenant_team_configs）与项目层的同名策略字段——有真实消费。
- `policy_defaults`（create-options 响应）里驱动风险等级默认值的部分，除非按 §3 先迁走。
