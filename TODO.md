# TODO — 刻意延后的工作

> 约定见 CLAUDE.md「延后工作」条。每条一行:日期 + 要做的事 + 参考文档。
> 完成后删除该行(历史归 git);这里不是任务系统,只是"别忘了"清单。

- [ ] 2026-07-19 飞书剩余联调 7 项(结果结论卡/卡内签署/any-of-N 双人/投影不阻塞/通讯录反查/换绑/重推幂等)——照 `docs/superpowers/manual-test-plans/2026-07-19-feishu-remaining-verification.md` 逐项执行,双人项需先备第二个真人飞书账号
- [ ] 2026-07-19 飞书 App Secret 轮换(明文曾出现在历史对话)——见同上手册末节
- [ ] 2026-07-19 生产桶 CORS 引导命令(不急,后续推进)——`apps/control-plane/cmd/bucket-cors/` 幂等 bootstrap(复用 CP S3 配置,origins 走 env,--check 模式),淘汰 dev 一次性脚本;规则模板与背景见 `docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2
- [ ] 2026-07-19 数字员工执行实例(dei)退役——待并发会话未提交改动落地、工作树干净后做;先外科手术版(readiness 视图改新判据+execution_summary 改取 task_runs 真实落点+封 PUT execution-instance 写路径+清空 dei 数据行),再可选物理拆表版;完整勘察结论/分期计划见 `docs/superpowers/specs/2026-07-19-dei-execution-instance-retirement.md`
- [ ] 2026-07-20 gate 型 `project_task_approval` 卡被人类驳回(rejected)无终结路径——待确认释放修复只覆盖非 gate 等待家族的 rejected→failed;gate-linked approval 的 `ApplyPreDispatchGateDecision` 仍只处理 approved,驳回时任务可能悬在 waiting_human。定位 `apps/control-plane/internal/workflow/projectcoordination/predispatch_gate.go`(`ApplyPreDispatchGateDecision`)+ 背景见 CHANGELOG 46b5050f
- [ ] 2026-07-20 failed 任务无限期点亮员工"异常"——`operational_has_task_failure`(`apps/control-plane/internal/storage/queries/employee_execution.sql` 的 operational_facts CTE)对 failed 任务无时间界,员工在运行总览恒显 error/异常;需加时间窗或以终态确认收敛。判据入口 `apps/control-plane/internal/employee/operational_status.go` + 背景见 CHANGELOG 46b5050f
