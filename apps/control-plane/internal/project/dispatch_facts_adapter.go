package project

import (
	"context"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/employee"
)

// ProjectDispatchFactsAdapter supplies project name + workspace ready status
// for run dispatch metadata and gates (stable CWD needs project_name).
type ProjectDispatchFactsAdapter struct {
	service *Service
}

func NewProjectDispatchFactsAdapter(service *Service) employee.ProjectDispatchFactsReader {
	if service == nil {
		return nil
	}
	return ProjectDispatchFactsAdapter{service: service}
}

func (a ProjectDispatchFactsAdapter) GetProjectDispatchFacts(ctx context.Context, tenantID, projectID uuid.UUID) (employee.ProjectDispatchFacts, error) {
	project, err := a.service.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return employee.ProjectDispatchFacts{}, err
	}
	return employee.ProjectDispatchFacts{
		Name:                 project.WorkspaceDirectoryName(),
		WorkspaceReadyStatus: string(project.WorkspaceReadyStatus),
	}, nil
}
