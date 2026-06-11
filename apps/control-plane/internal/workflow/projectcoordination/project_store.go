package projectcoordination

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

type ProjectStore struct {
	repository project.Repository
}

func NewProjectStore(repository project.Repository) *ProjectStore {
	return &ProjectStore{repository: repository}
}

func (s *ProjectStore) LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error) {
	if s.repository == nil {
		return CoordinationSnapshot{}, ErrActivityStoreRequired
	}
	projectRecord, err := s.repository.GetProject(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	demand, err := s.repository.GetProjectDemand(ctx, input.TenantID, input.DemandID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	members, err := s.repository.ListProjectMembers(ctx, input.TenantID, input.ProjectID)
	if err != nil {
		return CoordinationSnapshot{}, err
	}
	pool := make([]ProjectMemberSnapshot, 0, len(members))
	for _, member := range members {
		if member.PrincipalType != project.PrincipalTypeDigitalEmployee || member.Status != "active" || !isRoutableDigitalProjectRole(member.ProjectRole) {
			continue
		}
		displayName := ""
		if member.DisplayNameSnapshot != nil {
			displayName = *member.DisplayNameSnapshot
		}
		pool = append(pool, ProjectMemberSnapshot{
			PrincipalID: member.PrincipalID,
			ProjectRole: string(member.ProjectRole),
			Status:      member.Status,
			DisplayName: displayName,
		})
	}
	content := ""
	if demand.Content != nil {
		content = *demand.Content
	}
	return CoordinationSnapshot{
		ProjectID:           projectRecord.ID,
		Demand:              DemandSnapshot{ID: demand.ID, Title: demand.Title, Content: content},
		DigitalEmployeePool: pool,
		CoordinationPolicy:  projectRecord.CoordinationPolicy,
	}, nil
}

func (s *ProjectStore) CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error) {
	if s.repository == nil {
		return CoordinationJobResult{}, ErrActivityStoreRequired
	}
	triggerEventID := input.TriggerEventID
	job, err := s.repository.CreateCoordinationJob(ctx, project.CreateCoordinationJobRequest{
		TenantID:       input.TenantID,
		ProjectID:      input.ProjectID,
		WorkflowID:     input.WorkflowID,
		TriggerEventID: &triggerEventID,
		JobType:        input.JobType,
		Status:         "running",
		InputSnapshotRef: map[string]any{
			"trigger_event_id": input.TriggerEventID.String(),
			"job_type":         input.JobType,
		},
	})
	if err != nil {
		return CoordinationJobResult{}, err
	}
	if _, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventCoordinationJobCreated, input.WorkflowID, "协调作业已创建", map[string]any{"coordination_job_id": job.ID.String()})); err != nil {
		return CoordinationJobResult{}, err
	}
	return CoordinationJobResult{ID: job.ID}, nil
}

func (s *ProjectStore) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	if s.repository == nil {
		return RouteDecisionResult{}, ErrActivityStoreRequired
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventRouteDecisionCreated, input.JobID.String(), "路由决策已生成", map[string]any{
		"coordination_job_id": input.JobID.String(),
		"demand_id":           input.DemandID.String(),
	}))
	if err != nil {
		return RouteDecisionResult{}, err
	}
	demandID := input.DemandID
	decision, err := s.repository.CreateRouteDecision(ctx, project.CreateRouteDecisionRequest{
		TenantID:                    input.TenantID,
		ProjectID:                   input.ProjectID,
		CoordinationJobID:           input.JobID,
		DemandID:                    &demandID,
		CandidateDigitalEmployeeIDs: input.Decision.CandidateDigitalEmployeeIDs,
		SelectedDigitalEmployeeIDs:  input.Decision.SelectedDigitalEmployeeIDs,
		Reason:                      input.Decision.Reason,
		InputRequirements:           input.Decision.InputRequirements,
		ExpectedOutputs:             stringsToAny(input.Decision.ExpectedOutputs),
		BudgetEstimate:              input.Decision.BudgetEstimate,
		RequiresHumanReview:         input.Decision.RequiresHumanReview,
		CreatedEventID:              &event.ID,
	})
	if err != nil {
		return RouteDecisionResult{}, err
	}
	return RouteDecisionResult{ID: decision.ID, CreatedEventID: event.ID}, nil
}

func (s *ProjectStore) CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error) {
	if s.repository == nil {
		return nil, ErrActivityStoreRequired
	}
	results := make([]ProjectTaskResult, 0, len(input.Decision.SelectedDigitalEmployeeIDs))
	for _, employeeID := range input.Decision.SelectedDigitalEmployeeIDs {
		task, err := s.repository.CreateProjectTask(ctx, project.CreateProjectTaskRequest{
			TenantID:                  input.TenantID,
			ProjectID:                 input.ProjectID,
			DemandID:                  &input.DemandID,
			Title:                     input.Decision.TaskTitle,
			Summary:                   input.Decision.TaskSummary,
			Status:                    "planned",
			AssignedDigitalEmployeeID: &employeeID,
			RequiresHumanApproval:     input.Decision.RequiresHumanReview,
		})
		if err != nil {
			return nil, err
		}
		if _, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskCreated, task.ID.String(), "项目任务已创建", map[string]any{
			"project_task_id": task.ID.String(),
			"demand_id":       input.DemandID.String(),
		})); err != nil {
			return nil, err
		}
		results = append(results, ProjectTaskResult{ID: task.ID})
	}
	return results, nil
}

func (s *ProjectStore) AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error) {
	if s.repository == nil {
		return ProjectEventResult{}, ErrActivityStoreRequired
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventType(input.EventType), "project_coordinator", input.Summary, map[string]any{}))
	if err != nil {
		return ProjectEventResult{}, err
	}
	return ProjectEventResult{ID: event.ID}, nil
}

func (s *ProjectStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	event, err := s.repository.AppendProjectEvent(ctx, coordinatorEvent(input.TenantID, input.ProjectID, project.ProjectEventTaskDispatched, input.TaskID.String(), "项目任务已分派", map[string]any{"project_task_id": input.TaskID.String()}))
	if err != nil {
		return err
	}
	_, err = s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, input.TaskID, "assigned", &event.ID)
	return err
}

func (s *ProjectStore) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	if s.repository == nil {
		return ErrActivityStoreRequired
	}
	outputEventIDs := make([]any, 0, len(input.OutputEventIDs))
	for _, id := range input.OutputEventIDs {
		outputEventIDs = append(outputEventIDs, id.String())
	}
	_, err := s.repository.FinishCoordinationJob(ctx, project.FinishCoordinationJobRequest{
		TenantID:       input.TenantID,
		ID:             input.JobID,
		Status:         input.Status,
		OutputEventIDs: outputEventIDs,
	})
	return err
}

func coordinatorEvent(tenantID, projectID uuid.UUID, eventType project.ProjectEventType, actorID, summary string, payload map[string]any) project.AppendProjectEventRequest {
	if actorID == "" {
		actorID = "project_coordinator"
	}
	return project.AppendProjectEventRequest{
		TenantID:  tenantID,
		ProjectID: projectID,
		EventType: eventType,
		ActorType: "project_coordinator",
		ActorID:   actorID,
		Summary:   summary,
		Payload:   payload,
	}
}

func stringsToAny(values []string) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func isRoutableDigitalProjectRole(role project.ProjectRole) bool {
	return role == project.ProjectRoleExecutor || role == project.ProjectRoleReviewer
}
