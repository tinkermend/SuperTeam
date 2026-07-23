package project

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/employee"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

// RuntimeWorkspaceCommanderAdapter fans out ensure/remove project directory
// commands via the runtime connection registry and receipt wait loop.
type RuntimeWorkspaceCommanderAdapter struct {
	nodes      runtimeNodeLookup
	dispatcher runtimeCommandDispatcher
	receipts   runtimeCommandReceiptStore
}

type runtimeNodeLookup interface {
	GetNodeByID(ctx context.Context, id uuid.UUID) (runtimepkg.NodeRecord, error)
}

type runtimeCommandDispatcher interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command runtimepkg.RuntimeCommand) error
}

type runtimeCommandReceiptStore interface {
	CreateRuntimeCommandReceipt(ctx context.Context, req employee.CreateRuntimeCommandReceiptRequest) error
	WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*employee.RuntimeCommandReceipt, error)
}

func NewRuntimeWorkspaceCommanderAdapter(
	nodes runtimeNodeLookup,
	dispatcher runtimeCommandDispatcher,
	receipts runtimeCommandReceiptStore,
) RuntimeWorkspaceCommander {
	if nodes == nil || dispatcher == nil || receipts == nil {
		return nil
	}
	return &RuntimeWorkspaceCommanderAdapter{
		nodes:      nodes,
		dispatcher: dispatcher,
		receipts:   receipts,
	}
}

func (a *RuntimeWorkspaceCommanderAdapter) GetNodeByID(ctx context.Context, id uuid.UUID) (runtimepkg.NodeRecord, error) {
	return a.nodes.GetNodeByID(ctx, id)
}

func (a *RuntimeWorkspaceCommanderAdapter) IsConnected(nodeID string) bool {
	return a.dispatcher.IsConnected(nodeID)
}

func (a *RuntimeWorkspaceCommanderAdapter) Dispatch(ctx context.Context, nodeID string, command runtimepkg.RuntimeCommand) error {
	return a.dispatcher.Dispatch(ctx, nodeID, command)
}

func (a *RuntimeWorkspaceCommanderAdapter) CreateCommandReceipt(ctx context.Context, req WorkspaceCommandReceiptRequest) error {
	return a.receipts.CreateRuntimeCommandReceipt(ctx, employee.CreateRuntimeCommandReceiptRequest{
		TenantID:      req.TenantID,
		CommandID:     req.CommandID,
		CommandType:   req.CommandType,
		RuntimeNodeID: req.RuntimeNodeID,
		NodeID:        req.NodeID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		Status:        req.Status,
		Payload:       req.Payload,
		DispatchedAt:  req.DispatchedAt,
	})
}

func (a *RuntimeWorkspaceCommanderAdapter) WaitForCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*WorkspaceCommandReceipt, error) {
	receipt, err := a.receipts.WaitForRuntimeCommandCompletion(ctx, tenantID, commandID, interval)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil
	}
	return &WorkspaceCommandReceipt{
		CommandID:    receipt.CommandID,
		Status:       receipt.Status,
		ErrorMessage: receipt.ErrorMessage,
		Result:       receipt.Result,
	}, nil
}
