package project

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

const (
	runtimeCommandEnsureProjectDirectory   = "ensure_project_directory"
	runtimeCommandRemoveProjectDirectory   = "remove_project_directory"
	runtimeCommandCloneProjectRepository   = "clone_project_repository"
	runtimeCommandValidateProjectWorkspace = "validate_project_workspace"
	projectWorkspaceResourceType           = "project_workspace"
	defaultWorkspaceProvisionTimeout       = 45 * time.Second
	defaultWorkspaceValidateTimeout        = 20 * time.Second
	defaultWorkspaceProvisionPoll          = 250 * time.Millisecond
)

// RuntimeWorkspaceCommander 向关联 Runtime 节点 fan-out 项目目录 mkdir/rmdir。
type RuntimeWorkspaceCommander interface {
	GetNodeByID(ctx context.Context, id uuid.UUID) (runtimepkg.NodeRecord, error)
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command runtimepkg.RuntimeCommand) error
	CreateCommandReceipt(ctx context.Context, req WorkspaceCommandReceiptRequest) error
	WaitForCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*WorkspaceCommandReceipt, error)
}

type WorkspaceCommandReceiptRequest struct {
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceType  string
	ResourceID    uuid.UUID
	Status        string
	Payload       map[string]any
	DispatchedAt  *time.Time
}

type WorkspaceCommandReceipt struct {
	CommandID    string
	Status       string
	ErrorMessage *string
	Result       map[string]any
}

func (s *Service) SetRuntimeWorkspaceCommander(commander RuntimeWorkspaceCommander) {
	s.workspaceCommander = commander
}

func (s *Service) ensureProjectDirectoriesOnNodes(ctx context.Context, tenantID, projectID uuid.UUID, projectName string, runtimeNodeIDs []uuid.UUID) error {
	if s.workspaceCommander == nil {
		// 单测与未接线环境跳过磁盘 fan-out;生产装配必须注入 commander。
		slog.Default().Warn("skip project directory ensure: workspace commander not configured",
			"project_id", projectID.String(),
			"project_name", projectName,
		)
		return nil
	}
	succeeded := make([]uuid.UUID, 0, len(runtimeNodeIDs))
	for _, runtimeNodeID := range runtimeNodeIDs {
		if err := s.dispatchProjectDirectoryCommand(ctx, tenantID, projectID, projectName, runtimeNodeID, runtimeCommandEnsureProjectDirectory); err != nil {
			compensateErr := s.compensateProjectDirectories(ctx, tenantID, projectID, projectName, succeeded)
			if compensateErr != nil {
				slog.Default().Error("project workspace mkdir rollback incomplete",
					"project_id", projectID.String(),
					"project_name", projectName,
					"mkdir_error", err.Error(),
					"compensate_error", compensateErr.Error(),
				)
			}
			return err
		}
		succeeded = append(succeeded, runtimeNodeID)
	}
	return nil
}

func (s *Service) removeProjectDirectoriesOnNodes(ctx context.Context, tenantID, projectID uuid.UUID, projectName string, runtimeNodeIDs []uuid.UUID) error {
	if s.workspaceCommander == nil {
		slog.Default().Warn("skip project directory cleanup: workspace commander not configured",
			"project_id", projectID.String(),
			"project_name", projectName,
		)
		return nil
	}
	var firstErr error
	for _, runtimeNodeID := range runtimeNodeIDs {
		if err := s.dispatchProjectDirectoryCommand(ctx, tenantID, projectID, projectName, runtimeNodeID, runtimeCommandRemoveProjectDirectory); err != nil {
			slog.Default().Error("project workspace directory cleanup failed",
				"project_id", projectID.String(),
				"project_name", projectName,
				"runtime_node_id", runtimeNodeID.String(),
				"error", err.Error(),
			)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *Service) compensateProjectDirectories(ctx context.Context, tenantID, projectID uuid.UUID, projectName string, runtimeNodeIDs []uuid.UUID) error {
	return s.removeProjectDirectoriesOnNodes(ctx, tenantID, projectID, projectName, runtimeNodeIDs)
}

func (s *Service) dispatchProjectDirectoryCommand(
	ctx context.Context,
	tenantID, projectID uuid.UUID,
	projectName string,
	runtimeNodeID uuid.UUID,
	commandType string,
) error {
	node, err := s.workspaceCommander.GetNodeByID(ctx, runtimeNodeID)
	if err != nil {
		return fmt.Errorf("%w: resolve runtime node %s: %v", ErrProjectWorkspaceProvision, runtimeNodeID, err)
	}
	if node.TenantID != tenantID {
		return fmt.Errorf("%w: runtime node tenant mismatch", ErrProjectWorkspaceProvision)
	}
	nodeID := strings.TrimSpace(node.NodeID)
	if nodeID == "" {
		return fmt.Errorf("%w: runtime node %s has empty node_id", ErrProjectWorkspaceProvision, runtimeNodeID)
	}
	if !s.workspaceCommander.IsConnected(nodeID) {
		return fmt.Errorf("%w: runtime node %s (%s) is not connected", ErrProjectWorkspaceProvision, node.Name, nodeID)
	}

	commandID := uuid.NewString()
	payload := map[string]any{
		"project_id":   projectID.String(),
		"project_name": projectName,
	}
	now := time.Now().UTC()
	if err := s.workspaceCommander.CreateCommandReceipt(ctx, WorkspaceCommandReceiptRequest{
		TenantID:      tenantID,
		CommandID:     commandID,
		CommandType:   commandType,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        nodeID,
		ResourceType:  projectWorkspaceResourceType,
		ResourceID:    projectID,
		Status:        "pending",
		Payload:       payload,
		DispatchedAt:  &now,
	}); err != nil {
		return fmt.Errorf("%w: create command receipt: %v", ErrProjectWorkspaceProvision, err)
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: encode command payload: %v", ErrProjectWorkspaceProvision, err)
	}
	if err := s.workspaceCommander.Dispatch(ctx, nodeID, runtimepkg.RuntimeCommand{
		ID:      commandID,
		Type:    commandType,
		Payload: rawPayload,
	}); err != nil {
		return fmt.Errorf("%w: dispatch %s to %s: %v", ErrProjectWorkspaceProvision, commandType, nodeID, err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, defaultWorkspaceProvisionTimeout)
	defer cancel()
	return s.waitProjectWorkspaceCommand(waitCtx, tenantID, commandID, commandType, nodeID)
}

func (s *Service) dispatchProjectWorkspaceCommand(
	ctx context.Context,
	tenantID, projectID uuid.UUID,
	runtimeNodeID uuid.UUID,
	commandType string,
	payload map[string]any,
	timeout time.Duration,
	wait bool,
) error {
	node, err := s.workspaceCommander.GetNodeByID(ctx, runtimeNodeID)
	if err != nil {
		return fmt.Errorf("%w: resolve runtime node %s: %v", ErrProjectWorkspaceProvision, runtimeNodeID, err)
	}
	if node.TenantID != tenantID {
		return fmt.Errorf("%w: runtime node tenant mismatch", ErrProjectWorkspaceProvision)
	}
	nodeID := strings.TrimSpace(node.NodeID)
	if nodeID == "" {
		return fmt.Errorf("%w: runtime node %s has empty node_id", ErrProjectWorkspaceProvision, runtimeNodeID)
	}
	if !s.workspaceCommander.IsConnected(nodeID) {
		return fmt.Errorf("%w: runtime node %s (%s) is not connected", ErrProjectWorkspaceProvision, node.Name, nodeID)
	}

	commandID := uuid.NewString()
	if payload == nil {
		payload = map[string]any{}
	}
	now := time.Now().UTC()
	if err := s.workspaceCommander.CreateCommandReceipt(ctx, WorkspaceCommandReceiptRequest{
		TenantID:      tenantID,
		CommandID:     commandID,
		CommandType:   commandType,
		RuntimeNodeID: runtimeNodeID,
		NodeID:        nodeID,
		ResourceType:  projectWorkspaceResourceType,
		ResourceID:    projectID,
		Status:        "pending",
		Payload:       payload,
		DispatchedAt:  &now,
	}); err != nil {
		return fmt.Errorf("%w: create command receipt: %v", ErrProjectWorkspaceProvision, err)
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: encode command payload: %v", ErrProjectWorkspaceProvision, err)
	}
	if err := s.workspaceCommander.Dispatch(ctx, nodeID, runtimepkg.RuntimeCommand{
		ID:      commandID,
		Type:    commandType,
		Payload: rawPayload,
	}); err != nil {
		return fmt.Errorf("%w: dispatch %s to %s: %v", ErrProjectWorkspaceProvision, commandType, nodeID, err)
	}
	if !wait {
		return nil
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return s.waitProjectWorkspaceCommand(waitCtx, tenantID, commandID, commandType, nodeID)
}

func (s *Service) waitProjectWorkspaceCommand(ctx context.Context, tenantID uuid.UUID, commandID, commandType, nodeID string) error {
	receipt, err := s.workspaceCommander.WaitForCommandCompletion(ctx, tenantID, commandID, defaultWorkspaceProvisionPoll)
	if err != nil {
		return fmt.Errorf("%w: wait for %s on %s: %v", ErrProjectWorkspaceProvision, commandType, nodeID, err)
	}
	if receipt == nil {
		return fmt.Errorf("%w: missing receipt for %s on %s", ErrProjectWorkspaceProvision, commandType, nodeID)
	}
	switch receipt.Status {
	case "completed":
		return nil
	case "failed", "timed_out", "cancelled":
		msg := receipt.Status
		if receipt.ErrorMessage != nil && strings.TrimSpace(*receipt.ErrorMessage) != "" {
			msg = strings.TrimSpace(*receipt.ErrorMessage)
		}
		return fmt.Errorf("%w: %s on node %s: %s", ErrProjectWorkspaceProvision, commandType, nodeID, msg)
	default:
		return fmt.Errorf("%w: %s on node %s ended with status %q", ErrProjectWorkspaceProvision, commandType, nodeID, receipt.Status)
	}
}

func (s *Service) validateProjectWorkspaceOnNode(
	ctx context.Context,
	tenantID, projectID uuid.UUID,
	projectName string,
	runtimeNodeID uuid.UUID,
	requireGit bool,
) error {
	if s.workspaceCommander == nil {
		return nil
	}
	payload := map[string]any{
		"project_id":   projectID.String(),
		"project_name": projectName,
		"require_git":  requireGit,
	}
	return s.dispatchProjectWorkspaceCommand(
		ctx,
		tenantID,
		projectID,
		runtimeNodeID,
		runtimeCommandValidateProjectWorkspace,
		payload,
		defaultWorkspaceValidateTimeout,
		true,
	)
}
