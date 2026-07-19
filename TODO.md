# TODO — 刻意延后的工作

> 约定见 CLAUDE.md「延后工作」条。每条一行:日期 + 要做的事 + 参考文档。
> 完成后删除该行(历史归 git);这里不是任务系统,只是"别忘了"清单。

- [ ] 2026-07-19 飞书剩余联调 7 项(结果结论卡/卡内签署/any-of-N 双人/投影不阻塞/通讯录反查/换绑/重推幂等)——照 `docs/superpowers/manual-test-plans/2026-07-19-feishu-remaining-verification.md` 逐项执行,双人项需先备第二个真人飞书账号
- [ ] 2026-07-19 飞书 App Secret 轮换(明文曾出现在历史对话)——见同上手册末节
- [ ] 2026-07-19 生产桶 CORS 引导命令(不急,后续推进)——`apps/control-plane/cmd/bucket-cors/` 幂等 bootstrap(复用 CP S3 配置,origins 走 env,--check 模式),淘汰 dev 一次性脚本;规则模板与背景见 `docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2
- [ ] 2026-07-19 数字员工执行实例(dei)退役——待并发会话未提交改动落地、工作树干净后做;先外科手术版(readiness 视图改新判据+execution_summary 改取 task_runs 真实落点+封 PUT execution-instance 写路径+清空 dei 数据行),再可选物理拆表版;完整勘察结论/分期计划见 `docs/superpowers/specs/2026-07-19-dei-execution-instance-retirement.md`
