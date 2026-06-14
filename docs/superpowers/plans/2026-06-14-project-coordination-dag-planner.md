# Project Coordination DAG Planner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the backend foundation for DeepSeek-backed project coordination planning, persisted ProjectTask DAGs, dependency-aware dispatch, completion contract gates, failure recovery, append-only replanning, and a backend task-graph read API.

**Architecture:** Control Plane owns planning, governance, task graph persistence, and readiness computation. Temporal workflow stays deterministic and delegates DeepSeek calls, DB work, graph readiness, completion contract checks, and failure recovery to activities. Runtime and Provider layers only execute ready ProjectTasks after Control Plane dispatch.

**Tech Stack:** Go 1.25, PostgreSQL migrations managed by Atlas, sqlc, Temporal Go SDK, OpenAI-compatible DeepSeek Chat Completions, chi HTTP handlers, OpenAPI/oapi-codegen, pnpm repo scripts.

---

## Source Spec

Implement this plan against:

- `docs/superpowers/specs/2026-06-14-project-coordination-dag-planner-design.md`

Keep the spec open while implementing. When an implementation choice conflicts with this plan, prefer the spec and update this plan before coding further.

## File Structure

Create:

- `apps/control-plane/internal/storage/migrations/019_project_task_dependencies.sql`  
  Adds ProjectTask graph columns, dependency table, idempotency indexes, comments, and status comment update.
- `apps/control-plane/internal/workflow/projectcoordination/deepseek_planner.go`  
  DeepSeek/OpenAI-compatible planner implementation.
- `apps/control-plane/internal/workflow/projectcoordination/deepseek_planner_test.go`  
  Unit tests for request construction, JSON decoding, fallback classification, and fake HTTP client behavior.
- `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`  
  Graph-level validation gates and DAG cycle detection.
- `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`  
  Gate coverage for malformed plans, out-of-pool assignees, dangling blockers, cycles, no roots, and valid DAGs.
- `apps/control-plane/internal/project/task_graph_types.go`  
  Domain read-model types for task graph API.

Modify:

- `apps/control-plane/internal/config/config.go`  
  Add planner config and environment overrides.
- `apps/control-plane/config/config.example.yaml`  
  Add non-secret DeepSeek planner sample values.
- `apps/control-plane/go.mod` and `apps/control-plane/go.sum`  
  Add `github.com/openai/openai-go/v3` if using the official SDK directly.
- `apps/control-plane/internal/app/app.go`  
  Construct the route planner from config and inject it into project coordination activities.
- `apps/control-plane/internal/workflow/projectcoordination/activities.go`  
  Add planner injection and new graph/recovery activities.
- `apps/control-plane/internal/workflow/projectcoordination/planner.go`  
  Replace single-task planner shape with graph-aware planner interface, route plan types, heuristic fallback, and validation entrypoint.
- `apps/control-plane/internal/workflow/projectcoordination/types.go`  
  Add graph activity inputs/results, task result metadata, and human decision payload.
- `apps/control-plane/internal/workflow/projectcoordination/project_store.go`  
  Rework route persistence to aggregate route-decision fields from the task graph, then implement graph persistence, dispatchable queries, downstream readiness, failure hold, and recovery request projection.
- `apps/control-plane/internal/workflow/projectcoordination/workflow.go`  
  Dispatch only ready tasks, wake downstream on completion, hold downstream on failure, and split route-review vs failure-recovery decisions.
- `apps/control-plane/internal/workflow/projectcoordination/client.go`  
  Include human decision payload in workflow signals.
- `apps/control-plane/internal/project/types.go`  
  Add ProjectTask graph fields, dependency/read-model domain types, recovery request fields, and graph response types.
- `apps/control-plane/internal/project/repository.go`  
  Add graph persistence, dependency, completion-contract, idempotency, and task-graph read-model methods.
- `apps/control-plane/internal/project/pg_repository.go`  
  Wire sqlc queries, transactions, row mapping, idempotency handling, and graph read-model methods.
- `apps/control-plane/internal/project/service.go`  
  Add completion contract checks before task completion and expose task graph read model.
- `apps/control-plane/internal/project/handler.go`  
  Add `GET /task-graph` handler and include ProjectTask graph fields in responses.
- `apps/control-plane/internal/api/server.go`  
  Register the task-graph route under project routes.
- `apps/control-plane/internal/storage/queries/project.sql`  
  Add graph, dependency, contract, idempotency, and read-model queries.
- `contracts/control-plane/openapi.yaml`  
  Add ProjectTask fields, task graph endpoint, and response schemas.
- `apps/web/src/lib/api/generated/control-plane.ts` and related generated artifacts  
  Regenerate through `pnpm generate:control-plane`; do not hand-edit generated client output.

Test:

- `apps/control-plane/internal/storage/migrations_test.go`
- `apps/control-plane/internal/project/pg_repository_test.go`
- `apps/control-plane/internal/project/service_test.go`
- `apps/control-plane/internal/project/handler_test.go`
- `apps/control-plane/internal/workflow/projectcoordination/planner_test.go`
- `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`
- `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
- `apps/control-plane/internal/config/config_test.go`
- Web tests only if generated type changes break current compile or tests.

## Task 1: Add Planner Config

**Files:**

- Modify: `apps/control-plane/internal/config/config.go`
- Modify: `apps/control-plane/internal/config/config_test.go`
- Modify: `apps/control-plane/config/config.example.yaml`

- [ ] **Step 1: Write failing config tests**

Add tests that prove file config works and env overrides win:

```go
func TestLoadFromFilePlannerConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("REDIS_URL", "redis://example")
	t.Setenv("S3_ENDPOINT", "http://minio.local")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_BUCKET", "superteam")
	t.Setenv("S3_ACCESS_KEY_ID", "access")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secret")

	path := writeTempConfig(t, `
planner:
  provider: deepseek
  apiKey: file-key
  baseURL: https://api.deepseek.com
  model: deepseek-v4-pro
  maxTokens: 8192
  temperature: 0.1
  maxAttempts: 2
`)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	require.Equal(t, "deepseek", cfg.Planner.Provider)
	require.Equal(t, "file-key", cfg.Planner.APIKey)
	require.Equal(t, "https://api.deepseek.com", cfg.Planner.BaseURL)
	require.Equal(t, "deepseek-v4-pro", cfg.Planner.Model)
	require.Equal(t, 8192, cfg.Planner.MaxTokens)
	require.InDelta(t, 0.1, cfg.Planner.Temperature, 0.0001)
	require.Equal(t, 2, cfg.Planner.MaxAttempts)
}

func TestPlannerEnvOverridesFileConfig(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://example")
	t.Setenv("REDIS_URL", "redis://example")
	t.Setenv("S3_ENDPOINT", "http://minio.local")
	t.Setenv("S3_REGION", "us-east-1")
	t.Setenv("S3_BUCKET", "superteam")
	t.Setenv("S3_ACCESS_KEY_ID", "access")
	t.Setenv("S3_SECRET_ACCESS_KEY", "secret")
	t.Setenv("PLANNER_API_KEY", "env-key")
	t.Setenv("PLANNER_BASE_URL", "https://gateway.local")
	t.Setenv("PLANNER_MODEL", "deepseek-v4-flash")
	t.Setenv("PLANNER_MAX_TOKENS", "4096")
	t.Setenv("PLANNER_TEMPERATURE", "0")
	t.Setenv("PLANNER_MAX_ATTEMPTS", "3")

	path := writeTempConfig(t, `
planner:
  provider: deepseek
  apiKey: file-key
  baseURL: https://api.deepseek.com
  model: deepseek-v4-pro
  maxTokens: 8192
  temperature: 0.3
  maxAttempts: 1
`)

	cfg, err := LoadFromFile(path)
	require.NoError(t, err)
	require.Equal(t, "env-key", cfg.Planner.APIKey)
	require.Equal(t, "https://gateway.local", cfg.Planner.BaseURL)
	require.Equal(t, "deepseek-v4-flash", cfg.Planner.Model)
	require.Equal(t, 4096, cfg.Planner.MaxTokens)
	require.Equal(t, 0.0, cfg.Planner.Temperature)
	require.Equal(t, 3, cfg.Planner.MaxAttempts)
}
```

- [ ] **Step 2: Run failing config tests**

Run:

```bash
go test ./apps/control-plane/internal/config -run 'TestLoadFromFilePlannerConfig|TestPlannerEnvOverridesFileConfig'
```

Expected: fail because `Config.Planner` does not exist.

- [ ] **Step 3: Implement planner config**

Add to `config.go`:

```go
type Config struct {
	HTTP        HTTPConfig        `yaml:"http"`
	Postgres    PostgresConfig    `yaml:"postgres"`
	Redis       RedisConfig       `yaml:"redis"`
	ObjectStore ObjectStoreConfig `yaml:"objectStore"`
	Temporal    TemporalConfig    `yaml:"temporal"`
	Planner     PlannerConfig     `yaml:"planner"`
}

type PlannerConfig struct {
	Provider    string  `yaml:"provider"`
	APIKey      string  `yaml:"apiKey"`
	BaseURL     string  `yaml:"baseURL"`
	Model       string  `yaml:"model"`
	MaxTokens   int     `yaml:"maxTokens"`
	Temperature float64 `yaml:"temperature"`
	MaxAttempts int     `yaml:"maxAttempts"`
}
```

Update `defaultConfig()`:

```go
Planner: PlannerConfig{
	Provider:    "deepseek",
	BaseURL:     "https://api.deepseek.com",
	Model:       "deepseek-chat",
	MaxTokens:   8192,
	Temperature: 0,
	MaxAttempts: 2,
},
```

The default `Model` must be a real DeepSeek model id. DeepSeek's OpenAI-compatible API exposes `deepseek-chat` and `deepseek-reasoner`; an invented id like `deepseek-v4-pro` will fail the Task 12 real smoke with a model-not-found error. Confirm the current model id against DeepSeek docs before the smoke and override via `PLANNER_MODEL` if a newer id is preferred.

Update `applyEnv`:

```go
cfg.Planner.Provider = envOrDefault("PLANNER_PROVIDER", cfg.Planner.Provider)
cfg.Planner.APIKey = envOrDefault("PLANNER_API_KEY", cfg.Planner.APIKey)
cfg.Planner.BaseURL = envOrDefault("PLANNER_BASE_URL", cfg.Planner.BaseURL)
cfg.Planner.Model = envOrDefault("PLANNER_MODEL", cfg.Planner.Model)
if value := os.Getenv("PLANNER_MAX_TOKENS"); value != "" {
	if parsed, err := strconv.Atoi(value); err == nil {
		cfg.Planner.MaxTokens = parsed
	}
}
if value := os.Getenv("PLANNER_TEMPERATURE"); value != "" {
	if parsed, err := strconv.ParseFloat(value, 64); err == nil {
		cfg.Planner.Temperature = parsed
	}
}
if value := os.Getenv("PLANNER_MAX_ATTEMPTS"); value != "" {
	if parsed, err := strconv.Atoi(value); err == nil {
		cfg.Planner.MaxAttempts = parsed
	}
}
```

Do not make planner config required in `validate()`. Missing credentials must trigger heuristic fallback, not process startup failure.

- [ ] **Step 4: Update example config**

Add:

```yaml
planner:
  provider: "deepseek"
  apiKey: "replace-with-deepseek-api-key"
  baseURL: "https://api.deepseek.com"
  model: "deepseek-chat"
  maxTokens: 8192
  temperature: 0
  maxAttempts: 2
```

- [ ] **Step 5: Run config tests**

Run:

```bash
go test ./apps/control-plane/internal/config -run 'TestLoadFromFilePlannerConfig|TestPlannerEnvOverridesFileConfig'
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/config/config.go apps/control-plane/internal/config/config_test.go apps/control-plane/config/config.example.yaml
git commit -m "feat: add project planner config"
```

## Task 2: Add Migration 019 And sqlc Queries

**Files:**

- Create: `apps/control-plane/internal/storage/migrations/019_project_task_dependencies.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Generated: `apps/control-plane/internal/storage/queries/project.sql.go`
- Generated: `apps/control-plane/internal/storage/queries/models.go`
- Generated: `apps/control-plane/internal/storage/queries/querier.go`

- [ ] **Step 1: Write migration tests**

Add assertions to `migrations_test.go`:

```go
func TestProjectTaskDependenciesMigrationHasTenantFirstIndexes(t *testing.T) {
	sql := migrationsSQL(t)
	block := createTableBlock(t, sql, "project_task_dependencies")
	for _, fragment := range []string{
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"project_id UUID NOT NULL",
		"dependent_task_id UUID NOT NULL",
		"blocker_task_id UUID NOT NULL",
	} {
		if !strings.Contains(block, fragment) {
			t.Fatalf("expected project_task_dependencies block to include %q, got:\n%s", fragment, block)
		}
	}
	for _, fragment := range []string{
		"uq_ptd_edge",
		"idx_ptd_tenant_project_dependent",
		"idx_ptd_blocker",
		"idx_ptd_coordination_job",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected migration to include %q", fragment)
		}
	}
}

func TestProjectTasksMigrationAddsGraphContractColumns(t *testing.T) {
	sql := migrationsSQL(t)
	for _, fragment := range []string{
		"ADD COLUMN coordination_job_id UUID",
		"ADD COLUMN route_decision_id UUID",
		"ADD COLUMN planned_task_key VARCHAR(100)",
		"ADD COLUMN expected_outputs JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ADD COLUMN input_requirements JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD COLUMN handoff_contract JSONB NOT NULL DEFAULT '{}'::jsonb",
		"ADD COLUMN planner_metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"uq_project_tasks_coordination_planned_key",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected project task graph migration fragment %q", fragment)
		}
	}
}
```

- [ ] **Step 2: Run failing migration tests**

Run:

```bash
go test ./apps/control-plane/internal/storage -run 'TestProjectTaskDependenciesMigrationHasTenantFirstIndexes|TestProjectTasksMigrationAddsGraphContractColumns'
```

Expected: fail because migration 019 does not exist.

- [ ] **Step 3: Create migration**

Create `019_project_task_dependencies.sql` with:

```sql
ALTER TABLE project_tasks
    ADD COLUMN coordination_job_id UUID,
    ADD COLUMN route_decision_id UUID,
    ADD COLUMN planned_task_key VARCHAR(100),
    ADD COLUMN task_kind VARCHAR(100),
    ADD COLUMN stage_index INTEGER,
    ADD COLUMN expected_outputs JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN input_requirements JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN handoff_contract JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN planner_metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN project_tasks.coordination_job_id IS '创建该任务的项目协调作业ID，由应用层校验租户和项目范围';
COMMENT ON COLUMN project_tasks.route_decision_id IS '创建该任务的路由决策ID，由应用层校验租户和项目范围';
COMMENT ON COLUMN project_tasks.planned_task_key IS 'Planner在同一协调图内生成的稳定任务键，用于幂等重放和图展示';
COMMENT ON COLUMN project_tasks.task_kind IS '任务类型开放字符串，例如 analysis、implementation、review、test 或 summary，由应用层注册校验';
COMMENT ON COLUMN project_tasks.stage_index IS '任务在规划图中的展示阶段序号，执行事实仍以依赖边为准';
COMMENT ON COLUMN project_tasks.expected_outputs IS '任务级输出契约数组，用于完成前校验和下游交接';
COMMENT ON COLUMN project_tasks.input_requirements IS '任务输入要求JSON，描述执行该任务需要的上下文切片';
COMMENT ON COLUMN project_tasks.handoff_contract IS '任务交接契约JSON，描述下游可消费的证据、工件、结论和引用要求';
COMMENT ON COLUMN project_tasks.planner_metadata IS 'Planner审计摘要JSON，不保存长prompt或模型原文';
COMMENT ON COLUMN project_tasks.status IS '任务状态：pending, planned, blocked, assigned, running, waiting_human, completed, failed, cancelled';

CREATE UNIQUE INDEX uq_project_coordination_jobs_trigger
    ON project_coordination_jobs(tenant_id, workflow_id, trigger_event_id, job_type)
    WHERE trigger_event_id IS NOT NULL;

CREATE UNIQUE INDEX uq_project_route_decisions_job
    ON project_route_decisions(tenant_id, coordination_job_id);

CREATE TABLE project_task_dependencies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    coordination_job_id UUID,
    dependent_task_id UUID NOT NULL,
    blocker_task_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE project_task_dependencies IS '项目任务依赖边，记录一个任务被另一个任务完成结果阻塞的DAG关系';
COMMENT ON COLUMN project_task_dependencies.id IS '任务依赖边ID';
COMMENT ON COLUMN project_task_dependencies.tenant_id IS '租户ID';
COMMENT ON COLUMN project_task_dependencies.project_id IS '所属项目ID';
COMMENT ON COLUMN project_task_dependencies.coordination_job_id IS '生成该依赖边的协调作业ID';
COMMENT ON COLUMN project_task_dependencies.dependent_task_id IS '被阻塞的项目任务ID';
COMMENT ON COLUMN project_task_dependencies.blocker_task_id IS '必须先完成的前置项目任务ID';
COMMENT ON COLUMN project_task_dependencies.created_at IS '依赖边创建时间';

CREATE UNIQUE INDEX uq_project_tasks_coordination_planned_key
    ON project_tasks(tenant_id, project_id, coordination_job_id, planned_task_key)
    WHERE coordination_job_id IS NOT NULL AND planned_task_key IS NOT NULL;

CREATE UNIQUE INDEX uq_ptd_edge
    ON project_task_dependencies(tenant_id, dependent_task_id, blocker_task_id);

CREATE INDEX idx_ptd_tenant_project_dependent
    ON project_task_dependencies(tenant_id, project_id, dependent_task_id);

CREATE INDEX idx_ptd_blocker
    ON project_task_dependencies(tenant_id, blocker_task_id);

CREATE INDEX idx_project_tasks_coordination_job
    ON project_tasks(tenant_id, project_id, coordination_job_id, stage_index);

CREATE INDEX idx_ptd_coordination_job
    ON project_task_dependencies(tenant_id, project_id, coordination_job_id);
```

- [ ] **Step 4: Add sqlc queries**

Append queries to `internal/storage/queries/project.sql`:

```sql
-- name: GetProjectCoordinationJobByTrigger :one
SELECT * FROM project_coordination_jobs
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND workflow_id = sqlc.arg('workflow_id')::varchar
  AND trigger_event_id = sqlc.arg('trigger_event_id')::uuid
  AND job_type = sqlc.arg('job_type')::varchar;

-- name: GetProjectRouteDecisionByCoordinationJob :one
SELECT * FROM project_route_decisions
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND coordination_job_id = sqlc.arg('coordination_job_id')::uuid;

-- name: CreateProjectTaskDependency :one
INSERT INTO project_task_dependencies (
    tenant_id, project_id, coordination_job_id, dependent_task_id, blocker_task_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.arg('dependent_task_id')::uuid,
    sqlc.arg('blocker_task_id')::uuid
) RETURNING *;

-- name: ListProjectTaskDependencies :many
SELECT * FROM project_task_dependencies
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND dependent_task_id = ANY(sqlc.arg('dependent_task_ids')::uuid[])
ORDER BY created_at ASC;

-- name: ListDependentsOfTask :many
SELECT dependent_task_id
FROM project_task_dependencies
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND blocker_task_id = sqlc.arg('blocker_task_id')::uuid
ORDER BY created_at ASC;

-- name: ListUnresolvedBlockersForTasks :many
SELECT
    d.dependent_task_id,
    d.blocker_task_id,
    b.status AS blocker_status
FROM project_task_dependencies d
JOIN project_tasks b
  ON b.tenant_id = d.tenant_id
 AND b.id = d.blocker_task_id
WHERE d.tenant_id = sqlc.arg('tenant_id')::uuid
  AND d.project_id = sqlc.arg('project_id')::uuid
  AND d.dependent_task_id = ANY(sqlc.arg('dependent_task_ids')::uuid[])
  AND b.status <> 'completed'
ORDER BY d.dependent_task_id, d.created_at ASC;

-- name: ListProjectTasksByCoordinationJob :many
SELECT * FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND coordination_job_id = sqlc.arg('coordination_job_id')::uuid
ORDER BY stage_index ASC NULLS LAST, created_at ASC;

-- name: GetProjectTaskCompletionContract :one
SELECT id, tenant_id, project_id, expected_outputs, handoff_contract, digital_employee_run_id
FROM project_tasks
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('id')::uuid;
```

Update existing `CreateProjectTask` insert/select to include new nullable graph fields and JSON fields.

The new `uq_project_coordination_jobs_trigger` and `uq_project_route_decisions_job` indexes are added to **already-populated** tables. On a database that pre-dates this migration and already holds duplicate `(tenant_id, workflow_id, trigger_event_id, job_type)` jobs or duplicate `(tenant_id, coordination_job_id)` route decisions, `CREATE UNIQUE INDEX` will fail. This is expected to be applied against a clean development DB (see Task 12 Step 6); if `migrate-up` fails on the uniqueness, treat it as a data-state blocker, do not weaken the index. After this migration, `CreateCoordinationJob` / `PersistRouteDecision` should rely on these constraints for replay idempotency rather than re-implementing dedup in Go.

- [ ] **Step 5: Generate sqlc and atlas hash**

Requires the `atlas` CLI on PATH (there is no `make migrate-hash` target). Verify with `atlas version` first; if it is missing, install it before running the hash step or `atlas.sum` will drift from the migration and later `migrate-status` will fail. Run:

```bash
make -C apps/control-plane generate-sqlc
cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations
```

Expected: sqlc generated files update and `atlas.sum` changes.

- [ ] **Step 6: Run migration/sqlc tests**

Run:

```bash
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/storage/queries
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/019_project_task_dependencies.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries/project.sql apps/control-plane/internal/storage/queries/project.sql.go apps/control-plane/internal/storage/queries/models.go apps/control-plane/internal/storage/queries/querier.go
git commit -m "feat: add project task graph schema"
```

## Task 3: Add Domain Types And Row Mapping

**Files:**

- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Create: `apps/control-plane/internal/project/task_graph_types.go`

- [ ] **Step 1: Write failing mapping test**

In `pg_repository_test.go`, add a test around creating a ProjectTask with graph fields after migration fixtures exist:

```go
func TestCreateProjectTaskPersistsGraphContractFields(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	jobID := uuid.New()
	routeID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	stage := int32(2)

	task, err := repo.CreateProjectTask(context.Background(), project.CreateProjectTaskRequest{
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		DemandID:                  &demandID,
		Title:                     "复核证据",
		Summary:                   "检查上游产物",
		Status:                    "blocked",
		AssignedDigitalEmployeeID: &employeeID,
		CoordinationJobID:         &jobID,
		RouteDecisionID:           &routeID,
		PlannedTaskKey:            strPtr("review_1"),
		TaskKind:                  strPtr("review"),
		StageIndex:                &stage,
		ExpectedOutputs:           []any{"execution_summary", "evidence_refs"},
		InputRequirements:         map[string]any{"needs": []any{"implementation_summary"}},
		HandoffContract:           map[string]any{"required_refs": []any{"evidence_refs"}},
		PlannerMetadata:           map[string]any{"planner": "test"},
	})
	require.NoError(t, err)
	require.Equal(t, "review_1", *task.PlannedTaskKey)
	require.Equal(t, []any{"execution_summary", "evidence_refs"}, task.ExpectedOutputs)
	require.Equal(t, "test", task.PlannerMetadata["planner"])
}
```

- [ ] **Step 2: Run failing test**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestCreateProjectTaskPersistsGraphContractFields
```

Expected: fail because domain fields do not exist.

- [ ] **Step 3: Add ProjectTask fields**

Extend `ProjectTask` and `CreateProjectTaskRequest`:

```go
type ProjectTask struct {
	ID                        uuid.UUID
	TenantID                  uuid.UUID
	ProjectID                 uuid.UUID
	DemandID                  *uuid.UUID
	Title                     string
	Summary                   *string
	Status                    string
	AssignedDigitalEmployeeID *uuid.UUID
	RuntimeTaskID             *uuid.UUID
	DigitalEmployeeRunID      *uuid.UUID
	RiskLevel                 *string
	RequiresHumanApproval     bool
	CoordinationJobID         *uuid.UUID
	RouteDecisionID           *uuid.UUID
	PlannedTaskKey            *string
	TaskKind                  *string
	StageIndex                *int32
	ExpectedOutputs           []any
	InputRequirements         map[string]any
	HandoffContract           map[string]any
	PlannerMetadata           map[string]any
	BlockedByTaskIDs          []uuid.UUID
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}
```

Use the same new fields in `CreateProjectTaskRequest`.

- [ ] **Step 4: Add graph read-model domain types**

Create `task_graph_types.go`:

```go
package project

import "github.com/google/uuid"

type ProjectTaskDependency struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID *uuid.UUID
	DependentTaskID   uuid.UUID
	BlockerTaskID     uuid.UUID
}

type ProjectTaskGraph struct {
	Nodes              []ProjectTaskGraphNode
	Edges              []ProjectTaskGraphEdge
	Employees          []ProjectTaskGraphEmployee
	Runs               []ProjectTaskGraphRun
	ExecutionSummaries []ExecutionSummary
	RecentEvents       []ProjectEvent
	DecisionRequests   []DecisionRequest
}

type ProjectTaskGraphNode struct {
	Task ProjectTask
}

type ProjectTaskGraphEdge struct {
	DependentTaskID   uuid.UUID
	BlockerTaskID     uuid.UUID
	CoordinationJobID *uuid.UUID
	EdgeStatus        string
}

type ProjectTaskGraphEmployee struct {
	DigitalEmployeeID uuid.UUID
	DisplayName       string
	ProjectRole       ProjectRole
	Status            string
}

type ProjectTaskGraphRun struct {
	ProjectTaskID        uuid.UUID
	DigitalEmployeeRunID *uuid.UUID
	RuntimeTaskID        *uuid.UUID
	Status               string
	ProviderType         string
}
```

- [ ] **Step 5: Update repository interface**

Add methods listed in the spec:

```go
CreateProjectTaskGraph(ctx context.Context, req CreateProjectTaskGraphRequest) (CreateProjectTaskGraphResult, error)
ListProjectTaskDependencies(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependency, error)
ListDependentsOfTask(ctx context.Context, tenantID, projectID, blockerTaskID uuid.UUID) ([]uuid.UUID, error)
ListUnresolvedBlockersForTasks(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependencyReadiness, error)
ListProjectTasksByCoordinationJob(ctx context.Context, tenantID, projectID, coordinationJobID uuid.UUID) ([]ProjectTask, error)
GetProjectTaskCompletionContract(ctx context.Context, tenantID, taskID uuid.UUID) (ProjectTaskCompletionContract, error)
GetCoordinationJobByTrigger(ctx context.Context, tenantID uuid.UUID, workflowID string, triggerEventID uuid.UUID, jobType string) (CoordinationJob, error)
GetRouteDecisionByCoordinationJob(ctx context.Context, tenantID, coordinationJobID uuid.UUID) (RouteDecision, error)
GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (ProjectTaskGraph, error)
```

Define request/result structs next to existing repository request structs.

- [ ] **Step 6: Update row mapping**

Update `taskFromRecord` to map nullable UUIDs, nullable strings, nullable ints, and JSON fields:

```go
ExpectedOutputs:   jsonbArrayOrEmpty(row.ExpectedOutputs),
InputRequirements: jsonbObjectOrEmpty(row.InputRequirements),
HandoffContract:   jsonbObjectOrEmpty(row.HandoffContract),
PlannerMetadata:   jsonbObjectOrEmpty(row.PlannerMetadata),
```

Add helpers if missing:

```go
func int32PtrFromSQL(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	v := value.Int32
	return &v
}
```

- [ ] **Step 7: Run mapping tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestCreateProjectTaskPersistsGraphContractFields
```

Expected: pass.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/task_graph_types.go apps/control-plane/internal/project/pg_repository_test.go
git commit -m "feat: add project task graph domain types"
```

## Task 4: Define Graph Planner Types And Validation Gates

This task replaces the current single-task `RouteDecisionPlan` (`planner.go:33-43`) with the graph shape that every later task assumes, and keeps the package compiling by reworking the two existing consumers of the old shape (`PersistRouteDecision` and `CreateProjectTasks` in `project_store.go:113-160`). The type redefinition and the consumer rework must land in the same commit, because changing `RouteDecisionPlan` breaks compilation immediately.

**Files:**

- Modify: `apps/control-plane/internal/workflow/projectcoordination/planner.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/graph_validation.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go`

- [ ] **Step 1: Define graph planner contract types**

Replace the single-task `RouteDecisionPlan` and free-function planner in `planner.go` with the graph shape, planner interface, and heuristic fallback. Remove the old top-level fields (`CandidateDigitalEmployeeIDs`, `SelectedDigitalEmployeeIDs`, `InputRequirements`, `ExpectedOutputs []string`, `TaskTitle`, `TaskSummary`); they are now derived per task.

```go
type RoutePlanner interface {
	Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error)
}

type RouteDecisionPlan struct {
	Reason              string
	RequiresHumanReview bool
	BudgetEstimate      map[string]any
	TemplateKey         string
	PlannerMetadata     map[string]any
	Tasks               []PlannedTask
}

type PlannedTask struct {
	Key                   string
	Title                 string
	Summary               string
	SelectedEmployeeID    uuid.UUID
	TaskKind              string
	StageIndex            *int32
	RiskLevel             string
	RequiresHumanApproval bool
	ExpectedOutputs       []string
	InputRequirements     map[string]any
	HandoffContract       map[string]any
	BlockedByKeys         []string
}

type HeuristicRoutePlanner struct{}
```

`HeuristicRoutePlanner.Plan` returns a single-node graph: one `PlannedTask` with no `BlockedByKeys`, assigned to the first active executor, carrying the prior default `ExpectedOutputs` (`execution_summary`, `evidence_refs`, `recommended_next_action`), non-nil `InputRequirements` and `HandoffContract`, and `RequiresHumanReview` from `highRiskPolicyEnabled`. Keep a compatibility wrapper so existing callers (`activities.go:46`, `planner_test.go`) still compile:

```go
func PlanDemandRoute(snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	return HeuristicRoutePlanner{}.Plan(context.Background(), snapshot)
}
```

This also makes a later "emit graph from heuristic" step unnecessary; the heuristic emits a graph from the start.

- [ ] **Step 2: Rework route persistence and task creation for the graph shape**

`PersistRouteDecision` (`project_store.go:113-143`) and `CreateProjectTasks` (`project_store.go:145-160`) read fields that no longer exist on `RouteDecisionPlan`. Update both so the package compiles and `project_store_test.go` still passes:

- `PersistRouteDecision`: derive route-decision-level fields from `Decision.Tasks` instead of the removed top-level fields:
  - `SelectedDigitalEmployeeIDs` = de-duplicated `Tasks[].SelectedEmployeeID`.
  - `CandidateDigitalEmployeeIDs` = same de-duplicated set (the planner no longer emits a separate candidate list).
  - `ExpectedOutputs` = de-duplicated union of all `Tasks[].ExpectedOutputs`.
  - `InputRequirements` = `{"tasks": [...]}` structured summary keyed by `PlannedTask.Key` (do not store long prompts).
  - `BudgetEstimate` = `Decision.BudgetEstimate`; `Reason` = `Decision.Reason`; `RequiresHumanReview` = `Decision.RequiresHumanReview`.
  - Add a small `aggregateRouteDecisionFields(plan RouteDecisionPlan)` helper rather than inlining.
- `CreateProjectTasks`: iterate `Decision.Tasks` (one task per `PlannedTask`) instead of `SelectedDigitalEmployeeIDs`, mapping title/summary/assignee/risk per task. This is a flat interim implementation with no dependency edges; **Task 6 replaces it with `CreateProjectTaskGraph`.** Update `project_store_test.go` expectations to the per-task shape.

Run `go test ./apps/control-plane/internal/workflow/projectcoordination` and confirm the package compiles and existing store tests pass before moving on.

- [ ] **Step 3: Write validation tests**

Add tests:

```go
func TestValidateTaskGraphRejectsCycle(t *testing.T) {
	employeeID := uuid.New()
	plan := RouteDecisionPlan{Tasks: []PlannedTask{
		{Key: "a", Title: "A", Summary: "A", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, BlockedByKeys: []string{"b"}},
		{Key: "b", Title: "B", Summary: "B", SelectedEmployeeID: employeeID, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, BlockedByKeys: []string{"a"}},
	}}
	err := ValidateRouteDecisionGraph(plan, []uuid.UUID{employeeID}, GraphValidationPolicy{MaxTasks: 10})
	require.ErrorIs(t, err, ErrInvalidRouteDecision)
}

func TestValidateTaskGraphAcceptsParallelRoots(t *testing.T) {
	first := uuid.New()
	second := uuid.New()
	plan := RouteDecisionPlan{
		Reason: "parallel work",
		Tasks: []PlannedTask{
			{Key: "a", Title: "A", Summary: "A", SelectedEmployeeID: first, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}},
			{Key: "b", Title: "B", Summary: "B", SelectedEmployeeID: second, ExpectedOutputs: []string{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}},
		},
	}
	require.NoError(t, ValidateRouteDecisionGraph(plan, []uuid.UUID{first, second}, GraphValidationPolicy{MaxTasks: 10}))
}
```

Also cover duplicate key, empty expected outputs, dangling blocker, no root, nil employee, and task count over limit.

- [ ] **Step 4: Run failing validation tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestValidateTaskGraph'
```

Expected: fail because graph validation does not exist.

- [ ] **Step 5: Add graph validation types and errors**

Implement:

```go
type GraphValidationPolicy struct {
	MaxTasks int
}

func ValidateRouteDecisionGraph(plan RouteDecisionPlan, poolIDs []uuid.UUID, policy GraphValidationPolicy) error {
	if strings.TrimSpace(plan.Reason) == "" || len(plan.Tasks) == 0 {
		return ErrInvalidRouteDecision
	}
	if policy.MaxTasks <= 0 {
		policy.MaxTasks = 12
	}
	if len(plan.Tasks) > policy.MaxTasks {
		return ErrInvalidRouteDecision
	}
	pool := uuidSet(poolIDs)
	keys := map[string]PlannedTask{}
	for _, task := range plan.Tasks {
		key := strings.TrimSpace(task.Key)
		if key == "" || strings.TrimSpace(task.Title) == "" || strings.TrimSpace(task.Summary) == "" {
			return ErrInvalidRouteDecision
		}
		if _, exists := keys[key]; exists {
			return ErrInvalidRouteDecision
		}
		if _, ok := pool[task.SelectedEmployeeID]; !ok {
			return ErrInvalidRouteDecision
		}
		if len(task.ExpectedOutputs) == 0 || task.InputRequirements == nil || task.HandoffContract == nil {
			return ErrInvalidRouteDecision
		}
		keys[key] = task
	}
	for _, task := range plan.Tasks {
		for _, blocker := range task.BlockedByKeys {
			blocker = strings.TrimSpace(blocker)
			if blocker == "" || blocker == task.Key {
				return ErrInvalidRouteDecision
			}
			if _, ok := keys[blocker]; !ok {
				return ErrInvalidRouteDecision
			}
		}
	}
	if !hasRoot(plan.Tasks) || hasCycle(plan.Tasks) {
		return ErrInvalidRouteDecision
	}
	return nil
}
```

Add `hasRoot`, `hasCycle`, and `uuidSet` helpers in the same file. The graph-aware `RouteDecisionPlan`, `PlannedTask`, `RoutePlanner`, and `HeuristicRoutePlanner` were already defined in Step 1, so no further planner-shape change is needed here.

- [ ] **Step 6: Run planner tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestValidateTaskGraph|TestPlanDemandRoute'
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/planner.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/workflow/projectcoordination/graph_validation.go apps/control-plane/internal/workflow/projectcoordination/graph_validation_test.go apps/control-plane/internal/workflow/projectcoordination/planner_test.go
git commit -m "feat: graph-aware route plan types and validation"
```

## Task 5: Add DeepSeek Planner

**Files:**

- Modify (only if using the official SDK instead of the `net/http` adapter): `apps/control-plane/go.mod`
- Modify (only if using the official SDK instead of the `net/http` adapter): `apps/control-plane/go.sum`
- Create: `apps/control-plane/internal/workflow/projectcoordination/deepseek_planner.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/deepseek_planner_test.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`

- [ ] **Step 1: Choose the client transport (no SDK by default)**

Default to a narrow `net/http` adapter behind the `chatCompletionClient` interface that POSTs OpenAI-compatible JSON to `{baseURL}/chat/completions` with bearer auth and `response_format: {"type":"json_object"}`. This avoids adding an unused heavy dependency and keeps DeepSeek-compatible extra fields easy to set.

Only if you decide to use the official SDK instead, run:

```bash
cd apps/control-plane && go get github.com/openai/openai-go/v3@latest
```

and remove the `go.mod`/`go.sum` edits from this task's file list if you keep the `net/http` adapter. Do not add the dependency unless it is actually imported, or `go mod tidy` / the build will flag it.

- [ ] **Step 2: Write fake client planner tests**

Add tests for:

```go
func TestDeepSeekRoutePlannerParsesJSONGraph(t *testing.T) {
	employeeID := uuid.New()
	planner := NewDeepSeekRoutePlanner(DeepSeekPlannerConfig{
		APIKey: "test-key",
		BaseURL: "https://api.deepseek.com",
		Model: "deepseek-chat",
		MaxTokens: 1024,
		MaxAttempts: 1,
	}, fakeChatCompletionClient{content: fmt.Sprintf(`{
		"reason":"split demand",
		"requires_human_review":false,
		"tasks":[
			{"key":"t1","title":"分析","summary":"分析需求","selected_employee_id":%q,"stage_index":0,"expected_outputs":["execution_summary"],"input_requirements":{},"handoff_contract":{},"blocked_by_keys":[],"risk_level":"medium","task_kind":"analysis"}
		],
		"budget_estimate":{"mode":"planner"},
		"template_key":"default",
		"planner_metadata":{"provider":"deepseek"}
	}`, employeeID.String())})

	plan, err := planner.Plan(context.Background(), CoordinationSnapshot{
		Demand: DemandSnapshot{ID: uuid.New(), Title: "需求", Content: "内容"},
		DigitalEmployeePool: []ProjectMemberSnapshot{{PrincipalID: employeeID, ProjectRole: "executor", Status: "active"}},
	})
	require.NoError(t, err)
	require.Len(t, plan.Tasks, 1)
	require.Equal(t, employeeID, plan.Tasks[0].SelectedEmployeeID)
}
```

Define a tiny interface around the SDK so tests do not call network:

```go
type chatCompletionClient interface {
	CreateChatCompletion(ctx context.Context, req DeepSeekChatRequest) (string, error)
}
```

- [ ] **Step 3: Run failing DeepSeek tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestDeepSeekRoutePlanner
```

Expected: fail because planner does not exist.

- [ ] **Step 4: Implement planner**

Implement `DeepSeekRoutePlanner` with:

```go
type DeepSeekPlannerConfig struct {
	APIKey      string
	BaseURL     string
	Model       string
	MaxTokens   int
	Temperature float64
	MaxAttempts int
}

func (p *DeepSeekRoutePlanner) Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	if strings.TrimSpace(p.cfg.APIKey) == "" || strings.TrimSpace(p.cfg.BaseURL) == "" || strings.TrimSpace(p.cfg.Model) == "" {
		return RouteDecisionPlan{}, ErrPlannerUnavailable
	}
	var lastErr error
	attempts := p.cfg.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		content, err := p.client.CreateChatCompletion(ctx, DeepSeekChatRequest{
			Model:       p.cfg.Model,
			System:      buildPlannerSystemPrompt(),
			User:        buildPlannerUserPrompt(snapshot),
			MaxTokens:   p.cfg.MaxTokens,
			Temperature: p.cfg.Temperature,
		})
		if err != nil {
			lastErr = err
			continue
		}
		plan, err := decodePlannerJSON(content)
		if err != nil {
			lastErr = err
			continue
		}
		pool := activeExecutorIDs(snapshot.DigitalEmployeePool)
		if err := ValidateRouteDecisionGraph(plan, pool, GraphValidationPolicy{MaxTasks: 12}); err != nil {
			lastErr = err
			continue
		}
		return plan, nil
	}
	return RouteDecisionPlan{}, lastErr
}
```

The adapter must set `response_format` to `{"type":"json_object"}`. DeepSeek's json-object mode additionally **requires the literal word `json` to appear in the prompt**; `buildPlannerSystemPrompt` must explicitly instruct the model to return a single JSON object matching the schema (and `buildPlannerUserPrompt` should restate "respond with JSON only"). If `json` is absent the request is rejected, and a model that emits prose instead of JSON will fail `decodePlannerJSON` and fall through to the heuristic. Keep the public package boundary as `chatCompletionClient`.

- [ ] **Step 5: Inject planner into activities**

Change activities:

```go
type Activities struct {
	store   ActivityStore
	planner RoutePlanner
}

func NewActivities(store ActivityStore, planner ...RoutePlanner) *Activities {
	selected := RoutePlanner(HeuristicRoutePlanner{})
	if len(planner) > 0 && planner[0] != nil {
		selected = planner[0]
	}
	return &Activities{store: store, planner: selected}
}

func (a *Activities) PlanDemandRoute(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	plan, err := a.planner.Plan(ctx, snapshot)
	if err == nil {
		return plan, nil
	}
	return HeuristicRoutePlanner{}.Plan(ctx, snapshot)
}
```

In `app.go`, construct planner from `cfg.Planner` and pass it to `NewActivities`.

- [ ] **Step 6: Run DeepSeek planner tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestDeepSeekRoutePlanner|TestActivities'
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/go.mod apps/control-plane/go.sum apps/control-plane/internal/app/app.go apps/control-plane/internal/workflow/projectcoordination/activities.go apps/control-plane/internal/workflow/projectcoordination/deepseek_planner.go apps/control-plane/internal/workflow/projectcoordination/deepseek_planner_test.go
git commit -m "feat: add deepseek route planner"
```

## Task 6: Implement Graph Persistence

**Files:**

- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`

- [ ] **Step 1: Write repository graph transaction tests**

Add tests:

```go
func TestCreateProjectTaskGraphCreatesTasksEdgesAndEvents(t *testing.T) {
	repo, tenantID := newProjectRepositoryTestStore(t)
	projectID := createProjectFixture(t, repo, tenantID)
	demandID := createDemandFixture(t, repo, tenantID, projectID)
	jobID := createCoordinationJobFixture(t, repo, tenantID, projectID)
	routeID := createRouteDecisionFixture(t, repo, tenantID, projectID, jobID, demandID)
	employeeID := uuid.New()

	result, err := repo.CreateProjectTaskGraph(context.Background(), project.CreateProjectTaskGraphRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DemandID:          demandID,
		CoordinationJobID: jobID,
		RouteDecisionID:   routeID,
		Tasks: []project.ProjectTaskGraphCreateTask{
			{Key: "t1", Title: "分析", Summary: "分析", Status: "planned", AssignedDigitalEmployeeID: employeeID, ExpectedOutputs: []any{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, StageIndex: int32Ptr(0)},
			{Key: "t2", Title: "复核", Summary: "复核", Status: "blocked", AssignedDigitalEmployeeID: employeeID, ExpectedOutputs: []any{"execution_summary"}, InputRequirements: map[string]any{}, HandoffContract: map[string]any{}, StageIndex: int32Ptr(1), BlockedByKeys: []string{"t1"}},
		},
	})
	require.NoError(t, err)
	require.Len(t, result.Tasks, 2)
	require.Len(t, result.Dependencies, 1)
	require.True(t, result.Tasks[0].IsRoot)
	require.False(t, result.Tasks[1].IsRoot)
}
```

Also test idempotent replay and partial graph conflict.

- [ ] **Step 2: Run failing graph persistence tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestCreateProjectTaskGraph'
```

Expected: fail because method does not exist.

- [ ] **Step 3: Implement `CreateProjectTaskGraph`**

Use `withProjectQueries` transaction. Pseudocode to implement exactly:

```go
return withProjectQueries(ctx, r, "project task graph create", func(q *queries.Queries) (CreateProjectTaskGraphResult, error) {
	existing, err := r.listProjectTasksByCoordinationJobWithQueries(ctx, q, req.TenantID, req.ProjectID, req.CoordinationJobID)
	if err != nil {
		return CreateProjectTaskGraphResult{}, err
	}
	if len(existing) > 0 {
		if graphComplete(existing, req.Tasks) {
			return r.graphResultFromExisting(ctx, q, req, existing)
		}
		return CreateProjectTaskGraphResult{}, ErrProjectConflict
	}

	keyToID := map[string]uuid.UUID{}
	created := make([]ProjectTaskGraphTaskResult, 0, len(req.Tasks))
	for _, planned := range req.Tasks {
		summary := planned.Summary
		taskKind := planned.TaskKind
		riskLevel := planned.RiskLevel
		task, err := r.createProjectTaskWithQueries(ctx, q, CreateProjectTaskRequest{
			TenantID:                  req.TenantID,
			ProjectID:                 req.ProjectID,
			DemandID:                  &req.DemandID,
			Title:                     planned.Title,
			Summary:                   summary,
			Status:                    planned.Status,
			AssignedDigitalEmployeeID: &planned.AssignedDigitalEmployeeID,
			RiskLevel:                 riskLevel,
			RequiresHumanApproval:     planned.RequiresHumanApproval,
			CoordinationJobID:         &req.CoordinationJobID,
			RouteDecisionID:           &req.RouteDecisionID,
			PlannedTaskKey:            &planned.Key,
			TaskKind:                  &taskKind,
			StageIndex:                planned.StageIndex,
			ExpectedOutputs:           planned.ExpectedOutputs,
			InputRequirements:         planned.InputRequirements,
			HandoffContract:           planned.HandoffContract,
			PlannerMetadata:           planned.PlannerMetadata,
		})
		if err != nil {
			return CreateProjectTaskGraphResult{}, err
		}
		event, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventTaskCreated,
			ActorType:    "project_coordinator",
			ActorID:      task.ID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(task.ID.String()),
			Summary:      "项目任务已创建",
			Payload: map[string]any{
				"project_task_id": task.ID.String(),
				"demand_id": req.DemandID.String(),
				"coordination_job_id": req.CoordinationJobID.String(),
				"planned_task_key": planned.Key,
			},
		})
		if err != nil {
			return CreateProjectTaskGraphResult{}, err
		}
		keyToID[planned.Key] = task.ID
		created = append(created, ProjectTaskGraphTaskResult{ID: task.ID, PlannedTaskKey: planned.Key, CreatedEventID: event.ID, IsRoot: len(planned.BlockedByKeys) == 0})
	}
	for _, planned := range req.Tasks {
		for _, blockerKey := range planned.BlockedByKeys {
			edge, err := q.CreateProjectTaskDependency(ctx, queries.CreateProjectTaskDependencyParams{
				TenantID:          req.TenantID,
				ProjectID:         req.ProjectID,
				CoordinationJobID: nullUUID(&req.CoordinationJobID),
				DependentTaskID:   keyToID[planned.Key],
				BlockerTaskID:     keyToID[blockerKey],
			})
			if err != nil {
				return CreateProjectTaskGraphResult{}, err
			}
			dependencies = append(dependencies, dependencyFromRecord(edge))
		}
	}
	graphEvent, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
		TenantID:     req.TenantID,
		ProjectID:    req.ProjectID,
		EventType:    ProjectEventTaskGraphPlanned,
		ActorType:    "project_coordinator",
		ActorID:      req.CoordinationJobID.String(),
		ResourceType: strPtr("project_coordination_job"),
		ResourceID:   strPtr(req.CoordinationJobID.String()),
		Summary:      "项目任务图已规划",
		Payload: map[string]any{
			"coordination_job_id": req.CoordinationJobID.String(),
			"route_decision_id": req.RouteDecisionID.String(),
			"task_count": len(req.Tasks),
			"dependency_count": len(dependencies),
		},
	})
	if err != nil {
		return CreateProjectTaskGraphResult{}, err
	}
	return CreateProjectTaskGraphResult{Tasks: created, Dependencies: dependencies, GraphEventID: graphEvent.ID}, nil
})
```

Declare the two project event types this graph path emits next to the existing `ProjectEventTaskCreated` (`types.go:80`), using the same typed `ProjectEventType` so `AppendProjectEvent` stays type-consistent:

```go
ProjectEventTaskGraphPlanned ProjectEventType = "project_task_graph.planned"
```

(`ProjectEventTaskCreated` already exists; `ProjectEventTaskContractMissing` is added in Task 8.) Also declare the `dependencies` slice used above (`dependencies := make([]ProjectTaskDependency, 0)`).

- [ ] **Step 4: Update ProjectStore `CreateProjectTasks`**

Convert `RouteDecisionPlan.Tasks` into `project.CreateProjectTaskGraphRequest`. Root tasks get status `planned`; non-root tasks get status `blocked`. Include `CoordinationJobID` and `RouteDecisionID` in `CreateProjectTasksInput`.

- [ ] **Step 5: Run graph persistence and store tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestCreateProjectTaskGraph'
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStore.*Graph|TestProjectStore.*CreateProjectTasks'
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/workflow/projectcoordination/types.go
git commit -m "feat: persist project task graphs"
```

## Task 7: Dispatch Only Ready Tasks And Wake Downstream

**Files:**

- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Write workflow tests**

Add tests:

```go
func TestProjectCoordinatorDispatchesOnlyRootTasks(t *testing.T) {
	// Configure recordingActivityStore to return two tasks, one root and one blocked.
	// Expected calls include ListDispatchableTasks and one DispatchProjectTask.
}

func TestProjectCoordinatorWakesDownstreamOnCompletion(t *testing.T) {
	// Signal EmployeeTaskCompleted after first dispatch.
	// Store ResolveReadyDownstream returns the blocked task ID.
	// Expected calls include ResolveReadyDownstream then DispatchProjectTask for downstream.
}
```

Update `recordingActivityStore` with:

```go
dispatchableTaskIDs []uuid.UUID
readyDownstreamIDs  []uuid.UUID
```

and methods:

```go
func (s *recordingActivityStore) ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error) {
	s.calls = append(s.calls, "ListDispatchableTasks")
	return s.dispatchableTaskIDs, nil
}

func (s *recordingActivityStore) ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error) {
	s.calls = append(s.calls, "ResolveReadyDownstream")
	return s.readyDownstreamIDs, nil
}
```

- [ ] **Step 2: Run failing workflow tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinatorDispatchesOnlyRootTasks|TestProjectCoordinatorWakesDownstreamOnCompletion'
```

Expected: fail because activities and workflow calls do not exist.

- [ ] **Step 3: Add activity contracts**

Add:

```go
type ListDispatchableTasksInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID uuid.UUID
}

type ResolveReadyDownstreamInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CompletedTaskID uuid.UUID
}
```

Extend `ActivityStore`:

```go
ListDispatchableTasks(ctx context.Context, input ListDispatchableTasksInput) ([]uuid.UUID, error)
ResolveReadyDownstream(ctx context.Context, input ResolveReadyDownstreamInput) ([]uuid.UUID, error)
```

- [ ] **Step 4: Update workflow**

After `CreateProjectTasks`, call:

```go
readyTaskIDs, err := listDispatchableTasks(ctx, input.TenantID, signal.ProjectID, job.ID)
if err != nil {
	return nil, err
}
```

For route review pending state, store `CoordinationJobID`, not all task IDs as dispatch targets. On approval, call `ListDispatchableTasks` again and dispatch the result.

On completed signal:

```go
readyTaskIDs, err := resolveReadyDownstream(ctx, input.TenantID, input.ProjectID, signal.ProjectTaskID)
if err != nil {
	workflowErr = err
	return
}
workflowErr = dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs)
```

- [ ] **Step 5: Implement store methods**

`ListDispatchableTasks`:

1. Load tasks by coordination job.
2. Keep statuses `planned` and `pending`.
3. Call unresolved blocker query.
4. Return IDs with no unresolved blockers.

`ResolveReadyDownstream`:

1. List dependents of completed task.
2. Query unresolved blockers for dependents.
3. For tasks with zero unresolved blockers, update status `blocked -> planned`.
4. Return newly planned task IDs.

- [ ] **Step 6: Run workflow/store tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinatorDispatchesOnlyRootTasks|TestProjectCoordinatorWakesDownstreamOnCompletion|TestProjectStore.*Dispatchable|TestProjectStore.*Downstream'
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/workflow.go apps/control-plane/internal/workflow/projectcoordination/types.go apps/control-plane/internal/workflow/projectcoordination/activities.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/workflow_test.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "feat: dispatch ready project task graph nodes"
```

## Task 8: Enforce Completion Contracts

**Files:**

- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/types.go`

- [ ] **Step 1: Write failing service tests**

Add tests:

```go
func TestCompleteProjectTaskRejectsMissingRequiredEvidence(t *testing.T) {
	service, repo, coordinator := newProjectServiceWritebackFixture(t)
	task := repo.addRunningProjectTask(project.ProjectTask{
		ExpectedOutputs: []any{"execution_summary", "evidence_refs"},
		HandoffContract: map[string]any{},
	})

	_, err := service.CompleteProjectTask(context.Background(), project.CompleteProjectTaskRequest{
		TenantID:          task.TenantID,
		RuntimeNodeID:     *task.RuntimeTaskID,
		ProjectTaskID:     task.ID,
		DigitalEmployeeID: *task.AssignedDigitalEmployeeID,
		Conclusion:        "完成",
		EvidenceRefs:      nil,
	})
	require.ErrorIs(t, err, project.ErrInvalidProjectEvidence)
	require.False(t, coordinator.completedSignaled)
	require.Contains(t, repo.eventTypes(), project.ProjectEventTaskContractMissing)
}
```

Also test valid completion signals workflow and `missing_information` must be explicitly present when required.

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestCompleteProjectTaskRejectsMissingRequiredEvidence|TestCompleteProjectTask'
```

Expected: fail because contract validation does not exist.

- [ ] **Step 3: Add contract validation helper**

Implement in `service.go`:

```go
func validateProjectTaskCompletionContract(task ProjectTask, req CompleteProjectTaskRequest, runWorkProducts []any) error {
	required := stringSetFromAny(task.ExpectedOutputs)
	if required["execution_summary"] && strings.TrimSpace(req.Conclusion) == "" {
		return ErrInvalidProjectEvidence
	}
	if required["evidence_refs"] && len(req.EvidenceRefs) == 0 {
		return ErrInvalidProjectEvidence
	}
	if required["artifact_refs"] && len(req.ArtifactRefs) == 0 {
		return ErrInvalidProjectEvidence
	}
	if required["recommended_next_action"] && strings.TrimSpace(req.RecommendedNextAction) == "" {
		return ErrInvalidProjectEvidence
	}
	if required["missing_information"] && req.MissingInformation == nil {
		return ErrInvalidProjectEvidence
	}
	if required["work_products"] && len(runWorkProducts) == 0 {
		return ErrInvalidProjectEvidence
	}
	return validateRequiredHandoffRefs(task.HandoffContract, req, runWorkProducts)
}
```

If run work products are not currently accessible through the project repository, add a narrow repository method to load them by `digital_employee_run_id`.

- [ ] **Step 4: Add contract missing event path**

Before returning validation error:

```go
_ = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
	TenantID:     req.TenantID,
	ProjectID:    task.ProjectID,
	EventType:    ProjectEventTaskContractMissing,
	ActorType:    "digital_employee",
	ActorID:      req.DigitalEmployeeID.String(),
	ResourceType: strPtr("project_task"),
	ResourceID:   strPtr(task.ID.String()),
	Summary:      "项目任务完成输出未满足交接契约",
	Payload: map[string]any{
		"project_task_id": task.ID.String(),
		"missing_outputs": missingOutputs,
	},
})
```

Add `ProjectEventTaskContractMissing = "project_task.contract_missing"`.

- [ ] **Step 5: Run service tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestCompleteProjectTaskRejectsMissingRequiredEvidence|TestCompleteProjectTask'
```

Expected: pass.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/service_test.go apps/control-plane/internal/project/types.go
git commit -m "feat: enforce project task completion contracts"
```

## Task 9: Add Failure Recovery Decision Flow

**Files:**

- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/client.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/project/service.go`

- [ ] **Step 1: Write workflow failure recovery tests**

Add:

```go
func TestProjectCoordinatorRequestsFailureRecoveryWhenTaskFails(t *testing.T) {
	// Signal EmployeeTaskFailed.
	// Expected calls: AppendProjectEvent, HoldDownstreamForFailure.
}

func TestProjectCoordinatorRoutesHumanDecisionToFailureRecovery(t *testing.T) {
	// Store returns a failure recovery decision ID.
	// Signal HumanDecisionSubmitted with Payload {"recovery_action":"retry"}.
	// Expected call: ApplyFailureRecoveryDecision.
}
```

- [ ] **Step 2: Run failing tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinatorRequestsFailureRecovery|TestProjectCoordinatorRoutesHumanDecisionToFailureRecovery'
```

Expected: fail.

- [ ] **Step 3: Add signal payload**

Update:

```go
type HumanDecisionSubmitted struct {
	ApprovalRequestID uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
	ResolvedEventID   uuid.UUID
}
```

Update `SignalClient.SignalHumanDecisionSubmitted` to pass payload from project signal type. If the project signal type lacks payload, add it there and update callers.

- [ ] **Step 4: Add activities**

Add inputs:

```go
type HoldDownstreamForFailureInput struct {
	TenantID       uuid.UUID
	ProjectID      uuid.UUID
	FailedTaskID   uuid.UUID
	FailureSummary string
	FailedEventID  uuid.UUID
}

type ApplyFailureRecoveryDecisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	Payload           map[string]any
}
```

Add store/activity methods `HoldDownstreamForFailure` and `ApplyFailureRecoveryDecision`.

- [ ] **Step 5: Implement downstream hold**

In store:

1. Recursively list dependents with `ListDependentsOfTask`.
2. Update non-terminal downstream tasks to `blocked`.
3. Create approval request:

```go
approval.CreateRequestInput{
	TenantID:      input.TenantID,
	ResourceType:  "project_task",
	ResourceID:    input.FailedTaskID,
	RequesterType: "project_coordinator",
	TargetUserID: projectRecord.HumanOwnerUserID,
	DecisionType: "task_failure_recovery",
	Title:        "处理项目任务失败",
	RiskLevel:    "high",
	Options:      []any{"approved", "rejected", "needs_more_evidence"},
	ContextPayload: map[string]any{
		"project_id": input.ProjectID.String(),
		"failed_task_id": input.FailedTaskID.String(),
		"failure_summary": input.FailureSummary,
	},
}
```

4. Create project decision request with `DecisionType: "task_failure_recovery"` and `ProjectTaskID`.
5. Upsert inbox projection.

- [ ] **Step 6: Update workflow pending maps**

Use two maps:

```go
pendingReviews := map[string]pendingRouteDecisionReview{}
pendingFailureRecoveries := map[string]pendingTaskFailureRecovery{}
```

In human decision handler, check route review first, then failure recovery. Unknown decisions only append observed event.

- [ ] **Step 7: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinatorRequestsFailureRecovery|TestProjectCoordinatorRoutesHumanDecisionToFailureRecovery|TestProjectStore.*FailureRecovery'
```

Expected: pass.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/types.go apps/control-plane/internal/workflow/projectcoordination/client.go apps/control-plane/internal/workflow/projectcoordination/workflow.go apps/control-plane/internal/workflow/projectcoordination/activities.go apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/workflow_test.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/project/service.go
git commit -m "feat: add project task failure recovery decisions"
```

## Task 10: Add Recovery Actions And Append-Only Subgraphs

**Files:**

- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`

- [ ] **Step 1: Write recovery action tests**

Add tests:

```go
func TestApplyFailureRecoveryRetryCreatesAppendOnlySubgraph(t *testing.T) {
	// Existing A#1 failed and B#1 blocked downstream.
	// Payload recovery_action=retry.
	// Expect new A#2 task and downstream dependency rewired to A#2.
}

func TestApplyFailureRecoveryReassignRequiresNewEmployee(t *testing.T) {
	// Payload recovery_action=reassign without new_digital_employee_id.
	// Expect ErrInvalidRouteDecision or ErrInvalidProject.
}

func TestApplyFailureRecoveryCancelDownstreamCancelsBlockedDependents(t *testing.T) {
	// Payload recovery_action=cancel_downstream.
	// Expect blocked downstream tasks become cancelled and events are written.
}
```

- [ ] **Step 2: Run failing recovery action tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestApplyFailureRecovery'
```

Expected: fail.

- [ ] **Step 3: Implement payload parser**

```go
type FailureRecoveryAction struct {
	Action               string
	NewDigitalEmployeeID *uuid.UUID
}

func parseFailureRecoveryAction(decision string, payload map[string]any) (FailureRecoveryAction, error) {
	if decision == "needs_more_evidence" {
		return FailureRecoveryAction{Action: "needs_more_evidence"}, nil
	}
	if decision == "rejected" {
		return FailureRecoveryAction{Action: "cancel_downstream"}, nil
	}
	raw, _ := payload["recovery_action"].(string)
	switch raw {
	case "retry", "cancel_downstream":
		return FailureRecoveryAction{Action: raw}, nil
	case "reassign":
		idText, _ := payload["new_digital_employee_id"].(string)
		id, err := uuid.Parse(idText)
		if err != nil {
			return FailureRecoveryAction{}, project.ErrInvalidProject
		}
		return FailureRecoveryAction{Action: raw, NewDigitalEmployeeID: &id}, nil
	default:
		return FailureRecoveryAction{}, project.ErrInvalidProject
	}
}
```

- [ ] **Step 4: Implement deterministic retry/reassign subgraph**

For `retry`, create one replacement task:

- title: original title plus retry suffix.
- same demand.
- same assignee.
- same expected outputs/input/handoff contract.
- status `planned` if original blockers are complete, otherwise `blocked`.
- planner metadata includes `source_task_id`, `recovery_action`, `parent_coordination_job_id`.

For `reassign`, same as retry but assigned to `new_digital_employee_id` after project-member validation.

- [ ] **Step 5: Implement cancel downstream**

Recursively update blocked downstream tasks to `cancelled` with status guard `blocked/planned/pending`, and append one project event per cancelled task.

- [ ] **Step 6: Run recovery action tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestApplyFailureRecovery'
```

Expected: pass.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/project_store.go apps/control-plane/internal/workflow/projectcoordination/types.go apps/control-plane/internal/workflow/projectcoordination/project_store_test.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go
git commit -m "feat: apply project task recovery actions"
```

## Task 11: Add Task-Graph Read API

**Files:**

- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Generated: `apps/web/src/lib/api/generated/control-plane.ts`

- [ ] **Step 1: Write handler test**

Add:

```go
func TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions(t *testing.T) {
	// Seed project, tasks, dependencies, route decision, event, and decision request.
	// GET /api/v1/projects/{projectId}/task-graph?coordination_job_id={jobID}
	// Expect 200 and non-empty nodes/edges.
}
```

- [ ] **Step 2: Run failing handler test**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions
```

Expected: fail because route/handler does not exist.

- [ ] **Step 3: Add OpenAPI path and schemas**

Add path:

```yaml
  /api/v1/projects/{projectId}/task-graph:
    get:
      operationId: getProjectTaskGraph
      summary: Get project task graph read model
      parameters:
        - $ref: "#/components/parameters/ProjectId"
        - name: coordination_job_id
          in: query
          schema:
            type: string
            format: uuid
        - name: demand_id
          in: query
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Project task graph
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectTaskGraph"
```

Add `ProjectTask` fields and `ProjectTaskGraph`, `ProjectTaskGraphNode`, `ProjectTaskGraphEdge`, `ProjectTaskGraphRun`, `ProjectTaskGraphEmployee` schemas matching the spec.

- [ ] **Step 4: Implement service and repository read model**

`Service.GetProjectTaskGraph(ctx, req)` validates tenant/project IDs, normalizes filters, and delegates repository.

Repository read model:

1. Select tasks by coordination job or demand.
2. Batch load dependencies for task IDs.
3. Batch load execution summaries, recent events, decision requests, project members, and run binding summary.
4. Return empty slices, never nil slices.

- [ ] **Step 5: Add handler and route**

Add handler:

```go
func (h *HTTPHandler) GetProjectTaskGraph(w http.ResponseWriter, r *http.Request) {
	tenantID, _, projectID, service, ok := h.projectRouteContext(w, r)
	if !ok {
		return
	}
	req := GetProjectTaskGraphRequest{TenantID: tenantID, ProjectID: projectID}
	if raw := r.URL.Query().Get("coordination_job_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			http.Error(w, "invalid coordination_job_id", http.StatusBadRequest)
			return
		}
		req.CoordinationJobID = &id
	}
	graph, err := service.GetProjectTaskGraph(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, taskGraphResponseFromDomain(graph))
}
```

Register route under project routes.

- [ ] **Step 6: Generate OpenAPI client**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: generated API artifacts update and contract verification passes.

- [ ] **Step 7: Run API tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions
```

Expected: pass.

- [ ] **Step 8: Run Web verification if generated client changed**

Run:

```bash
corepack pnpm verify:web
```

Expected: pass. If it fails only due to current generated type shape, fix existing Web type references without adding task graph UI.

- [ ] **Step 9: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/server.go apps/web/src/lib/api/generated/control-plane.ts
git commit -m "feat: expose project task graph read model"
```

## Task 12: Integration Verification And Real Smoke

**Files:**

- Modify: only test fixtures or docs if verification discovers a real gap.

- [ ] **Step 1: Run focused workflow package tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination
```

Expected: pass.

- [ ] **Step 2: Run project package tests**

Run:

```bash
go test ./apps/control-plane/internal/project
```

Expected: pass.

- [ ] **Step 3: Run full control-plane tests**

Run:

```bash
go test ./apps/control-plane/...
```

Expected: pass.

- [ ] **Step 4: Run contract verification**

Run:

```bash
corepack pnpm verify:contracts
```

Expected: pass.

- [ ] **Step 5: Check local services before real smoke**

Run:

```bash
scripts/dev-services.sh status
```

Expected: identify whether DB, Temporal, Control Plane, Web, Runtime, and Provider are managed and running. Do not kill unmanaged user processes.

- [ ] **Step 6: Apply migration against intended development DB**

Only after confirming `DATABASE_URL` points to a safe development database, run:

```bash
DATABASE_URL="$DATABASE_URL" make -C apps/control-plane migrate-status
DATABASE_URL="$DATABASE_URL" make -C apps/control-plane migrate-up
```

Expected: migration 019 applies and status shows clean.

- [ ] **Step 7: Start current Control Plane and Temporal worker**

Run the repo's dev service command or:

```bash
corepack pnpm dev:control-plane
```

Expected logs include the Temporal worker task queue when `temporal.enabled: true`.

- [ ] **Step 8: Configure DeepSeek planner**

In ignored local config:

```yaml
planner:
  provider: "deepseek"
  apiKey: "${DEEPSEEK_API_KEY}"
  baseURL: "https://api.deepseek.com"
  model: "deepseek-chat"
  maxTokens: 8192
  temperature: 0
  maxAttempts: 2
```

Expected: config loads without printing the secret.

- [ ] **Step 9: Run real DAG smoke**

Use curl or browser against the real Control Plane:

1. Submit a demand likely to split into two tasks.
2. Confirm `GET /api/v1/projects/{projectId}/task-graph` returns at least two nodes and one edge.
3. Confirm root task is dispatched and dependent task is `blocked`.
4. Complete root task through the real Runtime ProjectTask writeback with required outputs.
5. Confirm dependent task becomes dispatched.
6. Submit an invalid completion missing a required output and confirm downstream is not released.
7. Fail a blocker and confirm a `task_failure_recovery` decision request reaches the project human owner.

Expected: all state changes are visible in API responses and project events, with no mock data.

- [ ] **Step 10: Run SuperTeam completion check**

Read and apply:

```bash
sed -n '1,260p' .codex/skills/superteam-completion-check/SKILL.md
```

Expected: final response can truthfully state either real-chain verification passed, or the implementation is blocked by a named dependency.

- [ ] **Step 11: Final commit or fixup**

If verification required code fixes, return to the task that owns the changed files, apply the fix there, rerun that task's tests, and use that task's commit command. Before the final response, run:

```bash
git status --short
```

Expected: only intentional source, test, contract, migration, or generated-code changes are present. Ignored local config and service logs are not commit scope. If no files changed after verification, do not create an empty commit.

## Self-Review Notes

Spec coverage:

- DeepSeek config and json-object planner: Tasks 1 and 5.
- DB graph facts and idempotency: Tasks 2, 3, and 6.
- Graph planner types, route-persistence rework, and validation gates: Task 4.
- Root-only dispatch and downstream wakeup: Task 7.
- Completion contract barrier: Task 8.
- Failure recovery and human decision payload: Task 9.
- Retry, reassign, cancel, append-only subgraphs: Task 10.
- Backend task-graph API and OpenAPI: Task 11.
- Real-chain verification: Task 12.

Type consistency:

- `RouteDecisionPlan.Tasks` is the planner graph source.
- `planned_task_key` maps to `PlannedTask.Key`.
- Dependency edges are `dependent_task_id` waits on `blocker_task_id`.
- Failure recovery payload key is `recovery_action`.
- DeepSeek stable mode is `response_format: {"type":"json_object"}`, and the prompt must contain the literal word `json`.
- Default planner model is a real DeepSeek id (`deepseek-chat`), not an invented one.
- `RouteDecisionPlan` loses its old single-task top-level fields in Task 4; `PersistRouteDecision` derives them from `Tasks`.

Implementation boundary:

- Do not build complex Web graph UI in this plan.
- Do not move planner execution into Runtime/Provider.
- Do not use DeepSeek beta strict function calling as the default path.
- Do not claim the foundation is usable until the real smoke in Task 12 passes or the blocker is explicitly reported.
