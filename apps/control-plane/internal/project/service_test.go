package project

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCreateProjectRequiresHumanOwnerAndCreatesEvents(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	ownerID := uuid.New()
	employeeID := uuid.New()

	created, err := service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      ownerID,
		Name:             "支付网关稳定性整改",
		Goal:             "修复超时链路并形成验收报告",
		HumanOwnerUserID: ownerID,
		Members: []ProjectMemberInput{
			{PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID, ProjectRole: ProjectRoleOwner, DisplayNameSnapshot: "王佩"},
			{PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: employeeID, ProjectRole: ProjectRoleExecutor, DisplayNameSnapshot: "后端执行 A", Settings: map[string]any{"concurrency_slots": float64(2)}},
		},
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.Project.Status != ProjectStatusRunning {
		t.Fatalf("expected running project, got %s", created.Project.Status)
	}
	if created.Project.CoordinationStatus != "registered" {
		t.Fatalf("expected registered coordination status, got %s", created.Project.CoordinationStatus)
	}
	if !strings.HasPrefix(created.Project.CoordinationWorkflowID, "project-coordinator:") {
		t.Fatalf("expected coordination workflow id, got %q", created.Project.CoordinationWorkflowID)
	}
	if repo.eventTypes[0] != ProjectEventCreated || repo.eventTypes[1] != ProjectEventConfigChanged {
		t.Fatalf("expected create/config events, got %#v", repo.eventTypes)
	}
	for _, member := range created.Members {
		if member.ProjectRole == ProjectRole("coordinator") {
			t.Fatal("coordinator must not be represented as a project member")
		}
	}
}

func TestCreateProjectRequiresMandatoryFields(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "缺少目标",
		HumanOwnerUserID: uuid.New(),
	})
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}
}

func TestCreateProjectRejectsCoordinatorMemberRole(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         uuid.New(),
		ActorUserID:      uuid.New(),
		Name:             "项目",
		Goal:             "目标",
		HumanOwnerUserID: uuid.New(),
		Members: []ProjectMemberInput{{
			PrincipalType: PrincipalTypeDigitalEmployee,
			PrincipalID:   uuid.New(),
			ProjectRole:   ProjectRole("coordinator"),
		}},
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid member error, got %v", err)
	}
}

func TestCreateProjectValidatesRolePrincipalTypes(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name          string
		principalType PrincipalType
		role          ProjectRole
	}{
		{name: "owner must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleOwner},
		{name: "leader must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleLeader},
		{name: "acceptance must be human", principalType: PrincipalTypeDigitalEmployee, role: ProjectRoleAcceptance},
		{name: "executor must be digital employee", principalType: PrincipalTypeHumanUser, role: ProjectRoleExecutor},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.CreateProject(context.Background(), CreateProjectRequest{
				TenantID:         uuid.New(),
				ActorUserID:      uuid.New(),
				Name:             "项目",
				Goal:             "目标",
				HumanOwnerUserID: uuid.New(),
				Members: []ProjectMemberInput{{
					PrincipalType: tc.principalType,
					PrincipalID:   uuid.New(),
					ProjectRole:   tc.role,
				}},
			})
			if !errors.Is(err, ErrInvalidProjectMember) {
				t.Fatalf("expected invalid member error, got %v", err)
			}
		})
	}
}

func TestSubmitDemandRecordsDemandAndEventWithoutAutoCreatingTask(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		Name:             "客户侧 Runtime 接入验收",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          repo.projects[projectID].TenantID,
		ProjectID:         projectID,
		SubmittedByUserID: uuid.New(),
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
		SourceType:        DemandSourceManual,
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("service must not create project tasks from demand directly")
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventDemandSubmitted {
		t.Fatalf("expected demand event only, got %#v", repo.eventTypes)
	}
}

func TestSubmitDemandSignalsProjectCoordinatorInV1(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	ownerID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "客户侧 Runtime 接入验收",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       ownerID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
		CoordinationStatus:     "registered",
	}

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: ownerID,
		Title:             "验证 Runtime 连接",
		Content:           "检查心跳和命令回写",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.Status != ProjectDemandStatusPlanningPending {
		t.Fatalf("expected planning pending demand, got %s", demand.Status)
	}
	if coordinator.demandSignals != 1 {
		t.Fatalf("expected one DemandSubmitted signal, got %d", coordinator.demandSignals)
	}
	if coordinator.lastDemand.DemandID != demand.ID || coordinator.lastDemand.CreatedEventID == uuid.Nil {
		t.Fatalf("unexpected demand signal: %#v", coordinator.lastDemand)
	}
}

func TestCompleteProjectTaskWritesSummaryAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	employeeID := uuid.New()
	taskID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       uuid.New(),
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.tasks = append(repo.tasks, ProjectTask{
		ID:                        taskID,
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		Title:                     "整理证据",
		Status:                    "assigned",
		AssignedDigitalEmployeeID: &employeeID,
	})

	summary, err := service.CompleteProjectTask(context.Background(), CompleteProjectTaskRequest{
		TenantID:              tenantID,
		ProjectTaskID:         taskID,
		DigitalEmployeeID:     employeeID,
		Conclusion:            "证据充分",
		EvidenceRefs:          []any{"s3://bucket/report.md"},
		ArtifactRefs:          []any{"artifact-1"},
		ConfidenceFactors:     map[string]any{"tests": "passed"},
		RecommendedNextAction: "提交负责人验收",
	})
	if err != nil {
		t.Fatalf("complete project task: %v", err)
	}
	if summary.ProjectTaskID != taskID || summary.DigitalEmployeeID != employeeID {
		t.Fatalf("unexpected summary: %#v", summary)
	}
	if repo.tasks[0].Status != "completed" {
		t.Fatalf("expected task completed, got %s", repo.tasks[0].Status)
	}
	if coordinator.completedSignals != 1 || coordinator.lastCompleted.ExecutionSummaryID != summary.ID {
		t.Fatalf("expected completed signal for summary, got count=%d signal=%#v", coordinator.completedSignals, coordinator.lastCompleted)
	}
	if repo.eventTypes[len(repo.eventTypes)-1] != ProjectEventTaskCompleted {
		t.Fatalf("expected completed event, got %#v", repo.eventTypes)
	}
}

func TestResolveDecisionUsesApprovalAndSignalsCoordinator(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	approvals := &fakeApprovalResolver{}
	service, err := NewServiceWithCoordinatorAndApprovals(repo, coordinator, approvals)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	approvalID := uuid.New()
	actorID := uuid.New()
	repo.projects[projectID] = Project{
		ID:                     projectID,
		TenantID:               tenantID,
		Name:                   "项目",
		Goal:                   "目标",
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       actorID,
		CoordinationWorkflowID: "project-coordinator:" + projectID.String(),
	}
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{
		ID:                decisionID,
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: approvalID,
		TargetUserID:      actorID,
		DecisionType:      "route_review",
		TitleSnapshot:     "需要负责人确认",
		StatusSnapshot:    "pending",
	})

	resolved, err := service.ResolveDecision(context.Background(), ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   actorID,
		Decision:          "approved",
		Comment:           "同意",
		Payload:           map[string]any{"source": "console"},
	})
	if err != nil {
		t.Fatalf("resolve decision: %v", err)
	}
	if resolved.StatusSnapshot != "approved" {
		t.Fatalf("expected approved projection, got %s", resolved.StatusSnapshot)
	}
	if approvals.calls != 1 || approvals.last.ApprovalRequestID != approvalID || approvals.last.Decision != "approved" {
		t.Fatalf("expected approval resolver call, got count=%d last=%#v", approvals.calls, approvals.last)
	}
	if coordinator.decisionSignals != 1 || coordinator.lastDecision.DecisionRequestID != decisionID || coordinator.lastDecision.ResolvedEventID == uuid.Nil {
		t.Fatalf("expected decision signal, got count=%d signal=%#v", coordinator.decisionSignals, coordinator.lastDecision)
	}
}

func TestUpdateConfigRejectsArchivedProject(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         uuid.New(),
		Name:             "已归档项目",
		Status:           ProjectStatusArchived,
		HumanOwnerUserID: uuid.New(),
	}
	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    repo.projects[projectID].TenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新名称",
	})
	if !errors.Is(err, ErrProjectArchived) {
		t.Fatalf("expected archived error, got %v", err)
	}
}

func TestUpdateProjectConfigCreatesRevision(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	updated, err := service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        "新项目",
		Goal:        "新目标",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if updated.Name != "新项目" {
		t.Fatalf("expected updated project name, got %q", updated.Name)
	}
	if len(repo.revisions) != 1 {
		t.Fatalf("expected config revision, got %d", len(repo.revisions))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
}

func TestUpdateProjectConfigRejectsMissingIDs(t *testing.T) {
	service, err := NewService(newMemoryRepository())
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, tc := range []struct {
		name string
		req  UpdateProjectConfigRequest
	}{
		{name: "tenant", req: UpdateProjectConfigRequest{ProjectID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "project", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ActorUserID: uuid.New()}},
		{name: "actor", req: UpdateProjectConfigRequest{TenantID: uuid.New(), ProjectID: uuid.New()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := service.UpdateProjectConfig(context.Background(), tc.req)
			if !errors.Is(err, ErrInvalidProject) {
				t.Fatalf("expected invalid project error, got %v", err)
			}
		})
	}
}

func TestUpdateProjectConfigWithoutMembersPreservesExistingMembers(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	memberID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "旧项目",
		Goal:             "旧目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: memberID,
	}
	repo.members[projectID] = []ProjectMember{{
		ID:            uuid.New(),
		TenantID:      tenantID,
		ProjectID:     projectID,
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   memberID,
		ProjectRole:   ProjectRoleOwner,
		Status:        "active",
	}}

	_, err = service.UpdateProjectConfig(context.Background(), UpdateProjectConfigRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: uuid.New(),
		Name:        " 新项目 ",
	})
	if err != nil {
		t.Fatalf("update config: %v", err)
	}
	if got := repo.projects[projectID].Name; got != "新项目" {
		t.Fatalf("expected trimmed name, got %q", got)
	}
	if len(repo.members[projectID]) != 1 {
		t.Fatalf("expected members to be preserved, got %d", len(repo.members[projectID]))
	}
}

func TestReplaceProjectMembersRequiresActorAndRecordsEvent(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	_, err = service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.Nil, nil)
	if !errors.Is(err, ErrInvalidProject) {
		t.Fatalf("expected invalid project error, got %v", err)
	}

	members, err := service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{{
		PrincipalType: PrincipalTypeDigitalEmployee,
		PrincipalID:   uuid.New(),
		ProjectRole:   ProjectRoleExecutor,
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected one member, got %d", len(members))
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventConfigChanged {
		t.Fatalf("expected config changed event, got %#v", repo.eventTypes)
	}
	if got := repo.events[0].Payload["member_count"]; got != 1 {
		t.Fatalf("expected member_count payload, got %#v", got)
	}
}

func TestListPaginationIsNormalized(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	projectID := uuid.New()
	tenantID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}

	if _, err := service.ListProjects(context.Background(), ListProjectsRequest{TenantID: tenantID, Limit: 200, Offset: -5}); err != nil {
		t.Fatalf("list projects: %v", err)
	}
	if repo.lastListProjects.Limit != 100 || repo.lastListProjects.Offset != 0 {
		t.Fatalf("expected projects pagination 100/0, got %d/%d", repo.lastListProjects.Limit, repo.lastListProjects.Offset)
	}
	if _, err := service.ListProjectEvents(context.Background(), tenantID, projectID, 0, -1); err != nil {
		t.Fatalf("list events: %v", err)
	}
	if repo.lastEventsLimit != 50 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected events pagination 50/0, got %d/%d", repo.lastEventsLimit, repo.lastEventsOffset)
	}
	if _, err := service.ListProjectDemands(context.Background(), tenantID, projectID, 101, -2); err != nil {
		t.Fatalf("list demands: %v", err)
	}
	if repo.lastDemandsLimit != 100 || repo.lastDemandsOffset != 0 {
		t.Fatalf("expected demands pagination 100/0, got %d/%d", repo.lastDemandsLimit, repo.lastDemandsOffset)
	}
	if _, err := service.GetOverview(context.Background(), tenantID, projectID); err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if repo.lastTasksLimit != 20 || repo.lastTasksOffset != 0 || repo.lastEventsLimit != 20 || repo.lastEventsOffset != 0 {
		t.Fatalf("expected overview pagination 20/0, got tasks %d/%d events %d/%d", repo.lastTasksLimit, repo.lastTasksOffset, repo.lastEventsLimit, repo.lastEventsOffset)
	}
}

type memoryRepository struct {
	projects           map[uuid.UUID]Project
	members            map[uuid.UUID][]ProjectMember
	tasks              []ProjectTask
	events             []ProjectEvent
	eventTypes         []ProjectEventType
	demands            []ProjectDemand
	revisions          []ProjectConfigRevision
	coordinationJobs   []CoordinationJob
	routeDecisions     []RouteDecision
	executionSummaries []ExecutionSummary
	transferRequests   []TransferRequest
	decisionRequests   []DecisionRequest
	lastListProjects   ListProjectsRequest
	lastTasksLimit     int32
	lastTasksOffset    int32
	lastEventsLimit    int32
	lastEventsOffset   int32
	lastDemandsLimit   int32
	lastDemandsOffset  int32
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		projects: map[uuid.UUID]Project{},
		members:  map[uuid.UUID][]ProjectMember{},
	}
}

func strPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func (r *memoryRepository) CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error) {
	project := Project{
		ID:                     projectID,
		TenantID:               req.TenantID,
		TeamID:                 req.TeamID,
		Name:                   req.Name,
		Description:            strPtrOrNil(req.Description),
		Goal:                   req.Goal,
		Status:                 ProjectStatusRunning,
		HumanOwnerUserID:       req.HumanOwnerUserID,
		LeaderUserID:           req.LeaderUserID,
		AcceptanceUserID:       req.AcceptanceUserID,
		CoordinationWorkflowID: workflowID,
		CoordinationStatus:     "registered",
		CoordinationPolicy:     req.CoordinationPolicy,
		ApprovalPolicy:         req.ApprovalPolicy,
		EvidencePolicy:         req.EvidencePolicy,
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *memoryRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	return project, nil
}

func (r *memoryRepository) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	r.lastListProjects = req
	projects := make([]Project, 0, len(r.projects))
	for _, project := range r.projects {
		if project.TenantID != req.TenantID {
			continue
		}
		if req.Status != nil && project.Status != *req.Status {
			continue
		}
		if req.Query != "" && !strings.Contains(project.Name, req.Query) && !strings.Contains(project.Goal, req.Query) {
			continue
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func (r *memoryRepository) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error) {
	project, ok := r.projects[req.ProjectID]
	if !ok || project.TenantID != req.TenantID {
		return Project{}, ErrProjectNotFound
	}
	if req.Name != "" {
		project.Name = req.Name
	}
	if req.Description != "" {
		project.Description = strPtrOrNil(req.Description)
	}
	if req.Goal != "" {
		project.Goal = req.Goal
	}
	if req.HumanOwnerUserID != uuid.Nil {
		project.HumanOwnerUserID = req.HumanOwnerUserID
	}
	if req.LeaderUserID != nil {
		project.LeaderUserID = req.LeaderUserID
	}
	if req.AcceptanceUserID != nil {
		project.AcceptanceUserID = req.AcceptanceUserID
	}
	if req.CoordinationPolicy != nil {
		project.CoordinationPolicy = req.CoordinationPolicy
	}
	if req.ApprovalPolicy != nil {
		project.ApprovalPolicy = req.ApprovalPolicy
	}
	if req.EvidencePolicy != nil {
		project.EvidencePolicy = req.EvidencePolicy
	}
	r.projects[project.ID] = project
	return project, nil
}

func (r *memoryRepository) ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return Project{}, ErrProjectNotFound
	}
	now := time.Now()
	project.Status = ProjectStatusArchived
	project.ArchivedAt = &now
	r.projects[projectID] = project
	return project, nil
}

func (r *memoryRepository) ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	project, ok := r.projects[projectID]
	if !ok || project.TenantID != tenantID {
		return nil, ErrProjectNotFound
	}
	mapped := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		mapped = append(mapped, ProjectMember{
			ID:                  uuid.New(),
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       member.PrincipalType,
			PrincipalID:         member.PrincipalID,
			ProjectRole:         member.ProjectRole,
			DisplayNameSnapshot: strPtrOrNil(member.DisplayNameSnapshot),
			Status:              "active",
			Settings:            member.Settings,
		})
	}
	r.members[projectID] = mapped
	return mapped, nil
}

func (r *memoryRepository) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	members := r.members[projectID]
	filtered := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		if member.TenantID == tenantID {
			filtered = append(filtered, member)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	r.lastTasksLimit = limit
	r.lastTasksOffset = offset
	filtered := make([]ProjectTask, 0, len(r.tasks))
	for _, task := range r.tasks {
		if task.TenantID == tenantID && task.ProjectID == projectID && (status == nil || task.Status == *status) {
			filtered = append(filtered, task)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error) {
	projectEvent := ProjectEvent{
		ID:             uuid.New(),
		TenantID:       event.TenantID,
		ProjectID:      event.ProjectID,
		SequenceNumber: int64(len(r.events) + 1),
		EventType:      event.EventType,
		ActorType:      event.ActorType,
		ActorID:        event.ActorID,
		ResourceType:   event.ResourceType,
		ResourceID:     event.ResourceID,
		Summary:        strPtrOrNil(event.Summary),
		Payload:        event.Payload,
	}
	r.events = append(r.events, projectEvent)
	r.eventTypes = append(r.eventTypes, event.EventType)
	return projectEvent, nil
}

func (r *memoryRepository) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	r.lastEventsLimit = limit
	r.lastEventsOffset = offset
	filtered := make([]ProjectEvent, 0, len(r.events))
	for _, event := range r.events {
		if event.TenantID == tenantID && event.ProjectID == projectID {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error) {
	demand := ProjectDemand{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		SubmittedByUserID: req.SubmittedByUserID,
		Title:             req.Title,
		Content:           strPtrOrNil(req.Content),
		SourceType:        req.SourceType,
		SourceRefs:        req.SourceRefs,
		Attachments:       req.Attachments,
		Status:            status,
		CreatedEventID:    createdEventID,
	}
	r.demands = append(r.demands, demand)
	return demand, nil
}

func (r *memoryRepository) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	r.lastDemandsLimit = limit
	r.lastDemandsOffset = offset
	filtered := make([]ProjectDemand, 0, len(r.demands))
	for _, demand := range r.demands {
		if demand.TenantID == tenantID && demand.ProjectID == projectID {
			filtered = append(filtered, demand)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error) {
	revision := ProjectConfigRevision{
		ID:              uuid.New(),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		RevisionNumber:  int32(len(r.revisions) + 1),
		ConfigSnapshot:  map[string]any{"name": project.Name, "status": string(project.Status)},
		ChangeSummary:   strPtrOrNil("项目配置已更新"),
		CreatedByUserID: req.ActorUserID,
		CreatedEventID:  &eventID,
	}
	r.revisions = append(r.revisions, revision)
	return revision, nil
}

func (r *memoryRepository) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error) {
	for _, demand := range r.demands {
		if demand.ID == demandID && demand.TenantID == tenantID {
			return demand, nil
		}
	}
	return ProjectDemand{}, ErrProjectNotFound
}

func (r *memoryRepository) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error) {
	for _, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error) {
	job := CoordinationJob{
		ID:               uuid.New(),
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		WorkflowID:       req.WorkflowID,
		TriggerEventID:   req.TriggerEventID,
		JobType:          req.JobType,
		Status:           req.Status,
		InputSnapshotRef: req.InputSnapshotRef,
		OutputEventIDs:   []any{},
		CreatedAt:        time.Now().UTC(),
	}
	r.coordinationJobs = append(r.coordinationJobs, job)
	return job, nil
}

func (r *memoryRepository) FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error) {
	for index, job := range r.coordinationJobs {
		if job.ID == req.ID && job.TenantID == req.TenantID {
			now := time.Now().UTC()
			job.Status = req.Status
			job.OutputEventIDs = req.OutputEventIDs
			job.FinishedAt = &now
			r.coordinationJobs[index] = job
			return job, nil
		}
	}
	return CoordinationJob{}, ErrProjectNotFound
}

func (r *memoryRepository) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	filtered := make([]CoordinationJob, 0, len(r.coordinationJobs))
	for _, job := range r.coordinationJobs {
		if job.TenantID == tenantID && job.ProjectID == projectID {
			filtered = append(filtered, job)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error) {
	decision := RouteDecision{
		ID:                          uuid.New(),
		TenantID:                    req.TenantID,
		ProjectID:                   req.ProjectID,
		CoordinationJobID:           req.CoordinationJobID,
		DemandID:                    req.DemandID,
		CandidateDigitalEmployeeIDs: req.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  req.SelectedDigitalEmployeeIDs,
		Reason:                      req.Reason,
		InputRequirements:           req.InputRequirements,
		ExpectedOutputs:             req.ExpectedOutputs,
		BudgetEstimate:              req.BudgetEstimate,
		RequiresHumanReview:         req.RequiresHumanReview,
		CreatedEventID:              req.CreatedEventID,
		CreatedAt:                   time.Now().UTC(),
	}
	r.routeDecisions = append(r.routeDecisions, decision)
	return decision, nil
}

func (r *memoryRepository) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	filtered := make([]RouteDecision, 0, len(r.routeDecisions))
	for _, decision := range r.routeDecisions {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error) {
	task := ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  req.DemandID,
		Title:                     req.Title,
		Summary:                   strPtrOrNil(req.Summary),
		Status:                    req.Status,
		AssignedDigitalEmployeeID: req.AssignedDigitalEmployeeID,
		RiskLevel:                 strPtrOrNil(req.RiskLevel),
		RequiresHumanApproval:     req.RequiresHumanApproval,
		CreatedAt:                 time.Now().UTC(),
		UpdatedAt:                 time.Now().UTC(),
	}
	r.tasks = append(r.tasks, task)
	return task, nil
}

func (r *memoryRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			task.Status = status
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) AssignProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, assignedDigitalEmployeeID, eventID *uuid.UUID) (ProjectTask, error) {
	for index, task := range r.tasks {
		if task.ID == projectTaskID && task.TenantID == tenantID {
			task.Status = status
			task.AssignedDigitalEmployeeID = assignedDigitalEmployeeID
			task.UpdatedAt = time.Now().UTC()
			r.tasks[index] = task
			return task, nil
		}
	}
	return ProjectTask{}, ErrProjectNotFound
}

func (r *memoryRepository) CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error) {
	summary := ExecutionSummary{
		ID:                    uuid.New(),
		TenantID:              req.TenantID,
		ProjectID:             req.ProjectID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          req.EvidenceRefs,
		ArtifactRefs:          req.ArtifactRefs,
		ConfidenceFactors:     req.ConfidenceFactors,
		Uncertainty:           strPtrOrNil(req.Uncertainty),
		MissingInformation:    req.MissingInformation,
		RecommendedNextAction: strPtrOrNil(req.RecommendedNextAction),
		RequiresHumanReview:   req.RequiresHumanReview,
		TransferRequestID:     req.TransferRequestID,
		CreatedEventID:        req.CreatedEventID,
		CreatedAt:             time.Now().UTC(),
	}
	r.executionSummaries = append(r.executionSummaries, summary)
	return summary, nil
}

func (r *memoryRepository) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	filtered := make([]ExecutionSummary, 0, len(r.executionSummaries))
	for _, summary := range r.executionSummaries {
		if summary.TenantID == tenantID && summary.ProjectID == projectID {
			filtered = append(filtered, summary)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error) {
	transfer := TransferRequest{
		ID:                           uuid.New(),
		TenantID:                     req.TenantID,
		ProjectID:                    req.ProjectID,
		ProjectTaskID:                req.ProjectTaskID,
		RequestedByDigitalEmployeeID: req.RequestedByDigitalEmployeeID,
		Reason:                       req.Reason,
		SuggestedEmployeeType:        strPtrOrNil(req.SuggestedEmployeeType),
		SuggestedDigitalEmployeeIDs:  req.SuggestedDigitalEmployeeIDs,
		MissingContextRefs:           req.MissingContextRefs,
		Status:                       req.Status,
		CreatedEventID:               req.CreatedEventID,
		CreatedAt:                    time.Now().UTC(),
		UpdatedAt:                    time.Now().UTC(),
	}
	r.transferRequests = append(r.transferRequests, transfer)
	return transfer, nil
}

func (r *memoryRepository) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	filtered := make([]TransferRequest, 0, len(r.transferRequests))
	for _, transfer := range r.transferRequests {
		if transfer.TenantID == tenantID && transfer.ProjectID == projectID {
			filtered = append(filtered, transfer)
		}
	}
	return filtered, nil
}

func (r *memoryRepository) CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error) {
	decision := DecisionRequest{
		ID:                uuid.New(),
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: req.ApprovalRequestID,
		CoordinationJobID: req.CoordinationJobID,
		ProjectTaskID:     req.ProjectTaskID,
		TargetUserID:      req.TargetUserID,
		DecisionType:      req.DecisionType,
		TitleSnapshot:     req.TitleSnapshot,
		SummarySnapshot:   strPtrOrNil(req.SummarySnapshot),
		RiskLevelSnapshot: strPtrOrNil(req.RiskLevelSnapshot),
		StatusSnapshot:    req.StatusSnapshot,
		CreatedEventID:    req.CreatedEventID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	r.decisionRequests = append(r.decisionRequests, decision)
	return decision, nil
}

func (r *memoryRepository) ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error) {
	for index, decision := range r.decisionRequests {
		if decision.ID == req.ID && decision.TenantID == req.TenantID {
			now := time.Now().UTC()
			decision.StatusSnapshot = req.StatusSnapshot
			decision.ResolvedEventID = req.ResolvedEventID
			decision.ResolvedAt = &now
			decision.UpdatedAt = now
			r.decisionRequests[index] = decision
			return decision, nil
		}
	}
	return DecisionRequest{}, ErrProjectNotFound
}

func (r *memoryRepository) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	filtered := make([]DecisionRequest, 0, len(r.decisionRequests))
	for _, decision := range r.decisionRequests {
		if decision.TenantID == tenantID && decision.ProjectID == projectID {
			filtered = append(filtered, decision)
		}
	}
	return filtered, nil
}

type fakeCoordinatorSignalClient struct {
	ensureSignals    int
	demandSignals    int
	policySignals    int
	memberSignals    int
	completedSignals int
	failedSignals    int
	transferSignals  int
	decisionSignals  int
	lastDemand       DemandSubmittedSignal
	lastCompleted    EmployeeTaskCompletedSignal
	lastDecision     HumanDecisionSubmittedSignal
}

func (f *fakeCoordinatorSignalClient) EnsureProjectCoordinator(ctx context.Context, signal ProjectCoordinatorSignal) error {
	f.ensureSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalDemandSubmitted(ctx context.Context, signal DemandSubmittedSignal) error {
	f.demandSignals++
	f.lastDemand = signal
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalProjectPolicyChanged(ctx context.Context, signal ProjectPolicyChangedSignal) error {
	f.policySignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalProjectMemberChanged(ctx context.Context, signal ProjectMemberChangedSignal) error {
	f.memberSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskCompleted(ctx context.Context, signal EmployeeTaskCompletedSignal) error {
	f.completedSignals++
	f.lastCompleted = signal
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTaskFailed(ctx context.Context, signal EmployeeTaskFailedSignal) error {
	f.failedSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalEmployeeTransferRequested(ctx context.Context, signal EmployeeTransferRequestedSignal) error {
	f.transferSignals++
	return nil
}

func (f *fakeCoordinatorSignalClient) SignalHumanDecisionSubmitted(ctx context.Context, signal HumanDecisionSubmittedSignal) error {
	f.decisionSignals++
	f.lastDecision = signal
	return nil
}

type fakeApprovalResolver struct {
	calls int
	last  ResolveApprovalRequest
}

func (f *fakeApprovalResolver) ResolveApproval(ctx context.Context, req ResolveApprovalRequest) error {
	f.calls++
	f.last = req
	return nil
}
