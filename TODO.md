# TODO — 刻意延后的工作

> 约定见 CLAUDE.md「延后工作」条。每条一行:日期 + 要做的事 + 参考文档。
> 完成后删除该行(历史归 git);这里不是任务系统,只是"别忘了"清单。

- [ ] 2026-07-19 飞书剩余联调 7 项(结果结论卡/卡内签署/any-of-N 双人/投影不阻塞/通讯录反查/换绑/重推幂等)——照 `docs/superpowers/manual-test-plans/2026-07-19-feishu-remaining-verification.md` 逐项执行,双人项需先备第二个真人飞书账号
- [ ] 2026-07-19 飞书 App Secret 轮换(明文曾出现在历史对话)——见同上手册末节
- [ ] 2026-07-19 生产桶 CORS 引导命令(不急,后续推进)——`apps/control-plane/cmd/bucket-cors/` 幂等 bootstrap(复用 CP S3 配置,origins 走 env,--check 模式),淘汰 dev 一次性脚本;规则模板与背景见 `docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2
- [ ] 2026-07-19 数字员工执行实例(dei)退役——待并发会话未提交改动落地、工作树干净后做;先外科手术版(readiness 视图改新判据+execution_summary 改取 task_runs 真实落点+封 PUT execution-instance 写路径+清空 dei 数据行),再可选物理拆表版;完整勘察结论/分期计划见 `docs/superpowers/specs/2026-07-19-dei-execution-instance-retirement.md`
- [ ] 2026-07-20 gate 型 `project_task_approval` 卡被人类驳回(rejected)无终结路径——待确认释放修复只覆盖非 gate 等待家族的 rejected→failed;gate-linked approval 的 `ApplyPreDispatchGateDecision` 仍只处理 approved,驳回时任务可能悬在 waiting_human。定位 `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`(`ApplyPreDispatchGateDecision`)+ 背景见 CHANGELOG 46b5050f
- [ ] 2026-07-20 failed 任务无限期点亮员工"异常"——`operational_has_task_failure`(`apps/control-plane/internal/storage/queries/employee_execution.sql` 的 operational_facts CTE)对 failed 任务无时间界,员工在运行总览恒显 error/异常;需加时间窗或以终态确认收敛。判据入口 `apps/control-plane/internal/employee/operational_status.go` + 背景见 CHANGELOG 46b5050f
- [ ] 2026-07-20 项目级 MCP 绑定后端退役——用户确认为错误定义(真实项目 MCP=克隆仓库自带 `.mcp.json`,平台不该管/不展现);配置页「MCP 绑定」tab 已删(提交见 config-page),但后端 `project_mcp_bindings` 表 + `GET/PUT /api/v1/projects/{id}/mcp-bindings`(`capability/handler.go` List/PutProjectMCPBindings)+ runtime 每run一次性投影仍在,需按 retired-features 流程整体退役(迁移 drop 表 + 删端点/handler/契约 + 删 runtime 投影段);先出退役 spec,退役先例参考 migration 085/087
- [ ] 2026-07-20 多人类负责人模型(用户拍板:引入多 owner 角色)——当前宪法为单一 `human_owner` 必绑锚点(决策兜底路由目标),改多 owner 的 blast radius:概览展示全部 owner(名称)+成员增删、`replaceMembers` 强制结果集 ≥1 human owner 护栏、coordinator 决策兜底路由从单目标改多目标、`human_owner_user_id` 单 FK 语义与项目创建校验、CLAUDE.md 协作模型条文更新;需先出设计 spec 再实施,勿 ad-hoc 改
- [ ] 2026-07-20 项目成员名服务端批量补名——config 读路径按成员 id 批量 join users+employees 补 `display_name`,替换前端客户端 join(`listUsers` limit 200 / `listDigitalEmployees` 无分页,规模上量后超出部分成员名静默丢失);project `HTTPHandler`/service 需接 user+employee 读依赖;对齐宪法"名称由服务端读路径批量补名",落点 `internal/project/handler.go` `projectConfigResponseFromDomain`
