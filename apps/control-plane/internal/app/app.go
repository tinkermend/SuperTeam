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
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/artifact"
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/capability"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/cost"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/prompttemplate"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/runtimecommand"
	"github.com/superteam/control-plane/internal/scenariotemplate"
	"github.com/superteam/control-plane/internal/skill"
	"github.com/superteam/control-plane/internal/storage"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/task"
	"github.com/superteam/control-plane/internal/teamlending"
	"github.com/superteam/control-plane/internal/tenant"
	"github.com/superteam/control-plane/internal/workflow/projectcoordination"
	temporalclient "go.temporal.io/sdk/client"
)

type lifecycleWorker interface {
	Start() error
	Stop()
}

type Container struct {
	Queries                        *queries.Queries
	TaskService                    *task.Service
	RuntimeService                 *runtimepkg.Service
	EmployeeService                *employee.Service
	ProjectService                 *project.Service
	ApprovalService                *approval.Service
	InboxService                   *inbox.Service
	ArtifactService                *artifact.Service
	EmployeeRun                    *employee.DigitalEmployeeRunService
	EmployeeRunWriteback           *employee.DigitalEmployeeRunWritebackService
	SkillService                   *skill.Service
	CapabilityService              *capability.Service
	TenantService                  *tenant.Service
	TeamLendingService             *teamlending.Service
	AuditService                   *audit.Service
	RuntimeCommands                *runtimepkg.ConnectionRegistry
	AuthService                    *auth.Service
	Authorizer                     authz.Authorizer
	AuthzCenter                    *authzcenter.Service
	Poller                         *runtimepkg.Poller
	CoordinationWorker             lifecycleWorker
	TemporalClientClose            func()
	TaskHandler                    *handlers.TaskHandler
	RuntimeHandler                 *handlers.RuntimeHandler
	RuntimeCommandWritebackHandler *handlers.RuntimeCommandWritebackHandler
	EmployeeHandler                *employee.HTTPHandler
	InboxHandler                   *inbox.HTTPHandler
	AuditHandler                   *audit.HTTPHandler
	ProjectHandler                 *project.HTTPHandler
	SkillHandler                   *skill.HTTPHandler
	CapabilityHandler              *capability.HTTPHandler
	PromptTemplateHandler          *prompttemplate.HTTPHandler
	TenantHandler                  *tenant.HTTPHandler
	TeamLendingHandler             *teamlending.HTTPHandler
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

func (a runtimeMCPListerAdapter) ListRuntimeMCPServersForRuntime(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]employee.RuntimeMCPServerPayload, error) {
	if a.capability == nil {
		return nil, nil
	}
	effective, err := a.capability.ListEffectiveMCPConfigForRuntime(ctx, tenantID, digitalEmployeeID)
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

type digitalEmployeeReadinessAdapter struct {
	repository employee.Repository
}

func (a digitalEmployeeReadinessAdapter) AreRuntimeReady(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	return a.repository.AreRuntimeReady(ctx, tenantID, employeeIDs)
}

// lendingTeamsResolver resolves the set of teams a project may currently borrow from
// (effective approved/auto_approved lending grants). Satisfied by teamlending's repository.
type lendingTeamsResolver interface {
	ListEffectiveLendingTeams(ctx context.Context, tenantID, projectID uuid.UUID) ([]uuid.UUID, error)
}

// lendingGatekeeperAdapter implements projectcoordination.LendingGatekeeper: it resolves
// each digital employee's owning team and the project's effective lending grants so the
// coordinator can exclude borrowed employees from foreign, ungranted teams.
type lendingGatekeeperAdapter struct {
	employees employee.Repository
	lending   lendingTeamsResolver
}

func (a lendingGatekeeperAdapter) ResolveEmployeeTeams(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
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

func (a lendingGatekeeperAdapter) EffectiveLendingTeams(ctx context.Context, tenantID, projectID uuid.UUID) (map[uuid.UUID]bool, error) {
	teamIDs, err := a.lending.ListEffectiveLendingTeams(ctx, tenantID, projectID)
	if err != nil {
		return nil, err
	}
	granted := make(map[uuid.UUID]bool, len(teamIDs))
	for _, teamID := range teamIDs {
		if teamID != uuid.Nil {
			granted[teamID] = true
		}
	}
	return granted, nil
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

// teamLendingInboxProjector 把待裁决的团队借调请求投影到团队负责人 inbox（D3）。
// inbox 仅作通知/提醒（无 inbox 内可执行动作）；裁决发生在团队详情「借调」tab。
type teamLendingInboxProjector struct {
	service *inbox.Service
}

func (p teamLendingInboxProjector) upsert(ctx context.Context, item teamlending.LendingInboxItem, status inbox.Status) error {
	if p.service == nil || len(item.OwnerUserIDs) == 0 {
		return nil
	}
	teamID := item.TeamID
	projectID := item.ProjectID
	title := item.Title
	if title == "" {
		// 裁决（resolve）路径不重传标题，给一个非空兜底以满足 inbox 校验。
		title = "团队借调请求"
	}
	_, err := p.service.UpsertItem(ctx, inbox.UpsertItemRequest{
		TenantID:        item.TenantID,
		TeamID:          &teamID,
		TargetUserID:    item.OwnerUserIDs[0],
		Scope:           "team",
		ItemType:        inbox.ItemTypeTeamLending,
		SourceType:      inbox.SourceTypeTeamLendingRequest,
		SourceID:        item.RequestID,
		SourceProjectID: &projectID,
		Title:           title,
		Summary:         item.Summary,
		RiskLevel:       item.RiskLevel,
		Status:          status,
		DeepLink: map[string]any{
			"route":      "/teams/" + item.TeamID.String(),
			"tab":        "lending",
			"request_id": item.RequestID.String(),
		},
	})
	return err
}

func (p teamLendingInboxProjector) UpsertLendingRequest(ctx context.Context, item teamlending.LendingInboxItem) error {
	return p.upsert(ctx, item, inbox.StatusOpen)
}

func (p teamLendingInboxProjector) ResolveLendingRequest(ctx context.Context, item teamlending.LendingInboxItem) error {
	return p.upsert(ctx, item, inbox.StatusResolved)
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
	skillInstallService := skill.NewInstallService(skillRepository, runtimeCommands, skill.InstallServiceOptions{})
	skillService.SetInstallService(skillInstallService)
	runtimeService.SetRequiredToolsResolver(skillService)
	employeeService, err := employee.NewService(employeeRepository)
	if err != nil {
		return nil, err
	}
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
	}
	decisionProjector := inbox.NewDecisionProjectorAdapter(inboxService)

	auditRepository := audit.NewPgRepository(q)
	auditService, err := audit.NewService(auditRepository)
	if err != nil {
		return nil, err
	}

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

	teamLendingRepository := teamlending.NewPgRepository(q)
	capabilityRepository := capability.NewPgRepository(q)

	coordinatorClient := project.CoordinatorSignalClient(project.NoopCoordinatorSignalClient{})
	var coordinationWorker lifecycleWorker
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
	projectTaskPreflights, _ := runRepository.(gateProjectTaskRunPreflightReader)
	if cfg.Temporal.Enabled {
		temporalClient, err := temporalclient.NewLazyClient(temporalclient.Options{
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
		).WithDigitalEmployeeReadiness(digitalEmployeeReadinessAdapter{repository: employeeRepository}).
			WithLendingGatekeeper(lendingGatekeeperAdapter{employees: employeeRepository, lending: teamLendingRepository}).
			WithDigitalEmployeePlanningProfiles(digitalEmployeePlanningProfileAdapter{reader: employeeRepository, projectTaskRuns: projectTaskPreflights}).
			WithPreDispatchGateReaders(gateAdapter, gateAdapter)
		coordinationActivities := projectcoordination.NewActivities(coordinationStore, routePlannerFromConfig(cfg.Planner))
		// Wire the adversarial-review judge client (same OpenAI-compatible seam as
		// the route planner) and the judge model id, so RunAdversarialReview /
		// AdversarialReviewForTask can decide adversarial_review criteria.
		coordinationActivities = projectcoordination.WithJudgeClient(
			coordinationActivities,
			projectcoordination.NewOpenAICompatibleChatCompletionClient(cfg.Planner.BaseURL, cfg.Planner.APIKey),
			cfg.Planner.Model,
		)
		coordinationWorker = projectcoordination.NewWorker(temporalClient, cfg.Temporal.TaskQueue, coordinationActivities)
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
	projectService.SetDigitalEmployeeIdentityLookup(project.NewDigitalEmployeeIdentityAdapter(employeeService))
	projectService.SetDigitalEmployeePlanningProfileSource(projectPlanningProfileAdapter{source: digitalEmployeePlanningProfileAdapter{reader: employeeRepository, projectTaskRuns: projectTaskPreflights}})
	projectService.SetProjectRuntimeNodeReader(projectRuntimeNodeReader{runtimeNodes: runtimePlacementNodes, runtimeCapabilities: runtimeService, connections: runtimeCommands})
	runService.SetProjectTaskNodeResolver(project.NewProjectTaskNodeResolverAdapter(projectService))
	runService.SetChatAnchorProjectValidator(project.NewChatAnchorProjectValidatorAdapter(projectService))
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
	var credentialSealer capability.CredentialSealer
	if credentialKey := os.Getenv("CONTROL_PLANE_CREDENTIAL_KEY"); credentialKey != "" {
		credentialSealer, err = capability.NewAESGCMCredentialSealer(credentialKey)
		if err != nil {
			return nil, err
		}
	}
	capabilityService := capability.NewService(capabilityRepository, credentialSealer)
	scenarioTemplateService := scenariotemplate.NewService(scenariotemplate.NewPgRepository(q))
	scenarioTemplateService.SetVocabularyRepository(scenariotemplate.NewPgVocabularyRepository(q))
	scenarioTemplateService.SetAuditRecorder(auditService)
	projectService.SetScenarioTemplateResolver(scenarioTemplateResolverAdapter{service: scenarioTemplateService})
	if coordinationStore != nil {
		coordinationStore.WithScenarioTemplateSource(scenarioTemplateSourceAdapter{service: scenarioTemplateService})
	}
	runService.SetMCPLister(runtimeMCPListerAdapter{capability: capabilityService})
	runService.SetSkillMCPDependencyLister(skillMCPDependencyListerAdapter{capability: capabilityService})
	capabilityService.SetEmployeeRuntimeSkillLister(employeeRuntimeSkillListerAdapter{skills: skillService})

	teamLendingService, err := teamlending.NewService(teamLendingRepository, auditService, teamLendingInboxProjector{service: inboxService})
	if err != nil {
		return nil, err
	}

	authRepository := auth.NewPgRepository(q, stores.Postgres)
	authService, err := auth.NewService(authRepository, auth.WithCaptchaEnabled(cfg.Auth.CaptchaEnabled))
	if err != nil {
		return nil, err
	}
	authzRepository := authz.NewPgRepository(q)
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
		authService.SetProjectTeamScopeSyncer(authz.NewOpenFGATupleSyncer(openFGAClient, authzRepository))
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
	taskHandler := handlers.NewTaskHandler(taskService)
	runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller, authorizer)
	genericCommandWritebackService := runtimecommand.NewWritebackService(runRepository)
	runtimeCommandWritebackHandler := handlers.NewRuntimeCommandWritebackHandler(handlers.NewRuntimeCommandWritebackRouter(runWritebackService, genericCommandWritebackService))
	employeeHandler := employee.NewHandlerWithRunService(employeeService, runService)
	inboxHandler := inbox.NewHandler(inboxService)
	auditHandler := audit.NewHandler(auditService)
	projectHandler := project.NewHandler(projectService)
	skillHandler := skill.NewHandler(skillService)
	capabilityHandler := capability.NewHandler(capabilityService)
	scenarioTemplateHandler := scenariotemplate.NewHandler(scenarioTemplateService)
	promptTemplateRepository := prompttemplate.NewPgRepository(q)
	promptTemplateService := prompttemplate.NewService(promptTemplateRepository, authService, nil)
	promptTemplateHandler := prompttemplate.NewHandler(promptTemplateService, authService, authorizer)
	tenantHandler := tenant.NewHandler(tenantService)
	costHandler := cost.NewHTTPHandler(cost.NewService(cost.NewPgRepository(stores.Postgres)))
	teamLendingHandler := teamlending.NewHandler(teamLendingService)
	runtimeHandler.SetConnectionRegistry(runtimeCommands)
	server := api.NewServerWithAuthzAndRuntimeSessionAuth(taskHandler, runtimeHandler, authService, authService, runtimeService, authorizer, authzCenterHandler)
	server.SetRuntimeCommandWritebackHandler(runtimeCommandWritebackHandler)
	server.SetTenantHandler(tenantHandler)
	server.SetTeamLendingHandler(teamLendingHandler)
	server.SetEmployeeHandler(employeeHandler)
	server.SetCostHandler(costHandler)
	server.SetInboxHandler(inboxHandler)
	server.SetAuditHandler(auditHandler)
	server.SetProjectHandler(projectHandler)
	server.SetSkillHandler(skillHandler)
	server.SetCapabilityHandler(capabilityHandler)
	server.SetScenarioTemplateHandler(scenarioTemplateHandler)
	server.SetPromptTemplateHandler(promptTemplateHandler)

	return &Container{
		Queries:                        q,
		TaskService:                    taskService,
		RuntimeService:                 runtimeService,
		EmployeeService:                employeeService,
		ProjectService:                 projectService,
		ApprovalService:                approvalService,
		InboxService:                   inboxService,
		ArtifactService:                artifactService,
		EmployeeRun:                    runService,
		EmployeeRunWriteback:           runWritebackService,
		SkillService:                   skillService,
		CapabilityService:              capabilityService,
		TenantService:                  tenantService,
		TeamLendingService:             teamLendingService,
		AuditService:                   auditService,
		RuntimeCommands:                runtimeCommands,
		AuthService:                    authService,
		Authorizer:                     authorizer,
		AuthzCenter:                    authzCenterService,
		Poller:                         poller,
		CoordinationWorker:             coordinationWorker,
		TemporalClientClose:            temporalClientClose,
		TaskHandler:                    taskHandler,
		RuntimeHandler:                 runtimeHandler,
		RuntimeCommandWritebackHandler: runtimeCommandWritebackHandler,
		EmployeeHandler:                employeeHandler,
		InboxHandler:                   inboxHandler,
		AuditHandler:                   auditHandler,
		ProjectHandler:                 projectHandler,
		SkillHandler:                   skillHandler,
		CapabilityHandler:              capabilityHandler,
		PromptTemplateHandler:          promptTemplateHandler,
		TenantHandler:                  tenantHandler,
		TeamLendingHandler:             teamLendingHandler,
		AuthzHandler:                   authzCenterHandler,
		Server:                         server,
	}, nil
}

func Run(ctx context.Context, cfg config.Config) error {
	stores, err := storage.NewClients(ctx, storage.Config{
		PostgresURL: cfg.Postgres.URL,
		RedisURL:    cfg.Redis.URL,
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
