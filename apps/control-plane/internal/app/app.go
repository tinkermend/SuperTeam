package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

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
	case errors.Is(err, employee.ErrEffectiveConfigRequired):
		return false
	case isRunStartIdempotencyFingerprintMismatch(err):
		return false
	default:
		return true
	}
}

func isRunStartIdempotencyFingerprintMismatch(err error) bool {
	return errors.Is(err, employee.ErrConflict) && strings.Contains(err.Error(), "idempotency fingerprint mismatch")
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

	employeeRepository := employee.NewPgRepository(q, stores.Postgres)
	skillRepository := skill.NewPgRepository(stores.Postgres, q)
	skillService := skill.NewService(skillRepository, stores.ObjectStore)
	skillInstallService := skill.NewInstallService(skillRepository, runtimeCommands, skill.InstallServiceOptions{})
	skillService.SetInstallService(skillInstallService)
	runtimeService.SetRequiredToolsResolver(skillService)
	employeeService, err := employee.NewServiceWithProvisioning(employeeRepository, runtimeCommands, skillService)
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
		projectTaskPreflights, ok := runRepository.(gateProjectTaskRunPreflightReader)
		if !ok {
			return nil, errors.New("run repository does not support project task preflight")
		}
		gateAdapter := preDispatchGateAdapter{
			employees:       employeeRepository,
			projectTaskRuns: projectTaskPreflights,
			runtimeNodes:    runtimeRepository,
			capabilities:    capabilityRepository,
		}
		coordinationStore := projectcoordination.NewProjectStoreWithApprovalsInboxAndRunStarter(
			projectRepository,
			approvalService,
			decisionProjector,
			projectTaskRunStarterAdapter{runService: runService},
		).WithDigitalEmployeeReadiness(digitalEmployeeReadinessAdapter{repository: employeeRepository}).
			WithLendingGatekeeper(lendingGatekeeperAdapter{employees: employeeRepository, lending: teamLendingRepository}).
			WithDigitalEmployeePlanningProfiles(digitalEmployeePlanningProfileAdapter{reader: employeeRepository, projectTaskRuns: projectTaskPreflights}).
			WithPreDispatchGateReaders(gateAdapter, gateAdapter)
		coordinationActivities := projectcoordination.NewActivities(coordinationStore, routePlannerFromConfig(cfg.Planner))
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
	projectService.SetDigitalEmployeeIdentityLookup(project.NewDigitalEmployeeIdentityAdapter(employeeService))
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
	runService.SetMCPLister(runtimeMCPListerAdapter{capability: capabilityService})

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
