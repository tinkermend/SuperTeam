package project

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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

func (r *PgRepository) AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error) {
	return r.appendProjectEventWithQueries(ctx, r.q, event)
}

func (r *PgRepository) appendProjectEventWithQueries(ctx context.Context, q *queries.Queries, event AppendProjectEventRequest) (ProjectEvent, error) {
	payload, err := jsonbObject(event.Payload, "payload")
	if err != nil {
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
	row, err := r.q.GetProjectTask(ctx, queries.GetProjectTaskParams{TenantID: tenantID, ID: projectTaskID})
	if err != nil {
		return ProjectTask{}, err
	}
	return taskFromRecord(row), nil
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

func (r *PgRepository) CreateCoordinationJob(ctx context.Context, req CreateCoordinationJobRequest) (CoordinationJob, error) {
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
	row, err := r.q.CreateProjectTask(ctx, queries.CreateProjectTaskParams{
		TenantID:                  req.TenantID,
		ProjectID:                 req.ProjectID,
		DemandID:                  nullUUID(req.DemandID),
		Title:                     req.Title,
		Summary:                   textOrNull(req.Summary),
		Status:                    req.Status,
		AssignedDigitalEmployeeID: nullUUID(req.AssignedDigitalEmployeeID),
		RuntimeTaskID:             nullUUID(req.RuntimeTaskID),
		DigitalEmployeeRunID:      nullUUID(req.DigitalEmployeeRunID),
		RiskLevel:                 textOrNull(req.RiskLevel),
		RequiresHumanApproval:     req.RequiresHumanApproval,
	})
	if err != nil {
		return ProjectTask{}, err
	}
	return taskFromRecord(row), nil
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
	return taskFromRecord(row), nil
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
	task := taskFromRecord(existing)
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
	return taskFromRecord(row), nil
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
	return taskFromRecord(row), nil
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
		return ProjectTaskWritebackResult{Task: task, Event: event}, nil
	})
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

func taskFromRecord(row queries.ProjectTask) ProjectTask {
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
		CreatedAt:                 row.CreatedAt.Time,
		UpdatedAt:                 row.UpdatedAt.Time,
	}
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
		tasks = append(tasks, taskFromRecord(row))
	}
	return tasks, nil
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
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "uq_project_events_project_sequence"
}

func isProjectConfigRevisionConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "uq_project_config_revisions_project_rev"
}
