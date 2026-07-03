# Project Task Graph Employee Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `GET /projects/{projectId}/task-graph` return each assigned digital employee's profession label (`employee_role`) and avatar (`avatar_asset`), so the web app can render employee avatars/titles on the plan/workflow graph instead of only a bare display name.

**Architecture:** The `project` package's `Service.GetProjectTaskGraph` already builds a `ProjectTaskGraph` from `Repository.GetProjectTaskGraph`. We add a service-layer enrichment step, injected via a narrow interface (`DigitalEmployeeIdentityLookup`), that fills in `Role`/`AvatarAsset` per employee after the repository call returns. A new adapter in the `project` package wraps `*employee.Service` to satisfy that interface, reusing the employee package's existing (currently unexported) avatar-from-metadata helper. `PgRepository` is untouched — it keeps sourcing `DisplayName`/`ProjectRole`/`Status` from `ListProjectMembers` exactly as today. This mirrors the existing `ApprovalResolver` / `NewApprovalServiceAdapter` pattern already used for `project` → `approval` cross-package wiring.

**Tech Stack:** Go (control-plane), sqlc-free (no new SQL — avatar catalog is an in-memory lookup), OpenAPI (contracts/control-plane), hand-authored TypeScript client types (apps/web).

## Global Constraints

- No new database columns, tables, or migrations — avatar resolution is an existing in-memory catalog lookup (`internal/avatar`), not a DB join.
- `PgRepository` must remain a pure SQL repository — do not add the `employee` package as a dependency of `pg_repository.go`. All employee-identity enrichment happens in `project.Service`, one layer up.
- Follow the existing narrow-interface pattern already used for `ApprovalResolver`/`ProjectTeamScopeAuthorizer` in `internal/project/service.go` — the consumer (`project` package) defines the interface; the concrete adapter lives in `internal/project/*_adapter.go` and imports the provider package (`employee`), not the other way around.
- Wiring of the concrete adapter happens once, at process bootstrap, in `apps/control-plane/internal/app/app.go` — never inside request-handling code.
- New field name is `EmployeeRole` / JSON `employee_role`, deliberately distinct from the existing `ProjectRole` / JSON `project_role` field on the same struct (permission role vs. profession title — do not conflate them).
- After editing `contracts/control-plane/openapi.yaml`, run `corepack pnpm generate:control-plane` and `corepack pnpm verify:contracts` before considering the contract task done.
- Go tests run via `corepack pnpm test:go` from the repo root (uses the root `go.work`, which references `apps/control-plane`'s module).

---

### Task 1: Export the avatar-from-metadata helper in the `employee` package

**Files:**
- Modify: `apps/control-plane/internal/employee/avatar_assets.go:39`
- Modify: `apps/control-plane/internal/employee/pg_repository.go:1112`
- Test: `apps/control-plane/internal/employee/avatar_assets_test.go` (new file)

**Interfaces:**
- Consumes: nothing new.
- Produces: `employee.AvatarAssetFromMetadata(metadata map[string]any) *DigitalEmployeeAvatarAsset` — exported so `internal/project`'s new adapter (Task 4) can call it. Behavior is unchanged from the current unexported `avatarAssetFromEmployeeMetadata`.

This function currently has zero test coverage. We add tests before the rename so we have a safety net, then rename.

- [ ] **Step 1: Write the failing test for current (unexported) behavior**

Create `apps/control-plane/internal/employee/avatar_assets_test.go`:

```go
package employee

import "testing"

func TestAvatarAssetFromMetadataReadsAvatarAssetID(t *testing.T) {
	known := ListDigitalEmployeeAvatarAssets()
	if len(known) == 0 {
		t.Fatal("expected at least one built-in avatar asset for this test to be meaningful")
	}
	want := known[0]

	metadata := map[string]any{"avatar_asset_id": want.ID}

	got := AvatarAssetFromMetadata(metadata)

	if got == nil {
		t.Fatalf("expected avatar asset for id %q, got nil", want.ID)
	}
	if got.ID != want.ID || got.ThumbnailURL != want.ThumbnailURL {
		t.Fatalf("unexpected avatar asset: got %#v, want %#v", got, want)
	}
}

func TestAvatarAssetFromMetadataReadsNestedAvatarObject(t *testing.T) {
	known := ListDigitalEmployeeAvatarAssets()
	if len(known) == 0 {
		t.Fatal("expected at least one built-in avatar asset for this test to be meaningful")
	}
	want := known[0]

	metadata := map[string]any{
		"avatar": map[string]any{"id": want.ID},
	}

	got := AvatarAssetFromMetadata(metadata)

	if got == nil || got.ID != want.ID {
		t.Fatalf("expected avatar asset for id %q from nested avatar object, got %#v", want.ID, got)
	}
}

func TestAvatarAssetFromMetadataReturnsNilWhenMissing(t *testing.T) {
	if got := AvatarAssetFromMetadata(nil); got != nil {
		t.Fatalf("expected nil for nil metadata, got %#v", got)
	}
	if got := AvatarAssetFromMetadata(map[string]any{}); got != nil {
		t.Fatalf("expected nil when no avatar reference present, got %#v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails to compile (function is still unexported as `avatarAssetFromEmployeeMetadata`)**

Run: `cd apps/control-plane && go test ./internal/employee/... -run TestAvatarAssetFromMetadata -v`
Expected: FAIL — `undefined: AvatarAssetFromMetadata`

- [ ] **Step 3: Rename the function to export it**

In `apps/control-plane/internal/employee/avatar_assets.go:39`, change:

```go
func avatarAssetFromEmployeeMetadata(metadata map[string]any) *DigitalEmployeeAvatarAsset {
```

to:

```go
func AvatarAssetFromMetadata(metadata map[string]any) *DigitalEmployeeAvatarAsset {
```

In `apps/control-plane/internal/employee/pg_repository.go:1112`, change:

```go
	return avatarAssetFromEmployeeMetadata(jsonMapFromBytes(metadata))
```

to:

```go
	return AvatarAssetFromMetadata(jsonMapFromBytes(metadata))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/control-plane && go test ./internal/employee/... -run TestAvatarAssetFromMetadata -v`
Expected: PASS (all three subtests)

- [ ] **Step 5: Run the full employee package test suite to confirm the rename didn't break the existing overview-query caller**

Run: `cd apps/control-plane && go test ./internal/employee/... -v 2>&1 | tail -40`
Expected: PASS, no references to the old unexported name remain (compile would already have failed at Step 3 if any did)

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/employee/avatar_assets.go apps/control-plane/internal/employee/avatar_assets_test.go apps/control-plane/internal/employee/pg_repository.go
git commit -m "refactor(employee): export AvatarAssetFromMetadata for cross-package reuse"
```

---

### Task 2: Add employee-identity fields and types to the project task graph domain

**Files:**
- Modify: `apps/control-plane/internal/project/task_graph_types.go`
- Modify: `apps/control-plane/internal/project/service.go:15-40` (struct + interfaces)
- Test: none (see note below)

This task only adds types/fields (no behavior). It exists as its own task because Task 3 (enrichment logic) and Task 4 (adapter) both depend on these exact names/shapes, and a reviewer should be able to approve "the shape is right" independently of "the enrichment logic is right."

**Interfaces:**
- Consumes: nothing new.
- Produces:
  - `ProjectTaskGraphEmployeeAvatarAsset{ID, Label, ImageURL, ThumbnailURL string}` (task_graph_types.go)
  - `ProjectTaskGraphEmployee` gains `EmployeeRole string` and `AvatarAsset *ProjectTaskGraphEmployeeAvatarAsset`
  - `DigitalEmployeeIdentity{Role string, AvatarAsset *ProjectTaskGraphEmployeeAvatarAsset}` (service.go)
  - `DigitalEmployeeIdentityLookup` interface: `GetDigitalEmployeeIdentity(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeIdentity, error)`
  - `Service` struct gains unexported field `digitalEmployeeIdentities DigitalEmployeeIdentityLookup`

`apps/control-plane/internal/project/task_graph_types_test.go` does not exist, and this task adds no behavior (pure struct/interface additions) — there is nothing to unit test yet. Task 3's tests exercise these types. Do not create a placeholder test file just to satisfy TDD ritual for a pure type addition.

- [ ] **Step 1: Add the avatar asset struct and extend `ProjectTaskGraphEmployee`**

In `apps/control-plane/internal/project/task_graph_types.go`, replace:

```go
type ProjectTaskGraphEmployee struct {
	DigitalEmployeeID uuid.UUID
	DisplayName       string
	ProjectRole       ProjectRole
	Status            string
}
```

with:

```go
type ProjectTaskGraphEmployee struct {
	DigitalEmployeeID uuid.UUID
	DisplayName       string
	ProjectRole       ProjectRole
	Status            string
	EmployeeRole      string
	AvatarAsset       *ProjectTaskGraphEmployeeAvatarAsset
}

type ProjectTaskGraphEmployeeAvatarAsset struct {
	ID           string
	Label        string
	ImageURL     string
	ThumbnailURL string
}
```

- [ ] **Step 2: Add the identity lookup interface and type to `service.go`**

In `apps/control-plane/internal/project/service.go`, find the existing interface block (around line 37):

```go
type ApprovalResolver interface {
	ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error
}
```

Add immediately after it:

```go
type DigitalEmployeeIdentity struct {
	Role        string
	AvatarAsset *ProjectTaskGraphEmployeeAvatarAsset
}

type DigitalEmployeeIdentityLookup interface {
	GetDigitalEmployeeIdentity(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeIdentity, error)
}
```

- [ ] **Step 3: Add the field to the `Service` struct**

In `apps/control-plane/internal/project/service.go`, find:

```go
type Service struct {
	repository            Repository
	coordinator           CoordinatorSignalClient
	approvals             ApprovalResolver
	inbox                 DecisionInboxProjector
	archiveArtifactLocker ArchiveArtifactLocker
	teamScopeAuthorizer   ProjectTeamScopeAuthorizer
}
```

Replace with:

```go
type Service struct {
	repository                Repository
	coordinator                CoordinatorSignalClient
	approvals                  ApprovalResolver
	inbox                      DecisionInboxProjector
	archiveArtifactLocker      ArchiveArtifactLocker
	teamScopeAuthorizer        ProjectTeamScopeAuthorizer
	digitalEmployeeIdentities  DigitalEmployeeIdentityLookup
}
```

(Field alignment will be fixed by gofmt in the next step — don't hand-align tabs.)

- [ ] **Step 4: Verify the package still builds**

Run: `cd apps/control-plane && gofmt -w internal/project/service.go internal/project/task_graph_types.go && go build ./...`
Expected: exits 0, no output from gofmt beyond reformatting

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/project/task_graph_types.go apps/control-plane/internal/project/service.go
git commit -m "feat(project): add employee identity types to task graph domain"
```

---

### Task 3: Enrich task graph employees with identity data in `Service.GetProjectTaskGraph`

**Files:**
- Modify: `apps/control-plane/internal/project/service.go` (add setter + enrichment call, near line 79 and line 1349)
- Test: `apps/control-plane/internal/project/service_test.go`

**Interfaces:**
- Consumes: `DigitalEmployeeIdentityLookup`, `DigitalEmployeeIdentity`, `ProjectTaskGraphEmployeeAvatarAsset` from Task 2.
- Produces: `func (s *Service) SetDigitalEmployeeIdentityLookup(lookup DigitalEmployeeIdentityLookup)` — consumed by Task 5 (app.go wiring).

Design decision baked into these tests: if the lookup is unset (nil), or if it returns an ordinary per-employee lookup error, `GetProjectTaskGraph` must NOT fail the whole request — it degrades to leaving `EmployeeRole`/`AvatarAsset` empty for that employee. A missing/renamed/deleted digital employee record must never break the task graph view. Request cancellation and deadline expiry are different: `context.Canceled` and `context.DeadlineExceeded` must propagate so callers do not receive stale work after the request is already dead.

- [ ] **Step 1: Write the failing tests**

In `apps/control-plane/internal/project/service_test.go`, add (near the existing `TestGetProjectTaskGraphBuildsStageSummariesWhenRepositoryOmitsThem` test, using the same `taskGraphLimitRepository` fixture pattern already in this file):

```go
type fakeDigitalEmployeeIdentityLookup struct {
	identities map[uuid.UUID]DigitalEmployeeIdentity
	err        map[uuid.UUID]error
	calls      []uuid.UUID
}

func (f *fakeDigitalEmployeeIdentityLookup) GetDigitalEmployeeIdentity(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeIdentity, error) {
	f.calls = append(f.calls, digitalEmployeeID)
	if err, ok := f.err[digitalEmployeeID]; ok {
		return DigitalEmployeeIdentity{}, err
	}
	return f.identities[digitalEmployeeID], nil
}

func TestGetProjectTaskGraphEnrichesEmployeeIdentityWhenLookupIsSet(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Employees: []ProjectTaskGraphEmployee{{
				DigitalEmployeeID: employeeID,
				DisplayName:       "执行员工",
				Status:            "active",
			}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	lookup := &fakeDigitalEmployeeIdentityLookup{
		identities: map[uuid.UUID]DigitalEmployeeIdentity{
			employeeID: {
				Role: "代码审查员",
				AvatarAsset: &ProjectTaskGraphEmployeeAvatarAsset{
					ID:           "avatar-1",
					Label:        "Adventurer 1",
					ThumbnailURL: "https://example.com/avatar-1-thumb.png",
				},
			},
		},
	}
	service.SetDigitalEmployeeIdentityLookup(lookup)

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if len(graph.Employees) != 1 {
		t.Fatalf("expected one employee, got %#v", graph.Employees)
	}
	got := graph.Employees[0]
	if got.EmployeeRole != "代码审查员" {
		t.Fatalf("expected enriched employee role, got %q", got.EmployeeRole)
	}
	if got.AvatarAsset == nil || got.AvatarAsset.ID != "avatar-1" {
		t.Fatalf("expected enriched avatar asset, got %#v", got.AvatarAsset)
	}
	if len(lookup.calls) != 1 || lookup.calls[0] != employeeID {
		t.Fatalf("expected lookup called once with employee id, got %#v", lookup.calls)
	}
}

func TestGetProjectTaskGraphSkipsEnrichmentWhenLookupIsUnset(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Employees: []ProjectTaskGraphEmployee{{DigitalEmployeeID: employeeID, DisplayName: "执行员工"}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if graph.Employees[0].EmployeeRole != "" || graph.Employees[0].AvatarAsset != nil {
		t.Fatalf("expected no enrichment without a lookup configured, got %#v", graph.Employees[0])
	}
}

func TestGetProjectTaskGraphIgnoresEmployeeIdentityLookupErrors(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Employees: []ProjectTaskGraphEmployee{{DigitalEmployeeID: employeeID, DisplayName: "执行员工"}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	lookup := &fakeDigitalEmployeeIdentityLookup{
		err: map[uuid.UUID]error{employeeID: errors.New("employee not found")},
	}
	service.SetDigitalEmployeeIdentityLookup(lookup)

	graph, err := service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if err != nil {
		t.Fatalf("expected lookup error to be swallowed, got %v", err)
	}
	if graph.Employees[0].EmployeeRole != "" || graph.Employees[0].AvatarAsset != nil {
		t.Fatalf("expected no enrichment when lookup errors, got %#v", graph.Employees[0])
	}
}

func TestGetProjectTaskGraphPropagatesEmployeeIdentityContextCancellation(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	employeeID := uuid.New()
	repo := &taskGraphLimitRepository{
		memoryRepository: newMemoryRepository(),
		graph: ProjectTaskGraph{
			Employees: []ProjectTaskGraphEmployee{{DigitalEmployeeID: employeeID, DisplayName: "执行员工"}},
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	lookup := &fakeDigitalEmployeeIdentityLookup{
		err: map[uuid.UUID]error{employeeID: context.Canceled},
	}
	service.SetDigitalEmployeeIdentityLookup(lookup)

	_, err = service.GetProjectTaskGraph(context.Background(), GetProjectTaskGraphRequest{
		TenantID: tenantID, ProjectID: projectID, DemandID: &demandID,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation to propagate, got %v", err)
	}
}
```

`service_test.go` already imports `"errors"` (used by the third test above) — no import changes needed.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd apps/control-plane && go test ./internal/project/... -run TestGetProjectTaskGraphEnrichesEmployeeIdentity -v`
Expected: FAIL — `service.SetDigitalEmployeeIdentityLookup undefined`

- [ ] **Step 3: Implement the setter and enrichment**

In `apps/control-plane/internal/project/service.go`, find the existing setter (around line 79):

```go
func (s *Service) SetTeamScopeAuthorizer(authorizer ProjectTeamScopeAuthorizer) {
	if s != nil {
		s.teamScopeAuthorizer = authorizer
	}
}
```

Add immediately after it:

```go
func (s *Service) SetDigitalEmployeeIdentityLookup(lookup DigitalEmployeeIdentityLookup) {
	if s != nil {
		s.digitalEmployeeIdentities = lookup
	}
}
```

Then find `GetProjectTaskGraph` (around line 1349):

```go
func (s *Service) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (*ProjectTaskGraph, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || (req.CoordinationJobID == nil && req.DemandID == nil) {
		return nil, ErrInvalidProject
	}
	graph, err := s.repository.GetProjectTaskGraph(ctx, req)
	if err != nil {
		return nil, err
	}
	normalizeProjectTaskGraph(&graph)
	return &graph, nil
}
```

Replace with:

```go
func (s *Service) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (*ProjectTaskGraph, error) {
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || (req.CoordinationJobID == nil && req.DemandID == nil) {
		return nil, ErrInvalidProject
	}
	graph, err := s.repository.GetProjectTaskGraph(ctx, req)
	if err != nil {
		return nil, err
	}
	normalizeProjectTaskGraph(&graph)
	if err := s.enrichProjectTaskGraphEmployeeIdentities(ctx, req.TenantID, &graph); err != nil {
		return nil, err
	}
	return &graph, nil
}

func (s *Service) enrichProjectTaskGraphEmployeeIdentities(ctx context.Context, tenantID uuid.UUID, graph *ProjectTaskGraph) error {
	if s.digitalEmployeeIdentities == nil {
		return nil
	}
	for i := range graph.Employees {
		identity, err := s.digitalEmployeeIdentities.GetDigitalEmployeeIdentity(ctx, tenantID, graph.Employees[i].DigitalEmployeeID)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			continue
		}
		graph.Employees[i].EmployeeRole = identity.Role
		graph.Employees[i].AvatarAsset = identity.AvatarAsset
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/control-plane && go test ./internal/project/... -run TestGetProjectTaskGraph -v`
Expected: PASS for all `TestGetProjectTaskGraph*` tests (including the two pre-existing ones from before this task)

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go
git commit -m "feat(project): enrich task graph employees with identity lookup"
```

---

### Task 4: Add the `employee.Service`-backed identity adapter

**Files:**
- Create: `apps/control-plane/internal/project/digital_employee_identity_adapter.go`
- Test: `apps/control-plane/internal/project/digital_employee_identity_adapter_test.go`

**Interfaces:**
- Consumes: `employee.AvatarAssetFromMetadata` (Task 1), `DigitalEmployeeIdentityLookup`/`DigitalEmployeeIdentity`/`ProjectTaskGraphEmployeeAvatarAsset` (Task 2).
- Produces: `func NewDigitalEmployeeIdentityAdapter(service digitalEmployeeReader) DigitalEmployeeIdentityLookup` — consumed by Task 5 (app.go wiring), where the real argument passed is `*employee.Service` (which structurally satisfies `digitalEmployeeReader`).

The adapter depends on a small unexported interface (`digitalEmployeeReader`) rather than the concrete `*employee.Service`, so this test can use a lightweight fake instead of constructing a full `employee.Service` (which needs a repository, runtime commands, and skill service — see `employee.NewServiceWithProvisioning` in `apps/control-plane/internal/app/app.go:394`). `*employee.Service.GetDigitalEmployee` already matches this interface's signature, so no changes to the `employee` package are needed for this to work at the real call site (Task 5).

- [ ] **Step 1: Write the failing test**

Create `apps/control-plane/internal/project/digital_employee_identity_adapter_test.go`:

```go
package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
)

type fakeDigitalEmployeeReader struct {
	employees map[uuid.UUID]*employee.DigitalEmployee
	err       error
}

func (f *fakeDigitalEmployeeReader) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployee, error) {
	if f.err != nil {
		return nil, f.err
	}
	emp, ok := f.employees[employeeID]
	if !ok {
		return nil, errors.New("employee not found")
	}
	return emp, nil
}

func TestDigitalEmployeeIdentityAdapterReturnsRoleAndAvatar(t *testing.T) {
	tenantID := uuid.New()
	employeeID := uuid.New()
	knownAssets := employee.ListDigitalEmployeeAvatarAssets()
	if len(knownAssets) == 0 {
		t.Fatal("expected at least one built-in avatar asset for this test to be meaningful")
	}
	asset := knownAssets[0]
	reader := &fakeDigitalEmployeeReader{
		employees: map[uuid.UUID]*employee.DigitalEmployee{
			employeeID: {
				ID:   employeeID,
				Role: "代码审查员",
				Metadata: map[string]any{
					"avatar_asset_id": asset.ID,
				},
			},
		},
	}
	adapter := NewDigitalEmployeeIdentityAdapter(reader)

	identity, err := adapter.GetDigitalEmployeeIdentity(context.Background(), tenantID, employeeID)
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if identity.Role != "代码审查员" {
		t.Fatalf("expected role to pass through, got %q", identity.Role)
	}
	if identity.AvatarAsset == nil || identity.AvatarAsset.ID != asset.ID || identity.AvatarAsset.ThumbnailURL != asset.ThumbnailURL {
		t.Fatalf("expected resolved avatar asset, got %#v", identity.AvatarAsset)
	}
}

func TestDigitalEmployeeIdentityAdapterPropagatesReaderError(t *testing.T) {
	reader := &fakeDigitalEmployeeReader{err: errors.New("boom")}
	adapter := NewDigitalEmployeeIdentityAdapter(reader)

	_, err := adapter.GetDigitalEmployeeIdentity(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error to propagate from reader")
	}
}

func TestNewDigitalEmployeeIdentityAdapterReturnsNilForNilReader(t *testing.T) {
	if adapter := NewDigitalEmployeeIdentityAdapter(nil); adapter != nil {
		t.Fatalf("expected nil adapter for nil reader, got %#v", adapter)
	}
}
```

- [ ] **Step 2: Run test to verify it fails to compile**

Run: `cd apps/control-plane && go test ./internal/project/... -run TestDigitalEmployeeIdentityAdapter -v`
Expected: FAIL — `undefined: NewDigitalEmployeeIdentityAdapter`

- [ ] **Step 3: Implement the adapter**

Create `apps/control-plane/internal/project/digital_employee_identity_adapter.go`:

```go
package project

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
)

type digitalEmployeeReader interface {
	GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (*employee.DigitalEmployee, error)
}

type DigitalEmployeeIdentityServiceAdapter struct {
	reader digitalEmployeeReader
}

func NewDigitalEmployeeIdentityAdapter(reader digitalEmployeeReader) DigitalEmployeeIdentityLookup {
	if reader == nil {
		return nil
	}
	return DigitalEmployeeIdentityServiceAdapter{reader: reader}
}

func (a DigitalEmployeeIdentityServiceAdapter) GetDigitalEmployeeIdentity(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeIdentity, error) {
	emp, err := a.reader.GetDigitalEmployee(ctx, tenantID, digitalEmployeeID)
	if err != nil {
		return DigitalEmployeeIdentity{}, err
	}
	identity := DigitalEmployeeIdentity{Role: emp.Role}
	if asset := employee.AvatarAssetFromMetadata(emp.Metadata); asset != nil {
		identity.AvatarAsset = &ProjectTaskGraphEmployeeAvatarAsset{
			ID:           asset.ID,
			Label:        asset.Label,
			ImageURL:     asset.ImageURL,
			ThumbnailURL: asset.ThumbnailURL,
		}
	}
	return identity, nil
}
```

**Important:** `NewDigitalEmployeeIdentityAdapter` returns the interface type `DigitalEmployeeIdentityLookup`. Returning a nil `reader` must produce a truly nil interface value (not a non-nil interface wrapping a nil pointer), which is why the `if reader == nil { return nil }` guard returns the bare `nil` literal rather than a `DigitalEmployeeIdentityServiceAdapter{}` zero value.

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd apps/control-plane && go test ./internal/project/... -run TestDigitalEmployeeIdentityAdapter -v`
Run: `cd apps/control-plane && go test ./internal/project/... -run TestNewDigitalEmployeeIdentityAdapter -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/project/digital_employee_identity_adapter.go apps/control-plane/internal/project/digital_employee_identity_adapter_test.go
git commit -m "feat(project): add employee.Service-backed identity adapter"
```

---

### Task 5: Wire the adapter at bootstrap

**Files:**
- Modify: `apps/control-plane/internal/app/app.go` (near lines 486-496)

**Interfaces:**
- Consumes: `project.NewDigitalEmployeeIdentityAdapter` (Task 4), `Service.SetDigitalEmployeeIdentityLookup` (Task 3), the already-constructed `employeeService` (`apps/control-plane/internal/app/app.go:394`).
- Produces: nothing consumed by later tasks — this is the terminal wiring step for the backend.

This is pure dependency wiring with no independently testable behavior of its own (the behavior it wires together is already covered by Task 3's and Task 4's unit tests). Verification here is a full build + the existing app-level test suite, not a new unit test — do not write a placeholder test for a three-line wiring change.

- [ ] **Step 1: Add the wiring call**

In `apps/control-plane/internal/app/app.go`, find:

```go
	projectService, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(
		projectRepository,
		coordinatorClient,
		project.NewApprovalServiceAdapter(approvalService),
		decisionProjector,
		projectArtifactLocker{artifactService: artifactService, projectEvents: projectRepository},
	)
	if err != nil {
		return nil, err
	}
	inboxService.SetApprovalActionResolver(inbox.NewApprovalActionAdapter(approvalService))
	inboxService.SetProjectDecisionActionResolver(inbox.NewProjectDecisionActionAdapter(projectService))
```

Replace with:

```go
	projectService, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(
		projectRepository,
		coordinatorClient,
		project.NewApprovalServiceAdapter(approvalService),
		decisionProjector,
		projectArtifactLocker{artifactService: artifactService, projectEvents: projectRepository},
	)
	if err != nil {
		return nil, err
	}
	projectService.SetDigitalEmployeeIdentityLookup(project.NewDigitalEmployeeIdentityAdapter(employeeService))
	inboxService.SetApprovalActionResolver(inbox.NewApprovalActionAdapter(approvalService))
	inboxService.SetProjectDecisionActionResolver(inbox.NewProjectDecisionActionAdapter(projectService))
```

`employeeService` is already in scope here — it's constructed earlier at `apps/control-plane/internal/app/app.go:394` (`employee.NewServiceWithProvisioning(...)`), well before this point in the same function.

- [ ] **Step 2: Verify the whole control-plane module builds**

Run: `cd apps/control-plane && go build ./...`
Expected: exits 0

- [ ] **Step 3: Run the full Go test suite to catch any wiring regressions**

Run: `corepack pnpm test:go`
Expected: PASS (from repo root, exercises `go.work`)

- [ ] **Step 4: Commit**

```bash
git add apps/control-plane/internal/app/app.go
git commit -m "feat(app): wire digital employee identity lookup into project service"
```

---

### Task 6: Surface the new fields in the task-graph HTTP response

**Files:**
- Modify: `apps/control-plane/internal/project/handler.go:1923-1928` (response struct)
- Modify: `apps/control-plane/internal/project/handler.go:2708-2719` (mapping function)
- Test: `apps/control-plane/internal/project/handler_test.go` (extend `TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions`)

**Interfaces:**
- Consumes: `ProjectTaskGraphEmployee.EmployeeRole`/`.AvatarAsset` (Task 2), populated end-to-end by Task 3+5 in production, but this task's test injects them directly via `handlerTestService.taskGraph` (bypassing the service layer, matching the existing test's style).
- Produces: `employee_role` and `avatar_asset` (with nested `id`/`label`/`image_url`/`thumbnail_url`) keys in the JSON response for `GET /projects/{projectId}/task-graph`.

- [ ] **Step 1: Write the failing test assertion**

In `apps/control-plane/internal/project/handler_test.go`, find the `Employees` field inside `TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions`'s `service.taskGraph` (around line 954):

```go
			Employees: []ProjectTaskGraphEmployee{{
				DigitalEmployeeID: employeeID,
				DisplayName:       "执行员工",
				ProjectRole:       ProjectRoleExecutor,
				Status:            "active",
			}},
```

Replace with:

```go
			Employees: []ProjectTaskGraphEmployee{{
				DigitalEmployeeID: employeeID,
				DisplayName:       "执行员工",
				ProjectRole:       ProjectRoleExecutor,
				Status:            "active",
				EmployeeRole:      "代码审查员",
				AvatarAsset: &ProjectTaskGraphEmployeeAvatarAsset{
					ID:           "avatar-1",
					Label:        "Adventurer 1",
					ImageURL:     "https://example.com/avatar-1.png",
					ThumbnailURL: "https://example.com/avatar-1-thumb.png",
				},
			}},
```

Then find the existing length-only assertion (around line 1049 in the same test, right after decoding `body`):

```go
	if len(body["employees"].([]any)) != 1 || len(body["runs"].([]any)) != 1 || len(body["execution_summaries"].([]any)) != 1 || len(body["recent_events"].([]any)) != 1 {
```

Keep that line as-is, and add a new assertion block immediately after its closing `}`:

```go
	employeeBody := body["employees"].([]any)[0].(map[string]any)
	if employeeBody["employee_role"] != "代码审查员" {
		t.Fatalf("expected employee_role in response, got %#v", employeeBody)
	}
	avatarBody, ok := employeeBody["avatar_asset"].(map[string]any)
	if !ok || avatarBody["id"] != "avatar-1" || avatarBody["thumbnail_url"] != "https://example.com/avatar-1-thumb.png" {
		t.Fatalf("expected avatar_asset in response, got %#v", employeeBody)
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/control-plane && go test ./internal/project/... -run TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions -v`
Expected: FAIL — `employeeBody["employee_role"]` is nil (field not yet serialized)

- [ ] **Step 3: Extend the response struct**

In `apps/control-plane/internal/project/handler.go:1923-1928`, replace:

```go
type projectTaskGraphEmployeeResponse struct {
	DigitalEmployeeID string      `json:"digital_employee_id"`
	DisplayName       string      `json:"display_name"`
	ProjectRole       ProjectRole `json:"project_role"`
	Status            string      `json:"status"`
}
```

with:

```go
type projectTaskGraphEmployeeResponse struct {
	DigitalEmployeeID string                                   `json:"digital_employee_id"`
	DisplayName       string                                   `json:"display_name"`
	ProjectRole       ProjectRole                               `json:"project_role"`
	Status            string                                   `json:"status"`
	EmployeeRole      string                                   `json:"employee_role,omitempty"`
	AvatarAsset       *projectTaskGraphEmployeeAvatarAssetResponse `json:"avatar_asset,omitempty"`
}

type projectTaskGraphEmployeeAvatarAssetResponse struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	ImageURL     string `json:"image_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}
```

- [ ] **Step 4: Populate the new fields in the mapping function**

In `apps/control-plane/internal/project/handler.go:2708-2719`, replace:

```go
func taskGraphEmployeeResponses(employees []ProjectTaskGraphEmployee) []projectTaskGraphEmployeeResponse {
	responses := make([]projectTaskGraphEmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		responses = append(responses, projectTaskGraphEmployeeResponse{
			DigitalEmployeeID: employee.DigitalEmployeeID.String(),
			DisplayName:       employee.DisplayName,
			ProjectRole:       employee.ProjectRole,
			Status:            employee.Status,
		})
	}
	return responses
}
```

with:

```go
func taskGraphEmployeeResponses(employees []ProjectTaskGraphEmployee) []projectTaskGraphEmployeeResponse {
	responses := make([]projectTaskGraphEmployeeResponse, 0, len(employees))
	for _, employee := range employees {
		responses = append(responses, projectTaskGraphEmployeeResponse{
			DigitalEmployeeID: employee.DigitalEmployeeID.String(),
			DisplayName:       employee.DisplayName,
			ProjectRole:       employee.ProjectRole,
			Status:            employee.Status,
			EmployeeRole:      employee.EmployeeRole,
			AvatarAsset:       taskGraphEmployeeAvatarAssetResponse(employee.AvatarAsset),
		})
	}
	return responses
}

func taskGraphEmployeeAvatarAssetResponse(asset *ProjectTaskGraphEmployeeAvatarAsset) *projectTaskGraphEmployeeAvatarAssetResponse {
	if asset == nil {
		return nil
	}
	return &projectTaskGraphEmployeeAvatarAssetResponse{
		ID:           asset.ID,
		Label:        asset.Label,
		ImageURL:     asset.ImageURL,
		ThumbnailURL: asset.ThumbnailURL,
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd apps/control-plane && gofmt -w internal/project/handler.go && go test ./internal/project/... -run TestGetProjectTaskGraphReturnsNodesEdgesAndDecisions -v`
Expected: PASS

- [ ] **Step 6: Run the full project package suite**

Run: `cd apps/control-plane && go test ./internal/project/... -v 2>&1 | tail -60`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project/handler.go apps/control-plane/internal/project/handler_test.go
git commit -m "feat(project): serialize employee_role and avatar_asset on task graph employees"
```

---

### Task 7: Update the OpenAPI contract and the frontend TypeScript type

**Files:**
- Modify: `contracts/control-plane/openapi.yaml:5353-5369`
- Modify: `apps/control-plane/internal/api/gen/control_plane.gen.go` (generated by `corepack pnpm generate:control-plane` when the schema changes)
- Modify: `apps/web/src/lib/api/projects.ts` (the `ProjectTaskGraphEmployee` type, around line 222-227)

**Interfaces:**
- Consumes: the JSON shape produced by Task 6 (`employee_role: string`, `avatar_asset: {id, label, image_url, thumbnail_url}`).
- Produces: `ProjectTaskGraphEmployeeAvatarAsset` and updated `ProjectTaskGraphEmployee` TS types, ready for the frontend graph-rendering work (a separate, later plan) to consume.

This task has no Go/TS unit test of its own. `verify:contracts` is still required, but it is not enough by itself: the current foundation-contract guard mainly checks route/path presence, not schema-field parity with the hand-authored TypeScript client. This task therefore includes explicit OpenAPI YAML, generated Go, and TypeScript field checks.

- [ ] **Step 1: Update the OpenAPI schema**

In `contracts/control-plane/openapi.yaml:5353-5369`, replace:

```yaml
    ProjectTaskGraphEmployee:
      type: object
      required:
        - digital_employee_id
        - display_name
        - project_role
        - status
      properties:
        digital_employee_id:
          type: string
          format: uuid
        display_name:
          type: string
        project_role:
          $ref: "#/components/schemas/ProjectRole"
        status:
          type: string
```

with:

```yaml
    ProjectTaskGraphEmployee:
      type: object
      required:
        - digital_employee_id
        - display_name
        - project_role
        - status
      properties:
        digital_employee_id:
          type: string
          format: uuid
        display_name:
          type: string
        project_role:
          $ref: "#/components/schemas/ProjectRole"
        status:
          type: string
        employee_role:
          type: string
        avatar_asset:
          $ref: "#/components/schemas/ProjectTaskGraphEmployeeAvatarAsset"
    ProjectTaskGraphEmployeeAvatarAsset:
      type: object
      required:
        - id
        - label
        - image_url
        - thumbnail_url
      properties:
        id:
          type: string
        label:
          type: string
        image_url:
          type: string
        thumbnail_url:
          type: string
```

- [ ] **Step 2: Regenerate the Go server contract types**

Run: `corepack pnpm generate:control-plane`
Expected: exits 0, and `apps/control-plane/internal/api/gen/control_plane.gen.go` now contains `ProjectTaskGraphEmployeeAvatarAsset`, `EmployeeRole`, and `AvatarAsset`. Verify with:

```bash
rg -n "type ProjectTaskGraphEmployeeAvatarAsset|EmployeeRole|AvatarAsset" apps/control-plane/internal/api/gen/control_plane.gen.go
```

Expected: at least one match for each of `ProjectTaskGraphEmployeeAvatarAsset`, `EmployeeRole`, and `AvatarAsset`. If the generated file does not change, stop and inspect the OpenAPI schema placement before continuing — do not assume the generator skipped this schema.

- [ ] **Step 3: Update the frontend TypeScript type**

In `apps/web/src/lib/api/projects.ts`, find (around line 222-227):

```ts
export type ProjectTaskGraphEmployee = {
  digital_employee_id: string;
  display_name: string;
  project_role: ProjectRole;
  status: string;
};
```

Replace with:

```ts
export type ProjectTaskGraphEmployeeAvatarAsset = {
  id: string;
  label: string;
  image_url: string;
  thumbnail_url: string;
};

export type ProjectTaskGraphEmployee = {
  digital_employee_id: string;
  display_name: string;
  project_role: ProjectRole;
  status: string;
  employee_role?: string;
  avatar_asset?: ProjectTaskGraphEmployeeAvatarAsset;
};
```

- [ ] **Step 4: Run explicit schema/type parity checks**

Run:

```bash
ruby -e 'require "yaml"; doc = YAML.load_file("contracts/control-plane/openapi.yaml"); props = doc.dig("components", "schemas", "ProjectTaskGraphEmployee", "properties") || {}; raise "missing employee_role" unless props.key?("employee_role"); raise "missing avatar_asset" unless props.key?("avatar_asset"); raise "missing ProjectTaskGraphEmployeeAvatarAsset" unless doc.dig("components", "schemas", "ProjectTaskGraphEmployeeAvatarAsset"); puts "ProjectTaskGraphEmployee schema ok"'
rg -n "ProjectTaskGraphEmployeeAvatarAsset|employee_role\\?|avatar_asset\\?" apps/web/src/lib/api/projects.ts
rg -n "ProjectTaskGraphEmployeeAvatarAsset|EmployeeRole|AvatarAsset" apps/control-plane/internal/api/gen/control_plane.gen.go
```

Expected: the Ruby command prints `ProjectTaskGraphEmployee schema ok`; both `rg` commands print matches for the new fields/types. This is the actual schema/type drift guard for this task.

- [ ] **Step 5: Run contract verification**

Run: `corepack pnpm verify:contracts`
Expected: exits 0

- [ ] **Step 6: Run the frontend test/typecheck to confirm no consumer of `ProjectTaskGraphEmployee` breaks**

Run: `corepack pnpm --filter ./apps/web run typecheck`
Expected: exits 0 (the new fields are optional, so no existing consumer should fail to typecheck)

- [ ] **Step 7: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api/gen/control_plane.gen.go apps/web/src/lib/api/projects.ts
git commit -m "feat(contracts): add employee_role and avatar_asset to ProjectTaskGraphEmployee"
```

---

### Task 8: Run final verification and real task-graph smoke

**Files:**
- Test only: no source files modified.

**Interfaces:**
- Consumes: all backend, contract, generated-code, and TypeScript changes from Tasks 1-7.
- Produces: completion evidence that a running Control Plane serves `employee_role` and `avatar_asset` from a real task-graph API response. This is required before calling the backend slice done.

- [ ] **Step 1: Run the full local verification stack**

Run:

```bash
corepack pnpm test:go
corepack pnpm verify:contracts
corepack pnpm --filter ./apps/web run typecheck
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 2: Confirm services and restart Control Plane so the live API loads current code**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh status
```

Expected: Control Plane is running after restart. If the script reports a service it does not manage or a failed restart, fix the local service state before continuing.

- [ ] **Step 3: Authenticate and discover a real assigned task-graph fixture**

Run:

```bash
export SUPERTEAM_API_BASE="${SUPERTEAM_API_BASE:-http://127.0.0.1:8081}"
export SUPERTEAM_COOKIE_JAR=".scratch/smoke/project-task-graph-employee-identity.cookie"
mkdir -p "$(dirname "$SUPERTEAM_COOKIE_JAR")"
rm -f "$SUPERTEAM_COOKIE_JAR"

curl -fsS -c "$SUPERTEAM_COOKIE_JAR" -b "$SUPERTEAM_COOKIE_JAR" \
  -H "Content-Type: application/json" \
  --data '{"username":"admin","password":"admin"}' \
  "$SUPERTEAM_API_BASE/api/auth/login" >/dev/null

candidate="$(
  curl -fsS -b "$SUPERTEAM_COOKIE_JAR" "$SUPERTEAM_API_BASE/api/v1/projects?limit=100" |
    jq -r '.[].id' |
    while read -r project_id; do
      curl -fsS -b "$SUPERTEAM_COOKIE_JAR" "$SUPERTEAM_API_BASE/api/v1/projects/$project_id/tasks?limit=1000" |
        jq -r --arg project_id "$project_id" '.[] | select(.demand_id != null and .assigned_digital_employee_id != null) | [$project_id, .demand_id] | @tsv' |
        head -n 1
    done |
    head -n 1
)"

if [ -z "$candidate" ]; then
  echo "no real project task with demand_id and assigned_digital_employee_id found; create or run a project with an assigned digital employee, then rerun this smoke" >&2
  exit 1
fi

SUPERTEAM_PROJECT_ID="$(printf '%s' "$candidate" | cut -f1)"
SUPERTEAM_DEMAND_ID="$(printf '%s' "$candidate" | cut -f2)"
printf 'project=%s demand=%s\n' "$SUPERTEAM_PROJECT_ID" "$SUPERTEAM_DEMAND_ID"
```

Expected: prints a real `project=<uuid> demand=<uuid>`. If no fixture exists, this task is blocked and the feature must not be marked done.

- [ ] **Step 4: Prove the live task-graph response contains enriched employee identity**

Run:

```bash
curl -fsS -b "$SUPERTEAM_COOKIE_JAR" \
  "$SUPERTEAM_API_BASE/api/v1/projects/$SUPERTEAM_PROJECT_ID/task-graph?demand_id=$SUPERTEAM_DEMAND_ID" \
  | tee .scratch/smoke/project-task-graph-employee-identity.json \
  | jq -e '.employees[] | select((.employee_role // "") != "" and (.avatar_asset.id // "") != "" and (.avatar_asset.thumbnail_url // "") != "")'
```

Expected: `jq` exits 0 and prints at least one employee object containing non-empty `employee_role`, `avatar_asset.id`, and `avatar_asset.thumbnail_url`. If the graph exists but no employee has these fields populated, treat this as a blocking bug or missing fixture; do not claim real end-to-end verification passed.

- [ ] **Step 5: Commit final verification metadata only if the implementation created any tracked verification artifact intentionally**

Normally there is nothing to commit in this task. Confirm with:

```bash
git status --short
```

Expected: no unexpected dirty files beyond intentional implementation commits. Do not commit `.scratch/smoke/project-task-graph-employee-identity.cookie` or `.scratch/smoke/project-task-graph-employee-identity.json`.

---

## Done Criteria

- `corepack pnpm test:go` passes.
- `corepack pnpm verify:contracts` passes.
- `corepack pnpm --filter ./apps/web run typecheck` passes.
- Task 8's authenticated live `curl` against a real `GET /api/v1/projects/{projectId}/task-graph?demand_id={demandId}` response shows `employee_role` and `avatar_asset` populated for at least one assigned digital employee. This is the real end-to-end verification required before this backend slice can be called done per this repo's completion rules; a passing Go/TS test suite alone is not sufficient.
