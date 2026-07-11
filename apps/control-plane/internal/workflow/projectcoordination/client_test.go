package projectcoordination

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/mocks"
)

func TestSignalClientTerminateProjectCoordinator(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	tenantID := uuid.New()
	reason := "project deleted"
	configuredWorkflowID := "custom-coordinator"

	tests := []struct {
		name      string
		workflow  string
		setupMock func(*mocks.Client)
		wantErr   bool
	}{
		{
			name:     "success",
			workflow: configuredWorkflowID,
			setupMock: func(client *mocks.Client) {
				client.On(
					"TerminateWorkflow",
					mock.Anything,
					configuredWorkflowID,
					"",
					reason,
				).Return(nil).Once()
			},
		},
		{
			name:     "default workflow id",
			workflow: "",
			setupMock: func(client *mocks.Client) {
				client.On(
					"TerminateWorkflow",
					mock.Anything,
					"project-coordinator:"+projectID.String(),
					"",
					reason,
				).Return(nil).Once()
			},
		},
		{
			name:     "not found is success",
			workflow: configuredWorkflowID,
			setupMock: func(client *mocks.Client) {
				client.On(
					"TerminateWorkflow",
					mock.Anything,
					configuredWorkflowID,
					"",
					reason,
				).Return(serviceerror.NewNotFound("workflow not found")).Once()
			},
		},
		{
			name:     "already completed is success",
			workflow: configuredWorkflowID,
			setupMock: func(client *mocks.Client) {
				client.On(
					"TerminateWorkflow",
					mock.Anything,
					configuredWorkflowID,
					"",
					reason,
				).Return(serviceerror.NewFailedPrecondition("workflow execution already completed")).Once()
			},
		},
		{
			name:     "other error propagates",
			workflow: configuredWorkflowID,
			setupMock: func(client *mocks.Client) {
				client.On(
					"TerminateWorkflow",
					mock.Anything,
					configuredWorkflowID,
					"",
					reason,
				).Return(errors.New("temporal unavailable")).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mockClient := mocks.NewClient(t)
			tt.setupMock(mockClient)

			client := NewSignalClient(mockClient, "project-coordination")
			err := client.TerminateProjectCoordinator(context.Background(), project.TerminateProjectCoordinatorSignal{
				TenantID:   tenantID,
				ProjectID:  projectID,
				WorkflowID: tt.workflow,
				Reason:     reason,
			})

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsWorkflowMissingOrCompleted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "not found", err: serviceerror.NewNotFound("workflow not found"), want: true},
		{name: "already completed", err: serviceerror.NewFailedPrecondition("workflow execution already completed"), want: true},
		{name: "other", err: errors.New("temporal unavailable"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isWorkflowMissingOrCompleted(tt.err))
		})
	}
}
