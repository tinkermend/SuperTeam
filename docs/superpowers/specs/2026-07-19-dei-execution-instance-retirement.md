# 数字员工执行实例(dei)退役 — 勘察结论与实施记录

> 复核状态：外科手术版A-D+物理拆表版均已实施（2026-07-21/2026-07-22）

> 状态:**外科手术版 A–D + 物理拆表版均已实施**(2026-07-21 / 2026-07-22)。
> 背景:员工卡 Provider 行改身份级 + workbench 判据去 dei 化已落地(见 CHANGELOG 2026-07-19 22:38)。本文记录残债处理结论。

## 术语

`dei` = 表 `digital_employee_execution_instances` = 员工级 Runtime 绑定。设计上早已 designed-off(派发落点由项目动态解析)；物理表与读写面已删除。

## 勘察结论(2026-07-19)

1. **无任何 FK 指向 dei 表** — 物理 `DROP TABLE` 不破坏引用完整性。
2. **`task_runs` 已记录真实落点** — `runtime_node_id` + `node_id` 为权威源。
3. **`execution_instance_id` 列由合成 compat UUID 填充** — `task_runs`/`provider_sessions` 等列无 FK，拆表后保留为无害 UUID。
4. **活派发路径已 dei-free** — 不读 dei；活 provider-session 路径不 JOIN dei。
5. **历史读者**(readiness 视图 / skill tools JOIN / 死 provider-session 查询 / overview LEFT JOIN)均已拆除或改源。

## 实施记录

### 外科手术版 A–D(2026-07-21)

- **A** 删除协调 `AreRuntimeReady` 备用腿与 planning/preflight 的 dei 回退；迁移重建 readiness 为身份+治理+租户 provider 判据。
- **B** overview `execution_summary` 改取最近一次 `task_runs` 落点。
- **C** 删除 `PUT /digital-employees/{id}/execution-instance`。
- **D** 迁移 `20260721152932_dei_surgical_retirement.sql`:`DELETE FROM digital_employee_execution_instances`。

### 物理拆表版(2026-07-22)

- 迁移 `20260721155354_drop_dei_execution_instances.sql`:`DROP VIEW digital_employee_runtime_readiness` + `DROP TABLE digital_employee_execution_instances`。
- 删除 dei CRUD/sqlc、死 provider-session JOIN、skill `ListRequiredToolsForNode` 旧 JOIN(现恒空)、GET `/execution-instance` 与 OpenAPI/Web 客户端。
- standalone preflight 改租户最少负载在线节点 + 合成 compat `execution_instance_id`。
- 删除级联审计中的 dei 计数/字段与测试假实现。
- 宪法「已知债」dei 残留条目已删；规范句写入「数字员工」定义。

远程库 version `20260721155354`；`to_regclass` 确认表与视图均不存在。
