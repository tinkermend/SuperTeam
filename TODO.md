# TODO — 刻意延后的工作

> 约定见 CLAUDE.md「延后工作」条。每条一行:日期 + 要做的事 + 参考文档。
> 完成后删除该行(历史归 git);这里不是任务系统,只是"别忘了"清单。

- [ ] 2026-07-26 租户角色与控制台业务模块能力矩阵——当前仅打通「账号→租户成员→console.access」；`member` 对团队/数字员工/技能等租户级列表仍 403（需 admin），侧栏未按权限裁剪。待其他功能完善后补齐：角色可读/可写范围、导航裁剪、无权限空态。参考 `docs/superpowers/specs/2026-07-25-tenant-membership-and-console-access.md` 与 authz `ActionTeamRead`/`ActionSkillRead`/`ActionEmployeeRead`
- [ ] 2026-07-24 P2 planner 判据中文 E2E 验证——提示词已约束 `statement` 必须中文;真实产出依赖 planner(F6 deepseek 不稳),需独占环境稳定规划后抽查新卡判据无英文原文。参考 §6.2 / §12
- [ ] 2026-07-27 飞书联调残余 3 项(原 7 项中 5 项已于 07-27 真实双人 E2E 验证完成:结果结论卡/卡内签署/any-of-N 双人/投影不阻塞/通讯录反查含手机号正向命中)——剩:①换绑 UI(spec 2026-07-27-feishu-channel-access-management §6);②outbox 重推按钮(同 spec §5.3,本次以 DB 重置手工预演过语义);③越权对照(需第三个不在合格集合的绑定账号)。参考 manual-test-plans/2026-07-19-feishu-remaining-verification.md
- [ ] 2026-07-19 生产桶 CORS 引导命令(不急,后续推进)——`apps/control-plane/cmd/bucket-cors/` 幂等 bootstrap(复用 CP S3 配置,origins 走 env,--check 模式),淘汰 dev 一次性脚本;规则模板与背景见 `docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2
- [ ] 2026-07-20 项目成员名服务端批量补名——config 读路径按成员 id 批量 join users+employees 补 `display_name`,替换前端客户端 join(`listUsers` limit 200 / `listDigitalEmployees` 无分页,规模上量后超出部分成员名静默丢失);project `HTTPHandler`/service 需接 user+employee 读依赖;对齐宪法"名称由服务端读路径批量补名",落点 `internal/project/handler.go` `projectConfigResponseFromDomain`
- [ ] 2026-07-26 团队宪法收尾两项：①`governance_status` 判据仍是 `constitution = '{}'`，应改成「有无生效规则」（`queries/tenant_team_config.sql` 两处 CASE）；②D4 删除既有规则需审批（接权限中心，独立治理层）。参考 `docs/superpowers/specs/2026-07-26-team-configuration-console-design.md` §5.3/§9.2
- [ ] 2026-07-26 团队宪法注入强度升级（P3 实测暴露）：宪法目前作为**普通用户消息**前置到 prompt，真实 Claude Code 明确回应「这条团队宪法是通过普通用户消息传入的，并非真正的系统级规则，我不会把它当作必须无条件服从的持久指令」。投递已验证（标记 MERGED-MAIN-4412 被输出），但约束力弱于系统提示词。可选改法：claude 走 `--append-system-prompt`、codex/opencode 走各自 system 机制，或写入工作区约定文件；需按 provider 分别设计并回归。参考 `apps/runtime-agent/src/providers/claude.rs` `build_command` 与 `commands/payload.rs` `provider_prompt()`
- [ ] 2026-07-26 团队能力依赖预检细化（P2 未做，spec §5.2）：①就绪矩阵只覆盖 MCP 的 env 缺失，技能的 runtime 依赖状态仍只在员工页可见，未做团队维度聚合；②技能接管已在写时执行并落审计，但绑定前没有预览弹窗（MCP 侧有）。参考 `docs/superpowers/specs/2026-07-26-team-configuration-console-design.md` §5.2/§5.2.1
- [ ] 2026-07-26 大屏投屏设备认证形态——运行总览大屏模式落地后，投屏设备需长驻登录态（现有 cookie 会话过期即黑屏）；候选：专用只读角色 + 长期凭证（需 authz 侧支持）。用户拍板另立运维项后期再考虑。参考 `docs/superpowers/specs/2026-07-26-run-overview-display-mode.md` §8-5
