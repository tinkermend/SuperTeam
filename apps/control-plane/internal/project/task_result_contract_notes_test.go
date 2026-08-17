package project

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestTaskResultHandoffNoteUnmarshal(t *testing.T) {
	var contract TaskResultContract
	payload := `{
		"status": "completed",
		"summary": "done",
		"handoff_notes": [
			{"name": "risk_notes", "value": "无风险"},
			{"name": "structured", "value": {"a": 1, "b": [2]}},
			{"name": "null_value", "value": null}
		]
	}`
	if err := json.Unmarshal([]byte(payload), &contract); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(contract.HandoffNotes) != 3 {
		t.Fatalf("notes: %#v", contract.HandoffNotes)
	}
	if contract.HandoffNotes[0].Value != "无风险" {
		t.Fatalf("plain value: %#v", contract.HandoffNotes[0])
	}
	if contract.HandoffNotes[1].Value != `{"a":1,"b":[2]}` {
		t.Fatalf("object value must compact-serialize: %#v", contract.HandoffNotes[1])
	}
	if contract.HandoffNotes[2].Value != "" {
		t.Fatalf("null value must be empty: %#v", contract.HandoffNotes[2])
	}
}

func TestHandoffNotesNotGatedOnCompletion(t *testing.T) {
	// 散文不进闸（2026-08-16 拍板 8）：handoff_notes 缺失不得让 completed 结果
	// 产生 validation_errors。
	task := ProjectTask{
		ExpectedOutputs: []any{"head_commit"},
		PlannerMetadata: map[string]any{"produces": []any{"head_commit"}},
	}
	result := TaskResultContract{
		Status:       TaskResultStatusCompleted,
		Deliverables: []TaskResultDeliverable{{Name: "head_commit", Value: "abc"}},
	}
	if errors := validateCompletedTaskResult(task, result); len(errors) != 0 {
		t.Fatalf("missing handoff_notes must not gate: %v", errors)
	}
}

func TestRequiredInputNameSetStructuredForm(t *testing.T) {
	raw := []any{
		"plain",
		map[string]any{"name": "structured"},
		map[string]any{"kind": "git_commit"},
	}
	set := requiredInputNameSet(raw)
	if !set["plain"] || !set["structured"] || set["git_commit"] {
		t.Fatalf("structured required_inputs must be recognized by name: %#v", set)
	}
}

func TestBlockedDeclarationStillResolvableWithStructuredInputs(t *testing.T) {
	// mapTaskResultDecision 的 blocked 申报白名单必须认得 v3 对象形态，
	// 否则结构化模板任务的 blocked 申报会错误升级为 waitHuman。
	task := ProjectTask{
		InputRequirements: map[string]any{
			"required_inputs": []any{
				map[string]any{"name": "head_commit", "kind": "git_commit", "required": true},
			},
		},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Blocker: &TaskResultBlocker{Reason: "缺输入", MissingInputs: []string{"head_commit"}},
	}
	if decision := mapTaskResultDecision(task, result); decision != TaskResultDecisionBlockedResolvableUpstream {
		t.Fatalf("decision = %s, want %s", decision, TaskResultDecisionBlockedResolvableUpstream)
	}
}

func TestBlockedDeclarationAcceptsNotesWhitelist(t *testing.T) {
	// H1b（spec 2026-08-16 交接包 §4.4）：申报白名单 = required_inputs ∪ notes 声明。
	task := ProjectTask{
		InputRequirements: map[string]any{"required_inputs": []any{"probe_commit"}},
		HandoffContract:   map[string]any{"notes": []any{map[string]any{"name": "risk_notes"}}},
	}
	result := TaskResultContract{
		Status: TaskResultStatusBlocked,
		Blocker: &TaskResultBlocker{
			Reason:              "上游没给结论",
			MissingInputs:       []string{"risk_notes"},
			MissingInputReasons: map[string]string{"risk_notes": "没有风险说明无法安排验证"},
		},
	}
	if decision := mapTaskResultDecision(task, result); decision != TaskResultDecisionBlockedResolvableUpstream {
		t.Fatalf("note-name declaration must be resolvable: %s", decision)
	}
	// 既不在 required_inputs 也不在 notes 的名字仍是契约违规 → 人。
	result.Blocker.MissingInputs = []string{"made_up_name"}
	if decision := mapTaskResultDecision(task, result); decision != TaskResultDecisionBlockedWaitingHuman {
		t.Fatalf("undeclared name must go to human: %s", decision)
	}
}

func TestInputGapsProjectedSoftly(t *testing.T) {
	taskID := uuid.New()
	task := ProjectTask{
		ID:              taskID,
		PlannerMetadata: map[string]any{"produces": []any{"probe_commit"}},
	}
	contract := &TaskResultContract{
		Deliverables: []TaskResultDeliverable{{Name: "probe_commit", Value: "v"}},
		InputGaps:    []TaskResultInputGap{{Name: "risk_notes", Reason: "上游未说明风险"}},
	}
	got := buildProjectTaskGraphHandoffAssessments([]ProjectTask{task}, map[uuid.UUID]*TaskResultContract{taskID: contract}, nil)
	if len(got[0].InputGaps) != 1 || got[0].InputGaps[0].Name != "risk_notes" || got[0].InputGaps[0].Reason != "上游未说明风险" {
		t.Fatalf("input_gaps projection: %#v", got[0].InputGaps)
	}
	// 软条目不影响 status。
	if got[0].Status != ProjectTaskGraphHandoffStatusFulfilled {
		t.Fatalf("input_gaps must not affect status: %s", got[0].Status)
	}
}

// spec 2026-08-17 L1：守卫 UPDATE 的 0 行命中收敛为 409 冲突语义。
func TestGuardWritebackConflict(t *testing.T) {
	if !errors.Is(guardWritebackConflict(pgx.ErrNoRows), ErrProjectConflict) {
		t.Fatal("ErrNoRows must map to ErrProjectConflict")
	}
	sentinel := errors.New("other")
	if !errors.Is(guardWritebackConflict(sentinel), sentinel) {
		t.Fatal("non-no-rows errors must pass through unchanged")
	}
}
