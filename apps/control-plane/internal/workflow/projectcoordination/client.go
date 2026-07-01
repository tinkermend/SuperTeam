package projectcoordination

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
)

type SignalClient struct {
	client    client.Client
	taskQueue string
}

func NewSignalClient(c client.Client, taskQueue string) *SignalClient {
	return &SignalClient{client: c, taskQueue: taskQueue}
}

func (c *SignalClient) EnsureProjectCoordinator(ctx context.Context, signal project.ProjectCoordinatorSignal) error {
	_, err := c.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:                    workflowID(signal.WorkflowID, signal.ProjectID.String()),
		TaskQueue:             c.taskQueue,
		WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
	}, ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   signal.TenantID,
		ProjectID:  signal.ProjectID,
		WorkflowID: workflowID(signal.WorkflowID, signal.ProjectID.String()),
	})
	if err != nil && temporal.IsWorkflowExecutionAlreadyStartedError(err) {
		return nil
	}
	return err
}

func (c *SignalClient) SignalDemandSubmitted(ctx context.Context, signal project.DemandSubmittedSignal) error {
	return c.signal(ctx, signal.TenantID, signal.WorkflowID, signal.ProjectID, SignalDemandSubmitted, DemandSubmitted{
		DemandID:          signal.DemandID,
		ProjectID:         signal.ProjectID,
		SubmittedByUserID: signal.SubmittedByUserID,
		CreatedEventID:    signal.CreatedEventID,
	})
}

func (c *SignalClient) SignalProjectPolicyChanged(ctx context.Context, signal project.ProjectPolicyChangedSignal) error {
	return c.signal(ctx, signal.TenantID, signal.WorkflowID, signal.ProjectID, SignalProjectPolicyChanged, ProjectPolicyChanged{
		ProjectID:        signal.ProjectID,
		ConfigRevisionID: signal.ConfigRevisionID,
		ChangedEventID:   signal.ChangedEventID,
	})
}

func (c *SignalClient) SignalProjectMemberChanged(ctx context.Context, signal project.ProjectMemberChangedSignal) error {
	return c.signal(ctx, signal.TenantID, signal.WorkflowID, signal.ProjectID, SignalProjectMemberChanged, ProjectMemberChanged{
		ProjectID:        signal.ProjectID,
		ChangedMemberIDs: signal.ChangedMemberIDs,
		ChangedEventID:   signal.ChangedEventID,
	})
}

func (c *SignalClient) SignalEmployeeTaskCompleted(ctx context.Context, signal project.EmployeeTaskCompletedSignal) error {
	return c.signal(ctx, signal.TenantID, signal.WorkflowID, signal.ProjectID, SignalEmployeeTaskCompleted, EmployeeTaskCompleted{
		ProjectTaskID:      signal.ProjectTaskID,
		ExecutionSummaryID: signal.ExecutionSummaryID,
		CompletedEventID:   signal.CompletedEventID,
	})
}

func (c *SignalClient) SignalEmployeeTaskFailed(ctx context.Context, signal project.EmployeeTaskFailedSignal) error {
	return c.signal(ctx, signal.TenantID, signal.WorkflowID, signal.ProjectID, SignalEmployeeTaskFailed, EmployeeTaskFailed{
		ProjectTaskID:  signal.ProjectTaskID,
		FailureSummary: signal.FailureSummary,
		FailedEventID:  signal.FailedEventID,
	})
}

func (c *SignalClient) SignalEmployeeTransferRequested(ctx context.Context, signal project.EmployeeTransferRequestedSignal) error {
	return c.signal(ctx, signal.TenantID, signal.WorkflowID, signal.ProjectID, SignalEmployeeTransferRequested, EmployeeTransferRequested{
		ProjectTaskID:     signal.ProjectTaskID,
		TransferRequestID: signal.TransferRequestID,
		RequestedEventID:  signal.RequestedEventID,
	})
}

func (c *SignalClient) SignalHumanDecisionSubmitted(ctx context.Context, signal project.HumanDecisionSubmittedSignal) error {
	return c.signal(ctx, signal.TenantID, signal.WorkflowID, signal.ProjectID, SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
		ApprovalRequestID: signal.ApprovalRequestID,
		DecisionRequestID: signal.DecisionRequestID,
		Decision:          signal.Decision,
		Payload:           signal.Payload,
		ResolvedEventID:   signal.ResolvedEventID,
	})
}

// signal delivers a signal via SignalWithStartWorkflow so a coordinator that has closed
// (e.g. it failed on an earlier activity, or hit its continue-as-new gap) is transparently
// restarted and the signal is not lost. A restarted run rebuilds its pending decision state
// from the store, so it can still process human decisions and task outcomes. When the
// coordinator is already running this behaves exactly like a plain signal.
func (c *SignalClient) signal(ctx context.Context, tenantID uuid.UUID, configuredWorkflowID string, projectID uuid.UUID, signalName string, payload any) error {
	wfID := workflowID(configuredWorkflowID, projectID.String())
	_, err := c.client.SignalWithStartWorkflow(ctx, wfID, signalName, payload, client.StartWorkflowOptions{
		ID:                    wfID,
		TaskQueue:             c.taskQueue,
		WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
	}, ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   tenantID,
		ProjectID:  projectID,
		WorkflowID: wfID,
	})
	return err
}

func workflowID(configuredWorkflowID, projectID string) string {
	if configuredWorkflowID != "" {
		return configuredWorkflowID
	}
	return fmt.Sprintf("project-coordinator:%s", projectID)
}
