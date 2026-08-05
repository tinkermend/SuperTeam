package project

// 事件叙事归一表(spec 2026-07-29 R2 §5.4/§5.5)。
//
// 这是项目事件 → 用户可读中文叙事的**唯一**映射来源:需求 dossier 的时间线与
// 项目级事件流都读它。此前项目级文案表(web 侧 opsEventTitle)只覆盖 8 个事件
// 类型、default 分支把 event_type 原样吐给用户,于是
// "project_task.dispatch_gate.replan_required" 这类英文蛇形串直接出现在页面
// 上,违反中文优先宪法。表放服务端而不是前端,是因为同一份文案要被需求级与
// 项目级两条读路径共用,放前端必然长出第二份并漂移。
//
// 表只按事件类型给"基础叙事"(不含实体名):调用方按需拼实体显示名,例如
// "任务完成 · 编写接口文档"。需要 payload 才能细分的语义(如 plan_review 决策
// 与普通决策)由调用方在基础叙事上再精化,保持本表纯函数、可全枚举测试。

// 时间线 kind:与 contracts/control-plane/openapi.yaml 的
// ProjectDemandDossierTimelineItem.kind 枚举逐字对应。
const (
	TimelineKindDemandSubmitted     = "demand_submitted"
	TimelineKindCoordinationStarted = "coordination_started"
	TimelineKindPlanReady           = "plan_ready"
	TimelineKindPlanAccepted        = "plan_accepted"
	TimelineKindPlanRejected        = "plan_rejected"
	TimelineKindPlanChangeRequested = "plan_change_requested"
	TimelineKindTaskCreated         = "task_created"
	TimelineKindTaskDispatched      = "task_dispatched"
	TimelineKindTaskWaitingHuman    = "task_waiting_human"
	TimelineKindTaskCompleted       = "task_completed"
	TimelineKindTaskFailed          = "task_failed"
	TimelineKindTaskCancelled       = "task_cancelled"
	TimelineKindResultRecorded      = "result_recorded"
	TimelineKindResultAccepted      = "result_accepted"
	TimelineKindResultRejected      = "result_rejected"
	TimelineKindDecisionOpened      = "decision_opened"
	TimelineKindDecisionResolved    = "decision_resolved"
	TimelineKindDispatchBlocked     = "dispatch_blocked"
	TimelineKindStaffingGap         = "staffing_gap"
	TimelineKindCoordinationBlocked = "coordination_blocked"
	TimelineKindOther               = "other"
)

// 叙事严重度:与前端 StatusPill / 时间线着色的 tone 口径一致。
const (
	NarrativeSeverityInfo    = "info"
	NarrativeSeveritySuccess = "success"
	NarrativeSeverityWarn    = "warn"
	NarrativeSeverityDanger  = "danger"
	NarrativeSeverityMute    = "mute"
)

// ProjectEventNarrative 是一个事件类型的用户可读叙事投影。
type ProjectEventNarrative struct {
	Kind     string
	Title    string
	Severity string
	// Noise 标记默认不进需求时间线的低信息量事件(心跳、闸门成功检查、纯信号
	// 转发、运维记账)。它们仍可进项目级事件流与排障日志——过滤是呈现层决定,
	// 不是丢弃事实。
	Noise bool
}

// projectEventNarratives 是全量映射。新增 ProjectEventType 而漏加条目会被
// TestProjectEventNarrativeCoversEveryEventType 拦下(该测试直接扫 types.go
// 源码取常量,不依赖手工维护的清单)。
var projectEventNarratives = map[ProjectEventType]ProjectEventNarrative{
	ProjectEventCreated:         {Kind: TimelineKindOther, Title: "项目创建", Severity: NarrativeSeverityInfo},
	ProjectEventConfigChanged:   {Kind: TimelineKindOther, Title: "项目配置变更", Severity: NarrativeSeverityInfo},
	ProjectEventArchived:        {Kind: TimelineKindOther, Title: "项目归档", Severity: NarrativeSeverityInfo},
	ProjectEventUnarchived:      {Kind: TimelineKindOther, Title: "项目取消归档", Severity: NarrativeSeverityInfo},
	ProjectEventDemandSubmitted: {Kind: TimelineKindDemandSubmitted, Title: "需求已提交", Severity: NarrativeSeverityInfo},

	ProjectEventRuntimePlacementUpdated:   {Kind: TimelineKindOther, Title: "运行落点更新", Severity: NarrativeSeverityInfo, Noise: true},
	ProjectEventRuntimePlacementReleased:  {Kind: TimelineKindOther, Title: "运行落点释放", Severity: NarrativeSeverityInfo, Noise: true},
	ProjectEventWorkspaceReady:            {Kind: TimelineKindOther, Title: "工作区就绪", Severity: NarrativeSeverityInfo, Noise: true},
	ProjectEventWorkspaceError:            {Kind: TimelineKindOther, Title: "工作区异常", Severity: NarrativeSeverityDanger},
	ProjectEventWorkspaceRecloneRequested: {Kind: TimelineKindOther, Title: "工作区重建请求", Severity: NarrativeSeverityInfo, Noise: true},
	ProjectEventWorkspaceMarkedReady:      {Kind: TimelineKindOther, Title: "工作区标记就绪", Severity: NarrativeSeverityInfo, Noise: true},

	ProjectEventCoordinationBlocked:              {Kind: TimelineKindCoordinationBlocked, Title: "协调受阻", Severity: NarrativeSeverityDanger},
	ProjectEventScenarioTemplateResolutionFailed: {Kind: TimelineKindCoordinationBlocked, Title: "剧本解析失败", Severity: NarrativeSeverityDanger},
	ProjectEventWorkflowCoordinationFailed:       {Kind: TimelineKindCoordinationBlocked, Title: "协调线程失败", Severity: NarrativeSeverityDanger},
	ProjectEventTaskDispatchBlocked:              {Kind: TimelineKindDispatchBlocked, Title: "派发受阻", Severity: NarrativeSeverityDanger},

	ProjectEventWorkflowSignaled:       {Kind: TimelineKindOther, Title: "协调信号", Severity: NarrativeSeverityMute, Noise: true},
	ProjectEventCoordinationJobCreated: {Kind: TimelineKindCoordinationStarted, Title: "协调开始", Severity: NarrativeSeverityInfo},
	ProjectEventRouteDecisionCreated:   {Kind: TimelineKindOther, Title: "已生成路由决策", Severity: NarrativeSeverityInfo},
	ProjectEventTaskCreated:            {Kind: TimelineKindTaskCreated, Title: "任务创建", Severity: NarrativeSeverityInfo},
	ProjectEventTaskGraphPlanned:       {Kind: TimelineKindPlanReady, Title: "计划已生成", Severity: NarrativeSeverityInfo},

	ProjectEventTaskDispatchGateChecked:        {Kind: TimelineKindOther, Title: "派发闸门检查", Severity: NarrativeSeverityMute, Noise: true},
	ProjectEventTaskDispatchGateBlocked:        {Kind: TimelineKindDispatchBlocked, Title: "派发闸门拦截", Severity: NarrativeSeverityWarn},
	ProjectEventTaskDispatchGateWaitingHuman:   {Kind: TimelineKindTaskWaitingHuman, Title: "等待人工处理", Severity: NarrativeSeverityWarn},
	ProjectEventTaskDispatchGateRetryLater:     {Kind: TimelineKindDispatchBlocked, Title: "派发稍后重试", Severity: NarrativeSeverityWarn},
	ProjectEventTaskDispatchGateReplanRequired: {Kind: TimelineKindDispatchBlocked, Title: "需重新规划", Severity: NarrativeSeverityWarn},

	ProjectEventTaskDispatched:        {Kind: TimelineKindTaskDispatched, Title: "任务开始", Severity: NarrativeSeverityInfo},
	ProjectEventTaskDispatchFailed:    {Kind: TimelineKindTaskFailed, Title: "任务派发失败", Severity: NarrativeSeverityDanger},
	ProjectEventTaskRetryScheduled:    {Kind: TimelineKindOther, Title: "已安排重试", Severity: NarrativeSeverityWarn},
	ProjectEventTaskAttemptLost:       {Kind: TimelineKindOther, Title: "执行尝试丢失", Severity: NarrativeSeverityWarn},
	ProjectEventTaskRecoveryRequested: {Kind: TimelineKindOther, Title: "请求恢复执行", Severity: NarrativeSeverityWarn},
	ProjectEventTaskContractMissing:   {Kind: TimelineKindOther, Title: "结果契约缺失", Severity: NarrativeSeverityWarn},
	ProjectEventTaskWaitingHuman:      {Kind: TimelineKindTaskWaitingHuman, Title: "等待人工处理", Severity: NarrativeSeverityWarn},
	ProjectEventTaskCancelled:         {Kind: TimelineKindTaskCancelled, Title: "任务取消", Severity: NarrativeSeverityMute},
	ProjectEventTaskDismissed:         {Kind: TimelineKindTaskCancelled, Title: "任务已撤除", Severity: NarrativeSeverityMute},
	ProjectEventTaskCompleted:         {Kind: TimelineKindTaskCompleted, Title: "任务完成", Severity: NarrativeSeveritySuccess},
	ProjectEventTaskFailed:            {Kind: TimelineKindTaskFailed, Title: "任务失败", Severity: NarrativeSeverityDanger},

	ProjectEventTaskResultSubmitted:        {Kind: TimelineKindResultRecorded, Title: "结果已提交", Severity: NarrativeSeverityInfo},
	ProjectEventTaskResultRecorded:         {Kind: TimelineKindResultRecorded, Title: "结果已记录", Severity: NarrativeSeverityInfo},
	ProjectEventTaskResultAccepted:         {Kind: TimelineKindResultAccepted, Title: "结果已采纳", Severity: NarrativeSeveritySuccess},
	ProjectEventTaskResultRejected:         {Kind: TimelineKindResultRejected, Title: "结果被驳回", Severity: NarrativeSeverityWarn},
	ProjectEventTaskResultBlocked:          {Kind: TimelineKindResultRejected, Title: "结果受阻", Severity: NarrativeSeverityWarn},
	ProjectEventTaskResultRetryableFailed:  {Kind: TimelineKindTaskFailed, Title: "结果失败（可重试）", Severity: NarrativeSeverityWarn},
	ProjectEventTaskResultValidationFailed: {Kind: TimelineKindResultRejected, Title: "结果校验未通过", Severity: NarrativeSeverityWarn},

	ProjectEventTaskRevisionRequested: {Kind: TimelineKindPlanChangeRequested, Title: "要求返工", Severity: NarrativeSeverityWarn},
	ProjectEventTaskRevisionCreated:   {Kind: TimelineKindTaskCreated, Title: "返工任务创建", Severity: NarrativeSeverityInfo},
	ProjectEventTaskReplanRequested:   {Kind: TimelineKindPlanChangeRequested, Title: "要求重新规划", Severity: NarrativeSeverityWarn},
	ProjectEventDemandSummaryCreated:  {Kind: TimelineKindResultRecorded, Title: "需求结论已生成", Severity: NarrativeSeveritySuccess},
	ProjectEventTransferRequested:     {Kind: TimelineKindOther, Title: "转派请求", Severity: NarrativeSeverityWarn},
	ProjectEventDecisionRequested:     {Kind: TimelineKindDecisionOpened, Title: "待人工决策", Severity: NarrativeSeverityWarn},
	ProjectEventDecisionSubmitted:     {Kind: TimelineKindDecisionResolved, Title: "决策已处理", Severity: NarrativeSeveritySuccess},

	ProjectEventEvidenceLinked:          {Kind: TimelineKindOther, Title: "证据已关联", Severity: NarrativeSeverityInfo},
	ProjectEventEvidenceVerified:        {Kind: TimelineKindOther, Title: "证据已核验", Severity: NarrativeSeveritySuccess},
	ProjectEventArtifactLinked:          {Kind: TimelineKindOther, Title: "工件已关联", Severity: NarrativeSeverityInfo},
	ProjectEventReportLinked:            {Kind: TimelineKindOther, Title: "报告已关联", Severity: NarrativeSeverityInfo},
	ProjectEventBudgetRecorded:          {Kind: TimelineKindOther, Title: "预算已记账", Severity: NarrativeSeverityInfo, Noise: true},
	ProjectEventAcceptanceSubmitted:     {Kind: TimelineKindOther, Title: "验收已提交", Severity: NarrativeSeverityInfo},
	ProjectEventArchiveSnapshotCreated:  {Kind: TimelineKindOther, Title: "归档快照已创建", Severity: NarrativeSeverityInfo, Noise: true},
	ProjectEventArchiveRetentionPending: {Kind: TimelineKindOther, Title: "归档留存待处理", Severity: NarrativeSeverityWarn},

	ProjectEventLendingEmployeeSkipped:  {Kind: TimelineKindStaffingGap, Title: "借调员工被跳过", Severity: NarrativeSeverityWarn},
	ProjectEventTeamlessEmployeeSkipped: {Kind: TimelineKindStaffingGap, Title: "无团队员工被跳过", Severity: NarrativeSeverityWarn},

	ProjectEventTaskUpstreamSupplementRejected:  {Kind: TimelineKindOther, Title: "上游补充被拒", Severity: NarrativeSeverityWarn},
	ProjectEventTaskUpstreamSupplementExhausted: {Kind: TimelineKindOther, Title: "上游补充已耗尽", Severity: NarrativeSeverityWarn},

	ProjectEventDemandReplanningReopened:  {Kind: TimelineKindPlanChangeRequested, Title: "需求重新规划", Severity: NarrativeSeverityWarn},
	ProjectEventDemandAcceptanceCompleted: {Kind: TimelineKindResultAccepted, Title: "验收通过", Severity: NarrativeSeveritySuccess},
	ProjectEventDemandAcceptanceRejected:  {Kind: TimelineKindResultRejected, Title: "验收未通过", Severity: NarrativeSeverityDanger},
	ProjectEventDemandCancelled:           {Kind: TimelineKindOther, Title: "需求已取消", Severity: NarrativeSeverityMute},

	// 语义扩编缺口发现（批三）：needed 时另有 decision.requested；本事件覆盖
	// 调用留痕与上限触顶（H9c），limit_reached 必须非 Noise 以便卷宗可见。
	ProjectEventCastingGapDiscovery: {Kind: TimelineKindStaffingGap, Title: "扩编缺口发现", Severity: NarrativeSeverityInfo},
}

// unknownProjectEventNarrative 是未登记事件类型的兜底。文案必须是通用中文:
// 兜底路径绝不能把英文蛇形原串暴露给用户(这正是旧 opsEventTitle 的缺陷)。
var unknownProjectEventNarrative = ProjectEventNarrative{
	Kind:     TimelineKindOther,
	Title:    "协调更新",
	Severity: NarrativeSeverityInfo,
}

// NarrateProjectEventType 返回一个事件类型的基础叙事;未登记类型回落通用中文。
func NarrateProjectEventType(eventType ProjectEventType) ProjectEventNarrative {
	if narrative, ok := projectEventNarratives[eventType]; ok {
		return narrative
	}
	return unknownProjectEventNarrative
}
