# Task Prompt Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the backend APIs and frontend UI for the Task Prompt Templates picker with dynamic variable replacement, integrated into the existing task-launch flow.

**Architecture:** Add a `task_prompt_templates` table in Postgres (UUID PK, tenant-first, soft-delete, Atlas-tracked). Generate sqlc queries. Expose list/create/apply REST endpoints under `/api/v1/templates` (the repo's canonical prefix), with server types regenerated from `contracts/control-plane/openapi.yaml` via `go generate ./internal/api/`. The frontend has **no generated API client** — it uses hand-written fetch wrappers in `apps/web/src/lib/api/`, so we add a new wrapper module there. The "✨ 浏览模板库" button and Dialog live **inside the existing `TaskLaunchForm`** (`apps/web/src/features/task-launches/components/task-launch-form.tsx`) so they can call the in-scope `setContent(...)` directly. Backend handler/service/repository are co-located in one `internal/prompttemplate/` package, matching the modern feature-package convention (`internal/capability/`, `internal/skill/`, etc.).

**Tech Stack:** Go 1.22+, PostgreSQL (Atlas-tracked migrations), sqlc (named-arg style), OpenAPI via oapi-codegen (`go:generate`), React, TanStack Router + Query.

## Global Constraints

- **Tenant-first:** Every query starts with `tenant_id`.
- **Scope-aware (SYSTEM / TEAM / PERSONAL):** Resolve the caller's team-id set in the service layer and pass it to SQL as a uuid[] arg — do not join membership tables inside the template query.
- **Atlas migrations are integrity-tracked:** `internal/storage/migrations/atlas.sum` must be refreshed with `atlas migrate hash` after adding/editing any migration file, or `atlas migrate apply` rejects the set.
- **DB-type baseline (follow `037_mcp_http_capability_registry.sql`):** UUID PK `DEFAULT gen_random_uuid()`, `TIMESTAMPTZ`, `CHECK` constraints for every enum column, soft-delete via `deleted_at` + partial indexes `WHERE deleted_at IS NULL`. `tenant_id`/`team_id`/`creator_id` are soft references (no hard FK) — this matches house style.
- **sqlc style:** named args, e.g. `sqlc.arg('tenant_id')::uuid`, optional args `sqlc.narg(...)`.
- **Contract changes:** after editing `openapi.yaml`, run `go generate ./internal/api/` (NOT `make generate-openapi`) and then `node scripts/verify-foundation-contracts.mjs`.
- **Completion gate:** per `CLAUDE.md`, the default done-condition is real end-to-end verification (running Control Plane + DB + browser/curl on the real `/api/v1/templates`), not "commit succeeded". Close out with the `$superteam-completion-check` skill.

---

## Resolved Design Questions (replaces "User Review Required")

- **Web type sync:** Not a concern. `apps/web` has no OpenAPI-generated client; API access is hand-written in `apps/web/src/lib/api/`. Adding a schema to `openapi.yaml` does not require syncing types into web.
- **Dynamic form engine:** None exists. Build a minimal ad-hoc form inside the Dialog, driven by the `variables` schema below. Reuse `@/components/ui/dialog` and mirror `apps/web/src/features/projects/components/submit-demand-dialog.tsx`.
- **`variables` shape** (the contract between DB / API / form):
  ```go
  type PromptTemplateVariable struct {
      Name     string   `json:"name"`               // must match {{name}} tokens in content
      Label    string   `json:"label"`              // UI label
      Type     string   `json:"type"`               // "string" | "text" | "select"
      Required bool     `json:"required"`
      Default  string   `json:"default,omitempty"`
      Options  []string `json:"options,omitempty"`  // populated when type == "select"
  }
  ```
  Server validates: every variable has a non-empty `name`; every `{{token}}` in `content` is declared in `variables`.

---

### Task 1: Database Migration and sqlc Queries

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/038_task_prompt_templates.sql`
- Create: `apps/control-plane/internal/storage/queries/task_prompt_templates.sql`

**Interfaces:**
- Produces: `TaskPromptTemplate` sqlc model + `ListPromptTemplates` / `CreatePromptTemplate` / `IncrementPromptTemplateUseCount` query methods in the `gen/` layer.

- [ ] **Step 1: Write the migration file** (`038_task_prompt_templates.sql`)
```sql
-- Task prompt templates: reusable prompt scaffolds with dynamic variables.
-- Scope: SYSTEM (whole tenant), TEAM (team_id), PERSONAL (creator_id).
-- Soft-delete + partial indexes follow the 037 baseline.

CREATE TABLE IF NOT EXISTS task_prompt_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    category_code VARCHAR(64) NOT NULL DEFAULT 'general',
    scope VARCHAR(16) NOT NULL,
    team_id UUID,
    creator_id UUID NOT NULL,
    variables JSONB NOT NULL DEFAULT '[]'::jsonb,
    use_count INTEGER NOT NULL DEFAULT 0,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_task_prompt_templates_title_not_blank   CHECK (btrim(title)   <> ''),
    CONSTRAINT ck_task_prompt_templates_content_not_blank CHECK (btrim(content) <> ''),
    CONSTRAINT ck_task_prompt_templates_scope             CHECK (scope IN ('SYSTEM', 'TEAM', 'PERSONAL')),
    CONSTRAINT ck_task_prompt_templates_team_scope        CHECK ((scope <> 'TEAM') OR (team_id IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_task_prompt_templates_tenant_active
    ON task_prompt_templates(tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_task_prompt_templates_tenant_scope_title_active
    ON task_prompt_templates(tenant_id, scope, title)
    WHERE deleted_at IS NULL;
```

- [ ] **Step 2: Write sqlc queries** (`queries/task_prompt_templates.sql`). The list query enforces scope visibility using a pre-resolved `team_ids` array (membership resolved in the service):
```sql
-- name: ListPromptTemplates :many
SELECT *
FROM task_prompt_templates
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL
  AND (
        scope = 'SYSTEM'
     OR (scope = 'TEAM'     AND team_id    = ANY(sqlc.arg('team_ids')::uuid[]))
     OR (scope = 'PERSONAL' AND creator_id = sqlc.arg('user_id')::uuid)
  )
ORDER BY use_count DESC, created_at DESC;

-- name: CreatePromptTemplate :one
INSERT INTO task_prompt_templates (
    tenant_id, title, content, category_code, scope, team_id, creator_id, variables
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('title')::text,
    sqlc.arg('content')::text,
    COALESCE(sqlc.narg('category_code')::text, 'general'),
    sqlc.arg('scope')::varchar,
    sqlc.narg('team_id')::uuid,
    sqlc.arg('creator_id')::uuid,
    COALESCE(sqlc.narg('variables')::jsonb, '[]'::jsonb)
)
RETURNING *;

-- name: IncrementPromptTemplateUseCount :exec
UPDATE task_prompt_templates
SET use_count = use_count + 1, updated_at = NOW()
WHERE id = sqlc.arg('id')::uuid
  AND tenant_id = sqlc.arg('tenant_id')::uuid
  AND deleted_at IS NULL;
```

- [ ] **Step 3: Generate sqlc code**
```bash
make -C apps/control-plane generate-sqlc
```

- [ ] **Step 4: Refresh Atlas integrity hash** (required — `atlas.sum` tracks the migration set)
```bash
cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations
```

- [ ] **Step 5: Commit**
```bash
git add apps/control-plane/internal/storage
git commit -m "feat: add task_prompt_templates migration and sqlc queries"
```

### Task 2: `prompttemplate` Package — Types and Repository

> Co-locate handler + service + repository in **one** package `internal/prompttemplate/`, matching `internal/capability/`, `internal/skill/`, etc. Do **not** use `internal/task/template/` (collides with the existing flat `internal/task/` package) and do **not** put the handler in the legacy `internal/api/handlers/` bag.

**Files:**
- Create: `apps/control-plane/internal/prompttemplate/types.go`
- Create: `apps/control-plane/internal/prompttemplate/pg_repository.go`

**Interfaces:**
- Consumes: sqlc models/queries from Task 1.
- Produces: domain `PromptTemplate`, `PromptTemplateVariable`, and a `Repository` interface with a pg implementation.

- [ ] **Step 1: Write domain types** (`types.go`) — `PromptTemplate`, `PromptTemplateVariable` (shape above), plus a `CreateTemplateInput` struct. Include a `Render(content, values map[string]string) string` helper that substitutes `{{name}}` tokens and rejects unknown tokens.

- [ ] **Step 2: Write the pg repository** (`pg_repository.go`) — wrap the sqlc `Queries`, mapping DB rows ↔ domain models (incl. JSONB marshal/unmarshal of `variables`). Methods: `List(ctx, tenantID, teamIDs, userID)`, `Create(ctx, input)`, `IncrementUseCount(ctx, id, tenantID)`.

- [ ] **Step 3: Commit**
```bash
git add apps/control-plane/internal/prompttemplate
git commit -m "feat: prompttemplate domain types and pg repository"
```

### Task 3: Service Layer

**Files:**
- Create: `apps/control-plane/internal/prompttemplate/service.go`

**Interfaces:**
- Consumes: `Repository` (+ the existing team-membership resolver used elsewhere, e.g. the backing logic for `queries/user_project_team_scopes.sql`, to expand the caller's `teamIDs`).
- Produces: `Service` with `ListTemplates`, `CreateTemplate`, `ApplyTemplate` (the last wires the previously-orphan `IncrementPromptTemplateUseCount`).

- [ ] **Step 1: Write service logic**
  - `ListTemplates(ctx, authCtx)`: resolve tenantID + userID + teamIDs from auth context, delegate to repo.
  - `CreateTemplate(ctx, input)`: enforce `scope`/`team_id` consistency (TEAM ⇒ team_id present; team_id must be a team the caller belongs to for non-admin), validate `variables` ↔ `{{tokens}}` per the contract above, then repo.Create.
  - `ApplyTemplate(ctx, id)`: repo.IncrementUseCount (idempotent best-effort; failure must not break the picker). This is what makes `use_count` meaningful.

- [ ] **Step 2: Commit**
```bash
git add apps/control-plane/internal/prompttemplate/service.go
git commit -m "feat: prompttemplate service layer"
```

### Task 4: OpenAPI Spec, Server Generation, Handler, and Wiring

**Files:**
- Modify: `contracts/control-plane/openapi.yaml` (the canonical Console→Control-Plane contract)
- Create: `apps/control-plane/internal/prompttemplate/handler.go`

**Interfaces:**
- Consumes: `Service`.
- Produces: `GET /api/v1/templates`, `POST /api/v1/templates`, `POST /api/v1/templates/{id}/apply` and the `PromptTemplate` / `PromptTemplateVariable` schemas.

- [ ] **Step 1: Add OpenAPI definitions** to `contracts/control-plane/openapi.yaml`:
  - `components/schemas/PromptTemplate` and `PromptTemplateVariable` (mirror the Go shapes above).
  - `paths`:
    - `GET /api/v1/templates` → `listPromptTemplates` (200: `PromptTemplate[]`).
    - `POST /api/v1/templates` → `createPromptTemplate` (201: `PromptTemplate`).
    - `POST /api/v1/templates/{id}/apply` → `applyPromptTemplate` (204).

- [ ] **Step 2: Regenerate server types** — use the `go:generate` directive in `apps/control-plane/internal/api/generate.go` (it points at `contracts/control-plane/oapi-codegen.server.yaml`). Do **not** use `make generate-openapi` — that target only regenerates `auth.yaml` and `authz.yaml` and does not touch `openapi.yaml`.
```bash
cd apps/control-plane && go generate ./internal/api/
```

- [ ] **Step 3: Run contract verification** (CLAUDE.md requires this after any contract edit; `scripts/verify-foundation-contracts.mjs` reads `openapi.yaml`)
```bash
node scripts/verify-foundation-contracts.mjs
```

- [ ] **Step 4: Write the handler** (`handler.go`) as `HTTPHandler` (same shape as `internal/capability/handler.go`): parse auth context, call the service, map domain ↔ generated OpenAPI types. Implement the three operations.

- [ ] **Step 5: Wire the handler** — add a `promptTemplateHandler *prompttemplate.HTTPHandler` field to `Server` in `apps/control-plane/internal/api/server.go`, set it via the same constructor/setter path used by `capabilityHandler`/`skillHandler`, instantiate it in the DI composition root under `apps/control-plane/internal/app/`, and mount the three routes in `registerRoutes()`.

- [ ] **Step 6: Commit**
```bash
git add contracts apps/control-plane/internal
git commit -m "feat: prompt template API (list/create/apply) + handler wiring"
```

### Task 5: Frontend — Templates Dialog in the Task-Launch Form

> Target file is **`apps/web/src/features/task-launches/components/task-launch-form.tsx`** (`TaskLaunchForm`). There is no `features/task/` dir and no `TaskInitiationEditor.tsx`. The prompt field is an inline `<textarea>` controlled by local `useState` `content` / `setContent`, which is private to the component — so the button + dialog must live **inside** `TaskLaunchForm`.

**Files:**
- Create: `apps/web/src/lib/api/prompt-templates.ts` (hand-written fetch wrapper, same style as `lib/api/projects.ts`; uses `client.ts` base + `resolveControlPlaneUrl()`)
- Create: `apps/web/src/features/task-launches/components/prompt-template-dialog.tsx` (Dialog built on `@/components/ui/dialog`, mirroring `features/projects/components/submit-demand-dialog.tsx`)
- Modify: `apps/web/src/features/task-launches/components/task-launch-form.tsx`

**Interfaces:**
- Consumes: the new `prompt-templates.ts` wrapper via TanStack Query.
- Produces: "✨ 浏览模板库" button in the editor toolbar; on confirm, synthesizes the final string and writes it into `content`.

- [ ] **Step 1: Write the API wrapper** (`lib/api/prompt-templates.ts`) — `listPromptTemplates(options)`, `applyPromptTemplate(options, id)` (fire-and-forget increment), and exported `PromptTemplate` / `PromptTemplateVariable` TS types matching the OpenAPI schemas.

- [ ] **Step 2: Build the Dialog** (`prompt-template-dialog.tsx`) — left sidebar of categories, right grid of template cards; on card click, if `variables` is non-empty, render an inline form (string/text/select per `type`) before confirming. On confirm, interpolate `{{name}}` and return the synthesized string via `onInsert(text)`.

- [ ] **Step 3: Integrate inside `TaskLaunchForm`** — add the "✨ 浏览模板库" button next to the `<textarea>`; on `onInsert`, **confirm before overwriting** when `content` is already non-empty (merge/replace choice — do not silently destroy user input), then call `setContent(synthesized)` and fire `applyPromptTemplate` for the use-count increment. Follow `DESIGN.md` and the v3 Soft-Flat primitives (`SoftCard`, `V3Button`) already used in this file.

- [ ] **Step 4: Commit**
```bash
git add apps/web/src
git commit -m "feat(web): task prompt template picker in task-launch form"
```

### Task 6: End-to-End Verification and Completion Check

- [ ] **Step 1: Restart Control Plane with current code**
```bash
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh status
```

- [ ] **Step 2: Apply the migration to the dev DB** (verify `atlas.sum` is valid)
```bash
cd apps/control-plane && DATABASE_URL=<dev url> make migrate-status
cd apps/control-plane && DATABASE_URL=<dev url> make migrate-up
```

- [ ] **Step 3: Real API smoke** — seed one template (POST), then `curl GET /api/v1/templates` with a real authenticated session; confirm scope filtering (SYSTEM visible to all, PERSONAL only to creator, TEAM only to members). Confirm `POST .../{id}/apply` increments `use_count`.

- [ ] **Step 4: Real browser flow** — in the running Web app, open the task-launch page, open the picker, fill variables for a template that has them, confirm the synthesized text lands in the editor (and confirm-before-overwrite behaves correctly). Must be the live Control Plane response, not a mock.

- [ ] **Step 5: Completion check** — run the `$superteam-completion-check` skill (`.codex/skills/superteam-completion-check/SKILL.md`). Do not mark done on build/unit tests alone.

## Deferred (out of scope, note for follow-up)

- Update / delete / soft-delete endpoints and admin authorization for template authoring (Task 4 ships create only).
- A dedicated admin UI for managing the template library.
- Caching/etag on the list endpoint.
