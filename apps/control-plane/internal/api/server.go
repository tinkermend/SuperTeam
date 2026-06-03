package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/tenant"
)

type Server struct {
	router             *chi.Mux
	taskHandler        *handlers.TaskHandler
	runtimeHandler     *handlers.RuntimeHandler
	runtimeAuthService middleware.AuthService
	runtimeSessionAuth middleware.RuntimeSessionAuthService
	authService        *auth.Service
	authorizer         authz.Authorizer
	authzCenterHandler *authzcenter.HTTPHandler
	employeeHandler    *employee.HTTPHandler
	tenantHandler      *tenant.HTTPHandler
}

func NewServer(taskHandler *handlers.TaskHandler, runtimeHandler *handlers.RuntimeHandler, runtimeAuthService ...middleware.AuthService) *Server {
	var authService middleware.AuthService
	if len(runtimeAuthService) > 0 {
		authService = runtimeAuthService[0]
	}

	s := &Server{
		taskHandler:        taskHandler,
		runtimeHandler:     runtimeHandler,
		runtimeAuthService: authService,
	}

	s.registerRoutes()
	return s
}

func NewServerWithRuntimeSessionAuth(
	taskHandler *handlers.TaskHandler,
	runtimeHandler *handlers.RuntimeHandler,
	runtimeAuthService middleware.AuthService,
	runtimeSessionAuth middleware.RuntimeSessionAuthService,
) *Server {
	server := NewServer(taskHandler, runtimeHandler, runtimeAuthService)
	server.runtimeSessionAuth = runtimeSessionAuth
	server.registerRoutes()
	return server
}

func NewServerWithAuth(taskHandler *handlers.TaskHandler, runtimeHandler *handlers.RuntimeHandler, authService *auth.Service, runtimeAuthService ...middleware.AuthService) *Server {
	var runtimeAuth middleware.AuthService
	if len(runtimeAuthService) > 0 {
		runtimeAuth = runtimeAuthService[0]
	}
	return NewServerWithAuthz(taskHandler, runtimeHandler, authService, runtimeAuth, nil)
}

func NewServerWithAuthz(
	taskHandler *handlers.TaskHandler,
	runtimeHandler *handlers.RuntimeHandler,
	authService *auth.Service,
	runtimeAuthService middleware.AuthService,
	authorizer authz.Authorizer,
	authzCenterHandlers ...*authzcenter.HTTPHandler,
) *Server {
	server := NewServer(taskHandler, runtimeHandler, runtimeAuthService)
	server.authService = authService
	server.authorizer = authorizer
	if len(authzCenterHandlers) > 0 {
		server.authzCenterHandler = authzCenterHandlers[0]
	}
	if authorizer != nil && runtimeHandler != nil {
		runtimeHandler.SetAuthorizer(authorizer)
	}
	server.registerRoutes()
	return server
}

func NewServerWithAuthzAndRuntimeSessionAuth(
	taskHandler *handlers.TaskHandler,
	runtimeHandler *handlers.RuntimeHandler,
	authService *auth.Service,
	runtimeAuthService middleware.AuthService,
	runtimeSessionAuth middleware.RuntimeSessionAuthService,
	authorizer authz.Authorizer,
	authzCenterHandlers ...*authzcenter.HTTPHandler,
) *Server {
	server := NewServerWithAuthz(taskHandler, runtimeHandler, authService, runtimeAuthService, authorizer, authzCenterHandlers...)
	server.runtimeSessionAuth = runtimeSessionAuth
	server.registerRoutes()
	return server
}

func (s *Server) SetEmployeeHandler(employeeHandler *employee.HTTPHandler) {
	s.employeeHandler = employeeHandler
	if employeeHandler != nil {
		employeeHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) SetTenantHandler(tenantHandler *tenant.HTTPHandler) {
	s.tenantHandler = tenantHandler
	if tenantHandler != nil {
		tenantHandler.SetAuthorizer(s.authorizer)
	}
	s.registerRoutes()
}

func (s *Server) registerRoutes() {
	s.router = chi.NewRouter()
	s.router.Use(middleware.Recovery())
	s.router.Use(middleware.Logger())
	s.router.Use(middleware.CORS())

	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeHealthResponse(w)
	})
	if s.authService != nil {
		auth.HandlerFromMux(auth.NewHandler(s.authService, s.authorizer), s.router)
	}
	if s.authzCenterHandler != nil {
		authzcenter.HandlerFromMux(s.authzCenterHandler, s.router)
	}

	s.router.Route("/api/v1", func(r chi.Router) {
		r.Route("/tasks", func(r chi.Router) {
			r.Post("/", s.taskHandler.CreateTask)
			r.Get("/", s.taskHandler.ListTasks)
			r.Get("/{id}", s.taskHandler.GetTask)
			r.Put("/{id}/status", s.taskHandler.UpdateTaskStatus)
			r.Post("/{id}/cancel", s.taskHandler.CancelTask)
		})

		if s.employeeHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/digital-employees", s.employeeHandler.ListDigitalEmployees)
				r.Post("/digital-employees", s.employeeHandler.CreateDigitalEmployee)
				r.Get("/digital-employees/{employeeId}", s.employeeHandler.GetDigitalEmployee)
				r.Put("/digital-employees/{employeeId}/status", s.employeeHandler.UpdateDigitalEmployeeStatus)
				r.Get("/digital-employees/{employeeId}/execution-instance", s.employeeHandler.GetDigitalEmployeeExecutionInstance)
				r.Put("/digital-employees/{employeeId}/execution-instance", s.employeeHandler.UpsertDigitalEmployeeExecutionInstance)
				r.Post("/digital-employees/{employeeId}/config-revisions", s.employeeHandler.CreateDigitalEmployeeConfigRevision)
				r.Post("/digital-employees/{employeeId}/effective-configs/preview", s.employeeHandler.PreviewDigitalEmployeeEffectiveConfig)
				r.Post("/digital-employees/{employeeId}/effective-configs/approve", s.employeeHandler.ApproveDigitalEmployeeEffectiveConfig)
			})
		}

		if s.tenantHandler != nil {
			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/teams", s.tenantHandler.ListTeams)
				r.Post("/teams", s.tenantHandler.CreateTeam)
				r.Get("/teams/{teamId}/overview", s.tenantHandler.GetTeamOverview)
				r.Patch("/teams/{teamId}", s.tenantHandler.UpdateTeam)
				r.Get("/teams/{teamId}", s.tenantHandler.GetTeam)
				r.Post("/teams/{teamId}/disable", s.tenantHandler.DisableTeam)
				r.Post("/teams/{teamId}/archive", s.tenantHandler.ArchiveTeam)
				r.Post("/teams/{teamId}/restore", s.tenantHandler.RestoreTeam)
				r.Post("/teams/{teamId}/config-revisions", s.tenantHandler.CreateTeamConfigRevision)
				r.Get("/teams/{teamId}/config-revisions/current", s.tenantHandler.GetCurrentTeamConfigRevision)
				r.Get("/teams/{teamId}/members", s.tenantHandler.ListTeamMembers)
				r.Post("/teams/{teamId}/members", s.tenantHandler.AddTeamMember)
				r.Delete("/teams/{teamId}/members/{memberId}", s.tenantHandler.RemoveTeamMember)
				r.Get("/teams/{teamId}/member-role-requests", s.tenantHandler.ListTeamMemberRoleRequests)
				r.Post("/teams/{teamId}/member-role-requests", s.tenantHandler.CreateTeamMemberRoleRequest)
				r.Post("/teams/{teamId}/member-role-requests/{requestId}/approve", s.tenantHandler.ApproveTeamMemberRoleRequest)
				r.Post("/teams/{teamId}/member-role-requests/{requestId}/reject", s.tenantHandler.RejectTeamMemberRoleRequest)
			})
		}

		r.Route("/runtime", func(r chi.Router) {
			r.Get("/nodes", s.runtimeHandler.ListNodes)
			r.Get("/nodes/{id}", s.runtimeHandler.GetNodeByID)
			r.Post("/enrollments/hello", s.runtimeHandler.EnrollHello)
			r.Post("/enroll/hello", s.runtimeHandler.EnrollHello)

			r.Group(func(r chi.Router) {
				r.Use(middleware.ConsoleUserAuth(s.authService))
				r.Get("/enrollments", s.runtimeHandler.ListRuntimeEnrollments)
				r.Post("/enrollments/{enrollmentId}/approve", s.runtimeHandler.ApproveEnrollment)
				r.Post("/enrollments/{enrollmentId}/reject", s.runtimeHandler.RejectEnrollment)
				r.Post("/enrollments/{enrollmentId}/revoke", s.runtimeHandler.RevokeEnrollment)
			})

			r.Group(func(r chi.Router) {
				if s.runtimeAuthService != nil {
					r.Use(middleware.RuntimeAuth(s.runtimeAuthService))
				}
				r.Post("/register", s.runtimeHandler.RegisterNode)
			})

			r.Group(func(r chi.Router) {
				r.Use(middleware.RuntimeSessionAuth(s.runtimeSessionAuth))
				r.Post("/session/renew", s.runtimeHandler.RenewRuntimeSession)
				r.Post("/sessions/{sessionId}/renew", s.runtimeHandler.RenewRuntimeSession)
				r.Put("/nodes/{nodeId}/capabilities", s.runtimeHandler.UpsertCapabilities)
				r.Post("/capabilities", s.runtimeHandler.UpsertCapabilities)
				r.Get("/ws", s.runtimeHandler.WebSocket)
			})

			r.Group(func(r chi.Router) {
				if s.runtimeAuthService != nil || s.runtimeSessionAuth != nil {
					r.Use(middleware.RuntimeSessionOrLegacyAuth(s.runtimeSessionAuth, s.runtimeAuthService))
				}
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
