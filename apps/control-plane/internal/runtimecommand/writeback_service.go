package runtimecommand

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/employee"
)

type ReceiptRepository interface {
	WithTransaction(ctx context.Context, fn func(employee.DigitalEmployeeRunRepository) error) error
	GetCommandReceiptForUpdate(ctx context.Context, tenantID uuid.UUID, commandID string) (*employee.RuntimeCommandReceipt, error)
	UpdateCommandReceipt(ctx context.Context, req employee.UpdateRuntimeCommandReceiptRequest) (*employee.RuntimeCommandReceipt, error)
}

type WritebackService struct {
	repository ReceiptRepository
}

func NewWritebackService(repository ReceiptRepository) *WritebackService {
	return &WritebackService{repository: repository}
}

func (s *WritebackService) Complete(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	if terminal.Status != employee.DigitalEmployeeRunStatusCompleted {
		return fmt.Errorf("%w: complete writeback requires completed status", employee.ErrInvalidInput)
	}
	return s.recordTerminal(ctx, identity, commandID, terminal, "completed")
}

func (s *WritebackService) Fail(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	if terminal.Status != employee.DigitalEmployeeRunStatusFailed {
		return fmt.Errorf("%w: fail writeback requires failed status", employee.ErrInvalidInput)
	}
	return s.recordTerminal(ctx, identity, commandID, terminal, "failed")
}

func (s *WritebackService) Cancel(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	if terminal.Status != employee.DigitalEmployeeRunStatusCancelled {
		return fmt.Errorf("%w: cancel writeback requires cancelled status", employee.ErrInvalidInput)
	}
	return s.recordTerminal(ctx, identity, commandID, terminal, "cancelled")
}

func (s *WritebackService) TimedOut(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	if terminal.Status != employee.DigitalEmployeeRunStatusTimedOut {
		return fmt.Errorf("%w: timed-out writeback requires timed_out status", employee.ErrInvalidInput)
	}
	terminal.TimedOut = true
	return s.recordTerminal(ctx, identity, commandID, terminal, "timed_out")
}

func (s *WritebackService) RecordEvent(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, event employee.RuntimeCommandEventWriteback) error {
	return fmt.Errorf("%w: command event writeback is not supported for generic runtime commands", employee.ErrNotFound)
}

func (s *WritebackService) recordTerminal(ctx context.Context, identity employee.RuntimeCommandWritebackIdentity, commandID string, terminal employee.RuntimeCommandTerminalWriteback, status string) error {
	if s == nil || s.repository == nil {
		return fmt.Errorf("%w: runtime command receipt repository is required", employee.ErrInvalidInput)
	}
	identity, commandID, err := validateIdentity(identity, commandID)
	if err != nil {
		return err
	}
	return s.repository.WithTransaction(ctx, func(repository employee.DigitalEmployeeRunRepository) error {
		receipt, err := repository.GetCommandReceiptForUpdate(ctx, identity.TenantID, commandID)
		if err != nil {
			return fmt.Errorf("get command receipt: %w", err)
		}
		if receipt == nil || receipt.TenantID != identity.TenantID || receipt.CommandID != commandID {
			return fmt.Errorf("%w: command receipt does not match request", employee.ErrInvalidInput)
		}
		if receipt.RuntimeNodeID != identity.RuntimeNodeID || strings.TrimSpace(receipt.NodeID) != identity.NodeID {
			return fmt.Errorf("%w: command receipt runtime identity does not match authenticated runtime", employee.ErrRuntimeIdentityMismatch)
		}
		if receipt.CommandType != "install_skills" || receipt.ResourceType != "skill" {
			return fmt.Errorf("%w: runtime command receipt is not supported by generic writeback", employee.ErrNotFound)
		}
		if isTerminalReceiptStatus(receipt.Status) && receipt.Status != status {
			return fmt.Errorf("%w: command receipt is already terminal with status %s", employee.ErrConflict, receipt.Status)
		}
		_, err = repository.UpdateCommandReceipt(ctx, employee.UpdateRuntimeCommandReceiptRequest{
			TenantID:     identity.TenantID,
			CommandID:    commandID,
			Status:       status,
			Result:       terminalResult(terminal),
			ErrorMessage: terminal.ErrorMessage,
		})
		if err != nil {
			return fmt.Errorf("update command receipt terminal status: %w", err)
		}
		return nil
	})
}

func validateIdentity(identity employee.RuntimeCommandWritebackIdentity, commandID string) (employee.RuntimeCommandWritebackIdentity, string, error) {
	if identity.TenantID == uuid.Nil {
		return employee.RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: tenant_id is required", employee.ErrInvalidInput)
	}
	if identity.RuntimeNodeID == uuid.Nil {
		return employee.RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: runtime_node_id is required", employee.ErrInvalidInput)
	}
	identity.NodeID = strings.TrimSpace(identity.NodeID)
	if identity.NodeID == "" {
		return employee.RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: node_id is required", employee.ErrInvalidInput)
	}
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return employee.RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: command_id is required", employee.ErrInvalidInput)
	}
	return identity, commandID, nil
}

func terminalResult(terminal employee.RuntimeCommandTerminalWriteback) map[string]any {
	result := map[string]any{}
	for key, value := range terminal.Result {
		result[key] = value
	}
	if strings.TrimSpace(terminal.Summary) != "" {
		result["summary"] = terminal.Summary
	}
	if terminal.Diagnostic != nil {
		result["diagnostic"] = terminal.Diagnostic
	}
	if terminal.WorkProducts != nil {
		result["work_products"] = terminal.WorkProducts
	}
	if terminal.LogRef != nil {
		result["log_ref"] = *terminal.LogRef
	}
	if terminal.RawResultRef != nil {
		result["raw_result_ref"] = *terminal.RawResultRef
	}
	if terminal.ErrorCode != nil {
		result["error_code"] = *terminal.ErrorCode
	}
	if terminal.ErrorFamily != nil {
		result["error_family"] = *terminal.ErrorFamily
	}
	if terminal.ExitCode != nil {
		result["exit_code"] = *terminal.ExitCode
	}
	if terminal.Signal != nil {
		result["signal"] = *terminal.Signal
	}
	if terminal.TimedOut {
		result["timed_out"] = true
	}
	return result
}

func isTerminalReceiptStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed", "cancelled", "timed_out":
		return true
	default:
		return false
	}
}
