package project

import "testing"

func TestOrphanWaitingHumanRepairSummaryIsChinese(t *testing.T) {
	t.Parallel()
	if containsASCIIWord(orphanWaitingHumanRepairSummary, "waiting_human") {
		t.Fatalf("summary leaks enum: %q", orphanWaitingHumanRepairSummary)
	}
	if containsASCIIWord(orphanWaitingHumanRepairSummary, "reconciler") {
		t.Fatalf("summary leaks technical term: %q", orphanWaitingHumanRepairSummary)
	}
	if humanWaitReasonLabel(HumanWaitReasonRuntimeRecovery) != "需要恢复 Runtime" {
		t.Fatalf("runtime recovery label: %q", humanWaitReasonLabel(HumanWaitReasonRuntimeRecovery))
	}
	if humanWaitReasonLabel(HumanWaitReasonClarification) != "需要澄清" {
		t.Fatalf("clarification label: %q", humanWaitReasonLabel(HumanWaitReasonClarification))
	}
}

func containsASCIIWord(s, word string) bool {
	return len(s) > 0 && (len(word) > 0) && (stringIndexFold(s, word) >= 0)
}

func stringIndexFold(s, substr string) int {
	// simple contains for ASCII tokens
	return indexOf(s, substr)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
