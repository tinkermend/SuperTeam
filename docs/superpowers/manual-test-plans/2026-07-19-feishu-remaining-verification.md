# 飞书集成剩余联调手册(2026-07-19)

> 背景:飞书 P1 快乐路径已真实走通(提需求→计划卡→批准→执行→验收→结果卡)。
> 本轮新增三块能力已上线但部分未经真实验证:卡片信息富化(45ef1476)、
> 卡内逐条签署(2b153840)、结果卡执行结论(a9643af1)。
> 以下各项按序独立执行,做完一项勾一项;任何一步与期望不符,把现象发给会话里的 AI 排查。

## 前置(每次联调前确认)

- `scripts/dev-services.sh status`:temporal / control-plane / web / runtime-agent 均 running;
  `scripts/dev-services.sh start feishu-connector`(不在默认 all 里)。
- connector 日志确认长连接已建立:`tail .scratch/dev-services/logs/feishu-connector.log`
  看到 `connected to wss://...`。
- 确认没有其他框架连着同一个飞书 app(cli_a80b4b3fec91d00d)的长连接(集群模式会分流消息)。
- 绑定关系:Console 用户管理页,admin 应显示已绑定飞书。

## 1. 结果卡执行结论(a9643af1 新增,未真实验证)

手机提一条小需求(计划模式)走完整流程:提需求→批计划→等执行完成→卡内签署→收结果卡。

**期望**:结果卡顶部有「执行结论」区块,内容是数字员工收尾交付的结论文本
(多任务时最后完成的在前,最多 3 条);其后才是「需求内容」摘录与任务统计。

## 2. 卡内逐条签署全流程(2b153840,若上一轮已在卡内签过"通过",只补"不通过"分支)

在第 1 项的验收环节顺带验证:

- 验收卡:每条待签判据一行(原文+验证方式+证据摘录),行下「通过 n」「不通过 n」按钮;
  无"一键全过";深链仅"在 Console 查看完整证据"。
- 点「通过」:toast 显示进度,卡片整卡重渲染(已签 ✅、进度 n/N);全部签完卡片转绿色
  「验收完成」,demand 进 completed。
- 点「不通过」:bot 发文本要理由;回复理由后原卡更新为红色「验收未通过」+确认文本;
  回复空文本被拒;发「取消」放弃。
- **注意**:「不通过」会让需求验收失败并把理由回灌返工,想只试交互就在测试需求上做。

## 3. any-of-N 双人(需要第二个真人飞书账号 B)

前置:B 注册平台账号→加入测试项目为人类成员→OAuth 绑定飞书(Console 用户页「绑定我的飞书」)。

- 双人类成员项目提需求 → 计划卡两人同收。
- A 批准 → A 卡片瞬时变灰保留详情;B 的卡片经 card_update 变「已处理」,
  **应显示处理人是 A 的名字**(本轮新增)。
- B 再点原卡按钮 → toast「已被处理」+卡片置换。
- 非成员 C 转发场景点卡 → 403 语义 toast。

## 4. 投影不阻塞

- psql 删掉 A 的绑定行(`DELETE FROM user_feishu_identities WHERE open_id='...'`)→ 触发一条决策。
- **期望**:`feishu_outbox` 单行 `skipped_unbound`,手机收不到卡,但 Console inbox 照常可批。
- 测完 OAuth 重新绑定。

## 5. 通讯录反查(contact-sync)

前置:飞书后台给 app 开 `contact:user.id:readonly` 权限;测试用户在平台侧配真实邮箱。

- `POST /api/v1/admin/feishu/contact-sync` → 报告命中数字;psql 断言 `bound_via=contact_sync`。
- 已知遗留:失败只回 500,无飞书错误码透出(遗留缺陷#3)。

## 6. 换绑

- OAuth 重复绑定同一飞书账号 → 幂等,不产生新行。
- 用另一个飞书账号 OAuth → 删旧建新,老 open_id 行消失。

## 7. 事件重推幂等

- `scripts/dev-services.sh stop feishu-connector` → 期间制造一条决策 → 重启 connector。
- **期望**:卡片恰好送达一次(outbox 状态机),无重复;卡片双击也全程幂等。

## 已知遗留(不在本手册范围,勿当缺陷报)

- 写回失败无持久重试(韧性家族,任务可能卡 running)——待立项。
- contact-sync 错误可观测性差(只回 500)。
- OAuth state 单副本内存态(多副本部署需外置)。
- 建议正式启用前在飞书后台轮换 App Secret(明文在历史对话出现过),
  轮换后 `POST /api/v1/admin/feishu/app-configs` 重新 upsert 即可。
