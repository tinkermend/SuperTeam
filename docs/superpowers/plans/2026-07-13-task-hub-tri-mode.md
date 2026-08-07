# 任务中枢三模式(Plan/Loop/Chat)实现计划
> 复核状态：已实现（07-14 全链 live E2E 通过）

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 任务中枢成为三模式入口:Plan/Loop 任务(coordination_mode 随需求冻结进 plan revision,workflow 上游阻塞分支按模式分流),Chat(与指定数字员工单次对话,复用 standalone run 路径,平台侧归类)。

**Architecture:** Chat = 一次普通 standalone digital-employee run,`run_kind` 归类落 `tasks` 表,追问经 `metadata["provider_session_id"]` 续 provider 会话(与项目返工续会话同一生产路径),Runtime/Provider 零改动。Plan/Loop = `coordination_mode` 从 demand → plan revision 冻结,`handleEmployeeTaskCompleted` 的 blocked 分支按模式分流:loop 自动补链(现行为),plan 发人类决策请求,批准后走同一补链机制。

**Tech Stack:** Go (chi + sqlc + Temporal)、Atlas 迁移、oapi-codegen、React + TanStack Query/Router + vitest。

**Spec:** `docs/superpowers/specs/2026-07-13-task-hub-tri-mode-design.md`

## Global Constraints

- 验证只用仓库脚本:`corepack pnpm verify:contracts | verify:control-plane | verify:web | verify:db`;契约生成 `corepack pnpm generate:control-plane`;sqlc 生成 `make -C apps/control-plane generate-sqlc`。
- 迁移唯一目录 `apps/control-plane/internal/storage/migrations/`;新迁移后必须更新 `atlas.sum` 并跑 `make -C apps/control-plane migrate-validate`。
- Temporal workflow 行为变更必须包在 `workflow.GetVersion(ctx, "<change-id>", workflow.DefaultVersion, 1)` 中(既有模式见 `workflow.go:86,:537`)。
- Web:改 UI 前先读 `DESIGN.md`;三态切换用 `V3Segmented`(`@/components/superteam`);玻璃卡只用 `GlassCard`;`.tl-*` CSS 只承载布局不得重声明玻璃表面;内部跳转用 TanStack Router;测试 `corepack pnpm --filter @superteam/web test`,禁止 `npx vitest run`。
- 词汇:`run_kind ∈ {task, chat}`(缺省 task);`coordination_mode ∈ {plan, loop}`(新需求缺省 plan;存量 plan revision 无值按 loop 解释)。两者都是平台级安全/协调词汇,封闭枚举,服务端校验。
- 提交信息末尾:`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`。
- Runtime Agent(`apps/runtime-agent/`)本计划零改动;若发现必须改,停下报人类。

## 背景事实(实现者必读,均已核实)

- **没有 `digital_employee_runs` 表**。standalone run 存 `tasks` + `task_runs` 两表,由 sqlc CTE `CreateDigitalEmployeeTaskRun`(`internal/storage/queries/tasks.sql:259`)一次插入。run 领域结构 `DigitalEmployeeRun` 在 `internal/employee/run_types.go:50`,**没有 Metadata 字段**,metadata 存 `tasks.params["metadata"]`。
- **会话机制**:Runtime `StartSession` 的会话 id 只来自 payload `metadata["provider_session_id"]` 或 `session_policy.provider_session_id`(`apps/runtime-agent/src/commands/executor.rs` `non_empty_session_id`);不带则必开新会话。控制平面项目路径已用 `metadata["provider_session_id"]` 续会话(`run_service.go:214-226`),chat 追问复用同一机制。**首问不需要任何改动即为全新会话。**
- Run 创建入口:handler `CreateDigitalEmployeeRun`(`run_handler.go:31`)解码**匿名 struct**(:44-57,不是生成类型)→ `service.CreateRun`(`run_service.go:111`)→ `createAndDispatchRun`(:253,内含 `GetActiveRun` 单活跃约束)→ `repository.CreateRun`(pg 实现 `pg_run_repository.go:375` 调 sqlc)。
- Demand 路径:`SubmitDemand` handler(`internal/project/handler.go:656`,解码 `submitDemandBody`)→ `Service.SubmitDemand`(`service.go:1725`)→ `repository.CreateProjectDemand` → signal 协调线程。Go 类型 `SubmitProjectDemandRequest` 在 `internal/project/types.go:1448`。`project_demands` 表建于迁移 013(:156)。
- Plan revision:store `PersistPlanRevision`(`projectcoordination/project_store.go:471`)→ `repository.CreatePlanRevision`(`pg_repository.go:1027`,sqlc `CreateProjectPlanRevision`);请求类型 `CreatePlanRevisionRequest`(`internal/project/repository.go:224`);表建于迁移 031;读取器 `GetPlanRevision`(`pg_repository.go:1107`)。
- Workflow blocked 分支在 `projectcoordination/workflow.go:536-556`;`InspectTaskResultDecisionResult` 在 `projectcoordination/types.go:208`;决策请求先例 `RequestProjectTaskIterationExhaustedReview`(store `project_store.go:953-998`,DecisionType `project_task_iteration_exhausted`);人类决策回流:`pendingTaskFailureRecovery`(`workflow.go:138`)→ `handleHumanDecisionSubmitted`(:279-286)→ store 侧 apply 后派发返回的 ready task ids。
- Workflow 测试模式:`testsuite.WorkflowTestSuite` + `recordingActivityStore` + `RegisterDelayedCallback` 发 signal + 断言 `store.calls` 顺序;范例 `TestProjectCoordinatorSupplementsUpstreamOwnerForResolvableBlockedResult`(`workflow_test.go:769`)。
- Web:任务中枢 `TaskLaunchView`(`features/task-launches/index.tsx:28`);表单 `task-launch-form.tsx`;优先级/风险是本地 state 提交时丢弃(:52-53,:82-88)。employees API 在 `lib/api/employees.ts`(`createDigitalEmployeeRun`:778、`getDigitalEmployeeRun`:826、`listDigitalEmployeeRunEvents`:841、`listDigitalEmployees`:597)。轮询先例:active run 时 `refetchInterval: 2500`(`employees/detail.tsx:108`、`run-detail-drawer.tsx:33`)。web 测试注入自定义 `fetcher` mock(`index.test.tsx:144-219`),不用 testing-library。

---

## Phase 1:数据库与契约

### Task 1: 迁移 058 —— 三模式字段

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/058_task_hub_tri_mode.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`(工具生成)

**Interfaces:**
- Produces: `tasks.run_kind`、`tasks.resume_of_run_id`、`project_demands.coordination_mode`、`project_plan_revisions.coordination_mode` 四列,后续所有任务依赖。

- [ ] **Step 1: 写迁移文件**

```sql
-- 058_task_hub_tri_mode.sql
-- 任务中枢三模式:run 归类(task/chat)与协调模式(plan/loop)。
-- 见 docs/superpowers/specs/2026-07-13-task-hub-tri-mode-design.md

ALTER TABLE tasks
    ADD COLUMN run_kind VARCHAR(20) NOT NULL DEFAULT 'task',
    ADD COLUMN resume_of_run_id UUID;

ALTER TABLE tasks
    ADD CONSTRAINT chk_tasks_run_kind CHECK (run_kind IN ('task', 'chat'));

CREATE INDEX idx_tasks_tenant_run_kind ON tasks (tenant_id, run_kind);

COMMENT ON COLUMN tasks.run_kind IS 'run 归类:task=任务执行,chat=数字员工单次对话。纯分类标签,不改变执行语义。';
COMMENT ON COLUMN tasks.resume_of_run_id IS 'chat 追问血缘:指向上一个 chat run(task_runs.id),仅审计用,无 FK。';

ALTER TABLE project_demands
    ADD COLUMN coordination_mode VARCHAR(10) NOT NULL DEFAULT 'plan';

ALTER TABLE project_demands
    ADD CONSTRAINT chk_project_demands_coordination_mode CHECK (coordination_mode IN ('plan', 'loop'));

COMMENT ON COLUMN project_demands.coordination_mode IS '协调模式:plan=上游阻塞时报人类决策;loop=自动补链。随需求提交,冻结进 plan revision。';

ALTER TABLE project_plan_revisions
    ADD COLUMN coordination_mode VARCHAR(10);

ALTER TABLE project_plan_revisions
    ADD CONSTRAINT chk_project_plan_revisions_coordination_mode
        CHECK (coordination_mode IS NULL OR coordination_mode IN ('plan', 'loop'));

COMMENT ON COLUMN project_plan_revisions.coordination_mode IS '从 demand 冻结的协调模式;NULL(存量)按 loop 解释。';
```

注意:`project_plan_revisions.coordination_mode` 允许 NULL 是刻意的——存量行无值按 loop 解释(spec §8.3),不回填。

- [ ] **Step 2: 更新 atlas.sum 并校验**

Run: `make -C apps/control-plane migrate-validate`(该目标会提示 atlas.sum 更新方式;若仓库惯例是 `atlas migrate hash --dir file://internal/storage/migrations` 先行,照 `migrations/README.md` 执行)
Expected: validate 通过,无 checksum 报错。

- [ ] **Step 3: 提交**

```bash
git add apps/control-plane/internal/storage/migrations/
git commit -m "feat(db): add run_kind and coordination_mode columns for tri-mode"
```

### Task 2: OpenAPI 契约

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
  - `CreateDigitalEmployeeRunRequest`(:10814)
  - `DigitalEmployeeRun`(:10616,required 列表 :10616-10630 附近)
  - `listDigitalEmployeeRuns` 参数(:3245)
  - `SubmitProjectDemandRequest`(:7313)
- Regenerate: `apps/control-plane/gen/control_plane.gen.go`、`apps/control-plane/internal/api/gen/control_plane.gen.go`

**Interfaces:**
- Produces: 契约字段 `run_kind`(enum task|chat)、`resume_of_run_id`(uuid)、`coordination_mode`(enum plan|loop);生成类型供 Task 4/5/6 使用。

- [ ] **Step 1: 改契约**

`CreateDigitalEmployeeRunRequest.properties` 追加:

```yaml
        run_kind:
          type: string
          enum:
            - task
            - chat
          default: task
        resume_of_run_id:
          type: string
          format: uuid
```

`DigitalEmployeeRun.properties` 追加(并把 `run_kind` 加进该 schema 的 `required` 列表):

```yaml
        run_kind:
          type: string
          enum:
            - task
            - chat
        resume_of_run_id:
          type: string
          format: uuid
```

`listDigitalEmployeeRuns` 的 `parameters` 追加(与既有 `status` 参数并列):

```yaml
        - name: run_kind
          in: query
          required: false
          schema:
            type: string
            enum:
              - task
              - chat
```

`SubmitProjectDemandRequest.properties` 追加:

```yaml
        coordination_mode:
          type: string
          enum:
            - plan
            - loop
          default: plan
```

- [ ] **Step 2: 生成并验证**

Run: `corepack pnpm generate:control-plane && corepack pnpm verify:contracts`
Expected: 两个 gen 文件更新,contracts 验证通过。

- [ ] **Step 3: 提交**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/gen/ apps/control-plane/internal/api/gen/
git commit -m "feat(contracts): run_kind, resume_of_run_id and coordination_mode"
```

### Task 3: sqlc 查询

**Files:**
- Modify: `apps/control-plane/internal/storage/queries/tasks.sql`(`CreateDigitalEmployeeTaskRun`:259、`ListDigitalEmployeeRuns`:473、`ListDigitalEmployeeRunsDetailed`:519,及 run 单查——用 `rg -n "name: GetDigitalEmployee.*Run" tasks.sql` 定位)
- Modify: `apps/control-plane/internal/storage/queries/project.sql`(`CreateProjectDemand`、`CreateProjectPlanRevision`——用 `rg -n "name: CreateProjectDemand|name: CreateProjectPlanRevision" project.sql` 定位)
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`

**Interfaces:**
- Produces: 生成的 Params/Row 结构新增 `RunKind string`、`ResumeOfRunID pgtype.UUID`(或仓库现用的 uuid 空值类型,以生成物为准)、`CoordinationMode`;Task 4/5/6/7 依赖。

- [ ] **Step 1: 改 `CreateDigitalEmployeeTaskRun`**

在 CTE `created_task AS ( INSERT INTO tasks ( ... ) VALUES ( ... ) )` 的列清单追加 `run_kind, resume_of_run_id`,VALUES 对应位置追加:

```sql
        sqlc.arg('run_kind')::varchar,
        sqlc.narg('resume_of_run_id')::uuid,
```

- [ ] **Step 2: 改查询回读与过滤**

run 单查与两个列表查询的 SELECT 列表追加 `t.run_kind, t.resume_of_run_id`(它们都 `FROM task_runs tr JOIN tasks t`)。`ListDigitalEmployeeRuns` 与 `ListDigitalEmployeeRunsDetailed` 的 WHERE 追加:

```sql
      AND (sqlc.narg('run_kind')::varchar IS NULL OR t.run_kind = sqlc.narg('run_kind')::varchar)
```

`CreateProjectDemand` 的 INSERT 列清单追加 `coordination_mode`,VALUES 追加 `sqlc.arg('coordination_mode')::varchar`。
`CreateProjectPlanRevision` 的 INSERT 列清单追加 `coordination_mode`,VALUES 追加 `sqlc.narg('coordination_mode')::varchar`;该查询的 RETURNING/后续 `GetProjectPlanRevision` 等 SELECT 同步补列(用 `rg -n "coordination_job_id" project.sql` 找同表 SELECT 逐一补)。

- [ ] **Step 3: 生成并编译**

Run: `make -C apps/control-plane generate-sqlc && cd apps/control-plane && go build ./...`
Expected: 生成通过;编译会因既有调用点未传新参数而失败——这是预期,Task 4/6/7 修复;若失败点超出 employee/project 两包,先在调用点用零值(`RunKind: "task"`)补齐使编译通过再提交。

- [ ] **Step 4: 提交**

```bash
git add apps/control-plane/internal/storage/queries/
git commit -m "feat(db): thread run_kind and coordination_mode through sqlc queries"
```

## Phase 2:Chat 后端

### Task 4: RunService 支持 chat run 与追问续会话

**Files:**
- Modify: `apps/control-plane/internal/employee/run_types.go`(:138 `CreateDigitalEmployeeRunRequest`、:50 `DigitalEmployeeRun`)
- Modify: `apps/control-plane/internal/employee/run_handler.go`(:44-57 匿名 struct、:62 调用、`runResponseFromDomain`)
- Modify: `apps/control-plane/internal/employee/run_service.go`(`CreateRun`:111、`createAndDispatchRun`:253、`buildRunParams`:1052)
- Modify: `apps/control-plane/internal/employee/run_repository.go`(:115 `CreateRunRecordRequest`)、`pg_run_repository.go`(:375 `CreateRun`、run 行→领域映射函数)
- Test: `apps/control-plane/internal/employee/run_service_test.go`(沿用现有 fake repository 测试设施)

**Interfaces:**
- Consumes: Task 3 生成的 sqlc 参数字段。
- Produces: `RunKindTask/RunKindChat` 常量;`CreateDigitalEmployeeRunRequest.RunKind string`、`.ResumeOfRunID *uuid.UUID`;`DigitalEmployeeRun.RunKind string`、`.ResumeOfRunID *uuid.UUID`;错误 `ErrInvalidRunKind`、`ErrInvalidResumeRun`(handler 映射 400)。

- [ ] **Step 1: 写失败测试**(放进 run_service_test.go,函数名按包内惯例)

```go
func TestCreateRunChatResumeValidation(t *testing.T) {
	// 用包内既有 fake repository/dispatcher 构造 service(参照现有 CreateRun 测试的 setup)
	cases := []struct {
		name    string
		mutate  func(req *CreateDigitalEmployeeRunRequest, prior *DigitalEmployeeRun)
		wantErr error
	}{
		{"run_kind 非法", func(req *CreateDigitalEmployeeRunRequest, _ *DigitalEmployeeRun) {
			req.RunKind = "banana"
		}, ErrInvalidRunKind},
		{"resume 但 run_kind 是 task", func(req *CreateDigitalEmployeeRunRequest, _ *DigitalEmployeeRun) {
			req.RunKind = RunKindTask
		}, ErrInvalidResumeRun},
		{"上个 run 属于别的员工", func(_ *CreateDigitalEmployeeRunRequest, prior *DigitalEmployeeRun) {
			prior.DigitalEmployeeID = uuid.New()
		}, ErrInvalidResumeRun},
		{"上个 run 不是 chat", func(_ *CreateDigitalEmployeeRunRequest, prior *DigitalEmployeeRun) {
			prior.RunKind = RunKindTask
		}, ErrInvalidResumeRun},
		{"上个 run 未终态", func(_ *CreateDigitalEmployeeRunRequest, prior *DigitalEmployeeRun) {
			prior.Status = DigitalEmployeeRunStatusRunning
		}, ErrInvalidResumeRun},
		{"上个 run 无 provider session", func(_ *CreateDigitalEmployeeRunRequest, prior *DigitalEmployeeRun) {
			prior.ProviderSessionID = nil
		}, ErrInvalidResumeRun},
	}
	// 每个 case:构造 prior(缺省合法:同员工、RunKind=chat、completed、有 ProviderSessionID)
	// → mutate → service.CreateRun → require.ErrorIs(t, err, tc.wantErr)
	_ = cases
}

func TestCreateRunChatResumeInjectsProviderSession(t *testing.T) {
	// prior 合法,ProviderSessionID = "sess-abc"
	// service.CreateRun(RunKind=chat, ResumeOfRunID=&prior.ID)
	// 断言 repository 收到的 CreateRunRecordRequest.Params["metadata"] 含
	//   "provider_session_id": "sess-abc" 和 "resume_of_run_id": prior.ID.String()
	// 断言 CreateRunRecordRequest.RunKind == RunKindChat
}

func TestCreateRunDefaultsRunKindTask(t *testing.T) {
	// 不传 RunKind → repository 收到 RunKind == RunKindTask,metadata 无 provider_session_id
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd apps/control-plane && go test ./internal/employee/ -run 'TestCreateRunChat|TestCreateRunDefaults' -v`
Expected: 编译失败(常量/字段不存在)。

- [ ] **Step 3: 实现**

`run_types.go`:

```go
const (
	RunKindTask = "task"
	RunKindChat = "chat"
)

var (
	ErrInvalidRunKind   = errors.New("invalid run_kind")
	ErrInvalidResumeRun = errors.New("invalid resume_of_run_id")
)
```

`CreateDigitalEmployeeRunRequest` 加 `RunKind string`、`ResumeOfRunID *uuid.UUID`;`DigitalEmployeeRun` 加 `RunKind string`、`ResumeOfRunID *uuid.UUID`。

`run_service.go` `CreateRun` 在既有校验后追加:

```go
	if req.RunKind == "" {
		req.RunKind = RunKindTask
	}
	if req.RunKind != RunKindTask && req.RunKind != RunKindChat {
		return nil, ErrInvalidRunKind
	}
	if req.ResumeOfRunID != nil {
		if req.RunKind != RunKindChat {
			return nil, ErrInvalidResumeRun
		}
		prior, err := s.repository.GetRun(ctx, req.TenantID, *req.ResumeOfRunID) // 签名以 pg_run_repository.go:409 的 GetRun 为准
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidResumeRun, err)
		}
		if prior == nil ||
			prior.DigitalEmployeeID != req.DigitalEmployeeID ||
			prior.RunKind != RunKindChat ||
			!prior.Status.IsTerminal() ||
			prior.ProviderSessionID == nil || strings.TrimSpace(*prior.ProviderSessionID) == "" {
			return nil, ErrInvalidResumeRun
		}
		if req.Metadata == nil {
			req.Metadata = map[string]any{}
		}
		req.Metadata["provider_session_id"] = *prior.ProviderSessionID
		req.Metadata["resume_of_run_id"] = prior.ID.String()
	}
```

`createAndDispatchRun` 构造 `CreateRunRecordRequest` 处传 `RunKind: req.RunKind`、`ResumeOfRunID: req.ResumeOfRunID`;`CreateRunRecordRequest` 加同名字段;`pg_run_repository.go` `CreateRun` 把两字段传给 sqlc 参数(Task 3 已加列),行→领域映射函数补 `RunKind`/`ResumeOfRunID` 回读。

`run_handler.go` 匿名 struct 加 `RunKind string \`json:"run_kind"\``、`ResumeOfRunID *uuid.UUID \`json:"resume_of_run_id"\``,传入 service 请求;错误映射处把 `ErrInvalidRunKind`/`ErrInvalidResumeRun` 归入 400(与既有校验错误同路);`runResponseFromDomain` 输出 `run_kind`、`resume_of_run_id`。

- [ ] **Step 4: 跑测试确认通过**

Run: `cd apps/control-plane && go test ./internal/employee/ -v`
Expected: 全部 PASS(含存量测试——若存量 fake repository 需补 `GetRun`/新字段,一并修)。

- [ ] **Step 5: 提交**

```bash
git add apps/control-plane/internal/employee/
git commit -m "feat(employee): chat run kind with provider session resume"
```

### Task 5: runs 列表按 run_kind 过滤

**Files:**
- Modify: `apps/control-plane/internal/employee/run_handler.go`(`ListDigitalEmployeeRuns` handler,解析 query)
- Modify: `apps/control-plane/internal/employee/run_repository.go` + `pg_run_repository.go`(列表方法签名加 filter 字段)
- Test: `apps/control-plane/internal/employee/run_handler_test.go` 或 repository 集成测试(沿用包内既有模式)

**Interfaces:**
- Consumes: Task 3 的列表查询 `run_kind` narg;Task 4 的 `RunKindChat` 常量。
- Produces: `ListRunsFilter`(或包内现有 filter struct)新增 `RunKind *string`;列表项 JSON 含 `run_kind`。

- [ ] **Step 1: 写失败测试**——handler 层:请求 `?run_kind=chat` 时断言 repository 收到 `RunKind != nil && *RunKind == "chat"`;`?run_kind=banana` 时 400。
- [ ] **Step 2: 确认失败**

Run: `cd apps/control-plane && go test ./internal/employee/ -run TestListDigitalEmployeeRuns -v`
Expected: FAIL。

- [ ] **Step 3: 实现**——handler 读 `r.URL.Query().Get("run_kind")`,空→nil,非 task/chat→400;filter struct 与 pg 实现透传给 sqlc。
- [ ] **Step 4: 确认通过并提交**

```bash
git add apps/control-plane/internal/employee/
git commit -m "feat(employee): filter runs by run_kind"
```

## Phase 3:Plan/Loop 后端

### Task 6: demand 携带 coordination_mode

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`(:1448 请求类型;新增常量)
- Modify: `apps/control-plane/internal/project/handler.go`(:656 `SubmitDemand` 与 `submitDemandBody`)
- Modify: `apps/control-plane/internal/project/service.go`(:1725 `SubmitDemand` 校验)
- Modify: `apps/control-plane/internal/project/pg_repository.go`(`CreateProjectDemand` 传参)+ demand 领域类型/响应映射补字段
- Test: `apps/control-plane/internal/project/service_test.go`(沿用既有 SubmitDemand 测试设施)

**Interfaces:**
- Produces: `project.CoordinationModePlan = "plan"`、`project.CoordinationModeLoop = "loop"`、`ErrInvalidCoordinationMode`;`SubmitProjectDemandRequest.CoordinationMode string`;`ProjectDemand.CoordinationMode string`。Task 7/8/10 依赖这些常量。

- [ ] **Step 1: 写失败测试**

```go
func TestSubmitDemandCoordinationMode(t *testing.T) {
	// 三个子用例:
	// 1. 不传 → repository 收到 CoordinationMode == CoordinationModePlan(缺省)
	// 2. 传 "loop" → 收到 loop
	// 3. 传 "banana" → require.ErrorIs(err, ErrInvalidCoordinationMode),不落库不发信号
}
```

- [ ] **Step 2: 确认失败**

Run: `cd apps/control-plane && go test ./internal/project/ -run TestSubmitDemandCoordinationMode -v`
Expected: 编译失败。

- [ ] **Step 3: 实现**

`types.go`:

```go
const (
	CoordinationModePlan = "plan"
	CoordinationModeLoop = "loop"
)

var ErrInvalidCoordinationMode = errors.New("invalid coordination_mode")
```

`SubmitProjectDemandRequest` 加 `CoordinationMode string`;`service.SubmitDemand` 开头:

```go
	if req.CoordinationMode == "" {
		req.CoordinationMode = CoordinationModePlan
	}
	if req.CoordinationMode != CoordinationModePlan && req.CoordinationMode != CoordinationModeLoop {
		return nil, ErrInvalidCoordinationMode
	}
```

`submitDemandBody` 加 `CoordinationMode string \`json:"coordination_mode"\``;handler 错误映射 400;pg `CreateProjectDemand` 传列;demand 领域类型与 `demandResponseFromDomain` 回传。

- [ ] **Step 4: 确认通过并提交**

```bash
git add apps/control-plane/internal/project/
git commit -m "feat(project): demand carries coordination_mode (default plan)"
```

### Task 7: 模式冻结进 plan revision

**Files:**
- Modify: `apps/control-plane/internal/project/repository.go`(:224 `CreatePlanRevisionRequest`)、`plan_revision.go`(:33 `PlanRevision`)、`pg_repository.go`(:1027 `CreatePlanRevision`、:1107 `GetPlanRevision` 等行映射)
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`(:471 `PersistPlanRevision`)
- Test: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

**Interfaces:**
- Consumes: Task 6 常量;Task 3 的 `CreateProjectPlanRevision` coordination_mode narg。
- Produces: `PlanRevision.CoordinationMode *string`(NULL=存量=loop 解释);`CreatePlanRevisionRequest.CoordinationMode *string`。

- [ ] **Step 1: 写失败测试**——store 测试:对 coordination_mode=loop 的 demand 执行 `PersistPlanRevision`,断言落库的 plan revision `CoordinationMode` 为 `"loop"`;plan 的 demand 得 `"plan"`。(demand 读取:用 `rg -n "GetProjectDemand" apps/control-plane/internal/project/pg_repository.go` 确认既有 getter 名;若无,则经 `CreateProjectDemand` 返回值/测试 fixture 注入。)
- [ ] **Step 2: 确认失败**

Run: `cd apps/control-plane && go test ./internal/workflow/projectcoordination/ -run TestPersistPlanRevision -v`
Expected: FAIL。

- [ ] **Step 3: 实现**——`PersistPlanRevision` 在构造 `CreatePlanRevisionRequest` 前读 demand(store 已持有 repository;input 里有 DemandID),取 `demand.CoordinationMode` 写入请求;repo/行映射透传。存量兼容:demand 读不到时 CoordinationMode 置 nil,不报错。
- [ ] **Step 4: 确认通过并提交**

```bash
git add apps/control-plane/internal/project/ apps/control-plane/internal/workflow/projectcoordination/
git commit -m "feat(coordination): freeze coordination_mode into plan revision"
```

### Task 8: InspectTaskResultDecision 返回模式

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`(:208)
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`(`InspectTaskResultDecision` 实现,用 `rg -n "func (s \*ProjectStore) InspectTaskResultDecision" project_store.go` 定位)
- Test: `project_store_test.go`

**Interfaces:**
- Consumes: Task 7 的 `PlanRevision.CoordinationMode`;`GetPlanRevision`(`pg_repository.go:1107`)。
- Produces: `InspectTaskResultDecisionResult.CoordinationMode string`(恒为 "plan" 或 "loop",已解析缺省)。Task 10 依赖。

- [ ] **Step 1: 写失败测试**——三个用例:任务的 accepted plan revision `coordination_mode='plan'` → 返回 `"plan"`;`='loop'` 或 NULL → `"loop"`;任务无 `AcceptedPlanRevisionID` → `"loop"`。
- [ ] **Step 2: 确认失败**(`go test ./internal/workflow/projectcoordination/ -run TestInspectTaskResultDecision -v`)
- [ ] **Step 3: 实现**

```go
	mode := project.CoordinationModeLoop
	if task.AcceptedPlanRevisionID != nil {
		if rev, err := s.repository.GetPlanRevision(ctx, input.TenantID, *task.AcceptedPlanRevisionID); err == nil &&
			rev.CoordinationMode != nil && *rev.CoordinationMode == project.CoordinationModePlan {
			mode = project.CoordinationModePlan
		}
	}
	result.CoordinationMode = mode
```

(读取失败静默按 loop:模式解析不得让结果检查整体失败。)

- [ ] **Step 4: 确认通过并提交**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "feat(coordination): resolve coordination mode in task result inspection"
```

### Task 9: 补链提案人类决策(store 侧)

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`(新增 `RequestUpstreamSupplementReview`,参照 :953-998 `RequestProjectTaskIterationExhaustedReview` 逐行模仿;扩展 apply 决策方法——用 `rg -n "applyFailureRecoveryDecision" workflow.go` 找 workflow helper,再定位其 store 方法)
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`(activity 接口 :632 附近登记新方法)与 `activities.go`(转发)
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`(input 类型)
- Test: `project_store_test.go`

**Interfaces:**
- Consumes: `CreateUpstreamSupplementTasks` store 逻辑(`project_store.go:790`)、决策请求三件套先例(approvals.CreateRequest + repository.CreateDecisionRequest + inbox.UpsertProjectDecisionRequest)。
- Produces:
  - `RequestUpstreamSupplementReview(ctx, RequestUpstreamSupplementReviewInput) (DecisionRequestResult, error)`;input:`TenantID, ProjectID, ProjectTaskID, ResultID, CompletedEventID uuid.UUID; MissingInputs []string`。
  - DecisionType 常量 `"upstream_supplement_review"`,Options `["approved","rejected"]`,ContextPayload 键:`project_id, project_task_id, result_id, completed_event_id, missing_inputs, summary`。
  - apply 决策方法新增 case:approved → 内部调用既有补链逻辑(等价 `CreateUpstreamSupplementTasks`,MissingInputs 取自 context payload)→ 返回 supplement TaskIDs 作为待派发;rejected → 追加审计事件 `project_task.upstream_supplement_rejected`,返回空;approved 但补链 Exhausted → 追加审计事件,返回空(人类已在环内,不再自动升级)。

- [ ] **Step 1: 写失败测试**——(a) `RequestUpstreamSupplementReview` 创建 DecisionType `upstream_supplement_review` 的决策请求且 context payload 含 missing_inputs、下游任务被置 blocked(照 exhausted 先例断言);(b) apply approved → 补链任务被创建并返回其 ID;(c) apply rejected → 无任务创建、有审计事件。
- [ ] **Step 2: 确认失败**(`go test ./internal/workflow/projectcoordination/ -run TestUpstreamSupplementReview -v`)
- [ ] **Step 3: 实现**(严格模仿 :953-998 的三件套与下游 block;apply 侧在既有决策类型 switch 中加 case)
- [ ] **Step 4: 确认通过并提交**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "feat(coordination): human review gate for upstream supplement proposals"
```

### Task 10: workflow 模式分支

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`(:536-556 分支;新增 helper `requestUpstreamSupplementReview`,模仿 :782-796)
- Test: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`(模仿 :769 与 :825 两个先例;`recordingActivityStore` 补新方法与录制)

**Interfaces:**
- Consumes: Task 8 的 `decision.CoordinationMode`;Task 9 的 activity。
- Produces: change id `"coordination-mode-branch"`。

- [ ] **Step 1: 写失败测试**

```go
// TestProjectCoordinatorRequestsSupplementReviewInPlanMode:
//   store 的 InspectTaskResultDecision 返回 Decision=blocked_resolvable_upstream,
//   CoordinationMode="plan",Blocker.MissingInputs=["load_test_report"]
//   → 断言 store.calls == ["AppendProjectEvent","InspectTaskResultDecision","RequestUpstreamSupplementReview"]
//   → 断言无 "CreateUpstreamSupplementTasks"、无 "DispatchProjectTask"
// TestProjectCoordinatorAutoSupplementsInLoopMode:
//   同上但 CoordinationMode="loop" → 断言与既有 :769 测试相同的调用序列(自动补链+派发)
// TestProjectCoordinatorDispatchesSupplementAfterApproval:
//   plan 模式发起 review 后,RegisterDelayedCallback 发 HumanDecisionSubmitted(approved,DecisionRequestID 匹配)
//   → 断言 apply 后返回的任务被 DispatchProjectTask(DispatchReasonRetry)
```

- [ ] **Step 2: 确认失败**(`go test ./internal/workflow/projectcoordination/ -run 'SupplementReviewInPlanMode|AutoSupplementsInLoopMode|SupplementAfterApproval' -v`)
- [ ] **Step 3: 实现**

```go
	if decision.Decision == string(project.TaskResultDecisionBlockedResolvableUpstream) &&
		workflow.GetVersion(ctx, "upstream-supplement-task", workflow.DefaultVersion, 1) != workflow.DefaultVersion {
		if decision.Blocker == nil {
			return taskCompletionPending{}, project.ErrInvalidProject
		}
		if workflow.GetVersion(ctx, "coordination-mode-branch", workflow.DefaultVersion, 1) != workflow.DefaultVersion &&
			decision.CoordinationMode == project.CoordinationModePlan {
			review, err := requestUpstreamSupplementReview(ctx, input.TenantID, input.ProjectID,
				signal.ProjectTaskID, decision.ResultID, signal.CompletedEventID, decision.Blocker.MissingInputs)
			if err != nil || review.ID == uuid.Nil {
				return taskCompletionPending{}, err
			}
			return taskCompletionPending{FailureRecovery: &pendingTaskFailureRecovery{
				DecisionRequestID: review.ID,
				ProjectID:         input.ProjectID,
			}}, nil
		}
		// loop 模式:以下为既有自动补链路径,原样保留
		supplement, err := createUpstreamSupplementTasks(...)
		...
	}
```

helper:

```go
func requestUpstreamSupplementReview(ctx workflow.Context, tenantID, projectID, projectTaskID, resultID, completedEventID uuid.UUID, missingInputs []string) (DecisionRequestResult, error) {
	var result DecisionRequestResult
	if err := workflow.ExecuteActivity(ctx, (*Activities).RequestUpstreamSupplementReview, RequestUpstreamSupplementReviewInput{
		TenantID: tenantID, ProjectID: projectID, ProjectTaskID: projectTaskID,
		ResultID: resultID, CompletedEventID: completedEventID, MissingInputs: missingInputs,
	}).Get(ctx, &result); err != nil {
		return DecisionRequestResult{}, err
	}
	return result, nil
}
```

- [ ] **Step 4: 全量确认**

Run: `cd apps/control-plane && go test ./internal/workflow/projectcoordination/ -v`
Expected: 全部 PASS(既有 loop 行为测试 :769/:825 必须原样通过——它们是"存量按 loop"的守门测试)。

- [ ] **Step 5: 提交**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/
git commit -m "feat(coordination): branch upstream supplement by coordination mode"
```

## Phase 4:Web

### Task 11: API 客户端类型

**Files:**
- Modify: `apps/web/src/lib/api/employees.ts`(:206 `DigitalEmployeeRunInput`、:221 `DigitalEmployeeRun`、:301 `ListDigitalEmployeeRunsFilter`、:793 query 拼接)
- Modify: `apps/web/src/lib/api/projects.ts`(:882 `SubmitProjectDemandInput`)
- Test: `apps/web/src/lib/api/projects.test.ts` 或就近类型使用处(类型级改动以调用方测试覆盖为主)

**Interfaces:**
- Produces:

```ts
export type DigitalEmployeeRunKind = "task" | "chat";
// DigitalEmployeeRunInput += run_kind?: DigitalEmployeeRunKind; resume_of_run_id?: string;
// DigitalEmployeeRun    += run_kind: DigitalEmployeeRunKind; resume_of_run_id?: string;
// ListDigitalEmployeeRunsFilter += run_kind?: DigitalEmployeeRunKind;(listDigitalEmployeeRuns 拼进 query)
export type ProjectCoordinationMode = "plan" | "loop";
// SubmitProjectDemandInput += coordination_mode?: ProjectCoordinationMode;
```

- [ ] **Step 1: 改类型与 query 拼接** → **Step 2:** `corepack pnpm --filter @superteam/web typecheck`(若无该脚本用 `corepack pnpm verify:web` 的类型阶段)Expected: PASS → **Step 3: 提交**

```bash
git add apps/web/src/lib/api/
git commit -m "feat(web): api types for run_kind and coordination_mode"
```

### Task 12: 任务中枢三态切换 + plan/loop 提交

**Files:**
- Modify: `apps/web/src/features/task-launches/components/task-launch-form.tsx`
- Modify: `apps/web/src/features/task-launches/index.tsx`(透传 mode)
- Test: `apps/web/src/features/task-launches/index.test.tsx`

**Interfaces:**
- Consumes: `V3Segmented`(`@/components/superteam`,`v3-components.tsx:502`);Task 11 类型。
- Produces: `type LaunchMode = "plan" | "loop" | "chat"`;`TaskLaunchForm` 新 props `mode: LaunchMode; onModeChange: (m: LaunchMode) => void; chatPanel?: ReactNode`(chat 态渲染 chatPanel 占位,Task 13 填充)。

- [ ] **Step 1: 写失败测试**(沿用 index.test.tsx 的 fetcher 注入模式)

```ts
// 1. 默认 plan:提交后 postBody(fetcher, "/api/v1/projects/project-1/demands")
//    含 coordination_mode: "plan",且不含 priority/risk 字段
// 2. 切到 Loop(点击 V3Segmented 的 "Loop 任务")再提交 → coordination_mode: "loop"
// 3. 页面不再渲染 "优先级" 与 "风险级别" 文案
// 4. 切到 "对话" → 不渲染项目选择 chip,渲染 data-testid="chat-panel" 占位
```

- [ ] **Step 2: 确认失败**

Run: `corepack pnpm --filter @superteam/web test -- src/features/task-launches`
Expected: FAIL。

- [ ] **Step 3: 实现**

- `TaskLaunchView` 持有 `const [mode, setMode] = useState<LaunchMode>("plan")`,传给表单;提交时 `input.coordination_mode = mode === "loop" ? "loop" : "plan"`(chat 态不走 demand 提交)。
- 表单头部(hero 下方、GlassCard 上沿)渲染:

```tsx
<V3Segmented<LaunchMode>
  options={[
    { label: "Plan 任务", value: "plan" },
    { label: "Loop 任务", value: "loop" },
    { label: "对话", value: "chat" },
  ]}
  value={mode}
  onChange={onModeChange}
/>
<p className="tl-sub">
  {mode === "plan" && "遇上游阻塞时暂停,提案报你决策后再补做"}
  {mode === "loop" && "遇上游阻塞时自动补做上游任务并重跑下游"}
  {mode === "chat" && "与指定数字员工单次对话,结果不进入项目流转"}
</p>
```

- 删除 `priority`/`riskLevel` state 与两个 `LaunchChip`(及 GitBranch/CircleAlert icon import);`mode === "chat"` 时渲染 `chatPanel`,否则渲染现有任务表单。
- 同步更新既有测试中对 payload 五字段的断言(现在多 `coordination_mode`)。

- [ ] **Step 4: 确认通过**

Run: `corepack pnpm --filter @superteam/web test -- src/features/task-launches`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/task-launches/
git commit -m "feat(web): tri-mode segmented switch in task hub"
```

### Task 13: 对话面板

**Files:**
- Create: `apps/web/src/features/task-launches/components/chat-panel.tsx`
- Modify: `apps/web/src/features/task-launches/index.tsx`(把 `<ChatPanel />` 传入 `chatPanel` prop,并接收"转为任务"回调)
- Modify: `apps/web/src/features/task-launches/components/task-launch-aurora.css`(新增 `.tl-chat-*` 布局类,仅布局,不得重声明玻璃表面)
- Test: `apps/web/src/features/task-launches/chat-panel.test.tsx`

**Interfaces:**
- Consumes: Task 11 API(`listDigitalEmployees`、`createDigitalEmployeeRun`、`getDigitalEmployeeRun`、`listDigitalEmployeeRunEvents`);Task 12 的 `chatPanel` 插槽。
- Produces: `ChatPanel({ apiOptions, onConvertToTask }: { apiOptions: ApiClientOptions; onConvertToTask: (draft: string) => void })`;`onConvertToTask` 由 index.tsx 实现为:`setMode("plan")` + 把 draft 写入表单内容(把表单 `content` state 提升到 `TaskLaunchView` 或经受控 prop 传入——实现时二选一,保持最小提升)。

- [ ] **Step 1: 写失败测试**(fetcher 注入模式,mock 员工列表/创建 run/get run/events)

```ts
// 1. 渲染后员工 Select 列出 mock 员工(name/role)
// 2. 输入问题点"发送" → 断言 POST /api/v1/digital-employees/emp-1/runs
//    body { objective: "问题文本", run_kind: "chat" }(无 resume_of_run_id)
// 3. mock getDigitalEmployeeRun 依次返回 running → completed(result 含 output 文本)
//    → waitFor 后回答文本渲染在 [data-testid="chat-thread"] 内
// 4. 再次发送 → 第二次 POST body 含 resume_of_run_id: "run-1"
// 5. 点"转为任务" → onConvertToTask 收到含问题与回答摘录的 draft 字符串
// 6. mock run 返回 failed → 渲染错误卡片与"重试";点重试 → 新 POST 无 resume_of_run_id
```

- [ ] **Step 2: 确认失败**

Run: `corepack pnpm --filter @superteam/web test -- src/features/task-launches/chat-panel`
Expected: FAIL(组件不存在)。

- [ ] **Step 3: 实现**

组件要点(完整结构,实现时按 DESIGN.md 表单规范微调):

```tsx
type ChatEntry = {
  runId: string;
  question: string;
  status: DigitalEmployeeRunStatus | "sending";
  answer?: string;
  error?: string;
};

export function ChatPanel({ apiOptions, onConvertToTask }: ChatPanelProps) {
  const [employeeId, setEmployeeId] = useState("");
  const [question, setQuestion] = useState("");
  const [thread, setThread] = useState<ChatEntry[]>([]);   // 页面本地线程,刷新即散(spec §9)
  const employeesQuery = useQuery({
    queryKey: ["chat-employees"],
    queryFn: () => listDigitalEmployees(apiOptions),
  });
  const lastCompleted = [...thread].reverse().find((e) => e.status === "completed");
  const activeEntry = thread.find((e) => isActiveRunStatus(e.status));

  const runQuery = useQuery({
    enabled: Boolean(activeEntry && employeeId),
    queryKey: ["chat-run", employeeId, activeEntry?.runId],
    queryFn: () => getDigitalEmployeeRun(apiOptions, employeeId, activeEntry!.runId),
    refetchInterval: 2500,           // 与 employees/detail.tsx:108 同节奏
  });
  // useEffect:runQuery.data 终态时把 answer/error 写回 thread 对应条目
  //   answer = extractAnswerText(runQuery.data)

  const sendMutation = useMutation({
    mutationFn: (input: { objective: string; resumeOf?: string }) =>
      createDigitalEmployeeRun(apiOptions, employeeId, {
        objective: input.objective,
        run_kind: "chat",
        ...(input.resumeOf ? { resume_of_run_id: input.resumeOf } : {}),
      }),
    onSuccess: (run, vars) => setThread((t) => [...t, { runId: run.id, question: vars.objective, status: run.status }]),
    onError: (err) => {/* 顶部错误条,发送按钮恢复 */},
  });
  // 发送:sendMutation.mutate({ objective: question, resumeOf: lastCompleted?.runId })
  // 重试:对 failed 条目重发其 question,不带 resumeOf
  // 换员工:setThread([]) —— 线程不跨员工
}

function extractAnswerText(run: DigitalEmployeeRun): string {
  const r = run.result as Record<string, unknown> | null | undefined;
  for (const key of ["output", "summary", "message", "text"]) {
    const v = r?.[key];
    if (typeof v === "string" && v.trim()) return v;
  }
  return r ? JSON.stringify(r, null, 2) : "(无结果内容)";
}

function buildTaskDraft(entry: ChatEntry, employeeName: string): string {
  const excerpt = (entry.answer ?? "").slice(0, 3000); // 需求框 5000 上限,给编辑留余量
  return `【目标】(请改写为你要的结果)\n\n${excerpt}\n\n【背景】源自与 @${employeeName} 的单次对话:${entry.question}`;
}
```

布局:线程区与输入区放 `.v3-glass-inner` 或实底容器(DESIGN.md 玻璃壳内核退回实底);员工 chip 复用 `LaunchChip` + shadcn `Select`。渲染态覆盖 loading/empty/error/执行中(DESIGN.md 检查清单)。

- [ ] **Step 4: 确认通过**

Run: `corepack pnpm --filter @superteam/web test -- src/features/task-launches`
Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/task-launches/
git commit -m "feat(web): chat panel with follow-up and convert-to-task"
```

### Task 14: 员工 run 历史归类徽章与筛选

**Files:**
- Modify: `apps/web/src/features/employees/components/employee-run-history-table.tsx`(:88-95 表头、:105-115 单元格、:65-77 筛选 chip 区)
- Test: `apps/web/src/features/employees/components/employee-run-history-table.test.tsx`(若无则新建,沿用组件测试惯例)

**Interfaces:**
- Consumes: Task 11 的 `DigitalEmployeeRun.run_kind`。
- Produces: 表格新列"类型"(`StatusPill`:chat → 文案"对话"、task → "任务");筛选 chip `V3Chip`(全部/任务/对话)驱动 `ListDigitalEmployeeRunsFilter.run_kind`。

- [ ] **Step 1: 写失败测试**(mock 列表项含 run_kind:"chat" → 断言渲染"对话"徽章;点击"对话"chip → 断言重新请求带 `run_kind=chat`)
- [ ] **Step 2: 确认失败** → **Step 3: 实现** → **Step 4: 确认通过**

Run: `corepack pnpm --filter @superteam/web test -- src/features/employees`

- [ ] **Step 5: 提交**

```bash
git add apps/web/src/features/employees/
git commit -m "feat(web): run history run_kind badge and filter"
```

## Phase 5:门禁与真实端到端

### Task 15: 分层门禁 + 真实链路验证

**Files:** 无新增代码;产出验证证据(贴命令输出)。

- [ ] **Step 1: 分层门禁**

Run(依次):
```bash
corepack pnpm verify:contracts
corepack pnpm verify:db
corepack pnpm verify:control-plane
corepack pnpm verify:web
```
Expected: 全部通过。

- [ ] **Step 2: 起服务**

Run: `scripts/dev-services.sh status`,按需 `scripts/dev-services.sh restart control-plane && scripts/dev-services.sh restart web`(control-plane restart 自动跑 Atlas 迁移,即应用 058)。确认 Runtime Agent 在线、员工有可用 Provider 执行实例。

- [ ] **Step 3: chat 真实链路**(浏览器,codex chrome plug 或手动)

1. 任务中枢切"对话",选员工,提问 → 回答就地渲染(确认非 mock:员工 run 历史出现 run_kind=chat 的新 run)。
2. 追问一个依赖前文的问题(如"把你刚才第二点展开")→ 回答确实引用前文 = 会话延续成立。
3. 点"转为任务" → 表单预填、切回 Plan 态 → 选项目提交 → 收件箱出现计划评审,demand 的 `source_refs` 含 `chat_run_id`(curl 核对)。

- [ ] **Step 4: plan/loop 真实链路**

1. Loop 态提交需求 → 构造上游阻塞场景(员工申报 blocked_resolvable_upstream)→ 确认自动补链任务被创建并派发(既有 E2E 场景可复用)。
2. Plan 态提交需求 → 同场景 → 确认收件箱出现"补链提案"决策(DecisionType `upstream_supplement_review`),批准后补链任务真实派发;驳回路径确认任务停在阻塞态且有审计事件。

- [ ] **Step 5: 收尾**

任一步无法满足(服务起不来、Provider 不可用等)→ 按 CLAUDE.md 标记阻塞并说明缺失依赖,不得声明完成。全部通过后跑 `$superteam-completion-check` skill 收尾。

---

## Self-review 记录

- Spec 覆盖:§5 数据流(Task 4/13)、§6 契约与 RunService(Task 1-5)、§7 转为任务(Task 13 buildTaskDraft + index 回调)、§8 plan/loop(Task 6-10)、§9 UI(Task 12-14)、§10 错误处理(Task 4 校验矩阵/Task 13 失败态/Task 6 400/Task 9 驳回路径)、§12 测试与 E2E(各 task TDD + Task 15)。spec §6.2 所写 `digital_employee_runs` 表不存在,本计划按真实表结构(`tasks`/`task_runs`)落地,语义不变。
- 类型一致性:`run_kind`/`RunKind`、`coordination_mode`/`CoordinationMode`、`resume_of_run_id`/`ResumeOfRunID` 全文统一;`InspectTaskResultDecisionResult.CoordinationMode` 在 Task 8 产出、Task 10 消费;`RequestUpstreamSupplementReviewInput` 在 Task 9 产出、Task 10 消费。
- 已知留白(刻意):sqlc 生成的空值类型(pgtype.UUID vs *uuid)以生成物为准;store apply 决策方法的确切名称以 `rg "applyFailureRecoveryDecision"` 锚点定位——两处均给了定位命令,不属于占位符。
