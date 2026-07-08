# 数字员工模板管理 CRUD Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn "数字员工模板管理" from a read-only projection of a hardcoded Go catalog into a real DB-backed CRUD feature (create / configure / enable-disable / delete), for both built-in and custom templates, with the create-employee wizard reading templates from the database.

**Architecture:** New `digital_employee_templates` table (tenant-scoped, soft-deletable). Backend CRUD lives inside the existing `internal/employee` Go package (mirrors its `env_*.go`/`run_*.go` file-per-feature convention) with new `template_types.go` / `template_repository.go` / `template_service.go` / `template_handler.go` files, sqlc-generated queries, and new REST routes alongside the existing `/digital-employees/create-options`. The hardcoded `employee_types.go` catalog is deleted (except a `custom_agent` sentinel used only for the "blank custom" creation mode) and every caller that used it switches to a repository-backed, tenant-scoped lookup. Frontend `templates.tsx` gains list actions (create/configure/enable-disable/delete) backed by a new `employee-templates.ts` API client, reusing existing `Dialog`/`ConfirmDialog`/`Textarea`-as-JSON-editor patterns already used elsewhere in the app.

**Tech Stack:** Go 1.x, chi router, sqlc (pgx/v5), Atlas migrations, Postgres JSONB; React + TanStack Router/Query, Vitest browser tests.

## Global Constraints

- Migration files live only in `apps/control-plane/internal/storage/migrations/`; after adding one, run `atlas migrate hash` and `make -C apps/control-plane migrate-validate`.
- Templates are tenant-scoped only (no team/personal scope).
- All template mutations reuse `authz.ActionEmployeeCreate` — no new permission action.
- Built-in (`is_system = true`) templates are fully editable and deletable — `is_system` is informational/UI-badge only, never a mutation guard.
- `type` is immutable after creation; only settable on `POST`.
- `custom_agent` is NOT migrated into the table; it stays a hardcoded sentinel in Go (used only for the blank-custom employee creation mode) and is excluded from `orderedEmployeeTypes` in the frontend as it is today.
- Deleting a template never touches existing `digital_employees` rows — there is no FK between them.
- Web tests run via `corepack pnpm --filter ./apps/web run test` only (never raw `npx vitest`/`playwright install`).
- Web internal navigation uses TanStack Router `Link`/`navigate`, never raw `<a href>`.
- Every task that touches Go code ends with `go vet ./...` and `go test ./...` passing in `apps/control-plane` (never `go build ./...` — see the note on Task 16 Step 1 for why); every task touching web code ends with the web test command passing for the affected files.

---

### Task 1: Migration `050_digital_employee_templates.sql`

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/050_digital_employee_templates.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`

**Interfaces:**
- Produces: table `digital_employee_templates` with columns `id, tenant_id, type, label, description, default_role, recommended_skills, recommended_mcp_servers, recommended_provider_types, default_capability_selection, default_context_policy_override, default_approval_policy, metadata, status, is_system, deleted_at, created_at, updated_at`; unique index `digital_employee_templates_tenant_type_key` on `(tenant_id, type) WHERE deleted_at IS NULL`; 9 seed rows for `platform.DefaultTenantID` (`00000000-0000-0000-0000-000000000001`).

- [ ] **Step 1: Write the migration file**

```sql
-- apps/control-plane/internal/storage/migrations/050_digital_employee_templates.sql
CREATE TABLE digital_employee_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  type VARCHAR(64) NOT NULL,
  label VARCHAR(128) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_role VARCHAR(128) NOT NULL DEFAULT '',
  recommended_skills JSONB NOT NULL DEFAULT '[]',
  recommended_mcp_servers JSONB NOT NULL DEFAULT '[]',
  recommended_provider_types JSONB NOT NULL DEFAULT '[]',
  default_capability_selection JSONB NOT NULL DEFAULT '{}',
  default_context_policy_override JSONB NOT NULL DEFAULT '{}',
  default_approval_policy JSONB NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  is_system BOOLEAN NOT NULL DEFAULT false,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT digital_employee_templates_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX digital_employee_templates_tenant_type_key
  ON digital_employee_templates (tenant_id, type) WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_templates_tenant_status_idx
  ON digital_employee_templates (tenant_id, status) WHERE deleted_at IS NULL;

INSERT INTO digital_employee_templates (
  tenant_id, type, label, description, default_role,
  recommended_skills, recommended_mcp_servers, recommended_provider_types,
  default_capability_selection, default_context_policy_override, default_approval_policy,
  metadata, status, is_system
) VALUES
(
  '00000000-0000-0000-0000-000000000001', 'database_admin', '数据库管理',
  '负责数据库运行维护、性能诊断、备份恢复、变更执行和数据安全检查。', 'database_admin',
  '["database-troubleshooting","sql-review","backup-restore","performance-tuning"]',
  '["postgres-readonly","mysql-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["database-troubleshooting","sql-review"],"enabled_mcp_servers":["postgres-readonly"],"enabled_provider_types":["codex"]}',
  '{"sources":["runbook","monitoring","database_schema"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'devops_engineer', 'DevOps 运维',
  '负责运行环境、发布流水线、故障处置、基础设施变更和可观测性排查。', 'devops_engineer',
  '["incident-diagnosis","release-operations","runtime-troubleshooting","observability-analysis"]',
  '["kubernetes-readonly","prometheus-readonly","grafana-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["incident-diagnosis","runtime-troubleshooting"],"enabled_mcp_servers":["prometheus-readonly"],"enabled_provider_types":["codex"]}',
  '{"sources":["runbook","monitoring","deployment_logs"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'security_engineer', '安全工程',
  '负责安全评审、漏洞分析、权限与配置风险检查、应急处置和修复建议。', 'security_engineer',
  '["security-review","vulnerability-analysis","permission-audit","incident-response"]',
  '["postgres-readonly","http-connector"]',
  '["codex","opencode"]',
  '{"enabled_skills":["security-review","vulnerability-analysis"],"enabled_provider_types":["codex"]}',
  '{"sources":["security_policy","audit_logs","repository"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'qa_engineer', '测试工程',
  '负责测试计划、用例设计、自动化验证、缺陷复现、回归检查和验收证据整理。', 'qa_engineer',
  '["test-planning","test-automation","bug-reproduction","regression-verification"]',
  '["browser"]',
  '["codex","opencode"]',
  '{"enabled_skills":["test-planning","regression-verification"],"enabled_provider_types":["codex"]}',
  '{"sources":["requirements","test_reports","browser_logs"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'frontend_engineer', '前端开发',
  '负责 Web 控制台界面开发、交互实现、前端状态管理和页面问题诊断。', 'frontend_engineer',
  '["frontend-implementation","ui-regression-check","accessibility-check","playwright-verification"]',
  '["browser"]',
  '["codex","opencode"]',
  '{"enabled_skills":["frontend-implementation","ui-regression-check"],"enabled_provider_types":["codex"]}',
  '{"sources":["design","frontend_code","browser_logs"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'backend_engineer', '后端开发',
  '负责控制平面后端服务、API 契约、业务逻辑、数据访问和服务端测试。', 'backend_engineer',
  '["backend-implementation","api-contract-check","database-query-review","go-test-verification"]',
  '["postgres-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["backend-implementation","api-contract-check"],"enabled_provider_types":["codex"]}',
  '{"sources":["api_contracts","backend_code","database_design"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'fullstack_engineer', '全栈开发',
  '负责跨前端、后端和契约的端到端功能实现、联调和回归验证。', 'fullstack_engineer',
  '["frontend-implementation","backend-implementation","api-contract-check","end-to-end-verification"]',
  '["browser","postgres-readonly"]',
  '["codex","opencode"]',
  '{"enabled_skills":["frontend-implementation","backend-implementation"],"enabled_provider_types":["codex"]}',
  '{"sources":["design","api_contracts","backend_code","frontend_code"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'implementation_engineer', '实施工程师',
  '负责客户侧部署配置、环境核对、能力接入、交付验证和问题闭环。', 'implementation_engineer',
  '["environment-check","connector-configuration","delivery-verification","customer-runbook-update"]',
  '["http-connector"]',
  '["codex","opencode"]',
  '{"enabled_skills":["environment-check","delivery-verification"],"enabled_provider_types":["codex"]}',
  '{"sources":["customer_profile","runbook","deployment_notes"]}',
  '{"min_risk_for_human":"high","write_actions_require_human":true}',
  '{}', 'active', true
),
(
  '00000000-0000-0000-0000-000000000001', 'general_engineer', '通用工程执行',
  '负责边界清晰、低风险的通用工程任务、资料整理、代码检查和验证执行。', 'general_engineer',
  '["code-reading","test-execution","artifact-preparation","technical-summary"]',
  '[]',
  '["codex","opencode"]',
  '{"enabled_skills":["code-reading","test-execution"],"enabled_provider_types":["codex"]}',
  '{"sources":["task_context","repository"]}',
  '{"min_risk_for_human":"medium"}',
  '{}', 'active', true
)
ON CONFLICT DO NOTHING;
```

- [ ] **Step 2: Update atlas.sum and validate**

Run:
```bash
cd apps/control-plane
atlas migrate hash --dir file://internal/storage/migrations
make migrate-validate
```
Expected: both commands exit 0; `git diff internal/storage/migrations/atlas.sum` shows a new entry for `050_digital_employee_templates.sql`.

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/050_digital_employee_templates.sql apps/control-plane/internal/storage/migrations/atlas.sum
git commit -m "feat(control-plane): add digital_employee_templates table with builtin seed data"
```

---

### Task 2: sqlc queries for `digital_employee_templates`

**Files:**
- Create: `apps/control-plane/internal/storage/queries/digital_employee_templates.sql`
- Generated (do not hand-edit): `apps/control-plane/internal/storage/queries/digital_employee_templates.sql.go`

**Interfaces:**
- Produces (after `sqlc generate`): `queries.DigitalEmployeeTemplate` struct (fields: `ID, TenantID, Type, Label, Description, DefaultRole, RecommendedSkills, RecommendedMcpServers, RecommendedProviderTypes, DefaultCapabilitySelection, DefaultContextPolicyOverride, DefaultApprovalPolicy, Metadata, Status, IsSystem, DeletedAt, CreatedAt, UpdatedAt` — the three JSONB list/map fields and `Metadata` are `[]byte`, `DeletedAt/CreatedAt/UpdatedAt` are `pgtype.Timestamptz`), and methods `ListEmployeeTemplates`, `GetEmployeeTemplateByID`, `GetEmployeeTemplateByType`, `CreateEmployeeTemplate`, `UpdateEmployeeTemplate`, `SetEmployeeTemplateStatus`, `SoftDeleteEmployeeTemplate`, `ListEmployeeTemplateLabels` on `*queries.Queries`.

- [ ] **Step 1: Write the query file**

```sql
-- apps/control-plane/internal/storage/queries/digital_employee_templates.sql

-- name: ListEmployeeTemplates :many
SELECT * FROM digital_employee_templates
WHERE tenant_id = $1 AND deleted_at IS NULL
ORDER BY created_at ASC;

-- name: GetEmployeeTemplateByID :one
SELECT * FROM digital_employee_templates
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: GetEmployeeTemplateByType :one
SELECT * FROM digital_employee_templates
WHERE tenant_id = $1 AND type = $2 AND deleted_at IS NULL;

-- name: CreateEmployeeTemplate :one
INSERT INTO digital_employee_templates (
  tenant_id, type, label, description, default_role,
  recommended_skills, recommended_mcp_servers, recommended_provider_types,
  default_capability_selection, default_context_policy_override, default_approval_policy,
  metadata, status, is_system
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'active', false
)
RETURNING *;

-- name: UpdateEmployeeTemplate :one
UPDATE digital_employee_templates SET
  label = $3,
  description = $4,
  default_role = $5,
  recommended_skills = $6,
  recommended_mcp_servers = $7,
  recommended_provider_types = $8,
  default_capability_selection = $9,
  default_context_policy_override = $10,
  default_approval_policy = $11,
  metadata = $12,
  updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SetEmployeeTemplateStatus :one
UPDATE digital_employee_templates SET
  status = $3,
  updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteEmployeeTemplate :execrows
UPDATE digital_employee_templates SET
  deleted_at = now(),
  updated_at = now()
WHERE tenant_id = $1 AND id = $2 AND deleted_at IS NULL;

-- name: ListEmployeeTemplateLabels :many
SELECT type, label FROM digital_employee_templates
WHERE tenant_id = $1;
```

- [ ] **Step 2: Generate and verify compile**

Run:
```bash
cd apps/control-plane
sqlc generate
go vet ./...
```
Expected: `internal/storage/queries/digital_employee_templates.sql.go` is created; `go vet` succeeds.

- [ ] **Step 3: Commit**

```bash
git add apps/control-plane/internal/storage/queries/digital_employee_templates.sql apps/control-plane/internal/storage/queries/digital_employee_templates.sql.go
git commit -m "feat(control-plane): add sqlc queries for digital_employee_templates"
```

---

### Task 3: Domain types and Repository interface extension

**Files:**
- Create: `apps/control-plane/internal/employee/template_types.go`
- Modify: `apps/control-plane/internal/employee/repository.go`

**Interfaces:**
- Consumes: `queries.DigitalEmployeeTemplate` (Task 2), `EmployeeTypeDefinition` (existing, `types.go:307-319`), `cloneStringSlice`/`cloneEmployeeTypeMap` (existing, `employee_types.go:241-259`).
- Produces: `EmployeeTemplateRecord` struct with method `ToDefinition() EmployeeTypeDefinition`; `ListEmployeeTemplatesParams`, `CreateEmployeeTemplateParams`, `UpdateEmployeeTemplateParams`; 8 new `Repository` interface methods (`ListEmployeeTemplates`, `GetEmployeeTemplate`, `GetEmployeeTemplateByType`, `CreateEmployeeTemplate`, `UpdateEmployeeTemplate`, `SetEmployeeTemplateStatus`, `SoftDeleteEmployeeTemplate`, `ListEmployeeTemplateLabels`).

- [ ] **Step 1: Write `template_types.go`**

```go
package employee

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeTemplateRecord struct {
	ID                           uuid.UUID
	TenantID                     uuid.UUID
	Type                         string
	Label                        string
	Description                  string
	DefaultRole                  string
	RecommendedSkills            []string
	RecommendedMCPServers        []string
	RecommendedProviderTypes     []string
	DefaultCapabilitySelection   map[string]any
	DefaultContextPolicyOverride map[string]any
	DefaultApprovalPolicy        map[string]any
	Metadata                     map[string]any
	Status                       string
	IsSystem                     bool
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

// ToDefinition projects the persisted template into the EmployeeTypeDefinition
// shape consumed by the create-employee wizard and creation-time defaults.
func (r EmployeeTemplateRecord) ToDefinition() EmployeeTypeDefinition {
	return EmployeeTypeDefinition{
		Type:                         r.Type,
		Label:                        r.Label,
		Description:                  r.Description,
		DefaultRole:                  r.DefaultRole,
		RecommendedSkills:            cloneStringSlice(r.RecommendedSkills),
		RecommendedMCPServers:        cloneStringSlice(r.RecommendedMCPServers),
		RecommendedProviderTypes:     cloneStringSlice(r.RecommendedProviderTypes),
		DefaultCapabilitySelection:   cloneEmployeeTypeMap(r.DefaultCapabilitySelection),
		DefaultContextPolicyOverride: cloneEmployeeTypeMap(r.DefaultContextPolicyOverride),
		DefaultApprovalPolicy:        cloneEmployeeTypeMap(r.DefaultApprovalPolicy),
		Metadata:                     cloneEmployeeTypeMap(r.Metadata),
	}
}

type ListEmployeeTemplatesParams struct {
	TenantID   uuid.UUID
	ActiveOnly bool
}

type CreateEmployeeTemplateParams struct {
	TenantID                     uuid.UUID
	Type                         string
	Label                        string
	Description                  string
	DefaultRole                  string
	RecommendedSkills            []string
	RecommendedMCPServers        []string
	RecommendedProviderTypes     []string
	DefaultCapabilitySelection   map[string]any
	DefaultContextPolicyOverride map[string]any
	DefaultApprovalPolicy        map[string]any
	Metadata                     map[string]any
}

type UpdateEmployeeTemplateParams struct {
	TenantID                     uuid.UUID
	ID                           uuid.UUID
	Label                        string
	Description                  string
	DefaultRole                  string
	RecommendedSkills            []string
	RecommendedMCPServers        []string
	RecommendedProviderTypes     []string
	DefaultCapabilitySelection   map[string]any
	DefaultContextPolicyOverride map[string]any
	DefaultApprovalPolicy        map[string]any
	Metadata                     map[string]any
}
```

- [ ] **Step 2: Extend the `Repository` interface**

In `repository.go`, add to the `Repository` interface (after line 46 `ListRunsDetailed(...)`, before the closing `}`):

```go
	ListEmployeeTemplates(ctx context.Context, params ListEmployeeTemplatesParams) ([]EmployeeTemplateRecord, error)
	GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error)
	GetEmployeeTemplateByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTemplateRecord, error)
	CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error)
	SoftDeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error
	ListEmployeeTemplateLabels(ctx context.Context, tenantID uuid.UUID) (map[string]string, error)
```

- [ ] **Step 3: Verify compile (expected to fail — interface not yet implemented)**

Run: `cd apps/control-plane && go vet ./...`
Expected: FAIL — `*PgRepository does not implement Repository` (missing methods). This confirms the interface changed; Task 4 implements it.

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/employee/template_types.go apps/control-plane/internal/employee/repository.go
git commit -m "feat(control-plane): add employee template domain types and repository interface"
```

---

### Task 4: `PgRepository` implementation

**Files:**
- Create: `apps/control-plane/internal/employee/template_repository.go`

**Interfaces:**
- Consumes: `queries.Queries` methods from Task 2; `jsonbFromMap`/`mapFromJSONB` (`pg_repository.go:1989-2005`); `timeFromTimestamptz`/`timePtrFromTimestamptz` (`pg_repository.go:1974-1987`); `mapNoRows` (`pg_repository.go:1909-1914`); `ErrInvalidInput`/`ErrNotFound` (`types.go:11-13`).
- Produces: the 8 `Repository` methods added in Task 3, implemented on `*PgRepository`.

- [ ] **Step 1: Write `template_repository.go`**

```go
package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/superteam/control-plane/internal/storage/queries"
)

func (r *PgRepository) ListEmployeeTemplates(ctx context.Context, params ListEmployeeTemplatesParams) ([]EmployeeTemplateRecord, error) {
	rows, err := r.q.ListEmployeeTemplates(ctx, params.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list employee templates: %w", err)
	}
	records := make([]EmployeeTemplateRecord, 0, len(rows))
	for _, row := range rows {
		record, err := employeeTemplateRecordFromRow(row)
		if err != nil {
			return nil, err
		}
		if params.ActiveOnly && record.Status != "active" {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error) {
	row, err := r.q.GetEmployeeTemplateByID(ctx, queries.GetEmployeeTemplateByIDParams{
		TenantID: tenantID,
		ID:       templateID,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) GetEmployeeTemplateByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTemplateRecord, error) {
	row, err := r.q.GetEmployeeTemplateByType(ctx, queries.GetEmployeeTemplateByTypeParams{
		TenantID: tenantID,
		Type:     employeeType,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	recommendedSkills, err := jsonbFromStringSlice(params.RecommendedSkills)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedMCPServers, err := jsonbFromStringSlice(params.RecommendedMCPServers)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedProviderTypes, err := jsonbFromStringSlice(params.RecommendedProviderTypes)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultCapabilitySelection, err := jsonbFromMap(params.DefaultCapabilitySelection, "default_capability_selection")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultContextPolicyOverride, err := jsonbFromMap(params.DefaultContextPolicyOverride, "default_context_policy_override")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultApprovalPolicy, err := jsonbFromMap(params.DefaultApprovalPolicy, "default_approval_policy")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}

	row, err := r.q.CreateEmployeeTemplate(ctx, queries.CreateEmployeeTemplateParams{
		TenantID:                     params.TenantID,
		Type:                         params.Type,
		Label:                        params.Label,
		Description:                  params.Description,
		DefaultRole:                  params.DefaultRole,
		RecommendedSkills:            recommendedSkills,
		RecommendedMcpServers:        recommendedMCPServers,
		RecommendedProviderTypes:     recommendedProviderTypes,
		DefaultCapabilitySelection:   defaultCapabilitySelection,
		DefaultContextPolicyOverride: defaultContextPolicyOverride,
		DefaultApprovalPolicy:        defaultApprovalPolicy,
		Metadata:                     metadata,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapTemplateConstraintError(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	recommendedSkills, err := jsonbFromStringSlice(params.RecommendedSkills)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedMCPServers, err := jsonbFromStringSlice(params.RecommendedMCPServers)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedProviderTypes, err := jsonbFromStringSlice(params.RecommendedProviderTypes)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultCapabilitySelection, err := jsonbFromMap(params.DefaultCapabilitySelection, "default_capability_selection")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultContextPolicyOverride, err := jsonbFromMap(params.DefaultContextPolicyOverride, "default_context_policy_override")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultApprovalPolicy, err := jsonbFromMap(params.DefaultApprovalPolicy, "default_approval_policy")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}

	row, err := r.q.UpdateEmployeeTemplate(ctx, queries.UpdateEmployeeTemplateParams{
		TenantID:                     params.TenantID,
		ID:                           params.ID,
		Label:                        params.Label,
		Description:                  params.Description,
		DefaultRole:                  params.DefaultRole,
		RecommendedSkills:            recommendedSkills,
		RecommendedMcpServers:        recommendedMCPServers,
		RecommendedProviderTypes:     recommendedProviderTypes,
		DefaultCapabilitySelection:   defaultCapabilitySelection,
		DefaultContextPolicyOverride: defaultContextPolicyOverride,
		DefaultApprovalPolicy:        defaultApprovalPolicy,
		Metadata:                     metadata,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error) {
	row, err := r.q.SetEmployeeTemplateStatus(ctx, queries.SetEmployeeTemplateStatusParams{
		TenantID: tenantID,
		ID:       templateID,
		Status:   status,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) SoftDeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	affected, err := r.q.SoftDeleteEmployeeTemplate(ctx, queries.SoftDeleteEmployeeTemplateParams{
		TenantID: tenantID,
		ID:       templateID,
	})
	if err != nil {
		return fmt.Errorf("soft delete employee template: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PgRepository) ListEmployeeTemplateLabels(ctx context.Context, tenantID uuid.UUID) (map[string]string, error) {
	rows, err := r.q.ListEmployeeTemplateLabels(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list employee template labels: %w", err)
	}
	labels := make(map[string]string, len(rows))
	for _, row := range rows {
		labels[row.Type] = row.Label
	}
	return labels, nil
}

func employeeTemplateRecordFromRow(row queries.DigitalEmployeeTemplate) (EmployeeTemplateRecord, error) {
	recommendedSkills, err := stringSliceFromJSONB(row.RecommendedSkills, "recommended_skills")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedMCPServers, err := stringSliceFromJSONB(row.RecommendedMcpServers, "recommended_mcp_servers")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedProviderTypes, err := stringSliceFromJSONB(row.RecommendedProviderTypes, "recommended_provider_types")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultCapabilitySelection, err := mapFromJSONB(row.DefaultCapabilitySelection, "default_capability_selection")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultContextPolicyOverride, err := mapFromJSONB(row.DefaultContextPolicyOverride, "default_context_policy_override")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultApprovalPolicy, err := mapFromJSONB(row.DefaultApprovalPolicy, "default_approval_policy")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	metadata, err := mapFromJSONB(row.Metadata, "metadata")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	return EmployeeTemplateRecord{
		ID:                           row.ID,
		TenantID:                     row.TenantID,
		Type:                         row.Type,
		Label:                        row.Label,
		Description:                  row.Description,
		DefaultRole:                  row.DefaultRole,
		RecommendedSkills:            recommendedSkills,
		RecommendedMCPServers:        recommendedMCPServers,
		RecommendedProviderTypes:     recommendedProviderTypes,
		DefaultCapabilitySelection:   defaultCapabilitySelection,
		DefaultContextPolicyOverride: defaultContextPolicyOverride,
		DefaultApprovalPolicy:        defaultApprovalPolicy,
		Metadata:                     metadata,
		Status:                       row.Status,
		IsSystem:                     row.IsSystem,
		CreatedAt:                    timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:                    timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

func jsonbFromStringSlice(values []string) ([]byte, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode string slice: %w", err)
	}
	return encoded, nil
}

func stringSliceFromJSONB(raw []byte, field string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if values == nil {
		values = []string{}
	}
	return values, nil
}

func mapTemplateConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: template type already exists for this tenant", ErrInvalidInput)
	}
	return err
}
```

- [ ] **Step 2: Verify compile**

Run: `cd apps/control-plane && go vet ./...`
Expected: PASS (interface now fully implemented).

- [ ] **Step 3: Integration test against a real Postgres**

Add to `apps/control-plane/internal/employee/pg_repository_test.go` (append at end of file, following the existing `TestGetTeamBaseline` pattern at lines 24-116):

```go
func TestEmployeeTemplateRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	cfg, ok := employeeRepoIntegrationTestConfig()
	if !ok {
		t.Skip("set TEST_DATABASE_URL, or set ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 with DATABASE_URL")
	}

	conn, err := pgx.Connect(ctx, cfg.databaseURL)
	require.NoError(t, err)
	defer conn.Close(ctx)

	schemaName := "employee_templates_" + strings.ReplaceAll(strings.ToLower(uuid.NewString()), "-", "_")
	_, err = conn.Exec(ctx, `CREATE SCHEMA `+schemaName)
	require.NoError(t, err)
	defer conn.Exec(ctx, `DROP SCHEMA IF EXISTS `+schemaName+` CASCADE`)

	_, err = conn.Exec(ctx, `SET search_path TO `+schemaName)
	require.NoError(t, err)
	require.NoError(t, runEmployeeRepoTestMigrations(ctx, conn))

	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	repo := NewPgRepository(queries.New(conn), conn)

	// The migration seeds 9 builtin templates for the default tenant.
	seeded, err := repo.ListEmployeeTemplates(ctx, ListEmployeeTemplatesParams{TenantID: tenantID})
	require.NoError(t, err)
	require.Len(t, seeded, 9)

	created, err := repo.CreateEmployeeTemplate(ctx, CreateEmployeeTemplateParams{
		TenantID:                  tenantID,
		Type:                      "custom_reviewer",
		Label:                     "自定义评审员",
		Description:               "自定义模板",
		DefaultRole:               "custom_reviewer",
		RecommendedSkills:         []string{"code-review"},
		RecommendedMCPServers:     []string{},
		RecommendedProviderTypes:  []string{"codex"},
		DefaultCapabilitySelection: map[string]any{"enabled_skills": []string{"code-review"}},
	})
	require.NoError(t, err)
	require.Equal(t, "custom_reviewer", created.Type)
	require.False(t, created.IsSystem)
	require.Equal(t, "active", created.Status)

	_, err = repo.CreateEmployeeTemplate(ctx, CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "重复类型",
	})
	require.ErrorIs(t, err, ErrInvalidInput)

	updated, err := repo.UpdateEmployeeTemplate(ctx, UpdateEmployeeTemplateParams{
		TenantID:    tenantID,
		ID:          created.ID,
		Label:       "评审员（已更新）",
		Description: "更新后的描述",
	})
	require.NoError(t, err)
	require.Equal(t, "评审员（已更新）", updated.Label)

	disabled, err := repo.SetEmployeeTemplateStatus(ctx, tenantID, created.ID, "disabled")
	require.NoError(t, err)
	require.Equal(t, "disabled", disabled.Status)

	active, err := repo.ListEmployeeTemplates(ctx, ListEmployeeTemplatesParams{TenantID: tenantID, ActiveOnly: true})
	require.NoError(t, err)
	for _, tmpl := range active {
		require.NotEqual(t, created.ID, tmpl.ID)
	}

	require.NoError(t, repo.SoftDeleteEmployeeTemplate(ctx, tenantID, created.ID))
	_, err = repo.GetEmployeeTemplate(ctx, tenantID, created.ID)
	require.ErrorIs(t, err, ErrNotFound)

	err = repo.SoftDeleteEmployeeTemplate(ctx, tenantID, created.ID)
	require.ErrorIs(t, err, ErrNotFound)

	labels, err := repo.ListEmployeeTemplateLabels(ctx, tenantID)
	require.NoError(t, err)
	require.Equal(t, "数据库管理", labels["database_admin"])
}
```

- [ ] **Step 4: Run the integration test**

Run:
```bash
cd apps/control-plane
ALLOW_DATABASE_URL_FOR_QUERY_TESTS=1 DATABASE_URL="$DATABASE_URL" go test ./internal/employee/ -run TestEmployeeTemplateRepositoryCRUD -v
```
Expected: PASS. If no local Postgres is reachable, this step is blocked and must be run once one is available (e.g. via `scripts/dev-services.sh status` to find the dev DB URL) before Task 4 can be marked verified — do not skip silently.

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/employee/template_repository.go apps/control-plane/internal/employee/pg_repository_test.go
git commit -m "feat(control-plane): implement PgRepository CRUD for employee templates"
```

---

### Task 4a: Add template fixtures and methods to the in-memory test double

**Files:**
- Modify: `apps/control-plane/internal/employee/service_test.go`

**Interfaces:**
- Consumes: `EmployeeTemplateRecord`, `ListEmployeeTemplatesParams`, `CreateEmployeeTemplateParams`, `UpdateEmployeeTemplateParams` (Task 3).
- Produces: `memoryRepository.templates map[uuid.UUID][]EmployeeTemplateRecord` (keyed by tenant, auto-seeded on first read with 9 fixture rows matching the migration's builtin set); implementations of all 8 new `Repository` methods on `*memoryRepository`, used by every existing test that calls `newMemoryRepository()`.

- [ ] **Step 1: Add the templates field and seeding to `memoryRepository`**

In `service_test.go`, modify the `memoryRepository` struct (around line 2308) to add one field:

```go
type memoryRepository struct {
	teams                     map[uuid.UUID]uuid.UUID
	employees                 map[uuid.UUID]DigitalEmployeeRecord
	instances                 map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord
	preflight                 RuntimeProvisioningPreflight
	preflightErr              error
	commandReceipts           map[string]*RuntimeCommandReceipt
	waitStatus                string
	waitErr                   error
	abortReasons              []string
	abortContextErrors        []error
	createdEmployeeCount      int
	teamConfigs               map[uuid.UUID]TeamConfigInput
	teamBaselines             map[uuid.UUID]TeamBaseline
	currentTeamConfigByTeam   map[uuid.UUID]uuid.UUID
	runtimeProviderOptions    []RuntimeProviderOption
	employeeConfigs           map[uuid.UUID]EmployeeConfigInput
	schedulingCapabilityFacts SchedulingCapabilityFacts
	envVars                   map[string]EnvironmentVariableRecord
	workspaceFiles            []WorkspaceFileRecord
	workspaceFileRevisions    []WorkspaceFileRevisionRecord
	nextConfigRevisionNumber  int32
	createdConfigRevision     CreateConfigRevisionParams
	digitalEmployeeOverview   *DigitalEmployeeOverview
	lastOverviewRequest       GetDigitalEmployeeOverviewRequest
	waitHook                  func(context.Context, uuid.UUID, string, time.Duration) (*RuntimeCommandReceipt, error)
	transactionCount          int
	transactionCommitCount    int
	transactionRollbackCount  int
	inTransaction             bool
	templates                 map[uuid.UUID][]EmployeeTemplateRecord
}
```

And in `newMemoryRepository()` (around line 2340), add the field init:

```go
func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		teams:                    make(map[uuid.UUID]uuid.UUID),
		employees:                make(map[uuid.UUID]DigitalEmployeeRecord),
		instances:                make(map[uuid.UUID]DigitalEmployeeExecutionInstanceRecord),
		commandReceipts:          make(map[string]*RuntimeCommandReceipt),
		teamConfigs:              make(map[uuid.UUID]TeamConfigInput),
		teamBaselines:            make(map[uuid.UUID]TeamBaseline),
		currentTeamConfigByTeam:  make(map[uuid.UUID]uuid.UUID),
		employeeConfigs:          make(map[uuid.UUID]EmployeeConfigInput),
		envVars:                  make(map[string]EnvironmentVariableRecord),
		nextConfigRevisionNumber: 1,
		templates:                make(map[uuid.UUID][]EmployeeTemplateRecord),
	}
}
```

- [ ] **Step 2: Add fixture data and Repository methods**

Append to `service_test.go` (new section, after the `memoryRepository`/`newMemoryRepository` block):

```go
func builtinEmployeeTemplateFixtures(tenantID uuid.UUID) []EmployeeTemplateRecord {
	now := time.Now().UTC()
	type seed struct {
		templateType string
		label        string
		defaultRole  string
		skills       []string
	}
	seeds := []seed{
		{"database_admin", "数据库管理", "database_admin", []string{"database-troubleshooting"}},
		{"devops_engineer", "DevOps 运维", "devops_engineer", []string{"incident-diagnosis"}},
		{"security_engineer", "安全工程", "security_engineer", []string{"security-review"}},
		{"qa_engineer", "测试工程", "qa_engineer", []string{"test-planning"}},
		{"frontend_engineer", "前端开发", "frontend_engineer", []string{"frontend-implementation"}},
		{"backend_engineer", "后端开发", "backend_engineer", []string{"backend-implementation"}},
		{"fullstack_engineer", "全栈开发", "fullstack_engineer", []string{"frontend-implementation", "backend-implementation"}},
		{"implementation_engineer", "实施工程师", "implementation_engineer", []string{"environment-check"}},
		{"general_engineer", "通用工程执行", "general_engineer", []string{"code-reading"}},
	}
	records := make([]EmployeeTemplateRecord, 0, len(seeds))
	for _, s := range seeds {
		records = append(records, EmployeeTemplateRecord{
			ID:                         uuid.New(),
			TenantID:                   tenantID,
			Type:                       s.templateType,
			Label:                      s.label,
			Description:                s.label,
			DefaultRole:                s.defaultRole,
			RecommendedSkills:          s.skills,
			RecommendedMCPServers:      []string{},
			RecommendedProviderTypes:   []string{"codex", "opencode"},
			DefaultCapabilitySelection: map[string]any{"enabled_skills": s.skills},
			Status:                     "active",
			IsSystem:                   true,
			CreatedAt:                  now,
			UpdatedAt:                  now,
		})
	}
	return records
}

func (r *memoryRepository) templatesForTenant(tenantID uuid.UUID) []EmployeeTemplateRecord {
	if _, ok := r.templates[tenantID]; !ok {
		r.templates[tenantID] = builtinEmployeeTemplateFixtures(tenantID)
	}
	return r.templates[tenantID]
}

func (r *memoryRepository) ListEmployeeTemplates(ctx context.Context, params ListEmployeeTemplatesParams) ([]EmployeeTemplateRecord, error) {
	result := make([]EmployeeTemplateRecord, 0)
	for _, tmpl := range r.templatesForTenant(params.TenantID) {
		if params.ActiveOnly && tmpl.Status != "active" {
			continue
		}
		result = append(result, tmpl)
	}
	return result, nil
}

func (r *memoryRepository) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error) {
	for _, tmpl := range r.templatesForTenant(tenantID) {
		if tmpl.ID == templateID {
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) GetEmployeeTemplateByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTemplateRecord, error) {
	for _, tmpl := range r.templatesForTenant(tenantID) {
		if tmpl.Type == employeeType {
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	for _, tmpl := range r.templatesForTenant(params.TenantID) {
		if tmpl.Type == params.Type {
			return EmployeeTemplateRecord{}, fmt.Errorf("%w: template type already exists for this tenant", ErrInvalidInput)
		}
	}
	now := time.Now().UTC()
	record := EmployeeTemplateRecord{
		ID:                           uuid.New(),
		TenantID:                     params.TenantID,
		Type:                         params.Type,
		Label:                        params.Label,
		Description:                  params.Description,
		DefaultRole:                  params.DefaultRole,
		RecommendedSkills:            params.RecommendedSkills,
		RecommendedMCPServers:        params.RecommendedMCPServers,
		RecommendedProviderTypes:     params.RecommendedProviderTypes,
		DefaultCapabilitySelection:   params.DefaultCapabilitySelection,
		DefaultContextPolicyOverride: params.DefaultContextPolicyOverride,
		DefaultApprovalPolicy:        params.DefaultApprovalPolicy,
		Metadata:                     params.Metadata,
		Status:                       "active",
		IsSystem:                    false,
		CreatedAt:                    now,
		UpdatedAt:                    now,
	}
	r.templates[params.TenantID] = append(r.templatesForTenant(params.TenantID), record)
	return record, nil
}

func (r *memoryRepository) UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	templates := r.templatesForTenant(params.TenantID)
	for i, tmpl := range templates {
		if tmpl.ID == params.ID {
			tmpl.Label = params.Label
			tmpl.Description = params.Description
			tmpl.DefaultRole = params.DefaultRole
			tmpl.RecommendedSkills = params.RecommendedSkills
			tmpl.RecommendedMCPServers = params.RecommendedMCPServers
			tmpl.RecommendedProviderTypes = params.RecommendedProviderTypes
			tmpl.DefaultCapabilitySelection = params.DefaultCapabilitySelection
			tmpl.DefaultContextPolicyOverride = params.DefaultContextPolicyOverride
			tmpl.DefaultApprovalPolicy = params.DefaultApprovalPolicy
			tmpl.Metadata = params.Metadata
			tmpl.UpdatedAt = time.Now().UTC()
			templates[i] = tmpl
			r.templates[params.TenantID] = templates
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error) {
	templates := r.templatesForTenant(tenantID)
	for i, tmpl := range templates {
		if tmpl.ID == templateID {
			tmpl.Status = status
			tmpl.UpdatedAt = time.Now().UTC()
			templates[i] = tmpl
			r.templates[tenantID] = templates
			return tmpl, nil
		}
	}
	return EmployeeTemplateRecord{}, ErrNotFound
}

func (r *memoryRepository) SoftDeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	templates := r.templatesForTenant(tenantID)
	for i, tmpl := range templates {
		if tmpl.ID == templateID {
			r.templates[tenantID] = append(templates[:i], templates[i+1:]...)
			return nil
		}
	}
	return ErrNotFound
}

func (r *memoryRepository) ListEmployeeTemplateLabels(ctx context.Context, tenantID uuid.UUID) (map[string]string, error) {
	labels := make(map[string]string)
	for _, tmpl := range r.templatesForTenant(tenantID) {
		labels[tmpl.Type] = tmpl.Label
	}
	return labels, nil
}
```

- [ ] **Step 3: Verify compile**

Run: `cd apps/control-plane && go vet ./internal/employee/...`
Expected: FAIL — this will still fail until Task 6/7 remove the now-dangling `DefaultEmployeeTypeDefinitions()`/`EmployeeTypeDefinitionByType` calls elsewhere. That is expected at this point; if it fails for any *other* reason, stop and investigate before continuing.

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/employee/service_test.go
git commit -m "test(control-plane): add employee template fixtures to memoryRepository test double"
```

---

### Task 5: Retire the hardcoded catalog, keep the `custom_agent` sentinel

**Files:**
- Modify: `apps/control-plane/internal/employee/employee_types.go`

**Interfaces:**
- Produces: `customAgentEmployeeTypeDefinition() EmployeeTypeDefinition` (replaces `DefaultEmployeeTypeDefinitions()`/`EmployeeTypeDefinitionByType()`, which are deleted).

- [ ] **Step 1: Replace the file contents**

Rewrite `employee_types.go` in full:

```go
package employee

// customAgentEmployeeTypeDefinition is the sentinel "blank custom" digital
// employee type. It is never persisted to digital_employee_templates —
// unlike every other type, it has no default role, skills, or policies for
// a user to configure; it exists purely so the create-employee wizard can
// offer a fully custom starting point.
func customAgentEmployeeTypeDefinition() EmployeeTypeDefinition {
	return EmployeeTypeDefinition{
		Type:                         "custom_agent",
		Label:                        "自定义数字员工",
		Description:                  "由用户直接定义职责定位、能力扩展、治理策略和执行器类型的自定义数字员工。",
		DefaultRole:                  "",
		RecommendedSkills:            []string{},
		RecommendedMCPServers:        []string{},
		RecommendedProviderTypes:     []string{"codex", "opencode", "claude-code"},
		DefaultCapabilitySelection:   map[string]any{},
		DefaultContextPolicyOverride: map[string]any{},
		DefaultApprovalPolicy:        map[string]any{},
		Metadata: map[string]any{
			"creation_mode": "blank_custom",
			"system_type":   true,
		},
	}
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneEmployeeTypeMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneEmployeeTypeValue(value)
	}
	return cloned
}

func cloneEmployeeTypeValue(value any) any {
	switch typed := value.(type) {
	case []string:
		return cloneStringSlice(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneEmployeeTypeValue(item)
		}
		return cloned
	case map[string]any:
		return cloneEmployeeTypeMap(typed)
	default:
		return typed
	}
}
```

- [ ] **Step 2: Commit**

```bash
git add apps/control-plane/internal/employee/employee_types.go
git commit -m "refactor(control-plane): retire hardcoded employee type catalog, keep custom_agent sentinel"
```

(Expect the build to still fail after this step — `service.go` and `pg_repository.go` still reference the removed functions. Task 6 fixes that.)

---

### Task 6: Wire `service.go` to the template repository

**Files:**
- Modify: `apps/control-plane/internal/employee/service.go`

**Interfaces:**
- Consumes: `s.repository.ListEmployeeTemplates`, `s.repository.GetEmployeeTemplateByType` (Task 3/4), `customAgentEmployeeTypeDefinition()` (Task 5).
- Produces: `(s *Service) employeeTypeDefinitionByType(ctx, tenantID, employeeType) (EmployeeTypeDefinition, error)`; `(s *Service) normalizeCreateDigitalEmployeeRequest(ctx, req)` (now a method, was a free function); `capabilityOptionsForCreate(employeeTypes []EmployeeTypeDefinition)`, `platformSkillOptions(employeeTypes []EmployeeTypeDefinition)`, `platformMCPServerOptions(employeeTypes []EmployeeTypeDefinition)` (now take the already-fetched list instead of calling the deleted `DefaultEmployeeTypeDefinitions()`).

- [ ] **Step 1: Update `GetCreateOptions` (around `service.go:206-253`)**

Replace:
```go
	employeeTypes := DefaultEmployeeTypeDefinitions()
	var runtimeOptions []RuntimeProviderOption
	if teamLess {
		runtimeOptions, err = s.repository.ListRuntimeProviderOptionsForTeamLessCreate(ctx, req.TenantID)
	} else {
		runtimeOptions, err = s.repository.ListRuntimeProviderOptionsForCreate(ctx, req.TenantID, *req.TeamID)
	}
	if err != nil {
		return nil, fmt.Errorf("list runtime provider options: %w", err)
	}
	capabilityOptions := capabilityOptionsForCreate()
```
with:
```go
	templates, err := s.repository.ListEmployeeTemplates(ctx, ListEmployeeTemplatesParams{TenantID: req.TenantID, ActiveOnly: true})
	if err != nil {
		return nil, fmt.Errorf("list employee templates: %w", err)
	}
	employeeTypes := make([]EmployeeTypeDefinition, 0, len(templates)+1)
	employeeTypes = append(employeeTypes, customAgentEmployeeTypeDefinition())
	for _, template := range templates {
		employeeTypes = append(employeeTypes, template.ToDefinition())
	}
	var runtimeOptions []RuntimeProviderOption
	if teamLess {
		runtimeOptions, err = s.repository.ListRuntimeProviderOptionsForTeamLessCreate(ctx, req.TenantID)
	} else {
		runtimeOptions, err = s.repository.ListRuntimeProviderOptionsForCreate(ctx, req.TenantID, *req.TeamID)
	}
	if err != nil {
		return nil, fmt.Errorf("list runtime provider options: %w", err)
	}
	capabilityOptions := capabilityOptionsForCreate(employeeTypes)
```

- [ ] **Step 2: Update `capabilityOptionsForCreate`/`platformSkillOptions`/`platformMCPServerOptions` (around `service.go:356-361` and `562-598`)**

Replace:
```go
func capabilityOptionsForCreate() CapabilityOptions {
	return CapabilityOptions{
		ProviderTypes: supportedProviderTypes(),
		Skills:        platformSkillOptions(),
		MCPServers:    platformMCPServerOptions(),
	}
}
```
with:
```go
func capabilityOptionsForCreate(employeeTypes []EmployeeTypeDefinition) CapabilityOptions {
	return CapabilityOptions{
		ProviderTypes: supportedProviderTypes(),
		Skills:        platformSkillOptions(employeeTypes),
		MCPServers:    platformMCPServerOptions(employeeTypes),
	}
}
```

Replace:
```go
func platformSkillOptions() []string {
	values := make(map[string]struct{})
	for _, definition := range DefaultEmployeeTypeDefinitions() {
		for _, skill := range definition.RecommendedSkills {
			if skill == "" {
				continue
			}
			values[skill] = struct{}{}
		}
		for _, skill := range stringList(definition.DefaultCapabilitySelection["enabled_skills"]) {
			if skill == "" {
				continue
			}
			values[skill] = struct{}{}
		}
	}
	return sortedKeys(values)
}

func platformMCPServerOptions() []string {
	values := make(map[string]struct{})
	for _, definition := range DefaultEmployeeTypeDefinitions() {
		for _, serverID := range definition.RecommendedMCPServers {
			if serverID == "" {
				continue
			}
			values[serverID] = struct{}{}
		}
		for _, serverID := range stringList(definition.DefaultCapabilitySelection["enabled_mcp_servers"]) {
			if serverID == "" {
				continue
			}
			values[serverID] = struct{}{}
		}
	}
	return sortedKeys(values)
}
```
with:
```go
func platformSkillOptions(employeeTypes []EmployeeTypeDefinition) []string {
	values := make(map[string]struct{})
	for _, definition := range employeeTypes {
		for _, skill := range definition.RecommendedSkills {
			if skill == "" {
				continue
			}
			values[skill] = struct{}{}
		}
		for _, skill := range stringList(definition.DefaultCapabilitySelection["enabled_skills"]) {
			if skill == "" {
				continue
			}
			values[skill] = struct{}{}
		}
	}
	return sortedKeys(values)
}

func platformMCPServerOptions(employeeTypes []EmployeeTypeDefinition) []string {
	values := make(map[string]struct{})
	for _, definition := range employeeTypes {
		for _, serverID := range definition.RecommendedMCPServers {
			if serverID == "" {
				continue
			}
			values[serverID] = struct{}{}
		}
		for _, serverID := range stringList(definition.DefaultCapabilitySelection["enabled_mcp_servers"]) {
			if serverID == "" {
				continue
			}
			values[serverID] = struct{}{}
		}
	}
	return sortedKeys(values)
}
```

- [ ] **Step 3: Convert `normalizeCreateDigitalEmployeeRequest` into a method and add the lookup helper**

Replace the function signature and body at `service.go:469-531`:
```go
func normalizeCreateDigitalEmployeeRequest(req CreateDigitalEmployeeRequest) (CreateDigitalEmployeeRequest, EmployeeTypeDefinition, error) {
	if req.TenantID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID != nil && *req.TeamID == uuid.Nil {
		req.TeamID = nil
	}
	if req.OwnerUserID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: owner_user_id is required", ErrInvalidInput)
	}
	employeeType := strings.ToLower(strings.TrimSpace(req.EmployeeType))
	if employeeType == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: employee_type is required", ErrInvalidInput)
	}
	definition, ok := EmployeeTypeDefinitionByType(employeeType)
	if !ok {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: unknown employee_type %q", ErrInvalidInput, employeeType)
	}
```
with:
```go
func (s *Service) normalizeCreateDigitalEmployeeRequest(ctx context.Context, req CreateDigitalEmployeeRequest) (CreateDigitalEmployeeRequest, EmployeeTypeDefinition, error) {
	if req.TenantID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID != nil && *req.TeamID == uuid.Nil {
		req.TeamID = nil
	}
	if req.OwnerUserID == uuid.Nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: owner_user_id is required", ErrInvalidInput)
	}
	employeeType := strings.ToLower(strings.TrimSpace(req.EmployeeType))
	if employeeType == "" {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, fmt.Errorf("%w: employee_type is required", ErrInvalidInput)
	}
	definition, err := s.employeeTypeDefinitionByType(ctx, req.TenantID, employeeType)
	if err != nil {
		return CreateDigitalEmployeeRequest{}, EmployeeTypeDefinition{}, err
	}
```
(the remainder of the function body, from `name := strings.TrimSpace(req.Name)` through `return req, definition, nil`, is unchanged).

Add a new method directly below it:
```go
// employeeTypeDefinitionByType resolves an employee_type string to its
// EmployeeTypeDefinition. custom_agent is a hardcoded sentinel (never
// persisted); every other type is looked up in digital_employee_templates,
// scoped to the tenant, and must be active to be usable for creation.
func (s *Service) employeeTypeDefinitionByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTypeDefinition, error) {
	if employeeType == "custom_agent" {
		return customAgentEmployeeTypeDefinition(), nil
	}
	template, err := s.repository.GetEmployeeTemplateByType(ctx, tenantID, employeeType)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return EmployeeTypeDefinition{}, fmt.Errorf("%w: unknown employee_type %q", ErrInvalidInput, employeeType)
		}
		return EmployeeTypeDefinition{}, err
	}
	if template.Status != "active" {
		return EmployeeTypeDefinition{}, fmt.Errorf("%w: employee_type %q is disabled", ErrInvalidInput, employeeType)
	}
	return template.ToDefinition(), nil
}
```

Check the top of `service.go` for an existing `"errors"` import; add it to the import block if missing.

- [ ] **Step 4: Update the call site in `CreateDigitalEmployee` (`service.go:412-413`)**

Replace:
```go
func (s *Service) CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error) {
	normalized, definition, err := normalizeCreateDigitalEmployeeRequest(req)
```
with:
```go
func (s *Service) CreateDigitalEmployee(ctx context.Context, req CreateDigitalEmployeeRequest) (*DigitalEmployee, error) {
	normalized, definition, err := s.normalizeCreateDigitalEmployeeRequest(ctx, req)
```

- [ ] **Step 5: Verify compile**

Run: `cd apps/control-plane && go vet ./...`
Expected: FAIL only on `pg_repository.go` (`overviewEmployeeTypeLabel` etc. still reference the deleted functions) and on `*_test.go` files still calling `DefaultEmployeeTypeDefinitions()`/`EmployeeTypeDefinitionByType()` — Tasks 7 and 8 fix these. If the failure list includes anything in `service.go` itself, stop and fix before proceeding.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/service.go
git commit -m "refactor(control-plane): resolve employee type definitions from the template repository"
```

---

### Task 7: Wire `pg_repository.go` overview label lookups to the template repository

**Files:**
- Modify: `apps/control-plane/internal/employee/pg_repository.go`

**Interfaces:**
- Consumes: `r.ListEmployeeTemplateLabels(ctx, tenantID)` (Task 4), `customAgentEmployeeTypeDefinition()` (Task 5).
- Produces: `overviewItemFromQuery(row, labelsByType map[string]string)`, `overviewFiltersFromQuery(rows, labelsByType map[string]string)`, `overviewFilterLabel(filterType, value, fallback string, labelsByType map[string]string)`, `overviewEmployeeTypeLabel(value string, labelsByType map[string]string)` — all gain a `labelsByType` parameter.

- [ ] **Step 1: Build the labels map once per overview request (`pg_repository.go:948-968`)**

Replace:
```go
	itemRows, err := r.q.ListDigitalEmployeeOverviewItems(ctx, queries.ListDigitalEmployeeOverviewItemsParams{
		TenantID:        req.TenantID,
		Q:               summaryParams.Q,
		TeamID:          summaryParams.TeamID,
		Status:          summaryParams.Status,
		EmployeeType:    summaryParams.EmployeeType,
		ProviderType:    summaryParams.ProviderType,
		RuntimeNodeID:   summaryParams.RuntimeNodeID,
		RiskLevel:       summaryParams.RiskLevel,
		ExecutionStatus: summaryParams.ExecutionStatus,
		RunStatus:       summaryParams.RunStatus,
		Limit:           req.Limit,
		Offset:          req.Offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]DigitalEmployeeOverviewItem, 0, len(itemRows))
	for _, row := range itemRows {
		items = append(items, overviewItemFromQuery(row))
	}
```
with:
```go
	itemRows, err := r.q.ListDigitalEmployeeOverviewItems(ctx, queries.ListDigitalEmployeeOverviewItemsParams{
		TenantID:        req.TenantID,
		Q:               summaryParams.Q,
		TeamID:          summaryParams.TeamID,
		Status:          summaryParams.Status,
		EmployeeType:    summaryParams.EmployeeType,
		ProviderType:    summaryParams.ProviderType,
		RuntimeNodeID:   summaryParams.RuntimeNodeID,
		RiskLevel:       summaryParams.RiskLevel,
		ExecutionStatus: summaryParams.ExecutionStatus,
		RunStatus:       summaryParams.RunStatus,
		Limit:           req.Limit,
		Offset:          req.Offset,
	})
	if err != nil {
		return nil, err
	}
	labelsByType, err := r.ListEmployeeTemplateLabels(ctx, req.TenantID)
	if err != nil {
		return nil, err
	}
	items := make([]DigitalEmployeeOverviewItem, 0, len(itemRows))
	for _, row := range itemRows {
		items = append(items, overviewItemFromQuery(row, labelsByType))
	}
```

- [ ] **Step 2: Thread `labelsByType` into the filters call (`pg_repository.go:995`)**

Replace:
```go
		Filters: overviewFiltersFromQuery(filterRows),
```
with:
```go
		Filters: overviewFiltersFromQuery(filterRows, labelsByType),
```

- [ ] **Step 3: Update function signatures (`pg_repository.go:1065`, `1127`, `1243`, `1263`, `1287-1315`)**

Change:
```go
func overviewItemFromQuery(row queries.ListDigitalEmployeeOverviewItemsRow) DigitalEmployeeOverviewItem {
```
to:
```go
func overviewItemFromQuery(row queries.ListDigitalEmployeeOverviewItemsRow, labelsByType map[string]string) DigitalEmployeeOverviewItem {
```

Change:
```go
			EmployeeTypeLabel: overviewEmployeeTypeLabel(row.EmployeeType),
```
to:
```go
			EmployeeTypeLabel: overviewEmployeeTypeLabel(row.EmployeeType, labelsByType),
```

Change:
```go
func overviewFiltersFromQuery(rows []queries.ListDigitalEmployeeOverviewFilterOptionsRow) DigitalEmployeeOverviewFilters {
```
to:
```go
func overviewFiltersFromQuery(rows []queries.ListDigitalEmployeeOverviewFilterOptionsRow, labelsByType map[string]string) DigitalEmployeeOverviewFilters {
```

Change:
```go
		label = overviewFilterLabel(row.FilterType, value, label)
```
to:
```go
		label = overviewFilterLabel(row.FilterType, value, label, labelsByType)
```

Change:
```go
func overviewFilterLabel(filterType, value, fallback string) string {
	switch filterType {
	case "employee_type":
		return overviewEmployeeTypeLabel(value)
```
to:
```go
func overviewFilterLabel(filterType, value, fallback string, labelsByType map[string]string) string {
	switch filterType {
	case "employee_type":
		return overviewEmployeeTypeLabel(value, labelsByType)
```

Change:
```go
func overviewEmployeeTypeLabel(value string) string {
	definition, ok := EmployeeTypeDefinitionByType(value)
	if !ok {
		return value
	}
	return definition.Label
}
```
to:
```go
func overviewEmployeeTypeLabel(value string, labelsByType map[string]string) string {
	if value == "custom_agent" {
		return customAgentEmployeeTypeDefinition().Label
	}
	if label, ok := labelsByType[value]; ok && label != "" {
		return label
	}
	return value
}
```

- [ ] **Step 4: Verify compile**

Run: `cd apps/control-plane && go vet ./...`
Expected: FAIL only in `*_test.go` files (Task 8 fixes these).

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/employee/pg_repository.go
git commit -m "refactor(control-plane): resolve overview employee-type labels from the template repository"
```

---

### Task 8: Fix existing tests broken by the catalog refactor

**Files:**
- Modify: `apps/control-plane/internal/employee/service_test.go`
- Modify: `apps/control-plane/internal/employee/pg_repository_test.go`

**Interfaces:**
- Consumes: `builtinEmployeeTemplateFixtures` (Task 4a), `customAgentEmployeeTypeDefinition()` (Task 5).

- [ ] **Step 1: Fix the two catalog-property tests in `service_test.go` (lines 20-72)**

Replace:
```go
func TestEmployeeTypeRegistryExcludesProjectCoordinator(t *testing.T) {
	types := DefaultEmployeeTypeDefinitions()
	if len(types) < 6 {
		t.Fatalf("expected professional engineer types, got %#v", types)
	}
	for _, item := range types {
		if strings.Contains(item.Type, "coordinator") || strings.Contains(item.Label, "协调") {
			t.Fatalf("project coordinator must not be a reusable employee type: %#v", item)
		}
	}
	if _, ok := EmployeeTypeDefinitionByType("database_admin"); !ok {
		t.Fatalf("expected database_admin type")
	}
	if _, ok := EmployeeTypeDefinitionByType("devops_engineer"); !ok {
		t.Fatalf("expected devops_engineer type")
	}
	if _, ok := EmployeeTypeDefinitionByType("security_engineer"); !ok {
		t.Fatalf("expected security_engineer type")
	}
	if _, ok := EmployeeTypeDefinitionByType("qa_engineer"); !ok {
		t.Fatalf("expected qa_engineer type")
	}
}

func TestEmployeeTypeRegistryReturnsClonedDefinitions(t *testing.T) {
	types := DefaultEmployeeTypeDefinitions()
	if len(types) == 0 {
		t.Fatalf("expected employee type definitions")
	}
	typeIndex := firstTypeWithDefaultSkills(t, types)
	originalSkill := types[typeIndex].RecommendedSkills[0]
	types[typeIndex].RecommendedSkills[0] = "mutated-skill"
	enabledSkills, ok := types[typeIndex].DefaultCapabilitySelection["enabled_skills"].([]string)
	if !ok || len(enabledSkills) == 0 {
		t.Fatalf("expected enabled_skills default selection, got %#v", types[typeIndex].DefaultCapabilitySelection)
	}
	enabledSkills[0] = "mutated-enabled-skill"

	fresh := DefaultEmployeeTypeDefinitions()
	if fresh[typeIndex].RecommendedSkills[0] != originalSkill {
		t.Fatalf("expected recommended skills to be cloned, got %#v", fresh[typeIndex].RecommendedSkills)
	}
	freshEnabledSkills, ok := fresh[typeIndex].DefaultCapabilitySelection["enabled_skills"].([]string)
	if !ok || len(freshEnabledSkills) == 0 {
		t.Fatalf("expected fresh enabled_skills default selection, got %#v", fresh[typeIndex].DefaultCapabilitySelection)
	}
	if freshEnabledSkills[0] == "mutated-enabled-skill" {
		t.Fatalf("expected default capability selection to be cloned, got %#v", fresh[typeIndex].DefaultCapabilitySelection)
	}
}

func firstTypeWithDefaultSkills(t *testing.T, types []EmployeeTypeDefinition) int {
	t.Helper()
	for index, definition := range types {
		if len(definition.RecommendedSkills) == 0 {
			continue
		}
		enabledSkills, ok := definition.DefaultCapabilitySelection["enabled_skills"].([]string)
		if ok && len(enabledSkills) > 0 {
			return index
		}
	}
	t.Fatalf("expected at least one employee type with default skills, got %#v", types)
	return 0
}

func TestCustomAgentEmployeeTypeDefinitionIsAvailableForBlankCustomCreate(t *testing.T) {
	definition, ok := EmployeeTypeDefinitionByType(" custom_agent ")
	require.True(t, ok)
	require.Equal(t, "custom_agent", definition.Type)
	require.Equal(t, "自定义数字员工", definition.Label)
	require.Empty(t, definition.DefaultRole)
	require.Empty(t, definition.RecommendedSkills)
	require.Empty(t, definition.RecommendedMCPServers)
	require.Empty(t, definition.DefaultCapabilitySelection)
	require.Contains(t, definition.Metadata, "creation_mode")
	require.Equal(t, "blank_custom", definition.Metadata["creation_mode"])
}
```
with:
```go
func TestBuiltinEmployeeTemplateFixturesExcludeProjectCoordinator(t *testing.T) {
	types := builtinEmployeeTemplateFixtures(uuid.New())
	if len(types) < 6 {
		t.Fatalf("expected professional engineer types, got %#v", types)
	}
	for _, item := range types {
		if strings.Contains(item.Type, "coordinator") || strings.Contains(item.Label, "协调") {
			t.Fatalf("project coordinator must not be a reusable employee type: %#v", item)
		}
	}
	wantTypes := []string{"database_admin", "devops_engineer", "security_engineer", "qa_engineer"}
	for _, wantType := range wantTypes {
		found := false
		for _, item := range types {
			if item.Type == wantType {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %s type in fixtures, got %#v", wantType, types)
		}
	}
}

func TestCustomAgentEmployeeTypeDefinitionIsAvailableForBlankCustomCreate(t *testing.T) {
	definition := customAgentEmployeeTypeDefinition()
	require.Equal(t, "custom_agent", definition.Type)
	require.Equal(t, "自定义数字员工", definition.Label)
	require.Empty(t, definition.DefaultRole)
	require.Empty(t, definition.RecommendedSkills)
	require.Empty(t, definition.RecommendedMCPServers)
	require.Empty(t, definition.DefaultCapabilitySelection)
	require.Contains(t, definition.Metadata, "creation_mode")
	require.Equal(t, "blank_custom", definition.Metadata["creation_mode"])
}
```

(`TestEmployeeTypeRegistryReturnsClonedDefinitions` is dropped — clone-safety of the fixture list is no longer a production-code concern since `builtinEmployeeTemplateFixtures` is test-only fixture data, not a runtime catalog; `ToDefinition()`'s clone behavior is exercised indirectly by every `GetCreateOptions` test below.)

- [ ] **Step 2: Fix the remaining `DefaultEmployeeTypeDefinitions()` count assertions (lines 145, 275)**

In `TestGetCreateOptionsReturnsTeamBaselineAndPlatformCandidates`, replace:
```go
	if len(options.EmployeeTypes) != len(DefaultEmployeeTypeDefinitions()) {
		t.Fatalf("expected full employee types, got %#v", options.EmployeeTypes)
	}
```
with:
```go
	if len(options.EmployeeTypes) != len(builtinEmployeeTemplateFixtures(tenantID))+1 {
		t.Fatalf("expected full employee types, got %#v", options.EmployeeTypes)
	}
```

In `TestCreateOptionsUsePlatformFullEmployeeTypes`, replace:
```go
	require.Len(t, options.EmployeeTypes, len(DefaultEmployeeTypeDefinitions()))
```
with:
```go
	require.Len(t, options.EmployeeTypes, len(builtinEmployeeTemplateFixtures(tenantID))+1)
```

- [ ] **Step 3: Fix the 15 `overviewItemFromQuery(row)` call sites in `pg_repository_test.go`**

Run:
```bash
cd apps/control-plane
sed -i '' 's/overviewItemFromQuery(row)/overviewItemFromQuery(row, map[string]string{})/g' internal/employee/pg_repository_test.go
```
(On Linux/CI use `sed -i` without the `''` argument.)

- [ ] **Step 4: Fix `TestOverviewFiltersFromQueryMapsStableLabels` (line 285-303)**

Replace:
```go
	filters := overviewFiltersFromQuery([]queries.ListDigitalEmployeeOverviewFilterOptionsRow{
```
with:
```go
	filters := overviewFiltersFromQuery([]queries.ListDigitalEmployeeOverviewFilterOptionsRow{
```
(unchanged opening line) and change the closing of that call from:
```go
		{FilterType: "provider", Value: "custom-provider", Label: "custom-provider"},
	})
```
to:
```go
		{FilterType: "provider", Value: "custom-provider", Label: "custom-provider"},
	}, map[string]string{})
```

- [ ] **Step 5: Verify the full package builds and tests pass**

Run:
```bash
cd apps/control-plane
go vet ./...
go test ./internal/employee/... -v 2>&1 | tail -100
```
Expected: `go vet` passes; `go test` passes (integration tests requiring `TEST_DATABASE_URL` will `SKIP`, that's fine — all non-skipped tests must pass).

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/service_test.go apps/control-plane/internal/employee/pg_repository_test.go
git commit -m "test(control-plane): fix tests broken by the employee template catalog refactor"
```

---

### Task 9: `template_service.go` — validation and CRUD orchestration

**Files:**
- Create: `apps/control-plane/internal/employee/template_service.go`
- Test: `apps/control-plane/internal/employee/template_service_test.go`

**Interfaces:**
- Consumes: `Repository` methods from Task 3/4a, `ErrInvalidInput`/`ErrNotFound` (`types.go:11-13`).
- Produces: `(s *Service) ListEmployeeTemplates(ctx, tenantID uuid.UUID) ([]EmployeeTemplateRecord, error)`, `(s *Service) GetEmployeeTemplate(ctx, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error)`, `(s *Service) CreateEmployeeTemplate(ctx, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error)`, `(s *Service) UpdateEmployeeTemplate(ctx, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error)`, `(s *Service) SetEmployeeTemplateStatus(ctx, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error)`, `(s *Service) DeleteEmployeeTemplate(ctx, tenantID, templateID uuid.UUID) error`.

- [ ] **Step 1: Write the failing tests**

```go
// apps/control-plane/internal/employee/template_service_test.go
package employee

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateEmployeeTemplateValidatesTypeFormat(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "Invalid Type!",
		Label:    "无效类型",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateEmployeeTemplateRequiresLabel(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "  ",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateEmployeeTemplateRejectsDuplicateTypeForTenant(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "database_admin",
		Label:    "重复的数据库管理",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateEmployeeTemplateSucceeds(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	created, err := svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:                 tenantID,
		Type:                     "custom_reviewer",
		Label:                    "自定义评审员",
		RecommendedSkills:        []string{"code-review"},
		RecommendedMCPServers:    []string{},
		RecommendedProviderTypes: []string{"codex"},
	})

	require.NoError(t, err)
	require.Equal(t, "custom_reviewer", created.Type)
	require.Equal(t, "active", created.Status)
	require.False(t, created.IsSystem)
}

func TestCreateEmployeeTemplateRejectsNonMapPolicyFields(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:                   tenantID,
		Type:                       "custom_reviewer",
		Label:                      "自定义评审员",
		DefaultCapabilitySelection: nil,
	})
	require.NoError(t, err, "nil policy maps are allowed and normalized to {}")
}

func TestUpdateEmployeeTemplateRejectsTypeChangeAttemptSilently(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	created, err := svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "自定义评审员",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateEmployeeTemplate(context.Background(), UpdateEmployeeTemplateParams{
		TenantID: tenantID,
		ID:       created.ID,
		Label:    "评审员 v2",
	})

	require.NoError(t, err)
	require.Equal(t, "custom_reviewer", updated.Type, "type must stay immutable regardless of what UpdateEmployeeTemplateParams carries")
	require.Equal(t, "评审员 v2", updated.Label)
}

func TestSetEmployeeTemplateStatusRejectsUnknownStatus(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	created, err := svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "自定义评审员",
	})
	require.NoError(t, err)

	_, err = svc.SetEmployeeTemplateStatus(context.Background(), tenantID, created.ID, "archived")

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestDeleteEmployeeTemplateAllowsDeletingSystemTemplates(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	templates, err := svc.ListEmployeeTemplates(context.Background(), tenantID)
	require.NoError(t, err)
	require.NotEmpty(t, templates)
	target := templates[0]
	require.True(t, target.IsSystem)

	err = svc.DeleteEmployeeTemplate(context.Background(), tenantID, target.ID)

	require.NoError(t, err)
	_, err = svc.GetEmployeeTemplate(context.Background(), tenantID, target.ID)
	require.ErrorIs(t, err, ErrNotFound)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd apps/control-plane && go test ./internal/employee/... -run TestCreateEmployeeTemplate -v`
Expected: FAIL with `svc.CreateEmployeeTemplate undefined` (method doesn't exist yet).

- [ ] **Step 3: Write `template_service.go`**

```go
package employee

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var employeeTemplateTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

func (s *Service) ListEmployeeTemplates(ctx context.Context, tenantID uuid.UUID) ([]EmployeeTemplateRecord, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	return s.repository.ListEmployeeTemplates(ctx, ListEmployeeTemplatesParams{TenantID: tenantID})
}

func (s *Service) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error) {
	if tenantID == uuid.Nil || templateID == uuid.Nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: tenant_id and template_id are required", ErrInvalidInput)
	}
	return s.repository.GetEmployeeTemplate(ctx, tenantID, templateID)
}

func (s *Service) CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	if params.TenantID == uuid.Nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	normalizedType := strings.ToLower(strings.TrimSpace(params.Type))
	if !employeeTemplateTypePattern.MatchString(normalizedType) {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: type must match %s", ErrInvalidInput, employeeTemplateTypePattern.String())
	}
	if normalizedType == "custom_agent" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: custom_agent is a reserved type", ErrInvalidInput)
	}
	label := strings.TrimSpace(params.Label)
	if label == "" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: label is required", ErrInvalidInput)
	}
	if _, err := s.repository.GetEmployeeTemplateByType(ctx, params.TenantID, normalizedType); err == nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: template type %q already exists for this tenant", ErrInvalidInput, normalizedType)
	} else if !errorIsNotFound(err) {
		return EmployeeTemplateRecord{}, err
	}

	params.Type = normalizedType
	params.Label = label
	params.RecommendedSkills = nonNilStringSlice(params.RecommendedSkills)
	params.RecommendedMCPServers = nonNilStringSlice(params.RecommendedMCPServers)
	params.RecommendedProviderTypes = nonNilStringSlice(params.RecommendedProviderTypes)
	params.DefaultCapabilitySelection = nonNilMap(params.DefaultCapabilitySelection)
	params.DefaultContextPolicyOverride = nonNilMap(params.DefaultContextPolicyOverride)
	params.DefaultApprovalPolicy = nonNilMap(params.DefaultApprovalPolicy)
	params.Metadata = nonNilMap(params.Metadata)

	return s.repository.CreateEmployeeTemplate(ctx, params)
}

func (s *Service) UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	if params.TenantID == uuid.Nil || params.ID == uuid.Nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: tenant_id and id are required", ErrInvalidInput)
	}
	label := strings.TrimSpace(params.Label)
	if label == "" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: label is required", ErrInvalidInput)
	}
	params.Label = label
	params.RecommendedSkills = nonNilStringSlice(params.RecommendedSkills)
	params.RecommendedMCPServers = nonNilStringSlice(params.RecommendedMCPServers)
	params.RecommendedProviderTypes = nonNilStringSlice(params.RecommendedProviderTypes)
	params.DefaultCapabilitySelection = nonNilMap(params.DefaultCapabilitySelection)
	params.DefaultContextPolicyOverride = nonNilMap(params.DefaultContextPolicyOverride)
	params.DefaultApprovalPolicy = nonNilMap(params.DefaultApprovalPolicy)
	params.Metadata = nonNilMap(params.Metadata)

	return s.repository.UpdateEmployeeTemplate(ctx, params)
}

func (s *Service) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error) {
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	if normalizedStatus != "active" && normalizedStatus != "disabled" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
	}
	return s.repository.SetEmployeeTemplateStatus(ctx, tenantID, templateID, normalizedStatus)
}

func (s *Service) DeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	if tenantID == uuid.Nil || templateID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id and template_id are required", ErrInvalidInput)
	}
	return s.repository.SoftDeleteEmployeeTemplate(ctx, tenantID, templateID)
}

func errorIsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func nonNilStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}
```

Add `"errors"` to the import block (`context`, `errors`, `fmt`, `regexp`, `strings`, `github.com/google/uuid`).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `cd apps/control-plane && go test ./internal/employee/... -run TestCreateEmployeeTemplate -v && go test ./internal/employee/... -run TestUpdateEmployeeTemplate -v && go test ./internal/employee/... -run TestSetEmployeeTemplateStatus -v && go test ./internal/employee/... -run TestDeleteEmployeeTemplate -v`
Expected: PASS.

- [ ] **Step 5: Run the full package test suite**

Run: `cd apps/control-plane && go vet ./... && go test ./internal/employee/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/template_service.go apps/control-plane/internal/employee/template_service_test.go
git commit -m "feat(control-plane): add employee template CRUD service with validation"
```

---

### Task 10: `template_handler.go` — HTTP handlers and routes

**Files:**
- Create: `apps/control-plane/internal/employee/template_handler.go`
- Test: append to `apps/control-plane/internal/employee/handler_test.go` (if it exists) or create `apps/control-plane/internal/employee/template_handler_test.go`
- Modify: `apps/control-plane/internal/employee/handler.go` (extend `HandlerService` interface)
- Modify: `apps/control-plane/internal/api/server.go`

**Interfaces:**
- Consumes: `Service.ListEmployeeTemplates/GetEmployeeTemplate/CreateEmployeeTemplate/UpdateEmployeeTemplate/SetEmployeeTemplateStatus/DeleteEmployeeTemplate` (Task 9), `h.authorizeDigitalEmployeeManagement` (`handler.go:672`), `writeJSON`/`writeHandlerError` (`handler.go:1078-1104`).
- Produces: `HTTPHandler.ListEmployeeTemplates/GetEmployeeTemplate/CreateEmployeeTemplate/UpdateEmployeeTemplate/SetEmployeeTemplateStatus/DeleteEmployeeTemplate` HTTP methods; 6 new routes under `/api/v1/digital-employee-templates`.

- [ ] **Step 1: Extend the `HandlerService` interface (`handler.go:18-34`)**

Add these 6 lines inside the `HandlerService` interface:
```go
	ListEmployeeTemplates(ctx context.Context, tenantID uuid.UUID) ([]EmployeeTemplateRecord, error)
	GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error)
	CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error)
	SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error)
	DeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error
```

- [ ] **Step 2: Write `template_handler.go`**

```go
package employee

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/authz"
)

type employeeTemplateResponse struct {
	ID                           string         `json:"id"`
	TenantID                     string         `json:"tenant_id"`
	Type                         string         `json:"type"`
	Label                        string         `json:"label"`
	Description                  string         `json:"description"`
	DefaultRole                  string         `json:"default_role"`
	RecommendedSkills            []string       `json:"recommended_skills"`
	RecommendedMCPServers        []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes     []string       `json:"recommended_provider_types"`
	DefaultCapabilitySelection   map[string]any `json:"default_capability_selection"`
	DefaultContextPolicyOverride map[string]any `json:"default_context_policy_override"`
	DefaultApprovalPolicy        map[string]any `json:"default_approval_policy"`
	Metadata                     map[string]any `json:"metadata"`
	Status                       string         `json:"status"`
	IsSystem                     bool           `json:"is_system"`
	CreatedAt                    string         `json:"created_at"`
	UpdatedAt                    string         `json:"updated_at"`
}

type createEmployeeTemplateRequest struct {
	Type                         string         `json:"type"`
	Label                        string         `json:"label"`
	Description                  string         `json:"description"`
	DefaultRole                  string         `json:"default_role"`
	RecommendedSkills            []string       `json:"recommended_skills"`
	RecommendedMCPServers        []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes     []string       `json:"recommended_provider_types"`
	DefaultCapabilitySelection   map[string]any `json:"default_capability_selection"`
	DefaultContextPolicyOverride map[string]any `json:"default_context_policy_override"`
	DefaultApprovalPolicy        map[string]any `json:"default_approval_policy"`
	Metadata                     map[string]any `json:"metadata"`
}

type updateEmployeeTemplateRequest struct {
	Label                        string         `json:"label"`
	Description                  string         `json:"description"`
	DefaultRole                  string         `json:"default_role"`
	RecommendedSkills            []string       `json:"recommended_skills"`
	RecommendedMCPServers        []string       `json:"recommended_mcp_servers"`
	RecommendedProviderTypes     []string       `json:"recommended_provider_types"`
	DefaultCapabilitySelection   map[string]any `json:"default_capability_selection"`
	DefaultContextPolicyOverride map[string]any `json:"default_context_policy_override"`
	DefaultApprovalPolicy        map[string]any `json:"default_approval_policy"`
	Metadata                     map[string]any `json:"metadata"`
}

type setEmployeeTemplateStatusRequest struct {
	Status string `json:"status"`
}

func (h *HTTPHandler) ListEmployeeTemplates(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template list")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templates, err := service.ListEmployeeTemplates(r.Context(), tenantID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponses(templates))
}

func (h *HTTPHandler) GetEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template read")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	template, err := service.GetEmployeeTemplate(r.Context(), tenantID, templateID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) CreateEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template create")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	var req createEmployeeTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	template, err := service.CreateEmployeeTemplate(r.Context(), CreateEmployeeTemplateParams{
		TenantID:                     tenantID,
		Type:                         req.Type,
		Label:                        req.Label,
		Description:                  req.Description,
		DefaultRole:                  req.DefaultRole,
		RecommendedSkills:            req.RecommendedSkills,
		RecommendedMCPServers:        req.RecommendedMCPServers,
		RecommendedProviderTypes:     req.RecommendedProviderTypes,
		DefaultCapabilitySelection:   req.DefaultCapabilitySelection,
		DefaultContextPolicyOverride: req.DefaultContextPolicyOverride,
		DefaultApprovalPolicy:        req.DefaultApprovalPolicy,
		Metadata:                     req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) UpdateEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	var req updateEmployeeTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	template, err := service.UpdateEmployeeTemplate(r.Context(), UpdateEmployeeTemplateParams{
		TenantID:                     tenantID,
		ID:                           templateID,
		Label:                        req.Label,
		Description:                  req.Description,
		DefaultRole:                  req.DefaultRole,
		RecommendedSkills:            req.RecommendedSkills,
		RecommendedMCPServers:        req.RecommendedMCPServers,
		RecommendedProviderTypes:     req.RecommendedProviderTypes,
		DefaultCapabilitySelection:   req.DefaultCapabilitySelection,
		DefaultContextPolicyOverride: req.DefaultContextPolicyOverride,
		DefaultApprovalPolicy:        req.DefaultApprovalPolicy,
		Metadata:                     req.Metadata,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) SetEmployeeTemplateStatus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template status update")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	var req setEmployeeTemplateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	template, err := service.SetEmployeeTemplateStatus(r.Context(), tenantID, templateID, req.Status)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, employeeTemplateResponseFromDomain(template))
}

func (h *HTTPHandler) DeleteEmployeeTemplate(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.authorizeDigitalEmployeeManagement(w, r, authz.ActionEmployeeCreate, nil, "employee template delete")
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	templateID, err := uuid.Parse(chi.URLParam(r, "templateId"))
	if err != nil {
		http.Error(w, "invalid templateId", http.StatusBadRequest)
		return
	}
	if err := service.DeleteEmployeeTemplate(r.Context(), tenantID, templateID); err != nil {
		writeHandlerError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func employeeTemplateResponses(templates []EmployeeTemplateRecord) []employeeTemplateResponse {
	responses := make([]employeeTemplateResponse, 0, len(templates))
	for _, template := range templates {
		responses = append(responses, employeeTemplateResponseFromDomain(template))
	}
	return responses
}

func employeeTemplateResponseFromDomain(t EmployeeTemplateRecord) employeeTemplateResponse {
	return employeeTemplateResponse{
		ID:                           t.ID.String(),
		TenantID:                     t.TenantID.String(),
		Type:                         t.Type,
		Label:                        t.Label,
		Description:                  t.Description,
		DefaultRole:                  t.DefaultRole,
		RecommendedSkills:            stringSliceForJSON(t.RecommendedSkills),
		RecommendedMCPServers:        stringSliceForJSON(t.RecommendedMCPServers),
		RecommendedProviderTypes:     stringSliceForJSON(t.RecommendedProviderTypes),
		DefaultCapabilitySelection:   cloneMap(t.DefaultCapabilitySelection),
		DefaultContextPolicyOverride: cloneMap(t.DefaultContextPolicyOverride),
		DefaultApprovalPolicy:        cloneMap(t.DefaultApprovalPolicy),
		Metadata:                     cloneMap(t.Metadata),
		Status:                       t.Status,
		IsSystem:                     t.IsSystem,
		CreatedAt:                    timeString(t.CreatedAt),
		UpdatedAt:                    timeString(t.UpdatedAt),
	}
}
```

`timeString(time.Time) string` already exists in `handler.go` (used by `employeeResponses` and others, e.g. `handler.go:1133-1134`) — reuse it as-is, no new helper needed.

- [ ] **Step 3: Register routes in `server.go`**

In `internal/api/server.go`, inside the `if s.employeeHandler != nil { ... }` block (after line 257, the `StopDigitalEmployeeRun` route), add:
```go
				r.Get("/digital-employee-templates", s.employeeHandler.ListEmployeeTemplates)
				r.Post("/digital-employee-templates", s.employeeHandler.CreateEmployeeTemplate)
				r.Get("/digital-employee-templates/{templateId}", s.employeeHandler.GetEmployeeTemplate)
				r.Patch("/digital-employee-templates/{templateId}", s.employeeHandler.UpdateEmployeeTemplate)
				r.Patch("/digital-employee-templates/{templateId}/status", s.employeeHandler.SetEmployeeTemplateStatus)
				r.Delete("/digital-employee-templates/{templateId}", s.employeeHandler.DeleteEmployeeTemplate)
```

- [ ] **Step 4: Verify compile**

Run: `cd apps/control-plane && go vet ./...`
Expected: PASS.

- [ ] **Step 5: Write and run handler tests**

Follow the existing handler test conventions in `apps/control-plane/internal/employee/handler_test.go` (check for a lightweight fake `HandlerService` + fake `authz.Authorizer` pattern used by other handler tests in that file, e.g. for `GetCreateOptions` or `CreateDigitalEmployee`) and add table-driven tests for:
- `ListEmployeeTemplates` returns 200 with the service's list, serialized.
- `CreateEmployeeTemplate` with a body missing `label` returns 400 (service returns `ErrInvalidInput`).
- `DeleteEmployeeTemplate` with an unknown `templateId` returns 404 (service returns `ErrNotFound`).
- Any handler returns 403 when `authorizeDigitalEmployeeManagement` denies (reuse whatever fake-authorizer test helper the file already has for `GetCreateOptions`'s 403 case).

Run: `cd apps/control-plane && go test ./internal/employee/... -run TestEmployeeTemplate -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/template_handler.go apps/control-plane/internal/employee/handler.go apps/control-plane/internal/employee/handler_test.go apps/control-plane/internal/api/server.go
git commit -m "feat(control-plane): add HTTP handlers and routes for employee template CRUD"
```

---

### Task 11: OpenAPI contract

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`

**Interfaces:**
- Produces: 6 new paths under `/api/v1/digital-employee-templates`, 4 new schemas (`EmployeeTemplate`, `CreateEmployeeTemplateRequest`, `UpdateEmployeeTemplateRequest`, `SetEmployeeTemplateStatusRequest`); regenerated `apps/control-plane/gen/control_plane.gen.go` and `apps/control-plane/internal/api/gen/control_plane.gen.go`.

- [ ] **Step 1: Add the paths**

In `openapi.yaml`, add a new top-level path block right before `/api/v1/templates:` (around line 3520), matching the existing indentation/style:

```yaml
  /api/v1/digital-employee-templates:
    get:
      operationId: listEmployeeTemplates
      summary: List digital employee templates for the current tenant
      responses:
        "200":
          description: Employee templates list
          content:
            application/json:
              schema:
                type: array
                items:
                  $ref: "#/components/schemas/EmployeeTemplate"
        "403":
          $ref: "#/components/responses/Error"
    post:
      operationId: createEmployeeTemplate
      summary: Create a digital employee template
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateEmployeeTemplateRequest"
      responses:
        "201":
          description: Employee template created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/EmployeeTemplate"
        "400":
          $ref: "#/components/responses/Error"
        "403":
          $ref: "#/components/responses/Error"
  /api/v1/digital-employee-templates/{templateId}:
    get:
      operationId: getEmployeeTemplate
      summary: Get a digital employee template
      parameters:
        - name: templateId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Employee template
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/EmployeeTemplate"
        "403":
          $ref: "#/components/responses/Error"
        "404":
          $ref: "#/components/responses/Error"
    patch:
      operationId: updateEmployeeTemplate
      summary: Update a digital employee template's configuration
      parameters:
        - name: templateId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/UpdateEmployeeTemplateRequest"
      responses:
        "200":
          description: Employee template updated
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/EmployeeTemplate"
        "400":
          $ref: "#/components/responses/Error"
        "403":
          $ref: "#/components/responses/Error"
        "404":
          $ref: "#/components/responses/Error"
    delete:
      operationId: deleteEmployeeTemplate
      summary: Soft-delete a digital employee template
      parameters:
        - name: templateId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "204":
          description: Employee template deleted
        "403":
          $ref: "#/components/responses/Error"
        "404":
          $ref: "#/components/responses/Error"
  /api/v1/digital-employee-templates/{templateId}/status:
    patch:
      operationId: setEmployeeTemplateStatus
      summary: Enable or disable a digital employee template
      parameters:
        - name: templateId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/SetEmployeeTemplateStatusRequest"
      responses:
        "200":
          description: Employee template status updated
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/EmployeeTemplate"
        "400":
          $ref: "#/components/responses/Error"
        "403":
          $ref: "#/components/responses/Error"
        "404":
          $ref: "#/components/responses/Error"
```

- [ ] **Step 2: Add the schemas**

In the `components.schemas` section, add these 4 schemas near `DigitalEmployeeTypeOption` (around line 9824):

```yaml
    EmployeeTemplate:
      type: object
      required:
        - id
        - tenant_id
        - type
        - label
        - status
        - is_system
        - created_at
        - updated_at
      properties:
        id:
          type: string
          format: uuid
        tenant_id:
          type: string
          format: uuid
        type:
          type: string
        label:
          type: string
        description:
          type: string
        default_role:
          type: string
        recommended_skills:
          type: array
          items:
            type: string
        recommended_mcp_servers:
          type: array
          items:
            type: string
        recommended_provider_types:
          type: array
          items:
            type: string
        default_capability_selection:
          type: object
          additionalProperties: true
        default_context_policy_override:
          type: object
          additionalProperties: true
        default_approval_policy:
          type: object
          additionalProperties: true
        metadata:
          type: object
          additionalProperties: true
        status:
          type: string
          enum: [active, disabled]
        is_system:
          type: boolean
        created_at:
          type: string
          format: date-time
        updated_at:
          type: string
          format: date-time
    CreateEmployeeTemplateRequest:
      type: object
      required:
        - type
        - label
      properties:
        type:
          type: string
        label:
          type: string
        description:
          type: string
        default_role:
          type: string
        recommended_skills:
          type: array
          items:
            type: string
        recommended_mcp_servers:
          type: array
          items:
            type: string
        recommended_provider_types:
          type: array
          items:
            type: string
        default_capability_selection:
          type: object
          additionalProperties: true
        default_context_policy_override:
          type: object
          additionalProperties: true
        default_approval_policy:
          type: object
          additionalProperties: true
        metadata:
          type: object
          additionalProperties: true
    UpdateEmployeeTemplateRequest:
      type: object
      required:
        - label
      properties:
        label:
          type: string
        description:
          type: string
        default_role:
          type: string
        recommended_skills:
          type: array
          items:
            type: string
        recommended_mcp_servers:
          type: array
          items:
            type: string
        recommended_provider_types:
          type: array
          items:
            type: string
        default_capability_selection:
          type: object
          additionalProperties: true
        default_context_policy_override:
          type: object
          additionalProperties: true
        default_approval_policy:
          type: object
          additionalProperties: true
        metadata:
          type: object
          additionalProperties: true
    SetEmployeeTemplateStatusRequest:
      type: object
      required:
        - status
      properties:
        status:
          type: string
          enum: [active, disabled]
```

- [ ] **Step 3: Regenerate and verify**

Run:
```bash
cd apps/control-plane
go generate ./...
go vet ./...
git diff --stat gen/control_plane.gen.go internal/api/gen/control_plane.gen.go
```
Expected: `go vet` passes; both generated files show a diff containing `EmployeeTemplate`, `ListEmployeeTemplates`, etc.

- [ ] **Step 4: Contract validation**

There is no dedicated `openapi.yaml` lint/validate command in this repo (confirmed: no `make`/`package.json` target references it outside the `apps/control-plane` codegen path). The available validation signal is `go generate ./... && go vet ./...` from Step 3 succeeding — that already exercises the spec through `oapi-codegen`, which fails loudly on malformed YAML or schema errors.

- [ ] **Step 5: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/gen/control_plane.gen.go apps/control-plane/internal/api/gen/control_plane.gen.go
git commit -m "feat(contracts): add digital employee template CRUD endpoints to openapi.yaml"
```

---

### Task 12: Frontend API client `employee-templates.ts`

**Files:**
- Create: `apps/web/src/lib/api/employee-templates.ts`

**Interfaces:**
- Consumes: `ApiClientOptions`, `buildApiUrl`, `parseJson`, `getJson`, `postJson`, `patchJson`, `deleteJson` (`apps/web/src/lib/api/client.ts`).
- Produces: `EmployeeTemplate` type, `CreateEmployeeTemplateInput`, `UpdateEmployeeTemplateInput`, `listEmployeeTemplates`, `getEmployeeTemplate`, `createEmployeeTemplate`, `updateEmployeeTemplate`, `setEmployeeTemplateStatus`, `deleteEmployeeTemplate`.

- [ ] **Step 1: Write the file**

```typescript
import type { ApiClientOptions } from "./client";
import { deleteJson, getJson, patchJson, postJson } from "./client";

export type EmployeeTemplateStatus = "active" | "disabled";

export type EmployeeTemplate = {
  id: string;
  tenant_id: string;
  type: string;
  label: string;
  description: string;
  default_role: string;
  recommended_skills: string[];
  recommended_mcp_servers: string[];
  recommended_provider_types: string[];
  default_capability_selection: Record<string, unknown>;
  default_context_policy_override: Record<string, unknown>;
  default_approval_policy: Record<string, unknown>;
  metadata: Record<string, unknown>;
  status: EmployeeTemplateStatus;
  is_system: boolean;
  created_at: string;
  updated_at: string;
};

export type CreateEmployeeTemplateInput = {
  type: string;
  label: string;
  description?: string;
  default_role?: string;
  recommended_skills?: string[];
  recommended_mcp_servers?: string[];
  recommended_provider_types?: string[];
  default_capability_selection?: Record<string, unknown>;
  default_context_policy_override?: Record<string, unknown>;
  default_approval_policy?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
};

export type UpdateEmployeeTemplateInput = Omit<CreateEmployeeTemplateInput, "type">;

export async function listEmployeeTemplates(
  options: ApiClientOptions,
): Promise<EmployeeTemplate[]> {
  return getJson<EmployeeTemplate[]>(options, "/api/v1/digital-employee-templates", "employee templates");
}

export async function getEmployeeTemplate(
  options: ApiClientOptions,
  templateId: string,
): Promise<EmployeeTemplate> {
  return getJson<EmployeeTemplate>(
    options,
    `/api/v1/digital-employee-templates/${templateId}`,
    "employee template",
  );
}

export async function createEmployeeTemplate(
  options: ApiClientOptions,
  input: CreateEmployeeTemplateInput,
): Promise<EmployeeTemplate> {
  return postJson<EmployeeTemplate>(
    options,
    "/api/v1/digital-employee-templates",
    input,
    "employee template",
  );
}

export async function updateEmployeeTemplate(
  options: ApiClientOptions,
  templateId: string,
  input: UpdateEmployeeTemplateInput,
): Promise<EmployeeTemplate> {
  return patchJson<EmployeeTemplate>(
    options,
    `/api/v1/digital-employee-templates/${templateId}`,
    input,
    "employee template",
  );
}

export async function setEmployeeTemplateStatus(
  options: ApiClientOptions,
  templateId: string,
  status: EmployeeTemplateStatus,
): Promise<EmployeeTemplate> {
  return patchJson<EmployeeTemplate>(
    options,
    `/api/v1/digital-employee-templates/${templateId}/status`,
    { status },
    "employee template",
  );
}

export async function deleteEmployeeTemplate(
  options: ApiClientOptions,
  templateId: string,
): Promise<void> {
  await deleteJson(options, `/api/v1/digital-employee-templates/${templateId}`, "employee template");
}
```

`getJson`/`postJson`/`patchJson`/`deleteJson` signatures are `(options, path, [input,] resource)`, verified against `apps/web/src/lib/api/client.ts:33-129`.

- [ ] **Step 2: Verify TypeScript compiles**

Run: `corepack pnpm --filter ./apps/web run typecheck` (confirmed present in `apps/web/package.json:13`).
Expected: PASS, no errors in the new file.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/lib/api/employee-templates.ts
git commit -m "feat(web): add API client for digital employee template CRUD"
```

---

### Task 13: Frontend `template-utils.ts` adjustments

**Files:**
- Modify: `apps/web/src/features/employees/template-utils.ts`

**Interfaces:**
- Consumes: `EmployeeTemplate` (Task 12).
- Produces: `templateStatusLabel(template: EmployeeTemplate): "已启用" | "已禁用"`, `templateStatusTone(template: EmployeeTemplate): "ok" | "mute"` (replace the team-baseline-derived `templateAvailabilityStatus` label/tone use in the list table — `templateAvailabilityStatus` itself stays, but only for the detail page's inheritance note block per the approved design).

- [ ] **Step 1: Add the new helpers**

Append to `template-utils.ts`:
```typescript
import type { EmployeeTemplate } from "@/lib/api/employee-templates";

export function templateStatusLabel(template: EmployeeTemplate): "已启用" | "已禁用" {
  return template.status === "active" ? "已启用" : "已禁用";
}

export function templateStatusTone(template: EmployeeTemplate): "ok" | "mute" {
  return template.status === "active" ? "ok" : "mute";
}
```
(Add the `EmployeeTemplate` import at the top of the file alongside the existing imports.)

- [ ] **Step 2: Verify TypeScript compiles**

Run: `corepack pnpm --filter ./apps/web run typecheck`.
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add apps/web/src/features/employees/template-utils.ts
git commit -m "feat(web): add template status label/tone helpers"
```

---

### Task 14: Frontend `templates.tsx` — CRUD UI

**Files:**
- Modify: `apps/web/src/features/employees/templates.tsx`

**Interfaces:**
- Consumes: `listEmployeeTemplates`, `createEmployeeTemplate`, `updateEmployeeTemplate`, `setEmployeeTemplateStatus`, `deleteEmployeeTemplate` (Task 12); `templateStatusLabel`, `templateStatusTone` (Task 13); `ConfirmDialog` (`apps/web/src/components/confirm-dialog.tsx`); `Dialog`/`DialogContent`/`DialogHeader`/`DialogTitle`/`DialogDescription` (`@/components/ui/dialog`); `Input`, `Label`, `Textarea` (`@/components/ui`).
- Produces: `TemplateListView` gains "新建模板" page action, per-row "配置"/"启用·禁用"/"删除" actions and an "内置" badge; new `TemplateFormDialog` component (create + edit modes); new delete `ConfirmDialog` usage.

This task replaces the read-only data source (`useTemplateCatalog` → `getDigitalEmployeeCreateOptions`) with the new CRUD-capable `listEmployeeTemplates`, while keeping the "用此模板创建数字员工" flow intact (it only needs `type`, which is present on `EmployeeTemplate` too).

- [ ] **Step 1: Replace the data-fetching hook**

Replace `useTemplateCatalog` (lines 458-489) with:
```typescript
function useTemplateCatalog(apiBaseUrl: string, fetcher?: typeof fetch) {
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const queryClient = useQueryClient();
  const templatesQuery = useQuery({
    queryKey: ["employee-templates"],
    queryFn: () => listEmployeeTemplates(apiOptions),
  });

  return {
    apiOptions,
    error: templatesQuery.error,
    isLoading: templatesQuery.isLoading,
    templates: templatesQuery.data ?? [],
    refetch: async () => {
      await templatesQuery.refetch();
    },
    invalidate: () => queryClient.invalidateQueries({ queryKey: ["employee-templates"] }),
  };
}
```
Add `useQueryClient` to the `@tanstack/react-query` import, and replace the `getDigitalEmployeeCreateOptions`/`listTeams` imports with:
```typescript
import {
  createEmployeeTemplate,
  deleteEmployeeTemplate,
  listEmployeeTemplates,
  setEmployeeTemplateStatus,
  updateEmployeeTemplate,
  type EmployeeTemplate,
} from "@/lib/api/employee-templates";
```
Remove the now-unused `getDigitalEmployeeCreateOptions`, `DigitalEmployeeCreateOptions`, `DigitalEmployeeTypeOption`, `listTeams`, `TeamListItem` imports and the `teamScopeLabel` function — the list/detail views no longer depend on team baseline data for their primary display (the "继承团队基线" note stays conceptually on the detail page per the approved design, but is no longer wired to a live team query in this pass; leave a static note referencing "创建时可能叠加团队治理基线" instead of a live lookup, since re-deriving live team-scoped inheritance data is out of scope for this CRUD feature).

- [ ] **Step 2: Update `TemplateListView` for CRUD actions**

Replace the `TemplateListView` function body to:
- Fetch via the new `useTemplateCatalog`.
- Add local state: `const [formState, setFormState] = useState<{ mode: "create" | "edit"; template?: EmployeeTemplate } | null>(null);` and `const [deleteTarget, setDeleteTarget] = useState<EmployeeTemplate | null>(null);`.
- Sort/filter with the existing `orderedEmployeeTypes`-equivalent logic, but operating on `EmployeeTemplate[]` (the `metadata["system_type"]` filter no longer applies since `custom_agent` is never in this list — drop that filter entirely for this data source).
- Page action: replace the single "创建数字员工" button with two buttons — keep "创建数字员工" (`Link to="/employees/new"`) and add `<V3Button variant="outline" onClick={() => setFormState({ mode: "create" })}>新建模板</V3Button>`.
- Table row actions column: replace the single "查看详情" link with a `<div className="flex items-center gap-2">` containing: "查看详情" (unchanged link), `<V3Button variant="outline" size="sm" onClick={() => setFormState({ mode: "edit", template })}>配置</V3Button>`, a status toggle button (`<V3Button variant="outline" size="sm" onClick={() => statusMutation.mutate(template)}>{template.status === "active" ? "禁用" : "启用"}</V3Button>`), and `<V3Button variant="outline" size="sm" onClick={() => setDeleteTarget(template)}>删除</V3Button>`.
- Add an "内置" `StatusPill` next to the label when `template.is_system` is true.
- Replace the "状态" column cell with `<StatusPill tone={templateStatusTone(template)}>{templateStatusLabel(template)}</StatusPill>`.
- Wire up `useMutation` for status toggle and delete, each calling `state.invalidate()` on success.
- Render `<TemplateFormDialog open={formState !== null} mode={formState?.mode ?? "create"} template={formState?.template} apiOptions={state.apiOptions} onOpenChange={(open) => !open && setFormState(null)} onSaved={() => { setFormState(null); state.invalidate(); }} />` and a `<ConfirmDialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)} title="删除模板" desc={`确认删除模板"${deleteTarget?.label}"？删除后不可恢复，已创建的数字员工不受影响。`} destructive handleConfirm={() => deleteMutation.mutate(deleteTarget!)} />` at the bottom of the view.

- [ ] **Step 3: Add the `TemplateFormDialog` component**

Append a new component to `templates.tsx`:
```typescript
type TemplateFormDialogProps = {
  open: boolean;
  mode: "create" | "edit";
  template?: EmployeeTemplate;
  apiOptions: { baseUrl: string; fetcher?: typeof fetch };
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
};

type TemplateFormDraft = {
  type: string;
  label: string;
  description: string;
  defaultRole: string;
  recommendedSkills: string;
  recommendedMcpServers: string;
  recommendedProviderTypes: string;
  defaultCapabilitySelection: string;
  defaultContextPolicyOverride: string;
  defaultApprovalPolicy: string;
};

function draftFromTemplate(template?: EmployeeTemplate): TemplateFormDraft {
  return {
    type: template?.type ?? "",
    label: template?.label ?? "",
    description: template?.description ?? "",
    defaultRole: template?.default_role ?? "",
    recommendedSkills: (template?.recommended_skills ?? []).join(", "),
    recommendedMcpServers: (template?.recommended_mcp_servers ?? []).join(", "),
    recommendedProviderTypes: (template?.recommended_provider_types ?? []).join(", "),
    defaultCapabilitySelection: JSON.stringify(template?.default_capability_selection ?? {}, null, 2),
    defaultContextPolicyOverride: JSON.stringify(template?.default_context_policy_override ?? {}, null, 2),
    defaultApprovalPolicy: JSON.stringify(template?.default_approval_policy ?? {}, null, 2),
  };
}

function parseCommaList(value: string): string[] {
  return value.split(",").map((item) => item.trim()).filter(Boolean);
}

function TemplateFormDialog({
  open,
  mode,
  template,
  apiOptions,
  onOpenChange,
  onSaved,
}: TemplateFormDialogProps) {
  const [draft, setDraft] = useState<TemplateFormDraft>(() => draftFromTemplate(template));
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      setDraft(draftFromTemplate(template));
      setError("");
    }
  }, [open, template]);

  const mutation = useMutation({
    mutationFn: async () => {
      let capabilitySelection: Record<string, unknown>;
      let contextPolicyOverride: Record<string, unknown>;
      let approvalPolicy: Record<string, unknown>;
      try {
        capabilitySelection = JSON.parse(draft.defaultCapabilitySelection || "{}");
        contextPolicyOverride = JSON.parse(draft.defaultContextPolicyOverride || "{}");
        approvalPolicy = JSON.parse(draft.defaultApprovalPolicy || "{}");
      } catch {
        throw new Error("默认能力选择 / 上下文策略 / 审批策略必须是合法 JSON");
      }
      const payload = {
        label: draft.label.trim(),
        description: draft.description.trim(),
        default_role: draft.defaultRole.trim(),
        recommended_skills: parseCommaList(draft.recommendedSkills),
        recommended_mcp_servers: parseCommaList(draft.recommendedMcpServers),
        recommended_provider_types: parseCommaList(draft.recommendedProviderTypes),
        default_capability_selection: capabilitySelection,
        default_context_policy_override: contextPolicyOverride,
        default_approval_policy: approvalPolicy,
      };
      if (mode === "create") {
        await createEmployeeTemplate(apiOptions, { ...payload, type: draft.type.trim() });
      } else if (template) {
        await updateEmployeeTemplate(apiOptions, template.id, payload);
      }
    },
    onError: (mutationError: unknown) => {
      setError(mutationError instanceof Error ? mutationError.message : "保存失败");
    },
    onSuccess: () => {
      onSaved();
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] w-[90vw] max-w-[640px] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>{mode === "create" ? "新建模板" : `配置模板：${template?.label ?? ""}`}</DialogTitle>
          <DialogDescription>
            模板用于在创建数字员工时带入默认角色、能力建议和治理策略默认值。
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 py-2">
          {mode === "create" ? (
            <div className="grid gap-2">
              <Label>模板标识（type）</Label>
              <Input
                value={draft.type}
                onChange={(e) => setDraft((prev) => ({ ...prev, type: e.target.value }))}
                placeholder="例如 custom_reviewer"
              />
            </div>
          ) : null}
          <div className="grid gap-2">
            <Label>名称</Label>
            <Input
              value={draft.label}
              onChange={(e) => setDraft((prev) => ({ ...prev, label: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>描述</Label>
            <Textarea
              value={draft.description}
              onChange={(e) => setDraft((prev) => ({ ...prev, description: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认角色</Label>
            <Input
              value={draft.defaultRole}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultRole: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>推荐技能（逗号分隔）</Label>
            <Input
              value={draft.recommendedSkills}
              onChange={(e) => setDraft((prev) => ({ ...prev, recommendedSkills: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>推荐 MCP（逗号分隔）</Label>
            <Input
              value={draft.recommendedMcpServers}
              onChange={(e) => setDraft((prev) => ({ ...prev, recommendedMcpServers: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>推荐 Provider（逗号分隔）</Label>
            <Input
              value={draft.recommendedProviderTypes}
              onChange={(e) => setDraft((prev) => ({ ...prev, recommendedProviderTypes: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认能力选择（JSON）</Label>
            <Textarea
              className="font-mono text-xs"
              rows={4}
              value={draft.defaultCapabilitySelection}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultCapabilitySelection: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认上下文策略覆盖（JSON）</Label>
            <Textarea
              className="font-mono text-xs"
              rows={4}
              value={draft.defaultContextPolicyOverride}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultContextPolicyOverride: e.target.value }))}
            />
          </div>
          <div className="grid gap-2">
            <Label>默认审批策略（JSON）</Label>
            <Textarea
              className="font-mono text-xs"
              rows={4}
              value={draft.defaultApprovalPolicy}
              onChange={(e) => setDraft((prev) => ({ ...prev, defaultApprovalPolicy: e.target.value }))}
            />
          </div>
          {error ? <p className="text-sm font-semibold text-destructive">{error}</p> : null}
        </div>
        <div className="flex justify-end gap-2 pt-2">
          <V3Button variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </V3Button>
          <V3Button
            disabled={mutation.isPending || !draft.label.trim() || (mode === "create" && !draft.type.trim())}
            onClick={() => mutation.mutate()}
          >
            保存
          </V3Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
```
Add `useEffect`, `useMutation` to the React/query imports, and `Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle` from `@/components/ui/dialog`, `Input` from `@/components/ui/input`, `Label` from `@/components/ui/label`, `Textarea` from `@/components/ui/textarea`, and `ConfirmDialog` from `@/components/confirm-dialog`.

- [ ] **Step 4: Update `TemplateDetailView`/`TemplateDetailContent`**

Update these to read from `EmployeeTemplate` instead of `DigitalEmployeeTypeOption` (field names are compatible: `label`, `description`, `default_role`, `recommended_skills`, etc. — only `type` stays the same, and the availability/inheritance block's "note" text becomes the static note from Step 1 instead of a live team-scoped derivation). Keep the "用此模板创建数字员工" button unchanged (`Link to="/employees/new" search={{ template: template.type }}`).

- [ ] **Step 5: Manual typecheck**

Run: `corepack pnpm --filter ./apps/web run typecheck`.
Expected: PASS. Fix any type errors before proceeding — do not silence with `any`.

- [ ] **Step 6: Commit**

```bash
git add apps/web/src/features/employees/templates.tsx
git commit -m "feat(web): add create/configure/enable-disable/delete UI to template management page"
```

---

### Task 15: Update frontend tests

**Files:**
- Modify: `apps/web/src/features/employees/templates.test.tsx`

**Interfaces:**
- Consumes: `TemplateListView`, `TemplateDetailView` (Task 14).

- [ ] **Step 1: Replace the fixtures and fetcher**

Replace `createOptionsFixture()`/`activeTeam`/`createTemplatesFetcher()`/`createFailingTeamsFetcher()` with fixtures matching `/api/v1/digital-employee-templates` (`GET` list, `POST` create, `PATCH` update, `PATCH .../status`, `DELETE`), following the same `jsonResponse`/`vi.fn` mocking pattern already in the file (lines 149-189).

- [ ] **Step 2: Update existing test assertions**

- Rename `"renders readonly built-in template rows..."` → `"renders template rows with status and detail links"`, dropping "只读" wording assertions and adding assertions for the new "配置"/"删除"/enable-disable buttons being present.
- Add a new test: clicking "新建模板" opens the dialog, filling `type`+`label` and clicking "保存" calls `POST /api/v1/digital-employee-templates` with the expected body, then the dialog closes and the list refetches.
- Add a new test: clicking "删除" opens the `ConfirmDialog`, confirming calls `DELETE /api/v1/digital-employee-templates/{id}`.
- Add a new test: clicking the status toggle calls `PATCH /api/v1/digital-employee-templates/{id}/status` with the opposite status.
- Keep the "unknown template" detail-view test, adjusted to the new data shape.

- [ ] **Step 3: Run the web test suite for this file**

Run: `corepack pnpm --filter ./apps/web run test -- templates.test.tsx` (the `test` script wraps vitest via `scripts/vitest-run.mjs`, which passes through any args after `--` to vitest, so this filters to the one file).
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/web/src/features/employees/templates.test.tsx
git commit -m "test(web): cover template CRUD interactions in templates.test.tsx"
```

---

### Task 16: Full verification and end-to-end walkthrough

**Files:** none (verification only)

- [ ] **Step 1: Full backend test suite**

Run:
```bash
cd apps/control-plane
go vet ./...
go test ./...
```
Expected: all PASS (DB-gated integration tests SKIP if no `TEST_DATABASE_URL`/`DATABASE_URL` — note which ones skipped in the report). Note: `go build ./...` is deliberately not used anywhere in this plan — `apps/control-plane/generate.go` is a `package main` file with no `func main()` (it exists only to hold a `//go:generate` directive), so `go build ./...` always fails at the repo root with "function main is undeclared" regardless of any change in this plan. This is pre-existing, confirmed present on `main` before this branch. `go vet ./...` compiles every package without attempting to link a binary and is the correct "does it compile" signal; `make build` (which targets `./cmd/control-plane` specifically) is the correct way to verify an actual binary links.

- [ ] **Step 2: Full frontend test suite**

Run: `corepack pnpm --filter ./apps/web run test`
Expected: PASS.

- [ ] **Step 3: Restart services with current code**

Run:
```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```
Expected: control-plane restart runs the Atlas migration automatically (per this repo's convention) and picks up migration 050; both services report running.

- [ ] **Step 4: Real end-to-end walkthrough in the browser**

Using the Chrome dev tools / codex chrome plug per this repo's testing convention (`docs/DESIGN.md` review not required here since no visual/layout system change, only new controls added to an existing page — but re-check `DESIGN.md` if any new component pattern was introduced in Task 14 that isn't already used elsewhere in the app):
1. Navigate to `/employees/templates`. Confirm the 9 built-in templates render with an "内置" badge and "已启用" status.
2. Click "新建模板", fill in `type=e2e_test_template`, `label=E2E 测试模板`, save. Confirm it appears in the list with an active status and no "内置" badge.
3. Click "配置" on it, change the label, save. Confirm the updated label renders.
4. Click "禁用" on it. Confirm status flips to "已禁用".
5. Navigate to `/employees/new` (or wherever the create-employee wizard reads `create-options`) and confirm the disabled template no longer appears as a selectable option, while the still-active ones (including built-ins) do.
6. Go back to `/employees/templates`, click "删除" on the test template, confirm in the dialog, confirm it disappears from the list.
7. Confirm an existing digital employee (if any exist in the dev DB) still loads fine after all the above — proving no regression from the `digital_employees` ↔ template decoupling.

- [ ] **Step 5: Report results**

Summarize: which automated checks passed, what was observed in the browser walkthrough (with confirmation the control-plane/web processes were running current code, not stale), and any blockers encountered (e.g. no reachable Postgres for the integration test in Task 4/Step 4 — flag explicitly rather than silently skipping).
