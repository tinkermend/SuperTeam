package tenant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/audit"
)

func TestTeamStatusAllowsArchived(t *testing.T) {
	if !TeamStatusArchived.IsValid() {
		t.Fatalf("expected archived team status to be valid")
	}
	if TeamStatus("paused").IsValid() {
		t.Fatalf("expected unknown team status to be invalid")
	}
}

func TestNewServiceRequiresTeamAuditReader(t *testing.T) {
	repo := newMemoryRepository()
	if _, err := NewService(repo, nil); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected missing team audit reader to fail with invalid input, got %v", err)
	}
	if _, err := NewService(repo, &fakeTeamAuditReader{}); err != nil {
		t.Fatalf("expected service with team audit reader: %v", err)
	}
}

func TestCreateTeamDefaultsActiveStatus(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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

func TestListTeamsRejectsNegativeOffset(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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
	svc, err := NewServiceWithoutAuditForTest(repo)
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

type fakeTeamAuditReader struct{}

func (r *fakeTeamAuditReader) ListTeamEvents(ctx context.Context, tenantID, teamID uuid.UUID, limit, offset int) ([]*audit.Event, error) {
	return []*audit.Event{}, nil
}
