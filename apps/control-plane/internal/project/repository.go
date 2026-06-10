package project

import (
	"context"

	"github.com/google/uuid"
)

type Repository interface {
	CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error)
	GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error)
	ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error)
	UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error)
	ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error)
	ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error)
	ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error)
	ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error)
	AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error)
	ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error)
	CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error)
	ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error)
	CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error)
}

type AppendProjectEventRequest struct {
	TenantID     uuid.UUID
	ProjectID    uuid.UUID
	EventType    ProjectEventType
	ActorType    string
	ActorID      string
	ResourceType *string
	ResourceID   *string
	Summary      string
	Payload      map[string]any
}
