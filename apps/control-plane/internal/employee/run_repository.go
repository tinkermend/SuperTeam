package employee

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type DigitalEmployeeRunRepository interface {
	WithTransaction(ctx context.Context, fn func(DigitalEmployeeRunRepository) error) error
	GetRunPreflight(ctx context.Context, tenantID, employeeID uuid.UUID) (RunPreflight, error)
	GetActiveRun(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRunByID(ctx context.Context, tenantID, runID uuid.UUID) (*DigitalEmployeeRun, error)
	GetRunByCommandID(ctx context.Context, tenantID uuid.UUID, commandID string) (*DigitalEmployeeRun, error)
	ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error)
	ListRunCalendar(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time, limit int32) (*DigitalEmployeeRunCalendarResult, error)
	ListRunEvents(ctx context.Context, tenantID, taskID, runID uuid.UUID, limit, offset int32) ([]RuntimeCommandEventWriteback, error)
	GetLatestDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (EmployeeConfigInput, error)
	GetDigitalEmployeeRunStats(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeRunStats, error)
	CreateRun(ctx context.Context, req CreateRunRecordRequest) (*DigitalEmployeeRun, error)
	UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) (*DigitalEmployeeRun, error)
	HasRunEventSequence(ctx context.Context, tenantID, taskID, runID uuid.UUID, sequenceNumber int32) (bool, error)
	CreateTaskEventIfAbsent(ctx context.Context, req CreateRunEventRecordRequest) (bool, error)
	UpsertProviderSession(ctx context.Context, req UpsertProviderSessionRequest) (uuid.UUID, error)
	CreateProviderSessionEventIfAbsent(ctx context.Context, req CreateProviderSessionEventRecordRequest) (uuid.UUID, error)
	// FindProviderSessionForTaskRoot looks up the latest recoverable
	// provider session (active, idle, or completed) scoped to (employee,
	// task lineage root). It returns an empty string when no eligible
	// session matches — this is a lookup, not an error path.
	FindProviderSessionForTaskRoot(ctx context.Context, tenantID, employeeID, taskRootID uuid.UUID) (string, error)
	// FindProviderSessionCandidateForTaskRoot 同上，但附带 resume 预检所需的
	// 事实（会话绑定的节点、最后一次被 runtime 看到的时间）。SessionID 为空
	// 表示无可续会话。
	FindProviderSessionCandidateForTaskRoot(ctx context.Context, tenantID, employeeID, taskRootID uuid.UUID) (ProviderSessionResumeCandidate, error)
	// GetRunTaskMetadata returns the metadata map persisted on the task
	// backing a run (tasks.params["metadata"]), so writeback can recover
	// dispatch-time context — e.g. revision_root_task_id — that isn't
	// otherwise echoed back on the runtime event. Returns an empty map,
	// not an error, when the task carries no metadata.
	GetRunTaskMetadata(ctx context.Context, tenantID, taskID uuid.UUID) (map[string]any, error)
	CreateCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
	GetCommandReceipt(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error)
	// Capability projection visibility (P3): skill display names + attestation conflicts for a run.
	ListSkillNamesByIDs(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) (map[uuid.UUID]string, error)
	ListAttestationMetadataByRunID(ctx context.Context, tenantID, runID uuid.UUID) ([][]byte, error)
	GetCommandReceiptForUpdate(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error)
	UpdateCommandReceipt(ctx context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error)
	ListCommandReceiptsByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID, commandType string, limit int32) ([]RuntimeCommandReceipt, error)
	UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error)
	DeleteDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) error
}

// EmployeePermissionPolicyReader 读取员工行上的 permission_policy(经权限中心审批的 A2 写回后
// 仍是运行时真源),供派发时按 allowed_actions 收敛可执行动作(P-enforce)。这是可选能力:未实现
// 该接口的仓库(如部分单测 fake)在派发处静默跳过收敛,不影响既有行为。
type EmployeePermissionPolicyReader interface {
	GetDigitalEmployeePermissionPolicy(ctx context.Context, tenantID, employeeID uuid.UUID) (map[string]any, error)
}

// ConfigRevisionCurrentReader 读取「已生效(active)」配置修订,供派发解析生效配置。这是治理 gate 的
// 承重读取:草案(draft)治理修订未批不得进入派发,否则审批 gate 形同虚设(员工配置页 spec §6.2/A)。
// 可选能力:未实现的仓库(部分单测 fake)回退到最新修订读取,保持既有行为。
type ConfigRevisionCurrentReader interface {
	GetCurrentDigitalEmployeeConfigRevision(ctx context.Context, tenantID, employeeID uuid.UUID) (EmployeeConfigInput, error)
}

// ProviderSessionResumeCandidate 是一条可续会话的预检事实。
type ProviderSessionResumeCandidate struct {
	SessionID         string
	RuntimeNodeID     uuid.UUID
	LastRuntimeSeenAt time.Time
	LastActiveAt      time.Time
}

type ProjectTaskRunPreflightRepository interface {
	// GetProjectTaskRunPreflight is discovery-only: it reports facts for an
	// employee against a deterministic representative of the project's
	// eligibility set. It does not pin a dispatch to a node.
	GetProjectTaskRunPreflight(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (StartProjectTaskRunPreflight, error)
	// GetProjectTaskRunPreflightForNode is the dispatch-path preflight: it takes
	// a node already chosen by ProjectTaskNodeResolver and confirms it is still
	// dispatchable.
	GetProjectTaskRunPreflightForNode(ctx context.Context, tenantID, employeeID, resolvedNodeID uuid.UUID) (StartProjectTaskRunPreflight, error)
	// ResolveProjectTaskLineageRoot resolves the session-lineage root task id
	// for projectTaskID, in this order:
	//   1. planner_metadata["revision_root_task_id"] if set
	//   2. revision_of_task_id (one hop)
	//   3. 接续链：任务所属 demand 有 continues_demand_id 时，沿祖先链**逐代**
	//      上溯，取**同一个 digitalEmployeeID** 的最近任务的根
	//      (spec 2026-08-01-demand-continuation-design §5.1/§5.2)
	//   4. the task's own id
	// 1/2 mirror projectcoordination's revisionRootTaskID without importing
	// that package. Provider session identity is scoped to this root, not to
	// the current project_task_id —— 3 是"接续时回到自己上次那条会话"的全部
	// 机制：键里带员工，所以多员工单里各员工各续各的，换人则落回 4 开新会话。
	ResolveProjectTaskLineageRoot(ctx context.Context, tenantID, digitalEmployeeID, projectTaskID uuid.UUID) (uuid.UUID, error)
}

// ResolveProjectTaskNodeRequest carries the identifiers the runtime node
// resolver needs. It intentionally has no dependency on the project package —
// see ProjectTaskNodeResolver.
type ResolveProjectTaskNodeRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DigitalEmployeeID uuid.UUID
	// ProjectTaskID selects the task hard-pin layer; Nil skips it (no task
	// context yet).
	ProjectTaskID uuid.UUID
	// DryRun resolves a node without persisting employee node affinity. Chat
	// runs use a project purely as a dispatch anchor (§13 design revision) and
	// must never steer future task placement for the employee, so chat
	// dispatch sets this true; project task dispatch leaves it false so
	// resolution keeps updating the sticky affinity as before.
	DryRun bool
}

// ProjectTaskNodeResolver picks the runtime node a project task dispatch should
// run on (project package's three-layer resolver: task pin > employee affinity
// > lowest load, all within the project's online eligibility set). It is
// declared here, not in the project package, so this package never imports
// project (project already imports employee); the concrete implementation is a
// project-package adapter wired in at composition (internal/app).
//
// On failure to resolve a runnable node, the returned error wraps a
// project-package sentinel (project.ErrProjectTaskNoEligibleOnlineNode or
// project.ErrProjectTaskPinnedNodeOffline) that callers outside this package
// (which may import project) can classify with errors.Is; this package never
// needs to know which sentinel it is.
type ProjectTaskNodeResolver interface {
	ResolveProjectTaskNode(ctx context.Context, req ResolveProjectTaskNodeRequest) (uuid.UUID, error)
}

// ProjectDispatchFacts carries project fields needed at dispatch time for
// stable project-directory CWD (name) and readiness messaging.
type ProjectDispatchFacts struct {
	Name                 string
	WorkspaceReadyStatus string
	// WorkspaceOwnership is platform_managed | attached (spec 2026-08-12).
	WorkspaceOwnership string
}

// ProjectDispatchFactsReader loads project dispatch facts without importing
// the project package (wired via adapter in internal/app).
type ProjectDispatchFactsReader interface {
	GetProjectDispatchFacts(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectDispatchFacts, error)
}

// ChatAnchorProjectValidator confirms a project can serve as a chat run's
// runtime anchor (§13 design revision): it must exist, belong to the
// requesting tenant, and not be archived. Chat runs carry no business effect
// on the project — this is purely a dispatch-scoping check, mirroring why
// ProjectTaskNodeResolver is declared here rather than in project: it lets
// this package depend on project-shaped facts without importing project
// (which already imports employee). The concrete implementation is a
// project-package adapter wired in at composition (internal/app). Any
// not-found/archived/cross-tenant outcome must be returned as an error this
// package's handler layer maps to 400 (i.e. wrapping ErrInvalidInput).
type ChatAnchorProjectValidator interface {
	ValidateChatAnchorProject(ctx context.Context, tenantID, projectID uuid.UUID) error
}

// ChatParticipantValidator optionally extends ChatAnchorProjectValidator with
// the participation gate: a chat run may only be driven by a digital employee
// that is an active digital_employee member of the anchor project. The
// production adapter always implements it; fakes without it skip the check.
type ChatParticipantValidator interface {
	ValidateChatParticipant(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) error
}

// ChatAnchorProjectGitResolver optionally extends ChatAnchorProjectValidator
// (目录与能力投影修订 spec §4): the chat anchor gains filesystem semantics, so
// dispatch needs the anchor project's repo binding to seed a readonly worktree.
// Returns the same metadata shape project task dispatch puts under
// metadata["project_git"] (url/default_branch/git_credential_ref/scope), or
// nil when the project has no repo binding. Resolved by type assertion so
// existing validator fakes keep compiling.
type ChatAnchorProjectGitResolver interface {
	ChatAnchorProjectGit(ctx context.Context, tenantID, projectID uuid.UUID) (map[string]any, error)
}

type RunPreflight struct {
	TenantID              uuid.UUID
	TeamID                uuid.UUID
	DigitalEmployeeID     uuid.UUID
	DigitalEmployeeStatus DigitalEmployeeStatus
	ExecutionInstanceID   uuid.UUID
	ExecutionStatus       ExecutionInstanceStatus
	RuntimeNodeID         uuid.UUID
	NodeID                string
	ProviderType          string
	AgentHomeDir          string
	RuntimeSelector       map[string]any
	SessionPolicy         map[string]any
	WorkspacePolicy       map[string]any
	BudgetPolicy          map[string]any
	TodayTokenUsage       int32
	BusinessTimezone      string
	ProviderHealthy       bool
}

type CreateRunRecordRequest struct {
	IdempotencyKey         *string
	IdempotencyFingerprint *string
	TenantID               uuid.UUID
	DigitalEmployeeID      uuid.UUID
	TeamID                 uuid.UUID
	Title                  string
	Description            *string
	Priority               int32
	ProviderType           string
	CreatorID              *uuid.UUID
	TargetNodeID           string
	WorkspacePath          *string
	Params                 map[string]any
	RiskLevel              *string
	NodeID                 string
	RuntimeNodeID          uuid.UUID
	ProviderSessionID      *string
	RunStatus              DigitalEmployeeRunStatus
	CommandID              string
	ExecutionInstanceID    uuid.UUID
	TimeoutSec             *int32
	GraceSec               *int32
	// RunKind and ResumeOfRunID mirror CreateDigitalEmployeeRunRequest's fields
	// of the same name onto the persisted task row.
	RunKind       string
	ResumeOfRunID *uuid.UUID
	// ChatThreadID persists the chat thread root on the task row for follow-up
	// turns; nil for task runs and for a conversation's root turn (whose
	// effective thread id is its own run id, resolved at read time).
	ChatThreadID *uuid.UUID
	// ProjectID is the run's project affiliation (运行必须归属项目 spec 2026-07-26):
	// persisted to the first-class task_runs.project_id column by every dispatch
	// path. Required — the column is NOT NULL.
	ProjectID uuid.UUID
}

type UpdateRunStatusRequest struct {
	TenantID                  uuid.UUID
	RunID                     uuid.UUID
	Status                    DigitalEmployeeRunStatus
	Result                    map[string]any
	ErrorMessage              *string
	Diagnostic                map[string]any
	LogRef                    *string
	RawResultRef              *string
	WorkProducts              []WorkProduct
	SessionState              map[string]any
	ErrorCode                 *string
	ErrorFamily               *string
	ExitCode                  *int32
	Signal                    *string
	ProviderSessionExternalID *string
	TimedOut                  bool
}

type CreateRunEventRecordRequest struct {
	TenantID       uuid.UUID
	TaskID         uuid.UUID
	RunID          uuid.UUID
	EventType      string
	SequenceNumber int32
	Payload        map[string]any
	CommandID      *string
	RawEventRef    *string
	LogRef         *string
	Metadata       map[string]any
}

type UpsertProviderSessionRequest struct {
	TenantID            uuid.UUID
	ProviderSessionID   string
	DigitalEmployeeID   uuid.UUID
	ExecutionInstanceID uuid.UUID
	RuntimeNodeID       uuid.UUID
	ProviderType        string
	Status              string
	Recoverable         bool
	SessionDisplayID    *string
	SessionParams       map[string]any
	SessionState        map[string]any
	LastSequenceNumber  int32
	LastCommandID       *string
	LastRunID           *uuid.UUID
	LastErrorFamily     *string
	Metadata            map[string]any
	ProjectTaskRootID   *uuid.UUID
}

type CreateProviderSessionEventRecordRequest struct {
	TenantID            uuid.UUID
	ProviderSessionUUID uuid.UUID
	EventType           string
	SequenceNumber      int32
	Payload             map[string]any
	RequestID           *string
	CommandID           *string
	RawEventRef         *string
	LogRef              *string
	SessionStatePatch   map[string]any
	Metadata            map[string]any
}

type CreateRuntimeCommandReceiptRequest struct {
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceType  string
	ResourceID    uuid.UUID
	Status        string
	Payload       map[string]any
	DispatchedAt  *time.Time
}

type UpdateRuntimeCommandReceiptRequest struct {
	TenantID     uuid.UUID
	CommandID    string
	Status       string
	Result       map[string]any
	ErrorMessage *string
}

type RuntimeCommandReceipt struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceType  string
	ResourceID    uuid.UUID
	Status        string
	Payload       map[string]any
	Result        map[string]any
	ErrorMessage  *string
	DispatchedAt  *time.Time
	CompletedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}
