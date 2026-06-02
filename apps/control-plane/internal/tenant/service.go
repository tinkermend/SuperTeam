package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: repository is required", ErrInvalidInput)
	}
	return &Service{repository: repository}, nil
}

func (s *Service) CreateTeam(ctx context.Context, req CreateTeamRequest) (*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		return nil, fmt.Errorf("%w: slug is required", ErrInvalidInput)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = TeamStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}

	record, err := s.repository.CreateTeam(ctx, CreateTeamParams{
		TenantID:         req.TenantID,
		Slug:             slug,
		Name:             name,
		Status:           status,
		HumanOwnerUserID: validUUIDPtr(req.HumanOwnerUserID),
		Metadata:         cloneMap(req.Metadata),
	})
	if err != nil {
		return nil, fmt.Errorf("create team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) ListTeams(ctx context.Context, req ListTeamsRequest) ([]*Team, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.Status != "" && !req.Status.IsValid() {
		return nil, fmt.Errorf("%w: invalid team status", ErrInvalidInput)
	}
	if req.Offset < 0 {
		return nil, fmt.Errorf("%w: offset must be non-negative", ErrInvalidInput)
	}
	if req.Limit < 0 {
		return nil, fmt.Errorf("%w: limit must be non-negative", ErrInvalidInput)
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	records, err := s.repository.ListTeams(ctx, ListTeamsParams{
		TenantID: req.TenantID,
		Status:   req.Status,
		Offset:   req.Offset,
		Limit:    req.Limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list teams: %w", err)
	}
	teams := make([]*Team, 0, len(records))
	for _, record := range records {
		teams = append(teams, teamFromRecord(record))
	}
	return teams, nil
}

func (s *Service) GetTeam(ctx context.Context, tenantID, teamID uuid.UUID) (*Team, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetTeam(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	return teamFromRecord(record), nil
}

func (s *Service) CreateConfigRevision(ctx context.Context, req CreateTeamConfigRevisionRequest) (*TeamConfigRevision, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.TeamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	if req.HumanOwnerUserID == nil || *req.HumanOwnerUserID == uuid.Nil {
		return nil, fmt.Errorf("%w: human_owner_user_id is required", ErrInvalidInput)
	}
	status := req.Status
	if status == "" {
		status = TeamConfigRevisionStatusActive
	}
	if !status.IsValid() {
		return nil, fmt.Errorf("%w: invalid config revision status", ErrInvalidInput)
	}

	if _, err := s.repository.GetTeam(ctx, req.TenantID, req.TeamID); err != nil {
		return nil, fmt.Errorf("get team: %w", err)
	}
	if status == TeamConfigRevisionStatusActive {
		if _, err := s.repository.GetCurrentTeamConfigRevision(ctx, req.TenantID, req.TeamID); err == nil {
			return nil, fmt.Errorf("%w: active config revision already exists", ErrInvalidInput)
		} else if !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("get current team config revision: %w", err)
		}
	}

	nextRevision, err := s.repository.GetNextTeamConfigRevisionNumber(ctx, req.TenantID, req.TeamID)
	if err != nil {
		return nil, fmt.Errorf("get next team config revision number: %w", err)
	}
	approvedBy := validUUIDPtr(req.ApprovedBy)
	var approvedAt *time.Time
	if status == TeamConfigRevisionStatusActive {
		now := time.Now().UTC()
		approvedAt = &now
	} else {
		approvedBy = nil
	}
	record, err := s.repository.CreateTeamConfigRevision(ctx, CreateTeamConfigRevisionParams{
		TenantID:                    req.TenantID,
		TeamID:                      req.TeamID,
		RevisionNumber:              nextRevision,
		Constitution:                cloneMap(req.Constitution),
		CapabilityPolicy:            cloneMap(req.CapabilityPolicy),
		ContextPolicy:               cloneMap(req.ContextPolicy),
		ApprovalPolicy:              cloneMap(req.ApprovalPolicy),
		ArtifactContract:            cloneMap(req.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(req.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(req.RuntimeScopePolicy),
		HumanOwnerUserID:            validUUIDPtr(req.HumanOwnerUserID),
		Status:                      status,
		ApprovedBy:                  approvedBy,
		ApprovedAt:                  approvedAt,
	})
	if err != nil {
		return nil, fmt.Errorf("create team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) GetConfigRevision(ctx context.Context, tenantID, revisionID uuid.UUID) (*TeamConfigRevision, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if revisionID == uuid.Nil {
		return nil, fmt.Errorf("%w: config_revision_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetTeamConfigRevision(ctx, tenantID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("get team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func (s *Service) GetCurrentConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (*TeamConfigRevision, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if teamID == uuid.Nil {
		return nil, fmt.Errorf("%w: team_id is required", ErrInvalidInput)
	}
	record, err := s.repository.GetCurrentTeamConfigRevision(ctx, tenantID, teamID)
	if err != nil {
		return nil, fmt.Errorf("get current team config revision: %w", err)
	}
	return configRevisionFromRecord(record), nil
}

func teamFromRecord(record TeamRecord) *Team {
	return &Team{
		ID:               record.ID,
		TenantID:         record.TenantID,
		Slug:             record.Slug,
		Name:             record.Name,
		Status:           record.Status,
		HumanOwnerUserID: validUUIDPtr(record.HumanOwnerUserID),
		Metadata:         cloneMap(record.Metadata),
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
	}
}

func configRevisionFromRecord(record TeamConfigRevisionRecord) *TeamConfigRevision {
	return &TeamConfigRevision{
		ID:                          record.ID,
		TenantID:                    record.TenantID,
		TeamID:                      record.TeamID,
		RevisionNumber:              record.RevisionNumber,
		Constitution:                cloneMap(record.Constitution),
		CapabilityPolicy:            cloneMap(record.CapabilityPolicy),
		ContextPolicy:               cloneMap(record.ContextPolicy),
		ApprovalPolicy:              cloneMap(record.ApprovalPolicy),
		ArtifactContract:            cloneMap(record.ArtifactContract),
		InternalCollaborationPolicy: cloneMap(record.InternalCollaborationPolicy),
		RuntimeScopePolicy:          cloneMap(record.RuntimeScopePolicy),
		HumanOwnerUserID:            validUUIDPtr(record.HumanOwnerUserID),
		Status:                      record.Status,
		ApprovedBy:                  validUUIDPtr(record.ApprovedBy),
		ApprovedAt:                  cloneTimePtr(record.ApprovedAt),
		CreatedAt:                   record.CreatedAt,
		UpdatedAt:                   record.UpdatedAt,
	}
}

func validUUIDPtr(value *uuid.UUID) *uuid.UUID {
	if value == nil || *value == uuid.Nil {
		return nil
	}
	copied := *value
	return &copied
}

func cloneMap(value map[string]any) map[string]any {
	cloned := make(map[string]any)
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
