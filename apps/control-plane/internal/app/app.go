package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/superteam/control-plane/internal/api"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/employee"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/storage"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/task"
)

type Container struct {
	Queries         *queries.Queries
	TaskService     *task.Service
	RuntimeService  *runtimepkg.Service
	EmployeeService *employee.Service
	RuntimeCommands *runtimepkg.ConnectionRegistry
	AuthService     *auth.Service
	Authorizer      authz.Authorizer
	AuthzCenter     *authzcenter.Service
	Poller          *runtimepkg.Poller
	TaskHandler     *handlers.TaskHandler
	RuntimeHandler  *handlers.RuntimeHandler
	EmployeeHandler *employee.HTTPHandler
	AuthzHandler    *authzcenter.HTTPHandler
	Server          *api.Server
}

func NewHealthOnlyRouter() http.Handler {
	return api.NewHealthOnlyRouter()
}

func NewContainer(stores *storage.Clients) (*Container, error) {
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

	employeeRepository := employee.NewPgRepository(q)
	employeeService, err := employee.NewService(employeeRepository)
	if err != nil {
		return nil, err
	}

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
	runtimeCommands := runtimepkg.NewConnectionRegistry()
	taskHandler := handlers.NewTaskHandler(taskService)
	runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller, authorizer)
	employeeHandler := employee.NewHandler(employeeService)
	runtimeHandler.SetConnectionRegistry(runtimeCommands)
	server := api.NewServerWithAuthzAndRuntimeSessionAuth(taskHandler, runtimeHandler, authService, authService, runtimeService, authorizer, authzCenterHandler)
	server.SetEmployeeHandler(employeeHandler)

	return &Container{
		Queries:         q,
		TaskService:     taskService,
		RuntimeService:  runtimeService,
		EmployeeService: employeeService,
		RuntimeCommands: runtimeCommands,
		AuthService:     authService,
		Authorizer:      authorizer,
		AuthzCenter:     authzCenterService,
		Poller:          poller,
		TaskHandler:     taskHandler,
		RuntimeHandler:  runtimeHandler,
		EmployeeHandler: employeeHandler,
		AuthzHandler:    authzCenterHandler,
		Server:          server,
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

	container, err := NewContainer(stores)
	if err != nil {
		return err
	}
	return runContainer(ctx, container, cfg.HTTP.Addr)
}

func runContainer(ctx context.Context, container *Container, addr string) error {
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
