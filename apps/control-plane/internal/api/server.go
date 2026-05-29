package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/api/middleware"
)

type Server struct {
	router      *chi.Mux
	taskHandler *handlers.TaskHandler
}

func NewServer(taskHandler *handlers.TaskHandler) *Server {
	r := chi.NewRouter()

	r.Use(middleware.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	s := &Server{
		router:      r,
		taskHandler: taskHandler,
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
	})
}

func (s *Server) Start(addr string) error {
	return http.ListenAndServe(addr, s.router)
}
