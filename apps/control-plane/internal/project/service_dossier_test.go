package project

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type dossierFixture struct {
	service   *Service
	repo      *memoryRepository
	tenantID  uuid.UUID
	projectID uuid.UUID
	ownerID   uuid.UUID
	demand    ProjectDemand
}

func newDossierFixture(t *testing.T) *dossierFixture {
	t.Helper()
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository()
	repo.projects[projectID] = Project{
		ID:               projectID,
		TenantID:         tenantID,
		Name:             "客服工单闭环",
		Status:           ProjectStatusRunning,
		HumanOwnerUserID: ownerID,
	}
	ownerName := "林工"
	repo.members[projectID] = append(repo.members[projectID], ProjectMember{
		ID:                  uuid.New(),
		TenantID:            tenantID,
		ProjectID:           projectID,
		PrincipalType:       PrincipalTypeHumanUser,
		PrincipalID:         ownerID,
		ProjectRole:         ProjectRoleOwner,
		DisplayNameSnapshot: &ownerName,
		Status:              "active",
	})
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID, Title: "修复登录超时",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	return &dossierFixture{
		service:   service,
		repo:      repo,
		tenantID:  tenantID,
		projectID: projectID,
		ownerID:   ownerID,
		demand:    *demand,
	}
}

func (f *dossierFixture) addTask(title, status string, statusChangedAt time.Time) ProjectTask {
	task := ProjectTask{
		ID:              uuid.New(),
		TenantID:        f.tenantID,
		ProjectID:       f.projectID,
		DemandID:        &f.demand.ID,
		Title:           title,
		Status:          status,
		StatusChangedAt: statusChangedAt,
	}
	f.repo.tasks = append(f.repo.tasks, task)
	return task
}

func (f *dossierFixture) addEvent(eventType ProjectEventType, resourceType string, resourceID uuid.UUID, at time.Time) ProjectEvent {
	rt := resourceType
	rid := resourceID.String()
	event := ProjectEvent{
		ID:           uuid.New(),
		TenantID:     f.tenantID,
		ProjectID:    f.projectID,
		EventType:    eventType,
		ActorType:    "human_user",
		ActorID:      f.ownerID.String(),
		ResourceType: &rt,
		ResourceID:   &rid,
		Payload:      map[string]any{},
		CreatedAt:    at,
	}
	f.repo.events = append(f.repo.events, event)
	return event
}

func (f *dossierFixture) get(t *testing.T) *DemandDossier {
	t.Helper()
	dossier, err := f.service.GetDemandDossier(context.Background(), GetDemandDossierRequest{
		TenantID: f.tenantID,
		DemandID: f.demand.ID,
	})
	if err != nil {
		t.Fatalf("get dossier: %v", err)
	}
	return dossier
}

func findSlot(slots []DemandDossierRailSlot, kind string) (DemandDossierRailSlot, bool) {
	for _, slot := range slots {
		if slot.Kind == kind {
			return slot, true
		}
	}
	return DemandDossierRailSlot{}, false
}

// 噪音事件不进时间线,关键事件必须进,且实体名补进标题(不是裸 UUID)。
func TestDemandDossierTimelineFiltersNoiseAndNamesEntities(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	task := f.addTask("补充超时重试", "running", base)

	f.addEvent(ProjectEventWorkflowSignaled, "project_task", task.ID, base.Add(1*time.Minute))
	f.addEvent(ProjectEventTaskDispatchGateChecked, "project_task", task.ID, base.Add(2*time.Minute))
	dispatched := f.addEvent(ProjectEventTaskDispatched, "project_task", task.ID, base.Add(3*time.Minute))

	dossier := f.get(t)

	for _, item := range dossier.Timeline.Items {
		if strings.Contains(item.Title, ".") || strings.Contains(item.Title, "_") {
			t.Fatalf("时间线标题泄漏了 event_type 原串: %q", item.Title)
		}
		if strings.Contains(item.Title, task.ID.String()) {
			t.Fatalf("时间线标题出现裸 UUID: %q", item.Title)
		}
	}

	var dispatchedItem *DemandDossierTimelineItem
	for i := range dossier.Timeline.Items {
		if dossier.Timeline.Items[i].ID == dispatched.ID.String() {
			dispatchedItem = &dossier.Timeline.Items[i]
		}
		if dossier.Timeline.Items[i].Kind == TimelineKindOther &&
			dossier.Timeline.Items[i].Title == "协调信号" {
			t.Fatalf("噪音事件 workflow.signaled 不该进时间线")
		}
	}
	if dispatchedItem == nil {
		t.Fatalf("任务派发事件应进时间线: %#v", dossier.Timeline.Items)
	}
	if dispatchedItem.Kind != TimelineKindTaskDispatched {
		t.Fatalf("kind 应为 task_dispatched,得到 %q", dispatchedItem.Kind)
	}
	if !strings.Contains(dispatchedItem.Title, "补充超时重试") {
		t.Fatalf("标题应补任务显示名,得到 %q", dispatchedItem.Title)
	}
	if dispatchedItem.ActorDisplayName != "林工" {
		t.Fatalf("actor 应补成员显示名,得到 %q", dispatchedItem.ActorDisplayName)
	}
	if dispatchedItem.OpenTarget == nil || dispatchedItem.OpenTarget.Type != "task_detail" {
		t.Fatalf("任务事件应可打开任务详情: %#v", dispatchedItem.OpenTarget)
	}
}

// 事件窗口外的任务终态用实体状态回填;已有对应事件时不得双计。
func TestDemandDossierTimelineBackfillsTerminalTasksWithoutDoubleCounting(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-2 * time.Hour)
	silent := f.addTask("早已完成但无事件", "completed", base)
	noisy := f.addTask("有完成事件", "completed", base.Add(10*time.Minute))
	f.addEvent(ProjectEventTaskCompleted, "project_task", noisy.ID, base.Add(11*time.Minute))

	dossier := f.get(t)

	completedByTask := map[uuid.UUID]int{}
	for _, item := range dossier.Timeline.Items {
		if item.Kind != TimelineKindTaskCompleted || item.Entity == nil {
			continue
		}
		taskID, err := uuid.Parse(item.Entity.ID)
		if err != nil {
			continue
		}
		completedByTask[taskID]++
	}
	if completedByTask[silent.ID] != 1 {
		t.Fatalf("无事件的已完成任务应回填一条,得到 %d", completedByTask[silent.ID])
	}
	if completedByTask[noisy.ID] != 1 {
		t.Fatalf("已有完成事件的任务不该再回填,得到 %d", completedByTask[noisy.ID])
	}
}

// 时间倒序 + 截断标记。
func TestDemandDossierTimelineOrdersDescendingAndTruncates(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	task := f.addTask("批量事件任务", "running", base)
	for i := 0; i < 8; i++ {
		f.addEvent(ProjectEventTaskDispatched, "project_task", task.ID, base.Add(time.Duration(i)*time.Minute))
	}

	dossier, err := f.service.GetDemandDossier(context.Background(), GetDemandDossierRequest{
		TenantID:      f.tenantID,
		DemandID:      f.demand.ID,
		TimelineLimit: 3,
	})
	if err != nil {
		t.Fatalf("get dossier: %v", err)
	}
	if len(dossier.Timeline.Items) != 3 || !dossier.Timeline.Truncated {
		t.Fatalf("应截断到 3 条并标记 truncated,得到 %d/%v", len(dossier.Timeline.Items), dossier.Timeline.Truncated)
	}
	for i := 1; i < len(dossier.Timeline.Items); i++ {
		if dossier.Timeline.Items[i].OccurredAt.After(dossier.Timeline.Items[i-1].OccurredAt) {
			t.Fatalf("时间线必须倒序")
		}
	}
}

// 计划确认是最高频人类动作,通用「待人工决策」要精化成「待计划确认」。
func TestDemandDossierTimelineRefinesPlanReviewDecision(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	task := f.addTask("实现登录重试", "waiting_human", base)
	decision := DecisionRequest{
		ID:             uuid.New(),
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		ProjectTaskID:  &task.ID,
		TargetUserID:   f.ownerID,
		DecisionType:   "plan_review",
		TitleSnapshot:  "确认执行计划",
		StatusSnapshot: "pending",
		CreatedAt:      base,
	}
	f.repo.decisionRequests = append(f.repo.decisionRequests, decision)
	f.addEvent(ProjectEventDecisionRequested, "decision_request", decision.ID, base.Add(time.Minute))

	dossier := f.get(t)

	found := false
	for _, item := range dossier.Timeline.Items {
		if item.Kind == TimelineKindDecisionOpened && item.Title == "待计划确认" {
			found = true
			if item.OpenTarget == nil || item.OpenTarget.Type != "decision" {
				t.Fatalf("决策条目应可深链决策: %#v", item.OpenTarget)
			}
		}
	}
	if !found {
		t.Fatalf("plan_review 决策应精化为「待计划确认」: %#v", dossier.Timeline.Items)
	}
}

// 现网 decision.requested 事件不带 resource_type/resource_id,身份只在 payload 的
// plan_revision_id 里。只认 resource_* 会让最该被一眼认出的「待计划确认」退回成
// 通用的「待人工决策」——真实 E2E 揪出的漏网路径。
func TestDemandDossierTimelineResolvesDecisionFromPayloadPlanRevision(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	revisionID := uuid.New()
	// 与现网一致:plan_review 决策挂在协调 job 上(没有 project_task_id),
	// launch facts 正是按 coordination_job_id 把它捞回来的。
	job := CoordinationJob{
		ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID,
		JobType: "demand_route", Status: "running",
		InputSnapshotRef: map[string]any{"demand_id": f.demand.ID.String()},
	}
	f.repo.coordinationJobs = append(f.repo.coordinationJobs, job)
	decision := DecisionRequest{
		ID:                uuid.New(),
		TenantID:          f.tenantID,
		ProjectID:         f.projectID,
		CoordinationJobID: &job.ID,
		PlanRevisionID:    &revisionID,
		TargetUserID:      f.ownerID,
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
		CreatedAt:         base,
	}
	f.repo.decisionRequests = append(f.repo.decisionRequests, decision)
	f.repo.planRevisions = append(f.repo.planRevisions, PlanRevision{
		ID: revisionID, TenantID: f.tenantID, ProjectID: f.projectID, DemandID: f.demand.ID,
		RevisionNumber: 1, Status: "pending_review",
	})
	// 关键:事件没有 resource_type/resource_id,只有 payload。
	f.repo.events = append(f.repo.events, ProjectEvent{
		ID:        uuid.New(),
		TenantID:  f.tenantID,
		ProjectID: f.projectID,
		EventType: ProjectEventDecisionRequested,
		ActorType: "project_coordinator",
		ActorID:   uuid.New().String(),
		// 与现网一致:决策事件只在 payload 里带 demand_id + plan_revision_id。
		Payload: map[string]any{
			"demand_id":        f.demand.ID.String(),
			"plan_revision_id": revisionID.String(),
		},
		CreatedAt: base.Add(time.Minute),
	})

	dossier := f.get(t)

	found := false
	for _, item := range dossier.Timeline.Items {
		if item.Title == "待计划确认" {
			found = true
			if item.OpenTarget == nil || item.OpenTarget.DecisionID == nil || *item.OpenTarget.DecisionID != decision.ID {
				t.Fatalf("应能深链到该决策: %#v", item.OpenTarget)
			}
		}
	}
	if !found {
		t.Fatalf("payload 携带 plan_revision_id 时应精化为「待计划确认」: %#v", dossier.Timeline.Items)
	}
}

// 同理:任务身份也可能只在 payload 里。
func TestDemandDossierTimelineResolvesTaskFromPayload(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	task := f.addTask("补充超时重试", "running", base)
	f.repo.events = append(f.repo.events, ProjectEvent{
		ID:        uuid.New(),
		TenantID:  f.tenantID,
		ProjectID: f.projectID,
		EventType: ProjectEventTaskDispatched,
		ActorType: "project_coordinator",
		ActorID:   uuid.New().String(),
		Payload:   map[string]any{"project_task_id": task.ID.String()},
		CreatedAt: base.Add(time.Minute),
	})

	dossier := f.get(t)

	for _, item := range dossier.Timeline.Items {
		if item.Kind == TimelineKindTaskDispatched {
			if item.Title != "任务开始 · 补充超时重试" {
				t.Fatalf("payload 里的任务身份应补进标题,得到 %q", item.Title)
			}
			if item.OpenTarget == nil || item.OpenTarget.Type != "task_detail" {
				t.Fatalf("应可打开任务详情: %#v", item.OpenTarget)
			}
			return
		}
	}
	t.Fatalf("未找到派发条目: %#v", dossier.Timeline.Items)
}

// 协调线程类 actor 的 actor_id 是 workflow/job 标识,匹配不到项目成员,必须回落
// 角色化中文而不是留空或吐 UUID。
func TestDemandDossierTimelineNamesCoordinatorActor(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	task := f.addTask("任务", "running", base)
	jobID := uuid.New()
	f.repo.events = append(f.repo.events, ProjectEvent{
		ID:        uuid.New(),
		TenantID:  f.tenantID,
		ProjectID: f.projectID,
		EventType: ProjectEventTaskDispatched,
		ActorType: "project_coordinator",
		ActorID:   jobID.String(),
		Payload:   map[string]any{"project_task_id": task.ID.String()},
		CreatedAt: base.Add(time.Minute),
	})

	dossier := f.get(t)

	for _, item := range dossier.Timeline.Items {
		if item.Kind != TimelineKindTaskDispatched {
			continue
		}
		if item.ActorDisplayName != "协调线程" {
			t.Fatalf("协调线程 actor 应回落角色化中文,得到 %q", item.ActorDisplayName)
		}
		if strings.Contains(item.ActorDisplayName, jobID.String()) {
			t.Fatalf("actor 不得回吐 UUID")
		}
		return
	}
	t.Fatalf("未找到派发条目")
}

// 右轨槽序:先按剧本 produces kind 保序,实际出现但剧本没声明的 kind 追加在后。
func TestDemandDossierRailOrdersSlotsByPlaybookThenActual(t *testing.T) {
	f := newDossierFixture(t)
	templateKey := "software_delivery"
	f.repo.projects[f.projectID] = Project{
		ID:                  f.projectID,
		TenantID:            f.tenantID,
		Name:                "客服工单闭环",
		Status:              ProjectStatusRunning,
		HumanOwnerUserID:    f.ownerID,
		ScenarioTemplateKey: &templateKey,
	}
	f.service.SetScenarioTemplateResolver(stubScenarioTemplateResolver{
		bindings:     map[string]ScenarioTemplateBinding{templateKey: {Key: templateKey, Name: "软件交付", Status: "active"}},
		produceKinds: map[string][]string{templateKey: {"branch_ref", "git_commit", "conclusion"}},
	})

	base := time.Now().UTC().Add(-time.Hour)
	task := f.addTask("提交实现", "completed", base)
	taskID := task.ID
	f.repo.evidenceRefs = append(f.repo.evidenceRefs, ProjectEvidenceRef{
		ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &taskID,
		Title: "回归测试记录", SourceType: "artifact", SourceRef: "artifacts/regression.log",
	})

	dossier := f.get(t)

	if dossier.EffectivePlaybook.Source != DossierPlaybookSourceProject {
		t.Fatalf("剧本应来自项目,得到 %q", dossier.EffectivePlaybook.Source)
	}
	kinds := make([]string, 0, len(dossier.Rail))
	for _, slot := range dossier.Rail {
		kinds = append(kinds, slot.Kind)
	}
	if len(kinds) < 4 {
		t.Fatalf("应至少有剧本三槽 + 证据槽,得到 %#v", kinds)
	}
	for i, expected := range []string{"branch_ref", "git_commit", "conclusion"} {
		if kinds[i] != expected {
			t.Fatalf("剧本槽应保序,位置 %d 期望 %q 得到 %q(全部 %#v)", i, expected, kinds[i], kinds)
		}
	}
	if kinds[len(kinds)-1] != DossierRailKindEvidenceRef {
		t.Fatalf("剧本未声明的实际 kind 应追加在后,得到 %#v", kinds)
	}
	evidenceSlot, ok := findSlot(dossier.Rail, DossierRailKindEvidenceRef)
	if !ok || len(evidenceSlot.Items) != 1 {
		t.Fatalf("证据槽应有一条: %#v", evidenceSlot)
	}
	if evidenceSlot.Title != "证据" {
		t.Fatalf("槽标题应中文,得到 %q", evidenceSlot.Title)
	}
	if evidenceSlot.Items[0].ProjectTaskName != "提交实现" {
		t.Fatalf("证据条目应补任务显示名,得到 %q", evidenceSlot.Items[0].ProjectTaskName)
	}
}

// 别的需求的证据/工件不得出现在本单右轨(G7 的单测对应物)。
func TestDemandDossierRailExcludesOtherDemandsEvidence(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	mine := f.addTask("本单任务", "completed", base)
	mineID := mine.ID

	otherDemandID := uuid.New()
	otherTaskID := uuid.New()
	f.repo.tasks = append(f.repo.tasks, ProjectTask{
		ID: otherTaskID, TenantID: f.tenantID, ProjectID: f.projectID,
		DemandID: &otherDemandID, Title: "别单任务", Status: "completed",
	})
	f.repo.evidenceRefs = append(f.repo.evidenceRefs,
		ProjectEvidenceRef{ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &mineID, Title: "本单证据", SourceRef: "artifacts/mine.log"},
		ProjectEvidenceRef{ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &otherTaskID, Title: "别单证据", SourceRef: "artifacts/other.log"},
	)
	f.repo.artifactRefs = append(f.repo.artifactRefs,
		ProjectArtifactRef{ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &mineID, Title: "本单工件", ObjectRef: "artifacts/mine.zip", ArtifactType: "declared"},
		ProjectArtifactRef{ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &otherTaskID, Title: "别单工件", ObjectRef: "artifacts/other.zip", ArtifactType: "declared"},
	)

	dossier := f.get(t)

	for _, slot := range dossier.Rail {
		for _, item := range slot.Items {
			if strings.Contains(item.Title, "别单") {
				t.Fatalf("别的需求的交付事实泄漏进本单右轨: %q", item.Title)
			}
		}
	}
	evidenceSlot, ok := findSlot(dossier.Rail, DossierRailKindEvidenceRef)
	if !ok || len(evidenceSlot.Items) != 1 || evidenceSlot.Items[0].Title != "本单证据" {
		t.Fatalf("本单证据应恰好一条: %#v", evidenceSlot)
	}
}

// 未知 kind 的槽标题回落通用中文,不得把技术键当标题。
func TestDemandDossierRailLabelsUnknownKindInChinese(t *testing.T) {
	if label := DossierRailKindLabel("some_new_kind"); label != "交付物" {
		t.Fatalf("未知 kind 标题应回落「交付物」,得到 %q", label)
	}
	for _, kind := range []string{DossierRailKindConclusion, DossierRailKindEvidenceRef, DossierRailKindArtifactRef, "branch_ref", "git_commit"} {
		label := DossierRailKindLabel(kind)
		if strings.ContainsAny(label, "._") {
			t.Fatalf("槽标题 %q 疑似技术键", label)
		}
	}
}

// unknown 不是失败:无结果契约的任务计入 unknown,且不进 unfulfilled。
func TestDemandDossierHandoffSummaryCountsUnknownSeparately(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	undeclared := f.addTask("无契约任务", "completed", base)
	delivered := f.addTask("有交付任务", "completed", base)

	resultID := uuid.New()
	for i := range f.repo.tasks {
		if f.repo.tasks[i].ID == delivered.ID {
			f.repo.tasks[i].LatestTaskResultID = &resultID
		}
	}
	f.repo.projectTaskResults = append(f.repo.projectTaskResults, ProjectTaskResult{
		ID: resultID, TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: delivered.ID,
		Contract: TaskResultContract{Deliverables: []TaskResultDeliverable{{
			Name: "branch_ref", Kind: "branch_ref", Ref: "refs/heads/fix-login",
		}}},
	})

	dossier := f.get(t)

	if dossier.HandoffSummary.Unknown != 1 {
		t.Fatalf("无契约任务应计 unknown 一条,得到 %#v", dossier.HandoffSummary)
	}
	if dossier.HandoffSummary.Unfulfilled != 0 {
		t.Fatalf("unknown 不得被算成 unfulfilled: %#v", dossier.HandoffSummary)
	}
	if dossier.HandoffSummary.Fulfilled != 1 {
		t.Fatalf("已交付任务应计 fulfilled: %#v", dossier.HandoffSummary)
	}
	for _, assessment := range dossier.HandoffSummary.Assessments {
		if assessment.ProjectTaskID == undeclared.ID && assessment.ProjectTaskName != "无契约任务" {
			t.Fatalf("交接明细应补任务显示名,得到 %q", assessment.ProjectTaskName)
		}
	}
}

// 密度信号:活跃任务计数必须与 isTerminalProjectTaskStatus 同源(含 done/success),
// 另拼三值集会把任务永久算成执行中。
func TestDemandDossierSignalsUseFiveValueTerminalSet(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	f.addTask("完成一", "completed", base)
	f.addTask("完成二", "done", base)
	f.addTask("完成三", "success", base)
	f.addTask("取消", "cancelled", base)
	f.addTask("失败", "failed", base)
	f.addTask("仍在跑", "running", base)

	dossier := f.get(t)

	if dossier.Signals.ActiveTaskCount != 1 {
		t.Fatalf("只有 running 该算活跃,得到 %d", dossier.Signals.ActiveTaskCount)
	}
	if dossier.Signals.HasOpenDecisions {
		t.Fatalf("无待决决策时不该置位")
	}
	if dossier.Signals.DemandTerminal {
		t.Fatalf("需求非终态时 demand_terminal 应为 false")
	}
}

func TestDemandDossierSignalsFlagOpenDecisions(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC()
	task := f.addTask("等待确认", "waiting_human", base)
	f.repo.decisionRequests = append(f.repo.decisionRequests, DecisionRequest{
		ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, ProjectTaskID: &task.ID,
		TargetUserID: f.ownerID, DecisionType: "plan_review", TitleSnapshot: "确认执行计划",
		StatusSnapshot: "pending", CreatedAt: base,
	})

	dossier := f.get(t)

	if !dossier.Signals.HasOpenDecisions {
		t.Fatalf("有 pending 决策时应置位")
	}
	if len(dossier.PendingActions) != 1 {
		t.Fatalf("待你处理应有一条: %#v", dossier.PendingActions)
	}
	action := dossier.PendingActions[0]
	if action.Title != "确认执行计划" || action.Href.Type != "inbox" {
		t.Fatalf("待办应带标题与收件箱深链: %#v", action)
	}
	if action.Href.DemandID == nil || *action.Href.DemandID != f.demand.ID {
		t.Fatalf("深链应带需求身份: %#v", action.Href)
	}
}

// 需求级剧本覆盖项目级。
func TestDemandDossierPlaybookDemandOverridesProject(t *testing.T) {
	f := newDossierFixture(t)
	projectKey := "project_playbook"
	demandKey := "demand_playbook"
	f.repo.projects[f.projectID] = Project{
		ID: f.projectID, TenantID: f.tenantID, Name: "客服工单闭环", Status: ProjectStatusRunning,
		HumanOwnerUserID: f.ownerID, ScenarioTemplateKey: &projectKey,
	}
	for id, demand := range f.repo.demands {
		if demand.ID == f.demand.ID {
			demand.ScenarioTemplateKey = &demandKey
			f.repo.demands[id] = demand
		}
	}
	f.service.SetScenarioTemplateResolver(stubScenarioTemplateResolver{
		bindings: map[string]ScenarioTemplateBinding{
			projectKey: {Key: projectKey, Name: "项目剧本", Status: "active"},
			demandKey:  {Key: demandKey, Name: "需求剧本", Status: "active"},
		},
		produceKinds: map[string][]string{
			projectKey: {"conclusion"},
			demandKey:  {"evidence_ref"},
		},
	})

	dossier := f.get(t)

	if dossier.EffectivePlaybook.Source != DossierPlaybookSourceDemand {
		t.Fatalf("需求剧本应覆盖项目剧本,得到 %q", dossier.EffectivePlaybook.Source)
	}
	if dossier.EffectivePlaybook.Name != "需求剧本" {
		t.Fatalf("剧本名应为需求剧本,得到 %q", dossier.EffectivePlaybook.Name)
	}
}

// 剧本解析失败必须降级 none 而不是 500——卷宗其余事实与剧本无关。
func TestDemandDossierPlaybookDegradesToNoneOnResolverFailure(t *testing.T) {
	f := newDossierFixture(t)
	templateKey := "missing_playbook"
	f.repo.projects[f.projectID] = Project{
		ID: f.projectID, TenantID: f.tenantID, Name: "客服工单闭环", Status: ProjectStatusRunning,
		HumanOwnerUserID: f.ownerID, ScenarioTemplateKey: &templateKey,
	}
	f.service.SetScenarioTemplateResolver(stubScenarioTemplateResolver{
		bindings:     map[string]ScenarioTemplateBinding{},
		produceKinds: map[string][]string{},
	})

	dossier := f.get(t)

	if dossier.EffectivePlaybook.Source != DossierPlaybookSourceNone {
		t.Fatalf("解析失败应降级 none,得到 %q", dossier.EffectivePlaybook.Source)
	}
	if len(dossier.EffectivePlaybook.ProduceKinds) != 0 {
		t.Fatalf("降级后不得有槽位次序: %#v", dossier.EffectivePlaybook.ProduceKinds)
	}
}

// produce kinds 解析失败时保留剧本身份,右轨改按实际产物推导,仍不 500。
func TestDemandDossierPlaybookKeepsBindingWhenKindsFail(t *testing.T) {
	f := newDossierFixture(t)
	templateKey := "kinds_broken"
	f.repo.projects[f.projectID] = Project{
		ID: f.projectID, TenantID: f.tenantID, Name: "客服工单闭环", Status: ProjectStatusRunning,
		HumanOwnerUserID: f.ownerID, ScenarioTemplateKey: &templateKey,
	}
	f.service.SetScenarioTemplateResolver(stubScenarioTemplateResolver{
		bindings: map[string]ScenarioTemplateBinding{templateKey: {Key: templateKey, Name: "坏剧本", Status: "active"}},
		kindsErr: errors.New("spec 解析失败"),
	})

	dossier := f.get(t)

	if dossier.EffectivePlaybook.Source != DossierPlaybookSourceProject {
		t.Fatalf("剧本身份应保留,得到 %q", dossier.EffectivePlaybook.Source)
	}
	if len(dossier.EffectivePlaybook.ProduceKinds) != 0 {
		t.Fatalf("kinds 解析失败时应为空: %#v", dossier.EffectivePlaybook.ProduceKinds)
	}
}

func (f *dossierFixture) addPlanRevision(number int32, status string, payload map[string]any) PlanRevision {
	revision := PlanRevision{
		ID:             uuid.New(),
		TenantID:       f.tenantID,
		ProjectID:      f.projectID,
		DemandID:       f.demand.ID,
		RevisionNumber: number,
		Status:         status,
		Payload:        payload,
	}
	f.repo.planRevisions = append(f.repo.planRevisions, revision)
	return revision
}

func exitPayload(deliverable string) map[string]any {
	return map[string]any{
		"exit_deliverable": deliverable,
		"available_exits": []any{
			map[string]any{"deliverable": "branch_ref", "label": "交付分支(不合入)"},
			map[string]any{"deliverable": "review_verdict", "label": "审查通过并合入"},
			map[string]any{"deliverable": "release_record", "label": "发布上线"},
		},
	}
}

// 本单收口取**生效**修订,不是最新一版:被驳回后重规划的历史版本不该盖过已确认口径。
func TestDemandDossierExitComesFromEffectiveRevision(t *testing.T) {
	f := newDossierFixture(t)
	f.addPlanRevision(1, PlanRevisionStatusAccepted, exitPayload("review_verdict"))
	f.addPlanRevision(2, PlanRevisionStatusRejected, exitPayload("release_record"))

	playbook := f.get(t).EffectivePlaybook

	if playbook.ExitDeliverable != "review_verdict" {
		t.Fatalf("收口应取生效修订,得到 %q", playbook.ExitDeliverable)
	}
	if playbook.ExitLabel != "审查通过并合入" {
		t.Fatalf("收口标签应取载荷快照,得到 %q", playbook.ExitLabel)
	}
	if playbook.ExitPending {
		t.Fatal("已确认的收口不得标为待确认")
	}
}

// 还没有生效修订时退到最新一版并标 pending——停在"等计划确认"恰恰是人最需要
// 看见"这一单打算走多深"的时刻,留白等于在最该说话时闭嘴。
func TestDemandDossierExitFallsBackToLatestAndFlagsPending(t *testing.T) {
	f := newDossierFixture(t)
	f.addPlanRevision(1, PlanRevisionStatusSuperseded, exitPayload("branch_ref"))
	f.addPlanRevision(2, PlanRevisionStatusPendingReview, exitPayload("release_record"))

	playbook := f.get(t).EffectivePlaybook

	if playbook.ExitDeliverable != "release_record" {
		t.Fatalf("无生效修订应退到最新一版,得到 %q", playbook.ExitDeliverable)
	}
	if playbook.ExitLabel != "发布上线" {
		t.Fatalf("收口标签错误: %q", playbook.ExitLabel)
	}
	if !playbook.ExitPending {
		t.Fatal("待确认计划的收口必须标 pending,否则会把承诺显示成事实")
	}
}

// 模板被删导致剧本降级 none 时,本单当初收口到哪仍是既成事实,不该跟着消失。
func TestDemandDossierExitSurvivesPlaybookDegradation(t *testing.T) {
	f := newDossierFixture(t)
	templateKey := "missing_playbook"
	f.repo.projects[f.projectID] = Project{
		ID: f.projectID, TenantID: f.tenantID, Name: "客服工单闭环", Status: ProjectStatusRunning,
		HumanOwnerUserID: f.ownerID, ScenarioTemplateKey: &templateKey,
	}
	f.service.SetScenarioTemplateResolver(stubScenarioTemplateResolver{
		bindings:     map[string]ScenarioTemplateBinding{},
		produceKinds: map[string][]string{},
	})
	f.addPlanRevision(1, PlanRevisionStatusDecomposed, exitPayload("branch_ref"))

	playbook := f.get(t).EffectivePlaybook

	if playbook.Source != DossierPlaybookSourceNone {
		t.Fatalf("剧本应降级 none,得到 %q", playbook.Source)
	}
	if playbook.ExitDeliverable != "branch_ref" || playbook.ExitLabel != "交付分支(不合入)" {
		t.Fatalf("剧本降级不得抹掉本单收口: %q / %q", playbook.ExitDeliverable, playbook.ExitLabel)
	}
}

// 未规划的需求、以及无出口声明的计划(generic 剧本/无模板)都留空,不编造收口。
func TestDemandDossierExitEmptyWhenUndeclared(t *testing.T) {
	f := newDossierFixture(t)
	if got := f.get(t).EffectivePlaybook.ExitDeliverable; got != "" {
		t.Fatalf("未规划的需求不应有收口,得到 %q", got)
	}

	f.addPlanRevision(1, PlanRevisionStatusAccepted, map[string]any{"summary": "无出口声明"})
	playbook := f.get(t).EffectivePlaybook
	if playbook.ExitDeliverable != "" || playbook.ExitPending {
		t.Fatalf("无出口声明应留空: %q / pending=%v", playbook.ExitDeliverable, playbook.ExitPending)
	}
}

// available_exits 缺该项时保留技术键、标签留空,由展示侧兜底,不 panic 也不吞掉收口。
func TestDemandDossierExitKeepsKeyWhenLabelMissing(t *testing.T) {
	f := newDossierFixture(t)
	f.addPlanRevision(1, PlanRevisionStatusAccepted, map[string]any{
		"exit_deliverable": "root_cause",
		"available_exits":  []any{map[string]any{"deliverable": "fix_record", "label": "实施修复"}},
	})

	playbook := f.get(t).EffectivePlaybook

	if playbook.ExitDeliverable != "root_cause" {
		t.Fatalf("收口键应保留,得到 %q", playbook.ExitDeliverable)
	}
	if playbook.ExitLabel != "" {
		t.Fatalf("无匹配标签时应留空,得到 %q", playbook.ExitLabel)
	}
}

// 无剧本且无产物 → 空槽 + 空时间线不 panic;这是 automation 单的合法形态。
func TestDemandDossierEmptyDemandIsHonestNotBroken(t *testing.T) {
	f := newDossierFixture(t)
	dossier := f.get(t)

	if dossier.EffectivePlaybook.Source != DossierPlaybookSourceNone {
		t.Fatalf("无剧本应为 none,得到 %q", dossier.EffectivePlaybook.Source)
	}
	if len(dossier.Rail) != 0 {
		t.Fatalf("无产物应无槽位: %#v", dossier.Rail)
	}
	if dossier.Timeline.Items == nil {
		t.Fatalf("时间线不得为 nil(契约要求数组)")
	}
	if dossier.Timeline.Truncated {
		t.Fatalf("空时间线不该标记截断")
	}
}

// 左轨角标:需求级决策(plan_review 没有 project_task_id)必须经 plan_revision
// 归属到需求,否则最高频的待办会显示成 0。
func TestDemandDossierSiblingPendingCountsDemandLevelDecisions(t *testing.T) {
	f := newDossierFixture(t)
	revisionID := uuid.New()
	f.repo.planRevisions = append(f.repo.planRevisions, PlanRevision{
		ID: revisionID, TenantID: f.tenantID, ProjectID: f.projectID, DemandID: f.demand.ID,
		RevisionNumber: 1, Status: "pending_review",
	})
	f.repo.decisionRequests = append(f.repo.decisionRequests, DecisionRequest{
		ID: uuid.New(), TenantID: f.tenantID, ProjectID: f.projectID, PlanRevisionID: &revisionID,
		TargetUserID: f.ownerID, DecisionType: "plan_review", TitleSnapshot: "确认执行计划",
		StatusSnapshot: "pending",
	})

	dossier, err := f.service.GetDemandDossier(context.Background(), GetDemandDossierRequest{
		TenantID:       f.tenantID,
		DemandID:       f.demand.ID,
		SiblingPending: true,
	})
	if err != nil {
		t.Fatalf("get dossier: %v", err)
	}

	found := false
	for _, sibling := range dossier.SiblingPending {
		if sibling.DemandID != f.demand.ID {
			continue
		}
		found = true
		if sibling.OpenDecisions != 1 {
			t.Fatalf("需求级 plan_review 应计入角标,得到 %d", sibling.OpenDecisions)
		}
	}
	if !found {
		t.Fatalf("角标应包含本需求: %#v", dossier.SiblingPending)
	}
}

// timeline_limit 越界要被夹住,不能让调用方拉爆一次请求。
func TestDemandDossierClampsTimelineLimit(t *testing.T) {
	f := newDossierFixture(t)
	base := time.Now().UTC().Add(-time.Hour)
	task := f.addTask("任务", "running", base)
	for i := 0; i < 3; i++ {
		f.addEvent(ProjectEventTaskDispatched, "project_task", task.ID, base.Add(time.Duration(i)*time.Minute))
	}

	dossier, err := f.service.GetDemandDossier(context.Background(), GetDemandDossierRequest{
		TenantID:      f.tenantID,
		DemandID:      f.demand.ID,
		TimelineLimit: 100000,
	})
	if err != nil {
		t.Fatalf("get dossier: %v", err)
	}
	if dossier.Timeline.Truncated {
		t.Fatalf("条数少于上限时不该截断")
	}
}
