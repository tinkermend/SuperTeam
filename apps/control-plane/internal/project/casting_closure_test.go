package project

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// castingCountStub 只回答「这个员工在这个项目里还占着几条编制」，用于 G6 护栏。
type castingCountStub struct {
	castingListStub
	countByEmployee map[uuid.UUID]int
	deletedRoleKeys []string
}

func (r *castingCountStub) CountCastingsForEmployee(_ context.Context, _, _, employeeID uuid.UUID) (int, error) {
	return r.countByEmployee[employeeID], nil
}

// G6：仍被编制引用的数字员工不得从成员池移除。
// 没有这条护栏，剧本的编制会指向一个已不在池里的人，可达收口与派发都会失真。
func TestReplaceProjectMembersRejectsStillCastEmployee(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	castEmployeeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "编制护栏项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{TenantID: tenantID, ProjectID: projectID, PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID, ProjectRole: ProjectRoleOwner, Status: "active"},
		{TenantID: tenantID, ProjectID: projectID, PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: castEmployeeID, ProjectRole: ProjectRoleExecutor, Status: "active"},
	}
	service.SetCastingRepository(&castingCountStub{countByEmployee: map[uuid.UUID]int{castEmployeeID: 1}})

	// 只留人类负责人 = 把仍被编制的数字员工移出池子。
	_, err = service.ReplaceProjectMembers(context.Background(), tenantID, projectID, ownerID, []ProjectMemberInput{{
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   ownerID,
		ProjectRole:   ProjectRoleOwner,
	}})
	if !errors.Is(err, ErrCastingEmployeeInUse) {
		t.Fatalf("expected ErrCastingEmployeeInUse, got %v", err)
	}
}

func TestReplaceProjectMembersAllowsRemovalWhenNoCasting(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	freeEmployeeID := uuid.New()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "编制护栏项目",
		Goal:             "目标",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	repo.members[projectID] = []ProjectMember{
		{TenantID: tenantID, ProjectID: projectID, PrincipalType: PrincipalTypeHumanUser, PrincipalID: ownerID, ProjectRole: ProjectRoleOwner, Status: "active"},
		{TenantID: tenantID, ProjectID: projectID, PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: freeEmployeeID, ProjectRole: ProjectRoleExecutor, Status: "active"},
	}
	service.SetCastingRepository(&castingCountStub{countByEmployee: map[uuid.UUID]int{}})

	members, err := service.ReplaceProjectMembers(context.Background(), tenantID, projectID, ownerID, []ProjectMemberInput{{
		PrincipalType: PrincipalTypeHumanUser,
		PrincipalID:   ownerID,
		ProjectRole:   ProjectRoleOwner,
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected one member left, got %d", len(members))
	}
}

// roleVocabStub 让指定 key 变成「非 active」（UnknownKeys 把停用视同未注册）。
type roleVocabStub struct {
	inactive map[string]struct{}
}

func (s *roleVocabStub) UnknownKeys(_ context.Context, _ uuid.UUID, keys []string) ([]string, error) {
	var out []string
	for _, key := range keys {
		if _, ok := s.inactive[key]; ok {
			out = append(out, key)
		}
	}
	return out, nil
}

// 停用词表角色后，员工身上的旧绑定仍在，pool 回退会命中他 —— 但 PutCasting 会以
// UnknownKeys 400 拒绝，这个角色实际上再也编制不上。读路径必须判为缺角色，
// 否则剧本选择器会说「这档收口可达」，而人怎么点都编不进去。
func TestMissingRolesTreatsDisabledVocabRoleAsMissing(t *testing.T) {
	holder := uuid.New()
	svc := &Service{}
	svc.SetDigitalEmployeeRoleSource(&castingRoleSourceStub{
		roleKeys: map[uuid.UUID][]string{holder: {"operator"}},
	})
	svc.SetRoleVocabulary(&roleVocabStub{inactive: map[string]struct{}{"operator": {}}})

	pool := map[string]map[uuid.UUID]struct{}{
		"operator": {holder: struct{}{}},
	}
	got := missingRoles([]string{"operator"}, map[string]uuid.UUID{}, pool, svc, uuid.New(), context.Background())
	if len(got) != 1 || got[0] != "operator" {
		t.Fatalf("disabled role must be missing even with pool holders, got %v", got)
	}

	// 对照：词表仍 active 时，pool 回退照旧生效。
	svc.SetRoleVocabulary(&roleVocabStub{inactive: map[string]struct{}{}})
	if got := missingRoles([]string{"operator"}, map[string]uuid.UUID{}, pool, svc, uuid.New(), context.Background()); len(got) != 0 {
		t.Fatalf("active role with pool holders must not be missing, got %v", got)
	}
}

// castingAlertNotifierSpy 记录级联通知与告警关闭。
type castingAlertNotifierSpy struct {
	notified []CastingInvalidationNotifyRequest
	resolved []uuid.UUID
}

func (s *castingAlertNotifierSpy) NotifyCastingInvalidated(_ context.Context, req CastingInvalidationNotifyRequest) error {
	s.notified = append(s.notified, req)
	return nil
}

func (s *castingAlertNotifierSpy) ResolveCastingAlerts(_ context.Context, _, projectID uuid.UUID) error {
	s.resolved = append(s.resolved, projectID)
	return nil
}

// 「编制失效」告警没有人类动词（照 channel_alert 先例），重新编制是唯一关闭者。
// 不关就永久滞留在每个负责人的收件箱里，变成假告警。
func TestPutCastingResolvesInvalidationAlerts(t *testing.T) {
	holder := uuid.New()
	repo := &castingTxProbeRepo{}
	svc := newCastingTxProbeService(t, repo)
	svc.SetDigitalEmployeeRoleSource(&castingRoleSourceStub{
		roleKeys: map[uuid.UUID][]string{holder: {"developer"}},
	})
	svc.SetScenarioTemplateSpecSource(&castingSpecStub{spec: scenariotemplate.SpecV2{
		Skeleton: []scenariotemplate.SpecSkeletonStep{{Role: "developer"}},
	}})
	spy := &castingAlertNotifierSpy{}
	svc.SetCastingInvalidationNotifier(spy)

	if _, err := svc.PutCasting(context.Background(), PutCastingRequest{
		TenantID:            castingProbeTenantID,
		ProjectID:           castingProbeProjectID,
		ActorUserID:         uuid.New(),
		ScenarioTemplateKey: "software_delivery",
		Assignments:         []CastingAssignment{{RoleKey: "developer", DigitalEmployeeID: holder}},
	}); err != nil {
		t.Fatalf("put casting: %v", err)
	}
	if len(spy.resolved) != 1 || spy.resolved[0] != castingProbeProjectID {
		t.Fatalf("re-casting must resolve open 编制失效 alerts, got %v", spy.resolved)
	}
}
