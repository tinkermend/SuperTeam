package task

import (
	"context"

	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateTask(ctx context.Context, params CreateTaskParams) (TaskRecord, error) {
	task, err := r.q.CreateTask(ctx, queries.CreateTaskParams{
		Title:         params.Title,
		Description:   params.Description,
		Status:        params.Status,
		Priority:      params.Priority,
		ProviderType:  params.ProviderType,
		CreatorID:     params.CreatorID,
		TargetNodeID:  params.TargetNodeID,
		WorkspacePath: params.WorkspacePath,
		Params:        params.Params,
	})
	if err != nil {
		return TaskRecord{}, err
	}
	return TaskRecord{
		ID:             task.ID,
		Title:          task.Title,
		Description:    task.Description,
		CreatorID:      task.CreatorID,
		ProviderType:   task.ProviderType,
		TargetNodeID:   task.TargetNodeID,
		AssignedNodeID: task.AssignedNodeID,
		Status:         task.Status,
		WorkspacePath:  task.WorkspacePath,
		Params:         task.Params,
		Priority:       task.Priority,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}, nil
}

func (r *PgRepository) GetTask(ctx context.Context, id int64) (TaskRecord, error) {
	task, err := r.q.GetTask(ctx, id)
	if err != nil {
		return TaskRecord{}, err
	}
	return TaskRecord{
		ID:             task.ID,
		Title:          task.Title,
		Description:    task.Description,
		CreatorID:      task.CreatorID,
		ProviderType:   task.ProviderType,
		TargetNodeID:   task.TargetNodeID,
		AssignedNodeID: task.AssignedNodeID,
		Status:         task.Status,
		WorkspacePath:  task.WorkspacePath,
		Params:         task.Params,
		Priority:       task.Priority,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}, nil
}

func (r *PgRepository) ListTasks(ctx context.Context, params ListTasksParams) ([]TaskRecord, error) {
	tasks, err := r.q.ListTasks(ctx, queries.ListTasksParams{
		Status:       params.Status,
		CreatorID:    params.CreatorID,
		ProviderType: params.ProviderType,
		Offset:       params.Offset,
		Limit:        params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]TaskRecord, len(tasks))
	for i, task := range tasks {
		records[i] = TaskRecord{
			ID:             task.ID,
			Title:          task.Title,
			Description:    task.Description,
			CreatorID:      task.CreatorID,
			ProviderType:   task.ProviderType,
			TargetNodeID:   task.TargetNodeID,
			AssignedNodeID: task.AssignedNodeID,
			Status:         task.Status,
			WorkspacePath:  task.WorkspacePath,
			Params:         task.Params,
			Priority:       task.Priority,
			CreatedAt:      task.CreatedAt,
			UpdatedAt:      task.UpdatedAt,
		}
	}
	return records, nil
}

func (r *PgRepository) UpdateTaskStatus(ctx context.Context, params UpdateTaskStatusParams) (TaskRecord, error) {
	task, err := r.q.UpdateTaskStatus(ctx, queries.UpdateTaskStatusParams{
		ID:     params.ID,
		Status: params.Status,
	})
	if err != nil {
		return TaskRecord{}, err
	}
	return TaskRecord{
		ID:             task.ID,
		Title:          task.Title,
		Description:    task.Description,
		CreatorID:      task.CreatorID,
		ProviderType:   task.ProviderType,
		TargetNodeID:   task.TargetNodeID,
		AssignedNodeID: task.AssignedNodeID,
		Status:         task.Status,
		WorkspacePath:  task.WorkspacePath,
		Params:         task.Params,
		Priority:       task.Priority,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}, nil
}

func (r *PgRepository) UpdateTask(ctx context.Context, params UpdateTaskParams) (TaskRecord, error) {
	task, err := r.q.UpdateTask(ctx, queries.UpdateTaskParams{
		Title:          params.Title,
		Description:    params.Description,
		Status:         params.Status,
		Priority:       params.Priority,
		TargetNodeID:   params.TargetNodeID,
		AssignedNodeID: params.AssignedNodeID,
		WorkspacePath:  params.WorkspacePath,
		Params:         params.Params,
		ID:             params.ID,
	})
	if err != nil {
		return TaskRecord{}, err
	}
	return TaskRecord{
		ID:             task.ID,
		Title:          task.Title,
		Description:    task.Description,
		CreatorID:      task.CreatorID,
		ProviderType:   task.ProviderType,
		TargetNodeID:   task.TargetNodeID,
		AssignedNodeID: task.AssignedNodeID,
		Status:         task.Status,
		WorkspacePath:  task.WorkspacePath,
		Params:         task.Params,
		Priority:       task.Priority,
		CreatedAt:      task.CreatedAt,
		UpdatedAt:      task.UpdatedAt,
	}, nil
}

func (r *PgRepository) DeleteTask(ctx context.Context, id int64) error {
	return r.q.DeleteTask(ctx, id)
}

func (r *PgRepository) CreateTaskStateHistory(ctx context.Context, params CreateTaskStateHistoryParams) error {
	_, err := r.q.CreateTaskStateHistory(ctx, queries.CreateTaskStateHistoryParams{
		TaskID:     params.TaskID,
		FromStatus: params.FromStatus,
		ToStatus:   params.ToStatus,
		ChangedBy:  params.ChangedBy,
		Reason:     params.Reason,
	})
	return err
}

func (r *PgRepository) CreateTaskEvent(ctx context.Context, params CreateTaskEventParams) (TaskEventRecord, error) {
	event, err := r.q.CreateTaskEvent(ctx, queries.CreateTaskEventParams{
		TaskID:         params.TaskID,
		ExecutionID:    params.ExecutionID,
		EventType:      params.EventType,
		SequenceNumber: params.SequenceNumber,
		Payload:        params.Payload,
	})
	if err != nil {
		return TaskEventRecord{}, err
	}
	return TaskEventRecord{
		ID:             event.ID,
		TaskID:         event.TaskID,
		ExecutionID:    event.ExecutionID,
		EventType:      event.EventType,
		SequenceNumber: event.SequenceNumber,
		Payload:        event.Payload,
		CreatedAt:      event.CreatedAt,
	}, nil
}

func (r *PgRepository) GetLatestTaskEventSequence(ctx context.Context, taskID int64) (int32, error) {
	return r.q.GetLatestTaskEventSequence(ctx, taskID)
}
