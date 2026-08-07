# 收件箱 SSE 推送(替代 5s 前端轮询)设计
> 复核状态：全项落地（2026-07-19 18:07）

- 日期: 2026-07-19
- 状态: 已实施(2026-07-19, §3 全项落地 + §4 四项真实 E2E 全 PASS, 实施记录见 CHANGELOG 2026-07-19 18:07 条目; §4-2 外部渠道以 DB 直写等价覆盖并留痕)
- 背景会话结论: 用户反馈收件箱页"每隔几秒刷新一次"。实际是 `apps/web/src/features/inbox/index.tsx:111` 的 `refetchInterval: 5000` 后台轮询,配合 `inbox-shell.tsx:165-167` 每次轮询插入/移除"正在刷新"胶囊造成整个三栏布局跳动,形成"页面强制刷新"观感。轮询是 2026-07-18 review-gate P1.1 跟进有意加的(外部渠道——飞书卡片签署/他人处理——的 resolve 不推送到已打开页面,轮询让待处理项 ≤6s 自动消失),需求本身成立,交付形态要改成推送。

## 1. 目标

1. 收件箱页不再有周期性可感知的"刷新"动作;新事项出现、已处理事项消失均由服务端推送驱动,数秒内自动反映。
2. 覆盖全部写路径:Web 内操作、飞书卡片回调、他人处理、系统催办(team_pending_delete_reminder)——不逐点埋事件,统一从数据面感知变化。
3. 前端轮询降级为低频兜底(流断开窗口期仍能自愈),不再作为主通道。

## 2. 现有资产(设计基座)

- **SSE 服务端模式**: `apps/control-plane/internal/employee/handler.go` `StreamActivity`(行 181)——服务端短间隔游标增量查询,新事件写 `event: activity` 帧,空轮询发 `: keepalive` 注释行保活,查询失败即断流靠客户端 EventSource 自动重连。契约已有先例: `contracts/control-plane/openapi.yaml` `/api/v1/digital-employees/activity/stream`(行 3308,`text/event-stream` response)。
- **SSE 客户端模式**: `apps/web/src/features/run-overview/index.tsx` 行 27-100——原生 `EventSource(url, { withCredentials: true })`,`eventSourceFactory` 注入点供测试,流断开自动重连+低频轮询兜底。
- **变更游标**: `inbox_items.updated_at` 由触发器 `update_inbox_items_updated_at`(迁移 016)维护,任何 upsert/resolve 都会推进;`(updated_at, id)` 可做单调游标,与 activity 流的 cursor 编码方式(`decodeActivityCursor`)同构。
- **收件箱模块**: `apps/control-plane/internal/inbox/`(service.ListItems/GetBadge/ExecuteAction,handler.canReadTeamInbox 团队视图授权)。

## 3. 方案

**推"脏通知"不推数据**。SSE 帧只告知"你可见范围内的收件箱有变化",客户端收到后 invalidate React Query 缓存,数据仍走既有 `GET /api/v1/inbox/items` 拉取。理由:

- 收件箱列表有视图(my/team)+多维筛选,筛选逻辑全在客户端查询参数里;推全量 item 需要在流里复刻 ListItems 的授权+过滤,重复且易漂移。
- 脏通知让授权天然收敛到既有 ListItems 路径(含 canReadTeamInbox),流端只需判"可见范围内是否有行变更",无二次判权面。
- 代价是变更后多一次拉取请求,频次等同真实变更频次,远低于现在的 5s 盲轮询。

### 3.1 服务端

- 新端点 `GET /api/v1/inbox/stream`,鉴权同 ListItems(actor 本人;团队范围变更是否可见复用 canReadTeamInbox 判定一次,流存续期内不重判)。
- 循环模式照抄 StreamActivity:短间隔(复用/对齐 `activityStreamInterval` 配置思路,默认 ~2s)执行游标增量查询;新增 sqlc 查询,形如:

  ```sql
  -- 返回 max(updated_at, id) 游标之后、actor 可见范围内是否有变更行(LIMIT 1 即可,不需要行内容)
  SELECT id, updated_at FROM inbox_items
  WHERE tenant_id = $1
    AND (target_user_id = $2 OR ($3::bool AND target_user_id IS DISTINCT FROM $2))
    AND (updated_at, id) > ($4, $5)
  ORDER BY updated_at, id LIMIT 1;
  ```

  ($3 = 该 actor 是否有团队视图读权。谓词以实施时 ListItems 的实际可见性口径为准,上式是方向示意,不得与 ListItems 漂移——实施时从同一处常量/查询片段派生。)
- 有变更 → 写一帧 `event: inbox-changed`,`data` 携带新游标(客户端不解析也可,仅服务端自用推进);空轮询 → keepalive 注释行,节奏同 StreamActivity。
- 索引核查: `inbox_items` 现有索引是否覆盖 `(tenant_id, updated_at, id)` 增量谓词,不够则补迁移(轻量,单索引)。
- 契约: openapi.yaml 增补该 path(照 activity/stream 条目),走 `generate:control-plane` + `verify:contracts`。

### 3.2 前端

- `features/inbox/index.tsx`: 挂 EventSource(模式照 run-overview,含 `eventSourceFactory` 测试注入点);收到 `inbox-changed` → `invalidateQueries(["inbox-items"])` + `invalidateQueries(["inbox-badge"])`。
- `refetchInterval: 5000` 改为 60s 兜底(流断开窗口自愈),注释同步改写理由。
- **删除"正在刷新"胶囊**(`inbox-shell.tsx:165-167`)——后台静默更新不需要向用户播报;这是本次"刷新感"的直接元凶,无论 SSE 是否上线都不应保留占布局空间的瞬态元素。
- 侧栏 badge(`app-sidebar.tsx:30` 的 30s 轮询): 收件箱页打开时会被上面的 invalidate 顺带刷新;全局层面是否也接流,实施时再定,默认不动(避免每个页面都挂长连接)。

## 4. 验证要求(按 CLAUDE.md,真实端到端为完成条件)

1. 浏览器开收件箱页 → 另一会话(curl 或第二浏览器)执行会产生/解决收件箱项的真实操作 → 页面数秒内自动出现/消失对应项,期间无布局跳动、无"正在刷新"胶囊。
2. 外部渠道路径至少一条真实验证(飞书回调若环境不可用,用 curl 直呼 resolve 类 API 等价覆盖,并在交付记录里说明)。
3. 断流自愈: 杀掉 control-plane 或断开流后恢复,确认 EventSource 重连或 60s 兜底轮询把状态追平。
4. 团队视图: 具/不具团队读权的两个账号各开一条流,确认团队范围变更只推给有权者。

## 5. 已知取舍与开放点

- 服务端仍是"轮询 DB→推流"(与 StreamActivity 同构),不是 LISTEN/NOTIFY 或事件总线。接受理由: 与既有模式一致、覆盖所有写路径、无新基础设施;每条打开的流 ~2s 一次 LIMIT 1 索引查,量级可忽略。若未来并发流数变大,再统一升级两处。
- 团队读权在流建立时判定一次,存续期内权限变更不生效(断线重连后生效)。与现状(页面打开期间权限也不重判)一致,接受。
- resolved 项从列表消失依赖客户端 refetch 时默认 `status=open` 过滤(2026-07-18 已改),流本身不区分变更类型。
