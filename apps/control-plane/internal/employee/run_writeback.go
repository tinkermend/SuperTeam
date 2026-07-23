package employee

import (
	"context"
	"errors"
	"fmt"
	"reflect"
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
	repository            DigitalEmployeeRunRepository
	audit                 AuditLogger
	runtimeEventRecorders []RuntimeEventRecorder
	executionLedger       ExecutionLedgerRecorder
	failureInbox          RunFailureInboxProjector
	employeeOwnerLookup   func(ctx context.Context, tenantID, employeeID uuid.UUID) (ownerUserID uuid.UUID, name string, err error)
	projectWorkspaceHook  ProjectWorkspaceCommandHook
}

// ProjectWorkspaceCommandHook observes project_workspace command terminals
// (clone) so Control Plane can flip workspace_ready_status.
type ProjectWorkspaceCommandHook interface {
	OnProjectWorkspaceCommandTerminal(ctx context.Context, receipt RuntimeCommandReceipt, success bool) error
}

func (s *DigitalEmployeeRunWritebackService) WithProjectWorkspaceCommandHook(hook ProjectWorkspaceCommandHook) *DigitalEmployeeRunWritebackService {
	s.projectWorkspaceHook = hook
	return s
}

type ExecutionLedgerRecorder interface {
	RecordProviderSessionEvent(ctx context.Context, req ProviderSessionEventLedgerRecordRequest) error
}

type ProviderSessionEventLedgerRecordRequest struct {
	TenantID               uuid.UUID
	DigitalEmployeeRunID   uuid.UUID
	ProviderSessionEventID uuid.UUID
}

func NewDigitalEmployeeRunWritebackService(repository DigitalEmployeeRunRepository, audit AuditLogger, recorders ...RuntimeEventRecorder) (*DigitalEmployeeRunWritebackService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: run repository is required", ErrInvalidInput)
	}
	return &DigitalEmployeeRunWritebackService{
		repository:            repository,
		audit:                 audit,
		runtimeEventRecorders: recorders,
	}, nil
}

func (s *DigitalEmployeeRunWritebackService) WithExecutionLedgerRecorder(recorder ExecutionLedgerRecorder) *DigitalEmployeeRunWritebackService {
	s.executionLedger = recorder
	return s
}

func (s *DigitalEmployeeRunWritebackService) WithEmployeeOwnerLookup(lookup func(ctx context.Context, tenantID, employeeID uuid.UUID) (uuid.UUID, string, error)) *DigitalEmployeeRunWritebackService {
	s.employeeOwnerLookup = lookup
	return s
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
		eventExists, err := s.repository.HasRunEventSequence(ctx, identity.TenantID, run.TaskID, run.ID, event.SequenceNumber)
		if err != nil {
			return fmt.Errorf("check existing terminal run event: %w", err)
		}
		if !eventExists {
			return fmt.Errorf("%w: run is terminal", ErrConflict)
		}
	}

	commandIDRef := commandID
	insertedTaskEvent, err := s.repository.CreateTaskEventIfAbsent(ctx, CreateRunEventRecordRequest{
		TenantID:       identity.TenantID,
		TaskID:         run.TaskID,
		RunID:          run.ID,
		EventType:      eventType,
		SequenceNumber: event.SequenceNumber,
		Payload:        cloneMap(event.Payload),
		CommandID:      &commandIDRef,
		RawEventRef:    event.RawEventRef,
		LogRef:         event.LogRef,
		Metadata:       redactRuntimeEventPayloadForPersistence(event.Metadata),
	})
	if err != nil {
		return fmt.Errorf("create task event: %w", err)
	}

	providerSessionExternalID := trimmedOptionalValue(event.ProviderSessionExternalID)
	if providerSessionExternalID == nil {
		if insertedTaskEvent {
			s.recordRuntimeCommandEventBestEffort(ctx, runtimeCommandEventRecordRequest(run, commandID, "command_event", "info", "Runtime 命令事件", runtimeCommandProviderEventPayload(eventType, event, nil)))
		}
		return nil
	}
	providerSessionUUID, err := s.upsertProviderSession(ctx, run, *providerSessionExternalID, "active", true, event.SequenceNumber, &commandID, nil, event.SessionStatePatch, event.Metadata)
	if err != nil {
		return err
	}
	providerSessionEventID, err := s.createProviderSessionEvent(ctx, run, providerSessionUUID, commandID, eventType, event.SequenceNumber, event.Payload, event.RawEventRef, event.LogRef, event.SessionStatePatch, event.Metadata)
	if err != nil {
		return err
	}
	s.recordProviderSessionEventLedgerBestEffort(ctx, ProviderSessionEventLedgerRecordRequest{
		TenantID:               run.TenantID,
		DigitalEmployeeRunID:   run.ID,
		ProviderSessionEventID: providerSessionEventID,
	})
	if insertedTaskEvent {
		s.recordRuntimeCommandEventBestEffort(ctx, runtimeCommandEventRecordRequest(run, commandID, "command_event", "info", "Runtime 命令事件", runtimeCommandProviderEventPayload(eventType, event, providerSessionExternalID)))
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
	shouldRecordRuntimeEvent := false
	var providerLedgerRequest *ProviderSessionEventLedgerRecordRequest
	if err := s.repository.WithTransaction(ctx, func(repository DigitalEmployeeRunRepository) error {
		txService := *s
		txService.repository = repository
		shouldRecord, ledgerRequest, err := txService.recordTerminalLocked(ctx, identity, commandID, terminal, spec)
		if err != nil {
			return err
		}
		shouldRecordRuntimeEvent = shouldRecord
		providerLedgerRequest = ledgerRequest
		return nil
	}); err != nil {
		return err
	}
	if providerLedgerRequest != nil {
		s.recordProviderSessionEventLedgerBestEffort(ctx, *providerLedgerRequest)
	}
	if shouldRecordRuntimeEvent {
		s.recordRuntimeTerminalEventBestEffort(ctx, identity, commandID, terminal.Status)
		if spec.status == DigitalEmployeeRunStatusFailed || spec.status == DigitalEmployeeRunStatusTimedOut {
			s.projectStandaloneFailureBestEffort(ctx, identity.TenantID, commandID)
		}
	}
	return nil
}

func (s *DigitalEmployeeRunWritebackService) recordTerminalLocked(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, terminal RuntimeCommandTerminalWriteback, spec terminalSpec) (bool, *ProviderSessionEventLedgerRecordRequest, error) {
	receipt, run, err := s.loadCommandRun(ctx, identity, commandID, true)
	if err != nil {
		return false, nil, err
	}
	if isTerminalReceiptStatus(receipt.Status) && receipt.Status != string(spec.status) {
		return false, nil, fmt.Errorf("%w: command receipt is already terminal with status %s", ErrConflict, receipt.Status)
	}
	if run == nil {
		// project_workspace 命令(ensure/remove project directory)无关联 run:
		// 只把回执置终态,供创建 fan-out 的同步等待解除阻塞。
		if receipt.ResourceType == "project_workspace" {
			if isTerminalReceiptStatus(receipt.Status) && receipt.Status != string(spec.status) {
				return false, nil, fmt.Errorf("%w: command receipt is already terminal with status %s", ErrConflict, receipt.Status)
			}
			receiptResult := terminalReceiptResult(terminal, spec.status)
			errorMessage := terminal.ErrorMessage
			if errorMessage == nil && spec.receiptErrorMsg != nil {
				errorMessage = spec.receiptErrorMsg
			}
			updated, err := s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
				TenantID:     identity.TenantID,
				CommandID:    commandID,
				Status:       string(spec.status),
				Result:       receiptResult,
				ErrorMessage: errorMessage,
			})
			if err != nil {
				return false, nil, fmt.Errorf("update project workspace command receipt: %w", err)
			}
			if s.projectWorkspaceHook != nil && updated != nil &&
				strings.TrimSpace(updated.CommandType) == "clone_project_repository" {
				success := updated.Status == "completed"
				if hookErr := s.projectWorkspaceHook.OnProjectWorkspaceCommandTerminal(ctx, *updated, success); hookErr != nil {
					return false, nil, fmt.Errorf("project workspace clone hook: %w", hookErr)
				}
			}
			return false, nil, nil
		}
		// provision_instance 与 sync_workspace_files 命令均已退役:
		// 不再存在任何合法的"无 run"命令回执。
		return false, nil, fmt.Errorf("%w: command receipt does not belong to a digital employee run", ErrNotFound)
	}
	wasTerminal := run.Status.IsTerminal()
	projectionTerminal := terminal
	updatedRun := run
	if wasTerminal {
		if run.Status != spec.status {
			return false, nil, fmt.Errorf("%w: run is already terminal with status %s", ErrConflict, run.Status)
		}
		if !terminalCompatibleWithRun(run, terminal) {
			return false, nil, fmt.Errorf("%w: terminal writeback conflicts with persisted run", ErrConflict)
		}
		projectionTerminal = terminalWritebackFromRun(run)
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
			return false, nil, fmt.Errorf("update run terminal status: %w", err)
		}
	}

	commandIDRef := commandID
	if _, err := s.repository.CreateTaskEventIfAbsent(ctx, CreateRunEventRecordRequest{
		TenantID:       identity.TenantID,
		TaskID:         run.TaskID,
		RunID:          run.ID,
		EventType:      spec.eventType,
		SequenceNumber: spec.sequenceNumber,
		Payload:        terminalEventPayload(projectionTerminal, spec.status),
		CommandID:      &commandIDRef,
		RawEventRef:    projectionTerminal.RawResultRef,
		LogRef:         projectionTerminal.LogRef,
		Metadata: map[string]any{
			"source": "runtime",
			"status": string(spec.status),
		},
	}); err != nil {
		return false, nil, fmt.Errorf("create terminal task event: %w", err)
	}

	var providerLedgerRequest *ProviderSessionEventLedgerRecordRequest
	providerSessionExternalID := trimmedOptionalValue(projectionTerminal.ProviderSessionExternalID)
	if providerSessionExternalID != nil {
		providerSessionUUID, err := s.upsertProviderSession(ctx, updatedRun, *providerSessionExternalID, spec.providerStatus, spec.recoverable, spec.sequenceNumber, &commandID, projectionTerminal.ErrorFamily, projectionTerminal.SessionStatePatch, map[string]any{"source": "runtime", "status": string(spec.status)})
		if err != nil {
			return false, nil, err
		}
		providerSessionEventID, err := s.createProviderSessionEvent(ctx, updatedRun, providerSessionUUID, commandID, spec.eventType, spec.sequenceNumber, terminalEventPayload(projectionTerminal, spec.status), projectionTerminal.RawResultRef, projectionTerminal.LogRef, projectionTerminal.SessionStatePatch, map[string]any{"source": "runtime", "status": string(spec.status)})
		if err != nil {
			return false, nil, err
		}
		providerLedgerRequest = &ProviderSessionEventLedgerRecordRequest{
			TenantID:               updatedRun.TenantID,
			DigitalEmployeeRunID:   updatedRun.ID,
			ProviderSessionEventID: providerSessionEventID,
		}
	}

	receiptResult := terminalReceiptResult(projectionTerminal, spec.status)
	if _, err := s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
		TenantID:     identity.TenantID,
		CommandID:    commandID,
		Status:       string(spec.status),
		Result:       receiptResult,
		ErrorMessage: projectionTerminal.ErrorMessage,
	}); err != nil {
		return false, nil, fmt.Errorf("update command receipt terminal status: %w", err)
	}
	if wasTerminal {
		return false, providerLedgerRequest, nil
	}
	if err := s.logRuntimeAudit(ctx, spec.auditEventType, run.NodeID, "digital_employee_run", run.ID.String(), spec.auditAction); err != nil {
		return false, nil, err
	}
	return true, providerLedgerRequest, nil
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
	if errors.Is(err, ErrNotFound) && (receipt.ResourceType == "digital_employee_execution_instance" || receipt.ResourceType == "digital_employee_workspace_sync" || receipt.ResourceType == "project_workspace") {
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

func (s *DigitalEmployeeRunWritebackService) projectStandaloneFailureBestEffort(ctx context.Context, tenantID uuid.UUID, commandID string) {
	if s.failureInbox == nil || s.employeeOwnerLookup == nil {
		return
	}
	run, err := s.repository.GetRunByCommandID(ctx, tenantID, commandID)
	if err != nil || run == nil {
		return
	}
	meta, err := s.repository.GetRunTaskMetadata(ctx, run.TenantID, run.TaskID)
	if err != nil {
		return
	}
	if projectTaskID, _ := meta["project_task_id"].(string); strings.TrimSpace(projectTaskID) != "" {
		return
	}
	ownerID, employeeName, err := s.employeeOwnerLookup(ctx, run.TenantID, run.DigitalEmployeeID)
	if err != nil || ownerID == uuid.Nil {
		return
	}
	title := objectiveFromRunMetadata(meta)
	if title == "" {
		title = "数字员工运行失败"
	}
	summary := "运行已失败，请选择重试或确认关闭。"
	if run.ErrorMessage != nil && strings.TrimSpace(*run.ErrorMessage) != "" {
		summary = strings.TrimSpace(*run.ErrorMessage)
	}
	_ = s.failureInbox.UpsertStandaloneRunFailure(ctx, StandaloneRunFailureInboxInput{
		TenantID:          run.TenantID,
		TargetUserID:      ownerID,
		RunID:             run.ID,
		DigitalEmployeeID: run.DigitalEmployeeID,
		EmployeeName:      employeeName,
		Title:             fmt.Sprintf("处理「%s」的运行失败", employeeName),
		Summary:           summary,
		RunKind:           run.RunKind,
		ProjectID:         projectIDFromRunMetadata(meta),
	})
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
		SessionState:        redactRuntimeEventPayloadForPersistence(sessionState),
		LastSequenceNumber:  sequenceNumber,
		LastCommandID:       commandID,
		LastRunID:           &runID,
		LastErrorFamily:     errorFamily,
		Metadata:            redactRuntimeEventPayloadForPersistence(metadata),
		ProjectTaskRootID:   s.resolveProjectTaskRootID(ctx, run, metadata),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("upsert provider session: %w", err)
	}
	return providerSessionUUID, nil
}

// resolveProjectTaskRootID recovers the session-lineage root task id for a
// run's provider session so it can be persisted onto provider_sessions.
// project_task_root_id. It first checks the runtime event's own metadata
// (in case a future writeback path starts echoing it back directly), then
// falls back to the metadata the control plane stamped on the task at
// dispatch time (see DigitalEmployeeRunService.StartProjectTaskRun, which
// always sets metadata["revision_root_task_id"]). A missing or unparsable
// value is treated as "not applicable" (e.g. non-project-task runs), not
// an error — the column stays null, matching pre-refactor behavior.
func (s *DigitalEmployeeRunWritebackService) resolveProjectTaskRootID(ctx context.Context, run *DigitalEmployeeRun, eventMetadata map[string]any) *uuid.UUID {
	if rootID := projectTaskRootIDFromMetadata(eventMetadata); rootID != nil {
		return rootID
	}
	taskMetadata, err := s.repository.GetRunTaskMetadata(ctx, run.TenantID, run.TaskID)
	if err != nil {
		return nil
	}
	return projectTaskRootIDFromMetadata(taskMetadata)
}

func projectTaskRootIDFromMetadata(metadata map[string]any) *uuid.UUID {
	value, ok := metadata["revision_root_task_id"].(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	rootID, err := uuid.Parse(trimmed)
	if err != nil {
		return nil
	}
	return &rootID
}

func (s *DigitalEmployeeRunWritebackService) createProviderSessionEvent(ctx context.Context, run *DigitalEmployeeRun, providerSessionUUID uuid.UUID, commandID, eventType string, sequenceNumber int32, payload map[string]any, rawEventRef, logRef *string, sessionStatePatch map[string]any, metadata map[string]any) (uuid.UUID, error) {
	commandIDRef := commandID
	providerSessionEventID, err := s.repository.CreateProviderSessionEventIfAbsent(ctx, CreateProviderSessionEventRecordRequest{
		TenantID:            run.TenantID,
		ProviderSessionUUID: providerSessionUUID,
		EventType:           eventType,
		SequenceNumber:      sequenceNumber,
		Payload:             redactRuntimeEventPayloadForPersistence(payload),
		CommandID:           &commandIDRef,
		RawEventRef:         rawEventRef,
		LogRef:              logRef,
		SessionStatePatch:   redactRuntimeEventPayloadForPersistence(sessionStatePatch),
		Metadata:            redactRuntimeEventPayloadForPersistence(metadata),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create provider session event: %w", err)
	}
	return providerSessionEventID, nil
}

func (s *DigitalEmployeeRunWritebackService) recordProviderSessionEventLedgerBestEffort(ctx context.Context, req ProviderSessionEventLedgerRecordRequest) {
	if s.executionLedger == nil {
		return
	}
	_ = s.executionLedger.RecordProviderSessionEvent(ctx, req)
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

func uuidFromPayload(payload map[string]any, key string) (uuid.UUID, error) {
	value, ok := payload[key]
	if !ok {
		return uuid.Nil, fmt.Errorf("%w: %s is required", ErrInvalidInput, key)
	}
	switch typed := value.(type) {
	case string:
		parsed, err := uuid.Parse(strings.TrimSpace(typed))
		if err != nil {
			return uuid.Nil, fmt.Errorf("%w: %s must be a UUID", ErrInvalidInput, key)
		}
		return parsed, nil
	case uuid.UUID:
		if typed == uuid.Nil {
			return uuid.Nil, fmt.Errorf("%w: %s is required", ErrInvalidInput, key)
		}
		return typed, nil
	default:
		return uuid.Nil, fmt.Errorf("%w: %s must be a UUID", ErrInvalidInput, key)
	}
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
	if !mapSubsetEqual(redactRuntimeEventPayloadForPersistence(terminal.Result), run.Result) {
		return false
	}
	if !mapSubsetEqual(redactRuntimeEventPayloadForPersistence(terminal.Diagnostic), run.Diagnostic) {
		return false
	}
	if !mapSubsetEqual(redactRuntimeEventPayloadForPersistence(terminal.SessionStatePatch), run.SessionState) {
		return false
	}
	if len(terminal.WorkProducts) > 0 && !reflect.DeepEqual(redactWorkProducts(terminal.WorkProducts), run.WorkProducts) {
		return false
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
	if terminal.ExitCode != nil && !sameOptionalInt32(run.ExitCode, terminal.ExitCode) {
		return false
	}
	if terminal.Signal != nil && !sameOptionalString(run.Signal, terminal.Signal) {
		return false
	}
	if terminal.RawResultRef != nil && !sameOptionalString(run.RawResultRef, terminal.RawResultRef) {
		return false
	}
	if terminal.LogRef != nil && !sameOptionalString(run.LogRef, terminal.LogRef) {
		return false
	}
	if terminal.ProviderSessionExternalID != nil && !sameOptionalString(run.ProviderSessionExternalID, terminal.ProviderSessionExternalID) {
		return false
	}
	if terminal.TimedOut && !run.TimedOut {
		return false
	}
	return true
}

func mapSubsetEqual(subset, superset map[string]any) bool {
	if len(subset) == 0 {
		return true
	}
	for key, value := range subset {
		if !reflect.DeepEqual(value, superset[key]) {
			return false
		}
	}
	return true
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func sameOptionalInt32(left, right *int32) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func terminalWritebackFromRun(run *DigitalEmployeeRun) RuntimeCommandTerminalWriteback {
	terminal := RuntimeCommandTerminalWriteback{
		Status:                    run.Status,
		Result:                    cloneMap(run.Result),
		Diagnostic:                cloneMap(run.Diagnostic),
		LogRef:                    run.LogRef,
		RawResultRef:              run.RawResultRef,
		WorkProducts:              append([]WorkProduct(nil), run.WorkProducts...),
		SessionStatePatch:         cloneMap(run.SessionState),
		ErrorMessage:              run.ErrorMessage,
		ErrorCode:                 run.ErrorCode,
		ErrorFamily:               run.ErrorFamily,
		ExitCode:                  run.ExitCode,
		Signal:                    run.Signal,
		ProviderSessionExternalID: run.ProviderSessionExternalID,
		TimedOut:                  run.TimedOut,
	}
	if summary, ok := run.Result["summary"].(string); ok {
		terminal.Summary = summary
	}
	return terminal
}

func terminalResult(terminal RuntimeCommandTerminalWriteback, status DigitalEmployeeRunStatus) map[string]any {
	result := redactRuntimeEventPayloadForPersistence(terminal.Result)
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
	diagnostic := redactRuntimeEventPayloadForPersistence(terminal.Diagnostic)
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
		payload["session_state_patch"] = redactRuntimeEventPayloadForPersistence(terminal.SessionStatePatch)
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
	for key, value := range redactRuntimeEventPayloadForPersistence(patch) {
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
		redacted[i].Metadata = redactRuntimeEventPayloadForPersistence(product.Metadata)
	}
	return redacted
}

func (s *DigitalEmployeeRunWritebackService) recordRuntimeTerminalEventBestEffort(ctx context.Context, identity RuntimeCommandWritebackIdentity, commandID string, status DigitalEmployeeRunStatus) {
	if len(s.runtimeEventRecorders) == 0 {
		return
	}
	run, err := s.repository.GetRunByCommandID(ctx, identity.TenantID, commandID)
	if errors.Is(err, ErrNotFound) || run == nil {
		return
	}
	if err != nil {
		fmt.Printf("load run for runtime event failed: %v\n", err)
		return
	}
	s.recordRuntimeCommandEventBestEffort(ctx, runtimeCommandEventRecordRequest(
		run,
		commandID,
		runtimeCommandTerminalEventType(status),
		runtimeCommandTerminalSeverity(status),
		runtimeCommandTerminalTitle(status),
		terminalRuntimeEventPayload(run, commandID, status),
	))
}

func (s *DigitalEmployeeRunWritebackService) recordRuntimeCommandEventBestEffort(ctx context.Context, req RuntimeEventRecordRequest) {
	for _, recorder := range s.runtimeEventRecorders {
		if recorder == nil {
			continue
		}
		if err := recorder.RecordRuntimeEvent(ctx, req); err != nil {
			fmt.Printf("record runtime command event failed: %v\n", err)
		}
	}
}

func runtimeCommandEventRecordRequest(run *DigitalEmployeeRun, commandID, eventType, severity, title string, payload map[string]any) RuntimeEventRecordRequest {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["status"] = string(run.Status)
	payload["command_id"] = commandID
	payload["run_id"] = run.ID.String()
	payload["task_id"] = run.TaskID.String()
	payload["digital_employee_id"] = run.DigitalEmployeeID.String()
	payload["execution_instance_id"] = run.ExecutionInstanceID.String()
	return RuntimeEventRecordRequest{
		TenantID:        run.TenantID,
		RuntimeNodeID:   run.RuntimeNodeID,
		NodeID:          run.NodeID,
		EventType:       eventType,
		Severity:        severity,
		Source:          "runtime_command",
		Title:           title,
		ProviderType:    run.ProviderType,
		CorrelationType: "runtime_command",
		CorrelationID:   commandID,
		Payload:         redactRuntimeEventPayloadForPersistence(payload),
	}
}

func runtimeCommandProviderEventPayload(eventType string, event RuntimeCommandEventWriteback, providerSessionExternalID *string) map[string]any {
	payload := map[string]any{
		"provider_event_type": eventType,
		"sequence_number":     event.SequenceNumber,
	}
	if providerSessionExternalID != nil {
		payload["provider_session_external_id"] = *providerSessionExternalID
	}
	if event.RawEventRef != nil {
		payload["raw_event_ref"] = *event.RawEventRef
	}
	if event.LogRef != nil {
		payload["log_ref"] = *event.LogRef
	}
	return payload
}

func terminalRuntimeEventPayload(run *DigitalEmployeeRun, commandID string, status DigitalEmployeeRunStatus) map[string]any {
	payload := map[string]any{
		"status":     string(status),
		"command_id": commandID,
	}
	if run.ProviderSessionExternalID != nil {
		payload["provider_session_external_id"] = *run.ProviderSessionExternalID
	}
	if run.RawResultRef != nil {
		payload["raw_result_ref"] = *run.RawResultRef
	}
	if run.LogRef != nil {
		payload["log_ref"] = *run.LogRef
	}
	if run.ErrorCode != nil {
		payload["error_code"] = *run.ErrorCode
	}
	if run.ErrorFamily != nil {
		payload["error_family"] = *run.ErrorFamily
	}
	if run.ExitCode != nil {
		payload["exit_code"] = *run.ExitCode
	}
	if run.Signal != nil {
		payload["signal"] = *run.Signal
	}
	if run.TimedOut {
		payload["timed_out"] = true
	}
	return payload
}

func runtimeCommandTerminalEventType(status DigitalEmployeeRunStatus) string {
	switch status {
	case DigitalEmployeeRunStatusCompleted:
		return "command_completed"
	case DigitalEmployeeRunStatusFailed:
		return "command_failed"
	case DigitalEmployeeRunStatusCancelled:
		return "command_cancelled"
	case DigitalEmployeeRunStatusTimedOut:
		return "command_timed_out"
	default:
		return "command_event"
	}
}

func runtimeCommandTerminalSeverity(status DigitalEmployeeRunStatus) string {
	switch status {
	case DigitalEmployeeRunStatusCompleted:
		return "success"
	case DigitalEmployeeRunStatusFailed, DigitalEmployeeRunStatusTimedOut:
		return "error"
	case DigitalEmployeeRunStatusCancelled:
		return "warning"
	default:
		return "info"
	}
}

func runtimeCommandTerminalTitle(status DigitalEmployeeRunStatus) string {
	switch status {
	case DigitalEmployeeRunStatusCompleted:
		return "Runtime 命令执行完成"
	case DigitalEmployeeRunStatusFailed:
		return "Runtime 命令执行失败"
	case DigitalEmployeeRunStatusCancelled:
		return "Runtime 命令执行已取消"
	case DigitalEmployeeRunStatusTimedOut:
		return "Runtime 命令执行超时"
	default:
		return "Runtime 命令事件"
	}
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
