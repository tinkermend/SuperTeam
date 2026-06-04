package employee

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	terminalCompletedSequence = int32(2147483600)
	terminalFailedSequence    = int32(2147483601)
	terminalCancelledSequence = int32(2147483602)
	terminalTimedOutSequence  = int32(2147483603)
)

type DigitalEmployeeRunWritebackService struct {
	repository DigitalEmployeeRunRepository
	audit      AuditLogger
}

func NewDigitalEmployeeRunWritebackService(repository DigitalEmployeeRunRepository, audit AuditLogger) (*DigitalEmployeeRunWritebackService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: run repository is required", ErrInvalidInput)
	}
	return &DigitalEmployeeRunWritebackService{
		repository: repository,
		audit:      audit,
	}, nil
}

func (s *DigitalEmployeeRunWritebackService) RecordEvent(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, event RuntimeCommandEventWriteback) error {
	identity, commandID, err := validateWritebackIdentity(identity, commandID)
	if err != nil {
		return err
	}
	eventType := strings.TrimSpace(event.EventType)
	if eventType == "" {
		return fmt.Errorf("%w: event_type is required", ErrInvalidInput)
	}
	if event.SequenceNumber <= 0 {
		return fmt.Errorf("%w: sequence_number is required", ErrInvalidInput)
	}

	_, run, err := s.loadCommandRun(ctx, identity, commandID, false)
	if err != nil {
		return err
	}
	if run == nil {
		return fmt.Errorf("%w: command is not associated with a run", ErrNotFound)
	}
	if run.Status.IsTerminal() {
		exists, err := s.repository.HasRunEventSequence(ctx, identity.TenantID, run.TaskID, run.ID, event.SequenceNumber)
		if err != nil {
			return fmt.Errorf("check existing terminal run event: %w", err)
		}
		if !exists {
			return fmt.Errorf("%w: run is terminal", ErrConflict)
		}
	}

	commandIDRef := commandID
	if err := s.repository.CreateTaskEventIfAbsent(ctx, CreateRunEventRecordRequest{
		TenantID:       identity.TenantID,
		TaskID:         run.TaskID,
		RunID:          run.ID,
		EventType:      eventType,
		SequenceNumber: event.SequenceNumber,
		Payload:        cloneMap(event.Payload),
		CommandID:      &commandIDRef,
		RawEventRef:    event.RawEventRef,
		LogRef:         event.LogRef,
		Metadata:       cloneMap(event.Metadata),
	}); err != nil {
		return fmt.Errorf("create task event: %w", err)
	}

	providerSessionExternalID := trimmedOptionalValue(event.ProviderSessionExternalID)
	if providerSessionExternalID == nil {
		return nil
	}
	providerSessionUUID, err := s.upsertProviderSession(ctx, run, *providerSessionExternalID, "active", true, event.SequenceNumber, &commandID, nil, event.SessionStatePatch, event.Metadata)
	if err != nil {
		return err
	}
	if err := s.createProviderSessionEvent(ctx, run, providerSessionUUID, commandID, eventType, event.SequenceNumber, event.Payload, event.RawEventRef, event.LogRef, event.SessionStatePatch, event.Metadata); err != nil {
		return err
	}
	return nil
}

func (s *DigitalEmployeeRunWritebackService) Complete(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
	if terminal.Status != DigitalEmployeeRunStatusCompleted {
		return fmt.Errorf("%w: complete writeback requires completed status", ErrInvalidInput)
	}
	return s.recordTerminal(ctx, identity, commandID, terminal, terminalSpec{
		status:          DigitalEmployeeRunStatusCompleted,
		eventType:       "run_completed",
		sequenceNumber:  terminalCompletedSequence,
		providerStatus:  "completed",
		recoverable:     false,
		auditEventType:  "digital_employee_run_completed",
		auditAction:     "employee.run.complete",
		receiptErrorMsg: nil,
	})
}

func (s *DigitalEmployeeRunWritebackService) Fail(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
	if terminal.Status != DigitalEmployeeRunStatusFailed {
		return fmt.Errorf("%w: fail writeback requires failed status", ErrInvalidInput)
	}
	return s.recordTerminal(ctx, identity, commandID, terminal, terminalSpec{
		status:         DigitalEmployeeRunStatusFailed,
		eventType:      "run_failed",
		sequenceNumber: terminalFailedSequence,
		providerStatus: "failed",
		recoverable:    false,
		auditEventType: "digital_employee_run_failed",
		auditAction:    "employee.run.fail",
	})
}

func (s *DigitalEmployeeRunWritebackService) Cancel(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
	if terminal.Status != DigitalEmployeeRunStatusCancelled {
		return fmt.Errorf("%w: cancelled writeback requires cancelled status", ErrInvalidInput)
	}
	return s.recordTerminal(ctx, identity, commandID, terminal, terminalSpec{
		status:         DigitalEmployeeRunStatusCancelled,
		eventType:      "run_cancelled",
		sequenceNumber: terminalCancelledSequence,
		providerStatus: "stopped",
		recoverable:    false,
		auditEventType: "digital_employee_run_cancelled",
		auditAction:    "employee.run.cancel",
	})
}

func (s *DigitalEmployeeRunWritebackService) TimedOut(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
	if terminal.Status != DigitalEmployeeRunStatusTimedOut {
		return fmt.Errorf("%w: timed-out writeback requires timed_out status", ErrInvalidInput)
	}
	terminal.TimedOut = true
	if terminal.ErrorFamily == nil {
		terminal.ErrorFamily = stringPtr("timeout")
	}
	return s.recordTerminal(ctx, identity, commandID, terminal, terminalSpec{
		status:         DigitalEmployeeRunStatusTimedOut,
		eventType:      "run_timed_out",
		sequenceNumber: terminalTimedOutSequence,
		providerStatus: "failed",
		recoverable:    false,
		auditEventType: "digital_employee_run_timed_out",
		auditAction:    "employee.run.timeout",
	})
}

type terminalSpec struct {
	status          DigitalEmployeeRunStatus
	eventType       string
	sequenceNumber  int32
	providerStatus  string
	recoverable     bool
	auditEventType  string
	auditAction     string
	receiptErrorMsg *string
}

func (s *DigitalEmployeeRunWritebackService) recordTerminal(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback, spec terminalSpec) error {
	identity, commandID, err := validateWritebackIdentity(identity, commandID)
	if err != nil {
		return err
	}
	return s.repository.WithTransaction(ctx, func(repository DigitalEmployeeRunRepository) error {
		txService := *s
		txService.repository = repository
		return txService.recordTerminalLocked(ctx, identity, commandID, terminal, spec)
	})
}

func (s *DigitalEmployeeRunWritebackService) recordTerminalLocked(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback, spec terminalSpec) error {
	receipt, run, err := s.loadCommandRun(ctx, identity, commandID, true)
	if err != nil {
		return err
	}
	if isTerminalReceiptStatus(receipt.Status) && receipt.Status != string(spec.status) {
		return fmt.Errorf("%w: command receipt is already terminal with status %s", ErrConflict, receipt.Status)
	}
	if run == nil {
		return s.recordProvisioningTerminal(ctx, identity, commandID, receipt, terminal, spec)
	}
	wasTerminal := run.Status.IsTerminal()
	updatedRun := run
	if wasTerminal {
		if run.Status != spec.status {
			return fmt.Errorf("%w: run is already terminal with status %s", ErrConflict, run.Status)
		}
		if !terminalCompatibleWithRun(run, terminal) {
			return fmt.Errorf("%w: terminal writeback conflicts with persisted run", ErrConflict)
		}
	} else {
		result := terminalResult(terminal, spec.status)
		diagnostic := terminalDiagnostic(terminal, spec.status)
		sessionState := mergeSessionStatePatch(run.SessionState, terminal.SessionStatePatch)
		workProducts := redactWorkProducts(terminal.WorkProducts)
		updatedRun, err = s.repository.UpdateRunStatus(ctx, UpdateRunStatusRequest{
			TenantID:                  identity.TenantID,
			RunID:                     run.ID,
			Status:                    spec.status,
			Result:                    result,
			ErrorMessage:              terminal.ErrorMessage,
			Diagnostic:                diagnostic,
			LogRef:                    terminal.LogRef,
			RawResultRef:              terminal.RawResultRef,
			WorkProducts:              workProducts,
			SessionState:              sessionState,
			ErrorCode:                 terminal.ErrorCode,
			ErrorFamily:               terminal.ErrorFamily,
			ExitCode:                  terminal.ExitCode,
			Signal:                    terminal.Signal,
			ProviderSessionExternalID: trimmedOptionalValue(terminal.ProviderSessionExternalID),
			TimedOut:                  spec.status == DigitalEmployeeRunStatusTimedOut || terminal.TimedOut,
		})
		if err != nil {
			return fmt.Errorf("update run terminal status: %w", err)
		}
	}

	commandIDRef := commandID
	if err := s.repository.CreateTaskEventIfAbsent(ctx, CreateRunEventRecordRequest{
		TenantID:       identity.TenantID,
		TaskID:         run.TaskID,
		RunID:          run.ID,
		EventType:      spec.eventType,
		SequenceNumber: spec.sequenceNumber,
		Payload:        terminalEventPayload(terminal, spec.status),
		CommandID:      &commandIDRef,
		RawEventRef:    terminal.RawResultRef,
		LogRef:         terminal.LogRef,
		Metadata: map[string]any{
			"source": "runtime",
			"status": string(spec.status),
		},
	}); err != nil {
		return fmt.Errorf("create terminal task event: %w", err)
	}

	providerSessionExternalID := trimmedOptionalValue(terminal.ProviderSessionExternalID)
	if providerSessionExternalID != nil {
		providerSessionUUID, err := s.upsertProviderSession(ctx, updatedRun, *providerSessionExternalID, spec.providerStatus, spec.recoverable, spec.sequenceNumber, &commandID, terminal.ErrorFamily, terminal.SessionStatePatch, map[string]any{"source": "runtime", "status": string(spec.status)})
		if err != nil {
			return err
		}
		if err := s.createProviderSessionEvent(ctx, updatedRun, providerSessionUUID, commandID, spec.eventType, spec.sequenceNumber, terminalEventPayload(terminal, spec.status), terminal.RawResultRef, terminal.LogRef, terminal.SessionStatePatch, map[string]any{"source": "runtime", "status": string(spec.status)}); err != nil {
			return err
		}
	}

	receiptResult := terminalReceiptResult(terminal, spec.status)
	if _, err := s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
		TenantID:     identity.TenantID,
		CommandID:    commandID,
		Status:       string(spec.status),
		Result:       receiptResult,
		ErrorMessage: terminal.ErrorMessage,
	}); err != nil {
		return fmt.Errorf("update command receipt terminal status: %w", err)
	}
	if wasTerminal {
		return nil
	}
	return s.logRuntimeAudit(ctx, spec.auditEventType, run.NodeID, "digital_employee_run", run.ID.String(), spec.auditAction)
}

func (s *DigitalEmployeeRunWritebackService) recordProvisioningTerminal(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, receipt *RuntimeCommandReceipt, terminal RuntimeCommandTerminalWriteback, spec terminalSpec) error {
	switch spec.status {
	case DigitalEmployeeRunStatusCompleted:
		return s.completeProvisioning(ctx, identity, commandID, receipt, terminal)
	case DigitalEmployeeRunStatusFailed:
		return s.failProvisioning(ctx, identity, commandID, receipt, terminal)
	default:
		return fmt.Errorf("%w: provisioning command only accepts completed or failed terminal writeback", ErrInvalidInput)
	}
}

func (s *DigitalEmployeeRunWritebackService) CompleteProvisioning(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
	if terminal.Status != DigitalEmployeeRunStatusCompleted {
		return fmt.Errorf("%w: provisioning complete writeback requires completed status", ErrInvalidInput)
	}
	identity, commandID, err := validateWritebackIdentity(identity, commandID)
	if err != nil {
		return err
	}
	return s.repository.WithTransaction(ctx, func(repository DigitalEmployeeRunRepository) error {
		txService := *s
		txService.repository = repository
		receipt, err := repository.GetCommandReceiptForUpdate(ctx, identity.TenantID, commandID)
		if err != nil {
			return fmt.Errorf("get provisioning command receipt: %w", err)
		}
		return txService.completeProvisioning(ctx, identity, commandID, receipt, terminal)
	})
}

func (s *DigitalEmployeeRunWritebackService) FailProvisioning(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback) error {
	if terminal.Status != DigitalEmployeeRunStatusFailed {
		return fmt.Errorf("%w: provisioning fail writeback requires failed status", ErrInvalidInput)
	}
	identity, commandID, err := validateWritebackIdentity(identity, commandID)
	if err != nil {
		return err
	}
	return s.repository.WithTransaction(ctx, func(repository DigitalEmployeeRunRepository) error {
		txService := *s
		txService.repository = repository
		receipt, err := repository.GetCommandReceiptForUpdate(ctx, identity.TenantID, commandID)
		if err != nil {
			return fmt.Errorf("get provisioning command receipt: %w", err)
		}
		return txService.failProvisioning(ctx, identity, commandID, receipt, terminal)
	})
}

func (s *DigitalEmployeeRunWritebackService) completeProvisioning(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, receipt *RuntimeCommandReceipt, terminal RuntimeCommandTerminalWriteback) error {
	if err := validateProvisioningReceipt(identity, commandID, receipt); err != nil {
		return err
	}
	if isTerminalReceiptStatus(receipt.Status) {
		if receipt.Status == string(DigitalEmployeeRunStatusCompleted) {
			return nil
		}
		return fmt.Errorf("%w: provisioning command receipt is already terminal with status %s", ErrConflict, receipt.Status)
	}

	instance, err := s.repository.UpdateExecutionInstanceStatus(ctx, identity.TenantID, receipt.ResourceID, ExecutionInstanceStatusReady, nil)
	if err != nil {
		return fmt.Errorf("mark execution instance ready: %w", err)
	}
	if _, err := s.repository.UpdateDigitalEmployeeStatus(ctx, identity.TenantID, instance.DigitalEmployeeID, DigitalEmployeeStatusReady); err != nil {
		return fmt.Errorf("mark digital employee ready: %w", err)
	}
	if _, err := s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
		TenantID:     identity.TenantID,
		CommandID:    commandID,
		Status:       string(DigitalEmployeeRunStatusCompleted),
		Result:       terminalReceiptResult(terminal, DigitalEmployeeRunStatusCompleted),
		ErrorMessage: nil,
	}); err != nil {
		return fmt.Errorf("update provisioning command receipt completed: %w", err)
	}
	return s.logRuntimeAudit(ctx, "digital_employee_instance_provisioned", receipt.NodeID, receipt.ResourceType, receipt.ResourceID.String(), "employee.instance.provision")
}

func (s *DigitalEmployeeRunWritebackService) failProvisioning(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, receipt *RuntimeCommandReceipt, terminal RuntimeCommandTerminalWriteback) error {
	if err := validateProvisioningReceipt(identity, commandID, receipt); err != nil {
		return err
	}
	if isTerminalReceiptStatus(receipt.Status) {
		if receipt.Status == string(DigitalEmployeeRunStatusFailed) {
			return nil
		}
		return fmt.Errorf("%w: provisioning command receipt is already terminal with status %s", ErrConflict, receipt.Status)
	}

	instance, err := s.repository.UpdateExecutionInstanceStatus(ctx, identity.TenantID, receipt.ResourceID, ExecutionInstanceStatusError, terminal.ErrorMessage)
	if err != nil {
		return fmt.Errorf("mark execution instance error: %w", err)
	}
	if err := s.repository.DeleteExecutionInstance(ctx, identity.TenantID, instance.ID); err != nil {
		return fmt.Errorf("delete failed provisioning execution instance: %w", err)
	}
	if err := s.repository.DeleteDigitalEmployee(ctx, identity.TenantID, instance.DigitalEmployeeID); err != nil {
		return fmt.Errorf("delete failed provisioning digital employee: %w", err)
	}
	if _, err := s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
		TenantID:     identity.TenantID,
		CommandID:    commandID,
		Status:       string(DigitalEmployeeRunStatusFailed),
		Result:       terminalReceiptResult(terminal, DigitalEmployeeRunStatusFailed),
		ErrorMessage: terminal.ErrorMessage,
	}); err != nil {
		return fmt.Errorf("update provisioning command receipt failed: %w", err)
	}
	return s.logRuntimeAudit(ctx, "digital_employee_instance_provision_failed", receipt.NodeID, receipt.ResourceType, receipt.ResourceID.String(), "employee.instance.provision_failed")
}

func (s *DigitalEmployeeRunWritebackService) loadCommandRun(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, forUpdate bool) (*RuntimeCommandReceipt, *DigitalEmployeeRun, error) {
	var (
		receipt *RuntimeCommandReceipt
		err     error
	)
	if forUpdate {
		receipt, err = s.repository.GetCommandReceiptForUpdate(ctx, identity.TenantID, commandID)
	} else {
		receipt, err = s.repository.GetCommandReceipt(ctx, identity.TenantID, commandID)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get command receipt: %w", err)
	}
	if receipt == nil || receipt.TenantID != identity.TenantID || receipt.CommandID != commandID {
		return nil, nil, fmt.Errorf("%w: command receipt does not match request", ErrInvalidInput)
	}
	if err := ensureReceiptRuntimeIdentity(identity, receipt); err != nil {
		return nil, nil, err
	}

	run, err := s.repository.GetRunByCommandID(ctx, identity.TenantID, commandID)
	if errors.Is(err, ErrNotFound) && receipt.ResourceType == "digital_employee_run" && receipt.ResourceID != uuid.Nil {
		run, err = s.repository.GetRunByID(ctx, identity.TenantID, receipt.ResourceID)
	}
	if errors.Is(err, ErrNotFound) && receipt.ResourceType == "digital_employee_execution_instance" {
		return receipt, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("get run for command: %w", err)
	}
	if run == nil {
		return nil, nil, fmt.Errorf("%w: command run is missing", ErrNotFound)
	}
	if run.TenantID != identity.TenantID {
		return nil, nil, fmt.Errorf("%w: run tenant does not match command", ErrInvalidInput)
	}
	if err := ensureRunRuntimeIdentity(identity, run); err != nil {
		return nil, nil, err
	}
	if run.CommandID != commandID {
		if receipt.ResourceType != "digital_employee_run" || receipt.ResourceID != run.ID {
			return nil, nil, fmt.Errorf("%w: run command does not match command receipt", ErrInvalidInput)
		}
	}
	if receipt.ResourceType == "digital_employee_run" && receipt.ResourceID != uuid.Nil && receipt.ResourceID != run.ID {
		return nil, nil, fmt.Errorf("%w: command receipt resource does not match run", ErrInvalidInput)
	}
	return receipt, run, nil
}

func (s *DigitalEmployeeRunWritebackService) upsertProviderSession(ctx context.Context, run *DigitalEmployeeRun, providerSessionExternalID, status string, recoverable bool, sequenceNumber int32, commandID *string, errorFamily *string, sessionState map[string]any, metadata map[string]any) (uuid.UUID, error) {
	runID := run.ID
	providerSessionUUID, err := s.repository.UpsertProviderSession(ctx, UpsertProviderSessionRequest{
		TenantID:            run.TenantID,
		ProviderSessionID:   providerSessionExternalID,
		DigitalEmployeeID:   run.DigitalEmployeeID,
		ExecutionInstanceID: run.ExecutionInstanceID,
		RuntimeNodeID:       run.RuntimeNodeID,
		ProviderType:        run.ProviderType,
		Status:              status,
		Recoverable:         recoverable,
		SessionState:        redactRuntimeEventPayload(sessionState),
		LastSequenceNumber:  sequenceNumber,
		LastCommandID:       commandID,
		LastRunID:           &runID,
		LastErrorFamily:     errorFamily,
		Metadata:            redactRuntimeEventPayload(metadata),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert provider session: %w", err)
	}
	return providerSessionUUID, nil
}

func (s *DigitalEmployeeRunWritebackService) createProviderSessionEvent(ctx context.Context, run *DigitalEmployeeRun, providerSessionUUID uuid.UUID, commandID, eventType string, sequenceNumber int32, payload map[string]any, rawEventRef, logRef *string, sessionStatePatch map[string]any, metadata map[string]any) error {
	commandIDRef := commandID
	if err := s.repository.CreateProviderSessionEventIfAbsent(ctx, CreateProviderSessionEventRecordRequest{
		TenantID:            run.TenantID,
		ProviderSessionUUID: providerSessionUUID,
		EventType:           eventType,
		SequenceNumber:      sequenceNumber,
		Payload:             redactRuntimeEventPayload(payload),
		CommandID:           &commandIDRef,
		RawEventRef:         rawEventRef,
		LogRef:              logRef,
		SessionStatePatch:   redactRuntimeEventPayload(sessionStatePatch),
		Metadata:            redactRuntimeEventPayload(metadata),
	}); err != nil {
		return fmt.Errorf("create provider session event: %w", err)
	}
	return nil
}

func validateWritebackIdentity(identity RuntimeCommandWritebackIdentity, commandID string) (RuntimeCommandWritebackIdentity, string, error) {
	if identity.TenantID == uuid.Nil {
		return RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if identity.RuntimeNodeID == uuid.Nil {
		return RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: runtime_node_id is required", ErrInvalidInput)
	}
	identity.NodeID = strings.TrimSpace(identity.NodeID)
	if identity.NodeID == "" {
		return RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: node_id is required", ErrInvalidInput)
	}
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return RuntimeCommandWritebackIdentity{}, "", fmt.Errorf("%w: command_id is required", ErrInvalidInput)
	}
	return identity, commandID, nil
}

func ensureReceiptRuntimeIdentity(identity RuntimeCommandWritebackIdentity, receipt *RuntimeCommandReceipt) error {
	if receipt == nil {
		return fmt.Errorf("%w: command receipt is missing", ErrNotFound)
	}
	if receipt.RuntimeNodeID != identity.RuntimeNodeID || strings.TrimSpace(receipt.NodeID) != identity.NodeID {
		return fmt.Errorf("%w: command receipt runtime identity does not match authenticated runtime", ErrRuntimeIdentityMismatch)
	}
	return nil
}

func ensureRunRuntimeIdentity(identity RuntimeCommandWritebackIdentity, run *DigitalEmployeeRun) error {
	if run == nil {
		return fmt.Errorf("%w: command run is missing", ErrNotFound)
	}
	if run.RuntimeNodeID != identity.RuntimeNodeID || strings.TrimSpace(run.NodeID) != identity.NodeID {
		return fmt.Errorf("%w: run runtime identity does not match authenticated runtime", ErrRuntimeIdentityMismatch)
	}
	return nil
}

func validateProvisioningReceipt(identity RuntimeCommandWritebackIdentity, commandID string, receipt *RuntimeCommandReceipt) error {
	if receipt == nil {
		return fmt.Errorf("%w: provisioning command receipt is missing", ErrNotFound)
	}
	if receipt.TenantID != identity.TenantID || receipt.CommandID != commandID {
		return fmt.Errorf("%w: provisioning command receipt does not match request", ErrInvalidInput)
	}
	if err := ensureReceiptRuntimeIdentity(identity, receipt); err != nil {
		return err
	}
	if receipt.ResourceType != "digital_employee_execution_instance" || receipt.ResourceID == uuid.Nil {
		return fmt.Errorf("%w: command receipt is not a digital employee execution instance provisioning command", ErrInvalidInput)
	}
	return nil
}

func isTerminalReceiptStatus(status string) bool {
	switch status {
	case string(DigitalEmployeeRunStatusCompleted), string(DigitalEmployeeRunStatusFailed), string(DigitalEmployeeRunStatusCancelled), string(DigitalEmployeeRunStatusTimedOut):
		return true
	default:
		return false
	}
}

func terminalCompatibleWithRun(run *DigitalEmployeeRun, terminal RuntimeCommandTerminalWriteback) bool {
	if terminal.Summary != "" {
		summary, ok := run.Result["summary"].(string)
		if !ok || summary != terminal.Summary {
			return false
		}
	}
	if terminal.ErrorMessage != nil && !sameOptionalString(run.ErrorMessage, terminal.ErrorMessage) {
		return false
	}
	if terminal.ErrorCode != nil && !sameOptionalString(run.ErrorCode, terminal.ErrorCode) {
		return false
	}
	if terminal.ErrorFamily != nil && !sameOptionalString(run.ErrorFamily, terminal.ErrorFamily) {
		return false
	}
	if terminal.ProviderSessionExternalID != nil && run.ProviderSessionExternalID != nil && *terminal.ProviderSessionExternalID != *run.ProviderSessionExternalID {
		return false
	}
	return true
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func terminalResult(terminal RuntimeCommandTerminalWriteback, status DigitalEmployeeRunStatus) map[string]any {
	result := redactRuntimeEventPayload(terminal.Result)
	if terminal.Summary != "" {
		result["summary"] = terminal.Summary
	}
	result["status"] = string(status)
	if status == DigitalEmployeeRunStatusTimedOut || terminal.TimedOut {
		result["timed_out"] = true
	}
	return result
}

func terminalDiagnostic(terminal RuntimeCommandTerminalWriteback, status DigitalEmployeeRunStatus) map[string]any {
	diagnostic := redactRuntimeEventPayload(terminal.Diagnostic)
	if terminal.ErrorCode != nil {
		diagnostic["error_code"] = *terminal.ErrorCode
	}
	if terminal.ErrorFamily != nil {
		diagnostic["error_family"] = *terminal.ErrorFamily
	}
	if terminal.ExitCode != nil {
		diagnostic["exit_code"] = *terminal.ExitCode
	}
	if terminal.Signal != nil {
		diagnostic["signal"] = *terminal.Signal
	}
	if status == DigitalEmployeeRunStatusTimedOut || terminal.TimedOut {
		diagnostic["timed_out"] = true
	}
	return diagnostic
}

func terminalEventPayload(terminal RuntimeCommandTerminalWriteback, status DigitalEmployeeRunStatus) map[string]any {
	payload := terminalReceiptResult(terminal, status)
	if len(terminal.WorkProducts) > 0 {
		payload["work_products"] = redactWorkProducts(terminal.WorkProducts)
	}
	if terminal.SessionStatePatch != nil {
		payload["session_state_patch"] = redactRuntimeEventPayload(terminal.SessionStatePatch)
	}
	return payload
}

func terminalReceiptResult(terminal RuntimeCommandTerminalWriteback, status DigitalEmployeeRunStatus) map[string]any {
	result := terminalResult(terminal, status)
	if diagnostic := terminalDiagnostic(terminal, status); len(diagnostic) > 0 {
		result["diagnostic"] = diagnostic
	}
	if terminal.RawResultRef != nil {
		result["raw_result_ref"] = *terminal.RawResultRef
	}
	if terminal.LogRef != nil {
		result["log_ref"] = *terminal.LogRef
	}
	if terminal.ErrorMessage != nil {
		result["error_message"] = *terminal.ErrorMessage
	}
	return result
}

func mergeSessionStatePatch(existing, patch map[string]any) map[string]any {
	if patch == nil {
		return nil
	}
	merged := cloneMap(existing)
	for key, value := range redactRuntimeEventPayload(patch) {
		merged[key] = value
	}
	return merged
}

func redactWorkProducts(products []WorkProduct) []WorkProduct {
	if products == nil {
		return nil
	}
	redacted := make([]WorkProduct, len(products))
	for i, product := range products {
		redacted[i] = product
		redacted[i].Metadata = redactRuntimeEventPayload(product.Metadata)
	}
	return redacted
}

func (s *DigitalEmployeeRunWritebackService) logRuntimeAudit(ctx context.Context, eventType, actorID, resourceType, resourceID, action string) error {
	if s.audit == nil {
		return nil
	}
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		actorID = "runtime"
	}
	if err := s.audit.LogEvent(ctx, eventType, "runtime", actorID, resourceType, resourceID, action); err != nil {
		return fmt.Errorf("log audit event: %w", err)
	}
	return nil
}
