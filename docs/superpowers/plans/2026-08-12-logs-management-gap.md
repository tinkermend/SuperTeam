# 日志管理缺口落地

日期：2026-08-12

## 边界（不变）

- `/logs`：控制台操作追溯、登录安全、Runtime 健康、消息投递。
- `/audit` + 对象详情：`audit_events` 业务事实链（不按时间删）、项目事件、执行账本、任务事件、审批。
- 本批**不**把 project_events / execution_ledger / task_events / 审批 / HTTP access log / 成本汇总灌进日志管理。

## 本批做

### P0 写入

1. 新增 `internal/oplog`：写 `web_operation_logs`；从 context 补 IP / UA / username / request_id。
2. 控制台 `CreateAuditEvent` 双写操作日志（白名单 event_type，忽略 `digital_employee_run_*`）。
   - `team_management` → `teams`（`team.skill.*` → `skills`）
   - `digital_employee_management` → `employees`
   - `project_management` → `projects`
   - `system_config` → `system_config`
   - `scenario_template` → `scenario_templates`
3. 员工 create / update profile / update status / config revision 写操作日志。
4. 技能安装失败写 `skills.install` failed；InstallSkill 把 ActorUserID 传给团队绑定。
5. 用户管理操作日志补 IP/UA（context）。
6. 操作日志默认排除 `module=authz`（chip「授权判定」才看判权洪水）。

### P0/P1 读与展现

7. 列表 API 增加 `since`；操作日志增加 `exclude_module`；操作记录返回 `details`。
8. 四 Tab：时间窗 chips（24h/7d/30d/全部）、行详情 Sheet（登录/操作对齐平台事件）、失败行 accent。
9. 操作 chips 改为真实 module；平台事件补来源列、事件类型 chips、provider 筛选。
10. 投递「全部」含 sent/pending；chips 增加已送达/待投递。
11. 模块/动作/事件类型走 `status-labels.ts`。

## 本批不做

- 概览聚合接口、同 IP/操作者/节点关联、CSV 导出、自动化触发 Tab、审计中心租户级全量（操作日志已覆盖控制台变更）。

## 验证

- 定向 Go/Vitest；`generate-sqlc` + `generate-openapi` + `verify:contracts`。
- 真实链路：restart 本 checkout 的 CP+Web，浏览器打开 `/logs`，制造一条控制台变更后在操作日志可见；登录失败可见 UA/详情；投递可筛已送达。
