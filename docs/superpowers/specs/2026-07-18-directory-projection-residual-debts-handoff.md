# 目录与能力投影修订 残债立项交接（run 永滞看门狗 + 项目 MCP 绑定 Web 页等）

> 日期：2026-07-18
> 状态：§1(runtime 层)/§3/§4 已完结入 main（2026-07-18, 8bc8304f, GATE 三项真实 E2E PASS,附录见 CHANGELOG 同日条目）;§1 第 2 层(CP 超时看门狗)已完结入 main（2026-07-18, 2dd98376, 直插 stale 行 E2E PASS）;§2(Web 页)已完结入 main（2026-07-18, 05567dec, 浏览器 E2E PASS,经子代理实现+主会话评审）;§5-§7 仍为观察/触发式——**至此可开工残债全部还清**
> 背景：`2026-07-17-directory-capability-projection-revision-design.md` 三分期（P1 稳定缓存与投影 / P3 清理闭环 / P2 治理增强）已全部合并 main 并经真实 E2E（GATE 记录见 CHANGELOG 2026-07-17 三条同名条目）。过程中揪修既有缺陷 4 个（opencode stdin 永挂、opencode 事件 schema 漂移、相对家目录软链悬空、worktree 恒检出 clone 旧 main），另沉淀下列残债。本文逐项立项，按优先级排列。
> 相关 spec：目录与能力投影修订（同目录 2026-07-17）；调度韧性缺陷家族（散见 handoff-loop / scenario-template-p2 记录，本文 §1 是家族新例）。

---

## 1.【承重·调度韧性家族新例】provider 早退/零事件时 run 永滞 dispatching，无看门狗

### 现象（GATE 现场三次复现）
opencode 事件 schema 漂移期间（修复前）：provider 进程正常退出（exit 0）但 runtime 解析出 **0 个事件** → run 停在 `dispatching` **永不转移**——无 started、无 failed、无任何回写；控制平面日志该命令 id 零出现。员工被活跃 run 占位，后续派发 409，只能靠 CP 侧 stale reaper（`isStalePreConfirmationRun`，分钟级窗口）兜底回收。schema 漂移已修，但"零事件流结束"这一类故障（未来任何 provider 输出格式漂移/包装脚本吞输出/崩溃前零输出）仍会复现同样的永滞。

### 根因
`drain_provider_events`（`apps/runtime-agent/src/commands/executor.rs`）尾部只有 `finish_successful_stream()` 返回 Some（观测到过 TurnCompleted）才回写 complete；流结束但从未出现 TurnCompleted/TurnError 时直接 `Ok(())` 返回——**runtime 没有"流结束但无终局事件"的语义**，既不 fail 本地 run，也不回写失败。

### 建议修法（两层）
1. **runtime 层（小改，先做）**：drain 尾部当流已结束、`terminal_writeback` 从未观测到终局事件且 run 未被取消时，视为 provider 异常早退：`runs.finish_failed(run_id, "provider exited without a terminal event")` + `writeback.fail(...)`（走既有失败路径，raw log 指针照带）。注意与 `stream_child_events` 既有的 provider-exit-error 事件路径（非零退出码会生成事件）互补——本修法专堵"exit 0 + 零事件"。
2. **CP 层（家族级，可另拆）**：dispatching/queued 态 run 超时看门狗（心跳周期检查，超 N 分钟无 started 回执 → failed + 事件），覆盖 runtime 进程整个死掉、命令根本没送达等 runtime 层修法够不着的场景。与调度韧性家族其他例（coordinator 死亡告警等）一并设计。

### 验收
- 单测/集成：fake provider 输出零事件即退出 → run 秒级转 failed 且携诊断消息；非零退出码路径不回退。
- 真实 E2E：包装脚本吞掉 provider stdout 派发一次 → run 不再永滞。
- CP 层看门狗（若同批做）：人为掐死 runtime 后派发 → 超时窗口后 run failed。

### 规模
runtime 层小（单文件+单测）；CP 层中（新周期任务+事件+测试）。

---

## 2.【功能补全】项目 MCP 绑定 Web 管理页

### 背景
P2 已落地 API：`PUT/GET /api/v1/projects/{projectId}/mcp-bindings`（声明式全量 PUT；authz `project.config.read`/`project.config.edit`，ResourceProject）；投影合并（员工∪项目、同 server_key 项目优先）已在派发链生效并经 E2E。**无 UI**——目前只能 curl。关联旧债：MCP 注册表页 `/mcp` 已有；team/employee 绑定管理 UX 当年也是 deferred 后补的，可参照其最终形态。

### 建议方案
- 项目运营详情的配置区（与成员/repo 绑定同级）加「MCP 绑定」卡：注册表 Select 添加、行内移除、整表保存（声明式 PUT 语义与后端对齐）；行显示 server_key/名称/transport/credential_env_var；说明文案点出"同 server_key 时项目绑定覆盖员工绑定"（spec §3.2 冲突语义）。
- 交互镜像团队能力 tab / 技能依赖区块的既有组件与样式；动 UI 前读 `DESIGN.md`；内部跳转 TanStack `Link`。
- 顺手一并评估 team/employee MCP 绑定 UX 是否借同一组件收敛（旧债，可选扩围）。

### 验收
- web 定向测试 + typecheck + verify:design-system（如触设计系统）。
- 真实链路：浏览器绑定 server → 派发该项目 chat → 工作区 `.superteam/mcp/claude.mcp.json` 含该 server（E2E 手法已验证过，照搬）。

### 规模
前端中等；后端契约零改动。

---

## 3.【API 健壮性·快赢】PUT mcp-bindings 缺 `items` 键被宽容为清空

### 现象（E2E 实测）
`PUT /projects/{id}/mcp-bindings` 带错误键名 body（如 `{"bindings":[...]}`）→ **200 且清空全部绑定**。契约 required `items` 未被服务端强制——手滑键名=静默清空生产绑定。

### 修法
handler decode 后区分「字段缺失」与「空数组」：缺失（nil）→ 400 "items is required"；`[]` 仍为合法的声明式清空。Go 侧用指针/`json.RawMessage` 判缺省。团队/员工绑定端点如有同型宽容，一并排查。

### 验收
单测三态（缺键 400 / `[]` 清空 / 正常替换）+ curl 复验。规模：小。

---

## 4.【技能包瑕疵】zip 根目录前缀不剥离

### 现象（两次 E2E 观察）
技能归档含统一根目录时解包不剥根：`<home>/.claude/skills/<key>/<zip根目录>/SKILL.md`——SKILL.md 不在 key 目录直下，不符技能目录规范（claude 仍能摸到但属侥幸；探针技能 evidence-e2e-probe 实际长这样）。evidence-grounding 收尾时已记"既有小瑕疵"（`common_root_prefix` 不剥根），出处 `apps/runtime-agent/src/skills.rs` 解包逻辑。

### 修法
解包时所有条目共享单一根目录则剥掉该前缀落盘。注意存量：`.skill-checksum` 未变的已装技能不会重物化——修复不自动矫正存量布局（可接受；如需矫正，重装或在 marker 里加布局版本）。

### 验收
单测（带根 zip / 平铺 zip / 多根 zip 不误剥）+ 真实上传探针技能复装确认 SKILL.md 落 key 直下。规模：小。

---

## 5.【上游跟踪·观察项】opencode 仓库配置屏蔽的上游化

skip-worktree+删除是绕行方案（有效但非本意）。理想态是 opencode 官方提供 strict/禁用项目配置开关（对齐 claude `--strict-mcp-config`）。跟踪 upstream release notes，有开关后把 `shield_repo_configs` 换成传开关并删绕行。无工期，升级 opencode 版本时顺手检查。

---

## 6.【触发式延后】员工能力缓存物理 LRU + manifest_version 全量清单

cache spec（2026-06-30）Phase 2 的剩余部分：员工缓存目录只增不减（绑定移除后投影层天然不软链，但磁盘残留）；员工级统一 manifest_version 未做（现用技能级 sha256 已满足"本地旧才拉新"）。**触发条件**：单员工技能数十个/节点员工数百个规模化，或节点磁盘压力出现。此前不做（YAGNI，spec 明确记录）。

## 7.【触发式可选】仓库 MCP 声明文件 IaC 导入注册表

spec §3.2 可选项：仓库放 provider 无关声明文件（`.superteam/mcp.yaml`）→ 平台导入流程登记进注册表、人类确认一次。**触发条件**：客户明确要求"MCP 配置随仓库版本走"。

---

## 8. 边界与现场记录（非债）

- **chat 态技能冲突无 attestation 行**：设计使然——attestation 只挂项目任务（chat 无 attempt 血缘），chat 冲突走 stderr 留痕。若未来 chat 也要审计面，属新需求非缺陷。
- **dev 库遗留 fixture**：员工「目录投影smoke-codex」「目录投影smoke-opencode」（各绑 evidence-e2e-probe 技能）保留作 provider smoke 复查资产；不再需要时一条 SQL 可清。
- **E2E 环境事实**（复用价值）：dev-services 以 `bash -lc` 启动，PATH 覆盖会被登录壳重置——给 runtime 传配置用 `RUNTIME_AGENT_*` env；需求状态无 `GET projects/{id}/demands/{id}` 端点，用 DB 或 launch-detail。
