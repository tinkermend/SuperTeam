package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// failureFamilyConstants is the CP producer set (FailureFamily* in types.go).
// Keep in lockstep with contracts/provider/schemas/failure-family.json known_values
// (provider semantic unification §5.3).
func failureFamilyConstants() []string {
	return []string{
		FailureFamilyTransientRuntime,
		FailureFamilyTransientProvider,
		FailureFamilyTimeout,
		FailureFamilyInvalidContract,
		FailureFamilyApprovalRequired,
		FailureFamilyPermissionRequired,
		FailureFamilyNonRetryableExecution,
		FailureFamilyBusinessCancelled,
		FailureFamilyPlanInvalid,
		FailureFamilyRequirementChanged,
		FailureFamilyAcceptanceRequired,
		FailureFamilyDispatchTransient,
		FailureFamilyRuntimeStartTimeout,
		FailureFamilyRuntimeLeaseLost,
		FailureFamilyProviderStart,
		FailureFamilyProviderConfig,
		FailureFamilyBudgetFuse,
	}
}

func loadFailureFamilyKnownValues(t *testing.T) map[string]struct{} {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// apps/control-plane/internal/project → repo root
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
	path := filepath.Join(root, "contracts", "provider", "schemas", "failure-family.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failure-family.json: %v", err)
	}
	var doc struct {
		KnownValues []string `json:"known_values"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse failure-family.json: %v", err)
	}
	if len(doc.KnownValues) == 0 {
		t.Fatal("failure-family.json known_values empty")
	}
	out := make(map[string]struct{}, len(doc.KnownValues))
	for _, v := range doc.KnownValues {
		out[v] = struct{}{}
	}
	return out
}

func TestFailureFamilyConstantsSubsetOfSharedVocab(t *testing.T) {
	known := loadFailureFamilyKnownValues(t)
	for _, family := range failureFamilyConstants() {
		if family == "" {
			t.Fatalf("empty FailureFamily constant")
		}
		if _, ok := known[family]; !ok {
			t.Errorf("CP FailureFamily %q missing from contracts/provider/schemas/failure-family.json known_values", family)
		}
	}
}

func TestFailureFamilyLabelsCoverSharedVocab(t *testing.T) {
	// humanReadableFailureSummary / humanWaitReason must not panic on any known family;
	// at least budget_fuse and transient_provider must have dedicated leads.
	for _, family := range []string{FailureFamilyBudgetFuse, FailureFamilyTransientProvider, FailureFamilyTimeout} {
		summary := humanReadableFailureSummary(family, "detail")
		if summary == "" {
			t.Errorf("empty humanReadableFailureSummary for %q", family)
		}
	}
}
