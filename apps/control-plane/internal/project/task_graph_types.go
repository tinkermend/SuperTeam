package project

import (
	"time"

	"github.com/google/uuid"
)

type ProjectTaskDependency struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	CoordinationJobID *uuid.UUID
	DependentTaskID   uuid.UUID
	BlockerTaskID     uuid.UUID
}

type ProjectTaskGraph struct {
	Nodes              []ProjectTaskGraphNode
	Edges              []ProjectTaskGraphEdge
	Employees          []ProjectTaskGraphEmployee
	Runs               []ProjectTaskGraphRun
	ExecutionSummaries []ExecutionSummary
	RecentEvents       []ProjectEvent
	DecisionRequests   []DecisionRequest
	StageSummaries     []ProjectTaskGraphStageSummary
	BlockingFacts      []ProjectTaskGraphBlockingFact
	HandoffAssessments []ProjectTaskGraphHandoffAssessment
	DispatchGates      []ProjectTaskGraphDispatchGate
}

// ProjectTaskGraphDispatchGate 是每个任务**当前**的派发闸门裁决（最新一条闸门结果）。
// 消费方判"是否仍被闸住"必须看这里，不要去数 project_events 的闸门事件——闸门事件
// 按 (任务, 事件类型) 至多发一次，任务二次卡人工不会有新事件，按事件推断会漏报。
type ProjectTaskGraphDispatchGate struct {
	ProjectTaskID     uuid.UUID
	Status            string
	CheckedAt         time.Time
	DecisionRequestID *uuid.UUID
}

// ProjectTaskGraphHandoffAssessmentStatus 是交接 verdict 的汇总口径
// (spec 2026-07-27 §5 P2-V)。unknown 是诚实边界：没有声明交付物数据
// (无已记录结果契约、或契约与 planner produces 均无声明)时不做启发式猜测。
const (
	ProjectTaskGraphHandoffStatusFulfilled   = "fulfilled"
	ProjectTaskGraphHandoffStatusPartial     = "partial"
	ProjectTaskGraphHandoffStatusUnfulfilled = "unfulfilled"
	ProjectTaskGraphHandoffStatusUnknown     = "unknown"

	ProjectTaskGraphHandoffDeliverableDelivered = "delivered"
	ProjectTaskGraphHandoffDeliverableMissing   = "missing"

	ProjectTaskGraphHandoffNoteDelivered = "delivered"
	ProjectTaskGraphHandoffNoteMissing   = "missing"
)

// ProjectTaskGraphHandoffAssessment 是单个任务(交接边的 blocker 侧)的结构化
// 交接 verdict:按声明交付物逐条核对 delivered/missing,纯读投影不持久化。
type ProjectTaskGraphHandoffAssessment struct {
	ProjectTaskID uuid.UUID
	Status        string
	Deliverables  []ProjectTaskGraphHandoffDeliverable
	// Notes 是下游声明的结论槽位软条目（spec 2026-08-16 交接包 §4.4）：
	// 逐条 delivered/missing，不进 Status 汇总、不打回。
	Notes []ProjectTaskGraphHandoffNote
	// InputGaps 是本任务**带缺口完成**时自行申报的输入缺口（同 spec §4.4）：
	// 只投影不触发补链，与 Notes 同为软视图。
	InputGaps []ProjectTaskGraphHandoffInputGap
}

// ProjectTaskGraphHandoffNote 是一条下游声明结论槽位的核对结果。声明来源:
// 下游任务 handoff_contract.notes 的 name；delivered 判据与 deliverables 一致
// (result_contract.handoff_notes 里对应 name 的 value 非空)。
type ProjectTaskGraphHandoffNote struct {
	Name    string
	Verdict string
}

// ProjectTaskGraphHandoffInputGap 是一条完成态申报的输入缺口（软条目）。
type ProjectTaskGraphHandoffInputGap struct {
	Name   string
	Reason string
}

// ProjectTaskGraphHandoffDeliverable 是一条声明交付物的核对结果。声明来源:
// 最新任务结果契约的 deliverables(v2 声明管道,Ref 已回填=已物化工件)与
// planner 声明的 produces 名单;delivered 判据与平台 produces 核对一致
// (Ref 或 Value 非空),不引入额外启发式。
type ProjectTaskGraphHandoffDeliverable struct {
	Name    string
	Kind    string
	Verdict string
	Ref     string
	Summary string
}

type ProjectTaskGraphNode struct {
	Task           ProjectTask
	StatusReason   string
	UpdatedAt      *time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CurrentBlocker *WorkflowInstanceCurrentBlocker
}

type ProjectTaskGraphEdge struct {
	DependentTaskID   uuid.UUID
	BlockerTaskID     uuid.UUID
	CoordinationJobID *uuid.UUID
	EdgeStatus        string
}

type ProjectTaskGraphEmployee struct {
	DigitalEmployeeID uuid.UUID
	DisplayName       string
	ProjectRole       ProjectRole
	EmployeeRole      string
	AvatarAsset       *ProjectTaskGraphEmployeeAvatarAsset
	Status            string
}

type ProjectTaskGraphEmployeeAvatarAsset struct {
	ID           string
	Label        string
	ImageURL     string
	ThumbnailURL string
}

type ProjectTaskGraphRun struct {
	ProjectTaskID        uuid.UUID
	DigitalEmployeeRunID *uuid.UUID
	RuntimeTaskID        *uuid.UUID
	RuntimeNodeID        *uuid.UUID
	RuntimeNodeSummary   string
	Status               string
	ProviderType         string
	StartedAt            *time.Time
	// FinishedAt is COALESCE(finished_at, completed_at): finished_at covers
	// failed/cancelled/timed-out terminals, completed_at the legacy success path.
	FinishedAt *time.Time
	// ErrorMessage is the run's terminal error, redacted through the shared
	// prose redaction rules (see redactProse) before leaving the read model —
	// raw stderr may embed credentials.
	ErrorMessage string
}

type ProjectTaskGraphStageSummary struct {
	StageIndex        int32
	Title             string
	TotalNodes        int32
	CompletedNodes    int32
	RunningNodes      int32
	WaitingHumanNodes int32
	BlockedNodes      int32
}

type ProjectTaskGraphBlockingFact struct {
	ReasonCode        string
	Message           string
	ResourceType      string
	ResourceID        string
	RecommendedAction string
	CreatedAt         time.Time
	// Gap is the structured staffing gap (RejectDemandPlanning's PlanningGap,
	// projectcoordination package) carried by a coordination.blocked event's "gap"
	// payload key. Nil for any non-structural diagnosis or event predating the
	// planning_gap decision flow — see projectTaskGraphBlockingFactFromEvent.
	Gap *ProjectTaskGraphBlockingFactGap
	// DecisionRequestID is the pending planning_gap DecisionRequest's id, carried by
	// the same blocked event's "decision_request_id" payload key (RejectDemandPlanning).
	// Empty when the event predates that field or no approval sink was wired.
	DecisionRequestID string
}

// ProjectTaskGraphBlockingFactGap mirrors projectcoordination.PlanningGap's shape
// for the web: the structural constraint a demand's route could not satisfy, plus
// the pool state and resolution options a human sees to act on it.
type ProjectTaskGraphBlockingFactGap struct {
	ConstraintKind       string
	Roles                []string
	RequiredCapabilities []string
	ActiveExecutorCount  int
	Options              []string
}
