package projectcoordination

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/project"
)

func handoffTestTask(requiredInputs any, notes []any) project.ProjectTask {
	task := project.ProjectTask{
		ID:                uuid.New(),
		InputRequirements: map[string]any{"required_inputs": requiredInputs},
	}
	if notes != nil {
		task.HandoffContract = map[string]any{"notes": notes}
	}
	return task
}

func TestHandoffSlotDeclarationsMixedForms(t *testing.T) {
	task := handoffTestTask([]any{
		"plain_string",
		map[string]any{"name": "structured", "kind": "git_commit", "required": false},
	}, nil)
	decls := handoffSlotDeclarations(task)
	if len(decls) != 2 {
		t.Fatalf("expected 2 decls, got %#v", decls)
	}
	if decls[0].Name != "plain_string" || !decls[0].Required {
		t.Fatalf("string form: %#v", decls[0])
	}
	if decls[1].Name != "structured" || decls[1].Kind != "git_commit" || decls[1].Required {
		t.Fatalf("object form: %#v", decls[1])
	}
}

func TestResolveHandoffSlotsByNameAndKindFallback(t *testing.T) {
	blocker := handoffBlockerMaterial{
		TaskID:    "task-a",
		TaskTitle: "开发",
		ResultID:  "result-1",
		Deliverables: []project.TaskResultDeliverable{
			{Name: "head_commit", Kind: "git_commit", Value: "abc123"},
			{Name: "other_branch", Kind: "branch_ref", Ref: "refs/heads/feature"},
		},
	}
	decls := []handoffSlotDecl{
		{Name: "head_commit", Kind: "git_commit", Required: true},
		{Name: "release_branch", Kind: "branch_ref", Required: true}, // name 未命中，kind 唯一回落
	}
	compiled := resolveHandoffSlots(decls, nil, []handoffBlockerMaterial{blocker}, true, nil)
	if len(compiled.Clarifications) != 0 {
		t.Fatalf("unexpected clarifications: %#v", compiled.Clarifications)
	}
	inputs := compiled.Package["resolved_inputs"].([]map[string]any)
	if len(inputs) != 2 {
		t.Fatalf("expected 2 resolved inputs, got %#v", inputs)
	}
	if inputs[0]["value"] != "abc123" || inputs[0]["source_task_id"] != "task-a" || inputs[0]["verified"] != true {
		t.Fatalf("name hit: %#v", inputs[0])
	}
	if inputs[1]["ref"] != "refs/heads/feature" {
		t.Fatalf("kind fallback: %#v", inputs[1])
	}
	// 未被声明命中的产出进紧凑索引；命中的两条不重复出现（键缺省即无剩余）。
	index := compiled.Package["upstream_index"].([]map[string]any)
	if others, exists := index[0]["other_deliverables"]; exists {
		t.Fatalf("consumed deliverables must not repeat in index: %#v", others)
	}
}

func TestResolveHandoffSlotsGateCoveredMissingDegrades(t *testing.T) {
	blocker := handoffBlockerMaterial{TaskID: "task-a", TaskTitle: "开发", ResultID: "result-1"}
	compiled := resolveHandoffSlots([]handoffSlotDecl{{Name: "head_commit", Required: true}}, nil, []handoffBlockerMaterial{blocker}, true, nil)
	if len(compiled.Clarifications) != 0 {
		t.Fatalf("gate-covered missing must degrade, not clarify: %#v", compiled.Clarifications)
	}
	inputs := compiled.Package["resolved_inputs"].([]map[string]any)
	if inputs[0]["missing"] != "unavailable" {
		t.Fatalf("missing marker: %#v", inputs[0])
	}
}

func TestResolveHandoffSlotsDynamicEdgeMissingClarifies(t *testing.T) {
	blocker := handoffBlockerMaterial{TaskID: "task-a", TaskTitle: "补做", ResultID: "result-1"}
	compiled := resolveHandoffSlots([]handoffSlotDecl{{Name: "head_commit", Required: true}}, nil, []handoffBlockerMaterial{blocker}, false, nil)
	if len(compiled.Clarifications) != 1 || compiled.Clarifications[0].Kind != "missing_input" {
		t.Fatalf("dynamic-edge missing must clarify: %#v", compiled.Clarifications)
	}
}

func TestResolveHandoffSlotsNameConflictClarifies(t *testing.T) {
	blockers := []handoffBlockerMaterial{
		{
			TaskID: "task-a", TaskTitle: "开发A", ResultID: "r1",
			Deliverables: []project.TaskResultDeliverable{{Name: "head_commit", Value: "aaa"}},
		},
		{
			TaskID: "task-b", TaskTitle: "开发B", ResultID: "r2",
			Deliverables: []project.TaskResultDeliverable{{Name: "head_commit", Value: "bbb"}},
		},
	}
	compiled := resolveHandoffSlots([]handoffSlotDecl{{Name: "head_commit", Required: true}}, nil, blockers, true, nil)
	if len(compiled.Clarifications) != 1 || compiled.Clarifications[0].Kind != "name_conflict" {
		t.Fatalf("same-name conflict must clarify with sources: %#v", compiled.Clarifications)
	}
	if !strings.Contains(strings.Join(compiled.Clarifications[0].Sources, ","), "开发A") {
		t.Fatalf("conflict sources: %#v", compiled.Clarifications[0].Sources)
	}
}

func TestResolveHandoffNotesExtractionAndBudget(t *testing.T) {
	blockers := []handoffBlockerMaterial{{
		TaskID: "task-a", TaskTitle: "开发", ResultID: "r1",
		Notes: []project.TaskResultHandoffNote{
			{Name: "risk_notes", Value: strings.Repeat("险", 3000)},
			{Name: "how_to_run", Value: "pnpm test"},
			{Name: "undeclared", Value: "不应被提取"},
		},
	}}
	decls := []handoffNoteDecl{{Name: "risk_notes"}, {Name: "how_to_run"}}
	compiled := resolveHandoffSlots(nil, decls, blockers, true, nil)
	notes := compiled.Package["upstream_notes"].([]map[string]any)
	if len(notes) != 2 {
		t.Fatalf("declared notes only: %#v", notes)
	}
	if len(notes[0]["value"].(string)) > handoffNoteValueLimitBytes+32 {
		t.Fatalf("per-note truncation: %d bytes", len(notes[0]["value"].(string)))
	}
	if notes[0]["truncated"] != true || notes[1]["truncated"] != nil {
		t.Fatalf("truncation flags: %#v / %#v", notes[0], notes[1])
	}
	if compiled.Package["notes_truncated"] != true {
		t.Fatalf("package-level truncation marker missing: %#v", compiled.Package)
	}
}

func TestTaskBelongsToAcceptedPlanDecomposition(t *testing.T) {
	revisionID := uuid.New()
	base := project.ProjectTask{AcceptedPlanRevisionID: &revisionID}
	if !taskBelongsToAcceptedPlanDecomposition(base) {
		t.Fatal("plain decomposed task must be gate-covered")
	}
	revisionOf := uuid.New()
	dynamic := project.ProjectTask{
		AcceptedPlanRevisionID: &revisionID,
		RevisionOfTaskID:       &revisionOf,
	}
	if taskBelongsToAcceptedPlanDecomposition(dynamic) {
		t.Fatal("revision task must not be gate-covered")
	}
	marked := project.ProjectTask{
		AcceptedPlanRevisionID: &revisionID,
		PlannerMetadata:        map[string]any{"source_task_id": "x"},
	}
	if taskBelongsToAcceptedPlanDecomposition(marked) {
		t.Fatal("metadata-marked dynamic task must not be gate-covered")
	}
	iteration := project.ProjectTask{AcceptedPlanRevisionID: &revisionID, PlanIteration: 2}
	if taskBelongsToAcceptedPlanDecomposition(iteration) {
		t.Fatal("supplement task (plan_iteration>0) must not be gate-covered")
	}
	if taskBelongsToAcceptedPlanDecomposition(project.ProjectTask{}) {
		t.Fatal("task without accepted revision must not be gate-covered")
	}
}

func TestPlannerRequiredInputsAcceptsObjectForm(t *testing.T) {
	raw := map[string]any{
		"required_inputs": []any{
			"plain",
			map[string]any{"name": "structured", "kind": "git_commit"},
			map[string]any{"kind": "nameless"},
		},
	}
	inputs := plannerRequiredInputs(raw)
	if len(inputs) != 2 || inputs[0] != "plain" || inputs[1] != "structured" {
		t.Fatalf("object form must normalize to names: %#v", inputs)
	}
}

func TestHandoffSupplementedEdgesBudgetPredicate(t *testing.T) {
	sourceID := uuid.New()
	ownerID := uuid.New()
	otherOwnerID := uuid.New()
	revisionOf := uuid.Nil
	supplement := project.ProjectTask{
		ID:               uuid.New(),
		RevisionOfTaskID: &ownerID,
		PlanIteration:    1,
		PlannerMetadata:  map[string]any{"supplement_for": sourceID.String()},
	}
	// 修订任务（非补做）不算预算。
	revision := project.ProjectTask{
		ID:               uuid.New(),
		RevisionOfTaskID: &otherOwnerID,
		PlannerMetadata:  map[string]any{"revision_root_task_id": uuid.NewString()},
	}
	// 指向别的申报方的补做不算本边预算。
	otherSourceSupplement := project.ProjectTask{
		ID:               uuid.New(),
		RevisionOfTaskID: &ownerID,
		PlanIteration:    1,
		PlannerMetadata:  map[string]any{"supplement_for": uuid.NewString()},
	}
	edges := handoffSupplementedEdges([]project.ProjectTask{supplement, revision, otherSourceSupplement, {ID: sourceID}}, sourceID)
	if !edges[ownerID] {
		t.Fatalf("owner edge must be budget-spent: %#v", edges)
	}
	if edges[otherOwnerID] {
		t.Fatalf("revision task must not spend supplement budget: %#v", edges)
	}
	_ = revisionOf
}

// ledgerEvents 记录补链预算记账（内存仓储补 CreateExecutionLedgerEvent）。
func (r *projectStoreMemoryRepository) CreateExecutionLedgerEvent(_ context.Context, req project.CreateExecutionLedgerEventRequest) (project.ExecutionLedgerEvent, error) {
	r.ledgerEventRequests = append(r.ledgerEventRequests, req)
	return project.ExecutionLedgerEvent{ID: uuid.New(), EventType: req.EventType}, nil
}

func TestResolveHandoffSlotsFanInMergesTwoBlockers(t *testing.T) {
	// fan-in：双 blocker 各供一槽，resolved_inputs 双条且溯源各自独立
	//（spec 2026-08-16 交接包 §6 H1a 验收 ④ 的编译层判据；真链形态由
	// task-graph 表达——汇总任务依赖两条边自然拿两份直接前驱）。
	blockers := []handoffBlockerMaterial{
		{
			TaskID: "task-a", TaskTitle: "生产A", ResultID: "ra",
			Deliverables: []project.TaskResultDeliverable{{Name: "slot_a", Kind: "conclusion", Value: "A1"}},
		},
		{
			TaskID: "task-b", TaskTitle: "生产B", ResultID: "rb",
			Deliverables: []project.TaskResultDeliverable{{Name: "slot_b", Kind: "conclusion", Value: "B2"}},
		},
	}
	decls := []handoffSlotDecl{
		{Name: "slot_a", Required: true},
		{Name: "slot_b", Required: true},
	}
	compiled := resolveHandoffSlots(decls, nil, blockers, true, nil)
	if len(compiled.Clarifications) != 0 {
		t.Fatalf("fan-in must not clarify: %#v", compiled.Clarifications)
	}
	inputs := compiled.Package["resolved_inputs"].([]map[string]any)
	if len(inputs) != 2 {
		t.Fatalf("expected 2 resolved inputs, got %#v", inputs)
	}
	byName := map[string]map[string]any{}
	for _, entry := range inputs {
		byName[entry["name"].(string)] = entry
	}
	if byName["slot_a"]["value"] != "A1" || byName["slot_a"]["source_task_id"] != "task-a" {
		t.Fatalf("slot_a: %#v", byName["slot_a"])
	}
	if byName["slot_b"]["value"] != "B2" || byName["slot_b"]["source_task_id"] != "task-b" {
		t.Fatalf("slot_b: %#v", byName["slot_b"])
	}
}

// ---- spec 2026-08-17-recovery-mode-and-active-run-poisoning-fix P1-A 决策表 ----

// modeFixture 组装决策表五行各自的最小仓储夹具。
func modeFixture(mode string, withGraphRevision bool, taskHasRevision bool) (*projectStoreMemoryRepository, project.ProjectTask) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	revisionID := uuid.New()
	demand := project.ProjectDemand{
		ID: demandID, TenantID: tenantID, ProjectID: projectID,
		Title: "mode-probe", CoordinationMode: mode,
	}
	task := project.ProjectTask{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		DemandID: &demandID, CoordinationJobID: &jobID,
		Status: "running",
	}
	if taskHasRevision {
		task.AcceptedPlanRevisionID = &revisionID
	}
	sibling := project.ProjectTask{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		DemandID: &demandID, CoordinationJobID: &jobID,
	}
	if withGraphRevision {
		sibling.AcceptedPlanRevisionID = &revisionID
	}
	loop := project.CoordinationModeLoop
	planMode := project.CoordinationModePlan
	var planRevisions []project.PlanRevision
	if taskHasRevision {
		// 修订可读：模式取 plan（r1 的 plan 形态）。
		planRevisions = append(planRevisions, project.PlanRevision{
			ID: revisionID, TenantID: tenantID, ProjectID: projectID,
			CoordinationMode: &planMode,
		})
	}
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID},
		demand:        demand,
		planRevisions: planRevisions,
		tasks:         []project.ProjectTask{task, sibling},
	}
	_ = loop
	return repo, task
}

func TestResolveTaskCoordinationModeDecisionTable(t *testing.T) {
	cases := []struct {
		name             string
		demandMode       string
		withGraphRevision bool
		taskHasRevision  bool
		want             string
	}{
		// r1：修订可读且为 plan → plan。
		{"r1 修订权威-plan", "loop", true, true, project.CoordinationModePlan},
		// r2：修订读失败（夹具不给修订行）→ plan。
		{"r2 修订读失败-plan", "loop", true, true, project.CoordinationModePlan},
		// r3：指针 nil、demand 可读、图上有带修订任务 → 按 demand（loop/plan 皆验）。
		{"r3 demand权威-loop", "loop", true, false, project.CoordinationModeLoop},
		{"r3 demand权威-plan", "plan", true, false, project.CoordinationModePlan},
		// r4：指针 nil、全图无修订 → loop（史前回兼容，无视回填 plan）。
		{"r4 史前-loop", "plan", false, false, project.CoordinationModeLoop},
		// r5：指针 nil、demand 不可读 → plan（catch-all）。
		{"r5 demand不可读-plan", "plan", false, false, project.CoordinationModePlan},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			repo, task := modeFixture(testCase.demandMode, testCase.withGraphRevision, testCase.taskHasRevision)
			if testCase.name == "r2 修订读失败-plan" {
				repo.planRevisions = nil // 制造修订指针非 nil 但读不到
			}
			if testCase.name == "r5 demand不可读-plan" {
				repo.demand = project.ProjectDemand{} // GetProjectDemand 落空
			}
			store := NewProjectStore(repo)
			if got := store.resolveTaskCoordinationMode(context.Background(), task); got != testCase.want {
				t.Fatalf("mode = %s, want %s", got, testCase.want)
			}
		})
	}
}

func TestRecoveryReplacementCarriesAcceptedPlanRevision(t *testing.T) {
	// P1-B：替换任务继承源任务的计划修订（spec §1.3）。
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	jobID := uuid.New()
	revisionID := uuid.New()
	employeeID := uuid.New()
	decisionID := uuid.New()
	source := project.ProjectTask{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		DemandID: &demandID, CoordinationJobID: &jobID,
		AcceptedPlanRevisionID: &revisionID,
		Status:                 "failed",
		AssignedDigitalEmployeeID: &employeeID,
		Title:                  "源任务",
	}
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{ID: projectID, TenantID: tenantID, HumanOwnerUserID: uuid.New()},
		tasks:         []project.ProjectTask{source},
		decisionRequests: []project.DecisionRequest{{
			ID: decisionID, TenantID: tenantID, ProjectID: projectID,
			DecisionType: "task_failure_recovery", ProjectTaskID: &source.ID,
			StatusSnapshot: "pending",
		}},
		members: []project.ProjectMember{projectStoreExecutorMember(tenantID, projectID, employeeID)},
	}
	store := NewProjectStoreWithApprovals(repo, &projectStoreApprovalCreator{approvalID: uuid.New()})
	_, err := store.ApplyFailureRecoveryDecision(context.Background(), ApplyFailureRecoveryDecisionInput{
		TenantID: tenantID, ProjectID: projectID,
		DecisionRequestID: decisionID, Decision: "approved",
	})
	if err != nil {
		t.Fatalf("ApplyFailureRecoveryDecision: %v", err)
	}
	if len(repo.projectTaskRequests) == 0 {
		t.Fatal("expected replacement task created")
	}
	request := repo.projectTaskRequests[0]
	if request.AcceptedPlanRevisionID == nil || *request.AcceptedPlanRevisionID != revisionID {
		t.Fatalf("replacement must inherit accepted plan revision: %#v", request.AcceptedPlanRevisionID)
	}
}
