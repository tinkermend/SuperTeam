package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

const maxProjectEventAppendAttempts = 3

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateProject(ctx context.Context, req CreateProjectRequest, projectID uuid.UUID, workflowID string) (Project, error) {
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
		CoordinationPolicy:     jsonbOrDefault(req.CoordinationPolicy),
		ApprovalPolicy:         jsonbOrDefault(req.ApprovalPolicy),
		EvidencePolicy:         jsonbOrDefault(req.EvidencePolicy),
	})
	if err != nil {
		return Project{}, err
	}
	return projectFromRecord(row)
}

func (r *PgRepository) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	row, err := r.q.GetProject(ctx, queries.GetProjectParams{TenantID: tenantID, ID: projectID})
	if err != nil {
		return Project{}, err
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
	row, err := r.q.UpdateProject(ctx, queries.UpdateProjectParams{
		TenantID:           req.TenantID,
		ID:                 req.ProjectID,
		Name:               textOrNull(req.Name),
		Description:        textOrNull(req.Description),
		Goal:               textOrNull(req.Goal),
		HumanOwnerUserID:   nullUUIDIfNotNil(req.HumanOwnerUserID),
		LeaderUserID:       nullUUID(req.LeaderUserID),
		AcceptanceUserID:   nullUUID(req.AcceptanceUserID),
		CoordinationPolicy: jsonbOrNull(req.CoordinationPolicy),
		ApprovalPolicy:     jsonbOrNull(req.ApprovalPolicy),
		EvidencePolicy:     jsonbOrNull(req.EvidencePolicy),
	})
	if err != nil {
		return Project{}, err
	}
	return projectFromRecord(row)
}

func (r *PgRepository) ArchiveProject(ctx context.Context, tenantID, projectID uuid.UUID) (Project, error) {
	row, err := r.q.ArchiveProject(ctx, queries.ArchiveProjectParams{TenantID: tenantID, ID: projectID})
	if err != nil {
		return Project{}, err
	}
	return projectFromRecord(row)
}

func (r *PgRepository) ReplaceProjectMembers(ctx context.Context, tenantID, projectID uuid.UUID, members []ProjectMemberInput) ([]ProjectMember, error) {
	if err := r.q.ReplaceProjectMembersDelete(ctx, queries.ReplaceProjectMembersDeleteParams{TenantID: tenantID, ProjectID: projectID}); err != nil {
		return nil, err
	}
	created := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		row, err := r.q.CreateProjectMember(ctx, queries.CreateProjectMemberParams{
			TenantID:            tenantID,
			ProjectID:           projectID,
			PrincipalType:       string(member.PrincipalType),
			PrincipalID:         member.PrincipalID,
			ProjectRole:         string(member.ProjectRole),
			DisplayNameSnapshot: textOrNull(member.DisplayNameSnapshot),
			Status:              "active",
			Settings:            jsonbOrDefault(member.Settings),
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

func (r *PgRepository) AppendProjectEvent(ctx context.Context, event AppendProjectEventRequest) (ProjectEvent, error) {
	var lastErr error
	for attempt := 0; attempt < maxProjectEventAppendAttempts; attempt++ {
		latest, err := r.q.GetLatestProjectEventSequence(ctx, queries.GetLatestProjectEventSequenceParams{TenantID: event.TenantID, ProjectID: event.ProjectID})
		if err != nil {
			return ProjectEvent{}, err
		}
		row, err := r.q.CreateProjectEvent(ctx, queries.CreateProjectEventParams{
			TenantID:       event.TenantID,
			ProjectID:      event.ProjectID,
			SequenceNumber: latest + 1,
			EventType:      string(event.EventType),
			ActorType:      event.ActorType,
			ActorID:        event.ActorID,
			ResourceType:   textPtr(event.ResourceType),
			ResourceID:     textPtr(event.ResourceID),
			Summary:        textOrNull(event.Summary),
			Payload:        jsonbOrDefault(event.Payload),
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

func (r *PgRepository) CreateProjectDemand(ctx context.Context, req SubmitProjectDemandRequest, status ProjectDemandStatus, createdEventID *uuid.UUID) (ProjectDemand, error) {
	row, err := r.q.CreateProjectDemand(ctx, queries.CreateProjectDemandParams{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		SubmittedByUserID: req.SubmittedByUserID,
		Title:             req.Title,
		Content:           textOrNull(req.Content),
		SourceType:        string(req.SourceType),
		SourceRefs:        jsonbOrDefault(nil),
		Attachments:       jsonbArrayOrDefault(nil),
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
	latest, err := r.q.GetLatestProjectConfigRevisionNumber(ctx, queries.GetLatestProjectConfigRevisionNumberParams{TenantID: req.TenantID, ProjectID: req.ProjectID})
	if err != nil {
		return ProjectConfigRevision{}, err
	}
	row, err := r.q.CreateProjectConfigRevision(ctx, queries.CreateProjectConfigRevisionParams{
		TenantID:        req.TenantID,
		ProjectID:       req.ProjectID,
		RevisionNumber:  latest + 1,
		ConfigSnapshot:  jsonbOrDefault(projectConfigSnapshot(project)),
		ChangeSummary:   textOrNull("项目配置已更新"),
		CreatedByUserID: req.ActorUserID,
		CreatedEventID:  nullUUID(&eventID),
	})
	if err != nil {
		return ProjectConfigRevision{}, err
	}
	return configRevisionFromRecord(row)
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
	if _, err := mapFromJSON(row.SourceRefs); err != nil {
		return ProjectDemand{}, fmt.Errorf("source_refs: %w", err)
	}
	if len(row.Attachments) > 0 {
		var attachments []any
		if err := json.Unmarshal(row.Attachments, &attachments); err != nil {
			return ProjectDemand{}, fmt.Errorf("attachments: %w", err)
		}
	}
	return ProjectDemand{
		ID:                row.ID,
		TenantID:          row.TenantID,
		ProjectID:         row.ProjectID,
		SubmittedByUserID: row.SubmittedByUserID,
		Title:             row.Title,
		Content:           ptrText(row.Content),
		SourceType:        DemandSourceType(row.SourceType),
		Status:            ProjectDemandStatus(row.Status),
		CreatedEventID:    ptrUUID(row.CreatedEventID),
		CreatedAt:         row.CreatedAt.Time,
		UpdatedAt:         row.UpdatedAt.Time,
	}, nil
}

func configRevisionFromRecord(row queries.ProjectConfigRevision) (ProjectConfigRevision, error) {
	snapshot, err := mapFromJSON(row.ConfigSnapshot)
	if err != nil {
		return ProjectConfigRevision{}, fmt.Errorf("config_snapshot: %w", err)
	}
	return ProjectConfigRevision{
		ID:              row.ID,
		TenantID:        row.TenantID,
		ProjectID:       row.ProjectID,
		RevisionNumber:  row.RevisionNumber,
		ConfigSnapshot:  snapshot,
		ChangeSummary:   ptrText(row.ChangeSummary),
		CreatedByUserID: row.CreatedByUserID,
		CreatedEventID:  ptrUUID(row.CreatedEventID),
		CreatedAt:       row.CreatedAt.Time,
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

func jsonbOrDefault(value map[string]any) []byte {
	if len(value) == 0 {
		return []byte("{}")
	}
	return mustJSON(value)
}

func jsonbOrNull(value map[string]any) []byte {
	if value == nil {
		return nil
	}
	return jsonbOrDefault(value)
}

func jsonbArrayOrDefault(value []any) []byte {
	if len(value) == 0 {
		return []byte("[]")
	}
	return mustJSON(value)
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

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte("{}")
	}
	return raw
}

func projectStatusPtr(status *ProjectStatus) pgtype.Text {
	if status == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(*status), Valid: true}
}

func projectConfigSnapshot(project Project) map[string]any {
	return map[string]any{
		"name":                project.Name,
		"goal":                project.Goal,
		"status":              string(project.Status),
		"coordination_policy": project.CoordinationPolicy,
		"approval_policy":     project.ApprovalPolicy,
		"evidence_policy":     project.EvidencePolicy,
	}
}

func isProjectEventSequenceConflict(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) &&
		pgErr.Code == "23505" &&
		pgErr.ConstraintName == "uq_project_events_project_sequence"
}
