package tenant

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTeamStatusAllowsArchived(t *testing.T) {
	if !TeamStatusArchived.IsValid() {
		t.Fatalf("expected archived team status to be valid")
	}
	if TeamStatus("paused").IsValid() {
		t.Fatalf("expected unknown team status to be invalid")
	}
}

func TestCreateTeamDefaultsActiveStatus(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()

	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         uuid.New(),
		Slug:             "engineering",
		Name:             "Engineering",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if team.Status != TeamStatusActive {
		t.Fatalf("expected active default status, got %q", team.Status)
	}
}

func TestCreateTeamRequiresHumanOwner(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID: uuid.New(),
		Slug:     "engineering",
		Name:     "Engineering",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
	if repo.createTeamCalled {
		t.Fatalf("expected invalid team not to reach repository")
	}
}

func TestCreateTeamConfigRevisionDefaultsActiveStatus(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	approvedBy := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "engineering",
		Name:             "Engineering",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	revision, err := svc.CreateConfigRevision(context.Background(), CreateTeamConfigRevisionRequest{
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		Constitution:                map[string]any{"principle": "review before execute"},
		CapabilityPolicy:            map[string]any{"providers": []any{"codex"}},
		ContextPolicy:               map[string]any{"sources": []any{"task"}},
		ApprovalPolicy:              map[string]any{"risk": "high"},
		ArtifactContract:            map[string]any{"required": []any{"handoff"}},
		InternalCollaborationPolicy: map[string]any{"mode": "structured"},
		RuntimeScopePolicy:          map[string]any{"scope": "team"},
		HumanOwnerUserID:            &ownerID,
		ApprovedBy:                  &approvedBy,
	})
	if err != nil {
		t.Fatalf("create config revision: %v", err)
	}

	if revision.Status != TeamConfigRevisionStatusActive {
		t.Fatalf("expected active status, got %q", revision.Status)
	}
	if revision.RevisionNumber != 1 {
		t.Fatalf("expected revision number 1, got %d", revision.RevisionNumber)
	}
	if revision.ApprovedAt == nil || revision.ApprovedAt.IsZero() {
		t.Fatalf("expected approved_at to default to current time")
	}
	if revision.ApprovedAt.Location() != time.UTC {
		t.Fatalf("expected approved_at in UTC, got %s", revision.ApprovedAt.Location())
	}
	if repo.createdRevision.Status != TeamConfigRevisionStatusActive {
		t.Fatalf("expected active status sent to repository, got %q", repo.createdRevision.Status)
	}
	if repo.createdRevision.RevisionNumber != 1 {
		t.Fatalf("expected repository revision number 1, got %d", repo.createdRevision.RevisionNumber)
	}
	repo.createdRevision.Constitution["principle"] = "mutated"
	if revision.Constitution["principle"] != "review before execute" {
		t.Fatalf("expected revision policy maps to be cloned, got %#v", revision.Constitution)
	}
}

func TestCreateTeamConfigRevisionRequiresExistingTeam(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	ownerID := uuid.New()
	tenantID := uuid.New()
	otherTenantID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "engineering",
		Name:             "Engineering",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	tests := []struct {
		name     string
		tenantID uuid.UUID
		teamID   uuid.UUID
	}{
		{name: "missing team", tenantID: tenantID, teamID: uuid.New()},
		{name: "wrong tenant", tenantID: otherTenantID, teamID: team.ID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beforeInserts := repo.createRevisionCalls
			_, err := svc.CreateConfigRevision(context.Background(), CreateTeamConfigRevisionRequest{
				TenantID:         tt.tenantID,
				TeamID:           tt.teamID,
				HumanOwnerUserID: &ownerID,
			})
			if !errors.Is(err, ErrNotFound) {
				t.Fatalf("expected not found error, got %v", err)
			}
			if repo.createRevisionCalls != beforeInserts {
				t.Fatalf("expected missing/wrong-tenant team not to insert revision")
			}
		})
	}
}

func TestCreateTeamConfigRevisionRejectsSecondActiveBeforeInsert(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "engineering",
		Name:             "Engineering",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if _, err := svc.CreateConfigRevision(context.Background(), CreateTeamConfigRevisionRequest{
		TenantID:         tenantID,
		TeamID:           team.ID,
		HumanOwnerUserID: &ownerID,
	}); err != nil {
		t.Fatalf("create first active revision: %v", err)
	}
	beforeInserts := repo.createRevisionCalls

	_, err = svc.CreateConfigRevision(context.Background(), CreateTeamConfigRevisionRequest{
		TenantID:         tenantID,
		TeamID:           team.ID,
		HumanOwnerUserID: &ownerID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for second active revision, got %v", err)
	}
	if repo.createRevisionCalls != beforeInserts {
		t.Fatalf("expected second active revision not to be inserted")
	}
}

func TestCreateTeamConfigRevisionDraftHasNoApprovalMetadata(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	approvedBy := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "engineering",
		Name:             "Engineering",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	revision, err := svc.CreateConfigRevision(context.Background(), CreateTeamConfigRevisionRequest{
		TenantID:         tenantID,
		TeamID:           team.ID,
		HumanOwnerUserID: &ownerID,
		Status:           TeamConfigRevisionStatusDraft,
		ApprovedBy:       &approvedBy,
	})
	if err != nil {
		t.Fatalf("create draft revision: %v", err)
	}
	if revision.Status != TeamConfigRevisionStatusDraft {
		t.Fatalf("expected draft status, got %q", revision.Status)
	}
	if revision.ApprovedAt != nil {
		t.Fatalf("expected draft revision approved_at to be nil, got %v", revision.ApprovedAt)
	}
	if revision.ApprovedBy != nil {
		t.Fatalf("expected draft revision approved_by to be nil, got %v", revision.ApprovedBy)
	}
	if repo.createdRevision.ApprovedAt != nil || repo.createdRevision.ApprovedBy != nil {
		t.Fatalf("expected draft approval metadata cleared before repository insert, got approved_at=%v approved_by=%v", repo.createdRevision.ApprovedAt, repo.createdRevision.ApprovedBy)
	}
}

func TestCreateTeamConfigRevisionRejectsTerminalStatusesBeforeInsert(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	for _, status := range []TeamConfigRevisionStatus{TeamConfigRevisionStatusRejected, TeamConfigRevisionStatusArchived} {
		t.Run(string(status), func(t *testing.T) {
			beforeInserts := repo.createRevisionCalls
			_, err := svc.CreateConfigRevision(context.Background(), CreateTeamConfigRevisionRequest{
				TenantID:         tenantID,
				TeamID:           team.ID,
				HumanOwnerUserID: &ownerID,
				Status:           status,
			})
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected invalid input for status %q, got %v", status, err)
			}
			if repo.createRevisionCalls != beforeInserts {
				t.Fatalf("expected terminal status %q not to insert revision", status)
			}
		})
	}
}

func TestApproveGovernanceDraftArchivesPreviousActive(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	approvedBy := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	now := time.Now().UTC()
	activeID := uuid.New()
	draftID := uuid.New()
	repo.revisions[activeID] = TeamConfigRevisionRecord{
		ID:                          activeID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              7,
		Constitution:                map[string]any{"hard_rules": []any{"existing approval rule"}},
		CapabilityPolicy:            map[string]any{"skill_bindings": []any{"incident-diagnosis"}},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusActive,
		ApprovedAt:                  &now,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}
	repo.revisions[draftID] = TeamConfigRevisionRecord{
		ID:                          draftID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              8,
		Constitution:                map[string]any{"hard_rules": []any{"existing approval rule", "new production write rule"}},
		CapabilityPolicy:            map[string]any{"skill_bindings": []any{"incident-diagnosis", "release-review"}},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusDraft,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}

	approved, err := svc.ApproveGovernanceDraft(context.Background(), tenantID, team.ID, draftID, approvedBy)
	if err != nil {
		t.Fatalf("approve governance draft: %v", err)
	}

	if approved.Status != TeamConfigRevisionStatusActive || approved.RevisionNumber != 8 {
		t.Fatalf("expected draft v8 to become active, got status=%q revision=%d", approved.Status, approved.RevisionNumber)
	}
	if approved.ApprovedBy == nil || *approved.ApprovedBy != approvedBy {
		t.Fatalf("expected approved_by %s, got %#v", approvedBy, approved.ApprovedBy)
	}
	if repo.revisions[activeID].Status != TeamConfigRevisionStatus("archived") {
		t.Fatalf("expected active v7 to be archived, got %q", repo.revisions[activeID].Status)
	}
	if repo.revisions[draftID].Status != TeamConfigRevisionStatusActive {
		t.Fatalf("expected draft v8 to be active in repository, got %q", repo.revisions[draftID].Status)
	}
}

func TestApproveGovernanceDraftRejectsMissingHardRules(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	approvedBy := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	draftID := uuid.New()
	repo.revisions[draftID] = TeamConfigRevisionRecord{
		ID:                          draftID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              8,
		Constitution:                map[string]any{"hard_rules": []any{}},
		CapabilityPolicy:            map[string]any{},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusDraft,
		CreatedAt:                   time.Now().UTC(),
		UpdatedAt:                   time.Now().UTC(),
	}

	_, err = svc.ApproveGovernanceDraft(context.Background(), tenantID, team.ID, draftID, approvedBy)

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for missing hard rules, got %v", err)
	}
	if repo.revisions[draftID].Status != TeamConfigRevisionStatusDraft {
		t.Fatalf("expected invalid draft to remain draft, got %q", repo.revisions[draftID].Status)
	}
}

func TestApproveGovernanceDraftRejectsNonStringHardRules(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	draftID := uuid.New()
	now := time.Now().UTC()
	repo.revisions[draftID] = TeamConfigRevisionRecord{
		ID:                          draftID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              8,
		Constitution:                map[string]any{"hard_rules": []any{"must approve", 123}},
		CapabilityPolicy:            map[string]any{},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusDraft,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}

	_, err = svc.ApproveGovernanceDraft(context.Background(), tenantID, team.ID, draftID, uuid.New())

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for non-string hard rules, got %v", err)
	}
	if repo.revisions[draftID].Status != TeamConfigRevisionStatusDraft {
		t.Fatalf("expected invalid draft to remain draft, got %q", repo.revisions[draftID].Status)
	}
}

func TestApproveGovernanceDraftKeepsActiveWhenApprovalFails(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	now := time.Now().UTC()
	activeID := uuid.New()
	draftID := uuid.New()
	repo.revisions[activeID] = TeamConfigRevisionRecord{
		ID:                          activeID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              7,
		Constitution:                map[string]any{"hard_rules": []any{"existing approval rule"}},
		CapabilityPolicy:            map[string]any{"skill_bindings": []any{"incident-diagnosis"}},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusActive,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}
	repo.revisions[draftID] = TeamConfigRevisionRecord{
		ID:                          draftID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              8,
		Constitution:                map[string]any{"hard_rules": []any{"new production write rule"}},
		CapabilityPolicy:            map[string]any{"skill_bindings": []any{"release-review"}},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusDraft,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}
	repo.approveRevisionErr = errors.New("approve failed")

	_, err = svc.ApproveGovernanceDraft(context.Background(), tenantID, team.ID, draftID, uuid.New())

	if err == nil {
		t.Fatalf("expected approval failure")
	}
	if repo.revisions[activeID].Status != TeamConfigRevisionStatusActive {
		t.Fatalf("expected active v7 to remain active after approval failure, got %q", repo.revisions[activeID].Status)
	}
	if repo.revisions[draftID].Status != TeamConfigRevisionStatusDraft {
		t.Fatalf("expected draft v8 to remain draft after approval failure, got %q", repo.revisions[draftID].Status)
	}
}

func TestUpdateGovernanceDraftStoresCapabilityBindings(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	draftID := uuid.New()
	now := time.Now().UTC()
	repo.revisions[draftID] = TeamConfigRevisionRecord{
		ID:                          draftID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              8,
		Constitution:                map[string]any{"hard_rules": []any{"human approval before deploy"}},
		CapabilityPolicy:            map[string]any{},
		ContextPolicy:               map[string]any{},
		ApprovalPolicy:              map[string]any{},
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusDraft,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}
	expectedCapabilityPolicy := map[string]any{
		"skill_bindings":               []any{"incident-diagnosis", "release-review"},
		"mcp_bindings":                 []any{"prometheus"},
		"knowledge_base_bindings":      []any{"runbook-prod"},
		"external_capability_bindings": []any{"deploy-api"},
	}

	updated, err := svc.UpdateGovernanceDraft(context.Background(), tenantID, team.ID, draftID, GovernanceDraftInput{
		Constitution:                map[string]any{"hard_rules": []any{"human approval before deploy"}},
		CapabilityPolicy:            expectedCapabilityPolicy,
		ContextPolicy:               map[string]any{"sources": []any{"task"}},
		ApprovalPolicy:              map[string]any{"min_risk_for_human": "high"},
		ArtifactContract:            map[string]any{"required": []any{"handoff"}},
		InternalCollaborationPolicy: map[string]any{"mode": "structured"},
		RuntimeScopePolicy:          map[string]any{"scope": "team"},
		HumanOwnerUserID:            &ownerID,
	})
	if err != nil {
		t.Fatalf("update governance draft: %v", err)
	}

	if !reflect.DeepEqual(expectedCapabilityPolicy, updated.CapabilityPolicy) {
		t.Fatalf("expected capability bindings to remain in capability_policy, got %#v", updated.CapabilityPolicy)
	}
	if !reflect.DeepEqual(expectedCapabilityPolicy, repo.revisions[draftID].CapabilityPolicy) {
		t.Fatalf("expected repository to store capability bindings, got %#v", repo.revisions[draftID].CapabilityPolicy)
	}
}

func TestUpdateGovernanceDraftPreservesOmittedPolicyMaps(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "platform",
		Name:             "Platform",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	draftID := uuid.New()
	now := time.Now().UTC()
	existingConstitution := map[string]any{"hard_rules": []any{"human approval before deploy"}}
	existingApprovalPolicy := map[string]any{"min_risk_for_human": "high"}
	repo.revisions[draftID] = TeamConfigRevisionRecord{
		ID:                          draftID,
		TenantID:                    tenantID,
		TeamID:                      team.ID,
		RevisionNumber:              8,
		Constitution:                existingConstitution,
		CapabilityPolicy:            map[string]any{"skill_bindings": []any{"incident-diagnosis"}},
		ContextPolicy:               map[string]any{"sources": []any{"task"}},
		ApprovalPolicy:              existingApprovalPolicy,
		ArtifactContract:            map[string]any{},
		InternalCollaborationPolicy: map[string]any{},
		RuntimeScopePolicy:          map[string]any{},
		HumanOwnerUserID:            &ownerID,
		Status:                      TeamConfigRevisionStatusDraft,
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}
	updatedCapabilityPolicy := map[string]any{"skill_bindings": []any{"release-review"}}

	updated, err := svc.UpdateGovernanceDraft(context.Background(), tenantID, team.ID, draftID, GovernanceDraftInput{
		CapabilityPolicy: updatedCapabilityPolicy,
	})
	if err != nil {
		t.Fatalf("update governance draft: %v", err)
	}

	if !reflect.DeepEqual(existingConstitution, updated.Constitution) {
		t.Fatalf("expected omitted constitution to be preserved, got %#v", updated.Constitution)
	}
	if !reflect.DeepEqual(existingApprovalPolicy, updated.ApprovalPolicy) {
		t.Fatalf("expected omitted approval policy to be preserved, got %#v", updated.ApprovalPolicy)
	}
	if !reflect.DeepEqual(updatedCapabilityPolicy, updated.CapabilityPolicy) {
		t.Fatalf("expected supplied capability policy to update, got %#v", updated.CapabilityPolicy)
	}
}

func TestListTeamsRejectsNegativeOffset(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.ListTeams(context.Background(), ListTeamsRequest{
		TenantID: uuid.New(),
		Offset:   -1,
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for negative offset, got %v", err)
	}
	if repo.listTeamsCalled {
		t.Fatalf("expected invalid list request not to reach repository")
	}
}

func TestUpdateTeamRejectsEmptyName(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.UpdateTeam(context.Background(), UpdateTeamRequest{
		TenantID: uuid.New(),
		TeamID:   uuid.New(),
		Slug:     "ops",
		Name:     "   ",
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for empty name, got %v", err)
	}
	if repo.updateTeamCalled {
		t.Fatalf("expected invalid update request not to reach repository")
	}
}

func TestUpdateTeamPreservesOwnerAndMetadataWhenOmitted(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "ops",
		Name:             "Ops",
		HumanOwnerUserID: &ownerID,
		Metadata:         map[string]any{"cost_center": "ops"},
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	updated, err := svc.UpdateTeam(context.Background(), UpdateTeamRequest{
		TenantID: tenantID,
		TeamID:   team.ID,
		Slug:     "platform-ops",
		Name:     "Platform Ops",
	})
	if err != nil {
		t.Fatalf("update team: %v", err)
	}

	if updated.HumanOwnerUserID == nil || *updated.HumanOwnerUserID != ownerID {
		t.Fatalf("expected owner to be preserved, got %#v", updated.HumanOwnerUserID)
	}
	if updated.Metadata["cost_center"] != "ops" {
		t.Fatalf("expected metadata to be preserved, got %#v", updated.Metadata)
	}
}

func TestChangeTeamStatusRejectsInvalidStatus(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.ChangeTeamStatus(context.Background(), ChangeTeamStatusRequest{
		TenantID: uuid.New(),
		TeamID:   uuid.New(),
		Status:   TeamStatus("paused"),
	})

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input for unknown status, got %v", err)
	}
	if repo.setTeamStatusCalled {
		t.Fatalf("expected invalid status request not to reach repository")
	}
}

func TestGetOverviewUsesTeamSummaryAggregate(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	team, err := svc.CreateTeam(context.Background(), CreateTeamRequest{
		TenantID:         tenantID,
		Slug:             "ops",
		Name:             "Ops",
		HumanOwnerUserID: &ownerID,
	})
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	repo.teamSummaries[team.ID] = TeamListItemRecord{
		Team:                 *team,
		MemberCount:          18,
		DigitalEmployeeCount: 6,
		CapabilityCount:      12,
		PendingDraftCount:    3,
	}

	overview, err := svc.GetOverview(context.Background(), tenantID, team.ID)
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}

	if !repo.getTeamSummaryCalled {
		t.Fatalf("expected overview to use team-scoped summary aggregate")
	}
	if repo.listTeamSummariesCalled {
		t.Fatalf("expected overview not to use paginated summary list")
	}
	if overview.MemberCount != 18 || overview.DigitalEmployeeCount != 6 || overview.CapabilityCount != 12 || overview.PendingItemCount != 3 {
		t.Fatalf("unexpected overview counts: %#v", overview)
	}
}

func TestListTeamSummariesDefaultsLimit(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = svc.ListTeamSummaries(context.Background(), ListTeamsRequest{
		TenantID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("list team summaries: %v", err)
	}

	if !repo.listTeamSummariesCalled {
		t.Fatalf("expected list summary request to reach repository")
	}
	if repo.lastListTeamSummariesParams.Limit != 50 {
		t.Fatalf("expected default limit 50, got %d", repo.lastListTeamSummariesParams.Limit)
	}
}

type memoryRepository struct {
	teams                       map[uuid.UUID]TeamRecord
	teamSummaries               map[uuid.UUID]TeamListItemRecord
	revisions                   map[uuid.UUID]TeamConfigRevisionRecord
	createTeamCalled            bool
	listTeamsCalled             bool
	listTeamSummariesCalled     bool
	getTeamSummaryCalled        bool
	updateTeamCalled            bool
	setTeamStatusCalled         bool
	lastListTeamSummariesParams ListTeamSummariesParams
	createRevisionCalls         int
	createdRevision             CreateTeamConfigRevisionParams
	approveRevisionErr          error
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		teams:         map[uuid.UUID]TeamRecord{},
		teamSummaries: map[uuid.UUID]TeamListItemRecord{},
		revisions:     map[uuid.UUID]TeamConfigRevisionRecord{},
	}
}

func (r *memoryRepository) CreateTeam(_ context.Context, params CreateTeamParams) (TeamRecord, error) {
	r.createTeamCalled = true
	now := time.Now().UTC()
	record := TeamRecord{
		ID:               uuid.New(),
		TenantID:         params.TenantID,
		Slug:             params.Slug,
		Name:             params.Name,
		Status:           params.Status,
		HumanOwnerUserID: params.HumanOwnerUserID,
		Metadata:         cloneMap(params.Metadata),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	r.teams[record.ID] = record
	return record, nil
}

func (r *memoryRepository) ListTeams(_ context.Context, params ListTeamsParams) ([]TeamRecord, error) {
	r.listTeamsCalled = true
	records := make([]TeamRecord, 0, len(r.teams))
	for _, record := range r.teams {
		if record.TenantID == params.TenantID {
			records = append(records, record)
		}
	}
	return records, nil
}

func (r *memoryRepository) ListTeamSummaries(_ context.Context, params ListTeamSummariesParams) ([]TeamListItemRecord, error) {
	r.listTeamSummariesCalled = true
	r.lastListTeamSummariesParams = params
	records := make([]TeamListItemRecord, 0, len(r.teams))
	for _, record := range r.teams {
		if record.TenantID == params.TenantID {
			records = append(records, TeamListItemRecord{Team: record})
		}
	}
	return records, nil
}

func (r *memoryRepository) GetTeamSummary(_ context.Context, tenantID, teamID uuid.UUID) (TeamListItemRecord, error) {
	r.getTeamSummaryCalled = true
	if record, ok := r.teamSummaries[teamID]; ok && record.TenantID == tenantID {
		return record, nil
	}
	record, ok := r.teams[teamID]
	if !ok || record.TenantID != tenantID {
		return TeamListItemRecord{}, ErrNotFound
	}
	return TeamListItemRecord{Team: record}, nil
}

func (r *memoryRepository) GetTeam(_ context.Context, tenantID, teamID uuid.UUID) (TeamRecord, error) {
	record, ok := r.teams[teamID]
	if !ok || record.TenantID != tenantID {
		return TeamRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) UpdateTeam(_ context.Context, params UpdateTeamParams) (TeamRecord, error) {
	r.updateTeamCalled = true
	record, ok := r.teams[params.TeamID]
	if !ok || record.TenantID != params.TenantID {
		return TeamRecord{}, ErrNotFound
	}
	record.Slug = params.Slug
	record.Name = params.Name
	record.HumanOwnerUserID = params.HumanOwnerUserID
	record.Metadata = cloneMap(params.Metadata)
	record.UpdatedAt = time.Now().UTC()
	r.teams[record.ID] = record
	return record, nil
}

func (r *memoryRepository) SetTeamStatus(_ context.Context, params SetTeamStatusParams) (TeamRecord, error) {
	r.setTeamStatusCalled = true
	record, ok := r.teams[params.TeamID]
	if !ok || record.TenantID != params.TenantID {
		return TeamRecord{}, ErrNotFound
	}
	record.Status = params.Status
	record.UpdatedAt = time.Now().UTC()
	r.teams[record.ID] = record
	return record, nil
}

func (r *memoryRepository) CreateTeamConfigRevision(_ context.Context, params CreateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error) {
	r.createRevisionCalls++
	r.createdRevision = params
	now := time.Now().UTC()
	record := TeamConfigRevisionRecord{
		ID:                          uuid.New(),
		TenantID:                    params.TenantID,
		TeamID:                      params.TeamID,
		RevisionNumber:              params.RevisionNumber,
		Constitution:                cloneMap(params.Constitution),
		CapabilityPolicy:            cloneMap(params.CapabilityPolicy),
		ContextPolicy:               cloneMap(params.ContextPolicy),
		ApprovalPolicy:              cloneMap(params.ApprovalPolicy),
		ArtifactContract:            cloneMap(params.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(params.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(params.RuntimeScopePolicy),
		HumanOwnerUserID:            params.HumanOwnerUserID,
		Status:                      params.Status,
		ApprovedBy:                  params.ApprovedBy,
		ApprovedAt:                  cloneTimePtr(params.ApprovedAt),
		CreatedAt:                   now,
		UpdatedAt:                   now,
	}
	r.revisions[record.ID] = record
	return record, nil
}

func (r *memoryRepository) GetTeamConfigRevision(_ context.Context, tenantID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error) {
	record, ok := r.revisions[revisionID]
	if !ok || record.TenantID != tenantID {
		return TeamConfigRevisionRecord{}, ErrNotFound
	}
	return record, nil
}

func (r *memoryRepository) GetCurrentTeamConfigRevision(_ context.Context, tenantID, teamID uuid.UUID) (TeamConfigRevisionRecord, error) {
	for _, record := range r.revisions {
		if record.TenantID == tenantID && record.TeamID == teamID && record.Status == TeamConfigRevisionStatusActive {
			return record, nil
		}
	}
	return TeamConfigRevisionRecord{}, ErrNotFound
}

func (r *memoryRepository) GetNextTeamConfigRevisionNumber(_ context.Context, tenantID, teamID uuid.UUID) (int32, error) {
	next := int32(1)
	for _, record := range r.revisions {
		if record.TenantID == tenantID && record.TeamID == teamID && record.RevisionNumber >= next {
			next = record.RevisionNumber + 1
		}
	}
	return next, nil
}

func (r *memoryRepository) ListTeamConfigDrafts(_ context.Context, params ListTeamConfigDraftsParams) ([]TeamConfigRevisionRecord, error) {
	records := make([]TeamConfigRevisionRecord, 0, len(r.revisions))
	for _, record := range r.revisions {
		if record.TenantID == params.TenantID && record.TeamID == params.TeamID && record.Status == TeamConfigRevisionStatusDraft {
			records = append(records, cloneRevisionRecord(record))
		}
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].RevisionNumber > records[j].RevisionNumber
	})
	start := int(params.Offset)
	if start >= len(records) {
		return []TeamConfigRevisionRecord{}, nil
	}
	end := start + int(params.Limit)
	if end > len(records) {
		end = len(records)
	}
	return records[start:end], nil
}

func (r *memoryRepository) UpdateTeamConfigRevisionDraft(_ context.Context, params UpdateTeamConfigRevisionDraftParams) (TeamConfigRevisionRecord, error) {
	record, ok := r.revisions[params.RevisionID]
	if !ok || record.TenantID != params.TenantID || record.TeamID != params.TeamID || record.Status != TeamConfigRevisionStatusDraft {
		return TeamConfigRevisionRecord{}, ErrNotFound
	}
	if params.Constitution != nil {
		record.Constitution = cloneMap(params.Constitution)
	}
	if params.CapabilityPolicy != nil {
		record.CapabilityPolicy = cloneMap(params.CapabilityPolicy)
	}
	if params.ContextPolicy != nil {
		record.ContextPolicy = cloneMap(params.ContextPolicy)
	}
	if params.ApprovalPolicy != nil {
		record.ApprovalPolicy = cloneMap(params.ApprovalPolicy)
	}
	if params.ArtifactContract != nil {
		record.ArtifactContract = cloneMap(params.ArtifactContract)
	}
	if params.InternalCollaborationPolicy != nil {
		record.InternalCollaborationPolicy = cloneMap(params.InternalCollaborationPolicy)
	}
	if params.RuntimeScopePolicy != nil {
		record.RuntimeScopePolicy = cloneMap(params.RuntimeScopePolicy)
	}
	if params.HumanOwnerUserID != nil {
		record.HumanOwnerUserID = params.HumanOwnerUserID
	}
	record.UpdatedAt = time.Now().UTC()
	r.revisions[record.ID] = record
	return cloneRevisionRecord(record), nil
}

func (r *memoryRepository) ApproveTeamConfigRevision(_ context.Context, params ActivateTeamConfigRevisionParams) (TeamConfigRevisionRecord, error) {
	if r.approveRevisionErr != nil {
		return TeamConfigRevisionRecord{}, r.approveRevisionErr
	}
	record, ok := r.revisions[params.RevisionID]
	if !ok || record.TenantID != params.TenantID || record.TeamID != params.TeamID || record.Status != TeamConfigRevisionStatusDraft {
		return TeamConfigRevisionRecord{}, ErrNotFound
	}
	for id, active := range r.revisions {
		if active.TenantID == params.TenantID && active.TeamID == params.TeamID && active.Status == TeamConfigRevisionStatusActive {
			active.Status = TeamConfigRevisionStatusArchived
			active.UpdatedAt = time.Now().UTC()
			r.revisions[id] = active
		}
	}
	now := time.Now().UTC()
	approvedBy := params.ApprovedBy
	record.Status = TeamConfigRevisionStatusActive
	record.ApprovedBy = &approvedBy
	record.ApprovedAt = &now
	record.UpdatedAt = now
	r.revisions[record.ID] = record
	return cloneRevisionRecord(record), nil
}

func (r *memoryRepository) RejectTeamConfigRevision(_ context.Context, tenantID, teamID, revisionID uuid.UUID) (TeamConfigRevisionRecord, error) {
	record, ok := r.revisions[revisionID]
	if !ok || record.TenantID != tenantID || record.TeamID != teamID || record.Status != TeamConfigRevisionStatusDraft {
		return TeamConfigRevisionRecord{}, ErrNotFound
	}
	record.Status = TeamConfigRevisionStatusRejected
	record.UpdatedAt = time.Now().UTC()
	r.revisions[record.ID] = record
	return cloneRevisionRecord(record), nil
}

func cloneRevisionRecord(record TeamConfigRevisionRecord) TeamConfigRevisionRecord {
	record.Constitution = cloneMap(record.Constitution)
	record.CapabilityPolicy = cloneMap(record.CapabilityPolicy)
	record.ContextPolicy = cloneMap(record.ContextPolicy)
	record.ApprovalPolicy = cloneMap(record.ApprovalPolicy)
	record.ArtifactContract = cloneMap(record.ArtifactContract)
	record.InternalCollaborationPolicy = cloneMap(record.InternalCollaborationPolicy)
	record.RuntimeScopePolicy = cloneMap(record.RuntimeScopePolicy)
	record.HumanOwnerUserID = validUUIDPtr(record.HumanOwnerUserID)
	record.ApprovedBy = validUUIDPtr(record.ApprovedBy)
	record.ApprovedAt = cloneTimePtr(record.ApprovedAt)
	return record
}
