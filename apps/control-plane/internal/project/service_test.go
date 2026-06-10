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

func TestSubmitDemandRecordsOnlyV0DemandAndEvent(t *testing.T) {
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
	if demand.Status != ProjectDemandStatusRecorded {
		t.Fatalf("expected recorded V0 demand, got %s", demand.Status)
	}
	if len(repo.tasks) != 0 {
		t.Fatalf("V0 must not create project tasks from demand automatically")
	}
	if len(repo.eventTypes) != 1 || repo.eventTypes[0] != ProjectEventDemandSubmitted {
		t.Fatalf("expected demand event only, got %#v", repo.eventTypes)
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
	projects          map[uuid.UUID]Project
	members           map[uuid.UUID][]ProjectMember
	tasks             []ProjectTask
	events            []ProjectEvent
	eventTypes        []ProjectEventType
	demands           []ProjectDemand
	revisions         []ProjectConfigRevision
	lastListProjects  ListProjectsRequest
	lastTasksLimit    int32
	lastTasksOffset   int32
	lastEventsLimit   int32
	lastEventsOffset  int32
	lastDemandsLimit  int32
	lastDemandsOffset int32
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
