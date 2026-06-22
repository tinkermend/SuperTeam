package employee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	cpruntime "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/skill"
)

const (
	providerRunProtocol             = "provider-run/v1"
	runDispatchedLifecycleSequence  = -1
	stopRequestedLifecycleSequence  = -2
	runReapedStaleLifecycleSequence = -3
	// staleDispatchTTL is how long a run may sit in a pre-confirmation state
	// (queued/dispatching) without any row update before it is treated as
	// abandoned. A run reaches "dispatching" once the start-session command has
	// been sent to the runtime node; if the runtime never reports back (node
	// gone, callback lost, process crash mid-dispatch) the run would otherwise
	// block every future dispatch to that digital employee via the
	// one-active-run guard. Reaping it lets the next dispatch proceed.
	staleDispatchTTL = 5 * time.Minute
)

type RuntimeCommandDispatcher interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command cpruntime.RuntimeCommand) error
}

type AuditLogger interface {
	LogEvent(ctx context.Context, eventType, actorType, actorID, resourceType, resourceID, action string) error
}

type RuntimeSkillLister interface {
	ListSkillsForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]skill.SkillRuntimeRecord, error)
}

type RuntimeCapabilityLister interface {
	ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]cpruntime.RuntimeCapability, error)
}

type RuntimeEnvironmentLister interface {
	ListRuntimeEnvironmentVariablesForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeEnvironmentVariablePayload, error)
}

type DigitalEmployeeRunService struct {
	repository              DigitalEmployeeRunRepository
	dispatcher              RuntimeCommandDispatcher
	audit                   AuditLogger
	skillLister             RuntimeSkillLister
	runtimeCapabilityLister RuntimeCapabilityLister
	environmentLister       RuntimeEnvironmentLister
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

func (s *DigitalEmployeeRunService) SetSkillLister(l RuntimeSkillLister) {
	s.skillLister = l
}

func (s *DigitalEmployeeRunService) SetRuntimeCapabilityLister(l RuntimeCapabilityLister) {
	s.runtimeCapabilityLister = l
}

func (s *DigitalEmployeeRunService) SetEnvironmentLister(l RuntimeEnvironmentLister) {
	s.environmentLister = l
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
			runtimeInputs, err := s.loadRuntimeRunInputs(ctx, req.TenantID, req.DigitalEmployeeID, preflight)
			if err != nil {
				return nil, err
			}
			return s.dispatchStartSession(ctx, req, objective, prompt, preflight, activeRun, runtimeInputs)
		}
		if isStalePreConfirmationRun(activeRun) {
			if _, err := s.reapStaleRun(ctx, req.TenantID, req.UserID, activeRun); err != nil {
				return nil, err
			}
			activeRun = nil
		} else {
			return nil, fmt.Errorf("%w: active digital employee run exists", ErrConflict)
		}
	}

	runtimeInputs, err := s.loadRuntimeRunInputs(ctx, req.TenantID, req.DigitalEmployeeID, preflight)
	if err != nil {
		return nil, err
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

	return s.dispatchStartSession(ctx, req, objective, prompt, preflight, run, runtimeInputs)
}

func (s *DigitalEmployeeRunService) dispatchStartSession(ctx context.Context, req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight, run *DigitalEmployeeRun, runtimeInputs runtimeRunInputs) (*DigitalEmployeeRun, error) {
	if run.Status.IsTerminal() || run.Status == DigitalEmployeeRunStatusRunning || run.Status == DigitalEmployeeRunStatusCancelling {
		return run, nil
	}

	workspaceFiles, err := s.repository.ListWorkspaceFilesForSync(ctx, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, fmt.Errorf("list workspace files for start session: %w", err)
	}
	payload := buildStartSessionPayload(req, objective, prompt, preflight, run, workspaceFiles, runtimeInputs)
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

// isStalePreConfirmationRun reports whether an active run is abandoned in a
// pre-confirmation state (queued/dispatching): the start-session command was
// sent (or is mid-flight) but the runtime has not confirmed session start, and
// the row has not been touched for longer than staleDispatchTTL. Such a run
// will never make progress on its own and is safe to reap. Runs the runtime has
// confirmed as running, or that are being cancelled, are never considered
// stale — those are genuinely active.
func isStalePreConfirmationRun(run *DigitalEmployeeRun) bool {
	if run == nil {
		return false
	}
	if run.Status != DigitalEmployeeRunStatusQueued && run.Status != DigitalEmployeeRunStatusDispatching {
		return false
	}
	return time.Since(run.UpdatedAt) > staleDispatchTTL
}

// reapStaleRun marks an abandoned pre-confirmation run as failed so it no longer
// occupies the digital employee's single active-run slot. It records a lifecycle
// event and an audit entry for observability. The triggering actor (the user or
// coordinator whose dispatch was blocked) is attributed for traceability.
func (s *DigitalEmployeeRunService) reapStaleRun(ctx context.Context, tenantID, actorID uuid.UUID, run *DigitalEmployeeRun) (*DigitalEmployeeRun, error) {
	reaped, err := s.repository.UpdateRunStatus(ctx, UpdateRunStatusRequest{
		TenantID:     tenantID,
		RunID:        run.ID,
		Status:       DigitalEmployeeRunStatusFailed,
		ErrorMessage: stringPtr("run abandoned in pre-confirmation state; reaped as stale"),
		ErrorCode:    stringPtr("dispatch_stale"),
		ErrorFamily:  stringPtr("dispatch_timeout"),
	})
	if err != nil {
		return nil, fmt.Errorf("reap stale dispatching run: %w", err)
	}
	_, _ = s.repository.CreateTaskEventIfAbsent(ctx, CreateRunEventRecordRequest{
		TenantID:       tenantID,
		TaskID:         run.TaskID,
		RunID:          run.ID,
		EventType:      "run_reaped_stale",
		SequenceNumber: runReapedStaleLifecycleSequence,
		Payload: map[string]any{
			"prior_status": string(run.Status),
			"command_id":   run.CommandID,
		},
		Metadata: map[string]any{"source": "control-plane"},
	})
	_ = s.logAudit(ctx, "digital_employee_run_reaped_stale", actorID, run.ID, "employee.run.reap_stale")
	return reaped, nil
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
		"session_policy":        runtimeSessionPolicyPayload(preflight.SessionPolicy),
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
		"session_policy":                runtimeSessionPolicyPayload(preflight.SessionPolicy),
		"runtime_selector":              cloneMap(preflight.RuntimeSelector),
		"idempotency_fingerprint":       fingerprint,
		"has_approved_effective_config": preflight.HasApprovedEffectiveConfig,
		"provider_healthy":              preflight.ProviderHealthy,
	}
}

type runtimeRunInputs struct {
	skills      []skill.SkillRuntimeRecord
	environment []RuntimeEnvironmentVariablePayload
}

func (s *DigitalEmployeeRunService) loadRuntimeRunInputs(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, preflight RunPreflight) (runtimeRunInputs, error) {
	inputs := runtimeRunInputs{}
	if s.skillLister != nil {
		skills, err := s.skillLister.ListSkillsForRuntime(ctx, tenantID, digitalEmployeeID)
		if err != nil {
			return runtimeRunInputs{}, fmt.Errorf("list skills for runtime: %w", err)
		}
		inputs.skills = skills
	}

	requiredTools := requiredRuntimeTools(inputs.skills)
	if len(requiredTools) > 0 {
		if s.runtimeCapabilityLister == nil {
			return runtimeRunInputs{}, missingRuntimeToolError(preflight.NodeID, requiredTools[0])
		}
		capabilities, err := s.runtimeCapabilityLister.ListRuntimeCapabilitiesForNode(ctx, tenantID, preflight.NodeID)
		if err != nil {
			return runtimeRunInputs{}, fmt.Errorf("list runtime capabilities: %w", err)
		}
		availableTools := availableRuntimeTools(capabilities)
		for _, tool := range requiredTools {
			if !availableTools[tool] {
				return runtimeRunInputs{}, missingRuntimeToolError(preflight.NodeID, tool)
			}
		}
	}

	requiredEnv := requiredRuntimeEnvironmentNames(inputs.skills)
	if len(requiredEnv) > 0 {
		if s.environmentLister == nil {
			return runtimeRunInputs{}, fmt.Errorf("%w: required runtime environment variables are not configured: %s", ErrInvalidInput, strings.Join(requiredEnv, ", "))
		}
		environment, err := s.environmentLister.ListRuntimeEnvironmentVariablesForRuntime(ctx, tenantID, digitalEmployeeID)
		if err != nil {
			return runtimeRunInputs{}, fmt.Errorf("list runtime environment variables: %w", err)
		}
		availableEnv := availableRuntimeEnvironment(environment)
		for _, name := range requiredEnv {
			if !availableEnv[name] {
				return runtimeRunInputs{}, fmt.Errorf("%w: required runtime environment variable is missing: %s", ErrInvalidInput, name)
			}
		}
		inputs.environment = environment
	}

	return inputs, nil
}

func buildStartSessionPayload(req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight, run *DigitalEmployeeRun, workspaceFiles []WorkspaceFileForSyncRecord, runtimeInputs runtimeRunInputs) map[string]any {
	metadata := cloneMap(req.Metadata)
	if metadata["source"] == "project_task_dispatch" {
		metadata["runtime_node_id"] = preflight.RuntimeNodeID.String()
		version, _ := metadata["execution_context_packet_version"].(string)
		if strings.TrimSpace(version) == "" {
			metadata["execution_context_packet_version"] = "v1"
		}
	}
	return map[string]any{
		"provider_run_protocol": providerRunProtocol,
		"tenant_id":             req.TenantID.String(),
		"team_id":               preflight.TeamID.String(),
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
		"session_policy":        runtimeSessionPolicyPayload(preflight.SessionPolicy),
		"runtime_selector":      cloneMap(preflight.RuntimeSelector),
		"workspace_files":       runtimeWorkspaceFilesPayload(workspaceFiles),
		"skills":                runtimeSkillsPayload(runtimeInputs.skills),
		"mcp_servers":           emptyRuntimeMCPServersPayload(),
		"environment":           runtimeEnvironmentPayload(runtimeInputs.environment),
		"metadata":              metadata,
	}
}

func requiredRuntimeTools(skills []skill.SkillRuntimeRecord) []string {
	return uniqueTrimmedValues(func(yield func(string)) {
		for _, item := range skills {
			for _, value := range item.RuntimeDependencies.Tools {
				yield(value)
			}
		}
	})
}

func requiredRuntimeEnvironmentNames(skills []skill.SkillRuntimeRecord) []string {
	return uniqueTrimmedValues(func(yield func(string)) {
		for _, item := range skills {
			for _, value := range item.RuntimeDependencies.Env {
				yield(value)
			}
		}
	})
}

func uniqueTrimmedValues(collect func(func(string))) []string {
	values := make([]string, 0)
	seen := map[string]struct{}{}
	collect(func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		values = append(values, value)
	})
	return values
}

func availableRuntimeTools(capabilities []cpruntime.RuntimeCapability) map[string]bool {
	available := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		if strings.TrimSpace(capability.CapabilityType) != "tool" || !capability.Available {
			continue
		}
		key := strings.TrimSpace(capability.CapabilityKey)
		if key == "" {
			continue
		}
		available[key] = true
	}
	return available
}

func availableRuntimeEnvironment(environment []RuntimeEnvironmentVariablePayload) map[string]bool {
	available := make(map[string]bool, len(environment))
	for _, item := range environment {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			available[name] = true
		}
	}
	return available
}

func runtimeEnvironmentPayload(environment []RuntimeEnvironmentVariablePayload) []map[string]any {
	out := make([]map[string]any, 0, len(environment))
	for _, item := range environment {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		out = append(out, map[string]any{
			"name":      name,
			"value":     item.Value,
			"sensitive": item.Sensitive,
		})
	}
	return out
}

func missingRuntimeToolError(nodeID, tool string) error {
	return fmt.Errorf("%w: pending_runtime required runtime tool %q has not been reported by node %s", ErrRuntimeUnavailable, tool, nodeID)
}

func runtimeSessionPolicyPayload(policy map[string]any) map[string]any {
	normalized := cloneMap(policy)
	mode, _ := normalized["mode"].(string)
	if strings.TrimSpace(mode) == "" {
		normalized["mode"] = "new"
	}
	if _, ok := normalized["recoverable"]; !ok {
		normalized["recoverable"] = true
	}
	return normalized
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
