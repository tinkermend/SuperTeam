package automation

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// castingGateProjects 让 MissingCastingRoles 返回可控的结构化缺口。
type castingGateProjects struct {
	fakeProjects
	missing []project.CastingInvalidation
	owners  []uuid.UUID
}

func (p *castingGateProjects) MissingCastingRoles(context.Context, uuid.UUID, uuid.UUID, string) ([]project.CastingInvalidation, error) {
	return p.missing, nil
}

func (p *castingGateProjects) GetProject(_ context.Context, _, projectID uuid.UUID) (ProjectInfo, error) {
	return ProjectInfo{ID: projectID, TeamID: uuid.New(), Name: "自动化项目", OwnerUserIDs: p.owners}, nil
}

// G7：规则保存期编制不全必须拒绝——把可预防的缺失挡在有人在场的时刻
// （基线 §4.7：运行期失效才允许失败，配置期缺失不该留到无人值守时爆）。
func TestValidateCastingForTemplateRejectsIncompleteOnSave(t *testing.T) {
	employeeID := uuid.New()
	svc := &Service{projects: &castingGateProjects{missing: []project.CastingInvalidation{
		{RoleKey: "operator", Reason: project.CastingReasonNotCast},
		{RoleKey: "verifier", EmployeeID: employeeID, EmployeeName: "验证-X", Reason: project.CastingReasonRoleNotHeld},
	}}}

	key := "incident_response"
	err := svc.validateCastingForTemplate(context.Background(), uuid.New(), uuid.New(), &key)
	if err == nil {
		t.Fatal("expected rule save to be rejected when casting is incomplete")
	}
	if !strings.Contains(err.Error(), "operator") || !strings.Contains(err.Error(), "verifier") {
		t.Fatalf("error must name the missing roles, got %v", err)
	}
}

func TestValidateCastingForTemplateAllowsCompleteCasting(t *testing.T) {
	svc := &Service{projects: &castingGateProjects{missing: nil}}
	key := "incident_response"
	if err := svc.validateCastingForTemplate(context.Background(), uuid.New(), uuid.New(), &key); err != nil {
		t.Fatalf("complete casting must pass rule save, got %v", err)
	}
}

func TestValidateCastingForTemplateSkipsWhenNoTemplateBound(t *testing.T) {
	svc := &Service{projects: &castingGateProjects{missing: []project.CastingInvalidation{
		{RoleKey: "operator", Reason: project.CastingReasonNotCast},
	}}}
	empty := "  "
	if err := svc.validateCastingForTemplate(context.Background(), uuid.New(), uuid.New(), &empty); err != nil {
		t.Fatalf("rule without scenario template must not hit the casting gate, got %v", err)
	}
	if err := svc.validateCastingForTemplate(context.Background(), uuid.New(), uuid.New(), nil); err != nil {
		t.Fatalf("nil template key must not hit the casting gate, got %v", err)
	}
}

// alertSpy 记录 fire 失败告警与恢复关闭。
type alertSpy struct {
	opened   []RuleFailureAlert
	resolved []uuid.UUID
}

func (s *alertSpy) OpenRuleFailureAlert(_ context.Context, req RuleFailureAlert) error {
	s.opened = append(s.opened, req)
	return nil
}

func (s *alertSpy) ResolveRuleAlerts(_ context.Context, _, ruleID uuid.UUID) error {
	s.resolved = append(s.resolved, ruleID)
	return nil
}

// 基线 §4.7 的通知半边：fire 失败必须推给项目负责人集合（any-of-N），
// 且文案要写明缺哪个角色、原选谁、为何失效——只写 fires 表等于没人知道。
func TestFailFireOpensOwnerAlertWithReason(t *testing.T) {
	owners := []uuid.UUID{uuid.New(), uuid.New()}
	spy := &alertSpy{}
	repo := newFakeRepo()
	svc := NewService(repo, &castingGateProjects{owners: owners}, nil, nil, nil)
	svc.SetAlertNotifier(spy)

	rule := Rule{ID: uuid.New(), TenantID: uuid.New(), ProjectID: uuid.New(), Name: "每日巡检"}
	fire := Fire{ID: uuid.New(), TenantID: rule.TenantID, RuleID: rule.ID}
	message := "剧本编制不完整或编制员工不可用: " + project.FormatCastingInvalidations([]project.CastingInvalidation{
		{RoleKey: "operator", Reason: project.CastingReasonNotCast},
		{RoleKey: "verifier", EmployeeName: "验证-X", Reason: project.CastingReasonEmployeeUnavailable},
	})
	if _, err := svc.failFire(context.Background(), rule, fire, "casting_incomplete", message); err != nil {
		t.Fatalf("failFire: %v", err)
	}

	if len(spy.opened) != 1 {
		t.Fatalf("expected one alert batch, got %d", len(spy.opened))
	}
	alert := spy.opened[0]
	if len(alert.OwnerUserIDs) != len(owners) {
		t.Fatalf("alert must target every project owner (any-of-N), got %v", alert.OwnerUserIDs)
	}
	if !strings.Contains(alert.ErrorMessage, "缺角色 operator") {
		t.Fatalf("alert must name the missing role, got %q", alert.ErrorMessage)
	}
	if !strings.Contains(alert.ErrorMessage, "原编制 验证-X") {
		t.Fatalf("alert must name who was originally cast, got %q", alert.ErrorMessage)
	}
	if alert.RuleID != rule.ID || alert.FireID != fire.ID {
		t.Fatalf("alert must carry rule/fire identity, got %+v", alert)
	}
}

func TestSucceedFireResolvesOwnerAlerts(t *testing.T) {
	spy := &alertSpy{}
	repo := newFakeRepo()
	svc := NewService(repo, &castingGateProjects{owners: []uuid.UUID{uuid.New()}}, nil, nil, nil)
	svc.SetAlertNotifier(spy)

	rule := Rule{ID: uuid.New(), TenantID: uuid.New(), ProjectID: uuid.New(), Name: "每日巡检"}
	fire := Fire{ID: uuid.New(), TenantID: rule.TenantID, RuleID: rule.ID}
	if _, err := svc.succeedFire(context.Background(), rule, fire, nil, nil); err != nil {
		t.Fatalf("succeedFire: %v", err)
	}
	if len(spy.resolved) != 1 || spy.resolved[0] != rule.ID {
		t.Fatalf("a successful fire must close the rule's open alerts, got %v", spy.resolved)
	}
}
