package artifact

import (
	"context"
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

func TestCanDeleteArtifactReturnsFalseWhenActiveHoldExists(t *testing.T) {
	artifactID := uuid.New()
	repo := &memoryRepository{activeCounts: map[uuid.UUID]int32{artifactID: 1}}
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	canDelete, err := service.CanDeleteArtifact(context.Background(), uuid.New(), artifactID)
	if err != nil {
		t.Fatalf("can delete artifact: %v", err)
	}
	if canDelete {
		t.Fatal("expected active hold to prevent deletion")
	}
}

type memoryRepository struct {
	activeCounts map[uuid.UUID]int32
}

func (r *memoryRepository) CreateRetentionHold(_ context.Context, req CreateRetentionHoldRequest) (RetentionHold, error) {
	hold := RetentionHold{
		ID:         uuid.New(),
		TenantID:   req.TenantID,
		ArtifactID: req.ArtifactID,
		Status:     "active",
	}
	r.activeCounts[req.ArtifactID]++
	return hold, nil
}

func (r *memoryRepository) CountActiveRetentionHolds(_ context.Context, _ uuid.UUID, artifactID uuid.UUID) (int32, error) {
	return r.activeCounts[artifactID], nil
}
