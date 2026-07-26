# TODO — 刻意延后的工作

> 约定见 CLAUDE.md「延后工作」条。每条一行:日期 + 要做的事 + 参考文档。
> 完成后删除该行(历史归 git);这里不是任务系统,只是"别忘了"清单。

- [ ] 2026-07-26 租户角色与控制台业务模块能力矩阵——当前仅打通「账号→租户成员→console.access」；`member` 对团队/数字员工/技能等租户级列表仍 403（需 admin），侧栏未按权限裁剪。待其他功能完善后补齐：角色可读/可写范围、导航裁剪、无权限空态。参考 `docs/superpowers/specs/2026-07-25-tenant-membership-and-console-access.md` 与 authz `ActionTeamRead`/`ActionSkillRead`/`ActionEmployeeRead`
- [ ] 2026-07-24 P2 planner 判据中文 E2E 验证——提示词已约束 `statement` 必须中文;真实产出依赖 planner(F6 deepseek 不稳),需独占环境稳定规划后抽查新卡判据无英文原文。参考 §6.2 / §12
- [ ] 2026-07-22 项目详情概览「需求→计划→执行→结果」横向管道可视化——概览重构时明确延后；需动计划确认卡约 300 行业务逻辑，独立一轮做。落点 `apps/web/src/features/projects/components/project-operational-detail.tsx`
- [ ] 2026-07-19 飞书剩余联调 7 项(结果结论卡/卡内签署/any-of-N 双人/投影不阻塞/通讯录反查/换绑/重推幂等)——照 `docs/superpowers/manual-test-plans/2026-07-19-feishu-remaining-verification.md` 逐项执行,双人项需先备第二个真人飞书账号
- [ ] 2026-07-19 生产桶 CORS 引导命令(不急,后续推进)——`apps/control-plane/cmd/bucket-cors/` 幂等 bootstrap(复用 CP S3 配置,origins 走 env,--check 模式),淘汰 dev 一次性脚本;规则模板与背景见 `docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md` §2
- [ ] 2026-07-20 项目成员名服务端批量补名——config 读路径按成员 id 批量 join users+employees 补 `display_name`,替换前端客户端 join(`listUsers` limit 200 / `listDigitalEmployees` 无分页,规模上量后超出部分成员名静默丢失);project `HTTPHandler`/service 需接 user+employee 读依赖;对齐宪法"名称由服务端读路径批量补名",落点 `internal/project/handler.go` `projectConfigResponseFromDomain`
- [ ] 2026-07-26 排查 `employees/$employeeId.tsx` 是否也因路由文件额外 export 组件（`EmployeeRouteContent`，为测试可 import）关掉了 TanStack Router 自动代码分割，把员工详情拽进首屏 chunk——团队侧同一写法实测入口 86→229 KB；参考 `docs/superpowers/specs/2026-07-26-team-configuration-console-design.md` §7「踩坑留证」
- [ ] 2026-07-26 团队宪法收尾两项：①`governance_status` 判据仍是 `constitution = '{}'`，应改成「有无生效规则」（`queries/tenant_team_config.sql` 两处 CASE）；②D4 删除既有规则需审批（接权限中心，独立治理层）。参考 `docs/superpowers/specs/2026-07-26-team-configuration-console-design.md` §5.3/§9.2
