# Skill Installation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build synchronous all-or-nothing skill installation from the skill marketplace into team or digital employee workspaces for `opencode`, `codex`, and `claude-code`.

**Architecture:** Control Plane owns install preflight, authorization, Runtime command dispatch, synchronous wait, successful binding persistence, and failure audit logs. Runtime Agent owns provider-specific skill directory mapping, archive download, checksum validation, safe extraction, atomic replacement, and rollback for the current command. Web exposes one marketplace install flow and skill-detail installation visibility.

**Atomicity & failure semantics:** "All-or-nothing" is guaranteed *per Runtime command* (one runtime node): a single node either fully materializes the skill or rolls back its own filesystem changes. Across multiple nodes (team-scope installs spanning several runtimes) there is no distributed rollback in this slice — the Control Plane has no uninstall command to compensate an already-applied remote node. Therefore: on any node failure the install fails as a whole, **no success rows or bindings are persisted**, and the failure audit record must surface the set of nodes that were already physically applied (`details.applied_nodes`, `details.requires_manual_cleanup`) so an operator can clean them up. Do not claim distributed atomicity; claim per-command atomicity plus explicit surfacing of partial physical state.

**Tech Stack:** Go Control Plane with chi handlers, pgx/sqlc migrations and hand-written skill repository SQL, PostgreSQL, OpenAPI contract, Rust Runtime Agent, React/TanStack Query/TanStack Router, Vitest Browser, Cargo tests.

---

## File Structure

Create:

- `apps/control-plane/internal/storage/migrations/035_skill_installations.sql` - successful physical install records.
- `apps/control-plane/internal/skill/install_service.go` - install preflight, Runtime command dispatch, wait, binding/install persistence, failure recording.
- `apps/control-plane/internal/skill/install_service_test.go` - preflight, all-or-nothing, timeout, and persistence service tests.
- `apps/control-plane/internal/skill/install_handler_test.go` - focused handler tests for `POST /skills/{id}/install` and installations listing.
- `apps/runtime-agent/src/commands/install_skills.rs` - Runtime install command payload, provider directory mapping, install orchestration, rollback helpers.
- `apps/runtime-agent/tests/install_skills_test.rs` - Runtime command and filesystem behavior tests.
- `apps/web/src/features/skills/install-dialog.tsx` - marketplace install dialog that loads installable teams and digital employees from their source APIs.

Modify:

- `apps/control-plane/internal/storage/migrations/atlas.sum` - migration hash.
- `apps/control-plane/internal/storage/migrations_test.go` - migration invariants.
- `apps/control-plane/internal/skill/types.go` - install request/response/domain types.
- `apps/control-plane/internal/skill/service.go` - repository/service interfaces and install service wiring.
- `apps/control-plane/internal/skill/pg_repository.go` - install target queries, transaction persistence, installation listing.
- `apps/control-plane/internal/skill/handler.go` - install and installation-list HTTP endpoints/responses.
- `apps/control-plane/internal/api/server.go` - skill install routes.
- `apps/control-plane/internal/api/skill_routes_test.go` - route registration and authz coverage.
- `apps/control-plane/internal/app/app.go` - inject Runtime command dispatcher into skill service.
- `contracts/control-plane/openapi.yaml` - install endpoints and schemas.
- `apps/runtime-agent/src/controlplane/models.rs` - `install_skills` command type.
- `apps/runtime-agent/src/commands/executor.rs` - route `InstallSkills` to the new handler.
- `apps/runtime-agent/src/skills.rs` - provider-specific target directories and reusable materialization primitives.
- `apps/runtime-agent/tests/runtime_command_payload_test.rs` - command parsing tests.
- `apps/web/src/lib/api/skills.ts` - install API types and clients.
- `apps/web/src/lib/api/skills.test.ts` - API client tests.
- `apps/web/src/features/skills/index.tsx` - install button and dialog state.
- `apps/web/src/features/skills/index.test.tsx` - marketplace install UI tests.
- `apps/web/src/features/skills/detail.tsx` - display successful installations.
- `apps/web/src/features/skills/detail.test.tsx` - detail page installation rendering.
- `CHANGELOG.md` - final timestamped implementation note after verification.

Generated:

- `apps/control-plane/internal/api/gen/control_plane.gen.go` - generated server interface and schema types from `contracts/control-plane/openapi.yaml`.

Do not add sqlc query files for this slice. Keep install-specific SQL in `apps/control-plane/internal/skill/pg_repository.go` to match the current skill module pattern.

## Task 1: Storage Model For Successful Installations

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/035_skill_installations.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`

- [ ] **Step 1: Add the failing migration test**

Append this test to `apps/control-plane/internal/storage/migrations_test.go`:

```go
func TestSkillInstallationsMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/035_skill_installations.sql")
	if err != nil {
		t.Fatalf("read skill installations migration: %v", err)
	}
	sql := string(body)

	for _, expected := range []string{
		"CREATE TABLE skill_installations",
		"id UUID PRIMARY KEY DEFAULT gen_random_uuid()",
		"tenant_id UUID NOT NULL",
		"skill_id UUID NOT NULL",
		"target_scope VARCHAR(40) NOT NULL",
		"team_id UUID",
		"digital_employee_id UUID NOT NULL",
		"runtime_node_id UUID NOT NULL",
		"provider_type VARCHAR(80) NOT NULL",
		"installed_path TEXT NOT NULL",
		"archive_checksum_sha256 VARCHAR(64) NOT NULL",
		"status VARCHAR(40) NOT NULL DEFAULT 'installed'",
		"installed_by UUID",
		"installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"metadata JSONB NOT NULL DEFAULT '{}'::jsonb",
		"deleted_at TIMESTAMPTZ",
		"CREATE UNIQUE INDEX uq_skill_installations_active_employee",
		"CREATE INDEX idx_skill_installations_skill",
		"CREATE INDEX idx_skill_installations_employee",
		"CREATE INDEX idx_skill_installations_team",
		"COMMENT ON TABLE skill_installations IS",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("expected skill installations migration to contain %q", expected)
		}
	}

	for _, forbidden := range []string{
		"CREATE TYPE skill_install",
		"ON DELETE CASCADE",
		"BIGSERIAL",
		"status VARCHAR(40) NOT NULL DEFAULT 'failed'",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("skill installations migration must not contain %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: Run the migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestSkillInstallationsMigration -count=1
```

Expected: FAIL with `read skill installations migration`.

- [ ] **Step 3: Add the migration**

Create `apps/control-plane/internal/storage/migrations/035_skill_installations.sql`:

```sql
CREATE TABLE skill_installations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    skill_id UUID NOT NULL,
    target_scope VARCHAR(40) NOT NULL,
    team_id UUID,
    digital_employee_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    provider_type VARCHAR(80) NOT NULL,
    installed_path TEXT NOT NULL,
    archive_checksum_sha256 VARCHAR(64) NOT NULL,
    status VARCHAR(40) NOT NULL DEFAULT 'installed',
    installed_by UUID,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT skill_installations_scope_supported CHECK (target_scope IN ('team', 'employee')),
    CONSTRAINT skill_installations_provider_supported CHECK (provider_type IN ('opencode', 'codex', 'claude-code')),
    CONSTRAINT skill_installations_status_supported CHECK (status = 'installed'),
    CONSTRAINT skill_installations_installed_path_not_blank CHECK (btrim(installed_path) <> ''),
    CONSTRAINT skill_installations_checksum_not_blank CHECK (btrim(archive_checksum_sha256) <> '')
);

CREATE UNIQUE INDEX uq_skill_installations_active_employee
    ON skill_installations (tenant_id, skill_id, digital_employee_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_skill_installations_skill
    ON skill_installations (tenant_id, skill_id, installed_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_skill_installations_employee
    ON skill_installations (tenant_id, digital_employee_id, installed_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_skill_installations_team
    ON skill_installations (tenant_id, team_id, installed_at DESC)
    WHERE deleted_at IS NULL;

COMMENT ON TABLE skill_installations IS '技能物理安装记录，只保存已成功写入数字员工 workspace 的安装事实';
COMMENT ON COLUMN skill_installations.target_scope IS '安装请求目标范围，team 表示团队批量安装，employee 表示单个数字员工安装';
COMMENT ON COLUMN skill_installations.installed_path IS 'Runtime Agent 实际写入的 provider 官方技能目录';
COMMENT ON COLUMN skill_installations.metadata IS '安装命令、Runtime 回执和排障扩展信息';
```

- [ ] **Step 4: Run the migration test and verify it passes**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestSkillInstallationsMigration -count=1
```

Expected: PASS.

- [ ] **Step 5: Refresh Atlas migration hash**

Run:

```bash
atlas migrate hash --dir file://apps/control-plane/internal/storage/migrations
```

Expected: `apps/control-plane/internal/storage/migrations/atlas.sum` updates and includes `035_skill_installations.sql`.

- [ ] **Step 6: Commit storage changes**

Run:

```bash
git add apps/control-plane/internal/storage/migrations/035_skill_installations.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go
git commit -m "feat: add skill installation storage"
```

Expected: commit includes only migration and migration test files.

## Task 2: Control Plane Install Domain And Preflight

**Files:**
- Modify: `apps/control-plane/internal/skill/types.go`
- Create: `apps/control-plane/internal/skill/install_service.go`
- Create: `apps/control-plane/internal/skill/install_service_test.go`
- Modify: `apps/control-plane/internal/skill/service.go`

- [ ] **Step 1: Add failing service tests for preflight**

Create `apps/control-plane/internal/skill/install_service_test.go`:

```go
package skill

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	cpruntime "github.com/superteam/control-plane/internal/runtime"
)

func TestInstallSkillPreflightRejectsUnsupportedProviderBeforeDispatch(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	repo := newInstallServiceRepoFixture(tenantID, skillID)
	repo.targets = []SkillInstallTarget{{
		TenantID: tenantID, TeamID: uuid.New(), DigitalEmployeeID: employeeID,
		RuntimeNodeID: uuid.New(), NodeID: "node-1", ProviderType: "other",
		AgentHomeDir: "/tmp/employee",
	}}
	dispatcher := &installServiceDispatcher{connected: map[string]bool{"node-1": true}}
	service := NewInstallService(repo, dispatcher, InstallServiceOptions{Timeout: 50 * time.Millisecond, PollInterval: time.Millisecond})

	result, err := service.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID: tenantID, SkillID: skillID, TargetScope: SkillInstallTargetEmployee,
		DigitalEmployeeID: employeeID, ActorUserID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected preflight error")
	}
	var installErr *InstallSkillError
	if !errors.As(err, &installErr) {
		t.Fatalf("expected InstallSkillError, got %T: %v", err, err)
	}
	if installErr.Phase != InstallFailurePhasePreflight {
		t.Fatalf("phase = %q, want preflight", installErr.Phase)
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("preflight failure must not dispatch commands: %#v", dispatcher.commands)
	}
	if len(repo.installations) != 0 || len(repo.teamBindings) != 0 || len(repo.employeeBindings) != 0 {
		t.Fatalf("preflight failure must not persist success rows: repo=%#v", repo)
	}
	if len(result.BlockedTargets) != 1 || result.BlockedTargets[0].ReasonCode != "unsupported_provider" {
		t.Fatalf("unexpected blockers: %#v", result.BlockedTargets)
	}
}

func TestInstallSkillPreflightRejectsMissingArchiveBeforeDispatch(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	repo := newInstallServiceRepoFixture(tenantID, skillID)
	repo.skill.ArchiveObjectRef = ""
	repo.targets = []SkillInstallTarget{{
		TenantID: tenantID, TeamID: uuid.New(), DigitalEmployeeID: employeeID,
		RuntimeNodeID: uuid.New(), NodeID: "node-1", ProviderType: "codex",
		AgentHomeDir: "/tmp/employee",
	}}
	dispatcher := &installServiceDispatcher{connected: map[string]bool{"node-1": true}}
	service := NewInstallService(repo, dispatcher, InstallServiceOptions{Timeout: 50 * time.Millisecond, PollInterval: time.Millisecond})

	_, err := service.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID: tenantID, SkillID: skillID, TargetScope: SkillInstallTargetEmployee,
		DigitalEmployeeID: employeeID, ActorUserID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected missing archive error")
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("missing archive must not dispatch commands: %#v", dispatcher.commands)
	}
	if got := repo.failureLogs[0].ReasonCode; got != "skill_archive_missing" {
		t.Fatalf("failure reason = %q, want skill_archive_missing", got)
	}
}

func TestInstallSkillPreflightRejectsDisconnectedRuntimeBeforeDispatch(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	repo := newInstallServiceRepoFixture(tenantID, skillID)
	repo.targets = []SkillInstallTarget{{
		TenantID: tenantID, TeamID: uuid.New(), DigitalEmployeeID: employeeID,
		RuntimeNodeID: uuid.New(), NodeID: "node-1", ProviderType: "codex",
		AgentHomeDir: "/tmp/employee",
	}}
	dispatcher := &installServiceDispatcher{connected: map[string]bool{}}
	service := NewInstallService(repo, dispatcher, InstallServiceOptions{Timeout: 50 * time.Millisecond, PollInterval: time.Millisecond})

	_, err := service.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID: tenantID, SkillID: skillID, TargetScope: SkillInstallTargetEmployee,
		DigitalEmployeeID: employeeID, ActorUserID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected runtime unavailable error")
	}
	if len(dispatcher.commands) != 0 {
		t.Fatalf("disconnected runtime must not dispatch commands: %#v", dispatcher.commands)
	}
	if got := repo.failureLogs[0].ReasonCode; got != "runtime_not_connected" {
		t.Fatalf("failure reason = %q, want runtime_not_connected", got)
	}
}

type installServiceRepo struct {
	skill            *Skill
	targets          []SkillInstallTarget
	receipts         map[string]*RuntimeInstallCommandReceipt
	waitErr          error
	installations    []SkillInstallation
	teamBindings     []BindTeamSkillRequest
	employeeBindings []BindEmployeeSkillRequest
	failureLogs      []SkillInstallFailureLog
}

func newInstallServiceRepoFixture(tenantID, skillID uuid.UUID) *installServiceRepo {
	return &installServiceRepo{
		skill: &Skill{
			ID: skillID, TenantID: tenantID, Slug: "review",
			ArchiveObjectRef: "s3://bucket/skills/review.zip",
			ArchiveChecksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			ArchiveSizeBytes: 128, ArchiveFileCount: 1,
		},
		receipts: map[string]*RuntimeInstallCommandReceipt{},
	}
}

func (r *installServiceRepo) GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error) {
	if r.skill == nil || r.skill.ID != req.SkillID || r.skill.TenantID != req.TenantID {
		return nil, ErrNotFound
	}
	return r.skill, nil
}

func (r *installServiceRepo) ListInstallTargets(ctx context.Context, req ListSkillInstallTargetsRequest) ([]SkillInstallTarget, error) {
	return append([]SkillInstallTarget(nil), r.targets...), nil
}

func (r *installServiceRepo) CreateInstallCommandReceipt(ctx context.Context, req CreateSkillInstallCommandReceiptRequest) error {
	r.receipts[req.CommandID] = &RuntimeInstallCommandReceipt{
		CommandID: req.CommandID, Status: "pending", RuntimeNodeID: req.RuntimeNodeID,
		NodeID: req.NodeID, Payload: req.Payload,
	}
	return nil
}

func (r *installServiceRepo) WaitForInstallCommand(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeInstallCommandReceipt, error) {
	if r.waitErr != nil {
		return nil, r.waitErr
	}
	receipt := r.receipts[commandID]
	if receipt == nil {
		return nil, ErrNotFound
	}
	if receipt.Status == "pending" {
		receipt.Status = "completed"
		receipt.Result = map[string]any{"installed": []any{}}
	}
	return receipt, nil
}

func (r *installServiceRepo) PersistInstallSuccess(ctx context.Context, req PersistSkillInstallSuccessRequest) (InstallSkillResult, error) {
	r.installations = append(r.installations, req.Installations...)
	if req.TargetScope == SkillInstallTargetTeam {
		r.teamBindings = append(r.teamBindings, BindTeamSkillRequest{TenantID: req.TenantID, TeamID: req.TeamID, SkillID: req.SkillID})
	} else {
		for _, installation := range req.Installations {
			r.employeeBindings = append(r.employeeBindings, BindEmployeeSkillRequest{TenantID: req.TenantID, DigitalEmployeeID: installation.DigitalEmployeeID, SkillID: req.SkillID})
		}
	}
	return InstallSkillResult{SkillID: req.SkillID, TargetScope: req.TargetScope, TeamID: req.TeamID, InstalledCount: len(req.Installations), Installations: req.Installations}, nil
}

func (r *installServiceRepo) RecordInstallFailure(ctx context.Context, req SkillInstallFailureLog) error {
	r.failureLogs = append(r.failureLogs, req)
	return nil
}

type installServiceDispatcher struct {
	connected map[string]bool
	commands  []cpruntime.RuntimeCommand
}

func (d *installServiceDispatcher) IsConnected(nodeID string) bool {
	return d.connected[nodeID]
}

func (d *installServiceDispatcher) Dispatch(ctx context.Context, nodeID string, command cpruntime.RuntimeCommand) error {
	d.commands = append(d.commands, command)
	return nil
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/skill -run 'TestInstallSkillPreflight' -count=1
```

Expected: FAIL because install types and `NewInstallService` do not exist.

- [ ] **Step 3: Add install domain types**

Append to `apps/control-plane/internal/skill/types.go`:

```go
type SkillInstallTargetScope string

const (
	SkillInstallTargetTeam     SkillInstallTargetScope = "team"
	SkillInstallTargetEmployee SkillInstallTargetScope = "employee"
)

type InstallFailurePhase string

const (
	InstallFailurePhasePreflight      InstallFailurePhase = "preflight"
	InstallFailurePhaseRuntimeInstall InstallFailurePhase = "runtime_install"
	InstallFailurePhaseTimeout        InstallFailurePhase = "timeout"
)

type InstallSkillRequest struct {
	TenantID          uuid.UUID
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	ActorUserID       uuid.UUID
	Timeout           time.Duration
}

type SkillInstallTarget struct {
	TenantID          uuid.UUID
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	EmployeeName      string
	RuntimeNodeID     uuid.UUID
	NodeID            string
	ProviderType      string
	AgentHomeDir      string
}

type SkillInstallation struct {
	ID                    uuid.UUID
	TenantID              uuid.UUID
	SkillID               uuid.UUID
	TargetScope           SkillInstallTargetScope
	TeamID                uuid.UUID
	DigitalEmployeeID     uuid.UUID
	EmployeeName          string
	RuntimeNodeID         uuid.UUID
	NodeID                string
	ProviderType          string
	InstalledPath         string
	ArchiveChecksumSHA256 string
	InstalledBy           uuid.UUID
	InstalledAt           time.Time
	Metadata              map[string]any
}

type InstallSkillResult struct {
	SkillID         uuid.UUID
	TargetScope     SkillInstallTargetScope
	TeamID          uuid.UUID
	DigitalEmployeeID uuid.UUID
	InstalledCount  int
	Installations   []SkillInstallation
	BlockedTargets  []SkillInstallBlockedTarget
}

type SkillInstallBlockedTarget struct {
	DigitalEmployeeID uuid.UUID `json:"digital_employee_id"`
	EmployeeName      string    `json:"employee_name,omitempty"`
	ProviderType      string    `json:"provider_type,omitempty"`
	RuntimeNodeID     uuid.UUID `json:"runtime_node_id,omitempty"`
	NodeID            string    `json:"node_id,omitempty"`
	ReasonCode       string    `json:"reason_code"`
	Message          string    `json:"message"`
}

type InstallSkillError struct {
	Phase          InstallFailurePhase
	Message        string
	BlockedTargets []SkillInstallBlockedTarget
}

func (e *InstallSkillError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type ListSkillInstallTargetsRequest struct {
	TenantID          uuid.UUID
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
}

type CreateSkillInstallCommandReceiptRequest struct {
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceID    uuid.UUID
	Payload       map[string]any
}

type RuntimeInstallCommandReceipt struct {
	CommandID     string
	Status        string
	RuntimeNodeID uuid.UUID
	NodeID        string
	Payload       map[string]any
	Result        map[string]any
	ErrorMessage  string
}

type PersistSkillInstallSuccessRequest struct {
	TenantID      uuid.UUID
	SkillID       uuid.UUID
	TargetScope   SkillInstallTargetScope
	TeamID        uuid.UUID
	InstalledBy   uuid.UUID
	Installations []SkillInstallation
}

type SkillInstallFailureLog struct {
	TenantID          uuid.UUID
	SkillID           uuid.UUID
	TargetScope       SkillInstallTargetScope
	TeamID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	RuntimeNodeID     uuid.UUID
	ProviderType      string
	Phase             InstallFailurePhase
	ReasonCode        string
	Message           string
	CommandID         string
	Details           map[string]any
}
```

- [ ] **Step 4: Add service interfaces and constructor**

Create `apps/control-plane/internal/skill/install_service.go`:

```go
package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	cpruntime "github.com/superteam/control-plane/internal/runtime"
)

const (
	defaultInstallTimeout      = 15 * time.Second
	defaultInstallPollInterval = 100 * time.Millisecond
)

var supportedInstallProviders = map[string]struct{}{
	"opencode":    {},
	"codex":       {},
	"claude-code": {},
}

type InstallRepository interface {
	GetSkill(ctx context.Context, req GetSkillRequest) (*Skill, error)
	ListInstallTargets(ctx context.Context, req ListSkillInstallTargetsRequest) ([]SkillInstallTarget, error)
	CreateInstallCommandReceipt(ctx context.Context, req CreateSkillInstallCommandReceiptRequest) error
	WaitForInstallCommand(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeInstallCommandReceipt, error)
	PersistInstallSuccess(ctx context.Context, req PersistSkillInstallSuccessRequest) (InstallSkillResult, error)
	RecordInstallFailure(ctx context.Context, req SkillInstallFailureLog) error
}

type InstallRuntimeDispatcher interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command cpruntime.RuntimeCommand) error
}

type InstallServiceOptions struct {
	Timeout      time.Duration
	PollInterval time.Duration
}

type InstallService struct {
	repository   InstallRepository
	dispatcher   InstallRuntimeDispatcher
	timeout      time.Duration
	pollInterval time.Duration
}

func NewInstallService(repository InstallRepository, dispatcher InstallRuntimeDispatcher, options InstallServiceOptions) *InstallService {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultInstallTimeout
	}
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = defaultInstallPollInterval
	}
	return &InstallService{repository: repository, dispatcher: dispatcher, timeout: timeout, pollInterval: pollInterval}
}

func (s *InstallService) InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	if s == nil || s.repository == nil || s.dispatcher == nil {
		return InstallSkillResult{}, fmt.Errorf("%w: install service is not configured", ErrInvalidInput)
	}
	if req.TenantID == uuid.Nil || req.SkillID == uuid.Nil {
		return InstallSkillResult{}, fmt.Errorf("%w: tenant_id and skill_id are required", ErrInvalidInput)
	}
	if req.TargetScope != SkillInstallTargetTeam && req.TargetScope != SkillInstallTargetEmployee {
		return InstallSkillResult{}, fmt.Errorf("%w: target_scope must be team or employee", ErrInvalidInput)
	}
	skill, err := s.repository.GetSkill(ctx, GetSkillRequest{TenantID: req.TenantID, SkillID: req.SkillID})
	if err != nil {
		return InstallSkillResult{}, err
	}
	targets, err := s.repository.ListInstallTargets(ctx, ListSkillInstallTargetsRequest{
		TenantID: req.TenantID, SkillID: req.SkillID, TargetScope: req.TargetScope,
		TeamID: req.TeamID, DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return InstallSkillResult{}, err
	}
	blockers := s.preflight(skill, targets)
	result := InstallSkillResult{SkillID: req.SkillID, TargetScope: req.TargetScope, TeamID: req.TeamID, DigitalEmployeeID: req.DigitalEmployeeID, BlockedTargets: blockers}
	if len(blockers) > 0 {
		_ = s.recordFailure(ctx, req, InstallFailurePhasePreflight, blockers[0].ReasonCode, "skill install preflight failed", "", blockers)
		return result, &InstallSkillError{Phase: InstallFailurePhasePreflight, Message: "skill install preflight failed", BlockedTargets: blockers}
	}
	return result, fmt.Errorf("%w: runtime install dispatch not implemented", ErrInvalidInput)
}

func (s *InstallService) preflight(skill *Skill, targets []SkillInstallTarget) []SkillInstallBlockedTarget {
	var blockers []SkillInstallBlockedTarget
	if strings.TrimSpace(skill.ArchiveObjectRef) == "" || strings.TrimSpace(skill.ArchiveChecksum) == "" || skill.ArchiveSizeBytes <= 0 || skill.ArchiveFileCount <= 0 {
		return []SkillInstallBlockedTarget{{ReasonCode: "skill_archive_missing", Message: "skill archive metadata is incomplete"}}
	}
	if len(targets) == 0 {
		return []SkillInstallBlockedTarget{{ReasonCode: "no_install_targets", Message: "no digital employees matched the install target"}}
	}
	for _, target := range targets {
		if target.RuntimeNodeID == uuid.Nil || strings.TrimSpace(target.NodeID) == "" {
			blockers = append(blockers, blockedTarget(target, "runtime_missing", "digital employee has no active Runtime"))
			continue
		}
		if strings.TrimSpace(target.AgentHomeDir) == "" {
			blockers = append(blockers, blockedTarget(target, "agent_home_dir_missing", "digital employee has no agent_home_dir"))
			continue
		}
		if _, ok := supportedInstallProviders[target.ProviderType]; !ok {
			blockers = append(blockers, blockedTarget(target, "unsupported_provider", "provider_type must be one of opencode, codex, claude-code"))
			continue
		}
		if !s.dispatcher.IsConnected(target.NodeID) {
			blockers = append(blockers, blockedTarget(target, "runtime_not_connected", "Runtime is not connected"))
			continue
		}
	}
	return blockers
}

func blockedTarget(target SkillInstallTarget, reasonCode, message string) SkillInstallBlockedTarget {
	return SkillInstallBlockedTarget{
		DigitalEmployeeID: target.DigitalEmployeeID,
		EmployeeName: target.EmployeeName,
		ProviderType: target.ProviderType,
		RuntimeNodeID: target.RuntimeNodeID,
		NodeID: target.NodeID,
		ReasonCode: reasonCode,
		Message: message,
	}
}

func (s *InstallService) recordFailure(ctx context.Context, req InstallSkillRequest, phase InstallFailurePhase, reasonCode, message, commandID string, blockers []SkillInstallBlockedTarget) error {
	details := map[string]any{"blocked_targets": blockers}
	return s.repository.RecordInstallFailure(ctx, SkillInstallFailureLog{
		TenantID: req.TenantID, SkillID: req.SkillID, TargetScope: req.TargetScope,
		TeamID: req.TeamID, DigitalEmployeeID: req.DigitalEmployeeID,
		Phase: phase, ReasonCode: reasonCode, Message: message, CommandID: commandID, Details: details,
	})
}

func runtimeInstallCommand(id string, payload map[string]any) (cpruntime.RuntimeCommand, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return cpruntime.RuntimeCommand{}, fmt.Errorf("encode runtime install command payload: %w", err)
	}
	return cpruntime.RuntimeCommand{ID: id, Type: "install_skills", Payload: encoded}, nil
}
```

- [ ] **Step 5: Run the tests and verify preflight passes**

Run:

```bash
go test ./apps/control-plane/internal/skill -run 'TestInstallSkillPreflight' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit preflight domain**

Run:

```bash
git add apps/control-plane/internal/skill/types.go \
  apps/control-plane/internal/skill/install_service.go \
  apps/control-plane/internal/skill/install_service_test.go \
  apps/control-plane/internal/skill/service.go
git commit -m "feat: add skill install preflight service"
```

Expected: commit includes install domain and preflight tests.

## Task 3: Control Plane Install Dispatch, Wait, And Persistence

**Files:**
- Modify: `apps/control-plane/internal/skill/install_service.go`
- Modify: `apps/control-plane/internal/skill/install_service_test.go`
- Modify: `apps/control-plane/internal/skill/pg_repository.go`
- Modify: `apps/control-plane/internal/skill/types.go`

- [ ] **Step 1: Add failing success and runtime-failure tests**

Append to `apps/control-plane/internal/skill/install_service_test.go`:

```go
func TestInstallSkillEmployeeSuccessDispatchesAndPersistsAfterRuntimeCompletion(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	runtimeNodeID := uuid.New()
	repo := newInstallServiceRepoFixture(tenantID, skillID)
	repo.targets = []SkillInstallTarget{{
		TenantID: tenantID, TeamID: uuid.New(), DigitalEmployeeID: employeeID,
		RuntimeNodeID: runtimeNodeID, NodeID: "node-1", ProviderType: "codex",
		AgentHomeDir: "/tmp/employee",
	}}
	dispatcher := &installServiceDispatcher{connected: map[string]bool{"node-1": true}}
	service := NewInstallService(repo, dispatcher, InstallServiceOptions{Timeout: 50 * time.Millisecond, PollInterval: time.Millisecond})

	result, err := service.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID: tenantID, SkillID: skillID, TargetScope: SkillInstallTargetEmployee,
		DigitalEmployeeID: employeeID, ActorUserID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("install skill: %v", err)
	}
	if len(dispatcher.commands) != 1 {
		t.Fatalf("expected one runtime command, got %#v", dispatcher.commands)
	}
	if dispatcher.commands[0].Type != "install_skills" {
		t.Fatalf("command type = %q, want install_skills", dispatcher.commands[0].Type)
	}
	if result.InstalledCount != 1 || len(repo.installations) != 1 {
		t.Fatalf("expected one installation, result=%#v repo=%#v", result, repo.installations)
	}
	if repo.installations[0].InstalledPath != "/tmp/employee/.agents/skills/review" {
		t.Fatalf("installed path = %q", repo.installations[0].InstalledPath)
	}
	if len(repo.employeeBindings) != 1 || len(repo.teamBindings) != 0 {
		t.Fatalf("expected employee binding only, team=%#v employee=%#v", repo.teamBindings, repo.employeeBindings)
	}
}

func TestInstallSkillRuntimeFailureDoesNotPersistSuccess(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	repo := newInstallServiceRepoFixture(tenantID, skillID)
	repo.targets = []SkillInstallTarget{{
		TenantID: tenantID, TeamID: uuid.New(), DigitalEmployeeID: employeeID,
		RuntimeNodeID: uuid.New(), NodeID: "node-1", ProviderType: "claude-code",
		AgentHomeDir: "/tmp/employee",
	}}
	repo.waitHook = func(commandID string) {
		repo.receipts[commandID].Status = "failed"
		repo.receipts[commandID].ErrorMessage = "checksum mismatch"
	}
	dispatcher := &installServiceDispatcher{connected: map[string]bool{"node-1": true}}
	service := NewInstallService(repo, dispatcher, InstallServiceOptions{Timeout: 50 * time.Millisecond, PollInterval: time.Millisecond})

	_, err := service.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID: tenantID, SkillID: skillID, TargetScope: SkillInstallTargetEmployee,
		DigitalEmployeeID: employeeID, ActorUserID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected runtime failure")
	}
	if len(repo.installations) != 0 || len(repo.employeeBindings) != 0 {
		t.Fatalf("runtime failure must not persist success, repo=%#v", repo)
	}
	if got := repo.failureLogs[len(repo.failureLogs)-1].Phase; got != InstallFailurePhaseRuntimeInstall {
		t.Fatalf("failure phase = %q, want runtime_install", got)
	}
}

func TestInstallSkillTeamPartialNodeFailureDoesNotPersistAndSurfacesAppliedNodes(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	teamID := uuid.New()
	repo := newInstallServiceRepoFixture(tenantID, skillID)
	repo.targets = []SkillInstallTarget{
		{TenantID: tenantID, TeamID: teamID, DigitalEmployeeID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-1", ProviderType: "codex", AgentHomeDir: "/tmp/e1"},
		{TenantID: tenantID, TeamID: teamID, DigitalEmployeeID: uuid.New(), RuntimeNodeID: uuid.New(), NodeID: "node-2", ProviderType: "codex", AgentHomeDir: "/tmp/e2"},
	}
	// node-1 is dispatched/waited first (deterministic sorted node order), node-2 fails.
	repo.waitHook = func(commandID string) {
		if repo.receipts[commandID].NodeID == "node-2" {
			repo.receipts[commandID].Status = "failed"
			repo.receipts[commandID].ErrorMessage = "extract failed"
		}
	}
	dispatcher := &installServiceDispatcher{connected: map[string]bool{"node-1": true, "node-2": true}}
	service := NewInstallService(repo, dispatcher, InstallServiceOptions{Timeout: 50 * time.Millisecond, PollInterval: time.Millisecond})

	_, err := service.InstallSkill(context.Background(), InstallSkillRequest{
		TenantID: tenantID, SkillID: skillID, TargetScope: SkillInstallTargetTeam, TeamID: teamID, ActorUserID: uuid.New(),
	})
	if err == nil {
		t.Fatal("expected partial node failure error")
	}
	if len(repo.installations) != 0 || len(repo.teamBindings) != 0 || len(repo.employeeBindings) != 0 {
		t.Fatalf("partial node failure must not persist success: repo=%#v", repo)
	}
	last := repo.failureLogs[len(repo.failureLogs)-1]
	if last.Phase != InstallFailurePhaseRuntimeInstall {
		t.Fatalf("failure phase = %q, want runtime_install", last.Phase)
	}
	applied, _ := last.Details["applied_nodes"].([]string)
	if len(applied) != 1 || applied[0] != "node-1" {
		t.Fatalf("expected node-1 surfaced as already-applied for manual cleanup, got %#v", last.Details["applied_nodes"])
	}
}
```

Add this field and hook in the test fake:

```go
waitHook func(commandID string)
```

and update `WaitForInstallCommand`:

```go
if r.waitHook != nil {
	r.waitHook(commandID)
}
```

- [ ] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/skill -run 'TestInstallSkill(EmployeeSuccess|RuntimeFailure)' -count=1
```

Expected: FAIL because dispatch/wait persistence still returns "not implemented".

- [ ] **Step 3: Implement grouped dispatch and successful result mapping**

Replace the final `return result, fmt.Errorf...` block in `InstallSkill` with:

```go
commands, err := s.dispatchInstallCommands(ctx, req, skill, targets)
if err != nil {
	_ = s.recordFailure(ctx, req, InstallFailurePhaseRuntimeInstall, "runtime_dispatch_failed", err.Error(), "", nil)
	return result, err
}
installations := make([]SkillInstallation, 0, len(targets))
appliedNodes := make([]string, 0, len(commands))
for _, command := range commands {
	waitCtx, cancel := context.WithTimeout(ctx, timeoutOrDefault(req.Timeout, s.timeout))
	receipt, waitErr := s.repository.WaitForInstallCommand(waitCtx, req.TenantID, command.CommandID, s.pollInterval)
	cancel()
	if waitErr != nil {
		s.recordPartialFailure(ctx, req, InstallFailurePhaseTimeout, "runtime_install_timeout", waitErr.Error(), command.CommandID, appliedNodes)
		return result, &InstallSkillError{Phase: InstallFailurePhaseTimeout, Message: waitErr.Error()}
	}
	if receipt.Status != "completed" {
		message := receipt.ErrorMessage
		if strings.TrimSpace(message) == "" {
			message = "runtime install failed"
		}
		s.recordPartialFailure(ctx, req, InstallFailurePhaseRuntimeInstall, "runtime_install_failed", message, command.CommandID, appliedNodes)
		return result, &InstallSkillError{Phase: InstallFailurePhaseRuntimeInstall, Message: message}
	}
	installations = append(installations, command.Installations...)
	appliedNodes = append(appliedNodes, command.NodeID)
}
return s.repository.PersistInstallSuccess(ctx, PersistSkillInstallSuccessRequest{
	TenantID: req.TenantID, SkillID: req.SkillID, TargetScope: req.TargetScope,
	TeamID: req.TeamID, InstalledBy: req.ActorUserID, Installations: installations,
})
```

Add helper types/functions to `install_service.go`:

```go
type dispatchedInstallCommand struct {
	CommandID     string
	RuntimeNodeID uuid.UUID
	NodeID        string
	Installations []SkillInstallation
}

func (s *InstallService) dispatchInstallCommands(ctx context.Context, req InstallSkillRequest, skill *Skill, targets []SkillInstallTarget) ([]dispatchedInstallCommand, error) {
	grouped := map[string][]SkillInstallTarget{}
	nodeOrder := make([]string, 0)
	for _, target := range targets {
		if _, seen := grouped[target.NodeID]; !seen {
			nodeOrder = append(nodeOrder, target.NodeID)
		}
		grouped[target.NodeID] = append(grouped[target.NodeID], target)
	}
	sort.Strings(nodeOrder) // deterministic dispatch/wait order so partial-failure applied_nodes is stable
	commands := make([]dispatchedInstallCommand, 0, len(grouped))
	for _, nodeID := range nodeOrder {
		nodeTargets := grouped[nodeID]
		commandID := newInstallCommandID()
		payload := buildInstallCommandPayload(commandID, req.TenantID, skill, nodeTargets)
		runtimeNodeID := nodeTargets[0].RuntimeNodeID
		if err := s.repository.CreateInstallCommandReceipt(ctx, CreateSkillInstallCommandReceiptRequest{
			TenantID: req.TenantID, CommandID: commandID, CommandType: "install_skills",
			RuntimeNodeID: runtimeNodeID, NodeID: nodeID, ResourceID: req.SkillID, Payload: payload,
		}); err != nil {
			return nil, err
		}
		command, err := runtimeInstallCommand(commandID, payload)
		if err != nil {
			return nil, err
		}
		if err := s.dispatcher.Dispatch(ctx, nodeID, command); err != nil {
			return nil, err
		}
		commands = append(commands, dispatchedInstallCommand{
			CommandID: commandID, RuntimeNodeID: runtimeNodeID, NodeID: nodeID,
			Installations: expectedInstallations(req, skill, nodeTargets, commandID),
		})
	}
	return commands, nil
}

func buildInstallCommandPayload(commandID string, tenantID uuid.UUID, skill *Skill, targets []SkillInstallTarget) map[string]any {
	payloadTargets := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		payloadTargets = append(payloadTargets, map[string]any{
			"team_id": target.TeamID.String(),
			"digital_employee_id": target.DigitalEmployeeID.String(),
			"provider_type": target.ProviderType,
			"agent_home_dir": target.AgentHomeDir,
		})
	}
	return map[string]any{
		"command_id": commandID,
		"tenant_id": tenantID.String(),
		"skill": map[string]any{
			"skill_id": skill.ID.String(),
			"skill_key": skill.Slug,
			"archive_object_ref": skill.ArchiveObjectRef,
			"archive_checksum_sha256": skill.ArchiveChecksum,
			"archive_size_bytes": skill.ArchiveSizeBytes,
			"archive_file_count": skill.ArchiveFileCount,
		},
		"targets": payloadTargets,
		"rollback_on_failure": true,
	}
}

func expectedInstallations(req InstallSkillRequest, skill *Skill, targets []SkillInstallTarget, commandID string) []SkillInstallation {
	installations := make([]SkillInstallation, 0, len(targets))
	now := time.Now().UTC()
	for _, target := range targets {
		installations = append(installations, SkillInstallation{
			TenantID: req.TenantID, SkillID: skill.ID, TargetScope: req.TargetScope,
			TeamID: target.TeamID, DigitalEmployeeID: target.DigitalEmployeeID,
			EmployeeName: target.EmployeeName, RuntimeNodeID: target.RuntimeNodeID,
			NodeID: target.NodeID, ProviderType: target.ProviderType,
			InstalledPath: providerSkillPath(target.AgentHomeDir, target.ProviderType, skill.Slug),
			ArchiveChecksumSHA256: skill.ArchiveChecksum, InstalledBy: req.ActorUserID,
			InstalledAt: now, Metadata: map[string]any{"command_id": commandID},
		})
	}
	return installations
}

func providerSkillPath(agentHomeDir, providerType, skillSlug string) string {
	base := strings.TrimRight(agentHomeDir, "/")
	switch providerType {
	case "opencode":
		return base + "/.opencode/skills/" + skillSlug
	case "codex":
		return base + "/.agents/skills/" + skillSlug
	case "claude-code":
		return base + "/.claude/skills/" + skillSlug
	default:
		return base + "/skills/" + skillSlug
	}
}

func timeoutOrDefault(requested, fallback time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return fallback
}

// recordPartialFailure logs a runtime/timeout failure and, when one or more
// nodes already physically applied the skill, surfaces them for manual cleanup.
// Cross-node rollback is out of scope for this slice (see "Atomicity & failure
// semantics"); the error is intentionally not propagated so the caller still
// returns the original install failure to the client.
func (s *InstallService) recordPartialFailure(ctx context.Context, req InstallSkillRequest, phase InstallFailurePhase, reasonCode, message, commandID string, appliedNodes []string) {
	details := map[string]any{}
	if len(appliedNodes) > 0 {
		details["applied_nodes"] = appliedNodes
		details["requires_manual_cleanup"] = true
	}
	_ = s.repository.RecordInstallFailure(ctx, SkillInstallFailureLog{
		TenantID: req.TenantID, SkillID: req.SkillID, TargetScope: req.TargetScope,
		TeamID: req.TeamID, DigitalEmployeeID: req.DigitalEmployeeID,
		Phase: phase, ReasonCode: reasonCode, Message: message, CommandID: commandID, Details: details,
	})
}

func newInstallCommandID() string {
	return "cmd-" + uuid.NewString()
}
```

- [ ] **Step 4: Run service tests and fix compile gaps**

Run:

```bash
go test ./apps/control-plane/internal/skill -run 'TestInstallSkill' -count=1
```

Expected: PASS. If the fake repository needs `waitHook` initialization fixes, make only those test-fake edits.

- [ ] **Step 5: Implement PgRepository install methods**

Add methods to `apps/control-plane/internal/skill/pg_repository.go`:

```go
func (r *PgRepository) ListInstallTargets(ctx context.Context, req ListSkillInstallTargetsRequest) ([]SkillInstallTarget, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	var rows pgx.Rows
	var err error
	if req.TargetScope == SkillInstallTargetTeam {
		rows, err = r.db.Query(ctx, `
SELECT de.tenant_id, de.team_id, de.id, de.name,
       dei.runtime_node_id, rn.node_id, dei.provider_type, dei.agent_home_dir
FROM digital_employees de
JOIN digital_employee_execution_instances dei
  ON dei.tenant_id = de.tenant_id
 AND dei.digital_employee_id = de.id
 AND dei.deleted_at IS NULL
 AND dei.status IN ('ready', 'active')
JOIN runtime_nodes rn
  ON rn.tenant_id = de.tenant_id
 AND rn.id = dei.runtime_node_id
 AND rn.archived_at IS NULL
WHERE de.tenant_id = $1
  AND de.team_id = $2
  AND de.deleted_at IS NULL
ORDER BY de.name ASC`, req.TenantID, req.TeamID)
	} else {
		rows, err = r.db.Query(ctx, `
SELECT de.tenant_id, de.team_id, de.id, de.name,
       dei.runtime_node_id, rn.node_id, dei.provider_type, dei.agent_home_dir
FROM digital_employees de
JOIN digital_employee_execution_instances dei
  ON dei.tenant_id = de.tenant_id
 AND dei.digital_employee_id = de.id
 AND dei.deleted_at IS NULL
 AND dei.status IN ('ready', 'active')
JOIN runtime_nodes rn
  ON rn.tenant_id = de.tenant_id
 AND rn.id = dei.runtime_node_id
 AND rn.archived_at IS NULL
WHERE de.tenant_id = $1
  AND de.id = $2
  AND de.deleted_at IS NULL`, req.TenantID, req.DigitalEmployeeID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []SkillInstallTarget
	for rows.Next() {
		var target SkillInstallTarget
		if err := rows.Scan(&target.TenantID, &target.TeamID, &target.DigitalEmployeeID, &target.EmployeeName, &target.RuntimeNodeID, &target.NodeID, &target.ProviderType, &target.AgentHomeDir); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func (r *PgRepository) CreateInstallCommandReceipt(ctx context.Context, req CreateSkillInstallCommandReceiptRequest) error {
	if r == nil || r.q == nil {
		return fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	payload, err := json.Marshal(req.Payload)
	if err != nil {
		return err
	}
	_, err = r.q.CreateRuntimeCommandReceipt(ctx, queries.CreateRuntimeCommandReceiptParams{
		TenantID: req.TenantID, CommandID: req.CommandID, CommandType: req.CommandType,
		RuntimeNodeID: req.RuntimeNodeID, NodeID: req.NodeID, ResourceType: "skill",
		ResourceID: req.ResourceID, Status: "pending", Payload: payload,
	})
	return err
}

func (r *PgRepository) WaitForInstallCommand(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeInstallCommandReceipt, error) {
	if interval <= 0 {
		interval = defaultInstallPollInterval
	}
	for {
		row, err := r.q.GetRuntimeCommandReceiptByCommandID(ctx, queries.GetRuntimeCommandReceiptByCommandIDParams{TenantID: tenantID, CommandID: commandID})
		if err != nil {
			return nil, mapNoRows(err)
		}
			receipt := &RuntimeInstallCommandReceipt{CommandID: row.CommandID, Status: row.Status, RuntimeNodeID: row.RuntimeNodeID, NodeID: row.NodeID, ErrorMessage: textFromPg(row.ErrorMessage)}
		_ = json.Unmarshal(row.Payload, &receipt.Payload)
		_ = json.Unmarshal(row.Result, &receipt.Result)
		if isTerminalReceiptStatus(receipt.Status) {
			return receipt, nil
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
```

Add this helper in `pg_repository.go`; generated runtime command rows use `pgtype.Text` for `error_message`:

```go
func textFromPg(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
```

Ensure `pg_repository.go` imports `github.com/jackc/pgx/v5/pgtype`.

- [ ] **Step 6: Add transaction persistence**

Add to `pg_repository.go`:

```go
func (r *PgRepository) PersistInstallSuccess(ctx context.Context, req PersistSkillInstallSuccessRequest) (InstallSkillResult, error) {
	if r == nil || r.db == nil {
		return InstallSkillResult{}, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return InstallSkillResult{}, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()
	if req.TargetScope == SkillInstallTargetTeam {
		if _, err = tx.Exec(ctx, `INSERT INTO skill_team_bindings (tenant_id, skill_id, team_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, req.TenantID, req.SkillID, req.TeamID); err != nil {
			return InstallSkillResult{}, err
		}
	} else {
		for _, installation := range req.Installations {
			if _, err = tx.Exec(ctx, `INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status) VALUES ($1,$2,$3,'enabled') ON CONFLICT (tenant_id, skill_id, digital_employee_id) DO UPDATE SET status='enabled', updated_at=NOW()`, req.TenantID, req.SkillID, installation.DigitalEmployeeID); err != nil {
				return InstallSkillResult{}, err
			}
		}
	}
	for _, item := range req.Installations {
		metadata, marshalErr := json.Marshal(item.Metadata)
		if marshalErr != nil {
			err = marshalErr
			return InstallSkillResult{}, err
		}
		_, err = tx.Exec(ctx, `
INSERT INTO skill_installations (
    tenant_id, skill_id, target_scope, team_id, digital_employee_id, runtime_node_id,
    provider_type, installed_path, archive_checksum_sha256, status, installed_by,
    installed_at, metadata, updated_at, deleted_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'installed',$10,$11,$12,NOW(),NULL)
ON CONFLICT (tenant_id, skill_id, digital_employee_id) WHERE deleted_at IS NULL
DO UPDATE SET
    target_scope = EXCLUDED.target_scope,
    team_id = EXCLUDED.team_id,
    runtime_node_id = EXCLUDED.runtime_node_id,
    provider_type = EXCLUDED.provider_type,
    installed_path = EXCLUDED.installed_path,
    archive_checksum_sha256 = EXCLUDED.archive_checksum_sha256,
    status = 'installed',
    installed_by = EXCLUDED.installed_by,
    installed_at = EXCLUDED.installed_at,
    metadata = EXCLUDED.metadata,
    updated_at = NOW(),
    deleted_at = NULL`,
			req.TenantID, req.SkillID, req.TargetScope, nullUUID(item.TeamID), item.DigitalEmployeeID,
			item.RuntimeNodeID, item.ProviderType, item.InstalledPath, item.ArchiveChecksumSHA256,
			nullUUID(req.InstalledBy), item.InstalledAt, metadata,
		)
		if err != nil {
			return InstallSkillResult{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return InstallSkillResult{}, err
	}
	err = nil
	return InstallSkillResult{SkillID: req.SkillID, TargetScope: req.TargetScope, TeamID: req.TeamID, InstalledCount: len(req.Installations), Installations: req.Installations}, nil
}

func (r *PgRepository) RecordInstallFailure(ctx context.Context, req SkillInstallFailureLog) error {
	if r == nil || r.db == nil {
		return fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	payload, err := json.Marshal(map[string]any{
		"skill_id": req.SkillID,
		"target_scope": req.TargetScope,
		"team_id": req.TeamID,
		"digital_employee_id": req.DigitalEmployeeID,
		"runtime_node_id": req.RuntimeNodeID,
		"provider_type": req.ProviderType,
		"phase": req.Phase,
		"reason_code": req.ReasonCode,
		"message": req.Message,
		"command_id": req.CommandID,
		"details": req.Details,
	})
	if err != nil {
		return err
	}
		_, err = r.db.Exec(ctx, `
INSERT INTO audit_events (tenant_id, event_type, actor_type, actor_id, action, resource_type, resource_id, details, created_at)
VALUES ($1, 'skill.install.failed', 'system', 'skill-install-service', 'skill.install.failed', 'skill', $2, $3, NOW())`,
		req.TenantID, req.SkillID.String(), payload,
	)
	return err
}
```

- [ ] **Step 7: Run focused skill tests**

Run:

```bash
go test ./apps/control-plane/internal/skill -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit dispatch and persistence**

Run:

```bash
git add apps/control-plane/internal/skill/install_service.go \
  apps/control-plane/internal/skill/install_service_test.go \
  apps/control-plane/internal/skill/pg_repository.go \
  apps/control-plane/internal/skill/types.go
git commit -m "feat: persist synchronous skill installs"
```

Expected: commit includes install service dispatch/wait and repository persistence.

## Task 4: Control Plane HTTP API And OpenAPI Contract

**Files:**
- Modify: `apps/control-plane/internal/skill/handler.go`
- Create: `apps/control-plane/internal/skill/install_handler_test.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/api/skill_routes_test.go`
- Modify: `apps/control-plane/internal/app/app.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Generated: `apps/control-plane/internal/api/gen/control_plane.gen.go`

- [ ] **Step 1: Add failing handler tests**

Create `apps/control-plane/internal/skill/install_handler_test.go`:

```go
package skill

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
)

func TestInstallSkillHandlerParsesEmployeeTarget(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	skillID := uuid.New()
	employeeID := uuid.New()
	service := &installHandlerService{result: InstallSkillResult{SkillID: skillID, TargetScope: SkillInstallTargetEmployee, DigitalEmployeeID: employeeID, InstalledCount: 1}}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills/"+skillID.String()+"/install", strings.NewReader(`{"target_scope":"employee","digital_employee_id":"`+employeeID.String()+`","timeout_sec":15}`))
	req = req.WithContext(middleware.WithUserID(middleware.WithTenantID(req.Context(), tenantID), userID))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("skillId", skillID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	resp := httptest.NewRecorder()

	handler.InstallSkill(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if service.installReq.SkillID != skillID || service.installReq.DigitalEmployeeID != employeeID || service.installReq.ActorUserID != userID {
		t.Fatalf("unexpected install request: %#v", service.installReq)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["installed_count"].(float64) != 1 {
		t.Fatalf("installed_count = %#v", body["installed_count"])
	}
}

func TestInstallSkillHandlerReturnsStructuredFailure(t *testing.T) {
	tenantID := uuid.New()
	skillID := uuid.New()
	service := &installHandlerService{err: &InstallSkillError{
		Phase: InstallFailurePhasePreflight,
		Message: "skill install preflight failed",
		BlockedTargets: []SkillInstallBlockedTarget{{ReasonCode: "runtime_not_connected", Message: "Runtime is not connected"}},
	}}
	handler := NewHandler(service)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/skills/"+skillID.String()+"/install", strings.NewReader(`{"target_scope":"team","team_id":"`+uuid.NewString()+`"}`))
	req = req.WithContext(middleware.WithTenantID(req.Context(), tenantID))
	routeCtx := chi.NewRouteContext()
	routeCtx.URLParams.Add("skillId", skillID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeCtx))
	resp := httptest.NewRecorder()

	handler.InstallSkill(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), "runtime_not_connected") {
		t.Fatalf("expected structured blocker in response: %s", resp.Body.String())
	}
}

type installHandlerService struct {
	HandlerService
	installReq InstallSkillRequest
	result     InstallSkillResult
	err        error
}

func (s *installHandlerService) InstallSkill(_ context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	s.installReq = req
	return s.result, s.err
}
```

- [ ] **Step 2: Run handler tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/skill -run 'TestInstallSkillHandler' -count=1
```

Expected: FAIL because `HandlerService` lacks `InstallSkill` and handler method does not exist.

- [ ] **Step 3: Add handler method and response structs**

Modify `apps/control-plane/internal/skill/handler.go`:

Add `InstallSkill` to `HandlerService`:

```go
InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error)
```

Add request/response structs:

```go
type installSkillRequestBody struct {
	TargetScope       string `json:"target_scope"`
	TeamID            string `json:"team_id"`
	DigitalEmployeeID string `json:"digital_employee_id"`
	TimeoutSec        int    `json:"timeout_sec"`
}

type installSkillResponse struct {
	SkillID           string                         `json:"skill_id"`
	TargetScope       string                         `json:"target_scope"`
	TeamID            string                         `json:"team_id,omitempty"`
	DigitalEmployeeID string                         `json:"digital_employee_id,omitempty"`
	InstalledCount    int                            `json:"installed_count"`
	Installations     []skillInstallationResponse    `json:"installations"`
	BlockedTargets    []SkillInstallBlockedTarget    `json:"blocked_targets,omitempty"`
}

type skillInstallationResponse struct {
	DigitalEmployeeID     string `json:"digital_employee_id"`
	EmployeeName          string `json:"employee_name,omitempty"`
	ProviderType          string `json:"provider_type"`
	RuntimeNodeID         string `json:"runtime_node_id"`
	NodeID                string `json:"node_id,omitempty"`
	InstalledPath         string `json:"installed_path"`
	ArchiveChecksumSHA256 string `json:"archive_checksum_sha256"`
	InstalledAt           string `json:"installed_at,omitempty"`
}

type installSkillErrorResponse struct {
	Error          string                      `json:"error"`
	Phase          string                      `json:"phase"`
	Message        string                      `json:"message"`
	BlockedTargets []SkillInstallBlockedTarget `json:"blocked_targets,omitempty"`
}
```

**Authorization decision required before writing this handler.** The snippet below calls `authorizeSkillAction(..., authz.ActionTeamCapabilityBind, authz.ResourceRef{Type: authz.ResourceSkill, ...})`. But `team.capability.bind` is modeled against a **Team** resource (`authorizer_test.go` shows it rejects non-team resources), and there is no `skill.install` action today. For employee-scope installs the request body carries no `team_id` at all, so it is undefined which team the check runs against. Resolve one of:
- add a dedicated `skill.install` action and authorize against the skill, or
- resolve the target team(s) from the install targets first and authorize `team.capability.bind` against each resolved Team resource.

Confirm the chosen model against `apps/control-plane/internal/authz/` and add an authz-coverage case in `skill_routes_test.go` (allow + deny) before locking in the handler. Do not ship the `ActionTeamCapabilityBind` + `ResourceSkill` pairing as-is.

Add handler:

```go
func (h *HTTPHandler) InstallSkill(w http.ResponseWriter, r *http.Request) {
	skillID, ok := skillIDFromRequest(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.authorizeSkillAction(w, r, authz.ActionTeamCapabilityBind, authz.ResourceRef{Type: authz.ResourceSkill, ID: skillID.String()}, "skill install")
	if !ok {
		return
	}
	var body installSkillRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	req := InstallSkillRequest{TenantID: tenantID, SkillID: skillID, TargetScope: SkillInstallTargetScope(body.TargetScope), ActorUserID: middleware.GetUserID(r.Context())}
	if body.TimeoutSec > 0 {
		req.Timeout = time.Duration(body.TimeoutSec) * time.Second
	}
	if body.TeamID != "" {
		parsed, err := uuid.Parse(body.TeamID)
		if err != nil {
			http.Error(w, "team_id must be a UUID", http.StatusBadRequest)
			return
		}
		req.TeamID = parsed
	}
	if body.DigitalEmployeeID != "" {
		parsed, err := uuid.Parse(body.DigitalEmployeeID)
		if err != nil {
			http.Error(w, "digital_employee_id must be a UUID", http.StatusBadRequest)
			return
		}
		req.DigitalEmployeeID = parsed
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	result, err := service.InstallSkill(r.Context(), req)
	if err != nil {
		var installErr *InstallSkillError
		if errors.As(err, &installErr) {
			writeJSON(w, http.StatusConflict, installSkillErrorResponse{
				Error: "skill_install_failed", Phase: string(installErr.Phase),
				Message: installErr.Message, BlockedTargets: installErr.BlockedTargets,
			})
			return
		}
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, installResponseFromDomain(result))
}
```

Add response mapper:

```go
func installResponseFromDomain(result InstallSkillResult) installSkillResponse {
	out := installSkillResponse{
		SkillID: result.SkillID.String(), TargetScope: string(result.TargetScope),
		InstalledCount: result.InstalledCount, BlockedTargets: result.BlockedTargets,
	}
	if result.TeamID != uuid.Nil {
		out.TeamID = result.TeamID.String()
	}
	if result.DigitalEmployeeID != uuid.Nil {
		out.DigitalEmployeeID = result.DigitalEmployeeID.String()
	}
	out.Installations = make([]skillInstallationResponse, 0, len(result.Installations))
	for _, item := range result.Installations {
		out.Installations = append(out.Installations, skillInstallationResponse{
			DigitalEmployeeID: item.DigitalEmployeeID.String(),
			EmployeeName: item.EmployeeName,
			ProviderType: item.ProviderType,
			RuntimeNodeID: item.RuntimeNodeID.String(),
			NodeID: item.NodeID,
			InstalledPath: item.InstalledPath,
			ArchiveChecksumSHA256: item.ArchiveChecksumSHA256,
			InstalledAt: item.InstalledAt.Format(time.RFC3339),
		})
	}
	return out
}
```

Ensure imports include `errors`, `time`, and `github.com/google/uuid` if not already present.

- [ ] **Step 4: Register route**

Modify `apps/control-plane/internal/api/server.go` inside the skill routes:

```go
r.Post("/skills/{skillId}/install", s.skillHandler.InstallSkill)
```

- [ ] **Step 5: Run handler and route tests**

Run:

```bash
go test ./apps/control-plane/internal/skill -run 'TestInstallSkillHandler' -count=1
go test ./apps/control-plane/internal/api -run TestSkillRoutes -count=1
```

Expected: handler tests PASS. API route tests may fail until fake route service implements `InstallSkill`; add this method to the route fake:

```go
func (s *routeSkillService) InstallSkill(_ context.Context, req skill.InstallSkillRequest) (skill.InstallSkillResult, error) {
	s.installReq = req
	return skill.InstallSkillResult{SkillID: req.SkillID, TargetScope: req.TargetScope, TeamID: req.TeamID, DigitalEmployeeID: req.DigitalEmployeeID}, nil
}
```

- [ ] **Step 6: Wire install service in app container**

Modify `apps/control-plane/internal/app/app.go` where `skillService` is created:

```go
skillInstallService := skill.NewInstallService(skillRepository, runtimeCommands, skill.InstallServiceOptions{})
skillService.SetInstallService(skillInstallService)
```

Add a small wrapper in `apps/control-plane/internal/skill/service.go`:

```go
type Installer interface {
	InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error)
}

func (s *Service) SetInstallService(installer Installer) {
	s.installer = installer
}

func (s *Service) InstallSkill(ctx context.Context, req InstallSkillRequest) (InstallSkillResult, error) {
	if s == nil || s.installer == nil {
		return InstallSkillResult{}, fmt.Errorf("%w: skill install service is not configured", ErrInvalidInput)
	}
	return s.installer.InstallSkill(ctx, req)
}
```

Add `installer Installer` to `Service`.

- [ ] **Step 7: Update OpenAPI contract**

Modify `contracts/control-plane/openapi.yaml` to include:

```yaml
  /api/v1/skills/{skillId}/install:
    post:
      summary: Install a skill to a team or digital employee
      parameters:
        - name: skillId
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
              $ref: "#/components/schemas/InstallSkillRequest"
      responses:
        "201":
          description: Skill installed
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/InstallSkillResponse"
        "409":
          description: Skill install failed
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/InstallSkillErrorResponse"
```

Add schemas:

```yaml
    InstallSkillRequest:
      type: object
      required: [target_scope]
      properties:
        target_scope:
          type: string
          enum: [team, employee]
        team_id:
          type: string
          format: uuid
        digital_employee_id:
          type: string
          format: uuid
        timeout_sec:
          type: integer
          minimum: 1
          maximum: 60
    InstallSkillResponse:
      type: object
      required: [skill_id, target_scope, installed_count, installations]
      properties:
        skill_id:
          type: string
          format: uuid
        target_scope:
          type: string
        team_id:
          type: string
          format: uuid
        digital_employee_id:
          type: string
          format: uuid
        installed_count:
          type: integer
        installations:
          type: array
          items:
            $ref: "#/components/schemas/SkillInstallation"
    SkillInstallation:
      type: object
      required: [digital_employee_id, provider_type, runtime_node_id, installed_path, archive_checksum_sha256]
      properties:
        digital_employee_id:
          type: string
          format: uuid
        employee_name:
          type: string
        provider_type:
          type: string
          enum: [opencode, codex, claude-code]
        runtime_node_id:
          type: string
          format: uuid
        node_id:
          type: string
        installed_path:
          type: string
        archive_checksum_sha256:
          type: string
        installed_at:
          type: string
          format: date-time
    InstallSkillErrorResponse:
      type: object
      required: [error, phase, message]
      properties:
        error:
          type: string
        phase:
          type: string
          enum: [preflight, runtime_install, timeout]
        message:
          type: string
        blocked_targets:
          type: array
          items:
            $ref: "#/components/schemas/SkillInstallBlockedTarget"
    SkillInstallBlockedTarget:
      type: object
      required: [reason_code, message]
      properties:
        digital_employee_id:
          type: string
          format: uuid
        employee_name:
          type: string
        provider_type:
          type: string
        runtime_node_id:
          type: string
          format: uuid
        node_id:
          type: string
        reason_code:
          type: string
        message:
          type: string
```

- [ ] **Step 8: Regenerate and verify contract**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
```

Expected: generated outputs are updated and contract verification passes.

- [ ] **Step 9: Run focused Control Plane tests**

Run:

```bash
go test ./apps/control-plane/internal/skill ./apps/control-plane/internal/api ./apps/control-plane/internal/app -count=1
```

Expected: PASS.

- [ ] **Step 10: Commit API layer**

Run:

```bash
git add apps/control-plane/internal/skill/handler.go \
  apps/control-plane/internal/skill/install_handler_test.go \
  apps/control-plane/internal/skill/service.go \
  apps/control-plane/internal/api/server.go \
  apps/control-plane/internal/api/skill_routes_test.go \
  apps/control-plane/internal/app/app.go \
  contracts/control-plane/openapi.yaml \
  apps/control-plane/internal/api/gen/control_plane.gen.go
git commit -m "feat: expose skill install API"
```

Expected: commit includes API, service wiring, contract, and generated outputs.

## Task 5: Runtime Agent Install Command

**Files:**
- Create: `apps/runtime-agent/src/commands/install_skills.rs`
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/src/skills.rs`
- Create: `apps/runtime-agent/tests/install_skills_test.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_payload_test.rs`

- [ ] **Step 0: Verify the codex skill directory against the live provider adapter**

The design spec maps `codex` → `.agents/skills/<slug>/` (citing `developers.openai.com/codex/skills`), but this repo's own codex assets live under `.codex/skills/` (e.g. `.codex/skills/superteam-completion-check`). Installing to a directory the codex provider does not actually read is a silent, hard-to-debug failure. The design spec states a codex provider adapter already exists, so it — not the spec table — is the source of truth.

Before writing any path mapping, confirm where the existing codex adapter loads skills from:

```bash
rg -n "\.codex/skills|\.agents/skills|skills" apps/runtime-agent/src --glob '*codex*'
rg -rn "skill" apps/runtime-agent/src/providers 2>/dev/null | rg -i "dir|path|home"
```

Expected: identify the directory the running codex provider reads. If it is `.codex/skills`, change `codex` to `.codex/skills` in **all five** places that hardcode it: the Go service test (`/tmp/employee/.agents/skills/review`), `providerSkillPath` in `install_service.go`, `provider_skill_dir` in `install_skills.rs`, the Rust mapping test, and the Step 9 E2E expectation. Do not proceed on the spec table alone; if the adapter location cannot be confirmed from code, treat codex as blocked and do a real codex provider smoke (per project rules) before baking the path in.

- [ ] **Step 1: Add failing provider directory mapping tests**

Create `apps/runtime-agent/tests/install_skills_test.rs`:

```rust
use std::path::PathBuf;

use superteam_runtime_agent::commands::install_skills::provider_skill_dir;

#[test]
fn provider_skill_dir_maps_supported_providers_to_official_project_dirs() {
    let home = PathBuf::from("/runtime/employee");
    assert_eq!(
        provider_skill_dir(&home, "opencode", "review").unwrap(),
        PathBuf::from("/runtime/employee/.opencode/skills/review")
    );
    assert_eq!(
        provider_skill_dir(&home, "codex", "review").unwrap(),
        PathBuf::from("/runtime/employee/.agents/skills/review")
    );
    assert_eq!(
        provider_skill_dir(&home, "claude-code", "review").unwrap(),
        PathBuf::from("/runtime/employee/.claude/skills/review")
    );
}

#[test]
fn provider_skill_dir_rejects_unsupported_provider_and_unsafe_key() {
    let home = PathBuf::from("/runtime/employee");
    assert!(provider_skill_dir(&home, "other", "review").is_err());
    assert!(provider_skill_dir(&home, "codex", "../review").is_err());
    assert!(provider_skill_dir(&home, "codex", "bad/key").is_err());
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml provider_skill_dir --test install_skills_test
```

Expected: FAIL because `commands::install_skills` does not exist.

- [ ] **Step 3: Add install command module**

Create `apps/runtime-agent/src/commands/install_skills.rs`:

```rust
use std::path::{Path, PathBuf};

use anyhow::Result;

pub fn provider_skill_dir(agent_home_dir: &Path, provider_type: &str, skill_key: &str) -> Result<PathBuf> {
    validate_skill_key(skill_key)?;
    let provider_root = match provider_type {
        "opencode" => ".opencode/skills",
        "codex" => ".agents/skills",
        "claude-code" => ".claude/skills",
        other => anyhow::bail!("unsupported provider for skill install: {other}"),
    };
    Ok(agent_home_dir.join(provider_root).join(skill_key))
}

pub fn validate_skill_key(key: &str) -> Result<()> {
    if key.is_empty() {
        anyhow::bail!("skill_key must not be empty");
    }
    if key.contains('/') || key.contains('\\') || key.contains('\0') {
        anyhow::bail!("skill_key must not contain path separators: {key}");
    }
    if key == "." || key == ".." {
        anyhow::bail!("skill_key must not be a path traversal: {key}");
    }
    Ok(())
}
```

Expose it from `apps/runtime-agent/src/commands/mod.rs` or the existing commands module root:

```rust
pub mod install_skills;
```

- [ ] **Step 4: Run mapping tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml provider_skill_dir --test install_skills_test
```

Expected: PASS.

- [ ] **Step 5: Add failing command parsing test**

Append to `apps/runtime-agent/tests/runtime_command_payload_test.rs`:

```rust
#[test]
fn parses_install_skills_command_type() {
    let value = serde_json::json!({
        "id": "cmd-1",
        "type": "install_skills",
        "payload": {
            "command_id": "cmd-1",
            "tenant_id": "11111111-1111-4111-8111-111111111111",
            "skill": {
                "skill_id": "22222222-2222-4222-8222-222222222222",
                "skill_key": "review",
                "archive_object_ref": "s3://bucket/skills/review.zip",
                "archive_checksum_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
                "archive_size_bytes": 100,
                "archive_file_count": 1
            },
            "targets": [{
                "team_id": "33333333-3333-4333-8333-333333333333",
                "digital_employee_id": "44444444-4444-4444-8444-444444444444",
                "provider_type": "codex",
                "agent_home_dir": "/tmp/employee"
            }],
            "rollback_on_failure": true
        }
    });
    let command: superteam_runtime_agent::controlplane::models::RuntimeCommand = serde_json::from_value(value).unwrap();
    assert!(matches!(command.command_type, superteam_runtime_agent::controlplane::models::RuntimeCommandType::InstallSkills));
}
```

- [ ] **Step 6: Implement command type**

Modify `apps/runtime-agent/src/controlplane/models.rs`:

```rust
InstallSkills,
```

and in the deserialize match:

```rust
"install_skills" => Self::InstallSkills,
```

- [ ] **Step 7: Run command parsing test**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml parses_install_skills_command_type --test runtime_command_payload_test
```

Expected: PASS.

- [ ] **Step 8: Add install payload and orchestration**

Extend `apps/runtime-agent/src/commands/install_skills.rs`:

```rust
use serde::{Deserialize, Serialize};
use crate::commands::payload::RuntimeSkillPayload;

#[derive(Debug, Clone, Deserialize)]
pub struct InstallSkillsCommandPayload {
    pub command_id: String,
    pub tenant_id: String,
    pub skill: RuntimeSkillPayload,
    #[serde(default)]
    pub targets: Vec<InstallSkillTargetPayload>,
    #[serde(default)]
    pub rollback_on_failure: bool,
}

#[derive(Debug, Clone, Deserialize)]
pub struct InstallSkillTargetPayload {
    pub team_id: String,
    pub digital_employee_id: String,
    pub provider_type: String,
    pub agent_home_dir: String,
}

#[derive(Debug, Clone, Serialize, PartialEq, Eq)]
pub struct InstalledSkillTarget {
    pub digital_employee_id: String,
    pub provider_type: String,
    pub installed_path: String,
    pub archive_checksum_sha256: String,
    pub archive_file_count: i64,
}
```

Refactor `apps/runtime-agent/src/skills.rs` so the existing archive download/extract logic can accept an explicit target directory. Replace the body of `materialize_skills` with the wrapper below and add `materialize_skill_to_dir` after it:

```rust
pub async fn materialize_skills(
    agent_home_dir: &Path,
    skills: &[RuntimeSkillPayload],
    s3_client: &S3Client,
    bucket: &str,
) -> Result<Vec<SyncedSkill>> {
    let mut synced = Vec::with_capacity(skills.len());

    for skill in skills {
        let target_dir = agent_home_dir.join("skills").join(&skill.skill_key);
        synced.push(materialize_skill_to_dir(&target_dir, agent_home_dir, skill, s3_client, bucket).await?);
    }

    Ok(synced)
}

pub async fn materialize_skill_to_dir(
    target_dir: &Path,
    temp_root: &Path,
    skill: &RuntimeSkillPayload,
    s3_client: &S3Client,
    bucket: &str,
) -> Result<SyncedSkill> {
    validate_skill_key(&skill.skill_key)?;
    ensure_real_directory(target_dir)?;

    let marker_path = target_dir.join(".skill-checksum");
    if let Ok(existing_hash) = fs::read_to_string(&marker_path) {
        if existing_hash == skill.archive_checksum_sha256 {
            return Ok(SyncedSkill {
                skill_id: skill.skill_id.clone(),
                skill_key: skill.skill_key.clone(),
                content_hash: skill.archive_checksum_sha256.clone(),
            });
        }
    }

    let object_key = extract_object_key(&skill.archive_object_ref)?;

    let response = s3_client
        .get_object()
        .bucket(bucket)
        .key(&object_key)
        .send()
        .await
        .with_context(|| {
            format!("failed to fetch skill archive from s3: {bucket}/{object_key}")
        })?;

    let body = response
        .body
        .collect()
        .await
        .map_err(|e| anyhow::anyhow!("read s3 body: {e}"))?;
    let archive_bytes = body.into_bytes();

    if archive_bytes.len() as u64 > MAX_ARCHIVE_SIZE {
        anyhow::bail!(
            "skill archive exceeds size limit: {} > {} bytes",
            archive_bytes.len(),
            MAX_ARCHIVE_SIZE
        );
    }

    let computed_hash = sha256_hex(&archive_bytes);
    if !computed_hash.eq_ignore_ascii_case(&skill.archive_checksum_sha256) {
        anyhow::bail!(
            "skill archive checksum mismatch for {}: expected {}, got {computed_hash}",
            skill.skill_key,
            skill.archive_checksum_sha256
        );
    }

    let temp_dir = temp_root.join(".skill-tmp").join(format!(
        "{}-{}",
        std::process::id(),
        skill.skill_key
    ));
    if temp_dir.exists() {
        fs::remove_dir_all(&temp_dir)?;
    }
    fs::create_dir_all(&temp_dir)?;

    let cursor = Cursor::new(&archive_bytes);
    let mut archive = ZipArchive::new(cursor)
        .with_context(|| format!("invalid zip archive for skill {}", skill.skill_key))?;

    if archive.len() > MAX_FILE_COUNT {
        anyhow::bail!(
            "skill archive exceeds file count limit: {} > {}",
            archive.len(),
            MAX_FILE_COUNT
        );
    }

    let entry_names: Vec<String> = (0..archive.len())
        .filter_map(|i| {
            archive
                .by_index(i)
                .ok()
                .map(|entry| entry.name().to_string())
        })
        .collect();
    let root_prefix = common_root_prefix(&entry_names);

    let mut file_count = 0u64;
    for i in 0..archive.len() {
        let mut entry = archive.by_index(i)?;
        let entry_name = entry.name().to_string();
        if entry.is_dir() {
            continue;
        }

        let relative = normalize_zip_path(&entry_name, &root_prefix)?;
        let target = temp_dir.join(&relative);
        if let Some(parent) = target.parent() {
            fs::create_dir_all(parent)?;
        }

        let mut buf = Vec::new();
        entry.read_to_end(&mut buf)?;
        atomic_write(&target, &buf)?;
        file_count += 1;
    }

    if file_count == 0 {
        fs::remove_dir_all(&temp_dir)?;
        anyhow::bail!("skill archive contains no files: {}", skill.skill_key);
    }

    if target_dir.exists() {
        fs::remove_dir_all(target_dir)?;
    }
    fs::rename(&temp_dir, target_dir)?;
    fs::write(&marker_path, &skill.archive_checksum_sha256)?;

    Ok(SyncedSkill {
        skill_id: skill.skill_id.clone(),
        skill_key: skill.skill_key.clone(),
        content_hash: skill.archive_checksum_sha256.clone(),
    })
}
```

- [ ] **Step 9: Implement Runtime handler**

In `install_skills.rs`, add:

```rust
pub async fn install_skill_targets(
    payload: &InstallSkillsCommandPayload,
    s3_client: &aws_sdk_s3::Client,
    bucket: &str,
) -> Result<Vec<InstalledSkillTarget>> {
    let mut installed = Vec::new();
    let mut changed_dirs: Vec<PathBuf> = Vec::new();
    for target in &payload.targets {
        let agent_home = PathBuf::from(&target.agent_home_dir);
        let target_dir = provider_skill_dir(&agent_home, &target.provider_type, &payload.skill.skill_key)?;
        match crate::skills::materialize_skill_to_dir(&target_dir, &agent_home, &payload.skill, s3_client, bucket).await {
            Ok(_) => {
                changed_dirs.push(target_dir.clone());
                installed.push(InstalledSkillTarget {
                    digital_employee_id: target.digital_employee_id.clone(),
                    provider_type: target.provider_type.clone(),
                    installed_path: target_dir.to_string_lossy().to_string(),
                    archive_checksum_sha256: payload.skill.archive_checksum_sha256.clone(),
                    archive_file_count: payload.skill.archive_file_count,
                });
            }
            Err(error) => {
                if payload.rollback_on_failure {
                    for dir in changed_dirs.iter().rev() {
                        let _ = std::fs::remove_dir_all(dir);
                    }
                }
                return Err(error);
            }
        }
    }
    Ok(installed)
}
```

- [ ] **Step 10: Route executor command**

Modify `apps/runtime-agent/src/commands/executor.rs`:

Add to match:

```rust
RuntimeCommandType::InstallSkills => self.handle_install_skills(command).await,
```

Add handler:

```rust
async fn handle_install_skills(&self, command: RuntimeCommand) -> anyhow::Result<RuntimeCommandOutcome> {
    let payload: crate::commands::install_skills::InstallSkillsCommandPayload =
        serde_json::from_value(command.payload.clone())
            .context("invalid runtime install_skills command payload")?;
    let (s3_client, bucket) = match (&self.s3_client, &self.s3_bucket) {
        (Some(client), Some(bucket)) => (client, bucket),
        _ => anyhow::bail!("skills require S3 configuration but s3 client is not configured"),
    };
    match crate::commands::install_skills::install_skill_targets(&payload, s3_client, bucket).await {
        Ok(installed) => {
            if let Some(control_plane) = &self.control_plane {
                control_plane.complete_runtime_command(&command.id, &install_skills_completed_terminal(installed)).await?;
            }
            Ok(RuntimeCommandOutcome { command_id: command.id, accepted: true, run_id: None })
        }
        Err(error) => {
            if let Some(control_plane) = &self.control_plane {
                control_plane.complete_runtime_command(&command.id, &install_skills_failed_terminal(error.to_string())).await?;
            }
            Err(error)
        }
    }
}
```

Add terminal helpers:

```rust
fn install_skills_completed_terminal(installed: Vec<crate::commands::install_skills::InstalledSkillTarget>) -> RuntimeCommandTerminalWriteback {
    let mut result = HashMap::new();
    result.insert("installed".to_string(), serde_json::to_value(installed).unwrap_or_else(|_| serde_json::Value::Array(Vec::new())));
    RuntimeCommandTerminalWriteback {
        status: "completed".to_string(),
        summary: Some("skills installed".to_string()),
        result: Some(result),
        diagnostic: None,
        provider_session_external_id: None,
        session_state_patch: None,
        log_ref: None,
        raw_result_ref: None,
        error_message: None,
        error_code: None,
        error_family: None,
    }
}

fn install_skills_failed_terminal(error_message: String) -> RuntimeCommandTerminalWriteback {
    RuntimeCommandTerminalWriteback {
        status: "failed".to_string(),
        summary: None,
        result: None,
        diagnostic: None,
        provider_session_external_id: None,
        session_state_patch: None,
        log_ref: None,
        raw_result_ref: None,
        error_message: Some(error_message),
        error_code: Some("install_skills_failed".to_string()),
        error_family: Some("runtime_skill_install".to_string()),
    }
}
```

- [ ] **Step 11: Run Runtime focused tests**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml install_skills --tests
cargo test --manifest-path apps/runtime-agent/Cargo.toml runtime_command_payload --test runtime_command_payload_test
```

Expected: PASS.

- [ ] **Step 12: Commit Runtime install command**

Run:

```bash
git add apps/runtime-agent/src/commands/install_skills.rs \
  apps/runtime-agent/src/controlplane/models.rs \
  apps/runtime-agent/src/commands/executor.rs \
  apps/runtime-agent/src/skills.rs \
  apps/runtime-agent/tests/install_skills_test.rs \
  apps/runtime-agent/tests/runtime_command_payload_test.rs
git commit -m "feat: install skills in runtime workspaces"
```

Expected: commit includes Runtime command parsing, install orchestration, and tests.

## Task 6: Web API Client And Install Dialog

**Files:**
- Modify: `apps/web/src/lib/api/skills.ts`
- Modify: `apps/web/src/lib/api/skills.test.ts`
- Create: `apps/web/src/features/skills/install-dialog.tsx`
- Modify: `apps/web/src/features/skills/index.tsx`
- Modify: `apps/web/src/features/skills/index.test.tsx`

- [ ] **Step 1: Add failing API client tests**

Append to `apps/web/src/lib/api/skills.test.ts`:

```ts
it("installs a skill to an employee", async () => {
  const calls: Array<{ url: string; init: RequestInit }> = [];
  const fetcher = vi.fn(async (url: string | URL, init?: RequestInit) => {
    calls.push({ url: String(url), init: init ?? {} });
    return jsonResponse({ skill_id: "skill-1", target_scope: "employee", installed_count: 1, installations: [] });
  });

  const result = await installSkill(
    { baseUrl: "http://control-plane.local", fetcher: fetcher as unknown as typeof fetch },
    "skill 1/ops",
    { target_scope: "employee", digital_employee_id: "employee-1", timeout_sec: 15 },
  );

  expect(result.installed_count).toBe(1);
  expect(calls[0].url).toBe("http://control-plane.local/api/v1/skills/skill%201%2Fops/install");
  expect(calls[0].init.method).toBe("POST");
  expect(JSON.parse(String(calls[0].init.body))).toEqual({
    target_scope: "employee",
    digital_employee_id: "employee-1",
    timeout_sec: 15,
  });
});
```

- [ ] **Step 2: Run client test and verify it fails**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- skills.test.ts
```

Expected: FAIL because `installSkill` does not exist.

- [ ] **Step 3: Add API types and client**

Modify `apps/web/src/lib/api/skills.ts`:

```ts
export type SkillInstallTargetScope = "team" | "employee";

export type InstallSkillInput = {
  target_scope: SkillInstallTargetScope;
  team_id?: string;
  digital_employee_id?: string;
  timeout_sec?: number;
};

export type SkillInstallation = {
  digital_employee_id: string;
  employee_name?: string;
  provider_type: "opencode" | "codex" | "claude-code";
  runtime_node_id: string;
  node_id?: string;
  installed_path: string;
  archive_checksum_sha256: string;
  installed_at?: string;
};

export type InstallSkillResult = {
  skill_id: string;
  target_scope: SkillInstallTargetScope;
  team_id?: string;
  digital_employee_id?: string;
  installed_count: number;
  installations: SkillInstallation[];
};

export async function installSkill(
  options: ApiClientOptions,
  skillId: string,
  input: InstallSkillInput,
): Promise<InstallSkillResult> {
  const fetcher = options.fetcher ?? fetch;
  const encodedSkillId = encodeURIComponent(skillId);
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/skills/${encodedSkillId}/install`), {
    body: JSON.stringify(input),
    credentials: "include",
    headers: { accept: "application/json", "content-type": "application/json" },
    method: "POST",
  });

  return parseJson<InstallSkillResult>(response, "install skill");
}
```

- [ ] **Step 4: Run client tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- skills.test.ts
```

Expected: PASS.

- [ ] **Step 5: Create install dialog**

Create `apps/web/src/features/skills/install-dialog.tsx`:

```tsx
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Bot, Users } from "lucide-react";
import { V3Button, V3Segmented } from "@/components/superteam";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { listDigitalEmployees, type DigitalEmployee } from "@/lib/api/employees";
import { installSkill, type InstallSkillResult, type Skill } from "@/lib/api/skills";
import { listTeams, type TeamListItem } from "@/lib/api/teams";

type SkillInstallDialogProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  skill: Skill | null;
};

export function SkillInstallDialog({ apiBaseUrl, fetcher, onOpenChange, open, skill }: SkillInstallDialogProps) {
  const queryClient = useQueryClient();
  const [targetScope, setTargetScope] = useState<"team" | "employee">("employee");
  const [targetId, setTargetId] = useState("");
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const teams = useQuery<TeamListItem[]>({
    enabled: open,
    queryKey: ["teams", "skill-install-targets"],
    queryFn: () => listTeams(apiOptions),
  });
  const employees = useQuery<DigitalEmployee[]>({
    enabled: open,
    queryKey: ["digital-employees", "skill-install-targets"],
    queryFn: () => listDigitalEmployees(apiOptions),
  });
  const mutation = useMutation<InstallSkillResult, Error>({
    mutationFn: () => {
      if (!skill) throw new Error("未选择技能");
      return installSkill(apiOptions, skill.id, {
        target_scope: targetScope,
        team_id: targetScope === "team" ? targetId : undefined,
        digital_employee_id: targetScope === "employee" ? targetId : undefined,
        timeout_sec: 15,
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["skills"] });
      if (skill) await queryClient.invalidateQueries({ queryKey: ["skill", skill.id] });
    },
  });

  const teamOptions = teams.data ?? [];
  const employeeOptions = employees.data ?? [];
  const targetOptionsLoading = targetScope === "team" ? teams.isPending : employees.isPending;
  const targetOptionsError = targetScope === "team" ? teams.error : employees.error;
  const canSubmit = Boolean(skill && targetId && !mutation.isPending && !targetOptionsLoading);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>安装技能</DialogTitle>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <p className="text-sm font-bold text-v3-ink">{skill?.name ?? "未选择技能"}</p>
            <p className="mt-1 text-xs text-v3-ink-2">{skill?.description}</p>
          </div>
          <V3Segmented
            ariaLabel="安装目标类型"
            items={[
              { label: "数字员工", value: "employee" },
              { label: "团队", value: "team" },
            ]}
            onValueChange={(value) => {
              setTargetScope(value as "team" | "employee");
              setTargetId("");
              mutation.reset();
            }}
            value={targetScope}
          />
          <Select value={targetId} onValueChange={setTargetId}>
            <SelectTrigger aria-label="选择安装目标">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {targetScope === "team"
                ? teamOptions.map((team) => (
                    <SelectItem key={team.id} value={team.id}>
                      <span className="inline-flex items-center gap-2">
                        <Users className="size-4" />
                        {team.name || team.id}
                      </span>
                    </SelectItem>
                  ))
                : employeeOptions.map((employee) => (
                    <SelectItem key={employee.id} value={employee.id}>
                      <span className="inline-flex items-center gap-2">
                        <Bot className="size-4" />
                        {employee.name || employee.id}
                      </span>
                    </SelectItem>
                  ))}
            </SelectContent>
          </Select>
          {targetOptionsError instanceof Error ? (
            <p className="rounded-v3-inner border border-v3-danger/30 bg-v3-danger/10 p-3 text-sm font-semibold text-v3-danger">
              {targetOptionsError.message}
            </p>
          ) : null}
          {!targetOptionsLoading && (targetScope === "team" ? teamOptions.length : employeeOptions.length) === 0 ? (
            <p className="rounded-v3-inner border border-v3-line bg-v3-card-inner p-3 text-sm font-semibold text-v3-ink-2">
              {targetScope === "team" ? "暂无可选团队" : "暂无可选数字员工"}
            </p>
          ) : null}
          {mutation.isError ? (
            <p className="rounded-v3-inner border border-v3-danger/30 bg-v3-danger/10 p-3 text-sm font-semibold text-v3-danger">
              {mutation.error.message}
            </p>
          ) : null}
          {mutation.isSuccess ? (
            <p className="rounded-v3-inner border border-v3-ok/30 bg-v3-ok/10 p-3 text-sm font-semibold text-v3-ok">
              已安装到 {mutation.data.installed_count} 个数字员工
            </p>
          ) : null}
          <div className="flex justify-end gap-2">
            <V3Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              关闭
            </V3Button>
            <V3Button type="button" disabled={!canSubmit} onClick={() => mutation.mutate()}>
              {mutation.isPending ? "安装中..." : "确认安装"}
            </V3Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
```

- [ ] **Step 6: Wire marketplace install button**

Modify `apps/web/src/features/skills/index.tsx`:

Import dialog:

```ts
import { SkillInstallDialog } from "@/features/skills/install-dialog";
```

Add state:

```ts
const [installSkillTarget, setInstallSkillTarget] = useState<Skill | null>(null);
```

Render dialog near the end of `SkillsView`:

```tsx
<SkillInstallDialog
  apiBaseUrl={apiBaseUrl}
  fetcher={fetcher}
  open={Boolean(installSkillTarget)}
  onOpenChange={(open) => {
    if (!open) setInstallSkillTarget(null);
  }}
  skill={installSkillTarget}
/>
```

Pass `onInstallSkill={setInstallSkillTarget}` into list/grid components and add a button:

```tsx
<V3Button size="sm" type="button" onClick={() => onInstallSkill(skill)}>
  安装
</V3Button>
```

Use an icon if the surrounding row already has icon-only action patterns; otherwise keep the text button consistent with existing table actions.

- [ ] **Step 7: Add UI tests**

Append to `apps/web/src/features/skills/index.test.tsx`:

```tsx
it("opens the install dialog from the skill market", async () => {
  render(<SkillsView apiBaseUrl="http://control-plane.local" fetcher={createSkillsFetcher([skillFixture])} />);

  await expect.element(screen.getByText(skillFixture.name)).toBeVisible();
  await userEvent.click(screen.getAllByRole("button", { name: /安装/ })[0]);

  await expect.element(screen.getByRole("dialog")).toBeVisible();
  await expect.element(screen.getByText("安装技能")).toBeVisible();
});

it("submits the selected employee install target", async () => {
  const fetcher = createSkillsFetcher([skillFixture]);
  fetcher.mockImplementation(async (url: string | URL, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    if (path === "/api/v1/teams") {
      return jsonResponse([]);
    }
    if (path === "/api/v1/digital-employees") {
      return jsonResponse([{ id: "employee-1", tenant_id: "tenant-1", team_id: "team-1", owner_user_id: "user-1", employee_type: "agent", name: "需求澄清 Agent", role: "analyst", status: "active", permission_policy: {}, context_policy: {}, approval_policy: {}, risk_level: "low" }]);
    }
    if (path === `/api/v1/skills/${skillFixture.id}/install` && init?.method === "POST") {
      return jsonResponse({ skill_id: skillFixture.id, target_scope: "employee", digital_employee_id: "employee-1", installed_count: 1, installations: [] }, 201);
    }
    return createSkillsFetcher([skillFixture])(url, init);
  });
  render(<SkillsView apiBaseUrl="http://control-plane.local" fetcher={fetcher as unknown as typeof fetch} />);

  await userEvent.click(screen.getAllByRole("button", { name: /安装/ })[0]);
  await userEvent.click(screen.getByLabelText("选择安装目标"));
  await userEvent.click(screen.getByText("需求澄清 Agent"));
  await userEvent.click(screen.getByRole("button", { name: "确认安装" }));

  await expect.element(screen.getByText("已安装到 1 个数字员工")).toBeVisible();
  const installCall = fetcher.mock.calls.find(([url, init]) => String(url).endsWith(`/api/v1/skills/${skillFixture.id}/install`) && init?.method === "POST");
  expect(JSON.parse(String(installCall?.[1]?.body))).toEqual({ target_scope: "employee", digital_employee_id: "employee-1", timeout_sec: 15 });
});
```

Import `userEvent` and `screen` from `@vitest/browser/context` if the file does not already import them.

- [ ] **Step 8: Run Web focused tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- skills
```

Expected: PASS.

- [ ] **Step 9: Commit Web install dialog**

Run:

```bash
git add apps/web/src/lib/api/skills.ts \
  apps/web/src/lib/api/skills.test.ts \
  apps/web/src/features/skills/install-dialog.tsx \
  apps/web/src/features/skills/index.tsx \
  apps/web/src/features/skills/index.test.tsx
git commit -m "feat(web): add skill install dialog"
```

Expected: commit includes Web API client and marketplace install UI.

## Task 7: Skill Detail Installation Visibility

**Files:**
- Modify: `apps/control-plane/internal/skill/types.go`
- Modify: `apps/control-plane/internal/skill/pg_repository.go`
- Modify: `apps/control-plane/internal/skill/handler.go`
- Modify: `apps/web/src/lib/api/skills.ts`
- Modify: `apps/web/src/features/skills/detail.tsx`
- Modify: `apps/web/src/features/skills/detail.test.tsx`

- [ ] **Step 1: Extend Skill type with installations**

Add to `Skill` in `apps/control-plane/internal/skill/types.go`:

```go
Installations []SkillInstallation
```

Add to Web `Skill` type in `apps/web/src/lib/api/skills.ts`:

```ts
installations?: SkillInstallation[];
```

- [ ] **Step 2: Load installations in repository**

In `apps/control-plane/internal/skill/pg_repository.go`, update `loadChildren`:

```go
installations, err := r.listInstallations(ctx, item.TenantID, item.ID)
if err != nil {
	return err
}
item.Installations = installations
```

Add:

```go
func (r *PgRepository) listInstallations(ctx context.Context, tenantID, skillID uuid.UUID) ([]SkillInstallation, error) {
	rows, err := r.db.Query(ctx, `
SELECT si.id, si.tenant_id, si.skill_id, si.target_scope, COALESCE(si.team_id::text, ''),
       si.digital_employee_id, COALESCE(de.name, ''), si.runtime_node_id,
       COALESCE(rn.node_id, ''), si.provider_type, si.installed_path,
       si.archive_checksum_sha256, COALESCE(si.installed_by::text, ''),
       si.installed_at, COALESCE(si.metadata, '{}'::jsonb)
FROM skill_installations si
LEFT JOIN digital_employees de ON de.tenant_id = si.tenant_id AND de.id = si.digital_employee_id
LEFT JOIN runtime_nodes rn ON rn.tenant_id = si.tenant_id AND rn.id = si.runtime_node_id
WHERE si.tenant_id = $1 AND si.skill_id = $2 AND si.deleted_at IS NULL
ORDER BY si.installed_at DESC`, tenantID, skillID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SkillInstallation
	for rows.Next() {
		var item SkillInstallation
		var teamIDText, installedByText string
		var metadata []byte
		if err := rows.Scan(&item.ID, &item.TenantID, &item.SkillID, &item.TargetScope, &teamIDText, &item.DigitalEmployeeID, &item.EmployeeName, &item.RuntimeNodeID, &item.NodeID, &item.ProviderType, &item.InstalledPath, &item.ArchiveChecksumSHA256, &installedByText, &item.InstalledAt, &metadata); err != nil {
			return nil, err
		}
		if teamIDText != "" {
			item.TeamID, _ = uuid.Parse(teamIDText)
		}
		if installedByText != "" {
			item.InstalledBy, _ = uuid.Parse(installedByText)
		}
		_ = json.Unmarshal(metadata, &item.Metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Include installations in skill response**

Modify `skillResponse` in `handler.go`:

```go
Installations []skillInstallationResponse `json:"installations"`
```

In `skillResponseFromDomain`, map:

```go
Installations: skillInstallationResponses(item.Installations),
```

Add:

```go
func skillInstallationResponses(items []SkillInstallation) []skillInstallationResponse {
	out := make([]skillInstallationResponse, 0, len(items))
	for _, item := range items {
		out = append(out, skillInstallationResponse{
			DigitalEmployeeID: item.DigitalEmployeeID.String(),
			EmployeeName: item.EmployeeName,
			ProviderType: item.ProviderType,
			RuntimeNodeID: item.RuntimeNodeID.String(),
			NodeID: item.NodeID,
			InstalledPath: item.InstalledPath,
			ArchiveChecksumSHA256: item.ArchiveChecksumSHA256,
			InstalledAt: item.InstalledAt.Format(time.RFC3339),
		})
	}
	return out
}
```

- [ ] **Step 4: Update detail UI**

Modify `apps/web/src/features/skills/detail.tsx` to replace the current inactive install sections with actual installation rows:

```tsx
<DetailSection icon={<PackageCheck />} title="数字员工装载">
  {(skill.installations?.length ?? 0) === 0 ? (
    <V3EmptyState title="暂无装载记录" description="安装成功后会显示实际 Provider、Runtime 与路径。" />
  ) : (
    <div className="grid gap-3">
      {skill.installations?.map((installation) => (
        <div key={`${installation.digital_employee_id}-${installation.installed_path}`} className="rounded-v3-inner border border-v3-line bg-v3-card-inner p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="font-bold text-v3-ink">{installation.employee_name || installation.digital_employee_id}</p>
            <StatusPill tone="ok">{installation.provider_type}</StatusPill>
          </div>
          <p className="mt-2 font-mono text-xs text-v3-ink-2">{installation.installed_path}</p>
          <p className="mt-1 text-xs text-v3-ink-3">{installation.node_id || installation.runtime_node_id}</p>
        </div>
      ))}
    </div>
  )}
</DetailSection>
```

- [ ] **Step 5: Add detail test**

Append to `apps/web/src/features/skills/detail.test.tsx`:

```tsx
it("renders physical skill installations", async () => {
  const skill = {
    ...skillFixture,
    installations: [{
      digital_employee_id: "employee-1",
      employee_name: "需求澄清 Agent",
      provider_type: "codex",
      runtime_node_id: "runtime-1",
      node_id: "node-local",
      installed_path: "/runtime/employee/.agents/skills/review",
      archive_checksum_sha256: "a".repeat(64),
      installed_at: "2026-06-24T10:00:00Z",
    }],
  };
  render(<SkillDetailView apiBaseUrl="http://control-plane.local" fetcher={createSkillFetcher(skill)} skillId={skill.id} />);

  await expect.element(screen.getByText("数字员工装载")).toBeVisible();
  await expect.element(screen.getByText("需求澄清 Agent")).toBeVisible();
  await expect.element(screen.getByText("/runtime/employee/.agents/skills/review")).toBeVisible();
  await expect.element(screen.getByText("codex")).toBeVisible();
});
```

- [ ] **Step 6: Run detail tests**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- detail.test.tsx
go test ./apps/control-plane/internal/skill -run Test -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit detail visibility**

Run:

```bash
git add apps/control-plane/internal/skill/types.go \
  apps/control-plane/internal/skill/pg_repository.go \
  apps/control-plane/internal/skill/handler.go \
  apps/web/src/lib/api/skills.ts \
  apps/web/src/features/skills/detail.tsx \
  apps/web/src/features/skills/detail.test.tsx
git commit -m "feat: show skill installation records"
```

Expected: commit includes detail API and UI visibility.

## Task 8: End-To-End Verification, Changelog, And Final Gates

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Run backend and Runtime tests**

Run:

```bash
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/skill ./apps/control-plane/internal/api ./apps/control-plane/internal/app -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml
```

Expected: all tests PASS.

- [ ] **Step 2: Run Web tests and typecheck**

Run:

```bash
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web run typecheck
```

Expected: tests and typecheck PASS.

- [ ] **Step 3: Verify contracts**

Run:

```bash
corepack pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 4: Run project hygiene check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Restart relevant services**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart runtime-agent
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Expected: Control Plane, Runtime Agent, and Web are running current code.

- [ ] **Step 6: Select real smoke records**

Use the running development stack and the existing admin cookie. Select one uploaded skill with archive metadata and one runnable digital employee:

```bash
SKILLS_JSON="$(curl -sS -b .scratch/admin-cookie.txt http://localhost:8081/api/v1/skills)"
SKILL_ID="$(printf '%s' "$SKILLS_JSON" | node -e 'let s="";process.stdin.on("data",c=>s+=c);process.stdin.on("end",()=>{const rows=JSON.parse(s);const row=rows.find(x=>x.archive_object_ref&&x.archive_checksum_sha256&&x.archive_size_bytes>0&&x.archive_file_count>0); if(!row) process.exit(2); console.log(row.id);})')"
EMPLOYEES_JSON="$(curl -sS -b .scratch/admin-cookie.txt http://localhost:8081/api/v1/digital-employees)"
EMPLOYEE_ID="$(printf '%s' "$EMPLOYEES_JSON" | node -e 'let s="";process.stdin.on("data",c=>s+=c);process.stdin.on("end",()=>{const rows=JSON.parse(s);const row=rows.find(x=>x.status==="active"||x.status==="ready"); if(!row) process.exit(2); console.log(row.id);})')"
printf 'SKILL_ID=%s\nEMPLOYEE_ID=%s\n' "$SKILL_ID" "$EMPLOYEE_ID"
```

Expected: both variables print non-empty UUIDs. If this command exits non-zero, create or upload the missing development data through the existing UI/API before continuing.

- [ ] **Step 7: Real smoke through API**

Run:

```bash
curl -i -sS \
  -H 'Content-Type: application/json' \
  -b .scratch/admin-cookie.txt \
  -X POST "http://localhost:8081/api/v1/skills/${SKILL_ID}/install" \
  --data "{\"target_scope\":\"employee\",\"digital_employee_id\":\"${EMPLOYEE_ID}\",\"timeout_sec\":15}"
```

Expected:

- HTTP status is `201`.
- Response contains `"installed_count":1`.
- Response contains an `installed_path` under `.opencode/skills`, `.agents/skills`, or `.claude/skills`.

- [ ] **Step 8: Verify filesystem and detail API**

Run:

```bash
INSTALL_RESPONSE="$(curl -sS \
  -H 'Content-Type: application/json' \
  -b .scratch/admin-cookie.txt \
  -X POST "http://localhost:8081/api/v1/skills/${SKILL_ID}/install" \
  --data "{\"target_scope\":\"employee\",\"digital_employee_id\":\"${EMPLOYEE_ID}\",\"timeout_sec\":15}")"
INSTALLED_PATH="$(printf '%s' "$INSTALL_RESPONSE" | node -e 'let s="";process.stdin.on("data",c=>s+=c);process.stdin.on("end",()=>{const body=JSON.parse(s); console.log(body.installations?.[0]?.installed_path || "");})')"
test -f "${INSTALLED_PATH}/SKILL.md"
curl -sS -b .scratch/admin-cookie.txt "http://localhost:8081/api/v1/skills/${SKILL_ID}" | rg '"installations"|installed_path|provider_type'
```

Expected:

- `SKILL.md` exists under the returned provider-specific directory.
- Skill detail API includes installation data.

- [ ] **Step 9: Real browser smoke**

Use the Codex Chrome plug/browser automation as required by `AGENTS.md`:

1. Open the running Web app URL from `scripts/dev-services.sh status`.
2. Use the existing local authenticated session or log in with the development admin account.
3. Navigate to `/skills`.
4. Click `Install` for the smoke skill.
5. Select the same target employee.
6. Confirm success or idempotent reinstall success.
7. Open the skill detail page.
8. Confirm the installation row shows employee, provider, Runtime, and installed path.

Expected: no console errors, no stuck loading state, and installation data is visible from the real Control Plane response.

- [ ] **Step 10: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add a `CHANGELOG.md` entry with that timestamp, summarizing:

- synchronous skill installation API
- Runtime `install_skills`
- provider-specific directories for `opencode`, `codex`, `claude-code`
- marketplace install dialog and detail installation records
- verification commands and real smoke evidence

- [ ] **Step 11: Final full check**

Run:

```bash
go test ./apps/control-plane/internal/skill ./apps/control-plane/internal/api -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml install_skills --tests
corepack pnpm --filter ./apps/web run test -- skills
git diff --check
```

Expected: all pass.

- [ ] **Step 12: Commit final verification/changelog**

Run:

```bash
git add CHANGELOG.md
git commit -m "docs: record skill install delivery"
```

Expected: final commit records verification evidence.

## Final Verification Checklist

Before claiming implementation complete, run the project completion skill and report one of:

- `真实链路验证：...` with the real API, filesystem, and browser evidence.
- `阻塞：...；尚不能声明完成` if Runtime, auth, migration, object storage, or Provider dependencies prevent a real smoke.

Do not call the feature usable if only unit, component, or mock tests passed.
