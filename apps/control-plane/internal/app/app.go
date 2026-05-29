package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/superteam/control-plane/internal/api"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/config"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/storage"
	"github.com/superteam/control-plane/internal/storage/queries"
	"github.com/superteam/control-plane/internal/task"
)

type Container struct {
	Queries        *queries.Queries
	TaskService    *task.Service
	RuntimeService *runtimepkg.Service
	Poller         *runtimepkg.Poller
	TaskHandler    *handlers.TaskHandler
	RuntimeHandler *handlers.RuntimeHandler
	Server         *api.Server
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

	poller := runtimepkg.NewPoller()
	taskHandler := handlers.NewTaskHandler(taskService)
	runtimeHandler := handlers.NewRuntimeHandler(runtimeService, taskService, poller)
	server := api.NewServer(taskHandler, runtimeHandler)

	return &Container{
		Queries:        q,
		TaskService:    taskService,
		RuntimeService: runtimeService,
		Poller:         poller,
		TaskHandler:    taskHandler,
		RuntimeHandler: runtimeHandler,
		Server:         server,
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
	defer container.Poller.Close()

	return container.Server.ListenAndServe(ctx, cfg.HTTP.Addr)
}
