//go:build live_db

package skill

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run with:
// DATABASE_URL=... go test -tags=live_db ./internal/skill -run TestLiveProjectionVenueFilter -count=1 -v
func TestLiveProjectionVenueFilter(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "superteam"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	repo := NewPgRepository(pool)
	svc := NewService(repo, nil)
	tenant := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	emp := uuid.MustParse("0be393bb-9dfd-48c8-b010-4b5abb114f23")
	projA := uuid.MustParse("56de8016-ce14-43d9-95bf-3fca89849b0a") // linux bound
	projB := uuid.MustParse("e5ed366a-cf0d-47fb-8bfb-0178b86f0876")
	skillID := uuid.MustParse("1191e00e-c679-4770-a4b5-cb55887c6533")

	_, err = pool.Exec(ctx, `
INSERT INTO skill_agent_bindings (tenant_id, skill_id, digital_employee_id, status)
SELECT $1,$2,$3,'enabled'
WHERE NOT EXISTS (
  SELECT 1 FROM skill_agent_bindings WHERE tenant_id=$1 AND skill_id=$2 AND digital_employee_id=$3
)`, tenant, skillID, emp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM skill_agent_bindings WHERE tenant_id=$1 AND skill_id=$2 AND digital_employee_id=$3`, tenant, skillID, emp)
	})

	has := func(res RuntimeSkillsResult, slug string) bool {
		for _, s := range res.Skills {
			if s.Slug == slug {
				return true
			}
		}
		return false
	}

	// Employee carries linux which is bound only to projA → S5: not in projB
	resB, err := svc.ListSkillsForRuntime(ctx, tenant, emp, &projB)
	if err != nil {
		t.Fatal(err)
	}
	if has(resB, "linux") {
		t.Fatalf("S5 fail: foreign project-bound skill projected into B: %#v", resB.Skills)
	}
	resA, err := svc.ListSkillsForRuntime(ctx, tenant, emp, &projA)
	if err != nil {
		t.Fatal(err)
	}
	if !has(resA, "linux") {
		t.Fatalf("S5/S6 fail: expected linux in A, got %#v", resA.Skills)
	}

	// S4: remove employee carry; project supply still projects in A
	_, err = pool.Exec(ctx, `DELETE FROM skill_agent_bindings WHERE tenant_id=$1 AND skill_id=$2 AND digital_employee_id=$3`, tenant, skillID, emp)
	if err != nil {
		t.Fatal(err)
	}
	resA2, err := svc.ListSkillsForRuntime(ctx, tenant, emp, &projA)
	if err != nil {
		t.Fatal(err)
	}
	if !has(resA2, "linux") {
		t.Fatalf("S4 fail: project supply should project linux in A: %#v", resA2.Skills)
	}
	resB2, err := svc.ListSkillsForRuntime(ctx, tenant, emp, &projB)
	if err != nil {
		t.Fatal(err)
	}
	if has(resB2, "linux") {
		t.Fatalf("S4 venue: linux must not appear in B: %#v", resB2.Skills)
	}
}
