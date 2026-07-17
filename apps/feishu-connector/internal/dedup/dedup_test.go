package dedup

import "testing"

func TestSeenDeduplicates(t *testing.T) {
	set := New(3)
	if set.Seen("a") {
		t.Fatalf("first occurrence must be new")
	}
	if !set.Seen("a") {
		t.Fatalf("second occurrence must dedupe")
	}
	if set.Seen("") {
		t.Fatalf("empty id never dedupes")
	}
}

func TestSeenEvictsOldest(t *testing.T) {
	set := New(2)
	set.Seen("a")
	set.Seen("b")
	set.Seen("c") // 淘汰 a
	if set.Seen("a") {
		t.Fatalf("evicted id should be treated as new again")
	}
	if !set.Seen("c") {
		t.Fatalf("recent id must still dedupe")
	}
}
