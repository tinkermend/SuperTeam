# 飞书集成 Phase 1 实现计划

> **For agentic workers:** 按任务逐个实施,TDD,每任务提交。步骤用 checkbox (`- [ ]`) 跟踪。

**Goal:** 外部服务凭据切片(ServiceAuth + on-behalf-of)落地;飞书身份绑定(通讯录反查+OAuth)可用;决策模型 any-of-N(项目人类成员同等身份,前置剃除 leader/验收人残留);飞书私聊发起需求、审批卡定向推送(分级可操作)、结果只读通知全链真实走通;outbox 三层幂等,飞书永远只是投影。

**Architecture:** 新独立进程 `apps/feishu-connector/`(Go, oapi-sdk-go/v3 长连接,纯翻译层,零业务状态);控制平面新增 `/api/v1/connector/*` 路由组(ServiceAuth)+ outbox 表(与决策创建同事务写入,仓储层单一咽喉 `CreateDecisionRequest`——7 个调用点全走它);判权不扩 OpenFGA(connector 动作全部 on-behalf-of 绑定用户,ActorUser 判权);any-of-N 用查询期集合扩展(EligibleDeciders = active human_user 成员 ∪ human_owner),`TargetUserID` 保留为兜底路由,存量零迁移。

**Tech Stack:** Go (chi + sqlc + Temporal) / Atlas / OpenAPI (Go server 生成, TS 手写) / React + TanStack Router + vitest browser / oapi-sdk-go v3 (larkws)。

**Spec:** `docs/superpowers/specs/2026-07-17-feishu-integration-design.md`(§0 九条已拍板决策不重议)。

**关键设计落点(探查确定):**
- 迁移编号从 **070** 起(当前最大 069;并发会话可能占用——Task 1 先 `ls migrations/*.sql | tail -1` 确认)。
- 残留剃除锚点:`types.go:149-150`(ProjectRoleLeader/Acceptance 常量)、`types.go:415-417`(Leader/AcceptanceUserID 字段家族,1572-1574/1623-1624 同)、`service.go:6936`(角色人类校验)、契约 ProjectRole enum、sqlc project 查询列。
- any-of-N 锚点:签署授权 `service.go:5725`/`5836`(`ActorUserID != TargetUserID && != HumanOwnerUserID`);ResolveDecision `service.go:5369`(本身无目标人校验,handler 层判权);决策创建点 `project_store.go:1138/1235/1330/2040/2163/3543/3753` 全走 `repository.CreateDecisionRequest`;inbox 过滤 `inbox/handler.go:199` + `pg_repository.go`。
- 服务凭据先例:`auth_runtime_tokens`(001 迁移)+ `middleware.RuntimeAuth`(`middleware/auth.go:60`,Bearer+X-Node-ID 双因子)——ServiceAuth 仿此(Bearer+X-Service-Name)。
- secret 加密:`capability.AESGCMCredentialSealer`(`capability/crypto.go:24`,Seal/Open)。
- go.work 只含 `./apps/control-plane`——connector 新模块加入 use 列表;`verify:foundation` 的 Go 步骤需确认覆盖新模块(探查 `scripts/verify-*.mjs` 的 go 命令是按 go.work 还是按目录)。
- dev-services 注册:`scripts/dev-services.sh:46` `SERVICES` 数组 + CMD/WAIT_URL case 分支;feishu-connector **不进默认 all**,单独 `start feishu-connector`。
- 长连接事实(官方):每 app ≤50 连接;集群模式随机分发不广播;at-least-once,`event_id` 必须去重;3 秒内处理完否则重推 → handler 只做去重+入队+ack,业务异步。
- 飞书 app:`cli_a80b4b3fec91d00d` 专属本项目,后台已配好并实测;App Secret 由人类提供,经种子脚本/管理端点写入 `feishu_app_configs`(sealed),**不进 repo/env 文件**。

## Global Constraints

- 根级命令 `corepack pnpm <script>`;Go 定向 `cd apps/control-plane && go test ./internal/...`(connector 同理);Web `corepack pnpm --filter @superteam/web test -- --no-file-parallelism`;禁止 npx。
- 迁移:编号按当前最大顺延;atlas hash + `make -C apps/control-plane migrate-validate`;全中文 COMMENT;遵循 DATABASE_DESIGN.md。
- 契约改动:openapi → `corepack pnpm generate:control-plane` + `verify:contracts`;TS 客户端手写。
- 前端改动前读 DESIGN.md;内部跳转仅 TanStack Link/navigate。
- **project_store.go / coordinator 相关变更必须跑 `go test ./internal/workflow/projectcoordination/ -run TestReplayRealCoordinatorHistory -count=1` 并报结果;禁止事后 GetVersion 围栏。**
- 每任务提交,尾行 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- 工作树纪律:全部工作在 worktree `/Users/tinker/src/singe/SuperTeam-wt-feishu`(分支 feat/feishu-integration-p1);主 checkout 不动;dev-services 由主 checkout 管理,GATE 前协调加载分支码(主 checkout 空闲则临时切分支,E2E 毕切回)。
- connector 铁律:不连 DB、不持业务状态、不自行判权;唯一本地状态=入站表单会话态(内存+TTL)。

---

### Task 1: 前置清理——leader/验收人残留剃除(迁移 070,TDD)

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/070_drop_project_leader_acceptance.sql`(编号先确认)
- Modify: `apps/control-plane/internal/project/types.go`(:149-150 常量、:415-417/:1572-1574/:1623-1624 字段家族)、`service.go:6936` 校验、`pg_repository.go`/`handler.go` 字段搬运、sqlc `project.sql` 列
- Modify: `contracts/control-plane/openapi.yaml`(ProjectRole enum、project schema 两字段)+ codegen
- Modify: `CLAUDE.md` 协作模型句 → "项目人类成员同等身份,human_owner 为必绑锚点与兜底路由目标"
- Modify: atlas.sum

**要点:** `DROP COLUMN projects.leader_user_id, acceptance_user_id`;`project_members.project_role` 注释收敛(结构不动);**勘误:web 并非零引用**(早前只按 Go 命名查漏了 snake_case)——`project-config-page.tsx:434-450` 仍暴露"负责人/验收人用户 ID"两输入框、`:644` 角色过滤、`lib/api/projects.ts` 三处类型,一并剃除(改动前读 DESIGN.md)。

- [x] **Step 1: 失败测试**——现有引用两字段/两常量的测试改期望(创建项目不再接受/返回两字段;角色校验仅剩 owner 分支)。
- [x] **Step 2: RED → 实现 → 全包测试 + migrate-validate + replay 测试**(types 被 coordinator 引用)。
- [x] **Step 3: Commit** — `refactor(project): 剃除leader/验收人残留——成员同等身份 (迁移070)`

---

### Task 2: any-of-N 决策模型(TDD)

**Files:**
- Modify: `apps/control-plane/internal/project/pg_repository.go`(新查询 `IsEligibleDecider(ctx, tenantID, projectID, userID) (bool, error)`:active human_user 成员 ∪ human_owner;sqlc)
- Modify: `apps/control-plane/internal/project/service.go`(:5725/:5836 签署授权、ResolveDecision 路径统一走 `isEligibleDecider`)
- Modify: `apps/control-plane/internal/inbox/`(列表过滤:target_user_id 命中 **或** 项目决策类 item 且请求者 ∈ 该项目合格集合——探查现有查询形态后定 join 还是应用层过滤)
- Test: service/repository/inbox 层

**Interfaces:** `EligibleDeciders` 语义单点实现,后续 outbox 展开(Task 6)复用同一查询的列表版 `ListEligibleDeciders(ctx, tenantID, projectID) ([]uuid.UUID, error)`。

- [x] **Step 1: 失败测试**——成员可签署/批准(原本 403 的用例反转);非成员仍 403;owner 不在成员表仍可(并集兜底);先到先得(A 批后 B 同值 200 幂等/异值 409);inbox 对成员可见非成员不可见。
- [x] **Step 2: RED → 实现 → 全包 + replay 测试。**
- [x] **Step 3: Commit** — `feat(project): 决策any-of-N——合格处理人集合取代单目标人`

---

### Task 3: 迁移 071——飞书四表

**Files:** `071_feishu_integration.sql` + atlas.sum

四表按 spec §9:`feishu_app_configs`(secret sealed)、`user_feishu_identities`(双 UNIQUE)、`auth_service_tokens`(hash 存库仿 auth_runtime_tokens)、`feishu_outbox`(pending 局部索引;status: pending|sent|failed|skipped_unbound|superseded)。全中文 COMMENT。

- [x] **Step 1: 写迁移 → atlas hash + migrate-validate + migrate-up + psql 抽查四表。**
- [x] **Step 2: Commit** — `feat(db): 飞书绑定/服务凭据/outbox四表 (迁移071)`

---

### Task 4: 服务凭据切片——ServiceAuth + connector 路由组骨架(TDD)

**Files:**
- Create: `apps/control-plane/internal/serviceauth/`(token 生成/验证 service + repository;bcrypt 或 sha256 hash 仿 runtime token 探查后一致)
- Modify: `apps/control-plane/internal/api/middleware/auth.go`(`ServiceAuth`:Bearer + `X-Service-Name`;on-behalf-of 解析:`X-On-Behalf-Of` + `X-Feishu-Open-Id` → 服务端反查绑定表核对一致 → 注入 acting user 上下文;无绑定头则纯服务身份)
- Modify: `apps/control-plane/internal/api/server.go`(`/api/v1/connector/*` 路由组挂 ServiceAuth;`GET /connector/bootstrap` 返回解密 app 配置)
- Create: token 签发走管理端点 `POST /api/v1/admin/service-tokens`(ConsoleUserAuth+authz admin 动作,返回明文一次)
- Modify: `contracts/control-plane/openapi.yaml` + codegen

**要点:** on-behalf-of 动作的 authz 一律 `ActorUser`=绑定用户(不扩 FGA);审计 payload 带 `{channel, service, feishu_open_id}`。

- [x] **Step 1: 失败测试**——无 token 401;错 token 401;吊销后 401;on-behalf-of 头与绑定表不一致 403;一致则上下文注入正确;bootstrap 返回 sealed 解密后配置且仅 ServiceAuth 可达。
- [x] **Step 2: RED → 实现 → 全包 + verify:contracts。**
- [x] **Step 3: Commit** — `feat(auth): 外部服务凭据切片——ServiceAuth+on-behalf-of判权`

---

### Task 5: 绑定链路(通讯录反查 + OAuth + Console 入口,TDD)

**Files:**
- Create: `apps/control-plane/internal/feishu/`(飞书 API client:tenant_access_token 管理、batch_get_id、OAuth code 换 token;secret 经 sealer 读 `feishu_app_configs`)
- Modify: project/api:`POST /api/v1/admin/feishu/contact-sync`(按 auth_users email/手机号批量反查写绑定,`bound_via=contact_sync`)、`GET|POST /api/v1/auth/feishu/oauth-callback`(ConsoleUserAuth 会话绑定当前登录用户,`bound_via=oauth`)、`GET /api/v1/connector/identity?open_id=`(未绑定 404)
- Modify: web 用户管理页——"绑定飞书"按钮(跳授权页)+ 管理员"同步飞书绑定"动作 + 绑定状态列(改动前读 DESIGN.md)
- Modify: openapi + codegen + TS 客户端

**要点:** 飞书 HTTP 调用封装在 `internal/feishu` 单包,单测用 httptest 假服务器;真实调用留 GATE。换绑=删旧建新+事件留痕。

- [x] **Step 1: 失败测试**——batch_get_id 命中写绑定/失配跳过;双 UNIQUE 冲突路径;oauth callback 绑定当前会话用户;identity 端点命中/404;web 组件测试(按钮/状态列)。
- [x] **Step 2: RED → 实现 → 全包 + web 定向测试 + verify:contracts。**
- [x] **Step 3: Commit** — `feat(feishu): 身份绑定双路径——通讯录批量反查+OAuth单点绑定`

---

### Task 6: outbox 写入与消费端点(TDD)

**Files:**
- Modify: `apps/control-plane/internal/project/pg_repository.go`(`CreateDecisionRequest` 仓储实现内同事务写 outbox:按 `ListEligibleDeciders` × 绑定表展开收件人,每人一行 `decision_card`;全员未绑定 → 单行 `skipped_unbound`+项目事件;探查该方法当前事务边界,若非事务先包)
- Modify: `ResolveDecisionRequest` 仓储实现(同决策 pending 行标 `superseded`;已 sent 行留给 connector 卡片更新——resolve 后追加一行 `kind=card_update` payload 带终态)
- Modify: demand 终态路径(`recomputeProjectDemandStatusWithQueries` 的 completed/failed/acceptance_pending 迁移点)追加 `result_notice` outbox 行(收件人=全体已绑定人类成员)
- Modify: api/server.go + openapi:`GET /api/v1/connector/outbox?limit=`、`POST /api/v1/connector/outbox/{id}/ack`(body: sent|failed+error+feishu_message_id)
- Test: repository/service 层

- [x] **Step 1: 失败测试**——决策创建产生 N 行(N=已绑定合格人数);零绑定 → skipped_unbound+事件;resolve → pending 变 superseded+新增 card_update;demand completed → result_notice;outbox 拉取只返回 pending;ack 幂等;attempts 递增,3 次失败标 failed。
- [x] **Step 2: RED → 实现 → 全包 + replay 测试**(project_store 触发路径)。
- [x] **Step 3: Commit** — `feat(feishu): 决策/结果outbox——同事务写入+收件人展开+消费端点`

---

### Task 7: connector 进程骨架(长连接+去重+异步队列)

**Files:**
- Create: `apps/feishu-connector/`(go.mod;`main.go`;`internal/gateway/`长连接 client+事件分发;`internal/dedup/` event_id LRU;`internal/cpclient/` 控制平面 API client(service token 从环境变量 `FEISHU_CONNECTOR_TOKEN` 注入——token 本身不是飞书 secret,可环境变量);`internal/session/` 表单会话态(内存+TTL 30min))
- Modify: `go.work`(use 追加)、`scripts/dev-services.sh`(SERVICES 外新增 case:`feishu-connector`,CMD=go run,WAIT 无 HTTP 面改进程存活探测——探查脚本对无端口服务的支持,不支持则加简易健康探测)
- Modify: 根 `package.json` 若 verify 脚本按目录枚举 Go 模块则补(探查 Task 落点第 6 条)

**要点:** 事件 handler ≤3 秒:去重→入 channel→返回;业务 goroutine 池消费。启动:bootstrap 拉 app 配置(重试退避)→ 建长连接。单测:gateway 事件→队列、去重、会话态 TTL(飞书 SDK 侧 mock 接口化)。

- [x] **Step 1: 失败测试 → RED → 实现 → `cd apps/feishu-connector && go test ./...` 全绿;dev-services start/stop/status 冒烟。**
- [x] **Step 2: Commit** — `feat(connector): 飞书connector骨架——长连接+event_id去重+异步消费`

---

### Task 8: connector 入站——引导/提需求表单/SubmitDemand(TDD)

**Files:**
- Create: `apps/feishu-connector/internal/inbound/`(消息路由:未绑定→引导卡;普通文本→功能引导卡;提需求流程:项目选择卡→模式卡→内容→确认→提交→回执)
- Modify: 控制平面 `POST /api/v1/connector/demands`(on-behalf-of SubmitDemand 包装,`SourceType=feishu` 注册)、`GET /api/v1/connector/my-projects`(on-behalf-of 成员项目列表)+ openapi
- Test: connector inbound 状态机(假 cpclient/假飞书 sender);控制平面两端点

- [x] **Step 1: 失败测试**——未绑定引导;表单全流程状态迁移;确认后调 demands 端点参数正确;demand `source_type=feishu`/`submitted_by`=绑定用户;my-projects 只列成员项目。
- [x] **Step 2: RED → 实现 → 双侧全包 + verify:contracts。**
- [x] **Step 3: Commit** — `feat(connector): 私聊显式发起需求——项目选择/模式/确认全流程`

---

### Task 9: connector 出站——卡片渲染分级+回调 resolve(TDD)

**Files:**
- Create: `apps/feishu-connector/internal/outbound/`(outbox 轮询 loop;卡片渲染器按 spec §8.2 分级:plan_review/planning_gap/clarification 可操作,demand_acceptance 富信息+深链,result_notice 只读;超限裁剪:先内容后动作,区块降级为深链)
- Create: `internal/inbound/card_action.go`(卡片回调→`POST /connector/decisions/{id}/resolve` on-behalf-of 操作者→成功更新卡片已处理态/409 更新为已由他人处理)
- Modify: 控制平面 `POST /api/v1/connector/decisions/{decisionId}/resolve` + openapi
- Test: 渲染器(各决策类型 fixture→卡片 JSON 断言,含裁剪);回调翻译;409 路径

- [x] **Step 1: 失败测试 → RED → 实现 → 双侧全包 + verify:contracts。**
- [x] **Step 2: Commit** — `feat(connector): 审批卡分级渲染+回调resolve+卡片状态更新`

---

### Task 10:【GATE】真实链路 E2E(spec §13 七条)

**Files:** 无代码;证据入本文件实施记录。

前置:Task 1-9 合入分支;dev-services 加载分支码(主 checkout 空闲则临时切分支,毕切回);飞书 app secret 经管理端点/种子写入;**用户其他框架长连接确认下线**;两个真人飞书账号(A=成员+绑定,B=非成员/未绑定)。

- [ ] **Step 1 绑定**:contact-sync 批量命中 A(psql 断言 bound_via);OAuth 按钮补绑;双 UNIQUE 生效。
- [ ] **Step 2 发起**:A 私聊全流程提需求 → psql `source_type=feishu`/`submitted_by=A`;Console workflows 可见;B 收引导卡且零对象创建。
- [ ] **Step 3 any-of-N**:双成员项目 plan_review → 两人收卡 → A 批 → resolved+事件 ActorID=A+payload.channel=feishu;B 卡片刷新已处理;B 再点 → 409 语义卡。
- [ ] **Step 4 签署深链**:acceptance_pending → owner 收富信息卡 → 深链 Console 完成签署。
- [ ] **Step 5 结果通知**:demand completed → 成员收只读卡。
- [ ] **Step 6 投影不阻塞**:删 A 绑定重跑决策 → skipped_unbound+事件,Console inbox 照常可处理。
- [ ] **Step 7 幂等**:卡片双击/事件重推 → 决策单次 resolve,无重复事件。FAIL → 停止回评审。记录 → 实施记录 + Commit `docs(plan): 飞书P1 GATE记录`。

---

### Task 11: 收尾——门禁+完成检查

- [ ] **Step 1**: `corepack pnpm verify:control-plane` + connector go test + web 全量串行 + typecheck + build + verify:contracts。
- [ ] **Step 2**: `$superteam-completion-check`;CHANGELOG(`TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`);实施记录;memory 更新;建议用户轮换 App Secret(明文出现过)。
- [ ] **Step 3**: Commit + 汇报(P2 衔接点:Chat 转发/卡内签署评估/状态查询命令;遗留)。

---

## 实施记录

**2026-07-17 Task 1-9 全部落地(9 commits, feat/feishu-integration-p1),Task 10 GATE 待真实飞书环境:**

- Task 1 `15fee4e7` 剃除 leader/验收人(迁移070+全链+CLAUDE.md;勘误:web 配置页原仍暴露两输入框,一并剃除)
- Task 2 `72f859f8` any-of-N(isEligibleDecider 四判权点+inbox 三查询集合扩展+DB 后端可见性测试)
- Task 3 `45cbc7df` 迁移071 四表(atlas podman scratch 全量 replay 过)
- Task 4 `123cab00` 服务凭据切片(ServiceAuth+on-behalf-of 绑定表反查防冒充;判权不扩 OpenFGA)
- Task 5 `eab45496` 绑定双路径(通讯录反查+OAuth 一次性 state+防开放重定向;web 用户页绑定列+同步按钮)
- Task 6 `b9d4ecc4` outbox(决策创建/resolve/需求终态三钩子同事务;收件人展开纯函数;消费端点;DB 生命周期测试)
- Task 7/8/9 `1b3811a4` connector 进程(合并提交——main.go 依赖 inbound/outbound 使分拆无法各自编译):
  长连接骨架/去重/会话态/入站四步提需求流/出站分级卡片/控制平面三业务端点
- 收尾门禁(可自动化部分):verify:control-plane 绿(双 Go 模块)、契约 guard 绿、
  web typecheck 0 错误、build 绿、用户页 13 例绿(全量串行见下一条记录)

**设计偏差(相对计划,均已在提交说明留痕):**
1. Task 6 result_notice 只发 completed/failed;acceptance_pending 由 demand_acceptance 决策卡承载,避免双消息。
2. skipped_unbound 留痕为 outbox 行自身(可查询),未另写 ProjectEvent(避免仓储层事件递归)。
3. clarification 决策卡 P1 深链 Console(卡内回文本留 P2)。
4. 决策卡 payload 富集程度受 outbox 快照字段限制(title/summary/risk);判据清单等更富内容需扩快照,留实施记录待 GATE 后评估。

**Task 10 GATE 前置(需人类提供):** 两个真人飞书账号(A=成员+绑定, B=非成员/未绑定);
App Secret 经 POST /api/v1/admin/feishu/app-configs 写入(cli_a80b4b3fec91d00d);
服务凭据经 POST /api/v1/admin/service-tokens 签发注入 FEISHU_CONNECTOR_TOKEN;
用户其他框架的长连接确认下线;dev-services 加载分支码。
