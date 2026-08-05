package project

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/scenariotemplate"
)

func TestDistinctRolesFromSteps(t *testing.T) {
	roles := distinctRolesFromSteps([]scenariotemplate.SpecSkeletonStep{
		{Step: "a", Role: "developer"},
		{Step: "b", Role: "reviewer"},
		{Step: "c", Role: "developer"},
	})
	if len(roles) != 2 || roles[0] != "developer" || roles[1] != "reviewer" {
		t.Fatalf("got %#v", roles)
	}
}

func TestComputePlaybookReadinessStopsAtMissingOperator(t *testing.T) {
	spec := scenariotemplate.SpecV2{
		Skeleton: []scenariotemplate.SpecSkeletonStep{
			{Step: "diag", Role: "diagnostician", ProducesDefaults: []scenariotemplate.SpecProduce{{Name: "root_cause"}}},
			{Step: "fix", Role: "operator", DependsOn: []string{"diag"}, ProducesDefaults: []scenariotemplate.SpecProduce{{Name: "fix_record"}}},
		},
		Exits: []scenariotemplate.SpecExit{
			{Deliverable: "root_cause", Label: "仅诊断根因"},
			{Deliverable: "fix_record", Label: "实施修复"},
		},
	}
	// Casting has diagnostician only — mirrors G2 operator vacancy.
	casting := map[string]uuid.UUID{
		"diagnostician": uuid.New(),
	}
	got := computePlaybookReadiness(context.Background(), nil, uuid.Nil, "incident_response", "故障排查", spec, casting, nil)
	if !got.Runnable || got.DeepestExit == nil || got.DeepestExit.Deliverable != "root_cause" {
		t.Fatalf("expected deepest root_cause, got %+v", got)
	}
	if len(got.NextExitNeedsRoles) != 1 || got.NextExitNeedsRoles[0] != "operator" {
		t.Fatalf("expected next needs operator, got %#v", got.NextExitNeedsRoles)
	}
}

// 入池与编制必须同事务（spec §4.4）。分两步做时，编制写入失败会留下
// 「入了池却没有编制」的员工——他仍可被 planner 选中派活，是治理泄漏。
// 本用例用一个「编制必失败」的假仓储证明：失败后不得有成员被留下。
type castingTxProbeRepo struct {
	replaceErr error
	joined     []uuid.UUID
	replaced   bool
}

func (r *castingTxProbeRepo) ListProjectCastings(context.Context, uuid.UUID, uuid.UUID, *string) ([]CastingEntry, error) {
	return nil, nil
}

func (r *castingTxProbeRepo) CountCastingsForEmployee(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (int, error) {
	return 0, nil
}

// ReplaceProjectCasting 模拟真实仓储的事务语义：入池写在同一事务里，
// 因此编制失败时入池也必须一起回滚（joined 保持为空）。
func (r *castingTxProbeRepo) ReplaceProjectCasting(_ context.Context, _, _, _ uuid.UUID, _ string, assignments []CastingAssignment, displayNames map[uuid.UUID]string) ([]CastingEntry, error) {
	staged := make([]uuid.UUID, 0, len(assignments))
	for _, a := range assignments {
		if _, ok := displayNames[a.DigitalEmployeeID]; !ok {
			return nil, fmt.Errorf("display name missing for %s", a.DigitalEmployeeID)
		}
		staged = append(staged, a.DigitalEmployeeID)
	}
	if r.replaceErr != nil {
		// 同事务 → 回滚，入池不落库。
		return nil, r.replaceErr
	}
	r.joined = append(r.joined, staged...)
	r.replaced = true
	entries := make([]CastingEntry, 0, len(assignments))
	for _, a := range assignments {
		entries = append(entries, CastingEntry{RoleKey: a.RoleKey, DigitalEmployeeID: a.DigitalEmployeeID})
	}
	return entries, nil
}

func TestPutCastingJoinsPoolInSameTransaction(t *testing.T) {
	employeeID := uuid.New()
	assignments := []CastingAssignment{{RoleKey: "developer", DigitalEmployeeID: employeeID}}

	t.Run("编制失败时不得留下孤儿成员", func(t *testing.T) {
		repo := &castingTxProbeRepo{replaceErr: fmt.Errorf("boom")}
		svc := newCastingTxProbeService(t, repo)
		_, err := svc.PutCasting(context.Background(), PutCastingRequest{
			TenantID:            castingProbeTenantID,
			ProjectID:           castingProbeProjectID,
			ActorUserID:         uuid.New(),
			ScenarioTemplateKey: "software_delivery",
			Assignments:         assignments,
		})
		if err == nil {
			t.Fatal("expected casting failure to surface")
		}
		if len(repo.joined) != 0 {
			t.Fatalf("编制失败后不得有成员入池，实际入池 %d 人", len(repo.joined))
		}
	})

	t.Run("成功时入池与编制一起生效", func(t *testing.T) {
		repo := &castingTxProbeRepo{}
		svc := newCastingTxProbeService(t, repo)
		entries, err := svc.PutCasting(context.Background(), PutCastingRequest{
			TenantID:            castingProbeTenantID,
			ProjectID:           castingProbeProjectID,
			ActorUserID:         uuid.New(),
			ScenarioTemplateKey: "software_delivery",
			Assignments:         assignments,
		})
		if err != nil {
			t.Fatalf("put casting: %v", err)
		}
		if !repo.replaced || len(entries) != 1 {
			t.Fatalf("编制未生效: replaced=%v entries=%d", repo.replaced, len(entries))
		}
		if len(repo.joined) != 1 || repo.joined[0] != employeeID {
			t.Fatalf("被编制员工必须入池，实际 %#v", repo.joined)
		}
	})
}

var (
	castingProbeTenantID  = uuid.MustParse("00000000-0000-0000-0000-0000000000c1")
	castingProbeProjectID = uuid.MustParse("00000000-0000-0000-0000-0000000000c2")
)

// newCastingTxProbeService 组装一个仅够跑 PutCasting 的 Service：
// 内存仓储提供 GetProject，探针仓储承担编制写入。
func newCastingTxProbeService(t *testing.T, repo CastingRepository) *Service {
	t.Helper()
	memory := newMemoryRepository()
	memory.projects[castingProbeProjectID] = Project{
		ID:       castingProbeProjectID,
		TenantID: castingProbeTenantID,
		Name:     "编制事务探针项目",
		Status:   ProjectStatusRunning,
	}
	service, err := NewService(memory)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.SetCastingRepository(repo)
	return service
}
