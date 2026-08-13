package autonomypolicy

import "testing"

func TestMinAndEffective(t *testing.T) {
	if got := Min(TierFullAuto, TierPauseAtGate); got != TierPauseAtGate {
		t.Fatalf("min full+pause = %s", got)
	}
	if got := Effective(TierFullAuto, "", TierFullAuto); got != TierFullAuto {
		t.Fatalf("no project ceiling: %s", got)
	}
	if got := Effective(TierFullAuto, TierPauseAtGate, TierFullAuto); got != TierPauseAtGate {
		t.Fatalf("project tighten: %s", got)
	}
	if got := Effective(TierPauseAtGate, "", TierFullAuto); got != TierPauseAtGate {
		t.Fatalf("playbook tighten: %s", got)
	}
	if _, err := Normalize("bogus"); err == nil {
		t.Fatal("expected error")
	}
}
