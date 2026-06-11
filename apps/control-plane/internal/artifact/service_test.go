package artifact

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestNewServiceRequiresRepository(t *testing.T) {
	if _, err := NewService(nil); err == nil {
		t.Fatal("expected nil repository to fail")
	}
}

func TestNewServiceAcceptsRepository(t *testing.T) {
	service, err := NewService(&memoryRepository{activeCounts: map[uuid.UUID]int32{}})
	if err != nil {
		t.Fatalf("expected service: %v", err)
	}
	if service == nil {
		t.Fatal("expected service")
	}
}

func TestProjectArchiveHoldCreatesOneActiveHoldPerArtifact(t *testing.T) {
	repo := &memoryRepository{activeCounts: map[uuid.UUID]int32{}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	artifactA := uuid.New()
	artifactB := uuid.New()

	result, err := service.HoldProjectArchiveArtifacts(context.Background(), HoldProjectArchiveArtifactsRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ArtifactIDs: []uuid.UUID{artifactA, artifactB},
		Reason:      "项目归档保留证据工件",
	})
	if err != nil {
		t.Fatalf("hold artifacts: %v", err)
	}
	if len(result.HoldIDs) != 2 || len(result.ArtifactIDs) != 2 {
		t.Fatalf("unexpected hold result: %#v", result)
	}
	if repo.activeCounts[artifactA] != 1 || repo.activeCounts[artifactB] != 1 {
		t.Fatalf("expected active holds for both artifacts, got %#v", repo.activeCounts)
	}
}

func TestProjectArchiveHoldDeduplicatesArtifactIDs(t *testing.T) {
	repo := &memoryRepository{activeCounts: map[uuid.UUID]int32{}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	artifactID := uuid.New()

	result, err := service.HoldProjectArchiveArtifacts(context.Background(), HoldProjectArchiveArtifactsRequest{
		TenantID:    uuid.New(),
		ProjectID:   uuid.New(),
		ArtifactIDs: []uuid.UUID{artifactID, artifactID},
		Reason:      "项目归档保留证据工件",
	})
	if err != nil {
		t.Fatalf("hold artifacts: %v", err)
	}
	if len(result.HoldIDs) != 1 || len(result.ArtifactIDs) != 1 {
		t.Fatalf("unexpected deduplicated hold result: %#v", result)
	}
	if repo.activeCounts[artifactID] != 1 {
		t.Fatalf("expected one active hold, got %#v", repo.activeCounts)
	}
}

func TestProjectArchiveHoldRejectsInvalidIDs(t *testing.T) {
	validTenantID := uuid.New()
	validProjectID := uuid.New()
	validArtifactID := uuid.New()
	tests := []struct {
		name string
		req  HoldProjectArchiveArtifactsRequest
	}{
		{
			name: "missing tenant",
			req: HoldProjectArchiveArtifactsRequest{
				ProjectID:   validProjectID,
				ArtifactIDs: []uuid.UUID{validArtifactID},
			},
		},
		{
			name: "missing project",
			req: HoldProjectArchiveArtifactsRequest{
				TenantID:    validTenantID,
				ArtifactIDs: []uuid.UUID{validArtifactID},
			},
		},
		{
			name: "empty artifacts",
			req: HoldProjectArchiveArtifactsRequest{
				TenantID:    validTenantID,
				ProjectID:   validProjectID,
				ArtifactIDs: []uuid.UUID{},
			},
		},
		{
			name: "missing artifact",
			req: HoldProjectArchiveArtifactsRequest{
				TenantID:    validTenantID,
				ProjectID:   validProjectID,
				ArtifactIDs: []uuid.UUID{uuid.Nil},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &memoryRepository{activeCounts: map[uuid.UUID]int32{}}
			service, err := NewService(repo)
			if err != nil {
				t.Fatalf("new service: %v", err)
			}

			if _, err := service.HoldProjectArchiveArtifacts(context.Background(), tt.req); !errors.Is(err, ErrInvalidRetentionHold) {
				t.Fatalf("expected ErrInvalidRetentionHold, got %v", err)
			}
			if len(repo.createdRequests) != 0 {
				t.Fatalf("expected invalid request to skip repository, got %#v", repo.createdRequests)
			}
		})
	}
}

func TestProjectArchiveHoldSendsRetentionHoldFields(t *testing.T) {
	repo := &memoryRepository{activeCounts: map[uuid.UUID]int32{}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	artifactID := uuid.New()
	reason := "项目归档保留证据工件"

	if _, err := service.HoldProjectArchiveArtifacts(context.Background(), HoldProjectArchiveArtifactsRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ArtifactIDs: []uuid.UUID{artifactID},
		Reason:      reason,
	}); err != nil {
		t.Fatalf("hold artifacts: %v", err)
	}
	if len(repo.createdRequests) != 1 {
		t.Fatalf("expected one retention hold request, got %#v", repo.createdRequests)
	}

	req := repo.createdRequests[0]
	if req.TenantID != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, req.TenantID)
	}
	if req.ArtifactID != artifactID {
		t.Fatalf("expected artifact %s, got %s", artifactID, req.ArtifactID)
	}
	if req.HoldType != projectArchiveHoldType {
		t.Fatalf("expected hold type %q, got %q", projectArchiveHoldType, req.HoldType)
	}
	if req.ResourceType != projectResourceType {
		t.Fatalf("expected resource type %q, got %q", projectResourceType, req.ResourceType)
	}
	if req.ResourceID != projectID {
		t.Fatalf("expected resource ID %s, got %s", projectID, req.ResourceID)
	}
	if req.Reason != reason {
		t.Fatalf("expected reason %q, got %q", reason, req.Reason)
	}
	if req.CreatedEventID != nil {
		t.Fatalf("expected no created event ID, got %#v", req.CreatedEventID)
	}
}

func TestCanDeleteArtifactReflectsActiveHoldCount(t *testing.T) {
	artifactWithoutHold := uuid.New()
	artifactWithHold := uuid.New()
	repo := &memoryRepository{
		activeCounts: map[uuid.UUID]int32{
			artifactWithHold: 2,
		},
	}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	canDelete, err := service.CanDeleteArtifact(context.Background(), uuid.New(), artifactWithoutHold)
	if err != nil {
		t.Fatalf("can delete artifact without hold: %v", err)
	}
	if !canDelete {
		t.Fatal("expected artifact without active holds to be deletable")
	}

	canDelete, err = service.CanDeleteArtifact(context.Background(), uuid.New(), artifactWithHold)
	if err != nil {
		t.Fatalf("can delete artifact with hold: %v", err)
	}
	if canDelete {
		t.Fatal("expected active hold to prevent deletion")
	}
}

func TestCanDeleteArtifactRejectsInvalidIDs(t *testing.T) {
	repo := &memoryRepository{activeCounts: map[uuid.UUID]int32{}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if _, err := service.CanDeleteArtifact(context.Background(), uuid.Nil, uuid.New()); !errors.Is(err, ErrInvalidRetentionHold) {
		t.Fatalf("expected missing tenant to return ErrInvalidRetentionHold, got %v", err)
	}
	if _, err := service.CanDeleteArtifact(context.Background(), uuid.New(), uuid.Nil); !errors.Is(err, ErrInvalidRetentionHold) {
		t.Fatalf("expected missing artifact to return ErrInvalidRetentionHold, got %v", err)
	}
}

type memoryRepository struct {
	activeCounts    map[uuid.UUID]int32
	createdRequests []CreateRetentionHoldRequest
}

func (r *memoryRepository) CreateRetentionHold(_ context.Context, req CreateRetentionHoldRequest) (RetentionHold, error) {
	hold := RetentionHold{
		ID:         uuid.New(),
		TenantID:   req.TenantID,
		ArtifactID: req.ArtifactID,
		Status:     "active",
	}
	r.createdRequests = append(r.createdRequests, req)
	r.activeCounts[req.ArtifactID]++
	return hold, nil
}

func (r *memoryRepository) CountActiveRetentionHolds(_ context.Context, _ uuid.UUID, artifactID uuid.UUID) (int32, error) {
	return r.activeCounts[artifactID], nil
}
