package employee

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// RunKindTask and RunKindChat are the two supported values of a run's run_kind:
// task is the default (a single-shot dispatch), chat is a conversational run that
// can be followed up via ResumeOfRunID to resume the same provider session.
const (
	RunKindTask = "task"
	RunKindChat = "chat"
)

var (
	// ErrInvalidRunKind is returned when CreateDigitalEmployeeRunRequest.RunKind
	// is set to a value other than RunKindTask/RunKindChat.
	ErrInvalidRunKind = errors.New("invalid run_kind")
	// ErrInvalidResumeRun is returned when ResumeOfRunID is set but does not
	// reference a resumable chat run: wrong run_kind, wrong employee, not yet
	// terminal, or missing a provider session id.
	ErrInvalidResumeRun = errors.New("invalid resume_of_run_id")
)

type DigitalEmployeeRunStatus string

const (
	DigitalEmployeeRunStatusQueued      DigitalEmployeeRunStatus = "queued"
	DigitalEmployeeRunStatusDispatching DigitalEmployeeRunStatus = "dispatching"
	DigitalEmployeeRunStatusRunning     DigitalEmployeeRunStatus = "running"
	DigitalEmployeeRunStatusCancelling  DigitalEmployeeRunStatus = "cancelling"
	DigitalEmployeeRunStatusCompleted   DigitalEmployeeRunStatus = "completed"
	DigitalEmployeeRunStatusFailed      DigitalEmployeeRunStatus = "failed"
	DigitalEmployeeRunStatusCancelled   DigitalEmployeeRunStatus = "cancelled"
	DigitalEmployeeRunStatusTimedOut    DigitalEmployeeRunStatus = "timed_out"
)

func (s DigitalEmployeeRunStatus) IsTerminal() bool {
	switch s {
	case DigitalEmployeeRunStatusCompleted, DigitalEmployeeRunStatusFailed, DigitalEmployeeRunStatusCancelled, DigitalEmployeeRunStatusTimedOut:
		return true
	default:
		return false
	}
}

func (s DigitalEmployeeRunStatus) IsActive() bool {
	switch s {
	case DigitalEmployeeRunStatusQueued, DigitalEmployeeRunStatusDispatching, DigitalEmployeeRunStatusRunning, DigitalEmployeeRunStatusCancelling:
		return true
	default:
		return false
	}
}

type WorkProduct struct {
	Type      string         `json:"type"`
	Title     string         `json:"title"`
	Summary   string         `json:"summary,omitempty"`
	Ref       string         `json:"ref,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at,omitempty"`
}

type DigitalEmployeeRun struct {
	ID                        uuid.UUID
	TenantID                  uuid.UUID
	TaskID                    uuid.UUID
	DigitalEmployeeID         uuid.UUID
	ExecutionInstanceID       uuid.UUID
	RuntimeNodeID             uuid.UUID
	NodeID                    string
	CommandID                 string
	ProviderType              string
	ProviderSessionID         *string
	ProviderSessionExternalID *string
	RunKind                   string
	ResumeOfRunID             *uuid.UUID
	// ChatThreadID is the effective chat conversation id: non-nil for every
	// chat run (the stored thread root, or the run's own id for a root turn),
	// nil for task runs.
	ChatThreadID           *uuid.UUID
	Status                 DigitalEmployeeRunStatus
	Result                 map[string]any
	Diagnostic             map[string]any
	LogRef                 *string
	RawResultRef           *string
	WorkProducts           []WorkProduct
	SessionState           map[string]any
	ErrorMessage           *string
	ErrorCode              *string
	ErrorFamily            *string
	ExitCode               *int32
	Signal                 *string
	TimedOut               bool
	FailureAcknowledgedAt  *time.Time
	IdempotencyKey         *string
	IdempotencyFingerprint *string
	TimeoutSec             *int32
	GraceSec               *int32
	StartedAt              time.Time
	CompletedAt            *time.Time
	FinishedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
	// ProjectID / ProjectName: project_tasks link, else chat/task-hub metadata anchors.
	// Soft-deleted projects still resolve; ProjectDeleted is true when deleted_at is set.
	ProjectID      *uuid.UUID
	ProjectName    *string
	ProjectDeleted bool
}

type DigitalEmployeeRunStats struct {
	TotalCount     int64
	SucceededCount int64
	FailedCount    int64
	CancelledCount int64
	Last7dCount    int64
	Prev7dCount    int64
	AvgDurationSec *float64
	P90DurationSec *float64
}

// DigitalEmployeeRunCalendarItem is the slim run projection for calendar boards.
// It intentionally omits result/diagnostic/session_state/work_products payloads.
type DigitalEmployeeRunCalendarItem struct {
	ID          uuid.UUID
	TaskTitle   string
	Status      DigitalEmployeeRunStatus
	RunKind     string
	CreatedAt   time.Time
	ProjectID   *uuid.UUID
	ProjectName *string
	// ProjectDeleted is true when the linked project has been soft-deleted.
	ProjectDeleted bool
}

// DigitalEmployeeRunCalendarResult is the calendar-window response: total matching
// runs in [From, To), whether the item list was truncated to the server cap, and
// the newest-first slim items actually returned.
type DigitalEmployeeRunCalendarResult struct {
	From       time.Time
	To         time.Time
	TotalCount int64
	Truncated  bool
	Items      []DigitalEmployeeRunCalendarItem
}

// DigitalEmployeeRunListFilter captures the filterable, pagination, and time-window
// parameters for the digital employee run history list endpoint. Statuses is a slice
// of run-status strings; an empty slice (or nil) means "no status filter". RunKind,
// when non-nil, must be RunKindTask or RunKindChat and scopes the list to that kind
// only; nil means "no run_kind filter".
type DigitalEmployeeRunListFilter struct {
	Statuses  []string
	ProjectID *uuid.UUID
	From      *time.Time
	To        *time.Time
	RunKind   *string
	// ChatThreadID, when non-nil, narrows the list to the chat runs of one
	// conversation (thread root run id); matches both the root turn and its
	// follow-ups, chat runs only.
	ChatThreadID *uuid.UUID
	Limit        int32
	Offset       int32
}

// DigitalEmployeeRunListItem augments a run with the joined task title, optional
// project association, work-product count, and finished-run duration. ProjectID and
// ProjectName are non-nil when linked via project_tasks or chat/task metadata
// (anchor_project_id / project_id). DurationSec is non-nil only for runs that have
// reached a terminal finished_at.
type DigitalEmployeeRunListItem struct {
	Run              *DigitalEmployeeRun
	TaskTitle        string
	ProjectID        *uuid.UUID
	ProjectName      *string
	ProjectDeleted   bool
	WorkProductCount int32
	DurationSec      *float64
}

// RunProjectOption is a distinct project option surfaced in the run list response
// filters, scoped to projects that currently have at least one run for the employee.
type RunProjectOption struct {
	ID      uuid.UUID
	Name    string
	Deleted bool
}

// DigitalEmployeeRunListResult is the aggregated payload returned by the run history
// repository: the paginated items, the total count of runs matching the SAME filters
// (ignoring pagination), and the project options for the filter dropdown.
type DigitalEmployeeRunListResult struct {
	Items      []DigitalEmployeeRunListItem
	TotalCount int64
	Projects   []RunProjectOption
}

type CreateDigitalEmployeeRunRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	Objective         string
	Prompt            string
	ContextRefs       []map[string]any
	ArtifactRefs      []map[string]any
	OutputSchema      map[string]any
	AllowedActions    []string
	ForbiddenActions  []string
	SecretRefs        []string
	IdempotencyKey    *string
	TimeoutSec        *int32
	GraceSec          *int32
	Metadata          map[string]any
	// RunKind selects between a single-shot task run (RunKindTask, the default
	// when empty) and a conversational chat run (RunKindChat). Only chat runs
	// may set ResumeOfRunID.
	RunKind string
	// ResumeOfRunID, when set, must reference a prior terminal chat run for the
	// same digital employee that carries a provider session id; CreateRun
	// injects that session id into Metadata so the runtime resumes the same
	// provider session instead of starting a fresh one.
	ResumeOfRunID *uuid.UUID
	// ProjectID is the chat run's runtime anchor (§13 design revision): a chat
	// run carries no project business effect, but resolving its dispatch node,
	// budget, and policy boundary requires a project context the way project
	// task dispatch does. Required when RunKind is RunKindChat; CreateRun
	// clears it to nil for RunKindTask (ignored, not validated).
	// 它同时是 MCP 投影的项目维度（目录与能力投影修订 spec §3.2）：非 nil 时
	// 派发依赖解析会把项目级 MCP 绑定并入员工侧集合（同 server_key 项目侧优
	// 先）。StartProjectTaskRun 绕过 CreateRun 组装请求时会据此回填项目 ID。
	ProjectID *uuid.UUID
	// chatThreadID is resolved by CreateRun itself (inherited from the resumed
	// run's effective thread id); caller-provided values are discarded. Kept
	// unexported so the handler layer cannot populate it.
	chatThreadID *uuid.UUID
}

type StartProjectTaskRunPreflight struct {
	TenantID              uuid.UUID
	TeamID                uuid.UUID
	DigitalEmployeeID     uuid.UUID
	DigitalEmployeeStatus DigitalEmployeeStatus
	RuntimeNodeID         uuid.UUID
	NodeID                string
	ProviderType          string
	WorkspaceBaseDir      string
	BudgetPolicy          map[string]any
	TodayTokenUsage       int32
	BusinessTimezone      string
	RuntimeSessionActive  bool
	ProviderHealthy       bool
}

type StartProjectTaskRunRequest struct {
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	DemandID             uuid.UUID
	ProjectTaskID        uuid.UUID
	ProjectTaskAttemptID uuid.UUID
	DigitalEmployeeID    uuid.UUID
	DispatchUserID       uuid.UUID
	Objective            string
	Prompt               string
	IdempotencyKey       string
	Metadata             map[string]any
	WorkspaceMode        string
	BaseRef              string
	ProjectGit           map[string]any
	TimeoutSec           *int32
	GraceSec             *int32
}

type StartProjectTaskRunResult struct {
	RunID         uuid.UUID
	RuntimeTaskID uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
	ProviderType  string
	// SessionResume 是派发期会话接续结论（spec 2026-08-07）；调用方据此写卷宗事件。
	SessionResume SessionResumeOutcome
}

type StopDigitalEmployeeRunRequest struct {
	TenantID          uuid.UUID
	UserID            uuid.UUID
	DigitalEmployeeID uuid.UUID
	RunID             uuid.UUID
	Reason            string
}

type RuntimeCommandWritebackIdentity struct {
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
}

type RuntimeEventRecordRequest struct {
	TenantID        uuid.UUID
	RuntimeNodeID   uuid.UUID
	NodeID          string
	EventType       string
	Severity        string
	Source          string
	Title           string
	Description     string
	ProviderType    string
	CorrelationType string
	CorrelationID   string
	Payload         map[string]any
}

type RuntimeEventRecorder interface {
	RecordRuntimeEvent(ctx context.Context, req RuntimeEventRecordRequest) error
}

type RuntimeCommandEventWriteback struct {
	EventType                 string         `json:"event_type"`
	SequenceNumber            int32          `json:"sequence_number"`
	Payload                   map[string]any `json:"payload"`
	ProviderSessionExternalID *string        `json:"provider_session_external_id,omitempty"`
	SessionStatePatch         map[string]any `json:"session_state_patch,omitempty"`
	LogRef                    *string        `json:"log_ref,omitempty"`
	RawEventRef               *string        `json:"raw_event_ref,omitempty"`
	Metadata                  map[string]any `json:"metadata,omitempty"`
}

type RuntimeCommandTerminalWriteback struct {
	Status                    DigitalEmployeeRunStatus `json:"status"`
	Summary                   string                   `json:"summary,omitempty"`
	Result                    map[string]any           `json:"result,omitempty"`
	Diagnostic                map[string]any           `json:"diagnostic,omitempty"`
	WorkProducts              []WorkProduct            `json:"work_products,omitempty"`
	ProviderSessionExternalID *string                  `json:"provider_session_external_id,omitempty"`
	SessionStatePatch         map[string]any           `json:"session_state_patch,omitempty"`
	LogRef                    *string                  `json:"log_ref,omitempty"`
	RawResultRef              *string                  `json:"raw_result_ref,omitempty"`
	ErrorMessage              *string                  `json:"error_message,omitempty"`
	ErrorCode                 *string                  `json:"error_code,omitempty"`
	ErrorFamily               *string                  `json:"error_family,omitempty"`
	ExitCode                  *int32                   `json:"exit_code,omitempty"`
	Signal                    *string                  `json:"signal,omitempty"`
	TimedOut                  bool                     `json:"timed_out,omitempty"`
}

// TeamConstitutionForDispatch 派发时注入的团队宪法：已渲染好的约束文本与其版本号。
// 版本号随执行留痕，供"这条任务当时受哪一版宪法约束"回溯。
type TeamConstitutionForDispatch struct {
	Prompt         string
	RevisionNumber int32
}
