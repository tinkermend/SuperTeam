package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/tenant"
)

type PgRunRepository struct {
	q  *queries.Queries
	db runTransactionBeginner
}

type runTransactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func NewPgRunRepository(q *queries.Queries, db ...runTransactionBeginner) DigitalEmployeeRunRepository {
	var beginner runTransactionBeginner
	if len(db) > 0 {
		beginner = db[0]
	}
	return &PgRunRepository{q: q, db: beginner}
}

func (r *PgRunRepository) WithTransaction(ctx context.Context, fn func(DigitalEmployeeRunRepository) error) error {
	if r.db == nil {
		return fn(r)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin run transaction: %w", err)
	}
	// Roll back unconditionally on any early return or panic; it is a no-op
	// once Commit succeeds, so a leaked open tx can never strand a pooled conn.
	defer func() { _ = tx.Rollback(ctx) }()
	txRepo := &PgRunRepository{q: r.q.WithTx(tx)}
	if err := fn(txRepo); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit run transaction: %w", err)
	}
	return nil
}

func (r *PgRunRepository) GetRunPreflight(ctx context.Context, tenantID, employeeID uuid.UUID) (RunPreflight, error) {
	preflight, err := r.q.GetDigitalEmployeeRunPreflight(ctx, queries.GetDigitalEmployeeRunPreflightParams{
		DigitalEmployeeID: employeeID,
		TenantID:          tenantID,
	})
	if err != nil {
		return RunPreflight{}, mapNoRows(err)
	}

	return runPreflightFromQuery(preflight)
}

// GetDigitalEmployeePermissionPolicy 取员工行 permission_policy(P-enforce 的 allowed_actions
// 上限来源)。复用现有 GetDigitalEmployee(SELECT *),无需新增查询/重生成。
func (r *PgRunRepository) GetDigitalEmployeePermissionPolicy(ctx context.Context, tenantID, employeeID uuid.UUID) (map[string]any, error) {
	employee, err := r.q.GetDigitalEmployee(ctx, queries.GetDigitalEmployeeParams{
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return mapFromJSONB(employee.PermissionPolicy, "permission_policy")
}

func (r *PgRunRepository) GetLatestDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (EmployeeConfigInput, error) {
	revision, err := r.q.GetLatestDigitalEmployeeConfigRevision(ctx, queries.GetLatestDigitalEmployeeConfigRevisionParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return EmployeeConfigInput{}, mapNoRows(err)
	}

	return employeeConfigInputFromQuery(digitalEmployeeConfigRevisionQueryAdapter{
		id:                    revision.ID,
		tenantID:              revision.TenantID,
		digitalEmployeeID:     revision.DigitalEmployeeID,
		revisionNumber:        revision.RevisionNumber,
		personaMemoryMarkdown: revision.PersonaMemoryMarkdown,
		capabilityBindings:    revision.CapabilityBindings,
		budgetPolicy:          revision.BudgetPolicy,
	})
}

// GetCurrentDigitalEmployeeConfigRevision 读取已生效(status=active、未归档)的最新修订,是派发解析
// 生效配置的承重读取(治理 gate)。查询 GetCurrentDigitalEmployeeConfigRevision 已存在,无需新增。
func (r *PgRunRepository) GetCurrentDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (EmployeeConfigInput, error) {
	revision, err := r.q.GetCurrentDigitalEmployeeConfigRevision(ctx, queries.GetCurrentDigitalEmployeeConfigRevisionParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return EmployeeConfigInput{}, mapNoRows(err)
	}

	return employeeConfigInputFromQuery(digitalEmployeeConfigRevisionQueryAdapter{
		id:                    revision.ID,
		tenantID:              revision.TenantID,
		digitalEmployeeID:     revision.DigitalEmployeeID,
		revisionNumber:        revision.RevisionNumber,
		personaMemoryMarkdown: revision.PersonaMemoryMarkdown,
		capabilityBindings:    revision.CapabilityBindings,
		budgetPolicy:          revision.BudgetPolicy,
	})
}

func (r *PgRunRepository) GetProjectTaskRunPreflight(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (StartProjectTaskRunPreflight, error) {
	preflight, err := r.q.GetProjectTaskRunPreflight(ctx, queries.GetProjectTaskRunPreflightParams{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return StartProjectTaskRunPreflight{}, mapNoRows(err)
	}
	return projectTaskRunPreflightFromQuery(preflight)
}

// GetProjectTaskRunPreflightForNode is the dispatch-path preflight: unlike
// GetProjectTaskRunPreflight (discovery-only, resolves an eligibility-set
// representative), it takes a node already chosen by the Go-level three-layer
// resolver and only re-confirms that node's health plus assembles the
// workspace/budget/session facts needed to start a run on it.
func (r *PgRunRepository) GetProjectTaskRunPreflightForNode(ctx context.Context, tenantID, employeeID, resolvedNodeID uuid.UUID) (StartProjectTaskRunPreflight, error) {
	preflight, err := r.q.GetProjectTaskRunPreflightForNode(ctx, queries.GetProjectTaskRunPreflightForNodeParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		ResolvedNodeID:    resolvedNodeID,
	})
	if err != nil {
		return StartProjectTaskRunPreflight{}, mapNoRows(err)
	}
	return projectTaskRunPreflightForNodeFromQuery(preflight)
}

// ResolveProjectTaskLineageRoot mirrors projectcoordination's
// revisionRootTaskID (see project_store.go) without importing that package:
// planner_metadata["revision_root_task_id"] wins if set, else
// revision_of_task_id (one hop), else the task's own id.
func (r *PgRunRepository) ResolveProjectTaskLineageRoot(ctx context.Context, tenantID, projectTaskID uuid.UUID) (uuid.UUID, error) {
	row, err := r.q.GetProjectTaskSessionLineage(ctx, queries.GetProjectTaskSessionLineageParams{
		TenantID: tenantID,
		ID:       projectTaskID,
	})
	if err != nil {
		return uuid.Nil, mapNoRows(err)
	}
	plannerMetadata, err := mapFromJSONB(row.PlannerMetadata, "planner_metadata")
	if err != nil {
		return uuid.Nil, err
	}
	if value, ok := plannerMetadata["revision_root_task_id"].(string); ok {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			rootID, err := uuid.Parse(trimmed)
			if err != nil {
				return uuid.Nil, fmt.Errorf("parse planner_metadata revision_root_task_id: %w", err)
			}
			return rootID, nil
		}
	}
	if row.RevisionOfTaskID.Valid && row.RevisionOfTaskID.UUID != uuid.Nil {
		return row.RevisionOfTaskID.UUID, nil
	}
	return projectTaskID, nil
}

func runPreflightFromQuery(preflight queries.GetDigitalEmployeeRunPreflightRow) (RunPreflight, error) {
	runtimeSelector, err := mapFromJSONB(preflight.RuntimeSelector, "runtime_selector")
	if err != nil {
		return RunPreflight{}, err
	}
	sessionPolicy, err := mapFromJSONB(preflight.SessionPolicy, "session_policy")
	if err != nil {
		return RunPreflight{}, err
	}
	workspacePolicy, err := mapFromJSONB(preflight.WorkspacePolicy, "workspace_policy")
	if err != nil {
		return RunPreflight{}, err
	}
	budgetPolicy, err := mapFromJSONB(preflight.BudgetPolicy, "budget_policy")
	if err != nil {
		return RunPreflight{}, err
	}

	return RunPreflight{
		TenantID:              preflight.TenantID,
		TeamID:                preflight.TeamID.UUID,
		DigitalEmployeeID:     preflight.DigitalEmployeeID,
		DigitalEmployeeStatus: DigitalEmployeeStatus(preflight.DigitalEmployeeStatus),
		// dei retired: synthesize a deterministic compat UUID; no longer read from dei table.
		ExecutionInstanceID: standaloneCompatibilityExecutionInstanceID(preflight.TenantID, preflight.DigitalEmployeeID),
		ExecutionStatus:     ExecutionInstanceStatusReady,
		RuntimeNodeID:       preflight.RuntimeNodeID,
		NodeID:              preflight.NodeID,
		ProviderType:        preflight.ProviderType,
		AgentHomeDir:        preflight.AgentHomeDir,
		RuntimeSelector:     runtimeSelector,
		SessionPolicy:       sessionPolicy,
		WorkspacePolicy:     workspacePolicy,
		BudgetPolicy:        budgetPolicy,
		TodayTokenUsage:     preflight.TodayTokenUsage,
		BusinessTimezone:    preflight.BusinessTimezone,
		ProviderHealthy:     preflight.ProviderHealthy,
	}, nil
}

func projectTaskRunPreflightFromQuery(preflight queries.GetProjectTaskRunPreflightRow) (StartProjectTaskRunPreflight, error) {
	budgetPolicy, err := mapFromJSONB(preflight.BudgetPolicy, "budget_policy")
	if err != nil {
		return StartProjectTaskRunPreflight{}, err
	}
	return StartProjectTaskRunPreflight{
		TenantID:              preflight.TenantID,
		TeamID:                preflight.TeamID.UUID,
		DigitalEmployeeID:     preflight.DigitalEmployeeID,
		DigitalEmployeeStatus: DigitalEmployeeStatus(preflight.DigitalEmployeeStatus),
		RuntimeNodeID:         preflight.RuntimeNodeID,
		NodeID:                preflight.NodeID,
		ProviderType:          preflight.ProviderType,
		WorkspaceBaseDir:      preflight.WorkspaceBaseDir,
		BudgetPolicy:          budgetPolicy,
		TodayTokenUsage:       preflight.TodayTokenUsage,
		BusinessTimezone:      preflight.BusinessTimezone,
		RuntimeSessionActive:  preflight.RuntimeSessionActive,
		ProviderHealthy:       preflight.ProviderHealthy,
	}, nil
}

func projectTaskRunPreflightForNodeFromQuery(preflight queries.GetProjectTaskRunPreflightForNodeRow) (StartProjectTaskRunPreflight, error) {
	budgetPolicy, err := mapFromJSONB(preflight.BudgetPolicy, "budget_policy")
	if err != nil {
		return StartProjectTaskRunPreflight{}, err
	}
	return StartProjectTaskRunPreflight{
		TenantID:              preflight.TenantID,
		TeamID:                preflight.TeamID.UUID,
		DigitalEmployeeID:     preflight.DigitalEmployeeID,
		DigitalEmployeeStatus: DigitalEmployeeStatus(preflight.DigitalEmployeeStatus),
		RuntimeNodeID:         preflight.RuntimeNodeID,
		NodeID:                preflight.NodeID,
		ProviderType:          preflight.ProviderType,
		WorkspaceBaseDir:      preflight.WorkspaceBaseDir,
		BudgetPolicy:          budgetPolicy,
		TodayTokenUsage:       preflight.TodayTokenUsage,
		BusinessTimezone:      preflight.BusinessTimezone,
		RuntimeSessionActive:  preflight.RuntimeSessionActive,
		ProviderHealthy:       preflight.ProviderHealthy,
	}, nil
}

func (r *PgRunRepository) GetActiveRun(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRun, error) {
	run, err := r.q.GetActiveDigitalEmployeeRun(ctx, queries.GetActiveDigitalEmployeeRunParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return digitalEmployeeRunFromQuery(run), nil
}

// ListStalePreConfirmationRuns 跨租户列出停留在 queued/dispatching 超过时限的
// run,供看门狗周期清扫(残债交接 §1 第 2 层)。
func (r *PgRunRepository) ListStalePreConfirmationRuns(ctx context.Context, staleBefore time.Time, limit int32) ([]*DigitalEmployeeRun, error) {
	rows, err := r.q.ListStalePreConfirmationDigitalEmployeeRuns(ctx, queries.ListStalePreConfirmationDigitalEmployeeRunsParams{
		StaleBefore: pgtype.Timestamptz{Time: staleBefore, Valid: true},
		BatchLimit:  limit,
	})
	if err != nil {
		return nil, err
	}
	runs := make([]*DigitalEmployeeRun, 0, len(rows))
	for _, row := range rows {
		runs = append(runs, digitalEmployeeRunFromQuery(row))
	}
	return runs, nil
}

func (r *PgRunRepository) GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	run, err := r.q.GetDigitalEmployeeRun(ctx, queries.GetDigitalEmployeeRunParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		RunID:             runID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	// GetDigitalEmployeeRunRow adds run_kind/resume_of_run_id (task-level columns) on top
	// of the task_runs columns; reassemble the task_runs subset into queries.TaskRun so the
	// existing mapper is reused, mirroring digitalEmployeeRunListItemFromDetailedRow below.
	// run_kind/resume_of_run_id have no equivalent on queries.TaskRun (they live on the
	// tasks table, not task_runs), so they are set on the mapped domain struct separately.
	mapped := digitalEmployeeRunFromQuery(queries.TaskRun{
		ID:                        run.ID,
		TenantID:                  run.TenantID,
		TaskID:                    run.TaskID,
		NodeID:                    run.NodeID,
		RuntimeNodeID:             run.RuntimeNodeID,
		ProviderSessionID:         run.ProviderSessionID,
		Status:                    run.Status,
		StartedAt:                 run.StartedAt,
		CompletedAt:               run.CompletedAt,
		FinishedAt:                run.FinishedAt,
		Result:                    run.Result,
		ErrorMessage:              run.ErrorMessage,
		CreatedAt:                 run.CreatedAt,
		UpdatedAt:                 run.UpdatedAt,
		CommandID:                 run.CommandID,
		DigitalEmployeeID:         run.DigitalEmployeeID,
		ExecutionInstanceID:       run.ExecutionInstanceID,
		IdempotencyKey:            run.IdempotencyKey,
		IdempotencyFingerprint:    run.IdempotencyFingerprint,
		TimeoutSec:                run.TimeoutSec,
		GraceSec:                  run.GraceSec,
		Diagnostic:                run.Diagnostic,
		LogRef:                    run.LogRef,
		RawResultRef:              run.RawResultRef,
		WorkProducts:              run.WorkProducts,
		SessionState:              run.SessionState,
		ErrorCode:                 run.ErrorCode,
		ErrorFamily:               run.ErrorFamily,
		ExitCode:                  run.ExitCode,
		Signal:                    run.Signal,
		TimedOut:                  run.TimedOut,
		ProviderType:              run.ProviderType,
		ProviderSessionExternalID: run.ProviderSessionExternalID,
		FailureAcknowledgedAt:     run.FailureAcknowledgedAt,
		FailureAcknowledgedBy:     run.FailureAcknowledgedBy,
	})
	mapped.RunKind = run.RunKind
	mapped.ResumeOfRunID = uuidPtrFromNull(run.ResumeOfRunID)
	mapped.ChatThreadID = effectiveChatThreadID(run.RunKind, run.ChatThreadID, run.ID)
	projectID := run.ProjectID
	mapped.ProjectID = &projectID
	if run.ProjectName.Valid {
		name := run.ProjectName.String
		mapped.ProjectName = &name
	}
	mapped.ProjectDeleted = run.ProjectDeleted
	return mapped, nil
}

// effectiveChatThreadID resolves a chat run's conversation id: the stored
// thread root when present, else the run's own id (a conversation's root turn
// stores NULL). Task runs have no thread.
func effectiveChatThreadID(runKind string, stored uuid.NullUUID, runID uuid.UUID) *uuid.UUID {
	if runKind != RunKindChat {
		return nil
	}
	id := runID
	if stored.Valid {
		id = stored.UUID
	}
	return &id
}

func (r *PgRunRepository) GetRunByID(ctx context.Context, tenantID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	run, err := r.q.GetTaskRun(ctx, queries.GetTaskRunParams{
		TenantID: uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
		ID:       runID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	mapped := digitalEmployeeRunFromQuery(run)
	if mapped.DigitalEmployeeID == uuid.Nil || mapped.CommandID == "" {
		return nil, ErrNotFound
	}
	return mapped, nil
}

func (r *PgRunRepository) GetRunByCommandID(ctx context.Context, tenantID uuid.UUID, commandID string) (*DigitalEmployeeRun, error) {
	run, err := r.q.GetDigitalEmployeeRunByCommandID(ctx, queries.GetDigitalEmployeeRunByCommandIDParams{
		TenantID:  tenantID,
		CommandID: commandID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return digitalEmployeeRunFromQuery(run), nil
}

func (r *PgRunRepository) ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error) {
	statuses := filter.Statuses
	if statuses == nil {
		statuses = []string{}
	}
	var projectID uuid.NullUUID
	if filter.ProjectID != nil {
		projectID = uuid.NullUUID{UUID: *filter.ProjectID, Valid: true}
	}
	var fromTime, toTime pgtype.Timestamptz
	if filter.From != nil {
		fromTime = pgtype.Timestamptz{Time: *filter.From, Valid: true}
	}
	if filter.To != nil {
		toTime = pgtype.Timestamptz{Time: *filter.To, Valid: true}
	}

	runKind := textFromPtr(filter.RunKind)
	chatThreadID := nullUUIDFromPtr(filter.ChatThreadID)

	rows, err := r.q.ListDigitalEmployeeRunsDetailed(ctx, queries.ListDigitalEmployeeRunsDetailedParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Statuses:          statuses,
		ProjectID:         projectID,
		FromTime:          fromTime,
		ToTime:            toTime,
		RunKind:           runKind,
		ChatThreadID:      chatThreadID,
		Limit:             filter.Limit,
		Offset:            filter.Offset,
	})
	if err != nil {
		return nil, err
	}

	total, err := r.q.CountDigitalEmployeeRunsDetailed(ctx, queries.CountDigitalEmployeeRunsDetailedParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		Statuses:          statuses,
		ProjectID:         projectID,
		FromTime:          fromTime,
		ToTime:            toTime,
		RunKind:           runKind,
		ChatThreadID:      chatThreadID,
	})
	if err != nil {
		return nil, err
	}

	projectRows, err := r.q.ListDigitalEmployeeRunProjectOptions(ctx, queries.ListDigitalEmployeeRunProjectOptionsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}

	items := make([]DigitalEmployeeRunListItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, digitalEmployeeRunListItemFromDetailedRow(row))
	}
	projects := make([]RunProjectOption, 0, len(projectRows))
	for _, p := range projectRows {
		projects = append(projects, RunProjectOption{ID: p.ID, Name: p.Name, Deleted: p.ProjectDeleted})
	}

	return &DigitalEmployeeRunListResult{Items: items, TotalCount: total, Projects: projects}, nil
}

func (r *PgRunRepository) ListRunCalendar(ctx context.Context, tenantID, employeeID uuid.UUID, from, to time.Time, limit int32) (*DigitalEmployeeRunCalendarResult, error) {
	fromTime := pgtype.Timestamptz{Time: from, Valid: true}
	toTime := pgtype.Timestamptz{Time: to, Valid: true}

	total, err := r.q.CountDigitalEmployeeRunCalendarItems(ctx, queries.CountDigitalEmployeeRunCalendarItemsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		FromTime:          fromTime,
		ToTime:            toTime,
	})
	if err != nil {
		return nil, err
	}

	rows, err := r.q.ListDigitalEmployeeRunCalendarItems(ctx, queries.ListDigitalEmployeeRunCalendarItemsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		FromTime:          fromTime,
		ToTime:            toTime,
		Limit:             limit,
	})
	if err != nil {
		return nil, err
	}

	items := make([]DigitalEmployeeRunCalendarItem, 0, len(rows))
	for _, row := range rows {
		item := DigitalEmployeeRunCalendarItem{
			ID:        row.ID,
			TaskTitle: row.TaskTitle,
			Status:    DigitalEmployeeRunStatus(row.Status),
			RunKind:   row.RunKind,
			CreatedAt: timeFromTimestamptz(row.CreatedAt),
		}
		pid := row.ProjectID
		item.ProjectID = &pid
		if row.ProjectName.Valid {
			name := row.ProjectName.String
			item.ProjectName = &name
		}
		item.ProjectDeleted = row.ProjectDeleted
		items = append(items, item)
	}

	return &DigitalEmployeeRunCalendarResult{
		From:       from,
		To:         to,
		TotalCount: total,
		Truncated:  total > int64(len(items)),
		Items:      items,
	}, nil
}

func (r *PgRunRepository) ListRunEvents(ctx context.Context, tenantID, taskID, runID uuid.UUID, limit, offset int32) ([]RuntimeCommandEventWriteback, error) {
	events, err := r.q.ListTaskEventsForRun(ctx, queries.ListTaskEventsForRunParams{
		TenantID: tenantID,
		TaskID:   taskID,
		RunID:    runID,
		Limit:    limit,
		Offset:   offset,
	})
	if err != nil {
		return nil, err
	}

	out := make([]RuntimeCommandEventWriteback, 0, len(events))
	for _, event := range events {
		out = append(out, runtimeCommandEventFromTaskEvent(event))
	}
	return out, nil
}

func (r *PgRunRepository) GetDigitalEmployeeRunStats(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeRunStats, error) {
	return (&PgRepository{q: r.q}).GetDigitalEmployeeRunStats(ctx, tenantID, digitalEmployeeID)
}

func (r *PgRunRepository) CreateRun(ctx context.Context, req CreateRunRecordRequest) (*DigitalEmployeeRun, error) {
	params, err := jsonBytesFromMap(req.Params, "params")
	if err != nil {
		return nil, err
	}

	row, err := r.q.CreateDigitalEmployeeTaskRun(ctx, queries.CreateDigitalEmployeeTaskRunParams{
		IdempotencyKey:         textFromPtr(req.IdempotencyKey),
		IdempotencyFingerprint: textFromPtr(req.IdempotencyFingerprint),
		TenantID:               req.TenantID,
		DigitalEmployeeID:      req.DigitalEmployeeID,
		TeamID:                 nullUUIDFromUUID(req.TeamID),
		Title:                  req.Title,
		Description:            textFromPtr(req.Description),
		Priority:               req.Priority,
		ProviderType:           req.ProviderType,
		CreatorID:              nullUUIDFromPtr(req.CreatorID),
		TargetNodeID:           req.TargetNodeID,
		WorkspacePath:          textFromPtr(req.WorkspacePath),
		Params:                 params,
		RiskLevel:              textFromPtr(req.RiskLevel),
		NodeID:                 req.NodeID,
		RuntimeNodeID:          req.RuntimeNodeID,
		ProviderSessionID:      textFromPtr(req.ProviderSessionID),
		RunStatus:              string(req.RunStatus),
		CommandID:              req.CommandID,
		ExecutionInstanceID:    req.ExecutionInstanceID,
		TimeoutSec:             int4FromPtr(req.TimeoutSec),
		GraceSec:               int4FromPtr(req.GraceSec),
		RunKind:                req.RunKind,
		ResumeOfRunID:          nullUUIDFromPtr(req.ResumeOfRunID),
		ChatThreadID:           nullUUIDFromPtr(req.ChatThreadID),
		ProjectID:              req.ProjectID,
	})
	if err != nil {
		return nil, mapCreateRunError(err, req)
	}

	return r.GetRun(ctx, req.TenantID, req.DigitalEmployeeID, row.RunID)
}

func mapCreateRunError(err error, req CreateRunRecordRequest) error {
	if errors.Is(err, pgx.ErrNoRows) && req.IdempotencyKey != nil {
		return fmt.Errorf("%w: idempotency fingerprint mismatch", ErrConflict)
	}
	return err
}

func (r *PgRunRepository) UpdateRunStatus(ctx context.Context, req UpdateRunStatusRequest) (*DigitalEmployeeRun, error) {
	result, err := nullableJSONBytesFromMap(req.Result, "result")
	if err != nil {
		return nil, err
	}
	diagnostic, err := nullableJSONBytesFromMap(req.Diagnostic, "diagnostic")
	if err != nil {
		return nil, err
	}
	workProducts, err := nullableJSONBytesFromWorkProducts(req.WorkProducts, "work_products")
	if err != nil {
		return nil, err
	}
	sessionState, err := nullableJSONBytesFromMap(req.SessionState, "session_state")
	if err != nil {
		return nil, err
	}

	run, err := r.q.UpdateDigitalEmployeeRunStatus(ctx, queries.UpdateDigitalEmployeeRunStatusParams{
		Status:                    string(req.Status),
		Result:                    result,
		ErrorMessage:              textFromPtr(req.ErrorMessage),
		Diagnostic:                diagnostic,
		LogRef:                    textFromPtr(req.LogRef),
		RawResultRef:              textFromPtr(req.RawResultRef),
		WorkProducts:              workProducts,
		SessionState:              sessionState,
		ErrorCode:                 textFromPtr(req.ErrorCode),
		ErrorFamily:               textFromPtr(req.ErrorFamily),
		ExitCode:                  int4FromPtr(req.ExitCode),
		Signal:                    textFromPtr(req.Signal),
		ProviderSessionExternalID: textFromPtr(req.ProviderSessionExternalID),
		TenantID:                  req.TenantID,
		RunID:                     req.RunID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return digitalEmployeeRunFromQuery(run), nil
}

func (r *PgRunRepository) HasRunEventSequence(ctx context.Context, tenantID, taskID, runID uuid.UUID, sequenceNumber int32) (bool, error) {
	event, err := r.q.GetTaskEvent(ctx, queries.GetTaskEventParams{
		TaskID:         taskID,
		SequenceNumber: sequenceNumber,
		TenantID:       uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
	})
	if err != nil {
		if errors.Is(mapNoRows(err), ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return event.RunID.Valid && event.RunID.UUID == runID, nil
}

func (r *PgRunRepository) CreateTaskEventIfAbsent(ctx context.Context, req CreateRunEventRecordRequest) (bool, error) {
	payload, err := jsonBytesFromMap(redactRuntimeEventPayloadForPersistence(req.Payload), "payload")
	if err != nil {
		return false, err
	}
	metadata, err := jsonBytesFromMap(redactRuntimeEventPayloadForPersistence(req.Metadata), "metadata")
	if err != nil {
		return false, err
	}

	row, err := r.q.CreateTaskEventIfAbsent(ctx, queries.CreateTaskEventIfAbsentParams{
		TenantID:       req.TenantID,
		TaskID:         req.TaskID,
		RunID:          req.RunID,
		EventType:      req.EventType,
		SequenceNumber: req.SequenceNumber,
		Payload:        payload,
		CommandID:      textFromPtr(req.CommandID),
		RawEventRef:    textFromPtr(req.RawEventRef),
		LogRef:         textFromPtr(req.LogRef),
		Metadata:       metadata,
	})
	return row.Inserted, err
}

func (r *PgRunRepository) UpsertProviderSession(ctx context.Context, req UpsertProviderSessionRequest) (uuid.UUID, error) {
	sessionParams, err := jsonBytesFromMap(req.SessionParams, "session_params")
	if err != nil {
		return uuid.Nil, err
	}
	sessionState, err := jsonBytesFromMap(req.SessionState, "session_state")
	if err != nil {
		return uuid.Nil, err
	}
	metadata, err := jsonBytesFromMap(redactRuntimeEventPayloadForPersistence(req.Metadata), "metadata")
	if err != nil {
		return uuid.Nil, err
	}

	session, err := r.q.UpsertProviderSessionByExternalID(ctx, queries.UpsertProviderSessionByExternalIDParams{
		TenantID:            req.TenantID,
		ProviderSessionID:   req.ProviderSessionID,
		DigitalEmployeeID:   req.DigitalEmployeeID,
		ExecutionInstanceID: req.ExecutionInstanceID,
		RuntimeNodeID:       req.RuntimeNodeID,
		ProviderType:        req.ProviderType,
		Status:              req.Status,
		Recoverable:         req.Recoverable,
		SessionDisplayID:    textFromPtr(req.SessionDisplayID),
		SessionParams:       sessionParams,
		SessionState:        sessionState,
		LastSequenceNumber:  req.LastSequenceNumber,
		LastCommandID:       textFromPtr(req.LastCommandID),
		LastRunID:           nullUUIDFromPtr(req.LastRunID),
		LastErrorFamily:     textFromPtr(req.LastErrorFamily),
		Metadata:            metadata,
		ProjectTaskRootID:   nullUUIDFromPtr(req.ProjectTaskRootID),
	})
	if err != nil {
		return uuid.Nil, err
	}
	return session.ID, nil
}

func (r *PgRunRepository) FindProviderSessionForTaskRoot(ctx context.Context, tenantID, employeeID, taskRootID uuid.UUID) (string, error) {
	providerSessionID, err := r.q.FindProviderSessionForTaskRoot(ctx, queries.FindProviderSessionForTaskRootParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		ProjectTaskRootID: taskRootID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return providerSessionID, nil
}

func (r *PgRunRepository) GetRunTaskMetadata(ctx context.Context, tenantID, taskID uuid.UUID) (map[string]any, error) {
	task, err := r.q.GetTask(ctx, queries.GetTaskParams{
		ID:       taskID,
		TenantID: uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	params, err := mapFromJSONB(task.Params, "params")
	if err != nil {
		return nil, err
	}
	metadata, _ := params["metadata"].(map[string]any)
	return metadata, nil
}

func (r *PgRunRepository) CreateProviderSessionEventIfAbsent(ctx context.Context, req CreateProviderSessionEventRecordRequest) (uuid.UUID, error) {
	payload, err := jsonBytesFromMap(redactRuntimeEventPayloadForPersistence(req.Payload), "payload")
	if err != nil {
		return uuid.Nil, err
	}
	sessionStatePatch, err := jsonBytesFromMap(req.SessionStatePatch, "session_state_patch")
	if err != nil {
		return uuid.Nil, err
	}
	metadata, err := jsonBytesFromMap(redactRuntimeEventPayloadForPersistence(req.Metadata), "metadata")
	if err != nil {
		return uuid.Nil, err
	}

	event, err := r.q.CreateProviderSessionEventIfAbsent(ctx, queries.CreateProviderSessionEventIfAbsentParams{
		EventType:           req.EventType,
		SequenceNumber:      req.SequenceNumber,
		Payload:             payload,
		RequestID:           textFromPtr(req.RequestID),
		CommandID:           textFromPtr(req.CommandID),
		RawEventRef:         textFromPtr(req.RawEventRef),
		LogRef:              textFromPtr(req.LogRef),
		SessionStatePatch:   sessionStatePatch,
		Metadata:            metadata,
		ProviderSessionUuid: req.ProviderSessionUUID,
		TenantID:            req.TenantID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	return event.ID, nil
}

func (r *PgRunRepository) CreateCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error {
	payload, err := jsonBytesFromMap(redactRuntimeEventPayloadForPersistence(req.Payload), "payload")
	if err != nil {
		return err
	}
	_, err = r.q.CreateRuntimeCommandReceipt(ctx, queries.CreateRuntimeCommandReceiptParams{
		TenantID:      req.TenantID,
		CommandID:     req.CommandID,
		CommandType:   req.CommandType,
		RuntimeNodeID: req.RuntimeNodeID,
		NodeID:        req.NodeID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		Status:        req.Status,
		Payload:       payload,
		DispatchedAt:  timestamptzFromPtr(req.DispatchedAt),
	})
	return err
}

func (r *PgRunRepository) GetCommandReceipt(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	receipt, err := r.q.GetRuntimeCommandReceiptByCommandID(ctx, queries.GetRuntimeCommandReceiptByCommandIDParams{
		TenantID:  tenantID,
		CommandID: commandID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return runtimeCommandReceiptFromQuery(receipt), nil
}

func (r *PgRunRepository) GetCommandReceiptForUpdate(ctx context.Context, tenantID uuid.UUID, commandID string) (*RuntimeCommandReceipt, error) {
	receipt, err := r.q.GetRuntimeCommandReceiptByCommandIDForUpdate(ctx, queries.GetRuntimeCommandReceiptByCommandIDForUpdateParams{
		TenantID:  tenantID,
		CommandID: commandID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return runtimeCommandReceiptFromQuery(receipt), nil
}

func (r *PgRunRepository) UpdateCommandReceipt(ctx context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error) {
	result, err := nullableJSONBytesFromMap(req.Result, "result")
	if err != nil {
		return nil, err
	}

	receipt, err := r.q.UpdateRuntimeCommandReceiptStatus(ctx, queries.UpdateRuntimeCommandReceiptStatusParams{
		Status:       req.Status,
		Result:       result,
		ErrorMessage: textFromPtr(req.ErrorMessage),
		TenantID:     req.TenantID,
		CommandID:    req.CommandID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	return runtimeCommandReceiptFromQuery(receipt), nil
}

func (r *PgRunRepository) ListCommandReceiptsByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID, commandType string, limit int32) ([]RuntimeCommandReceipt, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.q.ListRuntimeCommandReceiptsByResource(ctx, queries.ListRuntimeCommandReceiptsByResourceParams{
		TenantID:     tenantID,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		CommandType:  commandType,
		LimitCount:   limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]RuntimeCommandReceipt, 0, len(rows))
	for _, row := range rows {
		if mapped := runtimeCommandReceiptFromQuery(row); mapped != nil {
			out = append(out, *mapped)
		}
	}
	return out, nil
}

func (r *PgRunRepository) UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	record, err := r.q.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		Status:   string(status),
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, mapNoRows(err)
	}
	return digitalEmployeeRecordFromQuery(record)
}

func (r *PgRunRepository) DeleteDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) error {
	return r.q.DeleteDigitalEmployee(ctx, queries.DeleteDigitalEmployeeParams{
		ID:       employeeID,
		TenantID: tenantID,
	})
}

func runStatusFromString(value string) DigitalEmployeeRunStatus {
	return DigitalEmployeeRunStatus(value)
}

func jsonMapFromBytes(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{"_decode_error": err.Error()}
	}
	if out == nil {
		return map[string]any{}
	}
	return out
}

func redactRuntimeEventPayload(payload map[string]any) map[string]any {
	return redactRuntimeEventSecrets(payload)
}

func redactRuntimeEventPayloadForPersistence(payload map[string]any) map[string]any {
	return dropRuntimeLocalPathKeys(redactRuntimeEventSecrets(payload))
}

func redactRuntimeEventSecrets(payload map[string]any) map[string]any {
	blocked := map[string]struct{}{
		"authorization": {},
		"password":      {},
		"secret":        {},
		"token":         {},
		"access_token":  {},
		"refresh_token": {},
		"api_key":       {},
		"apikey":        {},
		"private_key":   {},
	}
	return redactMap(payload, blocked)
}

func redactMap(payload map[string]any, blocked map[string]struct{}) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	redacted := make(map[string]any, len(payload))
	for key, value := range payload {
		if strings.EqualFold(key, "environment") {
			redacted[key] = redactEnvironmentPayload(value)
			continue
		}
		if _, ok := blocked[strings.ToLower(key)]; ok {
			redacted[key] = "[redacted]"
			continue
		}
		redacted[key] = redactValue(value, blocked)
	}
	return redacted
}

func redactValue(value any, blocked map[string]struct{}) any {
	switch typed := value.(type) {
	case map[string]any:
		return redactMap(typed, blocked)
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactValue(item, blocked)
		}
		return redacted
	case []map[string]any:
		redacted := make([]map[string]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactMap(item, blocked)
		}
		return redacted
	default:
		return value
	}
}

func dropRuntimeLocalPathKeys(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	cleaned := make(map[string]any, len(payload))
	for key, value := range payload {
		if runtimeLocalPathKey(key) {
			continue
		}
		cleaned[key] = dropRuntimeLocalPathValue(value)
	}
	return cleaned
}

func dropRuntimeLocalPathValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return dropRuntimeLocalPathKeys(typed)
	case []any:
		cleaned := make([]any, len(typed))
		for index, item := range typed {
			cleaned[index] = dropRuntimeLocalPathValue(item)
		}
		return cleaned
	case []map[string]any:
		cleaned := make([]map[string]any, len(typed))
		for index, item := range typed {
			cleaned[index] = dropRuntimeLocalPathKeys(item)
		}
		return cleaned
	default:
		return value
	}
}

func runtimeLocalPathKey(key string) bool {
	switch strings.ToLower(key) {
	case "agent_home_dir", "workspace_base_dir", "workspace_path", "mcp_config_path", "employee_capability_dir":
		return true
	default:
		return false
	}
}

func redactEnvironmentPayload(value any) any {
	switch typed := value.(type) {
	case []any:
		redacted := make([]any, len(typed))
		for i, item := range typed {
			redacted[i] = redactEnvironmentEntry(item)
		}
		return redacted
	case []map[string]any:
		redacted := make([]map[string]any, len(typed))
		for i, item := range typed {
			if entry, ok := redactEnvironmentEntry(item).(map[string]any); ok {
				redacted[i] = entry
			}
		}
		return redacted
	default:
		return value
	}
}

func redactEnvironmentEntry(value any) any {
	entry, ok := value.(map[string]any)
	if !ok {
		return value
	}
	redacted := make(map[string]any, len(entry))
	for key, item := range entry {
		if strings.EqualFold(key, "value") {
			redacted[key] = "[redacted]"
			continue
		}
		redacted[key] = item
	}
	return redacted
}

func digitalEmployeeRunFromQuery(run queries.TaskRun) *DigitalEmployeeRun {
	return &DigitalEmployeeRun{
		ID:                        run.ID,
		TenantID:                  run.TenantID,
		TaskID:                    run.TaskID,
		DigitalEmployeeID:         uuidFromNull(run.DigitalEmployeeID),
		ExecutionInstanceID:       uuidFromNull(run.ExecutionInstanceID),
		RuntimeNodeID:             uuidFromNull(run.RuntimeNodeID),
		NodeID:                    run.NodeID,
		CommandID:                 stringFromText(run.CommandID),
		ProviderType:              stringFromText(run.ProviderType),
		ProviderSessionID:         stringPtrFromText(run.ProviderSessionID),
		ProviderSessionExternalID: stringPtrFromText(run.ProviderSessionExternalID),
		Status:                    runStatusFromString(run.Status),
		Result:                    jsonMapFromBytes(run.Result),
		Diagnostic:                jsonMapFromBytes(run.Diagnostic),
		LogRef:                    stringPtrFromText(run.LogRef),
		RawResultRef:              stringPtrFromText(run.RawResultRef),
		WorkProducts:              workProductsFromBytes(run.WorkProducts),
		SessionState:              jsonMapFromBytes(run.SessionState),
		ErrorMessage:              stringPtrFromText(run.ErrorMessage),
		ErrorCode:                 stringPtrFromText(run.ErrorCode),
		ErrorFamily:               stringPtrFromText(run.ErrorFamily),
		ExitCode:                  int32PtrFromInt4(run.ExitCode),
		Signal:                    stringPtrFromText(run.Signal),
		TimedOut:                  run.TimedOut,
		FailureAcknowledgedAt:     timePtrFromTimestamptz(run.FailureAcknowledgedAt),
		IdempotencyKey:            stringPtrFromText(run.IdempotencyKey),
		IdempotencyFingerprint:    stringPtrFromText(run.IdempotencyFingerprint),
		TimeoutSec:                int32PtrFromInt4(run.TimeoutSec),
		GraceSec:                  int32PtrFromInt4(run.GraceSec),
		StartedAt:                 timeFromTimestamptz(run.StartedAt),
		CompletedAt:               timePtrFromTimestamptz(run.CompletedAt),
		FinishedAt:                timePtrFromTimestamptz(run.FinishedAt),
		CreatedAt:                 timeFromTimestamptz(run.CreatedAt),
		UpdatedAt:                 timeFromTimestamptz(run.UpdatedAt),
	}
}

// digitalEmployeeRunListItemFromDetailedRow maps a joined task_runs row (with task title,
// project association, work-product count) into a DigitalEmployeeRunListItem. The row's
// TaskRun-equivalent columns are reassembled into a queries.TaskRun so the existing
// digitalEmployeeRunFromQuery mapper is reused. DurationSec is computed in Go from the
// already-nullable FinishedAt/StartedAt columns rather than via SQL EXTRACT, because
// sqlc infers EXTRACT(...)::float8 as a non-nullable float64 which would crash pgx when
// finished_at IS NULL for in-progress runs.
func digitalEmployeeRunListItemFromDetailedRow(row queries.ListDigitalEmployeeRunsDetailedRow) DigitalEmployeeRunListItem {
	run := digitalEmployeeRunFromQuery(queries.TaskRun{
		ID:                        row.ID,
		TenantID:                  row.TenantID,
		TaskID:                    row.TaskID,
		DigitalEmployeeID:         row.DigitalEmployeeID,
		ExecutionInstanceID:       row.ExecutionInstanceID,
		RuntimeNodeID:             row.RuntimeNodeID,
		NodeID:                    row.NodeID,
		CommandID:                 row.CommandID,
		ProviderType:              row.ProviderType,
		ProviderSessionID:         row.ProviderSessionID,
		ProviderSessionExternalID: row.ProviderSessionExternalID,
		Status:                    row.Status,
		Result:                    row.Result,
		Diagnostic:                row.Diagnostic,
		LogRef:                    row.LogRef,
		RawResultRef:              row.RawResultRef,
		WorkProducts:              row.WorkProducts,
		SessionState:              row.SessionState,
		ErrorMessage:              row.ErrorMessage,
		ErrorCode:                 row.ErrorCode,
		ErrorFamily:               row.ErrorFamily,
		ExitCode:                  row.ExitCode,
		Signal:                    row.Signal,
		TimedOut:                  row.TimedOut,
		IdempotencyKey:            row.IdempotencyKey,
		TimeoutSec:                row.TimeoutSec,
		GraceSec:                  row.GraceSec,
		StartedAt:                 row.StartedAt,
		CompletedAt:               row.CompletedAt,
		FinishedAt:                row.FinishedAt,
		CreatedAt:                 row.CreatedAt,
		UpdatedAt:                 row.UpdatedAt,
		FailureAcknowledgedAt:     row.FailureAcknowledgedAt,
		FailureAcknowledgedBy:     row.FailureAcknowledgedBy,
	})
	// run_kind/resume_of_run_id live on the joined tasks columns, not on the
	// task_runs subset reassembled above; carry them through explicitly (see
	// the equivalent fixup in GetRun).
	run.RunKind = row.RunKind
	run.ResumeOfRunID = uuidPtrFromNull(row.ResumeOfRunID)
	run.ChatThreadID = effectiveChatThreadID(row.RunKind, row.ChatThreadID, row.ID)

	item := DigitalEmployeeRunListItem{
		Run:              run,
		TaskTitle:        row.TaskTitle,
		WorkProductCount: row.WorkProductCount,
		DurationSec:      durationSecondsFromBounds(row.StartedAt, row.FinishedAt),
	}
	pid := row.ProjectID
	item.ProjectID = &pid
	if row.ProjectName.Valid {
		name := row.ProjectName.String
		item.ProjectName = &name
	}
	item.ProjectDeleted = row.ProjectDeleted
	return item
}

// durationSecondsFromBounds returns finished-start in seconds when both timestamps are
// valid and finished is not before started; nil otherwise. This guards against negative
// or nonsense durations from partial timestamp state.
func durationSecondsFromBounds(startedAt, finishedAt pgtype.Timestamptz) *float64 {
	if !startedAt.Valid || !finishedAt.Valid {
		return nil
	}
	delta := finishedAt.Time.Sub(startedAt.Time)
	if delta < 0 {
		return nil
	}
	seconds := delta.Seconds()
	return &seconds
}

func runtimeCommandEventFromTaskEvent(event queries.TaskEvent) RuntimeCommandEventWriteback {
	return RuntimeCommandEventWriteback{
		EventType:      event.EventType,
		SequenceNumber: event.SequenceNumber,
		Payload:        jsonMapFromBytes(event.Payload),
		LogRef:         stringPtrFromText(event.LogRef),
		RawEventRef:    stringPtrFromText(event.RawEventRef),
		Metadata:       jsonMapFromBytes(event.Metadata),
	}
}

func runtimeCommandReceiptFromQuery(receipt queries.RuntimeCommandReceipt) *RuntimeCommandReceipt {
	return &RuntimeCommandReceipt{
		ID:            receipt.ID,
		TenantID:      receipt.TenantID,
		CommandID:     receipt.CommandID,
		CommandType:   receipt.CommandType,
		RuntimeNodeID: receipt.RuntimeNodeID,
		NodeID:        receipt.NodeID,
		ResourceType:  receipt.ResourceType,
		ResourceID:    receipt.ResourceID,
		Status:        receipt.Status,
		Payload:       jsonMapFromBytes(receipt.Payload),
		Result:        jsonMapFromBytes(receipt.Result),
		ErrorMessage:  stringPtrFromText(receipt.ErrorMessage),
		DispatchedAt:  timePtrFromTimestamptz(receipt.DispatchedAt),
		CompletedAt:   timePtrFromTimestamptz(receipt.CompletedAt),
		CreatedAt:     timeFromTimestamptz(receipt.CreatedAt),
		UpdatedAt:     timeFromTimestamptz(receipt.UpdatedAt),
	}
}

func jsonBytesFromMap(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func nullableJSONBytesFromMap(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return jsonBytesFromMap(value, field)
}

func nullableJSONBytesFromWorkProducts(value []WorkProduct, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func workProductsFromBytes(raw []byte) []WorkProduct {
	if len(raw) == 0 {
		return []WorkProduct{}
	}
	var out []WorkProduct
	if err := json.Unmarshal(raw, &out); err != nil {
		return []WorkProduct{}
	}
	if out == nil {
		return []WorkProduct{}
	}
	return out
}

func int4FromPtr(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func int32PtrFromInt4(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copied := value.Int32
	return &copied
}

func uuidFromNull(value uuid.NullUUID) uuid.UUID {
	if !value.Valid {
		return uuid.Nil
	}
	return value.UUID
}

func stringFromText(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func stringFromMap(value map[string]any, key string) string {
	item, ok := value[key]
	if !ok {
		return ""
	}
	text, _ := item.(string)
	return text
}

// GetTeamConstitutionForDispatch 取团队当前生效宪法与版本号，供派发时注入
// provider 提示词（spec §5.3，D9 只作约束文本，不参与门禁判定）。
// 团队不存在或无宪法时返回空规则与版本 0，派发照常进行——宪法是约束，不是前置条件。
func (r *PgRunRepository) GetTeamConstitutionForDispatch(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConstitutionForDispatch, error) {
	if teamID == uuid.Nil {
		return TeamConstitutionForDispatch{}, nil
	}
	row, err := r.q.GetTeamConstitutionForDispatch(ctx, queries.GetTeamConstitutionForDispatchParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TeamConstitutionForDispatch{}, nil
		}
		return TeamConstitutionForDispatch{}, err
	}
	snapshot := map[string]any{}
	if len(row.Constitution) > 0 {
		if err := json.Unmarshal(row.Constitution, &snapshot); err != nil {
			return TeamConstitutionForDispatch{}, fmt.Errorf("decode team constitution: %w", err)
		}
	}
	return TeamConstitutionForDispatch{
		Prompt:         tenant.RenderConstitutionPrompt(tenant.ConstitutionRulesFromSnapshot(snapshot)),
		RevisionNumber: row.RevisionNumber,
	}, nil
}
