package projectcoordination

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

func TestProjectStoreSnapshotIncludesOnlyActiveDigitalExecutorsAndReviewers(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	executorID := uuid.New()
	reviewerID := uuid.New()
	observerID := uuid.New()
	inactiveExecutorID := uuid.New()
	humanID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:                 projectID,
			TenantID:           tenantID,
			CoordinationPolicy: map[string]any{"mode": "balanced"},
		},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "补齐验收证据",
			Content:   strPtr("整理日志并给出结论"),
		},
		members: []project.ProjectMember{
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: executorID, ProjectRole: project.ProjectRoleExecutor, Status: "active", DisplayNameSnapshot: strPtr("执行员工")},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: reviewerID, ProjectRole: project.ProjectRoleReviewer, Status: "active", DisplayNameSnapshot: strPtr("复核员工")},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: observerID, ProjectRole: project.ProjectRoleObserver, Status: "active"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeDigitalEmployee, PrincipalID: inactiveExecutorID, ProjectRole: project.ProjectRoleExecutor, Status: "inactive"},
			{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, PrincipalType: project.PrincipalTypeHumanUser, PrincipalID: humanID, ProjectRole: project.ProjectRoleOwner, Status: "active"},
		},
	}
	store := NewProjectStore(repo)

	snapshot, err := store.LoadProjectCoordinationSnapshot(context.Background(), LoadSnapshotInput{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	if len(snapshot.DigitalEmployeePool) != 2 {
		t.Fatalf("expected executor and reviewer only, got %#v", snapshot.DigitalEmployeePool)
	}
	if snapshot.DigitalEmployeePool[0].PrincipalID != executorID || snapshot.DigitalEmployeePool[1].PrincipalID != reviewerID {
		t.Fatalf("unexpected employee pool: %#v", snapshot.DigitalEmployeePool)
	}
}

type projectStoreMemoryRepository struct {
	project.Repository

	projectRecord project.Project
	demand        project.ProjectDemand
	members       []project.ProjectMember
}

func (r *projectStoreMemoryRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	if r.projectRecord.TenantID == tenantID && r.projectRecord.ID == projectID {
		return r.projectRecord, nil
	}
	return project.Project{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (project.ProjectDemand, error) {
	if r.demand.TenantID == tenantID && r.demand.ID == demandID {
		return r.demand, nil
	}
	return project.ProjectDemand{}, project.ErrProjectNotFound
}

func (r *projectStoreMemoryRepository) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]project.ProjectMember, error) {
	members := make([]project.ProjectMember, 0, len(r.members))
	for _, member := range r.members {
		if member.TenantID == tenantID && member.ProjectID == projectID {
			members = append(members, member)
		}
	}
	return members, nil
}

func (r *projectStoreMemoryRepository) CreateCoordinationJob(ctx context.Context, req project.CreateCoordinationJobRequest) (project.CoordinationJob, error) {
	return project.CoordinationJob{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, WorkflowID: req.WorkflowID, Status: req.Status, CreatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) AppendProjectEvent(ctx context.Context, req project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	return project.ProjectEvent{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, EventType: req.EventType, CreatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) CreateRouteDecision(ctx context.Context, req project.CreateRouteDecisionRequest) (project.RouteDecision, error) {
	return project.RouteDecision{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, CoordinationJobID: req.CoordinationJobID, DemandID: req.DemandID, CreatedEventID: req.CreatedEventID, CreatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) CreateProjectTask(ctx context.Context, req project.CreateProjectTaskRequest) (project.ProjectTask, error) {
	return project.ProjectTask{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, DemandID: req.DemandID, Status: req.Status, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID) (project.ProjectTask, error) {
	return project.ProjectTask{ID: projectTaskID, TenantID: tenantID, Status: status, UpdatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) FinishCoordinationJob(ctx context.Context, req project.FinishCoordinationJobRequest) (project.CoordinationJob, error) {
	return project.CoordinationJob{ID: req.ID, TenantID: req.TenantID, Status: req.Status, OutputEventIDs: req.OutputEventIDs, CreatedAt: time.Now().UTC()}, nil
}

func strPtr(value string) *string {
	return &value
}
