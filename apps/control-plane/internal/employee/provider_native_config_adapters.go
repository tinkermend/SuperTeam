package employee

import (
	"context"
	"time"

	"github.com/google/uuid"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

// NativeConfigCommanderAdapter dispatches provider-native config commands and waits on receipts.
type NativeConfigCommanderAdapter struct {
	dispatcher runtimeCommandDispatcher
	receipts   runtimeCommandReceiptStore
}

type runtimeCommandDispatcher interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command runtimepkg.RuntimeCommand) error
}

type runtimeCommandReceiptStore interface {
	CreateRuntimeCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error
	WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error)
	UpdateCommandReceipt(ctx context.Context, req UpdateRuntimeCommandReceiptRequest) (*RuntimeCommandReceipt, error)
}

func NewNativeConfigCommanderAdapter(
	dispatcher runtimeCommandDispatcher,
	receipts runtimeCommandReceiptStore,
) runtimepkg.NativeConfigCommander {
	if dispatcher == nil || receipts == nil {
		return nil
	}
	return &NativeConfigCommanderAdapter{
		dispatcher: dispatcher,
		receipts:   receipts,
	}
}

func (a *NativeConfigCommanderAdapter) IsConnected(nodeID string) bool {
	return a.dispatcher.IsConnected(nodeID)
}

func (a *NativeConfigCommanderAdapter) Dispatch(ctx context.Context, nodeID string, command runtimepkg.RuntimeCommand) error {
	return a.dispatcher.Dispatch(ctx, nodeID, command)
}

func (a *NativeConfigCommanderAdapter) CreateCommandReceipt(ctx context.Context, req runtimepkg.NativeConfigCommandReceiptRequest) error {
	return a.receipts.CreateRuntimeCommandReceipt(ctx, CreateRuntimeCommandReceiptRequest{
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

func (a *NativeConfigCommanderAdapter) WaitForCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*runtimepkg.NativeConfigCommandReceipt, error) {
	receipt, err := a.receipts.WaitForRuntimeCommandCompletion(ctx, tenantID, commandID, interval)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		return nil, nil
	}
	return &runtimepkg.NativeConfigCommandReceipt{
		CommandID:    receipt.CommandID,
		Status:       receipt.Status,
		ErrorMessage: receipt.ErrorMessage,
		Result:       receipt.Result,
	}, nil
}

func (a *NativeConfigCommanderAdapter) MarkCommandTimedOut(ctx context.Context, tenantID uuid.UUID, commandID, message string) error {
	// CAS on pending only (UpdateCommandReceipt timed_out path): late complete/fail wins.
	msg := message
	_, err := a.receipts.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
		TenantID:     tenantID,
		CommandID:    commandID,
		Status:       "timed_out",
		ErrorMessage: &msg,
		Result: map[string]any{
			"status":  "timed_out",
			"summary": "provider native config command wait timed out",
		},
	})
	return err
}

// ProviderNativeConfigWritebackAdapter persists encrypted snapshots on command terminal.
type ProviderNativeConfigWritebackAdapter struct {
	apply func(ctx context.Context, tenantID, runtimeNodeID uuid.UUID, nodeID, commandType string, result map[string]any, success bool) error
}

func NewProviderNativeConfigWritebackAdapter(service *runtimepkg.Service) *ProviderNativeConfigWritebackAdapter {
	if service == nil {
		return nil
	}
	return &ProviderNativeConfigWritebackAdapter{
		apply: service.ApplyNativeConfigWriteback,
	}
}

func (a *ProviderNativeConfigWritebackAdapter) OnProviderNativeConfigTerminal(
	ctx context.Context,
	receipt RuntimeCommandReceipt,
	terminal RuntimeCommandTerminalWriteback,
	success bool,
) error {
	if a == nil || a.apply == nil {
		return nil
	}
	result := terminal.Result
	if result == nil {
		result = map[string]any{}
	}
	return a.apply(ctx, receipt.TenantID, receipt.RuntimeNodeID, receipt.NodeID, receipt.CommandType, result, success)
}
