# 数字员工执行实例(dei)退役 — 勘察结论与分期计划

> 状态:**已勘察,未实施**(2026-07-19)。用户拍板:先等并发会话大量未提交改动落地、工作树干净后再做物理拆表版,彻底避免共享 checkout 的 git 裹挟。
> 背景:员工卡 Provider 行改身份级 + workbench 判据去 dei 化已落地(见 CHANGELOG 2026-07-19 22:38 / memory `employee-card-identity-provider-fix`)。本文处理其残债。

## 术语

`dei` = 表 `digital_employee_execution_instances` = 员工级 Runtime 绑定。设计上已 designed-off(`run_service.go` 注释:standalone preflight designed-off,派发落点由项目动态解析),但表、数据、代码面仍在。

## 勘察结论(2026-07-19,决定拆表可行性的硬事实)

1. **无任何 FK 指向 dei 表** — `rg "REFERENCES digital_employee_execution_instances"` 零命中。物理 DROP TABLE 不破坏引用完整性。
2. **`task_runs` 已记录真实落点** — `runtime_node_id`(NOT NULL)+`node_id`(NOT NULL),派发时由项目级节点解析写入(`run_service.go` metadata["runtime_node_id"] = preflight.RuntimeNodeID)。这是"员工实际在哪跑"的权威源,dei 的 node 纯属冗余+过时副本。
3. **`execution_instance_id` 列由合成 compat UUID 填充** — `task_runs`/`provider_sessions`/`workspace_file_syncs` 上的该列是裸 UUID(无 FK),派发路径用 `chatCompatibilityExecutionInstanceID`/`projectTaskCompatibilityExecutionInstanceID` 合成确定性 UUID 填入,**不依赖真实 dei 行**。拆表后这些列保留即可(值变成无意义但无害的 UUID)。
4. **活派发路径已 dei-free** — 派发不读 dei;活 provider-session 路径用 `UpsertProviderSessionByExternalID` + `CreateProviderSessionEventIfAbsent`,均不 JOIN dei。
5. **dei 表的读者仅四处**:
   - `digital_employee_runtime_readiness` 视图(迁移 022→047→087 三次重建)——**landmine**:仍 dei 基,判据=员工级绑定+绑定节点在线。协调线程 `project_store.go:runtimeReadyEmployeeIDs` 的**备用腿**(`AreRuntimeReady`,主 reader 未接或 projectID 为空时触发)查它,会把无绑定的新员工**静默判 not-ready 剔出候选池**。视图 COMMENT 声称"与 overview runnable 一致"——本次改后已成假话。
   - `skill_runtime.ListRequiredToolsForNode`(JOIN dei)——runtime 心跳期活调(`runtime/service.go:887`),但按"员工绑定节点→该节点需装哪些技能工具"的旧模型;新员工无 dei 行,**对新员工已恒返回空**,职能被 payload/mcp-config 投递取代(见 memory `project-code-workspace-runtime-affinity`)。
   - `provider_session.sql` 的 `CreateProviderSession` / `CreateProviderSessionEvent`(JOIN dei,要求 status ready/active)——**死查询**,无 Go 调用者。
   - `employee_execution.sql` 总览 items 的 `execution_summary` LEFT JOIN dei——**纯展示**(run-overview 员工地图读 node_id 定座位)。
6. **CRUD 面**:bind/get/list/upsert/updatestatus/delete/abort/softdelete + `AbortProvisionedDigitalEmployee`;契约 `PUT /digital-employees/{id}/execution-instance`(写路径,Web 已无调用者,只 detail 页 GET 读)。

## 结论:目标由"外科手术版"即可完全达成

用户目标 = 新逻辑权威 + 清理遗留数据。达成它**不需要**物理拆表:

- **A. readiness 视图改新判据**(或直接删视图+备用腿)——杀 landmine。新判据镜像本次总览:身份 active/ready + 治理 approved + 租户内存在在线 healthy 节点提供该 provider(参考本次 `available_provider_types` CTE,`employee_execution.sql`)。
- **B. `execution_summary` 改取 `task_runs` 真实落点**——总览 items 查询已有 `latest_runs` CTE,扩展携其 `runtime_node_id`/`node_id`;run-overview 座位改用它。无运行的新员工→无落点(候岗大厅),语义正确。**必须与 D 同批**,否则清空 dei 后地图丢全部落点。
- **C. 封写路径**——删 `PUT execution-instance` 端点 + provisioning preflight(`GetRuntimeProvisioningPreflight[TeamLess]`),杜绝新 dei 行。
- **D. 清空 dei 数据行**——迁移 `DELETE FROM digital_employee_execution_instances`(迁移 051 已有同类先例)。

做完 A–D:dei 表空且写封,不再驱动任何正确性决策;残余 JOIN(skill_runtime/死 provider-session/总览展示)对空表解析为 empty/null——与其对新员工的现有行为一致,无 landmine。

## 物理拆表版(可选卫生工作,树干净后做)

在 A–D 基础上额外:迁移 `DROP VIEW digital_employee_runtime_readiness` + `DROP TABLE digital_employee_execution_instances`;删协调线程备用腿 `AreRuntimeReady` + `digitalEmployeeReadinessAdapter`;删 `skill_runtime.ListRequiredToolsForNode` + runtime `requiredTools` 调用链(或改不依赖 dei);删死 provider-session 两查询;删员工 dei CRUD(Go+sqlc+契约 `DigitalEmployeeExecutionInstance`/`UpsertDigitalEmployeeExecutionInstanceRequest`/端点);总览 items 去 dei LEFT JOIN(execution_summary 完全改 task_runs 源)。**技术上干净**(无 FK、活路径已 dei-free),但 ~30 文件 + DROP TABLE + 全量 sqlc 重生成;**当前工作树有并发会话未提交大改动(employee-create UX/tenant/workflow/systemconfig 30+ 文件)与本次 provider 修复缠在一个 diff**,现在做会裹挟/损坏其工作(见 memory `shared-checkout-concurrent-session-git-safety`)。故延后至树干净。

## 执行顺序(将来)

1. 确认工作树干净(并发会话已提交/落地)。
2. 先 A–D(readiness 新判据 + execution_summary 改源 + 封写 + 清数据),真实 E2E:协调线程对新员工正常选人、run-overview 座位反映真实落点、员工页/详情不回归、一次真实项目任务或 chat 运行 smoke。
3. 再物理拆表版(可选),`make -C apps/control-plane migrate-validate` + 重启实跑迁移 + 复验。
