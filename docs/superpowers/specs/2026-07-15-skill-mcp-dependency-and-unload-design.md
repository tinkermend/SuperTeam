# 技能↔MCP 依赖 + 会话级自动卸载 设计

日期：2026-07-15
状态：已与人类确认设计方向，待评审
范围来源：用户最初判断"MCP 管理前后端是空壳"，经真实链路冒烟证伪（API 注册/列表/删除、`/mcp` 页渲染、07-08 Codex 真实链路 E2E 记录均在）。确认后的真实缺口只有两块，即本 spec 的范围。

## 背景与现状（已冒烟核实）

MCP 能力管理主链路四层已存在且贯通，不在本次范围内重做：

- DB：迁移 037 的 `mcp_servers` / `team_mcp_bindings` / `digital_employee_mcp_bindings_v2`。
- Control Plane：`internal/capability/` service/repository/handler，`/api/v1/mcp-servers`、绑定、`effective-mcp-config` 路由已注册注入。
- Web：`/mcp` 管理页、员工能力面板、团队能力 tab 均真实调 API。
- Runtime：start-session payload 投影 env-satisfied 的 effective MCP；`mcp_config.rs` 为 Claude Code（`.mcp.json`）/ Codex（`config.toml`）/ OpenCode（`opencode.json`）物化配置，已带增量合并保留既有配置逻辑（含单测）。

已知运行期陷阱（非代码缺口，记录备查）：`CONTROL_PLANE_CREDENTIAL_KEY` 未配置时 credential sealer 为 nil，凭据加密路径会失败。

## 缺口与目标

1. **技能↔MCP 依赖**：技能无法声明"我依赖某 MCP 能力"，只有员工模板 `recommended_mcp_servers` 软推荐；不存在装载时的依赖校验。
2. **自动卸载**：写进员工家目录的 provider MCP 配置持久残留，无 teardown；任务 workspace 路径随 worktree 消亡（维持现状即可）。

## 已拍板的设计决策

| 决策点 | 结论 |
|---|---|
| 授权边界 | **依赖只校验不授权**：MCP 生效的唯一授权源仍是员工/团队绑定；技能依赖是装载前的校验门，不自动授予能力 |
| 缺依赖行为 | **阻塞任务等人类**：装载校验失败 → 任务 blocked，结构化说明缺什么；人类补绑定/补 env 后重派。复用现有 blocked 模式 |
| 卸载机制 | **注入清单 + 会话结束回滚**：写入时记 manifest（新增/覆盖+原值备份），会话结束逆操作；异常退出由下次启动先清残留兜底 |
| 依赖建模 | **独立关系表** `skill_mcp_dependencies`，外键参照完整性，双向索引 |
| UI | 页面样式与布局一起完成，遵循 `DESIGN.md` 与布局宪法基元 |

## 1. 数据模型（新迁移）

新表 `skill_mcp_dependencies`：

```sql
CREATE TABLE skill_mcp_dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    mcp_server_id UUID NOT NULL REFERENCES mcp_servers(id) ON DELETE RESTRICT,
    note TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- UNIQUE(tenant_id, skill_id, mcp_server_id)
-- 索引：按 skill 查依赖、按 mcp_server 反查技能
```

- `ON DELETE RESTRICT`：保护硬删除路径的参照完整性。注意 `mcp_servers` 与 `skills` 的删除都是**软删除**（`deleted_at`），外键不会触发——删除保护必须在应用层实现：删 MCP 定义前查依赖它的活跃技能，存在则拒绝（409）；删技能时应用层同步硬删其依赖行。
- 遵循 `DATABASE_DESIGN.md`（UUID-first、租户列、索引规范）；更新 `atlas.sum` 并 `make -C apps/control-plane migrate-validate`。

## 2. Control Plane 后端

### API（openapi.yaml + generate:control-plane）

- `GET /api/v1/skills/{skillId}/mcp-dependencies` — 列出技能依赖（联注册表基本信息）。
- `PUT /api/v1/skills/{skillId}/mcp-dependencies` — 全量声明式覆盖（body 为 `mcp_server_id` 列表 + 可选 note），贴合"技能声明依赖"语义，天然幂等。
- `GET /api/v1/mcp-servers/{serverId}/dependent-skills` — 反向查询，供 MCP 管理页展示与删除保护提示。

Authz 复用现有 `mcp_registry.read/manage` 与技能相关 action，不新增权限点。

### 装载校验链路（只校验不授权）

`run_service.prepareStartSessionDependencies` 已存在技能依赖闸门 `validateRuntimeSkillDependencies`（校验 tools/env，缺失即阻断派发）。MCP 依赖校验挂进同一环节：

1. 取本次将装载的 runtime 技能集合（既有 `ListSkillsForRuntime`）。
2. 经新增 lister 查 `skill_mcp_dependencies` 得到依赖 MCP 集合。
3. 对照 `deps.runtimeMCP`（既有 mcpLister 产出，已做绑定 ∪ env-satisfied 过滤）。
4. 任一依赖缺失 → 阻断派发，结构化原因：`技能 {slug} 依赖 MCP {server_key}：未绑定或缺环境变量`，走既有 blocked 呈现链路。
5. 全部满足 → 正常派发，payload 不因依赖而扩集（授权源不变）。

Console 侧同步：`ListEffectiveEmployeeSkills` 的 `runtime_dependency_status` 增加 `missing_mcp` 字段，供员工面板警示。

## 3. Runtime 自动卸载（会话作用域化 + 注入清单回滚）

**关键事实修正**：现状家目录 MCP 配置在 ProvisionInstance（实例开通）一次性写入，会话不写家目录。若仅在会话结束回滚，会删掉开通时配置且后续会话不再拥有 MCP。已与人类确认改为**会话作用域**：

- **注入时机前移到会话开始**：`ensure_command_instance` 解析出 agent_home 后，按会话 payload 的 `mcp_servers`（即当前生效绑定）执行家目录物化；ProvisionInstance 不再写 MCP 配置。绑定变更下次会话自动生效。
- **注入清单 manifest**（JSON，存 `agent_home/.superteam/`）：写入前记录每个目标配置文件的整文件快照（存在与否 + 原内容）。因 codex/opencode 合并是整键替换，无法从结果反推，必须写前快照。
- **会话结束回滚**：run 收尾共同路径（`drain_provider_events` 成功/失败收尾）+ StopSession 取消路径 + 早期失败路径，按 manifest 还原快照/删除新建文件，然后删 manifest。用户自有配置零触碰（快照整文件还原）。
- **异常退出兜底**：会话开始注入前发现残留 manifest 先回滚再注入，幂等。控制平面保证同一员工同时仅一个活跃 run（有测试背书），无并发竞态。
- 任务 workspace 路径 `materialize_task_mcp_config` 维持现状（随 worktree 消亡即卸载）。
- 三种 provider 配置格式（`.mcp.json` / `config.toml` / `opencode.json`）的注入/回滚/残留兜底均需单测覆盖。

## 4. 前端 UI（含样式布局）

遵循 `DESIGN.md` 与布局宪法既有基元（`Main`、V3 组件、MasterDetailLayout 等）：

- **技能详情页**（`features/skills/detail.tsx`）新增"依赖 MCP"区块：依赖列表（名称/server_key/鉴权/风险/状态）+ 从注册表选择添加、移除，走 PUT 全量接口。
- **MCP 管理页**（`features/mcp/index.tsx`）：展示"被哪些技能依赖"；删除遇 RESTRICT 给出保护提示；顺带按设计规范核对现有表格/表单样式细节。
- **员工能力面板**（`employee-capabilities-panel.tsx`）：effective config 区显示依赖校验结果，缺失项警示条（哪个技能缺哪个 MCP/env），引导补绑定。
- **任务 blocked 呈现**：复用现有 blocked 原因展示，渲染结构化缺依赖原因。

## 5. 测试与验证

- Go：repository/service/handler 单测 + `run_service` 缺依赖 blocked 用例。
- Rust：三种 provider 格式的 manifest 写入/回滚/残留兜底单测。
- Web：vitest 组件测试（技能依赖区块、MCP 反向列表、员工面板警示）。
- **真实 E2E（默认完成条件）**：注册 MCP → 技能声明依赖 → 员工装技能未绑 MCP → 派发 → 确认 blocked 及原因 → 补绑定 → 重派 → provider 目录真实生成配置 → 会话结束 → 家目录配置回滚干净、manifest 删除。
- 门禁：`verify:foundation` / `verify:web` / `verify:runtime-agent` / `verify:db`。

## 明确不做（YAGNI）

- 不改现有授权模型/绑定表；不做"装技能自动绑 MCP"。
- 不做技能包 frontmatter 依赖声明的导入解析（可作后续增强，事实源仍是关系表）。
- 不动员工模板 `recommended_mcp_servers` 软推荐语义。
- 不做 stdio transport 支持（注册表现状仅 HTTP/streamable HTTP）。
