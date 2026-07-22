package automation

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// ProjectServiceGateway adapts project.Service for automation eligibility checks.
// isEligibleDecider is private on project.Service, so this duplicates the equal-status
// human decider set: active human members + human owners (IDs slice and primary).
type ProjectServiceGateway struct {
	projects *project.Service
}

func NewProjectServiceGateway(projects *project.Service) *ProjectServiceGateway {
	return &ProjectServiceGateway{projects: projects}
}

func (g *ProjectServiceGateway) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectInfo, error) {
	if g == nil || g.projects == nil {
		return ProjectInfo{}, ErrInvalidInput
	}
	record, err := g.projects.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return ProjectInfo{}, err
	}
	if record == nil {
		return ProjectInfo{}, ErrNotFound
	}
	info := ProjectInfo{
		ID:   record.ID,
		Name: record.Name,
	}
	if record.TeamID != nil {
		info.TeamID = *record.TeamID
	}
	return info, nil
}

func (g *ProjectServiceGateway) IsEligibleInitiator(ctx context.Context, tenantID, projectID, userID uuid.UUID) (bool, error) {
	if g == nil || g.projects == nil {
		return false, ErrInvalidInput
	}
	if userID == uuid.Nil {
		return false, nil
	}
	record, err := g.projects.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return false, err
	}
	if record == nil {
		return false, ErrNotFound
	}
	if userID == record.HumanOwnerUserID {
		return true, nil
	}
	for _, ownerID := range record.HumanOwnerUserIDs {
		if ownerID == userID {
			return true, nil
		}
	}
	members, err := g.projects.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return false, err
	}
	for _, member := range members {
		if member.PrincipalType == project.PrincipalTypeHumanUser && member.Status == "active" && member.PrincipalID == userID {
			return true, nil
		}
	}
	return false, nil
}
