package project

import (
	"context"
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
