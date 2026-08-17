package project

import (
	"testing"

	"github.com/google/uuid"
)

// 交接 verdict 组装层单测(spec 2026-07-27 §5 P2-V):声明全回填=fulfilled、
// 部分=partial、零声明=unknown,以及 produces 声明缺交付与无结果契约的边界。
func TestBuildProjectTaskGraphHandoffAssessments(t *testing.T) {
	taskID := uuid.New()
	resultID := uuid.New()
	makeTask := func(produces []any) ProjectTask {
		task := ProjectTask{ID: taskID, LatestTaskResultID: &resultID}
		if produces != nil {
			task.PlannerMetadata = map[string]any{"produces": produces}
		}
		return task
	}

	t.Run("全部声明交付物已回填为 fulfilled", func(t *testing.T) {
		contract := &TaskResultContract{Deliverables: []TaskResultDeliverable{
			{Name: "file-stats.json", Ref: uuid.NewString(), Summary: "deliverables/file-stats.json"},
			{Name: "scan-log", Value: "全部通过"},
		}}
		got := buildProjectTaskGraphHandoffAssessments(
			[]ProjectTask{makeTask([]any{"file-stats.json", "scan-log"})},
			map[uuid.UUID]*TaskResultContract{taskID: contract},
			nil,
		)
		if len(got) != 1 {
			t.Fatalf("assessments = %d, want 1", len(got))
		}
		if got[0].Status != ProjectTaskGraphHandoffStatusFulfilled {
			t.Fatalf("status = %q, want fulfilled", got[0].Status)
		}
		if len(got[0].Deliverables) != 2 {
			t.Fatalf("deliverables = %d, want 2", len(got[0].Deliverables))
		}
		for _, deliverable := range got[0].Deliverables {
			if deliverable.Verdict != ProjectTaskGraphHandoffDeliverableDelivered {
				t.Fatalf("deliverable %q verdict = %q, want delivered", deliverable.Name, deliverable.Verdict)
			}
		}
	})

	t.Run("部分交付为 partial 且 produces 缺项标记 missing", func(t *testing.T) {
		contract := &TaskResultContract{Deliverables: []TaskResultDeliverable{
			{Name: "file-stats.json", Ref: uuid.NewString()},
		}}
		got := buildProjectTaskGraphHandoffAssessments(
			[]ProjectTask{makeTask([]any{"file-stats.json", "scan-log"})},
			map[uuid.UUID]*TaskResultContract{taskID: contract},
			nil,
		)
		if got[0].Status != ProjectTaskGraphHandoffStatusPartial {
			t.Fatalf("status = %q, want partial", got[0].Status)
		}
		if len(got[0].Deliverables) != 2 {
			t.Fatalf("deliverables = %d, want 2", len(got[0].Deliverables))
		}
		missing := got[0].Deliverables[1]
		if missing.Name != "scan-log" || missing.Verdict != ProjectTaskGraphHandoffDeliverableMissing {
			t.Fatalf("produces 缺项 = %+v, want scan-log/missing", missing)
		}
	})

	t.Run("契约存在但全部未交付为 unfulfilled", func(t *testing.T) {
		contract := &TaskResultContract{Deliverables: []TaskResultDeliverable{
			{Name: "file-stats.json"}, // 无 Ref 无 Value
		}}
		got := buildProjectTaskGraphHandoffAssessments(
			[]ProjectTask{makeTask(nil)},
			map[uuid.UUID]*TaskResultContract{taskID: contract},
			nil,
		)
		if got[0].Status != ProjectTaskGraphHandoffStatusUnfulfilled {
			t.Fatalf("status = %q, want unfulfilled", got[0].Status)
		}
	})

	t.Run("零声明数据必须 unknown 禁止启发式", func(t *testing.T) {
		// 契约存在但无 deliverables 且 planner 无 produces。
		got := buildProjectTaskGraphHandoffAssessments(
			[]ProjectTask{makeTask(nil)},
			map[uuid.UUID]*TaskResultContract{taskID: {}},
			nil,
		)
		if got[0].Status != ProjectTaskGraphHandoffStatusUnknown {
			t.Fatalf("status = %q, want unknown", got[0].Status)
		}
		if len(got[0].Deliverables) != 0 {
			t.Fatalf("deliverables = %d, want 0", len(got[0].Deliverables))
		}
	})

	t.Run("无结果契约的任务为 unknown 即使 produces 已声明", func(t *testing.T) {
		got := buildProjectTaskGraphHandoffAssessments(
			[]ProjectTask{makeTask([]any{"file-stats.json"})},
			map[uuid.UUID]*TaskResultContract{},
			nil,
		)
		if got[0].Status != ProjectTaskGraphHandoffStatusUnknown {
			t.Fatalf("status = %q, want unknown", got[0].Status)
		}
		if len(got[0].Deliverables) != 0 {
			t.Fatalf("交接未发生时不得预判逐条 missing, got %d", len(got[0].Deliverables))
		}
	})
}

// 结论槽位软条目（spec 2026-08-16 交接包 §4.4）：下游声明的 notes 逐条
// delivered/missing，但不进 status 汇总——散文不进闸。
func TestBuildProjectTaskGraphHandoffAssessmentsNotes(t *testing.T) {
	blockerID := uuid.New()
	dependentID := uuid.New()
	blocker := ProjectTask{ID: blockerID}
	dependent := ProjectTask{
		ID:              dependentID,
		HandoffContract: map[string]any{"notes": []any{map[string]any{"name": "risk_notes"}, map[string]any{"name": "how_to_run"}}},
	}
	contract := &TaskResultContract{
		Deliverables:  []TaskResultDeliverable{{Name: "head_commit", Value: "abc"}},
		HandoffNotes:  []TaskResultHandoffNote{{Name: "risk_notes", Value: "无风险"}},
	}
	got := buildProjectTaskGraphHandoffAssessments(
		[]ProjectTask{blocker, dependent},
		map[uuid.UUID]*TaskResultContract{blockerID: contract},
		map[uuid.UUID][]uuid.UUID{blockerID: {dependentID}},
	)
	if len(got) != 2 {
		t.Fatalf("expected 2 assessments, got %d", len(got))
	}
	var blockerAssessment ProjectTaskGraphHandoffAssessment
	for _, item := range got {
		if item.ProjectTaskID == blockerID {
			blockerAssessment = item
		}
	}
	if len(blockerAssessment.Notes) != 2 {
		t.Fatalf("notes: %#v", blockerAssessment.Notes)
	}
	if blockerAssessment.Notes[0].Verdict != ProjectTaskGraphHandoffNoteDelivered {
		t.Fatalf("delivered note: %#v", blockerAssessment.Notes[0])
	}
	if blockerAssessment.Notes[1].Verdict != ProjectTaskGraphHandoffNoteMissing {
		t.Fatalf("missing note: %#v", blockerAssessment.Notes[1])
	}
	// 软条目不影响 status：deliverables 全交付仍是 fulfilled，尽管 note 缺失。
	if blockerAssessment.Status != ProjectTaskGraphHandoffStatusFulfilled {
		t.Fatalf("notes must not affect status: %s", blockerAssessment.Status)
	}
}
