# TODO — 刻意延后的工作

> 约定见 CLAUDE.md「延后工作」条。每条一行:日期 + 要做的事 + 参考文档。
> 完成后删除该行(历史归 git);这里不是任务系统,只是"别忘了"清单。

- [ ] 2026-07-19 飞书剩余联调 7 项(结果结论卡/卡内签署/any-of-N 双人/投影不阻塞/通讯录反查/换绑/重推幂等)——照 `docs/superpowers/manual-test-plans/2026-07-19-feishu-remaining-verification.md` 逐项执行,双人项需先备第二个真人飞书账号
- [ ] 2026-07-19 飞书 App Secret 轮换(明文曾出现在历史对话)——见同上手册末节
- [ ] 2026-07-19 runtime 写回持久重试(Rust runtime-agent):写回 400/失联时结果落本地持久队列重试,避免结果丢失致任务被恢复为失败——非紧急(CP 侧 attempt 看门狗已证明能兜住 runtime 死亡),属"减少僵尸产生+结果不丢"优化。设计见 `docs/superpowers/specs/2026-07-19-stuck-task-reconciliation-design.md` §3.2
- [ ] 2026-07-19 卡死任务收敛 P2 跨视图一致性统一出口:员工详情页第三套算法(本地 hasActiveRun)归一到 operational_state / overview working 可解释来源项目 / 项目措辞不把任务completed呈现为项目已完成——见同 spec §3.3(对应用户最初观察的"总览working vs项目看似完成")。P1 僵尸看门狗(orphan+attempt 两路)已落地并真实E2E PASS(未提交,工作树)
- [ ] 2026-07-19 生产桶 CORS 引导命令(不急,后续推进)——`apps/control-plane/cmd/bucket-cors/` 幂等 bootstrap(复用 CP S3 配置,origins 走 env,--check 模式),淘汰 dev 一次性脚本;规则模板与背景见 `docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2
