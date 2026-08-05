package project

import "testing"

func TestFirstCastingExpansionRoleKey(t *testing.T) {
	t.Parallel()
	if got := firstCastingExpansionRoleKey(nil); got != "" {
		t.Fatalf("nil → %q", got)
	}
	if got := firstCastingExpansionRoleKey([]string{"  reviewer  ", "tester"}); got != "reviewer" {
		t.Fatalf("trim → %q", got)
	}
	if got := firstCastingExpansionRoleKey([]string{"developer (员工不可用)", "reviewer"}); got != "developer" {
		t.Fatalf("strip unavailable annotate → %q", got)
	}
}
