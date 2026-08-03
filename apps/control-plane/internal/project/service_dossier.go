package project

import (
	"context"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// 一单卷宗(Demand Dossier,spec 2026-07-29 R2)的只读聚合。
//
// 它回答"回到这一单"的两个问题:中栏时间线讲**发生了什么**(协调叙事,不是原始
// 事件流水),右轨讲**交出了什么**(按剧本 produces kind 分槽的交付事实)。两者
// 此前都无处可看:执行轨迹是项目级不按需求过滤,交接判定只藏在流程图节点浮层里。
//
// 只读、无写路径、无事务。取数与 launch-detail 共用 loadDemandLaunchFacts,
// 交接判定复用 buildProjectTaskGraphHandoffAssessments(不建全图),证据/工件走
// 按任务批量查询(不按项目分页后内存过滤——会静默截断成假"交付缺失")。

const (
	demandDossierDefaultTimelineLimit int32 = 60
	demandDossierMaxTimelineLimit     int32 = 200
	// 事件要多取一些:归一会滤掉噪音事件,按最终条数取会填不满时间线。
	demandDossierEventFetchFactor int32 = 3
	demandDossierMaxEventFetch    int32 = 400
)

// 剧本来源。none = 需求与项目都没绑剧本(automation 路径合法),右轨此时只按
// 实际产物推导,不假装有剧本。
const (
	DossierPlaybookSourceDemand  = "demand"
	DossierPlaybookSourceProject = "project"
	DossierPlaybookSourceNone    = "none"
)

// 右轨条目状态。unknown ≠ 失败:它表示"无声明,无法判定"。
const (
	DossierRailItemStateDelivered = "delivered"
	DossierRailItemStateMissing   = "missing"
	DossierRailItemStateUnknown   = "unknown"
	DossierRailItemStateInfo      = "info"
)

// 右轨槽 kind:剧本 produces_defaults 的 kind 词汇 + 卷宗自己的兜底槽。
const (
	DossierRailKindConclusion  = "conclusion"
	DossierRailKindEvidenceRef = "evidence_ref"
	DossierRailKindArtifactRef = "artifact_ref"
)

type GetDemandDossierRequest struct {
	TenantID uuid.UUID
	DemandID uuid.UUID
	// TimelineLimit 是归一**之后**的条数上限(不是 raw 事件数)。0 取默认。
	TimelineLimit int32
	// SiblingPending 为 true 时附带同项目各需求的待办计数(左轨角标)。
	SiblingPending bool
}

// DemandDossierLineage 是这一单所属的接续链（spec 2026-08-01 §7.2）。
// 「一单」的用户身份是链而不是行：不给出链，每接续一次就会在需求列表里多出
// 一个看似无关的单。
type DemandDossierLineage struct {
	ContinuesDemandID *uuid.UUID
	ChainPosition     int
	ChainLength       int
	Chain             []DemandDossierChainItem
	// ContinueDemand 是"这一单现在能不能接续"的服务端判据。前端不自己算：
	// 能不能接续是业务规则，散到前端就会两处不一致。
	ContinueDemand DemandContinuationAvailability
}

type DemandDossierChainItem struct {
	DemandID  uuid.UUID
	Title     string
	Status    string
	CreatedAt time.Time
	IsCurrent bool
}

type DemandDossier struct {
	Demand            ProjectDemand
	Project           Project
	Lineage           DemandDossierLineage
	EffectivePlaybook DemandDossierPlaybook
	Signals           DemandDossierSignals
	PendingActions    []DemandDossierPendingAction
	Timeline          DemandDossierTimeline
	Rail              []DemandDossierRailSlot
	HandoffSummary    DemandDossierHandoffSummary
	Acceptance        DemandDossierAcceptance
	SiblingPending    []DemandDossierSiblingPending
}

type DemandDossierPlaybook struct {
	TemplateKey  *string
	Source       string
	Name         string
	ProduceKinds []string
	// ExitDeliverable/ExitLabel 是**本单收口**:这一单打算走多深。基线 §4.2 把
	// 收口定为剧本内概念(不存在跨剧本的全局"深度"),而它又是"范围可感知"的
	// 核心信息——只显示剧本名等于只说了"按哪套打法",没说"打到哪一步"。
	//
	// 取数不碰模板:成案时 available_exits 已把模板 exits 的 label 快照进计划
	// 载荷,因此这里读到的是**当时承诺的**口径,不会被模板事后改版改写。
	ExitDeliverable string
	ExitLabel       string
	// ExitPending 表示这个收口来自尚未确认的计划修订(还在等人确认)。此时
	// 面向用户必须标注"拟",不能把待确认的承诺显示成既成事实。
	ExitPending bool
}

// DemandDossierSignals 是密度判定的**原料**,不是结论。密度是用户偏好,由前端
// 决定并允许用户切换;服务端硬判密度必然要回头再补"用户覆盖"。
type DemandDossierSignals struct {
	HasOpenDecisions bool
	ActiveTaskCount  int
	DemandTerminal   bool
}

type DemandDossierPendingAction struct {
	ID        uuid.UUID
	Kind      string
	Title     string
	Status    string
	CreatedAt time.Time
	Href      DemandDossierHref
}

type DemandDossierHref struct {
	Type       string
	DecisionID *uuid.UUID
	DemandID   *uuid.UUID
	ProjectID  *uuid.UUID
}

type DemandDossierTimeline struct {
	Items     []DemandDossierTimelineItem
	Truncated bool
}

type DemandDossierTimelineItem struct {
	ID               string
	OccurredAt       time.Time
	Kind             string
	Title            string
	Summary          string
	Severity         string
	ActorDisplayName string
	Entity           *DemandDossierTimelineEntity
	OpenTarget       *DemandDossierTimelineTarget
}

type DemandDossierTimelineEntity struct {
	Type string
	ID   string
	Name string
}

type DemandDossierTimelineTarget struct {
	Type       string
	TaskID     *uuid.UUID
	DecisionID *uuid.UUID
}

type DemandDossierRailSlot struct {
	Kind  string
	Title string
	Items []DemandDossierRailItem
}

type DemandDossierRailItem struct {
	ID              string
	Title           string
	Summary         string
	State           string
	Ref             string
	ProjectTaskID   *uuid.UUID
	ProjectTaskName string
}

type DemandDossierHandoffSummary struct {
	Fulfilled   int
	Partial     int
	Unfulfilled int
	Unknown     int
	Assessments []DemandDossierHandoffAssessment
}

type DemandDossierHandoffAssessment struct {
	ProjectTaskGraphHandoffAssessment
	ProjectTaskName string
}

type DemandDossierAcceptance struct {
	DemandStatus         string
	CriteriaTotal        int
	PendingHumanJudgment int
}

type DemandDossierSiblingPending struct {
	DemandID      uuid.UUID
	OpenDecisions int
	DemandTitle   string
	DemandStatus  string
}

// GetDemandDossier 聚合一条需求的只读卷宗。
func (s *Service) GetDemandDossier(ctx context.Context, req GetDemandDossierRequest) (*DemandDossier, error) {
	timelineLimit := req.TimelineLimit
	if timelineLimit <= 0 {
		timelineLimit = demandDossierDefaultTimelineLimit
	}
	if timelineLimit > demandDossierMaxTimelineLimit {
		timelineLimit = demandDossierMaxTimelineLimit
	}
	eventLimit := timelineLimit * demandDossierEventFetchFactor
	if eventLimit > demandDossierMaxEventFetch {
		eventLimit = demandDossierMaxEventFetch
	}

	facts, err := s.loadDemandLaunchFacts(ctx, req.TenantID, req.DemandID, eventLimit)
	if err != nil {
		return nil, err
	}

	names := s.resolveDemandDossierNames(ctx, facts)
	playbook := s.resolveDemandDossierPlaybook(ctx, facts)
	s.resolveDemandDossierExit(ctx, facts, &playbook)

	contracts, err := s.repository.ListLatestTaskResultContractsByTasks(ctx, req.TenantID, facts.Project.ID, facts.ProjectTasks)
	if err != nil {
		return nil, err
	}
	assessments := buildProjectTaskGraphHandoffAssessments(facts.ProjectTasks, contracts)

	evidence, err := s.repository.ListEvidenceRefsByTaskIDs(ctx, req.TenantID, facts.Project.ID, facts.TaskIDs)
	if err != nil {
		return nil, err
	}
	artifacts, err := s.repository.ListArtifactRefsByTaskIDs(ctx, req.TenantID, facts.Project.ID, facts.TaskIDs)
	if err != nil {
		return nil, err
	}

	dossier := &DemandDossier{
		Demand:            facts.Demand,
		Project:           facts.Project,
		Lineage:           s.resolveDemandDossierLineage(ctx, facts),
		EffectivePlaybook: playbook,
		Signals:           buildDemandDossierSignals(facts),
		PendingActions:    buildDemandDossierPendingActions(facts),
		Timeline:          buildDemandDossierTimeline(facts, names, timelineLimit),
		Rail:              buildDemandDossierRail(playbook, facts, assessments, evidence, artifacts, names),
		HandoffSummary:    buildDemandDossierHandoffSummary(assessments, names),
		Acceptance:        s.resolveDemandDossierAcceptance(ctx, req.TenantID, facts),
	}

	if req.SiblingPending {
		siblings, err := s.resolveDemandDossierSiblingPending(ctx, req.TenantID, facts.Project.ID)
		if err != nil {
			return nil, err
		}
		dossier.SiblingPending = siblings
	}
	return dossier, nil
}

// demandDossierNames 是本单涉及实体的显示名字典。面向用户的字段一律经它补名:
// 裸 UUID 不得出现在时间线标题、右轨条目或交接明细里。
type demandDossierNames struct {
	tasks   map[uuid.UUID]string
	actors  map[string]string
	demands map[uuid.UUID]string
}

func (n demandDossierNames) task(id uuid.UUID) string {
	if name, ok := n.tasks[id]; ok {
		return name
	}
	return ""
}

func (n demandDossierNames) actor(actorType, actorID string) string {
	if name, ok := n.actors[strings.TrimSpace(actorID)]; ok && name != "" {
		return name
	}
	// 没有可解析的身份时给角色化中文,而不是回吐 UUID。协调线程类 actor 的
	// actor_id 是 workflow/job 标识,永远匹配不到项目成员,必须靠这里兜底。
	switch strings.TrimSpace(strings.ToLower(actorType)) {
	case "system", "workflow", "coordinator", "project_coordinator", "workflow_coordinator":
		return "协调线程"
	case "digital_employee":
		return "数字员工"
	case "user", "human", "human_user":
		return "成员"
	default:
		return ""
	}
}

func (s *Service) resolveDemandDossierNames(ctx context.Context, facts *demandLaunchFacts) demandDossierNames {
	names := demandDossierNames{
		tasks:   make(map[uuid.UUID]string, len(facts.ProjectTasks)),
		actors:  map[string]string{},
		demands: map[uuid.UUID]string{},
	}
	for _, task := range facts.ProjectTasks {
		names.tasks[task.ID] = strings.TrimSpace(task.Title)
	}
	names.demands[facts.Demand.ID] = strings.TrimSpace(facts.Demand.Title)

	// 项目成员一次取齐:人类与数字员工的显示名都在 DisplayNameSnapshot 上,
	// 一个查询同时覆盖事件 actor 的两种主体类型。
	members, err := s.repository.ListProjectMembers(ctx, facts.Project.TenantID, facts.Project.ID)
	if err != nil {
		// 补名失败不该让整张卷宗 500:退化成角色化中文,仍不暴露 UUID。
		slog.WarnContext(ctx, "一单卷宗补名失败,退化为角色化文案",
			"project_id", facts.Project.ID, "error", err)
		return names
	}
	for _, member := range members {
		if member.DisplayNameSnapshot == nil {
			continue
		}
		displayName := strings.TrimSpace(*member.DisplayNameSnapshot)
		if displayName == "" {
			continue
		}
		names.actors[member.PrincipalID.String()] = displayName
	}
	return names
}

// resolveDemandDossierPlaybook 解析有效剧本:需求覆盖项目,都没有则 none。
// 解析失败(模板被删/spec 不可解析/resolver 未接线)一律降级 none 并记 warn,
// 不 500——卷宗的其余事实与剧本无关,不该因为剧本读不出来整页打不开。
func (s *Service) resolveDemandDossierPlaybook(ctx context.Context, facts *demandLaunchFacts) DemandDossierPlaybook {
	playbook := DemandDossierPlaybook{Source: DossierPlaybookSourceNone, ProduceKinds: []string{}}

	var key string
	if facts.Demand.ScenarioTemplateKey != nil && strings.TrimSpace(*facts.Demand.ScenarioTemplateKey) != "" {
		key = strings.TrimSpace(*facts.Demand.ScenarioTemplateKey)
		playbook.Source = DossierPlaybookSourceDemand
	} else if facts.Project.ScenarioTemplateKey != nil && strings.TrimSpace(*facts.Project.ScenarioTemplateKey) != "" {
		key = strings.TrimSpace(*facts.Project.ScenarioTemplateKey)
		playbook.Source = DossierPlaybookSourceProject
	}
	if key == "" {
		return playbook
	}
	if s.scenarioTemplates == nil {
		playbook.Source = DossierPlaybookSourceNone
		return playbook
	}

	playbook.TemplateKey = &key
	binding, err := s.scenarioTemplates.ResolveScenarioTemplate(ctx, facts.Project.TenantID, key)
	if err != nil {
		slog.WarnContext(ctx, "一单卷宗剧本解析失败,降级为无剧本",
			"template_key", key, "error", err)
		return DemandDossierPlaybook{Source: DossierPlaybookSourceNone, ProduceKinds: []string{}}
	}
	playbook.Name = binding.Name

	kinds, err := s.scenarioTemplates.ResolveScenarioTemplateProduceKinds(ctx, facts.Project.TenantID, key)
	if err != nil {
		slog.WarnContext(ctx, "一单卷宗剧本产出 kind 解析失败,右轨改按实际产物推导",
			"template_key", key, "error", err)
		return playbook
	}
	playbook.ProduceKinds = dedupeStringsPreservingOrder(kinds)
	return playbook
}

// resolveDemandDossierLineage 解析这一单所属的接续链。
//
// 链读不出来时降级成"只有本单的单元素链"并记 warn，不 500：卷宗的其余事实与
// 血缘无关，不该因为链查询失败整页打不开（与剧本解析失败同口径）。
func (s *Service) resolveDemandDossierLineage(ctx context.Context, facts *demandLaunchFacts) DemandDossierLineage {
	demand := facts.Demand
	solo := DemandDossierLineage{
		ContinuesDemandID: demand.ContinuesDemandID,
		ChainPosition:     1,
		ChainLength:       1,
		Chain: []DemandDossierChainItem{{
			DemandID:  demand.ID,
			Title:     strings.TrimSpace(demand.Title),
			Status:    string(demand.Status),
			CreatedAt: demand.CreatedAt,
			IsCurrent: true,
		}},
		ContinueDemand: evaluateDemandContinuation(demand, 0),
	}

	chain, err := s.repository.ListProjectDemandContinuationChain(ctx, demand.TenantID, demand.ID, DefaultDemandContinuationMaxDepth)
	if err != nil {
		slog.WarnContext(ctx, "一单卷宗接续链读取失败,降级为单元素链",
			"demand_id", demand.ID, "error", err)
		return solo
	}
	if len(chain) == 0 {
		return solo
	}

	lineage := DemandDossierLineage{
		ContinuesDemandID: demand.ContinuesDemandID,
		ChainLength:       len(chain),
		Chain:             make([]DemandDossierChainItem, 0, len(chain)),
	}
	for index, item := range chain {
		isCurrent := item.ID == demand.ID
		if isCurrent {
			lineage.ChainPosition = index + 1
		}
		lineage.Chain = append(lineage.Chain, DemandDossierChainItem{
			DemandID:  item.ID,
			Title:     strings.TrimSpace(item.Title),
			Status:    string(item.Status),
			CreatedAt: item.CreatedAt,
			IsCurrent: isCurrent,
		})
	}
	if lineage.ChainPosition == 0 {
		// 本单不在返回的链里(不该发生)：宁可退回单元素链，也不给出一条不含
		// 自己的"链"——那会让前端把高亮画到别人身上。
		return solo
	}
	// 接续判据按**本单**算：链上位置即已用掉的代数。
	lineage.ContinueDemand = evaluateDemandContinuation(demand, int32(lineage.ChainPosition-1))
	return lineage
}

// resolveDemandDossierExit 填充本单收口。**独立于剧本解析成败**:模板被删或
// spec 不可解析时剧本降级 none,但这一单当初收口到哪是已成事实,不该跟着消失。
//
// 取生效计划修订(accepted/decomposing/decomposed)的收口;还没有生效修订时退到
// 最新一版并标 ExitPending——需求停在"等计划确认"恰恰是人最需要看见"这一单
// 打算走多深"的时刻,那时把收口留白等于在最该说话时闭嘴。
//
// 这里多打一次 ListPlanRevisionsForDemand(验收摘要那条路径内部也会取一次)。
// 按需求主键的小表读、有索引,换掉的是往剧本服务再要一次模板解析;若之后要
// 收敛,应把修订列表提到 GetDemandDossier 里一次取齐后向下传,而不是把收口
// 改回按模板现值渲染(那会让历史单显示改版后的口径)。
func (s *Service) resolveDemandDossierExit(ctx context.Context, facts *demandLaunchFacts, playbook *DemandDossierPlaybook) {
	revisions, err := s.repository.ListPlanRevisionsForDemand(ctx, facts.Project.TenantID, facts.Project.ID, facts.Demand.ID)
	if err != nil {
		slog.WarnContext(ctx, "一单卷宗收口读取失败,单头不显示收口",
			"demand_id", facts.Demand.ID, "error", err)
		return
	}
	revision, pending := currentOrLatestPlanRevision(revisions)
	if revision == nil {
		return
	}
	deliverable, label := planRevisionExit(revision.Payload)
	if deliverable == "" {
		return
	}
	playbook.ExitDeliverable = deliverable
	playbook.ExitLabel = label
	playbook.ExitPending = pending
}

// currentOrLatestPlanRevision 返回生效修订;没有生效修订时返回版本号最大的一版
// 并置 pending=true。空列表返回 nil(未规划的需求没有收口可言)。
func currentOrLatestPlanRevision(revisions []PlanRevision) (*PlanRevision, bool) {
	effectiveID := CurrentEffectivePlanRevisionID(revisions)
	var latest *PlanRevision
	for i := range revisions {
		if effectiveID != uuid.Nil && revisions[i].ID == effectiveID {
			return &revisions[i], false
		}
		if latest == nil || revisions[i].RevisionNumber > latest.RevisionNumber {
			latest = &revisions[i]
		}
	}
	if latest == nil {
		return nil, false
	}
	return latest, true
}

// planRevisionExit 取计划载荷的 exit_deliverable 及其在 available_exits 里的
// 中文 label(见 projectcoordination.PlanRevisionPayload / PlanExitOption)。
// 载荷里没有匹配项时 label 留空,由展示侧决定是否回退显示技术键。
func planRevisionExit(payload map[string]any) (string, string) {
	deliverable, _ := payload["exit_deliverable"].(string)
	deliverable = strings.TrimSpace(deliverable)
	if deliverable == "" {
		return "", ""
	}
	raw, _ := payload["available_exits"].([]any)
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		key, _ := entry["deliverable"].(string)
		if strings.TrimSpace(key) != deliverable {
			continue
		}
		label, _ := entry["label"].(string)
		return deliverable, strings.TrimSpace(label)
	}
	return deliverable, ""
}

func buildDemandDossierSignals(facts *demandLaunchFacts) DemandDossierSignals {
	signals := DemandDossierSignals{
		DemandTerminal: isTerminalProjectDemandStatus(facts.Demand.Status),
	}
	for _, decision := range facts.DecisionRequests {
		if isOpenDecisionStatus(decision.StatusSnapshot) {
			signals.HasOpenDecisions = true
			break
		}
	}
	for _, task := range facts.ProjectTasks {
		// 终态集必须与 isTerminalProjectTaskStatus 同源(含 done/success):
		// 另拼一套三值集会把任务永久算成"执行中"。
		if !isTerminalProjectTaskStatus(task.Status) {
			signals.ActiveTaskCount++
		}
	}
	return signals
}

func isOpenDecisionStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "pending")
}

func isTerminalProjectDemandStatus(status ProjectDemandStatus) bool {
	switch status {
	case ProjectDemandStatusCompleted, ProjectDemandStatusFailed, ProjectDemandStatusCancelled:
		return true
	default:
		return false
	}
}

func buildDemandDossierPendingActions(facts *demandLaunchFacts) []DemandDossierPendingAction {
	actions := make([]DemandDossierPendingAction, 0)
	projectID := facts.Project.ID
	demandID := facts.Demand.ID
	for _, decision := range facts.DecisionRequests {
		if !isOpenDecisionStatus(decision.StatusSnapshot) {
			continue
		}
		decisionID := decision.ID
		actions = append(actions, DemandDossierPendingAction{
			ID:        decision.ID,
			Kind:      decision.DecisionType,
			Title:     strings.TrimSpace(decision.TitleSnapshot),
			Status:    decision.StatusSnapshot,
			CreatedAt: decision.CreatedAt,
			Href: DemandDossierHref{
				Type:       "inbox",
				DecisionID: &decisionID,
				DemandID:   &demandID,
				ProjectID:  &projectID,
			},
		})
	}
	sort.SliceStable(actions, func(i, j int) bool {
		return actions[i].CreatedAt.After(actions[j].CreatedAt)
	})
	return actions
}

// buildDemandDossierTimeline 把事件流归一成协调叙事:噪音事件不进(仍在执行轨迹
// 里可查),决策事件按 decision_type 精化,任务终态在事件缺失时用实体状态补一条
// 合成条目(id 前缀 synthetic:,与事件条目共存时以事件为准、不双计)。
func buildDemandDossierTimeline(facts *demandLaunchFacts, names demandDossierNames, limit int32) DemandDossierTimeline {
	decisionsByID := make(map[uuid.UUID]DecisionRequest, len(facts.DecisionRequests))
	decisionsByPlanRevision := make(map[uuid.UUID]DecisionRequest, len(facts.DecisionRequests))
	for _, decision := range facts.DecisionRequests {
		decisionsByID[decision.ID] = decision
		if decision.PlanRevisionID != nil {
			decisionsByPlanRevision[*decision.PlanRevisionID] = decision
		}
	}

	items := make([]DemandDossierTimelineItem, 0, len(facts.Events))
	kindsByTask := map[uuid.UUID]map[string]bool{}

	for _, event := range facts.Events {
		narrative := NarrateProjectEventType(event.EventType)
		if narrative.Noise {
			continue
		}
		item := DemandDossierTimelineItem{
			ID:               event.ID.String(),
			OccurredAt:       event.CreatedAt,
			Kind:             narrative.Kind,
			Title:            narrative.Title,
			Severity:         narrative.Severity,
			ActorDisplayName: names.actor(event.ActorType, event.ActorID),
		}
		if event.Summary != nil {
			item.Summary = strings.TrimSpace(*event.Summary)
		}
		applyDemandDossierTimelineEntity(&item, event, facts, names, decisionsByID, decisionsByPlanRevision)
		items = append(items, item)

		if item.Entity != nil && item.Entity.Type == "task" {
			if taskID, err := uuid.Parse(item.Entity.ID); err == nil {
				if kindsByTask[taskID] == nil {
					kindsByTask[taskID] = map[string]bool{}
				}
				kindsByTask[taskID][item.Kind] = true
			}
		}
	}

	// 事件窗口之外(或历史事件缺失)的任务终态用实体状态回填,否则一条早就完成
	// 的任务会在时间线里凭空消失。
	for _, task := range facts.ProjectTasks {
		kind, title, severity, ok := terminalTaskNarrative(task.Status)
		if !ok || kindsByTask[task.ID][kind] {
			continue
		}
		taskID := task.ID
		items = append(items, DemandDossierTimelineItem{
			ID:         "synthetic:" + kind + ":" + task.ID.String(),
			OccurredAt: task.StatusChangedAt,
			Kind:       kind,
			Title:      title,
			Severity:   severity,
			Entity: &DemandDossierTimelineEntity{
				Type: "task",
				ID:   task.ID.String(),
				Name: names.task(task.ID),
			},
			OpenTarget: &DemandDossierTimelineTarget{Type: "task_detail", TaskID: &taskID},
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})

	timeline := DemandDossierTimeline{Items: items}
	if int32(len(items)) > limit {
		timeline.Items = items[:limit]
		timeline.Truncated = true
	}
	if timeline.Items == nil {
		timeline.Items = []DemandDossierTimelineItem{}
	}
	return timeline
}

// applyDemandDossierTimelineEntity 补实体身份与打开目标,并对决策事件按
// decision_type 精化文案(基础表按事件类型给通用文案,细分要看决策本体)。
//
// 身份来源有两条:事件的 resource_type/resource_id,以及 payload 里的外键。
// 两条都得认——现网 decision.requested 就不带 resource_*,只在 payload 里放
// plan_revision_id;只认 resource_* 会让"待计划确认"这条最该被人一眼认出的
// 条目退回成通用的「待人工决策」。
func applyDemandDossierTimelineEntity(
	item *DemandDossierTimelineItem,
	event ProjectEvent,
	facts *demandLaunchFacts,
	names demandDossierNames,
	decisionsByID map[uuid.UUID]DecisionRequest,
	decisionsByPlanRevision map[uuid.UUID]DecisionRequest,
) {
	resourceType := ""
	if event.ResourceType != nil {
		resourceType = strings.TrimSpace(*event.ResourceType)
	}
	resourceID := ""
	if event.ResourceID != nil {
		resourceID = strings.TrimSpace(*event.ResourceID)
	}

	attachTask := func(taskID uuid.UUID) {
		name := names.task(taskID)
		id := taskID
		item.Entity = &DemandDossierTimelineEntity{Type: "task", ID: id.String(), Name: name}
		item.OpenTarget = &DemandDossierTimelineTarget{Type: "task_detail", TaskID: &id}
		if name != "" {
			item.Title = item.Title + " · " + name
		}
	}
	attachDecision := func(decision DecisionRequest) {
		id := decision.ID
		item.Entity = &DemandDossierTimelineEntity{
			Type: "decision",
			ID:   id.String(),
			Name: strings.TrimSpace(decision.TitleSnapshot),
		}
		item.OpenTarget = &DemandDossierTimelineTarget{Type: "decision", DecisionID: &id}
		if refined, refinedKind, ok := refineDecisionNarrative(item.Kind, decision); ok {
			item.Title = refined
			item.Kind = refinedKind
		}
	}

	if parsedID, err := uuid.Parse(resourceID); err == nil {
		switch {
		case strings.Contains(resourceType, "task"):
			attachTask(parsedID)
			return
		case strings.Contains(resourceType, "decision"):
			if decision, ok := decisionsByID[parsedID]; ok {
				attachDecision(decision)
			} else {
				id := parsedID
				item.Entity = &DemandDossierTimelineEntity{Type: "decision", ID: id.String()}
				item.OpenTarget = &DemandDossierTimelineTarget{Type: "decision", DecisionID: &id}
			}
			return
		case strings.Contains(resourceType, "demand"):
			item.Entity = &DemandDossierTimelineEntity{
				Type: "demand",
				ID:   parsedID.String(),
				Name: names.demands[parsedID],
			}
			return
		}
	}

	// payload 兜底:按外键找回身份。
	if decisionID, ok := payloadUUID(event.Payload, "decision_request_id"); ok {
		if decision, found := decisionsByID[decisionID]; found {
			attachDecision(decision)
			return
		}
	}
	if revisionID, ok := payloadUUID(event.Payload, "plan_revision_id"); ok {
		if decision, found := decisionsByPlanRevision[revisionID]; found {
			attachDecision(decision)
			return
		}
	}
	if taskID, ok := payloadUUID(event.Payload, "project_task_id"); ok {
		attachTask(taskID)
		return
	}
	if demandID, ok := payloadUUID(event.Payload, "demand_id"); ok && demandID == facts.Demand.ID {
		item.Entity = &DemandDossierTimelineEntity{
			Type: "demand",
			ID:   demandID.String(),
			Name: names.demands[demandID],
		}
	}
}

func payloadUUID(payload map[string]any, key string) (uuid.UUID, bool) {
	raw, ok := payload[key]
	if !ok {
		return uuid.Nil, false
	}
	text, ok := raw.(string)
	if !ok {
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(strings.TrimSpace(text))
	if err != nil {
		return uuid.Nil, false
	}
	return parsed, true
}

// refineDecisionNarrative 把通用决策文案细分到具体决策语义。计划确认是最高频
// 的人类动作,必须一眼可辨。
func refineDecisionNarrative(kind string, decision DecisionRequest) (string, string, bool) {
	decisionType := strings.TrimSpace(strings.ToLower(decision.DecisionType))
	resolved := !isOpenDecisionStatus(decision.StatusSnapshot)
	switch decisionType {
	case "plan_review":
		if kind == TimelineKindDecisionOpened {
			return "待计划确认", TimelineKindDecisionOpened, true
		}
		if resolved && kind == TimelineKindDecisionResolved {
			if strings.EqualFold(decision.StatusSnapshot, "approved") {
				return "计划已确认", TimelineKindPlanAccepted, true
			}
			if strings.EqualFold(decision.StatusSnapshot, "rejected") {
				return "计划被驳回", TimelineKindPlanRejected, true
			}
		}
	case "demand_acceptance":
		if kind == TimelineKindDecisionOpened {
			return "待验收签署", TimelineKindDecisionOpened, true
		}
	}
	return "", "", false
}

func terminalTaskNarrative(status string) (kind, title, severity string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "done", "success":
		return TimelineKindTaskCompleted, "任务完成", NarrativeSeveritySuccess, true
	case "failed":
		return TimelineKindTaskFailed, "任务失败", NarrativeSeverityDanger, true
	case "cancelled":
		return TimelineKindTaskCancelled, "任务取消", NarrativeSeverityMute, true
	default:
		return "", "", "", false
	}
}

// buildDemandDossierRail 装配右轨槽位:先按有效剧本的 produces kind 保序,再把
// 实际出现但剧本没声明的 kind 追加在后。无剧本且无产物 → 空槽列表,由前端渲染
// 诚实空态,而不是编造领域文案。
func buildDemandDossierRail(
	playbook DemandDossierPlaybook,
	facts *demandLaunchFacts,
	assessments []ProjectTaskGraphHandoffAssessment,
	evidence []ProjectEvidenceRef,
	artifacts []ProjectArtifactRef,
	names demandDossierNames,
) []DemandDossierRailSlot {
	itemsByKind := map[string][]DemandDossierRailItem{}
	order := make([]string, 0, len(playbook.ProduceKinds))
	seenKind := map[string]bool{}
	appendKind := func(kind string) {
		if kind == "" || seenKind[kind] {
			return
		}
		seenKind[kind] = true
		order = append(order, kind)
	}
	for _, kind := range playbook.ProduceKinds {
		appendKind(kind)
	}

	// 1) 交接声明:交付物自带 kind,是最结构化的一手事实。
	for _, assessment := range assessments {
		taskID := assessment.ProjectTaskID
		taskName := names.task(taskID)
		for _, deliverable := range assessment.Deliverables {
			kind := strings.TrimSpace(deliverable.Kind)
			if kind == "" {
				kind = DossierRailKindConclusion
			}
			appendKind(kind)
			state := DossierRailItemStateMissing
			if deliverable.Verdict == ProjectTaskGraphHandoffDeliverableDelivered {
				state = DossierRailItemStateDelivered
			}
			id := taskID.String() + ":" + deliverable.Name
			itemsByKind[kind] = append(itemsByKind[kind], DemandDossierRailItem{
				ID:              id,
				Title:           firstNonEmpty(deliverable.Name, "未命名交付物"),
				Summary:         deliverable.Summary,
				State:           state,
				Ref:             deliverable.Ref,
				ProjectTaskID:   &taskID,
				ProjectTaskName: taskName,
			})
		}
	}

	// 2) 执行结论:没有结构化声明时,结论仍是人要看的东西。
	for _, summary := range facts.ExecutionSummaries {
		conclusion := strings.TrimSpace(summary.Conclusion)
		if conclusion == "" {
			continue
		}
		appendKind(DossierRailKindConclusion)
		taskID := summary.ProjectTaskID
		itemsByKind[DossierRailKindConclusion] = append(itemsByKind[DossierRailKindConclusion], DemandDossierRailItem{
			ID:              "summary:" + summary.ID.String(),
			Title:           firstNonEmpty(names.task(taskID), "执行结论"),
			Summary:         conclusion,
			State:           DossierRailItemStateInfo,
			ProjectTaskID:   &taskID,
			ProjectTaskName: names.task(taskID),
		})
	}

	// 3) 证据与工件:按本单任务批量取回,已在仓储层过滤,不会掺进别的需求。
	for _, ref := range evidence {
		appendKind(DossierRailKindEvidenceRef)
		item := DemandDossierRailItem{
			ID:    "evidence:" + ref.ID.String(),
			Title: firstNonEmpty(strings.TrimSpace(ref.Title), "证据"),
			State: DossierRailItemStateDelivered,
			Ref:   strings.TrimSpace(ref.SourceRef),
		}
		if ref.Summary != nil {
			item.Summary = strings.TrimSpace(*ref.Summary)
		}
		if ref.ProjectTaskID != nil {
			taskID := *ref.ProjectTaskID
			item.ProjectTaskID = &taskID
			item.ProjectTaskName = names.task(taskID)
		}
		itemsByKind[DossierRailKindEvidenceRef] = append(itemsByKind[DossierRailKindEvidenceRef], item)
	}
	for _, ref := range artifacts {
		appendKind(DossierRailKindArtifactRef)
		item := DemandDossierRailItem{
			ID:    "artifact:" + ref.ID.String(),
			Title: firstNonEmpty(strings.TrimSpace(ref.Title), "工件"),
			State: DossierRailItemStateDelivered,
			Ref:   strings.TrimSpace(ref.ObjectRef),
		}
		if ref.ProjectTaskID != nil {
			taskID := *ref.ProjectTaskID
			item.ProjectTaskID = &taskID
			item.ProjectTaskName = names.task(taskID)
		}
		itemsByKind[DossierRailKindArtifactRef] = append(itemsByKind[DossierRailKindArtifactRef], item)
	}

	slots := make([]DemandDossierRailSlot, 0, len(order))
	for _, kind := range order {
		items := itemsByKind[kind]
		if items == nil {
			items = []DemandDossierRailItem{}
		}
		slots = append(slots, DemandDossierRailSlot{
			Kind:  kind,
			Title: DossierRailKindLabel(kind),
			Items: items,
		})
	}
	return slots
}

// DossierRailKindLabel 给右轨槽的中文标题。未知 kind 回落通用「交付物」而不是
// 把技术键当标题吐给用户;kind 原文仍在 slot.kind 上供前端做技术判别。
func DossierRailKindLabel(kind string) string {
	switch strings.TrimSpace(kind) {
	case DossierRailKindConclusion:
		return "结论"
	case DossierRailKindEvidenceRef:
		return "证据"
	case DossierRailKindArtifactRef:
		return "工件"
	case "branch_ref":
		return "分支"
	case "git_commit":
		return "提交"
	case "report_ref":
		return "报告"
	case "decision_record":
		return "决策记录"
	default:
		return "交付物"
	}
}

func buildDemandDossierHandoffSummary(assessments []ProjectTaskGraphHandoffAssessment, names demandDossierNames) DemandDossierHandoffSummary {
	summary := DemandDossierHandoffSummary{Assessments: make([]DemandDossierHandoffAssessment, 0, len(assessments))}
	for _, assessment := range assessments {
		switch assessment.Status {
		case ProjectTaskGraphHandoffStatusFulfilled:
			summary.Fulfilled++
		case ProjectTaskGraphHandoffStatusPartial:
			summary.Partial++
		case ProjectTaskGraphHandoffStatusUnfulfilled:
			summary.Unfulfilled++
		default:
			summary.Unknown++
		}
		summary.Assessments = append(summary.Assessments, DemandDossierHandoffAssessment{
			ProjectTaskGraphHandoffAssessment: assessment,
			ProjectTaskName:                   names.task(assessment.ProjectTaskID),
		})
	}
	return summary
}

// resolveDemandDossierAcceptance 只给瘦摘要(总数 + 待人工签署数);明细继续由
// acceptance-criteria 端点提供,卷宗不复制一份判据模型。
func (s *Service) resolveDemandDossierAcceptance(ctx context.Context, tenantID uuid.UUID, facts *demandLaunchFacts) DemandDossierAcceptance {
	acceptance := DemandDossierAcceptance{DemandStatus: string(facts.Demand.Status)}
	detail, err := s.ListDemandAcceptanceCriteriaDetail(ctx, tenantID, facts.Demand.ID)
	if err != nil {
		slog.WarnContext(ctx, "一单卷宗验收摘要读取失败,仅回状态",
			"demand_id", facts.Demand.ID, "error", err)
		return acceptance
	}
	acceptance.CriteriaTotal = len(detail.Criteria)
	for _, criterion := range detail.Criteria {
		if criterion.Verdict == nil && strings.EqualFold(strings.TrimSpace(valueOrEmpty(criterion.JudgeType)), "human_judgment") {
			acceptance.PendingHumanJudgment++
		}
	}
	return acceptance
}

// resolveDemandDossierSiblingPending 算同项目每条需求的待办数(左轨角标)。
//
// 归属靠两条链一起补齐:任务级决策经 project_task.demand_id,需求级决策(最常见
// 的 plan_review / demand_acceptance 没有 project_task_id)经
// plan_revision.demand_id。只认前者会把计划确认漏掉——而那正是最高频的待办,
// 角标显示 0 会让人以为这一单不用管。
func (s *Service) resolveDemandDossierSiblingPending(ctx context.Context, tenantID, projectID uuid.UUID) ([]DemandDossierSiblingPending, error) {
	demands, err := s.repository.ListProjectDemands(ctx, tenantID, projectID, 200, 0)
	if err != nil {
		return nil, err
	}
	decisions, err := s.repository.ListDecisionRequests(ctx, tenantID, projectID, 500, 0)
	if err != nil {
		return nil, err
	}
	tasks, err := s.repository.ListProjectTasks(ctx, tenantID, projectID, nil, 500, 0)
	if err != nil {
		return nil, err
	}
	revisions, err := s.repository.ListPlanRevisions(ctx, ListPlanRevisionsRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     500,
	})
	if err != nil {
		return nil, err
	}

	demandByTask := make(map[uuid.UUID]uuid.UUID, len(tasks))
	for _, task := range tasks {
		if task.DemandID != nil {
			demandByTask[task.ID] = *task.DemandID
		}
	}
	demandByRevision := make(map[uuid.UUID]uuid.UUID, len(revisions))
	for _, revision := range revisions {
		demandByRevision[revision.ID] = revision.DemandID
	}

	counts := map[uuid.UUID]int{}
	for _, decision := range decisions {
		if !isOpenDecisionStatus(decision.StatusSnapshot) {
			continue
		}
		switch {
		case decision.ProjectTaskID != nil:
			if demandID, ok := demandByTask[*decision.ProjectTaskID]; ok {
				counts[demandID]++
			}
		case decision.PlanRevisionID != nil:
			if demandID, ok := demandByRevision[*decision.PlanRevisionID]; ok {
				counts[demandID]++
			}
		}
	}

	siblings := make([]DemandDossierSiblingPending, 0, len(demands))
	for _, demand := range demands {
		siblings = append(siblings, DemandDossierSiblingPending{
			DemandID:      demand.ID,
			OpenDecisions: counts[demand.ID],
			DemandTitle:   demand.Title,
			DemandStatus:  string(demand.Status),
		})
	}
	return siblings, nil
}

func dedupeStringsPreservingOrder(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		result = append(result, trimmed)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
