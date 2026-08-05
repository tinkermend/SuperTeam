package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/artifact"
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/automation"
	"github.com/superteam/control-plane/internal/capability"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/cost"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/feishu"
	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/permission"
	"github.com/superteam/control-plane/internal/platform"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/prompttemplate"
	"github.com/superteam/control-plane/internal/retention"
	"github.com/superteam/control-plane/internal/rolevocab"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/scenariotemplate"
	"github.com/superteam/control-plane/internal/serviceauth"
	"github.com/superteam/control-plane/internal/skill"
	"github.com/superteam/control-plane/internal/storage"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/systemconfig"
	"github.com/superteam/control-plane/internal/task"
	"github.com/superteam/control-plane/internal/tenant"
	"github.com/superteam/control-plane/internal/workflow/projectcoordination"
	temporalclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
)

type lifecycleWorker interface {
	Start() error
	Stop()
}

type Container struct {
	Queries              *queries.Queries
	TaskService          *task.Service
	RuntimeService       *runtimepkg.Service
	EmployeeService      *employee.Service
	ProjectService       *project.Service
	SystemConfig         systemconfig.Reader
	ApprovalService      *approval.Service
	InboxService         *inbox.Service
	ArtifactService      *artifact.Service
	FeishuService        *feishu.Service
	EmployeeRun          *employee.DigitalEmployeeRunService
	EmployeeRunWriteback *employee.DigitalEmployeeRunWritebackService
	SkillService         *skill.Service
	CapabilityService    *capability.Service
	TenantService        *tenant.Service
	AuditService         *audit.Service
	RuntimeCommands      *runtimepkg.ConnectionRegistry
	AuthService          *auth.Service
	Authorizer           authz.Authorizer
	AuthzCenter          *authzcenter.Service
	Poller               *runtimepkg.Poller
	Retention            *retention.Service
	InboxChangeNotifier  *inbox.ChangeNotifier
	// FeishuOutboxNotifier wakes connector long-poll ListOutbox on pending inserts.
	FeishuOutboxNotifier *feishu.OutboxChangeNotifier
	// CoordinationWorker serves the whole Temporal task queue: project
	// coordination plus automation, which registers onto it rather than running a
	// second worker on the same queue.
	CoordinationWorker             lifecycleWorker
	TemporalClientClose            func()
	TaskHandler                    *handlers.TaskHandler
	RuntimeHandler                 *handlers.RuntimeHandler
	RuntimeCommandWritebackHandler *handlers.RuntimeCommandWritebackHandler
	EmployeeHandler                *employee.HTTPHandler
	InboxHandler                   *inbox.HTTPHandler
	AuditHandler                   *audit.HTTPHandler
	ProjectHandler                 *project.HTTPHandler
	AutomationHandler              *automation.HTTPHandler
	SkillHandler                   *skill.HTTPHandler
	CapabilityHandler              *capability.HTTPHandler
	PromptTemplateHandler          *prompttemplate.HTTPHandler
	TenantHandler                  *tenant.HTTPHandler
	AuthzHandler                   *authzcenter.HTTPHandler
	Server                         *api.Server
}

func NewHealthOnlyRouter() http.Handler {
	return api.NewHealthOnlyRouter()
}

type runtimeEventRecorderAdapter struct {
	runtimeService *runtimepkg.Service
}

// runtimeMCPListerAdapter bridges the capability service's effective-MCP resolution to the
// run service's RuntimeMCPLister. It excludes bindings blocked by missing env vars so only
// env-satisfied MCP servers are projected into the runtime start-session payload.
type runtimeMCPListerAdapter struct {
	capability *capability.Service
}

func (a runtimeMCPListerAdapter) ListRuntimeMCPServersForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, projectID *uuid.UUID) ([]employee.RuntimeMCPServerPayload, error) {
	if a.capability == nil {
		return nil, nil
	}
	// projectID 保留调用签名兼容，但已退役项目级 MCP 绑定（spec 2026-07-22），
	// 投影仅为员工侧（团队继承 ∪ 个人绑定）；真实项目 MCP 来自仓库 .mcp.json。
	effective, err := a.capability.ListEffectiveMCPConfigForRuntime(ctx, tenantID, digitalEmployeeID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]employee.RuntimeMCPServerPayload, 0, len(effective))
	for _, server := range effective {
		if len(server.MissingEnvVars) > 0 {
			continue // blocked: required env vars not configured on this employee
		}
		out = append(out, employee.RuntimeMCPServerPayload{
			ServerID:         server.ServerID.String(),
			ServerKey:        server.ServerKey,
			Name:             server.Name,
			Transport:        string(server.Transport),
			URL:              server.URL,
			AuthStrategy:     string(server.AuthStrategy),
			CredentialEnvVar: server.CredentialEnvVar,
			RequiredEnvVars:  server.RequiredEnvVars,
			SourceScope:      server.SourceScope,
			PermissionScope: map[string]any{
				"tool_allowlist": server.ToolAllowlist,
			},
		})
	}
	return out, nil
}

// skillMCPDependencyListerAdapter bridges the capability service's skill->MCP dependency
// lookup to the run service's SkillMCPDependencyLister. Validation-only: it never grants
// an MCP server to an employee, it only reports what a loaded skill declares it needs.
type skillMCPDependencyListerAdapter struct {
	capability *capability.Service
}

func (a skillMCPDependencyListerAdapter) ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]employee.SkillMCPDependencyRecord, error) {
	if a.capability == nil {
		return nil, nil
	}
	deps, err := a.capability.ListSkillMCPDependenciesForRuntime(ctx, tenantID, skillIDs)
	if err != nil {
		return nil, err
	}
	out := make([]employee.SkillMCPDependencyRecord, 0, len(deps))
	for _, dep := range deps {
		out = append(out, employee.SkillMCPDependencyRecord{
			SkillID:     dep.SkillID,
			MCPServerID: dep.MCPServerID.String(),
			ServerKey:   dep.ServerKey,
		})
	}
	return out, nil
}

// employeeRuntimeSkillListerAdapter bridges the skill module's runtime-effective skill lookup
// to the capability service's EmployeeRuntimeSkillLister, used by
// EvaluateEmployeeSkillMCPDependencies (employee panel data source).
type employeeRuntimeSkillListerAdapter struct {
	skills *skill.Service
}

func (a employeeRuntimeSkillListerAdapter) ListEmployeeRuntimeSkillRefs(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]capability.RuntimeSkillRef, error) {
	if a.skills == nil {
		return nil, nil
	}
	records, err := a.skills.ListSkillsForRuntime(ctx, tenantID, digitalEmployeeID)
	if err != nil {
		return nil, err
	}
	refs := make([]capability.RuntimeSkillRef, 0, len(records))
	for _, record := range records {
		refs = append(refs, capability.RuntimeSkillRef{ID: record.ID, Slug: record.Slug})
	}
	return refs, nil
}

func (a runtimeEventRecorderAdapter) RecordRuntimeEvent(ctx context.Context, req employee.RuntimeEventRecordRequest) error {
	if a.runtimeService == nil {
		return nil
	}
	return a.runtimeService.CreateRuntimeEvent(ctx, runtimepkg.CreateRuntimeEventRequest{
		TenantID:        req.TenantID,
		RuntimeNodeID:   req.RuntimeNodeID,
		NodeID:          req.NodeID,
		EventType:       runtimepkg.RuntimeEventType(req.EventType),
		Severity:        runtimepkg.RuntimeEventSeverity(req.Severity),
		Source:          runtimepkg.RuntimeEventSource(req.Source),
		Title:           req.Title,
		Description:     req.Description,
		ProviderType:    req.ProviderType,
		CorrelationType: req.CorrelationType,
		CorrelationID:   req.CorrelationID,
		Payload:         req.Payload,
	})
}

type providerEventLedgerRecorder struct {
	repository project.ProviderEventExecutionLedgerRepository
}

func (r providerEventLedgerRecorder) RecordProviderSessionEvent(ctx context.Context, req employee.ProviderSessionEventLedgerRecordRequest) error {
	_, err := r.repository.CreateProviderSessionEventLedgerEvent(ctx, req.TenantID, req.DigitalEmployeeRunID, req.ProviderSessionEventID)
	return err
}

type projectTaskRunStarterAdapter struct {
	runService *employee.DigitalEmployeeRunService
}

func (a projectTaskRunStarterAdapter) StartProjectTaskRun(ctx context.Context, req projectcoordination.StartProjectTaskRunRequest) (projectcoordination.StartProjectTaskRunResult, error) {
	if a.runService == nil {
		return projectcoordination.StartProjectTaskRunResult{}, errors.New("digital employee run service is required")
	}
	idempotencyKey := req.IdempotencyKey
	run, err := a.runService.StartProjectTaskRun(ctx, employee.StartProjectTaskRunRequest{
		TenantID:             req.TenantID,
		ProjectID:            req.ProjectID,
		DemandID:             req.DemandID,
		ProjectTaskID:        req.ProjectTaskID,
		ProjectTaskAttemptID: req.ProjectTaskAttemptID,
		DigitalEmployeeID:    req.DigitalEmployeeID,
		DispatchUserID:       req.DispatchUserID,
		Objective:            req.Objective,
		Prompt:               req.Prompt,
		IdempotencyKey:       idempotencyKey,
		Metadata:             req.Metadata,
		WorkspaceMode:        req.WorkspaceMode,
		BaseRef:              req.BaseRef,
		ProjectGit:           req.ProjectGit,
	})
	if err != nil {
		return projectcoordination.StartProjectTaskRunResult{}, &projectcoordination.ProjectTaskRunStartError{
			Retryable: runStartRetryable(err),
			Err:       err,
		}
	}
	return projectcoordination.StartProjectTaskRunResult{
		RunID:         run.RunID,
		RuntimeTaskID: run.RuntimeTaskID,
		RuntimeNodeID: run.RuntimeNodeID,
		NodeID:        run.NodeID,
		ProviderType:  run.ProviderType,
	}, nil
}

// teamBoundaryGatekeeperAdapter implements projectcoordination.TeamBoundaryGatekeeper:
// resolves each digital employee's owning team for the teamless participation gate only
// (员工必须归属某团队；跨团队项目成员不再被踢出执行池).
type teamBoundaryGatekeeperAdapter struct {
	employees employee.Repository
}

func (a teamBoundaryGatekeeperAdapter) ResolveEmployeeTeams(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	teams := make(map[uuid.UUID]uuid.UUID, len(employeeIDs))
	for _, id := range employeeIDs {
		if id == uuid.Nil {
			continue
		}
		record, err := a.employees.GetDigitalEmployee(ctx, tenantID, id)
		if err != nil {
			return nil, err
		}
		if record.TeamID != nil && *record.TeamID != uuid.Nil {
			teams[id] = *record.TeamID
		}
	}
	return teams, nil
}

func runStartRetryable(err error) bool {
	switch {
	case errors.Is(err, employee.ErrInvalidInput):
		return false
	case isRunStartIdempotencyFingerprintMismatch(err):
		return false
	case errors.Is(err, project.ErrProjectTaskPinnedNodeOffline):
		// The task is hard-pinned to a specific node that is currently offline;
		// per the anti-drift rule it must wait for that node to recover rather
		// than being retried onto a different one. This is expected to be caught
		// by the pre-dispatch gate (runtime.pinned_node_offline) before dispatch
		// is attempted; reaching here is the race-condition fallback (node went
		// offline between gate evaluation and this call).
		return false
	case errors.Is(err, project.ErrProjectTaskNoEligibleOnlineNode):
		// No eligible node is online/has capacity right now; the set can regain
		// a usable node, so this is retryable.
		return true
	default:
		return true
	}
}

func isRunStartIdempotencyFingerprintMismatch(err error) bool {
	return errors.Is(err, employee.ErrConflict) && strings.Contains(err.Error(), "idempotency fingerprint mismatch")
}

// artifactObjectStoreAdapter narrows storage.S3ObjectStore to the primitive
// signature project.ArtifactObjectStore expects (no storage types leak into
// the project package).
type artifactObjectStoreAdapter struct {
	store *storage.S3ObjectStore
}

func (a artifactObjectStoreAdapter) StatObject(ctx context.Context, key string) (bool, int64, error) {
	stat, err := a.store.StatObject(ctx, key)
	if err != nil {
		return false, 0, err
	}
	return stat.Exists, stat.SizeBytes, nil
}

func (a artifactObjectStoreAdapter) PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error) {
	return a.store.PresignPut(ctx, key, contentType, ttl)
}

func (a artifactObjectStoreAdapter) PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error) {
	return a.store.PresignGet(ctx, key, ttl)
}

type projectArtifactLocker struct {
	artifactService *artifact.Service
	projectEvents   projectEventAppender
}

type projectEventAppender interface {
	AppendProjectEvent(ctx context.Context, event project.AppendProjectEventRequest) (project.ProjectEvent, error)
}

func (l projectArtifactLocker) LockProjectArtifacts(ctx context.Context, tenantID, projectID uuid.UUID, artifactIDs []uuid.UUID) (project.ArchiveArtifactLockResult, error) {
	if l.artifactService == nil {
		return project.ArchiveArtifactLockResult{}, nil
	}
	if l.projectEvents == nil {
		return project.ArchiveArtifactLockResult{}, errors.New("project event appender is required")
	}
	event, err := l.projectEvents.AppendProjectEvent(ctx, project.AppendProjectEventRequest{
		TenantID:     tenantID,
		ProjectID:    projectID,
		EventType:    project.ProjectEventArchiveRetentionPending,
		ActorType:    "system",
		ActorID:      "project_archive_retention",
		ResourceType: strPtr("project_archive_snapshot"),
		Summary:      "项目归档工件保留锁已请求",
		Payload: map[string]any{
			"artifact_count": len(artifactIDs),
			"artifact_ids":   uuidStrings(artifactIDs),
		},
	})
	if err != nil {
		return project.ArchiveArtifactLockResult{}, err
	}
	result, err := l.artifactService.HoldProjectArchiveArtifacts(ctx, artifact.HoldProjectArchiveArtifactsRequest{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ArtifactIDs: artifactIDs,
		Reason:      "project archive snapshot",
	})
	return project.ArchiveArtifactLockResult{
		HoldIDs:     result.HoldIDs,
		ArtifactIDs: result.ArtifactIDs,
		EventID:     &event.ID,
	}, err
}

func strPtr(value string) *string {
	return &value
}

func uuidStrings(ids []uuid.UUID) []string {
	values := make([]string, 0, len(ids))
	for _, id := range ids {
		values = append(values, id.String())
	}
	return values
}

func NewContainer(stores *storage.Clients) (*Container, error) {
	return NewContainerWithConfig(stores, config.Config{})
}

func NewContainerWithConfig(stores *storage.Clients, cfg config.Config) (*Container, error) {
	if stores == nil || stores.Postgres == nil {
		return nil, errors.New("postgres client is required")
	}

	q := queries.New(stores.Postgres)

	// 系统配置中心:先于各消费方构造,审计依赖在 auditService 就绪后回填。
	systemConfigService := systemconfig.NewService(systemconfig.NewPgRepository(q))

	taskRepository := task.NewPgRepository(q)
	taskService, err := task.NewService(taskRepository)
	if err != nil {
		return nil, err
	}

	runtimeRepository := runtimepkg.NewPgRepository(q)
	runtimeService, err := runtimepkg.NewService(runtimeRepository)
	if err != nil {
		return nil, err
	}
	runtimeCommands := runtimepkg.NewConnectionRegistry()
	runtimePlacementNodes, ok := runtimeRepository.(gateRuntimePlacementNodeReader)
	if !ok {
		return nil, errors.New("runtime repository does not support project runtime placement node listing")
	}

	employeeRepository := employee.NewPgRepository(q, stores.Postgres)
	skillRepository := skill.NewPgRepository(stores.Postgres, q)
	skillService := skill.NewService(skillRepository, stores.ObjectStore)
	skillService.SetSystemConfigReader(systemConfigService)
	runtimeService.SetRequiredToolsResolver(skillService)
	// runtime 包被 api/middleware 反向依赖不能 import systemconfig,以闭包注入。
	runtimeService.SetSessionTTLResolver(func(ctx context.Context, tenantID uuid.UUID) time.Duration {
		return systemConfigService.Duration(ctx, tenantID, systemconfig.KeyRuntimeSessionTTLSeconds)
	})
	runtimeService.SetHeartbeatTimeoutResolver(func(ctx context.Context, tenantID uuid.UUID) time.Duration {
		return systemConfigService.Duration(ctx, tenantID, systemconfig.KeyRuntimeHeartbeatTimeoutSeconds)
	})
	// 平台限额快照(P2):心跳响应把系统配置中心生效值下发给 runtime。
	runtimeService.SetPlatformLimitsResolver(func(ctx context.Context, tenantID uuid.UUID) runtimepkg.PlatformLimits {
		return runtimepkg.PlatformLimits{
			ArtifactMaxFileSizeBytes:   systemConfigService.Int64(ctx, tenantID, systemconfig.KeyArtifactMaxFileSizeBytes),
			AttachmentMaxFileSizeBytes: systemConfigService.Int64(ctx, tenantID, systemconfig.KeyArtifactAttachmentMaxFileSizeBytes),
			AttachmentMaxCount:         systemConfigService.Int64(ctx, tenantID, systemconfig.KeyArtifactAttachmentMaxCount),
			AttachmentTotalMaxBytes:    systemConfigService.Int64(ctx, tenantID, systemconfig.KeyArtifactAttachmentTotalMaxBytes),
			SkillArchiveMaxBytes:       systemConfigService.Int64(ctx, tenantID, systemconfig.KeySkillArchiveUnpackMaxBytes),
			SkillArchiveMaxFileCount:   systemConfigService.Int64(ctx, tenantID, systemconfig.KeySkillArchiveUnpackMaxFileCount),
			WorkspaceBaseDir:           systemConfigService.String(ctx, tenantID, systemconfig.KeyRuntimeWorkspaceBaseDir),
		}
	})
	employeeService, err := employee.NewService(employeeRepository)
	if err != nil {
		return nil, err
	}
	employeeService.SetSystemConfigReader(systemConfigService)
	envCodec, err := employee.NewEnvironmentValueCodec(employee.EnvironmentValueCodecConfig{
		Keys:        cfg.EmployeeEnv.Keys,
		ActiveKeyID: cfg.EmployeeEnv.ActiveKeyID,
	})
	if err != nil {
		return nil, fmt.Errorf("build env encryption codec: %w", err)
	}
	employeeService.SetEnvironmentCodec(envCodec)

	inboxRepository := inbox.NewPgRepository(q)
	inboxService, err := inbox.NewService(inboxRepository)
	if err != nil {
		return nil, err
	}
	approvalProjector := inbox.NewApprovalProjectorAdapter(inboxService)

	approvalRepository := approval.NewPgRepository(q)
	approvalService, err := approval.NewServiceWithInboxProjector(approvalRepository, approvalProjector)
	if err != nil {
		return nil, err
	}

	artifactRepository := artifact.NewPgRepository(q)
	artifactService, err := artifact.NewService(artifactRepository)
	if err != nil {
		return nil, err
	}

	projectRepository := project.NewPgRepository(q, stores.Postgres)
	if pgProjectRepository, ok := projectRepository.(*project.PgRepository); ok {
		adapter := artifactObjectStoreAdapter{store: stores.ObjectStore}
		pgProjectRepository.SetArtifactObjectVerifier(adapter.StatObject)
		// §5.4.1: approval projector skips when a project decision already owns
		// the approval row (DecisionProjectorAdapter is the sole inbox writer).
		approvalProjector.SetDecisionChecker(pgProjectRepository)
	}
	decisionProjector := inbox.NewDecisionProjectorAdapter(inboxService)

	auditRepository := audit.NewPgRepository(q)
	auditService, err := audit.NewService(auditRepository)
	if err != nil {
		return nil, err
	}
	systemConfigService.SetAuditRecorder(auditService)

	runRepository := employee.NewPgRunRepository(q, stores.Postgres)
	runService, err := employee.NewDigitalEmployeeRunService(runRepository, runtimeCommands, auditService)
	if err != nil {
		return nil, err
	}
	runService.SetSkillLister(skillService)
	runService.SetRuntimeCapabilityLister(runtimeService)
	runService.SetEnvironmentLister(employeeService)
	runWritebackService, err := employee.NewDigitalEmployeeRunWritebackService(runRepository, auditService, runtimeEventRecorderAdapter{runtimeService: runtimeService})
	if err != nil {
		return nil, err
	}
	providerLedgerRepository, ok := projectRepository.(project.ProviderEventExecutionLedgerRepository)
	if !ok {
		return nil, errors.New("project repository does not support provider event ledger recording")
	}
	runWritebackService.WithExecutionLedgerRecorder(providerEventLedgerRecorder{repository: providerLedgerRepository})

	capabilityRepository := capability.NewPgRepository(q)
	var credentialSealer capability.CredentialSealer
	credentialKey := os.Getenv("CONTROL_PLANE_CREDENTIAL_KEY")
	if credentialKey == "" {
		credentialKey = cfg.Security.CredentialEncryptionKey
	}
	if credentialKey != "" {
		credentialSealer, err = capability.NewAESGCMCredentialSealer(credentialKey)
		if err != nil {
			return nil, err
		}
	}
	capabilityService := capability.NewService(capabilityRepository, credentialSealer)

	// The planning profile's capability view comes from the authoritative
	// binding tables (team inheritance included), not from the retired
	// config-revision skills/mcp_servers declaration.
	planningEffectiveSkillSlugs := func(ctx context.Context, tenantID, employeeID uuid.UUID) ([]string, error) {
		effective, err := skillService.ListEffectiveEmployeeSkills(ctx, skill.ListEffectiveEmployeeSkillsRequest{
			TenantID:          tenantID,
			DigitalEmployeeID: employeeID,
		})
		if err != nil {
			return nil, err
		}
		slugs := make([]string, 0, len(effective))
		for _, item := range effective {
			slugs = append(slugs, item.Skill.Slug)
		}
		return slugs, nil
	}
	planningEffectiveMCPServerKeys := func(ctx context.Context, tenantID, employeeID uuid.UUID) ([]string, error) {
		// 规划画像是系统上下文（无控制台用户），必须走 ForRuntime 口径——
		// 控制台口径的 ListEffectiveMCPConfig 强制要求 user_id，会把项目
		// runtime-readiness 直接打成 500。
		servers, err := capabilityService.ListEffectiveMCPConfigForRuntime(ctx, tenantID, employeeID, nil)
		if err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(servers))
		for _, server := range servers {
			keys = append(keys, server.ServerKey)
		}
		return keys, nil
	}
	planningProfileSource := digitalEmployeePlanningProfileAdapter{
		reader:                 employeeRepository,
		effectiveSkillSlugs:    planningEffectiveSkillSlugs,
		effectiveMCPServerKeys: planningEffectiveMCPServerKeys,
	}

	coordinatorClient := project.CoordinatorSignalClient(project.NoopCoordinatorSignalClient{})
	var coordinationWorker lifecycleWorker
	// One worker serves the Temporal task queue. Automation registers onto this
	// same worker further down rather than starting a second one: two workers on
	// one queue with disjoint registrations get tasks routed to them at random,
	// so each side intermittently receives types it cannot execute. See
	// automation.RegisterWith.
	var temporalWorker worker.Worker
	var temporalClient temporalclient.Client
	var temporalClientClose func()
	// coordinationStore is declared here (rather than with := inside the
	// Temporal-enabled block below) so it can be attached to projectService's
	// node resolver after projectService is constructed further down — the two
	// have a circular-looking dependency (coordinationStore needs
	// coordinatorClient, which is only finalized inside the Temporal block;
	// projectService needs that same finalized coordinatorClient, so it must be
	// constructed after the block). Activities/the worker only read the
	// resolver field at gate-evaluation time, long after container
	// construction completes, so attaching it via this pointer after the fact
	// is safe.
	var coordinationStore *projectcoordination.ProjectStore
	// castingGapDiscoverer is wired when Temporal+planner client are available
	// (batch 3 semantic expansion; optional — nil skips discoverer path).
	var castingGapDiscoverer project.CastingGapDiscoverer
	projectTaskPreflights, _ := runRepository.(gateProjectTaskRunPreflightReader)
	if cfg.Temporal.Enabled {
		temporalClient, err = temporalclient.NewLazyClient(temporalclient.Options{
			HostPort:  cfg.Temporal.Address,
			Namespace: cfg.Temporal.Namespace,
		})
		if err != nil {
			return nil, err
		}
		temporalClientClose = temporalClient.Close
		coordinatorClient = projectcoordination.NewSignalClient(temporalClient, cfg.Temporal.TaskQueue)
		var ok bool
		projectTaskPreflights, ok = runRepository.(gateProjectTaskRunPreflightReader)
		if !ok {
			return nil, errors.New("run repository does not support project task preflight")
		}
		gateAdapter := preDispatchGateAdapter{
			employees:       employeeRepository,
			projectTaskRuns: projectTaskPreflights,
			runtimeNodes:    runtimeRepository,
		}
		coordinationStore = projectcoordination.NewProjectStoreWithApprovalsInboxAndRunStarter(
			projectRepository,
			approvalService,
			decisionProjector,
			projectTaskRunStarterAdapter{runService: runService},
		).WithTeamBoundaryGatekeeper(teamBoundaryGatekeeperAdapter{employees: employeeRepository}).
			WithDigitalEmployeePlanningProfiles(planningProfileSourceWithPreflights(planningProfileSource, projectTaskPreflights)).
			WithPreDispatchGateReaders(gateAdapter, gateAdapter).
			WithPlaybookCastingLister(project.NewPgCastingRepository(q, stores.Postgres)).
			WithMaxAttemptsDefault(func(ctx context.Context, tenantID uuid.UUID) int32 {
				return int32(systemConfigService.Int64(ctx, tenantID, systemconfig.KeyProjectTaskDefaultMaxAttempts))
			})
		coordinationActivities := projectcoordination.NewActivities(coordinationStore, routePlannerFromConfig(cfg.Planner))
		// Wire the adversarial-review judge client (same OpenAI-compatible seam as
		// the route planner) and the judge model id, so RunAdversarialReview /
		// AdversarialReviewForTask can decide adversarial_review criteria. The
		// review-gate detector Activity (RunReviewGateForTask) reuses the SAME
		// client/model for its code_review LLM detector; if unwired, the LLM
		// detectors fail open and only rule detectors (secret_leak) fire.
		// Batch 3 casting-gap discoverer reuses this same client (design §9 #1).
		judgeClient := projectcoordination.NewOpenAICompatibleChatCompletionClient(cfg.Planner.BaseURL, cfg.Planner.APIKey)
		coordinationActivities = projectcoordination.WithJudgeClient(
			coordinationActivities,
			judgeClient,
			cfg.Planner.Model,
		)
		castingGapDiscoverer = projectcoordination.NewCastingGapDiscoverer(judgeClient, cfg.Planner.Model)
		temporalWorker = projectcoordination.NewWorker(temporalClient, cfg.Temporal.TaskQueue, coordinationActivities)
		coordinationWorker = temporalWorker
	}
	projectService, err := project.NewServiceWithCoordinatorApprovalsInboxAndArchiveArtifactLocker(
		projectRepository,
		coordinatorClient,
		project.NewApprovalServiceAdapter(approvalService),
		decisionProjector,
		projectArtifactLocker{artifactService: artifactService, projectEvents: projectRepository},
	)
	if err != nil {
		return nil, err
	}
	projectService.SetArtifactObjectStore(artifactObjectStoreAdapter{store: stores.ObjectStore})
	projectService.SetSystemConfigReader(systemConfigService)
	// 工件上限版本偏斜护栏(P2 spec §5):租户内存在在线旧协议节点时 presign clamp。
	projectService.SetLegacyLimitNodesChecker(runtimeService.HasOnlineLegacyLimitNodes)
	projectService.SetDigitalEmployeeIdentityLookup(project.NewDigitalEmployeeIdentityAdapter(employeeService))
	projectService.SetDigitalEmployeePlanningProfileSource(projectPlanningProfileAdapter{source: planningProfileSourceWithPreflights(planningProfileSource, projectTaskPreflights)})
	projectService.SetProjectRuntimeNodeReader(projectRuntimeNodeReader{runtimeNodes: runtimePlacementNodes, runtimeCapabilities: runtimeService, connections: runtimeCommands})
	projectService.SetRuntimeWorkspaceCommander(project.NewRuntimeWorkspaceCommanderAdapter(runtimeRepository, runtimeCommands, employeeRepository))
	projectService.SetProjectWorkspaceReceiptLister(project.NewProjectWorkspaceReceiptListerAdapter(runRepository))
	runWritebackService.WithProjectWorkspaceCommandHook(project.NewProjectWorkspaceCloneWritebackAdapter(projectService))
	runService.SetProjectTaskNodeResolver(project.NewProjectTaskNodeResolverAdapter(projectService))
	runService.SetChatAnchorProjectValidator(project.NewChatAnchorProjectValidatorAdapter(projectService))
	runService.SetProjectDispatchFactsReader(project.NewProjectDispatchFactsAdapter(projectService))
	if coordinationStore != nil {
		coordinationStore.WithProjectTaskNodeResolver(gateProjectTaskNodeResolverAdapter{service: projectService})
	}
	inboxService.SetApprovalActionResolver(inbox.NewApprovalActionAdapter(approvalService))
	inboxService.SetProjectDecisionActionResolver(inbox.NewProjectDecisionActionAdapter(projectService))

	tenantRepository := tenant.NewPgRepository(q, stores.Postgres)
	tenantService, err := tenant.NewService(tenantRepository, auditService)
	if err != nil {
		return nil, err
	}
	tenantService.SetSystemConfigReader(systemConfigService)

	// 权限中心「权限审批」域: reads the approval domain directly (category=permission),
	// never via the inbox. Generic subject registry (D5) — S2 (team privileged role)
	// wired end-to-end; S1 (employee config revision) slot registered with a nil
	// activator until the employee-config spec delivers ActivateConfigRevision (§10.1).
	// TODO(permission-center): requester_name 补名 — wire a user display-name resolver
	// (currently nil → frontend falls back to id).
	permissionRegistry := permission.NewRegistry()
	permissionRegistry.Register(permission.NewTeamRoleSubject(tenantService))
	permissionRegistry.Register(permission.NewEmployeeConfigSubject(employee.NewConfigRevisionActivatorAdapter(employeeService)))
	permissionService, err := permission.NewService(approvalService, permissionRegistry, nil)
	if err != nil {
		return nil, err
	}
	permissionRouter := permission.NewApproverRouter(tenantService)
	// 接通员工域权限审批接缝:提交治理变更产生审批请求 + 批准写回员工行。
	employeeService.SetPermissionApprovalDependencies(approvalService, permissionRouter)
	permissionRoleProducer := permission.NewPrivilegedRoleProducer(approvalService, permissionRouter, permissionRegistry, nil)
	permissionHandler := permission.NewHandler(permissionService, permissionRoleProducer)

	scenarioTemplateService := scenariotemplate.NewService(scenariotemplate.NewPgRepository(q))
	scenarioTemplateService.SetVocabularyRepository(scenariotemplate.NewPgVocabularyRepository(q))
	scenarioTemplateService.SetAuditRecorder(auditService)
	roleVocabularyService := rolevocab.NewService(q)
	scenarioTemplateService.SetRoleVocabularyValidator(roleVocabularyService)
	// Role governance console: holder_count on template role-view (P0c).
	scenarioTemplateService.SetRoleHolderCounter(roleHolderCounterAdapter{q: q})
	projectService.SetScenarioTemplateResolver(scenarioTemplateResolverAdapter{service: scenarioTemplateService})
	projectService.SetRoleVocabulary(roleVocabularyService)
	// Batch 3: active role rows for semantic gap discoverer prompts + planner inject.
	roleVocabLister := roleVocabularyListerAdapter{svc: roleVocabularyService}
	projectService.SetRoleVocabularyLister(roleVocabLister)
	projectService.SetCastingGapDiscoverer(castingGapDiscoverer)
	projectService.SetScenarioTemplateSpecSource(scenarioTemplateSpecSourceAdapter{service: scenarioTemplateService})
	projectService.SetPlaybookTemplateLister(scenarioTemplateSpecSourceAdapter{service: scenarioTemplateService})
	projectService.SetCastingRepository(project.NewPgCastingRepository(q, stores.Postgres))
	projectService.SetDigitalEmployeeRoleSource(project.NewPgDigitalEmployeeRoleSource(q))
	// 能力词表是模板 required_capabilities 与员工 external_capabilities 的共用
	// 词表；此前只有模板侧校验，员工侧随便写。两侧抽同一份词表才算有词表。
	employeeService.SetCapabilityVocabularyValidator(scenarioTemplateService)
	employeeService.SetRoleVocabularyValidator(roleVocabularyService)
	employeeService.SetEmployeeRoleStore(employee.NewPgEmployeeRoleStore(q))
	if coordinationStore != nil {
		coordinationStore.WithScenarioTemplateSource(scenarioTemplateSourceAdapter{service: scenarioTemplateService})
		// G9 coordinator path: after accepted task completion, propose casting_expansion
		// when readiness still needs deeper-exit roles (vocab-constrained).
		// Batch 3: when casting is complete, projectService runs the semantic discoverer.
		coordinationStore.WithCastingExpansionProposer(projectService)
		// Planner role vocabulary injection (P1).
		coordinationStore.WithRoleVocabularySource(roleVocabLister)
	}
	runService.SetMCPLister(runtimeMCPListerAdapter{capability: capabilityService})
	runService.SetSkillMCPDependencyLister(skillMCPDependencyListerAdapter{capability: capabilityService})
	capabilityService.SetEmployeeRuntimeSkillLister(employeeRuntimeSkillListerAdapter{skills: skillService})

	authRepository := auth.NewPgRepository(q, stores.Postgres)
	authService, err := auth.NewService(authRepository, auth.WithCaptchaEnabled(cfg.Auth.CaptchaEnabled))
	if err != nil {
		return nil, err
	}
	// 登录先于租户上下文,登录会话 TTL 固定读平台默认租户的配置。
	authService.SetSessionTTLResolver(func(ctx context.Context) time.Duration {
		return systemConfigService.Duration(ctx, platform.DefaultTenantID, systemconfig.KeyAuthSessionTTLSeconds)
	})
	authzRepository := authz.NewPgRepository(q)
	// runtime scope 活性窗口跟随可配心跳超时(authz 不 import systemconfig,闭包注入)。
	authzRepository.SetHeartbeatTimeoutResolver(func(ctx context.Context, tenantID uuid.UUID) time.Duration {
		return systemConfigService.Duration(ctx, tenantID, systemconfig.KeyRuntimeHeartbeatTimeoutSeconds)
	})
	authzRecorder := authz.NewOperationLogDecisionRecorder(q)
	dbAuthorizer := authz.NewDBAuthorizer(authzRepository, authzRecorder)
	var authorizer authz.Authorizer = dbAuthorizer
	if cfg.Authz.Engine == "openfga_shadow" || cfg.Authz.Engine == "openfga" {
		openFGAClient := authz.NewOpenFGAHTTPClient(authz.OpenFGAClientConfig{
			APIURL:   cfg.Authz.OpenFGA.APIURL,
			StoreID:  cfg.Authz.OpenFGA.StoreID,
			ModelID:  cfg.Authz.OpenFGA.ModelID,
			APIToken: cfg.Authz.OpenFGA.APIToken,
		})
		tupleSyncer := authz.NewOpenFGATupleSyncer(openFGAClient, authzRepository)
		authService.SetProjectTeamScopeSyncer(tupleSyncer)
		authService.SetMembershipSyncer(tupleSyncer)
		switch cfg.Authz.Engine {
		case "openfga_shadow":
			authorizer = authz.NewShadowAuthorizer(dbAuthorizer, openFGAClient, authz.ShadowOptions{
				Recorder: authzRecorder,
				StoreID:  cfg.Authz.OpenFGA.StoreID,
				ModelID:  cfg.Authz.OpenFGA.ModelID,
			})
			if teamScopeAuthorizer, ok := projectRepository.(project.ProjectTeamScopeAuthorizer); ok {
				projectService.SetTeamScopeAuthorizer(project.NewTeamScopeShadowAuthorizer(teamScopeAuthorizer, openFGAClient, project.TeamScopeShadowOptions{
					Recorder: authzRecorder,
					StoreID:  cfg.Authz.OpenFGA.StoreID,
					ModelID:  cfg.Authz.OpenFGA.ModelID,
				}))
			}
		case "openfga":
			authorizer = authz.NewOpenFGAAuthorizer(openFGAClient, authz.OpenFGAAuthorizerOptions{
				Recorder: authzRecorder,
				StoreID:  cfg.Authz.OpenFGA.StoreID,
				ModelID:  cfg.Authz.OpenFGA.ModelID,
			})
		}
	}
	authzCenterRepository := authzcenter.NewPgRepository(q)
	authzCenterService := authzcenter.NewService(authzCenterRepository, authorizer)
	authzCenterHandler := authzcenter.NewHandler(authzCenterService, authService)

	poller := runtimepkg.NewPoller()
	// 数据保留作业(P1-B):此前 append-only 表无任何清理通道。singleton 用会话级
	// advisory lock 保证多副本下只有一个进程真的删,待 leader 选举落地后可替换。
	retentionService := retention.NewService(q, systemConfigService, retention.NewPgSingleton(stores.Postgres))
	taskHandler := handlers.NewTaskHandler(taskService)
	runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller, authorizer)
	// 通用 runtime 命令回执写回(runtimecommand 包)随 install_skills 命令一并
	// 退役:会话命令回执由 runWritebackService 独家承接。
	runtimeCommandWritebackHandler := handlers.NewRuntimeCommandWritebackHandler(runWritebackService)
	employeeHandler := employee.NewHandlerWithRunService(employeeService, runService)
	inboxHandler := inbox.NewHandler(inboxService)
	// 收件箱 SSE 由 LISTEN/NOTIFY 驱动(P1-C2):一条常驻监听连接扇出给本进程所有流,
	// 取代每流每 2 秒各打一次库的轮询;轮询降为兜底(NOTIFY 不保证送达)。
	inboxChangeNotifier := inbox.NewChangeNotifier(stores.Postgres)
	inboxHandler.SetChangeNotifier(inboxChangeNotifier)
	auditHandler := audit.NewHandler(auditService)
	projectHandler := project.NewHandler(projectService)
	automationRepo := automation.NewPgRepository(q)
	automationScheduler := automation.NewTemporalScheduler(temporalClient, cfg.Temporal.TaskQueue)
	automationService := automation.NewService(
		automationRepo,
		automation.NewProjectServiceGateway(projectService),
		automation.NewDemandSubmitter(projectService),
		automation.NewChatRunner(runService),
		automationScheduler,
	)
	projectService.SetAutomationActorRemover(automationService)
	projectService.SetAutomationProjectCascade(automationService)
	authService.SetUserDeactivatedHook(automationUserDeactivatedHook{service: automationService})
	if temporalWorker != nil {
		// Same task queue as coordination, so the same worker must serve both.
		automation.RegisterWith(temporalWorker, automation.NewActivities(automationService))
	}
	automationHandler := automation.NewHandler(automationService)
	skillHandler := skill.NewHandler(skillService)
	skillHandler.SetSystemConfigReader(systemConfigService)
	capabilityHandler := capability.NewHandler(capabilityService)
	systemConfigHandler := systemconfig.NewHandler(systemConfigService)
	scenarioTemplateHandler := scenariotemplate.NewHandler(scenarioTemplateService)
	roleVocabularyHandler := rolevocab.NewHandler(roleVocabularyService)
	promptTemplateRepository := prompttemplate.NewPgRepository(q)
	promptTemplateService := prompttemplate.NewService(promptTemplateRepository, authService, nil)
	promptTemplateHandler := prompttemplate.NewHandler(promptTemplateService, authService, authorizer)
	tenantHandler := tenant.NewHandler(tenantService)
	costHandler := cost.NewHTTPHandler(cost.NewService(cost.NewPgRepository(stores.Postgres)))
	serviceAuthCore := serviceauth.NewService(serviceauth.NewPgRepository(q))
	serviceTokenHandler := serviceauth.NewHTTPHandler(serviceAuthCore)
	feishuService := feishu.NewService(feishu.NewPgRepository(q), credentialSealer)
	feishuService.SetClient(feishu.NewClient(os.Getenv("FEISHU_API_BASE_URL")))
	if stores.Redis != nil {
		feishuService.SetHeartbeatStore(feishu.NewRedisHeartbeatStore(stores.Redis))
		feishuService.SetOAuthStateStore(feishu.NewRedisOAuthStateStore(stores.Redis))
	}
	feishuService.SetUserLister(feishuUserListerAdapter{auth: authService})
	feishuPublicOrigin := os.Getenv("CONTROL_PLANE_PUBLIC_ORIGIN")
	if feishuPublicOrigin == "" {
		feishuPublicOrigin = "http://127.0.0.1:8080"
	}
	feishuWebOrigin := os.Getenv("CONTROL_PLANE_WEB_ORIGIN")
	if feishuWebOrigin == "" {
		feishuWebOrigin = "http://127.0.0.1:3100"
	}
	feishuService.SetOAuthOrigins(feishuPublicOrigin, feishuWebOrigin)
	feishuConnectorHandler := feishu.NewConnectorHTTPHandler(feishuService)
	feishuConnectorHandler.SetOutboxRepository(feishu.NewPgRepository(q))
	feishuProjectGateway := feishuProjectGatewayAdapter{
		q:        q,
		projects: projectService,
		repo:     projectRepository,
	}
	feishuConnectorHandler.SetProjectGateway(feishuProjectGateway)
	// outbox ack 竞态恢复:发卡 ack 晚于 resolve 时补 card_update。
	feishuConnectorHandler.SetDecisionCardTerminalizer(feishuProjectGateway)
	// outbox 长轮询唤醒:pending 入队 → NOTIFY → ListOutbox wait 返回(PR-2)。
	feishuOutboxNotifier := feishu.NewOutboxChangeNotifier(stores.Postgres)
	feishuConnectorHandler.SetOutboxChangeNotifier(feishuOutboxNotifier)
	feishuAdminHandler := feishu.NewAdminHTTPHandler(feishuService)
	feishuOAuthHandler := feishu.NewOAuthHTTPHandler(feishuService)
	runtimeHandler.SetConnectionRegistry(runtimeCommands)
	server := api.NewServerWithAuthzAndRuntimeSessionAuth(taskHandler, runtimeHandler, authService, authService, runtimeService, authorizer, authzCenterHandler)
	server.SetAllowedOrigins(cfg.ResolvedAllowedOrigins())
	server.SetRuntimeCommandWritebackHandler(runtimeCommandWritebackHandler)
	server.SetTenantHandler(tenantHandler)
	server.SetEmployeeHandler(employeeHandler)
	server.SetCostHandler(costHandler)
	server.SetInboxHandler(inboxHandler)
	server.SetPermissionHandler(permissionHandler)
	server.SetAuditHandler(auditHandler)
	server.SetProjectHandler(projectHandler)
	server.SetAutomationHandler(automationHandler)
	server.SetSkillHandler(skillHandler)
	server.SetCapabilityHandler(capabilityHandler)
	server.SetSystemConfigHandler(systemConfigHandler)
	server.SetScenarioTemplateHandler(scenarioTemplateHandler)
	server.SetRoleVocabularyHandler(roleVocabularyHandler)
	server.SetPromptTemplateHandler(promptTemplateHandler)
	server.SetServiceTokenHandler(serviceTokenHandler)
	server.SetFeishuHandlers(feishuConnectorHandler, feishuAdminHandler)
	server.SetFeishuOAuthHandler(feishuOAuthHandler)
	server.SetServiceAuth(serviceAuthMiddlewareAdapter{core: serviceAuthCore}, feishuService)

	return &Container{
		Queries:                        q,
		TaskService:                    taskService,
		RuntimeService:                 runtimeService,
		EmployeeService:                employeeService,
		ProjectService:                 projectService,
		SystemConfig:                   systemConfigService,
		ApprovalService:                approvalService,
		InboxService:                   inboxService,
		ArtifactService:                artifactService,
		FeishuService:                  feishuService,
		EmployeeRun:                    runService,
		EmployeeRunWriteback:           runWritebackService,
		SkillService:                   skillService,
		CapabilityService:              capabilityService,
		TenantService:                  tenantService,
		AuditService:                   auditService,
		RuntimeCommands:                runtimeCommands,
		AuthService:                    authService,
		Authorizer:                     authorizer,
		AuthzCenter:                    authzCenterService,
		Poller:                         poller,
		Retention:                      retentionService,
		InboxChangeNotifier:            inboxChangeNotifier,
		FeishuOutboxNotifier:           feishuOutboxNotifier,
		CoordinationWorker:             coordinationWorker,
		TemporalClientClose:            temporalClientClose,
		TaskHandler:                    taskHandler,
		RuntimeHandler:                 runtimeHandler,
		RuntimeCommandWritebackHandler: runtimeCommandWritebackHandler,
		EmployeeHandler:                employeeHandler,
		InboxHandler:                   inboxHandler,
		AuditHandler:                   auditHandler,
		ProjectHandler:                 projectHandler,
		AutomationHandler:              automationHandler,
		SkillHandler:                   skillHandler,
		CapabilityHandler:              capabilityHandler,
		PromptTemplateHandler:          promptTemplateHandler,
		TenantHandler:                  tenantHandler,
		AuthzHandler:                   authzCenterHandler,
		Server:                         server,
	}, nil
}

func Run(ctx context.Context, cfg config.Config) error {
	stores, err := storage.NewClients(ctx, storage.Config{
		PostgresURL: cfg.Postgres.URL,
		Postgres: storage.PostgresPoolConfig{
			MaxConns:          cfg.Postgres.MaxConns,
			MinConns:          cfg.Postgres.MinConns,
			MaxConnLifetime:   cfg.Postgres.MaxConnLifetime,
			MaxConnIdleTime:   cfg.Postgres.MaxConnIdleTime,
			HealthCheckPeriod: cfg.Postgres.HealthCheckPeriod,
		},
		RedisURL: cfg.Redis.URL,
		ObjectStore: storage.ObjectStoreConfig{
			Endpoint:        cfg.ObjectStore.Endpoint,
			Region:          cfg.ObjectStore.Region,
			Bucket:          cfg.ObjectStore.Bucket,
			AccessKeyID:     cfg.ObjectStore.AccessKeyID,
			SecretAccessKey: cfg.ObjectStore.SecretAccessKey,
			ForcePathStyle:  cfg.ObjectStore.ForcePathStyle,
		},
	})
	if err != nil {
		return err
	}
	defer stores.Close()

	container, err := NewContainerWithConfig(stores, cfg)
	if err != nil {
		return err
	}
	return runContainer(ctx, container, cfg.HTTP.Addr)
}

func runContainer(ctx context.Context, container *Container, addr string) error {
	if container.TemporalClientClose != nil {
		defer container.TemporalClientClose()
	}
	if container.CoordinationWorker != nil {
		if err := container.CoordinationWorker.Start(); err != nil {
			return err
		}
		defer container.CoordinationWorker.Stop()
	}
	if container.EmployeeRun != nil {
		// 调度韧性:预确认态 run 的超时看门狗(残债交接 §1 第 2 层)。
		go container.EmployeeRun.StartStaleRunWatchdog(ctx, time.Minute)
	}
	if container.TenantService != nil && container.InboxService != nil {
		// 团队待确认删除滞留催办(生命周期收敛 P2:永不自动物理删,超时提醒)。
		go startTeamPendingDeleteReminder(ctx, container.TenantService, container.InboxService)
	}
	if container.InboxService != nil {
		// 飞书通道失联看门狗(接入管理 P1:心跳超时→收件箱告警,恢复自动 resolve)。
		// feishuService 经 container 间接持有;这里用 server 侧已装配的 admin 同源 service。
		if fs := container.FeishuService; fs != nil {
			go startFeishuChannelWatchdog(ctx, fs, container.InboxService)
		}
	}
	if container.ProjectService != nil && container.SystemConfig != nil {
		// 僵尸任务收敛看门狗(卡死任务收敛 spec P1):孤儿 running 任务兜底 + 既有 attempt 恢复接线。
		go startStuckTaskReconciler(ctx, container.ProjectService, container.SystemConfig)
	}
	if container.InboxChangeNotifier != nil {
		go container.InboxChangeNotifier.Start(ctx)
	}
	if container.FeishuOutboxNotifier != nil {
		go container.FeishuOutboxNotifier.Start(ctx)
	}
	if container.Retention != nil {
		// 数据保留作业(P1-B):append-only 表此前无任何清理通道。作业自带 advisory lock
		// 单跑保护,多副本下只有一个进程真的删。
		go container.Retention.Start(ctx, retention.SweepInterval)
	}
	stopWatching := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			container.Poller.Close()
		case <-stopWatching:
		}
	}()
	defer func() {
		close(stopWatching)
		container.Poller.Close()
	}()

	return container.Server.ListenAndServe(ctx, addr)
}

func routePlannerFromConfig(cfg config.PlannerConfig) projectcoordination.RoutePlanner {
	// Planning is reasoning-model only; there is no non-reasoning fallback, so any
	// configured provider resolves to the reasoning planner. Misconfiguration then
	// surfaces as a planning error instead of a silent heuristic fan-out.
	return projectcoordination.NewOpenAICompatibleRoutePlanner(projectcoordination.OpenAICompatiblePlannerConfig{
		APIKey:      cfg.APIKey,
		BaseURL:     cfg.BaseURL,
		Model:       cfg.Model,
		MaxTokens:   cfg.MaxTokens,
		Temperature: cfg.Temperature,
		MaxAttempts: cfg.MaxAttempts,
	})
}

type scenarioTemplateResolverAdapter struct {
	service *scenariotemplate.Service
}

func (a scenarioTemplateResolverAdapter) ResolveScenarioTemplate(ctx context.Context, tenantID uuid.UUID, key string) (project.ScenarioTemplateBinding, error) {
	template, err := a.service.GetByKey(ctx, tenantID, key)
	if err != nil {
		return project.ScenarioTemplateBinding{}, err
	}
	return project.ScenarioTemplateBinding{Key: template.Key, Name: template.Name, Status: template.Status}, nil
}

func (a scenarioTemplateResolverAdapter) ResolveScenarioTemplateProduceKinds(ctx context.Context, tenantID uuid.UUID, key string) ([]string, error) {
	return a.service.ProduceKinds(ctx, tenantID, key)
}

// scenarioTemplateSpecSourceAdapter feeds playbook-readiness with parsed specs.
type scenarioTemplateSpecSourceAdapter struct {
	service *scenariotemplate.Service
}

func (a scenarioTemplateSpecSourceAdapter) GetParsedSpec(ctx context.Context, tenantID uuid.UUID, key string) (scenariotemplate.SpecV2, string, error) {
	template, err := a.service.GetByKey(ctx, tenantID, key)
	if err != nil {
		return scenariotemplate.SpecV2{}, "", err
	}
	if template.Status != "active" {
		return scenariotemplate.SpecV2{}, "", fmt.Errorf("scenario template %q is %s", key, template.Status)
	}
	spec, err := scenariotemplate.ParseSpec(template.Spec)
	if err != nil {
		return scenariotemplate.SpecV2{}, "", err
	}
	return spec, template.Name, nil
}

func (a scenarioTemplateSpecSourceAdapter) ListActiveTemplateKeys(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	templates, err := a.service.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(templates))
	for _, t := range templates {
		if t.Status != "active" {
			continue
		}
		keys = append(keys, t.Key)
	}
	return keys, nil
}

type scenarioTemplateSourceAdapter struct {
	service *scenariotemplate.Service
}

func (a scenarioTemplateSourceAdapter) GetScenarioTemplateSnapshot(ctx context.Context, tenantID uuid.UUID, key string) (projectcoordination.ScenarioTemplateSnapshot, error) {
	template, err := a.service.GetByKey(ctx, tenantID, key)
	if err != nil {
		return projectcoordination.ScenarioTemplateSnapshot{}, err
	}
	if template.Status != "active" {
		return projectcoordination.ScenarioTemplateSnapshot{}, fmt.Errorf("scenario template %q is %s", key, template.Status)
	}
	return projectcoordination.ScenarioTemplateSnapshot{Key: template.Key, Name: template.Name, Version: template.ActiveVersion, Spec: template.Spec}, nil
}

// roleHolderCounterAdapter backs scenario-template role-view holder_count
// (role governance console P0c) without scenariotemplate importing queries.
type roleHolderCounterAdapter struct {
	q *queries.Queries
}

func (a roleHolderCounterAdapter) CountActiveHolders(ctx context.Context, tenantID uuid.UUID, roleKey string) (int, error) {
	n, err := a.q.CountEmployeesHoldingRole(ctx, queries.CountEmployeesHoldingRoleParams{
		TenantID: tenantID,
		RoleKey:  roleKey,
	})
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// roleVocabularyListerAdapter exposes active role rows to project service
// (gap discoverer) and projectcoordination (planner inject) without those
// packages importing rolevocab.
type roleVocabularyListerAdapter struct {
	svc *rolevocab.Service
}

func (a roleVocabularyListerAdapter) ListActiveRoleRows(ctx context.Context, tenantID uuid.UUID) ([]project.RoleVocabularyRow, error) {
	entries, err := a.svc.ListActive(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]project.RoleVocabularyRow, 0, len(entries))
	for _, e := range entries {
		out = append(out, project.RoleVocabularyRow{
			RoleKey:     e.RoleKey,
			Title:       e.Title,
			Description: e.Description,
		})
	}
	return out, nil
}

// ListActiveRoleVocabulary implements projectcoordination.RoleVocabularySource.
func (a roleVocabularyListerAdapter) ListActiveRoleVocabulary(ctx context.Context, tenantID uuid.UUID) ([]projectcoordination.RoleVocabularyPromptEntry, error) {
	rows, err := a.ListActiveRoleRows(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]projectcoordination.RoleVocabularyPromptEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, projectcoordination.RoleVocabularyPromptEntry{
			RoleKey:     r.RoleKey,
			Title:       r.Title,
			Description: r.Description,
		})
	}
	return out, nil
}

// serviceAuthMiddlewareAdapter 把 serviceauth.Service 适配成中间件需要的最小视图。
type serviceAuthMiddlewareAdapter struct {
	core *serviceauth.Service
}

func (a serviceAuthMiddlewareAdapter) ValidateServiceToken(ctx context.Context, serviceName, token string) (middleware.ServiceIdentity, error) {
	validated, err := a.core.ValidateServiceToken(ctx, serviceName, token)
	if err != nil {
		return middleware.ServiceIdentity{}, err
	}
	return middleware.ServiceIdentity{TokenID: validated.ID, TenantID: validated.TenantID}, nil
}

// feishuUserListerAdapter 把 auth 用户目录适配为通讯录反查所需的联系方式来源。
type feishuUserListerAdapter struct {
	auth *auth.Service
}

func (a feishuUserListerAdapter) ListActiveUsersWithContact(ctx context.Context) ([]feishu.UserContact, error) {
	users, err := a.auth.ListUsers(ctx, auth.ListUsersFilter{Status: "active", Limit: 1000})
	if err != nil {
		return nil, err
	}
	out := make([]feishu.UserContact, 0, len(users))
	for _, user := range users {
		if user.Email == "" && user.Mobile == "" {
			continue
		}
		out = append(out, feishu.UserContact{UserID: user.ID, Email: user.Email, Mobile: user.Mobile})
	}
	return out, nil
}

// feishuProjectGatewayAdapter 把 connector 业务动作接到项目域。
type feishuProjectGatewayAdapter struct {
	q        *queries.Queries
	projects *project.Service
	repo     project.Repository
}

func (a feishuProjectGatewayAdapter) ListProjectsForHumanMember(ctx context.Context, tenantID, userID uuid.UUID) ([]feishu.ProjectRef, error) {
	rows, err := a.q.ListProjectsForHumanMember(ctx, queries.ListProjectsForHumanMemberParams{
		TenantID:    tenantID,
		ActorUserID: userID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]feishu.ProjectRef, 0, len(rows))
	for _, row := range rows {
		out = append(out, feishu.ProjectRef{ID: row.ID, Name: row.Name})
	}
	return out, nil
}

func (a feishuProjectGatewayAdapter) SubmitDemand(ctx context.Context, tenantID, projectID, userID uuid.UUID, title, content, mode string) (uuid.UUID, string, error) {
	demand, err := a.projects.SubmitDemand(ctx, project.SubmitProjectDemandRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		SubmittedByUserID: userID,
		Title:             title,
		Content:           content,
		SourceType:        project.DemandSourceType("feishu"),
		CoordinationMode:  mode,
	})
	if err != nil {
		if errors.Is(err, project.ErrInvalidProject) || errors.Is(err, project.ErrInvalidCoordinationMode) {
			return uuid.Nil, "", fmt.Errorf("%w: %v", feishu.ErrGatewayBadInput, err)
		}
		return uuid.Nil, "", err
	}
	return demand.ID, string(demand.Status), nil
}

func (a feishuProjectGatewayAdapter) ResolveDecision(ctx context.Context, tenantID, projectID, decisionID, userID uuid.UUID, decision, comment string) (bool, error) {
	_, err := a.projects.ResolveDecision(ctx, project.ResolveDecisionRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
		DecidedByUserID:   userID,
		Decision:          decision,
		Comment:           comment,
		Channel:           "feishu",
	})
	if err == nil {
		return false, nil
	}
	if errors.Is(err, project.ErrProjectDecisionForbidden) {
		return false, feishu.ErrGatewayForbidden
	}
	if errors.Is(err, project.ErrInvalidProject) {
		// 已终态且异值 → 已由他人处理;其余按坏输入。
		if existing, getErr := a.repo.GetDecisionRequest(ctx, tenantID, projectID, decisionID); getErr == nil && existing.StatusSnapshot != "pending" {
			return true, nil
		}
		return false, fmt.Errorf("%w: %v", feishu.ErrGatewayBadInput, err)
	}
	return false, err
}

// SignDemandCriterion on-behalf-of 逐条签署验收判据(卡内签署),签署后取全量判据
// verdict 与刷新决策卡快照供飞书整卡重渲染。判权在 SignDemandCriterionVerdict 内部
// (any-of-N 合格处理人)。
func (a feishuProjectGatewayAdapter) SignDemandCriterion(ctx context.Context, req feishu.SignCriterionGatewayRequest) (*feishu.SignCriterionOutcome, error) {
	result, err := a.projects.SignDemandCriterionVerdict(ctx, project.SignDemandCriterionVerdictRequest{
		TenantID:    req.TenantID,
		DemandID:    req.DemandID,
		ActorUserID: req.ActorUserID,
		CriterionID: req.CriterionID,
		Verdict:     req.Verdict,
		Reason:      req.Reason,
		Channel:     "feishu",
	})
	if err != nil {
		switch {
		case errors.Is(err, project.ErrProjectDecisionForbidden):
			return nil, feishu.ErrGatewayForbidden
		case errors.Is(err, project.ErrProjectConflict):
			return nil, feishu.ErrGatewayConflict
		case errors.Is(err, project.ErrInvalidProject):
			return nil, fmt.Errorf("%w: %v", feishu.ErrGatewayBadInput, err)
		default:
			return nil, err
		}
	}
	outcome := &feishu.SignCriterionOutcome{
		DemandStatus: string(result.DemandStatus),
		Signed:       result.Signed,
		Total:        result.Total,
		Remaining:    result.Remaining,
	}
	// verdict 覆盖与卡快照 best-effort:取不到只降级为薄重渲染,不影响签署本身。
	if revisions, listErr := a.repo.ListPlanRevisionsForDemand(ctx, req.TenantID, req.ProjectID, req.DemandID); listErr == nil {
		if revisionID := project.CurrentEffectivePlanRevisionID(revisions); revisionID != uuid.Nil {
			criteria, criteriaErr := a.repo.ListDemandAcceptanceCriteria(ctx, req.TenantID, req.DemandID, revisionID)
			verdicts, verdictsErr := a.repo.ListDemandCriterionVerdicts(ctx, req.TenantID, req.DemandID, revisionID)
			if criteriaErr == nil && verdictsErr == nil {
				outcome.CriterionVerdicts = project.EffectiveCriterionVerdicts(criteria, verdicts)
			}
		}
	}
	if req.DecisionID != uuid.Nil {
		if card, cardErr := a.DecisionCardSnapshot(ctx, req.TenantID, req.ProjectID, req.DecisionID); cardErr == nil {
			outcome.CardPayload = card
		}
	}
	return outcome, nil
}

// DecisionCardSnapshot 返回与 outbox decision_card 同源的决策卡投影,含终态信息;
// 供 connector resolve 后即时渲染保留详情的终态卡。
func (a feishuProjectGatewayAdapter) DecisionCardSnapshot(ctx context.Context, tenantID, projectID, decisionID uuid.UUID) (map[string]any, error) {
	decision, err := a.repo.GetDecisionRequest(ctx, tenantID, projectID, decisionID)
	if err != nil {
		return nil, err
	}
	projectName := ""
	if projectRow, getErr := a.q.GetProject(ctx, queries.GetProjectParams{TenantID: tenantID, ID: projectID}); getErr == nil {
		projectName = projectRow.Name
	}
	payload := project.BuildDecisionCardPayload(ctx, a.q, decision, projectName)
	if decision.StatusSnapshot != "" && decision.StatusSnapshot != "pending" {
		payload["resolved_status"] = decision.StatusSnapshot
		if decision.ResolvedAt != nil {
			payload["resolved_at"] = decision.ResolvedAt.Format(time.RFC3339)
		}
	}
	return payload, nil
}

// EnsureDecisionCardsTerminal 实现 feishu.DecisionCardTerminalizer:
// outbox ack 在 resolve 之后才回填 message_id 时,补入 card_update 收敛飞书活卡。
func (a feishuProjectGatewayAdapter) EnsureDecisionCardsTerminal(ctx context.Context, tenantID, projectID, decisionID uuid.UUID) error {
	if a.repo == nil || decisionID == uuid.Nil {
		return nil
	}
	if projectID == uuid.Nil {
		return nil
	}
	decision, err := a.repo.GetDecisionRequest(ctx, tenantID, projectID, decisionID)
	if err != nil {
		return err
	}
	return a.repo.EnsureDecisionCardsTerminal(ctx, decision, uuid.Nil, "")
}

type automationUserDeactivatedHook struct {
	service *automation.Service
}

func (h automationUserDeactivatedHook) OnUserDeactivated(ctx context.Context, userID uuid.UUID) error {
	if h.service == nil {
		return nil
	}
	return h.service.DisableForActorDeactivated(ctx, platform.DefaultTenantID, userID)
}
