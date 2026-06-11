package projectcoordination

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
)

func TestProjectCoordinatorHandlesDemandSubmitted(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	executorID := uuid.New()
	store := &recordingActivityStore{
		snapshot: CoordinationSnapshot{
			ProjectID: uuid.New(),
			Demand: DemandSnapshot{
				ID:      uuid.New(),
				Title:   "验证 Runtime",
				Content: "检查心跳",
			},
			DigitalEmployeePool: []ProjectMemberSnapshot{
				{PrincipalID: executorID, ProjectRole: "executor", Status: "active"},
			},
		},
		jobID:         uuid.New(),
		routeID:       uuid.New(),
		routeEventID:  uuid.New(),
		taskID:        uuid.New(),
		dispatchEvent: uuid.New(),
	}
	activities := NewActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalDemandSubmitted, DemandSubmitted{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			SubmittedByUserID: uuid.New(),
			CreatedEventID:    uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  store.snapshot.ProjectID,
		WorkflowID: "project-coordinator:" + store.snapshot.ProjectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"CreateCoordinationJob",
		"LoadProjectCoordinationSnapshot",
		"PersistRouteDecision",
		"CreateProjectTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
}

type recordingActivityStore struct {
	calls         []string
	snapshot      CoordinationSnapshot
	jobID         uuid.UUID
	routeID       uuid.UUID
	routeEventID  uuid.UUID
	taskID        uuid.UUID
	dispatchEvent uuid.UUID
}

func (s *recordingActivityStore) LoadProjectCoordinationSnapshot(ctx context.Context, input LoadSnapshotInput) (CoordinationSnapshot, error) {
	s.calls = append(s.calls, "LoadProjectCoordinationSnapshot")
	return s.snapshot, nil
}

func (s *recordingActivityStore) CreateCoordinationJob(ctx context.Context, input CreateCoordinationJobInput) (CoordinationJobResult, error) {
	s.calls = append(s.calls, "CreateCoordinationJob")
	return CoordinationJobResult{ID: s.jobID}, nil
}

func (s *recordingActivityStore) PersistRouteDecision(ctx context.Context, input PersistRouteDecisionInput) (RouteDecisionResult, error) {
	s.calls = append(s.calls, "PersistRouteDecision")
	return RouteDecisionResult{ID: s.routeID, CreatedEventID: s.routeEventID}, nil
}

func (s *recordingActivityStore) CreateProjectTasks(ctx context.Context, input CreateProjectTasksInput) ([]ProjectTaskResult, error) {
	s.calls = append(s.calls, "CreateProjectTasks")
	return []ProjectTaskResult{{ID: s.taskID}}, nil
}

func (s *recordingActivityStore) AppendProjectEvent(ctx context.Context, input AppendProjectEventInput) (ProjectEventResult, error) {
	s.calls = append(s.calls, "AppendProjectEvent")
	return ProjectEventResult{ID: s.dispatchEvent}, nil
}

func (s *recordingActivityStore) DispatchProjectTask(ctx context.Context, input DispatchProjectTaskInput) error {
	s.calls = append(s.calls, "DispatchProjectTask")
	return nil
}

func (s *recordingActivityStore) FinishCoordinationJob(ctx context.Context, input FinishCoordinationJobInput) error {
	s.calls = append(s.calls, "FinishCoordinationJob")
	return nil
}
