package project

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// fakeMemberTeamResolver drives the participation gate in tests: assignments
// maps employee id -> team id pointer (nil = teamless); missing ids = 不存在.
type fakeMemberTeamResolver struct {
	assignments map[uuid.UUID]*uuid.UUID
	calls       int
}

func (f *fakeMemberTeamResolver) ListDigitalEmployeeTeamAssignments(_ context.Context, _ uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]*uuid.UUID, error) {
	f.calls++
	result := make(map[uuid.UUID]*uuid.UUID, len(employeeIDs))
	for _, id := range employeeIDs {
		if teamID, ok := f.assignments[id]; ok {
			result[id] = teamID
		}
	}
	return result, nil
}

func TestReplaceProjectMembersRejectsTeamlessDigitalEmployee(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}
	teamlessID := uuid.New()
	resolver := &fakeMemberTeamResolver{assignments: map[uuid.UUID]*uuid.UUID{teamlessID: nil}}
	service.SetMemberTeamAssignmentResolver(resolver)

	_, err = service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{{
		PrincipalType:       PrincipalTypeDigitalEmployee,
		PrincipalID:         teamlessID,
		ProjectRole:         ProjectRoleExecutor,
		DisplayNameSnapshot: "候岗员工甲",
	}})
	if !errors.Is(err, ErrTeamlessProjectMember) {
		t.Fatalf("expected teamless member error, got %v", err)
	}
	if !strings.Contains(err.Error(), "候岗员工甲") {
		t.Fatalf("expected offending member name in error, got %v", err)
	}
	if len(repo.eventTypes) != 0 {
		t.Fatalf("expected no events on rejection, got %#v", repo.eventTypes)
	}
}

func TestReplaceProjectMembersAllowsTeamAssignedDigitalEmployee(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}
	employeeID := uuid.New()
	teamID := uuid.New()
	resolver := &fakeMemberTeamResolver{assignments: map[uuid.UUID]*uuid.UUID{employeeID: &teamID}}
	service.SetMemberTeamAssignmentResolver(resolver)

	members, err := service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{
		{
			PrincipalType: PrincipalTypeHumanUser,
			PrincipalID:   uuid.New(),
			ProjectRole:   ProjectRoleOwner,
		},
		{
			PrincipalType: PrincipalTypeDigitalEmployee,
			PrincipalID:   employeeID,
			ProjectRole:   ProjectRoleExecutor,
		},
	})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("expected two members, got %d", len(members))
	}
	if resolver.calls != 1 {
		t.Fatalf("expected one resolver call, got %d", resolver.calls)
	}
}

func TestReplaceProjectMembersRejectsUnknownDigitalEmployee(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}
	service.SetMemberTeamAssignmentResolver(&fakeMemberTeamResolver{assignments: map[uuid.UUID]*uuid.UUID{}})

	_, err = service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{{
		PrincipalType: PrincipalTypeDigitalEmployee,
		PrincipalID:   uuid.New(),
		ProjectRole:   ProjectRoleExecutor,
	}})
	if !errors.Is(err, ErrTeamlessProjectMember) {
		t.Fatalf("expected teamless member error for unknown employee, got %v", err)
	}
	if !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("expected 不存在 marker in error, got %v", err)
	}
}

func TestReplaceProjectMembersSkipsResolverForHumanOnlyMembers(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: uuid.New(),
	}
	resolver := &fakeMemberTeamResolver{assignments: map[uuid.UUID]*uuid.UUID{}}
	service.SetMemberTeamAssignmentResolver(resolver)

	if _, err := service.ReplaceProjectMembers(context.Background(), tenantID, projectID, uuid.New(), []ProjectMemberInput{{
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   uuid.New(),
		ProjectRole:   ProjectRoleOwner,
	}}); err != nil {
		t.Fatalf("replace members: %v", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("expected resolver untouched for human-only members, got %d calls", resolver.calls)
	}
}

func TestCreateProjectRejectsTeamlessDigitalEmployeeMember(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &fakeCoordinatorSignalClient{}
	service, err := NewServiceWithCoordinator(repo, coordinator)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	actorID := uuid.New()
	teamlessID := uuid.New()
	runtimeNodeID := uuid.New()
	stubProjectRuntimeNodeReader(service, tenantID, runtimeNodeID)
	service.SetMemberTeamAssignmentResolver(&fakeMemberTeamResolver{assignments: map[uuid.UUID]*uuid.UUID{teamlessID: nil}})

	_, err = service.CreateProject(context.Background(), CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "teamless-employee-project",
		Goal:             "验证参与门禁",
		HumanOwnerUserID: uuid.New(),
		RuntimeNodeIDs:   []uuid.UUID{runtimeNodeID},
		Members: []ProjectMemberInput{{
			PrincipalType: PrincipalTypeDigitalEmployee,
			PrincipalID:   teamlessID,
			ProjectRole:   ProjectRoleExecutor,
		}},
	})
	if !errors.Is(err, ErrTeamlessProjectMember) {
		t.Fatalf("expected teamless member error, got %v", err)
	}
	assertNoCreateProjectSideEffects(t, repo, coordinator)
}
