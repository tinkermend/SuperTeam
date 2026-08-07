# 派发投影可见性：任务执行面展示本次能力清单

- 日期：2026-08-07
- 状态：**已实施（P0 + P0.1 + P1 + P2b）**（2026-08-07）
- 系列：能力供给三层模型（`2026-08-06-capability-supply-three-layer-design.md`）+ 控制台 UI（`2026-08-06-capability-binding-console-design.md`）的 **P3**
- 交付性质：**小后端读路径 + 前端展示**。不改投影语义、不改绑定 API、不改 runtime 物化/软链
- 目标读者：实施会话（本文自包含；实施前必读三层模型 §4.2/§4.3/§5 与 UI design §7 P3）

---

## 0.0 开工须知

### 环境

| 项 | 值 |
|---|---|
| Web | `http://127.0.0.1:3100` |
| Control Plane | `http://127.0.0.1:8080` |
| 登录 | `admin` / `admin` |
| 门禁 | `verify:contracts` / `verify:control-plane` / `verify:web` |

### 关键判断（纠正 UI design 的乐观假设）

UI design §7 写过：

> 数据在 attestation 里已经齐了（`skill_conflicts` + `source_scope`），缺的只是展示面。

**不完全准确。** 实测数据落点如下：

| 事实 | 落点 | Console 今天能否读 |
|---|---|---|
| 最终投影 `skills[]`（含 `source_scope`） | `runtime_command_receipts.payload.skills`（start_session） | **否**（无 console 读路径） |
| 最终投影 `mcp_servers[]`（含 `source_scope`） | 同上 `payload.mcp_servers` | **否** |
| 控制平面冲突（`source=project_binding`） | `payload.metadata.skill_conflicts` | **否** |
| 仓库原生键冲突（`source=workspace_native`） | **仅** runtime 回写的 `project_task_attestations.metadata.skill_conflicts` | **否**（无 list attestations 的 console API） |
| `GetDigitalEmployeeRun` / `ProjectExecutionTraceAttempt` | 结果/诊断/会话接续等 | **不含**能力投影 |

因此本批**必须**补一条「把已有 payload 收成安全读模型」的后端路径；**不是纯前端**。

### 可复用

- 执行轨迹面板：`apps/web/src/features/projects/components/project-execution-trace-panel.tsx` 的 `AttemptRow`（已有会话接续 MetaBlock 先例）
- 任务详情：`project-task-detail-dialog.tsx` 已有「查看执行轨迹」深链 `?tab=trace&task=`
- 词表：`status-labels.ts` 已有 `team` / `employee` / `project` / `dependency_closure` / `project_binding`（P3 展示直接用）
- 接续可观测 spec（`2026-08-07-session-resume-observability-design.md`）：同一执行轨迹 attempt 上挂结构化结论 + 中文标签的模式
- 链接：attempt.`digital_employee_run_id` → run.`command_id` → `GetRuntimeCommandReceiptByCommandID`

### 踩坑

- **禁止**把整个 `payload` 透给前端：`environment[].value` 含解密后的 env，属敏感。只投影白名单字段。
- skills payload **没有 name**，只有 `skill_id` / `skill_key` / `version` / `source_scope`。展示必须 batch 补名，或降级 `skill_key`，**不得裸 UUID**。
- attestation 在会话**开始后**才有 `workspace_native` 冲突；派发瞬间只有 CP 侧冲突。UI 要容忍「运行中冲突列表比派发时多」。
- 历史 attempt 若 `digital_employee_run_id` 为空（旧冲突路径残留），投影块显示「无投影快照」而不是报错。

---

## 1. 为什么要做

三层模型上线后，人能在配置页**配**场地供给，也能在员工页看到「限 N 个项目」，但一旦派发：

1. **看不见本次到底投了什么**——通用技能 / 项目供给 / 依赖闭包补的 MCP 混在一起，排障只能翻 DB 或 runtime 日志。
2. **看不见冲突谁赢了**——同 slug 项目优先、`source=project_binding` 已写进 metadata，人看不见会误以为「绑了员工技能却没生效」是 bug。
3. **配置页闭包预览与真实派发对不上号**——预览是「将一并投影」；派发结果才是权威。缺结果面，预览变成不可验证承诺。

本批目标：在**已经发生的一次 attempt** 上，人能回答三句话：

> 这次任务投了哪些技能 / MCP？  
> 各自从哪一层来（团队 / 员工 / 项目 / 依赖补全）？  
> 有没有同名冲突，谁留下了？

---

## 2. 非目标

| 不做 | 理由 |
|---|---|
| 改三层投影语义 / 闭包规则 / 绑定时校验 | 已验收；本批只读 |
| 新建「投影预览」独立页或全局能力地图 | 避免做成没人看的大盘；挂在已有执行路径上 |
| 把完整 command payload / env / archive URL 暴露给 Console | 安全；只出白名单投影快照 |
| 回填历史 attestation 或改 runtime 写路径形状 | 读路径从既有 receipt 抽取即可覆盖有 command 的历史 attempt |
| 项目级磁盘缓存、门禁、准入推导 | 三层模型明确延后/不做 |
| MCP 注册表「被哪些项目引用」影响面 | 对称性小债，**可选 P2b**，不挡本批 |
| 派发前 dry-run 投影 API | 有价值但另立；本批只做**已派发**结果 |

---

## 3. 读模型（契约）

### 3.1 `CapabilityProjectionSnapshot`

挂在 `ProjectExecutionTraceAttempt.capability_projection`（可选字段；缺省 = 无快照）。

```yaml
CapabilityProjectionSnapshot:
  type: object
  required: [skills, mcp_servers, skill_conflicts, available]
  properties:
    available:
      type: boolean
      description: false = 无法解析（无 run/command/payload）；UI 显示降级文案，不是 error
    skills:
      type: array
      items:
        $ref: '#/components/schemas/ProjectedSkillItem'
    mcp_servers:
      type: array
      items:
        $ref: '#/components/schemas/ProjectedMcpItem'
    skill_conflicts:
      type: array
      items:
        $ref: '#/components/schemas/ProjectedSkillConflict'
    summary:
      type: object
      description: 服务端预聚合，供卡片一行摘要；可选但建议带
      properties:
        skill_count: { type: integer }
        mcp_count: { type: integer }
        conflict_count: { type: integer }
        by_source:
          type: object
          additionalProperties: { type: integer }
          description: key = source_scope（team/employee/project/dependency_closure）

ProjectedSkillItem:
  required: [skill_id, skill_key, source_scope]
  properties:
    skill_id: { type: string, format: uuid }
    skill_key: { type: string }          # slug
    skill_name: { type: string }         # 补名；缺失时前端用 skill_key
    version: { type: string }
    source_scope:
      type: string
      description: team | employee | project（与派发 payload 一致）

ProjectedMcpItem:
  required: [server_id, server_key, source_scope]
  properties:
    server_id: { type: string, format: uuid }
    server_key: { type: string }
    server_name: { type: string }        # payload 已有 name
    source_scope:
      type: string
      description: team | employee | project | dependency_closure

ProjectedSkillConflict:
  required: [slug, source]
  properties:
    slug: { type: string }
    source:
      type: string
      description: project_binding | workspace_native | …
    winning_skill_id: { type: string, format: uuid }
    dropped_skill_id: { type: string, format: uuid }
    winning_source: { type: string }     # e.g. project / employee
    dropped_source: { type: string }
    winning_skill_name: { type: string }
    dropped_skill_name: { type: string }
```

**不新增端点。** 扩展既有：

- `GET /api/v1/projects/{projectId}/execution-trace` → attempt 内嵌 `capability_projection`
- （P1 可选）`GET /api/v1/digital-employees/{id}/runs/{runId}` → 同结构字段，供员工 Run 抽屉复用

权限沿用 execution-trace / get-run 既有 `project.read` / 员工读权限，不新造动作。

### 3.2 组装算法（控制平面）

对每个 attempt：

```
available = false
if attempt.digital_employee_run_id 空 → 空快照（available=false）
run = GetDigitalEmployeeRun(run_id)
receipt = GetRuntimeCommandReceiptByCommandID(run.command_id)
若 receipt 不存在或 payload 非 object → 空快照

skills = payload.skills 白名单字段
mcp   = payload.mcp_servers 白名单字段（禁止 credential 值、headers 实值）
conflicts_cp = payload.metadata.skill_conflicts || []

// 可选合并（本批建议做，成本低）：
atts = ListProjectTaskAttestations(attempt_id)  // 已有 sqlc，无 console 路由即可内部用
conflicts_rt = flatten(att.metadata.skill_conflicts)
conflicts = merge_by (slug, source, winning_skill_id, dropped_skill_id) 去重

// 补名：收集 skill_id ∪ winning/dropped → batch 查 skills.name
available = true
summary = 计数 + by_source
```

**性能**：execution-trace 一次可返回多 attempt。要求：

- 按 run_id / command_id **批量**取 receipt（可新增 sqlc `ListRuntimeCommandReceiptsByCommandIDs`，禁止 N+1）
- 技能名 **一次** `WHERE id = ANY($ids)`
- attestation 合并可按 attempt_ids 批量；若实现成本高，P0 可只出 CP 冲突，P0.1 再并 runtime

### 3.3 字段白名单（安全）

| 来源路径 | 允许 | 禁止 |
|---|---|---|
| `skills[]` | skill_id, skill_key, version, source_scope | archive_object_ref, 任意 URL |
| `mcp_servers[]` | server_id, server_key, name→server_name, source_scope | url, headers_env 实值, credential 明文 |
| `metadata.skill_conflicts` | 见 schema | — |
| `environment` | **整段丢弃** | 全部 |
| `prompt` / `persona` / secrets | **丢弃** | 全部 |

---

## 4. UI

### 4.1 主落点：执行证据链 `AttemptRow`（P0，必须）

文件：`project-execution-trace-panel.tsx`

在「会话接续 / Provider / Runtime」Meta 区下方、执行摘要上方，增加 **「能力投影」** 块：

```
能力投影                    技能 3 · MCP 2 · 冲突 1
┌─────────────────────────────────────────────┐
│ 技能                                        │
│  • 数据库巡检 (db-inspect)     [员工]       │
│  • Linux 排障 (linux)          [项目级]     │
│ MCP                                         │
│  • GitHub (github-mcp)         [依赖补全]   │
│  • 只读库 (pg-ro)              [团队]       │
│ 冲突                                        │
│  • linux：保留项目绑定，覆盖员工携带        │
└─────────────────────────────────────────────┘
```

规则：

| 状态 | 展示 |
|---|---|
| `available=false` | 一行 mute 文案：「本次尝试无能力投影快照」（不占大空态） |
| skills/mcp 皆空且 available | 「未投影任何技能或 MCP」 |
| 有冲突 | 冲突区用 `warn` 色；文案经词表，不直接甩枚举 |
| 来源 pill | `statusLabel(source_scope)`；`dependency_closure`→「依赖补全」，`project`→「项目级」 |
| 名称 | `skill_name || skill_key`；副文 `skill_key` mono；MCP 同理 |
| 默认折叠 | 技能+MCP 合计 ≤ 6 且无冲突时展开；否则摘要行 +「展开」按钮（避免轨迹刷屏） |

组件建议拆：`AttemptCapabilityProjection`（纯展示，props = snapshot），便于单测。

### 4.2 次落点：任务详情弹层摘要（P0，薄）

`project-task-detail-dialog.tsx`：若当前任务能从 trace 缓存/请求里拿到**最近一次 attempt** 的 snapshot，在执行区加一行：

> 最近投影：技能 N · MCP M · 冲突 K · [查看执行轨迹]

点进既有 `?tab=trace&task=` 深链。  
**不要**在任务详情再嵌完整双表——详情保持轻，细节归轨迹。

实现约束：任务详情已可能为别的事拉 trace 或可 `project_task_id` 过滤 `getProjectExecutionTrace`；若会多一次请求，limit 到该 task 的 attempts 即可（query 已支持 `project_task_id`）。

### 4.3 员工 Run 抽屉（P1，可选同批）

`run-detail-drawer.tsx`：GetRun 若带 `capability_projection`，同样挂一块。  
覆盖 chat run / 非项目路径排障。P0 不做不挡验收。

### 4.4 词表补丁

已有键够用。若展示 `workspace_native` 冲突来源，补：

```
workspace_native: "仓库原生"
```

冲突句式（组件内模板，枚举经词表）：

- `project_binding`：`「{slug}」保留{winning_source 中文}，覆盖{dropped_source 中文}`
- `workspace_native`：`「{slug}」工作区已有同名技能，跳过员工侧投影`
- 未知 source：`「{slug}」发生技能冲突（{source 中文或原文}）`

---

## 5. 分期

| 切片 | 内容 | 可独立验收 |
|---|---|---|
| **P0** | 契约 `CapabilityProjectionSnapshot` + execution-trace 组装（CP 冲突 + 批量 receipt）+ AttemptRow UI + 任务详情一行摘要 | `verify:contracts` / CP 单测 / 浏览器 |
| **P0.1** | 合并 attestation `workspace_native` 冲突 | 有真实冲突的 attempt 或 fixture |
| **P1** | GetDigitalEmployeeRun 同字段 + Run 抽屉 | **已实施** |
| **P2b（可选）** | MCP 注册表「项目绑定」影响面（与技能详情对称） | **已实施** |

P0 为完成定义下限。P0.1 强烈建议同批——否则 S7 仓库原生半边在 UI 仍不可见。

---

## 6. 后端落点清单（实施导航）

| 层 | 文件/点 | 动作 |
|---|---|---|
| OpenAPI | `contracts/control-plane/openapi.yaml` | 新 schema；`ProjectExecutionTraceAttempt.capability_projection`；可选 `DigitalEmployeeRun.capability_projection` |
| codegen | `generate:control-plane` | 生成 |
| sqlc | `runtime_command.sql` | `ListRuntimeCommandReceiptsByCommandIDs`（tenant_id + command_ids[]） |
| sqlc | attestations | 若尚无 by-attempt 批量，补 `ListProjectTaskAttestationsByAttemptIDs`（内部用） |
| project | `GetExecutionTrace` / `buildProjectExecutionTrace` | 批量装 snapshot，写入 attempt |
| employee | `runResponseFromDomain`（P1） | 同抽取函数复用 |
| 共享 | 新建小包函数如 `employee.ExtractCapabilityProjection(payload) (Snapshot, error)` 或 `project/capability_projection.go` | **单一抽取实现**，禁止两处手写 JSON 路径 |
| 单测 | payload fixture：含 project / dependency_closure / skill_conflicts；无 command；env 不得出现在输出 | |
| Web | `lib/api/projects.ts` 类型；`AttemptCapabilityProjection`；trace panel；task dialog 一行 | |
| 词表 | `workspace_native` | |

---

## 7. 验收 GATE

| ID | 步骤 | 期望 |
|---|---|---|
| V1 | 打开有真实派发的项目 → 执行证据链 → 展开 attempt | 出现「能力投影」块；技能/MCP 名称非裸 UUID；来源 pill 为中文 |
| V2 | 造：项目绑技能 A，员工不携带，派发 | 列表含 A，`source_scope` pill =「项目级」 |
| V3 | 造：技能依赖 MCP 且员工 env 齐，MCP 未绑项目 | MCP 出现且 pill =「依赖补全」 |
| V4 | 造：同 slug 员工携带 vs 项目绑定不同版本 | 冲突区可见；文案含保留/覆盖；投影列表仅胜出项 |
| V5 | attempt 无 `digital_employee_run_id` 或 receipt 丢失（可用 mock） | 降级文案，页面不炸、不 500 |
| V6 | 任务详情看最近投影摘要数字，点「查看执行轨迹」 | 深链到 trace 且 task 过滤正确 |
| V7 | 抓 execution-trace JSON | **无** `environment`、`archive_object_ref`、MCP url/headers 实值 |
| V8 | `verify:contracts` + `verify:control-plane` + `verify:web` | 全过 |

**承重**：V3（闭包来源可见）与 V4（冲突可见）与 V7（不泄密）。

**造数**：沿用三层模型验收数据路径；现网若有近期 S10 项目可直接打开 trace。无数据时用 live 测试或最小派发 fixture。

---

## 8. 风险

| 风险 | 缓解 |
|---|---|
| 轨迹页 attempt 多 → 批量 receipt 仍重 | command_ids 去重一次查；只取 payload 的 jsonb 子路径（SQL `payload - 'environment' - 'prompt' ...` 更佳） |
| 技能已删导致补名失败 | `skill_name` 空，UI 降级 skill_key；不 404 整条 trace |
| 前端把 source_scope 当英文渲染 | 强制 `statusLabel`；guard 测试覆盖新组件若有裸字段 |
| 与 session_resume 字段抢视觉 | 投影块在 Meta 网格下独立 section，不塞进 MetaBlock 第四列 |
| 「数据已在 attestation」误导后续会话 | 本文 §0.0 写明；同步改 UI design §7 指针 |

---

## 9. 与既有文档关系

| 文档 | 关系 |
|---|---|
| `2026-08-06-capability-supply-three-layer-design.md` §10 | UI 范围第 4 条「派发投影可见性」→ **本文落地** |
| `2026-08-06-capability-binding-console-design.md` §7 P3 | 从「待立项」改为指向本文；并更正「纯展示」为「读路径 + 展示」 |
| `2026-08-07-session-resume-observability-design.md` | 同 attempt 挂读模型的既有先例；不共享字段 |

---

## 10. 一句话方案

> **从 start_session 的 command receipt 抽出安全的能力投影快照（技能/MCP/来源/冲突），挂到执行轨迹的每次 attempt 上；人在排障路径直接看见「这次投了什么、从哪来、冲突谁赢」，且绝不把 env 与凭据带进 Console。**
