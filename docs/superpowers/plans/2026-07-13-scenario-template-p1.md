# 场景模板注册表 P1 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 落地 `docs/superpowers/specs/2026-07-13-scenario-template-registry-design.md` P1 + §9 P1 触点：`scenario_templates` 注册表（迁移+种子）、项目绑定、planner 注入与 `template_key` 校验闭环、template_key 落 PlanRevisionPayload、Console 只读目录页 + 项目创建下拉 + 计划卡显示。

**Architecture:** 后端照抄 capability(MCP registry) 模块五层模式（迁移/sqlc/模块/authz/openapi）；模板内容经 `CoordinationSnapshot` 新字段进 planner user prompt（system prompt 加静态指令），解码后服务端校验 `plan.TemplateKey == 绑定 key`；`template_key` 借 PlanRevisionPayload（jsonb）流到前端，**不加 route_decisions 列**。Web 照抄 /mcp 页模式。

**Tech Stack:** Postgres+Atlas+sqlc、Go chi、oapi-codegen、React+TanStack Router+react-query+v3 组件。

## Global Constraints

- 迁移唯一目录 `apps/control-plane/internal/storage/migrations/`，新编号 **058**；变更后 `atlas migrate hash --dir file://internal/storage/migrations` 更新 atlas.sum，并 `make -C apps/control-plane migrate-validate`（本地 dev 库可覆盖 DEV_URL）。**禁止回写已存在于 atlas.sum 的迁移**。
- 契约改动走 `corepack pnpm generate:control-plane`（生成 `apps/control-plane/gen/control_plane.gen.go`）+ `corepack pnpm verify:contracts`。
- 门禁：`verify:control-plane`、`verify:web`；Web 测试禁 `npx vitest`，用 `corepack pnpm --filter @superteam/web test`。
- 前端已读 DESIGN.md 关键约束：注册表页必须实底脆数据面（不透明 WorkSurface+V3Table，禁玻璃/模糊）；组件优先复用 v3 家族禁手搓；列表行带主时间字段（相对时间 tabular-nums）；页面骨架 = ShellPageHeader + 指标带 + WorkSurface + 统一四态。
- 内部跳转用 TanStack `Link`/`navigate`。
- **不做**：管理端点（POST/PUT/DELETE，P3）、可行性判定与 feasibility（P2）、需求级覆盖（P2）、模板编辑 UI（P3）。
- 宪法：核心不建封闭枚举——模板是数据行；`generic` 兜底=未绑定项目行为与今天完全一致（P1 实现为"未绑定 → 不注入、不校验 key"，等价于 generic 语义，见 Task 4 注）。
- git 提交尾注 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

## 已核实的关键事实（三路探查汇总，写码前不要重新怀疑）

| 事实 | 位置 |
|---|---|
| 迁移样板（UUID pk/tenant/partial 唯一索引/触发器/中文 COMMENT） | `migrations/037_mcp_http_capability_registry.sql` |
| 种子样板（固定 UUID + ON CONFLICT (id) DO NOTHING，dev 租户 `…0001`） | `migrations/009_skill_management.sql:129` |
| sqlc 查询样板 | `queries/capability.sql:224`（Create/List/Get/Delete MCPServerDefinition） |
| 模块样板 | `internal/capability/{types,service,pg_repository,handler}.go`；无加密可仿 skill 装配 |
| 路由注册 + handler 注入 | `api/server.go:404`（mcp-servers 组）、`:187 SetCapabilityHandler`；装配 `internal/app/app.go:476/618/636` |
| authz 样板 | `authz/types.go:56` ActionMCPRegistryRead/Manage；`authorizer.go:136` tenant 资源+checkTenantAdminAccess；handler 判权 `capability/handler.go:300,561` |
| openapi 样板 | `openapi.yaml:2188`（/api/v1/mcp-servers paths）、`:8934`（schema） |
| CreateProject 链 | openapi:7170 → `project/handler.go:244,257` → `service.go:133,159` → `pg_repository.go:66,83`（queries.CreateProjectParams）；projects 表列见 013/040/057 迁移，**无 scenario_template_key** |
| 快照 | `planner.go:12 CoordinationSnapshot{ProjectID,Demand,DigitalEmployeePool,CoordinationPolicy,PreviousRouteContext}`；装载 `project_store.go:282`，projects 行只进 ID+CoordinationPolicy（:353） |
| planner prompt | user：`openai_compatible_planner.go:293 buildPlannerUserPrompt`（整快照 json.Marshal 进 payload，`plannerPromptSnapshot` :308）；system：`:272 buildPlannerSystemPrompt()` 无参，调用点 :101 |
| template_key 现状 | 解码进 `RouteDecisionPlan.TemplateKey`（planner.go:46）后**不落任何表**；PlanRevisionPayload（plan_revision_payload.go:14）不含它 |
| 计划确认卡 | `features/projects/components/project-operational-detail.tsx:382-484`；单行 label/value 组件 `RuntimeMeta`(:1566)；数据 `ProjectPlanRevision.payload Record<string,unknown>`（projects.ts:507,518） |
| 侧栏 | `components/layout/data/sidebar-data.ts:81-116` 流程能力分组，条目 `{title,url,icon,iconTone:"neutral"}`，icon 自 lucide-react |
| 注册表页样板 | 路由壳 `routes/_authenticated/mcp/index.tsx`（createFileRoute 6 行）；页面 `features/mcp/index.tsx`（ShellPageHeader+Main+V3MetricCard 带+WorkSurface 四态+V3Table）；API client `lib/api/capabilities.ts:265`（getJson 封装 `lib/api/client.ts`）；测试 `features/mcp/index.test.tsx`（vi.mock api + QueryClientProvider wrapper + expect.element） |
| 项目创建表单 | `features/projects/components/create-project/`：draft state（create-project-draft.ts:18 ProjectCreateDraft + :127 buildProjectCreateInput）；shadcn Select 样板 submit-demand-dialog.tsx:127；提交 `lib/api/projects.ts:1091 createProject` + `:848 CreateProjectInput` |
| task_kind 约束 | system prompt :288 限定 canonical 三种且"Do not invent"——**P1 种子骨架不带 task_kind 字段**（step/role/produces 驱动，task_kind 仍由 planner 按 canonical 选），避免撞枚举校验 |

---

### Task 1: 迁移 058（表 + projects 加列 + 种子）+ sqlc 查询

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/058_scenario_templates.sql`
- Create: `apps/control-plane/internal/storage/queries/scenario_template.sql`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`（CreateProject INSERT 增列——先读现查询原文）
- 生成物：`make -C apps/control-plane generate-sqlc`；atlas.sum 重 hash

**Interfaces:**
- Produces: 表 `scenario_templates`；`projects.scenario_template_key TEXT`（可空）；sqlc `ListScenarioTemplates(tenantID) :many`、`GetScenarioTemplateByKey(tenantID,key) :one`；`queries.CreateProjectParams` 增 `ScenarioTemplateKey`。

- [ ] **Step 1: 写迁移**（骨架如下，COMMENT 全列补齐，种子五条用固定 UUID `…0401`–`…0405`）：

```sql
-- 场景模板注册表：租户级"这类场景该怎么干"的沉淀层（角色契约集合 + 分解骨架 + 默认交接契约）。
-- 内容有限、机制开放：加一类场景 = 插一行数据，核心代码不建枚举。

CREATE TABLE IF NOT EXISTS scenario_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    template_key TEXT NOT NULL,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    status VARCHAR(40) NOT NULL DEFAULT 'active',
    deleted_at TIMESTAMPTZ,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_scenario_templates_key_not_blank CHECK (btrim(template_key) <> ''),
    CONSTRAINT ck_scenario_templates_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT ck_scenario_templates_status_supported CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_scenario_templates_tenant_key_active
    ON scenario_templates(tenant_id, template_key) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_scenario_templates_tenant_status
    ON scenario_templates(tenant_id, status, created_at DESC) WHERE deleted_at IS NULL;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'update_scenario_templates_updated_at') THEN
        CREATE TRIGGER update_scenario_templates_updated_at
        BEFORE UPDATE ON scenario_templates
        FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
    END IF;
END $$;

ALTER TABLE projects ADD COLUMN IF NOT EXISTS scenario_template_key TEXT;

COMMENT ON TABLE scenario_templates IS '租户级场景模板注册表，驱动规划分解与交接契约实例化';
COMMENT ON COLUMN scenario_templates.template_key IS '模板稳定标识（如 software_delivery），租户内未删除时唯一';
COMMENT ON COLUMN scenario_templates.spec IS '模板内容：roles/skeleton/default_acceptance_criteria/risk_policy/feasibility_thresholds';
COMMENT ON COLUMN projects.scenario_template_key IS '项目绑定的场景模板 key，可空=generic 兜底（行为同无模板）';
-- （其余列 COMMENT 照 037 风格补齐）

INSERT INTO scenario_templates (id, tenant_id, template_key, name, description, spec) VALUES
('00000000-0000-0000-0000-000000000401'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'software_delivery', '软件开发',
 '开发→审查→测试的软件交付场景；审查者与开发者必须不同人（四眼原则），开发与测试可同人。',
 '{"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"],"collapsible_with":["tester"],"independent_from":[]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"],"collapsible_with":[],"independent_from":["developer"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"],"collapsible_with":["developer"],"independent_from":[]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"head_commit","kind":"git_commit"},{"name":"branch_ref","kind":"branch_ref"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]}],"default_acceptance_criteria":["变更以 branch+commit 交付且通过独立审查","测试报告覆盖主路径且结论可判"],"risk_policy":{"release_requires_human":true},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000402'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'ops_analysis', '运维分析',
 '采集→分析的系统运行分析场景；分析可与采集同人。',
 '{"roles":[{"key":"collector","title":"采集","required_capabilities":["log.analysis"],"collapsible_with":["analyst"],"independent_from":[]},{"key":"analyst","title":"分析","required_capabilities":["incident.triage"],"collapsible_with":["collector"],"independent_from":[]}],"skeleton":[{"step":"collect","role":"collector","produces_defaults":[{"name":"raw_metrics","kind":"evidence_ref"}]},{"step":"analyze","role":"analyst","depends_on":["collect"],"required_inputs_defaults":["raw_metrics"],"produces_defaults":[{"name":"analysis_conclusion","kind":"conclusion"}]}],"default_acceptance_criteria":["结论附证据指针，可追溯到采集数据"],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000403'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'incident_response', '故障排查',
 '诊断→修复→验证的故障处置场景；验证者与修复者必须不同人。',
 '{"roles":[{"key":"diagnostician","title":"诊断","required_capabilities":["incident.triage"],"collapsible_with":["operator"],"independent_from":[]},{"key":"operator","title":"修复","required_capabilities":["incident.triage"],"collapsible_with":["diagnostician"],"independent_from":[]},{"key":"verifier","title":"验证","required_capabilities":["incident.triage"],"collapsible_with":[],"independent_from":["operator"]}],"skeleton":[{"step":"diagnose","role":"diagnostician","produces_defaults":[{"name":"root_cause","kind":"conclusion"}]},{"step":"fix","role":"operator","depends_on":["diagnose"],"required_inputs_defaults":["root_cause"],"produces_defaults":[{"name":"fix_record","kind":"evidence_ref"}]},{"step":"verify","role":"verifier","depends_on":["fix"],"required_inputs_defaults":["fix_record"],"produces_defaults":[{"name":"verification_result","kind":"conclusion"}]}],"default_acceptance_criteria":["根因结论与修复记录可相互印证","验证结果由非修复者出具"],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000404'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'research_report', '调研报告',
 '检索→综合成稿的调研场景；两阶段可同人。',
 '{"roles":[{"key":"researcher","title":"检索","required_capabilities":[],"collapsible_with":["writer"],"independent_from":[]},{"key":"writer","title":"成稿","required_capabilities":[],"collapsible_with":["researcher"],"independent_from":[]}],"skeleton":[{"step":"search","role":"researcher","produces_defaults":[{"name":"source_list","kind":"evidence_ref"}]},{"step":"synthesize","role":"writer","depends_on":["search"],"required_inputs_defaults":["source_list"],"produces_defaults":[{"name":"final_report","kind":"artifact_ref"}]}],"default_acceptance_criteria":["报告结论均有来源清单支撑"],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb),
('00000000-0000-0000-0000-000000000405'::uuid, '00000000-0000-0000-0000-000000000001'::uuid,
 'generic', '通用兜底',
 '无场景约束的兜底模板：不注入骨架，规划行为与未绑定模板完全一致。',
 '{"roles":[],"skeleton":[],"default_acceptance_criteria":[],"risk_policy":{},"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb)
ON CONFLICT (id) DO NOTHING;
```

- [ ] **Step 2: sqlc 查询** `queries/scenario_template.sql`：

```sql
-- name: ListScenarioTemplates :many
SELECT * FROM scenario_templates
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
ORDER BY created_at ASC, template_key ASC;

-- name: GetScenarioTemplateByKey :one
SELECT * FROM scenario_templates
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND template_key = sqlc.arg('template_key')::text
  AND deleted_at IS NULL;
```

`queries/project.sql` 的 CreateProject INSERT 增加 `scenario_template_key`（值 `sqlc.narg('scenario_template_key')::text`）——先读该查询现文照改。

- [ ] **Step 3: 生成与校验**
Run: `make -C apps/control-plane generate-sqlc && (cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations) && make -C apps/control-plane migrate-validate`
Expected: 生成 `scenario_template.sql.go`、models 增 `ScenarioTemplate`；validate 通过。（DEV_URL 若默认 Docker 不可用，用真实 dev 库覆盖——照 CLAUDE.md。）

- [ ] **Step 4: 编译回归** `go build ./apps/control-plane/internal/... && go test ./apps/control-plane/internal/storage/...`
- [ ] **Step 5: 提交** `feat(db): add scenario_templates registry with seeds and project binding column`

---

### Task 2: Go 模块 `scenariotemplate`（只读）+ authz + 路由 + 契约

**Files:**
- Create: `apps/control-plane/internal/scenariotemplate/{types.go,pg_repository.go,service.go,handler.go,handler_test.go}`
- Modify: `internal/authz/types.go`、`internal/authz/authorizer.go`、`internal/api/server.go`、`internal/app/app.go`
- Modify: `contracts/control-plane/openapi.yaml` + `corepack pnpm generate:control-plane`

**Interfaces:**
- Produces: `scenariotemplate.ScenarioTemplate{ID,TenantID,Key,Name,Description,Spec map[string]any,Status,CreatedAt,UpdatedAt}`；`Service.List(ctx,tenantID)([]ScenarioTemplate,error)`、`Service.GetByKey(ctx,tenantID,key)(ScenarioTemplate,error)`、`ErrScenarioTemplateNotFound`；HTTP `GET /api/v1/scenario-templates`、`GET /api/v1/scenario-templates/{templateKey}`；authz `scenario_template.read`。

- [ ] **Step 1: 写失败测试**（handler_test.go，仿 capability/handler_test.go 的 mock service + httptest 模式——先读它抄骨架）：List 返回 200+数组；GetByKey 未知 key 返回 404；未授权 403。
- [ ] **Step 2: 实现四件套**：types（含 `specRoleCount` 等留给 handler 层不做，原样透传 Spec jsonb → map[string]any via json.Unmarshal）；pg_repository 用 `queries.ListScenarioTemplates/GetScenarioTemplateByKey`（`pgtype` 转换仿 capability/pg_repository.go）；service 薄透传；handler `List`/`GetByKey` + `SetAuthorizer` + `authorize(...)`（照 capability/handler.go:561 抄 authorize 辅助）。
- [ ] **Step 3: authz**：types.go 加 `ActionScenarioTemplateRead = "scenario_template.read"`、`ActionScenarioTemplateManage = "scenario_template.manage"`（manage 本期无路由，常量先立）；authorizer.go 在 mcp_registry case 同组加两常量（同为 tenant 资源 + checkTenantAdminAccess）。检查 authz 既有测试是否枚举 action 列表需同步。
- [ ] **Step 4: 路由与装配**：server.go 加字段+`SetScenarioTemplateHandler`（仿 :187）+ ConsoleUserAuth 组内 `r.Get("/scenario-templates", ...)`、`r.Get("/scenario-templates/{templateKey}", ...)`；app.go 装配 `scenariotemplate.NewPgRepository(q)` → `NewService(repo)` → `NewHandler(svc)` → `server.SetScenarioTemplateHandler(...)`。
- [ ] **Step 5: openapi**：paths 两个 GET（照 :2188 格式）+ `ScenarioTemplate` schema（id/tenant_id/template_key/name/description/spec object/status/created_at/updated_at）；`corepack pnpm generate:control-plane && corepack pnpm verify:contracts`。
- [ ] **Step 6: 测试通过 + 提交** `go test ./apps/control-plane/internal/scenariotemplate/ ./apps/control-plane/internal/api/ ./apps/control-plane/internal/authz/` 绿 → `feat(api): add scenario template read-only registry endpoints`

---

### Task 3: 项目绑定 scenario_template_key（后端全链）

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`（CreateProjectRequest + Project schema 加可空 `scenario_template_key`）+ regenerate
- Modify: `internal/project/types.go`（CreateProjectRequest + Project 加 `ScenarioTemplateKey *string`）、`handler.go:257` 映射、`service.go:133` 校验、`pg_repository.go:83` INSERT 参数、Project 行→结构映射处（读 GetProject 的 row mapping 函数补字段）
- Test: `internal/project/` 既有 service/handler 测试补用例

**Interfaces:**
- Consumes: Task 1 的 `CreateProjectParams.ScenarioTemplateKey`、Task 2 的 `Service.GetByKey`。
- Produces: `project.CreateProjectRequest.ScenarioTemplateKey *string`；`project.Project.ScenarioTemplateKey *string`（API 响应含之）；service 校验：key 非空时必须能在注册表解析且 status=active，否则 `ErrInvalidProject`（错误信息带 key）。校验依赖注入：`project.Service` 增可空字段 `scenarioTemplates ScenarioTemplateResolver`（`interface{ GetByKey(ctx, tenantID uuid.UUID, key string) (scenariotemplate.ScenarioTemplate, error) }` 以本地 interface 定义避免包循环——**project 包内定义窄接口，app.go 注入 scenariotemplate service**），nil 时跳过校验（测试兼容）。

- [ ] **Step 1: 失败测试**：service 层——绑定未知 key 创建项目返回 ErrInvalidProject；绑定 `ops_analysis`（mock resolver 返回 active）成功且 repository 收到 key；不带 key 行为不变。
- [ ] **Step 2: 实现**（openapi→gen→handler→service→repo→row mapping 全链）；`SetScenarioTemplateResolver` 或构造函数扩参照 project.Service 既有注入风格（先读 NewService 签名再定）。app.go 装配处把 Task 2 的 service 注入。
- [ ] **Step 3: 全包测试 + 提交** `feat(project): bind scenario template key at project creation`

---

### Task 4: 快照装载 + planner 注入 + template_key 校验闭环

**Files:**
- Modify: `internal/workflow/projectcoordination/planner.go`（CoordinationSnapshot 加字段 + 新类型）
- Modify: `internal/workflow/projectcoordination/project_store.go`（LoadProjectCoordinationSnapshot 装载 + ProjectStore 新可选依赖 `WithScenarioTemplateSource`）
- Modify: `internal/workflow/projectcoordination/openai_compatible_planner.go`（plannerPromptSnapshot 加字段；system prompt 加静态指令；Plan() 解码后校验 key）
- Modify: `internal/app/app.go`（注入 source）
- Test: `planner_test.go`/`openai_compatible_planner_test.go`/`project_store_test.go`

**Interfaces:**
- Produces:
```go
// planner.go
type ScenarioTemplateSnapshot struct {
	Key  string         `json:"key"`
	Name string         `json:"name"`
	Spec map[string]any `json:"spec"`
}
// CoordinationSnapshot 加：
ScenarioTemplate *ScenarioTemplateSnapshot `json:"scenario_template,omitempty"`
// project_store.go
type ScenarioTemplateSource interface {
	GetByKey(ctx context.Context, tenantID uuid.UUID, key string) (ScenarioTemplateSnapshot, error)
}
func (s *ProjectStore) WithScenarioTemplateSource(source ScenarioTemplateSource) *ProjectStore
```
- 装载规则（LoadProjectCoordinationSnapshot :353 前）：project 行 `ScenarioTemplateKey` 为空或 source 为 nil → snapshot.ScenarioTemplate=nil（= generic 语义，行为与今天一致）；key 非空但解析失败/disabled → **回落 nil + 记 warn 日志**（spec §5 的"回落 generic+事件"P1 简化为 warn，注入 nil 与 generic 等价；事件留 P2，Task 7 spec 修订注明）。注意：project.Project 需在快照装载可见 ScenarioTemplateKey——Task 3 已加。
- prompt：`plannerPromptSnapshot` 加 `ScenarioTemplate *ScenarioTemplateSnapshot json:"scenario_template,omitempty"`；system prompt（:272 静态文本）追加两句：
  - "When the snapshot contains scenario_template, instantiate its spec.skeleton as the task backbone: one task per skeleton step in order, honoring depends_on edges, seeding each task's produces from produces_defaults (names verbatim) and input_requirements.required_inputs from required_inputs_defaults; use spec.roles required_capabilities as the capability annotations, and fold spec.default_acceptance_criteria into plan_acceptance_criteria. You may add tasks the demand genuinely needs beyond the skeleton, but never drop or rename a skeleton step's produces names."
  - "Set template_key exactly to scenario_template.key when it is present; otherwise choose a short descriptive key."
- 校验闭环（openai_compatible_planner.go Plan() 内、图校验同层）：`snapshot.ScenarioTemplate != nil && plan.TemplateKey != snapshot.ScenarioTemplate.Key` → `invalidRouteDecision("plan template_key %q does not match bound scenario template %q", ...)`（进修复循环）。

- [ ] **Step 1: 失败测试**：①快照装载——绑定 key 的项目 LoadProjectCoordinationSnapshot 返回 ScenarioTemplate 非空（memory repo 补 GetProject 返回带 key 的 project + fake source）；解析失败回落 nil 不报错。②key 校验——构造 snapshot 带模板、plan.TemplateKey 不匹配 → ErrInvalidRouteDecision；匹配通过；无模板不校验。③prompt——`buildPlannerUserPrompt(snapshot含模板)` 输出包含 `"scenario_template"` 与 key。
- [ ] **Step 2: 实现**（注意 Plan() 里校验插入点在 `ValidateRouteDecisionPlan` 调用旁，两处：初始 :133 与修复 :145——抽成小函数复用）。
- [ ] **Step 3: 全包测试 + 提交** `feat(coordination): inject bound scenario template into planner and enforce template_key`

---

### Task 5: template_key 落 PlanRevisionPayload + Web 计划卡显示

**Files:**
- Modify: `internal/workflow/projectcoordination/plan_revision_payload.go`（:14 payload 结构加 `TemplateKey string json:"template_key,omitempty"`；Build/canonical 函数带上——先读 BuildPlanRevisionPayload 与 canonicalPlanRevision* 的对称结构）
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`（计划确认卡加一行）
- Test: `plan_revision_payload_test.go` + web 组件测试（若计划卡有测试文件则补断言，无则跳过 UI 单测、E2E 兜底）

**Interfaces:**
- Produces: plan revision `payload.template_key`（jsonb 内，无迁移）；Web 卡片在 payload.template_key 存在时渲染 `<RuntimeMeta label="场景模板" value={String(payload.template_key)} />`。

- [ ] **Step 1: Go 失败测试**：BuildPlanRevisionPayload(plan 带 TemplateKey) → payload.TemplateKey 相等；canonical 后仍在（fingerprint 稳定性测试若有需同步）。
- [ ] **Step 2: 实现 + Go 测试绿。**
- [ ] **Step 3: Web**：在计划确认卡 FactTile 区或状态行下加（数据取 `latestPlanRevision.payload?.["template_key"]`，仅存在时渲染）。`corepack pnpm --filter @superteam/web run typecheck`。
- [ ] **Step 4: 提交** `feat(coordination): carry template_key on plan revision payload and surface it on the plan card`

---

### Task 6: Web —— 场景模板目录页 + 侧栏 + 项目创建下拉

**Files:**
- Create: `apps/web/src/lib/api/scenario-templates.ts`
- Create: `apps/web/src/features/scenario-templates/index.tsx` + `index.test.tsx`
- Create: `apps/web/src/routes/_authenticated/scenario-templates/index.tsx`
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`（流程能力组加条目，icon 用 lucide `LayoutTemplate`）
- Modify: `apps/web/src/features/projects/components/create-project/create-project-draft.ts`（draft + buildProjectCreateInput）、基础信息步骤组件（加 Select）
- Modify: `apps/web/src/lib/api/projects.ts`（CreateProjectInput 加 `scenario_template_key?: string`）

**Interfaces:**
- Produces:
```ts
// lib/api/scenario-templates.ts
export type ScenarioTemplate = {
  id: string; tenant_id: string; template_key: string; name: string;
  description: string; spec: Record<string, unknown>; status: string;
  created_at: string; updated_at: string;
};
export function listScenarioTemplates(options: ApiClientOptions): Promise<ScenarioTemplate[]>;
```
- 页面（照 /mcp 骨架）：`ShellPageHeader icon={<LayoutTemplate/>} title="场景模板" subtitle="沉淀各类场景的分解骨架与交接契约，驱动规划实例化"` + 指标带（模板总数/启用数）+ 不透明 WorkSurface + V3Table，列：`模板/key/角色数/骨架步数/状态/更新时间(相对, tabular-nums)`；行可展开显示 roles 标题串与 skeleton 步骤链（`develop → review → test`）与 default_acceptance_criteria；四态齐全。只读——无创建/删除按钮。
- 项目创建：draft 加 `scenarioTemplateKey: string`（空串=不绑定）；基础步骤加 shadcn Select（选项 = useQuery listScenarioTemplates，含"不绑定（通用）"空项；照 submit-demand-dialog.tsx:127 写法）；`buildProjectCreateInput` 在非空时输出 `scenario_template_key`。

- [ ] **Step 1: 失败测试**（index.test.tsx，照 mcp/index.test.tsx 的 mock 套路）：mock listScenarioTemplates 返回两条 fixture → 断言页标题可见、表格含 `software_delivery` 行、状态 pill；空列表 → V3EmptyState 文案。
- [ ] **Step 2: 实现页面 + 路由壳 + 侧栏条目。**
- [ ] **Step 3: 项目创建接线**（draft/Select/build input/CreateProjectInput）。若 create-project 有既有测试断言 draft 形状，同步更新。
- [ ] **Step 4:** `corepack pnpm --filter @superteam/web test && corepack pnpm --filter @superteam/web run typecheck && corepack pnpm --filter @superteam/web run build`
- [ ] **Step 5: 提交** `feat(web): add scenario template directory page and project-create binding`

---

### Task 7: 门禁 + spec 状态修订 + CHANGELOG

- [ ] **Step 1:** `corepack pnpm verify:contracts && corepack pnpm verify:control-plane && corepack pnpm verify:web` 全绿；`make -C apps/control-plane migrate-validate` 复确认。
- [ ] **Step 2:** spec② 追加「P1 落地修订」小节：①P1"未绑定→不注入不校验"实现 generic 语义（generic 种子行保留供显式选择与 P2 消费）；②模板解析失败回落 = warn 日志（事件留 P2）；③种子骨架不带 task_kind（canonical 枚举约束，骨架由 step/role/produces 驱动）；④默认判据并入 plan_acceptance_criteria 由 prompt 指令承担，criterion 对象化属 intent spec。
- [ ] **Step 3:** CHANGELOG Unreleased 条目（`TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'` 生成时间戳），E2E 证据占位 Task 8 回填。
- [ ] **Step 4: 提交** `docs: record scenario template P1 amendments`

---

### Task 8: 真实 E2E（完成的必要条件）

阻塞规则同 CLAUDE.md。步骤：

- [ ] **Step 1:** `scripts/dev-services.sh restart control-plane`（自动跑迁移 058）+ `restart web`；psql 确认 `scenario_templates` 5 行种子、`projects.scenario_template_key` 列存在。
- [ ] **Step 2: 注册表 API/页面**：`GET /api/v1/scenario-templates` 返回 5 条；浏览器（playwright MCP）侧栏出现「场景模板」，页面渲染 5 行，展开 software_delivery 可见骨架链。截图存证。
- [ ] **Step 3: 绑定+模板驱动规划（spec §6-1）**：API 创建项目绑 `ops_analysis`（沿用 Handoff-E2E 的双员工+local-dev-node 配置）→ 提运维分析类需求 → 等计划生成：查库断言 ①plan revision `payload->>'template_key'='ops_analysis'`；②任务数=2 且依赖边 collect→analyze；③任务 PlannerMetadata produces 含 `raw_metrics`/`analysis_conclusion`（名字逐字）；④plan_acceptance_criteria 含模板默认判据。浏览器计划确认卡显示「场景模板 ops_analysis」。任务真实执行至 completed（上一轮 P1 的 upstream 注入应可见 raw_metrics 流入 analyze——顺带回归交接闭环）。
- [ ] **Step 4: 未绑定回归（spec §6-4）**：同项目形态不绑模板提一条简单需求 → 计划正常生成、template_key 为 planner 自选、行为与现状一致。
- [ ] **Step 5: 项目创建表单真实走查**：浏览器新建项目流程中模板下拉可选 `ops_analysis` 并成功创建（读库确认列值）。
- [ ] **Step 6:** 证据回填 CHANGELOG 与 spec 落地记录；`$superteam-completion-check` 清单过一遍；合并 main → 合并后确认 → 删分支。

## Self-Review

- **Spec 覆盖（P1 范围）**：§3.1 注册表映射（T1/T2）、§3.2 spec 结构（T1 种子，骨架实例化经 T4 prompt 指令）、§3.6 绑定点（T3+T6 表单，需求级覆盖=P2 不做）、§3.7 注入+key 闭环（T4）、§4 数据接口（T1/T2/T3，管理端点 P3 不做）、§5 错误处理（未绑/解析失败/key 不匹配三行落 T4，其余 P2）、§6-1/6-4 E2E（T8），§9 P1 三触点（T5 计划卡/T6 页面+下拉）。§3.3/3.4/3.5（可折叠/三档/feasibility）= P2，未纳入，正确。
- **占位符**：Task 2 Step 2"仿 capability 抄骨架"与 Task 3"先读 NewService 签名"是同 session 执行者的读前置指令；迁移 COMMENT"照 037 风格补齐"在执行时逐列写全。其余步骤代码/命令完整。
- **类型一致性**：`ScenarioTemplateSnapshot{Key,Name,Spec}` 在 T4 定义、T4 prompt/校验引用一致；`ScenarioTemplateSource` 接口返回该类型；web `ScenarioTemplate.template_key` 与后端列名/openapi 一致；`CreateProjectInput.scenario_template_key` 与 openapi/Go 字段一致。
