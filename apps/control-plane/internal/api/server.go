package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/api/middleware"
)

type Server struct {
	router             *chi.Mux
	taskHandler        *handlers.TaskHandler
	runtimeHandler     *handlers.RuntimeHandler
	runtimeAuthService middleware.AuthService
}

func NewServer(taskHandler *handlers.TaskHandler, runtimeHandler *handlers.RuntimeHandler, runtimeAuthService ...middleware.AuthService) *Server {
	r := chi.NewRouter()

	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeHealthResponse(w)
	})

	var authService middleware.AuthService
	if len(runtimeAuthService) > 0 {
		authService = runtimeAuthService[0]
	}

	s := &Server{
		router:             r,
		taskHandler:        taskHandler,
		runtimeHandler:     runtimeHandler,
		runtimeAuthService: authService,
	}

	s.registerRoutes()
	return s
}

func (s *Server) registerRoutes() {
	s.router.Route("/api/v1", func(r chi.Router) {
		r.Route("/tasks", func(r chi.Router) {
			r.Post("/", s.taskHandler.CreateTask)
			r.Get("/", s.taskHandler.ListTasks)
			r.Get("/{id}", s.taskHandler.GetTask)
			r.Put("/{id}/status", s.taskHandler.UpdateTaskStatus)
			r.Post("/{id}/cancel", s.taskHandler.CancelTask)
		})

		r.Route("/runtime", func(r chi.Router) {
			r.Get("/nodes", s.runtimeHandler.ListNodes)
			r.Get("/nodes/{id}", s.runtimeHandler.GetNodeByID)

			r.Group(func(r chi.Router) {
				if s.runtimeAuthService != nil {
					r.Use(middleware.RuntimeAuth(s.runtimeAuthService))
				}
				r.Post("/register", s.runtimeHandler.RegisterNode)
				r.Post("/heartbeat", s.runtimeHandler.Heartbeat)
				r.Post("/tasks/claim", s.runtimeHandler.ClaimTask)
				r.Post("/tasks/{id}/events", s.runtimeHandler.PushEvents)
				r.Post("/tasks/{id}/complete", s.runtimeHandler.CompleteTask)
				r.Post("/tasks/{id}/fail", s.runtimeHandler.FailTask)
				r.Post("/tasks/{id}/lease", s.runtimeHandler.RenewLease)
			})
		})
	})
}

func (s *Server) Start(addr string) error {
	return s.ListenAndServe(context.Background(), addr)
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	httpServer := &http.Server{
		Addr:    addr,
		Handler: s.router,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		if err := httpServer.Shutdown(context.Background()); err != nil {
			return err
		}
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}
