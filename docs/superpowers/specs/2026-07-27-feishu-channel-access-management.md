# 飞书通道接入与管理 Spec(通道即平台资源 · 接入闭环 / 可观测 / 绑定治理)

- 日期:2026-07-27
- 状态:**已拍板,待实施**(拍板见 §2;仅 connector 部署归属一项标注为"方向已定待复确认")
- 目标读者:接手实施的独立会话(不共享原始对话上下文,本文自包含)
- 基线:`2026-07-17-feishu-integration-design.md`(P1 消息面已交付:绑定双路径/审批卡/逐条签署/结果通知/外部服务凭据切片);`2026-07-25-human-task-load-budget-and-channel-grading.md`(kind 交互分级已落地)
- 交付性质:管理面补全(CP admin API 收口 + Console UI + 可观测机制)。**不动**卡片交互、词表、判权模型、HumanTask 域。

---

## 1. 背景:消息面是精装修,接入管理面是毛坯

2026-07-26 复盘(真实环境实查)结论:飞书的**消息通知与交互**(卡片渲染、卡内逐条签署、kind 分级、词表护栏、on-behalf-of 判权)是完善的;但**接入与管理**严重缺失。逐条证据:

1. **应用接入是 curl 驱动的**。`feishu_app_configs` 只有裸 API(`POST/GET /api/v1/admin/feishu/app-configs`,`admin_handler.go`),无任何 Console 面;`UpsertAppConfig` 保存时不做连通性自检(不试 tenant_access_token、不探 scope),配错要等 connector 启动失败或发卡失败才暴露。
2. **服务凭据生命周期全手工**。`POST /api/v1/admin/service-tokens`(body `{service_name}`)签发 → 人肉复制进 `FEISHU_CONNECTOR_TOKEN` 环境变量(dev 走 `.scratch/dev-services/feishu-connector.token` 文件)→ 手工重启 connector。无轮换、无管理面、无"当前有效 token 列表"视图。
3. **connector 是平台的盲区(最重)**。CP 不知道 connector 活没活:无注册、无心跳、无版本上报。07-26 实测 connector 对 outbox 轮询 connection refused 断了数小时(CP 重启窗口),平台侧零感知零告警;用户第一感知是"飞书没动静"。对比 runtime-agent(enrollment/心跳/节点管理/限额下发俱全)——同为平台的外部面,待遇完全不对称。
4. **outbox 无运营面**。实查 dev 库:19 条 `failed`(其中 18 条 `code=99991663 Invalid access token`,对应某段 secret/token 失效期)+ 1 条 `skipped_unbound`,无 UI 可见、无重推入口、无死信告警。失败=静默丢通知,丢的恰是"需要人类此刻决策"的卡。重试语义:connector ack `result=failed` 计数,3 次后终态 `failed` 不再消费(`OutboxMaxAttempts`,`outbox.go`)。
5. **绑定只有正向路径**。能绑(通讯录反查 `ContactSync` + OAuth `oauth_handler.go`)、能看(用户管理页状态列),但换绑/解绑无 UI(基线 spec"换绑=删旧建新"有模型无入口);"未绑定的负责人有多少"无视图——直接决定 any-of-N 到达率(`skipped_unbound` 即其后果)。实查 dev 库绑定仅 1 条(oauth)。
6. **多租户是 schema 预留、面为零**。基线 spec 明确 P1"一租一 app 无管理界面"。
7. **飞书侧权限清单无自检**。必需 scope 只活在文档/人脑,缺权限只在运行时以 99991663 等错误码暴露。

为什么重要:飞书是"人类守门"的**到达渠道**(用户原则:飞书必须自足)。守门可靠性现在依赖一条无人看守的通道;接入是客户落地第一公里,现在需要平台工程师全程手扶,不可复制。

---

## 2. 拍板结论(2026-07-26/27 讨论)

1. **通道模型收敛为两层:通道(租户级)+ 身份(用户级);投递目标不建模**。判权/响应/问责全部由 open_id→平台用户的身份绑定承载,与消息发生在私聊或群聊无关;出站寻址从身份推导(合格处理人的 open_id 私聊)。项目级通道绑定被明确否掉("太分散")。
2. **项目群不做**,仅记录触发条件(见 §8 附录):①私聊轰炸成为实测负荷问题;②客户明确要求项目群工作方式。触达再做,做法是项目配置里一个可选 chat_id 字段,不动身份/判权。
3. **不新增菜单**。通道/凭据/可观测 → **系统配置中心新增"消息通道"分区**(`apps/web/src/features/system-config/`);绑定治理留在**用户管理页**(绑定是用户维度数据,已有状态列/同步/OAuth 入口,不得两处管理);告警进**收件箱**(通道断连是待办不是页面)。
4. **分区按"通道注册表"通用形态建模**:通道列表,飞书是第一个条目;将来接钉钉/企微/Slack 是加条目不是加页面。单通道运营面深到装不下时再提升独立页(触达时迁移)。
5. **"人类成员必须绑定"从口头约定变为产品约束**(P2):负责人指派处校验绑定状态给出提示,用户管理页聚合"未绑定负责人"置顶。
6. **connector 部署归属:独立进程 + 轻量注册心跳**,与 runtime 节点同级对待(讨论中提出未被反对;实施前口头复确认即可,不阻塞 P0)。

---

## 3. 现状精确清单(实施前必读;行号会漂,以符号为准)

| 事实 | 位置 |
|---|---|
| connector 入口:`FEISHU_CONNECTOR_TOKEN`/`CONTROL_PLANE_URL` env,bootstrap 拉配置,按 app 起 gateway+poller | `apps/feishu-connector/main.go` |
| ServiceAuth 路由:`/api/v1/connector/{bootstrap,identity,outbox,outbox/{id}/ack,my-projects,...}` | `internal/api/server.go` |
| admin 路由:`/api/v1/admin/feishu/{app-configs,contact-sync,identities}`、`/api/v1/admin/service-tokens`(POST/DELETE) | 同上 |
| OAuth:`/api/v1/auth/feishu/oauth-start`(挂 Console 会话)、`/oauth-callback` | `internal/feishu/oauth_handler.go` |
| 表:`feishu_app_configs`(secret AESGCM 加密)、`user_feishu_identities`(双向 UNIQUE,`bound_via` oauth/contact_sync)、`feishu_outbox`(status: pending/sent/failed/superseded/skipped_unbound;`attempts`/`last_error`/`feishu_message_id`) | `internal/storage` |
| ack 语义:`{result: sent|failed, feishu_message_id?, error?}`,重复 ack 幂等返回 `already_acked`;failed 3 次终态 | `internal/feishu/connector_handler.go` / `outbox.go` |
| Web 已有:`lib/api/feishu.ts`(identities/contact-sync/oauth-start),用户管理页绑定状态列+同步按钮+OAuth 按钮 | `apps/web/src/features/users/index.tsx` |
| admin feishu 端点疑似未进 OpenAPI 契约(web 走手写 api 模块) | 实施时核对 `contracts/control-plane/openapi.yaml`,新增端点一律进契约走 `generate:control-plane` |
| 延后 7 项联调(结果结论卡/卡内签署/any-of-N 双人/投影不阻塞/通讯录反查/换绑/重推幂等) | `TODO.md` + `manual-test-plans/2026-07-19-feishu-remaining-verification.md`;本 spec P1 收编"重推幂等",P2 收编"换绑";其余(尤其双人项)仍等第二真人飞书账号,**不阻塞本 spec** |

---

## 4. P0 接入闭环:把接入变成 Console 操作

### 4.1 系统配置中心「消息通道」分区
- 通道列表(通用形态):每行 = 通道类型(feishu)+ app_id + 状态(active/disabled)+ 健康摘要(P1 补真数据,P0 先占位"未知")。
- app config 编辑:app_id / app_secret(只写不回显,secret 沿用 `capability.AESGCMCredentialSealer`)/ 启停。新增 DELETE/禁用端点若缺则补(现状仅 POST upsert + GET list)。
- **保存时连通性自检(本期核心)**:服务端用提交的凭据真实调飞书——①取 tenant_access_token(验 app_id/secret);②探测必需 scope 清单(以平台实际用到的 API 逐项探测或调权限查询接口),返回结构化结果 `{token_ok, scopes: [{scope, ok}], hint}`。**中文具体提示**("缺少 im:message 发消息权限,请在飞书开放平台为应用开通"),不得只报"配置无效"。自检失败允许保存但标 `unverified` 状态(管理员可能先填后开权限),通道列表显示该状态。
- 必需 scope 清单以代码内注册表维护(单一事实源,注释标注每个 scope 被哪个功能使用),自检与文档都从它出。

### 4.2 service token 面板(同分区内)
- 列表(service_name/签发时间/最后使用时间——最后使用需在 ServiceAuth 中间件补记录)、签发、吊销。
- **轮换引导**:签发新 token 时提示部署侧更新方式(env 名/token 文件路径),旧 token 显式吊销;不做自动下发(connector 无回连管理通道,保持简单)。

### 4.3 契约与权限
- 本期新增/收口的 admin 端点全部进 `contracts/control-plane/openapi.yaml` + `generate:control-plane` + `verify:contracts`。
- 权限沿用 admin authz(`AdminHTTPHandler.authorize` 既有动作模式);自检调用写审计(谁在何时用什么 app_id 试了接入)。

---

## 5. P1 通道可观测:让平台知道通道活着

### 5.1 connector 注册与心跳
- 新端点 `POST /api/v1/connector/heartbeat`(ServiceAuth):body 带版本、每 app 的长连接状态(connected/reconnecting)、最近一次 outbox 轮询成功时间。connector 主循环周期上报(30s,复用其已有的轮询节拍)。
- CP 侧落 `feishu_connector_status`(单行/每 service_name 一行即可,不需要 runtime 级别的节点表):last_heartbeat_at、版本、per-app 状态快照。
- 通道列表健康摘要接真数据:心跳新鲜度(阈值默认 90s,进系统配置 key 注册表)+ ws 状态 + 轮询新鲜度。

### 5.2 断连告警(收件箱)
- CP 看门狗(复用既有看门狗模式):心跳超时 → 给租户 admin/owner 开一张收件箱告警卡(幂等:同一断连事件一张卡,恢复后自动 resolve)。**自指问题明确**:通道挂了不能靠通道告警,Console 收件箱是完整操作面(connector 铁律注释既定分工),卡只进 inbox 不推飞书。
- 告警卡走 HumanTask 既有形态(kind 建议 `channel_down`,layer 平台级——若 kind 注册表仅覆盖项目域,允许先以既有"异常处理"归组的通用形态实现,不强行扩 kind 契约)。

### 5.3 outbox 运营面(消息通道分区内)
- 失败列表:status=failed/skipped_unbound,展示 kind/资源/收件人(补名,不裸 UUID)/attempts/last_error(中文引导+原文,对齐 G8 口径)。
- **幂等重推**(收编延后 7 项之一):failed 行重置为 pending+attempts=0,由 connector 自然消费;卡片有即时置换/card_update 语义,重复投递以 `feishu_message_id` 判重(实施时核对 connector 侧行为,补判重护栏测试)。
- `skipped_unbound` 聚合成"因未绑定而漏达 N 条,涉及用户列表",深链到用户管理页(与 P2 绑定质量视图同源)。

---

## 6. P2 绑定治理与多租户

- **通讯录反查补手机号腿**(07-27 讨论新增;**已提前实现入 main**,同日实施:迁移 `20260726170001_auth_users_mobile` + 双数组反查 + 冲突计数 + 管理员创建/联系方式维护 UI,详见 CHANGELOG 2026-07-27 00:54 条):`batch_get_id` 本就同时支持 `emails`/`mobiles`,现状只接了邮箱(`client.go BatchGetOpenIDsByEmail`),而国内飞书用户手机号必填、邮箱常空,邮箱腿实操命中率低。改动:`auth_users` 加 `mobile` 列(迁移)+ 用户创建/编辑抽屉收手机号 + ContactSync 双数组反查(邮箱、手机号任一命中即绑,双命中冲突时报出不静默)。同步结果面板把"未命中"从静默变成逐用户可见(缺邮箱/缺手机号/通讯录无此人三类原因)。
- **管理员代发绑定链接**(兜底,反查未命中时用):管理员在用户行生成一次性绑定链接(短时效、限定目标用户),对方打开走标准 OAuth 完成绑定——保住"open_id 不手填、授权由本人飞书完成"的安全语义,去掉"必须先登录 Console"的门槛。实现:oauth-start 的带签名 token 变体(state 绑定目标 user_id),回调校验 token 而非 Console 会话。
- **换绑/解绑 UI**(用户管理页,收编延后 7 项之一):换绑=删旧建新(事件留痕,基线既定语义),解绑需确认并提示后果("该用户将收不到飞书卡片");写审计。
- **绑定质量视图**:用户管理页聚合条"未绑定的项目负责人 N 人"置顶+过滤器;负责人指派/项目创建路径校验绑定状态,未绑定给非阻塞警示(不禁止——绑定是可达性问题不是资格问题)。
- **多 app 管理**:通道列表天然支持多行(schema 已按租户→app 建模);connector 已按 configs 多实例循环(`main.go`),补的只是 UI 与自检的多行呈现。
- 通道注册表通用化:类型枚举不在业务核心写死(对齐"Provider 类型不依赖封闭枚举"惯例),新通道=新条目+新 connector 实现。

---

## 7. 验收门禁(真实链路,非 mock)

- **G1(P0)**:Console 消息通道分区用错误 secret 保存 → 自检返回结构化失败+中文缺项提示,通道标 unverified;换正确 secret → token_ok+scope 全绿,通道 active。全程 curl 复核审计事件。
- **G2(P0)**:Console 签发新 service token → 用新 token 启 connector bootstrap 200;吊销旧 token → 旧 token 调 connector 端点 401。
- **G3(P1)**:connector 正常运行时通道卡显示健康(心跳/ws/轮询三新鲜度);`kill` connector → 阈值内通道卡转异常 + 收件箱出现告警卡;重启 connector → 通道卡恢复 + 告警卡自动 resolve(幂等:全程仅一张卡)。
- **G4(P1)**:人为制造一条 failed outbox(如临时置错 secret 发卡)→ 运营面可见失败原因 → 修复后点重推 → 卡真实到达飞书且无重复卡(judge by feishu_message_id/置换语义)。
- **G5(P2)**:换绑到另一 open_id → 旧 open_id 点卡 403/不可归因,新 open_id 正常;解绑后该用户成为 skipped_unbound 并出现在绑定质量聚合里;全程审计留痕。
- **G6(P2)**:第二个 app config 录入 → 通道列表两行各自自检/健康独立;connector 重启后双 app 双长连接(bootstrap 多配置既有能力)。
- 门禁:`verify:contracts`、`verify:control-plane`、`verify:web`;涉及 connector 的跑定向 go test + 真实推送抽检。飞书双人项(any-of-N)继续等第二真人账号,不计入本 spec 门禁。

---

## 8. 附录

### 8.1 项目群(明确不做,留触发条件)
触发条件(满足其一再立项):①结果通知类私聊量成为实测人类负荷问题(决策卡按负荷预算本就稀缺,大概率到不了);②客户明确要求项目群工作方式。届时做法:项目配置可选 `feishu_chat_id`(绑定既有群,验证 bot 在群内),投递策略字段控制"群内只读通知/决策卡进群";身份/判权模型零改动(群内点击仍走 open_id→绑定→资格)。自动建群、群成员同步明确不做,群即建群者自治的可见性边界。

### 8.2 非目标
- 不做钉钉/企微/Slack(形态预留即可);不做绑定码;不做通用开放平台(限流/多 key 自助/开发者门户)。
- 不动卡片交互形态、kind 分级表、词表 fixture、on-behalf-of 判权链。
- 不做 connector 自动更新/托管(独立进程,生命周期由部署方负责;平台只做注册/心跳/告警)。
