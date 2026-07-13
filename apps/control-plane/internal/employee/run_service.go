package employee

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
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

// RuntimeMCPLister resolves the effective MCP servers for an employee, already filtered to
// env-satisfied bindings, ready to project into the runtime start-session payload.
type RuntimeMCPLister interface {
	ListRuntimeMCPServersForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]RuntimeMCPServerPayload, error)
}

type DigitalEmployeeRunService struct {
	repository          DigitalEmployeeRunRepository
	dispatcher          RuntimeCommandDispatcher
	audit               AuditLogger
	skillLister         RuntimeSkillLister
	capabilityLister    RuntimeCapabilityLister
	envLister           RuntimeEnvironmentLister
	mcpLister           RuntimeMCPLister
	nodeResolver        ProjectTaskNodeResolver
	chatAnchorValidator ChatAnchorProjectValidator
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
	s.capabilityLister = l
}

func (s *DigitalEmployeeRunService) SetEnvironmentLister(l RuntimeEnvironmentLister) {
	s.envLister = l
}

func (s *DigitalEmployeeRunService) SetMCPLister(l RuntimeMCPLister) {
	s.mcpLister = l
}

// SetProjectTaskNodeResolver injects the three-layer runtime node resolver used
// by StartProjectTaskRun. It is required for project task dispatch; when unset,
// StartProjectTaskRun fails fast rather than falling back to any legacy
// single-node selection.
func (s *DigitalEmployeeRunService) SetProjectTaskNodeResolver(r ProjectTaskNodeResolver) {
	s.nodeResolver = r
}

// SetChatAnchorProjectValidator injects the project existence/tenant/archived
// check chat runs must pass before their project-anchor node resolution runs.
// It is required for chat run dispatch; when unset, CreateRun fails fast for
// chat requests rather than skipping the check.
func (s *DigitalEmployeeRunService) SetChatAnchorProjectValidator(v ChatAnchorProjectValidator) {
	s.chatAnchorValidator = v
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
	if req.RunKind == "" {
		req.RunKind = RunKindTask
	}
	if req.RunKind != RunKindTask && req.RunKind != RunKindChat {
		return nil, ErrInvalidRunKind
	}

	if req.RunKind == RunKindChat {
		if req.ProjectID == nil || *req.ProjectID == uuid.Nil {
			return nil, fmt.Errorf("%w: project_id is required for chat runs", ErrInvalidInput)
		}
	} else {
		// §13 design revision: task runs ignore any anchor project_id the
		// caller may have sent — it is a chat-only concept, not validated or
		// persisted for task-kind runs.
		req.ProjectID = nil
	}

	if req.ResumeOfRunID != nil {
		if req.RunKind != RunKindChat {
			return nil, ErrInvalidResumeRun
		}
		prior, err := s.repository.GetRun(ctx, req.TenantID, req.DigitalEmployeeID, *req.ResumeOfRunID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidResumeRun, err)
		}
		if prior == nil ||
			prior.DigitalEmployeeID != req.DigitalEmployeeID ||
			prior.RunKind != RunKindChat ||
			!prior.Status.IsTerminal() ||
			prior.ProviderSessionID == nil || strings.TrimSpace(*prior.ProviderSessionID) == "" {
			return nil, ErrInvalidResumeRun
		}
		// The resumed session lives on the prior run's anchor node; resuming
		// under a different anchor project is meaningless, so the request's
		// project_id must match the prior run's persisted anchor exactly.
		priorMetadata, err := s.repository.GetRunTaskMetadata(ctx, req.TenantID, prior.TaskID)
		if err != nil {
			return nil, fmt.Errorf("get prior chat run anchor: %w", err)
		}
		priorAnchor, _ := priorMetadata["anchor_project_id"].(string)
		if priorAnchor == "" || priorAnchor != req.ProjectID.String() {
			return nil, ErrInvalidResumeRun
		}
		if req.Metadata == nil {
			req.Metadata = map[string]any{}
		}
		req.Metadata["provider_session_id"] = *prior.ProviderSessionID
		req.Metadata["resume_of_run_id"] = prior.ID.String()
	}

	if req.RunKind == RunKindChat {
		return s.createChatRun(ctx, req, objective, prompt)
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

	return s.createAndDispatchRun(ctx, req, objective, prompt, preflight)
}

// createChatRun dispatches a chat run using its project anchor (§13 design
// revision): standalone preflight (digital_employee_execution_instances) is
// designed-off since runtime affinity no longer populates it, so chat reuses
// the project task's node-resolution + preflight-for-node path instead,
// exactly as project task dispatch does, but with no project task id (chat
// never has one — ProjectTaskID is passed as uuid.Nil, which
// ResolveProjectTaskNode documents as "skip the task hard-pin layer").
func (s *DigitalEmployeeRunService) createChatRun(ctx context.Context, req CreateDigitalEmployeeRunRequest, objective, prompt string) (*DigitalEmployeeRun, error) {
	preflightRepo, ok := s.repository.(ProjectTaskRunPreflightRepository)
	if !ok {
		return nil, fmt.Errorf("%w: project task run preflight repository is required", ErrInvalidInput)
	}
	if s.nodeResolver == nil {
		return nil, fmt.Errorf("%w: project task node resolver is required", ErrInvalidInput)
	}
	if s.chatAnchorValidator == nil {
		return nil, fmt.Errorf("%w: chat anchor project validator is required", ErrInvalidInput)
	}
	projectID := *req.ProjectID

	if err := s.chatAnchorValidator.ValidateChatAnchorProject(ctx, req.TenantID, projectID); err != nil {
		return nil, err
	}

	resolvedNodeID, err := s.nodeResolver.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeRequest{
		TenantID:          req.TenantID,
		ProjectID:         projectID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		ProjectTaskID:     uuid.Nil,
		// DryRun: a chat run's project anchor is a passive dispatch-scoping
		// concept (§13 design revision), not a signal of the employee's real
		// task placement — resolution must never persist/steer node affinity
		// as a side effect of sending a chat message.
		DryRun: true,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve chat anchor node: %w", err)
	}
	projectPreflight, err := preflightRepo.GetProjectTaskRunPreflightForNode(ctx, req.TenantID, req.DigitalEmployeeID, resolvedNodeID)
	if err != nil {
		return nil, fmt.Errorf("get chat anchor run preflight: %w", err)
	}
	if err := validateProjectTaskRunPreflight(projectPreflight); err != nil {
		return nil, err
	}
	if err := validateDailyTokenBudget(RunPreflight{BudgetPolicy: projectPreflight.BudgetPolicy, TodayTokenUsage: projectPreflight.TodayTokenUsage}); err != nil {
		return nil, err
	}
	if !s.dispatcher.IsConnected(projectPreflight.NodeID) {
		return nil, fmt.Errorf("%w: runtime node is not connected", ErrRuntimeUnavailable)
	}

	// The workspace/execution-instance nonce is derived from the caller's
	// idempotency_key when present so a genuine retry of the same logical
	// chat dispatch lands on the same one-off directory (preserving
	// createAndDispatchRun's idempotent re-dispatch path); without an
	// idempotency key, callers get no such retry-matching guarantee anyway
	// (sameIdempotentRun always requires an idempotency key), so a fresh
	// nonce per call is safe.
	nonce := chatDispatchNonce(req.IdempotencyKey)
	compatExecutionInstanceID := chatCompatibilityExecutionInstanceID(req.TenantID, projectID, req.DigitalEmployeeID, nonce)
	agentHomeDir := chatAgentHomeDir(projectPreflight.WorkspaceBaseDir, projectID, req.DigitalEmployeeID, nonce)
	preflight := projectTaskRunPreflightToRunPreflight(projectPreflight, compatExecutionInstanceID, agentHomeDir)

	if req.Metadata == nil {
		req.Metadata = map[string]any{}
	}
	// Audit-only anchor record (§13): not a new column, lives in
	// tasks.params["metadata"]["anchor_project_id"] via buildRunParams.
	req.Metadata["anchor_project_id"] = projectID.String()

	return s.createAndDispatchRun(ctx, req, objective, prompt, preflight)
}

func (s *DigitalEmployeeRunService) StartProjectTaskRun(ctx context.Context, req StartProjectTaskRunRequest) (StartProjectTaskRunResult, error) {
	objective := strings.TrimSpace(req.Objective)
	prompt := strings.TrimSpace(req.Prompt)
	if req.TenantID == uuid.Nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if req.ProjectID == uuid.Nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: project_id is required", ErrInvalidInput)
	}
	if req.DemandID == uuid.Nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: demand_id is required", ErrInvalidInput)
	}
	if req.ProjectTaskID == uuid.Nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: project_task_id is required", ErrInvalidInput)
	}
	if req.ProjectTaskAttemptID == uuid.Nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: project_task_attempt_id is required", ErrInvalidInput)
	}
	if req.DigitalEmployeeID == uuid.Nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	if req.DispatchUserID == uuid.Nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: dispatch_user_id is required", ErrInvalidInput)
	}
	if objective == "" {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: objective is required", ErrInvalidInput)
	}

	preflightRepo, ok := s.repository.(ProjectTaskRunPreflightRepository)
	if !ok {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: project task run preflight repository is required", ErrInvalidInput)
	}
	if s.nodeResolver == nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: project task node resolver is required", ErrInvalidInput)
	}
	resolvedNodeID, err := s.nodeResolver.ResolveProjectTaskNode(ctx, ResolveProjectTaskNodeRequest{
		TenantID:          req.TenantID,
		ProjectID:         req.ProjectID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		ProjectTaskID:     req.ProjectTaskID,
	})
	if err != nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("resolve project task node: %w", err)
	}
	projectPreflight, err := preflightRepo.GetProjectTaskRunPreflightForNode(ctx, req.TenantID, req.DigitalEmployeeID, resolvedNodeID)
	if err != nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("get project task run preflight: %w", err)
	}
	if err := validateProjectTaskRunPreflight(projectPreflight); err != nil {
		return StartProjectTaskRunResult{}, err
	}
	if err := validateDailyTokenBudget(RunPreflight{BudgetPolicy: projectPreflight.BudgetPolicy, TodayTokenUsage: projectPreflight.TodayTokenUsage}); err != nil {
		return StartProjectTaskRunResult{}, err
	}
	if !s.dispatcher.IsConnected(projectPreflight.NodeID) {
		return StartProjectTaskRunResult{}, fmt.Errorf("%w: runtime node is not connected", ErrRuntimeUnavailable)
	}

	rootTaskID, err := preflightRepo.ResolveProjectTaskLineageRoot(ctx, req.TenantID, req.ProjectTaskID)
	if err != nil {
		return StartProjectTaskRunResult{}, fmt.Errorf("resolve project task lineage root: %w", err)
	}

	compatExecutionInstanceID := projectTaskCompatibilityExecutionInstanceID(req)
	agentHomeDir := projectTaskAgentHomeDir(projectPreflight.WorkspaceBaseDir, req.ProjectID, req.ProjectTaskID, req.ProjectTaskAttemptID, req.DigitalEmployeeID)
	preflight := projectTaskRunPreflightToRunPreflight(projectPreflight, compatExecutionInstanceID, agentHomeDir)

	metadata := projectTaskRunMetadata(req, projectPreflight)
	metadata["revision_root_task_id"] = rootTaskID.String()

	// Session identity is decided here; runtime only consumes it. The
	// session key is the lineage root, never the current project_task_id —
	// this is what lets a revision task resume its predecessor's session.
	sessionPolicy := runtimeSessionPolicyPayload(preflight.SessionPolicy)
	if shouldAttemptSessionResume(sessionPolicy) {
		sessionID, err := s.repository.FindProviderSessionForTaskRoot(ctx, req.TenantID, req.DigitalEmployeeID, rootTaskID)
		if err != nil {
			return StartProjectTaskRunResult{}, fmt.Errorf("find provider session for task root: %w", err)
		}
		if sessionID != "" {
			metadata["provider_session_id"] = sessionID
		}
	}

	createReq := CreateDigitalEmployeeRunRequest{
		TenantID:          req.TenantID,
		UserID:            req.DispatchUserID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		Objective:         objective,
		Prompt:            prompt,
		IdempotencyKey:    trimmedOptionalValue(&req.IdempotencyKey),
		TimeoutSec:        req.TimeoutSec,
		GraceSec:          req.GraceSec,
		Metadata:          metadata,
		// Project task dispatch always produces a task run; chat runs are only
		// created via the workbench CreateRun path.
		RunKind: RunKindTask,
	}

	run, err := s.createAndDispatchRun(ctx, createReq, objective, prompt, preflight)
	if err != nil {
		return StartProjectTaskRunResult{}, err
	}
	return StartProjectTaskRunResult{
		RunID:         run.ID,
		RuntimeTaskID: run.TaskID,
		RuntimeNodeID: run.RuntimeNodeID,
		NodeID:        run.NodeID,
		ProviderType:  run.ProviderType,
	}, nil
}

func (s *DigitalEmployeeRunService) createAndDispatchRun(ctx context.Context, req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight) (*DigitalEmployeeRun, error) {
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
			deps, err := s.prepareStartSessionDependencies(ctx, req.TenantID, req.DigitalEmployeeID, preflight)
			if err != nil {
				return nil, err
			}
			return s.dispatchStartSession(ctx, req, objective, prompt, preflight, activeRun, deps)
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

	deps, err := s.prepareStartSessionDependencies(ctx, req.TenantID, req.DigitalEmployeeID, preflight)
	if err != nil {
		return nil, err
	}

	runKind := req.RunKind
	if runKind == "" {
		// Defensive default: callers that build CreateDigitalEmployeeRunRequest
		// directly (bypassing CreateRun's own normalization) must still persist a
		// valid run_kind, since the column is NOT NULL with a CHECK constraint.
		runKind = RunKindTask
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
		RunKind:                runKind,
		ResumeOfRunID:          req.ResumeOfRunID,
	}

	run, err := s.repository.CreateRun(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("create digital employee run: %w", err)
	}

	return s.dispatchStartSession(ctx, req, objective, prompt, preflight, run, deps)
}

type startSessionDependencies struct {
	runtimeSkills []skill.SkillRuntimeRecord
	runtimeEnv    []RuntimeEnvironmentVariablePayload
	runtimeMCP    []RuntimeMCPServerPayload
	configInput   EmployeeConfigInput
}

func (s *DigitalEmployeeRunService) dispatchStartSession(ctx context.Context, req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight, run *DigitalEmployeeRun, deps startSessionDependencies) (*DigitalEmployeeRun, error) {
	if run.Status.IsTerminal() || run.Status == DigitalEmployeeRunStatusRunning || run.Status == DigitalEmployeeRunStatusCancelling {
		return run, nil
	}

	payload := buildStartSessionPayload(req, objective, prompt, preflight, run, deps.configInput, deps.runtimeSkills, deps.runtimeEnv, deps.runtimeMCP)
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

type SkillDependencyEvaluation struct {
	LoadStatus   string
	MissingTools []string
	MissingEnv   []string
}

func (s *DigitalEmployeeRunService) prepareStartSessionDependencies(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, preflight RunPreflight) (startSessionDependencies, error) {
	var deps startSessionDependencies
	configInput, err := s.repository.GetLatestDigitalEmployeeConfigRevision(ctx, tenantID, digitalEmployeeID)
	if err != nil {
		return deps, fmt.Errorf("get latest employee config revision: %w", err)
	}
	deps.configInput = configInput
	if s.skillLister != nil {
		runtimeSkills, err := s.skillLister.ListSkillsForRuntime(ctx, tenantID, digitalEmployeeID)
		if err != nil {
			return deps, fmt.Errorf("list runtime skills: %w", err)
		}
		deps.runtimeSkills = runtimeSkills
	}
	var capabilities []cpruntime.RuntimeCapability
	if s.capabilityLister != nil {
		listed, err := s.capabilityLister.ListRuntimeCapabilitiesForNode(ctx, tenantID, preflight.NodeID)
		if err != nil {
			return deps, fmt.Errorf("list runtime capabilities: %w", err)
		}
		capabilities = listed
	}
	if s.envLister != nil {
		runtimeEnv, err := s.envLister.ListRuntimeEnvironmentVariablesForRuntime(ctx, tenantID, digitalEmployeeID)
		if err != nil {
			return deps, fmt.Errorf("list runtime environment variables: %w", err)
		}
		deps.runtimeEnv = runtimeEnv
	}
	if s.mcpLister != nil {
		// Loaded after env vars: the lister excludes bindings whose required env vars the
		// employee has not configured, so only env-satisfied MCP servers are projected.
		runtimeMCP, err := s.mcpLister.ListRuntimeMCPServersForRuntime(ctx, tenantID, digitalEmployeeID)
		if err != nil {
			return deps, fmt.Errorf("list runtime mcp servers: %w", err)
		}
		deps.runtimeMCP = runtimeMCP
	}

	availableTools := map[string]struct{}{}
	for _, capability := range capabilities {
		if capability.CapabilityType != "tool" || !capability.Available {
			continue
		}
		key := strings.TrimSpace(capability.CapabilityKey)
		if key != "" {
			availableTools[key] = struct{}{}
		}
	}
	availableEnv := map[string]struct{}{}
	for _, env := range deps.runtimeEnv {
		name := strings.TrimSpace(env.Name)
		if name != "" {
			availableEnv[name] = struct{}{}
		}
	}
	if err := validateRuntimeSkillDependencies(deps.runtimeSkills, availableTools, availableEnv, len(capabilities) > 0); err != nil {
		return deps, err
	}
	return deps, nil
}

func validateRuntimeSkillDependencies(runtimeSkills []skill.SkillRuntimeRecord, availableTools, availableEnv map[string]struct{}, nodeReportedAnyCapability bool) error {
	var messages []string
	code := ""
	for _, runtimeSkill := range runtimeSkills {
		evaluation := evaluateSkillDependencies(runtimeSkill, availableTools, availableEnv, nodeReportedAnyCapability)
		if evaluation.LoadStatus == "loadable" {
			continue
		}
		if code == "" || dependencyStatusPriority(evaluation.LoadStatus) < dependencyStatusPriority(code) {
			code = evaluation.LoadStatus
		}
		messages = append(messages, skillDependencyFailureMessage(runtimeSkill, evaluation))
	}
	if len(messages) == 0 {
		return nil
	}
	errorCode := "skill_dependencies_not_satisfied"
	if code == "pending_runtime" {
		errorCode = "skill_dependencies_pending_runtime"
	}
	return fmt.Errorf("%w: %s: %s", ErrInvalidInput, errorCode, strings.Join(messages, "; "))
}

func dependencyStatusPriority(status string) int {
	switch status {
	case "pending_runtime":
		return 0
	case "missing_tools":
		return 1
	case "missing_env":
		return 2
	default:
		return 3
	}
}

func skillDependencyFailureMessage(runtimeSkill skill.SkillRuntimeRecord, evaluation SkillDependencyEvaluation) string {
	parts := []string{fmt.Sprintf("skill %s %s", runtimeSkill.Slug, evaluation.LoadStatus)}
	if evaluation.LoadStatus == "pending_runtime" {
		parts = append(parts, "等待 Runtime 上报")
	}
	if len(evaluation.MissingTools) > 0 {
		parts = append(parts, "missing_tools="+strings.Join(evaluation.MissingTools, ","))
	}
	if len(evaluation.MissingEnv) > 0 {
		parts = append(parts, "missing_env="+strings.Join(evaluation.MissingEnv, ","))
	}
	return strings.Join(parts, " ")
}

// evaluateSkillDependencies computes the load status of one skill.
// MissingTools and MissingEnv are fully populated regardless of status.
func evaluateSkillDependencies(runtimeSkill skill.SkillRuntimeRecord, availableTools map[string]struct{}, availableEnv map[string]struct{}, nodeReportedAnyCapability bool) SkillDependencyEvaluation {
	missingTools := []string{}
	for _, tool := range runtimeSkill.RuntimeDependencies.Tools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if _, ok := availableTools[tool]; !ok {
			missingTools = append(missingTools, tool)
		}
	}
	missingEnv := []string{}
	for _, name := range runtimeSkill.RuntimeDependencies.Env {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := availableEnv[name]; !ok {
			missingEnv = append(missingEnv, name)
		}
	}

	status := "loadable"
	switch {
	case !nodeReportedAnyCapability && (len(missingTools) > 0 || len(missingEnv) > 0):
		status = "pending_runtime"
	case len(missingTools) > 0:
		status = "missing_tools"
	case len(missingEnv) > 0:
		status = "missing_env"
	}
	return SkillDependencyEvaluation{LoadStatus: status, MissingTools: missingTools, MissingEnv: missingEnv}
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

func (s *DigitalEmployeeRunService) ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	result, err := s.repository.ListRunsDetailed(ctx, tenantID, employeeID, filter)
	if err != nil {
		return nil, err
	}
	for index, item := range result.Items {
		reconciledRun, reconciled, err := s.reconcileTerminalReceipt(ctx, tenantID, item.Run)
		if err != nil {
			return nil, err
		}
		if reconciled {
			result.Items[index].Run = reconciledRun
		}
	}
	return result, nil
}

func (s *DigitalEmployeeRunService) GetRunStats(ctx context.Context, tenantID, employeeID uuid.UUID) (*DigitalEmployeeRunStats, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: digital_employee_id is required", ErrInvalidInput)
	}
	stats, err := s.repository.GetDigitalEmployeeRunStats(ctx, tenantID, employeeID)
	if err != nil {
		return nil, err
	}
	return &stats, nil
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
	if preflight.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: preflight digital_employee_id is required", ErrInvalidInput)
	}
	if preflight.DigitalEmployeeStatus != DigitalEmployeeStatusReady && preflight.DigitalEmployeeStatus != DigitalEmployeeStatusActive {
		return fmt.Errorf("%w: digital employee must be ready or active", ErrInvalidInput)
	}
	// Legacy workbench runs still require a concrete execution instance. ProjectTask dispatch uses project placement preflight instead.
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

func validateProjectTaskRunPreflight(preflight StartProjectTaskRunPreflight) error {
	if preflight.TenantID == uuid.Nil {
		return fmt.Errorf("%w: preflight tenant_id is required", ErrInvalidInput)
	}
	if preflight.DigitalEmployeeID == uuid.Nil {
		return fmt.Errorf("%w: preflight digital_employee_id is required", ErrInvalidInput)
	}
	if preflight.DigitalEmployeeStatus != DigitalEmployeeStatusReady && preflight.DigitalEmployeeStatus != DigitalEmployeeStatusActive {
		return fmt.Errorf("%w: digital employee must be ready or active", ErrInvalidInput)
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
	if !preflight.RuntimeSessionActive {
		return fmt.Errorf("%w: runtime session is not active", ErrRuntimeUnavailable)
	}
	if !preflight.ProviderHealthy {
		return fmt.Errorf("%w: provider capability must be healthy", ErrProviderUnavailable)
	}
	if strings.TrimSpace(preflight.WorkspaceBaseDir) == "" {
		return fmt.Errorf("%w: workspace_base_dir is required", ErrInvalidInput)
	}
	return nil
}

func projectTaskRunPreflightToRunPreflight(preflight StartProjectTaskRunPreflight, executionInstanceID uuid.UUID, agentHomeDir string) RunPreflight {
	return RunPreflight{
		TenantID:              preflight.TenantID,
		TeamID:                preflight.TeamID,
		DigitalEmployeeID:     preflight.DigitalEmployeeID,
		DigitalEmployeeStatus: preflight.DigitalEmployeeStatus,
		ExecutionInstanceID:   executionInstanceID,
		ExecutionStatus:       ExecutionInstanceStatusReady,
		RuntimeNodeID:         preflight.RuntimeNodeID,
		NodeID:                preflight.NodeID,
		ProviderType:          preflight.ProviderType,
		AgentHomeDir:          agentHomeDir,
		RuntimeSelector:       map[string]any{"source": "project_placement", "node_id": preflight.NodeID},
		SessionPolicy:         map[string]any{"resume": true},
		WorkspacePolicy:       map[string]any{"workspace_base_dir": preflight.WorkspaceBaseDir},
		BudgetPolicy:          cloneMap(preflight.BudgetPolicy),
		TodayTokenUsage:       preflight.TodayTokenUsage,
		BusinessTimezone:      preflight.BusinessTimezone,
		ProviderHealthy:       preflight.ProviderHealthy,
	}
}

func projectTaskAgentHomeDir(baseDir string, projectID, projectTaskID, attemptID, employeeID uuid.UUID) string {
	return path.Join(
		strings.TrimSpace(baseDir),
		"project-tasks",
		projectID.String(),
		projectTaskID.String(),
		attemptID.String(),
		"employees",
		employeeID.String(),
	)
}

// chatDispatchNonce derives the value that scopes a chat run's one-off
// workspace directory and compatibility execution-instance id (see
// createChatRun). When the caller supplies an idempotency_key, the nonce is a
// short hash of it so retries of the same logical dispatch are deterministic;
// otherwise a fresh value is used.
func chatDispatchNonce(idempotencyKey *string) string {
	if idempotencyKey != nil {
		if trimmed := strings.TrimSpace(*idempotencyKey); trimmed != "" {
			sum := sha256.Sum256([]byte(trimmed))
			return hex.EncodeToString(sum[:])[:32]
		}
	}
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

// chatAgentHomeDir derives a chat-specific, one-off working directory rooted
// under the resolved node's workspace base dir. Unlike project task dispatch,
// chat never reuses a project task worktree — the directory is scoped to
// (project anchor, employee, dispatch nonce) and is discarded once the run
// reaches a terminal state.
func chatAgentHomeDir(baseDir string, projectID, employeeID uuid.UUID, nonce string) string {
	return path.Join(
		strings.TrimSpace(baseDir),
		"chat",
		projectID.String(),
		employeeID.String(),
		nonce,
	)
}

// chatCompatibilityExecutionInstanceID mirrors
// projectTaskCompatibilityExecutionInstanceID's role for chat dispatch: a
// deterministic placeholder satisfying the legacy non-null
// execution_instance_id column without depending on
// digital_employee_execution_instances (designed-off, see package docs on
// BindExecutionInstance).
func chatCompatibilityExecutionInstanceID(tenantID, projectID, employeeID uuid.UUID, nonce string) uuid.UUID {
	seed := strings.Join([]string{
		tenantID.String(),
		projectID.String(),
		employeeID.String(),
		"chat",
		nonce,
	}, ":")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))
}

func projectTaskCompatibilityExecutionInstanceID(req StartProjectTaskRunRequest) uuid.UUID {
	seed := strings.Join([]string{
		req.TenantID.String(),
		req.ProjectID.String(),
		req.ProjectTaskID.String(),
		req.ProjectTaskAttemptID.String(),
		req.DigitalEmployeeID.String(),
	}, ":")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(seed))
}

func projectTaskRunMetadata(req StartProjectTaskRunRequest, preflight StartProjectTaskRunPreflight) map[string]any {
	metadata := cloneMap(req.Metadata)
	metadata["source"] = "project_task_dispatch"
	metadata["project_id"] = req.ProjectID.String()
	metadata["demand_id"] = req.DemandID.String()
	metadata["project_task_id"] = req.ProjectTaskID.String()
	metadata["project_task_attempt_id"] = req.ProjectTaskAttemptID.String()
	metadata["digital_employee_id"] = req.DigitalEmployeeID.String()
	metadata["runtime_node_id"] = preflight.RuntimeNodeID.String()
	metadata["node_id"] = preflight.NodeID
	metadata["provider_type"] = preflight.ProviderType
	if strings.TrimSpace(req.WorkspaceMode) != "" {
		metadata["workspace_mode"] = strings.TrimSpace(req.WorkspaceMode)
	}
	if strings.TrimSpace(req.BaseRef) != "" {
		metadata["base_ref"] = strings.TrimSpace(req.BaseRef)
	}
	if req.ProjectGit != nil {
		metadata["project_git"] = cloneMap(req.ProjectGit)
	}
	version, _ := metadata["execution_context_packet_version"].(string)
	if strings.TrimSpace(version) == "" {
		metadata["execution_context_packet_version"] = "v1"
	}
	return metadata
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
		"provider_run_protocol":   providerRunProtocol,
		"objective":               objective,
		"prompt":                  prompt,
		"context_refs":            req.ContextRefs,
		"artifact_refs":           req.ArtifactRefs,
		"output_schema":           req.OutputSchema,
		"allowed_actions":         req.AllowedActions,
		"forbidden_actions":       req.ForbiddenActions,
		"secret_refs":             req.SecretRefs,
		"timeout_sec":             req.TimeoutSec,
		"grace_sec":               req.GraceSec,
		"metadata":                cloneMap(req.Metadata),
		"workspace_policy":        cloneMap(preflight.WorkspacePolicy),
		"session_policy":          runtimeSessionPolicyPayload(preflight.SessionPolicy),
		"runtime_selector":        cloneMap(preflight.RuntimeSelector),
		"idempotency_fingerprint": fingerprint,
		"provider_healthy":        preflight.ProviderHealthy,
	}
}

func buildStartSessionPayload(req CreateDigitalEmployeeRunRequest, objective, prompt string, preflight RunPreflight, run *DigitalEmployeeRun, configInput EmployeeConfigInput, runtimeSkills []skill.SkillRuntimeRecord, runtimeEnv []RuntimeEnvironmentVariablePayload, runtimeMCPServers []RuntimeMCPServerPayload) map[string]any {
	metadata := cloneMap(req.Metadata)
	if metadata["source"] == "project_task_dispatch" {
		metadata["runtime_node_id"] = preflight.RuntimeNodeID.String()
		version, _ := metadata["execution_context_packet_version"].(string)
		if strings.TrimSpace(version) == "" {
			metadata["execution_context_packet_version"] = "v1"
		}
	}
	return map[string]any{
		"provider_run_protocol":   providerRunProtocol,
		"tenant_id":               req.TenantID.String(),
		"team_id":                 preflight.TeamID.String(),
		"task_id":                 run.TaskID.String(),
		"run_id":                  run.ID.String(),
		"command_id":              run.CommandID,
		"digital_employee_id":     req.DigitalEmployeeID.String(),
		"execution_instance_id":   preflight.ExecutionInstanceID.String(),
		"runtime_node_id":         preflight.RuntimeNodeID.String(),
		"node_id":                 preflight.NodeID,
		"provider_type":           preflight.ProviderType,
		"agent_home_dir":          preflight.AgentHomeDir,
		"persona_memory_markdown": configInput.PersonaMemoryMarkdown,
		"capability_bindings":     cloneMap(configInput.CapabilityBindings),
		"objective":               objective,
		"prompt":                  prompt,
		"input":                   prompt,
		"context_refs":            req.ContextRefs,
		"artifact_refs":           req.ArtifactRefs,
		"output_schema":           req.OutputSchema,
		"allowed_actions":         req.AllowedActions,
		"forbidden_actions":       req.ForbiddenActions,
		"secret_refs":             req.SecretRefs,
		"timeout_sec":             req.TimeoutSec,
		"grace_sec":               req.GraceSec,
		"workspace_policy":        cloneMap(preflight.WorkspacePolicy),
		"session_policy":          runtimeSessionPolicyPayload(preflight.SessionPolicy),
		"runtime_selector":        cloneMap(preflight.RuntimeSelector),
		"skills":                  runtimeSkillsPayload(runtimeSkills),
		"environment":             runtimeEnvironmentPayload(runtimeEnv),
		"mcp_servers":             startSessionMCPServersPayload(runtimeMCPServers),
		"metadata":                metadata,
	}
}

func startSessionMCPServersPayload(servers []RuntimeMCPServerPayload) []map[string]any {
	payload := make([]map[string]any, 0, len(servers))
	for _, server := range servers {
		payload = append(payload, map[string]any{
			"server_id":          server.ServerID,
			"server_key":         server.ServerKey,
			"name":               server.Name,
			"transport":          server.Transport,
			"url":                server.URL,
			"auth_strategy":      server.AuthStrategy,
			"credential_env_var": server.CredentialEnvVar,
			"required_env_vars":  stringSliceForRuntime(server.RequiredEnvVars),
			"headers_env":        stringMapForRuntime(server.HeadersEnv),
			"source_scope":       server.SourceScope,
			"permission_scope":   mapForRuntime(server.PermissionScope),
		})
	}
	return payload
}

func stringSliceForRuntime(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func stringMapForRuntime(values map[string]string) map[string]string {
	if values == nil {
		return map[string]string{}
	}
	return values
}

func mapForRuntime(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}

func runtimeEnvironmentPayload(env []RuntimeEnvironmentVariablePayload) []map[string]any {
	out := make([]map[string]any, 0, len(env))
	for _, item := range env {
		out = append(out, map[string]any{
			"name":      item.Name,
			"value":     item.Value,
			"sensitive": item.Sensitive,
		})
	}
	return out
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

// shouldAttemptSessionResume decides whether dispatch should look up (and
// potentially inject) a prior provider session id, given the *normalized*
// session_policy that will actually be sent to the runtime (i.e. already
// passed through runtimeSessionPolicyPayload, so "mode"/"recoverable" carry
// their defaults). The default is to attempt resume; only an explicit
// ephemeral mode or recoverable=false skips it.
func shouldAttemptSessionResume(normalizedSessionPolicy map[string]any) bool {
	recoverable, _ := normalizedSessionPolicy["recoverable"].(bool)
	mode, _ := normalizedSessionPolicy["mode"].(string)
	return recoverable && !strings.EqualFold(mode, "ephemeral")
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
