package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

const maxProjectEventAppendAttempts = 3
const maxProjectConfigRevisionAttempts = 3

type PgRepository struct {
	q  *queries.Queries
	db projectTransactionBeginner
}

type projectTransactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func NewPgRepository(q *queries.Queries, db ...projectTransactionBeginner) Repository {
	var beginner projectTransactionBeginner
	if len(db) > 0 {
		beginner = db[0]
	}
	return &PgRepository{q: q, db: beginner}
}

func withProjectQueries[T any](ctx context.Context, r *PgRepository, label string, fn func(*queries.Queries) (T, error)) (T, error) {
	var zero T
	if r.db == nil {
		return fn(r.q)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return zero, fmt.Errorf("begin %s transaction: %w", label, err)
	}
	result, err := fn(r.q.WithTx(tx))
	if err != nil {
		_ = tx.Rollback(ctx)
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, fmt.Errorf("commit %s transaction: %w", label, err)
	}
	return result, nil
}

func (r *PgRepository) CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error) {
	coordinationPolicy, err := jsonbObject(req.CoordinationPolicy, "coordination_policy")
	if err != nil {
		return Project{}, err
	}
	approvalPolicy, err := jsonbObject(req.ApprovalPolicy, "approval_policy")
	if err != nil {
		return Project{}, err
	}
	evidencePolicy, err := jsonbObject(req.EvidencePolicy, "evidence_policy")
	if err != nil {
		return Project{}, err
	}
	row, err := r.q.CreateProject(ctx, queries.CreateProjectParams{
		ID:                     projectID,
		TenantID:               req.TenantID,
		TeamID:                 nullUUID(req.TeamID),
		Name:                   req.Name,
		Description:            textOrNull(req.Description),
		Goal:                   textOrNull(req.Goal),
		Status:                 string(ProjectStatusRunning),
		HumanOwnerUserID:       req.HumanOwnerUserID,
		LeaderUserID:           nullUUID(req.LeaderUserID),
		AcceptanceUserID:       nullUUID(req.AcceptanceUserID),
		CoordinationWorkflowID: textOrNull(workflowID),
		CoordinationStatus:     textOrNull("registered"),
		CoordinationPolicy:     coordinationPolicy,
		ApprovalPolicy:         approvalPolicy,
		EvidencePolicy:         evidencePolicy,
	})
	if err != nil {
		return Project{}, err
	}
	return projectFromRecord(row)
}

func (r *PgRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	row, err := r.q.GetProject(ctx, queries.GetProjectParams{TenantID: tenantID, ID: projectID})
	if err != nil {
		return Project{}, projectRepositoryError(err)
	}
	return projectFromRecord(row)
}

func (r *PgRepository) ListProjects(ctx context.Context, req ListProjectsRequest) ([]Project, error) {
	rows, err := r.q.ListProjects(ctx, queries.ListProjectsParams{
		TenantID: req.TenantID,
		Status:   projectStatusPtr(req.Status),
		Q:        textOrNull(req.Query),
		Limit:    req.Limit,
		Offset:   req.Offset,
	})
	if err != nil {
		return nil, err
	}
	return projectsFromRecords(rows)
}

func (r *PgRepository) ListWorkflowInstances(ctx context.Context, req ListWorkflowInstancesRequest) ([]WorkflowInstanceSummary, error) {
	rows, err := r.q.ListWorkflowInstances(ctx, queries.ListWorkflowInstancesParams{
		TenantID:    req.TenantID,
		ActorUserID: req.ActorUserID,
		ProjectID:   nullUUID(req.ProjectID),
		Q:           textOrNull(req.Query),
		Limit:       req.Limit,
		Offset:      req.Offset,
	})
	if err != nil {
		return nil, projectRepositoryError(err)
	}
	items := make([]WorkflowInstanceSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, WorkflowInstanceSummary{
			DemandID:                  row.DemandID,
			ProjectID:                 row.ProjectID,
			ProjectName:               row.ProjectName,
			Title:                     row.Title,
			SubmittedByUserID:         row.SubmittedByUserID,
			SubmittedByDisplayName:    row.SubmittedByDisplayName,
			Status:                    WorkflowInstanceStatus(row.Status),
			StatusReason:              row.StatusReason,
			CreatedAt:                 row.CreatedAt.Time,
			UpdatedAt:                 row.UpdatedAt.Time,
			SelectedCoordinationJobID: ptrUUID(row.SelectedCoordinationJobID),
			Progress: WorkflowInstanceProgress{
				TotalNodes:        row.TotalNodes,
				CompletedNodes:    row.CompletedNodes,
				RunningNodes:      row.RunningNodes,
				BlockedNodes:      row.BlockedNodes,
				WaitingHumanNodes: row.WaitingHumanNodes,
				PlannedNodes:      row.PlannedNodes,
				FailedNodes:       row.FailedNodes,
				CancelledNodes:    row.CancelledNodes,
			},
			Priority: workflowPriorityFromRow(row.PriorityValue, row.PriorityLabel, row.PrioritySource),
			Risk:     workflowRiskFromRow(row.RiskLevel, row.RiskLabel, row.RiskSource),
			SLA:      workflowSLAFromRow(row.SlaDueAt, row.SlaRemainingSeconds, row.SlaBreached, row.SlaLabel, row.SlaSource),
			CurrentBlocker: workflowCurrentBlockerFromRow(
				row.CurrentBlockerType,
				row.CurrentBlockerTitle,
				row.CurrentBlockerResourceID,
			),
			RecentEvent: workflowRecentEventFromRow(
				row.RecentEventType,
				row.RecentEventSummary,
				row.RecentEventOccurredAt,
			),
		})
	}
	return items, nil
}

func (r *PgRepository) CanUseTeamForProject(ctx context.Context, tenantID, userID, teamID uuid.UUID) (bool, error) {
	return r.q.UserHasActiveProjectTeamScope(ctx, queries.UserHasActiveProjectTeamScopeParams{
		TenantID: tenantID,
		UserID:   userID,
		TeamID:   teamID,
	})
}

func workflowPriorityFromRow(value, label, source string) *WorkflowInstancePriority {
	if value == "" {
		return nil
	}
	return &WorkflowInstancePriority{Value: value, Label: stringValueOr(label, value), Source: stringValueOr(source, "unknown")}
}

func workflowRiskFromRow(level, label, source string) *WorkflowInstanceRisk {
	if level == "" {
		return nil
	}
	return &WorkflowInstanceRisk{Level: level, Label: stringValueOr(label, level), Source: stringValueOr(source, "unknown")}
}

func workflowSLAFromRow(dueAt pgtype.Timestamptz, remaining int32, breached bool, label, source string) *WorkflowInstanceSLA {
	if !dueAt.Valid && remaining == 0 && label == "" {
		return nil
	}
	var due *time.Time
	if dueAt.Valid {
		due = &dueAt.Time
	}
	seconds := remaining
	return &WorkflowInstanceSLA{
		DueAt:            due,
		RemainingSeconds: &seconds,
		Breached:         breached,
		Label:            label,
		Source:           stringValueOr(source, "unknown"),
	}
}

func workflowCurrentBlockerFromRow(blockerType, title string, resourceID uuid.UUID) *WorkflowInstanceCurrentBlocker {
	if blockerType == "" || resourceID == uuid.Nil {
		return nil
	}
	return &WorkflowInstanceCurrentBlocker{
		Type:       blockerType,
		Title:      stringValueOr(title, blockerType),
		ResourceID: &resourceID,
	}
}

func workflowRecentEventFromRow(eventType, summary string, occurredAt pgtype.Timestamptz) *WorkflowInstanceRecentEvent {
	if eventType == "" || !occurredAt.Valid {
		return nil
	}
	return &WorkflowInstanceRecentEvent{
		EventType:  eventType,
		Summary:    stringValueOr(summary, eventType),
		OccurredAt: occurredAt.Time,
	}
}

func stringValueOr(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func (r *PgRepository) UpdateProjectConfig(ctx context.Context, req UpdateProjectConfigRequest) (Project, error) {
	coordinationPolicy, err := jsonbObjectOrNull(req.CoordinationPolicy, "coordination_policy")
	if err != nil {
		return Project{}, err
	}
	approvalPolicy, err := jsonbObjectOrNull(req.ApprovalPolicy, "approval_policy")
	if err != nil {
		return Project{}, err
	}
	evidencePolicy, err := jsonbObjectOrNull(req.EvidencePolicy, "evidence_policy")
	if err != nil {
		return Project{}, err
	}
	row, err := r.q.UpdateProject(ctx, queries.UpdateProjectParams{
		TenantID:           req.TenantID,
		ID:                 req.ProjectID,
		Name:               textOrNull(req.Name),
		Description:        textOrNull(req.Description),
		Goal:               textOrNull(req.Goal),
		HumanOwnerUserID:   nullUUIDIfNotNil(req.HumanOwnerUserID),
		LeaderUserID:       nullUUID(req.LeaderUserID),
		AcceptanceUserID:   nullUUID(req.AcceptanceUserID),
		CoordinationPolicy: coordinationPolicy,
		ApprovalPolicy:     approvalPolicy,
		EvidencePolicy:     evidencePolicy,
	})
	if err != nil {
		return Project{}, err
	}
	return projectFromRecord(row)
}

func (r *PgRepository) ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	return r.archiveProjectWithQueries(ctx, r.q, tenantID, projectID)
}

func (r *PgRepository) archiveProjectWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID uuid.UUID) (Project, error) {
	row, err := q.ArchiveProject(ctx, queries.ArchiveProjectParams{TenantID: tenantID, ID: projectID})
	if err != nil {
		return Project{}, err
	}
	return projectFromRecord(row)
}

// TransitionProjectStatus moves a project's status forward only when its current
// status is in fromStatuses. If the guard does not match (e.g. already in the target
// status), it returns ErrProjectNotFound so callers can treat it as an idempotent no-op.
func (r *PgRepository) TransitionProjectStatus(ctx context.Context, tenantID, projectID uuid.UUID, fromStatuses []string, toStatus string) (Project, error) {
	row, err := r.q.TransitionProjectStatus(ctx, queries.TransitionProjectStatusParams{
		TenantID:     tenantID,
		ID:           projectID,
		FromStatuses: fromStatuses,
		ToStatus:     toStatus,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Project{}, ErrProjectNotFound
		}
		return Project{}, err
	}
	return projectFromRecord(row)
}

// AreAllProjectDemandsTerminal reports whether a project has at least one demand and
// every one of its demands is in a terminal state (completed/failed/cancelled).
func (r *PgRepository) AreAllProjectDemandsTerminal(ctx context.Context, tenantID, projectID uuid.UUID) (bool, error) {
	counts, err := r.q.CountProjectDemandsByTerminality(ctx, queries.CountProjectDemandsByTerminalityParams{
		TenantID:  tenantID,
		ProjectID: projectID,
	})
	if err != nil {
		return false, err
	}
	return counts.TotalCount > 0 && counts.NonTerminalCount == 0, nil
}

func (r *PgRepository) ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	created, err := withProjectQueries(ctx, r, "project members", func(q *queries.Queries) ([]ProjectMember, error) {
		return r.replaceProjectMembersWithQueries(ctx, q, tenantID, projectID, members)
	})
	return created, err
}

func (r *PgRepository) replaceProjectMembersWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	if err := q.ReplaceProjectMembersDelete(ctx, queries.ReplaceProjectMembersDeleteParams{TenantID: tenantID, ProjectID: projectID}); err != nil {
		return nil, err
	}
	created := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		settings, err := jsonbObject(member.Settings, "settings")
		if err != nil {
			return nil, err
		}
		row, err := q.CreateProjectMember(ctx, queries.CreateProjectMemberParams{
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       string(member.PrincipalType),
			PrincipalID:         member.PrincipalID,
			ProjectRole:         string(member.ProjectRole),
			DisplayNameSnapshot: textOrNull(member.DisplayNameSnapshot),
			Status:              "active",
			Settings:            settings,
		})
		if err != nil {
			return nil, err
		}
		mapped, err := memberFromRecord(row)
		if err != nil {
			return nil, err
		}
		created = append(created, mapped)
	}
	return created, nil
}

func (r *PgRepository) ListProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID) ([]ProjectMember, error) {
	rows, err := r.q.ListProjectMembers(ctx, queries.ListProjectMembersParams{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return nil, err
	}
	return membersFromRecords(rows)
}

func (r *PgRepository) ListProjectTasks(ctx context.Context, tenantID, projectID uuid.UUID, status *string, limit, offset int32) ([]ProjectTask, error) {
	rows, err := r.q.ListProjectTasks(ctx, queries.ListProjectTasksParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Status:    textPtr(status),
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return tasksFromRecords(rows)
}

func (r *PgRepository) ListDemandLaunchProjectTasks(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]ProjectTask, error) {
	rows, err := r.q.ListDemandLaunchProjectTasks(ctx, queries.ListDemandLaunchProjectTasksParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	return tasksFromRecords(rows)
}

func (r *PgRepository) listProjectTasksByDemand(ctx context.Context, tenantID, projectID, demandID uuid.UUID) ([]ProjectTask, error) {
	rows, err := r.q.ListProjectTasksByDemand(ctx, queries.ListProjectTasksByDemandParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
	})
	if err != nil {
		return nil, err
	}
	return tasksFromRecords(rows)
}

func (r *PgRepository) AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error) {
	return r.appendProjectEventWithQueries(ctx, r.q, event)
}

func (r *PgRepository) appendProjectEventWithQueries(ctx context.Context, q *queries.Queries, event AppendProjectEventRequest) (ProjectEvent, error) {
	payload, err := jsonbObject(event.Payload, "payload")
	if err != nil {
		return ProjectEvent{}, err
	}
	if err := q.LockProjectEventSequence(ctx, queries.LockProjectEventSequenceParams{
		TenantID:  event.TenantID,
		ProjectID: event.ProjectID,
	}); err != nil {
		return ProjectEvent{}, err
	}
	var lastErr error
	for attempt := 0; attempt < maxProjectEventAppendAttempts; attempt++ {
		latest, err := q.GetLatestProjectEventSequence(ctx, queries.GetLatestProjectEventSequenceParams{TenantID: event.TenantID, ProjectID: event.ProjectID})
		if err != nil {
			return ProjectEvent{}, err
		}
		row, err := q.CreateProjectEvent(ctx, queries.CreateProjectEventParams{
			TenantID:       event.TenantID,
			ProjectID:      event.ProjectID,
			SequenceNumber: latest + 1,
			EventType:      string(event.EventType),
			ActorType:      event.ActorType,
			ActorID:        event.ActorID,
			ResourceType:   textPtr(event.ResourceType),
			ResourceID:     textPtr(event.ResourceID),
			Summary:        textOrNull(event.Summary),
			Payload:        payload,
		})
		if err == nil {
			return eventFromRecord(row)
		}
		lastErr = err
		if !isProjectEventSequenceConflict(err) {
			return ProjectEvent{}, err
		}
	}
	return ProjectEvent{}, lastErr
}

func (r *PgRepository) ListProjectEvents(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectEvent, error) {
	rows, err := r.q.ListProjectEvents(ctx, queries.ListProjectEventsParams{TenantID: tenantID, ProjectID: projectID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	return eventsFromRecords(rows)
}

func (r *PgRepository) ListDemandLaunchEvents(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, projectTaskIDs, decisionRequestIDs []uuid.UUID, limit int32) ([]ProjectEvent, error) {
	rows, err := r.q.ListDemandLaunchProjectEvents(ctx, queries.ListDemandLaunchProjectEventsParams{
		TenantID:           tenantID,
		ProjectID:          projectID,
		CreatedEventID:     nullUUID(createdEventID),
		DemandID:           demandID,
		ProjectTaskIds:     uuidStrings(projectTaskIDs),
		DecisionRequestIds: uuidStrings(decisionRequestIDs),
		Limit:              limit,
	})
	if err != nil {
		return nil, err
	}
	return eventsFromRecords(rows)
}

func (r *PgRepository) GetProjectEvent(ctx context.Context, tenantID, projectID, eventID uuid.UUID) (ProjectEvent, error) {
	row, err := r.q.GetProjectEvent(ctx, queries.GetProjectEventParams{TenantID: tenantID, ProjectID: projectID, ID: eventID})
	if err != nil {
		return ProjectEvent{}, err
	}
	return eventFromRecord(row)
}

func (r *PgRepository) GetProjectEventByTypeAndActor(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (ProjectEvent, error) {
	row, err := r.q.GetProjectEventByTypeAndActor(ctx, queries.GetProjectEventByTypeAndActorParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: string(eventType),
		ActorID:   actorID,
	})
	if err != nil {
		return ProjectEvent{}, projectRepositoryError(err)
	}
	return eventFromRecord(row)
}

func (r *PgRepository) CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error) {
	sourceRefs, err := jsonbObject(req.SourceRefs, "source_refs")
	if err != nil {
		return ProjectDemand{}, err
	}
	attachments, err := jsonbArray(req.Attachments, "attachments")
	if err != nil {
		return ProjectDemand{}, err
	}
	row, err := r.q.CreateProjectDemand(ctx, queries.CreateProjectDemandParams{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		SubmittedByUserID: req.SubmittedByUserID,
		Title:             req.Title,
		Content:           textOrNull(req.Content),
		SourceType:        string(req.SourceType),
		SourceRefs:        sourceRefs,
		Attachments:       attachments,
		Status:            string(status),
		CreatedEventID:    nullUUID(createdEventID),
	})
	if err != nil {
		return ProjectDemand{}, err
	}
	return demandFromRecord(row)
}

func (r *PgRepository) ListProjectDemands(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectDemand, error) {
	rows, err := r.q.ListProjectDemands(ctx, queries.ListProjectDemandsParams{TenantID: tenantID, ProjectID: projectID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	return demandsFromRecords(rows)
}

func (r *PgRepository) CreateConfigRevision(ctx context.Context, req UpdateProjectConfigRequest, project Project, eventID uuid.UUID) (ProjectConfigRevision, error) {
	snapshotMap := projectConfigSnapshot(project)
	snapshot, err := jsonbObject(snapshotMap, "config_snapshot")
	if err != nil {
		return ProjectConfigRevision{}, err
	}
	changedSectionsValue := projectConfigChangedSections(req)
	changedSections, err := jsonbArray(changedSectionsValue, "changed_sections")
	if err != nil {
		return ProjectConfigRevision{}, err
	}
	diffSummary, err := jsonbObject(projectConfigDiffSummary(changedSectionsValue), "diff_summary")
	if err != nil {
		return ProjectConfigRevision{}, err
	}
	policyFingerprint, err := projectConfigPolicyFingerprint(snapshotMap)
	if err != nil {
		return ProjectConfigRevision{}, err
	}
	var lastErr error
	for attempt := 0; attempt < maxProjectConfigRevisionAttempts; attempt++ {
		latest, err := r.q.GetLatestProjectConfigRevisionNumber(ctx, queries.GetLatestProjectConfigRevisionNumberParams{TenantID: req.TenantID, ProjectID: req.ProjectID})
		if err != nil {
			return ProjectConfigRevision{}, err
		}
		var previousRevisionID uuid.NullUUID
		latestRevision, err := r.q.GetLatestProjectConfigRevision(ctx, queries.GetLatestProjectConfigRevisionParams{TenantID: req.TenantID, ProjectID: req.ProjectID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ProjectConfigRevision{}, err
		}
		if err == nil {
			previousRevisionID = uuid.NullUUID{UUID: latestRevision.ID, Valid: true}
		}
		row, err := r.q.CreateProjectConfigRevisionWithGovernanceFields(ctx, queries.CreateProjectConfigRevisionWithGovernanceFieldsParams{
			TenantID:           req.TenantID,
			ProjectID:          req.ProjectID,
			RevisionNumber:     latest + 1,
			ConfigSnapshot:     snapshot,
			ChangeSummary:      textOrNull("项目配置已更新"),
			ChangedSections:    changedSections,
			PreviousRevisionID: previousRevisionID,
			PolicyFingerprint:  textOrNull(policyFingerprint),
			DiffSummary:        diffSummary,
			CreatedByUserID:    req.ActorUserID,
			CreatedEventID:     nullUUID(&eventID),
		})
		if err == nil {
			return configRevisionFromRecord(row)
		}
		lastErr = err
		if !isProjectConfigRevisionConflict(err) {
			return ProjectConfigRevision{}, err
		}
	}
	return ProjectConfigRevision{}, lastErr
}

func (r *PgRepository) GetLatestConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectConfigRevision, error) {
	return r.GetLatestProjectConfigRevision(ctx, tenantID, projectID)
}

func (r *PgRepository) GetLatestProjectConfigRevision(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectConfigRevision, error) {
	row, err := r.q.GetLatestProjectConfigRevision(ctx, queries.GetLatestProjectConfigRevisionParams{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return ProjectConfigRevision{}, projectRepositoryError(err)
	}
	return configRevisionFromRecord(row)
}

func (r *PgRepository) GetProjectDemand(ctx context.Context, tenantID, demandID uuid.UUID) (ProjectDemand, error) {
	row, err := r.q.GetProjectDemand(ctx, queries.GetProjectDemandParams{TenantID: tenantID, ID: demandID})
	if err != nil {
		return ProjectDemand{}, err
	}
	return demandFromRecord(row)
}

func (r *PgRepository) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTask, error) {
	return r.getProjectTaskWithQueries(ctx, r.q, tenantID, projectTaskID)
}

func (r *PgRepository) getProjectTaskWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectTaskID uuid.UUID) (ProjectTask, error) {
	row, err := q.GetProjectTask(ctx, queries.GetProjectTaskParams{TenantID: tenantID, ID: projectTaskID})
	if err != nil {
		return ProjectTask{}, projectRepositoryError(err)
	}
	return taskFromRecord(row)
}

func (r *PgRepository) GetProjectTaskRunRuntimeNodeID(ctx context.Context, tenantID, projectTaskID, runID uuid.UUID) (uuid.UUID, error) {
	runtimeNodeID, err := r.q.GetProjectTaskRunRuntimeNodeID(ctx, queries.GetProjectTaskRunRuntimeNodeIDParams{
		TenantID:      tenantID,
		ProjectTaskID: projectTaskID,
		RunID:         runID,
	})
	if err != nil {
		return uuid.Nil, err
	}
	if !runtimeNodeID.Valid {
		return uuid.Nil, ErrProjectNotFound
	}
	return runtimeNodeID.UUID, nil
}

func (r *PgRepository) GetProjectTaskRunWorkProducts(ctx context.Context, tenantID, runID uuid.UUID) ([]any, error) {
	run, err := r.q.GetTaskRun(ctx, queries.GetTaskRunParams{
		ID:       runID,
		TenantID: uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
	})
	if err != nil {
		return nil, projectRepositoryError(err)
	}
	return anySliceFromJSON(run.WorkProducts)
}

func (r *PgRepository) CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error) {
	if req.TriggerEventID != nil {
		existing, err := r.GetCoordinationJobByTrigger(ctx, req.TenantID, req.WorkflowID, *req.TriggerEventID, req.JobType)
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, ErrProjectNotFound) {
			return CoordinationJob{}, err
		}
	}
	inputSnapshotRef, err := jsonbObject(req.InputSnapshotRef, "input_snapshot_ref")
	if err != nil {
		return CoordinationJob{}, err
	}
	row, err := r.q.CreateProjectCoordinationJob(ctx, queries.CreateProjectCoordinationJobParams{
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		WorkflowID:       req.WorkflowID,
		TriggerEventID:   nullUUID(req.TriggerEventID),
		JobType:          req.JobType,
		Status:           req.Status,
		InputSnapshotRef: inputSnapshotRef,
	})
	if err != nil {
		if req.TriggerEventID != nil && isPGUniqueConstraint(err, "uq_project_coordination_jobs_trigger") {
			return r.GetCoordinationJobByTrigger(ctx, req.TenantID, req.WorkflowID, *req.TriggerEventID, req.JobType)
		}
		return CoordinationJob{}, err
	}
	return coordinationJobFromRecord(row)
}

func (r *PgRepository) FinishCoordinationJob(ctx context.Context, req FinishCoordinationJobRequest) (CoordinationJob, error) {
	outputEventIDs, err := jsonbArray(req.OutputEventIDs, "output_event_ids")
	if err != nil {
		return CoordinationJob{}, err
	}
	row, err := r.q.FinishProjectCoordinationJob(ctx, queries.FinishProjectCoordinationJobParams{
		TenantID:       req.TenantID,
		ID:             req.ID,
		Status:         req.Status,
		OutputEventIds: outputEventIDs,
	})
	if err != nil {
		return CoordinationJob{}, err
	}
	return coordinationJobFromRecord(row)
}

func (r *PgRepository) ListCoordinationJobs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]CoordinationJob, error) {
	rows, err := r.q.ListProjectCoordinationJobs(ctx, queries.ListProjectCoordinationJobsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return coordinationJobsFromRecords(rows)
}

func (r *PgRepository) ListDemandLaunchCoordinationJobs(ctx context.Context, tenantID, projectID, demandID uuid.UUID, createdEventID *uuid.UUID, limit int32) ([]CoordinationJob, error) {
	rows, err := r.q.ListDemandLaunchCoordinationJobs(ctx, queries.ListDemandLaunchCoordinationJobsParams{
		TenantID:       tenantID,
		ProjectID:      projectID,
		CreatedEventID: nullUUID(createdEventID),
		DemandID:       demandID,
		Limit:          limit,
	})
	if err != nil {
		return nil, err
	}
	return coordinationJobsFromRecords(rows)
}

func (r *PgRepository) CreateRouteDecision(ctx context.Context, req CreateRouteDecisionRequest) (RouteDecision, error) {
	existing, err := r.GetRouteDecisionByCoordinationJob(ctx, req.TenantID, req.CoordinationJobID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrProjectNotFound) {
		return RouteDecision{}, err
	}
	candidateIDs, err := jsonbUUIDSlice(req.CandidateDigitalEmployeeIDs, "candidate_digital_employee_ids")
	if err != nil {
		return RouteDecision{}, err
	}
	selectedIDs, err := jsonbUUIDSlice(req.SelectedDigitalEmployeeIDs, "selected_digital_employee_ids")
	if err != nil {
		return RouteDecision{}, err
	}
	inputRequirements, err := jsonbObject(req.InputRequirements, "input_requirements")
	if err != nil {
		return RouteDecision{}, err
	}
	expectedOutputs, err := jsonbArray(req.ExpectedOutputs, "expected_outputs")
	if err != nil {
		return RouteDecision{}, err
	}
	budgetEstimate, err := jsonbObject(req.BudgetEstimate, "budget_estimate")
	if err != nil {
		return RouteDecision{}, err
	}
	row, err := r.q.CreateProjectRouteDecision(ctx, queries.CreateProjectRouteDecisionParams{
		TenantID:                    req.TenantID,
		ProjectID:                   req.ProjectID,
		CoordinationJobID:           req.CoordinationJobID,
		DemandID:                    nullUUID(req.DemandID),
		CandidateDigitalEmployeeIds: candidateIDs,
		SelectedDigitalEmployeeIds:  selectedIDs,
		Reason:                      req.Reason,
		InputRequirements:           inputRequirements,
		ExpectedOutputs:             expectedOutputs,
		BudgetEstimate:              budgetEstimate,
		RequiresHumanReview:         req.RequiresHumanReview,
		CreatedEventID:              nullUUID(req.CreatedEventID),
	})
	if err != nil {
		if isPGUniqueConstraint(err, "uq_project_route_decisions_job") {
			return r.GetRouteDecisionByCoordinationJob(ctx, req.TenantID, req.CoordinationJobID)
		}
		return RouteDecision{}, err
	}
	return routeDecisionFromRecord(row)
}

func (r *PgRepository) ListRouteDecisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]RouteDecision, error) {
	rows, err := r.q.ListProjectRouteDecisions(ctx, queries.ListProjectRouteDecisionsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return routeDecisionsFromRecords(rows)
}

func (r *PgRepository) ListDemandLaunchRouteDecisions(ctx context.Context, tenantID, projectID, demandID uuid.UUID, limit int32) ([]RouteDecision, error) {
	rows, err := r.q.ListDemandLaunchRouteDecisions(ctx, queries.ListDemandLaunchRouteDecisionsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		DemandID:  demandID,
		Limit:     limit,
	})
	if err != nil {
		return nil, err
	}
	return routeDecisionsFromRecords(rows)
}

func (r *PgRepository) CreateProjectTask(ctx context.Context, req CreateProjectTaskRequest) (ProjectTask, error) {
	return r.createProjectTaskWithQueries(ctx, r.q, req)
}

func (r *PgRepository) createProjectTaskAttemptWithQueries(ctx context.Context, q *queries.Queries, req QueueProjectTaskRequest, attemptID uuid.UUID, attemptNo int32, eventID *uuid.UUID) (ProjectTaskAttempt, error) {
	packet, err := jsonbObject(req.ExecutionContextPacket, "execution_context_packet")
	if err != nil {
		return ProjectTaskAttempt{}, err
	}
	version := strings.TrimSpace(req.ExecutionContextPacketVersion)
	if version == "" {
		version = "v1"
	}
	row, err := q.CreateProjectTaskAttempt(ctx, queries.CreateProjectTaskAttemptParams{
		ID:                            attemptID,
		TenantID:                      req.TenantID,
		ProjectTaskID:                 req.ProjectTaskID,
		AttemptNo:                     attemptNo,
		Status:                        ProjectTaskAttemptStatusQueued,
		DigitalEmployeeRunID:          nullUUID(req.DigitalEmployeeRunID),
		RuntimeTaskID:                 nullUUID(req.RuntimeTaskID),
		RuntimeNodeID:                 nullUUID(req.RuntimeNodeID),
		ExecutionContextPacket:        packet,
		ExecutionContextPacketVersion: version,
		LeaseToken:                    req.LeaseToken,
		LeaseExpiresAt:                timestamptzPtr(req.LeaseExpiresAt),
		IdempotencyKey:                req.IdempotencyKey,
		CreatedEventID:                nullUUID(eventID),
	})
	if err != nil {
		return ProjectTaskAttempt{}, err
	}
	return projectTaskAttemptFromRecord(row)
}

func (r *PgRepository) replayQueueProjectTaskAttemptWithQueries(ctx context.Context, q *queries.Queries, req QueueProjectTaskRequest) (QueueProjectTaskResult, bool, error) {
	row, err := q.GetProjectTaskAttemptByIdempotencyKey(ctx, queries.GetProjectTaskAttemptByIdempotencyKeyParams{
		TenantID:       req.TenantID,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return QueueProjectTaskResult{}, false, nil
		}
		return QueueProjectTaskResult{}, false, err
	}
	attempt, err := projectTaskAttemptFromRecord(row)
	if err != nil {
		return QueueProjectTaskResult{}, true, err
	}
	if attempt.ProjectTaskID != req.ProjectTaskID {
		return QueueProjectTaskResult{}, true, ErrProjectConflict
	}
	task, err := r.getProjectTaskWithQueries(ctx, q, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return QueueProjectTaskResult{}, true, err
	}
	if task.ProjectID != req.ProjectID {
		return QueueProjectTaskResult{}, true, ErrProjectNotFound
	}
	var event ProjectEvent
	if attempt.CreatedEventID != nil {
		eventRow, err := q.GetProjectEvent(ctx, queries.GetProjectEventParams{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			ID:        *attempt.CreatedEventID,
		})
		if err != nil {
			return QueueProjectTaskResult{}, true, err
		}
		event, err = eventFromRecord(eventRow)
		if err != nil {
			return QueueProjectTaskResult{}, true, err
		}
	}
	return QueueProjectTaskResult{Task: task, Attempt: attempt, Event: event}, true, nil
}

func (r *PgRepository) QueueProjectTaskWithAttempt(ctx context.Context, req QueueProjectTaskRequest) (QueueProjectTaskResult, error) {
	return withProjectQueries(ctx, r, "project task queue", func(q *queries.Queries) (QueueProjectTaskResult, error) {
		if result, replayed, err := r.replayQueueProjectTaskAttemptWithQueries(ctx, q, req); replayed || err != nil {
			return result, err
		}
		row, err := q.LockProjectTaskForQueue(ctx, queries.LockProjectTaskForQueueParams{
			TenantID:  req.TenantID,
			ProjectID: req.ProjectID,
			ID:        req.ProjectTaskID,
		})
		if err != nil {
			return QueueProjectTaskResult{}, projectRepositoryError(err)
		}
		task, err := taskFromRecord(row)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		if result, replayed, err := r.replayQueueProjectTaskAttemptWithQueries(ctx, q, req); replayed || err != nil {
			return result, err
		}
		if task.Status != ProjectTaskStatusPlanned && task.Status != ProjectTaskStatusWaitingHuman {
			return QueueProjectTaskResult{}, ErrProjectConflict
		}
		if task.AssignedDigitalEmployeeID != nil && *task.AssignedDigitalEmployeeID != req.DigitalEmployeeID {
			return QueueProjectTaskResult{}, ErrProjectTaskForbidden
		}
		if req.ExecutionContextPacket == nil {
			req.ExecutionContextPacket = map[string]any{}
		}
		if strings.TrimSpace(req.ExecutionContextPacketVersion) == "" {
			req.ExecutionContextPacketVersion = "v1"
		}
		attemptNo := task.AttemptCount + 1
		attemptID := uuid.New()

		event, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventTaskDispatched,
			ActorType:    "project_coordinator",
			ActorID:      req.ProjectTaskID.String(),
			ResourceType: strPtr("project_task"),
			ResourceID:   strPtr(req.ProjectTaskID.String()),
			Summary:      "项目任务已排队",
			Payload:      queueProjectTaskEventPayload(req, attemptID, attemptNo),
		})
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		attempt, err := r.createProjectTaskAttemptWithQueries(ctx, q, req, attemptID, attemptNo, &event.ID)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		queued, err := q.QueueProjectTask(ctx, queries.QueueProjectTaskParams{
			CurrentAttemptID:     attempt.ID,
			RuntimeTaskID:        nullUUID(req.RuntimeTaskID),
			DigitalEmployeeRunID: nullUUID(req.DigitalEmployeeRunID),
			LatestEventID:        uuid.NullUUID{UUID: event.ID, Valid: true},
			TenantID:             req.TenantID,
			ProjectID:            req.ProjectID,
			ID:                   req.ProjectTaskID,
		})
		if err != nil {
			return QueueProjectTaskResult{}, projectRepositoryError(err)
		}
		mappedTask, err := taskFromRecord(queued)
		if err != nil {
			return QueueProjectTaskResult{}, err
		}
		return QueueProjectTaskResult{Task: mappedTask, Attempt: attempt, Event: event}, nil
	})
}

func queueProjectTaskEventPayload(req QueueProjectTaskRequest, attemptID uuid.UUID, attemptNo int32) map[string]any {
	payload := map[string]any{
		"project_task_id":         req.ProjectTaskID.String(),
		"project_task_attempt_id": attemptID.String(),
		"project_task_status":     ProjectTaskStatusQueued,
		"digital_employee_id":     req.DigitalEmployeeID.String(),
		"attempt_no":              attemptNo,
		"idempotency_key":         req.IdempotencyKey,
		"lease_expires_at_set":    req.LeaseExpiresAt != nil,
	}
	if req.DigitalEmployeeRunID != nil {
		payload["digital_employee_run_id"] = req.DigitalEmployeeRunID.String()
	}
	if req.RuntimeTaskID != nil {
		payload["runtime_task_id"] = req.RuntimeTaskID.String()
	}
	if req.RuntimeNodeID != nil {
		payload["runtime_node_id"] = req.RuntimeNodeID.String()
	}
	return payload
}

func (r *PgRepository) GetProjectTaskAttempt(ctx context.Context, tenantID, attemptID uuid.UUID) (ProjectTaskAttempt, error) {
	row, err := r.q.GetProjectTaskAttempt(ctx, queries.GetProjectTaskAttemptParams{TenantID: tenantID, ID: attemptID})
	if err != nil {
		return ProjectTaskAttempt{}, projectRepositoryError(err)
	}
	return projectTaskAttemptFromRecord(row)
}

func (r *PgRepository) GetCurrentProjectTaskAttempt(ctx context.Context, tenantID, projectTaskID uuid.UUID) (ProjectTaskAttempt, error) {
	row, err := r.q.GetCurrentProjectTaskAttempt(ctx, queries.GetCurrentProjectTaskAttemptParams{TenantID: tenantID, ProjectTaskID: projectTaskID})
	if err != nil {
		return ProjectTaskAttempt{}, projectRepositoryError(err)
	}
	return projectTaskAttemptFromRecord(row)
}

func (r *PgRepository) DecomposeAcceptedPlanRevision(ctx context.Context, req DecomposeAcceptedPlanRevisionRequest) (DecomposeAcceptedPlanRevisionResult, error) {
	return DecomposeAcceptedPlanRevisionResult{}, ErrProjectTaskGraphPending
}

func (r *PgRepository) createProjectTaskWithQueries(ctx context.Context, q *queries.Queries, req CreateProjectTaskRequest) (ProjectTask, error) {
	expectedOutputs, err := jsonbArray(req.ExpectedOutputs, "expected_outputs")
	if err != nil {
		return ProjectTask{}, err
	}
	inputRequirements, err := jsonbObject(req.InputRequirements, "input_requirements")
	if err != nil {
		return ProjectTask{}, err
	}
	handoffContract, err := jsonbObject(req.HandoffContract, "handoff_contract")
	if err != nil {
		return ProjectTask{}, err
	}
	plannerMetadata, err := jsonbObject(req.PlannerMetadata, "planner_metadata")
	if err != nil {
		return ProjectTask{}, err
	}
	row, err := q.CreateProjectTask(ctx, queries.CreateProjectTaskParams{
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  nullUUID(req.DemandID),
		CoordinationJobID:         nullUUID(req.CoordinationJobID),
		RouteDecisionID:           nullUUID(req.RouteDecisionID),
		PlannedTaskKey:            textPtr(req.PlannedTaskKey),
		TaskKind:                  textPtr(req.TaskKind),
		StageIndex:                int4Ptr(req.StageIndex),
		Title:                     req.Title,
		Summary:                   textOrNull(req.Summary),
		Status:                    req.Status,
		AssignedDigitalEmployeeID: nullUUID(req.AssignedDigitalEmployeeID),
		RuntimeTaskID:             nullUUID(req.RuntimeTaskID),
		DigitalEmployeeRunID:      nullUUID(req.DigitalEmployeeRunID),
		RiskLevel:                 textOrNull(req.RiskLevel),
		RequiresHumanApproval:     req.RequiresHumanApproval,
		ExpectedOutputs:           expectedOutputs,
		InputRequirements:         inputRequirements,
		HandoffContract:           handoffContract,
		PlannerMetadata:           plannerMetadata,
	})
	if err != nil {
		return ProjectTask{}, err
	}
	return taskFromRecord(row)
}

func (r *PgRepository) CreateProjectTaskGraph(ctx context.Context, req CreateProjectTaskGraphRequest) (CreateProjectTaskGraphResult, error) {
	return withProjectQueries(ctx, r, "project task graph create", func(q *queries.Queries) (CreateProjectTaskGraphResult, error) {
		existing, err := r.listProjectTasksByCoordinationJobWithQueries(ctx, q, req.TenantID, req.ProjectID, req.CoordinationJobID)
		if err != nil {
			return CreateProjectTaskGraphResult{}, err
		}
		if len(existing) > 0 {
			complete, err := r.graphComplete(ctx, q, req, existing)
			if err != nil {
				return CreateProjectTaskGraphResult{}, err
			}
			if !complete {
				return CreateProjectTaskGraphResult{}, ErrProjectConflict
			}
			return r.graphResultFromExisting(ctx, q, req, existing)
		}

		keyToID := map[string]uuid.UUID{}
		created := make([]ProjectTaskGraphTaskResult, 0, len(req.Tasks))
		dependencies := make([]ProjectTaskDependency, 0)
		for _, planned := range req.Tasks {
			if planned.Key == "" {
				return CreateProjectTaskGraphResult{}, ErrInvalidProject
			}
			if _, exists := keyToID[planned.Key]; exists {
				return CreateProjectTaskGraphResult{}, ErrInvalidProject
			}
			employeeID := planned.AssignedDigitalEmployeeID
			taskKind := planned.TaskKind
			task, err := r.createProjectTaskWithQueries(ctx, q, CreateProjectTaskRequest{
				TenantID:                  req.TenantID,
				ProjectID:                 req.ProjectID,
				DemandID:                  &req.DemandID,
				Title:                     planned.Title,
				Summary:                   planned.Summary,
				Status:                    planned.Status,
				AssignedDigitalEmployeeID: &employeeID,
				RiskLevel:                 planned.RiskLevel,
				RequiresHumanApproval:     planned.RequiresHumanApproval,
				CoordinationJobID:         &req.CoordinationJobID,
				RouteDecisionID:           &req.RouteDecisionID,
				PlannedTaskKey:            &planned.Key,
				TaskKind:                  &taskKind,
				StageIndex:                planned.StageIndex,
				ExpectedOutputs:           planned.ExpectedOutputs,
				InputRequirements:         planned.InputRequirements,
				HandoffContract:           planned.HandoffContract,
				PlannerMetadata:           planned.PlannerMetadata,
			})
			if err != nil {
				return CreateProjectTaskGraphResult{}, err
			}
			event, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
				TenantID:     req.TenantID,
				ProjectID:    req.ProjectID,
				EventType:    ProjectEventTaskCreated,
				ActorType:    "project_coordinator",
				ActorID:      task.ID.String(),
				ResourceType: strPtr("project_task"),
				ResourceID:   strPtr(task.ID.String()),
				Summary:      "项目任务已创建",
				Payload: map[string]any{
					"project_task_id":     task.ID.String(),
					"demand_id":           req.DemandID.String(),
					"coordination_job_id": req.CoordinationJobID.String(),
					"planned_task_key":    planned.Key,
				},
			})
			if err != nil {
				return CreateProjectTaskGraphResult{}, err
			}
			keyToID[planned.Key] = task.ID
			created = append(created, ProjectTaskGraphTaskResult{
				ID:             task.ID,
				PlannedTaskKey: planned.Key,
				StageIndex:     planned.StageIndex,
				CreatedEventID: event.ID,
				IsRoot:         len(planned.BlockedByKeys) == 0,
			})
		}
		for _, planned := range req.Tasks {
			dependentTaskID, ok := keyToID[planned.Key]
			if !ok {
				return CreateProjectTaskGraphResult{}, ErrProjectConflict
			}
			for _, blockerKey := range planned.BlockedByKeys {
				blockerTaskID, ok := keyToID[blockerKey]
				if !ok {
					return CreateProjectTaskGraphResult{}, ErrProjectConflict
				}
				edge, err := q.CreateProjectTaskDependency(ctx, queries.CreateProjectTaskDependencyParams{
					TenantID:          req.TenantID,
					ProjectID:         req.ProjectID,
					CoordinationJobID: nullUUID(&req.CoordinationJobID),
					DependentTaskID:   dependentTaskID,
					BlockerTaskID:     blockerTaskID,
				})
				if err != nil {
					return CreateProjectTaskGraphResult{}, err
				}
				dependencies = append(dependencies, dependencyFromRecord(edge))
			}
		}
		graphEvent, err := r.appendProjectEventWithQueries(ctx, q, AppendProjectEventRequest{
			TenantID:     req.TenantID,
			ProjectID:    req.ProjectID,
			EventType:    ProjectEventTaskGraphPlanned,
			ActorType:    "project_coordinator",
			ActorID:      req.CoordinationJobID.String(),
			ResourceType: strPtr("project_coordination_job"),
			ResourceID:   strPtr(req.CoordinationJobID.String()),
			Summary:      "项目任务图已规划",
			Payload: map[string]any{
				"coordination_job_id": req.CoordinationJobID.String(),
				"route_decision_id":   req.RouteDecisionID.String(),
				"task_count":          len(req.Tasks),
				"dependency_count":    len(dependencies),
			},
		})
		if err != nil {
			return CreateProjectTaskGraphResult{}, err
		}
		if err := r.advanceProjectDemandStatusWithQueries(ctx, q, req.TenantID, req.ProjectID, req.DemandID, ProjectDemandStatusPlanned); err != nil {
			return CreateProjectTaskGraphResult{}, err
		}
		return CreateProjectTaskGraphResult{Tasks: created, Dependencies: dependencies, GraphEventID: graphEvent.ID}, nil
	})
}

func (r *PgRepository) CreateProjectTaskDependency(ctx context.Context, req CreateProjectTaskDependencyRequest) (ProjectTaskDependency, error) {
	row, err := r.q.CreateProjectTaskDependency(ctx, queries.CreateProjectTaskDependencyParams{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		CoordinationJobID: nullUUID(req.CoordinationJobID),
		DependentTaskID:   req.DependentTaskID,
		BlockerTaskID:     req.BlockerTaskID,
	})
	if err != nil {
		return ProjectTaskDependency{}, err
	}
	return dependencyFromRecord(row), nil
}

func (r *PgRepository) RewireProjectTaskDependencies(ctx context.Context, req RewireProjectTaskDependenciesRequest) ([]ProjectTaskDependency, error) {
	if len(req.DependentTaskIDs) == 0 {
		return []ProjectTaskDependency{}, nil
	}
	rows, err := r.q.RewireProjectTaskDependencies(ctx, queries.RewireProjectTaskDependenciesParams{
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		OldBlockerTaskID: req.OldBlockerTaskID,
		DependentTaskIds: req.DependentTaskIDs,
		NewBlockerTaskID: req.NewBlockerTaskID,
	})
	if err != nil {
		return nil, err
	}
	dependencies := make([]ProjectTaskDependency, 0, len(rows))
	for _, row := range rows {
		dependencies = append(dependencies, dependencyFromRewireRow(row))
	}
	return dependencies, nil
}

func (r *PgRepository) ListProjectTaskDependencies(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependency, error) {
	rows, err := r.q.ListProjectTaskDependencies(ctx, queries.ListProjectTaskDependenciesParams{
		TenantID:         tenantID,
		ProjectID:        projectID,
		DependentTaskIds: dependentTaskIDs,
	})
	if err != nil {
		return nil, err
	}
	return dependenciesFromRecords(rows), nil
}

func (r *PgRepository) ListDependentsOfTask(ctx context.Context, tenantID, projectID, blockerTaskID uuid.UUID) ([]uuid.UUID, error) {
	return r.q.ListDependentsOfTask(ctx, queries.ListDependentsOfTaskParams{
		TenantID:      tenantID,
		ProjectID:     projectID,
		BlockerTaskID: blockerTaskID,
	})
}

func (r *PgRepository) ListUnresolvedBlockersForTasks(ctx context.Context, tenantID, projectID uuid.UUID, dependentTaskIDs []uuid.UUID) ([]ProjectTaskDependencyReadiness, error) {
	rows, err := r.q.ListUnresolvedBlockersForTasks(ctx, queries.ListUnresolvedBlockersForTasksParams{
		TenantID:         tenantID,
		ProjectID:        projectID,
		DependentTaskIds: dependentTaskIDs,
	})
	if err != nil {
		return nil, err
	}
	readiness := make([]ProjectTaskDependencyReadiness, 0, len(rows))
	for _, row := range rows {
		readiness = append(readiness, ProjectTaskDependencyReadiness{
			DependentTaskID: row.DependentTaskID,
			BlockerTaskID:   row.BlockerTaskID,
			BlockerStatus:   row.BlockerStatus,
		})
	}
	return readiness, nil
}

func (r *PgRepository) ListProjectTasksByCoordinationJob(ctx context.Context, tenantID, projectID, coordinationJobID uuid.UUID) ([]ProjectTask, error) {
	return r.listProjectTasksByCoordinationJobWithQueries(ctx, r.q, tenantID, projectID, coordinationJobID)
}

func (r *PgRepository) listProjectTasksByCoordinationJobWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID, coordinationJobID uuid.UUID) ([]ProjectTask, error) {
	rows, err := q.ListProjectTasksByCoordinationJob(ctx, queries.ListProjectTasksByCoordinationJobParams{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: coordinationJobID,
	})
	if err != nil {
		return nil, err
	}
	return tasksFromRecords(rows)
}

func (r *PgRepository) graphComplete(ctx context.Context, q *queries.Queries, req CreateProjectTaskGraphRequest, existing []ProjectTask) (bool, error) {
	if len(existing) != len(req.Tasks) {
		return false, nil
	}
	existingByKey, existingIDs, ok := existingGraphTasksByKey(req, existing)
	if !ok {
		return false, nil
	}
	for _, planned := range req.Tasks {
		if !graphTaskPayloadMatchesRequest(req, planned, existingByKey[planned.Key]) {
			return false, nil
		}
	}
	dependencyRows, err := q.ListProjectTaskDependencies(ctx, queries.ListProjectTaskDependenciesParams{
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		DependentTaskIds: existingIDs,
	})
	if err != nil {
		return false, err
	}
	if !dependencyRowsMatchRequest(req, existingByKey, dependenciesFromRecords(dependencyRows)) {
		return false, nil
	}
	taskEventIDs, graphEventID, err := r.existingGraphEventIDs(ctx, q, req, existing)
	if err != nil {
		return false, err
	}
	return graphEventID != uuid.Nil && len(taskEventIDs) == len(existing), nil
}

func (r *PgRepository) graphResultFromExisting(ctx context.Context, q *queries.Queries, req CreateProjectTaskGraphRequest, existing []ProjectTask) (CreateProjectTaskGraphResult, error) {
	dependentIDs := graphProjectTaskIDs(existing)
	dependencyRows, err := q.ListProjectTaskDependencies(ctx, queries.ListProjectTaskDependenciesParams{
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		DependentTaskIds: dependentIDs,
	})
	if err != nil {
		return CreateProjectTaskGraphResult{}, err
	}
	dependencies := dependenciesFromRecords(dependencyRows)
	taskEventIDs, graphEventID, err := r.existingGraphEventIDs(ctx, q, req, existing)
	if err != nil {
		return CreateProjectTaskGraphResult{}, err
	}
	if graphEventID == uuid.Nil {
		return CreateProjectTaskGraphResult{}, ErrProjectConflict
	}
	blockedTaskIDs := map[uuid.UUID]struct{}{}
	for _, dependency := range dependencies {
		blockedTaskIDs[dependency.DependentTaskID] = struct{}{}
	}
	results := make([]ProjectTaskGraphTaskResult, 0, len(existing))
	for _, task := range existing {
		if task.PlannedTaskKey == nil {
			return CreateProjectTaskGraphResult{}, ErrProjectConflict
		}
		eventID := taskEventIDs[task.ID]
		if eventID == uuid.Nil {
			return CreateProjectTaskGraphResult{}, ErrProjectConflict
		}
		_, blocked := blockedTaskIDs[task.ID]
		results = append(results, ProjectTaskGraphTaskResult{
			ID:             task.ID,
			PlannedTaskKey: *task.PlannedTaskKey,
			StageIndex:     task.StageIndex,
			CreatedEventID: eventID,
			IsRoot:         !blocked,
		})
	}
	return CreateProjectTaskGraphResult{Tasks: results, Dependencies: dependencies, GraphEventID: graphEventID}, nil
}

func existingGraphTasksByKey(req CreateProjectTaskGraphRequest, existing []ProjectTask) (map[string]ProjectTask, []uuid.UUID, bool) {
	existingByKey := make(map[string]ProjectTask, len(existing))
	existingIDs := make([]uuid.UUID, 0, len(existing))
	for _, task := range existing {
		if task.TenantID != req.TenantID || task.ProjectID != req.ProjectID {
			return nil, nil, false
		}
		if task.DemandID == nil || *task.DemandID != req.DemandID ||
			task.CoordinationJobID == nil || *task.CoordinationJobID != req.CoordinationJobID ||
			task.RouteDecisionID == nil || *task.RouteDecisionID != req.RouteDecisionID ||
			task.PlannedTaskKey == nil || *task.PlannedTaskKey == "" {
			return nil, nil, false
		}
		if _, exists := existingByKey[*task.PlannedTaskKey]; exists {
			return nil, nil, false
		}
		existingByKey[*task.PlannedTaskKey] = task
		existingIDs = append(existingIDs, task.ID)
	}
	for _, planned := range req.Tasks {
		if _, exists := existingByKey[planned.Key]; !exists {
			return nil, nil, false
		}
	}
	return existingByKey, existingIDs, true
}

func graphTaskPayloadMatchesRequest(req CreateProjectTaskGraphRequest, planned ProjectTaskGraphCreateTask, existing ProjectTask) bool {
	if existing.TenantID != req.TenantID || existing.ProjectID != req.ProjectID {
		return false
	}
	if existing.DemandID == nil || *existing.DemandID != req.DemandID ||
		existing.CoordinationJobID == nil || *existing.CoordinationJobID != req.CoordinationJobID ||
		existing.RouteDecisionID == nil || *existing.RouteDecisionID != req.RouteDecisionID {
		return false
	}
	if existing.PlannedTaskKey == nil || *existing.PlannedTaskKey != planned.Key {
		return false
	}
	if existing.Title != planned.Title ||
		!storedTextMatches(existing.Summary, planned.Summary) ||
		existing.Status != planned.Status ||
		!storedUUIDMatches(existing.AssignedDigitalEmployeeID, planned.AssignedDigitalEmployeeID) ||
		!storedTextMatches(existing.TaskKind, planned.TaskKind) ||
		!storedInt32Matches(existing.StageIndex, planned.StageIndex) ||
		!storedTextMatches(existing.RiskLevel, planned.RiskLevel) ||
		existing.RequiresHumanApproval != planned.RequiresHumanApproval {
		return false
	}
	return storedJSONPayloadMatches(existing.ExpectedOutputs, plannedExpectedOutputs(planned.ExpectedOutputs)) &&
		storedJSONPayloadMatches(existing.InputRequirements, plannedObjectPayload(planned.InputRequirements)) &&
		storedJSONPayloadMatches(existing.HandoffContract, plannedObjectPayload(planned.HandoffContract)) &&
		storedJSONPayloadMatches(existing.PlannerMetadata, plannedObjectPayload(planned.PlannerMetadata))
}

func storedTextMatches(existing *string, planned string) bool {
	if planned == "" {
		return existing == nil
	}
	return existing != nil && *existing == planned
}

func storedUUIDMatches(existing *uuid.UUID, planned uuid.UUID) bool {
	if planned == uuid.Nil {
		return existing == nil
	}
	return existing != nil && *existing == planned
}

func storedInt32Matches(existing, planned *int32) bool {
	if existing == nil || planned == nil {
		return existing == nil && planned == nil
	}
	return *existing == *planned
}

func plannedExpectedOutputs(values []any) []any {
	if values == nil {
		return []any{}
	}
	return values
}

func plannedObjectPayload(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

func storedJSONPayloadMatches(existing, planned any) bool {
	existingJSON, existingErr := json.Marshal(existing)
	plannedJSON, plannedErr := json.Marshal(planned)
	return existingErr == nil && plannedErr == nil && bytes.Equal(existingJSON, plannedJSON)
}

func dependencyRowsMatchRequest(req CreateProjectTaskGraphRequest, existingByKey map[string]ProjectTask, dependencies []ProjectTaskDependency) bool {
	expectedEdges := map[string]struct{}{}
	for _, planned := range req.Tasks {
		dependent, ok := existingByKey[planned.Key]
		if !ok {
			return false
		}
		for _, blockerKey := range planned.BlockedByKeys {
			blocker, ok := existingByKey[blockerKey]
			if !ok {
				return false
			}
			expectedEdges[dependencyKey(dependent.ID, blocker.ID)] = struct{}{}
		}
	}
	if len(expectedEdges) != len(dependencies) {
		return false
	}
	for _, dependency := range dependencies {
		if _, ok := expectedEdges[dependencyKey(dependency.DependentTaskID, dependency.BlockerTaskID)]; !ok {
			return false
		}
	}
	return true
}

func dependencyKey(dependentTaskID, blockerTaskID uuid.UUID) string {
	return dependentTaskID.String() + ":" + blockerTaskID.String()
}

func graphProjectTaskIDs(tasks []ProjectTask) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func (r *PgRepository) existingGraphEventIDs(ctx context.Context, q *queries.Queries, req CreateProjectTaskGraphRequest, existing []ProjectTask) (map[uuid.UUID]uuid.UUID, uuid.UUID, error) {
	taskIDs := graphProjectTaskIDs(existing)
	rows, err := q.ListProjectTaskGraphReplayEvents(ctx, queries.ListProjectTaskGraphReplayEventsParams{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		EventTypes:        []string{string(ProjectEventTaskCreated), string(ProjectEventTaskGraphPlanned)},
		CoordinationJobID: req.CoordinationJobID.String(),
		ProjectTaskIds:    uuidStrings(taskIDs),
	})
	if err != nil {
		return nil, uuid.Nil, err
	}
	events, err := eventsFromRecords(rows)
	if err != nil {
		return nil, uuid.Nil, err
	}
	neededTasks := map[string]uuid.UUID{}
	for _, task := range existing {
		neededTasks[task.ID.String()] = task.ID
	}
	taskEventIDs := map[uuid.UUID]uuid.UUID{}
	graphEventID := uuid.Nil
	for _, event := range events {
		switch event.EventType {
		case ProjectEventTaskCreated:
			taskID, ok := graphTaskCreatedEventTaskID(event, neededTasks)
			if ok && taskEventIDs[taskID] == uuid.Nil {
				taskEventIDs[taskID] = event.ID
			}
		case ProjectEventTaskGraphPlanned:
			if graphEventID != uuid.Nil {
				continue
			}
			if event.ActorID == req.CoordinationJobID.String() {
				graphEventID = event.ID
				continue
			}
			if payloadJobID, ok := event.Payload["coordination_job_id"].(string); ok && payloadJobID == req.CoordinationJobID.String() {
				graphEventID = event.ID
			}
		}
	}
	return taskEventIDs, graphEventID, nil
}

func graphTaskCreatedEventTaskID(event ProjectEvent, neededTasks map[string]uuid.UUID) (uuid.UUID, bool) {
	if taskID, ok := neededTasks[event.ActorID]; ok {
		return taskID, true
	}
	payloadTaskID, ok := event.Payload["project_task_id"].(string)
	if !ok {
		return uuid.Nil, false
	}
	taskID, ok := neededTasks[payloadTaskID]
	return taskID, ok
}

func (r *PgRepository) GetProjectTaskCompletionContract(ctx context.Context, tenantID, taskID uuid.UUID) (ProjectTaskCompletionContract, error) {
	row, err := r.q.GetProjectTaskCompletionContract(ctx, queries.GetProjectTaskCompletionContractParams{TenantID: tenantID, ID: taskID})
	if err != nil {
		return ProjectTaskCompletionContract{}, projectRepositoryError(err)
	}
	return completionContractFromRecord(row)
}

func (r *PgRepository) GetCoordinationJobByTrigger(ctx context.Context, tenantID uuid.UUID, workflowID string, triggerEventID uuid.UUID, jobType string) (CoordinationJob, error) {
	row, err := r.q.GetProjectCoordinationJobByTrigger(ctx, queries.GetProjectCoordinationJobByTriggerParams{
		TenantID:       tenantID,
		WorkflowID:     workflowID,
		TriggerEventID: triggerEventID,
		JobType:        jobType,
	})
	if err != nil {
		return CoordinationJob{}, projectRepositoryError(err)
	}
	return coordinationJobFromRecord(row)
}

func (r *PgRepository) GetRouteDecisionByCoordinationJob(ctx context.Context, tenantID, coordinationJobID uuid.UUID) (RouteDecision, error) {
	row, err := r.q.GetProjectRouteDecisionByCoordinationJob(ctx, queries.GetProjectRouteDecisionByCoordinationJobParams{
		TenantID:          tenantID,
		CoordinationJobID: coordinationJobID,
	})
	if err != nil {
		return RouteDecision{}, projectRepositoryError(err)
	}
	return routeDecisionFromRecord(row)
}

func (r *PgRepository) GetProjectTaskGraph(ctx context.Context, req GetProjectTaskGraphRequest) (ProjectTaskGraph, error) {
	graph := emptyProjectTaskGraph()
	tasks, err := r.projectTaskGraphTasks(ctx, req)
	if err != nil {
		return graph, err
	}
	if len(tasks) == 0 {
		return graph, nil
	}
	graph.Nodes = taskGraphNodes(tasks)
	taskIDs := graphProjectTaskIDs(tasks)
	dependencies, err := r.ListProjectTaskDependencies(ctx, req.TenantID, req.ProjectID, taskIDs)
	if err != nil {
		return graph, err
	}
	graph.Edges, err = r.projectTaskGraphEdges(ctx, req, tasks, dependencies)
	if err != nil {
		return graph, err
	}
	graph.Employees, err = r.projectTaskGraphEmployees(ctx, req.TenantID, req.ProjectID, tasks)
	if err != nil {
		return graph, err
	}
	graph.Runs, err = r.projectTaskGraphRuns(ctx, req.TenantID, tasks)
	if err != nil {
		return graph, err
	}
	graph.ExecutionSummaries, err = r.listProjectExecutionSummariesByTaskIDs(ctx, req.TenantID, req.ProjectID, taskIDs)
	if err != nil {
		return graph, err
	}
	jobIDs := graphCoordinationJobIDs(req.CoordinationJobID, tasks)
	graph.DecisionRequests, err = r.listProjectTaskGraphDecisionRequests(ctx, req.TenantID, req.ProjectID, jobIDs, taskIDs)
	if err != nil {
		return graph, err
	}
	graph.Nodes = enrichProjectTaskGraphNodes(graph.Nodes, graph.DecisionRequests)
	graph.StageSummaries = buildProjectTaskGraphStageSummaries(graph.Nodes)
	graph.RecentEvents, err = r.projectTaskGraphEvents(ctx, req, jobIDs, taskIDs, decisionRequestIDs(graph.DecisionRequests))
	if err != nil {
		return graph, err
	}
	return graph, nil
}

func (r *PgRepository) projectTaskGraphTasks(ctx context.Context, req GetProjectTaskGraphRequest) ([]ProjectTask, error) {
	var (
		tasks []ProjectTask
		err   error
	)
	switch {
	case req.CoordinationJobID != nil:
		tasks, err = r.ListProjectTasksByCoordinationJob(ctx, req.TenantID, req.ProjectID, *req.CoordinationJobID)
	case req.DemandID != nil:
		tasks, err = r.listProjectTasksByDemand(ctx, req.TenantID, req.ProjectID, *req.DemandID)
	default:
		err = ErrInvalidProject
	}
	if err != nil {
		return nil, err
	}
	if req.DemandID != nil && req.CoordinationJobID != nil {
		tasks = filterTasksForDemand(tasks, *req.DemandID)
	}
	return tasks, nil
}

func (r *PgRepository) projectTaskGraphEdges(ctx context.Context, req GetProjectTaskGraphRequest, tasks []ProjectTask, dependencies []ProjectTaskDependency) ([]ProjectTaskGraphEdge, error) {
	statusByTaskID := map[uuid.UUID]string{}
	for _, task := range tasks {
		statusByTaskID[task.ID] = task.Status
	}
	edges := make([]ProjectTaskGraphEdge, 0, len(dependencies))
	for _, dependency := range dependencies {
		status, ok := statusByTaskID[dependency.BlockerTaskID]
		if !ok {
			blocker, err := r.GetProjectTask(ctx, req.TenantID, dependency.BlockerTaskID)
			if err != nil {
				return nil, err
			}
			if blocker.ProjectID != req.ProjectID {
				return nil, ErrProjectNotFound
			}
			status = blocker.Status
			statusByTaskID[dependency.BlockerTaskID] = status
		}
		edges = append(edges, ProjectTaskGraphEdge{
			DependentTaskID:   dependency.DependentTaskID,
			BlockerTaskID:     dependency.BlockerTaskID,
			CoordinationJobID: dependency.CoordinationJobID,
			EdgeStatus:        status,
		})
	}
	return edges, nil
}

func (r *PgRepository) projectTaskGraphEmployees(ctx context.Context, tenantID, projectID uuid.UUID, tasks []ProjectTask) ([]ProjectTaskGraphEmployee, error) {
	assignedIDs := map[uuid.UUID]struct{}{}
	for _, task := range tasks {
		if task.AssignedDigitalEmployeeID != nil {
			assignedIDs[*task.AssignedDigitalEmployeeID] = struct{}{}
		}
	}
	if len(assignedIDs) == 0 {
		return []ProjectTaskGraphEmployee{}, nil
	}
	members, err := r.ListProjectMembers(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	employees := make([]ProjectTaskGraphEmployee, 0, len(assignedIDs))
	seen := map[uuid.UUID]struct{}{}
	for _, member := range members {
		if member.PrincipalType != PrincipalTypeDigitalEmployee {
			continue
		}
		if _, needed := assignedIDs[member.PrincipalID]; !needed {
			continue
		}
		if _, exists := seen[member.PrincipalID]; exists {
			continue
		}
		displayName := ""
		if member.DisplayNameSnapshot != nil {
			displayName = *member.DisplayNameSnapshot
		}
		employees = append(employees, ProjectTaskGraphEmployee{
			DigitalEmployeeID: member.PrincipalID,
			DisplayName:       displayName,
			ProjectRole:       member.ProjectRole,
			Status:            member.Status,
		})
		seen[member.PrincipalID] = struct{}{}
	}
	return employees, nil
}

func (r *PgRepository) projectTaskGraphRuns(ctx context.Context, tenantID uuid.UUID, tasks []ProjectTask) ([]ProjectTaskGraphRun, error) {
	runIDs := graphDigitalEmployeeRunIDs(tasks)
	if len(runIDs) == 0 {
		return []ProjectTaskGraphRun{}, nil
	}
	rows, err := r.q.ListTaskRunsByIDs(ctx, queries.ListTaskRunsByIDsParams{
		Ids:      runIDs,
		TenantID: nullUUID(&tenantID),
	})
	if err != nil {
		return nil, err
	}
	return projectTaskGraphRunsFromRows(tasks, rows)
}

func (r *PgRepository) projectTaskGraphEvents(ctx context.Context, req GetProjectTaskGraphRequest, jobIDs, taskIDs, decisionIDs []uuid.UUID) ([]ProjectEvent, error) {
	if len(jobIDs) == 0 && len(taskIDs) == 0 && len(decisionIDs) == 0 {
		return []ProjectEvent{}, nil
	}
	rows, err := r.q.ListProjectTaskGraphEvents(ctx, queries.ListProjectTaskGraphEventsParams{
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		CoordinationJobIds: uuidStrings(jobIDs),
		ProjectTaskIds:     uuidStrings(taskIDs),
		DecisionRequestIds: uuidStrings(decisionIDs),
		Limit:              100,
	})
	if err != nil {
		return nil, err
	}
	return eventsFromRecords(rows)
}

func emptyProjectTaskGraph() ProjectTaskGraph {
	return ProjectTaskGraph{
		Nodes:              []ProjectTaskGraphNode{},
		Edges:              []ProjectTaskGraphEdge{},
		Employees:          []ProjectTaskGraphEmployee{},
		Runs:               []ProjectTaskGraphRun{},
		ExecutionSummaries: []ExecutionSummary{},
		RecentEvents:       []ProjectEvent{},
		DecisionRequests:   []DecisionRequest{},
	}
}

func taskGraphNodes(tasks []ProjectTask) []ProjectTaskGraphNode {
	nodes := make([]ProjectTaskGraphNode, 0, len(tasks))
	for _, task := range tasks {
		updatedAt := task.UpdatedAt
		nodes = append(nodes, ProjectTaskGraphNode{
			Task:           task,
			StatusReason:   projectTaskStatusReason(task.Status),
			UpdatedAt:      &updatedAt,
			CurrentBlocker: projectTaskCurrentBlocker(task),
		})
	}
	return nodes
}

func enrichProjectTaskGraphNodes(nodes []ProjectTaskGraphNode, decisions []DecisionRequest) []ProjectTaskGraphNode {
	latestDecisionByTaskID := map[uuid.UUID]DecisionRequest{}
	for _, decision := range decisions {
		if decision.ProjectTaskID == nil || !isPendingDecisionStatus(decision.StatusSnapshot) {
			continue
		}
		current, ok := latestDecisionByTaskID[*decision.ProjectTaskID]
		if !ok || decision.UpdatedAt.After(current.UpdatedAt) {
			latestDecisionByTaskID[*decision.ProjectTaskID] = decision
		}
	}
	for index, node := range nodes {
		decision, ok := latestDecisionByTaskID[node.Task.ID]
		if !ok {
			continue
		}
		nodes[index].StatusReason = "等待人工决策"
		nodes[index].CurrentBlocker = &WorkflowInstanceCurrentBlocker{
			Type:       "decision_request",
			Title:      stringValueOr(decision.TitleSnapshot, "等待人工决策"),
			ResourceID: &decision.ID,
		}
	}
	return nodes
}

func projectTaskStatusReason(status string) string {
	switch strings.ToLower(status) {
	case "failed":
		return "任务失败"
	case "blocked":
		return "任务受阻"
	case "waiting_human", "pending_review":
		return "等待人工决策"
	case "assigned", "running", "in_progress":
		return "任务执行中"
	case "planned", "pending":
		return "任务待执行"
	case "completed", "done", "success":
		return "任务已完成"
	case "cancelled":
		return "任务已取消"
	default:
		return ""
	}
}

func projectTaskCurrentBlocker(task ProjectTask) *WorkflowInstanceCurrentBlocker {
	switch strings.ToLower(task.Status) {
	case "failed", "blocked":
		taskID := task.ID
		return &WorkflowInstanceCurrentBlocker{
			Type:       "project_task",
			Title:      task.Title,
			ResourceID: &taskID,
		}
	default:
		return nil
	}
}

func isPendingDecisionStatus(status string) bool {
	switch strings.ToLower(status) {
	case "pending", "requested":
		return true
	default:
		return false
	}
}

func graphCoordinationJobIDs(filter *uuid.UUID, tasks []ProjectTask) []uuid.UUID {
	ids := make([]uuid.UUID, 0)
	seen := map[uuid.UUID]struct{}{}
	if filter != nil {
		ids = append(ids, *filter)
		seen[*filter] = struct{}{}
	}
	for _, task := range tasks {
		if task.CoordinationJobID == nil {
			continue
		}
		if _, exists := seen[*task.CoordinationJobID]; exists {
			continue
		}
		ids = append(ids, *task.CoordinationJobID)
		seen[*task.CoordinationJobID] = struct{}{}
	}
	return ids
}

func graphDigitalEmployeeRunIDs(tasks []ProjectTask) []uuid.UUID {
	ids := make([]uuid.UUID, 0)
	seen := map[uuid.UUID]struct{}{}
	for _, task := range tasks {
		if task.DigitalEmployeeRunID == nil {
			continue
		}
		if _, exists := seen[*task.DigitalEmployeeRunID]; exists {
			continue
		}
		ids = append(ids, *task.DigitalEmployeeRunID)
		seen[*task.DigitalEmployeeRunID] = struct{}{}
	}
	return ids
}

func projectTaskGraphRunsFromRows(tasks []ProjectTask, rows []queries.TaskRun) ([]ProjectTaskGraphRun, error) {
	rowsByID := make(map[uuid.UUID]queries.TaskRun, len(rows))
	for _, row := range rows {
		rowsByID[row.ID] = row
	}
	runs := make([]ProjectTaskGraphRun, 0, len(rows))
	for _, task := range tasks {
		if task.DigitalEmployeeRunID == nil {
			continue
		}
		row, ok := rowsByID[*task.DigitalEmployeeRunID]
		if !ok {
			return nil, ErrProjectNotFound
		}
		runtimeTaskID := task.RuntimeTaskID
		if runtimeTaskID == nil {
			id := row.TaskID
			runtimeTaskID = &id
		}
		runs = append(runs, ProjectTaskGraphRun{
			ProjectTaskID:        task.ID,
			DigitalEmployeeRunID: task.DigitalEmployeeRunID,
			RuntimeTaskID:        runtimeTaskID,
			RuntimeNodeID:        ptrUUID(row.RuntimeNodeID),
			RuntimeNodeSummary:   row.NodeID,
			Status:               row.Status,
			ProviderType:         textValue(row.ProviderType),
		})
	}
	return runs, nil
}

func filterEventsForTaskGraph(events []ProjectEvent, coordinationJobID *uuid.UUID, taskIDs, decisionIDs []uuid.UUID) []ProjectEvent {
	taskSet := uuidStringSet(taskIDs)
	decisionSet := uuidStringSet(decisionIDs)
	var coordinationJob string
	if coordinationJobID != nil {
		coordinationJob = coordinationJobID.String()
	}
	filtered := make([]ProjectEvent, 0)
	for _, event := range events {
		if coordinationJob != "" && event.ActorID == coordinationJob {
			filtered = append(filtered, event)
			continue
		}
		if _, ok := taskSet[event.ActorID]; ok {
			filtered = append(filtered, event)
			continue
		}
		if event.ResourceID != nil {
			if _, ok := taskSet[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
			if _, ok := decisionSet[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
		}
		if rawTaskID, ok := event.Payload["project_task_id"].(string); ok {
			if _, exists := taskSet[rawTaskID]; exists {
				filtered = append(filtered, event)
				continue
			}
		}
		if rawDecisionID, ok := event.Payload["decision_request_id"].(string); ok {
			if _, exists := decisionSet[rawDecisionID]; exists {
				filtered = append(filtered, event)
				continue
			}
		}
		if coordinationJob != "" {
			if rawJobID, ok := event.Payload["coordination_job_id"].(string); ok && rawJobID == coordinationJob {
				filtered = append(filtered, event)
			}
		}
	}
	return filtered
}

func uuidStringSet(ids []uuid.UUID) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		set[id.String()] = struct{}{}
	}
	return set
}

func (r *PgRepository) UpdateProjectTaskStatus(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error) {
	return r.updateProjectTaskStatusWithQueries(ctx, r.q, tenantID, projectTaskID, status, eventID, currentStatuses)
}

func (r *PgRepository) BindProjectTaskRun(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error) {
	row, err := r.q.BindProjectTaskRun(ctx, queries.BindProjectTaskRunParams{
		TenantID:             req.TenantID,
		ID:                   req.ProjectTaskID,
		RuntimeTaskID:        req.RuntimeTaskID,
		DigitalEmployeeRunID: req.DigitalEmployeeRunID,
		LatestEventID:        nullUUID(req.LatestEventID),
		CurrentStatuses:      req.CurrentStatuses,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return r.bindProjectTaskRunConflict(ctx, req)
		}
		return ProjectTask{}, err
	}
	return taskFromRecord(row)
}

// bindProjectTaskRunConflict distinguishes a missing task from a real binding
// conflict (task is bound to a different run, or is in a non-dispatchable state).
func (r *PgRepository) bindProjectTaskRunConflict(ctx context.Context, req BindProjectTaskRunRequest) (ProjectTask, error) {
	existing, err := r.q.GetProjectTask(ctx, queries.GetProjectTaskParams{TenantID: req.TenantID, ID: req.ProjectTaskID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProjectTask{}, ErrProjectNotFound
		}
		return ProjectTask{}, err
	}
	task, err := taskFromRecord(existing)
	if err != nil {
		return ProjectTask{}, err
	}
	if task.DigitalEmployeeRunID != nil && *task.DigitalEmployeeRunID == req.DigitalEmployeeRunID &&
		task.RuntimeTaskID != nil && *task.RuntimeTaskID == req.RuntimeTaskID {
		// Already bound to the same run and runtime task by a prior attempt; treat as idempotent success.
		return task, nil
	}
	return ProjectTask{}, ErrProjectConflict
}

func (r *PgRepository) ProjectTaskEventExists(ctx context.Context, tenantID, projectID uuid.UUID, eventType ProjectEventType, actorID string) (bool, error) {
	return r.q.ProjectTaskEventExists(ctx, queries.ProjectTaskEventExistsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: string(eventType),
		ActorID:   actorID,
	})
}

func (r *PgRepository) updateProjectTaskStatusWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectTaskID uuid.UUID, status string, eventID *uuid.UUID, currentStatuses []string) (ProjectTask, error) {
	row, err := q.UpdateProjectTaskStatus(ctx, queries.UpdateProjectTaskStatusParams{
		TenantID:        tenantID,
		ID:              projectTaskID,
		Status:          status,
		LatestEventID:   nullUUID(eventID),
		CurrentStatuses: currentStatuses,
	})
	if err != nil {
		return ProjectTask{}, err
	}
	return taskFromRecord(row)
}

func (r *PgRepository) AssignProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID, status string, assignedDigitalEmployeeID, eventID *uuid.UUID) (ProjectTask, error) {
	row, err := r.q.AssignProjectTask(ctx, queries.AssignProjectTaskParams{
		TenantID:                  tenantID,
		ID:                        projectTaskID,
		Status:                    status,
		AssignedDigitalEmployeeID: nullUUID(assignedDigitalEmployeeID),
		LatestEventID:             nullUUID(eventID),
	})
	if err != nil {
		return ProjectTask{}, err
	}
	return taskFromRecord(row)
}

func (r *PgRepository) CreateExecutionSummary(ctx context.Context, req CreateExecutionSummaryRequest) (ExecutionSummary, error) {
	return r.createExecutionSummaryWithQueries(ctx, r.q, req)
}

func (r *PgRepository) createExecutionSummaryWithQueries(ctx context.Context, q *queries.Queries, req CreateExecutionSummaryRequest) (ExecutionSummary, error) {
	evidenceRefs, err := jsonbArray(req.EvidenceRefs, "evidence_refs")
	if err != nil {
		return ExecutionSummary{}, err
	}
	artifactRefs, err := jsonbArray(req.ArtifactRefs, "artifact_refs")
	if err != nil {
		return ExecutionSummary{}, err
	}
	confidenceFactors, err := jsonbObject(req.ConfidenceFactors, "confidence_factors")
	if err != nil {
		return ExecutionSummary{}, err
	}
	missingInformation, err := jsonbArray(req.MissingInformation, "missing_information")
	if err != nil {
		return ExecutionSummary{}, err
	}
	row, err := q.CreateProjectExecutionSummary(ctx, queries.CreateProjectExecutionSummaryParams{
		TenantID:              req.TenantID,
		ProjectID:             req.ProjectID,
		ProjectTaskID:         req.ProjectTaskID,
		DigitalEmployeeID:     req.DigitalEmployeeID,
		Conclusion:            req.Conclusion,
		EvidenceRefs:          evidenceRefs,
		ArtifactRefs:          artifactRefs,
		ConfidenceFactors:     confidenceFactors,
		Uncertainty:           textOrNull(req.Uncertainty),
		MissingInformation:    missingInformation,
		RecommendedNextAction: textOrNull(req.RecommendedNextAction),
		RequiresHumanReview:   req.RequiresHumanReview,
		TransferRequestID:     nullUUID(req.TransferRequestID),
		CreatedEventID:        nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return ExecutionSummary{}, err
	}
	return executionSummaryFromRecord(row)
}

func (r *PgRepository) ListExecutionSummaries(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ExecutionSummary, error) {
	rows, err := r.q.ListProjectExecutionSummaries(ctx, queries.ListProjectExecutionSummariesParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return executionSummariesFromRecords(rows)
}

func (r *PgRepository) listProjectExecutionSummariesByTaskIDs(ctx context.Context, tenantID, projectID uuid.UUID, taskIDs []uuid.UUID) ([]ExecutionSummary, error) {
	if len(taskIDs) == 0 {
		return []ExecutionSummary{}, nil
	}
	rows, err := r.q.ListProjectExecutionSummariesByTaskIDs(ctx, queries.ListProjectExecutionSummariesByTaskIDsParams{
		TenantID:       tenantID,
		ProjectID:      projectID,
		ProjectTaskIds: taskIDs,
	})
	if err != nil {
		return nil, err
	}
	return executionSummariesFromRecords(rows)
}

func (r *PgRepository) CreateTransferRequest(ctx context.Context, req CreateTransferRequestRequest) (TransferRequest, error) {
	return r.createTransferRequestWithQueries(ctx, r.q, req)
}

func (r *PgRepository) CompleteProjectTaskWriteback(ctx context.Context, req CompleteProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	return withProjectQueries(ctx, r, "project task completion writeback", func(q *queries.Queries) (ProjectTaskWritebackResult, error) {
		if _, err := r.updateProjectTaskStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ID, "completed", nil, req.AllowedCurrentStatuses); err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		summaryReq := req.Summary
		summaryReq.CreatedEventID = &event.ID
		summary, err := r.createExecutionSummaryWithQueries(ctx, q, summaryReq)
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		task, err := r.updateProjectTaskStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ID, "completed", &event.ID, []string{"completed"})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		if req.Task.DemandID != nil {
			if err := r.recomputeProjectDemandStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ProjectID, *req.Task.DemandID); err != nil {
				return ProjectTaskWritebackResult{}, err
			}
		}
		return ProjectTaskWritebackResult{Task: task, Event: event, Summary: summary}, nil
	})
}

func (r *PgRepository) FailProjectTaskWriteback(ctx context.Context, req FailProjectTaskWritebackRequest) (ProjectTaskWritebackResult, error) {
	return withProjectQueries(ctx, r, "project task failure writeback", func(q *queries.Queries) (ProjectTaskWritebackResult, error) {
		if _, err := r.updateProjectTaskStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ID, "failed", nil, req.AllowedCurrentStatuses); err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		task, err := r.updateProjectTaskStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ID, "failed", &event.ID, []string{"failed"})
		if err != nil {
			return ProjectTaskWritebackResult{}, err
		}
		if req.Task.DemandID != nil {
			if err := r.recomputeProjectDemandStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ProjectID, *req.Task.DemandID); err != nil {
				return ProjectTaskWritebackResult{}, err
			}
		}
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	})
}

// advanceProjectDemandStatusWithQueries moves a demand forward in its lifecycle,
// guarded so status never regresses. It shares the per-project advisory lock with
// project event appends so concurrent task writebacks serialize their demand updates.
func (r *PgRepository) advanceProjectDemandStatusWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID, demandID uuid.UUID, target ProjectDemandStatus) error {
	if demandID == uuid.Nil {
		return nil
	}
	if err := q.LockProjectEventSequence(ctx, queries.LockProjectEventSequenceParams{
		TenantID:  tenantID,
		ProjectID: projectID,
	}); err != nil {
		return err
	}
	current, err := q.GetProjectDemand(ctx, queries.GetProjectDemandParams{TenantID: tenantID, ID: demandID})
	if err != nil {
		return err
	}
	if !ProjectDemandStatusCanAdvance(ProjectDemandStatus(current.Status), target) {
		return nil
	}
	_, err = q.UpdateProjectDemandStatus(ctx, queries.UpdateProjectDemandStatusParams{
		Status:   string(target),
		TenantID: tenantID,
		ID:       demandID,
	})
	return err
}

// recomputeProjectDemandStatusWithQueries derives a demand's lifecycle status from
// its project tasks: completed when all tasks finished cleanly, failed when all
// terminal with at least one failure, otherwise executing while work remains.
func (r *PgRepository) recomputeProjectDemandStatusWithQueries(ctx context.Context, q *queries.Queries, tenantID, projectID, demandID uuid.UUID) error {
	if demandID == uuid.Nil {
		return nil
	}
	counts, err := q.CountProjectTaskStatusesByDemand(ctx, queries.CountProjectTaskStatusesByDemandParams{
		TenantID: tenantID,
		DemandID: demandID,
	})
	if err != nil {
		return err
	}
	if counts.Total == 0 {
		return nil
	}
	target := ProjectDemandStatusExecuting
	if counts.Active == 0 {
		if counts.Failed > 0 {
			target = ProjectDemandStatusFailed
		} else {
			target = ProjectDemandStatusCompleted
		}
	}
	return r.advanceProjectDemandStatusWithQueries(ctx, q, tenantID, projectID, demandID, target)
}

// AdvanceProjectDemandStatus advances a demand's lifecycle status from outside a
// shared writeback transaction (e.g. at dispatch time). Forward-only and idempotent.
func (r *PgRepository) AdvanceProjectDemandStatus(ctx context.Context, tenantID, projectID, demandID uuid.UUID, target ProjectDemandStatus) error {
	_, err := withProjectQueries(ctx, r, "advance project demand status", func(q *queries.Queries) (struct{}, error) {
		return struct{}{}, r.advanceProjectDemandStatusWithQueries(ctx, q, tenantID, projectID, demandID, target)
	})
	return err
}

func (r *PgRepository) RequestProjectTaskTransferWriteback(ctx context.Context, req RequestProjectTaskTransferWritebackRequest) (ProjectTaskTransferWritebackResult, error) {
	return withProjectQueries(ctx, r, "project task transfer writeback", func(q *queries.Queries) (ProjectTaskTransferWritebackResult, error) {
		if _, err := r.updateProjectTaskStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ID, "waiting_human", nil, req.AllowedCurrentStatuses); err != nil {
			return ProjectTaskTransferWritebackResult{}, err
		}
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectTaskTransferWritebackResult{}, err
		}
		transferReq := req.Transfer
		transferReq.CreatedEventID = &event.ID
		transfer, err := r.createTransferRequestWithQueries(ctx, q, transferReq)
		if err != nil {
			return ProjectTaskTransferWritebackResult{}, err
		}
		task, err := r.updateProjectTaskStatusWithQueries(ctx, q, req.Task.TenantID, req.Task.ID, "waiting_human", &event.ID, []string{"waiting_human"})
		if err != nil {
			return ProjectTaskTransferWritebackResult{}, err
		}
		return ProjectTaskTransferWritebackResult{Task: task, Event: event, Transfer: transfer}, nil
	})
}

func (r *PgRepository) createTransferRequestWithQueries(ctx context.Context, q *queries.Queries, req CreateTransferRequestRequest) (TransferRequest, error) {
	suggestedIDs, err := jsonbUUIDSlice(req.SuggestedDigitalEmployeeIDs, "suggested_digital_employee_ids")
	if err != nil {
		return TransferRequest{}, err
	}
	missingContextRefs, err := jsonbArray(req.MissingContextRefs, "missing_context_refs")
	if err != nil {
		return TransferRequest{}, err
	}
	row, err := q.CreateProjectTransferRequest(ctx, queries.CreateProjectTransferRequestParams{
		TenantID:                     req.TenantID,
		ProjectID:                    req.ProjectID,
		ProjectTaskID:                req.ProjectTaskID,
		RequestedByDigitalEmployeeID: req.RequestedByDigitalEmployeeID,
		Reason:                       req.Reason,
		SuggestedEmployeeType:        textOrNull(req.SuggestedEmployeeType),
		SuggestedDigitalEmployeeIds:  suggestedIDs,
		MissingContextRefs:           missingContextRefs,
		Status:                       req.Status,
		CreatedEventID:               nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return TransferRequest{}, err
	}
	return transferRequestFromRecord(row)
}

func (r *PgRepository) ListTransferRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]TransferRequest, error) {
	rows, err := r.q.ListProjectTransferRequests(ctx, queries.ListProjectTransferRequestsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return transferRequestsFromRecords(rows)
}

func (r *PgRepository) CreateDecisionRequest(ctx context.Context, req CreateDecisionRequestRequest) (DecisionRequest, error) {
	row, err := r.q.CreateProjectDecisionRequest(ctx, queries.CreateProjectDecisionRequestParams{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		ApprovalRequestID: req.ApprovalRequestID,
		CoordinationJobID: nullUUID(req.CoordinationJobID),
		ProjectTaskID:     nullUUID(req.ProjectTaskID),
		TargetUserID:      req.TargetUserID,
		DecisionType:      req.DecisionType,
		TitleSnapshot:     req.TitleSnapshot,
		SummarySnapshot:   textOrNull(req.SummarySnapshot),
		RiskLevelSnapshot: textOrNull(req.RiskLevelSnapshot),
		StatusSnapshot:    req.StatusSnapshot,
		CreatedEventID:    nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return DecisionRequest{}, err
	}
	return decisionRequestFromRecord(row)
}

func (r *PgRepository) GetDecisionRequest(ctx context.Context, tenantID, projectID, decisionRequestID uuid.UUID) (DecisionRequest, error) {
	row, err := r.q.GetProjectDecisionRequest(ctx, queries.GetProjectDecisionRequestParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		ID:        decisionRequestID,
	})
	if err != nil {
		return DecisionRequest{}, err
	}
	return decisionRequestFromRecord(row)
}

func (r *PgRepository) ResolveDecisionRequest(ctx context.Context, req ResolveDecisionRequestRepositoryRequest) (DecisionRequest, error) {
	row, err := r.q.ResolveProjectDecisionRequest(ctx, queries.ResolveProjectDecisionRequestParams{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ID:              req.ID,
		StatusSnapshot:  req.StatusSnapshot,
		ResolvedEventID: nullUUID(req.ResolvedEventID),
	})
	if err != nil {
		return DecisionRequest{}, err
	}
	return decisionRequestFromRecord(row)
}

func (r *PgRepository) ListDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]DecisionRequest, error) {
	rows, err := r.q.ListProjectDecisionRequests(ctx, queries.ListProjectDecisionRequestsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return decisionRequestsFromRecords(rows)
}

func (r *PgRepository) ListDemandLaunchDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, coordinationJobIDs, projectTaskIDs []uuid.UUID, limit int32) ([]DecisionRequest, error) {
	rows, err := r.q.ListDemandLaunchDecisionRequests(ctx, queries.ListDemandLaunchDecisionRequestsParams{
		TenantID:           tenantID,
		ProjectID:          projectID,
		CoordinationJobIds: coordinationJobIDs,
		ProjectTaskIds:     projectTaskIDs,
		Limit:              limit,
	})
	if err != nil {
		return nil, err
	}
	return decisionRequestsFromRecords(rows)
}

func (r *PgRepository) listProjectTaskGraphDecisionRequests(ctx context.Context, tenantID, projectID uuid.UUID, coordinationJobIDs, projectTaskIDs []uuid.UUID) ([]DecisionRequest, error) {
	if len(coordinationJobIDs) == 0 && len(projectTaskIDs) == 0 {
		return []DecisionRequest{}, nil
	}
	rows, err := r.q.ListProjectTaskGraphDecisionRequests(ctx, queries.ListProjectTaskGraphDecisionRequestsParams{
		TenantID:           tenantID,
		ProjectID:          projectID,
		CoordinationJobIds: coordinationJobIDs,
		ProjectTaskIds:     projectTaskIDs,
	})
	if err != nil {
		return nil, err
	}
	return decisionRequestsFromRecords(rows)
}

func (r *PgRepository) CreateEvidenceRef(ctx context.Context, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error) {
	return r.createEvidenceRefWithQueries(ctx, r.q, req)
}

func (r *PgRepository) CreateEvidenceRefWithEvent(ctx context.Context, req CreateEvidenceRefWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	return withProjectQueries(ctx, r, "project evidence ref write", func(q *queries.Queries) (ProjectEvidenceRefWriteResult, error) {
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectEvidenceRefWriteResult{}, err
		}
		evidenceReq := req.Evidence
		evidenceReq.CreatedEventID = &event.ID
		evidence, err := r.createEvidenceRefWithQueries(ctx, q, evidenceReq)
		if err != nil {
			return ProjectEvidenceRefWriteResult{}, err
		}
		return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
	})
}

func (r *PgRepository) createEvidenceRefWithQueries(ctx context.Context, q *queries.Queries, req CreateEvidenceRefRequest) (ProjectEvidenceRef, error) {
	metadata, err := jsonbObject(req.Metadata, "metadata")
	if err != nil {
		return ProjectEvidenceRef{}, err
	}
	row, err := q.CreateProjectEvidenceRef(ctx, queries.CreateProjectEvidenceRefParams{
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ProjectTaskID:      nullUUID(req.ProjectTaskID),
		RouteDecisionID:    nullUUID(req.RouteDecisionID),
		ExecutionSummaryID: nullUUID(req.ExecutionSummaryID),
		EvidenceType:       req.EvidenceType,
		Title:              req.Title,
		Summary:            textOrNull(req.Summary),
		SourceType:         req.SourceType,
		SourceRef:          req.SourceRef,
		ArtifactRefID:      nullUUID(req.ArtifactRefID),
		SubmittedByType:    req.SubmittedByType,
		SubmittedByID:      nullUUID(req.SubmittedByID),
		VerificationStatus: string(req.VerificationStatus),
		Metadata:           metadata,
		CreatedEventID:     nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return ProjectEvidenceRef{}, err
	}
	return evidenceRefFromRecord(row)
}

func (r *PgRepository) ListEvidenceRefs(ctx context.Context, tenantID, projectID uuid.UUID, status *EvidenceVerificationStatus, limit, offset int32) ([]ProjectEvidenceRef, error) {
	rows, err := r.q.ListProjectEvidenceRefs(ctx, queries.ListProjectEvidenceRefsParams{
		TenantID:           tenantID,
		ProjectID:          projectID,
		VerificationStatus: evidenceVerificationStatusPtr(status),
		Limit:              limit,
		Offset:             offset,
	})
	if err != nil {
		return nil, err
	}
	return evidenceRefsFromRecords(rows)
}

func (r *PgRepository) UpdateEvidenceVerificationStatus(ctx context.Context, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error) {
	return r.updateEvidenceVerificationStatusWithQueries(ctx, r.q, req)
}

func (r *PgRepository) UpdateEvidenceVerificationStatusWithEvent(ctx context.Context, req UpdateEvidenceVerificationStatusWithEventRequest) (ProjectEvidenceRefWriteResult, error) {
	return withProjectQueries(ctx, r, "project evidence verification write", func(q *queries.Queries) (ProjectEvidenceRefWriteResult, error) {
		evidence, err := r.updateEvidenceVerificationStatusWithQueries(ctx, q, req.Evidence)
		if err != nil {
			return ProjectEvidenceRefWriteResult{}, err
		}
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectEvidenceRefWriteResult{}, err
		}
		return ProjectEvidenceRefWriteResult{Event: event, Evidence: evidence}, nil
	})
}

func (r *PgRepository) updateEvidenceVerificationStatusWithQueries(ctx context.Context, q *queries.Queries, req UpdateEvidenceVerificationStatusRequest) (ProjectEvidenceRef, error) {
	metadata, err := jsonbObjectOrNull(req.Metadata, "metadata")
	if err != nil {
		return ProjectEvidenceRef{}, err
	}
	row, err := q.UpdateProjectEvidenceVerificationStatus(ctx, queries.UpdateProjectEvidenceVerificationStatusParams{
		VerificationStatus: string(req.VerificationStatus),
		Metadata:           metadata,
		TenantID:           req.TenantID,
		ProjectID:          req.ProjectID,
		ID:                 req.ID,
	})
	if err != nil {
		return ProjectEvidenceRef{}, projectRepositoryError(err)
	}
	return evidenceRefFromRecord(row)
}

func (r *PgRepository) CreateArtifactRef(ctx context.Context, req CreateArtifactRefRequest) (ProjectArtifactRef, error) {
	metadata, err := jsonbObject(req.Metadata, "metadata")
	if err != nil {
		return ProjectArtifactRef{}, err
	}
	row, err := r.q.CreateProjectArtifactRef(ctx, queries.CreateProjectArtifactRefParams{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ProjectTaskID:   nullUUID(req.ProjectTaskID),
		ArtifactID:      nullUUID(req.ArtifactID),
		ArtifactType:    req.ArtifactType,
		Title:           req.Title,
		ObjectRef:       req.ObjectRef,
		ContentType:     textOrNull(req.ContentType),
		SizeBytes:       int8Ptr(req.SizeBytes),
		Checksum:        textOrNull(req.Checksum),
		RetentionStatus: textOrNull(req.RetentionStatus),
		RetentionHoldID: nullUUID(req.RetentionHoldID),
		Metadata:        metadata,
		CreatedEventID:  nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return ProjectArtifactRef{}, err
	}
	return artifactRefFromRecord(row)
}

func (r *PgRepository) ListArtifactRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArtifactRef, error) {
	rows, err := r.q.ListProjectArtifactRefs(ctx, queries.ListProjectArtifactRefsParams{
		TenantID:        tenantID,
		ProjectID:       projectID,
		ArtifactType:    pgtype.Text{},
		RetentionStatus: pgtype.Text{},
		Limit:           limit,
		Offset:          offset,
	})
	if err != nil {
		return nil, err
	}
	return artifactRefsFromRecords(rows)
}

func (r *PgRepository) UpdateArtifactRetention(ctx context.Context, req UpdateArtifactRetentionRequest) (ProjectArtifactRef, error) {
	row, err := r.q.UpdateProjectArtifactRetention(ctx, queries.UpdateProjectArtifactRetentionParams{
		RetentionStatus: req.RetentionStatus,
		RetentionHoldID: nullUUID(req.RetentionHoldID),
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ID:              req.ID,
	})
	if err != nil {
		return ProjectArtifactRef{}, err
	}
	return artifactRefFromRecord(row)
}

func (r *PgRepository) CreateReportRef(ctx context.Context, req CreateReportRefRequest) (ProjectReportRef, error) {
	row, err := r.q.CreateProjectReportRef(ctx, queries.CreateProjectReportRefParams{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		ReportType:      req.ReportType,
		Title:           req.Title,
		Summary:         textOrNull(req.Summary),
		ObjectRef:       req.ObjectRef,
		Format:          req.Format,
		GeneratedByType: req.GeneratedByType,
		GeneratedByID:   nullUUID(req.GeneratedByID),
		CreatedEventID:  nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return ProjectReportRef{}, err
	}
	return reportRefFromRecord(row)
}

func (r *PgRepository) ListReportRefs(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectReportRef, error) {
	rows, err := r.q.ListProjectReportRefs(ctx, queries.ListProjectReportRefsParams{
		TenantID:   tenantID,
		ProjectID:  projectID,
		ReportType: pgtype.Text{},
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		return nil, err
	}
	return reportRefsFromRecords(rows)
}

func (r *PgRepository) CreateBudgetLedgerEntry(ctx context.Context, req CreateBudgetLedgerEntryRequest) (ProjectBudgetLedgerEntry, error) {
	estimatedCost, err := numericFromDecimalString(req.EstimatedCost)
	if err != nil {
		return ProjectBudgetLedgerEntry{}, fmt.Errorf("estimated_cost: %w", err)
	}
	actualCost, err := numericFromDecimalString(req.ActualCost)
	if err != nil {
		return ProjectBudgetLedgerEntry{}, fmt.Errorf("actual_cost: %w", err)
	}
	row, err := r.q.CreateProjectBudgetLedgerEntry(ctx, queries.CreateProjectBudgetLedgerEntryParams{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		CoordinationJobID: nullUUID(req.CoordinationJobID),
		ProjectTaskID:     nullUUID(req.ProjectTaskID),
		DigitalEmployeeID: nullUUID(req.DigitalEmployeeID),
		CostType:          req.CostType,
		EstimatedTokens:   int8Ptr(req.EstimatedTokens),
		ActualTokens:      int8Ptr(req.ActualTokens),
		EstimatedCost:     estimatedCost,
		ActualCost:        actualCost,
		Source:            req.Source,
		Reason:            textOrNull(req.Reason),
		CreatedEventID:    nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return ProjectBudgetLedgerEntry{}, err
	}
	return budgetLedgerEntryFromRecord(row)
}

func (r *PgRepository) ListBudgetLedger(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectBudgetLedgerEntry, error) {
	rows, err := r.q.ListProjectBudgetLedger(ctx, queries.ListProjectBudgetLedgerParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		CostType:  pgtype.Text{},
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return budgetLedgerEntriesFromRecords(rows)
}

func (r *PgRepository) GetBudgetSummary(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectBudgetSummary, error) {
	row, err := r.q.GetProjectBudgetSummary(ctx, queries.GetProjectBudgetSummaryParams{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return ProjectBudgetSummary{}, err
	}
	return budgetSummaryFromRecord(row), nil
}

func (r *PgRepository) CreateAcceptanceRecord(ctx context.Context, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error) {
	return r.createAcceptanceRecordWithQueries(ctx, r.q, req)
}

func (r *PgRepository) CreateAcceptanceRecordWithEvent(ctx context.Context, req CreateAcceptanceRecordWithEventRequest) (ProjectAcceptanceRecordWriteResult, error) {
	return withProjectQueries(ctx, r, "project acceptance record write", func(q *queries.Queries) (ProjectAcceptanceRecordWriteResult, error) {
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectAcceptanceRecordWriteResult{}, err
		}
		acceptanceReq := req.Acceptance
		acceptanceReq.CreatedEventID = &event.ID
		acceptance, err := r.createAcceptanceRecordWithQueries(ctx, q, acceptanceReq)
		if err != nil {
			return ProjectAcceptanceRecordWriteResult{}, err
		}
		return ProjectAcceptanceRecordWriteResult{Event: event, Acceptance: acceptance}, nil
	})
}

func (r *PgRepository) createAcceptanceRecordWithQueries(ctx context.Context, q *queries.Queries, req CreateAcceptanceRecordRequest) (ProjectAcceptanceRecord, error) {
	evidenceRefIDs, err := jsonbUUIDSlice(req.EvidenceRefIDs, "evidence_ref_ids")
	if err != nil {
		return ProjectAcceptanceRecord{}, err
	}
	reportRefIDs, err := jsonbUUIDSlice(req.ReportRefIDs, "report_ref_ids")
	if err != nil {
		return ProjectAcceptanceRecord{}, err
	}
	unresolvedRisks, err := jsonbArray(req.UnresolvedRisks, "unresolved_risks")
	if err != nil {
		return ProjectAcceptanceRecord{}, err
	}
	row, err := q.CreateProjectAcceptanceRecord(ctx, queries.CreateProjectAcceptanceRecordParams{
		TenantID:         req.TenantID,
		ProjectID:        req.ProjectID,
		AcceptedByUserID: req.AcceptedByUserID,
		Status:           req.Status,
		Conclusion:       req.Conclusion,
		Summary:          textOrNull(req.Summary),
		EvidenceRefIds:   evidenceRefIDs,
		ReportRefIds:     reportRefIDs,
		UnresolvedRisks:  unresolvedRisks,
		CreatedEventID:   nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return ProjectAcceptanceRecord{}, err
	}
	return acceptanceRecordFromRecord(row)
}

func (r *PgRepository) GetLatestAcceptanceRecord(ctx context.Context, tenantID, projectID uuid.UUID) (ProjectAcceptanceRecord, error) {
	row, err := r.q.GetLatestProjectAcceptanceRecord(ctx, queries.GetLatestProjectAcceptanceRecordParams{TenantID: tenantID, ProjectID: projectID})
	if err != nil {
		return ProjectAcceptanceRecord{}, projectRepositoryError(err)
	}
	return acceptanceRecordFromRecord(row)
}

func (r *PgRepository) CreateArchiveSnapshot(ctx context.Context, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error) {
	return r.createArchiveSnapshotWithQueries(ctx, r.q, req)
}

func (r *PgRepository) CreateArchiveSnapshotWithEvent(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	return withProjectQueries(ctx, r, "project archive snapshot write", func(q *queries.Queries) (ProjectArchiveSnapshotWriteResult, error) {
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectArchiveSnapshotWriteResult{}, err
		}
		snapshotReq := req.Snapshot
		snapshotReq.CreatedEventID = &event.ID
		snapshot, err := r.createArchiveSnapshotWithQueries(ctx, q, snapshotReq)
		if err != nil {
			return ProjectArchiveSnapshotWriteResult{}, err
		}
		return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: snapshot}, nil
	})
}

func (r *PgRepository) CreateArchiveSnapshotWithEventAndArchiveProject(ctx context.Context, req CreateArchiveSnapshotWithEventRequest) (ProjectArchiveSnapshotWriteResult, error) {
	return withProjectQueries(ctx, r, "project archive finalization", func(q *queries.Queries) (ProjectArchiveSnapshotWriteResult, error) {
		event, err := r.appendProjectEventWithQueries(ctx, q, req.Event)
		if err != nil {
			return ProjectArchiveSnapshotWriteResult{}, err
		}
		snapshotReq := req.Snapshot
		snapshotReq.CreatedEventID = &event.ID
		snapshot, err := r.createArchiveSnapshotWithQueries(ctx, q, snapshotReq)
		if err != nil {
			return ProjectArchiveSnapshotWriteResult{}, err
		}
		if _, err := r.archiveProjectWithQueries(ctx, q, req.Snapshot.TenantID, req.Snapshot.ProjectID); err != nil {
			return ProjectArchiveSnapshotWriteResult{}, err
		}
		return ProjectArchiveSnapshotWriteResult{Event: event, Snapshot: snapshot}, nil
	})
}

func (r *PgRepository) createArchiveSnapshotWithQueries(ctx context.Context, q *queries.Queries, req CreateArchiveSnapshotRequest) (ProjectArchiveSnapshot, error) {
	includedCounts, err := jsonbObject(req.IncludedCounts, "included_counts")
	if err != nil {
		return ProjectArchiveSnapshot{}, err
	}
	retainedArtifactIDs, err := jsonbUUIDSlice(req.RetainedArtifactIDs, "retained_artifact_ids")
	if err != nil {
		return ProjectArchiveSnapshot{}, err
	}
	row, err := q.CreateProjectArchiveSnapshot(ctx, queries.CreateProjectArchiveSnapshotParams{
		TenantID:             req.TenantID,
		ProjectID:            req.ProjectID,
		SnapshotType:         req.SnapshotType,
		Status:               req.Status,
		ObjectRef:            textOrNull(req.ObjectRef),
		Summary:              textOrNull(req.Summary),
		IncludedCounts:       includedCounts,
		RetainedArtifactIds:  retainedArtifactIDs,
		RetentionLockEventID: nullUUID(req.RetentionLockEventID),
		CreatedByUserID:      req.CreatedByUserID,
		CreatedEventID:       nullUUID(req.CreatedEventID),
	})
	if err != nil {
		return ProjectArchiveSnapshot{}, err
	}
	return archiveSnapshotFromRecord(row)
}

func (r *PgRepository) ListArchiveSnapshots(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectArchiveSnapshot, error) {
	rows, err := r.q.ListProjectArchiveSnapshots(ctx, queries.ListProjectArchiveSnapshotsParams{
		TenantID:     tenantID,
		ProjectID:    projectID,
		SnapshotType: pgtype.Text{},
		Limit:        limit,
		Offset:       offset,
	})
	if err != nil {
		return nil, err
	}
	return archiveSnapshotsFromRecords(rows)
}

func (r *PgRepository) ListConfigRevisions(ctx context.Context, tenantID, projectID uuid.UUID, limit, offset int32) ([]ProjectConfigRevision, error) {
	rows, err := r.q.ListProjectConfigRevisions(ctx, queries.ListProjectConfigRevisionsParams{
		TenantID:  tenantID,
		ProjectID: projectID,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		return nil, err
	}
	return configRevisionsFromRecords(rows)
}

func (r *PgRepository) GetConfigRevision(ctx context.Context, tenantID, projectID, revisionID uuid.UUID) (ProjectConfigRevision, error) {
	row, err := r.q.GetProjectConfigRevision(ctx, queries.GetProjectConfigRevisionParams{TenantID: tenantID, ProjectID: projectID, ID: revisionID})
	if err != nil {
		return ProjectConfigRevision{}, projectRepositoryError(err)
	}
	return configRevisionFromRecord(row)
}

func projectRepositoryError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProjectNotFound
	}
	return err
}

func projectFromRecord(row queries.Project) (Project, error) {
	coordinationPolicy, err := mapFromJSON(row.CoordinationPolicy)
	if err != nil {
		return Project{}, fmt.Errorf("coordination_policy: %w", err)
	}
	approvalPolicy, err := mapFromJSON(row.ApprovalPolicy)
	if err != nil {
		return Project{}, fmt.Errorf("approval_policy: %w", err)
	}
	evidencePolicy, err := mapFromJSON(row.EvidencePolicy)
	if err != nil {
		return Project{}, fmt.Errorf("evidence_policy: %w", err)
	}
	return Project{
		ID:                     row.ID,
		TenantID:               row.TenantID,
		TeamID:                 ptrUUID(row.TeamID),
		Name:                   row.Name,
		Description:            ptrText(row.Description),
		Goal:                   textValue(row.Goal),
		Status:                 ProjectStatus(row.Status),
		HumanOwnerUserID:       row.HumanOwnerUserID,
		LeaderUserID:           ptrUUID(row.LeaderUserID),
		AcceptanceUserID:       ptrUUID(row.AcceptanceUserID),
		CoordinationWorkflowID: textValue(row.CoordinationWorkflowID),
		CoordinationStatus:     textValue(row.CoordinationStatus),
		CoordinationPolicy:     coordinationPolicy,
		ApprovalPolicy:         approvalPolicy,
		EvidencePolicy:         evidencePolicy,
		ArchivedAt:             ptrTime(row.ArchivedAt),
		CreatedAt:              row.CreatedAt.Time,
		UpdatedAt:              row.UpdatedAt.Time,
	}, nil
}

func memberFromRecord(row queries.ProjectMember) (ProjectMember, error) {
	settings, err := mapFromJSON(row.Settings)
	if err != nil {
		return ProjectMember{}, fmt.Errorf("settings: %w", err)
	}
	return ProjectMember{
		ID:                  row.ID,
		TenantID:            row.TenantID,
		ProjectID:           row.ProjectID,
		PrincipalType:       PrincipalType(row.PrincipalType),
		PrincipalID:         row.PrincipalID,
		ProjectRole:         ProjectRole(row.ProjectRole),
		DisplayNameSnapshot: ptrText(row.DisplayNameSnapshot),
		Status:              row.Status,
		Settings:            settings,
		CreatedAt:           row.CreatedAt.Time,
		UpdatedAt:           row.UpdatedAt.Time,
	}, nil
}

func taskFromRecord(row queries.ProjectTask) (ProjectTask, error) {
	expectedOutputs, err := contractArrayFromJSON(row.ExpectedOutputs, "expected_outputs")
	if err != nil {
		return ProjectTask{}, err
	}
	inputRequirements, err := contractObjectFromJSON(row.InputRequirements, "input_requirements")
	if err != nil {
		return ProjectTask{}, err
	}
	handoffContract, err := contractObjectFromJSON(row.HandoffContract, "handoff_contract")
	if err != nil {
		return ProjectTask{}, err
	}
	plannerMetadata, err := contractObjectFromJSON(row.PlannerMetadata, "planner_metadata")
	if err != nil {
		return ProjectTask{}, err
	}
	return ProjectTask{
		ID:                        row.ID,
		TenantID:                  row.TenantID,
		ProjectID:                 row.ProjectID,
		DemandID:                  ptrUUID(row.DemandID),
		Title:                     row.Title,
		Summary:                   ptrText(row.Summary),
		Status:                    row.Status,
		AssignedDigitalEmployeeID: ptrUUID(row.AssignedDigitalEmployeeID),
		RuntimeTaskID:             ptrUUID(row.RuntimeTaskID),
		DigitalEmployeeRunID:      ptrUUID(row.DigitalEmployeeRunID),
		RiskLevel:                 ptrText(row.RiskLevel),
		RequiresHumanApproval:     row.RequiresHumanApproval,
		CoordinationJobID:         ptrUUID(row.CoordinationJobID),
		RouteDecisionID:           ptrUUID(row.RouteDecisionID),
		PlannedTaskKey:            ptrText(row.PlannedTaskKey),
		TaskKind:                  ptrText(row.TaskKind),
		StageIndex:                int32PtrFromSQL(row.StageIndex),
		ExpectedOutputs:           expectedOutputs,
		InputRequirements:         inputRequirements,
		HandoffContract:           handoffContract,
		PlannerMetadata:           plannerMetadata,
		BlockedByTaskIDs:          []uuid.UUID{},
		CurrentAttemptID:          ptrUUID(row.CurrentAttemptID),
		AcceptedPlanRevisionID:    ptrUUID(row.AcceptedPlanRevisionID),
		DecompositionClaimKey:     ptrText(row.DecompositionClaimKey),
		AttemptCount:              row.AttemptCount,
		MaxAttempts:               int32PtrFromSQL(row.MaxAttempts),
		RetryNotBefore:            ptrTime(row.RetryNotBefore),
		WaitingReason:             ptrText(row.WaitingReason),
		WaitingRequestID:          ptrUUID(row.WaitingRequestID),
		TerminalReason:            ptrText(row.TerminalReason),
		TerminalEventID:           ptrUUID(row.TerminalEventID),
		CancelledBy:               ptrText(row.CancelledBy),
		FailedBy:                  ptrText(row.FailedBy),
		StatusChangedAt:           row.StatusChangedAt.Time,
		CreatedAt:                 row.CreatedAt.Time,
		UpdatedAt:                 row.UpdatedAt.Time,
	}, nil
}

func projectTaskAttemptFromRecord(row queries.ProjectTaskAttempt) (ProjectTaskAttempt, error) {
	packet, err := mapFromJSON(row.ExecutionContextPacket)
	if err != nil {
		return ProjectTaskAttempt{}, fmt.Errorf("execution_context_packet: %w", err)
	}
	return ProjectTaskAttempt{
		ID:                            row.ID,
		TenantID:                      row.TenantID,
		ProjectTaskID:                 row.ProjectTaskID,
		AttemptNo:                     row.AttemptNo,
		Status:                        row.Status,
		DigitalEmployeeRunID:          ptrUUID(row.DigitalEmployeeRunID),
		RuntimeTaskID:                 ptrUUID(row.RuntimeTaskID),
		RuntimeNodeID:                 ptrUUID(row.RuntimeNodeID),
		ProviderSessionID:             ptrText(row.ProviderSessionID),
		ExecutionContextPacket:        packet,
		ExecutionContextPacketVersion: row.ExecutionContextPacketVersion,
		LeaseToken:                    row.LeaseToken,
		LeaseExpiresAt:                ptrTime(row.LeaseExpiresAt),
		RenewedAt:                     ptrTime(row.RenewedAt),
		LostAt:                        ptrTime(row.LostAt),
		StartedAt:                     ptrTime(row.StartedAt),
		FinishedAt:                    ptrTime(row.FinishedAt),
		TimeoutAt:                     ptrTime(row.TimeoutAt),
		Retryable:                     ptrBool(row.Retryable),
		FailureFamily:                 ptrText(row.FailureFamily),
		FailureMessage:                ptrText(row.FailureMessage),
		IdempotencyKey:                row.IdempotencyKey,
		CreatedEventID:                ptrUUID(row.CreatedEventID),
		TerminalEventID:               ptrUUID(row.TerminalEventID),
		CreatedAt:                     row.CreatedAt.Time,
		UpdatedAt:                     row.UpdatedAt.Time,
	}, nil
}

func dependencyFromRecord(row queries.ProjectTaskDependency) ProjectTaskDependency {
	return ProjectTaskDependency{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ProjectID:         row.ProjectID,
		CoordinationJobID: ptrUUID(row.CoordinationJobID),
		DependentTaskID:   row.DependentTaskID,
		BlockerTaskID:     row.BlockerTaskID,
	}
}

func dependencyFromRewireRow(row queries.RewireProjectTaskDependenciesRow) ProjectTaskDependency {
	return ProjectTaskDependency{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ProjectID:         row.ProjectID,
		CoordinationJobID: ptrUUID(row.CoordinationJobID),
		DependentTaskID:   row.DependentTaskID,
		BlockerTaskID:     row.BlockerTaskID,
	}
}

func completionContractFromRecord(row queries.GetProjectTaskCompletionContractRow) (ProjectTaskCompletionContract, error) {
	expectedOutputs, err := contractArrayFromJSON(row.ExpectedOutputs, "expected_outputs")
	if err != nil {
		return ProjectTaskCompletionContract{}, err
	}
	handoffContract, err := contractObjectFromJSON(row.HandoffContract, "handoff_contract")
	if err != nil {
		return ProjectTaskCompletionContract{}, err
	}
	return ProjectTaskCompletionContract{
		ID:                   row.ID,
		TenantID:             row.TenantID,
		ProjectID:            row.ProjectID,
		ExpectedOutputs:      expectedOutputs,
		HandoffContract:      handoffContract,
		DigitalEmployeeRunID: ptrUUID(row.DigitalEmployeeRunID),
	}, nil
}

func eventFromRecord(row queries.ProjectEvent) (ProjectEvent, error) {
	payload, err := mapFromJSON(row.Payload)
	if err != nil {
		return ProjectEvent{}, fmt.Errorf("payload: %w", err)
	}
	return ProjectEvent{
		ID:             row.ID,
		TenantID:       row.TenantID,
		ProjectID:      row.ProjectID,
		SequenceNumber: row.SequenceNumber,
		EventType:      ProjectEventType(row.EventType),
		ActorType:      row.ActorType,
		ActorID:        row.ActorID,
		ResourceType:   ptrText(row.ResourceType),
		ResourceID:     ptrText(row.ResourceID),
		Summary:        ptrText(row.Summary),
		Payload:        payload,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func demandFromRecord(row queries.ProjectDemand) (ProjectDemand, error) {
	sourceRefs, err := mapFromJSON(row.SourceRefs)
	if err != nil {
		return ProjectDemand{}, fmt.Errorf("source_refs: %w", err)
	}
	preference := reviewerPreferenceFromSourceRefs(sourceRefs)
	attachments := []any{}
	if len(row.Attachments) > 0 {
		if err := json.Unmarshal(row.Attachments, &attachments); err != nil {
			return ProjectDemand{}, fmt.Errorf("attachments: %w", err)
		}
		if attachments == nil {
			attachments = []any{}
		}
	}
	return ProjectDemand{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		ProjectID:          row.ProjectID,
		SubmittedByUserID:  row.SubmittedByUserID,
		Title:              row.Title,
		Content:            ptrText(row.Content),
		SourceType:         DemandSourceType(row.SourceType),
		SourceRefs:         sourceRefs,
		Attachments:        attachments,
		ReviewerPreference: preference,
		Status:             ProjectDemandStatus(row.Status),
		CreatedEventID:     ptrUUID(row.CreatedEventID),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}, nil
}

func reviewerPreferenceFromSourceRefs(sourceRefs map[string]any) *ReviewerPreference {
	rawReviewer, ok := sourceRefs["reviewer_user_id"].(string)
	if !ok || rawReviewer == "" {
		return nil
	}
	reviewerID, err := uuid.Parse(rawReviewer)
	if err != nil {
		return nil
	}
	reason := ReviewerSelectionReason("")
	if rawReason, ok := sourceRefs["reviewer_selection_reason"].(string); ok {
		reason = ReviewerSelectionReason(rawReason)
	}
	role := ProjectRole("")
	if rawRole, ok := sourceRefs["reviewer_project_role"].(string); ok {
		role = ProjectRole(rawRole)
	}
	var displayName *string
	if rawDisplayName, ok := sourceRefs["reviewer_display_name"].(string); ok {
		displayName = &rawDisplayName
	}
	resolved, _ := sourceRefs["reviewer_resolved_from_rule"].(bool)
	return &ReviewerPreference{
		ReviewerUserID:   reviewerID,
		SelectionReason:  reason,
		DisplayName:      displayName,
		ProjectRole:      role,
		ResolvedFromRule: resolved,
	}
}

func configRevisionFromRecord(row queries.ProjectConfigRevision) (ProjectConfigRevision, error) {
	snapshot, err := mapFromJSON(row.ConfigSnapshot)
	if err != nil {
		return ProjectConfigRevision{}, fmt.Errorf("config_snapshot: %w", err)
	}
	changedSections, err := anySliceFromJSON(row.ChangedSections)
	if err != nil {
		return ProjectConfigRevision{}, fmt.Errorf("changed_sections: %w", err)
	}
	diffSummary, err := mapFromJSON(row.DiffSummary)
	if err != nil {
		return ProjectConfigRevision{}, fmt.Errorf("diff_summary: %w", err)
	}
	return ProjectConfigRevision{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		ProjectID:          row.ProjectID,
		RevisionNumber:     row.RevisionNumber,
		ConfigSnapshot:     snapshot,
		ChangeSummary:      ptrText(row.ChangeSummary),
		CreatedByUserID:    row.CreatedByUserID,
		CreatedEventID:     ptrUUID(row.CreatedEventID),
		CreatedAt:          row.CreatedAt.Time,
		ChangedSections:    changedSections,
		PreviousRevisionID: ptrUUID(row.PreviousRevisionID),
		PolicyFingerprint:  ptrText(row.PolicyFingerprint),
		DiffSummary:        diffSummary,
	}, nil
}

func coordinationJobFromRecord(row queries.ProjectCoordinationJob) (CoordinationJob, error) {
	inputSnapshotRef, err := mapFromJSON(row.InputSnapshotRef)
	if err != nil {
		return CoordinationJob{}, fmt.Errorf("input_snapshot_ref: %w", err)
	}
	outputEventIDs := []any{}
	if len(row.OutputEventIds) > 0 {
		if err := json.Unmarshal(row.OutputEventIds, &outputEventIDs); err != nil {
			return CoordinationJob{}, fmt.Errorf("output_event_ids: %w", err)
		}
		if outputEventIDs == nil {
			outputEventIDs = []any{}
		}
	}
	return CoordinationJob{
		ID:               row.ID,
		TenantID:         row.TenantID,
		ProjectID:        row.ProjectID,
		WorkflowID:       row.WorkflowID,
		TriggerEventID:   ptrUUID(row.TriggerEventID),
		JobType:          row.JobType,
		Status:           row.Status,
		InputSnapshotRef: inputSnapshotRef,
		OutputEventIDs:   outputEventIDs,
		StartedAt:        ptrTime(row.StartedAt),
		FinishedAt:       ptrTime(row.FinishedAt),
		CreatedAt:        row.CreatedAt.Time,
	}, nil
}

func routeDecisionFromRecord(row queries.ProjectRouteDecision) (RouteDecision, error) {
	candidateIDs, err := uuidSliceFromJSON(row.CandidateDigitalEmployeeIds)
	if err != nil {
		return RouteDecision{}, fmt.Errorf("candidate_digital_employee_ids: %w", err)
	}
	selectedIDs, err := uuidSliceFromJSON(row.SelectedDigitalEmployeeIds)
	if err != nil {
		return RouteDecision{}, fmt.Errorf("selected_digital_employee_ids: %w", err)
	}
	inputRequirements, err := mapFromJSON(row.InputRequirements)
	if err != nil {
		return RouteDecision{}, fmt.Errorf("input_requirements: %w", err)
	}
	expectedOutputs := []any{}
	if len(row.ExpectedOutputs) > 0 {
		if err := json.Unmarshal(row.ExpectedOutputs, &expectedOutputs); err != nil {
			return RouteDecision{}, fmt.Errorf("expected_outputs: %w", err)
		}
		if expectedOutputs == nil {
			expectedOutputs = []any{}
		}
	}
	budgetEstimate, err := mapFromJSON(row.BudgetEstimate)
	if err != nil {
		return RouteDecision{}, fmt.Errorf("budget_estimate: %w", err)
	}
	return RouteDecision{
		ID:                          row.ID,
		TenantID:                    row.TenantID,
		ProjectID:                   row.ProjectID,
		CoordinationJobID:           row.CoordinationJobID,
		DemandID:                    ptrUUID(row.DemandID),
		CandidateDigitalEmployeeIDs: candidateIDs,
		SelectedDigitalEmployeeIDs:  selectedIDs,
		Reason:                      row.Reason,
		InputRequirements:           inputRequirements,
		ExpectedOutputs:             expectedOutputs,
		BudgetEstimate:              budgetEstimate,
		RequiresHumanReview:         row.RequiresHumanReview,
		CreatedEventID:              ptrUUID(row.CreatedEventID),
		CreatedAt:                   row.CreatedAt.Time,
	}, nil
}

func executionSummaryFromRecord(row queries.ProjectExecutionSummary) (ExecutionSummary, error) {
	evidenceRefs, err := anySliceFromJSON(row.EvidenceRefs)
	if err != nil {
		return ExecutionSummary{}, fmt.Errorf("evidence_refs: %w", err)
	}
	artifactRefs, err := anySliceFromJSON(row.ArtifactRefs)
	if err != nil {
		return ExecutionSummary{}, fmt.Errorf("artifact_refs: %w", err)
	}
	confidenceFactors, err := mapFromJSON(row.ConfidenceFactors)
	if err != nil {
		return ExecutionSummary{}, fmt.Errorf("confidence_factors: %w", err)
	}
	missingInformation, err := anySliceFromJSON(row.MissingInformation)
	if err != nil {
		return ExecutionSummary{}, fmt.Errorf("missing_information: %w", err)
	}
	return ExecutionSummary{
		ID:                    row.ID,
		TenantID:              row.TenantID,
		ProjectID:             row.ProjectID,
		ProjectTaskID:         row.ProjectTaskID,
		DigitalEmployeeID:     row.DigitalEmployeeID,
		Conclusion:            row.Conclusion,
		EvidenceRefs:          evidenceRefs,
		ArtifactRefs:          artifactRefs,
		ConfidenceFactors:     confidenceFactors,
		Uncertainty:           ptrText(row.Uncertainty),
		MissingInformation:    missingInformation,
		RecommendedNextAction: ptrText(row.RecommendedNextAction),
		RequiresHumanReview:   row.RequiresHumanReview,
		TransferRequestID:     ptrUUID(row.TransferRequestID),
		CreatedEventID:        ptrUUID(row.CreatedEventID),
		CreatedAt:             row.CreatedAt.Time,
	}, nil
}

func transferRequestFromRecord(row queries.ProjectTransferRequest) (TransferRequest, error) {
	suggestedIDs, err := uuidSliceFromJSON(row.SuggestedDigitalEmployeeIds)
	if err != nil {
		return TransferRequest{}, fmt.Errorf("suggested_digital_employee_ids: %w", err)
	}
	missingContextRefs, err := anySliceFromJSON(row.MissingContextRefs)
	if err != nil {
		return TransferRequest{}, fmt.Errorf("missing_context_refs: %w", err)
	}
	return TransferRequest{
		ID:                           row.ID,
		TenantID:                     row.TenantID,
		ProjectID:                    row.ProjectID,
		ProjectTaskID:                row.ProjectTaskID,
		RequestedByDigitalEmployeeID: row.RequestedByDigitalEmployeeID,
		Reason:                       row.Reason,
		SuggestedEmployeeType:        ptrText(row.SuggestedEmployeeType),
		SuggestedDigitalEmployeeIDs:  suggestedIDs,
		MissingContextRefs:           missingContextRefs,
		Status:                       row.Status,
		CreatedEventID:               ptrUUID(row.CreatedEventID),
		CreatedAt:                    row.CreatedAt.Time,
		UpdatedAt:                    row.UpdatedAt.Time,
	}, nil
}

func decisionRequestFromRecord(row queries.ProjectDecisionRequest) (DecisionRequest, error) {
	return DecisionRequest{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ProjectID:         row.ProjectID,
		ApprovalRequestID: row.ApprovalRequestID,
		CoordinationJobID: ptrUUID(row.CoordinationJobID),
		ProjectTaskID:     ptrUUID(row.ProjectTaskID),
		TargetUserID:      row.TargetUserID,
		DecisionType:      row.DecisionType,
		TitleSnapshot:     row.TitleSnapshot,
		SummarySnapshot:   ptrText(row.SummarySnapshot),
		RiskLevelSnapshot: ptrText(row.RiskLevelSnapshot),
		StatusSnapshot:    row.StatusSnapshot,
		CreatedEventID:    ptrUUID(row.CreatedEventID),
		ResolvedEventID:   ptrUUID(row.ResolvedEventID),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
		ResolvedAt:        ptrTime(row.ResolvedAt),
	}, nil
}

func evidenceRefFromRecord(row queries.ProjectEvidenceRef) (ProjectEvidenceRef, error) {
	metadata, err := mapFromJSON(row.Metadata)
	if err != nil {
		return ProjectEvidenceRef{}, fmt.Errorf("metadata: %w", err)
	}
	return ProjectEvidenceRef{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		ProjectID:          row.ProjectID,
		ProjectTaskID:      ptrUUID(row.ProjectTaskID),
		RouteDecisionID:    ptrUUID(row.RouteDecisionID),
		ExecutionSummaryID: ptrUUID(row.ExecutionSummaryID),
		EvidenceType:       row.EvidenceType,
		Title:              row.Title,
		Summary:            ptrText(row.Summary),
		SourceType:         row.SourceType,
		SourceRef:          row.SourceRef,
		ArtifactRefID:      ptrUUID(row.ArtifactRefID),
		SubmittedByType:    row.SubmittedByType,
		SubmittedByID:      ptrUUID(row.SubmittedByID),
		VerificationStatus: EvidenceVerificationStatus(row.VerificationStatus),
		Metadata:           metadata,
		CreatedEventID:     ptrUUID(row.CreatedEventID),
		CreatedAt:          row.CreatedAt.Time,
		UpdatedAt:          row.UpdatedAt.Time,
	}, nil
}

func artifactRefFromRecord(row queries.ProjectArtifactRef) (ProjectArtifactRef, error) {
	metadata, err := mapFromJSON(row.Metadata)
	if err != nil {
		return ProjectArtifactRef{}, fmt.Errorf("metadata: %w", err)
	}
	return ProjectArtifactRef{
		ID:              row.ID,
		TenantID:        row.TenantID,
		ProjectID:       row.ProjectID,
		ProjectTaskID:   ptrUUID(row.ProjectTaskID),
		ArtifactID:      ptrUUID(row.ArtifactID),
		ArtifactType:    row.ArtifactType,
		Title:           row.Title,
		ObjectRef:       row.ObjectRef,
		ContentType:     ptrText(row.ContentType),
		SizeBytes:       ptrInt8(row.SizeBytes),
		Checksum:        ptrText(row.Checksum),
		RetentionStatus: row.RetentionStatus,
		RetentionHoldID: ptrUUID(row.RetentionHoldID),
		Metadata:        metadata,
		CreatedEventID:  ptrUUID(row.CreatedEventID),
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
	}, nil
}

func reportRefFromRecord(row queries.ProjectReportRef) (ProjectReportRef, error) {
	return ProjectReportRef{
		ID:              row.ID,
		TenantID:        row.TenantID,
		ProjectID:       row.ProjectID,
		ReportType:      row.ReportType,
		Title:           row.Title,
		Summary:         ptrText(row.Summary),
		ObjectRef:       row.ObjectRef,
		Format:          row.Format,
		GeneratedByType: row.GeneratedByType,
		GeneratedByID:   ptrUUID(row.GeneratedByID),
		CreatedEventID:  ptrUUID(row.CreatedEventID),
		CreatedAt:       row.CreatedAt.Time,
	}, nil
}

func budgetLedgerEntryFromRecord(row queries.ProjectBudgetLedger) (ProjectBudgetLedgerEntry, error) {
	estimatedCost, err := numericToString(row.EstimatedCost)
	if err != nil {
		return ProjectBudgetLedgerEntry{}, fmt.Errorf("estimated_cost: %w", err)
	}
	actualCost, err := numericToString(row.ActualCost)
	if err != nil {
		return ProjectBudgetLedgerEntry{}, fmt.Errorf("actual_cost: %w", err)
	}
	return ProjectBudgetLedgerEntry{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ProjectID:         row.ProjectID,
		CoordinationJobID: ptrUUID(row.CoordinationJobID),
		ProjectTaskID:     ptrUUID(row.ProjectTaskID),
		DigitalEmployeeID: ptrUUID(row.DigitalEmployeeID),
		CostType:          row.CostType,
		EstimatedTokens:   ptrInt8(row.EstimatedTokens),
		ActualTokens:      ptrInt8(row.ActualTokens),
		EstimatedCost:     estimatedCost,
		ActualCost:        actualCost,
		Source:            row.Source,
		Reason:            ptrText(row.Reason),
		CreatedEventID:    ptrUUID(row.CreatedEventID),
		CreatedAt:         row.CreatedAt.Time,
	}, nil
}

func budgetSummaryFromRecord(row queries.GetProjectBudgetSummaryRow) ProjectBudgetSummary {
	estimatedCost, _ := numericToString(row.EstimatedCost)
	actualCost, _ := numericToString(row.ActualCost)
	return ProjectBudgetSummary{
		EstimatedTokens: row.EstimatedTokens,
		ActualTokens:    row.ActualTokens,
		EstimatedCost:   estimatedCost,
		ActualCost:      actualCost,
		LedgerCount:     row.LedgerCount,
	}
}

func acceptanceRecordFromRecord(row queries.ProjectAcceptanceRecord) (ProjectAcceptanceRecord, error) {
	evidenceRefIDs, err := uuidSliceFromJSON(row.EvidenceRefIds)
	if err != nil {
		return ProjectAcceptanceRecord{}, fmt.Errorf("evidence_ref_ids: %w", err)
	}
	reportRefIDs, err := uuidSliceFromJSON(row.ReportRefIds)
	if err != nil {
		return ProjectAcceptanceRecord{}, fmt.Errorf("report_ref_ids: %w", err)
	}
	unresolvedRisks, err := anySliceFromJSON(row.UnresolvedRisks)
	if err != nil {
		return ProjectAcceptanceRecord{}, fmt.Errorf("unresolved_risks: %w", err)
	}
	return ProjectAcceptanceRecord{
		ID:               row.ID,
		TenantID:         row.TenantID,
		ProjectID:        row.ProjectID,
		AcceptedByUserID: row.AcceptedByUserID,
		Status:           row.Status,
		Conclusion:       row.Conclusion,
		Summary:          ptrText(row.Summary),
		EvidenceRefIDs:   evidenceRefIDs,
		ReportRefIDs:     reportRefIDs,
		UnresolvedRisks:  unresolvedRisks,
		CreatedEventID:   ptrUUID(row.CreatedEventID),
		CreatedAt:        row.CreatedAt.Time,
	}, nil
}

func archiveSnapshotFromRecord(row queries.ProjectArchiveSnapshot) (ProjectArchiveSnapshot, error) {
	includedCounts, err := mapFromJSON(row.IncludedCounts)
	if err != nil {
		return ProjectArchiveSnapshot{}, fmt.Errorf("included_counts: %w", err)
	}
	retainedArtifactIDs, err := uuidSliceFromJSON(row.RetainedArtifactIds)
	if err != nil {
		return ProjectArchiveSnapshot{}, fmt.Errorf("retained_artifact_ids: %w", err)
	}
	return ProjectArchiveSnapshot{
		ID:                   row.ID,
		TenantID:             row.TenantID,
		ProjectID:            row.ProjectID,
		SnapshotType:         row.SnapshotType,
		Status:               row.Status,
		ObjectRef:            ptrText(row.ObjectRef),
		Summary:              ptrText(row.Summary),
		IncludedCounts:       includedCounts,
		RetainedArtifactIDs:  retainedArtifactIDs,
		RetentionLockEventID: ptrUUID(row.RetentionLockEventID),
		CreatedByUserID:      row.CreatedByUserID,
		CreatedEventID:       ptrUUID(row.CreatedEventID),
		CreatedAt:            row.CreatedAt.Time,
	}, nil
}

func projectsFromRecords(rows []queries.Project) ([]Project, error) {
	projects := make([]Project, 0, len(rows))
	for _, row := range rows {
		project, err := projectFromRecord(row)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func membersFromRecords(rows []queries.ProjectMember) ([]ProjectMember, error) {
	members := make([]ProjectMember, 0, len(rows))
	for _, row := range rows {
		member, err := memberFromRecord(row)
		if err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, nil
}

func tasksFromRecords(rows []queries.ProjectTask) ([]ProjectTask, error) {
	tasks := make([]ProjectTask, 0, len(rows))
	for _, row := range rows {
		task, err := taskFromRecord(row)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func dependenciesFromRecords(rows []queries.ProjectTaskDependency) []ProjectTaskDependency {
	dependencies := make([]ProjectTaskDependency, 0, len(rows))
	for _, row := range rows {
		dependencies = append(dependencies, dependencyFromRecord(row))
	}
	return dependencies
}

func eventsFromRecords(rows []queries.ProjectEvent) ([]ProjectEvent, error) {
	events := make([]ProjectEvent, 0, len(rows))
	for _, row := range rows {
		event, err := eventFromRecord(row)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, nil
}

func demandsFromRecords(rows []queries.ProjectDemand) ([]ProjectDemand, error) {
	demands := make([]ProjectDemand, 0, len(rows))
	for _, row := range rows {
		demand, err := demandFromRecord(row)
		if err != nil {
			return nil, err
		}
		demands = append(demands, demand)
	}
	return demands, nil
}

func coordinationJobsFromRecords(rows []queries.ProjectCoordinationJob) ([]CoordinationJob, error) {
	jobs := make([]CoordinationJob, 0, len(rows))
	for _, row := range rows {
		job, err := coordinationJobFromRecord(row)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func routeDecisionsFromRecords(rows []queries.ProjectRouteDecision) ([]RouteDecision, error) {
	decisions := make([]RouteDecision, 0, len(rows))
	for _, row := range rows {
		decision, err := routeDecisionFromRecord(row)
		if err != nil {
			return nil, err
		}
		decisions = append(decisions, decision)
	}
	return decisions, nil
}

func executionSummariesFromRecords(rows []queries.ProjectExecutionSummary) ([]ExecutionSummary, error) {
	summaries := make([]ExecutionSummary, 0, len(rows))
	for _, row := range rows {
		summary, err := executionSummaryFromRecord(row)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

func transferRequestsFromRecords(rows []queries.ProjectTransferRequest) ([]TransferRequest, error) {
	requests := make([]TransferRequest, 0, len(rows))
	for _, row := range rows {
		request, err := transferRequestFromRecord(row)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func decisionRequestsFromRecords(rows []queries.ProjectDecisionRequest) ([]DecisionRequest, error) {
	requests := make([]DecisionRequest, 0, len(rows))
	for _, row := range rows {
		request, err := decisionRequestFromRecord(row)
		if err != nil {
			return nil, err
		}
		requests = append(requests, request)
	}
	return requests, nil
}

func evidenceRefsFromRecords(rows []queries.ProjectEvidenceRef) ([]ProjectEvidenceRef, error) {
	refs := make([]ProjectEvidenceRef, 0, len(rows))
	for _, row := range rows {
		ref, err := evidenceRefFromRecord(row)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func artifactRefsFromRecords(rows []queries.ProjectArtifactRef) ([]ProjectArtifactRef, error) {
	refs := make([]ProjectArtifactRef, 0, len(rows))
	for _, row := range rows {
		ref, err := artifactRefFromRecord(row)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func reportRefsFromRecords(rows []queries.ProjectReportRef) ([]ProjectReportRef, error) {
	refs := make([]ProjectReportRef, 0, len(rows))
	for _, row := range rows {
		ref, err := reportRefFromRecord(row)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func budgetLedgerEntriesFromRecords(rows []queries.ProjectBudgetLedger) ([]ProjectBudgetLedgerEntry, error) {
	entries := make([]ProjectBudgetLedgerEntry, 0, len(rows))
	for _, row := range rows {
		entry, err := budgetLedgerEntryFromRecord(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func archiveSnapshotsFromRecords(rows []queries.ProjectArchiveSnapshot) ([]ProjectArchiveSnapshot, error) {
	snapshots := make([]ProjectArchiveSnapshot, 0, len(rows))
	for _, row := range rows {
		snapshot, err := archiveSnapshotFromRecord(row)
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, snapshot)
	}
	return snapshots, nil
}

func configRevisionsFromRecords(rows []queries.ProjectConfigRevision) ([]ProjectConfigRevision, error) {
	revisions := make([]ProjectConfigRevision, 0, len(rows))
	for _, row := range rows {
		revision, err := configRevisionFromRecord(row)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, revision)
	}
	return revisions, nil
}

func textOrNull(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func textPtr(value *string) pgtype.Text {
	if value == nil || *value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func ptrText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func nullUUIDIfNotNil(value uuid.UUID) uuid.NullUUID {
	if value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: value, Valid: true}
}

func ptrUUID(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := value.UUID
	return &id
}

func ptrTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func timestamptzPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func ptrBool(value pgtype.Bool) *bool {
	if !value.Valid {
		return nil
	}
	b := value.Bool
	return &b
}

func int8Ptr(value *int64) pgtype.Int8 {
	if value == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *value, Valid: true}
}

func ptrInt8(value pgtype.Int8) *int64 {
	if !value.Valid {
		return nil
	}
	n := value.Int64
	return &n
}

func int4Ptr(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}

func int32PtrFromSQL(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	v := value.Int32
	return &v
}

func jsonbObject(value map[string]any, field string) ([]byte, error) {
	if len(value) == 0 {
		return []byte("{}"), nil
	}
	return marshalJSON(value, field)
}

func jsonbObjectOrNull(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return jsonbObject(value, field)
}

func jsonbArray(value []any, field string) ([]byte, error) {
	if len(value) == 0 {
		return []byte("[]"), nil
	}
	return marshalJSON(value, field)
}

func jsonbUUIDSlice(values []uuid.UUID, field string) ([]byte, error) {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			encoded = append(encoded, value.String())
		}
	}
	return marshalJSON(encoded, field)
}

func uuidSliceFromJSON(raw []byte) ([]uuid.UUID, error) {
	values := []string{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
	}
	ids := make([]uuid.UUID, 0, len(values))
	for _, value := range values {
		id, err := uuid.Parse(value)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func anySliceFromJSON(raw []byte) ([]any, error) {
	values := []any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &values); err != nil {
			return nil, err
		}
		if values == nil {
			values = []any{}
		}
	}
	return values, nil
}

func contractArrayFromJSON(raw []byte, field string) ([]any, error) {
	if len(raw) == 0 {
		return []any{}, nil
	}
	var values []any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if values == nil {
		return nil, fmt.Errorf("%s: json null is not a valid contract array", field)
	}
	return values, nil
}

func mapFromJSON(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]any{}, nil
	}
	return value, nil
}

func contractObjectFromJSON(raw []byte, field string) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("%s: %w", field, err)
	}
	if value == nil {
		return nil, fmt.Errorf("%s: json null is not a valid contract object", field)
	}
	return value, nil
}

func marshalJSON(value any, field string) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%s: marshal json: %w", field, err)
	}
	return raw, nil
}

func numericFromDecimalString(value string) (pgtype.Numeric, error) {
	if value == "" {
		return pgtype.Numeric{}, nil
	}
	var numeric pgtype.Numeric
	if err := numeric.Scan(value); err != nil {
		return pgtype.Numeric{}, err
	}
	return numeric, nil
}

func numericToString(value pgtype.Numeric) (string, error) {
	if !value.Valid {
		return "", nil
	}
	encoded, err := value.Value()
	if err != nil {
		return "", err
	}
	if encoded == nil {
		return "", nil
	}
	return fmt.Sprint(encoded), nil
}

func projectStatusPtr(status *ProjectStatus) pgtype.Text {
	if status == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*status), Valid: true}
}

func evidenceVerificationStatusPtr(status *EvidenceVerificationStatus) pgtype.Text {
	if status == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*status), Valid: true}
}

func projectConfigChangedSections(req UpdateProjectConfigRequest) []any {
	sections := make([]any, 0, 9)
	if req.Name != "" {
		sections = append(sections, "name")
	}
	if req.Description != "" {
		sections = append(sections, "description")
	}
	if req.Goal != "" {
		sections = append(sections, "goal")
	}
	if req.HumanOwnerUserID != uuid.Nil {
		sections = append(sections, "human_owner_user_id")
	}
	if req.LeaderUserID != nil {
		sections = append(sections, "leader_user_id")
	}
	if req.AcceptanceUserID != nil {
		sections = append(sections, "acceptance_user_id")
	}
	if req.CoordinationPolicy != nil {
		sections = append(sections, "coordination_policy")
	}
	if req.ApprovalPolicy != nil {
		sections = append(sections, "approval_policy")
	}
	if req.EvidencePolicy != nil {
		sections = append(sections, "evidence_policy")
	}
	if len(sections) > 0 {
		return sections
	}
	return []any{
		"name",
		"goal",
		"human_owner_user_id",
		"leader_user_id",
		"acceptance_user_id",
		"coordination_policy",
		"approval_policy",
		"evidence_policy",
	}
}

func projectConfigDiffSummary(changedSections []any) map[string]any {
	return map[string]any{
		"change_summary":   "项目配置已更新",
		"changed_sections": changedSections,
	}
}

func projectConfigPolicyFingerprint(snapshot map[string]any) (string, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", fmt.Errorf("marshal policy fingerprint snapshot: %w", err)
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("%x", sum), nil
}

func projectConfigSnapshot(project Project) map[string]any {
	snapshot := map[string]any{
		"name":                project.Name,
		"goal":                project.Goal,
		"status":              string(project.Status),
		"human_owner_user_id": project.HumanOwnerUserID.String(),
		"coordination_policy": project.CoordinationPolicy,
		"approval_policy":     project.ApprovalPolicy,
		"evidence_policy":     project.EvidencePolicy,
	}
	if project.LeaderUserID != nil {
		snapshot["leader_user_id"] = project.LeaderUserID.String()
	} else {
		snapshot["leader_user_id"] = ""
	}
	if project.AcceptanceUserID != nil {
		snapshot["acceptance_user_id"] = project.AcceptanceUserID.String()
	} else {
		snapshot["acceptance_user_id"] = ""
	}
	return snapshot
}

func isProjectEventSequenceConflict(err error) bool {
	return isPGUniqueConstraint(err, "uq_project_events_project_sequence")
}

func isProjectConfigRevisionConflict(err error) bool {
	return isPGUniqueConstraint(err, "uq_project_config_revisions_project_rev")
}

func isPGUniqueConstraint(err error, constraintName string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == constraintName
}
