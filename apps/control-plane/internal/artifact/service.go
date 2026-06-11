package artifact

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

const (
	projectArchiveHoldType = "project_archive_hold"
	projectResourceType    = "project"
)

var ErrInvalidRetentionHold = errors.New("invalid artifact retention hold")

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("artifact repository is required")
	}
	return &Service{repository: repository}, nil
}

type HoldProjectArchiveArtifactsRequest struct {
	TenantID    uuid.UUID
	ProjectID   uuid.UUID
	ArtifactIDs []uuid.UUID
	Reason      string
}

type HoldProjectArchiveArtifactsResult struct {
	HoldIDs     []uuid.UUID
	ArtifactIDs []uuid.UUID
}

func (s *Service) HoldProjectArchiveArtifacts(ctx context.Context, req HoldProjectArchiveArtifactsRequest) (HoldProjectArchiveArtifactsResult, error) {
	if req.TenantID == uuid.Nil {
		return HoldProjectArchiveArtifactsResult{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidRetentionHold)
	}
	if req.ProjectID == uuid.Nil {
		return HoldProjectArchiveArtifactsResult{}, fmt.Errorf("%w: project_id is required", ErrInvalidRetentionHold)
	}
	if len(req.ArtifactIDs) == 0 {
		return HoldProjectArchiveArtifactsResult{}, fmt.Errorf("%w: artifact_ids is required", ErrInvalidRetentionHold)
	}

	seen := make(map[uuid.UUID]struct{}, len(req.ArtifactIDs))
	artifactIDs := make([]uuid.UUID, 0, len(req.ArtifactIDs))
	for _, artifactID := range req.ArtifactIDs {
		if artifactID == uuid.Nil {
			return HoldProjectArchiveArtifactsResult{}, fmt.Errorf("%w: artifact_id is required", ErrInvalidRetentionHold)
		}
		if _, ok := seen[artifactID]; ok {
			continue
		}
		seen[artifactID] = struct{}{}
		artifactIDs = append(artifactIDs, artifactID)
	}

	result := HoldProjectArchiveArtifactsResult{
		HoldIDs:     make([]uuid.UUID, 0, len(artifactIDs)),
		ArtifactIDs: make([]uuid.UUID, 0, len(artifactIDs)),
	}
	for _, artifactID := range artifactIDs {
		hold, err := s.repository.CreateRetentionHold(ctx, CreateRetentionHoldRequest{
			TenantID:     req.TenantID,
			ArtifactID:   artifactID,
			HoldType:     projectArchiveHoldType,
			ResourceType: projectResourceType,
			ResourceID:   req.ProjectID,
			Reason:       req.Reason,
		})
		if err != nil {
			return HoldProjectArchiveArtifactsResult{}, err
		}
		result.HoldIDs = append(result.HoldIDs, hold.ID)
		result.ArtifactIDs = append(result.ArtifactIDs, artifactID)
	}
	return result, nil
}

func (s *Service) CanDeleteArtifact(ctx context.Context, tenantID, artifactID uuid.UUID) (bool, error) {
	if tenantID == uuid.Nil {
		return false, fmt.Errorf("%w: tenant_id is required", ErrInvalidRetentionHold)
	}
	if artifactID == uuid.Nil {
		return false, fmt.Errorf("%w: artifact_id is required", ErrInvalidRetentionHold)
	}
	count, err := s.repository.CountActiveRetentionHolds(ctx, tenantID, artifactID)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}
