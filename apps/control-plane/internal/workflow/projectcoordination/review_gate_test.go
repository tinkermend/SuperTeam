package projectcoordination

import (
	"context"
	"testing"

	"github.com/superteam/control-plane/internal/project"
)

// stubDetector is a tiny ConditionDetector that returns a fixed DetectionResult,
// so the aggregation core (runReviewGate) can be exercised without real rule/LLM
// detectors. Detected/Severity/Finding are whatever the test wires in.
type stubDetector struct {
	key    string
	result DetectionResult
}

func (d stubDetector) Key() string { return d.key }

func (d stubDetector) Detect(_ context.Context, _ DetectionArtifact) DetectionResult {
	return d.result
}

// enabledStub builds one EnabledCondition around a stubDetector with the given
// resolved action and detection result.
func enabledStub(key, action string, res DetectionResult) EnabledCondition {
	res.ConditionKey = key
	return EnabledCondition{
		Spec:   ConditionSpec{Key: key, Detector: stubDetector{key: key, result: res}},
		Action: action,
	}
}

// TestRunReviewGateDetectsViolation: a single enabled block-action condition that
// detects → HOLD (Violated=true) with Action="block" and the finding recorded.
func TestRunReviewGateDetectsViolation(t *testing.T) {
	enabled := []EnabledCondition{
		enabledStub("secret_leak", reviewGateActionBlock, DetectionResult{Detected: true, Severity: "block", Finding: "疑似密钥泄漏"}),
	}
	out := runReviewGate(context.Background(), DetectionArtifact{}, enabled, 0)
	if !out.Violated {
		t.Fatalf("expected Violated=true, got %#v", out)
	}
	if out.Action != reviewGateActionBlock {
		t.Fatalf("expected Action=block, got %q", out.Action)
	}
	if len(out.Findings) != 1 || out.Findings[0].ConditionKey != "secret_leak" {
		t.Fatalf("expected 1 secret_leak finding, got %#v", out.Findings)
	}
}

// TestRunReviewGateCleanReleases: no condition detects → default release
// (Violated=false, no findings).
func TestRunReviewGateCleanReleases(t *testing.T) {
	enabled := []EnabledCondition{
		enabledStub("secret_leak", reviewGateActionBlock, DetectionResult{}),
		enabledStub("code_review", reviewGateActionNeedHuman, DetectionResult{}),
	}
	out := runReviewGate(context.Background(), DetectionArtifact{}, enabled, 0)
	if out.Violated {
		t.Fatalf("expected Violated=false, got %#v", out)
	}
	if len(out.Findings) != 0 {
		t.Fatalf("expected no findings, got %#v", out.Findings)
	}
	if out.Action != "" {
		t.Fatalf("expected empty Action, got %q", out.Action)
	}
}

// TestRunReviewGateRecordOnlyDoesNotHold: a detected finding whose resolved
// action is record_only is recorded in Findings but does NOT set Violated.
func TestRunReviewGateRecordOnlyDoesNotHold(t *testing.T) {
	enabled := []EnabledCondition{
		enabledStub("code_review", reviewGateActionRecordOnly, DetectionResult{Detected: true, Severity: "major", Finding: "记一笔但不拦截"}),
	}
	out := runReviewGate(context.Background(), DetectionArtifact{}, enabled, 0)
	if out.Violated {
		t.Fatalf("expected Violated=false for record_only, got %#v", out)
	}
	if len(out.Findings) != 1 {
		t.Fatalf("expected the record_only finding recorded, got %#v", out.Findings)
	}
	if out.Action != "" {
		t.Fatalf("expected empty Action, got %q", out.Action)
	}
}

// TestRunReviewGateMinorToleranceReleases: N minor findings within tolerance do
// not hold; exceeding tolerance holds.
func TestRunReviewGateMinorToleranceReleases(t *testing.T) {
	minorFindings := func(n int) []EnabledCondition {
		out := make([]EnabledCondition, 0, n)
		for i := 0; i < n; i++ {
			key := "minor_cond"
			out = append(out, enabledStub(key, reviewGateActionNeedHuman, DetectionResult{Detected: true, Severity: "minor", Finding: "轻微问题"}))
		}
		return out
	}

	// 2 minor findings, tolerance 2 → within tolerance → released.
	out := runReviewGate(context.Background(), DetectionArtifact{}, minorFindings(2), 2)
	if out.Violated {
		t.Fatalf("expected minor findings within tolerance to release, got %#v", out)
	}
	if len(out.Findings) != 2 {
		t.Fatalf("expected 2 recorded findings, got %#v", out.Findings)
	}

	// 3 minor findings, tolerance 2 → exceeds → held.
	out = runReviewGate(context.Background(), DetectionArtifact{}, minorFindings(3), 2)
	if !out.Violated {
		t.Fatalf("expected minor findings exceeding tolerance to hold, got %#v", out)
	}
	if out.Action != reviewGateActionNeedHuman {
		t.Fatalf("expected Action=need_human, got %q", out.Action)
	}
}

// TestRunReviewGateStrictestAction: mixed block + need_human violations → the
// strictest action (block) wins.
func TestRunReviewGateStrictestAction(t *testing.T) {
	enabled := []EnabledCondition{
		enabledStub("code_review", reviewGateActionNeedHuman, DetectionResult{Detected: true, Severity: "major", Finding: "转人工"}),
		enabledStub("secret_leak", reviewGateActionBlock, DetectionResult{Detected: true, Severity: "block", Finding: "拦截"}),
	}
	out := runReviewGate(context.Background(), DetectionArtifact{}, enabled, 0)
	if !out.Violated {
		t.Fatalf("expected Violated=true, got %#v", out)
	}
	if out.Action != reviewGateActionBlock {
		t.Fatalf("expected strictest Action=block, got %q", out.Action)
	}
	if len(out.Findings) != 2 {
		t.Fatalf("expected both findings recorded, got %#v", out.Findings)
	}
}

// TestAssembleDetectionArtifactUsesInlineDiff: the assembled artifact carries the
// reviewed task's own summary/deliverable names/evidence refs, and DiffText is
// populated from inline deliverable/change content (not object-storage-only refs).
func TestAssembleDetectionArtifactUsesInlineDiff(t *testing.T) {
	result := &project.ProjectTaskResult{
		Contract: project.TaskResultContract{
			Summary: "实现登录接口",
			Deliverables: []project.TaskResultDeliverable{
				{Name: "auth.go", Value: "diff --git a/auth.go\n+password = \"hunter2\""},
			},
			EvidenceRefs: []project.TaskResultRef{{Ref: "artifact://run/1"}},
		},
	}
	art := assembleDetectionArtifact(result)
	if art.Summary != "实现登录接口" {
		t.Fatalf("expected summary carried, got %q", art.Summary)
	}
	if len(art.Deliverables) != 1 || art.Deliverables[0] != "auth.go" {
		t.Fatalf("expected deliverable name, got %#v", art.Deliverables)
	}
	if len(art.EvidenceRefs) != 1 || art.EvidenceRefs[0] != "artifact://run/1" {
		t.Fatalf("expected evidence ref, got %#v", art.EvidenceRefs)
	}
	if art.DiffText == "" {
		t.Fatalf("expected DiffText populated from inline deliverable value")
	}
	// The secret-leak rule detector must see the inline diff content.
	det := newSecretLeakDetector()
	if !det.Detect(context.Background(), art).Detected {
		t.Fatalf("expected inline diff content to be scannable by secret_leak, DiffText=%q", art.DiffText)
	}
}

// TestAssembleDetectionArtifactFallsBackToSummary: with no inline deliverable/
// change content, DiffText falls back to the contract summary.
func TestAssembleDetectionArtifactFallsBackToSummary(t *testing.T) {
	result := &project.ProjectTaskResult{
		Contract: project.TaskResultContract{
			Summary:      "sk-ABCDEFGHIJKLMNOP1234 leaked in prose",
			Deliverables: []project.TaskResultDeliverable{{Name: "notes"}},
		},
	}
	art := assembleDetectionArtifact(result)
	if art.DiffText != result.Contract.Summary {
		t.Fatalf("expected DiffText to fall back to summary, got %q", art.DiffText)
	}
}
