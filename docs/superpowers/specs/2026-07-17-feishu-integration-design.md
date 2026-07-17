# 飞书集成（P1）：IM 投影通道——绑定 / 发起 / 审批 / 通知 + 外部服务凭据切片

- 状态：待评审
- 日期：2026-07-17
- 决策来源：2026-07-16/17 与人类负责人的对齐讨论（记录于会话记忆 `feishu-integration-design`）。本文把口头拍板固化为可实施设计。
- 排序依据：意图与验收标准 P1 已合并（main `77293480`），审批/签署闭环已存在——飞书是把这个闭环推到人类手机上的通道，也是外部服务凭据的第一个真实消费者。自动化任务立项排在本文之后。

## 0. 已拍板决策（不再重议）

1. **connector 独立进程**：纯翻译层（飞书事件 ↔ 控制平面 API），不持业务状态；控制平面本体不拆。
2. **纯私聊，不建项目群**：项目锚点靠项目选择卡；审批/通知按人定向推送。
3. **身份绑定**：用户档案存 open_id，值不手填——通讯录 API 批量反查初始化 + Console OAuth 绑定按钮；绑定码兜底不做。
4. **any-of-N 审批**：审批卡同推项目全部人类成员，任意一人处理即生效（先到先得）。项目人类成员**同等身份**（leader/验收人角色剃除，见 §6.0），human_owner 保留为必绑锚点与兜底路由目标。
5. **P1 范围**：绑定 + 发起需求 + 审批卡 + 结果通知；**Chat 对话转发是 P2**。
6. **审批卡信息富集**：能塞尽塞；装不下降级为摘要 + 深链 Console。
7. **外部凭据切片并入本 spec**（§4），不单独立项。
8. **飞书是投影，不是事实源**：Console 永远完整可操作；推送失败不阻塞任何业务流。
9. **App 专属**：`cli_a80b4b3fec91d00d`（应用名"AI机器人"）专属 SuperTeam，后台配置（bot/长连接/权限/OAuth）已完成并实测收发过。联调前确认用户其他框架的长连接已下线。

## 1. 问题

1. 审批是全系统人类吞吐瓶颈：决策（plan_review / demand_acceptance / planning_gap / clarification）只能登 Console 处理，人类不在电脑前时整条协调链停等。
2. 发起需求同样只有 Console 入口。
3. 平台没有任何面向外部系统的机器凭据：所有业务路由挂 `ConsoleUserAuth`（session cookie，`middleware/auth.go:43`），Runtime Bearer token 只服务 `/api/v1/runtime/*`。任何外部通道（飞书、未来的自动化触发器、第三方系统）都无法合法调用。
4. 决策模型是单目标人（`DecisionRequest.TargetUserID`，创建点一律 `HumanOwnerUserID`，service.go:3112/4576/5061）：owner 休假即全项目决策停摆，与"项目人类成员同等身份"的产品共识不符。

## 2. 目标与非目标

**目标（P1）**

1. 外部服务凭据最小切片：connector 以服务身份认证 + 以绑定用户名义（on-behalf-of）判权与留痕。
2. 飞书身份绑定：`open_id ↔ auth_user`，通讯录批量反查 + OAuth 单点绑定双路径。
3. 决策模型 any-of-N：合格处理人从单 TargetUserID 扩为项目 active 人类成员集合（含前置清理：leader/验收人残留剃除）。
4. 入站：私聊显式发起需求（项目选择卡 → 模式选择 → SubmitDemand），普通消息不隐式创建任何工单。
5. 出站：决策创建 → 审批卡定向推送（可操作卡按决策类型分级，§8.2）；需求终态/停靠 → 只读结果卡。
6. 可靠性底线：event_id 幂等去重、3 秒 ack 异步处理、outbox 有限重试、全链审计事件。

**非目标（P1 明确不做）**

- Chat 对话转发（流式卡片/多轮上下文）——P2。
- 项目群（群生命周期/成员同步）——需求出现再立项。
- 判据签署在卡片内闭环——P1 卡内只展示富证据摘要 + 深链 Console 签署（保住"签署控件紧邻证据"的防橡皮图章设计）；P2 视证据摘要在卡片上的呈现质量再议。
- 多飞书企业租户的完整管理 UI——schema 按"租户→飞书 app 配置"建模（§9.1），但 P1 只配一租户一 app，无管理界面。
- 通用 API 开放平台（限流/多 key 自助管理/开发者门户）——出现第二个外部调用方再扩。
- 绑定码流程（bot 私聊发码）——OAuth + 通讯录反查已覆盖，不做。

## 3. 架构

### 3.1 进程形态

```
飞书开放平台 ⇄ (WebSocket 长连接, oapi-sdk-go/v3 larkws) ⇄ feishu-connector (新独立进程, Go)
                                                            ⇅ (HTTP, service token + on-behalf-of)
                                                          Control Plane
```

- 新目录 `apps/feishu-connector/`（Go，复用 control-plane 的 Go 工具链；`scripts/dev-services.sh` 增加 `feishu-connector` 服务项，不进默认 `all`，按需启动）。
- connector 职责：维护长连接、事件去重、卡片渲染、飞书 API 调用、轮询 outbox、把入站动作翻译成控制平面 API 调用。**禁止**：直接连数据库、持久化业务状态、自行判权。
- 唯一本地状态：入站多轮表单的会话态（选项目→选模式→填内容）放进程内存，带 TTL；进程重启丢失即重来——这是"不持业务状态"的刻意边界，不做持久化。

### 3.2 长连接事实（已核实，官方口径）

- 每 app 最多 50 条连接；**集群模式随机分发，不广播**——connector 多副本天然负载均衡，P1 单副本即可，HA 需求出现时直接加副本，无代码改动。
- 投递语义 **at-least-once**：全链路超时会触发重发，必须按 `event_id` 幂等去重（connector 内存 LRU + 控制平面侧业务幂等双保险）。
- **3 秒内必须处理完成且不抛异常**，否则触发重推：事件 handler 只做"去重 + 入本地队列 + ack"，业务处理异步。

### 3.3 投影原则（可靠性底线）

- 出站：控制平面写 outbox（§9.3），connector 轮询消费;推送失败重试有限次（3 次退避）后标 `failed` + 项目事件留痕，**决策在 Console inbox 照常存在可处理**。
- 入站：所有动作最终落到既有业务端点（SubmitDemand / ResolveDecision），复用其幂等与校验;卡片重复点击由决策 resolve 的既有幂等语义兜底（`StatusSnapshot == req.Decision` 返回 200，service.go:5415）。

## 4. 外部服务凭据切片（第一消费者：feishu-connector）

### 4.1 认证（authn）：服务凭据

- 新表 `auth_service_tokens`（§9.2），仿 `auth_runtime_tokens` 先例（001 迁移）：token hash 存库、可吊销、可轮换。
- 新中间件 `ServiceAuth`：`Authorization: Bearer <token>` + `X-Service-Name: feishu-connector`;验证通过后注入 `ServiceIdentity` 上下文。
- connector 专用路由组 `/api/v1/connector/*`（§10），全部挂 `ServiceAuth`——不给服务凭据开放 Console 路由面，API 面最小化。

### 4.2 判权（authz）：on-behalf-of，不扩 OpenFGA 模型

- **关键设计**：connector 的每个业务动作都以**绑定用户**为行为人。请求头 `X-On-Behalf-Of: <auth_user_id>`，控制平面校验该用户存在、active、且飞书绑定表中的 open_id 与请求声明一致（connector 传 `X-Feishu-Open-Id`，服务端反查绑定表核对——防 connector 被攻破后任意冒充）。
- 判权仍走既有决策点：`authz.Check(Actor{Type: ActorUser, ID: <绑定用户>})`——OpenFGA mapping 只支持 user actor（openfga_mapping.go:31），**不需要扩模型**;`authz.ActorService` 常量已存在，本期只用于审计标注，不进 FGA。
- 无绑定用户的动作（如 outbox 轮询、通讯录反查）以服务身份执行，路由面天然限定其能力。

### 4.3 审计：双主体

- 入站动作产生的项目事件在 payload 增加 `{"channel": "feishu", "service": "feishu-connector", "feishu_open_id": "..."}`;ActorType/ActorID 仍是行为人（human_user/绑定用户）——查"张三批了什么"和查"飞书通道进来了什么"两个维度都可回答。

## 5. 身份绑定

### 5.1 数据（§9.1 `user_feishu_identities`）

`auth_user_id ↔ (feishu_app_id, open_id)`，附 union_id（多企业兼容预留）、绑定方式、绑定时间。UNIQUE 双向：一个 open_id 只能绑一个用户，一个用户在一个 app 下只有一条绑定。换绑=删旧建新（事件留痕）。

### 5.2 两条绑定路径

1. **通讯录批量反查（管理员初始化）**：Console 用户管理新增"同步飞书绑定"动作 → 控制平面按 auth_users 的 email/手机号调 `batch_get_id` → 命中即写绑定（`bound_via=contact_sync`）。失配的留空，走路径 2。
2. **OAuth 单点绑定**：用户管理页"绑定飞书"按钮 → 飞书授权页（浏览器重定向，内网可用）→ 回调换 user_access_token → 取 open_id 写绑定（`bound_via=oauth`）。回调落在控制平面（`/api/v1/auth/feishu/oauth-callback`，挂 ConsoleUserAuth 会话——绑定的是当前登录用户，天然防绑错人）。

### 5.3 凭据存储

- 飞书 app 配置新表 `feishu_app_configs`（§9.1）：app_id 明文、app_secret 经 `capability.AESGCMCredentialSealer` 加密（复用既有 sealer 与主密钥机制）;connector 启动时经 `/api/v1/connector/bootstrap` 拉取解密后配置（仅 ServiceAuth 可达）。secret 不进环境变量、不进 repo。

## 6. 决策模型：any-of-N

### 6.0 前置清理：leader/验收人残留剃除（独立提交，先行）

产品层已放弃、web 零引用、后端休眠的残留，与本节改动同片代码，先剃干净：

- 迁移：`DROP COLUMN projects.leader_user_id, projects.acceptance_user_id`;`project_members.project_role` 列注释收敛（枚举应用层注册，DB 结构不动）。
- 代码：两字段全链搬运（sqlc/repository/handler/types/契约）、`ProjectRoleLeader`/`ProjectRoleAcceptance` 常量、service.go:6936 校验中对应分支。
- CLAUDE.md 协作模型句改为："项目人类成员同等身份，human_owner 为必绑锚点与兜底路由目标"。
- `generate:control-plane` + 全量测试 + **replay 测试**（若触及 coordinator 代码）。

### 6.1 合格处理人集合

- 定义：`EligibleDeciders(project) = { active project_members WHERE principal_type='human_user' } ∪ { human_owner }`（owner 可能不在成员表，并集兜底）。
- `DecisionRequest.TargetUserID` **保留不删**：语义降级为"主要目标/兜底路由"（仍=human_owner），存量数据零迁移。
- 授权检查变更：判据签署（service.go:5725/5836 的 `ActorUserID != TargetUserID && != HumanOwnerUserID`）与 ResolveDecision 路径统一改为 `isEligibleDecider(ctx, tenantID, projectID, actorUserID)`。
- 并发：先到先得。第二人提交时决策已 resolved → 既有幂等分支（同值 200 / 异值 `ErrInvalidProject`→409），飞书侧收到 409 即刷新卡片为"已由某某处理"。
- inbox 投影：decision inbox item 对合格集合内所有绑定用户可见（现状按 TargetUserID 过滤的查询扩为集合;Console 行为同步受益）。

## 7. 入站：私聊发起需求

### 7.1 显式意图规则

- 普通文本消息 → 回引导卡（"我能帮你：提需求 / 查项目状态（P2）"），**不创建任何业务对象**。
- "提需求"按钮 → 项目选择卡（调 `/api/v1/connector/my-projects`，on-behalf-of 列出该用户为成员的 active 项目）→ 模式选择卡（plan/loop，缺省 plan）→ 用户发标题与内容（一条消息，首行为题）→ 确认卡 → `SubmitDemand`（`SubmittedByUserID`=绑定用户，`SourceType=feishu`）→ 回执卡（含 demand 深链）。
- 未绑定用户来消息 → 引导卡：请到 Console 用户管理绑定飞书。

### 7.2 SourceType

`DemandSourceManual` 之外注册新值 `feishu`（demand source 本就是应用层注册的开放枚举）——报表与审计可分辨通道。

## 8. 出站：审批卡与结果通知

### 8.1 触发与投递

- 决策创建/需求终态的既有落库路径追加 outbox 行（§9.3;与业务写同事务，保证不丢）。
- connector 轮询 `/api/v1/connector/outbox?limit=N`（长轮询可选）→ 渲染卡片 → 飞书 API 推送 → `POST /outbox/{id}/ack`（成功/失败+原因）。
- 收件人展开在控制平面完成：写 outbox 时按 §6.1 集合 × 绑定表展开为"每绑定用户一行"，未绑定的成员静默跳过（若全员未绑定 → outbox 行直接标 `skipped_unbound` + 项目事件留痕）。

### 8.2 卡片分级（P1 卡内可操作范围）

| 决策类型 | 卡内动作 | 卡内容（能塞尽塞） |
|---|---|---|
| plan_review | 批准 / 请求修改（带理由输入） | 需求标题、计划摘要、任务列表、**判据清单（method/severity 徽标语义文字化）**、预算、风险标志、歧义黄标条目 |
| planning_gap | restaffed / exempted / rejected（带理由） | 诊断、缺口角色、候选补员 |
| clarification | 直接回文本 | 歧义描述、上下文摘录 |
| demand_acceptance（判据签署） | **仅深链 Console** | 判据逐条 verdict 状态、执行结论摘要、attestation 证据摘要——信息给足，动作去 Console |
| 结果通知（completed/failed/acceptance_pending） | 无（只读） | 终态、判据满足概览、深链 |

- 卡片超飞书大小上限 → 逐区块裁剪为摘要 + 深链（裁剪规则实现期定，原则先内容后动作）。
- 卡片动作回调（`card.action.trigger`，同走长连接）带 `decision_request_id` + 操作者 open_id → connector 翻译为 ResolveDecision（on-behalf-of 操作者）→ 按结果更新卡片（成功=已处理态;409=已由他人处理态）。

## 9. 数据模型（迁移 0NN，编号实施时按当前最大顺延确认）

### 9.1 绑定与配置

```sql
CREATE TABLE feishu_app_configs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    app_id VARCHAR(64) NOT NULL,
    app_secret_sealed TEXT NOT NULL,          -- AESGCMCredentialSealer
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_feishu_app_configs_tenant_app UNIQUE (tenant_id, app_id)
);

CREATE TABLE user_feishu_identities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    auth_user_id UUID NOT NULL,
    feishu_app_config_id UUID NOT NULL,
    open_id VARCHAR(128) NOT NULL,
    union_id VARCHAR(128),
    bound_via VARCHAR(32) NOT NULL,           -- contact_sync | oauth
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_feishu_identity_open UNIQUE (feishu_app_config_id, open_id),
    CONSTRAINT uq_feishu_identity_user UNIQUE (feishu_app_config_id, auth_user_id)
);
```

### 9.2 服务凭据

```sql
CREATE TABLE auth_service_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    service_name VARCHAR(64) NOT NULL,        -- feishu-connector
    token_hash VARCHAR(255) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    last_used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);
```

### 9.3 出站 outbox

```sql
CREATE TABLE feishu_outbox (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID,
    kind VARCHAR(64) NOT NULL,                -- decision_card | result_notice
    resource_type VARCHAR(64) NOT NULL, resource_id UUID NOT NULL,
    recipient_user_id UUID NOT NULL,
    recipient_open_id VARCHAR(128) NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,   -- 渲染所需快照
    status VARCHAR(32) NOT NULL DEFAULT 'pending',-- pending|sent|failed|skipped_unbound|superseded
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT,
    feishu_message_id VARCHAR(128),               -- 回填, 用于后续更新卡片
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_feishu_outbox_pending ON feishu_outbox(tenant_id, status, created_at) WHERE status = 'pending';
```

- 决策 resolved → 同决策未消费的 outbox 行标 `superseded`，已发送的按 `feishu_message_id` 更新卡片为已处理态。
- 全中文 COMMENT 逐表逐列（实施时补全，遵循 DATABASE_DESIGN.md）。

## 10. 契约

`contracts/control-plane/openapi.yaml` 新增（走 `generate:control-plane` + `verify:contracts`）：

- `POST /api/v1/connector/bootstrap`（拉 app 配置）
- `GET /api/v1/connector/outbox` + `POST /api/v1/connector/outbox/{id}/ack`
- `GET /api/v1/connector/my-projects`（on-behalf-of）
- `POST /api/v1/connector/demands`（on-behalf-of SubmitDemand 包装，收敛 connector 可传字段）
- `POST /api/v1/connector/decisions/{decisionId}/resolve`（on-behalf-of）
- `GET /api/v1/connector/identity`（open_id → 绑定用户，未绑定 404）
- Console 侧：`POST /api/v1/auth/feishu/oauth-callback`、`POST /api/v1/admin/feishu/contact-sync`
- connector 自身无对外 HTTP 面（纯出站连接），不需要 `contracts/feishu-connector`。

## 11. 安全与运维

1. secret 全链加密存储（§5.3）;用户已被建议在正式启用前于飞书后台轮换一次 App Secret（已在明文聊天中出现过）。
2. on-behalf-of 防冒充双校验（§4.2：open_id ↔ user 绑定表反查）。
3. 卡片回调不带业务权限语义——所有权限判定在控制平面按操作者身份重新判权，connector 的翻译层不可信任也不需要被信任。
4. event_id 去重 + 决策幂等 + outbox 状态机，三层幂等。
5. `dev-services.sh` 增 `feishu-connector`;联调前确认其他框架长连接已下线（集群模式会分流消息——E2E 消息偶发丢失先查这一点）。

## 12. 分期

- **P1**：本文全部目标（§2）。
- **P2**：Chat 对话转发（流式卡片）、判据签署卡内闭环评估、项目状态查询命令、多飞书企业管理 UI、项目群（若需求出现）。
- **P3**：其他 IM 通道抽象（钉钉/企微——connector 分层时预留 channel 接口但 P1 不抽象，两个实现之前抽象是过度设计）。

## 13. E2E 验收判据（GATE，真实链路，无 mock）

前置：两个真人飞书账号（A=项目成员+绑定，B=非成员或未绑定）;dev 环境全栈 + connector 启动;其他框架长连接已下线。

1. **绑定**：通讯录反查批量命中 A;OAuth 按钮对失配账号完成绑定;绑定表 psql 断言双 UNIQUE 生效。
2. **发起**：A 私聊走完 提需求→选项目→选模式→内容→回执;psql 断言 demand `source_type=feishu`、`submitted_by`=A;Console workflows 页可见同一 demand。B（未绑定）收到引导卡且零业务对象创建。
3. **审批 any-of-N**：造双人类成员项目，plan_review 决策创建 → 两人都收到卡 → A 批准 → 决策 resolved、事件 ActorID=A、payload 含 channel=feishu;B 的卡片刷新为已处理;B 再点 → 409 语义卡片。
4. **判据签署深链**：demand 到 acceptance_pending → owner 收到富信息卡（判据+verdict+证据摘要）→ 深链进 Console 签署页完成签署。
5. **结果通知**：demand completed → 成员收到只读结果卡。
6. **投影不阻塞**：把 A 的绑定删掉重跑决策 → outbox 标 skipped_unbound + 事件留痕，Console inbox 照常可处理。
7. **幂等**：同一卡片按钮双击/飞书重推同一事件 → 决策只 resolve 一次，无重复事件。

## 14. 风险与待决

1. **卡片信息密度 vs 大小上限**：plan_review 富卡在大计划下必然超限，裁剪规则（哪些区块先降级为深链）实施期做卡片原型时定。
2. **connector 会话态丢失**（进程重启中断多轮表单）：接受，重来成本一条消息;不做持久化。
3. **飞书 API 限频**：P1 用户量小不设防;outbox 消费天然串行是隐性限流,真撞限频再加令牌桶。
4. **决策可见性扩大**（any-of-N 使 inbox 对全体人类成员可见）：这是产品意图的直接后果,但存量项目若有"observer 型"人类成员会突然收到审批卡——实施时核对存量数据,必要时按 role 过滤 observer(与"同等身份"共识冲突时回人类拍板)。
