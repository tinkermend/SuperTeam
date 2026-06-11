package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/superteam/control-plane/internal/api"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/approval"
	"github.com/superteam/control-plane/internal/audit"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/project"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/skill"
	"github.com/superteam/control-plane/internal/storage"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/task"
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
	EmployeeRun                    *employee.DigitalEmployeeRunService
	EmployeeRunWriteback           *employee.DigitalEmployeeRunWritebackService
	SkillService                   *skill.Service
	TenantService                  *tenant.Service
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
	ProjectHandler                 *project.HTTPHandler
	SkillHandler                   *skill.HTTPHandler
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
	employeeService, err := employee.NewServiceWithProvisioning(employeeRepository, runtimeCommands)
	if err != nil {
		return nil, err
	}

	projectRepository := project.NewPgRepository(q, stores.Postgres)
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
		coordinationStore := projectcoordination.NewProjectStore(projectRepository)
		coordinationActivities := projectcoordination.NewActivities(coordinationStore)
		coordinationWorker = projectcoordination.NewWorker(temporalClient, cfg.Temporal.TaskQueue, coordinationActivities)
	}
	projectService, err := project.NewServiceWithCoordinator(projectRepository, coordinatorClient)
	if err != nil {
		return nil, err
	}

	approvalRepository := approval.NewPgRepository(q)
	approvalService, err := approval.NewService(approvalRepository)
	if err != nil {
		return nil, err
	}

	auditRepository := audit.NewPgRepository(q)
	auditService, err := audit.NewService(auditRepository)
	if err != nil {
		return nil, err
	}

	tenantRepository := tenant.NewPgRepository(q, stores.Postgres)
	tenantService, err := tenant.NewService(tenantRepository, auditService)
	if err != nil {
		return nil, err
	}
	skillRepository := skill.NewPgRepository(stores.Postgres)
	skillService := skill.NewService(skillRepository)

	authRepository := auth.NewPgRepository(q)
	authService, err := auth.NewService(authRepository)
	if err != nil {
		return nil, err
	}
	authzRepository := authz.NewPgRepository(q)
	authzRecorder := authz.NewOperationLogDecisionRecorder(q)
	authorizer := authz.NewDBAuthorizer(authzRepository, authzRecorder)
	authzCenterRepository := authzcenter.NewPgRepository(q)
	authzCenterService := authzcenter.NewService(authzCenterRepository, authorizer)
	authzCenterHandler := authzcenter.NewHandler(authzCenterService, authService)

	poller := runtimepkg.NewPoller()
	runRepository := employee.NewPgRunRepository(q, stores.Postgres)
	runService, err := employee.NewDigitalEmployeeRunService(runRepository, runtimeCommands, auditService)
	if err != nil {
		return nil, err
	}
	runWritebackService, err := employee.NewDigitalEmployeeRunWritebackService(runRepository, auditService, runtimeEventRecorderAdapter{runtimeService: runtimeService})
	if err != nil {
		return nil, err
	}
	taskHandler := handlers.NewTaskHandler(taskService)
	runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller, authorizer)
	runtimeCommandWritebackHandler := handlers.NewRuntimeCommandWritebackHandler(runWritebackService)
	employeeHandler := employee.NewHandlerWithRunService(employeeService, runService)
	projectHandler := project.NewHandler(projectService)
	skillHandler := skill.NewHandler(skillService)
	tenantHandler := tenant.NewHandler(tenantService)
	runtimeHandler.SetConnectionRegistry(runtimeCommands)
	server := api.NewServerWithAuthzAndRuntimeSessionAuth(taskHandler, runtimeHandler, authService, authService, runtimeService, authorizer, authzCenterHandler)
	server.SetRuntimeCommandWritebackHandler(runtimeCommandWritebackHandler)
	server.SetTenantHandler(tenantHandler)
	server.SetEmployeeHandler(employeeHandler)
	server.SetProjectHandler(projectHandler)
	server.SetSkillHandler(skillHandler)

	return &Container{
		Queries:                        q,
		TaskService:                    taskService,
		RuntimeService:                 runtimeService,
		EmployeeService:                employeeService,
		ProjectService:                 projectService,
		ApprovalService:                approvalService,
		EmployeeRun:                    runService,
		EmployeeRunWriteback:           runWritebackService,
		SkillService:                   skillService,
		TenantService:                  tenantService,
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
		ProjectHandler:                 projectHandler,
		SkillHandler:                   skillHandler,
		TenantHandler:                  tenantHandler,
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
