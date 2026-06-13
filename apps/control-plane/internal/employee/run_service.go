package employee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	cpruntime "github.com/superteam/control-plane/internal/runtime"
)

const (
	providerRunProtocol            = "provider-run/v1"
	runDispatchedLifecycleSequence = -1
	stopRequestedLifecycleSequence = -2
)

type RuntimeCommandDispatcher interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command cpruntime.RuntimeCommand) error
}

type AuditLogger interface {
	LogEvent(ctx context.Context, eventType, actorType, actorID, resourceType, resourceID, action string) error
}

type DigitalEmployeeRunService struct {
	repository DigitalEmployeeRunRepository
	dispatcher RuntimeCommandDispatcher
	audit      AuditLogger
}

func NewDigitalEmployeeRunService(repository DigitalEmployeeRunRepository, dispatcher RuntimeCommandDispatcher, audit AuditLogger) (*DigitalEmployeeRunService, error) {
	if repository == nil {
		return nil, fmt.Errorf("%w: run repository is required", ErrInvalidInput)
	}
	if dispatcher == nil {
		return nil, fmt.Errorf("%w: runtime command dispatcher is required", ErrInvalidInput)
	}
	return &DigitalEmployeeRunService{
		repository: repository,
		dispatcher: dispatcher,
		audit:      audit,
	}, nil
}

func (s *DigitalEmployeeRunService) CreateRun(ctx context.Context, req CreateDigitalEmployeeRunRequest) (*DigitalEmployeeRun, error) {
	objective := strings.TrimSpace(req.Objective)
	prompt := strings.TrimSpace(req.Prompt)
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if objective == "" {
		return nil, fmt.Errorf("%w: objective is required", ErrInvalidInput)
	}

	preflight, err := s.repository.GetRunPreflight(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get run preflight: %w", err)
	}
	if err := validateRunPreflight(preflight); err != nil {
		return nil, err
	}
	if err := validateDailyTokenBudget(preflight); err != nil {
		return nil, err
	}
	if !s.dispatcher.IsConnected(preflight.NodeID) {
		return nil, fmt.Errorf("%w: runtime node is not connected", ErrRuntimeUnavailable)
	}

	idempotencyKey := trimmedOptionalValue(req.IdempotencyKey)
	fingerprint, err := computeRunIdempotencyFingerprint(req, objective, prompt, preflight)
	if err != nil {
		return nil, err
	}

	activeRun, err := s.repository.GetActiveRun(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("get active run: %w", err)
	}
	if activeRun != nil {
		reconciledRun, reconciled, err := s.reconcileTerminalReceipt(ctx, req.TenantID, activeRun)
		if err != nil {
			return nil, err
		}
		if reconciled && reconciledRun.Status.IsTerminal() {
			if sameIdempotentRun(reconciledRun, idempotencyKey, fingerprint) {
				return reconciledRun, nil
			}
			activeRun = nil
		}
	}
	if activeRun != nil {
		if sameIdempotentRun(activeRun, idempotencyKey, fingerprint) {
			return s.dispatchStartSession(ctx, req, objective, prompt, preflight, activeRun)
		}
		return nil, fmt.Errorf("%w: active digital employee run exists", ErrConflict)
	}

	commandID := newRuntimeCommandID()
	createReq := CreateRunRecordRequest{
		IdempotencyKey:         idempotencyKey,
		IdempotencyFingerprint: &fingerprint,
		TenantID:               req.TenantID,
		DigitalEmployeeID:      req.DigitalEmployeeID,
		TeamID:                 preflight.TeamID,
		Title:                  objective,
		Description:            stringPtrIfNotEmpty(prompt),
		Priority:               0,
		ProviderType:           preflight.ProviderType,
		CreatorID:              &req.UserID,
		TargetNodeID:           preflight.NodeID,
		Params:                 buildRunParams(req, objective, prompt, preflight, fingerprint),
		NodeID:                 preflight.NodeID,
		RuntimeNodeID:          preflight.RuntimeNodeID,
		RunStatus:              DigitalEmployeeRunStatusQueued,
		CommandID:              commandID,
		ExecutionInstanceID:    preflight.ExecutionInstanceID,
		TimeoutSec:             req.TimeoutSec,
		GraceSec:               req.GraceSec,
	}

	run, err := s.repository.CreateRun(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create digital employee run: %w", err)
	}

	return s.dispatchStartSession(ctx, req, objective, prompt, preflight, run)
}

func (s *DigitalEmployeeRunService) dispatchStartSession(ctx context.Context, req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight, run *DigitalEmployeeRun) (*DigitalEmployeeRun, error) {
	if run.Status.IsTerminal() || run.Status == DigitalEmployeeRunStatusRunning || run.Status == DigitalEmployeeRunStatusCancelling {
		return run, nil
	}

	payload := buildStartSessionPayload(req, objective, prompt, preflight, run)
	receipt, err := s.repository.GetCommandReceipt(ctx, req.TenantID, run.CommandID)
	if err != nil {
		if !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("get runtime command receipt: %w", err)
		}
		if err := s.repository.CreateCommandReceipt(ctx, CreateRuntimeCommandReceiptRequest{
			TenantID:      req.TenantID,
			CommandID:     run.CommandID,
			CommandType:   "start_session",
			RuntimeNodeID: preflight.RuntimeNodeID,
			NodeID:        preflight.NodeID,
			ResourceType:  "digital_employee_run",
			ResourceID:    run.ID,
			Status:        "pending",
			Payload:       payload,
		}); err != nil {
			return nil, fmt.Errorf("create runtime command receipt: %w", err)
		}
	} else if receipt != nil {
		switch receipt.Status {
		case "dispatched":
			if run.Status == DigitalEmployeeRunStatusQueued {
				return s.markRunDispatched(ctx, req, preflight, run)
			}
			return run, nil
		case "completed", "cancelled", "timed_out":
			return run, nil
		case "failed":
			failedRun, updateErr := s.repository.UpdateRunStatus(ctx, UpdateRunStatusRequest{
				TenantID:     req.TenantID,
				RunID:        run.ID,
				Status:       DigitalEmployeeRunStatusFailed,
				ErrorMessage: receipt.ErrorMessage,
				ErrorCode:    stringPtr("dispatch_failed"),
				ErrorFamily:  stringPtr("dispatch_failed"),
			})
			if updateErr != nil {
				return nil, fmt.Errorf("mark run failed from failed command receipt: %w", updateErr)
			}
			return failedRun, nil
		}
	}

	command, err := runtimeCommand(run.CommandID, "start_session", payload)
	if err != nil {
		return nil, err
	}
	if err := s.dispatcher.Dispatch(ctx, preflight.NodeID, command); err != nil {
		_, _ = s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
			TenantID:     req.TenantID,
			CommandID:    run.CommandID,
			Status:       "failed",
			ErrorMessage: stringPtr(err.Error()),
		})
		_, _ = s.repository.UpdateRunStatus(ctx, UpdateRunStatusRequest{
			TenantID:     req.TenantID,
			RunID:        run.ID,
			Status:       DigitalEmployeeRunStatusFailed,
			ErrorMessage: stringPtr(err.Error()),
			ErrorCode:    stringPtr("dispatch_failed"),
			ErrorFamily:  stringPtr("dispatch_failed"),
		})
		_ = s.logAudit(ctx, "digital_employee_run_dispatch_failed", req.UserID, run.ID, "employee.run.create")
		return nil, fmt.Errorf("%w: dispatch start session: %w", ErrRuntimeUnavailable, err)
	}

	if _, err := s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
		TenantID:  req.TenantID,
		CommandID: run.CommandID,
		Status:    "dispatched",
		Result:    map[string]any{"dispatched": true},
	}); err != nil {
		return nil, fmt.Errorf("mark command receipt dispatched: %w", err)
	}
	return s.markRunDispatched(ctx, req, preflight, run)
}

func (s *DigitalEmployeeRunService) markRunDispatched(ctx context.Context, req CreateDigitalEmployeeRunRequest, preflight RunPreflight, run *DigitalEmployeeRun) (*DigitalEmployeeRun, error) {
	dispatchedRun, err := s.repository.UpdateRunStatus(ctx, UpdateRunStatusRequest{
		TenantID: req.TenantID,
		RunID:    run.ID,
		Status:   DigitalEmployeeRunStatusDispatching,
	})
	if err != nil {
		return nil, fmt.Errorf("mark run dispatching: %w", err)
	}
	if _, err := s.repository.CreateTaskEventIfAbsent(ctx, CreateRunEventRecordRequest{
		TenantID:       req.TenantID,
		TaskID:         run.TaskID,
		RunID:          run.ID,
		EventType:      "run_dispatched",
		SequenceNumber: runDispatchedLifecycleSequence,
		Payload: map[string]any{
			"command_id": run.CommandID,
			"node_id":    preflight.NodeID,
		},
		CommandID: &run.CommandID,
		Metadata:  map[string]any{"source": "control-plane"},
	}); err != nil {
		return nil, fmt.Errorf("append run dispatched event: %w", err)
	}
	if err := s.logAudit(ctx, "digital_employee_run_created", req.UserID, run.ID, "employee.run.create"); err != nil {
		return nil, err
	}
	return dispatchedRun, nil
}

func (s *DigitalEmployeeRunService) StopRun(ctx context.Context, req StopDigitalEmployeeRunRequest) (*DigitalEmployeeRun, error) {
	if req.TenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.UserID == uuid.Nil {
		return nil, fmt.Errorf("%w: user_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.RunID == uuid.Nil {
		return nil, fmt.Errorf("%w: run_id is required", ErrInvalidInput)
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: reason is required", ErrInvalidInput)
	}

	run, err := s.repository.GetRun(ctx, req.TenantID, req.DigitalEmployeeID, req.RunID)
	if err != nil {
		return nil, fmt.Errorf("get digital employee run: %w", err)
	}
	if run.TenantID != req.TenantID || run.DigitalEmployeeID != req.DigitalEmployeeID {
		return nil, fmt.Errorf("%w: run does not belong to digital employee", ErrInvalidInput)
	}
	if run.Status == DigitalEmployeeRunStatusCancelling {
		return nil, fmt.Errorf("%w: run is already cancelling", ErrConflict)
	}
	if !run.Status.IsActive() {
		return nil, fmt.Errorf("%w: run is not active", ErrInvalidInput)
	}

	cancellingRun, err := s.repository.UpdateRunStatus(ctx, UpdateRunStatusRequest{
		TenantID: req.TenantID,
		RunID:    run.ID,
		Status:   DigitalEmployeeRunStatusCancelling,
	})
	if err != nil {
		return nil, fmt.Errorf("mark run cancelling: %w", err)
	}
	stopCommandID := newRuntimeCommandID()
	payload := buildStopSessionPayload(run, stopCommandID, reason)
	if err := s.repository.CreateCommandReceipt(ctx, CreateRuntimeCommandReceiptRequest{
		TenantID:      req.TenantID,
		CommandID:     stopCommandID,
		CommandType:   "stop_session",
		RuntimeNodeID: run.RuntimeNodeID,
		NodeID:        run.NodeID,
		ResourceType:  "digital_employee_run",
		ResourceID:    run.ID,
		Status:        "pending",
		Payload:       payload,
	}); err != nil {
		return nil, fmt.Errorf("create stop command receipt: %w", err)
	}
	command, err := runtimeCommand(stopCommandID, "stop_session", payload)
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.CreateTaskEventIfAbsent(ctx, CreateRunEventRecordRequest{
		TenantID:       req.TenantID,
		TaskID:         run.TaskID,
		RunID:          run.ID,
		EventType:      "stop_requested",
		SequenceNumber: stopRequestedLifecycleSequence,
		Payload: map[string]any{
			"command_id":       stopCommandID,
			"start_command_id": run.CommandID,
			"reason":           reason,
		},
		CommandID: &stopCommandID,
		Metadata:  map[string]any{"source": "control-plane"},
	}); err != nil {
		return nil, fmt.Errorf("append stop requested event: %w", err)
	}
	if err := s.dispatcher.Dispatch(ctx, run.NodeID, command); err != nil {
		_, _ = s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
			TenantID:     req.TenantID,
			CommandID:    stopCommandID,
			Status:       "failed",
			ErrorMessage: stringPtr(err.Error()),
		})
		return nil, fmt.Errorf("%w: dispatch stop session: %w", ErrRuntimeUnavailable, err)
	}
	if _, err := s.repository.UpdateCommandReceipt(ctx, UpdateRuntimeCommandReceiptRequest{
		TenantID:  req.TenantID,
		CommandID: stopCommandID,
		Status:    "dispatched",
		Result:    map[string]any{"dispatched": true},
	}); err != nil {
		return nil, fmt.Errorf("mark stop command receipt dispatched: %w", err)
	}
	if err := s.logAudit(ctx, "digital_employee_run_stop_requested", req.UserID, run.ID, "employee.run.stop"); err != nil {
		return nil, err
	}
	return cancellingRun, nil
}

func (s *DigitalEmployeeRunService) ListRuns(ctx context.Context, tenantID, employeeID uuid.UUID, limit, offset int32) ([]*DigitalEmployeeRun, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	runs, err := s.repository.ListRuns(ctx, tenantID, employeeID, limit, offset)
	if err != nil {
		return nil, err
	}
	for index, run := range runs {
		reconciledRun, reconciled, err := s.reconcileTerminalReceipt(ctx, tenantID, run)
		if err != nil {
			return nil, err
		}
		if reconciled {
			runs[index] = reconciledRun
		}
	}
	return runs, nil
}

func (s *DigitalEmployeeRunService) GetRun(ctx context.Context, tenantID, employeeID, runID uuid.UUID) (*DigitalEmployeeRun, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if runID == uuid.Nil {
		return nil, fmt.Errorf("%w: run_id is required", ErrInvalidInput)
	}
	return s.repository.GetRun(ctx, tenantID, employeeID, runID)
}

func (s *DigitalEmployeeRunService) ListRunEvents(ctx context.Context, tenantID, employeeID, runID uuid.UUID, limit, offset int32) ([]RuntimeCommandEventWriteback, error) {
	run, err := s.GetRun(ctx, tenantID, employeeID, runID)
	if err != nil {
		return nil, err
	}
	return s.repository.ListRunEvents(ctx, tenantID, run.TaskID, run.ID, limit, offset)
}

func (s *DigitalEmployeeRunService) reconcileTerminalReceipt(ctx context.Context, tenantID uuid.UUID, run *DigitalEmployeeRun) (*DigitalEmployeeRun, bool, error) {
	if run == nil || !run.Status.IsActive() || strings.TrimSpace(run.CommandID) == "" {
		return run, false, nil
	}
	receipt, err := s.repository.GetCommandReceipt(ctx, tenantID, run.CommandID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return run, false, nil
		}
		return nil, false, fmt.Errorf("get terminal receipt for active run: %w", err)
	}
	if receipt == nil || !isTerminalReceiptStatus(receipt.Status) {
		return run, false, nil
	}
	updatedRun, err := s.repository.UpdateRunStatus(ctx, UpdateRunStatusRequest{
		TenantID:     tenantID,
		RunID:        run.ID,
		Status:       DigitalEmployeeRunStatus(receipt.Status),
		ErrorMessage: receipt.ErrorMessage,
		ErrorCode:    terminalReceiptErrorCode(receipt),
		ErrorFamily:  terminalReceiptErrorFamily(receipt),
	})
	if err != nil {
		return nil, false, fmt.Errorf("reconcile terminal receipt for active run: %w", err)
	}
	return updatedRun, true, nil
}

func terminalReceiptErrorCode(receipt *RuntimeCommandReceipt) *string {
	if receipt == nil || receipt.Status != string(DigitalEmployeeRunStatusFailed) {
		return nil
	}
	return stringPtr("dispatch_failed")
}

func terminalReceiptErrorFamily(receipt *RuntimeCommandReceipt) *string {
	if receipt == nil {
		return nil
	}
	switch receipt.Status {
	case string(DigitalEmployeeRunStatusFailed):
		return stringPtr("dispatch_failed")
	case string(DigitalEmployeeRunStatusTimedOut):
		return stringPtr("timeout")
	default:
		return nil
	}
}

func validateRunPreflight(preflight RunPreflight) error {
	if preflight.TenantID == uuid.Nil {
		return fmt.Errorf("%w: preflight tenant_id is required", ErrInvalidInput)
	}
	if preflight.TeamID == uuid.Nil {
		return fmt.Errorf("%w: preflight team_id is required", ErrInvalidInput)
	}
	if preflight.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: preflight digital_employee_id is required", ErrInvalidInput)
	}
	if preflight.DigitalEmployeeStatus != DigitalEmployeeStatusReady && preflight.DigitalEmployeeStatus != DigitalEmployeeStatusActive {
		return fmt.Errorf("%w: digital employee must be ready or active", ErrInvalidInput)
	}
	if !preflight.HasApprovedEffectiveConfig {
		return fmt.Errorf("%w: approved effective config is required", ErrEffectiveConfigRequired)
	}
	if preflight.ExecutionInstanceID == uuid.Nil {
		return fmt.Errorf("%w: execution_instance_id is required", ErrInvalidInput)
	}
	if preflight.ExecutionStatus != ExecutionInstanceStatusReady && preflight.ExecutionStatus != ExecutionInstanceStatusActive {
		return fmt.Errorf("%w: execution instance must be ready or active", ErrInvalidInput)
	}
	if preflight.RuntimeNodeID == uuid.Nil {
		return fmt.Errorf("%w: runtime_node_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(preflight.NodeID) == "" {
		return fmt.Errorf("%w: node_id is required", ErrInvalidInput)
	}
	if strings.TrimSpace(preflight.ProviderType) == "" {
		return fmt.Errorf("%w: provider_type is required", ErrInvalidInput)
	}
	if strings.TrimSpace(preflight.AgentHomeDir) == "" {
		return fmt.Errorf("%w: agent_home_dir is required", ErrInvalidInput)
	}
	if !preflight.ProviderHealthy {
		return fmt.Errorf("%w: provider capability must be healthy", ErrProviderUnavailable)
	}
	return nil
}

func validateDailyTokenBudget(preflight RunPreflight) error {
	policy, err := normalizeBudgetPolicy(preflight.BudgetPolicy)
	if err != nil {
		return err
	}
	value, ok := policy["daily_token_limit"].(float64)
	if !ok || value <= 0 {
		return nil
	}
	limit := int32(value)
	if preflight.TodayTokenUsage >= limit {
		return fmt.Errorf("%w: employee daily token budget exceeded", ErrInvalidInput)
	}
	return nil
}

func sameIdempotentRun(run *DigitalEmployeeRun, idempotencyKey *string, fingerprint string) bool {
	if run == nil || idempotencyKey == nil || run.IdempotencyKey == nil || run.IdempotencyFingerprint == nil {
		return false
	}
	return *run.IdempotencyKey == *idempotencyKey && *run.IdempotencyFingerprint == fingerprint
}

func computeRunIdempotencyFingerprint(req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight) (string, error) {
	input := map[string]any{
		"provider_run_protocol": providerRunProtocol,
		"tenant_id":             req.TenantID.String(),
		"digital_employee_id":   req.DigitalEmployeeID.String(),
		"execution_instance_id": preflight.ExecutionInstanceID.String(),
		"runtime_node_id":       preflight.RuntimeNodeID.String(),
		"node_id":               preflight.NodeID,
		"provider_type":         preflight.ProviderType,
		"agent_home_dir":        preflight.AgentHomeDir,
		"objective":             objective,
		"prompt":                prompt,
		"context_refs":          req.ContextRefs,
		"artifact_refs":         req.ArtifactRefs,
		"output_schema":         req.OutputSchema,
		"allowed_actions":       req.AllowedActions,
		"forbidden_actions":     req.ForbiddenActions,
		"secret_refs":           req.SecretRefs,
		"timeout_sec":           req.TimeoutSec,
		"grace_sec":             req.GraceSec,
		"metadata":              req.Metadata,
		"workspace_policy":      preflight.WorkspacePolicy,
		"session_policy":        preflight.SessionPolicy,
		"runtime_selector":      preflight.RuntimeSelector,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("%w: idempotency input must be json serializable: %v", ErrInvalidInput, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func buildRunParams(req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight, fingerprint string) map[string]any {
	return map[string]any{
		"provider_run_protocol":         providerRunProtocol,
		"objective":                     objective,
		"prompt":                        prompt,
		"context_refs":                  req.ContextRefs,
		"artifact_refs":                 req.ArtifactRefs,
		"output_schema":                 req.OutputSchema,
		"allowed_actions":               req.AllowedActions,
		"forbidden_actions":             req.ForbiddenActions,
		"secret_refs":                   req.SecretRefs,
		"timeout_sec":                   req.TimeoutSec,
		"grace_sec":                     req.GraceSec,
		"metadata":                      cloneMap(req.Metadata),
		"workspace_policy":              cloneMap(preflight.WorkspacePolicy),
		"session_policy":                cloneMap(preflight.SessionPolicy),
		"runtime_selector":              cloneMap(preflight.RuntimeSelector),
		"idempotency_fingerprint":       fingerprint,
		"has_approved_effective_config": preflight.HasApprovedEffectiveConfig,
		"provider_healthy":              preflight.ProviderHealthy,
	}
}

func buildStartSessionPayload(req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight, run *DigitalEmployeeRun) map[string]any {
	return map[string]any{
		"provider_run_protocol": providerRunProtocol,
		"tenant_id":             req.TenantID.String(),
		"task_id":               run.TaskID.String(),
		"run_id":                run.ID.String(),
		"command_id":            run.CommandID,
		"digital_employee_id":   req.DigitalEmployeeID.String(),
		"execution_instance_id": preflight.ExecutionInstanceID.String(),
		"runtime_node_id":       preflight.RuntimeNodeID.String(),
		"node_id":               preflight.NodeID,
		"provider_type":         preflight.ProviderType,
		"agent_home_dir":        preflight.AgentHomeDir,
		"objective":             objective,
		"prompt":                prompt,
		"input":                 prompt,
		"context_refs":          req.ContextRefs,
		"artifact_refs":         req.ArtifactRefs,
		"output_schema":         req.OutputSchema,
		"allowed_actions":       req.AllowedActions,
		"forbidden_actions":     req.ForbiddenActions,
		"secret_refs":           req.SecretRefs,
		"timeout_sec":           req.TimeoutSec,
		"grace_sec":             req.GraceSec,
		"workspace_policy":      cloneMap(preflight.WorkspacePolicy),
		"session_policy":        cloneMap(preflight.SessionPolicy),
		"runtime_selector":      cloneMap(preflight.RuntimeSelector),
		"metadata":              cloneMap(req.Metadata),
	}
}

func buildStopSessionPayload(run *DigitalEmployeeRun, commandID, reason string) map[string]any {
	return map[string]any{
		"provider_run_protocol": providerRunProtocol,
		"run_id":                run.ID.String(),
		"task_id":               run.TaskID.String(),
		"command_id":            commandID,
		"start_command_id":      run.CommandID,
		"reason":                reason,
		"grace_sec":             run.GraceSec,
	}
}

func runtimeCommand(id, commandType string, payload map[string]any) (cpruntime.RuntimeCommand, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return cpruntime.RuntimeCommand{}, fmt.Errorf("encode runtime command payload: %w", err)
	}
	return cpruntime.RuntimeCommand{
		ID:      id,
		Type:    commandType,
		Payload: encoded,
	}, nil
}

func newRuntimeCommandID() string {
	return "cmd-" + uuid.NewString()
}

func trimmedOptionalValue(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringPtrIfNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringPtr(value string) *string {
	return &value
}

func (s *DigitalEmployeeRunService) logAudit(ctx context.Context, eventType string, actorID, runID uuid.UUID, action string) error {
	if s.audit == nil {
		return nil
	}
	if err := s.audit.LogEvent(ctx, eventType, "user", actorID.String(), "digital_employee_run", runID.String(), action); err != nil {
		return fmt.Errorf("log audit event: %w", err)
	}
	return nil
}
