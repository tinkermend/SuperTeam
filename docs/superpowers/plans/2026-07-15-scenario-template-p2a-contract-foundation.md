# 场景模板 P2a（契约化地基）实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 把场景模板从"prompt 建议"升级为"服务端契约"：需求级引用、版本钉住、出口=交付物剪枝、骨架遵循校验、三种约束服务端强制、Plan 模式计划确认全量强制——并在中途用真实 planner 验证"拒绝→重规划收敛"生死判据。

**Architecture:** 全部构建在已 E2E 验证的现有管道上：模板快照装载（`project_store.go`）→ planner prompt 注入（`openai_compatible_planner.go`）→ 计划校验（`graph_validation.go`）→ `PlanRevisionPayload` 落库 → 确认卡（`project-operational-detail.tsx`）。新增：`scenariotemplate/spec.go` 类型化 spec v2 解析器、`projectcoordination` 内的剪枝/约束评估器、`scenario_template_versions` 版本表。

**Tech Stack:** Go (chi + sqlc + Temporal workflow) / Atlas migrations / OpenAPI (oapi-codegen, Go server only, TS 客户端手写) / React + TanStack Router + vitest browser mode。

**Spec:** `docs/superpowers/specs/2026-07-15-scenario-template-p2-contract-governance-design.md`（本计划只覆盖 §7 P2a；P2b——管理 API、casting 三档、词汇表、补员、豁免记录——待 Task 10 收敛闸门通过后另立计划）。

**本计划显式排除（除 P2b 外）：** planner 推断模板候选 + 确认卡上改选模板（P2a 只做需求显式 > 项目默认 > generic 三级解析）；Loop 信封；chat 高危拦截。

## Global Constraints

- 根级命令一律 `corepack pnpm <script>`；Go 测试用 `cd apps/control-plane && go test ./internal/...`（定向包）；Web 测试 `corepack pnpm --filter @superteam/web test`（需要时加 `--no-file-parallelism`）；**禁止** `npx playwright install`、`npx vitest run`。
- 迁移唯一目录 `apps/control-plane/internal/storage/migrations/`；最高编号 060，新迁移用 **061**；写完必须 `atlas migrate hash` 更新 `atlas.sum`，并 `make -C apps/control-plane migrate-validate`（本地可覆盖 `DEV_URL`）。表规则见 DATABASE_DESIGN.md：UUID-first、tenant_id 开头索引、TIMESTAMPTZ、JSONB、status 用 VARCHAR+应用层校验、**全部中文 COMMENT 强制**。
- 契约改动：编辑 `contracts/control-plane/openapi.yaml` 后跑 `corepack pnpm generate:control-plane`（只生成 Go），TS 客户端手写在 `apps/web/src/lib/api/`；提交前 `corepack pnpm verify:contracts`。
- 前端改动前必须读 `DESIGN.md`；内部跳转只用 TanStack Router `Link`/`navigate`。
- 服务启停 `scripts/dev-services.sh start|status|restart|stop`（必须在主 checkout 跑）；`start|restart control-plane` 自动执行迁移。
- 每个任务收尾提交；commit message 末尾带 `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。

---

### Task 1: 迁移 061 —— 版本表、种子 v2、需求级绑定列

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/061_scenario_template_versions_and_demand_binding.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`（经 atlas hash）

**Interfaces:**
- Produces: 表 `scenario_template_versions(id, tenant_id, template_id, version, spec, created_by, created_at)`；`scenario_templates.active_version INT`；`project_demands.scenario_template_key TEXT`；五个种子的 spec 升级为 v2 结构（含 `exits`/`constraints`/`collapse_rules`，`spec_version:2`），主表 `spec` 始终镜像 active 版本（读路径不需要 join，版本表做历史与审计）。

**设计要点（写进迁移注释）：** `exits` 数组按"由浅到深"声明，约束条件 `exit_at_or_beyond` 按该数组下标比较；`software_delivery` v2 新增 `release` 骨架步骤。

- [ ] **Step 1: 写迁移文件**

```sql
-- 场景模板版本表：模板 spec 的不可变历史快照；主表 spec 始终镜像 active 版本，
-- 版本表供审计血缘与（后续）计划钉住回读。exits 数组按由浅到深声明，
-- 约束条件 exit_at_or_beyond 按 exits 下标比较。
CREATE TABLE IF NOT EXISTS scenario_template_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    template_id UUID NOT NULL,
    version INT NOT NULL,
    spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_scenario_template_versions_template_version UNIQUE (template_id, version)
);
CREATE INDEX IF NOT EXISTS idx_scenario_template_versions_tenant_template
    ON scenario_template_versions(tenant_id, template_id, version DESC);

COMMENT ON TABLE scenario_template_versions IS '场景模板 spec 的不可变版本历史，供审计血缘与计划钉住';
COMMENT ON COLUMN scenario_template_versions.id IS '版本记录主键 UUID';
COMMENT ON COLUMN scenario_template_versions.tenant_id IS '所属租户 ID';
COMMENT ON COLUMN scenario_template_versions.template_id IS '所属场景模板 ID';
COMMENT ON COLUMN scenario_template_versions.version IS '版本号，从 1 单调递增';
COMMENT ON COLUMN scenario_template_versions.spec IS '该版本的完整模板 spec JSONB';
COMMENT ON COLUMN scenario_template_versions.created_by IS '创建该版本的用户 ID，迁移生成为 NULL';
COMMENT ON COLUMN scenario_template_versions.created_at IS '版本创建时间';

ALTER TABLE scenario_templates ADD COLUMN IF NOT EXISTS active_version INT NOT NULL DEFAULT 1;
COMMENT ON COLUMN scenario_templates.active_version IS '当前生效版本号，主表 spec 始终镜像该版本内容';

ALTER TABLE project_demands ADD COLUMN IF NOT EXISTS scenario_template_key TEXT;
COMMENT ON COLUMN project_demands.scenario_template_key IS '需求级场景模板 key；解析顺序：需求显式 > 项目默认 > generic 兜底';

-- 存量模板 spec 归档为 v1
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 1, spec FROM scenario_templates WHERE deleted_at IS NULL;
```

紧接着在同一文件里，对五个种子模板逐个执行"UPDATE 主表 spec 为 v2 literal + active_version=2，再 INSERT 对应 v2 版本行"。五个 v2 spec literal 全文如下（`software_delivery`，其余四个按同样模式，内容在下方给全）：

```sql
UPDATE scenario_templates SET active_version = 2, spec =
'{"spec_version":2,"roles":[{"key":"developer","title":"开发","required_capabilities":["code_implementation"]},{"key":"reviewer","title":"审查","required_capabilities":["code_review"]},{"key":"tester","title":"测试","required_capabilities":["test_execution"]}],"skeleton":[{"step":"develop","role":"developer","produces_defaults":[{"name":"branch_ref","kind":"branch_ref"},{"name":"head_commit","kind":"git_commit"}]},{"step":"review","role":"reviewer","depends_on":["develop"],"required_inputs_defaults":["head_commit"],"produces_defaults":[{"name":"review_verdict","kind":"conclusion"}]},{"step":"test","role":"tester","depends_on":["develop"],"required_inputs_defaults":["branch_ref"],"produces_defaults":[{"name":"test_report","kind":"conclusion"}]},{"step":"release","role":"developer","depends_on":["review","test"],"required_inputs_defaults":["review_verdict","test_report"],"produces_defaults":[{"name":"release_record","kind":"evidence_ref"}]}],"exits":[{"deliverable":"branch_ref","label":"交付分支（不合入）"},{"deliverable":"review_verdict","label":"审查通过并合入"},{"deliverable":"release_record","label":"发布上线"}],"constraints":[{"kind":"role_independence","roles":["reviewer","developer"],"when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"review","when":{"exit_at_or_beyond":"review_verdict"}},{"kind":"stage_required","step":"test","when":{"exit_at_or_beyond":"release_record"}},{"kind":"human_gate","target":"release","when":{"exit_at_or_beyond":"release_record"}}],"collapse_rules":[{"roles":["developer","tester"]}],"default_acceptance_criteria":[{"statement":"变更以 branch+commit 交付","applies_from_exit":"branch_ref"},{"statement":"通过独立审查","applies_from_exit":"review_verdict"},{"statement":"测试报告覆盖主路径且结论可判","applies_from_exit":"release_record"}],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}'::jsonb
WHERE id = '00000000-0000-0000-0000-000000000401'::uuid;
INSERT INTO scenario_template_versions (tenant_id, template_id, version, spec)
SELECT tenant_id, id, 2, spec FROM scenario_templates WHERE id = '00000000-0000-0000-0000-000000000401'::uuid;
```

其余四个种子的 v2 spec（照上面模式各写一组 UPDATE+INSERT，种子 id 见迁移 058：ops_analysis=…0402, incident_response=…0403, research_report=…0404, generic=…0405）：

- `ops_analysis`：roles collector/analyst（能力不变，去掉 v1 的 collapsible_with/independent_from 字段）；skeleton collect→analyze 不变；`"exits":[{"deliverable":"raw_metrics","label":"仅采集数据"},{"deliverable":"analysis_conclusion","label":"给出分析结论"}]`；`"constraints":[]`；`"collapse_rules":[{"roles":["collector","analyst"]}]`；criteria=`[{"statement":"结论附证据指针，可追溯到采集数据","applies_from_exit":"analysis_conclusion"}]`。
- `incident_response`：roles diagnostician/operator/verifier；skeleton diagnose→fix→verify 不变；`"exits":[{"deliverable":"root_cause","label":"仅诊断根因"},{"deliverable":"fix_record","label":"实施修复"},{"deliverable":"verification_result","label":"修复并独立验证"}]`；`"constraints":[{"kind":"role_independence","roles":["verifier","operator"],"when":{"exit_at_or_beyond":"verification_result"}},{"kind":"stage_required","step":"verify","when":{"exit_at_or_beyond":"verification_result"}}]`；`"collapse_rules":[{"roles":["diagnostician","operator"]}]`；criteria 两条对应挂 `fix_record`/`verification_result`。
- `research_report`：exits=`[{"deliverable":"source_list","label":"仅出来源清单"},{"deliverable":"final_report","label":"成稿"}]`；constraints=[]；collapse_rules=`[{"roles":["researcher","writer"]}]`；criteria 挂 `final_report`。
- `generic`：`{"spec_version":2,"roles":[],"skeleton":[],"exits":[],"constraints":[],"collapse_rules":[],"default_acceptance_criteria":[],"feasibility_thresholds":{"pass":0.8,"degrade":0.5}}`。

- [ ] **Step 2: 更新 atlas.sum 并校验**

Run: `cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations && make migrate-validate`
Expected: validate 通过（本地无 docker 时 `make migrate-validate DEV_URL=<本地一次性库 URL>`）。

- [ ] **Step 3: 应用到本地 dev 库并抽查**

Run: `cd apps/control-plane && make migrate-up`，然后
`psql "$DATABASE_URL" -c "SELECT template_key, active_version, spec->>'spec_version' FROM scenario_templates ORDER BY template_key;"`
Expected: 五行，active_version=2，spec_version=2；`SELECT count(*) FROM scenario_template_versions;` = 10（5 个 v1 + 5 个 v2）。

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/
git commit -m "feat(db): 场景模板版本表+种子v2(约束/出口)+需求级绑定列 (迁移061)"
```

---

### Task 2: spec v2 类型与解析器（TDD）

**Files:**
- Create: `apps/control-plane/internal/scenariotemplate/spec.go`
- Test: `apps/control-plane/internal/scenariotemplate/spec_test.go`

**Interfaces:**
- Produces（后续任务全部依赖这些精确签名）:
```go
package scenariotemplate

type SpecProduce struct { Name string `json:"name"`; Kind string `json:"kind,omitempty"` }
type SpecRole struct { Key, Title string; RequiredCapabilities []string `json:"required_capabilities"` }
type SpecSkeletonStep struct {
    Step string `json:"step"`; Role string `json:"role"`
    DependsOn []string `json:"depends_on,omitempty"`
    ProducesDefaults []SpecProduce `json:"produces_defaults,omitempty"`
    RequiredInputsDefaults []string `json:"required_inputs_defaults,omitempty"`
}
type SpecExit struct { Deliverable string `json:"deliverable"`; Label string `json:"label"` }
type SpecConstraintWhen struct { ExitAtOrBeyond string `json:"exit_at_or_beyond,omitempty"` } // 空 = 无条件
type SpecConstraint struct {
    Kind string `json:"kind"` // 注册于 knownConstraintKinds: role_independence | stage_required | human_gate
    Roles []string `json:"roles,omitempty"`; Step string `json:"step,omitempty"`; Target string `json:"target,omitempty"`
    When SpecConstraintWhen `json:"when,omitempty"`
}
type SpecCollapseRule struct { Roles []string `json:"roles"` }
type SpecAcceptanceCriterion struct { Statement string `json:"statement"`; AppliesFromExit string `json:"applies_from_exit,omitempty"` }
type SpecV2 struct {
    SpecVersion int `json:"spec_version"`
    Roles []SpecRole `json:"roles"`; Skeleton []SpecSkeletonStep `json:"skeleton"`
    Exits []SpecExit `json:"exits"`; Constraints []SpecConstraint `json:"constraints"`
    CollapseRules []SpecCollapseRule `json:"collapse_rules"`
    DefaultAcceptanceCriteria []SpecAcceptanceCriterion `json:"default_acceptance_criteria"`
    FeasibilityThresholds map[string]float64 `json:"feasibility_thresholds,omitempty"`
    BudgetProfile map[string]any `json:"budget_profile,omitempty"`
}
func ParseSpec(raw map[string]any) (SpecV2, error)
func (s SpecV2) ExitIndex(deliverable string) int // 在 Exits 中的下标，未找到 -1
func (s SpecV2) StepByProduce(name string) (SpecSkeletonStep, bool)
```

- [ ] **Step 1: 写失败测试**（`spec_test.go`）——四个用例：
  1. `TestParseSpecV2` — 用 Task 1 的 software_delivery v2 literal（json.Unmarshal 到 map 后传入），断言 `SpecVersion==2`、4 个 skeleton 步、3 个 exits、4 条 constraints、`ExitIndex("review_verdict")==1`、`StepByProduce("release_record").Step=="release"`。
  2. `TestParseSpecV1Normalizes` — 用迁移 058 的 v1 software_delivery literal，断言归一化：`roles[].independent_from` → `SpecConstraint{Kind:"role_independence", Roles:[该角色, 对方角色]}`（无条件）；`collapsible_with` → `CollapseRules`；字符串判据 → `SpecAcceptanceCriterion{Statement: s}`；`risk_policy.release_requires_human=true` 且存在名为 release 的 step 时 → `human_gate`（v1 种子无 release step，断言不产生该约束）。
  3. `TestParseSpecUnknownConstraintKind` — constraints 含 `{"kind":"made_up"}` → error 含 `"unknown constraint kind"`。
  4. `TestParseSpecEmptyIsGeneric` — `{}` → 零值 SpecV2、无 error（generic 兜底语义）。

- [ ] **Step 2: 跑测试确认失败**

Run: `cd apps/control-plane && go test ./internal/scenariotemplate/ -run TestParseSpec -v`
Expected: FAIL（`ParseSpec` 未定义）。

- [ ] **Step 3: 实现 `spec.go`** —— 核心：`json.Marshal(raw)` 再 `json.Unmarshal` 到 SpecV2；若 `spec_version < 2` 走 `normalizeV1(raw)`（手工提取 `roles[].independent_from`/`collapsible_with`、字符串判据、`risk_policy`）；`knownConstraintKinds = map[string]bool{"role_independence":true,"stage_required":true,"human_gate":true}`（包级 var，注释注明"注册表：新增 kind = 此处加 evaluator，见 projectcoordination/template_governance.go"）；解析后校验 constraints 的 kind 已注册、`when.exit_at_or_beyond` 若非空必须命中某 exit 的 deliverable（v1 归一化产物除外）。

- [ ] **Step 4: 跑测试确认通过**

Run: 同 Step 2。Expected: PASS。

- [ ] **Step 5: Commit** — `git add apps/control-plane/internal/scenariotemplate/ && git commit -m "feat(scenariotemplate): spec v2 类型化解析器含 v1 归一化"`

---

### Task 3: 版本号贯通快照（sqlc 再生成 + snapshot Version）

**Files:**
- Modify: `apps/control-plane/internal/scenariotemplate/types.go`（ScenarioTemplate 加 `ActiveVersion int`）
- Modify: `apps/control-plane/internal/scenariotemplate/pg_repository.go`（`scenarioTemplateFromRow` 映射新列）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go:25-29`（`ScenarioTemplateSnapshot` 加 `Version int \`json:"version,omitempty"\``）
- Modify: `apps/control-plane/internal/app/app.go:769-778`（适配器映射 `Version: template.ActiveVersion`）
- Test: `apps/control-plane/internal/scenariotemplate/` 既有测试 + `go build ./...`

**Interfaces:**
- Consumes: Task 1 的 `active_version` 列。
- Produces: `ScenarioTemplateSnapshot{Key, Name, Version, Spec}`——Task 5/6/7 读 `snapshot.ScenarioTemplate.Version`。

- [ ] **Step 1: 再生成 sqlc**（migrations 即 schema 源）：`rg -n "sqlc" apps/control-plane/Makefile` 找到生成目标并执行（无目标则 `cd apps/control-plane && sqlc generate`）。Expected: `queries/models.go` 的 `ScenarioTemplate` 获得 `ActiveVersion` 字段，`scenario_template.sql.go` 的 SELECT * 行随之更新。
- [ ] **Step 2: 编译驱动修改**：`go build ./...` 按报错依次补 `types.go`、`scenarioTemplateFromRow`、`ScenarioTemplateSnapshot`、app.go 适配器四处。
- [ ] **Step 3: 跑包测试**：`go test ./internal/scenariotemplate/ ./internal/workflow/projectcoordination/ -count=1`。Expected: PASS（既有测试对新字段零值不敏感）。
- [ ] **Step 4: Commit** — `feat(scenariotemplate): active_version 贯通到规划快照`

---

### Task 4: 需求级模板键（后端 + 契约）

**Files:**
- Modify: `apps/control-plane/internal/project/types.go:1205-1221`（`ProjectDemand` 加 `ScenarioTemplateKey *string`）
- Modify: 需求创建/读取的 sqlc 查询（`rg -n "INSERT INTO project_demands" apps/control-plane/internal/storage/queries/` 定位查询文件与名字，加列）+ 再生成
- Modify: `apps/control-plane/internal/project/service.go` 与 `handler.go` 的需求提交路径（`rg -n "SubmitDemand\|CreateDemand" internal/project/` 定位；请求结构体加 `ScenarioTemplateKey string \`json:"scenario_template_key,omitempty"\``，service 落库前 TrimSpace、非空时校验该 key 在租户内存在且 active——用 `scenariotemplate.Service.GetByKey`，不存在返回 400）
- Modify: `contracts/control-plane/openapi.yaml`（需求提交请求 schema 加可选 `scenario_template_key: {type: string, description: 需求级场景模板 key；缺省回落项目默认}`；需求响应 schema 同步加）
- Test: `apps/control-plane/internal/project/` 就近既有 handler/service 测试模式加用例

**Interfaces:**
- Produces: `ProjectDemand.ScenarioTemplateKey *string`——Task 5 的解析顺序消费；OpenAPI `scenario_template_key` 字段——Task 12 web 表单消费。

- [ ] **Step 1: 写失败测试**：service 层用例 `TestSubmitDemandRejectsUnknownScenarioTemplateKey`（仿同文件既有 demand 测试 fixture）：提交带 `scenario_template_key:"nope"` 的需求 → error；带 `"ops_analysis"`（mock template source 返回存在）→ 落库字段持久化。
- [ ] **Step 2: 跑测试失败** → **Step 3: 实现**（类型/查询/service 校验/handler 透传/openapi + `corepack pnpm generate:control-plane`）→ **Step 4: 测试通过 + `corepack pnpm verify:contracts`** → **Step 5: Commit** — `feat(project): 需求级场景模板键（API+校验+持久化）`

---

### Task 5: 快照解析顺序 + 解析失败项目事件

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go:371-383`（装载逻辑改三级解析）
- Modify: `apps/control-plane/internal/project/types.go`（新增 `ProjectEventScenarioTemplateResolutionFailed ProjectEventType = "scenario_template.resolution_failed"`，与 :178 的常量并列）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go:31-35`（`DemandSnapshot` 加 `ScenarioTemplateKey string \`json:"scenario_template_key,omitempty"\``，装载处从 demand 记录填充）
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store` 既有快照装载测试旁新增

**Interfaces:**
- Consumes: Task 4 的 `ProjectDemand.ScenarioTemplateKey`；既有 `coordinatorEvent(...)`（project_store.go:3176）与 `s.repository.AppendProjectEvent`。
- Produces: 解析顺序语义——需求 key 非空用需求的，否则项目的，否则 nil（generic）；任一级解析失败（不存在/disabled）→ 落 `scenario_template.resolution_failed` 项目事件（payload 带 `requested_key`、`source: demand|project`）并继续降级，不阻断规划。

- [ ] **Step 1: 失败测试**：`TestLoadSnapshotPrefersDemandTemplateKey`（demand key=research_report、project key=software_delivery → snapshot.ScenarioTemplate.Key=="research_report"）；`TestLoadSnapshotResolutionFailureEmitsProjectEvent`（source 返回 error → snapshot.ScenarioTemplate==nil 且 AppendProjectEvent 被调、事件类型正确——仿既有测试的 repository fake）。
- [ ] **Step 2: 失败** → **Step 3: 实现**（把 :378 的 `log.Printf` 分支替换为 `s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventScenarioTemplateResolutionFailed, key, "场景模板解析失败，回落 generic", map[string]any{"requested_key": key, "source": source}))` + 保留降级）→ **Step 4: 通过** → **Step 5: Commit** — `feat(coordination): 需求级模板解析顺序+解析失败项目事件`

---

### Task 6: 出口字段贯通 planner 与 payload

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go:53-61`（`RouteDecisionPlan` 加 `ExitDeliverable string`、`TemplateVersion int`、`AvailableExits []PlanExitOption`、`ConstraintNotes []PlanConstraintNote`；新类型 `PlanExitOption{Deliverable, Label string}`、`PlanConstraintNote{Kind, Message string}` 定义在同文件）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/openai_compatible_planner.go`（:276 schema 行加 `exit_deliverable string`；:287 骨架指令重写——见 Step 3；:329 处解码映射；:368 `plannerJSON` 加 `ExitDeliverable string \`json:"exit_deliverable"\``；:311-318 `plannerPromptSnapshot` 加 `PinnedExitDeliverable string \`json:"pinned_exit_deliverable,omitempty"\``）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go:12-22`（`CoordinationSnapshot` 加 `PinnedExitDeliverable string \`json:"pinned_exit_deliverable,omitempty"\``——Task 11 回灌用，本任务只加字段）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/plan_revision_payload.go:14-23`（`PlanRevisionPayload` 加 `TemplateVersion int \`json:"template_version,omitempty"\``、`ExitDeliverable string \`json:"exit_deliverable,omitempty"\``、`AvailableExits []PlanExitOption \`json:"available_exits,omitempty"\``、`ConstraintNotes []PlanConstraintNote \`json:"constraint_notes,omitempty"\``；`BuildPlanRevisionPayload` :137 附近逐一复制；`canonicalPlanRevisionPayload` :285-308 纳入 ExitDeliverable 与 TemplateVersion——available_exits/constraint_notes 是服务端注记，**不纳入指纹**，加注释说明）
- Test: `plan_revision_payload_test.go`（仿 :255 `TestBuildPlanRevisionPayloadCarriesTemplateKey`）

**Interfaces:**
- Produces: `RouteDecisionPlan.ExitDeliverable/TemplateVersion/AvailableExits/ConstraintNotes`（Task 7/8 填充与校验）；payload 同名字段（Task 12 前端读 `revision.payload["exit_deliverable"]` 等）。

- [ ] **Step 1: 失败测试**：`TestBuildPlanRevisionPayloadCarriesExitAndVersion` — plan 设 `ExitDeliverable:"review_verdict", TemplateVersion:2, AvailableExits:[{“branch_ref”,“交付分支（不合入）”}], ConstraintNotes:[{Kind:"human_gate",Message:"发布任务已强制人类审批"}]` → payload JSON 含四字段；`TestCanonicalPlanFingerprintIgnoresConstraintNotes` — 仅 notes 不同的两 payload 指纹相同。
- [ ] **Step 2: 失败** → **Step 3: 实现**。:287 的骨架指令行替换为（一整行英文 prompt，保持既有风格）：

```
"When the snapshot contains scenario_template, first choose exit_deliverable: exactly one deliverable name from scenario_template.spec.exits that best matches how far the demand asks to go (if snapshot.pinned_exit_deliverable is set, you MUST use it verbatim). Then instantiate ONLY the skeleton steps in the dependency-ancestor closure of the step producing that deliverable: one task per included step in order, honoring depends_on edges, seeding each task's produces from that step's produces_defaults (names verbatim) and its input_requirements.required_inputs from required_inputs_defaults; use the matching spec.roles required_capabilities as capability annotations and fold the spec.default_acceptance_criteria whose applies_from_exit is at or before your chosen exit into plan_acceptance_criteria. You may add tasks the demand genuinely needs beyond the skeleton, but never drop an included skeleton step or rename its produces names. Every skeleton-derived task is still a full task object: include ALL required task fields exactly as for any other task.",
```

→ **Step 4: 通过**（`go test ./internal/workflow/projectcoordination/ -count=1`）→ **Step 5: Commit** — `feat(planner): exit_deliverable 贯通 plan/prompt/payload`

---

### Task 7: 骨架剪枝 + 遵循校验（TDD）

**Files:**
- Create: `apps/control-plane/internal/workflow/projectcoordination/template_governance.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go:55-86`（`ValidateRouteDecisionPlan` 在 :59-62 模板 key 检查后追加骨架遵循校验调用）
- Test: `apps/control-plane/internal/workflow/projectcoordination/template_governance_test.go`

**Interfaces:**
- Consumes: `scenariotemplate.ParseSpec`（Task 2）、`snapshot.ScenarioTemplate.Spec`、`plan.ExitDeliverable`、既有 helper `validGraphPlan`/`planTaskWithIO`/`validationSnapshotWithProfile`（graph_validation_test.go:552/576/607）。
- Produces:
```go
func pruneSkeletonForExit(spec scenariotemplate.SpecV2, exitDeliverable string) ([]scenariotemplate.SpecSkeletonStep, error)
func validateSkeletonAdherence(spec scenariotemplate.SpecV2, plan RouteDecisionPlan) error
```

- [ ] **Step 1: 失败测试**（software_delivery v2 spec 作 fixture 常量）：
  1. `TestPruneSkeletonForExit` — exit=branch_ref → 仅 develop；exit=review_verdict → develop+review；exit=release_record → 全部 4 步；exit 不在 exits → error。
  2. `TestValidateSkeletonAdherenceMissingStep` — exit=review_verdict 但 plan 无任务 produces `review_verdict` → `ErrInvalidRouteDecision` 且 message 含 `skeleton step "review"`。
  3. `TestValidateSkeletonAdherencePasses` — 用 `planTaskWithIO` 造 develop(produces branch_ref+head_commit) 与 review(produces review_verdict, blockedBy develop, requires head_commit) 两任务 → NoError。
  4. `TestValidateSkeletonAdherenceRequiresExit` — 模板有 exits 而 `plan.ExitDeliverable==""` → error 含 `exit_deliverable required`。
  5. `TestValidateSkeletonAdherenceGenericNoop` — 空 skeleton spec → NoError（任意 plan）。
- [ ] **Step 2: 失败** → **Step 3: 实现**：

```go
func pruneSkeletonForExit(spec scenariotemplate.SpecV2, exitDeliverable string) ([]scenariotemplate.SpecSkeletonStep, error) {
    if spec.ExitIndex(exitDeliverable) < 0 {
        return nil, fmt.Errorf("exit deliverable %q is not a declared exit", exitDeliverable)
    }
    target, ok := spec.StepByProduce(exitDeliverable)
    if !ok { return nil, fmt.Errorf("no skeleton step produces %q", exitDeliverable) }
    byStep := map[string]scenariotemplate.SpecSkeletonStep{}
    for _, s := range spec.Skeleton { byStep[s.Step] = s }
    included := map[string]bool{}
    var visit func(step scenariotemplate.SpecSkeletonStep)
    visit = func(step scenariotemplate.SpecSkeletonStep) {
        if included[step.Step] { return }
        included[step.Step] = true
        for _, dep := range step.DependsOn { if d, ok := byStep[dep]; ok { visit(d) } }
    }
    visit(target)
    var pruned []scenariotemplate.SpecSkeletonStep
    for _, s := range spec.Skeleton { if included[s.Step] { pruned = append(pruned, s) } } // 保持声明序
    return pruned, nil
}

func validateSkeletonAdherence(spec scenariotemplate.SpecV2, plan RouteDecisionPlan) error {
    if len(spec.Skeleton) == 0 { return nil }
    if len(spec.Exits) > 0 && strings.TrimSpace(plan.ExitDeliverable) == "" {
        names := make([]string, 0, len(spec.Exits))
        for _, e := range spec.Exits { names = append(names, e.Deliverable) }
        return invalidRouteDecision("exit_deliverable required: template declares exits %v", names)
    }
    steps := spec.Skeleton
    if len(spec.Exits) > 0 {
        pruned, err := pruneSkeletonForExit(spec, plan.ExitDeliverable)
        if err != nil { return invalidRouteDecision("invalid exit_deliverable %q: %v", plan.ExitDeliverable, err) }
        steps = pruned
    }
    producedBy := map[string]string{} // produce name -> task key（全局唯一性已由 ValidateRouteDecisionGraph 保证）
    for _, task := range plan.Tasks { for _, p := range task.Produces { producedBy[p] = task.Key } }
    for _, step := range steps {
        for _, p := range step.ProducesDefaults {
            if _, ok := producedBy[p.Name]; !ok {
                return invalidRouteDecision("skeleton step %q deliverable %q missing from plan produces", step.Step, p.Name)
            }
        }
    }
    return nil
}
```

在 `ValidateRouteDecisionPlan` 的模板 key 检查（:62）后插入：

```go
if snapshot.ScenarioTemplate != nil {
    spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
    if err != nil { return invalidRouteDecision("scenario template spec unparsable: %v", err) }
    if err := validateSkeletonAdherence(spec, plan); err != nil { return err }
    if snapshot.PinnedExitDeliverable != "" && plan.ExitDeliverable != snapshot.PinnedExitDeliverable {
        return invalidRouteDecision("plan exit %q does not honor human-pinned exit %q", plan.ExitDeliverable, snapshot.PinnedExitDeliverable)
    }
}
```

- [ ] **Step 4: 通过**（`go test ./internal/workflow/projectcoordination/ -count=1`；既有 `TestValidateRouteDecisionPlanEnforcesBoundScenarioTemplateKey` 的 snapshot 无 Spec ——确认 `ParseSpec(nil map)` 走 generic 空值不误伤，必要时给该测试的 snapshot 补空 Spec）→ **Step 5: Commit** — `feat(validation): 出口剪枝+骨架遵循服务端校验`

---

### Task 8: 约束评估器 + governance 接线（TDD）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/template_governance.go`（追加评估器）
- Modify: `ValidateRouteDecisionPlan` 的每个生产调用点之后接线（定位：`rg -n "ValidateRouteDecisionPlan(" apps/control-plane --glob '!*_test.go'`——预期在 route decision 持久化/规划管线内，模式与 `ApplyPlanningProfileScores`（graph_validation.go:88）的调用点一致）
- Test: `template_governance_test.go` 追加

**Interfaces:**
- Consumes: Task 7 的 `pruneSkeletonForExit`；`SpecConstraint`（Task 2）；`plan.Tasks[].SelectedEmployeeID`。
- Produces:
```go
// EnforceScenarioTemplateGovernance 在 ValidateRouteDecisionPlan 通过后调用；
// 对 plan 施加模板约束：违反 → error；human_gate → 强制任务 RequiresHumanApproval；
// 折叠命中 → 服务端生成降级标注并置 plan.RequiresHumanReview。
// 同时把 TemplateVersion/AvailableExits/ConstraintNotes 写回 plan（payload 数据源）。
func EnforceScenarioTemplateGovernance(snapshot CoordinationSnapshot, plan *RouteDecisionPlan) error
```

- [ ] **Step 1: 失败测试**（沿用 software_delivery v2 fixture；员工 A/B 两个 uuid）：
  1. `TestGovernanceRoleIndependenceViolation` — exit=review_verdict，develop 与 review 任务同为员工 A → `ErrInvalidRouteDecision`，message 含 `role_independence` 与两角色名。
  2. `TestGovernanceRoleIndependencePassesWithTwoEmployees` — review 归员工 B → NoError。
  3. `TestGovernanceRoleIndependenceNotTriggeredBelowExit` — exit=branch_ref（review 不在剪枝集）→ 同员工 NoError（`when.exit_at_or_beyond` 未命中）。
  4. `TestGovernanceHumanGateForcesApproval` — exit=release_record、全链任务 → release 步骤任务 `RequiresHumanApproval==true`（即使 planner 置 false），`ConstraintNotes` 含 `{Kind:"human_gate"}`。
  5. `TestGovernanceCollapseNoteAnnotates` — developer 与 tester 同员工（collapse_rules 命中）→ NoError 但 `plan.RequiresHumanReview==true` 且 notes 含 `{Kind:"collapse"}`、message 含两角色标题。
  6. `TestGovernancePopulatesVersionAndExits` — 执行后 `plan.TemplateVersion==snapshot.ScenarioTemplate.Version`、`AvailableExits` 与 spec.Exits 等长同序。
- [ ] **Step 2: 失败** → **Step 3: 实现**要点：
  - 条件判定 `exitCondMet(spec, cond SpecConstraintWhen, exit string) bool`：cond 空 → true；否则 `spec.ExitIndex(exit) >= spec.ExitIndex(cond.ExitAtOrBeyond)`（exit 未知时按 true 处理，宁严勿漏）。
  - 角色→员工映射：对剪枝集内每个 step，用第一条 produces_defaults 名经 `producedBy` 找任务 → `SelectedEmployeeID`；一个角色可能对应多 step，聚成 `map[roleKey]map[uuid.UUID]bool`。
  - `role_independence`：两角色员工集合有交集 → `invalidRouteDecision("constraint role_independence violated: roles %v share employee %s", c.Roles, id)`。
  - `stage_required`：目标 step 不在剪枝集或无对应任务 → 违反（剪枝通常已保证祖先，本约束防出口序声明与依赖图不一致的模板数据）。
  - `human_gate`：target step 的任务 `RequiresHumanApproval=true` + note（message 格式：`发布任务已强制人类审批：由 human_gate@<key> v<version> 触发`）。
  - collapse：**服务端生成**标注（不要求 planner 自报——比 spec §3.4-3 的"缺标注拒绝"更强且不可遗漏，实现处注释说明此偏差），note.Kind="collapse"，`plan.RequiresHumanReview=true`。
  - 末尾填 `plan.TemplateVersion`、`plan.AvailableExits`（映射 spec.Exits）。
  - 接线：每个 `ValidateRouteDecisionPlan` 生产调用点成功后立即 `EnforceScenarioTemplateGovernance(snapshot, &plan)`，error 与校验错误同路径处理。
- [ ] **Step 4: 通过** → **Step 5: Commit** — `feat(validation): 模板约束评估器（四眼/强制阶段/人类门/折叠标注）服务端强制`

---

### Task 9: Plan 模式计划确认全量强制

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go:482-487`（`PersistPlanRevision` 的 status 决定逻辑）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go:31-35`（`DemandSnapshot` 加 `CoordinationMode string \`json:"coordination_mode,omitempty"\``，装载处从 demand 记录的 `CoordinationMode` 填充——列已存在于迁移 059）
- Test: 既有 `project_store` / `workflow` 测试更新

**Interfaces:**
- Consumes: `ProjectDemand.CoordinationMode`（既有字段）。
- Produces: 语义——`coordination_mode` 为空或 `"plan"` 的需求，计划一律 `PendingReview`（`Accepted` 自动派发路径仅保留给 loop/chat 模式，待 Loop 信封 spec 接管）；`ValidationFailed` 分支不变。

- [ ] **Step 1: 失败测试**：`TestPersistPlanRevisionPlanModeAlwaysPendingReview` — RequiresHumanReview=false、validation 通过、mode="plan" → status==`PlanRevisionStatusPendingReview`；`TestPersistPlanRevisionLoopModeKeepsAccepted` — mode="loop" 同条件 → `Accepted`。
- [ ] **Step 2: 失败** → **Step 3: 实现**：status 逻辑改为

```go
status := project.PlanRevisionStatusPendingReview
if !validation.Acceptable {
    status = project.PlanRevisionStatusValidationFailed
} else if isAutonomousCoordinationMode(input.CoordinationMode) && !validation.ReviewRequired && !input.Decision.RequiresHumanReview {
    status = project.PlanRevisionStatusAccepted // loop/chat 暂保现状，Loop 信封 spec 接管
}
// func isAutonomousCoordinationMode(mode string) bool { mode = strings.TrimSpace(mode); return mode == "loop" || mode == "chat" }
```

`PersistPlanRevision` 的 input 结构体加 `CoordinationMode string`，workflow 调用处从 `snapshot.Demand.CoordinationMode` 传入。
- [ ] **Step 4: 全包测试并修复受影响断言**：`go test ./internal/workflow/projectcoordination/ -count=1` ——原先断言自动派发（Accepted）的 plan 模式用例改为断言 PendingReview 后走 `handlePlanReviewDecision` 批准路径。Expected: 全绿。
- [ ] **Step 5: Commit** — `feat(coordination): plan 模式计划确认全量强制（loop/chat 暂保现状）`

---

### Task 10: 【GATE·生死判据】真实 planner 拒绝→重规划收敛 E2E

**Files:** 无代码变更；产出验证记录（追加到本计划文件末尾"实施记录"节）。

**前置：** Task 1–9 已合入工作分支；`scripts/dev-services.sh status` 确认 Temporal/Control Plane/Web/Runtime 全绿（在主 checkout 跑；control-plane restart 会自动跑迁移 061）。

- [ ] **Step 1: 造实验场**：浏览器（codex chrome plug）登录 `http://localhost:3000`（admin/admin）→ 创建项目绑定 `software_delivery`，项目池配 2 名可调度数字员工（A 具 code_implementation、B 具 code_review；不就绪则按既有员工创建流补，执行实例可直插 DB 造——见 worktree E2E 环境事实）。
- [ ] **Step 2: 触发真实规划**：项目内提交需求（plan 模式）："给 demo 仓库的 CLI 增加 --version 参数并通过审查合入"。观察计划生成。
- [ ] **Step 3: 测量收敛（判决点）**：`psql "$DATABASE_URL"` 查 route decisions / plan revisions（或 `curl -s --cookie <会话> http://localhost:8080/api/v1/projects/{projectId}/plan-revisions | jq '.[].status'`）。统计从需求提交到出现 `pending_review` 版本之间被 `ErrInvalidRouteDecision` 家族拒绝的规划轮数（control-plane 日志 `rg "invalid route decision" logs/`）。
  - **通过标准**：≤2 轮内产出通过全部新校验（骨架遵循+约束+出口）的 pending_review 计划，payload 含 `template_key/template_version/exit_deliverable/available_exits`。
  - **失败处置**：>2 轮或死循环 → **停止后续任务**，记录失败样本（planner 原始输出+拒绝理由），回到 spec 评审"放宽校验 or 加固 prompt or 引入结构化重试反馈"，不得带病推进 Task 11/12。
- [ ] **Step 4: 顺手验证三条判据**：① 同需求换措辞"只出分支不用合入" → 计划 exit=branch_ref 且无 review 任务；② 单员工项目提"合入"需求 → 规划期被 role_independence 拒绝（当前无补员出路，确认拒绝理由结构化可读即可，缺口报告 UI 是 P2b）；③ 批准 pending_review 计划 → 正常派发执行。
- [ ] **Step 5: 记录**：把轮数、样本、三条判据结果写进本文件"实施记录"节并 commit — `docs(plan): P2a 收敛闸门 E2E 记录`

---

### Task 11: 出口改选回灌（确认卡 request_changes 钉住出口）

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`（`HumanDecisionSubmitted` 载荷加 `TargetExitDeliverable string \`json:"target_exit_deliverable,omitempty"\``）
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go:417-486`（`replanAfterPlanReviewChanges` 在 `LoadProjectCoordinationSnapshot` 后置 `snapshot.PinnedExitDeliverable = signal.TargetExitDeliverable`）
- Modify: 决策解析链路把字段从 Console 请求透传到信号：`rg -n "HumanDecisionSubmitted{" apps/control-plane/internal --glob '!*_test.go'` 定位构造点（project service 的决策解析路径），其请求 DTO 与 `contracts/control-plane/openapi.yaml` 对应 schema 加同名可选字段，`corepack pnpm generate:control-plane`
- Test: workflow 测试 + service 透传测试

**Interfaces:**
- Consumes: Task 6 的 `CoordinationSnapshot.PinnedExitDeliverable` 与 prompt 注入、Task 7 的 pinned 校验。
- Produces: OpenAPI 决策请求字段 `target_exit_deliverable`（Task 12 前端提交用）。

- [ ] **Step 1: 失败测试**：workflow 层 `TestReplanAfterExitOverridePinsExit` — request_changes 信号带 `TargetExitDeliverable:"branch_ref"` → 重规划调用的 snapshot `PinnedExitDeliverable=="branch_ref"`（用既有 workflow 测试的 activity mock 捕获参数）。
- [ ] **Step 2: 失败** → **Step 3: 实现**（信号结构体、replan 注入、service/handler/openapi 透传）→ **Step 4: 通过 + `corepack pnpm verify:contracts`** → **Step 5: Commit** — `feat(coordination): 确认卡改选出口经 request_changes 钉住重规划`

---

### Task 12: Web —— 需求表单模板选择 + 确认卡扩展

**改动前必读 `DESIGN.md`。**

**Files:**
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`（计划确认面板 :386-395、场景模板 row :454-459、payload helper :2125-2128）
- Modify: 需求提交表单（`rg -ln "提交需求\|submitDemand" apps/web/src/features/projects/` 定位）加可选场景模板 Select——复用 `project-basics-step.tsx:25-31` 的 `["scenario-templates"]` query + active 过滤 + `NO_TEMPLATE_VALUE` 模式，提交体带 `scenario_template_key`
- Modify: `apps/web/src/lib/api/` 对应手写客户端（需求提交请求 + 决策请求加 `target_exit_deliverable`）
- Test: `apps/web/src/features/projects/components/project-operational-detail.test.tsx` 追加用例

**Interfaces:**
- Consumes: payload 字段 `template_key/template_version/exit_deliverable/available_exits/constraint_notes`（Task 6/8）；OpenAPI `scenario_template_key`（Task 4）、`target_exit_deliverable`（Task 11）。

- [ ] **Step 1: 失败组件测试**（vitest browser，仿同文件既有用例的 `render` + fixture revision）：payload 带 `{template_key:"software_delivery", template_version:2, exit_deliverable:"review_verdict", available_exits:[...], constraint_notes:[{kind:"human_gate",message:"发布任务已强制人类审批：由 human_gate@software_delivery v2 触发"}]}` → 断言可见文本 `software_delivery@v2`、`审查通过并合入`、约束说明文本。
- [ ] **Step 2: 失败** → **Step 3: 实现**：
  - 场景模板 row 值改为 `template_key@v{template_version}`（helper 扩展读两字段，缺 version 时退回裸 key 兼容存量 payload）。
  - 新增"交付出口" `RuntimeMeta` row：显示 `available_exits` 中匹配 `exit_deliverable` 的 label（缺失退回裸 deliverable）。
  - 新增"约束说明"块：`constraint_notes` 非空时按 kind 徽标 + message 列表渲染（v3 组件，紧凑列表形态）。
  - request_changes 交互处（既有计划评审操作区）：当 `available_exits.length > 1` 时提供出口 Select，选中值随决策请求体 `target_exit_deliverable` 提交。
  - 需求表单模板 Select + 客户端字段透传。
- [ ] **Step 4: 通过**：`corepack pnpm --filter @superteam/web test -- --no-file-parallelism`。Expected: 全绿。
- [ ] **Step 5: Commit** — `feat(web): 需求级模板选择+确认卡出口/约束说明/改选出口`

---

### Task 13: 收尾 —— 分层门禁 + 真实端到端 + 完成检查

- [ ] **Step 1: 分层门禁**：`corepack pnpm verify:control-plane && corepack pnpm verify:web`。Expected: 全绿（门禁≠端到端，下一步才是完成条件）。
- [ ] **Step 2: 真实端到端回归**（dev-services 全绿后浏览器全流程）：Task 10 场景 + Task 12 交互——需求表单选 `research_report` 覆盖项目默认 → 计划按调研骨架、确认卡显示 `research_report@v2` 与出口；request_changes 改选出口 → 重规划计划 exit 钉住；generic 项目（不绑模板）行为与现状一致；批准 → 派发 → 任务真实执行完成。逐条记录到本文件"实施记录"节。
- [ ] **Step 3: 跑项目完成检查 skill**：`$superteam-completion-check`（`.codex/skills/superteam-completion-check/SKILL.md`）。
- [ ] **Step 4: Commit + 汇报**：实施记录入库；向人类汇报收敛闸门数据与遗留项（P2b 待立、loop/chat 确认语义待 Loop 信封 spec）。

---

## 实施记录

（实施时追加：收敛闸门轮数与样本、E2E 逐条结果、偏差与遗留。）
