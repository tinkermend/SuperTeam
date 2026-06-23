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
	if len(repo.failureLogs) != 1 {
		t.Fatalf("expected one failure log, got %#v", repo.failureLogs)
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
	if len(repo.installations) != 0 || len(repo.teamBindings) != 0 || len(repo.employeeBindings) != 0 {
		t.Fatalf("missing archive must not persist success rows: repo=%#v", repo)
	}
	if len(repo.failureLogs) != 1 {
		t.Fatalf("expected one failure log, got %#v", repo.failureLogs)
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
	if len(repo.installations) != 0 || len(repo.teamBindings) != 0 || len(repo.employeeBindings) != 0 {
		t.Fatalf("disconnected runtime must not persist success rows: repo=%#v", repo)
	}
	if len(repo.failureLogs) != 1 {
		t.Fatalf("expected one failure log, got %#v", repo.failureLogs)
	}
	if got := repo.failureLogs[0].ReasonCode; got != "runtime_not_connected" {
		t.Fatalf("failure reason = %q, want runtime_not_connected", got)
	}
}

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
	if len(repo.failureLogs) == 0 {
		t.Fatal("expected runtime failure log")
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
	if cleanup, _ := last.Details["requires_manual_cleanup"].(bool); !cleanup {
		t.Fatalf("expected requires_manual_cleanup, got %#v", last.Details)
	}
}

type installServiceRepo struct {
	skill            *Skill
	targets          []SkillInstallTarget
	receipts         map[string]*RuntimeInstallCommandReceipt
	waitHook         func(commandID string)
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
			ArchiveChecksum:  "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
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
	if r.waitHook != nil {
		r.waitHook(commandID)
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
