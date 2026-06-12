package projectcoordination

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/approval"
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

func TestProjectStoreRequestRouteDecisionReviewCreatesApprovalAndDecisionProjection(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	jobID := uuid.New()
	demandID := uuid.New()
	routeID := uuid.New()
	taskID := uuid.New()
	employeeID := uuid.New()
	approvalID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			HumanOwnerUserID: ownerID,
		},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "需要人工确认",
		},
		approvalID: approvalID,
	}
	approvals := &projectStoreApprovalCreator{approvalID: approvalID}
	store := NewProjectStoreWithApprovals(repo, approvals)

	result, err := store.RequestRouteDecisionReview(context.Background(), RequestRouteDecisionReviewInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: jobID,
		DemandID:          demandID,
		RouteDecisionID:   routeID,
		Decision: RouteDecisionPlan{
			SelectedDigitalEmployeeIDs: []uuid.UUID{employeeID},
			Reason:                     "高风险需求需要负责人确认",
		},
		ProjectTaskIDs:      []uuid.UUID{taskID},
		RouteCreatedEventID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("request route review: %v", err)
	}
	if result.ID == uuid.Nil {
		t.Fatal("expected decision request id")
	}
	if approvals.last.TargetUserID != ownerID || approvals.last.ResourceID != routeID || approvals.last.DecisionType != "route_review" {
		t.Fatalf("unexpected approval request: %#v", approvals.last)
	}
	if approvals.last.ContextPayload["project_id"] != projectID.String() {
		t.Fatalf("expected project context payload, got %#v", approvals.last.ContextPayload)
	}
	if len(repo.events) != 1 || repo.events[0].EventType != project.ProjectEventDecisionRequested {
		t.Fatalf("expected decision requested event, got %#v", repo.events)
	}
	if len(repo.decisionRequests) != 1 {
		t.Fatalf("expected project decision projection, got %d", len(repo.decisionRequests))
	}
	decision := repo.decisionRequests[0]
	if decision.ApprovalRequestID != approvalID || decision.TargetUserID != ownerID || decision.StatusSnapshot != "pending" {
		t.Fatalf("unexpected decision projection: %#v", decision)
	}
}

func TestProjectStoreRequestRouteDecisionReviewTargetsDemandReviewerPreference(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	jobID := uuid.New()
	demandID := uuid.New()
	routeID := uuid.New()
	approvalID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			HumanOwnerUserID: ownerID,
		},
		demand: project.ProjectDemand{
			ID:        demandID,
			TenantID:  tenantID,
			ProjectID: projectID,
			Title:     "需要指定审核人确认",
			ReviewerPreference: &project.ReviewerPreference{
				ReviewerUserID:   reviewerID,
				SelectionReason:  project.ReviewerSelectionUserSelected,
				ProjectRole:      project.ProjectRoleReviewer,
				ResolvedFromRule: false,
			},
		},
		approvalID: approvalID,
	}
	approvals := &projectStoreApprovalCreator{approvalID: approvalID}
	store := NewProjectStoreWithApprovals(repo, approvals)

	_, err := store.RequestRouteDecisionReview(context.Background(), RequestRouteDecisionReviewInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: jobID,
		DemandID:          demandID,
		RouteDecisionID:   routeID,
		Decision: RouteDecisionPlan{
			Reason:              "风险动作需要指定审核人确认",
			RequiresHumanReview: true,
		},
		RouteCreatedEventID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("request route review: %v", err)
	}

	if approvals.last.TargetUserID != reviewerID {
		t.Fatalf("expected approval target reviewer, got %#v", approvals.last)
	}
	if len(repo.decisionRequests) != 1 || repo.decisionRequests[0].TargetUserID != reviewerID {
		t.Fatalf("expected decision request target reviewer, got %#v", repo.decisionRequests)
	}
	if len(repo.events) != 1 || repo.events[0].Payload["target_user_id"] != reviewerID.String() {
		t.Fatalf("expected target user event payload, got %#v", repo.events)
	}
}

type projectStoreMemoryRepository struct {
	project.Repository

	projectRecord project.Project
	demand        project.ProjectDemand
	members       []project.ProjectMember
	approvalID    uuid.UUID

	events           []project.ProjectEvent
	decisionRequests []project.DecisionRequest
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
	event := project.ProjectEvent{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, EventType: req.EventType, Payload: req.Payload, CreatedAt: time.Now().UTC()}
	r.events = append(r.events, event)
	return event, nil
}

func (r *projectStoreMemoryRepository) CreateRouteDecision(ctx context.Context, req project.CreateRouteDecisionRequest) (project.RouteDecision, error) {
	return project.RouteDecision{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, CoordinationJobID: req.CoordinationJobID, DemandID: req.DemandID, CreatedEventID: req.CreatedEventID, CreatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) CreateProjectTask(ctx context.Context, req project.CreateProjectTaskRequest) (project.ProjectTask, error) {
	return project.ProjectTask{ID: uuid.New(), TenantID: req.TenantID, ProjectID: req.ProjectID, DemandID: req.DemandID, Status: req.Status, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (project.ProjectTask, error) {
	return project.ProjectTask{ID: projectTaskID, TenantID: tenantID, Status: status, UpdatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) FinishCoordinationJob(ctx context.Context, req project.FinishCoordinationJobRequest) (project.CoordinationJob, error) {
	return project.CoordinationJob{ID: req.ID, TenantID: req.TenantID, Status: req.Status, OutputEventIDs: req.OutputEventIDs, CreatedAt: time.Now().UTC()}, nil
}

func (r *projectStoreMemoryRepository) CreateDecisionRequest(ctx context.Context, req project.CreateDecisionRequestRequest) (project.DecisionRequest, error) {
	decision := project.DecisionRequest{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: req.ApprovalRequestID,
		CoordinationJobID: req.CoordinationJobID,
		TargetUserID:      req.TargetUserID,
		DecisionType:      req.DecisionType,
		TitleSnapshot:     req.TitleSnapshot,
		StatusSnapshot:    req.StatusSnapshot,
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	r.decisionRequests = append(r.decisionRequests, decision)
	return decision, nil
}

type projectStoreApprovalCreator struct {
	approvalID uuid.UUID
	last       approval.CreateRequestInput
}

func (c *projectStoreApprovalCreator) CreateRequest(ctx context.Context, input approval.CreateRequestInput) (*approval.ApprovalRequest, error) {
	c.last = input
	id := c.approvalID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &approval.ApprovalRequest{
		ID:           id,
		TenantID:     input.TenantID,
		ResourceType: input.ResourceType,
		ResourceID:   input.ResourceID,
		TargetUserID: input.TargetUserID,
		DecisionType: input.DecisionType,
		Title:        input.Title,
		Status:       approval.ApprovalStatusPending,
	}, nil
}

func strPtr(value string) *string {
	return &value
}
